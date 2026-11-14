//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	logrustest "github.com/sirupsen/logrus/hooks/test"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// The sentinels the privacy sweeps seed and send: a word, a keyword, a note name and a content text of the three lists. They
// are distinct, lower case (the stores lower-case what they keep) and carry no digit, so a search for one is unambiguous.
const (
	settingsListSentinelWord    = settingsListSentinel + "bl"
	settingsListSentinelKeyword = settingsListSentinel + "kw"
	settingsListSentinelName    = settingsListSentinel + "nm"
	settingsListSentinelContent = settingsListSentinel + "ct"
)

// settingsListSentinels are all of them, for a search of everything a group member could read or a log could carry.
var settingsListSentinels = []string{
	settingsListSentinelWord, settingsListSentinelKeyword, settingsListSentinelName, settingsListSentinelContent,
}

// TestSettingsListGroupMenuShowsNothing: with sentinel entries seeded in all three lists, a menu opened in the group (the
// place every member can read) never shows an entry, a count or an action of a list, whatever is pressed in it, never
// reads a list, and refuses every list action with the invalid-request alert before any lookup (D-01, T-19-48).
func TestSettingsListGroupMenuShowsNothing(t *testing.T) {
	setCachedBotUsername(t, "FukuTestBot")
	env := newSettingsListEnv(t)
	settingsListSeedBlacklistAction(t, env.chat.Id, settingsListSentinelWord, "ban")
	settingsListSeedFilters(t, env.chat.Id, settingsListSentinelKeyword)
	settingsListSeedAdminNote(t, env.chat.Id, settingsListSentinelName)

	bReads, bCounts := settingsBlacklistCountSeams(t)
	fReads, fCounts := settingsFiltersCountSeams(t)
	nReads, nCounts := settingsNotesCountSeams(t)

	groupMsg := env.openMenu(env.admin)
	press := func(kv ...string) {
		env.press(env.admin, groupMsg, env.groupFields(env.admin, kv...))
	}
	// Every list page's cell, Back, a forged Cancel naming each list page and a forged Refresh of each.
	for _, page := range settingsPhase19Pages {
		press("a", settingsActOpen, "p", page)
		if text, _ := env.lastEdit(env.chat.Id, groupMsg); !strings.Contains(text, staffMarker("settings_entries_group_note")) {
			t.Errorf("page %s opened in a group menu drew %q, want only the group note", page, text)
		}
		press("a", settingsActCancel, "p", page)
		press("a", settingsActBack)
	}

	// Every list action of a group menu is an invalid request before any lookup.
	for _, row := range settingsPhase19Codes(settingsCardExpiry(time.Now())) {
		members, chats, answers := len(env.fake.callsFor("getChatMember")), len(env.fake.callsFor("getChat")), env.answerCount()
		kv := []string{"a", row.code}
		for key, value := range row.fields {
			kv = append(kv, key, value)
		}
		press(kv...)
		if got := env.answerCount() - answers; got != 1 {
			t.Errorf("%s from a group menu was answered %d times, want once", row.code, got)
		}
		if text, alert := env.lastAnswer(); !strings.Contains(text, staffMarker("common_callback_invalid_request")) || !alert {
			t.Errorf("%s from a group menu answered %q (alert %v), want the invalid request alert", row.code, text, alert)
		}
		if got := len(env.fake.callsFor("getChatMember")) - members; got != 0 {
			t.Errorf("%s from a group menu made %d getChatMember calls, want none", row.code, got)
		}
		if got := len(env.fake.callsFor("getChat")) - chats; got != 0 {
			t.Errorf("%s from a group menu made %d getChat calls, want none", row.code, got)
		}
	}

	// Nothing the group received, in any text, button or answer, holds a sentinel, a count line, an action line or an entry line.
	everything := settingsBlacklistTexts(env)
	for _, leak := range append(append([]string{}, settingsListSentinels...),
		staffMarker("settings_bl_count"), staffMarker("settings_flt_count"), staffMarker("settings_notes_count"),
		staffMarker("settings_bl_action_line"), staffMarker("settings_notes_admin_tag"), "1. <code>",
	) {
		if strings.Contains(everything, leak) {
			t.Errorf("a group menu showed %q", leak)
		}
	}
	for name, n := range map[string]int64{
		"blacklist reads": bReads.Load(), "blacklist counts": bCounts.Load(), "filters reads": fReads.Load(),
		"filters counts": fCounts.Load(), "notes reads": nReads.Load(), "notes counts": nCounts.Load(),
	} {
		if n != 0 {
			t.Errorf("a group menu made %d %s, want none", n, name)
		}
	}
	// The seeded rows are untouched.
	if got := settingsBlacklistStored(t, env.chat.Id); len(got) != 1 || got[settingsListSentinelWord] != "ban" {
		t.Errorf("the group menu changed the blacklist: %v", got)
	}
}

