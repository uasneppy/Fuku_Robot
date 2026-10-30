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
	aiEnabled      bool
	aiThreshold    int
}

func columnsOf(s *models.ChatRaidDetection) raidColumns {
	return raidColumns{s.JoinEnabled, s.JoinThreshold, s.FloodEnabled, s.FloodThreshold, s.AIEnabled, s.AIThreshold}
}

func TestRaidDetectionSettings(t *testing.T) {
	defaults := raidColumns{true, DefaultJoinThreshold, true, DefaultFloodThreshold, true, DefaultAIThreshold}

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
		want := raidColumns{false, DefaultJoinThreshold, true, DefaultFloodThreshold, true, DefaultAIThreshold}
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
		want := raidColumns{true, 20, true, DefaultFloodThreshold, true, DefaultAIThreshold}
		if got := coldSettings(t, chatID); columnsOf(got) != want {
			t.Errorf("settings = %+v, want %+v", columnsOf(got), want)
		}

		if err := SetFloodEnabled(chatID, false); err != nil {
			t.Fatalf("SetFloodEnabled(false): %v", err)
		}
		if err := SetFloodThreshold(chatID, 45); err != nil {
			t.Fatalf("SetFloodThreshold(45): %v", err)
		}
		want = raidColumns{true, 20, true, 45, true, DefaultAIThreshold}
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
		want := raidColumns{true, 20, false, 45, true, DefaultAIThreshold}
		if got := coldSettings(t, chatID); columnsOf(got) != want {
			t.Errorf("after switching flood off: %+v, want %+v (join columns and the flood threshold untouched)", columnsOf(got), want)
		}

		if err := SetJoinEnabled(chatID, false); err != nil {
			t.Fatalf("SetJoinEnabled(false): %v", err)
		}
		want = raidColumns{false, 20, false, 45, true, DefaultAIThreshold}
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
		want := raidColumns{true, MinThreshold, true, MaxThreshold, true, DefaultAIThreshold}
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

	t.Run("a chat with no row reads the AI defaults", func(t *testing.T) {
		chatID := raidSettingsChat(t)
		got, err := GetSettings(context.Background(), chatID)
		if err != nil {
			t.Fatalf("GetSettings: %v", err)
		}
		if !got.AIEnabled || got.AIThreshold != 5 {
			t.Errorf("AI settings = (%v, %d), want (true, 5)", got.AIEnabled, got.AIThreshold)
		}
	})

	t.Run("an AI threshold turns the AI trigger on", func(t *testing.T) {
		chatID := raidSettingsChat(t)
		if err := SetAIEnabled(chatID, false); err != nil {
			t.Fatalf("SetAIEnabled(false): %v", err)
		}
		if err := SetAIThreshold(chatID, 7); err != nil {
			t.Fatalf("SetAIThreshold(7): %v", err)
		}
		got := coldSettings(t, chatID)
		if !got.AIEnabled || got.AIThreshold != 7 {
			t.Errorf("AI settings = (%v, %d), want (true, 7)", got.AIEnabled, got.AIThreshold)
		}
	})

	t.Run("the AI off switch survives a cold read and keeps its threshold", func(t *testing.T) {
		chatID := raidSettingsChat(t)
		if err := SetAIThreshold(chatID, 9); err != nil {
			t.Fatalf("SetAIThreshold(9): %v", err)
		}
		if err := SetAIEnabled(chatID, false); err != nil {
			t.Fatalf("SetAIEnabled(false): %v", err)
		}
		row, ok := storedSettings(t, chatID)
		if !ok || row.AIEnabled {
			t.Fatalf("stored row = %+v (found %v), want ai_enabled false", row, ok)
		}
		got := coldSettings(t, chatID)
		if got.AIEnabled || got.AIThreshold != 9 {
			t.Errorf("AI settings = (%v, %d), want (false, 9)", got.AIEnabled, got.AIThreshold)
		}
	})

	t.Run("AI writes never change the join or flood columns, and their writes never change the AI columns", func(t *testing.T) {
		chatID := raidSettingsChat(t)
		steps := []struct {
			name  string
			write func() error
			want  raidColumns
		}{
			{"join threshold 20", func() error { return SetJoinThreshold(chatID, 20) }, raidColumns{true, 20, true, DefaultFloodThreshold, true, 5}},
			{"AI threshold 8", func() error { return SetAIThreshold(chatID, 8) }, raidColumns{true, 20, true, DefaultFloodThreshold, true, 8}},
			{"flood threshold 45", func() error { return SetFloodThreshold(chatID, 45) }, raidColumns{true, 20, true, 45, true, 8}},
			{"AI off", func() error { return SetAIEnabled(chatID, false) }, raidColumns{true, 20, true, 45, false, 8}},
			{"join off", func() error { return SetJoinEnabled(chatID, false) }, raidColumns{false, 20, true, 45, false, 8}},
			{"flood off", func() error { return SetFloodEnabled(chatID, false) }, raidColumns{false, 20, false, 45, false, 8}},
			{"AI on", func() error { return SetAIEnabled(chatID, true) }, raidColumns{false, 20, false, 45, true, 8}},
		}
		for _, step := range steps {
			if err := step.write(); err != nil {
				t.Fatalf("%s: %v", step.name, err)
			}
			if got := coldSettings(t, chatID); columnsOf(got) != step.want {
				t.Fatalf("after %s: %+v, want %+v", step.name, columnsOf(got), step.want)
			}
		}
	})

	t.Run("an AI threshold outside 3 to 500 is refused and writes nothing", func(t *testing.T) {
		chatID := raidSettingsChat(t)
		for _, threshold := range []int{2, 0, -5, 501} {
			if err := SetAIThreshold(chatID, threshold); !errors.Is(err, ErrThresholdOutOfRange) {
				t.Errorf("SetAIThreshold(%d): err = %v, want ErrThresholdOutOfRange", threshold, err)
			}
		}
		if _, ok := storedSettings(t, chatID); ok {
			t.Error("a refused AI threshold wrote a row")
		}
	})

	t.Run("a row written directly with ai_threshold 2 is refused by the database", func(t *testing.T) {
		chatID := raidSettingsChat(t)
		row := Defaults(chatID)
		row.AIThreshold = 2
		if err := db.DB.Create(row).Error; err == nil {
			t.Error("the database accepted ai_threshold 2, want the chk_chat_raid_detection_ai check to refuse it")
		}
		if _, ok := storedSettings(t, chatID); ok {
			t.Error("a refused row exists")
		}
	})

	t.Run("a read cached before an AI write sees the new value after it", func(t *testing.T) {
		chatID := raidSettingsChat(t)
		if _, err := GetSettings(context.Background(), chatID); err != nil {
			t.Fatalf("GetSettings: %v", err)
		}
		if err := SetAIThreshold(chatID, 12); err != nil {
			t.Fatalf("SetAIThreshold(12): %v", err)
		}
		after, err := GetSettings(context.Background(), chatID)
		if err != nil {
			t.Fatalf("GetSettings: %v", err)
		}
		if after.AIThreshold != 12 {
			t.Errorf("AI threshold after the write = %d, want 12: the write must invalidate the cached read", after.AIThreshold)
		}
		if err := SetAIEnabled(chatID, false); err != nil {
			t.Fatalf("SetAIEnabled(false): %v", err)
		}
		after, err = GetSettings(context.Background(), chatID)
		if err != nil {
			t.Fatalf("GetSettings: %v", err)
		}
		if after.AIEnabled {
			t.Error("ai_enabled after switching it off is still true: the write must invalidate the cached read")
		}
	})
}

