//go:build testtools

package modules

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/admin"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/utils/chat_status"
)

// settingsAnonCleanup deletes the admin rows of the given chats before the test and when it ends,
// then resets the local cache layer.
func settingsAnonCleanup(t *testing.T, chatIDs ...int64) {
	t.Helper()
	clean := func() {
		db.DB.Where("chat_id IN ?", chatIDs).Delete(&models.AdminSettings{})
		dbcache.ResetLocalForTest()
	}
	clean()
	t.Cleanup(clean)
}

// settingsAnonUsersCleanup deletes the users rows and cached copies of the given users when the
// test ends (a command or a press records its sender).
func settingsAnonUsersCleanup(t *testing.T, userIDs ...int64) {
	t.Helper()
	t.Cleanup(func() {
		db.DB.Where("user_id IN ?", userIDs).Delete(&models.User{})
		for _, id := range userIDs {
			dbcache.DeleteCache(dbcache.CacheKey("user", id))
		}
		dbcache.ResetLocalForTest()
	})
}

// newSettingsAnonEnv is the settings environment plus /anonadmin, so a parity test can run the
// command next to the page. The env's chat has no admin row.
func newSettingsAnonEnv(t *testing.T) *settingsEnv {
	t.Helper()
	env := newSettingsEnv(t)
	LoadAdmin(env.dispatcher)
	settingsAnonCleanup(t, env.chat.Id)
	settingsAnonUsersCleanup(t, env.admin.Id)
	return env
}

// settingsAnonOwnerOf makes one user the live creator of each of the chats, with the env's
// administrator listed beside them, and returns that user.
func settingsAnonOwnerOf(t *testing.T, env *settingsEnv, chats ...gotgbot.Chat) gotgbot.User {
	t.Helper()
	owner := env.newOwner(chats[0])
	for _, chat := range chats[1:] {
		env.fake.setMember(chat.Id, owner.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusCreator})
		env.fake.setCreator(chat.Id, owner.Id)
		env.fake.listAdministrator(chat.Id, env.admin.Id)
	}
	settingsAnonUsersCleanup(t, owner.Id)
	return owner
}

// settingsAnonPress hand-builds a press of the Anonymous admins page on message msgID of chat in a
// group menu opened by opener, pressed by from.
func settingsAnonPress(t *testing.T, env *settingsEnv, chat gotgbot.Chat, from gotgbot.User, msgID int64, kv ...string) {
	t.Helper()
	fields := env.groupFields(from, kv...)
	data := encodeCallbackData(settingsCallbackNamespace, fields)
	if data == "" {
		t.Fatalf("hand-built press %v did not encode", fields)
	}
	env.pressRaw(chat, from, msgID, data)
}

// settingsAnonToggle is the tg press of the page's toggle with the absolute value v.
func settingsAnonToggle(t *testing.T, env *settingsEnv, chat gotgbot.Chat, from gotgbot.User, msgID int64, v string) {
	t.Helper()
	settingsAnonPress(t, env, chat, from, msgID, "a", "tg", "p", settingsPageAnon, "t", settingsAnonControl, "v", v)
}

// settingsAnonMenuMsgID is the message ID of the hand-built presses that open the page.
const settingsAnonMenuMsgID = 9191

// settingsAnonOpenPage opens the Anonymous admins page of chat in a group menu by a hand-built
// press of from and returns the message ID, the page text and its keyboard.
func settingsAnonOpenPage(t *testing.T, env *settingsEnv, chat gotgbot.Chat, from gotgbot.User) (int64, string, [][]gotgbot.InlineKeyboardButton) {
	t.Helper()
	settingsAnonPress(t, env, chat, from, settingsAnonMenuMsgID, "a", "op", "p", settingsPageAnon)
	text, keyboard := env.lastEdit(chat.Id, settingsAnonMenuMsgID)
	return settingsAnonMenuMsgID, text, keyboard
}

// settingsAnonEnforces reports whether the anonymous-admin check the bot's permission predicates
// make treats an anonymous sender of chat as an administrator without the proof button. With the
// mode off the check sends the proof button and answers false.
func settingsAnonEnforces(env *settingsEnv, chat gotgbot.Chat) bool {
	env.t.Helper()
	from := gotgbot.User{Id: lockdownTestAnonBotID, IsBot: true, FirstName: "Group", Username: "GroupAnonymousBot"}
	senderChat := chat
	update := &gotgbot.Update{
		UpdateId: env.updateID(),
		Message: &gotgbot.Message{
			MessageId: 4242, Date: 1, Chat: chat, From: &from, SenderChat: &senderChat, Text: "/anything",
		},
	}
	return chat_status.CanUserChangeInfo(env.bot, ext.NewContext(env.bot, update, nil), &chat, from.Id)
}

