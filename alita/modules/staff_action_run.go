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
	"github.com/divkix/Alita_Robot/alita/db/staff"
	"github.com/divkix/Alita_Robot/alita/i18n"
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
	// when a progress edit is held or the final edit waits to be retried.
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

// markDirty says the card does not show the current results, so the next
// snapshot reports a change. The coordinator calls it after a rate-limited edit.
func (p *staffActionProgress) markDirty() {
	p.mu.Lock()
	defer p.mu.Unlock()
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

// staffRunSpec describes one staff run for startStaffRun: the card it belongs to,
// the groups it visits, the message it keeps up to date, and the functions that do
// the per-group work and render the text. Phase 2's action run is the first spec;
// the undo run is another. Everything else (the one writer of the message, the
// throttled edits, the 429 hold, the lock renewal, the shutdown sweep and the final
// delivery) belongs to the engine and is the same for every run.
type staffRunSpec struct {
	// Card owns the target lock the run releases, names the Staff Group whose
	// language the text is rendered in, and supplies the final log line.
	Card *staffActionCard
	// Links are the linked groups the run visits, in the order the summary lists them.
	Links []models.StaffGroupLink
	// ChatID and MsgID are the message the coordinator edits and finally replaces
	// with the summary.
	ChatID, MsgID int64
	// Group does the work for group i and returns how it ended. It runs in a worker
	// goroutine and gets the run's shared paced owner pass.
	Group func(ctx context.Context, i int, pass *staffOwnerPass) staffGroupResult
	// AfterGroup runs in the same worker right after group i's result is set in the
	// run's progress, so the summary already shows it. It may be nil. It must not
	// change the result.
	AfterGroup func(ctx context.Context, i int, res staffGroupResult)
	// Render builds the card text for a set of results: with final false, a progress
	// text whose pending lines may collapse; with final true, the last text plus any
	// continuation messages.
	Render func(tr *i18n.Translator, results []staffGroupResult, final bool) (string, []string)
	// Finish runs once after the unfinished groups are swept into failed lines and
	// before the final summary is delivered. It returns the keyboard the final
	// summary carries, an empty markup for none. It may be nil.
	Finish func(results []staffGroupResult) gotgbot.InlineKeyboardMarkup
	// Delivered runs after the final summary was delivered with the ID of the
	// message that holds it, 0 when it could not be delivered at all. It may be nil.
	Delivered func(landedMsgID int64)
}

// startStaffRun runs spec in the background: it fans out over spec.Links, keeps the
// message current, then puts the final summary on it and marks the card done. One
// coordinator goroutine per run is the only writer of the message.
func startStaffRun(b *gotgbot.Bot, spec staffRunSpec) {
	card := spec.Card
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
		progress := newStaffActionProgress(spec.Links)

		// The fan-out runs in its own goroutine so this one stays the only writer of
		// the card from here on: it alone edits the message, on a timer.
		fanOutDone := make(chan struct{})
		go func() {
			defer error_handling.RecoverFromPanic("staffActionFanOut", "StaffActions")
			defer close(fanOutDone)
			runStaffFanOut(ctx, b, spec, progress)
		}()

		// editHoldUntil is when Telegram's last retry_after for a progress edit of this
		// card has passed. Only this goroutine reads and writes it.
		var editHoldUntil time.Time
		ticker := time.NewTicker(staffActionEditEvery)
		defer ticker.Stop()
		// The lock guards the group writes, so it is renewed only while the fan-out
		// runs: none happen after it, and the deferred release frees the lock once
		// the summary is delivered.
		lockRenew := time.NewTicker(staffTargetLockRenewEvery)
		defer lockRenew.Stop()
	progressLoop:
		for {
			select {
			case <-fanOutDone:
				break progressLoop
			case <-lockRenew.C:
				held, err := renewStaffTargetLock(card.Target, card.Token)
				if err != nil {
					log.Warnf("[StaffActions] renew target lock %d for card %s: %v", card.Target, card.Token, err)
				} else if !held {
					log.Warnf("[StaffActions] target lock %d is no longer held by card %s", card.Target, card.Token)
				}
			case <-ticker.C:
				// Inside a hold the tick is skipped before the snapshot, so the changes
				// stay marked and the newest results go out on the first tick after it.
				if time.Now().Before(editHoldUntil) {
					continue
				}
				snapshot, dirty := progress.snapshot()
				if !dirty {
					continue
				}
				text, _ := spec.Render(tr, snapshot, false)
				err := editStaffActionMessage(b, spec.ChatID, spec.MsgID, text)
				if err == nil {
					continue
				}
				// A failed progress edit is skipped; the next tick or the final summary
				// carries the same information. A 429 also holds the card and keeps
				// the snapshot marked as unsent. Any other error is not re-marked, so
				// a deleted card is not edited on every tick.
				if hold, limited := staffRetryAfterHold(err); limited {
					editHoldUntil = time.Now().Add(hold)
					progress.markDirty()
					log.Warnf("[StaffActions] progress edit of card %s rate limited, holding edits for %s: %v",
						card.Token, hold, err)
					continue
				}
				log.Warnf("[StaffActions] progress edit of card %s: %v", card.Token, err)
			}
		}
		ticker.Stop()
		lockRenew.Stop()

		// A group that never reported was cut off by a shutdown or lost to a panic.
		sweep := staffReasonFailInternal
		if ctx.Err() != nil {
			sweep = staffReasonFailInterrupted
		}
		results := progress.sweepPending(sweep)
		// The record is closed before anything is shown: its writes use db.DB, never
		// ctx, so a shutdown that cancelled the run cannot lose them.
		var markup gotgbot.InlineKeyboardMarkup
		if spec.Finish != nil {
			markup = spec.Finish(results)
		}

		final, continuation := spec.Render(tr, results, true)
		// Delivery never runs on the run's own context, which a shutdown may already
		// have cancelled: each message opens a fresh budget of its own.
		landed := deliverStaffActionFinal(b, spec.ChatID, spec.MsgID, final, continuation, editHoldUntil, markup)
		if spec.Delivered != nil {
			spec.Delivered(landed)
		}
		if err := setStaffActionCardState(card.Token, staffCardDone); err != nil {
			log.Warnf("[StaffActions] mark card %s done: %v", card.Token, err)
		}

		done, skipped, failed, _ := staffSummaryTally(results)
		log.Infof("[StaffActions] %s by %d on %d: %d done, %d skipped, %d failed",
			card.Kind, card.Issuer, card.Target, done, skipped, failed)
	}()
}

