//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
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
	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
	"github.com/uasneppy/fuku_robot/fuku/utils/content"
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

// settingsFiltersCommand runs `/filter <keyword>` (or `/stop <keyword>`) as from in chat through the real dispatcher, as a
// reply to reply when reply is not nil.
func settingsFiltersCommand(env *settingsListEnv, chat gotgbot.Chat, from gotgbot.User, text string, reply *gotgbot.Message) {
	env.t.Helper()
	id := env.updateID()
	msg := &gotgbot.Message{MessageId: id, Date: 1, Chat: chat, From: &from, Text: text}
	if reply != nil {
		replied := *reply
		replied.MessageId, replied.Date, replied.Chat, replied.From = env.updateID(), 1, chat, &from
		msg.ReplyToMessage = &replied
	}
	if err := env.dispatcher.ProcessUpdate(env.bot, &gotgbot.Update{UpdateId: id, Message: msg}, nil); err != nil {
		env.t.Fatalf("ProcessUpdate(%d) error = %v", id, err)
	}
}

// settingsFiltersCommandTwin makes the command twin of the flow's chat: a second chat of the env whose creator is the same
// owner, with the command module loaded on the env's dispatcher.
func settingsFiltersCommandTwin(f *settingsFiltersFlow) gotgbot.Chat {
	f.env.t.Helper()
	twin := f.env.addChat()
	settingsListCleanup(f.env.t, twin.Id)
	f.env.fake.setMember(twin.Id, f.user.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusCreator})
	f.env.fake.setCreator(twin.Id, f.user.Id)
	return twin
}

// settingsFiltersRowShape is the stored row as one comparable value: everything but its ID, chat and times.
func settingsFiltersRowShape(row db.ChatFilters) string {
	return fmt.Sprintf("%q|%q|%d|%q|%+v|%v", row.KeyWord, row.FilterReply, row.MsgType, row.FileID, []db.Button(row.Buttons), row.NoNotif)
}

func TestSettingsParityFilterAdd(t *testing.T) {
	env := newSettingsListEnv(t)
	LoadFilters(env.dispatcher)
	f := newSettingsFiltersFlow(env, env.chat)
	pageChat := env.chat
	commandChat := settingsFiltersCommandTwin(f)

	photo := settingsMediaItemID("photo", "parity-photo")
	photo.Caption = "Our prices"
	photo.CaptionEntities = []gotgbot.MessageEntity{{Type: "bold", Offset: 4, Length: 6}}
	sticker := settingsMediaItemID("sticker", "parity-sticker")
	cases := []struct {
		name    string
		message gotgbot.Message
	}{
		{"a text with entities and newlines", gotgbot.Message{
			Text: "Hello\nbold world", Entities: []gotgbot.MessageEntity{{Type: "bold", Offset: 6, Length: 4}},
		}},
		{"a photo with a caption", photo},
		{"a sticker", sticker},
		{"a text with markdown buttons, one with a javascript link", gotgbot.Message{
			Text: "Open [Docs](buttonurl://https://example.com) or [Bad](buttonurl://javascript:alert(1))",
		}},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			keyword := fmt.Sprintf("parity%d", i)
			pageKeys := settingsFiltersPrimeCaches(t, env, pageChat.Id)
			commandKeys := settingsFiltersPrimeCaches(t, env, commandChat.Id)

			f.begin()
			f.say(keyword)
			sendPrivateMessage(env.settingsEnv, f.user, tc.message)
			settingsFiltersCommand(env, commandChat, f.user, "/filter "+keyword, &tc.message)

			pageRow, ok := settingsFiltersStored(t, pageChat.Id)[keyword]
			if !ok {
				t.Fatalf("the page stored no filter for %q: %v", keyword, settingsFiltersStored(t, pageChat.Id))
			}
			commandRow, ok := settingsFiltersStored(t, commandChat.Id)[keyword]
			if !ok {
				t.Fatalf("/filter stored no filter for %q: %v", keyword, settingsFiltersStored(t, commandChat.Id))
			}
			if got, want := settingsFiltersRowShape(pageRow), settingsFiltersRowShape(commandRow); got != want {
				t.Errorf("the page stored %s, /filter stored %s", got, want)
			}
			for _, key := range append(pageKeys, commandKeys...) {
				if env.redis.Exists(key) {
					t.Errorf("the cache key %s survived the add", key)
				}
			}
		})
	}
}

