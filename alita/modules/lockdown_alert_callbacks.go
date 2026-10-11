package modules

import (
	"context"
	"html"
	"strconv"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/db/lockdown"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/i18n"
	"github.com/divkix/Alita_Robot/alita/utils/formatting"
)

// The lockdown| buttons act on another group than the one they sit in, so nothing the
// press carries is an authority. The handler loads the alert copy row named by r and its
// lockdown fresh, takes the locked chat from the lockdown row, and judges the presser by
// a live getChatMember of that chat. Confirm cards keep no state: the asker u and the
// expiry e travel in the callback data, so the buttons work with Redis down, and every
// fact that matters is checked live again at Confirm.

// lockdownAlertCallback dispatches the lockdown| buttons. Every path answers the query
// exactly once.
func (m moduleStruct) lockdownAlertCallback(b *gotgbot.Bot, ctx *ext.Context) error {
	query, ok := callbackQueryFromContext(ctx)
	if !ok {
		return ext.EndGroups
	}
	decoded, ok := decodeCallbackData(query.Data, lockdownCallbackNamespace)
	if !ok || query.Message == nil {
		return answerInvalidCallback(b, ctx, query)
	}
	fields := decoded.Fields
	fallback := ctxTr(ctx)

	// A Telegram service identity is refused before any read or lookup. A callback's
	// sender is the real pressing user, so this is defence in depth (D-05).
	if staffServiceUserIDs[query.From.Id] {
		answerStaffCallback(b, query, lockdownText(fallback, "lockdown_alert_post_as_yourself"), true)
		return ext.EndGroups
	}

	id, ok := lockdownParseCopyID(fields)
	if !ok {
		answerStaffCallback(b, query, lockdownText(fallback, "lockdown_alert_expired"), false)
		return ext.EndGroups
	}
	slot, err := lockdown.GetAlertSlotFresh(id)
	if err != nil {
		answerStaffCallback(b, query, lockdownText(fallback, "lockdown_alert_check_failed"), true)
		return ext.EndGroups
	}
	if slot == nil {
		answerStaffCallback(b, query, lockdownText(fallback, "lockdown_alert_expired"), false)
		return ext.EndGroups
	}
	// A copy row is honoured only on the chat it was posted in, so forged or replayed
	// data never reaches a lookup.
	if slot.DestChatID != query.Message.GetChat().Id {
		answerStaffCallback(b, query, lockdownText(fallback, "lockdown_alert_expired"), false)
		return ext.EndGroups
	}
	tr := lockdownSlotTranslator(slot)
	ld, err := lockdown.GetFresh(slot.LockdownID)
	if err != nil {
		answerStaffCallback(b, query, lockdownText(tr, "lockdown_alert_check_failed"), true)
		return ext.EndGroups
	}
	if ld == nil {
		answerStaffCallback(b, query, lockdownText(tr, "lockdown_alert_expired"), false)
		return ext.EndGroups
	}

	// A button of a lockdown that is lifting or lifted does nothing but say so. This runs
	// before any action and before any lookup of the chat's current lockdown: the copy row
	// names its own lockdown, so an old alert can never reach a newer one.
	if ld.State != models.LockdownStateActive {
		return m.lockdownStalePress(b, query, tr, slot, ld)
	}

	switch fields["a"] {
	case lockdownActLiftAsk:
		return m.lockdownLiftAsk(b, query, tr, slot, ld)
	case lockdownActLiftConfirm:
		return m.lockdownLiftConfirm(b, query, tr, ld, fields)
	case lockdownActCancel:
		return m.lockdownCardCancel(b, query, tr, fields)
	}
	answerStaffCallback(b, query, lockdownText(tr, "lockdown_alert_expired"), false)
	return ext.EndGroups
}

// lockdownParseCopyID reads the alert copy row ID r: an integer from 1 to the largest
// 63-bit value. Anything else is not ok, and the dispatcher answers it as no longer valid
// before any read.
func lockdownParseCopyID(fields map[string]string) (uint, bool) {
	id, err := strconv.ParseUint(fields["r"], 10, 63)
	if err != nil || id == 0 {
		return 0, false
	}
	return uint(id), true
}

