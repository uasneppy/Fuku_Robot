package modules

import (
	"context"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/eventmsg"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/error_handling"
	"github.com/uasneppy/fuku_robot/fuku/utils/keyboard"
	"github.com/uasneppy/fuku_robot/fuku/utils/media"
	"github.com/uasneppy/fuku_robot/fuku/utils/ratelimit"
)

// The private copy engine (Phase 21). A group can also tell the punished person in private: each
// event with a registered spec has a private copy (the event's pm row), off until the owner switches
// it on. The copy is offered by replyEvent after the punishment and after the group step (D-03,
// D-11), whatever the group step did, and is sent from a bounded job through its own fleet-wide
// pacer (D-13, D-14), so a ban, a mute or a group reply never waits for it and is never changed by
// it. The jobs have no shutdown drain and no flag: a restart cuts off the copies in flight, which are
// best effort. No log line of these paths carries a stored or typed text, a caption, a file ID, a
// button, a reason or a name: the chat ID, the user ID, the event and the outcome only (D-31).

// eventPMSpec is how an event's private copy is built. DefaultKey is the locale key of the event's
// default private text, used when the pm row stores no text. Automatic marks an event the bot raises
// by itself (the per-group cap of a later phase applies to it); the manual events are not automatic.
type eventPMSpec struct {
	DefaultKey string
	Automatic  bool
}

// eventPMSpecs is every event that has a private copy, keyed by event. It is filled at init by
// registerEventPM and only read afterwards.
var eventPMSpecs = map[string]eventPMSpec{}

// registerEventPM gives an event a private copy. It panics at init on an event eventmsg does not
// know, an empty default key or a second registration of one event, so a wrong table never ships.
func registerEventPM(event string, spec eventPMSpec) {
	if !eventmsg.ValidEvent(event) {
		panic(fmt.Sprintf("registerEventPM: unknown event %q", event))
	}
	if spec.DefaultKey == "" {
		panic(fmt.Sprintf("registerEventPM: event %q has no default key", event))
	}
	if _, dup := eventPMSpecs[event]; dup {
		panic(fmt.Sprintf("registerEventPM: event %q registered twice", event))
	}
	eventPMSpecs[event] = spec
}

// eventPMSpecFor returns the spec of an event's private copy and whether the event has one.
func eventPMSpecFor(event string) (eventPMSpec, bool) {
	spec, ok := eventPMSpecs[event]
	return spec, ok
}

func init() {
	// The seven manual events of Phase 20 (D-06): /dban, /dkick and /dmute share their parent's
	// event, and the silent variants never reach replyEvent, so they send no copy.
	registerEventPM(eventmsg.EventBan, eventPMSpec{DefaultKey: "eventmsg_pm_ban", Automatic: false})
}

// pmCopyPacerOptions are the options of the private copies' pacer: its own Redis keys, so a burst of
// copies never spends the slots of a ban, a group reply or any other pacer, and at most five sends a
// second of Telegram's budget. A slot more than MaxWait away fails at once with no request. It is a
// variable so a test can build a fast pacer from it.
var pmCopyPacerOptions = ratelimit.TelegramPacerOptions{
	NextKey:  "fuku:pmcopy:pace:next",
	BlockKey: "fuku:pmcopy:pace:block",
	Interval: 200 * time.Millisecond,
	// MaxRetries 1 is the smallest the pacer accepts (a value of 0 or less becomes 3).
	MaxRetries: 1,
	MaxWait:    15 * time.Second,
}

// pmCopyPacer paces every Telegram call of a private copy job across the fleet. Nothing else uses its
// keys. It is a variable so a test can install a fast one.
var pmCopyPacer = ratelimit.NewTelegramPacer(pmCopyPacerOptions)

// pmCopyCallTimeout bounds one Telegram request of a private copy job.
const pmCopyCallTimeout = 10 * time.Second

// pmCopyPaced runs one Telegram call of a private copy job under pmCopyPacer, bounded by
// pmCopyCallTimeout; on a 429 it waits out Telegram's retry_after within the pacer's limits.
func pmCopyPaced(ctx context.Context, call func(context.Context) error) error {
	return pmCopyPacer.Do(ctx, func(callCtx context.Context) error {
		bounded, cancel := context.WithTimeout(callCtx, pmCopyCallTimeout)
		defer cancel()
		return call(bounded)
	})
}

// pmCopyJobs counts the private copy jobs in flight on this replica. It bounds them (a full pool sheds
// the copy) and lets a test wait for them; it is not a shutdown drain (D-13).
var pmCopyJobs raidPostTracker

// pmCopyMaxInFlight is how many private copy jobs one replica runs at once.
const pmCopyMaxInFlight = 16

// pmCopyJobTimeout bounds one private copy job, its waits for pacer slots included. It is a variable
// only so a test can shorten it; production code never reassigns it.
var pmCopyJobTimeout = 60 * time.Second