func TestSettingsParityFilterRemove(t *testing.T) {
	env := newSettingsListEnv(t)
	LoadFilters(env.dispatcher)
	f := newSettingsFiltersFlow(env, env.chat)
	pageChat := env.chat
	commandChat := settingsFiltersCommandTwin(f)
	for _, chat := range []gotgbot.Chat{pageChat, commandChat} {
		settingsListSeedFilters(t, chat.Id, "alpha", "beta", "gamma")
	}
	pageKeys := settingsFiltersPrimeCaches(t, env, pageChat.Id)
	commandKeys := settingsFiltersPrimeCaches(t, env, commandChat.Id)

	m := settingsListOpenMenuPage(env, pageChat, f.user, settingsPageFilters)
	m.tap(m.button("🗑 2"))
	yes, _ := m.markedButton("settings_btn_yes")
	m.tap(yes)
	settingsFiltersCommand(env, commandChat, f.user, "/stop beta", nil)

	shapes := func(chatID int64) []string {
		var out []string
		for _, row := range settingsFiltersStored(t, chatID) {
			out = append(out, settingsFiltersRowShape(row))
		}
		slices.Sort(out)
		return out
	}
	if page, command := shapes(pageChat.Id), shapes(commandChat.Id); !reflect.DeepEqual(page, command) || len(page) != 2 {
		t.Errorf("the page left %v and /stop left %v, want the same two rows", page, command)
	}
	for _, key := range append(pageKeys, commandKeys...) {
		if env.redis.Exists(key) {
			t.Errorf("the cache key %s survived the delete", key)
		}
	}
}

func TestSettingsDriftFilters(t *testing.T) {
	env := newSettingsListEnv(t)
	LoadFilters(env.dispatcher)
	f := newSettingsFiltersFlow(env, env.chat)
	m := settingsListOpenMenuPage(env, env.chat, f.user, settingsPageFilters)
	draw := func() string {
		t.Helper()
		m.press("a", settingsActOpen, "p", settingsPageFilters)
		text, _ := m.last()
		return text
	}
	mainLine := func() string {
		t.Helper()
		msgID := env.updateID()
		env.pressPrivate(f.user, msgID, env.privateFields(env.chat.Id, "a", settingsActBack))
		text, _ := env.lastEdit(f.user.Id, msgID)
		line, _ := settingsMainStateLine(text, "settings_page_filters")
		return line
	}
	text := &gotgbot.Message{Text: "the reply"}

	settingsFiltersCommand(env, env.chat, f.user, "/filter spam", text)
	settingsFiltersCommand(env, env.chat, f.user, "/filter scam", text)
	out := draw()
	for _, want := range []string{"1. <code>scam</code>", "2. <code>spam</code>", staffMarker("settings_flt_count") + " 2 150"} {
		if !strings.Contains(out, want) {
			t.Errorf("a filter added by /filter is not drawn (%q):\n%s", want, out)
		}
	}
	if line := mainLine(); !strings.HasSuffix(line, " 2/150") {
		t.Errorf("Main's Filters line = %q, want it to end in 2/150", line)
	}
	settingsFiltersCommand(env, env.chat, f.user, "/stop spam", nil)
	if out = draw(); strings.Contains(out, "<code>spam</code>") || !strings.Contains(out, staffMarker("settings_flt_count")+" 1 150") {
		t.Errorf("a filter removed by /stop is still drawn:\n%s", out)
	}
	if line := mainLine(); !strings.HasSuffix(line, " 1/150") {
		t.Errorf("Main's Filters line = %q after /stop, want it to end in 1/150", line)
	}
}

