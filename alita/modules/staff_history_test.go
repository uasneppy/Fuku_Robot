//go:build testtools

package modules

import (
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/db/staff"
	"github.com/divkix/Alita_Robot/alita/i18n"
)

// historyTargetSeq hands out target IDs that no other test uses, so a test can
// assert that one chat's history never carries another chat's target.
var historyTargetSeq atomic.Int64

func init() { historyTargetSeq.Store(710_000_000) }

// seedStaffActions stores n finished staff actions in staffChat, oldest first,
// each with one done group row. edit may change a record before it is inserted.
// The records are returned with their IDs; the cleanup of staffChat's rows is the
// caller's staffCleanup.
func seedStaffActions(t *testing.T, staffChat int64, n int, edit func(i int, a *models.StaffAction)) []models.StaffAction {
	t.Helper()
	out := make([]models.StaffAction, 0, n)
	for i := 0; i < n; i++ {
		action := models.StaffAction{
			StaffChatID:   staffChat,
			IssuerUserID:  5000 + int64(i),
			IssuerName:    fmt.Sprintf("Issuer%d", i),
			TargetUserID:  historyTargetSeq.Add(1),
			TargetName:    fmt.Sprintf("Target%d", i),
			Action:        "kick",
			Reason:        "spamming",
			GroupCount:    1,
			SummaryChatID: staffChat,
			SummaryMsgID:  int64(100 + i),
		}
		if edit != nil {
			edit(i, &action)
		}
		group := models.StaffActionGroup{
			GroupChatID: uniqueModuleChatID(),
			GroupTitle:  "Group",
			Outcome:     models.StaffActionOutcomePending,
		}
		if err := staff.CreateAction(&action, []models.StaffActionGroup{group}); err != nil {
			t.Fatalf("CreateAction %d: %v", i, err)
		}
		err := staff.FinalizeAction(action.ID, []staff.ActionGroupResult{
			{GroupChatID: group.GroupChatID, Outcome: models.StaffActionOutcomeDone},
		})
		if err != nil {
			t.Fatalf("FinalizeAction %d: %v", i, err)
		}
		out = append(out, action)
	}
	return out
}

var historyEntryRe = regexp.MustCompile(`(?m)^\d+\. .*$`)

// historyEntryLines returns the entry lines of a history text: the lines that start
// with a number and a dot.
func historyEntryLines(text string) []string { return historyEntryRe.FindAllString(text, -1) }

// historyKeys is a history keyboard split by what each button does.
type historyKeys struct {
	next, prev, back *gotgbot.InlineKeyboardButton
	// backList is the detail view's Back to list button; detail holds the per-entry
	// detail buttons of a list, in order.
	backList *gotgbot.InlineKeyboardButton
	detail   []gotgbot.InlineKeyboardButton
	fields   map[string]map[string]string
	total    int
}

// historyKeyboardOf classifies every button of a history keyboard and fails on one
// whose data is empty, longer than 64 bytes, undecodable or of an unknown action.
func historyKeyboardOf(t *testing.T, keyboard [][]gotgbot.InlineKeyboardButton) historyKeys {
	t.Helper()
	keys := historyKeys{fields: make(map[string]map[string]string)}
	for _, row := range keyboard {
		for i := range row {
			button := row[i]
			keys.total++
			if n := len(button.CallbackData); n < 1 || n > 64 {
				t.Errorf("button %q has %d bytes of callback data, want 1 to 64", button.Text, n)
			}
			decoded, ok := decodeCallbackData(button.CallbackData, staffCallbackNamespace)
			if !ok {
				t.Errorf("button %q has undecodable callback data %q", button.Text, button.CallbackData)
				continue
			}
			keys.fields[button.CallbackData] = decoded.Fields
			switch {
			case decoded.Fields["a"] == "dt":
				keys.detail = append(keys.detail, button)
			case decoded.Fields["a"] == "rc" && strings.Contains(button.Text, staffMarker("staff_history_back_list")):
				keys.backList = &button
			case decoded.Fields["a"] == "rc" && strings.Contains(button.Text, staffMarker("staff_panel_next")):
				keys.next = &button
			case decoded.Fields["a"] == "rc" && strings.Contains(button.Text, staffMarker("staff_panel_prev")):
				keys.prev = &button
			case decoded.Fields["a"] == staffActRefresh:
				keys.back = &button
			default:
				t.Errorf("button %q carries unexpected action %q", button.Text, decoded.Fields["a"])
			}
		}
	}
	return keys
}

// historyTapMsgID is the message every history test presses buttons on.
const historyTapMsgID int64 = 4242

// pressHistory presses the history list (Recent actions, Prev or Next) with the
// given offset as from in chat, and returns the text and keyboard of the edit it
// caused.
func (e *staffActionEnv) pressHistory(from gotgbot.User, chat gotgbot.Chat, offset string) (string, historyKeys) {
	e.t.Helper()
	data := encodeCallbackData(staffCallbackNamespace, map[string]string{"a": "rc", "o": offset})
	if data == "" {
		e.t.Fatalf("history callback data for offset %s did not encode", offset)
	}
	e.tapData(from, chat, historyTapMsgID, data)
	edits := e.edits(chat.Id, historyTapMsgID)
	if len(edits) == 0 {
		e.t.Fatalf("pressing history offset %s edited nothing", offset)
	}
	last := edits[len(edits)-1]
	return fmt.Sprint(last.Params["text"]), historyKeyboardOf(e.t, staffKeyboardOf(last.Params["reply_markup"]))
}

