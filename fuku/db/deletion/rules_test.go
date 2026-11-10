//go:build testtools

package deletion

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"testing"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	utilsCache "github.com/uasneppy/fuku_robot/fuku/utils/cache"
)

// uniqueDeletionChatID returns a random large negative chat ID, so tests sharing one
// database never collide.
func uniqueDeletionChatID() int64 {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		panic("uniqueDeletionChatID: crypto/rand failed: " + err.Error())
	}
	return -1000000000000 - int64(binary.BigEndian.Uint64(buf[:])&0x7fffffffffffffff)
}

// deletionChat returns a fresh chat ID whose rule rows, queue rows, greetings row and cached
// reads are removed when the test ends.
func deletionChat(t *testing.T) int64 {
	t.Helper()
	utilsCache.SetupTestMemoryMarshaler(t)
	chatID := uniqueDeletionChatID()
	t.Cleanup(func() {
		db.DB.Where("chat_id = ?", chatID).Delete(&models.DeleteRule{})
		db.DB.Where("chat_id = ?", chatID).Delete(&models.ScheduledDeletion{})
		db.DB.Where("chat_id = ?", chatID).Delete(&models.GreetingSettings{})
		db.DB.Where("chat_id = ?", chatID).Delete(&models.Chat{})
		db.DB.Where("chat_id = ?", chatID).Delete(&models.StaffGroup{})
		cache.DeleteCache(cache.CacheKey("delete_rules", chatID))
		cache.DeleteCache(cache.CacheKey("greetings", chatID))
		cache.ResetLocalForTest()
	})
	return chatID
}

// storedRules reads the rule rows of a chat straight from the database.
func storedRules(t *testing.T, chatID int64) map[string]int {
	t.Helper()
	var rows []models.DeleteRule
	if err := db.DB.Where("chat_id = ?", chatID).Find(&rows).Error; err != nil {
		t.Fatalf("read the rule rows: %v", err)
	}
	out := make(map[string]int, len(rows))
	for _, row := range rows {
		out[row.Kind] = row.DelaySecs
	}
	return out
}

