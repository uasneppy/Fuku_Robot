//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/adminindex"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// settingsHubSeedGroup records chat in the roster with the given entries and stores its title,
// the way the sweep and the admin cache feed do, and cleans both up with the test.
func settingsHubSeedGroup(t *testing.T, chat gotgbot.Chat, title string, entries ...adminindex.Entry) {
	t.Helper()
	settingsCleanup(t, chat.Id)
	if err := db.DB.Create(&models.Chat{ChatId: chat.Id, ChatName: title}).Error; err != nil {
		t.Fatalf("store chat %d: %v", chat.Id, err)
	}
	if err := adminindex.ReplaceChat(context.Background(), chat.Id, entries); err != nil {
		t.Fatalf("seed roster of %d: %v", chat.Id, err)
	}
}

// settingsHubCreator is the roster entry of a group's creator.
func settingsHubCreator(user gotgbot.User) adminindex.Entry {
	return adminindex.Entry{UserID: user.Id, Role: models.AdminIndexRoleCreator}
}

func TestSettingsHubPick(t *testing.T) {
	env := newSettingsEnv(t)
	owner := env.newOwner(env.chat)
	settingsHubSeedGroup(t, env.chat, "Hub Group", settingsHubCreator(owner))
	lookups := len(env.fake.callsFor("getChatMember"))

	env.sendPrivate(owner, "/settings")

	list := settingsPrivateOnlySent(t, env, owner)
	text := fmt.Sprint(list.Params["text"])
	for _, want := range []string{
		staffMarker("settings_header"),
		staffMarker("settings_page_groups"),
		staffMarker("settings_list_intro"),
		staffMarker("settings_list_hint"),
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the list %q does not contain %q", text, want)
		}
	}
	if got := len(env.fake.callsFor("getChatMember")) - lookups; got != 0 {
		t.Errorf("building the list made %d getChatMember calls, want none", got)
	}
	keyboard := staffKeyboardOf(list.Params["reply_markup"])
	if len(keyboard) != 2 || len(keyboard[0]) != 1 || len(keyboard[1]) != 1 {
		t.Fatalf("the list keyboard = %+v, want one row button and Close", keyboard)
	}
	row, closeButton := keyboard[0][0], keyboard[1][0]
	if row.Text != "Hub Group" {
		t.Errorf("row label = %q, want the group's title", row.Text)
	}
	fields := settingsButtonFields(t, row)
	if fields["a"] != settingsActPick || fields["c"] != fmt.Sprint(env.chat.Id) || len(fields) != 2 {
		t.Errorf("row fields = %v, want a=pk and c=%d only", fields, env.chat.Id)
	}
	closeData, ok := decodeCallbackData(closeButton.CallbackData, settingsHubNamespace)
	if !ok || closeData.Fields["a"] != settingsHubActClose {
		t.Errorf("Close = %q, want shub a=cx", closeButton.CallbackData)
	}

	answers := env.answerCount()
	env.pressRaw(settingsPrivateChat(owner), owner, list.MessageID, row.CallbackData)

	if got := env.answerCount() - answers; got != 1 {
		t.Fatalf("the pick answered %d times, want once", got)
	}
	if got := len(env.edits(owner.Id, list.MessageID)); got != 1 {
		t.Fatalf("the list message was edited %d times, want once", got)
	}
	main, _ := env.lastEdit(owner.Id, list.MessageID)
	if !strings.Contains(main, staffMarker("settings_header_main")) || !strings.Contains(main, fmt.Sprint(env.chat.Id)) {
		t.Errorf("the pick drew %q, want the group's Main page", main)
	}
	if sent := env.fake.sentTo(owner.Id); len(sent) != 1 {
		t.Errorf("the pick sent a new message: %d messages in the private chat, want 1", len(sent))
	}
	if got := len(callsToChat(env.fake.staffBotClient, "deleteMessage", owner.Id)); got != 0 {
		t.Errorf("the pick deleted the list message %d times, want never", got)
	}
}

func TestSettingsHubEmpty(t *testing.T) {
	env := newSettingsEnv(t)
	stranger := gotgbot.User{Id: uniqueLinkOwnerID() + 41, FirstName: "Stranger"}

	env.sendPrivate(stranger, "/settings")

	msg := settingsPrivateOnlySent(t, env, stranger)
	text := fmt.Sprint(msg.Params["text"])
	for _, want := range []string{staffMarker("settings_list_empty"), staffMarker("settings_list_hint")} {
		if !strings.Contains(text, want) {
			t.Errorf("the empty list %q does not contain %q", text, want)
		}
	}
	buttons := settingsAllButtons(staffKeyboardOf(msg.Params["reply_markup"]))
	if len(buttons) != 1 || !containsMarker(buttons[0].Text, "settings_btn_close") {
		t.Errorf("the empty list buttons = %+v, want Close only", buttons)
	}
}

