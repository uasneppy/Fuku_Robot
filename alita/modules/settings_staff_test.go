//go:build testtools

package modules

import (
	"errors"
	"fmt"
	"maps"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"gorm.io/gorm"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/db/staff"
	"github.com/divkix/Alita_Robot/alita/i18n"
	"github.com/divkix/Alita_Robot/alita/utils/chat_status"
)

// settingsStaffTestPrivateMsg is the ID of the message a private Staff Group page sits on.
const settingsStaffTestPrivateMsg int64 = 4242

// settingsStaffTestCalls is a snapshot of every Bot API call the fake has recorded, in order.
func settingsStaffTestCalls(env *settingsEnv) []moduleBotCall {
	env.fake.mu.Lock()
	defer env.fake.mu.Unlock()
	return append([]moduleBotCall(nil), env.fake.calls...)
}

// settingsStaffTestFailRead makes every read of table fail with a database error until the
// test ends, through a real GORM callback on the test database.
func settingsStaffTestFailRead(t *testing.T, table string) {
	t.Helper()
	name := "test:fail_read_" + table + "_" + t.Name()
	err := db.DB.Callback().Query().Before("gorm:query").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == table {
			_ = tx.AddError(errors.New("database is down"))
		}
	})
	if err != nil {
		t.Fatalf("register callback: %v", err)
	}
	t.Cleanup(func() {
		if err := db.DB.Callback().Query().Remove(name); err != nil {
			t.Errorf("remove callback: %v", err)
		}
	})
}

// settingsStaffTestShape describes a keyboard by the action code of each button, one row per
// semicolon: "op;ua;bk,cl".
func settingsStaffTestShape(t *testing.T, keyboard [][]gotgbot.InlineKeyboardButton) string {
	t.Helper()
	rows := make([]string, 0, len(keyboard))
	for _, row := range keyboard {
		actions := make([]string, 0, len(row))
		for _, button := range row {
			actions = append(actions, settingsButtonFields(t, button)["a"])
		}
		rows = append(rows, strings.Join(actions, ","))
	}
	return strings.Join(rows, ";")
}

// settingsStaffTestWantFields fails unless the button carries exactly the wanted fields.
func settingsStaffTestWantFields(t *testing.T, button gotgbot.InlineKeyboardButton, want map[string]string) {
	t.Helper()
	got := settingsButtonFields(t, button)
	if !maps.Equal(got, want) {
		t.Errorf("button %q carries %v, want %v", button.Text, got, want)
	}
}

// settingsStaffTestLinked makes a Staff Group titled "Staff HQ" owned by the env's
// administrator (the live creator of it), links the env's chat to it and returns it.
func settingsStaffTestLinked(env *settingsEnv) gotgbot.Chat {
	env.t.Helper()
	staffChat := linkLockdownStaffGroup(env.lockdownEnv)
	env.fake.setCreator(staffChat.Id, env.admin.Id)
	return staffChat
}

// settingsStaffTestOpenGroup opens the menu in the group and presses the Staff Group page,
// returning the menu message ID.
func settingsStaffTestOpenGroup(env *settingsEnv) int64 {
	env.t.Helper()
	msgID := env.openMenu(env.admin)
	env.press(env.admin, msgID, env.groupFields(env.admin, "a", "op", "p", "s"))
	return msgID
}

// settingsStaffTestOpenPrivate presses the Staff Group page of a menu opened in private.
func settingsStaffTestOpenPrivate(env *settingsEnv) {
	env.t.Helper()
	env.pressPrivate(env.admin, settingsStaffTestPrivateMsg, env.privateFields(env.chat.Id, "a", "op", "p", "s"))
}

// settingsStaffTestPrivatePage is the text and keyboard of the private Staff Group page.
func settingsStaffTestPrivatePage(env *settingsEnv) (string, [][]gotgbot.InlineKeyboardButton) {
	env.t.Helper()
	return env.lastEdit(env.admin.Id, settingsStaffTestPrivateMsg)
}

// settingsStaffTestWantContains fails unless text holds every want.
func settingsStaffTestWantContains(t *testing.T, text string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(text, want) {
			t.Errorf("text %q does not contain %q", text, want)
		}
	}
}

// settingsStaffTestWantAbsent fails when text holds any of the unwanted strings.
func settingsStaffTestWantAbsent(t *testing.T, text string, unwanted ...string) {
	t.Helper()
	for _, bad := range unwanted {
		if strings.Contains(text, bad) {
			t.Errorf("text %q must not contain %q", text, bad)
		}
	}
}

// settingsStaffTestStaffGroupWithLinks makes the env's chat itself a Staff Group with n linked
// groups titled Alpha01, Alpha02, ..., all created by the env's administrator, and returns the
// linked group IDs in link order.
func settingsStaffTestStaffGroupWithLinks(env *settingsEnv, n int) []int64 {
	env.t.Helper()
	ids := []int64{env.chat.Id}
	groups := make([]int64, 0, n)
	for i := 0; i < n; i++ {
		id := uniqueModuleChatID()
		env.fake.setCreator(id, env.admin.Id)
		groups = append(groups, id)
		ids = append(ids, id)
	}
	staffCleanup(env.t, ids...)
	if _, err := staff.CreateStaffGroup(env.chat.Id, env.admin.Id, env.chat.Title); err != nil {
		env.t.Fatalf("create Staff Group: %v", err)
	}
	for i, id := range groups {
		link := &models.StaffGroupLink{
			GroupChatID: id,
			StaffChatID: env.chat.Id,
			OwnerUserID: env.admin.Id,
			GroupTitle:  fmt.Sprintf("Alpha%02d", i+1),
		}
		if err := staff.CreateLink(link); err != nil {
			env.t.Fatalf("create link %d: %v", i, err)
		}
	}
	return groups
}