func TestDeleteRules(t *testing.T) {
	ctx := context.Background()

	t.Run("no row", func(t *testing.T) {
		chatID := deletionChat(t)
		for name, read := range map[string]func(context.Context, int64) (*Rules, error){
			"cached": GetRules,
			"fresh":  GetRulesFresh,
		} {
			got, err := read(ctx, chatID)
			if err != nil {
				t.Fatalf("%s read: %v", name, err)
			}
			if got == nil || got.Delays == nil || len(got.Delays) != 0 {
				t.Fatalf("%s read of a chat with no row = %+v, want an empty non-nil map", name, got)
			}
		}
		if n := len(storedRules(t, chatID)); n != 0 {
			t.Fatalf("a read created %d rows, want 0", n)
		}
	})

	t.Run("set and read", func(t *testing.T) {
		chatID := deletionChat(t)
		if err := SetRule(ctx, chatID, KindPinned, 600); err != nil {
			t.Fatalf("SetRule: %v", err)
		}
		got, err := GetRules(ctx, chatID)
		if err != nil {
			t.Fatalf("GetRules: %v", err)
		}
		if delay, ok := got.Delays[KindPinned]; !ok || delay != 600 {
			t.Fatalf("pinned delay = %d (present %v), want 600", delay, ok)
		}

		// A delay of 0 is Delete now, distinct from no row.
		if err := SetRule(ctx, chatID, KindPinned, 0); err != nil {
			t.Fatalf("SetRule 0: %v", err)
		}
		got, err = GetRules(ctx, chatID)
		if err != nil {
			t.Fatalf("GetRules after 0: %v", err)
		}
		if delay, ok := got.Delays[KindPinned]; !ok || delay != 0 {
			t.Fatalf("pinned delay = %d (present %v), want a stored 0 (the write must delete the cached read)", delay, ok)
		}
		if stored := storedRules(t, chatID); len(stored) != 1 {
			t.Fatalf("stored rules = %v, want one row", stored)
		}

		if err := RemoveRule(ctx, chatID, KindPinned); err != nil {
			t.Fatalf("RemoveRule: %v", err)
		}
		got, err = GetRules(ctx, chatID)
		if err != nil {
			t.Fatalf("GetRules after remove: %v", err)
		}
		if _, ok := got.Delays[KindPinned]; ok {
			t.Fatal("pinned is still present after RemoveRule")
		}
		if err := RemoveRule(ctx, chatID, KindPinned); err != nil {
			t.Fatalf("RemoveRule of an absent kind: %v", err)
		}
	})

	t.Run("refused before any write", func(t *testing.T) {
		chatID := deletionChat(t)
		cases := []struct {
			name  string
			kind  string
			delay int
			want  error
		}{
			{"unknown kind", "service:nope", 60, ErrUnknownKind},
			{"empty kind", "", 60, ErrUnknownKind},
			{"negative delay", KindPinned, -1, ErrDelayOutOfRange},
			{"delay over a day", KindPinned, MaxDelaySecs + 1, ErrDelayOutOfRange},
		}
		for _, tc := range cases {
			if err := SetRule(ctx, chatID, tc.kind, tc.delay); !errors.Is(err, tc.want) {
				t.Errorf("%s: SetRule error = %v, want %v", tc.name, err, tc.want)
			}
		}
		if err := RemoveRule(ctx, chatID, "service:nope"); !errors.Is(err, ErrUnknownKind) {
			t.Errorf("RemoveRule of an unknown kind = %v, want ErrUnknownKind", err)
		}
		if err := SetRule(ctx, chatID, KindPinned, MaxDelaySecs); err != nil {
			t.Errorf("the largest delay is refused: %v", err)
		}
		if err := RemoveRule(ctx, chatID, KindPinned); err != nil {
			t.Fatal(err)
		}
		if n := len(storedRules(t, chatID)); n != 0 {
			t.Fatalf("%d rows exist after refused writes, want 0", n)
		}
	})

	t.Run("chats are independent", func(t *testing.T) {
		a, b := deletionChat(t), deletionChat(t)
		if err := SetRule(ctx, a, KindTitle, 30); err != nil {
			t.Fatal(err)
		}
		if err := SetRule(ctx, b, KindTitle, 90); err != nil {
			t.Fatal(err)
		}
		gotA, errA := GetRules(ctx, a)
		gotB, errB := GetRules(ctx, b)
		if errA != nil || errB != nil {
			t.Fatalf("reads: %v %v", errA, errB)
		}
		if gotA.Delays[KindTitle] != 30 || gotB.Delays[KindTitle] != 90 {
			t.Fatalf("delays = %d and %d, want 30 and 90", gotA.Delays[KindTitle], gotB.Delays[KindTitle])
		}
	})

	t.Run("served from the cache", func(t *testing.T) {
		chatID := deletionChat(t)
		if err := SetRule(ctx, chatID, KindBoost, 45); err != nil {
			t.Fatal(err)
		}
		if _, err := GetRules(ctx, chatID); err != nil {
			t.Fatal(err)
		}
		// Change the row behind the cache's back: the cached read must still answer 45.
		if err := db.DB.Model(&models.DeleteRule{}).Where("chat_id = ? AND kind = ?", chatID, KindBoost).
			Update("delay_secs", 99).Error; err != nil {
			t.Fatal(err)
		}
		cache.ResetLocalForTest()
		got, err := GetRules(ctx, chatID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Delays[KindBoost] != 45 {
			t.Fatalf("cached read = %d, want the cached 45, not the database's 99", got.Delays[KindBoost])
		}
		fresh, err := GetRulesFresh(ctx, chatID)
		if err != nil {
			t.Fatal(err)
		}
		if fresh.Delays[KindBoost] != 99 {
			t.Fatalf("fresh read = %d, want 99", fresh.Delays[KindBoost])
		}
		cache.DeleteCache(cache.CacheKey("delete_rules", chatID))
		got, err = GetRules(ctx, chatID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Delays[KindBoost] != 99 {
			t.Fatalf("read after DeleteCache = %d, want 99", got.Delays[KindBoost])
		}
	})
}

func TestKindRegistry(t *testing.T) {
	kinds := Kinds()
	if len(kinds) != 17 {
		t.Fatalf("Kinds() has %d entries, want 17", len(kinds))
	}
	seen := map[string]bool{}
	for _, kind := range kinds {
		if seen[kind] {
			t.Errorf("kind %q is listed twice", kind)
		}
		seen[kind] = true
		if !ValidKind(kind) {
			t.Errorf("registered kind %q is not valid", kind)
		}
		if IsServiceKind(kind) == IsBotKind(kind) {
			t.Errorf("kind %q must be exactly one of service and bot", kind)
		}
		if len(kind) > 40 {
			t.Errorf("kind %q does not fit VARCHAR(40)", kind)
		}
	}
	for _, kind := range []string{"", "service:", "bot:", "member:left", "service:new_chat_members "} {
		if ValidKind(kind) {
			t.Errorf("ValidKind(%q) = true, want false", kind)
		}
	}
	for _, kind := range kinds {
		want := kind == KindJoined || kind == KindLeft
		if SharesCleanServiceFlag(kind) != want {
			t.Errorf("SharesCleanServiceFlag(%q) = %v, want %v", kind, !want, want)
		}
	}
}
