package modules

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/db/staff"
	"github.com/divkix/Alita_Robot/alita/i18n"
	"github.com/divkix/Alita_Robot/alita/utils/cache"
	"github.com/divkix/Alita_Robot/alita/utils/chat_status"
	"github.com/divkix/Alita_Robot/alita/utils/error_handling"
	"github.com/divkix/Alita_Robot/alita/utils/extraction"
)

// staffActionCardPrefix is the Redis key family of the confirm cards. It sits
// outside the alita:cache: prefix, so CLEAR_CACHE_ON_STARTUP keeps pending cards.
const staffActionCardPrefix = "alita:staff:act:"

// staffActionCardLifetime is how long an unconfirmed card stays usable (D-07). It
// is a variable so tests can shorten it.
var staffActionCardLifetime = 5 * time.Minute

// staffActionExpirySlack is how long after the card's lifetime the expiry timer
// fires, so it never runs before the card's own expires_at. It is a variable so
// tests can shorten it.
var staffActionExpirySlack = 2 * time.Second

// scheduleStaffActionExpiry edits the card to "Expired" shortly after its lifetime
// when nobody has acted on it. The timer lives on the replica that created the
// card; a restart loses it, and the lazy check on any later tap (and the hash TTL)
// covers that case.
func scheduleStaffActionExpiry(b *gotgbot.Bot, token string, chatID, msgID int64) {
	time.AfterFunc(staffActionCardLifetime+staffActionExpirySlack, func() {
		defer error_handling.RecoverFromPanic("staffActionExpiry", "StaffActions")
		expireStaffActionCard(b, token, chatID, msgID)
	})
}

// expireStaffActionCard is the timer's work: it moves a still-pending card to
// expired and edits it, buttons removed. The move is the same Redis compare-and-set
// a tap uses, so the timer and a late tap never both edit the card, and a card that
// was confirmed or cancelled, on this replica or another, is left alone (D-07).
func expireStaffActionCard(b *gotgbot.Bot, token string, chatID, msgID int64) {
	card, err := loadStaffActionCard(token)
	if err != nil {
		log.Warnf("[StaffActions] expiry load of card %s: %v", token, err)
		return
	}
	if card == nil || card.State != staffCardPending {
		return
	}
	claim, _, err := transitionStaffActionCard(token, 0, staffCardExpired, false)
	if err != nil {
		log.Warnf("[StaffActions] expire card %s: %v", token, err)
		return
	}
	if claim != staffClaimOK && claim != staffClaimExpired {
		return
	}
	tr := staffChatTranslator(card.StaffChat)
	text, _ := tr.GetString("staff_act_card_expired_text")
	if err := editStaffActionMessage(b, chatID, msgID, staffActionHeader(tr, card)+"\n\n"+text); err != nil {
		log.Warnf("[StaffActions] edit expired card in chat %d: %v", chatID, err)
	}
}

// staffTargetLockPrefix is the Redis key family of the per-target fan-out lock.
const staffTargetLockPrefix = "alita:staff:lock:target:"

// staffTargetLockTTL is the safety net that frees a target lock whose run died.
var staffTargetLockTTL = 30 * time.Minute

// releaseStaffTargetLockScript deletes the lock only while it still holds the
// releasing card's token, so a card whose lock expired never frees a newer card's
// lock.
var releaseStaffTargetLockScript = redis.NewScript(`
	if redis.call("GET", KEYS[1]) == ARGV[1] then
		return redis.call("DEL", KEYS[1])
	end
	return 0
`)

// staffLinksSignature identifies a set of linked groups: the first 16 hex
// characters of the sha256 of the ascending group chat IDs joined with commas. A
// card stores it when it is shown and Confirm compares it again, so a group that
// was swapped for another is caught even when the count is the same.
func staffLinksSignature(links []models.StaffGroupLink) string {
	ids := make([]int64, len(links))
	for i, link := range links {
		ids[i] = link.GroupChatID
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.FormatInt(id, 10)
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, ",")))
	return hex.EncodeToString(sum[:])[:16]
}

