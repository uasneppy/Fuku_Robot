package modules

import (
	"context"
	"errors"
	"fmt"
	"html"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/db/staff"
	"github.com/divkix/Alita_Robot/alita/i18n"
	"github.com/divkix/Alita_Robot/alita/utils/chat_status"
	"github.com/divkix/Alita_Robot/alita/utils/formatting"
)

// staffKindUndo is the Kind of an undo confirm card. It is never an action kind: it
// exists so the staff action Confirm and the undo Confirm can tell the two cards
// apart. The action the card undoes is the card's UndoKind.
const staffKindUndo staffActionKind = "undo"

// staffClaimUndo claims the one undo of a record and returns the claim time the run
// later hands to staff.ReleaseUndo. It is a test seam, like staffCreateActionRecord:
// tests replace it to make another claimer win between Confirm's last read and its
// claim, which no sequence of taps can do on purpose. Production never reassigns it.
var staffClaimUndo = func(actionID uint, by int64, byName string) (time.Time, bool, error) {
	return staff.ClaimUndo(actionID, by, byName)
}

// staffUndoNameRunes caps the name of the member who undid an action wherever it is
// spliced into a message.
const staffUndoNameRunes = 64

// staffPlainName collapses whitespace and cuts a stored name to staffUndoNameRunes,
// without escaping, for the places that take plain text (a callback answer).
func staffPlainName(name string) string {
	name = strings.Join(strings.Fields(name), " ")
	if utf8.RuneCountInString(name) > staffUndoNameRunes {
		name = string([]rune(name)[:staffUndoNameRunes]) + "…"
	}
	return name
}

// staffUndoClaimedText answers a press on a record whose undo was claimed, by what
// the record says about that undo (staffUndoStateOf): it is running, it was
// interrupted, it changed nothing, or it undid something ("Already undone by
// <name>."). undoDone is the number of groups whose undo_outcome is done. The name is
// spliced in after translation. For an HTML message the name is escaped; for a
// callback answer, which is plain text, it is not.
func staffUndoClaimedText(tr *i18n.Translator, a *models.StaffAction, undoDone int, now time.Time, forHTML bool) string {
	name := staffPlainName(a.UndoByName)
	if forHTML {
		name = html.EscapeString(name)
	}
	key := "staff_undo_already"
	switch staffUndoStateOf(a, undoDone, now) {
	case staffUndoRunning:
		key = "staff_undo_already_running"
	case staffUndoInterrupted:
		key = "staff_undo_already_interrupted"
	case staffUndoNothing:
		key = "staff_undo_already_nothing"
	}
	text, _ := tr.GetString(key, i18n.TranslationParams{"name": staffUserToken})
	return strings.Replace(text, staffUserToken, name, 1)
}

// staffActionUndoable reports whether a record can be undone: it finished, nobody
// claimed an undo yet, it is a ban, mute, unban or unmute, and at least one group
// has the action applied. A kick has nothing to put back.
func staffActionUndoable(a *models.StaffAction, groups []models.StaffActionGroup) bool {
	if a == nil || a.FinishedAt == nil || a.UndoStartedAt != nil {
		return false
	}
	switch staffActionKind(a.Action) {
	case staffKindBan, staffKindMute, staffKindUnban, staffKindUnmute:
	default:
		return false
	}
	for _, row := range groups {
		if staffOutcomeFromName(row.Outcome) == staffOutcomeDone {
			return true
		}
	}
	return false
}

// staffUndoState is what the record says about a staff action's undo.
type staffUndoState int

const (
	// staffUndoNone: nobody claimed an undo.
	staffUndoNone staffUndoState = iota
	// staffUndoRunning: claimed, not finished, and the run wrote recently.
	staffUndoRunning
	// staffUndoInterrupted: claimed, not finished, and silent for longer than the
	// target lock lives, so its run is dead.
	staffUndoInterrupted
	// staffUndoUndone: finished and at least one group was undone.
	staffUndoUndone
	// staffUndoNothing: finished and no group was undone.
	staffUndoNothing
)