func TestSettingsAuthorityParityFilters(t *testing.T) {
	type scenario struct {
		name string
		// member is the presser's member record and listed makes it appear in the cached administrator list with
		// can_change_info (both false for the env's administrator, who lacks the right).
		setup func(env *settingsListEnv, chat gotgbot.Chat) gotgbot.User
		// wantKey is the refusal the menu answers as an alert; empty means the menu writes.
		wantKey string
		// typedKey is what the door answers a typed step with; empty means it writes.
		typedKey string
		// commandWrites is whether /filter writes for the same records.
		commandWrites bool
	}
	scenarios := []scenario{
		{name: "an administrator without can_change_info", wantKey: "settings_no_change_info", typedKey: "settings_no_change_info",
			setup: func(env *settingsListEnv, chat gotgbot.Chat) gotgbot.User {
				// The live record and the cached administrator list the command reads agree: no can_change_info.
				listEditAdmin(env.settingsEnv, chat, env.admin, false)
				cache.InvalidateAdminCache(chat.Id)
				return env.admin
			}},
		{name: "a plain member", wantKey: "settings_not_admin", typedKey: "settings_edit_refuse_access_lost",
			setup: func(env *settingsListEnv, chat gotgbot.Chat) gotgbot.User {
				user := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Presser"}
				env.fake.setMember(chat.Id, user.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
				return user
			}},
		{name: "the owner gate refuses a non-owner administrator who holds can_change_info", wantKey: "settings_gate_owner_only",
			typedKey: "settings_gate_owner_only", commandWrites: true,
			setup: func(env *settingsListEnv, chat gotgbot.Chat) gotgbot.User {
				user := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Presser"}
				listEditAdmin(env.settingsEnv, chat, user, true)
				env.newOwner(chat)
				env.setAdminsAllowed(chat.Id, false)
				return user
			}},
		{name: "every check held", commandWrites: true,
			setup: func(env *settingsListEnv, chat gotgbot.Chat) gotgbot.User {
				return settingsFiltersOwner(env, chat)
			}},
	}

	env := newSettingsListEnv(t)
	LoadFilters(env.dispatcher)
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			chat := env.addChat()
			settingsListCleanup(t, chat.Id)
			settingsListSeedFilters(t, chat.Id, "alpha", "beta")
			user := sc.setup(env, chat)
			t.Cleanup(func() {
				db.DB.Where("user_id = ?", user.Id).Delete(&models.User{})
				dbcache.DeleteCache(dbcache.CacheKey("user", user.Id))
				dbcache.ResetLocalForTest()
			})
			alphaID := settingsFiltersIDOf(t, chat.Id, "alpha")

			// The command first: it refuses for the same member records, or writes (the owner gate is the documented
			// carve-out, which gates the menu and not the command).
			settingsFiltersCommand(env, chat, user, "/filter viacommand", &gotgbot.Message{Text: "the reply"})
			if _, wrote := settingsFiltersStored(t, chat.Id)["viacommand"]; wrote != sc.commandWrites {
				t.Errorf("/filter wrote = %v, want %v (stored %v)", wrote, sc.commandWrites, settingsFiltersStored(t, chat.Id))
			}

			// A typed step is judged like a press. A wait is written directly (the Add press itself is refused when the
			// records refuse); the door then reads the same records live.
			for _, step := range []struct {
				name string
				part string
				text string
				key  string
			}{
				{"the keyword step", "k", "typedword", "typedword"},
				{"the reply step", "y", "the reply", "typedstep"},
			} {
				wait := settingsInputWait{GroupID: chat.Id, Page: settingsPageFilters, Part: "k", PromptMsgID: 1, Nonce: "0123456789abcdef"}
				if _, err := settingsInputSet(context.Background(), user.Id, wait); err != nil {
					t.Fatalf("%s: write the wait: %v", step.name, err)
				}
				if step.part == "y" {
					wait.Part, wait.Nonce, wait.Pending, wait.StepMsgID = "y", "fedcba9876543210", "typedstep", 1
					wait.ExpiresAt = time.Now().Add(settingsInputTTL)
					if advanced, err := settingsInputAdvance(context.Background(), user.Id, "0123456789abcdef", wait); err != nil || !advanced {
						t.Fatalf("%s: move the wait to step 2: %v, %v", step.name, advanced, err)
					}
				}
				before := len(env.fake.sentTo(user.Id))
				if step.part == "k" {
					settingsListDoorSay(env, user, step.text)
				} else {
					sendPrivateMessage(env.settingsEnv, user, gotgbot.Message{Text: step.text})
				}
				_, stored := settingsFiltersStored(t, chat.Id)[step.key]
				if sc.typedKey != "" {
					if got := fmt.Sprint(env.fake.sentTo(user.Id)[before].Params["text"]); !strings.Contains(got, staffMarker(sc.typedKey)) {
						t.Errorf("%s: answered %q, want %s", step.name, got, sc.typedKey)
					}
					if stored || env.redis.Exists(settingsInputKey(user.Id)) {
						t.Errorf("%s: a refused typed step wrote or left the wait (stored %v)", step.name, stored)
					}
					if sc.typedKey != "settings_edit_refuse_access_lost" && !strings.Contains(fmt.Sprint(env.fake.sentTo(user.Id)[before].Params["text"]), staffMarker("settings_edit_wait_stopped")) {
						t.Errorf("%s: the refusal does not say the wait stopped", step.name)
					}
				} else if step.part == "y" && !stored {
					t.Errorf("%s: the typed reply was not stored where the commands write", step.name)
				}
				if sc.typedKey == "" && step.part == "k" {
					// An accepted keyword moves the wait to the reply step; the next iteration replaces it.
					if got := env.redis.HGet(settingsInputKey(user.Id), "t"); got != "y" {
						t.Errorf("%s: the wait is at %q after an accepted keyword, want y", step.name, got)
					}
					continue
				}
				settingsEditWaitGone(t, env.settingsEnv, user)
			}
			if sc.typedKey != "" && sc.typedKey != "settings_gate_owner_only" && sc.commandWrites {
				t.Errorf("a typed step was refused where the commands write (%s), and that is not the owner-gate carve-out", sc.typedKey)
			}
			before := settingsFiltersStored(t, chat.Id)

			m := settingsListOpenMenuPage(env, chat, user, settingsPageFilters)
			presses := map[string][]string{
				"the delete ask": {"a", "ix", "r", strconv.FormatInt(alphaID, 10), "v", "1"},
				"the delete Yes": {"a", "iy", "r", strconv.FormatInt(alphaID, 10), "e", settingsCardExpiry(time.Now())},
				"Add":            {"a", "ia"},
			}
			for _, name := range []string{"the delete ask", "the delete Yes", "Add"} {
				answers, edits := env.answerCount(), len(env.edits(user.Id, m.msgID))
				m.press(presses[name]...)
				if got := env.answerCount() - answers; got != 1 {
					t.Errorf("%s: answered %d times, want once", name, got)
				}
				if sc.wantKey == "" {
					continue
				}
				if text, alert := env.lastAnswer(); !strings.Contains(text, staffMarker(sc.wantKey)) || !alert {
					t.Errorf("%s: answered %q (alert %v), want the alert %s", name, text, alert, sc.wantKey)
				}
				if got := len(env.edits(user.Id, m.msgID)); got != edits {
					t.Errorf("%s: an alert refusal edited the menu", name)
				}
			}
			after := settingsFiltersStored(t, chat.Id)
			if sc.wantKey != "" {
				if !reflect.DeepEqual(before, after) {
					t.Errorf("a refused press wrote: %v -> %v", before, after)
				}
				if env.redis.Exists(settingsInputKey(user.Id)) {
					t.Errorf("a refused Add created a wait")
				}
				return
			}
			if _, still := after["alpha"]; still {
				t.Errorf("the permitted Yes did not delete alpha: %v", after)
			}
			if !env.redis.Exists(settingsInputKey(user.Id)) {
				t.Errorf("the permitted Add created no wait")
			}
		})
	}
}

