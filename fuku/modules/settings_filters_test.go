//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	db_filters "github.com/uasneppy/fuku_robot/fuku/db/filters"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// settingsFiltersOwner is a live creator of chat, who therefore holds can_change_info (every Filters write needs it),
// with the users row and cache key the dispatcher may write for them removed when the test ends.
func settingsFiltersOwner(env *settingsListEnv, chat gotgbot.Chat) gotgbot.User {
	env.t.Helper()
	owner := env.newOwner(chat)
	env.t.Cleanup(func() {
		db.DB.Where("user_id = ?", owner.Id).Delete(&models.User{})
		dbcache.DeleteCache(dbcache.CacheKey("user", owner.Id))
		dbcache.ResetLocalForTest()
	})
	return owner
}

// settingsFiltersCountSeams replaces the page's read and count seams with counting wrappers and restores them.
func settingsFiltersCountSeams(t *testing.T) (reads, counts *atomic.Int64) {
	t.Helper()
	reads, counts = new(atomic.Int64), new(atomic.Int64)
	read, count := settingsFiltersRead, settingsFiltersCount
	settingsFiltersRead = func(ctx context.Context, chatID int64) ([]settingsListEntry, error) {
		reads.Add(1)
		return read(ctx, chatID)
	}
	settingsFiltersCount = func(ctx context.Context, chatID int64) (int, error) {
		counts.Add(1)
		return count(ctx, chatID)
	}
	t.Cleanup(func() { settingsFiltersRead, settingsFiltersCount = read, count })
	return reads, counts
}

// settingsFiltersStored is the chat's stored filter rows by keyword.
func settingsFiltersStored(t *testing.T, chatID int64) map[string]db.ChatFilters {
	t.Helper()
	var rows []db.ChatFilters
	if err := db.DB.Where("chat_id = ?", chatID).Find(&rows).Error; err != nil {
		t.Fatalf("list the filters: %v", err)
	}
	stored := make(map[string]db.ChatFilters, len(rows))
	for _, row := range rows {
		stored[row.KeyWord] = row
	}
	return stored
}

func TestSettingsFiltersOpens(t *testing.T) {
	t.Run("a private menu lists the keywords in order with the intro, the count of 150 and the Add button", func(t *testing.T) {
		env := newSettingsListEnv(t)
		settingsListSeedFilters(t, env.chat.Id, "gamma", "alpha", "beta")
		_, text, keyboard := settingsListOpenPrivate(env, env.admin, settingsPageFilters)

		for _, want := range []string{
			staffMarker("settings_header"), staffMarker("settings_page_filters"), staffMarker("settings_flt_intro"),
			staffMarker("settings_flt_count") + " 3 150",
		} {
			if !strings.Contains(text, want) {
				t.Errorf("the page lacks %q:\n%s", want, text)
			}
		}
		last := -1
		for i, keyword := range []string{"alpha", "beta", "gamma"} {
			line := fmt.Sprintf("%d. <code>%s</code>", i+1, keyword)
			at := strings.Index(text, line)
			if at < 0 || at < last {
				t.Errorf("the line %q is missing or out of order:\n%s", line, text)
			}
			last = at
		}
		if got := settingsListRowWidths(keyboard); fmt.Sprint(got) != fmt.Sprint([]int{3, 1, 2}) {
			t.Errorf("row widths = %v, want the delete row, the Add button alone and the navigation row", got)
		}
		if _, ok := settingsButtonByMarker(keyboard, "settings_btn_flt_add"); !ok {
			t.Errorf("the page has no Add filter button: %+v", keyboard)
		}
	})

	t.Run("an empty list draws the empty line and the Add button", func(t *testing.T) {
		env := newSettingsListEnv(t)
		chat := env.addChat()
		settingsListCleanup(t, chat.Id)
		m := settingsListOpenMenuPage(env, chat, env.admin, settingsPageFilters)
		text, keyboard := m.last()
		if !strings.Contains(text, staffMarker("settings_flt_empty")) || strings.Contains(text, staffMarker("settings_flt_count")) {
			t.Errorf("the empty page drew:\n%s", text)
		}
		if got := settingsListRowWidths(keyboard); fmt.Sprint(got) != fmt.Sprint([]int{1, 2}) {
			t.Errorf("empty rows = %v, want {1,2}", got)
		}
	})

	t.Run("Main's Filters line reads n/150 in private, the unknown mark on a failing count and In private in a group", func(t *testing.T) {
		setCachedBotUsername(t, "FukuTestBot")
		env := newSettingsListEnv(t)
		settingsListSeedFilters(t, env.chat.Id, settingsListWords(3)...)
		msgID := env.updateID()
		env.pressPrivate(env.admin, msgID, env.privateFields(env.chat.Id, "a", settingsActBack))
		text, keyboard := env.lastEdit(env.admin.Id, msgID)
		line, ok := settingsMainStateLine(text, "settings_page_filters")
		if !ok || !strings.HasSuffix(line, " 3/150") {
			t.Fatalf("Main's Filters line = %q, want it to end in 3/150:\n%s", line, text)
		}
		if _, ok := settingsButtonByMarker(keyboard, "settings_page_filters"); !ok {
			t.Errorf("Main has no Filters cell")
		}

		reads, counts := settingsFiltersCountSeams(t)
		groupMsg := env.openMenu(env.admin)
		groupMain, _ := env.lastEdit(env.chat.Id, groupMsg)
		groupLine, ok := settingsMainStateLine(groupMain, "settings_page_filters")
		if !ok || !strings.HasSuffix(groupLine, staffMarker("settings_state_private_only")) {
			t.Errorf("a group Main's Filters line = %q, want In private", groupLine)
		}
		if counts.Load() != 0 || reads.Load() != 0 {
			t.Errorf("a group Main read the filters %d times and counted them %d times, want none", reads.Load(), counts.Load())
		}

		settingsFiltersCount = func(context.Context, int64) (int, error) { return 0, errors.New("count failed") }
		failedMsg := env.updateID()
		env.pressPrivate(env.admin, failedMsg, env.privateFields(env.chat.Id, "a", settingsActBack))
		failed, _ := env.lastEdit(env.admin.Id, failedMsg)
		unknown, ok := settingsMainStateLine(failed, "settings_page_filters")
		if !ok || !strings.HasSuffix(unknown, settingsUnknownState) {
			t.Errorf("a failing count reads %q, want the unknown mark", unknown)
		}
	})

	t.Run("a failing read is the cannot-open edit", func(t *testing.T) {
		env := newSettingsListEnv(t)
		read := settingsFiltersRead
		settingsFiltersRead = func(context.Context, int64) ([]settingsListEntry, error) { return nil, errors.New("read failed") }
		t.Cleanup(func() { settingsFiltersRead = read })
		msgID := env.updateID()
		env.pressPrivate(env.admin, msgID, env.privateFields(env.chat.Id, "a", settingsActOpen, "p", settingsPageFilters))
		text, keyboard := env.lastEdit(env.admin.Id, msgID)
		if !strings.Contains(text, staffMarker("settings_cannot_open")) || len(keyboard) != 0 {
			t.Errorf("a failing read drew %q with %d rows, want the cannot-open edit with no keyboard", text, len(keyboard))
		}
	})
}

