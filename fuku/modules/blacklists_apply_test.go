//go:build testtools

package modules

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
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

	t.Run("remove by ID", func(t *testing.T) {
		chatID := blacklistApplyChat(t)
		other := blacklistApplyChat(t)
		blacklistApplySeed(t, chatID, "warn", secretWord, "eggs")
		blacklistApplySeed(t, other, "warn", "foreign")
		rowID := func(chat int64, word string) int64 {
			var row models.BlacklistSettings
			if err := db.DB.Where("chat_id = ? AND word = ?", chat, word).Take(&row).Error; err != nil {
				t.Fatalf("read row id: %v", err)
			}
			return int64(row.ID)
		}
		ownID, foreignID, eggsID := rowID(chatID, secretWord), rowID(other, "foreign"), rowID(chatID, "eggs")

		hook.Reset()
		if got := applyBlacklistRemoveByID(ctx, chatID, foreignID); got.Outcome != blacklistRemoveGone || got.Word != "" {
			t.Errorf("another chat's ID: result = %+v, want gone", got)
		}
		if got := blacklistApplyStoredWords(t, other); !slices.Equal(got, []string{"foreign=warn"}) {
			t.Errorf("another chat's row changed: %q", got)
		}
		got := applyBlacklistRemoveByID(ctx, chatID, ownID)
		if got.Outcome != blacklistRemovedByID || got.Word != secretWord {
			t.Errorf("own ID: result = %+v, want removed with the stored word", got)
		}
		if got := blacklists.GetBlacklistSettingsContext(ctx, chatID).Triggers(); !slices.Equal(got, []string{"eggs"}) {
			t.Errorf("the chat reads %q after the delete, want [eggs]", got)
		}
		if got := applyBlacklistRemoveByID(ctx, chatID, ownID).Outcome; got != blacklistRemoveGone {
			t.Errorf("second delete = %v, want gone", got)
		}

		failing := blacklistWrites
		failing.RemoveByID = func(context.Context, int64, int64) (string, bool, error) {
			return "", false, errors.New("database is down")
		}
		blacklistApplySeams(t, failing)
		if got := applyBlacklistRemoveByID(ctx, chatID, eggsID); got.Outcome != blacklistRemoveByIDSaveFailed || got.Err == nil {
			t.Errorf("failing seam: result = %+v, want save failed with the error", got)
		}
		assertNoWordLogged(t)
	})
}

func TestApplyBlacklistTooBroad(t *testing.T) {
	ctx := context.Background()

	t.Run("apply refuses a one-character word", func(t *testing.T) {
		chatID := blacklistApplyChat(t)
		result := applyBlacklistAdd(ctx, chatID, []string{"a", "spam", "🙂"}, nil)
		if !slices.Equal(result.TooBroad, []string{"a"}) {
			t.Errorf("too broad = %q, want [a]", result.TooBroad)
		}
		if !slices.Equal(result.Added, []string{"spam", "🙂"}) {
			t.Errorf("added = %q, want [spam 🙂]", result.Added)
		}
		if got := blacklistApplyStoredWords(t, chatID); !slices.Equal(got, []string{"spam=warn", "🙂=warn"}) {
			t.Errorf("stored = %q", got)
		}
	})

	t.Run("apply refuses a too broad word even when it is already listed", func(t *testing.T) {
		chatID := blacklistApplyChat(t)
		result := applyBlacklistAdd(ctx, chatID, []string{"a"}, models.BlacklistSettingsSlice{{Word: "a"}})
		if !slices.Equal(result.TooBroad, []string{"a"}) || len(result.Already) != 0 || result.Outcome != blacklistNothingAdded {
			t.Errorf("result = %+v, want only a too-broad word", result)
		}
	})

	t.Run("the command replies the too-broad line then the usual one", func(t *testing.T) {
		withRealLocales(t)
		chatID := blacklistApplyChat(t)
		client := newModuleBotClient()
		bot := newModuleTestBot(client)
		chat := gotgbot.Chat{Id: chatID, Type: "supergroup", Title: "Blacklist Apply"}
		admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}
		run := func(text string) []string {
			before := len(client.callsFor("sendMessage"))
			if err := blacklistsModule.addBlacklist(bot, newModuleMessageContext(bot, chat, admin, text)); err != ext.EndGroups {
				t.Fatalf("addBlacklist error = %v, want EndGroups", err)
			}
			var texts []string
			for _, call := range client.callsFor("sendMessage")[before:] {
				texts = append(texts, call.Params["text"].(string))
			}
			return texts
		}

		texts := run("/addblacklist a spam")
		if len(texts) != 2 || !strings.Contains(texts[0], ": a") || !strings.Contains(texts[1], "spam") {
			t.Fatalf("replies = %q, want the too-broad line naming a, then the added list", texts)
		}
		texts = run("/addblacklist <")
		if len(texts) != 1 || !strings.Contains(texts[0], "&lt;") {
			t.Fatalf("replies = %q, want one too-broad reply with the word escaped", texts)
		}
		if got := blacklistApplyStoredWords(t, chatID); !slices.Equal(got, []string{"spam=warn"}) {
			t.Errorf("stored = %q, want [spam=warn]", got)
		}
	})
}
