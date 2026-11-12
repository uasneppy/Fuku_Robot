package modules

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/db/staff"
	"github.com/uasneppy/fuku_robot/fuku/utils/chat_status"
	"github.com/uasneppy/fuku_robot/fuku/utils/error_handling"
	fukuerrors "github.com/uasneppy/fuku_robot/fuku/utils/errors"
	"github.com/uasneppy/fuku_robot/fuku/utils/ratelimit"
)

// The deletion core: the pacer every deletion Telegram call goes through, the inline delete of a
// message whose rule says "now", the classifier of a delete's answer, the negative latch of a chat
// the bot cannot act in, the queue writer and deletionAct, the one function that decides what
// happens to a message that has a delay.

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

// deletionCallTimeout bounds one Telegram request of the deletion code.
const deletionCallTimeout = 10 * time.Second

// deletionInlineTimeout bounds one inline delete, the wait for its pacer slot included. It is a
// variable only so a test can shorten it; production code never reassigns it.
var deletionInlineTimeout = 15 * time.Second

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

// deletionIsStaffChat reports whether chatID is a Staff Group. A read that fails counts as one:
// nothing is deleted or scheduled in a chat that cannot be told apart from a Staff Group.
func deletionIsStaffChat(chatID int64) bool {
	group, err := deletionStaffLookup(chatID)
	if err != nil {
		log.Warnf("[Deletion] could not check chat %d for a Staff Group: %v", chatID, err)
		return true
	}
	return group != nil
}

// deletionInlineMax is the most inline deletes in flight at once; a variable so a test can
// shrink it. A call beyond it takes the durable path instead of parking another goroutine.
var deletionInlineMax = 64

// deletionInline counts the inline deletes in flight. It is a counter and not a sync.WaitGroup
// because the stop gives up after its bounded wait while a new delete can still start.
var deletionInline raidPostTracker

// deletionVerdict is how deletionClassify reads the answer of a delete call.
type deletionVerdict int

const (
	// deletionVerdictDone means the call succeeded.
	deletionVerdictDone deletionVerdict = iota
	// deletionVerdictRetryFree means the call was shed, rate limited or cut off by a shutdown:
	// the message is not at fault, so no attempt is counted.
	deletionVerdictRetryFree
	// deletionVerdictGone means the bot cannot act in the chat at all (it is not in it, was
	// kicked, or the chat is gone or was upgraded): the chat is latched for every kind.
	deletionVerdictGone
	// deletionVerdictRefused means Telegram refused this one message: it is dropped quietly,
	// with no log line and no latch.
	deletionVerdictRefused
	// deletionVerdictRetry means a failure that may pass (a 5xx, a timeout, a transport error).
	deletionVerdictRetry
)

// deletionGoneFragments are the lower-cased fragments of a description that say the chat is
// unreachable for good. Only the first three have production history in this repository; the
// rest are assumptions and the code never depends on one of them being exact (an unmatched
// description falls to refused or retry).
var deletionGoneFragments = []string{
	"group chat was deactivated", "chat_not_found", "peer_id_invalid", "upgraded to a supergroup",
}

// deletionRefusedFragments are the lower-cased fragments of a description that refuse one message.
var deletionRefusedFragments = []string{
	"message to delete not found", "message can't be deleted", "message_id_invalid",
}

// deletionTelegramText returns the lower-cased description of a Telegram error, or "".
func deletionTelegramText(err error) string {
	var tgErr *gotgbot.TelegramError
	if errors.As(err, &tgErr) && tgErr != nil {
		return strings.ToLower(tgErr.Description)
	}
	return ""
}

// deletionHasFragment reports whether the description of err holds one of the fragments.
func deletionHasFragment(err error, fragments []string) bool {
	text := deletionTelegramText(err)
	for _, fragment := range fragments {
		if strings.Contains(text, fragment) {
			return true
		}
	}
	return false
}

// deletionIsRetryFree: a shed or rate-limited call, a cancelled context, a 429.
func deletionIsRetryFree(ctx context.Context, err error) bool {
	if errors.Is(err, ratelimit.ErrRateLimited) || errors.Is(err, context.Canceled) || ctx.Err() != nil {
		return true
	}
	var tgErr *gotgbot.TelegramError
	return errors.As(err, &tgErr) && tgErr != nil && tgErr.Code == 429
}

