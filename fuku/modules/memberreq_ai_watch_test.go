//go:build testtools

package modules

import (
	"context"
	"errors"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/redis/go-redis/v9"

	"github.com/uasneppy/fuku_robot/fuku/config"
	"github.com/uasneppy/fuku_robot/fuku/db/approvals"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/captcha"
	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// The watcher's AI read and the held release with names (plan 15-05; D-02, D-21). The watcher is
// a pure reader: three single-key GETs in its one pipelined round, no write of a verdict, a
// baseline or a marker, and no call to Telegram or the provider.

// mreqAIWatchEnv builds an environment whose watcher is loaded on the real dispatcher, with the
// AI rule stored at action and the stand-in installed.
func mreqAIWatchEnv(t *testing.T, action int) (*memberReqEnv, *mreqAIState) {
	t.Helper()
	env, ai := mreqAIEnvOf(t, map[memberreq.Rule]int{memberreq.RuleAI: action}, false)
	env.loadMemberReq()
	return env, ai
}

// mreqAIKeyspace is every fuku:memberreq:* key with its value, except the pass keys: the pass key
// is the existing Phase 13 guard, written by the clean path that the watcher always had.
func mreqAIKeyspace(t *testing.T, env *memberReqEnv) map[string]string {
	t.Helper()
	space := map[string]string{}
	for _, key := range env.redis.Keys() {
		if !strings.HasPrefix(key, "fuku:memberreq:") || strings.HasPrefix(key, "fuku:memberreq:pass:") {
			continue
		}
		value, err := env.redis.Get(key)
		if err != nil {
			value = "<" + err.Error() + ">"
		}
		space[key] = value
	}
	return space
}

// mreqAISameSpace fails the test when the Redis keyspace of the requirements changed.
func mreqAISameSpace(t *testing.T, before, after map[string]string) {
	t.Helper()
	for key, value := range after {
		if was, ok := before[key]; !ok || was != value {
			t.Errorf("the watcher wrote %s", key)
		}
	}
	for key := range before {
		if _, ok := after[key]; !ok {
			t.Errorf("the watcher removed %s", key)
		}
	}
}

func TestMemberReqWatcherAI(t *testing.T) {
	// watch sends one message of member through the real dispatcher with the runner refusing every
	// job, and reports how many jobs the watcher tried to start, whether the keyspace stayed as it
	// was and whether the watcher or the provider made a request about the member.
	watch := func(t *testing.T, env *memberReqEnv, ai *mreqAIState, member gotgbot.User) (starts int) {
		t.Helper()
		count := mreqCountJobStarts(t)
		before := mreqAIKeyspace(t, env)
		requests := ai.count()
		env.mreqSend(member, "hello")
		memberReqJobs.wait()
		mreqAISameSpace(t, before, mreqAIKeyspace(t, env))
		if n := ai.count() - requests; n != 0 {
			t.Errorf("provider requests = %d, want none from the watcher", n)
		}
		if n := mreqLookupRequests(env, member); n != 0 {
			t.Errorf("Telegram requests about the member = %d, want none from the watcher", n)
		}
		return count()
	}

	satisfied := []struct {
		name  string
		setup func(t *testing.T, env *memberReqEnv, member gotgbot.User)
	}{
		{"a cached verdict below the bar", func(t *testing.T, env *memberReqEnv, member gotgbot.User) {
			mreqAIPlant(t, env, mreqAIKeyOf(t, mreqAINamesOfUser(member)), "0.2")
		}},
		{"a baseline equal to the names and no marker", func(t *testing.T, env *memberReqEnv, member gotgbot.User) {
			mreqAIPlant(t, env, memberReqAIBaselineKey(member.Id), mreqAIHashOfUser(t, member))
		}},
		{"the rule (SEC-01): a baseline equal to the names, no marker and another account's cached breach", func(t *testing.T, env *memberReqEnv, member gotgbot.User) {
			mreqAIPlant(t, env, memberReqAIBaselineKey(member.Id), mreqAIHashOfUser(t, member))
			mreqAIPlant(t, env, mreqAIKeyOf(t, mreqAINamesOfUser(member)), "0.95")
		}},
	}
	for _, tc := range satisfied {
		t.Run("satisfied: "+tc.name, func(t *testing.T) {
			env, ai := mreqAIWatchEnv(t, models.RequirementActionMute)
			member := mreqAIOrdinary(env)
			tc.setup(t, env, member)
			if n := watch(t, env, ai, member); n != 0 {
				t.Errorf("job starts = %d, want none", n)
			}
		})
	}

	jobs := []struct {
		name  string
		setup func(t *testing.T, env *memberReqEnv, member gotgbot.User)
	}{
		{"no verdict, no baseline and no marker", func(*testing.T, *memberReqEnv, gotgbot.User) {}},
		{"a marker, even with a baseline equal to the names", func(t *testing.T, env *memberReqEnv, member gotgbot.User) {
			mreqAIPlant(t, env, memberReqAIBaselineKey(member.Id), mreqAIHashOfUser(t, member))
			mreqAIMarkNew(t, env.chat.Id, member.Id)
		}},
		{"a baseline that differs (a rename)", func(t *testing.T, env *memberReqEnv, member gotgbot.User) {
			mreqAIPlant(t, env, memberReqAIBaselineKey(member.Id), "0123456789abcdef0123456789abcdef")
		}},
		{"a cached breach and no baseline", func(t *testing.T, env *memberReqEnv, member gotgbot.User) {
			mreqAIPlant(t, env, mreqAIKeyOf(t, mreqAINamesOfUser(member)), "0.95")
		}},
		{"a cached breach and a baseline that differs", func(t *testing.T, env *memberReqEnv, member gotgbot.User) {
			mreqAIPlant(t, env, mreqAIKeyOf(t, mreqAINamesOfUser(member)), "0.95")
			mreqAIPlant(t, env, memberReqAIBaselineKey(member.Id), "0123456789abcdef0123456789abcdef")
		}},
		{"a verdict value that is not a probability", func(t *testing.T, env *memberReqEnv, member gotgbot.User) {
			mreqAIPlant(t, env, mreqAIKeyOf(t, mreqAINamesOfUser(member)), "NaN")
		}},
	}
	for _, tc := range jobs {
		t.Run("one job: "+tc.name, func(t *testing.T) {
			env, ai := mreqAIWatchEnv(t, models.RequirementActionMute)
			member := mreqAIOrdinary(env)
			tc.setup(t, env, member)
			if n := watch(t, env, ai, member); n != 1 {
				t.Errorf("job starts = %d, want exactly 1", n)
			}
		})
	}

	t.Run("a member the cache knows as an administrator or who is approved starts no job", func(t *testing.T) {
		env, ai := mreqAIWatchEnv(t, models.RequirementActionMute)
		admin := mreqAIOrdinary(env)
		env.mreqPromote(admin, gotgbot.ChatMemberStatusAdministrator)
		env.mreqAdminList()
		if n := watch(t, env, ai, admin); n != 0 {
			t.Errorf("an administrator: job starts = %d, want none", n)
		}

		approved := mreqAIOrdinary(env)
		if err := approvals.AddApprovedUser(env.chat.Id, approved.Id, env.admin.Id, "trusted"); err != nil {
			t.Fatal(err)
		}
		if n := watch(t, env, ai, approved); n != 0 {
			t.Errorf("an approved member: job starts = %d, want none", n)
		}
	})

	t.Run("an approvals read that fails starts no job", func(t *testing.T) {
		env, ai := mreqAIWatchEnv(t, models.RequirementActionMute)
		member := mreqAIOrdinary(env)
		mreqSwap(t, &memberReqApprovedLookup, func(context.Context, int64, int64) (bool, error) {
			return false, errors.New("approvals are down")
		})
		if n := watch(t, env, ai, member); n != 0 {
			t.Errorf("job starts = %d, want none", n)
		}
	})

	t.Run("names that cannot be judged are satisfied", func(t *testing.T) {
		env, ai := mreqAIWatchEnv(t, models.RequirementActionMute)
		member := env.mreqMember("", "", "")
		if n := watch(t, env, ai, member); n != 0 {
			t.Errorf("job starts = %d, want none for names that are not judgeable", n)
		}
	})

	t.Run("no Redis starts a job", func(t *testing.T) {
		env, ai := mreqAIWatchEnv(t, models.RequirementActionMute)
		member := mreqAIOrdinary(env)
		mreqAIPlant(t, env, memberReqAIBaselineKey(member.Id), mreqAIHashOfUser(t, member))
		mreqSwap(t, &memberReqRedisClient, func() *redis.Client { return nil })
		if n := watch(t, env, ai, member); n != 1 {
			t.Errorf("no Redis: job starts = %d, want 1", n)
		}
	})

	t.Run("an unconfigured AI is read as Off: no job and no AI key", func(t *testing.T) {
		env, ai := mreqAIWatchEnv(t, models.RequirementActionMute)
		mreqSwap(t, &config.AppConfig.AnthropicAPIKey, "")
		member := mreqAIOrdinary(env)
		if n := watch(t, env, ai, member); n != 0 {
			t.Errorf("job starts = %d, want none for an unconfigured stored rule", n)
		}
		before := env.redis.CommandCount()
		settings, err := memberreq.GetSettingsFresh(context.Background(), env.chat.Id)
		if err != nil {
			t.Fatal(err)
		}
		if memberReqLookupRulesOf(settings).any() {
			t.Errorf("lookup rules = %+v, want none while the AI is not configured", memberReqLookupRulesOf(settings))
		}
		if env.redis.CommandCount() != before {
			t.Errorf("reading the rules made Redis commands")
		}
	})

	t.Run("a rule that is Off is not read", func(t *testing.T) {
		env, ai := mreqAIWatchEnv(t, models.RequirementActionOff)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionWarn})
		member := mreqAIOrdinary(env)
		if n := watch(t, env, ai, member); n != 0 {
			t.Errorf("job starts = %d, want none (the Phase 13 clean path)", n)
		}
	})

	t.Run("the AI part adds exactly three GETs and a group with name rules only keeps its single GET", func(t *testing.T) {
		env, _ := mreqAIWatchEnv(t, models.RequirementActionMute)
		member := mreqAIOrdinary(env)
		settings, err := memberreq.GetSettingsFresh(context.Background(), env.chat.Id)
		if err != nil {
			t.Fatal(err)
		}
		names := memberReqNamesOf(member)
		before := env.redis.CommandCount()
		memberReqLookupCached(settings, env.chat.Id, member.Id, "value", names)
		if got := env.redis.CommandCount() - before; got != 4 {
			t.Errorf("Redis commands of the AI-only read = %d, want 4 (the pass key and three AI keys)", got)
		}

		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleAI: models.RequirementActionOff, memberreq.RuleLastName: models.RequirementActionWarn})
		nameOnly, err := memberreq.GetSettingsFresh(context.Background(), env.chat.Id)
		if err != nil {
			t.Fatal(err)
		}
		if memberReqLookupRulesOf(nameOnly).any() {
			t.Fatalf("lookup rules = %+v, want none for name rules only", memberReqLookupRulesOf(nameOnly))
		}
		before = env.redis.CommandCount()
		memberReqCleanMember(context.Background(), env.chat.Id, member.Id, time.Now().Unix(), nameOnly, names)
		if got := env.redis.CommandCount() - before; got < 1 {
			t.Errorf("Redis commands of the name-only path = %d, want its pass-key read", got)
		}
		if _, err := env.redis.Get(memberReqAIBaselineKey(member.Id)); err == nil {
			t.Errorf("the name-only path touched the AI keys")
		}
	})

	t.Run("the watcher returns ext.ContinueGroups on every path and the update reaches the neighbours", func(t *testing.T) {
		env, _ := mreqAIWatchEnv(t, models.RequirementActionMute)
		seen := env.mreqWatchGroups()
		run := func(t *testing.T, member gotgbot.User) {
			t.Helper()
			msg := &gotgbot.Message{Chat: env.chat, From: &member, Text: "hello", Date: time.Now().Unix()}
			if err := memberReqModule.memberReqWatcher(env.bot, env.mreqWatcherCtx(msg)); !errors.Is(err, ext.ContinueGroups) {
				t.Errorf("watcher returned %v, want ext.ContinueGroups", err)
			}
			before, beforeLate := seen.count("g-1"), seen.count("g3")
			env.mreqProcess(msg)
			memberReqJobs.wait()
			if seen.count("g-1") != before+1 || seen.count("g3") != beforeLate+1 {
				t.Error("the update did not reach both group -1 and group 3")
			}
		}
		t.Run("a job handed off", func(t *testing.T) { run(t, mreqAIOrdinary(env)) })
		t.Run("a job shed by a full runner", func(t *testing.T) {
			mreqSwap(t, &memberReqMaxInFlight, 0)
			run(t, mreqAIOrdinary(env))
		})
		t.Run("a satisfied member", func(t *testing.T) {
			member := mreqAIOrdinary(env)
			mreqAIPlant(t, env, memberReqAIBaselineKey(member.Id), mreqAIHashOfUser(t, member))
			run(t, member)
		})
		t.Run("a Redis failure", func(t *testing.T) {
			mreqSwap(t, &memberReqRedisClient, func() *redis.Client { return nil })
			run(t, mreqAIOrdinary(env))
		})
		t.Run("a recovered panic", func(t *testing.T) {
			mreqSwap(t, &memberReqApprovedLookup, func(context.Context, int64, int64) (bool, error) {
				panic("injected into the approvals seam")
			})
			run(t, mreqAIOrdinary(env))
		})
	})

	t.Run("the decision is a pure function of the verdict, the baseline, the marker and the hash", func(t *testing.T) {
		const hash = "0123456789abcdef0123456789abcdef"
		const other = "fedcba9876543210fedcba9876543210"
		for _, tc := range []struct {
			name     string
			verdict  string
			baseline string
			marker   bool
			want     bool
		}{
			{"below the bar", "0.2", "", true, true},
			{"just below the bar", "0.8999", other, true, true},
			{"zero", "0", "", false, true},
			{"at the bar, nothing else", "0.9", "", false, false},
			{"above the bar, nothing else", "0.97", "", false, false},
			{"above the bar with the equal baseline and no marker (the rule, SEC-01)", "0.97", hash, false, true},
			{"above the bar with the equal baseline and a marker", "0.97", hash, true, false},
			{"NaN", "NaN", "", false, false},
			{"infinity", "+Inf", "", false, false},
			{"negative infinity", "-Inf", "", false, false},
			{"out of range", "1.5", "", false, false},
			{"negative", "-0.1", "", false, false},
			{"text", "abc", "", false, false},
			{"no verdict, equal baseline, no marker", "", hash, false, true},
			{"no verdict, equal baseline, a marker", "", hash, true, false},
			{"no verdict, different baseline, no marker", "", other, false, false},
			{"no verdict, different baseline, a marker", "", other, true, false},
			{"no verdict, no baseline, no marker", "", "", false, false},
			{"no verdict, no baseline, a marker", "", "", true, false},
			{"NaN with the equal baseline and no marker", "NaN", hash, false, true},
		} {
			if got := memberReqAIWatchSatisfied(tc.verdict, tc.baseline, tc.marker, hash); got != tc.want {
				t.Errorf("%s: satisfied = %t, want %t", tc.name, got, tc.want)
			}
		}
		if memberReqAIWatchSatisfied(strconv.FormatFloat(math.NaN(), 'f', -1, 64), "", false, hash) {
			t.Errorf("a NaN verdict satisfied a member")
		}
	})
}

