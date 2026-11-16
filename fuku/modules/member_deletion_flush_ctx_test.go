//go:build testtools

package modules

import (
	"context"
	"testing"
	"time"

	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
)

// TestMemberDeleteFlushWriteOwnContext holds IN-02: a screening that used up the flush's budget does
// not make the write of the already screened entries fail, because the write has its own bounded
// context.
func TestMemberDeleteFlushWriteOwnContext(t *testing.T) {
	env := newDeletionEnv(t)
	drainMemberDeleteQueue(t)
	restoreMemberDeleteBatcher(t)
	deletionSetRule(t, env.chat.Id, deletion.KindMemberSelfDestruct, 600)
	// The count is the slow screening step: it returns only when the flush's own deadline ran out.
	mreqSwap(t, &memberDeleteCountPending,
		func(ctx context.Context, _ int64, _ string, _ int) (int64, error) {
			<-ctx.Done()
			return 0, nil
		})
	var writeErr error
	var writeHadDeadline bool
	mreqSwap(t, &memberDeleteBatchWrite,
		func(ctx context.Context, entries []deletion.Entry) (int, error) {
			writeErr = ctx.Err()
			_, writeHadDeadline = ctx.Deadline()
			return deletion.ScheduleBatch(ctx, entries)
		})
	memberDeleteOfferN(env.chat.Id, 882001, 2, env.now)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if got := memberDeleteFlush(ctx); got != 2 {
		t.Fatalf("memberDeleteFlush() = %d, want 2 written although screening used the whole budget", got)
	}
	if writeErr != nil {
		t.Fatalf("the write's context was already done: %v", writeErr)
	}
	if !writeHadDeadline {
		t.Fatal("the write's context has no deadline of its own, want a bounded one")
	}
	if rows := env.queuedNoWait(env.chat.Id); len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
}