func TestStaffHistoryLine(t *testing.T) {
	tr := panelMarkerTranslator(t)
	now := time.Date(2026, 10, 6, 9, 0, 0, 0, time.UTC)
	created := time.Date(2026, 10, 5, 12, 4, 30, 0, time.UTC)
	finished := created.Add(time.Minute)
	undone := created.Add(time.Hour)

	ban := models.StaffAction{
		ID: 9, IssuerName: "Alice", TargetUserID: 123, TargetName: "Name", Action: "ban",
		Reason: "spamming", DurationSec: 172800, DurationAmount: 2, DurationUnit: "d",
		CreatedAt: created, FinishedAt: &finished, UndoStartedAt: &undone,
	}
	line := staffHistoryLine(tr, 1, &ban, staff.ActionTally{Done: 4, Skipped: 1, Failed: 1}, now)
	for _, want := range []string{
		"🔨", staffMarker("staff_act_name_ban"), "Name", "(<code>123</code>)", "2d", "spamming",
		staffMarker("staff_history_by"), "Alice", "5 Oct 12:04", "✅4 ⏭1 ❌1", staffMarker("staff_history_undone"),
	} {
		if !strings.Contains(line, want) {
			t.Errorf("ban line %q lacks %q", line, want)
		}
	}
	if strings.Contains(line, "\n") {
		t.Errorf("an entry must be one line, got %q", line)
	}

	t.Run("a kick has no duration segment", func(t *testing.T) {
		kick := models.StaffAction{
			ID: 10, IssuerName: "Alice", TargetUserID: 123, TargetName: "Name", Action: "kick",
			Reason: "spamming", CreatedAt: created, FinishedAt: &finished,
		}
		got := staffHistoryLine(tr, 2, &kick, staff.ActionTally{Done: 1}, now)
		if strings.Contains(got, staffMarker("staff_act_duration_permanent")) {
			t.Errorf("kick line %q must not carry a duration", got)
		}
		if strings.Contains(got, staffMarker("staff_history_undone")) {
			t.Errorf("a record with no undo must not read undone: %q", got)
		}
	})

	t.Run("a ban with no duration is permanent", func(t *testing.T) {
		permanent := ban
		permanent.DurationSec, permanent.DurationAmount, permanent.DurationUnit = 0, 0, ""
		permanent.UndoStartedAt = nil
		got := staffHistoryLine(tr, 1, &permanent, staff.ActionTally{Done: 1}, now)
		if !strings.Contains(got, staffMarker("staff_act_duration_permanent")) {
			t.Errorf("permanent ban line %q lacks the permanent text", got)
		}
	})

	t.Run("an empty reason says so", func(t *testing.T) {
		noReason := ban
		noReason.Reason = "  "
		got := staffHistoryLine(tr, 1, &noReason, staff.ActionTally{Done: 1}, now)
		if !strings.Contains(got, staffMarker("staff_act_no_reason")) {
			t.Errorf("line %q lacks the no-reason text", got)
		}
	})
}

func TestStaffHistoryList(t *testing.T) {
	env := newStaffActionEnv(t, 1)
	other := uniqueModuleChatID()
	staffCleanup(t, other)

	mine := seedStaffActions(t, env.staffChat, 12, nil)
	theirs := seedStaffActions(t, other, 2, nil)

	text, keys := env.pressHistory(env.issuer, env.staffChatObj(), "0")
	if !strings.Contains(text, staffMarker("staff_history_title")) {
		t.Errorf("first page %q lacks the title", text)
	}
	lines := historyEntryLines(text)
	if len(lines) != 10 {
		t.Fatalf("first page has %d entry lines, want 10:\n%s", len(lines), text)
	}
	newest := strconv.FormatInt(mine[11].TargetUserID, 10)
	if !strings.Contains(lines[0], "(<code>"+newest+"</code>)") {
		t.Errorf("first entry %q must name the newest target %s", lines[0], newest)
	}
	if keys.next == nil || keys.fields[keys.next.CallbackData]["o"] != "10" {
		t.Errorf("first page Next = %+v, want o=10", keys.next)
	}
	if keys.prev != nil {
		t.Errorf("first page must have no Prev, got %+v", keys.prev)
	}
	if keys.back == nil || keys.fields[keys.back.CallbackData]["p"] != "0" ||
		!strings.Contains(keys.back.Text, staffMarker("staff_history_back")) {
		t.Errorf("first page Back = %+v, want a=rf p=0 labelled staff_history_back", keys.back)
	}

	second, keys2 := env.pressHistory(env.issuer, env.staffChatObj(), keys.fields[keys.next.CallbackData]["o"])
	if got := historyEntryLines(second); len(got) != 2 {
		t.Fatalf("second page has %d entry lines, want 2:\n%s", len(got), second)
	}
	if keys2.next != nil {
		t.Errorf("last page must have no Next, got %+v", keys2.next)
	}
	if keys2.prev == nil || keys2.fields[keys2.prev.CallbackData]["o"] != "0" {
		t.Errorf("second page Prev = %+v, want o=0", keys2.prev)
	}

	for _, page := range []string{text, second} {
		for _, row := range theirs {
			if strings.Contains(page, strconv.FormatInt(row.TargetUserID, 10)) {
				t.Errorf("history of one Staff Group shows target %d of another", row.TargetUserID)
			}
		}
	}
}

func TestStaffHistoryPageBoundary(t *testing.T) {
	t.Run("exactly ten records have no Next", func(t *testing.T) {
		env := newStaffActionEnv(t, 1)
		seedStaffActions(t, env.staffChat, 10, nil)
		text, keys := env.pressHistory(env.issuer, env.staffChatObj(), "0")
		if got := len(historyEntryLines(text)); got != 10 {
			t.Fatalf("page has %d entries, want 10", got)
		}
		if keys.next != nil {
			t.Errorf("10 records must show no Next, got %+v", keys.next)
		}
	})

	t.Run("eleven records page to the oldest alone", func(t *testing.T) {
		env := newStaffActionEnv(t, 1)
		seeded := seedStaffActions(t, env.staffChat, 11, nil)
		_, keys := env.pressHistory(env.issuer, env.staffChatObj(), "0")
		if keys.next == nil || keys.fields[keys.next.CallbackData]["o"] != "10" {
			t.Fatalf("11 records must show Next with o=10, got %+v", keys.next)
		}
		text, _ := env.pressHistory(env.issuer, env.staffChatObj(), keys.fields[keys.next.CallbackData]["o"])
		lines := historyEntryLines(text)
		oldest := strconv.FormatInt(seeded[0].TargetUserID, 10)
		if len(lines) != 1 || !strings.Contains(lines[0], "(<code>"+oldest+"</code>)") {
			t.Errorf("second page = %v, want only the oldest record %s", lines, oldest)
		}
	})
}