// withSettingsHubGroups replaces the list read with a function of the user ID, for a test that
// needs a list of a size the roster fixtures would make slow to seed.
func withSettingsHubGroups(t *testing.T, read func(userID int64) ([]adminindex.ListedGroup, error)) {
	t.Helper()
	previous := settingsHubLookup
	settingsHubLookup = func(_ context.Context, userID int64) ([]adminindex.ListedGroup, error) {
		return read(userID)
	}
	t.Cleanup(func() { settingsHubLookup = previous })
}

// settingsHubGroupsN makes n groups titled "Group 01", "Group 02", ... with distinct chat IDs.
func settingsHubGroupsN(n int) []adminindex.ListedGroup {
	groups := make([]adminindex.ListedGroup, 0, n)
	for i := 1; i <= n; i++ {
		groups = append(groups, adminindex.ListedGroup{ChatID: -1001000000000 - int64(i), Title: fmt.Sprintf("Group %02d", i)})
	}
	return groups
}

// settingsHubPressData presses a button carrying data on message msgID of user's private chat.
func settingsHubPressData(env *settingsEnv, user gotgbot.User, msgID int64, data string) {
	env.t.Helper()
	env.pressRaw(settingsPrivateChat(user), user, msgID, data)
}

// settingsHubPress presses a shub button built from fields.
func settingsHubPress(env *settingsEnv, user gotgbot.User, msgID int64, fields map[string]string) {
	env.t.Helper()
	data := encodeCallbackData(settingsHubNamespace, fields)
	if data == "" {
		env.t.Fatalf("shub callback data %v did not encode", fields)
	}
	settingsHubPressData(env, user, msgID, data)
}

// settingsHubPickedChats returns the chat IDs of the pick buttons of a keyboard, in order.
func settingsHubPickedChats(t *testing.T, keyboard [][]gotgbot.InlineKeyboardButton) []string {
	t.Helper()
	var chats []string
	for _, button := range settingsAllButtons(keyboard) {
		decoded, ok := decodeCallbackData(button.CallbackData, settingsCallbackNamespace)
		if ok && decoded.Fields["a"] == settingsActPick {
			chats = append(chats, decoded.Fields["c"])
		}
	}
	return chats
}

// settingsHubButtonTexts returns the labels of the pick buttons of a screen, in order.
func settingsHubRowLabels(keyboard [][]gotgbot.InlineKeyboardButton) []string {
	var labels []string
	for _, button := range settingsAllButtons(keyboard) {
		if decoded, ok := decodeCallbackData(button.CallbackData, settingsCallbackNamespace); ok && decoded.Fields["a"] == settingsActPick {
			labels = append(labels, button.Text)
		}
	}
	return labels
}

func TestSettingsHubList(t *testing.T) {
	t.Run("labels", func(t *testing.T) {
		long := strings.Repeat("ж", 40)
		for _, tc := range []struct {
			name, title string
			chatID      int64
			want        string
		}{
			{"a long title is cut at 32 code points with an ellipsis", long, -1001, strings.Repeat("ж", 32) + "…"},
			{"exactly 32 code points is not cut", strings.Repeat("ж", 32), -1001, strings.Repeat("ж", 32)},
			{"line breaks and runs of spaces collapse", "A\n\n  B", -1001, "A B"},
			{"the ends are trimmed", "  \t Night Owls \n", -1001, "Night Owls"},
			{"an empty title shows the chat ID", "", -1001234567890, "-1001234567890"},
			{"a title of only whitespace shows the chat ID", " \n\t ", -1002, "-1002"},
			{"a title is plain text and is never escaped", "Tom & Jerry <3", -1001, "Tom & Jerry <3"},
			{"an emoji is one code point", strings.Repeat("😀", 33), -1001, strings.Repeat("😀", 32) + "…"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				got := settingsHubLabel(tc.title, tc.chatID)
				if got != tc.want {
					t.Errorf("settingsHubLabel(%q) = %q, want %q", tc.title, got, tc.want)
				}
				if n := utf8.RuneCountInString(got); n > settingsHubLabelMax+1 {
					t.Errorf("label %q is %d code points, want at most %d", got, n, settingsHubLabelMax+1)
				}
			})
		}
	})

	t.Run("order is the lower-cased title in code point order, ties by chat ID", func(t *testing.T) {
		groups := []adminindex.ListedGroup{
			{ChatID: -100500, Title: "beta"},
			{ChatID: -100200, Title: "Alpha"},
			{ChatID: -100100, Title: "Ёлка"},
			{ChatID: -100300, Title: "alpha"},
		}
		want := []int64{-100300, -100200, -100500, -100100}
		ids := func(list []adminindex.ListedGroup) []int64 {
			var out []int64
			for _, g := range list {
				out = append(out, g.ChatID)
			}
			return out
		}
		first := settingsHubSort(groups)
		if got := ids(first); !slices.Equal(got, want) {
			t.Fatalf("order = %v, want %v", got, want)
		}
		reversed := slices.Clone(groups)
		slices.Reverse(reversed)
		if got := ids(settingsHubSort(reversed)); !slices.Equal(got, want) {
			t.Errorf("order of the reversed input = %v, want the same %v", got, want)
		}
		if got := ids(groups); got[0] != -100500 {
			t.Errorf("settingsHubSort reordered its input: %v", got)
		}
	})

	t.Run("a row carries the plain title, the message carries no title", func(t *testing.T) {
		withRealLocales(t)
		tr := i18n.MustNewTranslator("en")
		screen := renderSettingsHubList(tr, []adminindex.ListedGroup{{ChatID: -1001, Title: "Tom & Jerry"}}, 1)
		labels := settingsHubRowLabels(screen.Keyboard.InlineKeyboard)
		if len(labels) != 1 || labels[0] != "Tom & Jerry" {
			t.Errorf("row labels = %q, want the title unescaped", labels)
		}
		if strings.Contains(screen.Text, "Tom") || strings.Contains(screen.Text, "&amp;") {
			t.Errorf("the list message %q names a group", screen.Text)
		}
	})

	t.Run("one group is still a one button list", func(t *testing.T) {
		env := newSettingsEnv(t)
		owner := env.newOwner(env.chat)
		settingsHubSeedGroup(t, env.chat, "Only One", settingsHubCreator(owner))
		env.sendPrivate(owner, "/settings")
		msg := settingsPrivateOnlySent(t, env, owner)
		keyboard := staffKeyboardOf(msg.Params["reply_markup"])
		if got := settingsHubRowLabels(keyboard); len(got) != 1 || got[0] != "Only One" {
			t.Errorf("rows = %q, want one row Only One", got)
		}
		if strings.Contains(fmt.Sprint(msg.Params["text"]), staffMarker("staff_panel_page")) {
			t.Errorf("a one page list shows a Page line: %q", msg.Params["text"])
		}
	})
}

