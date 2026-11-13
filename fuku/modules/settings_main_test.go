//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// settingsMainStateLine returns the status line of the page whose title is the locale key
// titleKey in a Main page's text (marker locale), and whether there is one.
func settingsMainStateLine(text, titleKey string) (string, bool) {
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, staffMarker("settings_main_state_line")) && strings.Contains(line, staffMarker(titleKey)) {
			return line, true
		}
	}
	return "", false
}

// The real Main's shape for the owner, as every plan that registers a page leaves it (Phase 18:
// the tests that read the real Main compare with these, so a later plan edits only here). The
// private shape ends with the Groups and Close row; the hand-built subtests of
// TestSettingsMainGrid keep their own literals.
const (
	settingsMainShapeGroup   = "s|l / c|f / n|k / b|w / g|r / m|d / t|u / o|y / a / close"
	settingsMainShapePrivate = "s|l / c|f / n|k / b|w / g|r / m|d / t|u / o|y / a / groups|close"
	settingsMainOwnerRows    = 10
)

// settingsMainShape writes a Main keyboard as its rows of short names: a page button is its p
// value, the Groups button "groups" and Close "close", the buttons of a row joined by "|".
func settingsMainShape(t *testing.T, keyboard [][]gotgbot.InlineKeyboardButton) string {
	t.Helper()
	rows := make([]string, 0, len(keyboard))
	for _, row := range keyboard {
		names := make([]string, 0, len(row))
		for _, button := range row {
			if strings.HasPrefix(button.CallbackData, settingsHubNamespace+"|") {
				names = append(names, "groups")
				continue
			}
			fields := settingsButtonFields(t, button)
			switch fields["a"] {
			case settingsActOpen:
				names = append(names, fields["p"])
			case settingsActClose:
				names = append(names, "close")
			default:
				names = append(names, "?"+fields["a"])
			}
		}
		rows = append(rows, strings.Join(names, "|"))
	}
	return strings.Join(rows, " / ")
}

// settingsMainTestEntry is a Main entry whose title is the marker of a page title key.
func settingsMainTestEntry(tr *i18n.Translator, key, icon, titleKey string, ownerOnly bool) settingsMainEntry {
	return settingsMainEntry{
		Key: key, Icon: icon, Title: settingsText(tr, titleKey),
		State: staffMarker("state_of_" + key), Known: true, OwnerOnly: ownerOnly,
	}
}

