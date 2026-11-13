//go:build testtools

package modules

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	logrustest "github.com/sirupsen/logrus/hooks/test"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/blacklists"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// blacklistApplyChat returns a fresh chat ID whose blacklist rows and cache entry are
// deleted before the test and when it ends.
func blacklistApplyChat(t *testing.T) int64 {
	t.Helper()
	chatID := uniqueModuleChatID()
	clean := func() {
		db.DB.Where("chat_id = ?", chatID).Delete(&models.BlacklistSettings{})
		dbcache.DeleteCache(dbcache.CacheKey("blacklist", chatID))
		dbcache.ResetLocalForTest()
	}
	clean()
	t.Cleanup(clean)
	return chatID
}

// blacklistApplySeams swaps the write seams for the test and restores them when it ends.
func blacklistApplySeams(t *testing.T, seams blacklistWriteSeams) {
	t.Helper()
	previous := blacklistWrites
	blacklistWrites = seams
	t.Cleanup(func() { blacklistWrites = previous })
}

// blacklistApplyStoredWords reads the chat's stored words straight from the table, sorted.
func blacklistApplyStoredWords(t *testing.T, chatID int64) []string {
	t.Helper()
	var rows []models.BlacklistSettings
	if err := db.DB.Where("chat_id = ?", chatID).Find(&rows).Error; err != nil {
		t.Fatalf("read blacklists: %v", err)
	}
	words := make([]string, 0, len(rows))
	for _, row := range rows {
		words = append(words, row.Word+"="+row.Action)
	}
	slices.Sort(words)
	return words
}

func blacklistApplySeed(t *testing.T, chatID int64, action string, words ...string) {
	t.Helper()
	for _, word := range words {
		if err := blacklists.AddBlacklistContext(context.Background(), chatID, word, action); err != nil {
			t.Fatalf("seed %q: %v", word, err)
		}
	}
}

