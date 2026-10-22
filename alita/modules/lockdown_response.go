package modules

import (
	"context"
	"errors"
	"fmt"
	"html"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/db/approvals"
	"github.com/divkix/Alita_Robot/alita/db/lockdown"
	"github.com/divkix/Alita_Robot/alita/db/logchannels"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/i18n"
	"github.com/divkix/Alita_Robot/alita/utils/actionlog"
	"github.com/divkix/Alita_Robot/alita/utils/formatting"
)

// lockdownApprovedLookup reads the approved users of a chat fresh from the database and
// returns its error. A seam so a test can make the read fail: the cached IsUserApproved
// turns a failed read into "not approved", which would ban an approved user.
var lockdownApprovedLookup = approvals.ListApprovedIDsFresh

// "Ban N recent joiners" bans the accounts of a lockdown's burst for good. This file is
// the policy and safety core of that: one function decides and bans one account, so
// that no path to a ban with no end date skips the checks. The run, its card and its
// button sit on top of lockdownResponseBanOne.

const (
	// lockdownBanSkipAdmin is the reason of an account that is a live creator or
	// administrator.
	lockdownBanSkipAdmin = "admin"
	// lockdownBanSkipApproved is the reason of an approved user of the group.
	lockdownBanSkipApproved = "approved"
	// lockdownBanSkipBanned is the reason of an account someone else already banned.
	lockdownBanSkipBanned = "already_banned"
	// lockdownBanSkipSelf is the reason of the bot's own account.
	lockdownBanSkipSelf = "bot_self"
	// lockdownBanSkipLetIn is the reason of an account the lockdown recorded as exempt,
	// because a live creator or administrator (or an anonymous admin) let them in.
	lockdownBanSkipLetIn = models.BurstReasonLetIn
	// lockdownBanReasonRequest marks a banned burst row whose account was a join request
	// the lockdown declined, so the report can say so.
	lockdownBanReasonRequest = "request"
)

// lockdownResponseClaimStates are the states in which a joiner row may be claimed (to
// acting) before its account is banned for good. Exempt is never in the list: someone
// an admin let in is skipped, not claimed. Acting and unbanning belong to the worker and
// the lift, which the run waits for instead.
var lockdownResponseClaimStates = []string{
	models.JoinerStatePending,
	models.JoinerStateBanned,
	models.JoinerStateBanFailed,
	models.JoinerStateDeclined,
	models.JoinerStateDeclineFailed,
	models.JoinerStateKept,
	models.JoinerStateCancelled,
}

// decideLockdownResponseBan decides, from the account's live status alone, whether
// "Ban N recent joiners" bans it. It is pure so the whole table is tested without a
// bot. banUntil is the end date the lockdown's own ban carries (the account's joiner
// row), or 0 for an account the lockdown never recorded.
//
// A creator or administrator is never banned (skip admin). A kicked account is banned
// again only when it is under the lockdown's own ban (isLockdownBan), which the new ban
// with no end date upgrades; any other ban, with no end date or with another one, was
// placed by someone else and is left exactly as it is (skip already_banned), because
// banChatMember on a kicked account would replace that ban. Every other status (member,
// restricted, left) is banned.
func decideLockdownResponseBan(m gotgbot.MergedChatMember, banUntil int64) (ban bool, skip string) {
	switch m.Status {
	case gotgbot.ChatMemberStatusCreator, gotgbot.ChatMemberStatusAdministrator:
		return false, lockdownBanSkipAdmin
	case gotgbot.ChatMemberStatusKicked:
		if isLockdownBan(m, banUntil) {
			return true, ""
		}
		return false, lockdownBanSkipBanned
	}
	return true, ""
}

// moveLockdownBurst moves a burst row and logs a failed write; the row then stays in its
// claimed state until the stale-claim release returns it to the queue.
func moveLockdownBurst(id uint, from, to, reason, detail string, countAttempt bool) {
	if _, err := lockdown.MoveBurst(id, from, to, reason, detail, countAttempt); err != nil {
		log.Errorf("[Lockdown] burst row %d could not move from %s to %s: %v", id, from, to, err)
	}
}

