package modules

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strconv"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
	"github.com/uasneppy/fuku_robot/fuku/utils/error_handling"
	"github.com/uasneppy/fuku_robot/fuku/utils/helpers"
)

// The message editor's wait for a typed reply (D-03, D-04, D-12, D-13). When the owner presses
// Set on a part of a group's message, the bot waits for that person's next private message.
// The wait lives only in Redis, so any replica can read it: one hash per user whose own
// PEXPIRE is the wait's expiry. There is no timer, no process map and no shutdown drain, and
// the hash holds IDs and codes only, never message text or a file ID.
//
// The keys sit in the fuku:settings:* family, outside the fuku:cache: prefix, so
// CLEAR_CACHE_ON_STARTUP keeps them.

const (
	// settingsInputKeyPrefix is the wait's hash: c group chat ID, p page key, t part, m prompt
	// message ID, n nonce, x expiry in unix ms, and k while a claim is held.
	settingsInputKeyPrefix = "fuku:settings:input:"
	// settingsInputDoneKeyPrefix is the marker "c|p|t". Every Set writes it and every deliberate
	// end deletes it with the wait, so a marker with no wait means only "the wait expired".
	settingsInputDoneKeyPrefix = "fuku:settings:inputdone:"
	// settingsInputAlbumKeyPrefix is the album memory: the first item of an album is handled and
	// its siblings are dropped.
	settingsInputAlbumKeyPrefix = "fuku:settings:inputalbum:"

	// settingsInputTTL is how long the bot waits for the reply.
	settingsInputTTL = 10 * time.Minute
	// settingsInputMarkerTTL is how long the "the wait expired" marker lives.
	settingsInputMarkerTTL = 15 * time.Minute
	// settingsInputAlbumTTL is how long the album memory lives.
	settingsInputAlbumTTL = 30 * time.Second
	// settingsInputRedisTimeout bounds one wait-store round trip.
	settingsInputRedisTimeout = 5 * time.Second
)

// errSettingsInputUnavailable means there is no Redis, so no wait can be kept. The editor's
// Set answers settings_edit_unavailable and changes nothing.
var errSettingsInputUnavailable = errors.New("settings input: redis is not available")

// errSettingsInputInvalid means a wait with a field no reader would accept was refused before
// any write.
var errSettingsInputInvalid = errors.New("settings input: invalid wait")

// settingsInputWait is one person's wait for one part of one group's message.
type settingsInputWait struct {
	GroupID     int64
	Page        string
	Part        string
	PromptMsgID int64
	Nonce       string
	ExpiresAt   time.Time
}

// settingsInputCancelOutcome is what a cancel found.
type settingsInputCancelOutcome int

// The values are the codes of settingsInputCancelScript.
const (
	// settingsInputCancelNone means there was neither a wait nor a marker.
	settingsInputCancelNone settingsInputCancelOutcome = 0
	// settingsInputCancelDeleted means the wait and its marker were deleted.
	settingsInputCancelDeleted settingsInputCancelOutcome = 1
	// settingsInputCancelBusy means a claim is held: a write is in flight, nothing was deleted.
	settingsInputCancelBusy settingsInputCancelOutcome = 2
	// settingsInputCancelOther means the live wait is for another group, page or part: it was
	// left untouched.
	settingsInputCancelOther settingsInputCancelOutcome = 3
	// settingsInputCancelExpired means there was no wait but its marker was: the wait expired.
	// The marker is deleted, because the person has now been told.
	settingsInputCancelExpired settingsInputCancelOutcome = 4
)

// settingsInputSetScript writes a wait and its marker and returns the earlier wait's fields.
//
// KEYS[1] = the wait, KEYS[2] = the marker. ARGV = ttl ms, c, p, t, m, n, x, marker value,
// marker ttl ms.
var settingsInputSetScript = redis.NewScript(`
local old = redis.call('HGETALL', KEYS[1])
redis.call('DEL', KEYS[1])
redis.call('HSET', KEYS[1], 'c', ARGV[2], 'p', ARGV[3], 't', ARGV[4], 'm', ARGV[5], 'n', ARGV[6], 'x', ARGV[7])
redis.call('PEXPIRE', KEYS[1], ARGV[1])
redis.call('SET', KEYS[2], ARGV[8], 'PX', ARGV[9])
return old
`)

