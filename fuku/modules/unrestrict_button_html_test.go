package modules

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

// TestUnrestrictButtonEditKeepsTextSafe holds M8: the unrestrict card's edit
// re-inserts the card's own text as HTML, so a name with "<" or "&" is escaped
// (and a mention entity kept), and the press is answered even when the edit fails.
func TestUnrestrictButtonEditKeepsTextSafe(t *testing.T) {
	chat := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Ban Chat"}
	admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	data := encodeCallbackData("unrestrict", map[string]string{"a": "unban", "u": "42"})
	cardText := "Banned <b>Eve</b> & co"

	t.Run("escaped", func(t *testing.T) {
		client := newModuleBotClient()
		bot := newModuleTestBot(client)
		ctx := newModuleCallbackContext(bot, chat, admin, data)
		ctx.EffectiveMessage.Text = cardText
		_ = bansModule.unrestrictButtonHandler(bot, ctx)
		calls := client.callsFor("editMessageText")
		if len(calls) != 1 {
			t.Fatalf("editMessageText calls = %d, want 1", len(calls))
		}
		text := fmt.Sprint(calls[0].Params["text"])
		if !strings.Contains(text, "&lt;b&gt;Eve&lt;/b&gt; &amp; co") {
			t.Fatalf("edited text = %q, want the card's text escaped", text)
		}
	})

	t.Run("answered when the edit fails", func(t *testing.T) {
		client := newModuleBotClient()
		client.errors["editMessageText"] = errors.New("Bad Request: message can't be edited")
		bot := newModuleTestBot(client)
		ctx := newModuleCallbackContext(bot, chat, admin, data)
		_ = bansModule.unrestrictButtonHandler(bot, ctx)
		if calls := client.callsFor("answerCallbackQuery"); len(calls) != 1 {
			t.Fatalf("answerCallbackQuery calls = %d, want 1", len(calls))
		}
	})
}