// lockdownResponseBanOne decides and bans one account of the burst for good. The burst
// row is already claimed to acting by the caller. It reports whether the row reached a
// final state (banned, skipped or failed for good); false means it went back to the
// queue.
//
// The account is skipped, with a call to nothing, when it is an approved user (approved
// is read fresh by the caller), the bot itself, or a joiner the lockdown recorded as
// exempt (let in by an admin, D-22), also when that row turned exempt after the run was
// queued, because the joiner row is read fresh here. Otherwise the joiner row, if the
// lockdown recorded one, is claimed to acting before anything is sent: that keeps the
// worker's marker ban and the lift's unban away from the account for the whole call, so
// neither can overwrite or undo a ban with no end date. A joiner row held by the worker
// or the lift (acting, unbanning) or lost in a race sends the account back to the queue
// without an attempt.
//
// One paced unit then reads the live status and, when decideLockdownResponseBan says
// ban, reads the lockdown once more and sends banChatMember with empty options: no end
// date and no message removal. When the lockdown is no longer active by then (a lift
// recorded while the unit waited for its slot) nothing is sent: the joiner row is
// restored and the account goes back to the queue without an attempt. A
// ban moves the burst row to banned (reason request for a declined join request) and
// the joiner row to kept with detail response_ban, which the lift leaves banned. Every
// other outcome puts the joiner row back in the state and detail it came from. A rate
// limit or a shutdown sends the burst row back without an attempt; another failure
// costs one, and the third is final (failed).
//
// A crash between the claim and the move leaves the joiner row acting; ReleaseStaleClaims
// returns it to pending (active lockdown) or banned (lifting), and the worker's and the
// lift's live checks then keep a ban with no end date (Phase 4 rules). The burst row is
// returned to the queue by ReleaseStaleBurstClaims.
func lockdownResponseBanOne(
	ctx context.Context,
	b *gotgbot.Bot,
	ld models.ChatLockdown,
	row models.ChatLockdownBurst,
	approved map[int64]bool,
) bool {
	skip := func(reason string) bool {
		moveLockdownBurst(row.ID, models.BurstStateActing, models.BurstStateSkipped, reason, "", false)
		return true
	}
	requeue := func() bool {
		moveLockdownBurst(row.ID, models.BurstStateActing, models.BurstStateQueued, "", "", false)
		return false
	}

	if approved[row.UserID] {
		return skip(lockdownBanSkipApproved)
	}
	if row.UserID == b.Id {
		return skip(lockdownBanSkipSelf)
	}

	joiner, err := lockdown.GetJoinerOfUserFresh(ld.ID, row.UserID)
	if err != nil {
		return requeue()
	}

	var banUntil int64
	claimed := false
	request := false
	if joiner != nil {
		banUntil = joiner.BanUntil
		request = joiner.JoinPath == models.JoinPathRequest
		switch {
		case joiner.State == models.JoinerStateExempt:
			return skip(lockdownBanSkipLetIn)
		case joiner.State == models.JoinerStateActing || joiner.State == models.JoinerStateUnbanning:
			return requeue()
		case slices.Contains(lockdownResponseClaimStates, joiner.State):
			won, claimErr := lockdown.ClaimJoiner(joiner.ID, joiner.State, models.JoinerStateActing)
			if claimErr != nil || !won {
				return requeue()
			}
			claimed = true
		}
	}
	restoreJoiner := func() {
		if claimed {
			moveLockdownJoiner(joiner.ID, models.JoinerStateActing, joiner.State, joiner.Detail, false)
		}
	}

	var (
		ban        bool
		skipReason string
		// ended is set when the lockdown stopped being active while the account waited for
		// its slot, so no ban was sent.
		ended bool
	)
	err = lockdownPaced(ctx, func(callCtx context.Context) error {
		call, cancel := context.WithTimeout(callCtx, lockdownCallTimeout)
		defer cancel()
		ban, skipReason, ended = false, "", false
		live, lookupErr := b.GetChatMemberWithContext(call, ld.ChatID, row.UserID, nil)
		if lookupErr != nil {
			return lookupErr
		}
		if live == nil {
			return errors.New("getChatMember returned no member")
		}
		ban, skipReason = decideLockdownResponseBan(live.MergeChatMember(), banUntil)
		if !ban {
			return nil
		}
		// The caller read the lockdown before this unit waited for its slot, which can take
		// up to a minute during a raid. A lift recorded in that wait must not get a ban for
		// good it will never take back (D-12), so the lockdown is read once more right
		// before the call. An unreadable lockdown fails the call: nobody is banned without it.
		current, readErr := lockdown.GetFresh(ld.ID)
		if readErr != nil {
			return readErr
		}
		if current == nil || current.State != models.LockdownStateActive {
			ban, ended = false, true
			return nil
		}
		_, banErr := b.BanChatMemberWithContext(call, ld.ChatID, row.UserID, &gotgbot.BanChatMemberOpts{})
		return banErr
	})

	switch lockdownClassify(ctx, err) {
	case lockdownCallDone:
		if ended {
			// The run stops at its next account; the account goes back unhandled, with the
			// joiner row as it was found.
			restoreJoiner()
			return requeue()
		}
		if !ban {
			restoreJoiner()
			return skip(skipReason)
		}
		reason := ""
		if request {
			reason = lockdownBanReasonRequest
		}
		moveLockdownBurst(row.ID, models.BurstStateActing, models.BurstStateBanned, reason, "", false)
		if claimed {
			moveLockdownJoiner(joiner.ID, models.JoinerStateActing, models.JoinerStateKept, models.JoinerDetailResponseBan, false)
		}
		return true
	case lockdownCallRetryFree:
		restoreJoiner()
		return requeue()
	}

	log.Warnf("[Lockdown] ban for good of user %d in chat %d failed: %v", row.UserID, ld.ChatID, err)
	restoreJoiner()
	if row.Attempts+1 >= lockdownMaxAttempts {
		moveLockdownBurst(row.ID, models.BurstStateActing, models.BurstStateFailed, "", telegramErrorDetail(err), true)
		return true
	}
	moveLockdownBurst(row.ID, models.BurstStateActing, models.BurstStateQueued, "", telegramErrorDetail(err), true)
	return false
}

