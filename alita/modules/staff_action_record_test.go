//go:build testtools

package modules

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
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

// seedTargetInGroups puts the target into every group of env with the same record.
func seedTargetInGroups(env *staffActionEnv, m staffFakeMember) {
	for _, group := range env.groups {
		member := m
		env.fake.setMember(group, staffTestTarget, member)
	}
}

func TestStaffActionRecordEveryKind(t *testing.T) {
	now := time.Now().Unix()

	// Every kind is recorded with the state the target had before it: run the
	// command, then read the record fresh and compare.
	type want struct {
		action        string
		priorStatus   string
		priorIsMember bool
		priorUntil    int64
		reason        staffReason
	}
	run := func(t *testing.T, env *staffActionEnv, command string) (*models.StaffAction, []models.StaffActionGroup) {
		t.Helper()
		_, msgID := env.startRun(command)
		env.waitRuns()
		action, rows := recordOfCard(t, msgID)
		if len(rows) != len(env.groups) {
			t.Fatalf("group rows = %d, want %d", len(rows), len(env.groups))
		}
		return action, rows
	}
	check := func(t *testing.T, action *models.StaffAction, rows []models.StaffActionGroup, w want) {
		t.Helper()
		if action.Action != w.action {
			t.Fatalf("action = %q, want %q", action.Action, w.action)
		}
		for _, row := range rows {
			if row.Outcome != models.StaffActionOutcomeDone || row.Reason != string(w.reason) {
				t.Fatalf("group %d = %q %q, want done %s", row.GroupChatID, row.Outcome, row.Reason, w.reason)
			}
			if row.PriorStatus != w.priorStatus || row.PriorIsMember != w.priorIsMember || row.PriorUntil != w.priorUntil {
				t.Fatalf("group %d prior = %q member=%v until=%d, want %q member=%v until=%d",
					row.GroupChatID, row.PriorStatus, row.PriorIsMember, row.PriorUntil,
					w.priorStatus, w.priorIsMember, w.priorUntil)
			}
		}
	}

	t.Run("mute over a member", func(t *testing.T) {
		env := newStaffActionEnv(t, 2)
		seedTargetInGroups(env, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
		action, rows := run(t, env, "/mute 4242")
		check(t, action, rows, want{"mute", gotgbot.ChatMemberStatusMember, true, 0, staffReasonMuted})
		for _, row := range rows {
			if row.PriorPermissions != "" {
				t.Fatalf("a member's prior permissions = %q, want none", row.PriorPermissions)
			}
		}
	})

	t.Run("mute over a shorter mute", func(t *testing.T) {
		env := newStaffActionEnv(t, 2)
		shorter := now + 3600
		seedTargetInGroups(env, staffFakeMember{
			Status: gotgbot.ChatMemberStatusRestricted, IsMember: true, CanSendMessages: false, UntilDate: shorter,
		})
		action, rows := run(t, env, "/mute 4242 1d")
		check(t, action, rows, want{"mute", gotgbot.ChatMemberStatusRestricted, true, shorter, staffReasonMuted})
		if action.DurationSec != 86400 || action.UntilDate == 0 {
			t.Fatalf("mute duration %ds until %d, want one day with an end date", action.DurationSec, action.UntilDate)
		}
	})

	t.Run("unban over a ban", func(t *testing.T) {
		env := newStaffActionEnv(t, 2)
		banEnds := now + 7200
		seedTargetInGroups(env, staffFakeMember{Status: gotgbot.ChatMemberStatusKicked, UntilDate: banEnds})
		action, rows := run(t, env, "/unban 4242")
		check(t, action, rows, want{"unban", gotgbot.ChatMemberStatusKicked, false, banEnds, staffReasonUnbanned})
	})

	t.Run("unmute over a partial restriction", func(t *testing.T) {
		env := newStaffActionEnv(t, 2)
		// The member cannot send text (so the staff's unmute applies) but keeps
		// polls, invites and pins: only the stored permission set can put that back.
		partial := gotgbot.ChatPermissions{
			CanSendMessages:       false,
			CanSendPhotos:         false,
			CanSendVideos:         false,
			CanSendOtherMessages:  false,
			CanAddWebPagePreviews: false,
			CanSendPolls:          true,
			CanInviteUsers:        true,
			CanPinMessages:        true,
		}
		until := now + 7200
		seedTargetInGroups(env, staffFakeMember{
			Status: gotgbot.ChatMemberStatusRestricted, IsMember: true, UntilDate: until, Perms: &partial,
		})
		action, rows := run(t, env, "/unmute 4242")
		check(t, action, rows, want{"unmute", gotgbot.ChatMemberStatusRestricted, true, until, staffReasonUnmuted})

		// The three optional flags are stored as explicit false, since the live read
		// carries them as plain booleans.
		wantPerms := partial
		wantPerms.CanReactToMessages = helpersPtrFalse()
		wantPerms.CanEditTag = helpersPtrFalse()
		wantPerms.CanManageTopics = helpersPtrFalse()
		for _, row := range rows {
			if got := priorPermissionsOf(t, row); !reflect.DeepEqual(got, wantPerms) {
				t.Fatalf("group %d prior permissions = %+v, want exactly %+v", row.GroupChatID, got, wantPerms)
			}
			prior, err := staffPriorFromRow(row)
			if err != nil || prior.Perms == nil || !reflect.DeepEqual(*prior.Perms, wantPerms) {
				t.Fatalf("staffPriorFromRow = %+v, %v, want the same permission set", prior, err)
			}
		}
	})

	t.Run("kick over a member", func(t *testing.T) {
		env := newStaffActionEnv(t, 2)
		seedTargetInGroups(env, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
		action, rows := run(t, env, "/kick 4242")
		check(t, action, rows, want{"kick", gotgbot.ChatMemberStatusMember, true, 0, staffReasonKicked})
	})

	t.Run("over-limit ban", func(t *testing.T) {
		env := newStaffActionEnv(t, 2)
		seedTargetInGroups(env, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
		action, rows := run(t, env, "/ban 4242 400d")
		check(t, action, rows, want{"ban", gotgbot.ChatMemberStatusMember, true, 0, staffReasonBanned})
		if action.UntilDate != 0 || !action.OverLimit || action.DurationAmount != 400 || action.DurationUnit != "d" {
			t.Fatalf("over-limit ban stored until=%d over_limit=%v as %d%s, want 0, true, 400d",
				action.UntilDate, action.OverLimit, action.DurationAmount, action.DurationUnit)
		}
	})
}

// helpersPtrFalse is a pointer to false, the shape a permission flag has after it
// went through the record.
func helpersPtrFalse() *bool {
	v := false
	return &v
}

// countActionRows counts staff_actions rows of the Staff Group whose summary is
// the card message msgID.
func countActionRows(t *testing.T, staffChat, msgID int64) int64 {
	t.Helper()
	var n int64
	err := db.DB.Model(&models.StaffAction{}).
		Where("staff_chat_id = ? AND summary_msg_id = ?", staffChat, msgID).Count(&n).Error
	if err != nil {
		t.Fatalf("count staff_actions: %v", err)
	}
	return n
}

func TestStaffActionRecordFailureFailsClosed(t *testing.T) {
	t.Run("record create fails", func(t *testing.T) {
		env := newStaffActionEnv(t, 3)
		seedMembers(env)
		previous := staffCreateActionRecord
		staffCreateActionRecord = func(*models.StaffAction, []models.StaffActionGroup) error {
			return errors.New("database is down")
		}
		t.Cleanup(func() { staffCreateActionRecord = previous })

		env.send(env.issuer, "/ban 4242")
		token, msgID := env.card()
		env.tap(env.issuer, staffActRunConfirm, token, msgID)
		env.waitRuns()

		if text := env.lastEditText(env.staffChat, msgID); !strings.Contains(text, staffMarker("staff_act_abort_check_failed")) {
			t.Fatalf("card = %q, want the could-not-check abort", text)
		}
		for _, group := range env.groups {
			if writes := env.writes(group); len(writes) != 0 {
				t.Fatalf("write calls in group %d = %+v, want none without a record", group, writes)
			}
		}
		if n := countActionRows(t, env.staffChat, msgID); n != 0 {
			t.Fatalf("staff_actions rows = %d, want 0", n)
		}
		if holder := targetLockHolder(t, staffTestTarget); holder != "" {
			t.Fatalf("target lock is still held by %q after the abort", holder)
		}
	})

	t.Run("prior write fails in one group", func(t *testing.T) {
		env := newStaffActionEnv(t, 3)
		seedMembers(env)
		failing := env.groups[1]
		previous := staffSavePrior
		staffSavePrior = func(actionID uint, groupChatID int64, prior staff.ActionPrior) error {
			if groupChatID == failing {
				return errors.New("database is down")
			}
			return previous(actionID, groupChatID, prior)
		}
		t.Cleanup(func() { staffSavePrior = previous })

		summary, msgID := env.runStaffCommand("/ban 4242")

		if line := summaryLine(t, summary, "Group B"); !strings.Contains(line, staffMarker("staff_act_fail_internal")) {
			t.Fatalf("line for Group B = %q, want the internal-error reason", line)
		}
		if writes := env.writes(failing); len(writes) != 0 {
			t.Fatalf("write calls in the failing group = %+v, want none: its prior state was not stored", writes)
		}
		wantDoneLine(t, summary, "Group A")
		wantDoneLine(t, summary, "Group C")
		_, rows := recordOfCard(t, msgID)
		for _, row := range rows {
			if row.GroupChatID == failing {
				if row.Outcome != models.StaffActionOutcomeFailed || row.Reason != string(staffReasonFailInternal) {
					t.Fatalf("failing group row = %q %q, want failed fail_internal", row.Outcome, row.Reason)
				}
				continue
			}
			if row.Outcome != models.StaffActionOutcomeDone {
				t.Fatalf("group %d row = %q, want done", row.GroupChatID, row.Outcome)
			}
		}
	})
}

func TestStopStaffActionsRecordsInterrupted(t *testing.T) {
	env := newStaffActionEnv(t, 8)
	seedMembers(env)
	withStaffActionTimers(t, time.Hour, 5*time.Millisecond)
	t.Cleanup(func() {
		staffActionsMu.Lock()
		defer staffActionsMu.Unlock()
		staffActionsCtx, staffActionsCancel = context.WithCancel(context.Background())
	})
	for _, group := range env.groups {
		env.fake.setDelay("getChatMember", group, 200*time.Millisecond)
	}

	_, msgID := env.startRun("/ban 4242")
	time.Sleep(50 * time.Millisecond)
	StopStaffActions()
	env.waitRuns()

	action, rows := recordOfCard(t, msgID)
	if action.FinishedAt == nil {
		t.Fatal("finished_at is nil after a shutdown finalized the run")
	}
	interrupted := 0
	for _, row := range rows {
		if row.Outcome == models.StaffActionOutcomePending {
			t.Fatalf("group %d is still pending in the record", row.GroupChatID)
		}
		if row.Outcome == models.StaffActionOutcomeFailed && row.Reason == string(staffReasonFailInterrupted) {
			interrupted++
		}
	}
	if interrupted == 0 {
		t.Fatalf("no group reads failed fail_interrupted: %+v", rows)
	}
}

func TestStaffActionRecordTwice(t *testing.T) {
	env := newStaffActionEnv(t, 3)
	seedMembers(env)

	_, firstMsg := env.startRun("/ban 4242")
	env.waitRuns()
	_, secondMsg := env.startRun("/unban 4242")
	env.waitRuns()

	first, firstRows := recordOfCard(t, firstMsg)
	second, secondRows := recordOfCard(t, secondMsg)
	if first.ID == second.ID {
		t.Fatalf("both actions share record %d, want two separate records", first.ID)
	}
	if first.Action != "ban" || second.Action != "unban" {
		t.Fatalf("actions = %q, %q, want ban then unban", first.Action, second.Action)
	}
	if len(firstRows) != 3 || len(secondRows) != 3 {
		t.Fatalf("group rows = %d and %d, want 3 each", len(firstRows), len(secondRows))
	}
	for _, row := range secondRows {
		if row.ActionID != second.ID {
			t.Fatalf("group row of action %d belongs to action %d", second.ID, row.ActionID)
		}
	}
}

func TestStaffActionRecordAllSkipped(t *testing.T) {
	env := newStaffActionEnv(t, 3)
	for _, group := range env.groups {
		env.fake.setMember(group, env.issuer.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
	}

	_, msgID := env.startRun("/ban 4242")
	env.waitRuns()

	action, rows := recordOfCard(t, msgID)
	if len(rows) != 3 {
		t.Fatalf("group rows = %d, want 3", len(rows))
	}
	for _, row := range rows {
		if row.Outcome != models.StaffActionOutcomeSkipped {
			t.Fatalf("group %d = %q, want skipped", row.GroupChatID, row.Outcome)
		}
	}
	if action.FinishedAt == nil {
		t.Fatal("finished_at is nil: an all-skipped action is still a finished record")
	}
	if action.Reason != "" {
		t.Fatalf("reason = %q, want empty for a command without a reason", action.Reason)
	}
}

func TestStaffActionRecordNoRecordOnAbort(t *testing.T) {
	env := newStaffActionEnv(t, 2)
	seedMembers(env)
	env.send(env.issuer, "/ban 4242")
	token, msgID := env.card()
	third := env.addLinkedGroup("Group C")
	env.fake.setMember(third, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})

	env.tap(env.issuer, staffActRunConfirm, token, msgID)
	env.waitRuns()

	if text := env.lastEditText(env.staffChat, msgID); !strings.Contains(text, staffMarker("staff_act_abort_links_changed")) {
		t.Fatalf("card = %q, want the links-changed abort", text)
	}
	if n := countActionRows(t, env.staffChat, msgID); n != 0 {
		t.Fatalf("staff_actions rows = %d, want 0: a card that aborts before the run leaves no record", n)
	}
}
