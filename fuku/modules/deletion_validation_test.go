package modules

import (
	"context"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// deletionValidationPin delivers a pinned-message notice of the env chat dated sentAt through the
// real dispatcher and returns its message ID.
func deletionValidationPin(env *deletionEnv, sentAt time.Time) int64 {
	env.t.Helper()
	id := env.updateID()
	env.deliver(id, env.chat, &env.admin, func(m *gotgbot.Message) {
		m.Date = sentAt.Unix()
		m.PinnedMessage = gotgbot.Message{MessageId: 1, Chat: env.chat}
	})
	return id
}

// deletionValidationDeleted reports how many times the env's fake was asked to delete message id,
// by a batch or by a single call.
func deletionValidationDeleted(env *deletionEnv, id int64) int {
	n := 0
	for _, batch := range env.batchCalls(env.chat.Id) {
		for _, got := range batch {
			if got == id {
				n++
			}
		}
	}
	for _, got := range env.singleCalls(env.chat.Id) {
		if got == id {
			n++
		}
	}
	return n
}

// TestValidationServiceMessageDeliveredLate proves DEL-06's late-message promise for the service
// watcher: a service message that reaches the bot long after it was sent is timed from its own
// date (a delay that already ran out is due at once, never stretched from the arrival), and one
// that is already older than Telegram's window is dropped by the worker without a request.
func TestValidationServiceMessageDeliveredLate(t *testing.T) {
	env := newDeletionEnv(t)
	deletionSetRule(t, env.chat.Id, deletion.KindPinned, 600)

	t.Run("a message delivered 30 minutes late with a 10 minute rule is deleted by the next cycle", func(t *testing.T) {
		sent := env.now.Add(-30 * time.Minute)
		id := deletionValidationPin(env, sent)

		if calls := env.deletes(env.chat.Id); len(calls) != 0 {
			t.Fatalf("inline deletes for a rule of 600 s = %d, want none (the row carries the delay)", len(calls))
		}
		rows := env.queued(env.chat.Id)
		if len(rows) != 1 || rows[0].MessageID != id {
			t.Fatalf("queue rows = %+v, want exactly the late message %d", rows, id)
		}
		if !rows[0].SentAt.Equal(sent) || !rows[0].DeleteAt.Equal(sent.Add(600*time.Second)) {
			t.Fatalf("sent_at = %s, delete_at = %s, want the message's own date %s and +600 s (never counted from arrival)",
				rows[0].SentAt, rows[0].DeleteAt, sent)
		}

		env.pass()

		if got := deletionValidationDeleted(env, id); got != 1 {
			t.Fatalf("message %d was sent to Telegram %d times by the worker, want once", id, got)
		}
		if left := env.queuedNoWait(env.chat.Id); len(left) != 0 {
			t.Fatalf("rows left = %+v, want none", left)
		}
	})

	t.Run("a message older than 47 hours is queued but dropped without a request", func(t *testing.T) {
		sent := env.now.Add(-50 * time.Hour)
		id := deletionValidationPin(env, sent)

		rows := env.queued(env.chat.Id)
		if len(rows) != 1 || rows[0].MessageID != id {
			t.Fatalf("queue rows = %+v, want exactly message %d", rows, id)
		}

		env.pass()

		if got := deletionValidationDeleted(env, id); got != 0 {
			t.Fatalf("a 50 hour old message was sent to Telegram %d times, want never", got)
		}
		if left := env.queuedNoWait(env.chat.Id); len(left) != 0 {
			t.Fatalf("rows left = %+v, want none: the worker drops an over-age row quietly", left)
		}
	})
}

// TestValidationWorkerCycleRecoversFromPanic proves PLAT-06's "recovers from crashes" for the
// worker: a panic in the middle of a cycle is recovered, the rows the cycle claimed go back to
// pending with no token and no counted attempt, and the next cycle deletes every one of them once.
func TestValidationWorkerCycleRecoversFromPanic(t *testing.T) {
	env := newDeletionEnv(t)
	ids := env.seedDue(env.chat.Id, 61000, 4, deletion.KindBotWelcome)

	var panicked atomic.Int32
	env.script.set(func(_ context.Context, method string, _ map[string]any) error {
		if method == "deleteMessages" && panicked.Add(1) == 1 {
			panic("injected panic inside the worker's paced call")
		}
		return nil
	})

	env.cycle() // must return: a panic that escaped would fail the whole test binary

	if panicked.Load() != 1 {
		t.Fatalf("the injected panic ran %d times, want 1, so the case proves nothing", panicked.Load())
	}
	rows := env.queuedNoWait(env.chat.Id)
	if len(rows) != len(ids) {
		t.Fatalf("rows after the panicking cycle = %d, want all %d", len(rows), len(ids))
	}
	for _, row := range rows {
		if row.State != models.ScheduledDeletionPending || row.ClaimToken != "" || row.Attempts != 0 {
			t.Fatalf("row %+v after the panic, want pending with no claim token and no counted attempt", row)
		}
	}

	env.script.set(nil)
	env.pass()

	if left := env.queuedNoWait(env.chat.Id); len(left) != 0 {
		t.Fatalf("rows left after the next cycles = %+v, want none", left)
	}
	for _, id := range ids {
		// The panicking call may or may not have reached the fake before it panicked, so count
		// only what the cycles after the recovery sent: every ID at least once, none twice.
		if got := deletionValidationDeleted(env, id); got < 1 || got > 2 {
			t.Errorf("message %d was sent %d times, want it deleted after the recovery", id, got)
		}
	}
	if seen := slices.Concat(env.batchCalls(env.chat.Id)...); len(seen) < len(ids) {
		t.Fatalf("batch calls carried %d IDs, want at least %d", len(seen), len(ids))
	}
}

// TestValidationInlineDeleteRecoversFromPanic proves the same for the "delete now" goroutine: a
// panic inside it is recovered, releases its slot (a later wait returns) and the next service
// message is still deleted.
func TestValidationInlineDeleteRecoversFromPanic(t *testing.T) {
	env := newDeletionEnv(t)
	deletionSetRule(t, env.chat.Id, deletion.KindPinned, 0)

	var panicked atomic.Int32
	env.script.set(func(_ context.Context, method string, _ map[string]any) error {
		if method == "deleteMessage" && panicked.Add(1) == 1 {
			panic("injected panic inside the inline delete")
		}
		return nil
	})

	first := env.sendPin()
	finished := make(chan struct{})
	go func() {
		deletionInline.wait()
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(10 * time.Second):
		t.Fatal("the inline slot of the panicking delete was never released")
	}
	if panicked.Load() != 1 {
		t.Fatalf("the injected panic ran %d times, want 1, so the case proves nothing", panicked.Load())
	}
	if rows := env.queuedNoWait(env.chat.Id); len(rows) != 0 {
		t.Fatalf("queue rows after a recovered inline panic = %+v, want none (a crash may lose that one message, D-08)", rows)
	}

	second := env.sendPin()
	var deleted []int64
	for _, call := range env.deletes(env.chat.Id) {
		deleted = append(deleted, staffParamInt(call.Params, "message_id"))
	}
	if !slices.Contains(deleted, second) {
		t.Fatalf("deleted messages = %v, want the next notice %d to be deleted after the recovered panic (first was %d)", deleted, second, first)
	}
}
