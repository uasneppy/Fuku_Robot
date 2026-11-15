//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"

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

// mreqAIFailure is a provider failure the way the Claude client reports it.
func mreqAIFailure(status int, errType string) error {
	return &claude.HTTPError{Status: status, Type: errType}
}

// mreqAIPauseWarnings counts the warnings that announce a pause of the name check whose text
// contains marker ("this group" for a group's, "every group" for the provider-wide one).
func mreqAIPauseWarnings(hook interface{ AllEntries() []*log.Entry }, marker string) int {
	count := 0
	for _, entry := range hook.AllEntries() {
		if entry.Level == log.WarnLevel && strings.Contains(entry.Message, "AI name checks paused") && strings.Contains(entry.Message, marker) {
			count++
		}
	}
	return count
}

// mreqAITripGroup makes five provider failures count against a group, directly.
func mreqAITripGroup(chatID int64) {
	for i := 0; i < memberReqAIFailures; i++ {
		memberReqAIRecordFailure(chatID, mreqAIFailure(400, "invalid_request_error"))
	}
}

func TestMemberReqAIBreaker(t *testing.T) {
	// asker returns a function that judges a new string of a new member of the group and says what
	// the stand-in answered with step first.
	asker := func(t *testing.T, env *memberReqEnv, ai *mreqAIState, chatID int64, total int) func(steps ...mreqAIStep) memberReqVerdict {
		t.Helper()
		users := mreqAIUsers(t, env, total)
		next := 0
		return func(steps ...mreqAIStep) memberReqVerdict {
			t.Helper()
			ai.enqueue(steps...)
			user := users[next]
			next++
			return mreqAIAskIn(env, chatID, user, mreqAIUniqueNames())
		}
	}
	fourHundred := mreqAIStatus(400, "invalid_request_error", "")

	t.Run("five failures pause the group, nothing is spent while paused, another group is unaffected and the pause ends", func(t *testing.T) {
		clock := mreqAIInstallClock(t)
		env, ai := mreqAIGuardEnv(t)
		ask := asker(t, env, ai, env.chat.Id, 10)
		otherChat := uniqueModuleChatID()
		t.Cleanup(func() { memberReqDeleteRedisKeys([]int64{otherChat}) })
		// A 500 is retried once by the client (two requests), a 400 and a dropped connection are final:
		// each kind counts one failure.
		kinds := [][]mreqAIStep{
			{mreqAIStatus(500, "api_error", ""), mreqAIStatus(500, "api_error", "")},
			{fourHundred}, {mreqAIDrop()}, {fourHundred}, {fourHundred},
		}
		for i, steps := range kinds {
			if paused := memberReqAIPaused(env.chat.Id); paused {
				t.Fatalf("the group is paused before failure %d", i+1)
			}
			mreqAIWantVerdict(t, fmt.Sprintf("failure %d", i+1), ask(steps...), memberReqUnknown)
		}
		if !memberReqAIPaused(env.chat.Id) {
			t.Fatal("the group is not paused after five failures")
		}
		requests, stamp := ai.count(), mreqAIStamp()
		chatUnits := mreqAICounter(t, mreqAIChatKey(env.chat.Id, stamp))
		before := memberReqAIStatsSnapshot()
		mreqAIWantVerdict(t, "a check while paused", ask(), memberReqUnknown)
		if got := ai.count(); got != requests {
			t.Errorf("requests while paused = %d, want %d", got, requests)
		}
		if got := mreqAICounter(t, mreqAIChatKey(env.chat.Id, stamp)); got != chatUnits {
			t.Errorf("the group's counter moved from %d to %d while paused: a paused check spends no unit", chatUnits, got)
		}
		if got := memberReqAIStatsSnapshot().Shed - before.Shed; got != 1 {
			t.Errorf("shed counter moved by %d, want 1", got)
		}
		if memberReqAIPaused(otherChat) {
			t.Error("another group is paused by this group's failures")
		}
		mreqAIWantVerdict(t, "another group", mreqAIAskIn(env, otherChat, mreqAIUsers(t, env, 1)[0], mreqAIUniqueNames()), memberReqPass)
		clock.advance(memberReqAICooldown + time.Second)
		if memberReqAIPaused(env.chat.Id) {
			t.Error("the group is still paused after the cooldown")
		}
		mreqAIWantVerdict(t, "a check after the cooldown", ask(), memberReqPass)
	})

	t.Run("a success between failures clears the count and a refusal stop counts as an answer", func(t *testing.T) {
		for name, answer := range map[string]mreqAIStep{
			"a success":      mreqAIProbability(0.1),
			"a refusal stop": mreqAIBody(raidAIClaudeRefusal()),
		} {
			t.Run(name, func(t *testing.T) {
				env, ai := mreqAIGuardEnv(t)
				ask := asker(t, env, ai, env.chat.Id, 12)
				for i := 0; i < memberReqAIFailures-1; i++ {
					ask(fourHundred)
				}
				ask(answer)
				for i := 0; i < memberReqAIFailures-1; i++ {
					ask(fourHundred)
				}
				if memberReqAIPaused(env.chat.Id) {
					t.Fatal("the group is paused: the answer between the failures should have cleared the count")
				}
				ask(fourHundred)
				if !memberReqAIPaused(env.chat.Id) {
					t.Error("the group is not paused after five failures in a row")
				}
			})
		}
	})

	t.Run("a cancelled context is neither a failure nor a success", func(t *testing.T) {
		mreqSwap(t, &memberReqLookupFlightTimeout, 150*time.Millisecond)
		env, ai := mreqAIGuardEnv(t)
		ask := asker(t, env, ai, env.chat.Id, 8)
		for i := 0; i < memberReqAIFailures-1; i++ {
			ask(fourHundred)
		}
		mreqAIWantVerdict(t, "a request that outlives the flight", ask(mreqAIHang()), memberReqUnknown)
		time.Sleep(100 * time.Millisecond)
		if memberReqAIPaused(env.chat.Id) {
			t.Fatal("the cancelled call was counted as a failure")
		}
		ask(fourHundred)
		if !memberReqAIPaused(env.chat.Id) {
			t.Error("the group is not paused: the cancelled call cleared the count as a success")
		}
	})

	t.Run("a rejected key or account pauses every group", func(t *testing.T) {
		for _, failure := range []struct {
			status  int
			errType string
		}{{401, "authentication_error"}, {402, "billing_error"}, {403, "permission_error"}} {
			t.Run(fmt.Sprint(failure.status), func(t *testing.T) {
				clock := mreqAIInstallClock(t)
				hook := mreqLogCapture(t)
				env, ai := mreqAIGuardEnv(t)
				ask := asker(t, env, ai, env.chat.Id, 2)
				mreqAIWantVerdict(t, "the rejected call", ask(mreqAIStatus(failure.status, failure.errType, "")), memberReqUnknown)
				otherChat := uniqueModuleChatID()
				if !memberReqAIPaused(otherChat) {
					t.Error("another group is not paused by a rejected key")
				}
				requests := ai.count()
				mreqAIWantVerdict(t, "a check in another group", mreqAIAskIn(env, otherChat, mreqAIUsers(t, env, 1)[0], mreqAIUniqueNames()), memberReqUnknown)
				if got := ai.count(); got != requests {
					t.Errorf("requests while the provider is paused = %d, want %d", got, requests)
				}
				clock.advance(memberReqAIKeyPause - time.Second)
				if !memberReqAIPaused(otherChat) {
					t.Error("the provider-wide pause ended early")
				}
				clock.advance(2 * time.Second)
				if memberReqAIPaused(otherChat) {
					t.Error("the provider-wide pause outlived its span")
				}
				if got := mreqAIPauseWarnings(hook, "every group"); got != 1 {
					t.Errorf("provider-wide pause warnings = %d, want exactly one", got)
				}
				mreqWantCleanLogs(t, hook, append([]string{raidAITestKey, "Promo"}, mreqLogForbidden...),
					fmt.Sprintf("HTTP %d %s", failure.status, failure.errType))
			})
		}
	})

	t.Run("a 429 pauses every group only for a retry-after over two seconds, for at most five minutes", func(t *testing.T) {
		for _, tc := range []struct {
			retryAfter string
			span       time.Duration
		}{{"30", 30 * time.Second}, {"900", memberReqAIMaxPause}} {
			t.Run("retry-after "+tc.retryAfter, func(t *testing.T) {
				clock := mreqAIInstallClock(t)
				env, ai := mreqAIGuardEnv(t)
				ask := asker(t, env, ai, env.chat.Id, 1)
				ask(mreqAIStatus(429, "rate_limit_error", tc.retryAfter))
				otherChat := uniqueModuleChatID()
				clock.advance(tc.span - time.Second)
				if !memberReqAIPaused(otherChat) {
					t.Errorf("another group is not paused %v into a %v pause", tc.span-time.Second, tc.span)
				}
				clock.advance(2 * time.Second)
				if memberReqAIPaused(otherChat) {
					t.Errorf("the pause outlived %v", tc.span)
				}
			})
		}
		t.Run("a short retry-after only counts toward its group", func(t *testing.T) {
			env, ai := mreqAIGuardEnv(t)
			ask := asker(t, env, ai, env.chat.Id, 6)
			// The client waits the one second and retries once, so two answers make one failure.
			ask(mreqAIStatus(429, "rate_limit_error", "1"), mreqAIStatus(429, "rate_limit_error", "1"))
			if memberReqAIPaused(uniqueModuleChatID()) {
				t.Fatal("a short rate limit paused every group")
			}
			for i := 0; i < memberReqAIFailures-1; i++ {
				ask(fourHundred)
			}
			if !memberReqAIPaused(env.chat.Id) {
				t.Error("the short rate limit did not count toward its group")
			}
		})
	})

	t.Run("a pause that is running is not extended or logged again by checks still in flight", func(t *testing.T) {
		clock := mreqAIInstallClock(t)
		hook := mreqLogCapture(t)
		mreqAIFreshBreaker(t)
		chat := uniqueModuleChatID()
		mreqAITripGroup(chat)
		if !memberReqAIPaused(chat) {
			t.Fatal("the group is not paused after five failures")
		}
		clock.advance(memberReqAICooldown - time.Minute)
		for i := 0; i < 3; i++ {
			memberReqAIRecordFailure(chat, mreqAIFailure(500, "api_error"))
		}
		clock.advance(time.Minute + time.Second)
		if memberReqAIPaused(chat) {
			t.Error("late failures extended a pause that was running")
		}
		if got := mreqAIPauseWarnings(hook, "this group"); got != 1 {
			t.Errorf("group pause warnings = %d, want exactly one", got)
		}
		memberReqAIRecordFailure(chat, mreqAIFailure(401, "authentication_error"))
		clock.advance(time.Minute)
		memberReqAIRecordFailure(chat, mreqAIFailure(401, "authentication_error"))
		if got := mreqAIPauseWarnings(hook, "every group"); got != 1 {
			t.Errorf("provider pause warnings = %d, want exactly one: a running pause is not logged again", got)
		}
	})

	t.Run("the group map is bounded at 10,000 and an idle breaker is pruned to make room", func(t *testing.T) {
		hook := mreqLogCapture(t)
		full := newMemberReqAIBreaker()
		for id := int64(1); id <= memberReqAIBreakerMaxChats; id++ {
			full.groups[id] = &memberReqAIGroup{failures: 1}
		}
		mreqSwap(t, &memberReqAIBreakers, full)
		stranger := int64(memberReqAIBreakerMaxChats + 1)
		for i := 0; i < 2*memberReqAIFailures; i++ {
			memberReqAIRecordFailure(stranger, mreqAIFailure(400, "invalid_request_error"))
		}
		if memberReqAIPaused(stranger) {
			t.Error("a group that is not tracked was paused")
		}
		if got := len(full.groups); got != memberReqAIBreakerMaxChats {
			t.Errorf("tracked groups = %d, want the bound %d", got, memberReqAIBreakerMaxChats)
		}
		warnings := 0
		for _, entry := range hook.AllEntries() {
			if entry.Level == log.WarnLevel && strings.Contains(entry.Message, "breaker map is full") {
				warnings++
			}
		}
		if warnings != 1 {
			t.Errorf("map-full warnings = %d, want exactly one per process", warnings)
		}

		idle := newMemberReqAIBreaker()
		for id := int64(1); id <= memberReqAIBreakerMaxChats; id++ {
			idle.groups[id] = &memberReqAIGroup{}
		}
		mreqSwap(t, &memberReqAIBreakers, idle)
		mreqAITripGroup(stranger)
		if !memberReqAIPaused(stranger) {
			t.Error("an idle breaker was not pruned to make room for a failing group")
		}
	})

	t.Run("the image check and the name check share no breaker", func(t *testing.T) {
		resetRaidAIImageBreakers(t)
		mreqAIFreshBreaker(t)
		chat := uniqueModuleChatID()
		for i := 0; i < memberReqAIFailures; i++ {
			memberReqAIRecordFailure(chat, mreqAIFailure(500, "api_error"))
		}
		memberReqAIRecordFailure(chat, mreqAIFailure(401, "authentication_error"))
		if raidAIImagePaused(chat) || raidAIImagePaused(uniqueModuleChatID()) {
			t.Error("name check failures paused the image check")
		}
		mreqAIFreshBreaker(t)
		for i := 0; i < raidAIImageFailures; i++ {
			raidAIImageRecordFailure(chat, mreqAIFailure(500, "api_error"))
		}
		raidAIImageRecordFailure(chat, mreqAIFailure(401, "authentication_error"))
		if !raidAIImagePaused(chat) {
			t.Fatal("the image breaker did not pause: the test does not prove isolation")
		}
		if memberReqAIPaused(chat) || memberReqAIPaused(uniqueModuleChatID()) {
			t.Error("image check failures paused the name check")
		}
	})
}

