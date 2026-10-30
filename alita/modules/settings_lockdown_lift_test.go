//go:build testtools

package modules

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db/lockdown"
	"github.com/divkix/Alita_Robot/alita/db/models"
)

// settingsLiftTestButton returns the first button of keyboard whose label carries the marker
// of key, failing the test when there is none.
func settingsLiftTestButton(t *testing.T, keyboard [][]gotgbot.InlineKeyboardButton, key string) gotgbot.InlineKeyboardButton {
	t.Helper()
	button, ok := settingsButtonByMarker(keyboard, key)
	if !ok {
		t.Fatalf("keyboard %+v has no button labelled %s", keyboard, key)
	}
	return button
}

// settingsLiftTestHasButton reports whether keyboard has a button labelled with key.
func settingsLiftTestHasButton(keyboard [][]gotgbot.InlineKeyboardButton, key string) bool {
	_, ok := settingsButtonByMarker(keyboard, key)
	return ok
}

// settingsLiftTestAsk presses the Lift button of the page on msgID as opener and returns the
// text and keyboard the message was edited to.
func settingsLiftTestAsk(t *testing.T, env *settingsEnv, opener gotgbot.User, msgID int64) (string, [][]gotgbot.InlineKeyboardButton) {
	t.Helper()
	_, keyboard := env.lastEdit(env.chat.Id, msgID)
	lift := settingsLiftTestButton(t, keyboard, "lockdown_alert_btn_lift")
	env.pressRaw(env.chat, opener, msgID, lift.CallbackData)
	return env.lastEdit(env.chat.Id, msgID)
}

// settingsLiftTestRestores counts the setChatPermissions calls made for the env's chat so far.
func settingsLiftTestRestores(env *settingsEnv) int {
	return len(env.calls("setChatPermissions"))
}

