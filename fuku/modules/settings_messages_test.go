//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db/eventmsg"
	"github.com/uasneppy/fuku_robot/fuku/db/staff"
)

// settingsMsgButtonWith returns the first button whose fields carry every given pair.
func settingsMsgButtonWith(t *testing.T, keyboard [][]gotgbot.InlineKeyboardButton, kv ...string) (gotgbot.InlineKeyboardButton, bool) {
	t.Helper()
	for _, button := range settingsAllButtons(keyboard) {
		decoded, ok := decodeCallbackData(button.CallbackData, settingsCallbackNamespace)
		if !ok {
			continue
		}
		match := true
		for i := 0; i+1 < len(kv); i += 2 {
			if decoded.Fields[kv[i]] != kv[i+1] {
				match = false
				break
			}
		}
		if match {
			return button, true
		}
	}
	return gotgbot.InlineKeyboardButton{}, false
}

// settingsMsgWantWidths fails the test unless the keyboard's rows have the given widths.
func settingsMsgWantWidths(t *testing.T, what string, keyboard [][]gotgbot.InlineKeyboardButton, want ...int) {
	t.Helper()
	if got := settingsListRowWidths(keyboard); !slices.Equal(got, want) {
		t.Errorf("%s rows = %v, want %v", what, got, want)
	}
}

// settingsMsgWantStates fails the test unless every event of the manual family has its line with the
// expected state word in text (events not in want read Default).
func settingsMsgWantStates(t *testing.T, text string, want map[string]string) {
	t.Helper()
	for _, ev := range settingsMsgManualEvents {
		word := "settings_state_default"
		if w, ok := want[ev.Key]; ok {
			word = w
		}
		line, ok := settingsMainStateLine(text, ev.NameKey)
		if !ok {
			t.Errorf("no line for %s in %q", ev.Key, text)
			continue
		}
		if !strings.Contains(line, ev.Icon) || !strings.Contains(line, staffMarker(word)) {
			t.Errorf("%s line = %q, want the icon %s and %s", ev.Key, line, ev.Icon, word)
		}
	}
}

