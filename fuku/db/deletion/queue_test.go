//go:build testtools

package deletion

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// storedQueue reads the queue rows of a chat straight from the database, oldest first.
func storedQueue(t *testing.T, chatID int64) []models.ScheduledDeletion {
	t.Helper()
	var rows []models.ScheduledDeletion
	if err := db.DB.Where("chat_id = ?", chatID).Order("message_id").Find(&rows).Error; err != nil {
		t.Fatalf("read the queue rows: %v", err)
	}
	return rows
}

// seedDue inserts count pending rows of a chat directly, all due at due.
func seedDue(t *testing.T, chatID int64, firstMessage int64, count int, due time.Time) {
	t.Helper()
	due = due.UTC().Truncate(time.Second)
	for i := range count {
		row := models.ScheduledDeletion{
			ChatID: chatID, MessageID: firstMessage + int64(i), Kind: KindBotWelcome,
			SentAt: due, DeleteAt: due, State: models.ScheduledDeletionPending, CreatedAt: due,
		}
		if err := db.DB.Create(&row).Error; err != nil {
			t.Fatalf("seed row %d: %v", i, err)
		}
	}
}

func rowIDs(rows []models.ScheduledDeletion) []int64 {
	ids := make([]int64, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
	}
	return ids
}

func TestScheduleRules(t *testing.T) {
	ctx := context.Background()
	sent := time.Date(2026, 11, 10, 12, 0, 0, 123456789, time.FixedZone("x", 3*3600))

	t.Run("creates a pending row", func(t *testing.T) {
		chatID := deletionChat(t)
		wrote, err := Schedule(ctx, Entry{ChatID: chatID, MessageID: 11, Kind: KindBotWelcome, SentAt: sent, DelaySecs: 90})
		if err != nil || !wrote {
			t.Fatalf("Schedule = (%v, %v), want (true, nil)", wrote, err)
		}
		rows := storedQueue(t, chatID)
		if len(rows) != 1 {
			t.Fatalf("%d rows, want 1", len(rows))
		}
		row := rows[0]
		wantSent := sent.UTC().Truncate(time.Second)
		if !row.SentAt.Equal(wantSent) {
			t.Errorf("sent_at = %v, want %v (UTC, whole seconds)", row.SentAt, wantSent)
		}
		if !row.DeleteAt.Equal(wantSent.Add(90 * time.Second)) {
			t.Errorf("delete_at = %v, want sent_at + 90s", row.DeleteAt)
		}
		if row.State != models.ScheduledDeletionPending || row.ClaimToken != "" || row.Attempts != 0 || row.ClaimedAt != nil {
			t.Errorf("row = %+v, want pending with no token, no attempts and no claim time", row)
		}
		if row.Kind != KindBotWelcome {
			t.Errorf("kind = %q", row.Kind)
		}
	})

	t.Run("delay zero is due at once", func(t *testing.T) {
		chatID := deletionChat(t)
		if wrote, err := Schedule(ctx, Entry{ChatID: chatID, MessageID: 12, Kind: KindPinned, SentAt: sent, DelaySecs: 0}); err != nil || !wrote {
			t.Fatalf("Schedule = (%v, %v)", wrote, err)
		}
		row := storedQueue(t, chatID)[0]
		if !row.DeleteAt.Equal(row.SentAt) {
			t.Errorf("delete_at = %v, want sent_at %v", row.DeleteAt, row.SentAt)
		}
	})

	t.Run("refused before any write", func(t *testing.T) {
		chatID := deletionChat(t)
		cases := []struct {
			name string
			e    Entry
			want error
		}{
			{"negative delay", Entry{ChatID: chatID, MessageID: 1, Kind: KindPinned, SentAt: sent, DelaySecs: -1}, ErrDelayOutOfRange},
			{"delay over a day", Entry{ChatID: chatID, MessageID: 2, Kind: KindPinned, SentAt: sent, DelaySecs: MaxDelaySecs + 1}, ErrDelayOutOfRange},
			{"unknown kind", Entry{ChatID: chatID, MessageID: 3, Kind: "service:nope", SentAt: sent, DelaySecs: 5}, ErrUnknownKind},
			{"zero chat", Entry{ChatID: 0, MessageID: 4, Kind: KindPinned, SentAt: sent, DelaySecs: 5}, ErrInvalidEntry},
			{"zero message", Entry{ChatID: chatID, MessageID: 0, Kind: KindPinned, SentAt: sent, DelaySecs: 5}, ErrInvalidEntry},
			{"no sent time", Entry{ChatID: chatID, MessageID: 6, Kind: KindPinned, DelaySecs: 5}, ErrInvalidEntry},
		}
		for _, tc := range cases {
			wrote, err := Schedule(ctx, tc.e)
			if wrote || !errors.Is(err, tc.want) {
				t.Errorf("%s: Schedule = (%v, %v), want (false, %v)", tc.name, wrote, err, tc.want)
			}
		}
		if n := len(storedQueue(t, chatID)); n != 0 {
			t.Fatalf("%d rows exist after refused schedules, want 0", n)
		}
		if wrote, err := Schedule(ctx, Entry{ChatID: chatID, MessageID: 7, Kind: KindPinned, SentAt: sent, DelaySecs: MaxDelaySecs}); err != nil || !wrote {
			t.Errorf("the largest delay: (%v, %v)", wrote, err)
		}
	})
}