func TestStaffHistoryEmpty(t *testing.T) {
	env := newStaffActionEnv(t, 1)
	text, keys := env.pressHistory(env.issuer, env.staffChatObj(), "0")
	if !strings.Contains(text, staffMarker("staff_history_empty")) {
		t.Errorf("empty history %q lacks the empty text", text)
	}
	if len(historyEntryLines(text)) != 0 {
		t.Errorf("empty history must list no entry, got %q", text)
	}
	if keys.total != 1 || keys.back == nil {
		t.Errorf("empty history keyboard has %d buttons (back %+v), want only Back", keys.total, keys.back)
	}
}

func TestStaffHistorySameSecond(t *testing.T) {
	env := newStaffActionEnv(t, 1)
	moment := time.Now().UTC().Truncate(time.Second)
	seeded := seedStaffActions(t, env.staffChat, 2, func(_ int, a *models.StaffAction) { a.CreatedAt = moment })

	text, _ := env.pressHistory(env.issuer, env.staffChatObj(), "0")
	lines := historyEntryLines(text)
	if len(lines) != 2 {
		t.Fatalf("got %d entries, want 2:\n%s", len(lines), text)
	}
	higher := strconv.FormatInt(seeded[1].TargetUserID, 10)
	if !strings.Contains(lines[0], "(<code>"+higher+"</code>)") {
		t.Errorf("two records of the same second must list the higher ID first, got %q first", lines[0])
	}
}

func TestStaffPanelRenderRecentButton(t *testing.T) {
	tr := panelMarkerTranslator(t)
	_, keyboard := renderStaffPanel(tr, panelRenderGroup(), panelRows(2, func(i int) string { return "G" + strconv.Itoa(i) }), "fukubot", 0, time.Now())
	buttons, decoded := panelSplitKeyboard(t, keyboard)
	if len(buttons.recent) != 1 {
		t.Fatalf("panel has %d Recent actions buttons, want 1", len(buttons.recent))
	}
	if !strings.Contains(buttons.recent[0].Text, staffMarker("staff_panel_recent_button")) {
		t.Errorf("Recent actions label = %q, want staff_panel_recent_button", buttons.recent[0].Text)
	}
	if got := decoded[buttons.recent[0].CallbackData]["o"]; got != "0" {
		t.Errorf("Recent actions offset = %q, want 0", got)
	}
}

func TestStaffHistoryAccess(t *testing.T) {
	env := newStaffActionEnv(t, 1)
	seedStaffActions(t, env.staffChat, 3, nil)

	history := func(offset string) string {
		data := encodeCallbackData(staffCallbackNamespace, map[string]string{"a": "rc", "o": offset})
		if data == "" {
			t.Fatalf("history callback data for offset %q did not encode", offset)
		}
		return data
	}

	t.Run("non-member", func(t *testing.T) {
		outsider := gotgbot.User{Id: env.issuer.Id + 100, FirstName: "Outsider"}
		env.fake.setMember(env.staffChat, outsider.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusLeft})
		env.tapData(outsider, env.staffChatObj(), 5001, history("0"))

		text, alert := env.lastAnswer()
		if !strings.Contains(text, staffMarker("staff_cb_members_only")) || !alert {
			t.Errorf("answer = %q alert=%v, want the members-only alert", text, alert)
		}
		if edits := env.edits(env.staffChat, 5001); len(edits) != 0 {
			t.Errorf("a non-member's press edited the message %d time(s), want none", len(edits))
		}
	})

	t.Run("not a Staff Group", func(t *testing.T) {
		plain := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Plain"}
		env.tapData(env.issuer, plain, 5002, history("0"))

		text, _ := env.lastAnswer()
		if !strings.Contains(text, staffMarker("staff_cb_expired")) {
			t.Errorf("answer = %q, want the expired answer", text)
		}
		if edits := env.edits(plain.Id, 5002); len(edits) != 0 {
			t.Errorf("a press in a chat that is not a Staff Group edited the message %d time(s), want none", len(edits))
		}
	})

	t.Run("bad offset", func(t *testing.T) {
		for i, offset := range []string{"-1", "abc", "", "1000001"} {
			msgID := int64(5100 + i)
			env.tapData(env.issuer, env.staffChatObj(), msgID, history(offset))

			text, _ := env.lastAnswer()
			if !strings.Contains(text, staffMarker("staff_cb_expired")) {
				t.Errorf("offset %q: answer = %q, want the expired answer", offset, text)
			}
			if edits := env.edits(env.staffChat, msgID); len(edits) != 0 {
				t.Errorf("offset %q edited the message %d time(s), want none", offset, len(edits))
			}
		}
	})

	t.Run("a member answers once and sees the list", func(t *testing.T) {
		before := env.answerCount()
		env.tapData(env.issuer, env.staffChatObj(), 5200, history("0"))
		if got := env.answerCount() - before; got != 1 {
			t.Errorf("the press was answered %d times, want exactly once", got)
		}
		if text := env.lastEditText(env.staffChat, 5200); !strings.Contains(text, staffMarker("staff_history_title")) {
			t.Errorf("a member's press showed %q, want the history", text)
		}
	})

	t.Run("back to links", func(t *testing.T) {
		back := encodeCallbackData(staffCallbackNamespace, map[string]string{"a": staffActRefresh, "p": "0"})
		env.tapData(env.issuer, env.staffChatObj(), 5300, back)
		if text := env.lastEditText(env.staffChat, 5300); !strings.Contains(text, staffMarker("staff_panel_chat_id")) {
			t.Errorf("Back showed %q, want the links panel", text)
		}
	})
}

