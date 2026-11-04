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

func TestGreetingColumnWritersKeepOtherParts(t *testing.T) {
	skipIfNoDb(t)

	b1 := models.Button{Name: "one", Url: "https://example.com/1"}
	b2 := models.Button{Name: "two", Url: "https://example.com/2", SameLine: true}

	type part struct {
		text    string
		fileID  string
		kind    int
		buttons models.ButtonArray
	}
	welcome := func(s *models.GreetingSettings) part {
		w := s.WelcomeSettings
		return part{w.WelcomeText, w.FileID, w.WelcomeType, w.Button}
	}
	goodbye := func(s *models.GreetingSettings) part {
		g := s.GoodbyeSettings
		return part{g.GoodbyeText, g.FileID, g.GoodbyeType, g.Button}
	}
	load := func(t *testing.T, chatID int64) *models.GreetingSettings {
		t.Helper()
		s, err := LoadSettingsContext(context.Background(), chatID)
		if err != nil {
			t.Fatalf("LoadSettingsContext() error = %v", err)
		}
		return s
	}
	sameButtons := func(a, b models.ButtonArray) bool {
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}
	samePart := func(a, b part) bool {
		return a.text == b.text && a.fileID == b.fileID && a.kind == b.kind && sameButtons(a.buttons, b.buttons)
	}

	sides := []struct {
		name       string
		full       func(chatID int64, text, fileID string, buttons []models.Button, kind int) error
		textOnly   func(chatID int64, text string) error
		mediaOnly  func(chatID int64, fileID string, kind int) error
		buttonOnly func(chatID int64, buttons []models.Button) error
		mine       func(*models.GreetingSettings) part
		other      func(*models.GreetingSettings) part
	}{
		{"welcome", SetWelcomeText, SetWelcomeTextOnly, SetWelcomeMediaOnly, SetWelcomeButtonsOnly, welcome, goodbye},
		{"goodbye", SetGoodbyeText, SetGoodbyeTextOnly, SetGoodbyeMediaOnly, SetGoodbyeButtonsOnly, goodbye, welcome},
	}

	for _, side := range sides {
		t.Run(side.name, func(t *testing.T) {
			chatID := time.Now().UnixNano()
			cleanupEditorChat(t, chatID)
			if err := side.full(chatID, "<b>a</b>", "file-1", []models.Button{b1}, db.PHOTO); err != nil {
				t.Fatalf("full write error = %v", err)
			}
			otherBefore := side.other(load(t, chatID))

			if err := side.textOnly(chatID, "x"); err != nil {
				t.Fatalf("text only error = %v", err)
			}
			got := side.mine(load(t, chatID))
			want := part{"x", "file-1", db.PHOTO, models.ButtonArray{b1}}
			if !samePart(got, want) {
				t.Fatalf("text only changed other parts: got %+v want %+v", got, want)
			}

			if err := side.mediaOnly(chatID, "file-2", db.VIDEO); err != nil {
				t.Fatalf("media only error = %v", err)
			}
			got = side.mine(load(t, chatID))
			want = part{"x", "file-2", db.VIDEO, models.ButtonArray{b1}}
			if !samePart(got, want) {
				t.Fatalf("media only changed other parts: got %+v want %+v", got, want)
			}

			if err := side.buttonOnly(chatID, []models.Button{b1, b2}); err != nil {
				t.Fatalf("buttons only error = %v", err)
			}
			got = side.mine(load(t, chatID))
			want = part{"x", "file-2", db.VIDEO, models.ButtonArray{b1, b2}}
			if !samePart(got, want) {
				t.Fatalf("two buttons did not round-trip: got %+v want %+v", got, want)
			}

			if err := side.buttonOnly(chatID, nil); err != nil {
				t.Fatalf("clear buttons error = %v", err)
			}
			got = side.mine(load(t, chatID))
			want = part{"x", "file-2", db.VIDEO, models.ButtonArray{}}
			if !samePart(got, want) {
				t.Fatalf("clearing buttons changed other parts: got %+v want %+v", got, want)
			}

			if err := side.mediaOnly(chatID, "", db.TEXT); err != nil {
				t.Fatalf("clear media error = %v", err)
			}
			got = side.mine(load(t, chatID))
			want = part{"x", "", db.TEXT, models.ButtonArray{}}
			if !samePart(got, want) {
				t.Fatalf("clearing media changed other parts: got %+v want %+v", got, want)
			}

			if otherAfter := side.other(load(t, chatID)); !samePart(otherBefore, otherAfter) {
				t.Fatalf("the other side changed: before %+v after %+v", otherBefore, otherAfter)
			}
		})
	}

	t.Run("first write on a chat with no row", func(t *testing.T) {
		chatID := time.Now().UnixNano()
		cleanupEditorChat(t, chatID)

		if err := SetWelcomeTextOnly(chatID, "hello"); err != nil {
			t.Fatalf("SetWelcomeTextOnly() error = %v", err)
		}
		s := load(t, chatID)
		if s.WelcomeSettings.WelcomeText != "hello" {
			t.Fatalf("text not stored: %q", s.WelcomeSettings.WelcomeText)
		}
		if !s.WelcomeSettings.ShouldWelcome {
			t.Fatalf("Welcome must stay on")
		}
		if !s.GoodbyeSettings.ShouldGoodbye {
			t.Fatalf("Goodbye must be on after the first write (database default)")
		}
		if s.GoodbyeSettings.GoodbyeText != db.DefaultGoodbye {
			t.Fatalf("goodbye text changed: %q", s.GoodbyeSettings.GoodbyeText)
		}
	})
}
