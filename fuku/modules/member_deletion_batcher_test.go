//go:build testtools

package modules

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	logrustest "github.com/sirupsen/logrus/hooks/test"

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

// memberDeleteLogLines returns the warn-level (or worse) lines the hook captured that name chatID.
func memberDeleteLogLines(hook *logrustest.Hook, chatID int64) []string {
	var lines []string
	for _, entry := range hook.AllEntries() {
		if entry.Level <= log.WarnLevel && strings.Contains(entry.Message, strconv.FormatInt(chatID, 10)) {
			lines = append(lines, entry.Message)
		}
	}
	return lines
}

// memberDeleteOfferN offers n entries of one chat, message IDs first..first+n-1, and returns how many
// were accepted.
func memberDeleteOfferN(chatID, first int64, n int, sentAt time.Time) int {
	accepted := 0
	for i := range n {
		if memberDeleteOffer(selfDestructEntry(chatID, first+int64(i), sentAt, 600)) {
			accepted++
		}
	}
	return accepted
}

// TestMemberSelfDestructBatcherSheds: every refusal keeps the message and is counted, a ceiling is
// exact, one Warn per chat per hour names the chat and no message, and a failing count or write
// sheds without touching the other chats.
func TestMemberSelfDestructBatcherSheds(t *testing.T) {
	t.Run("a full channel refuses the offer and writes nothing", func(t *testing.T) {
		env := newDeletionEnv(t)
		drainMemberDeleteQueue(t)
		withMemberDeleteQueue(t, 3)
		restoreMemberDeleteBatcher(t)
		deletionSetRule(t, env.chat.Id, deletion.KindMemberSelfDestruct, 600)

		if got := memberDeleteOfferN(env.chat.Id, 880201, 3, env.now); got != 3 {
			t.Fatalf("accepted = %d, want the first 3", got)
		}
		if memberDeleteOffer(selfDestructEntry(env.chat.Id, 880204, env.now, 600)) {
			t.Fatal("the fourth offer was accepted, want a refusal")
		}
		if got := memberDeleteShedTotal(memberDeleteShedFull); got != 1 {
			t.Fatalf("shed as full = %d, want 1", got)
		}
		if got := memberDeleteTestFlush(t); got != 3 {
			t.Fatalf("memberDeleteFlush() = %d, want the 3 buffered entries", got)
		}
		for _, row := range env.queuedNoWait(env.chat.Id) {
			if row.MessageID == 880204 {
				t.Fatalf("the refused message has a queue row %+v", row)
			}
		}
	})

	t.Run("the ceiling is exact and logs once per chat per hour", func(t *testing.T) {
		env := newDeletionEnv(t)
		drainMemberDeleteQueue(t)
		restoreMemberDeleteBatcher(t)
		hook := captureLockdownLogs(t)
		mreqSwap(t, &memberSelfDestructCeiling, 5)
		deletionSetRule(t, env.chat.Id, deletion.KindMemberSelfDestruct, 600)
		for i := range 4 {
			seedDeletionRow(t, env.chat.Id, 880300+int64(i), env.now.Add(time.Hour), env.now, deletion.KindMemberSelfDestruct)
		}

		memberDeleteOfferN(env.chat.Id, 880310, 3, env.now)
		if got := memberDeleteTestFlush(t); got != 1 {
			t.Fatalf("memberDeleteFlush() = %d, want 1 written (room 1 under the ceiling of 5)", got)
		}
		if got := memberDeleteShedTotal(memberDeleteShedCeiling); got != 2 {
			t.Fatalf("shed over the ceiling = %d, want 2", got)
		}
		if rows := env.queuedNoWait(env.chat.Id); len(rows) != 5 {
			t.Fatalf("queued rows = %d, want exactly the ceiling of 5", len(rows))
		}

		// Later floods in the same hour shed more and log nothing more.
		memberDeleteOfferN(env.chat.Id, 880320, 3, env.now)
		if got := memberDeleteTestFlush(t); got != 0 {
			t.Fatalf("memberDeleteFlush() at the ceiling = %d, want 0", got)
		}
		if got := memberDeleteShedTotal(memberDeleteShedCeiling); got != 5 {
			t.Fatalf("shed over the ceiling = %d, want 5 in all", got)
		}
		if lines := memberDeleteLogLines(hook, env.chat.Id); len(lines) != 1 {
			t.Fatalf("warn lines naming the chat = %q, want exactly one", lines)
		}
		deletionWantNoMessageIDs(t, hook, []int64{880310, 880311, 880312, 880320, 880321, 880322})

		// An hour later the chat may log once more.
		deletionTestAdvance(t, memberDeleteWarnEvery+time.Second)
		memberDeleteOfferN(env.chat.Id, 880330, 1, env.now)
		memberDeleteTestFlush(t)
		if lines := memberDeleteLogLines(hook, env.chat.Id); len(lines) != 2 {
			t.Fatalf("warn lines naming the chat after an hour = %q, want two", lines)
		}
	})

	t.Run("a failing batch writer sheds every entry of the flush", func(t *testing.T) {
		env := newDeletionEnv(t)
		drainMemberDeleteQueue(t)
		restoreMemberDeleteBatcher(t)
		captureLockdownLogs(t)
		deletionSetRule(t, env.chat.Id, deletion.KindMemberSelfDestruct, 600)
		mreqSwap(t, &memberDeleteBatchWrite, func(context.Context, []deletion.Entry) (int, error) {
			return 0, errors.New("database is down")
		})

		memberDeleteOfferN(env.chat.Id, 880401, 4, env.now)
		if got := memberDeleteTestFlush(t); got != 0 {
			t.Fatalf("memberDeleteFlush() = %d, want 0", got)
		}
		if got := memberDeleteShedTotal(memberDeleteShedWrite); got != 4 {
			t.Fatalf("shed as a failed write = %d, want 4", got)
		}
		if rows := env.queuedNoWait(env.chat.Id); len(rows) != 0 {
			t.Fatalf("rows = %+v, want none: the messages stay", rows)
		}
	})

	t.Run("a failing count sheds that chat only", func(t *testing.T) {
		env := newDeletionEnv(t)
		drainMemberDeleteQueue(t)
		restoreMemberDeleteBatcher(t)
		other := uniqueModuleChatID()
		deletionCleanup(t, other)
		deletionSetRule(t, env.chat.Id, deletion.KindMemberSelfDestruct, 600)
		deletionSetRule(t, other, deletion.KindMemberSelfDestruct, 600)
		failing := env.chat.Id
		mreqSwap(t, &memberDeleteCountPending, func(ctx context.Context, chatID int64, kind string, limit int) (int64, error) {
			if chatID == failing {
				return 0, errors.New("count failed")
			}
			return deletion.CountPending(ctx, chatID, kind, limit)
		})

		memberDeleteOfferN(env.chat.Id, 880501, 3, env.now)
		memberDeleteOfferN(other, 880601, 2, env.now)
		if got := memberDeleteTestFlush(t); got != 2 {
			t.Fatalf("memberDeleteFlush() = %d, want only the other chat's 2", got)
		}
		if got := memberDeleteShedTotal(memberDeleteShedCount); got != 3 {
			t.Fatalf("shed as an unreadable count = %d, want 3", got)
		}
		if rows := env.queuedNoWait(env.chat.Id); len(rows) != 0 {
			t.Fatalf("rows of the chat whose count failed = %+v, want none", rows)
		}
		if rows := env.queuedNoWait(other); len(rows) != 2 {
			t.Fatalf("rows of the other chat = %d, want 2", len(rows))
		}
	})

	t.Run("after the stop an offer is refused", func(t *testing.T) {
		env := newDeletionEnv(t)
		drainMemberDeleteQueue(t)
		restoreDeletionWorker(t)
		restoreMemberDeleteBatcher(t)
		deletionSetRule(t, env.chat.Id, deletion.KindMemberSelfDestruct, 600)

		StopDeletionWorker()
		if memberDeleteOffer(selfDestructEntry(env.chat.Id, 880701, env.now, 600)) {
			t.Fatal("an offer after StopDeletionWorker was accepted, want a refusal")
		}
		if got := memberDeleteShedTotal(memberDeleteShedDraining); got != 1 {
			t.Fatalf("shed as draining = %d, want 1", got)
		}
		if got := memberDeleteTestFlush(t); got != 0 {
			t.Fatalf("memberDeleteFlush() = %d, want 0", got)
		}
	})

	t.Run("an entry of another kind is refused", func(t *testing.T) {
		env := newDeletionEnv(t)
		drainMemberDeleteQueue(t)
		restoreMemberDeleteBatcher(t)
		e := selfDestructEntry(env.chat.Id, 880801, env.now, 0)
		e.Kind = deletion.KindMemberCommands
		if memberDeleteOffer(e) {
			t.Fatal("a member:commands entry was accepted by the self-destruct batcher")
		}
	})
}

