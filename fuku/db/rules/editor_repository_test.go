package rules

import (
	"context"
	"testing"
	"time"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/chats"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

func TestLoadRulesCreatesNothing(t *testing.T) {
	skipIfNoDb(t)

	count := func(model any, chatID int64) int64 {
		t.Helper()
		var n int64
		if err := db.DB.Model(model).Where("chat_id = ?", chatID).Count(&n).Error; err != nil {
			t.Fatalf("count rows: %v", err)
		}
		return n
	}

	t.Run("no rules row is empty text and no error", func(t *testing.T) {
		chatID := time.Now().UnixNano()

		got, err := LoadRulesContext(context.Background(), chatID)
		if err != nil {
			t.Fatalf("LoadRulesContext() error = %v", err)
		}
		if got != "" {
			t.Fatalf("expected empty rules, got %q", got)
		}
		if n := count(&models.RulesSettings{}, chatID); n != 0 {
			t.Fatalf("created %d rules rows", n)
		}
		if n := count(&models.Chat{}, chatID); n != 0 {
			t.Fatalf("created %d chats rows", n)
		}
	})

	t.Run("stored text is returned after SetChatRules", func(t *testing.T) {
		chatID := time.Now().UnixNano()
		t.Cleanup(func() {
			_ = db.DB.Where("chat_id = ?", chatID).Delete(&models.RulesSettings{}).Error
			_ = db.DB.Where("chat_id = ?", chatID).Delete(&models.Chat{}).Error
		})
		if err := chats.EnsureChatInDb(chatID, ""); err != nil {
			t.Fatalf("EnsureChatInDb() error = %v", err)
		}
		if err := SetChatRules(chatID, "be kind"); err != nil {
			t.Fatalf("SetChatRules() error = %v", err)
		}

		got, err := LoadRulesContext(context.Background(), chatID)
		if err != nil {
			t.Fatalf("LoadRulesContext() error = %v", err)
		}
		if got != "be kind" {
			t.Fatalf("expected stored rules, got %q", got)
		}
	})

	t.Run("a cancelled context is an error", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		got, err := LoadRulesContext(ctx, time.Now().UnixNano())
		if err == nil {
			t.Fatalf("expected an error for a cancelled context, got %q", got)
		}
		if got != "" {
			t.Fatalf("expected no text with an error, got %q", got)
		}
	})
}
