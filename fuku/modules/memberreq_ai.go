package modules

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/semaphore"

	"github.com/uasneppy/fuku_robot/fuku/config"
	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/utils/claude"
	"github.com/uasneppy/fuku_robot/fuku/utils/namecanon"
)

// The AI spam-name rule of the member requirements: one Claude call per distinct set of
// names, cached by the names, judged only after the executor's exemptions.
//
// The rule's order (D-02, D-21; research S-03), for one member whose names are given:
//
//  1. no names is unknown; an AI that is not configured and names that are not judgeable are
//     inert; no Redis is unknown;
//  2. one pipelined round reads the cached verdict (nv), the member's baseline (nm, the hash of
//     the names the bot last accepted for this account) and the new-member marker (nu, per chat
//     and user); a Redis error is unknown;
//  3. a readable verdict decides first (below the bar records a pass, at or above it sets the
//     marker and is a breach), and it is answered before any provider call;
//  4. a member who is not joining, has no marker and a baseline equal to the current names passes
//     with no write, and one with no baseline at all is recorded with SET NX and passes with no
//     call (an established member's first sighting costs nothing, so switching the rule on in a
//     large group causes no burst of provider calls);
//  5. everyone else (a joiner, a member with a marker, a rename) is judged through one flight, and
//     the caller records the answer: a pass writes nm = h and deletes nu in one step, a breach
//     sets nu, an unknown writes nothing.
//
// The marker exists because "no baseline" is two different members: an established one the rule
// has never seen, and a new joiner whose join judgement was unanswered. The join entry sets the
// marker for every joiner, a breach sets it again, and only a definite pass clears it, so a
// baseline that expired during a long hold can never launder a member whose names were judged
// spam. The baseline and the marker are written by the rule only (the job, the join entry's call,
// the held release, "I fixed it"), never by the watcher. The baseline lives 180 days and the marker
// 30; a member dormant for over 180 days is first-sighted again and baselined with no call (accepted).
//
// Like every lookup row, an answer that is not a probability punishes nobody: the verdict's
// zero value is unknown and only a probability at or above memberReqAIBar is a breach. The
// provider call runs inside the flight, on a context derived from memberReqContext(), so
// StopMemberReqJobs ends it; it reads and writes only Redis and Anthropic. No name, hash,
// probability, instruction text or key is ever logged or put in an error: logs carry chat and
// user IDs, outcome names, a provider status and an error type.
//
// The flight's order (D-24, research S-04), after the cache re-read: the breaker (a paused
// group or provider is a sticky unknown that spends nothing), the in-flight gate (four calls at
// once per replica, a bounded wait; a gate that cannot be had is a sticky unknown and spends no
// unit, because it is taken before the budget), the three hourly budgets (a used-up allowance is a
// sticky unknown, a counter that failed a plain one), the call, the breaker's record and the cache
// write. A sticky unknown is a state of the system, not a fault of the moment: the executor does
// not let it keep a weaker name punishment from standing (D-22).

// memberReqAIBar is the spam probability at or above which the AI rule is a breach (D-09). The
// verdict cache holds the probability itself, not the verdict, so a retuned bar applies to
// cached verdicts at once.
const memberReqAIBar = 0.9

// memberReqAIVerdictPrefix is the Redis key family of the verdict cache. It sits outside the
// fuku:cache: prefix, so CLEAR_CACHE_ON_STARTUP keeps it, and its key is a hash of the names,
// the model and the prompt version, never a name.
const memberReqAIVerdictPrefix = "fuku:memberreq:nv:"

// memberReqAIBaselinePrefix is the Redis key family of the baseline: fuku:memberreq:nm:<user>
// holds the names hash (32 hex characters, never a name) of the names the bot last accepted for
// this account. A name belongs to the account, so the key has no chat. Outside the fuku:cache:
// prefix, so CLEAR_CACHE_ON_STARTUP keeps it.
const memberReqAIBaselinePrefix = "fuku:memberreq:nm:"

