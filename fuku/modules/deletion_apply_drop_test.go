//go:build testtools

package modules

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// WR-03: switching a rule off (Remove, Keep, /cleanservice off) drops the chat's PENDING queue
// rows of the affected kinds in the same write. These tests go through the apply functions and the
// real queue; they never touch an acting row, another kind, another chat or a changed delay.

// dropFixture is one chat with queued rows of several kinds, plus the same kinds in another chat.
type dropFixture struct {
	chat, other int64
	next        int64
}

func newDropFixture(t *testing.T) *dropFixture {
	t.Helper()
	f := &dropFixture{chat: uniqueModuleChatID(), other: uniqueModuleChatID(), next: 70000}
	deletionCleanup(t, f.chat, f.other)
	return f
}

// queue seeds one pending row of the kind in the chat, due in an hour, and returns its message ID.
func (f *dropFixture) queue(t *testing.T, chatID int64, kind string) int64 {
	t.Helper()
	f.next++
	now := time.Now().UTC()
	seedDeletionRow(t, chatID, f.next, now.Add(time.Hour), now.Add(-time.Minute), kind)
	return f.next
}

// queueActing seeds a row a worker already claimed (state acting, with a token).
func (f *dropFixture) queueActing(t *testing.T, chatID int64, kind string) int64 {
	t.Helper()
	id := f.queue(t, chatID, kind)
	claimed := time.Now().UTC().Truncate(time.Second)
	err := db.DB.Model(&models.ScheduledDeletion{}).Where("chat_id = ? AND message_id = ?", chatID, id).
		Updates(map[string]any{"state": models.ScheduledDeletionActing, "claim_token": "worker-token", "claimed_at": claimed}).Error
	if err != nil {
		t.Fatalf("mark row %d acting: %v", id, err)
	}
	return id
}

// dropLeft returns the message IDs still queued in the chat, with their state.
func dropLeft(t *testing.T, chatID int64) map[int64]string {
	t.Helper()
	var rows []models.ScheduledDeletion
	if err := db.DB.Where("chat_id = ?", chatID).Find(&rows).Error; err != nil {
		t.Fatalf("read the queue of chat %d: %v", chatID, err)
	}
	out := make(map[int64]string, len(rows))
	for _, row := range rows {
		out[row.MessageID] = row.State
	}
	return out
}

func wantQueued(t *testing.T, chatID int64, want map[int64]string) {
	t.Helper()
	got := dropLeft(t, chatID)
	if len(got) != len(want) {
		t.Fatalf("queue of chat %d = %v, want %v", chatID, got, want)
	}
	for id, state := range want {
		if got[id] != state {
			t.Fatalf("queue of chat %d = %v, want %v", chatID, got, want)
		}
	}
}

