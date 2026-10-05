package modules

import (
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
)

const (
	// staffHistoryPageSize is how many entries one page of the history shows.
	staffHistoryPageSize = 10
	// staffHistoryNameRunes caps a target name or issuer name in one line.
	staffHistoryNameRunes = 16
	// staffHistoryReasonRunes caps a reason in one line.
	staffHistoryReasonRunes = 24
	// staffHistoryMaxOffset is the largest offset a callback may carry; anything
	// above it is refused as expired.
	staffHistoryMaxOffset = 1_000_000
	// staffHistoryDetailPerRow is how many detail buttons share a keyboard row.
	staffHistoryDetailPerRow = 5
	// staffHistoryDetailReasonRunes caps the reason in a detail header, so a reason
	// typed at Telegram's own message limit cannot push the view over the cap.
	staffHistoryDetailReasonRunes = 300
)

// staffHistoryCut makes a stored name or reason safe for one history line: the
// whitespace is collapsed to single spaces (a newline would break the one-line
// entry), the text is cut to n runes plus "…" when longer, and only then escaped,
// so an entity is never cut in half.
func staffHistoryCut(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) > n {
		s = string([]rune(s)[:n]) + "…"
	}
	return html.EscapeString(s)
}

// staffHistoryDuration is the compact duration of a ban or mute: "2d" as typed, or
// the permanent text when there was no end date or it was over the limit. Every
// other action has no duration and gets "".
func staffHistoryDuration(tr *i18n.Translator, a *models.StaffAction) string {
	if !staffActionHasDuration(staffActionKind(a.Action)) {
		return ""
	}
	if a.OverLimit || a.DurationSec == 0 || a.DurationAmount <= 0 {
		text, _ := tr.GetString("staff_act_duration_permanent")
		return text
	}
	return strconv.FormatInt(a.DurationAmount, 10) + html.EscapeString(a.DurationUnit)
}

// staffHistoryState is the closing segment of an entry. An undo that was claimed
// reads "undone". A run that never finished reads "running" while its last update
// is younger than the target lock's lifetime, and "interrupted" after that, since a
// run that long dead will not write again. A finished record that was not undone
// has no state segment.
func staffHistoryState(tr *i18n.Translator, a *models.StaffAction, now time.Time) string {
	var text string
	switch {
	case a.UndoStartedAt != nil:
		text, _ = tr.GetString("staff_history_undone")
	case a.FinishedAt == nil && now.Sub(a.UpdatedAt) < staffTargetLockTTL:
		text, _ = tr.GetString("staff_history_running")
	case a.FinishedAt == nil:
		text, _ = tr.GetString("staff_history_interrupted")
	}
	return text
}

// staffHistoryLine is one entry of the history: its number, the action's icon and
// name, then the target, the duration (ban and mute), the reason, who issued it,
// when (UTC), the per-group tally and the state, joined by " · ". Stored names and
// reasons are cut and escaped (staffHistoryCut) and the issuer is spliced in after
// translation, so no user text goes through the translator.
func staffHistoryLine(tr *i18n.Translator, index int, a *models.StaffAction, t staff.ActionTally, now time.Time) string {
	kind := staffActionKind(a.Action)
	var sb strings.Builder
	sb.WriteString(strconv.Itoa(index))
	sb.WriteString(". ")
	sb.WriteString(staffActionIcon(kind))
	sb.WriteString(" ")
	sb.WriteString(staffActionName(tr, kind))
	segment := func(text string) {
		sb.WriteString(" · ")
		sb.WriteString(text)
	}

	name := staffHistoryCut(a.TargetName, staffHistoryNameRunes)
	if name == "" {
		unknown, _ := tr.GetString("staff_act_target_unknown_name")
		name = html.EscapeString(unknown)
	}
	segment(name + " (<code>" + strconv.FormatInt(a.TargetUserID, 10) + "</code>)")

	if duration := staffHistoryDuration(tr, a); duration != "" {
		segment(duration)
	}

	reason := staffHistoryCut(a.Reason, staffHistoryReasonRunes)
	if reason == "" {
		reason, _ = tr.GetString("staff_act_no_reason")
	}
	segment(reason)

	issuer := staffHistoryCut(a.IssuerName, staffHistoryNameRunes)
	if issuer == "" {
		issuer = strconv.FormatInt(a.IssuerUserID, 10)
	}
	by, _ := tr.GetString("staff_history_by", i18n.TranslationParams{"name": staffUserToken})
	segment(strings.Replace(by, staffUserToken, issuer, 1))

	segment(a.CreatedAt.UTC().Format("2 Jan 15:04"))
	segment(fmt.Sprintf("✅%d ⏭%d ❌%d", t.Done, t.Skipped, t.Failed))
	if state := staffHistoryState(tr, a, now); state != "" {
		segment(state)
	}
	return sb.String()
}