// TestScheduleKeepsEarliest runs on SQLite in make test and on PostgreSQL in the integrity
// target: scheduling one (chat, message) again never moves its time later, and never
// rewrites the kind or the sent time of the first insert.
func TestScheduleKeepsEarliest(t *testing.T) {
	ctx := context.Background()
	sent := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)

	t.Run("sequential", func(t *testing.T) {
		chatID := deletionChat(t)
		for _, delay := range []int{3600, 60, 7200} {
			if wrote, err := Schedule(ctx, Entry{ChatID: chatID, MessageID: 5, Kind: KindBotWelcome, SentAt: sent, DelaySecs: delay}); err != nil || !wrote {
				t.Fatalf("Schedule(%d) = (%v, %v)", delay, wrote, err)
			}
		}
		rows := storedQueue(t, chatID)
		if len(rows) != 1 {
			t.Fatalf("%d rows, want 1", len(rows))
		}
		if !rows[0].DeleteAt.Equal(sent.Add(time.Minute)) {
			t.Errorf("delete_at = %v, want sent_at + 1 minute", rows[0].DeleteAt)
		}
	})

	t.Run("kind and sent time of the first insert stay", func(t *testing.T) {
		chatID := deletionChat(t)
		if _, err := Schedule(ctx, Entry{ChatID: chatID, MessageID: 6, Kind: KindBotWelcome, SentAt: sent, DelaySecs: 600}); err != nil {
			t.Fatal(err)
		}
		later := sent.Add(5 * time.Minute)
		if _, err := Schedule(ctx, Entry{ChatID: chatID, MessageID: 6, Kind: KindBotRules, SentAt: later, DelaySecs: 3600}); err != nil {
			t.Fatal(err)
		}
		row := storedQueue(t, chatID)[0]
		if row.Kind != KindBotWelcome || !row.SentAt.Equal(sent) {
			t.Errorf("row = kind %q sent %v, want the first insert's %q %v", row.Kind, row.SentAt, KindBotWelcome, sent)
		}
		// The second call's own time (later + 1h) is after the stored one, so the stored one stays.
		if !row.DeleteAt.Equal(sent.Add(10 * time.Minute)) {
			t.Errorf("delete_at = %v, want the earlier sent_at + 600s", row.DeleteAt)
		}
	})

	t.Run("concurrent callers keep the minimum", func(t *testing.T) {
		chatID := deletionChat(t)
		delays := []int{900, 4000, 30, 7000, 120, 86400, 45, 600}
		var wg sync.WaitGroup
		errs := make(chan error, len(delays))
		start := make(chan struct{})
		for _, delay := range delays {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				_, err := Schedule(ctx, Entry{ChatID: chatID, MessageID: 9, Kind: KindBotGoodbye, SentAt: sent, DelaySecs: delay})
				errs <- err
			}()
		}
		close(start)
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatalf("a concurrent Schedule failed: %v", err)
			}
		}
		rows := storedQueue(t, chatID)
		if len(rows) != 1 {
			t.Fatalf("%d rows, want 1", len(rows))
		}
		if !rows[0].DeleteAt.Equal(sent.Add(30 * time.Second)) {
			t.Errorf("delete_at = %v, want sent_at + 30s", rows[0].DeleteAt)
		}
	})
}

