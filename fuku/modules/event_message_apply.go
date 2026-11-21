package modules

import (
	"context"

	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/eventmsg"
)

// eventMsgWrites holds the event message repository writes the apply functions call. It is a
// variable only so a test can make a write fail.
var eventMsgWrites = struct {
	Silent    func(ctx context.Context, chatID int64, event, dest string, silent bool, by int64) error
	Text      func(ctx context.Context, chatID int64, event, dest, text string, by int64) error
	Media     func(ctx context.Context, chatID int64, event, dest, fileID string, mediaType int, by int64) error
	Buttons   func(ctx context.Context, chatID int64, event, dest string, buttons []db.Button, by int64) error
	PMDetails func(ctx context.Context, chatID int64, on bool, by int64) error
	PMLog     func(ctx context.Context, chatID int64, on bool, by int64) error
}{
	Silent:    eventmsg.SetSilent,
	Text:      eventmsg.SetText,
	Media:     eventmsg.SetMedia,
	Buttons:   eventmsg.SetButtons,
	PMDetails: eventmsg.SetPMDetails,
	PMLog:     eventmsg.SetPMLog,
}

// eventMsgApplyResult is what an event message apply function returns: the outcome, the stored
// message of the event and destination read back fresh (nil when the write failed, the read-back
// did or the event has no row of that destination), the whole fresh read the row came from (nil when
// the write or the read-back failed, in which case the caller reloads; a screen that also shows
// another destination's state draws from it) and the repository error.
type eventMsgApplyResult struct {
	Outcome  greetingApplyOutcome
	Row      *eventmsg.Message
	Messages *eventmsg.Messages
	Err      error
}

// eventMsgReadBack is one fresh read of a chat's event messages and the row of one event and
// destination in it.
type eventMsgReadBack struct {
	msgs *eventmsg.Messages
	row  *eventmsg.Message
}

// eventMsgApplyReadBack finishes an event message write: on success it reads the chat's messages
// back fresh and keeps them with the row of the event and destination. what names the part in the log
// lines, which carry the chat ID, the event, the destination and the part only, never a text, a file
// ID or a button.
func eventMsgApplyReadBack(ctx context.Context, chatID int64, event, dest, what string, writeErr error) eventMsgApplyResult {
	back, outcome, err := applyReadBack(ctx, writeErr,
		func(e error) {
			log.Errorf("[EventMessages] storing the %s %s %s failed for chat %d: %v", event, dest, what, chatID, e)
		},
		func(e error) {
			log.Warnf("[EventMessages] read back the %s %s message of chat %d after its %s: %v", event, dest, chatID, what, e)
		},
		func(c context.Context) (eventMsgReadBack, error) {
			msgs, readErr := eventmsg.GetFresh(c, chatID)
			if readErr != nil {
				return eventMsgReadBack{}, readErr
			}
			msg, ok := msgs.Lookup(event, dest)
			if !ok {
				return eventMsgReadBack{msgs: msgs}, nil
			}
			return eventMsgReadBack{msgs: msgs, row: &msg}, nil
		})
	return eventMsgApplyResult{Outcome: outcome, Row: back.row, Messages: back.msgs, Err: err}
}

// applyEventSilent stores the Silent switch of one event's group message. It is the one write path
// of the Messages page's Silent switch, which has no command, and it judges no person's rights (the
// press pipeline judged them). The value is absolute, so a repeated call stores the same row. The
// repository drops event_messages:<chat>, which is in skipLocal, so the send path reads the new
// value at the very next command of that event on every replica.
func applyEventSilent(ctx context.Context, chatID int64, event string, on bool, by int64) eventMsgApplyResult {
	dest := eventmsg.DestGroup
	writeErr := eventMsgWrites.Silent(ctx, chatID, event, dest, on, by)
	return eventMsgApplyReadBack(ctx, chatID, event, dest, "Silent switch", writeErr)
}

// applyEventPM stores the switch of one event's private copy: on writes the pm row's Silent false, off
// writes it true (D-08: "Silent" means nothing is sent to that destination, and a pm row whose switch was
// never written reads as off). It is the one write path of the private copy screen's switch, which has
// no command, and it judges no person's rights (the press pipeline judged them). The value is absolute,
// so a repeated call stores the same row. The repository drops event_messages:<chat>, which is in
// skipLocal, so the send path reads the new switch at the very next command of that event on every
// replica.
func applyEventPM(ctx context.Context, chatID int64, event string, on bool, by int64) eventMsgApplyResult {
	writeErr := eventMsgWrites.Silent(ctx, chatID, event, eventmsg.DestPM, !on, by)
	return eventMsgApplyReadBack(ctx, chatID, event, eventmsg.DestPM, "private copy switch", writeErr)
}

