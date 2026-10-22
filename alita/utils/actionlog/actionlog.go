package actionlog

import (
	"fmt"
	"html"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/db/logchannels"
	"github.com/divkix/Alita_Robot/alita/utils/formatting"
	"github.com/divkix/Alita_Robot/alita/utils/helpers"
)

// Destination resolves where a log post for chat in category goes. It returns the
// log channel's ID and the header every post starts with: the bold chat title and
// the chat ID in a code tag, followed by a newline. ok is false when there is no
// post to make: chat is nil or a channel, the chat has no log channel, or the
// category is off. Log and the staff log posts share it, so the destination checks
// and the header stay in one place.
func Destination(chat *gotgbot.Chat, category string) (channelID int64, header string, ok bool) {
	if chat == nil || chat.Type == "channel" {
		return 0, "", false
	}
	settings := logchannels.Get(chat.Id)
	if settings == nil || !logchannels.CategoryEnabled(settings, category) {
		return 0, "", false
	}
	title := html.EscapeString(chat.Title)
	return settings.LogChannelID, fmt.Sprintf("<b>%s</b> (<code>%d</code>)\n", title, chat.Id), true
}

// Failures are swallowed so logging never blocks a user-facing command.
func Log(b *gotgbot.Bot, chat *gotgbot.Chat, category, htmlText string) {
	if b == nil || chat == nil || htmlText == "" {
		return
	}
	channelID, header, ok := Destination(chat, category)
	if !ok {
		return
	}
	if _, err := helpers.SendMessageWithErrorHandling(b, channelID, header+htmlText, &gotgbot.SendMessageOpts{
		ParseMode: formatting.HTML,
	}); err != nil {
		log.Debugf("[ActionLog] failed to send %s log for chat %d: %v", category, chat.Id, err)
	}
}

func Admin(b *gotgbot.Bot, chat *gotgbot.Chat, actor *gotgbot.User, action string, targetID int64, reason string) {
	if actor == nil {
		return
	}
	text := fmt.Sprintf(
		"#%s\nAdmin: %s\nUser: <code>%d</code>",
		html.EscapeString(action),
		formatting.MentionHtml(actor.Id, actor.FirstName),
		targetID,
	)
	if reason != "" {
		text += "\nReason: " + html.EscapeString(reason)
	}
	Log(b, chat, logchannels.CategoryAdmin, text)
}

func User(b *gotgbot.Bot, chat *gotgbot.Chat, actor *gotgbot.User, action string) {
	if actor == nil {
		return
	}
	text := fmt.Sprintf(
		"#%s\nUser: %s",
		html.EscapeString(action),
		formatting.MentionHtml(actor.Id, actor.FirstName),
	)
	Log(b, chat, logchannels.CategoryUser, text)
}

func Reports(b *gotgbot.Bot, chat *gotgbot.Chat, reporter *gotgbot.User, targetID int64) {
	if reporter == nil {
		return
	}
	text := fmt.Sprintf(
		"#REPORT\nReporter: %s\nTarget: <code>%d</code>",
		formatting.MentionHtml(reporter.Id, reporter.FirstName),
		targetID,
	)
	Log(b, chat, logchannels.CategoryReports, text)
}

func Settings(b *gotgbot.Bot, chat *gotgbot.Chat, actor *gotgbot.User, summary string) {
	actorBit := "unknown"
	if actor != nil {
		actorBit = formatting.MentionHtml(actor.Id, actor.FirstName)
	}
	Log(b, chat, logchannels.CategorySettings, fmt.Sprintf("#SETTINGS\nAdmin: %s\n%s", actorBit, html.EscapeString(summary)))
}
