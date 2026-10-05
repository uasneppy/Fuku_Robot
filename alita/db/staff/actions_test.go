//go:build testtools

package staff

import (
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/models"
)

// cleanupActionRows deletes the audit rows of the given Staff Group chat IDs when
// the test ends, children first.
func cleanupActionRows(t *testing.T, staffChatIDs ...int64) {
	t.Helper()
	t.Cleanup(func() {
		db.DB.Where("action_id IN (?)",
			db.DB.Model(&models.StaffAction{}).Select("id").Where("staff_chat_id IN ?", staffChatIDs)).
			Delete(&models.StaffActionGroup{})
		db.DB.Where("staff_chat_id IN ?", staffChatIDs).Delete(&models.StaffAction{})
	})
}

// newTestAction is a ban of a target by an issuer, linked to the given groups in
// the order given.
func newTestAction(staffChatID int64, seqs []int, groupChatIDs []int64) (*models.StaffAction, []models.StaffActionGroup) {
	action := &models.StaffAction{
		StaffChatID:    staffChatID,
		IssuerUserID:   11,
		IssuerName:     "Issuer <b>",
		TargetUserID:   22,
		TargetName:     "Target",
		Action:         "ban",
		Reason:         "spamming",
		DurationSec:    172800,
		DurationAmount: 2,
		DurationUnit:   "d",
		UntilDate:      1900000000,
		GroupCount:     len(groupChatIDs),
		SummaryChatID:  staffChatID,
		SummaryMsgID:   77,
	}
	groups := make([]models.StaffActionGroup, len(groupChatIDs))
	for i, id := range groupChatIDs {
		groups[i] = models.StaffActionGroup{Seq: seqs[i], GroupChatID: id, GroupTitle: "Title", Outcome: models.StaffActionOutcomePending}
	}
	return action, groups
}

func TestStaffActionRecordCreate(t *testing.T) {
	staffChat := uniqueStaffChatID()
	cleanupActionRows(t, staffChat)
	groupA, groupB, groupC := uniqueStaffChatID(), uniqueStaffChatID(), uniqueStaffChatID()

	// Inserted out of order on purpose: reads must come back by seq.
	action, groups := newTestAction(staffChat, []int{2, 0, 1}, []int64{groupC, groupA, groupB})
	if err := CreateAction(action, groups); err != nil {
		t.Fatalf("CreateAction: %v", err)
	}
	if action.ID == 0 {
		t.Fatal("CreateAction left action.ID empty")
	}
	for i, group := range groups {
		if group.ID == 0 || group.ActionID != action.ID {
			t.Fatalf("group %d = id %d action %d, want a filled id and action %d", i, group.ID, group.ActionID, action.ID)
		}
	}

	got, err := GetActionFresh(action.ID)
	if err != nil || got == nil {
		t.Fatalf("GetActionFresh = %v, %v", got, err)
	}
	if got.FinishedAt != nil || got.UndoStartedAt != nil || got.UndoBy != nil {
		t.Fatalf("a new record has finished_at=%v undo_started_at=%v undo_by=%v, want all NULL", got.FinishedAt, got.UndoStartedAt, got.UndoBy)
	}
	rows, err := ListActionGroupsFresh(action.ID)
	if err != nil {
		t.Fatalf("ListActionGroupsFresh: %v", err)
	}
	wantOrder := []int64{groupA, groupB, groupC}
	if len(rows) != 3 {
		t.Fatalf("group rows = %d, want 3", len(rows))
	}
	for i, row := range rows {
		if row.Seq != i || row.GroupChatID != wantOrder[i] {
			t.Fatalf("row %d = seq %d group %d, want seq %d group %d", i, row.Seq, row.GroupChatID, i, wantOrder[i])
		}
		if row.Outcome != models.StaffActionOutcomePending || row.PriorStatus != "" || row.AppliedAt != nil || row.UndoOutcome != "" {
			t.Fatalf("new row %d = %q prior %q applied %v undo %q, want pending with nothing else set",
				i, row.Outcome, row.PriorStatus, row.AppliedAt, row.UndoOutcome)
		}
	}

	if missing, err := GetActionFresh(action.ID + 100000); err != nil || missing != nil {
		t.Fatalf("GetActionFresh(missing) = %v, %v, want nil, nil", missing, err)
	}
}

