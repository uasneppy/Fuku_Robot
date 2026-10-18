package modules

import (
	"context"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/utils/cache"
)

// raidFireTimeout bounds one automatic start: the bot-rights check, getChat, the row
// insert and the lock call.
const raidFireTimeout = 30 * time.Second

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
