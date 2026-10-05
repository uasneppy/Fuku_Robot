package modules

import (
	"context"
	"html"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/i18n"
	"github.com/divkix/Alita_Robot/alita/utils/formatting"
	"github.com/divkix/Alita_Robot/alita/utils/ratelimit"
)

const (
	// staffErrorToken stands in for Telegram's error text while a failure reason is
	// translated; the text is spliced in afterwards.
	staffErrorToken = "<<staff-error>>"
	// staffReasonToken stands in for the reason inside a skipped or failed line.
	staffReasonToken = "<<staff-reason>>"
)

// staffActionEditTimeout bounds one edit of the card message.
const staffActionEditTimeout = 10 * time.Second

// staffActionIcon is the icon in front of an action's header line. The icons are
// not localized.
func staffActionIcon(kind staffActionKind) string {
	switch kind {
	case staffKindBan:
		return "🔨"
	case staffKindMute:
		return "🔇"
	case staffKindKick:
		return "👢"
	case staffKindUnban:
		return "🔓"
	case staffKindUnmute:
		return "🔊"
	}
	return "⚙️"
}

// staffActionName is the localized name of an action. Every case reads a literal
// locale key so make check-translations sees it.
func staffActionName(tr *i18n.Translator, kind staffActionKind) string {
	switch kind {
	case staffKindBan:
		text, _ := tr.GetString("staff_act_name_ban")
		return text
	case staffKindMute:
		text, _ := tr.GetString("staff_act_name_mute")
		return text
	case staffKindKick:
		text, _ := tr.GetString("staff_act_name_kick")
		return text
	case staffKindUnban:
		text, _ := tr.GetString("staff_act_name_unban")
		return text
	case staffKindUnmute:
		text, _ := tr.GetString("staff_act_name_unmute")
		return text
	}
	return ""
}

// staffActionHasDuration reports whether an action has an end date to show:
// only ban and mute do.
func staffActionHasDuration(kind staffActionKind) bool {
	return kind == staffKindBan || kind == staffKindMute
}

// staffDurationLabel is the duration segment of the header: the amount and unit
// exactly as typed ("2 day(s)"), "permanent", or the over-limit wording. It is what
// makes a misparsed duration visible before Confirm. Every case reads a literal
// locale key so make check-translations sees it.
func staffDurationLabel(tr *i18n.Translator, card *staffActionCard) string {
	var text string
	switch {
	case card.OverLimit:
		text, _ = tr.GetString("staff_act_duration_over_limit")
	case card.DurationSec == 0:
		text, _ = tr.GetString("staff_act_duration_permanent")
	default:
		params := i18n.TranslationParams{"n": card.DurationAmount}
		switch card.DurationUnit {
		case "m":
			text, _ = tr.GetString("staff_act_duration_minutes", params)
		case "h":
			text, _ = tr.GetString("staff_act_duration_hours", params)
		case "d":
			text, _ = tr.GetString("staff_act_duration_days", params)
		case "w":
			text, _ = tr.GetString("staff_act_duration_weeks", params)
		default:
			text, _ = tr.GetString("staff_act_duration_permanent")
		}
	}
	return text
}

// staffTargetDisplay shows the target as its stored name in bold followed by the
// ID in a code tag. A target the bot has never seen reads "unknown name".
func staffTargetDisplay(tr *i18n.Translator, card *staffActionCard) string {
	name := strings.TrimSpace(card.TargetName)
	if name == "" {
		name, _ = tr.GetString("staff_act_target_unknown_name")
		return "<b>" + html.EscapeString(name) + "</b> (<code>" + strconv.FormatInt(card.Target, 10) + "</code>)"
	}
	return "<b>" + staffDisplayTitle(name) + "</b> (<code>" + strconv.FormatInt(card.Target, 10) + "</code>)"
}

