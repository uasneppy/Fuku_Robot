//go:build testtools

package modules

import (
	"fmt"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db/models"
)

// wantUndoClaimGivenBack fails unless the record is exactly as it was before any
// undo claimed it: no claimer, no start or finish time, and no group row with an
// undo outcome, reason or detail.
func wantUndoClaimGivenBack(t *testing.T, msgID int64) {
	t.Helper()
	record, rows := recordOfCard(t, msgID)
	if record.UndoBy != nil || record.UndoByName != "" || record.UndoStartedAt != nil || record.UndoFinishedAt != nil {
		t.Fatalf("record after a given-back undo = undo_by %v name %q started %v finished %v, want all empty",
			record.UndoBy, record.UndoByName, record.UndoStartedAt, record.UndoFinishedAt)
	}
	for _, row := range rows {
		if row.UndoOutcome != "" || row.UndoReason != "" || row.UndoDetail != "" {
			t.Fatalf("group %d undo = %q / %q / %q, want every undo column empty",
				row.GroupChatID, row.UndoOutcome, row.UndoReason, row.UndoDetail)
		}
	}
}

func TestStaffUndoClaimReleased(t *testing.T) {
	env := newStaffActionEnv(t, 2)
	bob := env.undoPresser("Bob")
	action, msgID := env.undoFinishedBan()
	// Bob is only a plain member of every linked group when he presses Undo, so no
	// group can be changed by him.
	for _, group := range env.groups {
		env.fake.setMember(group, bob.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
	}
	writesBefore := make([]int, len(env.groups))
	for i, group := range env.groups {
		writesBefore[i] = len(env.writes(group))
	}
	originalEditsBefore := len(env.edits(env.staffChat, msgID))

	cardMsgID := env.runUndo(bob, msgID, action.ID)

	for i, group := range env.groups {
		if got := len(env.writes(group)); got != writesBefore[i] {
			t.Fatalf("group %d got %d write call(s) from a presser who is not its admin", group, got-writesBefore[i])
		}
	}
	summary := env.lastEditText(env.staffChat, cardMsgID)
	for _, title := range []string{"Group A", "Group B"} {
		wantSkippedLine(t, summary, title, "staff_act_skip_issuer_not_admin")
	}
	if !strings.Contains(summary, "✅ 0 · ⏭ 2 · ❌ 0") || strings.Contains(summary, "⏳") {
		t.Fatalf("a no-effect undo has no final tally:\n%s", summary)
	}
	if !strings.Contains(summary, staffMarker("staff_undo_released_note")) {
		t.Fatalf("a given-back undo does not say the action can still be undone:\n%s", summary)
	}
	wantUndoClaimGivenBack(t, msgID)
	if got := len(env.edits(env.staffChat, msgID)); got != originalEditsBefore {
		t.Fatalf("the original summary was edited %d time(s) by an undo that changed nothing", got-originalEditsBefore)
	}
	undoButtonData(t, env, msgID, action.ID)
	if holder := targetLockHolder(t, staffTestTarget); holder != "" {
		t.Fatalf("target lock holder after the undo = %q, want the key gone", holder)
	}

	// The action was not used up: a member with the restrict right undoes it.
	carol := env.undoMember("Carol", 2)
	carolMsgID := env.runUndo(carol, msgID, action.ID)

	if text := env.lastEditText(env.staffChat, carolMsgID); strings.Contains(text, staffMarker("staff_undo_released_note")) {
		t.Fatalf("an undo that changed both groups says it was given back:\n%s", text)
	}
	for _, group := range env.groups {
		wantMember(t, env.fake, group, staffTestTarget, gotgbot.ChatMemberStatusLeft)
	}
	done, rows := recordOfCard(t, msgID)
	if done.UndoBy == nil || *done.UndoBy != carol.Id || done.UndoStartedAt == nil || done.UndoFinishedAt == nil {
		t.Fatalf("record after Carol's undo = undo_by %v started %v finished %v, want Carol %d with both times set",
			done.UndoBy, done.UndoStartedAt, done.UndoFinishedAt, carol.Id)
	}
	for _, row := range rows {
		if row.UndoOutcome != models.StaffActionOutcomeDone {
			t.Fatalf("group %d undo outcome = %q after Carol's undo, want done", row.GroupChatID, row.UndoOutcome)
		}
	}
	original := env.lastEditText(env.staffChat, msgID)
	if !strings.Contains(original, staffMarker("staff_undo_marker")) || !strings.Contains(original, "Carol") {
		t.Fatalf("the original summary was not marked as undone by Carol:\n%s", original)
	}
	edits := env.edits(env.staffChat, msgID)
	wantNoKeyboard(t, edits[len(edits)-1])
}

func TestStaffUndoNothingChanged(t *testing.T) {
	env := newStaffActionEnv(t, 2)
	bob := env.undoPresser("Bob")
	action, msgID := env.undoFinishedBan()
	// Every unban is tried and refused: the write was reached, so the undo is spent.
	for _, group := range env.groups {
		env.fake.script("unbanChatMember", group, staffFakeError(400, "Bad Request: CHAT_ADMIN_REQUIRED"))
	}

	cardMsgID := env.runUndo(bob, msgID, action.ID)

	for _, group := range env.groups {
		if got := len(env.callsTo("unbanChatMember", group)); got != 1 {
			t.Fatalf("group %d unban calls = %d, want exactly one", group, got)
		}
	}
	summary := env.lastEditText(env.staffChat, cardMsgID)
	if !strings.Contains(summary, "✅ 0 · ⏭ 0 · ❌ 2") || strings.Contains(summary, "⏳") {
		t.Fatalf("an undo whose writes all failed has no final tally of two failures:\n%s", summary)
	}
	if strings.Contains(summary, staffMarker("staff_undo_released_note")) {
		t.Fatalf("an undo that tried both groups says it was given back:\n%s", summary)
	}
	done, rows := recordOfCard(t, msgID)
	if done.UndoBy == nil || *done.UndoBy != bob.Id || done.UndoStartedAt == nil || done.UndoFinishedAt == nil {
		t.Fatalf("record = undo_by %v started %v finished %v, want Bob %d with both times set",
			done.UndoBy, done.UndoStartedAt, done.UndoFinishedAt, bob.Id)
	}
	for _, row := range rows {
		if row.UndoOutcome != "failed" {
			t.Fatalf("group %d undo outcome = %q, want failed", row.GroupChatID, row.UndoOutcome)
		}
	}
	original := env.lastEditText(env.staffChat, msgID)
	if !strings.Contains(original, staffMarker("staff_undo_marker_nothing")) || !strings.Contains(original, "Bob") ||
		strings.Contains(original, staffMarker("staff_undo_marker")) {
		t.Fatalf("the original summary does not say Bob's undo changed nothing:\n%s", original)
	}
	edits := env.edits(env.staffChat, msgID)
	wantNoKeyboard(t, edits[len(edits)-1])

	// The undo was tried, so it is not available to anyone else.
	carol := env.undoMember("Carol", 2)
	sentBefore := len(env.fake.sentTo(env.staffChat))
	unbansBefore := env.unbansIn()
	env.tapData(carol, env.staffChatObj(), msgID,
		encodeCallbackData(staffCallbackNamespace, map[string]string{"a": undoAskCode, "r": fmt.Sprint(action.ID)}))
	if text, alert := env.lastAnswer(); !strings.Contains(text, "Bob") || !alert {
		t.Fatalf("answer to Undo after a tried undo = %q alert=%v, want an alert naming Bob", text, alert)
	}
	if got := len(env.fake.sentTo(env.staffChat)); got != sentBefore {
		t.Fatalf("Undo after a tried undo posted %d message(s), want none", got-sentBefore)
	}
	if got := env.unbansIn(); got != unbansBefore {
		t.Fatalf("Undo after a tried undo made %d more unban call(s), want none", got-unbansBefore)
	}
}

func TestStaffUndoPanicKeepsClaim(t *testing.T) {
	env := newStaffActionEnv(t, 2)
	bob := env.undoPresser("Bob")
	groupA, groupB := env.groups[0], env.groups[1]
	// Bob can change group A only. Its worker panics inside the write call, so the
	// run never learns whether the write happened.
	env.fake.setMember(groupB, bob.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
	action, msgID := env.undoFinishedBan()
	env.fake.setPanic("unbanChatMember", groupA)

	cardMsgID := env.runUndo(bob, msgID, action.ID)

	summary := env.lastEditText(env.staffChat, cardMsgID)
	if line := summaryLine(t, summary, "Group A"); !strings.Contains(line, staffMarker("staff_act_line_failed")) ||
		!strings.Contains(line, staffMarker("staff_act_fail_internal")) {
		t.Fatalf("line for Group A = %q, want failed with staff_act_fail_internal", line)
	}
	wantSkippedLine(t, summary, "Group B", "staff_act_skip_issuer_not_admin")
	if strings.Contains(summary, "⏳") {
		t.Fatalf("the undo summary still has a pending line:\n%s", summary)
	}
	// A group lost to a panic may have reached its write, so the claim stays (D-09).
	done, _ := recordOfCard(t, msgID)
	if done.UndoBy == nil || *done.UndoBy != bob.Id || done.UndoStartedAt == nil || done.UndoFinishedAt == nil {
		t.Fatalf("record = undo_by %v started %v finished %v, want Bob %d with both times set",
			done.UndoBy, done.UndoStartedAt, done.UndoFinishedAt, bob.Id)
	}
	original := env.lastEditText(env.staffChat, msgID)
	if !strings.Contains(original, staffMarker("staff_undo_marker_nothing")) {
		t.Fatalf("the original summary does not say the undo changed nothing:\n%s", original)
	}
}
