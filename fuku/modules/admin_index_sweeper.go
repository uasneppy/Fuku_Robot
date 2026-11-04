package modules

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db/adminindex"
	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
	"github.com/uasneppy/fuku_robot/fuku/utils/error_handling"
)

// adminIndexSweepLockKey is the operational Redis key one replica holds per cycle. It sits in
// the new fuku:settings:* family, outside the fuku:cache: prefix, so CLEAR_CACHE_ON_STARTUP
// leaves it alone.
const adminIndexSweepLockKey = "fuku:settings:sweep:lock"

// adminIndexSweepOwner identifies this process as the holder of the sweep lock.
var adminIndexSweepOwner = uuid.NewString()

// Variables rather than constants so tests can shorten them.
var (
	// adminIndexSweepInterval is how often the sweep runs after the backfill.
	adminIndexSweepInterval = time.Hour
	// adminIndexSweepPace is the pause before each Telegram call of the sweep, so a sweep over
	// many groups stays well under the Bot API rate limits (4 calls a second per runner).
	adminIndexSweepPace = 250 * time.Millisecond
	// adminIndexSweepCallTimeout bounds one Telegram call of the sweep.
	adminIndexSweepCallTimeout = 5 * time.Second
	// adminIndexSweep429Max caps how long the sweep waits out one rate limit.
	adminIndexSweep429Max = 30 * time.Second
)

// adminIndexSweepFirstDelay returns how long to wait before the first cycle: 30 to 90 seconds
// after start, so the private list is filled soon after a deploy (the backfill, D-05). A
// variable so tests can shorten it.
var adminIndexSweepFirstDelay = func() time.Duration {
	return 30*time.Second + time.Duration(rand.IntN(61))*time.Second
}

// adminIndexSweepCycleHook, when not nil, runs at the start of every cycle. Production leaves
// it nil; tests use it to count cycles or inject a panic.
var adminIndexSweepCycleHook func()

// adminIndexSweepReport is what one sweep cycle did.
type adminIndexSweepReport struct {
	// Ran is false when another replica holds the sweep lock for this cycle.
	Ran bool
	// Chats is the number of chats the cycle walked.
	Chats int
	// Replaced is the number of chats whose rows now hold a fresh roster.
	Replaced int
	// Cleared is the number of chats whose rows were removed (the bot is not an administrator
	// there, or cannot see the chat).
	Cleared int
	// Unchanged is the number of chats left as they were: an unreadable or empty answer.
	Unchanged int
	// RateLimited is true when the cycle ended early on a second 429 for one chat.
	RateLimited bool
}

// Lifecycle state of the sweeper goroutine.
var (
	adminIndexSweepMu     sync.Mutex
	adminIndexSweepCancel context.CancelFunc
	adminIndexSweepWG     sync.WaitGroup
)

// StartAdminIndexSweeper starts the background loop that fills chat_admin_index for every group
// where the bot is an administrator (the first cycle, 30 to 90 seconds after start) and then
// rechecks every group hourly. It is idempotent: while a sweeper is running, further calls do
// nothing. Stop it with StopAdminIndexSweeper before the database closes.
func StartAdminIndexSweeper(b *gotgbot.Bot) {
	adminIndexSweepMu.Lock()
	defer adminIndexSweepMu.Unlock()
	if adminIndexSweepCancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	adminIndexSweepCancel = cancel
	adminIndexSweepWG.Add(1)
	go func() {
		defer adminIndexSweepWG.Done()
		defer error_handling.RecoverFromPanic("adminIndexSweeper", "AdminIndex")
		adminIndexSweepLoop(ctx, b)
	}()
}

// StopAdminIndexSweeper cancels the sweeper and waits for it to finish, so no sweep is touching
// the database when shutdown closes it. It is safe to call when the sweeper is not running, and
// more than once.
func StopAdminIndexSweeper() {
	adminIndexSweepMu.Lock()
	cancel := adminIndexSweepCancel
	adminIndexSweepMu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	// Wait without holding the mutex (cf. StopStaffSweeper): a sweep that is mid-call must not
	// wedge shutdown behind a lock.
	adminIndexSweepWG.Wait()
	adminIndexSweepMu.Lock()
	adminIndexSweepCancel = nil
	adminIndexSweepMu.Unlock()
}

