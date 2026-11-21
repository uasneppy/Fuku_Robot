//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/eventmsg"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// The private copies block of the Messages hub (plan 22-06, D-24, D-25): the Details option's toggle ho, its one write
// path applyEventPMDetails, what the send path reads after a press, and who may press it.

// settingsMsgDetailsLine is the hub's Details line in the marker locale: on reads the check and On, off the cross and
// Off.
func settingsMsgDetailsLine(on bool) string {
	if on {
		return staffMarker("settings_msg_line_pm_details") + " ✅ " + staffMarker("common_status_on")
	}
	return staffMarker("settings_msg_line_pm_details") + " ❌ " + staffMarker("common_status_off")
}

// settingsMsgDetailsToggle returns the hub's Details toggle and its decoded fields, failing the test when the hub has
// none.
func settingsMsgDetailsToggle(t *testing.T, keyboard [][]gotgbot.InlineKeyboardButton) (gotgbot.InlineKeyboardButton, map[string]string) {
	t.Helper()
	button, ok := settingsMsgButtonWith(t, keyboard, "a", settingsActMsgPMOption, "t", "d")
	if !ok {
		t.Fatalf("the hub has no Details toggle: %+v", keyboard)
	}
	return button, settingsButtonFields(t, button)
}

// settingsMsgStoredOptions reads the chat's private copy options row straight from the database, past every cache
// layer. ok is false when the chat has no row.
func settingsMsgStoredOptions(t *testing.T, chatID int64) (models.ChatEventMessageOptions, bool) {
	t.Helper()
	var rows []models.ChatEventMessageOptions
	if err := db.DB.Where("chat_id = ?", chatID).Find(&rows).Error; err != nil {
		t.Fatalf("read the private copy options of chat %d: %v", chatID, err)
	}
	if len(rows) == 0 {
		return models.ChatEventMessageOptions{}, false
	}
	return rows[0], true
}

// settingsMsgSendPathOptions is the chat's private copy options as the send path would read them right now: the cached
// read of event_messages:<chat> (eventMessagesLookup), with no cache reset.
func settingsMsgSendPathOptions(t *testing.T, chatID int64) eventmsg.Options {
	t.Helper()
	msgs, err := eventMessagesLookup(context.Background(), chatID)
	if err != nil {
		t.Fatalf("the send path's read of chat %d failed: %v", chatID, err)
	}
	return msgs.Options
}

func TestSettingsMessagesPMOptions(t *testing.T) {
	env := newSettingsMsgEnv(t)
	listEditAdmin(env.settingsEnv, env.chat, env.admin, true)

	// The send path has read the option off and cached that before any press.
	if settingsMsgSendPathOptions(t, env.chat.Id).PMDetails {
		t.Fatal("the send path reads the Details option on before any press")
	}
	m, text, keyboard := settingsMsgOpenPrivate(env, env.admin)
	for _, key := range []string{"settings_msg_pm_title", "settings_msg_hint_pm_options"} {
		if !containsMarker(text, key) {
			t.Errorf("the hub %q has no %s", text, key)
		}
	}
	if !strings.Contains(text, settingsMsgDetailsLine(false)) {
		t.Errorf("the hub %q does not read the Details option Off", text)
	}
	settingsMsgWantWidths(t, "the hub", keyboard, 1, 1, 2)
	toggle, fields := settingsMsgDetailsToggle(t, keyboard)
	if fields["v"] != "1" || fields["c"] != strconv.FormatInt(env.chat.Id, 10) {
		t.Errorf("the Details toggle carries %v, want v=1 for chat %d", fields, env.chat.Id)
	}
	if len(toggle.CallbackData) > 64 {
		t.Errorf("the Details toggle's data %q is %d bytes, over 64", toggle.CallbackData, len(toggle.CallbackData))
	}

	// A press writes the option on and redraws the hub reading On with v=0; the send path's cached read sees it at once.
	answers := env.answerCount()
	text, keyboard = m.pressButton(toggle)
	if row, ok := settingsMsgStoredOptions(t, env.chat.Id); !ok || !row.PMDetails || row.PMLog || row.UpdatedBy != env.admin.Id {
		t.Fatalf("after the press the options row is %+v (row %v), want Details on by %d", row, ok, env.admin.Id)
	}
	if n := settingsMsgRows(t, env.chat.Id); n != 0 {
		t.Errorf("the Details toggle wrote %d event message rows, want none", n)
	}
	if !strings.Contains(text, settingsMsgDetailsLine(true)) || containsMarker(text, "common_settings_save_failed") {
		t.Errorf("the redraw %q is not the hub reading On with no notice", text)
	}
	settingsMsgWantWidths(t, "the redrawn hub", keyboard, 1, 1, 2)
	if _, fields := settingsMsgDetailsToggle(t, keyboard); fields["v"] != "0" {
		t.Errorf("the redrawn toggle carries %v, want v=0", fields)
	}
	if got := env.answerCount() - answers; got != 1 {
		t.Errorf("the press answered %d times, want once", got)
	}
	if answer, alert := env.lastAnswer(); answer != "" || alert {
		t.Errorf("the press answered %q (alert %v), want an empty answer", answer, alert)
	}
	if !settingsMsgSendPathOptions(t, env.chat.Id).PMDetails {
		t.Error("after the hub switched the Details option on, the send path still reads it off")
	}

	// A second press writes it off again.
	toggle, _ = settingsMsgDetailsToggle(t, keyboard)
	text, _ = m.pressButton(toggle)
	if row, _ := settingsMsgStoredOptions(t, env.chat.Id); row.PMDetails {
		t.Error("the second press left the Details option on")
	}
	if !strings.Contains(text, settingsMsgDetailsLine(false)) {
		t.Errorf("the redraw after the second press %q does not read Off", text)
	}
	if settingsMsgSendPathOptions(t, env.chat.Id).PMDetails {
		t.Error("after the hub switched the Details option off, the send path still reads it on")
	}

	// The toggle is 41 bytes at a 14-character chat ID.
	data := encodeCallbackData(settingsCallbackNamespace, map[string]string{
		"a": settingsActMsgPMOption, "c": "-1001234567890", "t": "d", "v": "1",
	})
	if len(data) != 41 {
		t.Errorf("the toggle's data %q is %d bytes, want 41", data, len(data))
	}

	t.Run("a failed write redraws the stored state with the save-failed notice", func(t *testing.T) {
		mreqSwap(t, &eventMsgWrites.PMDetails, func(context.Context, int64, bool, int64) error {
			return errors.New("the private copy options are not writable")
		})
		text, keyboard := m.press("a", settingsActMsgPMOption, "t", "d", "v", "1")
		if !containsMarker(text, "common_settings_save_failed") || !strings.Contains(text, settingsMsgDetailsLine(false)) {
			t.Errorf("the failed press drew %q, want the save-failed notice over the option reading Off", text)
		}
		if _, fields := settingsMsgDetailsToggle(t, keyboard); fields["v"] != "1" {
			t.Errorf("the toggle after the failed press carries %v, want v=1", fields)
		}
		if row, _ := settingsMsgStoredOptions(t, env.chat.Id); row.PMDetails {
			t.Error("the failed press turned the Details option on")
		}
	})
}