func TestSettingsLockdownLift(t *testing.T) {
	env := newSettingsEnv(t)
	row := env.lockAsAdmin("raid")
	if row.ChatTitle == "" {
		t.Fatal("setup: the lockdown stored no chat title")
	}
	msgID := settingsLockdownTestOpen(t, env)
	owner := fmt.Sprint(env.admin.Id)

	_, keyboard := env.lastEdit(env.chat.Id, msgID)
	if len(keyboard) == 0 || len(keyboard[0]) != 1 || !strings.Contains(keyboard[0][0].Text, staffMarker("lockdown_alert_btn_lift")) {
		t.Fatalf("first content row = %+v, want the single Lift button", keyboard)
	}
	if fields := settingsButtonFields(t, keyboard[0][0]); fmt.Sprint(fields) != fmt.Sprint(map[string]string{"a": "ls", "u": owner}) {
		t.Errorf("Lift button fields = %v, want a=ls and the opener", fields)
	}

	restoresBefore := settingsLiftTestRestores(env)
	cardText, card := settingsLiftTestAsk(t, env, env.admin, msgID)
	for _, want := range []string{staffMarker("lockdown_alert_lift_ask"), row.ChatTitle} {
		if !strings.Contains(cardText, want) {
			t.Errorf("Lift card %q does not contain %q", cardText, want)
		}
	}
	if len(card) != 2 || len(card[0]) != 2 || len(card[1]) != 2 {
		t.Fatalf("Lift card keyboard = %+v, want [Yes No] and [Back Close]", card)
	}
	yes, no := card[0][0], card[0][1]
	if !strings.Contains(yes.Text, staffMarker("settings_btn_yes")) || !strings.Contains(no.Text, staffMarker("settings_btn_no")) {
		t.Errorf("card buttons = %q, %q, want Yes then No", yes.Text, no.Text)
	}
	yesFields := settingsButtonFields(t, yes)
	if yesFields["a"] != "ly" || yesFields["u"] != owner || !regexp.MustCompile(`^\d+$`).MatchString(yesFields["e"]) || len(yesFields) != 3 {
		t.Errorf("Yes fields = %v, want a=ly, the opener and a numeric expiry", yesFields)
	}
	if fields := settingsButtonFields(t, no); fmt.Sprint(fields) != fmt.Sprint(map[string]string{"a": "no", "p": "l", "u": owner}) {
		t.Errorf("No fields = %v, want a=no back to page l", fields)
	}
	if got := settingsLiftTestRestores(env); got != restoresBefore {
		t.Errorf("asking made %d restore calls, want none", got-restoresBefore)
	}
	if active, err := lockdown.GetActiveFresh(env.chat.Id); err != nil || active == nil {
		t.Fatalf("GetActiveFresh = %v, %v, want the lockdown still active after the ask", active, err)
	}

	// No goes back to the Lockdown page and lifts nothing.
	env.pressRaw(env.chat, env.admin, msgID, no.CallbackData)
	pageText, pageKeyboard := env.lastEdit(env.chat.Id, msgID)
	if !strings.Contains(pageText, staffMarker("lockdown_status_active")) || !settingsLiftTestHasButton(pageKeyboard, "lockdown_alert_btn_lift") {
		t.Errorf("after No the message is %q with keyboard %+v, want the Lockdown page with its Lift button", pageText, pageKeyboard)
	}
	if got := settingsLiftTestRestores(env); got != restoresBefore {
		t.Errorf("No made %d restore calls, want none", got-restoresBefore)
	}

	// Yes answers first, then lifts through the path /unlockdown uses.
	_, card = settingsLiftTestAsk(t, env, env.admin, msgID)
	yes = settingsLiftTestButton(t, card, "settings_btn_yes")
	var mu sync.Mutex
	answersAtRestore := -1
	env.fake.setOnSetPermissions(func(int64) {
		n := env.answerCount()
		mu.Lock()
		defer mu.Unlock()
		answersAtRestore = n
	})
	answersBefore := env.answerCount()
	env.pressRaw(env.chat, env.admin, msgID, yes.CallbackData)

	if got := env.answerCount() - answersBefore; got != 1 {
		t.Errorf("the Yes press was answered %d times, want exactly once", got)
	}
	mu.Lock()
	if answersAtRestore != answersBefore+1 {
		t.Errorf("answers at the restore call = %d, want %d: Yes answers before it lifts", answersAtRestore, answersBefore+1)
	}
	mu.Unlock()
	if got := settingsLiftTestRestores(env) - restoresBefore; got != 1 {
		t.Errorf("Yes made %d restore calls, want exactly the one liftLockdownRow makes", got)
	}
	if got := env.fake.chatPermsRaw(env.chat.Id); got != lockdownTestPrePermissions {
		t.Errorf("permissions after the lift = %q, want the stored pre-lockdown set", got)
	}
	lifted := env.freshRow(row.ID)
	if lifted.State != models.LockdownStateLifted && lifted.State != models.LockdownStateLifting {
		t.Errorf("State = %q, want lifting or lifted", lifted.State)
	}
	if lifted.LiftedBy == nil || *lifted.LiftedBy != env.admin.Id {
		t.Errorf("LiftedBy = %v, want the presser %d", lifted.LiftedBy, env.admin.Id)
	}
	if got := env.repliesWith(staffMarker("lockdown_lifted")); len(got) != 1 {
		t.Errorf("%d public lift notices reached the group, want exactly 1", len(got))
	}
	finalText, finalKeyboard := env.lastEdit(env.chat.Id, msgID)
	if !strings.Contains(finalText, staffMarker("lockdown_lifted")) {
		t.Errorf("redrawn page %q does not carry the lift result as a notice", finalText)
	}
	if settingsLiftTestHasButton(finalKeyboard, "lockdown_alert_btn_lift") {
		t.Errorf("the page still offers Lift after the lockdown was lifted: %+v", finalKeyboard)
	}
}

