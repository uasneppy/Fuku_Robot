package captchaweb

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// StreakKey is the fleet-wide count of failed solves since the last confirmed pass, beside
	// the challenge and window keys and outside the cache prefix for the same reason.
	StreakKey = "alita:captcha:streak"
	// StreakTTL is how long the count survives without a new failed solve. It is renewed by
	// every failed solve, so a streak that goes on stays tripped and a quiet one ends.
	StreakTTL = 10 * time.Minute
	// StreakLimit is how many failed solves in a row (no confirmed pass between them) are
	// still enforced. The next one is held as a retry (WR-04).
	StreakLimit = 5
)

// failedSolveScript counts one failed solve and renews the streak's expiry.
//
// KEYS[1] = the streak key. ARGV[1] = the expiry in milliseconds. Returns the new count.
var failedSolveScript = redis.NewScript(`
local n = redis.call('INCR', KEYS[1])
redis.call('PEXPIRE', KEYS[1], ARGV[1])
return n
`)

// RecordFailedSolve counts one failed solve in the streak and returns the streak including it.
// The call is bounded by RedisTimeout. A Redis error is returned as an error and the caller
// fails open: it must neither hold the solve back nor block the member (D-28's classification
// stays in force when the breaker cannot be read).
func (s *Store) RecordFailedSolve(ctx context.Context) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, RedisTimeout)
	defer cancel()
	return failedSolveScript.Run(ctx, s.client, []string{StreakKey}, StreakTTL.Milliseconds()).Int64()
}

// ClearFailedSolves ends the streak. A confirmed pass proves the site key and the secret belong
// to one widget, so it runs on every pass. The call is bounded by RedisTimeout.
func (s *Store) ClearFailedSolves(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, RedisTimeout)
	defer cancel()
	return s.client.Del(ctx, StreakKey).Err()
}
