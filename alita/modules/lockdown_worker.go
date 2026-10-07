package modules

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/db/lockdown"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/i18n"
	"github.com/divkix/Alita_Robot/alita/utils/error_handling"
	"github.com/divkix/Alita_Robot/alita/utils/formatting"
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

// lockdownStaleClaim is how old a claim (acting or unbanning with claimed_at) must be
// before it is taken to belong to a worker that stopped. It is far above the 10 s a
// Telegram call may take, so a live claim is never released. A variable so a test can
// shorten it.
var lockdownStaleClaim = 2 * time.Minute

// lockdownReleaseStale gives back the claims older than lockdownStaleClaim on every
// lockdown, on any replica. No attempt is counted for a release. A failure is logged
// and the next cycle tries again.
func lockdownReleaseStale() {
	released, err := lockdown.ReleaseStaleClaims(time.Now().Add(-lockdownStaleClaim))
	if err != nil {
		log.Errorf("[Lockdown] worker could not release stale claims: %v", err)
		return
	}
	if released > 0 {
		log.Warnf("[Lockdown] released %d stale claim(s) of a worker that stopped", released)
	}
}

// lockdownUnconfirmedGrace is how long a lockdown row may stay unconfirmed (locked_at
// NULL) before it is taken to be a /lockdown that stopped half way and is settled from
// the live permissions. It is far above the time one lock call takes, so a row whose
// own /lockdown is still running is never touched. A variable so a test can shorten it.
var lockdownUnconfirmedGrace = time.Minute

// settleUnconfirmedLockdown decides what an unconfirmed lockdown really is by reading
// the group's live permissions, paced like every worker call. Permissions equal to the
// locked set mean the lock took effect: the row is confirmed and nothing is announced,
// because the admin who typed /lockdown never got an answer and nothing more can be
// said. Anything else means the lock never took effect, and the row is deleted so the
// group is free to be locked again. A permissions answer that cannot be read touches
// the row, so it is retried after another grace period, and returns the error. confirmed
// and deleted are both false when the row changed under it (another replica settled it
// first), which is not an error.
func settleUnconfirmedLockdown(ctx context.Context, b *gotgbot.Bot, ld *models.ChatLockdown) (confirmed bool, deleted bool, err error) {
	var chat lockdownChat
	err = lockdownPaced(ctx, func(callCtx context.Context) error {
		var fetchErr error
		chat, fetchErr = fetchLockdownChat(callCtx, b, ld.ChatID)
		return fetchErr
	})
	if err == nil && !lockdownHasPermissions(chat.Permissions) {
		err = errors.New("getChat returned no permissions")
	}
	var same bool
	if err == nil {
		same, err = samePermissions(string(chat.Permissions), ld.LockedPermissions)
	}
	if err != nil {
		if ctx.Err() == nil {
			if touchErr := lockdown.TouchLockdown(ld.ID); touchErr != nil {
				log.Errorf("[Lockdown] unconfirmed lockdown %d could not be put back: %v", ld.ID, touchErr)
			}
		}
		return false, false, err
	}

	if same {
		confirmed, err = lockdown.ConfirmLocked(ld.ID)
		return confirmed, false, err
	}
	deleted, err = lockdown.DeleteUnconfirmed(ld.ID)
	return false, deleted, err
}

// lockdownSettleUnconfirmed settles every lockdown that stayed unconfirmed for longer
// than lockdownUnconfirmedGrace, on any replica. Settling never posts a message. It
// reports whether any row was confirmed or deleted.
func lockdownSettleUnconfirmed(ctx context.Context, b *gotgbot.Bot) bool {
	rows, err := lockdown.ListUnconfirmedFresh(time.Now().Add(-lockdownUnconfirmedGrace))
	if err != nil {
		log.Errorf("[Lockdown] worker could not list unconfirmed lockdowns: %v", err)
		return false
	}
	progress := false
	for _, ld := range rows {
		if ctx.Err() != nil {
			break
		}
		confirmed, deleted, err := settleUnconfirmedLockdown(ctx, b, &ld)
		if err != nil {
			log.Warnf("[Lockdown] unconfirmed lockdown %d of chat %d could not be settled: %v", ld.ID, ld.ChatID, err)
			continue
		}
		if confirmed || deleted {
			progress = true
		}
	}
	return progress
}

// lockdownTallyListMax is how many joiners the lift's tally names when they could not
// be unbanned; the rest are counted. A variable so a test can shrink it.
var lockdownTallyListMax = 25

// lockdownListToken stands in for the tally's list of failed unbans until the
// translated text is assembled, so names and Telegram's reasons are never run through
// the translator's printf-style pass.
const lockdownListToken = "<<lockdown-list>>"

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