// lockdownBanExcluded is the list of users a ban run never queues: the approved users of
// the locked chat, read fresh, and the bot itself. It returns the approvals' error, and
// then nothing may be banned: the Ask refuses, a Confirm starts no run and a running run
// waits.
func lockdownBanExcluded(b *gotgbot.Bot, chatID int64) ([]int64, error) {
	approved, err := lockdownApprovedLookup(chatID)
	if err != nil {
		return nil, err
	}
	excluded := make([]int64, 0, len(approved)+1)
	for id := range approved {
		excluded = append(excluded, id)
	}
	return append(excluded, b.Id), nil
}

// lockdownProcessBanRuns advances every running ban run by one batch: the queued accounts
// are claimed one at a time and handed to lockdownResponseBanOne, and a run with nothing
// left open is finished. A run whose lockdown is no longer active is stopped instead
// (lockdownStopBanRun). It reports whether any account reached a final state or any run
// finished.
func lockdownProcessBanRuns(ctx context.Context, b *gotgbot.Bot) bool {
	runs, err := lockdown.ListRunningBanRunsFresh()
	if err != nil {
		log.Errorf("[Lockdown] worker could not list running ban runs: %v", err)
		return false
	}
	progress := false
	for i := range runs {
		if ctx.Err() != nil {
			return progress
		}
		if lockdownProcessBanRun(ctx, b, &runs[i]) {
			progress = true
		}
	}
	return progress
}

