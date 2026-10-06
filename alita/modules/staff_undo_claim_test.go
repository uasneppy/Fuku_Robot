//go:build testtools

package modules

import (
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
	env.runUndo(carol, msgID, action.ID)

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
