package modules

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/config"
	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
	"github.com/uasneppy/fuku_robot/fuku/utils/error_handling"
	"github.com/uasneppy/fuku_robot/fuku/utils/namecanon"
	"github.com/uasneppy/fuku_robot/fuku/utils/tracing"
)

// The message watcher of the member requirements (D-12, D-19, D-20). It sits at handler group
// -2, after the raid flood counter (-4) so raid detection still counts every message, and
// before the users tracker (-1) and the commands (0), so a violator's command is screened and
// still served. It is the only message handler of that group, and it returns
// ext.ContinueGroups on every path: the dispatcher reads a nil as "leave this group", which
// would silently kill every later group-0 handler, and ext.EndGroups would end the update for
// the commands, aispam, antiflood, locks and blacklists.

// memberReqWatchGroup is the handler group of the message watcher. No other message handler
// may be added to it.
const memberReqWatchGroup = -2

// memberReqJobTimeout bounds one enforcement job. It is 30 s shorter than
// memberreq.StaleClaim (2 minutes), so a job always ends before another replica's reclaim of
// the hold it claimed can start, and two jobs never act on one hold (research S-07).
const memberReqJobTimeout = 90 * time.Second

const (
	// memberReqPassTTL is how long a clean verdict is remembered per member.
	memberReqPassTTL = 30 * time.Minute

	// memberReqPassMaxAge is how old a message may be and still write a clean verdict. A
	// message processed long after it was sent (a backlog after downtime) must not hide a
	// hold that is created later from the clean predicate for the whole TTL.
	memberReqPassMaxAge = 2 * time.Minute

	// memberReqCleanTimeout bounds the clean predicate's one database statement.
	memberReqCleanTimeout = 3 * time.Second
)

// memberReqMaxInFlight is the most enforcement jobs that run at once on this replica. Over it
// a job is shed with one log line and the member's next message is checked again. A variable
// so a test can make it small.
var memberReqMaxInFlight = 32

// memberReqRedisClient is the operational Redis client of the pass cache, nil without Redis.
// A variable so a test can take Redis away.
var memberReqRedisClient = raidRedis

// memberReqJobs counts the enforcement jobs in flight. It is a counter and not a
// sync.WaitGroup because the shutdown drain gives up after its wait while a new job can still
// start.
var memberReqJobs raidPostTracker

// memberReqDraining is set before the shutdown drain, so a late message starts no job.
var memberReqDraining atomic.Bool

// memberReqRoot holds the context every job derives from. StopMemberReqJobs cancels it, which
// aborts the paced Telegram calls of every job in flight at once.
var memberReqRoot struct {
	mu     sync.Mutex
	ctx    context.Context
	cancel context.CancelFunc
}

// memberReqContext returns the root context of the jobs, creating it on first use.
func memberReqContext() context.Context {
	memberReqRoot.mu.Lock()
	defer memberReqRoot.mu.Unlock()
	if memberReqRoot.ctx == nil {
		memberReqRoot.ctx, memberReqRoot.cancel = context.WithCancel(context.Background())
	}
	return memberReqRoot.ctx
}

// startBelow counts one more job when fewer than limit are running and reports whether it
// did. Check and increment are one step, so the cap holds under concurrent starts.
func (t *raidPostTracker) startBelow(limit int) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.running >= limit {
		return false
	}
	if t.running == 0 {
		t.idle = make(chan struct{})
	}
	t.running++
	return true
}

// memberReqPassKey is the Redis key of a member's clean verdict in a chat. It is outside the
// fuku:cache: prefix (so CLEAR_CACHE_ON_STARTUP keeps it) and holds no name: the value is a
// fingerprint of the rule settings and the names.
func memberReqPassKey(chatID, userID int64) string {
	return "fuku:memberreq:pass:" + strconv.FormatInt(chatID, 10) + ":" + strconv.FormatInt(userID, 10)
}

