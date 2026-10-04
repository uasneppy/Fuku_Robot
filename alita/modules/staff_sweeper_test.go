//go:build testtools

package modules

import (
	"context"
	"strings"
	"testing"

	"github.com/divkix/Alita_Robot/alita/db/models"
)

// sweepNoPace removes the pause between link checks for the test's duration.
func sweepNoPace(t *testing.T) {
	t.Helper()
	previous := staffSweepPace
	staffSweepPace = 0
	t.Cleanup(func() { staffSweepPace = previous })
}

func TestStaffSweepTracer(t *testing.T) {
	sweepNoPace(t)
	env := newOwnershipEnv(t)
	g1 := env.addGroup(t, env.ownerID, "Group One")
	g2 := env.addGroup(t, env.ownerID, "Group Two")

	// Nobody ran a command and no watcher fired: G2's creator changed while the
	// bot is only a plain member there, and G1's bot lost the restrict right.
	env.client.setCreator(g2.GroupChatID, env.ownerID+1)
	env.client.smu.Lock()
	env.client.botRole[g2.GroupChatID] = staffRoleMember
	env.client.botRole[g1.GroupChatID] = staffRoleAdminNoRestrict
	env.client.smu.Unlock()

	report := runStaffSweep(context.Background(), env.bot)

	if !report.Ran || report.Removed != 1 || report.HealthChanged != 1 {
		t.Fatalf("report = %+v, want Ran, Removed 1, HealthChanged 1", report)
	}
	wantLinkGone(t, g2.GroupChatID)
	wantHealth(t, g1.GroupChatID, models.StaffHealthBotCannotRestrict)

	notices := textsToChat(env.client, env.staffID)
	if len(notices) != 2 {
		t.Fatalf("notices to the Staff Group = %q, want 2", notices)
	}
	var unlinked, health int
	for _, text := range notices {
		switch {
		case strings.Contains(text, staffMarker("staff_notice_unlinked_group_owner_changed")):
			unlinked++
		case strings.Contains(text, staffMarker("staff_notice_health_bot_cannot_restrict")):
			health++
		}
	}
	if unlinked != 1 || health != 1 {
		t.Fatalf("notices = %q, want one owner-changed and one cannot-restrict", notices)
	}

	before := len(env.client.callsFor("sendMessage"))
	again := runStaffSweep(context.Background(), env.bot)
	if again.Removed != 0 || again.HealthChanged != 0 {
		t.Fatalf("second report = %+v, want no changes", again)
	}
	if after := len(env.client.callsFor("sendMessage")); after != before {
		t.Fatalf("second sweep sent %d more message(s), want none", after-before)
	}
}
