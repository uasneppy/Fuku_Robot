//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// settingsMreqIcons and settingsMreqWordKeys are the UI contract's punishment icons and the reused
// option words, by stored action 0..4 (written out here, not read from the code under test).
var (
	settingsMreqIcons    = []string{"❌", "❗", "👢", "⛔", "🔇"}
	settingsMreqWordKeys = []string{"settings_state_off_plain", "settings_opt_warn", "settings_opt_kick", "settings_opt_ban", "settings_opt_mute"}
	settingsMreqRuleKeys = []string{
		"settings_mreq_rule_lastname", "settings_mreq_rule_username", "settings_mreq_rule_russian",
		"settings_mreq_rule_arabic", "settings_mreq_rule_chinese", "settings_mreq_rule_photo",
		"settings_mreq_rule_channel",
	}
)

// settingsMreqActionsOf is the seven stored punishments of a row in rule order.
func settingsMreqActionsOf(row models.ChatRequirements) [7]int {
	return [7]int{
		row.LastNameAction, row.UsernameAction, row.RussianAction, row.ArabicAction, row.ChineseAction,
		row.PhotoAction, row.ChannelAction,
	}
}

// settingsMreqSeed replaces the requirements row of chatID with the given actions and switches
// through a direct write, then drops the cached read on every layer.
func settingsMreqSeed(t *testing.T, chatID int64, actions [7]int, join, del bool) {
	t.Helper()
	if err := db.DB.Where("chat_id = ?", chatID).Delete(&models.ChatRequirements{}).Error; err != nil {
		t.Fatalf("clear the requirements row of %d: %v", chatID, err)
	}
	row := &models.ChatRequirements{
		ChatID: chatID, LastNameAction: actions[0], UsernameAction: actions[1], RussianAction: actions[2],
		ArabicAction: actions[3], ChineseAction: actions[4], PhotoAction: actions[5], ChannelAction: actions[6],
		CheckOnJoin: join, DeleteMessages: del,
	}
	if err := db.DB.Create(row).Error; err != nil {
		t.Fatalf("seed the requirements row of %d: %v", chatID, err)
	}
	dbcache.DeleteCache(dbcache.CacheKey("member_requirements", chatID))
	dbcache.ResetLocalForTest()
}

// settingsMreqStored reads the requirements row of chatID straight from the database.
func settingsMreqStored(t *testing.T, chatID int64) (models.ChatRequirements, bool) {
	t.Helper()
	var rows []models.ChatRequirements
	if err := db.DB.Where("chat_id = ?", chatID).Find(&rows).Error; err != nil {
		t.Fatalf("read the requirements row of %d: %v", chatID, err)
	}
	if len(rows) == 0 {
		return models.ChatRequirements{}, false
	}
	return rows[0], true
}

// settingsMreqRuleLine is the body line of a rule with the given stored action, as the marker
// locale renders it.
func settingsMreqRuleLine(rule int, action int) string {
	return staffMarker("settings_mreq_line") + " " + staffMarker(settingsMreqRuleKeys[rule]) + " " +
		settingsMreqIcons[action] + " " + staffMarker(settingsMreqWordKeys[action])
}

// settingsMreqSwitchLine is the body line of a switch.
func settingsMreqSwitchLine(key string, on bool) string {
	if on {
		return staffMarker(key) + " ✅ " + staffMarker("common_status_on")
	}
	return staffMarker(key) + " ❌ " + staffMarker("common_status_off")
}

// settingsMreqPageMarkers is the body of the page in the UI contract's order for the given
// actions and switches.
func settingsMreqPageMarkers(actions [7]int, join, del bool) []string {
	markers := []string{
		staffMarker("settings_header") + " " + staffMarker("settings_page_mreq"),
		staffMarker("settings_mreq_intro"),
	}
	allOff := true
	for _, a := range actions {
		if a != 0 {
			allOff = false
		}
	}
	if allOff {
		markers = append(markers, staffMarker("settings_mreq_all_off"))
	}
	for i, a := range actions {
		markers = append(markers, settingsMreqRuleLine(i, a))
	}
	markers = append(markers,
		settingsMreqSwitchLine("settings_mreq_line_join", join),
		settingsMreqSwitchLine("settings_mreq_line_delete", del),
		staffMarker("settings_mreq_hint_letters"),
		staffMarker("settings_mreq_hint_blank"),
		staffMarker("settings_mreq_hint_join"),
		staffMarker("settings_mreq_hint_delete"),
		staffMarker("settings_mreq_hint_mute"),
	)
	return markers
}

// settingsMreqWantPage fails the test unless text holds every marker, in order, and holds the
// all-off line exactly when allOff says so.
func settingsMreqWantPage(t *testing.T, text string, actions [7]int, join, del bool) {
	t.Helper()
	at := 0
	for _, marker := range settingsMreqPageMarkers(actions, join, del) {
		i := strings.Index(text[at:], marker)
		if i < 0 {
			t.Fatalf("the page %q lacks %q after offset %d", text, marker, at)
		}
		at += i + len(marker)
	}
	anyOn := false
	for _, a := range actions {
		anyOn = anyOn || a != 0
	}
	if has := strings.Contains(text, staffMarker("settings_mreq_all_off")); has == anyOn {
		t.Errorf("all-off line present = %v with actions %v", has, actions)
	}
}

// settingsMreqWantSwitchRow fails the test unless the keyboard is the page's six rows (four rule
// grid rows, the switch row, the navigation row), with each switch cell showing the given state
// icon. The grid itself is judged by settingsMreqWantGrid.
func settingsMreqWantSwitchRow(t *testing.T, keyboard [][]gotgbot.InlineKeyboardButton, join, del bool) {
	t.Helper()
	if len(keyboard) != 6 {
		t.Fatalf("the page has %d keyboard rows, want the four grid rows, the switch row and the navigation row: %+v", len(keyboard), keyboard)
	}
	switchRow := keyboard[4]
	if len(switchRow) != 2 {
		t.Fatalf("the switch row has %d buttons, want 2", len(switchRow))
	}
	icon := func(on bool) string {
		if on {
			return "✅"
		}
		return "❌"
	}
	for i, want := range []struct {
		key  string
		on   bool
		code string
	}{
		{"settings_btn_mreq_switch_join", join, "j"},
		{"settings_btn_mreq_switch_delete", del, "d"},
	} {
		button := switchRow[i]
		if !containsMarker(button.Text, want.key) || !strings.Contains(button.Text, icon(want.on)) {
			t.Errorf("switch cell %d = %q, want the %s marker with %s", i, button.Text, want.key, icon(want.on))
		}
		fields := settingsButtonFields(t, button)
		wantV := "1"
		if want.on {
			wantV = "0"
		}
		if fields["a"] != "tg" || fields["p"] != settingsPageMreq || fields["t"] != want.code || fields["v"] != wantV {
			t.Errorf("switch cell %d fields = %v, want tg on page m, control %s, v=%s", i, fields, want.code, wantV)
		}
	}
	nav := keyboard[5]
	if len(nav) != 2 || !containsMarker(nav[0].Text, "settings_btn_back") || !containsMarker(nav[1].Text, "settings_btn_close") {
		t.Errorf("navigation row = %+v, want Back and Close", nav)
	}
}

// settingsMreqRuleCodes and settingsMreqCellKeys are the contract's rule codes and cell label keys in
// rule order (written out here, not read from the code under test).
var (
	settingsMreqRuleCodes = []string{"ln", "un", "ru", "ar", "zh", "ph", "ch"}
	settingsMreqCellKeys  = []string{
		"settings_btn_mreq_lastname", "settings_btn_mreq_username", "settings_btn_mreq_russian",
		"settings_btn_mreq_arabic", "settings_btn_mreq_chinese", "settings_btn_mreq_photo",
		"settings_btn_mreq_channel",
	}
)

// settingsMreqWantGrid fails the test unless the first four rows of the keyboard are the rule grid
// of the contract: two cells per row filled left to right, the odd last cell alone, each cell the
// rule's stored punishment icon and its label key, pressing a=mo with that rule's code.
func settingsMreqWantGrid(t *testing.T, keyboard [][]gotgbot.InlineKeyboardButton, actions [7]int) {
	t.Helper()
	if len(keyboard) < 4 {
		t.Fatalf("the keyboard has %d rows, want the rule grid first: %+v", len(keyboard), keyboard)
	}
	i := 0
	for r, width := range []int{2, 2, 2, 1} {
		if len(keyboard[r]) != width {
			t.Fatalf("grid row %d has %d cells, want %d: %+v", r, len(keyboard[r]), width, keyboard[r])
		}
		for _, button := range keyboard[r] {
			if !containsMarker(button.Text, settingsMreqCellKeys[i]) || !strings.Contains(button.Text, settingsMreqIcons[actions[i]]) {
				t.Errorf("grid cell %d = %q, want %s and the icon %s", i, button.Text, settingsMreqCellKeys[i], settingsMreqIcons[actions[i]])
			}
			fields := settingsButtonFields(t, button)
			if fields["a"] != "mo" || fields["t"] != settingsMreqRuleCodes[i] || len(fields) != 3 {
				t.Errorf("grid cell %d fields = %v, want exactly a=mo, t=%s and the owner", i, fields, settingsMreqRuleCodes[i])
			}
			i++
		}
	}
}