// staffTargetLockKey is the Redis key of a target's fan-out lock.
func staffTargetLockKey(target int64) string {
	return staffTargetLockPrefix + strconv.FormatInt(target, 10)
}

// acquireStaffTargetLock takes the per-target lock for the card with token: SET NX
// with the token as value and a safety TTL. It reports false when another card
// holds it. Two staff actions on one person never run at once, so a restrict from
// one can not replace the ban of the other.
func acquireStaffTargetLock(target int64, token string) (bool, error) {
	client := cache.GetRedisClient()
	if client == nil {
		return false, errStaffCardNoRedis
	}
	ctx, cancel := cache.ContextWithTimeout()
	defer cancel()
	return client.SetNX(ctx, staffTargetLockKey(target), token, staffTargetLockTTL).Result()
}

// staffTargetLockHolder is the token that holds the target's lock, "" when none.
func staffTargetLockHolder(target int64) (string, error) {
	client := cache.GetRedisClient()
	if client == nil {
		return "", errStaffCardNoRedis
	}
	ctx, cancel := cache.ContextWithTimeout()
	defer cancel()
	holder, err := client.Get(ctx, staffTargetLockKey(target)).Result()
	if errors.Is(err, redis.Nil) {
		return "", nil
	}
	return holder, err
}

// releaseStaffTargetLock frees the target's lock when the card with token holds
// it. A failure is logged and left to the lock's TTL.
func releaseStaffTargetLock(target int64, token string) {
	client := cache.GetRedisClient()
	if client == nil {
		return
	}
	ctx, cancel := cache.ContextWithTimeout()
	defer cancel()
	if err := releaseStaffTargetLockScript.Run(ctx, client, []string{staffTargetLockKey(target)}, token).Err(); err != nil && !errors.Is(err, redis.Nil) {
		log.Warnf("[StaffActions] release target lock %d: %v", target, err)
	}
}

const (
	// staffActionCardGrace keeps a pending card's key a little past its expiry, so
	// a late tap can still be answered "expired".
	staffActionCardGrace = time.Minute
	// staffActionCardDoneTTL is how long a card in a terminal state is kept.
	staffActionCardDoneTTL = time.Hour
)

// Card states. A card moves only from pending, and only once.
const (
	staffCardPending   = "pending"
	staffCardRunning   = "running"
	staffCardDone      = "done"
	staffCardCancelled = "cancelled"
	staffCardExpired   = "expired"
	staffCardAborted   = "aborted"
)

// staffActionCard is one staff action waiting for, or past, its Confirm tap. It
// lives in the Redis hash alita:staff:act:<token>; the callback data carries only
// the token.
type staffActionCard struct {
	Token     string
	State     string
	Issuer    int64
	StaffChat int64
	Kind      staffActionKind
	Target    int64
	// TargetName is the stored display name; empty when the bot has never seen the
	// target.
	TargetName string
	Reason     string
	GroupCount int
	// LinksSig is staffLinksSignature of the linked groups the card was shown for.
	LinksSig string
	// ExpiresAt is a Unix time in milliseconds.
	ExpiresAt int64
	// DurationSec is the length of a ban or mute in seconds; 0 means permanent. The
	// end date is not stored: it is computed once at Confirm.
	DurationSec int64
	// DurationAmount and DurationUnit (m, h, d or w) are the duration as typed, for
	// the card's label.
	DurationAmount int64
	DurationUnit   string
	// OverLimit marks a duration typed above 366 days, which is applied as permanent.
	OverLimit bool
}

var staffActionTokenRe = regexp.MustCompile(`^[0-9a-f]{16}$`)

