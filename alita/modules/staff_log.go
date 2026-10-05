package modules

import (
	"context"
	"html"
	"strings"
	"unicode/utf8"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/db/logchannels"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/i18n"
	"github.com/divkix/Alita_Robot/alita/utils/actionlog"
	"github.com/divkix/Alita_Robot/alita/utils/formatting"
)

// staffLogReasonMaxRunes caps the reason in a log post. It is applied before the
// reason is HTML-escaped, so an entity is never cut in half. The stored reason is
// already capped at 200 runes; this is a second guard for rows read back later.
const staffLogReasonMaxRunes = 300

// staffLogHashtag is the hashtag that opens a staff action's log post. Like every
// log hashtag it is not localized.
func staffLogHashtag(kind staffActionKind) string {
	switch kind {
	case staffKindBan:
		return "#STAFF_BAN"
	case staffKindMute:
		return "#STAFF_MUTE"
	case staffKindKick:
		return "#STAFF_KICK"
	case staffKindUnban:
		return "#STAFF_UNBAN"
	case staffKindUnmute:
		return "#STAFF_UNMUTE"
	}
	return "#STAFF"
}

// staffLogReason is the reason segment of a log post: "no reason given" for an
// empty one, otherwise the reason cut to staffLogReasonMaxRunes runes (plus an
// ellipsis) and only then HTML-escaped, so no raw tag from it reaches Telegram.
func staffLogReason(tr *i18n.Translator, reason string) string {
	if strings.TrimSpace(reason) == "" {
		text, _ := tr.GetString("staff_act_no_reason")
		return text
	}
	if utf8.RuneCountInString(reason) > staffLogReasonMaxRunes {
		reason = string([]rune(reason)[:staffLogReasonMaxRunes]) + "…"
	}
	return html.EscapeString(reason)
}

// composeStaffActionLog builds the body of a log post, without the group header:
// the hashtag, the issuer, the target, the duration (ban and mute only), the reason
// and the "via Staff Group" marker, joined with " · ". Labels are translated; the
// user text (names, reason) is spliced in afterwards and never goes through the
// translator. It never reads the card's Staff Group chat, so the Staff Group's ID
// cannot reach a linked group's log channel.
func composeStaffActionLog(tr *i18n.Translator, card *staffActionCard) string {
	adminLabel, _ := tr.GetString("staff_log_admin_label")
	userLabel, _ := tr.GetString("staff_log_user_label")
	via, _ := tr.GetString("staff_log_via_staff_group")

	segments := []string{
		staffLogHashtag(card.Kind),
		adminLabel + ": " + formatting.MentionHtml(card.Issuer, card.IssuerName),
		userLabel + ": " + staffTargetDisplay(tr, card),
	}
	if staffActionHasDuration(card.Kind) {
		segments = append(segments, staffDurationLabel(tr, card))
	}
	segments = append(segments, staffLogReason(tr, card.Reason), via)
	return strings.Join(segments, " · ")
}

// sendStaffLogPost posts one staff log message to the admin log channel of link's
// group: it looks up the destination, translates in the group's language and sends
// one paced message under the group's usual header. It does nothing when the run was
// cancelled, the group has no log channel or its admin category is off, and nothing
// is ever sent into the group's own chat. body builds the text after the header.
//
// Logging never changes the group's result (D-12): a failed or rate-limited post is
// logged at warn and nothing else happens.
func sendStaffLogPost(ctx context.Context, b *gotgbot.Bot, link models.StaffGroupLink, body func(tr *i18n.Translator) string) {
	if ctx.Err() != nil {
		return
	}
	chat := &gotgbot.Chat{Id: link.GroupChatID, Title: link.GroupTitle, Type: "supergroup"}
	channelID, header, ok := actionlog.Destination(chat, logchannels.CategoryAdmin)
	if !ok {
		return
	}
	text := header + body(staffChatTranslator(link.GroupChatID))
	err := staffPaced(ctx, func(ctx context.Context) error {
		callCtx, cancel := context.WithTimeout(ctx, staffActionCallTimeout)
		defer cancel()
		_, err := b.SendMessageWithContext(callCtx, channelID, text, &gotgbot.SendMessageOpts{
			ParseMode:          formatting.HTML,
			LinkPreviewOptions: &gotgbot.LinkPreviewOptions{IsDisabled: true},
		})
		return err
	})
	if err != nil {
		log.Warnf("[StaffActions] log post for group %d: %v", link.GroupChatID, err)
	}
}

// postStaffActionLog posts an applied staff action to the admin log channel of the
// group it was applied in, through sendStaffLogPost.
func postStaffActionLog(ctx context.Context, b *gotgbot.Bot, card *staffActionCard, link models.StaffGroupLink) {
	sendStaffLogPost(ctx, b, link, func(tr *i18n.Translator) string {
		return composeStaffActionLog(tr, card)
	})
}

// staffReverseKind is the action that reverses kind: ban and unban, mute and unmute.
// Kick has nothing to reverse and maps to itself.
func staffReverseKind(kind staffActionKind) staffActionKind {
	switch kind {
	case staffKindBan:
		return staffKindUnban
	case staffKindMute:
		return staffKindUnmute
	case staffKindUnban:
		return staffKindBan
	case staffKindUnmute:
		return staffKindMute
	}
	return kind
}

// staffActionNameToken stands in for the undone action's name while "undoes ..." is
// translated.
const staffActionNameToken = "<<staff-action-name>>"

// composeStaffUndoLog builds the body of an undo's log post, without the group
// header: "#STAFF_UNDO", the reverse action's name, the presser (card.Issuer, named
// by card.IssuerName), the target and "undoes <action> by <original issuer>", then
// the "via Staff Group" marker, joined with " · ". Labels are translated; the user
// text (names) is spliced in afterwards through tokens and never goes through the
// translator. It never reads the card's Staff Group chat, so the Staff Group's ID
// cannot reach a linked group's log channel.
func composeStaffUndoLog(tr *i18n.Translator, card *staffActionCard, originalIssuer string) string {
	adminLabel, _ := tr.GetString("staff_log_admin_label")
	userLabel, _ := tr.GetString("staff_log_user_label")
	via, _ := tr.GetString("staff_log_via_staff_group")
	undoes, _ := tr.GetString("staff_log_undoes", i18n.TranslationParams{
		"action": staffActionNameToken,
		"name":   staffUserToken,
	})
	undoes = strings.Replace(undoes, staffActionNameToken, staffActionName(tr, card.UndoKind), 1)
	undoes = strings.Replace(undoes, staffUserToken, html.EscapeString(staffPlainName(originalIssuer)), 1)

	segments := []string{
		"#STAFF_UNDO",
		staffActionName(tr, staffReverseKind(card.UndoKind)),
		adminLabel + ": " + formatting.MentionHtml(card.Issuer, card.IssuerName),
		userLabel + ": " + staffTargetDisplay(tr, card),
		undoes,
		via,
	}
	return strings.Join(segments, " · ")
}

// postStaffUndoLog posts a successful undo to the admin log channel of the group it
// was undone in, naming the presser and what was undone. It runs after the group's
// undo result is stored and, like the action's post, never changes it.
func postStaffUndoLog(ctx context.Context, b *gotgbot.Bot, card *staffActionCard, a *models.StaffAction, link models.StaffGroupLink) {
	sendStaffLogPost(ctx, b, link, func(tr *i18n.Translator) string {
		return composeStaffUndoLog(tr, card, a.IssuerName)
	})
}
