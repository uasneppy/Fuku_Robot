//go:build testtools

package modules

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db/menuaccess"
)

// settingsAccessStored reads the owner switch of chatID straight from the database.
func settingsAccessStored(t *testing.T, chatID int64) bool {
	t.Helper()
	allowed, err := menuaccess.AdminsAllowedFresh(context.Background(), chatID)
	if err != nil {
		t.Fatalf("AdminsAllowedFresh(%d) error = %v", chatID, err)
	}
	return allowed
}

// settingsAccessFind returns the first button whose decoded fields satisfy match.
func settingsAccessFind(t *testing.T, keyboard [][]gotgbot.InlineKeyboardButton, match func(map[string]string) bool) (gotgbot.InlineKeyboardButton, bool) {
	t.Helper()
	for _, button := range settingsAllButtons(keyboard) {
		if match(settingsButtonFields(t, button)) {
			return button, true
		}
	}
	return gotgbot.InlineKeyboardButton{}, false
}

// settingsAccessToggle finds the switch toggle of a drawn Access page and returns it with
// the target value it sends.
func settingsAccessToggle(t *testing.T, keyboard [][]gotgbot.InlineKeyboardButton) (gotgbot.InlineKeyboardButton, string) {
	t.Helper()
	button, ok := settingsAccessFind(t, keyboard, func(f map[string]string) bool {
		return f["a"] == "tg" && f["p"] == settingsPageAccess && f["t"] == settingsAccessControlSwitch
	})
	if !ok {
		t.Fatalf("the Access page has no toggle: %+v", keyboard)
	}
	return button, settingsButtonFields(t, button)["v"]
}

// settingsAccessOpenFromMain presses the Access button of the Main page drawn on msgID and
// returns once the page is drawn. press is the test's way to press a raw button.
func settingsAccessOpenFromMain(t *testing.T, keyboard [][]gotgbot.InlineKeyboardButton, press func(gotgbot.InlineKeyboardButton)) {
	t.Helper()
	button, ok := settingsPageButton(t, keyboard, settingsPageAccess)
	if !ok {
		t.Fatalf("the Main page has no Access button: %+v", keyboard)
	}
	press(button)
}