func TestStaffHistoryLengthCap(t *testing.T) {
	env := newStaffActionEnv(t, 1)
	hostile := strings.Repeat(`&"<>`, 16)    // 64 runes
	longReason := strings.Repeat(`&"<>`, 50) // 200 runes
	seedStaffActions(t, env.staffChat, 10, func(_ int, a *models.StaffAction) {
		a.TargetName, a.IssuerName, a.Reason = hostile, hostile, longReason
	})

	text, keys := env.pressHistory(env.issuer, env.staffChatObj(), "0")
	if got := staffSummaryLen(text); got > staffPanelMaxUTF16 {
		t.Errorf("history text is %d UTF-16 units, want at most %d", got, staffPanelMaxUTF16)
	}
	if strings.Contains(text, "<>") || strings.Contains(text, `&"`) {
		t.Errorf("a name or reason reached the HTML unescaped: %q", text)
	}
	shown := len(historyEntryLines(text))
	if shown < 1 {
		t.Fatalf("the page shows no entry:\n%s", text)
	}
	if shown < staffHistoryPageSize {
		if keys.next == nil || keys.fields[keys.next.CallbackData]["o"] != strconv.Itoa(shown) {
			t.Errorf("Next = %+v, want it to start at offset %d, right after the last entry shown", keys.next, shown)
		}
	}
}

// TestStaffHistoryShrunkPageHidesNothing uses a translation so long that one line
// fills a third of the cap, so a page of ten entries has to shrink. Next must start
// right after the last entry shown, and the next page must number on from it.
func TestStaffHistoryShrunkPageHidesNothing(t *testing.T) {
	restore, err := i18n.OverrideManagerForTest(`staff_history_title: "T"
staff_history_by: "` + strings.Repeat("x", 600) + ` {name}"
staff_act_name_kick: "Kick"
staff_act_no_reason: "none"
staff_act_target_unknown_name: "unknown"
staff_panel_prev: "Prev"
staff_panel_next: "Next"
staff_history_back: "Back"
`)
	if err != nil {
		t.Fatalf("override locale: %v", err)
	}
	t.Cleanup(restore)
	tr := i18n.MustNewTranslator("en")

	now := time.Now()
	finished := now
	actions := make([]models.StaffAction, 10)
	for i := range actions {
		actions[i] = models.StaffAction{
			ID: uint(i + 1), Action: "kick", TargetUserID: int64(900 + i),
			CreatedAt: now, FinishedAt: &finished,
		}
	}

	text, keyboard, shown := renderStaffHistory(tr, actions, nil, 0, true, now)
	if shown < 1 || shown >= staffHistoryPageSize {
		t.Fatalf("the page shows %d entries, want between 1 and %d so it shrank", shown, staffHistoryPageSize-1)
	}
	if got := staffSummaryLen(text); got > staffPanelMaxUTF16 {
		t.Errorf("shrunk page is %d UTF-16 units, want at most %d", got, staffPanelMaxUTF16)
	}
	if got := len(historyEntryLines(text)); got != shown {
		t.Errorf("text has %d entry lines, renderStaffHistory reported %d", got, shown)
	}
	var nextOffset string
	for _, row := range keyboard.InlineKeyboard {
		for _, button := range row {
			if decoded, ok := decodeCallbackData(button.CallbackData, staffCallbackNamespace); ok && decoded.Fields["a"] == "rc" {
				nextOffset = decoded.Fields["o"]
			}
		}
	}
	if nextOffset != strconv.Itoa(shown) {
		t.Fatalf("Next offset = %q, want %d, right after the last entry shown", nextOffset, shown)
	}

	// The page that Next opens starts with the first entry the shrunk page left out.
	next, _, _ := renderStaffHistory(tr, actions[shown:], nil, shown, true, now)
	lines := historyEntryLines(next)
	if len(lines) == 0 || !strings.HasPrefix(lines[0], strconv.Itoa(shown+1)+". ") ||
		!strings.Contains(lines[0], "(<code>"+strconv.Itoa(900+shown)+"</code>)") {
		t.Errorf("the next page starts with %v, want entry %d for target %d", lines, shown+1, 900+shown)
	}
}

func TestStaffHistoryCallbackBudget(t *testing.T) {
	tr := panelMarkerTranslator(t)
	now := time.Now()
	finished := now
	actions := []models.StaffAction{{ID: 1, Action: "ban", TargetUserID: 1, CreatedAt: now, FinishedAt: &finished}}

	_, keyboard, shown := renderStaffHistory(tr, actions, nil, 999990, true, now)
	if shown != 1 {
		t.Fatalf("renderStaffHistory shows %d entries, want 1", shown)
	}
	keys := historyKeyboardOf(t, keyboard.InlineKeyboard)
	if keys.total != 4 || len(keys.detail) != 1 || keys.prev == nil || keys.next == nil || keys.back == nil {
		t.Errorf("keyboard has %d buttons (detail %d prev %v next %v back %v), want one detail button, Prev, Next and Back",
			keys.total, len(keys.detail), keys.prev != nil, keys.next != nil, keys.back != nil)
	}

	button, ok := staffRecentButton(tr)
	if !ok {
		t.Fatal("staffRecentButton did not encode")
	}
	if n := len(button.CallbackData); n < 1 || n > 64 {
		t.Errorf("Recent actions callback data is %d bytes, want 1 to 64", n)
	}
}

func TestStaffHistoryUnfinished(t *testing.T) {
	tr := panelMarkerTranslator(t)
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	record := func(updated time.Time, finished *time.Time) *models.StaffAction {
		return &models.StaffAction{ID: 1, Action: "ban", TargetUserID: 5, CreatedAt: now.Add(-2 * time.Hour), UpdatedAt: updated, FinishedAt: finished}
	}
	running, interrupted := staffMarker("staff_history_running"), staffMarker("staff_history_interrupted")

	fresh := staffHistoryLine(tr, 1, record(now, nil), staff.ActionTally{Pending: 2}, now)
	if !strings.Contains(fresh, running) || strings.Contains(fresh, interrupted) {
		t.Errorf("a record updated just now reads %q, want running", fresh)
	}

	young := staffHistoryLine(tr, 1, record(now.Add(-29*time.Minute), nil), staff.ActionTally{}, now)
	if !strings.Contains(young, running) {
		t.Errorf("a record updated 29 minutes ago reads %q, want running", young)
	}

	stale := staffHistoryLine(tr, 1, record(now.Add(-31*time.Minute), nil), staff.ActionTally{Done: 1}, now)
	if !strings.Contains(stale, interrupted) || strings.Contains(stale, running) {
		t.Errorf("a record updated 31 minutes ago reads %q, want interrupted", stale)
	}

	finished := now.Add(-time.Hour)
	done := staffHistoryLine(tr, 1, record(now.Add(-time.Hour), &finished), staff.ActionTally{Done: 1}, now)
	if strings.Contains(done, running) || strings.Contains(done, interrupted) {
		t.Errorf("a finished record reads %q, want neither running nor interrupted", done)
	}
}