// staffActionHeader is the first line of the card and of the summary: icon,
// action, target, duration (ban and mute only) and reason. It is built by
// concatenation, so no user text goes through the translator.
func staffActionHeader(tr *i18n.Translator, card *staffActionCard) string {
	reason := html.EscapeString(card.Reason)
	if strings.TrimSpace(card.Reason) == "" {
		reason, _ = tr.GetString("staff_act_no_reason")
	}
	header := staffActionIcon(card.Kind) + " " + staffActionName(tr, card.Kind) + " · " + staffTargetDisplay(tr, card)
	if staffActionHasDuration(card.Kind) {
		header += " · " + staffDurationLabel(tr, card)
	}
	return header + " · " + reason
}

// staffReasonText is the localized words for a result reason. Telegram's error
// text is passed as detail, already escaped, and spliced in after translation.
func staffReasonText(tr *i18n.Translator, r staffReason, detail string) string {
	var text string
	switch r {
	case staffReasonBanned, staffReasonMuted, staffReasonKicked, staffReasonUnbanned, staffReasonUnmuted:
		return ""
	case staffReasonBannedNotInGroup:
		text, _ = tr.GetString("staff_act_banned_not_in_group")
	case staffReasonSkipIssuerNotAdmin:
		text, _ = tr.GetString("staff_act_skip_issuer_not_admin")
	case staffReasonSkipIssuerNoRight:
		text, _ = tr.GetString("staff_act_skip_issuer_no_right")
	case staffReasonSkipTargetAdmin:
		text, _ = tr.GetString("staff_act_skip_target_admin")
	case staffReasonSkipTargetBot:
		text, _ = tr.GetString("staff_act_skip_target_bot")
	case staffReasonSkipTargetService:
		text, _ = tr.GetString("staff_act_skip_target_service")
	case staffReasonSkipAlreadyBanned:
		text, _ = tr.GetString("staff_act_skip_already_banned")
	case staffReasonSkipNotInGroup:
		text, _ = tr.GetString("staff_act_skip_not_in_group")
	case staffReasonSkipAlreadyMuted:
		text, _ = tr.GetString("staff_act_skip_already_muted")
	case staffReasonSkipNotBanned:
		text, _ = tr.GetString("staff_act_skip_not_banned")
	case staffReasonSkipNotMuted:
		text, _ = tr.GetString("staff_act_skip_not_muted")
	case staffReasonSkipLinkRemoved:
		text, _ = tr.GetString("staff_act_skip_link_removed")
	case staffReasonSkipStaffGroup:
		text, _ = tr.GetString("staff_act_skip_staff_group")
	case staffReasonFailOwnerUnknown:
		text, _ = tr.GetString("staff_act_fail_owner_unknown")
	case staffReasonFailLookup:
		text, _ = tr.GetString("staff_act_fail_lookup")
	case staffReasonFailTelegram:
		text, _ = tr.GetString("staff_act_fail_telegram", i18n.TranslationParams{"error": staffErrorToken})
		text = strings.Replace(text, staffErrorToken, detail, 1)
	case staffReasonFailRateLimited:
		text, _ = tr.GetString("staff_act_fail_rate_limited")
	case staffReasonFailBotNotAdmin:
		text, _ = tr.GetString("staff_act_fail_bot_not_admin")
	case staffReasonFailBotNoRights:
		text, _ = tr.GetString("staff_act_fail_bot_no_rights")
	case staffReasonFailGroupNotFound:
		text, _ = tr.GetString("staff_act_fail_group_not_found")
	case staffReasonFailInternal:
		text, _ = tr.GetString("staff_act_fail_internal")
	case staffReasonFailInterrupted:
		text, _ = tr.GetString("staff_act_fail_interrupted")
	}
	return text
}