func TestSettingsMainGrid(t *testing.T) {
	withStaffLocale(t)
	tr := i18n.MustNewTranslator("en")
	groupView := settingsView{
		Tr: tr, OwnerKey: "u", OwnerValue: "9999999999",
		GroupID: -1001234567890, GroupTitle: "G", OpenerName: "Opener",
	}
	privateView := settingsView{
		Tr: tr, Private: true, OwnerKey: "c", OwnerValue: "-1001234567890",
		GroupID: -1001234567890, GroupTitle: "G", OpenerName: "Opener",
	}
	staffE := settingsMainTestEntry(tr, "s", "👥", "settings_page_staff", false)
	lockE := settingsMainTestEntry(tr, "l", "🔒", "settings_page_lockdown", false)
	captchaE := settingsMainTestEntry(tr, "c", "🧩", "settings_page_captcha", false)
	floodE := settingsMainTestEntry(tr, "f", "🌊", "settings_page_antiflood", false)
	accessE := settingsMainTestEntry(tr, "a", "🔑", "settings_page_access", true)

	hint := staffMarker("settings_main_hint")
	lines := func(entries ...settingsMainEntry) string {
		out := make([]string, 0, len(entries))
		for _, e := range entries {
			out = append(out, fmt.Sprintf("%s %s %s %s", staffMarker("settings_main_state_line"), e.Icon, e.Title, e.State))
		}
		return strings.Join(out, "\n")
	}

	t.Run("the owner's private grid, status block and navigation", func(t *testing.T) {
		screen := renderSettingsMain(privateView, []settingsMainEntry{staffE, lockE, captchaE, floodE, accessE}, "")
		if got, want := settingsMainShape(t, screen.Keyboard.InlineKeyboard), "s|l / c|f / a / groups|close"; got != want {
			t.Errorf("keyboard = %q, want %q", got, want)
		}
		for _, e := range []settingsMainEntry{staffE, lockE, captchaE, floodE, accessE} {
			var found bool
			for _, button := range settingsAllButtons(screen.Keyboard.InlineKeyboard) {
				if button.Text == e.Icon+" "+e.Title {
					found = true
				}
				if strings.Contains(button.Text, e.State) {
					t.Errorf("button %q carries the state %q: the state belongs in the body", button.Text, e.State)
				}
			}
			if !found {
				t.Errorf("no button labelled %q", e.Icon+" "+e.Title)
			}
		}
		want := hint + "\n\n" + lines(staffE, lockE, captchaE, floodE, accessE)
		if !strings.Contains(screen.Text, want) {
			t.Errorf("text %q does not contain the hint, one blank line and the five status lines Access last:\n%s", screen.Text, want)
		}
		if strings.Contains(screen.Text, staffMarker("settings_main_owner")) {
			t.Errorf("a private Main carries the owner line: %q", screen.Text)
		}
		if screen.Omitted != 0 {
			t.Errorf("Omitted = %d, want 0", screen.Omitted)
		}
	})

	t.Run("the same owner in a group menu", func(t *testing.T) {
		screen := renderSettingsMain(groupView, []settingsMainEntry{staffE, lockE, captchaE, floodE, accessE}, "")
		if got, want := settingsMainShape(t, screen.Keyboard.InlineKeyboard), "s|l / c|f / a / close"; got != want {
			t.Errorf("keyboard = %q, want %q", got, want)
		}
		wantStart := hint + "\n" + staffMarker("settings_main_owner")
		if !strings.Contains(screen.Text, wantStart) || !strings.Contains(screen.Text, "\n\n"+lines(staffE, lockE, captchaE, floodE, accessE)) {
			t.Errorf("text %q: want the hint, the owner line, a blank line and the status block", screen.Text)
		}
		for _, button := range settingsAllButtons(screen.Keyboard.InlineKeyboard) {
			if strings.HasPrefix(button.CallbackData, settingsHubNamespace+"|") {
				t.Errorf("a group menu's Main carries a hub button: %q", button.CallbackData)
			}
		}
	})

	t.Run("nine grid cells keep seven rows, the odd last cell alone", func(t *testing.T) {
		entries := []settingsMainEntry{staffE, lockE, captchaE, floodE}
		for _, e := range []struct{ key, icon, title string }{
			{"w", "🎉", "settings_page_welcome"}, {"g", "👋", "settings_page_goodbye"},
			{"r", "📜", "settings_page_rules"}, {"m", "🛂", "settings_page_mreq"}, {"d", "🧹", "settings_page_del"},
		} {
			entries = append(entries, settingsMainTestEntry(tr, e.key, e.icon, e.title, false))
		}
		entries = append(entries, accessE)
		for name, view := range map[string]settingsView{"private": privateView, "group": groupView} {
			screen := renderSettingsMain(view, entries, "")
			want := "s|l / c|f / w|g / r|m / d / a / close"
			if view.Private {
				want = "s|l / c|f / w|g / r|m / d / a / groups|close"
			}
			if got := settingsMainShape(t, screen.Keyboard.InlineKeyboard); got != want {
				t.Errorf("%s keyboard = %q, want %q", name, got, want)
			}
			if rows := len(screen.Keyboard.InlineKeyboard); rows != 7 {
				t.Errorf("%s Main has %d rows, want 7", name, rows)
			}
		}
	})

	t.Run("fifteen grid cells keep eight rows, the odd last cell alone, in the UI-SPEC's order", func(t *testing.T) {
		// The pages of the final menu by their real keys, in the order the contract fixes: eight
		// rows of the grid (seven pairs and the Anonymous admins cell alone), Access, navigation.
		order := []string{
			settingsPageStaff, settingsPageLockdown, settingsPageCaptcha, settingsPageAntiflood,
			settingsPageWarns, settingsPageLocks, settingsPageWelcome, settingsPageGoodbye,
			settingsPageRules, settingsPageMreq, settingsPageDeletion, settingsPageSwitches,
			settingsPageLang, settingsPageLog, settingsPageAnon,
		}
		var entries []settingsMainEntry
		for _, key := range order {
			pg, ok := settingsPageByKey(key)
			if !ok {
				t.Fatalf("page %q is not registered", key)
			}
			entries = append(entries, settingsMainTestEntry(tr, key, pg.Icon, pg.TitleKey, false))
		}
		entries = append(entries, accessE)
		for name, view := range map[string]settingsView{"private": privateView, "group": groupView} {
			screen := renderSettingsMain(view, entries, "")
			nav := "close"
			if view.Private {
				nav = "groups|close"
			}
			if got, want := settingsMainShape(t, screen.Keyboard.InlineKeyboard), "s|l / c|f / n|k / w|g / r|m / d|t / u|o / y / a / "+nav; got != want {
				t.Errorf("%s owner keyboard = %q, want %q", name, got, want)
			}
			if rows := len(screen.Keyboard.InlineKeyboard); rows != 10 {
				t.Errorf("%s owner Main has %d rows, want 10", name, rows)
			}
			if got := strings.Count(screen.Text, staffMarker("settings_main_state_line")); got != 16 {
				t.Errorf("%s owner Main has %d state lines, want 16", name, got)
			}
			if screen.Omitted != 0 {
				t.Errorf("%s Main omitted %d buttons", name, screen.Omitted)
			}
			admin := renderSettingsMain(view, entries[:len(order)], "")
			if rows := len(admin.Keyboard.InlineKeyboard); rows != 9 {
				t.Errorf("%s admin Main has %d rows, want 9", name, rows)
			}
			if got := settingsMainShape(t, admin.Keyboard.InlineKeyboard); got != "s|l / c|f / n|k / w|g / r|m / d|t / u|o / y / "+nav {
				t.Errorf("%s admin keyboard = %q", name, got)
			}
			// Telegram's own limits: at most 100 buttons and 8 to a row.
			for _, row := range screen.Keyboard.InlineKeyboard {
				if len(row) > 8 {
					t.Errorf("%s has a row of %d buttons", name, len(row))
				}
			}
		}
	})

	t.Run("an allowed admin has no Access row and no Access line", func(t *testing.T) {
		for name, view := range map[string]settingsView{"private": privateView, "group": groupView} {
			screen := renderSettingsMain(view, []settingsMainEntry{staffE, lockE, captchaE, floodE}, "")
			want := "s|l / c|f / close"
			if view.Private {
				want = "s|l / c|f / groups|close"
			}
			if got := settingsMainShape(t, screen.Keyboard.InlineKeyboard); got != want {
				t.Errorf("%s keyboard = %q, want %q", name, got, want)
			}
			if _, ok := settingsMainStateLine(screen.Text, "settings_page_access"); ok {
				t.Errorf("%s: an admin's Main carries an Access line: %q", name, screen.Text)
			}
		}
	})

	t.Run("an odd count leaves the last page alone, then the Access row", func(t *testing.T) {
		screen := renderSettingsMain(privateView, []settingsMainEntry{staffE, lockE, captchaE, accessE}, "")
		if got, want := settingsMainShape(t, screen.Keyboard.InlineKeyboard), "s|l / c / a / groups|close"; got != want {
			t.Errorf("keyboard = %q, want %q", got, want)
		}
		screen = renderSettingsMain(groupView, []settingsMainEntry{staffE}, "")
		if got, want := settingsMainShape(t, screen.Keyboard.InlineKeyboard), "s / close"; got != want {
			t.Errorf("one page: keyboard = %q, want %q", got, want)
		}
	})

	t.Run("the owner's grid differs from an admin's by the Access row only", func(t *testing.T) {
		withAccess := renderSettingsMain(groupView, []settingsMainEntry{staffE, lockE, captchaE, floodE, accessE}, "")
		without := renderSettingsMain(groupView, []settingsMainEntry{staffE, lockE, captchaE, floodE}, "")
		if len(withAccess.Keyboard.InlineKeyboard) != len(without.Keyboard.InlineKeyboard)+1 {
			t.Fatalf("rows %d and %d, want exactly one more for the owner", len(withAccess.Keyboard.InlineKeyboard), len(without.Keyboard.InlineKeyboard))
		}
		for i, row := range without.Keyboard.InlineKeyboard[:2] {
			for j, button := range row {
				if got := withAccess.Keyboard.InlineKeyboard[i][j]; got != button {
					t.Errorf("grid button %d,%d differs: %+v vs %+v", i, j, got, button)
				}
			}
		}
	})

	t.Run("a page whose state could not be read keeps its line with the unknown mark", func(t *testing.T) {
		unknown := floodE
		unknown.Known = false
		screen := renderSettingsMain(groupView, []settingsMainEntry{staffE, unknown}, "")
		line, ok := settingsMainStateLine(screen.Text, "settings_page_antiflood")
		if !ok || !strings.HasSuffix(line, settingsUnknownState) {
			t.Errorf("antiflood line = %q (found %v), want it to end with the unknown mark", line, ok)
		}
		if strings.Contains(line, floodE.State) {
			t.Errorf("antiflood line %q shows a state that was not read", line)
		}
	})

	t.Run("a notice follows the status block", func(t *testing.T) {
		screen := renderSettingsMain(groupView, []settingsMainEntry{staffE}, "NOTICE")
		if !strings.HasSuffix(screen.Text, lines(staffE)+"\n\nNOTICE") {
			t.Errorf("text %q does not end with the status block and the notice", screen.Text)
		}
	})

	t.Run("no visible page still renders", func(t *testing.T) {
		screen := renderSettingsMain(privateView, nil, "")
		if got, want := settingsMainShape(t, screen.Keyboard.InlineKeyboard), "groups|close"; got != want {
			t.Errorf("private keyboard = %q, want %q", got, want)
		}
		if !strings.HasSuffix(screen.Text, hint) || strings.Contains(screen.Text, staffMarker("settings_main_state_line")) {
			t.Errorf("private text %q: want it to end with the hint and carry no status block", screen.Text)
		}
		screen = renderSettingsMain(groupView, nil, "")
		if got, want := settingsMainShape(t, screen.Keyboard.InlineKeyboard), "close"; got != want {
			t.Errorf("group keyboard = %q, want %q", got, want)
		}
		if !strings.HasSuffix(screen.Text, hint+"\n"+staffMarker("settings_main_owner")+" Opener") {
			t.Errorf("group text %q carries a blank line for a status block that is not there", screen.Text)
		}
	})

	t.Run("end to end, the owner in the group and in private, an admin in the group", func(t *testing.T) {
		env := newSettingsEnv(t)
		var visible, ordinary []string
		for _, pg := range settingsPagesInOrder() {
			visible = append(visible, pg.Key)
			if !pg.OwnerOnly {
				ordinary = append(ordinary, pg.Key)
			}
		}
		// The grid the registry gives an owner and the one it gives an admin.
		wantShape := func(keys []string, nav string) string {
			var rows []string
			var grid, anchored []string
			for _, pg := range settingsPagesInOrder() {
				if pg.OwnerOnly {
					anchored = append(anchored, pg.Key)
				} else if slices.Contains(keys, pg.Key) {
					grid = append(grid, pg.Key)
				}
			}
			for i := 0; i < len(grid); i += 2 {
				rows = append(rows, strings.Join(grid[i:min(i+2, len(grid))], "|"))
			}
			if len(keys) > len(grid) {
				rows = append(rows, anchored...)
			}
			return strings.Join(append(rows, nav), " / ")
		}

		owner := env.newOwner(env.chat)
		msgID := env.openMenu(owner)
		text, keyboard := env.lastEdit(env.chat.Id, msgID)
		if got, want := settingsMainShape(t, keyboard), wantShape(visible, "close"); got != want {
			t.Errorf("owner's group Main = %q, want %q", got, want)
		}
		if _, ok := settingsMainStateLine(text, "settings_page_access"); !ok {
			t.Errorf("the owner's Main has no Access line: %q", text)
		}
		for _, key := range []string{"settings_page_staff", "settings_page_lockdown", "settings_page_captcha", "settings_page_antiflood"} {
			if _, ok := settingsMainStateLine(text, key); !ok {
				t.Errorf("the owner's Main has no %s line: %q", key, text)
			}
		}

		env.sendPrivate(owner, fmt.Sprintf("/start settings_%d", env.chat.Id))
		sent := settingsPrivateOnlySent(t, env, owner)
		if got, want := settingsMainShape(t, staffKeyboardOf(sent.Params["reply_markup"])), wantShape(visible, "groups|close"); got != want {
			t.Errorf("owner's private Main = %q, want %q", got, want)
		}

		env.setAdminsAllowed(env.chat.Id, true)
		adminMsg := env.openMenu(env.admin)
		adminText, adminKeyboard := env.lastEdit(env.chat.Id, adminMsg)
		if got, want := settingsMainShape(t, adminKeyboard), wantShape(ordinary, "close"); got != want {
			t.Errorf("admin's group Main = %q, want %q", got, want)
		}
		if _, ok := settingsMainStateLine(adminText, "settings_page_access"); ok {
			t.Errorf("the admin's Main has an Access line: %q", adminText)
		}
	})
}

