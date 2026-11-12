//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/redis/go-redis/v9"

	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/utils/ratelimit"
)

func TestDeletionClassify(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	migrated := &gotgbot.TelegramError{
		Code:           400,
		Description:    "Bad Request: something",
		ResponseParams: &gotgbot.ResponseParameters{MigrateToChatId: -1001234567890},
	}
	cases := []struct {
		name string
		ctx  context.Context
		err  error
		want deletionVerdict
	}{
		{"nil is done", context.Background(), nil, deletionVerdictDone},
		{"the pacer's rate limit", context.Background(), ratelimit.ErrRateLimited, deletionVerdictRetryFree},
		{"a wrapped rate limit", context.Background(), fmt.Errorf("pace: %w", ratelimit.ErrRateLimited), deletionVerdictRetryFree},
		{"a cancelled context", cancelled, errors.New("anything"), deletionVerdictRetryFree},
		{"a cancelled error on a live context", context.Background(), context.Canceled, deletionVerdictRetryFree},
		{"a 429 that reached the classifier", context.Background(), staffFake429(3), deletionVerdictRetryFree},
		{"message to delete not found", context.Background(), staffFakeError(400, "Bad Request: message to delete not found"), deletionVerdictRefused},
		{"message can't be deleted", context.Background(), staffFakeError(400, "Bad Request: message can't be deleted"), deletionVerdictRefused},
		{"MESSAGE_ID_INVALID", context.Background(), staffFakeError(400, "Bad Request: MESSAGE_ID_INVALID"), deletionVerdictRefused},
		{"not enough rights", context.Background(), staffFakeError(403, "Forbidden: not enough rights"), deletionVerdictRefused},
		{"a wrapped refusal", context.Background(), fmt.Errorf("delete: %w", staffFakeError(400, "Bad Request: message can't be deleted")), deletionVerdictRefused},
		{"bot was kicked", context.Background(), staffFakeError(403, "Forbidden: bot was kicked from the supergroup chat"), deletionVerdictGone},
		{"bot is not a member", context.Background(), staffFakeError(403, "Forbidden: bot is not a member of the supergroup chat"), deletionVerdictGone},
		{"chat not found", context.Background(), staffFakeError(400, "Bad Request: chat not found"), deletionVerdictGone},
		{"CHAT_NOT_FOUND", context.Background(), staffFakeError(400, "Bad Request: CHAT_NOT_FOUND"), deletionVerdictGone},
		{"PEER_ID_INVALID", context.Background(), staffFakeError(400, "Bad Request: PEER_ID_INVALID"), deletionVerdictGone},
		{"group chat was deactivated", context.Background(), staffFakeError(403, "Forbidden: group chat was deactivated"), deletionVerdictGone},
		{"upgraded to a supergroup", context.Background(), staffFakeError(400, "Bad Request: group chat was upgraded to a supergroup chat"), deletionVerdictGone},
		{"a migrate_to_chat_id parameter", context.Background(), migrated, deletionVerdictGone},
		{"a 500", context.Background(), staffFakeError(500, "Internal Server Error"), deletionVerdictRetry},
		{"a transport error", context.Background(), errors.New("dial tcp: connection refused"), deletionVerdictRetry},
		{"a deadline of the call itself", context.Background(), context.DeadlineExceeded, deletionVerdictRetry},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := deletionClassify(tc.ctx, tc.err); got != tc.want {
				t.Fatalf("deletionClassify(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestDeletionLatch(t *testing.T) {
	ctx := context.Background()
	serviceKind, botKind := deletion.KindPinned, deletion.KindBotWelcome

	t.Run("a gone latch stops every kind", func(t *testing.T) {
		env := newDeletionEnv(t)
		deletionSetLatch(ctx, env.chat.Id, deletionScopeGone)
		for _, kind := range []string{serviceKind, botKind} {
			if !deletionLatched(ctx, env.chat.Id, kind) {
				t.Fatalf("a gone latch does not stop %s", kind)
			}
		}
		if ttl := env.redis.TTL(deletionLatchKey(env.chat.Id)); ttl != deletionLatchTTL {
			t.Fatalf("latch TTL = %s, want %s", ttl, deletionLatchTTL)
		}
		env.redis.FastForward(deletionLatchTTL + time.Second)
		if deletionLatched(ctx, env.chat.Id, serviceKind) {
			t.Fatal("the latch is still set after its 10 minutes")
		}
	})

	t.Run("the service scope stops only service kinds", func(t *testing.T) {
		env := newDeletionEnv(t)
		deletionSetLatch(ctx, env.chat.Id, deletionScopeService)
		if !deletionLatched(ctx, env.chat.Id, serviceKind) {
			t.Fatal("a service latch does not stop a service kind")
		}
		if deletionLatched(ctx, env.chat.Id, botKind) {
			t.Fatal("a service latch stops a bot kind: a plain-member bot can still delete its own messages")
		}
	})

	t.Run("a gone latch is never downgraded by a service latch", func(t *testing.T) {
		env := newDeletionEnv(t)
		deletionSetLatch(ctx, env.chat.Id, deletionScopeGone)
		deletionSetLatch(ctx, env.chat.Id, deletionScopeService)
		if !deletionLatched(ctx, env.chat.Id, botKind) {
			t.Fatal("the service latch downgraded the gone latch")
		}
		other := env.chat.Id - 1
		deletionSetLatch(ctx, other, deletionScopeService)
		deletionSetLatch(ctx, other, deletionScopeGone)
		defer env.redis.Del(deletionLatchKey(other))
		if !deletionLatched(ctx, other, botKind) {
			t.Fatal("a gone latch did not overwrite a service latch")
		}
	})

	t.Run("no Redis and a Redis error read as not latched", func(t *testing.T) {
		env := newDeletionEnv(t)
		deletionSetLatch(ctx, env.chat.Id, deletionScopeGone)
		mreqSwap(t, &deletionRedisClient, func() *redis.Client { return nil })
		if deletionLatched(ctx, env.chat.Id, serviceKind) {
			t.Fatal("a latch is read with no Redis")
		}
		broken := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: -1, DialTimeout: 50 * time.Millisecond})
		t.Cleanup(func() { _ = broken.Close() })
		mreqSwap(t, &deletionRedisClient, func() *redis.Client { return broken })
		if deletionLatched(ctx, env.chat.Id, serviceKind) {
			t.Fatal("a latch is read through a Redis error")
		}
	})

	t.Run("the inline path latches a gone chat and never latches a refused message", func(t *testing.T) {
		env := newDeletionEnv(t)
		deletionSetRule(t, env.chat.Id, deletion.KindPinned, 0)

		env.fake.script("deleteMessage", env.chat.Id, staffFakeError(400, "Bad Request: message can't be deleted"))
		env.sendPin()
		env.deletes(env.chat.Id)
		if env.redis.Exists(deletionLatchKey(env.chat.Id)) {
			t.Fatal("a refused message set a latch")
		}

		env.fake.script("deleteMessage", env.chat.Id, staffFakeError(403, "Forbidden: bot was kicked from the supergroup chat"))
		env.sendPin()
		env.deletes(env.chat.Id)
		if got, err := env.redis.Get(deletionLatchKey(env.chat.Id)); err != nil || got != deletionScopeGone {
			t.Fatalf("latch after a kicked bot = %q (%v), want %q", got, err, deletionScopeGone)
		}

		// A latched chat is left alone, whatever the rule says.
		before := len(env.deletes(env.chat.Id))
		env.sendPin()
		if after := len(env.deletes(env.chat.Id)); after != before {
			t.Fatalf("deleteMessage calls in a latched chat grew from %d to %d", before, after)
		}
		deletionSetRule(t, env.chat.Id, deletion.KindPinned, 600)
		env.sendPin()
		if rows := env.queued(env.chat.Id); len(rows) != 0 {
			t.Fatalf("queue rows for a latched chat = %d, want none", len(rows))
		}
	})
}

func TestDeletionInlineCapFallsBackToQueue(t *testing.T) {
	env := newDeletionEnv(t)
	hang := &deletionHangClient{BotClient: env.fake}
	env.bot.BotClient = hang
	hang.hang.Store(true)
	restoreDeletionWorker(t)
	mreqSwap(t, &deletionInlineMax, 2)
	deletionSetRule(t, env.chat.Id, deletion.KindPinned, 0)

	// Two inline deletes park inside the pacer call; the next two take the durable path.
	for len(deletionWake) > 0 {
		<-deletionWake
	}
	env.sendPin()
	env.sendPin()
	third := env.sendPin()
	fourth := env.sendPin()
	if running := deletionInlineRunning(); running != 2 {
		t.Fatalf("inline deletes in flight = %d, want the cap of 2 (no goroutine is parked beyond it)", running)
	}
	rows := env.queuedNoWait(env.chat.Id)
	if len(rows) != 2 || rows[0].MessageID != third || rows[1].MessageID != fourth {
		t.Fatalf("queue rows = %+v, want messages %d and %d due now", rows, third, fourth)
	}
	for _, row := range rows {
		if !row.DeleteAt.Equal(env.now) {
			t.Fatalf("fallback row due %s, want now (%s)", row.DeleteAt, env.now)
		}
	}
	select {
	case <-deletionWake:
	default:
		t.Fatal("the worker was not woken for a row due now")
	}

	// Cancelling the root context ends every parked goroutine.
	cancelDeletionContext()
	select {
	case <-deletionInline.idleChan():
	case <-time.After(5 * time.Second):
		t.Fatalf("%d inline deletes are still in flight 5 s after the root was cancelled", deletionInlineRunning())
	}
	if rows := env.queued(env.chat.Id); len(rows) != 2 {
		t.Fatalf("queue rows after the cancel = %d, want 2 (a cancelled delete is lost, never queued)", len(rows))
	}
}

func TestDeletionInlineShedFallsBackToQueue(t *testing.T) {
	env := newDeletionEnv(t)
	deletionSetRule(t, env.chat.Id, deletion.KindPinned, 0)
	// A block longer than the pacer's MaxWait sheds the call at once, with no Telegram request.
	env.redis.Set("fuku:delete:pace:block", "1")
	env.redis.SetTTL("fuku:delete:pace:block", 120*time.Second)
	t.Cleanup(func() { env.redis.Del("fuku:delete:pace:block") })

	id := env.sendPin()
	if calls := env.deletes(env.chat.Id); len(calls) != 0 {
		t.Fatalf("deleteMessage calls under a shed pacer = %d, want 0", len(calls))
	}
	rows := env.queued(env.chat.Id)
	if len(rows) != 1 || rows[0].MessageID != id || !rows[0].DeleteAt.Equal(env.now) {
		t.Fatalf("queue rows = %+v, want one due now for message %d", rows, id)
	}
}

func TestDeletionPacerIsIndependent(t *testing.T) {
	env := newDeletionEnv(t)
	ctx := context.Background()
	others := []struct {
		name  string
		key   string
		paced func(context.Context, func(context.Context) error) error
	}{
		{"member requirements", "fuku:memberreq:pace:block", memberReqPaced},
		{"lockdown", "fuku:lockdown:pace:block", lockdownPaced},
		{"staff", "fuku:staff:pace:block", staffPaced},
	}
	block := func(key string) {
		env.redis.Set(key, "1")
		env.redis.SetTTL(key, 30*time.Second)
		t.Cleanup(func() { env.redis.Del(key) })
	}
	quick := func(what string, paced func(context.Context, func(context.Context) error) error) {
		t.Helper()
		started := time.Now()
		var ran bool
		err := paced(ctx, func(context.Context) error {
			ran = true
			return nil
		})
		if err != nil || !ran {
			t.Fatalf("%s call: ran = %v, err = %v", what, ran, err)
		}
		if took := time.Since(started); took > 2*time.Second {
			t.Fatalf("%s call took %s under another pacer's 30 s block, want it unaffected", what, took)
		}
	}

	block("fuku:delete:pace:block")
	for _, other := range others {
		quick(other.name, other.paced)
	}
	env.redis.Del("fuku:delete:pace:block")

	for _, other := range others {
		block(other.key)
	}
	quick("deletion", deletionPaced)
}

func TestDeletionDelayCapped(t *testing.T) {
	env := newDeletionEnv(t)
	ctx := context.Background()
	for _, delay := range []int{-1, deletion.MaxDelaySecs + 1} {
		t.Run(strconv.Itoa(delay), func(t *testing.T) {
			written, err := deletionSchedule(ctx, deletion.Entry{
				ChatID: env.chat.Id, MessageID: 900, Kind: deletion.KindPinned, SentAt: env.now, DelaySecs: delay,
			})
			if written || err == nil {
				t.Fatalf("deletionSchedule with delay %d = (%v, %v), want (false, an error)", delay, written, err)
			}
			if rows := env.queued(env.chat.Id); len(rows) != 0 {
				t.Fatalf("queue rows after a refused delay = %d, want none", len(rows))
			}
		})
	}
	written, err := deletionSchedule(ctx, deletion.Entry{
		ChatID: env.chat.Id, MessageID: 901, Kind: deletion.KindPinned, SentAt: env.now, DelaySecs: deletion.MaxDelaySecs,
	})
	if !written || err != nil {
		t.Fatalf("deletionSchedule with the largest delay = (%v, %v), want (true, nil)", written, err)
	}
}
