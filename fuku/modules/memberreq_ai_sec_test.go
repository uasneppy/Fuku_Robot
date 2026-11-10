//go:build testtools

package modules

import (
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// Phase 15 security SEC-01. The verdict cache is keyed by the canonical string, which is global to
// the fleet, so another account's breach of an identical string sits in the cache for 24 hours. The
// owner-facing text promises that members already in the group are checked only after a name change:
// an established member whose baseline equals their names and who has no marker must therefore pass
// at once, in the job as in the watcher, whatever the cache holds. Every other order keeps the
// verdict first (a joiner, a marker holder, a renamer, the held release and "I fixed it").

// mreqAIPlantBreach caches another account's breach (0.97) for the member's canonical names.
func mreqAIPlantBreach(t *testing.T, env *memberReqEnv, member gotgbot.User) {
	t.Helper()
	mreqAIPlant(t, env, mreqAIKeyOf(t, mreqAINamesOfUser(member)), "0.97")
}

func TestMemberReqAIEstablishedMemberIgnoresCachedBreach(t *testing.T) {
	jobEnv := func(t *testing.T) (*memberReqEnv, *mreqAIState) {
		t.Helper()
		return mreqAIEnvOf(t, map[memberreq.Rule]int{memberreq.RuleAI: models.RequirementActionMute}, false)
	}

	t.Run("an equal baseline and no marker pass at once: nobody is punished and the provider is not asked", func(t *testing.T) {
		env, ai := jobEnv(t)
		member := mreqAIOrdinary(env)
		mreqAIPlant(t, env, memberReqAIBaselineKey(member.Id), mreqAIHashOfUser(t, member))
		mreqAIPlantBreach(t, env, member)
		before := mreqAIVerdictKeys(t)

		got := env.mreqEnforce(env.mreqCase(member))

		mreqWantNothing(t, env, member, got)
		if n := ai.count(); n != 0 {
			t.Errorf("provider requests = %d, want none", n)
		}
		if after := mreqAIVerdictKeys(t); len(after) != len(before) {
			t.Errorf("verdict keys = %v, want the planted one only", after)
		}
		mreqAIWantMarker(t, env.chat.Id, member.Id, false)
		mreqAIWantBaseline(t, member.Id, mreqAIHashOfUser(t, member))
	})

	t.Run("the rule itself answers pass for an equal baseline and no marker", func(t *testing.T) {
		env, ai := jobEnv(t)
		member := mreqAIOrdinary(env)
		mreqAIPlant(t, env, memberReqAIBaselineKey(member.Id), mreqAIHashOfUser(t, member))
		mreqAIPlantBreach(t, env, member)

		if got := mreqAIAskAs(env, member.Id, mreqAINamesOfUser(member), memberReqLookupMode{}); got != memberReqPass {
			t.Errorf("verdict = %v, want pass", got)
		}
		if n := ai.count(); n != 0 {
			t.Errorf("provider requests = %d, want none", n)
		}
	})

	t.Run("a renamer with a cached breach is punished", func(t *testing.T) {
		env, ai := jobEnv(t)
		member := mreqAIOrdinary(env)
		mreqAIPlant(t, env, memberReqAIBaselineKey(member.Id), "0123456789abcdef0123456789abcdef")
		mreqAIPlantBreach(t, env, member)

		got := env.mreqEnforce(env.mreqCase(member))

		if got.Outcome != memberReqMuted || got.Hold == nil {
			t.Fatalf("outcome = %+v, want Muted: a baseline that differs is a rename", got)
		}
		if n := ai.count(); n != 0 {
			t.Errorf("provider requests = %d, want none: the cached verdict answers", n)
		}
		mreqAIWantMarker(t, env.chat.Id, member.Id, true)
	})

	t.Run("a member with no baseline and a cached breach is punished", func(t *testing.T) {
		env, _ := jobEnv(t)
		member := mreqAIOrdinary(env)
		mreqAIPlantBreach(t, env, member)

		got := env.mreqEnforce(env.mreqCase(member))

		if got.Outcome != memberReqMuted || got.Hold == nil {
			t.Fatalf("outcome = %+v, want Muted: no baseline is not a record that the names were accepted", got)
		}
	})

	t.Run("a marker holder with an equal baseline and a cached breach is punished", func(t *testing.T) {
		env, _ := jobEnv(t)
		member := mreqAIOrdinary(env)
		mreqAIPlant(t, env, memberReqAIBaselineKey(member.Id), mreqAIHashOfUser(t, member))
		mreqAIMarkNew(t, env.chat.Id, member.Id)
		mreqAIPlantBreach(t, env, member)

		got := env.mreqEnforce(env.mreqCase(member))

		if got.Outcome != memberReqMuted || got.Hold == nil {
			t.Fatalf("outcome = %+v, want Muted: the marker means the member still has to be judged", got)
		}
	})

	t.Run("a joiner with an equal baseline and a cached breach is punished", func(t *testing.T) {
		env, _ := jobEnv(t)
		member := mreqAIOrdinary(env)
		mreqAIPlant(t, env, memberReqAIBaselineKey(member.Id), mreqAIHashOfUser(t, member))
		mreqAIPlantBreach(t, env, member)
		join := env.mreqCase(member)
		join.Source, join.MessageID, join.MessageDate = memberReqFromJoin, 0, 0

		got := env.mreqEnforce(join)

		if got.Outcome != memberReqMuted || got.Hold == nil {
			t.Fatalf("outcome = %+v, want Muted: a rejoiner is a new membership", got)
		}
	})

	t.Run("a held member keeps the hold: IgnoreBaseline reads the cached breach", func(t *testing.T) {
		env, ai := mreqAIWatchEnv(t, models.RequirementActionWarn)
		member := mreqAIOrdinary(env)
		mreqSeedHeld(t, env, member, mreqBitOf(t, "ai"), models.RequirementActionWarn, 10*time.Minute)
		mreqAIPlant(t, env, memberReqAIBaselineKey(member.Id), mreqAIHashOfUser(t, member))
		mreqAIPlantBreach(t, env, member)

		got := env.mreqEnforce(env.mreqCase(member))

		if got.Outcome != memberReqHeld {
			t.Errorf("outcome = %+v, want Held", got)
		}
		if env.mreqHold(member.Id) == nil {
			t.Error("the hold was released although the cached verdict is a breach")
		}
		if n := ai.count(); n != 0 {
			t.Errorf("provider requests = %d, want none", n)
		}
		mreqHeldNoPunishment(t, env, member)
	})

	t.Run("the rule with IgnoreBaseline reads the cached breach", func(t *testing.T) {
		env, _ := jobEnv(t)
		member := mreqAIOrdinary(env)
		mreqAIPlant(t, env, memberReqAIBaselineKey(member.Id), mreqAIHashOfUser(t, member))
		mreqAIPlantBreach(t, env, member)

		got := mreqAIAskAs(env, member.Id, mreqAINamesOfUser(member), memberReqLookupMode{IgnoreBaseline: true})

		if got != memberReqBreach {
			t.Errorf("verdict = %v, want breach", got)
		}
	})
}
