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

// postStaffActionLog posts an applied staff action to the admin log channel of the
// group it was applied in: one paced message under the group's usual header. It
// does nothing when the run was cancelled, the group has no log channel or its
// admin category is off, and nothing is ever sent into the group's own chat.
//
// Logging never changes the group's result (D-12): a failed or rate-limited post is
// logged at warn and nothing else happens.
func postStaffActionLog(ctx context.Context, b *gotgbot.Bot, card *staffActionCard, link models.StaffGroupLink) {
	if ctx.Err() != nil {
		return
	}
	chat := &gotgbot.Chat{Id: link.GroupChatID, Title: link.GroupTitle, Type: "supergroup"}
	channelID, header, ok := actionlog.Destination(chat, logchannels.CategoryAdmin)
	if !ok {
		return
	}
	text := header + composeStaffActionLog(staffChatTranslator(link.GroupChatID), card)
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
