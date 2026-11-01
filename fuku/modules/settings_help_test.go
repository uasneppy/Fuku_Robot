//go:build testtools

package modules

import (
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2/ext"
)

// TestLoadSettingsRegistersHelp pins the user-visible integration: a registered command
// alone does not put a module in the /help menu, and the name an admin types after /help
// must resolve to the Settings page, whose text is the module's settings_help_msg.
func TestLoadSettingsRegistersHelp(t *testing.T) {
	withRealLocales(t)

	registry := DefaultHelpRegistry()
	previousEnabled, hadEnabled := registry.AbleMap[settingsModule.moduleName]
	t.Cleanup(func() {
		if hadEnabled {
			registry.AbleMap[settingsModule.moduleName] = previousEnabled
			return
		}
		delete(registry.AbleMap, settingsModule.moduleName)
	})
	delete(registry.AbleMap, settingsModule.moduleName)

	LoadSettings(ext.NewDispatcher(&ext.DispatcherOpts{MaxRoutines: -1}))

	if !registry.AbleMap[settingsModule.moduleName] {
		t.Fatalf("%s is not enabled in the help registry after LoadSettings", settingsModule.moduleName)
	}

	var button bool
	for _, row := range initHelpButtonsFrom(registry).InlineKeyboard {
		for _, each := range row {
			button = button || each.Text == settingsModule.moduleName
		}
	}
	if !button {
		t.Fatalf("%s is missing from the /help menu", settingsModule.moduleName)
	}

	if got := getModuleNameFromAltName("settings", registry); got != settingsModule.moduleName {
		t.Fatalf("/help settings resolves to %q, want %q", got, settingsModule.moduleName)
	}

	text, _ := getModuleHelpAndKb(settingsModule.moduleName, "en", registry)
	for _, want := range []string{"/settings", "/setfloodmode"} {
		if !strings.Contains(text, want) {
			t.Errorf("help text for %s lacks %q:\n%s", settingsModule.moduleName, want, text)
		}
	}
}
