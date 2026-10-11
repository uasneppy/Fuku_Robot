//go:build testtools

package modules

import (
	"fmt"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/lockdown"
	"github.com/divkix/Alita_Robot/alita/db/logchannels"
	"github.com/divkix/Alita_Robot/alita/db/models"
)

// alertRowTo returns the env's alert copy row posted (or planned) for destChatID, or
// fails when there is none.
func (e *lockdownEnv) alertRowTo(destChatID int64) models.ChatLockdownAlert {
	e.t.Helper()
	for _, row := range e.alertRows() {
		if row.DestChatID == destChatID {
			return row
		}
	}
	e.t.Fatalf("no alert copy row for chat %d in %+v", destChatID, e.alertRows())
	return models.ChatLockdownAlert{}
}

// cleanupAlerts deletes the alert copy rows of the env's chat when the test ends, for a
// test that links no Staff Group (which registers the same cleanup).
func (e *lockdownEnv) cleanupAlerts() {
	e.t.Helper()
	e.t.Cleanup(func() {
		db.DB.Where("chat_id = ?", e.chat.Id).Delete(&models.ChatLockdownAlert{})
	})
}

// wantAlertHeader is the header a log channel copy of the env's chat starts with.
func (e *lockdownEnv) wantAlertHeader() string {
	return fmt.Sprintf("<b>%s</b> (<code>%d</code>)\n", e.chat.Title, e.chat.Id)
}

