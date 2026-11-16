//go:build testtools

package modules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/aispam"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/chats"
	"github.com/uasneppy/fuku_robot/fuku/db/greetings"
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

// newSettingsSwitchesEnv is the settings environment plus the Reports and AI spam commands (and
// the Greetings commands, for /autoapprove), so a parity test can run each command next to the
// page. The env's chat has no report, AI spam or greetings row.
func newSettingsSwitchesEnv(t *testing.T) *settingsEnv {
	t.Helper()
	env := newSettingsEditEnv(t)
	LoadReports(env.dispatcher)
	LoadAISpam(env.dispatcher)
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

// settingsSwAIRow reads the AI spam row of chatID straight from the database. ok is false when
// the chat has no row.
func settingsSwAIRow(t *testing.T, chatID int64) (row models.AISpamSettings, ok bool) {
	t.Helper()
	if err := db.DB.Where("chat_id = ?", chatID).Limit(1).Find(&row).Error; err != nil {
		t.Fatalf("read ai_spam_settings of %d: %v", chatID, err)
	}
	return row, row.ID != 0
}

// settingsSwAICached reports whether Redis holds an ai_spam_settings cache key for chatID.
func settingsSwAICached(env *settingsEnv, chatID int64) bool {
	for _, key := range env.redis.Keys() {
		if strings.Contains(key, "ai_spam_settings") && strings.Contains(key, fmt.Sprint(chatID)) {
			return true
		}
	}
	return false
}

// settingsSwRowJSON is a greetings row as JSON with the fields that differ per chat and per run
// (its ID, chat and timestamps) cleared, so two chats' rows can be compared.
func settingsSwRowJSON(t *testing.T, row models.GreetingSettings) string {
	t.Helper()
	row.ID, row.ChatID = 0, 0
	row.CreatedAt, row.UpdatedAt = time.Time{}, time.Time{}
	raw, err := json.Marshal(row)
	if err != nil {
		t.Fatalf("marshal greetings row: %v", err)
	}
	return string(raw)
}

// settingsSwEditorState opens the Welcome and Goodbye pages of chat on message msgID and returns
// the text of both, so a test can compare what they draw before and after a write.
func settingsSwEditorState(t *testing.T, env *settingsEnv, chat gotgbot.Chat, msgID int64) string {
	t.Helper()
	var drawn []string
	for _, page := range []string{"w", "g"} {
		settingsSwPress(t, env, chat, msgID, "a", "op", "p", page)
		text, _ := env.lastEdit(chat.Id, msgID)
		drawn = append(drawn, text)
	}
	return strings.Join(drawn, "\n---\n")
}

func TestSettingsParityAISpam(t *testing.T) {
	const menuMsgID = 7272

	// run turns the filter on or off on two chats, /aispam on one and a press of the page on
	// the other, and requires the same stored row and the same cache state.
	run := func(t *testing.T, seedOn bool, word string) {
		t.Helper()
		withAISpamConfigured(t)
		env := newSettingsSwitchesEnv(t)
		commandChat := env.chat
		menuChat := env.addChat()
		settingsSwCleanup(t, commandChat.Id, menuChat.Id)
		if seedOn {
			for _, id := range []int64{commandChat.Id, menuChat.Id} {
				if err := aispam.SetAISpamEnabled(id, word == "off"); err != nil {
					t.Fatalf("seed: %v", err)
				}
			}
		}
		value := "0"
		if word == "on" {
			value = "1"
		}

		env.sendIn(commandChat, env.admin, "/aispam "+word)
		settingsSwToggle(t, env, menuChat, menuMsgID, "a", value)

		commandRow, ok := settingsSwAIRow(t, commandChat.Id)
		if !ok {
			t.Fatal("/aispam stored no row")
		}
		menuRow, ok := settingsSwAIRow(t, menuChat.Id)
		if !ok {
			t.Fatal("the page stored no row")
		}
		want := word == "on"
		if commandRow.Enabled != want || menuRow.Enabled != commandRow.Enabled {
			t.Errorf("/aispam %s stored %v and the page %v, want both %v", word, commandRow.Enabled, menuRow.Enabled, want)
		}
		if c, m := settingsSwAICached(env, commandChat.Id), settingsSwAICached(env, menuChat.Id); c != m {
			t.Errorf("cache key present: command %v, page %v, want the same", c, m)
		}
		if got := aispam.IsAISpamEnabled(menuChat.Id); got != want {
			t.Errorf("the enforcing path reads %v after the page, want %v", got, want)
		}
		text, _ := env.lastEdit(menuChat.Id, menuMsgID)
		sentence := "settings_sw_ai_off"
		if want {
			sentence = "aispam_status_enabled"
		}
		if !strings.Contains(text, staffMarker(sentence)) || strings.Contains(text, "⚠️") {
			t.Errorf("the page redrew %q, want the %s sentence and no notice", text, sentence)
		}
	}

	t.Run("on, configured, a chat with no row", func(t *testing.T) { run(t, false, "on") })
	t.Run("off, configured, a chat with a row", func(t *testing.T) { run(t, true, "off") })
	for _, how := range []struct {
		name string
		kind aispamNotConfiguredHow
	}{{"the provider value empty", aispamProviderValueEmpty}, {"the global switch off", aispamSwitchOff}} {
		t.Run("on refuses with "+how.name+" and stores nothing", func(t *testing.T) {
			withAISpamNotConfigured(t, how.kind)
			env := newSettingsSwitchesEnv(t)
			commandChat := env.chat
			menuChat := env.addChat()
			settingsSwCleanup(t, commandChat.Id, menuChat.Id)

			env.sendIn(commandChat, env.admin, "/aispam on")
			settingsSwToggle(t, env, menuChat, menuMsgID, "a", "1")

			if _, has := settingsSwAIRow(t, commandChat.Id); has {
				t.Error("/aispam on stored a row while unavailable")
			}
			if _, has := settingsSwAIRow(t, menuChat.Id); has {
				t.Error("the page's On press stored a row while unavailable")
			}
			if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("settings_sw_ai_unavailable")) {
				t.Errorf("answer = %q (alert %v), want the unavailable alert", answer, alert)
			}
		})
	}
}