func TestSettingsHubPaging(t *testing.T) {
	withRealLocales(t)
	tr := i18n.MustNewTranslator("en")
	pagingOf := func(screen settingsScreen) (prev, next string) {
		for _, button := range settingsAllButtons(screen.Keyboard.InlineKeyboard) {
			decoded, ok := decodeCallbackData(button.CallbackData, settingsHubNamespace)
			if !ok || decoded.Fields["a"] != settingsHubActPage {
				continue
			}
			if strings.Contains(button.Text, "Prev") {
				prev = decoded.Fields["v"]
			} else {
				next = decoded.Fields["v"]
			}
		}
		return prev, next
	}

	for _, tc := range []struct {
		name       string
		n, page    int
		rows       int
		pageLine   bool
		prev, next string
	}{
		{"one group", 1, 1, 1, false, "", ""},
		{"eight groups are one page", 8, 1, 8, false, "", ""},
		{"nine groups: page 1 has Next only", 9, 1, 8, true, "", "2"},
		{"nine groups: page 2 has Prev only", 9, 2, 1, true, "1", ""},
		{"seventeen groups: a middle page has both", 17, 2, 8, true, "1", "3"},
		{"a page past the last is clamped to it", 17, 99, 1, true, "2", ""},
		{"a page below one is clamped to it", 17, 0, 8, true, "", "2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			screen := renderSettingsHubList(tr, settingsHubSort(settingsHubGroupsN(tc.n)), tc.page)
			if got := len(settingsHubRowLabels(screen.Keyboard.InlineKeyboard)); got != tc.rows {
				t.Errorf("rows = %d, want %d", got, tc.rows)
			}
			if got := strings.Contains(screen.Text, "Page "); got != tc.pageLine {
				t.Errorf("Page line present = %v, want %v in %q", got, tc.pageLine, screen.Text)
			}
			if prev, next := pagingOf(screen); prev != tc.prev || next != tc.next {
				t.Errorf("paging = prev %q next %q, want prev %q next %q", prev, next, tc.prev, tc.next)
			}
			rows := screen.Keyboard.InlineKeyboard
			last := rows[len(rows)-1]
			if len(last) != 1 || !strings.Contains(last[0].Text, "Close") {
				t.Errorf("the last row = %+v, want Close alone", last)
			}
		})
	}

	t.Run("the first row of page 2 is the ninth group in order", func(t *testing.T) {
		groups := settingsHubSort(settingsHubGroupsN(9))
		screen := renderSettingsHubList(tr, groups, 2)
		if got := settingsHubRowLabels(screen.Keyboard.InlineKeyboard); len(got) != 1 || got[0] != "Group 09" {
			t.Errorf("page 2 rows = %q, want Group 09", got)
		}
	})
}

func TestSettingsHubEmptyOnlyClose(t *testing.T) {
	withRealLocales(t)
	tr := i18n.MustNewTranslator("en")
	screen := renderSettingsHubList(tr, nil, 5)
	buttons := settingsAllButtons(screen.Keyboard.InlineKeyboard)
	if len(buttons) != 1 {
		t.Fatalf("the empty list has %d buttons, want Close only", len(buttons))
	}
}

