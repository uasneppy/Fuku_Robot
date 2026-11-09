//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/utils/claude"
)

// The tests of the AI name check's cost guards: the three hourly budgets, the four-call gate, the
// one flight per string, the breaker, the sticky unknown, the stale-names guard and the verdict
// cache. They call the rule directly (as a joiner, so the baseline never stands in for a judgement)
// or run the executor, always against the Anthropic stand-in: no real provider, no network.

// mreqAIGuardEnv is the environment of a guard test: the AI check configured and the rule stored at
// Ban for the group.
func mreqAIGuardEnv(t *testing.T) (*memberReqEnv, *mreqAIState) {
	t.Helper()
	return mreqAIEnvOf(t, map[memberreq.Rule]int{memberreq.RuleAI: models.RequirementActionBan}, false)
}

// mreqAIUsers mints n user IDs that never held a row, and removes their baselines and the chat's
// markers when the test ends.
func mreqAIUsers(t *testing.T, env *memberReqEnv, n int) []int64 {
	t.Helper()
	ids := make([]int64, n)
	for i := range ids {
		ids[i] = uniqueLinkOwnerID()
	}
	mreqAICleanup(t, []int64{env.chat.Id}, ids)
	return ids
}

// mreqAIAskIn calls the AI rule directly as a joiner, in the given group, for the given user.
func mreqAIAskIn(env *memberReqEnv, chatID, userID int64, names *memberReqNames) memberReqVerdict {
	return memberReqAIRule(context.Background(), env.bot, chatID, userID, memberReqLookupMode{Names: names, Joining: true})
}

// mreqAICounter reads an hourly budget counter; a key that does not exist is zero.
func mreqAICounter(t *testing.T, key string) int64 {
	t.Helper()
	client := memberReqRedisClient()
	if client == nil {
		t.Fatal("no Redis client")
	}
	value, err := client.Get(context.Background(), key).Int64()
	if errors.Is(err, redis.Nil) {
		return 0
	}
	if err != nil {
		t.Fatalf("read %s: %v", key, err)
	}
	return value
}

// The three budget keys as the rulebook names them, built by hand so a drift of the production
// builders is caught.
func mreqAIUserKey(userID int64, stamp string) string {
	return fmt.Sprintf("fuku:memberreq:aib:u:%d:%s", userID, stamp)
}

func mreqAIChatKey(chatID int64, stamp string) string {
	return fmt.Sprintf("fuku:memberreq:aib:c:%d:%s", chatID, stamp)
}

func mreqAIGlobalKey(stamp string) string {
	return "fuku:memberreq:aib:g:" + stamp
}

// mreqAIStamp is the UTC hour stamp of the budget keys now.
func mreqAIStamp() string {
	return raidAIHourStamp(raidNow())
}

// mreqAIWantVerdict fails the test unless the verdict is the wanted one.
func mreqAIWantVerdict(t *testing.T, what string, got, want memberReqVerdict) {
	t.Helper()
	if got != want {
		t.Errorf("%s: verdict = %v, want %v", what, got, want)
	}
}

// mreqAIClock is a test clock for raidNow that a test can move forward.
type mreqAIClock struct {
	ns atomic.Int64
}

func (c *mreqAIClock) now() time.Time { return time.Unix(0, c.ns.Load()).UTC() }

func (c *mreqAIClock) advance(d time.Duration) { c.ns.Add(int64(d)) }

// mreqAIInstallClock replaces raidNow with a clock fixed at the start of the current UTC hour plus
// ten minutes, so a test that moves it by less than fifty minutes stays in one hour.
func mreqAIInstallClock(t *testing.T) *mreqAIClock {
	t.Helper()
	clock := &mreqAIClock{}
	clock.ns.Store(time.Now().UTC().Truncate(time.Hour).Add(10 * time.Minute).UnixNano())
	mreqSwap(t, &raidNow, clock.now)
	return clock
}