// settingsInputCancelAnyScript is /cancel: it deletes the wait and the marker unless a claim is
// held.
//
// KEYS[1] = the wait, KEYS[2] = the marker. Returns 2 while a claim is held (nothing is
// deleted), 1 after deleting a wait, 0 when there was none (the marker is deleted too).
var settingsInputCancelAnyScript = redis.NewScript(`
if redis.call('HEXISTS', KEYS[1], 'k') == 1 then return 2 end
if redis.call('EXISTS', KEYS[1]) == 1 then
  redis.call('DEL', KEYS[1]); redis.call('DEL', KEYS[2]); return 1
end
redis.call('DEL', KEYS[2])
return 0
`)

// settingsInputKey is the wait's hash of userID.
func settingsInputKey(userID int64) string {
	return settingsInputKeyPrefix + strconv.FormatInt(userID, 10)
}

// settingsInputDoneKey is the marker of userID.
func settingsInputDoneKey(userID int64) string {
	return settingsInputDoneKeyPrefix + strconv.FormatInt(userID, 10)
}

// settingsInputAlbumKey is the album memory of one album of userID.
func settingsInputAlbumKey(userID int64, mediaGroupID string) string {
	return settingsInputAlbumKeyPrefix + strconv.FormatInt(userID, 10) + ":" + mediaGroupID
}

// newSettingsInputNonce returns 8 random bytes, hex-encoded: 16 characters.
func newSettingsInputNonce() (string, error) {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}