func TestApplyDeleteRuleRemoveDropsPendingRows(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct{ name, kind, otherKind string }{
		{"a service kind", deletion.KindPinned, deletion.KindVideo},
		{"a bot kind", deletion.KindBotWelcome, deletion.KindBotGoodbye},
		{"a bot kind next to a service kind", deletion.KindBotRules, deletion.KindPinned},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newDropFixture(t)
			deletionSetRule(t, f.chat, tc.kind, 600)
			f.queue(t, f.chat, tc.kind)
			f.queue(t, f.chat, tc.kind)
			acting := f.queueActing(t, f.chat, tc.kind)
			sameChatOtherKind := f.queue(t, f.chat, tc.otherKind)
			otherChat := f.queue(t, f.other, tc.kind)

			result := applyDeleteRule(ctx, f.chat, tc.kind, deletionRemove)
			if result.Outcome != deletionApplied || result.Err != nil {
				t.Fatalf("remove = %+v, want applied", result)
			}
			if _, has := deletionStoredRow(t, f.chat, tc.kind); has {
				t.Fatal("the rule row survived Remove")
			}
			wantQueued(t, f.chat, map[int64]string{
				acting:            models.ScheduledDeletionActing,
				sameChatOtherKind: models.ScheduledDeletionPending,
			})
			wantQueued(t, f.other, map[int64]string{otherChat: models.ScheduledDeletionPending})
		})
	}

	t.Run("a prefix of the kind is not a match", func(t *testing.T) {
		// Exact kind match: removing one kind never drops a kind that merely shares its start.
		f := newDropFixture(t)
		deletionSetRule(t, f.chat, deletion.KindBotAuto, 600)
		f.queue(t, f.chat, deletion.KindBotAuto)
		manual := f.queue(t, f.chat, deletion.KindBotManual)
		if result := applyDeleteRule(ctx, f.chat, deletion.KindBotAuto, deletionRemove); result.Outcome != deletionApplied {
			t.Fatalf("remove = %+v, want applied", result)
		}
		wantQueued(t, f.chat, map[int64]string{manual: models.ScheduledDeletionPending})
	})

	t.Run("changing the delay drops nothing", func(t *testing.T) {
		f := newDropFixture(t)
		deletionSetRule(t, f.chat, deletion.KindPinned, 600)
		a := f.queue(t, f.chat, deletion.KindPinned)
		b := f.queue(t, f.chat, deletion.KindPinned)
		for _, delay := range []int{1200, 0, 30} {
			if result := applyDeleteRule(ctx, f.chat, deletion.KindPinned, deletionSet(delay)); result.Outcome != deletionApplied {
				t.Fatalf("set %d = %+v, want applied", delay, result)
			}
			wantQueued(t, f.chat, map[int64]string{a: models.ScheduledDeletionPending, b: models.ScheduledDeletionPending})
		}
	})

	t.Run("a failed write reports a failure and drops nothing", func(t *testing.T) {
		f := newDropFixture(t)
		deletionSetRule(t, f.chat, deletion.KindPinned, 600)
		a := f.queue(t, f.chat, deletion.KindPinned)
		boom := errors.New("write refused")
		mreqSwap(t, &deletionRuleWrites, deletionRuleWriter{Set: deletion.SetRule,
			Remove: func(context.Context, int64, string) error { return boom }})
		result := applyDeleteRule(ctx, f.chat, deletion.KindPinned, deletionRemove)
		if result.Outcome != deletionSaveFailed || !errors.Is(result.Err, boom) {
			t.Fatalf("remove = %+v, want a save failure carrying the error", result)
		}
		wantQueued(t, f.chat, map[int64]string{a: models.ScheduledDeletionPending})
		if row, has := deletionStoredRow(t, f.chat, deletion.KindPinned); !has || row.DelaySecs != 600 {
			t.Fatalf("rule row = %+v (%v), want it untouched", row, has)
		}
	})
}