// deletionIsGone: the bot is not in the chat, the chat is gone, or it migrated.
func deletionIsGone(_ context.Context, err error) bool {
	if chat_status.IsBotMissingError(err) || deletionHasFragment(err, deletionGoneFragments) {
		return true
	}
	var tgErr *gotgbot.TelegramError
	return errors.As(err, &tgErr) && tgErr != nil && tgErr.ResponseParams != nil && tgErr.ResponseParams.MigrateToChatId != 0
}

// deletionIsRefused: this one message cannot be deleted, or any other client error.
func deletionIsRefused(_ context.Context, err error) bool {
	return deletionHasFragment(err, deletionRefusedFragments) || isTelegramClientError(err)
}

// deletionClassRules is the classifier's table, tried in order: a shed or cancelled call is
// never read as a refusal, and a gone chat is never read as a refused message.
var deletionClassRules = []struct {
	verdict deletionVerdict
	match   func(context.Context, error) bool
}{
	{deletionVerdictRetryFree, deletionIsRetryFree},
	{deletionVerdictGone, deletionIsGone},
	{deletionVerdictRefused, deletionIsRefused},
}

// deletionClassify reads the answer of one delete call. It matches on lower-cased fragments and
// on codes, never on one string, and everything it cannot place is a retry.
func deletionClassify(ctx context.Context, err error) deletionVerdict {
	if err == nil {
		return deletionVerdictDone
	}
	for _, rule := range deletionClassRules {
		if rule.match(ctx, err) {
			return rule.verdict
		}
	}
	return deletionVerdictRetry
}

const (
	// deletionLatchTTL is how long the negative latch of a chat lasts.
	deletionLatchTTL = 10 * time.Minute
	// deletionScopeGone is the latch value that stops every kind: the bot is not in the chat.
	deletionScopeGone = "gone"
	// deletionScopeService is the latch value that stops the kinds that need the delete right
	// (service:* and member:*, never bot:*): the bot cannot delete other people's messages there.
	// The value keeps its name from Phase 16. It is set only after a live right check, never from
	// a description (a message-level refusal says nothing about the right).
	deletionScopeService = "service"
)

// deletionRedisClient is the operational Redis client of the latch, nil without Redis. A
// variable so a test can take Redis away.
var deletionRedisClient = raidRedis

// deletionLatchKey is the Redis key of a chat's negative latch. It is outside the fuku:cache:
// prefix, so CLEAR_CACHE_ON_STARTUP keeps it.
func deletionLatchKey(chatID int64) string {
	return "fuku:delete:norights:" + strconv.FormatInt(chatID, 10)
}

// deletionLatched reports whether deletions of the kind are stopped in the chat. The service scope
// means "the bot cannot delete other people's messages", so it stops the kinds that need that right
// (service and member kinds, deletion.NeedsDeleteRight) and never a bot kind. One Redis read;
// no Redis and a Redis error are "not latched": the latch only saves calls, correctness never
// depends on it.
func deletionLatched(ctx context.Context, chatID int64, kind string) bool {
	client := deletionRedisClient()
	if client == nil {
		return false
	}
	readCtx, cancel := context.WithTimeout(ctx, raidRedisTimeout)
	defer cancel()
	scope, err := client.Get(readCtx, deletionLatchKey(chatID)).Result()
	if err != nil {
		return false
	}
	switch scope {
	case deletionScopeGone:
		return true
	case deletionScopeService:
		return deletion.NeedsDeleteRight(kind)
	}
	return false
}

// deletionSetLatch latches the chat for deletionLatchTTL. The gone scope overwrites; the service
// scope is SET NX, so it never downgrades a gone latch. A Redis error is only logged, by chat ID.
func deletionSetLatch(ctx context.Context, chatID int64, scope string) {
	client := deletionRedisClient()
	if client == nil {
		return
	}
	setCtx, cancel := context.WithTimeout(ctx, raidRedisTimeout)
	defer cancel()
	var err error
	if scope == deletionScopeService {
		err = client.SetNX(setCtx, deletionLatchKey(chatID), scope, deletionLatchTTL).Err()
	} else {
		err = client.Set(setCtx, deletionLatchKey(chatID), scope, deletionLatchTTL).Err()
	}
	if err != nil {
		log.Warnf("[Deletion] could not latch chat %d: %v", chatID, err)
	}
}