func TestSettingsEmptyListSameForAll(t *testing.T) {
	env := newSettingsEnv(t)
	creator := gotgbot.User{Id: uniqueLinkOwnerID() + 51, FirstName: "Creator"}
	gatedChat := env.chat
	settingsHubSeedGroup(t, gatedChat, "Gated Group", settingsHubCreator(creator),
		adminindex.Entry{UserID: env.admin.Id, Role: models.AdminIndexRoleAdministrator})
	env.setAdminsAllowed(gatedChat.Id, false)

	stranger := gotgbot.User{Id: uniqueLinkOwnerID() + 52, FirstName: "Stranger"}
	ownerNoGroup := gotgbot.User{Id: uniqueLinkOwnerID() + 53, FirstName: "Owner"}

	type reply struct{ text, markup string }
	got := map[string]reply{}
	for name, user := range map[string]gotgbot.User{
		"stranger": stranger, "gated admin": env.admin, "owner with no recorded group": ownerNoGroup,
	} {
		env.sendPrivate(user, "/settings")
		msg := settingsPrivateOnlySent(t, env, user)
		got[name] = reply{fmt.Sprint(msg.Params["text"]), fmt.Sprint(msg.Params["reply_markup"])}
	}
	want := got["stranger"]
	if !strings.Contains(want.text, staffMarker("settings_list_empty")) {
		t.Fatalf("the stranger's list %q is not the empty text", want.text)
	}
	for name, r := range got {
		if r != want {
			t.Errorf("%s got %+v, the stranger got %+v", name, r, want)
		}
	}
}

func TestSettingsListHidesGated(t *testing.T) {
	listed := func(t *testing.T, env *settingsEnv, user gotgbot.User) []string {
		t.Helper()
		env.sendPrivate(user, "/settings")
		sent := env.fake.sentTo(user.Id)
		if len(sent) == 0 {
			t.Fatal("no list was sent")
		}
		return settingsHubPickedChats(t, staffKeyboardOf(sent[len(sent)-1].Params["reply_markup"]))
	}
	creator := gotgbot.User{Id: uniqueLinkOwnerID() + 61, FirstName: "Creator"}

	t.Run("a gated administrator's group is hidden", func(t *testing.T) {
		env := newSettingsEnv(t)
		settingsHubSeedGroup(t, env.chat, "G", settingsHubCreator(creator),
			adminindex.Entry{UserID: env.admin.Id, Role: models.AdminIndexRoleAdministrator})
		env.setAdminsAllowed(env.chat.Id, false)
		if got := listed(t, env, env.admin); len(got) != 0 {
			t.Errorf("the gated administrator's list = %v, want empty", got)
		}
	})

	t.Run("the group is listed when the switch is on", func(t *testing.T) {
		env := newSettingsEnv(t)
		settingsHubSeedGroup(t, env.chat, "G", settingsHubCreator(creator),
			adminindex.Entry{UserID: env.admin.Id, Role: models.AdminIndexRoleAdministrator})
		env.setAdminsAllowed(env.chat.Id, true)
		if got := listed(t, env, env.admin); !slices.Equal(got, []string{strconv.FormatInt(env.chat.Id, 10)}) {
			t.Errorf("the list = %v, want the group", got)
		}
	})

	t.Run("the group is listed when it has no recorded owner", func(t *testing.T) {
		env := newSettingsEnv(t)
		settingsHubSeedGroup(t, env.chat, "G", adminindex.Entry{UserID: env.admin.Id, Role: models.AdminIndexRoleAdministrator})
		env.setAdminsAllowed(env.chat.Id, false)
		if got := listed(t, env, env.admin); !slices.Equal(got, []string{strconv.FormatInt(env.chat.Id, 10)}) {
			t.Errorf("the list = %v, want the group", got)
		}
	})

	t.Run("the owner of X who is a gated administrator of Y sees X only", func(t *testing.T) {
		env := newSettingsEnv(t)
		other := env.addChat()
		user := env.admin
		settingsHubSeedGroup(t, env.chat, "X", settingsHubCreator(user))
		settingsHubSeedGroup(t, other, "Y", settingsHubCreator(creator),
			adminindex.Entry{UserID: user.Id, Role: models.AdminIndexRoleAdministrator})
		env.setAdminsAllowed(other.Id, false)
		if got := listed(t, env, user); !slices.Equal(got, []string{strconv.FormatInt(env.chat.Id, 10)}) {
			t.Errorf("the list = %v, want X only", got)
		}
	})
}

