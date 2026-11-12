package modules

import (
	"context"
	"strconv"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/error_handling"
	"github.com/uasneppy/fuku_robot/fuku/utils/formatting"
	"github.com/uasneppy/fuku_robot/fuku/utils/tracing"
)

// The "I fixed it" button of a requirement mute notice (mreq|v1, a=f, h the hold ID, u the
// member). It frees only the member whose ID is u, and only from a mute the requirements
// placed, which the live marker proves (isMemberReqMute): an admin's, a staff, the captcha's,
// antiflood's or a warn-limit mute is never lifted by it, and admins keep /unmute. The steps
// run in the order of the UI contract, each refusing before the next, and nothing is changed
// before the hold is taken.

// memberReqFixedTimeout bounds the whole flow of one press, so the answer is sent while the
// query can still be answered.
const memberReqFixedTimeout = 10 * time.Second

// memberReqFixedLookupBudget is the part of the press's memberReqFixedTimeout the fresh photo and
// channel lookups of step 5 may use together, so at least half of the press remains for the
// permissions, the unmute and the edit. A lookup that uses it up is unknown and the press changes
// nothing. It is a variable so a test can shorten it.
var memberReqFixedLookupBudget = 5 * time.Second

// memberReqFixedResult is what a press ends with: the answer, whether it is an alert (else a
// toast), and, for a join hold that was freed, the member whose held welcome is still to send.
type memberReqFixedResult struct {
	Text    string
	Alert   bool
	Welcome *gotgbot.User
}

// memberReqParseFixed reads the fields of an "I fixed it" button: exactly a=f, u and h, both
// positive whole numbers of at most 19 digits. Anything else is not ok, and the callback is
// answered as an invalid request before any read.
func memberReqParseFixed(fields map[string]string) (holdID, userID int64, ok bool) {
	if len(fields) != 3 || fields["a"] != "f" {
		return 0, 0, false
	}
	holdID, ok = settingsCardNumber(fields["h"])
	if !ok {
		return 0, 0, false
	}
	userID, ok = settingsCardNumber(fields["u"])
	if !ok {
		return 0, 0, false
	}
	return holdID, userID, true
}

// memberReqFixedCallback answers the mreq| buttons. Every path answers the query exactly once,
// in the group's language: a panic anywhere below is recovered and still answers (empty).
func (moduleStruct) memberReqFixedCallback(b *gotgbot.Bot, ctx *ext.Context) error {
	defer error_handling.RecoverFromPanic("memberReqFixedCallback", "MemberReq")

	query, ok := callbackQueryFromContext(ctx)
	if !ok {
		return ext.EndGroups
	}
	tr := ctxTr(ctx)
	answered := false
	answer := func(text string, alert bool) {
		if answered {
			return
		}
		answered = true
		answerStaffCallback(b, query, text, alert)
	}
	// Deferred after the recover above, so it runs first: a panic still answers the query once.
	defer answer("", false)

	decoded, ok := decodeCallbackData(query.Data, memberReqCallbackNamespace)
	if !ok || query.Message == nil {
		answer(memberReqText(tr, "common_callback_invalid_request"), true)
		return ext.EndGroups
	}
	holdID, userID, ok := memberReqParseFixed(decoded.Fields)
	chat := query.Message.GetChat()
	if !ok || (chat.Type != "group" && chat.Type != "supergroup") {
		answer(memberReqText(tr, "common_callback_invalid_request"), true)
		return ext.EndGroups
	}
	// Only the member the notice is about may press it, checked before any read.
	if query.From.Id != userID {
		answer(memberReqText(tr, "mreq_fixed_not_yours"), true)
		return ext.EndGroups
	}

	runCtx, cancel := context.WithTimeout(tracing.UpdateContext(ctx), memberReqFixedTimeout)
	defer cancel()
	result := memberReqFixedRun(runCtx, b, query, tr, holdID, userID)
	answer(result.Text, result.Alert)
	if result.Welcome != nil {
		memberReqSendHeldWelcome(b, ctx, chat, *result.Welcome)
	}
	return ext.EndGroups
}