func TestMemberReqWatcherAIBurst(t *testing.T) {
	env, ai := mreqAIWatchEnv(t, models.RequirementActionMute)
	// The runner's cap (32) and its shedding are proved by TestMemberReqRunnerSheds. Here every
	// poster must get a job, so the cap is lifted above the group's size.
	mreqSwap(t, &memberReqMaxInFlight, 64)
	members := make([]gotgbot.User, 40)
	for i := range members {
		members[i] = mreqAIOrdinary(env)
	}
	for _, member := range members {
		env.mreqSend(member, "hello")
	}
	memberReqJobs.wait()

	if n := ai.count(); n != 0 {
		t.Errorf("provider requests = %d, want none: switching the rule on costs no provider call", n)
	}
	for _, member := range members {
		mreqAIWantBaseline(t, member.Id, mreqAIHashOfUser(t, member))
		if hold := env.mreqHold(member.Id); hold != nil {
			t.Errorf("hold = %+v, want none for an established member", hold)
		}
	}
	if n := len(env.mreqWrites()); n != 0 {
		t.Errorf("ban, restrict and unban requests = %d, want none", n)
	}

	starts := mreqCountJobStarts(t)
	for _, member := range members {
		env.mreqSend(member, "hello again")
	}
	memberReqJobs.wait()
	if n := starts(); n != 0 {
		t.Errorf("job starts for the second round = %d, want none (every member has a baseline)", n)
	}
	if n := ai.count(); n != 0 {
		t.Errorf("provider requests = %d, want none", n)
	}
}

