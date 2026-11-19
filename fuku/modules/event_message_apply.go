package modules

import (
	"context"

	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db/eventmsg"
)

// eventMsgWrites holds the event message repository writes the apply functions call. It is a
// variable only so a test can make a write fail.
var eventMsgWrites = struct {
	Silent func(ctx context.Context, chatID int64, event, dest string, silent bool, by int64) error
}{
	Silent: eventmsg.SetSilent,
}

// eventMsgApplyResult is what an event message apply function returns: the outcome, the stored
// message of the event read back fresh (nil when the write failed or the read-back did, in which
// case the caller reloads) and the repository error.
type eventMsgApplyResult struct {
	Outcome greetingApplyOutcome
	Row     *eventmsg.Message
	Err     error
}

// applyEventSilent stores the Silent switch of one event's group message. It is the one write path
// of the Messages page's Silent switch, which has no command, and it judges no person's rights (the
// press pipeline judged them). The value is absolute, so a repeated call stores the same row. The
// repository drops event_messages:<chat>, which is in skipLocal, so the send path reads the new
// value at the very next command of that event on every replica.
func applyEventSilent(ctx context.Context, chatID int64, event string, on bool, by int64) eventMsgApplyResult {
	writeErr := eventMsgWrites.Silent(ctx, chatID, event, eventmsg.DestGroup, on, by)
	row, outcome, err := applyReadBack(ctx, writeErr,
		func(e error) {
			log.Errorf("[EventMessages] storing the %s Silent switch failed for chat %d: %v", event, chatID, e)
		},
		func(e error) {
			log.Warnf("[EventMessages] read back the %s message of chat %d after its Silent switch: %v", event, chatID, e)
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