// memberReqAIMarkerPrefix is the Redis key family of the new-member marker:
// fuku:memberreq:nu:<chat>:<user> holds the digit 1. A membership belongs to a group, so the key
// has the chat. Outside the fuku:cache: prefix, so CLEAR_CACHE_ON_STARTUP keeps it.
const memberReqAIMarkerPrefix = "fuku:memberreq:nu:"

// memberReqAIStateTTLDefault is how long the marker lives.
const memberReqAIStateTTLDefault = 30 * 24 * time.Hour

// memberReqAIBaselineTTLDefault is how long the baseline lives. The baseline is an absolute TTL
// that nothing refreshes (the watcher is read-only and an equal baseline is a no-write pass), so
// when it lapses the next sighting is a first sighting again and is baselined with no call, whatever
// names the member carries then. 30 days made that hole reopen for every member every month, so
// it lives 180 days: only an account dormant for longer than that is first-sighted again, the same
// D-02 limit as an established member who was in the group when the rule was switched on (WR-02).
const memberReqAIBaselineTTLDefault = 180 * 24 * time.Hour

// memberReqAIVerdictTTLDefault is how long a verdict stays cached (D-11).
const memberReqAIVerdictTTLDefault = 24 * time.Hour

var (
	// memberReqAIVerdictTTL is the life of a cached verdict. A variable so a test can shorten it.
	memberReqAIVerdictTTL = memberReqAIVerdictTTLDefault

	// memberReqAIStateTTL is the life of the marker. A variable so a test can shorten it.
	memberReqAIStateTTL = memberReqAIStateTTLDefault

	// memberReqAIBaselineTTL is the life of the baseline. A variable so a test can shorten it.
	memberReqAIBaselineTTL = memberReqAIBaselineTTLDefault

	// memberReqAIEndpoint is the Anthropic API host of the names request. A variable only so a
	// test points it at an httptest server; production code never reassigns it.
	memberReqAIEndpoint = claude.DefaultEndpoint
	// memberReqAIHTTP is the transport of the names request; nil is the client's own, which has
	// the call timeout and follows no redirect. A variable only for tests.
	memberReqAIHTTP *http.Client

	// memberReqAIGate bounds the provider calls that run at once on this replica (D-24). A variable
	// only so a test can resize it; production code never reassigns it.
	memberReqAIGate = semaphore.NewWeighted(memberReqAIGateSize)
	// memberReqAIGateWait is how long a flight waits for a gate slot before it gives up as a sticky
	// unknown. A variable only so a test can shorten it.
	memberReqAIGateWait = 3 * time.Second

	// memberReqAICall makes the names request. Its default is ClassifyNames; a test replaces it to
	// make a call panic, because a recovered panic cannot be provoked through the HTTP client.
	// Production code never reassigns it.
	memberReqAICall = func(ctx context.Context, client *claude.Client, names claude.Names) (*claude.Verdict, error) {
		return client.ClassifyNames(ctx, names)
	}
)

// memberReqAIGateSize is how many names requests one replica makes at once.
const memberReqAIGateSize = 4

// memberReqAIConfigured reports whether the AI name check can run: the ENABLE_AISPAM kill
// switch is on and an Anthropic key is set. It is its own function, not raidAIImageConfigured,
// because it must never grow a Redis or a raid ai_enabled term: those are other facts (the
// verdict cache needs Redis and the rule answers unknown without it, and the raid switch is the
// lockdown trigger's own), and the "AI not configured" status of the settings page is exactly
// this condition.
func memberReqAIConfigured() bool {
	return config.AppConfig.EnableAISpam && config.AppConfig.AnthropicAPIKey != ""
}

// memberReqAIClient builds the Claude client of one judgement from the configured key and
// model. It returns nil without ANTHROPIC_API_KEY.
func memberReqAIClient() *claude.Client {
	key := config.AppConfig.AnthropicAPIKey
	if key == "" {
		return nil
	}
	return &claude.Client{
		Endpoint: memberReqAIEndpoint,
		APIKey:   key,
		Model:    config.AppConfig.AnthropicModel,
		HTTP:     memberReqAIHTTP,
	}
}

