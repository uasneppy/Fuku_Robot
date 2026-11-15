//go:build testtools

package modules

import (
	"context"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/redis/go-redis/v9"

	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
)

// TestMemberSelfDestructBatcherLatch holds WR-02 and SEC-17-02: a chat whose delete-right latch is set
// has its offered entries shed (the messages stay) with one latch read per chat per flush, an
// unlatched chat is unchanged, and a latch read that fails never blocks or deletes anything.
func TestMemberSelfDestructBatcherLatch(t *testing.T) {
	t.Run("a latched chat leaves no row and an unlatched chat is unchanged", func(t *testing.T) {
		env := newDeletionEnv(t)
		drainMemberDeleteQueue(t)
		restoreMemberDeleteBatcher(t)
		other := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Other"}
		deletionCleanup(t, other.Id)
		deletionSetRule(t, env.chat.Id, deletion.KindMemberSelfDestruct, 600)
		deletionSetRule(t, other.Id, deletion.KindMemberSelfDestruct, 600)
		deletionSetLatch(context.Background(), env.chat.Id, deletionScopeService)

		memberDeleteOfferN(env.chat.Id, 881101, 3, env.now)
		memberDeleteOfferN(other.Id, 881201, 2, env.now)
		if got := memberDeleteTestFlush(t); got != 2 {
			t.Fatalf("memberDeleteFlush() = %d, want 2 (only the unlatched chat's entries)", got)
		}
		if rows := env.queuedNoWait(env.chat.Id); len(rows) != 0 {
			t.Fatalf("latched chat rows = %+v, want none", rows)
		}
		if rows := env.queuedNoWait(other.Id); len(rows) != 2 {
			t.Fatalf("unlatched chat rows = %d, want 2", len(rows))
		}
		if got := memberDeleteShedTotal(memberDeleteShedLatched); got != 3 {
			t.Fatalf("shed for the latch = %d, want 3", got)
		}
	})

	t.Run("the gone scope stops the chat too", func(t *testing.T) {
		env := newDeletionEnv(t)
		drainMemberDeleteQueue(t)
		restoreMemberDeleteBatcher(t)
		deletionSetRule(t, env.chat.Id, deletion.KindMemberSelfDestruct, 600)
		deletionSetLatch(context.Background(), env.chat.Id, deletionScopeGone)
		memberDeleteOfferN(env.chat.Id, 881301, 2, env.now)
		if got := memberDeleteTestFlush(t); got != 0 {
			t.Fatalf("memberDeleteFlush() = %d, want 0", got)
		}
		if rows := env.queuedNoWait(env.chat.Id); len(rows) != 0 {
			t.Fatalf("rows = %+v, want none", rows)
		}
	})

	t.Run("a latch read that fails writes as before", func(t *testing.T) {
		env := newDeletionEnv(t)
		drainMemberDeleteQueue(t)
		restoreMemberDeleteBatcher(t)
		deletionSetRule(t, env.chat.Id, deletion.KindMemberSelfDestruct, 600)
		broken := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: -1, DialTimeout: 50 * time.Millisecond})
		t.Cleanup(func() { _ = broken.Close() })
		mreqSwap(t, &deletionRedisClient, func() *redis.Client { return broken })
		memberDeleteOfferN(env.chat.Id, 881401, 2, env.now)
		if got := memberDeleteTestFlush(t); got != 2 {
			t.Fatalf("memberDeleteFlush() = %d, want 2: a failing latch read is not a latch", got)
		}
		if got := memberDeleteShedTotal(memberDeleteShedLatched); got != 0 {
			t.Fatalf("shed for the latch = %d, want 0", got)
		}
	})

	t.Run("an off chat costs no latch read", func(t *testing.T) {
		env := newDeletionEnv(t)
		drainMemberDeleteQueue(t)
		restoreMemberDeleteBatcher(t)
		deletionSetLatch(context.Background(), env.chat.Id, deletionScopeService)
		memberDeleteOfferN(env.chat.Id, 881501, 2, env.now)
		if got := memberDeleteTestFlush(t); got != 0 {
			t.Fatalf("memberDeleteFlush() = %d, want 0", got)
		}
		if got := memberDeleteShedTotal(memberDeleteShedLatched); got != 0 {
			t.Fatalf("shed for the latch = %d, want 0: a chat with the rule off is dropped as off, not as latched", got)
		}
		if got := memberDeletionDroppedOffTotal(); got != 2 {
			t.Fatalf("dropped as off = %d, want 2", got)
		}
	})
}

// memberDeletionDroppedOffTotal returns the entries a flush dropped because the rule is off.
func memberDeletionDroppedOffTotal() int64 { return memberDeleteDroppedOff.Load() }