func TestSettingsStaffPageVariants(t *testing.T) {
	healthy := staffMarker("staff_panel_row_status") + " ✅ ✅ ✅"

	t.Run("a linked group in a group menu never names the Staff Group", func(t *testing.T) {
		env := newSettingsEnv(t)
		staffChat := settingsStaffTestLinked(env)
		msgID := env.openMenu(env.admin)

		// Main shows the state word.
		_, main := env.lastEdit(env.chat.Id, msgID)
		button, ok := settingsButtonByMarker(main, "settings_page_staff")
		if !ok || !containsMarker(button.Text, "settings_state_linked") {
			t.Fatalf("Main page Staff button = %q (found %v), want the linked state", button.Text, ok)
		}

		before := len(settingsStaffTestCalls(env))
		answers := env.answerCount()
		env.press(env.admin, msgID, env.groupFields(env.admin, "a", "op", "p", "s"))

		text, keyboard := env.lastEdit(env.chat.Id, msgID)
		settingsStaffTestWantContains(t, text,
			staffMarker("settings_header"), staffMarker("settings_staff_linked_hidden"), healthy,
			staffMarker("staff_panel_legend"), staffMarker("staff_panel_updated"))
		settingsStaffTestWantAbsent(t, text, "Staff HQ", strconv.FormatInt(staffChat.Id, 10),
			staffMarker("settings_staff_linked_to"))

		if got := settingsStaffTestShape(t, keyboard); got != "op;ua;bk,cl" {
			t.Fatalf("keyboard = %q, want Refresh, Unlink and the navigation row", got)
		}
		settingsStaffTestWantFields(t, keyboard[0][0], env.groupFields(env.admin, "a", "op", "p", "s"))
		settingsStaffTestWantFields(t, keyboard[1][0], env.groupFields(env.admin, "a", "ua"))
		if !containsMarker(keyboard[0][0].Text, "staff_panel_refresh_button") || !containsMarker(keyboard[1][0].Text, "settings_btn_unlink") {
			t.Errorf("button labels = %q and %q", keyboard[0][0].Text, keyboard[1][0].Text)
		}

		// The press is answered once, before the first live check of the build.
		if got := env.answerCount() - answers; got != 1 {
			t.Errorf("the press was answered %d times, want exactly once", got)
		}
		answered, firstLive := -1, -1
		for i, call := range settingsStaffTestCalls(env)[before:] {
			if call.Method == "answerCallbackQuery" && answered < 0 {
				answered = i
			}
			live := call.Method == "getChatAdministrators" ||
				(call.Method == "getChatMember" && staffParamInt(call.Params, "user_id") == staffTestBotID)
			if live && firstLive < 0 {
				firstLive = i
			}
		}
		if answered < 0 || firstLive < 0 || answered > firstLive {
			t.Errorf("answer at call %d, first live check at call %d: the press must be answered before the build", answered, firstLive)
		}
	})

	t.Run("the private menu shows the Staff Group", func(t *testing.T) {
		env := newSettingsEnv(t)
		staffChat := settingsStaffTestLinked(env)
		settingsStaffTestOpenPrivate(env)

		text, keyboard := settingsStaffTestPrivatePage(env)
		settingsStaffTestWantContains(t, text,
			staffMarker("settings_staff_linked_to"), "Staff HQ", strconv.FormatInt(staffChat.Id, 10), healthy)
		settingsStaffTestWantAbsent(t, text, staffMarker("settings_staff_linked_hidden"))
		if got := settingsStaffTestShape(t, keyboard); got != "op;ua;bk,cl" {
			t.Fatalf("keyboard = %q, want Refresh, Unlink and the navigation row", got)
		}
		settingsStaffTestWantFields(t, keyboard[1][0], env.privateFields(env.chat.Id, "a", "ua"))
	})

	t.Run("a bot that cannot restrict shows the cross and the reason", func(t *testing.T) {
		env := newSettingsEnv(t)
		settingsStaffTestLinked(env)
		env.fake.setBotRole(env.chat.Id, staffRoleAdminNoRestrict)
		msgID := settingsStaffTestOpenGroup(env)

		text, _ := env.lastEdit(env.chat.Id, msgID)
		settingsStaffTestWantContains(t, text,
			staffMarker("staff_panel_row_status")+" ✅ ❌ ✅", staffMarker("staff_panel_reason_bot_cannot_restrict"))
	})

	t.Run("a confirmed lockdown adds the lockdown line", func(t *testing.T) {
		env := newSettingsEnv(t)
		settingsStaffTestLinked(env)
		startPanelLockdown(t, env.chat.Id, true)
		msgID := settingsStaffTestOpenGroup(env)

		text, _ := env.lastEdit(env.chat.Id, msgID)
		settingsStaffTestWantContains(t, text, staffMarker("staff_panel_row_lockdown"))
		settingsStaffTestWantAbsent(t, text, "secret raid", "Locker Name")
	})

	t.Run("an owner check with no answer shows the question mark and keeps Unlink", func(t *testing.T) {
		env := newSettingsEnv(t)
		settingsStaffTestLinked(env)
		env.fake.setFailure("getChatAdministrators", env.chat.Id, errors.New("boom"))
		msgID := settingsStaffTestOpenGroup(env)

		text, keyboard := env.lastEdit(env.chat.Id, msgID)
		settingsStaffTestWantContains(t, text,
			staffMarker("staff_panel_row_status")+" ✅ ✅ ❔", staffMarker("staff_panel_reason_owner_unknown"))
		if got := settingsStaffTestShape(t, keyboard); got != "op;ua;bk,cl" {
			t.Errorf("keyboard = %q, want Unlink to stay", got)
		}
		link, err := staff.GetLinkOfGroupFresh(env.chat.Id)
		if err != nil || link == nil {
			t.Errorf("an unanswered owner check removed the link: %v, %v", link, err)
		}
	})

	t.Run("a link whose owner is no longer the creator is removed and drawn as not linked", func(t *testing.T) {
		env := newSettingsEnv(t)
		settingsStaffTestLinked(env)
		env.fake.setCreator(env.chat.Id, env.admin.Id+1)
		msgID := settingsStaffTestOpenGroup(env)

		text, keyboard := env.lastEdit(env.chat.Id, msgID)
		settingsStaffTestWantContains(t, text, staffMarker("settings_staff_not_linked"))
		settingsStaffTestWantAbsent(t, text, settingsNoticeWarnIcon, settingsNoticeOKIcon)
		if got := settingsStaffTestShape(t, keyboard); got != "lk;bk,cl" {
			t.Errorf("keyboard = %q, want Link and the navigation row", got)
		}
		if link, err := staff.GetLinkOfGroupFresh(env.chat.Id); err != nil || link != nil {
			t.Errorf("link after the owner changed = %v, %v, want none", link, err)
		}
	})

	t.Run("a group that is not linked offers Link", func(t *testing.T) {
		env := newSettingsEnv(t)
		msgID := env.openMenu(env.admin)
		_, main := env.lastEdit(env.chat.Id, msgID)
		button, ok := settingsButtonByMarker(main, "settings_page_staff")
		if !ok || !containsMarker(button.Text, "settings_state_not_linked") {
			t.Fatalf("Main page Staff button = %q (found %v), want the not linked state", button.Text, ok)
		}

		env.press(env.admin, msgID, env.groupFields(env.admin, "a", "op", "p", "s"))
		text, keyboard := env.lastEdit(env.chat.Id, msgID)
		settingsStaffTestWantContains(t, text, staffMarker("settings_staff_not_linked"), staffMarker("settings_staff_link_hint"))
		if got := settingsStaffTestShape(t, keyboard); got != "lk;bk,cl" {
			t.Fatalf("keyboard = %q, want Link and the navigation row", got)
		}
		settingsStaffTestWantFields(t, keyboard[0][0], env.groupFields(env.admin, "a", "lk"))
		if !containsMarker(keyboard[0][0].Text, "settings_btn_link") {
			t.Errorf("Link button label = %q", keyboard[0][0].Text)
		}
	})

	t.Run("a failed read of the link is the cannot-open edit", func(t *testing.T) {
		env := newSettingsEnv(t)
		msgID := env.openMenu(env.admin)
		settingsStaffTestFailRead(t, "staff_group_links")

		env.press(env.admin, msgID, env.groupFields(env.admin, "a", "op", "p", "s"))
		text, keyboard := env.lastEdit(env.chat.Id, msgID)
		settingsStaffTestWantContains(t, text, staffMarker("settings_cannot_open"))
		if len(keyboard) != 0 {
			t.Errorf("the cannot-open edit carries a keyboard: %+v", keyboard)
		}
		if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("settings_cannot_open_alert")) {
			t.Errorf("answer = %q alert %v, want the cannot-open alert", answer, alert)
		}
	})
}