func TestSettingsHubCallbacks(t *testing.T) {
	const listMsg = 6101
	answersOf := func(env *settingsEnv, before int) int { return env.answerCount() - before }

	t.Run("a shub press on a message in a group is an invalid request", func(t *testing.T) {
		env := newSettingsEnv(t)
		before := env.answerCount()
		env.pressRaw(env.chat, env.admin, listMsg, encodeCallbackData(settingsHubNamespace, map[string]string{"a": settingsHubActClose}))
		if got := answersOf(env, before); got != 1 {
			t.Fatalf("answered %d times, want once", got)
		}
		if text, alert := env.lastAnswer(); !alert || !strings.Contains(text, staffMarker("common_callback_invalid_request")) {
			t.Errorf("answer = %q (alert %v), want common_callback_invalid_request", text, alert)
		}
		if got := len(callsToChat(env.fake.staffBotClient, "deleteMessage", env.chat.Id)); got != 0 {
			t.Errorf("the press deleted %d messages in the group, want none", got)
		}
	})

	t.Run("a Telegram service identity is refused before any read", func(t *testing.T) {
		env := newSettingsEnv(t)
		reads := 0
		withSettingsHubGroups(t, func(int64) ([]adminindex.ListedGroup, error) { reads++; return nil, nil })
		service := gotgbot.User{Id: 777000, FirstName: "Telegram"}
		before := env.answerCount()
		settingsHubPress(env, service, listMsg, map[string]string{"a": settingsHubActPage, "v": "1"})
		if got := answersOf(env, before); got != 1 {
			t.Fatalf("answered %d times, want once", got)
		}
		if text, alert := env.lastAnswer(); !alert || !strings.Contains(text, staffMarker("lockdown_alert_post_as_yourself")) {
			t.Errorf("answer = %q (alert %v), want lockdown_alert_post_as_yourself", text, alert)
		}
		if reads != 0 || len(env.edits(service.Id, listMsg)) != 0 {
			t.Errorf("a service identity read the list %d times and edited it %d times, want neither", reads, len(env.edits(service.Id, listMsg)))
		}
	})

	t.Run("a press whose message is gone is answered once", func(t *testing.T) {
		env := newSettingsEnv(t)
		before := env.answerCount()
		env.pressNoMessage(env.admin, encodeCallbackData(settingsHubNamespace, map[string]string{"a": settingsHubActClose}))
		if got := answersOf(env, before); got != 1 {
			t.Fatalf("answered %d times, want once", got)
		}
		if text, _ := env.lastAnswer(); !strings.Contains(text, staffMarker("common_callback_message_unavailable")) {
			t.Errorf("answer = %q, want common_callback_message_unavailable", text)
		}
	})

	t.Run("a page value that is not a whole number of 1 to 3 digits, or is 0, is invalid", func(t *testing.T) {
		for _, fields := range []map[string]string{
			{"a": settingsHubActPage, "v": "0"},
			{"a": settingsHubActPage, "v": "000"},
			{"a": settingsHubActPage, "v": "abcd"},
			{"a": settingsHubActPage, "v": "-1"},
			{"a": settingsHubActPage, "v": "1000"},
			{"a": settingsHubActPage, "v": ""},
			{"a": settingsHubActPage},
			{"a": settingsHubActPage, "v": "1", "x": "1"},
			{"a": settingsHubActClose, "v": "1"},
			{"a": "zz"},
		} {
			t.Run(fmt.Sprint(fields), func(t *testing.T) {
				env := newSettingsEnv(t)
				withSettingsHubGroups(t, func(int64) ([]adminindex.ListedGroup, error) { return settingsHubGroupsN(17), nil })
				before := env.answerCount()
				settingsHubPress(env, env.admin, listMsg, fields)
				if got := answersOf(env, before); got != 1 {
					t.Fatalf("answered %d times, want once", got)
				}
				if text, alert := env.lastAnswer(); !alert || !strings.Contains(text, staffMarker("common_callback_invalid_request")) {
					t.Errorf("answer = %q (alert %v), want common_callback_invalid_request", text, alert)
				}
				if len(env.edits(env.admin.Id, listMsg)) != 0 || len(callsToChat(env.fake.staffBotClient, "deleteMessage", env.admin.Id)) != 0 {
					t.Errorf("an invalid press changed the list message")
				}
			})
		}
	})

	t.Run("a page past the last shows the last page", func(t *testing.T) {
		env := newSettingsEnv(t)
		withSettingsHubGroups(t, func(int64) ([]adminindex.ListedGroup, error) { return settingsHubGroupsN(17), nil })
		before := env.answerCount()
		settingsHubPress(env, env.admin, listMsg, map[string]string{"a": settingsHubActPage, "v": "99"})
		if got := answersOf(env, before); got != 1 {
			t.Fatalf("answered %d times, want once", got)
		}
		text, keyboard := env.lastEdit(env.admin.Id, listMsg)
		if !strings.Contains(text, staffMarker("staff_panel_page")) {
			t.Errorf("the last page %q has no Page line", text)
		}
		if labels := settingsHubRowLabels(keyboard); len(labels) != 1 || labels[0] != "Group 17" {
			t.Errorf("the last page rows = %q, want Group 17", labels)
		}
	})

	t.Run("a replayed or forwarded list button shows the presser their own groups", func(t *testing.T) {
		env := newSettingsEnv(t)
		a := gotgbot.User{Id: uniqueLinkOwnerID() + 71, FirstName: "A"}
		b := gotgbot.User{Id: uniqueLinkOwnerID() + 72, FirstName: "B"}
		withSettingsHubGroups(t, func(userID int64) ([]adminindex.ListedGroup, error) {
			if userID == a.Id {
				return settingsHubGroupsN(17), nil
			}
			return []adminindex.ListedGroup{{ChatID: -1009990001, Title: "B One"}, {ChatID: -1009990002, Title: "B Two"}}, nil
		})
		env.sendPrivate(a, "/settings")
		list := settingsPrivateOnlySent(t, env, a)
		next, ok := settingsButtonByMarker(staffKeyboardOf(list.Params["reply_markup"]), "staff_panel_next")
		if !ok {
			t.Fatal("A's first page has no Next button")
		}

		// B presses A's Next button on a message in B's own private chat.
		settingsHubPressData(env, b, listMsg, next.CallbackData)
		_, keyboard := env.lastEdit(b.Id, listMsg)
		if got := settingsHubPickedChats(t, keyboard); !slices.Equal(got, []string{"-1009990001", "-1009990002"}) {
			t.Errorf("B saw %v, want B's own two groups", got)
		}
	})

	t.Run("a failed read answers an alert and edits nothing", func(t *testing.T) {
		env := newSettingsEnv(t)
		withSettingsHubGroups(t, func(int64) ([]adminindex.ListedGroup, error) { return nil, errors.New("database down") })
		before := env.answerCount()
		settingsHubPress(env, env.admin, listMsg, map[string]string{"a": settingsHubActPage, "v": "2"})
		if got := answersOf(env, before); got != 1 {
			t.Fatalf("answered %d times, want once", got)
		}
		if text, alert := env.lastAnswer(); !alert || !strings.Contains(text, staffMarker("settings_check_failed")) {
			t.Errorf("answer = %q (alert %v), want the settings_check_failed alert", text, alert)
		}
		if len(env.edits(env.admin.Id, listMsg)) != 0 {
			t.Errorf("a failed read edited the list")
		}
	})

	t.Run("Close deletes the message", func(t *testing.T) {
		env := newSettingsEnv(t)
		before := env.answerCount()
		settingsHubPress(env, env.admin, listMsg, map[string]string{"a": settingsHubActClose})
		if got := answersOf(env, before); got != 1 {
			t.Fatalf("answered %d times, want once", got)
		}
		deletes := callsToChat(env.fake.staffBotClient, "deleteMessage", env.admin.Id)
		if len(deletes) != 1 || staffParamInt(deletes[0].Params, "message_id") != listMsg {
			t.Errorf("Close made deleteMessage calls %+v, want one for message %d", deletes, listMsg)
		}
	})

	t.Run("Close edits the message when the delete fails", func(t *testing.T) {
		env := newSettingsEnv(t)
		env.fake.script("deleteMessage", env.admin.Id, staffFakeError(400, "Bad Request: message can't be deleted"))
		before := env.answerCount()
		settingsHubPress(env, env.admin, listMsg, map[string]string{"a": settingsHubActClose})
		if got := answersOf(env, before); got != 1 {
			t.Fatalf("answered %d times, want once", got)
		}
		text, keyboard := env.lastEdit(env.admin.Id, listMsg)
		if !strings.Contains(text, staffMarker("settings_closed")) || len(keyboard) != 0 {
			t.Errorf("edit = %q with keyboard %+v, want settings_closed and no keyboard", text, keyboard)
		}
	})
}

