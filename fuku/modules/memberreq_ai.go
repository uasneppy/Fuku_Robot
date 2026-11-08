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

	"github.com/uasneppy/fuku_robot/fuku/config"
	"github.com/uasneppy/fuku_robot/fuku/utils/claude"
	"github.com/uasneppy/fuku_robot/fuku/utils/namecanon"
)

// The AI spam-name rule of the member requirements: one Claude call per distinct set of
// names, cached by the names, judged only after the executor's exemptions.
//
// This file is the MINIMAL rule of plan 15-04, the tracer of the phase, and it says so on
// purpose so that a reader of this commit is not misled. In order the rule returns unknown
// without names, inert while the AI is not configured, inert for names that are not
// judgeable, unknown without Redis or on a Redis read error, answers from the verdict cache
// and otherwise judges through one flight. There is no baseline of an established member's
// names and no new-member marker (plan 15-05 adds both, so in this commit a message job of an
// established member also judges the member), no names-bound gate, hourly budget or breaker
// (plan 15-06), no remedy sentence (15-07) and no page status (15-08).
//
// Like every lookup row, an answer that is not a probability punishes nobody: the verdict's
// zero value is unknown and only a probability at or above memberReqAIBar is a breach. The
// provider call runs inside the flight, on a context derived from memberReqContext(), so
// StopMemberReqJobs ends it; it reads and writes only Redis and Anthropic. No name, hash,
// probability, instruction text or key is ever logged or put in an error: logs carry chat and
// user IDs, outcome names, a provider status and an error type.

// memberReqAIBar is the spam probability at or above which the AI rule is a breach (D-09). The
// verdict cache holds the probability itself, not the verdict, so a retuned bar applies to
// cached verdicts at once.
const memberReqAIBar = 0.9

// memberReqAIVerdictPrefix is the Redis key family of the verdict cache. It sits outside the
// fuku:cache: prefix, so CLEAR_CACHE_ON_STARTUP keeps it, and its key is a hash of the names,
// the model and the prompt version, never a name.
const memberReqAIVerdictPrefix = "fuku:memberreq:nv:"

// memberReqAIVerdictTTLDefault is how long a verdict stays cached (D-11).
const memberReqAIVerdictTTLDefault = 24 * time.Hour

var (
	// memberReqAIVerdictTTL is the life of a cached verdict. A variable so a test can shorten it.
	memberReqAIVerdictTTL = memberReqAIVerdictTTLDefault

	// memberReqAIEndpoint is the Anthropic API host of the names request. A variable only so a
	// test points it at an httptest server; production code never reassigns it.
	memberReqAIEndpoint = claude.DefaultEndpoint
	// memberReqAIHTTP is the transport of the names request; nil is the client's own, which has
	// the call timeout and follows no redirect. A variable only for tests.
	memberReqAIHTTP *http.Client
)

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

// memberReqAIPlan is what memberReqAIRule works out before any provider call: either a final
// verdict (done) or the canonical names and the cache key to judge.
type memberReqAIPlan struct {
	canon   namecanon.Canon
	key     string
	verdict memberReqVerdict
	done    bool
}

// memberReqAIFinal is a plan that is already decided.
func memberReqAIFinal(verdict memberReqVerdict) memberReqAIPlan {
	return memberReqAIPlan{verdict: verdict, done: true}
}

// memberReqAIPrepare is the read step of the rule, in order: no names is unknown, an AI that is
// not configured and names that are not judgeable are inert, no Redis or a Redis read error is
// unknown (the cost guard: with the cache unreadable the provider is not asked), and a cached
// probability is the answer. mode.Fresh and mode.Classify are ignored: the AI row answers from
// the cache whatever the mode (D-23).
func memberReqAIPrepare(ctx context.Context, mode memberReqLookupMode) memberReqAIPlan {
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
	probability, hit, err := memberReqAIRead(ctx, key)
	switch {
	case err != nil:
		return memberReqAIFinal(memberReqUnknown)
	case hit:
		return memberReqAIFinal(memberReqAIVerdictOf(probability))
	}
	return memberReqAIPlan{canon: canon, key: key}
}

// memberReqAIRule judges whether a member's names look like a spam account (the AI row of the
// rule table). It is the decide step after memberReqAIPrepare: one provider call inside one
// flight per verdict key (n:<hash>, so every concurrent judgement of one string is one call),
// on a context derived from memberReqContext(). A caller whose own context ends first gets
// unknown.
func memberReqAIRule(ctx context.Context, _ *gotgbot.Bot, chatID, userID int64, mode memberReqLookupMode) memberReqVerdict {
	plan := memberReqAIPrepare(ctx, mode)
	if plan.done {
		return plan.verdict
	}
	return memberReqFlight(ctx, "n:"+plan.key, func(flightCtx context.Context) memberReqVerdict {
		return memberReqAIJudge(flightCtx, chatID, userID, plan)
	})
}

// memberReqAIJudge is the flight body. It reads the cache once more (a caller whose read missed
// a moment before the previous flight wrote its verdict starts a new flight and must not make a
// second call), builds the client, asks the model for the canonical strings and, on a verdict,
// stores the probability before it answers pass or breach by the bar. Every failure is unknown
// and caches nothing.
func memberReqAIJudge(ctx context.Context, chatID, userID int64, plan memberReqAIPlan) memberReqVerdict {
	probability, hit, err := memberReqAIRead(ctx, plan.key)
	if err != nil {
		return memberReqUnknown
	}
	if hit {
		return memberReqAIVerdictOf(probability)
	}
	client := memberReqAIClient()
	if client == nil {
		return memberReqUnknown
	}
	answer, err := client.ClassifyNames(ctx, claude.Names{First: plan.canon.First, Last: plan.canon.Last, Username: plan.canon.Username})
	if err != nil {
		memberReqAILogFailure(ctx, chatID, userID, client.APIKey, err)
		return memberReqUnknown
	}
	memberReqCacheSetValue(ctx, plan.key, strconv.FormatFloat(answer.SpamProbability, 'f', -1, 64), memberReqAIVerdictTTL)
	return memberReqAIVerdictOf(answer.SpamProbability)
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
