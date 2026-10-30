package captchaweb

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// verifyIPLimit is how many verify requests one client address may send per window.
	verifyIPLimit = 60
	// verifyUserLimit is how many verify requests one verified Telegram user may send per window.
	verifyUserLimit = 20
	// verifyTokenUserLimit is how many verify requests one verified user may send per window for
	// one challenge token. The window is keyed by the token and the verified user, never by the
	// token alone, so a member who hammers another member's public token spends only their own
	// budget (D-26).
	verifyTokenUserLimit = 8
	// verifyWindow is the fixed window of all three budgets.
	verifyWindow = time.Minute

	// The window keys sit outside the alita:cache: prefix, like the challenge keys.
	limitIPKeyPrefix        = "alita:captcha:rl:ip:"
	limitUserKeyPrefix      = "alita:captcha:rl:u:"
	limitTokenUserKeyPrefix = "alita:captcha:rl:t:"
)

// ErrLimiterUnavailable means the limiter has no Redis client.
var ErrLimiterUnavailable = errors.New("captchaweb: limiter unavailable")

// Limiter counts requests in fixed windows kept in Redis, so every replica shares the same
// budgets. It holds no in-process state. An error from Redis is returned as an error and is
// never an unlimited pass: the caller refuses the request.
type Limiter struct {
	client *redis.Client
}

// NewLimiter returns a Limiter over client.
func NewLimiter(client *redis.Client) *Limiter {
	return &Limiter{client: client}
}

// windowScript counts one request in a fixed window.
//
// KEYS[1] = the window key. ARGV[1] = the window in milliseconds. The window starts at the
// first request (INCR then PEXPIRE) and a key that somehow lost its expiry gets one again, so a
// window can never become permanent. Returns {count, pttl in ms}.
var windowScript = redis.NewScript(`
local n = redis.call('INCR', KEYS[1])
if n == 1 then redis.call('PEXPIRE', KEYS[1], ARGV[1]) end
local ttl = redis.call('PTTL', KEYS[1])
if ttl < 0 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
  ttl = tonumber(ARGV[1])
end
return {n, ttl}
`)

// Allow counts one request under key and reports whether it is within limit for the window.
// When it is not, retryAfter is the time until the window ends. The call is bounded by
// RedisTimeout.
func (l *Limiter) Allow(ctx context.Context, key string, limit int64, window time.Duration) (allowed bool, retryAfter time.Duration, err error) {
	if l == nil || l.client == nil {
		return false, 0, ErrLimiterUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, RedisTimeout)
	defer cancel()

	raw, err := windowScript.Run(ctx, l.client, []string{key}, window.Milliseconds()).Slice()
	if err != nil {
		return false, 0, err
	}
	if len(raw) != 2 {
		return false, 0, ErrLimiterUnavailable
	}
	count, countOK := raw[0].(int64)
	pttl, pttlOK := raw[1].(int64)
	if !countOK || !pttlOK {
		return false, 0, ErrLimiterUnavailable
	}
	if count <= limit {
		return true, 0, nil
	}
	retryAfter = time.Duration(pttl) * time.Millisecond
	if retryAfter <= 0 || retryAfter > window {
		retryAfter = window
	}
	return false, retryAfter, nil
}