func TestSettingsLockdownLiftOutcomes(t *testing.T) {
	t.Run("no lockdown redraws with the not-active notice and no card", func(t *testing.T) {
		env := newSettingsEnv(t)
		msgID := settingsLockdownTestOpen(t, env)
		env.press(env.admin, msgID, env.groupFields(env.admin, "a", "ls"))

		text, keyboard := env.lastEdit(env.chat.Id, msgID)
		if !strings.Contains(text, staffMarker("lockdown_not_active")) {
			t.Errorf("page %q does not carry %s", text, staffMarker("lockdown_not_active"))
		}
		if settingsLiftTestHasButton(keyboard, "settings_btn_yes") {
			t.Errorf("a card was drawn without a lockdown: %+v", keyboard)
		}
		if got := settingsLiftTestRestores(env); got != 0 {
			t.Errorf("%d restore calls with no lockdown", got)
		}
	})

	t.Run("a refused restore keeps the lockdown and posts no notice", func(t *testing.T) {
		env := newSettingsEnv(t)
		env.lockAsAdmin("raid")
		msgID := settingsLockdownTestOpen(t, env)
		_, card := settingsLiftTestAsk(t, env, env.admin, msgID)
		yes := settingsLiftTestButton(t, card, "settings_btn_yes")
		env.fake.script("setChatPermissions", env.chat.Id,
			staffFakeError(400, "Bad Request: not enough rights to change chat permissions"))

		env.pressRaw(env.chat, env.admin, msgID, yes.CallbackData)

		text, keyboard := env.lastEdit(env.chat.Id, msgID)
		if !strings.Contains(text, staffMarker("lockdown_restore_failed")) || strings.Contains(text, "not enough rights") {
			t.Errorf("page %q, want the plain restore failure", text)
		}
		if active, err := lockdown.GetActiveFresh(env.chat.Id); err != nil || active == nil {
			t.Errorf("GetActiveFresh = %v, %v, want the lockdown still active", active, err)
		}
		if got := env.repliesWith(staffMarker("lockdown_lifted")); len(got) != 0 {
			t.Errorf("%d public lift notices after a failed restore, want none", len(got))
		}
		if !settingsLiftTestHasButton(keyboard, "lockdown_alert_btn_lift") {
			t.Errorf("a lockdown that is still active lost its Lift button: %+v", keyboard)
		}
	})

	t.Run("a failed record keeps the lockdown and posts no notice", func(t *testing.T) {
		env := newSettingsEnv(t)
		env.lockAsAdmin("raid")
		msgID := settingsLockdownTestOpen(t, env)
		_, card := settingsLiftTestAsk(t, env, env.admin, msgID)
		yes := settingsLiftTestButton(t, card, "settings_btn_yes")
		previous := lockdownBeginLift
		lockdownBeginLift = func(uint, int64, string, bool) (bool, error) { return false, errors.New("database is down") }
		t.Cleanup(func() { lockdownBeginLift = previous })

		env.pressRaw(env.chat, env.admin, msgID, yes.CallbackData)

		text, _ := env.lastEdit(env.chat.Id, msgID)
		if !strings.Contains(text, staffMarker("lockdown_lift_record_failed")) {
			t.Errorf("page %q does not carry %s", text, staffMarker("lockdown_lift_record_failed"))
		}
		if active, err := lockdown.GetActiveFresh(env.chat.Id); err != nil || active == nil {
			t.Errorf("GetActiveFresh = %v, %v, want the lockdown still active", active, err)
		}
		if got := env.repliesWith(staffMarker("lockdown_lifted")); len(got) != 0 {
			t.Errorf("%d public lift notices after a failed record, want none", len(got))
		}
	})

	t.Run("a lockdown being lifted shows no Lift button and a hand-built ask says so", func(t *testing.T) {
		env := newSettingsEnv(t)
		row := env.lockAsAdmin("raid")
		if began, err := lockdown.BeginLift(row.ID, env.admin.Id, "Ad<b>min", false); err != nil || !began {
			t.Fatalf("setup: BeginLift = %v, %v", began, err)
		}
		seedLockdownJoiner(t, row.ID, env.chat.Id, 9701, models.JoinerStateBanned)
		restoresBefore := settingsLiftTestRestores(env)
		msgID := settingsLockdownTestOpen(t, env)

		_, keyboard := env.lastEdit(env.chat.Id, msgID)
		if settingsLiftTestHasButton(keyboard, "lockdown_alert_btn_lift") {
			t.Errorf("a lockdown being lifted offers Lift: %+v", keyboard)
		}
		env.press(env.admin, msgID, env.groupFields(env.admin, "a", "ls"))

		text, keyboard := env.lastEdit(env.chat.Id, msgID)
		if !strings.Contains(text, staffMarker("lockdown_lift_in_progress")) {
			t.Errorf("page %q does not carry %s", text, staffMarker("lockdown_lift_in_progress"))
		}
		if settingsLiftTestHasButton(keyboard, "settings_btn_yes") {
			t.Errorf("a card was drawn for a lockdown being lifted: %+v", keyboard)
		}
		if got := settingsLiftTestRestores(env); got != restoresBefore {
			t.Errorf("%d restore calls for a lockdown already being lifted", got-restoresBefore)
		}
	})
}
