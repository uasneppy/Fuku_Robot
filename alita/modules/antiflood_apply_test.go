//go:build testtools

package modules

import (
	"context"
	"fmt"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/antiflood"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/i18n"
)

// floodApplyFreshChat returns a chat ID with no antiflood row and removes whatever the test
// writes for it.
func floodApplyFreshChat(t *testing.T) int64 {
	t.Helper()
	chatID := uniqueModuleChatID()
	t.Cleanup(func() {
		if err := db.DB.Where("chat_id = ?", chatID).Delete(&models.AntifloodSettings{}).Error; err != nil {
			t.Errorf("cleanup failed: %v", err)
		}
	})
	return chatID
}

// floodApplyRowCount counts the antiflood rows of a chat straight from the database.
func floodApplyRowCount(t *testing.T, chatID int64) int64 {
	t.Helper()
	var count int64
	if err := db.DB.Model(&models.AntifloodSettings{}).Where("chat_id = ?", chatID).Count(&count).Error; err != nil {
		t.Fatalf("count query failed: %v", err)
	}
	return count
}

// floodApplyStored reads the row the way the apply functions do: uncached, from the database.
func floodApplyStored(t *testing.T, chatID int64) *models.AntifloodSettings {
	t.Helper()
	stored, err := antiflood.GetAntifloodSettingsContext(context.Background(), chatID)
	if err != nil {
		t.Fatalf("uncached read failed: %v", err)
	}
	return stored
}

// floodApplyBrokenDB swaps db.DB for an empty SQLite database with no tables, so every read and
// write fails, and restores it when the test ends.
func floodApplyBrokenDB(t *testing.T) {
	t.Helper()
	original := db.DB
	broken, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := broken.DB()
	if err != nil {
		t.Fatal(err)
	}
	db.DB = broken
	t.Cleanup(func() {
		db.DB = original
		_ = sqlDB.Close()
	})
}

func TestApplyFloodLimit(t *testing.T) {
	ctx := context.Background()

	t.Run("a limit in range is stored and read back", func(t *testing.T) {
		chatID := floodApplyFreshChat(t)
		got := applyFloodLimit(ctx, chatID, 7)
		if got.Outcome != floodApplied || got.Err != nil {
			t.Fatalf("applyFloodLimit(7) = outcome %v err %v, want floodApplied", got.Outcome, got.Err)
		}
		if got.Stored == nil || got.Stored.Limit != 7 {
			t.Fatalf("stored = %+v, want limit 7 read from the database", got.Stored)
		}
		if row := floodApplyStored(t, chatID); row.Limit != 7 {
			t.Fatalf("database limit = %d, want 7", row.Limit)
		}
	})

	t.Run("the range edges are accepted", func(t *testing.T) {
		for _, limit := range []int{3, 100} {
			chatID := floodApplyFreshChat(t)
			got := applyFloodLimit(ctx, chatID, limit)
			if got.Outcome != floodApplied || got.Stored == nil || got.Stored.Limit != limit {
				t.Fatalf("applyFloodLimit(%d) = %+v, want floodApplied with that limit", limit, got)
			}
		}
	})

	t.Run("zero turns antiflood off", func(t *testing.T) {
		chatID := floodApplyFreshChat(t)
		if first := applyFloodLimit(ctx, chatID, 7); first.Outcome != floodApplied {
			t.Fatalf("setup applyFloodLimit(7) = %+v", first)
		}
		got := applyFloodLimit(ctx, chatID, 0)
		if got.Outcome != floodDisabled || got.Err != nil {
			t.Fatalf("applyFloodLimit(0) = outcome %v err %v, want floodDisabled", got.Outcome, got.Err)
		}
		if got.Stored == nil || got.Stored.Limit != 0 {
			t.Fatalf("stored = %+v, want limit 0", got.Stored)
		}
	})

	t.Run("zero on a fresh chat reports a disabled row", func(t *testing.T) {
		chatID := floodApplyFreshChat(t)
		got := applyFloodLimit(ctx, chatID, 0)
		if got.Outcome != floodDisabled || got.Stored == nil || got.Stored.Limit != 0 {
			t.Fatalf("applyFloodLimit(0) on a fresh chat = %+v, want floodDisabled with limit 0", got)
		}
	})

	t.Run("out of range writes nothing", func(t *testing.T) {
		for _, limit := range []int{1, 2, 101, -5} {
			chatID := floodApplyFreshChat(t)
			got := applyFloodLimit(ctx, chatID, limit)
			if got.Outcome != floodOutOfRange || got.Stored != nil || got.Err != nil {
				t.Fatalf("applyFloodLimit(%d) = %+v, want floodOutOfRange with no row and no error", limit, got)
			}
			if count := floodApplyRowCount(t, chatID); count != 0 {
				t.Fatalf("applyFloodLimit(%d) created %d row(s), want none", limit, count)
			}
		}
	})

	t.Run("out of range leaves an existing limit unchanged", func(t *testing.T) {
		chatID := floodApplyFreshChat(t)
		if first := applyFloodLimit(ctx, chatID, 9); first.Outcome != floodApplied {
			t.Fatalf("setup applyFloodLimit(9) = %+v", first)
		}
		if got := applyFloodLimit(ctx, chatID, 2); got.Outcome != floodOutOfRange {
			t.Fatalf("applyFloodLimit(2) = %+v, want floodOutOfRange", got)
		}
		if row := floodApplyStored(t, chatID); row.Limit != 9 {
			t.Fatalf("database limit = %d, want 9 unchanged", row.Limit)
		}
	})

	t.Run("a repository error is a failed save", func(t *testing.T) {
		floodApplyBrokenDB(t)
		got := applyFloodLimit(ctx, uniqueModuleChatID(), 7)
		if got.Outcome != floodSaveFailed || got.Err == nil || got.Stored != nil {
			t.Fatalf("applyFloodLimit on a broken database = %+v, want floodSaveFailed with Err set and no row", got)
		}
	})
}

