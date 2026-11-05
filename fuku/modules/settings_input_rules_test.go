//go:build testtools

package modules

import (
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/rules"
)

// settingsRulesTypedOn runs fn with the env's group set to chat, the group the settings helpers address.
func settingsRulesTypedOn(env *settingsEnv, chat gotgbot.Chat, fn func()) {
	previous := env.chat
	env.chat = chat
	defer func() { env.chat = previous }()
	fn()
}

// TestSettingsEditRulesTypedParity: the owner's typed media and buttons for a Rules wait are stored as the
// Welcome stores the same message (the same file ID and type, a GIF as an animation, the same buttons), the
// saved page is the Phase 11 one, and /rules then sends them.
func TestSettingsEditRulesTypedParity(t *testing.T) {
	for _, kind := range []string{"photo", "video", "animation", "document", "sticker", "audio", "voice"} {
		t.Run("media/"+kind, func(t *testing.T) {
			env := newSettingsEditEnv(t)
			pair := newSettingsEditPair(env)
			settingsRulesTypedOn(env, pair.command, func() {
				settingsEditSetWait(env, pair.commandOwner, settingsPageWelcome, settingsEditPartMedia)
				sendPrivateMedia(env, pair.commandOwner, kind)
			})
			_, wantFile, wantType, _ := settingsEditWelcomeOf(t, pair.command.Id, settingsPageWelcome)
			if wantFile == "" {
				t.Fatalf("the Welcome stored no %s", kind)
			}

			settingsRulesTypedOn(env, pair.editor, func() {
				settingsEditSetWait(env, pair.editorOwner, settingsPageRules, settingsEditPartMedia)
				before := settingsEditSentCount(env, pair.editorOwner)
				sendPrivateMedia(env, pair.editorOwner, kind)
				settingsEditWantReply(t, env, pair.editorOwner, before, "settings_edit_saved_media")
			})
			row, ok := settingsEditRulesStored(t, pair.editor.Id)
			if !ok {
				t.Fatal("the Rules media was not stored")
			}
			if row.FileID != wantFile || row.MediaType != wantType {
				t.Errorf("the rules media is file %q type %d, the Welcome stored file %q type %d",
					row.FileID, row.MediaType, wantFile, wantType)
			}
			if kind == "animation" && row.MediaType != db.ANIMATION {
				t.Errorf("a GIF is stored as type %d, want the animation type %d", row.MediaType, db.ANIMATION)
			}
			settingsEditWaitGone(t, env, pair.editorOwner)
		})
	}

	t.Run("buttons", func(t *testing.T) {
		const typed = "[Site](buttonurl://https://example.com)\n[Chat](buttonurl://https://t.me/x:same)"
		env := newSettingsEditEnv(t)
		pair := newSettingsEditPair(env)
		settingsRulesTypedOn(env, pair.command, func() {
			settingsEditSetWait(env, pair.commandOwner, settingsPageWelcome, settingsEditPartButtons)
			sendPrivateMessage(env, pair.commandOwner, gotgbot.Message{Text: typed})
		})
		_, _, _, want := settingsEditWelcomeOf(t, pair.command.Id, settingsPageWelcome)
		if len(want) != 2 {
			t.Fatalf("the Welcome stored %d buttons, want 2", len(want))
		}

		settingsRulesTypedOn(env, pair.editor, func() {
			settingsEditSetWait(env, pair.editorOwner, settingsPageRules, settingsEditPartButtons)
			before := settingsEditSentCount(env, pair.editorOwner)
			sendPrivateMessage(env, pair.editorOwner, gotgbot.Message{Text: typed})
			settingsEditWantReply(t, env, pair.editorOwner, before, "settings_edit_saved_buttons")
		})
		row, _ := settingsEditRulesStored(t, pair.editor.Id)
		if len(row.Buttons) != len(want) {
			t.Fatalf("the rules hold %d buttons, the Welcome stored %d", len(row.Buttons), len(want))
		}
		for i := range want {
			if row.Buttons[i].Name != want[i].Name || row.Buttons[i].Url != want[i].Url || row.Buttons[i].SameLine != want[i].SameLine {
				t.Errorf("button %d is %+v, the Welcome stored %+v", i+1, row.Buttons[i], want[i])
			}
		}
		settingsEditWaitGone(t, env, pair.editorOwner)
	})

	t.Run("/rules sends the typed photo with the text as its caption and the button", func(t *testing.T) {
		env := newSettingsEditEnv(t)
		owner := env.newOwner(env.chat)
		if err := rules.SetChatRules(env.chat.Id, "Be kind"); err != nil {
			t.Fatalf("SetChatRules: %v", err)
		}
		settingsEditSetWait(env, owner, settingsPageRules, settingsEditPartMedia)
		sendPrivateMessage(env, owner, settingsMediaItemID("photo", "typed-rules-photo"))
		settingsEditSetWait(env, owner, settingsPageRules, settingsEditPartButtons)
		sendPrivateMessage(env, owner, gotgbot.Message{Text: "[Site](buttonurl://https://example.com)"})

		client, _ := rulesGroupCommand(t, env.chat.Id, "Test Chat")
		params := rulesOnlyCall(t, client, "sendPhoto")
		if params["photo"] != "typed-rules-photo" {
			t.Errorf("photo = %v, want typed-rules-photo", params["photo"])
		}
		if caption, _ := params["caption"].(string); !strings.Contains(caption, "Test Chat") || !strings.Contains(caption, "Be kind") {
			t.Errorf("caption = %q, want the heading and the rules text", params["caption"])
		}
		rows := rulesKeyboardButtons(params)
		if len(rows) != 1 {
			t.Fatalf("keyboard rows = %v, want the one button", rows)
		}
		if button := rows[0].([]any)[0].(map[string]any); button["text"] != "Site" || button["url"] != "https://example.com" {
			t.Errorf("button = %v, want Site https://example.com", button)
		}
	})
}
