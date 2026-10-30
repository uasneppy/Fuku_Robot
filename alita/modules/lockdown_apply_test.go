//go:build testtools

package modules

import (
	"context"
	"errors"
	"testing"

	"github.com/divkix/Alita_Robot/alita/db/models"
)

// TestApplyAutoLockdown pins the one write path of the automatic lockdown triggers: the
// outcomes of applyAutoLockdown and the rows it leaves behind.
func TestApplyAutoLockdown(t *testing.T) {
	t.Run("a threshold is saved and turns its trigger on", func(t *testing.T) {
		env := newLockdownEnv(t)
		raidCleanup(t, env.chat.Id)

		got := applyAutoLockdown(&env.chat, autoLockdownChange{
			Trigger: autoTriggerJoins, SetThreshold: true, Threshold: 20, Enable: true,
		})

		if got.Outcome != autoApplySaved || got.Err != nil || got.ReadErr != nil {
			t.Fatalf("result = %+v, want saved with no error", got)
		}
		if got.Settings == nil || got.Settings.JoinThreshold != 20 || !got.Settings.JoinEnabled {
			t.Errorf("settings read back = %+v, want joins on at 20", got.Settings)
		}
		env.wantAutoRow(true, 20, true, 30)
	})

	t.Run("a switch is saved", func(t *testing.T) {
		env := newLockdownEnv(t)
		raidCleanup(t, env.chat.Id)

		got := applyAutoLockdown(&env.chat, autoLockdownChange{Trigger: autoTriggerFlood, Enable: false})

		if got.Outcome != autoApplySaved || got.Settings == nil || got.Settings.FloodEnabled {
			t.Fatalf("result = %+v, want saved with flood off", got)
		}
		env.wantAutoRow(true, 15, false, 30)
	})

	t.Run("a threshold outside 3..500 is a range refusal and changes nothing", func(t *testing.T) {
		for _, n := range []int{2, 501} {
			env := newLockdownEnv(t)
			raidCleanup(t, env.chat.Id)
			seeded := applyAutoLockdown(&env.chat, autoLockdownChange{
				Trigger: autoTriggerAI, SetThreshold: true, Threshold: 7, Enable: true,
			})
			if seeded.Outcome != autoApplySaved {
				t.Fatalf("setup: result = %+v, want saved", seeded)
			}

			got := applyAutoLockdown(&env.chat, autoLockdownChange{
				Trigger: autoTriggerAI, SetThreshold: true, Threshold: n, Enable: true,
			})

			if got.Outcome != autoApplyRange {
				t.Errorf("threshold %d: outcome = %v, want autoApplyRange", n, got.Outcome)
			}
			env.wantAutoAIRow(true, 7)
		}
	})

	t.Run("a basic group is refused and writes nothing", func(t *testing.T) {
		env := newLockdownEnv(t)
		raidCleanup(t, env.chat.Id)
		env.chat.Type = "group"

		got := applyAutoLockdown(&env.chat, autoLockdownChange{
			Trigger: autoTriggerJoins, SetThreshold: true, Threshold: 20, Enable: true,
		})

		if got.Outcome != autoApplyBasicGroup {
			t.Errorf("outcome = %v, want autoApplyBasicGroup", got.Outcome)
		}
		env.wantNoAutoRow()
	})

	t.Run("a failed write is a save failure carrying the error", func(t *testing.T) {
		env := newLockdownEnv(t)
		raidCleanup(t, env.chat.Id)
		previous := autolockdownWrite
		autolockdownWrite = func(int64, autoLockdownChange) error { return errors.New("database is gone") }
		t.Cleanup(func() { autolockdownWrite = previous })

		got := applyAutoLockdown(&env.chat, autoLockdownChange{Trigger: autoTriggerJoins, Enable: false})

		if got.Outcome != autoApplySaveFailed || got.Err == nil {
			t.Errorf("result = %+v, want autoApplySaveFailed with Err set", got)
		}
		env.wantNoAutoRow()
	})

	t.Run("a failed readback after the write is saved with ReadErr", func(t *testing.T) {
		env := newLockdownEnv(t)
		raidCleanup(t, env.chat.Id)
		previous := raidSettingsLookup
		raidSettingsLookup = func(context.Context, int64) (*models.ChatRaidDetection, error) {
			return nil, errors.New("settings unreadable")
		}
		t.Cleanup(func() { raidSettingsLookup = previous })

		got := applyAutoLockdown(&env.chat, autoLockdownChange{Trigger: autoTriggerJoins, Enable: false})

		if got.Outcome != autoApplySaved || got.ReadErr == nil || got.Settings != nil {
			t.Errorf("result = %+v, want saved with ReadErr set and no settings", got)
		}
		env.wantAutoRow(false, 15, true, 30)
	})
}
