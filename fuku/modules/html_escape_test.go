package modules

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/chats"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/db/notes"
	"github.com/uasneppy/fuku_robot/fuku/utils/helpers"
)

// sentTextOf is the text of the only sendMessage call.
func sentTextOf(t *testing.T, client *moduleBotClient) string {
	t.Helper()
	calls := client.callsFor("sendMessage")
	if len(calls) == 0 {
		t.Fatal("no sendMessage call")
	}
	return fmt.Sprint(calls[len(calls)-1].Params["text"])
}

// TestRepliesEscapeUserText holds M9: names, titles, translations and note
// names that hold HTML go into HTML replies escaped.
func TestRepliesEscapeUserText(t *testing.T) {
	withRealLocales(t)
	chat := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Escape Chat"}
	admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}

	t.Run("title", func(t *testing.T) {
		client := newModuleBotClient()
		client.responses["getChatMember"] = []byte(
			`{"status":"administrator","user":{"id":42,"is_bot":false,"first_name":"<i>Eve"},"custom_title":"<b>Boss","can_promote_members":true}`,
		)
		client.responses["getChatAdministrators"] = []byte(
			`[{"status":"administrator","user":{"id":999,"is_bot":true,"first_name":"Fuku"}},` +
				`{"status":"administrator","user":{"id":42,"is_bot":false,"first_name":"<i>Eve"},"can_promote_members":true}]`,
		)
		bot := newModuleTestBot(client)
		ctx := newBanReplyContext(bot, chat, admin, gotgbot.User{Id: 42, FirstName: "<i>Eve"}, "/title Boss")
		cmdCtx, err := helpers.BuildCommandContext(bot, ctx)
		if err != nil {
			t.Fatalf("BuildCommandContext failed: %v", err)
		}
		if err := adminModule.setTitle(cmdCtx); err != ext.EndGroups {
			t.Fatalf("setTitle error = %v", err)
		}
		text := sentTextOf(t, client)
		if strings.Contains(text, "<i>") || strings.Contains(text, "<b><b>") || !strings.Contains(text, "&lt;i&gt;Eve") {
			t.Fatalf("title reply = %q, want the name and title escaped", text)
		}
	})

	t.Run("translate", func(t *testing.T) {
		withMiscHTTPClient(t, &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(`[["<script>x</script>","e<n"]]`)),
				Header:     make(http.Header),
			}, nil
		})})
		client := newModuleBotClient()
		bot := newModuleTestBot(client)
		ctx := newModuleMessageContext(bot, chat, admin, "/tr en hola")
		if err := miscModule.translate(bot, ctx); err != ext.EndGroups {
			t.Fatalf("translate error = %v", err)
		}
		text := sentTextOf(t, client)
		if strings.Contains(text, "<script>") || !strings.Contains(text, "&lt;script&gt;") || !strings.Contains(text, "e&lt;n") {
			t.Fatalf("translate reply = %q, want the translation escaped", text)
		}
	})

	t.Run("group notes list", func(t *testing.T) {
		chatID := uniqueModuleChatID()
		if err := chats.EnsureChatInDb(chatID, "notes chat"); err != nil {
			t.Fatalf("ensure chat: %v", err)
		}
		if err := notes.AddNote(chatID, "a<b", "text", "", nil, 1, false, false, false, false, false, false); err != nil {
			t.Fatalf("add note: %v", err)
		}
		t.Cleanup(func() {
			_ = db.DB.Where("chat_id = ?", chatID).Delete(&models.Notes{}).Error
			_ = db.DB.Where("chat_id = ?", chatID).Delete(&models.NotesSettings{}).Error
			_ = db.DB.Where("chat_id = ?", chatID).Delete(&models.Chat{}).Error
			cache.DeleteCache(cache.CacheKey("notes_list", chatID))
		})
		client := newModuleBotClient()
		bot := newModuleTestBot(client)
		group := gotgbot.Chat{Id: chatID, Type: "supergroup", Title: "Notes Chat"}
		ctx := newModuleMessageContext(bot, group, admin, "/notes")
		if err := notesModule.notesList(bot, ctx); err != ext.EndGroups {
			t.Fatalf("notesList error = %v", err)
		}
		text := sentTextOf(t, client)
		if strings.Contains(text, "#a<b") || !strings.Contains(text, "#a&lt;b") {
			t.Fatalf("notes list = %q, want the note name escaped", text)
		}
	})
}
