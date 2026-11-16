//go:build testtools

package filters

import (
	"context"
	"errors"
	"slices"
	"testing"

	"gorm.io/gorm"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	utilsCache "github.com/uasneppy/fuku_robot/fuku/utils/cache"
)

// entriesChats returns two chat IDs no other test of the package uses, with their rows and cache
// entries deleted before the test and when it ends.
func entriesChats(t *testing.T) (chatA, chatB int64) {
	t.Helper()
	utilsCache.SetupTestMemoryMarshaler(t)
	chatA, chatB = -1009919003001, -1009919003002
	clean := func() {
		db.DB.Where("chat_id IN ?", []int64{chatA, chatB}).Delete(&models.ChatFilters{})
		for _, chatID := range []int64{chatA, chatB} {
			cache.DeleteCache(filterListCacheKey(chatID))
			cache.DeleteCache(optimizedFilterCacheKey(chatID))
		}
		cache.ResetLocalForTest()
	}
	clean()
	t.Cleanup(clean)
	return chatA, chatB
}

// entriesFailReads makes every read of the filters table fail until the test ends.
func entriesFailReads(t *testing.T) {
	t.Helper()
	name := "test:fail_filter_query"
	err := db.DB.Callback().Query().Before("gorm:query").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "filters" {
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
}

func entriesSeed(t *testing.T, chatID int64, keywords ...string) []int64 {
	t.Helper()
	ids := make([]int64, 0, len(keywords))
	for _, keyword := range keywords {
		row := &models.ChatFilters{ChatId: chatID, KeyWord: keyword, FilterReply: "reply", MsgType: db.TEXT}
		if err := db.DB.Create(row).Error; err != nil {
			t.Fatalf("seed %q: %v", keyword, err)
		}
		ids = append(ids, int64(row.ID))
	}
	return ids
}

func entriesChatRowCount(t *testing.T, chatID int64) int64 {
	t.Helper()
	var count int64
	if err := db.DB.Model(&models.ChatFilters{}).Where("chat_id = ?", chatID).Count(&count).Error; err != nil {
		t.Fatalf("count rows: %v", err)
	}
	return count
}

func TestFilterEntriesFresh(t *testing.T) {
	ctx := context.Background()
	chatA, chatB := entriesChats(t)
	idsA := entriesSeed(t, chatA, "spam", "eggs")
	idsB := entriesSeed(t, chatB, "other")

	t.Run("list is chat-scoped and creates nothing", func(t *testing.T) {
		rows, err := ListFilterEntriesFresh(ctx, chatA)
		if err != nil {
			t.Fatalf("ListFilterEntriesFresh() error = %v", err)
		}
		var got []string
		for _, row := range rows {
			got = append(got, row.KeyWord)
		}
		if want := []string{"spam", "eggs"}; !slices.Equal(got, want) {
			t.Errorf("rows = %q, want %q", got, want)
		}
		if rows[0].ID == 0 {
			t.Error("row ID was not read")
		}

		empty, err := ListFilterEntriesFresh(ctx, -1009919003003)
		if err != nil || len(empty) != 0 {
			t.Errorf("empty chat: rows = %v, err = %v, want none and nil", empty, err)
		}
		if got := entriesChatRowCount(t, -1009919003003); got != 0 {
			t.Errorf("a read created %d rows", got)
		}
	})

	t.Run("reads fill no cache entry", func(t *testing.T) {
		// A cached list would still show a row deleted straight from the table.
		if _, err := ListFilterEntriesFresh(ctx, chatB); err != nil {
			t.Fatalf("ListFilterEntriesFresh() error = %v", err)
		}
		if _, err := CountFiltersContext(ctx, chatB); err != nil {
			t.Fatalf("CountFiltersContext() error = %v", err)
		}
		if _, err := FilterExistsContext(ctx, chatB, "other"); err != nil {
			t.Fatalf("FilterExistsContext() error = %v", err)
		}
		if err := db.DB.Where("chat_id = ?", chatB).Delete(&models.ChatFilters{}).Error; err != nil {
			t.Fatalf("delete straight: %v", err)
		}
		if got := GetFiltersList(chatB); len(got) != 0 {
			t.Errorf("filter_list holds %v after the fresh reads", got)
		}
		if got, err := GetChatFiltersCached(chatB); err != nil || len(got) != 0 {
			t.Errorf("filters_optimized holds %d rows (%v) after the fresh reads", len(got), err)
		}
		idsB = entriesSeed(t, chatB, "other")
		cache.DeleteCache(filterListCacheKey(chatB))
		cache.DeleteCache(optimizedFilterCacheKey(chatB))
	})

	t.Run("get by ID is chat-scoped", func(t *testing.T) {
		row, err := GetFilterEntryFresh(ctx, chatA, idsA[0])
		if err != nil || row == nil || row.KeyWord != "spam" || row.FilterReply != "reply" {
			t.Fatalf("own row = %+v, err = %v", row, err)
		}
		foreign, err := GetFilterEntryFresh(ctx, chatA, idsB[0])
		if err != nil || foreign != nil {
			t.Errorf("another chat's row = %+v, err = %v, want nil and nil", foreign, err)
		}
		missing, err := GetFilterEntryFresh(ctx, chatA, 999999999)
		if err != nil || missing != nil {
			t.Errorf("missing ID = %+v, err = %v, want nil and nil", missing, err)
		}
	})

	t.Run("count and existence are chat-scoped", func(t *testing.T) {
		if got, err := CountFiltersContext(ctx, chatA); err != nil || got != 2 {
			t.Errorf("CountFiltersContext() = %d, %v, want 2", got, err)
		}
		if got, err := FilterExistsContext(ctx, chatA, "SPAM"); err != nil || !got {
			t.Errorf("FilterExistsContext(own, upper case) = %v, %v, want true", got, err)
		}
		if got, err := FilterExistsContext(ctx, chatA, "other"); err != nil || got {
			t.Errorf("FilterExistsContext(another chat's keyword) = %v, %v, want false", got, err)
		}
	})

	t.Run("a read error is returned", func(t *testing.T) {
		entriesFailReads(t)
		if _, err := ListFilterEntriesFresh(ctx, chatA); err == nil {
			t.Error("ListFilterEntriesFresh() hid a read error")
		}
		if _, err := GetFilterEntryFresh(ctx, chatA, idsA[0]); err == nil {
			t.Error("GetFilterEntryFresh() hid a read error")
		}
		if _, err := CountFiltersContext(ctx, chatA); err == nil {
			t.Error("CountFiltersContext() hid a read error")
		}
		if _, err := FilterExistsContext(ctx, chatA, "spam"); err == nil {
			t.Error("FilterExistsContext() hid a read error")
		}
	})
}

func TestRemoveFilterByID(t *testing.T) {
	ctx := context.Background()
	chatA, chatB := entriesChats(t)
	idsA := entriesSeed(t, chatA, "spam", "eggs")
	idsB := entriesSeed(t, chatB, "other")

	// Fill both chats' cache entries.
	if got := GetFiltersList(chatA); len(got) != 2 {
		t.Fatalf("seeded chat A reads %d keywords, want 2", len(got))
	}
	if got := GetFiltersList(chatB); len(got) != 1 {
		t.Fatalf("seeded chat B reads %d keywords, want 1", len(got))
	}
	if got, err := GetChatFiltersCached(chatA); err != nil || len(got) != 2 {
		t.Fatalf("seeded chat A optimized cache = %d, %v", len(got), err)
	}

	t.Run("another chat's ID deletes nothing and keeps its cache", func(t *testing.T) {
		// Delete chat B's row straight from the table: only a stale cache entry still lists it.
		if err := db.DB.Where("chat_id = ?", chatB).Delete(&models.ChatFilters{}).Error; err != nil {
			t.Fatalf("delete straight: %v", err)
		}
		keyword, removed, err := RemoveFilterByID(ctx, chatA, idsB[0])
		if err != nil || removed || keyword != "" {
			t.Errorf("RemoveFilterByID() = %q, %v, %v, want no removal", keyword, removed, err)
		}
		if got := GetFiltersList(chatB); len(got) != 1 {
			t.Errorf("chat B's cache was dropped by a foreign ID (reads %d keywords)", len(got))
		}
		if got := entriesChatRowCount(t, chatA); got != 2 {
			t.Errorf("chat A has %d rows, want 2", got)
		}
	})

	t.Run("own ID deletes the row, returns its keyword and drops both caches", func(t *testing.T) {
		keyword, removed, err := RemoveFilterByID(ctx, chatA, idsA[0])
		if err != nil || !removed || keyword != "spam" {
			t.Fatalf("RemoveFilterByID() = %q, %v, %v, want spam removed", keyword, removed, err)
		}
		if got := GetFiltersList(chatA); !slices.Equal(got, []string{"eggs"}) {
			t.Errorf("chat A filter_list reads %q after the delete, want [eggs]", got)
		}
		if got, err := GetChatFiltersCached(chatA); err != nil || len(got) != 1 {
			t.Errorf("chat A filters_optimized reads %d rows (%v) after the delete, want 1", len(got), err)
		}
	})

	t.Run("a second delete of the same ID removes nothing", func(t *testing.T) {
		keyword, removed, err := RemoveFilterByID(ctx, chatA, idsA[0])
		if err != nil || removed || keyword != "" {
			t.Errorf("RemoveFilterByID() = %q, %v, %v, want no removal", keyword, removed, err)
		}
	})

	t.Run("a delete error is returned", func(t *testing.T) {
		name := "test:fail_filter_delete"
		err := db.DB.Callback().Delete().Before("gorm:delete").Register(name, func(tx *gorm.DB) {
			if tx.Statement.Table == "filters" {
				_ = tx.AddError(errors.New("database is down"))
			}
		})
		if err != nil {
			t.Fatalf("register callback: %v", err)
		}
		t.Cleanup(func() {
			if err := db.DB.Callback().Delete().Remove(name); err != nil {
				t.Errorf("remove callback: %v", err)
			}
		})
		_, removed, delErr := RemoveFilterByID(ctx, chatA, idsA[1])
		if delErr == nil || removed {
			t.Errorf("RemoveFilterByID() = removed %v, err %v, want the error", removed, delErr)
		}
	})
}