func TestSettingsStaffReadOnly(t *testing.T) {
	t.Run("a group menu inside a Staff Group is the read-only panel", func(t *testing.T) {
		env := newSettingsEnv(t)
		settingsStaffTestStaffGroupWithLinks(env, 10)
		msgID := env.openMenu(env.admin)

		_, main := env.lastEdit(env.chat.Id, msgID)
		button, ok := settingsButtonByMarker(main, "settings_page_staff")
		if !ok || !containsMarker(button.Text, "settings_state_staff_n") || !strings.HasSuffix(button.Text, " 10") {
			t.Fatalf("Main page Staff button = %q (found %v), want the count of 10 linked groups", button.Text, ok)
		}

		env.press(env.admin, msgID, env.groupFields(env.admin, "a", "op", "p", "s"))
		text, keyboard := env.lastEdit(env.chat.Id, msgID)
		settingsStaffTestWantContains(t, text,
			staffMarker("staff_panel_chat_id"), staffMarker("staff_panel_links_header")+" 10", staffMarker("staff_panel_page")+" 1 2",
			staffMarker("staff_panel_legend"), staffMarker("staff_panel_updated"), staffMarker("settings_staff_readonly"),
			"Alpha01", "Alpha08")
		settingsStaffTestWantAbsent(t, text, "Alpha09", "Alpha10")
		if got := strings.Count(text, staffMarker("staff_panel_row_status")); got != 8 {
			t.Errorf("the first page shows %d rows, want 8", got)
		}
		if n := len(utf16.Encode([]rune(text))); n > staffPanelMaxUTF16 {
			t.Errorf("the page is %d UTF-16 units, want at most %d", n, staffPanelMaxUTF16)
		}

		if got := settingsStaffTestShape(t, keyboard); got != "pg;pg;bk,cl" {
			t.Fatalf("keyboard = %q, want Refresh, Next and the navigation row", got)
		}
		for _, button := range settingsAllButtons(keyboard) {
			if strings.HasPrefix(button.CallbackData, "staff|") {
				t.Errorf("button %q is a staff| button", button.Text)
			}
		}
		settingsStaffTestWantFields(t, keyboard[0][0], env.groupFields(env.admin, "a", "pg", "v", "0"))
		settingsStaffTestWantFields(t, keyboard[1][0], env.groupFields(env.admin, "a", "pg", "v", "1"))
		if !containsMarker(keyboard[1][0].Text, "staff_panel_next") {
			t.Errorf("second row label = %q, want Next", keyboard[1][0].Text)
		}

		// Next shows the last two rows and Prev.
		env.press(env.admin, msgID, env.groupFields(env.admin, "a", "pg", "v", "1"))
		text, keyboard = env.lastEdit(env.chat.Id, msgID)
		settingsStaffTestWantContains(t, text, "Alpha09", "Alpha10", staffMarker("staff_panel_page")+" 2 2")
		settingsStaffTestWantAbsent(t, text, "Alpha08")
		if got := settingsStaffTestShape(t, keyboard); got != "pg;pg;bk,cl" {
			t.Fatalf("last page keyboard = %q", got)
		}
		settingsStaffTestWantFields(t, keyboard[1][0], env.groupFields(env.admin, "a", "pg", "v", "0"))
		if !containsMarker(keyboard[1][0].Text, "staff_panel_prev") {
			t.Errorf("last page second row label = %q, want Prev", keyboard[1][0].Text)
		}

		// A page past the end is clamped to the last page.
		env.press(env.admin, msgID, env.groupFields(env.admin, "a", "pg", "v", "99"))
		text, _ = env.lastEdit(env.chat.Id, msgID)
		settingsStaffTestWantContains(t, text, "Alpha10", staffMarker("staff_panel_page")+" 2 2")

		// A page that is not a number is an invalid request, not an edit.
		edits := len(env.edits(env.chat.Id, msgID))
		env.press(env.admin, msgID, env.groupFields(env.admin, "a", "pg", "v", "x"))
		if got := len(env.edits(env.chat.Id, msgID)); got != edits {
			t.Errorf("a forged page edited the menu %d times", got-edits)
		}
		if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("common_callback_invalid_request")) {
			t.Errorf("answer = %q alert %v, want the invalid request alert", answer, alert)
		}
	})

	t.Run("the private menu of a Staff Group is the same panel", func(t *testing.T) {
		env := newSettingsEnv(t)
		settingsStaffTestStaffGroupWithLinks(env, 3)
		settingsStaffTestOpenPrivate(env)

		text, keyboard := settingsStaffTestPrivatePage(env)
		settingsStaffTestWantContains(t, text,
			staffMarker("staff_panel_chat_id"), staffMarker("staff_panel_links_header")+" 3", staffMarker("settings_staff_readonly"))
		settingsStaffTestWantAbsent(t, text, staffMarker("staff_panel_page"))
		if got := settingsStaffTestShape(t, keyboard); got != "pg;bk,cl" {
			t.Errorf("keyboard = %q, want Refresh and the navigation row only", got)
		}
	})

	t.Run("no linked groups says so and shows no legend", func(t *testing.T) {
		env := newSettingsEnv(t)
		settingsStaffTestStaffGroupWithLinks(env, 0)
		msgID := settingsStaffTestOpenGroup(env)

		text, _ := env.lastEdit(env.chat.Id, msgID)
		settingsStaffTestWantContains(t, text, staffMarker("staff_panel_no_links"), staffMarker("settings_staff_readonly"))
		settingsStaffTestWantAbsent(t, text, staffMarker("staff_panel_legend"))
	})

	t.Run("a failed read of the linked groups is the cannot-open edit", func(t *testing.T) {
		env := newSettingsEnv(t)
		settingsStaffTestStaffGroupWithLinks(env, 2)
		msgID := env.openMenu(env.admin)
		settingsStaffTestFailRead(t, "staff_group_links")

		env.press(env.admin, msgID, env.groupFields(env.admin, "a", "op", "p", "s"))
		text, keyboard := env.lastEdit(env.chat.Id, msgID)
		settingsStaffTestWantContains(t, text, staffMarker("settings_cannot_open"))
		if len(keyboard) != 0 {
			t.Errorf("the cannot-open edit carries a keyboard: %+v", keyboard)
		}
	})
}

// settingsStaffTestOwnedStaffGroup makes a Staff Group titled title that the env's
// administrator created (the recorded owner and, live, the creator) and returns its chat.
func settingsStaffTestOwnedStaffGroup(env *settingsEnv, title string) gotgbot.Chat {
	env.t.Helper()
	chat := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: title}
	staffCleanup(env.t, chat.Id, env.chat.Id)
	if _, err := staff.CreateStaffGroup(chat.Id, env.admin.Id, title); err != nil {
		env.t.Fatalf("create Staff Group %q: %v", title, err)
	}
	env.fake.setCreator(chat.Id, env.admin.Id)
	return chat
}

// settingsStaffTestMessagesWith counts the messages sent to chatID that carry the marker of
// the locale key.
func settingsStaffTestMessagesWith(env *settingsEnv, chatID int64, key string) int {
	n := 0
	for _, sent := range env.fake.sentTo(chatID) {
		if strings.Contains(fmt.Sprint(sent.Params["text"]), staffMarker(key)) {
			n++
		}
	}
	return n
}

// settingsStaffTestStaffRowIDs lists the staff_groups row IDs of the env's administrator, in
// the order the repository lists them.
func settingsStaffTestStaffRowIDs(t *testing.T, env *settingsEnv) []uint {
	t.Helper()
	groups, err := staff.ListStaffGroupsByOwner(env.admin.Id)
	if err != nil {
		t.Fatalf("list Staff Groups: %v", err)
	}
	ids := make([]uint, 0, len(groups))
	for _, group := range groups {
		ids = append(ids, group.ID)
	}
	return ids
}

// settingsStaffTestLinkOf reads the link of the env's chat, failing the test on a read error.
func settingsStaffTestLinkOf(t *testing.T, env *settingsEnv) *models.StaffGroupLink {
	t.Helper()
	link, err := staff.GetLinkOfGroupFresh(env.chat.Id)
	if err != nil {
		t.Fatalf("read the link: %v", err)
	}
	return link
}

