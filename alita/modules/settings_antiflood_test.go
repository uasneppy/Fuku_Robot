//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db"
	dbcache "github.com/divkix/Alita_Robot/alita/db/cache"
	"github.com/divkix/Alita_Robot/alita/db/models"
)

// settingsAntifloodRow reads the antiflood row of chatID straight from the database, past
// every cache layer. ok is false when the chat has no row.
func settingsAntifloodRow(t *testing.T, chatID int64) (row models.AntifloodSettings, ok bool) {
	t.Helper()
	if err := db.DB.Where("chat_id = ?", chatID).Limit(1).Find(&row).Error; err != nil {
		t.Fatalf("read antiflood_settings of %d: %v", chatID, err)
	}
	return row, row.ID != 0
}

// settingsAntifloodSeed writes an antiflood row with a limit of 3 or more directly, which no
// command can do for warn, tban or tmute, and drops the cached copy.
func settingsAntifloodSeed(t *testing.T, chatID int64, limit int, action string) {
	t.Helper()
	if err := db.DB.Create(&models.AntifloodSettings{ChatId: chatID, Limit: limit, Action: action}).Error; err != nil {
		t.Fatalf("seed antiflood_settings of %d: %v", chatID, err)
	}
	dbcache.DeleteCache(dbcache.CacheKey("antiflood", chatID))
	dbcache.ResetLocalForTest()
}

// settingsAntifloodOpen opens the menu as the env's administrator, presses the Antiflood
// button and returns the menu message ID.
func settingsAntifloodOpen(t *testing.T, env *settingsEnv) int64 {
	t.Helper()
	msgID := env.openMenu(env.admin)
	_, keyboard := env.lastEdit(env.chat.Id, msgID)
	button, ok := settingsPageButton(t, keyboard, settingsPageAntiflood)
	if !ok {
		t.Fatal("the Main page has no Antiflood button")
	}
	env.pressRaw(env.chat, env.admin, msgID, button.CallbackData)
	return msgID
}

// settingsAntifloodStepper returns the five buttons of the limit stepper on the current page
// of msgID.
func settingsAntifloodStepper(t *testing.T, env *settingsEnv, msgID int64) []gotgbot.InlineKeyboardButton {
	t.Helper()
	_, keyboard := env.lastEdit(env.chat.Id, msgID)
	if len(keyboard) != 4 {
		t.Fatalf("Antiflood page has %d keyboard rows, want stepper, choice, toggle and navigation", len(keyboard))
	}
	if len(keyboard[0]) != 5 {
		t.Fatalf("Antiflood page stepper row has %d buttons, want 5", len(keyboard[0]))
	}
	return keyboard[0]
}

// settingsAntifloodControlButton finds the button of control t whose target is v.
func settingsAntifloodControlButton(t *testing.T, keyboard [][]gotgbot.InlineKeyboardButton, control, value string) (gotgbot.InlineKeyboardButton, bool) {
	t.Helper()
	for _, button := range settingsAllButtons(keyboard) {
		fields := settingsButtonFields(t, button)
		if fields["t"] == control && fields["v"] == value {
			return button, true
		}
	}
	return gotgbot.InlineKeyboardButton{}, false
}

// settingsAntifloodWantFields fails unless the button's callback data is exactly want.
func settingsAntifloodWantFields(t *testing.T, name string, button gotgbot.InlineKeyboardButton, want map[string]string) {
	t.Helper()
	if got := settingsButtonFields(t, button); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("%s fields = %v, want %v", name, got, want)
	}
}

