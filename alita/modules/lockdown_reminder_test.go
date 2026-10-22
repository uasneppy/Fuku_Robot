//go:build testtools

package modules

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/lockdown"
	"github.com/divkix/Alita_Robot/alita/db/logchannels"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/db/staff"
)

// ageAlert moves the alert_started_at of a lockdown back by the given time with a
// direct update, so a test can show what the hourly reminders do without sleeping.
func ageAlert(t *testing.T, lockdownID uint, by time.Duration) {
	t.Helper()
	err := db.DB.Model(&models.ChatLockdown{}).Where("id = ?", lockdownID).
		UpdateColumn("alert_started_at", time.Now().UTC().Add(-by)).Error
	if err != nil {
		t.Fatalf("age the alert of lockdown %d: %v", lockdownID, err)
	}
}

// reminderRows reads the reminder copy rows of the env's chat, oldest first.
func (e *lockdownEnv) reminderRows() []models.ChatLockdownAlert {
	e.t.Helper()
	var rows []models.ChatLockdownAlert
	for _, row := range e.alertRows() {
		if row.Kind == models.AlertKindReminder {
			rows = append(rows, row)
		}
	}
	return rows
}

// reminderScene is a locked group whose alert reached a Staff Group and a log channel.
type reminderScene struct {
	env     *lockdownEnv
	staff   gotgbot.Chat
	channel int64
	row     *models.ChatLockdown
}

// newReminderScene locks a group with reason "raid" and lets the worker post the alert
// in both destinations.
func newReminderScene(t *testing.T) *reminderScene {
	t.Helper()
	env := newLockdownEnv(t)
	staffChat := lockdownBurstEnv(env)
	channel := uniqueModuleChatID()
	setStaffLogChannel(t, env.chat.Id, channel)
	row := env.lockAsAdmin("raid")
	env.cycle()
	if len(env.fake.sentTo(staffChat.Id)) != 1 || len(env.fake.sentTo(channel)) != 1 {
		t.Fatalf("%d alerts reached the Staff Group and %d the log channel, want 1 each",
			len(env.fake.sentTo(staffChat.Id)), len(env.fake.sentTo(channel)))
	}
	return &reminderScene{env: env, staff: staffChat, channel: channel, row: row}
}

// posted are the messages sent to the Staff Group and to the log channel so far.
func (s *reminderScene) posted() (toStaff, toLog []staffSentMessage) {
	return s.env.fake.sentTo(s.staff.Id), s.env.fake.sentTo(s.channel)
}

// wantPosted fails unless exactly want messages reached each destination.
func (s *reminderScene) wantPosted(want int) {
	s.env.t.Helper()
	toStaff, toLog := s.posted()
	if len(toStaff) != want || len(toLog) != want {
		s.env.t.Fatalf("%d messages reached the Staff Group and %d the log channel, want %d each", len(toStaff), len(toLog), want)
	}
}