// staffHistoryButton builds one button of the history keyboard from callback
// fields. It reports false, and logs, when the data does not fit Telegram's 64
// bytes, so no dead button ships.
func staffHistoryButton(label string, fields map[string]string) (gotgbot.InlineKeyboardButton, bool) {
	data := encodeCallbackData(staffCallbackNamespace, fields)
	if data == "" {
		log.Warnf("[Staff] history button %q skipped: callback data does not fit", label)
		return gotgbot.InlineKeyboardButton{}, false
	}
	return gotgbot.InlineKeyboardButton{Text: label, CallbackData: data}, true
}

// renderStaffHistory builds the text and keyboard of one page of the history and
// reports how many entries the text shows. actions holds the page's records, newest
// first, and may carry one record more than a page (the probe for hasMore). It is
// pure: no I/O. Paging uses an offset cursor, so when the text has to shrink to
// fit the length cap, Next starts right after the last entry shown and no entry
// is hidden. The keyboard is a Prev and Next row, then a Back row to the links
// panel.
func renderStaffHistory(
	tr *i18n.Translator,
	actions []models.StaffAction,
	tallies map[uint]staff.ActionTally,
	offset int,
	hasMore bool,
	now time.Time,
) (string, gotgbot.InlineKeyboardMarkup, int) {
	title, _ := tr.GetString("staff_history_title")
	head := "<b>" + title + "</b>"

	var text string
	shown := 0
	if len(actions) == 0 {
		empty, _ := tr.GetString("staff_history_empty")
		text = head + "\n\n" + empty
	} else {
		page := actions[:min(len(actions), staffHistoryPageSize)]
		lines := make([]string, len(page))
		for i := range page {
			lines[i] = staffHistoryLine(tr, offset+i+1, &page[i], tallies[page[i].ID], now)
		}
		text, shown = fitStaffSummaryLines(head, lines, "", true)
	}

	var keyboard gotgbot.InlineKeyboardMarkup
	// One detail button per entry shown, labelled with the entry's number, five to a
	// row. An entry whose data does not fit is skipped, never shipped dead.
	var detailRow []gotgbot.InlineKeyboardButton
	for i := 0; i < shown; i++ {
		fields := map[string]string{
			"a": staffActDetail,
			"r": strconv.FormatUint(uint64(actions[i].ID), 10),
			"o": strconv.Itoa(offset),
		}
		if button, ok := staffHistoryButton(strconv.Itoa(offset+i+1), fields); ok {
			detailRow = append(detailRow, button)
		}
		if len(detailRow) == staffHistoryDetailPerRow || (i == shown-1 && len(detailRow) > 0) {
			keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, detailRow)
			detailRow = nil
		}
	}
	var paging []gotgbot.InlineKeyboardButton
	if offset > 0 {
		label, _ := tr.GetString("staff_panel_prev")
		prevOffset := max(0, offset-staffHistoryPageSize)
		if button, ok := staffHistoryButton(label, map[string]string{"a": staffActRecent, "o": strconv.Itoa(prevOffset)}); ok {
			paging = append(paging, button)
		}
	}
	if hasMore || shown < len(actions) {
		label, _ := tr.GetString("staff_panel_next")
		if button, ok := staffHistoryButton(label, map[string]string{"a": staffActRecent, "o": strconv.Itoa(offset + shown)}); ok {
			paging = append(paging, button)
		}
	}
	if len(paging) > 0 {
		keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, paging)
	}
	backLabel, _ := tr.GetString("staff_history_back")
	if back, ok := staffHistoryButton(backLabel, map[string]string{"a": staffActRefresh, "p": "0"}); ok {
		keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, []gotgbot.InlineKeyboardButton{back})
	}
	return text, keyboard, shown
}

// staffRecentButton builds the Recent actions button of the /staff panel. ok is
// false when the data does not fit Telegram's 64 bytes.
func staffRecentButton(tr *i18n.Translator) (gotgbot.InlineKeyboardButton, bool) {
	label, _ := tr.GetString("staff_panel_recent_button")
	return staffHistoryButton(label, map[string]string{"a": staffActRecent, "o": "0"})
}