func TestApplyBlacklist(t *testing.T) {
	ctx := context.Background()
	hook := logrustest.NewGlobal()
	t.Cleanup(hook.Reset)
	const secretWord = "zzsecretwordzz"
	assertNoWordLogged := func(t *testing.T) {
		t.Helper()
		for _, entry := range hook.AllEntries() {
			if strings.Contains(strings.ToLower(entry.Message), secretWord) {
				t.Errorf("log line carries a word: %q", entry.Message)
			}
			for key, value := range entry.Data {
				if strings.Contains(strings.ToLower(key), "word") {
					t.Errorf("log field %q names a word", key)
				}
				if text, ok := value.(string); ok && strings.Contains(strings.ToLower(text), secretWord) {
					t.Errorf("log field %q carries a word", key)
				}
			}
		}
	}

	t.Run("add sorts words", func(t *testing.T) {
		chatID := blacklistApplyChat(t)
		failing := blacklistWrites
		failing.Add = func(ctx context.Context, chat int64, word, action string) error {
			if word == secretWord {
				return errors.New("database is down")
			}
			return blacklists.AddBlacklistContext(ctx, chat, word, action)
		}
		blacklistApplySeams(t, failing)
		hook.Reset()

		words := []string{"SpAm", "spam", strings.Repeat("x", 101), strings.ToUpper(secretWord), "eggs"}
		result := applyBlacklistAdd(ctx, chatID, words, models.BlacklistSettingsSlice{{Word: "eggs"}})
		if result.Outcome != blacklistAdded {
			t.Errorf("outcome = %v, want applied", result.Outcome)
		}
		if !slices.Equal(result.Added, []string{"spam"}) {
			t.Errorf("added = %q, want [spam]", result.Added)
		}
		if !slices.Equal(result.Already, []string{"spam", "eggs"}) {
			t.Errorf("already = %q, want [spam eggs]", result.Already)
		}
		if want := strings.Repeat("x", 20) + "..."; !slices.Equal(result.TooLong, []string{want}) {
			t.Errorf("too long = %q, want [%s]", result.TooLong, want)
		}
		if result.Failed != 1 {
			t.Errorf("failed = %d, want 1", result.Failed)
		}
		if got := blacklistApplyStoredWords(t, chatID); !slices.Equal(got, []string{"spam=warn"}) {
			t.Errorf("stored = %q, want [spam=warn]", got)
		}
		assertNoWordLogged(t)
	})

	t.Run("add outcomes", func(t *testing.T) {
		chatID := blacklistApplyChat(t)
		if got := applyBlacklistAdd(ctx, chatID, []string{"spam"}, models.BlacklistSettingsSlice{{Word: "spam"}}).Outcome; got != blacklistNothingAdded {
			t.Errorf("outcome of an already listed word = %v, want nothing added", got)
		}
		blacklistApplySeams(t, blacklistWriteSeams{
			Add: func(context.Context, int64, string, string) error { return errors.New("database is down") },
		})
		if got := applyBlacklistAdd(ctx, chatID, []string{"spam"}, nil).Outcome; got != blacklistAddSaveFailed {
			t.Errorf("outcome of a failed write = %v, want save failed", got)
		}
	})

	t.Run("remove only listed words", func(t *testing.T) {
		chatID := blacklistApplyChat(t)
		blacklistApplySeed(t, chatID, "warn", "spam", "eggs", secretWord)
		failing := blacklistWrites
		failing.Remove = func(ctx context.Context, chat int64, word string) error {
			if word == secretWord {
				return errors.New("database is down")
			}
			return blacklists.RemoveBlacklistContext(ctx, chat, word)
		}
		blacklistApplySeams(t, failing)
		hook.Reset()

		existing := blacklists.GetBlacklistSettingsContext(ctx, chatID).Triggers()
		result := applyBlacklistRemove(ctx, chatID, []string{"SPAM", "ghost", strings.ToUpper(secretWord)}, existing)
		if !slices.Equal(result.Removed, []string{"spam"}) {
			t.Errorf("removed = %q, want [spam]", result.Removed)
		}
		if result.Failed != 1 {
			t.Errorf("failed = %d, want 1", result.Failed)
		}
		if got := blacklistApplyStoredWords(t, chatID); !slices.Equal(got, []string{"eggs=warn", secretWord + "=warn"}) {
			t.Errorf("stored = %q", got)
		}
		assertNoWordLogged(t)
	})

	t.Run("action", func(t *testing.T) {
		chatID := blacklistApplyChat(t)
		blacklistApplySeed(t, chatID, "warn", "spam", "eggs")
		for _, action := range blacklistActionValues {
			result := applyBlacklistAction(ctx, chatID, action)
			if result.Outcome != blacklistActionApplied || result.Rows != 2 {
				t.Errorf("%s: result = %+v, want applied over 2 rows", action, result)
			}
			if got := blacklistApplyStoredWords(t, chatID); !slices.Equal(got, []string{"eggs=" + action, "spam=" + action}) {
				t.Errorf("%s: stored = %q", action, got)
			}
		}
		for _, action := range []string{"Ban", "tban", "delete", ""} {
			if got := applyBlacklistAction(ctx, chatID, action).Outcome; got != blacklistActionUnknown {
				t.Errorf("%q: outcome = %v, want unknown", action, got)
			}
		}
		if got := blacklistApplyStoredWords(t, chatID); !slices.Equal(got, []string{"eggs=none", "spam=none"}) {
			t.Errorf("unknown actions changed the rows: %q", got)
		}
		empty := blacklistApplyChat(t)
		if result := applyBlacklistAction(ctx, empty, "ban"); result.Outcome != blacklistActionNoWords || result.Rows != 0 {
			t.Errorf("empty chat: result = %+v, want no words over 0 rows", result)
		}
		blacklistApplySeams(t, blacklistWriteSeams{
			SetAction: func(context.Context, int64, string) (int64, error) { return 0, errors.New("database is down") },
		})
		result := applyBlacklistAction(ctx, chatID, "ban")
		if result.Outcome != blacklistActionSaveFailed || result.Err == nil {
			t.Errorf("failing seam: result = %+v, want save failed with the error", result)
		}
	})
}