func TestLockdownReminders(t *testing.T) {
	t.Run("first", func(t *testing.T) {
		s := newReminderScene(t)
		env := s.env
		ageAlert(t, s.row.ID, 61*time.Minute)

		env.cycle()

		s.wantPosted(2)
		toStaff, toLog := s.posted()
		for name, sent := range map[string]staffSentMessage{"staff": toStaff[1], "log": toLog[1]} {
			text := fmt.Sprint(sent.Params["text"])
			if !strings.Contains(text, staffMarker("lockdown_reminder_title")) {
				t.Errorf("%s reminder %q does not contain the reminder title", name, text)
			}
			lockdownButtonData(t, sent.Params["reply_markup"], "lk")
			lockdownButtonData(t, sent.Params["reply_markup"], "bn")
		}
		if text := fmt.Sprint(toLog[1].Params["text"]); !strings.HasPrefix(text, env.wantAlertHeader()) {
			t.Errorf("log reminder %q does not start with the usual log header %q", text, env.wantAlertHeader())
		}
		rows := env.reminderRows()
		if len(rows) != 2 {
			t.Fatalf("%d reminder rows, want 2: %+v", len(rows), rows)
		}
		for _, row := range rows {
			if row.Seq != 1 || row.State != models.AlertStatePosted || row.Kind != models.AlertKindReminder {
				t.Errorf("reminder row = %+v, want a posted reminder of seq 1", row)
			}
		}

		env.cycle()
		env.cycle()
		s.wantPosted(2)
	})

	t.Run("second", func(t *testing.T) {
		s := newReminderScene(t)
		env := s.env
		ageAlert(t, s.row.ID, 61*time.Minute)
		env.cycle()
		ageAlert(t, s.row.ID, 121*time.Minute)

		env.cycle()
		env.cycle()
		env.cycle()

		s.wantPosted(3)
		toStaff, toLog := s.posted()
		for name, copies := range map[string]struct {
			chat int64
			sent []staffSentMessage
		}{"staff": {s.staff.Id, toStaff}, "log": {s.channel, toLog}} {
			alert, first, second := copies.sent[0], copies.sent[1], copies.sent[2]
			if text := fmt.Sprint(second.Params["text"]); !strings.Contains(text, staffMarker("lockdown_reminder_title")) {
				t.Errorf("%s second reminder %q does not contain the reminder title", name, text)
			}
			edits := env.alertEdits(copies.chat, first.MessageID)
			if len(edits) != 1 {
				t.Errorf("%s first reminder was edited %d times, want once", name, len(edits))
			} else {
				wantNoKeyboard(t, edits[0])
			}
			if got := len(env.alertEdits(copies.chat, alert.MessageID)); got != 0 {
				t.Errorf("%s original alert was edited %d times, want none: it keeps its buttons until the lift", name, got)
			}
			if got := len(env.alertEdits(copies.chat, second.MessageID)); got != 0 {
				t.Errorf("%s newest reminder was edited %d times, want none", name, got)
			}
		}
		for _, row := range env.alertRows() {
			wantLive := row.Kind == models.AlertKindAlert || row.Seq == 2
			if row.ButtonsLive != wantLive || row.EditPending {
				t.Errorf("copy row %+v, want buttons_live %v and no edit pending", row, wantLive)
			}
		}
	})

	t.Run("no catch-up", func(t *testing.T) {
		s := newReminderScene(t)
		env := s.env
		ageAlert(t, s.row.ID, 5*time.Hour+30*time.Minute)

		env.cycle()
		env.cycle()

		s.wantPosted(2)
		rows := env.reminderRows()
		if len(rows) != 2 {
			t.Fatalf("%d reminder rows, want 2: %+v", len(rows), rows)
		}
		for _, row := range rows {
			if row.Seq != 5 {
				t.Errorf("reminder row = %+v, want only seq 5", row)
			}
		}
	})

	t.Run("concurrent", func(t *testing.T) {
		s := newReminderScene(t)
		env := s.env
		ageAlert(t, s.row.ID, 61*time.Minute)

		var wg sync.WaitGroup
		for range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				runLockdownCycle(context.Background(), env.bot)
			}()
		}
		wg.Wait()
		// A cycle that lost a race leaves its copy for the next one; nothing is posted twice.
		env.cycle()
		env.cycle()

		s.wantPosted(2)
		if rows := env.reminderRows(); len(rows) != 2 {
			t.Errorf("%d reminder rows, want 2: %+v", len(rows), rows)
		}
	})

	t.Run("stops at the lift", func(t *testing.T) {
		s := newReminderScene(t)
		env := s.env
		ageAlert(t, s.row.ID, 61*time.Minute)
		env.cycle()
		s.wantPosted(2)

		env.send(env.admin, "/unlockdown")
		ageAlert(t, s.row.ID, 3*time.Hour)
		env.cycleUntilIdle()

		s.wantPosted(2)
		if rows := env.reminderRows(); len(rows) != 2 {
			t.Errorf("%d reminder rows after the lift, want still 2: %+v", len(rows), rows)
		}
		toStaff, toLog := s.posted()
		for name, copies := range map[string]struct {
			chat int64
			sent []staffSentMessage
		}{"staff": {s.staff.Id, toStaff}, "log": {s.channel, toLog}} {
			for i, label := range []string{"alert", "reminder"} {
				edits := env.alertEdits(copies.chat, copies.sent[i].MessageID)
				if len(edits) != 1 {
					t.Errorf("%s %s was edited %d times, want once", name, label, len(edits))
					continue
				}
				if text := fmt.Sprint(edits[0].Params["text"]); !strings.Contains(text, staffMarker("lockdown_alert_lifted_footer")) {
					t.Errorf("%s %s edit %q does not contain the lifted footer", name, label, text)
				}
				wantNoKeyboard(t, edits[0])
			}
		}
		for _, row := range env.alertRows() {
			if row.ClosedAt == nil {
				t.Errorf("copy row %+v is not closed, want every alert and reminder closed", row)
			}
		}
	})

	t.Run("stale pending", func(t *testing.T) {
		s := newReminderScene(t)
		env := s.env
		stale := func(seq int) []models.ChatLockdownAlert {
			return []models.ChatLockdownAlert{
				{LockdownID: s.row.ID, ChatID: env.chat.Id, Kind: models.AlertKindReminder, Seq: seq, Dest: models.AlertDestStaff, DestChatID: s.staff.Id},
				{LockdownID: s.row.ID, ChatID: env.chat.Id, Kind: models.AlertKindReminder, Seq: seq, Dest: models.AlertDestLog, DestChatID: s.channel},
			}
		}
		if inserted, err := lockdown.PlanReminderSlots(s.row.ID, 1, stale(1)); err != nil || inserted != 2 {
			t.Fatalf("PlanReminderSlots = %d, %v, want the two pending seq 1 rows (their send never happened)", inserted, err)
		}
		ageAlert(t, s.row.ID, 121*time.Minute)

		env.cycle()
		env.cycle()

		s.wantPosted(2)
		toStaff, toLog := s.posted()
		for name, sent := range map[string]staffSentMessage{"staff": toStaff[1], "log": toLog[1]} {
			if text := fmt.Sprint(sent.Params["text"]); !strings.Contains(text, staffMarker("lockdown_reminder_title")) {
				t.Errorf("%s message %q is not the reminder", name, text)
			}
		}
		for _, row := range env.reminderRows() {
			switch row.Seq {
			case 1:
				if row.State != models.AlertStateFailed || row.Detail != "superseded" {
					t.Errorf("stale reminder row = %+v, want failed with detail superseded", row)
				}
			case 2:
				if row.State != models.AlertStatePosted {
					t.Errorf("current reminder row = %+v, want posted", row)
				}
			default:
				t.Errorf("unexpected reminder row %+v", row)
			}
		}
	})
}

