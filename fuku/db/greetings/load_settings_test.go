package greetings

import (
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/chats"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// failGreetingsReads makes every read of the greetings table fail with a database error until the test ends,
// through a real GORM callback on the test database.
func failGreetingsReads(t *testing.T) {
	t.Helper()
	name := "test:fail_greetings_read_" + t.Name()
	err := db.DB.Callback().Query().Before("gorm:query").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "greetings" {
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

// TestLoadGreetingSettingsReturnsReadError proves the welcome path's read returns a database error, where
// GetGreetingSettings keeps hiding it behind the Welcome-on defaults every other caller relies on.
func TestLoadGreetingSettingsReturnsReadError(t *testing.T) {
	skipIfNoDb(t)

	chatID := time.Now().UnixNano()
	if err := chats.EnsureChatInDb(chatID, "load_greetings_error"); err != nil {
		t.Fatalf("EnsureChatInDb() error = %v", err)
	}
	t.Cleanup(func() {
		db.DB.Where("chat_id = ?", chatID).Delete(&models.GreetingSettings{})
		db.DB.Where("chat_id = ?", chatID).Delete(&models.Chat{})
	})
	if err := SetWelcomeOnce(chatID, true); err != nil {
		t.Fatalf("SetWelcomeOnce setup error = %v", err)
	}

	failGreetingsReads(t)

	settings, err := LoadGreetingSettings(chatID)
	if err == nil {
		t.Fatal("LoadGreetingSettings() error = nil, want the database error: the welcome path must not guess")
	}
	if settings == nil || settings.WelcomeSettings == nil || settings.GoodbyeSettings == nil {
		t.Fatalf("LoadGreetingSettings() settings = %+v, want the fallback defaults next to the error", settings)
	}

	fallback := GetGreetingSettings(chatID)
	if fallback == nil || fallback.WelcomeSettings == nil || !fallback.WelcomeSettings.ShouldWelcome ||
		fallback.WelcomeSettings.Once {
		t.Fatalf("GetGreetingSettings() = %+v, want the unchanged fallback (Welcome on, Greet once off)", fallback)
	}
}

// TestLoadGreetingSettingsMatchesGetGreetingSettings proves a successful read is exactly today's: a stored row is
// returned as stored, and a chat with no row gets the defaults, creating the row only for a chat the bot knows.
func TestLoadGreetingSettingsMatchesGetGreetingSettings(t *testing.T) {
	skipIfNoDb(t)

	stored := time.Now().UnixNano()
	known := stored + 1
	unknown := stored + 2
	for _, id := range []int64{stored, known} {
		if err := chats.EnsureChatInDb(id, "load_greetings_ok"); err != nil {
			t.Fatalf("EnsureChatInDb(%d) error = %v", id, err)
		}
	}
	t.Cleanup(func() {
		for _, id := range []int64{stored, known, unknown} {
			db.DB.Where("chat_id = ?", id).Delete(&models.GreetingSettings{})
			db.DB.Where("chat_id = ?", id).Delete(&models.Chat{})
		}
	})
	if err := SetWelcomeText(stored, "Welcome {first}", "", nil, db.TEXT); err != nil {
		t.Fatalf("SetWelcomeText setup error = %v", err)
	}
	if err := SetWelcomeOnce(stored, true); err != nil {
		t.Fatalf("SetWelcomeOnce setup error = %v", err)
	}

	got, err := LoadGreetingSettings(stored)
	if err != nil {
		t.Fatalf("LoadGreetingSettings(stored) error = %v", err)
	}
	if got.WelcomeSettings == nil || got.WelcomeSettings.WelcomeText != "Welcome {first}" || !got.WelcomeSettings.Once {
		t.Fatalf("LoadGreetingSettings(stored) = %+v, want the stored row", got.WelcomeSettings)
	}

	got, err = LoadGreetingSettings(known)
	if err != nil {
		t.Fatalf("LoadGreetingSettings(known, no row) error = %v", err)
	}
	if got.WelcomeSettings == nil || !got.WelcomeSettings.ShouldWelcome || got.WelcomeSettings.WelcomeText != db.DefaultWelcome {
		t.Fatalf("LoadGreetingSettings(known, no row) = %+v, want the defaults", got.WelcomeSettings)
	}
	var rows int64
	if err := db.DB.Model(&models.GreetingSettings{}).Where("chat_id = ?", known).Count(&rows).Error; err != nil || rows != 1 {
		t.Fatalf("rows for a known chat after the read = %d, %v, want the row GetGreetingSettings creates", rows, err)
	}

	got, err = LoadGreetingSettings(unknown)
	if err != nil {
		t.Fatalf("LoadGreetingSettings(unknown chat) error = %v", err)
	}
	if got.WelcomeSettings == nil || !got.WelcomeSettings.ShouldWelcome {
		t.Fatalf("LoadGreetingSettings(unknown chat) = %+v, want the defaults", got.WelcomeSettings)
	}
	if err := db.DB.Model(&models.GreetingSettings{}).Where("chat_id = ?", unknown).Count(&rows).Error; err != nil || rows != 0 {
		t.Fatalf("rows for an unknown chat after the read = %d, %v, want none", rows, err)
	}
}