// memberReqAIVerdictKey is the Redis key of a cached verdict: the family prefix and the verdict
// key of namecanon.VerdictKey (a hash of the prompt version, the model and the names hash).
func memberReqAIVerdictKey(model string, canon namecanon.Canon) string {
	return memberReqAIVerdictPrefix + namecanon.VerdictKey(model, canon)
}

// memberReqAIParseProbability is the one place a stored probability is parsed. It accepts a
// finite number from 0 to 1 and treats everything else as a miss. strconv.ParseFloat accepts
// the texts NaN and Inf, and a NaN passes both a below-0 and an above-1 test, so a planted NaN
// would otherwise read as not below the bar: the guard is the same one the client's answer
// parser has.
func memberReqAIParseProbability(text string) (float64, bool) {
	p, err := strconv.ParseFloat(text, 64)
	if err != nil || math.IsNaN(p) || math.IsInf(p, 0) || p < 0 || p > 1 {
		return 0, false
	}
	return p, true
}

// memberReqAINamesHash is the canonical names hash of a member's names, the empty string when the
// names are not judgeable. Two spellings that canonicalise to one string have one hash, so an
// invisible-character change is not a rename, and a change from judgeable to not judgeable is one.
func memberReqAINamesHash(names memberReqNames) string {
	canon, ok := namecanon.Of(names.First, names.Last, names.Username)
	if !ok {
		return ""
	}
	return canon.Hash
}

// memberReqAIVerdictOf judges a probability by the bar: a breach at or above it, else a pass.
func memberReqAIVerdictOf(probability float64) memberReqVerdict {
	if probability >= memberReqAIBar {
		return memberReqBreach
	}
	return memberReqPass
}

// memberReqAIRead reads a cached verdict. hit is true for a stored probability that parses;
// err is set only for a Redis read error (a miss and an unparsable value are not errors), so a
// caller can tell "not cached" from "cannot know".
func memberReqAIRead(ctx context.Context, key string) (probability float64, hit bool, err error) {
	client := memberReqRedisClient()
	if client == nil {
		return 0, false, errors.New("no redis")
	}
	readCtx, cancel := context.WithTimeout(ctx, raidRedisTimeout)
	defer cancel()
	value, err := client.Get(readCtx, key).Result()
	if errors.Is(err, redis.Nil) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	probability, hit = memberReqAIParseProbability(value)
	return probability, hit, nil
}

// memberReqAIBaselineKey is the Redis key of an account's baseline.
func memberReqAIBaselineKey(userID int64) string {
	return memberReqAIBaselinePrefix + strconv.FormatInt(userID, 10)
}

// memberReqAIMarkerKey is the Redis key of a member's new-member marker in one chat.
func memberReqAIMarkerKey(chatID, userID int64) string {
	return memberReqAIMarkerPrefix + strconv.FormatInt(chatID, 10) + ":" + strconv.FormatInt(userID, 10)
}

// memberReqAILive reports whether the AI rule is live for a group: its stored action is not Off
// and the process is configured (an unconfigured stored value counts as Off). The join entry
// sets the marker, and the watcher reads the AI keys, only while it holds, so an unconfigured
// group costs neither a write nor a job per message.
func memberReqAILive(settings *models.ChatRequirements) bool {
	return memberreq.ActionOf(settings, memberreq.RuleAI) != models.RequirementActionOff && memberReqAIConfigured()
}

// memberReqAIState is what one pipelined round read for a member: the cached verdict text ("" for
// none), the baseline ("" for none) and whether the marker exists.
type memberReqAIState struct {
	verdict  string
	baseline string
	marker   bool
}

// memberReqAIReadCmd reads one GET of a pipeline: a missing key is an empty value, any other
// error is returned.
func memberReqAIReadCmd(cmd *redis.StringCmd) (value string, found bool, err error) {
	value, err = cmd.Result()
	if errors.Is(err, redis.Nil) {
		return "", false, nil
	}
	return value, err == nil, err
}