func TestApplyFloodMode(t *testing.T) {
	ctx := context.Background()

	t.Run("ban kick and mute are stored", func(t *testing.T) {
		chatID := floodApplyFreshChat(t)
		for _, mode := range []string{"ban", "kick", "mute"} {
			got := applyFloodMode(ctx, chatID, mode)
			if got.Outcome != floodApplied || got.Err != nil {
				t.Fatalf("applyFloodMode(%q) = outcome %v err %v, want floodApplied", mode, got.Outcome, got.Err)
			}
			if got.Stored == nil || got.Stored.Action != mode {
				t.Fatalf("applyFloodMode(%q) stored = %+v, want that action", mode, got.Stored)
			}
			if row := floodApplyStored(t, chatID); row.Action != mode {
				t.Fatalf("database action = %q, want %q", row.Action, mode)
			}
		}
	})

	t.Run("a first mode write on a fresh chat reports the limit it stored", func(t *testing.T) {
		chatID := floodApplyFreshChat(t)
		got := applyFloodMode(ctx, chatID, "ban")
		// TestSetFloodModeOnFreshChatStoredLimit pins the repository's 5 (D-27).
		if got.Stored == nil || got.Stored.Limit != 5 {
			t.Fatalf("stored = %+v, want the pinned first-write limit 5", got.Stored)
		}
	})

	t.Run("anything else is refused and writes nothing", func(t *testing.T) {
		for _, mode := range []string{"warn", "tban", "tmute", "Ban ", "BAN", ""} {
			chatID := floodApplyFreshChat(t)
			got := applyFloodMode(ctx, chatID, mode)
			if got.Outcome != floodUnknownMode || got.Stored != nil || got.Err != nil {
				t.Fatalf("applyFloodMode(%q) = %+v, want floodUnknownMode with no row and no error", mode, got)
			}
			if count := floodApplyRowCount(t, chatID); count != 0 {
				t.Fatalf("applyFloodMode(%q) created %d row(s), want none", mode, count)
			}
		}
	})

	t.Run("a repository error is a failed save", func(t *testing.T) {
		floodApplyBrokenDB(t)
		got := applyFloodMode(ctx, uniqueModuleChatID(), "ban")
		if got.Outcome != floodSaveFailed || got.Err == nil || got.Stored != nil {
			t.Fatalf("applyFloodMode on a broken database = %+v, want floodSaveFailed with Err set and no row", got)
		}
	})
}

func TestApplyFloodDelete(t *testing.T) {
	ctx := context.Background()

	t.Run("true and false are stored and read back", func(t *testing.T) {
		chatID := floodApplyFreshChat(t)
		on := applyFloodDelete(ctx, chatID, true)
		if on.Outcome != floodApplied || on.Err != nil || on.Stored == nil || !on.Stored.DeleteAntifloodMessage {
			t.Fatalf("applyFloodDelete(true) = %+v, want floodApplied with the switch on", on)
		}
		off := applyFloodDelete(ctx, chatID, false)
		if off.Outcome != floodApplied || off.Err != nil || off.Stored == nil || off.Stored.DeleteAntifloodMessage {
			t.Fatalf("applyFloodDelete(false) = %+v, want floodApplied with the switch off", off)
		}
		if row := floodApplyStored(t, chatID); row.DeleteAntifloodMessage {
			t.Fatal("database delete switch = true, want false")
		}
	})

	t.Run("a repository error is a failed save", func(t *testing.T) {
		floodApplyBrokenDB(t)
		got := applyFloodDelete(ctx, uniqueModuleChatID(), true)
		if got.Outcome != floodSaveFailed || got.Err == nil || got.Stored != nil {
			t.Fatalf("applyFloodDelete on a broken database = %+v, want floodSaveFailed with Err set and no row", got)
		}
	})
}

