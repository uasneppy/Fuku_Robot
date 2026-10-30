//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db"
	dbcache "github.com/divkix/Alita_Robot/alita/db/cache"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/i18n"
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

// settingsAntifloodWantRow fails unless the stored row of chatID holds exactly the given
// values, read straight from the database.
func settingsAntifloodWantRow(t *testing.T, chatID int64, limit int, action string, deleteOn bool) {
	t.Helper()
	row, ok := settingsAntifloodRow(t, chatID)
	if !ok {
		t.Fatalf("chat %d has no antiflood row, want limit %d action %s delete %v", chatID, limit, action, deleteOn)
	}
	if row.Limit != limit || row.Action != action || row.DeleteAntifloodMessage != deleteOn {
		t.Errorf("stored row = limit %d action %s delete %v, want limit %d action %s delete %v",
			row.Limit, row.Action, row.DeleteAntifloodMessage, limit, action, deleteOn)
	}
}

// settingsAntifloodSentHas reports whether the bot sent chatID a message carrying text.
func settingsAntifloodSentHas(env *settingsEnv, chatID int64, text string) bool {
	for _, sent := range env.fake.sentTo(chatID) {
		if strings.Contains(fmt.Sprint(sent.Params["text"]), text) {
			return true
		}
	}
	return false
}

// settingsAntifloodPress hand-builds a press of the Antiflood page on message msgID of chat,
// as the env's administrator in a group menu.
func settingsAntifloodPress(t *testing.T, env *settingsEnv, chat gotgbot.Chat, msgID int64, kv ...string) {
	t.Helper()
	fields := env.groupFields(env.admin, kv...)
	data := encodeCallbackData(settingsCallbackNamespace, fields)
	if data == "" {
		t.Fatalf("hand-built press %v did not encode", fields)
	}
	env.pressRaw(chat, env.admin, msgID, data)
}

func TestSettingsParityAntiflood(t *testing.T) {
	env := newSettingsEnv(t)
	msgID := settingsAntifloodOpen(t, env)
	admin := env.admin
	chatID := env.chat.Id
	redraw := func() (string, [][]gotgbot.InlineKeyboardButton) {
		env.press(admin, msgID, env.groupFields(admin, "a", "op", "p", "f"))
		return env.lastEdit(chatID, msgID)
	}

	// (1) /setflood writes, the page shows it.
	env.send(admin, "/setflood 9")
	text, _ := redraw()
	if want := staffMarker("settings_antiflood_limit") + " 9"; !strings.Contains(text, want) {
		t.Errorf("after /setflood 9 the page shows %q, want %q", text, want)
	}
	if row, ok := settingsAntifloodRow(t, chatID); !ok || row.Limit != 9 {
		t.Errorf("stored limit = %d (row %v) after /setflood 9, want 9", row.Limit, ok)
	}

	// (2) the page writes, /flood shows it.
	settingsAntifloodPress(t, env, env.chat, msgID, "a", "st", "p", "f", "t", "l", "v", "12")
	env.send(admin, "/flood")
	env.wantReplyHas(staffMarker("antiflood_flood_show_settings"), "12")
	if row, ok := settingsAntifloodRow(t, chatID); !ok || row.Limit != 12 {
		t.Errorf("stored limit = %d (row %v) after the menu step to 12, want 12", row.Limit, ok)
	}

	// (3) /setfloodmode kick writes, the page marks Kick.
	env.send(admin, "/setfloodmode kick")
	_, keyboard := redraw()
	kick, ok := settingsAntifloodControlButton(t, keyboard, "m", "kick")
	if !ok || !strings.HasPrefix(kick.Text, "✅ ") {
		t.Errorf("after /setfloodmode kick the Kick button is %q, want it selected", kick.Text)
	}
	settingsAntifloodWantRow(t, chatID, 12, "kick", false)

	// (4) the page's Ban writes, /flood shows banned.
	settingsAntifloodPress(t, env, env.chat, msgID, "a", "st", "p", "f", "t", "m", "v", "ban")
	env.send(admin, "/flood")
	env.wantReplyHas(staffMarker("antiflood_flood_show_settings"), "12", "banned")
	settingsAntifloodWantRow(t, chatID, 12, "ban", false)

	// (5) /delflood on writes, the page's switch is on.
	env.send(admin, "/delflood on")
	_, keyboard = redraw()
	toggle, ok := settingsAntifloodControlButton(t, keyboard, "d", "0")
	if !ok || !strings.Contains(toggle.Text, "✅ "+staffMarker("common_status_on")) {
		t.Errorf("after /delflood on the switch is %q (found %v), want it on and offering to turn off", toggle.Text, ok)
	}
	settingsAntifloodWantRow(t, chatID, 12, "ban", true)

	// (6) the page's toggle turns it off, /delflood without an argument says so.
	settingsAntifloodPress(t, env, env.chat, msgID, "a", "tg", "p", "f", "t", "d", "v", "0")
	env.send(admin, "/delflood")
	env.wantReplyHas(staffMarker("antiflood_flood_deleter_already_disabled"))
	settingsAntifloodWantRow(t, chatID, 12, "ban", false)
}

