//go:build testtools

package notes

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

// entriesChats returns two chat IDs no other test of the package uses, with their rows, settings rows
// and cache entries deleted before the test and when it ends.
func entriesChats(t *testing.T) (chatA, chatB int64) {
	t.Helper()
	utilsCache.SetupTestMemoryMarshaler(t)
	chatA, chatB = -1009919004001, -1009919004002
	clean := func() {
		db.DB.Where("chat_id IN ?", []int64{chatA, chatB}).Delete(&models.Notes{})
		db.DB.Where("chat_id IN ?", []int64{chatA, chatB}).Delete(&models.NotesSettings{})
		for _, chatID := range []int64{chatA, chatB} {
			cache.DeleteCache(notesListCacheKey(chatID))
			cache.DeleteCache(cache.CacheKey("notes_settings", chatID))
		}
		cache.ResetLocalForTest()
	}
	clean()
	t.Cleanup(clean)
	return chatA, chatB
}

// entriesFailReads makes every read of the notes table fail until the test ends.
func entriesFailReads(t *testing.T) {
	t.Helper()
	name := "test:fail_note_query"
	err := db.DB.Callback().Query().Before("gorm:query").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "notes" {
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

func entriesSeed(t *testing.T, chatID int64, names ...string) []int64 {
	t.Helper()
	ids := make([]int64, 0, len(names))
	for _, name := range names {
		row := &models.Notes{ChatId: chatID, NoteName: name, NoteContent: "body", MsgType: db.TEXT, AdminOnly: name == "secret"}
		if err := db.DB.Create(row).Error; err != nil {
			t.Fatalf("seed %q: %v", name, err)
		}
		ids = append(ids, int64(row.ID))
	}
	return ids
}

func entriesRowCount(t *testing.T, model any, chatID int64) int64 {
	t.Helper()
	var count int64
	if err := db.DB.Model(model).Where("chat_id = ?", chatID).Count(&count).Error; err != nil {
		t.Fatalf("count rows: %v", err)
	}
	return count
}

func TestNoteEntriesFresh(t *testing.T) {
	ctx := context.Background()
	chatA, chatB := entriesChats(t)
	idsA := entriesSeed(t, chatA, "rules", "secret")
	idsB := entriesSeed(t, chatB, "other")

	t.Run("list is chat-scoped and creates nothing", func(t *testing.T) {
		rows, err := ListNoteEntriesFresh(ctx, chatA)
		if err != nil {
			t.Fatalf("ListNoteEntriesFresh() error = %v", err)
		}
		var got []string
		for _, row := range rows {
			got = append(got, row.NoteName)
		}
		if want := []string{"rules", "secret"}; !slices.Equal(got, want) {
			t.Errorf("rows = %q, want %q", got, want)
		}
		if rows[0].ID == 0 || rows[0].AdminOnly || !rows[1].AdminOnly {
			t.Errorf("ID or admin-only flag was not read: %+v", rows)
		}

		const emptyChat = -1009919004003
		empty, err := ListNoteEntriesFresh(ctx, emptyChat)
		if err != nil || len(empty) != 0 {
			t.Errorf("empty chat: rows = %v, err = %v, want none and nil", empty, err)
		}
		if got := entriesRowCount(t, &models.Notes{}, emptyChat) + entriesRowCount(t, &models.NotesSettings{}, emptyChat); got != 0 {
			t.Errorf("a read created %d rows", got)
		}
		if got := entriesRowCount(t, &models.NotesSettings{}, chatA); got != 0 {
			t.Errorf("a read created %d notes_settings rows", got)
		}
	})

	t.Run("reads fill no cache entry", func(t *testing.T) {
		// A cached list would still show a row deleted straight from the table.
		if _, err := ListNoteEntriesFresh(ctx, chatB); err != nil {
			t.Fatalf("ListNoteEntriesFresh() error = %v", err)
		}
		if _, err := CountNotesFresh(ctx, chatB); err != nil {
			t.Fatalf("CountNotesFresh() error = %v", err)
		}
		if _, err := NoteExistsContext(ctx, chatB, "other"); err != nil {
			t.Fatalf("NoteExistsContext() error = %v", err)
		}
		if err := db.DB.Where("chat_id = ?", chatB).Delete(&models.Notes{}).Error; err != nil {
			t.Fatalf("delete straight: %v", err)
		}
		if got := GetNotesList(chatB, true); len(got) != 0 {
			t.Errorf("notes_list holds %v after the fresh reads", got)
		}
		idsB = entriesSeed(t, chatB, "other")
		cache.DeleteCache(notesListCacheKey(chatB))
	})

	t.Run("get by ID is chat-scoped", func(t *testing.T) {
		row, err := GetNoteEntryFresh(ctx, chatA, idsA[0])
		if err != nil || row == nil || row.NoteName != "rules" || row.NoteContent != "body" {
			t.Fatalf("own row = %+v, err = %v", row, err)
		}
		foreign, err := GetNoteEntryFresh(ctx, chatA, idsB[0])
		if err != nil || foreign != nil {
			t.Errorf("another chat's row = %+v, err = %v, want nil and nil", foreign, err)
		}
		missing, err := GetNoteEntryFresh(ctx, chatA, 999999999)
		if err != nil || missing != nil {
			t.Errorf("missing ID = %+v, err = %v, want nil and nil", missing, err)
		}
	})

	t.Run("count and existence are chat-scoped", func(t *testing.T) {
		if got, err := CountNotesFresh(ctx, chatA); err != nil || got != 2 {
			t.Errorf("CountNotesFresh() = %d, %v, want 2", got, err)
		}
		if got, err := NoteExistsContext(ctx, chatA, "rules"); err != nil || !got {
			t.Errorf("NoteExistsContext(own) = %v, %v, want true", got, err)
		}
		if got, err := NoteExistsContext(ctx, chatA, "other"); err != nil || got {
			t.Errorf("NoteExistsContext(another chat's name) = %v, %v, want false", got, err)
		}
	})

	t.Run("a read error is returned", func(t *testing.T) {
		entriesFailReads(t)
		if _, err := ListNoteEntriesFresh(ctx, chatA); err == nil {
			t.Error("ListNoteEntriesFresh() hid a read error")
		}
		if _, err := GetNoteEntryFresh(ctx, chatA, idsA[0]); err == nil {
			t.Error("GetNoteEntryFresh() hid a read error")
		}
		if _, err := CountNotesFresh(ctx, chatA); err == nil {
			t.Error("CountNotesFresh() hid a read error")
		}
		if _, err := NoteExistsContext(ctx, chatA, "rules"); err == nil {
			t.Error("NoteExistsContext() hid a read error")
		}
	})
}

func TestRemoveNoteByID(t *testing.T) {
	ctx := context.Background()
	chatA, chatB := entriesChats(t)
	idsA := entriesSeed(t, chatA, "rules", "faq")
	idsB := entriesSeed(t, chatB, "other")

	// Fill both chats' cache entries.
	if got := GetNotesList(chatA, true); len(got) != 2 {
		t.Fatalf("seeded chat A reads %d names, want 2", len(got))
	}
	if got := GetNotesList(chatB, true); len(got) != 1 {
		t.Fatalf("seeded chat B reads %d names, want 1", len(got))
	}

	t.Run("another chat's ID deletes nothing and keeps its cache", func(t *testing.T) {
		// Delete chat B's row straight from the table: only a stale cache entry still lists it.
		if err := db.DB.Where("chat_id = ?", chatB).Delete(&models.Notes{}).Error; err != nil {
			t.Fatalf("delete straight: %v", err)
		}
		name, removed, err := RemoveNoteByID(ctx, chatA, idsB[0])
		if err != nil || removed || name != "" {
			t.Errorf("RemoveNoteByID() = %q, %v, %v, want no removal", name, removed, err)
		}
		if got := GetNotesList(chatB, true); len(got) != 1 {
			t.Errorf("chat B's cache was dropped by a foreign ID (reads %d names)", len(got))
		}
		if got := entriesRowCount(t, &models.Notes{}, chatA); got != 2 {
			t.Errorf("chat A has %d rows, want 2", got)
		}
	})

	t.Run("own ID deletes the row, returns its name and drops notes_list", func(t *testing.T) {
		name, removed, err := RemoveNoteByID(ctx, chatA, idsA[0])
		if err != nil || !removed || name != "rules" {
			t.Fatalf("RemoveNoteByID() = %q, %v, %v, want rules removed", name, removed, err)
		}
		if got := GetNotesList(chatA, true); !slices.Equal(got, []string{"faq"}) {
			t.Errorf("chat A notes_list reads %q after the delete, want [faq]", got)
		}
		if got := entriesRowCount(t, &models.NotesSettings{}, chatA); got != 0 {
			t.Errorf("the delete created %d notes_settings rows", got)
		}
	})

	t.Run("a second delete of the same ID removes nothing", func(t *testing.T) {
		name, removed, err := RemoveNoteByID(ctx, chatA, idsA[0])
		if err != nil || removed || name != "" {
			t.Errorf("RemoveNoteByID() = %q, %v, %v, want no removal", name, removed, err)
		}
	})

	t.Run("a delete error is returned", func(t *testing.T) {
		name := "test:fail_note_delete"
		err := db.DB.Callback().Delete().Before("gorm:delete").Register(name, func(tx *gorm.DB) {
			if tx.Statement.Table == "notes" {
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
		_, removed, delErr := RemoveNoteByID(ctx, chatA, idsA[1])
		if delErr == nil || removed {
			t.Errorf("RemoveNoteByID() = removed %v, err %v, want the error", removed, delErr)
		}
	})
}