func TestSettingsMreqOpensFromMain(t *testing.T) {
	t.Run("in a group menu", func(t *testing.T) {
		env := newSettingsEnv(t)
		owner := env.newOwner(env.chat)
		msgID := env.openMenu(owner)
		mainText, mainKeyboard := env.lastEdit(env.chat.Id, msgID)

		if got, want := settingsMainShape(t, mainKeyboard), "s|l / c|f / w|g / r|m / a / close"; got != want {
			t.Errorf("the owner's group Main = %q, want %q", got, want)
		}
		line, found := settingsMainStateLine(mainText, "settings_page_mreq")
		if !found || !strings.Contains(line, "🛂") || !strings.Contains(line, staffMarker("settings_state_off_plain")) {
			t.Errorf("Main status line = %q (found %v), want 🛂 and the Off word", line, found)
		}

		button, ok := settingsPageButton(t, mainKeyboard, settingsPageMreq)
		if !ok || !strings.Contains(button.Text, "🛂") || !containsMarker(button.Text, "settings_page_mreq") {
			t.Fatalf("Main has no 🛂 Requirements button: %+v", mainKeyboard)
		}
		env.pressRaw(env.chat, owner, msgID, button.CallbackData)

		text, keyboard := env.lastEdit(env.chat.Id, msgID)
		settingsMreqWantPage(t, text, [7]int{}, false, false)
		settingsMreqWantSwitchRow(t, keyboard, false, false)
		for _, b := range settingsAllButtons(keyboard) {
			if b.Url != "" {
				t.Errorf("the page has a URL button %q, want no Open in private row", b.Url)
			}
		}
		if _, has := settingsMreqStored(t, env.chat.Id); has {
			t.Error("opening Main and the page created a chat_requirements row")
		}
	})

	t.Run("in a private menu", func(t *testing.T) {
		env := newSettingsEnv(t)
		owner := env.newOwner(env.chat)
		env.sendPrivate(owner, fmt.Sprintf("/start settings_%d", env.chat.Id))
		sentMain := settingsPrivateOnlySent(t, env, owner)
		msgID := sentMain.MessageID
		mainKeyboard := staffKeyboardOf(sentMain.Params["reply_markup"])
		private := settingsPrivateChat(owner)

		if got, want := settingsMainShape(t, mainKeyboard), "s|l / c|f / w|g / r|m / a / groups|close"; got != want {
			t.Errorf("the owner's private Main = %q, want %q", got, want)
		}
		button, ok := settingsPageButton(t, mainKeyboard, settingsPageMreq)
		if !ok {
			t.Fatalf("the private Main has no Requirements button: %+v", mainKeyboard)
		}
		if got := settingsButtonFields(t, button)["c"]; got != fmt.Sprint(env.chat.Id) {
			t.Errorf("the private grid button carries c=%q, want the group", got)
		}
		env.pressRaw(private, owner, msgID, button.CallbackData)

		text, keyboard := env.lastEdit(owner.Id, msgID)
		settingsMreqWantPage(t, text, [7]int{}, false, false)
		settingsMreqWantSwitchRow(t, keyboard, false, false)
		if _, has := settingsMreqStored(t, env.chat.Id); has {
			t.Error("opening the private page created a chat_requirements row")
		}
	})
}

// TestSettingsMreqLookupRulesStore is the tracer of the photo and channel rules (D-13, D-14): an
// owner in a private menu sees seven rule lines and seven cells, opens the photo rule, picks Warn
// (stored at once in the new column), sees it on the page and on Main, picks Mute (a card first,
// then Yes), and sets the channel rule Off, which never draws a card. Nothing here looks a member up.
func TestSettingsMreqLookupRulesStore(t *testing.T) {
	env := newSettingsEnv(t)
	// The channel rule's screen draws all three rows only for a group linked to a channel.
	settingsMreqLinkChannel(t, env)
	owner := env.newOwner(env.chat)
	env.sendPrivate(owner, fmt.Sprintf("/start settings_%d", env.chat.Id))
	msgID := settingsPrivateOnlySent(t, env, owner).MessageID
	private := settingsPrivateChat(owner)
	press := func(kv ...string) {
		t.Helper()
		env.pressPrivate(owner, msgID, env.privateFields(env.chat.Id, kv...))
	}
	lastEdit := func() (string, [][]gotgbot.InlineKeyboardButton) {
		t.Helper()
		return env.lastEdit(private.Id, msgID)
	}
	mainLine := func() string {
		t.Helper()
		press("a", "bk")
		text, _ := lastEdit()
		line, found := settingsMainStateLine(text, "settings_page_mreq")
		if !found {
			t.Fatalf("Main %q has no Requirements line", text)
		}
		return line
	}
	if line := mainLine(); !strings.Contains(line, staffMarker("settings_state_off_plain")) {
		t.Fatalf("Main line = %q, want the Off word", line)
	}

	press("a", "op", "p", settingsPageMreq)
	text, keyboard := lastEdit()
	settingsMreqWantPage(t, text, [7]int{}, false, false)
	settingsMreqWantGrid(t, keyboard, [7]int{})
	settingsMreqWantSwitchRow(t, keyboard, false, false)
	if cells := len(settingsAllButtons(keyboard[:4])); cells != 7 {
		t.Fatalf("the page has %d rule cells, want 7", cells)
	}

	t.Run("the photo cell opens the photo rule's screen", func(t *testing.T) {
		press("a", "mo", "t", "ph")
		text, keyboard := lastEdit()
		settingsMreqWantRuleScreen(t, text, 5, 0)
		settingsMreqWantRuleKeyboard(t, keyboard, 5, 0)
	})

	t.Run("Warn is stored at once and shown on the page and on Main", func(t *testing.T) {
		press("a", "st", "p", settingsPageMreq, "t", "ph", "v", "1")
		if got, want := settingsMreqStoredActions(t, env.chat.Id), [7]int{5: 1}; got != want {
			t.Fatalf("stored %v, want only the photo rule on Warn", got)
		}
		text, keyboard := lastEdit()
		settingsMreqWantRuleScreen(t, text, 5, 1)
		settingsMreqWantRuleKeyboard(t, keyboard, 5, 1)

		press("a", "op", "p", settingsPageMreq)
		text, keyboard = lastEdit()
		settingsMreqWantPage(t, text, [7]int{5: 1}, false, false)
		settingsMreqWantGrid(t, keyboard, [7]int{5: 1})
		if line := mainLine(); !strings.Contains(line, staffMarker("settings_state_mreq_on")+" 1") {
			t.Errorf("Main line = %q, want 1 on", line)
		}
	})

	t.Run("Mute draws the card, writes nothing, and Yes writes Mute", func(t *testing.T) {
		press("a", "st", "p", settingsPageMreq, "t", "ph", "v", "4")
		text, keyboard := lastEdit()
		yes, _, _ := settingsMreqWantCard(t, text, keyboard, 5, models.RequirementActionMute)
		if got := settingsMreqStoredActions(t, env.chat.Id); got != [7]int{5: 1} {
			t.Fatalf("drawing the card wrote %v", got)
		}
		env.pressRaw(private, owner, msgID, yes.CallbackData)
		if got := settingsMreqStoredActions(t, env.chat.Id); got != [7]int{5: models.RequirementActionMute} {
			t.Fatalf("after Yes stored %v, want only the photo rule on Mute", got)
		}
	})

	t.Run("the channel rule opens, and Off is stored at once with no card", func(t *testing.T) {
		settingsMreqSeed(t, env.chat.Id, [7]int{0, 0, 0, 0, 0, models.RequirementActionMute, models.RequirementActionKick}, false, false)
		press("a", "mo", "t", "ch")
		text, keyboard := lastEdit()
		settingsMreqWantRuleScreen(t, text, 6, models.RequirementActionKick)
		settingsMreqWantRuleKeyboard(t, keyboard, 6, models.RequirementActionKick)

		press("a", "st", "p", settingsPageMreq, "t", "ch", "v", "0")
		if got, want := settingsMreqStoredActions(t, env.chat.Id), [7]int{5: models.RequirementActionMute}; got != want {
			t.Fatalf("stored %v, want the channel rule Off and the photo rule still on Mute", got)
		}
		text, _ = lastEdit()
		if strings.Contains(text, staffMarker("settings_mreq_confirm")) {
			t.Errorf("Off drew a card: %q", text)
		}
		settingsMreqWantRuleScreen(t, text, 6, 0)
	})

	if got := len(env.fake.callsFor("getUserProfilePhotos")); got != 0 {
		t.Errorf("the page and its rule screens made %d getUserProfilePhotos requests, want none", got)
	}
}