// staffResultLine is one group's line of the summary: an icon and the escaped
// title, plus the reason for anything that was not a plain success.
func staffResultLine(tr *i18n.Translator, res staffGroupResult) string {
	title := staffDisplayTitle(res.Link.GroupTitle)
	switch res.Outcome {
	case staffOutcomeDone:
		line := "✅ " + title
		if reason := staffReasonText(tr, res.Reason, res.Detail); reason != "" {
			line += ": " + reason
		}
		return line
	case staffOutcomeSkipped:
		text, _ := tr.GetString("staff_act_line_skipped", i18n.TranslationParams{"reason": staffReasonToken})
		return "⏭ " + title + ": " + strings.Replace(text, staffReasonToken, staffReasonText(tr, res.Reason, res.Detail), 1)
	case staffOutcomeFailed:
		text, _ := tr.GetString("staff_act_line_failed", i18n.TranslationParams{"reason": staffReasonToken})
		return "❌ " + title + ": " + strings.Replace(text, staffReasonToken, staffReasonText(tr, res.Reason, res.Detail), 1)
	}
	return "⏳ " + title
}

// staffSummaryLen is the length of text in UTF-16 code units, the unit Telegram's
// message cap is counted in. It is meant for the final HTML, after every title,
// name and reason has been escaped.
func staffSummaryLen(text string) int {
	return len(utf16.Encode([]rune(text)))
}

// staffSummaryFits reports whether the final HTML text is within the length cap.
func staffSummaryFits(text string) bool {
	return staffSummaryLen(text) <= staffPanelMaxUTF16
}

// staffSummaryTally counts the results by outcome.
func staffSummaryTally(results []staffGroupResult) (done, skipped, failed, pending int) {
	for _, res := range results {
		switch res.Outcome {
		case staffOutcomeDone:
			done++
		case staffOutcomeSkipped:
			skipped++
		case staffOutcomeFailed:
			failed++
		default:
			pending++
		}
	}
	return done, skipped, failed, pending
}

// staffSummaryTallyLine is the line under the header (D-17). The icons and the
// digits are not localized.
func staffSummaryTallyLine(done, skipped, failed int) string {
	return "✅ " + strconv.Itoa(done) + " · ⏭ " + strconv.Itoa(skipped) + " · ❌ " + strconv.Itoa(failed)
}

// staffSummaryText joins a head and the group lines. Lines are whole units built
// from escaped pieces, so a message is never cut in the middle of a tag.
func staffSummaryText(head string, lines []string) string {
	return head + "\n\n" + strings.Join(lines, "\n")
}

// fitStaffSummaryLines puts as many whole lines under head as the cap allows and
// ends the text with tail when it is not empty. It returns the text and how many
// lines went in. With atLeastOne a first line is always taken, so a caller that
// loops over the remaining lines always makes progress.
func fitStaffSummaryLines(head string, lines []string, tail string, atLeastOne bool) (string, int) {
	budget := staffPanelMaxUTF16 - staffSummaryLen(head) - len("\n\n")
	tailCost := 0
	if tail != "" {
		tailCost = len("\n") + staffSummaryLen(tail)
	}
	used, n := 0, 0
	for n < len(lines) {
		cost := staffSummaryLen(lines[n])
		if n > 0 {
			cost += len("\n")
		}
		if used+cost+tailCost > budget && (n > 0 || !atLeastOne) {
			break
		}
		used += cost
		n++
	}
	text := staffSummaryText(head, lines[:n])
	if tail != "" {
		if n > 0 {
			text += "\n"
		}
		text += tail
	}
	return text, n
}