// memberReqAIReadState reads the verdict, the baseline and the marker of one member in one
// round trip under raidRedisTimeout. A Redis error, no Redis and a failed read of any of the
// three are an error: the caller answers unknown and nothing is written or asked.
func memberReqAIReadState(ctx context.Context, verdictKey string, chatID, userID int64) (memberReqAIState, error) {
	var state memberReqAIState
	client := memberReqRedisClient()
	if client == nil {
		return state, errors.New("no redis")
	}
	readCtx, cancel := context.WithTimeout(ctx, raidRedisTimeout)
	defer cancel()
	pipe := client.Pipeline()
	verdictCmd := pipe.Get(readCtx, verdictKey)
	baselineCmd := pipe.Get(readCtx, memberReqAIBaselineKey(userID))
	markerCmd := pipe.Get(readCtx, memberReqAIMarkerKey(chatID, userID))
	// Exec reports the first command's error, a miss included, so every command is read on its own.
	if _, err := pipe.Exec(readCtx); err != nil && !errors.Is(err, redis.Nil) {
		return state, err
	}
	var err error
	if state.verdict, _, err = memberReqAIReadCmd(verdictCmd); err != nil {
		return state, err
	}
	if state.baseline, _, err = memberReqAIReadCmd(baselineCmd); err != nil {
		return state, err
	}
	if _, state.marker, err = memberReqAIReadCmd(markerCmd); err != nil {
		return state, err
	}
	return state, nil
}

// memberReqAIPlan is what memberReqAIRule works out before any provider call: a final verdict
// with nothing to record (done), a verdict read from the cache that the rule records (cached), an
// established member's first sighting that only needs its baseline written (baselineOnly), or
// the canonical names and the cache key to judge through the flight.
type memberReqAIPlan struct {
	canon        namecanon.Canon
	key          string
	verdict      memberReqVerdict
	done         bool
	cached       bool
	baselineOnly bool
}

// memberReqAIFinal is a plan that is already decided, with nothing to record.
func memberReqAIFinal(verdict memberReqVerdict) memberReqAIPlan {
	return memberReqAIPlan{verdict: verdict, done: true}
}

// memberReqAIPrepare is the read step of the rule: no names is unknown, an AI that is not
// configured and names that are not judgeable are inert, no Redis or a Redis read error is
// unknown (the cost guard: with the cache unreadable the provider is not asked), and the state
// is read in one round. mode.Fresh and mode.Classify are ignored: the AI row answers from the
// cache whatever the mode (D-23). mode.Joining and mode.IgnoreBaseline both skip the baseline step.
func memberReqAIPrepare(ctx context.Context, chatID, userID int64, mode memberReqLookupMode) memberReqAIPlan {
	if mode.Names == nil {
		return memberReqAIFinal(memberReqUnknown)
	}
	if !memberReqAIConfigured() {
		return memberReqAIFinal(memberReqInert)
	}
	canon, ok := namecanon.Of(mode.Names.First, mode.Names.Last, mode.Names.Username)
	if !ok {
		return memberReqAIFinal(memberReqInert)
	}
	key := memberReqAIVerdictKey(config.AppConfig.AnthropicModel, canon)
	state, err := memberReqAIReadState(ctx, key, chatID, userID)
	if err != nil {
		return memberReqAIFinal(memberReqUnknown)
	}
	return memberReqAIDecide(canon, key, state, mode.Joining || mode.IgnoreBaseline)
}

// memberReqAIDecide is the decide step, on what the round read. A readable verdict decides
// first. Otherwise a member who is not joining and has no marker passes with no call: with a
// baseline equal to the current names hash nothing is written, with none the baseline is
// recorded. A joiner, a member with a marker and a baseline that differs (a rename) are judged,
// and so is a caller that ignores the baseline (skipBaseline: a joiner, or a held member or a
// button press, for whom a baseline-only pass must never stand in for a judgement, WR-01).
func memberReqAIDecide(canon namecanon.Canon, key string, state memberReqAIState, skipBaseline bool) memberReqAIPlan {
	if probability, hit := memberReqAIParseProbability(state.verdict); hit {
		return memberReqAIPlan{canon: canon, key: key, verdict: memberReqAIVerdictOf(probability), cached: true}
	}
	if !skipBaseline && !state.marker {
		switch state.baseline {
		case canon.Hash:
			return memberReqAIFinal(memberReqPass)
		case "":
			return memberReqAIPlan{canon: canon, key: key, verdict: memberReqPass, baselineOnly: true}
		}
	}
	return memberReqAIPlan{canon: canon, key: key}
}

