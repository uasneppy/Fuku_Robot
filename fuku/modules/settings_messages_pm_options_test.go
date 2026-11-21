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
	"github.com/uasneppy/fuku_robot/fuku/db/logchannels"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// The private copies block of the Messages hub (plans 22-06 and 22-07, D-24, D-25): the Details and log options'
// toggles ho, their write paths applyEventPMDetails and applyEventPMLog, what the send path reads after a press, and who
// may press them. Every case runs for both option rows from one table.

// settingsMsgOptionCase is one option of the hub's private copies block as the tests see it: its code (the t value of
// ho), its line key, how it reads from the stored row and from the send path's options, the write seam its apply
// function calls and the repository writer that sets it directly.
type settingsMsgOptionCase struct {
	name   string
	code   string
	line   string
	stored func(models.ChatEventMessageOptions) bool
	read   func(eventmsg.Options) bool
	seam   *func(ctx context.Context, chatID int64, on bool, by int64) error
	set    func(ctx context.Context, chatID int64, on bool, by int64) error
}

// settingsMsgOptionCases are the hub's two options, Details then the log line.
var settingsMsgOptionCases = []settingsMsgOptionCase{
	{
		name: "details", code: "d", line: "settings_msg_line_pm_details",
		stored: func(r models.ChatEventMessageOptions) bool { return r.PMDetails },
		read:   func(o eventmsg.Options) bool { return o.PMDetails },
		seam:   &eventMsgWrites.PMDetails, set: eventmsg.SetPMDetails,
	},
	{
		name: "log", code: "l", line: "settings_msg_line_pm_log",
		stored: func(r models.ChatEventMessageOptions) bool { return r.PMLog },
		read:   func(o eventmsg.Options) bool { return o.PMLog },
		seam:   &eventMsgWrites.PMLog, set: eventmsg.SetPMLog,
	},
}

// settingsMsgOptionLine is the hub's line of the option in the marker locale: on reads the check and On, off the cross
// and Off.
func settingsMsgOptionLine(opt settingsMsgOptionCase, on bool) string {
	if on {
		return staffMarker(opt.line) + " ✅ " + staffMarker("common_status_on")
	}
	return staffMarker(opt.line) + " ❌ " + staffMarker("common_status_off")
}