func TestMemberReqPassCacheWithAI(t *testing.T) {
	env, ai := mreqAIWatchEnv(t, models.RequirementActionMute)
	member := mreqAIOrdinary(env)
	settings, err := memberreq.GetSettingsFresh(context.Background(), env.chat.Id)
	if err != nil {
		t.Fatal(err)
	}
	mreqStorePass(t, env, member, settings)

	// The pass key is stored, but the member has neither a verdict nor a baseline: it never stands
	// in for the AI positive, so a job runs and records the baseline.
	env.mreqSend(member, "the pass key is stored")
	memberReqJobs.wait()
	mreqAIWantBaseline(t, member.Id, mreqAIHashOfUser(t, member))
	if n := ai.count(); n != 0 {
		t.Errorf("provider requests = %d, want none for the baseline job", n)
	}

	// With the baseline recorded the member is satisfied, and the pass key is read as before: an old
	// hold that only the clean predicate could delete survives, so no predicate ran.
	env.mreqOldHold(member.Id)
	starts := mreqCountJobStarts(t)
	env.mreqSend(member, "second message")
	memberReqJobs.wait()
	if n := starts(); n != 0 {
		t.Errorf("job starts = %d, want none once the baseline is recorded", n)
	}
	if env.mreqHold(member.Id) == nil {
		t.Error("the old hold was deleted: the watcher ran the clean predicate although the pass key hit")
	}
}

