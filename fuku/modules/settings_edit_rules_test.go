//go:build testtools

package modules

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/db/rules"
)

// settingsRulesSeedAll stores rules with the text "Be kind", a photo and two buttons for chatID, through
// the repository's own writers.
func settingsRulesSeedAll(t *testing.T, chatID int64) {
	t.Helper()
	if err := rules.SetChatRules(chatID, "Be kind"); err != nil {
		t.Fatalf("SetChatRules: %v", err)
	}
	if err := rules.SetRulesMediaOnly(chatID, "rules-photo", db.PHOTO); err != nil {
		t.Fatalf("SetRulesMediaOnly: %v", err)
	}
	buttons := []models.Button{{Name: "One", Url: "https://one.example"}, {Name: "Two", Url: "https://two.example"}}
	if err := rules.SetRulesButtonsOnly(chatID, buttons); err != nil {
		t.Fatalf("SetRulesButtonsOnly: %v", err)
	}
}

func TestSettingsEditRulesParts(t *testing.T) {
	env := newSettingsEditEnv(t)
	owner := env.newOwner(env.chat)
	settingsRulesSeedAll(t, env.chat.Id)
	menuID := settingsEditPrivateMenu(env, owner, settingsPageRules)

	text, keyboard := env.lastEdit(owner.Id, menuID)
	for _, want := range []string{
		staffMarker("settings_edit_state_text_len") + " 7",
		staffMarker("settings_edit_line_media") + " " + staffMarker("settings_edit_media_photo"),
		staffMarker("settings_edit_state_buttons_n") + " 2",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the Rules page %q lacks %q", text, want)
		}
	}
	if got := strings.Join(settingsEditRemoveParts(t, keyboard, "rr"), ","); got != "x,m,b" {
		t.Fatalf("Remove buttons on a fully set Rules page = %q, want x,m,b", got)
	}
	for _, label := range []string{"settings_btn_edit_set_text", "settings_btn_edit_set_media", "settings_btn_edit_set_buttons"} {
		if !settingsEditHasButton(keyboard, label) {
			t.Errorf("the Rules page has no %s button", label)
		}
	}

	// Remove media clears the file and the type only.
	answers := env.answerCount()
	settingsEditPressButton(env, owner, menuID, "settings_btn_edit_remove_media")
	if got := env.answerCount() - answers; got != 1 {
		t.Errorf("Remove was answered %d times, want once", got)
	}
	row, ok := settingsEditRulesStored(t, env.chat.Id)
	if !ok || row.FileID != "" || row.MediaType != 0 || row.Rules != "Be kind" || len(row.Buttons) != 2 {
		t.Errorf("after Remove media the rules are %+v (found %v), want no media with the text and buttons kept", row, ok)
	}
	text, keyboard = env.lastEdit(owner.Id, menuID)
	if !strings.Contains(text, settingsNoticeOKIcon+staffMarker("settings_edit_removed_media")) {
		t.Errorf("the page %q lacks the removed-media notice", text)
	}
	if want := staffMarker("settings_edit_line_media") + " " + staffMarker("settings_state_not_set"); !strings.Contains(text, want) {
		t.Errorf("the page %q lacks %q after Remove media", text, want)
	}
	if got := strings.Join(settingsEditRemoveParts(t, keyboard, "rr"), ","); got != "x,b" {
		t.Errorf("Remove buttons after Remove media = %q, want x,b", got)
	}

	// Set media from a private menu writes the wait for part m.
	settingsEditPressButton(env, owner, menuID, "settings_btn_edit_set_media")
	wait := settingsEditWaitOf(env, owner)
	if wait["c"] != strconv.FormatInt(env.chat.Id, 10) || wait["p"] != "r" || wait["t"] != "m" {
		t.Fatalf("wait = %v, want the group, page r and part m", wait)
	}
	if prompt, _ := env.lastEdit(owner.Id, menuID); !strings.Contains(prompt, staffMarker("settings_edit_wait_media")) {
		t.Errorf("the prompt %q lacks settings_edit_wait_media", prompt)
	}
}

