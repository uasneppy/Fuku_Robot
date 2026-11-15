//go:build testtools

package modules

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/uasneppy/fuku_robot/fuku/db"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// settingsFwdWorstEntries makes n entries with 17-digit row IDs and a 14-character source ID, the worst case the delete
// buttons carry.
func settingsFwdWorstEntries(n int) []settingsListEntry {
	entries := make([]settingsListEntry, 0, n)
	for i := 0; i < n; i++ {
		entries = append(entries, settingsListEntry{
			ID: 12345678901234500 + int64(i), Text: forwardBlockEntryText(-1001234567890-int64(i), "Title"), Action: "ban",
		})
	}
	return entries
}

func TestSettingsListFwdBlockCallbackBytes(t *testing.T) {
	withRealLocales(t)
	tr := i18n.MustNewTranslator("en")
	views := settingsBlacklistWorstViews(tr)
	private := views["private"]
	def := settingsFwdBlockDef()
	// 79,992 entries are 9,999 pages: the pager cursor v reaches its four digits.
	worst := settingsFwdWorstEntries(settingsListPageSize * 9999)

	sizes := make(map[string]int)
	measure := func(name string, screen settingsScreen) {
		t.Helper()
		if screen.Omitted != 0 {
			t.Errorf("%s: %d buttons were omitted for their size, want none", name, screen.Omitted)
		}
		for _, button := range settingsAllButtons(screen.Keyboard.InlineKeyboard) {
			if button.CallbackData == "" {
				continue
			}
			if size := len(button.CallbackData); size > 64 {
				t.Errorf("%s: button %q carries %d bytes, over the 64-byte cap", name, button.Text, size)
			}
			fields := settingsButtonFields(t, button)
			key := fields["a"]
			if key == settingsActFwdBlockSet {
				key += ":" + fields["v"]
			}
			sizes[key] = max(sizes[key], len(button.CallbackData))
		}
	}

	for name, page := range map[string]int{"first": 1, "middle": 5000, "page 9998": 9998, "last": 9999} {
		measure("list "+name, renderSettingsList(private, def, worst, page, ""))
	}
	measure("a one-entry list", renderSettingsList(private, def, worst[:1], 1, ""))
	measure("an empty list", renderSettingsList(private, def, nil, 1, ""))
	measure("the card", renderSettingsListCard(private, def, worst[0], 9999))
	for _, action := range []string{"warn", "mute", "kick", "ban", "none"} {
		measure("the action screen of "+action, renderSettingsFwdBlockAction(private, action, ""))
	}
	measure("the group note", renderSettingsPrivateOnlyNote(views["group"], def.TitleKey, "FukuTestBot"))
	for _, code := range []string{"vg", "vx", "vy", "va", "ve"} {
		if sizes[code] == 0 {
			t.Errorf("no button of the code %s was drawn", code)
		}
	}

	// The two-letter codes keep the sizes the Blacklist page's codes have in the UI-SPEC's table.
	for code, want := range map[string]int{
		"vg": 40, "vx": 60, "vy": 64, "ve": 33,
		settingsActFwdBlockSet + ":warn": 40, settingsActFwdBlockSet + ":mute": 40,
		settingsActFwdBlockSet + ":kick": 40, settingsActFwdBlockSet + ":none": 40, settingsActFwdBlockSet + ":ban": 39,
	} {
		if got := sizes[code]; got != want {
			t.Errorf("the largest %s button is %d bytes, want %d", code, got, want)
		}
	}

	t.Run("a row whose ID has 18 digits has no delete button and is counted", func(t *testing.T) {
		entries := settingsFwdWorstEntries(3)
		entries[1].ID = 123456789012345678
		screen := renderSettingsList(private, def, entries, 1, "")
		if screen.Omitted != 1 {
			t.Errorf("Omitted = %d, want 1", screen.Omitted)
		}
	})
}