func TestSettingsParityAutoApprove(t *testing.T) {
	const menuMsgID = 7373

	// run turns Auto-approve on or off on two chats, /autoapprove on one and a press of the page
	// on the other, and requires the same whole greetings row, and that the Welcome and Goodbye
	// pages draw the same before and after the press.
	run := func(t *testing.T, seeded bool, word string) {
		t.Helper()
		env := newSettingsSwitchesEnv(t)
		commandChat := env.chat
		menuChat := env.addChat()
		settingsSwCleanup(t, commandChat.Id, menuChat.Id)
		listEditAdmin(env, commandChat, env.admin, true)
		listEditAdmin(env, menuChat, env.admin, true)
		// A group the bot serves is in the chats table: the greetings pages read a missing row as
		// Goodbye on for such a chat, which is what the first write stores (C-09).
		for _, id := range []int64{commandChat.Id, menuChat.Id} {
			if err := chats.EnsureChatInDb(id, "Switches Chat"); err != nil {
				t.Fatalf("ensure chat: %v", err)
			}
		}
		if seeded {
			for _, id := range []int64{commandChat.Id, menuChat.Id} {
				if err := greetings.SetShouldAutoApprove(id, word == "off"); err != nil {
					t.Fatalf("seed: %v", err)
				}
			}
		}
		value := "0"
		if word == "on" {
			value = "1"
		}
		beforeState := settingsSwEditorState(t, env, menuChat, menuMsgID)

		env.sendIn(commandChat, env.admin, "/autoapprove "+word)
		settingsSwToggle(t, env, menuChat, menuMsgID, "j", value)

		commandRow, ok := settingsEditStored(t, commandChat.Id)
		if !ok {
			t.Fatal("/autoapprove stored no row")
		}
		menuRow, ok := settingsEditStored(t, menuChat.Id)
		if !ok {
			t.Fatal("the page stored no row")
		}
		if commandRow.ShouldAutoApprove != (word == "on") {
			t.Errorf("/autoapprove %s stored %v", word, commandRow.ShouldAutoApprove)
		}
		if c, m := settingsSwRowJSON(t, commandRow), settingsSwRowJSON(t, menuRow); c != m {
			t.Errorf("the page stored a different row\n command %s\n    page %s", c, m)
		}
		if after := settingsSwEditorState(t, env, menuChat, menuMsgID); after != beforeState {
			t.Errorf("the Welcome and Goodbye pages changed with the first write\n before %q\n  after %q", beforeState, after)
		}
	}

	t.Run("on, a chat with no greetings row", func(t *testing.T) { run(t, false, "on") })
	t.Run("off, a chat with no greetings row", func(t *testing.T) { run(t, false, "off") })
	t.Run("on, a chat with a row", func(t *testing.T) { run(t, true, "on") })
	t.Run("off, a chat with a row", func(t *testing.T) { run(t, true, "off") })

	t.Run("the page redraws the state sentence from the read-back and a double tap is the same state", func(t *testing.T) {
		env := newSettingsSwitchesEnv(t)
		chat := env.addChat()
		settingsSwCleanup(t, chat.Id)
		listEditAdmin(env, chat, env.admin, true)
		settingsSwToggle(t, env, chat, menuMsgID, "j", "1")
		settingsSwToggle(t, env, chat, menuMsgID, "j", "1")
		text, _ := env.lastEdit(chat.Id, menuMsgID)
		if !strings.Contains(text, staffMarker("greetings_auto_approve_enabled")) || strings.Contains(text, "⚠️") {
			t.Errorf("the page drew %q, want the enabled sentence and no notice", text)
		}
		settingsSwToggle(t, env, chat, menuMsgID, "j", "0")
		text, _ = env.lastEdit(chat.Id, menuMsgID)
		if !strings.Contains(text, staffMarker("greetings_auto_approve_disabled")) {
			t.Errorf("the page drew %q, want the disabled sentence", text)
		}
	})

	t.Run("an administrator without can_change_info is refused like the command", func(t *testing.T) {
		env := newSettingsSwitchesEnv(t)
		chat := env.addChat()
		settingsSwCleanup(t, chat.Id)
		presser := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Presser"}
		listEditAdmin(env, chat, presser, false)

		env.sendIn(chat, presser, "/autoapprove on")
		if !settingsAntifloodSentHas(env, chat.Id, staffMarker("chat_status_change_info_cmd_error")) {
			t.Fatal("/autoapprove on was not refused for an administrator without can_change_info")
		}
		answers := env.answerCount()
		fields := env.groupFields(presser, "a", "tg", "p", "t", "t", "j", "v", "1")
		data := encodeCallbackData(settingsCallbackNamespace, fields)
		env.pressRaw(chat, presser, menuMsgID, data)
		if got := env.answerCount() - answers; got != 1 {
			t.Errorf("the press answered %d times, want once", got)
		}
		if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("settings_no_change_info")) {
			t.Errorf("answer = %q (alert %v), want the settings_no_change_info alert", answer, alert)
		}
		if _, has := settingsEditStored(t, chat.Id); has {
			t.Error("a refused press or command wrote a greetings row")
		}
	})

	t.Run("a bot that is not an admin is refused like the command", func(t *testing.T) {
		env := newSettingsSwitchesEnv(t)
		chat := env.addChat()
		settingsSwCleanup(t, chat.Id)
		env.fake.setBotRole(chat.Id, staffRoleMember)
		refusal := staffMarker("connections_is_user_connected_bot_not_admin")

		env.sendIn(chat, env.admin, "/autoapprove on")
		if !settingsAntifloodSentHas(env, chat.Id, refusal) {
			t.Fatal("/autoapprove on was not refused for a bot that is not an admin")
		}
		settingsSwToggle(t, env, chat, menuMsgID, "j", "1")
		text, _ := env.lastEdit(chat.Id, menuMsgID)
		if !strings.Contains(text, refusal) {
			t.Errorf("the press drew %q, want the notice %s", text, refusal)
		}
		if _, has := settingsEditStored(t, chat.Id); has {
			t.Error("a refused press or command wrote a greetings row")
		}
	})

	t.Run("a failing write redraws the stored state with the save-failed notice", func(t *testing.T) {
		env := newSettingsSwitchesEnv(t)
		chat := env.addChat()
		settingsSwCleanup(t, chat.Id)
		listEditAdmin(env, chat, env.admin, true)
		previous := greetingWrites
		greetingWrites.AutoApprove = func(int64, bool) error { return errors.New("disk is full") }
		t.Cleanup(func() { greetingWrites = previous })

		settingsSwToggle(t, env, chat, menuMsgID, "j", "1")
		text, _ := env.lastEdit(chat.Id, menuMsgID)
		if !strings.Contains(text, staffMarker("common_settings_save_failed")) || !strings.Contains(text, staffMarker("greetings_auto_approve_disabled")) {
			t.Errorf("the page drew %q, want the stored (disabled) sentence and the save-failed notice", text)
		}
	})
}

