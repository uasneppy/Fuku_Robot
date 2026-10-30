//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"html"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/lockdown"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/i18n"
)

// settingsLockdownTestOpenAs opens the menu as opener, presses the Lockdown button and
// returns the menu message ID.
func settingsLockdownTestOpenAs(t *testing.T, env *settingsEnv, opener gotgbot.User) int64 {
	t.Helper()
	msgID := env.openMenu(opener)
	_, keyboard := env.lastEdit(env.chat.Id, msgID)
	button, ok := settingsPageButton(t, keyboard, settingsPageLockdown)
	if !ok {
		t.Fatal("the Main page has no Lockdown button")
	}
	env.pressRaw(env.chat, opener, msgID, button.CallbackData)
	return msgID
}

// settingsLockdownTestOpen opens the Lockdown page as the env's administrator.
func settingsLockdownTestOpen(t *testing.T, env *settingsEnv) int64 {
	t.Helper()
	return settingsLockdownTestOpenAs(t, env, env.admin)
}

// settingsLockdownTestRefresh presses the value button of the page (an op on the page), which
// redraws it from fresh reads.
func settingsLockdownTestRefresh(env *settingsEnv, opener gotgbot.User, msgID int64) {
	env.press(opener, msgID, env.groupFields(opener, "a", settingsActOpen, "p", settingsPageLockdown))
}

// settingsLockdownTestStart records a lockdown of the env's chat the way /lockdown leaves
// it. A confirmed row has locked_at set.
func settingsLockdownTestStart(t *testing.T, env *settingsEnv, confirm bool, mutate func(*models.ChatLockdown)) *models.ChatLockdown {
	t.Helper()
	row := &models.ChatLockdown{
		ChatID:            env.chat.Id,
		TriggerKind:       models.LockdownTriggerManual,
		StartedBy:         env.admin.Id,
		StartedByName:     "Ad<b>min",
		PrePermissions:    lockdownTestPrePermissions,
		LockedPermissions: lockdownLockedPermissions,
	}
	if mutate != nil {
		mutate(row)
	}
	if started, err := lockdown.Start(row); err != nil || !started {
		t.Fatalf("setup: Start = %v, %v", started, err)
	}
	if confirm {
		if ok, err := lockdown.ConfirmLocked(row.ID); err != nil || !ok {
			t.Fatalf("setup: ConfirmLocked = %v, %v", ok, err)
		}
	}
	fresh, err := lockdown.GetFresh(row.ID)
	if err != nil || fresh == nil {
		t.Fatalf("setup: GetFresh = %v, %v", fresh, err)
	}
	return fresh
}

// settingsLockdownTestRow reads the chat_raid_detection row of chatID past every cache.
func settingsLockdownTestRow(t *testing.T, chatID int64) (models.ChatRaidDetection, bool) {
	t.Helper()
	var rows []models.ChatRaidDetection
	if err := db.DB.Where("chat_id = ?", chatID).Find(&rows).Error; err != nil {
		t.Fatalf("read the raid detection row: %v", err)
	}
	if len(rows) == 0 {
		return models.ChatRaidDetection{}, false
	}
	return rows[0], true
}

// settingsLockdownTestPageText is the text of the page last drawn on msgID.
func settingsLockdownTestPageText(env *settingsEnv, msgID int64) string {
	env.t.Helper()
	text, _ := env.lastEdit(env.chat.Id, msgID)
	return text
}

// settingsLockdownTestOn is the state word the page shows for a switch that is on.
func settingsLockdownTestOn() string { return "✅ " + staffMarker("common_status_on") }

// settingsLockdownTestOff is the state word the page shows for a switch that is off.
func settingsLockdownTestOff() string { return "❌ " + staffMarker("common_status_off") }

// settingsLockdownTestRows checks the shape of the content rows of the page (a toggle row and a
// stepper row per trigger, then the navigation row) and returns the three toggle buttons and
// the three stepper rows.
func settingsLockdownTestRows(t *testing.T, keyboard [][]gotgbot.InlineKeyboardButton) ([]gotgbot.InlineKeyboardButton, [][]gotgbot.InlineKeyboardButton) {
	t.Helper()
	if len(keyboard) != 7 {
		t.Fatalf("Lockdown page has %d keyboard rows, want 3 toggle rows, 3 stepper rows and the navigation row", len(keyboard))
	}
	var toggles []gotgbot.InlineKeyboardButton
	var steppers [][]gotgbot.InlineKeyboardButton
	for i := 0; i < 3; i++ {
		if len(keyboard[2*i]) != 1 {
			t.Fatalf("toggle row %d has %d buttons, want 1", i, len(keyboard[2*i]))
		}
		if len(keyboard[2*i+1]) != 5 {
			t.Fatalf("stepper row %d has %d buttons, want 5", i, len(keyboard[2*i+1]))
		}
		toggles = append(toggles, keyboard[2*i][0])
		steppers = append(steppers, keyboard[2*i+1])
	}
	nav := keyboard[6]
	if len(nav) != 2 {
		t.Fatalf("navigation row has %d buttons, want Back and Close", len(nav))
	}
	return toggles, steppers
}