// parseStaffHistoryOffset reads the list offset a history button carries. It
// reports false for anything outside 0..staffHistoryMaxOffset.
func parseStaffHistoryOffset(fields map[string]string) (int, bool) {
	offset, err := strconv.Atoi(fields["o"])
	if err != nil || offset < 0 || offset > staffHistoryMaxOffset {
		return 0, false
	}
	return offset, true
}

// staffHistoryGate is the access sequence of every history button. The chat is the
// one the button sits in, which must be a Staff Group, and the presser must be a
// live member of it. When it reports false it has already answered the press (an
// expired toast, a members-only or check-failed alert) and the caller must do
// nothing more. When it reports true the press is still unanswered.
func staffHistoryGate(
	b *gotgbot.Bot,
	query *gotgbot.CallbackQuery,
	tr *i18n.Translator,
) (gotgbot.Chat, bool) {
	chat := query.Message.GetChat()
	group, err := staff.GetStaffGroupFresh(chat.Id)
	if err != nil {
		text, _ := tr.GetString("staff_check_failed")
		answerStaffCallback(b, query, text, true)
		return chat, false
	}
	if group == nil {
		text, _ := tr.GetString("staff_cb_expired")
		answerStaffCallback(b, query, text, false)
		return chat, false
	}

	member, err := chat_status.IsUserInChatWithError(b, &chat, query.From.Id)
	if err != nil {
		log.Warnf("[Staff] history press: membership check of user %d in chat %d failed: %v", query.From.Id, chat.Id, err)
		text, _ := tr.GetString("staff_check_failed")
		answerStaffCallback(b, query, text, true)
		return chat, false
	}
	if !member {
		text, _ := tr.GetString("staff_cb_members_only")
		answerStaffCallback(b, query, text, true)
		return chat, false
	}
	return chat, true
}

