//go:build testtools

package modules

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/db/staff"
)

// detailKeyboardOf presses the detail view of record on message msgID as from and
// returns the text and the raw keyboard rows of the edit it caused, without
// classifying the buttons (historyKeyboardOf refuses an Undo button).
func (e *staffActionEnv) detailKeyboardOf(from gotgbot.User, msgID int64, recordID uint) (string, [][]gotgbot.InlineKeyboardButton) {
	e.t.Helper()
	e.tapData(from, e.staffChatObj(), msgID, detailData(e.t, strconv.FormatUint(uint64(recordID), 10), "0"))
	edits := e.edits(e.staffChat, msgID)
	if len(edits) == 0 {
		e.t.Fatalf("pressing the detail of record %d edited nothing", recordID)
	}
	last := edits[len(edits)-1]
	return fmt.Sprint(last.Params["text"]), staffKeyboardOf(last.Params["reply_markup"])
}

// wantNoUndoButton fails when any button of the keyboard asks for an undo.
func wantNoUndoButton(t *testing.T, keyboard [][]gotgbot.InlineKeyboardButton) {
	t.Helper()
	for _, button := range undoButtons(keyboard) {
		decoded, ok := decodeCallbackData(button.CallbackData, staffCallbackNamespace)
		if ok && decoded.Fields["a"] == undoAskCode {
			t.Fatalf("the detail view offers Undo (%q), want none", button.Text)
		}
	}
}

func TestStaffHistoryDetailUndo(t *testing.T) {
	env := newStaffActionEnv(t, 2)
	bob := env.undoPresser("Bob")

	_, msgID := env.startRun("/ban 4242 spamming")
	env.waitRuns()
	record, _ := recordOfCard(t, msgID)

	// The list shows the action, not yet undone.
	listText, _ := env.pressHistory(env.issuer, env.staffChatObj(), "0")
	if strings.Contains(listText, staffMarker("staff_history_undone")) {
		t.Fatalf("the list marks a fresh action as undone:\n%s", listText)
	}

	_, keyboard := env.detailKeyboardOf(env.issuer, 6100, record.ID)
	if len(keyboard) != 2 {
		t.Fatalf("detail keyboard has %d rows, want the Undo row above the Back row: %+v", len(keyboard), keyboard)
	}
	undoRow, backRow := keyboard[0], keyboard[1]
	if len(undoRow) != 1 || undoRow[0].Text != staffMarker("staff_undo_button") {
		t.Fatalf("first row = %+v, want one button labelled %q", undoRow, staffMarker("staff_undo_button"))
	}
	decoded, ok := decodeCallbackData(undoRow[0].CallbackData, staffCallbackNamespace)
	if !ok || decoded.Fields["a"] != undoAskCode || decoded.Fields["r"] != fmt.Sprint(record.ID) || len(decoded.Fields) != 2 {
		t.Fatalf("the detail Undo button carries %v ok=%v, want only a=%s r=%d", decoded.Fields, ok, undoAskCode, record.ID)
	}
	if len(backRow) != 1 || !strings.Contains(backRow[0].Text, staffMarker("staff_history_back_list")) {
		t.Fatalf("second row = %+v, want the Back button", backRow)
	}

	// Pressing it runs the same flow as the summary's button: a confirm card that
	// replies to the original summary, and nothing undone until Confirm.
	sentBefore := len(env.fake.sentTo(env.staffChat))
	env.tapData(bob, env.staffChatObj(), 6100, undoRow[0].CallbackData)
	sent := env.fake.sentTo(env.staffChat)
	if len(sent) != sentBefore+1 {
		t.Fatalf("messages after Undo = %d, want one confirm card", len(sent)-sentBefore)
	}
	reply, ok := sent[len(sent)-1].Params["reply_parameters"].(*gotgbot.ReplyParameters)
	if !ok || reply == nil || reply.MessageId != msgID {
		t.Fatalf("undo card reply_parameters = %+v, want a reply to the original summary %d", sent[len(sent)-1].Params["reply_parameters"], msgID)
	}
	for _, group := range env.groups {
		if got := len(env.callsTo("unbanChatMember", group)); got != 0 {
			t.Fatalf("group %d got %d unban calls before Confirm, want none", group, got)
		}
	}

	// Only the presser confirms.
	token, cardMsgID := undoCard(t, env)
	env.tapUndoCard(env.issuer, undoConfirmCode, token, cardMsgID)
	for _, group := range env.groups {
		if got := len(env.callsTo("unbanChatMember", group)); got != 0 {
			t.Fatalf("group %d was unbanned by someone who did not press Undo", group)
		}
	}
	env.tapUndoCard(bob, undoConfirmCode, token, cardMsgID)
	env.waitRuns()

	// Reopened, the detail view shows the undo and offers none.
	text, keyboard := env.detailKeyboardOf(env.issuer, 6101, record.ID)
	wantNoUndoButton(t, keyboard)
	wantInOrder(t, text, staffMarker("staff_history_undone_by"), "Bob")
	if got := strings.Count(text, staffMarker("staff_undo_unbanned")); got != 2 {
		t.Errorf("detail shows %d unbanned lines, want 2:\n%s", got, text)
	}

	listText, _ = env.pressHistory(env.issuer, env.staffChatObj(), "0")
	if !strings.Contains(listText, staffMarker("staff_history_undone")) {
		t.Errorf("the list line lacks the undone mark:\n%s", listText)
	}
}

