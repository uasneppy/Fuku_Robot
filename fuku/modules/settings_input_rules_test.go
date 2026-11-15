//go:build testtools

package modules

import (
	"strings"
	"sync/atomic"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/db/rules"
)

// settingsRulesCleanUsers removes, when the test ends, the users rows and cached copies that the owners'
// private messages left behind, so a later test of the package finds those IDs unknown.
func settingsRulesCleanUsers(t *testing.T, ids ...int64) {
	t.Helper()
	t.Cleanup(func() {
		DrainUsersAsyncWrites()
		db.DB.Where("user_id IN ?", ids).Delete(&models.User{})
		for _, id := range ids {
			dbcache.DeleteCache(dbcache.CacheKey("user", id))
		}
		dbcache.ResetLocalForTest()
	})
}

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
			settingsRulesCleanUsers(t, pair.commandOwner.Id, pair.editorOwner.Id)
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
		settingsRulesCleanUsers(t, pair.commandOwner.Id, pair.editorOwner.Id)
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
		buttonOwner := env.newOwner(env.chat)
		settingsRulesCleanUsers(t, owner.Id, buttonOwner.Id)
		if err := rules.SetChatRules(env.chat.Id, "Be kind"); err != nil {
			t.Fatalf("SetChatRules: %v", err)
		}
		settingsEditSetWait(env, owner, settingsPageRules, settingsEditPartMedia)
		sendPrivateMessage(env, owner, settingsMediaItemID("photo", "typed-rules-photo"))
		// The private menu is opened by the deep link once per person, so the buttons come from a second owner.
		settingsEditSetWait(env, buttonOwner, settingsPageRules, settingsEditPartButtons)
		sendPrivateMessage(env, buttonOwner, gotgbot.Message{Text: "[Site](buttonurl://https://example.com)"})

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

// settingsRulesWantRefusal fails the test unless the bot sent exactly one reply since before, carrying the reason,
// the still-waiting line and one Cancel button, and the wait is still there.
func settingsRulesWantRefusal(t *testing.T, env *settingsEnv, owner gotgbot.User, before int, reason string) {
	t.Helper()
	settingsEditWantReply(t, env, owner, before, reason, "settings_edit_still_waiting")
	sent := env.fake.sentTo(owner.Id)
	buttons := settingsAllButtons(staffKeyboardOf(sent[len(sent)-1].Params["reply_markup"]))
	if len(buttons) != 1 || !strings.Contains(buttons[0].Text, staffMarker("settings_btn_edit_cancel")) {
		t.Errorf("the refusal carries the buttons %+v, want the one Cancel button", buttons)
	}
	settingsEditWaitKept(t, env, owner)
}

// settingsRulesStoredNothing fails the test when the chat's rules row holds media or buttons.
func settingsRulesStoredNothing(t *testing.T, chatID int64) {
	t.Helper()
	if row, ok := settingsEditRulesStored(t, chatID); ok && (row.FileID != "" || row.MediaType != 0 || len(row.Buttons) > 0) {
		t.Errorf("a refused message stored media or buttons: %+v", row)
	}
}