// settingsFiltersWorstEntries makes n entries with 17-digit row IDs, the worst case the delete buttons carry.
func settingsFiltersWorstEntries(n int) []settingsListEntry {
	entries := make([]settingsListEntry, 0, n)
	for i := 0; i < n; i++ {
		entries = append(entries, settingsListEntry{ID: 12345678901234500 + int64(i), Text: fmt.Sprintf("k%06d", i)})
	}
	return entries
}

func TestSettingsListFiltersCallbackBytes(t *testing.T) {
	withRealLocales(t)
	tr := i18n.MustNewTranslator("en")
	views := settingsBlacklistWorstViews(tr)
	private := views["private"]
	def := settingsFiltersDef()
	// 79,992 entries are 9,999 pages: the pager cursor v reaches its four digits.
	worst := settingsFiltersWorstEntries(settingsListPageSize * 9999)

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
			code := settingsButtonFields(t, button)["a"]
			sizes[code] = max(sizes[code], len(button.CallbackData))
		}
	}
	for name, page := range map[string]int{"first": 1, "middle": 5000, "page 9998": 9998, "last": 9999} {
		measure("list "+name, renderSettingsList(private, def, worst, page, ""))
	}
	measure("a one-entry list", renderSettingsList(private, def, worst[:1], 1, ""))
	measure("an empty list", renderSettingsList(private, def, nil, 1, ""))
	measure("the card", renderSettingsListCard(private, def, worst[0], 9999))
	until := time.Now()
	measure("the keyword prompt", renderSettingsListPrompt(private, def, "k", until, true, ""))
	measure("the reply prompt", renderSettingsListPrompt(private, def, "y", until, true, "keyword"))
	measure("the group note", renderSettingsPrivateOnlyNote(views["group"], def.TitleKey, "FukuTestBot"))

	for code, want := range map[string]int{"ig": 40, "ix": 60, "iy": 64, "ia": 33, "ic": 37} {
		if got := sizes[code]; got != want {
			t.Errorf("the largest %s button is %d bytes, the UI-SPEC table says %d", code, got, want)
		}
	}
	cell := func(v settingsView) int {
		screen := renderSettingsMain(v, []settingsMainEntry{{Key: settingsPageFilters, Icon: def.Icon, Title: "Filters", State: "1/150", Known: true}}, "")
		for _, button := range settingsAllButtons(screen.Keyboard.InlineKeyboard) {
			if f := settingsButtonFields(t, button); f["a"] == settingsActOpen {
				return len(button.CallbackData)
			}
		}
		return 0
	}
	if got := cell(private); got != 37 {
		t.Errorf("the Main cell is %d bytes in a private menu, want 37", got)
	}
	if got := cell(views["group"]); got != 33 {
		t.Errorf("the Main cell is %d bytes in a group menu, want 33", got)
	}

	t.Run("a row whose ID has 18 digits has no delete button and is counted", func(t *testing.T) {
		entries := settingsFiltersWorstEntries(3)
		entries[1].ID = 123456789012345678
		screen := renderSettingsList(private, def, entries, 1, "")
		if screen.Omitted != 1 {
			t.Errorf("Omitted = %d, want 1", screen.Omitted)
		}
		if got := len(screen.Keyboard.InlineKeyboard[0]); got != 2 {
			t.Errorf("the delete row has %d buttons, want 2", got)
		}
	})
}