// settingsBeforeAI is the settings value the previous build cached: the Phase 6 fields
// only, under the same Go field names, so msgpack encodes it the way that build did.
type settingsBeforeAI struct {
	ChatID         int64
	JoinEnabled    bool
	JoinThreshold  int
	FloodEnabled   bool
	FloodThreshold int
}

// TestRaidDetectionStaleCacheDefaults covers a deploy over the previous build's cache: a
// value cached without the AI fields decodes as (false, 0), which must read as the
// defaults, never as "AI off" or a threshold of 0 (research Pitfall 1).
func TestRaidDetectionStaleCacheDefaults(t *testing.T) {
	t.Run("a value cached before the AI fields reads as AI on at 5", func(t *testing.T) {
		chatID := raidSettingsChat(t)
		key := cache.CacheKey("raid_detection", chatID)
		old := settingsBeforeAI{ChatID: chatID, JoinEnabled: false, JoinThreshold: 20, FloodEnabled: true, FloodThreshold: 45}
		if err := utilsCache.GetMarshal().Set(utilsCache.Context, key, old); err != nil {
			t.Fatalf("cache the old-shape value: %v", err)
		}
		got, err := GetSettings(context.Background(), chatID)
		if err != nil {
			t.Fatalf("GetSettings: %v", err)
		}
		want := raidColumns{false, 20, true, 45, true, DefaultAIThreshold}
		if columnsOf(got) != want {
			t.Errorf("settings = %+v, want %+v (stored join and flood values kept, AI at its defaults)", columnsOf(got), want)
		}
	})

	t.Run("a cached value with its own AI settings is returned unchanged", func(t *testing.T) {
		chatID := raidSettingsChat(t)
		key := cache.CacheKey("raid_detection", chatID)
		cached := models.ChatRaidDetection{
			ChatID: chatID, JoinEnabled: true, JoinThreshold: 15, FloodEnabled: true, FloodThreshold: 30,
			AIEnabled: false, AIThreshold: 9,
		}
		if err := utilsCache.GetMarshal().Set(utilsCache.Context, key, cached); err != nil {
			t.Fatalf("cache the value: %v", err)
		}
		got, err := GetSettings(context.Background(), chatID)
		if err != nil {
			t.Fatalf("GetSettings: %v", err)
		}
		if got.AIEnabled || got.AIThreshold != 9 {
			t.Errorf("AI settings = (%v, %d), want the cached (false, 9)", got.AIEnabled, got.AIThreshold)
		}
	})
}
