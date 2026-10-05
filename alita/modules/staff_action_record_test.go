//go:build testtools

package modules

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"gorm.io/gorm"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/db/staff"
)

// recordOfCard reads the audit record of the staff action whose card message is
// msgID, with a fresh query and no in-memory state, so it shows what survives a
// restart. It fails the test when the action left no record.
func recordOfCard(t *testing.T, msgID int64) (*models.StaffAction, []models.StaffActionGroup) {
	t.Helper()
	var action models.StaffAction
	err := db.DB.Where("summary_msg_id = ?", msgID).Order("id DESC").First(&action).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("no staff_actions row for card message %d: the confirmed action left no audit record", msgID)
	}
	if err != nil {
		t.Fatalf("read staff_actions for card message %d: %v", msgID, err)
	}
	groups, err := staff.ListActionGroupsFresh(action.ID)
	if err != nil {
		t.Fatalf("list groups of staff action %d: %v", action.ID, err)
	}
	return &action, groups
}

// priorPermissionsOf decodes the stored permission snapshot of a group row.
func priorPermissionsOf(t *testing.T, row models.StaffActionGroup) gotgbot.ChatPermissions {
	t.Helper()
	var perms gotgbot.ChatPermissions
	if err := json.Unmarshal([]byte(row.PriorPermissions), &perms); err != nil {
		t.Fatalf("prior_permissions %q of group %d is not a ChatPermissions JSON: %v", row.PriorPermissions, row.GroupChatID, err)
	}
	return perms
}

func TestStaffActionRecordsPriorState(t *testing.T) {
	env := newStaffActionEnv(t, 3)
	groupA, groupB, groupC := env.groups[0], env.groups[1], env.groups[2]
	now := time.Now().Unix()
	// A: a plain member (the fake's default). B: restricted for two more hours.
	// C: banned for one more hour, a shorter ban than the staff's two days.
	restrictedUntil := now + 7200
	keptBanUntil := now + 3600
	env.fake.setMember(groupB, staffTestTarget, staffFakeMember{
		Status: gotgbot.ChatMemberStatusRestricted, IsMember: true, CanSendMessages: false, UntilDate: restrictedUntil,
	})
	env.fake.setMember(groupC, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusKicked, UntilDate: keptBanUntil})

	_, msgID := env.startRun("/ban 4242 2d spamming")
	env.waitRuns()

	_, rows := recordOfCard(t, msgID)
	if len(rows) != 3 {
		t.Fatalf("group rows = %d, want 3", len(rows))
	}
	byGroup := map[int64]models.StaffActionGroup{}
	for _, row := range rows {
		byGroup[row.GroupChatID] = row
	}

	a := byGroup[groupA]
	if a.PriorStatus != gotgbot.ChatMemberStatusMember || !a.PriorIsMember || a.PriorUntil != 0 || a.PriorPermissions != "" {
		t.Fatalf("group A prior = %q member=%v until=%d perms=%q, want member, in the group, 0, no permissions",
			a.PriorStatus, a.PriorIsMember, a.PriorUntil, a.PriorPermissions)
	}
	b := byGroup[groupB]
	if b.PriorStatus != gotgbot.ChatMemberStatusRestricted || !b.PriorIsMember || b.PriorUntil != restrictedUntil {
		t.Fatalf("group B prior = %q member=%v until=%d, want restricted, in the group, until %d",
			b.PriorStatus, b.PriorIsMember, b.PriorUntil, restrictedUntil)
	}
	if perms := priorPermissionsOf(t, b); perms.CanSendMessages {
		t.Fatalf("group B prior permissions %+v, want CanSendMessages false", perms)
	}
	c := byGroup[groupC]
	if c.PriorStatus != gotgbot.ChatMemberStatusKicked || c.PriorUntil != keptBanUntil || c.PriorPermissions != "" {
		t.Fatalf("group C prior = %q until=%d perms=%q, want kicked, until %d, no permissions",
			c.PriorStatus, c.PriorUntil, c.PriorPermissions, keptBanUntil)
	}
	for _, row := range rows {
		if row.Outcome != models.StaffActionOutcomeDone {
			t.Fatalf("group %d outcome = %q (%s), want done", row.GroupChatID, row.Outcome, row.Reason)
		}
	}
}

