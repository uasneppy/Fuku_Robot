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

	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/utils/error_handling"
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

// runStaffActionFanOut visits the linked groups one after another and returns
// one result per group, in link order. One owner pass is shared, so the Staff
// Group's creator is asked about once.
func runStaffActionFanOut(
	ctx context.Context,
	b *gotgbot.Bot,
	card *staffActionCard,
	links []models.StaffGroupLink,
	newUntil int64,
) []staffGroupResult {
	pass := newStaffOwnerPass()
	results := make([]staffGroupResult, len(links))
	for i, link := range links {
		results[i] = runStaffActionInGroup(ctx, b, card, link, newUntil, pass)
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
		return result(staffReasonFailOwnerUnknown, "")
	}

	// The only authority for the issuer in this group is this live answer.
	issuer, err := fetchLiveMember(ctx, b, link.GroupChatID, card.Issuer)
	if err != nil {
		log.Warnf("[StaffActions] issuer lookup in group %d: %v", link.GroupChatID, err)
		return result(staffReasonFailLookup, telegramErrorDetail(err))
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
		return result(staffReasonFailLookup, telegramErrorDetail(err))
	}

	verdict := decideStaffAction(card.Kind, staffTargetStateFrom(target), newUntil)
	if verdict.Call == staffCallNone {
		return result(verdict.Reason, "")
	}
	if err := executeStaffCall(ctx, b, link.GroupChatID, card.Target, verdict, newUntil); err != nil {
		log.Warnf("[StaffActions] %s in group %d: %v", card.Kind, link.GroupChatID, err)
		return result(staffReasonFailTelegram, telegramErrorDetail(err))
	}
	return result(verdict.Reason, "")
}

// fetchLiveMember asks Telegram, live and uncached, for one member of one group.
// It is the only per-group authority source for staff actions: the admin cache
// and the command pipeline's checks answer for the chat a command was typed in,
// never for another group.
func fetchLiveMember(ctx context.Context, b *gotgbot.Bot, chatID, userID int64) (gotgbot.MergedChatMember, error) {
	callCtx, cancel := context.WithTimeout(ctx, staffActionCallTimeout)
	defer cancel()
	member, err := b.GetChatMemberWithContext(callCtx, chatID, userID, nil)
	if err != nil {
		return gotgbot.MergedChatMember{}, err
	}
	if member == nil {
		return gotgbot.MergedChatMember{}, errors.New("getChatMember returned no member")
	}
	return member.MergeChatMember(), nil
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
	callCtx, cancel := context.WithTimeout(ctx, staffActionCallTimeout)
	defer cancel()
	switch verdict.Call {
	case staffCallBan:
		_, err := b.BanChatMemberWithContext(callCtx, groupID, targetID, &gotgbot.BanChatMemberOpts{UntilDate: newUntil})
		return err
	case staffCallMute:
		_, err := b.RestrictChatMemberWithContext(callCtx, groupID, targetID, MutedPermissions,
			&gotgbot.RestrictChatMemberOpts{UntilDate: newUntil})
		return err
	case staffCallKick:
		// The same call per-group /kick makes: it removes a current member without
		// leaving a ban, so they can rejoin.
		_, err := b.UnbanChatMemberWithContext(callCtx, groupID, targetID, &gotgbot.UnbanChatMemberOpts{OnlyIfBanned: false})
		return err
	case staffCallUnban:
		_, err := b.UnbanChatMemberWithContext(callCtx, groupID, targetID, &gotgbot.UnbanChatMemberOpts{OnlyIfBanned: true})
		return err
	case staffCallUnmute:
		// Like per-group /unmute, the group's default permissions come from a live
		// getChat and a failure there fails the group.
		info, err := b.GetChatWithContext(callCtx, groupID, nil)
		if err != nil {
			return err
		}
		_, err = b.RestrictChatMemberWithContext(callCtx, groupID, targetID, resolveUnmutePermissions(info), nil)
		return err
	}
	return fmt.Errorf("unknown staff call %d", verdict.Call)
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
