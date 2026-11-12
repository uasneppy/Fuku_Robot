package lang

import (
	"context"
	"testing"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

func TestLoadGroupLanguageContext(t *testing.T) {
	skipIfNoDb(t)

	cleanup := func(chatID int64) {
		clean := func() {
			_ = db.DB.Where("chat_id = ?", chatID).Delete(&models.Chat{}).Error
			cache.DeleteCache(cache.CacheKey("chat_lang", chatID))
			cache.DeleteCache(cache.CacheKey("chat_settings", chatID))
			cache.DeleteCache(cache.CacheKey("chat", chatID))
			cache.ResetLocalForTest()
		}
		clean()
		t.Cleanup(clean)
	}
	countChats := func(chatID int64) int64 {
		var n int64
		if err := db.DB.Model(&models.Chat{}).Where("chat_id = ?", chatID).Count(&n).Error; err != nil {
			t.Fatalf("count chats: %v", err)
		}
		return n
	}

	t.Run("no row reads uk and creates nothing", func(t *testing.T) {
		const chatID int64 = -1009918001601
		cleanup(chatID)
		got, err := LoadGroupLanguageContext(context.Background(), chatID)
		if err != nil || got != "uk" {
			t.Fatalf("LoadGroupLanguageContext() = %q, %v; want uk, nil", got, err)
		}
		if n := countChats(chatID); n != 0 {
			t.Fatalf("chats rows = %d, want none created", n)
		}
	})

	t.Run("a stored language reads back", func(t *testing.T) {
		const chatID int64 = -1009918001602
		cleanup(chatID)
		if err := db.DB.Create(&models.Chat{ChatId: chatID, Language: "es"}).Error; err != nil {
			t.Fatalf("create chat: %v", err)
		}
		got, err := LoadGroupLanguageContext(context.Background(), chatID)
		if err != nil || got != "es" {
			t.Fatalf("LoadGroupLanguageContext() = %q, %v; want es, nil", got, err)
		}
	})

	t.Run("a language written through ChangeGroupLanguage is seen", func(t *testing.T) {
		const chatID int64 = -1009918001603
		cleanup(chatID)
		if err := ChangeGroupLanguage(chatID, "en"); err != nil {
			t.Fatalf("ChangeGroupLanguage(en): %v", err)
		}
		if got, err := LoadGroupLanguageContext(context.Background(), chatID); err != nil || got != "en" {
			t.Fatalf("after en: %q, %v; want en, nil", got, err)
		}
		if err := ChangeGroupLanguage(chatID, "fr"); err != nil {
			t.Fatalf("ChangeGroupLanguage(fr): %v", err)
		}
		if got, err := LoadGroupLanguageContext(context.Background(), chatID); err != nil || got != "fr" {
			t.Fatalf("after fr: %q, %v; want fr, nil", got, err)
		}
	})

	t.Run("a read error is returned, not uk", func(t *testing.T) {
		const chatID int64 = -1009918001604
		cleanup(chatID)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		got, err := LoadGroupLanguageContext(ctx, chatID)
		if err == nil {
			t.Fatalf("LoadGroupLanguageContext() with a cancelled context = %q, nil; want an error", got)
		}
		if got != "" {
			t.Fatalf("language on error = %q, want empty", got)
		}
	})
}