// settingsAnonRowCount is the number of admin rows of the chat.
func settingsAnonRowCount(t *testing.T, chatID int64) int64 {
	t.Helper()
	var n int64
	if err := db.DB.Model(&models.AdminSettings{}).Where("chat_id = ?", chatID).Count(&n).Error; err != nil {
		t.Fatalf("count admin rows of %d: %v", chatID, err)
	}
	return n
}

// settingsAnonToggleOf is the page's toggle button of a keyboard and its fields.
func settingsAnonToggleOf(t *testing.T, keyboard [][]gotgbot.InlineKeyboardButton) (gotgbot.InlineKeyboardButton, map[string]string) {
	t.Helper()
	button, ok := settingsButtonByMarker(keyboard, "settings_btn_anon_toggle")
	if !ok {
		t.Fatalf("the page has no toggle: %+v", keyboard)
	}
	return button, settingsButtonFields(t, button)
}

func TestSettingsAnonAdminOpens(t *testing.T) {
	t.Run("in a group menu, with the mode off and on", func(t *testing.T) {
		env := newSettingsAnonEnv(t)
		for _, on := range []bool{false, true} {
			if err := admin.SetAnonAdminMode(env.chat.Id, on); err != nil {
				t.Fatalf("seed: %v", err)
			}
			_, text, keyboard := settingsAnonOpenPage(t, env, env.chat, env.admin)
			sentence := "admin_anon_admin_disabled"
			wantValue := "1"
			if on {
				sentence, wantValue = "admin_anon_admin_enabled", "0"
			}
			for _, marker := range []string{sentence, "settings_anon_hint", "settings_page_anon"} {
				if !containsMarker(text, marker) {
					t.Errorf("mode %v: page %q lacks %s", on, text, marker)
				}
			}
			if strings.Contains(text, "⚠️") {
				t.Errorf("mode %v: opening the page showed a notice: %q", on, text)
			}
			if got := settingsSwWidths(keyboard); !slices.Equal(got, []int{1, 2}) {
				t.Errorf("mode %v: keyboard widths = %v, want [1 2]", on, got)
			}
			_, fields := settingsAnonToggleOf(t, keyboard)
			if fields["a"] != settingsActToggle || fields["p"] != settingsPageAnon || fields["t"] != settingsAnonControl || fields["v"] != wantValue {
				t.Errorf("mode %v: toggle fields = %v, want tg y m v=%s", on, fields, wantValue)
			}
			label, _ := settingsButtonByMarker(keyboard, "settings_btn_anon_toggle")
			if !strings.Contains(label.Text, settingsStateIcon(on)) {
				t.Errorf("mode %v: toggle label %q lacks the state icon", on, label.Text)
			}
		}
	})

	t.Run("in a private menu, for the live creator", func(t *testing.T) {
		env := newSettingsAnonEnv(t)
		owner := settingsAnonOwnerOf(t, env, env.chat)
		env.sendPrivate(owner, fmt.Sprintf("/start settings_%d", env.chat.Id))
		sentMain := settingsPrivateOnlySent(t, env, owner)
		button, ok := settingsPageButton(t, staffKeyboardOf(sentMain.Params["reply_markup"]), settingsPageAnon)
		if !ok {
			t.Fatal("the private Main has no Anonymous admins button")
		}
		env.pressRaw(settingsPrivateChat(owner), owner, sentMain.MessageID, button.CallbackData)
		text, keyboard := env.lastEdit(owner.Id, sentMain.MessageID)
		if !containsMarker(text, "admin_anon_admin_disabled") || !containsMarker(text, "settings_anon_hint") {
			t.Errorf("private page = %q, want the off sentence and the hint", text)
		}
		if got := settingsSwWidths(keyboard); !slices.Equal(got, []int{1, 2}) {
			t.Errorf("private keyboard widths = %v, want [1 2]", got)
		}
		_, fields := settingsAnonToggleOf(t, keyboard)
		if fields["c"] != fmt.Sprint(env.chat.Id) || fields["u"] != "" {
			t.Errorf("private toggle fields = %v, want c alone", fields)
		}
	})

	t.Run("while the bot is not an administrator", func(t *testing.T) {
		// The command's status view needs the bot as an administrator, the page does not: it only
		// shows what any administrator sees in the group, and writes nothing.
		env := newSettingsAnonEnv(t)
		chat := env.addChat()
		settingsAnonCleanup(t, chat.Id)
		env.fake.setBotRole(chat.Id, staffRoleMember)
		_, text, keyboard := settingsAnonOpenPage(t, env, chat, env.admin)
		if !containsMarker(text, "admin_anon_admin_disabled") || len(keyboard) != 2 {
			t.Errorf("page = %q with %d rows, want the off sentence and the toggle", text, len(keyboard))
		}
		if n := settingsAnonRowCount(t, chat.Id); n != 0 {
			t.Errorf("opening the page created %d admin rows", n)
		}
	})

	t.Run("Main lists it with its state and opening it creates no row", func(t *testing.T) {
		env := newSettingsAnonEnv(t)
		chat := env.addChat()
		settingsAnonCleanup(t, chat.Id)
		owner := settingsAnonOwnerOf(t, env, chat)
		settingsAnonPress(t, env, chat, owner, settingsAnonMenuMsgID, "a", "bk")
		mainText, mainKeyboard := env.lastEdit(chat.Id, settingsAnonMenuMsgID)
		line, found := settingsMainStateLine(mainText, "settings_page_anon")
		if !found || !strings.HasSuffix(line, staffMarker("common_status_off")) {
			t.Errorf("Main status line = %q (found %v), want the Off state", line, found)
		}
		if got := settingsMainShape(t, mainKeyboard); got != settingsMainShapeGroup {
			t.Errorf("Main shape = %q, want %q", got, settingsMainShapeGroup)
		}
		if n := settingsAnonRowCount(t, chat.Id); n != 0 {
			t.Errorf("opening Main created %d admin rows", n)
		}
	})
}