// TestSettingsEditRulesTypedRefusals: a Rules wait refuses what the Welcome's refuses and the two caption
// refusals, keeps the wait with the Cancel button, and saves nothing; an album is one item, a caption is
// ignored, and a bot that is no administrator any more ends the wait.
func TestSettingsEditRulesTypedRefusals(t *testing.T) {
	t.Run("a Rules media wait refuses a text and a video note", func(t *testing.T) {
		env := newSettingsEditEnv(t)
		owner := env.newOwner(env.chat)
		settingsRulesCleanUsers(t, owner.Id)
		settingsEditSetWait(env, owner, settingsPageRules, settingsEditPartMedia)
		for _, kind := range []string{"text", "videonote"} {
			before := settingsEditSentCount(env, owner)
			sendPrivateMedia(env, owner, kind)
			settingsRulesWantRefusal(t, env, owner, before, "settings_edit_refuse_need_media")
		}
		settingsRulesStoredNothing(t, env.chat.Id)
	})

	t.Run("a Rules media over a long stored text is refused for the caption", func(t *testing.T) {
		env := newSettingsEditEnv(t)
		owner := env.newOwner(env.chat)
		settingsRulesCleanUsers(t, owner.Id)
		if err := rules.SetChatRules(env.chat.Id, strings.Repeat("a", 1100)); err != nil {
			t.Fatalf("SetChatRules: %v", err)
		}
		settingsEditSetWait(env, owner, settingsPageRules, settingsEditPartMedia)
		before := settingsEditSentCount(env, owner)
		sendPrivateMedia(env, owner, "photo")
		settingsRulesWantRefusal(t, env, owner, before, "settings_edit_refuse_media_rules_long")
		settingsRulesStoredNothing(t, env.chat.Id)
	})

	t.Run("a Rules text over the caption limit is refused while media is stored", func(t *testing.T) {
		env := newSettingsEditEnv(t)
		owner := env.newOwner(env.chat)
		settingsRulesCleanUsers(t, owner.Id)
		if err := rules.SetRulesMediaOnly(env.chat.Id, "rules-photo", db.PHOTO); err != nil {
			t.Fatalf("SetRulesMediaOnly: %v", err)
		}
		settingsEditSetWait(env, owner, settingsPageRules, settingsEditPartText)
		before := settingsEditSentCount(env, owner)
		sendPrivateMessage(env, owner, gotgbot.Message{Text: strings.Repeat("a", 1100)})
		settingsRulesWantRefusal(t, env, owner, before, "settings_edit_refuse_text_rules_long")
		if row, _ := settingsEditRulesStored(t, env.chat.Id); row.Rules != "" {
			t.Errorf("a refused rules text was stored: %q", row.Rules)
		}
	})

	t.Run("a Rules buttons wait refuses a bad link and 101 buttons", func(t *testing.T) {
		env := newSettingsEditEnv(t)
		owner := env.newOwner(env.chat)
		settingsRulesCleanUsers(t, owner.Id)
		settingsEditSetWait(env, owner, settingsPageRules, settingsEditPartButtons)
		for _, tc := range []struct{ text, want string }{
			{"[a](buttonurl://https://e.com)\n[b](buttonurl://javascript:alert(1))", "settings_edit_refuse_button_url"},
			{settingsEditButtonLines(101, 1000), "settings_edit_refuse_buttons_limit"},
		} {
			before := settingsEditSentCount(env, owner)
			sendPrivateMessage(env, owner, gotgbot.Message{Text: tc.text})
			settingsRulesWantRefusal(t, env, owner, before, tc.want)
		}
		settingsRulesStoredNothing(t, env.chat.Id)
	})

	t.Run("an album sent to a Rules media wait stores one item and says so", func(t *testing.T) {
		env := newSettingsEditEnv(t)
		owner := env.newOwner(env.chat)
		settingsRulesCleanUsers(t, owner.Id)
		settingsEditSetWait(env, owner, settingsPageRules, settingsEditPartMedia)
		var writes atomic.Int32
		previous := rulesWrites.SetRulesMediaOnly
		rulesWrites.SetRulesMediaOnly = func(chatID int64, fileID string, mediaType int) error {
			writes.Add(1)
			return previous(chatID, fileID, mediaType)
		}
		t.Cleanup(func() { rulesWrites.SetRulesMediaOnly = previous })
		before := settingsEditSentCount(env, owner)

		sendPrivateAlbum(env, owner, settingsMediaAlbumOf(6), true)

		if got := writes.Load(); got != 1 {
			t.Errorf("the album wrote the rules media %d times, want once", got)
		}
		row, _ := settingsEditRulesStored(t, env.chat.Id)
		if !strings.HasPrefix(row.FileID, "album-") || row.MediaType != db.PHOTO {
			t.Errorf("the rules media is file %q type %d, want one of the album's photos", row.FileID, row.MediaType)
		}
		settingsEditWantReply(t, env, owner, before, "settings_edit_saved_media", "settings_edit_album_cut")
		settingsEditWaitGone(t, env, owner)
	})

	t.Run("a caption on a Rules photo is not used and is not echoed", func(t *testing.T) {
		env := newSettingsEditEnv(t)
		owner := env.newOwner(env.chat)
		settingsRulesCleanUsers(t, owner.Id)
		settingsEditSetWait(env, owner, settingsPageRules, settingsEditPartMedia)
		before := settingsEditSentCount(env, owner)
		photo := settingsMediaItemID("photo", "rules-big")
		photo.Caption = "SECRETCAPTION"

		sendPrivateMessage(env, owner, photo)

		row, _ := settingsEditRulesStored(t, env.chat.Id)
		if row.FileID != "rules-big" || row.MediaType != db.PHOTO || row.Rules != "" {
			t.Errorf("the rules are %+v, want only the photo", row)
		}
		settingsEditWantReply(t, env, owner, before, "settings_edit_saved_media", "settings_edit_caption_ignored")
		if strings.Contains(settingsEditPageText(env, owner), "SECRETCAPTION") {
			t.Errorf("the saved page echoes the caption: %q", settingsEditPageText(env, owner))
		}
	})

	t.Run("a bot that is no administrator any more ends the Rules media wait", func(t *testing.T) {
		env := newSettingsEditEnv(t)
		owner := env.newOwner(env.chat)
		settingsRulesCleanUsers(t, owner.Id)
		settingsEditSetWait(env, owner, settingsPageRules, settingsEditPartMedia)
		env.fake.setBotRole(env.chat.Id, staffRoleMember)
		dbcache.DeleteCache(dbcache.CacheKey("admin_list", env.chat.Id))
		dbcache.ResetLocalForTest()
		before := settingsEditSentCount(env, owner)

		sendPrivateMedia(env, owner, "photo")

		settingsEditWantReply(t, env, owner, before, "chat_status_bot_not_admin")
		text := settingsEditTextsSince(env, owner, before)[0]
		if !strings.HasSuffix(text, staffMarker("chat_status_bot_not_admin")+"\n\n"+staffMarker("settings_edit_wait_stopped")) {
			t.Errorf("the reply %q is not the reason, a blank line and the wait-stopped line", text)
		}
		settingsEditWaitGone(t, env, owner)
		settingsRulesStoredNothing(t, env.chat.Id)
	})
}