func TestSettingsAntifloodPage(t *testing.T) {
	t.Run("Main button shows the limit or Off", func(t *testing.T) {
		env := newSettingsEnv(t)
		msgID := env.openMenu(env.admin)
		_, keyboard := env.lastEdit(env.chat.Id, msgID)
		button, ok := settingsButtonByMarker(keyboard, "settings_page_antiflood")
		if !ok {
			t.Fatal("no Antiflood button on the Main page")
		}
		if !strings.Contains(button.Text, staffMarker("settings_state_off_plain")) {
			t.Errorf("Antiflood button %q does not show Off on a fresh chat", button.Text)
		}

		env.send(env.admin, "/setflood 7")
		msgID = env.openMenu(env.admin)
		_, keyboard = env.lastEdit(env.chat.Id, msgID)
		button, ok = settingsButtonByMarker(keyboard, "settings_page_antiflood")
		if !ok {
			t.Fatal("no Antiflood button on the Main page after /setflood 7")
		}
		if !strings.Contains(button.Text, staffMarker("settings_state_limit")) || !strings.Contains(button.Text, "7") {
			t.Errorf("Antiflood button %q does not show the limit 7", button.Text)
		}
	})

	t.Run("limit 7, action ban, delete off", func(t *testing.T) {
		env := newSettingsEnv(t)
		env.send(env.admin, "/setflood 7")
		env.send(env.admin, "/setfloodmode ban")
		msgID := settingsAntifloodOpen(t, env)
		admin := fmt.Sprint(env.admin.Id)

		text, keyboard := env.lastEdit(env.chat.Id, msgID)
		for _, want := range []string{
			staffMarker("settings_header") + " " + staffMarker("settings_page_antiflood"),
			"<b>Lock Chat</b> <code>" + fmt.Sprint(env.chat.Id) + "</code>",
			staffMarker("settings_antiflood_limit") + " 7",
			staffMarker("settings_antiflood_action") + " " + staffMarker("settings_opt_ban"),
			staffMarker("settings_antiflood_delete") + " ❌ " + staffMarker("common_status_off"),
		} {
			if !strings.Contains(text, want) {
				t.Errorf("Antiflood page %q does not contain %q", text, want)
			}
		}
		if strings.Contains(text, staffMarker("settings_antiflood_action_note")) {
			t.Errorf("Antiflood page %q carries the legacy action note for a settable action", text)
		}

		if len(keyboard) != 4 {
			t.Fatalf("keyboard has %d rows, want 4", len(keyboard))
		}
		stepper := keyboard[0]
		if len(stepper) != 5 {
			t.Fatalf("stepper row has %d buttons, want 5", len(stepper))
		}
		for i, want := range []struct{ label, v string }{
			{settingsStepLabel(-10), "0"}, {settingsStepLabel(-1), "6"}, {"7", ""}, {settingsStepLabel(1), "8"}, {settingsStepLabel(10), "17"},
		} {
			if stepper[i].Text != want.label {
				t.Errorf("stepper button %d is labelled %q, want %q", i, stepper[i].Text, want.label)
			}
			if i == 2 {
				settingsAntifloodWantFields(t, "value button", stepper[i], map[string]string{"a": "op", "u": admin, "p": "f"})
				continue
			}
			settingsAntifloodWantFields(t, fmt.Sprintf("stepper button %d", i), stepper[i],
				map[string]string{"a": "st", "u": admin, "p": "f", "t": "l", "v": want.v})
		}

		choice := keyboard[1]
		if len(choice) != 3 {
			t.Fatalf("choice row has %d buttons, want Mute, Ban and Kick", len(choice))
		}
		for i, want := range []struct{ marker, v string }{
			{"settings_opt_mute", "mute"}, {"settings_opt_ban", "ban"}, {"settings_opt_kick", "kick"},
		} {
			if !strings.Contains(choice[i].Text, staffMarker(want.marker)) {
				t.Errorf("choice button %d %q is not %s", i, choice[i].Text, want.marker)
			}
			if selected := strings.HasPrefix(choice[i].Text, "✅ "); selected != (want.v == "ban") {
				t.Errorf("choice button %q selected = %v, want only Ban selected", choice[i].Text, selected)
			}
			settingsAntifloodWantFields(t, "choice button "+want.v, choice[i],
				map[string]string{"a": "st", "u": admin, "p": "f", "t": "m", "v": want.v})
		}

		toggle := keyboard[2]
		if len(toggle) != 1 || !strings.Contains(toggle[0].Text, staffMarker("settings_btn_antiflood_delete")) {
			t.Fatalf("toggle row = %+v, want the Delete flood messages switch", toggle)
		}
		settingsAntifloodWantFields(t, "delete toggle", toggle[0], map[string]string{"a": "tg", "u": admin, "p": "f", "t": "d", "v": "1"})

		nav := keyboard[3]
		if len(nav) != 2 {
			t.Fatalf("navigation row = %+v, want [Back | Close]", nav)
		}
	})

	t.Run("limit Off", func(t *testing.T) {
		env := newSettingsEnv(t)
		msgID := settingsAntifloodOpen(t, env)
		text, _ := env.lastEdit(env.chat.Id, msgID)
		if !strings.Contains(text, staffMarker("settings_antiflood_off")) {
			t.Errorf("Antiflood page %q does not carry the off line", text)
		}
		if strings.Contains(text, staffMarker("settings_antiflood_limit")) {
			t.Errorf("Antiflood page %q carries a limit line while antiflood is off", text)
		}
		if !strings.Contains(text, staffMarker("settings_antiflood_action")) || !strings.Contains(text, staffMarker("settings_antiflood_delete")) {
			t.Errorf("Antiflood page %q lost the action or delete line while antiflood is off", text)
		}
		stepper := settingsAntifloodStepper(t, env, msgID)
		if stepper[2].Text != staffMarker("settings_state_off_plain") {
			t.Errorf("value button is %q, want the Off word", stepper[2].Text)
		}
	})
}