func TestSettingsMreqMainState(t *testing.T) {
	env := newSettingsEnv(t)
	owner := env.newOwner(env.chat)
	msgID := env.openMenu(owner)
	redrawMain := func() string {
		t.Helper()
		env.press(owner, msgID, env.groupFields(owner, "a", "bk"))
		text, _ := env.lastEdit(env.chat.Id, msgID)
		line, found := settingsMainStateLine(text, "settings_page_mreq")
		if !found {
			t.Fatalf("Main %q has no Requirements line", text)
		}
		return line
	}

	t.Run("every rule off reads Off whatever the switches say", func(t *testing.T) {
		settingsMreqSeed(t, env.chat.Id, [7]int{}, true, true)
		if line := redrawMain(); !strings.Contains(line, staffMarker("settings_state_off_plain")) {
			t.Errorf("line = %q, want the Off word", line)
		}
	})

	for n := 1; n <= 7; n++ {
		t.Run(fmt.Sprintf("%d rules on", n), func(t *testing.T) {
			var actions [7]int
			for i := 0; i < n; i++ {
				actions[i] = []int{1, 2, 3, 4, 1, 4, 2}[i]
			}
			settingsMreqSeed(t, env.chat.Id, actions, false, false)
			want := fmt.Sprintf("%s %d", staffMarker("settings_state_mreq_on"), n)
			if line := redrawMain(); !strings.Contains(line, want) {
				t.Errorf("line = %q, want %q", line, want)
			}
		})
	}

	t.Run("an unreadable setting is the unknown mark and the cannot-open edit", func(t *testing.T) {
		settingsMreqSeams(t)
		settingsMreqSeed(t, env.chat.Id, [7]int{1}, false, false)
		memberReqSettingsLookup = func(context.Context, int64) (*models.ChatRequirements, error) {
			return nil, errors.New("settings are unreadable")
		}
		if line := redrawMain(); !strings.HasSuffix(line, settingsUnknownState) {
			t.Errorf("line = %q, want the unknown mark at its end", line)
		}
		before := len(env.edits(env.chat.Id, msgID))
		env.press(owner, msgID, env.groupFields(owner, "a", "op", "p", settingsPageMreq))
		if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("settings_cannot_open_alert")) {
			t.Errorf("answer = %q (alert %v), want the cannot-open alert", answer, alert)
		}
		text, keyboard := env.lastEdit(env.chat.Id, msgID)
		if len(env.edits(env.chat.Id, msgID)) != before+1 || !strings.Contains(text, staffMarker("settings_cannot_open")) || len(keyboard) != 0 {
			t.Errorf("edit = %q with %d rows, want the cannot-open note with no keyboard", text, len(keyboard))
		}
	})
}

// settingsMreqSeams restores the read and write seams of the page when the test ends.
func settingsMreqSeams(t *testing.T) {
	t.Helper()
	lookup, writes := memberReqSettingsLookup, memberReqWrites
	t.Cleanup(func() { memberReqSettingsLookup, memberReqWrites = lookup, writes })
}

// settingsMreqOpenPage opens the menu as opener and then the Requirements page, and returns the
// menu message's ID.
func settingsMreqOpenPage(t *testing.T, env *settingsEnv, opener gotgbot.User) int64 {
	t.Helper()
	msgID := env.openMenu(opener)
	env.press(opener, msgID, env.groupFields(opener, "a", "op", "p", settingsPageMreq))
	return msgID
}

// settingsMreqSwitchFields is the field set of a switch press of a menu opened in the group.
func settingsMreqSwitchFields(env *settingsEnv, opener gotgbot.User, action, control, value string) map[string]string {
	return env.groupFields(opener, "a", action, "p", settingsPageMreq, "t", control, "v", value)
}

func TestSettingsMreqSwitches(t *testing.T) {
	env := newSettingsEnv(t)
	owner := env.newOwner(env.chat)
	msgID := settingsMreqOpenPage(t, env, owner)
	press := func(action, control, value string) {
		t.Helper()
		env.press(owner, msgID, settingsMreqSwitchFields(env, owner, action, control, value))
	}

	cases := []struct {
		name, action, control string
		join, del             bool
	}{
		{"tg joins on", "tg", "j", true, false},
		{"tg joins off", "tg", "j", false, false},
		{"st joins on", "st", "j", true, false},
		{"st joins off", "st", "j", false, false},
		{"tg delete on", "tg", "d", false, true},
		{"tg delete off", "tg", "d", false, false},
		{"st delete on", "st", "d", false, true},
		{"st delete off", "st", "d", false, false},
	}
	for _, c := range cases {
		value := "0"
		if (c.control == "j" && c.join) || (c.control == "d" && c.del) {
			value = "1"
		}
		press(c.action, c.control, value)
		stored, has := settingsMreqStored(t, env.chat.Id)
		if !has || stored.CheckOnJoin != c.join || stored.DeleteMessages != c.del {
			t.Fatalf("%s: stored = %+v (row %v), want join %v and delete %v", c.name, stored, has, c.join, c.del)
		}
		text, keyboard := env.lastEdit(env.chat.Id, msgID)
		settingsMreqWantPage(t, text, [7]int{}, c.join, c.del)
		settingsMreqWantSwitchRow(t, keyboard, c.join, c.del)
		if strings.Contains(text, "⚠️") {
			t.Errorf("%s: the redraw carries a notice: %q", c.name, text)
		}
	}

	t.Run("a value that is not 0 or 1 is refused before any lookup", func(t *testing.T) {
		for _, bad := range []struct{ action, control, value string }{
			{"tg", "j", "2"}, {"tg", "d", "x"}, {"st", "j", "01"}, {"st", "d", ""}, {"tg", "zz", "1"}, {"st", "zz", "1"}, {"tg", "ln", "1"},
		} {
			lookups := env.memberLookupsOf(env.chat.Id, owner.Id)
			edits, answers := len(env.edits(env.chat.Id, msgID)), env.answerCount()
			press(bad.action, bad.control, bad.value)
			if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("common_callback_invalid_request")) {
				t.Errorf("%+v answered %q (alert %v), want the invalid request alert", bad, answer, alert)
			}
			if got := env.answerCount() - answers; got != 1 {
				t.Errorf("%+v answered %d times, want once", bad, got)
			}
			if got := env.memberLookupsOf(env.chat.Id, owner.Id); got != lookups {
				t.Errorf("%+v made %d getChatMember calls before refusing, want none", bad, got-lookups)
			}
			if got := len(env.edits(env.chat.Id, msgID)); got != edits {
				t.Errorf("%+v edited the page", bad)
			}
			if stored, _ := settingsMreqStored(t, env.chat.Id); stored.CheckOnJoin || stored.DeleteMessages {
				t.Errorf("%+v wrote a switch: %+v", bad, stored)
			}
		}
	})

	t.Run("a failed write redraws the stored state with the save-failed notice", func(t *testing.T) {
		settingsMreqSeams(t)
		memberReqWrites.CheckOnJoin = func(context.Context, int64, bool) error { return errors.New("write failed") }
		memberReqWrites.DeleteMessages = func(context.Context, int64, bool) error { return errors.New("write failed") }
		settingsMreqSeed(t, env.chat.Id, [7]int{1, 0, 0, 3, 0}, false, true)
		for _, c := range []struct{ control, value string }{{"j", "1"}, {"d", "0"}} {
			press("tg", c.control, c.value)
			text, keyboard := env.lastEdit(env.chat.Id, msgID)
			if want := settingsNoticeWarnMarker("common_settings_save_failed"); !strings.Contains(text, want) {
				t.Errorf("control %s: the redraw %q lacks the notice %q", c.control, text, want)
			}
			settingsMreqWantPage(t, text, [7]int{1, 0, 0, 3, 0}, false, true)
			settingsMreqWantSwitchRow(t, keyboard, false, true)
			if stored, _ := settingsMreqStored(t, env.chat.Id); stored.CheckOnJoin || !stored.DeleteMessages {
				t.Errorf("control %s: stored = %+v after a failed write, want it unchanged", c.control, stored)
			}
		}
	})
}

// settingsNoticeWarnMarker is the notice line of a warning whose text is the marker of key.
func settingsNoticeWarnMarker(key string) string {
	return settingsNoticeWarnIcon + staffMarker(key)
}