// settingsSwControls are the three controls, each as the press that turns it on and the command
// it mirrors, with the test that its write is stored.
var settingsSwControls = []struct {
	name    string
	press   []string
	command string
	wrote   func(t *testing.T, chatID int64) bool
}{
	{"Reports", []string{"a", "tg", "p", "t", "t", "r", "v", "0"}, "/reports off",
		func(t *testing.T, id int64) bool { row, ok := settingsSwReportsRow(t, id); return ok && !row.Status }},
	{"AI spam", []string{"a", "tg", "p", "t", "t", "a", "v", "1"}, "/aispam on",
		func(t *testing.T, id int64) bool { row, ok := settingsSwAIRow(t, id); return ok && row.Enabled }},
	{"Auto-approve", []string{"a", "tg", "p", "t", "t", "j", "v", "1"}, "/autoapprove on",
		func(t *testing.T, id int64) bool {
			row, ok := settingsEditStored(t, id)
			return ok && row.ShouldAutoApprove
		}},
}

func TestSettingsDriftReports(t *testing.T) {
	const menuMsgID = 7575
	env := newSettingsSwitchesEnv(t)
	chat := env.addChat()
	settingsSwCleanup(t, chat.Id)

	// A command write is what the next draw shows, across the cache.
	settingsSwRunReports(env, chat, "off")
	dbcache.ResetLocalForTest()
	settingsSwPress(t, env, chat, menuMsgID, "a", "op", "p", "t")
	text, keyboard := env.lastEdit(chat.Id, menuMsgID)
	if !strings.Contains(text, staffMarker("reports_status_disabled_group")) {
		t.Errorf("after /reports off the page drew %q, want the disabled sentence", text)
	}
	toggle, _ := settingsButtonByMarker(keyboard, "settings_btn_sw_reports")
	if fields := settingsButtonFields(t, toggle); fields["v"] != "1" {
		t.Errorf("Reports toggle target = %q after /reports off, want 1", fields["v"])
	}

	// A page press is what /reports and the report path read.
	settingsSwToggle(t, env, chat, menuMsgID, "r", "1")
	dbcache.ResetLocalForTest()
	if !reports.GetChatReportSettings(chat.Id).Status {
		t.Error("the report path reads status off after the page turned Reports on")
	}
	before := len(env.fake.sentTo(chat.Id))
	settingsSwRunReports(env, chat, "")
	sent := env.fake.sentTo(chat.Id)
	if len(sent) != before+1 || !strings.Contains(fmt.Sprint(sent[len(sent)-1].Params["text"]), staffMarker("reports_status_enabled_group")) {
		t.Errorf("/reports after the page's On answered %+v, want the enabled status", sent[before:])
	}
}