// lockdownParseAsker reads the asker u: a positive 63-bit integer.
func lockdownParseAsker(fields map[string]string) (int64, bool) {
	asker, err := strconv.ParseInt(fields["u"], 10, 64)
	if err != nil || asker <= 0 {
		return 0, false
	}
	return asker, true
}

// lockdownParseCardFields reads a Confirm card's asker u and expiry e (unix minutes).
// Both must be positive 63-bit integers; anything else is not ok.
func lockdownParseCardFields(fields map[string]string) (asker int64, expiryMinute int64, ok bool) {
	asker, ok = lockdownParseAsker(fields)
	if !ok {
		return 0, 0, false
	}
	expiryMinute, err := strconv.ParseInt(fields["e"], 10, 64)
	if err != nil || expiryMinute <= 0 {
		return 0, 0, false
	}
	return asker, expiryMinute, true
}

// lockdownStalePress answers a press on any button of a lockdown that is lifting or
// lifted: "Already lifted by X at T", and nothing else. When the pressed message is the
// alert copy itself it is closed at once (edited to its lifted state, buttons gone);
// otherwise it is a confirm card, which is edited to the same text. No card is posted,
// nothing is lifted and the chat's current lockdown is never looked at.
func (m moduleStruct) lockdownStalePress(
	b *gotgbot.Bot, query *gotgbot.CallbackQuery, tr *i18n.Translator,
	slot *models.ChatLockdownAlert, ld *models.ChatLockdown,
) error {
	answerStaffCallback(b, query, lockdownAlreadyLiftedText(tr, ld, false), false)
	if query.Message.GetChat().Id == slot.DestChatID && query.Message.GetMessageId() == slot.MessageID {
		lockdownCloseSlotNow(context.Background(), b, slot, ld)
		return ext.EndGroups
	}
	editStaffCallbackMessage(b, query, lockdownAlreadyLiftedText(tr, ld, true))
	return ext.EndGroups
}

// lockdownButtonAuthority judges who pressed a button of a lockdown alert: a live
// getChatMember of the locked chat (taken from the lockdown row, never from the chat the
// button sits in), creator or administrator with can_restrict_members. The admin cache
// never authorizes, and a failed lookup refuses. When it refuses, refusalKey is the
// locale key of the reason.
func lockdownButtonAuthority(b *gotgbot.Bot, chatID, userID int64) (ok bool, refusalKey string) {
	member, err := lockdownLiveMember(b, chatID, userID)
	if err != nil {
		log.Warnf("[Lockdown] live check of user %d in chat %d for an alert button failed: %v", userID, chatID, err)
		return false, "lockdown_alert_check_failed"
	}
	switch staffIssuerSkipReason(member) {
	case "":
		return true, ""
	case staffReasonSkipIssuerNoRight:
		return false, "lockdown_alert_no_restrict"
	}
	return false, "lockdown_alert_not_admin"
}

// lockdownAlreadyLiftedText says who lifted the lockdown and when. asHTML escapes the
// name and mentions the lifter, for a card; otherwise it is plain text, for a toast.
func lockdownAlreadyLiftedText(tr *i18n.Translator, ld *models.ChatLockdown, asHTML bool) string {
	when := ld.UpdatedAt
	if ld.LiftStartedAt != nil {
		when = *ld.LiftStartedAt
	}
	text := lockdownText(tr, "lockdown_alert_already_lifted", i18n.TranslationParams{
		"name": lockdownNameToken,
		"time": lockdownTime(when),
	})
	name := ld.LiftedByName
	switch {
	case !asHTML:
	case ld.LiftedBy != nil:
		name = lockdownMention(*ld.LiftedBy, name)
	default:
		name = html.EscapeString(name)
	}
	return lockdownSplice(text, lockdownNameToken, name)
}

