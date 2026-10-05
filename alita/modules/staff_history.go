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

// staffHistoryList handles the Recent actions, Prev and Next buttons: it edits the
// message the button sits on into the history page that starts at the offset in
// fields["o"]. Authority is the same as the panel's: the chat is the one the
// button sits in, which must be a Staff Group, and the presser must be a live
// member of it. Only that chat's actions are read. The press is answered exactly
// once.
func (moduleStruct) staffHistoryList(
	b *gotgbot.Bot,
	query *gotgbot.CallbackQuery,
	tr *i18n.Translator,
	fields map[string]string,
) error {
	offset, err := strconv.Atoi(fields["o"])
	if err != nil || offset < 0 || offset > staffHistoryMaxOffset {
		text, _ := tr.GetString("staff_cb_expired")
		answerStaffCallback(b, query, text, false)
		return ext.EndGroups
	}

	chat := query.Message.GetChat()
	group, err := staff.GetStaffGroupFresh(chat.Id)
	if err != nil {
		text, _ := tr.GetString("staff_check_failed")
		answerStaffCallback(b, query, text, true)
		return ext.EndGroups
	}
	if group == nil {
		text, _ := tr.GetString("staff_cb_expired")
		answerStaffCallback(b, query, text, false)
		return ext.EndGroups
	}

	member, err := chat_status.IsUserInChatWithError(b, &chat, query.From.Id)
	if err != nil {
		log.Warnf("[Staff] history press: membership check of user %d in chat %d failed: %v", query.From.Id, chat.Id, err)
		text, _ := tr.GetString("staff_check_failed")
		answerStaffCallback(b, query, text, true)
		return ext.EndGroups
	}
	if !member {
		text, _ := tr.GetString("staff_cb_members_only")
		answerStaffCallback(b, query, text, true)
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