func TestSettingsParityAnonAdmin(t *testing.T) {
	const menuMsgID = 8181

	// run turns the mode on or off on two chats, /anonadmin by the live creator on one and a press
	// of the page by the same creator on the other, and requires the same stored row and the same
	// behaviour of the anonymous-admin check.
	run := func(t *testing.T, seedOn bool, word string) {
		t.Helper()
		env := newSettingsAnonEnv(t)
		commandChat := env.chat
		menuChat := env.addChat()
		settingsAnonCleanup(t, commandChat.Id, menuChat.Id)
		owner := settingsAnonOwnerOf(t, env, commandChat, menuChat)
		want := word == "on"
		if seedOn {
			for _, id := range []int64{commandChat.Id, menuChat.Id} {
				if err := admin.SetAnonAdminMode(id, !want); err != nil {
					t.Fatalf("seed: %v", err)
				}
			}
		}
		value := "0"
		if want {
			value = "1"
		}

		env.sendIn(commandChat, owner, "/anonadmin "+word)
		settingsAnonToggle(t, env, menuChat, owner, menuMsgID, value)

		wantStored := "off"
		if want {
			wantStored = "on"
		}
		for name, id := range map[string]int64{"command": commandChat.Id, "page": menuChat.Id} {
			if got := anonGoldenStored(t, id); got != wantStored {
				t.Errorf("%s stored %s, want %s", name, got, wantStored)
			}
			if got := settingsAnonEnforces(env, gotgbot.Chat{Id: id, Type: "supergroup", Title: "G"}); got != want {
				t.Errorf("the anonymous-admin check on the %s chat treats an anonymous sender as an admin = %v, want %v", name, got, want)
			}
		}
		text, _ := env.lastEdit(menuChat.Id, menuMsgID)
		sentence := "admin_anon_admin_disabled"
		if want {
			sentence = "admin_anon_admin_enabled"
		}
		if !containsMarker(text, sentence) || strings.Contains(text, "⚠️") {
			t.Errorf("the page redrew %q, want the %s sentence and no notice", text, sentence)
		}
	}

	t.Run("on, a chat with no row", func(t *testing.T) { run(t, false, "on") })
	t.Run("on, a chat with the mode off", func(t *testing.T) { run(t, true, "on") })
	t.Run("off, a chat with the mode on", func(t *testing.T) { run(t, true, "off") })

	t.Run("a press of the stored value writes nothing and shows no already text", func(t *testing.T) {
		for _, stored := range []bool{false, true} {
			env := newSettingsAnonEnv(t)
			chat := env.addChat()
			settingsAnonCleanup(t, chat.Id)
			owner := settingsAnonOwnerOf(t, env, chat)
			if err := admin.SetAnonAdminMode(chat.Id, stored); err != nil {
				t.Fatalf("seed: %v", err)
			}
			calls := anonCountingWrite(t)
			value := "0"
			if stored {
				value = "1"
			}
			settingsAnonToggle(t, env, chat, owner, menuMsgID, value)
			if *calls != 0 {
				t.Errorf("stored %v: a press of the stored value wrote %d times, want 0", stored, *calls)
			}
			text, _ := env.lastEdit(chat.Id, menuMsgID)
			if strings.Contains(text, "⚠️") || containsMarker(text, "admin_anon_admin_already_enabled") || containsMarker(text, "admin_anon_admin_already_disabled") {
				t.Errorf("stored %v: the redraw shows a notice: %q", stored, text)
			}
			if got := anonGoldenStored(t, chat.Id); (got == "on") != stored {
				t.Errorf("stored %v: the row reads %s afterwards", stored, got)
			}
		}
	})

	t.Run("a failed save redraws the stored state with the command's failure text", func(t *testing.T) {
		env := newSettingsAnonEnv(t)
		chat := env.addChat()
		settingsAnonCleanup(t, chat.Id)
		owner := settingsAnonOwnerOf(t, env, chat)
		saved := anonAdminWrites
		t.Cleanup(func() { anonAdminWrites = saved })
		anonAdminWrites.SetMode = func(int64, bool) error { return errors.New("database is down") }

		settingsAnonToggle(t, env, chat, owner, menuMsgID, "1")
		text, _ := env.lastEdit(chat.Id, menuMsgID)
		if !containsMarker(text, "admin_anon_admin_db_error") || !containsMarker(text, "admin_anon_admin_disabled") {
			t.Errorf("the page drew %q, want the stored off sentence and the failure notice", text)
		}
		if got := anonGoldenStored(t, chat.Id); got == "on" {
			t.Error("a failed save left the mode on")
		}
	})
}