// memberReqSendHeldWelcome sends the welcome a join mute held back, once, through
// SendWelcomeMessage, so Greet once claims the member only now. Its error is only logged.
func memberReqSendHeldWelcome(b *gotgbot.Bot, ctx *ext.Context, chat gotgbot.Chat, member gotgbot.User) {
	welcomeCtx := &ext.Context{EffectiveChat: &chat, EffectiveMessage: ctx.EffectiveMessage}
	if err := SendWelcomeMessage(b, welcomeCtx, member.Id, member.FirstName); err != nil {
		log.Warnf("[MemberReq] the held welcome of user %d in chat %d was not sent: %s", member.Id, chat.Id, memberReqErrText(err))
	}
}

// memberReqFixedRun runs the checks and the unmute of one press, in the order of the UI
// contract (steps 3 to 10): the hold matched by its ID, the chat and the member alone; the
// live marker; the approvals (4b); the rules judged again (5); the permissions resolved before
// the hold is touched; the conditional delete of the hold; the unmute; the edit of the notice.
// It never reads the press's own notice ID: the message edited is the one the button sits on.
func memberReqFixedRun(ctx context.Context, b *gotgbot.Bot, query *gotgbot.CallbackQuery, tr *i18n.Translator, holdID, userID int64) memberReqFixedResult {
	chat := query.Message.GetChat()
	refuse := func(key string) memberReqFixedResult {
		return memberReqFixedResult{Text: memberReqText(tr, key), Alert: true}
	}

	hold, err := memberreq.GetHoldByIDFresh(ctx, holdID)
	if err != nil {
		log.Warnf("[MemberReq] the hold %d for user %d in chat %d could not be read: %s", holdID, userID, chat.Id, memberReqErrText(err))
		return refuse("mreq_fixed_check_failed")
	}
	if hold == nil || hold.ChatID != chat.Id || hold.UserID != userID {
		return refuse("mreq_fixed_stale")
	}

	live, err := memberReqLiveMember(ctx, b, chat.Id, userID)
	if err != nil {
		log.Warnf("[MemberReq] the live record of user %d in chat %d could not be read for I fixed it: %s", userID, chat.Id, memberReqErrText(err))
		return refuse("mreq_fixed_check_failed")
	}
	if !isMemberReqMute(live, hold.MuteUntil) {
		return refuse("mreq_fixed_not_ours")
	}

	// An approved member is judged clean for every rule, the name rules included, so nothing below
	// is read for them. The approvals come after the live marker check, so only a mute the
	// requirements placed can be freed this way, and an unknown answer changes nothing.
	approved, err := memberReqApprovedLookup(ctx, chat.Id, userID)
	if err != nil {
		log.Warnf("[MemberReq] the approvals of chat %d could not be read for the fix of user %d: %s", chat.Id, userID, memberReqErrText(err))
		return refuse("mreq_fixed_check_failed")
	}
	lookedUp := false
	if !approved {
		// Every enabled rule is judged again against the group's current settings: the name rules
		// on the live record and the lookup rules with fresh lookups.
		var refused *memberReqFixedResult
		if refused, lookedUp = memberReqFixedJudge(ctx, b, tr, chat.Id, userID, live); refused != nil {
			return *refused
		}
	}

	perms, lockedDown, err := memberReqUnmutePermissions(ctx, b, chat.Id)
	if err != nil {
		log.Warnf("[MemberReq] the unmute permissions of chat %d could not be resolved for user %d: %s", chat.Id, userID, memberReqErrText(err))
		return refuse("mreq_fixed_check_failed")
	}

	// WR-04: the marker was proved at step 4, and the fresh lookups of step 5 may have taken seconds
	// since. When they made a request, the live record is read once more, right before the take,
	// and the press frees the member only while the marker is still on them: an admin's /mute that
	// landed meanwhile replaced it and is never undone here. Worst case of the press, in paced
	// requests: the live record (1), the unmute permissions (1), this read (1), the lift (1) and
	// the notice edit (1), plus the lookups of step 5, which sit under memberReqFixedLookupBudget
	// (5 s), so everything before the take fits the 10 s of memberReqFixedTimeout with the budget to
	// spare, and a press that runs out of time here changes nothing.
	if lookedUp {
		again, err := memberReqLiveMember(ctx, b, chat.Id, userID)
		if err != nil {
			log.Warnf("[MemberReq] the live record of user %d in chat %d could not be read again before the fix: %s", userID, chat.Id, memberReqErrText(err))
			return refuse("mreq_fixed_check_failed")
		}
		if !isMemberReqMute(again, hold.MuteUntil) {
			return refuse("mreq_fixed_not_ours")
		}
	}

	taken, err := memberReqFixedTake(ctx, holdID, chat.Id, userID)
	if err != nil {
		log.Warnf("[MemberReq] taking the hold %d of user %d in chat %d failed: %s", holdID, userID, chat.Id, memberReqErrText(err))
		return refuse("mreq_fixed_check_failed")
	}
	if taken == nil {
		return refuse("mreq_fixed_stale")
	}

	if err := memberReqLiftMute(ctx, b, chat.Id, userID, perms); err != nil {
		log.Warnf("[MemberReq] the unmute of user %d in chat %d failed, so the hold is put back: %s", userID, chat.Id, memberReqErrText(err))
		memberReqFixedRestore(ctx, taken)
		return refuse("mreq_fixed_check_failed")
	}

	// During a lockdown the member got the stored pre-lockdown permissions and can chat once it is
	// lifted, which matters enough to be an alert and its own notice.
	noticeKey, result := "mreq_notice_fixed", memberReqFixedResult{Text: memberReqText(tr, "mreq_fixed_done")}
	if lockedDown {
		noticeKey, result = "mreq_notice_fixed_lockdown", memberReqFixedResult{Text: memberReqText(tr, "mreq_fixed_done_lockdown"), Alert: true}
	}
	memberReqFixedEdit(ctx, b, tr, chat.Id, chat.Type, query.Message.GetMessageId(), live.User, noticeKey)
	if taken.WelcomeHeld {
		member := live.User
		result.Welcome = &member
	}
	return result
}