// TestAntifloodCommandsUseApply drives each changing command and checks that its reply is the
// one it always sent and that the row it left behind is the row the apply function leaves for
// the same input.
func TestAntifloodCommandsUseApply(t *testing.T) {
	admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	tr := i18n.MustNewTranslator("en")
	text := func(key string, args ...any) string {
		temp, err := tr.GetString(key)
		if err != nil || temp == "" {
			t.Fatalf("locale key %q missing: %v", key, err)
		}
		if len(args) == 0 {
			return temp
		}
		return fmt.Sprintf(temp, args...)
	}

	tests := []struct {
		name    string
		command string
		run     func(*gotgbot.Bot, *ext.Context) error
		reply   string
		// apply runs the same input through the apply function on another chat.
		apply func(ctx context.Context, chatID int64)
	}{
		{
			name: "setflood number", command: "/setflood 7", run: antifloodModule.setFlood,
			reply: text("antiflood_setflood_success", 7),
			apply: func(ctx context.Context, chatID int64) { applyFloodLimit(ctx, chatID, 7) },
		},
		{
			name: "setflood off", command: "/setflood off", run: antifloodModule.setFlood,
			reply: text("antiflood_setflood_disabled"),
			apply: func(ctx context.Context, chatID int64) { applyFloodLimit(ctx, chatID, 0) },
		},
		{
			name: "setflood below range", command: "/setflood 1", run: antifloodModule.setFlood,
			reply: text("antiflood_errors_set_in_limit"),
			apply: func(ctx context.Context, chatID int64) { applyFloodLimit(ctx, chatID, 1) },
		},
		{
			name: "setfloodmode kick", command: "/setfloodmode kick", run: antifloodModule.setFloodMode,
			reply: text("antiflood_setfloodmode_success", "kick"),
			apply: func(ctx context.Context, chatID int64) { applyFloodMode(ctx, chatID, "kick") },
		},
		{
			name: "setfloodmode upper case", command: "/setfloodmode BAN", run: antifloodModule.setFloodMode,
			reply: text("antiflood_setfloodmode_success", "ban"),
			apply: func(ctx context.Context, chatID int64) { applyFloodMode(ctx, chatID, "ban") },
		},
		{
			name: "setfloodmode warn", command: "/setfloodmode warn", run: antifloodModule.setFloodMode,
			reply: text("antiflood_setfloodmode_unknown_type", "warn"),
			apply: func(ctx context.Context, chatID int64) { applyFloodMode(ctx, chatID, "warn") },
		},
		{
			name: "delflood on", command: "/delflood on", run: antifloodModule.setFloodDeleter,
			reply: text("antiflood_flood_deleter_enabled"),
			apply: func(ctx context.Context, chatID int64) { applyFloodDelete(ctx, chatID, true) },
		},
		{
			name: "delflood off", command: "/delflood off", run: antifloodModule.setFloodDeleter,
			reply: text("antiflood_flood_deleter_disabled"),
			apply: func(ctx context.Context, chatID int64) { applyFloodDelete(ctx, chatID, false) },
		},
		{
			name: "delflood invalid", command: "/delflood maybe", run: antifloodModule.setFloodDeleter,
			reply: text("antiflood_flood_deleter_invalid_option"),
			apply: func(context.Context, int64) {},
		},
		{
			name: "clearflood alias", command: "/clearflood yes", run: antifloodModule.setFloodDeleter,
			reply: text("antiflood_flood_deleter_enabled"),
			apply: func(ctx context.Context, chatID int64) { applyFloodDelete(ctx, chatID, true) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetAntifloodState(t)
			client := newModuleBotClient()
			bot := newModuleTestBot(client)
			chatID := floodApplyFreshChat(t)
			chat := gotgbot.Chat{Id: chatID, Type: "supergroup", Title: "Flood Chat"}

			if err := tt.run(bot, newModuleMessageContext(bot, chat, admin, tt.command)); err != ext.EndGroups {
				t.Fatalf("%s error = %v, want EndGroups", tt.command, err)
			}
			calls := client.callsFor("sendMessage")
			if len(calls) != 1 {
				t.Fatalf("sendMessage calls = %d, want one reply", len(calls))
			}
			if got := fmt.Sprint(calls[0].Params["text"]); got != tt.reply {
				t.Fatalf("reply = %q, want %q", got, tt.reply)
			}

			wantChat := floodApplyFreshChat(t)
			tt.apply(context.Background(), wantChat)
			got, want := floodApplyStored(t, chatID), floodApplyStored(t, wantChat)
			if got.Limit != want.Limit || got.Action != want.Action || got.DeleteAntifloodMessage != want.DeleteAntifloodMessage {
				t.Fatalf("row after %s = %+v, want the apply function's row %+v", tt.command, got, want)
			}
		})
	}
}