// memberReqPassHit reports whether the member's stored clean verdict equals value. A Redis
// error, no Redis and a missing key are all a miss: the clean predicate then runs, which is
// correct and only slower.
func memberReqPassHit(chatID, userID int64, value string) bool {
	client := memberReqRedisClient()
	if client == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(cache.Context, raidRedisTimeout)
	defer cancel()
	stored, err := client.Get(ctx, memberReqPassKey(chatID, userID)).Result()
	return err == nil && stored == value
}

// memberReqClean runs the clean predicate for a member whose names break no rule: it ends the
// hold of an earlier episode that the member has since fixed, so a later break is a new
// episode (D-07). Only after the predicate succeeded, and only for a message that is not
// stale, is the clean verdict stored, so the statement runs once per member per name change,
// settings change or TTL instead of once per message. A violation never reaches this.
func memberReqClean(ctx context.Context, chatID, userID int64, msgDate time.Time, value string) {
	cleanCtx, cancel := context.WithTimeout(ctx, memberReqCleanTimeout)
	defer cancel()
	if _, err := memberreq.ClearHoldOnClean(cleanCtx, chatID, userID, msgDate); err != nil {
		log.Warnf("[MemberReq] the clean check of user %d in chat %d failed: %s", userID, chatID, memberReqErrText(err))
		return
	}
	if time.Since(msgDate) > memberReqPassMaxAge {
		return
	}
	client := memberReqRedisClient()
	if client == nil {
		return
	}
	setCtx, setCancel := context.WithTimeout(cache.Context, raidRedisTimeout)
	defer setCancel()
	if err := client.Set(setCtx, memberReqPassKey(chatID, userID), value, memberReqPassTTL).Err(); err != nil {
		log.Debugf("[MemberReq] the clean verdict of user %d in chat %d was not stored: %s", userID, chatID, memberReqErrText(err))
	}
}

// memberReqCleanMember is the watcher's path for a member whose names break no rule.
func memberReqCleanMember(ctx context.Context, chatID, userID, msgDate int64, settings *models.ChatRequirements, names memberReqNames) {
	value := memberReqPassValue(settings, names)
	if memberReqPassHit(chatID, userID, value) {
		return
	}
	memberReqClean(ctx, chatID, userID, time.Unix(msgDate, 0), value)
}

// memberReqLookupRules says which lookup rules the watcher has to read for a group: the photo and
// channel rules when their action is not Off, and the AI rule only while it is live (its action
// is not Off and the process is configured: an unconfigured stored value counts as Off here, or
// every message of every member would start a job for ever).
type memberReqLookupRules struct {
	Photo, Channel, AI bool
}

// any reports whether at least one lookup rule is on.
func (r memberReqLookupRules) any() bool {
	return r.Photo || r.Channel || r.AI
}

// memberReqLookupRulesOf reads the lookup rules that are on for settings.
func memberReqLookupRulesOf(settings *models.ChatRequirements) memberReqLookupRules {
	return memberReqLookupRules{
		Photo:   memberreq.ActionOf(settings, memberreq.RulePhoto) != models.RequirementActionOff,
		Channel: memberreq.ActionOf(settings, memberreq.RuleChannel) != models.RequirementActionOff,
		AI:      memberReqAILive(settings),
	}
}

// memberReqAIWatchRead is the three commands the watcher queued for the AI rule on its pipeline
// and the names hash the decision compares with. A zero value (no commands) is the AI rule not
// being read, or a member whose names cannot be judged: it is satisfied.
type memberReqAIWatchRead struct {
	verdict, baseline, marker *redis.StringCmd
	hash                      string
}

