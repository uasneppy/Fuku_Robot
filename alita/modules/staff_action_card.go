package modules

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/db/staff"
	"github.com/divkix/Alita_Robot/alita/i18n"
	"github.com/divkix/Alita_Robot/alita/utils/cache"
	"github.com/divkix/Alita_Robot/alita/utils/chat_status"
)

// staffActionCardPrefix is the Redis key family of the confirm cards. It sits
// outside the alita:cache: prefix, so CLEAR_CACHE_ON_STARTUP keeps pending cards.
const staffActionCardPrefix = "alita:staff:act:"

// staffActionCardLifetime is how long an unconfirmed card stays usable (D-07). It
// is a variable so tests can shorten it.
var staffActionCardLifetime = 5 * time.Minute

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
	// ExpiresAt is a Unix time in milliseconds.
	ExpiresAt int64
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
			"expires_at", card.ExpiresAt,
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

// staffActionCardKeyboard builds the card's Confirm button. ok is false when the
// button data or its label cannot be produced, in which case the card must not be
// sent: an empty callback or label makes Telegram reject the whole message.
func staffActionCardKeyboard(tr *i18n.Translator, token string) (gotgbot.InlineKeyboardMarkup, bool) {
	var keyboard gotgbot.InlineKeyboardMarkup
	confirmData := encodeCallbackData(staffCallbackNamespace, map[string]string{"a": staffActRunConfirm, "t": token})
	confirmLabel, _ := tr.GetString("staff_btn_confirm")
	if confirmData == "" || confirmLabel == "" {
		return keyboard, false
	}
	keyboard.InlineKeyboard = [][]gotgbot.InlineKeyboardButton{{
		{Text: confirmLabel, CallbackData: confirmData},
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

// staffActionConfirm handles the Confirm button. Only the issuer's tap moves the
// card to running, and only once; every live check then runs before the first
// write, and each failure ends the card with the reason shown on it.
func (moduleStruct) staffActionConfirm(
	b *gotgbot.Bot,
	query *gotgbot.CallbackQuery,
	tr *i18n.Translator,
	fields map[string]string,
) error {
	card := loadStaffCardForTap(b, query, tr, fields)
	if card == nil {
		return ext.EndGroups
	}

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

	// Permanent for now; a later plan adds durations.
	var newUntil int64

	if err := editStaffActionMessage(b, staffChat.Id, msgID, renderStaffActionSummary(staffTr, card, pendingResults(links))); err != nil {
		log.Warnf("[StaffActions] edit card %s into summary: %v", card.Token, err)
	}
	startStaffActionRun(b, card, links, newUntil, staffChat.Id, msgID)
	return ext.EndGroups
}
