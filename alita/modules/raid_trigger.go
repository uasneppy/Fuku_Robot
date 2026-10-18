package modules

import (
	"context"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/db/lockdown"
	"github.com/divkix/Alita_Robot/alita/utils/cache"
)

// raidFireTimeout bounds one automatic start: the bot-rights check, getChat, the row
// insert and the lock call.
const raidFireTimeout = 30 * time.Second

// raidRetryLatch is how long the fire latch lasts after an attempt that failed for a
// reason that may pass at once (a Telegram or database error), so the next qualifying join
// tries again soon.
const raidRetryLatch = 10 * time.Second

// raidShortenLatch cuts the chat's fire latch down to d, best effort: the latch only sheds
// load, so a Redis error is ignored and the latch simply keeps its full length.
func raidShortenLatch(chatID int64, d time.Duration) {
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
// chat is still decided by lockdown.Start behind uk_chat_lockdowns_active. A lost latch
// or a Redis error ends the attempt: the next join counts again.
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
		raidShortenLatch(chat.Id, raidRetryLatch)
		return
	}
	if active != nil {
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
	log.Infof("[RaidDetection] trigger %s fired in chat %d (count %d, window %d s, percent %d): outcome %d",
		trig.Kind, chat.Id, trig.Count, trig.Window, trig.Percent, res.Outcome)
}