func TestSettingsCardExpiry(t *testing.T) {
	now := time.Now()
	want := strconv.FormatInt(now.Add(settingsCardLifetime).Unix()/60, 10)
	if got := settingsCardExpiry(now); got != want {
		t.Fatalf("settingsCardExpiry = %q, want the unix minute %q", got, want)
	}

	if expired, ok := settingsCardExpired(map[string]string{"e": settingsCardExpiry(time.Now())}); !ok || expired {
		t.Errorf("a card made now: expired = %v, ok = %v, want (false, true)", expired, ok)
	}
	past := strconv.FormatInt(time.Now().Unix()/60-1, 10)
	if expired, ok := settingsCardExpired(map[string]string{"e": past}); !ok || !expired {
		t.Errorf("a card from a minute ago: expired = %v, ok = %v, want (true, true)", expired, ok)
	}
	for _, bad := range []string{"", "0", "-5", "1a", "+5", " 5", "9223372036854775808"} {
		if _, ok := settingsCardExpired(map[string]string{"e": bad}); ok {
			t.Errorf("e = %q was accepted", bad)
		}
	}
	if _, ok := settingsCardExpired(map[string]string{}); ok {
		t.Error("a missing e was accepted")
	}
}

func TestSettingsStaffLink(t *testing.T) {
	t.Run("an opener who owns no Staff Group gets the menu's own line", func(t *testing.T) {
		env := newSettingsEnv(t)
		msgID := env.openMenu(env.admin)
		env.press(env.admin, msgID, env.groupFields(env.admin, "a", "lk"))

		text, _ := env.lastEdit(env.chat.Id, msgID)
		settingsStaffTestWantContains(t, text, settingsNoticeWarnIcon+staffMarker("settings_staff_no_group"),
			staffMarker("settings_staff_not_linked"))
		if strings.Contains(text, staffMarker("staff_link_no_staff_group")) {
			t.Errorf("the menu shows the command's line, which tells the presser to send a command again: %q", text)
		}
		if link := settingsStaffTestLinkOf(t, env); link != nil {
			t.Errorf("a link exists: %+v", link)
		}
	})

	t.Run("exactly one verified Staff Group links at once", func(t *testing.T) {
		env := newSettingsEnv(t)
		staffChat := settingsStaffTestOwnedStaffGroup(env, "Staff HQ")
		msgID := env.openMenu(env.admin)
		inGroup := len(env.fake.sentTo(env.chat.Id))
		env.press(env.admin, msgID, env.groupFields(env.admin, "a", "lk"))

		link := settingsStaffTestLinkOf(t, env)
		if link == nil || link.StaffChatID != staffChat.Id || link.OwnerUserID != env.admin.Id {
			t.Fatalf("link = %+v, want one to Staff Group %d by the opener", link, staffChat.Id)
		}
		if got := settingsStaffTestMessagesWith(env, staffChat.Id, "staff_link_done"); got != 1 {
			t.Errorf("the Staff Group got %d link notices, want exactly 1", got)
		}
		if got := len(env.fake.sentTo(env.chat.Id)); got != inGroup {
			t.Errorf("%d messages reached the linked group, want none", got-inGroup)
		}
		text, _ := env.lastEdit(env.chat.Id, msgID)
		settingsStaffTestWantContains(t, text, settingsNoticeOKIcon+staffMarker("settings_notice_linked"),
			staffMarker("settings_staff_linked_hidden"))
		settingsStaffTestWantAbsent(t, text, "Staff HQ", strconv.FormatInt(staffChat.Id, 10))
	})

	t.Run("a candidate whose owner check fails links nothing", func(t *testing.T) {
		env := newSettingsEnv(t)
		staffChat := settingsStaffTestOwnedStaffGroup(env, "Staff HQ")
		env.fake.setFailure("getChatAdministrators", staffChat.Id, errors.New("boom"))
		msgID := env.openMenu(env.admin)
		env.press(env.admin, msgID, env.groupFields(env.admin, "a", "lk"))

		text, _ := env.lastEdit(env.chat.Id, msgID)
		settingsStaffTestWantContains(t, text, settingsNoticeWarnIcon+staffMarker("staff_check_failed"))
		if link := settingsStaffTestLinkOf(t, env); link != nil {
			t.Errorf("a link exists: %+v", link)
		}
		if got := settingsStaffTestMessagesWith(env, staffChat.Id, "staff_link_done"); got != 0 {
			t.Errorf("the Staff Group got %d link notices, want none", got)
		}
	})

	// /linkstaff without an argument fails closed when a candidate cannot be verified, even
	// beside one verified match, because the unverified one might be a second match. The menu
	// refuses the same way instead of showing a one-button picker (IN-04).
	t.Run("one verified match beside an unverifiable one is refused like the command", func(t *testing.T) {
		previousDelay := staffSelfDeleteAfter
		staffSelfDeleteAfter = time.Hour
		t.Cleanup(func() { staffSelfDeleteAfter = previousDelay })

		env := newSettingsEnv(t)
		good := settingsStaffTestOwnedStaffGroup(env, "Good Staff")
		unverifiable := settingsStaffTestOwnedStaffGroup(env, "Unverifiable Staff")
		env.fake.setFailure("getChatAdministrators", unverifiable.Id, errors.New("boom"))

		env.send(env.admin, "/linkstaff")
		if reply := env.lastReply(); !strings.Contains(reply, staffMarker("staff_check_failed")) {
			t.Fatalf("/linkstaff replied %q, want staff_check_failed", reply)
		}
		if link := settingsStaffTestLinkOf(t, env); link != nil {
			t.Fatalf("/linkstaff linked the group beside an unverifiable candidate: %+v", link)
		}

		msgID := env.openMenu(env.admin)
		env.press(env.admin, msgID, env.groupFields(env.admin, "a", "lk"))

		text, keyboard := env.lastEdit(env.chat.Id, msgID)
		settingsStaffTestWantContains(t, text, settingsNoticeWarnIcon+staffMarker("staff_check_failed"))
		settingsStaffTestWantAbsent(t, text, staffMarker("settings_staff_pick"), good.Title)
		if _, onPicker := settingsButtonByMarker(keyboard, "settings_btn_staff_pick_n"); onPicker {
			t.Errorf("the menu showed a picker: %+v", keyboard)
		}
		if link := settingsStaffTestLinkOf(t, env); link != nil {
			t.Errorf("the menu linked the group beside an unverifiable candidate: %+v", link)
		}
		if got := settingsStaffTestMessagesWith(env, good.Id, "staff_link_done"); got != 0 {
			t.Errorf("the Staff Group got %d link notices, want none", got)
		}
	})

	t.Run("a basic group is refused with its escaped title", func(t *testing.T) {
		env := newSettingsEnv(t)
		settingsStaffTestOwnedStaffGroup(env, "Staff HQ")
		msgID := env.openMenu(env.admin)
		basic := gotgbot.Chat{Id: env.chat.Id, Type: "group", Title: "Basic <b>Chat</b>"}
		data := encodeCallbackData(settingsCallbackNamespace, env.groupFields(env.admin, "a", "lk"))
		env.pressRaw(basic, env.admin, msgID, data)

		text, _ := env.lastEdit(env.chat.Id, msgID)
		settingsStaffTestWantContains(t, text, settingsNoticeWarnIcon+staffMarker("staff_link_refuse_not_supergroup"),
			"Basic &lt;b&gt;Chat&lt;/b&gt;")
		if link := settingsStaffTestLinkOf(t, env); link != nil {
			t.Errorf("a link exists: %+v", link)
		}
	})
}

