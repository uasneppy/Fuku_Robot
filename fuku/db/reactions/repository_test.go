package reactions

import (
	"context"
	"testing"
	"time"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/cache"
)

func skipIfNoDb(t *testing.T) {
	t.Helper()
	if db.DB == nil {
		t.Fatal("test database was not initialized")
	}
}

func TestReactionsRoundTrip(t *testing.T) {
	skipIfNoDb(t)

	chatID := time.Now().UnixNano()
	t.Cleanup(func() {
		_ = ResetReactions(chatID)
		cache.DeleteCache(cache.CacheKey("reactions", chatID))
	})

	if got := GetReactions(chatID); len(got) != 0 {
		t.Fatalf("GetReactions on fresh chat = %v, want empty", got)
	}

	if err := AddReaction(chatID, "hello", "👋"); err != nil {
		t.Fatalf("AddReaction() error = %v", err)
	}
	if err := AddReaction(chatID, "bye", "🚀"); err != nil {
		t.Fatalf("AddReaction(second) error = %v", err)
	}
	if got := GetReactions(chatID); got["hello"] != "👋" || got["bye"] != "🚀" {
		t.Fatalf("GetReactions = %v, want hello=👋 bye=🚀", got)
	}

	// Upsert: re-adding the same keyword updates the emoji, not a duplicate.
	if err := AddReaction(chatID, "hello", "😀"); err != nil {
		t.Fatalf("AddReaction(upsert) error = %v", err)
	}
	if got := GetReactions(chatID); got["hello"] != "😀" || len(got) != 2 {
		t.Fatalf("GetReactions after upsert = %v, want hello=😀 and 2 entries", got)
	}

	if err := RemoveReaction(chatID, "bye"); err != nil {
		t.Fatalf("RemoveReaction() error = %v", err)
	}
	got := GetReactions(chatID)
	if _, ok := got["bye"]; ok || len(got) != 1 {
		t.Fatalf("GetReactions after remove = %v, want only hello", got)
	}

	if err := ResetReactions(chatID); err != nil {
		t.Fatalf("ResetReactions() error = %v", err)
	}
	if got := GetReactions(chatID); len(got) != 0 {
		t.Fatalf("GetReactions after reset = %v, want empty", got)
	}
}

func TestReactionFreshReadersAndRemoveByID(t *testing.T) {
	skipIfNoDb(t)
	ctx := context.Background()
	chatID, other := -time.Now().UnixNano(), -time.Now().UnixNano()-1
	t.Cleanup(func() {
		_ = ResetReactions(chatID)
		_ = ResetReactions(other)
	})

	if rows, err := ListReactionEntriesFresh(ctx, chatID); err != nil || len(rows) != 0 {
		t.Fatalf("fresh list of an empty chat = %v, %v", rows, err)
	}
	if added, err := AddReactionContext(ctx, chatID, "hello", "👍", false); err != nil || !added {
		t.Fatalf("first add = %v, %v", added, err)
	}
	if added, err := AddReactionContext(ctx, chatID, "hello", "🔥", false); err != nil || added {
		t.Fatalf("add without overwrite of an existing keyword = %v, %v, want not added", added, err)
	}
	if got := GetReactions(chatID)["hello"]; got != "👍" {
		t.Fatalf("emoji after a refused overwrite = %q", got)
	}
	if added, err := AddReactionContext(ctx, chatID, "hello", "🔥", true); err != nil || !added {
		t.Fatalf("overwrite = %v, %v", added, err)
	}
	if got := GetReactions(chatID)["hello"]; got != "🔥" {
		t.Fatalf("emoji after overwrite = %q, want the cache dropped and the new emoji", got)
	}
	if _, err := AddReactionContext(ctx, other, "foreign", "👍", false); err != nil {
		t.Fatal(err)
	}
	rows, err := ListReactionEntriesFresh(ctx, chatID)
	if err != nil || len(rows) != 1 || rows[0].Keyword != "hello" || rows[0].Emoji != "🔥" {
		t.Fatalf("fresh list = %+v, %v", rows, err)
	}
	if n, err := CountReactionsFresh(ctx, chatID); err != nil || n != 1 {
		t.Fatalf("count = %d, %v", n, err)
	}
	if ok, err := ReactionExistsFresh(ctx, chatID, "hello"); err != nil || !ok {
		t.Fatalf("exists = %v, %v", ok, err)
	}
	foreign, err := ListReactionEntriesFresh(ctx, other)
	if err != nil || len(foreign) != 1 {
		t.Fatal(foreign, err)
	}
	if row, err := GetReactionEntryFresh(ctx, chatID, int64(foreign[0].ID)); err != nil || row != nil {
		t.Fatalf("another chat's ID read %+v, %v, want nil", row, err)
	}
	if row, err := RemoveReactionByID(ctx, chatID, int64(foreign[0].ID)); err != nil || row != nil {
		t.Fatalf("another chat's ID removed %+v, %v", row, err)
	}
	row, err := RemoveReactionByID(ctx, chatID, int64(rows[0].ID))
	if err != nil || row == nil || row.Keyword != "hello" {
		t.Fatalf("remove by ID = %+v, %v", row, err)
	}
	if n, _ := CountReactionsFresh(ctx, chatID); n != 0 {
		t.Fatalf("count after remove = %d", n)
	}
	if n, _ := CountReactionsFresh(ctx, other); n != 1 {
		t.Fatalf("the other chat lost its reaction")
	}
}
