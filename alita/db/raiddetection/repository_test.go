//go:build testtools

package raiddetection

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"testing"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/cache"
	"github.com/divkix/Alita_Robot/alita/db/models"
	utilsCache "github.com/divkix/Alita_Robot/alita/utils/cache"
)

// uniqueRaidChatID returns a random large negative chat ID, so tests sharing one
// database never collide.
func uniqueRaidChatID() int64 {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		panic("uniqueRaidChatID: crypto/rand failed: " + err.Error())
	}
	return -1000000000000 - int64(binary.BigEndian.Uint64(buf[:])&0x7fffffffffffffff)
}

// raidSettingsChat returns a fresh chat ID whose settings row and cached copy are
// removed when the test ends.
func raidSettingsChat(t *testing.T) int64 {
	t.Helper()
	utilsCache.SetupTestMemoryMarshaler(t)
	chatID := uniqueRaidChatID()
	t.Cleanup(func() {
		db.DB.Where("chat_id = ?", chatID).Delete(&models.ChatRaidDetection{})
		cache.DeleteCache(cache.CacheKey("raid_detection", chatID))
	})
	return chatID
}

// storedSettings reads the chat's row straight from the database, bypassing every
// cache, and reports whether there is one.
func storedSettings(t *testing.T, chatID int64) (models.ChatRaidDetection, bool) {
	t.Helper()
	var rows []models.ChatRaidDetection
	if err := db.DB.Where("chat_id = ?", chatID).Find(&rows).Error; err != nil {
		t.Fatalf("read the settings row: %v", err)
	}
	if len(rows) == 0 {
		return models.ChatRaidDetection{}, false
	}
	return rows[0], true
}

// coldSettings drops the cached copy and reads the settings again, so the answer comes
// from the database.
func coldSettings(t *testing.T, chatID int64) *models.ChatRaidDetection {
	t.Helper()
	cache.DeleteCache(cache.CacheKey("raid_detection", chatID))
	got, err := GetSettings(context.Background(), chatID)
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	return got
}

type raidColumns struct {
	joinEnabled    bool
	joinThreshold  int
	floodEnabled   bool
	floodThreshold int
}

func columnsOf(s *models.ChatRaidDetection) raidColumns {
	return raidColumns{s.JoinEnabled, s.JoinThreshold, s.FloodEnabled, s.FloodThreshold}
}