func TestSettingsListFiltersPageFitsEveryLocale(t *testing.T) {
	withRealLocales(t)
	title := strings.Repeat("Ж", 64)
	ampersands := strings.Repeat("&", 100)
	def := settingsFiltersDef()
	// The entries a middle page of the worst entries shows: 32 ampersands each, five UTF-16 units once escaped.
	entries := make([]settingsListEntry, 0, 24)
	for i := 0; i < 24; i++ {
		entries = append(entries, settingsListEntry{ID: int64(10 + i), Text: strings.Repeat("&", 32) + string(rune('a'+i))})
	}
	widths := func(screen settingsScreen) string {
		return fmt.Sprint(settingsListRowWidths(screen.Keyboard.InlineKeyboard))
	}

	for _, code := range memberReqScreenLanguages {
		t.Run(code, func(t *testing.T) {
			tr := i18n.MustNewTranslator(code)
			views := settingsBlacklistWorstViews(tr)
			for name, v := range views {
				v.GroupTitle = title
				views[name] = v
			}
			private := views["private"]
			check := func(name string, screen settingsScreen, wantWidths string) {
				t.Helper()
				if units := panelUTF16Len(screen.Text); units == 0 || units > 3800 {
					t.Errorf("%s is %d UTF-16 units, want 1..3800", name, units)
				}
				if screen.Omitted != 0 {
					t.Errorf("%s omitted %d buttons", name, screen.Omitted)
				}
				if wantWidths != "" {
					if got := widths(screen); got != wantWidths {
						t.Errorf("%s has row widths %s, want %s", name, got, wantWidths)
					}
				}
				for _, button := range settingsAllButtons(screen.Keyboard.InlineKeyboard) {
					if label := []rune(button.Text); len(label) > 20 && !strings.HasPrefix(button.Text, "🗑") && button.Url == "" {
						t.Errorf("%s: button %q has %d code points, want at most 20", name, button.Text, len(label))
					}
				}
			}

			// A notice only adds its own length, so the two longest of the notices a page of this table can show hold the
			// worst case of every combination: the removed notice, the cap, the expiry and the save failure.
			notices := settingsBlacklistLongest([]string{
				settingsNoticeOK(settingsListEntryText(tr, "settings_flt_removed", "keyword", ampersands)),
				settingsNoticeWarn(settingsFiltersCapText(tr)),
				settingsNoticeWarn(settingsText(tr, "settings_card_expired")),
				settingsNoticeWarn(settingsText(tr, "common_settings_save_failed")),
			}, 2)
			for i, notice := range notices {
				check(fmt.Sprintf("the list page with notice %d", i), renderSettingsList(private, def, entries, 2, notice), "[4 4 2 1 2]")
			}
			check("the empty list", renderSettingsList(private, def, nil, 1, ""), "[1 2]")
			card := renderSettingsListCard(private, def, settingsListEntry{ID: 12345678901234567, Text: ampersands}, 9999)
			check("the card", card, "[2 2]")
			if !strings.Contains(card.Text, strings.Repeat("&amp;", 100)) {
				t.Errorf("the card does not name the 100-code-point keyword")
			}
			until := time.Now()
			check("the keyword prompt with the replaced note", renderSettingsListPrompt(private, def, "k", until, true, ""), "[1]")
			step2 := renderSettingsListPrompt(private, def, "y", until, true, ampersands)
			check("the reply prompt with a 100-code-point keyword and the replaced note", step2, "[1]")
			if !strings.Contains(step2.Text, strings.Repeat("&amp;", 100)) {
				t.Errorf("the reply prompt does not name the 100-code-point keyword")
			}

			// Every refusal reply, each with the Cancel button and the still-waiting line, and the replies that end a wait.
			refusals := []string{
				settingsText(tr, "settings_entries_refuse_blank"),
				settingsText(tr, "filters_keyword_too_long"),
				settingsText(tr, "filters_keyword_too_broad"),
				settingsFiltersExistsText(tr, ampersands),
				settingsFiltersCapText(tr),
				settingsText(tr, "settings_edit_refuse_need_text"),
				settingsText(tr, "content_need_filter_content"),
				content.TextLengthRefusal(strings.Repeat("x", 5000), db.TEXT, code),
				content.TextLengthRefusal(strings.Repeat("x", 2000), db.PHOTO, code),
				settingsText(tr, "common_settings_save_failed"),
			}
			for i, reason := range refusals {
				check(fmt.Sprintf("refusal %d", i), renderSettingsInputRefusalCode(private, def.CancelCode, "y", reason), "[1]")
				if ended := reason + "\n\n" + settingsText(tr, "settings_edit_wait_stopped"); panelUTF16Len(ended) > 3800 {
					t.Errorf("the wait-ended reply %d is %d UTF-16 units", i, panelUTF16Len(ended))
				}
			}
			saved := settingsNoticeOK(settingsListEntryText(tr, def.TwoStep.SavedKey, def.EntryParam, ampersands)) + "\n" + settingsText(tr, "settings_edit_album_cut")
			check("the saved page", renderSettingsList(private, def, entries, 2, saved), "[4 4 2 1 2]")
			check("the group note", renderSettingsPrivateOnlyNote(views["group"], def.TitleKey, "FukuTestBot"), "[1 2]")
		})
	}
}