func TestSettingsDriftAISpam(t *testing.T) {
	const menuMsgID = 7676
	withAISpamConfigured(t)
	env := newSettingsSwitchesEnv(t)
	chat := env.addChat()
	settingsSwCleanup(t, chat.Id)

	env.sendIn(chat, env.admin, "/aispam on")
	dbcache.ResetLocalForTest()
	settingsSwPress(t, env, chat, menuMsgID, "a", "op", "p", "t")
	text, keyboard := env.lastEdit(chat.Id, menuMsgID)
	if !strings.Contains(text, staffMarker("aispam_status_enabled")) {
		t.Errorf("after /aispam on the page drew %q, want the on sentence", text)
	}
	toggle, _ := settingsButtonByMarker(keyboard, "settings_btn_sw_aispam")
	if fields := settingsButtonFields(t, toggle); fields["v"] != "0" {
		t.Errorf("AI toggle target = %q after /aispam on, want 0", fields["v"])
	}

	settingsSwToggle(t, env, chat, menuMsgID, "a", "0")
	dbcache.ResetLocalForTest()
	if aispam.IsAISpamEnabled(chat.Id) {
		t.Error("the enforcing path reads the filter on after the page turned it off")
	}
	before := len(env.fake.sentTo(chat.Id))
	env.sendIn(chat, env.admin, "/aispam")
	sent := env.fake.sentTo(chat.Id)
	if len(sent) != before+1 || !strings.Contains(fmt.Sprint(sent[len(sent)-1].Params["text"]), staffMarker("aispam_status_disabled")) {
		t.Errorf("/aispam after the page's Off answered %+v, want the off status", sent[before:])
	}
}