// pressDetail presses a detail or Back-to-list button (raw data) on message msgID of
// chat as from, and returns the text and keyboard of the edit it caused.
func (e *staffActionEnv) pressDetail(from gotgbot.User, chat gotgbot.Chat, msgID int64, data string) (string, historyKeys) {
	e.t.Helper()
	e.tapData(from, chat, msgID, data)
	edits := e.edits(chat.Id, msgID)
	if len(edits) == 0 {
		e.t.Fatalf("pressing %q edited nothing", data)
	}
	last := edits[len(edits)-1]
	return fmt.Sprint(last.Params["text"]), historyKeyboardOf(e.t, staffKeyboardOf(last.Params["reply_markup"]))
}

// wantInOrder fails unless every want occurs in text, each after the one before.
func wantInOrder(t *testing.T, text string, wants ...string) {
	t.Helper()
	from := 0
	for _, want := range wants {
		i := strings.Index(text[from:], want)
		if i < 0 {
			t.Errorf("text %q lacks %q after offset %d", text, want, from)
			return
		}
		from += i + len(want)
	}
}

func TestStaffHistoryDetail(t *testing.T) {
	env := newStaffActionEnv(t, 3)
	// The issuer is only a plain member of Group C, so that group is skipped.
	env.fake.setMember(env.groups[2], env.issuer.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
	_, cardMsg := env.startRun("/ban 4242 2d spamming")
	env.waitRuns()
	record, _ := recordOfCard(t, cardMsg)

	_, keys := env.pressHistory(env.issuer, env.staffChatObj(), "0")
	if len(keys.detail) != 1 || keys.detail[0].Text != "1" {
		t.Fatalf("list has detail buttons %+v, want one labelled 1", keys.detail)
	}
	fields := keys.fields[keys.detail[0].CallbackData]
	if fields["r"] != strconv.FormatUint(uint64(record.ID), 10) || fields["o"] != "0" {
		t.Fatalf("detail button fields = %v, want r=%d o=0", fields, record.ID)
	}

	text, detailKeys := env.pressDetail(env.issuer, env.staffChatObj(), 6001, keys.detail[0].CallbackData)
	wantInOrder(t, text,
		staffMarker("staff_act_name_ban"),
		"(<code>4242</code>)",
		staffMarker("staff_act_duration_days"),
		"spamming",
		staffMarker("staff_history_by"),
		html.EscapeString(env.issuer.FirstName),
		"✅ 2 · ⏭ 1 · ❌ 0",
		"✅ Group A",
		"✅ Group B",
		"⏭ Group C: ",
		staffMarker("staff_act_skip_issuer_not_admin"),
	)
	if detailKeys.total != 1 || detailKeys.backList == nil ||
		detailKeys.fields[detailKeys.backList.CallbackData]["a"] != "rc" ||
		detailKeys.fields[detailKeys.backList.CallbackData]["o"] != "0" {
		t.Errorf("detail keyboard = %+v, want one Back button with a=rc o=0", detailKeys)
	}
}

func TestStaffHistoryDetailBack(t *testing.T) {
	env := newStaffActionEnv(t, 1)
	seedStaffActions(t, env.staffChat, 12, nil)

	_, page := env.pressHistory(env.issuer, env.staffChatObj(), "10")
	if len(page.detail) != 2 {
		t.Fatalf("second page has %d detail buttons, want 2", len(page.detail))
	}
	if page.detail[0].Text != "11" || page.detail[1].Text != "12" {
		t.Errorf("second page detail labels = %q, %q, want 11 and 12", page.detail[0].Text, page.detail[1].Text)
	}

	_, detailKeys := env.pressDetail(env.issuer, env.staffChatObj(), 6002, page.detail[0].CallbackData)
	if detailKeys.backList == nil {
		t.Fatal("the detail view has no Back button")
	}

	text, listKeys := env.pressDetail(env.issuer, env.staffChatObj(), 6002, detailKeys.backList.CallbackData)
	lines := historyEntryLines(text)
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "11. ") {
		t.Errorf("Back showed entries %v, want the list at offset 10 (entries 11 and 12)", lines)
	}
	if listKeys.prev == nil || listKeys.fields[listKeys.prev.CallbackData]["o"] != "0" {
		t.Errorf("the list after Back has Prev %+v, want o=0", listKeys.prev)
	}
}

// detailData encodes a detail button's data for a record ID and a list offset, both
// given as the text the callback carries so a forged value can be tried.
func detailData(t *testing.T, record, offset string) string {
	t.Helper()
	data := encodeCallbackData(staffCallbackNamespace, map[string]string{"a": "dt", "r": record, "o": offset})
	if data == "" {
		t.Fatalf("detail callback data for record %s offset %s did not encode", record, offset)
	}
	return data
}

// lineStartingWith returns the first line of text that starts with prefix, or "".
func lineStartingWith(text, prefix string) string {
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, prefix) {
			return line
		}
	}
	return ""
}