// settingsMainWorstState is the longest state text a page can show on Main in the translator's
// language, so the fit test holds every page to the worst case.
func settingsMainWorstState(tr *i18n.Translator) string {
	candidates := []string{
		settingsText(tr, "settings_state_linked"),
		settingsText(tr, "settings_state_not_linked"),
		settingsText(tr, "settings_state_staff_n", i18n.TranslationParams{"count": 99999}),
		settingsText(tr, "settings_state_lock_none"),
		settingsText(tr, "settings_state_lock_active"),
		settingsText(tr, "settings_state_lock_lifting"),
		settingsText(tr, "settings_state_limit", i18n.TranslationParams{"n": 99999}),
		settingsText(tr, "settings_state_off_plain"),
		settingsText(tr, "settings_state_del_on", i18n.TranslationParams{"n": 99}),
		settingsText(tr, "settings_state_access_on"),
		settingsText(tr, "settings_state_access_off"),
		settingsText(tr, "settings_state_set"),
		settingsText(tr, "settings_state_not_set"),
		settingsStateWord(tr, true),
		settingsStateWord(tr, false),
		// Phase 18: the Switches and Locks counts at their largest, the Anonymous admins and Log
		// channel words, and every Warnings action at the largest limit.
		settingsText(tr, "settings_state_sw_on", i18n.TranslationParams{"n": 3}),
		settingsText(tr, "settings_state_locks_on", i18n.TranslationParams{"n": len(settingsLockRows)}),
		settingsText(tr, "common_status_on"),
		settingsText(tr, "common_status_off"),
		// Phase 19: a list page's fixed state in a group menu and its largest private count (a capped list
		// reads n/150, digits only, so five digits is the widest).
		settingsText(tr, "settings_state_private_only"),
		"99999",
	}
	for _, mode := range settingsWarnsStoredModes {
		candidates = append(candidates, settingsText(tr, "settings_state_warns", i18n.TranslationParams{
			"n": settingsWarnsMaxLimit, "action": settingsWarnsActionWord(tr, mode),
		}))
	}
	// The Language line is the stored language by its own name and flag: the longest of the eight.
	for _, code := range groupLanguageCodes {
		candidates = append(candidates, settingsLangDisplay(code))
	}
	worst := ""
	for _, c := range candidates {
		if utf8.RuneCountInString(c) > utf8.RuneCountInString(worst) {
			worst = c
		}
	}
	return worst
}