func TestScheduleRefusesStaffGroup(t *testing.T) {
	ctx := context.Background()
	sent := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)

	t.Run("a Staff Group is refused", func(t *testing.T) {
		chatID := deletionChat(t)
		if err := db.DB.Create(&models.StaffGroup{ChatID: chatID, OwnerUserID: 1, Title: "staff"}).Error; err != nil {
			t.Fatal(err)
		}
		wrote, err := Schedule(ctx, Entry{ChatID: chatID, MessageID: 1, Kind: KindBotWelcome, SentAt: sent, DelaySecs: 60})
		if wrote || err != nil {
			t.Fatalf("Schedule = (%v, %v), want (false, nil)", wrote, err)
		}
		if n := len(storedQueue(t, chatID)); n != 0 {
			t.Fatalf("%d rows written for a Staff Group", n)
		}
	})

	t.Run("a failed lookup writes nothing", func(t *testing.T) {
		chatID := deletionChat(t)
		boom := errors.New("staff read failed")
		previous := staffGroupLookup
		staffGroupLookup = func(int64) (*models.StaffGroup, error) { return nil, boom }
		t.Cleanup(func() { staffGroupLookup = previous })
		wrote, err := Schedule(ctx, Entry{ChatID: chatID, MessageID: 1, Kind: KindBotWelcome, SentAt: sent, DelaySecs: 60})
		if wrote || !errors.Is(err, boom) {
			t.Fatalf("Schedule = (%v, %v), want (false, the lookup error)", wrote, err)
		}
		if n := len(storedQueue(t, chatID)); n != 0 {
			t.Fatalf("%d rows written after a failed lookup", n)
		}
	})

	t.Run("a chat with no staff row is written", func(t *testing.T) {
		chatID := deletionChat(t)
		if wrote, err := Schedule(ctx, Entry{ChatID: chatID, MessageID: 1, Kind: KindBotWelcome, SentAt: sent, DelaySecs: 60}); err != nil || !wrote {
			t.Fatalf("Schedule = (%v, %v), want (true, nil)", wrote, err)
		}
	})
}

// TestClaimBatchOneWinner runs on SQLite in make test and on PostgreSQL in the integrity
// target: concurrent claimers each win a disjoint set of rows and together drain the queue.
func TestClaimBatchOneWinner(t *testing.T) {
	ctx := context.Background()
	const (
		rowCount = 60
		workers  = 8
		limit    = 20
	)
	chatID := deletionChat(t)
	now := time.Now().UTC().Truncate(time.Second)
	seedDue(t, chatID, 1000, rowCount, now.Add(-time.Hour))
	future := models.ScheduledDeletion{
		ChatID: chatID, MessageID: 5000, Kind: KindBotWelcome, SentAt: now, DeleteAt: now.Add(time.Hour),
		State: models.ScheduledDeletionPending, CreatedAt: now,
	}
	if err := db.DB.Create(&future).Error; err != nil {
		t.Fatal(err)
	}

	var (
		mu      sync.Mutex
		claimed = map[int64]string{}
		dupes   []int64
		wg      sync.WaitGroup
		failure error
		start   = make(chan struct{})
	)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for {
				token := NewClaimToken()
				rows, err := ClaimDue(ctx, now, limit, token)
				mu.Lock()
				if err != nil && failure == nil {
					failure = err
				}
				for _, row := range rows {
					if _, taken := claimed[row.ID]; taken {
						dupes = append(dupes, row.ID)
					}
					claimed[row.ID] = token
					if row.State != models.ScheduledDeletionActing || row.ClaimToken != token || row.ClaimedAt == nil {
						failure = errors.New("a claimed row is not acting with the caller's token and claim time")
					}
					if row.MessageID == future.MessageID {
						failure = errors.New("a not-yet-due row was claimed")
					}
				}
				mu.Unlock()
				if err != nil || len(rows) == 0 {
					return
				}
			}
		}()
	}
	close(start)
	wg.Wait()

	if failure != nil {
		t.Fatalf("a claimer failed: %v", failure)
	}
	if len(dupes) != 0 {
		t.Fatalf("rows returned to two callers: %v", dupes)
	}
	if len(claimed) != rowCount {
		t.Fatalf("%d rows claimed in all, want %d", len(claimed), rowCount)
	}
	pending := 0
	for _, row := range storedQueue(t, chatID) {
		if row.MessageID == future.MessageID {
			if row.State != models.ScheduledDeletionPending {
				t.Errorf("the not-yet-due row is %q, want pending", row.State)
			}
			pending++
		}
	}
	if pending != 1 {
		t.Errorf("the not-yet-due row is missing")
	}
}