func TestSettingsDriftMessagesPMOptions(t *testing.T) {
	t.Run("the hub draws what the repository stored", func(t *testing.T) {
		env := newSettingsMsgEnv(t)
		listEditAdmin(env.settingsEnv, env.chat, env.admin, true)
		if err := eventmsg.SetPMDetails(context.Background(), env.chat.Id, true, 1); err != nil {
			t.Fatalf("SetPMDetails error = %v", err)
		}
		_, text, keyboard := settingsMsgOpenPrivate(env, env.admin)
		if !strings.Contains(text, settingsMsgDetailsLine(true)) {
			t.Errorf("the hub %q does not read the Details option the repository stored", text)
		}
		if _, fields := settingsMsgDetailsToggle(t, keyboard); fields["v"] != "0" {
			t.Errorf("the toggle carries %v, want v=0", fields)
		}
	})

	t.Run("what the hub sets is what a blocked copy's job reads at the next /ban", func(t *testing.T) {
		withFastPMCopyPacer(t)
		e := newMessagesE2E(t, false)
		t.Cleanup(pmCopyJobs.wait)
		blocked := &gotgbot.TelegramError{Method: "sendMessage", Code: 403, Description: "Forbidden: bot was blocked by the user"}
		first, second := e.member("Ann"), e.member("Bob")
		e.client.refuse(first.Id, blocked)
		e.client.refuse(second.Id, blocked)
		if err := eventmsg.SetSilent(context.Background(), e.chat.Id, eventmsg.EventBan, eventmsg.DestPM, false, 1); err != nil {
			t.Fatalf("switch the ban's private copy on: %v", err)
		}

		// With the option off, a blocked copy leaves the group message as it was; the send path has cached the off.
		e.command(e.settingsEnv, e.admin, "/ban spam", first)
		waitEventPM(t)
		if edits := callsToChat(e.fake.staffBotClient, "editMessageReplyMarkup", e.chat.Id); len(edits) != 0 {
			t.Fatalf("with the option off the group message was edited %d times, want none", len(edits))
		}

		// The owner switches the option on with the hub's drawn toggle.
		m := e.openMain(e.owner)
		m.tap("a", settingsActOpen, "p", settingsPageMessages)
		m.tap("a", settingsActMsgPMOption, "t", "d", "v", "1")
		if row, ok := settingsMsgStoredOptions(t, e.chat.Id); !ok || !row.PMDetails {
			t.Fatalf("the hub left the options row %+v (row %v), want Details on", row, ok)
		}

		// The next /ban's blocked copy reads it and adds the Details button.
		e.command(e.settingsEnv, e.admin, "/ban spam", second)
		waitEventPM(t)
		edits := callsToChat(e.fake.staffBotClient, "editMessageReplyMarkup", e.chat.Id)
		if len(edits) != 1 {
			t.Fatalf("after the hub switched the option on the group message was edited %d times, want once", len(edits))
		}
		if markup := fmt.Sprint(edits[0].Params["reply_markup"]); !strings.Contains(markup, "https://t.me/FukuTestBot?start="+pmCopyDetailsPrefix) {
			t.Errorf("the edit's keyboard %s has no Details button", markup)
		}
		if got := len(e.fake.sentTo(second.Id)); got != 0 {
			t.Errorf("the blocked member got %d private messages, want none", got)
		}
	})
}

