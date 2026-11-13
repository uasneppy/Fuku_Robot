//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
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
		if got := settingsListRowWidths(keyboard); fmt.Sprint(got) != fmt.Sprint([]int{1, 2}) {
			t.Errorf("row widths = %v, want the Add button alone and the navigation row", got)
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
