package modules

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/helpers"
)

// helpModuleLabelMax is the longest translated module name, in code points, so three
// buttons fit one row of the /help keyboard.
const helpModuleLabelMax = 14

// withFreshHelpRegistry swaps in an empty help registry for the test's lifetime.
func withFreshHelpRegistry(t *testing.T) *moduleStruct {
	t.Helper()
	previous := defaultHelpRegistry
	defaultHelpRegistry = newHelpRegistry()
	t.Cleanup(func() { defaultHelpRegistry = previous })
	return defaultHelpRegistry
}

// TestHelpModuleNamesExist loads every module and holds that each help module has a
// translated name in all 8 languages, short enough for a three-button row.
func TestHelpModuleNamesExist(t *testing.T) {
	withRealLocales(t)
	registry := withFreshHelpRegistry(t)
	// Loading every module appends to the disableable list again; put it back.
	previousDisable := append([]string(nil), helpers.DisableCmds...)
	t.Cleanup(func() { helpers.DisableCmds = previousDisable })
	LoadAllModules(ext.NewDispatcher(&ext.DispatcherOpts{MaxRoutines: -1}))

	modules := listModulesFrom(registry)
	if len(modules) < 20 {
		t.Fatalf("help modules = %v, want every module loaded", modules)
	}
	for _, code := range []string{"uk", "en", "es", "fr", "hi", "id", "pt", "ru"} {
		tr := i18n.MustNewTranslator(code)
		for _, module := range modules {
			label, err := tr.GetString(helpModuleNameKey(module))
			if err != nil || strings.TrimSpace(label) == "" {
				t.Errorf("%s: %s has no %s", code, module, helpModuleNameKey(module))
				continue
			}
			if n := utf8.RuneCountInString(label); n > helpModuleLabelMax {
				t.Errorf("%s: %s label %q is %d code points, want at most %d", code, module, label, n, helpModuleLabelMax)
			}
			if label != strings.TrimSpace(label) || strings.ContainsAny(label, "<>&*`") {
				t.Errorf("%s: %s label %q carries markup or spaces at an end", code, module, label)
			}
		}
	}
}

// TestHelpModuleLabelFallsBack holds that a module with no name key shows its
// internal name.
func TestHelpModuleLabelFallsBack(t *testing.T) {
	withRealLocales(t)
	if got := helpModuleLabel(i18n.MustNewTranslator("uk"), "NoSuchModule"); got != "NoSuchModule" {
		t.Fatalf("label = %q, want the internal name", got)
	}
	if got := helpModuleLabel(i18n.MustNewTranslator("uk"), "Bans"); got != "Бани" {
		t.Fatalf("label = %q, want Бани", got)
	}
}

// TestHelpTranslatedNameResolves holds that /help takes the translated name of the
// reader's language beside the internal and alt names.
func TestHelpTranslatedNameResolves(t *testing.T) {
	withRealLocales(t)
	registry := withFreshHelpRegistry(t)
	registry.AbleMap["Bans"] = true
	registry.AbleMap["LogChannels"] = true

	for _, tc := range []struct{ typed, lang, want string }{
		{"бани", "uk", "Bans"},
		{"Бани", "uk", "Bans"},
		{"bans", "uk", "Bans"},
		{"bans", "en", "Bans"},
		{"ban", "en", "Bans"},
		{"log channel", "en", "LogChannels"},
		{"logchannel", "en", "LogChannels"},
		{"канал журналу", "uk", "LogChannels"},
		{"бани", "en", ""},
		{"nothing", "uk", ""},
	} {
		if got := findHelpModule(tc.typed, tc.lang, registry); got != tc.want {
			t.Errorf("findHelpModule(%q, %s) = %q, want %q", tc.typed, tc.lang, got, tc.want)
		}
	}
}

// TestHelpTranslatedButtonOpensModule presses the Ukrainian Bans button of the /help
// keyboard and holds that it opens the Bans page with the translated header and Back
// and Home in the reader's language; /help бани sends the same page.
func TestHelpTranslatedButtonOpensModule(t *testing.T) {
	withRealLocales(t)
	registry := withFreshHelpRegistry(t)
	registry.AbleMap["Bans"] = true
	registry.AbleMap["Warns"] = true

	kb := initHelpButtonsFrom(registry, "uk")
	var data string
	for _, row := range kb.InlineKeyboard {
		for _, button := range row {
			if button.Text == "Бани" {
				data = button.CallbackData
			}
		}
	}
	if data == "" {
		t.Fatalf("Ukrainian /help keyboard = %#v, want a Бани button", kb.InlineKeyboard)
	}

	client := newModuleBotClient()
	bot := newModuleTestBot(client)
	user := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Helper"}
	chat := gotgbot.Chat{Id: user.Id, Type: "private", FirstName: "Helper"}
	if err := registry.helpButtonHandler(bot, newModuleCallbackContext(bot, chat, user, data)); err != ext.EndGroups {
		t.Fatalf("helpButtonHandler = %v, want EndGroups", err)
	}
	if err := registry.help(bot, newModuleMessageContext(bot, chat, user, "/help бани")); err != ext.EndGroups {
		t.Fatalf("help = %v, want EndGroups", err)
	}

	edit := client.callsFor("editMessageText")
	sent := client.callsFor("sendMessage")
	if len(edit) != 1 || len(sent) != 1 {
		t.Fatalf("edits = %d, sends = %d, want one of each", len(edit), len(sent))
	}
	editText, _ := edit[0].Params["text"].(string)
	sendText, _ := sent[0].Params["text"].(string)
	if !strings.Contains(editText, "Бани") || editText != sendText {
		t.Fatalf("button page = %q, /help бани page = %q, want the same Bans page with the Ukrainian name", editText, sendText)
	}
	kbText := strings.ToLower(strings.Join(buttonTexts(edit[0].Params["reply_markup"]), "|"))
	if !strings.Contains(kbText, "назад") || !strings.Contains(kbText, "головна") {
		t.Fatalf("page keyboard = %s, want Back and Home in Ukrainian", kbText)
	}
}

// buttonTexts lists the labels of a recorded reply markup.
func buttonTexts(raw any) []string {
	var out []string
	switch kb := raw.(type) {
	case gotgbot.InlineKeyboardMarkup:
		for _, row := range kb.InlineKeyboard {
			for _, b := range row {
				out = append(out, b.Text)
			}
		}
	case *gotgbot.InlineKeyboardMarkup:
		if kb != nil {
			return buttonTexts(*kb)
		}
	}
	return out
}