// applyEventText stores the text of one event's message for a destination (the group message or the
// private copy): the HTML the editor's check made of the owner's typed message, or "" to remove it
// (the media and buttons stay). It is the one write path of the text part, which has no command, and
// it judges no person's rights (the press pipeline or the typed-reply door judged them). A new pm row
// is stored with its switch off, so storing a part never turns a private copy on. The repository drops
// event_messages:<chat>, which is in skipLocal, so the send path reads the new text at the very next
// command of that event.
func applyEventText(ctx context.Context, chatID int64, event, dest, text string, by int64) eventMsgApplyResult {
	writeErr := eventMsgWrites.Text(ctx, chatID, event, dest, text, by)
	return eventMsgApplyReadBack(ctx, chatID, event, dest, "text", writeErr)
}

// applyEventMedia stores the one media item of one event's message for a destination: its file ID and
// media type together, or "" and 0 to remove it (the text and buttons stay). It is the one write path of
// the media part, and it judges no person's rights.
func applyEventMedia(ctx context.Context, chatID int64, event, dest, fileID string, mediaType int, by int64) eventMsgApplyResult {
	writeErr := eventMsgWrites.Media(ctx, chatID, event, dest, fileID, mediaType, by)
	return eventMsgApplyReadBack(ctx, chatID, event, dest, "media", writeErr)
}

// applyEventButtons replaces the URL buttons of one event's message for a destination, or removes them
// with nil (the text and media stay). It is the one write path of the buttons part, and it judges no
// person's rights.
func applyEventButtons(ctx context.Context, chatID int64, event, dest string, buttons []db.Button, by int64) eventMsgApplyResult {
	writeErr := eventMsgWrites.Buttons(ctx, chatID, event, dest, buttons, by)
	return eventMsgApplyReadBack(ctx, chatID, event, dest, "buttons", writeErr)
}

// eventMsgOptionsResult is what an apply function of the group's private copy options returns: the outcome, the chat's
// event messages read back fresh after the write (nil when the write or the read-back failed; their Options hold the
// stored options) and the repository error.
type eventMsgOptionsResult struct {
	Outcome  greetingApplyOutcome
	Messages *eventmsg.Messages
	Err      error
}

// eventMsgOptionsReadBack finishes a write of one of the group's private copy options: on success it reads the chat's
// messages back fresh. what names the option in the log lines, which carry the chat ID and the option only.
func eventMsgOptionsReadBack(ctx context.Context, chatID int64, what string, writeErr error) eventMsgOptionsResult {
	msgs, outcome, err := applyReadBack(ctx, writeErr,
		func(e error) {
			log.Errorf("[EventMessages] storing the private copy %s option failed for chat %d: %v", what, chatID, e)
		},
		func(e error) {
			log.Warnf("[EventMessages] read back the private copy options of chat %d after the %s option: %v", chatID, what, e)
		},
		func(c context.Context) (*eventmsg.Messages, error) {
			return eventmsg.GetFresh(c, chatID)
		})
	return eventMsgOptionsResult{Outcome: outcome, Messages: msgs, Err: err}
}

// applyEventPMDetails stores the group's Details option of the private copies (D-24): with it on, a private copy that
// was not delivered adds a Details button to the group message. It is the option's one write path, which has no command,
// and it judges no person's rights (the press pipeline judged them). The value is absolute, so a repeated call stores the
// same row. The repository drops event_messages:<chat>, which is in skipLocal, so the send path reads the new option at
// the very next command on every replica. The messages are read back fresh.
func applyEventPMDetails(ctx context.Context, chatID int64, on bool, by int64) eventMsgOptionsResult {
	writeErr := eventMsgWrites.PMDetails(ctx, chatID, on, by)
	return eventMsgOptionsReadBack(ctx, chatID, "Details", writeErr)
}

// applyEventPMLog stores the group's log option of the private copies (D-24): with it on, a private copy that was not
// delivered leaves one line in the group's log channel (pmCopyPostLog). It is the option's one write path, which has no
// command, and it judges no person's rights (the press pipeline judged them). The value is absolute, so a repeated call
// stores the same row. The repository drops event_messages:<chat>, which is in skipLocal, so the send path reads the new
// option at the very next command on every replica. The messages are read back fresh.
func applyEventPMLog(ctx context.Context, chatID int64, on bool, by int64) eventMsgOptionsResult {
	writeErr := eventMsgWrites.PMLog(ctx, chatID, on, by)
	return eventMsgOptionsReadBack(ctx, chatID, "log", writeErr)
}
