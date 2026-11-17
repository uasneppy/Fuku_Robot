package modules

import (
	"errors"
	"fmt"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"gorm.io/gorm"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/chats"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/db/warns"
)

// failWarnSettingsReads makes every query of warns_settings fail until stop is called.
func failWarnSettingsReads(t *testing.T) (stop func()) {
	t.Helper()
	name := fmt.Sprintf("warns-test:fail-settings-reads:%d", uniqueLinkOwnerID())
	err := db.DB.Callback().Query().Before("gorm:query").Register(name, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == (models.WarnSettings{}).TableName() {
			_ = tx.AddError(errors.New("warn settings read failed"))
		}
	})
	if err != nil {
		t.Fatalf("register the failing warn settings read: %v", err)
	}
	removed := false
	stop = func() {
		if removed {
			return
		}
		removed = true
		if err := db.DB.Callback().Query().Remove(name); err != nil {
			t.Errorf("remove the failing warn settings read: %v", err)
		}
	}
	t.Cleanup(stop)
	return stop
}

// TestWarnFailsClosedOnSettingsReadError holds M5: when the warn settings cannot
// be read, /warn stores no warn and punishes nobody, and no default is cached in
// place of the stored settings.
func TestWarnFailsClosedOnSettingsReadError(t *testing.T) {
	client := newModuleBotClient()
	bot := newModuleTestBot(client)
	chatID := uniqueModuleChatID()
	if err := chats.EnsureChatInDb(chatID, "warn fault chat"); err != nil {
		t.Fatalf("ensure chat: %v", err)
	}
	if err := warns.SetWarnLimit(chatID, 7); err != nil {
		t.Fatalf("set warn limit: %v", err)
	}
	target := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Member"}
	t.Cleanup(func() {
		_ = db.DB.Where("chat_id = ?", chatID).Delete(&models.WarnSettings{}).Error
		_ = db.DB.Where("chat_id = ?", chatID).Delete(&models.Warns{}).Error
		_ = db.DB.Where("chat_id = ?", chatID).Delete(&models.Chat{}).Error
		cache.DeleteCache(cache.CacheKey("warn_settings", chatID))
	})
	cache.DeleteCache(cache.CacheKey("warn_settings", chatID))
	cache.ResetLocalForTest()

	stop := failWarnSettingsReads(t)
	chat := gotgbot.Chat{Id: chatID, Type: "supergroup", Title: "Warn Chat"}
	ctx := newWarnReplyContext(bot, chat, gotgbot.User{Id: 777000, FirstName: "Telegram"}, target, "/warn spam")
	if err := warnsModule.warnUser(bot, ctx); err != ext.EndGroups {
		t.Fatalf("warnUser error = %v, want EndGroups", err)
	}
	stop()

	if n, _ := warns.GetWarns(target.Id, chatID); n != 0 {
		t.Fatalf("warns = %d, want none stored while the settings cannot be read", n)
	}
	for _, method := range []string{"restrictChatMember", "banChatMember", "unbanChatMember"} {
		if calls := client.callsFor(method); len(calls) != 0 {
			t.Fatalf("%s calls = %d, want none", method, len(calls))
		}
	}
	if calls := client.callsFor("sendMessage"); len(calls) != 1 {
		t.Fatalf("sendMessage calls = %d, want one refusal", len(calls))
	}
	if got := warns.GetWarnSetting(chatID).WarnLimit; got != 7 {
		t.Fatalf("warn limit after the fault = %d, want the stored 7 (a default was cached)", got)
	}

	// Control: with the settings readable the same /warn is stored.
	ctx = newWarnReplyContext(bot, chat, gotgbot.User{Id: 777000, FirstName: "Telegram"}, target, "/warn spam")
	if err := warnsModule.warnUser(bot, ctx); err != ext.EndGroups {
		t.Fatalf("warnUser error = %v, want EndGroups", err)
	}
	if n, _ := warns.GetWarns(target.Id, chatID); n != 1 {
		t.Fatalf("warns = %d, want 1 once the settings are readable", n)
	}
}
