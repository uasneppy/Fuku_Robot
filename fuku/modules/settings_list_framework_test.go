//go:build testtools

package modules

import (
	"fmt"
	"maps"
	"strconv"
	"strings"
	"testing"
	"time"

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

func TestSettingsCardRowsTo(t *testing.T) {
	v := settingsView{Tr: i18n.MustNewTranslator("en"), OwnerKey: "u", OwnerValue: "7", GroupID: -1001234567890}
	fieldsOf := func(button gotgbot.InlineKeyboardButton) map[string]string {
		decoded, ok := decodeCallbackData(button.CallbackData, settingsCallbackNamespace)
		if !ok {
			t.Fatalf("button %q carries %q, not settings callback data", button.Text, button.CallbackData)
		}
		return decoded.Fields
	}
	wantExpiry := func(e string) {
		t.Helper()
		minute, err := strconv.ParseInt(e, 10, 64)
		if err != nil {
			t.Fatalf("e = %q is not a minute", e)
		}
		if want := time.Now().Add(settingsCardLifetime).Unix() / 60; minute < want-1 || minute > want+1 {
			t.Errorf("e = %d, want about %d (one card lifetime ahead)", minute, want)
		}
	}

	t.Run("the caller chooses the No fields", func(t *testing.T) {
		k := newSettingsKeyboard(v)
		settingsCardRowsTo(k, map[string]string{"a": "zy", "r": "5"}, map[string]string{"a": "zg", "v": "3"})
		if len(k.rows) != 2 || len(k.rows[0]) != 2 || len(k.rows[1]) != 2 {
			t.Fatalf("card rows = %+v, want [Yes No] then Back and Close", k.rows)
		}
		yes, no := fieldsOf(k.rows[0][0]), fieldsOf(k.rows[0][1])
		wantExpiry(yes["e"])
		delete(yes, "e")
		if want := map[string]string{"a": "zy", "r": "5", "u": "7"}; !maps.Equal(yes, want) {
			t.Errorf("Yes fields = %v, want %v plus e", yes, want)
		}
		if want := map[string]string{"a": "zg", "v": "3", "u": "7"}; !maps.Equal(no, want) {
			t.Errorf("No fields = %v, want exactly %v", no, want)
		}
		if got := fieldsOf(k.rows[1][0])["a"]; got != settingsActBack {
			t.Errorf("the navigation row starts with %q, want Back", got)
		}
	})

	t.Run("settingsCardRows keeps its No: cancel back to the page", func(t *testing.T) {
		k := newSettingsKeyboard(v)
		settingsCardRows(k, map[string]string{"a": "zy"}, "l")
		yes, no := fieldsOf(k.rows[0][0]), fieldsOf(k.rows[0][1])
		wantExpiry(yes["e"])
		if want := map[string]string{"a": settingsActCancel, "p": "l", "u": "7"}; !maps.Equal(no, want) {
			t.Errorf("No fields = %v, want %v", no, want)
		}
		if yes["a"] != "zy" || len(k.rows) != 2 {
			t.Errorf("Yes = %v and %d rows, want a=zy and two rows", yes, len(k.rows))
		}
	})

	t.Run("the caller's map is not changed", func(t *testing.T) {
		yes := map[string]string{"a": "zy"}
		settingsCardRowsTo(newSettingsKeyboard(v), yes, map[string]string{"a": "zg"})
		if len(yes) != 1 {
			t.Errorf("the caller's Yes fields became %v", yes)
		}
	})
}

// settingsBotHalvesRow is what the two bot halves answer for one right with the bot in one state.
type settingsBotHalvesRow struct {
	first, after, afterTyped string
}

func TestSettingsRightBotHalves(t *testing.T) {
	const (
		connected  = "connections_is_user_connected_bot_not_admin"
		botNotAdm  = "chat_status_bot_not_admin"
		restrict   = "chat_status_bot_restrict_error"
		restrictGr = "chat_status_bot_restrict_group_error"
	)
	env := newSettingsEnv(t)
	states := []struct {
		name, role string
	}{
		{"the bot is a member", staffRoleMember},
		{"the bot is an administrator without restrict", staffRoleAdminNoRestrict},
		{"the bot is an administrator with restrict", staffRoleAdmin},
	}
	// want[right][state index]. A right with no bot part answers nothing in every state.
	none := settingsBotHalvesRow{}
	want := map[settingsRight][3]settingsBotHalvesRow{
		settingsRightAdmin:               {none, none, none},
		settingsRightAdminBotAdmin:       {{"", botNotAdm, botNotAdm}, none, none},
		settingsRightAdminFloodBot:       {{"", connected, connected}, none, none},
		settingsRightRestrict:            {none, none, none},
		settingsRightOwner:               {none, none, none},
		settingsRightChangeInfo:          {none, none, none},
		settingsRightChangeInfoBotAdmin:  {{connected, "", ""}, none, none},
		settingsRightOwnerBotAdmin:       {{botNotAdm, "", ""}, none, none},
		settingsRightRestrictBotRestrict: {{connected, restrict, restrictGr}, {"", restrict, restrictGr}, none},
	}
	if len(want) != 9 {
		t.Fatalf("the table covers %d rights, want all nine", len(want))
	}
	for right, rows := range want {
		for i, state := range states {
			t.Run(fmt.Sprintf("right %d, %s", right, state.name), func(t *testing.T) {
				chat := env.addChat()
				env.fake.setBotRole(chat.Id, state.role)
				got := settingsBotHalvesRow{
					first:      settingsRightBotFirst(env.bot, &chat, right),
					after:      settingsRightBotAfter(env.bot, &chat, right, false),
					afterTyped: settingsRightBotAfter(env.bot, &chat, right, true),
				}
				if got != rows[i] {
					t.Errorf("halves = %+v, want %+v", got, rows[i])
				}
			})
		}
	}
}