func TestSettingsDriftMreq(t *testing.T) {
	env := newSettingsEnv(t)
	owner := env.newOwner(env.chat)
	seeded := [7]int{1, 2, 3, 4, 0, 1, 4}

	t.Run("the page and Main read through the cached loader", func(t *testing.T) {
		settingsMreqSeed(t, env.chat.Id, seeded, false, true)
		// Load the key into the cache, then change the table behind it: whatever the cache holds is
		// what the page and Main draw, so they read the key the watcher will read.
		if _, err := memberreq.GetSettings(context.Background(), env.chat.Id); err != nil {
			t.Fatalf("GetSettings: %v", err)
		}
		if err := db.DB.Model(&models.ChatRequirements{}).Where("chat_id = ?", env.chat.Id).
			Updates(map[string]any{"last_name_action": 0, "username_action": 0, "russian_action": 4}).Error; err != nil {
			t.Fatalf("change the row behind the cache: %v", err)
		}
		msgID := settingsMreqOpenPage(t, env, owner)
		text, _ := env.lastEdit(env.chat.Id, msgID)
		settingsMreqWantPage(t, text, seeded, false, true)

		// The seam is the only read: a crafted answer is what the page draws.
		settingsMreqSeams(t)
		var asked []int64
		memberReqSettingsLookup = func(_ context.Context, chatID int64) (*models.ChatRequirements, error) {
			asked = append(asked, chatID)
			return &models.ChatRequirements{ChatID: chatID, ArabicAction: models.RequirementActionKick, CheckOnJoin: true}, nil
		}
		env.press(owner, msgID, env.groupFields(owner, "a", "op", "p", settingsPageMreq))
		text, _ = env.lastEdit(env.chat.Id, msgID)
		settingsMreqWantPage(t, text, [7]int{0, 0, 0, 2, 0}, true, false)
		env.press(owner, msgID, env.groupFields(owner, "a", "bk"))
		if len(asked) < 2 {
			t.Errorf("the seam was asked %d times by the page and Main, want at least 2", len(asked))
		}
		for _, id := range asked {
			if id != env.chat.Id {
				t.Errorf("the seam was asked about chat %d, want only %d", id, env.chat.Id)
			}
		}
	})

	t.Run("each switch changes only its column and drops the cached key", func(t *testing.T) {
		settingsMreqSeed(t, env.chat.Id, seeded, false, true)
		msgID := settingsMreqOpenPage(t, env, owner)
		if _, err := memberreq.GetSettings(context.Background(), env.chat.Id); err != nil {
			t.Fatalf("GetSettings: %v", err)
		}
		env.press(owner, msgID, settingsMreqSwitchFields(env, owner, "tg", "j", "1"))
		stored, _ := settingsMreqStored(t, env.chat.Id)
		if got := settingsMreqActionsOf(stored); got != seeded || !stored.CheckOnJoin || !stored.DeleteMessages {
			t.Errorf("after Check on join: %+v, want the actions %v, join on and delete still on", stored, seeded)
		}
		cached, err := memberreq.GetSettings(context.Background(), env.chat.Id)
		if err != nil || !cached.CheckOnJoin {
			t.Errorf("the cached read after the write = %+v, %v, want the new value (the key was dropped)", cached, err)
		}

		env.press(owner, msgID, settingsMreqSwitchFields(env, owner, "tg", "d", "0"))
		stored, _ = settingsMreqStored(t, env.chat.Id)
		if got := settingsMreqActionsOf(stored); got != seeded || !stored.CheckOnJoin || stored.DeleteMessages {
			t.Errorf("after Delete messages: %+v, want the actions %v, join still on and delete off", stored, seeded)
		}
		cached, err = memberreq.GetSettings(context.Background(), env.chat.Id)
		if err != nil || cached.DeleteMessages {
			t.Errorf("the cached read after the write = %+v, %v, want delete off (the key was dropped)", cached, err)
		}
		text, _ := env.lastEdit(env.chat.Id, msgID)
		settingsMreqWantPage(t, text, seeded, true, false)
	})

	t.Run("the photo and channel presses change only their own columns and drop the cached key", func(t *testing.T) {
		linked := newSettingsEnv(t)
		settingsMreqLinkChannel(t, linked)
		linkedOwner := linked.newOwner(linked.chat)
		base := [7]int{1, 2, 3, 4, 0, 0, 0}
		settingsMreqSeed(t, linked.chat.Id, base, true, false)
		msgID := settingsMreqOpenPage(t, linked, linkedOwner)
		if _, err := memberreq.GetSettings(context.Background(), linked.chat.Id); err != nil {
			t.Fatalf("GetSettings: %v", err)
		}
		want := base
		for _, c := range []struct {
			code string
			slot int
		}{{"ph", 5}, {"ch", 6}} {
			linked.press(linkedOwner, msgID, settingsMreqRuleFields(linked, linkedOwner, "st", c.code, "p", settingsPageMreq, "v", "1"))
			want[c.slot] = models.RequirementActionWarn
			stored, _ := settingsMreqStored(t, linked.chat.Id)
			if got := settingsMreqActionsOf(stored); got != want || !stored.CheckOnJoin || stored.DeleteMessages {
				t.Errorf("after st %s: %+v, want the actions %v and the switches untouched", c.code, stored, want)
			}
			cached, err := memberreq.GetSettings(context.Background(), linked.chat.Id)
			if err != nil || settingsMreqActionsOf(*cached) != want {
				t.Errorf("the cached read after st %s = %+v, %v, want %v (the key was dropped)", c.code, cached, err, want)
			}
		}
		linked.press(linkedOwner, msgID, linked.groupFields(linkedOwner, "a", "op", "p", settingsPageMreq))
		text, _ := linked.lastEdit(linked.chat.Id, msgID)
		settingsMreqWantPage(t, text, want, true, false)
	})

	t.Run("opening the page and Main creates no row", func(t *testing.T) {
		fresh := newSettingsEnv(t)
		freshOwner := fresh.newOwner(fresh.chat)
		msgID := settingsMreqOpenPage(t, fresh, freshOwner)
		fresh.press(freshOwner, msgID, fresh.groupFields(freshOwner, "a", "bk"))
		fresh.press(freshOwner, msgID, fresh.groupFields(freshOwner, "a", "op", "p", settingsPageMreq))
		if _, has := settingsMreqStored(t, fresh.chat.Id); has {
			t.Error("opening the page and Main left a chat_requirements row")
		}
	})
}

func TestSettingsAuthorityMreq(t *testing.T) {
	t.Run("an administrator without can_restrict_members opens the page and is refused every switch", func(t *testing.T) {
		env := newSettingsEnv(t)
		helper := env.newOutsider()
		env.fake.setMember(env.chat.Id, helper.Id, staffFakeMember{
			Status: gotgbot.ChatMemberStatusAdministrator, CanRestrictMembers: false,
		})
		msgID := settingsMreqOpenPage(t, env, helper)
		text, keyboard := env.lastEdit(env.chat.Id, msgID)
		settingsMreqWantPage(t, text, [7]int{}, false, false)
		settingsMreqWantSwitchRow(t, keyboard, false, false)

		for _, control := range []string{"j", "d"} {
			for _, action := range []string{"tg", "st"} {
				edits, answers := len(env.edits(env.chat.Id, msgID)), env.answerCount()
				env.press(helper, msgID, settingsMreqSwitchFields(env, helper, action, control, "1"))
				if got := env.answerCount() - answers; got != 1 {
					t.Errorf("%s on %s answered %d times, want once", action, control, got)
				}
				if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("settings_no_restrict")) {
					t.Errorf("%s on %s answered %q (alert %v), want the settings_no_restrict alert", action, control, answer, alert)
				}
				if got := len(env.edits(env.chat.Id, msgID)); got != edits {
					t.Errorf("%s on %s edited the page", action, control)
				}
			}
		}
		if _, has := settingsMreqStored(t, env.chat.Id); has {
			t.Error("a refused press wrote a row")
		}
	})

	t.Run("an administrator without can_restrict_members opens every rule screen and is refused every choice and Yes", func(t *testing.T) {
		env := newSettingsEnv(t)
		settingsMreqLinkChannel(t, env)
		writes := settingsMreqCountWrites(t)
		helper := env.newOutsider()
		env.fake.setMember(env.chat.Id, helper.Id, staffFakeMember{
			Status: gotgbot.ChatMemberStatusAdministrator, CanRestrictMembers: false,
		})
		msgID := settingsMreqOpenPage(t, env, helper)
		for i, code := range settingsMreqRuleCodes {
			env.press(helper, msgID, settingsMreqRuleFields(env, helper, "mo", code))
			text, keyboard := env.lastEdit(env.chat.Id, msgID)
			settingsMreqWantRuleScreen(t, text, i, 0)
			settingsMreqWantRuleKeyboard(t, keyboard, i, 0)
		}

		refused := func(name string, fields map[string]string) {
			t.Helper()
			edits, answers := len(env.edits(env.chat.Id, msgID)), env.answerCount()
			env.press(helper, msgID, fields)
			if got := env.answerCount() - answers; got != 1 {
				t.Errorf("%s answered %d times, want once", name, got)
			}
			if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("settings_no_restrict")) {
				t.Errorf("%s answered %q (alert %v), want the settings_no_restrict alert", name, answer, alert)
			}
			if got := len(env.edits(env.chat.Id, msgID)); got != edits {
				t.Errorf("%s edited the menu (a card must not even be drawn without the right)", name)
			}
		}
		for _, code := range settingsMreqRuleCodes {
			for v := 0; v <= 4; v++ {
				refused(fmt.Sprintf("st %s v=%d", code, v), settingsMreqRuleFields(env, helper, "st", code, "p", settingsPageMreq, "v", fmt.Sprint(v)))
			}
			for v := 2; v <= 4; v++ {
				refused(fmt.Sprintf("my %s v=%d", code, v), settingsMreqRuleFields(env, helper, "my", code, "v", fmt.Sprint(v), "e", settingsMreqExpiryMinute()))
			}
		}
		if _, has := settingsMreqStored(t, env.chat.Id); has || *writes != 0 {
			t.Errorf("a refused press wrote (row %v, %d writes)", has, *writes)
		}
	})

	t.Run("a plain member is not an administrator", func(t *testing.T) {
		env := newSettingsEnv(t)
		member := env.newOutsider()
		env.fake.setMember(env.chat.Id, member.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
		msgID := settingsMreqOpenPage(t, env, env.admin)
		edits := len(env.edits(env.chat.Id, msgID))
		env.press(member, msgID, settingsMreqSwitchFields(env, member, "tg", "j", "1"))
		if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("settings_not_admin")) {
			t.Errorf("answer = %q (alert %v), want settings_not_admin", answer, alert)
		}
		if got := len(env.edits(env.chat.Id, msgID)); got != edits {
			t.Error("the press edited the page")
		}
		if _, has := settingsMreqStored(t, env.chat.Id); has {
			t.Error("a refused press wrote a row")
		}
	})

	t.Run("the owner gate decides before the right", func(t *testing.T) {
		resetSettingsOwnerMemo(t)
		env := newSettingsEnv(t)
		env.newOwner(env.chat)
		helper := env.newOutsider()
		env.fake.setMember(env.chat.Id, helper.Id, staffFakeMember{
			Status: gotgbot.ChatMemberStatusAdministrator, CanRestrictMembers: false,
		})
		env.fake.listAdministrator(env.chat.Id, helper.Id)
		env.setAdminsAllowed(env.chat.Id, false)
		const menuMsgID = 4747
		env.press(helper, menuMsgID, settingsMreqSwitchFields(env, helper, "tg", "j", "1"))
		if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("settings_gate_owner_only")) {
			t.Errorf("answer = %q (alert %v), want settings_gate_owner_only before any right is judged", answer, alert)
		}
		if _, has := settingsMreqStored(t, env.chat.Id); has {
			t.Error("a gated press wrote a row")
		}
	})

	t.Run("the creator and an administrator with the right write both ways", func(t *testing.T) {
		env := newSettingsEnv(t)
		owner := env.newOwner(env.chat)
		for _, opener := range []gotgbot.User{owner, env.admin} {
			msgID := settingsMreqOpenPage(t, env, opener)
			env.press(opener, msgID, settingsMreqSwitchFields(env, opener, "tg", "j", "1"))
			if stored, _ := settingsMreqStored(t, env.chat.Id); !stored.CheckOnJoin {
				t.Errorf("opener %d: Check on join was not written", opener.Id)
			}
			env.press(opener, msgID, settingsMreqSwitchFields(env, opener, "tg", "j", "0"))
			if stored, has := settingsMreqStored(t, env.chat.Id); !has || stored.CheckOnJoin {
				t.Errorf("opener %d: Check on join was not turned off (row %v)", opener.Id, has)
			}
		}
	})
}

