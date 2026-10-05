package ratelimit

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/utils/cache"
)

const (
	defaultPacerInterval   = 100 * time.Millisecond
	defaultPacerMaxRetries = 3
	defaultPacerMaxWait    = 60 * time.Second
	// pacerRedisTimeout bounds one reservation or block write, so a dead Redis
	// costs a fallback to the local interval and not a stalled run.
	pacerRedisTimeout = 2 * time.Second
	// pacerKeySlackMs keeps the slot key alive a little past the slot it records.
	pacerKeySlackMs = 1000
	// pacerWarnEvery limits the Redis-down warning to one per pacer per interval.
	pacerWarnEvery = time.Minute
	// pacerRedisBackoff is how long the pacer stays on its local interval after a
	// Redis failure, so an outage does not add a dial timeout to every call.
	pacerRedisBackoff = 5 * time.Second
)

// TelegramPacerOptions tunes a TelegramPacer. Zero values take the defaults:
// 100 ms between calls, 3 retries, a 60 s cap on one wait and seconds as the unit
// of Telegram's retry_after.
type TelegramPacerOptions struct {
	// NextKey holds the next free slot, as a Unix time in milliseconds.
	NextKey string
	// BlockKey is set from every 429's retry_after and holds every reservation
	// back until it expires.
	BlockKey string
	// Interval is the least gap between two calls across every replica.
	Interval time.Duration
	// MaxRetries is how many times a call is repeated after a 429.
	MaxRetries int
	// MaxWait is the longest wait a call accepts, for its own retry_after or for
	// its slot behind the shared block and other callers. A longer one fails the
	// call at once with ErrRateLimited and no Telegram request.
	MaxWait time.Duration
	// RetryAfterUnit is the length of one retry_after second; tests shorten it.
	RetryAfterUnit time.Duration
}

// TelegramPacer paces Telegram calls across every replica that shares one Redis.
// Each call reserves the next free slot in Redis, so the fleet as a whole stays
// one Interval apart, and every 429 extends a shared block that every replica's
// next reservation honours. If Redis fails it falls back to a local interval.
type TelegramPacer struct {
	opts TelegramPacerOptions

	mu         sync.Mutex
	localNext  time.Time
	localBlock time.Time
	lastWarn   time.Time
	// redisDownUntil is when the pacer next tries Redis again after a failure.
	redisDownUntil time.Time
}

// ErrRateLimited is returned when Telegram kept answering 429 after the allowed
// retries, or asked for a wait longer than the cap.
var ErrRateLimited = errors.New("telegram rate limit: gave up after retries")

// reserveSlotScript hands the caller a wait, in milliseconds, so that calls made
// through it are at least ARGV[1] ms apart and none starts before the shared
// block ends. The slot key lives until the slot it records, plus ARGV[2] ms.
// ARGV[3] is the caller's max wait in ms: when the slot is further away than that
// the script returns the distance without taking the slot, so a refused caller
// leaves the next-slot key exactly as it was.
var reserveSlotScript = redis.NewScript(`
local t = redis.call('TIME')
local now = tonumber(t[1]) * 1000 + math.floor(tonumber(t[2]) / 1000)
local nxt = tonumber(redis.call('GET', KEYS[1]) or '0')
local blk = redis.call('PTTL', KEYS[2])
local slot = math.max(now, nxt)
if blk > 0 then slot = math.max(slot, now + blk) end
local interval = tonumber(ARGV[1])
if slot - now > tonumber(ARGV[3]) then return slot - now end
redis.call('SET', KEYS[1], slot + interval, 'PX', slot - now + interval + tonumber(ARGV[2]))
return slot - now
`)

// extendBlockScript sets the block to ARGV[1] ms unless it already lasts longer.
var extendBlockScript = redis.NewScript(`
if redis.call('PTTL', KEYS[1]) < tonumber(ARGV[1]) then
  redis.call('SET', KEYS[1], 1, 'PX', ARGV[1])
end
return 1
`)

// NewTelegramPacer returns a pacer for opts, with the defaults filled in.
func NewTelegramPacer(opts TelegramPacerOptions) *TelegramPacer {
	if opts.Interval <= 0 {
		opts.Interval = defaultPacerInterval
	}
	if opts.MaxRetries <= 0 {
		opts.MaxRetries = defaultPacerMaxRetries
	}
	if opts.MaxWait <= 0 {
		opts.MaxWait = defaultPacerMaxWait
	}
	if opts.RetryAfterUnit <= 0 {
		opts.RetryAfterUnit = time.Second
	}
	return &TelegramPacer{opts: opts}
}