func TestSettingsStaffPick(t *testing.T) {
	newPicker := func(t *testing.T) (*settingsEnv, []gotgbot.Chat, []uint) {
		t.Helper()
		env := newSettingsEnv(t)
		chats := []gotgbot.Chat{
			settingsStaffTestOwnedStaffGroup(env, "Alpha Staff"),
			settingsStaffTestOwnedStaffGroup(env, "Bravo Staff"),
			settingsStaffTestOwnedStaffGroup(env, "Charlie Staff"),
		}
		return env, chats, settingsStaffTestStaffRowIDs(t, env)
	}

	t.Run("a group menu offers positional buttons that carry row IDs", func(t *testing.T) {
		env, chats, ids := newPicker(t)
		if len(ids) != 3 {
			t.Fatalf("%d Staff Groups, want 3", len(ids))
		}
		msgID := env.openMenu(env.admin)
		env.press(env.admin, msgID, env.groupFields(env.admin, "a", "lk"))

		text, keyboard := env.lastEdit(env.chat.Id, msgID)
		settingsStaffTestWantContains(t, text, staffMarker("settings_staff_pick"), staffMarker("settings_staff_pick_private_hint"))
		if got := settingsStaffTestShape(t, keyboard); got != "lk;lk;lk;bk,cl" {
			t.Fatalf("keyboard = %q, want three pick buttons and the navigation row", got)
		}
		for i := range chats {
			button := keyboard[i][0]
			if !strings.Contains(button.Text, staffMarker("settings_btn_staff_pick_n")) || !strings.HasSuffix(button.Text, " "+strconv.Itoa(i+1)) {
				t.Errorf("button %d label = %q, want the positional label for %d", i, button.Text, i+1)
			}
			settingsStaffTestWantFields(t, button, env.groupFields(env.admin, "a", "lk", "s", strconv.FormatUint(uint64(ids[i]), 10)))
			settingsStaffTestWantAbsent(t, text+button.Text+button.CallbackData, chats[i].Title, strconv.FormatInt(chats[i].Id, 10))
		}
	})

	// The picker lists settingsStaffPickMax groups. A presser with more is told how many are
	// missing and that /linkstaff takes an ID, and a presser with no more is told nothing
	// (IN-04).
	t.Run("a presser with more Staff Groups than the picker lists is told how to link the rest", func(t *testing.T) {
		env := newSettingsEnv(t)
		const owned = settingsStaffPickMax + 2
		for i := 0; i < owned; i++ {
			settingsStaffTestOwnedStaffGroup(env, fmt.Sprintf("Staff %02d", i))
		}
		msgID := env.openMenu(env.admin)
		env.press(env.admin, msgID, env.groupFields(env.admin, "a", "lk"))

		text, keyboard := env.lastEdit(env.chat.Id, msgID)
		settingsStaffTestWantContains(t, text, staffMarker("settings_staff_pick_more")+" "+strconv.Itoa(owned-settingsStaffPickMax))
		if got := len(settingsAllButtons(keyboard)); got != settingsStaffPickMax+2 {
			t.Errorf("the picker has %d buttons, want %d picks and Back and Close", got, settingsStaffPickMax)
		}
	})

	t.Run("a picker that lists every Staff Group has no more line", func(t *testing.T) {
		env, _, _ := newPicker(t)
		msgID := env.openMenu(env.admin)
		env.press(env.admin, msgID, env.groupFields(env.admin, "a", "lk"))

		text, _ := env.lastEdit(env.chat.Id, msgID)
		settingsStaffTestWantAbsent(t, text, staffMarker("settings_staff_pick_more"))
	})

	t.Run("a private menu labels the buttons with the titles", func(t *testing.T) {
		env, chats, _ := newPicker(t)
		env.pressPrivate(env.admin, settingsStaffTestPrivateMsg, env.privateFields(env.chat.Id, "a", "lk"))

		text, keyboard := settingsStaffTestPrivatePage(env)
		settingsStaffTestWantContains(t, text, staffMarker("settings_staff_pick"))
		settingsStaffTestWantAbsent(t, text, staffMarker("settings_staff_pick_private_hint"))
		for i, chat := range chats {
			if want := "👥 " + chat.Title; keyboard[i][0].Text != want {
				t.Errorf("button %d label = %q, want %q", i, keyboard[i][0].Text, want)
			}
		}
	})

	t.Run("the pick links the row whose ID was pressed, not a position", func(t *testing.T) {
		env, chats, ids := newPicker(t)
		msgID := env.openMenu(env.admin)
		env.press(env.admin, msgID, env.groupFields(env.admin, "a", "lk"))
		_, keyboard := env.lastEdit(env.chat.Id, msgID)
		second := keyboard[1][0]

		// Re-create the first Staff Group: its row now sorts last, so position 2 is another group.
		if err := db.DB.Where("chat_id = ?", chats[0].Id).Delete(&models.StaffGroup{}).Error; err != nil {
			t.Fatalf("delete the first Staff Group: %v", err)
		}
		if _, err := staff.CreateStaffGroup(chats[0].Id, env.admin.Id, chats[0].Title); err != nil {
			t.Fatalf("re-create the first Staff Group: %v", err)
		}
		if now := settingsStaffTestStaffRowIDs(t, env); now[1] == ids[1] {
			t.Fatalf("the rows did not move: %v", now)
		}

		env.pressRaw(env.chat, env.admin, msgID, second.CallbackData)
		link := settingsStaffTestLinkOf(t, env)
		if link == nil || link.StaffChatID != chats[1].Id {
			t.Fatalf("link = %+v, want a link to the second Staff Group %d", link, chats[1].Id)
		}
		text, _ := env.lastEdit(env.chat.Id, msgID)
		settingsStaffTestWantContains(t, text, settingsNoticeOKIcon+staffMarker("settings_notice_linked"))
	})

	t.Run("a forged s links nothing", func(t *testing.T) {
		env, chats, ids := newPicker(t)
		msgID := env.openMenu(env.admin)

		stranger := gotgbot.User{Id: env.admin.Id + 1, FirstName: "Stranger"}
		otherChat := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Other Staff"}
		staffCleanup(t, otherChat.Id)
		if _, err := staff.CreateStaffGroup(otherChat.Id, stranger.Id, otherChat.Title); err != nil {
			t.Fatalf("create the stranger's Staff Group: %v", err)
		}
		env.fake.setCreator(otherChat.Id, stranger.Id)
		var otherRow models.StaffGroup
		if err := db.DB.Where("chat_id = ?", otherChat.Id).First(&otherRow).Error; err != nil {
			t.Fatalf("read the stranger's row: %v", err)
		}

		for name, s := range map[string]string{
			"another owner's row":     strconv.FormatUint(uint64(otherRow.ID), 10),
			"a row that is not there": strconv.FormatUint(uint64(ids[2])+1_000_000, 10),
		} {
			env.press(env.admin, msgID, env.groupFields(env.admin, "a", "lk", "s", s))
			text, _ := env.lastEdit(env.chat.Id, msgID)
			settingsStaffTestWantContains(t, text, settingsNoticeWarnIcon+staffMarker("staff_link_refuse_not_staff_owner"))
			if link := settingsStaffTestLinkOf(t, env); link != nil {
				t.Fatalf("%s linked the group: %+v", name, link)
			}
		}

		// Not a number, or zero: an invalid request, not an edit.
		for _, s := range []string{"abc", "0"} {
			edits := len(env.edits(env.chat.Id, msgID))
			env.press(env.admin, msgID, env.groupFields(env.admin, "a", "lk", "s", s))
			if got := len(env.edits(env.chat.Id, msgID)); got != edits {
				t.Errorf("s = %q edited the menu", s)
			}
			if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("common_callback_invalid_request")) {
				t.Errorf("s = %q answered %q alert %v, want the invalid request alert", s, answer, alert)
			}
			if link := settingsStaffTestLinkOf(t, env); link != nil {
				t.Fatalf("s = %q linked the group: %+v", s, link)
			}
		}

		// A Staff Group the presser no longer creates, live, is refused with the same text.
		env.fake.setCreator(chats[1].Id, stranger.Id)
		env.press(env.admin, msgID, env.groupFields(env.admin, "a", "lk", "s", strconv.FormatUint(uint64(ids[1]), 10)))
		text, _ := env.lastEdit(env.chat.Id, msgID)
		settingsStaffTestWantContains(t, text, settingsNoticeWarnIcon+staffMarker("staff_link_refuse_not_staff_owner"))
		if link := settingsStaffTestLinkOf(t, env); link != nil {
			t.Fatalf("a Staff Group that is no longer the presser's was linked: %+v", link)
		}
	})
}