// settingsInputNonceValid reports whether n is exactly 16 lowercase hex characters.
func settingsInputNonceValid(n string) bool {
	if len(n) != 16 {
		return false
	}
	for _, r := range n {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

// settingsInputValidPage reports whether page is a page key the editor can wait for.
func settingsInputValidPage(page string) bool {
	switch page {
	case "w", "g", "r":
		return true
	}
	return false
}

// settingsInputValidPart reports whether part is a part of page the editor can wait for.
func settingsInputValidPart(page, part string) bool {
	if !settingsInputValidPage(page) {
		return false
	}
	switch part {
	case "x", "m", "b":
		return true
	}
	return false
}

// settingsInputMarkerValue is the marker's value for a wait: "c|p|t".
func settingsInputMarkerValue(groupID int64, page, part string) string {
	return strconv.FormatInt(groupID, 10) + "|" + page + "|" + part
}

// settingsInputWaitFromFields reads a wait from its hash fields. A missing field, a group ID
// that is not negative, an unknown page or part, a bad nonce or a number that does not parse
// is no wait.
func settingsInputWaitFromFields(fields map[string]string) (*settingsInputWait, bool) {
	groupID, err := strconv.ParseInt(fields["c"], 10, 64)
	if err != nil || groupID >= 0 {
		return nil, false
	}
	page, part := fields["p"], fields["t"]
	if !settingsInputValidPart(page, part) {
		return nil, false
	}
	promptID, err := strconv.ParseInt(fields["m"], 10, 64)
	if err != nil || promptID < 0 {
		return nil, false
	}
	nonce := fields["n"]
	if !settingsInputNonceValid(nonce) {
		return nil, false
	}
	expiry, err := strconv.ParseInt(fields["x"], 10, 64)
	if err != nil || expiry <= 0 {
		return nil, false
	}
	return &settingsInputWait{
		GroupID:     groupID,
		Page:        page,
		Part:        part,
		PromptMsgID: promptID,
		Nonce:       nonce,
		ExpiresAt:   time.UnixMilli(expiry),
	}, true
}

// settingsInputClient is the Redis client, or errSettingsInputUnavailable without one.
func settingsInputClient() (*redis.Client, error) {
	client := cache.GetRedisClient()
	if client == nil {
		return nil, errSettingsInputUnavailable
	}
	return client, nil
}

// settingsInputFieldsOf turns the flat HGETALL reply of a script into a map.
func settingsInputFieldsOf(raw []any) map[string]string {
	fields := make(map[string]string, len(raw)/2)
	for i := 0; i+1 < len(raw); i += 2 {
		key, _ := raw[i].(string)
		value, _ := raw[i+1].(string)
		fields[key] = value
	}
	return fields
}

// settingsInputSet writes userID's wait and its marker and returns the wait it replaced, nil
// when there was none (or one no reader would accept). A zero ExpiresAt means ten minutes
// from now. The caller compares the previous wait's group, page and part with the new ones to
// tell a replaced wait from a refreshed one.
func settingsInputSet(ctx context.Context, userID int64, w settingsInputWait) (*settingsInputWait, error) {
	client, err := settingsInputClient()
	if err != nil {
		return nil, err
	}
	if w.GroupID >= 0 || !settingsInputValidPart(w.Page, w.Part) || w.PromptMsgID < 0 || !settingsInputNonceValid(w.Nonce) {
		return nil, errSettingsInputInvalid
	}
	if w.ExpiresAt.IsZero() {
		w.ExpiresAt = time.Now().Add(settingsInputTTL)
	}
	ctx, cancel := context.WithTimeout(ctx, settingsInputRedisTimeout)
	defer cancel()

	raw, err := settingsInputSetScript.Run(ctx, client,
		[]string{settingsInputKey(userID), settingsInputDoneKey(userID)},
		settingsInputTTL.Milliseconds(),
		strconv.FormatInt(w.GroupID, 10), w.Page, w.Part,
		strconv.FormatInt(w.PromptMsgID, 10), w.Nonce, strconv.FormatInt(w.ExpiresAt.UnixMilli(), 10),
		settingsInputMarkerValue(w.GroupID, w.Page, w.Part), settingsInputMarkerTTL.Milliseconds(),
	).Slice()
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}
	previous, ok := settingsInputWaitFromFields(settingsInputFieldsOf(raw))
	if !ok {
		return nil, nil
	}
	return previous, nil
}

// settingsInputCancelAny deletes userID's wait and marker, whatever part the wait is for. A
// held claim means a write is in flight: nothing is deleted and the outcome is Busy.
func settingsInputCancelAny(ctx context.Context, userID int64) (settingsInputCancelOutcome, error) {
	client, err := settingsInputClient()
	if err != nil {
		return settingsInputCancelNone, err
	}
	ctx, cancel := context.WithTimeout(ctx, settingsInputRedisTimeout)
	defer cancel()

	code, err := settingsInputCancelAnyScript.Run(ctx, client,
		[]string{settingsInputKey(userID), settingsInputDoneKey(userID)}).Int()
	if err != nil {
		return settingsInputCancelNone, err
	}
	switch code {
	case 1:
		return settingsInputCancelDeleted, nil
	case 2:
		return settingsInputCancelBusy, nil
	}
	return settingsInputCancelNone, nil
}

// settingsInputPrivateOnly is the check of /cancel: it passes in the bot's private chat and
// never replies, so /cancel in a group is silent.
func settingsInputPrivateOnly() helpers.CheckFunc {
	return func(c *helpers.CommandContext) bool {
		return c.Chat != nil && c.Chat.Type == "private"
	}
}

// settingsCancelDesc is /cancel. It is not Disableable: it is only ever sent to the bot's
// private chat, where there is nothing to disable.
var settingsCancelDesc = helpers.CommandDescriptor{
	Name:           "cancel",
	RequiredChecks: []helpers.CheckFunc{settingsInputPrivateOnly()},
}

// settingsCancel is /cancel in the bot's private chat: it ends the sender's wait for a typed
// reply. Only the sender's own keys are touched. It answers settings_edit_cancelled when a
// wait was deleted and settings_edit_cancel_nothing for everything else, a wait left to a
// write in flight, a Redis error and no Redis included.
func (m moduleStruct) settingsCancel(b *gotgbot.Bot, ctx *ext.Context) error {
	defer error_handling.RecoverFromPanic("settingsCancel", "Settings")
	user := ctx.EffectiveUser
	msg := ctx.EffectiveMessage
	if user == nil || msg == nil {
		return ext.EndGroups
	}
	outcome, err := settingsInputCancelAny(context.Background(), user.Id)
	if err != nil && !errors.Is(err, errSettingsInputUnavailable) {
		log.Warnf("[Settings] /cancel for user %d failed: %v", user.Id, err)
	}
	key := "settings_edit_cancel_nothing"
	if err == nil && outcome == settingsInputCancelDeleted {
		key = "settings_edit_cancelled"
	}
	log.Debugf("[Settings] /cancel for user %d: outcome %d", user.Id, int(outcome))
	replyHTML(b, msg, settingsText(ctxTr(ctx), key))
	return ext.EndGroups
}

// LoadSettingsInput registers /cancel. LoadSettings calls it, so every environment that
// loads the settings module also gets the wait's command.
func LoadSettingsInput(dispatcher *ext.Dispatcher) {
	helpers.WrapCommand(dispatcher, settingsCancelDesc, pipelineHandler(settingsModule.settingsCancel))
}
