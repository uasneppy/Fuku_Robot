//go:build testtools

package staff

import (
	"errors"
	"sync"
	"sync/atomic"
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

func TestStaffActionListFresh(t *testing.T) {
	chat, other := uniqueStaffChatID(), uniqueStaffChatID()
	cleanupActionRows(t, chat, other)

	var ids []uint
	for i := 0; i < 12; i++ {
		action, groups := newTestAction(chat, []int{0}, []int64{uniqueStaffChatID()})
		if err := CreateAction(action, groups); err != nil {
			t.Fatalf("CreateAction %d: %v", i, err)
		}
		ids = append(ids, action.ID)
	}
	for i := 0; i < 2; i++ {
		action, groups := newTestAction(other, []int{0}, []int64{uniqueStaffChatID()})
		if err := CreateAction(action, groups); err != nil {
			t.Fatalf("CreateAction (other chat) %d: %v", i, err)
		}
	}

	page, err := ListActionsFresh(chat, 0, 11)
	if err != nil {
		t.Fatalf("ListActionsFresh(0, 11): %v", err)
	}
	if len(page) != 11 {
		t.Fatalf("first page has %d rows, want 11", len(page))
	}
	for i, row := range page {
		if row.StaffChatID != chat {
			t.Fatalf("row %d belongs to chat %d, want %d", i, row.StaffChatID, chat)
		}
		if want := ids[len(ids)-1-i]; row.ID != want {
			t.Fatalf("row %d has id %d, want %d (newest first)", i, row.ID, want)
		}
	}

	rest, err := ListActionsFresh(chat, 10, 11)
	if err != nil {
		t.Fatalf("ListActionsFresh(10, 11): %v", err)
	}
	if len(rest) != 2 || rest[0].ID != ids[1] || rest[1].ID != ids[0] {
		t.Fatalf("second page = %+v, want the 2 oldest rows %d and %d", rest, ids[1], ids[0])
	}
}

func TestStaffActionTally(t *testing.T) {
	chat := uniqueStaffChatID()
	cleanupActionRows(t, chat)
	groups := []int64{uniqueStaffChatID(), uniqueStaffChatID(), uniqueStaffChatID(), uniqueStaffChatID(), uniqueStaffChatID()}

	action, rows := newTestAction(chat, []int{0, 1, 2, 3, 4}, groups)
	if err := CreateAction(action, rows); err != nil {
		t.Fatalf("CreateAction: %v", err)
	}
	outcomes := []string{
		models.StaffActionOutcomeDone, models.StaffActionOutcomeDone,
		models.StaffActionOutcomeSkipped, models.StaffActionOutcomeFailed,
	}
	for i, outcome := range outcomes {
		if err := SaveGroupResult(action.ID, ActionGroupResult{GroupChatID: groups[i], Outcome: outcome}); err != nil {
			t.Fatalf("SaveGroupResult %d: %v", i, err)
		}
	}

	// An undo changed group 0 and skipped group 1: only the first counts as undone.
	for i, outcome := range []string{models.StaffActionOutcomeDone, models.StaffActionOutcomeSkipped} {
		if err := SaveUndoResult(action.ID, ActionGroupResult{GroupChatID: groups[i], Outcome: outcome}); err != nil {
			t.Fatalf("SaveUndoResult %d: %v", i, err)
		}
	}

	empty, noGroups := newTestAction(chat, nil, nil)
	if err := CreateAction(empty, noGroups); err != nil {
		t.Fatalf("CreateAction (no groups): %v", err)
	}

	tallies, err := TallyActionGroups([]uint{action.ID, empty.ID})
	if err != nil {
		t.Fatalf("TallyActionGroups: %v", err)
	}
	if got, want := tallies[action.ID], (ActionTally{Done: 2, Skipped: 1, Failed: 1, Pending: 1, UndoDone: 1}); got != want {
		t.Fatalf("tally = %+v, want %+v", got, want)
	}
	if _, present := tallies[empty.ID]; present {
		t.Fatalf("an action with no groups must be absent from the tally map, got %+v", tallies[empty.ID])
	}

	none, err := TallyActionGroups(nil)
	if err != nil || len(none) != 0 {
		t.Fatalf("TallyActionGroups(nil) = %v, %v, want an empty map and no error", none, err)
	}
}

func TestStaffActionClaimUndo(t *testing.T) {
	chat := uniqueStaffChatID()
	cleanupActionRows(t, chat)
	action, rows := newTestAction(chat, []int{0}, []int64{uniqueStaffChatID()})
	if err := CreateAction(action, rows); err != nil {
		t.Fatalf("CreateAction: %v", err)
	}

	claimedAt, claimed, err := ClaimUndo(action.ID, 501, "First <b>")
	if err != nil || !claimed || claimedAt.IsZero() {
		t.Fatalf("first ClaimUndo = %v, %v, %v, want a claim time and true", claimedAt, claimed, err)
	}
	_, claimed, err = ClaimUndo(action.ID, 502, "Second")
	if err != nil || claimed {
		t.Fatalf("second ClaimUndo = %v, %v, want false and no error", claimed, err)
	}
	got, err := GetActionFresh(action.ID)
	if err != nil || got == nil {
		t.Fatalf("GetActionFresh = %v, %v", got, err)
	}
	if got.UndoBy == nil || *got.UndoBy != 501 || got.UndoByName != "First <b>" || got.UndoStartedAt == nil {
		t.Fatalf("record after two claims = undo_by %v name %q started %v, want the first claimer's 501 and name",
			got.UndoBy, got.UndoByName, got.UndoStartedAt)
	}
	if !got.UndoStartedAt.Equal(claimedAt) {
		t.Fatalf("stored undo_started_at = %v, want exactly the claim time %v ClaimUndo returned", got.UndoStartedAt, claimedAt)
	}

	if _, claimed, err := ClaimUndo(action.ID+100000, 503, "Nobody"); err != nil || claimed {
		t.Fatalf("ClaimUndo of a missing record = %v, %v, want false and no error", claimed, err)
	}

	// Eight claimers at once on a fresh record: exactly one wins.
	racy, racyRows := newTestAction(chat, []int{0}, []int64{uniqueStaffChatID()})
	if err := CreateAction(racy, racyRows); err != nil {
		t.Fatalf("CreateAction: %v", err)
	}
	var (
		wg      sync.WaitGroup
		wins    atomic.Int32
		failure atomic.Value
	)
	start := make(chan struct{})
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, claimed, err := ClaimUndo(racy.ID, int64(600+i), "Racer")
			if err != nil {
				failure.Store(err)
				return
			}
			if claimed {
				wins.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()
	if err, _ := failure.Load().(error); err != nil {
		t.Fatalf("concurrent ClaimUndo: %v", err)
	}
	if wins.Load() != 1 {
		t.Fatalf("%d of 8 concurrent claims won, want exactly 1", wins.Load())
	}
}

func TestStaffActionFinalizeUndo(t *testing.T) {
	chat := uniqueStaffChatID()
	cleanupActionRows(t, chat)
	groups := []int64{uniqueStaffChatID(), uniqueStaffChatID(), uniqueStaffChatID()}
	action, rows := newTestAction(chat, []int{0, 1, 2}, groups)
	if err := CreateAction(action, rows); err != nil {
		t.Fatalf("CreateAction: %v", err)
	}
	// Groups 0 and 1 were applied; group 2 was skipped by the original.
	if err := FinalizeAction(action.ID, []ActionGroupResult{
		{GroupChatID: groups[0], Outcome: models.StaffActionOutcomeDone, Reason: "banned"},
		{GroupChatID: groups[1], Outcome: models.StaffActionOutcomeDone, Reason: "banned"},
		{GroupChatID: groups[2], Outcome: models.StaffActionOutcomeSkipped, Reason: "skip_issuer_not_admin"},
	}); err != nil {
		t.Fatalf("FinalizeAction: %v", err)
	}

	// One result is saved as the group finishes, before the finalize.
	if err := SaveUndoResult(action.ID, ActionGroupResult{
		GroupChatID: groups[0], Outcome: models.StaffActionOutcomeDone, Reason: "undone_unbanned",
	}); err != nil {
		t.Fatalf("SaveUndoResult: %v", err)
	}
	if err := SaveUndoResult(action.ID, ActionGroupResult{GroupChatID: groups[0] + 1000000, Outcome: "done"}); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("SaveUndoResult for a group of no row = %v, want gorm.ErrRecordNotFound", err)
	}

	finalResults := []ActionGroupResult{
		{GroupChatID: groups[0], Outcome: models.StaffActionOutcomeDone, Reason: "undone_unbanned"},
		{GroupChatID: groups[1], Outcome: models.StaffActionOutcomeSkipped, Reason: "skip_changed_since", Detail: "x &amp; y"},
	}
	if err := FinalizeUndo(action.ID, finalResults); err != nil {
		t.Fatalf("FinalizeUndo: %v", err)
	}
	first, err := GetActionFresh(action.ID)
	if err != nil || first == nil || first.UndoFinishedAt == nil {
		t.Fatalf("after FinalizeUndo undo_finished_at is not set: %v, %v", first, err)
	}
	got, err := ListActionGroupsFresh(action.ID)
	if err != nil {
		t.Fatalf("ListActionGroupsFresh: %v", err)
	}
	want := []struct{ outcome, reason, detail string }{
		{"done", "undone_unbanned", ""},
		{"skipped", "skip_changed_since", "x &amp; y"},
		{"skipped", "skip_not_applied", ""},
	}
	for i, row := range got {
		if row.UndoOutcome != want[i].outcome || row.UndoReason != want[i].reason || row.UndoDetail != want[i].detail {
			t.Fatalf("group %d undo = %q / %q / %q, want %q / %q / %q", i,
				row.UndoOutcome, row.UndoReason, row.UndoDetail, want[i].outcome, want[i].reason, want[i].detail)
		}
	}

	// A second call is harmless and keeps the first finish time.
	time.Sleep(10 * time.Millisecond)
	if err := FinalizeUndo(action.ID, finalResults); err != nil {
		t.Fatalf("second FinalizeUndo: %v", err)
	}
	second, err := GetActionFresh(action.ID)
	if err != nil || second == nil || second.UndoFinishedAt == nil || !second.UndoFinishedAt.Equal(*first.UndoFinishedAt) {
		t.Fatalf("undo_finished_at after a second FinalizeUndo = %v, want the first time %v", second.UndoFinishedAt, first.UndoFinishedAt)
	}
}

func TestStaffActionReleaseUndo(t *testing.T) {
	chat := uniqueStaffChatID()
	cleanupActionRows(t, chat)
	groups := []int64{uniqueStaffChatID(), uniqueStaffChatID()}
	action, rows := newTestAction(chat, []int{0, 1}, groups)
	if err := CreateAction(action, rows); err != nil {
		t.Fatalf("CreateAction: %v", err)
	}
	if err := FinalizeAction(action.ID, []ActionGroupResult{
		{GroupChatID: groups[0], Outcome: models.StaffActionOutcomeDone, Reason: "banned"},
		{GroupChatID: groups[1], Outcome: models.StaffActionOutcomeSkipped, Reason: "skip_issuer_not_admin"},
	}); err != nil {
		t.Fatalf("FinalizeAction: %v", err)
	}

	claimedAt, claimed, err := ClaimUndo(action.ID, 501, "Bob")
	if err != nil || !claimed || claimedAt.IsZero() {
		t.Fatalf("ClaimUndo = %v, %v, %v, want a claim time and true", claimedAt, claimed, err)
	}
	if err := SaveUndoResult(action.ID, ActionGroupResult{
		GroupChatID: groups[0], Outcome: models.StaffActionOutcomeSkipped, Reason: "skip_issuer_not_admin", Detail: "x &amp; y",
	}); err != nil {
		t.Fatalf("SaveUndoResult: %v", err)
	}

	// wantClaimKept fails unless the claim and the group's undo columns are as the
	// claim and SaveUndoResult left them.
	wantClaimKept := func(step string) {
		t.Helper()
		got, err := GetActionFresh(action.ID)
		if err != nil || got == nil {
			t.Fatalf("%s: GetActionFresh = %v, %v", step, got, err)
		}
		if got.UndoBy == nil || *got.UndoBy != 501 || got.UndoByName != "Bob" ||
			got.UndoStartedAt == nil || !got.UndoStartedAt.Equal(claimedAt) || got.UndoFinishedAt != nil {
			t.Fatalf("%s: record = undo_by %v name %q started %v finished %v, want Bob's untouched claim",
				step, got.UndoBy, got.UndoByName, got.UndoStartedAt, got.UndoFinishedAt)
		}
		groupRows, err := ListActionGroupsFresh(action.ID)
		if err != nil {
			t.Fatalf("%s: ListActionGroupsFresh: %v", step, err)
		}
		first := groupRows[0]
		if first.UndoOutcome != "skipped" || first.UndoReason != "skip_issuer_not_admin" || first.UndoDetail != "x &amp; y" {
			t.Fatalf("%s: group 0 undo = %q / %q / %q, want the saved result", step, first.UndoOutcome, first.UndoReason, first.UndoDetail)
		}
	}

	// Another presser, or another claim time, never matches this claim.
	if released, err := ReleaseUndo(action.ID, 502, claimedAt); err != nil || released {
		t.Fatalf("ReleaseUndo by another presser = %v, %v, want false and no error", released, err)
	}
	wantClaimKept("after a release by another presser")
	if released, err := ReleaseUndo(action.ID, 501, claimedAt.Add(time.Microsecond)); err != nil || released {
		t.Fatalf("ReleaseUndo of another claim time = %v, %v, want false and no error", released, err)
	}
	wantClaimKept("after a release of another claim time")

	released, err := ReleaseUndo(action.ID, 501, claimedAt)
	if err != nil || !released {
		t.Fatalf("ReleaseUndo of the claim = %v, %v, want true", released, err)
	}
	got, err := GetActionFresh(action.ID)
	if err != nil || got == nil {
		t.Fatalf("GetActionFresh = %v, %v", got, err)
	}
	if got.UndoBy != nil || got.UndoByName != "" || got.UndoStartedAt != nil || got.UndoFinishedAt != nil {
		t.Fatalf("record after the release = undo_by %v name %q started %v finished %v, want all empty",
			got.UndoBy, got.UndoByName, got.UndoStartedAt, got.UndoFinishedAt)
	}
	groupRows, err := ListActionGroupsFresh(action.ID)
	if err != nil {
		t.Fatalf("ListActionGroupsFresh: %v", err)
	}
	wantOriginal := []struct{ outcome, reason string }{{"done", "banned"}, {"skipped", "skip_issuer_not_admin"}}
	for i, row := range groupRows {
		if row.UndoOutcome != "" || row.UndoReason != "" || row.UndoDetail != "" {
			t.Fatalf("group %d undo = %q / %q / %q after the release, want every undo column empty", i, row.UndoOutcome, row.UndoReason, row.UndoDetail)
		}
		if row.Outcome != wantOriginal[i].outcome || row.Reason != wantOriginal[i].reason {
			t.Fatalf("group %d original = %q / %q after the release, want %q / %q untouched",
				i, row.Outcome, row.Reason, wantOriginal[i].outcome, wantOriginal[i].reason)
		}
	}

	if released, err := ReleaseUndo(action.ID, 501, claimedAt); err != nil || released {
		t.Fatalf("second ReleaseUndo = %v, %v, want false and no error", released, err)
	}
	if _, claimed, err := ClaimUndo(action.ID, 503, "Carol"); err != nil || !claimed {
		t.Fatalf("ClaimUndo after a release = %v, %v, want Carol to win", claimed, err)
	}

	// A claim that was finalized is a spent undo: it is never released.
	spent, spentRows := newTestAction(chat, []int{0}, []int64{uniqueStaffChatID()})
	if err := CreateAction(spent, spentRows); err != nil {
		t.Fatalf("CreateAction: %v", err)
	}
	spentAt, claimed, err := ClaimUndo(spent.ID, 501, "Bob")
	if err != nil || !claimed {
		t.Fatalf("ClaimUndo = %v, %v, want true", claimed, err)
	}
	if err := FinalizeUndo(spent.ID, nil); err != nil {
		t.Fatalf("FinalizeUndo: %v", err)
	}
	if released, err := ReleaseUndo(spent.ID, 501, spentAt); err != nil || released {
		t.Fatalf("ReleaseUndo of a finalized undo = %v, %v, want false and no error", released, err)
	}
	after, err := GetActionFresh(spent.ID)
	if err != nil || after == nil || after.UndoStartedAt == nil || after.UndoFinishedAt == nil || after.UndoBy == nil || *after.UndoBy != 501 {
		t.Fatalf("a finalized undo after ReleaseUndo = %+v, %v, want its claim kept", after, err)
	}

	if released, err := ReleaseUndo(action.ID+100000, 501, claimedAt); err != nil || released {
		t.Fatalf("ReleaseUndo of a missing record = %v, %v, want false and no error", released, err)
	}
}

func TestStaffActionSetSummaryMessage(t *testing.T) {
	chat := uniqueStaffChatID()
	cleanupActionRows(t, chat)
	action, rows := newTestAction(chat, []int{0}, []int64{uniqueStaffChatID()})
	if err := CreateAction(action, rows); err != nil {
		t.Fatalf("CreateAction: %v", err)
	}

	if err := SetSummaryMessage(action.ID, chat, 9001); err != nil {
		t.Fatalf("SetSummaryMessage: %v", err)
	}
	got, err := GetActionFresh(action.ID)
	if err != nil || got == nil {
		t.Fatalf("GetActionFresh = %v, %v", got, err)
	}
	if got.SummaryChatID != chat || got.SummaryMsgID != 9001 {
		t.Fatalf("summary = chat %d message %d, want chat %d message 9001", got.SummaryChatID, got.SummaryMsgID, chat)
	}
	if err := SetSummaryMessage(action.ID+100000, chat, 1); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("SetSummaryMessage of a missing record = %v, want gorm.ErrRecordNotFound", err)
	}
}