func TestSettingsLockdownPage(t *testing.T) {
	t.Run("no lockdown shows the triggers", func(t *testing.T) {
		env := newSettingsEnv(t)
		withAIKeys(t, "", "")
		msgID := settingsLockdownTestOpen(t, env)

		text, keyboard := env.lastEdit(env.chat.Id, msgID)
		for _, want := range []string{
			staffMarker("settings_header") + " " + staffMarker("settings_page_lockdown"),
			staffMarker("settings_lockdown_none"),
			staffMarker("settings_lockdown_triggers"),
			staffMarker("settings_trigger_joins") + " " + settingsLockdownTestOn() + " 15",
			staffMarker("settings_trigger_flood") + " " + settingsLockdownTestOn() + " 30",
			staffMarker("settings_trigger_ai") + " " + settingsLockdownTestOn() + " 5",
			staffMarker("lockdown_auto_ai_unavailable"),
			staffMarker("staff_panel_updated"),
		} {
			if !strings.Contains(text, want) {
				t.Errorf("Lockdown page %q does not contain %q", text, want)
			}
		}

		toggles, steppers := settingsLockdownTestRows(t, keyboard)
		owner := fmt.Sprint(env.admin.Id)
		for i, tc := range []struct {
			control string
			value   int
			label   string
		}{
			{settingsTriggerJoins, 15, "settings_btn_trigger_joins"},
			{settingsTriggerFlood, 30, "settings_btn_trigger_flood"},
			{settingsTriggerAI, 5, "settings_btn_trigger_ai"},
		} {
			if !strings.Contains(toggles[i].Text, staffMarker(tc.label)) {
				t.Errorf("toggle %d label %q is not %s", i, toggles[i].Text, tc.label)
			}
			fields := settingsButtonFields(t, toggles[i])
			want := map[string]string{"a": "tg", "p": "l", "t": tc.control, "v": "0", "u": owner}
			if fmt.Sprint(fields) != fmt.Sprint(want) {
				t.Errorf("toggle %q fields = %v, want %v", tc.control, fields, want)
			}

			targets := []string{
				fmt.Sprint(max(tc.value-10, 3)), fmt.Sprint(max(tc.value-1, 3)), "", fmt.Sprint(tc.value + 1), fmt.Sprint(tc.value + 10),
			}
			for j, button := range steppers[i] {
				fields := settingsButtonFields(t, button)
				if j == 2 {
					if button.Text != fmt.Sprint(tc.value) || fields["a"] != "op" || fields["p"] != "l" || len(fields) != 3 {
						t.Errorf("value button of %q = %q %v, want the value %d redrawing page l", tc.control, button.Text, fields, tc.value)
					}
					continue
				}
				if fields["a"] != "st" || fields["p"] != "l" || fields["t"] != tc.control || fields["v"] != targets[j] {
					t.Errorf("stepper %q button %d = %v, want st on page l, control %s, v %s", tc.control, j, fields, tc.control, targets[j])
				}
			}
		}
		if fields := settingsButtonFields(t, keyboard[6][0]); fields["a"] != "bk" {
			t.Errorf("first navigation button fields = %v, want Back", fields)
		}
		if fields := settingsButtonFields(t, keyboard[6][1]); fields["a"] != "cl" {
			t.Errorf("second navigation button fields = %v, want Close", fields)
		}
	})

	t.Run("the AI notice follows the provider setup", func(t *testing.T) {
		env := newSettingsEnv(t)
		withAIKeys(t, "typesafe-test-key", "")
		msgID := settingsLockdownTestOpen(t, env)
		if text := settingsLockdownTestPageText(env, msgID); strings.Contains(text, staffMarker("lockdown_auto_ai_unavailable")) {
			t.Errorf("page %q shows the AI notice although a provider is set up", text)
		}
	})

	t.Run("an active manual lockdown shows its reason cut at 200 runes", func(t *testing.T) {
		env := newSettingsEnv(t)
		row := settingsLockdownTestStart(t, env, true, func(r *models.ChatLockdown) {
			r.Reason = "<" + strings.Repeat("a", 299)
		})
		msgID := settingsLockdownTestOpen(t, env)

		text := settingsLockdownTestPageText(env, msgID)
		wantReason := html.EscapeString("<"+strings.Repeat("a", 199)) + "…"
		for _, want := range []string{
			staffMarker("lockdown_status_active"),
			lockdownTime(*row.LockedAt),
			"Ad&lt;b&gt;min",
			staffMarker("lockdown_reason_line"),
			wantReason,
		} {
			if !strings.Contains(text, want) {
				t.Errorf("page %q does not contain %q", text, want)
			}
		}
		if strings.Contains(text, strings.Repeat("a", 200)) {
			t.Errorf("page %q shows more than 200 runes of the reason", text)
		}
		if strings.Contains(text, staffMarker("lockdown_status_unconfirmed")) || strings.Contains(text, staffMarker("settings_lockdown_none")) {
			t.Errorf("page %q shows a state that does not apply", text)
		}
	})

	t.Run("an automatic lockdown names its trigger and has no reason line", func(t *testing.T) {
		env := newSettingsEnv(t)
		settingsLockdownTestStart(t, env, true, func(r *models.ChatLockdown) {
			r.TriggerKind = models.LockdownTriggerJoinSurge
			r.TriggerCount = 18
			r.TriggerWindow = 60
			r.StartedBy = env.admin.Id
			r.Reason = "never shown"
		})
		msgID := settingsLockdownTestOpen(t, env)

		text := settingsLockdownTestPageText(env, msgID)
		for _, want := range []string{staffMarker("lockdown_status_active_auto"), staffMarker("lockdown_trigger_join_surge")} {
			if !strings.Contains(text, want) {
				t.Errorf("page %q does not contain %q", text, want)
			}
		}
		for _, unwanted := range []string{staffMarker("lockdown_reason_line"), "never shown"} {
			if strings.Contains(text, unwanted) {
				t.Errorf("page %q contains %q, want it absent", text, unwanted)
			}
		}
	})

	t.Run("an unconfirmed lockdown says so", func(t *testing.T) {
		env := newSettingsEnv(t)
		row := settingsLockdownTestStart(t, env, false, nil)
		msgID := settingsLockdownTestOpen(t, env)

		text := settingsLockdownTestPageText(env, msgID)
		for _, want := range []string{
			staffMarker("lockdown_status_active"), lockdownTime(row.CreatedAt), staffMarker("lockdown_status_unconfirmed"),
		} {
			if !strings.Contains(text, want) {
				t.Errorf("page %q does not contain %q", text, want)
			}
		}
	})

	t.Run("a lockdown being lifted shows the unbanning progress", func(t *testing.T) {
		env := newSettingsEnv(t)
		row := settingsLockdownTestStart(t, env, true, nil)
		if began, err := lockdown.BeginLift(row.ID, env.admin.Id, "Ad<b>min", false); err != nil || !began {
			t.Fatalf("setup: BeginLift = %v, %v", began, err)
		}
		seedLockdownJoiner(t, row.ID, env.chat.Id, 9301, models.JoinerStateUnbanned)
		seedLockdownJoiner(t, row.ID, env.chat.Id, 9302, models.JoinerStateBanned)
		seedLockdownJoiner(t, row.ID, env.chat.Id, 9303, models.JoinerStateKept)
		msgID := settingsLockdownTestOpen(t, env)

		text, keyboard := env.lastEdit(env.chat.Id, msgID)
		if !regexp.MustCompile(`@@lockdown-status-lifting@@ .* 2 3`).MatchString(text) {
			t.Errorf("page %q does not show the lifting marker with 2 of 3 done", text)
		}
		if strings.Contains(text, staffMarker("lockdown_status_active")) {
			t.Errorf("page %q still shows the active state", text)
		}
		settingsLockdownTestRows(t, keyboard)
	})

	t.Run("Main shows the lockdown state", func(t *testing.T) {
		env := newSettingsEnv(t)
		mainButton := func() string {
			msgID := env.openMenu(env.admin)
			_, keyboard := env.lastEdit(env.chat.Id, msgID)
			button, ok := settingsPageButton(t, keyboard, settingsPageLockdown)
			if !ok {
				t.Fatal("the Main page has no Lockdown button")
			}
			return button.Text
		}
		if label := mainButton(); !strings.Contains(label, staffMarker("settings_state_lock_none")) {
			t.Errorf("Main shows %q with no lockdown, want settings_state_lock_none", label)
		}
		row := settingsLockdownTestStart(t, env, true, nil)
		if label := mainButton(); !strings.Contains(label, staffMarker("settings_state_lock_active")) {
			t.Errorf("Main shows %q with an active lockdown, want settings_state_lock_active", label)
		}
		if began, err := lockdown.BeginLift(row.ID, env.admin.Id, "Ad<b>min", false); err != nil || !began {
			t.Fatalf("setup: BeginLift = %v, %v", began, err)
		}
		if label := mainButton(); !strings.Contains(label, staffMarker("settings_state_lock_lifting")) {
			t.Errorf("Main shows %q with a lockdown being lifted, want settings_state_lock_lifting", label)
		}
	})
}

