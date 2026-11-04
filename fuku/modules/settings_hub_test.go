//go:build testtools

package modules

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/adminindex"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// settingsHubSeedGroup records chat in the roster with the given entries and stores its title,
// the way the sweep and the admin cache feed do, and cleans both up with the test.
func settingsHubSeedGroup(t *testing.T, chat gotgbot.Chat, title string, entries ...adminindex.Entry) {
	t.Helper()
	settingsCleanup(t, chat.Id)
	if err := db.DB.Create(&models.Chat{ChatId: chat.Id, ChatName: title}).Error; err != nil {
		t.Fatalf("store chat %d: %v", chat.Id, err)
	}
	if err := adminindex.ReplaceChat(context.Background(), chat.Id, entries); err != nil {
		t.Fatalf("seed roster of %d: %v", chat.Id, err)
	}
}

// settingsHubCreator is the roster entry of a group's creator.
func settingsHubCreator(user gotgbot.User) adminindex.Entry {
	return adminindex.Entry{UserID: user.Id, Role: models.AdminIndexRoleCreator}
}

func TestSettingsHubPick(t *testing.T) {
	env := newSettingsEnv(t)
	owner := env.newOwner(env.chat)
	settingsHubSeedGroup(t, env.chat, "Hub Group", settingsHubCreator(owner))
	lookups := len(env.fake.callsFor("getChatMember"))

	env.sendPrivate(owner, "/settings")

	list := settingsPrivateOnlySent(t, env, owner)
	text := fmt.Sprint(list.Params["text"])
	for _, want := range []string{
		staffMarker("settings_header"),
		staffMarker("settings_page_groups"),
		staffMarker("settings_list_intro"),
		staffMarker("settings_list_hint"),
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the list %q does not contain %q", text, want)
		}
	}
	if got := len(env.fake.callsFor("getChatMember")) - lookups; got != 0 {
		t.Errorf("building the list made %d getChatMember calls, want none", got)
	}
	keyboard := staffKeyboardOf(list.Params["reply_markup"])
	if len(keyboard) != 2 || len(keyboard[0]) != 1 || len(keyboard[1]) != 1 {
		t.Fatalf("the list keyboard = %+v, want one row button and Close", keyboard)
	}
	row, closeButton := keyboard[0][0], keyboard[1][0]
	if row.Text != "Hub Group" {
		t.Errorf("row label = %q, want the group's title", row.Text)
	}
	fields := settingsButtonFields(t, row)
	if fields["a"] != settingsActPick || fields["c"] != fmt.Sprint(env.chat.Id) || len(fields) != 2 {
		t.Errorf("row fields = %v, want a=pk and c=%d only", fields, env.chat.Id)
	}
	closeData, ok := decodeCallbackData(closeButton.CallbackData, settingsHubNamespace)
	if !ok || closeData.Fields["a"] != settingsHubActClose {
		t.Errorf("Close = %q, want shub a=cx", closeButton.CallbackData)
	}

	answers := env.answerCount()
	env.pressRaw(settingsPrivateChat(owner), owner, list.MessageID, row.CallbackData)

	if got := env.answerCount() - answers; got != 1 {
		t.Fatalf("the pick answered %d times, want once", got)
	}
	if got := len(env.edits(owner.Id, list.MessageID)); got != 1 {
		t.Fatalf("the list message was edited %d times, want once", got)
	}
	main, _ := env.lastEdit(owner.Id, list.MessageID)
	if !strings.Contains(main, staffMarker("settings_header_main")) || !strings.Contains(main, fmt.Sprint(env.chat.Id)) {
		t.Errorf("the pick drew %q, want the group's Main page", main)
	}
	if sent := env.fake.sentTo(owner.Id); len(sent) != 1 {
		t.Errorf("the pick sent a new message: %d messages in the private chat, want 1", len(sent))
	}
	if got := len(callsToChat(env.fake.staffBotClient, "deleteMessage", owner.Id)); got != 0 {
		t.Errorf("the pick deleted the list message %d times, want never", got)
	}
}

func TestSettingsHubEmpty(t *testing.T) {
	env := newSettingsEnv(t)
	stranger := gotgbot.User{Id: uniqueLinkOwnerID() + 41, FirstName: "Stranger"}

	env.sendPrivate(stranger, "/settings")

	msg := settingsPrivateOnlySent(t, env, stranger)
	text := fmt.Sprint(msg.Params["text"])
	for _, want := range []string{staffMarker("settings_list_empty"), staffMarker("settings_list_hint")} {
		if !strings.Contains(text, want) {
			t.Errorf("the empty list %q does not contain %q", text, want)
		}
	}
	buttons := settingsAllButtons(staffKeyboardOf(msg.Params["reply_markup"]))
	if len(buttons) != 1 || !containsMarker(buttons[0].Text, "settings_btn_close") {
		t.Errorf("the empty list buttons = %+v, want Close only", buttons)
	}
}