// banRecordWithSkip runs "/ban 4242 2d spamming" over three groups where the issuer
// is only a plain member of the third, so the record has two done groups and one
// skipped group, and returns the stored record.
func banRecordWithSkip(t *testing.T, env *staffActionEnv) *models.StaffAction {
	t.Helper()
	env.fake.setMember(env.groups[2], env.issuer.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
	_, cardMsg := env.startRun("/ban 4242 2d spamming")
	env.waitRuns()
	record, _ := recordOfCard(t, cardMsg)
	return record
}

func TestStaffReasonText(t *testing.T) {
	withStaffLocale(t)
	tr := i18n.MustNewTranslator("en")
	plain := []staffReason{
		staffReasonBanned, staffReasonMuted, staffReasonKicked, staffReasonUnbanned, staffReasonUnmuted,
	}
	worded := []staffReason{
		staffReasonBannedNotInGroup,
		staffReasonSkipIssuerNotAdmin, staffReasonSkipIssuerNoRight, staffReasonSkipTargetAdmin,
		staffReasonSkipTargetBot, staffReasonSkipTargetService, staffReasonSkipAlreadyBanned,
		staffReasonSkipNotInGroup, staffReasonSkipAlreadyMuted, staffReasonSkipNotBanned,
		staffReasonSkipNotMuted, staffReasonSkipLinkRemoved, staffReasonSkipStaffGroup,
		staffReasonFailOwnerUnknown, staffReasonFailLookup, staffReasonFailTelegram,
		staffReasonFailRateLimited, staffReasonFailBotNotAdmin, staffReasonFailBotNoRights,
		staffReasonFailGroupNotFound, staffReasonFailInternal, staffReasonFailInterrupted,
		staffReasonUndoneUnbanned, staffReasonUndoneUnmuted, staffReasonUndoneBanRestored,
		staffReasonUndoneRestrictionRestored, staffReasonSkipNotApplied, staffReasonSkipChangedSince,
		staffReasonSkipRestrictionEnded, staffReasonSkipNoPriorState,
	}
	for _, reason := range plain {
		if got := staffReasonText(tr, reason, ""); got != "" {
			t.Errorf("staffReasonText(%q) = %q, want no text for a plain success", reason, got)
		}
	}
	for _, reason := range worded {
		if got := staffReasonText(tr, reason, "detail"); strings.TrimSpace(got) == "" {
			t.Errorf("staffReasonText(%q) is empty, a skipped or failed line would end in nothing", reason)
		}
	}
}

func TestStaffHistoryDetailUnlinked(t *testing.T) {
	env := newStaffActionEnv(t, 3)
	record := banRecordWithSkip(t, env)

	link, err := staff.GetLinkOfGroupFresh(env.groups[1])
	if err != nil || link == nil {
		t.Fatalf("link of Group B: %v, %v", link, err)
	}
	if deleted, err := staff.DeleteLink(link.ID); err != nil || !deleted {
		t.Fatalf("DeleteLink = %v, %v, want true", deleted, err)
	}

	text, _ := env.pressDetail(env.issuer, env.staffChatObj(), 6010, detailData(t, strconv.FormatUint(uint64(record.ID), 10), "0"))
	lineB := lineStartingWith(text, "✅ Group B")
	if lineB == "" || !strings.Contains(lineB, staffMarker("staff_history_unlinked")) {
		t.Errorf("Group B's line = %q, want the stored title and the no-longer-linked mark", lineB)
	}
	lineA := lineStartingWith(text, "✅ Group A")
	if lineA == "" || strings.Contains(lineA, staffMarker("staff_history_unlinked")) {
		t.Errorf("Group A's line = %q, want it present and not marked", lineA)
	}
}

func TestStaffHistoryDetailUndoOutcome(t *testing.T) {
	env := newStaffActionEnv(t, 3)
	record := banRecordWithSkip(t, env)
	groupA, groupB, groupC := env.groups[0], env.groups[1], env.groups[2]
	data := func(id int) string {
		return detailData(t, strconv.FormatUint(uint64(record.ID), 10), strconv.Itoa(id))
	}

	started := time.Date(2026, 10, 5, 14, 30, 0, 0, time.UTC)
	finished := started.Add(time.Minute)
	setUndo := func(finishedAt *time.Time) {
		t.Helper()
		err := db.DB.Model(&models.StaffAction{}).Where("id = ?", record.ID).Updates(map[string]any{
			"undo_started_at": started, "undo_finished_at": finishedAt, "undo_by_name": "Bob",
		}).Error
		if err != nil {
			t.Fatalf("mark undo: %v", err)
		}
	}
	setGroup := func(group int64, outcome, reason string) {
		t.Helper()
		err := db.DB.Model(&models.StaffActionGroup{}).
			Where("action_id = ? AND group_chat_id = ?", record.ID, group).
			Updates(map[string]any{"undo_outcome": outcome, "undo_reason": reason}).Error
		if err != nil {
			t.Fatalf("mark undo result of group %d: %v", group, err)
		}
	}

	setUndo(&finished)
	setGroup(groupA, models.StaffActionOutcomeDone, "undone_unbanned")
	setGroup(groupB, models.StaffActionOutcomeSkipped, "skip_changed_since")
	setGroup(groupC, models.StaffActionOutcomeSkipped, "skip_not_applied")

	text, _ := env.pressDetail(env.issuer, env.staffChatObj(), 6020, data(0))
	before, after, found := strings.Cut(text, staffMarker("staff_history_undone_by"))
	if !found {
		t.Fatalf("detail %q lacks the undone-by line", text)
	}
	wantInOrder(t, after, "Bob", "5 Oct 14:30")
	if !strings.Contains(before, "⏭ Group C") {
		t.Errorf("the original block %q must still show Group C's skip", before)
	}
	lineA := lineStartingWith(after, "✅ Group A")
	if !strings.Contains(lineA, staffMarker("staff_undo_unbanned")) {
		t.Errorf("Group A's undo line = %q, want the unbanned wording", lineA)
	}
	lineB := lineStartingWith(after, "⏭ Group B")
	if !strings.Contains(lineB, staffMarker("staff_undo_skip_changed_since")) {
		t.Errorf("Group B's undo line = %q, want the changed-since wording", lineB)
	}
	if strings.Contains(after, "Group C") {
		t.Errorf("Group C was never banned, so it must have no undo line: %q", after)
	}

	// An undo still running: no finish time, and Group B has no result yet.
	setUndo(nil)
	setGroup(groupB, "", "")
	text, _ = env.pressDetail(env.issuer, env.staffChatObj(), 6021, data(0))
	_, after, found = strings.Cut(text, staffMarker("staff_history_undone_by"))
	if !found || !strings.Contains(after, "⏳ Group B") {
		t.Errorf("a running undo shows %q after the header, want Group B as ⏳", after)
	}
}

func TestStaffHistoryDetailUnfinished(t *testing.T) {
	env := newStaffActionEnv(t, 1)
	action := models.StaffAction{
		StaffChatID: env.staffChat, IssuerUserID: env.issuer.Id, IssuerName: "Issuer",
		TargetUserID: 4242, Action: "ban", GroupCount: 1, SummaryChatID: env.staffChat, SummaryMsgID: 7001,
	}
	group := models.StaffActionGroup{GroupChatID: env.groups[0], GroupTitle: "Group A", Outcome: models.StaffActionOutcomePending}
	if err := staff.CreateAction(&action, []models.StaffActionGroup{group}); err != nil {
		t.Fatalf("CreateAction: %v", err)
	}
	data := detailData(t, strconv.FormatUint(uint64(action.ID), 10), "0")
	setUpdated := func(at time.Time) {
		t.Helper()
		if err := db.DB.Model(&models.StaffAction{}).Where("id = ?", action.ID).UpdateColumn("updated_at", at).Error; err != nil {
			t.Fatalf("set updated_at: %v", err)
		}
	}

	setUpdated(time.Now())
	text, _ := env.pressDetail(env.issuer, env.staffChatObj(), 6030, data)
	if line := lineStartingWith(text, "⏳ Group A"); line == "" {
		t.Errorf("a record updated just now shows %q, want Group A as ⏳", text)
	}

	setUpdated(time.Now().Add(-31 * time.Minute))
	text, _ = env.pressDetail(env.issuer, env.staffChatObj(), 6031, data)
	line := lineStartingWith(text, "❌ Group A")
	if line == "" || !strings.Contains(line, staffMarker("staff_act_fail_interrupted")) {
		t.Errorf("a record updated 31 minutes ago shows %q, want Group A as failed: interrupted", text)
	}

	rows, err := staff.ListActionGroupsFresh(action.ID)
	if err != nil || len(rows) != 1 || rows[0].Outcome != models.StaffActionOutcomePending {
		t.Errorf("rows after viewing = %+v (%v), want the row still pending: the view never writes", rows, err)
	}
}

func TestStaffHistoryDetailLong(t *testing.T) {
	tr := panelMarkerTranslator(t)
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	finished := now.Add(-time.Hour)
	hostile := strings.Repeat("&", 64)
	record := func(n int, outcome func(i int) string) (*models.StaffAction, []models.StaffActionGroup) {
		action := &models.StaffAction{
			ID: 1, Action: "ban", TargetUserID: 5, IssuerName: hostile, Reason: strings.Repeat("&", 5000),
			CreatedAt: now.Add(-2 * time.Hour), UpdatedAt: finished, FinishedAt: &finished, GroupCount: n,
		}
		groups := make([]models.StaffActionGroup, n)
		for i := range groups {
			groups[i] = models.StaffActionGroup{
				Seq: i, GroupChatID: int64(-1000 - i), GroupTitle: hostile,
				Outcome: outcome(i), Reason: "skip_not_in_group",
			}
			if groups[i].Outcome == models.StaffActionOutcomeDone {
				groups[i].Reason = "banned"
			}
			if groups[i].Outcome == models.StaffActionOutcomeFailed {
				groups[i].Reason = "fail_internal"
			}
		}
		return action, groups
	}

	t.Run("mixed outcomes list what fits and count the rest", func(t *testing.T) {
		mixed := func(i int) string {
			return []string{models.StaffActionOutcomeDone, models.StaffActionOutcomeSkipped, models.StaffActionOutcomeFailed}[i%3]
		}
		action, groups := record(150, mixed)
		text, _ := renderStaffHistoryDetail(tr, action, groups, nil, 0, now)
		if got := staffSummaryLen(text); got > staffPanelMaxUTF16 {
			t.Fatalf("detail text is %d UTF-16 units, want at most %d", got, staffPanelMaxUTF16)
		}
		last := text[strings.LastIndex(text, "\n")+1:]
		if !strings.HasPrefix(last, staffMarker("staff_history_more_groups")) {
			t.Fatalf("the text ends with %q, want the more-groups note", last)
		}
		var left int
		if _, err := fmt.Sscanf(strings.TrimPrefix(last, staffMarker("staff_history_more_groups")), "%d", &left); err != nil {
			t.Fatalf("the more-groups note %q carries no count: %v", last, err)
		}
		shown := 0
		for _, line := range strings.Split(text, "\n") {
			if strings.HasPrefix(line, "⏭ ") || strings.HasPrefix(line, "❌ ") {
				shown++
			}
		}
		if shown+left != 100 {
			t.Errorf("%d skipped and failed lines shown plus %d left out = %d, want all 100", shown, left, shown+left)
		}
		if !strings.Contains(text, staffMarker("staff_act_summary_done_collapsed")) {
			t.Errorf("the done lines were not collapsed into a count: %q", text)
		}
	})

	t.Run("all done collapse to one count and nothing is left out", func(t *testing.T) {
		action, groups := record(60, func(int) string { return models.StaffActionOutcomeDone })
		text, _ := renderStaffHistoryDetail(tr, action, groups, nil, 0, now)
		if got := staffSummaryLen(text); got > staffPanelMaxUTF16 {
			t.Fatalf("detail text is %d UTF-16 units, want at most %d", got, staffPanelMaxUTF16)
		}
		if strings.Contains(text, staffMarker("staff_history_more_groups")) {
			t.Errorf("a collapse that fits must not claim groups are hidden: %q", text)
		}
		if !strings.Contains(text, staffMarker("staff_act_summary_done_collapsed")+" 60") {
			t.Errorf("text %q lacks the collapsed count of 60", text)
		}
	})
}

func TestStaffHistoryDetailAccess(t *testing.T) {
	env := newStaffActionEnv(t, 1)
	mine := seedStaffActions(t, env.staffChat, 1, nil)
	other := uniqueModuleChatID()
	staffCleanup(t, other)
	theirs := seedStaffActions(t, other, 1, nil)
	mineID := strconv.FormatUint(uint64(mine[0].ID), 10)

	// refused presses a detail button and demands one answer with the wanted text and
	// alert flag, and no edit of the message.
	refused := func(t *testing.T, from gotgbot.User, msgID int64, data, wantKey string, wantAlert bool) {
		t.Helper()
		before := env.answerCount()
		env.tapData(from, env.staffChatObj(), msgID, data)
		if got := env.answerCount() - before; got != 1 {
			t.Errorf("the press was answered %d times, want exactly once", got)
		}
		text, alert := env.lastAnswer()
		if !strings.Contains(text, staffMarker(wantKey)) || alert != wantAlert {
			t.Errorf("answer = %q alert=%v, want %s alert=%v", text, alert, wantKey, wantAlert)
		}
		if edits := env.edits(env.staffChat, msgID); len(edits) != 0 {
			t.Errorf("a refused press edited the message %d time(s), want none", len(edits))
		}
	}

	t.Run("other Staff Group", func(t *testing.T) {
		refused(t, env.issuer, 6040, detailData(t, strconv.FormatUint(uint64(theirs[0].ID), 10), "0"), "staff_cb_denied", true)
	})
	t.Run("missing record", func(t *testing.T) {
		refused(t, env.issuer, 6041, detailData(t, "999999999", "0"), "staff_cb_expired", false)
	})
	t.Run("bad id", func(t *testing.T) {
		for i, id := range []string{"0", "-5", "abc", "", "18446744073709551615"} {
			refused(t, env.issuer, int64(6050+i), detailData(t, id, "0"), "staff_cb_expired", false)
		}
	})
	t.Run("bad offset", func(t *testing.T) {
		refused(t, env.issuer, 6060, detailData(t, mineID, "-1"), "staff_cb_expired", false)
	})
	t.Run("non-member", func(t *testing.T) {
		outsider := gotgbot.User{Id: env.issuer.Id + 100, FirstName: "Outsider"}
		env.fake.setMember(env.staffChat, outsider.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusLeft})
		refused(t, outsider, 6070, detailData(t, mineID, "0"), "staff_cb_members_only", true)
	})
	t.Run("a member answers once and sees the record", func(t *testing.T) {
		before := env.answerCount()
		env.tapData(env.issuer, env.staffChatObj(), 6080, detailData(t, mineID, "0"))
		if got := env.answerCount() - before; got != 1 {
			t.Errorf("the press was answered %d times, want exactly once", got)
		}
		if text := env.lastEditText(env.staffChat, 6080); !strings.Contains(text, staffMarker("staff_act_name_kick")) {
			t.Errorf("a member's press showed %q, want the record", text)
		}
	})
}