func TestSettingsMainPageFitsEveryLocale(t *testing.T) {
	withRealLocales(t)
	titles := map[string]string{
		"ampersands": strings.Repeat("&", 64),
		"astral":     strings.Repeat("😀", 64),
		"multibyte":  strings.Repeat("Ж", 64),
	}
	for _, code := range []string{"uk", "en", "es", "fr", "hi", "id", "pt", "ru"} {
		t.Run(code, func(t *testing.T) {
			tr := i18n.MustNewTranslator(code)
			state := settingsMainWorstState(tr)
			var entries []settingsMainEntry
			for _, pg := range settingsPagesInOrder() {
				entries = append(entries, settingsMainEntry{
					Key: pg.Key, Icon: pg.Icon, Title: settingsText(tr, pg.TitleKey),
					State: state, Known: true, OwnerOnly: pg.OwnerOnly,
				})
			}
			if len(entries) < 5 {
				t.Fatalf("only %d pages are registered, want the four ordinary ones and Access", len(entries))
			}
			for name, title := range titles {
				for _, private := range []bool{false, true} {
					view := settingsView{
						Tr: tr, Private: private, OwnerKey: "u", OwnerValue: "9999999999",
						GroupID: -1001234567890, GroupTitle: title, OpenerName: title,
					}
					if private {
						view.OwnerKey, view.OwnerValue = "c", "-1001234567890"
					}
					label := fmt.Sprintf("%s %s private=%v", code, name, private)
					screen := renderSettingsMain(view, entries, settingsNoticeWarn(settingsText(tr, "settings_check_failed")))
					if screen.Omitted != 0 {
						t.Errorf("%s omitted %d buttons", label, screen.Omitted)
					}
					if got := panelUTF16Len(screen.Text); got > 3800 || got == 0 {
						t.Errorf("%s is %d UTF-16 units, want 1..3800", label, got)
					}
					for _, button := range settingsAllButtons(screen.Keyboard.InlineKeyboard) {
						runes := utf8.RuneCountInString(button.Text)
						switch {
						case strings.TrimSpace(button.Text) == "":
							t.Errorf("%s has a button with an empty label (data %q)", label, button.CallbackData)
						case strings.HasPrefix(button.CallbackData, settingsHubNamespace+"|"):
							if runes > 18 {
								t.Errorf("%s Groups button %q is %d code points, want at most 18", label, button.Text, runes)
							}
						case runes > 20:
							t.Errorf("%s button %q is %d code points, want at most 20", label, button.Text, runes)
						}
					}
				}
			}
		})
	}
}

