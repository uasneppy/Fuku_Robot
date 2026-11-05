//go:build testtools

package memberreq

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// holdCount returns how many holds exist for (chat, user), read straight from the database.
func holdCount(t *testing.T, chatID, userID int64) int64 {
	t.Helper()
	var n int64
	if err := db.DB.Model(&models.ChatRequirementHold{}).Where("chat_id = ? AND user_id = ?", chatID, userID).Count(&n).Error; err != nil {
		t.Fatalf("count holds: %v", err)
	}
	return n
}

// storedHold reads the single hold of (chat, user) straight from the database.
func storedHold(t *testing.T, chatID, userID int64) models.ChatRequirementHold {
	t.Helper()
	var row models.ChatRequirementHold
	if err := db.DB.Where("chat_id = ? AND user_id = ?", chatID, userID).First(&row).Error; err != nil {
		t.Fatalf("read hold of user %d in chat %d: %v", userID, chatID, err)
	}
	return row
}

// newHold returns a pending hold for (chat, user) for a last-name warn.
func newHold(chatID, userID int64) *models.ChatRequirementHold {
	return &models.ChatRequirementHold{
		ChatID:    chatID,
		UserID:    userID,
		Rules:     1,
		Action:    models.RequirementActionWarn,
		CreatedAt: time.Now(),
	}
}

// raceClaims runs workers copies of fn behind a start barrier and returns how many
// returned won, how many lost and how many errored.
func raceClaims(workers int, fn func() (bool, error)) (won, lost, failed int32) {
	var (
		wg      sync.WaitGroup
		w, l, f atomic.Int32
		start   = make(chan struct{})
	)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			ok, err := fn()
			switch {
			case err != nil:
				f.Add(1)
			case ok:
				w.Add(1)
			default:
				l.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()
	return w.Load(), l.Load(), f.Load()
}

func TestRequirementHoldClaimConcurrent(t *testing.T) {
	ctx := context.Background()
	const workers = 16

	t.Run("claim", func(t *testing.T) {
		chatID := requirementsChat(t)
		const userID = int64(7)
		var ids sync.Map
		won, lost, failed := raceClaims(workers, func() (bool, error) {
			hold := newHold(chatID, userID)
			ok, err := ClaimHold(ctx, hold)
			if ok {
				ids.Store(hold.ID, true)
			}
			return ok, err
		})
		if failed != 0 {
			t.Fatalf("%d claims returned an error", failed)
		}
		if won != 1 || lost != workers-1 {
			t.Fatalf("winners = %d, losers = %d, want 1 and %d", won, lost, workers-1)
		}
		if n := holdCount(t, chatID, userID); n != 1 {
			t.Fatalf("%d rows exist, want 1", n)
		}
		stored := storedHold(t, chatID, userID)
		if stored.ID <= 0 {
			t.Fatalf("stored ID = %d, want positive", stored.ID)
		}
		if _, ok := ids.Load(stored.ID); !ok {
			t.Errorf("the winner's hold carries an ID that is not the stored row's %d", stored.ID)
		}
	})

	t.Run("reclaim", func(t *testing.T) {
		chatID := requirementsChat(t)
		const userID = int64(8)
		if ok, err := ClaimHold(ctx, newHold(chatID, userID)); err != nil || !ok {
			t.Fatalf("seed claim = (%v, %v), want (true, nil)", ok, err)
		}
		fresh := func() *models.ChatRequirementHold {
			h := newHold(chatID, userID)
			h.Rules = 2
			h.Action = models.RequirementActionMute
			h.MuteUntil = 12345
			h.WelcomeHeld = true
			return h
		}

		// A pending hold younger than StaleClaim is never reclaimed.
		if won, _, err := ReclaimStaleHold(ctx, fresh()); err != nil || won {
			t.Fatalf("reclaim of a young pending hold = (%v, %v), want (false, nil)", won, err)
		}

		old := time.Now().Add(-3 * time.Minute)
		if err := db.DB.Model(&models.ChatRequirementHold{}).Where("chat_id = ? AND user_id = ?", chatID, userID).
			Updates(map[string]any{"created_at": old, "notice_msg_id": 99}).Error; err != nil {
			t.Fatalf("age the hold: %v", err)
		}
		seeded := storedHold(t, chatID, userID)

		var gotID atomic.Int64
		won, lost, failed := raceClaims(workers, func() (bool, error) {
			ok, id, err := ReclaimStaleHold(ctx, fresh())
			if ok {
				gotID.Store(id)
			}
			return ok, err
		})
		if failed != 0 {
			t.Fatalf("%d reclaims returned an error", failed)
		}
		if won != 1 || lost != workers-1 {
			t.Fatalf("winners = %d, losers = %d, want 1 and %d", won, lost, workers-1)
		}
		if gotID.Load() != seeded.ID {
			t.Errorf("reclaim returned ID %d, want the stored %d", gotID.Load(), seeded.ID)
		}
		got := storedHold(t, chatID, userID)
		if time.Since(got.CreatedAt) > time.Minute {
			t.Errorf("created_at = %v, want now", got.CreatedAt)
		}
		if got.Rules != 2 || got.Action != models.RequirementActionMute || got.MuteUntil != 12345 || !got.WelcomeHeld || got.NoticeMsgID != 0 {
			t.Errorf("reclaimed hold = %+v, want the fresh rules, action, mute_until, welcome_held and notice 0", got)
		}
		if n := holdCount(t, chatID, userID); n != 1 {
			t.Errorf("%d rows exist after the reclaim, want 1", n)
		}

		// A punished hold is never reclaimed, however old.
		if err := db.DB.Model(&models.ChatRequirementHold{}).Where("id = ?", got.ID).
			Updates(map[string]any{"created_at": old, "punished_at": old}).Error; err != nil {
			t.Fatalf("punish the hold: %v", err)
		}
		if won, _, err := ReclaimStaleHold(ctx, fresh()); err != nil || won {
			t.Errorf("reclaim of a punished hold = (%v, %v), want (false, nil)", won, err)
		}
	})
}
