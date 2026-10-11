//go:build testtools

package modules

import (
	"fmt"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db/models"
)

// alertEdits returns the editMessageText calls on message msgID of chatID, in order.
func (e *lockdownEnv) alertEdits(chatID, msgID int64) []moduleBotCall {
	var matched []moduleBotCall
	for _, call := range e.staffCalls("editMessageText", chatID) {
		if staffParamInt(call.Params, "message_id") == msgID {
			matched = append(matched, call)
		}
	}
	return matched
}

// lockedWithAlert locks the env's chat, lets the worker post the alert in a new Staff
// Group and returns the lockdown, the Staff Group and the alert's message.
func lockedWithAlert(t *testing.T, env *lockdownEnv) (*models.ChatLockdown, gotgbot.Chat, staffSentMessage) {
	t.Helper()
	staffChat := linkLockdownStaffGroup(env)
	row := env.lockAsAdmin("raid")
	env.cycle()
	alerts := env.fake.sentTo(staffChat.Id)
	if len(alerts) != 1 {
		t.Fatalf("%d messages reached the Staff Group, want exactly the alert", len(alerts))
	}
	return row, staffChat, alerts[0]
}

// wantLiftedAlert fails unless the alert's last edit is its body followed by the lifted
// footer naming the lifter, with no buttons, and the copy row is closed.
func (e *lockdownEnv) wantLiftedAlert(staffChat gotgbot.Chat, alert staffSentMessage) {
	e.t.Helper()
	edits := e.alertEdits(staffChat.Id, alert.MessageID)
	if len(edits) == 0 {
		e.t.Fatalf("the alert was never edited, want its body plus %q", staffMarker("lockdown_alert_lifted_footer"))
	}
	last := edits[len(edits)-1]
	text := fmt.Sprint(last.Params["text"])
	body := fmt.Sprint(alert.Params["text"])
	if !strings.HasPrefix(text, body) {
		e.t.Errorf("edited alert %q does not start with the body as posted %q", text, body)
	}
	footer := text[min(len(body), len(text)):]
	for _, want := range []string{staffMarker("lockdown_alert_lifted_footer"), "Ad&lt;b&gt;min"} {
		if !strings.Contains(footer, want) {
			e.t.Errorf("footer %q does not contain %q", footer, want)
		}
	}
	wantNoKeyboard(e.t, last)
	rows := e.alertRows()
	if len(rows) != 1 || rows[0].ClosedAt == nil || rows[0].ButtonsLive || rows[0].ClaimedAt != nil {
		e.t.Errorf("alert rows = %+v, want one closed copy with no live buttons and no claim", rows)
	}
}

func TestLockdownAlertClosedAtLift(t *testing.T) {
	t.Run("command", func(t *testing.T) {
		env := newLockdownEnv(t)
		_, staffChat, alert := lockedWithAlert(t, env)

		env.send(env.admin, "/unlockdown")
		env.cycle()

		env.wantLiftedAlert(staffChat, alert)
		if got := len(env.alertEdits(staffChat.Id, alert.MessageID)); got != 1 {
			t.Fatalf("%d edits of the alert, want exactly 1", got)
		}

		env.cycle()
		env.cycle()
		if got := len(env.alertEdits(staffChat.Id, alert.MessageID)); got != 1 {
			t.Errorf("%d edits of the alert after further cycles, want still 1", got)
		}
	})

	t.Run("button", func(t *testing.T) {
		env := newLockdownEnv(t)
		card := askToLiftLockdown(t, env)
		alert := env.fake.sentTo(card.staffChat.Id)[0]

		pressLockdownButton(env, env.admin, card.staffChat, card.msgID, card.confirm)
		env.cycle()

		env.wantLiftedAlert(card.staffChat, alert)
		if got := len(env.alertEdits(card.staffChat.Id, alert.MessageID)); got != 1 {
			t.Errorf("%d edits of the alert, want exactly 1", got)
		}
	})

	t.Run("gone", func(t *testing.T) {
		env := newLockdownEnv(t)
		_, staffChat, alert := lockedWithAlert(t, env)
		env.fake.script("editMessageText", staffChat.Id, staffFakeError(400, "Bad Request: message to edit not found"))

		env.send(env.admin, "/unlockdown")
		env.cycle()

		rows := env.alertRows()
		if len(rows) != 1 || rows[0].ClosedAt == nil || rows[0].ButtonsLive {
			t.Fatalf("alert rows = %+v, want the copy closed because its message is gone", rows)
		}
		env.cycle()
		env.cycle()
		if got := len(env.alertEdits(staffChat.Id, alert.MessageID)); got != 1 {
			t.Errorf("%d edit attempts, want exactly 1: a gone message is not retried", got)
		}
	})

	t.Run("not modified", func(t *testing.T) {
		env := newLockdownEnv(t)
		_, staffChat, alert := lockedWithAlert(t, env)
		env.fake.script("editMessageText", staffChat.Id, staffFakeError(400, "Bad Request: message is not modified"))

		env.send(env.admin, "/unlockdown")
		env.cycle()

		rows := env.alertRows()
		if len(rows) != 1 || rows[0].ClosedAt == nil || rows[0].Attempts != 0 {
			t.Fatalf("alert rows = %+v, want the copy closed as success with no attempt", rows)
		}
		env.cycle()
		if got := len(env.alertEdits(staffChat.Id, alert.MessageID)); got != 1 {
			t.Errorf("%d edit attempts, want exactly 1", got)
		}
	})

	t.Run("rate limit", func(t *testing.T) {
		env := newLockdownEnv(t)
		_, staffChat, alert := lockedWithAlert(t, env)
		limited := staffFake429(1)
		env.fake.script("editMessageText", staffChat.Id, limited, limited, limited, limited)

		env.send(env.admin, "/unlockdown")
		env.cycle()

		rows := env.alertRows()
		if len(rows) != 1 || rows[0].ClosedAt != nil || rows[0].Attempts != 0 || rows[0].ClaimedAt != nil {
			t.Fatalf("alert rows = %+v, want the copy open, unclaimed, with 0 attempts after a rate limit", rows)
		}

		env.cycle()
		env.wantLiftedAlert(staffChat, alert)
	})

	t.Run("other failure is given up after three attempts", func(t *testing.T) {
		env := newLockdownEnv(t)
		_, staffChat, alert := lockedWithAlert(t, env)
		refusal := staffFakeError(400, "Bad Request: can't parse entities <b>")
		env.fake.script("editMessageText", staffChat.Id, refusal, refusal, refusal)

		env.send(env.admin, "/unlockdown")
		env.cycle()
		env.cycle()
		rows := env.alertRows()
		if len(rows) != 1 || rows[0].ClosedAt != nil || rows[0].Attempts != 2 {
			t.Fatalf("alert rows = %+v, want the copy open with 2 attempts", rows)
		}
		env.cycle()

		rows = env.alertRows()
		if len(rows) != 1 || rows[0].ClosedAt == nil ||
			!strings.Contains(rows[0].Detail, "can&#39;t parse entities &lt;b&gt;") {
			t.Fatalf("alert rows = %+v, want the copy given up after 3 attempts with Telegram's reason", rows)
		}
		env.cycle()
		if got := len(env.alertEdits(staffChat.Id, alert.MessageID)); got != 3 {
			t.Errorf("%d edit attempts, want 3: a given-up copy is not tried again", got)
		}
	})
}
