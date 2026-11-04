//go:build testtools

package modules

import (
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

// settingsEditControl returns the button labelled labelKey on a page, checks its control is t, and returns
// its decoded fields.
func settingsEditControl(t *testing.T, keyboard [][]gotgbot.InlineKeyboardButton, labelKey, control string) (gotgbot.InlineKeyboardButton, map[string]string) {
	t.Helper()
	button, ok := settingsButtonByMarker(keyboard, labelKey)
	if !ok {
		t.Fatalf("the page has no button labelled %s: %+v", labelKey, keyboard)
	}
	fields := settingsButtonFields(t, button)
	if fields["t"] != control {
		t.Fatalf("button %s carries control %q, want %q", labelKey, fields["t"], control)
	}
	return button, fields
}

func TestSettingsEditWelcomeSwitch(t *testing.T) {
	env := newSettingsEditEnv(t)
	owner := env.newOwner(env.chat)

	// Main shows the Welcome button and its state line.
	msgID := env.openMenu(owner)
	mainText, mainKeyboard := env.lastEdit(env.chat.Id, msgID)
	if _, ok := settingsButtonByMarker(mainKeyboard, "settings_page_welcome"); !ok {
		t.Fatal("the Main page has no Welcome button")
	}
	line, ok := settingsMainStateLine(mainText, "settings_page_welcome")
	if !ok || !strings.Contains(line, staffMarker("common_status_on")) {
		t.Fatalf("Welcome status line = %q (found %v), want it to read On", line, ok)
	}

	// The page lists the three parts, both hints and the switch.
	env.pressRaw(env.chat, owner, msgID, mustSettingsPageButton(t, mainKeyboard, settingsPageWelcome).CallbackData)
	text, keyboard := env.lastEdit(env.chat.Id, msgID)
	for _, want := range []string{
		staffMarker("settings_edit_line_text"), staffMarker("settings_state_default"),
		staffMarker("settings_edit_line_media"), staffMarker("settings_edit_line_buttons"), staffMarker("settings_state_not_set"),
		staffMarker("settings_edit_hint_greeting"), staffMarker("settings_edit_clean_hint_welcome"),
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the Welcome page %q lacks %s", text, want)
		}
	}
	toggle, fields := settingsEditControl(t, keyboard, "settings_btn_edit_welcome_toggle", settingsEditControlEnabled)
	if fields["a"] != settingsActToggle || fields["p"] != settingsPageWelcome || fields["v"] != "0" {
		t.Fatalf("the switch carries %v, want tg on page w with v=0", fields)
	}
	if !strings.Contains(toggle.Text, staffMarker("common_status_on")) {
		t.Errorf("the switch label %q does not show On", toggle.Text)
	}
	if _, ok := settingsEditStored(t, env.chat.Id); ok {
		t.Fatal("opening the page created a greetings row")
	}

	// Pressing it stores what /welcome off stores.
	answers := env.answerCount()
	env.pressRaw(env.chat, owner, msgID, toggle.CallbackData)
	if got := env.answerCount() - answers; got != 1 {
		t.Errorf("the press was answered %d times, want once", got)
	}
	row, ok := settingsEditStored(t, env.chat.Id)
	if !ok || row.WelcomeSettings == nil || row.WelcomeSettings.ShouldWelcome {
		t.Fatalf("stored row = %+v (found %v), want welcome_enabled false", row, ok)
	}
	text, keyboard = env.lastEdit(env.chat.Id, msgID)
	toggle, fields = settingsEditControl(t, keyboard, "settings_btn_edit_welcome_toggle", settingsEditControlEnabled)
	if fields["v"] != "1" || !strings.Contains(toggle.Text, staffMarker("common_status_off")) {
		t.Errorf("after the press the switch is %q with %v, want Off and v=1", toggle.Text, fields)
	}
	if strings.Contains(text, settingsNoticeWarnIcon) {
		t.Errorf("a stored write redrew with a notice: %q", text)
	}

	twin := env.addChat()
	editCleanup(t, twin.Id)
	twinOwner := env.newOwner(twin)
	env.sendIn(twin, twinOwner, "/welcome off")
	requireSameGreetingRow(t, loadGreetingRow(t, twin.Id), loadGreetingRow(t, env.chat.Id))

	// A stale press of the same absolute value writes the same value again.
	stale := encodeCallbackData(settingsCallbackNamespace, env.groupFields(owner, "a", "tg", "p", "w", "t", "e", "v", "0"))
	env.pressRaw(env.chat, owner, msgID, stale)
	if row, _ := settingsEditStored(t, env.chat.Id); row.WelcomeSettings.ShouldWelcome {
		t.Error("a repeated v=0 press turned the welcome back on")
	}

	// v=1 turns it on.
	env.pressRaw(env.chat, owner, msgID, toggle.CallbackData)
	if row, _ := settingsEditStored(t, env.chat.Id); !row.WelcomeSettings.ShouldWelcome {
		t.Error("the v=1 press did not turn the welcome on")
	}
}

// mustSettingsPageButton is settingsPageButton that fails the test when the page has no button.
func mustSettingsPageButton(t *testing.T, keyboard [][]gotgbot.InlineKeyboardButton, pageKey string) gotgbot.InlineKeyboardButton {
	t.Helper()
	button, ok := settingsPageButton(t, keyboard, pageKey)
	if !ok {
		t.Fatalf("no button for page %q", pageKey)
	}
	return button
}