func TestSettingsLockdownBasicGroup(t *testing.T) {
	env := newSettingsEnv(t)
	env.chat.Type = "group"
	msgID := settingsLockdownTestOpen(t, env)

	text, keyboard := env.lastEdit(env.chat.Id, msgID)
	if !strings.Contains(text, staffMarker("settings_lockdown_basic")) {
		t.Errorf("page %q does not say Lockdown needs a supergroup", text)
	}
	for _, unwanted := range []string{staffMarker("settings_lockdown_triggers"), staffMarker("settings_trigger_joins")} {
		if strings.Contains(text, unwanted) {
			t.Errorf("page %q of a basic group shows %q", text, unwanted)
		}
	}
	if len(keyboard) != 1 || len(keyboard[0]) != 2 {
		t.Fatalf("basic group keyboard = %+v, want only the Back and Close row", keyboard)
	}
	if a, b := settingsButtonFields(t, keyboard[0][0])["a"], settingsButtonFields(t, keyboard[0][1])["a"]; a != "bk" || b != "cl" {
		t.Errorf("navigation actions = %q, %q, want bk and cl", a, b)
	}

	before := env.answerCount()
	env.press(env.admin, msgID, env.groupFields(env.admin, "a", "tg", "p", "l", "t", "j", "v", "0"))
	if got := env.answerCount() - before; got != 1 {
		t.Errorf("the forged press answered %d times, want once", got)
	}
	text, _ = env.lastEdit(env.chat.Id, msgID)
	if !strings.Contains(text, staffMarker("lockdown_basic_group")) {
		t.Errorf("the forged press redrew %q, want the command's lockdown_basic_group notice", text)
	}
	if row, ok := settingsLockdownTestRow(t, env.chat.Id); ok {
		t.Errorf("a chat_raid_detection row exists (%+v) after a forged press on a basic group", row)
	}
}