var (
	// createStaffCardScript creates the hash and its expiry only when the key is
	// free, so a token collision never overwrites a live card. ARGV[1] is the TTL in
	// milliseconds, followed by field/value pairs.
	createStaffCardScript = redis.NewScript(`
		if redis.call("EXISTS", KEYS[1]) == 1 then
			return 0
		end
		for i = 2, #ARGV, 2 do
			redis.call("HSET", KEYS[1], ARGV[i], ARGV[i + 1])
		end
		redis.call("PEXPIRE", KEYS[1], ARGV[1])
		return 1
	`)

	// transitionStaffCardScript moves a card out of pending exactly once. ARGV is the
	// presser ID, the target state, now in milliseconds, the terminal TTL in
	// milliseconds and "1" when the presser must be the issuer. It returns a code
	// and the state: 0 missing, 1 moved, 2 not pending, 3 not the issuer, 4 expired.
	transitionStaffCardScript = redis.NewScript(`
		local state = redis.call("HGET", KEYS[1], "state")
		if not state then
			return {0, ""}
		end
		if state ~= "pending" then
			return {2, state}
		end
		local expires = tonumber(redis.call("HGET", KEYS[1], "expires_at"))
		if expires and tonumber(ARGV[3]) >= expires then
			redis.call("HSET", KEYS[1], "state", "expired")
			redis.call("PEXPIRE", KEYS[1], ARGV[4])
			return {4, "expired"}
		end
		if ARGV[5] == "1" and redis.call("HGET", KEYS[1], "issuer") ~= ARGV[1] then
			return {3, state}
		end
		redis.call("HSET", KEYS[1], "state", ARGV[2])
		redis.call("PEXPIRE", KEYS[1], ARGV[4])
		return {1, ARGV[2]}
	`)
)

// errStaffCardNoRedis means there is no Redis client to keep the card in.
var errStaffCardNoRedis = errors.New("staff action card: redis is not available")

// staffCardKey is the Redis key of a card.
func staffCardKey(token string) string {
	return staffActionCardPrefix + token
}

// newStaffActionToken returns 8 random bytes, hex-encoded: 16 characters.
func newStaffActionToken() (string, error) {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}

// parseStaffActionToken accepts exactly 16 lowercase hex characters.
func parseStaffActionToken(raw string) (string, bool) {
	if !staffActionTokenRe.MatchString(raw) {
		return "", false
	}
	return raw, true
}

// saveStaffActionCard stores the card as pending under a fresh token and fills in
// Token, State and ExpiresAt. A token collision is retried up to three times.
func saveStaffActionCard(card *staffActionCard) error {
	client := cache.GetRedisClient()
	if client == nil {
		return errStaffCardNoRedis
	}
	card.State = staffCardPending
	card.ExpiresAt = time.Now().Add(staffActionCardLifetime).UnixMilli()
	ttl := staffActionCardLifetime + staffActionCardGrace

	for attempt := 0; attempt < 3; attempt++ {
		token, err := newStaffActionToken()
		if err != nil {
			return err
		}
		ctx, cancel := cache.ContextWithTimeout()
		created, err := createStaffCardScript.Run(ctx, client, []string{staffCardKey(token)},
			ttl.Milliseconds(),
			"state", card.State,
			"issuer", card.Issuer,
			"staff_chat", card.StaffChat,
			"action", string(card.Kind),
			"target", card.Target,
			"target_name", card.TargetName,
			"reason", card.Reason,
			"group_count", card.GroupCount,
			"links_sig", card.LinksSig,
			"expires_at", card.ExpiresAt,
			"duration_s", card.DurationSec,
			"dur_n", card.DurationAmount,
			"dur_u", card.DurationUnit,
			"over_limit", staffBoolField(card.OverLimit),
		).Int()
		cancel()
		if err != nil {
			return err
		}
		if created == 1 {
			card.Token = token
			return nil
		}
	}
	return errors.New("staff action card: could not find a free token")
}

// staffBoolField stores a bool as the hash value "1" or "0".
func staffBoolField(v bool) string {
	if v {
		return "1"
	}
	return "0"
}