// lockdownGroupText translates key with the locked group's label spliced in after the
// translation.
func lockdownGroupText(tr *i18n.Translator, key string, ld *models.ChatLockdown, extra ...string) string {
	text := lockdownText(tr, key, i18n.TranslationParams{"group": lockdownGroupToken, "name": lockdownNameToken})
	pairs := append([]string{lockdownGroupToken, lockdownGroupLabel(ld)}, extra...)
	return lockdownSplice(text, pairs...)
}

// lockdownLiftAsk answers the alert's Lift lockdown button. A lockdown that is no longer
// active is answered as lifted. Otherwise the presser must be a live creator or
// administrator with can_restrict_members of the locked group, and gets a confirm card,
// a reply to the alert that only they can use.
func (m moduleStruct) lockdownLiftAsk(
	b *gotgbot.Bot, query *gotgbot.CallbackQuery, tr *i18n.Translator,
	slot *models.ChatLockdownAlert, ld *models.ChatLockdown,
) error {
	if ld.State != models.LockdownStateActive {
		answerStaffCallback(b, query, lockdownAlreadyLiftedText(tr, ld, false), false)
		return ext.EndGroups
	}
	if ok, key := lockdownButtonAuthority(b, ld.ChatID, query.From.Id); !ok {
		answerStaffCallback(b, query, lockdownText(tr, key), true)
		return ext.EndGroups
	}

	asker := strconv.FormatInt(query.From.Id, 10)
	copyID := strconv.FormatUint(uint64(slot.ID), 10)
	expiry := strconv.FormatInt(time.Now().Add(lockdownCardLifetime).Unix()/60, 10)
	confirmData := encodeCallbackData(lockdownCallbackNamespace, map[string]string{
		"a": lockdownActLiftConfirm, "r": copyID, "u": asker, "e": expiry,
	})
	cancelData := encodeCallbackData(lockdownCallbackNamespace, map[string]string{
		"a": lockdownActCancel, "r": copyID, "u": asker,
	})
	if confirmData == "" || cancelData == "" {
		answerStaffCallback(b, query, lockdownText(tr, "lockdown_alert_check_failed"), true)
		return ext.EndGroups
	}

	answerStaffCallback(b, query, "", false)
	if _, err := b.SendMessage(query.Message.GetChat().Id, lockdownGroupText(tr, "lockdown_alert_lift_ask", ld), &gotgbot.SendMessageOpts{
		ParseMode:          formatting.HTML,
		LinkPreviewOptions: &gotgbot.LinkPreviewOptions{IsDisabled: true},
		ReplyParameters: &gotgbot.ReplyParameters{
			MessageId:                query.Message.GetMessageId(),
			AllowSendingWithoutReply: true,
		},
		ReplyMarkup: gotgbot.InlineKeyboardMarkup{InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{
			{Text: lockdownText(tr, "lockdown_alert_btn_confirm"), CallbackData: confirmData},
			{Text: lockdownText(tr, "lockdown_alert_btn_cancel"), CallbackData: cancelData},
		}}},
	}); err != nil {
		log.Warnf("[Lockdown] lift confirm card for lockdown %d could not be posted: %v", ld.ID, err)
	}
	return ext.EndGroups
}

