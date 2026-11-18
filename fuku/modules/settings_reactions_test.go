//go:build testtools

package modules

import (
	"context"
	"fmt"
	"strconv"
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

// settingsReactionsCleanup deletes the chats' reaction rows and cache keys before the test and again when it ends.
func settingsReactionsCleanup(t *testing.T, chatIDs ...int64) {
	t.Helper()
	clean := func() {
		db.DB.Where("chat_id IN ?", chatIDs).Delete(&models.Reactions{})
		for _, id := range chatIDs {
			dbcache.DeleteCache(dbcache.CacheKey("reactions", id))
		}
		dbcache.ResetLocalForTest()
	}
	clean()
	t.Cleanup(clean)
}

// settingsReactionsSeed stores reactions through the repository, as /addreaction does, from "keyword emoji" pairs.
func settingsReactionsSeed(t *testing.T, chatID int64, pairs ...string) {
	t.Helper()
	for _, pair := range pairs {
		f := strings.Fields(pair)
		if err := db.DB.Create(&models.Reactions{ChatID: chatID, Keyword: f[0], Emoji: f[1]}).Error; err != nil {
			t.Fatalf("seed %q: %v", pair, err)
		}
	}
}

// settingsReactionsStored is the chat's stored reactions as a sorted "keyword|emoji" list.
func settingsReactionsStored(t *testing.T, chatID int64) string {
	t.Helper()
	return reactionGoldenStored(t, chatID)
}

// settingsReactionsIDOf is the row ID of the chat's reaction for keyword.
func settingsReactionsIDOf(t *testing.T, chatID int64, keyword string) int64 {
	t.Helper()
	var row models.Reactions
	if err := db.DB.Where("chat_id = ? AND keyword = ?", chatID, keyword).Take(&row).Error; err != nil {
		t.Fatalf("find %q: %v", keyword, err)
	}
	return int64(row.ID)
}

// newSettingsReactionsEnv is the list environment with the Reactions module loaded and the env chat's rows cleaned.
func newSettingsReactionsEnv(t *testing.T) *settingsListEnv {
	t.Helper()
	env := newSettingsListEnv(t)
	LoadReactions(env.dispatcher)
	settingsReactionsCleanup(t, env.chat.Id)
	return env
}

// settingsReactionsBegin opens the page as user and presses Add.
func settingsReactionsBegin(env *settingsListEnv, user gotgbot.User) *settingsBlacklistMenu {
	env.t.Helper()
	m := settingsListOpenMenuPage(env, env.chat, user, settingsPageReactions)
	add, ok := m.markedButton("settings_btn_rct_add")
	if !ok {
		env.t.Fatalf("the Reactions page has no Add button")
	}
	m.tap(add)
	return m
}

func TestSettingsReactionsOpens(t *testing.T) {
	setCachedBotUsername(t, "FukuTestBot")
	env := newSettingsReactionsEnv(t)
	settingsReactionsSeed(t, env.chat.Id, "gamma 🔥", "alpha 👍", "b<c> ❤")
	_, text, keyboard := settingsListOpenPrivate(env, env.admin, settingsPageReactions)
	for _, want := range []string{
		staffMarker("settings_page_reactions"), staffMarker("settings_rct_intro"), staffMarker("settings_rct_count") + " 3 100",
		"1. <code>alpha</code> → 👍", "2. <code>b&lt;c&gt;</code> → ❤", "3. <code>gamma</code> → 🔥",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the page lacks %q:\n%s", want, text)
		}
	}
	if got := settingsListRowWidths(keyboard); fmt.Sprint(got) != fmt.Sprint([]int{3, 1, 2}) {
		t.Errorf("row widths = %v, want the delete row, Add and the navigation row", got)
	}

	msgID := env.updateID()
	env.pressPrivate(env.admin, msgID, env.privateFields(env.chat.Id, "a", settingsActBack))
	mainText, _ := env.lastEdit(env.admin.Id, msgID)
	if line, ok := settingsMainStateLine(mainText, "settings_page_reactions"); !ok || !strings.HasSuffix(line, " 3/100") {
		t.Errorf("Main's Reactions line = %q, want it to end in 3/100", line)
	}

	// A group menu shows In private on Main and only the open-in-private note on the page.
	groupMsg := env.openMenu(env.admin)
	groupMain, _ := env.lastEdit(env.chat.Id, groupMsg)
	if line, ok := settingsMainStateLine(groupMain, "settings_page_reactions"); !ok || !strings.HasSuffix(line, staffMarker("settings_state_private_only")) {
		t.Errorf("a group Main's Reactions line = %q, want In private", line)
	}
	env.press(env.admin, groupMsg, env.groupFields(env.admin, "a", settingsActOpen, "p", settingsPageReactions))
	note, _ := env.lastEdit(env.chat.Id, groupMsg)
	if !strings.Contains(note, staffMarker("settings_entries_group_note")) || strings.Contains(note, "alpha") {
		t.Errorf("the group page drew:\n%s", note)
	}
}

