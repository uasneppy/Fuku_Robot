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

// seedDueBulk writes n due pending rows of one kind for chatID in one bulk insert, message IDs
// first..first+n-1, and returns the IDs.
func (e *deletionEnv) seedDueBulk(chatID, first int64, n int, kind string) []int64 {
	e.t.Helper()
	rows := make([]models.ScheduledDeletion, n)
	ids := make([]int64, n)
	sentAt := e.now.Add(-2 * time.Hour).UTC().Truncate(time.Second)
	for i := range rows {
		ids[i] = first + int64(i)
		rows[i] = models.ScheduledDeletion{
			ChatID: chatID, MessageID: ids[i], Kind: kind, SentAt: sentAt,
			DeleteAt: e.now.Add(-time.Hour + time.Duration(i%600)*time.Second).UTC().Truncate(time.Second),
			State:    models.ScheduledDeletionPending, CreatedAt: sentAt,
		}
	}
	if err := db.DB.CreateInBatches(&rows, 100).Error; err != nil {
		e.t.Fatalf("seed %d rows of chat %d: %v", n, chatID, err)
	}
	return ids
}

// TestDeletionWorkerManyChats: a cycle claims 1000 rows, and each chat costs one paced call per
// chunk of at most 100, so many busy groups are drained together.
func TestDeletionWorkerManyChats(t *testing.T) {
	t.Run("40 chats of 25 due rows are one cycle of 40 calls", func(t *testing.T) {
		env := newDeletionEnv(t)
		chats := make([]int64, 40)
		for i := range chats {
			chats[i] = uniqueModuleChatID()
			deletionCleanup(t, chats[i])
			env.seedDueBulk(chats[i], int64(70000+i*100), 25, deletion.KindBotWelcome)
		}

		if !env.cycle() {
			t.Fatal("the first cycle made no progress")
		}
		total := 0
		for _, chatID := range chats {
			calls := env.batchCalls(chatID)
			if len(calls) != 1 || len(calls[0]) != 25 {
				t.Fatalf("deleteMessages calls of chat %d = %v, want one call of 25 IDs", chatID, calls)
			}
			if singles := env.singleCalls(chatID); len(singles) != 0 {
				t.Fatalf("single deleteMessage calls of chat %d = %v, want none", chatID, singles)
			}
			if left := env.queuedNoWait(chatID); len(left) != 0 {
				t.Fatalf("rows of chat %d after one cycle = %d, want none", chatID, len(left))
			}
			total += len(calls)
		}
		if total != 40 {
			t.Fatalf("calls in one cycle = %d, want 40 (one per chat)", total)
		}
	})

	t.Run("3000 rows of one chat are 30 calls over three cycles", func(t *testing.T) {
		env := newDeletionEnv(t)
		env.seedDueBulk(env.chat.Id, 90000, 3000, deletion.KindBotWelcome)

		for cycle := 1; cycle <= 3; cycle++ {
			if !env.cycle() {
				t.Fatalf("cycle %d made no progress", cycle)
			}
			if got, want := len(env.batchCalls(env.chat.Id)), 10*cycle; got != want {
				t.Fatalf("calls after cycle %d = %d, want %d (1000 rows claimed, 10 chunks of 100)", cycle, got, want)
			}
		}
		if env.cycle() {
			t.Fatal("a fourth cycle made progress, want the queue empty")
		}
		if got := len(env.batchCalls(env.chat.Id)); got != 30 {
			t.Fatalf("calls in all = %d, want exactly 30", got)
		}
		if left := env.queuedNoWait(env.chat.Id); len(left) != 0 {
			t.Fatalf("rows left = %d, want none", len(left))
		}
	})
}

// TestDeletionWorkerScreensSelfDestructOff: a claimed member:selfdestruct row is deleted only while
// the chat's rule is on; with the rule gone it is completed with no call, with the rules unreadable
// it goes back free, and the rows of other kinds are never screened.
func TestDeletionWorkerScreensSelfDestructOff(t *testing.T) {
	t.Run("a rule that is off completes the rows with no call", func(t *testing.T) {
		env := newDeletionEnv(t)
		gone := env.seedDue(env.chat.Id, 91000, 3, deletion.KindMemberSelfDestruct)
		edited := env.seedDue(env.chat.Id, 91100, 2, deletion.KindMemberEdited)

		env.pass()
		for _, id := range gone {
			if env.callsNaming(id) != 0 {
				t.Fatalf("message %d of an Off rule was sent to Telegram, want no call", id)
			}
		}
		if left := env.queuedNoWait(env.chat.Id); len(left) != 0 {
			t.Fatalf("rows left = %+v, want none: the self-destruct rows completed, the edited rows deleted", left)
		}
		calls := env.batchCalls(env.chat.Id)
		if len(calls) != 1 || len(calls[0]) != len(edited) {
			t.Fatalf("deleteMessages calls = %v, want one call with the %d member:edited rows, which are never screened", calls, len(edited))
		}
	})

	t.Run("a rule that is on deletes the rows", func(t *testing.T) {
		env := newDeletionEnv(t)
		deletionSetRule(t, env.chat.Id, deletion.KindMemberSelfDestruct, 600)
		ids := env.seedDue(env.chat.Id, 92000, 3, deletion.KindMemberSelfDestruct)

		env.pass()
		calls := env.batchCalls(env.chat.Id)
		if len(calls) != 1 || len(calls[0]) != len(ids) {
			t.Fatalf("deleteMessages calls = %v, want one call with %d IDs", calls, len(ids))
		}
		if left := env.queuedNoWait(env.chat.Id); len(left) != 0 {
			t.Fatalf("rows left = %+v, want none", left)
		}
	})

	t.Run("rules that cannot be read put the rows back free and delete the other kinds", func(t *testing.T) {
		env := newDeletionEnv(t)
		deletionSetRule(t, env.chat.Id, deletion.KindMemberSelfDestruct, 600)
		held := env.seedDue(env.chat.Id, 93000, 3, deletion.KindMemberSelfDestruct)
		welcome := env.seedDue(env.chat.Id, 93100, 2, deletion.KindBotWelcome)
		mreqSwap(t, &deletionRulesLookup, func(context.Context, int64) (*deletion.Rules, error) {
			return nil, errors.New("rules unreadable")
		})

		env.pass()
		calls := env.batchCalls(env.chat.Id)
		if len(calls) != 1 || len(calls[0]) != len(welcome) {
			t.Fatalf("deleteMessages calls = %v, want one call with the %d bot:welcome rows", calls, len(welcome))
		}
		for _, id := range held {
			if env.callsNaming(id) != 0 {
				t.Fatalf("message %d was sent to Telegram with its rules unreadable", id)
			}
		}
		left := env.queuedNoWait(env.chat.Id)
		if len(left) != len(held) {
			t.Fatalf("rows left = %d, want the %d self-destruct rows put back", len(left), len(held))
		}
		for _, row := range left {
			if row.State != models.ScheduledDeletionPending || row.ClaimToken != "" || row.Attempts != 0 || row.Kind != deletion.KindMemberSelfDestruct {
				t.Fatalf("row %+v, want pending, no claim token, no attempt counted", row)
			}
		}
	})
}

// callsNaming counts the Telegram delete calls of the env chat that name message id, batch or single.
func (e *deletionEnv) callsNaming(id int64) int {
	e.t.Helper()
	n := 0
	for _, batch := range e.batchCalls(e.chat.Id) {
		for _, got := range batch {
			if got == id {
				n++
			}
		}
	}
	for _, got := range e.singleCalls(e.chat.Id) {
		if got == id {
			n++
		}
	}
	return n
}