// loadStaffActionCard reads a card. A missing or expired-away key gives (nil,
// nil).
func loadStaffActionCard(token string) (*staffActionCard, error) {
	client := cache.GetRedisClient()
	if client == nil {
		return nil, errStaffCardNoRedis
	}
	ctx, cancel := cache.ContextWithTimeout()
	defer cancel()
	fields, err := client.HGetAll(ctx, staffCardKey(token)).Result()
	if err != nil {
		return nil, err
	}
	if len(fields) == 0 {
		return nil, nil
	}
	card := &staffActionCard{
		Token:      token,
		State:      fields["state"],
		Kind:       staffActionKind(fields["action"]),
		TargetName: fields["target_name"],
		Reason:     fields["reason"],
		LinksSig:   fields["links_sig"],
	}
	ints := []struct {
		name string
		dst  *int64
	}{
		{"issuer", &card.Issuer},
		{"staff_chat", &card.StaffChat},
		{"target", &card.Target},
		{"expires_at", &card.ExpiresAt},
	}
	for _, field := range ints {
		value, err := strconv.ParseInt(fields[field.name], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("staff action card %s: bad %s: %w", token, field.name, err)
		}
		*field.dst = value
	}
	count, err := strconv.Atoi(fields["group_count"])
	if err != nil {
		return nil, fmt.Errorf("staff action card %s: bad group_count: %w", token, err)
	}
	card.GroupCount = count

	// The duration fields are optional: a card saved before they existed has none and
	// reads as permanent.
	for _, field := range []struct {
		name string
		dst  *int64
	}{
		{"duration_s", &card.DurationSec},
		{"dur_n", &card.DurationAmount},
	} {
		raw := fields[field.name]
		if raw == "" {
			continue
		}
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || value < 0 {
			return nil, fmt.Errorf("staff action card %s: bad %s: %q", token, field.name, raw)
		}
		*field.dst = value
	}
	card.DurationUnit = fields["dur_u"]
	card.OverLimit = fields["over_limit"] == "1"
	return card, nil
}

// staffCardClaim is the answer of transitionStaffActionCard.
type staffCardClaim int

const (
	// staffClaimOK means the card moved to the requested state.
	staffClaimOK staffCardClaim = iota
	// staffClaimMissing means there is no such card.
	staffClaimMissing
	// staffClaimWrongState means the card was no longer pending.
	staffClaimWrongState
	// staffClaimNotIssuer means someone other than the issuer pressed the button.
	staffClaimNotIssuer
	// staffClaimExpired means the card's time was up; it is now expired.
	staffClaimExpired
)

// transitionStaffActionCard asks Redis to move the card from pending to the given
// state. The issuer comparison and the expiry check happen inside the script, so
// two taps, or two replicas, can never both win. The returned string is the state
// the card is in afterwards.
func transitionStaffActionCard(token string, presser int64, to string, checkIssuer bool) (staffCardClaim, string, error) {
	client := cache.GetRedisClient()
	if client == nil {
		return staffClaimMissing, "", errStaffCardNoRedis
	}
	flag := "0"
	if checkIssuer {
		flag = "1"
	}
	ctx, cancel := cache.ContextWithTimeout()
	defer cancel()
	raw, err := transitionStaffCardScript.Run(ctx, client, []string{staffCardKey(token)},
		presser, to, time.Now().UnixMilli(), staffActionCardDoneTTL.Milliseconds(), flag).Slice()
	if err != nil {
		return staffClaimMissing, "", err
	}
	if len(raw) != 2 {
		return staffClaimMissing, "", fmt.Errorf("staff action card: unexpected script answer %v", raw)
	}
	code, _ := raw[0].(int64)
	state, _ := raw[1].(string)
	switch code {
	case 1:
		return staffClaimOK, state, nil
	case 2:
		return staffClaimWrongState, state, nil
	case 3:
		return staffClaimNotIssuer, state, nil
	case 4:
		return staffClaimExpired, state, nil
	}
	return staffClaimMissing, state, nil
}

