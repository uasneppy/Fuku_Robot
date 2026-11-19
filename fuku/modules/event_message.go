package modules

import (
	"context"
	"html"
	"strings"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/db/eventmsg"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/formatting"
	"github.com/uasneppy/fuku_robot/fuku/utils/media"
)

// The event message engine (Phase 20). A group can store its own message for an event (a manual
// punishment such as ban); the success reply of that event is then the stored message instead of
// the default text. The engine runs in the reply step of the moderation pipeline, after execute
// applied the punishment (D-03): a message never delays, undoes or skips a punishment. With no row,
// an empty row or a read that fails, it sends the default reply through replySuccess, which is
// literally the call the send site made before Phase 20 (TestLegacyModerationRequests pins it).
// No stored text, caption, button or file ID is ever logged: a line names the chat ID and the event.

// eventMessagesLookup reads a chat's stored event messages. It is a seam so a test can make the
// read fail; the default is the cached read, whose key is in skipLocal.
var eventMessagesLookup = eventmsg.Get

// eventMessageReadTimeout bounds the one read of a reply. The punishment has already happened, so a
// slow read only delays the reply, and a read that runs out sends the default.
var eventMessageReadTimeout = 2 * time.Second

// eventValues are the event's own values, filled into the event placeholders of a custom text. A
// field with no value for the event (a permanent ban has no duration) renders as nothing (D-09).
type eventValues struct {
	Reason   string
	Duration string
	Until    time.Time
}

// eventActor is the admin who issued the command. Anonymous is set for an anonymous admin, whose
// name {admin} never shows (D-12).
type eventActor struct {
	User      *gotgbot.User
	Anonymous bool
}

// eventDefaultInput is what a default reply builder needs: the punished member, the typed reason
// and the duration text of a timed punishment.
type eventDefaultInput struct {
	Target  gotgbot.User
	Reason  string
	TimeVal string
}

// eventDefaultReply builds an event's default reply: the text and the options the send site sent
// before Phase 20, byte for byte, in the translator's language (the group's).
type eventDefaultReply func(tr *i18n.Translator, in eventDefaultInput) (string, *gotgbot.SendMessageOpts)

// eventReply is one event's reply as a send site hands it to replyEvent: the event key, the punished
// member, the event's values and the default reply already built (text and options). Note is a line
// the site appends to its default (the /unmute lockdown note); a custom text is followed by it too.
type eventReply struct {
	Event   string
	Target  gotgbot.User
	Values  eventValues
	Default string
	Opts    *gotgbot.SendMessageOpts
	Note    string
}

// eventTokenNames are the event placeholders. {count}, {action}, {limit} and {window} belong to the
// event (empty for the manual events), so {count} in an event message is never the member count.
var eventTokenNames = []string{"reason", "admin", "duration", "until", "action", "count", "limit", "window"}

// eventTokenSentinel is the marker an event placeholder is turned into before the Welcome
// placeholders are filled. The stored text is HTML, where a literal '<' is always escaped, so the
// marker can never come from the owner's text, and a value filled in later is never filled again.
func eventTokenSentinel(name string) string {
	return "<<fuku-event-" + name + ">>"
}

// eventPlaceholderValues are the escaped values of the event placeholders for one reply. A
// placeholder with no value is the empty string, never raw. {until} is shown in the chat's zone.
func eventPlaceholderValues(tr *i18n.Translator, chatID int64, actor eventActor, v eventValues) map[string]string {
	values := make(map[string]string, len(eventTokenNames))
	for _, name := range eventTokenNames {
		values[name] = ""
	}
	values["reason"] = html.EscapeString(v.Reason)
	values["admin"] = eventAdminName(tr, actor)
	values["duration"] = html.EscapeString(v.Duration)
	if !v.Until.IsZero() {
		values["until"] = formatChatTime(v.Until, chatClockOf(chatID), "02.01.2006 15:04")
	}
	return values
}

// eventAdminName is the escaped full name of the acting admin.
func eventAdminName(_ *i18n.Translator, actor eventActor) string {
	if actor.User == nil {
		return ""
	}
	return html.EscapeString(formatting.GetFullName(actor.User.FirstName, actor.User.LastName))
}

