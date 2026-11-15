package rules

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/chats"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

func cleanupRulesChat(t *testing.T, chatID int64) {
	t.Helper()
	t.Cleanup(func() {
		_ = db.DB.Where("chat_id = ?", chatID).Delete(&models.RulesSettings{}).Error
		_ = db.DB.Where("chat_id = ?", chatID).Delete(&models.Chat{}).Error
	})
}

func loadRulesRow(t *testing.T, chatID int64) models.RulesSettings {
	t.Helper()
	row, err := LoadRulesSettingsContext(context.Background(), chatID)
	if err != nil {
		t.Fatalf("LoadRulesSettingsContext() error = %v", err)
	}
	return *row
}

func storedButtonsJSON(t *testing.T, chatID int64) string {
	t.Helper()
	var raw string
	if err := db.DB.Raw("SELECT CAST(btns AS TEXT) FROM rules WHERE chat_id = ?", chatID).Scan(&raw).Error; err != nil {
		t.Fatalf("read stored buttons: %v", err)
	}
	return raw
}

// sameRules compares two rows on every column the editor writes, leaving out the row ID
// and the timestamps.
func sameRules(a, b models.RulesSettings) bool {
	a.ID, b.ID = 0, 0
	a.CreatedAt, b.CreatedAt = time.Time{}, time.Time{}
	a.UpdatedAt, b.UpdatedAt = time.Time{}, time.Time{}
	if len(a.Buttons) == 0 && len(b.Buttons) == 0 {
		a.Buttons, b.Buttons = nil, nil
	}
	return reflect.DeepEqual(a, b)
}

// seedRulesRow stores a rules row with every column set to something other than its zero.
func seedRulesRow(t *testing.T, chatID int64) models.RulesSettings {
	t.Helper()
	cleanupRulesChat(t, chatID)
	if err := SetChatRules(chatID, "be kind"); err != nil {
		t.Fatalf("SetChatRules() error = %v", err)
	}
	if err := SetChatRulesButton(chatID, "Read"); err != nil {
		t.Fatalf("SetChatRulesButton() error = %v", err)
	}
	if err := SetPrivateRules(chatID, true); err != nil {
		t.Fatalf("SetPrivateRules() error = %v", err)
	}
	return loadRulesRow(t, chatID)
}

func TestLoadRulesCreatesNothing(t *testing.T) {
	skipIfNoDb(t)

	count := func(model any, chatID int64) int64 {
		t.Helper()
		var n int64
		if err := db.DB.Model(model).Where("chat_id = ?", chatID).Count(&n).Error; err != nil {
			t.Fatalf("count rows: %v", err)
		}
		return n
	}

	t.Run("no rules row is empty text and no error", func(t *testing.T) {
		chatID := time.Now().UnixNano()

		got, err := LoadRulesContext(context.Background(), chatID)
		if err != nil {
			t.Fatalf("LoadRulesContext() error = %v", err)
		}
		if got != "" {
			t.Fatalf("expected empty rules, got %q", got)
		}
		if n := count(&models.RulesSettings{}, chatID); n != 0 {
			t.Fatalf("created %d rules rows", n)
		}
		if n := count(&models.Chat{}, chatID); n != 0 {
			t.Fatalf("created %d chats rows", n)
		}
	})

	t.Run("stored text is returned after SetChatRules", func(t *testing.T) {
		chatID := time.Now().UnixNano()
		t.Cleanup(func() {
			_ = db.DB.Where("chat_id = ?", chatID).Delete(&models.RulesSettings{}).Error
			_ = db.DB.Where("chat_id = ?", chatID).Delete(&models.Chat{}).Error
		})
		if err := chats.EnsureChatInDb(chatID, ""); err != nil {
			t.Fatalf("EnsureChatInDb() error = %v", err)
		}
		if err := SetChatRules(chatID, "be kind"); err != nil {
			t.Fatalf("SetChatRules() error = %v", err)
		}

		got, err := LoadRulesContext(context.Background(), chatID)
		if err != nil {
			t.Fatalf("LoadRulesContext() error = %v", err)
		}
		if got != "be kind" {
			t.Fatalf("expected stored rules, got %q", got)
		}
	})

	t.Run("a cancelled context is an error", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		got, err := LoadRulesContext(ctx, time.Now().UnixNano())
		if err == nil {
			t.Fatalf("expected an error for a cancelled context, got %q", got)
		}
		if got != "" {
			t.Fatalf("expected no text with an error, got %q", got)
		}
	})
}