func TestSettingsFiltersReadsCreateNothing(t *testing.T) {
	setCachedBotUsername(t, "FukuTestBot")
	env := newSettingsListEnv(t)
	chat := env.addChat()
	settingsListCleanup(t, chat.Id)
	count := func() (rows, chats int64) {
		db.DB.Model(&models.ChatFilters{}).Where("chat_id = ?", chat.Id).Count(&rows)
		db.DB.Model(&models.Chat{}).Where("chat_id = ?", chat.Id).Count(&chats)
		return rows, chats
	}
	keys := []string{dbcache.CacheKey("filter_list", chat.Id), dbcache.CacheKey("filters_optimized", chat.Id)}

	// A private menu: Main, the page and the card of a forged ask on a chat with no filters.
	rows, chats := count()
	if rows != 0 {
		t.Fatalf("the fresh chat already has %d filters", rows)
	}
	owner := settingsFiltersOwner(env, chat)
	m := settingsListOpenMenuPage(env, chat, owner, settingsPageFilters)
	m.press("a", settingsActBack)
	m.press("a", settingsActOpen, "p", settingsPageFilters)
	m.press("a", "ix", "r", "42", "v", "1")
	if text, _ := m.last(); !strings.Contains(text, staffMarker("settings_entries_already_removed")) {
		t.Fatalf("the screens did not draw as expected:\n%s", text)
	}
	if gotRows, gotChats := count(); gotRows != 0 || gotChats != chats {
		t.Errorf("a private menu left %d filters and changed the chat rows from %d to %d", gotRows, chats, gotChats)
	}

	// A group menu: Main and the page's note read nothing and create nothing.
	groupMsg := env.openMenu(env.admin)
	env.press(env.admin, groupMsg, env.groupFields(env.admin, "a", settingsActOpen, "p", settingsPageFilters))
	if gotRows, _ := count(); gotRows != 0 {
		t.Errorf("a group menu left %d filters", gotRows)
	}
	for _, key := range keys {
		if env.redis.Exists(key) {
			t.Errorf("opening filled the command's cache key %s", key)
		}
	}
}
