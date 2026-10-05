package modules

import (
	"context"
	"errors"
	"html"
	"strconv"
	"strings"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/i18n"
	"github.com/divkix/Alita_Robot/alita/utils/formatting"
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

// renderStaffActionSummary is the card text once the run has started: the header,
// a blank line, then one line per linked group in the order given.
func renderStaffActionSummary(tr *i18n.Translator, card *staffActionCard, results []staffGroupResult) string {
	lines := make([]string, len(results))
	for i, res := range results {
		lines[i] = staffResultLine(tr, res)
	}
	return staffActionHeader(tr, card) + "\n\n" + strings.Join(lines, "\n")
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

// deliverStaffActionSummary puts the final text on the card. When the card cannot
// be edited (deleted, or any other error) the summary is posted as a new message
// in the Staff Group, so the issuer always gets the result.
func deliverStaffActionSummary(b *gotgbot.Bot, chatID, msgID int64, text string) {
	err := editStaffActionMessage(b, chatID, msgID, text)
	if err == nil {
		return
	}
	var tgErr *gotgbot.TelegramError
	if errors.As(err, &tgErr) {
		log.Warnf("[StaffActions] could not edit summary in chat %d: %s", chatID, tgErr.Description)
	} else {
		log.Warnf("[StaffActions] could not edit summary in chat %d: %v", chatID, err)
	}
	_ = sendStaffNotice(b, chatID, text)
}