// memberReqFixedJudge is step 5 of a press: it judges every enabled rule of the group against
// the member as they are now and returns the answer that ends the press, or nil when the member
// breaks nothing and can be freed. A rule now set to Off is not checked, and a member who fixed
// only some of several rules stays held. The name rules are judged on the live record; the photo
// and channel rules ask Telegram again with fresh lookups that bypass the positive caches, and the
// AI rule judges the live record's names from the verdict cache (one call only for names with no
// verdict, D-23), all under memberReqFixedLookupBudget of the press's own time. A definite broken rule is the answer even
// when another lookup could not be answered (it tells the member something true), and an
// unanswered lookup with nothing broken is mreq_fixed_check_failed and changes nothing, exactly
// like an unreadable setting. A channel the bot cannot judge in this group is inert, so it is
// neither broken nor unknown (the lookup layer classifies it). lookedUp reports whether a lookup
// went beyond the cache, that is made or waited for a Telegram request, so the caller knows the
// live record it proved at step 4 may be stale (WR-04).
func memberReqFixedJudge(ctx context.Context, b *gotgbot.Bot, tr *i18n.Translator, chatID, userID int64, live gotgbot.MergedChatMember) (refused *memberReqFixedResult, lookedUp bool) {
	settings, err := memberReqSettingsLookup(ctx, chatID)
	if err != nil {
		log.Warnf("[MemberReq] the requirements of chat %d could not be read for the fix of user %d: %s", chatID, userID, memberReqErrText(err))
		return &memberReqFixedResult{Text: memberReqText(tr, "mreq_fixed_check_failed"), Alert: true}, false
	}
	liveNames := memberReqNamesOf(live.User)
	broken := memberReqBroken(settings, liveNames)
	lookupCtx, cancel := context.WithTimeout(ctx, memberReqFixedLookupBudget)
	defer cancel()
	touch := &memberReqLookupTouch{}
	lookupCtx = context.WithValue(lookupCtx, memberReqLookupTouchKey{}, touch)
	// The AI row reads the live record's names and answers from the verdict cache whatever Fresh says
	// (D-23): a press is deterministic for unchanged names, and one call is made only for names the
	// cache has no verdict for. The photo and channel rows bypass their positive caches as before.
	// IgnoreBaseline: the press frees a mute the hold proves was placed, so a baseline-only pass (a
	// Redis that lost every AI key) must never stand in for a judgement (WR-01).
	mode := memberReqLookupMode{Fresh: true, Classify: true, Names: &liveNames, IgnoreBaseline: true}
	lookupBroken, unknown := memberReqLookupBroken(lookupCtx, b, settings, chatID, userID, mode)
	broken |= lookupBroken
	lookedUp = touch.touched.Load()
	if broken != 0 {
		return &memberReqFixedResult{Text: memberReqStillBrokenText(tr, broken), Alert: true}, lookedUp
	}
	if unknown {
		log.Warnf("[MemberReq] a lookup for the fix of user %d in chat %d could not be answered, so nothing changed", userID, chatID)
		return &memberReqFixedResult{Text: memberReqText(tr, "mreq_fixed_check_failed"), Alert: true}, lookedUp
	}
	return nil, lookedUp
}