// memberReqAIRule judges whether a member's names look like a spam account (the AI row of the
// rule table). After memberReqAIPrepare and memberReqAIDecide it either answers from the cache,
// records an established member's baseline and passes, or makes one provider call inside one
// flight per verdict key (n:<hash>, so every concurrent judgement of one string is one call), on
// a context derived from memberReqContext(). A caller whose own context ends first gets unknown.
// The flight does only the call and the verdict write: each caller records its own baseline and
// marker from the answer, because callers with different user IDs share the flight (D-24).
func memberReqAIRule(ctx context.Context, _ *gotgbot.Bot, chatID, userID int64, mode memberReqLookupMode) memberReqVerdict {
	plan := memberReqAIPrepare(ctx, chatID, userID, mode)
	if plan.done {
		return plan.verdict
	}
	if plan.baselineOnly {
		memberReqAIBaseline(ctx, userID, plan.canon.Hash)
		return plan.verdict
	}
	verdict := plan.verdict
	if plan.cached {
		memberReqAIStats.cached.Add(1)
	} else {
		answer, _ := memberReqFlightValue(ctx, "n:"+plan.key, func(flightCtx context.Context) memberReqAIAnswer {
			return memberReqAIJudge(flightCtx, chatID, userID, plan)
		})
		verdict = answer.Verdict
		if answer.Sticky {
			memberReqNoteSticky(ctx)
		}
	}
	memberReqAIRecord(ctx, chatID, userID, plan.canon.Hash, verdict)
	return verdict
}

// memberReqAIRecord records one definite answer for the member: a pass is the baseline and the end
// of the marker, a breach is the marker, anything else writes nothing.
func memberReqAIRecord(ctx context.Context, chatID, userID int64, hash string, verdict memberReqVerdict) {
	switch verdict {
	case memberReqPass:
		memberReqAIRecordPass(ctx, chatID, userID, hash)
	case memberReqBreach:
		memberReqAIMark(ctx, chatID, userID)
	}
}

// memberReqAIRecordPass writes nm = h and deletes the marker in one transaction, so a baseline
// never exists beside a marker that says the member still has to be judged. Best effort: an error
// is logged by IDs only, and a member whose pass was not recorded is judged again, from the
// cache, at their next message.
func memberReqAIRecordPass(ctx context.Context, chatID, userID int64, hash string) {
	client := memberReqRedisClient()
	if client == nil {
		return
	}
	writeCtx, cancel := context.WithTimeout(ctx, raidRedisTimeout)
	defer cancel()
	pipe := client.TxPipeline()
	pipe.Set(writeCtx, memberReqAIBaselineKey(userID), hash, memberReqAIBaselineTTL)
	pipe.Del(writeCtx, memberReqAIMarkerKey(chatID, userID))
	if _, err := pipe.Exec(writeCtx); err != nil {
		log.Debugf("[MemberReq] the AI baseline of user %d in chat %d was not recorded: %s", userID, chatID, memberReqErrText(err))
	}
}

// memberReqAIMark sets the new-member marker. It is the one write of a breach: while it stands the
// member is judged again whatever the baseline says, so a baseline that expired during a long hold
// can never launder them. A failed write is logged by IDs only.
func memberReqAIMark(ctx context.Context, chatID, userID int64) {
	client := memberReqRedisClient()
	if client == nil {
		return
	}
	writeCtx, cancel := context.WithTimeout(ctx, raidRedisTimeout)
	defer cancel()
	if err := client.Set(writeCtx, memberReqAIMarkerKey(chatID, userID), "1", memberReqAIStateTTL).Err(); err != nil {
		log.Debugf("[MemberReq] the AI marker of user %d in chat %d was not written: %s", userID, chatID, memberReqErrText(err))
	}
}

