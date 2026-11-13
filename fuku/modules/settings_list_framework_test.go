package modules

import (
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// The tests of the three framework pieces plan 19-02 adds for the list pages: PrivateOnly (M39),
// settingsCardRowsTo (M40) and the ninth right with its bot halves (M42). The list pages themselves
// come in later plans, so PrivateOnly is exercised through a page this file registers.

// privateOnlyProbe counts what a test page's code did.
type privateOnlyProbe struct {
	mainState, open int
}

// registerPrivateOnlyProbe registers a PrivateOnly page that counts every call of its code and
// removes it again when the test ends.
func registerPrivateOnlyProbe(t *testing.T, key string, privateOnly bool) *privateOnlyProbe {
	t.Helper()
	probe := &privateOnlyProbe{}
	registerSettingsPage(settingsPage{
		Key:         key,
		Order:       9999,
		Icon:        "🧪",
		TitleKey:    "settings_page_locks",
		PrivateOnly: privateOnly,
		MainState: func(*gotgbot.Bot, *i18n.Translator, *gotgbot.Chat) (string, bool) {
			probe.mainState++
			return "PAGESTATE", true
		},
		Open: func(p *settingsPress, notice string) (settingsScreen, error) {
			probe.open++
			k := newSettingsKeyboard(p.view())
			k.navRow(false)
			return k.screen(settingsCompose(p.view(), "settings_page_locks", []string{"PAGEOPENED"}, notice, false)), nil
		},
	})
	t.Cleanup(func() { delete(settingsPages, key) })
	return probe
}

// setCachedBotUsername pins the bot username the Open in private row reads and restores it.
func setCachedBotUsername(t *testing.T, name string) {
	t.Helper()
	cachedBotUsernameMu.Lock()
	cachedBotUsername = name
	cachedBotUsernameMu.Unlock()
	t.Cleanup(resetCachedBotUsername)
}

func settingsURLButtons(keyboard [][]gotgbot.InlineKeyboardButton) []string {
	var urls []string
	for _, button := range settingsAllButtons(keyboard) {
		if button.Url != "" {
			urls = append(urls, button.Url)
		}
	}
	return urls
}

func TestSettingsPrivateOnlyPage(t *testing.T) {
	const key = "zp"
	setCachedBotUsername(t, "FukuTestBot")

	t.Run("a group menu shows the fixed state and the note and runs no page code", func(t *testing.T) {
		probe := registerPrivateOnlyProbe(t, key, true)
		env := newSettingsEnv(t)
		msgID := env.openMenu(env.admin)

		text, _ := env.lastEdit(env.chat.Id, msgID)
		if !strings.Contains(text, staffMarker("settings_state_private_only")) {
			t.Errorf("a group Main %q lacks the fixed private-only state", text)
		}
		if strings.Contains(text, "PAGESTATE") {
			t.Errorf("a group Main shows the page's own state: %q", text)
		}

		env.press(env.admin, msgID, env.groupFields(env.admin, "a", settingsActOpen, "p", key))
		text, keyboard := env.lastEdit(env.chat.Id, msgID)
		if !strings.Contains(text, staffMarker("settings_entries_group_note")) || strings.Contains(text, "PAGEOPENED") {
			t.Errorf("the group page drew %q, want the note alone", text)
		}
		if urls, want := settingsURLButtons(keyboard), settingsPrivateURL("FukuTestBot", env.chat.Id); len(urls) != 1 || urls[0] != want {
			t.Errorf("Open in private URL buttons = %v, want [%s]", urls, want)
		}
		if got, want := len(keyboard), 2; got != want {
			t.Errorf("the note has %d rows, want %d (Open in private, navigation)", got, want)
		}
		if last := keyboard[len(keyboard)-1]; len(last) != 2 || !strings.Contains(last[0].Text, staffMarker("settings_btn_back")) {
			t.Errorf("the note ends with %+v, want Back and Close", last)
		}

		// Refresh-like reopenings and Back to the page draw the same note, and Back from it draws Main.
		env.press(env.admin, msgID, env.groupFields(env.admin, "a", settingsActBack))
		if text, _ := env.lastEdit(env.chat.Id, msgID); !strings.Contains(text, staffMarker("settings_header_main")) {
			t.Errorf("Back from the note drew %q, want Main", text)
		}
		if probe.mainState != 0 || probe.open != 0 {
			t.Errorf("a group menu ran the page's code: MainState %d times, Open %d times, want none", probe.mainState, probe.open)
		}
	})

	t.Run("without a bot username the URL row is left out", func(t *testing.T) {
		setCachedBotUsername(t, "")
		v := settingsView{Tr: i18n.MustNewTranslator("en"), GroupID: -1001234567890, GroupTitle: "Group"}
		screen := renderSettingsPrivateOnlyNote(v, "settings_page_locks", "")
		if urls := settingsURLButtons(screen.Keyboard.InlineKeyboard); len(urls) != 0 {
			t.Errorf("a note without a bot username has URL buttons %v", urls)
		}
		if got := len(screen.Keyboard.InlineKeyboard); got != 1 {
			t.Errorf("a note without a bot username has %d rows, want the navigation row alone", got)
		}
	})

	t.Run("a private menu calls the page's own code", func(t *testing.T) {
		probe := registerPrivateOnlyProbe(t, key, true)
		env := newSettingsEnv(t)
		const privateMsg = 5601

		env.pressPrivate(env.admin, privateMsg, env.privateFields(env.chat.Id, "a", settingsActBack))
		text, _ := env.lastEdit(env.admin.Id, privateMsg)
		if !strings.Contains(text, "PAGESTATE") || strings.Contains(text, staffMarker("settings_state_private_only")) {
			t.Errorf("a private Main %q should show the page's own state", text)
		}
		if probe.mainState != 1 {
			t.Errorf("a private Main called MainState %d times, want once", probe.mainState)
		}

		env.pressPrivate(env.admin, privateMsg, env.privateFields(env.chat.Id, "a", settingsActOpen, "p", key))
		text, keyboard := env.lastEdit(env.admin.Id, privateMsg)
		if !strings.Contains(text, "PAGEOPENED") || strings.Contains(text, staffMarker("settings_entries_group_note")) {
			t.Errorf("the private page drew %q, want the page's own screen", text)
		}
		if probe.open != 1 {
			t.Errorf("the private press called Open %d times, want once", probe.open)
		}
		if urls := settingsURLButtons(keyboard); len(urls) != 0 {
			t.Errorf("the private page has URL buttons %v", urls)
		}
	})

	t.Run("a page without PrivateOnly is drawn by its own code in both menus", func(t *testing.T) {
		probe := registerPrivateOnlyProbe(t, key, false)
		env := newSettingsEnv(t)
		msgID := env.openMenu(env.admin)
		if text, _ := env.lastEdit(env.chat.Id, msgID); !strings.Contains(text, "PAGESTATE") {
			t.Errorf("a group Main %q should show an ordinary page's state", text)
		}
		env.press(env.admin, msgID, env.groupFields(env.admin, "a", settingsActOpen, "p", key))
		if text, _ := env.lastEdit(env.chat.Id, msgID); !strings.Contains(text, "PAGEOPENED") {
			t.Errorf("an ordinary page drew %q in a group menu, want its own screen", text)
		}
		if probe.mainState != 1 || probe.open != 1 {
			t.Errorf("an ordinary page ran MainState %d and Open %d times in a group menu, want once each", probe.mainState, probe.open)
		}
	})
}
