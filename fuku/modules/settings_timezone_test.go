//go:build testtools

package modules

import (
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
)

// newSettingsTimezoneEnv is the list environment with /timezone loaded and the env chat's zone cleaned.
func newSettingsTimezoneEnv(t *testing.T) *settingsListEnv {
	t.Helper()
	setCachedBotUsername(t, "FukuTestBot")
	env := newSettingsListEnv(t)
	LoadTimezone(env.dispatcher)
	timezoneCleanup(t, env.chat.Id)
	return env
}

// settingsTimezoneStored is the chat's stored zone, read fresh ("" for none).
func settingsTimezoneStored(t *testing.T, chatID int64) string {
	t.Helper()
	var row models.ChatTimezone
	if err := db.DB.Where("chat_id = ?", chatID).Take(&row).Error; err != nil {
		return ""
	}
	return row.TZ
}

func TestSettingsTimezoneOpens(t *testing.T) {
	env := newSettingsTimezoneEnv(t)
	owner := settingsFiltersOwner(env, env.chat)
	_, text, keyboard := settingsListOpenPrivate(env, owner, settingsPageTimezone)
	for _, want := range []string{staffMarker("settings_page_tz"), staffMarker("settings_tz_current") + " Europe/Kyiv", staffMarker("settings_tz_is_default")} {
		if !strings.Contains(text, want) {
			t.Errorf("the page lacks %q:\n%s", want, text)
		}
	}
	if got := settingsListRowWidths(keyboard); len(got) != 5 || got[0] != 3 || got[3] != 2 {
		t.Errorf("rows = %v, want three preset rows, the default and Type another, and the navigation row", got)
	}
	msgID := env.updateID()
	env.pressPrivate(owner, msgID, env.privateFields(env.chat.Id, "a", settingsActBack))
	mainText, _ := env.lastEdit(owner.Id, msgID)
	if line, ok := settingsMainStateLine(mainText, "settings_page_tz"); !ok || !strings.Contains(line, "UTC+") {
		t.Errorf("Main's Timezone line = %q, want the zone's label", line)
	}
	var n int64
	db.DB.Model(&models.ChatTimezone{}).Where("chat_id = ?", env.chat.Id).Count(&n)
	if n != 0 {
		t.Errorf("opening the page created %d rows", n)
	}
}

func TestSettingsParityTimezone(t *testing.T) {
	env := newSettingsTimezoneEnv(t)
	owner := settingsFiltersOwner(env, env.chat)
	twin := env.addChat()
	timezoneCleanup(t, twin.Id)
	env.fake.setMember(twin.Id, owner.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusCreator})
	env.fake.setCreator(twin.Id, owner.Id)

	m := settingsListOpenMenuPage(env, env.chat, owner, settingsPageTimezone)
	m.press("a", settingsActSet, "p", settingsPageTimezone, "t", settingsTimezoneControlKey, "v", "wa")
	env.sendIn(twin, owner, "/timezone Europe/Warsaw")
	if page, command := settingsTimezoneStored(t, env.chat.Id), settingsTimezoneStored(t, twin.Id); page != command || page != "Europe/Warsaw" {
		t.Errorf("the page stored %q and /timezone %q", page, command)
	}
	if text, _ := m.last(); !strings.Contains(text, settingsSelectedPrefix+staffMarker("settings_tz_city_warsaw")) && !strings.Contains(text, "Europe/Warsaw") {
		t.Errorf("the redraw does not show Warsaw:\n%s", text)
	}

	// Drift: a zone the command stored is what the page draws, and default resets both ways.
	env.sendIn(env.chat, owner, "/timezone +5:30")
	m.press("a", settingsActOpen, "p", settingsPageTimezone)
	if text, _ := m.last(); !strings.Contains(text, "UTC+05:30") || !strings.Contains(text, "UTC+5:30") {
		t.Errorf("a zone set by /timezone is not drawn:\n%s", text)
	}
	m.press("a", settingsActSet, "p", settingsPageTimezone, "t", settingsTimezoneControlKey, "v", "d")
	if got := settingsTimezoneStored(t, env.chat.Id); got != "" {
		t.Errorf("Use default left %q", got)
	}
}