// settingsMreqStatuses are the statuses a draw can carry, by name: nothing read (the Phase 13
// shape), a linked channel with an administrator bot, a bot that is not an administrator, no linked
// channel, a read that could not be answered, and a basic group.
var settingsMreqStatuses = []struct {
	Name   string
	Status settingsMreqStatus
}{
	{"unread", settingsMreqStatus{}},
	{"ok", settingsMreqStatus{Channel: settingsMreqChannelOK}},
	{"notadmin", settingsMreqStatus{Channel: settingsMreqChannelNotAdmin}},
	{"nolink", settingsMreqStatus{Channel: settingsMreqChannelNoLink}},
	{"unknown", settingsMreqStatus{Channel: settingsMreqChannelUnknown}},
	{"basic", settingsMreqStatus{Basic: true}},
}

// settingsMreqOffOnlyScreen reports whether a rule's screen under a status is the Off-only channel
// screen (two rows).
func settingsMreqOffOnlyScreen(rule memberReqRule, status settingsMreqStatus) bool {
	return rule.Code == "ch" && (status.Basic || status.Channel == settingsMreqChannelNoLink)
}

// settingsMreqScreens renders the page in the states that matter for size: every rule off, and
// every punishment used once with both switches on, each without a notice and under the notices a
// press can put on it and under every status, then every rule screen under every status and every
// card.
func settingsMreqScreens(tr *i18n.Translator, view settingsView) map[string]settingsScreen {
	states := map[string]*models.ChatRequirements{
		"all off": {},
		"every punishment": {
			LastNameAction: models.RequirementActionWarn, UsernameAction: models.RequirementActionKick,
			RussianAction: models.RequirementActionBan, ArabicAction: models.RequirementActionMute,
			ChineseAction: models.RequirementActionMute, PhotoAction: models.RequirementActionWarn,
			ChannelAction: models.RequirementActionKick, CheckOnJoin: true, DeleteMessages: true,
		},
	}
	notices := map[string]string{
		"no notice":   "",
		"save failed": settingsNoticeWarn(settingsText(tr, "common_settings_save_failed")),
		"expired":     settingsNoticeWarn(settingsText(tr, "settings_card_expired")),
	}
	screens := map[string]settingsScreen{}
	for stateName, row := range states {
		for noticeName, notice := range notices {
			for _, st := range settingsMreqStatuses {
				screens[stateName+", "+noticeName+", "+st.Name] = renderSettingsMreqPageStatus(view, row, notice, st.Status)
			}
		}
	}
	// The seven rule screens (every punishment used once, each under every notice and every
	// status) and the twenty-one cards.
	for _, rule := range memberReqRules {
		for noticeName, notice := range notices {
			for _, st := range settingsMreqStatuses {
				screens["rule "+rule.Code+", "+noticeName+", "+st.Name] = renderSettingsMreqRuleScreenStatus(view, rule, states["every punishment"], notice, st.Status)
			}
		}
		for _, action := range []int{models.RequirementActionKick, models.RequirementActionBan, models.RequirementActionMute} {
			screens[fmt.Sprintf("card %s %d", rule.Code, action)] = renderSettingsMreqCard(view, rule, action)
		}
	}
	return screens
}

func TestSettingsMreqPageFitsEveryLocale(t *testing.T) {
	withRealLocales(t)
	titles := map[string]string{
		"ampersands": strings.Repeat("&", 64),
		"astral":     strings.Repeat("😀", 64),
		"multibyte":  strings.Repeat("Ж", 64),
	}
	for _, code := range []string{"uk", "en", "es", "fr", "hi", "id", "pt", "ru"} {
		t.Run(code, func(t *testing.T) {
			tr := i18n.MustNewTranslator(code)
			for titleName, title := range titles {
				for _, private := range []bool{false, true} {
					view := settingsEditView(tr, private)
					view.GroupTitle, view.OpenerName = title, title
					screens := settingsMreqScreens(tr, view)
					if want := 2*3*6 + 7*3*6 + 21; len(screens) != want {
						t.Fatalf("%s %s: %d screens were rendered, want %d (36 page states, 126 rule screens, 21 cards)", code, titleName, len(screens), want)
					}
					for name, screen := range screens {
						label := fmt.Sprintf("%s %s private=%v %s", code, titleName, private, name)
						if units := panelUTF16Len(screen.Text); units == 0 || units > 3800 {
							t.Errorf("%s is %d UTF-16 units, want 1..3800", label, units)
						}
						for _, placeholder := range []string{"{state}", "{rule}", "{action}", "{page}", "%!"} {
							if strings.Contains(screen.Text, placeholder) {
								t.Errorf("%s keeps the unfilled %s: %q", label, placeholder, screen.Text)
							}
						}
						if screen.Omitted != 0 {
							t.Errorf("%s omitted %d buttons for overflow", label, screen.Omitted)
						}
						for _, leak := range []string{"ZZ Staff", settingsStaffPrivacyDigits} {
							if strings.Contains(settingsStaffPrivacyScreenText(screen), leak) {
								t.Errorf("%s names a Staff Group marker %q", label, leak)
							}
						}
						if rows, limit := len(screen.Keyboard.InlineKeyboard), map[bool]int{true: 8, false: 9}[private]; rows > limit {
							t.Errorf("%s has %d rows, want at most %d", label, rows, limit)
						}
						for _, button := range settingsAllButtons(screen.Keyboard.InlineKeyboard) {
							if strings.TrimSpace(button.Text) == "" {
								t.Errorf("%s has a button with an empty label", label)
							}
							if n := len(button.CallbackData); n < 1 || n > 64 {
								t.Errorf("%s button %q has %d bytes of callback data, want 1..64", label, button.Text, n)
							}
						}
					}
				}
			}
		})
	}
}

// settingsMreqCodeShape is what a callback field of this page may hold: a whole number, or a
// code of one or two lower-case letters. Never a name or a text.
var settingsMreqCodeShape = regexp.MustCompile(`^(-?[0-9]+|[a-z]{1,2})$`)

