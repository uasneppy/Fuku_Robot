package greetings

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	gocache "github.com/eko/gocache/lib/v4/cache"
	"github.com/eko/gocache/lib/v4/marshaler"
	gocache_store "github.com/eko/gocache/store/redis/v4"
	"github.com/redis/go-redis/v9"

	"github.com/uasneppy/fuku_robot/fuku/config"
	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/chats"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	utilsCache "github.com/uasneppy/fuku_robot/fuku/utils/cache"
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

// useMiniredisCache wires the real cache stack against miniredis for one test, so a test
// can seed a cache key and see a write delete it.
func useMiniredisCache(t *testing.T) *miniredis.Miniredis {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	manager := gocache.New[any](gocache_store.NewRedis(client))
	previousMarshal, previousManager, previousClient := utilsCache.GetCacheState()
	utilsCache.SetCacheState(marshaler.New(manager), manager, client)

	previousConfig := config.AppConfig
	cfg := config.Config{}
	if previousConfig != nil {
		cfg = *previousConfig
	}
	cfg.DisableCache = false
	config.AppConfig = &cfg
	cache.ResetLocalForTest()

	t.Cleanup(func() {
		config.AppConfig = previousConfig
		cache.ResetLocalForTest()
		_ = client.Close()
		utilsCache.SetCacheState(previousMarshal, previousManager, previousClient)
	})
	return mr
}

func TestSetMediaBelowColumnTargeted(t *testing.T) {
	skipIfNoDb(t)
	mr := useMiniredisCache(t)

	b1 := models.Button{Name: "one", Url: "https://example.com/1"}
	load := func(t *testing.T, chatID int64) models.GreetingSettings {
		t.Helper()
		s, err := LoadSettingsContext(context.Background(), chatID)
		if err != nil {
			t.Fatalf("LoadSettingsContext() error = %v", err)
		}
		return *s
	}
	// rest is the row with the one column under test and the timestamps cleared, so two
	// rows compare equal exactly when every other column is unchanged.
	rest := func(s models.GreetingSettings, clear func(*models.GreetingSettings)) models.GreetingSettings {
		w, g := *s.WelcomeSettings, *s.GoodbyeSettings
		s.WelcomeSettings, s.GoodbyeSettings = &w, &g
		s.CreatedAt, s.UpdatedAt = time.Time{}, time.Time{}
		clear(&s)
		return s
	}

	writers := []struct {
		name  string
		write func(chatID int64, on bool) error
		get   func(*models.GreetingSettings) bool
		clear func(*models.GreetingSettings)
	}{
		{"welcome media below", SetWelcomeMediaBelow,
			func(s *models.GreetingSettings) bool { return s.WelcomeSettings.MediaBelow },
			func(s *models.GreetingSettings) { s.WelcomeSettings.MediaBelow = false }},
		{"goodbye media below", SetGoodbyeMediaBelow,
			func(s *models.GreetingSettings) bool { return s.GoodbyeSettings.MediaBelow },
			func(s *models.GreetingSettings) { s.GoodbyeSettings.MediaBelow = false }},
		{"welcome once", SetWelcomeOnce,
			func(s *models.GreetingSettings) bool { return s.WelcomeSettings.Once },
			func(s *models.GreetingSettings) { s.WelcomeSettings.Once = false }},
	}

	for _, w := range writers {
		t.Run(w.name, func(t *testing.T) {
			chatID := time.Now().UnixNano()
			cleanupEditorChat(t, chatID)
			key := cache.CacheKey("greetings", chatID)

			if err := SetWelcomeText(chatID, "<b>hi</b>", "file-1", []models.Button{b1}, db.PHOTO); err != nil {
				t.Fatalf("SetWelcomeText() error = %v", err)
			}
			if err := SetGoodbyeText(chatID, "bye", "file-2", []models.Button{b1}, db.PHOTO); err != nil {
				t.Fatalf("SetGoodbyeText() error = %v", err)
			}
			if err := SetWelcomeToggle(chatID, false); err != nil {
				t.Fatalf("SetWelcomeToggle() error = %v", err)
			}
			if err := SetCleanWelcomeSetting(chatID, true); err != nil {
				t.Fatalf("SetCleanWelcomeSetting() error = %v", err)
			}
			before := rest(load(t, chatID), w.clear)

			for _, on := range []bool{true, false} {
				if err := mr.Set(key, "stale"); err != nil {
					t.Fatalf("seed cache key: %v", err)
				}
				if err := w.write(chatID, on); err != nil {
					t.Fatalf("write(%v) error = %v", on, err)
				}
				if mr.Exists(key) {
					t.Fatalf("write(%v) left the greetings cache key behind", on)
				}
				got := load(t, chatID)
				if w.get(&got) != on {
					t.Fatalf("after write(%v) the column reads %v", on, w.get(&got))
				}
				if after := rest(got, w.clear); !reflect.DeepEqual(before, after) {
					t.Fatalf("write(%v) changed another column:\nbefore %+v\nafter  %+v", on, before, after)
				}
			}
		})

		t.Run(w.name+" on a fresh chat", func(t *testing.T) {
			chatID := time.Now().UnixNano()
			cleanupEditorChat(t, chatID)

			if err := w.write(chatID, true); err != nil {
				t.Fatalf("write(true) error = %v", err)
			}
			if n := countGreetingRows(t, chatID); n != 1 {
				t.Fatalf("expected one greetings row, got %d", n)
			}
			got := load(t, chatID)
			if !w.get(&got) {
				t.Fatal("the column was not stored on a fresh chat")
			}
		})
	}
}
