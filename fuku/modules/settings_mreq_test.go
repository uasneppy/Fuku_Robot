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
		settingsMreqWantPage(t, text, [5]int{}, c.join, c.del)
		settingsMreqWantSwitchRow(t, keyboard, c.join, c.del)
		if strings.Contains(text, "⚠️") {
			t.Errorf("%s: the redraw carries a notice: %q", c.name, text)
		}
	}

	t.Run("a value that is not 0 or 1 is refused before any lookup", func(t *testing.T) {
		for _, bad := range []struct{ action, control, value string }{
			{"tg", "j", "2"}, {"tg", "d", "x"}, {"st", "j", "01"}, {"st", "d", ""}, {"tg", "zz", "1"}, {"st", "ln", "1"},
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
		settingsMreqSeed(t, env.chat.Id, [5]int{1, 0, 0, 3, 0}, false, true)
		for _, c := range []struct{ control, value string }{{"j", "1"}, {"d", "0"}} {
			press("tg", c.control, c.value)
			text, keyboard := env.lastEdit(env.chat.Id, msgID)
			if want := settingsNoticeWarnMarker("common_settings_save_failed"); !strings.Contains(text, want) {
				t.Errorf("control %s: the redraw %q lacks the notice %q", c.control, text, want)
			}
			settingsMreqWantPage(t, text, [5]int{1, 0, 0, 3, 0}, false, true)
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
	seeded := [5]int{1, 2, 3, 4, 0}

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
		settingsMreqWantPage(t, text, [5]int{0, 0, 0, 2, 0}, true, false)
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
		if got := [5]int{stored.LastNameAction, stored.UsernameAction, stored.RussianAction, stored.ArabicAction, stored.ChineseAction}; got != seeded || !stored.CheckOnJoin || !stored.DeleteMessages {
			t.Errorf("after Check on join: %+v, want the actions %v, join on and delete still on", stored, seeded)
		}
		cached, err := memberreq.GetSettings(context.Background(), env.chat.Id)
		if err != nil || !cached.CheckOnJoin {
			t.Errorf("the cached read after the write = %+v, %v, want the new value (the key was dropped)", cached, err)
		}

		env.press(owner, msgID, settingsMreqSwitchFields(env, owner, "tg", "d", "0"))
		stored, _ = settingsMreqStored(t, env.chat.Id)
		if got := [5]int{stored.LastNameAction, stored.UsernameAction, stored.RussianAction, stored.ArabicAction, stored.ChineseAction}; got != seeded || !stored.CheckOnJoin || stored.DeleteMessages {
			t.Errorf("after Delete messages: %+v, want the actions %v, join still on and delete off", stored, seeded)
		}
		cached, err = memberreq.GetSettings(context.Background(), env.chat.Id)
		if err != nil || cached.DeleteMessages {
			t.Errorf("the cached read after the write = %+v, %v, want delete off (the key was dropped)", cached, err)
		}
		text, _ := env.lastEdit(env.chat.Id, msgID)
		settingsMreqWantPage(t, text, seeded, true, false)
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
		settingsMreqWantPage(t, text, [5]int{}, false, false)
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

// settingsMreqScreens renders the page in the states that matter for size: every rule off, and
// every punishment used once with both switches on, each without a notice and under the notices a
// press can put on it.
func settingsMreqScreens(tr *i18n.Translator, view settingsView) map[string]settingsScreen {
	states := map[string]*models.ChatRequirements{
		"all off": {},
		"every punishment": {
			LastNameAction: models.RequirementActionWarn, UsernameAction: models.RequirementActionKick,
			RussianAction: models.RequirementActionBan, ArabicAction: models.RequirementActionMute,
			ChineseAction: models.RequirementActionMute, CheckOnJoin: true, DeleteMessages: true,
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
			screens[stateName+", "+noticeName] = renderSettingsMreqPage(view, row, notice)
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
					if len(screens) != 6 {
						t.Fatalf("%s %s: %d screens were rendered, want 6", code, titleName, len(screens))
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

		screen := renderSettingsMreqPage(view, &models.ChatRequirements{CheckOnJoin: true, DeleteMessages: true}, "")
		if screen.Omitted != 0 {
			t.Errorf("%s: the page omitted %d buttons for overflow", label, screen.Omitted)
		}
		if len(screen.Keyboard.InlineKeyboard) != 2 {
			t.Errorf("%s: the page has %d rows, want the switch row and the navigation row", label, len(screen.Keyboard.InlineKeyboard))
		}
		for _, button := range settingsAllButtons(screen.Keyboard.InlineKeyboard) {
			for key, value := range settingsButtonFields(t, button) {
				if !settingsMreqCodeShape.MatchString(value) {
					t.Errorf("%s: button %q field %s=%q is not a number or a code", label, button.Text, key, value)
				}
			}
		}
		for _, button := range screen.Keyboard.InlineKeyboard[0] {
			if n := len(button.CallbackData); n != wantSwitch {
				t.Errorf("%s: the switch button %q has %d bytes, want %d", label, button.Text, n, wantSwitch)
			}
		}
	}
}
