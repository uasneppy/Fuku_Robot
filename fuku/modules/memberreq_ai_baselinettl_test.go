//go:build testtools

package modules

import (
	"testing"
	"time"

	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// Phase 15 review WR-02. The baseline is an absolute TTL that nothing refreshes (the watcher is
// read-only and an equal baseline is a no-write pass), so a short TTL lapses for every member and
// the next sighting re-baselines whatever names they carry then. The baseline lives 180 days, so
// only an account dormant for longer than that is first-sighted again (the accepted D-02 limit);
// the marker keeps its 30 days. The expectations are literal day counts, not the package variables,
// so the test pins the rulebook and not itself.
func TestMemberReqAIBaselineTTL(t *testing.T) {
	const day = 24 * time.Hour
	wantAbout := func(t *testing.T, what string, ttl, want time.Duration) {
		t.Helper()
		if ttl <= want-time.Hour || ttl > want {
			t.Errorf("%s ttl = %v, want about %v", what, ttl, want)
		}
	}
	jobEnv := func(t *testing.T) (*memberReqEnv, *mreqAIState) {
		t.Helper()
		return mreqAIEnvOf(t, map[memberreq.Rule]int{memberreq.RuleAI: models.RequirementActionMute}, false)
	}

	t.Run("a first sighting records the baseline for 180 days", func(t *testing.T) {
		env, ai := jobEnv(t)
		member := mreqAIOrdinary(env)
		if got := mreqAIAskAs(env, member.Id, mreqAINamesOfUser(member), memberReqLookupMode{}); got != memberReqPass {
			t.Fatalf("verdict = %v, want pass", got)
		}
		if n := ai.count(); n != 0 {
			t.Errorf("provider requests = %d, want none for a first sighting", n)
		}
		_, ttl, ok := mreqAIBaselineOf(t, member.Id)
		if !ok {
			t.Fatal("no baseline was recorded")
		}
		wantAbout(t, "baseline", ttl, 180*day)
	})

	t.Run("a definite pass writes the baseline for 180 days and ends the marker", func(t *testing.T) {
		env, _ := jobEnv(t)
		member := mreqAIOrdinary(env)
		mreqAIMarkNew(t, env.chat.Id, member.Id)
		if got := mreqAIAskAs(env, member.Id, mreqAINamesOfUser(member), memberReqLookupMode{}); got != memberReqPass {
			t.Fatalf("verdict = %v, want pass", got)
		}
		_, ttl, ok := mreqAIBaselineOf(t, member.Id)
		if !ok {
			t.Fatal("no baseline was recorded")
		}
		wantAbout(t, "baseline", ttl, 180*day)
		mreqAIWantMarker(t, env.chat.Id, member.Id, false)
	})

	t.Run("a breach keeps the marker at 30 days and writes no baseline", func(t *testing.T) {
		env, ai := jobEnv(t)
		ai.setDefault(mreqAIProbability(0.97))
		member := mreqAIMember(env, "Casino")
		if got := mreqAIAskAs(env, member.Id, mreqAINamesOfUser(member), memberReqLookupMode{Joining: true}); got != memberReqBreach {
			t.Fatalf("verdict = %v, want breach", got)
		}
		_, ttl, ok := mreqAIMarkerOf(t, env.chat.Id, member.Id)
		if !ok {
			t.Fatal("no marker was written")
		}
		wantAbout(t, "marker", ttl, 30*day)
		mreqAIWantNoBaseline(t, member.Id)
	})

	t.Run("an equal baseline is still a pass with no write", func(t *testing.T) {
		env, _ := jobEnv(t)
		member := mreqAIOrdinary(env)
		mreqAIPlant(t, env, memberReqAIBaselineKey(member.Id), mreqAIHashOfUser(t, member))
		env.redis.SetTTL(memberReqAIBaselineKey(member.Id), time.Hour)
		if got := mreqAIAskAs(env, member.Id, mreqAINamesOfUser(member), memberReqLookupMode{}); got != memberReqPass {
			t.Fatalf("verdict = %v, want pass", got)
		}
		if ttl := env.redis.TTL(memberReqAIBaselineKey(member.Id)); ttl > time.Hour {
			t.Errorf("baseline ttl = %v, want it not rewritten", ttl)
		}
	})
}