func TestSettingsPickSameAnswer(t *testing.T) {
	const listMsg = 6201
	type outcome struct {
		text  string
		alert bool
	}
	pick := func(t *testing.T, script func(env *settingsEnv)) outcome {
		t.Helper()
		resetSettingsOwnerMemo(t)
		env := newSettingsEnv(t)
		script(env)
		before := env.answerCount()
		env.pressPrivate(env.admin, listMsg, env.privateFields(env.chat.Id, "a", settingsActPick))
		if got := env.answerCount() - before; got != 1 {
			t.Fatalf("the pick answered %d times, want once", got)
		}
		if got := len(env.edits(env.admin.Id, listMsg)); got != 0 {
			t.Errorf("a refused pick edited the list %d times, want none", got)
		}
		if got := len(callsToChat(env.fake.staffBotClient, "deleteMessage", env.admin.Id)); got != 0 {
			t.Errorf("a refused pick deleted the list")
		}
		text, alert := env.lastAnswer()
		return outcome{text, alert}
	}

	baseline := pick(t, func(env *settingsEnv) {
		env.fake.setMember(env.chat.Id, env.admin.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
	})
	if !baseline.alert || !strings.Contains(baseline.text, staffMarker("settings_pick_unavailable")) {
		t.Fatalf("a plain member's pick answered %+v, want the settings_pick_unavailable alert", baseline)
	}

	t.Run("every outcome that would tell a forger about the chat is the plain member's answer", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			script func(env *settingsEnv)
		}{
			{"the bot is not in the chat", func(env *settingsEnv) {
				env.fake.script("getChatMember", env.chat.Id, staffFakeError(403, "Forbidden: bot is not a member of the supergroup chat"))
			}},
			{"the bot was kicked", func(env *settingsEnv) {
				env.fake.script("getChatMember", env.chat.Id, staffFakeError(403, "Forbidden: bot was kicked from the supergroup chat"))
			}},
			{"the chat is gone at getChatMember", func(env *settingsEnv) {
				env.fake.script("getChatMember", env.chat.Id, staffFakeError(400, "Bad Request: chat not found"))
			}},
			{"an unrecognised 400 wording at getChatMember", func(env *settingsEnv) {
				env.fake.script("getChatMember", env.chat.Id, staffFakeError(400, "Bad Request: some wording nobody listed"))
			}},
			{"an unrecognised 403 wording at getChatMember", func(env *settingsEnv) {
				env.fake.script("getChatMember", env.chat.Id, staffFakeError(403, "Forbidden: some wording nobody listed"))
			}},
			{"a 400 at getChat", func(env *settingsEnv) {
				env.fake.script("getChat", env.chat.Id, staffFakeError(400, "Bad Request: chat not found"))
			}},
			{"an unrecognised 403 at getChat", func(env *settingsEnv) {
				env.fake.script("getChat", env.chat.Id, staffFakeError(403, "Forbidden: some wording nobody listed"))
			}},
			{"a channel at getChat", func(env *settingsEnv) { env.fake.setChatType(env.chat.Id, "channel") }},
			{"a private chat at getChat", func(env *settingsEnv) { env.fake.setChatType(env.chat.Id, "private") }},
			{"a kicked user", func(env *settingsEnv) {
				env.fake.setMember(env.chat.Id, env.admin.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusKicked})
			}},
			{"a user who left", func(env *settingsEnv) {
				env.fake.setMember(env.chat.Id, env.admin.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusLeft})
			}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				if got := pick(t, tc.script); got != baseline {
					t.Errorf("answered %+v, the plain member got %+v", got, baseline)
				}
			})
		}
	})

	t.Run("a failure that does not depend on the chat is settings_check_failed", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			script func(env *settingsEnv)
		}{
			{"a 429 at getChatMember", func(env *settingsEnv) { env.fake.script("getChatMember", env.chat.Id, staffFake429(5)) }},
			{"a 500 at getChatMember", func(env *settingsEnv) {
				env.fake.script("getChatMember", env.chat.Id, staffFakeError(500, "Internal Server Error"))
			}},
			{"a transport error at getChatMember", func(env *settingsEnv) {
				env.fake.script("getChatMember", env.chat.Id, errors.New("connection reset by peer"))
			}},
			{"a timeout at getChatMember", func(env *settingsEnv) {
				env.fake.script("getChatMember", env.chat.Id, context.DeadlineExceeded)
			}},
			{"a 429 at getChat", func(env *settingsEnv) { env.fake.script("getChat", env.chat.Id, staffFake429(5)) }},
			{"a 500 at getChat", func(env *settingsEnv) {
				env.fake.script("getChat", env.chat.Id, staffFakeError(500, "Internal Server Error"))
			}},
			{"a transport error at getChat", func(env *settingsEnv) {
				env.fake.script("getChat", env.chat.Id, errors.New("i/o timeout"))
			}},
			{"a timeout at getChat", func(env *settingsEnv) {
				env.fake.script("getChat", env.chat.Id, context.DeadlineExceeded)
			}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				got := pick(t, tc.script)
				if !got.alert || !strings.Contains(got.text, staffMarker("settings_check_failed")) {
					t.Errorf("answered %+v, want the settings_check_failed alert", got)
				}
			})
		}
	})

	t.Run("a proven administrator the gate refuses", func(t *testing.T) {
		got := pick(t, func(env *settingsEnv) {
			env.newOwner(env.chat)
			env.setAdminsAllowed(env.chat.Id, false)
		})
		if !got.alert || !strings.Contains(got.text, staffMarker("settings_gate_owner_only")) {
			t.Errorf("answered %+v, want the settings_gate_owner_only alert", got)
		}
	})

	t.Run("a gate that cannot decide", func(t *testing.T) {
		got := pick(t, func(env *settingsEnv) {
			env.newOwner(env.chat)
			env.setAdminsAllowed(env.chat.Id, false)
			env.fake.setFailure("getChatAdministrators", env.chat.Id, staffFakeError(500, "Internal Server Error"))
		})
		if !got.alert || !strings.Contains(got.text, staffMarker("settings_check_failed")) {
			t.Errorf("answered %+v, want the settings_check_failed alert", got)
		}
	})

	t.Run("a pick carries exactly a and c", func(t *testing.T) {
		env := newSettingsEnv(t)
		for _, fields := range []map[string]string{
			{"a": settingsActPick},
			{"a": settingsActPick, "u": fmt.Sprint(env.admin.Id)},
			{"a": settingsActPick, "c": fmt.Sprint(env.chat.Id), "p": "c"},
			{"a": settingsActPick, "c": "12345"},
		} {
			before := env.answerCount()
			env.pressPrivate(env.admin, listMsg, fields)
			if got := env.answerCount() - before; got != 1 {
				t.Fatalf("%v answered %d times, want once", fields, got)
			}
			if text, _ := env.lastAnswer(); !strings.Contains(text, staffMarker("common_callback_invalid_request")) {
				t.Errorf("%v answered %q, want common_callback_invalid_request", fields, text)
			}
		}
		if got := len(env.edits(env.admin.Id, listMsg)); got != 0 {
			t.Errorf("an invalid pick edited the list %d times", got)
		}
	})
}