func TestMemberReqAIUnknownDefers(t *testing.T) {
	type cause struct {
		name   string
		sticky bool
		arm    func(t *testing.T, env *memberReqEnv, ai *mreqAIState)
	}
	causes := []cause{
		{"a 400", false, func(_ *testing.T, _ *memberReqEnv, ai *mreqAIState) {
			ai.setDefault(mreqAIStatus(400, "invalid_request_error", ""))
		}},
		{"a timeout", false, func(t *testing.T, _ *memberReqEnv, ai *mreqAIState) {
			mreqSwap(t, &memberReqLookupFlightTimeout, 100*time.Millisecond)
			ai.setDefault(mreqAIHang())
		}},
		{"a paused breaker", true, func(_ *testing.T, env *memberReqEnv, _ *mreqAIState) { mreqAITripGroup(env.chat.Id) }},
		{"a used-up budget", true, func(t *testing.T, _ *memberReqEnv, _ *mreqAIState) { withAIBudgets(t, 0, 100, 400) }},
		{"a full gate", true, func(t *testing.T, _ *memberReqEnv, _ *mreqAIState) {
			withAIGate(t, 1, 100*time.Millisecond)
			if err := memberReqAIGate.Acquire(context.Background(), 1); err != nil {
				t.Fatalf("take the only slot: %v", err)
			}
		}},
	}
	rows := []struct{ name, ai int }{
		{models.RequirementActionWarn, models.RequirementActionBan},
		{models.RequirementActionWarn, models.RequirementActionMute},
		{models.RequirementActionWarn, models.RequirementActionWarn},
		{models.RequirementActionMute, models.RequirementActionMute},
		{models.RequirementActionKick, models.RequirementActionKick},
		{models.RequirementActionKick, models.RequirementActionWarn},
		{models.RequirementActionMute, models.RequirementActionBan},
	}
	for _, row := range rows {
		for _, c := range causes {
			// The AI rule is asked after the name rules, and a stricter one that could not be answered
			// defers the claim (WR-02) unless its unknown is sticky (D-22).
			deferred := memberReqSeverity(row.ai) > memberReqSeverity(row.name) && !c.sticky
			t.Run(fmt.Sprintf("name rule %d, AI rule %d, %s", row.name, row.ai, c.name), func(t *testing.T) {
				env, ai := mreqAIEnvOf(t, map[memberreq.Rule]int{memberreq.RuleLastName: row.name, memberreq.RuleAI: row.ai}, false)
				member := mreqAIMember(env, "")
				mreqAIMarkNew(t, env.chat.Id, member.Id)
				c.arm(t, env, ai)
				got := env.mreqEnforce(env.mreqCase(member))
				if deferred {
					mreqAIWantNothing(t, env, member, got)
					return
				}
				if got.Outcome == memberReqNone {
					t.Fatalf("outcome = %+v, want the name rule's punishment to stand", got)
				}
				if hold := env.mreqHold(member.Id); hold == nil || hold.Rules != mreqBitOf(t, "ln") || hold.Action != row.name {
					t.Errorf("hold = %+v, want one with the last name bit only and the name rule's action", hold)
				}
			})
		}
	}

	t.Run("a sticky unknown stays unknown in the lookup fold, which the held release and I fixed it read", func(t *testing.T) {
		env, _ := mreqAIEnvOf(t, map[memberreq.Rule]int{memberreq.RuleAI: models.RequirementActionBan}, false)
		withAIBudgets(t, 0, 100, 400)
		member := mreqAIMember(env, "Casino")
		settings := env.mreqRules(map[memberreq.Rule]int{memberreq.RuleAI: models.RequirementActionBan})
		names := mreqAINamesOfUser(member)
		broken, unknown := memberReqLookupBroken(context.Background(), env.bot, settings, env.chat.Id, member.Id,
			memberReqLookupMode{Names: names, Joining: true, Fresh: true, Classify: true})
		if broken != 0 || !unknown {
			t.Errorf("fold = broken %d unknown %v, want an unknown that is not a pass or a breach", broken, unknown)
		}
	})

	t.Run("a held member with a hold carrying the AI bit is not released while the answer is a sticky unknown", func(t *testing.T) {
		env, ai := mreqAIWatchEnv(t, models.RequirementActionWarn)
		withAIBudgets(t, 0, 100, 400)
		member := mreqAIMember(env, "Casino")
		mreqSeedHeld(t, env, member, mreqBitOf(t, "ai"), models.RequirementActionWarn, 10*time.Minute)
		mreqAIMarkNew(t, env.chat.Id, member.Id)
		renamed := mreqAIOrdinary(env)
		env.fake.setProfile(member.Id, renamed.FirstName, renamed.LastName, renamed.Username)
		member.FirstName, member.LastName, member.Username = renamed.FirstName, renamed.LastName, renamed.Username
		env.mreqSend(member, "I changed my name")
		memberReqJobs.wait()
		if n := ai.count(); n != 0 {
			t.Errorf("provider requests = %d, want none: the budget is used up", n)
		}
		if hold := env.mreqHold(member.Id); hold == nil {
			t.Error("the hold was released on a sticky unknown")
		}
	})
}