// settingsRulesSeedKept stores the rules columns that no Remove and no /clearrules may touch: Media under text, the
// private switch and the rules button.
func settingsRulesSeedKept(t *testing.T, chatID int64) {
	t.Helper()
	if err := rules.SetRulesMediaBelow(chatID, true); err != nil {
		t.Fatalf("SetRulesMediaBelow: %v", err)
	}
	if err := rules.SetPrivateRules(chatID, true); err != nil {
		t.Fatalf("SetPrivateRules: %v", err)
	}
	if err := rules.SetChatRulesButton(chatID, "Read me"); err != nil {
		t.Fatalf("SetChatRulesButton: %v", err)
	}
}

// settingsRulesKeptOK reports whether the row still has Media under text, private rules and the rules button.
func settingsRulesKeptOK(row models.RulesSettings) bool {
	return row.MediaBelow && row.Private && row.RulesBtn == "Read me"
}

// TestSettingsEditRulesRemoveParity: the three Remove presses of the Rules page store exactly what /clearrules
// stores, and neither touches Media under text, private rules or the rules button.
func TestSettingsEditRulesRemoveParity(t *testing.T) {
	env := newSettingsEditEnv(t)

	t.Run("three Removes equal /clearrules", func(t *testing.T) {
		menuChat, commandChat := env.addChat(), env.addChat()
		editCleanup(t, menuChat.Id, commandChat.Id)
		menuOwner, commandOwner := env.newOwner(menuChat), env.newOwner(commandChat)
		for _, id := range []int64{menuChat.Id, commandChat.Id} {
			settingsRulesSeedAll(t, id)
			settingsRulesSeedKept(t, id)
		}
		for _, part := range []string{"x", "m", "b"} {
			env.pressPrivate(menuOwner, 8181, map[string]string{"a": "rr", "t": part, "c": strconv.FormatInt(menuChat.Id, 10)})
		}
		env.sendIn(commandChat, commandOwner, "/clearrules")

		menuRow, menuOK := settingsEditRulesStored(t, menuChat.Id)
		commandRow, commandOK := settingsEditRulesStored(t, commandChat.Id)
		if !menuOK || !commandOK {
			t.Fatalf("a row is missing: menu %v, command %v", menuOK, commandOK)
		}
		for name, row := range map[string]models.RulesSettings{"menu": menuRow, "command": commandRow} {
			if row.Rules != "" || row.FileID != "" || row.MediaType != 0 || len(row.Buttons) != 0 {
				t.Errorf("%s left %+v, want no text, media or buttons", name, row)
			}
			if !settingsRulesKeptOK(row) {
				t.Errorf("%s changed Media under text, private rules or the rules button: %+v", name, row)
			}
		}
	})

	t.Run("a Remove of an empty part writes nothing", func(t *testing.T) {
		chat := env.addChat()
		editCleanup(t, chat.Id)
		owner := env.newOwner(chat)
		if err := rules.SetChatRules(chat.Id, "Be kind"); err != nil {
			t.Fatalf("SetChatRules: %v", err)
		}
		menuID := int64(8282)
		for _, part := range []string{"m", "b"} {
			env.pressPrivate(owner, menuID, map[string]string{"a": "rr", "t": part, "c": strconv.FormatInt(chat.Id, 10)})
			text, _ := env.lastEdit(owner.Id, menuID)
			if !strings.Contains(text, settingsNoticeWarnIcon+staffMarker("settings_edit_nothing_to_remove")) {
				t.Errorf("a stale Remove of %s drew %q, want the nothing-to-remove notice", part, text)
			}
		}
		if row, _ := settingsEditRulesStored(t, chat.Id); row.Rules != "Be kind" {
			t.Errorf("a stale Remove changed the rules to %+v", row)
		}
	})

	t.Run("a failed write redraws the old state", func(t *testing.T) {
		chat := env.addChat()
		editCleanup(t, chat.Id)
		owner := env.newOwner(chat)
		settingsRulesSeedAll(t, chat.Id)
		saved := rulesWrites.SetRulesMediaOnly
		rulesWrites.SetRulesMediaOnly = func(int64, string, int) error { return errors.New("write refused") }
		t.Cleanup(func() { rulesWrites.SetRulesMediaOnly = saved })

		menuID := int64(8383)
		env.pressPrivate(owner, menuID, map[string]string{"a": "rr", "t": "m", "c": strconv.FormatInt(chat.Id, 10)})
		text, keyboard := env.lastEdit(owner.Id, menuID)
		if !strings.Contains(text, settingsNoticeWarnIcon+staffMarker("common_settings_save_failed")) {
			t.Errorf("a failed Remove drew %q, want the save-failed notice", text)
		}
		if got := strings.Join(settingsEditRemoveParts(t, keyboard, "rr"), ","); got != "x,m,b" {
			t.Errorf("the old state lost Remove buttons: %q", got)
		}
		if row, _ := settingsEditRulesStored(t, chat.Id); row.FileID != "rules-photo" || row.MediaType != db.PHOTO {
			t.Errorf("a failed write changed the media: %+v", row)
		}
	})
}