// setStaffActionCardState writes a state without a transition check and shortens
// the key's life to the terminal TTL. It is used for the states that follow
// running (done, aborted).
func setStaffActionCardState(token, state string) error {
	client := cache.GetRedisClient()
	if client == nil {
		return errStaffCardNoRedis
	}
	ctx, cancel := cache.ContextWithTimeout()
	defer cancel()
	pipe := client.Pipeline()
	pipe.HSet(ctx, staffCardKey(token), "state", state)
	pipe.PExpire(ctx, staffCardKey(token), staffActionCardDoneTTL)
	_, err := pipe.Exec(ctx)
	return err
}

// deleteStaffActionCard drops a card that was never shown.
func deleteStaffActionCard(token string) {
	client := cache.GetRedisClient()
	if client == nil || token == "" {
		return
	}
	ctx, cancel := cache.ContextWithTimeout()
	defer cancel()
	if err := client.Del(ctx, staffCardKey(token)).Err(); err != nil {
		log.Warnf("[StaffActions] delete card %s: %v", token, err)
	}
}

// staffActionCardKeyboard builds the card's Confirm and Cancel buttons on one
// row (D-05). ok is false when any button data or label cannot be produced, in
// which case the card must not be sent: an empty callback or label makes Telegram
// reject the whole message.
func staffActionCardKeyboard(tr *i18n.Translator, token string) (gotgbot.InlineKeyboardMarkup, bool) {
	var keyboard gotgbot.InlineKeyboardMarkup
	confirmData := encodeCallbackData(staffCallbackNamespace, map[string]string{"a": staffActRunConfirm, "t": token})
	cancelData := encodeCallbackData(staffCallbackNamespace, map[string]string{"a": staffActRunCancel, "t": token})
	confirmLabel, _ := tr.GetString("staff_btn_confirm")
	cancelLabel, _ := tr.GetString("staff_btn_cancel")
	if confirmData == "" || cancelData == "" || confirmLabel == "" || cancelLabel == "" {
		return keyboard, false
	}
	keyboard.InlineKeyboard = [][]gotgbot.InlineKeyboardButton{{
		{Text: confirmLabel, CallbackData: confirmData},
		{Text: cancelLabel, CallbackData: cancelData},
	}}
	return keyboard, true
}

// staffActionCardText is the text of an unconfirmed card: the header and how many
// linked groups it applies to.
func staffActionCardText(tr *i18n.Translator, card *staffActionCard) string {
	applies, _ := tr.GetString("staff_act_card_applies", i18n.TranslationParams{"count": card.GroupCount})
	return staffActionHeader(tr, card) + "\n\n" + applies
}

// answerStaffCardExpiredMissing answers a tap on a card that no longer exists and
// clears the stale buttons.
func answerStaffCardExpiredMissing(b *gotgbot.Bot, query *gotgbot.CallbackQuery, tr *i18n.Translator) {
	toast, _ := tr.GetString("staff_act_card_expired")
	answerStaffCallback(b, query, toast, false)
	text, _ := tr.GetString("staff_act_card_expired_text")
	chat := query.Message.GetChat()
	if err := editStaffActionMessage(b, chat.Id, query.Message.GetMessageId(), text); err != nil {
		log.Warnf("[StaffActions] edit expired card in chat %d: %v", chat.Id, err)
	}
}