func TestSettingsHubCallbackBytes(t *testing.T) {
	withRealLocales(t)
	tr := i18n.MustNewTranslator("en")
	const worstChat = int64(-1001234567890)
	groups := make([]adminindex.ListedGroup, settingsHubMaxPages*settingsHubPageSize)
	for i := range groups {
		groups[i] = adminindex.ListedGroup{ChatID: worstChat, Title: "G"}
	}
	for _, tc := range []struct {
		name string
		page int
	}{{"first page", 1}, {"a middle page", settingsHubMaxPages - 1}, {"the last page", settingsHubMaxPages}} {
		t.Run(tc.name, func(t *testing.T) {
			screen := renderSettingsHubList(tr, groups, tc.page)
			if screen.Omitted != 0 {
				t.Fatalf("omitted %d buttons", screen.Omitted)
			}
			sizes := map[string]int{}
			for _, button := range settingsAllButtons(screen.Keyboard.InlineKeyboard) {
				if n := len(button.CallbackData); n < 1 || n > 64 {
					t.Errorf("button %q has %d bytes of callback data, want 1..64", button.Text, n)
				}
				if decoded, ok := decodeCallbackData(button.CallbackData, settingsHubNamespace); ok {
					sizes[decoded.Fields["a"]] = len(button.CallbackData)
					if strings.Contains(button.CallbackData, "G") {
						t.Errorf("callback data %q carries a title", button.CallbackData)
					}
					continue
				}
				sizes["pick"] = len(button.CallbackData)
			}
			if sizes["pick"] != 33 {
				t.Errorf("a pick is %d bytes, want 33", sizes["pick"])
			}
			if sizes[settingsHubActClose] != 12 {
				t.Errorf("Close is %d bytes, want 12", sizes[settingsHubActClose])
			}
			if tc.page > 1 && sizes[settingsHubActPage] != 18 {
				t.Errorf("a paging button is %d bytes, want 18", sizes[settingsHubActPage])
			}
		})
	}
}

