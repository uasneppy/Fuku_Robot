package modules

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/utils/error_handling"
)

// The deletion worker carries out the rows of scheduled_deletions (D-06). It runs on every
// replica: a tick plus a wake, every cycle under its own panic recovery. Every row resumes from
// the database, so a worker cut off at shutdown loses nothing.

var (
	// deletionWorkerTick is how often the worker looks for work when nothing woke it.
	deletionWorkerTick = time.Second
	// deletionBatch is how many due rows one cycle claims.
	deletionBatch = 200
	// deletionChunk is the most message IDs one deleteMessages call carries (Telegram's limit).
	deletionChunk = 100
	// deletionStaleClaim is how old a claim must be before it is taken to belong to a worker
	// that stopped. It is above deletionCycleGuard, so a live cycle is never double-processed.
	deletionStaleClaim = 2 * time.Minute
	// deletionCycleGuard is how long one cycle may run: past it every call is cut off and the
	// rows the cycle has not finished are released free, before the stale release can hand them
	// to another replica.
	deletionCycleGuard = 90 * time.Second
	// deletionMaxAttempts is how many counted failures a row is given before it is dropped.
	deletionMaxAttempts = 3
	// deletionDropAge is how old a message may be, by its own date, before its row is dropped
	// quietly: Telegram deletes messages for 48 hours only (D-07, D-20).
	deletionDropAge = 47 * time.Hour
	// deletionWorkerStopWait is the longest StopDeletionWorker waits for the worker (D-23).
	deletionWorkerStopWait = 3 * time.Second
)

// Lifecycle state of the worker goroutine. Each run has its own done channel rather than a
// shared WaitGroup, so a worker that was cut off at shutdown and is still finishing a call never
// shares state with the next one.
var (
	deletionWorkerMu     sync.Mutex
	deletionWorkerCancel context.CancelFunc
	deletionWorkerDone   chan struct{}
)

// StartDeletionWorker starts the background worker that deletes the queued messages whose time
// has come. It is idempotent: while a worker is running, further calls do nothing. Its context
// derives from the deletion root, so StopDeletionWorker ends the loop, every Telegram call of a
// cycle and every inline delete with one cancel.
func StartDeletionWorker(b *gotgbot.Bot) {
	deletionWorkerMu.Lock()
	defer deletionWorkerMu.Unlock()
	if deletionWorkerCancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(deletionContext())
	done := make(chan struct{})
	deletionWorkerCancel = cancel
	deletionWorkerDone = done
	go func() {
		defer close(done)
		defer error_handling.RecoverFromPanic("deletionWorker", "Deletion")
		deletionWorkerLoop(ctx, b)
	}()
}

// StopDeletionWorker cancels the deletion root and waits for the worker for at most
// deletionWorkerStopWait, so no cycle is still writing when shutdown closes the database. A call
// blocked in Telegram does not hold shutdown up: the root cancel ends it, and a row it was
// working on is put back or resumed after the 2 minute stale release. It is safe to call when the
// worker is not running, and more than once.
func StopDeletionWorker() {
	cancelDeletionContext()
	deletionWorkerMu.Lock()
	done := deletionWorkerDone
	deletionWorkerMu.Unlock()
	if done == nil {
		return
	}
	select {
	case <-done:
	case <-time.After(deletionWorkerStopWait):
		log.Warnf("[Deletion] worker did not stop within %s; every row resumes from the database", deletionWorkerStopWait)
	}
	deletionWorkerMu.Lock()
	if deletionWorkerDone == done {
		deletionWorkerCancel = nil
		deletionWorkerDone = nil
	}
	deletionWorkerMu.Unlock()
}

// deletionWorkerLoop runs one pass at start, which resumes whatever a restart left, then a pass
// on every tick or wake, until ctx is cancelled. A pass is cycles run back to back until one
// makes no progress.
func deletionWorkerLoop(ctx context.Context, b *gotgbot.Bot) {
	ticker := time.NewTicker(deletionWorkerTick)
	defer ticker.Stop()
	for {
		for ctx.Err() == nil && runDeletionCycle(ctx, b) {
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-deletionWake:
		}
	}
}

// deletionCycle is the state of one cycle: the rows it claimed and which of them are settled.
// A row that is not settled when the cycle ends is released free, so none is lost.
type deletionCycle struct {
	ctx   context.Context
	bot   *gotgbot.Bot
	token string

	rows     map[int64]models.ScheduledDeletion
	settled  map[int64]bool
	progress bool
	// cut is set when a call was shed, rate limited or cut off: the cycle ends and the rest of
	// its rows go back free.
	cut bool
}

// writeContext returns the context of a row write: detached from the run, so a shutdown that
// cancelled the run never strands a claim, and bounded on its own.
func (c *deletionCycle) writeContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(c.ctx), deletionRecordTimeout)
}