// TestSettingsEditRulesMainState: Main reads Set for rules that hold a text, a media or a button (HasContent) and
// Not set for none of them, and the page and Main read through the side-effect-free loader.
func TestSettingsEditRulesMainState(t *testing.T) {
	env := newSettingsEditEnv(t)
	for _, tc := range []struct {
		name string
		seed func(t *testing.T, chatID int64)
		want string
	}{
		{"no row", func(*testing.T, int64) {}, "settings_state_not_set"},
		{"text only", func(t *testing.T, id int64) {
			if err := rules.SetChatRules(id, "Be kind"); err != nil {
				t.Fatal(err)
			}
		}, "settings_state_set"},
		{"media only", func(t *testing.T, id int64) {
			if err := rules.SetRulesMediaOnly(id, "rules-photo", db.PHOTO); err != nil {
				t.Fatal(err)
			}
		}, "settings_state_set"},
		{"buttons only", func(t *testing.T, id int64) {
			if err := rules.SetRulesButtonsOnly(id, []models.Button{{Name: "One", Url: "https://one.example"}}); err != nil {
				t.Fatal(err)
			}
		}, "settings_state_set"},
		{"only Media under text and the label", func(t *testing.T, id int64) { settingsRulesSeedKept(t, id) }, "settings_state_not_set"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			editCleanup(t, env.chat.Id)
			tc.seed(t, env.chat.Id)
			msgID := env.openMenu(env.admin)
			text, _ := env.lastEdit(env.chat.Id, msgID)
			line, found := settingsMainStateLine(text, "settings_page_rules")
			if !found || !strings.HasSuffix(line, staffMarker(tc.want)) {
				t.Errorf("the Rules status line %q (found %v) does not end with %s", line, found, tc.want)
			}
		})
	}
}

// settingsRulesBelowStored reads Media under text of the chat's rules straight from the database.
func settingsRulesBelowStored(t *testing.T, chatID int64) bool {
	t.Helper()
	row, ok := settingsEditRulesStored(t, chatID)
	return ok && row.MediaBelow
}