func TestSettingsReactionsAdd(t *testing.T) {
	env := newSettingsReactionsEnv(t)
	owner := settingsFiltersOwner(env, env.chat)
	settingsReactionsSeed(t, env.chat.Id, "taken 👍")
	settingsReactionsBegin(env, owner)
	if got := env.redis.HGet(settingsInputKey(owner.Id), "t"); got != "r" {
		t.Fatalf("the wait's part = %q, want r", got)
	}

	for _, tc := range []struct{ text, wantKey string }{
		{"hello", "settings_rct_refuse_two"},
		{"a b c", "settings_rct_refuse_two"},
		{"hello x", "settings_rct_refuse_emoji"},
		{"Taken 🔥", "settings_rct_refuse_exists"},
		{strings.Repeat("k", 101) + " 🔥", "settings_rct_refuse_long"},
	} {
		sent := len(env.fake.sentTo(owner.Id))
		settingsListDoorSay(env, owner, tc.text)
		replies := env.fake.sentTo(owner.Id)[sent:]
		if len(replies) != 1 || !strings.Contains(sentText(replies[0]), staffMarker(tc.wantKey)) {
			t.Errorf("%q: replies %+v, want one %s refusal", tc.text, replies, tc.wantKey)
		}
		if got := env.redis.HGet(settingsInputKey(owner.Id), "t"); got != "r" {
			t.Errorf("%q: the wait ended, want it kept", tc.text)
		}
	}
	if got := settingsReactionsStored(t, env.chat.Id); got != "taken|👍" {
		t.Fatalf("refusals stored %q", got)
	}

	sent := len(env.fake.sentTo(owner.Id))
	settingsListDoorSay(env, owner, "Hello ❤️")
	if got := settingsReactionsStored(t, env.chat.Id); got != "hello|❤;taken|👍" {
		t.Fatalf("stored = %q, want hello with the plain heart", got)
	}
	replies := env.fake.sentTo(owner.Id)[sent:]
	if len(replies) == 0 || !strings.Contains(sentText(replies[0]), staffMarker("settings_rct_saved")+" hello") || !strings.Contains(sentText(replies[0]), "<code>hello</code> → ❤") {
		t.Errorf("the saved page = %+v, want the list with the saved notice", replies)
	}
	if env.redis.Exists(settingsInputKey(owner.Id)) {
		t.Errorf("the wait is still there after a save")
	}

	// The bot now reacts to a member's message containing the word, and stops once it is deleted on the page.
	member := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Member"}
	env.sendIn(env.chat, member, "well HELLO there")
	if calls := env.fake.callsFor("setMessageReaction"); len(calls) != 1 {
		t.Fatalf("setMessageReaction calls = %d, want 1", len(calls))
	}
	m := settingsListOpenMenuPage(env, env.chat, owner, settingsPageReactions)
	m.tap(m.button("🗑 1"))
	yes, _ := m.markedButton("settings_btn_yes")
	m.tap(yes)
	env.sendIn(env.chat, member, "hello again")
	if calls := env.fake.callsFor("setMessageReaction"); len(calls) != 1 {
		t.Errorf("setMessageReaction calls = %d after the delete, want still 1", len(calls))
	}
}