// staffHistoryList handles the Recent actions, Prev and Next buttons: it edits the
// message the button sits on into the history page that starts at the offset in
// fields["o"]. Authority is staffHistoryGate's. Only that chat's actions are read.
// The press is answered exactly once.
func (moduleStruct) staffHistoryList(
	b *gotgbot.Bot,
	query *gotgbot.CallbackQuery,
	tr *i18n.Translator,
	fields map[string]string,
) error {
	offset, ok := parseStaffHistoryOffset(fields)
	if !ok {
		text, _ := tr.GetString("staff_cb_expired")
		answerStaffCallback(b, query, text, false)
		return ext.EndGroups
	}

	chat, ok := staffHistoryGate(b, query, tr)
	if !ok {
		return ext.EndGroups
	}

	answerStaffCallback(b, query, "", false)

	rows, err := staff.ListActionsFresh(chat.Id, offset, staffHistoryPageSize+1)
	if err != nil {
		log.Errorf("[Staff] history press: list actions of chat %d: %v", chat.Id, err)
		return ext.EndGroups
	}
	ids := make([]uint, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	tallies, err := staff.TallyActionGroups(ids)
	if err != nil {
		log.Errorf("[Staff] history press: tally actions of chat %d: %v", chat.Id, err)
		return ext.EndGroups
	}

	text, keyboard, _ := renderStaffHistory(tr, rows, tallies, offset, len(rows) > staffHistoryPageSize, time.Now())
	_ = editStaffMessage(b, query.Message, text, keyboard)
	return ext.EndGroups
}

// renderStaffHistoryDetail builds the full record of one staff action: the run
// summary's header, who issued it and when, the tally, and one line per linked
// group in the order the run visited them, in the summary's own line format, then
// the undo's outcome once an undo was claimed. It is pure: no I/O. linked holds the
// group IDs still linked to the Staff Group; a group missing from it keeps the title
// stored at action time and is marked as no longer linked (a nil map marks none).
// The text never exceeds the length cap and never drops a group silently
// (fitStaffHistoryDetail). The keyboard is one Back button to the list at offset.
func renderStaffHistoryDetail(
	tr *i18n.Translator,
	a *models.StaffAction,
	groups []models.StaffActionGroup,
	linked map[int64]bool,
	offset int,
	now time.Time,
) (string, gotgbot.InlineKeyboardMarkup) {
	card := staffCardFromRecord(a)
	if utf8.RuneCountInString(card.Reason) > staffHistoryDetailReasonRunes {
		card.Reason = string([]rune(card.Reason)[:staffHistoryDetailReasonRunes]) + "…"
	}
	results := staffResultsFromRecord(groups)
	// A run that never finished and has not written for longer than the target lock
	// lives is dead: its pending groups read as failed "interrupted by restart".
	// This is display only, nothing is written.
	if a.FinishedAt == nil && now.Sub(a.UpdatedAt) >= staffTargetLockTTL {
		for i := range results {
			if results[i].Outcome == staffOutcomePending {
				results[i].Outcome = staffOutcomeFailed
				results[i].Reason = staffReasonFailInterrupted
			}
		}
	}
	done, skipped, failed, _ := staffSummaryTally(results)

	issuer := staffHistoryCut(a.IssuerName, staffHistoryNameRunes)
	if issuer == "" {
		issuer = strconv.FormatInt(a.IssuerUserID, 10)
	}
	by, _ := tr.GetString("staff_history_by", i18n.TranslationParams{"name": staffUserToken})
	head := staffActionHeader(tr, card) + "\n" +
		strings.Replace(by, staffUserToken, issuer, 1) + " · " +
		a.CreatedAt.UTC().Format("2 Jan 15:04") + " UTC"
	if state := staffHistoryState(tr, a, now); state != "" {
		head += " · " + state
	}
	head += "\n" + staffSummaryTallyLine(done, skipped, failed)

	// The undo block, once an undo was claimed: who and when, then the undo's result
	// for each group the action was applied in.
	var undoHeader string
	var undoResults []staffGroupResult
	if a.UndoStartedAt != nil {
		undoBy := staffHistoryCut(a.UndoByName, staffHistoryNameRunes)
		if undoBy == "" && a.UndoBy != nil {
			undoBy = strconv.FormatInt(*a.UndoBy, 10)
		}
		text, _ := tr.GetString("staff_history_undone_by", i18n.TranslationParams{
			"name": staffUserToken,
			"time": a.UndoStartedAt.UTC().Format("2 Jan 15:04"),
		})
		undoHeader = strings.Replace(text, staffUserToken, undoBy, 1)
		undoResults = staffUndoResultsFromRecord(groups)
	}

	unlinked, _ := tr.GetString("staff_history_unlinked")
	lineOf := func(res staffGroupResult) string {
		line := staffResultLine(tr, res)
		if linked != nil && !linked[res.Link.GroupChatID] {
			line += " · " + unlinked
		}
		return line
	}
	lines := make([]string, len(results))
	for i, res := range results {
		lines[i] = lineOf(res)
	}
	undoLines := make([]string, len(undoResults))
	for i, res := range undoResults {
		undoLines[i] = lineOf(res)
	}
	// An undo with no group to show has only its header, which rides in the head so
	// it can never be the one thing the fitting cuts.
	if undoHeader != "" && len(undoResults) == 0 {
		head += "\n" + undoHeader
		undoHeader = ""
	}

	text := fitStaffHistoryDetail(tr, head, results, lines, undoHeader, undoResults, undoLines)

	var keyboard gotgbot.InlineKeyboardMarkup
	backLabel, _ := tr.GetString("staff_history_back_list")
	if back, ok := staffHistoryButton(backLabel, map[string]string{"a": staffActRecent, "o": strconv.Itoa(offset)}); ok {
		keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, []gotgbot.InlineKeyboardButton{back})
	}
	return text, keyboard
}

// staffDetailItem is one line of the detail text and how many groups it stands for:
// 1 for a group's line, the number of groups for a collapsed done line.
type staffDetailItem struct {
	text   string
	groups int
}

// staffDetailBlock lists one block's lines. With collapse the done lines are
// replaced by one "N groups done" line placed first, like the run summary does.
func staffDetailBlock(tr *i18n.Translator, results []staffGroupResult, lines []string, collapse bool) []staffDetailItem {
	var items []staffDetailItem
	if collapse {
		done := 0
		for _, res := range results {
			if res.Outcome == staffOutcomeDone {
				done++
			}
		}
		if done > 0 {
			text, _ := tr.GetString("staff_act_summary_done_collapsed", i18n.TranslationParams{"count": done})
			items = append(items, staffDetailItem{text: "✅ " + text, groups: done})
		}
	}
	for i, res := range results {
		if collapse && res.Outcome == staffOutcomeDone {
			continue
		}
		items = append(items, staffDetailItem{text: lines[i], groups: 1})
	}
	return items
}

