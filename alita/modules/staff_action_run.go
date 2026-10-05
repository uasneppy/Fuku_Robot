package modules

import (
	"context"
	"errors"
	"fmt"
	"html"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"

	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/utils/chat_status"
	"github.com/divkix/Alita_Robot/alita/utils/error_handling"
	"github.com/divkix/Alita_Robot/alita/utils/ratelimit"
)

// staffGroupResult is how one linked group ended. Detail carries Telegram's
// error text, already HTML-escaped, for a staffReasonFailTelegram result.
type staffGroupResult struct {
	Link    models.StaffGroupLink
	Outcome staffOutcome
	Reason  staffReason
	Detail  string
}

var (
	// staffActionCallTimeout bounds each Telegram call made for a staff action.
	staffActionCallTimeout = 8 * time.Second
	// staffActionRunsWG joins every running fan-out, so a shutdown can wait for them.
	staffActionRunsWG sync.WaitGroup
	// staffActionsMu guards staffActionsCtx and staffActionsCancel.
	staffActionsMu sync.Mutex
	// staffActionsCtx is the context every run starts with; StopStaffActions
	// cancels it on shutdown.
	staffActionsCtx, staffActionsCancel = context.WithCancel(context.Background())
)

var (
	// staffActionEditEvery is the least time between two progress edits of the card
	// (D-16). It is a variable so tests can run the clock fast.
	staffActionEditEvery = 2500 * time.Millisecond
	// staffActionEditRetryUnit is what one second of Telegram's retry_after is worth
	// when the final edit waits to be retried.
	staffActionEditRetryUnit = time.Second
	// staffActionStopWait bounds how long StopStaffActions waits for the runs.
	staffActionStopWait = 30 * time.Second
)

// staffActionsContext is the context a new run starts with.
func staffActionsContext() context.Context {
	staffActionsMu.Lock()
	defer staffActionsMu.Unlock()
	return staffActionsCtx
}

// StopStaffActions cancels every running staff fan-out and waits for each to put
// its final summary on its card, so a group still unfinished reads "interrupted by
// restart" instead of staying pending. It is safe to call when nothing runs and
// more than once. The wait is bounded by staffActionStopWait.
//
// It must run before the database closes: the fan-out workers still write links
// through recheckLink while they wind down.
func StopStaffActions() {
	staffActionsMu.Lock()
	cancel := staffActionsCancel
	staffActionsMu.Unlock()
	cancel()

	// Wait without holding the mutex, so a late run can still read the context.
	drained := make(chan struct{})
	go func() {
		defer error_handling.RecoverFromPanic("StopStaffActions", "StaffActions")
		defer close(drained)
		staffActionRunsWG.Wait()
	}()
	timer := time.NewTimer(staffActionStopWait)
	defer timer.Stop()
	select {
	case <-drained:
	case <-timer.C:
		log.Warnf("[StaffActions] runs did not finish within %s of the shutdown", staffActionStopWait)
	}
}

// staffActionProgress holds the results of one run while it is going. Workers
// write their own slot through set; the coordinator reads a copy through
// snapshot. dirty says something changed since the last snapshot, which is what
// lets the coordinator skip an edit that would change nothing.
type staffActionProgress struct {
	mu      sync.Mutex
	results []staffGroupResult
	dirty   bool
}

func newStaffActionProgress(links []models.StaffGroupLink) *staffActionProgress {
	return &staffActionProgress{results: pendingResults(links)}
}

// set records how group i ended.
func (p *staffActionProgress) set(i int, r staffGroupResult) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.results[i] = r
	p.dirty = true
}

// snapshot returns a copy of the results and whether anything changed since the
// last call, and clears the flag.
func (p *staffActionProgress) snapshot() (results []staffGroupResult, dirty bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	results = make([]staffGroupResult, len(p.results))
	copy(results, p.results)
	dirty = p.dirty
	p.dirty = false
	return results, dirty
}

// sweepPending turns every group that never reported into a failed line with
// reason, and returns the final results. A worker that panicked, or a group a
// shutdown cut off, would otherwise stay pending and be dropped from the summary
// (STAFF-08).
func (p *staffActionProgress) sweepPending(reason staffReason) []staffGroupResult {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := range p.results {
		if p.results[i].Outcome == staffOutcomePending {
			log.Errorf("[StaffActions] group %d finished without a result; reporting it as failed (%s)",
				p.results[i].Link.GroupChatID, reason)
			p.results[i].Outcome = staffReasonOutcome(reason)
			p.results[i].Reason = reason
		}
	}
	results := make([]staffGroupResult, len(p.results))
	copy(results, p.results)
	p.dirty = false
	return results
}

