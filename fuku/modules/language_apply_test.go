//go:build testtools

package modules

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/uasneppy/fuku_robot/fuku/db"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/chats"
	"github.com/uasneppy/fuku_robot/fuku/db/lang"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/utils/keyboard"
)

// languageApplyKeys are the cache keys lang.ChangeGroupLanguage drops after a write that a test
// can fill with a stale marker. The third, chat, holds a typed entry that chats.GetChatSettings
// reads, so it is proved through that reader instead (languageApplyChatRead).
func languageApplyKeys(chatID int64) []string {
	return []string{
		dbcache.CacheKey("chat_lang", chatID),
		dbcache.CacheKey("chat_settings", chatID),
	}
}

// languageApplyChatRead is the language the cached chat entry reads now.
func languageApplyChatRead(chatID int64) string {
	return chats.GetChatSettings(chatID).Language
}

// languageApplyPrime fills the stale-marker keys and the chat entry, which it reads once.
func languageApplyPrime(t *testing.T, chatID int64) {
	t.Helper()
	for _, key := range languageApplyKeys(chatID) {
		got, err := dbcache.GetFromCacheOrLoad(context.Background(), key, time.Minute, func(context.Context) (string, error) {
			return "stale", nil
		})
		if err != nil || got != "stale" {
			t.Fatalf("prime %s = %q, %v", key, got, err)
		}
	}
	languageApplyChatRead(chatID)
}

// languageApplyDropped reports, for each key, whether the stale marker is gone: a reload that
// runs its loader proves the key was dropped.
func languageApplyDropped(t *testing.T, chatID int64) map[string]bool {
	t.Helper()
	dropped := make(map[string]bool)
	for _, key := range languageApplyKeys(chatID) {
		got, err := dbcache.GetFromCacheOrLoad(context.Background(), key, time.Minute, func(context.Context) (string, error) {
			return "fresh", nil
		})
		if err != nil {
			t.Fatalf("probe %s: %v", key, err)
		}
		dropped[key] = got == "fresh"
	}
	return dropped
}

func languageApplyStored(t *testing.T, chatID int64) string {
	t.Helper()
	var row models.Chat
	if err := db.DB.Where("chat_id = ?", chatID).First(&row).Error; err != nil {
		t.Fatalf("read chats row: %v", err)
	}
	return row.Language
}

func TestApplyGroupLanguage(t *testing.T) {
	withRealLocales(t)

	newChat := func(t *testing.T) int64 {
		t.Helper()
		chatID := uniqueModuleChatID()
		languageGoldenCleanup(t, []int64{chatID}, nil)
		return chatID
	}

	t.Run("every code is written and drops the three keys", func(t *testing.T) {
		for _, code := range groupLanguageCodes {
			// The repository returns early, with no write and no cache drop, for the code that is
			// already stored, so each code is applied over a different stored one.
			seed := "en"
			if code == "en" {
				seed = "uk"
			}
			chatID := newChat(t)
			if err := lang.ChangeGroupLanguage(chatID, seed); err != nil {
				t.Fatalf("seed %s: %v", seed, err)
			}
			languageApplyPrime(t, chatID)

			result := applyGroupLanguage(context.Background(), chatID, code)
			if result.Outcome != langApplied || result.Err != nil {
				t.Fatalf("%s: outcome %v, err %v; want langApplied", code, result.Outcome, result.Err)
			}
			if result.Key != "language_changed_group" {
				t.Errorf("%s: key = %q, want language_changed_group", code, result.Key)
			}
			if got := result.Params["s"]; got != keyboard.GetLangFormat(code) {
				t.Errorf("%s: parameter s = %v, want %q", code, got, keyboard.GetLangFormat(code))
			}
			if got := languageApplyStored(t, chatID); got != code {
				t.Errorf("%s: stored language = %q", code, got)
			}
			for key, dropped := range languageApplyDropped(t, chatID) {
				if !dropped {
					t.Errorf("%s: cache key %s was not dropped", code, key)
				}
			}
			if got := languageApplyChatRead(chatID); got != code {
				t.Errorf("%s: the chat cache entry still reads %q", code, got)
			}
		}
	})

	t.Run("the stored code again writes nothing and drops nothing", func(t *testing.T) {
		chatID := newChat(t)
		if err := lang.ChangeGroupLanguage(chatID, "es"); err != nil {
			t.Fatalf("seed: %v", err)
		}
		languageApplyPrime(t, chatID)
		result := applyGroupLanguage(context.Background(), chatID, "es")
		if result.Outcome != langApplied || result.Err != nil {
			t.Fatalf("outcome %v, err %v; want langApplied", result.Outcome, result.Err)
		}
		for key, dropped := range languageApplyDropped(t, chatID) {
			if dropped {
				t.Errorf("cache key %s was dropped by a no-op write", key)
			}
		}
		if got := languageApplyStored(t, chatID); got != "es" {
			t.Errorf("stored language = %q, want es", got)
		}
	})

	t.Run("a code outside the eight is refused before any write", func(t *testing.T) {
		writes := 0
		mreqSwap(t, &languageWrites, func(int64, string) error { writes++; return nil })
		for _, code := range []string{"de", "UK", "", "ukr", "en "} {
			chatID := newChat(t)
			result := applyGroupLanguage(context.Background(), chatID, code)
			if result.Outcome != langUnsupported || result.Key != "language_invalid_selection" {
				t.Errorf("%q: outcome %v key %q; want langUnsupported language_invalid_selection", code, result.Outcome, result.Key)
			}
		}
		if writes != 0 {
			t.Errorf("write seam called %d times for unsupported codes, want 0", writes)
		}
	})

	t.Run("a failed write is langSaveFailed with the error", func(t *testing.T) {
		boom := errors.New("database is down")
		mreqSwap(t, &languageWrites, func(int64, string) error { return boom })
		result := applyGroupLanguage(context.Background(), newChat(t), "es")
		if result.Outcome != langSaveFailed || !errors.Is(result.Err, boom) {
			t.Fatalf("outcome %v, err %v; want langSaveFailed with the write error", result.Outcome, result.Err)
		}
		if result.Key != "common_settings_save_failed" {
			t.Errorf("key = %q, want common_settings_save_failed", result.Key)
		}
	})

	t.Run("a cancelled context writes nothing", func(t *testing.T) {
		writes := 0
		mreqSwap(t, &languageWrites, func(int64, string) error { writes++; return nil })
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		result := applyGroupLanguage(ctx, newChat(t), "es")
		if result.Outcome != langSaveFailed || writes != 0 {
			t.Fatalf("outcome %v with %d writes; want langSaveFailed and none", result.Outcome, writes)
		}
	})
}
