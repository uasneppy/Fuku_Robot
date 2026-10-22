package modules

import (
	"context"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/utils/cache"
)

// The shared Redis counters of automatic raid detection. They are best effort and only
// ever count: a count starts a lockdown through startLockdownRow and never bans, restricts
// or deletes anything by itself. Without Redis there are no counters and automatic
// detection is off on that replica; /lockdown still works.

const (
	// raidKeyPrefix starts every key of the family. It sits outside alita:cache:, so
	// CLEAR_CACHE_ON_STARTUP keeps it.
	raidKeyPrefix = "alita:raid:"
	// raidPartJoined is the sorted set of the accounts that joined a chat, scored by the
	// time of their latest join.
	raidPartJoined = "joined"
	// raidPartFire is the latch that sheds load while one trigger is starting a lockdown.
	raidPartFire = "fire"

	// raidJoinWindow is the window the join surge counts in (fixed, D-01).
	raidJoinWindow = 60 * time.Second
	// raidRecentWindow is how long a join stays in the set, for the flood trigger's "new
	// member" test (D-05).
	raidRecentWindow = 24 * time.Hour
	// raidJoinedTTL lets an idle chat's set expire on its own.
	raidJoinedTTL = 25 * time.Hour
	// raidRedisTimeout bounds the one Redis round trip the join hot path makes.
	raidRedisTimeout = 250 * time.Millisecond
	// raidFireLatch is how long one fire attempt keeps the others of the chat out.
	raidFireLatch = 60 * time.Second
)

// raidJoinedCap is the most accounts the joined set keeps per chat (the newest). It is a
// variable only so a test can make the cap small; production code never reassigns it.
var raidJoinedCap = 5000

// raidNow is the clock the counters score with. It is a variable only so a test can move
// time; production code never reassigns it.
var raidNow = time.Now

// raidNoRedisLogged makes the "no Redis" notice appear once per process. It is a package
// variable so a test can reset it.
var raidNoRedisLogged atomic.Bool

// raidKey is the Redis key of one counter of a chat. The chat ID is a hash tag, so the
// multi-key scripts of one chat stay on one cluster slot.
func raidKey(chatID int64, part string) string {
	return raidKeyPrefix + "{" + strconv.FormatInt(chatID, 10) + "}:" + part
}

// raidRedis returns the Redis client, or nil without Redis, in which case automatic raid
// detection is inert on this replica (D-04). The first call without Redis logs one line.
func raidRedis() *redis.Client {
	if cache.IsRedisAvailable() {
		if client := cache.GetRedisClient(); client != nil {
			return client
		}
	}
	if raidNoRedisLogged.CompareAndSwap(false, true) {
		log.Info("[RaidDetection] Redis is not available, so automatic raid detection is off on this replica; /lockdown still works")
	}
	return nil
}

// raidJoinScript records joins and returns the number of different accounts that joined
// within the surge window. The member is the user ID and the score the time of the latest
// join (ZADD without NX), so the chat_member update and the service message of one join
// are one member, and a re-join refreshes the account. It trims what is older than the
// recent window, keeps the newest raidJoinedCap members and sets the TTL.
//
// KEYS[1] = joined set. ARGV: 1 now (ms), 2 surge window (ms), 3 recent window (ms),
// 4 cap, 5 TTL (ms), 6.. user IDs.
var raidJoinScript = redis.NewScript(`
local now = tonumber(ARGV[1])
for i = 6, #ARGV do
  redis.call('ZADD', KEYS[1], now, ARGV[i])
end
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now - tonumber(ARGV[3]))
local size = redis.call('ZCARD', KEYS[1])
local cap = tonumber(ARGV[4])
if size > cap then redis.call('ZREMRANGEBYRANK', KEYS[1], 0, size - cap - 1) end
redis.call('PEXPIRE', KEYS[1], ARGV[5])
return redis.call('ZCOUNT', KEYS[1], '(' .. (now - tonumber(ARGV[2])), '+inf')
`)

// recordRaidJoins stores the joins of userIDs in the chat's joined set and returns how
// many different accounts joined within raidJoinWindow, the new ones included. Without
// Redis it returns (0, nil). One script call under raidRedisTimeout; an error is returned
// for the caller to log and treat as "no information".
func recordRaidJoins(chatID int64, userIDs []int64) (int, error) {
	client := raidRedis()
	if client == nil || len(userIDs) == 0 {
		return 0, nil
	}

	args := make([]any, 0, 5+len(userIDs))
	args = append(args,
		raidNow().UnixMilli(),
		raidJoinWindow.Milliseconds(),
		raidRecentWindow.Milliseconds(),
		raidJoinedCap,
		raidJoinedTTL.Milliseconds(),
	)
	for _, id := range userIDs {
		args = append(args, strconv.FormatInt(id, 10))
	}

	ctx, cancel := context.WithTimeout(cache.Context, raidRedisTimeout)
	defer cancel()
	return raidJoinScript.Run(ctx, client, []string{raidKey(chatID, raidPartJoined)}, args...).Int()
}