func TestApplyCleanServiceRuleKeepDropsPendingRows(t *testing.T) {
	ctx := context.Background()
	for _, kind := range []string{deletion.KindJoined, deletion.KindLeft} {
		t.Run("keep from "+kind, func(t *testing.T) {
			f := newDropFixture(t)
			deletionSeedJoinLeft(t, f.chat, true, 600, 30)
			f.queue(t, f.chat, deletion.KindJoined)
			f.queue(t, f.chat, deletion.KindLeft)
			actingJoined := f.queueActing(t, f.chat, deletion.KindJoined)
			pinned := f.queue(t, f.chat, deletion.KindPinned)
			welcome := f.queue(t, f.chat, deletion.KindBotWelcome)
			otherJoined := f.queue(t, f.other, deletion.KindJoined)

			result := applyCleanServiceRule(ctx, f.chat, kind, deletion.JoinLeftState{Mode: deletion.JoinLeftKeep})
			if result.Outcome != deletionApplied || result.Err != nil {
				t.Fatalf("keep = %+v, want applied", result)
			}
			if deletionStoredFlag(t, f.chat) {
				t.Fatal("the flag is still on after Keep")
			}
			wantQueued(t, f.chat, map[int64]string{
				actingJoined: models.ScheduledDeletionActing,
				pinned:       models.ScheduledDeletionPending,
				welcome:      models.ScheduledDeletionPending,
			})
			wantQueued(t, f.other, map[int64]string{otherJoined: models.ScheduledDeletionPending})
		})
	}

	t.Run("now and after drop nothing", func(t *testing.T) {
		f := newDropFixture(t)
		deletionSeedJoinLeft(t, f.chat, true, 600, 30)
		joined := f.queue(t, f.chat, deletion.KindJoined)
		left := f.queue(t, f.chat, deletion.KindLeft)
		want := map[int64]string{joined: models.ScheduledDeletionPending, left: models.ScheduledDeletionPending}
		for _, state := range []deletion.JoinLeftState{
			{Mode: deletion.JoinLeftNow},
			{Mode: deletion.JoinLeftAfter, DelaySecs: 900},
		} {
			if result := applyCleanServiceRule(ctx, f.chat, deletion.KindJoined, state); result.Outcome != deletionApplied {
				t.Fatalf("apply %+v = %+v, want applied", state, result)
			}
			wantQueued(t, f.chat, want)
		}
	})

	t.Run("a failure inside the transaction keeps the rows and the rules", func(t *testing.T) {
		f := newDropFixture(t)
		deletionSeedJoinLeft(t, f.chat, true, 600, 30)
		joined := f.queue(t, f.chat, deletion.KindJoined)
		left := f.queue(t, f.chat, deletion.KindLeft)
		deletionCountHook(t, errors.New("second step failed"))
		result := applyCleanServiceRule(ctx, f.chat, deletion.KindJoined, deletion.JoinLeftState{Mode: deletion.JoinLeftKeep})
		if result.Outcome != deletionSaveFailed || result.Err == nil {
			t.Fatalf("keep = %+v, want a save failure", result)
		}
		wantQueued(t, f.chat, map[int64]string{joined: models.ScheduledDeletionPending, left: models.ScheduledDeletionPending})
		if !deletionStoredFlag(t, f.chat) {
			t.Fatal("the flag changed although the transaction failed")
		}
		if _, has := deletionStoredRow(t, f.chat, deletion.KindJoined); !has {
			t.Fatal("the joined rule row was removed although the transaction failed")
		}
	})
}

func TestApplyCleanServiceFlagOffDropsPendingRows(t *testing.T) {
	ctx := context.Background()
	f := newDropFixture(t)
	deletionSeedJoinLeft(t, f.chat, true, 600, -1)
	joined := f.queue(t, f.chat, deletion.KindJoined)
	left := f.queue(t, f.chat, deletion.KindLeft)
	actingLeft := f.queueActing(t, f.chat, deletion.KindLeft)
	pinned := f.queue(t, f.chat, deletion.KindPinned)
	otherLeft := f.queue(t, f.other, deletion.KindLeft)

	// /cleanservice on keeps whatever is queued (it only sets the flag and clears the delay rows).
	if res := applyCleanService(ctx, f.chat, true); res.Err != nil {
		t.Fatalf("/cleanservice on = %v", res.Err)
	}
	wantQueued(t, f.chat, map[int64]string{
		joined: models.ScheduledDeletionPending, left: models.ScheduledDeletionPending,
		actingLeft: models.ScheduledDeletionActing, pinned: models.ScheduledDeletionPending,
	})

	// /cleanservice off drops the pending joined and left rows of this chat and nothing else.
	if res := applyCleanService(ctx, f.chat, false); res.Err != nil {
		t.Fatalf("/cleanservice off = %v", res.Err)
	}
	if deletionStoredFlag(t, f.chat) {
		t.Fatal("the flag is still on after /cleanservice off")
	}
	wantQueued(t, f.chat, map[int64]string{
		actingLeft: models.ScheduledDeletionActing, pinned: models.ScheduledDeletionPending,
	})
	wantQueued(t, f.other, map[int64]string{otherLeft: models.ScheduledDeletionPending})
}
