package modules

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
	"github.com/uasneppy/fuku_robot/fuku/utils/error_handling"
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

// memberReqRunAsync hands a violating member to the executor on its own goroutine, so the
// update goroutine goes on to the commands without waiting on a pacer slot or a Telegram
// call. The job is counted before the draining flag is read (a drain that gives up cannot
// miss it), capped at memberReqMaxInFlight (a full runner sheds it: one log line with the
// chat and user IDs, the member's next message tries again) and recovers from a panic. Its
// context ends memberReqJobTimeout after it starts, before memberReqEnforce claims the hold.
// It reports whether a job was started.
func memberReqRunAsync(b *gotgbot.Bot, c memberReqCase) bool {
	if !memberReqJobs.startBelow(memberReqMaxInFlight) {
		log.Warnf("[MemberReq] the runner is full, so the check of user %d in chat %d was shed; their next message is checked again", c.User.Id, c.Chat.Id)
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
// violation costs more, and that runs on a job. It makes no Telegram call and no database
// write for a clean member, never caches a violation and never ends or skips an update.
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
		memberReqCleanMember(updateCtx, chat.Id, userID, msg.Date, settings, names)
		return ext.ContinueGroups
	}
	if known, isAdmin := cachedAdminStatus(chat.Id, userID); known && isAdmin {
		return ext.ContinueGroups
	}
	snapshot := *settings
	memberReqRunAsync(b, memberReqCase{
		Chat:        *chat,
		User:        *msg.From,
		Source:      memberReqFromMessage,
		MessageID:   msg.MessageId,
		MessageDate: msg.Date,
		Settings:    &snapshot,
	})
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