func TestSettingsMreqCallbackBytes(t *testing.T) {
	tr := i18n.MustNewTranslator("en")
	for _, private := range []bool{true, false} {
		view := settingsEditView(tr, private)
		label := fmt.Sprintf("private=%v", private)
		wantMain, wantSwitch := 33, 41
		if private {
			wantMain, wantSwitch = 37, 45
		}

		main := renderSettingsMain(view, []settingsMainEntry{
			{Key: settingsPageMreq, Icon: "🛂", Title: settingsText(tr, "settings_page_mreq"), State: settingsText(tr, "settings_state_off_plain"), Known: true},
		}, "")
		grid, ok := settingsPageButton(t, main.Keyboard.InlineKeyboard, settingsPageMreq)
		if !ok {
			t.Fatalf("%s: the Main grid has no Requirements button", label)
		}
		if n := len(grid.CallbackData); n != wantMain {
			t.Errorf("%s: the Main grid button has %d bytes, want %d", label, n, wantMain)
		}

		screen := renderSettingsMreqPage(view, &models.ChatRequirements{
			LastNameAction: models.RequirementActionWarn, PhotoAction: models.RequirementActionMute,
			ChannelAction: models.RequirementActionBan, CheckOnJoin: true, DeleteMessages: true,
		}, "")
		if screen.Omitted != 0 {
			t.Errorf("%s: the page omitted %d buttons for overflow", label, screen.Omitted)
		}
		if len(screen.Keyboard.InlineKeyboard) != 6 {
			t.Errorf("%s: the page has %d rows, want the grid, the switch row and the navigation row", label, len(screen.Keyboard.InlineKeyboard))
		}
		for _, button := range settingsAllButtons(screen.Keyboard.InlineKeyboard) {
			for key, value := range settingsButtonFields(t, button) {
				if !settingsMreqCodeShape.MatchString(value) {
					t.Errorf("%s: button %q field %s=%q is not a number or a code", label, button.Text, key, value)
				}
			}
		}
		for _, button := range screen.Keyboard.InlineKeyboard[4] {
			if n := len(button.CallbackData); n != wantSwitch {
				t.Errorf("%s: the switch button %q has %d bytes, want %d", label, button.Text, n, wantSwitch)
			}
		}

		// Phase 13 plan 06, now seven cells (the ph and ch codes are two letters like the others, so no
		// callback grows): the rule cells, the choices, the card and the rule screen's Back.
		wantCell, wantChoice, wantYes, wantNo := 34, 42, 49, 33
		if private {
			wantCell, wantChoice, wantYes, wantNo = 38, 46, 53, 37
		}
		checkBytes := func(what string, screen settingsScreen, buttons []gotgbot.InlineKeyboardButton, want int) {
			t.Helper()
			if screen.Omitted != 0 {
				t.Errorf("%s: %s omitted %d buttons for overflow", label, what, screen.Omitted)
			}
			for _, button := range buttons {
				if n := len(button.CallbackData); n != want {
					t.Errorf("%s: %s button %q has %d bytes, want %d", label, what, button.Text, n, want)
				}
				for key, value := range settingsButtonFields(t, button) {
					if !settingsMreqCodeShape.MatchString(value) {
						t.Errorf("%s: %s button %q field %s=%q is not a number or a code", label, what, button.Text, key, value)
					}
				}
			}
		}
		checkBytes("rule cell", screen, settingsAllButtons(screen.Keyboard.InlineKeyboard[:4]), wantCell)
		// The status screens add body lines and, for the channel rule, drop a choice row: no
		// button grows, and the single [Off] of the Off-only screen is a choice button like the rest.
		channelRule, _ := memberReqRuleByCode("ch")
		for _, st := range settingsMreqStatuses {
			for _, rule := range memberReqRules {
				rs := renderSettingsMreqRuleScreenStatus(view, rule, &models.ChatRequirements{}, "", st.Status)
				choiceRows := 2
				if settingsMreqOffOnlyScreen(rule, st.Status) {
					choiceRows = 1
				}
				checkBytes("choice under "+st.Name, rs, settingsAllButtons(rs.Keyboard.InlineKeyboard[:choiceRows]), wantChoice)
				checkBytes("Back under "+st.Name, rs, rs.Keyboard.InlineKeyboard[choiceRows][:1], wantMain)
			}
			checkBytes("page cell under "+st.Name, renderSettingsMreqPageStatus(view, &models.ChatRequirements{}, "", st.Status),
				settingsAllButtons(renderSettingsMreqPageStatus(view, &models.ChatRequirements{}, "", st.Status).Keyboard.InlineKeyboard[:4]), wantCell)
		}
		offOnly := renderSettingsMreqRuleScreenStatus(view, channelRule, &models.ChatRequirements{}, "", settingsMreqStatus{Channel: settingsMreqChannelNoLink})
		if len(offOnly.Keyboard.InlineKeyboard) != 2 || len(offOnly.Keyboard.InlineKeyboard[0]) != 1 {
			t.Errorf("%s: the Off-only screen keyboard = %+v, want [Off] and [Back | Close]", label, offOnly.Keyboard.InlineKeyboard)
		}
		for _, rule := range memberReqRules {
			rs := renderSettingsMreqRuleScreen(view, rule, &models.ChatRequirements{}, "")
			checkBytes("choice", rs, settingsAllButtons(rs.Keyboard.InlineKeyboard[:2]), wantChoice)
			checkBytes("rule screen Back", rs, rs.Keyboard.InlineKeyboard[2][:1], wantMain)
			for _, action := range []int{models.RequirementActionKick, models.RequirementActionBan, models.RequirementActionMute} {
				card := renderSettingsMreqCard(view, rule, action)
				if len(card.Keyboard.InlineKeyboard) != 2 {
					t.Fatalf("%s: the card has %d rows, want 2", label, len(card.Keyboard.InlineKeyboard))
				}
				checkBytes("card Yes", card, card.Keyboard.InlineKeyboard[0][:1], wantYes)
				checkBytes("card No", card, card.Keyboard.InlineKeyboard[0][1:], wantNo)
			}
		}
	}
}

// settingsMreqDescKeys are the contract's "what breaks it" keys in rule order.
var settingsMreqDescKeys = []string{
	"settings_mreq_desc_lastname", "settings_mreq_desc_username", "settings_mreq_desc_russian",
	"settings_mreq_desc_arabic", "settings_mreq_desc_chinese", "settings_mreq_desc_photo",
	"settings_mreq_desc_channel",
}

// settingsMreqHintKeys is the hint a rule's screen carries right after its description, by rule
// order: the blank last name hint, and the photo rule's Warn or Mute recommendation (D-04).
var settingsMreqHintKeys = []string{"settings_mreq_hint_blank", "", "", "", "", "settings_mreq_hint_photo", ""}

// settingsMreqWantRuleScreen fails the test unless text is the rule screen of rule i with the given
// stored punishment: the rule's name, what breaks it, the blank hint on the last-name rule only,
// the punishment line and the two hints, in order, and nothing of the page or of another rule.
func settingsMreqWantRuleScreen(t *testing.T, text string, i, action int) {
	t.Helper()
	markers := []string{
		staffMarker("settings_header") + " " + staffMarker("settings_page_mreq"),
		staffMarker(settingsMreqRuleKeys[i]),
		staffMarker(settingsMreqDescKeys[i]),
	}
	if settingsMreqHintKeys[i] != "" {
		markers = append(markers, staffMarker(settingsMreqHintKeys[i]))
	}
	markers = append(markers,
		staffMarker("settings_mreq_now")+" "+settingsMreqIcons[action]+" "+staffMarker(settingsMreqWordKeys[action]),
		staffMarker("settings_mreq_choose_hint"),
		staffMarker("settings_mreq_actions_hint"),
	)
	at := 0
	for _, marker := range markers {
		idx := strings.Index(text[at:], marker)
		if idx < 0 {
			t.Fatalf("the rule screen %q lacks %q after offset %d", text, marker, at)
		}
		at += idx + len(marker)
	}
	for hintRule, hintKey := range settingsMreqHintKeys {
		if hintKey != "" && strings.Contains(text, staffMarker(hintKey)) != (hintRule == i) {
			t.Errorf("rule %d: the hint %s present = %v, want it on rule %d only", i, hintKey, hintRule != i, hintRule)
		}
	}
	for other, key := range settingsMreqRuleKeys {
		if other != i && strings.Contains(text, staffMarker(key)) {
			t.Errorf("rule %d screen names rule %d: %q", i, other, text)
		}
	}
	for _, page := range []string{"settings_mreq_intro", "settings_mreq_hint_letters", "settings_mreq_hint_join"} {
		if strings.Contains(text, staffMarker(page)) {
			t.Errorf("rule %d screen carries the page text %s", i, page)
		}
	}
}

// settingsMreqWantRuleKeyboard fails the test unless the keyboard is the rule screen's three rows:
// [Off | Warn], [Kick | Ban | Mute] with the check mark on the stored value only, each pressing st
// with its absolute value, then Back to the page (a=op, p=m) and Close.
func settingsMreqWantRuleKeyboard(t *testing.T, keyboard [][]gotgbot.InlineKeyboardButton, i, action int) {
	t.Helper()
	if len(keyboard) != 3 {
		t.Fatalf("the rule screen has %d rows, want 3: %+v", len(keyboard), keyboard)
	}
	v := 0
	for r, width := range []int{2, 3} {
		if len(keyboard[r]) != width {
			t.Fatalf("choice row %d has %d buttons, want %d", r, len(keyboard[r]), width)
		}
		for _, button := range keyboard[r] {
			want := staffMarker(settingsMreqWordKeys[v])
			if v == action {
				want = "✅ " + want
			}
			if button.Text != want {
				t.Errorf("choice %d = %q, want %q", v, button.Text, want)
			}
			fields := settingsButtonFields(t, button)
			if fields["a"] != "st" || fields["p"] != settingsPageMreq || fields["t"] != settingsMreqRuleCodes[i] || fields["v"] != fmt.Sprint(v) || len(fields) != 5 {
				t.Errorf("choice %d fields = %v, want st, p=m, t=%s, v=%d and the owner", v, fields, settingsMreqRuleCodes[i], v)
			}
			v++
		}
	}
	nav := keyboard[2]
	if len(nav) != 2 || !containsMarker(nav[0].Text, "settings_btn_back") || !containsMarker(nav[1].Text, "settings_btn_close") {
		t.Fatalf("navigation row = %+v, want Back and Close", nav)
	}
	if back := settingsButtonFields(t, nav[0]); back["a"] != "op" || back["p"] != settingsPageMreq || len(back) != 3 {
		t.Errorf("Back fields = %v, want a=op and p=m (the page, not Main)", back)
	}
	if closeFields := settingsButtonFields(t, nav[1]); closeFields["a"] != "cl" {
		t.Errorf("Close fields = %v, want a=cl", closeFields)
	}
}

