package modules

import (
	"errors"
	"fmt"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/utils/claude"
)

// The name check's own breaker (D-12, D-26). A failing provider must neither be called for every
// member nor change anything else, so two guards sit in front of the call: a per-group count (five
// provider failures pause that group's name checks) and a provider-wide pause (a rejected key or
// account, or a long rate limit, pauses every group's). Only provider answers count: a cancelled
// call is neither a failure nor a success, and a refusal stop is an answer. No notice is posted
// anywhere; a pause is one log line, the shed counter and a sticky unknown for the executor.
//
// The state belongs to the name check alone, in its own type: the raid image checks have their own
// breaker (raid_ai_breaker.go), so an image failure never pauses a name check and the reverse. All
// times come from raidNow, so a test moves them.

const (
	// memberReqAIFailures is how many provider failures in a row pause one group.
	memberReqAIFailures = 5
	// memberReqAILongRateLimit is the retry delay of a 429 above which every group is paused. The
	// Claude client retries a shorter wait once itself and drops a longer one.
	memberReqAILongRateLimit = 2 * time.Second
	// memberReqAIMaxPause caps a pause taken from the provider's own retry delay.
	memberReqAIMaxPause = 5 * time.Minute
	// memberReqAIBreakerMaxChats bounds the number of groups the breaker tracks.
	memberReqAIBreakerMaxChats = 10000
)

var (
	// memberReqAICooldown is how long a group's name checks stay paused after five failures. A
	// variable only so a test can shorten it.
	memberReqAICooldown = 5 * time.Minute
	// memberReqAIKeyPause is how long every group's name checks stay paused after the key or its
	// account was rejected. A variable only so a test can shorten it.
	memberReqAIKeyPause = 5 * time.Minute
)

// memberReqAIGroup is what the breaker knows about one group.
type memberReqAIGroup struct {
	failures   int
	pausedTill time.Time
}

// paused reports whether the group is paused at now.
func (g *memberReqAIGroup) paused(now time.Time) bool {
	return !g.pausedTill.IsZero() && now.Before(g.pausedTill)
}

// idle reports whether the group is neither failing nor paused, so it can make room for another.
func (g *memberReqAIGroup) idle(now time.Time) bool {
	return g.failures == 0 && !g.paused(now)
}

// memberReqAIBreaker is the state of the breaker: the tracked groups (a bounded map), the
// provider-wide pause and whether the map-full warning was said.
type memberReqAIBreaker struct {
	mu           sync.Mutex
	groups       map[int64]*memberReqAIGroup
	providerTill time.Time
	fullWarned   bool
}

// newMemberReqAIBreaker returns a breaker that tracks nothing.
func newMemberReqAIBreaker() *memberReqAIBreaker {
	return &memberReqAIBreaker{groups: make(map[int64]*memberReqAIGroup)}
}

// memberReqAIBreakers is the process's breaker. A variable only so a test can install an empty one;
// production code never reassigns it.
var memberReqAIBreakers = newMemberReqAIBreaker()

// isPaused reports whether the group's name checks are paused at now, by the provider-wide pause
// or by the group's own count.
func (b *memberReqAIBreaker) isPaused(chatID int64, now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if now.Before(b.providerTill) {
		return true
	}
	group, ok := b.groups[chatID]
	return ok && group.paused(now)
}

// pruneIdle drops the groups that are neither failing nor paused. The caller holds b.mu.
func (b *memberReqAIBreaker) pruneIdle(now time.Time) {
	for chatID, group := range b.groups {
		if group.idle(now) {
			delete(b.groups, chatID)
		}
	}
}

// admit finds the group's state or makes it, keeping the map within its bound: a full map is pruned
// of idle groups first, and when every tracked group is failing or paused the new one stays
// untracked (nil), so it can never be paused. warn is true once per breaker, for the first group
// that could not be tracked. The caller holds b.mu.
func (b *memberReqAIBreaker) admit(chatID int64, now time.Time) (group *memberReqAIGroup, warn bool) {
	if group, ok := b.groups[chatID]; ok {
		return group, false
	}
	if len(b.groups) >= memberReqAIBreakerMaxChats {
		b.pruneIdle(now)
	}
	if len(b.groups) >= memberReqAIBreakerMaxChats {
		warn = !b.fullWarned
		b.fullWarned = true
		return nil, warn
	}
	group = &memberReqAIGroup{}
	b.groups[chatID] = group
	return group, false
}