// renderEventText fills a stored text for one reply. Each event placeholder is first turned into
// its sentinel, then formatting.FormattingReplacer fills the Welcome placeholders for the target and
// the group (and adds the {rules} button), then every sentinel is replaced by its escaped value in
// one pass. So a reason that contains {first} and a name that contains {reason} stay as typed, and
// an unknown {word} of the owner's text stays as typed.
func renderEventText(b *gotgbot.Bot, chat *gotgbot.Chat, target *gotgbot.User, stored string, buttons []db.Button, values map[string]string) (string, []db.Button) {
	marks := make([]string, 0, 2*len(eventTokenNames))
	fills := make([]string, 0, 2*len(eventTokenNames))
	for _, name := range eventTokenNames {
		marks = append(marks, "{"+name+"}", eventTokenSentinel(name))
		fills = append(fills, eventTokenSentinel(name), values[name])
	}
	marked := strings.NewReplacer(marks...).Replace(stored)
	filled, btns := formatting.FormattingReplacer(b, chat, target, marked, buttons)
	return strings.NewReplacer(fills...).Replace(filled), btns
}

// eventBotRows are the bot's own keyboard rows of a default reply (the Unban row of a ban), which a
// custom message keeps first. Options with no inline keyboard have none.
func eventBotRows(opts *gotgbot.SendMessageOpts) [][]gotgbot.InlineKeyboardButton {
	if opts == nil {
		return nil
	}
	switch markup := opts.ReplyMarkup.(type) {
	case gotgbot.InlineKeyboardMarkup:
		return markup.InlineKeyboard
	case *gotgbot.InlineKeyboardMarkup:
		if markup != nil {
			return markup.InlineKeyboard
		}
	}
	return nil
}

// composeEventMessage builds the custom message of one reply from the stored row: the filled custom
// text and the bot's own rows. The caller sets the reply target and the topic.
func composeEventMessage(b *gotgbot.Bot, chat *gotgbot.Chat, tr *i18n.Translator, r eventReply, row eventmsg.Message, actor eventActor) media.GreetingMessage {
	text := r.Default
	if row.Text != "" {
		target := r.Target
		text, _ = renderEventText(b, chat, &target, row.Text, nil, eventPlaceholderValues(tr, chat.Id, actor, r.Values))
	}
	msg := media.GreetingMessage{Text: text, MsgType: db.TEXT}
	if rows := eventBotRows(r.Opts); len(rows) > 0 {
		msg.Keyboard = &gotgbot.InlineKeyboardMarkup{InlineKeyboard: rows}
	}
	return msg
}

// eventTargetOf is the punished member as a user, from the getChat answer a send site already read.
func eventTargetOf(info *gotgbot.ChatFullInfo) gotgbot.User {
	if info == nil {
		return gotgbot.User{}
	}
	return gotgbot.User{Id: info.Id, FirstName: info.FirstName, LastName: info.LastName, Username: info.Username}
}

// replyEvent sends the success reply of an event. The punishment has already been applied (D-03).
// It reads the group's stored messages once (bounded by eventMessageReadTimeout): a read error is
// one Warn and the default; no row, or a row with no content and Silent off, is the default through
// replySuccess (today's call); Silent posts and schedules nothing; otherwise the custom message is
// sent as a reply to the command and queued under bot:punish_manual, the default's kind (D-15).
func (c *moderationCtx) replyEvent(r eventReply) error {
	readCtx, cancel := context.WithTimeout(context.Background(), eventMessageReadTimeout)
	msgs, err := eventMessagesLookup(readCtx, c.Chat.Id)
	cancel()
	if err != nil {
		log.Warnf("[EventMessage] the messages of chat %d could not be read for %s, the default was sent", c.Chat.Id, r.Event)
		return c.replySuccess(r.Default, r.Opts)
	}
	row, ok := msgs.Lookup(r.Event, eventmsg.DestGroup)
	if ok && row.Silent {
		log.Debugf("[EventMessage] the %s message of chat %d is silent, nothing was posted", r.Event, c.Chat.Id)
		return nil
	}
	if !ok || !row.HasContent() {
		return c.replySuccess(r.Default, r.Opts)
	}

	msg := composeEventMessage(c.Bot, c.Chat, c.Tr, r, row, eventActor{User: c.User})
	msg.ReplyMsgID = c.Msg.MessageId
	if c.Msg.IsTopicMessage {
		msg.ThreadID = c.Msg.MessageThreadId
	}
	sent, err := media.SendGreetingMessage(c.Bot, c.Chat.Id, msg)
	if err != nil {
		return err
	}
	scheduleBotMessage(sent, deletion.KindBotManual)
	return nil
}