func TestStaffHistoryDetailUndoRules(t *testing.T) {
	t.Run("a kick has no Undo", func(t *testing.T) {
		env := newStaffActionEnv(t, 2)
		_, msgID := env.startRun("/kick 4242")
		env.waitRuns()
		record, _ := recordOfCard(t, msgID)
		_, keyboard := env.detailKeyboardOf(env.issuer, 6200, record.ID)
		wantNoUndoButton(t, keyboard)
	})

	t.Run("an unfinished record has no Undo", func(t *testing.T) {
		env := newStaffActionEnv(t, 1)
		action := models.StaffAction{
			StaffChatID: env.staffChat, IssuerUserID: env.issuer.Id, IssuerName: "Issuer",
			TargetUserID: 4242, Action: "ban", GroupCount: 1, SummaryChatID: env.staffChat, SummaryMsgID: 7101,
		}
		group := models.StaffActionGroup{GroupChatID: env.groups[0], GroupTitle: "Group A", Outcome: models.StaffActionOutcomeDone}
		if err := staff.CreateAction(&action, []models.StaffActionGroup{group}); err != nil {
			t.Fatalf("CreateAction: %v", err)
		}
		_, keyboard := env.detailKeyboardOf(env.issuer, 6201, action.ID)
		wantNoUndoButton(t, keyboard)
	})

	t.Run("a record applied nowhere has no Undo", func(t *testing.T) {
		env := newStaffActionEnv(t, 2)
		for _, group := range env.groups {
			env.fake.setMember(group, env.issuer.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
		}
		_, msgID := env.startRun("/ban 4242")
		env.waitRuns()
		record, _ := recordOfCard(t, msgID)
		_, keyboard := env.detailKeyboardOf(env.issuer, 6202, record.ID)
		wantNoUndoButton(t, keyboard)
	})

	t.Run("an undone record has no Undo", func(t *testing.T) {
		env := newStaffActionEnv(t, 2)
		bob := env.undoPresser("Bob")
		_, msgID := env.startRun("/ban 4242")
		env.waitRuns()
		record, _ := recordOfCard(t, msgID)
		env.runUndo(bob, msgID, record.ID)
		_, keyboard := env.detailKeyboardOf(env.issuer, 6203, record.ID)
		wantNoUndoButton(t, keyboard)
	})
}