// memberReqFixedTake deletes the hold by its ID, chat and member in one conditional statement,
// on a context that survives the press's deadline: of two taps exactly one gets the row, the
// other gets nil.
func memberReqFixedTake(ctx context.Context, holdID, chatID, userID int64) (*models.ChatRequirementHold, error) {
	recordCtx, cancel := memberReqRecordCtx(ctx)
	defer cancel()
	hold, taken, err := memberreq.TakeHoldByID(recordCtx, holdID, chatID, userID)
	if err != nil || !taken {
		return nil, err
	}
	return hold, nil
}

// memberReqFixedRestore puts a taken hold back with its original row ID, notice ID and every
// other column, so the button on the notice, which carries that ID, stays valid. A failure
// leaves the member muted with no hold and is logged at Error by IDs; the rejoin reset and an
// admin's /unmute still end the mute.
func memberReqFixedRestore(ctx context.Context, hold *models.ChatRequirementHold) {
	recordCtx, cancel := memberReqRecordCtx(ctx)
	defer cancel()
	if err := memberreq.RestoreHold(recordCtx, hold); err != nil {
		log.Errorf("[MemberReq] restoring the hold %d of user %d in chat %d failed: %s", hold.ID, hold.UserID, hold.ChatID, memberReqErrText(err))
	}
}

// memberReqFixedEdit replaces the mute notice with the fix text, with no keyboard: the key's
// template is translated with the name token and the member's mention is spliced in after.
// A failed or rate-limited edit is logged once by its code and changes no outcome and schedules
// nothing: the notice then keeps the 24 hour deletion time it was queued at. After a successful
// edit the confirmation follows the automatic-punishments timer counted from this moment (D-14,
// D-25), which moves the notice's 24 hour row up and never later.
func memberReqFixedEdit(ctx context.Context, b *gotgbot.Bot, tr *i18n.Translator, chatID int64, chatType string, messageID int64, member gotgbot.User, key string) {
	fallback := memberReqText(tr, "mreq_name_unknown", i18n.TranslationParams{"id": strconv.FormatInt(member.Id, 10)})
	text := memberReqText(tr, key, i18n.TranslationParams{"name": memberReqNamePlaceholder})
	text = lockdownSplice(text, memberReqNamePlaceholder, memberReqMention(member, fallback))
	err := memberReqPaced(ctx, func(callCtx context.Context) error {
		_, _, editErr := b.EditMessageTextWithContext(callCtx, &gotgbot.EditMessageTextOpts{
			ChatId:             chatID,
			MessageId:          messageID,
			Text:               text,
			ParseMode:          formatting.HTML,
			LinkPreviewOptions: &gotgbot.LinkPreviewOptions{IsDisabled: true},
		})
		return editErr
	})
	if err != nil {
		log.Warnf("[MemberReq] editing the notice %d in chat %d after I fixed it failed: %s", messageID, chatID, memberReqErrText(err))
		return
	}
	scheduleBotMessageFromNow(chatID, chatType, messageID, deletion.KindBotAuto)
}
