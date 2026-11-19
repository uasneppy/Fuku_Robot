//go:build testtools

package modules

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/eventmsg"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// settingsMsgSilentStored reads the Silent switch of an event's group message straight from the database,
// past every cache layer, with the user who wrote it last. ok is false when the event has no row.
func settingsMsgSilentStored(t *testing.T, chatID int64, event string) (silent bool, by int64, ok bool) {
	t.Helper()
	var rows []models.ChatEventMessage
	if err := db.DB.Where("chat_id = ? AND event = ? AND dest = ?", chatID, event, eventmsg.DestGroup).Find(&rows).Error; err != nil {
		t.Fatalf("read the %s message of chat %d: %v", event, chatID, err)
	}
	if len(rows) == 0 {
		return false, 0, false
	}
	return rows[0].Silent, rows[0].UpdatedBy, true
}

// settingsMsgSilentToggle returns the Silent toggle of an event screen and its decoded fields, failing the
// test when the screen has none.
func settingsMsgSilentToggle(t *testing.T, keyboard [][]gotgbot.InlineKeyboardButton) (gotgbot.InlineKeyboardButton, map[string]string) {
	t.Helper()
	button, ok := settingsMsgButtonWith(t, keyboard, "a", settingsActMsgSilent)
	if !ok {
		t.Fatalf("the event screen has no Silent toggle: %+v", keyboard)
	}
	return button, settingsButtonFields(t, button)
}

// settingsMsgWantSilentScreen fails the test unless text and keyboard are the ban screen with the Silent
// switch drawn in the given state: the line with the state word, the hint, and a full-width toggle with the
// state icon that targets the opposite state, above Back and Close.
func settingsMsgWantSilentScreen(t *testing.T, text string, keyboard [][]gotgbot.InlineKeyboardButton, on bool) {
	t.Helper()
	word, icon, target := staffMarker("common_status_off"), "❌", "1"
	if on {
		word, icon, target = staffMarker("common_status_on"), "✅", "0"
	}
	if want := staffMarker("settings_msg_line_silent") + " " + icon + " " + word; !strings.Contains(text, want) {
		t.Errorf("the ban screen %q lacks the Silent line %q", text, want)
	}
	if !containsMarker(text, "settings_msg_hint_silent") {
		t.Errorf("the ban screen %q lacks the Silent hint", text)
	}
	settingsMsgWantWidths(t, "the ban screen", keyboard, 1, 2)
	toggle, fields := settingsMsgSilentToggle(t, keyboard)
	if keyboard[0][0].CallbackData != toggle.CallbackData {
		t.Errorf("the Silent toggle is not the first row: %+v", keyboard)
	}
	if want := staffMarker("settings_btn_msg_silent") + " " + icon; toggle.Text != want {
		t.Errorf("the Silent toggle reads %q, want %q", toggle.Text, want)
	}
	if fields[settingsMsgEventField] != "bn" || fields["v"] != target || fields["u"] != "" || fields["c"] == "" {
		t.Errorf("the Silent toggle carries %v, want k=bn, v=%s and the owner field c", fields, target)
	}
}