// answerStaffCardClaim answers every non-OK claim of a Confirm or Cancel tap. A
// tap that finds the card already handled never edits the message, so it cannot
// overwrite a live summary.
func answerStaffCardClaim(
	b *gotgbot.Bot,
	query *gotgbot.CallbackQuery,
	tr *i18n.Translator,
	card *staffActionCard,
	claim staffCardClaim,
	state string,
) {
	switch claim {
	case staffClaimNotIssuer:
		text, _ := tr.GetString("staff_act_card_issuer_only")
		answerStaffCallback(b, query, text, true)
	case staffClaimWrongState:
		var text string
		if state == staffCardExpired {
			text, _ = tr.GetString("staff_act_card_expired")
		} else {
			text, _ = tr.GetString("staff_act_card_handled")
		}
		answerStaffCallback(b, query, text, false)
	case staffClaimExpired:
		toast, _ := tr.GetString("staff_act_card_expired")
		answerStaffCallback(b, query, toast, false)
		expired, _ := tr.GetString("staff_act_card_expired_text")
		chat := query.Message.GetChat()
		if err := editStaffActionMessage(b, chat.Id, query.Message.GetMessageId(), staffActionHeader(tr, card)+"\n\n"+expired); err != nil {
			log.Warnf("[StaffActions] edit expired card in chat %d: %v", chat.Id, err)
		}
	default:
		answerStaffCardExpiredMissing(b, query, tr)
	}
}

// abortStaffActionCard ends a confirmed card without acting: the state becomes
// aborted and the card shows why.
func abortStaffActionCard(b *gotgbot.Bot, tr *i18n.Translator, card *staffActionCard, chatID, msgID int64, text string) {
	if err := setStaffActionCardState(card.Token, staffCardAborted); err != nil {
		log.Warnf("[StaffActions] mark card %s aborted: %v", card.Token, err)
	}
	if err := editStaffActionMessage(b, chatID, msgID, staffActionHeader(tr, card)+"\n\n"+text); err != nil {
		log.Warnf("[StaffActions] edit aborted card in chat %d: %v", chatID, err)
	}
}

// loadStaffCardForTap resolves the card a button names. On any failure it has
// already answered the tap and returns nil.
func loadStaffCardForTap(
	b *gotgbot.Bot,
	query *gotgbot.CallbackQuery,
	tr *i18n.Translator,
	fields map[string]string,
) *staffActionCard {
	token, ok := parseStaffActionToken(fields["t"])
	if !ok {
		text, _ := tr.GetString("staff_cb_expired")
		answerStaffCallback(b, query, text, false)
		return nil
	}
	card, err := loadStaffActionCard(token)
	if err != nil {
		log.Warnf("[StaffActions] load card: %v", err)
		text, _ := tr.GetString("staff_check_failed")
		answerStaffCallback(b, query, text, true)
		return nil
	}
	if card == nil {
		answerStaffCardExpiredMissing(b, query, tr)
		return nil
	}
	if query.Message.GetChat().Id != card.StaffChat {
		text, _ := tr.GetString("staff_cb_denied")
		answerStaffCallback(b, query, text, true)
		return nil
	}
	return card
}

// staffActionCancel handles the Cancel button. Only the issuer's tap cancels, and
// only while the card is pending. The card is edited to say who cancelled and its
// buttons go away; it is never deleted (D-08), so the staff can see an action was
// considered and dropped.
func (m moduleStruct) staffActionCancel(
	b *gotgbot.Bot,
	query *gotgbot.CallbackQuery,
	tr *i18n.Translator,
	fields map[string]string,
) error {
	card := loadStaffCardForTap(b, query, tr, fields)
	if card == nil {
		return ext.EndGroups
	}

	claim, state, err := transitionStaffActionCard(card.Token, query.From.Id, staffCardCancelled, true)
	if err != nil {
		log.Warnf("[StaffActions] cancel card %s: %v", card.Token, err)
		text, _ := tr.GetString("staff_check_failed")
		answerStaffCallback(b, query, text, true)
		return ext.EndGroups
	}
	if claim != staffClaimOK {
		answerStaffCardClaim(b, query, tr, card, claim, state)
		return ext.EndGroups
	}
	answerStaffCallback(b, query, "", false)

	staffTr := staffChatTranslator(card.StaffChat)
	cancelled, _ := staffTr.GetString("staff_act_card_cancelled", i18n.TranslationParams{"name": staffUserToken})
	cancelled = strings.Replace(cancelled, staffUserToken, html.EscapeString(staffFullName(&query.From)), 1)
	chat := query.Message.GetChat()
	if err := editStaffActionMessage(b, chat.Id, query.Message.GetMessageId(), staffActionHeader(staffTr, card)+"\n\n"+cancelled); err != nil {
		log.Warnf("[StaffActions] edit cancelled card in chat %d: %v", chat.Id, err)
	}
	return ext.EndGroups
}