func TestSettingsStaffUnlink(t *testing.T) {
	// ask presses Unlink on the Staff page of a linked group and returns the menu message and
	// the card's keyboard.
	ask := func(t *testing.T, env *settingsEnv, from gotgbot.User) (int64, [][]gotgbot.InlineKeyboardButton) {
		t.Helper()
		msgID := env.openMenu(from)
		env.press(from, msgID, env.groupFields(from, "a", "op", "p", "s"))
		env.press(from, msgID, env.groupFields(from, "a", "ua"))
		_, keyboard := env.lastEdit(env.chat.Id, msgID)
		return msgID, keyboard
	}

	t.Run("the owner gets a card and No returns to the page", func(t *testing.T) {
		env := newSettingsEnv(t)
		settingsStaffTestLinked(env)
		msgID, keyboard := ask(t, env, env.admin)

		text, _ := env.lastEdit(env.chat.Id, msgID)
		settingsStaffTestWantContains(t, text, staffMarker("staff_unlink_confirm_prompt"), "Lock Chat")
		if got := settingsStaffTestShape(t, keyboard); got != "uy,no;bk,cl" {
			t.Fatalf("card keyboard = %q, want Yes and No and the navigation row", got)
		}
		yes, no := keyboard[0][0], keyboard[0][1]
		if !containsMarker(yes.Text, "settings_btn_yes") || !containsMarker(no.Text, "settings_btn_no") {
			t.Errorf("labels = %q and %q", yes.Text, no.Text)
		}
		fields := settingsButtonFields(t, yes)
		minute := time.Now().Unix() / 60
		if e, err := strconv.ParseInt(fields["e"], 10, 64); err != nil || e < minute+4 || e > minute+6 {
			t.Errorf("Yes carries e = %q, want about five minutes ahead of minute %d", fields["e"], minute)
		}
		settingsStaffTestWantFields(t, no, env.groupFields(env.admin, "a", "no", "p", "s"))
		if link := settingsStaffTestLinkOf(t, env); link == nil {
			t.Fatal("the ask removed the link")
		}

		env.pressRaw(env.chat, env.admin, msgID, no.CallbackData)
		text, _ = env.lastEdit(env.chat.Id, msgID)
		settingsStaffTestWantContains(t, text, staffMarker("settings_staff_linked_hidden"))
		if link := settingsStaffTestLinkOf(t, env); link == nil {
			t.Fatal("No removed the link")
		}
	})

	t.Run("Yes unlinks, tells the Staff Group only and redraws as not linked", func(t *testing.T) {
		env := newSettingsEnv(t)
		staffChat := settingsStaffTestLinked(env)
		msgID, keyboard := ask(t, env, env.admin)
		inGroup := len(env.fake.sentTo(env.chat.Id))

		env.pressRaw(env.chat, env.admin, msgID, keyboard[0][0].CallbackData)
		if link := settingsStaffTestLinkOf(t, env); link != nil {
			t.Fatalf("the link survived Yes: %+v", link)
		}
		if got := settingsStaffTestMessagesWith(env, staffChat.Id, "staff_notice_unlinked_by"); got != 1 {
			t.Errorf("the Staff Group got %d unlink notices, want 1", got)
		}
		if got := len(env.fake.sentTo(env.chat.Id)); got != inGroup {
			t.Errorf("%d messages reached the unlinked group, want none", got-inGroup)
		}
		text, kb := env.lastEdit(env.chat.Id, msgID)
		settingsStaffTestWantContains(t, text, settingsNoticeOKIcon+staffMarker("settings_notice_unlinked"),
			staffMarker("settings_staff_not_linked"))
		if got := settingsStaffTestShape(t, kb); got != "lk;bk,cl" {
			t.Errorf("keyboard = %q, want Link and the navigation row", got)
		}
	})

	t.Run("two Yes presses unlink once and post one notice", func(t *testing.T) {
		env := newSettingsEnv(t)
		staffChat := settingsStaffTestLinked(env)
		msgID, keyboard := ask(t, env, env.admin)

		env.pressRaw(env.chat, env.admin, msgID, keyboard[0][0].CallbackData)
		env.pressRaw(env.chat, env.admin, msgID, keyboard[0][0].CallbackData)
		if got := settingsStaffTestMessagesWith(env, staffChat.Id, "staff_notice_unlinked_by"); got != 1 {
			t.Errorf("the Staff Group got %d unlink notices, want exactly 1", got)
		}
		text, _ := env.lastEdit(env.chat.Id, msgID)
		settingsStaffTestWantContains(t, text, settingsNoticeWarnIcon+staffMarker("staff_unlink_not_linked"))
	})

	t.Run("a Yes after its expiry keeps the link", func(t *testing.T) {
		env := newSettingsEnv(t)
		settingsStaffTestLinked(env)
		previous := settingsCardLifetime
		settingsCardLifetime = -2 * time.Minute
		t.Cleanup(func() { settingsCardLifetime = previous })
		msgID, keyboard := ask(t, env, env.admin)

		env.pressRaw(env.chat, env.admin, msgID, keyboard[0][0].CallbackData)
		text, _ := env.lastEdit(env.chat.Id, msgID)
		settingsStaffTestWantContains(t, text, settingsNoticeWarnIcon+staffMarker("settings_card_expired"),
			staffMarker("settings_staff_linked_hidden"))
		if link := settingsStaffTestLinkOf(t, env); link == nil {
			t.Fatal("an expired Yes removed the link")
		}
	})

	t.Run("a Yes with a forged expiry is an invalid request", func(t *testing.T) {
		env := newSettingsEnv(t)
		settingsStaffTestLinked(env)
		msgID, _ := ask(t, env, env.admin)
		edits := len(env.edits(env.chat.Id, msgID))

		for _, e := range []string{"", "abc", "0", "-3"} {
			fields := env.groupFields(env.admin, "a", "uy")
			if e != "" {
				fields["e"] = e
			}
			env.press(env.admin, msgID, fields)
			if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("common_callback_invalid_request")) {
				t.Errorf("e = %q answered %q alert %v, want the invalid request alert", e, answer, alert)
			}
		}
		if got := len(env.edits(env.chat.Id, msgID)); got != edits {
			t.Errorf("a forged Yes edited the menu %d times", got-edits)
		}
		if link := settingsStaffTestLinkOf(t, env); link == nil {
			t.Fatal("a forged Yes removed the link")
		}
	})

	// D-33 (WR-01): the Yes of an Unlink card removes the link the card was drawn for and no
	// other.
	t.Run("Yes carries the link it was drawn for", func(t *testing.T) {
		env := newSettingsEnv(t)
		settingsStaffTestLinked(env)
		link := settingsStaffTestLinkOf(t, env)
		_, keyboard := ask(t, env, env.admin)

		fields := settingsButtonFields(t, keyboard[0][0])
		if fields["l"] != strconv.FormatUint(uint64(link.ID), 10) || len(fields) != 4 {
			t.Errorf("Yes fields = %v, want a=uy, the opener, e and l = %d", fields, link.ID)
		}
	})

	t.Run("a card of a link that was replaced does not remove the new link", func(t *testing.T) {
		env := newSettingsEnv(t)
		settingsStaffTestLinked(env)
		first := settingsStaffTestLinkOf(t, env)
		msgID, keyboard := ask(t, env, env.admin)

		// The link of the card is removed and the group is linked again, to another Staff
		// Group: a new row.
		if deleted, err := staff.DeleteLink(first.ID); err != nil || !deleted {
			t.Fatalf("delete the first link: %v, %v", deleted, err)
		}
		otherStaff := newLockdownStaffGroup(env.lockdownEnv)
		env.fake.setCreator(otherStaff.Id, env.admin.Id)
		linkLockdownGroupTo(env.lockdownEnv, otherStaff)
		second := settingsStaffTestLinkOf(t, env)
		if second == nil || second.ID == first.ID {
			t.Fatalf("setup: the second link = %+v beside the first %+v", second, first)
		}
		notices := settingsStaffTestMessagesWith(env, otherStaff.Id, "staff_notice_unlinked_by")

		env.pressRaw(env.chat, env.admin, msgID, keyboard[0][0].CallbackData)

		now := settingsStaffTestLinkOf(t, env)
		if now == nil || now.ID != second.ID {
			t.Fatalf("link after the stale Yes = %+v, want the second link %d untouched", now, second.ID)
		}
		if got := settingsStaffTestMessagesWith(env, otherStaff.Id, "staff_notice_unlinked_by"); got != notices {
			t.Errorf("a stale Yes posted %d unlink notices", got-notices)
		}
		text, _ := env.lastEdit(env.chat.Id, msgID)
		settingsStaffTestWantContains(t, text, settingsNoticeWarnIcon+staffMarker("staff_unlink_not_linked"))
	})

	t.Run("a Yes with a forged l is an invalid request", func(t *testing.T) {
		env := newSettingsEnv(t)
		settingsStaffTestLinked(env)
		link := settingsStaffTestLinkOf(t, env)
		msgID, _ := ask(t, env, env.admin)
		edits := len(env.edits(env.chat.Id, msgID))

		for _, l := range []string{"", "0", "-3", "abc", "1 2", "9223372036854775808"} {
			fields := env.groupFields(env.admin, "a", "uy", "e", settingsCardExpiry(time.Now()))
			if l != "" {
				fields["l"] = l
			}
			env.press(env.admin, msgID, fields)
			if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("common_callback_invalid_request")) {
				t.Errorf("l = %q answered %q alert %v, want the invalid request alert", l, answer, alert)
			}
		}
		if got := len(env.edits(env.chat.Id, msgID)); got != edits {
			t.Errorf("a forged Yes edited the menu %d times", got-edits)
		}
		if now := settingsStaffTestLinkOf(t, env); now == nil || now.ID != link.ID {
			t.Fatalf("a forged Yes changed the link: %+v", now)
		}
	})

	t.Run("an admin who created neither group is refused at the ask", func(t *testing.T) {
		env := newSettingsEnv(t)
		settingsStaffTestLinked(env)
		other := gotgbot.User{Id: env.admin.Id + 7, FirstName: "Other"}
		env.fake.setMember(env.chat.Id, other.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusAdministrator, CanRestrictMembers: true})

		msgID := env.openMenu(other)
		env.press(other, msgID, env.groupFields(other, "a", "op", "p", "s"))
		env.press(other, msgID, env.groupFields(other, "a", "ua"))
		text, keyboard := env.lastEdit(env.chat.Id, msgID)
		settingsStaffTestWantContains(t, text, settingsNoticeWarnIcon+staffMarker("staff_unlink_refuse_not_owner"))
		for _, button := range settingsAllButtons(keyboard) {
			if settingsButtonFields(t, button)["a"] == "uy" {
				t.Errorf("a refused ask drew a Yes button")
			}
		}
		if strings.Contains(text, staffMarker("staff_unlink_confirm_prompt")) {
			t.Errorf("a refused ask drew the card: %q", text)
		}
		if link := settingsStaffTestLinkOf(t, env); link == nil {
			t.Fatal("a refused ask removed the link")
		}
	})

	t.Run("ownership that moves between the ask and Yes is refused", func(t *testing.T) {
		env := newSettingsEnv(t)
		staffChat := settingsStaffTestLinked(env)
		msgID, keyboard := ask(t, env, env.admin)

		// The Staff Group's creator changes after the card was drawn. The refusal is the
		// command's own text and Yes itself unlinks nothing and posts no unlink notice; the
		// page redraw's owner recheck then removes the link of a maker who no longer owns the
		// Staff Group, as /staff does, and says so to the Staff Group alone.
		env.fake.setCreator(staffChat.Id, env.admin.Id+1)
		env.pressRaw(env.chat, env.admin, msgID, keyboard[0][0].CallbackData)
		text, _ := env.lastEdit(env.chat.Id, msgID)
		settingsStaffTestWantContains(t, text, settingsNoticeWarnIcon+staffMarker("staff_unlink_refuse_not_owner"))
		if got := settingsStaffTestMessagesWith(env, staffChat.Id, "staff_notice_unlinked_by"); got != 0 {
			t.Errorf("a refused Yes posted %d unlink notices, want none", got)
		}
	})

	t.Run("an owner check that cannot be answered at Yes keeps the link", func(t *testing.T) {
		env := newSettingsEnv(t)
		staffChat := settingsStaffTestLinked(env)
		msgID, keyboard := ask(t, env, env.admin)

		env.fake.setFailure("getChatAdministrators", staffChat.Id, errors.New("boom"))
		env.pressRaw(env.chat, env.admin, msgID, keyboard[0][0].CallbackData)
		text, _ := env.lastEdit(env.chat.Id, msgID)
		settingsStaffTestWantContains(t, text, settingsNoticeWarnIcon+staffMarker("staff_check_failed"))
		if link := settingsStaffTestLinkOf(t, env); link == nil {
			t.Fatal("an unanswered owner check removed the link")
		}
	})
}

