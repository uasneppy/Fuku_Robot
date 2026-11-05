package greetings

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

func countGreeted(t *testing.T, chatID, userID int64) int64 {
	t.Helper()
	var n int64
	if err := db.DB.Model(&models.GreetedMember{}).
		Where("chat_id = ? AND user_id = ?", chatID, userID).Count(&n).Error; err != nil {
		t.Fatalf("count greeted members: %v", err)
	}
	return n
}

func cleanupGreeted(t *testing.T, chatID int64) {
	t.Helper()
	t.Cleanup(func() {
		db.DB.Where("chat_id = ?", chatID).Delete(&models.GreetedMember{})
	})
}

func mustClaim(t *testing.T, chatID, userID int64) bool {
	t.Helper()
	claimed, err := ClaimGreeting(chatID, userID)
	if err != nil {
		t.Fatalf("ClaimGreeting(%d, %d) error = %v", chatID, userID, err)
	}
	return claimed
}

func TestGreetedMemberClaim(t *testing.T) {
	skipIfNoDb(t)

	chatA := time.Now().UnixNano()
	chatB := chatA + 1
	cleanupGreeted(t, chatA)
	cleanupGreeted(t, chatB)
	const user = int64(1)

	if !mustClaim(t, chatA, user) {
		t.Fatal("first claim of a member should win")
	}
	var row models.GreetedMember
	if err := db.DB.Where("chat_id = ? AND user_id = ?", chatA, user).First(&row).Error; err != nil {
		t.Fatalf("read claimed row: %v", err)
	}
	if row.CreatedAt.IsZero() {
		t.Fatal("claimed row has no created_at")
	}
	if mustClaim(t, chatA, user) {
		t.Fatal("second claim of the same member should lose")
	}
	if n := countGreeted(t, chatA, user); n != 1 {
		t.Fatalf("expected one row, got %d", n)
	}

	if !mustClaim(t, chatB, user) {
		t.Fatal("a claim in another chat is independent")
	}

	if err := ReleaseGreeting(chatA, user); err != nil {
		t.Fatalf("ReleaseGreeting() error = %v", err)
	}
	if n := countGreeted(t, chatA, user); n != 0 {
		t.Fatalf("release left %d rows", n)
	}
	if !mustClaim(t, chatA, user) {
		t.Fatal("a released member should be claimable again")
	}
	if n := countGreeted(t, chatB, user); n != 1 {
		t.Fatalf("releasing chat A touched chat B, rows = %d", n)
	}

	if err := ReleaseGreeting(chatA, user+100); err != nil {
		t.Fatalf("releasing an absent row should not error, got %v", err)
	}
}

func TestGreetedMemberClaimConcurrent(t *testing.T) {
	skipIfNoDb(t)

	chatID := time.Now().UnixNano()
	cleanupGreeted(t, chatID)
	const user = int64(7)
	const workers = 16

	var (
		wg      sync.WaitGroup
		winners atomic.Int32
		losers  atomic.Int32
		failed  atomic.Int32
	)
	start := make(chan struct{})
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			claimed, err := ClaimGreeting(chatID, user)
			switch {
			case err != nil:
				failed.Add(1)
			case claimed:
				winners.Add(1)
			default:
				losers.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()

	if failed.Load() != 0 {
		t.Fatalf("%d claims returned an error", failed.Load())
	}
	if winners.Load() != 1 || losers.Load() != workers-1 {
		t.Fatalf("winners = %d, losers = %d, want 1 and %d", winners.Load(), losers.Load(), workers-1)
	}
	if n := countGreeted(t, chatID, user); n != 1 {
		t.Fatalf("expected one row, got %d", n)
	}
}
