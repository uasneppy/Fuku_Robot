package modules

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
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

// memberReqMaxInFlight is the most enforcement jobs that run at once on this replica. Over it
// a job is shed with one log line and the member's next message is checked again. A variable
// so a test can make it small.
var memberReqMaxInFlight = 32

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
	settings, lookupErr := memberReqSettingsLookup(tracing.UpdateContext(ctx), chat.Id)
	if lookupErr != nil || settings == nil || !memberreq.AnyRuleOn(settings) {
		return ext.ContinueGroups
	}
	names := memberReqNamesOf(*msg.From)
	if memberReqBroken(settings, names) == 0 {
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