func TestSettingsLockdownUnreadable(t *testing.T) {
	t.Run("the lockdown state cannot be read", func(t *testing.T) {
		env := newSettingsEnv(t)
		previous := lockdownCurrentLookup
		lockdownCurrentLookup = func(int64) (*models.ChatLockdown, error) { return nil, errors.New("database is down") }
		restored := false
		restore := func() {
			if !restored {
				lockdownCurrentLookup = previous
				restored = true
			}
		}
		t.Cleanup(restore)

		mainID := env.openMenu(env.admin)
		_, mainKeyboard := env.lastEdit(env.chat.Id, mainID)
		mainButton, ok := settingsPageButton(t, mainKeyboard, settingsPageLockdown)
		if !ok {
			t.Fatal("the Main page has no Lockdown button")
		}
		if !strings.HasSuffix(mainButton.Text, " · ❔") {
			t.Errorf("Main shows %q, want the Lockdown button to end with the unknown mark", mainButton.Text)
		}

		env.pressRaw(env.chat, env.admin, mainID, mainButton.CallbackData)
		text, keyboard := env.lastEdit(env.chat.Id, mainID)
		if !strings.Contains(text, staffMarker("settings_lockdown_unreadable")) {
			t.Errorf("page %q does not say the state could not be read", text)
		}
		for _, unwanted := range []string{staffMarker("settings_lockdown_none"), staffMarker("settings_trigger_joins")} {
			if strings.Contains(text, unwanted) {
				t.Errorf("page %q shows %q instead of guessing nothing", text, unwanted)
			}
		}
		if len(keyboard) != 2 || len(keyboard[0]) != 1 || len(keyboard[1]) != 2 {
			t.Fatalf("keyboard = %+v, want Refresh then Back and Close", keyboard)
		}
		refresh := keyboard[0][0]
		if !strings.Contains(refresh.Text, staffMarker("staff_panel_refresh_button")) {
			t.Errorf("first button %q is not Refresh", refresh.Text)
		}
		fields := settingsButtonFields(t, refresh)
		if fields["a"] != "op" || fields["p"] != "l" || len(fields) != 3 {
			t.Errorf("Refresh fields = %v, want {a: op, p: l} and the owner", fields)
		}

		restore()
		env.pressRaw(env.chat, env.admin, mainID, refresh.CallbackData)
		if text := settingsLockdownTestPageText(env, mainID); !strings.Contains(text, staffMarker("settings_lockdown_triggers")) {
			t.Errorf("after Refresh the page %q does not show the triggers", text)
		}
	})

	t.Run("the trigger settings cannot be read", func(t *testing.T) {
		env := newSettingsEnv(t)
		msgID := settingsLockdownTestOpen(t, env)
		previous := raidSettingsLookup
		raidSettingsLookup = func(context.Context, int64) (*models.ChatRaidDetection, error) {
			return nil, errors.New("database is down")
		}
		t.Cleanup(func() { raidSettingsLookup = previous })

		settingsLockdownTestRefresh(env, env.admin, msgID)
		text, keyboard := env.lastEdit(env.chat.Id, msgID)
		if !strings.Contains(text, staffMarker("settings_lockdown_unreadable")) || strings.Contains(text, staffMarker("settings_trigger_joins")) {
			t.Errorf("page %q, want the unreadable body with no trigger lines", text)
		}
		if len(keyboard) != 2 {
			t.Errorf("keyboard has %d rows, want Refresh and the navigation row", len(keyboard))
		}
	})
}