// adminIndexSweepLoop waits the jittered first delay, then runs a cycle every
// adminIndexSweepInterval until ctx is cancelled. The first cycle on an empty table is the
// backfill and the later ones are the sweep: one code path (D-05).
func adminIndexSweepLoop(ctx context.Context, b *gotgbot.Bot) {
	timer := time.NewTimer(adminIndexSweepFirstDelay())
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		adminIndexSweepCycle(ctx, b)
		timer.Reset(adminIndexSweepInterval)
	}
}

// adminIndexSweepCycle runs one sweep with its own panic recovery, so a panicking cycle is
// logged and the loop carries on to the next one.
func adminIndexSweepCycle(ctx context.Context, b *gotgbot.Bot) {
	defer error_handling.RecoverFromPanic("adminIndexSweepCycle", "AdminIndex")
	runAdminIndexSweep(ctx, b)
}

// adminIndexSweepPaceFunc waits adminIndexSweepPace before the next Telegram call. It returns
// false when ctx is cancelled first, so the caller stops promptly.
func adminIndexSweepPaceFunc(ctx context.Context) bool {
	if ctx.Err() != nil {
		return false
	}
	if adminIndexSweepPace <= 0 {
		return true
	}
	timer := time.NewTimer(adminIndexSweepPace)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// adminIndexSweepLockTTL is how long a replica holds the sweep lock: one minute short of the
// interval, so the next hourly cycle can always take it.
func adminIndexSweepLockTTL() time.Duration {
	ttl := adminIndexSweepInterval - time.Minute
	if ttl <= 0 {
		ttl = adminIndexSweepInterval / 2
	}
	return ttl
}

// acquireAdminIndexSweepLock reports whether this replica may run the current cycle. Without
// Redis, or on a Redis error, it returns true: the sweep stays correct unguarded because every
// roster write is idempotent (a replace of the whole roster or a clear), so correctness never
// depends on the lock.
func acquireAdminIndexSweepLock() bool {
	client := cache.GetRedisClient()
	if client == nil {
		return true
	}
	ok, err := client.SetNX(cache.Context, adminIndexSweepLockKey, adminIndexSweepOwner, adminIndexSweepLockTTL()).Result()
	if err != nil {
		log.Warnf("[AdminIndexSweeper] lock unavailable, sweeping unguarded: %v", err)
		return true
	}
	return ok
}

// adminIndexSweepUnlockScript deletes the lock only while it still holds the caller's owner.
const adminIndexSweepUnlockScript = `if redis.call("GET", KEYS[1]) == ARGV[1] then return redis.call("DEL", KEYS[1]) end return 0`

// releaseAdminIndexSweepLock gives the lock back by compare-and-delete on this process's owner
// token. It is called only when a cycle was cut off by a cancel, so a restarted replica's first
// cycle is not skipped for an hour; a finished cycle keeps the lock until its TTL. It runs on
// a context of its own, because the sweep's is already cancelled. Best effort.
func releaseAdminIndexSweepLock() {
	client := cache.GetRedisClient()
	if client == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Eval(ctx, adminIndexSweepUnlockScript, []string{adminIndexSweepLockKey}, adminIndexSweepOwner).Err(); err != nil {
		log.Warnf("[AdminIndexSweeper] lock release failed: %v", err)
	}
}

// adminIndexPacedClient paces and bounds every Telegram request of a sweep: each call waits
// the sweep pace first and then runs under adminIndexSweepCallTimeout, both derived from the
// sweep's context, so a cancel returns at once with no further request.
type adminIndexPacedClient struct {
	gotgbot.BotClient
}

// RequestWithContext implements gotgbot.BotClient.
func (c adminIndexPacedClient) RequestWithContext(ctx context.Context, token, method string, params map[string]any, opts *gotgbot.RequestOpts) (json.RawMessage, error) {
	if !adminIndexSweepPaceFunc(ctx) {
		return nil, ctx.Err()
	}
	callCtx, cancel := context.WithTimeout(ctx, adminIndexSweepCallTimeout)
	defer cancel()
	return c.BotClient.RequestWithContext(callCtx, token, method, params, opts)
}

// adminIndexSweepRetryAfter is how long to wait out a 429: Telegram's retry_after (at least one
// second), capped at adminIndexSweep429Max.
func adminIndexSweepRetryAfter(tgErr *gotgbot.TelegramError) time.Duration {
	wait := time.Second
	if tgErr.ResponseParams != nil && tgErr.ResponseParams.RetryAfter > 0 {
		wait = time.Duration(tgErr.ResponseParams.RetryAfter) * time.Second
	}
	if wait > adminIndexSweep429Max {
		wait = adminIndexSweep429Max
	}
	return wait
}

// adminIndexRateLimit returns the 429 inside err, if there is one.
func adminIndexRateLimit(err error) (*gotgbot.TelegramError, bool) {
	var tgErr *gotgbot.TelegramError
	if errors.As(err, &tgErr) && tgErr.Code == 429 {
		return tgErr, true
	}
	return nil, false
}

// runAdminIndexSweep refreshes chat_admin_index for every candidate chat (every negative chat
// ID in chats, inactive ones included), with no command run, so the private list works right
// after a deploy and an ownership or admin change Telegram never announced is caught (D-05).
//
// Only one replica runs a given cycle (acquireAdminIndexSweepLock); the others get Ran=false
// and make no Telegram call. Each chat is refreshed through refreshChatAdminIndex, which
// changes rows only on definitive facts, so an error, a timeout or a rate limit leaves a chat's
// rows as they were. Every Telegram request is paced and bounded (adminIndexPacedClient). A 429
// is waited out (capped) and that chat retried once; a second 429 ends the cycle with the
// remaining chats untouched. A cancel returns at once and gives the lock back.
func runAdminIndexSweep(ctx context.Context, b *gotgbot.Bot) adminIndexSweepReport {
	if hook := adminIndexSweepCycleHook; hook != nil {
		hook()
	}
	if !acquireAdminIndexSweepLock() {
		return adminIndexSweepReport{}
	}
	report := adminIndexSweepReport{Ran: true}

	// cutOff is set when a cancel ended the cycle early; only then is the lock given back.
	cutOff := false
	defer func() {
		if cutOff {
			releaseAdminIndexSweepLock()
		}
	}()

	chatIDs, err := adminindex.ListCandidateChatIDsFresh(ctx)
	if err != nil {
		if ctx.Err() != nil {
			cutOff = true
		}
		log.Errorf("[AdminIndexSweeper] candidate list failed: %v", err)
		return report
	}

	paced := *b
	paced.BotClient = adminIndexPacedClient{BotClient: b.BotClient}

	for _, chatID := range chatIDs {
		if ctx.Err() != nil {
			cutOff = true
			break
		}
		report.Chats++

		outcome, err := refreshChatAdminIndex(ctx, &paced, chatID)
		if tgErr, limited := adminIndexRateLimit(err); limited {
			if !adminIndexSweepWait(ctx, adminIndexSweepRetryAfter(tgErr)) {
				cutOff = true
				report.Unchanged++
				break
			}
			outcome, err = refreshChatAdminIndex(ctx, &paced, chatID)
			if _, again := adminIndexRateLimit(err); again {
				log.Warnf("[AdminIndexSweeper] rate limited twice at chat %d, ending the cycle", chatID)
				report.Unchanged++
				report.RateLimited = true
				break
			}
		}
		if ctx.Err() != nil {
			cutOff = true
		}

		switch outcome {
		case adminIndexReplaced:
			report.Replaced++
		case adminIndexCleared:
			report.Cleared++
		default:
			report.Unchanged++
		}
		if err != nil && ctx.Err() == nil {
			log.Debugf("[AdminIndexSweeper] chat %d: %v", chatID, err)
		}
		if cutOff {
			break
		}
	}

	log.Infof("[AdminIndexSweeper] cycle done: %d chat(s), %d replaced, %d cleared, %d unchanged, rate limited %t",
		report.Chats, report.Replaced, report.Cleared, report.Unchanged, report.RateLimited)
	return report
}

// adminIndexSweepWait sleeps d, returning false when ctx is cancelled first.
func adminIndexSweepWait(ctx context.Context, d time.Duration) bool {
	if ctx.Err() != nil {
		return false
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