func TestMemberReqAIBudget(t *testing.T) {
	t.Run("the sixth string of one account in an hour makes no request, in any group", func(t *testing.T) {
		withAIBudgets(t, 5, 100, 400)
		env, ai := mreqAIGuardEnv(t)
		user := mreqAIUsers(t, env, 1)[0]
		before := memberReqAIStatsSnapshot()
		for i := 0; i < 5; i++ {
			mreqAIWantVerdict(t, fmt.Sprintf("string %d", i+1), mreqAIAskIn(env, env.chat.Id, user, mreqAIUniqueNames()), memberReqPass)
		}
		if got := ai.count(); got != 5 {
			t.Fatalf("requests after five strings = %d, want 5", got)
		}
		mreqAIWantVerdict(t, "the sixth string", mreqAIAskIn(env, env.chat.Id, user, mreqAIUniqueNames()), memberReqUnknown)
		otherChat := uniqueModuleChatID()
		t.Cleanup(func() { memberReqDeleteRedisKeys([]int64{otherChat}) })
		mreqAIWantVerdict(t, "the same account's string in a second group", mreqAIAskIn(env, otherChat, user, mreqAIUniqueNames()), memberReqUnknown)
		if got := ai.count(); got != 5 {
			t.Errorf("requests = %d, want 5: a refused account makes no request", got)
		}
		stamp := mreqAIStamp()
		if got := mreqAICounter(t, mreqAIChatKey(env.chat.Id, stamp)); got != 5 {
			t.Errorf("the group's counter = %d, want 5: a refused account spends no group unit", got)
		}
		if got := mreqAICounter(t, mreqAIGlobalKey(stamp)); got != 5 {
			t.Errorf("the fleet's counter = %d, want 5: a refused account spends no fleet unit", got)
		}
		if got := mreqAICounter(t, mreqAIChatKey(otherChat, stamp)); got != 0 {
			t.Errorf("the second group's counter = %d, want 0", got)
		}
		after := memberReqAIStatsSnapshot()
		if got := after.Capped - before.Capped; got != 2 {
			t.Errorf("capped counter moved by %d, want 2", got)
		}
		if got := after.Checked - before.Checked; got != 5 {
			t.Errorf("checked counter moved by %d, want 5", got)
		}
	})

	t.Run("a group over its own budget spends no fleet unit", func(t *testing.T) {
		withAIBudgets(t, 100, 2, 400)
		env, ai := mreqAIGuardEnv(t)
		users := mreqAIUsers(t, env, 3)
		for i, user := range users {
			want := memberReqPass
			if i == 2 {
				want = memberReqUnknown
			}
			mreqAIWantVerdict(t, fmt.Sprintf("member %d", i+1), mreqAIAskIn(env, env.chat.Id, user, mreqAIUniqueNames()), want)
		}
		if got := ai.count(); got != 2 {
			t.Errorf("requests = %d, want 2", got)
		}
		if got := mreqAICounter(t, mreqAIGlobalKey(mreqAIStamp())); got != 2 {
			t.Errorf("the fleet's counter = %d, want 2: a group over its own budget never spends the fleet's", got)
		}
	})

	t.Run("the fleet's budget ends the calls of every group", func(t *testing.T) {
		withAIBudgets(t, 100, 100, 2)
		env, ai := mreqAIGuardEnv(t)
		users := mreqAIUsers(t, env, 3)
		otherChat := uniqueModuleChatID()
		t.Cleanup(func() { memberReqDeleteRedisKeys([]int64{otherChat}) })
		mreqAIWantVerdict(t, "first", mreqAIAskIn(env, env.chat.Id, users[0], mreqAIUniqueNames()), memberReqPass)
		mreqAIWantVerdict(t, "second, in another group", mreqAIAskIn(env, otherChat, users[1], mreqAIUniqueNames()), memberReqPass)
		mreqAIWantVerdict(t, "third", mreqAIAskIn(env, env.chat.Id, users[2], mreqAIUniqueNames()), memberReqUnknown)
		if got := ai.count(); got != 2 {
			t.Errorf("requests = %d, want 2", got)
		}
	})

	t.Run("a verdict served from the cache spends no unit at any level", func(t *testing.T) {
		withAIBudgets(t, 1, 100, 400)
		env, ai := mreqAIGuardEnv(t)
		users := mreqAIUsers(t, env, 2)
		names := mreqAIUniqueNames()
		mreqAIWantVerdict(t, "the string is judged", mreqAIAskIn(env, env.chat.Id, users[0], names), memberReqPass)
		stamp := mreqAIStamp()
		chatBefore, globalBefore := mreqAICounter(t, mreqAIChatKey(env.chat.Id, stamp)), mreqAICounter(t, mreqAIGlobalKey(stamp))
		mreqAIWantVerdict(t, "another member, same string", mreqAIAskIn(env, env.chat.Id, users[1], names), memberReqPass)
		mreqAIWantVerdict(t, "the first member's other string", mreqAIAskIn(env, env.chat.Id, users[0], mreqAIUniqueNames()), memberReqUnknown)
		userBefore := mreqAICounter(t, mreqAIUserKey(users[0], stamp))
		before := memberReqAIStatsSnapshot()
		mreqAIWantVerdict(t, "the first member, cached string, budget spent", mreqAIAskIn(env, env.chat.Id, users[0], names), memberReqPass)
		if got := ai.count(); got != 1 {
			t.Errorf("requests = %d, want 1", got)
		}
		if got := mreqAICounter(t, mreqAIUserKey(users[0], stamp)); got != userBefore {
			t.Errorf("the account's counter moved from %d to %d on a cached answer", userBefore, got)
		}
		if got := mreqAICounter(t, mreqAIChatKey(env.chat.Id, stamp)); got != chatBefore {
			t.Errorf("the group's counter moved from %d to %d", chatBefore, got)
		}
		if got := mreqAICounter(t, mreqAIGlobalKey(stamp)); got != globalBefore {
			t.Errorf("the fleet's counter moved from %d to %d", globalBefore, got)
		}
		if got := memberReqAIStatsSnapshot().Cached - before.Cached; got != 1 {
			t.Errorf("cached counter moved by %d, want 1", got)
		}
	})

	t.Run("the next UTC hour starts new counters", func(t *testing.T) {
		withAIBudgets(t, 1, 100, 400)
		clock := mreqAIInstallClock(t)
		env, ai := mreqAIGuardEnv(t)
		user := mreqAIUsers(t, env, 1)[0]
		mreqAIWantVerdict(t, "first string", mreqAIAskIn(env, env.chat.Id, user, mreqAIUniqueNames()), memberReqPass)
		mreqAIWantVerdict(t, "second string, same hour", mreqAIAskIn(env, env.chat.Id, user, mreqAIUniqueNames()), memberReqUnknown)
		clock.advance(time.Hour)
		mreqAIWantVerdict(t, "second string, next hour", mreqAIAskIn(env, env.chat.Id, user, mreqAIUniqueNames()), memberReqPass)
		if got := ai.count(); got != 2 {
			t.Errorf("requests = %d, want 2", got)
		}
	})

	t.Run("a Redis error on a budget key is unknown with no request", func(t *testing.T) {
		env, ai := mreqAIGuardEnv(t)
		user := mreqAIUsers(t, env, 1)[0]
		mreqSwap(t, &memberReqAITake, func(*redis.Client, string, int64) (bool, error) {
			return false, errors.New("redis is down")
		})
		before := memberReqAIStatsSnapshot()
		// More failures than the breaker's threshold: a fault of the moment is no state.
		for i := 0; i < 8; i++ {
			mreqAIWantVerdict(t, fmt.Sprintf("ask %d", i+1), mreqAIAskIn(env, env.chat.Id, user, mreqAIUniqueNames()), memberReqUnknown)
		}
		if got := ai.count(); got != 0 {
			t.Errorf("requests = %d, want 0", got)
		}
		if got := memberReqAIStatsSnapshot().Shed - before.Shed; got != 8 {
			t.Errorf("shed counter moved by %d, want 8", got)
		}
	})

	t.Run("the keys are the three of the family, each with a two hour expiry, and the image budgets are untouched", func(t *testing.T) {
		env, _ := mreqAIGuardEnv(t)
		user := mreqAIUsers(t, env, 1)[0]
		mreqAIWantVerdict(t, "ask", mreqAIAskIn(env, env.chat.Id, user, mreqAIUniqueNames()), memberReqPass)
		stamp := mreqAIStamp()
		client := memberReqRedisClient()
		for _, key := range []string{mreqAIUserKey(user, stamp), mreqAIChatKey(env.chat.Id, stamp), mreqAIGlobalKey(stamp)} {
			ttl := client.TTL(context.Background(), key).Val()
			if ttl <= time.Hour || ttl > 2*time.Hour {
				t.Errorf("TTL of %s = %v, want a two hour expiry", key, ttl)
			}
		}
		keys, err := client.Keys(context.Background(), "fuku:memberreq:aib:*").Result()
		if err != nil {
			t.Fatal(err)
		}
		if len(keys) != 3 {
			t.Errorf("budget keys = %v, want exactly the account's, the group's and the fleet's", keys)
		}
		for _, pattern := range []string{"fuku:raid:aibudget:*", "fuku:raid:*:ab:*", "fuku:raid:*:abu:*"} {
			if found := client.Keys(context.Background(), pattern).Val(); len(found) != 0 {
				t.Errorf("image budget keys %v exist: the name check must neither read nor write them", found)
			}
		}
	})
}

