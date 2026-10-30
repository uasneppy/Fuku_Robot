//go:build testtools

package modules

import (
	"errors"
	"fmt"
	"maps"
	"strconv"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"gorm.io/gorm"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/db/staff"
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
