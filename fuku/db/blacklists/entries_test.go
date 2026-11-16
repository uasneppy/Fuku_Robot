//go:build testtools

package blacklists

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

// entriesChats returns two chat IDs no other test of the package uses, with their rows and
// cache entries deleted before the test and when it ends.
func entriesChats(t *testing.T) (chatA, chatB int64) {
	t.Helper()
	utilsCache.SetupTestMemoryMarshaler(t)
	chatA, chatB = -1009919002001, -1009919002002
	clean := func() {
		db.DB.Where("chat_id IN ?", []int64{chatA, chatB}).Delete(&models.BlacklistSettings{})
		cache.DeleteCache(cache.CacheKey("blacklist", chatA))
		cache.DeleteCache(cache.CacheKey("blacklist", chatB))
		cache.ResetLocalForTest()
	}
	clean()
	t.Cleanup(clean)
	return chatA, chatB
}

// entriesFailReads makes every read of the blacklists table fail until the test ends.
func entriesFailReads(t *testing.T) {
	t.Helper()
	name := "test:fail_blacklist_query"
	err := db.DB.Callback().Query().Before("gorm:query").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "blacklists" {
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

func entriesSeed(t *testing.T, chatID int64, action string, words ...string) []int64 {
	t.Helper()
	ids := make([]int64, 0, len(words))
	for _, word := range words {
		row := &models.BlacklistSettings{ChatId: chatID, Word: word, Action: action, Reason: "Blacklisted word: '%s'"}
		if err := db.DB.Create(row).Error; err != nil {
			t.Fatalf("seed %q: %v", word, err)
		}
		ids = append(ids, int64(row.ID))
	}
	return ids
}

func entriesChatRowCount(t *testing.T, chatID int64) int64 {
	t.Helper()
	var count int64
	if err := db.DB.Model(&models.BlacklistSettings{}).Where("chat_id = ?", chatID).Count(&count).Error; err != nil {
		t.Fatalf("count rows: %v", err)
	}
	return count
}

func TestBlacklistEntriesFresh(t *testing.T) {
	ctx := context.Background()
	chatA, chatB := entriesChats(t)
	idsA := entriesSeed(t, chatA, "ban", "spam", "eggs")
	idsB := entriesSeed(t, chatB, "warn", "other")

	t.Run("list is chat-scoped and creates nothing", func(t *testing.T) {
		rows, err := ListBlacklistEntriesFresh(ctx, chatA)
		if err != nil {
			t.Fatalf("ListBlacklistEntriesFresh() error = %v", err)
		}
		var got []string
		for _, row := range rows {
			got = append(got, row.Word+"="+row.Action)
		}
		if want := []string{"spam=ban", "eggs=ban"}; !slices.Equal(got, want) {
			t.Errorf("rows = %q, want %q", got, want)
		}
		if rows[0].ID == 0 {
			t.Error("row ID was not read")
		}

		empty, err := ListBlacklistEntriesFresh(ctx, -1009919002003)
		if err != nil || len(empty) != 0 {
			t.Errorf("empty chat: rows = %v, err = %v, want none and nil", empty, err)
		}
		if got := entriesChatRowCount(t, -1009919002003); got != 0 {
			t.Errorf("a read created %d rows", got)
		}
	})

	t.Run("reads fill no cache entry", func(t *testing.T) {
		// A cached list would still show a row deleted straight from the table.
		if _, err := ListBlacklistEntriesFresh(ctx, chatB); err != nil {
			t.Fatalf("ListBlacklistEntriesFresh() error = %v", err)
		}
		if _, err := CountBlacklistFresh(ctx, chatB); err != nil {
			t.Fatalf("CountBlacklistFresh() error = %v", err)
		}
		if err := db.DB.Where("chat_id = ?", chatB).Delete(&models.BlacklistSettings{}).Error; err != nil {
			t.Fatalf("delete straight: %v", err)
		}
		if got := GetBlacklistSettings(chatB); len(got) != 0 {
			t.Errorf("the cached read returned %d rows after the fresh reads", len(got))
		}
		idsB = entriesSeed(t, chatB, "warn", "other")
		cache.DeleteCache(cache.CacheKey("blacklist", chatB))
	})

	t.Run("get by ID is chat-scoped", func(t *testing.T) {
		row, err := GetBlacklistEntryFresh(ctx, chatA, idsA[0])
		if err != nil || row == nil || row.Word != "spam" || row.Action != "ban" {
			t.Fatalf("own row = %+v, err = %v", row, err)
		}
		foreign, err := GetBlacklistEntryFresh(ctx, chatA, idsB[0])
		if err != nil || foreign != nil {
			t.Errorf("another chat's row = %+v, err = %v, want nil and nil", foreign, err)
		}
		missing, err := GetBlacklistEntryFresh(ctx, chatA, 999999999)
		if err != nil || missing != nil {
			t.Errorf("missing ID = %+v, err = %v, want nil and nil", missing, err)
		}
	})

	t.Run("count is chat-scoped", func(t *testing.T) {
		got, err := CountBlacklistFresh(ctx, chatA)
		if err != nil || got != 2 {
			t.Errorf("CountBlacklistFresh() = %d, %v, want 2", got, err)
		}
	})

	t.Run("a read error is returned", func(t *testing.T) {
		entriesFailReads(t)
		if _, err := ListBlacklistEntriesFresh(ctx, chatA); err == nil {
			t.Error("ListBlacklistEntriesFresh() hid a read error")
		}
		if _, err := GetBlacklistEntryFresh(ctx, chatA, idsA[0]); err == nil {
			t.Error("GetBlacklistEntryFresh() hid a read error")
		}
		if _, err := CountBlacklistFresh(ctx, chatA); err == nil {
			t.Error("CountBlacklistFresh() hid a read error")
		}
	})
}

func TestRemoveBlacklistByID(t *testing.T) {
	ctx := context.Background()
	chatA, chatB := entriesChats(t)
	idsA := entriesSeed(t, chatA, "warn", "spam", "eggs")
	idsB := entriesSeed(t, chatB, "warn", "other")

	// Fill both chats' cache entries.
	if got := GetBlacklistSettings(chatA); len(got) != 2 {
		t.Fatalf("seeded chat A reads %d words, want 2", len(got))
	}
	if got := GetBlacklistSettings(chatB); len(got) != 1 {
		t.Fatalf("seeded chat B reads %d words, want 1", len(got))
	}

	t.Run("another chat's ID deletes nothing and keeps its cache", func(t *testing.T) {
		// Delete chat B's row straight from the table: only a stale cache entry still lists it.
		if err := db.DB.Where("chat_id = ?", chatB).Delete(&models.BlacklistSettings{}).Error; err != nil {
			t.Fatalf("delete straight: %v", err)
		}
		word, removed, err := RemoveBlacklistByID(ctx, chatA, idsB[0])
		if err != nil || removed || word != "" {
			t.Errorf("RemoveBlacklistByID() = %q, %v, %v, want no removal", word, removed, err)
		}
		if got := GetBlacklistSettings(chatB); len(got) != 1 {
			t.Errorf("chat B's cache was dropped by a foreign ID (reads %d words)", len(got))
		}
		if got := entriesChatRowCount(t, chatA); got != 2 {
			t.Errorf("chat A has %d rows, want 2", got)
		}
	})

	t.Run("own ID deletes the row and drops the cache", func(t *testing.T) {
		word, removed, err := RemoveBlacklistByID(ctx, chatA, idsA[0])
		if err != nil || !removed || word != "spam" {
			t.Fatalf("RemoveBlacklistByID() = %q, %v, %v, want spam removed", word, removed, err)
		}
		if got := GetBlacklistSettings(chatA).Triggers(); !slices.Equal(got, []string{"eggs"}) {
			t.Errorf("chat A reads %q after the delete, want [eggs]", got)
		}
		if got := entriesChatRowCount(t, chatA); got != 1 {
			t.Errorf("chat A has %d rows, want 1", got)
		}
	})

	t.Run("a second delete of the same ID removes nothing", func(t *testing.T) {
		word, removed, err := RemoveBlacklistByID(ctx, chatA, idsA[0])
		if err != nil || removed || word != "" {
			t.Errorf("RemoveBlacklistByID() = %q, %v, %v, want no removal", word, removed, err)
		}
	})

	t.Run("a delete error is returned", func(t *testing.T) {
		name := "test:fail_blacklist_delete"
		err := db.DB.Callback().Delete().Before("gorm:delete").Register(name, func(tx *gorm.DB) {
			if tx.Statement.Table == "blacklists" {
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
		_, removed, delErr := RemoveBlacklistByID(ctx, chatA, idsA[1])
		if delErr == nil || removed {
			t.Errorf("RemoveBlacklistByID() = removed %v, err %v, want the error", removed, delErr)
		}
	})
}
