package greetings

import (
	"context"
	"testing"
	"time"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/chats"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

func countGreetingRows(t *testing.T, chatID int64) int64 {
	t.Helper()
	var n int64
	if err := db.DB.Model(&models.GreetingSettings{}).Where("chat_id = ?", chatID).Count(&n).Error; err != nil {
		t.Fatalf("count greetings rows: %v", err)
	}
	return n
}

func countChatRows(t *testing.T, chatID int64) int64 {
	t.Helper()
	var n int64
	if err := db.DB.Model(&models.Chat{}).Where("chat_id = ?", chatID).Count(&n).Error; err != nil {
		t.Fatalf("count chat rows: %v", err)
	}
	return n
}

func cleanupEditorChat(t *testing.T, chatID int64) {
	t.Helper()
	t.Cleanup(func() {
		db.DB.Where("chat_id = ?", chatID).Delete(&models.GreetingSettings{})
		db.DB.Where("chat_id = ?", chatID).Delete(&models.Chat{})
	})
}

func TestLoadGreetingSettingsCreatesNothing(t *testing.T) {
	skipIfNoDb(t)

	t.Run("known chat without a greetings row", func(t *testing.T) {
		chatID := time.Now().UnixNano()
		cleanupEditorChat(t, chatID)
		if err := chats.EnsureChatInDb(chatID, "editor_known"); err != nil {
			t.Fatalf("EnsureChatInDb() error = %v", err)
		}

		s, err := LoadSettingsContext(context.Background(), chatID)
		if err != nil {
			t.Fatalf("LoadSettingsContext() error = %v", err)
		}
		if !s.WelcomeSettings.ShouldWelcome || s.WelcomeSettings.CleanWelcome {
			t.Fatalf("welcome defaults wrong: %+v", s.WelcomeSettings)
		}
		if !s.GoodbyeSettings.ShouldGoodbye || s.GoodbyeSettings.CleanGoodbye {
			t.Fatalf("goodbye defaults wrong for a known chat: %+v", s.GoodbyeSettings)
		}
		if s.WelcomeSettings.WelcomeText != db.DefaultWelcome || s.GoodbyeSettings.GoodbyeText != db.DefaultGoodbye {
			t.Fatalf("default texts wrong: %q / %q", s.WelcomeSettings.WelcomeText, s.GoodbyeSettings.GoodbyeText)
		}
		if s.WelcomeSettings.WelcomeType != db.TEXT || s.GoodbyeSettings.GoodbyeType != db.TEXT {
			t.Fatalf("default types wrong")
		}
		if len(s.WelcomeSettings.Button) != 0 || len(s.GoodbyeSettings.Button) != 0 {
			t.Fatalf("default buttons must be empty")
		}
		if n := countGreetingRows(t, chatID); n != 0 {
			t.Fatalf("LoadSettingsContext created %d greetings rows", n)
		}
	})

	t.Run("unknown chat", func(t *testing.T) {
		chatID := time.Now().UnixNano()
		cleanupEditorChat(t, chatID)

		s, err := LoadSettingsContext(context.Background(), chatID)
		if err != nil {
			t.Fatalf("LoadSettingsContext() error = %v", err)
		}
		if s.GoodbyeSettings.ShouldGoodbye {
			t.Fatalf("Goodbye must be off for a chat the chats table does not know")
		}
		if n := countGreetingRows(t, chatID); n != 0 {
			t.Fatalf("created %d greetings rows", n)
		}
		if n := countChatRows(t, chatID); n != 0 {
			t.Fatalf("created %d chats rows", n)
		}
	})

	t.Run("empty stored text reads as the default", func(t *testing.T) {
		chatID := time.Now().UnixNano()
		cleanupEditorChat(t, chatID)
		if err := SetWelcomeText(chatID, "x", "", nil, db.TEXT); err != nil {
			t.Fatalf("SetWelcomeText() error = %v", err)
		}
		if err := db.DB.Model(&models.GreetingSettings{}).Where("chat_id = ?", chatID).
			Updates(map[string]any{"welcome_text": "", "goodbye_text": ""}).Error; err != nil {
			t.Fatalf("blank texts: %v", err)
		}

		s, err := LoadSettingsContext(context.Background(), chatID)
		if err != nil {
			t.Fatalf("LoadSettingsContext() error = %v", err)
		}
		if s.WelcomeSettings.WelcomeText != db.DefaultWelcome || s.GoodbyeSettings.GoodbyeText != db.DefaultGoodbye {
			t.Fatalf("empty texts must read as defaults, got %q / %q", s.WelcomeSettings.WelcomeText, s.GoodbyeSettings.GoodbyeText)
		}
	})

	t.Run("a cancelled context is an error, not defaults", func(t *testing.T) {
		chatID := time.Now().UnixNano()
		cleanupEditorChat(t, chatID)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		s, err := LoadSettingsContext(ctx, chatID)
		if err == nil {
			t.Fatalf("expected an error for a cancelled context, got settings %+v", s)
		}
		if s != nil {
			t.Fatalf("expected no settings with an error")
		}
	})
}

func TestLoadGreetingSettingsMatchesRuntime(t *testing.T) {
	skipIfNoDb(t)

	cases := []struct {
		name  string
		known bool
	}{
		{"known chat", true},
		{"unknown chat", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chatID := time.Now().UnixNano()
			cleanupEditorChat(t, chatID)
			if tc.known {
				if err := chats.EnsureChatInDb(chatID, "editor_match"); err != nil {
					t.Fatalf("EnsureChatInDb() error = %v", err)
				}
			}

			page, err := LoadSettingsContext(context.Background(), chatID)
			if err != nil {
				t.Fatalf("LoadSettingsContext() error = %v", err)
			}
			runtime := GetGreetingSettings(chatID)

			pw, rw := page.WelcomeSettings, runtime.WelcomeSettings
			if pw.ShouldWelcome != rw.ShouldWelcome || pw.CleanWelcome != rw.CleanWelcome ||
				pw.WelcomeText != rw.WelcomeText || pw.WelcomeType != rw.WelcomeType ||
				pw.FileID != rw.FileID || len(pw.Button) != len(rw.Button) {
				t.Fatalf("welcome differs: page %+v runtime %+v", pw, rw)
			}
			pg, rg := page.GoodbyeSettings, runtime.GoodbyeSettings
			if pg.ShouldGoodbye != rg.ShouldGoodbye || pg.CleanGoodbye != rg.CleanGoodbye ||
				pg.GoodbyeText != rg.GoodbyeText || pg.GoodbyeType != rg.GoodbyeType ||
				pg.FileID != rg.FileID || len(pg.Button) != len(rg.Button) {
				t.Fatalf("goodbye differs: page %+v runtime %+v", pg, rg)
			}
		})
	}
}