// deletionWake wakes the deletion worker for a row due now. It holds one pending wake, so a
// send never blocks and is safe with no reader.
var deletionWake = make(chan struct{}, 1)

// wakeDeletionWorker asks the worker for a pass at once.
func wakeDeletionWorker() {
	select {
	case deletionWake <- struct{}{}:
	default:
	}
}

// deletionSentAt is the time a message was sent: its own date as UTC, or now when the date is
// zero, negative or in the future (clock skew must never stretch a message past its delay).
func deletionSentAt(date int64) time.Time {
	now := deletionNow().UTC()
	if date <= 0 {
		return now
	}
	sent := time.Unix(date, 0).UTC()
	if sent.After(now) {
		return now
	}
	return sent
}

// deletionQueueWrite is the repository's queue writer. It is a variable only so a test can make
// a write fail; deletionSchedule is the only caller in this package.
var deletionQueueWrite = deletion.Schedule

// deletionSchedule queues one message for deletion and reports whether a row was written. It
// refuses a delay outside 0..86400 itself, writes nothing for a latched chat, and wakes the
// worker when the row is due now. The queue writer refuses a Staff Group again.
func deletionSchedule(ctx context.Context, e deletion.Entry) (bool, error) {
	if e.DelaySecs < 0 || e.DelaySecs > deletion.MaxDelaySecs {
		return false, fukuerrors.Wrapf(deletion.ErrDelayOutOfRange, "schedule a deletion in chat %d", e.ChatID)
	}
	if deletionLatched(ctx, e.ChatID, e.Kind) {
		return false, nil
	}
	written, err := deletionQueueWrite(ctx, e)
	if err != nil || !written {
		return false, err
	}
	if !e.SentAt.Add(time.Duration(e.DelaySecs) * time.Second).After(deletionNow()) {
		wakeDeletionWorker()
	}
	return true, nil
}

// deletionRecordTimeout bounds one queue write of the deletion code. The writes never use a
// request or run context that a shutdown cancels.
const deletionRecordTimeout = 5 * time.Second

// deletionAction is one message with a delay: where it is, what it is and when it was sent.
type deletionAction struct {
	ChatID    int64
	MessageID int64
	Kind      string
	SentAt    time.Time
	Delay     int
}

// deletionQueue writes the action to the queue and logs a failure by chat ID only.
func deletionQueue(a deletionAction) {
	ctx, cancel := context.WithTimeout(context.Background(), deletionRecordTimeout)
	defer cancel()
	_, err := deletionSchedule(ctx, deletion.Entry{
		ChatID: a.ChatID, MessageID: a.MessageID, Kind: a.Kind, SentAt: a.SentAt, DelaySecs: a.Delay,
	})
	if err != nil {
		log.Warnf("[Deletion] could not queue a deletion in chat %d: %v", a.ChatID, err)
	}
}

// deletionAct is the one place that decides what happens to a message with a delay: a chat ID that
// is not a group's (zero or positive) is refused before any read or write, on the inline branch
// as on the queue branch (SEC-05, symmetric with deletion.Schedule), a latched
// chat is left alone, a Staff Group (or one that cannot be told apart from one) is never touched,
// a delay of 0 is an inline delete, and any other delay is a queue row at the message's own time
// plus the delay. An inline delete the cap or the pacer refuses becomes a row due now.
func deletionAct(b *gotgbot.Bot, a deletionAction) {
	if a.ChatID >= 0 {
		return
	}
	latchCtx, cancel := context.WithTimeout(context.Background(), raidRedisTimeout)
	latched := deletionLatched(latchCtx, a.ChatID, a.Kind)
	cancel()
	if latched || deletionIsStaffChat(a.ChatID) {
		return
	}
	if a.Delay != 0 {
		deletionQueue(a)
		return
	}
	if !deletionDeleteNow(b, a) {
		deletionQueue(a)
	}
}