func TestSettingsAuthorityParityStaff(t *testing.T) {
	// A refusal posted in the issuing group deletes itself after a delay; keep the timers out
	// of the test.
	previousDelay := staffSelfDeleteAfter
	staffSelfDeleteAfter = time.Hour
	t.Cleanup(func() { staffSelfDeleteAfter = previousDelay })

	t.Run("an administrator who owns no Staff Group", func(t *testing.T) {
		env := newSettingsEnv(t)
		env.send(env.admin, "/linkstaff")
		if reply := env.lastReply(); !strings.Contains(reply, staffMarker("staff_link_no_staff_group")) {
			t.Fatalf("/linkstaff replied %q, want staff_link_no_staff_group", reply)
		}

		msgID := env.openMenu(env.admin)
		env.press(env.admin, msgID, env.groupFields(env.admin, "a", "lk"))
		text, _ := env.lastEdit(env.chat.Id, msgID)
		// Same refusal, nothing linked; only the wording differs, because the command's ends
		// "send this command again" (IN-05).
		settingsStaffTestWantContains(t, text, staffMarker("settings_staff_no_group"))
		if link := settingsStaffTestLinkOf(t, env); link != nil {
			t.Errorf("a refused press linked the group: %+v", link)
		}
	})

	t.Run("an administrator who is not the creator of the Staff Group", func(t *testing.T) {
		env := newSettingsEnv(t)
		settingsStaffTestLinked(env)
		other := gotgbot.User{Id: env.admin.Id + 7, FirstName: "Other"}
		env.fake.setMember(env.chat.Id, other.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusAdministrator, CanRestrictMembers: true})

		env.send(other, "/unlinkstaff")
		if reply := env.lastReply(); !strings.Contains(reply, staffMarker("staff_unlink_refuse_not_owner")) {
			t.Fatalf("/unlinkstaff replied %q, want staff_unlink_refuse_not_owner", reply)
		}
		if link := settingsStaffTestLinkOf(t, env); link == nil {
			t.Fatal("/unlinkstaff by a stranger removed the link")
		}

		msgID := env.openMenu(other)
		env.press(other, msgID, env.groupFields(other, "a", "ua"))
		text, _ := env.lastEdit(env.chat.Id, msgID)
		settingsStaffTestWantContains(t, text, staffMarker("staff_unlink_refuse_not_owner"))
		if link := settingsStaffTestLinkOf(t, env); link == nil {
			t.Fatal("the menu's Unlink by a stranger removed the link")
		}
	})

	t.Run("a plain member", func(t *testing.T) {
		env := newSettingsEnv(t)
		member := gotgbot.User{Id: env.admin.Id + 9, FirstName: "Member"}
		env.fake.setMember(env.chat.Id, member.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
		memberStaff := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Member Staff"}
		staffCleanup(t, memberStaff.Id, env.chat.Id)
		if _, err := staff.CreateStaffGroup(memberStaff.Id, member.Id, memberStaff.Title); err != nil {
			t.Fatalf("create the member's Staff Group: %v", err)
		}
		env.fake.setCreator(memberStaff.Id, member.Id)

		env.send(member, "/linkstaff")
		if got := settingsStaffTestMessagesWith(env, memberStaff.Id, "staff_link_refuse_not_target_owner"); got != 1 {
			t.Fatalf("/linkstaff by a member posted %d refusals to the Staff Group, want 1", got)
		}
		if link := settingsStaffTestLinkOf(t, env); link != nil {
			t.Fatalf("/linkstaff by a member linked the group: %+v", link)
		}

		msgID := int64(5151)
		env.pressPrivate(member, msgID, env.privateFields(env.chat.Id, "a", "lk"))
		if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("settings_not_admin")) {
			t.Errorf("the member's private press answered %q alert %v, want the settings_not_admin alert", answer, alert)
		}
		if got := len(env.edits(member.Id, msgID)); got != 0 {
			t.Errorf("a refused private press edited the message %d times", got)
		}
		if link := settingsStaffTestLinkOf(t, env); link != nil {
			t.Fatalf("a refused press linked the group: %+v", link)
		}
	})
}

