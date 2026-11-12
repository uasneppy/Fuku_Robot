package modules

import (
	"context"
	"sync"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db/staff"
	"github.com/uasneppy/fuku_robot/fuku/utils/error_handling"
	"github.com/uasneppy/fuku_robot/fuku/utils/ratelimit"
)

// The deletion core: the pacer every deletion Telegram call goes through, the inline delete of a
// message whose rule says "now", and the Staff Group read every deletion site shares. The queue,
// the latch and the one decision function (deletionAct) are added on top of it.

// deletionNow is the clock the deletion code reads. It is a variable only so a test can fix it,
// as raidNow is for the raid counters; production code never reassigns it.
var deletionNow = time.Now

// deletionPacer paces every Telegram call of the deletion code. Its slot reservation and
// retry_after block live in Redis, so every replica shares one budget, separate from the member
// requirements, staff and lockdown budgets (D-09). It is a variable so tests can install a fast one.
var deletionPacer = ratelimit.NewTelegramPacer(ratelimit.TelegramPacerOptions{
	NextKey:    "fuku:delete:pace:next",
	BlockKey:   "fuku:delete:pace:block",
	Interval:   100 * time.Millisecond,
	MaxRetries: 3,
	MaxWait:    60 * time.Second,
})

const (
	// deletionCallTimeout bounds one Telegram request of the deletion code.
	deletionCallTimeout = 10 * time.Second
	// deletionInlineTimeout bounds one inline delete, the wait for its pacer slot included.
	deletionInlineTimeout = 15 * time.Second
)

// deletionPaced runs one Telegram call under the deletion pacer: it waits for its slot, bounds
// the call and, on a 429, waits out Telegram's retry_after and repeats the same closure. A slot
// more than the pacer's MaxWait away fails at once as ratelimit.ErrRateLimited, with no request.
func deletionPaced(ctx context.Context, call func(context.Context) error) error {
	return deletionPacer.Do(ctx, func(callCtx context.Context) error {
		bounded, cancel := context.WithTimeout(callCtx, deletionCallTimeout)
		defer cancel()
		return call(bounded)
	})
}

// deletionRoot holds the context every deletion Telegram call derives from. The worker's stop
// cancels it, which aborts the paced calls of every inline delete in flight at once.
var deletionRoot struct {
	mu     sync.Mutex
	ctx    context.Context
	cancel context.CancelFunc
}

// deletionContext returns the root context of the deletion code, creating it on first use.
func deletionContext() context.Context {
	deletionRoot.mu.Lock()
	defer deletionRoot.mu.Unlock()
	if deletionRoot.ctx == nil {
		deletionRoot.ctx, deletionRoot.cancel = context.WithCancel(context.Background())
	}
	return deletionRoot.ctx
}

// cancelDeletionContext cancels the root context. The worker's stop calls it.
func cancelDeletionContext() {
	deletionContext()
	deletionRoot.mu.Lock()
	defer deletionRoot.mu.Unlock()
	deletionRoot.cancel()
}

// deletionStaffLookup reads the Staff Group of a chat fresh from the database. It is the one
// Staff Group read of the deletion code (the watcher, cleanService, the worker and the page) and
// a variable only so a test can fail it or make it answer.
var deletionStaffLookup = staff.GetStaffGroupFresh

// deletionInlineMax is the most inline deletes in flight at once; a variable so a test can
// shrink it.
var deletionInlineMax = 64

// deletionInline counts the inline deletes in flight. It is a counter and not a sync.WaitGroup
// because the stop gives up after its bounded wait while a new delete can still start.
var deletionInline raidPostTracker

// deletionDeleteNow starts one inline delete of a message on its own goroutine and reports
// whether it started. The goroutine is counted in deletionInline, recovers from a panic and
// derives its context from deletionContext(), so the worker's stop ends it.
func deletionDeleteNow(b *gotgbot.Bot, chatID, messageID int64) bool {
	if !deletionInline.startBelow(deletionInlineMax) {
		return false
	}
	go func() {
		defer deletionInline.finish()
		defer error_handling.RecoverFromPanic("deletionInline", "Deletion")
		ctx, cancel := context.WithTimeout(deletionContext(), deletionInlineTimeout)
		defer cancel()
		_ = deletionPaced(ctx, func(callCtx context.Context) error {
			_, err := b.DeleteMessageWithContext(callCtx, chatID, messageID, nil)
			return err
		})
	}()
	return true
}