// deletionDeleteNow starts one inline delete of a message on its own goroutine and reports
// whether it started; false means the cap of deletionInlineMax is reached or the root was
// cancelled by the worker's stop, and the caller takes the durable path. The goroutine is counted in deletionInline, recovers from a panic and
// derives its context from deletionContext(), so the worker's stop ends it. It never retries: a
// gone answer latches the chat, a refused one is dropped quietly, a shed delete becomes a row
// due now, and any other failure loses that one message (D-08).
func deletionDeleteNow(b *gotgbot.Bot, a deletionAction) bool {
	if deletionContext().Err() != nil {
		// The worker was stopped: no goroutine starts, the durable row resumes after a restart.
		return false
	}
	if !deletionInline.startBelow(deletionInlineMax) {
		return false
	}
	go func() {
		defer deletionInline.finish()
		defer error_handling.RecoverFromPanic("deletionInline", "Deletion")
		ctx, cancel := context.WithTimeout(deletionContext(), deletionInlineTimeout)
		defer cancel()
		err := deletionPaced(ctx, func(callCtx context.Context) error {
			_, callErr := b.DeleteMessageWithContext(callCtx, a.ChatID, a.MessageID, nil)
			return callErr
		})
		deletionInlineSettle(ctx, b, a, err)
	}()
	return true
}

// deletionInlineSettle acts on the answer of an inline delete.
func deletionInlineSettle(ctx context.Context, b *gotgbot.Bot, a deletionAction, err error) {
	switch deletionClassify(ctx, err) {
	case deletionVerdictGone:
		recordCtx, cancel := context.WithTimeout(context.Background(), deletionRecordTimeout)
		defer cancel()
		deletionSetLatch(recordCtx, a.ChatID, deletionScopeGone)
	case deletionVerdictRefused:
		deletionInlineDecideLatch(b, a)
	case deletionVerdictRetryFree:
		if deletionInlineShed(ctx, err) {
			deletionQueue(a)
		}
	}
}

// deletionCheckTTL is how long the guard of the inline latch decision lasts: the live right check
// of a chat is made at most once in that time (SEC-02).
const deletionCheckTTL = 10 * time.Minute

// deletionCheckKey is the Redis key of a chat's inline latch decision guard. It is outside the
// fuku:cache: prefix, so CLEAR_CACHE_ON_STARTUP keeps it.
func deletionCheckKey(chatID int64) string {
	return "fuku:delete:chk:" + strconv.FormatInt(chatID, 10)
}

// deletionTakeCheck takes the guard of chatID with SET NX EX and reports whether this caller may
// make the live right check. No Redis, a Redis error and a lost SET NX all answer false: the latch
// only saves calls, so the guard fails quiet and never turns into an unbounded loop of checks.
func deletionTakeCheck(ctx context.Context, chatID int64) bool {
	client := deletionRedisClient()
	if client == nil {
		return false
	}
	setCtx, cancel := context.WithTimeout(ctx, raidRedisTimeout)
	defer cancel()
	taken, err := client.SetNX(setCtx, deletionCheckKey(chatID), "1", deletionCheckTTL).Result()
	return err == nil && taken
}

// deletionInlineDecideLatch decides the service latch after an inline delete of a kind that needs the
// delete right (service and member kinds) was refused with a message-level answer, which says
// nothing about the right on its own (SEC-02): through the same live check the worker makes
// (deletionDecideServiceLatch), at most once per chat per deletionCheckTTL, so a group where the
// bot can never delete costs one read and then no calls. A bot kind never decides it (a plain-member bot can delete its own messages). It runs inside the
// inline delete's goroutine, which already recovers from a panic, on a context derived from the
// deletion root, so the worker's stop cancels it; only the classifier's live cannot answer writes
// the latch.
func deletionInlineDecideLatch(b *gotgbot.Bot, a deletionAction) {
	if !deletion.NeedsDeleteRight(a.Kind) {
		return
	}
	ctx, cancel := context.WithTimeout(deletionContext(), deletionInlineTimeout)
	defer cancel()
	if !deletionTakeCheck(ctx, a.ChatID) {
		return
	}
	deletionDecideServiceLatch(ctx, b, a.ChatID)
}

// deletionInlineShed reports whether a retry-free inline answer means the delete never happened
// for a reason of that moment, so the message takes the durable path as a row due now: the pacer
// shed it, or the inline delete's own deadline ran out inside the pacer's wait while the deletion
// root is still live (WR-01: a flood-control wait longer than deletionInlineTimeout). A cancelled
// root is a shutdown: no row is written from a goroutine whose database may be closing, and the
// message is lost as before.
func deletionInlineShed(ctx context.Context, err error) bool {
	if errors.Is(err, ratelimit.ErrRateLimited) {
		return true
	}
	return errors.Is(ctx.Err(), context.DeadlineExceeded) && deletionContext().Err() == nil
}