// composeStaffActionSummary builds the summary text for a set of results. Lines
// keep their link order and only their icon changes (D-17). A text over the cap
// collapses the done lines into one count (D-18); no skipped or failed line is
// ever summarized, so no group is hidden (STAFF-08). When final is false the
// pending lines may collapse too, and what still does not fit is left for the
// final summary. When final is true the lines that do not fit in the first
// message are returned as continuation messages, each starting with the header.
func composeStaffActionSummary(
	tr *i18n.Translator,
	card *staffActionCard,
	results []staffGroupResult,
	final bool,
) (string, []string) {
	header := staffActionHeader(tr, card)
	done, skipped, failed, pending := staffSummaryTally(results)
	head := header + "\n" + staffSummaryTallyLine(done, skipped, failed)

	lines := make([]string, len(results))
	for i, res := range results {
		lines[i] = staffResultLine(tr, res)
	}
	if text := staffSummaryText(head, lines); staffSummaryFits(text) {
		return text, nil
	}

	var doneLine string
	if done > 0 {
		text, _ := tr.GetString("staff_act_summary_done_collapsed", i18n.TranslationParams{"count": done})
		doneLine = "✅ " + text
	}
	collapsed := make([]string, 0, len(lines))
	if doneLine != "" {
		collapsed = append(collapsed, doneLine)
	}
	for i, res := range results {
		if res.Outcome != staffOutcomeDone {
			collapsed = append(collapsed, lines[i])
		}
	}
	if text := staffSummaryText(head, collapsed); staffSummaryFits(text) {
		return text, nil
	}

	continued, _ := tr.GetString("staff_act_summary_continued")
	if !final {
		kept := make([]string, 0, len(lines))
		if doneLine != "" {
			kept = append(kept, doneLine)
		}
		if pending > 0 {
			text, _ := tr.GetString("staff_act_summary_pending_collapsed", i18n.TranslationParams{"count": pending})
			kept = append(kept, "⏳ "+text)
		}
		for i, res := range results {
			if res.Outcome == staffOutcomeSkipped || res.Outcome == staffOutcomeFailed {
				kept = append(kept, lines[i])
			}
		}
		if text, n := fitStaffSummaryLines(head, kept, "", false); n == len(kept) {
			return text, nil
		}
		text, _ := fitStaffSummaryLines(head, kept, continued, false)
		return text, nil
	}

	first, n := fitStaffSummaryLines(head, collapsed, continued, false)
	remaining := collapsed[n:]
	marker, _ := tr.GetString("staff_act_summary_continuation")
	continuationHead := header + "\n" + marker
	var parts []string
	for len(remaining) > 0 {
		if text := staffSummaryText(continuationHead, remaining); staffSummaryFits(text) {
			parts = append(parts, text)
			break
		}
		text, taken := fitStaffSummaryLines(continuationHead, remaining, continued, true)
		parts = append(parts, text)
		remaining = remaining[taken:]
	}
	return first, parts
}

// renderStaffActionSummary is the card text while the run is going and when it
// starts: the header, the tally, a blank line, then one line per linked group in
// link order. A text over the cap collapses done and then pending lines into
// counts and keeps every skipped and failed line it can.
func renderStaffActionSummary(tr *i18n.Translator, card *staffActionCard, results []staffGroupResult) string {
	text, _ := composeStaffActionSummary(tr, card, results, false)
	return text
}

// renderStaffActionSummaryFinal renders the last summary of a run. The caller
// guarantees no result is still pending. The first return value is the text of the
// card; the second holds the continuation messages, in order, when the skipped and
// failed lines alone do not fit in one message. Every group appears exactly once
// across all of them (STAFF-08).
func renderStaffActionSummaryFinal(tr *i18n.Translator, card *staffActionCard, results []staffGroupResult) (string, []string) {
	return composeStaffActionSummary(tr, card, results, true)
}

// editStaffActionMessage replaces the text of a message in the Staff Group and
// leaves out the reply markup, which removes the buttons. Telegram's "message is
// not modified" answer is not a failure.
func editStaffActionMessage(b *gotgbot.Bot, chatID, msgID int64, text string) error {
	ctx, cancel := context.WithTimeout(context.Background(), staffActionEditTimeout)
	defer cancel()
	_, _, err := b.EditMessageTextWithContext(ctx, &gotgbot.EditMessageTextOpts{
		ChatId:             chatID,
		MessageId:          msgID,
		Text:               text,
		ParseMode:          formatting.HTML,
		LinkPreviewOptions: &gotgbot.LinkPreviewOptions{IsDisabled: true},
	})
	if err != nil && !isMessageNotModified(err) {
		return err
	}
	return nil
}

const (
	// staffActionFinalAttempts is how many times the final edit is tried.
	staffActionFinalAttempts = 3
	// staffActionRetryAfterCap caps one wait for Telegram's retry_after.
	staffActionRetryAfterCap = 60 * time.Second
)

