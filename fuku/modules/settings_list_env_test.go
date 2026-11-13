//go:build testtools

package modules

import (
	"context"
	"fmt"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/blacklists"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// settingsListSentinel is the word the privacy tests seed and then search every group-menu edit and answer for: it
// must never appear where a member can read.
const settingsListSentinel = "zzsentinelword"

// settingsListEnv is the toolkit of the list pages' tests: the settings environment with the blacklist commands
// loaded on its dispatcher (so a parity or drift test can run /addblacklist and /rmblacklist), fresh chats cleaned up
// with their list rows and cache keys, and helpers that seed a list and open a page in a private menu.
type settingsListEnv struct {
	*settingsEnv
}

// newSettingsListEnv builds the environment and cleans the env chat's list rows before and after the test.
func newSettingsListEnv(t *testing.T) *settingsListEnv {
	t.Helper()
	env := &settingsListEnv{settingsEnv: newSettingsEnv(t)}
	LoadBlacklists(env.dispatcher)
	settingsListCleanup(t, env.chat.Id)
	return env
}

// settingsListCleanup deletes the list rows of the chats (blacklists and filters) and their cached copies before the
// test and again when it ends, then resets the in-process cache layer.
func settingsListCleanup(t *testing.T, chatIDs ...int64) {
	t.Helper()
	clean := func() {
		if len(chatIDs) == 0 {
			return
		}
		db.DB.Where("chat_id IN ?", chatIDs).Delete(&models.BlacklistSettings{})
		db.DB.Where("chat_id IN ?", chatIDs).Delete(&db.ChatFilters{})
		for _, id := range chatIDs {
			for _, key := range []string{"blacklist", "filter_list", "filters_optimized", "notes_list"} {
				dbcache.DeleteCache(dbcache.CacheKey(key, id))
			}
		}
		dbcache.ResetLocalForTest()
	}
	clean()
	t.Cleanup(clean)
}

// settingsListSeedBlacklist stores words on warn in the chat through the repository, as /addblacklist does.
func settingsListSeedBlacklist(t *testing.T, chatID int64, words ...string) {
	t.Helper()
	for _, word := range words {
		settingsListSeedBlacklistAction(t, chatID, word, "warn")
	}
}

// settingsListSeedBlacklistAction stores one word with the given stored action.
func settingsListSeedBlacklistAction(t *testing.T, chatID int64, word, action string) {
	t.Helper()
	if err := blacklists.AddBlacklistContext(context.Background(), chatID, word, action); err != nil {
		t.Fatalf("seed blacklist word %q (%s): %v", word, action, err)
	}
}

// settingsListOpenPrivate presses the Main cell of page in a private menu of the env's chat as user (a fresh message
// in the user's private chat) and returns the message ID with the text and keyboard it was edited to.
func settingsListOpenPrivate(e *settingsListEnv, user gotgbot.User, page string, kv ...string) (int64, string, [][]gotgbot.InlineKeyboardButton) {
	e.t.Helper()
	msgID := e.updateID()
	fields := e.privateFields(e.chat.Id, append([]string{"a", settingsActOpen, "p", page}, kv...)...)
	e.pressPrivate(user, msgID, fields)
	text, keyboard := e.lastEdit(user.Id, msgID)
	return msgID, text, keyboard
}

// settingsListRowWidths is the width of each row of a keyboard.
func settingsListRowWidths(keyboard [][]gotgbot.InlineKeyboardButton) []int {
	widths := make([]int, 0, len(keyboard))
	for _, row := range keyboard {
		widths = append(widths, len(row))
	}
	return widths
}

// settingsListWords makes n distinct lower-case words, sorted in the order the list shows them: w01, w02, ...
func settingsListWords(n int) []string {
	words := make([]string, 0, n)
	for i := 1; i <= n; i++ {
		words = append(words, fmt.Sprintf("w%02d", i))
	}
	return words
}