func TestMemberReqAIGate(t *testing.T) {
	t.Run("eight strings at once run at most two requests at once and all get their verdicts", func(t *testing.T) {
		withAIGate(t, 2, 5*time.Second)
		env, ai := mreqAIGuardEnv(t)
		ai.setDefault(mreqAIDelayed(40*time.Millisecond, mreqAIProbability(0.1)))
		users := mreqAIUsers(t, env, 8)
		verdicts := make([]memberReqVerdict, len(users))
		var wg sync.WaitGroup
		for i, user := range users {
			wg.Add(1)
			go func() {
				defer wg.Done()
				verdicts[i] = mreqAIAskIn(env, env.chat.Id, user, mreqAIUniqueNames())
			}()
		}
		wg.Wait()
		for i, got := range verdicts {
			mreqAIWantVerdict(t, fmt.Sprintf("caller %d", i), got, memberReqPass)
		}
		if got := ai.maxInFlight(); got < 1 || got > 2 {
			t.Errorf("most requests in flight = %d, want at most the gate's 2", got)
		}
		if got := ai.count(); got != 8 {
			t.Errorf("requests = %d, want 8", got)
		}
	})

	t.Run("a flight that gets no slot in time is a sticky unknown that spends nothing, and the held slot is not leaked", func(t *testing.T) {
		withAIGate(t, 1, 150*time.Millisecond)
		env, ai := mreqAIGuardEnv(t)
		open := make(chan struct{})
		ai.setDefault(mreqAIGated(open, mreqAIProbability(0.1)))
		users := mreqAIUsers(t, env, 4)
		holder := make(chan memberReqVerdict, 1)
		go func() { holder <- mreqAIAskIn(env, env.chat.Id, users[0], mreqAIUniqueNames()) }()
		ai.waitCount(t, 1)

		waiting := mreqAIUniqueNames()
		before := memberReqAIStatsSnapshot()
		started := time.Now()
		mreqAIWantVerdict(t, "the caller with no slot", mreqAIAskIn(env, env.chat.Id, users[1], waiting), memberReqUnknown)
		if took := time.Since(started); took < 100*time.Millisecond || took > 2*time.Second {
			t.Errorf("the caller waited %v, want about the gate's 150 ms", took)
		}
		if got := ai.count(); got != 1 {
			t.Errorf("requests = %d, want 1: no slot, no request", got)
		}
		stamp := mreqAIStamp()
		if got := mreqAICounter(t, mreqAIUserKey(users[1], stamp)); got != 0 {
			t.Errorf("the waiting account's counter = %d, want 0: the gate is taken before the budget", got)
		}
		if got := mreqAICounter(t, mreqAIChatKey(env.chat.Id, stamp)); got != 1 {
			t.Errorf("the group's counter = %d, want 1 (the holder's)", got)
		}
		if got := memberReqAIStatsSnapshot().Shed - before.Shed; got != 1 {
			t.Errorf("shed counter moved by %d, want 1", got)
		}

		// The same refusal through the flight body shows the answer is sticky.
		plan := memberReqAIPlan{canon: mreqAICanon(t, waiting.First, waiting.Last, waiting.Username), key: mreqAIKeyOf(t, waiting)}
		answer := memberReqAIJudge(context.Background(), env.chat.Id, users[2], plan)
		if answer.Verdict != memberReqUnknown || !answer.Sticky {
			t.Errorf("answer without a slot = %+v, want a sticky unknown", answer)
		}

		close(open)
		mreqAIWantVerdict(t, "the holder", <-holder, memberReqPass)
		mreqAIWantVerdict(t, "a later flight", mreqAIAskIn(env, env.chat.Id, users[3], mreqAIUniqueNames()), memberReqPass)
		if got := ai.count(); got != 2 {
			t.Errorf("requests = %d, want 2", got)
		}
	})

	t.Run("a cancelled flight context returns at once with the slot released", func(t *testing.T) {
		withAIGate(t, 1, 3*time.Second)
		env, ai := mreqAIGuardEnv(t)
		open := make(chan struct{})
		ai.setDefault(mreqAIGated(open, mreqAIProbability(0.1)))
		users := mreqAIUsers(t, env, 3)
		holder := make(chan memberReqVerdict, 1)
		go func() { holder <- mreqAIAskIn(env, env.chat.Id, users[0], mreqAIUniqueNames()) }()
		ai.waitCount(t, 1)

		names := mreqAIUniqueNames()
		plan := memberReqAIPlan{canon: mreqAICanon(t, names.First, names.Last, names.Username), key: mreqAIKeyOf(t, names)}
		ctx, cancel := context.WithCancel(context.Background())
		answers := make(chan memberReqAIAnswer, 1)
		go func() { answers <- memberReqAIJudge(ctx, env.chat.Id, users[1], plan) }()
		time.Sleep(50 * time.Millisecond)
		started := time.Now()
		cancel()
		select {
		case answer := <-answers:
			if took := time.Since(started); took > time.Second {
				t.Errorf("a cancelled flight took %v to return, want at once", took)
			}
			if answer.Verdict != memberReqUnknown || answer.Sticky {
				t.Errorf("answer after a cancel = %+v, want a plain unknown (shutdown is not a state)", answer)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("a cancelled flight never returned")
		}
		close(open)
		mreqAIWantVerdict(t, "the holder", <-holder, memberReqPass)
		mreqAIWantVerdict(t, "a flight after the cancel", mreqAIAskIn(env, env.chat.Id, users[2], mreqAIUniqueNames()), memberReqPass)
	})

	t.Run("a call that panics is recovered, returns unknown and releases the slot", func(t *testing.T) {
		withAIGate(t, 1, 3*time.Second)
		env, ai := mreqAIGuardEnv(t)
		users := mreqAIUsers(t, env, 2)
		previous := memberReqAICall
		memberReqAICall = func(context.Context, *claude.Client, claude.Names) (*claude.Verdict, error) {
			panic("the call blew up")
		}
		t.Cleanup(func() { memberReqAICall = previous })
		mreqAIWantVerdict(t, "the panicking call", mreqAIAskIn(env, env.chat.Id, users[0], mreqAIUniqueNames()), memberReqUnknown)
		if got := mreqAICounter(t, mreqAIUserKey(users[0], mreqAIStamp())); got != 1 {
			t.Errorf("the account's counter = %d, want 1: the unit is reserved before the call", got)
		}
		memberReqAICall = previous
		mreqAIWantVerdict(t, "the call after the panic", mreqAIAskIn(env, env.chat.Id, users[1], mreqAIUniqueNames()), memberReqPass)
		if got := ai.count(); got != 1 {
			t.Errorf("requests = %d, want 1", got)
		}
	})
}

func TestMemberReqAIFlight(t *testing.T) {
	t.Run("100 members with one string make exactly one request", func(t *testing.T) {
		env, ai := mreqAIGuardEnv(t)
		users := mreqAIUsers(t, env, 100)
		names := mreqAIUniqueNames()
		start := make(chan struct{})
		verdicts := make([]memberReqVerdict, len(users))
		var wg sync.WaitGroup
		for i, user := range users {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				verdicts[i] = mreqAIAskIn(env, env.chat.Id, user, names)
			}()
		}
		close(start)
		wg.Wait()
		for i, got := range verdicts {
			if got != memberReqPass {
				t.Fatalf("member %d: verdict = %v, want pass", i, got)
			}
		}
		if got := ai.count(); got != 1 {
			t.Errorf("requests = %d, want exactly 1", got)
		}
	})

	t.Run("100 members with the call held open still make one request", func(t *testing.T) {
		env, ai := mreqAIGuardEnv(t)
		open := make(chan struct{})
		ai.setDefault(mreqAIGated(open, mreqAIProbability(0.1)))
		users := mreqAIUsers(t, env, 100)
		names := mreqAIUniqueNames()
		verdicts := make([]memberReqVerdict, len(users))
		var wg sync.WaitGroup
		for i, user := range users {
			wg.Add(1)
			go func() {
				defer wg.Done()
				verdicts[i] = mreqAIAskIn(env, env.chat.Id, user, names)
			}()
		}
		ai.waitCount(t, 1)
		time.Sleep(150 * time.Millisecond)
		if got := ai.count(); got != 1 {
			t.Errorf("requests while the call is held = %d, want 1", got)
		}
		close(open)
		wg.Wait()
		for i, got := range verdicts {
			if got != memberReqPass {
				t.Fatalf("member %d: verdict = %v, want pass", i, got)
			}
		}
		if got := ai.count(); got != 1 {
			t.Errorf("requests = %d, want exactly 1", got)
		}
	})

	t.Run("a follower with a short context gets unknown while the leader still gets its answer", func(t *testing.T) {
		env, ai := mreqAIGuardEnv(t)
		open := make(chan struct{})
		ai.setDefault(mreqAIGated(open, mreqAIProbability(0.1)))
		users := mreqAIUsers(t, env, 2)
		names := mreqAIUniqueNames()
		leader := make(chan memberReqVerdict, 1)
		go func() { leader <- mreqAIAskIn(env, env.chat.Id, users[0], names) }()
		ai.waitCount(t, 1)
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		got := memberReqAIRule(ctx, env.bot, env.chat.Id, users[1], memberReqLookupMode{Names: names, Joining: true})
		mreqAIWantVerdict(t, "the follower", got, memberReqUnknown)
		close(open)
		mreqAIWantVerdict(t, "the leader", <-leader, memberReqPass)
		if got := ai.count(); got != 1 {
			t.Errorf("requests = %d, want 1", got)
		}
	})

	t.Run("100 distinct strings with the gate at four make 100 requests, at most four at once", func(t *testing.T) {
		withAIGate(t, 4, 20*time.Second)
		withAIBudgets(t, 5, 1000, 1000)
		env, ai := mreqAIGuardEnv(t)
		ai.setDefault(mreqAIDelayed(5*time.Millisecond, mreqAIProbability(0.1)))
		users := mreqAIUsers(t, env, 100)
		verdicts := make([]memberReqVerdict, len(users))
		var wg sync.WaitGroup
		for i, user := range users {
			wg.Add(1)
			go func() {
				defer wg.Done()
				verdicts[i] = mreqAIAskIn(env, env.chat.Id, user, mreqAIUniqueNames())
			}()
		}
		wg.Wait()
		for i, got := range verdicts {
			if got != memberReqPass {
				t.Fatalf("caller %d: verdict = %v, want pass", i, got)
			}
		}
		if got := ai.count(); got != 100 {
			t.Errorf("requests = %d, want 100", got)
		}
		if got := ai.maxInFlight(); got > 4 {
			t.Errorf("most requests in flight = %d, want at most 4", got)
		}
	})
}
