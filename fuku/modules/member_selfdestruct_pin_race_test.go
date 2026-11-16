//go:build testtools

package modules

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

// memberPinnedSize returns how many pins are remembered.
func memberPinnedSize() int {
	memberPinnedNotes.mu.Lock()
	defer memberPinnedNotes.mu.Unlock()
	return len(memberPinnedNotes.at)
}

// TestMemberSelfDestructPinRace holds WR-03: a message pinned while it sat in the batcher buffer is
// dropped by the flush instead of written, the note expires, and a drop that failed is retried once.
func TestMemberSelfDestructPinRace(t *testing.T) {
	t.Run("offer, pin, flush leaves no row for the pinned message", func(t *testing.T) {
		env := newSelfDestructEnv(t, memberSelfDestructRuleDelay)
		selfDestructClock(t, env.now)
		member := env.member()
		pinned := env.sendMember(&member, func(m *gotgbot.Message) { m.Text = "pin me" })
		other := env.sendMember(&member, func(m *gotgbot.Message) { m.Text = "not pinned" })
		env.sendPinOf(env.chat, &env.admin, pinned)
		if got := memberDeleteTestFlush(t); got != 1 {
			t.Fatalf("memberDeleteFlush() = %d, want 1 (the pinned message is dropped from the batch)", got)
		}
		rows := env.selfDestructRows(env.chat.Id)
		if len(rows) != 1 || rows[0].MessageID != other {
			t.Fatalf("rows = %+v, want only message %d", rows, other)
		}
		if got := memberDeletePinnedDropped.Load(); got != 1 {
			t.Fatalf("entries dropped as pinned = %d, want 1", got)
		}
	})

	t.Run("the bot's own pin reaches the buffer too and the note expires", func(t *testing.T) {
		env := newSelfDestructEnv(t, memberSelfDestructRuleDelay)
		setNow := selfDestructClock(t, env.now)
		const messageID int64 = 881601
		memberDeletePinned(env.chat.Id, messageID)
		if !memberDeleteOffer(selfDestructEntry(env.chat.Id, messageID, env.now, 600)) {
			t.Fatal("memberDeleteOffer() = false, want accepted")
		}
		if got := memberDeleteTestFlush(t); got != 0 {
			t.Fatalf("memberDeleteFlush() = %d, want 0 for a message pinned a moment ago", got)
		}
		setNow(env.now.Add(memberPinnedTTL + time.Second))
		if !memberDeleteOffer(selfDestructEntry(env.chat.Id, messageID, env.now, 600)) {
			t.Fatal("memberDeleteOffer() = false, want accepted")
		}
		if got := memberDeleteTestFlush(t); got != 1 {
			t.Fatalf("memberDeleteFlush() = %d, want 1 once the note has expired", got)
		}
	})

	t.Run("the note set is bounded", func(t *testing.T) {
		env := newSelfDestructEnv(t, memberSelfDestructRuleDelay)
		setNow := selfDestructClock(t, env.now)
		mreqSwap(t, &memberPinnedMax, 4)
		for i := range 10 {
			memberDeletePinned(env.chat.Id, 881700+int64(i))
		}
		if got := memberPinnedSize(); got > 4 {
			t.Fatalf("pinned notes = %d, want at most 4", got)
		}
		if !memberPinnedRecently(env.chat.Id, 881709, deletionNow()) {
			t.Fatal("the newest pin was not noted: the bound must evict the oldest, not refuse the newest")
		}
		setNow(env.now.Add(memberPinnedTTL + time.Second))
		memberDeletePinned(env.chat.Id, 881799)
		if got := memberPinnedSize(); got != 1 {
			t.Fatalf("pinned notes after the TTL = %d, want 1 (expired notes are pruned on write)", got)
		}
	})

	t.Run("a failed drop is retried once", func(t *testing.T) {
		env := newSelfDestructEnv(t, memberSelfDestructRuleDelay)
		calls := 0
		mreqSwap(t, &memberDeleteDropPinned,
			func(_ context.Context, _, _ int64, _ string) (int64, error) {
				calls++
				if calls == 1 {
					return 0, errors.New("database is locked")
				}
				return 1, nil
			})
		memberDeletePinned(env.chat.Id, 881801)
		if calls != 2 {
			t.Fatalf("drop attempts = %d, want 2 (one retry)", calls)
		}
		calls = 0
		mreqSwap(t, &memberDeleteDropPinned,
			func(context.Context, int64, int64, string) (int64, error) {
				calls++
				return 0, errors.New("database is locked")
			})
		memberDeletePinned(env.chat.Id, 881802)
		if calls != 2 {
			t.Fatalf("drop attempts for a drop that keeps failing = %d, want exactly 2", calls)
		}
	})
}