// settingsMreqRuleFields is the field set of a menu button of the rule screen of a group menu.
func settingsMreqRuleFields(env *settingsEnv, opener gotgbot.User, action, code string, kv ...string) map[string]string {
	return env.groupFields(opener, append([]string{"a", action, "t", code}, kv...)...)
}

// settingsMreqCountWrites makes the rule write seam count its calls, then runs the real write.
func settingsMreqCountWrites(t *testing.T) *int {
	t.Helper()
	settingsMreqSeams(t)
	calls := 0
	real := memberReqWrites.Action
	memberReqWrites.Action = func(ctx context.Context, chatID int64, rule memberreq.Rule, action int) error {
		calls++
		return real(ctx, chatID, rule, action)
	}
	return &calls
}

// TestSettingsMreqPhotoScreen proves the photo rule's screen text and keyboard (D-01, D-04): the
// rule name, the description, the Warn-or-Mute hint right after it with no blank line between, a
// blank line, the Punishment line, a blank line and the two Phase 13 hints; no other rule shows the
// hint, the last-name rule keeps the blank-name hint in the same slot, and opening the screen makes
// no Telegram request beyond the press pipeline's own member lookup.
func TestSettingsMreqPhotoScreen(t *testing.T) {
	env := newSettingsEnv(t)
	owner := env.newOwner(env.chat)
	msgID := settingsMreqOpenPage(t, env, owner)

	methods := []string{"getUserProfilePhotos", "getChat", "getChatAdministrators", "getChatMemberCount"}
	callCounts := func() map[string]int {
		counts := map[string]int{}
		for _, method := range methods {
			counts[method] = len(env.fake.callsFor(method))
		}
		return counts
	}
	before, lookups := callCounts(), env.memberLookupsOf(env.chat.Id, owner.Id)
	env.press(owner, msgID, settingsMreqRuleFields(env, owner, "mo", "ph"))
	text, keyboard := env.lastEdit(env.chat.Id, msgID)
	settingsMreqWantRuleScreen(t, text, 5, 0)
	settingsMreqWantRuleKeyboard(t, keyboard, 5, 0)
	for method, count := range callCounts() {
		if count != before[method] {
			t.Errorf("opening the photo screen made %d %s calls, want none", count-before[method], method)
		}
	}
	if got := env.memberLookupsOf(env.chat.Id, owner.Id) - lookups; got != 1 {
		t.Errorf("opening the photo screen made %d getChatMember calls of the presser, want the pipeline's one", got)
	}
	exact := staffMarker("settings_mreq_desc_photo") + "\n" + staffMarker("settings_mreq_hint_photo") + "\n\n" +
		staffMarker("settings_mreq_now") + " " + settingsMreqIcons[0] + " " + staffMarker(settingsMreqWordKeys[0]) + "\n\n" +
		staffMarker("settings_mreq_choose_hint") + "\n" + staffMarker("settings_mreq_actions_hint")
	if !strings.Contains(text, exact) {
		t.Errorf("the photo screen %q lacks the exact tail %q", text, exact)
	}

	// The check mark follows the stored value, and the last-name rule keeps its own hint.
	settingsMreqSeed(t, env.chat.Id, [7]int{0, 0, 0, 0, 0, models.RequirementActionMute}, false, false)
	env.press(owner, msgID, settingsMreqRuleFields(env, owner, "mo", "ph"))
	text, keyboard = env.lastEdit(env.chat.Id, msgID)
	settingsMreqWantRuleScreen(t, text, 5, models.RequirementActionMute)
	settingsMreqWantRuleKeyboard(t, keyboard, 5, models.RequirementActionMute)
	env.press(owner, msgID, settingsMreqRuleFields(env, owner, "mo", "ln"))
	text, _ = env.lastEdit(env.chat.Id, msgID)
	settingsMreqWantRuleScreen(t, text, 0, 0)
}

func TestSettingsMreqRuleScreen(t *testing.T) {
	env := newSettingsEnv(t)
	settingsMreqLinkChannel(t, env)
	owner := env.newOwner(env.chat)
	writes := settingsMreqCountWrites(t)
	msgID := settingsMreqOpenPage(t, env, owner)

	t.Run("the page carries the rule grid above the switch row", func(t *testing.T) {
		_, keyboard := env.lastEdit(env.chat.Id, msgID)
		settingsMreqWantGrid(t, keyboard, [7]int{})
		settingsMreqWantSwitchRow(t, keyboard, false, false)
	})

	t.Run("each cell opens its own screen", func(t *testing.T) {
		for i, code := range settingsMreqRuleCodes {
			env.press(owner, msgID, settingsMreqRuleFields(env, owner, "mo", code))
			text, keyboard := env.lastEdit(env.chat.Id, msgID)
			settingsMreqWantRuleScreen(t, text, i, 0)
			settingsMreqWantRuleKeyboard(t, keyboard, i, 0)
		}
		if _, has := settingsMreqStored(t, env.chat.Id); has || *writes != 0 {
			t.Errorf("opening the rule screens wrote (row %v, %d writes)", has, *writes)
		}
	})

	t.Run("Warn and Off write at once and redraw the rule screen and the page", func(t *testing.T) {
		env.press(owner, msgID, settingsMreqRuleFields(env, owner, "st", "ar", "p", settingsPageMreq, "v", "1"))
		stored, has := settingsMreqStored(t, env.chat.Id)
		if !has || settingsMreqActionsOf(stored) != [7]int{0, 0, 0, 1, 0} {
			t.Fatalf("stored = %+v (row %v), want only the Arabic rule on Warn", stored, has)
		}
		text, keyboard := env.lastEdit(env.chat.Id, msgID)
		settingsMreqWantRuleScreen(t, text, 3, 1)
		settingsMreqWantRuleKeyboard(t, keyboard, 3, 1)
		if strings.Contains(text, "⚠️") {
			t.Errorf("the redraw carries a notice: %q", text)
		}

		env.press(owner, msgID, env.groupFields(owner, "a", "op", "p", settingsPageMreq))
		pageText, pageKeyboard := env.lastEdit(env.chat.Id, msgID)
		settingsMreqWantPage(t, pageText, [7]int{0, 0, 0, 1, 0}, false, false)
		settingsMreqWantGrid(t, pageKeyboard, [7]int{0, 0, 0, 1, 0})

		env.press(owner, msgID, settingsMreqRuleFields(env, owner, "st", "ar", "p", settingsPageMreq, "v", "0"))
		stored, _ = settingsMreqStored(t, env.chat.Id)
		if stored.ArabicAction != 0 {
			t.Errorf("Off stored %d, want 0 (a Go zero is written)", stored.ArabicAction)
		}
		text, keyboard = env.lastEdit(env.chat.Id, msgID)
		settingsMreqWantRuleScreen(t, text, 3, 0)
		settingsMreqWantRuleKeyboard(t, keyboard, 3, 0)
	})

	t.Run("pressing the stored value writes nothing", func(t *testing.T) {
		env.press(owner, msgID, settingsMreqRuleFields(env, owner, "st", "un", "p", settingsPageMreq, "v", "1"))
		before := *writes
		env.press(owner, msgID, settingsMreqRuleFields(env, owner, "st", "un", "p", settingsPageMreq, "v", "1"))
		if *writes != before {
			t.Errorf("a press of the stored value made %d writes, want none", *writes-before)
		}
		text, keyboard := env.lastEdit(env.chat.Id, msgID)
		settingsMreqWantRuleScreen(t, text, 1, 1)
		settingsMreqWantRuleKeyboard(t, keyboard, 1, 1)
	})

	t.Run("a forged mo is refused before any lookup", func(t *testing.T) {
		for name, fields := range map[string]map[string]string{
			"without t":      env.groupFields(owner, "a", "mo"),
			"an unknown t":   env.groupFields(owner, "a", "mo", "t", "xx"),
			"an empty t":     env.groupFields(owner, "a", "mo", "t", "ln "),
			"an extra key":   env.groupFields(owner, "a", "mo", "t", "ln", "v", "1"),
			"a page key too": env.groupFields(owner, "a", "mo", "t", "ln", "p", settingsPageMreq),
		} {
			lookups, edits, answers := env.memberLookupsOf(env.chat.Id, owner.Id), len(env.edits(env.chat.Id, msgID)), env.answerCount()
			env.press(owner, msgID, fields)
			if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("common_callback_invalid_request")) {
				t.Errorf("%s answered %q (alert %v), want the invalid request alert", name, answer, alert)
			}
			if got := env.answerCount() - answers; got != 1 {
				t.Errorf("%s answered %d times, want once", name, got)
			}
			if got := env.memberLookupsOf(env.chat.Id, owner.Id); got != lookups {
				t.Errorf("%s made %d getChatMember calls, want none", name, got-lookups)
			}
			if got := len(env.edits(env.chat.Id, msgID)); got != edits {
				t.Errorf("%s edited the message", name)
			}
		}
	})
}

