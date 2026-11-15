//go:build testtools

package modules

import (
	"context"
	"errors"
	"testing"

	"github.com/redis/go-redis/v9"

	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// Phase 15 review WR-03. A Redis outage, a missing Redis client, a failed state read, a failed
// cache re-read and a budget counter fault are states of the system, not faults of one member's
// answer, so the AI row's unknown for them is sticky (D-22): it does not let a stricter AI action
// defer the weaker name punishment of every group while Redis is down. The verdict itself stays
// unknown, which the held release and "I fixed it" still refuse on.
func TestMemberReqAIRedisFaultsAreSticky(t *testing.T) {
	type cause struct {
		name string
		arm  func(t *testing.T, env *memberReqEnv)
	}
	causes := []cause{
		{"no Redis client", func(t *testing.T, _ *memberReqEnv) {
			mreqSwap(t, &memberReqRedisClient, func() *redis.Client { return nil })
		}},
		{"a failed state read", func(t *testing.T, env *memberReqEnv) {
			env.redis.SetError("connection refused")
			t.Cleanup(func() { env.redis.SetError("") })
		}},
		{"a budget counter fault", func(t *testing.T, _ *memberReqEnv) {
			mreqSwap(t, &memberReqAITake, func(*redis.Client, string, int64) (bool, error) {
				return false, errors.New("counter fault")
			})
		}},
	}
	for _, c := range causes {
		t.Run("last name Warn and AI Ban with "+c.name+": the Warn stands", func(t *testing.T) {
			env, ai := mreqAIEnvOf(t, map[memberreq.Rule]int{
				memberreq.RuleLastName: models.RequirementActionWarn,
				memberreq.RuleAI:       models.RequirementActionBan,
			}, false)
			member := mreqAIMember(env, "")
			mreqAIMarkNew(t, env.chat.Id, member.Id)
			c.arm(t, env)

			got := env.mreqEnforce(env.mreqCase(member))

			if got.Outcome != memberReqWarned {
				t.Fatalf("outcome = %+v, want the name rule's Warn to stand", got)
			}
			if hold := env.mreqHold(member.Id); hold == nil || hold.Rules != mreqBitOf(t, "ln") || hold.Action != models.RequirementActionWarn {
				t.Errorf("hold = %+v, want one with the last name bit only and the Warn action", hold)
			}
			if n := ai.count(); n != 0 {
				t.Errorf("provider requests = %d, want none", n)
			}
		})
	}

	t.Run("a non-sticky unknown with Redis up (a 500) still defers the Warn", func(t *testing.T) {
		env, ai := mreqAIEnvOf(t, map[memberreq.Rule]int{
			memberreq.RuleLastName: models.RequirementActionWarn,
			memberreq.RuleAI:       models.RequirementActionBan,
		}, false)
		ai.setDefault(mreqAIStatus(500, "api_error", ""))
		member := mreqAIMember(env, "")
		mreqAIMarkNew(t, env.chat.Id, member.Id)

		mreqAIWantNothing(t, env, member, env.mreqEnforce(env.mreqCase(member)))
	})

	t.Run("the verdict stays unknown in the lookup fold for every cause", func(t *testing.T) {
		for _, c := range causes {
			t.Run(c.name, func(t *testing.T) {
				env, _ := mreqAIEnvOf(t, map[memberreq.Rule]int{memberreq.RuleAI: models.RequirementActionBan}, false)
				member := mreqAIMember(env, "Casino")
				settings := env.mreqRules(map[memberreq.Rule]int{memberreq.RuleAI: models.RequirementActionBan})
				mreqAIMarkNew(t, env.chat.Id, member.Id)
				c.arm(t, env)
				broken, unknown := memberReqLookupBroken(context.Background(), env.bot, settings, env.chat.Id, member.Id,
					memberReqLookupMode{Names: mreqAINamesOfUser(member), IgnoreBaseline: true, Fresh: true, Classify: true})
				if broken != 0 || !unknown {
					t.Errorf("fold = broken %d unknown %v, want an unknown that is neither a pass nor a breach", broken, unknown)
				}
			})
		}
	})

	t.Run("a failed cache re-read inside the flight is a sticky unknown", func(t *testing.T) {
		env, ai := mreqAIEnvOf(t, map[memberreq.Rule]int{memberreq.RuleAI: models.RequirementActionBan}, false)
		member := mreqAIMember(env, "Casino")
		plan := memberReqAIPlan{canon: mreqAICanon(t, member.FirstName, member.LastName, member.Username)}
		plan.key = memberReqAIVerdictKey(raidAITestModel, plan.canon)
		mreqSwap(t, &memberReqRedisClient, func() *redis.Client { return nil })

		answer := memberReqAIJudge(context.Background(), env.chat.Id, member.Id, plan)

		if answer.Verdict != memberReqUnknown || !answer.Sticky {
			t.Errorf("answer = %+v, want a sticky unknown", answer)
		}
		if n := ai.count(); n != 0 {
			t.Errorf("provider requests = %d, want none", n)
		}
	})
}