func TestSettingsReactionsDelete(t *testing.T) {
	env := newSettingsReactionsEnv(t)
	owner := settingsFiltersOwner(env, env.chat)
	other := env.addChat()
	settingsReactionsCleanup(t, other.Id)
	settingsReactionsSeed(t, env.chat.Id, "alpha 👍", "beta 🔥")
	settingsReactionsSeed(t, other.Id, "foreign 👍")
	_ = settingsLookupReactions(env.chat.Id)
	m := settingsListOpenMenuPage(env, env.chat, owner, settingsPageReactions)

	m.tap(m.button("🗑 2"))
	text, _ := m.last()
	if !strings.Contains(text, staffMarker("settings_rct_delete_ask")+" beta") {
		t.Fatalf("the card does not name the keyword:\n%s", text)
	}
	yes, _ := m.markedButton("settings_btn_yes")
	if f := settingsButtonFields(t, yes); f["a"] != "xy" || f["r"] != strconv.FormatInt(settingsReactionsIDOf(t, env.chat.Id, "beta"), 10) {
		t.Errorf("Yes carries %v", f)
	}
	m.tap(yes)
	text, _ = m.last()
	if !strings.Contains(text, staffMarker("settings_rct_removed")+" beta") || settingsReactionsStored(t, env.chat.Id) != "alpha|👍" {
		t.Errorf("the delete drew:\n%s\nstored %q", text, settingsReactionsStored(t, env.chat.Id))
	}
	if env.redis.Exists(dbcache.CacheKey("reactions", env.chat.Id)) {
		t.Errorf("the delete did not drop the cache key")
	}

	foreign := strconv.FormatInt(settingsReactionsIDOf(t, other.Id, "foreign"), 10)
	m.press("a", "xy", "r", foreign, "e", settingsCardExpiry(time.Now()))
	if text, _ = m.last(); !strings.Contains(text, staffMarker("settings_entries_already_removed")) {
		t.Errorf("a foreign ID's Yes drew:\n%s", text)
	}
	if got := settingsReactionsStored(t, other.Id); got != "foreign|👍" {
		t.Errorf("a forged ID deleted another chat's reaction: %q", got)
	}
}

// settingsLookupReactions fills the chat's reactions cache key, as the watcher does.
func settingsLookupReactions(chatID int64) map[string]string {
	return reactionWrites.Lookup(context.Background(), chatID)
}

func TestSettingsReactionsCap(t *testing.T) {
	env := newSettingsReactionsEnv(t)
	owner := settingsFiltersOwner(env, env.chat)
	pairs := make([]string, 0, reactionsCapMax)
	for i := 0; i < reactionsCapMax; i++ {
		pairs = append(pairs, fmt.Sprintf("w%03d 👍", i))
	}
	settingsReactionsSeed(t, env.chat.Id, pairs...)
	m := settingsListOpenMenuPage(env, env.chat, owner, settingsPageReactions)
	add, _ := m.markedButton("settings_btn_rct_add")
	m.tap(add)
	text, _ := m.last()
	if !strings.Contains(text, staffMarker("settings_rct_refuse_cap")) {
		t.Errorf("Add at the cap drew:\n%s", text)
	}
	if env.redis.Exists(settingsInputKey(owner.Id)) {
		t.Errorf("Add at the cap made a wait")
	}
}

func TestSettingsParityReactionAdd(t *testing.T) {
	env := newSettingsReactionsEnv(t)
	owner := settingsFiltersOwner(env, env.chat)
	twin := env.addChat()
	settingsReactionsCleanup(t, twin.Id)
	env.fake.setMember(twin.Id, owner.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusCreator})
	env.fake.setCreator(twin.Id, owner.Id)

	settingsReactionsBegin(env, owner)
	settingsListDoorSay(env, owner, "Gift 🎉")
	env.sendIn(twin, owner, "/addreaction Gift 🎉")
	if page, command := settingsReactionsStored(t, env.chat.Id), settingsReactionsStored(t, twin.Id); page != command || page != "gift|🎉" {
		t.Errorf("the page stored %q and /addreaction %q, want the same row", page, command)
	}

	// Drift: a reaction added or removed by the commands is what the page draws.
	env.sendIn(env.chat, owner, "/addreaction zeta 😁")
	_, text, _ := settingsListOpenPrivate(env, owner, settingsPageReactions)
	if !strings.Contains(text, "<code>zeta</code> → 😁") {
		t.Errorf("a reaction added by /addreaction is not drawn:\n%s", text)
	}
	env.sendIn(env.chat, owner, "/removereaction zeta")
	if _, text, _ = settingsListOpenPrivate(env, owner, settingsPageReactions); strings.Contains(text, "zeta") {
		t.Errorf("a reaction removed by /removereaction is still drawn:\n%s", text)
	}
}