func TestSettingsDriftAntiflood(t *testing.T) {
	setup := func(t *testing.T) (*settingsEnv, gotgbot.Chat) {
		t.Helper()
		env := newSettingsEnv(t)
		return env, env.addChat()
	}
	const menuMsgID = 4242

	t.Run("limit", func(t *testing.T) {
		for _, tc := range []struct {
			name    string
			command string
			menu    string
			want    int
			refused bool
		}{
			{"1 is refused", "/setflood 1", "1", 7, true},
			{"2 is refused", "/setflood 2", "2", 7, true},
			{"101 is refused", "/setflood 101", "101", 7, true},
			{"3 is stored", "/setflood 3", "3", 3, false},
			{"100 is stored", "/setflood 100", "100", 100, false},
			{"off turns it off", "/setflood off", "0", 0, false},
		} {
			t.Run(tc.name, func(t *testing.T) {
				env, menuChat := setup(t)
				env.send(env.admin, "/setflood 7")
				env.sendIn(menuChat, env.admin, "/setflood 7")

				env.send(env.admin, tc.command)
				if tc.refused {
					env.wantReplyHas(staffMarker("antiflood_errors_set_in_limit"))
				}
				settingsAntifloodPress(t, env, menuChat, menuMsgID, "a", "st", "p", "f", "t", "l", "v", tc.menu)

				settingsAntifloodWantRow(t, env.chat.Id, tc.want, "mute", false)
				settingsAntifloodWantRow(t, menuChat.Id, tc.want, "mute", false)
				if tc.refused {
					text, _ := env.lastEdit(menuChat.Id, menuMsgID)
					if !strings.Contains(text, staffMarker("antiflood_errors_set_in_limit")) {
						t.Errorf("the menu redrew %q, want a notice with the command's own antiflood_errors_set_in_limit", text)
					}
				}
			})
		}
	})

	t.Run("action", func(t *testing.T) {
		t.Run("warn is refused by both", func(t *testing.T) {
			env, menuChat := setup(t)
			env.send(env.admin, "/setfloodmode warn")
			env.wantReplyHas(staffMarker("antiflood_setfloodmode_unknown_type"))
			settingsAntifloodPress(t, env, menuChat, menuMsgID, "a", "st", "p", "f", "t", "m", "v", "warn")

			if _, ok := settingsAntifloodRow(t, env.chat.Id); ok {
				t.Error("/setfloodmode warn wrote a row")
			}
			if _, ok := settingsAntifloodRow(t, menuChat.Id); ok {
				t.Error("the menu's warn wrote a row")
			}
			text, _ := env.lastEdit(menuChat.Id, menuMsgID)
			if !strings.Contains(text, staffMarker("antiflood_setfloodmode_unknown_type")) {
				t.Errorf("the menu redrew %q, want a notice with the command's own antiflood_setfloodmode_unknown_type", text)
			}
		})

		t.Run("ban is stored by both", func(t *testing.T) {
			env, menuChat := setup(t)
			env.send(env.admin, "/setfloodmode ban")
			settingsAntifloodPress(t, env, menuChat, menuMsgID, "a", "st", "p", "f", "t", "m", "v", "ban")

			commandRow, ok := settingsAntifloodRow(t, env.chat.Id)
			if !ok {
				t.Fatal("/setfloodmode ban stored no row")
			}
			settingsAntifloodWantRow(t, menuChat.Id, commandRow.Limit, "ban", commandRow.DeleteAntifloodMessage)
			if commandRow.Action != "ban" {
				t.Errorf("command stored action %q, want ban", commandRow.Action)
			}
		})
	})

	t.Run("malformed values change nothing", func(t *testing.T) {
		for _, tc := range []struct{ control, value string }{
			{"l", "abc"}, {"l", "1000"}, {"l", "-1"}, {"l", "٣"},
			{"m", "BAN"}, {"m", "ban1"}, {"m", "abcdefghijk"},
			{"d", "2"}, {"d", "on"},
		} {
			t.Run(tc.control+"="+tc.value, func(t *testing.T) {
				env, menuChat := setup(t)
				settingsAntifloodPress(t, env, menuChat, menuMsgID, "a", "st", "p", "f", "t", tc.control, "v", tc.value)
				answer, alert := env.lastAnswer()
				if !alert || !strings.Contains(answer, staffMarker("common_callback_invalid_request")) {
					t.Errorf("answer = %q (alert %v), want the invalid request alert", answer, alert)
				}
				if _, ok := settingsAntifloodRow(t, menuChat.Id); ok {
					t.Error("a malformed press wrote a row")
				}
			})
		}
	})
}