func TestSettingsHubPageFitsEveryLocale(t *testing.T) {
	withRealLocales(t)
	titles := []string{
		strings.Repeat("Ж", 32),
		strings.Repeat("क", 32),
		strings.Repeat("😀", 32),
		strings.Repeat("A&<", 11),
	}
	var groups []adminindex.ListedGroup
	for i := 0; i < 17; i++ {
		groups = append(groups, adminindex.ListedGroup{ChatID: -1001234567890 + int64(i), Title: titles[i%len(titles)]})
	}
	for _, code := range []string{"uk", "en", "es", "fr", "hi", "id", "pt", "ru"} {
		t.Run(code, func(t *testing.T) {
			tr := i18n.MustNewTranslator(code)
			for name, screen := range map[string]settingsScreen{
				"page 1":  renderSettingsHubList(tr, groups, 1),
				"page 2":  renderSettingsHubList(tr, groups, 2),
				"page 3":  renderSettingsHubList(tr, groups, 3),
				"empty":   renderSettingsHubList(tr, nil, 1),
				"1 group": renderSettingsHubList(tr, groups[:1], 1),
			} {
				if screen.Omitted != 0 {
					t.Errorf("%s %s omitted %d buttons", code, name, screen.Omitted)
				}
				if got := panelUTF16Len(screen.Text); got > 3800 || got == 0 {
					t.Errorf("%s %s is %d UTF-16 units, want 1..3800", code, name, got)
				}
				for _, button := range settingsAllButtons(screen.Keyboard.InlineKeyboard) {
					if strings.TrimSpace(button.Text) == "" {
						t.Errorf("%s %s has a button with an empty label (data %q)", code, name, button.CallbackData)
					}
					if n := utf8.RuneCountInString(button.Text); n > settingsHubLabelMax+1 {
						t.Errorf("%s %s label %q is %d code points, want at most %d", code, name, button.Text, n, settingsHubLabelMax+1)
					}
				}
			}
		})
	}
}
