package modules

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/db/lockdown"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/utils/error_handling"
	"github.com/divkix/Alita_Robot/alita/utils/ratelimit"
)

// The lockdown worker makes every Telegram write of a lockdown that is not the
// answer to a command: it bans the joiners the guard recorded and, at a lift, unbans
// the lockdown's own bans. All of its state is in PostgreSQL (the joiner rows), every
// replica runs one, and every row is claimed with one conditional update, so a
// restart or a second replica only resumes the work.

// lockdownPacer paces every Telegram call of the lockdown worker. Its slot
// reservation and retry_after block live in Redis, so every replica shares one
// budget, separate from the staff fan-outs' budget. It is a variable so tests can
// install a fast one.
var lockdownPacer = ratelimit.NewTelegramPacer(ratelimit.TelegramPacerOptions{
	NextKey:    "alita:lockdown:pace:next",
	BlockKey:   "alita:lockdown:pace:block",
	Interval:   100 * time.Millisecond,
	MaxRetries: 3,
	MaxWait:    60 * time.Second,
})

// lockdownPaced runs one Telegram call under the lockdown pacer: it waits for its
// slot and, on a 429, waits out Telegram's retry_after and repeats the same closure.
// A slot more than MaxWait away fails at once as ratelimit.ErrRateLimited, with no
// Telegram request.
func lockdownPaced(ctx context.Context, call func(context.Context) error) error {
	return lockdownPacer.Do(ctx, call)
}

var (
	// lockdownWorkerTick is how often the worker looks for work when nothing woke it.
	lockdownWorkerTick = 2 * time.Second
	// lockdownWorkerStopWait is the longest StopLockdownWorker waits for the worker.
	// Every row is resumable from the database, so a worker cut off at shutdown loses
	// nothing.
	lockdownWorkerStopWait = 5 * time.Second
)

const (
	// lockdownWorkerBatch is how many rows one cycle reads for each kind of work.
	lockdownWorkerBatch = 50
	// lockdownMaxAttempts is how many failed Telegram calls a joiner row is given
	// before its failure is final.
	lockdownMaxAttempts = 3
)

// lockdownWake wakes the worker of this replica when its own guard records a joiner.
// Other replicas pick the work up at their next tick.
var lockdownWake = make(chan struct{}, 1)

// wakeLockdownWorker asks the worker to run a cycle now. It never blocks.
func wakeLockdownWorker() {
	select {
	case lockdownWake <- struct{}{}:
	default:
	}
}

// Lifecycle state of the worker goroutine.
var (
	lockdownWorkerMu     sync.Mutex
	lockdownWorkerCancel context.CancelFunc
	lockdownWorkerWG     sync.WaitGroup
)

// StartLockdownWorker starts the background worker that bans recorded joiners and
// finishes lifts. It is idempotent: while a worker is running, further calls do
// nothing. Stop it with StopLockdownWorker before the database closes.
func StartLockdownWorker(b *gotgbot.Bot) {
	lockdownWorkerMu.Lock()
	defer lockdownWorkerMu.Unlock()
	if lockdownWorkerCancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	lockdownWorkerCancel = cancel
	lockdownWorkerWG.Add(1)
	go func() {
		defer lockdownWorkerWG.Done()
		defer error_handling.RecoverFromPanic("lockdownWorker", "Lockdown")
		lockdownWorkerLoop(ctx, b)
	}()
}

// StopLockdownWorker cancels the worker and waits for it for at most
// lockdownWorkerStopWait, so no cycle is still writing when shutdown closes the
// database. It is safe to call when the worker is not running, and more than once.
func StopLockdownWorker() {
	lockdownWorkerMu.Lock()
	cancel := lockdownWorkerCancel
	lockdownWorkerMu.Unlock()
	if cancel == nil {
		return
	}
	cancel()

	// Wait without holding the mutex: a cycle that is mid-call must not wedge
	// shutdown behind a lock.
	done := make(chan struct{})
	go func() {
		defer error_handling.RecoverFromPanic("lockdownWorkerStop", "Lockdown")
		lockdownWorkerWG.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(lockdownWorkerStopWait):
		log.Warnf("[Lockdown] worker did not stop within %s; every row resumes from the database", lockdownWorkerStopWait)
	}

	lockdownWorkerMu.Lock()
	lockdownWorkerCancel = nil
	lockdownWorkerMu.Unlock()
}