func TestLockdownAlertDestinations(t *testing.T) {
	t.Run("both", func(t *testing.T) {
		env := newLockdownEnv(t)
		staffChat := linkLockdownStaffGroup(env)
		channel := uniqueModuleChatID()
		setStaffLogChannel(t, env.chat.Id, channel)

		env.send(env.admin, "/lockdown raid")
		replies := len(env.replies())
		env.cycle()
		env.cycle()

		toStaff, toLog := env.fake.sentTo(staffChat.Id), env.fake.sentTo(channel)
		if len(toStaff) != 1 || len(toLog) != 1 {
			t.Fatalf("%d copies reached the Staff Group and %d the log channel, want 1 each", len(toStaff), len(toLog))
		}
		logText := fmt.Sprint(toLog[0].Params["text"])
		if !strings.HasPrefix(logText, env.wantAlertHeader()) {
			t.Errorf("log copy %q does not start with the usual log header %q", logText, env.wantAlertHeader())
		}
		if !strings.Contains(logText, staffMarker("lockdown_alert_title")) {
			t.Errorf("log copy %q does not contain the alert title", logText)
		}
		if staffText := fmt.Sprint(toStaff[0].Params["text"]); strings.HasPrefix(staffText, env.wantAlertHeader()) {
			t.Errorf("Staff Group copy %q carries the log header, want none", staffText)
		}
		for name, sent := range map[string]staffSentMessage{"staff": toStaff[0], "log": toLog[0]} {
			if data := lockdownButtonData(t, sent.Params["reply_markup"], "lk"); data == "" {
				t.Errorf("%s copy has no Lift button", name)
			}
		}

		rows := env.alertRows()
		if len(rows) != 2 {
			t.Fatalf("%d copy rows, want 2: %+v", len(rows), rows)
		}
		staffRow, logRow := env.alertRowTo(staffChat.Id), env.alertRowTo(channel)
		if staffRow.Dest != models.AlertDestStaff || logRow.Dest != models.AlertDestLog {
			t.Errorf("destinations = %q and %q, want staff and log", staffRow.Dest, logRow.Dest)
		}
		if logRow.ChatID != env.chat.Id || logRow.State != models.AlertStatePosted || logRow.Body != logText {
			t.Errorf("log row = %+v, want a posted copy of the locked group with the exact text sent", logRow)
		}
		if got := len(env.replies()); got != replies {
			t.Errorf("the locked group received %d messages after /lockdown's reply, want none", got-replies)
		}
	})

	t.Run("category off", func(t *testing.T) {
		env := newLockdownEnv(t)
		staffChat := linkLockdownStaffGroup(env)
		channel := uniqueModuleChatID()
		setStaffLogChannel(t, env.chat.Id, channel)
		if err := logchannels.SetCategory(env.chat.Id, logchannels.CategoryAdmin, false); err != nil {
			t.Fatalf("turn the admin category off: %v", err)
		}

		env.send(env.admin, "/lockdown raid")
		env.cycle()
		env.cycle()

		if got := len(env.fake.sentTo(staffChat.Id)); got != 1 {
			t.Errorf("%d copies reached the Staff Group, want 1", got)
		}
		if got := len(env.fake.sentTo(channel)); got != 0 {
			t.Errorf("%d copies reached a log channel whose admin category is off, want none", got)
		}
		if rows := env.alertRows(); len(rows) != 1 || rows[0].Dest != models.AlertDestStaff {
			t.Errorf("copy rows = %+v, want only the staff copy", rows)
		}
	})

	t.Run("log only", func(t *testing.T) {
		env := newLockdownEnv(t)
		channel := uniqueModuleChatID()
		setStaffLogChannel(t, env.chat.Id, channel)
		env.cleanupAlerts()

		env.send(env.admin, "/lockdown raid")
		env.cycle()
		env.cycle()

		if got := len(env.fake.sentTo(channel)); got != 1 {
			t.Errorf("%d copies reached the log channel, want 1", got)
		}
		if rows := env.alertRows(); len(rows) != 1 || rows[0].Dest != models.AlertDestLog || rows[0].DestChatID != channel {
			t.Errorf("copy rows = %+v, want only the log copy", rows)
		}
		for _, chat := range env.sentChats() {
			if chat != env.chat.Id && chat != channel {
				t.Errorf("a message went to chat %d, want only the locked group's reply and the log channel", chat)
			}
		}
	})

	t.Run("neither", func(t *testing.T) {
		env := newLockdownEnv(t)
		env.cleanupAlerts()

		env.send(env.admin, "/lockdown raid")
		env.cycle()
		env.cycle()

		if rows := env.alertRows(); len(rows) != 0 {
			t.Errorf("copy rows = %+v, want none for a group with neither destination", rows)
		}
		active, err := lockdown.GetActiveFresh(env.chat.Id)
		if err != nil || active == nil || active.AlertStartedAt == nil {
			t.Fatalf("GetActiveFresh = %+v, %v, want the lockdown with alert_started_at set", active, err)
		}
		env.wantSentOnlyTo(env.chat.Id)
	})

	t.Run("linked later", func(t *testing.T) {
		env := newLockdownEnv(t)
		staffChat := newLockdownStaffGroup(env)
		channel := uniqueModuleChatID()
		setStaffLogChannel(t, env.chat.Id, channel)

		env.send(env.admin, "/lockdown raid")
		env.cycle()
		linkLockdownGroupTo(env, staffChat)
		env.cycle()
		env.cycle()

		if got := len(env.fake.sentTo(channel)); got != 1 {
			t.Errorf("%d copies reached the log channel, want 1", got)
		}
		if got := len(env.fake.sentTo(staffChat.Id)); got != 0 {
			t.Errorf("%d copies reached a Staff Group linked after the alert was decided, want none", got)
		}
	})

	t.Run("category turned off before the post", func(t *testing.T) {
		env := newLockdownEnv(t)
		channel := uniqueModuleChatID()
		setStaffLogChannel(t, env.chat.Id, channel)
		env.cleanupAlerts()

		env.send(env.admin, "/lockdown raid")
		lockdownPlanAlerts(t.Context(), env.bot)
		if err := logchannels.SetCategory(env.chat.Id, logchannels.CategoryAdmin, false); err != nil {
			t.Fatalf("turn the admin category off: %v", err)
		}
		env.cycle()
		env.cycle()

		if got := len(env.fake.sentTo(channel)); got != 0 {
			t.Errorf("%d copies reached a log channel whose admin category was turned off, want none", got)
		}
		if rows := env.alertRows(); len(rows) != 1 || rows[0].State != models.AlertStateFailed || rows[0].Detail == "" {
			t.Errorf("copy rows = %+v, want the log copy ended as failed with a reason", rows)
		}
	})
}