// settingsListLogged is a logrus hook at debug level with the sentinels' search: it collects every line of the test and
// tells which of them mention a chat.
type settingsListLogged struct {
	hook *logrustest.Hook
}

// newSettingsListLogged installs the hook and puts the level back when the test ends.
func newSettingsListLogged(t *testing.T) *settingsListLogged {
	t.Helper()
	level := log.GetLevel()
	log.SetLevel(log.DebugLevel)
	hook := logrustest.NewGlobal()
	t.Cleanup(func() {
		hook.Reset()
		log.SetLevel(level)
	})
	return &settingsListLogged{hook: hook}
}

// mark is how many lines were logged so far.
func (l *settingsListLogged) mark() int { return len(l.hook.AllEntries()) }

// since returns the lines logged after mark, each as its message and fields.
func (l *settingsListLogged) since(mark int) []string {
	entries := l.hook.AllEntries()
	lines := make([]string, 0, len(entries)-mark)
	for _, entry := range entries[mark:] {
		lines = append(lines, entry.Message+" "+fmt.Sprint(entry.Data))
	}
	return lines
}

// wantChat fails the test unless a line after mark names the chat, and returns the lines.
func (l *settingsListLogged) wantChat(t *testing.T, name string, mark int, chatID int64) {
	t.Helper()
	id := strconv.FormatInt(chatID, 10)
	for _, line := range l.since(mark) {
		if strings.Contains(line, id) {
			return
		}
	}
	t.Errorf("%s logged no line naming chat %d: %q", name, chatID, l.since(mark))
}

// settingsListNoteAddFailing is a note write that always fails with err.
func settingsListNoteAddFailing(err error) func(context.Context, int64, string, string, string, models.ButtonArray, int, bool, bool, bool, bool, bool, bool) (bool, error) {
	return func(context.Context, int64, string, string, string, models.ButtonArray, int, bool, bool, bool, bool, bool, bool) (bool, error) {
		return false, err
	}
}

