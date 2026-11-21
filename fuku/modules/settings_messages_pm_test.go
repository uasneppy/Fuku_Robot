//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
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
		settingsMsgWantWidths(t, "the empty private copy screen", keyboard, 1, 1, 1, 1, 1, 2)
		for i, part := range []string{settingsEditPartText, settingsEditPartMedia, settingsEditPartButtons} {
			fields := settingsButtonFields(t, keyboard[i][0])
			if fields["a"] != settingsActMsgSet || fields["t"] != part || fields[settingsMsgEventField] != "bn" ||
				fields[settingsMsgDestField] != settingsMsgDestPMCode || fields["c"] != group || len(fields) != 5 {
				t.Errorf("part row %d carries %v, want a=hs, t=%s, k=bn, d=p and c", i, fields, part)
			}
		}
		toggle, fields := settingsMsgPMSwitch(t, keyboard)
		if keyboard[4][0].CallbackData != toggle.CallbackData || toggle.Text != staffMarker("settings_btn_msg_pm")+" ❌" {
			t.Errorf("the switch %q is not the full-width fifth row reading Off: %+v", toggle.Text, keyboard)
		}
		if want := map[string]string{"a": settingsActMsgPM, "c": group, settingsMsgEventField: "bn", "v": "1"}; !maps.Equal(fields, want) {
			t.Errorf("the switch carries %v, want %v", fields, want)
		}
		if back := settingsButtonFields(t, keyboard[5][0]); !maps.Equal(back, map[string]string{"a": settingsActMsgEvent, "c": group, settingsMsgEventField: "bn"}) {
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

// settingsMsgSetPMWait opens the private copy screen of the event code in a fresh private menu of user and presses
// Set of part; the menu message is then the waiting prompt, returned with the menu.
func settingsMsgSetPMWait(t *testing.T, env *settingsMsgEnv, user gotgbot.User, code, part string) (*settingsMsgMenu, string, [][]gotgbot.InlineKeyboardButton) {
	t.Helper()
	m := settingsMsgPrivateMenu(env, env.chat, user)
	_, keyboard := settingsMsgOpenPM(t, m, code)
	text, prompt := m.pressButton(settingsMsgPartButton(t, keyboard, settingsActMsgSet, part))
	if !env.redis.Exists(settingsInputKey(user.Id)) {
		t.Fatalf("Set of the %s private copy's %s left user %d with no wait", code, part, user.Id)
	}
	return m, text, prompt
}

func TestSettingsMessagesPMTyped(t *testing.T) {
	env := newSettingsMsgEnv(t)
	listEditAdmin(env.settingsEnv, env.chat, env.admin, true)
	user := env.admin
	group := strconv.FormatInt(env.chat.Id, 10)
	settingsMsgSeed(t, env.chat.Id, eventmsg.EventBan, "Group text", false)
	groupRow, _ := settingsMsgStored(t, env.chat.Id, eventmsg.EventBan)

	t.Run("a typed text is stored on the pm row and the copy stays off", func(t *testing.T) {
		m, prompt, keyboard := settingsMsgSetPMWait(t, env, user, "bn", settingsEditPartText)
		if wait := settingsEditWaitOf(env.settingsEnv, user); wait["p"] != "hbnp" || wait["t"] != settingsEditPartText || wait["c"] != group {
			t.Fatalf("wait = %v, want the group, page hbnp and part x", wait)
		}
		if !containsMarker(prompt, "settings_msg_pm_title") || !containsMarker(prompt, "settings_msg_wait_text_manual") ||
			!settingsEditPromptHas(prompt, settingsEditTextMax) {
			t.Errorf("the prompt %q lacks the private copy's title, the family's prompt or the limit %d", prompt, settingsEditTextMax)
		}
		cancel := settingsButtonFields(t, keyboard[0][0])
		if cancel["a"] != settingsActMsgCancel || cancel[settingsMsgEventField] != "bn" || cancel[settingsMsgDestField] != settingsMsgDestPMCode {
			t.Errorf("the prompt's Cancel carries %v, want a=hc, k=bn and d=p", cancel)
		}
		before := settingsEditSentCount(env.settingsEnv, user)
		sendPrivateMessage(env.settingsEnv, user, gotgbot.Message{
			Text: "{first}, {reason}", Entities: []gotgbot.MessageEntity{{Type: "bold", Offset: 0, Length: 7}},
		})
		const want = "<b>{first}</b>, {reason}"
		if row, _ := settingsMsgStoredPM(t, env.chat.Id, eventmsg.EventBan); row.Text != want || !row.Silent || row.UpdatedBy != user.Id {
			t.Errorf("the pm row is %+v, want the text %q, the copy still off, written by %d", row, want, user.Id)
		}
		settingsEditWantReply(t, env.settingsEnv, user, before, "settings_edit_saved_text", "settings_msg_pm_title", "settings_msg_pm_intro")
		page, pageKeyboard := settingsMsgLastSent(t, env, user)
		if line := staffMarker("settings_edit_line_text") + " " + staffMarker("settings_edit_state_text_len"); !strings.Contains(page, line) {
			t.Errorf("the saved page %q does not read the text as Set", page)
		}
		if !strings.Contains(page, settingsMsgPMLine(false)) {
			t.Errorf("the saved page %q does not read the copy Off", page)
		}
		if _, ok := settingsMsgButtonWith(t, pageKeyboard, "a", settingsActMsgRemove, "t", settingsEditPartText, settingsMsgDestField, settingsMsgDestPMCode); !ok {
			t.Errorf("the saved page does not offer Remove for the private copy's text: %+v", pageKeyboard)
		}
		if closed, _ := env.lastEdit(user.Id, m.msgID); !containsMarker(closed, "settings_edit_prompt_closed") {
			t.Errorf("the prompt was edited to %q, want the closed line", closed)
		}
		settingsEditWaitGone(t, env.settingsEnv, user)
		if pm := settingsMsgSendPathPM(t, env.chat.Id, eventmsg.EventBan); pm.Text != want || !pm.Silent {
			t.Errorf("the send path reads the private copy %+v, want the text and the copy off", pm)
		}
	})

	t.Run("a typed photo and typed buttons are stored on the pm row", func(t *testing.T) {
		settingsMsgSetPMWait(t, env, user, "bn", settingsEditPartMedia)
		sendPrivateMessage(env.settingsEnv, user, settingsMediaItemID("photo", "pm-photo"))
		settingsMsgSetPMWait(t, env, user, "bn", settingsEditPartButtons)
		sendPrivateMessage(env.settingsEnv, user, gotgbot.Message{Text: "[Rules](buttonurl://https://example.com/rules)"})
		row, _ := settingsMsgStoredPM(t, env.chat.Id, eventmsg.EventBan)
		if row.FileID != "pm-photo" || len(row.Buttons) != 1 || row.Buttons[0].Url != "https://example.com/rules" || !row.Silent {
			t.Errorf("the pm row is %+v, want the photo, one Rules button and the copy off", row)
		}
	})

	for _, tc := range []struct {
		name, part, key string
		msg             gotgbot.Message
	}{
		{"a javascript link", settingsEditPartButtons, "settings_edit_refuse_button_url", gotgbot.Message{Text: "[a](buttonurl://javascript:alert)"}},
		{"a text over 4096", settingsEditPartText, "content_text_too_long", gotgbot.Message{Text: strings.Repeat("x", 4097)}},
		{"a slash text", settingsEditPartText, "settings_edit_refuse_slash", gotgbot.Message{Text: "/notacommandzz now"}},
	} {
		t.Run("refused: "+tc.name, func(t *testing.T) {
			settingsMsgSetPMWait(t, env, user, "kk", tc.part)
			before := settingsEditSentCount(env.settingsEnv, user)
			sendPrivateMessage(env.settingsEnv, user, tc.msg)
			settingsEditWantReply(t, env.settingsEnv, user, before, tc.key, "settings_edit_still_waiting")
			_, keyboard := settingsMsgLastSent(t, env, user)
			if _, ok := settingsMsgButtonWith(t, keyboard, "a", settingsActMsgCancel, settingsMsgEventField, "kk", settingsMsgDestField, settingsMsgDestPMCode); !ok {
				t.Errorf("the refusal carries no Cancel with k=kk and d=p: %+v", keyboard)
			}
			settingsEditWaitKept(t, env.settingsEnv, user)
			if _, ok := settingsMsgStoredPM(t, env.chat.Id, eventmsg.EventKick); ok {
				t.Error("the refused message wrote the kick's pm row")
			}
		})
	}

	t.Run("Remove clears one part of the pm row and never the group row", func(t *testing.T) {
		m := settingsMsgPrivateMenu(env, env.chat, user)
		for _, part := range []string{settingsEditPartText, settingsEditPartMedia, settingsEditPartButtons} {
			_, keyboard := settingsMsgOpenPM(t, m, "bn")
			text, _ := m.pressButton(settingsMsgPartButton(t, keyboard, settingsActMsgRemove, part))
			if !containsMarker(text, "settings_msg_pm_title") {
				t.Errorf("the redraw after Remove %s %q is not the private copy screen", part, text)
			}
		}
		row, ok := settingsMsgStoredPM(t, env.chat.Id, eventmsg.EventBan)
		if !ok || row.Text != "" || row.FileID != "" || len(row.Buttons) != 0 || !row.Silent {
			t.Errorf("after the three Removes the pm row is %+v (row %v), want an empty row with the copy off", row, ok)
		}
		if after, _ := settingsMsgStored(t, env.chat.Id, eventmsg.EventBan); after.Text != "Group text" || !after.UpdatedAt.Equal(groupRow.UpdatedAt) {
			t.Errorf("the private copy's Removes touched the group row: %+v", after)
		}
	})
}

func TestSettingsDriftMessagesPM(t *testing.T) {
	t.Run("the page draws what the repository stored", func(t *testing.T) {
		env := newSettingsMsgEnv(t)
		listEditAdmin(env.settingsEnv, env.chat, env.admin, true)
		ctx := context.Background()
		if err := eventmsg.SetText(ctx, env.chat.Id, eventmsg.EventKick, eventmsg.DestPM, "You were kicked", 1); err != nil {
			t.Fatalf("seed the kick's private copy: %v", err)
		}
		m := settingsMsgPrivateMenu(env, env.chat, env.admin)
		text, _ := settingsMsgOpenPM(t, m, "kk")
		if line := staffMarker("settings_edit_line_text") + " " + staffMarker("settings_edit_state_text_len"); !strings.Contains(text, line) ||
			!strings.Contains(text, settingsMsgPMLine(false)) {
			t.Errorf("the private copy screen %q does not read a stored text with the copy Off", text)
		}
		if err := eventmsg.SetSilent(ctx, env.chat.Id, eventmsg.EventKick, eventmsg.DestPM, false, 1); err != nil {
			t.Fatalf("switch the kick's private copy on: %v", err)
		}
		if text, _ := settingsMsgOpenPM(t, m, "kk"); !strings.Contains(text, settingsMsgPMLine(true)) {
			t.Errorf("the private copy screen %q does not read the switch the repository stored", text)
		}
		if text, _ := m.press("a", settingsActMsgEvent, settingsMsgEventField, "kk"); !strings.Contains(text, settingsMsgPMLine(true)) {
			t.Errorf("the kick screen %q does not read the private copy On", text)
		}
		// The group row's Silent switch and the private copy's switch are two values: the event screen's two lines
		// read them apart, and a Silent press keeps the private copy's line.
		text, _ = m.press("a", settingsActMsgSilent, settingsMsgEventField, "kk", "v", "1")
		if !strings.Contains(text, settingsMsgPMLine(true)) || !strings.Contains(text, staffMarker("settings_msg_line_silent")+" ✅") {
			t.Errorf("the kick screen after a Silent press %q does not read Silent On and the private copy On", text)
		}
	})

	t.Run("what the page sets is what /ban sends in private", func(t *testing.T) {
		withFastPMCopyPacer(t)
		e := newMessagesE2E(t, false)
		t.Cleanup(pmCopyJobs.wait)
		member := e.member("Ann")

		// The owner sets the ban's private text and switches the copy on, each a press of a drawn button or a typed
		// message in private.
		m := e.openEvent(e.owner, "bn")
		m.tap("a", settingsActMsgEvent, settingsMsgDestField, settingsMsgDestPMCode)
		m.tap("a", settingsActMsgSet, "t", settingsEditPartText, settingsMsgDestField, settingsMsgDestPMCode)
		sendPrivateMessage(e.settingsEnv, e.owner, gotgbot.Message{Text: "You were banned from {chatname}"})
		m = e.openEvent(e.owner, "bn")
		m.tap("a", settingsActMsgEvent, settingsMsgDestField, settingsMsgDestPMCode)
		m.tap("a", settingsActMsgPM, "v", "1")
		if row, _ := settingsMsgStoredPM(t, e.chat.Id, eventmsg.EventBan); row.Silent || row.Text != "You were banned from {chatname}" {
			t.Fatalf("the page left the pm row %+v, want the typed text and the copy on", row)
		}

		e.command(e.settingsEnv, e.admin, "/ban spam", member)
		waitEventPM(t)
		private := e.fake.sentTo(member.Id)
		if len(private) != 1 {
			t.Fatalf("the member got %d private messages, want the one private copy", len(private))
		}
		if text := fmt.Sprint(private[0].Params["text"]); !strings.HasPrefix(text, "You were banned from ") {
			t.Errorf("the private copy reads %q, want the text the page set, filled", text)
		}
		group := e.fake.sentTo(e.chat.Id)
		if len(group) == 0 || group[len(group)-1].MessageID > private[0].MessageID {
			t.Errorf("the private copy was not sent after the group reply: group %+v, private %+v", group, private)
		}
	})
}

func TestSettingsAuthorityMessagesPM(t *testing.T) {
	// refused presses kv (plus the private owner field) as user on a fresh private message and fails the test unless it
	// is answered the alert key with nothing edited, no wait and the ban's pm row unchanged.
	refused := func(t *testing.T, env *settingsMsgEnv, user gotgbot.User, key string, kv ...string) {
		t.Helper()
		before, existed := settingsMsgStoredPM(t, env.chat.Id, eventmsg.EventBan)
		msgID := env.updateID()
		env.pressPrivate(user, msgID, env.privateFields(env.chat.Id, kv...))
		if answer, alert := env.lastAnswer(); !alert || !containsMarker(answer, key) {
			t.Errorf("%v answered %q (alert %v), want the alert %s", kv, answer, alert, key)
		}
		if got := len(env.edits(user.Id, msgID)); got != 0 {
			t.Errorf("the refused %v made %d edits, want none", kv, got)
		}
		if env.redis.Exists(settingsInputKey(user.Id)) {
			t.Errorf("the refused %v left a wait", kv)
		}
		after, exists := settingsMsgStoredPM(t, env.chat.Id, eventmsg.EventBan)
		if exists != existed || after.Silent != before.Silent || after.Text != before.Text || !after.UpdatedAt.Equal(before.UpdatedAt) {
			t.Errorf("the refused %v wrote the pm row: %+v", kv, after)
		}
	}
	set := []string{"a", settingsActMsgSet, settingsMsgEventField, "bn", settingsMsgDestField, settingsMsgDestPMCode, "t", settingsEditPartText}
	remove := []string{"a", settingsActMsgRemove, settingsMsgEventField, "bn", settingsMsgDestField, settingsMsgDestPMCode, "t", settingsEditPartText}
	toggle := []string{"a", settingsActMsgPM, settingsMsgEventField, "bn", "v", "1"}

	t.Run("an administrator without can_change_info views but changes nothing", func(t *testing.T) {
		env := newSettingsMsgEnv(t)
		listEditAdmin(env.settingsEnv, env.chat, env.admin, false)
		m := settingsMsgPrivateMenu(env, env.chat, env.admin)
		if text, _ := settingsMsgOpenPM(t, m, "bn"); !containsMarker(text, "settings_msg_pm_intro") {
			t.Fatalf("the private copy screen did not open for an administrator without can_change_info: %q", text)
		}
		refused(t, env, env.admin, "settings_no_change_info", set...)
		refused(t, env, env.admin, "settings_no_change_info", toggle...)
		if err := eventmsg.SetText(context.Background(), env.chat.Id, eventmsg.EventBan, eventmsg.DestPM, "Seeded", 1); err != nil {
			t.Fatalf("seed the ban's private copy: %v", err)
		}
		refused(t, env, env.admin, "settings_no_change_info", remove...)
	})

	t.Run("a wait whose sender is demoted before the typed message stores nothing", func(t *testing.T) {
		env := newSettingsMsgEnv(t)
		listEditAdmin(env.settingsEnv, env.chat, env.admin, true)
		settingsMsgSetPMWait(t, env, env.admin, "bn", settingsEditPartText)
		listEditAdmin(env.settingsEnv, env.chat, env.admin, false)
		before := settingsEditSentCount(env.settingsEnv, env.admin)
		sendPrivateMessage(env.settingsEnv, env.admin, gotgbot.Message{Text: "Typed after the demotion"})
		settingsEditWantReply(t, env.settingsEnv, env.admin, before, "settings_no_change_info", "settings_edit_wait_stopped")
		settingsEditWaitGone(t, env.settingsEnv, env.admin)
		if row, ok := settingsMsgStoredPM(t, env.chat.Id, eventmsg.EventBan); ok && row.Text != "" {
			t.Errorf("the typed text was stored on the pm row: %+v", row)
		}
	})

	t.Run("an administrator the owner gate refuses changes nothing", func(t *testing.T) {
		resetSettingsOwnerMemo(t)
		env := newSettingsMsgEnv(t)
		env.newOwner(env.chat)
		listEditAdmin(env.settingsEnv, env.chat, env.admin, true)
		env.setAdminsAllowed(env.chat.Id, false)
		refused(t, env, env.admin, "settings_gate_owner_only", toggle...)
		refused(t, env, env.admin, "settings_gate_owner_only", set...)
	})

	t.Run("the live creator switches the copy on", func(t *testing.T) {
		resetSettingsOwnerMemo(t)
		env := newSettingsMsgEnv(t)
		owner := env.newOwner(env.chat)
		env.setAdminsAllowed(env.chat.Id, false)
		settingsMsgPrivateMenu(env, env.chat, owner).press(toggle...)
		if row, ok := settingsMsgStoredPM(t, env.chat.Id, eventmsg.EventBan); !ok || row.Silent || row.UpdatedBy != owner.Id {
			t.Errorf("the creator's press left the pm row %+v (row %v), want the copy on by %d", row, ok, owner.Id)
		}
	})

	t.Run("a group menu's presses are refused in step 1", func(t *testing.T) {
		env := newSettingsMsgEnv(t)
		listEditAdmin(env.settingsEnv, env.chat, env.admin, true)
		msgID := env.openMenu(env.admin)
		for _, kv := range [][]string{
			toggle,
			{"a", settingsActMsgEvent, settingsMsgEventField, "bn", settingsMsgDestField, settingsMsgDestPMCode},
			set,
		} {
			lookups, edits := env.memberLookupsOf(env.chat.Id, env.admin.Id), len(env.edits(env.chat.Id, msgID))
			env.press(env.admin, msgID, env.groupFields(env.admin, kv...))
			if got := settingsEditLastAnswer(env.settingsEnv); !containsMarker(got, "common_callback_invalid_request") {
				t.Errorf("a group menu's %v answered %q, want the invalid request", kv, got)
			}
			if got := env.memberLookupsOf(env.chat.Id, env.admin.Id); got != lookups {
				t.Errorf("a group menu's %v made %d member lookups, want none", kv, got-lookups)
			}
			if got := len(env.edits(env.chat.Id, msgID)); got != edits {
				t.Errorf("a group menu's %v edited the menu", kv)
			}
		}
		if n := settingsMsgRows(t, env.chat.Id); n != 0 {
			t.Errorf("the group menu's presses left %d rows, want none", n)
		}
	})
}
