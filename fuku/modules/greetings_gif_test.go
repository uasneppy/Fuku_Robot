//go:build testtools

package modules

import (
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/uasneppy/fuku_robot/fuku/db"
)

// TestSetWelcomeGifIsAnimation drives /setwelcome as a reply to a GIF (Telegram sets both the
// animation and a document on it): the welcome is stored as an animation with the animation's
// file ID, the editor page names it, and a join sends it with sendAnimation and no sendDocument.
func TestSetWelcomeGifIsAnimation(t *testing.T) {
	env := newSettingsEditEnv(t)
	owner := env.newOwner(env.chat)

	gif := gotgbot.Message{
		Animation: &gotgbot.Animation{FileId: "gif-anim", FileUniqueId: "gif-anim-u", Width: 1, Height: 1, Duration: 1},
		Document:  &gotgbot.Document{FileId: "gif-doc", FileUniqueId: "gif-doc-u"},
	}
	const command = "/setwelcome"
	sendGroupMessage(env, env.chat, owner, gotgbot.Message{
		Text:           command,
		Entities:       []gotgbot.MessageEntity{{Type: "bot_command", Offset: 0, Length: int64(len(command))}},
		ReplyToMessage: &gif,
	})

	_, fileID, dataType, _ := settingsEditWelcomeOf(t, env.chat.Id, settingsPageWelcome)
	if dataType != db.ANIMATION || fileID != "gif-anim" {
		t.Fatalf("the welcome is stored as (%q, type %d), want (gif-anim, animation type %d)", fileID, dataType, db.ANIMATION)
	}

	msgID := openEditPage(env, owner, settingsPageWelcome)
	page, _ := env.lastEdit(env.chat.Id, msgID)
	if want := staffMarker("settings_edit_line_media") + " " + staffMarker("settings_edit_media_animation"); !strings.Contains(page, want) {
		t.Errorf("the Welcome page %q lacks %q", page, want)
	}

	client := legacyGreetingClient()
	bot := newModuleTestBot(client)
	member := gotgbot.User{Id: 4243, FirstName: "Newbie"}
	ctx := newChatMemberContext(bot, env.chat, owner,
		gotgbot.ChatMemberLeft{User: member}, gotgbot.ChatMemberMember{User: member})
	clearRecentJoinProcessing(env.chat.Id, member.Id)
	if err := greetingsModule.newMember(bot, ctx); err != ext.EndGroups {
		t.Fatalf("newMember error = %v, want EndGroups", err)
	}
	calls := client.callsFor("sendAnimation")
	if len(calls) != 1 {
		t.Fatalf("sendAnimation calls = %d, want 1", len(calls))
	}
	if got := legacyRequestJSON(t, calls[0]); !strings.Contains(got, `"animation":"gif-anim"`) {
		t.Errorf("sendAnimation request %s lacks the animation file ID", got)
	}
	if docs := client.callsFor("sendDocument"); len(docs) != 0 {
		t.Errorf("sendDocument calls = %d, want 0", len(docs))
	}
}