// settingsFiltersFlow is a creator of one chat working the Filters page's Add flow: the owner, their private menu and
// the helpers that read the wait and the bot's replies.
type settingsFiltersFlow struct {
	env  *settingsListEnv
	chat gotgbot.Chat
	user gotgbot.User
}

// newSettingsFiltersFlow makes the creator of chat.
func newSettingsFiltersFlow(env *settingsListEnv, chat gotgbot.Chat) *settingsFiltersFlow {
	env.t.Helper()
	return &settingsFiltersFlow{env: env, chat: chat, user: settingsFiltersOwner(env, chat)}
}

// begin opens the Filters page in a fresh private menu and presses Add, so the owner has a wait for the keyword part. It
// returns the menu.
func (f *settingsFiltersFlow) begin() *settingsBlacklistMenu {
	f.env.t.Helper()
	m := settingsListOpenMenuPage(f.env, f.chat, f.user, settingsPageFilters)
	add, ok := m.markedButton("settings_btn_flt_add")
	if !ok {
		f.env.t.Fatalf("the Filters page has no Add button")
	}
	m.tap(add)
	return m
}

// field reads one field of the owner's wait hash.
func (f *settingsFiltersFlow) field(name string) string {
	return f.env.redis.HGet(settingsInputKey(f.user.Id), name)
}

// endWait removes whatever wait and marker the previous case left, so the cases of one test do not see each other.
func (f *settingsFiltersFlow) endWait() {
	f.env.t.Helper()
	f.env.redis.Del(settingsInputKey(f.user.Id))
	f.env.redis.Del(settingsInputDoneKey(f.user.Id))
}

// say sends text as a private message of the owner and returns the message's ID.
func (f *settingsFiltersFlow) say(text string) int64 {
	f.env.t.Helper()
	return settingsListDoorSay(f.env, f.user, text)
}

// sent is how many messages the bot has sent to the owner so far.
func (f *settingsFiltersFlow) sent() int {
	return len(f.env.fake.sentTo(f.user.Id))
}

