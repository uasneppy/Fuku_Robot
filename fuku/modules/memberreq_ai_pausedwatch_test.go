//go:build testtools

package modules

import (
	"testing"
	"time"

	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// Phase 15 review WR-04. While the AI breaker is paused (a rejected key, billing, a long 429, a
// per-group trip) nothing can be judged, yet every member who still has a join marker is never
// "satisfied" in the watcher, so each of their messages was a runner job and a paced live
// getChatMember that ended in nothing. The watcher now reads the in-memory breaker: when the AI part
// is the only reason a member is not satisfied and it is paused for the chat, it starts no job and
// runs no clean path (satisfied would send the member down the clean path and delete the hold of a
// member held for an AI breach). Any other unsatisfied reason still hands a job.

// mreqAIPauseProvider pauses every group's name check the way a rejected key does.
func mreqAIPauseProvider() {
	memberReqAIRecordFailure(0, mreqAIFailure(401, "authentication_error"))
}

func TestMemberReqWatcherAIPaused(t *testing.T) {
	const messages = 6

	t.Run("six clean messages of a marker holder under a 401 pause start no job", func(t *testing.T) {
		env, ai := mreqAIWatchEnv(t, models.RequirementActionMute)
		member := mreqAIOrdinary(env)
		mreqAIMarkNew(t, env.chat.Id, member.Id)
		mreqAIPauseProvider()
		starts := mreqCountJobStarts(t)
		before := mreqAIKeyspace(t, env)

		for i := 0; i < messages; i++ {
			env.mreqSend(member, "hello")
		}
		memberReqJobs.wait()

		if n := starts(); n != 0 {
			t.Errorf("job starts = %d, want none while the check is paused", n)
		}
		mreqAISameSpace(t, before, mreqAIKeyspace(t, env))
		if n := ai.count(); n != 0 {
			t.Errorf("provider requests = %d, want none", n)
		}
	})

	t.Run("the same six messages cost no live lookup either", func(t *testing.T) {
		env, _ := mreqAIWatchEnv(t, models.RequirementActionMute)
		member := mreqAIOrdinary(env)
		mreqAIMarkNew(t, env.chat.Id, member.Id)
		mreqAIPauseProvider()

		for i := 0; i < messages; i++ {
			env.mreqSend(member, "hello")
		}
		memberReqJobs.wait()

		if n := mreqLookupRequests(env, member); n != 0 {
			t.Errorf("Telegram requests about the member = %d, want none while the check is paused", n)
		}
	})

	t.Run("a group's own pause counts as well", func(t *testing.T) {
		env, _ := mreqAIWatchEnv(t, models.RequirementActionMute)
		member := mreqAIOrdinary(env)
		mreqAIMarkNew(t, env.chat.Id, member.Id)
		mreqAITripGroup(env.chat.Id)
		starts := mreqCountJobStarts(t)

		for i := 0; i < messages; i++ {
			env.mreqSend(member, "hello")
		}
		memberReqJobs.wait()

		if n := starts(); n != 0 {
			t.Errorf("job starts = %d, want none while the group is paused", n)
		}
	})

	t.Run("a member held for an AI breach keeps the hold: no clean path runs", func(t *testing.T) {
		env, ai := mreqAIWatchEnv(t, models.RequirementActionWarn)
		member := mreqAIMember(env, "Casino")
		mreqSeedHeld(t, env, member, mreqBitOf(t, "ai"), models.RequirementActionWarn, 10*time.Minute)
		mreqAIMarkNew(t, env.chat.Id, member.Id)
		mreqAIPauseProvider()
		starts := mreqCountJobStarts(t)

		for i := 0; i < messages; i++ {
			env.mreqSend(member, "hello")
		}
		memberReqJobs.wait()

		if n := starts(); n != 0 {
			t.Errorf("job starts = %d, want none while the check is paused", n)
		}
		if env.mreqHold(member.Id) == nil {
			t.Error("the hold was deleted: the clean path ran for a member held for an AI breach")
		}
		if env.redis.Exists(env.mreqPassKeyOf(member.Id)) {
			t.Error("a pass key was written for a member who was never judged clean")
		}
		if n := ai.count(); n != 0 {
			t.Errorf("provider requests = %d, want none", n)
		}
		mreqHeldNoPunishment(t, env, member)
	})

	t.Run("after the pause ends the next message starts one job and is judged", func(t *testing.T) {
		env, ai := mreqAIWatchEnv(t, models.RequirementActionMute)
		member := mreqAIOrdinary(env)
		mreqAIMarkNew(t, env.chat.Id, member.Id)
		mreqAIPauseProvider()
		for i := 0; i < messages; i++ {
			env.mreqSend(member, "hello")
		}
		memberReqJobs.wait()
		if n := ai.count(); n != 0 {
			t.Fatalf("provider requests during the pause = %d, want none", n)
		}

		mreqAIFreshBreaker(t)
		env.mreqSend(member, "hello again")
		memberReqJobs.wait()

		if n := ai.count(); n != 1 {
			t.Errorf("provider requests after the pause = %d, want 1 (the member is judged)", n)
		}
		mreqAIWantBaseline(t, member.Id, mreqAIHashOfUser(t, member))
		mreqAIWantMarker(t, env.chat.Id, member.Id, false)
	})

	t.Run("another unsatisfied reason still hands a job, and a satisfied one does not", func(t *testing.T) {
		env, state, _ := mreqAILookupEnv(t, map[memberreq.Rule]int{
			memberreq.RulePhoto: models.RequirementActionMute,
			memberreq.RuleAI:    models.RequirementActionMute,
		})
		mreqAIPauseProvider()

		noPhoto := mreqAIOrdinary(env)
		state.set(noPhoto.Id, mreqPhotoAnswer{})
		mreqAIMarkNew(t, env.chat.Id, noPhoto.Id)
		starts := mreqCountJobStarts(t)
		env.mreqSend(noPhoto, "hello")
		memberReqJobs.wait()
		if n := starts(); n != 1 {
			t.Errorf("job starts for a member with no cached photo = %d, want 1: the photo rule is not paused", n)
		}

		withPhoto := mreqAIOrdinary(env)
		mreqAIMarkNew(t, env.chat.Id, withPhoto.Id)
		mreqAIPlant(t, env, memberReqPhotoKey(withPhoto.Id), "1")
		before := starts()
		env.mreqSend(withPhoto, "hello")
		memberReqJobs.wait()
		if n := starts() - before; n != 0 {
			t.Errorf("job starts for a member whose only open rule is the paused AI rule = %d, want none", n)
		}
	})
}
