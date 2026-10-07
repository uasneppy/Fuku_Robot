//go:build testtools

package modules

import (
	"strings"
	"testing"

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
		StartedBy:         4242,
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
	if began, err := lockdown.BeginLift(deltaRow.ID, 4242, "Locker Name", false); err != nil || !began {
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