func TestSettingsAnonAdminRights(t *testing.T) {
	// storedAfter is true when the chat's mode reads on in the database.
	storedAfter := func(t *testing.T, chatID int64) bool {
		t.Helper()
		return anonGoldenStored(t, chatID) == "on"
	}

	t.Run("an administrator who is not the creator reads the page and is refused the change", func(t *testing.T) {
		env := newSettingsAnonEnv(t)
		chat := env.addChat()
		settingsAnonCleanup(t, chat.Id)

		_, text, keyboard := settingsAnonOpenPage(t, env, chat, env.admin)
		if !containsMarker(text, "admin_anon_admin_disabled") || !containsMarker(text, "settings_anon_hint") {
			t.Fatalf("the administrator's page = %q, want the sentence and the hint", text)
		}
		if _, fields := settingsAnonToggleOf(t, keyboard); fields["v"] != "1" {
			t.Errorf("the toggle is drawn for an administrator with v=%q, want 1", fields["v"])
		}

		before := settingsMenuMarkOf(env)
		settingsAnonToggle(t, env, chat, env.admin, settingsAnonMenuMsgID, "1")
		settingsMenuWantRefused(t, env, before, "chat_status_owner_button_error", false)
		if storedAfter(t, chat.Id) {
			t.Error("a non-owner administrator's press wrote the mode")
		}
		if n := settingsAnonRowCount(t, chat.Id); n != 0 {
			t.Errorf("the refused press created %d admin rows", n)
		}
	})

	t.Run("the creator with the bot not an administrator gets the bot notice and nothing is written", func(t *testing.T) {
		env := newSettingsAnonEnv(t)
		chat := env.addChat()
		settingsAnonCleanup(t, chat.Id)
		owner := settingsAnonOwnerOf(t, env, chat)
		env.fake.setBotRole(chat.Id, staffRoleMember)

		answers := env.answerCount()
		settingsAnonToggle(t, env, chat, owner, settingsAnonMenuMsgID, "1")
		if got := env.answerCount() - answers; got != 1 {
			t.Errorf("the press answered %d times, want once", got)
		}
		text, _ := env.lastEdit(chat.Id, settingsAnonMenuMsgID)
		if !containsMarker(text, "chat_status_bot_not_admin") || !containsMarker(text, "admin_anon_admin_disabled") {
			t.Errorf("the press drew %q, want the bot notice over the stored off page", text)
		}
		if storedAfter(t, chat.Id) {
			t.Error("the mode was written while the bot is not an administrator")
		}
	})

	t.Run("with both refusals the bot notice comes first", func(t *testing.T) {
		env := newSettingsAnonEnv(t)
		chat := env.addChat()
		settingsAnonCleanup(t, chat.Id)
		env.fake.setBotRole(chat.Id, staffRoleMember)

		settingsAnonToggle(t, env, chat, env.admin, settingsAnonMenuMsgID, "1")
		text, _ := env.lastEdit(chat.Id, settingsAnonMenuMsgID)
		if !containsMarker(text, "chat_status_bot_not_admin") {
			t.Errorf("the press drew %q, want the bot notice first", text)
		}
		if storedAfter(t, chat.Id) {
			t.Error("the mode was written")
		}
	})

	t.Run("the creator with the bot an administrator writes, on and off", func(t *testing.T) {
		env := newSettingsAnonEnv(t)
		chat := env.addChat()
		settingsAnonCleanup(t, chat.Id)
		owner := settingsAnonOwnerOf(t, env, chat)

		settingsAnonToggle(t, env, chat, owner, settingsAnonMenuMsgID, "1")
		if !storedAfter(t, chat.Id) {
			t.Fatal("the creator's On press did not write")
		}
		settingsAnonToggle(t, env, chat, owner, settingsAnonMenuMsgID, "0")
		if storedAfter(t, chat.Id) {
			t.Fatal("the creator's Off press did not write")
		}
	})

	t.Run("a plain member is refused before any right is judged", func(t *testing.T) {
		env := newSettingsAnonEnv(t)
		chat := env.addChat()
		settingsAnonCleanup(t, chat.Id)
		member := gotgbot.User{Id: uniqueLinkOwnerID() + 47, FirstName: "Plain"}
		settingsAnonUsersCleanup(t, member.Id)
		env.fake.setMember(chat.Id, member.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})

		before := settingsMenuMarkOf(env)
		settingsAnonToggle(t, env, chat, member, settingsAnonMenuMsgID, "1")
		after := settingsMenuMarkOf(env)
		if got := after.answers - before.answers; got != 1 {
			t.Errorf("the press answered %d times, want once", got)
		}
		if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("settings_not_admin")) {
			t.Errorf("answer = %q (alert %v), want the alert settings_not_admin", answer, alert)
		}
		if storedAfter(t, chat.Id) {
			t.Error("a plain member's press wrote the mode")
		}
	})
}

