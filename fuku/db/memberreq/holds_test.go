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

// seedHold inserts a hold with explicit times straight into the database and returns it.
func seedHold(t *testing.T, chatID, userID int64, action int, created time.Time, punished *time.Time) models.ChatRequirementHold {
	t.Helper()
	hold := models.ChatRequirementHold{
		ChatID: chatID, UserID: userID, Rules: 1, Action: action, CreatedAt: created, PunishedAt: punished,
	}
	if err := db.DB.Create(&hold).Error; err != nil {
		t.Fatalf("seed hold: %v", err)
	}
	return hold
}

func TestRequirementHolds(t *testing.T) {
	ctx := context.Background()
	const userID = int64(21)
	msgDate := time.Now()
	ago := func(d time.Duration) time.Time { return msgDate.Add(-d) }
	at := func(ts time.Time) *time.Time { return &ts }

	t.Run("a claim stores every column", func(t *testing.T) {
		chatID := requirementsChat(t)
		hold := newHold(chatID, userID)
		hold.Rules = 5
		hold.Action = models.RequirementActionMute
		hold.MuteUntil = 777
		hold.WelcomeHeld = true
		if ok, err := ClaimHold(ctx, hold); err != nil || !ok {
			t.Fatalf("ClaimHold = (%v, %v)", ok, err)
		}
		got, err := GetHoldFresh(ctx, chatID, userID)
		if err != nil || got == nil {
			t.Fatalf("GetHoldFresh = (%v, %v)", got, err)
		}
		if got.ID != hold.ID || got.Rules != 5 || got.Action != 4 || got.MuteUntil != 777 || !got.WelcomeHeld ||
			got.NoticeMsgID != 0 || got.PunishedAt != nil || time.Since(got.CreatedAt) > time.Minute {
			t.Errorf("stored hold = %+v, want what the claim wrote, pending, created now", got)
		}
		byID, err := GetHoldByIDFresh(ctx, hold.ID)
		if err != nil || byID == nil || byID.UserID != userID {
			t.Errorf("GetHoldByIDFresh = (%v, %v), want the hold", byID, err)
		}
	})

	t.Run("gets return nil and no error when there is none", func(t *testing.T) {
		chatID := requirementsChat(t)
		if got, err := GetHoldFresh(ctx, chatID, userID); err != nil || got != nil {
			t.Errorf("GetHoldFresh = (%v, %v), want (nil, nil)", got, err)
		}
		if got, err := GetHoldByIDFresh(ctx, 1<<60); err != nil || got != nil {
			t.Errorf("GetHoldByIDFresh = (%v, %v), want (nil, nil)", got, err)
		}
	})

	t.Run("single column updates", func(t *testing.T) {
		chatID := requirementsChat(t)
		a := seedHold(t, chatID, userID, models.RequirementActionMute, ago(time.Hour), nil)
		b := seedHold(t, chatID, userID+1, models.RequirementActionMute, ago(time.Hour), nil)

		if ok, err := MarkHoldPunished(ctx, a.ID); err != nil || !ok {
			t.Fatalf("MarkHoldPunished = (%v, %v)", ok, err)
		}
		if got := storedHold(t, chatID, userID); got.PunishedAt == nil || got.NoticeMsgID != 0 || got.WelcomeHeld {
			t.Errorf("after MarkHoldPunished: %+v, want only punished_at set", got)
		}
		if got := storedHold(t, chatID, userID+1); got.PunishedAt != nil {
			t.Error("MarkHoldPunished changed another hold")
		}

		if ok, err := SetHoldNotice(ctx, a.ID, 4242); err != nil || !ok {
			t.Fatalf("SetHoldNotice = (%v, %v)", ok, err)
		}
		if got := storedHold(t, chatID, userID); got.NoticeMsgID != 4242 || got.WelcomeHeld || got.PunishedAt == nil {
			t.Errorf("after SetHoldNotice: %+v, want only the notice ID added", got)
		}
		if ok, err := SetHoldWelcomeHeld(ctx, a.ID); err != nil || !ok {
			t.Fatalf("SetHoldWelcomeHeld = (%v, %v)", ok, err)
		}
		if got := storedHold(t, chatID, userID); !got.WelcomeHeld || got.NoticeMsgID != 4242 {
			t.Errorf("after SetHoldWelcomeHeld: %+v, want welcome_held added and the notice kept", got)
		}
		if got := storedHold(t, chatID, userID+1); got.NoticeMsgID != 0 || got.WelcomeHeld {
			t.Error("a column update changed another hold")
		}
		if ok, err := SetHoldNotice(ctx, b.ID+1000000, 1); err != nil || ok {
			t.Errorf("SetHoldNotice of a missing ID = (%v, %v), want (false, nil)", ok, err)
		}
	})

	t.Run("backoff and revert", func(t *testing.T) {
		chatID := requirementsChat(t)
		pending := seedHold(t, chatID, userID, models.RequirementActionWarn, ago(time.Hour), nil)
		punished := seedHold(t, chatID, userID+1, models.RequirementActionMute, ago(time.Hour), at(ago(time.Hour)))

		if ok, err := BackoffHold(ctx, pending.ID); err != nil || !ok {
			t.Fatalf("BackoffHold of a pending hold = (%v, %v)", ok, err)
		}
		if got := storedHold(t, chatID, userID); time.Since(got.CreatedAt) > time.Minute || got.PunishedAt != nil {
			t.Errorf("pending hold after backoff: %+v, want created now and still pending", got)
		}
		if ok, err := BackoffHold(ctx, punished.ID); err != nil || ok {
			t.Errorf("BackoffHold of a punished hold = (%v, %v), want (false, nil)", ok, err)
		}
		if got := storedHold(t, chatID, userID+1); time.Since(got.CreatedAt) < 30*time.Minute || got.PunishedAt == nil {
			t.Errorf("punished hold after backoff: %+v, want it untouched", got)
		}

		if ok, err := RevertHoldToPending(ctx, punished.ID); err != nil || !ok {
			t.Fatalf("RevertHoldToPending of a punished hold = (%v, %v)", ok, err)
		}
		if got := storedHold(t, chatID, userID+1); got.PunishedAt != nil || time.Since(got.CreatedAt) > time.Minute {
			t.Errorf("reverted hold: %+v, want pending and created now", got)
		}
		old := seedHold(t, chatID, userID+2, models.RequirementActionWarn, ago(time.Hour), nil)
		if ok, err := RevertHoldToPending(ctx, old.ID); err != nil || ok {
			t.Errorf("RevertHoldToPending of a pending hold = (%v, %v), want (false, nil)", ok, err)
		}
		if got := storedHold(t, chatID, userID+2); time.Since(got.CreatedAt) < 30*time.Minute {
			t.Error("RevertHoldToPending refreshed a pending hold")
		}
	})

	t.Run("take and restore", func(t *testing.T) {
		chatID := requirementsChat(t)
		hold := seedHold(t, chatID, userID, models.RequirementActionMute, ago(time.Hour), at(ago(time.Hour)))
		if ok, err := SetHoldNotice(ctx, hold.ID, 555); err != nil || !ok {
			t.Fatalf("SetHoldNotice = (%v, %v)", ok, err)
		}

		if got, ok, err := TakeHoldByID(ctx, hold.ID, chatID+1, userID); err != nil || ok || got != nil {
			t.Errorf("take with another chat = (%v, %v, %v), want nothing", got, ok, err)
		}
		if got, ok, err := TakeHoldByID(ctx, hold.ID, chatID, userID+1); err != nil || ok || got != nil {
			t.Errorf("take with another user = (%v, %v, %v), want nothing", got, ok, err)
		}
		if n := holdCount(t, chatID, userID); n != 1 {
			t.Fatalf("a refused take removed the hold (%d rows)", n)
		}

		taken, ok, err := TakeHoldByID(ctx, hold.ID, chatID, userID)
		if err != nil || !ok || taken == nil {
			t.Fatalf("take = (%v, %v, %v), want the hold", taken, ok, err)
		}
		if taken.ID != hold.ID || taken.NoticeMsgID != 555 || taken.Action != 4 || taken.PunishedAt == nil {
			t.Errorf("taken hold = %+v, want the stored row", taken)
		}
		if n := holdCount(t, chatID, userID); n != 0 {
			t.Errorf("%d rows after a take, want 0", n)
		}
		if got, ok, err := TakeHoldByID(ctx, hold.ID, chatID, userID); err != nil || ok || got != nil {
			t.Errorf("second take = (%v, %v, %v), want nothing", got, ok, err)
		}

		if err := RestoreHold(ctx, taken); err != nil {
			t.Fatalf("RestoreHold: %v", err)
		}
		back := storedHold(t, chatID, userID)
		if back.ID != hold.ID || back.NoticeMsgID != 555 || back.Action != 4 || back.PunishedAt == nil || back.Rules != 1 {
			t.Errorf("restored hold = %+v, want the same ID and columns", back)
		}
		if err := RestoreHold(ctx, taken); err != nil {
			t.Errorf("second RestoreHold = %v, want nil", err)
		}
		if n := holdCount(t, chatID, userID); n != 1 {
			t.Errorf("%d rows after a second restore, want 1", n)
		}
	})

	t.Run("the clean predicate", func(t *testing.T) {
		chatID := requirementsChat(t)
		cases := []struct {
			name     string
			action   int
			created  time.Time
			punished *time.Time
			deleted  bool
		}{
			{"warn created long before the message", models.RequirementActionWarn, ago(10 * time.Second), nil, true},
			{"warn created within 2 s of the message", models.RequirementActionWarn, ago(time.Second), nil, false},
			{"warn created after the message", models.RequirementActionWarn, msgDate.Add(5 * time.Second), nil, false},
			{"mute punished long before the message", models.RequirementActionMute, ago(time.Minute), at(ago(10 * time.Second)), true},
			{"mute punished after the message", models.RequirementActionMute, ago(time.Minute), at(msgDate.Add(5 * time.Second)), false},
			{"mute punished within 2 s of the message", models.RequirementActionMute, ago(time.Minute), at(ago(time.Second)), false},
			{"pending mute older than StaleClaim", models.RequirementActionMute, ago(3 * time.Minute), nil, true},
			{"pending mute younger than StaleClaim", models.RequirementActionMute, ago(10 * time.Second), nil, false},
			{"kick created long before the message", models.RequirementActionKick, ago(time.Minute), at(ago(time.Minute)), true},
		}
		for i, tc := range cases {
			user := userID + int64(i)
			seedHold(t, chatID, user, tc.action, tc.created, tc.punished)
			deleted, err := ClearHoldOnClean(ctx, chatID, user, msgDate)
			if err != nil {
				t.Fatalf("%s: ClearHoldOnClean: %v", tc.name, err)
			}
			if deleted != tc.deleted {
				t.Errorf("%s: deleted = %v, want %v", tc.name, deleted, tc.deleted)
			}
			if gone := holdCount(t, chatID, user) == 0; gone != tc.deleted {
				t.Errorf("%s: hold gone = %v, want %v", tc.name, gone, tc.deleted)
			}
		}
		if deleted, err := ClearHoldOnClean(ctx, chatID, userID+500, msgDate); err != nil || deleted {
			t.Errorf("clean with no hold = (%v, %v), want (false, nil)", deleted, err)
		}
	})

	t.Run("clear all and take before a join", func(t *testing.T) {
		chatID := requirementsChat(t)
		otherChat := requirementsChat(t)
		seedHold(t, chatID, userID, models.RequirementActionWarn, ago(time.Hour), nil)
		seedHold(t, chatID, userID+1, models.RequirementActionWarn, ago(time.Hour), nil)
		seedHold(t, otherChat, userID, models.RequirementActionWarn, ago(time.Hour), nil)

		if n, err := ClearHolds(ctx, chatID, userID); err != nil || n != 1 {
			t.Fatalf("ClearHolds = (%d, %v), want (1, nil)", n, err)
		}
		if holdCount(t, chatID, userID) != 0 || holdCount(t, chatID, userID+1) != 1 || holdCount(t, otherChat, userID) != 1 {
			t.Error("ClearHolds removed a hold of another user or chat")
		}

		before := seedHold(t, chatID, userID+2, models.RequirementActionMute, ago(time.Hour), nil)
		got, err := TakeHoldsBefore(ctx, chatID, userID+2, msgDate)
		if err != nil || len(got) != 1 || got[0].ID != before.ID {
			t.Fatalf("TakeHoldsBefore = (%v, %v), want the older hold", got, err)
		}
		if holdCount(t, chatID, userID+2) != 0 {
			t.Error("TakeHoldsBefore left the hold behind")
		}
		seedHold(t, chatID, userID+3, models.RequirementActionMute, msgDate.Add(time.Minute), nil)
		got, err = TakeHoldsBefore(ctx, chatID, userID+3, msgDate)
		if err != nil || len(got) != 0 {
			t.Errorf("TakeHoldsBefore of a newer hold = (%v, %v), want nothing", got, err)
		}
		if holdCount(t, chatID, userID+3) != 1 {
			t.Error("TakeHoldsBefore took a hold created after the join")
		}
	})
}
