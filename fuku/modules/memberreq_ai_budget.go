package modules

import (
	"strconv"
	"sync/atomic"

	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"
)

// The hourly budgets of the AI name check (D-10, D-25). A member who renames, or a raid that joins
// with many distinct names, decides how many provider calls are made, so every call reserves one
// unit of three hourly allowances first: the account's (fleet-wide, because a name belongs to the
// account and renaming is the attack), the group's and the fleet's. The families are the name
// check's own (fuku:memberreq:aib:*), separate from the raid image budgets, so a name burst never
// spends the image allowance and the reverse. Only the stateless counter of the image code is
// shared (raidAIBudgetTake, raidAIHourStamp): no key, limit or counter.

// memberReqAIBudgetPrefix is the Redis key family of the hourly budgets. It sits in the
// fuku:memberreq:* family, outside the fuku:cache: prefix, so CLEAR_CACHE_ON_STARTUP keeps it.
// Every key is used in single-key commands only, so no hash tag is needed.
const memberReqAIBudgetPrefix = "fuku:memberreq:aib:"

var (
	// memberReqAIUserHourlyBudget is how many provider calls one account may cost in an hour, in
	// every group together. A package variable only so a test can shrink it.
	memberReqAIUserHourlyBudget int64 = 5
	// memberReqAIChatHourlyBudget is how many provider calls one group may cost in an hour. A
	// package variable only so a test can shrink it.
	memberReqAIChatHourlyBudget int64 = 100
	// memberReqAIGlobalHourlyBudget is how many provider calls the whole fleet may spend on names
	// in an hour. A package variable only so a test can shrink it.
	memberReqAIGlobalHourlyBudget int64 = 400

	// memberReqAITake is the counter the reservation uses: one INCR plus a 2 hour expiry in one
	// transaction. A variable only so a test can make it fail; production code never reassigns it.
	memberReqAITake = raidAIBudgetTake
)

// memberReqAIUserBudgetKey is the account's counter for one UTC hour.
func memberReqAIUserBudgetKey(userID int64, stamp string) string {
	return memberReqAIBudgetPrefix + "u:" + strconv.FormatInt(userID, 10) + ":" + stamp
}

// memberReqAIChatBudgetKey is the group's counter for one UTC hour.
func memberReqAIChatBudgetKey(chatID int64, stamp string) string {
	return memberReqAIBudgetPrefix + "c:" + strconv.FormatInt(chatID, 10) + ":" + stamp
}

// memberReqAIGlobalBudgetKey is the fleet's counter for one UTC hour.
func memberReqAIGlobalBudgetKey(stamp string) string {
	return memberReqAIBudgetPrefix + "g:" + stamp
}

// memberReqAIBudgetResult is the three-way outcome of a reservation: the caller must tell a state
// (an allowance that is used up) from a fault of the moment (Redis did not answer).
type memberReqAIBudgetResult int

const (
	// memberReqAIReserved means all three units were taken and the call may be made.
	memberReqAIReserved memberReqAIBudgetResult = iota
	// memberReqAICapped means an allowance is used up. It is a state: the caller's unknown is sticky.
	memberReqAICapped
	// memberReqAIBudgetFault means a counter could not be read. It is a fault of the moment: the
	// caller's unknown is plain, and the call is never made without a unit.
	memberReqAIBudgetFault
)

// memberReqAIReserve takes the account's unit first, then the group's, then the fleet's, through
// memberReqAITake on the UTC hour of raidNow. The first refusal or error stops the chain, so an
// account over its own budget spends neither shared unit and a group over its own never spends the
// fleet's. A unit is reserved before the call, so a call that fails still spends it (the
// conservative side).
func memberReqAIReserve(client *redis.Client, chatID, userID int64) memberReqAIBudgetResult {
	stamp := raidAIHourStamp(raidNow())
	steps := [...]struct {
		key   string
		limit int64
		scope string
	}{
		{memberReqAIUserBudgetKey(userID, stamp), memberReqAIUserHourlyBudget, "account"},
		{memberReqAIChatBudgetKey(chatID, stamp), memberReqAIChatHourlyBudget, "group"},
		{memberReqAIGlobalBudgetKey(stamp), memberReqAIGlobalHourlyBudget, "fleet"},
	}
	for _, step := range steps {
		allowed, err := memberReqAITake(client, step.key, step.limit)
		if err != nil {
			log.Debugf("[MemberReq] the AI name budget of the %s could not be counted for user %d in chat %d, no verdict", step.scope, userID, chatID)
			return memberReqAIBudgetFault
		}
		if !allowed {
			log.Debugf("[MemberReq] the AI name budget of the %s is used up for user %d in chat %d, no verdict", step.scope, userID, chatID)
			return memberReqAICapped
		}
	}
	return memberReqAIReserved
}

// memberReqAICounters are the in-memory numbers of the name check, for tuning (D-32): they belong
// to this process, start at zero on every restart and are not fleet totals.
type memberReqAICounters struct {
	checked  atomic.Int64
	passed   atomic.Int64
	breached atomic.Int64
	cached   atomic.Int64
	capped   atomic.Int64
	shed     atomic.Int64
	failed   atomic.Int64
	blocked  atomic.Int64
}

// memberReqAIStats is the process's counters.
var memberReqAIStats memberReqAICounters

// memberReqAIStatsSnap is a snapshot of the name check's counters. Checked counts provider answers that
// carried a verdict, Passed and Breached the judgements they made, Cached the verdicts served from
// the cache, Capped the refusals of a used-up budget, Shed every other call that was not made
// (a paused breaker, a full gate, a counter that failed, a cancel), Failed the provider failures
// and Blocked the refusal stops.
type memberReqAIStatsSnap struct {
	Checked, Passed, Breached, Cached, Capped, Shed, Failed, Blocked int64
}

// memberReqAIStatsSnapshot reads the counters.
func memberReqAIStatsSnapshot() memberReqAIStatsSnap {
	return memberReqAIStatsSnap{
		Checked:  memberReqAIStats.checked.Load(),
		Passed:   memberReqAIStats.passed.Load(),
		Breached: memberReqAIStats.breached.Load(),
		Cached:   memberReqAIStats.cached.Load(),
		Capped:   memberReqAIStats.capped.Load(),
		Shed:     memberReqAIStats.shed.Load(),
		Failed:   memberReqAIStats.failed.Load(),
		Blocked:  memberReqAIStats.blocked.Load(),
	}
}
