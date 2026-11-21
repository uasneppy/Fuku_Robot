package modules

import (
	"context"
	"errors"
	"html"
	"strings"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/db/eventmsg"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/formatting"
	"github.com/uasneppy/fuku_robot/fuku/utils/keyboard"
	"github.com/uasneppy/fuku_robot/fuku/utils/media"
)

// The event message engine (Phase 21). A group can store its own message for an event (a manual
// punishment such as ban); the success reply of that event is then the stored message instead of
// the default text. The engine runs in the reply step of the moderation pipeline, after execute
// applied the punishment (D-03): a message never delays, undoes or skips a punishment. With no row,
// an empty row or a read that fails, it sends the default reply through replySuccess, which is
// literally the call the send site made before Phase 21 (TestLegacyModerationRequests pins it).
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
// before Phase 21, byte for byte, in the translator's language (the group's).
type eventDefaultReply func(tr *i18n.Translator, in eventDefaultInput) (string, *gotgbot.SendMessageOpts)

// eventReply is one event's reply as a send site hands it to replyEvent: the event key, the punished
// member, the event's values and the default reply already built (text and options). Note is a line
// the site appends to its default (the /unmute lockdown note); a custom text is followed by it too.
// PMNote is the private wording of Note, followed after a private copy's text, custom or default.
type eventReply struct {
	Event   string
	Target  gotgbot.User
	Values  eventValues
	Default string
	Opts    *gotgbot.SendMessageOpts
	Note    string
	PMNote  string
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

// eventAdminName is the escaped full name of the acting admin, or the translated "an admin"
// (eventmsg_admin_anon, the translator's language: the group's) for an anonymous admin or an actor
// the reply cannot name. It never names the Staff Group (D-12).
func eventAdminName(tr *i18n.Translator, actor eventActor) string {
	if actor.Anonymous || actor.User == nil {
		word, _ := tr.GetString("eventmsg_admin_anon")
		return html.EscapeString(word)
	}
	return html.EscapeString(formatting.GetFullName(actor.User.FirstName, actor.User.LastName))
}

// eventAnonActorKey is the ctx.Data key that marks the command's issuer as an anonymous admin. The
// anonymous admin's proof tap re-enters the command with the tapper as the sender, so the mark is
// carried from the original update to the reply in the update's data.
const eventAnonActorKey = "event_message_anonymous_actor"

// markAnonymousActor marks the update's issuer as an anonymous admin, so the event reply reads
// {admin} as "an admin" although the re-entered command's sender is the tapper. verifyAnonymousAdmin
// calls it right before the re-entry.
func markAnonymousActor(ctx *ext.Context) {
	if ctx == nil {
		return
	}
	if ctx.Data == nil {
		ctx.Data = make(map[string]any)
	}
	ctx.Data[eventAnonActorKey] = true
}

// actorIsAnonymous reports whether the command's issuer is an anonymous admin: the update carries
// the proof tap's mark, or the sender is the group itself (an anonymous admin in a group whose
// AnonAdmin mode lets the command run at once, where the moderation context's user is the Group
// Anonymous Bot).
func actorIsAnonymous(ctx *ext.Context) bool {
	if ctx == nil {
		return false
	}
	if marked, _ := ctx.Data[eventAnonActorKey].(bool); marked {
		return true
	}
	return ctx.EffectiveSender != nil && ctx.EffectiveSender.IsAnonymousAdmin()
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

// composeEventMessage builds the custom message of one reply from the stored row. With a custom text
// the text is filled (renderEventText, which also adds the {rules} button) and followed by r.Note on
// a new line; with none the text is r.Default, the default reply's own text with its reason line (and
// its note, which the site already appended). The media is the row's file and type (text when there
// is no file). The keyboard is the bot's own rows (the Unban row of a ban) followed by the custom and
// rules buttons; with neither there is no keyboard. The caller sets the reply target and the topic.
func composeEventMessage(b *gotgbot.Bot, chat *gotgbot.Chat, tr *i18n.Translator, r eventReply, row eventmsg.Message, actor eventActor) media.GreetingMessage {
	text := r.Default
	buttons := row.Buttons
	if row.Text != "" {
		target := r.Target
		text, buttons = renderEventText(b, chat, &target, row.Text, row.Buttons, eventPlaceholderValues(tr, chat.Id, actor, r.Values))
		if r.Note != "" {
			text += "\n" + r.Note
		}
	}
	msg := media.GreetingMessage{Text: text, FileID: row.FileID, MsgType: row.MediaType}
	if row.FileID == "" || row.MediaType == 0 {
		msg.FileID, msg.MsgType = "", db.TEXT
	}
	// A fresh slice: the default's options are reused by the refusal fallback and must not change.
	rows := append(append([][]gotgbot.InlineKeyboardButton(nil), eventBotRows(r.Opts)...), keyboard.BuildKeyboard(buttons)...)
	if len(rows) > 0 {
		msg.Keyboard = &gotgbot.InlineKeyboardMarkup{InlineKeyboard: rows}
	}
	return msg
}

// eventMessageRefused reports whether Telegram definitely refused a custom message: a 4xx other than
// 429, or the media sender's ErrNoPermission (it turns a permission refusal into that error). The
// default reply is then sent once (D-13). A 429, a 5xx or a transport error is not a refusal.
func eventMessageRefused(err error) bool {
	return isTelegramClientError(err) || errors.Is(err, media.ErrNoPermission)
}

// eventTargetOf is the punished member as a user, from the getChat answer a send site already read.
func eventTargetOf(info *gotgbot.ChatFullInfo) gotgbot.User {
	if info == nil {
		return gotgbot.User{}
	}
	return gotgbot.User{Id: info.Id, FirstName: info.FirstName, LastName: info.LastName, Username: info.Username}
}

// replyEvent sends the success reply of an event and then offers its private copy. The punishment
// has already been applied (D-03). It reads the group's stored messages once (bounded by
// eventMessageReadTimeout): a read error is one Warn, the default and no private copy (the switch is
// unknown, and unknown never sends). Otherwise the group step runs exactly as before the private copy
// existed (replyEventGroup), and then, whatever it did, the private copy is offered (offerReplyPM),
// which never blocks. The error is the group step's.
func (c *moderationCtx) replyEvent(r eventReply) error {
	readCtx, cancel := context.WithTimeout(context.Background(), eventMessageReadTimeout)
	msgs, err := eventMessagesLookup(readCtx, c.Chat.Id)
	cancel()
	if err != nil {
		log.Warnf("[EventMessage] the messages of chat %d could not be read for %s, the default was sent", c.Chat.Id, r.Event)
		return c.replySuccess(r.Default, r.Opts)
	}
	post, err := c.replyEventGroup(r, msgs)
	c.offerReplyPM(r, msgs, post)
	return err
}

// eventGroupPost is what the group step of a reply posted: the message (nil when nothing was posted) and the
// keyboard rows it was sent with (the bot's own rows of the default, or a custom message's composed rows), which
// the Details button of the private copy keeps (pmCopyAddDetails).
type eventGroupPost struct {
	Msg  *gotgbot.Message
	Rows [][]gotgbot.InlineKeyboardButton
}

// replyEventGroup is the group step of an event reply and returns what it posted (no message when
// nothing was posted) with the keyboard rows the message was sent with. No row, or a row with no content and Silent off, is the default through
// sendSuccess (today's call); Silent posts and schedules nothing; otherwise the custom message is
// sent as a reply to the command and queued under bot:punish_manual, the default's kind (D-15). A
// custom message Telegram definitely refuses (eventMessageRefused) is replaced by the default, sent
// once; any other send error is returned, as the default's send error always was.
func (c *moderationCtx) replyEventGroup(r eventReply, msgs *eventmsg.Messages) (eventGroupPost, error) {
	row, ok := msgs.Lookup(r.Event, eventmsg.DestGroup)
	if ok && row.Silent {
		log.Debugf("[EventMessage] the %s message of chat %d is silent, nothing was posted", r.Event, c.Chat.Id)
		return eventGroupPost{}, nil
	}
	if !ok || !row.HasContent() {
		return c.sendDefaultPost(r)
	}

	actor := eventActor{User: c.User, Anonymous: actorIsAnonymous(c.Ctx)}
	msg := composeEventMessage(c.Bot, c.Chat, c.Tr, r, row, actor)
	msg.ReplyMsgID = c.Msg.MessageId
	if c.Msg.IsTopicMessage {
		msg.ThreadID = c.Msg.MessageThreadId
	}
	sent, err := media.SendGreetingMessage(c.Bot, c.Chat.Id, msg)
	if err != nil {
		if eventMessageRefused(err) {
			log.Warnf("[EventMessage] the custom %s message of chat %d was refused, the default was sent", r.Event, c.Chat.Id)
			return c.sendDefaultPost(r)
		}
		return eventGroupPost{}, err
	}
	scheduleBotMessage(sent, deletion.KindBotManual)
	post := eventGroupPost{Msg: sent}
	if msg.Keyboard != nil {
		post.Rows = msg.Keyboard.InlineKeyboard
	}
	return post, nil
}

// sendDefaultPost sends the default reply through sendSuccess (today's call, byte for byte) and returns it with
// the bot's own rows it carries.
func (c *moderationCtx) sendDefaultPost(r eventReply) (eventGroupPost, error) {
	sent, err := c.sendSuccess(r.Default, r.Opts)
	if err != nil {
		return eventGroupPost{}, err
	}
	return eventGroupPost{Msg: sent, Rows: eventBotRows(r.Opts)}, nil
}

// offerReplyPM offers the private copy of an event reply after the group step (D-11). It offers
// nothing for an event without a spec or a private copy whose switch is off (Resolve: no pm row, or a
// pm row whose switch was never turned on, reads as Silent) and for a target that can never receive one
// (pmCopySkipTarget, checked before the render). The copy is composed here, in the group's language and
// with the group step's actor, together with the default private text a refused custom copy falls back
// to, and sent by a job (offerEventPM), so the handler returns at once. The offer also carries the group's
// private copy options, the group message the group step posted (none when it was Silent or failed) with its
// rows, and the Details button's label in the group's language, for the Details button of a copy that is not
// delivered.
func (c *moderationCtx) offerReplyPM(r eventReply, msgs *eventmsg.Messages, post eventGroupPost) {
	spec, ok := eventPMSpecFor(r.Event)
	if !ok {
		return
	}
	row := msgs.Resolve(r.Event, eventmsg.DestPM)
	if row.Silent {
		return
	}
	if pmCopySkipTarget(c.Bot, r.Target.Id) {
		return
	}
	actor := eventActor{User: c.User, Anonymous: actorIsAnonymous(c.Ctx)}
	offer := eventPMOffer{
		Bot:     c.Bot,
		Chat:    *c.Chat,
		Event:   r.Event,
		UserID:  r.Target.Id,
		Message: composeEventPM(c.Bot, c.Chat, c.Tr, spec, r, row, actor),
		Custom:  row.HasContent(),
		Options: msgs.Options,
	}
	if post.Msg != nil {
		offer.GroupMsgID = post.Msg.MessageId
		offer.GroupRows = post.Rows
	}
	if offer.Options.PMDetails {
		offer.DetailsLabel, _ = c.Tr.GetString("eventmsg_btn_details")
	}
	if offer.Custom {
		offer.Fallback = eventPMDefaultText(c.Tr, spec, c.Chat, r.Values, r.PMNote)
	}
	offerEventPM(offer)
}
