//go:build testtools

package modules

import (
	"testing"
	"time"

	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// TestDeletionWorkerBatches is the tracer of plan 16-04: the real worker claims due rows, deletes
// them in batch calls of at most 100 IDs through the pacer and removes them, and leaves rows that
// are not due yet alone.
func TestDeletionWorkerBatches(t *testing.T) {
	env := newDeletionEnv(t)
	other := uniqueModuleChatID()
	deletionCleanup(t, other)

	// 250 due rows for the first chat, 3 later due rows for a second chat and 5 rows due in an
	// hour for the first chat.
	firstIDs := env.seedDue(env.chat.Id, 1000, 250, deletion.KindPinned)
	for i := int64(0); i < 3; i++ {
		seedDeletionRow(t, other, 2000+i, env.now.Add(-10*time.Minute+time.Duration(i)*time.Second),
			env.now.Add(-2*time.Hour), deletion.KindPinned)
	}
	for i := int64(0); i < 5; i++ {
		seedDeletionRow(t, env.chat.Id, 3000+i, env.now.Add(time.Hour), env.now.Add(-time.Minute), deletion.KindPinned)
	}

	env.pass()

	batches := env.batchCalls(env.chat.Id)
	if len(batches) != 3 || len(batches[0]) != 100 || len(batches[1]) != 100 || len(batches[2]) != 50 {
		sizes := make([]int, 0, len(batches))
		for _, batch := range batches {
			sizes = append(sizes, len(batch))
		}
		t.Fatalf("deleteMessages calls for the first chat have sizes %v, want [100 100 50]", sizes)
	}
	var sent []int64
	for _, batch := range batches {
		sent = append(sent, batch...)
	}
	for i, id := range sent {
		if id != firstIDs[i] {
			t.Fatalf("batch ID %d = %d, want %d (ascending by delete_at)", i, id, firstIDs[i])
		}
	}
	if second := env.batchCalls(other); len(second) != 1 || len(second[0]) != 3 {
		t.Fatalf("deleteMessages calls for the second chat = %v, want one call of 3 IDs", second)
	}
	if singles := env.singleCalls(env.chat.Id); len(singles) != 0 {
		t.Fatalf("deleteMessage (single) calls = %v, want none when every batch succeeded", singles)
	}

	rows := env.queuedNoWait(env.chat.Id)
	if len(rows) != 5 {
		t.Fatalf("rows left for the first chat = %d, want the 5 that are not due", len(rows))
	}
	for _, row := range rows {
		if row.State != models.ScheduledDeletionPending || row.ClaimToken != "" || row.Attempts != 0 {
			t.Fatalf("future row %d = %+v, want untouched pending", row.MessageID, row)
		}
	}
	if left := env.queuedNoWait(other); len(left) != 0 {
		t.Fatalf("rows left for the second chat = %d, want 0", len(left))
	}

	// A cycle that finds nothing due makes no Telegram call.
	before := len(env.fake.callsFor("deleteMessages"))
	if env.cycle() {
		t.Fatal("a cycle with nothing due reported progress")
	}
	if after := len(env.fake.callsFor("deleteMessages")); after != before {
		t.Fatalf("an idle cycle made %d deleteMessages calls, want 0", after-before)
	}
}