// staffFullName is a user's first name followed by their last name, if any.
func staffFullName(u *gotgbot.User) string {
	if u.LastName == "" {
		return u.FirstName
	}
	return u.FirstName + " " + u.LastName
}

// takeStaffTargetLock takes the card's target lock for a Confirm tap. When it
// returns false it has already answered the tap and the card is left untouched:
//   - the lock is held by this same card, so a run of this very card is starting or
//     in flight and the tap is a repeat: "already handled";
//   - the lock is held by another card: the "target busy" alert, the card stays
//     pending and usable;
//   - Redis failed: the "could not check" alert.
func takeStaffTargetLock(b *gotgbot.Bot, query *gotgbot.CallbackQuery, tr *i18n.Translator, card *staffActionCard) bool {
	// A holder that releases between the two reads is retried once, so a free target
	// is never reported busy.
	for attempt := 0; attempt < 2; attempt++ {
		taken, err := acquireStaffTargetLock(card.Target, card.Token)
		if err != nil {
			log.Warnf("[StaffActions] target lock for card %s: %v", card.Token, err)
			text, _ := tr.GetString("staff_check_failed")
			answerStaffCallback(b, query, text, true)
			return false
		}
		if taken {
			return true
		}
		holder, err := staffTargetLockHolder(card.Target)
		if err != nil {
			log.Warnf("[StaffActions] read target lock for card %s: %v", card.Token, err)
			text, _ := tr.GetString("staff_check_failed")
			answerStaffCallback(b, query, text, true)
			return false
		}
		switch holder {
		case "":
			continue
		case card.Token:
			text, _ := tr.GetString("staff_act_card_handled")
			answerStaffCallback(b, query, text, false)
			return false
		}
		break
	}
	text, _ := tr.GetString("staff_act_target_busy")
	answerStaffCallback(b, query, text, true)
	return false
}