// startStaffActionRun runs a confirmed staff action as the first staffRunSpec: each
// group goes through the per-group check chain, its result is recorded and, when
// the action was applied there, posted to the group's log channel.
func startStaffActionRun(
	b *gotgbot.Bot,
	card *staffActionCard,
	links []models.StaffGroupLink,
	newUntil int64,
	chatID, msgID int64,
) {
	startStaffRun(b, staffRunSpec{
		Card:   card,
		Links:  links,
		ChatID: chatID,
		MsgID:  msgID,
		Group: func(ctx context.Context, i int, pass *staffOwnerPass) staffGroupResult {
			return runStaffActionInGroup(ctx, b, card, links[i], newUntil, pass)
		},
		AfterGroup: func(ctx context.Context, _ int, res staffGroupResult) {
			saveStaffGroupResult(card, res)
			// The post comes after the record write and never changes the result.
			if res.Outcome == staffOutcomeDone {
				postStaffActionLog(ctx, b, card, res.Link)
			}
		},
		Render: func(tr *i18n.Translator, results []staffGroupResult, final bool) (string, []string) {
			return composeStaffActionSummary(tr, card, results, final)
		},
		Finish: func(results []staffGroupResult) gotgbot.InlineKeyboardMarkup {
			finalized := finalizeStaffActionRecord(card, results)
			// The Undo button exists only on the final text of a record that was
			// finalized: Ask and Confirm both read the record, so a run whose record
			// could not be closed offers nothing to press. Kick has no undo.
			if !finalized || card.Kind == staffKindKick {
				return gotgbot.InlineKeyboardMarkup{}
			}
			if done, _, _, _ := staffSummaryTally(results); done == 0 {
				return gotgbot.InlineKeyboardMarkup{}
			}
			keyboard, _ := staffUndoKeyboard(staffChatTranslator(card.StaffChat), card.ActionID)
			return keyboard
		},
		Delivered: func(landed int64) {
			// A fallback message is where staff read the summary now, so the record
			// follows it: the "Undone" line must land on that message.
			if landed == 0 || landed == msgID || card.ActionID == 0 {
				return
			}
			if err := staff.SetSummaryMessage(card.ActionID, chatID, landed); err != nil {
				log.Errorf("[StaffActions] point action %d at its summary message %d: %v", card.ActionID, landed, err)
			}
		},
	})
}

// runStaffFanOut visits spec.Links, staffActionWorkers at a time, and records each
// group's result in progress as soon as it is known. One paced owner pass is
// shared, so the Staff Group's creator is asked about once.
//
// Every slot starts pending and each worker writes only its own. A worker that
// panics leaves its slot pending; the coordinator sweeps what is left into a
// failed line, so no group is ever dropped from the summary (STAFF-08).
func runStaffFanOut(
	ctx context.Context,
	b *gotgbot.Bot,
	spec staffRunSpec,
	progress *staffActionProgress,
) {
	pass := newPacedStaffOwnerPass(func(run func() error) error {
		return staffPaced(ctx, func(context.Context) error { return run() })
	})

	var workers errgroup.Group
	workers.SetLimit(staffActionWorkers)
	for i := range spec.Links {
		workers.Go(func() error {
			defer error_handling.RecoverFromPanic("staffActionWorker", "StaffActions")
			res := spec.Group(ctx, i, pass)
			progress.set(i, res)
			if spec.AfterGroup != nil {
				spec.AfterGroup(ctx, i, res)
			}
			return nil
		})
	}
	_ = workers.Wait()
}

