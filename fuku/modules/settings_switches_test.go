//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/db/reports"
)

// settingsSwCleanup deletes the chats' report, AI spam and greetings rows and the cached AI spam
// and greetings copies before the test and when it ends, then resets the local cache layer.
func settingsSwCleanup(t *testing.T, chatIDs ...int64) {
	t.Helper()
	clean := func() {
		db.DB.Where("chat_id IN ?", chatIDs).Delete(&models.ReportChatSettings{})
		db.DB.Where("chat_id IN ?", chatIDs).Delete(&models.AISpamSettings{})
		db.DB.Where("chat_id IN ?", chatIDs).Delete(&models.GreetingSettings{})
		for _, id := range chatIDs {
			dbcache.DeleteCache(dbcache.CacheKey("ai_spam_settings", id))
			dbcache.DeleteCache(dbcache.CacheKey("greetings", id))
		}
		dbcache.ResetLocalForTest()
	}
	clean()
	t.Cleanup(clean)
}

// newSettingsSwitchesEnv is the settings environment plus the Reports command, so a parity test
// can run /reports next to the page. The env's chat has no report, AI spam or greetings row.
func newSettingsSwitchesEnv(t *testing.T) *settingsEnv {
	t.Helper()
	env := newSettingsEditEnv(t)
	LoadReports(env.dispatcher)
	settingsSwCleanup(t, env.chat.Id)
	return env
}

// settingsSwReportsRow reads the report row of chatID straight from the database. ok is false
// when the chat has no row.
func settingsSwReportsRow(t *testing.T, chatID int64) (row models.ReportChatSettings, ok bool) {
	t.Helper()
	if err := db.DB.Where("chat_id = ?", chatID).Limit(1).Find(&row).Error; err != nil {
		t.Fatalf("read report_chat_settings of %d: %v", chatID, err)
	}
	return row, row.ID != 0
}

// settingsSwChatRows counts the chats rows of chatID.
func settingsSwChatRows(t *testing.T, chatID int64) int64 {
	t.Helper()
	var n int64
	if err := db.DB.Model(&models.Chat{}).Where("chat_id = ?", chatID).Count(&n).Error; err != nil {
		t.Fatalf("count chats of %d: %v", chatID, err)
	}
	return n
}

// settingsSwNoRows fails the test when the chat has a report, AI spam or greetings row.
func settingsSwNoRows(t *testing.T, chatID int64, when string) {
	t.Helper()
	if _, has := settingsSwReportsRow(t, chatID); has {
		t.Errorf("%s created a report row", when)
	}
	var ai int64
	if err := db.DB.Model(&models.AISpamSettings{}).Where("chat_id = ?", chatID).Count(&ai).Error; err != nil || ai != 0 {
		t.Errorf("%s created %d AI spam rows (err %v)", when, ai, err)
	}
	if _, has := settingsEditStored(t, chatID); has {
		t.Errorf("%s created a greetings row", when)
	}
}

// settingsSwPress hand-builds a press of the Switches page on message msgID of chat, as the
// env's administrator in a group menu.
func settingsSwPress(t *testing.T, env *settingsEnv, chat gotgbot.Chat, msgID int64, kv ...string) {
	t.Helper()
	fields := env.groupFields(env.admin, kv...)
	data := encodeCallbackData(settingsCallbackNamespace, fields)
	if data == "" {
		t.Fatalf("hand-built press %v did not encode", fields)
	}
	env.pressRaw(chat, env.admin, msgID, data)
}

// settingsSwToggle is the tg press of one control of the page with the absolute value v.
func settingsSwToggle(t *testing.T, env *settingsEnv, chat gotgbot.Chat, msgID int64, control, v string) {
	t.Helper()
	settingsSwPress(t, env, chat, msgID, "a", "tg", "p", "t", "t", control, "v", v)
}

