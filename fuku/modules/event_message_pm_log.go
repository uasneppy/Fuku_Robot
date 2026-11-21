package modules

import (
	"context"
	"fmt"
	"html"
	"strconv"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db/logchannels"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/actionlog"
	"github.com/uasneppy/fuku_robot/fuku/utils/formatting"
)

// The log line of a private copy that was not delivered (Phase 22, PMSG-07, D-21). With the group's log option on,
// a private copy job whose final outcome is not delivered posts one line in the group's log channel under the admin
// category, so the admins can see which copies did not arrive. Nothing is posted for a delivered copy, a copy the
// pool shed or the automatic cap refused (those never reach the end of the job, D-18), a group with no log channel or
// with its admin category off. The line is the usual header of the destination, the untranslated #PM_COPY tag and one
// sentence in the group's language naming the member (mention and ID), the event's name and the reason class. It
// carries no message content, no reason, no Details token and names no Staff Group or other group (D-30, D-31): it is
// built from the event's own chat and the offer alone.

// pmCopyLogTag is the tag a log line of an undelivered private copy starts with, never translated (like
// #LOCKDOWN_BAN), so the admins can search the log channel for it.
const pmCopyLogTag = "#PM_COPY"

// eventPMEventName is the name of event as the Messages page shows it (the page's name key of the event, in the
// translator's language), or the event key escaped when no family of the page lists the event.
func eventPMEventName(tr *i18n.Translator, event string) string {
	for _, fam := range settingsMsgFamilies {
		for _, ev := range fam.Events {
			if ev.Key == event {
				return settingsText(tr, ev.NameKey)
			}
		}
	}
	return html.EscapeString(event)
}

// pmCopyLogText is the log line of an undelivered private copy without the destination's header: #PM_COPY on its own
// line, then eventmsg_pm_log in the translator's language with {user} the member's mention followed by their ID in a
// code tag, {event} the event's name (eventPMEventName) and {why} the reason class: eventmsg_pm_why_blocked for a
// blocked copy and eventmsg_pm_why_failed for every other outcome. It is pure, so a test renders exactly what is
// posted. The member's name appears only inside the mention, escaped.
func pmCopyLogText(tr *i18n.Translator, o eventPMOffer, outcome pmCopyOutcome) string {
	whyKey := "eventmsg_pm_why_failed"
	if outcome == pmCopyBlocked {
		whyKey = "eventmsg_pm_why_blocked"
	}
	why, _ := tr.GetString(whyKey)
	name := o.TargetName
	if name == "" {
		name = strconv.FormatInt(o.UserID, 10)
	}
	user := fmt.Sprintf("%s (<code>%d</code>)", formatting.MentionHtml(o.UserID, name), o.UserID)
	sentence, _ := tr.GetString("eventmsg_pm_log", i18n.TranslationParams{
		"user": user, "event": eventPMEventName(tr, o.Event), "why": why,
	})
	return pmCopyLogTag + "\n" + sentence
}

// pmCopyPostLog posts the log line of a private copy that was not delivered (D-21). runEventPM calls it only for a
// final outcome other than delivered, with the group's log option on. It resolves the group's log channel through
// actionlog.Destination under the admin category (no channel, or the category off, posts nothing; that read is the
// job's only database read, so once the database is closed at shutdown it fails and nothing is posted) and sends the
// header and pmCopyLogText as one HTML message, link previews off, through pmCopyPaced, the private copies' own pacer.
// It is best effort: a failed post is one Warn line with the chat ID and the Telegram code, and it never changes the
// punishment, the group reply or the copy. It builds no reply parameters.
func pmCopyPostLog(ctx context.Context, o eventPMOffer, outcome pmCopyOutcome) {
	if o.Tr == nil || o.Bot == nil || ctx.Err() != nil {
		log.Debugf("[EventMessage] no log line for user %d in chat %d: nothing to post with", o.UserID, o.Chat.Id)
		return
	}
	chat := o.Chat
	channelID, header, ok := actionlog.Destination(&chat, logchannels.CategoryAdmin)
	if !ok {
		log.Debugf("[EventMessage] no log line for user %d in chat %d: no log channel takes the admin category", o.UserID, o.Chat.Id)
		return
	}
	text := header + pmCopyLogText(o.Tr, o, outcome)
	err := pmCopyPaced(ctx, func(callCtx context.Context) error {
		_, err := o.Bot.SendMessageWithContext(callCtx, channelID, text, &gotgbot.SendMessageOpts{
			ParseMode:          formatting.HTML,
			LinkPreviewOptions: &gotgbot.LinkPreviewOptions{IsDisabled: true},
		})
		return err
	})
	if err != nil {
		log.Warnf("[EventMessage] the %s log line of chat %d could not be posted (code %d)", pmCopyLogTag, o.Chat.Id, pmCopyErrCode(err))
	}
}