// staffTelegramDetailRunes caps the Telegram error text shown on a failed line.
const staffTelegramDetailRunes = 120

// staffActionWorkers is how many linked groups one run processes at once.
const staffActionWorkers = 4

// staffActionPacer paces every Telegram call of a staff fan-out. Its slot
// reservation and retry_after block live in Redis, so every replica shares one
// budget. It is a variable so tests can install a fast one.
var staffActionPacer = ratelimit.NewTelegramPacer(ratelimit.TelegramPacerOptions{
	NextKey:    "alita:staff:pace:next",
	BlockKey:   "alita:staff:pace:block",
	Interval:   100 * time.Millisecond,
	MaxRetries: 3,
	MaxWait:    60 * time.Second,
})

// staffPaced runs one Telegram call under the shared pacer: it waits for its slot
// and, on a 429, waits out Telegram's retry_after and repeats the same closure.
func staffPaced(ctx context.Context, call func(context.Context) error) error {
	return staffActionPacer.Do(ctx, call)
}

// pendingResults is the summary's starting point: every linked group, in order,
// not finished yet (D-17).
func pendingResults(links []models.StaffGroupLink) []staffGroupResult {
	results := make([]staffGroupResult, len(links))
	for i, link := range links {
		results[i] = staffGroupResult{Link: link, Outcome: staffOutcomePending}
	}
	return results
}

// startStaffActionRun runs the fan-out in the background, then puts the final
// summary on the card and marks the card done.
func startStaffActionRun(
	b *gotgbot.Bot,
	card *staffActionCard,
	links []models.StaffGroupLink,
	newUntil int64,
	chatID, msgID int64,
) {
	// Added before the context is read, so a shutdown that cancels it afterwards
	// still finds this run in the wait group.
	staffActionRunsWG.Add(1)
	ctx := staffActionsContext()
	go func() {
		defer staffActionRunsWG.Done()
		defer error_handling.RecoverFromPanic("staffActionRun", "StaffActions")
		// Frees the target for the next staff action once this run is over, a panic
		// included. The lock's TTL is the safety net for a crash.
		defer releaseStaffTargetLock(card.Target, card.Token)

		tr := staffChatTranslator(card.StaffChat)
		progress := newStaffActionProgress(links)

		// The fan-out runs in its own goroutine so this one stays the only writer of
		// the card from here on: it alone edits the message, on a timer.
		fanOutDone := make(chan struct{})
		go func() {
			defer error_handling.RecoverFromPanic("staffActionFanOut", "StaffActions")
			defer close(fanOutDone)
			runStaffActionFanOut(ctx, b, card, links, newUntil, progress)
		}()

		ticker := time.NewTicker(staffActionEditEvery)
		defer ticker.Stop()
	progressLoop:
		for {
			select {
			case <-fanOutDone:
				break progressLoop
			case <-ticker.C:
				snapshot, dirty := progress.snapshot()
				if !dirty {
					continue
				}
				// A failed progress edit is skipped; the next tick or the final
				// summary carries the same information.
				if err := editStaffActionMessage(b, chatID, msgID, renderStaffActionSummary(tr, card, snapshot)); err != nil {
					log.Warnf("[StaffActions] progress edit of card %s: %v", card.Token, err)
				}
			}
		}
		ticker.Stop()

		// A group that never reported was cut off by a shutdown or lost to a panic.
		sweep := staffReasonFailInternal
		if ctx.Err() != nil {
			sweep = staffReasonFailInterrupted
		}
		results := progress.sweepPending(sweep)

		final, continuation := renderStaffActionSummaryFinal(tr, card, results)
		// Delivery never runs on the run's own context, which a shutdown may already
		// have cancelled: each message opens a fresh budget of its own.
		deliverStaffActionFinal(b, chatID, msgID, final, continuation, time.Time{})
		if err := setStaffActionCardState(card.Token, staffCardDone); err != nil {
			log.Warnf("[StaffActions] mark card %s done: %v", card.Token, err)
		}

		done, skipped, failed, _ := staffSummaryTally(results)
		log.Infof("[StaffActions] %s by %d on %d: %d done, %d skipped, %d failed",
			card.Kind, card.Issuer, card.Target, done, skipped, failed)
	}()
}