// lastTo is the last message sent to chatID so far.
func (e *lockdownEnv) lastTo(chatID int64) staffSentMessage {
	e.t.Helper()
	sent := e.fake.sentTo(chatID)
	if len(sent) == 0 {
		e.t.Fatalf("nothing was sent to chat %d", chatID)
	}
	return sent[len(sent)-1]
}

// wantKeyboardActions fails unless the keyboard of markup holds exactly the lockdown
// actions in want, in any order.
func wantKeyboardActions(t *testing.T, name string, markup any, want ...string) {
	t.Helper()
	var got []string
	for _, row := range staffKeyboardOf(markup) {
		for _, button := range row {
			if decoded, ok := decodeCallbackData(button.CallbackData, "lockdown"); ok {
				got = append(got, decoded.Fields["a"])
			}
		}
	}
	if len(got) != len(want) {
		t.Errorf("%s keyboard actions = %v, want %v", name, got, want)
		return
	}
	for _, action := range want {
		found := false
		for _, have := range got {
			found = found || have == action
		}
		if !found {
			t.Errorf("%s keyboard actions = %v, want %v", name, got, want)
		}
	}
}

func TestLockdownReminderContent(t *testing.T) {
	t.Run("counts", func(t *testing.T) {
		env := newLockdownEnv(t)
		staffChat := lockdownBurstEnv(env)
		row := env.lockAsAdmin("spam <wave>")
		env.cycle()
		for i := 0; i < 3; i++ {
			seedLockdownJoiner(t, row.ID, env.chat.Id, uniqueLinkOwnerID(), models.JoinerStateBanned)
		}
		seedLockdownJoiner(t, row.ID, env.chat.Id, uniqueLinkOwnerID(), models.JoinerStatePending)
		for i := 0; i < 2; i++ {
			kept := seedLockdownJoiner(t, row.ID, env.chat.Id, uniqueLinkOwnerID(), models.JoinerStateKept)
			if err := db.DB.Model(&models.LockdownJoiner{}).Where("id = ?", kept.ID).
				UpdateColumn("detail", models.JoinerDetailResponseBan).Error; err != nil {
				t.Fatalf("mark joiner %d as a response ban: %v", kept.ID, err)
			}
		}
		ageAlert(t, row.ID, 61*time.Minute)

		env.cycle()

		reminder := env.lastTo(staffChat.Id)
		text := fmt.Sprint(reminder.Params["text"])
		for _, want := range []string{
			staffMarker("lockdown_reminder_title"),
			lockdownTime(*row.LockedAt),
			staffMarker("lockdown_reason_line"),
			"spam &lt;wave&gt;",
			staffMarker("lockdown_reminder_counts") + " 5 1",
		} {
			if !strings.Contains(text, want) {
				t.Errorf("reminder %q does not contain %q", text, want)
			}
		}
		if strings.Contains(text, staffMarker("lockdown_alert_sample_title")) || strings.Contains(text, staffMarker("lockdown_alert_counts")) {
			t.Errorf("reminder %q carries the burst sample, want none (D-16)", text)
		}
	})

	t.Run("buttons", func(t *testing.T) {
		s := newRevokeScene(t, revokeTestLink)
		env := s.env

		// Someone joins during the lockdown and is banned by it: the reminder's Ban count is fresh.
		extra := env.newJoiner("Late")
		env.join(extra, extra, revokeTestLink)
		env.cycle()
		ageAlert(t, s.row.ID, 61*time.Minute)
		env.cycle()

		alertKeyboard := staffKeyboardOf(s.staffMsg.Params["reply_markup"])
		reminder := env.lastTo(s.staff.Id)
		if reminder.MessageID == s.staffMsg.MessageID {
			t.Fatal("no reminder was posted to the Staff Group")
		}
		wantKeyboardActions(t, "reminder", reminder.Params["reply_markup"], "lk", "bn", "rk")
		keyboard := staffKeyboardOf(reminder.Params["reply_markup"])
		if len(keyboard) != 3 {
			t.Fatalf("reminder keyboard = %+v, want Lift, Ban and Revoke on three rows", keyboard)
		}
		if got, was := keyboard[1][0].Text, alertKeyboard[1][0].Text; got == was || !strings.Contains(got, staffMarker("lockdown_alert_btn_ban")+" 3") {
			t.Errorf("reminder Ban label = %q (the alert's was %q), want a fresh count of 3", got, was)
		}
		if rows := env.reminderRows(); len(rows) != 2 || rows[0].Link != revokeTestLink {
			t.Fatalf("reminder rows = %+v, want two copies that stored the link %s", rows, revokeTestLink)
		}

		// Revoke works from the reminder like from the alert.
		before := len(env.fake.sentTo(s.staff.Id))
		s.press(s.asker, reminder.MessageID, lockdownButtonData(t, reminder.Params["reply_markup"], "rk"))
		if len(env.fake.sentTo(s.staff.Id)) != before+1 {
			t.Fatal("a live administrator with can_invite_users got no confirm card from the reminder")
		}
		card := env.lastTo(s.staff.Id)
		s.press(s.asker, card.MessageID, lockdownButtonData(t, card.Params["reply_markup"], "rc"))
		if got := len(s.revokeCalls()); got != 1 {
			t.Fatalf("%d revokeChatInviteLink calls, want 1", got)
		}
		env.cycle()
		edits := env.alertEdits(s.staff.Id, reminder.MessageID)
		if len(edits) != 1 || !strings.Contains(fmt.Sprint(edits[0].Params["text"]), staffMarker("lockdown_alert_link_revoked")) {
			t.Fatalf("reminder edits = %+v, want one with the revoked footer", edits)
		}
		wantKeyboardActions(t, "refreshed reminder", edits[0].Params["reply_markup"], "lk", "bn")

		// The next reminder has no Revoke button and says the link was revoked, once.
		ageAlert(t, s.row.ID, 121*time.Minute)
		env.cycle()
		env.cycle()
		next := env.lastTo(s.staff.Id)
		if next.MessageID == card.MessageID || next.MessageID == reminder.MessageID {
			t.Fatal("no second reminder was posted to the Staff Group")
		}
		wantKeyboardActions(t, "second reminder", next.Params["reply_markup"], "lk", "bn")
		if text := fmt.Sprint(next.Params["text"]); strings.Count(text, staffMarker("lockdown_alert_link_revoked")) != 1 {
			t.Errorf("second reminder %q, want the revoked footer exactly once", text)
		}
		if got := len(env.alertEdits(s.staff.Id, next.MessageID)); got != 0 {
			t.Errorf("second reminder was edited %d times, want none: its footer was posted with it", got)
		}
	})

	t.Run("lift from a reminder", func(t *testing.T) {
		s := newReminderScene(t)
		env := s.env
		ageAlert(t, s.row.ID, 61*time.Minute)
		env.cycle()
		reminder := env.lastTo(s.staff.Id)
		liftData := lockdownButtonData(t, reminder.Params["reply_markup"], "lk")

		member := env.newOutsider()
		before := len(env.fake.sentTo(s.staff.Id))
		pressLockdownButton(env, member, s.staff, reminder.MessageID, liftData)
		if text, _ := env.lastAnswer(); !strings.Contains(text, staffMarker("lockdown_alert_not_admin")) {
			t.Errorf("refusal answer = %q, want %q", text, staffMarker("lockdown_alert_not_admin"))
		}
		if got := len(env.fake.sentTo(s.staff.Id)); got != before {
			t.Fatalf("%d messages in the Staff Group after a refused press, were %d: no card", got, before)
		}

		pressLockdownButton(env, env.admin, s.staff, reminder.MessageID, liftData)
		card := env.lastTo(s.staff.Id)
		if reply, ok := card.Params["reply_parameters"].(*gotgbot.ReplyParameters); !ok || reply.MessageId != reminder.MessageID {
			t.Fatalf("card reply_parameters = %+v, want a reply to the reminder %d", card.Params["reply_parameters"], reminder.MessageID)
		}
		pressLockdownButton(env, env.admin, s.staff, card.MessageID, lockdownButtonData(t, card.Params["reply_markup"], "lc"))

		if lifted := env.freshRow(s.row.ID); lifted.LiftedBy == nil || *lifted.LiftedBy != env.admin.Id {
			t.Fatalf("LiftedBy = %v, want %d", lifted.LiftedBy, env.admin.Id)
		}
		env.cycleUntilIdle()
		for _, copyRow := range env.alertRows() {
			if copyRow.ClosedAt == nil {
				t.Errorf("copy row %+v is not closed after the lift", copyRow)
			}
		}
		edits := env.alertEdits(s.staff.Id, reminder.MessageID)
		if len(edits) == 0 || !strings.Contains(fmt.Sprint(edits[len(edits)-1].Params["text"]), staffMarker("lockdown_alert_lifted_footer")) {
			t.Errorf("reminder edits = %+v, want the last to carry the lifted footer", edits)
		}
	})

	t.Run("ban from a reminder", func(t *testing.T) {
		s := newRevokeScene(t, revokeTestLink)
		env := s.env
		ageAlert(t, s.row.ID, 61*time.Minute)
		env.cycle()
		reminder := env.lastTo(s.staff.Id)

		a := banAlert{
			staffChat: s.staff,
			alert:     reminder,
			banData:   lockdownButtonData(t, reminder.Params["reply_markup"], "bn"),
		}
		card := env.askBan(a, env.admin)
		pressLockdownButton(env, env.admin, s.staff, card.msg.MessageID, card.confirm)
		env.cycleUntilIdle()

		if runs := env.banRuns(); len(runs) != 1 || runs[0].State != models.BanRunStateDone || runs[0].CardMsgID != card.msg.MessageID {
			t.Fatalf("ban runs = %+v, want one done run with the reminder's card", runs)
		}
		for _, row := range env.burstRows() {
			if row.State != models.BurstStateBanned {
				t.Errorf("burst row %+v, want every account banned by the run", row)
			}
		}
	})
}