func TestSettingsLockdownTriggers(t *testing.T) {
	// pressStep presses the stepper button at index of the trigger's stepper row on msgID.
	pressStep := func(t *testing.T, env *settingsEnv, msgID int64, trigger, index int) {
		t.Helper()
		_, keyboard := env.lastEdit(env.chat.Id, msgID)
		_, steppers := settingsLockdownTestRows(t, keyboard)
		env.pressRaw(env.chat, env.admin, msgID, steppers[trigger][index].CallbackData)
	}
	pressToggle := func(t *testing.T, env *settingsEnv, msgID int64, trigger int) {
		t.Helper()
		_, keyboard := env.lastEdit(env.chat.Id, msgID)
		toggles, _ := settingsLockdownTestRows(t, keyboard)
		env.pressRaw(env.chat, env.admin, msgID, toggles[trigger].CallbackData)
	}

	t.Run("a toggle writes and the redraw offers the opposite", func(t *testing.T) {
		env := newSettingsEnv(t)
		msgID := settingsLockdownTestOpen(t, env)
		before := env.answerCount()

		pressToggle(t, env, msgID, 0)

		env.wantAutoRow(false, 15, true, 30)
		text, keyboard := env.lastEdit(env.chat.Id, msgID)
		if !strings.Contains(text, staffMarker("settings_trigger_joins")+" "+settingsLockdownTestOff()+" 15") {
			t.Errorf("page %q does not show join surge off at 15", text)
		}
		toggles, _ := settingsLockdownTestRows(t, keyboard)
		if v := settingsButtonFields(t, toggles[0])["v"]; v != "1" {
			t.Errorf("the redrawn join surge toggle sends v %q, want 1", v)
		}
		if got := env.answerCount() - before; got != 1 {
			t.Errorf("the toggle press answered %d times, want once", got)
		}
	})

	t.Run("a step also turns its trigger on", func(t *testing.T) {
		env := newSettingsEnv(t)
		msgID := settingsLockdownTestOpen(t, env)
		pressToggle(t, env, msgID, 1)
		env.wantAutoRow(true, 15, false, 30)

		pressStep(t, env, msgID, 1, 3)

		env.wantAutoRow(true, 15, true, 31)
		text := settingsLockdownTestPageText(env, msgID)
		if !strings.Contains(text, staffMarker("settings_trigger_flood")+" "+settingsLockdownTestOn()+" 31") {
			t.Errorf("page %q does not show flood on at 31", text)
		}
	})

	t.Run("AI plus ten from 495 stores 500", func(t *testing.T) {
		env := newSettingsEnv(t)
		env.send(env.admin, "/autolockdown ai 495")
		msgID := settingsLockdownTestOpen(t, env)

		pressStep(t, env, msgID, 2, 4)

		env.wantAutoAIRow(true, 500)
	})

	t.Run("a step at the top bound answers a toast and writes nothing", func(t *testing.T) {
		env := newSettingsEnv(t)
		env.send(env.admin, "/autolockdown ai 500")
		msgID := settingsLockdownTestOpen(t, env)
		_, keyboard := env.lastEdit(env.chat.Id, msgID)
		_, steppers := settingsLockdownTestRows(t, keyboard)
		before, _ := settingsLockdownTestRow(t, env.chat.Id)

		for _, index := range []int{3, 4} {
			button := steppers[2][index]
			if button.Text != "·" || settingsButtonFields(t, button)["v"] != "500" {
				t.Fatalf("stepper button %d = %q %v, want · sending 500", index, button.Text, settingsButtonFields(t, button))
			}
			edits := len(env.edits(env.chat.Id, msgID))
			env.pressRaw(env.chat, env.admin, msgID, button.CallbackData)
			answer, alert := env.lastAnswer()
			if !strings.Contains(answer, staffMarker("settings_max_value")) || !strings.Contains(answer, "500") || alert {
				t.Errorf("answer = %q (alert %v), want the max-value toast with 500", answer, alert)
			}
			if got := len(env.edits(env.chat.Id, msgID)); got != edits {
				t.Errorf("a step at the bound edited the page %d more time(s)", got-edits)
			}
		}
		after, _ := settingsLockdownTestRow(t, env.chat.Id)
		if !after.UpdatedAt.Equal(before.UpdatedAt) || after.AIThreshold != 500 {
			t.Errorf("row changed from %+v to %+v at the bound", before, after)
		}
	})

	t.Run("a step at the bottom bound answers a toast and writes nothing", func(t *testing.T) {
		env := newSettingsEnv(t)
		env.send(env.admin, "/autolockdown joins 3")
		msgID := settingsLockdownTestOpen(t, env)
		_, keyboard := env.lastEdit(env.chat.Id, msgID)
		_, steppers := settingsLockdownTestRows(t, keyboard)
		before, _ := settingsLockdownTestRow(t, env.chat.Id)

		for _, index := range []int{0, 1} {
			button := steppers[0][index]
			if button.Text != "·" || settingsButtonFields(t, button)["v"] != "3" {
				t.Fatalf("stepper button %d = %q %v, want · sending 3", index, button.Text, settingsButtonFields(t, button))
			}
			env.pressRaw(env.chat, env.admin, msgID, button.CallbackData)
			answer, _ := env.lastAnswer()
			if !strings.Contains(answer, staffMarker("settings_min_value")) || !strings.Contains(answer, "3") {
				t.Errorf("answer = %q, want the min-value toast with 3", answer)
			}
		}
		after, _ := settingsLockdownTestRow(t, env.chat.Id)
		if !after.UpdatedAt.Equal(before.UpdatedAt) || after.JoinThreshold != 3 {
			t.Errorf("row changed from %+v to %+v at the bound", before, after)
		}
	})

	t.Run("a failed save redraws with the command's own text", func(t *testing.T) {
		env := newSettingsEnv(t)
		msgID := settingsLockdownTestOpen(t, env)
		previous := autolockdownWrite
		autolockdownWrite = func(int64, autoLockdownChange) error { return errors.New("database is down") }
		t.Cleanup(func() { autolockdownWrite = previous })

		pressToggle(t, env, msgID, 0)

		text := settingsLockdownTestPageText(env, msgID)
		if !strings.Contains(text, staffMarker("lockdown_auto_save_failed")) {
			t.Errorf("page %q does not carry lockdown_auto_save_failed", text)
		}
		env.wantNoAutoRow()
	})

	t.Run("a malformed value is an invalid request and writes nothing", func(t *testing.T) {
		env := newSettingsEnv(t)
		msgID := settingsLockdownTestOpen(t, env)
		for _, fields := range []map[string]string{
			{"a": "tg", "p": "l", "t": "j", "v": "2"},
			{"a": "st", "p": "l", "t": "j", "v": "x"},
			{"a": "st", "p": "l", "t": "j", "v": "1000"},
			{"a": "st", "p": "l", "t": "z", "v": "10"},
		} {
			fields["u"] = fmt.Sprint(env.admin.Id)
			env.press(env.admin, msgID, fields)
			if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("common_callback_invalid_request")) {
				t.Errorf("press %v answered %q (alert %v), want the invalid request alert", fields, answer, alert)
			}
		}
		env.wantNoAutoRow()
	})
}

