//go:build testtools

package modules

import (
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// TestReactionCap holds the cap of reactionsCapMax reactions a chat: /addreaction of a new keyword at the cap is
// refused with the limit text and stores nothing, while a new emoji for a keyword the chat has still lands.
func TestReactionCap(t *testing.T) {
	withRealLocales(t)
	chatID := uniqueModuleChatID()
	reactionGoldenCleanup(t, chatID)
	batch := make([]models.Reactions, 0, reactionsCapMax)
	for n := 0; n < reactionsCapMax; n++ {
		batch = append(batch, models.Reactions{ChatID: chatID, Keyword: "w" + strings.Repeat("x", n%5) + string(rune('a'+n%26)) + string(rune('a'+n/26)), Emoji: "👍"})
	}
	if err := db.DB.CreateInBatches(batch, 50).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	chat := gotgbot.Chat{Id: chatID, Type: "supergroup", Title: "Cap"}
	run := func(text string) string {
		client := newModuleBotClient()
		bot := newModuleTestBot(client)
		if err := reactionsModule.addReaction(bot, newModuleMessageContext(bot, chat, admin, text)); err != ext.EndGroups {
			t.Fatalf("addReaction = %v", err)
		}
		calls := client.callsFor("sendMessage")
		if len(calls) != 1 {
			t.Fatalf("replies = %d, want 1", len(calls))
		}
		got, _ := calls[0].Params["text"].(string)
		return got
	}
	if got := run("/addreaction brandnew 🔥"); !strings.Contains(got, "100") {
		t.Fatalf("reply at the cap = %q, want the limit text with 100", got)
	}
	var n int64
	db.DB.Model(&models.Reactions{}).Where("chat_id = ?", chatID).Count(&n)
	if n != reactionsCapMax {
		t.Fatalf("rows = %d, want %d", n, reactionsCapMax)
	}
	keyword := batch[0].Keyword
	if got := run("/addreaction " + keyword + " 🔥"); strings.Contains(got, "100") {
		t.Fatalf("replacing an existing keyword's emoji at the cap = %q, want it stored", got)
	}
	var row models.Reactions
	if err := db.DB.Where("chat_id = ? AND keyword = ?", chatID, keyword).Take(&row).Error; err != nil || row.Emoji != "🔥" {
		t.Fatalf("row = %+v (%v), want the new emoji", row, err)
	}
}