func TestSettingsMessagesOpens(t *testing.T) {
	env := newSettingsMsgEnv(t)
	settingsMsgSeed(t, env.chat.Id, eventmsg.EventBan, "Banned {mention}", false)
	settingsMsgSeed(t, env.chat.Id, eventmsg.EventKick, "", true)

	// Main carries the Messages cell.
	m := settingsMsgPrivateMenu(env, env.chat, env.admin)
	_, mainKeyboard := m.press("a", settingsActBack)
	cell, ok := settingsMsgButtonWith(t, mainKeyboard, "a", settingsActOpen, "p", settingsPageMessages)
	if !ok || !strings.HasPrefix(cell.Text, "📨 ") || !containsMarker(cell.Text, "settings_page_msgs") {
		t.Fatalf("Main has no 📨 Messages cell: %+v", mainKeyboard)
	}

	// The hub: intro, the family heading, seven event lines with their states, [1] [2].
	text, keyboard := m.pressButton(cell)
	for _, key := range []string{"settings_msg_intro", "settings_msg_fam_manual"} {
		if !containsMarker(text, key) {
			t.Errorf("the hub %q lacks %s", text, key)
		}
	}
	settingsMsgWantStates(t, text, map[string]string{
		eventmsg.EventBan: "settings_msg_state_custom", eventmsg.EventKick: "settings_msg_state_silent",
	})
	settingsMsgWantWidths(t, "the hub", keyboard, 1, 2)
	if back := settingsButtonFields(t, keyboard[1][0]); back["a"] != settingsActBack {
		t.Errorf("the hub's Back carries %v, want Back to Main", back)
	}

	// The family list: seven cells [2 2 2 1 2] and Back to the hub.
	family, ok := settingsMsgButtonWith(t, keyboard, "a", settingsActMsgFamily, "t", "m")
	if !ok {
		t.Fatalf("the hub has no Admin actions cell: %+v", keyboard)
	}
	text, keyboard = m.pressButton(family)
	if !containsMarker(text, "settings_msg_fam_manual_intro") || !containsMarker(text, "settings_msg_list_hint") {
		t.Errorf("the family list %q lacks its intro or hint", text)
	}
	settingsMsgWantStates(t, text, map[string]string{
		eventmsg.EventBan: "settings_msg_state_custom", eventmsg.EventKick: "settings_msg_state_silent",
	})
	settingsMsgWantWidths(t, "the family list", keyboard, 2, 2, 2, 1, 2)
	if back := settingsButtonFields(t, keyboard[4][0]); back["a"] != settingsActOpen || back["p"] != settingsPageMessages {
		t.Errorf("the family list's Back carries %v, want the hub", back)
	}

	// The ban screen: its name, description and part states, the part rows, the Silent toggle and Back to the
	// family.
	ban, ok := settingsMsgButtonWith(t, keyboard, "a", settingsActMsgEvent, settingsMsgEventField, "bn")
	if !ok || !strings.HasPrefix(ban.Text, "🚫 ") {
		t.Fatalf("the family list has no 🚫 ban cell: %+v", keyboard)
	}
	text, keyboard = m.pressButton(ban)
	for _, key := range []string{"settings_msg_ev_ban", "settings_msg_desc_ban", "settings_edit_line_text", "settings_edit_line_media", "settings_edit_line_buttons"} {
		if !containsMarker(text, key) {
			t.Errorf("the ban screen %q lacks %s", text, key)
		}
	}
	if want := staffMarker("settings_edit_state_text_len") + " " + fmt.Sprint(len("Banned {mention}")); !strings.Contains(text, want) {
		t.Errorf("the ban screen %q lacks the stored text's length %q", text, want)
	}
	if n := strings.Count(text, staffMarker("settings_state_not_set")); n != 2 {
		t.Errorf("the ban screen reads Not set %d times, want 2 (media and buttons):\n%s", n, text)
	}
	// The text is set, so its row is [Set | Remove]; media and buttons have Set alone; then the toggle and
	// Back and Close.
	settingsMsgWantWidths(t, "the ban screen", keyboard, 2, 1, 1, 1, 2)
	if back := settingsButtonFields(t, keyboard[4][0]); back["a"] != settingsActMsgFamily || back["t"] != "m" {
		t.Errorf("the ban screen's Back carries %v, want the family", back)
	}

	// A chat with nothing stored creates no row through any of these draws.
	other := env.addChat()
	o := settingsMsgPrivateMenu(env, other, env.admin)
	o.press("a", settingsActBack)
	text, _ = o.press("a", settingsActOpen, "p", settingsPageMessages)
	settingsMsgWantStates(t, text, nil)
	o.press("a", settingsActMsgFamily, "t", "m")
	text, _ = o.press("a", settingsActMsgEvent, settingsMsgEventField, "mu")
	if !strings.Contains(text, staffMarker("settings_state_default")) {
		t.Errorf("the mute screen of an empty chat %q does not read Default for its text", text)
	}
	if n := settingsMsgRows(t, other.Id); n != 0 {
		t.Errorf("drawing the page created %d event message rows, want none", n)
	}
}

func TestSettingsMessagesMainState(t *testing.T) {
	env := newSettingsMsgEnv(t)
	mainLine := func(t *testing.T) string {
		t.Helper()
		m := settingsMsgPrivateMenu(env, env.chat, env.admin)
		text, _ := m.press("a", settingsActBack)
		line, ok := settingsMainStateLine(text, "settings_page_msgs")
		if !ok {
			t.Fatalf("Main has no Messages line: %q", text)
		}
		return line
	}

	if line := mainLine(t); !containsMarker(line, "settings_state_default") {
		t.Errorf("Main's Messages line with nothing stored = %q, want Default", line)
	}

	settingsMsgSeed(t, env.chat.Id, eventmsg.EventTimedBan, "Out for a while", false)
	settingsMsgSeed(t, env.chat.Id, eventmsg.EventUnmute, "", true)
	if line, want := mainLine(t), staffMarker("settings_state_msg_n")+" 2"; !strings.Contains(line, want) {
		t.Errorf("Main's Messages line with two events customised = %q, want %q", line, want)
	}

	mreqSwap(t, &settingsMsgRead, func(context.Context, int64) (*eventmsg.Messages, error) {
		return nil, errors.New("event messages are unreadable")
	})
	if line := mainLine(t); !strings.HasSuffix(line, settingsUnknownState) {
		t.Errorf("Main's Messages line with a failing read = %q, want the unknown mark", line)
	}
	_, text, keyboard := settingsMsgOpenPrivate(env, env.admin)
	if !containsMarker(text, "settings_cannot_open") || len(keyboard) != 0 {
		t.Errorf("opening the page with a failing read drew %q with %d rows, want the cannot-open edit", text, len(keyboard))
	}
}