func TestLockdownReminderDestinations(t *testing.T) {
	t.Run("unlinked", func(t *testing.T) {
		s := newReminderScene(t)
		env := s.env
		link, err := staff.GetLinkOfGroupFresh(env.chat.Id)
		if err != nil || link == nil {
			t.Fatalf("GetLinkOfGroupFresh = %v, %v, want the link", link, err)
		}
		if deleted, err := staff.DeleteLink(link.ID); err != nil || !deleted {
			t.Fatalf("DeleteLink = %v, %v, want true", deleted, err)
		}
		ageAlert(t, s.row.ID, 61*time.Minute)

		env.cycle()
		env.cycle()

		if got := len(env.fake.sentTo(s.staff.Id)); got != 1 {
			t.Errorf("%d messages in the Staff Group the group was unlinked from, want only the old alert", got)
		}
		if got := len(env.fake.sentTo(s.channel)); got != 2 {
			t.Errorf("%d messages in the log channel, want the alert and one reminder", got)
		}
		if rows := env.reminderRows(); len(rows) != 1 || rows[0].Dest != models.AlertDestLog {
			t.Errorf("reminder rows = %+v, want only the log copy", rows)
		}
	})

	t.Run("linked later", func(t *testing.T) {
		env := newLockdownEnv(t)
		staffChat := newLockdownStaffGroup(env)
		channel := uniqueModuleChatID()
		setStaffLogChannel(t, env.chat.Id, channel)
		row := env.lockAsAdmin("raid")
		env.cycle()
		linkLockdownGroupTo(env, staffChat)
		ageAlert(t, row.ID, 61*time.Minute)

		env.cycle()
		env.cycle()

		if got := len(env.fake.sentTo(staffChat.Id)); got != 0 {
			t.Errorf("%d messages reached a Staff Group linked after the alert, want none", got)
		}
		if got := len(env.fake.sentTo(channel)); got != 2 {
			t.Errorf("%d messages in the log channel, want the alert and one reminder", got)
		}
		if rows := env.reminderRows(); len(rows) != 1 || rows[0].Dest != models.AlertDestLog {
			t.Errorf("reminder rows = %+v, want only the log copy", rows)
		}
	})

	t.Run("category off later", func(t *testing.T) {
		s := newReminderScene(t)
		env := s.env
		if err := logchannels.SetCategory(env.chat.Id, logchannels.CategoryAdmin, false); err != nil {
			t.Fatalf("turn the admin category off: %v", err)
		}
		ageAlert(t, s.row.ID, 61*time.Minute)

		env.cycle()
		env.cycle()

		if got := len(env.fake.sentTo(s.staff.Id)); got != 2 {
			t.Errorf("%d messages in the Staff Group, want the alert and one reminder", got)
		}
		if got := len(env.fake.sentTo(s.channel)); got != 1 {
			t.Errorf("%d messages in a log channel whose admin category is off, want only the old alert", got)
		}
		if rows := env.reminderRows(); len(rows) != 1 || rows[0].Dest != models.AlertDestStaff {
			t.Errorf("reminder rows = %+v, want only the staff copy", rows)
		}
	})
}