// TestSettingsListLogsCarryIDsOnly: every failure path of the three lists (a page read, an Add gate, a delete, each typed step,
// the wait store and every apply function) logs at least one line naming the chat and no line holds a word, keyword, note
// name or content (D-10, C-14, T-19-50).
func TestSettingsListLogsCarryIDsOnly(t *testing.T) {
	setCachedBotUsername(t, "FukuTestBot")
	logged := newSettingsListLogged(t)
	env := newSettingsListEnv(t)
	LoadNotes(env.dispatcher)
	chat := env.chat
	owner := settingsFiltersOwner(env, chat)
	settingsListSeedBlacklistAction(t, chat.Id, settingsListSentinelWord, "ban")
	settingsListSeedFilters(t, chat.Id, settingsListSentinelKeyword)
	settingsListSeedAdminNote(t, chat.Id, settingsListSentinelName)
	broken := errors.New("the database is down")
	// asks presses the ask of the entry of the page's list and then Yes of the card it drew.
	deletes := func(m *settingsBlacklistMenu, ask string, id int64) {
		t.Helper()
		m.press("a", ask, "r", strconv.FormatInt(id, 10), "v", "1")
		_, keyboard := m.last()
		m.tap(keyboard[0][0])
	}

	t.Run("a page whose read fails", func(t *testing.T) {
		mark := logged.mark()
		mreqSwap(t, &settingsBlacklistRead, func(context.Context, int64) ([]settingsListEntry, error) { return nil, broken })
		mreqSwap(t, &settingsFiltersRead, func(context.Context, int64) ([]settingsListEntry, error) { return nil, broken })
		mreqSwap(t, &settingsNotesRead, func(context.Context, int64) ([]settingsListEntry, error) { return nil, broken })
		for _, page := range settingsPhase19Pages {
			settingsListOpenMenuPage(env, chat, owner, page)
		}
		logged.wantChat(t, "a failing read", mark, chat.Id)
	})

	t.Run("an Add that cannot count the filters", func(t *testing.T) {
		mark := logged.mark()
		mreqSwap(t, &settingsFiltersCount, func(context.Context, int64) (int, error) { return 0, broken })
		m := settingsListOpenMenuPage(env, chat, owner, settingsPageFilters)
		m.press("a", "ia")
		logged.wantChat(t, "a failing count at Add", mark, chat.Id)
	})

	t.Run("a delete whose write fails", func(t *testing.T) {
		mark := logged.mark()
		mreqSwap(t, &blacklistWrites.RemoveByID, func(context.Context, int64, int64) (string, bool, error) { return "", false, broken })
		mreqSwap(t, &filterWrites.RemoveByID, func(context.Context, int64, int64) (string, bool, error) { return "", false, broken })
		mreqSwap(t, &noteWrites.RemoveByID, func(context.Context, int64, int64) (string, bool, error) { return "", false, broken })
		deletes(settingsListOpenMenuPage(env, chat, owner, settingsPageBlacklist), "bx", settingsBlacklistIDOf(t, chat.Id, settingsListSentinelWord))
		deletes(settingsListOpenMenuPage(env, chat, owner, settingsPageFilters), "ix", settingsFiltersIDOf(t, chat.Id, settingsListSentinelKeyword))
		deletes(settingsListOpenMenuPage(env, chat, owner, settingsPageNotes), "ex", settingsNotesIDOf(t, chat.Id, settingsListSentinelName))
		logged.wantChat(t, "a failing delete", mark, chat.Id)
		if len(settingsBlacklistStored(t, chat.Id)) != 1 || len(settingsFiltersStored(t, chat.Id)) != 1 || len(settingsNotesStored(t, chat.Id)) != 1 {
			t.Errorf("a failing delete removed a row")
		}
	})

	t.Run("an action set whose write fails", func(t *testing.T) {
		mark := logged.mark()
		mreqSwap(t, &blacklistWrites.SetAction, func(context.Context, int64, string) (int64, error) { return 0, broken })
		m := settingsListOpenMenuPage(env, chat, owner, settingsPageBlacklist)
		m.press("a", settingsActBlacklistOpen)
		m.press("a", settingsActBlacklistSet, "v", "mute")
		logged.wantChat(t, "a failing action", mark, chat.Id)
	})

	t.Run("typed words whose writes fail", func(t *testing.T) {
		mark := logged.mark()
		mreqSwap(t, &blacklistWrites.Add, func(context.Context, int64, string, string) error { return broken })
		settingsListDoorWait(env, chat, owner)
		settingsListDoorSay(env, owner, settingsListSentinelWord+"x "+settingsListSentinelContent)
		logged.wantChat(t, "failing typed words", mark, chat.Id)
		if len(settingsBlacklistStored(t, chat.Id)) != 1 {
			t.Errorf("a failing write stored a word")
		}
	})

	t.Run("the two typed steps of Filters with a failing check and a failing write", func(t *testing.T) {
		mark := logged.mark()
		f := newSettingsFiltersFlow(env, chat)
		f.begin()
		exists := filterWrites.Exists
		mreqSwap(t, &filterWrites.Exists, func(context.Context, int64, string) (bool, error) { return false, broken })
		f.say(settingsListSentinelKeyword + "x")
		logged.wantChat(t, "a failing keyword check", mark, chat.Id)
		filterWrites.Exists = exists
		mark = logged.mark()
		f.say(settingsListSentinelKeyword + "x")
		mreqSwap(t, &filterWrites.Add, func(context.Context, int64, string, string, string, []db.Button, int) (bool, error) {
			return false, broken
		})
		f.say(settingsListSentinelContent)
		logged.wantChat(t, "a failing filter write", mark, chat.Id)
		if len(settingsFiltersStored(t, chat.Id)) != 1 {
			t.Errorf("a failing write stored a filter")
		}
	})

	t.Run("the two typed steps of Notes with a failing check and a failing write", func(t *testing.T) {
		mark := logged.mark()
		f := newSettingsNotesFlow(env, chat)
		f.begin()
		exists := noteWrites.Exists
		mreqSwap(t, &noteWrites.Exists, func(context.Context, int64, string) (bool, error) { return false, broken })
		f.say(settingsListSentinelName + "x")
		logged.wantChat(t, "a failing name check", mark, chat.Id)
		noteWrites.Exists = exists
		mark = logged.mark()
		f.say(settingsListSentinelName + "x")
		mreqSwap(t, &noteWrites.Add, settingsListNoteAddFailing(broken))
		f.say(settingsListSentinelContent)
		logged.wantChat(t, "a failing note write", mark, chat.Id)
		if len(settingsNotesStored(t, chat.Id)) != 1 {
			t.Errorf("a failing write stored a note")
		}
	})

	t.Run("the wait store with a failing Redis", func(t *testing.T) {
		mark := logged.mark()
		m := settingsListOpenMenuPage(env, chat, owner, settingsPageBlacklist)
		env.redis.SetError("LOADING the dataset is loading")
		t.Cleanup(func() { env.redis.SetError("") })
		m.press("a", "bn")
		m.press("a", "bc", "t", "w")
		env.redis.SetError("")
		logged.wantChat(t, "the wait store with a failing Redis", mark, chat.Id)
	})

	t.Run("every apply function with a failing write", func(t *testing.T) {
		ctx := context.Background()
		word, keyword, name := settingsListSentinelWord, settingsListSentinelKeyword, settingsListSentinelName
		for _, c := range []struct {
			name string
			run  func()
		}{
			{"blacklist add", func() {
				mreqSwap(t, &blacklistWrites.Add, func(context.Context, int64, string, string) error { return broken })
				applyBlacklistAdd(ctx, chat.Id, []string{word}, nil)
			}},
			{"blacklist remove", func() {
				mreqSwap(t, &blacklistWrites.Remove, func(context.Context, int64, string) error { return broken })
				applyBlacklistRemove(ctx, chat.Id, []string{word}, []string{word})
			}},
			{"blacklist remove by ID", func() {
				mreqSwap(t, &blacklistWrites.RemoveByID, func(context.Context, int64, int64) (string, bool, error) { return "", false, broken })
				applyBlacklistRemoveByID(ctx, chat.Id, 5)
			}},
			{"blacklist action", func() {
				mreqSwap(t, &blacklistWrites.SetAction, func(context.Context, int64, string) (int64, error) { return 0, broken })
				applyBlacklistAction(ctx, chat.Id, "ban")
			}},
			{"filter add", func() {
				mreqSwap(t, &filterWrites.Exists, func(context.Context, int64, string) (bool, error) { return false, nil })
				mreqSwap(t, &filterWrites.Count, func(context.Context, int64) (int64, error) { return 0, nil })
				mreqSwap(t, &filterWrites.Add, func(context.Context, int64, string, string, string, []db.Button, int) (bool, error) {
					return false, broken
				})
				applyFilterAdd(ctx, chat.Id, filterAddInput{Keyword: keyword + "y", Text: settingsListSentinelContent, DataType: db.TEXT})
			}},
			{"filter add, the existing keyword read fails", func() {
				mreqSwap(t, &filterWrites.Exists, func(context.Context, int64, string) (bool, error) { return false, broken })
				applyFilterAdd(ctx, chat.Id, filterAddInput{Keyword: keyword + "y", Text: settingsListSentinelContent, DataType: db.TEXT})
			}},
			{"filter add, the count fails", func() {
				mreqSwap(t, &filterWrites.Exists, func(context.Context, int64, string) (bool, error) { return false, nil })
				mreqSwap(t, &filterWrites.Count, func(context.Context, int64) (int64, error) { return 0, broken })
				applyFilterAdd(ctx, chat.Id, filterAddInput{Keyword: keyword + "y", Text: settingsListSentinelContent, DataType: db.TEXT})
			}},
			{"filter remove", func() {
				mreqSwap(t, &filterWrites.Remove, func(context.Context, int64, string) error { return broken })
				applyFilterRemove(ctx, chat.Id, keyword)
			}},
			{"filter remove by ID", func() {
				mreqSwap(t, &filterWrites.RemoveByID, func(context.Context, int64, int64) (string, bool, error) { return "", false, broken })
				applyFilterRemoveByID(ctx, chat.Id, 5)
			}},
			{"note add", func() {
				mreqSwap(t, &noteWrites.Exists, func(context.Context, int64, string) (bool, error) { return false, nil })
				mreqSwap(t, &noteWrites.Add, settingsListNoteAddFailing(broken))
				applyNoteAdd(ctx, chat.Id, noteAddInput{Name: name + "y", Text: settingsListSentinelContent, DataType: db.TEXT})
			}},
			{"note add, the existing name read fails", func() {
				mreqSwap(t, &noteWrites.Exists, func(context.Context, int64, string) (bool, error) { return false, broken })
				applyNoteAdd(ctx, chat.Id, noteAddInput{Name: name + "y", Text: settingsListSentinelContent, DataType: db.TEXT})
			}},
			{"note remove", func() {
				mreqSwap(t, &noteWrites.Remove, func(context.Context, int64, string) error { return broken })
				applyNoteRemove(ctx, chat.Id, name)
			}},
			{"note remove by ID", func() {
				mreqSwap(t, &noteWrites.RemoveByID, func(context.Context, int64, int64) (string, bool, error) { return "", false, broken })
				applyNoteRemoveByID(ctx, chat.Id, 5)
			}},
		} {
			mark := logged.mark()
			c.run()
			logged.wantChat(t, c.name, mark, chat.Id)
		}
	})

	for _, line := range logged.since(0) {
		for _, sentinel := range settingsListSentinels {
			if strings.Contains(line, sentinel) {
				t.Errorf("a log line carries %q: %s", sentinel, line)
			}
		}
	}
}
