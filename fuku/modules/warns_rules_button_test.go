package modules

import (
	"fmt"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/chats"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/db/rules"
)

// TestWarnRulesButtonForMediaOnlyRules holds M12: a warn shows the Rules button
// whenever the group's rules are set (RulesSettings.HasContent), a media-only
// rules message included.
func TestWarnRulesButtonForMediaOnlyRules(t *testing.T) {
	client := newModuleBotClient()
	bot := newModuleTestBot(client)
	chatID := uniqueModuleChatID()
	if err := chats.EnsureChatInDb(chatID, "rules chat"); err != nil {
		t.Fatalf("ensure chat: %v", err)
	}
	if err := rules.SetRulesMediaOnly(chatID, "photo-file-id", 2); err != nil {
		t.Fatalf("set rules media: %v", err)
	}
	target := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Member"}
	t.Cleanup(func() {
		_ = db.DB.Where("chat_id = ?", chatID).Delete(&models.RulesSettings{}).Error
		_ = db.DB.Where("chat_id = ?", chatID).Delete(&models.Warns{}).Error
		_ = db.DB.Where("chat_id = ?", chatID).Delete(&models.WarnSettings{}).Error
		_ = db.DB.Where("chat_id = ?", chatID).Delete(&models.Chat{}).Error
		_ = db.DB.Where("user_id = ?", target.Id).Delete(&models.User{}).Error
		cache.DeleteCache(cache.CacheKey("warn_settings", chatID))
	})
	chat := gotgbot.Chat{Id: chatID, Type: "supergroup", Title: "Rules Chat"}
	ctx := newWarnReplyContext(bot, chat, gotgbot.User{Id: 777000, FirstName: "Telegram"}, target, "/warn spam")
	if err := warnsModule.warnUser(bot, ctx); err != ext.EndGroups {
		t.Fatalf("warnUser error = %v, want EndGroups", err)
	}
	calls := client.callsFor("sendMessage")
	if len(calls) != 1 {
		t.Fatalf("sendMessage calls = %d, want 1", len(calls))
	}
	markup := fmt.Sprintf("%+v", calls[0].Params["reply_markup"])
	if !strings.Contains(markup, fmt.Sprintf("start=rules_%d", chatID)) {
		t.Fatalf("warn keyboard = %s, want the Rules button", markup)
	}
}