// reply fails the test unless exactly one message was sent after the first `before` and it carries every key's marker.
func (f *settingsFiltersFlow) reply(before int, keys ...string) staffSentMessage {
	f.env.t.Helper()
	return settingsListDoorReply(f.env.t, f.env, f.user, before, keys...)
}

// waitIs fails the test unless the wait is at part holding no claim, with keyword in w.
func (f *settingsFiltersFlow) waitIs(part, keyword string) {
	f.env.t.Helper()
	settingsEditWaitKept(f.env.t, f.env.settingsEnv, f.user)
	if f.field("t") != part || f.field("w") != keyword {
		f.env.t.Errorf("the wait is t=%q w=%q, want t=%q w=%q", f.field("t"), f.field("w"), part, keyword)
	}
}

// settingsFiltersIDOf is the row ID of the chat's stored filter.
func settingsFiltersIDOf(t *testing.T, chatID int64, keyword string) int64 {
	t.Helper()
	row, ok := settingsFiltersStored(t, chatID)[keyword]
	if !ok {
		t.Fatalf("filter %q is not stored in chat %d", keyword, chatID)
	}
	return int64(row.ID)
}

// settingsFiltersPrimeCaches fills both filter cache keys of the chat and returns them.
func settingsFiltersPrimeCaches(t *testing.T, env *settingsListEnv, chatID int64) []string {
	t.Helper()
	db_filters.GetFiltersListContext(context.Background(), chatID)
	if _, err := db_filters.GetChatFiltersCachedContext(context.Background(), chatID); err != nil {
		t.Fatalf("prime the optimized filters: %v", err)
	}
	keys := []string{dbcache.CacheKey("filter_list", chatID), dbcache.CacheKey("filters_optimized", chatID)}
	for _, key := range keys {
		if !env.redis.Exists(key) {
			t.Fatalf("the cache key %s was not primed", key)
		}
	}
	return keys
}