// memberReqAIOutcome is what counting one failure did to a group.
type memberReqAIOutcome struct {
	tripped bool
	till    time.Time
	warnMap bool
}

// countFailure adds one provider failure to the group. A group that is paused counts nothing more,
// so the checks still in flight when it tripped do not pause it again; a pause that has ended
// starts a new count. Five in a row pause the group for memberReqAICooldown.
func (b *memberReqAIBreaker) countFailure(chatID int64, now time.Time) memberReqAIOutcome {
	b.mu.Lock()
	defer b.mu.Unlock()
	group, warn := b.admit(chatID, now)
	if group == nil {
		return memberReqAIOutcome{warnMap: warn}
	}
	if group.paused(now) {
		return memberReqAIOutcome{}
	}
	if !group.pausedTill.IsZero() {
		*group = memberReqAIGroup{}
	}
	group.failures++
	if group.failures < memberReqAIFailures {
		return memberReqAIOutcome{}
	}
	group.failures = 0
	group.pausedTill = now.Add(memberReqAICooldown)
	return memberReqAIOutcome{tripped: true, till: group.pausedTill}
}

// pauseProvider starts a provider-wide pause of the given span unless one is already running, and
// reports whether it started one and when it ends.
func (b *memberReqAIBreaker) pauseProvider(now time.Time, span time.Duration) (till time.Time, started bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if now.Before(b.providerTill) {
		return b.providerTill, false
	}
	b.providerTill = now.Add(span)
	return b.providerTill, true
}

// clear forgets the group's failures and pause: the provider answered.
func (b *memberReqAIBreaker) clear(chatID int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if group, ok := b.groups[chatID]; ok {
		*group = memberReqAIGroup{}
	}
}

// memberReqAIProviderSpan says whether err calls for a provider-wide pause, for how long and why.
// A rejected key or account pauses for memberReqAIKeyPause; a 429 whose retry delay is over
// memberReqAILongRateLimit pauses for that delay, at most memberReqAIMaxPause. The reason names the
// status and the error type only. span is zero when err calls for none.
func memberReqAIProviderSpan(err error) (span time.Duration, reason string) {
	var httpErr *claude.HTTPError
	if !errors.As(err, &httpErr) {
		return 0, ""
	}
	if httpErr.KeyRejected() {
		return memberReqAIKeyPause, fmt.Sprintf("the key or its account was rejected (HTTP %d %s)", httpErr.Status, httpErr.Type)
	}
	if httpErr.Status == 429 && httpErr.RetryDelay > memberReqAILongRateLimit {
		return min(httpErr.RetryDelay, memberReqAIMaxPause), fmt.Sprintf("rate limited for %s (HTTP 429 %s)", httpErr.RetryDelay.Round(time.Second), httpErr.Type)
	}
	return 0, ""
}

// memberReqAIPaused reports whether the group's name checks are paused now.
func memberReqAIPaused(chatID int64) bool {
	return memberReqAIBreakers.isPaused(chatID, raidNow())
}

// memberReqAIRecordFailure counts one failed provider answer toward the group, which five of them
// pause, and pauses every group when err says so. Each pause logs one warning naming the status
// and the error type (never a name, a hash, the key or a body) and posts nothing.
func memberReqAIRecordFailure(chatID int64, err error) {
	now := raidNow()
	if span, reason := memberReqAIProviderSpan(err); span > 0 {
		if till, started := memberReqAIBreakers.pauseProvider(now, span); started {
			log.Warnf("[MemberReq] AI name checks paused for every group: %s; they resume at %s", reason, till.UTC().Format(time.RFC3339))
		}
	}
	outcome := memberReqAIBreakers.countFailure(chatID, now)
	if outcome.warnMap {
		log.Warnf("[MemberReq] the AI name check breaker map is full (%d groups failing or paused); further groups are not tracked and cannot be paused",
			memberReqAIBreakerMaxChats)
	}
	if outcome.tripped {
		log.WithField("chat_id", chatID).Warnf("[MemberReq] AI name checks paused for this group after %d provider failures, until %s",
			memberReqAIFailures, outcome.till.UTC().Format(time.RFC3339))
	}
}

// memberReqAIRecordSuccess clears the group's failure count and pause: the provider answered (a
// refusal stop is an answer too).
func memberReqAIRecordSuccess(chatID int64) {
	memberReqAIBreakers.clear(chatID)
}