func TestSettingsAuthorityParityTimezone(t *testing.T) {
	env := newSettingsTimezoneEnv(t)
	// The cached administrator list the command reads agrees with the live record: no can_change_info.
	listEditAdmin(env.settingsEnv, env.chat, env.admin, false)
	cache.InvalidateAdminCache(env.chat.Id)
	// env.admin is an administrator without can_change_info: the page and /timezone both refuse the change,
	// and both still show the zone.
	m := settingsListOpenMenuPage(env, env.chat, env.admin, settingsPageTimezone)
	m.press("a", settingsActSet, "p", settingsPageTimezone, "t", settingsTimezoneControlKey, "v", "wa")
	if text, alert := env.lastAnswer(); !alert || !strings.Contains(text, staffMarker("settings_no_change_info")) {
		t.Errorf("the press answered %q (alert %v), want settings_no_change_info", text, alert)
	}
	env.sendIn(env.chat, env.admin, "/timezone Europe/Warsaw")
	if got := settingsTimezoneStored(t, env.chat.Id); got != "" {
		t.Errorf("a refused change stored %q", got)
	}
	before := len(env.fake.sentTo(env.chat.Id))
	env.sendIn(env.chat, env.admin, "/timezone")
	sent := env.fake.sentTo(env.chat.Id)[before:]
	if len(sent) != 1 || !strings.Contains(sentText(sent[0]), staffMarker("timezone_current")) {
		t.Errorf("an administrator's /timezone answered %+v, want the current zone", sent)
	}
}

func TestSettingsTimezoneTyped(t *testing.T) {
	env := newSettingsTimezoneEnv(t)
	owner := settingsFiltersOwner(env, env.chat)
	m := settingsListOpenMenuPage(env, env.chat, owner, settingsPageTimezone)
	m.press("a", settingsActTimezoneType)
	if text, _ := m.last(); !strings.Contains(text, staffMarker("settings_tz_wait")) {
		t.Fatalf("Type another drew:\n%s", text)
	}
	sent := len(env.fake.sentTo(owner.Id))
	settingsListDoorSay(env, owner, "Mars/Olympus")
	replies := env.fake.sentTo(owner.Id)[sent:]
	if len(replies) != 1 || !strings.Contains(sentText(replies[0]), staffMarker("timezone_invalid")) {
		t.Errorf("a bad zone answered %+v", replies)
	}
	if !env.redis.Exists(settingsInputKey(owner.Id)) {
		t.Fatalf("the refusal ended the wait")
	}
	settingsListDoorSay(env, owner, "asia/tbilisi")
	if got := settingsTimezoneStored(t, env.chat.Id); got != "Asia/Tbilisi" {
		t.Errorf("the typed zone stored %q", got)
	}
	if env.redis.Exists(settingsInputKey(owner.Id)) {
		t.Errorf("the wait is still there after a save")
	}

	// A group menu's Type another makes no wait and points to the private chat.
	groupMsg := env.openMenu(owner)
	env.press(owner, groupMsg, env.groupFields(owner, "a", settingsActOpen, "p", settingsPageTimezone))
	env.press(owner, groupMsg, env.groupFields(owner, "a", settingsActTimezoneType))
	text, _ := env.lastEdit(env.chat.Id, groupMsg)
	if !strings.Contains(text, staffMarker("settings_tz_group_note")) || env.redis.Exists(settingsInputKey(owner.Id)) {
		t.Errorf("a group menu's Type another drew:\n%s", text)
	}
}

// TestSettingsTimezoneFits holds every button at most 64 bytes and 20 code points and every screen inside 3800
// UTF-16 units in all 8 languages, for a stored zone and the default, in both menus.
func TestSettingsTimezoneFits(t *testing.T) {
	withRealLocales(t)
	for _, code := range memberReqScreenLanguages {
		tr := i18n.MustNewTranslator(code)
		views := settingsBlacklistWorstViews(tr)
		for viewName, v := range views {
			v.GroupTitle = strings.Repeat("Ж", 64)
			for _, stored := range []string{"", "America/Argentina/ComodRivadavia"} {
				screens := map[string]settingsScreen{
					"page":   renderSettingsTimezone(v, stored, time.Now(), "FukuTestBot", settingsNoticeWarn(settingsText(tr, "settings_tz_group_note"))),
					"prompt": renderSettingsTimezonePrompt(v, time.Now(), true),
				}
				for name, screen := range screens {
					if units := panelUTF16Len(screen.Text); units == 0 || units > 3800 {
						t.Errorf("%s %s %s is %d UTF-16 units", code, viewName, name, units)
					}
					for _, button := range settingsAllButtons(screen.Keyboard.InlineKeyboard) {
						if len(button.CallbackData) > 64 {
							t.Errorf("%s %s %s: %q carries %d bytes", code, viewName, name, button.Text, len(button.CallbackData))
						}
						if n := len([]rune(button.Text)); n > 20 && button.Url == "" {
							t.Errorf("%s %s %s: %q has %d code points", code, viewName, name, button.Text, n)
						}
					}
				}
			}
		}
	}
	dbcache.ResetLocalForTest()
}