func TestSettingsDriftAutoApprove(t *testing.T) {
	const menuMsgID = 7777
	env := newSettingsSwitchesEnv(t)
	chat := env.addChat()
	settingsSwCleanup(t, chat.Id)
	listEditAdmin(env, chat, env.admin, true)

	env.sendIn(chat, env.admin, "/autoapprove on")
	dbcache.ResetLocalForTest()
	settingsSwPress(t, env, chat, menuMsgID, "a", "op", "p", "t")
	text, keyboard := env.lastEdit(chat.Id, menuMsgID)
	if !strings.Contains(text, staffMarker("greetings_auto_approve_enabled")) {
		t.Errorf("after /autoapprove on the page drew %q, want the enabled sentence", text)
	}
	toggle, _ := settingsButtonByMarker(keyboard, "settings_btn_sw_approve")
	if fields := settingsButtonFields(t, toggle); fields["v"] != "0" {
		t.Errorf("Auto-approve toggle target = %q after /autoapprove on, want 0", fields["v"])
	}

	settingsSwToggle(t, env, chat, menuMsgID, "j", "0")
	dbcache.ResetLocalForTest()
	if greetings.GetGreetingSettings(chat.Id).ShouldAutoApprove {
		t.Error("the join-request handler reads auto-approve on after the page turned it off")
	}
	before := len(env.fake.sentTo(chat.Id))
	env.sendIn(chat, env.admin, "/autoapprove")
	sent := env.fake.sentTo(chat.Id)
	if len(sent) != before+1 || !strings.Contains(fmt.Sprint(sent[len(sent)-1].Params["text"]), staffMarker("greetings_auto_approve_disabled")) {
		t.Errorf("/autoapprove after the page's Off answered %+v, want the disabled status", sent[before:])
	}
}