// runStaffActionFanOut visits the linked groups, staffActionWorkers at a time, and
// records each group's result in progress as soon as it is known. One paced owner
// pass is shared, so the Staff Group's creator is asked about once.
//
// Every slot starts pending and each worker writes only its own. A worker that
// panics leaves its slot pending; the coordinator sweeps what is left into a
// failed line, so no group is ever dropped from the summary (STAFF-08).
func runStaffActionFanOut(
	ctx context.Context,
	b *gotgbot.Bot,
	card *staffActionCard,
	links []models.StaffGroupLink,
	newUntil int64,
	progress *staffActionProgress,
) {
	pass := newPacedStaffOwnerPass(func(run func() error) error {
		return staffPaced(ctx, func(context.Context) error { return run() })
	})

	var workers errgroup.Group
	workers.SetLimit(staffActionWorkers)
	for i, link := range links {
		workers.Go(func() error {
			defer error_handling.RecoverFromPanic("staffActionWorker", "StaffActions")
			progress.set(i, runStaffActionInGroup(ctx, b, card, link, newUntil, pass))
			return nil
		})
	}
	_ = workers.Wait()
}

// runStaffActionInGroup is the whole per-group check chain. The order is part of
// the safety contract: a group that fails any step gets no write call, and the
// issuer's rights are read live before the target is even looked up.
func runStaffActionInGroup(
	ctx context.Context,
	b *gotgbot.Bot,
	card *staffActionCard,
	link models.StaffGroupLink,
	newUntil int64,
	pass *staffOwnerPass,
) staffGroupResult {
	result := func(reason staffReason, detail string) staffGroupResult {
		return staffGroupResult{Link: link, Outcome: staffReasonOutcome(reason), Reason: reason, Detail: detail}
	}

	// A shutdown ends the run between steps: a group that has not reached its write
	// call gets none, and reads "interrupted by restart".
	interrupted := func() bool { return ctx.Err() != nil }
	if interrupted() {
		return result(staffReasonFailInterrupted, "")
	}

	// Defensive: the database already forbids a link to the Staff Group itself.
	if link.GroupChatID == card.StaffChat {
		return result(staffReasonSkipStaffGroup, "")
	}

	switch recheckLink(b, link, pass) {
	case staffRecheckRemoved, staffRecheckGone:
		return result(staffReasonSkipLinkRemoved, "")
	case staffRecheckUnknown:
		if interrupted() {
			return result(staffReasonFailInterrupted, "")
		}
		return result(staffOwnerUnknownReason(b, link, pass), "")
	}
	if interrupted() {
		return result(staffReasonFailInterrupted, "")
	}

	// The only authority for the issuer in this group is this live answer.
	issuer, err := fetchLiveMember(ctx, b, link.GroupChatID, card.Issuer)
	if err != nil {
		log.Warnf("[StaffActions] issuer lookup in group %d: %v", link.GroupChatID, err)
		return result(classifyStaffLookupFailure(ctx, b, link.GroupChatID, err))
	}
	if reason := staffIssuerSkipReason(issuer); reason != "" {
		return result(reason, "")
	}

	if card.Target == b.Id {
		return result(staffReasonSkipTargetBot, "")
	}
	if staffServiceUserIDs[card.Target] {
		return result(staffReasonSkipTargetService, "")
	}

	if interrupted() {
		return result(staffReasonFailInterrupted, "")
	}
	target, err := fetchLiveMember(ctx, b, link.GroupChatID, card.Target)
	if err != nil {
		log.Warnf("[StaffActions] target lookup in group %d: %v", link.GroupChatID, err)
		return result(classifyStaffLookupFailure(ctx, b, link.GroupChatID, err))
	}

	verdict := decideStaffAction(card.Kind, staffTargetStateFrom(target), newUntil)
	if verdict.Call == staffCallNone {
		return result(verdict.Reason, "")
	}
	if interrupted() {
		return result(staffReasonFailInterrupted, "")
	}
	if err := executeStaffCall(ctx, b, link.GroupChatID, card.Target, verdict, newUntil); err != nil {
		log.Warnf("[StaffActions] %s in group %d: %v", card.Kind, link.GroupChatID, err)
		return result(classifyStaffFailure(ctx, b, link.GroupChatID, err))
	}
	return result(verdict.Reason, "")
}