// staffUndoStateOf is the one place an undo's state is decided (owner decision b on
// D-09): the claim alone never says "undone". undoDone is the number of groups whose
// undo_outcome is done. An unfinished claim is running while the record's heartbeat
// (updated_at, bumped by the claim and by every group result) is younger than
// staffTargetLockTTL; a run renews its target lock every 10 minutes, so one silent
// that long is dead, the same rule as the action's own state.
func staffUndoStateOf(a *models.StaffAction, undoDone int, now time.Time) staffUndoState {
	switch {
	case a == nil || a.UndoStartedAt == nil:
		return staffUndoNone
	case a.UndoFinishedAt == nil && now.Sub(a.UpdatedAt) < staffTargetLockTTL:
		return staffUndoRunning
	case a.UndoFinishedAt == nil:
		return staffUndoInterrupted
	case undoDone > 0:
		return staffUndoUndone
	default:
		return staffUndoNothing
	}
}

// staffUndoDoneCount counts the groups whose undo changed something.
func staffUndoDoneCount(groups []models.StaffActionGroup) int {
	n := 0
	for _, row := range groups {
		if row.UndoOutcome == models.StaffActionOutcomeDone {
			n++
		}
	}
	return n
}

// staffUndoHeader is the first line of an undo card and of its summary: an undo
// arrow and "Undo" in front of the header of the action being undone. It is built
// from a copy of the card whose Kind is the undone action, so the icon, target,
// duration and reason read as they did on the action's own summary.
func staffUndoHeader(tr *i18n.Translator, card *staffActionCard) string {
	inner := *card
	inner.Kind = card.UndoKind
	if inner.Kind == staffKindUndo {
		inner.Kind = ""
	}
	label, _ := tr.GetString("staff_undo_label")
	return "↩ " + label + " · " + staffActionHeader(tr, &inner)
}

// staffUndoKeyboard is the one button of a finished summary: it carries only the
// record ID. ok is false, and the reason logged, when the data does not encode or
// the label is empty; the summary then goes out without a button.
func staffUndoKeyboard(tr *i18n.Translator, actionID uint) (gotgbot.InlineKeyboardMarkup, bool) {
	var keyboard gotgbot.InlineKeyboardMarkup
	data := encodeCallbackData(staffCallbackNamespace, map[string]string{
		"a": staffActUndoAsk,
		"r": strconv.FormatUint(uint64(actionID), 10),
	})
	label, _ := tr.GetString("staff_undo_button")
	if data == "" || label == "" {
		log.Errorf("[StaffActions] the Undo button of action %d cannot be built (data %q, label %q)", actionID, data, label)
		return keyboard, false
	}
	keyboard.InlineKeyboard = [][]gotgbot.InlineKeyboardButton{{{Text: label, CallbackData: data}}}
	return keyboard, true
}

