//go:build testtools

package modules

import (
	"testing"
	"time"

	"github.com/divkix/Alita_Robot/alita/db"
	dbcache "github.com/divkix/Alita_Robot/alita/db/cache"
	"github.com/divkix/Alita_Robot/alita/db/models"
)

// autoRow reads the env's chat's chat_raid_detection row straight from the database and
// reports whether there is one.
func (e *lockdownEnv) autoRow() (models.ChatRaidDetection, bool) {
	e.t.Helper()
	var rows []models.ChatRaidDetection
	if err := db.DB.Where("chat_id = ?", e.chat.Id).Find(&rows).Error; err != nil {
		e.t.Fatalf("read the raid detection row: %v", err)
	}
	if len(rows) == 0 {
		return models.ChatRaidDetection{}, false
	}
	return rows[0], true
}

// wantAutoRow fails unless the env's chat has a row with exactly these columns.
func (e *lockdownEnv) wantAutoRow(joinOn bool, joinN int, floodOn bool, floodN int) {
	e.t.Helper()
	row, ok := e.autoRow()
	if !ok {
		e.t.Fatal("no chat_raid_detection row, want one")
	}
	if row.JoinEnabled != joinOn || row.JoinThreshold != joinN || row.FloodEnabled != floodOn || row.FloodThreshold != floodN {
		e.t.Errorf("row = joins %v/%d, flood %v/%d; want joins %v/%d, flood %v/%d",
			row.JoinEnabled, row.JoinThreshold, row.FloodEnabled, row.FloodThreshold, joinOn, joinN, floodOn, floodN)
	}
}

// wantNoAutoRow fails when the env's chat has a chat_raid_detection row: a refusal
// saves nothing.
func (e *lockdownEnv) wantNoAutoRow() {
	e.t.Helper()
	if row, ok := e.autoRow(); ok {
		e.t.Errorf("a chat_raid_detection row exists (%+v), want none", row)
	}
}

func TestAutoLockdown(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		env := newLockdownEnv(t)
		raidCleanup(t, env.chat.Id)

		env.send(env.admin, "/autolockdown")

		env.wantReplyHas(
			staffMarker("lockdown_auto_title"),
			staffMarker("lockdown_auto_joins_on")+" 15",
			staffMarker("lockdown_auto_flood_on")+" 30",
		)
		env.wantNoAutoRow()
	})

	t.Run("joins 20 is followed by the next burst on any replica", func(t *testing.T) {
		env := newLockdownEnv(t)
		raidCleanup(t, env.chat.Id)

		env.send(env.admin, "/autolockdown joins 20")

		env.wantReplyHas(
			staffMarker("lockdown_auto_saved"),
			staffMarker("lockdown_auto_joins_on")+" 20",
			staffMarker("lockdown_auto_flood_on")+" 30",
		)
		env.wantAutoRow(true, 20, true, 30)

		// Another replica has nothing of this one's in-process cache.
		dbcache.ResetLocalForTest()

		env.selfJoins(19)
		if row := env.activeLockdown(); row != nil {
			t.Fatalf("a lockdown (kind %q) exists after 19 joins with the threshold at 20, want none", row.TriggerKind)
		}
		twentieth := env.newJoiner("Twentieth")
		env.join(twentieth, twentieth, "")

		row := env.activeLockdown()
		if row == nil {
			t.Fatal("no active lockdown after the 20th join in a minute")
		}
		if row.TriggerKind != models.LockdownTriggerJoinSurge || row.TriggerCount != 20 {
			t.Errorf("trigger = %q with count %d, want a join surge of 20", row.TriggerKind, row.TriggerCount)
		}
	})

	t.Run("joins off then on keeps the threshold", func(t *testing.T) {
		env := newLockdownEnv(t)
		raidCleanup(t, env.chat.Id)

		env.send(env.admin, "/autolockdown joins off")

		env.wantReplyHas(staffMarker("lockdown_auto_saved"), staffMarker("lockdown_auto_joins_off")+" 15")
		env.wantAutoRow(false, 15, true, 30)

		env.selfJoins(25)
		if row := env.activeLockdown(); row != nil {
			t.Fatalf("a lockdown (kind %q) exists after 25 joins with the join surge off, want none", row.TriggerKind)
		}

		env.send(env.admin, "/autolockdown joins on")

		env.wantReplyHas(staffMarker("lockdown_auto_saved"), staffMarker("lockdown_auto_joins_on")+" 15")
		env.wantAutoRow(true, 15, true, 30)

		next := env.newJoiner("Twenty Sixth")
		env.join(next, next, "")
		row := env.activeLockdown()
		if row == nil {
			t.Fatal("no active lockdown on the next join after the join surge was switched on again")
		}
		if row.TriggerKind != models.LockdownTriggerJoinSurge {
			t.Errorf("TriggerKind = %q, want %q", row.TriggerKind, models.LockdownTriggerJoinSurge)
		}
	})

	t.Run("flood off leaves the join surge on", func(t *testing.T) {
		env := newLockdownEnv(t)
		raidCleanup(t, env.chat.Id)

		env.send(env.admin, "/autolockdown flood off")

		env.wantReplyHas(staffMarker("lockdown_auto_saved"), staffMarker("lockdown_auto_flood_off")+" 30")
		env.wantAutoRow(true, 15, false, 30)

		recent := env.newMembers(4, 2*time.Hour)
		env.sendItems(recent, 30)
		if row := env.activeLockdown(); row != nil {
			t.Fatalf("a lockdown (kind %q) exists after a flood with the flood trigger off, want none", row.TriggerKind)
		}

		env.selfJoins(15)
		row := env.activeLockdown()
		if row == nil {
			t.Fatal("no active lockdown after 15 joins: switching the flood trigger off must leave the join surge on")
		}
		if row.TriggerKind != models.LockdownTriggerJoinSurge {
			t.Errorf("TriggerKind = %q, want %q", row.TriggerKind, models.LockdownTriggerJoinSurge)
		}
	})
}
