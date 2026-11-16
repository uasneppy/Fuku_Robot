//go:build testtools

package modules

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// Phase 16 security hardening pass (16-SECURITY.md SEC-02 and SEC-05).

// deletionBotReads counts the live getChatMember reads of the bot itself in chatID.
func deletionBotReads(env *deletionEnv, chatID int64) int {
	n := 0
	for _, call := range env.fake.callsFor("getChatMember") {
		if staffParamInt(call.Params, "chat_id") == chatID && staffParamInt(call.Params, "user_id") == staffTestBotID {
			n++
		}
	}
	return n
}

// deletionRefuseInline makes every deleteMessage answer with a message-level refusal and the
// live read of the bot answer with memberErr (nil lets it through to the fake's record).
func deletionRefuseInline(env *deletionEnv, memberErr error) {
	env.script.set(func(_ context.Context, method string, _ map[string]any) error {
		switch method {
		case "deleteMessage":
			return deletionRefusal()
		case "getChatMember":
			return memberErr
		}
		return nil
	})
}

// deletionChatCheckKey is the Redis key that guards the inline latch decision of a chat.
func deletionChatCheckKey(chatID int64) string {
	return "fuku:delete:chk:" + strconv.FormatInt(chatID, 10)
}

// TestDeletionInlineRefusalDecidesLatch is SEC-02 part 1: a refused inline service delete decides
// the service latch through the live right check, at most once per chat per 10 minutes, so a chat
// where the bot can never delete costs one read and then no further calls.
func TestDeletionInlineRefusalDecidesLatch(t *testing.T) {
	const plainMember = `{"status":"member","user":{"id":999,"is_bot":true,"first_name":"Fuku"}}`

	t.Run("a bot that cannot delete latches the service kinds and later joins cost nothing", func(t *testing.T) {
		env := newDeletionEnv(t)
		env.fake.setBotMember(env.chat.Id, plainMember)
		deletionSetRule(t, env.chat.Id, deletion.KindPinned, 0)
		deletionRefuseInline(env, nil)

		env.sendPin()
		env.deletes(env.chat.Id)

		if got := deletionLatchValue(t, env, env.chat.Id); got != deletionScopeService {
			t.Fatalf("latch = %q, want %q", got, deletionScopeService)
		}
		if n := deletionBotReads(env, env.chat.Id); n != 1 {
			t.Fatalf("live reads of the bot = %d, want exactly 1", n)
		}
		before := len(env.deletes(env.chat.Id))
		env.sendPin()
		env.sendPin()
		if after := len(env.deletes(env.chat.Id)); after != before {
			t.Fatalf("deleteMessage calls after the latch grew from %d to %d", before, after)
		}
		if n := deletionBotReads(env, env.chat.Id); n != 1 {
			t.Fatalf("live reads of the bot after the latch = %d, want still 1", n)
		}
	})

	t.Run("an unknown answer latches nothing and the check is made once per 10 minutes", func(t *testing.T) {
		env := newDeletionEnv(t)
		deletionSetRule(t, env.chat.Id, deletion.KindPinned, 0)
		deletionRefuseInline(env, staffFakeError(500, "Internal Server Error"))

		for i := 0; i < 4; i++ {
			env.sendPin()
		}
		env.deletes(env.chat.Id)

		if got := deletionLatchValue(t, env, env.chat.Id); got != "" {
			t.Fatalf("latch = %q, want none", got)
		}
		if n := len(env.deletes(env.chat.Id)); n != 4 {
			t.Fatalf("deleteMessage calls = %d, want 4 (nothing latched, each refused alone)", n)
		}
		if n := deletionBotReads(env, env.chat.Id); n != 1 {
			t.Fatalf("live reads of the bot = %d, want exactly 1 for four refusals", n)
		}
		if ttl := env.redis.TTL(deletionChatCheckKey(env.chat.Id)); ttl <= 0 || ttl > 10*time.Minute {
			t.Fatalf("guard TTL = %s, want within (0, 10m]", ttl)
		}

		env.redis.FastForward(11 * time.Minute)
		env.sendPin()
		env.deletes(env.chat.Id)
		if n := deletionBotReads(env, env.chat.Id); n != 2 {
			t.Fatalf("live reads after the guard expired = %d, want 2", n)
		}
	})

	t.Run("a bot kind never asks about the right", func(t *testing.T) {
		env := newDeletionEnv(t)
		env.fake.setBotMember(env.chat.Id, plainMember)
		deletionRefuseInline(env, nil)

		deletionAct(env.bot, deletionAction{
			ChatID: env.chat.Id, MessageID: 77001, Kind: deletion.KindBotWelcome, SentAt: env.now, Delay: 0,
		})
		env.deletes(env.chat.Id)

		if n := deletionBotReads(env, env.chat.Id); n != 0 {
			t.Fatalf("live reads of the bot = %d, want 0", n)
		}
		if got := deletionLatchValue(t, env, env.chat.Id); got != "" {
			t.Fatalf("latch = %q, want none for a bot kind", got)
		}
		if env.redis.Exists(deletionChatCheckKey(env.chat.Id)) {
			t.Fatal("a bot kind took the check guard")
		}
	})

	t.Run("no Redis and a Redis error make no check", func(t *testing.T) {
		broken := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: -1, DialTimeout: 50 * time.Millisecond})
		t.Cleanup(func() { _ = broken.Close() })
		for name, client := range map[string]func() *redis.Client{
			"no Redis":    func() *redis.Client { return nil },
			"Redis error": func() *redis.Client { return broken },
		} {
			env := newDeletionEnv(t)
			env.fake.setBotMember(env.chat.Id, plainMember)
			deletionSetRule(t, env.chat.Id, deletion.KindPinned, 0)
			deletionRefuseInline(env, nil)
			mreqSwap(t, &deletionRedisClient, client)

			env.sendPin()
			env.deletes(env.chat.Id)

			if n := deletionBotReads(env, env.chat.Id); n != 0 {
				t.Errorf("%s: live reads of the bot = %d, want 0 (fail quiet)", name, n)
			}
			if n := len(env.deletes(env.chat.Id)); n != 1 {
				t.Errorf("%s: deleteMessage calls = %d, want the one refused call", name, n)
			}
		}
	})
}