func TestRaidDetectionSettings(t *testing.T) {
	defaults := raidColumns{true, DefaultJoinThreshold, true, DefaultFloodThreshold}

	t.Run("a chat with no row reads the defaults", func(t *testing.T) {
		chatID := raidSettingsChat(t)
		got, err := GetSettings(context.Background(), chatID)
		if err != nil {
			t.Fatalf("GetSettings: %v", err)
		}
		if columnsOf(got) != defaults {
			t.Errorf("settings = %+v, want the defaults %+v", columnsOf(got), defaults)
		}
		if _, ok := storedSettings(t, chatID); ok {
			t.Error("reading the defaults wrote a row")
		}
	})

	t.Run("an off switch survives a cold read", func(t *testing.T) {
		chatID := raidSettingsChat(t)
		if err := SetJoinEnabled(chatID, false); err != nil {
			t.Fatalf("SetJoinEnabled(false): %v", err)
		}
		row, ok := storedSettings(t, chatID)
		if !ok {
			t.Fatal("no row after SetJoinEnabled(false)")
		}
		if row.JoinEnabled {
			t.Error("the stored join_enabled is true, want the false that was written")
		}
		got := coldSettings(t, chatID)
		want := raidColumns{false, DefaultJoinThreshold, true, DefaultFloodThreshold}
		if columnsOf(got) != want {
			t.Errorf("cold settings = %+v, want %+v (the first write inserts the other trigger's defaults)", columnsOf(got), want)
		}
	})

	t.Run("a threshold turns its trigger on", func(t *testing.T) {
		chatID := raidSettingsChat(t)
		if err := SetJoinEnabled(chatID, false); err != nil {
			t.Fatalf("SetJoinEnabled(false): %v", err)
		}
		if err := SetJoinThreshold(chatID, 20); err != nil {
			t.Fatalf("SetJoinThreshold(20): %v", err)
		}
		want := raidColumns{true, 20, true, DefaultFloodThreshold}
		if got := coldSettings(t, chatID); columnsOf(got) != want {
			t.Errorf("settings = %+v, want %+v", columnsOf(got), want)
		}

		if err := SetFloodEnabled(chatID, false); err != nil {
			t.Fatalf("SetFloodEnabled(false): %v", err)
		}
		if err := SetFloodThreshold(chatID, 45); err != nil {
			t.Fatalf("SetFloodThreshold(45): %v", err)
		}
		want = raidColumns{true, 20, true, 45}
		if got := coldSettings(t, chatID); columnsOf(got) != want {
			t.Errorf("settings = %+v, want %+v", columnsOf(got), want)
		}
	})

	t.Run("one trigger's write never changes the other's columns", func(t *testing.T) {
		chatID := raidSettingsChat(t)
		if err := SetJoinThreshold(chatID, 20); err != nil {
			t.Fatalf("SetJoinThreshold(20): %v", err)
		}
		if err := SetFloodThreshold(chatID, 45); err != nil {
			t.Fatalf("SetFloodThreshold(45): %v", err)
		}

		if err := SetFloodEnabled(chatID, false); err != nil {
			t.Fatalf("SetFloodEnabled(false): %v", err)
		}
		want := raidColumns{true, 20, false, 45}
		if got := coldSettings(t, chatID); columnsOf(got) != want {
			t.Errorf("after switching flood off: %+v, want %+v (join columns and the flood threshold untouched)", columnsOf(got), want)
		}

		if err := SetJoinEnabled(chatID, false); err != nil {
			t.Fatalf("SetJoinEnabled(false): %v", err)
		}
		want = raidColumns{false, 20, false, 45}
		if got := coldSettings(t, chatID); columnsOf(got) != want {
			t.Errorf("after switching joins off: %+v, want %+v (both thresholds kept)", columnsOf(got), want)
		}
	})

	t.Run("a threshold outside 3 to 500 is refused and writes nothing", func(t *testing.T) {
		chatID := raidSettingsChat(t)
		for name, write := range map[string]func() error{
			"join 2":    func() error { return SetJoinThreshold(chatID, 2) },
			"join 0":    func() error { return SetJoinThreshold(chatID, 0) },
			"join -5":   func() error { return SetJoinThreshold(chatID, -5) },
			"flood 501": func() error { return SetFloodThreshold(chatID, 501) },
		} {
			if err := write(); !errors.Is(err, ErrThresholdOutOfRange) {
				t.Errorf("%s: err = %v, want ErrThresholdOutOfRange", name, err)
			}
		}
		if _, ok := storedSettings(t, chatID); ok {
			t.Error("a refused threshold wrote a row")
		}
	})

	t.Run("the limits themselves are accepted", func(t *testing.T) {
		chatID := raidSettingsChat(t)
		if err := SetJoinThreshold(chatID, MinThreshold); err != nil {
			t.Fatalf("SetJoinThreshold(%d): %v", MinThreshold, err)
		}
		if err := SetFloodThreshold(chatID, MaxThreshold); err != nil {
			t.Fatalf("SetFloodThreshold(%d): %v", MaxThreshold, err)
		}
		want := raidColumns{true, MinThreshold, true, MaxThreshold}
		if got := coldSettings(t, chatID); columnsOf(got) != want {
			t.Errorf("settings = %+v, want %+v", columnsOf(got), want)
		}
	})

	t.Run("a read cached before a write sees the new value after it", func(t *testing.T) {
		chatID := raidSettingsChat(t)
		before, err := GetSettings(context.Background(), chatID)
		if err != nil {
			t.Fatalf("GetSettings: %v", err)
		}
		if columnsOf(before) != defaults {
			t.Fatalf("settings before the write = %+v, want the defaults", columnsOf(before))
		}

		if err := SetJoinThreshold(chatID, 20); err != nil {
			t.Fatalf("SetJoinThreshold(20): %v", err)
		}
		after, err := GetSettings(context.Background(), chatID)
		if err != nil {
			t.Fatalf("GetSettings: %v", err)
		}
		if after.JoinThreshold != 20 {
			t.Errorf("join threshold after the write = %d, want 20: the write must invalidate the cached read", after.JoinThreshold)
		}

		if err := SetJoinEnabled(chatID, false); err != nil {
			t.Fatalf("SetJoinEnabled(false): %v", err)
		}
		after, err = GetSettings(context.Background(), chatID)
		if err != nil {
			t.Fatalf("GetSettings: %v", err)
		}
		if after.JoinEnabled {
			t.Error("join_enabled after switching it off is still true: the write must invalidate the cached read")
		}
	})
}