func TestSettingsAuthorityMessagesPMOptions(t *testing.T) {
	toggle := []string{"a", settingsActMsgPMOption, "t", "d", "v", "1"}

	// refused presses kv (plus the private owner field) as user on a fresh private message and fails the test unless it
	// is answered the alert key with nothing edited and no options row written.
	refused := func(t *testing.T, env *settingsMsgEnv, user gotgbot.User, key string, kv ...string) {
		t.Helper()
		before, existed := settingsMsgStoredOptions(t, env.chat.Id)
		msgID := env.updateID()
		env.pressPrivate(user, msgID, env.privateFields(env.chat.Id, kv...))
		if answer, alert := env.lastAnswer(); !alert || !containsMarker(answer, key) {
			t.Errorf("%v answered %q (alert %v), want the alert %s", kv, answer, alert, key)
		}
		if got := len(env.edits(user.Id, msgID)); got != 0 {
			t.Errorf("the refused %v made %d edits, want none", kv, got)
		}
		after, exists := settingsMsgStoredOptions(t, env.chat.Id)
		if exists != existed || after.PMDetails != before.PMDetails || !after.UpdatedAt.Equal(before.UpdatedAt) {
			t.Errorf("the refused %v wrote the options row: %+v", kv, after)
		}
	}

	t.Run("an administrator without can_change_info views but changes nothing", func(t *testing.T) {
		env := newSettingsMsgEnv(t)
		listEditAdmin(env.settingsEnv, env.chat, env.admin, false)
		_, text, keyboard := settingsMsgOpenPrivate(env, env.admin)
		if !strings.Contains(text, settingsMsgDetailsLine(false)) {
			t.Fatalf("the hub did not open for an administrator without can_change_info: %q", text)
		}
		settingsMsgDetailsToggle(t, keyboard)
		refused(t, env, env.admin, "settings_no_change_info", toggle...)
	})

	t.Run("an administrator the owner gate refuses changes nothing", func(t *testing.T) {
		resetSettingsOwnerMemo(t)
		env := newSettingsMsgEnv(t)
		env.newOwner(env.chat)
		listEditAdmin(env.settingsEnv, env.chat, env.admin, true)
		env.setAdminsAllowed(env.chat.Id, false)
		refused(t, env, env.admin, "settings_gate_owner_only", toggle...)
	})

	t.Run("forged fields and a group menu's press are refused in step 1", func(t *testing.T) {
		env := newSettingsMsgEnv(t)
		listEditAdmin(env.settingsEnv, env.chat, env.admin, true)
		for _, kv := range [][]string{
			{"a", settingsActMsgPMOption, "t", "x", "v", "1"},
			{"a", settingsActMsgPMOption, "t", "d", "v", "2"},
			{"a", settingsActMsgPMOption, "t", "d"},
		} {
			lookups := env.memberLookupsOf(env.chat.Id, env.admin.Id)
			msgID := env.updateID()
			env.pressPrivate(env.admin, msgID, env.privateFields(env.chat.Id, kv...))
			if got := settingsEditLastAnswer(env.settingsEnv); !containsMarker(got, "common_callback_invalid_request") {
				t.Errorf("the private %v answered %q, want the invalid request", kv, got)
			}
			if got := env.memberLookupsOf(env.chat.Id, env.admin.Id); got != lookups {
				t.Errorf("the private %v made %d member lookups, want none", kv, got-lookups)
			}
		}
		msgID := env.openMenu(env.admin)
		lookups, edits := env.memberLookupsOf(env.chat.Id, env.admin.Id), len(env.edits(env.chat.Id, msgID))
		env.press(env.admin, msgID, env.groupFields(env.admin, toggle...))
		if got := settingsEditLastAnswer(env.settingsEnv); !containsMarker(got, "common_callback_invalid_request") {
			t.Errorf("a group menu's ho answered %q, want the invalid request", got)
		}
		if got := env.memberLookupsOf(env.chat.Id, env.admin.Id); got != lookups {
			t.Errorf("a group menu's ho made %d member lookups, want none", got-lookups)
		}
		if got := len(env.edits(env.chat.Id, msgID)); got != edits {
			t.Error("a group menu's ho edited the menu")
		}
		if _, ok := settingsMsgStoredOptions(t, env.chat.Id); ok {
			t.Error("the refused presses wrote an options row")
		}
	})
}
