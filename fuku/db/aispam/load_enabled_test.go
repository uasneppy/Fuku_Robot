//go:build testtools

package aispam

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
)

func TestLoadEnabledContext(t *testing.T) {
	skipIfNoDb(t)
	cache.SetupTestMemoryMarshaler(t)
	ctx := context.Background()

	countRows := func(chatID int64) int64 {
		t.Helper()
		var count int64
		if err := db.DB.Model(&models.AISpamSettings{}).Where("chat_id = ?", chatID).Count(&count).Error; err != nil {
			t.Fatalf("count failed: %v", err)
		}
		return count
	}

	t.Run("missing row reads off and creates nothing", func(t *testing.T) {
		chatID := time.Now().UnixNano()
		cleanupChat(t, chatID)

		enabled, err := LoadEnabledContext(ctx, chatID)
		if err != nil {
			t.Fatalf("LoadEnabledContext error = %v", err)
		}
		if enabled {
			t.Error("missing row = on, want off")
		}
		if n := countRows(chatID); n != 0 {
			t.Errorf("rows after a read = %d, want 0", n)
		}
	})

	t.Run("stored value and a later write are seen", func(t *testing.T) {
		chatID := time.Now().UnixNano() + 1
		cleanupChat(t, chatID)

		if err := SetAISpamEnabled(chatID, true); err != nil {
			t.Fatalf("SetAISpamEnabled(true) error = %v", err)
		}
		enabled, err := LoadEnabledContext(ctx, chatID)
		if err != nil || !enabled {
			t.Fatalf("after enable = %v, %v, want on, nil", enabled, err)
		}

		if err := SetAISpamEnabled(chatID, false); err != nil {
			t.Fatalf("SetAISpamEnabled(false) error = %v", err)
		}
		enabled, err = LoadEnabledContext(ctx, chatID)
		if err != nil || enabled {
			t.Fatalf("after disable = %v, %v, want off, nil (the write drops the cache key)", enabled, err)
		}
	})

	t.Run("a read error is returned, not hidden as off", func(t *testing.T) {
		chatID := time.Now().UnixNano() + 2
		cleanupChat(t, chatID)

		const name = "test:fail_ai_spam_read"
		err := db.DB.Callback().Query().Before("gorm:query").Register(name, func(tx *gorm.DB) {
			if tx.Statement.Table == "ai_spam_settings" {
				_ = tx.AddError(errors.New("database is down"))
			}
		})
		if err != nil {
			t.Fatalf("register callback: %v", err)
		}
		t.Cleanup(func() {
			if err := db.DB.Callback().Query().Remove(name); err != nil {
				t.Errorf("remove callback: %v", err)
			}
		})

		enabled, err := LoadEnabledContext(ctx, chatID)
		if err == nil {
			t.Fatal("LoadEnabledContext error = nil, want the read error")
		}
		if enabled {
			t.Error("enabled = true alongside an error")
		}
		if IsAISpamEnabledContext(ctx, chatID) {
			t.Error("the hiding getter must still read an error as off")
		}
	})
}