func TestSettingsAuthorityParityAntiflood(t *testing.T) {
	const menuMsgID = 5151

	t.Run("a bot that is not an admin is refused like the commands", func(t *testing.T) {
		env := newSettingsEnv(t)
		chat := env.addChat()
		env.fake.setBotRole(chat.Id, staffRoleMember)
		refusal := staffMarker("connections_is_user_connected_bot_not_admin")

		env.sendIn(chat, env.admin, "/setflood 5")
		if !settingsAntifloodSentHas(env, chat.Id, refusal) {
			t.Fatal("/setflood 5 was not refused for a bot that is not an admin")
		}

		// Viewing needs only the live admin.
		settingsAntifloodPress(t, env, chat, menuMsgID, "a", "op", "p", "f")
		text, _ := env.lastEdit(chat.Id, menuMsgID)
		if !strings.Contains(text, staffMarker("settings_page_antiflood")) || strings.Contains(text, staffMarker("settings_cannot_open")) {
			t.Fatalf("opening the page drew %q, want the Antiflood page", text)
		}

		for _, press := range [][]string{
			{"a", "st", "p", "f", "t", "l", "v", "6"},
			{"a", "st", "p", "f", "t", "m", "v", "ban"},
			{"a", "tg", "p", "f", "t", "d", "v", "1"},
		} {
			answers := env.answerCount()
			settingsAntifloodPress(t, env, chat, menuMsgID, press...)
			text, _ = env.lastEdit(chat.Id, menuMsgID)
			if !strings.Contains(text, refusal) {
				t.Errorf("press %v drew %q, want the notice %s", press, text, refusal)
			}
			if got := env.answerCount() - answers; got != 1 {
				t.Errorf("press %v answered %d times, want once", press, got)
			}
			if _, ok := settingsAntifloodRow(t, chat.Id); ok {
				t.Fatalf("press %v wrote a row although the bot is not an admin", press)
			}
		}
	})

	t.Run("a bot that is an admin lets both succeed", func(t *testing.T) {
		env := newSettingsEnv(t)
		chat := env.addChat()
		env.sendIn(chat, env.admin, "/setflood 5")
		if !settingsAntifloodSentHas(env, chat.Id, staffMarker("antiflood_setflood_success")) {
			t.Fatal("/setflood 5 was not accepted for a bot that is an admin")
		}
		settingsAntifloodPress(t, env, chat, menuMsgID, "a", "st", "p", "f", "t", "l", "v", "6")
		settingsAntifloodWantRow(t, chat.Id, 6, "mute", false)
		text, _ := env.lastEdit(chat.Id, menuMsgID)
		if strings.Contains(text, staffMarker("connections_is_user_connected_bot_not_admin")) {
			t.Errorf("the menu refused %q although the bot is an admin", text)
		}
	})

	t.Run("a plain member's private press is refused", func(t *testing.T) {
		env := newSettingsEnv(t)
		member := gotgbot.User{Id: uniqueLinkOwnerID() + 31, FirstName: "Plain"}
		env.fake.setMember(env.chat.Id, member.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})

		env.pressPrivate(member, 777, env.privateFields(env.chat.Id, "a", "st", "p", "f", "t", "l", "v", "5"))

		answer, alert := env.lastAnswer()
		if !alert || !strings.Contains(answer, staffMarker("settings_not_admin")) {
			t.Errorf("answer = %q (alert %v), want the alert settings_not_admin", answer, alert)
		}
		if _, ok := settingsAntifloodRow(t, env.chat.Id); ok {
			t.Error("a plain member's press wrote a row")
		}
	})
}

