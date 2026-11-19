//go:build testtools

package modules

import (
	"context"
	"errors"
	"maps"
	"strconv"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db/eventmsg"
)

// settingsMsgPMLine is the private copy line of a screen in the marker locale: on reads the check and On, off the cross
// and Off.
func settingsMsgPMLine(on bool) string {
	if on {
		return staffMarker("settings_msg_line_pm") + " ✅ " + staffMarker("common_status_on")
	}
	return staffMarker("settings_msg_line_pm") + " ❌ " + staffMarker("common_status_off")
}

// settingsMsgSendPathPM is the private copy of an event of chatID the send path would read right now: the cached read
// of event_messages:<chat> (eventMessagesLookup) resolved for the pm destination, with no cache reset.
func settingsMsgSendPathPM(t *testing.T, chatID int64, event string) eventmsg.Message {
	t.Helper()
	msgs, err := eventMessagesLookup(context.Background(), chatID)
	if err != nil {
		t.Fatalf("the send path's read of chat %d failed: %v", chatID, err)
	}
	return msgs.Resolve(event, eventmsg.DestPM)
}

// settingsMsgPMSwitch returns the switch of a private copy screen and its decoded fields, failing the test when the
// screen has none.
func settingsMsgPMSwitch(t *testing.T, keyboard [][]gotgbot.InlineKeyboardButton) (gotgbot.InlineKeyboardButton, map[string]string) {
	t.Helper()
	button, ok := settingsMsgButtonWith(t, keyboard, "a", settingsActMsgPM)
	if !ok {
		t.Fatalf("the private copy screen has no switch: %+v", keyboard)
	}
	return button, settingsButtonFields(t, button)
}

// withoutEventPMSpec removes the private copy spec of event for the test, as for an event a later phase registers
// without one; the table is put back when the test ends.
func withoutEventPMSpec(t *testing.T, event string) {
	t.Helper()
	saved := eventPMSpecs
	eventPMSpecs = maps.Clone(saved)
	delete(eventPMSpecs, event)
	t.Cleanup(func() { eventPMSpecs = saved })
}