// lockdownWorkerLoop runs one pass at start, which resumes whatever a restart left,
// then a pass on every tick or wake, until ctx is cancelled. A pass is cycles run
// back to back until one makes no progress.
func lockdownWorkerLoop(ctx context.Context, b *gotgbot.Bot) {
	ticker := time.NewTicker(lockdownWorkerTick)
	defer ticker.Stop()
	for {
		for ctx.Err() == nil && runLockdownCycle(ctx, b) {
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-lockdownWake:
		}
	}
}

// runLockdownCycle does one round of work with its own panic recovery, so a
// panicking cycle is logged and the loop carries on. It reports whether any row made
// progress; a row that was only put back to be retried (a rate limit, a failed call
// with tries left) is not progress, so the loop waits for the next tick instead of
// spinning on it.
func runLockdownCycle(ctx context.Context, b *gotgbot.Bot) bool {
	defer error_handling.RecoverFromPanic("lockdownCycle", "Lockdown")
	return lockdownBanPending(ctx, b)
}

// lockdownCallOutcome is how one Telegram call of the worker ended.
type lockdownCallOutcome int

const (
	// lockdownCallDone means Telegram answered success.
	lockdownCallDone lockdownCallOutcome = iota
	// lockdownCallRetryFree means the row goes back without costing an attempt: the
	// pacer refused (rate limited) or the worker is shutting down.
	lockdownCallRetryFree
	// lockdownCallFailed means Telegram refused or the call failed.
	lockdownCallFailed
)

// lockdownClassify sorts the error of a paced call.
func lockdownClassify(ctx context.Context, err error) lockdownCallOutcome {
	switch {
	case err == nil:
		return lockdownCallDone
	case errors.Is(err, ratelimit.ErrRateLimited), ctx.Err() != nil:
		return lockdownCallRetryFree
	}
	return lockdownCallFailed
}

// lockdownBanPending bans the pending joiners of confirmed active lockdowns, oldest
// first. Each row is claimed first, so one replica bans a joiner exactly once; the
// ban ends on the row's ban_until, which marks it as the lockdown's own.
func lockdownBanPending(ctx context.Context, b *gotgbot.Bot) bool {
	rows, err := lockdown.ListPendingFresh(lockdownWorkerBatch)
	if err != nil {
		log.Errorf("[Lockdown] worker could not list pending joiners: %v", err)
		return false
	}
	progress := false
	for _, row := range rows {
		if ctx.Err() != nil {
			break
		}
		won, err := lockdown.ClaimJoiner(row.ID, models.JoinerStatePending, models.JoinerStateActing)
		if err != nil || !won {
			continue
		}
		if lockdownBanOne(ctx, b, row) {
			progress = true
		}
	}
	return progress
}

// lockdownBanOne bans one claimed joiner and records the result. It reports whether
// the row reached a final state for this stage.
func lockdownBanOne(ctx context.Context, b *gotgbot.Bot, row models.LockdownJoiner) bool {
	err := lockdownPaced(ctx, func(callCtx context.Context) error {
		call, cancel := context.WithTimeout(callCtx, lockdownCallTimeout)
		defer cancel()
		_, banErr := b.BanChatMemberWithContext(call, row.ChatID, row.UserID, &gotgbot.BanChatMemberOpts{UntilDate: row.BanUntil})
		return banErr
	})

	switch lockdownClassify(ctx, err) {
	case lockdownCallDone:
		moveLockdownJoiner(row.ID, models.JoinerStateActing, models.JoinerStateBanned, "", false)
		return true
	case lockdownCallRetryFree:
		moveLockdownJoiner(row.ID, models.JoinerStateActing, models.JoinerStatePending, "", false)
		return false
	}

	log.Warnf("[Lockdown] ban of user %d in chat %d failed: %v", row.UserID, row.ChatID, err)
	if row.Attempts+1 >= lockdownMaxAttempts {
		moveLockdownJoiner(row.ID, models.JoinerStateActing, models.JoinerStateBanFailed, telegramErrorDetail(err), true)
		return true
	}
	moveLockdownJoiner(row.ID, models.JoinerStateActing, models.JoinerStatePending, telegramErrorDetail(err), true)
	return false
}

// moveLockdownJoiner moves a row and logs a failed write; the row then stays in its
// claimed state until a later plan's stale-claim recovery returns it.
func moveLockdownJoiner(id uint, from, to, detail string, countAttempt bool) {
	if _, err := lockdown.MoveJoiner(id, from, to, detail, countAttempt); err != nil {
		log.Errorf("[Lockdown] joiner row %d could not move from %s to %s: %v", id, from, to, err)
	}
}