func TestSettingsAuthorityParitySwitches(t *testing.T) {
	const menuMsgID = 7878

	// newChat adds a chat whose administrator list and live answers make env.admin an
	// administrator with can_change_info, with the AI filter available.
	newChat := func(t *testing.T, env *settingsEnv) gotgbot.Chat {
		t.Helper()
		chat := env.addChat()
		settingsSwCleanup(t, chat.Id)
		listEditAdmin(env, chat, env.admin, true)
		return chat
	}

	t.Run("a bot that is not an admin: the menu and the commands agree", func(t *testing.T) {
		withAISpamConfigured(t)
		for _, control := range settingsSwControls {
			t.Run(control.name, func(t *testing.T) {
				env := newSettingsSwitchesEnv(t)
				commandChat, menuChat := newChat(t, env), newChat(t, env)
				env.fake.setBotRole(commandChat.Id, staffRoleMember)
				env.fake.setBotRole(menuChat.Id, staffRoleMember)
				refusal := staffMarker("connections_is_user_connected_bot_not_admin")

				env.sendIn(commandChat, env.admin, control.command)
				commandRefused := settingsAntifloodSentHas(env, commandChat.Id, refusal)
				answers := env.answerCount()
				settingsSwPress(t, env, menuChat, menuMsgID, control.press...)
				text, _ := env.lastEdit(menuChat.Id, menuMsgID)
				menuRefused := strings.Contains(text, refusal)
				// /aispam has no bot check, so for it both write.
				wantRefused := control.name != "AI spam"
				if commandRefused != wantRefused || menuRefused != wantRefused {
					t.Errorf("bot refusal: command %v, menu %v, want both %v", commandRefused, menuRefused, wantRefused)
				}
				if got := env.answerCount() - answers; got != 1 {
					t.Errorf("the press answered %d times, want once", got)
				}
				for _, id := range []int64{commandChat.Id, menuChat.Id} {
					if got := control.wrote(t, id); got == wantRefused {
						t.Errorf("chat %d: write stored = %v, want %v", id, got, !wantRefused)
					}
				}
			})
		}
	})

	t.Run("an administrator without can_change_info: Auto-approve is refused by both, the others write", func(t *testing.T) {
		withAISpamConfigured(t)
		for _, control := range settingsSwControls {
			t.Run(control.name, func(t *testing.T) {
				env := newSettingsSwitchesEnv(t)
				commandChat, menuChat := env.addChat(), env.addChat()
				settingsSwCleanup(t, commandChat.Id, menuChat.Id)
				listEditAdmin(env, commandChat, env.admin, false)
				listEditAdmin(env, menuChat, env.admin, false)
				wantRefused := control.name == "Auto-approve"

				env.sendIn(commandChat, env.admin, control.command)
				answers := env.answerCount()
				settingsSwPress(t, env, menuChat, menuMsgID, control.press...)
				if got := env.answerCount() - answers; got != 1 {
					t.Errorf("the press answered %d times, want once", got)
				}
				if wantRefused {
					if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("settings_no_change_info")) {
						t.Errorf("answer = %q (alert %v), want the alert settings_no_change_info", answer, alert)
					}
					if !settingsAntifloodSentHas(env, commandChat.Id, staffMarker("chat_status_change_info_cmd_error")) {
						t.Error("/autoapprove on was not refused for can_change_info")
					}
				}
				for _, id := range []int64{commandChat.Id, menuChat.Id} {
					if got := control.wrote(t, id); got == wantRefused {
						t.Errorf("chat %d: write stored = %v, want %v", id, got, !wantRefused)
					}
				}
			})
		}
	})

	t.Run("the bot refusal comes before the change-info refusal for Auto-approve", func(t *testing.T) {
		env := newSettingsSwitchesEnv(t)
		chat := env.addChat()
		settingsSwCleanup(t, chat.Id)
		listEditAdmin(env, chat, env.admin, false)
		env.fake.setBotRole(chat.Id, staffRoleMember)
		settingsSwPress(t, env, chat, menuMsgID, "a", "tg", "p", "t", "t", "j", "v", "1")
		text, _ := env.lastEdit(chat.Id, menuMsgID)
		if !strings.Contains(text, staffMarker("connections_is_user_connected_bot_not_admin")) {
			t.Errorf("the press drew %q, want the bot notice first", text)
		}
	})

	t.Run("a plain member is refused by both", func(t *testing.T) {
		withAISpamConfigured(t)
		for _, control := range settingsSwControls {
			t.Run(control.name, func(t *testing.T) {
				env := newSettingsSwitchesEnv(t)
				chat := newChat(t, env)
				member := gotgbot.User{Id: uniqueLinkOwnerID() + 43, FirstName: "Plain"}
				t.Cleanup(func() {
					db.DB.Where("user_id IN ?", []int64{member.Id}).Delete(&models.User{})
					dbcache.DeleteCache(dbcache.CacheKey("user", member.Id))
					dbcache.ResetLocalForTest()
				})
				env.fake.setMember(chat.Id, member.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})

				env.sendIn(chat, member, control.command)
				env.pressPrivate(member, 777, env.privateFields(chat.Id, control.press...))
				if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("settings_not_admin")) {
					t.Errorf("answer = %q (alert %v), want the alert settings_not_admin", answer, alert)
				}
				if control.wrote(t, chat.Id) {
					t.Error("a plain member's press or command wrote")
				}
			})
		}
	})

	t.Run("an administrator the owner gate refuses is refused by the menu only (the documented carve-out)", func(t *testing.T) {
		withAISpamConfigured(t)
		for _, control := range settingsSwControls {
			t.Run(control.name, func(t *testing.T) {
				resetSettingsOwnerMemo(t)
				env := newSettingsSwitchesEnv(t)
				commandChat, menuChat := newChat(t, env), newChat(t, env)
				for _, chat := range []gotgbot.Chat{commandChat, menuChat} {
					env.newOwner(chat)
					env.setAdminsAllowed(chat.Id, false)
				}

				answers := env.answerCount()
				settingsSwPress(t, env, menuChat, menuMsgID, control.press...)
				if got := env.answerCount() - answers; got != 1 {
					t.Errorf("the gated press answered %d times, want once", got)
				}
				if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("settings_gate_owner_only")) {
					t.Errorf("answer = %q (alert %v), want the alert settings_gate_owner_only", answer, alert)
				}
				if control.wrote(t, menuChat.Id) {
					t.Error("the gated press wrote")
				}
				env.sendIn(commandChat, env.admin, control.command)
				if !control.wrote(t, commandChat.Id) {
					t.Errorf("%s did not write for an administrator the menu refuses", control.command)
				}
				env.setAdminsAllowed(menuChat.Id, true)
				settingsSwPress(t, env, menuChat, menuMsgID, control.press...)
				if !control.wrote(t, menuChat.Id) {
					t.Error("with the switch on the press did not write")
				}
			})
		}
	})
}