// staffGroupPrechecks is the first half of the per-group check chain, shared by the
// staff action and its undo: the interrupted check, the Staff Group guard, the link
// owner recheck, the actor's live rights, the bot and service-ID guards and the
// target's live state. The actor is card.Issuer, which is the issuer of an action and
// the member who pressed Undo for an undo card. It returns the live target and true,
// or the group's early result and false. The order is part of the safety contract: a
// group that fails any step gets no write call, and the actor's rights are read live
// before the target is even looked up.
func staffGroupPrechecks(
	ctx context.Context,
	b *gotgbot.Bot,
	card *staffActionCard,
	link models.StaffGroupLink,
	pass *staffOwnerPass,
) (gotgbot.MergedChatMember, staffGroupResult, bool) {
	early := func(reason staffReason, detail string) (gotgbot.MergedChatMember, staffGroupResult, bool) {
		return gotgbot.MergedChatMember{}, staffGroupResult{
			Link: link, Outcome: staffReasonOutcome(reason), Reason: reason, Detail: detail,
		}, false
	}

	// A shutdown ends the run between steps: a group that has not reached its write
	// call gets none, and reads "interrupted by restart".
	interrupted := func() bool { return ctx.Err() != nil }
	if interrupted() {
		return early(staffReasonFailInterrupted, "")
	}

	// Defensive: the database already forbids a link to the Staff Group itself.
	if link.GroupChatID == card.StaffChat {
		return early(staffReasonSkipStaffGroup, "")
	}

	switch recheckLink(b, link, pass) {
	case staffRecheckRemoved, staffRecheckGone:
		return early(staffReasonSkipLinkRemoved, "")
	case staffRecheckUnknown:
		if interrupted() {
			return early(staffReasonFailInterrupted, "")
		}
		return early(staffOwnerUnknownReason(b, link, pass), "")
	}
	if interrupted() {
		return early(staffReasonFailInterrupted, "")
	}

	// The only authority for the actor in this group is this live answer.
	issuer, err := fetchLiveMember(ctx, b, link.GroupChatID, card.Issuer)
	if err != nil {
		log.Warnf("[StaffActions] issuer lookup in group %d: %v", link.GroupChatID, err)
		return early(classifyStaffLookupFailure(ctx, b, link.GroupChatID, err))
	}
	if reason := staffIssuerSkipReason(issuer); reason != "" {
		return early(reason, "")
	}

	if card.Target == b.Id {
		return early(staffReasonSkipTargetBot, "")
	}
	if staffServiceUserIDs[card.Target] {
		return early(staffReasonSkipTargetService, "")
	}

	if interrupted() {
		return early(staffReasonFailInterrupted, "")
	}
	target, err := fetchLiveMember(ctx, b, link.GroupChatID, card.Target)
	if err != nil {
		log.Warnf("[StaffActions] target lookup in group %d: %v", link.GroupChatID, err)
		return early(classifyStaffLookupFailure(ctx, b, link.GroupChatID, err))
	}
	return target, staffGroupResult{}, true
}

// runStaffActionInGroup is the whole per-group check chain of a staff action: the
// shared prechecks, then the decision and the write. The order is part of the safety
// contract: a group that fails any step gets no write call, and the issuer's rights
// are read live before the target is even looked up.
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
	interrupted := func() bool { return ctx.Err() != nil }

	target, skipped, ok := staffGroupPrechecks(ctx, b, card, link, pass)
	if !ok {
		return skipped
	}

	verdict := decideStaffAction(card.Kind, staffTargetStateFrom(target), newUntil)
	if verdict.Call == staffCallNone {
		return result(verdict.Reason, "")
	}
	// Write-ahead: the target's state in this group, from the very read the verdict
	// was made on, is committed before the write call. If that cannot be stored the
	// group gets no write, because an action whose prior state is lost can never be
	// undone correctly (D-02).
	priorRow, err := staffPriorFromMember(target).row()
	if err == nil {
		err = staffSavePrior(card.ActionID, link.GroupChatID, priorRow)
	}
	if err != nil {
		log.Errorf("[StaffActions] save prior state in group %d: %v", link.GroupChatID, err)
		return result(staffReasonFailInternal, "")
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
