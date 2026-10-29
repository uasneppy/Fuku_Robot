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
	// raidPartAISpammers is the snapshot of the accounts behind the counted verdicts, taken
	// when the AI trigger fires: a sorted set of user IDs scored by the time of their join
	// when it is within 24 hours, else by the time of their verdict, in unix milliseconds.
	raidPartAISpammers = "aispammers"
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

// raidAISpammersScript copies the accounts behind the counted verdicts into the snapshot
// set when the AI trigger fires (D-09, research Pitfall 10). The verdict sets only keep
// 120 seconds and expire in minutes, and the join tracker only knows the last 10 minutes
// of joins, so an account that joined hours ago, or an established member whose delete
// verdict counted (D-19), would be on neither once the alert is decided. It replaces any
// older snapshot, takes the newest accounts of the accounts set (at most the given
// maximum) and scores each by its join time when the joined set has it within the recent
// window, else by its verdict time, then sets the TTL. It returns how many accounts it
// kept.
//
// KEYS: 1 AI accounts, 2 joined set, 3 spammers snapshot. ARGV: 1 now (ms), 2 recent
// window (ms), 3 maximum accounts, 4 TTL (ms).
var raidAISpammersScript = redis.NewScript(`
redis.call('DEL', KEYS[3])
local now = tonumber(ARGV[1])
local cutoff = now - tonumber(ARGV[2])
local entries = redis.call('ZREVRANGE', KEYS[1], 0, tonumber(ARGV[3]) - 1, 'WITHSCORES')
local kept = 0
for i = 1, #entries, 2 do
  local id = entries[i]
  local score = entries[i + 1]
  local joinedAt = redis.call('ZSCORE', KEYS[2], id)
  if joinedAt and tonumber(joinedAt) > cutoff then
    score = joinedAt
  end
  redis.call('ZADD', KEYS[3], score, id)
  kept = kept + 1
end
if kept > 0 then redis.call('PEXPIRE', KEYS[3], ARGV[4]) end
return kept
`)

// raidSnapshotAISpammers saves the accounts behind the chat's AI spam verdicts with a
// time each, for the alert of the lockdown the AI trigger is about to start. It returns
// how many accounts it saved. Without Redis it saves nothing and returns (0, nil). One
// script call under raidRedisTimeout; an error is returned for the caller to log, and the
// lockdown starts without a snapshot.
func raidSnapshotAISpammers(chatID int64) (int, error) {
	client := raidRedis()
	if client == nil {
		return 0, nil
	}
	keys := []string{
		raidKey(chatID, raidPartAIAccounts),
		raidKey(chatID, raidPartJoined),
		raidKey(chatID, raidPartAISpammers),
	}
	ctx, cancel := context.WithTimeout(cache.Context, raidRedisTimeout)
	defer cancel()
	return raidAISpammersScript.Run(ctx, client, keys,
		raidNow().UnixMilli(),
		raidRecentWindow.Milliseconds(),
		raidFloodersMax,
		raidFloodersTTL.Milliseconds(),
	).Int()
}

// readRaidAISpammers returns the chat's AI spammer snapshot, oldest time first, each as a
// lockdownRecentJoin whose At is the account's join time (or, for an account that did not
// join recently, the time of its verdict) and which carries no name: the snapshot holds
// IDs only. Without Redis it returns nothing and no error; no snapshot is the same.
func readRaidAISpammers(chatID int64) ([]lockdownRecentJoin, error) {
	client := raidRedis()
	if client == nil {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(cache.Context, raidRedisTimeout)
	defer cancel()
	entries, err := client.ZRangeWithScores(ctx, raidKey(chatID, raidPartAISpammers), 0, -1).Result()
	if err != nil {
		return nil, err
	}
	spammers := make([]lockdownRecentJoin, 0, len(entries))
	for _, entry := range entries {
		raw, ok := entry.Member.(string)
		if !ok {
			continue
		}
		userID, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil || userID == 0 {
			continue
		}
		spammers = append(spammers, lockdownRecentJoin{UserID: userID, At: time.UnixMilli(int64(entry.Score))})
	}
	return spammers, nil
}

// raidAISpammersLookup is the snapshot read of the AI spammer accounts. It is a variable
// only so a test can replace or watch it; production code never reassigns it.
var raidAISpammersLookup = readRaidAISpammers
