package modules

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/db/raiddetection"
	"github.com/divkix/Alita_Robot/alita/utils/cache"
)

// The Redis verdict counter of the AI burst trigger of automatic raid detection (RAID-03,
// D-05, D-06). Every /aispam delete verdict, from any account (D-19), adds one item to a
// per-chat sorted set; a window of 120 seconds that holds at least the threshold of items
// from at least 3 different accounts starts a lockdown through raidFire, exactly as
// /lockdown does.
//
// AI never decides alone (RAID-05, D-07): a verdict only adds to the count, and only the
// rule (raidAICounts) fires. Nothing on the counting path deletes, restricts or bans; the
// /aispam deletion that produced the verdict is its own, unchanged code. The counter is
// best effort and Redis only: without Redis the trigger does nothing. Message text is never
// stored, only item keys and user IDs.

const (
	// raidAIWindow is the window the verdicts count in (fixed, D-05).
	raidAIWindow = 120 * time.Second
	// raidAIMinAccounts is how many different accounts must have sent the counted messages
	// (D-05): one spammer posting many messages never locks a group.
	raidAIMinAccounts = 3
	// raidAITTL lets an idle chat's sets expire on their own.
	raidAITTL = 5 * time.Minute
	// raidAIDefaultThreshold is the verdict count of the D-05 default, on in every group.
	// The chat's own stored threshold replaces it once the setting exists (plan 07-04).
	raidAIDefaultThreshold = 5

	// raidPartAIVerdicts is the sorted set of the window's verdict items: the item key
	// (m:<message id>, or g:<media group id>) scored by the time it was reported in unix
	// milliseconds.
	raidPartAIVerdicts = "av"
	// raidPartAIAccounts is the sorted set of the accounts behind them: the user ID scored by
	// the time of that account's latest newly counted verdict.
	raidPartAIAccounts = "aa"
)

// raidAICap is the most members each of the two sets keeps per chat (the newest). It is
// larger than the highest threshold a chat can store, so the cap never hides a count the
// rule needs. It is a variable only so a test can make the cap small; production code
// never reassigns it, and a test never sets it below 500 except to prove the cap itself.
var raidAICap = 1000

// raidAIVerdictScript adds one spam verdict and returns the sizes of the two sets: verdict
// items and accounts. ZADD NX makes a second report of the same message or album a no-op,
// and the account is written only when its item was newly added, so a duplicate can never
// refresh an account whose items already aged out. Both sets are trimmed to the window,
// capped by rank on every write and expire on their own.
//
// KEYS: 1 verdict items, 2 accounts. ARGV: 1 now (ms), 2 window (ms), 3 item key, 4 user
// ID, 5 cap, 6 TTL (ms).
var raidAIVerdictScript = redis.NewScript(`
local now = tonumber(ARGV[1])
local cutoff = now - tonumber(ARGV[2])
local added = redis.call('ZADD', KEYS[1], 'NX', now, ARGV[3])
if added == 1 then redis.call('ZADD', KEYS[2], now, ARGV[4]) end
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', cutoff)
redis.call('ZREMRANGEBYSCORE', KEYS[2], '-inf', cutoff)
local cap = tonumber(ARGV[5])
for i = 1, 2 do
  local n = redis.call('ZCARD', KEYS[i])
  if n > cap then redis.call('ZREMRANGEBYRANK', KEYS[i], 0, n - cap - 1) end
end
for i = 1, 2 do redis.call('PEXPIRE', KEYS[i], ARGV[6]) end
return {redis.call('ZCARD', KEYS[1]), redis.call('ZCARD', KEYS[2])}
`)

// recordRaidAIVerdict adds one spam verdict of userID to the chat's AI sets and returns
// their sizes: the verdict items and the accounts in the window. One script call under
// raidRedisTimeout; an error is returned for the caller to log and treat as "no
// information".
func recordRaidAIVerdict(client *redis.Client, chatID, userID int64, itemKey string) (items, accounts int, err error) {
	keys := []string{
		raidKey(chatID, raidPartAIVerdicts),
		raidKey(chatID, raidPartAIAccounts),
	}
	ctx, cancel := context.WithTimeout(cache.Context, raidRedisTimeout)
	defer cancel()
	sizes, err := raidAIVerdictScript.Run(ctx, client, keys,
		raidNow().UnixMilli(),
		raidAIWindow.Milliseconds(),
		itemKey,
		strconv.FormatInt(userID, 10),
		raidAICap,
		raidAITTL.Milliseconds(),
	).Int64Slice()
	if err != nil {
		return 0, 0, err
	}
	if len(sizes) != 2 {
		return 0, 0, fmt.Errorf("AI verdict script returned %d counts, want 2", len(sizes))
	}
	return int(sizes[0]), int(sizes[1]), nil
}

// raidAICounts is the AI burst rule (D-05): fire when the window holds at least threshold
// verdict items from at least raidAIMinAccounts different accounts. A threshold below the
// lowest one a chat can store is refused, so a corrupt setting can never make one verdict
// fire.
func raidAICounts(items, accounts, threshold int) bool {
	return threshold >= raiddetection.MinThreshold &&
		items >= threshold &&
		accounts >= raidAIMinAccounts
}

// raidAIItemKey is the verdict counter's key for a message, the same shape as the flood
// counter's: g:<media_group_id> for a member of an album, so an album is one item, else
// m:<message_id>.
func raidAIItemKey(msg *gotgbot.Message) string {
	if msg.MediaGroupId != "" {
		return "g:" + msg.MediaGroupId
	}
	return "m:" + strconv.FormatInt(msg.MessageId, 10)
}

// raidAIRecordVerdict reports one /aispam delete verdict to the burst counter and starts
// the lockdown when the rule holds. It does nothing for a basic group, an anonymous or
// non-positive sender, the bot itself or without Redis. The verdict only adds one item; it
// never deletes, restricts or bans, and only raidAICounts calls raidFire. The call is
// synchronous in the /aispam worker (drained by DrainAISpamChecks): raidFire is bounded by
// raidFireTimeout and its latch lets one attempt through per chat per minute.
func raidAIRecordVerdict(b *gotgbot.Bot, chat gotgbot.Chat, userID int64, itemKey string) {
	if chat.Type != "supergroup" || userID <= 0 || userID == b.Id {
		return
	}
	client := raidRedis()
	if client == nil {
		return
	}

	items, accounts, err := recordRaidAIVerdict(client, chat.Id, userID, itemKey)
	if err != nil {
		log.Debugf("[RaidDetection] AI verdict counter write for chat %d, user %d failed: %v", chat.Id, userID, err)
		return
	}
	if !raidAICounts(items, accounts, raidAIDefaultThreshold) {
		return
	}
	raidFire(b, &chat, raidTrigger{
		Kind:    models.LockdownTriggerAIBurst,
		Count:   items,
		Window:  int(raidAIWindow / time.Second),
		Percent: 0,
	})
}