func TestMemberReqHeldReleaseAI(t *testing.T) {
	heldEnv := func(t *testing.T) (*memberReqEnv, *mreqAIState) {
		t.Helper()
		return mreqAIWatchEnv(t, models.RequirementActionWarn)
	}
	// seed gives the member a punished Warn hold carrying bit 128 and the marker a breach leaves.
	seed := func(t *testing.T, env *memberReqEnv, member gotgbot.User) {
		t.Helper()
		mreqSeedHeld(t, env, member, mreqBitOf(t, "ai"), models.RequirementActionWarn, 10*time.Minute)
		mreqAIMarkNew(t, env.chat.Id, member.Id)
	}

	t.Run("a member who renamed to an ordinary name is released with one request", func(t *testing.T) {
		env, ai := heldEnv(t)
		member := mreqAIMember(env, "Casino")
		seed(t, env, member)
		renamed := mreqAIOrdinary(env)
		env.fake.setProfile(member.Id, renamed.FirstName, renamed.LastName, renamed.Username)
		member.FirstName, member.LastName, member.Username = renamed.FirstName, renamed.LastName, renamed.Username

		env.mreqSend(member, "I changed my name")
		memberReqJobs.wait()

		if n := ai.count(); n != 1 {
			t.Errorf("provider requests = %d, want 1", n)
		}
		if hold := env.mreqHold(member.Id); hold != nil {
			t.Errorf("hold = %+v, want it released", hold)
		}
		mreqAIWantBaseline(t, member.Id, mreqAIHashOfUser(t, member))
		mreqAIWantMarker(t, env.chat.Id, member.Id, false)
		mreqHeldNoPunishment(t, env, member)
		if deleted := env.mreqDeleted(); len(deleted) != 0 {
			t.Errorf("deleted messages = %v, want none", deleted)
		}
	})

	t.Run("a rename to a cached pass is released with no request", func(t *testing.T) {
		env, ai := heldEnv(t)
		member := mreqAIMember(env, "Casino")
		seed(t, env, member)
		renamed := mreqAIOrdinary(env)
		mreqAIPlant(t, env, mreqAIKeyOf(t, mreqAINamesOfUser(renamed)), "0.1")
		renamed.Id = member.Id

		got := env.mreqEnforce(env.mreqCase(renamed))
		if got.Outcome != memberReqNone || got.Hold != nil {
			t.Errorf("outcome = %+v, want the empty result of a release", got)
		}
		if n := ai.count(); n != 0 {
			t.Errorf("provider requests = %d, want none", n)
		}
		if hold := env.mreqHold(member.Id); hold != nil {
			t.Errorf("hold = %+v, want it released", hold)
		}
	})

	t.Run("the same names with the breach cached keep the hold with no request", func(t *testing.T) {
		env, ai := heldEnv(t)
		member := mreqAIMember(env, "Casino")
		seed(t, env, member)
		mreqAIPlant(t, env, mreqAIKeyOf(t, mreqAINamesOfUser(member)), "0.95")

		if got := env.mreqEnforce(env.mreqCase(member)); got.Outcome != memberReqHeld {
			t.Errorf("outcome = %+v, want Held", got)
		}
		if n := ai.count(); n != 0 {
			t.Errorf("provider requests = %d, want none for a cached breach", n)
		}
		if env.mreqHold(member.Id) == nil {
			t.Error("the hold was released although the names are still judged spam")
		}
		mreqAIWantMarker(t, env.chat.Id, member.Id, true)
	})

	t.Run("with the verdict and the baseline expired the marker still forces a request", func(t *testing.T) {
		env, ai := heldEnv(t)
		ai.setDefault(mreqAIProbability(0.97))
		member := mreqAIMember(env, "Casino")
		seed(t, env, member)

		if got := env.mreqEnforce(env.mreqCase(member)); got.Outcome != memberReqHeld {
			t.Errorf("outcome = %+v, want Held", got)
		}
		if n := ai.count(); n != 1 {
			t.Errorf("provider requests = %d, want 1 (the marker forces a judgement)", n)
		}
		mreqAIWantNoBaseline(t, member.Id)
		if env.mreqHold(member.Id) == nil {
			t.Error("the hold was released although the names were judged spam")
		}
	})

	t.Run("a rename to another spam-like name keeps the hold", func(t *testing.T) {
		env, ai := heldEnv(t)
		ai.setDefault(mreqAIProbability(0.97))
		member := mreqAIMember(env, "Casino")
		seed(t, env, member)
		other := *mreqAIUniqueNames()
		member.FirstName, member.LastName, member.Username = other.First, other.Last, other.Username

		if got := env.mreqEnforce(env.mreqCase(member)); got.Outcome != memberReqHeld {
			t.Errorf("outcome = %+v, want Held", got)
		}
		if n := ai.count(); n != 1 {
			t.Errorf("provider requests = %d, want 1", n)
		}
		if env.mreqHold(member.Id) == nil {
			t.Error("the hold was released by a rename to another spam name")
		}
	})

	t.Run("a hold that carries only a name bit or the photo bit makes no AI request", func(t *testing.T) {
		env, ai := heldEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionWarn})
		for _, bit := range []int{mreqBitOf(t, "ln"), mreqBitOf(t, "ph")} {
			member := mreqAIOrdinary(env)
			mreqSeedHeld(t, env, member, bit, models.RequirementActionWarn, 10*time.Minute)
			mreqAIMarkNew(t, env.chat.Id, member.Id)

			env.mreqEnforce(env.mreqCase(member))
			if n := ai.count(); n != 0 {
				t.Errorf("hold bit %d: provider requests = %d, want none", bit, n)
			}
		}
	})

	t.Run("an approved member and a member with a pending captcha make no request", func(t *testing.T) {
		env, ai := heldEnv(t)
		ai.setDefault(mreqAIProbability(0.99))
		approved := mreqAIMember(env, "Casino")
		seed(t, env, approved)
		if err := approvals.AddApprovedUser(env.chat.Id, approved.Id, env.admin.Id, "trusted"); err != nil {
			t.Fatal(err)
		}
		if got := env.mreqEnforce(env.mreqCase(approved)); got.Outcome != memberReqNone {
			t.Errorf("approved: outcome = %+v, want None", got)
		}

		pending := mreqAIMember(env, "Casino")
		seed(t, env, pending)
		if _, err := captcha.CreateCaptchaAttemptPreMessage(pending.Id, env.chat.Id, "answer", 5); err != nil {
			t.Fatal(err)
		}
		dbcache.DeleteCache(dbcache.CacheKey("captcha_pending", env.chat.Id))
		if got := env.mreqEnforce(env.mreqCase(pending)); got.Outcome != memberReqNone {
			t.Errorf("pending captcha: outcome = %+v, want None", got)
		}
		if n := ai.count(); n != 0 {
			t.Errorf("provider requests = %d, want none for an exempt member", n)
		}
	})

	t.Run("an administrator the cached list knows costs no request, and an unknown one never costs a punishment", func(t *testing.T) {
		env, ai := heldEnv(t)
		ai.setDefault(mreqAIProbability(0.99))
		known := mreqAIMember(env, "Casino")
		seed(t, env, known)
		env.mreqPromote(known, gotgbot.ChatMemberStatusAdministrator)
		env.mreqAdminList()
		env.mreqSend(known, "hello")
		memberReqJobs.wait()
		if n := ai.count(); n != 0 {
			t.Errorf("a known administrator: provider requests = %d, want none (the watcher starts no job)", n)
		}

		unknown := mreqAIMember(env, "Casino")
		seed(t, env, unknown)
		env.fake.setMember(env.chat.Id, unknown.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusAdministrator, CanRestrictMembers: true})
		got := env.mreqEnforce(env.mreqCase(unknown))
		if got.Outcome == memberReqWarned || got.Outcome == memberReqMuted || got.Outcome == memberReqRemoved {
			t.Errorf("an administrator whose list entry is unknown: outcome = %+v, want no punishment", got)
		}
		if n := ai.count(); n > 1 {
			t.Errorf("provider requests = %d, want at most 1 for an unknown administrator", n)
		}
		mreqHeldNoPunishment(t, env, unknown)
	})
}
