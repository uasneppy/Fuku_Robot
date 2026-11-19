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

// settingsMsgWantSilentScreen fails the test unless text and keyboard are the screen of the event code with
// the Silent switch drawn in the given state: the line with the state word, the hint, and a full-width toggle
// with the state icon that targets the opposite state, below the three part rows ([Set] or [Set | Remove])
// and the preview rows (one or two, the last ending with Full preview) and above Back and Close.
func settingsMsgWantSilentScreen(t *testing.T, text string, keyboard [][]gotgbot.InlineKeyboardButton, code string, on bool) {
	t.Helper()
	word, icon, target := staffMarker("common_status_off"), "❌", "1"
	if on {
		word, icon, target = staffMarker("common_status_on"), "✅", "0"
	}
	if want := staffMarker("settings_msg_line_silent") + " " + icon + " " + word; !strings.Contains(text, want) {
		t.Errorf("the %s screen %q lacks the Silent line %q", code, text, want)
	}
	if !containsMarker(text, "settings_msg_hint_silent") {
		t.Errorf("the %s screen %q lacks the Silent hint", code, text)
	}
	widths := settingsListRowWidths(keyboard)
	last := len(widths) - 1
	if len(widths) < 6 || len(widths) > 7 || widths[last-1] != 1 || widths[last] != 2 {
		t.Fatalf("the %s screen rows = %v, want three part rows, one or two preview rows, the toggle and Back and Close", code, widths)
	}
	for i := range 3 {
		if widths[i] < 1 || widths[i] > 2 || settingsButtonFields(t, keyboard[i][0])["a"] != settingsActMsgSet {
			t.Errorf("row %d of the %s screen is not a part row: %+v", i, code, keyboard[i])
		}
	}
	for i := 3; i < last-1; i++ {
		for _, button := range keyboard[i] {
			if a := settingsButtonFields(t, button)["a"]; a != settingsActMsgPreview && a != settingsActMsgFull {
				t.Errorf("row %d of the %s screen is not a preview row: %+v", i, code, keyboard[i])
			}
		}
	}
	if previews := keyboard[last-2]; settingsButtonFields(t, previews[len(previews)-1])["a"] != settingsActMsgFull {
		t.Errorf("the %s screen's preview rows do not end with Full preview: %+v", code, keyboard)
	}
	toggle, fields := settingsMsgSilentToggle(t, keyboard)
	if keyboard[last-1][0].CallbackData != toggle.CallbackData {
		t.Errorf("the Silent toggle is not the row after the preview rows: %+v", keyboard)
	}
	if want := staffMarker("settings_btn_msg_silent") + " " + icon; toggle.Text != want {
		t.Errorf("the Silent toggle reads %q, want %q", toggle.Text, want)
	}
	if fields[settingsMsgEventField] != code || fields["v"] != target || fields["u"] != "" || fields["c"] == "" {
		t.Errorf("the Silent toggle carries %v, want k=%s, v=%s and the owner field c", fields, code, target)
	}
}

func TestSettingsMessagesSilent(t *testing.T) {
	env := newSettingsMsgEnv(t)
	listEditAdmin(env.settingsEnv, env.chat, env.admin, true)
	m := settingsMsgPrivateMenu(env, env.chat, env.admin)

	text, keyboard := m.press("a", settingsActMsgEvent, settingsMsgEventField, "bn")
	settingsMsgWantSilentScreen(t, text, keyboard, "bn", false)

	// The press stores the target, redraws with Silent On, answers once with no notice and the hub reads Silent.
	toggle, _ := settingsMsgSilentToggle(t, keyboard)
	answers := env.answerCount()
	text, keyboard = m.pressButton(toggle)
	if silent, by, ok := settingsMsgSilentStored(t, env.chat.Id, eventmsg.EventBan); !ok || !silent || by != env.admin.Id {
		t.Fatalf("after the press the ban row is silent=%v by %d (row %v), want silent by %d", silent, by, ok, env.admin.Id)
	}
	settingsMsgWantSilentScreen(t, text, keyboard, "bn", true)
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
	_, keyboard = m.press("a", settingsActMsgEvent, settingsMsgEventField, "bn")
	toggle, _ = settingsMsgSilentToggle(t, keyboard)
	text, keyboard = m.pressButton(toggle)
	if silent, _, ok := settingsMsgSilentStored(t, env.chat.Id, eventmsg.EventBan); !ok || silent {
		t.Fatalf("after the second press the ban row is silent=%v (row %v), want not silent", silent, ok)
	}
	settingsMsgWantSilentScreen(t, text, keyboard, "bn", false)
	hub, _ = m.press("a", settingsActOpen, "p", settingsPageMessages)
	settingsMsgWantStates(t, hub, nil)

	// A stale screen's press of the stored value writes the same value again, with no notice.
	text, keyboard = m.press("a", settingsActMsgSilent, settingsMsgEventField, "bn", "v", "0")
	if silent, _, ok := settingsMsgSilentStored(t, env.chat.Id, eventmsg.EventBan); !ok || silent {
		t.Errorf("a stale press of Off left silent=%v (row %v), want not silent", silent, ok)
	}
	settingsMsgWantSilentScreen(t, text, keyboard, "bn", false)
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
		settingsMsgWantSilentScreen(t, text, keyboard, "bn", false)
		if silent, _, ok := settingsMsgSilentStored(t, env.chat.Id, eventmsg.EventBan); !ok || silent {
			t.Errorf("the failed press left silent=%v (row %v), want the stored row unchanged", silent, ok)
		}
	})
}