func TestSettingsMessagesPMScreen(t *testing.T) {
	env := newSettingsMsgEnv(t)
	listEditAdmin(env.settingsEnv, env.chat, env.admin, true)
	group := strconv.FormatInt(env.chat.Id, 10)
	m := settingsMsgPrivateMenu(env, env.chat, env.admin)

	t.Run("event screen row", func(t *testing.T) {
		text, keyboard := m.press("a", settingsActMsgEvent, settingsMsgEventField, "bn")
		if !strings.Contains(text, settingsMsgPMLine(false)) {
			t.Errorf("the ban screen %q lacks the private copy line reading Off", text)
		}
		if silent, pm := strings.Index(text, staffMarker("settings_msg_line_silent")), strings.Index(text, staffMarker("settings_msg_line_pm")); silent < 0 || pm < silent {
			t.Errorf("the private copy line is not after the Silent line in %q", text)
		}
		open, ok := settingsMsgButtonWith(t, keyboard, "a", settingsActMsgEvent, settingsMsgDestField, settingsMsgDestPMCode)
		if !ok {
			t.Fatalf("the ban screen has no private copy row: %+v", keyboard)
		}
		want := encodeCallbackData(settingsCallbackNamespace, map[string]string{
			"a": settingsActMsgEvent, "c": group, settingsMsgDestField: settingsMsgDestPMCode, settingsMsgEventField: "bn",
		})
		if open.CallbackData != want || !containsMarker(open.Text, "settings_btn_msg_pm_open") {
			t.Errorf("the private copy row is %q with %q, want the open label with %q", open.Text, open.CallbackData, want)
		}
		if row := keyboard[len(keyboard)-2]; len(row) != 1 || row[0].CallbackData != want {
			t.Errorf("the private copy row is not the full-width row above Back and Close: %+v", keyboard)
		}

		// Every part set and Silent on: the worst case, eight rows.
		settingsMsgSeedFull(t, env.chat.Id, eventmsg.EventMute)
		settingsMsgSeed(t, env.chat.Id, eventmsg.EventMute, "", true)
		_, keyboard = m.press("a", settingsActMsgEvent, settingsMsgEventField, "mu")
		settingsMsgWantWidths(t, "the full mute screen", keyboard, 2, 2, 2, 2, 2, 1, 1, 2)
	})

	t.Run("pm screen", func(t *testing.T) {
		text, keyboard := settingsMsgOpenPM(t, m, "bn")
		for _, key := range []string{"settings_msg_pm_title", "settings_msg_ev_ban", "settings_msg_pm_intro", "settings_msg_hint_manual", "settings_msg_hint_pm"} {
			if !containsMarker(text, key) {
				t.Errorf("the private copy screen %q lacks %s", text, key)
			}
		}
		if want := staffMarker("settings_edit_line_text") + " " + staffMarker("settings_state_default"); !strings.Contains(text, want) {
			t.Errorf("the private copy screen %q does not read its text as Default", text)
		}
		if n := strings.Count(text, staffMarker("settings_state_not_set")); n != 2 {
			t.Errorf("the private copy screen reads Not set %d times, want 2 (media and buttons):\n%s", n, text)
		}
		if !strings.Contains(text, settingsMsgPMLine(false)) {
			t.Errorf("the private copy screen %q lacks its line reading Off", text)
		}
		settingsMsgWantWidths(t, "the empty private copy screen", keyboard, 1, 1, 1, 1, 2)
		for i, part := range []string{settingsEditPartText, settingsEditPartMedia, settingsEditPartButtons} {
			fields := settingsButtonFields(t, keyboard[i][0])
			if fields["a"] != settingsActMsgSet || fields["t"] != part || fields[settingsMsgEventField] != "bn" ||
				fields[settingsMsgDestField] != settingsMsgDestPMCode || fields["c"] != group || len(fields) != 5 {
				t.Errorf("part row %d carries %v, want a=hs, t=%s, k=bn, d=p and c", i, fields, part)
			}
		}
		toggle, fields := settingsMsgPMSwitch(t, keyboard)
		if keyboard[3][0].CallbackData != toggle.CallbackData || toggle.Text != staffMarker("settings_btn_msg_pm")+" ❌" {
			t.Errorf("the switch %q is not the full-width fourth row reading Off: %+v", toggle.Text, keyboard)
		}
		if want := map[string]string{"a": settingsActMsgPM, "c": group, settingsMsgEventField: "bn", "v": "1"}; !maps.Equal(fields, want) {
			t.Errorf("the switch carries %v, want %v", fields, want)
		}
		if back := settingsButtonFields(t, keyboard[4][0]); !maps.Equal(back, map[string]string{"a": settingsActMsgEvent, "c": group, settingsMsgEventField: "bn"}) {
			t.Errorf("Back carries %v, want the ban screen with no d", back)
		}
		// A screen with nothing stored writes nothing.
		if _, ok := settingsMsgStoredPM(t, env.chat.Id, eventmsg.EventBan); ok {
			t.Error("drawing the private copy screen created a pm row")
		}
	})

	t.Run("group buttons unchanged", func(t *testing.T) {
		screens := map[string][][]gotgbot.InlineKeyboardButton{}
		_, screens["hub"] = m.press("a", settingsActOpen, "p", settingsPageMessages)
		_, screens["family"] = m.press("a", settingsActMsgFamily, "t", "m")
		for _, ev := range settingsMsgManualEvents {
			_, screens[ev.Code] = m.press("a", settingsActMsgEvent, settingsMsgEventField, ev.Code)
		}
		for name, keyboard := range screens {
			for _, button := range settingsAllButtons(keyboard) {
				fields := settingsButtonFields(t, button)
				d, has := fields[settingsMsgDestField]
				if has && (fields["a"] != settingsActMsgEvent || d != settingsMsgDestPMCode) {
					t.Errorf("%s: button %q carries d=%q outside the private copy row", name, button.Text, d)
				}
			}
		}
	})

	t.Run("unknown d", func(t *testing.T) {
		withoutEventPMSpec(t, eventmsg.EventUnban)
		forged := [][]string{
			{"a", settingsActMsgEvent, settingsMsgEventField, "bn", settingsMsgDestField, "g"},
			{"a", settingsActMsgEvent, settingsMsgEventField, "bn", settingsMsgDestField, "x"},
			{"a", settingsActMsgEvent, settingsMsgEventField, "bn", settingsMsgDestField, "pp"},
			{"a", settingsActMsgEvent, settingsMsgEventField, "bn", settingsMsgDestField, ""},
			{"a", settingsActMsgSet, settingsMsgEventField, "bn", settingsMsgDestField, "x", "t", settingsEditPartText},
			{"a", settingsActMsgPM, settingsMsgEventField, "bn", settingsMsgDestField, settingsMsgDestPMCode, "v", "1"},
			{"a", settingsActMsgPM, settingsMsgEventField, "bn", "v", "2"},
			{"a", settingsActMsgSilent, settingsMsgEventField, "bn", settingsMsgDestField, settingsMsgDestPMCode, "v", "1"},
			// An event with no private copy spec has no private copy to open, set or switch.
			{"a", settingsActMsgEvent, settingsMsgEventField, "ub", settingsMsgDestField, settingsMsgDestPMCode},
			{"a", settingsActMsgSet, settingsMsgEventField, "ub", settingsMsgDestField, settingsMsgDestPMCode, "t", settingsEditPartText},
			{"a", settingsActMsgPM, settingsMsgEventField, "ub", "v", "1"},
		}
		rows := settingsMsgRows(t, env.chat.Id)
		for _, kv := range forged {
			lookups := env.memberLookupsOf(env.chat.Id, env.admin.Id)
			env.pressPrivate(env.admin, m.msgID, env.privateFields(env.chat.Id, kv...))
			if got := settingsEditLastAnswer(env.settingsEnv); !containsMarker(got, "common_callback_invalid_request") {
				t.Errorf("%v answered %q, want the invalid request", kv, got)
			}
			if got := env.memberLookupsOf(env.chat.Id, env.admin.Id); got != lookups {
				t.Errorf("%v made %d member lookups, want none", kv, got-lookups)
			}
		}
		if n := settingsMsgRows(t, env.chat.Id); n != rows {
			t.Errorf("the forged presses changed the event message rows from %d to %d", rows, n)
		}

		// The unban screen of an event without a spec has neither the line nor the row.
		text, keyboard := m.press("a", settingsActMsgEvent, settingsMsgEventField, "ub")
		if containsMarker(text, "settings_msg_line_pm") {
			t.Errorf("the unban screen without a spec %q has the private copy line", text)
		}
		if _, ok := settingsMsgButtonWith(t, keyboard, settingsMsgDestField, settingsMsgDestPMCode); ok {
			t.Errorf("the unban screen without a spec has a private copy row: %+v", keyboard)
		}
	})
}

