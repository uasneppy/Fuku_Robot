//go:build testtools

package modules

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/lockdown"
	"github.com/divkix/Alita_Robot/alita/db/models"
)

// startPanelLockdown starts a lockdown of chatID with a reason and starter name the
// /staff panel must never show, and confirms it when confirmed is true.
func startPanelLockdown(t *testing.T, chatID int64, confirmed bool) *models.ChatLockdown {
	t.Helper()
	lockdownCleanup(t, chatID)
	row := &models.ChatLockdown{
		ChatID:            chatID,
		TriggerKind:       models.LockdownTriggerManual,
		Reason:            "secret raid",
		StartedBy:         9191,
		StartedByName:     "Locker Name",
		PrePermissions:    lockdownTestPrePermissions,
		LockedPermissions: `{"can_send_messages":false}`,
	}
	if started, err := lockdown.Start(row); err != nil || !started {
		t.Fatalf("lockdown.Start(%d) = %v, %v, want true", chatID, started, err)
	}
	if confirmed {
		if ok, err := lockdown.ConfirmLocked(row.ID); err != nil || !ok {
			t.Fatalf("lockdown.ConfirmLocked(%d) = %v, %v, want true", chatID, ok, err)
		}
	}
	return row
}

func TestStaffPanelShowsLockedGroup(t *testing.T) {
	env := newPanelEnv(t)
	alpha := env.addGroup(t, env.ownerID, "Alpha")
	bravo := env.addGroup(t, env.ownerID, "Bravo")
	charlie := env.addGroup(t, env.ownerID, "Charlie")
	delta := env.addGroup(t, env.ownerID, "Delta")
	env.addGroup(t, env.ownerID, "Echo")

	// Alpha: a confirmed lockdown. Bravo: confirmed, and the bot was removed from it.
	// Charlie: an active row Telegram never confirmed. Delta: a lift in progress.
	alphaRow := startPanelLockdown(t, alpha.GroupChatID, true)
	startPanelLockdown(t, bravo.GroupChatID, true)
	startPanelLockdown(t, charlie.GroupChatID, false)
	deltaRow := startPanelLockdown(t, delta.GroupChatID, true)
	if began, err := lockdown.BeginLift(deltaRow.ID, 9191, "Locker Name", false); err != nil || !began {
		t.Fatalf("BeginLift = %v, %v, want true", began, err)
	}
	env.setBotRole(bravo.GroupChatID, staffRoleAbsent)

	env.openPanel(t)

	panels := env.panelTexts()
	if len(panels) != 1 {
		t.Fatalf("%d panels were sent, want 1", len(panels))
	}
	panel := panels[0]
	marker := staffMarker("staff_panel_row_lockdown")

	fresh, err := lockdown.GetFresh(alphaRow.ID)
	if err != nil || fresh == nil || fresh.LockedAt == nil {
		t.Fatalf("GetFresh = %v, %v, want a confirmed row", fresh, err)
	}
	wantSince := fresh.LockedAt.UTC().Format("2 Jan 15:04")

	alphaBlock := panelRowBlock(panel, "Alpha")
	if !strings.Contains(alphaBlock, marker) || !strings.Contains(alphaBlock, wantSince) {
		t.Errorf("Alpha row %q must carry the lockdown line with %q", alphaBlock, wantSince)
	}
	bravoBlock := panelRowBlock(panel, "Bravo")
	if !strings.Contains(bravoBlock, marker) {
		t.Errorf("Bravo row %q must carry the lockdown line", bravoBlock)
	}
	if !strings.Contains(bravoBlock, staffMarker("staff_panel_reason_bot_missing")) {
		t.Errorf("Bravo row %q must carry the bot-missing reason as well", bravoBlock)
	}
	for _, title := range []string{"Charlie", "Delta", "Echo"} {
		if block := panelRowBlock(panel, title); block == "" || strings.Contains(block, marker) {
			t.Errorf("%s row %q must be listed without a lockdown line", title, block)
		}
	}
	if got := strings.Count(panel, marker); got != 2 {
		t.Errorf("panel carries %d lockdown lines, want 2 (Alpha and Bravo)", got)
	}
	for _, secret := range []string{"secret raid", "Locker Name"} {
		if strings.Contains(panel, secret) {
			t.Errorf("panel must not show %q: the reason and the starter stay in the group's own /lockdownstatus", secret)
		}
	}
}