func TestSettingsParityLockdownTriggers(t *testing.T) {
	env := newSettingsEnv(t)
	msgID := settingsLockdownTestOpen(t, env)

	// (1) the command writes, the page shows it.
	env.send(env.admin, "/autolockdown joins 20")
	settingsLockdownTestRefresh(env, env.admin, msgID)
	text := settingsLockdownTestPageText(env, msgID)
	if !strings.Contains(text, staffMarker("settings_trigger_joins")+" "+settingsLockdownTestOn()+" 20") {
		t.Errorf("after /autolockdown joins 20 the page shows %q, want join surge on at 20", text)
	}
	env.wantAutoRow(true, 20, true, 30)

	// (2) the menu writes, the command shows it.
	_, keyboard := env.lastEdit(env.chat.Id, msgID)
	toggles, _ := settingsLockdownTestRows(t, keyboard)
	env.pressRaw(env.chat, env.admin, msgID, toggles[1].CallbackData)
	env.send(env.admin, "/autolockdown")
	env.wantReplyHas(staffMarker("lockdown_auto_flood_off") + " 30")
	env.wantAutoRow(true, 20, false, 30)

	// (3) the AI trigger agrees too.
	env.send(env.admin, "/autolockdown ai 7")
	settingsLockdownTestRefresh(env, env.admin, msgID)
	text = settingsLockdownTestPageText(env, msgID)
	if !strings.Contains(text, staffMarker("settings_trigger_ai")+" "+settingsLockdownTestOn()+" 7") {
		t.Errorf("after /autolockdown ai 7 the page shows %q, want AI spam on at 7", text)
	}
	env.wantAutoAIRow(true, 7)

	// (4) one row holds what both sides show.
	row, ok := settingsLockdownTestRow(t, env.chat.Id)
	if !ok || row.JoinThreshold != 20 || row.FloodEnabled || row.AIThreshold != 7 || !row.AIEnabled {
		t.Errorf("stored row = %+v (row %v), want joins 20, flood off, ai 7 on", row, ok)
	}
}