func TestSettingsAnonAdminForged(t *testing.T) {
	type forgedCase struct {
		name   string
		fields []string
		extra  map[string]string
		both   bool
	}
	tg := func(v string) []string {
		return []string{"a", "tg", "p", settingsPageAnon, "t", settingsAnonControl, "v", v}
	}
	forged := []forgedCase{
		{name: "st on m", fields: []string{"a", "st", "p", settingsPageAnon, "t", settingsAnonControl, "v", "1"}},
		{name: "v=2", fields: tg("2")},
		{name: "empty v", fields: tg("")},
		{name: "v=on", fields: tg("on")},
		{name: "an unknown control", fields: []string{"a", "tg", "p", settingsPageAnon, "t", "x", "v", "1"}},
		{name: "an extra key", fields: tg("1"), extra: map[string]string{"x": "1"}},
		{name: "c and u together", fields: tg("1"), both: true},
	}
	for _, tc := range forged {
		t.Run(tc.name, func(t *testing.T) {
			env := newSettingsAnonEnv(t)
			fields := env.groupFields(env.admin, tc.fields...)
			for k, v := range tc.extra {
				fields[k] = v
			}
			if tc.both {
				fields["c"] = "-1001"
			}
			data := encodeCallbackData(settingsCallbackNamespace, fields)
			if data == "" {
				t.Fatalf("forged press %v did not encode", fields)
			}
			before := settingsMenuMarkOf(env)
			env.pressRaw(env.chat, env.admin, settingsAnonMenuMsgID, data)
			settingsMenuWantRefused(t, env, before, "common_callback_invalid_request", true)
			if n := settingsAnonRowCount(t, env.chat.Id); n != 0 {
				t.Errorf("a forged press created %d admin rows", n)
			}
			if got := anonGoldenStored(t, env.chat.Id); got == "on" {
				t.Error("a forged press switched the mode on")
			}
		})
	}
}