// Lifecycle state of the worker goroutine. Each run has its own done channel rather
// than a shared WaitGroup, so a worker that was cut off at shutdown and is still
// finishing a call never shares state with the next one.
var (
	lockdownWorkerMu     sync.Mutex
	lockdownWorkerCancel context.CancelFunc
	lockdownWorkerDone   chan struct{}
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
	done := make(chan struct{})
	lockdownWorkerCancel = cancel
	lockdownWorkerDone = done
	go func() {
		defer close(done)
		defer error_handling.RecoverFromPanic("lockdownWorker", "Lockdown")
		lockdownWorkerLoop(ctx, b)
	}()
}

// StopLockdownWorker cancels the worker and waits for it for at most
// lockdownWorkerStopWait, so no cycle is still writing when shutdown closes the
// database. A call that is blocked in Telegram does not hold shutdown up: the wait
// ends and the row it was working on is resumed from the database, by a restart or by
// another replica once its claim is two minutes old. It is safe to call when the
// worker is not running, and more than once.
func StopLockdownWorker() {
	lockdownWorkerMu.Lock()
	cancel := lockdownWorkerCancel
	done := lockdownWorkerDone
	lockdownWorkerMu.Unlock()
	if cancel == nil {
		return
	}
	cancel()

	// Wait without holding the mutex: a cycle that is mid-call must not wedge
	// shutdown behind a lock.
	select {
	case <-done:
	case <-time.After(lockdownWorkerStopWait):
		log.Warnf("[Lockdown] worker did not stop within %s; every row resumes from the database", lockdownWorkerStopWait)
	}

	lockdownWorkerMu.Lock()
	if lockdownWorkerDone == done {
		lockdownWorkerCancel = nil
		lockdownWorkerDone = nil
	}
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
	// First of all, give back what a worker that stopped left claimed, so the steps
	// below see it as work again.
	lockdownReleaseStale()
	settled := lockdownSettleUnconfirmed(ctx, b)
	banned := lockdownBanPending(ctx, b)
	deleted := lockdownDeleteJoinMessages(ctx, b)
	lifted := lockdownProcessLifts(ctx, b)
	return settled || banned || deleted || lifted
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
// first, and declines the pending join requests (lockdownDeclineOne). Each row is
// claimed first, so one replica handles a joiner exactly once; a ban ends on the
// row's ban_until, which marks it as the lockdown's own.
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
		handle := lockdownBanOne
		if row.JoinPath == models.JoinPathRequest {
			handle = lockdownDeclineOne
		}
		if handle(ctx, b, row) {
			progress = true
		}
	}
	return progress
}

// lockdownDeclineOne declines one claimed join request and records the result: a
// request Telegram reports as already gone counts as declined, a rate limit or a
// shutdown puts the row back without costing an attempt, and any other failure costs
// one, the third being final (decline_failed). The person can request again after
// the lift, or at once during the lockdown to be declined again. It reports whether
// the row reached a final state for this stage.
func lockdownDeclineOne(ctx context.Context, b *gotgbot.Bot, row models.LockdownJoiner) bool {
	err := lockdownPaced(ctx, func(callCtx context.Context) error {
		call, cancel := context.WithTimeout(callCtx, lockdownCallTimeout)
		defer cancel()
		_, declineErr := b.DeclineChatJoinRequestWithContext(call, row.ChatID, row.UserID, nil)
		return declineErr
	})

	if isJoinRequestGone(err) {
		moveLockdownJoiner(row.ID, models.JoinerStateActing, models.JoinerStateDeclined, "", false)
		return true
	}
	switch lockdownClassify(ctx, err) {
	case lockdownCallDone:
		moveLockdownJoiner(row.ID, models.JoinerStateActing, models.JoinerStateDeclined, "", false)
		return true
	case lockdownCallRetryFree:
		moveLockdownJoiner(row.ID, models.JoinerStateActing, models.JoinerStatePending, "", false)
		return false
	}

	log.Warnf("[Lockdown] decline of the join request of user %d in chat %d failed: %v", row.UserID, row.ChatID, err)
	if row.Attempts+1 >= lockdownMaxAttempts {
		moveLockdownJoiner(row.ID, models.JoinerStateActing, models.JoinerStateDeclineFailed, telegramErrorDetail(err), true)
		return true
	}
	moveLockdownJoiner(row.ID, models.JoinerStateActing, models.JoinerStatePending, telegramErrorDetail(err), true)
	return false
}