func TestSettingsMessagesSilent(t *testing.T) {
	env := newSettingsMsgEnv(t)
	listEditAdmin(env.settingsEnv, env.chat, env.admin, true)
	m := settingsMsgPrivateMenu(env, env.chat, env.admin)

	text, keyboard := m.press("a", settingsActMsgEvent, settingsMsgEventField, "bn")
	settingsMsgWantSilentScreen(t, text, keyboard, false)

	// The press stores the target, redraws with Silent On, answers once with no notice and the hub reads Silent.
	toggle, _ := settingsMsgSilentToggle(t, keyboard)
	answers := env.answerCount()
	text, keyboard = m.pressButton(toggle)
	if silent, by, ok := settingsMsgSilentStored(t, env.chat.Id, eventmsg.EventBan); !ok || !silent || by != env.admin.Id {
		t.Fatalf("after the press the ban row is silent=%v by %d (row %v), want silent by %d", silent, by, ok, env.admin.Id)
	}
	settingsMsgWantSilentScreen(t, text, keyboard, true)
	if got := env.answerCount() - answers; got != 1 {
		t.Errorf("the press answered %d times, want once", got)
	}
	if answer, alert := env.lastAnswer(); answer != "" || alert {
		t.Errorf("the press answered %q (alert %v), want an empty answer", answer, alert)
	}
	if containsMarker(text, "common_settings_save_failed") {
		t.Errorf("the redraw %q carries a notice", text)
	}
	hub, _ := m.press("a", settingsActOpen, "p", settingsPageMessages)
	settingsMsgWantStates(t, hub, map[string]string{eventmsg.EventBan: "settings_msg_state_silent"})

	// A second press stores false and the line reads Default again.
	text, keyboard = m.press("a", settingsActMsgEvent, settingsMsgEventField, "bn")
	toggle, _ = settingsMsgSilentToggle(t, keyboard)
	text, keyboard = m.pressButton(toggle)
	if silent, _, ok := settingsMsgSilentStored(t, env.chat.Id, eventmsg.EventBan); !ok || silent {
		t.Fatalf("after the second press the ban row is silent=%v (row %v), want not silent", silent, ok)
	}
	settingsMsgWantSilentScreen(t, text, keyboard, false)
	hub, _ = m.press("a", settingsActOpen, "p", settingsPageMessages)
	settingsMsgWantStates(t, hub, nil)

	// A stale screen's press of the stored value writes the same value again, with no notice.
	text, keyboard = m.press("a", settingsActMsgSilent, settingsMsgEventField, "bn", "v", "0")
	if silent, _, ok := settingsMsgSilentStored(t, env.chat.Id, eventmsg.EventBan); !ok || silent {
		t.Errorf("a stale press of Off left silent=%v (row %v), want not silent", silent, ok)
	}
	settingsMsgWantSilentScreen(t, text, keyboard, false)
	if containsMarker(text, "common_settings_save_failed") {
		t.Errorf("the stale press's redraw %q carries a notice", text)
	}

	// The toggle is 42 bytes at a 14-character chat ID.
	data := encodeCallbackData(settingsCallbackNamespace, map[string]string{
		"a": settingsActMsgSilent, "c": "-1001234567890", settingsMsgEventField: "bn", "v": "1",
	})
	if len(data) != 42 {
		t.Errorf("the Silent toggle's data %q is %d bytes, want 42", data, len(data))
	}

	t.Run("forged presses are refused before any lookup", func(t *testing.T) {
		msgID := env.openMenu(env.admin)
		forged := []struct {
			name  string
			group bool
			kv    []string
		}{
			{"v=2", false, []string{"a", settingsActMsgSilent, settingsMsgEventField, "bn", "v", "2"}},
			{"no v", false, []string{"a", settingsActMsgSilent, settingsMsgEventField, "bn"}},
			{"k=zz", false, []string{"a", settingsActMsgSilent, settingsMsgEventField, "zz", "v", "1"}},
			{"an extra t", false, []string{"a", settingsActMsgSilent, settingsMsgEventField, "bn", "v", "1", "t", "m"}},
			{"a group menu's u", true, []string{"a", settingsActMsgSilent, settingsMsgEventField, "bn", "v", "1"}},
		}
		for _, f := range forged {
			lookups := env.memberLookupsOf(env.chat.Id, env.admin.Id)
			if f.group {
				env.press(env.admin, msgID, env.groupFields(env.admin, f.kv...))
			} else {
				env.pressPrivate(env.admin, m.msgID, env.privateFields(env.chat.Id, f.kv...))
			}
			if got := settingsEditLastAnswer(env.settingsEnv); !containsMarker(got, "common_callback_invalid_request") {
				t.Errorf("%s: answered %q, want the invalid request", f.name, got)
			}
			if got := env.memberLookupsOf(env.chat.Id, env.admin.Id); got != lookups {
				t.Errorf("%s: made %d member lookups, want none", f.name, got-lookups)
			}
			if silent, _, _ := settingsMsgSilentStored(t, env.chat.Id, eventmsg.EventBan); silent {
				t.Errorf("%s: stored the ban message as silent", f.name)
			}
		}
	})

	t.Run("a failed write redraws the stored state with the save-failed notice", func(t *testing.T) {
		mreqSwap(t, &eventMsgWrites.Silent, func(context.Context, int64, string, string, bool, int64) error {
			return errors.New("the event messages are not writable")
		})
		text, keyboard := m.press("a", settingsActMsgSilent, settingsMsgEventField, "bn", "v", "1")
		if !containsMarker(text, "common_settings_save_failed") {
			t.Errorf("the failed press drew %q, want the save-failed notice", text)
		}
		settingsMsgWantSilentScreen(t, text, keyboard, false)
		if silent, _, ok := settingsMsgSilentStored(t, env.chat.Id, eventmsg.EventBan); !ok || silent {
			t.Errorf("the failed press left silent=%v (row %v), want the stored row unchanged", silent, ok)
		}
	})
}