// TestDeletionWorkerTrustsAdminFlagInSupergroups is SEC-02 part 2: for a supergroup or channel ID
// (at or below -1000000000000) the worker reads an administrator without the delete right as
// "cannot" and latches the service kinds; for a basic group's ID, and for any other ID, it still
// does not trust the flag, so an administrator is never latched there.
func TestDeletionWorkerTrustsAdminFlagInSupergroups(t *testing.T) {
	cases := []struct {
		name      string
		chatID    int64
		wantLatch string
	}{
		{"a supergroup ID", uniqueModuleChatID(), deletionScopeService},
		{"the first supergroup ID", -1000000000000, deletionScopeService},
		{"the last basic group ID", -999999999999, ""},
		{"a small basic group ID", -4242424, ""},
		{"a positive ID", 4242424, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newDeletionEnv(t)
			deletionCleanup(t, tc.chatID)
			env.fake.setBotMember(tc.chatID, lockdownBotAdminJSON(false, false, false))
			service := env.seedDue(tc.chatID, 51000, 3, deletion.KindPinned)
			perID := make(map[int64]error, len(service))
			for _, id := range service {
				perID[id] = deletionRefusal()
			}
			env.scriptDelete(deletionRefusal(), perID)

			env.pass()

			got, err := env.redis.Get(deletionLatchKey(tc.chatID))
			if err != nil {
				got = ""
			}
			if got != tc.wantLatch {
				t.Fatalf("latch = %q, want %q", got, tc.wantLatch)
			}
			var reads int
			for _, call := range env.fake.callsFor("getChatMember") {
				if staffParamInt(call.Params, "chat_id") == tc.chatID && staffParamInt(call.Params, "user_id") == staffTestBotID {
					reads++
				}
			}
			if reads != 1 {
				t.Fatalf("live reads of the bot = %d, want 1 per chat per cycle", reads)
			}
			if left := env.queuedNoWait(tc.chatID); len(left) != 0 {
				t.Fatalf("rows left = %+v, want none", left)
			}
		})
	}
}

// TestDeletionActRefusesNonNegativeChat is SEC-05: deletionAct refuses a chat ID that is not a
// group's before any read or write, on its inline branch and on its queue branch.
func TestDeletionActRefusesNonNegativeChat(t *testing.T) {
	for _, chatID := range []int64{0, 4242424, 1234567890123} {
		for _, delay := range []int{0, 600} {
			env := newDeletionEnv(t)
			deletionCleanup(t, chatID)
			staffReads := 0
			mreqSwap(t, &deletionStaffLookup, func(id int64) (*models.StaffGroup, error) {
				staffReads++
				return nil, nil
			})

			deletionAct(env.bot, deletionAction{
				ChatID: chatID, MessageID: 88001, Kind: deletion.KindBotWelcome, SentAt: env.now, Delay: delay,
			})

			if calls := env.deletes(chatID); len(calls) != 0 {
				t.Errorf("chat %d delay %d: deleteMessage calls = %d, want none", chatID, delay, len(calls))
			}
			if rows := env.queued(chatID); len(rows) != 0 {
				t.Errorf("chat %d delay %d: queue rows = %+v, want none", chatID, delay, rows)
			}
			if staffReads != 0 {
				t.Errorf("chat %d delay %d: Staff Group reads = %d, want none (refused before any read)", chatID, delay, staffReads)
			}
		}
	}
}