// eventPMOffer is one private copy ready to send: the bot, a copy of the group (only its ID is read by
// the job), the event, the punished member's user ID (the private chat the copy goes to) and the
// composed message, which carries no reply target and no topic.
type eventPMOffer struct {
	Bot     *gotgbot.Bot
	Chat    gotgbot.Chat
	Event   string
	UserID  int64
	Message media.GreetingMessage
}

// offerEventPM hands one private copy to a job and returns at once. The punishment and the group step
// have already happened (D-03). A pool at pmCopyMaxInFlight sheds the copy with one Debug line. The job
// recovers from a panic, runs under pmCopyJobTimeout from a background context (never the update's)
// and is not drained at shutdown.
func offerEventPM(o eventPMOffer) {
	if !pmCopyJobs.startBelow(pmCopyMaxInFlight) {
		log.Debugf("[EventMessage] the private %s copy of user %d in chat %d was shed: too many copies in flight", o.Event, o.UserID, o.Chat.Id)
		return
	}
	go func() {
		defer pmCopyJobs.finish()
		defer error_handling.RecoverFromPanic("offerEventPM", "EventMessage")
		ctx, cancel := context.WithTimeout(context.Background(), pmCopyJobTimeout)
		defer cancel()
		runEventPM(ctx, o)
	}()
}

// runEventPM sends one private copy to the member's private chat through pmCopyPaced and logs the
// outcome at Debug with the IDs, the event and a Telegram error by its code only. It builds no reply
// parameters: the copy is a new message in a private chat, never a reply to the command.
func runEventPM(ctx context.Context, o eventPMOffer) {
	err := pmCopyPaced(ctx, func(context.Context) error {
		_, sendErr := media.SendGreetingMessage(o.Bot, o.UserID, o.Message)
		return sendErr
	})
	if err != nil {
		log.Debugf("[EventMessage] the private %s copy of user %d in chat %d was not delivered: %s", o.Event, o.UserID, o.Chat.Id, memberReqErrText(err))
		return
	}
	log.Debugf("[EventMessage] the private %s copy of user %d in chat %d was delivered", o.Event, o.UserID, o.Chat.Id)
}

// composeEventPM builds the private copy of one reply from the pm row (D-12). With a stored text the
// text is filled by the group renderer (renderEventText: the Welcome placeholders for the member and
// the group, the event placeholders escaped, {admin} the translated "an admin" for an anonymous admin,
// and the {rules} button), followed by r.PMNote on a new line; with none it is the event's default
// private text (eventPMDefaultText). The keyboard is the stored buttons and the {rules} button only,
// never the bot's own Unban or Unmute row, which are group callbacks; the media is the row's file.
func composeEventPM(b *gotgbot.Bot, chat *gotgbot.Chat, tr *i18n.Translator, spec eventPMSpec, r eventReply, row eventmsg.Message, actor eventActor) media.GreetingMessage {
	buttons := row.Buttons
	var text string
	if row.Text == "" {
		text = eventPMDefaultText(tr, spec, chat, r.Values, r.PMNote)
	} else {
		target := r.Target
		text, buttons = renderEventText(b, chat, &target, row.Text, row.Buttons, eventPlaceholderValues(tr, chat.Id, actor, r.Values))
		if r.PMNote != "" {
			text += "\n" + r.PMNote
		}
	}
	msg := media.GreetingMessage{Text: text, FileID: row.FileID, MsgType: row.MediaType}
	if row.FileID == "" || row.MediaType == 0 {
		msg.FileID, msg.MsgType = "", db.TEXT
	}
	if rows := keyboard.BuildKeyboard(buttons); len(rows) > 0 {
		msg.Keyboard = &gotgbot.InlineKeyboardMarkup{InlineKeyboard: rows}
	}
	return msg
}

// eventPMDefaultText is an event's default private text in the translator's language (the group's):
// the spec's DefaultKey with {group} the group's title escaped and cut like a Staff Group title, and
// {until} the end in the group's zone for a timed event, then, each on a new line, the reason line
// (eventmsg_pm_reason with the reason escaped) when a reason was given and the note when set. It
// never names the acting admin.
func eventPMDefaultText(tr *i18n.Translator, spec eventPMSpec, chat *gotgbot.Chat, values eventValues, note string) string {
	params := i18n.TranslationParams{"group": staffDisplayTitle(chat.Title), "until": ""}
	if !values.Until.IsZero() {
		params["until"] = formatChatTime(values.Until, chatClockOf(chat.Id), "02.01.2006 15:04")
	}
	text, _ := tr.GetString(spec.DefaultKey, params)
	lines := []string{text}
	if values.Reason != "" {
		reason, _ := tr.GetString("eventmsg_pm_reason", i18n.TranslationParams{"reason": html.EscapeString(values.Reason)})
		lines = append(lines, reason)
	}
	if note != "" {
		lines = append(lines, note)
	}
	return strings.Join(lines, "\n")
}