// memberReqAIMarkJoin records that the bot saw the member join, so they are judged, at join or at
// their first message, and never baselined. The join entry calls it while the rule is live
// (memberReqAILive), whatever the raid settings and Check on join say. It is a no-op without Redis
// and logs an error by IDs only: a member with no marker is baselined and unjudged until they
// rename (the safe direction).
func memberReqAIMarkJoin(ctx context.Context, chatID, userID int64) {
	memberReqAIMark(ctx, chatID, userID)
}

// memberReqAIBaseline records the baseline of an established member's first sighting with SET NX,
// so two replicas that see the member at once write one value and nobody's later pass is
// overwritten by it. Best effort, logged by IDs only.
func memberReqAIBaseline(ctx context.Context, userID int64, hash string) {
	client := memberReqRedisClient()
	if client == nil {
		return
	}
	writeCtx, cancel := context.WithTimeout(ctx, raidRedisTimeout)
	defer cancel()
	if err := client.SetNX(writeCtx, memberReqAIBaselineKey(userID), hash, memberReqAIBaselineTTL).Err(); err != nil {
		log.Debugf("[MemberReq] the AI baseline of user %d was not recorded: %s", userID, memberReqErrText(err))
	}
}

// memberReqAIAnswer is the value of a flight: the verdict and whether an unknown verdict is sticky,
// a state of the system (a paused breaker, a used-up budget, a gate that could not be had) and not a
// fault of the moment. The zero value is an unknown that is not sticky.
type memberReqAIAnswer struct {
	Verdict memberReqVerdict
	Sticky  bool
}

// memberReqAIStickyUnknown is an unknown that the executor does not let defer a weaker punishment.
var memberReqAIStickyUnknown = memberReqAIAnswer{Verdict: memberReqUnknown, Sticky: true}

// memberReqAIJudge is the flight body. It reads the cache once more (a caller whose read missed
// a moment before the previous flight wrote its verdict starts a new flight and must not make a
// second call), then passes the guards in order (the breaker, the gate, the budgets), asks the model for the
// canonical strings and, on a verdict, stores the probability before it answers pass or breach by
// the bar. Every failure is unknown and caches nothing, and the gate slot is released by defer on
// every path, a panic and a cancel included.
func memberReqAIJudge(ctx context.Context, chatID, userID int64, plan memberReqAIPlan) memberReqAIAnswer {
	probability, hit, err := memberReqAIRead(ctx, plan.key)
	if err != nil {
		return memberReqAIAnswer{}
	}
	if hit {
		memberReqAIStats.cached.Add(1)
		return memberReqAIAnswer{Verdict: memberReqAIVerdictOf(probability)}
	}
	client := memberReqAIClient()
	if client == nil {
		return memberReqAIAnswer{}
	}
	if refusal, ok := memberReqAIAcquire(ctx, chatID); !ok {
		return refusal
	}
	defer memberReqAIGate.Release(1)
	if refusal, ok := memberReqAIReserveUnit(chatID, userID); !ok {
		return refusal
	}
	names := claude.Names{First: plan.canon.First, Last: plan.canon.Last, Username: plan.canon.Username}
	answer, err := memberReqAICall(ctx, client, names)
	if err != nil {
		memberReqAIAfterFailure(ctx, chatID, userID, client.APIKey, err)
		return memberReqAIAnswer{}
	}
	memberReqAIRecordSuccess(chatID)
	memberReqAIStats.checked.Add(1)
	memberReqCacheSetValue(ctx, plan.key, strconv.FormatFloat(answer.SpamProbability, 'f', -1, 64), memberReqAIVerdictTTL)
	verdict := memberReqAIVerdictOf(answer.SpamProbability)
	if verdict == memberReqBreach {
		memberReqAIStats.breached.Add(1)
	} else {
		memberReqAIStats.passed.Add(1)
	}
	return memberReqAIAnswer{Verdict: verdict}
}