func TestSettingsDriftLockdownTriggers(t *testing.T) {
	for _, n := range []int{2, 3, 500, 501} {
		t.Run(fmt.Sprintf("N=%d", n), func(t *testing.T) {
			env := newSettingsEnv(t)
			commandChat := env.chat
			menuChat := env.addChat()
			valid := n >= 3 && n <= 500

			env.send(env.admin, fmt.Sprintf("/autolockdown joins %d", n))
			if !valid {
				env.wantReplyHas(staffMarker("lockdown_auto_range"))
			}

			fields := map[string]string{"a": "st", "u": fmt.Sprint(env.admin.Id), "p": "l", "t": "j", "v": fmt.Sprint(n)}
			data := encodeCallbackData(settingsCallbackNamespace, fields)
			if data == "" {
				t.Fatalf("hand-built press %v did not encode", fields)
			}
			const menuMsgID = 4343
			env.pressRaw(menuChat, env.admin, menuMsgID, data)

			commandRow, commandHas := settingsLockdownTestRow(t, commandChat.Id)
			menuRow, menuHas := settingsLockdownTestRow(t, menuChat.Id)
			if commandHas != menuHas || commandRow.JoinThreshold != menuRow.JoinThreshold || commandRow.JoinEnabled != menuRow.JoinEnabled {
				t.Fatalf("command stored (%+v, row %v), menu stored (%+v, row %v): they disagree", commandRow, commandHas, menuRow, menuHas)
			}
			if valid && (!menuHas || menuRow.JoinThreshold != n || !menuRow.JoinEnabled) {
				t.Errorf("both sides stored (%+v, row %v), want joins on at %d", menuRow, menuHas, n)
			}
			if !valid {
				if menuHas {
					t.Errorf("an invalid value %d was stored (%+v)", n, menuRow)
				}
				text, _ := env.lastEdit(menuChat.Id, menuMsgID)
				if !strings.Contains(text, staffMarker("lockdown_auto_range")) {
					t.Errorf("the menu redrew %q, want a notice with the command's own lockdown_auto_range", text)
				}
			}
		})
	}
}

func TestSettingsAuthorityParityLockdownTriggers(t *testing.T) {
	t.Run("an administrator without can_restrict_members views but cannot change", func(t *testing.T) {
		env := newSettingsEnv(t)
		helper := env.newOutsider()
		env.fake.setMember(env.chat.Id, helper.Id, staffFakeMember{
			Status:             gotgbot.ChatMemberStatusAdministrator,
			CanRestrictMembers: false,
		})
		msgID := settingsLockdownTestOpenAs(t, env, helper)

		text, keyboard := env.lastEdit(env.chat.Id, msgID)
		if !strings.Contains(text, staffMarker("settings_lockdown_triggers")) {
			t.Fatalf("the administrator without the right cannot view the page: %q", text)
		}
		toggles, steppers := settingsLockdownTestRows(t, keyboard)

		for name, button := range map[string]gotgbot.InlineKeyboardButton{"toggle": toggles[0], "step": steppers[1][3]} {
			edits := len(env.edits(env.chat.Id, msgID))
			before := env.answerCount()
			env.pressRaw(env.chat, helper, msgID, button.CallbackData)
			if got := env.answerCount() - before; got != 1 {
				t.Errorf("%s press answered %d times, want once", name, got)
			}
			if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("settings_no_restrict")) {
				t.Errorf("%s press answered %q (alert %v), want the settings_no_restrict alert", name, answer, alert)
			}
			if got := len(env.edits(env.chat.Id, msgID)); got != edits {
				t.Errorf("%s press edited the page", name)
			}
		}
		env.wantNoAutoRow()

		env.send(helper, "/autolockdown joins 20")
		env.wantReplyHas(staffMarker("chat_status_restrict_cmd_error"))
		env.wantNoAutoRow()
	})

	t.Run("the creator and an administrator with the right succeed both ways", func(t *testing.T) {
		env := newSettingsEnv(t)
		owner := env.newOutsider()
		env.fake.setMember(env.chat.Id, owner.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusCreator})

		for name, who := range map[string]gotgbot.User{"administrator": env.admin, "creator": owner} {
			msgID := settingsLockdownTestOpenAs(t, env, who)
			_, keyboard := env.lastEdit(env.chat.Id, msgID)
			toggles, _ := settingsLockdownTestRows(t, keyboard)
			env.pressRaw(env.chat, who, msgID, toggles[2].CallbackData)
			if row, ok := settingsLockdownTestRow(t, env.chat.Id); !ok || row.AIEnabled {
				t.Errorf("%s: menu toggle left %+v (row %v), want AI off", name, row, ok)
			}
			env.send(who, "/autolockdown ai on")
			env.wantReplyHas(staffMarker("lockdown_auto_saved"))
			if row, ok := settingsLockdownTestRow(t, env.chat.Id); !ok || !row.AIEnabled {
				t.Errorf("%s: /autolockdown ai on left %+v (row %v), want AI on", name, row, ok)
			}
		}
	})
}