// fitStaffHistoryDetail joins the head, the group lines and the undo block into
// one text within the length cap, and never drops a group silently. A text over the
// cap first collapses the done lines of each block into a count; when that is still
// too long as many lines as fit are listed and the text ends with the number of
// groups left out. The undo header always travels with the first line under it.
func fitStaffHistoryDetail(
	tr *i18n.Translator,
	head string,
	results []staffGroupResult,
	lines []string,
	undoHeader string,
	undoResults []staffGroupResult,
	undoLines []string,
) string {
	assemble := func(collapse bool) []staffDetailItem {
		items := staffDetailBlock(tr, results, lines, collapse)
		if undoHeader == "" {
			return items
		}
		block := staffDetailBlock(tr, undoResults, undoLines, collapse)
		block[0].text = "\n" + undoHeader + "\n" + block[0].text
		return append(items, block...)
	}
	texts := func(items []staffDetailItem) []string {
		out := make([]string, len(items))
		for i, item := range items {
			out[i] = item.text
		}
		return out
	}
	join := func(items []staffDetailItem) string {
		if len(items) == 0 {
			return head
		}
		return staffSummaryText(head, texts(items))
	}

	items := assemble(false)
	if text := join(items); staffSummaryFits(text) {
		return text
	}
	items = assemble(true)
	if text := join(items); staffSummaryFits(text) {
		return text
	}

	moreText := func(n int) string {
		text, _ := tr.GetString("staff_history_more_groups", i18n.TranslationParams{"count": n})
		return text
	}
	total := 0
	for _, item := range items {
		total += item.groups
	}
	// Size the tail with the largest count it can carry, so the final one, with a
	// count of at most as many digits, always fits.
	_, n := fitStaffSummaryLines(head, texts(items), moreText(total), true)
	left := 0
	for _, item := range items[n:] {
		left += item.groups
	}
	if left == 0 {
		return join(items[:n])
	}
	text, _ := fitStaffSummaryLines(head, texts(items[:n]), moreText(left), true)
	return text
}

// staffHistoryDetail handles an entry's detail button: it edits the message the
// button sits on into that action's full record. A malformed or zero record ID or
// offset is answered as expired before anything is read. Authority is
// staffHistoryGate's, and the record is then loaded fresh and refused unless its
// staff_chat_id is the chat of the pressed message, so a forged or replayed button
// naming another Staff Group's record shows nothing. Every refusal is answered
// before any edit, and the press is answered exactly once.
func (moduleStruct) staffHistoryDetail(
	b *gotgbot.Bot,
	query *gotgbot.CallbackQuery,
	tr *i18n.Translator,
	fields map[string]string,
) error {
	id, idErr := strconv.ParseUint(fields["r"], 10, 63)
	offset, offsetOK := parseStaffHistoryOffset(fields)
	if idErr != nil || id == 0 || !offsetOK {
		text, _ := tr.GetString("staff_cb_expired")
		answerStaffCallback(b, query, text, false)
		return ext.EndGroups
	}

	chat, ok := staffHistoryGate(b, query, tr)
	if !ok {
		return ext.EndGroups
	}

	a, err := staff.GetActionFresh(uint(id))
	if err != nil {
		text, _ := tr.GetString("staff_check_failed")
		answerStaffCallback(b, query, text, true)
		return ext.EndGroups
	}
	if a == nil {
		text, _ := tr.GetString("staff_cb_expired")
		answerStaffCallback(b, query, text, false)
		return ext.EndGroups
	}
	if a.StaffChatID != chat.Id {
		text, _ := tr.GetString("staff_cb_denied")
		answerStaffCallback(b, query, text, true)
		return ext.EndGroups
	}

	groups, err := staff.ListActionGroupsFresh(a.ID)
	if err != nil {
		log.Errorf("[Staff] detail press: list groups of action %d: %v", a.ID, err)
		text, _ := tr.GetString("staff_check_failed")
		answerStaffCallback(b, query, text, true)
		return ext.EndGroups
	}
	links, err := staff.ListLinksByStaffFresh(chat.Id)
	if err != nil {
		log.Errorf("[Staff] detail press: list links of chat %d: %v", chat.Id, err)
		text, _ := tr.GetString("staff_check_failed")
		answerStaffCallback(b, query, text, true)
		return ext.EndGroups
	}
	linked := make(map[int64]bool, len(links))
	for _, link := range links {
		linked[link.GroupChatID] = true
	}

	answerStaffCallback(b, query, "", false)
	text, keyboard := renderStaffHistoryDetail(tr, a, groups, linked, offset, time.Now())
	_ = editStaffMessage(b, query.Message, text, keyboard)
	return ext.EndGroups
}