// fetchLiveMember asks Telegram, live and uncached, for one member of one group.
// It is the only per-group authority source for staff actions: the admin cache
// and the command pipeline's checks answer for the chat a command was typed in,
// never for another group.
func fetchLiveMember(ctx context.Context, b *gotgbot.Bot, chatID, userID int64) (gotgbot.MergedChatMember, error) {
	var merged gotgbot.MergedChatMember
	err := staffPaced(ctx, func(ctx context.Context) error {
		callCtx, cancel := context.WithTimeout(ctx, staffActionCallTimeout)
		defer cancel()
		member, err := b.GetChatMemberWithContext(callCtx, chatID, userID, nil)
		if err != nil {
			return err
		}
		if member == nil {
			return errors.New("getChatMember returned no member")
		}
		merged = member.MergeChatMember()
		return nil
	})
	if err != nil {
		return gotgbot.MergedChatMember{}, err
	}
	return merged, nil
}

// staffIssuerSkipReason returns the empty reason when the issuer may restrict
// members in the group: its live creator, or an administrator holding the
// restrict right. Anything else is the reason the group is skipped.
func staffIssuerSkipReason(m gotgbot.MergedChatMember) staffReason {
	switch m.Status {
	case gotgbot.ChatMemberStatusCreator:
		return ""
	case gotgbot.ChatMemberStatusAdministrator:
		if m.CanRestrictMembers {
			return ""
		}
		return staffReasonSkipIssuerNoRight
	}
	return staffReasonSkipIssuerNotAdmin
}

// executeStaffCall makes the one write call a verdict asks for. The verdict's
// Call is the only thing that selects the Telegram method, so the decision table
// in decideStaffAction stays the single place that keeps a restrict or a
// member-removing unban away from a banned target.
func executeStaffCall(
	ctx context.Context,
	b *gotgbot.Bot,
	groupID, targetID int64,
	verdict staffVerdict,
	newUntil int64,
) error {
	// Each Telegram call is one paced unit with its own timeout. The closure is the
	// same on every retry, so a retried call carries identical parameters,
	// until_date included.
	paced := func(call func(callCtx context.Context) error) error {
		return staffPaced(ctx, func(ctx context.Context) error {
			callCtx, cancel := context.WithTimeout(ctx, staffActionCallTimeout)
			defer cancel()
			return call(callCtx)
		})
	}
	switch verdict.Call {
	case staffCallBan:
		return paced(func(callCtx context.Context) error {
			_, err := b.BanChatMemberWithContext(callCtx, groupID, targetID, &gotgbot.BanChatMemberOpts{UntilDate: newUntil})
			return err
		})
	case staffCallMute:
		return paced(func(callCtx context.Context) error {
			_, err := b.RestrictChatMemberWithContext(callCtx, groupID, targetID, MutedPermissions,
				&gotgbot.RestrictChatMemberOpts{UntilDate: newUntil})
			return err
		})
	case staffCallKick:
		// The same call per-group /kick makes: it removes a current member without
		// leaving a ban, so they can rejoin.
		return paced(func(callCtx context.Context) error {
			_, err := b.UnbanChatMemberWithContext(callCtx, groupID, targetID, &gotgbot.UnbanChatMemberOpts{OnlyIfBanned: false})
			return err
		})
	case staffCallUnban:
		return paced(func(callCtx context.Context) error {
			_, err := b.UnbanChatMemberWithContext(callCtx, groupID, targetID, &gotgbot.UnbanChatMemberOpts{OnlyIfBanned: true})
			return err
		})
	case staffCallUnmute:
		// Like per-group /unmute, the group's default permissions come from a live
		// getChat and a failure there fails the group. The getChat and the restrict
		// are two paced calls.
		var info *gotgbot.ChatFullInfo
		if err := paced(func(callCtx context.Context) error {
			var err error
			info, err = b.GetChatWithContext(callCtx, groupID, nil)
			return err
		}); err != nil {
			return err
		}
		return paced(func(callCtx context.Context) error {
			_, err := b.RestrictChatMemberWithContext(callCtx, groupID, targetID, resolveUnmutePermissions(info), nil)
			return err
		})
	}
	return fmt.Errorf("unknown staff call %d", verdict.Call)
}