func TestSettingsMessagesGroupMenu(t *testing.T) {
	env := newSettingsMsgEnv(t)
	settingsMsgSeed(t, env.chat.Id, eventmsg.EventBan, "zzsentinelmessage", false)
	reads := 0
	mreqSwap(t, &settingsMsgRead, func(ctx context.Context, chatID int64) (*eventmsg.Messages, error) {
		reads++
		return eventmsg.GetFresh(ctx, chatID)
	})

	msgID := env.openMenu(env.admin)
	text, _ := env.lastEdit(env.chat.Id, msgID)
	if line, ok := settingsMainStateLine(text, "settings_page_msgs"); !ok || !containsMarker(line, "settings_state_private_only") {
		t.Errorf("a group Main's Messages line = %q, want In private", line)
	}

	env.press(env.admin, msgID, env.groupFields(env.admin, "a", settingsActOpen, "p", settingsPageMessages))
	text, keyboard := env.lastEdit(env.chat.Id, msgID)
	if !containsMarker(text, "settings_msg_group_note") || containsMarker(text, "settings_msg_intro") || strings.Contains(text, "zzsentinel") {
		t.Errorf("the group page drew %q, want the Messages note alone", text)
	}
	if urls, want := settingsURLButtons(keyboard), settingsPrivateURL("FukuTestBot", env.chat.Id); len(urls) != 1 || urls[0] != want {
		t.Errorf("Open in private URL buttons = %v, want [%s]", urls, want)
	}
	settingsMsgWantWidths(t, "the group note", keyboard, 1, 2)

	for _, forged := range [][]string{
		{"a", settingsActMsgFamily, "t", "m"},
		{"a", settingsActMsgEvent, settingsMsgEventField, "bn"},
	} {
		lookups, edits := env.memberLookupsOf(env.chat.Id, env.admin.Id), len(env.edits(env.chat.Id, msgID))
		env.press(env.admin, msgID, env.groupFields(env.admin, forged...))
		if got := settingsEditLastAnswer(env.settingsEnv); !containsMarker(got, "common_callback_invalid_request") {
			t.Errorf("a forged group press %v answered %q, want the invalid request", forged, got)
		}
		if got := env.memberLookupsOf(env.chat.Id, env.admin.Id); got != lookups {
			t.Errorf("a forged group press %v made %d member lookups, want none", forged, got-lookups)
		}
		if got := len(env.edits(env.chat.Id, msgID)); got != edits {
			t.Errorf("a forged group press %v edited the menu", forged)
		}
	}
	if reads != 0 {
		t.Errorf("a group menu read the event messages %d times, want never", reads)
	}
}

func TestSettingsMessagesStaffGroup(t *testing.T) {
	env := newSettingsMsgEnv(t)
	if _, err := staff.CreateStaffGroup(env.chat.Id, env.admin.Id, "Staff"); err != nil {
		t.Fatalf("CreateStaffGroup(%d) error = %v", env.chat.Id, err)
	}

	m := settingsMsgPrivateMenu(env, env.chat, env.admin)
	text, keyboard := m.press("a", settingsActBack)
	if _, ok := settingsMsgButtonWith(t, keyboard, "a", settingsActOpen, "p", settingsPageMessages); ok {
		t.Errorf("a Staff Group's Main has a Messages cell: %+v", keyboard)
	}
	if _, ok := settingsMainStateLine(text, "settings_page_msgs"); ok {
		t.Errorf("a Staff Group's Main has a Messages line: %q", text)
	}

	for _, forged := range [][]string{
		{"a", settingsActOpen, "p", settingsPageMessages},
		{"a", settingsActMsgEvent, settingsMsgEventField, "bn"},
	} {
		edits := len(env.edits(env.admin.Id, m.msgID))
		env.pressPrivate(env.admin, m.msgID, env.privateFields(env.chat.Id, forged...))
		if got := settingsEditLastAnswer(env.settingsEnv); !containsMarker(got, "common_callback_invalid_request") {
			t.Errorf("a forged Staff Group press %v answered %q, want the invalid request", forged, got)
		}
		if got := len(env.edits(env.admin.Id, m.msgID)); got != edits {
			t.Errorf("a forged Staff Group press %v edited the menu", forged)
		}
	}
}