func TestLockdownAlertLiftFromLogChannel(t *testing.T) {
	env := newLockdownEnv(t)
	staffChat := linkLockdownStaffGroup(env)
	channel := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "channel", Title: "Logs"}
	setStaffLogChannel(t, env.chat.Id, channel.Id)

	row := env.lockAsAdmin("raid")
	env.cycle()

	logCopies := env.fake.sentTo(channel.Id)
	staffCopies := env.fake.sentTo(staffChat.Id)
	if len(logCopies) != 1 || len(staffCopies) != 1 {
		t.Fatalf("%d log copies and %d Staff Group copies, want 1 each", len(logCopies), len(staffCopies))
	}
	logCopy := logCopies[0]
	liftData := lockdownButtonData(t, logCopy.Params["reply_markup"], "lk")

	// A plain member is refused on the log channel copy exactly like on the Staff Group's.
	member := env.newOutsider()
	pressLockdownButton(env, member, channel, logCopy.MessageID, liftData)
	if text, _ := env.lastAnswer(); !strings.Contains(text, staffMarker("lockdown_alert_not_admin")) {
		t.Errorf("refusal answer = %q, want %q", text, staffMarker("lockdown_alert_not_admin"))
	}
	if got := len(env.fake.sentTo(channel.Id)); got != 1 {
		t.Fatalf("%d messages in the log channel after a refused press, want only the copy", got)
	}

	// A live admin of the locked group gets the confirm card in the log channel.
	pressLockdownButton(env, env.admin, channel, logCopy.MessageID, liftData)
	posted := env.fake.sentTo(channel.Id)
	if len(posted) != 2 {
		t.Fatalf("%d messages in the log channel, want the copy and one confirm card", len(posted))
	}
	card := posted[1]
	reply, ok := card.Params["reply_parameters"].(*gotgbot.ReplyParameters)
	if !ok || reply.MessageId != logCopy.MessageID {
		t.Fatalf("card reply_parameters = %+v, want a reply to the log copy %d", card.Params["reply_parameters"], logCopy.MessageID)
	}
	if got := len(env.fake.sentTo(staffChat.Id)); got != 1 {
		t.Errorf("%d messages in the Staff Group after a press on the log copy, want only its own copy", got)
	}
	confirmData := lockdownButtonData(t, card.Params["reply_markup"], "lc")

	setsBefore := len(env.calls("setChatPermissions"))
	pressLockdownButton(env, env.admin, channel, card.MessageID, confirmData)

	if got := len(env.calls("setChatPermissions")); got != setsBefore+1 {
		t.Fatalf("setChatPermissions calls = %d, want %d: one restore", got, setsBefore+1)
	}
	if got := env.repliesWith(staffMarker("lockdown_lifted")); len(got) != 1 {
		t.Errorf("%d lift notices reached the locked group, want 1", len(got))
	}
	if lifted := env.freshRow(row.ID); lifted.LiftedBy == nil || *lifted.LiftedBy != env.admin.Id {
		t.Fatalf("LiftedBy = %v, want %d", lifted.LiftedBy, env.admin.Id)
	}

	env.cycle()

	for name, target := range map[string]struct {
		chat gotgbot.Chat
		sent staffSentMessage
	}{
		"log":   {channel, logCopy},
		"staff": {staffChat, staffCopies[0]},
	} {
		edits := env.alertEdits(target.chat.Id, target.sent.MessageID)
		if len(edits) != 1 {
			t.Errorf("%s copy edited %d times, want once", name, len(edits))
			continue
		}
		text := fmt.Sprint(edits[0].Params["text"])
		if !strings.HasPrefix(text, fmt.Sprint(target.sent.Params["text"])) || !strings.Contains(text, staffMarker("lockdown_alert_lifted_footer")) {
			t.Errorf("%s copy edit %q, want the copy's body plus the lifted footer", name, text)
		}
		wantNoKeyboard(t, edits[0])
	}
	for _, copyRow := range env.alertRows() {
		if copyRow.ClosedAt == nil {
			t.Errorf("copy row %+v is not closed, want both closed", copyRow)
		}
	}
}