// memberReqAIAcquire checks the breaker and takes a gate slot, waiting at most memberReqAIGateWait.
// Both come before the budget, so a paused check and a flight that cannot get a slot spend no unit.
// When ok is false refusal is the answer: sticky for a paused group or provider and when the wait
// ran out while the flight's context is live (states: the provider is failing, the replica is
// saturated), plain when the context itself ended (shutdown is not a state). On success the caller
// owns the slot and releases it by defer, so a panic and a cancel release it too.
func memberReqAIAcquire(ctx context.Context, chatID int64) (refusal memberReqAIAnswer, ok bool) {
	if memberReqAIPaused(chatID) {
		memberReqAIStats.shed.Add(1)
		return memberReqAIStickyUnknown, false
	}
	gateCtx, cancel := context.WithTimeout(ctx, memberReqAIGateWait)
	defer cancel()
	if err := memberReqAIGate.Acquire(gateCtx, 1); err != nil {
		memberReqAIStats.shed.Add(1)
		if ctx.Err() != nil {
			return memberReqAIAnswer{}, false
		}
		return memberReqAIStickyUnknown, false
	}
	return memberReqAIAnswer{}, true
}

// memberReqAIReserveUnit reserves the call's three budget units. A used-up allowance is a sticky
// unknown (a state); a counter that failed, or no Redis, is a plain one (a fault of the moment).
// The call is never made without a unit.
func memberReqAIReserveUnit(chatID, userID int64) (refusal memberReqAIAnswer, ok bool) {
	client := memberReqRedisClient()
	if client == nil {
		memberReqAIStats.shed.Add(1)
		return memberReqAIAnswer{}, false
	}
	switch memberReqAIReserve(client, chatID, userID) {
	case memberReqAICapped:
		memberReqAIStats.capped.Add(1)
		return memberReqAIStickyUnknown, false
	case memberReqAIBudgetFault:
		memberReqAIStats.shed.Add(1)
		return memberReqAIAnswer{}, false
	}
	return memberReqAIAnswer{}, true
}

// memberReqAIAfterFailure settles a names request that gave no verdict. A cancelled flight (the
// shutdown, or the flight's own timeout) is neither a failure nor a success of the provider; a
// refusal stop is the provider's answer (a success for the breaker); anything else is a failure.
// The client and its key are only used to scrub the log line.
func memberReqAIAfterFailure(ctx context.Context, chatID, userID int64, key string, err error) {
	switch {
	case ctx.Err() != nil:
		memberReqAIStats.shed.Add(1)
	case errors.Is(err, claude.ErrBlocked):
		memberReqAIRecordSuccess(chatID)
		memberReqAIStats.blocked.Add(1)
	default:
		memberReqAIRecordFailure(chatID, err)
		memberReqAIStats.failed.Add(1)
	}
	memberReqAILogFailure(ctx, chatID, userID, key, err)
}

// memberReqAILogFailure logs a names request that gave no verdict by the chat and user IDs and
// the provider's status and error type only. A cancelled flight (the shutdown, or the flight's
// own timeout) logs nothing; a refusal stop is the provider's answer, not an outage, and is an
// Info line; any other failure is a Warn whose text passes raidAIScrub with the key. Never a
// name, a hash, a probability or the request.
func memberReqAILogFailure(ctx context.Context, chatID, userID int64, key string, err error) {
	if ctx.Err() != nil {
		return
	}
	fields := log.Fields{"chat_id": chatID, "user_id": userID}
	if errors.Is(err, claude.ErrBlocked) {
		log.WithFields(fields).Info("[MemberReq] the AI name check was refused by the provider, so nobody is punished")
		return
	}
	entry := log.WithFields(fields)
	var httpErr *claude.HTTPError
	if errors.As(err, &httpErr) {
		entry = entry.WithField("status", httpErr.Status).WithField("error_type", httpErr.Type)
	}
	entry.Warnf("[MemberReq] the AI name check failed, so nobody is punished: %s", raidAIScrub(err.Error(), key))
}
