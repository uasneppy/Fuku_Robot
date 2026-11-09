//go:build testtools

package modules

import (
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/redis/go-redis/v9"

	"github.com/uasneppy/fuku_robot/fuku/config"
	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// The baseline, the new-member marker and joining (plan 15-05; D-02, D-21). An established
// member's first sighting is recorded without a call, a member the bot saw join is judged at join
// or at their first message, a rename is judged again, and a flagged member is never laundered by
// an expired baseline.

var mreqAIHashPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

// mreqAIOrdinary mints a member of the environment's group with ordinary names that carry its
// user ID, so no two tests share a verdict key or a baseline.
func mreqAIOrdinary(env *memberReqEnv) gotgbot.User {
	env.t.Helper()
	member := env.mreqMember("Alex", "Smith", "alexsmith")
	member.FirstName = fmt.Sprintf("Alex%d", member.Id)
	member.Username = fmt.Sprintf("alexsmith%d", member.Id)
	env.fake.setProfile(member.Id, member.FirstName, member.LastName, member.Username)
	return member
}

// mreqAINamesOfUser are the names of a user as the rule receives them.
func mreqAINamesOfUser(user gotgbot.User) *memberReqNames {
	names := memberReqNamesOf(user)
	return &names
}

// mreqAIHashOfUser is the names hash of a user's current names.
func mreqAIHashOfUser(t *testing.T, user gotgbot.User) string {
	t.Helper()
	return mreqAICanon(t, user.FirstName, user.LastName, user.Username).Hash
}

// mreqAIWantBaseline fails the test unless the user's baseline holds want and lives about 180 days
// (memberReqAIBaselineTTL, WR-02).
func mreqAIWantBaseline(t *testing.T, userID int64, want string) {
	t.Helper()
	value, ttl, ok := mreqAIBaselineOf(t, userID)
	if !ok || value != want {
		t.Fatalf("baseline = %q (present %t), want %q", value, ok, want)
	}
	if !mreqAIHashPattern.MatchString(value) {
		t.Errorf("baseline %q is not a 32 hex hash", value)
	}
	if ttl <= memberReqAIBaselineTTL-time.Hour || ttl > memberReqAIBaselineTTL {
		t.Errorf("baseline ttl = %v, want about %v", ttl, memberReqAIBaselineTTL)
	}
}

// mreqAIWantNoBaseline fails the test when the user has a baseline.
func mreqAIWantNoBaseline(t *testing.T, userID int64) {
	t.Helper()
	if value, _, ok := mreqAIBaselineOf(t, userID); ok {
		t.Errorf("baseline = %q, want none", value)
	}
}

// mreqAIWantMarker fails the test unless the member's marker is set (value 1, about 30 days) or,
// for want false, unless there is none.
func mreqAIWantMarker(t *testing.T, chatID, userID int64, want bool) {
	t.Helper()
	value, ttl, ok := mreqAIMarkerOf(t, chatID, userID)
	if ok != want {
		t.Fatalf("marker present = %t, want %t", ok, want)
	}
	if ok && (value != "1" || ttl <= memberReqAIStateTTL-time.Hour || ttl > memberReqAIStateTTL) {
		t.Errorf("marker = %q ttl %v, want 1 for about %v", value, ttl, memberReqAIStateTTL)
	}
}

func TestMemberReqAIBaseline(t *testing.T) {
	jobEnv := func(t *testing.T) (*memberReqEnv, *mreqAIState) {
		t.Helper()
		return mreqAIEnvOf(t, map[memberreq.Rule]int{memberreq.RuleAI: models.RequirementActionMute}, false)
	}

	t.Run("an established member's first sighting records a baseline and asks nobody", func(t *testing.T) {
		env, ai := jobEnv(t)
		ai.setDefault(mreqAIProbability(0.99))
		member := mreqAIOrdinary(env)
		hash := mreqAIHashOfUser(t, member)

		mreqAIWantNothing(t, env, member, env.mreqEnforce(env.mreqCase(member)))
		if n := ai.count(); n != 0 {
			t.Errorf("provider requests = %d, want none for a first sighting", n)
		}
		mreqAIWantBaseline(t, member.Id, hash)
		mreqAIWantMarker(t, env.chat.Id, member.Id, false)
		if value, _, _ := mreqAIBaselineOf(t, member.Id); value == member.FirstName || value == member.Username {
			t.Errorf("the baseline holds a name")
		}

		// The second sighting is a pass with no call and no write: the baseline's own expiry, which
		// a rewrite would reset, is left as the test shortened it.
		env.redis.SetTTL(memberReqAIBaselineKey(member.Id), time.Hour)
		mreqAIWantNothing(t, env, member, env.mreqEnforce(env.mreqCase(member)))
		if n := ai.count(); n != 0 {
			t.Errorf("provider requests = %d, want none for the second sighting", n)
		}
		if ttl := env.redis.TTL(memberReqAIBaselineKey(member.Id)); ttl > time.Hour {
			t.Errorf("baseline ttl = %v after the second sighting, want it not rewritten", ttl)
		}
	})

	t.Run("a baseline equal to the current names passes with no write at all", func(t *testing.T) {
		env, ai := jobEnv(t)
		member := mreqAIOrdinary(env)
		mreqAIPlant(t, env, memberReqAIBaselineKey(member.Id), mreqAIHashOfUser(t, member))
		env.redis.SetTTL(memberReqAIBaselineKey(member.Id), time.Hour)

		if got := mreqAIAskAs(env, member.Id, mreqAINamesOfUser(member), memberReqLookupMode{}); got != memberReqPass {
			t.Errorf("verdict = %v, want pass", got)
		}
		if n := ai.count(); n != 0 {
			t.Errorf("provider requests = %d, want none", n)
		}
		if ttl := env.redis.TTL(memberReqAIBaselineKey(member.Id)); ttl > time.Hour {
			t.Errorf("baseline ttl = %v, want it not rewritten", ttl)
		}
		if keys := mreqAIVerdictKeys(t); len(keys) != 0 {
			t.Errorf("verdict keys = %v, want none", keys)
		}
	})

	t.Run("with no Redis the result is unknown and nothing is written or asked", func(t *testing.T) {
		env, ai := jobEnv(t)
		member := mreqAIOrdinary(env)
		mreqSwap(t, &memberReqRedisClient, func() *redis.Client { return nil })
		if got := mreqAIAskAs(env, member.Id, mreqAINamesOfUser(member), memberReqLookupMode{}); got != memberReqUnknown {
			t.Errorf("verdict = %v, want unknown", got)
		}
		memberReqRedisClient = raidRedis
		mreqAIWantNoBaseline(t, member.Id)
		if n := ai.count(); n != 0 {
			t.Errorf("provider requests = %d, want none", n)
		}
	})

	t.Run("a Redis read error is unknown and nothing is written or asked", func(t *testing.T) {
		env, ai := jobEnv(t)
		member := mreqAIOrdinary(env)
		env.redis.SetError("connection refused")
		got := mreqAIAskAs(env, member.Id, mreqAINamesOfUser(member), memberReqLookupMode{})
		env.redis.SetError("")
		if got != memberReqUnknown {
			t.Errorf("verdict = %v, want unknown", got)
		}
		mreqAIWantNoBaseline(t, member.Id)
		if n := ai.count(); n != 0 {
			t.Errorf("provider requests = %d, want none", n)
		}
	})

	t.Run("a member who cannot be judged is inert and writes nothing", func(t *testing.T) {
		env, ai := jobEnv(t)
		userID := uniqueLinkOwnerID()
		for _, names := range []*memberReqNames{{}, {First: "\u200b"}, {First: "Deleted Account"}} {
			if got := mreqAIAskAs(env, userID, names, memberReqLookupMode{}); got != memberReqInert {
				t.Errorf("names %+v: verdict = %v, want inert", names, got)
			}
		}
		mreqAIWantNoBaseline(t, userID)
		mreqAIWantMarker(t, env.chat.Id, userID, false)
		if n := ai.count(); n != 0 {
			t.Errorf("provider requests = %d, want none", n)
		}
	})

	t.Run("a member with a marker is never baselined", func(t *testing.T) {
		env, ai := jobEnv(t)
		ai.setDefault(mreqAIStatus(500, "api_error", ""))
		member := mreqAIOrdinary(env)
		mreqAIMarkNew(t, env.chat.Id, member.Id)

		if got := mreqAIAskAs(env, member.Id, mreqAINamesOfUser(member), memberReqLookupMode{}); got != memberReqUnknown {
			t.Errorf("verdict = %v, want unknown (the provider failed)", got)
		}
		if n := ai.count(); n == 0 {
			t.Errorf("provider requests = 0, want the marker to force a judgement")
		}
		mreqAIWantNoBaseline(t, member.Id)
		mreqAIWantMarker(t, env.chat.Id, member.Id, true)
	})
}

func TestMemberReqAIRename(t *testing.T) {
	direct := func(t *testing.T) (*memberReqEnv, *mreqAIState) {
		t.Helper()
		return mreqAIEnvOf(t, map[memberreq.Rule]int{memberreq.RuleAI: models.RequirementActionMute}, false)
	}

	t.Run("a rename is judged once, a breach marks, and renaming back is judged again", func(t *testing.T) {
		env, ai := direct(t)
		member := mreqAIOrdinary(env)
		ordinary := mreqAINamesOfUser(member)
		hash := mreqAIHashOfUser(t, member)
		spam := mreqAIUniqueNames()
		ask := func(names *memberReqNames) memberReqVerdict {
			return mreqAIAskAs(env, member.Id, names, memberReqLookupMode{})
		}

		if got := ask(ordinary); got != memberReqPass {
			t.Fatalf("first sighting = %v, want pass", got)
		}
		mreqAIWantBaseline(t, member.Id, hash)

		ai.setDefault(mreqAIProbability(0.97))
		if got := ask(spam); got != memberReqBreach {
			t.Errorf("renamed to a spam name = %v, want breach", got)
		}
		if got := ask(spam); got != memberReqBreach {
			t.Errorf("the same names again = %v, want breach (cached)", got)
		}
		if n := ai.count(); n != 1 {
			t.Errorf("provider requests = %d, want 1 (a rename is judged once)", n)
		}
		mreqAIWantMarker(t, env.chat.Id, member.Id, true)
		mreqAIWantBaseline(t, member.Id, hash)

		ai.setDefault(mreqAIProbability(0.1))
		if got := ask(ordinary); got != memberReqPass {
			t.Errorf("renamed back = %v, want pass", got)
		}
		if n := ai.count(); n != 2 {
			t.Errorf("provider requests = %d, want 2 (the marker forces a judgement of the old names)", n)
		}
		mreqAIWantMarker(t, env.chat.Id, member.Id, false)
		mreqAIWantBaseline(t, member.Id, hash)
	})

	t.Run("a pass writes the new hash and a rename to a cached pass costs no request", func(t *testing.T) {
		env, ai := direct(t)
		member := mreqAIOrdinary(env)
		if got := mreqAIAskAs(env, member.Id, mreqAINamesOfUser(member), memberReqLookupMode{}); got != memberReqPass {
			t.Fatalf("first sighting = %v, want pass", got)
		}
		renamed := &memberReqNames{First: fmt.Sprintf("Maria%d", member.Id), Username: fmt.Sprintf("maria%d", member.Id)}
		mreqAIPlant(t, env, mreqAIKeyOf(t, renamed), "0.2")

		if got := mreqAIAskAs(env, member.Id, renamed, memberReqLookupMode{}); got != memberReqPass {
			t.Errorf("renamed = %v, want pass", got)
		}
		if n := ai.count(); n != 0 {
			t.Errorf("provider requests = %d, want none for a cached pass", n)
		}
		mreqAIWantBaseline(t, member.Id, mreqAICanon(t, renamed.First, renamed.Last, renamed.Username).Hash)
	})

	t.Run("a member who renames to spam is punished by the rule's action at their next message", func(t *testing.T) {
		env, ai := direct(t)
		member := mreqAIOrdinary(env)
		mreqAIWantNothing(t, env, member, env.mreqEnforce(env.mreqCase(member)))
		mreqAIWantBaseline(t, member.Id, mreqAIHashOfUser(t, member))

		ai.setDefault(mreqAIProbability(0.97))
		renamed := mreqAIRename(env, member, "Casino")
		got := env.mreqEnforce(env.mreqCase(renamed))
		if got.Outcome != memberReqMuted || got.Hold == nil {
			t.Fatalf("outcome = %+v, want Muted", got)
		}
		if n := ai.count(); n != 1 {
			t.Errorf("provider requests = %d, want 1", n)
		}
		mreqAIWantMarker(t, env.chat.Id, member.Id, true)
	})
}

// mreqAIJoinEnv is mreqAIEnvOf for a join with the watcher loaded, so a member's first message can
// be sent through the dispatcher.
func mreqAIJoinEnv(t *testing.T, action int, checkOnJoin bool) (*memberReqEnv, *mreqAIState) {
	t.Helper()
	withAIConfigured(t)
	ai := mreqInstallAIFake(t)
	mreqAICleanup(t, nil, nil)
	env := mreqJoinEnv(t, map[memberreq.Rule]int{memberreq.RuleAI: action}, checkOnJoin)
	env.loadMemberReq()
	return env, ai
}

func TestMemberReqAINewMember(t *testing.T) {
	t.Run("the join sets the marker, judges at once and ignores the baseline", func(t *testing.T) {
		env, ai := mreqAIJoinEnv(t, models.RequirementActionMute, true)
		ai.setDefault(mreqAIProbability(0.97))
		member := mreqAIMember(env, "Casino")
		hash := mreqAIHashOfUser(t, member)
		mreqAIPlant(t, env, memberReqAIBaselineKey(member.Id), hash)

		env.mreqJoin(member)
		if n := ai.count(); n != 1 {
			t.Errorf("provider requests = %d, want 1 (a joiner is judged although the baseline equals their names)", n)
		}
		if hold := env.mreqHold(member.Id); hold == nil || hold.Rules != mreqBitOf(t, "ai") || hold.PunishedAt == nil {
			t.Errorf("hold = %+v, want a punished AI hold", hold)
		}
		mreqAIWantMarker(t, env.chat.Id, member.Id, true)
		if value, _, _ := mreqAIBaselineOf(t, member.Id); value != hash {
			t.Errorf("baseline = %q, want it unchanged by a breach", value)
		}
	})

	t.Run("a pass at join writes the baseline and deletes the marker", func(t *testing.T) {
		env, ai := mreqAIJoinEnv(t, models.RequirementActionMute, true)
		member := mreqAIOrdinary(env)

		env.mreqJoin(member)
		if got := mreqAIWelcomes(env); got != 1 {
			t.Errorf("welcomes = %d, want 1", got)
		}
		if n := ai.count(); n != 1 {
			t.Errorf("provider requests = %d, want 1", n)
		}
		mreqAIWantBaseline(t, member.Id, mreqAIHashOfUser(t, member))
		mreqAIWantMarker(t, env.chat.Id, member.Id, false)
	})

	t.Run("an unanswered join welcomes the joiner, keeps the marker, and their first message is judged", func(t *testing.T) {
		env, ai := mreqAIJoinEnv(t, models.RequirementActionMute, true)
		ai.setDefault(mreqAIStatus(500, "api_error", ""))
		member := mreqAIMember(env, "Casino")

		env.mreqJoin(member)
		if got := mreqAIWelcomes(env); got != 1 {
			t.Errorf("welcomes = %d, want 1 (an unanswered check welcomes)", got)
		}
		mreqAIWantMarker(t, env.chat.Id, member.Id, true)
		mreqAIWantNoBaseline(t, member.Id)
		if hold := env.mreqHold(member.Id); hold != nil {
			t.Fatalf("hold = %+v, want none after an unanswered join", hold)
		}

		ai.setDefault(mreqAIProbability(0.97))
		before := ai.count()
		env.mreqSend(member, "hello")
		memberReqJobs.wait()
		if n := ai.count() - before; n != 1 {
			t.Errorf("provider requests at the first message = %d, want 1 (the marker forces a judgement, never a baseline)", n)
		}
		if hold := env.mreqHold(member.Id); hold == nil || hold.Rules != mreqBitOf(t, "ai") {
			t.Errorf("hold = %+v, want the AI hold from the first message", hold)
		}
		mreqAIWantNoBaseline(t, member.Id)
	})

	t.Run("a joiner who rejoins is judged again", func(t *testing.T) {
		env, ai := mreqAIJoinEnv(t, models.RequirementActionMute, true)
		member := mreqAIOrdinary(env)

		env.mreqJoin(member)
		if n := ai.count(); n != 1 {
			t.Fatalf("provider requests after the first join = %d, want 1", n)
		}
		mreqAIWantBaseline(t, member.Id, mreqAIHashOfUser(t, member))

		// A new membership: the leave clears the join dedupe claim, and the cached verdict of the
		// old names is gone, so only the rule's own order can decide whether the baseline counts.
		memberReqDeleteJoinClaims([]int64{env.chat.Id})
		mreqAIDeleteVerdicts()
		ai.setDefault(mreqAIProbability(0.97))
		env.mreqJoin(member)
		if n := ai.count(); n != 2 {
			t.Errorf("provider requests after the rejoin = %d, want 2 (the old baseline does not stand in for a new membership)", n)
		}
		if hold := env.mreqHold(member.Id); hold == nil || hold.Rules != mreqBitOf(t, "ai") {
			t.Errorf("hold = %+v, want the AI hold of the rejoin", hold)
		}
	})

	t.Run("with the rule Off or the AI not configured no marker is written and nobody is asked", func(t *testing.T) {
		for _, tc := range []struct {
			name      string
			action    int
			configure bool
		}{
			{"the rule Off", models.RequirementActionOff, true},
			{"the AI not configured", models.RequirementActionMute, false},
		} {
			env, ai := mreqAIJoinEnv(t, tc.action, true)
			if !tc.configure {
				mreqSwap(t, &config.AppConfig.AnthropicAPIKey, "")
			}
			// A rule that is Off needs another rule on, or the entry returns before it reads anything.
			env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionWarn})
			member := mreqAIOrdinary(env)
			env.mreqJoin(member)
			mreqAIWantMarker(t, env.chat.Id, member.Id, false)
			mreqAIWantNoBaseline(t, member.Id)
			if n := ai.count(); n != 0 {
				t.Errorf("%s: provider requests = %d, want none", tc.name, n)
			}
		}
	})

	t.Run("with no Redis the join writes nothing and welcomes", func(t *testing.T) {
		env, ai := mreqAIJoinEnv(t, models.RequirementActionMute, true)
		member := mreqAIMember(env, "Casino")
		mreqSwap(t, &memberReqRedisClient, func() *redis.Client { return nil })
		env.mreqJoin(member)
		memberReqRedisClient = raidRedis
		if got := mreqAIWelcomes(env); got != 1 {
			t.Errorf("welcomes = %d, want 1", got)
		}
		mreqAIWantMarker(t, env.chat.Id, member.Id, false)
		mreqAIWantNoBaseline(t, member.Id)
		if n := ai.count(); n != 0 {
			t.Errorf("provider requests = %d, want none", n)
		}
	})
}

