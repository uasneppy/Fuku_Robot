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
	"github.com/divkix/Alita_Robot/alita/db/models"
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