// settingsMsgSendPathSilent is what the send path would read for an event of chatID right now: the cached
// read of event_messages:<chat> (eventMessagesLookup), with no cache reset.
func settingsMsgSendPathSilent(t *testing.T, chatID int64, event string) bool {
	t.Helper()
	msgs, err := eventMessagesLookup(context.Background(), chatID)
	if err != nil {
		t.Fatalf("the send path's read of chat %d failed: %v", chatID, err)
	}
	msg, _ := msgs.Lookup(event, eventmsg.DestGroup)
	return msg.Silent
}

func TestSettingsDriftMessagesSilent(t *testing.T) {
	env := newSettingsMsgEnv(t)
	listEditAdmin(env.settingsEnv, env.chat, env.admin, true)
	m := settingsMsgPrivateMenu(env, env.chat, env.admin)

	// Silent stored straight through the repository is drawn On on the hub, the family list and the screen.
	settingsMsgSeed(t, env.chat.Id, eventmsg.EventMute, "", true)
	silentMute := map[string]string{eventmsg.EventMute: "settings_msg_state_silent"}
	text, _ := m.press("a", settingsActOpen, "p", settingsPageMessages)
	settingsMsgWantStates(t, text, silentMute)
	text, _ = m.press("a", settingsActMsgFamily, "t", "m")
	settingsMsgWantStates(t, text, silentMute)
	text, keyboard := m.press("a", settingsActMsgEvent, settingsMsgEventField, "mu")
	settingsMsgWantSilentScreen(t, text, keyboard, "mu", true)

	// A page press is what the send path reads at once, after it had cached the old value.
	if settingsMsgSendPathSilent(t, env.chat.Id, eventmsg.EventBan) {
		t.Fatal("the send path reads the ban message as silent before any press")
	}
	m.press("a", settingsActMsgSilent, settingsMsgEventField, "bn", "v", "1")
	if !settingsMsgSendPathSilent(t, env.chat.Id, eventmsg.EventBan) {
		t.Error("after the page turned Silent on, the send path still reads the ban message as not silent")
	}
	m.press("a", settingsActMsgSilent, settingsMsgEventField, "bn", "v", "0")
	if settingsMsgSendPathSilent(t, env.chat.Id, eventmsg.EventBan) {
		t.Error("after the page turned Silent off, the send path still reads the ban message as silent")
	}

	// Opening every screen of the page writes nothing.
	other := env.addChat()
	o := settingsMsgPrivateMenu(env, other, env.admin)
	o.press("a", settingsActOpen, "p", settingsPageMessages)
	o.press("a", settingsActMsgFamily, "t", "m")
	for _, ev := range settingsMsgManualEvents {
		text, keyboard := o.press("a", settingsActMsgEvent, settingsMsgEventField, ev.Code)
		settingsMsgWantSilentScreen(t, text, keyboard, ev.Code, false)
	}
	if n := settingsMsgRows(t, other.Id); n != 0 {
		t.Errorf("opening every screen wrote %d event message rows, want none", n)
	}
}