// TestMemberDeleteWarnGateBound: the gate tracks a bounded number of chats, forgets the ones older
// than an hour to make room, and logs nothing for a new chat when none can be forgotten.
func TestMemberDeleteWarnGateBound(t *testing.T) {
	restoreMemberDeleteBatcher(t)
	mreqSwap(t, &memberDeleteWarnMax, 2)
	now := time.Now()
	var gate memberDeleteWarnGate
	if !gate.allow(-1, now) || !gate.allow(-2, now) {
		t.Fatal("the first two chats were refused")
	}
	if gate.allow(-1, now.Add(time.Minute)) {
		t.Error("a second line of one chat inside the hour was allowed")
	}
	if gate.allow(-3, now.Add(time.Minute)) {
		t.Error("a third chat was allowed while the map is full of recent entries")
	}
	if !gate.allow(-3, now.Add(memberDeleteWarnEvery+time.Second)) {
		t.Error("a new chat was refused after the older entries could be forgotten")
	}
}

// TestMemberSelfDestructBatcherOffRace: nothing is written for a rule that is no longer on, for a
// chat whose rules cannot be read, for a Staff Group, and a chat whose Staff Group read failed leaves
// the other chats of the flush written.
func TestMemberSelfDestructBatcherOffRace(t *testing.T) {
	t.Run("a rule switched off between the offer and the flush", func(t *testing.T) {
		env := newDeletionEnv(t)
		drainMemberDeleteQueue(t)
		restoreMemberDeleteBatcher(t)
		deletionSetRule(t, env.chat.Id, deletion.KindMemberSelfDestruct, 600)

		memberDeleteOfferN(env.chat.Id, 880901, 2, env.now)
		res := applyDeleteRule(context.Background(), env.chat.Id, deletion.KindMemberSelfDestruct, deletionRemove)
		if res.Outcome != deletionApplied {
			t.Fatalf("switching the rule off = %+v, want applied", res)
		}
		if got := memberDeleteTestFlush(t); got != 0 {
			t.Fatalf("memberDeleteFlush() = %d, want 0 after Off", got)
		}
		if rows := env.queuedNoWait(env.chat.Id); len(rows) != 0 {
			t.Fatalf("rows after Off = %+v, want none", rows)
		}
		if got := memberDeleteDroppedOff.Load(); got != 2 {
			t.Fatalf("entries dropped for a rule that is off = %d, want 2", got)
		}
	})

	t.Run("an unreadable rule keeps the message and is counted", func(t *testing.T) {
		env := newDeletionEnv(t)
		drainMemberDeleteQueue(t)
		restoreMemberDeleteBatcher(t)
		deletionSetRule(t, env.chat.Id, deletion.KindMemberSelfDestruct, 600)
		mreqSwap(t, &deletionRulesLookup, func(context.Context, int64) (*deletion.Rules, error) {
			return nil, errors.New("rules unreadable")
		})

		memberDeleteOfferN(env.chat.Id, 881001, 2, env.now)
		if got := memberDeleteTestFlush(t); got != 0 {
			t.Fatalf("memberDeleteFlush() = %d, want 0", got)
		}
		if got := memberDeleteShedTotal(memberDeleteShedRule); got != 2 {
			t.Fatalf("shed as unreadable rules = %d, want 2", got)
		}
		if rows := env.queuedNoWait(env.chat.Id); len(rows) != 0 {
			t.Fatalf("rows = %+v, want none", rows)
		}
	})

	t.Run("a Staff Group gets no row while another chat is written", func(t *testing.T) {
		env := newDeletionEnv(t)
		drainMemberDeleteQueue(t)
		restoreMemberDeleteBatcher(t)
		staffChat := uniqueModuleChatID()
		deletionCleanup(t, staffChat)
		env.makeStaffGroup(staffChat)
		deletionSetRule(t, staffChat, deletion.KindMemberSelfDestruct, 600)
		deletionSetRule(t, env.chat.Id, deletion.KindMemberSelfDestruct, 600)

		memberDeleteOfferN(staffChat, 881101, 2, env.now)
		memberDeleteOfferN(env.chat.Id, 881201, 1, env.now)
		memberDeleteTestFlush(t)
		if rows := env.queuedNoWait(staffChat); len(rows) != 0 {
			t.Fatalf("rows of the Staff Group = %+v, want none (layer two of DEL-07)", rows)
		}
		if rows := env.queuedNoWait(env.chat.Id); len(rows) != 1 {
			t.Fatalf("rows of the other chat = %d, want 1", len(rows))
		}
	})

	t.Run("a failed Staff Group read leaves the other chats written", func(t *testing.T) {
		env := newDeletionEnv(t)
		drainMemberDeleteQueue(t)
		restoreMemberDeleteBatcher(t)
		captureLockdownLogs(t)
		unreadable := uniqueModuleChatID()
		deletionCleanup(t, unreadable)
		deletionSetRule(t, unreadable, deletion.KindMemberSelfDestruct, 600)
		deletionSetRule(t, env.chat.Id, deletion.KindMemberSelfDestruct, 600)
		// The batch writer answers as deletion.ScheduleBatch does when a chat's Staff Group read
		// fails: that chat's entries are dropped, the others are written, and the error is returned.
		mreqSwap(t, &memberDeleteBatchWrite, func(ctx context.Context, entries []deletion.Entry) (int, error) {
			var rest []deletion.Entry
			for _, e := range entries {
				if e.ChatID != unreadable {
					rest = append(rest, e)
				}
			}
			wrote, err := deletion.ScheduleBatch(ctx, rest)
			return wrote, errors.Join(err, errors.New("check chat for a Staff Group failed"))
		})

		memberDeleteOfferN(unreadable, 881301, 2, env.now)
		memberDeleteOfferN(env.chat.Id, 881401, 1, env.now)
		if got := memberDeleteTestFlush(t); got != 1 {
			t.Fatalf("memberDeleteFlush() = %d, want the other chat's 1", got)
		}
		if got := memberDeleteShedTotal(memberDeleteShedWrite); got != 2 {
			t.Fatalf("shed as a failed write = %d, want the 2 of the unreadable chat", got)
		}
		if rows := env.queuedNoWait(unreadable); len(rows) != 0 {
			t.Fatalf("rows of the chat whose Staff Group read failed = %+v, want none", rows)
		}
		if rows := env.queuedNoWait(env.chat.Id); len(rows) != 1 {
			t.Fatalf("rows of the other chat = %d, want 1", len(rows))
		}
	})
}

