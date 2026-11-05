//go:build testtools

package modules

import (
	"errors"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/greetings"
)

// settingsOncePage opens the Welcome page of the env's chat, in the group menu or in the owner's
// private chat, and returns the message ID plus a press function that sends the given fields
// of the page in the same place.
func settingsOncePage(env *settingsEnv, owner gotgbot.User, private bool) (int64, func(msgID int64, kv ...string)) {
	env.t.Helper()
	if private {
		msgID := openEditPagePrivate(env, owner, settingsPageWelcome)
		return msgID, func(id int64, kv ...string) {
			env.pressPrivate(owner, id, env.privateFields(env.chat.Id, kv...))
		}
	}
	msgID := openEditPage(env, owner, settingsPageWelcome)
	return msgID, func(id int64, kv ...string) {
		env.press(owner, id, env.groupFields(owner, kv...))
	}
}

// settingsOnceChat is the supergroup the env's chat stands for, as the join handlers see it.
func settingsOnceChat(env *settingsEnv) gotgbot.Chat {
	return gotgbot.Chat{Id: env.chat.Id, Type: "supergroup", Title: env.chat.Title}
}

// settingsOnceStored reads welcome_once of the chat straight from the database.
func settingsOnceStored(t *testing.T, chatID int64) bool {
	t.Helper()
	row, ok := settingsEditStored(t, chatID)
	return ok && row.WelcomeSettings != nil && row.WelcomeSettings.Once
}

func TestSettingsEditOnceSwitch(t *testing.T) {
	for _, private := range []bool{true, false} {
		name := "group menu"
		if private {
			name = "private menu"
		}
		t.Run(name, func(t *testing.T) {
			env := newSettingsEditEnv(t)
			owner := env.newOwner(env.chat)
			chat := settingsOnceChat(env)
			greetOnceCleanup(t, chat.Id)
			// The member is welcomed by this text; the page itself never reads it.
			if err := greetings.SetWelcomeText(chat.Id, "Welcome {first}", "", nil, db.TEXT); err != nil {
				t.Fatalf("SetWelcomeText: %v", err)
			}
			msgID, press := settingsOncePage(env, owner, private)

			text, keyboard := env.lastEdit(chatForPage(env, owner, private), msgID)
			if want := staffMarker("settings_edit_line_once") + " ❌ " + staffMarker("common_status_off"); !strings.Contains(text, want) {
				t.Errorf("the Welcome page %q lacks the off line %q", text, want)
			}
			once, fields := settingsEditControl(t, keyboard, "settings_btn_edit_switch_once", settingsEditControlOnce)
			if !strings.Contains(once.Text, "❌") || fields["a"] != settingsActToggle || fields["p"] != settingsPageWelcome || fields["v"] != "1" {
				t.Fatalf("the Greet once cell is %q with %v, want the off icon and tg on page w with v=1", once.Text, fields)
			}

			// Pressing it stores welcome_once and redraws the line and the cell.
			answers := env.answerCount()
			press(msgID, "a", "tg", "p", "w", "t", "o", "v", "1")
			if got := env.answerCount() - answers; got != 1 {
				t.Errorf("the press was answered %d times, want once", got)
			}
			if !settingsOnceStored(t, chat.Id) {
				t.Fatal("the press did not store welcome_once true")
			}
			text, keyboard = env.lastEdit(chatForPage(env, owner, private), msgID)
			if want := staffMarker("settings_edit_line_once") + " ✅ " + staffMarker("common_status_on"); !strings.Contains(text, want) {
				t.Errorf("after the press the page %q lacks the on line %q", text, want)
			}
			if strings.Contains(text, settingsNoticeWarnIcon) || strings.Contains(text, settingsNoticeOKIcon) {
				t.Errorf("a stored write redrew with a notice: %q", text)
			}
			once, fields = settingsEditControl(t, keyboard, "settings_btn_edit_switch_once", settingsEditControlOnce)
			if !strings.Contains(once.Text, "✅") || fields["v"] != "0" {
				t.Errorf("after the press the cell is %q with %v, want the on icon and v=0", once.Text, fields)
			}

			// A double tap of the same absolute value stores the same row.
			press(msgID, "a", "tg", "p", "w", "t", "o", "v", "1")
			if !settingsOnceStored(t, chat.Id) {
				t.Error("a repeated v=1 press turned Greet once off")
			}

			// The switch is the runtime's: a member who joins, leaves and joins again is welcomed once.
			client := newModuleBotClient()
			bot := newModuleTestBot(client)
			member := gotgbot.User{Id: 5151, FirstName: "Alice"}
			greetOnceJoin(t, bot, chat, member)
			greetOnceLeave(t, bot, chat, member)
			greetOnceJoin(t, bot, chat, member)
			if got := greetOnceWelcomes(client); got != 1 {
				t.Errorf("a member who joined, left and joined again was welcomed %d times, want once", got)
			}

			// /resetwelcome leaves the switch as it is (UI-SPEC assumption A3).
			env.sendIn(env.chat, owner, "/resetwelcome")
			if !settingsOnceStored(t, chat.Id) {
				t.Error("/resetwelcome turned Greet once off")
			}

			// A failed write redraws the freshly loaded page with the old state.
			previous := greetingWrites.WelcomeOnce
			greetingWrites.WelcomeOnce = func(int64, bool) error { return errors.New("write failed") }
			t.Cleanup(func() { greetingWrites.WelcomeOnce = previous })
			press(msgID, "a", "tg", "p", "w", "t", "o", "v", "0")
			text, keyboard = env.lastEdit(chatForPage(env, owner, private), msgID)
			if !strings.Contains(text, settingsNoticeWarnIcon+staffMarker("common_settings_save_failed")) {
				t.Errorf("a failed write drew %q, want the save-failed notice", text)
			}
			once, fields = settingsEditControl(t, keyboard, "settings_btn_edit_switch_once", settingsEditControlOnce)
			if !strings.Contains(once.Text, "✅") || fields["v"] != "0" || !settingsOnceStored(t, chat.Id) {
				t.Errorf("after a failed write the cell is %q with %v, want the old state on", once.Text, fields)
			}
		})
	}
}

// chatForPage is the chat the page message sits in: the group for a group menu, the owner's
// private chat for a private one.
func chatForPage(env *settingsEnv, owner gotgbot.User, private bool) int64 {
	if private {
		return owner.Id
	}
	return env.chat.Id
}
