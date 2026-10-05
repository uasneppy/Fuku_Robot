//go:build testtools

package modules

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/db/staff"
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
	fields           map[string]map[string]string
	total            int
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