// lockdownProcessBanRun is lockdownProcessBanRuns for one run.
func lockdownProcessBanRun(ctx context.Context, b *gotgbot.Bot, run *models.ChatLockdownBanRun) bool {
	ld, err := lockdown.GetFresh(run.LockdownID)
	if err != nil {
		log.Warnf("[Lockdown] the lockdown of ban run %d could not be read, so the run waits: %v", run.ID, err)
		return false
	}
	if ld == nil || ld.State != models.LockdownStateActive {
		return lockdownStopBanRun(run)
	}
	// The approved users are read fresh on every pass. When they cannot be read the run
	// waits, because an account banned for good without the list could be an approved one.
	approved, err := lockdownApprovedLookup(ld.ChatID)
	if err != nil {
		log.Warnf("[Lockdown] the approved users of chat %d could not be read, so ban run %d waits: %v", ld.ChatID, run.ID, err)
		return false
	}
	rows, err := lockdown.ListQueuedBurstFresh(run.ID, lockdownWorkerBatch)
	if err != nil {
		log.Errorf("[Lockdown] worker could not list the queued accounts of ban run %d: %v", run.ID, err)
		return false
	}
	progress := false
	for _, row := range rows {
		if ctx.Err() != nil {
			return progress
		}
		// The lockdown is read again before every account: a lift that starts in the
		// middle of a batch stops the run at the next account, because a lifting
		// lockdown must not get a ban for good it will never take back (D-12).
		current, err := lockdown.GetFresh(run.LockdownID)
		if err != nil {
			log.Warnf("[Lockdown] the lockdown of ban run %d could not be read, so the run waits: %v", run.ID, err)
			return progress
		}
		if current == nil || current.State != models.LockdownStateActive {
			return lockdownStopBanRun(run) || progress
		}
		won, err := lockdown.ClaimBurst(row.ID, models.BurstStateQueued, models.BurstStateActing)
		if err != nil || !won {
			continue
		}
		if lockdownResponseBanOne(ctx, b, *ld, row, approved) {
			progress = true
		}
	}
	if ctx.Err() != nil {
		return progress
	}
	finished, err := lockdown.FinishBanRun(run.ID, models.BanRunStateDone)
	if err != nil {
		log.Errorf("[Lockdown] ban run %d could not be finished: %v", run.ID, err)
		return progress
	}
	if finished {
		// The result is delivered by lockdownReportBanRuns, which resumes from the database.
		progress = true
	}
	return progress
}

// lockdownStopBanRun ends a run whose lockdown is no longer active (lifting, lifted or
// gone): its queued accounts go back to unhandled, and the run moves to stopped through
// the same conditional finish as a normal end. The result card is then edited by
// lockdownReportBanRuns, which says how far the run got and that it stopped. An account
// whose call is in flight on another replica (acting) is left to that call or to the
// stale-claim release. A failed write is logged and retried on the next cycle. It reports
// whether this call stopped the run.
func lockdownStopBanRun(run *models.ChatLockdownBanRun) bool {
	if _, err := lockdown.UnqueueBanRun(run.ID); err != nil {
		log.Errorf("[Lockdown] ban run %d could not give back its queued accounts: %v", run.ID, err)
		return false
	}
	finished, err := lockdown.FinishBanRun(run.ID, models.BanRunStateStopped)
	if err != nil {
		log.Errorf("[Lockdown] ban run %d could not be stopped: %v", run.ID, err)
		return false
	}
	return finished
}

// lockdownBanRunTranslator is the translator of the chat a run's card lives in: the Staff
// Group for a staff card, the locked group (whose log channel holds the card) for a log
// card.
func lockdownBanRunTranslator(run *models.ChatLockdownBanRun, ld *models.ChatLockdown) *i18n.Translator {
	if run.CardDest == models.AlertDestLog {
		return staffChatTranslator(ld.ChatID)
	}
	return staffChatTranslator(run.CardChatID)
}

// lockdownTallySum adds up the counts of one state of a run's tally, whatever the reason.
func lockdownTallySum(tally map[string]map[string]int64, state string) int64 {
	var total int64
	for _, count := range tally[state] {
		total += count
	}
	return total
}

// lockdownBanFailedListMax is how many failed accounts a ban run's report names; the
// rest are counted.
const lockdownBanFailedListMax = 10

// lockdownBanSkipLines are the skip reasons the report counts on a line of their own, in
// the order they are listed. Every skip is in the skipped total; the bot's own account is
// never queued, so it has no line.
var lockdownBanSkipLines = []struct{ reason, key string }{
	{lockdownBanSkipAdmin, "lockdown_ban_skip_admin"},
	{lockdownBanSkipApproved, "lockdown_ban_skip_approved"},
	{lockdownBanSkipBanned, "lockdown_ban_skip_banned"},
	{lockdownBanSkipLetIn, "lockdown_ban_skip_let_in"},
}