// memberReqAIWatchQueue adds the AI rule's three single-key GETs to the watcher's pipeline:
// fuku:memberreq:nv:<verdict key hash>, fuku:memberreq:nm:<user> and fuku:memberreq:nu:<chat>:<user>.
// The canonical names and the verdict key are computed by pure Go (namecanon, with the configured
// model). For names the rule never judges nothing is queued and the zero value is returned,
// which is satisfied with no key read. It queues reads only; it writes nothing and calls nobody.
func memberReqAIWatchQueue(ctx context.Context, pipe redis.Pipeliner, chatID, userID int64, names memberReqNames) memberReqAIWatchRead {
	canon, ok := namecanon.Of(names.First, names.Last, names.Username)
	if !ok {
		return memberReqAIWatchRead{}
	}
	return memberReqAIWatchRead{
		verdict:  pipe.Get(ctx, memberReqAIVerdictKey(config.AppConfig.AnthropicModel, canon)),
		baseline: pipe.Get(ctx, memberReqAIBaselineKey(userID)),
		marker:   pipe.Get(ctx, memberReqAIMarkerKey(chatID, userID)),
		hash:     canon.Hash,
	}
}

// satisfied reads the queued commands after the pipeline ran: nothing queued is satisfied, any
// error other than a miss is not (the caller hands the member to a job), otherwise
// memberReqAIWatchSatisfied decides.
func (r memberReqAIWatchRead) satisfied() bool {
	if r.verdict == nil {
		return true
	}
	verdict, _, err := memberReqAIReadCmd(r.verdict)
	if err != nil {
		return false
	}
	baseline, _, err := memberReqAIReadCmd(r.baseline)
	if err != nil {
		return false
	}
	_, marker, err := memberReqAIReadCmd(r.marker)
	if err != nil {
		return false
	}
	return memberReqAIWatchSatisfied(verdict, baseline, marker, r.hash)
}

// memberReqAIWatchSatisfied is the watcher's decision for the AI rule (D-21), pure: no Redis and
// no clock. The member needs no job when the cached verdict of their names parses and is below
// the bar, or when they have no marker and their baseline equals the current names hash. A miss
// (empty value), a text that does not parse, NaN, an infinity and a value out of range are not a
// verdict. The baseline half is the rule, not a gap (SEC-01): a member whose baseline equals their
// names and who has no marker stays satisfied even when another account's judgement of the same
// string has since cached a breach, because the baseline is the bot's record that these names were
// accepted for this member (it errs toward not punishing); the job answers the same way, because
// memberReqAIDecide puts the baseline step before the verdict cache.
func memberReqAIWatchSatisfied(verdictValue, baseline string, marker bool, hash string) bool {
	if probability, ok := memberReqAIParseProbability(verdictValue); ok && probability < memberReqAIBar {
		return true
	}
	return !marker && baseline != "" && baseline == hash
}

// memberReqLookupCached is the watcher's whole read for a member whose names are clean in a
// group with a lookup rule on. It never calls Telegram or the provider: one pipelined round of
// single-key GETs reads the pass key, fuku:memberreq:photo:<user> when the photo rule is on,
// fuku:memberreq:lc:<chat> when the channel rule is on, and the AI rule's verdict, baseline and
// marker keys while the AI rule is live (memberReqAIWatchQueue), and a second read of
// fuku:memberreq:sub:<channel>:<user> follows only when lc holds a channel ID, and
// fuku:memberreq:cna:<channel> after it when the subscription is missing. satisfied is true
// only when every lookup rule that is on is satisfied: the photo key exists; the channel is
// either known to be none (lc reads 0, nothing to verify), or the subscription key exists, or
// the channel-level not-administrator fact exists (the rule is inert there); and the AI rule's
// names are not judgeable or pass memberReqAIWatchSatisfied. passHit reports whether the pass key
// held passValue, read in the same round. A missing client, any Redis error, a miss, an unreadable
// lc and a positive lc (never written) are all "not satisfied": the caller then hands the member
// to a job, which is correct and only slower. It writes nothing, and the pass hit never stands in
// for a lookup positive (R2).
func memberReqLookupCached(settings *models.ChatRequirements, chatID, userID int64, passValue string, names memberReqNames) (satisfied, passHit bool) {
	return memberReqLookupCachedWith(settings, chatID, userID, passValue, names, false)
}