func TestSettingsAccessOwnerFlow(t *testing.T) {
	t.Run("in a group menu", func(t *testing.T) {
		resetSettingsOwnerMemo(t)
		env := newSettingsEnv(t)
		owner := env.newOwner(env.chat)
		env.setAdminsAllowed(env.chat.Id, false)

		msgID := env.openMenu(owner)
		_, main := env.lastEdit(env.chat.Id, msgID)
		settingsAccessOpenFromMain(t, main, func(b gotgbot.InlineKeyboardButton) {
			env.pressRaw(env.chat, owner, msgID, b.CallbackData)
		})

		text, keyboard := env.lastEdit(env.chat.Id, msgID)
		for _, want := range []string{
			staffMarker("settings_access_status") + " ❌ " + staffMarker("common_status_off"),
			staffMarker("settings_access_off_desc"),
		} {
			if !strings.Contains(text, want) {
				t.Fatalf("Access page %q does not contain %q", text, want)
			}
		}
		toggle, target := settingsAccessToggle(t, keyboard)
		if target != "1" {
			t.Fatalf("the off toggle sends v=%s, want 1", target)
		}

		// The first tap toward ON draws the card and writes nothing.
		env.pressRaw(env.chat, owner, msgID, toggle.CallbackData)
		text, keyboard = env.lastEdit(env.chat.Id, msgID)
		if !strings.Contains(text, staffMarker("settings_access_confirm")) {
			t.Fatalf("the first tap drew %q, want the confirm card", text)
		}
		if settingsAccessStored(t, env.chat.Id) {
			t.Fatal("the first tap toward ON wrote ON")
		}
		yes, ok := settingsAccessFind(t, keyboard, func(f map[string]string) bool { return f["a"] == settingsActAccessYes && f["e"] != "" })
		if !ok {
			t.Fatalf("the card has no Yes with an expiry: %+v", keyboard)
		}
		if _, ok := settingsAccessFind(t, keyboard, func(f map[string]string) bool { return f["a"] == "no" && f["p"] == settingsPageAccess }); !ok {
			t.Errorf("the card has no No back to the Access page: %+v", keyboard)
		}

		// The admin is still shut out until Yes.
		env.press(env.admin, msgID, env.groupFields(env.admin, "a", "bk"))
		if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("settings_gate_owner_only")) {
			t.Errorf("before Yes the admin's answer = %q (alert %v), want settings_gate_owner_only", answer, alert)
		}

		env.pressRaw(env.chat, owner, msgID, yes.CallbackData)
		if !settingsAccessStored(t, env.chat.Id) {
			t.Fatal("Yes did not write ON")
		}
		text, keyboard = env.lastEdit(env.chat.Id, msgID)
		if !strings.Contains(text, staffMarker("settings_access_on_desc")) {
			t.Errorf("after Yes the page drew %q, want the on description", text)
		}
		toggle, target = settingsAccessToggle(t, keyboard)
		if target != "0" {
			t.Fatalf("the on toggle sends v=%s, want 0", target)
		}

		// The admin's next press passes the gate.
		editsBefore := len(env.edits(env.chat.Id, msgID))
		env.press(env.admin, msgID, env.groupFields(env.admin, "a", "bk"))
		if got := len(env.edits(env.chat.Id, msgID)); got != editsBefore+1 {
			t.Errorf("the admin's press made %d edits, want 1", got-editsBefore)
		}

		// One tap turns it off at once and the admin is shut out again.
		env.pressRaw(env.chat, owner, msgID, toggle.CallbackData)
		if settingsAccessStored(t, env.chat.Id) {
			t.Fatal("the OFF tap did not write OFF")
		}
		env.press(env.admin, msgID, env.groupFields(env.admin, "a", "bk"))
		if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("settings_gate_owner_only")) {
			t.Errorf("after OFF the admin's answer = %q (alert %v), want settings_gate_owner_only", answer, alert)
		}
	})

	t.Run("in a private menu", func(t *testing.T) {
		resetSettingsOwnerMemo(t)
		env := newSettingsEnv(t)
		owner := env.newOwner(env.chat)
		env.setAdminsAllowed(env.chat.Id, false)

		env.sendPrivate(owner, fmt.Sprintf("/start settings_%d", env.chat.Id))
		msgID := settingsPrivateOnlySent(t, env, owner).MessageID
		private := settingsPrivateChat(owner)
		sent := env.fake.sentTo(owner.Id)
		mainKeyboard := staffKeyboardOf(sent[len(sent)-1].Params["reply_markup"])
		settingsAccessOpenFromMain(t, mainKeyboard, func(b gotgbot.InlineKeyboardButton) {
			env.pressRaw(private, owner, msgID, b.CallbackData)
		})

		text, keyboard := env.lastEdit(owner.Id, msgID)
		if !strings.Contains(text, staffMarker("settings_access_off_desc")) {
			t.Fatalf("private Access page %q does not describe the off state", text)
		}
		toggle, target := settingsAccessToggle(t, keyboard)
		if target != "1" || settingsButtonFields(t, toggle)["c"] != fmt.Sprint(env.chat.Id) {
			t.Fatalf("private toggle fields = %v, want v=1 and the group's c", settingsButtonFields(t, toggle))
		}

		env.pressRaw(private, owner, msgID, toggle.CallbackData)
		_, keyboard = env.lastEdit(owner.Id, msgID)
		if settingsAccessStored(t, env.chat.Id) {
			t.Fatal("the first private tap toward ON wrote ON")
		}
		yes, ok := settingsAccessFind(t, keyboard, func(f map[string]string) bool { return f["a"] == settingsActAccessYes })
		if !ok {
			t.Fatalf("the private card has no Yes: %+v", keyboard)
		}
		env.pressRaw(private, owner, msgID, yes.CallbackData)
		if !settingsAccessStored(t, env.chat.Id) {
			t.Fatal("the private Yes did not write ON")
		}
		_, keyboard = env.lastEdit(owner.Id, msgID)
		toggle, target = settingsAccessToggle(t, keyboard)
		if target != "0" {
			t.Fatalf("the private on toggle sends v=%s, want 0", target)
		}
		env.pressRaw(private, owner, msgID, toggle.CallbackData)
		if settingsAccessStored(t, env.chat.Id) {
			t.Fatal("the private OFF tap did not write OFF")
		}
	})
}