func TestStaffHistoryDetailCallbackBudget(t *testing.T) {
	for _, data := range []string{
		detailData(t, "18446744073709551615", "999999"),
		encodeCallbackData(staffCallbackNamespace, map[string]string{"a": "rc", "o": "999999"}),
	} {
		if n := len(data); n < 1 || n > 64 {
			t.Errorf("callback data %q is %d bytes, want 1 to 64", data, n)
		}
	}

	tr := panelMarkerTranslator(t)
	now := time.Now()
	finished := now
	actions := []models.StaffAction{{ID: ^uint(0), Action: "ban", TargetUserID: 1, CreatedAt: now, FinishedAt: &finished}}
	_, keyboard, _ := renderStaffHistory(tr, actions, nil, 999990, false, now)
	if keys := historyKeyboardOf(t, keyboard.InlineKeyboard); len(keys.detail) != 1 {
		t.Errorf("a record with the largest ID got %d detail buttons, want 1", len(keys.detail))
	}
	_, detailKeyboard := renderStaffHistoryDetail(tr, &actions[0], nil, nil, 999999, now)
	if keys := historyKeyboardOf(t, detailKeyboard.InlineKeyboard); keys.backList == nil {
		t.Error("the detail view at offset 999999 has no Back button")
	}
}

func TestStaffResultsFromRecord(t *testing.T) {
	rows := []models.StaffActionGroup{
		{Seq: 2, GroupChatID: -1003, GroupTitle: "Third", Outcome: models.StaffActionOutcomeFailed, Reason: "fail_telegram", Detail: "Bad &amp; sad"},
		{Seq: 0, GroupChatID: -1001, GroupTitle: "First", Outcome: models.StaffActionOutcomeDone, Reason: "banned"},
		{Seq: 3, GroupChatID: -1004, GroupTitle: "Fourth", Outcome: models.StaffActionOutcomePending},
		{Seq: 1, GroupChatID: -1002, GroupTitle: "Second", Outcome: models.StaffActionOutcomeSkipped, Reason: "skip_not_in_group"},
	}
	got := staffResultsFromRecord(rows)
	want := []staffGroupResult{
		{Link: models.StaffGroupLink{GroupChatID: -1001, GroupTitle: "First"}, Outcome: staffOutcomeDone, Reason: staffReasonBanned},
		{Link: models.StaffGroupLink{GroupChatID: -1002, GroupTitle: "Second"}, Outcome: staffOutcomeSkipped, Reason: staffReasonSkipNotInGroup},
		{Link: models.StaffGroupLink{GroupChatID: -1003, GroupTitle: "Third"}, Outcome: staffOutcomeFailed, Reason: staffReasonFailTelegram, Detail: "Bad &amp; sad"},
		{Link: models.StaffGroupLink{GroupChatID: -1004, GroupTitle: "Fourth"}, Outcome: staffOutcomePending},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d results, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("result %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}