// memberReqLookupCachedWith is memberReqLookupCached with the AI rule optionally left out (ignoreAI:
// its keys are not queued and its part counts as satisfied), so the watcher can ask whether the AI
// rule is the only reason a member is not satisfied (memberReqAIOnlyPaused, WR-04).
func memberReqLookupCachedWith(settings *models.ChatRequirements, chatID, userID int64, passValue string, names memberReqNames, ignoreAI bool) (satisfied, passHit bool) {
	client := memberReqRedisClient()
	if client == nil {
		return false, false
	}
	rules := memberReqLookupRulesOf(settings)
	if ignoreAI {
		rules.AI = false
	}
	ctx, cancel := context.WithTimeout(cache.Context, raidRedisTimeout)
	defer cancel()
	pipe := client.Pipeline()
	passCmd := pipe.Get(ctx, memberReqPassKey(chatID, userID))
	var photoCmd, linkedCmd *redis.StringCmd
	if rules.Photo {
		photoCmd = pipe.Get(ctx, memberReqPhotoKey(userID))
	}
	if rules.Channel {
		linkedCmd = pipe.Get(ctx, memberReqLinkedKey(chatID))
	}
	var aiRead memberReqAIWatchRead
	if rules.AI {
		aiRead = memberReqAIWatchQueue(ctx, pipe, chatID, userID, names)
	}
	// Exec reports the first command's error, a miss included, so every command is read on its own.
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return false, false
	}
	stored, err := passCmd.Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return false, false
	}
	passHit = err == nil && stored == passValue
	if !aiRead.satisfied() {
		return false, passHit
	}
	if rules.Photo {
		if _, err := photoCmd.Result(); err != nil {
			return false, passHit
		}
	}
	if rules.Channel {
		channel, ok := memberReqCachedChannel(linkedCmd)
		if !ok {
			return false, passHit
		}
		// A cached subscription is a positive of this member; a cached "the bot is not an
		// administrator of the channel" (WR-01) is a fact about the channel that makes the rule
		// inert for everybody, so either one means the channel part needs no job.
		if channel != 0 && !memberReqCacheHas(ctx, memberReqSubKey(channel, userID)) &&
			!memberReqCacheHas(ctx, memberReqChanNotAdminKey(channel)) {
			return false, passHit
		}
	}
	return true, passHit
}

// memberReqAIOnlyPaused reports whether a member the cached read could not call satisfied is open
// only because of the AI rule, which is paused for the chat (the provider-wide pause or the group's
// own, memberReqAIPaused: in memory and pure). While the breaker is paused nothing can be judged, so
// a job would make a paced live lookup and end in nothing for every message of every member who
// still has a join marker (WR-04). The watcher then does nothing at all: it starts no job and does
// NOT take the clean path, because "satisfied" would run ClearHoldOnClean and delete the hold of a
// member held for an AI breach; the marker stays and the first message after the pause is judged.
// The cheap tests come first, so an unpaused group costs nothing extra; during a pause one more
// Redis-only read asks whether every other lookup rule is satisfied (it never calls Telegram or the
// provider and writes nothing). Any other open reason is false and the member is handed to a job.
func memberReqAIOnlyPaused(settings *models.ChatRequirements, chatID, userID int64, passValue string, names memberReqNames) bool {
	if !memberReqLookupRulesOf(settings).AI || !memberReqAIPaused(chatID) {
		return false
	}
	othersSatisfied, _ := memberReqLookupCachedWith(settings, chatID, userID, passValue, names, true)
	return othersSatisfied
}