func TestLoadRulesSettingsCreatesNothing(t *testing.T) {
	skipIfNoDb(t)

	count := func(model any, chatID int64) int64 {
		t.Helper()
		var n int64
		if err := db.DB.Model(model).Where("chat_id = ?", chatID).Count(&n).Error; err != nil {
			t.Fatalf("count rows: %v", err)
		}
		return n
	}

	t.Run("no row reads a zero row and no error", func(t *testing.T) {
		chatID := time.Now().UnixNano()

		got, err := LoadRulesSettingsContext(context.Background(), chatID)
		if err != nil {
			t.Fatalf("LoadRulesSettingsContext() error = %v", err)
		}
		if got == nil || got.ChatId != chatID || got.HasContent() || got.MediaBelow || got.Private {
			t.Fatalf("expected a zero row for the chat, got %+v", got)
		}
		if n := count(&models.RulesSettings{}, chatID); n != 0 {
			t.Fatalf("created %d rules rows", n)
		}
		if n := count(&models.Chat{}, chatID); n != 0 {
			t.Fatalf("created %d chats rows", n)
		}
	})

	t.Run("a stored row reads with all its columns", func(t *testing.T) {
		chatID := time.Now().UnixNano()
		seedRulesRow(t, chatID)
		buttons := []models.Button{{Name: "one", Url: "https://example.com/1"}, {Name: "two", Url: "https://example.com/2", SameLine: true}}
		if err := SetRulesMediaOnly(chatID, "file-9", db.PHOTO); err != nil {
			t.Fatalf("SetRulesMediaOnly() error = %v", err)
		}
		if err := SetRulesButtonsOnly(chatID, buttons); err != nil {
			t.Fatalf("SetRulesButtonsOnly() error = %v", err)
		}
		if err := SetRulesMediaBelow(chatID, true); err != nil {
			t.Fatalf("SetRulesMediaBelow() error = %v", err)
		}

		got := loadRulesRow(t, chatID)
		if got.Rules != "be kind" || got.RulesBtn != "Read" || !got.Private ||
			got.FileID != "file-9" || got.MediaType != db.PHOTO || !got.MediaBelow ||
			!reflect.DeepEqual([]models.Button(got.Buttons), buttons) {
			t.Fatalf("stored row read back wrong: %+v", got)
		}
	})

	t.Run("a cancelled context is an error", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		got, err := LoadRulesSettingsContext(ctx, time.Now().UnixNano())
		if err == nil {
			t.Fatalf("expected an error for a cancelled context, got %+v", got)
		}
	})

	t.Run("LoadRulesContext still returns the text", func(t *testing.T) {
		chatID := time.Now().UnixNano()
		seedRulesRow(t, chatID)

		got, err := LoadRulesContext(context.Background(), chatID)
		if err != nil || got != "be kind" {
			t.Fatalf("LoadRulesContext() = %q, %v", got, err)
		}
	})
}

func TestRulesMediaWriters(t *testing.T) {
	skipIfNoDb(t)

	buttons := []models.Button{{Name: "one", Url: "https://example.com/1"}, {Name: "two", Url: "https://example.com/2", SameLine: true}}

	t.Run("media only keeps every other part", func(t *testing.T) {
		chatID := time.Now().UnixNano()
		seedRulesRow(t, chatID)
		if err := SetRulesButtonsOnly(chatID, buttons); err != nil {
			t.Fatalf("SetRulesButtonsOnly() error = %v", err)
		}
		if err := SetRulesMediaBelow(chatID, true); err != nil {
			t.Fatalf("SetRulesMediaBelow() error = %v", err)
		}
		before := loadRulesRow(t, chatID)

		if err := SetRulesMediaOnly(chatID, "f", db.PHOTO); err != nil {
			t.Fatalf("SetRulesMediaOnly() error = %v", err)
		}
		want := before
		want.FileID, want.MediaType = "f", db.PHOTO
		if got := loadRulesRow(t, chatID); !sameRules(got, want) {
			t.Fatalf("media write changed another column:\nwant %+v\ngot  %+v", want, got)
		}
	})

	t.Run("buttons only keeps the media", func(t *testing.T) {
		chatID := time.Now().UnixNano()
		seedRulesRow(t, chatID)
		if err := SetRulesMediaOnly(chatID, "f", db.PHOTO); err != nil {
			t.Fatalf("SetRulesMediaOnly() error = %v", err)
		}
		before := loadRulesRow(t, chatID)

		if err := SetRulesButtonsOnly(chatID, buttons); err != nil {
			t.Fatalf("SetRulesButtonsOnly() error = %v", err)
		}
		want := before
		want.Buttons = models.ButtonArray(buttons)
		if got := loadRulesRow(t, chatID); !sameRules(got, want) {
			t.Fatalf("buttons write changed another column:\nwant %+v\ngot  %+v", want, got)
		}

		if err := SetRulesButtonsOnly(chatID, nil); err != nil {
			t.Fatalf("SetRulesButtonsOnly(nil) error = %v", err)
		}
		if got := storedButtonsJSON(t, chatID); got != "[]" {
			t.Fatalf("expected an empty array stored, got %q", got)
		}
		if got := loadRulesRow(t, chatID); got.FileID != "f" || len(got.Buttons) != 0 {
			t.Fatalf("clearing the buttons changed the media or kept buttons: %+v", got)
		}
	})

	t.Run("media below stores true and false", func(t *testing.T) {
		chatID := time.Now().UnixNano()
		seedRulesRow(t, chatID)

		for _, on := range []bool{true, false} {
			if err := SetRulesMediaBelow(chatID, on); err != nil {
				t.Fatalf("SetRulesMediaBelow(%v) error = %v", on, err)
			}
			if got := loadRulesRow(t, chatID); got.MediaBelow != on {
				t.Fatalf("after SetRulesMediaBelow(%v) the column reads %v", on, got.MediaBelow)
			}
		}
	})

	t.Run("media only with an empty file ID clears the media", func(t *testing.T) {
		chatID := time.Now().UnixNano()
		seedRulesRow(t, chatID)
		if err := SetRulesMediaOnly(chatID, "f", db.PHOTO); err != nil {
			t.Fatalf("SetRulesMediaOnly() error = %v", err)
		}

		if err := SetRulesMediaOnly(chatID, "", 0); err != nil {
			t.Fatalf("SetRulesMediaOnly(clear) error = %v", err)
		}
		if got := loadRulesRow(t, chatID); got.FileID != "" || got.MediaType != 0 || got.Rules != "be kind" {
			t.Fatalf("media not cleared or text lost: %+v", got)
		}
	})

	t.Run("a writer on a chat with no row creates the row", func(t *testing.T) {
		chatID := time.Now().UnixNano()
		cleanupRulesChat(t, chatID)

		if err := SetRulesMediaOnly(chatID, "f", db.PHOTO); err != nil {
			t.Fatalf("SetRulesMediaOnly() error = %v", err)
		}
		if got := loadRulesRow(t, chatID); got.FileID != "f" || got.MediaType != db.PHOTO {
			t.Fatalf("media not stored on a fresh chat: %+v", got)
		}
	})
}