func TestSettingsControlActionAllowList(t *testing.T) {
	env := newSettingsEnv(t)
	owner := env.newOwner(env.chat)
	msgID := settingsMreqOpenPage(t, env, owner)
	storedActions := func() [7]int {
		stored, _ := settingsMreqStored(t, env.chat.Id)
		return settingsMreqActionsOf(stored)
	}
	refused := func(name string, fields map[string]string) {
		t.Helper()
		lookups, edits, answers := env.memberLookupsOf(env.chat.Id, owner.Id), len(env.edits(env.chat.Id, msgID)), env.answerCount()
		env.press(owner, msgID, fields)
		if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("common_callback_invalid_request")) {
			t.Errorf("%s answered %q (alert %v), want the invalid request alert", name, answer, alert)
		}
		if got := env.answerCount() - answers; got != 1 {
			t.Errorf("%s answered %d times, want once", name, got)
		}
		if got := env.memberLookupsOf(env.chat.Id, owner.Id); got != lookups {
			t.Errorf("%s made %d getChatMember calls before refusing, want none", name, got-lookups)
		}
		if got := len(env.edits(env.chat.Id, msgID)); got != edits {
			t.Errorf("%s edited the message", name)
		}
	}

	t.Run("tg on a rule control is refused in step 1", func(t *testing.T) {
		refused("tg ln", settingsMreqSwitchFields(env, owner, "tg", "ln", "1"))
		if _, has := settingsMreqStored(t, env.chat.Id); has {
			t.Error("a refused tg wrote a row")
		}
	})

	t.Run("st on a rule control is accepted and a bad value is not", func(t *testing.T) {
		env.press(owner, msgID, settingsMreqSwitchFields(env, owner, "st", "ln", "1"))
		if got := storedActions(); got != [7]int{1, 0, 0, 0, 0} {
			t.Fatalf("st ln v=1 stored %v, want the last-name rule on Warn", got)
		}
		for _, bad := range []string{"5", "04", "x", "", "-1", "10"} {
			refused("st ln v="+bad, settingsMreqSwitchFields(env, owner, "st", "ln", bad))
		}
		if got := storedActions(); got != [7]int{1, 0, 0, 0, 0} {
			t.Errorf("a refused value changed the rules to %v", got)
		}
	})

	t.Run("tg on every rule control, the photo and channel rules included, is refused in step 1", func(t *testing.T) {
		for _, code := range settingsMreqRuleCodes {
			refused("tg "+code, settingsMreqSwitchFields(env, owner, "tg", code, "1"))
		}
		if got := storedActions(); got != [7]int{1, 0, 0, 0, 0} {
			t.Errorf("a refused tg changed the rules to %v", got)
		}
	})

	t.Run("st on the photo and channel rules is accepted and a bad value is not", func(t *testing.T) {
		settingsMreqLinkChannel(t, env)
		for _, code := range []string{"ph", "ch"} {
			env.press(owner, msgID, settingsMreqSwitchFields(env, owner, "st", code, "1"))
		}
		if got := storedActions(); got != [7]int{1, 0, 0, 0, 0, 1, 1} {
			t.Fatalf("st ph and ch v=1 stored %v, want both new rules on Warn", got)
		}
		for _, code := range []string{"ph", "ch"} {
			for _, bad := range []string{"5", "04", "x", "", "-1"} {
				refused("st "+code+" v="+bad, settingsMreqSwitchFields(env, owner, "st", code, bad))
			}
		}
		if got := storedActions(); got != [7]int{1, 0, 0, 0, 0, 1, 1} {
			t.Errorf("a refused value changed the rules to %v", got)
		}
	})

	t.Run("tg and st stay accepted on the two switches", func(t *testing.T) {
		for _, action := range []string{"tg", "st"} {
			for _, control := range []string{"j", "d"} {
				env.press(owner, msgID, settingsMreqSwitchFields(env, owner, action, control, "1"))
				stored, _ := settingsMreqStored(t, env.chat.Id)
				if (control == "j" && !stored.CheckOnJoin) || (control == "d" && !stored.DeleteMessages) {
					t.Errorf("%s on %s did not write: %+v", action, control, stored)
				}
				env.press(owner, msgID, settingsMreqSwitchFields(env, owner, action, control, "0"))
			}
		}
	})

	t.Run("every control of the other pages still accepts tg and st alike", func(t *testing.T) {
		checked := 0
		for _, pg := range settingsPagesInOrder() {
			if pg.Key == settingsPageMreq {
				continue
			}
			for code := range pg.Controls {
				fields := map[string]string{"p": pg.Key, "t": code, "v": "1"}
				tg, st := settingsControlFieldsValid("tg", fields), settingsControlFieldsValid("st", fields)
				if tg != st {
					t.Errorf("page %s control %s: tg accepted %v, st accepted %v, want the same (no allow-list)", pg.Key, code, tg, st)
				}
				checked++
			}
		}
		if checked < 8 {
			t.Fatalf("only %d controls of the other pages were checked", checked)
		}
		for _, c := range []struct{ page, control string }{{settingsPageCaptcha, settingsCaptchaEnabled}, {settingsPageAccess, settingsAccessControlSwitch}} {
			fields := map[string]string{"p": c.page, "t": c.control, "v": "1"}
			if !settingsControlFieldsValid("tg", fields) || !settingsControlFieldsValid("st", fields) {
				t.Errorf("page %s control %s must accept both tg and st", c.page, c.control)
			}
		}
	})
}

func TestApplyMemberReqAction(t *testing.T) {
	chatID := uniqueModuleChatID()
	settingsCleanup(t, chatID)
	writes := settingsMreqCountWrites(t)
	ctx := context.Background()

	for _, bad := range []struct {
		name   string
		rule   memberreq.Rule
		action int
	}{
		{"an action above 4", memberreq.RuleArabic, 5},
		{"a negative action", memberreq.RuleArabic, -1},
		{"an unknown rule", memberreq.Rule("profile_photo"), 1},
	} {
		result := applyMemberReqAction(ctx, chatID, bad.rule, bad.action)
		if result.Outcome != memberReqSaveFailed || result.Key != "common_settings_save_failed" || result.Err == nil {
			t.Errorf("%s: result = %+v, want a refused save with the save-failed key", bad.name, result)
		}
		if *writes != 0 {
			t.Fatalf("%s reached the write seam", bad.name)
		}
	}
	if _, has := settingsMreqStored(t, chatID); has {
		t.Error("a refused apply created a row")
	}

	// A cached read first: the apply must drop the key, so the next cached read sees the write.
	if _, err := memberreq.GetSettings(ctx, chatID); err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	result := applyMemberReqAction(ctx, chatID, memberreq.RuleChinese, models.RequirementActionMute)
	if result.Outcome != memberReqApplied || result.Row == nil || result.Row.ChineseAction != models.RequirementActionMute {
		t.Fatalf("result = %+v, want applied with the Chinese rule on Mute read back", result)
	}
	if cached, err := memberreq.GetSettings(ctx, chatID); err != nil || cached.ChineseAction != models.RequirementActionMute {
		t.Errorf("the cached read after the write = %+v, %v, want the new value (the key was dropped)", cached, err)
	}
	if result = applyMemberReqAction(ctx, chatID, memberreq.RuleChinese, models.RequirementActionOff); result.Row == nil || result.Row.ChineseAction != 0 {
		t.Errorf("Off result = %+v, want a Go zero written and read back", result)
	}
}

func TestSettingsMreqRowBudget(t *testing.T) {
	tr := i18n.MustNewTranslator("en")
	everyPunishment := &models.ChatRequirements{
		LastNameAction: models.RequirementActionWarn, UsernameAction: models.RequirementActionKick,
		RussianAction: models.RequirementActionBan, ArabicAction: models.RequirementActionMute,
		ChineseAction: models.RequirementActionOff, PhotoAction: models.RequirementActionWarn,
		ChannelAction: models.RequirementActionMute, CheckOnJoin: true, DeleteMessages: true,
	}
	for _, private := range []bool{true, false} {
		view := settingsEditView(tr, private)
		label := fmt.Sprintf("private=%v", private)

		for _, st := range settingsMreqStatuses {
			page := renderSettingsMreqPageStatus(view, everyPunishment, "", st.Status)
			if rows := len(page.Keyboard.InlineKeyboard); rows != 6 {
				t.Errorf("%s %s: the page has %d rows, want 6 (the four grid rows, the switches, the navigation)", label, st.Name, rows)
			}
			for r, widths := range []int{2, 2, 2, 1, 2, 2} {
				if got := len(page.Keyboard.InlineKeyboard[r]); got != widths {
					t.Errorf("%s %s: page row %d has %d buttons, want %d", label, st.Name, r, got, widths)
				}
			}
			for _, button := range settingsAllButtons(page.Keyboard.InlineKeyboard) {
				if button.Url != "" {
					t.Errorf("%s %s: the page has a URL button %q, want no Open in private row", label, st.Name, button.Text)
				}
			}
			for _, rule := range memberReqRules {
				want := 3
				if settingsMreqOffOnlyScreen(rule, st.Status) {
					want = 2
				}
				if rows := len(renderSettingsMreqRuleScreenStatus(view, rule, everyPunishment, "", st.Status).Keyboard.InlineKeyboard); rows != want {
					t.Errorf("%s %s: the %s rule screen has %d rows, want %d", label, st.Name, rule.Code, rows, want)
				}
			}
		}
		for _, rule := range memberReqRules {
			for _, action := range []int{models.RequirementActionKick, models.RequirementActionBan, models.RequirementActionMute} {
				if rows := len(renderSettingsMreqCard(view, rule, action).Keyboard.InlineKeyboard); rows != 2 {
					t.Errorf("%s: the %s card has %d rows, want 2", label, rule.Code, rows)
				}
			}
		}
	}
}