// lockdownBanFailedLines renders the accounts of a run that could not be banned, in the
// order they were queued, one line each: the escaped name (or the ID when there is no
// name), the ID in code tags and Telegram's stored reason, which is already escaped. It
// stops at lockdownBanFailedListMax lines or lockdownTallyMaxRunes and reports how many of
// failed it left out.
func lockdownBanFailedLines(runID uint, failed int64) (lines []string, more int64) {
	rows, err := lockdown.ListBurstOfRunFresh(runID, models.BurstStateFailed, lockdownBanFailedListMax)
	if err != nil {
		log.Errorf("[Lockdown] failed accounts of ban run %d could not be listed: %v", runID, err)
		return nil, failed
	}
	used := 0
	for _, row := range rows {
		name := strconv.FormatInt(row.UserID, 10)
		if row.FirstName != "" {
			name = html.EscapeString(lockdownCapRunes(row.FirstName, lockdownNameMaxRunes))
		}
		line := fmt.Sprintf("%s (<code>%d</code>)", name, row.UserID)
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

// lockdownBanResultText is the report of a finished (or stopped) ban run, in tr's
// language: how many accounts were banned (and how many of those had asked to join and
// were declined), skipped with one line per reason, and failed, with up to
// lockdownBanFailedListMax accounts named, and a line when the run was stopped by the
// lift. The group label and the names and reasons of failed accounts are user text, so
// they are spliced in after the translation.
func lockdownBanResultText(
	tr *i18n.Translator, ld *models.ChatLockdown, run *models.ChatLockdownBanRun,
	tally map[string]map[string]int64, stopped bool,
) string {
	failed := lockdownTallySum(tally, models.BurstStateFailed)
	lines := []string{lockdownText(tr, "lockdown_ban_result", i18n.TranslationParams{
		"group":   lockdownGroupToken,
		"banned":  strconv.FormatInt(lockdownTallySum(tally, models.BurstStateBanned), 10),
		"skipped": strconv.FormatInt(lockdownTallySum(tally, models.BurstStateSkipped), 10),
		"failed":  strconv.FormatInt(failed, 10),
	})}
	if requests := tally[models.BurstStateBanned][lockdownBanReasonRequest]; requests > 0 {
		lines = append(lines, lockdownText(tr, "lockdown_ban_result_requests", i18n.TranslationParams{
			"count": strconv.FormatInt(requests, 10),
		}))
	}
	for _, skip := range lockdownBanSkipLines {
		if count := tally[models.BurstStateSkipped][skip.reason]; count > 0 {
			lines = append(lines, lockdownText(tr, skip.key, i18n.TranslationParams{
				"count": strconv.FormatInt(count, 10),
			}))
		}
	}
	list := ""
	if failed > 0 {
		lines = append(lines,
			lockdownText(tr, "lockdown_ban_failed_line", i18n.TranslationParams{"count": strconv.FormatInt(failed, 10)}),
			lockdownListToken)
		failedLines, more := lockdownBanFailedLines(run.ID, failed)
		if more > 0 {
			failedLines = append(failedLines, lockdownText(tr, "lockdown_lift_tally_more", i18n.TranslationParams{
				"count": strconv.FormatInt(more, 10),
			}))
		}
		list = lockdownJoinLines(failedLines)
	}
	if stopped {
		lines = append(lines, lockdownText(tr, "lockdown_ban_stopped"))
	}
	return lockdownSplice(lockdownJoinLines(lines),
		lockdownGroupToken, lockdownGroupLabel(ld),
		lockdownListToken, list)
}

// lockdownReportBanRuns delivers the result of every finished ban run whose report is
// not settled: the card is edited to lockdownBanResultText and the locked group's log
// channel gets the one #LOCKDOWN_BAN post. It follows the alert close step: a run is
// claimed first so one replica delivers it, a rate limit or a shutdown gives the claim
// back without costing an attempt and ends the step for the cycle, and the stale-claim
// release in lockdownReleaseStale covers a worker that stopped. At most lockdownAlertBatch
// runs are taken per cycle. It reports whether any step of any run was settled.
func lockdownReportBanRuns(ctx context.Context, b *gotgbot.Bot) bool {
	runs, err := lockdown.ListUnreportedBanRunsFresh(lockdownAlertBatch)
	if err != nil {
		log.Errorf("[Lockdown] ban runs whose report is not delivered could not be listed: %v", err)
		return false
	}
	progress := false
	for i := range runs {
		if ctx.Err() != nil {
			return progress
		}
		done, stop := lockdownReportBanRun(ctx, b, runs[i].ID)
		if done {
			progress = true
		}
		if stop {
			// The same pacer limits every delivery, so the rest would only be refused too.
			break
		}
	}
	return progress
}

// releaseBanRunReport gives a claimed report back, logging a failure.
func releaseBanRunReport(runID uint, countAttempt bool) {
	if _, err := lockdown.ReleaseBanRunReport(runID, countAttempt); err != nil {
		log.Errorf("[Lockdown] the report of ban run %d could not be given back: %v", runID, err)
	}
}

// lockdownReportBanRun claims and delivers the report of one run. The card edit and the
// log post are separate steps, each recorded once settled, so a step that was delivered is
// never repeated (the #LOCKDOWN_BAN post is sent at most once after it was recorded)
// while the other is retried. A step that failed lockdownMaxAttempts times is given up on
// and logged at warn, so a run never loops. The accounts' states are already stored, and
// nothing is written to the staff audit. done reports whether a step was settled; stop
// that the step should end for this cycle.
func lockdownReportBanRun(ctx context.Context, b *gotgbot.Bot, runID uint) (done, stop bool) {
	claimed, err := lockdown.ClaimBanRunReport(runID)
	if err != nil || !claimed {
		return false, false
	}
	// The run is read again once it is claimed, so the steps another replica settled
	// before this claim are not repeated.
	run, err := lockdown.GetBanRunFresh(runID)
	if err != nil || run == nil {
		releaseBanRunReport(runID, false)
		return false, false
	}
	ld, err := lockdown.GetFresh(run.LockdownID)
	if err != nil {
		releaseBanRunReport(runID, false)
		return false, false
	}
	settle := func(step string) bool {
		if _, err := lockdown.RecordBanRunReportStep(runID, step); err != nil {
			log.Errorf("[Lockdown] the %s step of the report of ban run %d was delivered but not recorded: %v", step, runID, err)
			return false
		}
		run.ReportAttempts = 0
		return true
	}
	if ld == nil {
		log.Warnf("[Lockdown] ban run %d finished, but its lockdown is gone, so there is nothing to report on", runID)
		a := settle(lockdown.BanRunReportCardStep)
		c := settle(lockdown.BanRunReportLogStep)
		return a || c, false
	}
	tally, err := lockdown.TallyBanRun(runID)
	if err != nil {
		log.Warnf("[Lockdown] the result of ban run %d could not be counted: %v", runID, err)
		releaseBanRunReport(runID, false)
		return false, false
	}

	// fail handles a step whose delivery failed for another reason than a rate limit or a
	// shutdown: it costs an attempt, and the last of lockdownMaxAttempts gives the step up.
	fail := func(step string, cause error) (settled bool) {
		log.Warnf("[Lockdown] the %s step of the report of ban run %d failed: %v", step, runID, cause)
		if run.ReportAttempts+1 >= lockdownMaxAttempts {
			log.Warnf("[Lockdown] the %s step of the report of ban run %d is given up after %d attempts", step, runID, lockdownMaxAttempts)
			return settle(step)
		}
		releaseBanRunReport(runID, true)
		return false
	}

	if run.ReportCardDoneAt == nil {
		outcome, cause := lockdownBanRunEditCard(ctx, b, run, ld, tally)
		switch outcome {
		case lockdownCallDone:
			if !settle(lockdown.BanRunReportCardStep) {
				return done, false
			}
			done = true
		case lockdownCallRetryFree:
			releaseBanRunReport(runID, false)
			return done, true
		default:
			if !fail(lockdown.BanRunReportCardStep, cause) {
				return done, false
			}
			done = true
		}
	}
	if run.ReportLogDoneAt == nil {
		outcome, cause := lockdownBanLogPost(ctx, b, ld, run, tally)
		switch outcome {
		case lockdownCallDone:
			if !settle(lockdown.BanRunReportLogStep) {
				return done, false
			}
			done = true
		case lockdownCallRetryFree:
			releaseBanRunReport(runID, false)
			return done, true
		default:
			if !fail(lockdown.BanRunReportLogStep, cause) {
				return done, false
			}
			done = true
		}
	}
	return done, false
}

// lockdownBanRunEditCard edits the run's card to the result through the lockdown pacer. A
// run with no card, an edit Telegram reports as not modified and a card that is gone all
// count as delivered (lockdownCallDone); a rate limit or a shutdown is
// lockdownCallRetryFree; any other failure is lockdownCallFailed with its cause.
func lockdownBanRunEditCard(
	ctx context.Context, b *gotgbot.Bot, run *models.ChatLockdownBanRun, ld *models.ChatLockdown,
	tally map[string]map[string]int64,
) (lockdownCallOutcome, error) {
	if run.CardChatID == 0 || run.CardMsgID == 0 {
		log.Warnf("[Lockdown] ban run %d finished with no card to report on", run.ID)
		return lockdownCallDone, nil
	}
	text := lockdownBanResultText(lockdownBanRunTranslator(run, ld), ld, run, tally, run.State == models.BanRunStateStopped)
	err := lockdownPaced(ctx, func(callCtx context.Context) error {
		call, cancel := context.WithTimeout(callCtx, lockdownCallTimeout)
		defer cancel()
		_, _, editErr := b.EditMessageTextWithContext(call, &gotgbot.EditMessageTextOpts{
			Text:               text,
			ChatId:             run.CardChatID,
			MessageId:          run.CardMsgID,
			ParseMode:          formatting.HTML,
			LinkPreviewOptions: &gotgbot.LinkPreviewOptions{IsDisabled: true},
		})
		return editErr
	})
	switch {
	case isMessageNotModified(err), lockdownEditGone(err):
		return lockdownCallDone, nil
	}
	return lockdownClassify(ctx, err), err
}

// lockdownBanLogPost posts the one record of a ban run in the locked group's log channel,
// admin category: "#LOCKDOWN_BAN", the admin who pressed Ban and how many accounts were
// banned, skipped and failed, in the locked group's language. The hashtag is not
// localized. It posts nothing for a run that handled no account, when the group has no
// log channel or its admin category is off, and nothing is ever sent into the group
// itself; those count as delivered (lockdownCallDone). A rate limit or a shutdown is
// lockdownCallRetryFree; any other failure is lockdownCallFailed with its cause. The text
// says only that accounts were banned: no claim is made about their messages.
func lockdownBanLogPost(
	ctx context.Context, b *gotgbot.Bot, ld *models.ChatLockdown, run *models.ChatLockdownBanRun,
	tally map[string]map[string]int64,
) (lockdownCallOutcome, error) {
	if ctx.Err() != nil {
		return lockdownCallRetryFree, ctx.Err()
	}
	banned := lockdownTallySum(tally, models.BurstStateBanned)
	skipped := lockdownTallySum(tally, models.BurstStateSkipped)
	failed := lockdownTallySum(tally, models.BurstStateFailed)
	if banned+skipped+failed == 0 {
		return lockdownCallDone, nil
	}
	chat := &gotgbot.Chat{Id: ld.ChatID, Title: ld.ChatTitle, Type: "supergroup"}
	channelID, header, ok := actionlog.Destination(chat, logchannels.CategoryAdmin)
	if !ok {
		return lockdownCallDone, nil
	}
	tr := staffChatTranslator(ld.ChatID)
	adminLabel, _ := tr.GetString("staff_log_admin_label")
	text := header + strings.Join([]string{
		"#LOCKDOWN_BAN",
		adminLabel + ": " + formatting.MentionHtml(run.StartedBy, run.StartedByName),
		lockdownText(tr, "lockdown_ban_log", i18n.TranslationParams{
			"banned":  strconv.FormatInt(banned, 10),
			"skipped": strconv.FormatInt(skipped, 10),
			"failed":  strconv.FormatInt(failed, 10),
		}),
	}, " · ")
	err := lockdownPaced(ctx, func(callCtx context.Context) error {
		call, cancel := context.WithTimeout(callCtx, lockdownCallTimeout)
		defer cancel()
		_, sendErr := b.SendMessageWithContext(call, channelID, text, &gotgbot.SendMessageOpts{
			ParseMode:          formatting.HTML,
			LinkPreviewOptions: &gotgbot.LinkPreviewOptions{IsDisabled: true},
		})
		return sendErr
	})
	return lockdownClassify(ctx, err), err
}
