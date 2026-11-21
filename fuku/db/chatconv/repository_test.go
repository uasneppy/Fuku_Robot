//go:build testtools

package chatconv

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

func newChat(t *testing.T) int64 {
	t.Helper()
	chatID := -time.Now().UnixNano()
	t.Cleanup(func() {
		db.DB.Where("chat_id = ?", chatID).Delete(&models.ChatConversation{})
		cache.DeleteCache(CacheKey(chatID))
	})
	return chatID
}

func rowCount(t *testing.T, chatID int64) int64 {
	t.Helper()
	var n int64
	if err := db.DB.Model(&models.ChatConversation{}).Where("chat_id = ?", chatID).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

func TestChatConversationSettings(t *testing.T) {
	ctx := context.Background()

	t.Run("a chat with no row reads the defaults and creates nothing", func(t *testing.T) {
		chatID := newChat(t)
		got, err := Get(ctx, chatID)
		if err != nil || got != Defaults() {
			t.Fatalf("Get = %+v, %v", got, err)
		}
		if rowCount(t, chatID) != 0 {
			t.Fatal("a read created a row")
		}
	})

	t.Run("each write changes its own column and drops the key", func(t *testing.T) {
		chatID := newChat(t)
		if _, err := Get(ctx, chatID); err != nil { // fill the cache
			t.Fatal(err)
		}
		if err := SetProvider(ctx, chatID, ProviderGoogle, 7); err != nil {
			t.Fatal(err)
		}
		if got, _ := Get(ctx, chatID); got.Provider != ProviderGoogle || got.Enabled {
			t.Fatalf("after SetProvider: %+v", got)
		}
		if err := SetEnabled(ctx, chatID, true, 7); err != nil {
			t.Fatal(err)
		}
		if err := SetAudience(ctx, chatID, AudienceAdminsApproved, 8); err != nil {
			t.Fatal(err)
		}
		got, err := Get(ctx, chatID)
		want := Settings{Enabled: true, Provider: ProviderGoogle, Audience: AudienceAdminsApproved}
		if err != nil || got != want {
			t.Fatalf("Get = %+v, %v, want %+v", got, err, want)
		}
		if fresh, _ := GetFresh(ctx, chatID); fresh != want {
			t.Fatalf("GetFresh = %+v", fresh)
		}

		// A Go false is written.
		if err := SetEnabled(ctx, chatID, false, 9); err != nil {
			t.Fatal(err)
		}
		if got, _ := Get(ctx, chatID); got.Enabled || got.Provider != ProviderGoogle || got.Audience != AudienceAdminsApproved {
			t.Fatalf("after off: %+v", got)
		}
		var row models.ChatConversation
		db.DB.Where("chat_id = ?", chatID).Take(&row)
		if row.UpdatedBy != 9 {
			t.Errorf("updated_by = %d", row.UpdatedBy)
		}
	})

	t.Run("an invalid value writes nothing", func(t *testing.T) {
		chatID := newChat(t)
		if err := SetProvider(ctx, chatID, "openai", 1); !errors.Is(err, ErrInvalidValue) {
			t.Errorf("provider: %v", err)
		}
		if err := SetAudience(ctx, chatID, 2, 1); !errors.Is(err, ErrInvalidValue) {
			t.Errorf("audience: %v", err)
		}
		if rowCount(t, chatID) != 0 {
			t.Fatal("an invalid value wrote a row")
		}
	})

	t.Run("the table's checks refuse values outside the stored ones", func(t *testing.T) {
		chatID := newChat(t)
		bad := &models.ChatConversation{ChatID: chatID, Provider: "openai"}
		if err := db.DB.Create(bad).Error; err == nil {
			t.Error("a provider outside the check was stored")
		}
		bad = &models.ChatConversation{ChatID: chatID, Provider: ProviderAnthropic, Audience: 2}
		if err := db.DB.Create(bad).Error; err == nil {
			t.Error("an audience outside the check was stored")
		}
	})

}