// staffActionConfirm handles the Confirm button. Only the issuer's tap moves the
// card to running, and only once; every live check then runs before the first
// write, and each failure ends the card with the reason shown on it.
func (m moduleStruct) staffActionConfirm(
	b *gotgbot.Bot,
	query *gotgbot.CallbackQuery,
	tr *i18n.Translator,
	fields map[string]string,
) error {
	card := loadStaffCardForTap(b, query, tr, fields)
	if card == nil {
		return ext.EndGroups
	}

	// Two staff actions on one person never run at once (A9). Only a live pending
	// card from its own issuer asks for the target lock; any other tap goes straight
	// to the compare-and-set, which answers handled, expired or issuer-only.
	lockHeld, runStarted := false, false
	if card.State == staffCardPending && time.Now().UnixMilli() < card.ExpiresAt {
		if card.Issuer != query.From.Id {
			text, _ := tr.GetString("staff_act_card_issuer_only")
			answerStaffCallback(b, query, text, true)
			return ext.EndGroups
		}
		if !takeStaffTargetLock(b, query, tr, card) {
			return ext.EndGroups
		}
		lockHeld = true
	}
	// Every path that ends before the run starts frees the lock; once it starts, the
	// run goroutine frees it after the final delivery.
	defer func() {
		if lockHeld && !runStarted {
			releaseStaffTargetLock(card.Target, card.Token)
		}
	}()

	claim, state, err := transitionStaffActionCard(card.Token, query.From.Id, staffCardRunning, true)
	if err != nil {
		log.Warnf("[StaffActions] confirm card %s: %v", card.Token, err)
		text, _ := tr.GetString("staff_check_failed")
		answerStaffCallback(b, query, text, true)
		return ext.EndGroups
	}
	if claim != staffClaimOK {
		answerStaffCardClaim(b, query, tr, card, claim, state)
		return ext.EndGroups
	}
	answerStaffCallback(b, query, "", false)

	staffTr := staffChatTranslator(card.StaffChat)
	staffChat := query.Message.GetChat()
	msgID := query.Message.GetMessageId()

	group, err := staff.GetStaffGroupFresh(staffChat.Id)
	if err != nil {
		text, _ := staffTr.GetString("staff_act_abort_check_failed")
		abortStaffActionCard(b, staffTr, card, staffChat.Id, msgID, text)
		return ext.EndGroups
	}
	if group == nil {
		text, _ := staffTr.GetString("staff_act_abort_not_staff")
		abortStaffActionCard(b, staffTr, card, staffChat.Id, msgID, text)
		return ext.EndGroups
	}

	inStaff, err := chat_status.IsUserInChatWithError(b, &staffChat, card.Issuer)
	if err != nil {
		log.Warnf("[StaffActions] issuer membership check in chat %d: %v", staffChat.Id, err)
		text, _ := staffTr.GetString("staff_act_abort_check_failed")
		abortStaffActionCard(b, staffTr, card, staffChat.Id, msgID, text)
		return ext.EndGroups
	}
	if !inStaff {
		text, _ := staffTr.GetString("staff_act_abort_issuer_left")
		abortStaffActionCard(b, staffTr, card, staffChat.Id, msgID, text)
		return ext.EndGroups
	}

	links, err := staff.ListLinksByStaffFresh(staffChat.Id)
	if err != nil {
		text, _ := staffTr.GetString("staff_act_abort_check_failed")
		abortStaffActionCard(b, staffTr, card, staffChat.Id, msgID, text)
		return ext.EndGroups
	}
	if len(links) == 0 {
		text, _ := staffTr.GetString("staff_act_no_links")
		abortStaffActionCard(b, staffTr, card, staffChat.Id, msgID, text)
		return ext.EndGroups
	}
	// The card told the issuer "applies to N linked groups" (D-06). Acting on a
	// different set would act on something they never confirmed, so a changed count
	// or a changed set of groups, even at the same count, ends the card (STAFF-04).
	// A card without a stored signature is compared by count only.
	if len(links) != card.GroupCount || (card.LinksSig != "" && staffLinksSignature(links) != card.LinksSig) {
		text, _ := staffTr.GetString("staff_act_abort_links_changed",
			i18n.TranslationParams{"old": card.GroupCount, "new": len(links)})
		abortStaffActionCard(b, staffTr, card, staffChat.Id, msgID, text)
		return ext.EndGroups
	}

	// The end date is fixed here, once: this one value goes to every linked group
	// and to every retry, so all groups end at the same moment. A card without a
	// duration is permanent (until_date 0).
	var newUntil int64
	if card.DurationSec > 0 {
		until, ok := extraction.TemporaryUntilDate(time.Now().Unix(), card.DurationSec)
		if !ok {
			text, _ := staffTr.GetString("staff_act_abort_check_failed")
			abortStaffActionCard(b, staffTr, card, staffChat.Id, msgID, text)
			return ext.EndGroups
		}
		newUntil = until
	}

	if err := editStaffActionMessage(b, staffChat.Id, msgID, renderStaffActionSummary(staffTr, card, pendingResults(links))); err != nil {
		log.Warnf("[StaffActions] edit card %s into summary: %v", card.Token, err)
	}
	runStarted = true
	startStaffActionRun(b, card, links, newUntil, staffChat.Id, msgID)
	return ext.EndGroups
}