// lockdownLiftConfirm runs the lift for the person who asked. The card's u field must be
// the presser; the lockdown and the presser's rights are read again live, and the lift
// itself is liftLockdownRow, the same path as /unlockdown, so the stored permissions go
// back before the lift is recorded. Every outcome is told on the card, which is edited to
// its final text and never deleted.
func (m moduleStruct) lockdownLiftConfirm(
	b *gotgbot.Bot, query *gotgbot.CallbackQuery, tr *i18n.Translator,
	ld *models.ChatLockdown, fields map[string]string,
) error {
	asker, expiryMinute, ok := lockdownParseCardFields(fields)
	if !ok {
		answerStaffCallback(b, query, lockdownText(tr, "lockdown_alert_expired"), false)
		return ext.EndGroups
	}
	if asker != query.From.Id {
		answerStaffCallback(b, query, lockdownText(tr, "lockdown_alert_not_yours"), true)
		return ext.EndGroups
	}
	// Expiry is lazy: the minute travels in the data, no timer and no Redis key.
	if expiryMinute < time.Now().Unix()/60 {
		answerStaffCallback(b, query, "", false)
		editStaffCallbackMessage(b, query, lockdownText(tr, "lockdown_alert_card_expired"))
		return ext.EndGroups
	}
	answerStaffCallback(b, query, lockdownText(tr, "lockdown_alert_working"), false)

	if ld.State != models.LockdownStateActive {
		editStaffCallbackMessage(b, query, lockdownAlreadyLiftedText(tr, ld, true))
		return ext.EndGroups
	}
	if ok, key := lockdownButtonAuthority(b, ld.ChatID, query.From.Id); !ok {
		editStaffCallbackMessage(b, query, lockdownText(tr, key))
		return ext.EndGroups
	}

	name := lockdownCapRunes(staffFullName(&query.From), lockdownNameMaxRunes)
	res := liftLockdownRow(context.Background(), b, ld, query.From.Id, name)
	switch res.Outcome {
	case lockdownLiftRestoreFailed:
		editStaffCallbackMessage(b, query, lockdownGroupText(tr, "lockdown_alert_restore_failed", ld))
		return ext.EndGroups
	case lockdownLiftRecordFailed:
		editStaffCallbackMessage(b, query, lockdownGroupText(tr, "lockdown_alert_record_failed", ld))
		return ext.EndGroups
	case lockdownLiftLost:
		// Another admin recorded the lift first; who and when are read fresh.
		current, err := lockdown.GetFresh(ld.ID)
		if err != nil || current == nil {
			log.Warnf("[Lockdown] lockdown %d could not be read after a lost lift: %v", ld.ID, err)
			current = ld
		}
		editStaffCallbackMessage(b, query, lockdownAlreadyLiftedText(tr, current, true))
		return ext.EndGroups
	}

	editStaffCallbackMessage(b, query, lockdownGroupText(tr, "lockdown_alert_lift_done", ld,
		lockdownNameToken, lockdownMention(query.From.Id, name)))
	lockdownPostLiftNotice(b, ld, lockdownLiftedText(staffChatTranslator(ld.ChatID), query.From.Id, name, res))
	wakeLockdownWorker()
	return ext.EndGroups
}

// lockdownPostLiftNotice posts the public lift notice into the locked group, under the
// lockdown pacer like every lockdown post. A failure is logged and changes nothing: the
// lift is already recorded.
func lockdownPostLiftNotice(b *gotgbot.Bot, ld *models.ChatLockdown, text string) {
	if text == "" {
		return
	}
	err := lockdownPaced(context.Background(), func(callCtx context.Context) error {
		call, cancel := context.WithTimeout(callCtx, lockdownCallTimeout)
		defer cancel()
		_, sendErr := b.SendMessageWithContext(call, ld.ChatID, text, &gotgbot.SendMessageOpts{
			ParseMode:          formatting.HTML,
			LinkPreviewOptions: &gotgbot.LinkPreviewOptions{IsDisabled: true},
		})
		return sendErr
	})
	if err != nil {
		log.Warnf("[Lockdown] lift notice for chat %d could not be posted: %v", ld.ChatID, err)
	}
}

// lockdownCardCancel closes a confirm card without changing anything. Only the person
// who asked may cancel it.
func (m moduleStruct) lockdownCardCancel(
	b *gotgbot.Bot, query *gotgbot.CallbackQuery, tr *i18n.Translator, fields map[string]string,
) error {
	asker, ok := lockdownParseAsker(fields)
	if !ok {
		answerStaffCallback(b, query, lockdownText(tr, "lockdown_alert_expired"), false)
		return ext.EndGroups
	}
	if asker != query.From.Id {
		answerStaffCallback(b, query, lockdownText(tr, "lockdown_alert_not_yours"), true)
		return ext.EndGroups
	}
	answerStaffCallback(b, query, "", false)
	editStaffCallbackMessage(b, query, lockdownText(tr, "lockdown_alert_cancelled"))
	return ext.EndGroups
}
