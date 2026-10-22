//go:build testtools

package lockdown

import (
	"testing"
	"time"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/models"
)

// setLiftStarted writes a lockdown row's lift_started_at directly, so a test can place a
// lift at a chosen time without waiting.
func setLiftStarted(t *testing.T, id uint, at time.Time) {
	t.Helper()
	err := db.DB.Model(&models.ChatLockdown{}).Where("id = ?", id).UpdateColumn("lift_started_at", at.UTC()).Error
	if err != nil {
		t.Fatalf("set lift_started_at of lockdown %d: %v", id, err)
	}
}

func TestRecentlyLiftedFresh(t *testing.T) {
	since := func() time.Time { return time.Now().UTC().Add(-10 * time.Minute) }

	t.Run("a lift five minutes ago", func(t *testing.T) {
		chat := uniqueLockdownChatID(t)
		cleanupLockdowns(t, chat)
		row := mustStart(t, chat)
		if ok, err := BeginLift(row.ID, 42, "Admin", false); err != nil || !ok {
			t.Fatalf("BeginLift = %v, %v, want true, nil", ok, err)
		}
		setLiftStarted(t, row.ID, time.Now().Add(-5*time.Minute))

		lifted, err := RecentlyLiftedFresh(chat, since())
		if err != nil {
			t.Fatalf("RecentlyLiftedFresh error = %v", err)
		}
		if !lifted {
			t.Error("RecentlyLiftedFresh = false for a lift 5 minutes ago and a 10 minute window, want true")
		}
	})

	t.Run("a lift fifteen minutes ago", func(t *testing.T) {
		chat := uniqueLockdownChatID(t)
		cleanupLockdowns(t, chat)
		row := mustStart(t, chat)
		if ok, err := BeginLift(row.ID, 42, "Admin", false); err != nil || !ok {
			t.Fatalf("BeginLift = %v, %v, want true, nil", ok, err)
		}
		setLiftStarted(t, row.ID, time.Now().Add(-15*time.Minute))

		lifted, err := RecentlyLiftedFresh(chat, since())
		if err != nil {
			t.Fatalf("RecentlyLiftedFresh error = %v", err)
		}
		if lifted {
			t.Error("RecentlyLiftedFresh = true for a lift 15 minutes ago and a 10 minute window, want false")
		}
	})

	t.Run("an active lockdown has no lift", func(t *testing.T) {
		chat := uniqueLockdownChatID(t)
		cleanupLockdowns(t, chat)
		mustStart(t, chat)

		lifted, err := RecentlyLiftedFresh(chat, since())
		if err != nil {
			t.Fatalf("RecentlyLiftedFresh error = %v", err)
		}
		if lifted {
			t.Error("RecentlyLiftedFresh = true for a chat whose lockdown is active, want false")
		}
	})

	t.Run("a finished lift counts too", func(t *testing.T) {
		chat := uniqueLockdownChatID(t)
		cleanupLockdowns(t, chat)
		row := mustStart(t, chat)
		if ok, err := BeginLift(row.ID, 42, "Admin", false); err != nil || !ok {
			t.Fatalf("BeginLift = %v, %v, want true, nil", ok, err)
		}
		if ok, err := FinishLift(row.ID); err != nil || !ok {
			t.Fatalf("FinishLift = %v, %v, want true, nil", ok, err)
		}

		lifted, err := RecentlyLiftedFresh(chat, since())
		if err != nil {
			t.Fatalf("RecentlyLiftedFresh error = %v", err)
		}
		if !lifted {
			t.Error("RecentlyLiftedFresh = false for a lockdown lifted just now, want true")
		}
	})

	t.Run("another chat's lift does not count", func(t *testing.T) {
		chatA := uniqueLockdownChatID(t)
		chatB := uniqueLockdownChatID(t)
		cleanupLockdowns(t, chatA, chatB)
		rowB := mustStart(t, chatB)
		if ok, err := BeginLift(rowB.ID, 42, "Admin", false); err != nil || !ok {
			t.Fatalf("BeginLift = %v, %v, want true, nil", ok, err)
		}
		setLiftStarted(t, rowB.ID, time.Now().Add(-5*time.Minute))

		liftedA, err := RecentlyLiftedFresh(chatA, since())
		if err != nil {
			t.Fatalf("RecentlyLiftedFresh(A) error = %v", err)
		}
		if liftedA {
			t.Error("RecentlyLiftedFresh(A) = true, want false: only chat B was lifted")
		}
		liftedB, err := RecentlyLiftedFresh(chatB, since())
		if err != nil {
			t.Fatalf("RecentlyLiftedFresh(B) error = %v", err)
		}
		if !liftedB {
			t.Error("RecentlyLiftedFresh(B) = false, want true")
		}
	})
}