func TestStaffActionRecordPerGroup(t *testing.T) {
	env := newStaffActionEnv(t, 3)
	// The issuer is only a plain member in group C, so C is skipped.
	env.fake.setMember(env.groups[2], env.issuer.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})

	_, msgID := env.startRun("/ban 4242")
	env.waitRuns()

	_, rows := recordOfCard(t, msgID)
	if len(rows) != 3 {
		t.Fatalf("group rows = %d, want 3", len(rows))
	}
	titles := []string{"Group A", "Group B", "Group C"}
	for i, row := range rows {
		if row.Seq != i || row.GroupChatID != env.groups[i] || row.GroupTitle != titles[i] {
			t.Fatalf("row %d = seq %d group %d %q, want seq %d group %d %q",
				i, row.Seq, row.GroupChatID, row.GroupTitle, i, env.groups[i], titles[i])
		}
	}
	for _, row := range rows[:2] {
		if row.Outcome != models.StaffActionOutcomeDone || row.Reason != string(staffReasonBanned) || row.AppliedAt == nil {
			t.Fatalf("group %d = %q %q applied=%v, want done banned with applied_at", row.GroupChatID, row.Outcome, row.Reason, row.AppliedAt)
		}
	}
	skipped := rows[2]
	if skipped.Outcome != models.StaffActionOutcomeSkipped || skipped.Reason != string(staffReasonSkipIssuerNotAdmin) {
		t.Fatalf("group C = %q %q, want skipped skip_issuer_not_admin", skipped.Outcome, skipped.Reason)
	}
	if skipped.PriorStatus != "" || skipped.AppliedAt != nil {
		t.Fatalf("group C prior = %q applied=%v, want no prior state and no applied_at (no write was made)",
			skipped.PriorStatus, skipped.AppliedAt)
	}
}

func TestStaffActionRecordFinalize(t *testing.T) {
	env := newStaffActionEnv(t, 3)

	_, msgID := env.startRun("/ban 4242 2d spamming")
	env.waitRuns()

	action, _ := recordOfCard(t, msgID)
	bans := env.callsTo("banChatMember", env.groups[0])
	if len(bans) != 1 {
		t.Fatalf("banChatMember calls in group A = %d, want 1", len(bans))
	}
	sentUntil := staffParamInt(bans[0].Params, "until_date")
	if sentUntil == 0 {
		t.Fatal("the ban carried no until_date")
	}

	if action.Action != string(staffKindBan) || action.Reason != "spamming" {
		t.Fatalf("action = %q reason %q, want ban, spamming", action.Action, action.Reason)
	}
	if action.IssuerUserID != env.issuer.Id || action.IssuerName != "Iss<i>uer" {
		t.Fatalf("issuer = %d %q, want %d %q (stored raw)", action.IssuerUserID, action.IssuerName, env.issuer.Id, "Iss<i>uer")
	}
	if action.TargetUserID != staffTestTarget {
		t.Fatalf("target = %d, want %d", action.TargetUserID, staffTestTarget)
	}
	if action.DurationSec != 172800 || action.DurationAmount != 2 || action.DurationUnit != "d" || action.OverLimit {
		t.Fatalf("duration = %ds as %d%s over_limit=%v, want 172800s as 2d within the limit",
			action.DurationSec, action.DurationAmount, action.DurationUnit, action.OverLimit)
	}
	if action.UntilDate != sentUntil {
		t.Fatalf("until_date = %d, want the %d sent to the groups", action.UntilDate, sentUntil)
	}
	if action.GroupCount != 3 {
		t.Fatalf("group_count = %d, want 3", action.GroupCount)
	}
	if action.StaffChatID != env.staffChat || action.SummaryChatID != env.staffChat || action.SummaryMsgID != msgID {
		t.Fatalf("staff chat %d summary %d/%d, want %d and %d/%d",
			action.StaffChatID, action.SummaryChatID, action.SummaryMsgID, env.staffChat, env.staffChat, msgID)
	}
	if action.FinishedAt == nil {
		t.Fatal("finished_at is nil after the run ended")
	}
	if action.CreatedAt.IsZero() {
		t.Fatal("created_at was not set")
	}
}
