//go:build testtools

package modules

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/divkix/Alita_Robot/alita/db/models"
)

// surgeLockdown makes the env's chat lock itself through a join surge of 15 accounts and
// returns the lockdown row.
func (e *lockdownEnv) surgeLockdown() *models.ChatLockdown {
	e.t.Helper()
	e.selfJoins(15)
	row := e.activeLockdown()
	if row == nil || row.TriggerKind != models.LockdownTriggerJoinSurge || row.LockedAt == nil {
		e.t.Fatalf("setup: active lockdown = %+v, want a confirmed join surge lockdown", row)
	}
	return row
}

// floodLockdown makes the env's chat lock itself through a flood of 30 items from four
// recent joiners and returns the lockdown row.
func (e *lockdownEnv) floodLockdown() *models.ChatLockdown {
	e.t.Helper()
	recent := e.newMembers(4, 2*time.Hour)
	e.sendItems(recent, 30)
	row := e.activeLockdown()
	if row == nil || row.TriggerKind != models.LockdownTriggerFlood || row.LockedAt == nil {
		e.t.Fatalf("setup: active lockdown = %+v, want a confirmed flood lockdown", row)
	}
	return row
}

func TestRaidLockdownStatus(t *testing.T) {
	t.Run("surge status", func(t *testing.T) {
		env := newLockdownEnv(t)
		linkLockdownStaffGroup(env)
		raidCleanup(t, env.chat.Id)
		row := env.surgeLockdown()

		env.askStatus(env.admin)

		env.wantReplyHas(
			staffMarker("lockdown_status_active_auto"),
			lockdownTime(*row.LockedAt),
			staffMarker("lockdown_trigger_join_surge")+" 15 60",
			staffMarker("lockdown_status_removed"),
		)
		env.wantReplyLacks(staffMarker("lockdown_status_active"), staffMarker("lockdown_reason_line"))
	})

	t.Run("flood status", func(t *testing.T) {
		env := newLockdownEnv(t)
		linkLockdownStaffGroup(env)
		raidCleanup(t, env.chat.Id)
		env.floodLockdown()

		env.askStatus(env.admin)

		env.wantReplyHas(
			staffMarker("lockdown_status_active_auto"),
			staffMarker("lockdown_trigger_flood")+" 30 30 100",
		)
	})

	t.Run("already active", func(t *testing.T) {
		env := newLockdownEnv(t)
		linkLockdownStaffGroup(env)
		raidCleanup(t, env.chat.Id)
		env.surgeLockdown()
		locksBefore := len(env.calls("setChatPermissions"))

		env.send(env.admin, "/lockdown")

		env.wantReplyHas(
			staffMarker("lockdown_already_active_auto"),
			staffMarker("lockdown_trigger_join_surge")+" 15 60",
		)
		if got := len(env.calls("setChatPermissions")); got != locksBefore {
			t.Errorf("setChatPermissions calls = %d after /lockdown, want still %d", got, locksBefore)
		}
	})

	t.Run("manual unchanged", func(t *testing.T) {
		env := newLockdownEnv(t)
		env.lockAsAdmin("raid")

		env.askStatus(env.admin)

		env.wantReplyHas(staffMarker("lockdown_status_active"), "Ad&lt;b&gt;min", staffMarker("lockdown_reason_line"))
		env.wantReplyLacks(staffMarker("lockdown_trigger_join_surge"), staffMarker("lockdown_trigger_flood"),
			staffMarker("lockdown_status_active_auto"))

		env.send(env.admin, "/lockdown")
		env.wantReplyHas(staffMarker("lockdown_already_active"), "Ad&lt;b&gt;min")
		env.wantReplyLacks(staffMarker("lockdown_already_active_auto"), staffMarker("lockdown_trigger_join_surge"))
	})
}

func TestRaidLockdownReminder(t *testing.T) {
	// secondStaffMessage lets the worker post the alert, ages it by 61 minutes, runs one
	// more cycle and returns the reminder the Staff Group received after the alert.
	secondStaffMessage := func(t *testing.T, env *lockdownEnv, lockdownID uint, staffChatID int64) string {
		t.Helper()
		env.cycle()
		if got := len(env.fake.sentTo(staffChatID)); got != 1 {
			t.Fatalf("%d messages reached the Staff Group after the alert cycle, want 1", got)
		}
		ageAlert(t, lockdownID, 61*time.Minute)
		env.cycle()
		sent := env.fake.sentTo(staffChatID)
		if len(sent) != 2 {
			t.Fatalf("%d messages reached the Staff Group, want the alert and one reminder", len(sent))
		}
		return fmt.Sprint(sent[1].Params["text"])
	}

	t.Run("surge reminder", func(t *testing.T) {
		env := newLockdownEnv(t)
		staffChat := lockdownBurstEnv(env)
		raidCleanup(t, env.chat.Id)
		row := env.surgeLockdown()

		text := secondStaffMessage(t, env, row.ID, staffChat.Id)

		for _, want := range []string{
			staffMarker("lockdown_reminder_title"),
			staffMarker("lockdown_trigger_join_surge") + " 15 60",
		} {
			if !strings.Contains(text, want) {
				t.Errorf("reminder %q does not contain %q", text, want)
			}
		}
		if strings.Contains(text, staffMarker("lockdown_reason_line")) {
			t.Errorf("reminder %q has a reason line", text)
		}
	})

	t.Run("flood reminder", func(t *testing.T) {
		env := newLockdownEnv(t)
		staffChat := lockdownBurstEnv(env)
		raidCleanup(t, env.chat.Id)
		row := env.floodLockdown()

		text := secondStaffMessage(t, env, row.ID, staffChat.Id)

		for _, want := range []string{
			staffMarker("lockdown_reminder_title"),
			staffMarker("lockdown_trigger_flood") + " 30 30 100",
		} {
			if !strings.Contains(text, want) {
				t.Errorf("reminder %q does not contain %q", text, want)
			}
		}
	})

	t.Run("manual reminder", func(t *testing.T) {
		env := newLockdownEnv(t)
		staffChat := lockdownBurstEnv(env)
		row := env.lockAsAdmin("raid")

		text := secondStaffMessage(t, env, row.ID, staffChat.Id)

		for _, want := range []string{staffMarker("lockdown_reminder_title"), staffMarker("lockdown_reason_line"), "raid"} {
			if !strings.Contains(text, want) {
				t.Errorf("reminder %q does not contain %q", text, want)
			}
		}
		for _, unwanted := range []string{staffMarker("lockdown_trigger_join_surge"), staffMarker("lockdown_trigger_flood")} {
			if strings.Contains(text, unwanted) {
				t.Errorf("reminder %q contains %q", text, unwanted)
			}
		}
	})
}