// staffRetryAfterWait is how long to wait before retrying after err: Telegram's
// retry_after times staffActionEditRetryUnit, capped. It is false when err is not
// a 429 that names a wait.
func staffRetryAfterWait(err error) (time.Duration, bool) {
	seconds, ok := ratelimit.RetryAfterSeconds(err)
	if !ok {
		return 0, false
	}
	return min(time.Duration(seconds)*staffActionEditRetryUnit, staffActionRetryAfterCap), true
}

// sleepStaffRetry waits d, or less when ctx ends first, and reports whether the
// full wait elapsed.
func sleepStaffRetry(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// staffActionSleep waits out one retry. It is a variable so tests can install a
// clock that checks each wait against the deadline of its context.
var staffActionSleep = sleepStaffRetry

// editStaffActionFinal puts the final text on the card, trying up to
// staffActionFinalAttempts times. A 429 waits out Telegram's retry_after and tries
// again; "message is not modified" counts as success (editStaffActionMessage);
// any other error, or running out of attempts or time, reports false.
func editStaffActionFinal(ctx context.Context, b *gotgbot.Bot, chatID, msgID int64, text string) bool {
	for attempt := 1; attempt <= staffActionFinalAttempts; attempt++ {
		err := editStaffActionMessage(b, chatID, msgID, text)
		if err == nil {
			return true
		}
		wait, limited := staffRetryAfterWait(err)
		if !limited || attempt == staffActionFinalAttempts || !staffActionSleep(ctx, wait) {
			log.Warnf("[StaffActions] could not edit the final summary in chat %d: %v", chatID, err)
			return false
		}
	}
	return false
}

// sendStaffSummaryPart posts one summary message into the Staff Group, waiting out
// a 429 the same way the final edit does. It reports whether the message was sent.
func sendStaffSummaryPart(ctx context.Context, b *gotgbot.Bot, chatID int64, text string) bool {
	for attempt := 1; attempt <= staffActionFinalAttempts; attempt++ {
		err := sendStaffNotice(b, chatID, text)
		if err == nil {
			return true
		}
		wait, limited := staffRetryAfterWait(err)
		if !limited || attempt == staffActionFinalAttempts || !staffActionSleep(ctx, wait) {
			return false
		}
	}
	return false
}

// staffActionDeliverPartTimeout bounds the delivery of one summary message. Before
// each of its attempts the message may wait once at the retry cap, and each attempt
// has its own edit timeout: 210 s with today's values.
const staffActionDeliverPartTimeout = staffActionFinalAttempts * (staffActionRetryAfterCap + staffActionEditTimeout)

// newStaffDeliverContext opens the budget of one summary message. It is never
// derived from the run's context, which a shutdown may already have cancelled.
func newStaffDeliverContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), staffActionDeliverPartTimeout)
}

// deliverStaffActionFinal puts a run's final summary in front of the issuer. The
// card is edited first, with retries (STAFF-12). When it cannot be edited (deleted,
// or any other error) the summary is posted as a new message in the Staff Group,
// so the result always arrives. Each continuation message follows in order
// (STAFF-08); one that cannot be sent is logged and the rest still go out. Every
// message has its own budget, so a long wait on one never starves the next.
func deliverStaffActionFinal(b *gotgbot.Bot, chatID, msgID int64, text string, continuation []string) {
	editCtx, cancelEdit := newStaffDeliverContext()
	edited := editStaffActionFinal(editCtx, b, chatID, msgID, text)
	cancelEdit()
	if !edited {
		sendCtx, cancelSend := newStaffDeliverContext()
		sent := sendStaffSummaryPart(sendCtx, b, chatID, text)
		cancelSend()
		if !sent {
			log.Errorf("[StaffActions] the final summary could not be delivered to chat %d", chatID)
		}
	}
	for i, part := range continuation {
		partCtx, cancelPart := newStaffDeliverContext()
		sent := sendStaffSummaryPart(partCtx, b, chatID, part)
		cancelPart()
		if !sent {
			log.Errorf("[StaffActions] continuation %d of the summary could not be delivered to chat %d", i+1, chatID)
		}
	}
}