// complete removes the rows from the queue and marks them settled.
func (c *deletionCycle) complete(rows []models.ScheduledDeletion) {
	if len(rows) == 0 {
		return
	}
	ctx, cancel := c.writeContext()
	defer cancel()
	if err := deletion.Complete(ctx, c.token, deletionIDs(rows)); err != nil {
		log.Warnf("[Deletion] could not complete %d rows: %v", len(rows), err)
		return
	}
	c.settle(rows)
	c.progress = true
}

// release puts the rows back to pending: counted costs an attempt and the third counted failure
// drops the row, free costs nothing.
func (c *deletionCycle) release(rows []models.ScheduledDeletion, counted bool) {
	if len(rows) == 0 {
		return
	}
	ctx, cancel := c.writeContext()
	defer cancel()
	if err := deletion.Release(ctx, c.token, deletionIDs(rows), counted, deletionMaxAttempts); err != nil {
		log.Warnf("[Deletion] could not release %d rows: %v", len(rows), err)
		return
	}
	c.settle(rows)
}

func (c *deletionCycle) settle(rows []models.ScheduledDeletion) {
	for _, row := range rows {
		c.settled[row.ID] = true
	}
}

// unsettled returns the claimed rows nothing has settled yet, in claim order.
func (c *deletionCycle) unsettled() []models.ScheduledDeletion {
	var out []models.ScheduledDeletion
	for _, row := range c.rows {
		if !c.settled[row.ID] {
			out = append(out, row)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func deletionIDs(rows []models.ScheduledDeletion) []int64 {
	ids := make([]int64, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
	}
	return ids
}

func deletionMessageIDs(rows []models.ScheduledDeletion) []int64 {
	ids := make([]int64, len(rows))
	for i, row := range rows {
		ids[i] = row.MessageID
	}
	return ids
}

// runDeletionCycle does one round of work with its own panic recovery, so a panicking cycle is
// logged and the loop carries on. It reports whether any row was settled; a row only put back to
// be retried is not progress, so the loop waits for the next tick instead of spinning on it.
func runDeletionCycle(ctx context.Context, b *gotgbot.Bot) bool {
	defer error_handling.RecoverFromPanic("deletionCycle", "Deletion")
	// First of all, give back what a worker that stopped left claimed.
	if released, err := deletion.ReleaseStale(ctx, deletionNow().Add(-deletionStaleClaim)); err != nil {
		log.Warnf("[Deletion] could not release stale claims: %v", err)
	} else if released > 0 {
		log.Debugf("[Deletion] released %d stale claims", released)
	}
	token := deletion.NewClaimToken()
	claimed, err := deletion.ClaimDue(ctx, deletionNow(), deletionBatch, token)
	if err != nil {
		log.Warnf("[Deletion] could not claim due rows: %v", err)
		return false
	}
	if len(claimed) == 0 {
		return false
	}
	guardCtx, cancel := context.WithTimeout(ctx, deletionCycleGuard)
	defer cancel()
	c := &deletionCycle{
		ctx: guardCtx, bot: b, token: token,
		rows: make(map[int64]models.ScheduledDeletion, len(claimed)), settled: make(map[int64]bool, len(claimed)),
	}
	for _, row := range claimed {
		c.rows[row.ID] = row
	}
	// Whatever the cycle did not settle (a cut call, a panic, the guard) goes back free.
	defer func() { c.release(c.unsettled(), false) }()
	c.run(claimed)
	return c.progress
}

// run settles the claimed rows chat by chat in ascending chat order.
func (c *deletionCycle) run(claimed []models.ScheduledDeletion) {
	byChat := make(map[int64][]models.ScheduledDeletion)
	for _, row := range claimed {
		byChat[row.ChatID] = append(byChat[row.ChatID], row)
	}
	chats := make([]int64, 0, len(byChat))
	for chatID := range byChat {
		chats = append(chats, chatID)
	}
	sort.Slice(chats, func(i, j int) bool { return chats[i] < chats[j] })
	for _, chatID := range chats {
		if c.cut || c.ctx.Err() != nil {
			return
		}
		c.processChat(chatID, byChat[chatID])
	}
}

// processChat deletes one chat's claimed rows in chunks of at most deletionChunk IDs.
func (c *deletionCycle) processChat(chatID int64, rows []models.ScheduledDeletion) {
	deleted := 0
	for start := 0; start < len(rows) && !c.cut; start += deletionChunk {
		end := min(start+deletionChunk, len(rows))
		chunk := rows[start:end]
		err := deletionPaced(c.ctx, func(callCtx context.Context) error {
			_, callErr := c.bot.DeleteMessagesWithContext(callCtx, chatID, deletionMessageIDs(chunk), nil)
			return callErr
		})
		if err != nil {
			// Failure handling by class follows in the next task: until then a failed chunk goes
			// back free so no row is lost.
			c.cut = true
			return
		}
		c.complete(chunk)
		deleted += len(chunk)
	}
	log.Debugf("[Deletion] chat %d: %d deleted of %d claimed", chatID, deleted, len(rows))
}