// settingsSwOpenPage opens the Switches page of the env's chat in a group menu and returns the
// message ID, the page text and its keyboard.
func settingsSwOpenPage(t *testing.T, env *settingsEnv) (int64, string, [][]gotgbot.InlineKeyboardButton) {
	t.Helper()
	msgID := env.openMenu(env.admin)
	_, mainKeyboard := env.lastEdit(env.chat.Id, msgID)
	button, ok := settingsPageButton(t, mainKeyboard, settingsPageSwitches)
	if !ok {
		t.Fatalf("Main has no Switches button: %+v", mainKeyboard)
	}
	env.pressRaw(env.chat, env.admin, msgID, button.CallbackData)
	text, keyboard := env.lastEdit(env.chat.Id, msgID)
	return msgID, text, keyboard
}

func TestSettingsSwitchesOpens(t *testing.T) {
	t.Run("in a group menu", func(t *testing.T) {
		env := newSettingsSwitchesEnv(t)
		chatsBefore := settingsSwChatRows(t, env.chat.Id)
		msgID := env.openMenu(env.admin)
		mainText, mainKeyboard := env.lastEdit(env.chat.Id, msgID)

		// A chat with no rows reads Reports on, AI spam off and Auto-approve off: one switch on.
		line, found := settingsMainStateLine(mainText, "settings_page_switches")
		if !found || !strings.Contains(line, "🔘") || !strings.Contains(line, staffMarker("settings_state_sw_on")) {
			t.Errorf("Main status line = %q (found %v), want 🔘 and the {n} on state", line, found)
		}
		button, ok := settingsPageButton(t, mainKeyboard, settingsPageSwitches)
		if !ok || !strings.HasPrefix(button.Text, "🔘 ") || !containsMarker(button.Text, "settings_page_switches") {
			t.Fatalf("Main has no 🔘 Switches button: %+v", mainKeyboard)
		}
		env.pressRaw(env.chat, env.admin, msgID, button.CallbackData)

		text, keyboard := env.lastEdit(env.chat.Id, msgID)
		for _, want := range []string{
			staffMarker("settings_header") + " " + staffMarker("settings_page_switches"),
			staffMarker("reports_status_enabled_group"),
		} {
			if !strings.Contains(text, want) {
				t.Errorf("Switches page %q does not contain %q", text, want)
			}
		}
		toggle, ok := settingsButtonByMarker(keyboard, "settings_btn_sw_reports")
		if !ok {
			t.Fatalf("the page has no Reports toggle: %+v", keyboard)
		}
		fields := settingsButtonFields(t, toggle)
		if fields["a"] != "tg" || fields["p"] != "t" || fields["t"] != "r" || fields["v"] != "0" {
			t.Errorf("Reports toggle fields = %v, want a=tg p=t t=r v=0 (it is on, the target is off)", fields)
		}
		last := keyboard[len(keyboard)-1]
		if len(last) != 2 || !containsMarker(last[0].Text, "settings_btn_back") || !containsMarker(last[1].Text, "settings_btn_close") {
			t.Errorf("last row = %+v, want Back and Close", last)
		}
		settingsSwNoRows(t, env.chat.Id, "opening Main and the page")
		if got := settingsSwChatRows(t, env.chat.Id); got != chatsBefore {
			t.Errorf("chats rows went from %d to %d on opening Main and the page", chatsBefore, got)
		}
	})

	t.Run("in a private menu", func(t *testing.T) {
		env := newSettingsSwitchesEnv(t)
		owner := env.newOwner(env.chat)
		env.sendPrivate(owner, fmt.Sprintf("/start settings_%d", env.chat.Id))
		sentMain := settingsPrivateOnlySent(t, env, owner)
		button, ok := settingsPageButton(t, staffKeyboardOf(sentMain.Params["reply_markup"]), settingsPageSwitches)
		if !ok {
			t.Fatal("the private Main has no Switches button")
		}
		env.pressRaw(settingsPrivateChat(owner), owner, sentMain.MessageID, button.CallbackData)
		text, keyboard := env.lastEdit(owner.Id, sentMain.MessageID)
		if !strings.Contains(text, staffMarker("reports_status_enabled_group")) {
			t.Errorf("private Switches page = %q, want the Reports sentence", text)
		}
		toggle, ok := settingsButtonByMarker(keyboard, "settings_btn_sw_reports")
		if !ok {
			t.Fatalf("the private page has no Reports toggle: %+v", keyboard)
		}
		if fields := settingsButtonFields(t, toggle); fields["c"] != fmt.Sprint(env.chat.Id) || fields["a"] != "tg" {
			t.Errorf("Reports toggle fields = %v, want a=tg and the group in c", fields)
		}
		settingsSwNoRows(t, env.chat.Id, "a private open")
	})

	t.Run("Main reads Off when none of the three is on", func(t *testing.T) {
		env := newSettingsSwitchesEnv(t)
		if err := reports.SetChatReportStatus(env.chat.Id, false); err != nil {
			t.Fatalf("seed report status: %v", err)
		}
		msgID := env.openMenu(env.admin)
		mainText, _ := env.lastEdit(env.chat.Id, msgID)
		line, found := settingsMainStateLine(mainText, "settings_page_switches")
		if !found || !strings.Contains(line, staffMarker("settings_state_off_plain")) {
			t.Errorf("Main status line = %q (found %v), want the plain Off state", line, found)
		}
	})

	t.Run("a failing reader is the cannot-open edit and the unknown mark", func(t *testing.T) {
		env := newSettingsSwitchesEnv(t)
		msgID := env.openMenu(env.admin)
		_, mainKeyboard := env.lastEdit(env.chat.Id, msgID)
		button, ok := settingsPageButton(t, mainKeyboard, settingsPageSwitches)
		if !ok {
			t.Fatal("Main has no Switches button")
		}
		previous := settingsSwReportsRead
		settingsSwReportsRead = func(_ context.Context, _ int64) (bool, error) { return false, errors.New("database is down") }
		t.Cleanup(func() { settingsSwReportsRead = previous })

		env.pressRaw(env.chat, env.admin, msgID, button.CallbackData)
		text, _ := env.lastEdit(env.chat.Id, msgID)
		if !strings.Contains(text, staffMarker("settings_cannot_open")) {
			t.Errorf("a failed read drew %q, want the cannot-open edit", text)
		}
		msgID = env.openMenu(env.admin)
		mainText, _ := env.lastEdit(env.chat.Id, msgID)
		line, found := settingsMainStateLine(mainText, "settings_page_switches")
		if !found || !strings.HasSuffix(line, settingsUnknownState) {
			t.Errorf("Main status line = %q (found %v), want the unknown mark", line, found)
		}
	})
}

