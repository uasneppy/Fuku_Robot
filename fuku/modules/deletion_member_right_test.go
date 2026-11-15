//go:build testtools

package modules

import (
	"context"
	"testing"

	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
)

// Plan 17-04 (D-24, C-04): the delete-right latch and its live check cover every kind that needs the
// right to delete other people's messages, the service kinds and the three member kinds, and never a
// bot kind. The latch keeps its stored value `service` and its keys; only its meaning widened.

const deletionPlainMemberBot = `{"status":"member","user":{"id":999,"is_bot":true,"first_name":"Fuku"}}`

// deletionTotalRequests is how many delete requests of either method the env's fake has answered
// for chatID.
func deletionTotalRequests(env *deletionEnv, chatID int64) int {
	n := 0
	for _, method := range []string{"deleteMessage", "deleteMessages"} {
		for _, call := range env.fake.callsFor(method) {
			if staffParamInt(call.Params, "chat_id") == chatID {
				n++
			}
		}
	}
	return n
}

func TestMemberKindsNeedDeleteRight(t *testing.T) {
	inline := []struct {
		name string
		kind string
	}{
		{"commands", deletion.KindMemberCommands},
		{"edits", deletion.KindMemberEdited},
	}
	for _, tc := range inline {
		t.Run(tc.name, func(t *testing.T) {
			env := newDeletionEnv(t)
			env.fake.setBotMember(env.chat.Id, deletionPlainMemberBot)
			deletionRefuseInline(env, nil)

			deletionAct(env.bot, deletionAction{ChatID: env.chat.Id, MessageID: 81001, Kind: tc.kind, SentAt: env.now, Delay: 0})
			env.deletes(env.chat.Id)

			if got := deletionLatchValue(t, env, env.chat.Id); got != deletionScopeService {
				t.Fatalf("latch after the first refused %s = %q, want %q", tc.name, got, deletionScopeService)
			}
			if n := deletionBotReads(env, env.chat.Id); n != 1 {
				t.Fatalf("live reads of the bot = %d, want exactly 1", n)
			}

			deletionAct(env.bot, deletionAction{ChatID: env.chat.Id, MessageID: 81002, Kind: tc.kind, SentAt: env.now, Delay: 0})
			deletionAct(env.bot, deletionAction{ChatID: env.chat.Id, MessageID: 81003, Kind: deletion.KindMemberSelfDestruct, SentAt: env.now, Delay: 0})
			if n := len(env.deletes(env.chat.Id)); n != 1 {
				t.Fatalf("deleteMessage calls = %d, want the one refused call and none after the latch", n)
			}
			if n := deletionBotReads(env, env.chat.Id); n != 1 {
				t.Fatalf("live reads after the latch = %d, want still 1", n)
			}
			if rows := env.queuedNoWait(env.chat.Id); len(rows) != 0 {
				t.Fatalf("queue rows = %+v, want none: a latched chat writes no row", rows)
			}
		})
	}

	t.Run("self-destruct rows", func(t *testing.T) {
		env := newDeletionEnv(t)
		env.fake.setBotMember(env.chat.Id, deletionPlainMemberBot)
		// The worker screens claimed self-destruct rows against the stored rule and drops
		// them with no call when the rule is absent (D-26), so the rule must exist for the
		// cycle to reach the delete-right latch this subtest is about.
		deletionSetRule(t, env.chat.Id, deletion.KindMemberSelfDestruct, 600)
		rows := env.seedDue(env.chat.Id, 82000, 4, deletion.KindMemberSelfDestruct)
		perID := make(map[int64]error, len(rows))
		for _, id := range rows {
			perID[id] = deletionRefusal()
		}
		env.scriptDelete(deletionRefusal(), perID)

		env.pass()

		if got := deletionLatchValue(t, env, env.chat.Id); got != deletionScopeService {
			t.Fatalf("latch = %q, want %q", got, deletionScopeService)
		}
		if n := deletionBotReads(env, env.chat.Id); n != 1 {
			t.Fatalf("live reads of the bot = %d, want exactly 1 for the cycle", n)
		}
		if singles := env.singleCalls(env.chat.Id); len(singles) > 1 {
			t.Fatalf("single deletes = %v, want at most the one that was refused: the latch drops the rest", singles)
		}
		if left := env.queuedNoWait(env.chat.Id); len(left) != 0 {
			t.Fatalf("rows left = %+v, want none", left)
		}

		// Rows seeded after the latch are dropped by its screen with no call at all.
		before := deletionTotalRequests(env, env.chat.Id)
		env.seedDue(env.chat.Id, 82100, 3, deletion.KindMemberSelfDestruct)
		env.pass()
		if after := deletionTotalRequests(env, env.chat.Id); after != before {
			t.Fatalf("delete requests grew from %d to %d for rows of a latched chat", before, after)
		}
		if left := env.queuedNoWait(env.chat.Id); len(left) != 0 {
			t.Fatalf("rows left = %+v, want none", left)
		}
	})

	t.Run("pending member rows go with the decision", func(t *testing.T) {
		env := newDeletionEnv(t)
		env.fake.setBotMember(env.chat.Id, deletionPlainMemberBot)
		deletionRefuseInline(env, nil)
		pending := env.seedDue(env.chat.Id, 83000, 3, deletion.KindMemberSelfDestruct)
		keep := env.seedDue(env.chat.Id, 83100, 2, deletion.KindBotWelcome)

		deletionAct(env.bot, deletionAction{ChatID: env.chat.Id, MessageID: 83900, Kind: deletion.KindMemberCommands, SentAt: env.now, Delay: 0})
		env.deletes(env.chat.Id)

		left := env.queuedNoWait(env.chat.Id)
		if len(left) != len(keep) {
			t.Fatalf("rows left = %+v, want only the %d bot rows (member rows %v dropped with the latch)", left, len(keep), pending)
		}
		for _, row := range left {
			if row.Kind != deletion.KindBotWelcome {
				t.Fatalf("row %+v left, want only bot rows", row)
			}
		}
	})

	t.Run("bot kinds keep working", func(t *testing.T) {
		env := newDeletionEnv(t)
		env.fake.setBotMember(env.chat.Id, deletionPlainMemberBot)
		members := env.seedDue(env.chat.Id, 84000, 3, deletion.KindMemberSelfDestruct)
		bots := env.seedDue(env.chat.Id, 84100, 2, deletion.KindBotWelcome)
		deletionSetLatch(context.Background(), env.chat.Id, deletionScopeService)

		env.pass()

		batches := env.batchCalls(env.chat.Id)
		if len(batches) != 1 || len(batches[0]) != len(bots) {
			t.Fatalf("batch calls = %v, want one call of the bot rows only (members %v)", batches, members)
		}
		if n := deletionBotReads(env, env.chat.Id); n != 0 {
			t.Fatalf("live reads of the bot = %d, want 0: a bot kind never takes the check", n)
		}
		if left := env.queuedNoWait(env.chat.Id); len(left) != 0 {
			t.Fatalf("rows left = %+v, want none", left)
		}
	})

	t.Run("a bot with the right", func(t *testing.T) {
		env := newDeletionEnv(t)
		deletionRefuseInline(env, nil)

		for i, kind := range []string{deletion.KindMemberCommands, deletion.KindMemberEdited, deletion.KindMemberSelfDestruct} {
			deletionAct(env.bot, deletionAction{ChatID: env.chat.Id, MessageID: int64(85001 + i), Kind: kind, SentAt: env.now, Delay: 0})
		}
		env.deletes(env.chat.Id)

		if got := deletionLatchValue(t, env, env.chat.Id); got != "" {
			t.Fatalf("latch = %q, want none: the bot can delete, only this message was refused", got)
		}
		if n := deletionBotReads(env, env.chat.Id); n != 1 {
			t.Fatalf("live reads of the bot = %d, want 1 for three refusals (the guard)", n)
		}
		if !env.redis.Exists(deletionChatCheckKey(env.chat.Id)) {
			t.Fatal("the check guard was not taken")
		}
	})
}
