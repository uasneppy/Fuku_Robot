//go:build testtools

package modules

import (
	"context"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/redis/go-redis/v9"

	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// Phase 15 security SEC-02. The WR-04 fix stops the per-message job only while the breaker is
// paused. A spent group or fleet allowance, a gate that stayed full and a failed budget counter are
// sticky unknowns too, and the watcher cannot see them: each message of every marker holder was
// still a runner job and a paced live getChatMember. The AI row now latches the chat when it is
// refused for one of those reasons (the end of the UTC hour for the group's and the fleet's
// allowance, 60 seconds for a gate or a counter), and the watcher reads the latch as it reads the
// breaker. An account's own allowance latches nothing.

// mreqAILatchFirstMessage sends the marker holder's first message through the real dispatcher and
// real runner, so the AI row runs once and meets the refusal that the test armed. It fails the test
// when the provider was asked: every refusal comes before the call.
func mreqAILatchFirstMessage(t *testing.T, env *memberReqEnv, ai *mreqAIState, member gotgbot.User) {
	t.Helper()
	env.mreqSend(member, "hello")
	memberReqJobs.wait()
	if n := ai.count(); n != 0 {
		t.Fatalf("provider requests after the first message = %d, want none: the refusal comes before the call", n)
	}
}

// mreqAILatchMember mints a marker holder of the AI watch environment.
func mreqAILatchMember(t *testing.T, env *memberReqEnv) gotgbot.User {
	t.Helper()
	member := mreqAIOrdinary(env)
	mreqAIMarkNew(t, env.chat.Id, member.Id)
	return member
}

// mreqAILatchQuiet sends messages through the dispatcher with the runner refusing every job and
// fails the test unless the watcher started none, made no live request about the member and left
// the requirements keyspace as it was.
func mreqAILatchQuiet(t *testing.T, env *memberReqEnv, member gotgbot.User, messages int) {
	t.Helper()
	starts := mreqCountJobStarts(t)
	lookups := mreqLookupRequests(env, member)
	before := mreqAIKeyspace(t, env)
	for i := 0; i < messages; i++ {
		env.mreqSend(member, "hello")
	}
	memberReqJobs.wait()
	if n := starts(); n != 0 {
		t.Errorf("job starts = %d, want none while the chat is latched", n)
	}
	if n := mreqLookupRequests(env, member) - lookups; n != 0 {
		t.Errorf("Telegram requests about the member = %d, want none while the chat is latched", n)
	}
	mreqAISameSpace(t, before, mreqAIKeyspace(t, env))
}

// mreqAILatchStarts sends one message with the runner refusing every job and reports how many job
// starts the watcher tried.
func mreqAILatchStarts(t *testing.T, env *memberReqEnv, member gotgbot.User) int {
	t.Helper()
	starts := mreqCountJobStarts(t)
	env.mreqSend(member, "hello")
	memberReqJobs.wait()
	return starts()
}

func TestMemberReqWatcherAILatch(t *testing.T) {
	const messages = 6

	t.Run("a spent group allowance: six clean messages of a marker holder cost no job after the first", func(t *testing.T) {
		withAIBudgets(t, 5, 0, 400)
		env, ai := mreqAIWatchEnv(t, models.RequirementActionMute)
		member := mreqAILatchMember(t, env)
		mreqAILatchFirstMessage(t, env, ai, member)

		mreqAILatchQuiet(t, env, member, messages)
	})

	t.Run("a spent fleet allowance: six clean messages of a marker holder cost no job after the first", func(t *testing.T) {
		withAIBudgets(t, 5, 100, 0)
		env, ai := mreqAIWatchEnv(t, models.RequirementActionMute)
		member := mreqAILatchMember(t, env)
		mreqAILatchFirstMessage(t, env, ai, member)

		mreqAILatchQuiet(t, env, member, messages)
	})

	t.Run("a full gate latches the chat for 60 seconds and then releases it", func(t *testing.T) {
		clock := mreqAIInstallClock(t)
		withAIGate(t, 1, 50*time.Millisecond)
		if err := memberReqAIGate.Acquire(context.Background(), 1); err != nil {
			t.Fatalf("take the only slot: %v", err)
		}
		env, ai := mreqAIWatchEnv(t, models.RequirementActionMute)
		member := mreqAILatchMember(t, env)
		mreqAILatchFirstMessage(t, env, ai, member)

		mreqAILatchQuiet(t, env, member, messages)
		clock.advance(59 * time.Second)
		mreqAILatchQuiet(t, env, member, 1)
		clock.advance(2 * time.Second)
		memberReqAIGate.Release(1)
		if n := mreqAILatchStarts(t, env, member); n != 1 {
			t.Errorf("job starts after the 60 seconds = %d, want 1: the latch released", n)
		}
	})

	t.Run("a failed budget counter latches the chat for 60 seconds and then releases it", func(t *testing.T) {
		clock := mreqAIInstallClock(t)
		mreqSwap(t, &memberReqAITake, func(*redis.Client, string, int64) (bool, error) {
			return false, context.DeadlineExceeded
		})
		env, ai := mreqAIWatchEnv(t, models.RequirementActionMute)
		member := mreqAILatchMember(t, env)
		mreqAILatchFirstMessage(t, env, ai, member)

		mreqAILatchQuiet(t, env, member, messages)
		clock.advance(59 * time.Second)
		mreqAILatchQuiet(t, env, member, 1)
		clock.advance(2 * time.Second)
		if n := mreqAILatchStarts(t, env, member); n != 1 {
			t.Errorf("job starts after the 60 seconds = %d, want 1: the latch released", n)
		}
	})

	t.Run("an account's own allowance does not latch: the next member's message still starts a job", func(t *testing.T) {
		withAIBudgets(t, 0, 100, 400)
		env, ai := mreqAIWatchEnv(t, models.RequirementActionMute)
		first := mreqAILatchMember(t, env)
		mreqAILatchFirstMessage(t, env, ai, first)

		second := mreqAILatchMember(t, env)
		if n := mreqAILatchStarts(t, env, second); n != 1 {
			t.Errorf("job starts for the next member = %d, want 1: an account's limit is its own", n)
		}
		if n := mreqAILatchStarts(t, env, first); n != 1 {
			t.Errorf("job starts for the member whose allowance is spent = %d, want 1: nothing is latched", n)
		}
	})

	t.Run("a member held for an AI breach keeps the hold while the chat is latched", func(t *testing.T) {
		withAIBudgets(t, 5, 0, 400)
		env, ai := mreqAIWatchEnv(t, models.RequirementActionWarn)
		member := mreqAILatchMember(t, env)
		mreqSeedHeld(t, env, member, mreqBitOf(t, "ai"), models.RequirementActionWarn, 10*time.Minute)
		mreqAILatchFirstMessage(t, env, ai, member)

		mreqAILatchQuiet(t, env, member, messages)

		if env.mreqHold(member.Id) == nil {
			t.Error("the hold was deleted: the clean path ran for a member held for an AI breach")
		}
		if env.redis.Exists(env.mreqPassKeyOf(member.Id)) {
			t.Error("a pass key was written for a member who was never judged clean")
		}
		mreqHeldNoPunishment(t, env, member)
	})

	t.Run("another open reason still hands a job while the chat is latched", func(t *testing.T) {
		withAIBudgets(t, 5, 0, 400)
		env, state, ai := mreqAILookupEnv(t, map[memberreq.Rule]int{
			memberreq.RulePhoto: models.RequirementActionMute,
			memberreq.RuleAI:    models.RequirementActionMute,
		})
		noPhoto := mreqAIOrdinary(env)
		state.set(noPhoto.Id, mreqPhotoAnswer{})
		mreqAIMarkNew(t, env.chat.Id, noPhoto.Id)
		mreqAILatchFirstMessage(t, env, ai, noPhoto)

		if n := mreqAILatchStarts(t, env, noPhoto); n != 1 {
			t.Errorf("job starts for a member with no cached photo = %d, want 1: the photo rule is not latched", n)
		}
	})

	t.Run("after the hour rolls the next message starts one job and is judged", func(t *testing.T) {
		clock := mreqAIInstallClock(t)
		withAIBudgets(t, 5, 0, 400)
		env, ai := mreqAIWatchEnv(t, models.RequirementActionMute)
		member := mreqAILatchMember(t, env)
		mreqAILatchFirstMessage(t, env, ai, member)
		lookups := mreqLookupRequests(env, member)
		for i := 0; i < messages; i++ {
			env.mreqSend(member, "hello")
		}
		memberReqJobs.wait()
		if n := mreqLookupRequests(env, member) - lookups; n != 0 {
			t.Fatalf("Telegram requests about the member during the latch = %d, want none", n)
		}

		clock.advance(51 * time.Minute)
		memberReqAIChatHourlyBudget = 100
		env.mreqSend(member, "hello again")
		memberReqJobs.wait()

		if n := ai.count(); n != 1 {
			t.Errorf("provider requests after the hour rolled = %d, want 1: the member is judged", n)
		}
		mreqAIWantBaseline(t, member.Id, mreqAIHashOfUser(t, member))
		mreqAIWantMarker(t, env.chat.Id, member.Id, false)
	})
}