// staffOwnerUnknownReason says why recheckLink could not judge a link: a rate
// limit that outlasted the retries, or any other missing answer. It reads the
// pass's memoised answers, so it makes no new call in the usual case.
func staffOwnerUnknownReason(b *gotgbot.Bot, link models.StaffGroupLink, pass *staffOwnerPass) staffReason {
	groupResult, _, groupErr := pass.check(b, link.GroupChatID, link.OwnerUserID)
	if errors.Is(groupErr, ratelimit.ErrRateLimited) {
		return staffReasonFailRateLimited
	}
	// recheckLink asks about the Staff Group only when the group side was not a
	// mismatch, so this repeats its own question and nothing more.
	if groupResult != chat_status.OwnerMismatch {
		_, _, staffErr := pass.check(b, link.StaffChatID, link.OwnerUserID)
		if errors.Is(staffErr, ratelimit.ErrRateLimited) {
			return staffReasonFailRateLimited
		}
	}
	return staffReasonFailOwnerUnknown
}

// probeStaffBot asks Telegram, through the pacer, for the bot's own live rights in
// a group. Anything but a definite answer is BotMemberUnknown.
func probeStaffBot(ctx context.Context, b *gotgbot.Bot, groupID int64) (gotgbot.MergedChatMember, chat_status.BotMemberResult) {
	var member gotgbot.MergedChatMember
	result := chat_status.BotMemberUnknown
	err := staffPaced(ctx, func(context.Context) error {
		var err error
		member, result, err = chat_status.FetchBotMember(b, groupID)
		return err
	})
	if err != nil {
		return gotgbot.MergedChatMember{}, chat_status.BotMemberUnknown
	}
	return member, result
}

// isStaffClientError reports whether err is a Telegram 400 or 403, the answers a
// missing right or a missing bot produces and the only ones worth a probe.
func isStaffClientError(err error) bool {
	var tgErr *gotgbot.TelegramError
	return errors.As(err, &tgErr) && (tgErr.Code == 400 || tgErr.Code == 403)
}

// classifyStaffFailure turns a failed write call into a reason a person can act
// on (D-19). A rate limit that outlasted the retries is its own reason. A 400 or
// 403 is explained by a live, paced probe of the bot's rights in that group, never
// by matching error text: the bot is gone, is not an admin, or lacks the
// restrict right. Anything else is shown as Telegram's own, escaped text.
func classifyStaffFailure(ctx context.Context, b *gotgbot.Bot, groupID int64, err error) (staffReason, string) {
	if ctx.Err() != nil {
		return staffReasonFailInterrupted, ""
	}
	if errors.Is(err, ratelimit.ErrRateLimited) {
		return staffReasonFailRateLimited, ""
	}
	if isStaffClientError(err) {
		member, result := probeStaffBot(ctx, b, groupID)
		switch result {
		case chat_status.BotMemberMissing:
			return staffReasonFailGroupNotFound, ""
		case chat_status.BotMemberFound:
			switch member.Status {
			case gotgbot.ChatMemberStatusAdministrator:
				if !member.CanRestrictMembers {
					return staffReasonFailBotNoRights, ""
				}
			case gotgbot.ChatMemberStatusCreator:
				// The bot cannot own a group, so there is nothing to explain.
			default:
				return staffReasonFailBotNotAdmin, ""
			}
		}
	}
	return staffReasonFailTelegram, telegramErrorDetail(err)
}

// classifyStaffLookupFailure is classifyStaffFailure for a failed getChatMember of
// the issuer or the target: a rate limit and a bot that is gone are told apart,
// and every other failure stays "could not check members".
func classifyStaffLookupFailure(ctx context.Context, b *gotgbot.Bot, groupID int64, err error) (staffReason, string) {
	if ctx.Err() != nil {
		return staffReasonFailInterrupted, ""
	}
	if errors.Is(err, ratelimit.ErrRateLimited) {
		return staffReasonFailRateLimited, ""
	}
	if isStaffClientError(err) {
		if _, result := probeStaffBot(ctx, b, groupID); result == chat_status.BotMemberMissing {
			return staffReasonFailGroupNotFound, ""
		}
	}
	return staffReasonFailLookup, telegramErrorDetail(err)
}

// telegramErrorDetail is the short, HTML-escaped text shown on a failed line:
// Telegram's own description when there is one, cut to 120 runes.
func telegramErrorDetail(err error) string {
	if err == nil {
		return ""
	}
	text := err.Error()
	var tgErr *gotgbot.TelegramError
	if errors.As(err, &tgErr) {
		text = tgErr.Description
	}
	if utf8.RuneCountInString(text) > staffTelegramDetailRunes {
		text = string([]rune(text)[:staffTelegramDetailRunes])
	}
	return html.EscapeString(text)
}