// lockdownBanOne bans one claimed joiner and records the result. It looks at the
// joiner's live status first, in the same paced unit as the ban: banChatMember on a
// kicked user replaces the end date of that ban, so a ban someone placed after the guard
// recorded the joiner (an admin's /ban, a staff fan-out) would otherwise be turned into
// the lockdown's own and lifted. A joiner who is already kicked on a date other than
// the row's ban_until is therefore kept, with no ban call (D-05); one who is kicked on
// the row's own ban_until is the lockdown's ban whose answer was lost, and is recorded as
// banned. A status that cannot be read is a failed call, never a ban. It reports whether
// the row reached a final state for this stage.
func lockdownBanOne(ctx context.Context, b *gotgbot.Bot, row models.LockdownJoiner) bool {
	var deliberate bool
	err := lockdownPaced(ctx, func(callCtx context.Context) error {
		call, cancel := context.WithTimeout(callCtx, lockdownCallTimeout)
		defer cancel()
		deliberate = false
		live, lookupErr := b.GetChatMemberWithContext(call, row.ChatID, row.UserID, nil)
		if lookupErr != nil {
			return lookupErr
		}
		if live == nil {
			return errors.New("getChatMember returned no member")
		}
		if member := live.MergeChatMember(); member.Status == gotgbot.ChatMemberStatusKicked {
			deliberate = !isLockdownBan(member, row.BanUntil)
			return nil
		}
		_, banErr := b.BanChatMemberWithContext(call, row.ChatID, row.UserID, &gotgbot.BanChatMemberOpts{UntilDate: row.BanUntil})
		return banErr
	})

	switch lockdownClassify(ctx, err) {
	case lockdownCallDone:
		if deliberate {
			moveLockdownJoiner(row.ID, models.JoinerStateActing, models.JoinerStateKept, "", false)
			return true
		}
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

// lockdownDeleteJoinMessages deletes the join service messages the guard stored on
// rows whose joiner is banned now, so the announcement of a removed raider does not
// stay in the group. It runs after the bans and is best effort: a delete that fails
// (the bot has no delete right, the message is gone) leaves the message and is
// logged at debug, and a rate limit or a shutdown leaves the row for the next cycle.
// It reports whether any row was handled.
func lockdownDeleteJoinMessages(ctx context.Context, b *gotgbot.Bot) bool {
	rows, err := lockdown.ListJoinMsgsToDeleteFresh(lockdownWorkerBatch)
	if err != nil {
		log.Errorf("[Lockdown] worker could not list join messages to delete: %v", err)
		return false
	}
	progress := false
	for _, row := range rows {
		if ctx.Err() != nil {
			break
		}
		err := lockdownPaced(ctx, func(callCtx context.Context) error {
			call, cancel := context.WithTimeout(callCtx, lockdownCallTimeout)
			defer cancel()
			_, delErr := b.DeleteMessageWithContext(call, row.ChatID, row.JoinMsgID, nil)
			return delErr
		})
		if lockdownClassify(ctx, err) == lockdownCallRetryFree {
			// The same pacer limits every row, so the rest would only be refused too.
			break
		}
		if err != nil {
			log.Debugf("[Lockdown] join message %d of chat %d was not deleted: %v", row.JoinMsgID, row.ChatID, err)
		}
		if clearErr := lockdown.ClearJoinMsg(row.ID); clearErr != nil {
			log.Errorf("[Lockdown] join message of joiner row %d could not be cleared: %v", row.ID, clearErr)
			continue
		}
		progress = true
	}
	return progress
}

// lockdownProcessLifts works through every lockdown that is being lifted: joiners
// still pending are cancelled and never banned, the lockdown's own bans are unbanned
// in the order they were recorded, and the lockdown is finished once nothing is left,
// with one tally. It reports whether any row made progress.
func lockdownProcessLifts(ctx context.Context, b *gotgbot.Bot) bool {
	lifts, err := lockdown.ListLiftingFresh()
	if err != nil {
		log.Errorf("[Lockdown] worker could not list lifting lockdowns: %v", err)
		return false
	}
	progress := false
	for _, ld := range lifts {
		if ctx.Err() != nil {
			break
		}
		if lockdownProcessLift(ctx, b, ld) {
			progress = true
		}
	}
	return progress
}

// lockdownProcessLift is one cycle of one lift.
func lockdownProcessLift(ctx context.Context, b *gotgbot.Bot, ld models.ChatLockdown) bool {
	progress := false
	if cancelled, err := lockdown.CancelPending(ld.ID); err != nil {
		log.Errorf("[Lockdown] pending joiners of lockdown %d were not cancelled: %v", ld.ID, err)
	} else if cancelled > 0 {
		progress = true
	}

	rows, err := lockdown.ListJoinersInState(ld.ID, models.JoinerStateBanned, lockdownWorkerBatch)
	if err != nil {
		log.Errorf("[Lockdown] worker could not list banned joiners of lockdown %d: %v", ld.ID, err)
		return progress
	}
	for _, row := range rows {
		if ctx.Err() != nil {
			return progress
		}
		won, err := lockdown.ClaimJoiner(row.ID, models.JoinerStateBanned, models.JoinerStateUnbanning)
		if err != nil || !won {
			continue
		}
		if lockdownUnbanOne(ctx, b, row) {
			progress = true
		}
	}
	if ctx.Err() != nil {
		return progress
	}

	// One conditional update: it matches only while no joiner row is unfinished, and
	// only the replica that wins it posts the tally.
	finished, err := lockdown.FinishLift(ld.ID)
	if err != nil {
		log.Errorf("[Lockdown] lockdown %d lift was not finished: %v", ld.ID, err)
		return progress
	}
	if finished {
		lockdownPostTally(ctx, b, ld)
		progress = true
	}
	return progress
}

// lockdownUnbanOne lifts the lockdown's ban on one claimed joiner, if it is still the
// lockdown's own. It looks at the live member first: only a member that is kicked
// with this row's ban_until (isLockdownBan) is unbanned, with only_if_banned so the
// call can never remove a member; anyone else, a deliberate ban included, is kept and
// never touched. It reports whether the row reached a final state.
func lockdownUnbanOne(ctx context.Context, b *gotgbot.Bot, row models.LockdownJoiner) bool {
	var member gotgbot.MergedChatMember
	err := lockdownPaced(ctx, func(callCtx context.Context) error {
		call, cancel := context.WithTimeout(callCtx, lockdownCallTimeout)
		defer cancel()
		live, lookupErr := b.GetChatMemberWithContext(call, row.ChatID, row.UserID, nil)
		if lookupErr != nil {
			return lookupErr
		}
		if live == nil {
			return errors.New("getChatMember returned no member")
		}
		member = live.MergeChatMember()
		return nil
	})
	if err != nil {
		return lockdownUnbanFailed(ctx, row, err)
	}

	if !isLockdownBan(member, row.BanUntil) {
		detail := ""
		if lockdownBanExpired(member, row.BanUntil, time.Now()) {
			detail = models.JoinerDetailBanExpired
		}
		moveLockdownJoiner(row.ID, models.JoinerStateUnbanning, models.JoinerStateKept, detail, false)
		return true
	}

	err = lockdownPaced(ctx, func(callCtx context.Context) error {
		call, cancel := context.WithTimeout(callCtx, lockdownCallTimeout)
		defer cancel()
		_, unbanErr := b.UnbanChatMemberWithContext(call, row.ChatID, row.UserID, &gotgbot.UnbanChatMemberOpts{OnlyIfBanned: true})
		return unbanErr
	})
	if err != nil {
		return lockdownUnbanFailed(ctx, row, err)
	}
	moveLockdownJoiner(row.ID, models.JoinerStateUnbanning, models.JoinerStateUnbanned, "", false)
	return true
}

// lockdownUnbanFailed records a failed look-up or unban of a claimed row. A rate
// limit or a shutdown puts the row back without costing an attempt; any other failure
// costs one, and the third is final (unban_failed, named in the tally).
func lockdownUnbanFailed(ctx context.Context, row models.LockdownJoiner, err error) bool {
	if lockdownClassify(ctx, err) == lockdownCallRetryFree {
		moveLockdownJoiner(row.ID, models.JoinerStateUnbanning, models.JoinerStateBanned, "", false)
		return false
	}
	log.Warnf("[Lockdown] unban of user %d in chat %d failed: %v", row.UserID, row.ChatID, err)
	if row.Attempts+1 >= lockdownMaxAttempts {
		moveLockdownJoiner(row.ID, models.JoinerStateUnbanning, models.JoinerStateUnbanFailed, telegramErrorDetail(err), true)
		return true
	}
	moveLockdownJoiner(row.ID, models.JoinerStateUnbanning, models.JoinerStateBanned, telegramErrorDetail(err), true)
	return false
}

// lockdownTallyMaxRunes bounds the failure list of the tally, far under Telegram's
// 4096-character limit, so a long list never makes the tally itself fail to send.
const lockdownTallyMaxRunes = 3000

// lockdownFailureLines renders the joiners that could not be unbanned, in the order
// they were recorded, one line each: a mention of the name, the ID in code tags and
// Telegram's stored reason. It stops at lockdownTallyListMax lines or the size bound
// and reports how many of failed it left out.
func lockdownFailureLines(lockdownID uint, failed int64) (lines []string, more int64) {
	rows, err := lockdown.ListJoinersInState(lockdownID, models.JoinerStateUnbanFailed, lockdownTallyListMax)
	if err != nil {
		log.Errorf("[Lockdown] failed unbans of lockdown %d could not be listed: %v", lockdownID, err)
		return nil, failed
	}
	used := 0
	for _, row := range rows {
		name := row.FirstName
		if name == "" {
			name = fmt.Sprint(row.UserID)
		}
		line := fmt.Sprintf("%s (<code>%d</code>)", lockdownMention(row.UserID, name), row.UserID)
		if row.Detail != "" {
			line += ": " + row.Detail
		}
		used += utf8.RuneCountInString(line) + 1
		if used > lockdownTallyMaxRunes {
			break
		}
		lines = append(lines, line)
	}
	return lines, failed - int64(len(lines))
}

// lockdownPostTally posts the one message that closes a lift, in the chat's language:
// how many removed joiners were unbanned, how many were left as they are because the
// ban was no longer the lockdown's own, how many were no longer banned because the
// lockdown's 330-day ban had run out, and each joiner who could not be unbanned by
// name and ID. A lockdown whose lift had nothing to report posts nothing. Only the
// replica that won FinishLift calls it.
func lockdownPostTally(ctx context.Context, b *gotgbot.Bot, ld models.ChatLockdown) {
	tally, err := lockdown.TallyJoiners(ld.ID)
	if err != nil {
		log.Warnf("[Lockdown] tally of lockdown %d could not be read: %v", ld.ID, err)
		return
	}
	unbanned := tally[models.JoinerStateUnbanned]
	kept := tally[models.JoinerStateKept]
	failed := tally[models.JoinerStateUnbanFailed]
	if unbanned+kept+failed == 0 {
		return
	}
	// Kept rows whose ban had run out on its own are told apart from the ones left on
	// purpose. A count that cannot be read leaves them in the kept line.
	expired, err := lockdown.CountJoinersWithDetail(ld.ID, models.JoinerStateKept, models.JoinerDetailBanExpired)
	if err != nil {
		log.Warnf("[Lockdown] expired bans of lockdown %d could not be counted: %v", ld.ID, err)
		expired = 0
	}
	kept -= expired

	tr := staffChatTranslator(ld.ChatID)
	lines := []string{lockdownText(tr, "lockdown_lift_tally", i18n.TranslationParams{"count": unbanned})}
	if kept > 0 {
		lines = append(lines, lockdownText(tr, "lockdown_lift_tally_kept", i18n.TranslationParams{"count": kept}))
	}
	if expired > 0 {
		lines = append(lines, lockdownText(tr, "lockdown_lift_tally_expired", i18n.TranslationParams{"count": expired}))
	}
	list := ""
	if failed > 0 {
		lines = append(lines,
			lockdownText(tr, "lockdown_lift_tally_failed", i18n.TranslationParams{"count": failed}),
			lockdownListToken)
		failureLines, more := lockdownFailureLines(ld.ID, failed)
		if more > 0 {
			failureLines = append(failureLines, lockdownText(tr, "lockdown_lift_tally_more", i18n.TranslationParams{"count": more}))
		}
		list = lockdownJoinLines(failureLines)
	}
	// The names and reasons are user-controlled text, so the list goes in after the
	// translation, in place of its token.
	text := lockdownSplice(lockdownJoinLines(lines), lockdownListToken, list)

	err = lockdownPaced(ctx, func(callCtx context.Context) error {
		call, cancel := context.WithTimeout(callCtx, lockdownCallTimeout)
		defer cancel()
		_, sendErr := b.SendMessageWithContext(call, ld.ChatID, text, &gotgbot.SendMessageOpts{
			ParseMode:          formatting.HTML,
			LinkPreviewOptions: &gotgbot.LinkPreviewOptions{IsDisabled: true},
		})
		return sendErr
	})
	if err != nil {
		log.Warnf("[Lockdown] lift tally for chat %d was not delivered: %v", ld.ChatID, err)
	}
}

// moveLockdownJoiner moves a row and logs a failed write; the row then stays in its
// claimed state until the stale-claim release returns it two minutes later.
func moveLockdownJoiner(id uint, from, to, detail string, countAttempt bool) {
	if _, err := lockdown.MoveJoiner(id, from, to, detail, countAttempt); err != nil {
		log.Errorf("[Lockdown] joiner row %d could not move from %s to %s: %v", id, from, to, err)
	}
}