func TestMemberReqAINamesChangedDuringLookup(t *testing.T) {
	// changing makes the stand-in change the member's profile at the fake when the first request
	// arrives, then answer 0.97 for the string it was asked about; every later request is a 0.1.
	changing := func(env *memberReqEnv, ai *mreqAIState, member gotgbot.User, first, last string) {
		var once sync.Once
		ai.setOnRequest(func() {
			once.Do(func() { env.fake.setProfile(member.Id, first, last, member.Username) })
		})
		ai.enqueue(mreqAIProbability(0.97))
	}

	t.Run("a member who renamed during the call is not punished for the old string and is judged again at their next message", func(t *testing.T) {
		env, ai := mreqAIGuardEnv(t)
		member := mreqAIMember(env, "Casino")
		mreqAIMarkNew(t, env.chat.Id, member.Id)
		oldNames := mreqAINamesOfUser(member)
		other := mreqAIUsers(t, env, 1)[0] // minted now: mreqAIUsers also empties the verdict cache
		renamed := member
		renamed.FirstName = fmt.Sprintf("Fresh%d", member.Id)
		changing(env, ai, member, renamed.FirstName, renamed.LastName)

		got := env.mreqEnforce(env.mreqCase(member))
		mreqWantNothing(t, env, member, got)
		if n := ai.count(); n != 1 {
			t.Fatalf("requests = %d, want 1 (the old string was judged)", n)
		}
		// The verdict of the old string is a fact about that string and stays cached for anyone who has it.
		mreqAIWantVerdict(t, "another member with the old string", mreqAIAskIn(env, env.chat.Id, other, oldNames), memberReqBreach)
		if n := ai.count(); n != 1 {
			t.Errorf("requests = %d, want 1: the old string's verdict is cached", n)
		}

		got = env.mreqEnforce(env.mreqCase(renamed))
		mreqWantNothing(t, env, renamed, got)
		if n := ai.count(); n != 2 {
			t.Errorf("requests = %d, want 2: the new names are judged at the next message", n)
		}
		mreqAIWantBaseline(t, member.Id, mreqAIHashOfUser(t, renamed))
		mreqAIWantMarker(t, env.chat.Id, member.Id, false)
	})

	t.Run("the control case with an unchanged profile punishes", func(t *testing.T) {
		env, ai := mreqAIGuardEnv(t)
		member := mreqAIMember(env, "Casino")
		mreqAIMarkNew(t, env.chat.Id, member.Id)
		ai.enqueue(mreqAIProbability(0.97))
		if got := env.mreqEnforce(env.mreqCase(member)); got.Outcome != memberReqRemoved {
			t.Errorf("outcome = %+v, want Removed (ban)", got)
		}
	})

	t.Run("a change that canonicalisation removes does not drop the bit", func(t *testing.T) {
		env, ai := mreqAIGuardEnv(t)
		member := mreqAIMember(env, "Casino")
		mreqAIMarkNew(t, env.chat.Id, member.Id)
		changing(env, ai, member, member.FirstName+"\u200b", member.LastName)
		if got := env.mreqEnforce(env.mreqCase(member)); got.Outcome != memberReqRemoved {
			t.Errorf("outcome = %+v, want Removed: a zero-width character is not a rename", got)
		}
	})

	t.Run("a name rule broken by the new names is still punished on the re-read record", func(t *testing.T) {
		env, ai := mreqAIEnvOf(t, map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionWarn, memberreq.RuleAI: models.RequirementActionBan}, false)
		member := mreqAIMember(env, "Casino")
		mreqAIMarkNew(t, env.chat.Id, member.Id)
		changing(env, ai, member, member.FirstName, "")
		got := env.mreqEnforce(env.mreqCase(member))
		if got.Outcome != memberReqWarned {
			t.Fatalf("outcome = %+v, want Warned for the last name rule the new names break", got)
		}
		if hold := env.mreqHold(member.Id); hold == nil || hold.Rules != mreqBitOf(t, "ln") {
			t.Errorf("hold = %+v, want one with the last name bit only", hold)
		}
	})

	t.Run("a change from judgeable to not judgeable drops the bit", func(t *testing.T) {
		env, ai := mreqAIGuardEnv(t)
		member := mreqAIMember(env, "Casino")
		mreqAIMarkNew(t, env.chat.Id, member.Id)
		var once sync.Once
		ai.setOnRequest(func() {
			once.Do(func() { env.fake.setProfile(member.Id, "\u200b", "", "") })
		})
		ai.enqueue(mreqAIProbability(0.97))
		mreqWantNothing(t, env, member, env.mreqEnforce(env.mreqCase(member)))
	})
}