func TestSettingsHubGroupsButton(t *testing.T) {
	env := newSettingsEnv(t)
	owner := env.newOwner(env.chat)
	settingsHubSeedGroup(t, env.chat, "Hub Group", settingsHubCreator(owner))

	env.sendPrivate(owner, "/settings")
	list := settingsPrivateOnlySent(t, env, owner)
	picked := settingsHubPickedChats(t, staffKeyboardOf(list.Params["reply_markup"]))
	if len(picked) != 1 {
		t.Fatalf("the list offers %v, want the one seeded group", picked)
	}
	var pickData string
	for _, button := range settingsAllButtons(staffKeyboardOf(list.Params["reply_markup"])) {
		if decoded, ok := decodeCallbackData(button.CallbackData, settingsCallbackNamespace); ok && decoded.Fields["a"] == settingsActPick {
			pickData = button.CallbackData
		}
	}
	env.pressRaw(settingsPrivateChat(owner), owner, list.MessageID, pickData)

	mainText, mainKeyboard := env.lastEdit(owner.Id, list.MessageID)
	if !strings.Contains(mainText, staffMarker("settings_header_main")) {
		t.Fatalf("the pick drew %q, want the group's Main page", mainText)
	}
	last := mainKeyboard[len(mainKeyboard)-1]
	if len(last) != 2 || !strings.HasPrefix(last[0].CallbackData, settingsHubNamespace+"|") ||
		!strings.Contains(last[0].Text, staffMarker("settings_btn_groups")) || !strings.Contains(last[1].Text, staffMarker("settings_btn_close")) {
		t.Fatalf("the private Main ends with %+v, want Groups and Close", last)
	}
	groups := last[0]
	if n := len(groups.CallbackData); n > 16 {
		t.Errorf("the Groups data %q is %d bytes, want at most 16", groups.CallbackData, n)
	}
	decoded, ok := decodeCallbackData(groups.CallbackData, settingsHubNamespace)
	if !ok || decoded.Fields["a"] != settingsHubActPage || decoded.Fields["v"] != "1" || len(decoded.Fields) != 2 {
		t.Errorf("the Groups data %q decodes to %v, want a=lp and v=1 only", groups.CallbackData, decoded.Fields)
	}

	answers := env.answerCount()
	edits := len(env.edits(owner.Id, list.MessageID))
	env.pressRaw(settingsPrivateChat(owner), owner, list.MessageID, groups.CallbackData)

	if got := env.answerCount() - answers; got != 1 {
		t.Errorf("Groups was answered %d times, want once", got)
	}
	if got := len(env.edits(owner.Id, list.MessageID)) - edits; got != 1 {
		t.Errorf("Groups made %d edits, want one", got)
	}
	text, keyboard := env.lastEdit(owner.Id, list.MessageID)
	if !strings.Contains(text, staffMarker("settings_page_groups")) || !strings.Contains(text, staffMarker("settings_list_intro")) {
		t.Errorf("Groups drew %q, want the list", text)
	}
	if got := settingsHubPickedChats(t, keyboard); len(got) != 1 || got[0] != fmt.Sprint(env.chat.Id) {
		t.Errorf("the redrawn list offers %v, want chat %d", got, env.chat.Id)
	}
	if sent := env.fake.sentTo(owner.Id); len(sent) != 1 {
		t.Errorf("Groups sent a new message: %d messages in the private chat, want 1", len(sent))
	}
	if got := len(callsToChat(env.fake.staffBotClient, "deleteMessage", owner.Id)); got != 0 {
		t.Errorf("Groups deleted a message %d times, want never", got)
	}

	// A group menu never carries the Groups button.
	msgID := env.openMenu(owner)
	_, groupKeyboard := env.lastEdit(env.chat.Id, msgID)
	for _, button := range settingsAllButtons(groupKeyboard) {
		if strings.HasPrefix(button.CallbackData, settingsHubNamespace+"|") {
			t.Errorf("a group menu's Main carries the hub button %q", button.CallbackData)
		}
	}
}