func TestSettingsAuthorityParityReactions(t *testing.T) {
	env := newSettingsReactionsEnv(t)
	settingsReactionsSeed(t, env.chat.Id, "alpha 👍")
	// The cached administrator list the command reads agrees with the live record: no can_change_info.
	listEditAdmin(env.settingsEnv, env.chat, env.admin, false)
	cache.InvalidateAdminCache(env.chat.Id)
	// env.admin is a live administrator without can_change_info: the page lets them view, refuses every write, and
	// /addreaction refuses them too.
	m := settingsListOpenMenuPage(env, env.chat, env.admin, settingsPageReactions)
	if text, _ := m.last(); !strings.Contains(text, "<code>alpha</code>") {
		t.Fatalf("an administrator cannot view the page:\n%s", text)
	}
	for _, button := range []gotgbot.InlineKeyboardButton{m.button("🗑 1")} {
		m.tap(button)
		if text, alert := env.lastAnswer(); !alert || !strings.Contains(text, staffMarker("settings_no_change_info")) {
			t.Errorf("the delete ask answered %q (alert %v), want settings_no_change_info", text, alert)
		}
	}
	add, _ := m.markedButton("settings_btn_rct_add")
	m.tap(add)
	if text, alert := env.lastAnswer(); !alert || !strings.Contains(text, staffMarker("settings_no_change_info")) {
		t.Errorf("Add answered %q (alert %v), want settings_no_change_info", text, alert)
	}
	if env.redis.Exists(settingsInputKey(env.admin.Id)) {
		t.Errorf("a refused Add made a wait")
	}
	env.sendIn(env.chat, env.admin, "/addreaction beta 🔥")
	if got := settingsReactionsStored(t, env.chat.Id); got != "alpha|👍" {
		t.Errorf("/addreaction by the same administrator stored %q", got)
	}
}

func TestSettingsReactionsReadsCreateNothing(t *testing.T) {
	env := newSettingsReactionsEnv(t)
	chat := env.addChat()
	settingsReactionsCleanup(t, chat.Id)
	m := settingsListOpenMenuPage(env, chat, env.admin, settingsPageReactions)
	if text, _ := m.last(); !strings.Contains(text, staffMarker("settings_rct_empty")) {
		t.Errorf("an empty chat's page drew:\n%s", text)
	}
	var n int64
	db.DB.Model(&models.Reactions{}).Where("chat_id = ?", chat.Id).Count(&n)
	if n != 0 || env.redis.Exists(dbcache.CacheKey("reactions", chat.Id)) {
		t.Errorf("opening the page left %d rows and cache key %v", n, env.redis.Exists(dbcache.CacheKey("reactions", chat.Id)))
	}
}

// TestSettingsListReactionsFits holds every button of the page at most 64 bytes at the worst row ID and page, and the
// list page, the card, the prompt and the longest refusals inside 3800 UTF-16 units in all 8 languages.
func TestSettingsListReactionsFits(t *testing.T) {
	withRealLocales(t)
	def := settingsReactionsDef()
	worst := make([]settingsListEntry, 0, 24)
	for i := 0; i < 24; i++ {
		worst = append(worst, settingsListEntry{ID: 12345678901234567 - int64(i), Text: strings.Repeat("&", 99) + string(rune('a'+i)), Detail: "❤‍🔥"})
	}
	for _, code := range memberReqScreenLanguages {
		t.Run(code, func(t *testing.T) {
			tr := i18n.MustNewTranslator(code)
			views := settingsBlacklistWorstViews(tr)
			private := views["private"]
			private.GroupTitle = strings.Repeat("Ж", 64)
			long := strings.Repeat("&", 100)
			for name, screen := range map[string]settingsScreen{
				"list":   renderSettingsList(private, def, worst, 2, settingsNoticeOK(settingsListEntryText(tr, "settings_rct_saved", "keyword", long))+"\n"+settingsText(tr, "settings_edit_album_cut")),
				"card":   renderSettingsListCard(private, def, worst[0], 9999),
				"prompt": renderSettingsListPrompt(private, def, "r", time.Now(), true, ""),
				"refuse": renderSettingsInputRefusalCode(private, def.CancelCode, "r", settingsListEntryText(tr, "settings_rct_refuse_exists", "keyword", long)),
				"emoji":  renderSettingsInputRefusalCode(private, def.CancelCode, "r", settingsText(tr, "settings_rct_refuse_emoji", i18n.TranslationParams{"examples": settingsReactionExamples})),
				"note":   renderSettingsPrivateOnlyNote(views["group"], def.TitleKey, "FukuTestBot"),
			} {
				if units := panelUTF16Len(screen.Text); units == 0 || units > 3800 {
					t.Errorf("%s is %d UTF-16 units", name, units)
				}
				if screen.Omitted != 0 {
					t.Errorf("%s omitted %d buttons", name, screen.Omitted)
				}
				for _, button := range settingsAllButtons(screen.Keyboard.InlineKeyboard) {
					if len(button.CallbackData) > 64 {
						t.Errorf("%s: %q carries %d bytes", name, button.Text, len(button.CallbackData))
					}
					if n := len([]rune(button.Text)); n > 20 && button.Url == "" {
						t.Errorf("%s: %q has %d code points", name, button.Text, n)
					}
				}
			}
		})
	}
}

// sentText is the text of a message the fake recorded.
func sentText(m staffSentMessage) string {
	text, _ := m.Params["text"].(string)
	return text
}
