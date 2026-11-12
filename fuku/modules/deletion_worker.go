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
	// deletionBatch is how many due rows one cycle claims. A 1000-row claim costs about 0.1 ms
	// with 520,000 pending rows (the due index serves it) and stays far under the 90 s cycle guard
	// and the 2 minute stale claim; each chat still gets one paced call per chunk of 100 rows
	// (D-32).
	deletionBatch = 1000
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
	// deletionRetryBackoff is how long a row waits after a counted failure (a 5xx, a transport
	// error or a per-call timeout) before it is due again, so the three counted tries are spaced
	// instead of falling in three consecutive ticks (WR-02).
	deletionRetryBackoff = 60 * time.Second
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
	// The self-destruct batcher lives and stops with the worker (D-19).
	memberDeleteStart()
}

// StopDeletionWorker cancels the deletion root and waits for the worker for at most
// deletionWorkerStopWait, so no cycle is still writing when shutdown closes the database. A call
// blocked in Telegram does not hold shutdown up: the root cancel ends it, and a row it was
// working on is put back or resumed after the 2 minute stale release. It is safe to call when the
// worker is not running, and more than once.
//
// It also ends the member self-destruct batcher (D-08, D-19): later offers are shed, the batcher
// runs one final flush bounded to 2 s on a context detached from the cancelled root, and the worker
// and the batcher are waited for under the one deletionWorkerStopWait timer, so the stop adds no
// allowance and no drain. A flush cut off loses only entries that were never queued, whose
// messages stay.
func StopDeletionWorker() {
	cancelDeletionContext()
	batcherDone := memberDeleteStop()
	deletionWorkerMu.Lock()
	done := deletionWorkerDone
	deletionWorkerMu.Unlock()
	if done == nil && batcherDone == nil {
		return
	}
	deadline := time.NewTimer(deletionWorkerStopWait)
	defer deadline.Stop()
	expired := false
	for _, w := range []struct {
		name string
		ch   <-chan struct{}
	}{{"worker", done}, {"self-destruct batcher", batcherDone}} {
		if w.ch == nil || expired {
			continue
		}
		select {
		case <-w.ch:
		case <-deadline.C:
			expired = true
			log.Warnf("[Deletion] %s did not stop within %s; every row resumes from the database", w.name, deletionWorkerStopWait)
		}
	}
	if done == nil {
		return
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
	if err := deletion.Release(ctx, c.token, deletionIDs(rows), counted, deletionMaxAttempts,
		deletionNow().Add(deletionRetryBackoff)); err != nil {
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

// run drops the rows whose message is too old, then settles the rest chat by chat in ascending
// chat order.
func (c *deletionCycle) run(claimed []models.ScheduledDeletion) {
	oldest := deletionNow().Add(-deletionDropAge)
	byChat := make(map[int64][]models.ScheduledDeletion)
	var expired []models.ScheduledDeletion
	for _, row := range claimed {
		if row.SentAt.Before(oldest) {
			expired = append(expired, row)
			continue
		}
		byChat[row.ChatID] = append(byChat[row.ChatID], row)
	}
	// A message older than Telegram's window cannot be deleted: its row is dropped quietly.
	c.complete(expired)
	chats := make([]int64, 0, len(byChat))
	for chatID := range byChat {
		chats = append(chats, chatID)
	}
	sort.Slice(chats, func(i, j int) bool { return chats[i] < chats[j] })
	for _, chatID := range chats {
		if c.cut || c.ctx.Err() != nil {
			return
		}
		(&deletionChatRun{cycle: c, chatID: chatID, rows: byChat[chatID]}).process()
	}
}

// deletionChatRun is one chat's share of a cycle: its claimed rows and what has happened to them.
type deletionChatRun struct {
	cycle  *deletionCycle
	chatID int64
	rows   []models.ScheduledDeletion
	// decided is set once the service latch was decided in this cycle: at most one live right
	// check per chat per cycle.
	decided bool
	// stop is set when the chat is gone: nothing more is sent to it this cycle.
	stop                      bool
	deleted, dropped, putBack int
}

// halted reports whether this chat's run must send nothing more.
func (r *deletionChatRun) halted() bool {
	return r.stop || r.cycle.cut || r.cycle.ctx.Err() != nil
}

// done completes rows Telegram deleted.
func (r *deletionChatRun) done(rows ...models.ScheduledDeletion) {
	r.cycle.complete(rows)
	r.deleted += len(rows)
}

// drop completes rows that need no call, quietly.
func (r *deletionChatRun) drop(rows ...models.ScheduledDeletion) {
	r.cycle.complete(rows)
	r.dropped += len(rows)
}

// process screens the chat's rows (latch, Staff Group, the self-destruct rule) and then deletes them
// in chunks.
func (r *deletionChatRun) process() {
	rows := r.screenLatch(r.rows)
	if len(rows) == 0 {
		return
	}
	group, err := deletionStaffLookup(r.chatID)
	if err != nil {
		// A read that fails releases the rows free and sends nothing: a chat that cannot be told
		// apart from a Staff Group is never touched.
		log.Warnf("[Deletion] could not check chat %d for a Staff Group: %v", r.chatID, err)
		r.cycle.release(rows, false)
		return
	}
	if group != nil {
		r.drop(rows...)
		return
	}
	rows = r.screenSelfDestruct(rows)
	if len(rows) == 0 {
		return
	}
	for start := 0; start < len(rows) && !r.halted(); start += deletionChunk {
		r.chunk(r.pendingOf(rows[start:min(start+deletionChunk, len(rows))]))
	}
	log.Debugf("[Deletion] chat %d: %d deleted, %d dropped, %d put back of %d claimed",
		r.chatID, r.deleted, r.dropped, r.putBack, len(r.rows))
}

// deletionScreenWarn lets the worker's "rules unreadable" line through once per chat per hour.
var deletionScreenWarn = memberDeleteWarnGate{label: "the self-destruct screen log"}

// screenSelfDestruct re-checks the chat's claimed member:selfdestruct rows against the chat's current
// rule, with one cached rules read per chat per cycle, and returns the rows that may be deleted.
// A row this cycle has not yet claimed and screened is never deleted after the owner turned the
// rule off (D-26b): a row queued before the switch, by a batch that raced it or by another replica,
// is completed without a call when the rule is absent or holds a value the kind does not accept,
// and is put back free, with no attempt counted, when the rules cannot be read (unknown never
// deletes, D-33). A batch already claimed and screened before the switch can finish within its
// cycle, up to the 90 s guard (SEC-17-05, like Phase 16's accepted NEW-01). Rows of every other
// kind pass untouched.
func (r *deletionChatRun) screenSelfDestruct(rows []models.ScheduledDeletion) []models.ScheduledDeletion {
	var selfDestruct, rest []models.ScheduledDeletion
	for _, row := range rows {
		if row.Kind == deletion.KindMemberSelfDestruct {
			selfDestruct = append(selfDestruct, row)
		} else {
			rest = append(rest, row)
		}
	}
	if len(selfDestruct) == 0 {
		return rows
	}
	rules, err := deletionRulesLookup(r.cycle.ctx, r.chatID)
	if err != nil || rules == nil {
		// An uncounted release leaves delete_at alone, so the rows are claimed again on the next tick:
		// the line goes through an hourly per-chat gate (IN-01), or a rules fault would log once a
		// second per chat. The release itself is unchanged.
		if deletionScreenWarn.allow(r.chatID, deletionNow()) {
			log.Warnf("[Deletion] could not read the rules of chat %d to screen self-destruct rows: %v", r.chatID, err)
		}
		r.cycle.release(selfDestruct, false)
		return rest
	}
	delay, present := rules.Delays[deletion.KindMemberSelfDestruct]
	if !present || deletion.ValidateRuleDelay(deletion.KindMemberSelfDestruct, delay) != nil {
		r.drop(selfDestruct...)
		return rest
	}
	return rows
}

// screenLatch drops the rows of a kind the chat's latch stops and returns the rest.
func (r *deletionChatRun) screenLatch(rows []models.ScheduledDeletion) []models.ScheduledDeletion {
	latched := make(map[string]bool)
	var keep, stopped []models.ScheduledDeletion
	for _, row := range rows {
		stop, known := latched[row.Kind]
		if !known {
			stop = deletionLatched(r.cycle.ctx, r.chatID, row.Kind)
			latched[row.Kind] = stop
		}
		if stop {
			stopped = append(stopped, row)
		} else {
			keep = append(keep, row)
		}
	}
	r.drop(stopped...)
	return keep
}

// pendingOf returns the rows of the chunk that nothing has settled since the chunk was cut.
func (r *deletionChatRun) pendingOf(rows []models.ScheduledDeletion) []models.ScheduledDeletion {
	var out []models.ScheduledDeletion
	for _, row := range rows {
		if !r.cycle.settled[row.ID] {
			out = append(out, row)
		}
	}
	return out
}

// chunk deletes up to deletionChunk messages with one call and classifies its answer.
func (r *deletionChatRun) chunk(rows []models.ScheduledDeletion) {
	if len(rows) == 0 {
		return
	}
	err := deletionPaced(r.cycle.ctx, func(callCtx context.Context) error {
		_, callErr := r.cycle.bot.DeleteMessagesWithContext(callCtx, r.chatID, deletionMessageIDs(rows), nil)
		return callErr
	})
	r.settleChunk(rows, err)
}

// settleChunk acts on the answer of a batch call. A batch that fails for a reason of its own falls
// back to one call per message, so the one undeletable ID is isolated.
func (r *deletionChatRun) settleChunk(rows []models.ScheduledDeletion, err error) {
	switch deletionClassify(r.cycle.ctx, err) {
	case deletionVerdictDone:
		r.done(rows...)
	case deletionVerdictRetryFree:
		r.cycle.cut = true
	case deletionVerdictGone:
		r.gone()
	case deletionVerdictRefused, deletionVerdictRetry:
		r.fallBack(rows)
	}
}

// fallBack deletes the messages one by one and classifies each answer on its own.
func (r *deletionChatRun) fallBack(rows []models.ScheduledDeletion) {
	for _, row := range rows {
		if r.halted() {
			return
		}
		if r.cycle.settled[row.ID] {
			continue
		}
		err := deletionPaced(r.cycle.ctx, func(callCtx context.Context) error {
			_, callErr := r.cycle.bot.DeleteMessageWithContext(callCtx, r.chatID, row.MessageID, nil)
			return callErr
		})
		r.settleOne(row, err)
	}
}

// settleOne acts on the answer of one message's own call.
func (r *deletionChatRun) settleOne(row models.ScheduledDeletion, err error) {
	switch deletionClassify(r.cycle.ctx, err) {
	case deletionVerdictDone:
		r.done(row)
	case deletionVerdictRefused:
		r.drop(row)
		r.maybeLatchService(row)
	case deletionVerdictRetryFree:
		r.cycle.cut = true
	case deletionVerdictGone:
		r.gone()
	case deletionVerdictRetry:
		r.cycle.release([]models.ScheduledDeletion{row}, true)
		r.putBack++
	}
}

// maybeLatchService decides the service latch after a message of a kind that needs the delete right
// (service or member, deletion.NeedsDeleteRight) was refused: at most once per chat per cycle, and
// never for a bot kind, whose own messages a plain member bot can delete. When the live check shows
// the bot cannot delete, the chat's other claimed rows of those kinds go too.
func (r *deletionChatRun) maybeLatchService(row models.ScheduledDeletion) {
	if r.decided || !deletion.NeedsDeleteRight(row.Kind) {
		return
	}
	r.decided = true
	if deletionDecideServiceLatch(r.cycle.ctx, r.cycle.bot, r.chatID) != deletionRightCannot {
		return
	}
	var stopped []models.ScheduledDeletion
	for _, other := range r.cycle.unsettled() {
		if other.ChatID == r.chatID && deletion.NeedsDeleteRight(other.Kind) {
			stopped = append(stopped, other)
		}
	}
	r.drop(stopped...)
}

// gone latches a chat the bot cannot act in at all and drops its claimed and pending rows.
func (r *deletionChatRun) gone() {
	r.stop = true
	ctx, cancel := r.cycle.writeContext()
	defer cancel()
	deletionSetLatch(ctx, r.chatID, deletionScopeGone)
	if _, err := deletion.DropChatPending(ctx, r.chatID, ""); err != nil {
		log.Warnf("[Deletion] could not drop the pending rows of chat %d: %v", r.chatID, err)
	}
	var claimed []models.ScheduledDeletion
	for _, row := range r.cycle.unsettled() {
		if row.ChatID == r.chatID {
			claimed = append(claimed, row)
		}
	}
	r.drop(claimed...)
}