func TestMemberReqAICheckOnJoinOff(t *testing.T) {
	env, ai := mreqAIJoinEnv(t, models.RequirementActionMute, false)
	ai.setDefault(mreqAIProbability(0.97))
	joiner := mreqAIMember(env, "Casino")

	env.mreqJoin(joiner)
	mreqAIWantMarker(t, env.chat.Id, joiner.Id, true)
	if n := ai.count(); n != 0 {
		t.Errorf("provider requests at join = %d, want none (Check on join is off)", n)
	}
	if got := mreqAIWelcomes(env); got != 1 {
		t.Errorf("welcomes = %d, want 1", got)
	}

	env.mreqSend(joiner, "hello")
	memberReqJobs.wait()
	if n := ai.count(); n != 1 {
		t.Errorf("provider requests after the first message = %d, want 1 (the marker is the join record)", n)
	}
	if hold := env.mreqHold(joiner.Id); hold == nil || hold.Rules != mreqBitOf(t, "ai") || hold.Action != models.RequirementActionMute {
		t.Errorf("hold = %+v, want the AI mute hold", hold)
	}

	// An established member of the same group has no join record: baselined, no request.
	established := mreqAIOrdinary(env)
	env.mreqSend(established, "hello")
	memberReqJobs.wait()
	if n := ai.count(); n != 1 {
		t.Errorf("provider requests = %d, want still 1 (an established member costs no request)", n)
	}
	mreqAIWantBaseline(t, established.Id, mreqAIHashOfUser(t, established))
	if hold := env.mreqHold(established.Id); hold != nil {
		t.Errorf("hold = %+v, want none for an established member", hold)
	}
}

