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
	// staffActionsCtx is the context every run starts with; a later plan cancels it
	// on shutdown.
	staffActionsCtx, staffActionsCancel = context.WithCancel(context.Background()) //nolint:unused // the shutdown drain that calls it arrives in a later plan
)

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
	staffActionRunsWG.Add(1)
	go func() {
		defer staffActionRunsWG.Done()
		defer error_handling.RecoverFromPanic("staffActionRun", "StaffActions")
		// Frees the target for the next staff action once this run is over, a panic
		// included. The lock's TTL is the safety net for a crash.
		defer releaseStaffTargetLock(card.Target, card.Token)

		tr := staffChatTranslator(card.StaffChat)
		results := runStaffActionFanOut(staffActionsCtx, b, card, links, newUntil)
		deliverStaffActionSummary(b, chatID, msgID, renderStaffActionSummary(tr, card, results))
		if err := setStaffActionCardState(card.Token, staffCardDone); err != nil {
			log.Warnf("[StaffActions] mark card %s done: %v", card.Token, err)
		}

		var done, skipped, failed int
		for _, res := range results {
			switch res.Outcome {
			case staffOutcomeDone:
				done++
			case staffOutcomeSkipped:
				skipped++
			default:
				failed++
			}
		}
		log.Infof("[StaffActions] %s by %d on %d: %d done, %d skipped, %d failed",
			card.Kind, card.Issuer, card.Target, done, skipped, failed)
	}()
}

// runStaffActionFanOut visits the linked groups, staffActionWorkers at a time, and
// returns one result per group, in link order. One paced owner pass is shared, so
// the Staff Group's creator is asked about once.
//
// Every slot starts pending and each worker writes only its own. A worker that
// panics leaves its slot pending, and the sweep after the wait turns it into a
// failed line, so no group is ever dropped from the summary (STAFF-08).
func runStaffActionFanOut(
	ctx context.Context,
	b *gotgbot.Bot,
	card *staffActionCard,
	links []models.StaffGroupLink,
	newUntil int64,
) []staffGroupResult {
	pass := newPacedStaffOwnerPass(func(run func() error) error {
		return staffPaced(ctx, func(context.Context) error { return run() })
	})
	results := pendingResults(links)

	var workers errgroup.Group
	workers.SetLimit(staffActionWorkers)
	for i, link := range links {
		workers.Go(func() error {
			defer error_handling.RecoverFromPanic("staffActionWorker", "StaffActions")
			results[i] = runStaffActionInGroup(ctx, b, card, link, newUntil, pass)
			return nil
		})
	}
	_ = workers.Wait()

	for i := range results {
		if results[i].Outcome == staffOutcomePending {
			log.Errorf("[StaffActions] group %d finished without a result; reporting it as failed", results[i].Link.GroupChatID)
			results[i].Outcome = staffReasonOutcome(staffReasonFailInternal)
			results[i].Reason = staffReasonFailInternal
		}
	}
	return results
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

	// Defensive: the database already forbids a link to the Staff Group itself.
	if link.GroupChatID == card.StaffChat {
		return result(staffReasonSkipStaffGroup, "")
	}

	switch recheckLink(b, link, pass) {
	case staffRecheckRemoved, staffRecheckGone:
		return result(staffReasonSkipLinkRemoved, "")
	case staffRecheckUnknown:
		return result(staffOwnerUnknownReason(b, link, pass), "")
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

	target, err := fetchLiveMember(ctx, b, link.GroupChatID, card.Target)
	if err != nil {
		log.Warnf("[StaffActions] target lookup in group %d: %v", link.GroupChatID, err)
		return result(classifyStaffLookupFailure(ctx, b, link.GroupChatID, err))
	}

	verdict := decideStaffAction(card.Kind, staffTargetStateFrom(target), newUntil)
	if verdict.Call == staffCallNone {
		return result(verdict.Reason, "")
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