func TestSettingsFiltersDelete(t *testing.T) {
	t.Run("the ask draws a card naming the keyword, Yes deletes that row and drops both caches, No returns to the page", func(t *testing.T) {
		env := newSettingsListEnv(t)
		owner := settingsFiltersOwner(env, env.chat)
		settingsListSeedFilters(t, env.chat.Id, "alpha", "beta", "gamma", "delta")
		keys := settingsFiltersPrimeCaches(t, env, env.chat.Id)
		m := settingsListOpenMenuPage(env, env.chat, owner, settingsPageFilters)
		deltaID := settingsFiltersIDOf(t, env.chat.Id, "delta")

		m.tap(m.button("🗑 3"))
		text, keyboard := m.last()
		if !strings.Contains(text, staffMarker("settings_flt_delete_ask")+" delta") {
			t.Fatalf("the card does not name the keyword:\n%s", text)
		}
		if got := settingsListRowWidths(keyboard); fmt.Sprint(got) != fmt.Sprint([]int{2, 2}) {
			t.Errorf("card rows = %v, want {2,2}", got)
		}
		yes, _ := m.markedButton("settings_btn_yes")
		yesFields := settingsButtonFields(t, yes)
		if yesFields["a"] != "iy" || yesFields["r"] != strconv.FormatInt(deltaID, 10) || yesFields["e"] == "" || len(yesFields) != 4 {
			t.Errorf("Yes carries %v, want a=iy, the row ID, an expiry and c", yesFields)
		}
		no, _ := m.markedButton("settings_btn_no")
		if f := settingsButtonFields(t, no); f["a"] != "ig" || f["v"] != "1" || len(f) != 3 {
			t.Errorf("No carries %v, want a=ig, v=1 and c", f)
		}
		if got := settingsFiltersStored(t, env.chat.Id); len(got) != 4 {
			t.Errorf("the ask changed the list: %v", got)
		}

		answers := env.answerCount()
		m.tap(yes)
		if got := env.answerCount() - answers; got != 1 {
			t.Errorf("the Yes was answered %d times, want once", got)
		}
		text, _ = m.last()
		if !strings.Contains(text, settingsNoticeOKIcon+staffMarker("settings_flt_removed")+" delta") {
			t.Errorf("the page lacks the removed notice naming the keyword:\n%s", text)
		}
		for _, line := range []string{"1. <code>alpha</code>", "2. <code>beta</code>", "3. <code>gamma</code>"} {
			if !strings.Contains(text, line) {
				t.Errorf("the redrawn page lacks %q:\n%s", line, text)
			}
		}
		if _, still := settingsFiltersStored(t, env.chat.Id)["delta"]; still {
			t.Errorf("the filter is still stored")
		}
		for _, key := range keys {
			if env.redis.Exists(key) {
				t.Errorf("the delete did not drop %s", key)
			}
		}
	})

	t.Run("No returns to the page the ask came from", func(t *testing.T) {
		env := newSettingsListEnv(t)
		owner := settingsFiltersOwner(env, env.chat)
		settingsListSeedFilters(t, env.chat.Id, settingsListWords(20)...)
		m := settingsListOpenMenuPage(env, env.chat, owner, settingsPageFilters)
		m.press("a", "ig", "v", "2")
		m.tap(m.button("🗑 10"))
		no, _ := m.markedButton("settings_btn_no")
		m.tap(no)
		text, _ := m.last()
		if !strings.Contains(text, "9. <code>w09</code>") || strings.Contains(text, settingsNoticeOKIcon) || strings.Contains(text, settingsNoticeWarnIcon) {
			t.Errorf("No did not return to page 2 without a notice:\n%s", text)
		}
	})

	t.Run("an administrator without can_change_info is refused at the ask and sees no card", func(t *testing.T) {
		env := newSettingsListEnv(t)
		settingsListSeedFilters(t, env.chat.Id, "alpha")
		m := settingsListOpenMenuPage(env, env.chat, env.admin, settingsPageFilters)
		edits, answers := len(env.edits(env.admin.Id, m.msgID)), env.answerCount()
		m.tap(m.button("🗑 1"))
		if text, alert := env.lastAnswer(); !strings.Contains(text, staffMarker("settings_no_change_info")) || !alert {
			t.Errorf("the ask answered %q (alert %v), want the settings_no_change_info alert", text, alert)
		}
		if got := env.answerCount() - answers; got != 1 {
			t.Errorf("the ask was answered %d times, want once", got)
		}
		if got := len(env.edits(env.admin.Id, m.msgID)); got != edits {
			t.Errorf("the refused ask edited the menu")
		}
	})

	t.Run("another chat's row ID answers already removed and deletes nothing", func(t *testing.T) {
		env := newSettingsListEnv(t)
		owner := settingsFiltersOwner(env, env.chat)
		other := env.addChat()
		settingsListCleanup(t, other.Id)
		settingsListSeedFilters(t, env.chat.Id, "alpha")
		settingsListSeedFilters(t, other.Id, "foreign")
		foreignID := settingsFiltersIDOf(t, other.Id, "foreign")
		m := settingsListOpenMenuPage(env, env.chat, owner, settingsPageFilters)

		m.press("a", "ix", "r", strconv.FormatInt(foreignID, 10), "v", "1")
		text, _ := m.last()
		if !strings.Contains(text, settingsNoticeWarnIcon+staffMarker("settings_entries_already_removed")) || strings.Contains(text, staffMarker("settings_flt_delete_ask")) {
			t.Errorf("a foreign ID's ask drew:\n%s", text)
		}
		m.press("a", "iy", "r", strconv.FormatInt(foreignID, 10), "e", settingsCardExpiry(time.Now()))
		text, _ = m.last()
		if !strings.Contains(text, staffMarker("settings_entries_already_removed")) {
			t.Errorf("a foreign ID's Yes drew:\n%s", text)
		}
		if got := settingsFiltersStored(t, other.Id); len(got) != 1 {
			t.Errorf("a forged ID deleted another chat's filter: %v", got)
		}
		if got := settingsFiltersStored(t, env.chat.Id); len(got) != 1 {
			t.Errorf("a forged ID changed this chat's filters: %v", got)
		}
	})
}

// settingsFiltersForgedPresses are the Filters presses that step 1 must refuse before any I/O (the cases the Blacklist's
// forged table holds for its own codes).
func settingsFiltersForgedPresses(valid string) map[string][]string {
	return map[string][]string{
		"ix with an 18-digit r":    {"a", "ix", "r", "123456789012345678", "v", "1"},
		"ix with a leading zero":   {"a", "ix", "r", "012", "v", "1"},
		"ix without v":             {"a", "ix", "r", "5"},
		"ix with an extra key":     {"a", "ix", "r", "5", "v", "1", "x", "1"},
		"iy without e":             {"a", "iy", "r", "5"},
		"iy with a non-numeric e":  {"a", "iy", "r", "5", "e", "abc"},
		"iy with a v":              {"a", "iy", "r", "5", "e", valid, "v", "1"},
		"ia with an extra key":     {"a", "ia", "x", "1"},
		"ic with another page's w": {"a", "ic", "t", "w"},
		"ic with another page's n": {"a", "ic", "t", "n"},
		"ic without t":             {"a", "ic"},
		"ig with a letter v":       {"a", "ig", "v", "x"},
		"ig with a 5-digit v":      {"a", "ig", "v", "12345"},
		"st naming page i":         {"a", "st", "p", "i", "t", "x", "v", "1"},
		"tg naming page i":         {"a", "tg", "p", "i", "t", "x", "v", "1"},
	}
}