func TestSettingsListFwdBlockPageFitsEveryLocale(t *testing.T) {
	withRealLocales(t)
	title := strings.Repeat("Ж", 64)
	ampersands := strings.Repeat("&", 64)
	def := settingsFwdBlockDef()
	// A middle page of long entries: the title is 64 code points of ampersands, five UTF-16 units each once escaped.
	entries := make([]settingsListEntry, 0, 24)
	for i := 0; i < 24; i++ {
		entries = append(entries, settingsListEntry{ID: int64(10 + i), Text: forwardBlockEntryText(-1001234567890-int64(i), ampersands), Action: "ban"})
	}
	hundred := forwardBlockEntryText(-1001234567890, ampersands)
	until := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)

	for _, code := range memberReqScreenLanguages {
		t.Run(code, func(t *testing.T) {
			tr := i18n.MustNewTranslator(code)
			views := settingsBlacklistWorstViews(tr)
			for name, v := range views {
				v.GroupTitle = title
				views[name] = v
			}
			private := views["private"]
			notices := settingsBlacklistLongest([]string{
				settingsNoticeOK(settingsListEntryText(tr, "settings_fwd_removed", "source", hundred)),
				settingsFwdBlockSavedNotice(tr, forwardBlockAddResult{SourceChatID: -1001234567890, Title: strings.Repeat("&", forwardBlockTitleMaxRunes)}, true),
				settingsNoticeWarn(settingsText(tr, "settings_card_expired")),
				settingsNoticeWarn(settingsText(tr, "settings_entries_already_removed")),
				settingsNoticeWarn(settingsText(tr, "common_settings_save_failed")),
				settingsNoticeWarn(settingsText(tr, "staff_unset_not_staff")),
			}, 2)

			check := func(name string, screen settingsScreen, wantWidths string) {
				t.Helper()
				if units := panelUTF16Len(screen.Text); units == 0 || units > 3800 {
					t.Errorf("%s is %d UTF-16 units, want 1..3800", name, units)
				}
				if screen.Omitted != 0 {
					t.Errorf("%s omitted %d buttons", name, screen.Omitted)
				}
				if got := fmt.Sprint(settingsListRowWidths(screen.Keyboard.InlineKeyboard)); got != wantWidths {
					t.Errorf("%s has row widths %s, want %s", name, got, wantWidths)
				}
				for _, button := range settingsAllButtons(screen.Keyboard.InlineKeyboard) {
					if label := []rune(button.Text); len(label) > 20 && !strings.HasPrefix(button.Text, "🗑") && button.Url == "" {
						t.Errorf("%s: button %q has %d code points, want at most 20", name, button.Text, len(label))
					}
				}
			}
			for i, notice := range notices {
				check(fmt.Sprintf("the list page with notice %d", i), renderSettingsList(private, def, entries, 2, notice), "[4 4 2 2 2]")
				check(fmt.Sprintf("the action screen with notice %d", i), renderSettingsFwdBlockAction(private, "ban", notice), "[3 2 2]")
			}
			check("the empty list", renderSettingsList(private, def, nil, 1, ""), "[2 2]")
			card := renderSettingsListCard(private, def, settingsListEntry{ID: 12345678901234567, Text: hundred}, 9999)
			check("the card", card, "[2 2]")
			if !strings.Contains(card.Text, "-1001234567890 "+strings.Repeat("&amp;", 64)) {
				t.Errorf("the card does not name the long entry in full")
			}
			check("the Add prompt", renderSettingsListPrompt(private, def, "s", until, true, ""), "[1]")
			check("the group note", renderSettingsPrivateOnlyNote(views["group"], def.TitleKey, "FukuTestBot"), "[1 2]")
			check("the group note without a bot username", renderSettingsPrivateOnlyNote(views["group"], def.TitleKey, ""), "[2]")
		})
	}
}