func TestClearRulesStoresZeroes(t *testing.T) {
	skipIfNoDb(t)

	chatID := time.Now().UnixNano()
	seedRulesRow(t, chatID)
	if err := SetRulesMediaOnly(chatID, "f", db.PHOTO); err != nil {
		t.Fatalf("SetRulesMediaOnly() error = %v", err)
	}
	if err := SetRulesButtonsOnly(chatID, []models.Button{{Name: "one", Url: "https://example.com/1"}}); err != nil {
		t.Fatalf("SetRulesButtonsOnly() error = %v", err)
	}
	if err := SetRulesMediaBelow(chatID, true); err != nil {
		t.Fatalf("SetRulesMediaBelow() error = %v", err)
	}
	before := loadRulesRow(t, chatID)
	if !before.HasContent() {
		t.Fatal("the seeded row should have content")
	}

	if err := ClearRules(chatID); err != nil {
		t.Fatalf("ClearRules() error = %v", err)
	}

	got := loadRulesRow(t, chatID)
	if got.Rules != "" || got.FileID != "" || got.MediaType != 0 || len(got.Buttons) != 0 {
		t.Fatalf("text, media or buttons not cleared: %+v", got)
	}
	if got.RulesBtn != before.RulesBtn || got.Private != before.Private || got.MediaBelow != before.MediaBelow {
		t.Fatalf("ClearRules touched rules_btn, private or media_below: before %+v, after %+v", before, got)
	}
	if got.HasContent() {
		t.Fatal("a cleared row still has content")
	}
	if raw := storedButtonsJSON(t, chatID); raw != "[]" {
		t.Fatalf("expected an empty array stored, got %q", raw)
	}
}

func TestSetMediaBelowColumnTargeted(t *testing.T) {
	skipIfNoDb(t)

	chatID := time.Now().UnixNano()
	seedRulesRow(t, chatID)
	if err := SetRulesMediaOnly(chatID, "f", db.PHOTO); err != nil {
		t.Fatalf("SetRulesMediaOnly() error = %v", err)
	}
	if err := SetRulesButtonsOnly(chatID, []models.Button{{Name: "one", Url: "https://example.com/1"}}); err != nil {
		t.Fatalf("SetRulesButtonsOnly() error = %v", err)
	}
	before := loadRulesRow(t, chatID)

	for _, on := range []bool{true, false} {
		if err := SetRulesMediaBelow(chatID, on); err != nil {
			t.Fatalf("SetRulesMediaBelow(%v) error = %v", on, err)
		}
		want := before
		want.MediaBelow = on
		if got := loadRulesRow(t, chatID); !sameRules(got, want) {
			t.Fatalf("SetRulesMediaBelow(%v) changed another column:\nwant %+v\ngot  %+v", on, want, got)
		}
	}
}
