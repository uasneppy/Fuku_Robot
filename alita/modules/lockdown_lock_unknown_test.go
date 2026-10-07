//go:build testtools

package modules

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/divkix/Alita_Robot/alita/db/lockdown"
	"github.com/divkix/Alita_Robot/alita/db/models"
)

// A lock call that fails without Telegram refusing anything (a timeout, a dropped
// connection) may have taken effect. The lockdown row holds the only copy of the
// group's pre-lockdown permissions, so it must survive: /unlockdown restores from it
// and the worker settles it from the live permissions.

// unconfirmedRowAfterUnknownLock reads the one lockdown row left by a lock call whose
// outcome was unknown and checks that it is still a usable, unconfirmed lockdown.
func (e *lockdownEnv) unconfirmedRowAfterUnknownLock() *models.ChatLockdown {
	e.t.Helper()
	row, err := lockdown.GetActiveFresh(e.chat.Id)
	if err != nil || row == nil {
		e.t.Fatalf("GetActiveFresh = %v, %v, want the lockdown row kept: the lock may have taken effect", row, err)
	}
	if row.LockedAt != nil {
		e.t.Fatalf("LockedAt = %v, want nil: Telegram never confirmed the lock", row.LockedAt)
	}
	if row.PrePermissions != lockdownTestPrePermissions {
		e.t.Fatalf("PrePermissions = %q, want the snapshot kept for the restore", row.PrePermissions)
	}
	return row
}

func TestLockdownLockOutcomeUnknown(t *testing.T) {
	timeout := fmt.Errorf("unable to execute setChatPermissions: %w", context.DeadlineExceeded)

	t.Run("lock applied and the answer lost", func(t *testing.T) {
		env := newLockdownEnv(t)
		env.fake.setFailAfterSetPermissions(timeout)

		env.send(env.admin, "/lockdown raid")

		env.wantReplyHas(staffMarker("lockdown_lock_unknown"))
		env.wantReplyLacks(staffMarker("lockdown_lock_failed"))
		if got := env.fake.chatPermsRaw(env.chat.Id); got != lockdownLockedPermissions {
			t.Fatalf("setup: the group permissions = %q, want the locked set (the lock took effect)", got)
		}
		row := env.unconfirmedRowAfterUnknownLock()

		// The admin can put the permissions back at once from the stored snapshot.
		env.send(env.admin, "/unlockdown")

		if got := env.fake.chatPermsRaw(env.chat.Id); got != lockdownTestPrePermissions {
			t.Errorf("permissions after /unlockdown = %q, want the pre-lockdown snapshot", got)
		}
		if lifted := env.freshRow(row.ID); lifted.State != models.LockdownStateLifted {
			t.Errorf("State = %q, want lifted", lifted.State)
		}
	})

	t.Run("lock applied and the worker settles it", func(t *testing.T) {
		env := newLockdownEnv(t)
		env.fake.setFailAfterSetPermissions(timeout)

		env.send(env.admin, "/lockdown raid")
		row := env.unconfirmedRowAfterUnknownLock()
		ageLockdown(t, row.ID, 2*time.Minute)

		env.cycle()

		if settled := env.freshRow(row.ID); settled.LockedAt == nil || settled.State != models.LockdownStateActive {
			t.Errorf("row = %+v, want active and confirmed: the live permissions are the locked set", settled)
		}
	})

	t.Run("lock not applied and the worker drops the row", func(t *testing.T) {
		env := newLockdownEnv(t)
		env.fake.script("setChatPermissions", env.chat.Id, staffFakeError(502, "Bad Gateway"))

		env.send(env.admin, "/lockdown raid")

		env.wantReplyHas(staffMarker("lockdown_lock_unknown"))
		row := env.unconfirmedRowAfterUnknownLock()
		ageLockdown(t, row.ID, 2*time.Minute)

		env.cycle()

		if current, err := lockdown.GetActiveFresh(env.chat.Id); err != nil || current != nil {
			t.Errorf("GetActiveFresh = %v, %v, want no lockdown: the lock never took effect", current, err)
		}
		if got := env.fake.chatPermsRaw(env.chat.Id); got != lockdownTestPrePermissions {
			t.Errorf("group permissions = %q, want unchanged", got)
		}
	})
}
