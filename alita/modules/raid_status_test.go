//go:build testtools

package modules

import (
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
