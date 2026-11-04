package modules

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
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

// settingsInputCancelScript is the Cancel button: it deletes the wait and the marker only for
// the button's own group, page and part, and only when the message the button sits on is the
// wait's own prompt or one of the refusal replies noted for that wait.
//
// KEYS[1] = the wait, KEYS[2] = the marker. ARGV = c, p, t, the ID of the message the button was
// pressed on. Returns 2 while a claim is held, 4 when there is no wait but a marker (the wait
// expired; the marker is deleted because the person has now been told), 0 when there is neither,
// 1 after deleting the wait and marker when c, p and t match and the message is the stored prompt
// (field m) or a noted refusal (field r<id>), and 3 otherwise (a stale prompt: the live wait is
// untouched).
var settingsInputCancelScript = redis.NewScript(`
if redis.call('HEXISTS', KEYS[1], 'k') == 1 then return 2 end
local f = redis.call('HMGET', KEYS[1], 'c', 'p', 't', 'm')
if not f[1] then
  if redis.call('EXISTS', KEYS[2]) == 1 then redis.call('DEL', KEYS[2]); return 4 end
  return 0
end
if f[1] == ARGV[1] and f[2] == ARGV[2] and f[3] == ARGV[3]
  and (f[4] == ARGV[4] or redis.call('HEXISTS', KEYS[1], 'r' .. ARGV[4]) == 1) then
  redis.call('DEL', KEYS[1]); redis.call('DEL', KEYS[2]); return 1
end
return 3
`)

// settingsInputNoteRefusalScript records the ID of a refusal reply (the message that carries a
// Cancel button besides the prompt) on the wait of the holder of the nonce, so that button can end
// the wait it belongs to. A Set deletes the hash, so the refusals of an earlier wait never match a
// newer one. At most 40 fields are kept, which bounds the hash.
//
// KEYS[1] = the wait. ARGV = the wait's nonce, the refusal message's ID.
var settingsInputNoteRefusalScript = redis.NewScript(`
if redis.call('HGET', KEYS[1], 'n') ~= ARGV[1] then return 0 end
if redis.call('HLEN', KEYS[1]) >= 40 then return 0 end
redis.call('HSET', KEYS[1], 'r' .. ARGV[2], '1')
return 1
`)

// settingsInputClaimScript claims the wait for the holder of the nonce, once.
//
// KEYS[1] = the wait. ARGV = the wait's nonce, the new claim. Returns 0 for a stale nonce, a
// gone wait or one already claimed, else the wait's remaining PTTL.
var settingsInputClaimScript = redis.NewScript(`
if redis.call('HGET', KEYS[1], 'n') ~= ARGV[1] then return 0 end
if redis.call('HEXISTS', KEYS[1], 'k') == 1 then return 0 end
redis.call('HSET', KEYS[1], 'k', ARGV[2])
return redis.call('PTTL', KEYS[1])
`)

// settingsInputFinishScript deletes the wait and the marker for the holder of the claim.
//
// KEYS[1] = the wait, KEYS[2] = the marker. ARGV[1] = the claim.
var settingsInputFinishScript = redis.NewScript(`
if redis.call('HGET', KEYS[1], 'k') == ARGV[1] then redis.call('DEL', KEYS[2]); return redis.call('DEL', KEYS[1]) end
return 0
`)