func TestCompleteIsTokenGuarded(t *testing.T) {
	ctx := context.Background()
	chatID := deletionChat(t)
	now := time.Now().UTC().Truncate(time.Second)
	seedDue(t, chatID, 1, 4, now.Add(-time.Minute))

	tokenA := NewClaimToken()
	rows, err := ClaimDue(ctx, now, 2, tokenA)
	if err != nil || len(rows) != 2 {
		t.Fatalf("claim A = %d rows, %v", len(rows), err)
	}
	idsA := rowIDs(rows)

	// Another token completes nothing.
	if err := Complete(ctx, NewClaimToken(), idsA); err != nil {
		t.Fatal(err)
	}
	if n := len(storedQueue(t, chatID)); n != 4 {
		t.Fatalf("%d rows after a foreign Complete, want 4", n)
	}
	// An empty token completes nothing either (a pending row has an empty token).
	if err := Complete(ctx, "", rowIDs(storedQueue(t, chatID))); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("Complete with an empty token = %v, want ErrInvalidToken", err)
	}
	if n := len(storedQueue(t, chatID)); n != 4 {
		t.Fatalf("%d rows after an empty-token Complete, want 4", n)
	}

	// The right token removes exactly the listed rows it holds.
	if err := Complete(ctx, tokenA, idsA[:1]); err != nil {
		t.Fatal(err)
	}
	if n := len(storedQueue(t, chatID)); n != 3 {
		t.Fatalf("%d rows after completing one, want 3", n)
	}

	// A stale claim handed on to a second caller is not removed by the first one's late Complete.
	if _, err := ReleaseStale(ctx, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	tokenB := NewClaimToken()
	claimedB, err := ClaimDue(ctx, now, 10, tokenB)
	if err != nil || len(claimedB) != 3 {
		t.Fatalf("claim B = %d rows, %v, want the 3 left", len(claimedB), err)
	}
	if err := Complete(ctx, tokenA, idsA[1:]); err != nil {
		t.Fatal(err)
	}
	if n := len(storedQueue(t, chatID)); n != 3 {
		t.Fatalf("%d rows after the late Complete of the first claim, want 3", n)
	}
	if err := Complete(ctx, tokenB, rowIDs(claimedB)); err != nil {
		t.Fatal(err)
	}
	if n := len(storedQueue(t, chatID)); n != 0 {
		t.Fatalf("%d rows after the second claim completed, want 0", n)
	}
}