// settingsMsgOptionToggle returns the hub's toggle of the option and its decoded fields, failing the test when the hub
// has none.
func settingsMsgOptionToggle(t *testing.T, keyboard [][]gotgbot.InlineKeyboardButton, opt settingsMsgOptionCase) (gotgbot.InlineKeyboardButton, map[string]string) {
	t.Helper()
	button, ok := settingsMsgButtonWith(t, keyboard, "a", settingsActMsgPMOption, "t", opt.code)
	if !ok {
		t.Fatalf("the hub has no %s toggle: %+v", opt.name, keyboard)
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
	for _, opt := range settingsMsgOptionCases {
		t.Run(opt.name, func(t *testing.T) {
			env := newSettingsMsgEnv(t)
			listEditAdmin(env.settingsEnv, env.chat, env.admin, true)

			// The send path has read the option off and cached that before any press.
			if opt.read(settingsMsgSendPathOptions(t, env.chat.Id)) {
				t.Fatalf("the send path reads the %s option on before any press", opt.name)
			}
			m, text, keyboard := settingsMsgOpenPrivate(env, env.admin)
			for _, key := range []string{"settings_msg_pm_title", "settings_msg_hint_pm_options", "settings_msg_hint_pm_log"} {
				if !containsMarker(text, key) {
					t.Errorf("the hub %q has no %s", text, key)
				}
			}
			for _, each := range settingsMsgOptionCases {
				if !strings.Contains(text, settingsMsgOptionLine(each, false)) {
					t.Errorf("the hub %q does not read the %s option Off", text, each.name)
				}
			}
			settingsMsgWantWidths(t, "the hub", keyboard, 1, 1, 1, 2)
			toggle, fields := settingsMsgOptionToggle(t, keyboard, opt)
			if fields["v"] != "1" || fields["c"] != strconv.FormatInt(env.chat.Id, 10) {
				t.Errorf("the %s toggle carries %v, want v=1 for chat %d", opt.name, fields, env.chat.Id)
			}
			if len(toggle.CallbackData) > 64 {
				t.Errorf("the %s toggle's data %q is %d bytes, over 64", opt.name, toggle.CallbackData, len(toggle.CallbackData))
			}

			// A press writes this option on and no other, and redraws the hub reading On with v=0; the send path's cached
			// read sees it at once.
			answers := env.answerCount()
			text, keyboard = m.pressButton(toggle)
			row, ok := settingsMsgStoredOptions(t, env.chat.Id)
			if !ok || !opt.stored(row) || row.UpdatedBy != env.admin.Id {
				t.Fatalf("after the press the options row is %+v (row %v), want %s on by %d", row, ok, opt.name, env.admin.Id)
			}
			for _, each := range settingsMsgOptionCases {
				if each.code != opt.code && each.stored(row) {
					t.Errorf("the %s toggle also switched the %s option on: %+v", opt.name, each.name, row)
				}
			}
			if n := settingsMsgRows(t, env.chat.Id); n != 0 {
				t.Errorf("the %s toggle wrote %d event message rows, want none", opt.name, n)
			}
			if !strings.Contains(text, settingsMsgOptionLine(opt, true)) || containsMarker(text, "common_settings_save_failed") {
				t.Errorf("the redraw %q is not the hub reading %s On with no notice", text, opt.name)
			}
			settingsMsgWantWidths(t, "the redrawn hub", keyboard, 1, 1, 1, 2)
			if _, fields := settingsMsgOptionToggle(t, keyboard, opt); fields["v"] != "0" {
				t.Errorf("the redrawn toggle carries %v, want v=0", fields)
			}
			if got := env.answerCount() - answers; got != 1 {
				t.Errorf("the press answered %d times, want once", got)
			}
			if answer, alert := env.lastAnswer(); answer != "" || alert {
				t.Errorf("the press answered %q (alert %v), want an empty answer", answer, alert)
			}
			if !opt.read(settingsMsgSendPathOptions(t, env.chat.Id)) {
				t.Errorf("after the hub switched the %s option on, the send path still reads it off", opt.name)
			}

			// A second press writes it off again.
			toggle, _ = settingsMsgOptionToggle(t, keyboard, opt)
			text, _ = m.pressButton(toggle)
			if row, _ := settingsMsgStoredOptions(t, env.chat.Id); opt.stored(row) {
				t.Errorf("the second press left the %s option on", opt.name)
			}
			if !strings.Contains(text, settingsMsgOptionLine(opt, false)) {
				t.Errorf("the redraw after the second press %q does not read Off", text)
			}
			if opt.read(settingsMsgSendPathOptions(t, env.chat.Id)) {
				t.Errorf("after the hub switched the %s option off, the send path still reads it on", opt.name)
			}

			// The toggle is 41 bytes at a 14-character chat ID.
			data := encodeCallbackData(settingsCallbackNamespace, map[string]string{
				"a": settingsActMsgPMOption, "c": "-1001234567890", "t": opt.code, "v": "1",
			})
			if len(data) != 41 {
				t.Errorf("the toggle's data %q is %d bytes, want 41", data, len(data))
			}

			t.Run("a failed write redraws the stored state with the save-failed notice", func(t *testing.T) {
				mreqSwap(t, opt.seam, func(context.Context, int64, bool, int64) error {
					return errors.New("the private copy options are not writable")
				})
				text, keyboard := m.press("a", settingsActMsgPMOption, "t", opt.code, "v", "1")
				if !containsMarker(text, "common_settings_save_failed") || !strings.Contains(text, settingsMsgOptionLine(opt, false)) {
					t.Errorf("the failed press drew %q, want the save-failed notice over the option reading Off", text)
				}
				if _, fields := settingsMsgOptionToggle(t, keyboard, opt); fields["v"] != "1" {
					t.Errorf("the toggle after the failed press carries %v, want v=1", fields)
				}
				if row, _ := settingsMsgStoredOptions(t, env.chat.Id); opt.stored(row) {
					t.Errorf("the failed press turned the %s option on", opt.name)
				}
			})
		})
	}
}

// settingsMsgOptionEffect counts what a blocked copy's job did for the option after the /ban that punished member: the
// Details edits of the group message, or the #PM_COPY posts in the log channel.
func settingsMsgOptionEffect(e *messagesE2E, opt settingsMsgOptionCase, channelID int64) int {
	if opt.code == "d" {
		return len(callsToChat(e.fake.staffBotClient, "editMessageReplyMarkup", e.chat.Id))
	}
	n := 0
	for _, call := range callsToChat(e.fake.staffBotClient, "sendMessage", channelID) {
		if strings.Contains(fmt.Sprint(call.Params["text"]), pmCopyLogTag) {
			n++
		}
	}
	return n
}

func TestSettingsDriftMessagesPMOptions(t *testing.T) {
	for _, opt := range settingsMsgOptionCases {
		t.Run(opt.name, func(t *testing.T) {
			t.Run("the hub draws what the repository stored", func(t *testing.T) {
				env := newSettingsMsgEnv(t)
				listEditAdmin(env.settingsEnv, env.chat, env.admin, true)
				if err := opt.set(context.Background(), env.chat.Id, true, 1); err != nil {
					t.Fatalf("setting the %s option error = %v", opt.name, err)
				}
				_, text, keyboard := settingsMsgOpenPrivate(env, env.admin)
				if !strings.Contains(text, settingsMsgOptionLine(opt, true)) {
					t.Errorf("the hub %q does not read the %s option the repository stored", text, opt.name)
				}
				if _, fields := settingsMsgOptionToggle(t, keyboard, opt); fields["v"] != "0" {
					t.Errorf("the toggle carries %v, want v=0", fields)
				}
			})

			t.Run("what the hub sets is what a blocked copy's job reads at the next /ban", func(t *testing.T) {
				withFastPMCopyPacer(t)
				e := newMessagesE2E(t, false)
				t.Cleanup(pmCopyJobs.wait)
				channelID := uniqueModuleChatID()
				if err := logchannels.Set(e.chat.Id, e.chat.Title, channelID); err != nil {
					t.Fatalf("logchannels.Set error = %v", err)
				}
				t.Cleanup(func() { db.DB.Where("chat_id = ?", e.chat.Id).Delete(&models.LogChannel{}) })
				blocked := &gotgbot.TelegramError{Method: "sendMessage", Code: 403, Description: "Forbidden: bot was blocked by the user"}
				first, second := e.member("Ann"), e.member("Bob")
				e.client.refuse(first.Id, blocked)
				e.client.refuse(second.Id, blocked)
				if err := eventmsg.SetSilent(context.Background(), e.chat.Id, eventmsg.EventBan, eventmsg.DestPM, false, 1); err != nil {
					t.Fatalf("switch the ban's private copy on: %v", err)
				}

				// With the option off, a blocked copy does nothing for it; the send path has cached the off.
				e.command(e.settingsEnv, e.admin, "/ban spam", first)
				waitEventPM(t)
				if n := settingsMsgOptionEffect(e, opt, channelID); n != 0 {
					t.Fatalf("with the %s option off the blocked copy's job did it %d times, want never", opt.name, n)
				}

				// The owner switches the option on with the hub's drawn toggle.
				m := e.openMain(e.owner)
				m.tap("a", settingsActOpen, "p", settingsPageMessages)
				m.tap("a", settingsActMsgPMOption, "t", opt.code, "v", "1")
				if row, ok := settingsMsgStoredOptions(t, e.chat.Id); !ok || !opt.stored(row) {
					t.Fatalf("the hub left the options row %+v (row %v), want %s on", row, ok, opt.name)
				}

				// The next /ban's blocked copy reads it and acts once.
				e.command(e.settingsEnv, e.admin, "/ban spam", second)
				waitEventPM(t)
				if n := settingsMsgOptionEffect(e, opt, channelID); n != 1 {
					t.Fatalf("after the hub switched the %s option on the blocked copy's job did it %d times, want once", opt.name, n)
				}
				if opt.code == "d" {
					edits := callsToChat(e.fake.staffBotClient, "editMessageReplyMarkup", e.chat.Id)
					if markup := fmt.Sprint(edits[0].Params["reply_markup"]); !strings.Contains(markup, "https://t.me/FukuTestBot?start="+pmCopyDetailsPrefix) {
						t.Errorf("the edit's keyboard %s has no Details button", markup)
					}
				}
				if got := len(e.fake.sentTo(second.Id)); got != 0 {
					t.Errorf("the blocked member got %d private messages, want none", got)
				}
			})
		})
	}
}

func TestSettingsAuthorityMessagesPMOptions(t *testing.T) {
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
		if exists != existed || after.PMDetails != before.PMDetails || after.PMLog != before.PMLog || !after.UpdatedAt.Equal(before.UpdatedAt) {
			t.Errorf("the refused %v wrote the options row: %+v", kv, after)
		}
	}

	for _, opt := range settingsMsgOptionCases {
		toggle := []string{"a", settingsActMsgPMOption, "t", opt.code, "v", "1"}
		t.Run(opt.name, func(t *testing.T) {
			t.Run("an administrator without can_change_info views but changes nothing", func(t *testing.T) {
				env := newSettingsMsgEnv(t)
				listEditAdmin(env.settingsEnv, env.chat, env.admin, false)
				_, text, keyboard := settingsMsgOpenPrivate(env, env.admin)
				if !strings.Contains(text, settingsMsgOptionLine(opt, false)) {
					t.Fatalf("the hub did not open for an administrator without can_change_info: %q", text)
				}
				settingsMsgOptionToggle(t, keyboard, opt)
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
					{"a", settingsActMsgPMOption, "t", opt.code, "v", "2"},
					{"a", settingsActMsgPMOption, "t", opt.code},
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
		})
	}
}