func TestSettingsAuthorityMessagesSilent(t *testing.T) {
	// refused presses the Silent toggle of the ban screen as user on a fresh private message and fails the
	// test unless it is answered the alert key with nothing edited and nothing written.
	refused := func(t *testing.T, env *settingsMsgEnv, user gotgbot.User, key string, wantSilent bool) {
		t.Helper()
		msgID := env.updateID()
		env.pressPrivate(user, msgID, env.privateFields(env.chat.Id, "a", settingsActMsgSilent, settingsMsgEventField, "bn", "v", "1"))
		if answer, alert := env.lastAnswer(); !alert || !containsMarker(answer, key) {
			t.Errorf("answer = %q (alert %v), want the alert %s", answer, alert, key)
		}
		if got := len(env.edits(user.Id, msgID)); got != 0 {
			t.Errorf("the refused press made %d edits, want none", got)
		}
		if silent, _, _ := settingsMsgSilentStored(t, env.chat.Id, eventmsg.EventBan); silent != wantSilent {
			t.Errorf("after the refused press the ban message is silent=%v, want %v", silent, wantSilent)
		}
	}
	// writes presses the toggle as user and fails the test unless the ban message is stored silent by user.
	writes := func(t *testing.T, env *settingsMsgEnv, user gotgbot.User) {
		t.Helper()
		settingsMsgPrivateMenu(env, env.chat, user).press("a", settingsActMsgSilent, settingsMsgEventField, "bn", "v", "1")
		if answer, alert := env.lastAnswer(); alert {
			t.Errorf("the press was refused with %q", answer)
		}
		if silent, by, ok := settingsMsgSilentStored(t, env.chat.Id, eventmsg.EventBan); !ok || !silent || by != user.Id {
			t.Errorf("the press left silent=%v by %d (row %v), want silent by %d", silent, by, ok, user.Id)
		}
	}

	t.Run("an administrator with can_change_info writes", func(t *testing.T) {
		env := newSettingsMsgEnv(t)
		listEditAdmin(env.settingsEnv, env.chat, env.admin, true)
		writes(t, env, env.admin)
	})

	t.Run("an administrator without can_change_info is refused", func(t *testing.T) {
		env := newSettingsMsgEnv(t)
		listEditAdmin(env.settingsEnv, env.chat, env.admin, false)
		refused(t, env, env.admin, "settings_no_change_info", false)
		if n := settingsMsgRows(t, env.chat.Id); n != 0 {
			t.Errorf("the refused press created %d rows, want none", n)
		}
	})

	t.Run("an administrator demoted between two presses is refused at the second", func(t *testing.T) {
		env := newSettingsMsgEnv(t)
		listEditAdmin(env.settingsEnv, env.chat, env.admin, true)
		writes(t, env, env.admin)
		listEditAdmin(env.settingsEnv, env.chat, env.admin, false)
		msgID := env.updateID()
		env.pressPrivate(env.admin, msgID, env.privateFields(env.chat.Id, "a", settingsActMsgSilent, settingsMsgEventField, "bn", "v", "0"))
		if answer, alert := env.lastAnswer(); !alert || !containsMarker(answer, "settings_no_change_info") {
			t.Errorf("answer = %q (alert %v), want the alert settings_no_change_info", answer, alert)
		}
		if silent, _, _ := settingsMsgSilentStored(t, env.chat.Id, eventmsg.EventBan); !silent {
			t.Error("the demoted administrator's press turned Silent off")
		}
	})

	t.Run("a plain member is refused", func(t *testing.T) {
		env := newSettingsMsgEnv(t)
		member := gotgbot.User{Id: uniqueLinkOwnerID() + 9_000_000, FirstName: "Member"}
		env.fake.setMember(env.chat.Id, member.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
		refused(t, env, member, "settings_not_admin", false)
	})

	t.Run("an administrator the owner gate refuses writes nothing", func(t *testing.T) {
		resetSettingsOwnerMemo(t)
		env := newSettingsMsgEnv(t)
		env.newOwner(env.chat)
		listEditAdmin(env.settingsEnv, env.chat, env.admin, true)
		env.setAdminsAllowed(env.chat.Id, false)
		refused(t, env, env.admin, "settings_gate_owner_only", false)
	})

	t.Run("the live creator writes", func(t *testing.T) {
		resetSettingsOwnerMemo(t)
		env := newSettingsMsgEnv(t)
		owner := env.newOwner(env.chat)
		env.setAdminsAllowed(env.chat.Id, false)
		writes(t, env, owner)
	})
}
