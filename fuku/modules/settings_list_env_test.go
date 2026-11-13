//go:build testtools

package modules

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/blacklists"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	db_filters "github.com/uasneppy/fuku_robot/fuku/db/filters"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/content"
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
	env.fake.setBotRole(env.chat.Id, staffRoleAdmin)
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

// settingsListSeedFilters stores text filters with the given keywords in the chat through the repository, as /filter
// does (each with a reply naming its keyword).
func settingsListSeedFilters(t *testing.T, chatID int64, keywords ...string) {
	t.Helper()
	for _, keyword := range keywords {
		if _, err := db_filters.AddFilterContext(context.Background(), chatID, keyword, "reply "+keyword, "", nil, db.TEXT); err != nil {
			t.Fatalf("seed filter %q: %v", keyword, err)
		}
	}
}

// settingsListOpenMenuPage opens the list page of the given key for chat in a fresh private message of user and returns
// the menu on it.
func settingsListOpenMenuPage(env *settingsListEnv, chat gotgbot.Chat, user gotgbot.User, page string) *settingsBlacklistMenu {
	env.t.Helper()
	m := &settingsBlacklistMenu{env: env, chat: chat, user: user, msgID: env.updateID()}
	m.press("a", settingsActOpen, "p", page)
	return m
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

// settingsListRegistry is what the settings registries hold for one page key and a set of action codes: the page, the
// list table and the action entries. withTestSettingsList saves it, replaces it and restores it.
type settingsListRegistry struct {
	page       settingsPage
	hasPage    bool
	list       settingsListDef
	hasList    bool
	actions    map[string]settingsActionEntry
	hasActions map[string]bool
}

// settingsListRegistryOf snapshots the registry entries of key and codes.
func settingsListRegistryOf(key string, codes []string) settingsListRegistry {
	snap := settingsListRegistry{actions: map[string]settingsActionEntry{}, hasActions: map[string]bool{}}
	snap.page, snap.hasPage = settingsPages[key]
	snap.list, snap.hasList = settingsLists[key]
	for _, code := range codes {
		snap.actions[code], snap.hasActions[code] = settingsActions[code]
	}
	return snap
}

// restore puts the snapshot back: an entry that was absent is removed again.
func (snap settingsListRegistry) restore(key string, codes []string) {
	if snap.hasPage {
		settingsPages[key] = snap.page
	} else {
		delete(settingsPages, key)
	}
	if snap.hasList {
		settingsLists[key] = snap.list
	} else {
		delete(settingsLists, key)
	}
	for _, code := range codes {
		if snap.hasActions[code] {
			settingsActions[code] = snap.actions[code]
		} else {
			delete(settingsActions, code)
		}
	}
}

// withTestSettingsList installs the page and the list table of def over the registries by saving what is there under
// def.Key and the page's action codes and replacing it, and restores every entry in t.Cleanup. It never calls
// registerSettingsPage, which panics on a duplicate key at init, so a test page may reuse the key of a real page
// (Filters or Notes) whether or not that page is registered. It refuses a code that another page owns. A test that
// uses it never calls t.Parallel.
func withTestSettingsList(t *testing.T, def settingsListDef) {
	t.Helper()
	page := settingsListPage(def)
	codes := make([]string, 0, len(page.Actions))
	for code := range page.Actions {
		if owner, taken := settingsActions[code]; taken && owner.pageKey != def.Key {
			t.Fatalf("the test list page %q cannot use action code %q: page %q owns it", def.Key, code, owner.pageKey)
		}
		codes = append(codes, code)
	}
	saved := settingsListRegistryOf(def.Key, codes)
	settingsPages[def.Key] = page
	settingsLists[def.Key] = def
	for code, action := range page.Actions {
		settingsActions[code] = settingsActionEntry{pageKey: def.Key, action: action}
	}
	t.Cleanup(func() { saved.restore(def.Key, codes) })
}

// settingsListTestApply is one call of the test page's Apply: the keyword it was given and the extraction.
type settingsListTestApply struct {
	Norm string
	Ex   content.ExtractResult
}

// settingsListTestStore is the in-memory list behind the test-only two-step page and the knobs a test turns: the
// refusal or error its Check answers, the gate screen its Add shows and the outcome its Apply answers.
type settingsListTestStore struct {
	entries      []settingsListEntry
	nextID       int64
	checkRefusal string
	checkErr     error
	checkCalls   int
	gate         string
	applyOutcome settingsListApplyOutcome
	applyText    string
	applyNotice  string
	applies      []settingsListTestApply
}

// settingsListTestTwoStepCodes are the action codes of the test page: unused by every real page.
var settingsListTestTwoStepCodes = []string{"zg", "zx", "zy", "za", "zc"}

// settingsListTestTwoStepDef is the table of the test-only two-step page: key i (the Filters parts of the static part
// table), its own codes, existing locale keys standing in for the Filters wording and the store's hooks.
func settingsListTestTwoStepDef(store *settingsListTestStore) settingsListDef {
	return settingsListDef{
		Key:          "i",
		Order:        47,
		Icon:         "🧪",
		TitleKey:     "settings_page_blacklist",
		PagerCode:    "zg",
		AskCode:      "zx",
		YesCode:      "zy",
		AddCode:      "za",
		CancelCode:   "zc",
		AddKey:       "settings_btn_bl_add",
		Right:        settingsRightAdmin,
		IntroKey:     "settings_bl_intro",
		CountKey:     "settings_bl_count",
		EmptyKey:     "settings_bl_empty",
		DeleteAskKey: "settings_bl_delete_ask",
		RemovedKey:   "settings_bl_removed",
		EntryParam:   "word",
		Read: func(context.Context, int64) ([]settingsListEntry, error) {
			return append([]settingsListEntry(nil), store.entries...), nil
		},
		Entry: func(_ context.Context, _, id int64) (*settingsListEntry, error) {
			for _, e := range store.entries {
				if e.ID == id {
					return &e, nil
				}
			}
			return nil, nil
		},
		Count:  func(context.Context, int64) (int, error) { return len(store.entries), nil },
		Remove: func(context.Context, int64, int64) settingsListRemoval { return settingsListRemoval{} },
		Prompt: func(v settingsView, _ string) []string { return []string{settingsText(v.Tr, "settings_bl_wait")} },
		AddGate: func(*settingsPress) *settingsScreen {
			if store.gate == "" {
				return nil
			}
			return &settingsScreen{Text: store.gate}
		},
		TwoStep: &settingsListTwoStep{
			Normalise: func(text string) string { return strings.ToLower(strings.Join(strings.Fields(text), " ")) },
			Check: func(_ context.Context, _ int64, norm string, _ *i18n.Translator) (string, error) {
				store.checkCalls++
				if utf8.RuneCountInString(norm) > listKeywordMaxRunes {
					return "too-long-keyword", nil
				}
				return store.checkRefusal, store.checkErr
			},
			IsFilter: true,
			Apply: func(_ context.Context, _ int64, norm string, ex content.ExtractResult, _ *i18n.Translator) settingsListApplyResult {
				store.applies = append(store.applies, settingsListTestApply{Norm: norm, Ex: ex})
				if store.applyOutcome == settingsListApplySaved {
					store.nextID++
					store.entries = append(store.entries, settingsListEntry{ID: store.nextID, Text: norm})
				}
				return settingsListApplyResult{Outcome: store.applyOutcome, Text: store.applyText, Notice: store.applyNotice}
			},
			KeyLineKey: "settings_bl_delete_ask",
			SavedKey:   "settings_bl_removed",
		},
	}
}