func TestStaffActionRecordRoundTrip(t *testing.T) {
	staffChat := uniqueStaffChatID()
	cleanupActionRows(t, staffChat)
	groupA, groupB := uniqueStaffChatID(), uniqueStaffChatID()
	action, groups := newTestAction(staffChat, []int{0, 1}, []int64{groupA, groupB})
	if err := CreateAction(action, groups); err != nil {
		t.Fatalf("CreateAction: %v", err)
	}

	// Every parent column survives a fresh read.
	parent, err := GetActionFresh(action.ID)
	if err != nil || parent == nil {
		t.Fatalf("GetActionFresh = %v, %v", parent, err)
	}
	if parent.StaffChatID != staffChat || parent.IssuerUserID != 11 || parent.IssuerName != "Issuer <b>" ||
		parent.TargetUserID != 22 || parent.TargetName != "Target" || parent.Action != "ban" ||
		parent.Reason != "spamming" || parent.DurationSec != 172800 || parent.DurationAmount != 2 ||
		parent.DurationUnit != "d" || parent.OverLimit || parent.UntilDate != 1900000000 ||
		parent.GroupCount != 2 || parent.SummaryChatID != staffChat || parent.SummaryMsgID != 77 {
		t.Fatalf("parent did not round-trip: %+v", parent)
	}

	// SavePrior writes every prior column, zero values included.
	prior := ActionPrior{Status: "restricted", IsMember: true, Until: 12345, Permissions: `{"can_send_polls":true}`}
	if err := SavePrior(action.ID, groupA, prior); err != nil {
		t.Fatalf("SavePrior: %v", err)
	}
	rows, err := ListActionGroupsFresh(action.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := rows[0]; got.PriorStatus != prior.Status || !got.PriorIsMember || got.PriorUntil != prior.Until || got.PriorPermissions != prior.Permissions {
		t.Fatalf("prior did not round-trip: %+v", got)
	}
	if rows[1].PriorStatus != "" {
		t.Fatalf("SavePrior of group A touched group B: %+v", rows[1])
	}
	if err := SavePrior(action.ID, groupA, ActionPrior{Status: "member"}); err != nil {
		t.Fatalf("SavePrior(zero values): %v", err)
	}
	rows, _ = ListActionGroupsFresh(action.ID)
	if got := rows[0]; got.PriorStatus != "member" || got.PriorIsMember || got.PriorUntil != 0 || got.PriorPermissions != "" {
		t.Fatalf("SavePrior skipped zero values: %+v", got)
	}

	// A group with no row is an error that says so: a prior state that was not
	// stored must never look stored.
	err = SavePrior(action.ID, uniqueStaffChatID(), prior)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("SavePrior(missing row) = %v, want an error wrapping gorm.ErrRecordNotFound", err)
	}

	// SaveGroupResult sets applied_at only for done and moves the parent's
	// heartbeat.
	before := parent.UpdatedAt
	time.Sleep(5 * time.Millisecond)
	if err := SaveGroupResult(action.ID, ActionGroupResult{GroupChatID: groupA, Outcome: models.StaffActionOutcomeDone, Reason: "banned"}); err != nil {
		t.Fatalf("SaveGroupResult(done): %v", err)
	}
	if err := SaveGroupResult(action.ID, ActionGroupResult{GroupChatID: groupB, Outcome: models.StaffActionOutcomeSkipped, Reason: "skip_not_in_group", Detail: "x"}); err != nil {
		t.Fatalf("SaveGroupResult(skipped): %v", err)
	}
	rows, _ = ListActionGroupsFresh(action.ID)
	if rows[0].Outcome != models.StaffActionOutcomeDone || rows[0].Reason != "banned" || rows[0].AppliedAt == nil {
		t.Fatalf("done row = %+v, want done banned with applied_at", rows[0])
	}
	firstApplied := *rows[0].AppliedAt
	if rows[1].Outcome != models.StaffActionOutcomeSkipped || rows[1].Detail != "x" || rows[1].AppliedAt != nil {
		t.Fatalf("skipped row = %+v, want skipped with detail and no applied_at", rows[1])
	}
	advanced, _ := GetActionFresh(action.ID)
	if !advanced.UpdatedAt.After(before) {
		t.Fatalf("parent updated_at %v did not advance past %v", advanced.UpdatedAt, before)
	}

	// FinalizeAction writes every result and finished_at once; a second call keeps
	// the first finish time and the first applied_at.
	results := []ActionGroupResult{
		{GroupChatID: groupA, Outcome: models.StaffActionOutcomeDone, Reason: "banned"},
		{GroupChatID: groupB, Outcome: models.StaffActionOutcomeFailed, Reason: "fail_interrupted"},
	}
	if err := FinalizeAction(action.ID, results); err != nil {
		t.Fatalf("FinalizeAction: %v", err)
	}
	finished, _ := GetActionFresh(action.ID)
	if finished.FinishedAt == nil {
		t.Fatal("finished_at is nil after FinalizeAction")
	}
	first := *finished.FinishedAt
	time.Sleep(5 * time.Millisecond)
	if err := FinalizeAction(action.ID, results); err != nil {
		t.Fatalf("second FinalizeAction: %v", err)
	}
	again, _ := GetActionFresh(action.ID)
	if again.FinishedAt == nil || !again.FinishedAt.Equal(first) {
		t.Fatalf("finished_at = %v after the second finalize, want the first time %v", again.FinishedAt, first)
	}
	rows, _ = ListActionGroupsFresh(action.ID)
	if rows[0].AppliedAt == nil || !rows[0].AppliedAt.Equal(firstApplied) {
		t.Fatalf("applied_at = %v after finalize, want the first time %v", rows[0].AppliedAt, firstApplied)
	}
	if rows[1].Outcome != models.StaffActionOutcomeFailed || rows[1].Reason != "fail_interrupted" {
		t.Fatalf("group B = %q %q after finalize, want failed fail_interrupted", rows[1].Outcome, rows[1].Reason)
	}
}