// settingsInputReleaseScript gives the wait back, with its remaining TTL, for the holder of the
// claim.
//
// KEYS[1] = the wait. ARGV[1] = the claim.
var settingsInputReleaseScript = redis.NewScript(`
if redis.call('HGET', KEYS[1], 'k') == ARGV[1] then return redis.call('HDEL', KEYS[1], 'k') end
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

// settingsInputValidPart reports whether part is a part of page the editor registers.
func settingsInputValidPart(page, part string) bool {
	def, ok := settingsEditPageByKey(page)
	return ok && settingsEditPartByCode(def, part) != nil
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
	return settingsInputRunCancel(ctx, userID, settingsInputCancelAnyScript)
}

// settingsInputRead reads userID's wait and marker in one round trip (HGETALL and GET in one
// pipeline). A hash with a missing field, an unknown page or part or a number that does not
// parse reads as no wait, and the user ID is logged once. The marker is "" when there is none.
func settingsInputRead(ctx context.Context, userID int64) (*settingsInputWait, string, error) {
	client, err := settingsInputClient()
	if err != nil {
		return nil, "", err
	}
	ctx, cancel := context.WithTimeout(ctx, settingsInputRedisTimeout)
	defer cancel()

	pipe := client.Pipeline()
	waitCmd := pipe.HGetAll(ctx, settingsInputKey(userID))
	markerCmd := pipe.Get(ctx, settingsInputDoneKey(userID))
	// redis.Nil from the marker's GET is the expected "no marker".
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return nil, "", err
	}
	if err := waitCmd.Err(); err != nil {
		return nil, "", err
	}
	marker, err := markerCmd.Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, "", err
	}

	fields := waitCmd.Val()
	if len(fields) == 0 {
		return nil, marker, nil
	}
	wait, ok := settingsInputWaitFromFields(fields)
	if !ok {
		log.Warnf("[Settings] the typed-reply wait of user %d is not readable and is ignored", userID)
		return nil, marker, nil
	}
	return wait, marker, nil
}

// settingsInputTakeMarker returns userID's marker and deletes it in one step (GETDEL), so
// exactly one reply is sent per marker even when two deliveries race.
func settingsInputTakeMarker(ctx context.Context, userID int64) (string, bool, error) {
	client, err := settingsInputClient()
	if err != nil {
		return "", false, err
	}
	ctx, cancel := context.WithTimeout(ctx, settingsInputRedisTimeout)
	defer cancel()

	marker, err := client.GetDel(ctx, settingsInputDoneKey(userID)).Result()
	if errors.Is(err, redis.Nil) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return marker, true, nil
}

// settingsInputClaim claims userID's wait for the holder of nonce, once. It returns the claim
// that settingsInputFinish and settingsInputRelease need; ok is false for a stale nonce, a
// wait that is gone or one already claimed, and then nothing was changed.
func settingsInputClaim(ctx context.Context, userID int64, nonce string) (string, bool, error) {
	client, err := settingsInputClient()
	if err != nil {
		return "", false, err
	}
	if !settingsInputNonceValid(nonce) {
		return "", false, nil
	}
	claim, err := newSettingsInputNonce()
	if err != nil {
		return "", false, err
	}
	ctx, cancel := context.WithTimeout(ctx, settingsInputRedisTimeout)
	defer cancel()

	left, err := settingsInputClaimScript.Run(ctx, client, []string{settingsInputKey(userID)}, nonce, claim).Int64()
	if err != nil {
		return "", false, err
	}
	if left == 0 {
		return "", false, nil
	}
	return claim, true, nil
}

// settingsInputSettle runs a finish or release script for the holder of claim on a context of
// its own: the one the update ran under may be the very thing that ended, and an unreleased
// claim would strand the wait until it expires.
func settingsInputSettle(ctx context.Context, script *redis.Script, keys []string, claim string) error {
	client, err := settingsInputClient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), settingsInputRedisTimeout)
	defer cancel()
	if err := script.Run(ctx, client, keys, claim).Err(); err != nil && !errors.Is(err, redis.Nil) {
		return err
	}
	return nil
}

// settingsInputFinish deletes userID's wait and marker, for the holder of claim only.
func settingsInputFinish(ctx context.Context, userID int64, claim string) error {
	return settingsInputSettle(ctx, settingsInputFinishScript,
		[]string{settingsInputKey(userID), settingsInputDoneKey(userID)}, claim)
}

// settingsInputRelease gives userID's wait back with its remaining TTL, for the holder of
// claim only.
func settingsInputRelease(ctx context.Context, userID int64, claim string) error {
	return settingsInputSettle(ctx, settingsInputReleaseScript, []string{settingsInputKey(userID)}, claim)
}

// settingsInputRunCancel runs a cancel script over userID's wait and marker and maps its code.
func settingsInputRunCancel(ctx context.Context, userID int64, script *redis.Script, argv ...any) (settingsInputCancelOutcome, error) {
	client, err := settingsInputClient()
	if err != nil {
		return settingsInputCancelNone, err
	}
	ctx, cancel := context.WithTimeout(ctx, settingsInputRedisTimeout)
	defer cancel()

	code, err := script.Run(ctx, client,
		[]string{settingsInputKey(userID), settingsInputDoneKey(userID)}, argv...).Int()
	if err != nil {
		return settingsInputCancelNone, err
	}
	switch outcome := settingsInputCancelOutcome(code); outcome {
	case settingsInputCancelDeleted, settingsInputCancelBusy, settingsInputCancelOther, settingsInputCancelExpired:
		return outcome, nil
	}
	return settingsInputCancelNone, nil
}

// settingsInputCancel is the Cancel button: it deletes userID's wait and marker only when the
// wait is for the button's group, page and part and the button sits on the wait's own prompt
// (promptMsgID, the pressed message's ID) or on a refusal reply noted for it, so a stale prompt
// never ends a newer wait, not even one for the same group, page and part.
func settingsInputCancel(
	ctx context.Context, userID, groupID int64, page, part string, promptMsgID int64,
) (settingsInputCancelOutcome, error) {
	return settingsInputRunCancel(ctx, userID, settingsInputCancelScript,
		strconv.FormatInt(groupID, 10), page, part, strconv.FormatInt(promptMsgID, 10))
}

// settingsInputNoteRefusal records messageID, a refusal reply of the wait of the holder of nonce,
// so its Cancel button is accepted. Without a usable nonce or ID it writes nothing.
func settingsInputNoteRefusal(ctx context.Context, userID int64, nonce string, messageID int64) error {
	client, err := settingsInputClient()
	if err != nil {
		return err
	}
	if messageID <= 0 || !settingsInputNonceValid(nonce) {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, settingsInputRedisTimeout)
	defer cancel()
	err = settingsInputNoteRefusalScript.Run(ctx, client,
		[]string{settingsInputKey(userID)}, nonce, strconv.FormatInt(messageID, 10)).Err()
	if err != nil && !errors.Is(err, redis.Nil) {
		return err
	}
	return nil
}

// settingsInputAlbumIDRe is the media group ID of an album: 1 to 20 ASCII digits.
var settingsInputAlbumIDRe = regexp.MustCompile(`^[0-9]{1,20}$`)

// settingsInputAlbumFirst reports whether this is the first item of an album that userID
// sent in the last 30 s (SET NX EX 30). A media group ID that is not 1 to 20 digits is not
// an album: it returns false and writes nothing.
func settingsInputAlbumFirst(ctx context.Context, userID int64, mediaGroupID string) (bool, error) {
	client, err := settingsInputClient()
	if err != nil {
		return false, err
	}
	if !settingsInputAlbumIDRe.MatchString(mediaGroupID) {
		return false, nil
	}
	ctx, cancel := context.WithTimeout(ctx, settingsInputRedisTimeout)
	defer cancel()
	return client.SetNX(ctx, settingsInputAlbumKey(userID, mediaGroupID), "1", settingsInputAlbumTTL).Result()
}

// settingsInputParseMarker reads a marker "c|p|t": a negative group ID, a page and a part the
// editor can wait for.
func settingsInputParseMarker(v string) (int64, string, string, bool) {
	fields := strings.Split(v, "|")
	if len(fields) != 3 {
		return 0, "", "", false
	}
	groupID, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil || groupID >= 0 || !settingsInputValidPart(fields[1], fields[2]) {
		return 0, "", "", false
	}
	return groupID, fields[1], fields[2], true
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

// LoadSettingsInput registers /cancel and the typed-reply door (handler group 1). LoadSettings
// calls it, so every environment that loads the settings module also gets the wait's command and
// the door that reads the reply.
func LoadSettingsInput(dispatcher *ext.Dispatcher) {
	helpers.WrapCommand(dispatcher, settingsCancelDesc, pipelineHandler(settingsModule.settingsCancel))
	dispatcher.AddHandlerToGroup(handlers.NewMessage(settingsInputIsPrivate, settingsModule.settingsInput), settingsInputGroup)
}