func TestMemberReqAIMarkerSurvivesLongHold(t *testing.T) {
	env, ai := mreqAIEnvOf(t, map[memberreq.Rule]int{memberreq.RuleAI: models.RequirementActionWarn}, false)
	ai.setDefault(mreqAIProbability(0.97))
	member := mreqAIMember(env, "Casino")
	mreqAIMarkNew(t, env.chat.Id, member.Id)

	if got := env.mreqEnforce(env.mreqCase(member)); got.Outcome != memberReqWarned {
		t.Fatalf("first outcome = %+v, want Warned", got)
	}
	mreqAIWantMarker(t, env.chat.Id, member.Id, true)

	// The baseline and the verdict expire during the hold. The marker is what is left.
	mreqAIDeleteVerdicts()
	env.redis.Del(memberReqAIBaselineKey(member.Id))
	requests := ai.count()

	later := env.mreqCase(member)
	later.MessageDate = time.Now().Add(5 * time.Second).Unix()
	if got := env.mreqEnforce(later); got.Outcome != memberReqHeld {
		t.Fatalf("second outcome = %+v, want Held (the hold is kept)", got)
	}
	if n := ai.count() - requests; n != 1 {
		t.Errorf("provider requests = %d, want 1 (the marker forces a judgement, a baseline-only pass would launder the member)", n)
	}
	mreqAIWantNoBaseline(t, member.Id)
	mreqAIWantMarker(t, env.chat.Id, member.Id, true)
	if hold := env.mreqHold(member.Id); hold == nil {
		t.Fatalf("the hold was released although the names were judged spam again")
	}

	// A pass clears the marker and the hold.
	mreqAIDeleteVerdicts()
	ai.setDefault(mreqAIProbability(0.1))
	later.MessageDate = time.Now().Add(6 * time.Second).Unix()
	if got := env.mreqEnforce(later); got.Outcome != memberReqNone {
		t.Errorf("third outcome = %+v, want None once the names pass", got)
	}
	if hold := env.mreqHold(member.Id); hold != nil {
		t.Errorf("hold = %+v, want it released by the pass", hold)
	}
	mreqAIWantMarker(t, env.chat.Id, member.Id, false)
	mreqAIWantBaseline(t, member.Id, mreqAIHashOfUser(t, member))
}