func TestSettingsMessagesPMSwitch(t *testing.T) {
	env := newSettingsMsgEnv(t)
	listEditAdmin(env.settingsEnv, env.chat, env.admin, true)
	m := settingsMsgPrivateMenu(env, env.chat, env.admin)

	// The send path has read the copy off and cached that before any press.
	if pm := settingsMsgSendPathPM(t, env.chat.Id, eventmsg.EventBan); !pm.Silent {
		t.Fatal("the send path reads the ban's private copy on before any press")
	}
	_, keyboard := settingsMsgOpenPM(t, m, "bn")
	toggle, _ := settingsMsgPMSwitch(t, keyboard)
	answers := env.answerCount()
	text, keyboard := m.pressButton(toggle)
	if row, ok := settingsMsgStoredPM(t, env.chat.Id, eventmsg.EventBan); !ok || row.Silent || row.UpdatedBy != env.admin.Id {
		t.Fatalf("after the press the pm row is %+v (row %v), want silent false by %d", row, ok, env.admin.Id)
	}
	if _, ok := settingsMsgStored(t, env.chat.Id, eventmsg.EventBan); ok {
		t.Error("the private copy switch wrote the group row")
	}
	if !strings.Contains(text, settingsMsgPMLine(true)) || !containsMarker(text, "settings_msg_pm_intro") {
		t.Errorf("the redraw %q is not the private copy screen reading On", text)
	}
	if _, fields := settingsMsgPMSwitch(t, keyboard); fields["v"] != "0" {
		t.Errorf("the redrawn switch carries %v, want v=0", fields)
	}
	if got := env.answerCount() - answers; got != 1 {
		t.Errorf("the press answered %d times, want once", got)
	}
	if answer, alert := env.lastAnswer(); answer != "" || alert {
		t.Errorf("the press answered %q (alert %v), want an empty answer", answer, alert)
	}
	if pm := settingsMsgSendPathPM(t, env.chat.Id, eventmsg.EventBan); pm.Silent {
		t.Error("after the page switched the copy on, the send path still reads it off")
	}

	// Back is the ban screen, whose line now reads On.
	back, ok := settingsMsgButtonWith(t, keyboard, "a", settingsActMsgEvent, settingsMsgEventField, "bn")
	if !ok {
		t.Fatalf("the private copy screen has no Back to the event: %+v", keyboard)
	}
	text, _ = m.pressButton(back)
	if !strings.Contains(text, settingsMsgPMLine(true)) || !containsMarker(text, "settings_msg_desc_ban") {
		t.Errorf("the ban screen after Back %q does not read the private copy On", text)
	}

	// A second press writes silent true again.
	_, keyboard = settingsMsgOpenPM(t, m, "bn")
	toggle, _ = settingsMsgPMSwitch(t, keyboard)
	text, _ = m.pressButton(toggle)
	if row, _ := settingsMsgStoredPM(t, env.chat.Id, eventmsg.EventBan); !row.Silent {
		t.Error("the second press left the private copy on")
	}
	if !strings.Contains(text, settingsMsgPMLine(false)) {
		t.Errorf("the redraw after the second press %q does not read Off", text)
	}
	if pm := settingsMsgSendPathPM(t, env.chat.Id, eventmsg.EventBan); !pm.Silent {
		t.Error("after the page switched the copy off, the send path still reads it on")
	}

	// A stale screen's press of the stored value writes the same value again, with no notice.
	text, _ = m.press("a", settingsActMsgPM, settingsMsgEventField, "bn", "v", "0")
	if row, _ := settingsMsgStoredPM(t, env.chat.Id, eventmsg.EventBan); !row.Silent {
		t.Error("a stale press of Off turned the copy on")
	}
	if containsMarker(text, "common_settings_save_failed") || !strings.Contains(text, settingsMsgPMLine(false)) {
		t.Errorf("the stale press drew %q, want the screen reading Off with no notice", text)
	}

	// The switch is 42 bytes at a 14-character chat ID.
	data := encodeCallbackData(settingsCallbackNamespace, map[string]string{
		"a": settingsActMsgPM, "c": "-1001234567890", settingsMsgEventField: "bn", "v": "1",
	})
	if len(data) != 42 {
		t.Errorf("the switch's data %q is %d bytes, want 42", data, len(data))
	}

	t.Run("a failed write redraws the stored state with the save-failed notice", func(t *testing.T) {
		mreqSwap(t, &eventMsgWrites.Silent, func(context.Context, int64, string, string, bool, int64) error {
			return errors.New("the event messages are not writable")
		})
		text, keyboard := m.press("a", settingsActMsgPM, settingsMsgEventField, "bn", "v", "1")
		if !containsMarker(text, "common_settings_save_failed") || !strings.Contains(text, settingsMsgPMLine(false)) {
			t.Errorf("the failed press drew %q, want the save-failed notice over the copy reading Off", text)
		}
		if _, fields := settingsMsgPMSwitch(t, keyboard); fields["v"] != "1" {
			t.Errorf("the switch after the failed press carries %v, want v=1", fields)
		}
		if row, _ := settingsMsgStoredPM(t, env.chat.Id, eventmsg.EventBan); !row.Silent {
			t.Error("the failed press turned the copy on")
		}
	})
}