// seedLockdownBannedTarget makes the staff target a lockdown joiner of group in a
// confirmed lockdown: a banned joiner row whose ban_until is banUntil, and the live
// member the fake reports as kicked until that date.
func seedLockdownBannedTarget(t *testing.T, env *staffActionEnv, group, banUntil int64) *models.ChatLockdown {
	t.Helper()
	row := startPanelLockdown(t, group, true)
	joiner := models.LockdownJoiner{
		LockdownID: row.ID,
		ChatID:     group,
		UserID:     staffTestTarget,
		FirstName:  "Target",
		JoinPath:   models.JoinPathMember,
		State:      models.JoinerStateBanned,
		BanUntil:   banUntil,
	}
	if err := db.DB.Create(&joiner).Error; err != nil {
		t.Fatalf("seed lockdown joiner: %v", err)
	}
	env.fake.setMember(group, staffTestTarget, staffFakeMember{
		Status: gotgbot.ChatMemberStatusKicked, UntilDate: banUntil,
	})
	return row
}

func TestStaffBanOnLockdownJoinerSurvivesLift(t *testing.T) {
	env := newStaffActionEnv(t, 2)
	withFastLockdownPacer(t)
	groupA, groupB := env.groups[0], env.groups[1]
	banUntil := time.Now().Add(330 * 24 * time.Hour).Unix()
	row := seedLockdownBannedTarget(t, env, groupA, banUntil)
	env.fake.setMember(groupB, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})

	summary, before, after := env.confirmWindow("/tban 4242 1d raid")

	callsA := env.callsTo("banChatMember", groupA)
	if len(callsA) != 1 {
		t.Fatalf("banChatMember calls in the locked group = %+v, want one: the staff ban replaces the lockdown's ban", callsA)
	}
	untilA := staffParamInt(callsA[0].Params, "until_date")
	wantUntilNear(t, "staff ban over a lockdown ban", untilA, before, after, 86400)
	if untilA == banUntil {
		t.Fatalf("until_date = %d, the lockdown's own date: the ban would still read as the lockdown's", untilA)
	}
	if callsB := env.callsTo("banChatMember", groupB); len(callsB) != 1 {
		t.Fatalf("banChatMember calls in the other group = %+v, want one", callsB)
	}
	wantDoneLine(t, summary, "Group A")
	wantDoneLine(t, summary, "Group B")

	// The lockdown is lifted: the worker looks at the live member, sees a ban that is
	// not its own and leaves it.
	if began, err := lockdown.BeginLift(row.ID, 9191, "Locker Name", false); err != nil || !began {
		t.Fatalf("BeginLift = %v, %v, want true", began, err)
	}
	runLockdownCycle(context.Background(), env.bot)

	if unbans := env.callsTo("unbanChatMember", groupA); len(unbans) != 0 {
		t.Fatalf("unbanChatMember calls in the locked group = %+v, want none: the lift must keep the staff ban", unbans)
	}
	wantMember(t, env.fake, groupA, staffTestTarget, gotgbot.ChatMemberStatusKicked)

	var joiner models.LockdownJoiner
	if err := db.DB.Where("lockdown_id = ? AND user_id = ?", row.ID, staffTestTarget).First(&joiner).Error; err != nil {
		t.Fatalf("load joiner row: %v", err)
	}
	if joiner.State != models.JoinerStateKept {
		t.Errorf("joiner state = %q, want %q: the row is kept, not unbanned", joiner.State, models.JoinerStateKept)
	}
}

func TestStaffBanLockdownLookupFails(t *testing.T) {
	env := newStaffActionEnv(t, 2)
	groupA, groupB := env.groups[0], env.groups[1]
	banUntil := time.Now().Add(330 * 24 * time.Hour).Unix()
	seedLockdownBannedTarget(t, env, groupA, banUntil)
	env.fake.setMember(groupB, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})

	previous := staffLockdownBanLookup
	t.Cleanup(func() { staffLockdownBanLookup = previous })
	staffLockdownBanLookup = func(int64, int64, int64) (bool, error) {
		return false, errors.New("database is down")
	}

	summary, _ := env.runStaffCommand("/tban 4242 1d raid")

	if writes := env.writes(groupA); len(writes) != 0 {
		t.Fatalf("write calls in the group whose lookup failed = %+v, want none", writes)
	}
	wantFailedLine(t, summary, "Group A", "staff_act_fail_internal")
	wantMember(t, env.fake, groupA, staffTestTarget, gotgbot.ChatMemberStatusKicked)
	if callsB := env.callsTo("banChatMember", groupB); len(callsB) != 1 {
		t.Fatalf("banChatMember calls in the other group = %+v, want one: one failed lookup must not stop the fan-out", callsB)
	}
	wantDoneLine(t, summary, "Group B")
}
