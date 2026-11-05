//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// settingsMreqIcons and settingsMreqWordKeys are the UI contract's punishment icons and the reused
// option words, by stored action 0..4 (written out here, not read from the code under test).
var (
	settingsMreqIcons    = []string{"❌", "❗", "👢", "⛔", "🔇"}
	settingsMreqWordKeys = []string{"settings_state_off_plain", "settings_opt_warn", "settings_opt_kick", "settings_opt_ban", "settings_opt_mute"}
	settingsMreqRuleKeys = []string{
		"settings_mreq_rule_lastname", "settings_mreq_rule_username", "settings_mreq_rule_russian",
		"settings_mreq_rule_arabic", "settings_mreq_rule_chinese",
	}
)

// settingsMreqSeed replaces the requirements row of chatID with the given actions and switches
// through a direct write, then drops the cached read on every layer.
func settingsMreqSeed(t *testing.T, chatID int64, actions [5]int, join, del bool) {
	t.Helper()
	if err := db.DB.Where("chat_id = ?", chatID).Delete(&models.ChatRequirements{}).Error; err != nil {
		t.Fatalf("clear the requirements row of %d: %v", chatID, err)
	}
	row := &models.ChatRequirements{
		ChatID: chatID, LastNameAction: actions[0], UsernameAction: actions[1], RussianAction: actions[2],
		ArabicAction: actions[3], ChineseAction: actions[4], CheckOnJoin: join, DeleteMessages: del,
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
func settingsMreqPageMarkers(actions [5]int, join, del bool) []string {
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
func settingsMreqWantPage(t *testing.T, text string, actions [5]int, join, del bool) {
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

// settingsMreqWantSwitchRow fails the test unless the keyboard is the switch row and the
// navigation row, with each switch cell showing the given state icon.
func settingsMreqWantSwitchRow(t *testing.T, keyboard [][]gotgbot.InlineKeyboardButton, join, del bool) {
	t.Helper()
	if len(keyboard) != 2 {
		t.Fatalf("the page has %d keyboard rows, want the switch row and the navigation row: %+v", len(keyboard), keyboard)
	}
	if len(keyboard[0]) != 2 {
		t.Fatalf("the switch row has %d buttons, want 2", len(keyboard[0]))
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
		button := keyboard[0][i]
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
	nav := keyboard[1]
	if len(nav) != 2 || !containsMarker(nav[0].Text, "settings_btn_back") || !containsMarker(nav[1].Text, "settings_btn_close") {
		t.Errorf("navigation row = %+v, want Back and Close", nav)
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
		settingsMreqWantPage(t, text, [5]int{}, false, false)
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
		settingsMreqWantPage(t, text, [5]int{}, false, false)
		settingsMreqWantSwitchRow(t, keyboard, false, false)
		if _, has := settingsMreqStored(t, env.chat.Id); has {
			t.Error("opening the private page created a chat_requirements row")
		}
	})
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
		settingsMreqSeed(t, env.chat.Id, [5]int{}, true, true)
		if line := redrawMain(); !strings.Contains(line, staffMarker("settings_state_off_plain")) {
			t.Errorf("line = %q, want the Off word", line)
		}
	})

	for n := 1; n <= 5; n++ {
		t.Run(fmt.Sprintf("%d rules on", n), func(t *testing.T) {
			var actions [5]int
			for i := 0; i < n; i++ {
				actions[i] = []int{1, 2, 3, 4, 1}[i]
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
		settingsMreqSeed(t, env.chat.Id, [5]int{1}, false, false)
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

// settingsMreqSeams restores the read seam of the page when the test ends.
func settingsMreqSeams(t *testing.T) {
	t.Helper()
	lookup := memberReqSettingsLookup
	t.Cleanup(func() { memberReqSettingsLookup = lookup })
}