// settingsMainLines is the state lines of a Main page's text, in the order they are drawn.
func settingsMainLines(text string) []string {
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, staffMarker("settings_main_state_line")) {
			lines = append(lines, line)
		}
	}
	return lines
}

// settingsMainTitleKeys are the title keys of the seventeen pages in the order Main draws them for the
// owner: the sixteen grid pages in grid order, then Access.
var settingsMainTitleKeys = []string{
	"settings_page_staff", "settings_page_lockdown", "settings_page_captcha", "settings_page_antiflood",
	"settings_page_warns", "settings_page_locks", "settings_page_blacklist", "settings_page_welcome", "settings_page_goodbye",
	"settings_page_rules", "settings_page_mreq", "settings_page_del", "settings_page_switches",
	"settings_page_lang", "settings_page_log", "settings_page_anon", "settings_page_access",
}

func TestSettingsMainPhase18(t *testing.T) {
	tr := func() *i18n.Translator { return i18n.MustNewTranslator("en") }

	// wantLine is the line Main draws for the page of the title key with the given state.
	wantLine := func(titleKey, state string) string {
		for _, pg := range settingsPagesInOrder() {
			if pg.TitleKey == titleKey {
				return settingsText(tr(), "settings_main_state_line", i18n.TranslationParams{
					"icon": pg.Icon, "page": settingsText(tr(), titleKey), "state": state,
				})
			}
		}
		t.Fatalf("no page has the title key %s", titleKey)
		return ""
	}
	// linesByTitle maps each title key to its line of a Main text.
	linesByTitle := func(t *testing.T, text string) map[string]string {
		t.Helper()
		out := make(map[string]string)
		for _, key := range settingsMainTitleKeys {
			if line, ok := settingsMainStateLine(text, key); ok {
				out[key] = line
			}
		}
		return out
	}

	t.Run("the owner's Main in a group and in private: the final grid, sixteen lines in order, the defaults", func(t *testing.T) {
		settingsDelMenus(t, func(t *testing.T, m *settingsDelMenu) {
			shape := settingsMainShapeGroup
			if m.private {
				shape = settingsMainShapePrivate
			}
			if got := settingsMainShape(t, m.mainKeyboard); got != shape {
				t.Errorf("Main = %q, want %q", got, shape)
			}
			if rows := len(m.mainKeyboard); rows != settingsMainOwnerRows {
				t.Errorf("Main has %d rows, want %d", rows, settingsMainOwnerRows)
			}
			lines := settingsMainLines(m.mainText)
			if len(lines) != len(settingsMainTitleKeys) {
				t.Fatalf("Main has %d state lines, want %d:\n%s", len(lines), len(settingsMainTitleKeys), m.mainText)
			}
			for i, key := range settingsMainTitleKeys {
				if !strings.Contains(lines[i], staffMarker(key)) {
					t.Errorf("line %d = %q, want the line of %s", i+1, lines[i], key)
				}
				if strings.HasSuffix(lines[i], settingsUnknownState) {
					t.Errorf("line %d (%s) is the unknown mark on a chat the bot can read", i+1, key)
				}
			}
			// A chat with no rows reads each new page's default: the warn limit 3 with Mute, no lock,
			// Reports on by its model default, the group language Ukrainian, no log channel, anonymous
			// admins off.
			got := linesByTitle(t, m.mainText)
			for titleKey, state := range map[string]string{
				"settings_page_warns": settingsText(tr(), "settings_state_warns", i18n.TranslationParams{
					"n": 3, "action": settingsText(tr(), "settings_opt_mute"),
				}),
				"settings_page_locks":    settingsText(tr(), "settings_state_locks_on", i18n.TranslationParams{"n": 0}),
				"settings_page_switches": settingsText(tr(), "settings_state_sw_on", i18n.TranslationParams{"n": 1}),
				"settings_page_lang":     settingsLangDisplay("uk"),
				"settings_page_log":      settingsText(tr(), "settings_state_not_linked"),
				"settings_page_anon":     settingsText(tr(), "common_status_off"),
			} {
				if want := wantLine(titleKey, state); got[titleKey] != want {
					t.Errorf("%s line = %q, want %q", titleKey, got[titleKey], want)
				}
			}
		})
	})

	t.Run("an allowed administrator's Main has nine rows and no Access line", func(t *testing.T) {
		env := newSettingsDelEnv(t)
		m := env.groupMenuOf(env.admin)
		want := strings.TrimSuffix(settingsMainShapeGroup, " / a / close") + " / close"
		if got := settingsMainShape(t, m.mainKeyboard); got != want {
			t.Errorf("admin Main = %q, want %q", got, want)
		}
		if rows := len(m.mainKeyboard); rows != settingsMainOwnerRows-1 {
			t.Errorf("admin Main has %d rows, want %d", rows, settingsMainOwnerRows-1)
		}
		if lines := settingsMainLines(m.mainText); len(lines) != len(settingsMainTitleKeys)-1 {
			t.Errorf("admin Main has %d state lines, want %d", len(lines), len(settingsMainTitleKeys)-1)
		}
		if _, ok := settingsMainStateLine(m.mainText, "settings_page_access"); ok {
			t.Errorf("the admin's Main has an Access line: %q", m.mainText)
		}
	})

	t.Run("the cells and data of the earlier pages are what they were before Phase 18", func(t *testing.T) {
		env := newSettingsDelEnv(t)
		m := env.groupMenu()
		for _, want := range []struct{ key, icon, title string }{
			{settingsPageStaff, "👥", "settings_page_staff"}, {settingsPageLockdown, "🔒", "settings_page_lockdown"},
			{settingsPageCaptcha, "🧩", "settings_page_captcha"}, {settingsPageAntiflood, "🌊", "settings_page_antiflood"},
			{settingsPageWelcome, "🎉", "settings_page_welcome"}, {settingsPageGoodbye, "👋", "settings_page_goodbye"},
			{settingsPageRules, "📜", "settings_page_rules"}, {settingsPageMreq, "🛂", "settings_page_mreq"},
			{settingsPageDeletion, "🧹", "settings_page_del"}, {settingsPageAccess, "🔑", "settings_page_access"},
		} {
			button, ok := settingsPageButton(t, m.mainKeyboard, want.key)
			if !ok {
				t.Errorf("Main has no cell for page %s", want.key)
				continue
			}
			if label := want.icon + " " + settingsText(tr(), want.title); button.Text != label {
				t.Errorf("page %s cell = %q, want %q", want.key, button.Text, label)
			}
			wantData := encodeCallbackData(settingsCallbackNamespace, env.groupFields(env.owner, "a", "op", "p", want.key))
			if button.CallbackData != wantData {
				t.Errorf("page %s data = %q, want %q", want.key, button.CallbackData, wantData)
			}
		}
	})

	t.Run("a page whose read fails shows the unknown mark and no other line changes", func(t *testing.T) {
		env := newSettingsDelEnv(t)
		draw := func() map[string]string {
			entries := settingsMainEntries(env.bot, tr(), &env.chat, true, true)
			if len(entries) != len(settingsMainTitleKeys) {
				t.Fatalf("%d entries, want %d", len(entries), len(settingsMainTitleKeys))
			}
			view := settingsView{
				Tr: tr(), OwnerKey: "u", OwnerValue: "9999999999",
				GroupID: env.chat.Id, GroupTitle: "G", OpenerName: "Opener",
			}
			return linesByTitle(t, renderSettingsMain(view, entries, "").Text)
		}
		before := settingsLocksCallTotal(env.settingsEnv)
		baseline := draw()
		if len(baseline) != len(settingsMainTitleKeys) {
			t.Fatalf("baseline has %d lines, want %d", len(baseline), len(settingsMainTitleKeys))
		}
		if got := settingsLocksCallTotal(env.settingsEnv) - before; got != 0 {
			t.Errorf("drawing Main made %d Telegram calls of its own, want none", got)
		}

		failure := errors.New("database is gone")
		for _, failing := range []struct {
			titleKey string
			swap     func(t *testing.T)
		}{
			{"settings_page_warns", func(t *testing.T) {
				mreqSwap(t, &settingsWarnsRead, func(context.Context, int64) (*models.WarnSettings, error) { return nil, failure })
			}},
			{"settings_page_locks", func(t *testing.T) {
				mreqSwap(t, &settingsLocksRead, func(context.Context, int64) (map[string]bool, error) { return nil, failure })
			}},
			{"settings_page_switches", func(t *testing.T) {
				mreqSwap(t, &settingsSwReportsRead, func(context.Context, int64) (bool, error) { return false, failure })
			}},
			{"settings_page_lang", func(t *testing.T) {
				mreqSwap(t, &settingsLangRead, func(context.Context, int64) (string, error) { return "", failure })
			}},
			{"settings_page_log", func(t *testing.T) {
				mreqSwap(t, &settingsLogRead, func(context.Context, int64) (*models.LogChannel, error) { return nil, failure })
			}},
			{"settings_page_anon", func(t *testing.T) {
				mreqSwap(t, &settingsAnonRead, func(context.Context, int64) (bool, error) { return false, failure })
			}},
		} {
			t.Run(failing.titleKey, func(t *testing.T) {
				failing.swap(t)
				got := draw()
				for key, line := range got {
					if key == failing.titleKey {
						if !strings.HasSuffix(line, settingsUnknownState) {
							t.Errorf("the failing page's line = %q, want the unknown mark", line)
						}
						continue
					}
					if line != baseline[key] {
						t.Errorf("%s changed to %q from %q while another page's read failed", key, line, baseline[key])
					}
				}
			})
		}
	})

	t.Run("redrawing Main costs the pipeline's calls only and writes no row", func(t *testing.T) {
		env := newSettingsDelEnv(t)
		m := env.groupMenu()
		m.press("a", "op", "p", settingsPageLocks)
		before := settingsMenuMarkOf(env.settingsEnv)
		botReads := env.botReads(env.chat.Id)
		calls := settingsLocksCallTotal(env.settingsEnv)
		m.press("a", "bk")
		after := settingsMenuMarkOf(env.settingsEnv)
		if got := after.members - before.members; got != 1 {
			t.Errorf("Back made %d getChatMember calls, want the presser's one", got)
		}
		if got := env.botReads(env.chat.Id) - botReads; got != 0 {
			t.Errorf("Back read the bot %d times, want none", got)
		}
		if after.chats != before.chats || after.edits-before.edits != 1 || after.answers-before.answers != 1 {
			t.Errorf("Back made %+v, want one edit, one answer and no getChat", after)
		}
		if got := settingsLocksCallTotal(env.settingsEnv) - calls; got != 3 {
			t.Errorf("Back made %d Telegram calls, want 3 (the presser's record, the edit, the answer)", got)
		}
		if n := settingsPhase18RowCount(t, env.chat.Id); n != 0 {
			t.Errorf("drawing Main left %d rows in the settings tables", n)
		}
	})
}
