//go:build testtools

package modules

import (
	"context"
	"testing"
	"time"

	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// selfDestructEntry is a self-destruct entry for a message of chatID sent at sentAt.
func selfDestructEntry(chatID, messageID int64, sentAt time.Time, delay int) deletion.Entry {
	return deletion.Entry{
		ChatID: chatID, MessageID: messageID, Kind: deletion.KindMemberSelfDestruct, SentAt: sentAt, DelaySecs: delay,
	}
}

// memberDeleteTestFlush runs one flush with a bounded context and returns what it wrote.
func memberDeleteTestFlush(t *testing.T) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return memberDeleteFlush(ctx)
}

// TestMemberSelfDestructBatcherQueues is the plan's tracer: an entry offered to the batcher is not
// written until a flush, becomes one queue row at the message's own date plus the stored delay, and
// the real worker deletes it when that time comes and not before.
func TestMemberSelfDestructBatcherQueues(t *testing.T) {
	env := newDeletionEnv(t)
	drainMemberDeleteQueue(t)
	restoreMemberDeleteBatcher(t)
	deletionSetRule(t, env.chat.Id, deletion.KindMemberSelfDestruct, 600)

	sentAt := env.now.Add(-60 * time.Second)
	current := env.now
	mreqSwap(t, &deletionNow, func() time.Time { return current })

	const messageID int64 = 880101
	if !memberDeleteOffer(selfDestructEntry(env.chat.Id, messageID, sentAt, 600)) {
		t.Fatal("memberDeleteOffer() = false, want accepted")
	}
	if rows := env.queuedNoWait(env.chat.Id); len(rows) != 0 {
		t.Fatalf("rows before the flush = %+v, want none: an offer writes nothing", rows)
	}
	if got := memberDeleteTestFlush(t); got != 1 {
		t.Fatalf("memberDeleteFlush() = %d, want 1 written", got)
	}
	rows := env.queuedNoWait(env.chat.Id)
	if len(rows) != 1 {
		t.Fatalf("rows after the flush = %+v, want exactly one", rows)
	}
	row := rows[0]
	wantAt := sentAt.Add(600 * time.Second)
	if row.Kind != deletion.KindMemberSelfDestruct || row.MessageID != messageID || row.State != models.ScheduledDeletionPending ||
		!row.DeleteAt.Equal(wantAt) || !row.SentAt.Equal(sentAt) {
		t.Fatalf("row = %+v, want pending %s of message %d sent %s due %s", row, deletion.KindMemberSelfDestruct, messageID, sentAt, wantAt)
	}

	current = sentAt.Add(599 * time.Second)
	env.pass()
	if calls := env.batchCalls(env.chat.Id); len(calls) != 0 {
		t.Fatalf("deleteMessages calls at date+599s = %v, want none", calls)
	}
	if rows := env.queuedNoWait(env.chat.Id); len(rows) != 1 {
		t.Fatalf("rows at date+599s = %d, want the row still queued", len(rows))
	}

	current = sentAt.Add(601 * time.Second)
	env.pass()
	calls := env.batchCalls(env.chat.Id)
	if len(calls) != 1 || len(calls[0]) != 1 || calls[0][0] != messageID {
		t.Fatalf("deleteMessages calls at date+601s = %v, want one call with message %d", calls, messageID)
	}
	if rows := env.queuedNoWait(env.chat.Id); len(rows) != 0 {
		t.Fatalf("rows after the delete = %+v, want none", rows)
	}
}

// TestMemberSelfDestructBatcherEarliest: two offers of one message with different delays leave one
// row at the earlier time.
func TestMemberSelfDestructBatcherEarliest(t *testing.T) {
	env := newDeletionEnv(t)
	drainMemberDeleteQueue(t)
	restoreMemberDeleteBatcher(t)
	deletionSetRule(t, env.chat.Id, deletion.KindMemberSelfDestruct, 600)

	sentAt := env.now.Add(-30 * time.Second)
	const messageID int64 = 880102
	for _, delay := range []int{3600, 600} {
		if !memberDeleteOffer(selfDestructEntry(env.chat.Id, messageID, sentAt, delay)) {
			t.Fatalf("offer with delay %d refused", delay)
		}
	}
	if got := memberDeleteTestFlush(t); got != 1 {
		t.Fatalf("memberDeleteFlush() = %d, want 1 row for one message", got)
	}
	rows := env.queuedNoWait(env.chat.Id)
	if len(rows) != 1 || !rows[0].DeleteAt.Equal(sentAt.Add(600*time.Second)) {
		t.Fatalf("rows = %+v, want one row due at the earlier time %s", rows, sentAt.Add(600*time.Second))
	}
}