// TestSettingsRulesMediaBelow: the Rules page offers Media under text only for a photo, a video or an animation,
// the press stores it through applyRulesMediaBelow, /rules then sends the caption above the media, a stale press
// writes nothing, and the bot must be an administrator.
func TestSettingsRulesMediaBelow(t *testing.T) {
	for _, private := range []bool{true, false} {
		name := "group menu"
		if private {
			name = "private menu"
		}
		t.Run("a photo in the "+name, func(t *testing.T) {
			env := newSettingsEditEnv(t)
			owner := env.newOwner(env.chat)
			settingsRulesSeedAll(t, env.chat.Id)
			msgID, press := settingsEditPageFor(env, owner, settingsPageRules, private)
			shown := chatForPage(env, owner, private)

			text, keyboard := env.lastEdit(shown, msgID)
			if want := staffMarker("settings_edit_line_below") + " ❌ " + staffMarker("common_status_off"); !strings.Contains(text, want) {
				t.Errorf("the page %q lacks the off line %q", text, want)
			}
			below, fields := settingsEditControl(t, keyboard, "settings_btn_edit_switch_below", settingsEditControlBelow)
			if !strings.Contains(below.Text, "❌") || fields["a"] != settingsActToggle || fields["p"] != settingsPageRules || fields["v"] != "1" {
				t.Fatalf("the Media under text cell is %q with %v, want the off icon and tg on page r with v=1", below.Text, fields)
			}

			answers := env.answerCount()
			press(msgID, "a", "tg", "p", settingsPageRules, "t", "u", "v", "1")
			if got := env.answerCount() - answers; got != 1 {
				t.Errorf("the press was answered %d times, want once", got)
			}
			row, _ := settingsEditRulesStored(t, env.chat.Id)
			if !row.MediaBelow || row.Rules != "Be kind" || row.FileID != "rules-photo" || len(row.Buttons) != 2 {
				t.Fatalf("after the press the rules are %+v, want only media_below turned on", row)
			}
			text, keyboard = env.lastEdit(shown, msgID)
			if want := staffMarker("settings_edit_line_below") + " ✅ " + staffMarker("common_status_on"); !strings.Contains(text, want) {
				t.Errorf("after the press the page %q lacks the on line %q", text, want)
			}
			if settingsEditHasNoticeLine(text, settingsNoticeWarnIcon) || settingsEditHasNoticeLine(text, settingsNoticeOKIcon) {
				t.Errorf("a stored write redrew with a notice: %q", text)
			}
			if _, fields = settingsEditControl(t, keyboard, "settings_btn_edit_switch_below", settingsEditControlBelow); fields["v"] != "0" {
				t.Errorf("after the press the cell carries %v, want v=0", fields)
			}

			// The switch is the runtime's: /rules sends the photo with the caption above it.
			client, _ := rulesGroupCommand(t, env.chat.Id, "Test Chat")
			if params := rulesOnlyCall(t, client, "sendPhoto"); params["show_caption_above_media"] != true {
				t.Errorf("sendPhoto show_caption_above_media = %v, want true", params["show_caption_above_media"])
			}

			press(msgID, "a", "tg", "p", settingsPageRules, "t", "u", "v", "0")
			if settingsRulesBelowStored(t, env.chat.Id) {
				t.Error("a v=0 press left the switch on")
			}
		})
	}

	t.Run("kinds that cannot carry the caption above show no cell", func(t *testing.T) {
		for name, tc := range map[string]struct {
			mediaType int
			hint      bool
		}{
			"document": {db.DOCUMENT, true}, "audio": {db.AUDIO, false}, "sticker": {db.STICKER, false}, "none": {0, false},
		} {
			env := newSettingsEditEnv(t)
			owner := env.newOwner(env.chat)
			if tc.mediaType != 0 {
				if err := rules.SetRulesMediaOnly(env.chat.Id, "rules-file", tc.mediaType); err != nil {
					t.Fatalf("SetRulesMediaOnly: %v", err)
				}
			}
			msgID := settingsEditPrivateMenu(env, owner, settingsPageRules)
			text, keyboard := env.lastEdit(owner.Id, msgID)
			if strings.Contains(text, staffMarker("settings_edit_line_below")) || settingsEditHasButton(keyboard, "settings_btn_edit_switch_below") {
				t.Errorf("%s: the page %q draws Media under text", name, text)
			}
			if got := strings.Contains(text, staffMarker("settings_edit_hint_below_file")); got != tc.hint {
				t.Errorf("%s: the file hint is shown = %v, want %v", name, got, tc.hint)
			}
		}
	})

	t.Run("a press after the media became a sticker writes nothing", func(t *testing.T) {
		env := newSettingsEditEnv(t)
		owner := env.newOwner(env.chat)
		settingsRulesSeedAll(t, env.chat.Id)
		msgID, press := settingsEditPageFor(env, owner, settingsPageRules, true)
		if res := applyRulesMedia(t.Context(), env.chat.Id, "rules-sticker", db.STICKER); res.Outcome != greetingApplied {
			t.Fatalf("applyRulesMedia outcome = %v, err = %v", res.Outcome, res.Err)
		}
		answers := env.answerCount()
		press(msgID, "a", "tg", "p", settingsPageRules, "t", "u", "v", "1")
		if got := env.answerCount() - answers; got != 1 {
			t.Errorf("the stale press was answered %d times, want once", got)
		}
		if settingsRulesBelowStored(t, env.chat.Id) {
			t.Error("a stale press wrote media_below")
		}
		text, keyboard := env.lastEdit(owner.Id, msgID)
		if !strings.Contains(text, settingsNoticeWarnIcon+staffMarker("settings_edit_below_unavailable")) {
			t.Errorf("the stale press drew %q, want the warning settings_edit_below_unavailable", text)
		}
		if settingsEditHasButton(keyboard, "settings_btn_edit_switch_below") {
			t.Error("the page redrawn for a sticker still has the Media under text cell")
		}
	})

	t.Run("the bot must be an administrator", func(t *testing.T) {
		env := newSettingsEditEnv(t)
		env.fake.setBotRole(env.chat.Id, staffRoleMember)
		owner := env.newOwner(env.chat)
		settingsRulesSeedAll(t, env.chat.Id)
		answers, edits := env.answerCount(), len(env.edits(owner.Id, 9191))
		env.pressPrivate(owner, 9191, env.privateFields(env.chat.Id, "a", "tg", "p", settingsPageRules, "t", "u", "v", "1"))
		if got := env.answerCount() - answers; got != 1 {
			t.Errorf("the press was answered %d times, want once", got)
		}
		if got := len(env.edits(owner.Id, 9191)) - edits; got != 1 {
			t.Errorf("the press edited the message %d times, want the one redraw with the notice", got)
		}
		if text, _ := env.lastEdit(owner.Id, 9191); !strings.Contains(text, settingsNoticeWarnIcon+staffMarker("chat_status_bot_not_admin")) {
			t.Errorf("the refused press drew %q, want the notice chat_status_bot_not_admin", text)
		}
		if settingsRulesBelowStored(t, env.chat.Id) {
			t.Error("a refused press wrote media_below")
		}
	})

	t.Run("another control of the page is an invalid request", func(t *testing.T) {
		env := newSettingsEditEnv(t)
		owner := env.newOwner(env.chat)
		settingsRulesSeedAll(t, env.chat.Id)
		msgID := settingsEditPrivateMenu(env, owner, settingsPageRules)
		for _, control := range []string{"o", "e", "d"} {
			lookups := env.memberLookupsOf(env.chat.Id, owner.Id)
			edits := len(env.edits(owner.Id, msgID))
			env.pressPrivate(owner, msgID, env.privateFields(env.chat.Id, "a", "tg", "p", settingsPageRules, "t", control, "v", "1"))
			if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("common_callback_invalid_request")) {
				t.Errorf("t=%s answered %q (alert %v), want the alert common_callback_invalid_request", control, answer, alert)
			}
			if got := env.memberLookupsOf(env.chat.Id, owner.Id); got != lookups {
				t.Errorf("t=%s made %d getChatMember calls", control, got-lookups)
			}
			if got := len(env.edits(owner.Id, msgID)); got != edits {
				t.Errorf("t=%s edited the message %d times", control, got-edits)
			}
		}
		if settingsRulesBelowStored(t, env.chat.Id) {
			t.Error("an invalid press wrote media_below")
		}
	})
}
