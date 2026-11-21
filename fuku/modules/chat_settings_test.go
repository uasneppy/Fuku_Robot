//go:build testtools

package modules

import (
	"context"
	"errors"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/chatconv"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// withChatRedis points the conversation feature at a fresh miniredis for one test.
func withChatRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	previous := chatRedisClient
	chatRedisClient = func() *redis.Client { return client }
	t.Cleanup(func() { chatRedisClient = previous })
	return mr, client
}

func chatConvCleanup(t *testing.T, chatID int64) {
	t.Helper()
	t.Cleanup(func() {
		db.DB.Where("chat_id = ?", chatID).Delete(&models.ChatConversation{})
		cache.DeleteCache(chatconv.CacheKey(chatID))
	})
}

func TestApplyChatSettings(t *testing.T) {
	ctx := context.Background()

	t.Run("on, provider, audience and off write and read back", func(t *testing.T) {
		chatID := uniqueModuleChatID()
		chatConvCleanup(t, chatID)
		mr, _ := withChatRedis(t)

		if r := applyChatEnabled(ctx, chatID, true, 5); r.Outcome != chatApplied || r.Settings == nil || !r.Settings.Enabled {
			t.Fatalf("on: %+v", r)
		}
		if r := applyChatProvider(ctx, chatID, chatconv.ProviderGoogle, 5); r.Outcome != chatApplied || r.Settings.Provider != chatconv.ProviderGoogle || !r.Settings.Enabled {
			t.Fatalf("provider: %+v", r)
		}
		if r := applyChatAudience(ctx, chatID, chatconv.AudienceAdminsApproved, 5); r.Outcome != chatApplied || r.Settings.Audience != chatconv.AudienceAdminsApproved {
			t.Fatalf("audience: %+v", r)
		}

		_ = mr.Set(chatHistoryKey(chatID), "transcript")
		_ = mr.Set(chatMineKey(chatID), "ids")
		other := chatID - 1
		_ = mr.Set(chatHistoryKey(other), "another chat")

		r := applyChatEnabled(ctx, chatID, false, 5)
		if r.Outcome != chatApplied || r.Settings.Enabled || r.Settings.Provider != chatconv.ProviderGoogle {
			t.Fatalf("off: %+v", r)
		}
		if mr.Exists(chatHistoryKey(chatID)) || mr.Exists(chatMineKey(chatID)) {
			t.Error("off left the transcript or the ID set behind")
		}
		if !mr.Exists(chatHistoryKey(other)) {
			t.Error("off deleted another chat's transcript")
		}
		if got, _ := chatconv.Get(ctx, chatID); got.Enabled {
			t.Error("the cached setting still reads on")
		}
	})

	t.Run("an invalid value writes nothing", func(t *testing.T) {
		chatID := uniqueModuleChatID()
		chatConvCleanup(t, chatID)
		if r := applyChatProvider(ctx, chatID, "openai", 5); r.Outcome != chatInvalidValue {
			t.Errorf("provider: %+v", r)
		}
		if r := applyChatAudience(ctx, chatID, 7, 5); r.Outcome != chatInvalidValue {
			t.Errorf("audience: %+v", r)
		}
		var n int64
		db.DB.Model(&models.ChatConversation{}).Where("chat_id = ?", chatID).Count(&n)
		if n != 0 {
			t.Fatal("an invalid value wrote a row")
		}
	})

	t.Run("a failed write keeps the transcript and reports the failure", func(t *testing.T) {
		chatID := uniqueModuleChatID()
		chatConvCleanup(t, chatID)
		mr, _ := withChatRedis(t)
		_ = mr.Set(chatHistoryKey(chatID), "transcript")
		previous := chatConvWrites.Enabled
		chatConvWrites.Enabled = func(context.Context, int64, bool, int64) error { return errors.New("db down") }
		t.Cleanup(func() { chatConvWrites.Enabled = previous })

		if r := applyChatEnabled(ctx, chatID, false, 5); r.Outcome != chatSaveFailed || r.Err == nil {
			t.Fatalf("result %+v", r)
		}
		if !mr.Exists(chatHistoryKey(chatID)) {
			t.Error("a failed write wiped the transcript")
		}
	})

	t.Run("without Redis an off write still lands", func(t *testing.T) {
		chatID := uniqueModuleChatID()
		chatConvCleanup(t, chatID)
		previous := chatRedisClient
		chatRedisClient = func() *redis.Client { return nil }
		t.Cleanup(func() { chatRedisClient = previous })
		if r := applyChatEnabled(ctx, chatID, false, 5); r.Outcome != chatApplied || r.Settings == nil || r.Settings.Enabled {
			t.Fatalf("result %+v", r)
		}
	})
}
