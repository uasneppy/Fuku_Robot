//go:build testtools

package modules

import (
	"fmt"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/utils/content"
)

// sendPrivateMessage runs msg through the real dispatcher as a private message of from to the
// bot (the chat, the sender, the ID and the date are filled in) and returns the message's ID.
func sendPrivateMessage(env *settingsEnv, from gotgbot.User, msg gotgbot.Message) int64 {
	env.t.Helper()
	id := env.updateID()
	msg.MessageId = id
	msg.Date = 1
	msg.Chat = settingsPrivateChat(from)
	msg.From = &from
	update := &gotgbot.Update{UpdateId: id, Message: &msg}
	if err := env.dispatcher.ProcessUpdate(env.bot, update, nil); err != nil {
		env.t.Fatalf("ProcessUpdate(%d) error = %v", id, err)
	}
	return id
}

// settingsEditSetWait opens page pageKey of the env's group in user's private menu and presses
// Set of part, so user has a wait. It returns the ID of the menu message, which is now the
// waiting prompt.
func settingsEditSetWait(env *settingsEnv, user gotgbot.User, pageKey, part string) int64 {
	env.t.Helper()
	def, ok := settingsEditPageByKey(pageKey)
	if !ok {
		env.t.Fatalf("no editor page %q", pageKey)
	}
	p := settingsEditPartByCode(def, part)
	if p == nil {
		env.t.Fatalf("page %q has no part %q", pageKey, part)
	}
	menuID := openEditPagePrivate(env, user, pageKey)
	settingsEditPressButton(env, user, menuID, p.SetLabelKey)
	if !env.redis.Exists(settingsInputKey(user.Id)) {
		env.t.Fatalf("pressing Set of %s/%s left user %d with no wait", pageKey, part, user.Id)
	}
	return menuID
}

func TestSettingsEditTypedTextSaved(t *testing.T) {
	env := newSettingsEditEnv(t)
	owner := env.newOwner(env.chat)
	settingsEditSeedGreetings(t, env.chat.Id)
	before, _ := settingsEditStored(t, env.chat.Id)
	promptID := settingsEditSetWait(env, owner, settingsPageWelcome, settingsEditPartText)
	sentBefore := len(env.fake.sentTo(owner.Id))

	// "Hi {first} 😀 bold" with bold on the last word; the emoji is two UTF-16 units.
	const typed = "Hi {first} 😀 bold"
	entities := []gotgbot.MessageEntity{{Type: "bold", Offset: 14, Length: 4}}
	sendPrivateMessage(env, owner, gotgbot.Message{Text: typed, Entities: entities})

	want := content.ExtractWelcome(&gotgbot.Message{
		Text:     "/setwelcome " + typed,
		Entities: shiftEntities(entities, "/setwelcome "),
	}, "welcome", "en")
	if want.DataType == -1 {
		t.Fatalf("the command refused the text: %q", want.ErrorMsg)
	}
	row, ok := settingsEditStored(t, env.chat.Id)
	if !ok {
		t.Fatal("nothing was stored")
	}
	if row.WelcomeSettings.WelcomeText != want.Text {
		t.Errorf("welcome_text = %q, want what /setwelcome stores: %q", row.WelcomeSettings.WelcomeText, want.Text)
	}
	if row.WelcomeSettings.FileID != before.WelcomeSettings.FileID ||
		row.WelcomeSettings.WelcomeType != before.WelcomeSettings.WelcomeType ||
		len(row.WelcomeSettings.Button) != len(before.WelcomeSettings.Button) {
		t.Errorf("the media or buttons changed: before %+v, after %+v", before.WelcomeSettings, row.WelcomeSettings)
	}
	if row.GoodbyeSettings.GoodbyeText != before.GoodbyeSettings.GoodbyeText {
		t.Errorf("the goodbye changed to %q", row.GoodbyeSettings.GoodbyeText)
	}

	// The saved page is one new message in the owner's private chat, with the page's keyboard.
	sent := env.fake.sentTo(owner.Id)
	if len(sent) != sentBefore+1 {
		t.Fatalf("the bot sent %d new messages to the owner, want exactly 1: %+v", len(sent)-sentBefore, sent[sentBefore:])
	}
	page := sent[len(sent)-1]
	text := fmt.Sprint(page.Params["text"])
	for _, wantText := range []string{
		staffMarker("settings_header") + " " + staffMarker("settings_page_welcome"),
		fmt.Sprint(env.chat.Id),
		staffMarker("settings_edit_saved_text"),
	} {
		if !strings.Contains(text, wantText) {
			t.Errorf("the saved page %q lacks %q", text, wantText)
		}
	}
	if strings.Contains(text, staffMarker("settings_edit_caption_ignored")) {
		t.Errorf("a text reply carries the caption note: %q", text)
	}
	keyboard := staffKeyboardOf(page.Params["reply_markup"])
	if _, ok := settingsButtonByMarker(keyboard, "settings_btn_edit_set_text"); !ok {
		t.Errorf("the saved page has no Set text button: %+v", keyboard)
	}
	if _, ok := settingsButtonByMarker(keyboard, "settings_btn_edit_remove_text"); !ok {
		t.Errorf("the saved page does not offer Remove for the text it just saved: %+v", keyboard)
	}

	// The prompt is closed with no keyboard, and the wait and its marker are gone.
	closed, closedKeyboard := env.lastEdit(owner.Id, promptID)
	if !strings.Contains(closed, staffMarker("settings_edit_prompt_closed")) || len(closedKeyboard) != 0 {
		t.Errorf("the prompt was edited to %q with keyboard %+v, want the closed line and none", closed, closedKeyboard)
	}
	if keys := settingsEditInputKeys(env); len(keys) != 0 {
		t.Errorf("the wait left %v behind", keys)
	}
}