// settingsSwRunReports runs /reports with args as the env's administrator in chat through the
// real dispatcher.
func settingsSwRunReports(env *settingsEnv, chat gotgbot.Chat, args string) {
	env.t.Helper()
	env.sendIn(chat, env.admin, "/reports "+args)
}

func TestSettingsParityReports(t *testing.T) {
	const menuMsgID = 7171

	// run changes the same chat state two ways, /reports on one chat and a press of the page on
	// another, and requires the same stored row and the same chats row.
	run := func(t *testing.T, seed bool, word string, taps int) {
		t.Helper()
		env := newSettingsSwitchesEnv(t)
		commandChat := env.chat
		menuChat := env.addChat()
		settingsSwCleanup(t, commandChat.Id, menuChat.Id)
		if seed {
			for _, id := range []int64{commandChat.Id, menuChat.Id} {
				if err := reports.SetChatReportStatus(id, word != "on"); err != nil {
					t.Fatalf("seed report status: %v", err)
				}
			}
		}
		value := "0"
		if word == "on" {
			value = "1"
		}

		settingsSwRunReports(env, commandChat, word)
		for range taps {
			settingsSwToggle(t, env, menuChat, menuMsgID, "r", value)
		}

		commandRow, ok := settingsSwReportsRow(t, commandChat.Id)
		if !ok {
			t.Fatal("/reports stored no row")
		}
		menuRow, ok := settingsSwReportsRow(t, menuChat.Id)
		if !ok {
			t.Fatal("the page stored no row")
		}
		want := word == "on"
		if commandRow.Enabled != want || commandRow.Status != want {
			t.Errorf("/reports %s stored enabled %v status %v, want both %v", word, commandRow.Enabled, commandRow.Status, want)
		}
		if menuRow.Enabled != commandRow.Enabled || menuRow.Status != commandRow.Status {
			t.Errorf("the page stored enabled %v status %v, the command enabled %v status %v, want the same row",
				menuRow.Enabled, menuRow.Status, commandRow.Enabled, commandRow.Status)
		}
		if c, m := settingsSwChatRows(t, commandChat.Id), settingsSwChatRows(t, menuChat.Id); c != m {
			t.Errorf("chats rows: command %d, page %d, want the same", c, m)
		}
		// The report path reads the chat's status through the command's own getter.
		if got := reports.GetChatReportSettings(menuChat.Id).Status; got != want {
			t.Errorf("the report path reads status %v after the page, want %v", got, want)
		}
		text, _ := env.lastEdit(menuChat.Id, menuMsgID)
		sentence := "reports_status_disabled_group"
		if want {
			sentence = "reports_status_enabled_group"
		}
		if !strings.Contains(text, staffMarker(sentence)) || strings.Contains(text, "⚠️") {
			t.Errorf("the page redrew %q, want the %s sentence and no notice", text, sentence)
		}
	}

	t.Run("off on a chat with no row", func(t *testing.T) { run(t, false, "off", 1) })
	t.Run("on on a chat with no row", func(t *testing.T) { run(t, false, "on", 1) })
	t.Run("off on a chat with a row", func(t *testing.T) { run(t, true, "off", 1) })
	t.Run("on on a chat with a row", func(t *testing.T) { run(t, true, "on", 1) })
	t.Run("a double tap leaves the same state", func(t *testing.T) { run(t, true, "off", 2) })

	t.Run("a bot that is not an admin is refused like the command", func(t *testing.T) {
		env := newSettingsSwitchesEnv(t)
		chat := env.addChat()
		settingsSwCleanup(t, chat.Id)
		env.fake.setBotRole(chat.Id, staffRoleMember)
		refusal := staffMarker("connections_is_user_connected_bot_not_admin")

		settingsSwRunReports(env, chat, "off")
		if !settingsAntifloodSentHas(env, chat.Id, refusal) {
			t.Fatal("/reports off was not refused for a bot that is not an admin")
		}

		answers := env.answerCount()
		settingsSwToggle(t, env, chat, menuMsgID, "r", "0")
		text, _ := env.lastEdit(chat.Id, menuMsgID)
		if !strings.Contains(text, refusal) {
			t.Errorf("the press drew %q, want the notice %s", text, refusal)
		}
		if got := env.answerCount() - answers; got != 1 {
			t.Errorf("the press answered %d times, want once", got)
		}
		if _, has := settingsSwReportsRow(t, chat.Id); has {
			t.Error("a refused press or command wrote a row")
		}
	})

	t.Run("a failing write redraws the stored state with the save-failed notice", func(t *testing.T) {
		env := newSettingsSwitchesEnv(t)
		chat := env.addChat()
		settingsSwCleanup(t, chat.Id)
		if err := reports.SetChatReportStatus(chat.Id, true); err != nil {
			t.Fatalf("seed: %v", err)
		}
		previous := reportsWrites
		reportsWrites.ChatStatus = func(int64, bool) error { return errors.New("disk is full") }
		t.Cleanup(func() { reportsWrites = previous })

		settingsSwToggle(t, env, chat, menuMsgID, "r", "0")
		text, _ := env.lastEdit(chat.Id, menuMsgID)
		if !strings.Contains(text, staffMarker("common_settings_save_failed")) || !strings.Contains(text, staffMarker("reports_status_enabled_group")) {
			t.Errorf("the page drew %q, want the stored (enabled) sentence and the save-failed notice", text)
		}
	})
}
