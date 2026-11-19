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
	Silent  func(ctx context.Context, chatID int64, event, dest string, silent bool, by int64) error
	Text    func(ctx context.Context, chatID int64, event, dest, text string, by int64) error
	Media   func(ctx context.Context, chatID int64, event, dest, fileID string, mediaType int, by int64) error
	Buttons func(ctx context.Context, chatID int64, event, dest string, buttons []db.Button, by int64) error
}{
	Silent:  eventmsg.SetSilent,
	Text:    eventmsg.SetText,
	Media:   eventmsg.SetMedia,
	Buttons: eventmsg.SetButtons,
}

// eventMsgApplyResult is what an event message apply function returns: the outcome, the stored
// message of the event read back fresh (nil when the write failed or the read-back did, in which
// case the caller reloads) and the repository error.
type eventMsgApplyResult struct {
	Outcome greetingApplyOutcome
	Row     *eventmsg.Message
	Err     error
}

// eventMsgApplyReadBack finishes an event message write: on success it reads the event's group
// message back fresh. what names the part in the log lines, which carry the chat ID, the event and
// the part only, never a text, a file ID or a button.
func eventMsgApplyReadBack(ctx context.Context, chatID int64, event, what string, writeErr error) eventMsgApplyResult {
	row, outcome, err := applyReadBack(ctx, writeErr,
		func(e error) {
			log.Errorf("[EventMessages] storing the %s %s failed for chat %d: %v", event, what, chatID, e)
		},
		func(e error) {
			log.Warnf("[EventMessages] read back the %s message of chat %d after its %s: %v", event, chatID, what, e)
		},
		func(c context.Context) (*eventmsg.Message, error) {
			msgs, readErr := eventmsg.GetFresh(c, chatID)
			if readErr != nil {
				return nil, readErr
			}
			msg, ok := msgs.Lookup(event, eventmsg.DestGroup)
			if !ok {
				return nil, nil
			}
			return &msg, nil
		})
	return eventMsgApplyResult{Outcome: outcome, Row: row, Err: err}
}

// applyEventSilent stores the Silent switch of one event's group message. It is the one write path
// of the Messages page's Silent switch, which has no command, and it judges no person's rights (the
// press pipeline judged them). The value is absolute, so a repeated call stores the same row. The
// repository drops event_messages:<chat>, which is in skipLocal, so the send path reads the new
// value at the very next command of that event on every replica.
func applyEventSilent(ctx context.Context, chatID int64, event string, on bool, by int64) eventMsgApplyResult {
	writeErr := eventMsgWrites.Silent(ctx, chatID, event, eventmsg.DestGroup, on, by)
	return eventMsgApplyReadBack(ctx, chatID, event, "Silent switch", writeErr)
}

// applyEventText stores the text of one event's group message: the HTML the editor's check made of
// the owner's typed message, or "" to remove it (the media and buttons stay). It is the one write
// path of the text part, which has no command, and it judges no person's rights (the press pipeline
// or the typed-reply door judged them). The repository drops event_messages:<chat>, which is in
// skipLocal, so the send path reads the new text at the very next command of that event.
func applyEventText(ctx context.Context, chatID int64, event, text string, by int64) eventMsgApplyResult {
	writeErr := eventMsgWrites.Text(ctx, chatID, event, eventmsg.DestGroup, text, by)
	return eventMsgApplyReadBack(ctx, chatID, event, "text", writeErr)
}

// applyEventMedia stores the one media item of one event's group message: its file ID and media type
// together, or "" and 0 to remove it (the text and buttons stay). It is the one write path of the media
// part, and it judges no person's rights.
func applyEventMedia(ctx context.Context, chatID int64, event, fileID string, mediaType int, by int64) eventMsgApplyResult {
	writeErr := eventMsgWrites.Media(ctx, chatID, event, eventmsg.DestGroup, fileID, mediaType, by)
	return eventMsgApplyReadBack(ctx, chatID, event, "media", writeErr)
}

// applyEventButtons replaces the URL buttons of one event's group message, or removes them with nil
// (the text and media stay). It is the one write path of the buttons part, and it judges no person's
// rights.
func applyEventButtons(ctx context.Context, chatID int64, event string, buttons []db.Button, by int64) eventMsgApplyResult {
	writeErr := eventMsgWrites.Buttons(ctx, chatID, event, eventmsg.DestGroup, buttons, by)
	return eventMsgApplyReadBack(ctx, chatID, event, "buttons", writeErr)
}