// TestShutdownEndsMemberDeletionBatcher: StopDeletionWorker ends the batcher inside the unchanged 3 s
// wait with no new drain. Entries offered just before the stop are written by the final flush, and a
// writer that hangs costs only those entries (no row: the messages stay), never the bound.
func TestShutdownEndsMemberDeletionBatcher(t *testing.T) {
	env := newDeletionEnv(t)
	drainMemberDeleteQueue(t)
	deletionDrainWake()
	mreqSwap(t, &deletionWorkerTick, 10*time.Millisecond)
	mreqSwap(t, &memberDeleteFlushEvery, time.Hour)
	deletionSetRule(t, env.chat.Id, deletion.KindMemberSelfDestruct, 600)
	deletionWorkerStopped(t)
	restoreMemberDeleteBatcher(t)
	margin := 500 * time.Millisecond

	stopWithin := func(t *testing.T) time.Duration {
		t.Helper()
		began := time.Now()
		StopDeletionWorker()
		elapsed := time.Since(began)
		if elapsed > deletionWorkerStopWait+margin {
			t.Fatalf("StopDeletionWorker took %s, want at most %s", elapsed, deletionWorkerStopWait+margin)
		}
		return elapsed
	}

	// The final flush writes what was offered just before the stop.
	StartDeletionWorker(env.bot)
	if got := memberDeleteOfferN(env.chat.Id, 881501, 5, env.now); got != 5 {
		t.Fatalf("accepted = %d, want 5", got)
	}
	stopWithin(t)
	rows := env.queuedNoWait(env.chat.Id)
	if len(rows) != 5 {
		t.Fatalf("rows after the stop = %d, want the 5 offered before it written by the final flush", len(rows))
	}
	if memberDeleteOffer(selfDestructEntry(env.chat.Id, 881510, env.now, 600)) {
		t.Fatal("an offer after the stop was accepted")
	}

	// A writer that hangs until its context ends: the stop still returns inside the bound.
	freshDeletionRoot()
	mreqSwap(t, &memberDeleteBatchWrite, func(ctx context.Context, _ []deletion.Entry) (int, error) {
		<-ctx.Done()
		return 0, ctx.Err()
	})
	StartDeletionWorker(env.bot)
	memberDeleteOfferN(env.chat.Id, 881601, 3, env.now)
	elapsed := stopWithin(t)
	if elapsed < memberDeleteFinalFlushWait/2 {
		t.Fatalf("StopDeletionWorker took %s, want it to wait for the final flush that hangs", elapsed)
	}
	for _, row := range env.queuedNoWait(env.chat.Id) {
		if row.MessageID >= 881601 {
			t.Fatalf("row %+v exists for an entry whose write hung, want none: the message stays", row)
		}
	}
}