func TestSettingsAntifloodStepper(t *testing.T) {
	for _, tc := range []struct {
		name      string
		preset    string
		index     int
		wantLimit int
	}{
		{"Off +1 stores 3", "", 3, 3},
		{"Off +10 stores 12", "", 4, 12},
		{"3 -1 stores Off", "/setflood 3", 1, 0},
		{"3 -10 stores Off", "/setflood 3", 0, 0},
		{"7 +1 stores 8", "/setflood 7", 3, 8},
		{"95 +10 stores 100", "/setflood 95", 4, 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := newSettingsEnv(t)
			if tc.preset != "" {
				env.send(env.admin, tc.preset)
			}
			msgID := settingsAntifloodOpen(t, env)
			stepper := settingsAntifloodStepper(t, env, msgID)
			before := env.answerCount()

			env.pressRaw(env.chat, env.admin, msgID, stepper[tc.index].CallbackData)

			row, ok := settingsAntifloodRow(t, env.chat.Id)
			if !ok || row.Limit != tc.wantLimit {
				t.Fatalf("stored limit = %d (row %v), want %d", row.Limit, ok, tc.wantLimit)
			}
			text, _ := env.lastEdit(env.chat.Id, msgID)
			want := staffMarker("settings_antiflood_limit") + " " + fmt.Sprint(tc.wantLimit)
			if tc.wantLimit == 0 {
				want = staffMarker("settings_antiflood_off")
			}
			if !strings.Contains(text, want) {
				t.Errorf("redrawn page %q does not contain %q", text, want)
			}
			if got := env.answerCount() - before; got != 1 {
				t.Errorf("the step answered the callback %d times, want once", got)
			}
		})
	}

	for _, bound := range []struct {
		name    string
		preset  string
		indexes []int
		target  string
		toast   string
		word    string
	}{
		{"highest", "/setflood 100", []int{3, 4}, "100", "settings_max_value", "100"},
		{"lowest", "", []int{0, 1}, "0", "settings_min_value", staffMarker("settings_state_off_plain")},
	} {
		t.Run("bound "+bound.name, func(t *testing.T) {
			env := newSettingsEnv(t)
			if bound.preset != "" {
				env.send(env.admin, bound.preset)
			}
			msgID := settingsAntifloodOpen(t, env)
			for _, index := range bound.indexes {
				stepper := settingsAntifloodStepper(t, env, msgID)
				step := stepper[index]
				if step.Text != "·" {
					t.Fatalf("step button %d at the bound is labelled %q, want ·", index, step.Text)
				}
				if v := settingsButtonFields(t, step)["v"]; v != bound.target {
					t.Fatalf("step button %d at the bound targets %q, want %s", index, v, bound.target)
				}
				beforeRow, hadRow := settingsAntifloodRow(t, env.chat.Id)
				edits := len(env.edits(env.chat.Id, msgID))
				answers := env.answerCount()

				env.pressRaw(env.chat, env.admin, msgID, step.CallbackData)

				if got := env.answerCount() - answers; got != 1 {
					t.Fatalf("bound press answered %d times, want once", got)
				}
				answer, alert := env.lastAnswer()
				if alert || !strings.Contains(answer, staffMarker(bound.toast)) || !strings.Contains(answer, bound.word) {
					t.Errorf("bound answer = %q (alert %v), want the %s toast carrying %s", answer, alert, bound.toast, bound.word)
				}
				if got := len(env.edits(env.chat.Id, msgID)); got != edits {
					t.Errorf("bound press edited the menu (%d -> %d edits), want none", edits, got)
				}
				afterRow, hasRow := settingsAntifloodRow(t, env.chat.Id)
				if hadRow != hasRow || !afterRow.UpdatedAt.Equal(beforeRow.UpdatedAt) || afterRow.Limit != beforeRow.Limit {
					t.Errorf("bound press changed the row: %+v (%v) -> %+v (%v)", beforeRow, hadRow, afterRow, hasRow)
				}
			}
		})
	}
}