// staffUndoCardKeyboard builds the undo card's Confirm and Cancel buttons on one
// row, like staffActionCardKeyboard but with the undo callback codes. ok is false
// when any button data or label cannot be produced, in which case the card must not
// be sent.
func staffUndoCardKeyboard(tr *i18n.Translator, token string) (gotgbot.InlineKeyboardMarkup, bool) {
	var keyboard gotgbot.InlineKeyboardMarkup
	confirmData := encodeCallbackData(staffCallbackNamespace, map[string]string{"a": staffActUndoConfirm, "t": token})
	cancelData := encodeCallbackData(staffCallbackNamespace, map[string]string{"a": staffActUndoCancel, "t": token})
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

// staffUndoAsk handles the Undo button on a finished summary. Any live member of
// the Staff Group may press it (D-06). It posts a new confirm card, as a reply to
// the summary, and does nothing else: nothing is undone until Confirm (D-07). The
// press is answered exactly once on every path.
func (m moduleStruct) staffUndoAsk(
	b *gotgbot.Bot,
	query *gotgbot.CallbackQuery,
	tr *i18n.Translator,
	fields map[string]string,
) error {
	answer := func(key string, alert bool) {
		text, _ := tr.GetString(key)
		answerStaffCallback(b, query, text, alert)
	}

	// A Telegram service identity is refused before any lookup. A callback's sender is
	// the real pressing user, so this is defence in depth (research A4).
	if staffServiceUserIDs[query.From.Id] {
		answer("staff_post_as_yourself", true)
		return ext.EndGroups
	}

	id, err := strconv.ParseUint(fields["r"], 10, 63)
	if err != nil || id == 0 {
		answer("staff_cb_expired", false)
		return ext.EndGroups
	}
	chat := query.Message.GetChat()

	group, err := staff.GetStaffGroupFresh(chat.Id)
	if err != nil {
		answer("staff_check_failed", true)
		return ext.EndGroups
	}
	if group == nil {
		answer("staff_cb_expired", false)
		return ext.EndGroups
	}

	member, err := chat_status.IsUserInChatWithError(b, &chat, query.From.Id)
	if err != nil {
		log.Warnf("[StaffActions] undo press: membership check of user %d in chat %d failed: %v", query.From.Id, chat.Id, err)
		answer("staff_check_failed", true)
		return ext.EndGroups
	}
	if !member {
		answer("staff_cb_members_only", true)
		return ext.EndGroups
	}

	action, err := staff.GetActionFresh(uint(id))
	if err != nil {
		answer("staff_check_failed", true)
		return ext.EndGroups
	}
	if action == nil {
		answer("staff_cb_expired", false)
		return ext.EndGroups
	}
	// A record is only ever undone from the Staff Group that owns it, so a forged or
	// replayed button naming another Staff Group's record does nothing.
	if action.StaffChatID != chat.Id {
		answer("staff_cb_denied", true)
		return ext.EndGroups
	}
	groups, err := staff.ListActionGroupsFresh(action.ID)
	if err != nil {
		answer("staff_check_failed", true)
		return ext.EndGroups
	}
	if action.UndoStartedAt != nil {
		answerStaffCallback(b, query, staffUndoClaimedText(tr, action, staffUndoDoneCount(groups), time.Now(), false), true)
		return ext.EndGroups
	}
	if !staffActionUndoable(action, groups) {
		answer("staff_undo_not_available", true)
		return ext.EndGroups
	}

	applied := 0
	for _, row := range groups {
		if staffOutcomeFromName(row.Outcome) == staffOutcomeDone {
			applied++
		}
	}
	card := &staffActionCard{
		Issuer:         query.From.Id,
		StaffChat:      chat.Id,
		Kind:           staffKindUndo,
		UndoKind:       staffActionKind(action.Action),
		UndoOf:         action.ID,
		Target:         action.TargetUserID,
		TargetName:     action.TargetName,
		Reason:         action.Reason,
		GroupCount:     applied,
		DurationSec:    action.DurationSec,
		DurationAmount: action.DurationAmount,
		DurationUnit:   action.DurationUnit,
		OverLimit:      action.OverLimit,
	}
	if err := saveStaffActionCard(card); err != nil {
		log.Warnf("[StaffActions] save undo card for action %d: %v", action.ID, err)
		if errors.Is(err, errStaffCardNoRedis) {
			answer("staff_act_redis_unavailable", true)
		} else {
			answer("staff_check_failed", true)
		}
		return ext.EndGroups
	}
	keyboard, ok := staffUndoCardKeyboard(tr, card.Token)
	if !ok {
		deleteStaffActionCard(card.Token)
		answer("staff_check_failed", true)
		return ext.EndGroups
	}

	applies, _ := tr.GetString("staff_undo_card_applies", i18n.TranslationParams{"count": applied})
	opts := &gotgbot.SendMessageOpts{
		ParseMode:          formatting.HTML,
		LinkPreviewOptions: &gotgbot.LinkPreviewOptions{IsDisabled: true},
		ReplyMarkup:        keyboard,
	}
	// Message IDs belong to their chat: the reply goes to the stored summary only
	// when it was posted in the chat this press came from. After a Staff Group
	// migration it was not, and the card is posted without a reply.
	if action.SummaryChatID == chat.Id && action.SummaryMsgID != 0 {
		opts.ReplyParameters = &gotgbot.ReplyParameters{MessageId: action.SummaryMsgID, AllowSendingWithoutReply: true}
	}
	sent, err := b.SendMessage(chat.Id, staffUndoHeader(tr, card)+"\n\n"+applies, opts)
	if err != nil {
		log.Warnf("[StaffActions] post undo card for action %d in chat %d: %v", action.ID, chat.Id, err)
		deleteStaffActionCard(card.Token)
		answer("staff_check_failed", true)
		return ext.EndGroups
	}
	answerStaffCallback(b, query, "", false)
	scheduleStaffActionExpiry(b, card.Token, chat.Id, sent.MessageId)
	return ext.EndGroups
}

// staffUndoConfirm handles the undo card's Confirm button. Only the member who
// pressed Undo can confirm (the card's issuer is the presser, checked inside the
// Redis compare-and-set), and only once. It re-checks live that the Staff Group
// still exists and the presser is still in it, claims the undo with one conditional
// update of the record, and only the winner of that claim runs (D-09).
func (m moduleStruct) staffUndoConfirm(
	b *gotgbot.Bot,
	query *gotgbot.CallbackQuery,
	tr *i18n.Translator,
	fields map[string]string,
) error {
	// A Telegram service identity is refused before any lookup. A callback's sender is
	// the real pressing user, so this is defence in depth (research A4).
	if staffServiceUserIDs[query.From.Id] {
		text, _ := tr.GetString("staff_post_as_yourself")
		answerStaffCallback(b, query, text, true)
		return ext.EndGroups
	}
	card := loadStaffCardForTap(b, query, tr, fields)
	if card == nil {
		return ext.EndGroups
	}
	// A staff action card has its own Confirm; run through this one it would skip
	// the checks that card was built for.
	if card.Kind != staffKindUndo {
		text, _ := tr.GetString("staff_cb_expired")
		answerStaffCallback(b, query, text, false)
		return ext.EndGroups
	}

	// The same target lock as a staff action: an undo and an action on one person
	// never run at once. Only a live pending card from its own presser asks for it.
	lockHeld, runStarted := false, false
	if card.State == staffCardPending && time.Now().UnixMilli() < card.ExpiresAt {
		if card.Issuer != query.From.Id {
			text, _ := tr.GetString("staff_undo_card_presser_only")
			answerStaffCallback(b, query, text, true)
			return ext.EndGroups
		}
		if !takeStaffTargetLock(b, query, tr, card) {
			return ext.EndGroups
		}
		lockHeld = true
	}
	defer func() {
		if lockHeld && !runStarted {
			releaseStaffTargetLock(card.Target, card.Token)
		}
	}()

	claim, state, err := transitionStaffActionCard(card.Token, query.From.Id, staffCardRunning, true)
	if err != nil {
		log.Warnf("[StaffActions] confirm undo card %s: %v", card.Token, err)
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
	abort := func(key string) {
		text, _ := staffTr.GetString(key)
		abortStaffActionCard(b, staffTr, card, staffChat.Id, msgID, text)
	}

	staffGroup, err := staff.GetStaffGroupFresh(staffChat.Id)
	if err != nil {
		abort("staff_act_abort_check_failed")
		return ext.EndGroups
	}
	if staffGroup == nil {
		abort("staff_act_abort_not_staff")
		return ext.EndGroups
	}
	inStaff, err := chat_status.IsUserInChatWithError(b, &staffChat, card.Issuer)
	if err != nil {
		log.Warnf("[StaffActions] undo presser membership check in chat %d: %v", staffChat.Id, err)
		abort("staff_act_abort_check_failed")
		return ext.EndGroups
	}
	if !inStaff {
		abort("staff_act_abort_issuer_left")
		return ext.EndGroups
	}

	action, err := staff.GetActionFresh(card.UndoOf)
	if err != nil || action == nil || action.StaffChatID != staffChat.Id {
		abort("staff_act_abort_check_failed")
		return ext.EndGroups
	}
	groups, err := staff.ListActionGroupsFresh(action.ID)
	if err != nil {
		abort("staff_act_abort_check_failed")
		return ext.EndGroups
	}
	abortAlready := func(a *models.StaffAction, claimedGroups []models.StaffActionGroup) {
		text := staffUndoClaimedText(staffTr, a, staffUndoDoneCount(claimedGroups), time.Now(), true)
		abortStaffActionCard(b, staffTr, card, staffChat.Id, msgID, text)
	}
	if action.UndoStartedAt != nil {
		abortAlready(action, groups)
		return ext.EndGroups
	}
	if !staffActionUndoable(action, groups) {
		abort("staff_undo_not_available")
		return ext.EndGroups
	}
	links, err := staff.ListLinksByStaffFresh(staffChat.Id)
	if err != nil {
		abort("staff_act_abort_check_failed")
		return ext.EndGroups
	}

	// The run joins the shutdown drain before it claims, so a StopStaffActions that
	// starts between the claim and the run's start waits for it instead of returning
	// over a claim nobody will finish. The registration is given back here on every
	// path that does not start the run; the started run's coordinator owns the Done.
	runCtx := joinStaffRuns()
	defer func() {
		if !runStarted {
			staffActionRunsWG.Done()
		}
	}()
	// Once the shutdown cancelled the run context nothing may be claimed: the run
	// would only cut every group off, and the action keeps its one undo (D-09).
	if runCtx.Err() != nil {
		abort("staff_undo_abort_restarting")
		return ext.EndGroups
	}

	// The claim is the last check before the run: one conditional update, so of two
	// cards for one record, on any replica, only one ever starts.
	presserName := staffFullName(&query.From)
	// The log posts name the presser, not whoever created the card.
	card.IssuerName = presserName
	claimedAt, claimed, err := staffClaimUndo(action.ID, card.Issuer, presserName)
	if err != nil {
		abort("staff_act_abort_check_failed")
		return ext.EndGroups
	}
	if !claimed {
		winner, readErr := staff.GetActionFresh(action.ID)
		if readErr != nil || winner == nil || winner.UndoStartedAt == nil {
			abort("staff_act_abort_check_failed")
			return ext.EndGroups
		}
		// The winner's groups are read again so the text reflects its current state.
		winnerGroups, groupsErr := staff.ListActionGroupsFresh(winner.ID)
		if groupsErr != nil {
			abort("staff_act_abort_check_failed")
			return ext.EndGroups
		}
		abortAlready(winner, winnerGroups)
		return ext.EndGroups
	}

	targets, notApplied := staffUndoTargets(groups, links)
	pendingLinks := make([]models.StaffGroupLink, len(targets))
	for i, t := range targets {
		pendingLinks[i] = t.Link
	}
	pending, _ := composeStaffSummary(staffTr, staffUndoSummaryHeader(staffTr, card, notApplied, false), pendingResults(pendingLinks), false)
	if err := editStaffActionMessage(b, staffChat.Id, msgID, pending); err != nil {
		log.Warnf("[StaffActions] edit undo card %s into summary: %v", card.Token, err)
	}
	runStarted = true
	startStaffUndoRun(b, runCtx, card, action, groups, targets, notApplied, claimedAt, staffChat.Id, msgID)
	return ext.EndGroups
}

// editStaffUndoneOriginal adds "Undone by <name>, see the reply" (markerKey
// staff_undo_marker), or "Undo by <name> changed nothing, see the reply" (markerKey
// staff_undo_marker_nothing), to the original summary and removes its Undo button
// (D-08). It runs at the end of the undo, after the undo's own summary was
// delivered. It acts only when the record's summary chat is the Staff Group the undo
// came from: a message ID belongs to the chat it was sent to, and after a Staff
// Group migration the stored one points nowhere in the new chat. The edit has a
// delivery budget of its own, never the run's context, which a shutdown may already
// have cancelled. A failed edit is logged and nothing else changes.
func editStaffUndoneOriginal(
	b *gotgbot.Bot,
	tr *i18n.Translator,
	a *models.StaffAction,
	groups []models.StaffActionGroup,
	staffChatID int64,
	presserName string,
	markerKey string,
) {
	if a.SummaryChatID != staffChatID || a.SummaryMsgID == 0 {
		return
	}
	marker, _ := tr.GetString(markerKey, i18n.TranslationParams{"name": staffUserToken})
	marker = strings.Replace(marker, staffUserToken, html.EscapeString(staffPlainName(presserName)), 1)
	header := staffActionHeader(tr, staffCardFromRecord(a)) + "\n" + marker
	text, _ := composeStaffSummary(tr, header, staffResultsFromRecord(groups), true)
	ctx, cancel := newStaffDeliverContext()
	defer cancel()
	// An empty markup removes the Undo button.
	if !editStaffActionFinal(ctx, b, staffChatID, a.SummaryMsgID, text, gotgbot.InlineKeyboardMarkup{}) {
		log.Warnf("[StaffActions] could not mark the original summary of action %d as undone", a.ID)
	}
}

// staffUndoTarget is one group an undo visits: the original's row for it and the
// link it is reached through. Linked is false when the group is no longer linked to
// the Staff Group, in which case Link holds only what the record knew about it.
type staffUndoTarget struct {
	Row    models.StaffActionGroup
	Link   models.StaffGroupLink
	Linked bool
}

// staffUndoTargets lists the groups an undo visits: the rows where the original was
// applied, in the original's seq order. The second value is how many rows were not
// applied, which the summary counts in one line instead of listing (D-03). A group
// that is still linked is reached through its current link, titled as the record
// titled it; one that is not has a link value holding only its ID and stored title.
func staffUndoTargets(groups []models.StaffActionGroup, links []models.StaffGroupLink) ([]staffUndoTarget, int) {
	byGroup := make(map[int64]models.StaffGroupLink, len(links))
	for _, link := range links {
		byGroup[link.GroupChatID] = link
	}
	var targets []staffUndoTarget
	notApplied := 0
	for _, row := range staffGroupsBySeq(groups) {
		if staffOutcomeFromName(row.Outcome) != staffOutcomeDone {
			notApplied++
			continue
		}
		link, linked := byGroup[row.GroupChatID]
		if linked {
			link.GroupTitle = row.GroupTitle
		} else {
			link = models.StaffGroupLink{GroupChatID: row.GroupChatID, GroupTitle: row.GroupTitle}
		}
		targets = append(targets, staffUndoTarget{Row: row, Link: link, Linked: linked})
	}
	return targets, notApplied
}

// staffUndoSummaryHeader is the header of an undo's summary: the undo header and,
// when some groups were not part of it, the line that counts them. released adds the
// line saying no group was changed and the action can still be undone, which only
// the final summary of a given-back undo carries.
func staffUndoSummaryHeader(tr *i18n.Translator, card *staffActionCard, notApplied int, released bool) string {
	header := staffUndoHeader(tr, card)
	if notApplied > 0 {
		note, _ := tr.GetString("staff_undo_not_applied_note", i18n.TranslationParams{"count": notApplied})
		header += "\n" + note
	}
	if released {
		note, _ := tr.GetString("staff_undo_released_note")
		header += "\n" + note
	}
	return header
}

// anyStaffGroupReached reports whether any group of an undo run got as far as its
// Telegram write call.
func anyStaffGroupReached(results []staffGroupResult) bool {
	for _, res := range results {
		if res.Reached {
			return true
		}
	}
	return false
}

// startStaffUndoRun runs a claimed undo as a staffRunSpec on the same engine the
// staff actions use. Each group goes through runStaffUndoInGroup and its result is
// stored as the group finishes. At the end the claim is given back when no group
// reached a Telegram write (owner decision b on D-09), and otherwise the record is
// finalized once. The original summary is edited after the undo's own summary is
// delivered, and only when the claim was kept (D-08). claimedAt is the claim time
// staff.ClaimUndo returned, which names this run's claim to staff.ReleaseUndo. runCtx
// is the context joinStaffRuns returned before the claim; the run's coordinator owns
// the matching wait-group Done.
func startStaffUndoRun(
	b *gotgbot.Bot,
	runCtx context.Context,
	card *staffActionCard,
	a *models.StaffAction,
	groups []models.StaffActionGroup,
	targets []staffUndoTarget,
	notApplied int,
	claimedAt time.Time,
	chatID, msgID int64,
) {
	links := make([]models.StaffGroupLink, len(targets))
	for i, t := range targets {
		links[i] = t.Link
	}
	// released and undone are written by Finish and read by Render and Delivered. All
	// of them run on the run's coordinator goroutine after the fan-out ended (Render
	// also runs there for progress edits, before Finish, when both are still zero),
	// so no lock is needed.
	released := false
	undone := 0
	startJoinedStaffRun(b, runCtx, staffRunSpec{
		Card:   card,
		Links:  links,
		ChatID: chatID,
		MsgID:  msgID,
		Group: func(ctx context.Context, i int, pass *staffOwnerPass) staffGroupResult {
			return runStaffUndoInGroup(ctx, b, card, a, targets[i], pass)
		},
		AfterGroup: func(ctx context.Context, _ int, res staffGroupResult) {
			if err := staff.SaveUndoResult(a.ID, staffResultRow(res)); err != nil {
				log.Errorf("[StaffActions] save undo result of group %d for action %d: %v", res.Link.GroupChatID, a.ID, err)
			}
			// The post comes after the record write and never changes the result.
			if res.Outcome == staffOutcomeDone {
				postStaffUndoLog(ctx, b, card, a, res.Link)
			}
		},
		Render: func(tr *i18n.Translator, results []staffGroupResult, final bool) (string, []string) {
			return composeStaffSummary(tr, staffUndoSummaryHeader(tr, card, notApplied, final && released), results, final)
		},
		Finish: func(results []staffGroupResult) gotgbot.InlineKeyboardMarkup {
			undone, _, _, _ = staffSummaryTally(results)
			// An undo that reached no Telegram write in any group changed nothing, so
			// it gives the action's one undo back. Written with db.DB, never the run's
			// context, and while the run still holds the target lock. One retry after
			// an error. Anything but a confirmed release falls through to the
			// finalize below, so the claim is kept (fail closed, D-09).
			if !anyStaffGroupReached(results) {
				var err error
				for attempt := 0; attempt < 2; attempt++ {
					if released, err = staff.ReleaseUndo(a.ID, card.Issuer, claimedAt); err == nil {
						break
					}
				}
				switch {
				case err != nil:
					log.Errorf("[StaffActions] give back the undo of action %d: %v", a.ID, err)
				case released:
					return gotgbot.InlineKeyboardMarkup{}
				default:
					log.Warnf("[StaffActions] the undo claim of action %d no longer matched; keeping it", a.ID)
				}
			}
			rows := make([]staff.ActionGroupResult, len(results))
			for i, res := range results {
				rows[i] = staffResultRow(res)
			}
			// Written with db.DB, never the run's context, so a shutdown that
			// cancelled the run cannot lose the undo's record. One retry, like the
			// action's finalize.
			var err error
			for attempt := 0; attempt < 2; attempt++ {
				if err = staff.FinalizeUndo(a.ID, rows); err == nil {
					break
				}
			}
			if err != nil {
				log.Errorf("[StaffActions] finalize undo of action %d: %v", a.ID, err)
			}
			// An undo cannot itself be undone, so its summary never has a button.
			return gotgbot.InlineKeyboardMarkup{}
		},
		Delivered: func(int64) {
			// The original keeps its text and its Undo button when the claim was given
			// back: nothing was undone.
			if released {
				return
			}
			// "Undone by" only when at least one group was undone; when writes were
			// tried and none succeeded the original says nothing changed (D-08).
			markerKey := "staff_undo_marker_nothing"
			if undone > 0 {
				markerKey = "staff_undo_marker"
			}
			editStaffUndoneOriginal(b, staffChatTranslator(card.StaffChat), a, groups, chatID, card.IssuerName, markerKey)
		},
	})
}

// runStaffUndoInGroup is the per-group chain of an undo, with the member who pressed
// Undo (card.Issuer) as the actor. A group no longer linked is skipped with no
// Telegram call. Otherwise the shared prechecks run for the presser: link owner,
// the presser's live getChatMember (creator, or administrator with
// can_restrict_members), the bot and service-ID guards and the target's live state.
// Only then does decideStaffUndo choose the one call, which executeStaffUndoCall
// makes.
func runStaffUndoInGroup(
	ctx context.Context,
	b *gotgbot.Bot,
	card *staffActionCard,
	a *models.StaffAction,
	t staffUndoTarget,
	pass *staffOwnerPass,
) staffGroupResult {
	link := t.Link
	result := func(reason staffReason, detail string) staffGroupResult {
		return staffGroupResult{Link: link, Outcome: staffReasonOutcome(reason), Reason: reason, Detail: detail}
	}
	if !t.Linked {
		return result(staffReasonSkipLinkRemoved, "")
	}

	target, early, ok := staffGroupPrechecks(ctx, b, card, link, pass)
	if !ok {
		return early
	}

	prior, err := staffPriorFromRow(t.Row)
	if err != nil {
		log.Warnf("[StaffActions] undo of action %d: %v", a.ID, err)
		return result(staffReasonSkipNoPriorState, "")
	}
	var appliedAt int64
	if t.Row.AppliedAt != nil {
		appliedAt = t.Row.AppliedAt.Unix()
	}
	verdict := decideStaffUndo(card.UndoKind, prior,
		staffAppliedState{Until: a.UntilDate, AppliedAt: appliedAt},
		staffTargetStateFrom(target), time.Now().Unix())
	if verdict.Call == staffCallNone {
		return result(verdict.Reason, "")
	}
	if ctx.Err() != nil {
		return result(staffReasonFailInterrupted, "")
	}
	// From here the write call has been made, whatever it returns: the group counts as
	// reached, so the run keeps its undo claim (D-09).
	reached := func(reason staffReason, detail string) staffGroupResult {
		res := result(reason, detail)
		res.Reached = true
		return res
	}
	if err := executeStaffUndoCall(ctx, b, link.GroupChatID, card.Target, verdict); err != nil {
		log.Warnf("[StaffActions] undo of %s in group %d: %v", card.UndoKind, link.GroupChatID, err)
		reason, detail := classifyStaffFailure(ctx, b, link.GroupChatID, err)
		return reached(reason, detail)
	}
	return reached(verdict.Reason, "")
}

// executeStaffUndoCall makes the one write call an undo verdict asks for. Like
// executeStaffCall it switches only on the verdict, so decideStaffUndo stays the one
// place undo calls are chosen: a ban, unban or unmute goes through executeStaffCall
// and a restore is one restrictChatMember with the recorded permissions, sent with
// use_independent_chat_permissions so Telegram stores exactly that set. Every call is
// paced and has its own timeout.
func executeStaffUndoCall(
	ctx context.Context,
	b *gotgbot.Bot,
	groupID, targetID int64,
	v staffUndoVerdict,
) error {
	switch v.Call {
	case staffCallBan:
		return executeStaffCall(ctx, b, groupID, targetID, v.staffVerdict, v.Until)
	case staffCallUnban, staffCallUnmute:
		return executeStaffCall(ctx, b, groupID, targetID, v.staffVerdict, 0)
	case staffCallRestore:
		return staffPaced(ctx, func(ctx context.Context) error {
			callCtx, cancel := context.WithTimeout(ctx, staffActionCallTimeout)
			defer cancel()
			_, err := b.RestrictChatMemberWithContext(callCtx, groupID, targetID, v.Perms,
				&gotgbot.RestrictChatMemberOpts{UseIndependentChatPermissions: true, UntilDate: v.Until})
			return err
		})
	}
	return fmt.Errorf("staff undo cannot make call %d", v.Call)
}
