package lang

import (
	"context"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

func skipIfNoDb(t *testing.T) {
	if db.DB == nil {
		t.Fatal("test database was not initialized")
	}
}

func TestLanguageCRUD(t *testing.T) {
	skipIfNoDb(t)

	t.Run("group defaults to uk", func(t *testing.T) {
		chatID := time.Now().UnixNano()

		t.Cleanup(func() {
			_ = db.DB.Where("chat_id = ?", chatID).Delete(&db.Chat{}).Error
			cache.DeleteCache(cache.CacheKey("chat_lang", chatID))
		})

		lang := getGroupLanguage(chatID)
		if lang != "uk" {
			t.Fatalf("expected default language 'en', got %q", lang)
		}
	})

	t.Run("user defaults to uk", func(t *testing.T) {
		userID := time.Now().UnixNano()

		t.Cleanup(func() {
			_ = db.DB.Where("user_id = ?", userID).Delete(&db.User{}).Error
			cache.DeleteCache(cache.CacheKey("user_lang", userID))
		})

		lang := getUserLanguage(userID)
		if lang != "uk" {
			t.Fatalf("expected default language 'en', got %q", lang)
		}
	})

	t.Run("group set and get", func(t *testing.T) {
		chatID := time.Now().UnixNano()

		t.Cleanup(func() {
			_ = db.DB.Where("chat_id = ?", chatID).Delete(&db.Chat{}).Error
			cache.DeleteCache(cache.CacheKey("chat_lang", chatID))
			cache.DeleteCache(cache.CacheKey("chat_settings", chatID))
			cache.DeleteCache(cache.CacheKey("chat", chatID))
		})

		_ = ChangeGroupLanguage(chatID, "es")

		lang := getGroupLanguage(chatID)
		if lang != "es" {
			t.Fatalf("expected language 'es', got %q", lang)
		}
	})

	t.Run("user set and get", func(t *testing.T) {
		userID := time.Now().UnixNano()

		t.Cleanup(func() {
			_ = db.DB.Where("user_id = ?", userID).Delete(&db.User{}).Error
			cache.DeleteCache(cache.CacheKey("user_lang", userID))
			cache.DeleteCache(cache.CacheKey("user", userID))
		})

		_ = ChangeUserLanguage(userID, "fr")

		lang := getUserLanguage(userID)
		if lang != "fr" {
			t.Fatalf("expected language 'fr', got %q", lang)
		}
	})

	t.Run("group overwrite", func(t *testing.T) {
		chatID := time.Now().UnixNano()

		t.Cleanup(func() {
			_ = db.DB.Where("chat_id = ?", chatID).Delete(&db.Chat{}).Error
			cache.DeleteCache(cache.CacheKey("chat_lang", chatID))
			cache.DeleteCache(cache.CacheKey("chat_settings", chatID))
			cache.DeleteCache(cache.CacheKey("chat", chatID))
		})

		_ = ChangeGroupLanguage(chatID, "en")

		_ = ChangeGroupLanguage(chatID, "hi")

		lang := getGroupLanguage(chatID)
		if lang != "hi" {
			t.Fatalf("expected language 'hi', got %q", lang)
		}
	})

	t.Run("user overwrite", func(t *testing.T) {
		userID := time.Now().UnixNano()

		t.Cleanup(func() {
			_ = db.DB.Where("user_id = ?", userID).Delete(&db.User{}).Error
			cache.DeleteCache(cache.CacheKey("user_lang", userID))
			cache.DeleteCache(cache.CacheKey("user", userID))
		})

		_ = ChangeUserLanguage(userID, "en")

		_ = ChangeUserLanguage(userID, "es")

		lang := getUserLanguage(userID)
		if lang != "es" {
			t.Fatalf("expected language 'es', got %q", lang)
		}
	})

	t.Run("group noop when same", func(t *testing.T) {
		chatID := time.Now().UnixNano()

		t.Cleanup(func() {
			_ = db.DB.Where("chat_id = ?", chatID).Delete(&db.Chat{}).Error
			cache.DeleteCache(cache.CacheKey("chat_lang", chatID))
			cache.DeleteCache(cache.CacheKey("chat_settings", chatID))
			cache.DeleteCache(cache.CacheKey("chat", chatID))
		})

		_ = ChangeGroupLanguage(chatID, "en")
		// Calling again with same value should be a no-op (no error)
		_ = ChangeGroupLanguage(chatID, "en")

		lang := getGroupLanguage(chatID)
		if lang != "en" {
			t.Fatalf("expected language 'en', got %q", lang)
		}
	})

	t.Run("user noop when same", func(t *testing.T) {
		userID := time.Now().UnixNano()

		t.Cleanup(func() {
			_ = db.DB.Where("user_id = ?", userID).Delete(&db.User{}).Error
			cache.DeleteCache(cache.CacheKey("user_lang", userID))
			cache.DeleteCache(cache.CacheKey("user", userID))
		})

		_ = ChangeUserLanguage(userID, "fr")
		// Calling again with same value should be a no-op
		_ = ChangeUserLanguage(userID, "fr")

		lang := getUserLanguage(userID)
		if lang != "fr" {
			t.Fatalf("expected language 'fr', got %q", lang)
		}
	})
}

func TestGetLanguageFromPrivateAndGroupContexts(t *testing.T) {
	skipIfNoDb(t)

	base := time.Now().UnixNano()
	userID := base + 901001
	groupID := -1000000000000 - (base % 1000000)
	privateChatID := userID + 1

	t.Cleanup(func() {
		if err := db.DB.Where("user_id = ?", userID).Delete(&db.User{}).Error; err != nil {
			t.Fatalf("cleanup user language: %v", err)
		}
		cache.DeleteCache(cache.CacheKey("user_lang", userID))
		if err := db.DB.Where("chat_id = ?", groupID).Delete(&db.Chat{}).Error; err != nil {
			t.Fatalf("cleanup group language: %v", err)
		}
		cache.DeleteCache(cache.CacheKey("chat_lang", groupID))
	})

	if got := GetLanguage(nil); got != "uk" {
		t.Fatalf("GetLanguage(nil) = %q, want uk", got)
	}
	if got := GetLanguage(&ext.Context{}); got != "uk" {
		t.Fatalf("GetLanguage(empty context) = %q, want uk", got)
	}

	if err := ChangeUserLanguage(userID, "es"); err != nil {
		t.Fatalf("ChangeUserLanguage() error = %v", err)
	}
	privateCtx := ext.NewContext(
		&gotgbot.Bot{User: gotgbot.User{Id: 999, IsBot: true}},
		&gotgbot.Update{
			Message: &gotgbot.Message{
				Chat: gotgbot.Chat{Id: userID, Type: "private", FirstName: "Tester"},
				From: &gotgbot.User{Id: userID, FirstName: "Tester"},
			},
		},
		nil,
	)
	if got := GetLanguage(privateCtx); got != "es" {
		t.Fatalf("GetLanguage(private) = %q, want es", got)
	}

	noSenderCtx := ext.NewContext(
		&gotgbot.Bot{User: gotgbot.User{Id: 999, IsBot: true}},
		&gotgbot.Update{Message: &gotgbot.Message{
			Chat: gotgbot.Chat{Id: privateChatID, Type: "private", FirstName: "No Sender"},
		}},
		nil,
	)
	if got := GetLanguage(noSenderCtx); got != "uk" {
		t.Fatalf("GetLanguage(private without sender) = %q, want uk", got)
	}

	if err := ChangeGroupLanguage(groupID, "fr"); err != nil {
		t.Fatalf("ChangeGroupLanguage() error = %v", err)
	}
	groupCtx := ext.NewContext(
		&gotgbot.Bot{User: gotgbot.User{Id: 999, IsBot: true}},
		&gotgbot.Update{Message: &gotgbot.Message{
			Chat: gotgbot.Chat{Id: groupID, Type: "supergroup", Title: "Lang Group"},
			From: &gotgbot.User{Id: userID, FirstName: "Tester"},
		}},
		nil,
	)
	if got := GetLanguage(groupCtx); got != "fr" {
		t.Fatalf("GetLanguage(group) = %q, want fr", got)
	}
}

func TestGetGroupLanguage(t *testing.T) {
	skipIfNoDb(t)

	ctx := context.Background()
	base := time.Now().UnixNano()
	forget := func(t *testing.T, chatID int64) {
		t.Helper()
		t.Cleanup(func() {
			_ = db.DB.Where("chat_id = ?", chatID).Delete(&models.Chat{}).Error
			cache.DeleteCache(cache.CacheKey("chat_lang", chatID))
			cache.DeleteCache(cache.CacheKey("chat_settings", chatID))
			cache.DeleteCache(cache.CacheKey("chat", chatID))
		})
	}
	chatRows := func(t *testing.T, chatID int64) int64 {
		t.Helper()
		var n int64
		if err := db.DB.Model(&models.Chat{}).Where("chat_id = ?", chatID).Count(&n).Error; err != nil {
			t.Fatalf("count chats rows: %v", err)
		}
		return n
	}

	t.Run("a stored language is read", func(t *testing.T) {
		chatID := base + 1
		forget(t, chatID)
		if err := ChangeGroupLanguage(chatID, "es"); err != nil {
			t.Fatalf("ChangeGroupLanguage() error = %v", err)
		}
		if got := GetGroupLanguage(ctx, chatID); got != "es" {
			t.Fatalf("GetGroupLanguage() = %q, want es", got)
		}
	})

	t.Run("a chat with no row reads uk and creates nothing", func(t *testing.T) {
		chatID := base + 2
		forget(t, chatID)
		if got := GetGroupLanguage(ctx, chatID); got != "uk" {
			t.Fatalf("GetGroupLanguage() = %q, want uk", got)
		}
		if n := chatRows(t, chatID); n != 0 {
			t.Fatalf("the read created %d chats rows", n)
		}
	})

	t.Run("an empty language reads uk", func(t *testing.T) {
		chatID := base + 3
		forget(t, chatID)
		if err := ChangeGroupLanguage(chatID, "es"); err != nil {
			t.Fatalf("ChangeGroupLanguage() error = %v", err)
		}
		if err := db.DB.Model(&models.Chat{}).Where("chat_id = ?", chatID).Update("language", "").Error; err != nil {
			t.Fatalf("blank the language: %v", err)
		}
		cache.DeleteCache(cache.CacheKey("chat_lang", chatID))
		if got := GetGroupLanguage(ctx, chatID); got != "uk" {
			t.Fatalf("GetGroupLanguage() = %q, want uk", got)
		}
	})

	t.Run("a change is seen by the next read", func(t *testing.T) {
		chatID := base + 4
		forget(t, chatID)
		if err := ChangeGroupLanguage(chatID, "es"); err != nil {
			t.Fatalf("ChangeGroupLanguage() error = %v", err)
		}
		if got := GetGroupLanguage(ctx, chatID); got != "es" {
			t.Fatalf("GetGroupLanguage() = %q, want es", got)
		}
		if err := ChangeGroupLanguage(chatID, "fr"); err != nil {
			t.Fatalf("ChangeGroupLanguage() error = %v", err)
		}
		if got := GetGroupLanguage(ctx, chatID); got != "fr" {
			t.Fatalf("GetGroupLanguage() after a change = %q, want fr", got)
		}
	})

	t.Run("it equals GetLanguage of a group context", func(t *testing.T) {
		chatID := -1000000000000 - (base % 1000000)
		forget(t, chatID)
		if err := ChangeGroupLanguage(chatID, "pt"); err != nil {
			t.Fatalf("ChangeGroupLanguage() error = %v", err)
		}
		groupCtx := ext.NewContext(
			&gotgbot.Bot{User: gotgbot.User{Id: 999, IsBot: true}},
			&gotgbot.Update{Message: &gotgbot.Message{
				Chat: gotgbot.Chat{Id: chatID, Type: "supergroup", Title: "Lang Group"},
				From: &gotgbot.User{Id: 5, FirstName: "Tester"},
			}},
			nil,
		)
		if got, want := GetGroupLanguage(ctx, chatID), GetLanguage(groupCtx); got != want || got != "pt" {
			t.Fatalf("GetGroupLanguage() = %q, GetLanguage() = %q, want both pt", got, want)
		}
	})
}