// memberReqCachedChannel reads the linked channel out of the lc command: 0 for no linked channel,
// a negative chat ID for a channel, and false for a miss, an error or a value that is neither
// (a channel ID is never positive, and none is ever written as one).
func memberReqCachedChannel(cmd *redis.StringCmd) (int64, bool) {
	value, err := cmd.Result()
	if err != nil {
		return 0, false
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id > 0 {
		return 0, false
	}
	return id, true
}

// memberReqStartJob hands the member to the executor on the runner, with a snapshot of the
// settings the watcher read as the cheap pre-filter.
func memberReqStartJob(b *gotgbot.Bot, chat *gotgbot.Chat, msg *gotgbot.Message, settings *models.ChatRequirements) {
	snapshot := *settings
	memberReqRunAsync(b, memberReqCase{
		Chat:        *chat,
		User:        *msg.From,
		Source:      memberReqFromMessage,
		MessageID:   msg.MessageId,
		MessageDate: msg.Date,
		Settings:    &snapshot,
	})
}

// memberReqWatchClean is the watcher's path for a member whose names break no rule. With no
// lookup rule on it is the Phase 13 clean path exactly. With one on, the member is clean only
// when every lookup rule has a cached positive (memberReqLookupCached); the pass key's clean
// predicate then runs as before. Otherwise the member is handed to one job, which asks Telegram:
// the watcher never does, never classifies and never writes a verdict of any kind, except that a
// member open only because of a paused AI rule gets neither a job nor the clean path
// (memberReqAIOnlyPaused, WR-04).
func memberReqWatchClean(ctx context.Context, b *gotgbot.Bot, chat *gotgbot.Chat, msg *gotgbot.Message, userID int64, settings *models.ChatRequirements, names memberReqNames) {
	if !memberReqLookupRulesOf(settings).any() {
		memberReqCleanMember(ctx, chat.Id, userID, msg.Date, settings, names)
		return
	}
	value := memberReqPassValue(settings, names)
	satisfied, passHit := memberReqLookupCached(settings, chat.Id, userID, value, names)
	if satisfied {
		if !passHit {
			memberReqClean(ctx, chat.Id, userID, time.Unix(msg.Date, 0), value)
		}
		return
	}
	if memberReqAIOnlyPaused(settings, chat.Id, userID, value, names) {
		return
	}
	if memberReqMissExempt(ctx, chat.Id, userID) {
		return
	}
	memberReqStartJob(b, chat, msg, settings)
}

// memberReqMissExempt reports whether a member with a lookup cache miss costs no job (D-25): the
// cached admin list knows them as an administrator, they are approved, or the approvals read
// failed (an unknown, which never starts a job and so never punishes). Without it an exempt member
// with no photo, who is exempt inside the job and so never gets a positive cached, would cost one
// job per message for ever. It reads the cached admin list and the cached approvals only, and
// makes no Telegram call.
func memberReqMissExempt(ctx context.Context, chatID, userID int64) bool {
	if known, isAdmin := cachedAdminStatus(chatID, userID); known && isAdmin {
		return true
	}
	approved, err := memberReqApprovedLookup(ctx, chatID, userID)
	if err != nil {
		log.Debugf("[MemberReq] approval of user %d in chat %d is unknown, so no lookup job is started: %s", userID, chatID, memberReqErrText(err))
		return true
	}
	return approved
}

// memberReqRunAsync hands a violating member to the executor on its own goroutine, so the
// update goroutine goes on to the commands without waiting on a pacer slot or a Telegram
// call. The job is counted before the draining flag is read (a drain that gives up cannot
// miss it), capped at memberReqMaxInFlight (a full runner sheds it: one log line with the
// chat and user IDs, the member's next message tries again) and recovers from a panic. Its
// context ends memberReqJobTimeout after it starts, before memberReqEnforce claims the hold.
// It reports whether a job was started.
func memberReqRunAsync(b *gotgbot.Bot, c memberReqCase) bool {
	if !memberReqJobs.startBelow(memberReqMaxInFlight) {
		// Debug, not Warn: a busy group with a lookup rule on can shed many checks a second.
		log.Debugf("[MemberReq] the runner is full, so the check of user %d in chat %d was shed; their next message is checked again", c.User.Id, c.Chat.Id)
		return false
	}
	if memberReqDraining.Load() {
		memberReqJobs.finish()
		return false
	}
	go func() {
		defer memberReqJobs.finish()
		defer error_handling.RecoverFromPanic("memberReqJob", "MemberRequirements")
		ctx, cancel := context.WithTimeout(memberReqContext(), memberReqJobTimeout)
		defer cancel()
		memberReqEnforce(ctx, b, c)
	}()
	return true
}

// memberReqWatcher is the group -2 message watcher. Checks run cheapest first, because it
// sees every supergroup message: the switch, the item and the sender's shape (pure Go), the
// chat's cached settings, then the pure name rules and the cached admin list; only a
// violation costs more, and that runs on a job. For a member with clean names in a group with
// a lookup rule on it also reads the cached lookup positives (memberReqLookupCached, Redis
// only) and hands a miss to a job. It makes no Telegram call and no database write for a clean
// member beyond the pass-guarded clean predicate, never caches a violation or a lookup verdict
// and never ends or skips an update.
//
// The result is named and preset: a panic the deferred recover swallows would otherwise
// return nil, which the dispatcher reads as "leave the group".
func (m moduleStruct) memberReqWatcher(b *gotgbot.Bot, ctx *ext.Context) (err error) {
	err = ext.ContinueGroups
	defer error_handling.RecoverFromPanic("memberReqWatcher", "MemberRequirements")
	if !memberReqLive() {
		return ext.ContinueGroups
	}
	msg, chat := ctx.EffectiveMessage, ctx.EffectiveChat
	if msg == nil || chat == nil || chat.Type != "supergroup" || msg.From == nil {
		return ext.ContinueGroups
	}
	if _, _, ok := raidFloodItemKey(msg); !ok {
		return ext.ContinueGroups
	}
	userID, ok := raidFloodSenderCounts(b, ctx)
	if !ok {
		return ext.ContinueGroups
	}
	updateCtx := tracing.UpdateContext(ctx)
	settings, lookupErr := memberReqSettingsLookup(updateCtx, chat.Id)
	if lookupErr != nil || settings == nil || !memberreq.AnyRuleOn(settings) {
		return ext.ContinueGroups
	}
	names := memberReqNamesOf(*msg.From)
	if memberReqBroken(settings, names) == 0 {
		memberReqWatchClean(updateCtx, b, chat, msg, userID, settings, names)
		return ext.ContinueGroups
	}
	if known, isAdmin := cachedAdminStatus(chat.Id, userID); known && isAdmin {
		return ext.ContinueGroups
	}
	memberReqStartJob(b, chat, msg, settings)
	return ext.ContinueGroups
}

// memberReqStopWait is the longest StopMemberReqJobs waits for the jobs in flight at
// shutdown. A job cut off leaves at worst a pending hold that a reclaim retries after
// memberreq.StaleClaim, so nothing is lost by giving up. A variable so a test can shorten it.
var memberReqStopWait = 2 * time.Second

// StopMemberReqJobs is the shutdown drain of the enforcement jobs. It sets the draining flag,
// so a late message starts no job, cancels the jobs' root context first (every paced Telegram
// call and database write of a job in flight aborts at once, and a job cut off punishes
// nobody it has not already punished), then waits at most memberReqStopWait for the jobs and
// reports whether they all finished. It is registered after the database close, because a job
// reads and writes the database.
func StopMemberReqJobs() bool {
	memberReqDraining.Store(true)
	memberReqContext()
	memberReqRoot.mu.Lock()
	memberReqRoot.cancel()
	memberReqRoot.mu.Unlock()

	select {
	case <-memberReqJobs.idleChan():
		return true
	case <-time.After(memberReqStopWait):
		log.Warn("[MemberReq] shutdown drain timed out with enforcement jobs still in flight")
		return false
	}
}