func TestSettingsSwitchesForged(t *testing.T) {
	const menuMsgID = 7979

	type forgedCase struct {
		name   string
		fields []string
		extra  map[string]string
		both   bool
	}
	var forged []forgedCase
	for _, control := range []string{"r", "a", "j"} {
		tg := func(v string) []string { return []string{"a", "tg", "p", "t", "t", control, "v", v} }
		forged = append(forged,
			forgedCase{name: "st on " + control, fields: []string{"a", "st", "p", "t", "t", control, "v", "1"}},
			forgedCase{name: "v=2 on " + control, fields: tg("2")},
			forgedCase{name: "empty v on " + control, fields: tg("")},
			forgedCase{name: "v=on on " + control, fields: tg("on")},
			forgedCase{name: "an extra key on " + control, fields: tg("1"), extra: map[string]string{"x": "1"}},
			forgedCase{name: "c and u together on " + control, fields: tg("1"), both: true},
		)
	}
	forged = append(forged, forgedCase{name: "an unknown control", fields: []string{"a", "tg", "p", "t", "t", "x", "v", "1"}})

	for _, tc := range forged {
		t.Run(tc.name, func(t *testing.T) {
			withAISpamConfigured(t)
			env := newSettingsSwitchesEnv(t)
			fields := env.groupFields(env.admin, tc.fields...)
			for k, v := range tc.extra {
				fields[k] = v
			}
			if tc.both {
				fields["c"] = "-1001"
			}
			data := encodeCallbackData(settingsCallbackNamespace, fields)
			if data == "" {
				t.Fatalf("forged press %v did not encode", fields)
			}
			before := settingsMenuMarkOf(env)
			env.pressRaw(env.chat, env.admin, menuMsgID, data)
			settingsMenuWantRefused(t, env, before, "common_callback_invalid_request", true)
			settingsSwNoRows(t, env.chat.Id, "a forged press")
		})
	}
}

