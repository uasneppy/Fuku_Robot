package modules

import (
	"context"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/db/lockdown"
	"github.com/divkix/Alita_Robot/alita/utils/cache"
)

const (
	// raidFireTimeout bounds one automatic start: the bot-rights check, getChat, the row
	// insert and the lock call.
	raidFireTimeout = 30 * time.Second

	// raidLiftQuiet is how long after an admin started lifting a group's lockdown the
	// automatic triggers leave the group alone (D-15).
	raidLiftQuiet = 10 * time.Minute

	// raidRetryLatch is how long the fire latch lasts after an attempt that failed for a
	// reason that may pass at once (a Telegram or database error), so the next qualifying
	// join tries again soon.
	raidRetryLatch = 10 * time.Second

	// raidCantLockLatch is how long the fire latch lasts for a group the bot cannot lock
	// (not a supergroup, no restrict right, or a lock Telegram refused), so a sustained
	// raid does not repeat the same failing calls on every join.
	raidCantLockLatch = 10 * time.Minute
)

// raidSetLatch sets the remaining life of the chat's fire latch to d, best effort: the
// latch only sheds load, so a Redis error is ignored and the latch keeps the length it has.
func raidSetLatch(chatID int64, d time.Duration) {
	client := raidRedis()
	if client == nil {
		return
	}
	ctx, cancel := context.WithTimeout(cache.Context, raidRedisTimeout)
	defer cancel()
	if err := client.PExpire(ctx, raidKey(chatID, raidPartFire), d).Err(); err != nil {
		log.Debugf("[RaidDetection] could not set the fire latch of chat %d to %s: %v", chatID, d, err)
	}
}

// raidOutcomeLatch is the latch length an outcome of startLockdownRow asks for, and its
// name for the log. A zero length keeps the latch's full 60 seconds: a started lockdown,
// one that was already active, and a lock whose answer was lost (the worker settles that
// row). A failure that may pass at once is retried after raidRetryLatch, and a group the
// bot cannot lock waits raidCantLockLatch.
func raidOutcomeLatch(outcome lockdownStartOutcome) (name string, latch time.Duration) {
	switch outcome {
	case lockdownStartStarted:
		return "started", 0
	case lockdownStartAlreadyActive:
		return "already active", 0
	case lockdownStartLockUnknown:
		return "lock unknown", 0
	case lockdownStartBotCheckFailed:
		return "bot check failed", raidRetryLatch
	case lockdownStartPermissionsUnreadable:
		return "permissions unreadable", raidRetryLatch
	case lockdownStartStateFailed:
		return "state failed", raidRetryLatch
	case lockdownStartNotSupergroup:
		return "not a supergroup", raidCantLockLatch
	case lockdownStartBotCannotRestrict:
		return "bot cannot restrict", raidCantLockLatch
	case lockdownStartLockFailed:
		return "lock refused", raidCantLockLatch
	}
	return "unknown", raidRetryLatch
}

// raidTrigger is what fired: the trigger kind (models.LockdownTrigger*) and the numbers
// behind it. Percent is 0 for a join surge.
type raidTrigger struct {
	Kind    string
	Count   int
	Window  int
	Percent int
}

// raidFire starts the lockdown a trigger asks for, through startLockdownRow exactly as
// /lockdown does, with the bot as the starter and the trigger's numbers on the row. It
// runs synchronously in the caller's handler: a crash half way leaves an unconfirmed row
// the lockdown worker settles on its own.
//
// The latch (SET NX with a 60 second expiry) only sheds load, so a burst of joins past
// the threshold does not make every join attempt the start; the one active lockdown per
// chat is still decided by lockdown.Start behind uk_chat_lockdowns_active, and the one
// alert by lockdown.BeginAlert, however many triggers and replicas fire at once. A lost
// latch or a Redis error ends the attempt: the next join counts again.
//
// After the latch it stays silent, with no Telegram call, while the chat has an active
// lockdown and for raidLiftQuiet after an admin started lifting one (D-15); a failed read
// of either shortens the latch to raidRetryLatch. After the start the latch is cut or
// stretched by outcome (raidOutcomeLatch).
func raidFire(b *gotgbot.Bot, chat *gotgbot.Chat, trig raidTrigger) {
	client := raidRedis()
	if client == nil {
		return
	}

	latchCtx, cancelLatch := context.WithTimeout(cache.Context, raidRedisTimeout)
	won, err := client.SetNX(latchCtx, raidKey(chat.Id, raidPartFire), "1", raidFireLatch).Result()
	cancelLatch()
	if err != nil {
		log.Debugf("[RaidDetection] fire latch of chat %d failed: %v", chat.Id, err)
		return
	}
	if !won {
		return
	}

	// A lockdown that is active, confirmed or not, already answers this raid: stay silent
	// before any Telegram call, so a trigger that keeps firing costs one database read per
	// latch period. The latch is kept. Exactly one lockdown is still lockdown.Start's
	// partial unique index, this read only saves the calls a loser would make.
	active, err := lockdown.GetActiveFresh(chat.Id)
	if err != nil {
		log.Warnf("[RaidDetection] the lockdown of chat %d could not be read, trying again soon: %v", chat.Id, err)
		raidSetLatch(chat.Id, raidRetryLatch)
		return
	}
	if active != nil {
		return
	}

	// An admin lifted this group's lockdown a moment ago, so the raid they ended is not a
	// new one. lift_started_at is written with real time, so the window is real time too.
	// An unreadable answer is no information: try again soon rather than lock or wait.
	lifted, err := lockdown.RecentlyLiftedFresh(chat.Id, time.Now().Add(-raidLiftQuiet))
	if err != nil {
		log.Warnf("[RaidDetection] the recent lifts of chat %d could not be read, trying again soon: %v", chat.Id, err)
		raidSetLatch(chat.Id, raidRetryLatch)
		return
	}
	if lifted {
		log.Infof("[RaidDetection] trigger %s in chat %d ignored: its lockdown was lifted less than %s ago",
			trig.Kind, chat.Id, raidLiftQuiet)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), raidFireTimeout)
	defer cancel()
	res := startLockdownRow(ctx, b, lockdownStartParams{
		Chat:           chat,
		TriggerKind:    trig.Kind,
		StarterID:      b.Id,
		StarterName:    lockdownCapRunes(b.FirstName, lockdownNameMaxRunes),
		TriggerCount:   trig.Count,
		TriggerWindow:  trig.Window,
		TriggerPercent: trig.Percent,
	})
	name, latch := raidOutcomeLatch(res.Outcome)
	if latch > 0 {
		raidSetLatch(chat.Id, latch)
	}
	log.Infof("[RaidDetection] trigger %s fired in chat %d (count %d, window %d s, percent %d): %s",
		trig.Kind, chat.Id, trig.Count, trig.Window, trig.Percent, name)
}
