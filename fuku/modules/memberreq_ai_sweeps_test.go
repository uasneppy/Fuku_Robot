//go:build testtools

package modules

import (
	"testing"
	"time"

	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// The cross-cutting tests of the AI spam-name rule (plan 15-10): the join never waits longer than
// its lookup budget for a slow provider, the request holds exactly the three names and the
// instruction, and nothing the call shares can starve it or be starved by it. The shutdown proof is
// TestShutdownEndsMemberReqAI (shutdown_drains_test.go) and the log sweep is the AI part of
// TestMemberReqLookupLogsCarryIDsOnly (memberreq_lookup_budget_test.go). No real provider is called.

// mreqAIWaitVerdictKey waits until the stand-in's answer has been cached and fails after five seconds.
func mreqAIWaitVerdictKey(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for len(mreqAIVerdictKeys(t)) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the flight never cached its verdict")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestMemberReqAIJoinBudget: the join entry never parks a dispatcher goroutine longer than the join's
// lookup budget. A provider call slower than the budget leaves the member welcomed, unjudged and
// marked, the flight finishes on its own and caches the verdict, and the member's first message then
// judges from the cache with no second request. A slow photo lookup that spends the whole budget
// leaves the AI row unasked, and a message's job on a hung call never delays the update.
func TestMemberReqAIJoinBudget(t *testing.T) {
	t.Run("a call slower than the budget returns at the budget and the flight finishes by itself", func(t *testing.T) {
		env, ai := mreqAIJoinEnv(t, models.RequirementActionMute, true)
		mreqSwap(t, &memberReqLookupJoinBudget, 200*time.Millisecond)
		mreqSwap(t, &memberReqLookupFlightTimeout, 4*time.Second)
		const delay = 1500 * time.Millisecond
		ai.setDefault(mreqAIDelayed(delay, mreqAIProbability(0.97)))
		member := mreqAIMember(env, "Casino")

		started := time.Now()
		env.mreqJoin(member)
		took := time.Since(started)

		if took >= delay {
			t.Errorf("the join took %s, want it back at the 200 ms budget and not after the %s call", took, delay)
		}
		if keys := mreqAIVerdictKeys(t); len(keys) != 0 {
			t.Errorf("verdict keys = %v right after the join, want none: the call was still running when the join returned", keys)
		}
		mreqAIWantNothing(t, env, member, memberReqResult{})
		if got := mreqAIWelcomes(env); got != 1 {
			t.Errorf("welcomes = %d, want 1 (a joiner whose call outlived the budget is welcomed)", got)
		}
		mreqAIWantMarker(t, env.chat.Id, member.Id, true)

		// The flight outlived its caller: it finishes and caches the verdict on its own.
		mreqAIWaitVerdictKey(t)
		if got := ai.count(); got != 1 {
			t.Errorf("provider requests = %d, want 1", got)
		}

		// The first message judges from the cache: no second request, and the spam verdict punishes.
		env.mreqSend(member, "buy now")
		memberReqJobs.wait()
		if got := ai.count(); got != 1 {
			t.Errorf("provider requests after the first message = %d, want 1: the verdict was cached", got)
		}
		hold := env.mreqHold(member.Id)
		if hold == nil || hold.Rules != mreqBitOf(t, "ai") {
			t.Errorf("hold = %+v, want a hold with the AI bit", hold)
		}
		if n := env.mreqCallCount("restrictChatMember"); n != 1 {
			t.Errorf("restrict calls = %d, want 1 (the mute)", n)
		}
	})

	t.Run("a photo lookup that spends the whole budget leaves the AI row unasked", func(t *testing.T) {
		withAIConfigured(t)
		ai := mreqInstallAIFake(t)
		mreqAICleanup(t, nil, nil)
		env := mreqJoinEnv(t, map[memberreq.Rule]int{
			memberreq.RulePhoto: models.RequirementActionMute,
			memberreq.RuleAI:    models.RequirementActionMute,
		}, true)
		withFastLookupPacer(t)
		state := newMreqLookupState()
		mreqInstallLookupFake(env, state)
		ai.setDefault(mreqAIProbability(0.97))
		mreqSwap(t, &memberReqLookupJoinBudget, 200*time.Millisecond)
		mreqSwap(t, &memberReqLookupFlightTimeout, 600*time.Millisecond)
		member := mreqAIMember(env, "Casino")
		mreqLookupCleanup(t, []int64{member.Id}, nil)
		state.set(member.Id, mreqPhotoAnswer{})
		release := state.hold(member.Id)
		t.Cleanup(release)
		// Runs before the row cleanups: the abandoned photo flight has ended by its own timeout.
		t.Cleanup(func() { time.Sleep(700 * time.Millisecond) })

		started := time.Now()
		env.mreqJoin(member)

		if took := time.Since(started); took > 3*time.Second {
			t.Errorf("the join took %s, want it back at about the 200 ms budget", took)
		}
		if got := ai.count(); got != 0 {
			t.Errorf("provider requests = %d, want none: the photo lookup spent the whole join budget", got)
		}
		mreqAIWantNothing(t, env, member, memberReqResult{})
		if got := mreqAIWelcomes(env); got != 1 {
			t.Errorf("welcomes = %d, want 1 (a joiner whose lookups could not finish is welcomed)", got)
		}
		mreqAIWantMarker(t, env.chat.Id, member.Id, true)
	})

	t.Run("a message job on a hung call never delays the update", func(t *testing.T) {
		env, ai := mreqAIWatchEnv(t, models.RequirementActionMute)
		ai.setDefault(mreqAIHang())
		member := mreqAIMember(env, "Casino")
		mreqAIMarkNew(t, env.chat.Id, member.Id)

		returned := make(chan struct{})
		go func() {
			defer close(returned)
			env.mreqSend(member, "buy now")
		}()
		select {
		case <-returned:
		case <-time.After(3 * time.Second):
			t.Fatal("the update waited for the provider: the watcher must hand the call to a job and return")
		}
		ai.waitCount(t, 1)
		if hold := env.mreqHold(member.Id); hold != nil {
			t.Errorf("hold = %+v while the call is held, want none", hold)
		}

		// Release the hung request: it answers nothing, which is unknown, and nobody is punished.
		ai.releaseHangs()
		memberReqJobs.wait()
		mreqAIWantNothing(t, env, member, memberReqResult{})
	})
}