// settingsAntifloodCheckBytes fails unless every callback button of the screen fits.
func settingsAntifloodCheckBytes(t *testing.T, name string, screen settingsScreen) {
	t.Helper()
	if screen.Omitted != 0 {
		t.Errorf("%s omitted %d buttons for overflow, want none", name, screen.Omitted)
	}
	buttons := settingsAllButtons(screen.Keyboard.InlineKeyboard)
	if len(buttons) == 0 {
		t.Errorf("%s has no buttons", name)
	}
	for _, button := range buttons {
		if n := len(button.CallbackData); n < 1 || n > 64 {
			t.Errorf("%s button %q has %d bytes of callback data, want 1..64", name, button.Text, n)
		}
	}
}

func TestSettingsAntifloodCallbackBytes(t *testing.T) {
	tr := i18n.MustNewTranslator("en")
	for _, variant := range []struct {
		name string
		view settingsView
	}{
		{"private", settingsView{Tr: tr, Private: true, OwnerKey: "c", OwnerValue: "-1001234567890", GroupID: -1001234567890, GroupTitle: "G", OpenerName: "N"}},
		{"group", settingsView{Tr: tr, OwnerKey: "u", OwnerValue: "9999999999", GroupID: -1001234567890, GroupTitle: "G", OpenerName: "N"}},
	} {
		t.Run(variant.name, func(t *testing.T) {
			for _, limit := range []int{0, 3, 50, 100} {
				for _, action := range []string{"mute", "ban", "kick", "warn", "tban", "tmute"} {
					for _, deleteOn := range []bool{false, true} {
						name := fmt.Sprintf("limit %d action %s delete %v", limit, action, deleteOn)
						screen := renderSettingsAntiflood(variant.view,
							&models.AntifloodSettings{Limit: limit, Action: action, DeleteAntifloodMessage: deleteOn}, "")
						settingsAntifloodCheckBytes(t, name, screen)
					}
				}
			}
		})
	}
}

func TestSettingsAntifloodPageFitsEveryLocale(t *testing.T) {
	withRealLocales(t)
	for _, code := range []string{"en", "es", "fr", "hi", "id", "pt", "ru"} {
		t.Run(code, func(t *testing.T) {
			tr := i18n.MustNewTranslator(code)
			view := settingsView{
				Tr: tr, OwnerKey: "u", OwnerValue: "9999999999", GroupID: -1001234567890,
				GroupTitle: strings.Repeat("&", 64), OpenerName: "N",
			}
			notice := settingsNoticeWarn(fmt.Sprintf(settingsText(tr, "antiflood_setfloodmode_unknown_type"), strings.Repeat("a", settingsFloodModeMaxLen)))
			for _, s := range []*models.AntifloodSettings{
				{Limit: 100, Action: "tmute", DeleteAntifloodMessage: true},
				{Limit: 0, Action: "tban"},
				{Limit: 5, Action: "mute"},
			} {
				screen := renderSettingsAntiflood(view, s, notice)
				if units := len(utf16.Encode([]rune(screen.Text))); units > 3800 {
					t.Errorf("%s page for %+v is %d UTF-16 units, want at most 3800", code, s, units)
				}
				for _, placeholder := range []string{"{n}", "{action}", "{state}", "{page}", "%!"} {
					if strings.Contains(screen.Text, placeholder) {
						t.Errorf("%s page %q keeps the unfilled %s", code, screen.Text, placeholder)
					}
				}
				for _, button := range settingsAllButtons(screen.Keyboard.InlineKeyboard) {
					if strings.TrimSpace(button.Text) == "" {
						t.Errorf("%s page for %+v has a button with an empty label", code, s)
					}
				}
				settingsAntifloodCheckBytes(t, code, screen)
			}
		})
	}
}