// RetryAfterSeconds reports Telegram's retry_after for a 429 flood-control answer.
// It is false for any other error, and for a 429 that carries no retry_after.
func RetryAfterSeconds(err error) (int64, bool) {
	var tgErr *gotgbot.TelegramError
	if errors.As(err, &tgErr) && tgErr.Code == 429 && tgErr.ResponseParams != nil && tgErr.ResponseParams.RetryAfter > 0 {
		return tgErr.ResponseParams.RetryAfter, true
	}
	return 0, false
}

// Do waits for a slot, runs call and, when Telegram answers 429 with a
// retry_after, extends the shared block and runs call again, up to MaxRetries
// times. The same closure is invoked every time, so its arguments (an until_date,
// for one) are identical on every attempt. Any other error is returned as it is.
// A retry_after above MaxWait, one still answered after MaxRetries retries, or a
// slot more than MaxWait away (behind a long shared block) returns an error that
// wraps ErrRateLimited; in the last case no Telegram request is made and no slot
// is taken.
func (p *TelegramPacer) Do(ctx context.Context, call func(context.Context) error) error {
	for retries := 0; ; retries++ {
		if err := p.wait(ctx); err != nil {
			return err
		}
		err := call(ctx)
		seconds, limited := RetryAfterSeconds(err)
		if !limited {
			return err
		}
		d := time.Duration(seconds) * p.opts.RetryAfterUnit
		p.block(d)
		if d > p.opts.MaxWait || retries >= p.opts.MaxRetries {
			return fmt.Errorf("%w: %v", ErrRateLimited, err)
		}
	}
}

// wait reserves a slot and sleeps until it starts. It returns the context's error
// if the context ends first.
func (p *TelegramPacer) wait(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	delay, ok := p.reserve(ctx)
	if !ok {
		delay = p.reserveLocal()
	}
	if delay > p.opts.MaxWait {
		return fmt.Errorf("%w: next slot is %s away, over the %s cap", ErrRateLimited, delay, p.opts.MaxWait)
	}
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// reserve takes the next fleet-wide slot from Redis. It reports false when Redis
// is unavailable or fails, and the caller then uses the local interval.
func (p *TelegramPacer) reserve(ctx context.Context) (time.Duration, bool) {
	if p.redisBackedOff() {
		return 0, false
	}
	client := cache.GetRedisClient()
	if client == nil {
		p.warnRedisDown(errors.New("no Redis client"))
		return 0, false
	}
	callCtx, cancel := context.WithTimeout(ctx, pacerRedisTimeout)
	defer cancel()
	ms, err := reserveSlotScript.Run(callCtx, client,
		[]string{p.opts.NextKey, p.opts.BlockKey},
		max(p.opts.Interval.Milliseconds(), 1), pacerKeySlackMs, p.opts.MaxWait.Milliseconds()).Int64()
	if err != nil {
		p.warnRedisDown(err)
		return 0, false
	}
	return time.Duration(ms) * time.Millisecond, true
}

// reserveLocal is the per-replica stand-in for reserve when Redis is down: it
// spaces this replica's calls one interval apart and honours the local block.
func (p *TelegramPacer) reserveLocal() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	slot := now
	if p.localNext.After(slot) {
		slot = p.localNext
	}
	if p.localBlock.After(slot) {
		slot = p.localBlock
	}
	p.localNext = slot.Add(p.opts.Interval)
	return slot.Sub(now)
}

// block extends the shared block, and the local one, by d.
func (p *TelegramPacer) block(d time.Duration) {
	p.mu.Lock()
	if until := time.Now().Add(d); until.After(p.localBlock) {
		p.localBlock = until
	}
	p.mu.Unlock()

	client := cache.GetRedisClient()
	if client == nil || p.redisBackedOff() {
		return
	}
	// A fresh context: the block must be recorded even when the caller's context
	// has just ended.
	ctx, cancel := context.WithTimeout(context.Background(), pacerRedisTimeout)
	defer cancel()
	if err := extendBlockScript.Run(ctx, client, []string{p.opts.BlockKey}, max(d.Milliseconds(), 1)).Err(); err != nil {
		p.warnRedisDown(err)
	}
}

// redisBackedOff reports whether a recent Redis failure still keeps the pacer on
// its local interval.
func (p *TelegramPacer) redisBackedOff() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return time.Now().Before(p.redisDownUntil)
}

// warnRedisDown records a Redis failure, which keeps the pacer local for a few
// seconds, and logs it at most once per minute per pacer.
func (p *TelegramPacer) warnRedisDown(err error) {
	p.mu.Lock()
	now := time.Now()
	p.redisDownUntil = now.Add(pacerRedisBackoff)
	if !p.lastWarn.IsZero() && now.Sub(p.lastWarn) < pacerWarnEvery {
		p.mu.Unlock()
		return
	}
	p.lastWarn = now
	p.mu.Unlock()
	log.Warnf("[TelegramPacer] Redis unavailable (%v); pacing locally for %s", err, p.opts.NextKey)
}