func TestSettingsFwdBlockReadsCreateNothing(t *testing.T) {
	setCachedBotUsername(t, "FukuTestBot")
	e := newSettingsFwdEnv(t)
	count := func() (rows, settings, chats int64) {
		db.DB.Model(&models.StaffForwardBlock{}).Where("staff_chat_id = ?", e.chat.Id).Count(&rows)
		db.DB.Model(&models.StaffForwardBlockSettings{}).Where("staff_chat_id = ?", e.chat.Id).Count(&settings)
		db.DB.Model(&models.Chat{}).Where("chat_id = ?", e.chat.Id).Count(&chats)
		return rows, settings, chats
	}
	cacheKey := dbcache.CacheKey("staff_forward_block", e.chat.Id)
	rows, settings, chats := count()
	if rows != 0 || settings != 0 {
		t.Fatalf("the fresh Staff Group already has %d source rows and %d action rows", rows, settings)
	}

	// A private menu: Main, the list page, the action screen and a forged ask and Yes on a chat with no rows.
	m := e.open(e.chat, e.owner)
	m.press("a", settingsActBack)
	m.press("a", settingsActOpen, "p", settingsPageFwdBlock)
	m.press("a", settingsActFwdBlockOpen)
	m.press("a", "vx", "r", "42", "v", "1")
	m.press("a", "vy", "r", "42", "e", settingsCardExpiry(time.Now()))
	if text, _ := m.last(); !strings.Contains(text, staffMarker("settings_entries_already_removed")) {
		t.Fatalf("the screens did not draw as expected:\n%s", text)
	}
	if gotRows, gotSettings, gotChats := count(); gotRows != 0 || gotSettings != 0 || gotChats != chats {
		t.Errorf("a private menu left %d source rows and %d action rows and changed the chat rows from %d to %d", gotRows, gotSettings, chats, gotChats)
	}
	if e.redis.Exists(cacheKey) {
		t.Errorf("a private menu filled %s", cacheKey)
	}

	// A group menu: Main and the page's note read nothing and create nothing.
	groupMsg := e.openMenu(e.owner)
	e.press(e.owner, groupMsg, e.groupFields(e.owner, "a", settingsActOpen, "p", settingsPageFwdBlock))
	if gotRows, gotSettings, _ := count(); gotRows != 0 || gotSettings != 0 || e.redis.Exists(cacheKey) {
		t.Errorf("a group menu left %d source rows and %d action rows or filled %s", gotRows, gotSettings, cacheKey)
	}
}

// TestSettingsFwdBlockPageKeys holds the page's own tables: the registered page, its seven codes and the one part, so a
// code or part added to the page without a row here is noticed.
func TestSettingsFwdBlockPageKeys(t *testing.T) {
	pg, ok := settingsPageByKey(settingsPageFwdBlock)
	if !ok || !pg.PrivateOnly || pg.Hidden == nil || pg.Order != 11 {
		t.Fatalf("the page is registered %v, private-only %v, hidden hook set %v, order %d", ok, pg.PrivateOnly, pg.Hidden != nil, pg.Order)
	}
	var codes []string
	for code := range pg.Actions {
		codes = append(codes, code)
	}
	slices.Sort(codes)
	if want := []string{"va", "vc", "ve", "vg", "vs", "vx", "vy"}; !slices.Equal(codes, want) {
		t.Errorf("the page's codes are %v, want %v", codes, want)
	}
	if part := settingsListPartByCode(settingsPageFwdBlock, "s"); part == nil || part.Step != 1 || part.WordKey != "settings_entries_part_source" {
		t.Errorf("the page's part is %+v, want step 1 with the source word", part)
	}
	def, _ := settingsListByPage(settingsPageFwdBlock)
	if def.Right != settingsRightStaffOwner || def.CountMax != 200 {
		t.Errorf("the table's right is %d and its cap %d, want the Staff Group creator's right and 200", def.Right, def.CountMax)
	}
	if actions := settingsFwdBlockActions(); actions[settingsActFwdBlockSet].Right != settingsRightStaffOwner || actions[settingsActFwdBlockOpen].Right != settingsRightAdmin {
		t.Errorf("the action set needs %d and the action screen %d, want the creator's right and the admin check", actions[settingsActFwdBlockSet].Right, actions[settingsActFwdBlockOpen].Right)
	}
}