func TestSettingsStaffCallbackBytes(t *testing.T) {
	withRealLocales(t)
	tr := i18n.MustNewTranslator("en")

	const staffID = int64(-1001111111111)
	staffGroup := models.StaffGroup{ID: 4294967295, ChatID: staffID, OwnerUserID: 55, Title: "Fuku Staff"}
	link := models.StaffGroupLink{ID: 4294967295, GroupChatID: -1001234567890, StaffChatID: staffID, OwnerUserID: 55, GroupTitle: "Fuku Test Group"}
	row := staffLinkRow{Link: link, Health: models.StaffHealthOK, HealthKnown: true, OwnerState: chat_status.OwnerMatch}
	pickable := make([]models.StaffGroup, 10)
	for i := range pickable {
		pickable[i] = models.StaffGroup{ID: 4294967295, ChatID: staffID - int64(i), OwnerUserID: 55, Title: "Fuku Staff"}
	}
	panelRows := make([]staffLinkRow, 20)
	for i := range panelRows {
		panelRows[i] = row
	}

	for _, variant := range []struct {
		name string
		view settingsView
	}{
		{"private", settingsView{Tr: tr, Private: true, OwnerKey: "c", OwnerValue: "-1001234567890", GroupID: -1001234567890, GroupTitle: "G", OpenerName: "N"}},
		{"group", settingsView{Tr: tr, OwnerKey: "u", OwnerValue: "9999999999", GroupID: -1001234567890, GroupTitle: "G", OpenerName: "N"}},
	} {
		t.Run(variant.name, func(t *testing.T) {
			v := variant.view
			screens := map[string]settingsScreen{
				"linked":     renderSettingsStaffLinked(v, settingsStaffViewFor(v, false, &staffGroup, &row), ""),
				"not linked": renderSettingsStaffNotLinked(v, ""),
				"panel":      renderSettingsStaffPanel(v, staffGroup, panelRows, 1, time.Now(), ""),
				"picker":     renderSettingsStaffPicker(v, pickable, ""),
				"card":       renderSettingsStaffUnlinkCard(v, link),
			}
			for name, screen := range screens {
				if screen.Omitted != 0 {
					t.Errorf("%s omitted %d buttons for overflow, want none", name, screen.Omitted)
				}
				buttons := settingsAllButtons(screen.Keyboard.InlineKeyboard)
				if len(buttons) == 0 {
					t.Errorf("%s has no buttons", name)
				}
				for _, button := range buttons {
					if n := len(button.CallbackData); n < 1 || n > 64 {
						t.Errorf("%s button %q has %d bytes of callback data, want 1..64", name, button.Text, n)
					}
				}
			}
			if variant.name == "private" {
				for _, button := range settingsAllButtons(screens["picker"].Keyboard.InlineKeyboard) {
					if fields := settingsButtonFields(t, button); fields["a"] == "lk" && len(button.CallbackData) != 46 {
						t.Errorf("the private pick button is %d bytes, want 46", len(button.CallbackData))
					}
				}
			}
			// The Unlink Yes carries the link row ID l beside e (D-33); the worst case here is a
			// 10-digit ID, and every button of the card is held to 64 bytes above.
			yes := 0
			for _, button := range settingsAllButtons(screens["card"].Keyboard.InlineKeyboard) {
				if fields := settingsButtonFields(t, button); fields["a"] == "uy" {
					yes++
					if fields["l"] != "4294967295" {
						t.Errorf("the Unlink Yes carries l = %q, want the link ID", fields["l"])
					}
				}
			}
			if yes != 1 {
				t.Errorf("the Unlink card has %d Yes buttons, want 1", yes)
			}
			wide := link
			wide.ID = 99999999999999999
			if card := renderSettingsStaffUnlinkCard(v, wide); card.Omitted != 0 {
				t.Errorf("a 17-digit link ID omitted %d card buttons, want none", card.Omitted)
			}
			if got := len(settingsAllButtons(screens["picker"].Keyboard.InlineKeyboard)); got != settingsStaffPickMax+2 {
				t.Errorf("the picker has %d buttons, want %d picks and Back and Close", got, settingsStaffPickMax)
			}
		})
	}
}