func TestSettingsLockdownCallbackBytes(t *testing.T) {
	tr := i18n.MustNewTranslator("en")
	for _, variant := range []struct {
		name string
		view settingsView
	}{
		{"private", settingsView{Tr: tr, Private: true, OwnerKey: "c", OwnerValue: "-1001234567890", GroupID: -1001234567890, GroupTitle: "G", OpenerName: "N"}},
		{"group", settingsView{Tr: tr, OwnerKey: "u", OwnerValue: "9999999999", GroupID: -1001234567890, GroupTitle: "G", OpenerName: "N"}},
	} {
		t.Run(variant.name, func(t *testing.T) {
			longest := 0
			for _, n := range []int{3, 250, 500} {
				for _, on := range []bool{true, false} {
					snap := settingsLockdownSnapshot{
						Triggers: &models.ChatRaidDetection{
							JoinEnabled: on, JoinThreshold: n, FloodEnabled: on, FloodThreshold: n, AIEnabled: on, AIThreshold: n,
						},
						AIAvailable: true,
					}
					screen := renderSettingsLockdown(variant.view, snap, "")
					if screen.Omitted != 0 {
						t.Errorf("threshold %d on %v omitted %d buttons, want none", n, on, screen.Omitted)
					}
					buttons := settingsAllButtons(screen.Keyboard.InlineKeyboard)
					if len(buttons) > 20 {
						t.Errorf("threshold %d on %v has %d buttons, want at most 20 before the Lift row", n, on, len(buttons))
					}
					for _, button := range buttons {
						size := len(button.CallbackData)
						longest = max(longest, size)
						if size < 1 || size > 64 {
							t.Errorf("button %q has %d bytes of callback data, want 1..64", button.Text, size)
						}
					}
				}
			}
			if variant.view.Private && longest != 47 {
				t.Errorf("the longest private callback is %d bytes, want 47", longest)
			}
		})
	}
}

func TestSettingsLockdownPageFitsEveryLocale(t *testing.T) {
	withRealLocales(t)
	name := string([]rune(strings.Repeat("&<ü✓", 16))[:64])
	title := name
	locked := time.Date(2026, 10, 30, 13, 40, 0, 0, time.UTC)
	row := &models.ChatLockdown{
		ChatID:        -1001234567890,
		TriggerKind:   models.LockdownTriggerManual,
		StartedBy:     9999999999,
		StartedByName: name,
		Reason:        "<" + strings.Repeat("ü", 299),
		LockedAt:      &locked,
	}
	row.CreatedAt = locked
	snap := settingsLockdownSnapshot{
		Row:      row,
		Triggers: &models.ChatRaidDetection{JoinEnabled: true, JoinThreshold: 500, FloodEnabled: false, FloodThreshold: 500, AIEnabled: true, AIThreshold: 500},
	}

	for _, code := range []string{"en", "es", "fr", "hi", "id", "pt", "ru"} {
		t.Run(code, func(t *testing.T) {
			tr := i18n.MustNewTranslator(code)
			notice := settingsNoticeWarn(autolockdownApplyText(tr, autoLockdownApplyResult{Outcome: autoApplyRange}))
			view := settingsView{
				Tr: tr, OwnerKey: "u", OwnerValue: "9999999999", GroupID: -1001234567890,
				GroupTitle: title, OpenerName: name, Now: locked,
			}
			for label, s := range map[string]settingsLockdownSnapshot{"no AI provider": snap, "AI provider": {Row: row, Triggers: snap.Triggers, AIAvailable: true}} {
				screen := renderSettingsLockdown(view, s, notice)
				if units := panelUTF16Len(screen.Text); units > 3800 {
					t.Errorf("%s: page is %d UTF-16 units, want at most 3800", label, units)
				}
				if screen.Omitted != 0 {
					t.Errorf("%s: %d buttons omitted", label, screen.Omitted)
				}
				for _, button := range settingsAllButtons(screen.Keyboard.InlineKeyboard) {
					if strings.TrimSpace(button.Text) == "" {
						t.Errorf("%s: a button has an empty label", label)
					}
				}
				if !strings.Contains(screen.Text, "&lt;") {
					t.Errorf("%s: the page %q carries no escaped reason", label, screen.Text)
				}
			}
		})
	}
}