func TestSettingsSwitchesReadsCreateNothing(t *testing.T) {
	const menuMsgID = 8080

	t.Run("opening Main and the page leaves no row", func(t *testing.T) {
		env := newSettingsSwitchesEnv(t)
		chat := env.addChat()
		settingsSwCleanup(t, chat.Id)
		chatsBefore := settingsSwChatRows(t, chat.Id)

		settingsSwPress(t, env, chat, menuMsgID, "a", "op", "p", "t")
		text, _ := env.lastEdit(chat.Id, menuMsgID)
		if !strings.Contains(text, staffMarker("reports_status_enabled_group")) || !strings.Contains(text, staffMarker("greetings_auto_approve_disabled")) {
			t.Fatalf("the page drew %q, want the defaults (Reports on, Auto-approve off)", text)
		}
		settingsSwPress(t, env, chat, menuMsgID, "a", "bk")
		mainText, _ := env.lastEdit(chat.Id, menuMsgID)
		if line, found := settingsMainStateLine(mainText, "settings_page_switches"); !found || strings.HasSuffix(line, settingsUnknownState) {
			t.Errorf("Main status line = %q (found %v), want the default state", line, found)
		}

		settingsSwNoRows(t, chat.Id, "opening Main and the page")
		if got := settingsSwChatRows(t, chat.Id); got != chatsBefore {
			t.Errorf("chats rows went from %d to %d", chatsBefore, got)
		}
		for _, key := range env.redis.Keys() {
			// The AI reader uses the cached getter /aispam reads, so its key is the command's own;
			// the other two readers are uncached and leave none.
			if strings.Contains(key, fmt.Sprint(chat.Id)) && (strings.Contains(key, "report") || strings.Contains(key, "greeting")) {
				t.Errorf("the reads left the cache key %s", key)
			}
		}
	})

	readers := []struct {
		name string
		fail func(t *testing.T)
	}{
		{"Reports", func(t *testing.T) {
			previous := settingsSwReportsRead
			settingsSwReportsRead = func(context.Context, int64) (bool, error) { return false, errors.New("database is down") }
			t.Cleanup(func() { settingsSwReportsRead = previous })
		}},
		{"AI spam", func(t *testing.T) {
			previous := settingsSwAIRead
			settingsSwAIRead = func(context.Context, int64) (bool, error) { return false, errors.New("database is down") }
			t.Cleanup(func() { settingsSwAIRead = previous })
		}},
		{"Auto-approve", func(t *testing.T) {
			previous := settingsSwApproveRead
			settingsSwApproveRead = func(context.Context, int64) (bool, error) { return false, errors.New("database is down") }
			t.Cleanup(func() { settingsSwApproveRead = previous })
		}},
	}
	for _, reader := range readers {
		t.Run("a failing "+reader.name+" reader is the cannot-open edit and the unknown mark", func(t *testing.T) {
			env := newSettingsSwitchesEnv(t)
			msgID := env.openMenu(env.admin)
			_, keyboard := env.lastEdit(env.chat.Id, msgID)
			button, ok := settingsPageButton(t, keyboard, settingsPageSwitches)
			if !ok {
				t.Fatal("Main has no Switches button")
			}
			reader.fail(t)

			msgID = env.openMenu(env.admin)
			mainText, _ := env.lastEdit(env.chat.Id, msgID)
			if line, found := settingsMainStateLine(mainText, "settings_page_switches"); !found || !strings.HasSuffix(line, settingsUnknownState) {
				t.Errorf("Switches status line = %q (found %v), want the unknown mark", line, found)
			}
			if line, found := settingsMainStateLine(mainText, "settings_page_antiflood"); !found || strings.HasSuffix(line, settingsUnknownState) {
				t.Errorf("Antiflood status line = %q (found %v), want its own state intact", line, found)
			}
			env.pressRaw(env.chat, env.admin, msgID, button.CallbackData)
			text, kb := env.lastEdit(env.chat.Id, msgID)
			if !strings.Contains(text, staffMarker("settings_cannot_open")) || len(kb) != 0 {
				t.Errorf("a failed read drew %q with %d rows, want the cannot-open edit", text, len(kb))
			}
			settingsSwNoRows(t, env.chat.Id, "a failed read")
		})
	}
}