func TestSettingsAntifloodFreshChat(t *testing.T) {
	env := newSettingsEnv(t)
	msgID := settingsAntifloodOpen(t, env)
	if _, ok := settingsAntifloodRow(t, env.chat.Id); ok {
		t.Fatal("opening the page created an antiflood row")
	}
	_, keyboard := env.lastEdit(env.chat.Id, msgID)
	ban, ok := settingsAntifloodControlButton(t, keyboard, "m", "ban")
	if !ok {
		t.Fatal("no Ban button on the Antiflood page")
	}

	env.pressRaw(env.chat, env.admin, msgID, ban.CallbackData)

	row, ok := settingsAntifloodRow(t, env.chat.Id)
	if !ok || row.Action != "ban" {
		t.Fatalf("stored row = %+v (row %v), want the action ban", row, ok)
	}
	text, _ := env.lastEdit(env.chat.Id, msgID)
	want := staffMarker("settings_antiflood_limit") + " " + fmt.Sprint(row.Limit)
	if row.Limit == 0 {
		want = staffMarker("settings_antiflood_off")
	}
	if !strings.Contains(text, want) {
		t.Errorf("the redraw %q does not show the limit the repository stored (%d): want %q", text, row.Limit, want)
	}

	other := env.addChat()
	env.sendIn(other, env.admin, "/setfloodmode ban")
	otherRow, ok := settingsAntifloodRow(t, other.Id)
	if !ok {
		t.Fatal("/setfloodmode ban on a fresh chat stored no row")
	}
	if otherRow.Limit != row.Limit || otherRow.Action != row.Action || otherRow.DeleteAntifloodMessage != row.DeleteAntifloodMessage {
		t.Errorf("the command left %+v, the menu left %+v: they must agree", otherRow, row)
	}
}

func TestSettingsAntifloodStoredLegacyAction(t *testing.T) {
	for _, tc := range []struct{ action, marker string }{
		{"warn", "settings_opt_warn"},
		{"tban", "settings_opt_tban"},
		{"tmute", "settings_opt_tmute"},
	} {
		t.Run(tc.action, func(t *testing.T) {
			env := newSettingsEnv(t)
			settingsAntifloodSeed(t, env.chat.Id, 5, tc.action)
			msgID := settingsAntifloodOpen(t, env)

			text, keyboard := env.lastEdit(env.chat.Id, msgID)
			if !strings.Contains(text, staffMarker("settings_antiflood_action")+" "+staffMarker(tc.marker)) {
				t.Errorf("page %q does not show the %s word on the action line", text, tc.action)
			}
			if !strings.Contains(text, staffMarker("settings_antiflood_action_note")) {
				t.Errorf("page %q does not carry the read-only action note", text)
			}
			for _, button := range settingsAllButtons(keyboard) {
				if strings.HasPrefix(button.Text, "✅ ") && settingsButtonFields(t, button)["t"] == "m" {
					t.Errorf("action button %q is marked selected for a stored %s", button.Text, tc.action)
				}
			}

			mute, ok := settingsAntifloodControlButton(t, keyboard, "m", "mute")
			if !ok {
				t.Fatal("no Mute button on the page")
			}
			env.pressRaw(env.chat, env.admin, msgID, mute.CallbackData)

			row, _ := settingsAntifloodRow(t, env.chat.Id)
			if row.Action != "mute" {
				t.Errorf("stored action = %q after Mute, want mute", row.Action)
			}
			text, _ = env.lastEdit(env.chat.Id, msgID)
			if strings.Contains(text, staffMarker("settings_antiflood_action_note")) {
				t.Errorf("page %q still carries the note after the action was replaced", text)
			}
		})
	}
}

func TestSettingsAntifloodReadFailure(t *testing.T) {
	env := newSettingsEnv(t)
	previous := settingsFloodRead
	settingsFloodRead = func(context.Context, int64) (*models.AntifloodSettings, error) {
		return nil, errors.New("database unavailable")
	}
	t.Cleanup(func() { settingsFloodRead = previous })

	msgID := env.openMenu(env.admin)
	_, keyboard := env.lastEdit(env.chat.Id, msgID)
	button, ok := settingsButtonByMarker(keyboard, "settings_page_antiflood")
	if !ok {
		t.Fatal("no Antiflood button on the Main page")
	}
	if !strings.HasSuffix(button.Text, " · ❔") {
		t.Errorf("Antiflood button %q does not end with the unreadable state ' · ❔'", button.Text)
	}

	before := env.answerCount()
	env.pressRaw(env.chat, env.admin, msgID, button.CallbackData)

	if got := env.answerCount() - before; got != 1 {
		t.Errorf("the press was answered %d times, want once", got)
	}
	answer, alert := env.lastAnswer()
	if !alert || !strings.Contains(answer, staffMarker("settings_cannot_open_alert")) {
		t.Errorf("answer = %q (alert %v), want the alert settings_cannot_open_alert", answer, alert)
	}
	text, kb := env.lastEdit(env.chat.Id, msgID)
	if !strings.Contains(text, staffMarker("settings_cannot_open")) || strings.Contains(text, staffMarker("settings_cannot_open_alert")) {
		t.Errorf("the menu was edited to %q, want settings_cannot_open", text)
	}
	if len(kb) != 0 {
		t.Errorf("the cannot-open edit kept the keyboard %+v", kb)
	}
}