func TestMemberReqAIVerdictCache(t *testing.T) {
	t.Run("the value is the probability as a decimal string and it expires", func(t *testing.T) {
		env, ai := mreqAIGuardEnv(t)
		names := mreqAIUniqueNames()
		ai.setDefault(mreqAIProbability(0.97))
		mreqAIWantVerdict(t, "first", mreqAIAsk(env, names, memberReqLookupMode{}), memberReqBreach)
		key := mreqAIKeyOf(t, names)
		value, ttl, ok := mreqAIKeyState(t, key)
		if !ok || value != "0.97" {
			t.Fatalf("stored value = %q (found %v), want the decimal string 0.97", value, ok)
		}
		if ttl <= 23*time.Hour || ttl > 24*time.Hour {
			t.Errorf("TTL = %v, want the 24 hour expiry", ttl)
		}
		mreqAIWantVerdict(t, "second, cached", mreqAIAsk(env, names, memberReqLookupMode{}), memberReqBreach)
		if n := ai.count(); n != 1 {
			t.Fatalf("requests = %d, want 1", n)
		}
		env.redis.FastForward(24*time.Hour + time.Minute)
		ai.setDefault(mreqAIProbability(0.1))
		mreqAIWantVerdict(t, "after the expiry", mreqAIAsk(env, names, memberReqLookupMode{}), memberReqPass)
		if n := ai.count(); n != 2 {
			t.Errorf("requests = %d, want 2: the string is judged again after the expiry", n)
		}
	})

	t.Run("a value that is not a finite probability from 0 to 1 is a miss in the rule's read and the watcher's", func(t *testing.T) {
		const hash = "0123456789abcdef0123456789abcdef"
		for _, planted := range []string{"", "abc", "0.5x", "-0.1", "1.5", "NaN", "+Inf", "Inf", "-Inf"} {
			t.Run(fmt.Sprintf("%q", planted), func(t *testing.T) {
				env, ai := mreqAIGuardEnv(t)
				names := mreqAIUniqueNames()
				mreqAIPlant(t, env, mreqAIKeyOf(t, names), planted)
				got := mreqAIAsk(env, names, memberReqLookupMode{})
				if got != memberReqPass {
					t.Errorf("verdict = %v, want a pass from a fresh 0.1 judgement: a planted %q is no cached verdict", got, planted)
				}
				if n := ai.count(); n != 1 {
					t.Errorf("requests = %d, want 1: the value is a miss", n)
				}
				if value, _, _ := mreqAIKeyState(t, mreqAIKeyOf(t, names)); value != "0.1" {
					t.Errorf("stored value = %q, want it overwritten by the answer 0.1", value)
				}
				if memberReqAIWatchSatisfied(planted, "", false, hash) || memberReqAIWatchSatisfied(planted, hash, true, hash) {
					t.Errorf("the watcher took %q for a verdict below the bar", planted)
				}
				if _, ok := memberReqAIParseProbability(planted); ok {
					t.Errorf("memberReqAIParseProbability accepted %q", planted)
				}
			})
		}
		if _, ok := memberReqAIParseProbability(strconv.FormatFloat(math.NaN(), 'f', -1, 64)); ok {
			t.Error("a formatted NaN parsed as a probability")
		}
	})

	t.Run("an unknown never writes a key", func(t *testing.T) {
		for _, shape := range mreqAIShapes() {
			if shape.hangs || shape.stops {
				continue
			}
			t.Run(shape.name, func(t *testing.T) {
				env, ai := mreqAIGuardEnv(t)
				ai.setDefault(shape.step)
				mreqAIWantVerdict(t, shape.name, mreqAIAsk(env, mreqAIUniqueNames(), memberReqLookupMode{}), memberReqUnknown)
				if keys := mreqAIVerdictKeys(t); len(keys) != 0 {
					t.Errorf("verdict keys = %v, want none after %s", keys, shape.name)
				}
			})
		}
	})

	t.Run("a pass and a breach are both cached and the bar applies when the value is read", func(t *testing.T) {
		env, ai := mreqAIGuardEnv(t)
		for _, tc := range []struct {
			planted string
			want    memberReqVerdict
		}{{"0.85", memberReqPass}, {"0.95", memberReqBreach}, {"0.9", memberReqBreach}, {"0", memberReqPass}, {"1", memberReqBreach}} {
			names := mreqAIUniqueNames()
			mreqAIPlant(t, env, mreqAIKeyOf(t, names), tc.planted)
			mreqAIWantVerdict(t, "cached "+tc.planted, mreqAIAsk(env, names, memberReqLookupMode{}), tc.want)
		}
		if n := ai.count(); n != 0 {
			t.Errorf("requests = %d, want none: every value was cached", n)
		}
		for _, answer := range []float64{0.1, 0.97} {
			names := mreqAIUniqueNames()
			ai.setDefault(mreqAIProbability(answer))
			mreqAIAsk(env, names, memberReqLookupMode{})
			if value, _, ok := mreqAIKeyState(t, mreqAIKeyOf(t, names)); !ok || value != strconv.FormatFloat(answer, 'f', -1, 64) {
				t.Errorf("stored value for an answer of %v = %q (found %v)", answer, value, ok)
			}
		}
	})
}
