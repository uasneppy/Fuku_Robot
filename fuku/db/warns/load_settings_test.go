package warns

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	gocache "github.com/eko/gocache/lib/v4/cache"
	"github.com/eko/gocache/lib/v4/marshaler"
	gocache_store "github.com/eko/gocache/store/redis/v4"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/uasneppy/fuku_robot/fuku/config"
	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/chats"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	utilsCache "github.com/uasneppy/fuku_robot/fuku/utils/cache"
)

// useMiniredisCache wires the real cache stack against miniredis for one test, so a test can
// see which keys a read leaves behind.
func useMiniredisCache(t *testing.T) *miniredis.Miniredis {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	manager := gocache.New[any](gocache_store.NewRedis(client))
	previousMarshal, previousManager, previousClient := utilsCache.GetCacheState()
	utilsCache.SetCacheState(marshaler.New(manager), manager, client)

	previousConfig := config.AppConfig
	cfg := config.Config{}
	if previousConfig != nil {
		cfg = *previousConfig
	}
	cfg.DisableCache = false
	config.AppConfig = &cfg
	cache.ResetLocalForTest()

	t.Cleanup(func() {
		config.AppConfig = previousConfig
		cache.ResetLocalForTest()
		_ = client.Close()
		utilsCache.SetCacheState(previousMarshal, previousManager, previousClient)
	})
	return mr
}

// warnSettingsKeys lists the cache keys of the chat's warn settings.
func warnSettingsKeys(mr *miniredis.Miniredis, chatID int64) []string {
	var keys []string
	for _, key := range mr.Keys() {
		if strings.Contains(key, "warn_settings") && strings.Contains(key, strconv.FormatInt(chatID, 10)) {
			keys = append(keys, key)
		}
	}
	return keys
}

// countWarnSettingsRows counts the chat's warns_settings rows straight from the table.
func countWarnSettingsRows(t *testing.T, chatID int64) int64 {
	t.Helper()
	var n int64
	if err := db.DB.Model(&models.WarnSettings{}).Where("chat_id = ?", chatID).Count(&n).Error; err != nil {
		t.Fatalf("count warns_settings rows: %v", err)
	}
	return n
}

// TestLoadWarnSettingsContext proves the page's reader: a missing row reads the defaults and
// creates and caches nothing, a stored row is read back, a write is seen at once and a read
// error is returned, not hidden.
func TestLoadWarnSettingsContext(t *testing.T) {
	skipIfNoDb(t)
	ctx := context.Background()

	known := func(t *testing.T) int64 {
		t.Helper()
		chatID := time.Now().UnixNano()
		if err := chats.EnsureChatInDb(chatID, "load-warn-settings"); err != nil {
			t.Fatalf("EnsureChatInDb() error = %v", err)
		}
		t.Cleanup(func() {
			db.DB.Where("chat_id = ?", chatID).Delete(&models.WarnSettings{})
			db.DB.Where("chat_id = ?", chatID).Delete(&models.Chat{})
			cache.DeleteCache(cache.CacheKey("warn_settings", chatID))
		})
		return chatID
	}

	t.Run("missing row reads defaults and creates nothing", func(t *testing.T) {
		mr := useMiniredisCache(t)
		chatID := known(t)

		got, err := LoadWarnSettingsContext(ctx, chatID)
		if err != nil {
			t.Fatalf("LoadWarnSettingsContext() error = %v", err)
		}
		if got.WarnLimit != 3 || got.WarnMode != "mute" || got.ChatId != chatID {
			t.Fatalf("LoadWarnSettingsContext() = %+v, want limit 3 and mode mute for the chat", got)
		}
		if n := countWarnSettingsRows(t, chatID); n != 0 {
			t.Fatalf("rows after the read = %d, want none", n)
		}
		if keys := warnSettingsKeys(mr, chatID); len(keys) != 0 {
			t.Fatalf("cache keys after the read = %v, want none", keys)
		}

		// The command's own loader still finds its miss and creates the row, as before.
		if cmd := GetWarnSettingContext(ctx, chatID); cmd.WarnLimit != 3 || cmd.WarnMode != "mute" {
			t.Fatalf("GetWarnSettingContext() = %+v, want the defaults", cmd)
		}
		if n := countWarnSettingsRows(t, chatID); n != 1 {
			t.Fatalf("rows after the command's read = %d, want the one row it creates", n)
		}
	})

	t.Run("stored row is read back and cached under the command's key", func(t *testing.T) {
		mr := useMiniredisCache(t)
		chatID := known(t)
		if err := SetWarnLimit(chatID, 12); err != nil {
			t.Fatalf("SetWarnLimit() error = %v", err)
		}
		if err := SetWarnMode(chatID, "ban"); err != nil {
			t.Fatalf("SetWarnMode() error = %v", err)
		}

		got, err := LoadWarnSettingsContext(ctx, chatID)
		if err != nil {
			t.Fatalf("LoadWarnSettingsContext() error = %v", err)
		}
		if got.WarnLimit != 12 || got.WarnMode != "ban" {
			t.Fatalf("LoadWarnSettingsContext() = %+v, want limit 12 and mode ban", got)
		}
		if keys := warnSettingsKeys(mr, chatID); len(keys) != 1 {
			t.Fatalf("cache keys after reading a stored row = %v, want the command's one key", keys)
		}
		if cmd := GetWarnSettingContext(ctx, chatID); cmd.WarnLimit != 12 || cmd.WarnMode != "ban" {
			t.Fatalf("GetWarnSettingContext() = %+v, want what the reader cached", cmd)
		}
	})

	t.Run("a write drops the key so the reader sees the new value", func(t *testing.T) {
		mr := useMiniredisCache(t)
		chatID := known(t)
		if err := SetWarnLimit(chatID, 4); err != nil {
			t.Fatalf("SetWarnLimit() error = %v", err)
		}
		if got, err := LoadWarnSettingsContext(ctx, chatID); err != nil || got.WarnLimit != 4 {
			t.Fatalf("first read = %+v, %v, want limit 4", got, err)
		}
		if err := SetWarnLimit(chatID, 9); err != nil {
			t.Fatalf("SetWarnLimit() second error = %v", err)
		}
		if keys := warnSettingsKeys(mr, chatID); len(keys) != 0 {
			t.Fatalf("cache keys after the write = %v, want the write to have dropped them", keys)
		}
		got, err := LoadWarnSettingsContext(ctx, chatID)
		if err != nil || got.WarnLimit != 9 {
			t.Fatalf("second read = %+v, %v, want limit 9", got, err)
		}
	})

	t.Run("a read error is returned and nothing is cached", func(t *testing.T) {
		mr := useMiniredisCache(t)
		chatID := known(t)
		name := "test:fail_warn_settings_read_" + t.Name()
		err := db.DB.Callback().Query().Before("gorm:query").Register(name, func(tx *gorm.DB) {
			if tx.Statement.Table == "warns_settings" {
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

		got, err := LoadWarnSettingsContext(ctx, chatID)
		if err == nil {
			t.Fatalf("LoadWarnSettingsContext() = %+v, nil error, want the database error", got)
		}
		if got != nil {
			t.Fatalf("LoadWarnSettingsContext() settings = %+v next to an error, want nil", got)
		}
		if keys := warnSettingsKeys(mr, chatID); len(keys) != 0 {
			t.Fatalf("cache keys after a failed read = %v, want none", keys)
		}
	})
}
