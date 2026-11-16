package logchannels

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	gocache "github.com/eko/gocache/lib/v4/cache"
	"github.com/eko/gocache/lib/v4/marshaler"
	gocache_store "github.com/eko/gocache/store/redis/v4"
	"github.com/redis/go-redis/v9"

	"github.com/uasneppy/fuku_robot/fuku/config"
	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/cache"
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

// logChannelKeys lists the cache keys of the chat's log channel row.
func logChannelKeys(mr *miniredis.Miniredis, chatID int64) []string {
	var keys []string
	for _, key := range mr.Keys() {
		if strings.Contains(key, cachePrefix) && strings.Contains(key, strconv.FormatInt(chatID, 10)) {
			keys = append(keys, key)
		}
	}
	return keys
}

// TestLoadLogChannelContext proves the page's reader: a chat with no linked channel is (nil, nil)
// and creates and caches nothing, a linked chat reads back through the commands' own key, a
// write is seen at once by both readers and a read error is returned, not hidden.
func TestLoadLogChannelContext(t *testing.T) {
	if db.DB == nil {
		t.Fatal("test database was not initialized")
	}
	ctx := context.Background()

	fresh := func(t *testing.T) (chatID, channelID int64) {
		t.Helper()
		chatID = -time.Now().UnixNano()
		channelID = chatID - 1
		t.Cleanup(func() {
			db.DB.Where("chat_id = ?", chatID).Delete(&models.LogChannel{})
			db.DB.Where("chat_id = ?", chatID).Delete(&models.Chat{})
			cache.DeleteCache(cache.CacheKey(cachePrefix, chatID))
		})
		return chatID, channelID
	}
	countRows := func(t *testing.T, chatID int64) int64 {
		t.Helper()
		var n int64
		if err := db.DB.Model(&models.LogChannel{}).Where("chat_id = ?", chatID).Count(&n).Error; err != nil {
			t.Fatalf("count log_channels rows: %v", err)
		}
		return n
	}

	t.Run("no row is nil with no error and creates and caches nothing", func(t *testing.T) {
		mr := useMiniredisCache(t)
		chatID, _ := fresh(t)

		got, err := LoadContext(ctx, chatID)
		if err != nil || got != nil {
			t.Fatalf("LoadContext() = %+v, %v; want nil, nil", got, err)
		}
		if n := countRows(t, chatID); n != 0 {
			t.Fatalf("log_channels rows after the read = %d, want none", n)
		}
		if keys := logChannelKeys(mr, chatID); len(keys) != 0 {
			t.Fatalf("cache keys after the read = %v, want none", keys)
		}
		if Get(chatID) != nil {
			t.Fatal("Get() after the read found a channel for a chat with none")
		}
		if keys := logChannelKeys(mr, chatID); len(keys) != 0 {
			t.Fatalf("cache keys after Get() = %v, want none: the commands' loader caches nothing for a miss either", keys)
		}
	})

	t.Run("a linked chat reads back under the commands' key", func(t *testing.T) {
		mr := useMiniredisCache(t)
		chatID, channelID := fresh(t)
		if err := Set(chatID, "load-log-channel", channelID); err != nil {
			t.Fatalf("Set() error = %v", err)
		}

		got, err := LoadContext(ctx, chatID)
		if err != nil || got == nil {
			t.Fatalf("LoadContext() = %+v, %v; want the row", got, err)
		}
		if got.LogChannelID != channelID || !got.CatSettings || !got.CatAdmin || !got.CatOther {
			t.Fatalf("LoadContext() = %+v, want the channel with every category on", got)
		}
		if keys := logChannelKeys(mr, chatID); len(keys) != 1 {
			t.Fatalf("cache keys after the read = %v, want exactly the commands' key", keys)
		}
		if cmd := Get(chatID); cmd == nil || cmd.LogChannelID != channelID {
			t.Fatalf("Get() = %+v, want the same row", cmd)
		}
	})

	t.Run("a write is seen at once by both readers", func(t *testing.T) {
		useMiniredisCache(t)
		chatID, channelID := fresh(t)
		if err := Set(chatID, "load-log-channel", channelID); err != nil {
			t.Fatalf("Set() error = %v", err)
		}
		if got, err := LoadContext(ctx, chatID); err != nil || got == nil || !got.CatAdmin {
			t.Fatalf("before: %+v, %v; want admin on", got, err)
		}
		if err := SetCategory(chatID, CategoryAdmin, false); err != nil {
			t.Fatalf("SetCategory() error = %v", err)
		}
		if got, err := LoadContext(ctx, chatID); err != nil || got == nil || got.CatAdmin || !got.CatUser {
			t.Fatalf("after SetCategory: %+v, %v; want admin off and user on", got, err)
		}
		if cmd := Get(chatID); cmd == nil || cmd.CatAdmin {
			t.Fatalf("Get() after SetCategory = %+v, want admin off", cmd)
		}
		if err := Unset(chatID); err != nil {
			t.Fatalf("Unset() error = %v", err)
		}
		if got, err := LoadContext(ctx, chatID); err != nil || got != nil {
			t.Fatalf("after Unset: %+v, %v; want nil, nil", got, err)
		}
	})

	t.Run("a read error is returned, not nil", func(t *testing.T) {
		useMiniredisCache(t)
		chatID, channelID := fresh(t)
		if err := Set(chatID, "load-log-channel", channelID); err != nil {
			t.Fatalf("Set() error = %v", err)
		}
		cache.DeleteCache(cache.CacheKey(cachePrefix, chatID))
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		got, err := LoadContext(cancelled, chatID)
		if err == nil {
			t.Fatalf("LoadContext() with a cancelled context = %+v, nil; want an error", got)
		}
		if got != nil {
			t.Fatalf("row on error = %+v, want nil", got)
		}
	})
}