func TestReleaseStaleClaims(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	t.Run("release without counting", func(t *testing.T) {
		chatID := deletionChat(t)
		seedDue(t, chatID, 1, 2, now.Add(-time.Minute))
		token := NewClaimToken()
		rows, err := ClaimDue(ctx, now, 10, token)
		if err != nil || len(rows) != 2 {
			t.Fatalf("claim = %d, %v", len(rows), err)
		}
		if err := Release(ctx, token, rowIDs(rows), false, 3, now.Add(time.Minute)); err != nil {
			t.Fatal(err)
		}
		for _, row := range storedQueue(t, chatID) {
			if row.State != models.ScheduledDeletionPending || row.ClaimToken != "" || row.ClaimedAt != nil || row.Attempts != 0 {
				t.Errorf("released row = %+v, want pending, no token, no claim time, 0 attempts", row)
			}
		}
		// A token that does not hold the rows releases nothing.
		rows, err = ClaimDue(ctx, now, 10, NewClaimToken())
		if err != nil || len(rows) != 2 {
			t.Fatalf("second claim = %d, %v", len(rows), err)
		}
		if err := Release(ctx, token, rowIDs(rows), true, 3, now.Add(time.Minute)); err != nil {
			t.Fatal(err)
		}
		for _, row := range storedQueue(t, chatID) {
			if row.State != models.ScheduledDeletionActing || row.Attempts != 0 {
				t.Errorf("a foreign Release changed %+v", row)
			}
		}
	})

	t.Run("the third counted release drops the row", func(t *testing.T) {
		chatID := deletionChat(t)
		seedDue(t, chatID, 1, 1, now.Add(-time.Minute))
		for attempt := int16(1); attempt <= 3; attempt++ {
			token := NewClaimToken()
			// A counted release pushes delete_at to now+1m, so each try claims after that.
			rows, err := ClaimDue(ctx, now.Add(2*time.Minute), 10, token)
			if err != nil || len(rows) != 1 {
				t.Fatalf("claim %d = %d rows, %v", attempt, len(rows), err)
			}
			if err := Release(ctx, token, rowIDs(rows), true, 3, now.Add(time.Minute)); err != nil {
				t.Fatal(err)
			}
			stored := storedQueue(t, chatID)
			if attempt < 3 {
				if len(stored) != 1 || stored[0].Attempts != attempt || stored[0].State != models.ScheduledDeletionPending {
					t.Fatalf("after counted release %d: %+v, want one pending row with %d attempts", attempt, stored, attempt)
				}
			} else if len(stored) != 0 {
				t.Fatalf("after the third counted release %d rows remain, want 0", len(stored))
			}
		}
	})

	t.Run("a counted release moves delete_at forward, an uncounted one leaves it", func(t *testing.T) {
		chatID := deletionChat(t)
		due := now.Add(-time.Hour)
		seedDue(t, chatID, 1, 2, due)
		retryAt := now.Add(90 * time.Second)

		// Uncounted: back to pending, delete_at exactly as it was, due again at once.
		token := NewClaimToken()
		rows, err := ClaimDue(ctx, now, 10, token)
		if err != nil || len(rows) != 2 {
			t.Fatalf("claim = %d, %v", len(rows), err)
		}
		if err := Release(ctx, token, rowIDs(rows), false, 3, retryAt); err != nil {
			t.Fatal(err)
		}
		for _, row := range storedQueue(t, chatID) {
			if !row.DeleteAt.Equal(due) {
				t.Errorf("an uncounted release moved delete_at to %v, want %v", row.DeleteAt, due)
			}
		}

		// Counted: delete_at is retryAt, so the rows are not due before it and are due at it.
		token = NewClaimToken()
		rows, err = ClaimDue(ctx, now, 10, token)
		if err != nil || len(rows) != 2 {
			t.Fatalf("second claim = %d, %v", len(rows), err)
		}
		if err := Release(ctx, token, rowIDs(rows), true, 3, retryAt.Add(500*time.Millisecond)); err != nil {
			t.Fatal(err)
		}
		for _, row := range storedQueue(t, chatID) {
			if !row.DeleteAt.Equal(retryAt) {
				t.Errorf("a counted release set delete_at to %v, want %v (UTC, whole seconds)", row.DeleteAt, retryAt)
			}
			if row.Attempts != 1 || row.State != models.ScheduledDeletionPending {
				t.Errorf("counted row = %+v, want pending with 1 attempt", row)
			}
		}
		if early, err := ClaimDue(ctx, retryAt.Add(-time.Second), 10, NewClaimToken()); err != nil || len(early) != 0 {
			t.Fatalf("claim one second before the retry time = %d rows, %v, want none", len(early), err)
		}
		if ready, err := ClaimDue(ctx, retryAt, 10, NewClaimToken()); err != nil || len(ready) != 2 {
			t.Fatalf("claim at the retry time = %d rows, %v, want 2", len(ready), err)
		}
	})

	t.Run("a counted release needs a retry time", func(t *testing.T) {
		if err := Release(ctx, NewClaimToken(), []int64{1}, true, 3, time.Time{}); !errors.Is(err, ErrInvalidRetryAt) {
			t.Fatalf("counted Release with no retry time = %v, want ErrInvalidRetryAt", err)
		}
	})

	t.Run("a counted release needs a positive maximum", func(t *testing.T) {
		if err := Release(ctx, NewClaimToken(), []int64{1}, true, 0, now.Add(time.Minute)); err == nil {
			t.Fatal("Release counted with a maximum of 0 and returned no error")
		}
	})

	t.Run("stale claims", func(t *testing.T) {
		chatID := deletionChat(t)
		seedDue(t, chatID, 1, 3, now.Add(-time.Minute))
		oldToken := NewClaimToken()
		old, err := ClaimDue(ctx, now, 2, oldToken)
		if err != nil || len(old) != 2 {
			t.Fatalf("claim old = %d, %v", len(old), err)
		}
		// Age the first two claims to 5 minutes ago; the third is claimed now.
		if err := db.DB.Model(&models.ScheduledDeletion{}).Where("claim_token = ?", oldToken).
			Update("claimed_at", now.Add(-5*time.Minute)).Error; err != nil {
			t.Fatal(err)
		}
		fresh, err := ClaimDue(ctx, now, 2, NewClaimToken())
		if err != nil || len(fresh) != 1 {
			t.Fatalf("claim fresh = %d, %v", len(fresh), err)
		}
		released, err := ReleaseStale(ctx, now.Add(-2*time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		if released < 2 {
			t.Fatalf("ReleaseStale returned %d, want at least the 2 old claims", released)
		}
		byMessage := map[int64]models.ScheduledDeletion{}
		for _, row := range storedQueue(t, chatID) {
			byMessage[row.MessageID] = row
		}
		var states []string
		for _, row := range byMessage {
			states = append(states, row.State)
			if row.Attempts != 0 {
				t.Errorf("a stale release counted an attempt: %+v", row)
			}
		}
		sort.Strings(states)
		if len(states) != 3 || states[0] != models.ScheduledDeletionActing || states[1] != models.ScheduledDeletionPending || states[2] != models.ScheduledDeletionPending {
			t.Errorf("states = %v, want the fresh claim acting and the two old claims pending", states)
		}
	})

	t.Run("drop a chat's pending rows", func(t *testing.T) {
		chatID, other := deletionChat(t), deletionChat(t)
		for i, kind := range []string{KindPinned, KindTitle, KindBotWelcome} {
			if _, err := Schedule(ctx, Entry{ChatID: chatID, MessageID: int64(10 + i), Kind: kind, SentAt: now, DelaySecs: 600}); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := Schedule(ctx, Entry{ChatID: other, MessageID: 10, Kind: KindPinned, SentAt: now, DelaySecs: 600}); err != nil {
			t.Fatal(err)
		}
		if _, err := Schedule(ctx, Entry{ChatID: chatID, MessageID: 20, Kind: KindPinned, SentAt: now.Add(-time.Hour), DelaySecs: 0}); err != nil {
			t.Fatal(err)
		}
		// Claim the one due row, so an acting row must survive the drop.
		token := NewClaimToken()
		if rows, err := ClaimDue(ctx, now, 10, token); err != nil || len(rows) != 1 {
			t.Fatalf("claim = %d, %v", len(rows), err)
		}
		dropped, err := DropChatPending(ctx, chatID, "service:")
		if err != nil || dropped != 2 {
			t.Fatalf("DropChatPending(service:) = (%d, %v), want (2, nil)", dropped, err)
		}
		left := storedQueue(t, chatID)
		if len(left) != 2 {
			t.Fatalf("%d rows left, want the bot row and the acting row", len(left))
		}
		dropped, err = DropChatPending(ctx, chatID, "")
		if err != nil || dropped != 1 {
			t.Fatalf("DropChatPending(all) = (%d, %v), want (1, nil)", dropped, err)
		}
		if rows := storedQueue(t, chatID); len(rows) != 1 || rows[0].State != models.ScheduledDeletionActing {
			t.Fatalf("rows = %+v, want only the acting row", rows)
		}
		if n := len(storedQueue(t, other)); n != 1 {
			t.Fatalf("another chat has %d rows, want 1", n)
		}
	})
}
