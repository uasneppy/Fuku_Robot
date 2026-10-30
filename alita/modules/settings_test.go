//go:build testtools

package modules

import (
	"fmt"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db"
	dbcache "github.com/divkix/Alita_Robot/alita/db/cache"
	"github.com/divkix/Alita_Robot/alita/db/captcha"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/i18n"
)

// settingsStoredTimeout reads the captcha time limit of chatID from the database row
// itself, past every cache layer. ok is false when the chat has no row.
func settingsStoredTimeout(t *testing.T, chatID int64) (timeout int, ok bool) {
	t.Helper()
	var row models.CaptchaSettings
	err := db.DB.Where("chat_id = ?", chatID).Limit(1).Find(&row).Error
	if err != nil {
		t.Fatalf("read captcha_settings of %d: %v", chatID, err)
	}
	if row.ID == 0 {
		return 0, false
	}
	return row.Timeout, true
}

// settingsShownTimeout reads the time limit the commands see, through the cache.
func settingsShownTimeout(t *testing.T, chatID int64) int {
	t.Helper()
	dbcache.ResetLocalForTest()
	settings, err := captcha.GetCaptchaSettings(chatID)
	if err != nil {
		t.Fatalf("GetCaptchaSettings(%d): %v", chatID, err)
	}
	return settings.Timeout
}

// settingsOpenCaptchaPage opens the menu as the env's administrator and presses the
// Captcha button, returning the menu message ID.
func settingsOpenCaptchaPage(t *testing.T, env *settingsEnv) int64 {
	t.Helper()
	msgID := env.openMenu(env.admin)
	_, keyboard := env.lastEdit(env.chat.Id, msgID)
	button, ok := settingsPageButton(t, keyboard, settingsPageCaptcha)
	if !ok {
		t.Fatal("the Main page has no Captcha button")
	}
	env.pressRaw(env.chat, env.admin, msgID, button.CallbackData)
	return msgID
}

// settingsPageButton finds the Main page button that opens the page with the given key, by
// its callback data, so it works with the marker locale and with the real one.
func settingsPageButton(t *testing.T, keyboard [][]gotgbot.InlineKeyboardButton, pageKey string) (gotgbot.InlineKeyboardButton, bool) {
	t.Helper()
	for _, button := range settingsAllButtons(keyboard) {
		fields := settingsButtonFields(t, button)
		if fields["a"] == "op" && fields["p"] == pageKey {
			return button, true
		}
	}
	return gotgbot.InlineKeyboardButton{}, false
}

// settingsStepper returns the three buttons of the captcha time stepper on the current
// page of msgID.
func settingsStepper(t *testing.T, env *settingsEnv, msgID int64) []gotgbot.InlineKeyboardButton {
	t.Helper()
	_, keyboard := env.lastEdit(env.chat.Id, msgID)
	if len(keyboard) != 2 {
		t.Fatalf("Captcha page has %d keyboard rows, want the stepper row and the navigation row", len(keyboard))
	}
	if len(keyboard[0]) != 3 {
		t.Fatalf("Captcha page stepper row has %d buttons, want 3", len(keyboard[0]))
	}
	return keyboard[0]
}

func TestSettingsTracer(t *testing.T) {
	t.Run("choice message", func(t *testing.T) {
		env := newSettingsEnv(t)
		commandID := env.sendIn(env.chat, env.admin, "/settings")

		sent := env.fake.sentTo(env.chat.Id)
		if len(sent) != 1 {
			t.Fatalf("the bot sent %d messages, want exactly the choice message", len(sent))
		}
		text := fmt.Sprint(sent[0].Params["text"])
		for _, want := range []string{staffMarker("settings_choose"), "Lock Chat"} {
			if !strings.Contains(text, want) {
				t.Errorf("choice message %q does not contain %q", text, want)
			}
		}
		reply, ok := sent[0].Params["reply_parameters"].(*gotgbot.ReplyParameters)
		if !ok || reply.MessageId != commandID {
			t.Errorf("choice message reply_parameters = %+v, want a reply to the command message %d", sent[0].Params["reply_parameters"], commandID)
		}

		keyboard := staffKeyboardOf(sent[0].Params["reply_markup"])
		if len(keyboard) != 1 || len(keyboard[0]) != 2 {
			t.Fatalf("choice keyboard = %+v, want one row of two buttons", keyboard)
		}
		here, private := keyboard[0][0], keyboard[0][1]
		if !strings.Contains(here.Text, staffMarker("settings_btn_open_here")) {
			t.Errorf("first button %q is not Open here", here.Text)
		}
		fields := settingsButtonFields(t, here)
		if len(fields) != 2 || fields["a"] != "oh" || fields["u"] != fmt.Sprint(env.admin.Id) {
			t.Errorf("Open here fields = %v, want exactly {a: oh, u: %d}", fields, env.admin.Id)
		}
		if !strings.Contains(private.Text, staffMarker("settings_btn_open_private")) {
			t.Errorf("second button %q is not Open in private", private.Text)
		}
		if want := settingsPrivateURL(getBotUsername(env.bot), env.chat.Id); private.Url != want || private.CallbackData != "" {
			t.Errorf("Open in private = url %q data %q, want url %q and no callback data", private.Url, private.CallbackData, want)
		}
		if !strings.HasSuffix(private.Url, "?start=settings_"+fmt.Sprint(env.chat.Id)) {
			t.Errorf("Open in private url %q does not carry settings_<chat id>", private.Url)
		}
	})

	t.Run("Open here becomes the Main page", func(t *testing.T) {
		env := newSettingsEnv(t)
		before := env.answerCount()
		msgID := env.openMenu(env.admin)

		edits := env.edits(env.chat.Id, msgID)
		if len(edits) != 1 {
			t.Fatalf("the choice message was edited %d times, want once", len(edits))
		}
		text, keyboard := env.lastEdit(env.chat.Id, msgID)
		for _, want := range []string{
			staffMarker("settings_header_main"),
			"<b>Lock Chat</b> <code>" + fmt.Sprint(env.chat.Id) + "</code>",
			staffMarker("settings_main_hint"),
			staffMarker("settings_main_owner"),
			"Ad&lt;b&gt;min",
		} {
			if !strings.Contains(text, want) {
				t.Errorf("Main page %q does not contain %q", text, want)
			}
		}
		if strings.Contains(text, "Ad<b>min") {
			t.Errorf("Main page %q carries the opener's name unescaped", text)
		}

		pages := settingsPagesInOrder()
		if len(keyboard) != len(pages)+1 {
			t.Fatalf("Main keyboard has %d rows, want one per page (%d) and the Close row", len(keyboard), len(pages))
		}
		for i, page := range pages {
			if len(keyboard[i]) != 1 {
				t.Fatalf("page row %d has %d buttons, want 1", i, len(keyboard[i]))
			}
			fields := settingsButtonFields(t, keyboard[i][0])
			if len(fields) != 3 || fields["a"] != "op" || fields["u"] != fmt.Sprint(env.admin.Id) || fields["p"] != page.Key {
				t.Errorf("page %q button fields = %v, want {a: op, u: %d, p: %s}", page.Key, fields, env.admin.Id, page.Key)
			}
		}
		last := keyboard[len(keyboard)-1]
		if len(last) != 1 || !strings.Contains(last[0].Text, staffMarker("settings_btn_close")) {
			t.Fatalf("last row = %+v, want only the Close button", last)
		}
		closeFields := settingsButtonFields(t, last[0])
		if len(closeFields) != 2 || closeFields["a"] != "cl" || closeFields["u"] != fmt.Sprint(env.admin.Id) {
			t.Errorf("Close fields = %v, want {a: cl, u: %d}", closeFields, env.admin.Id)
		}

		captchaButton, ok := settingsButtonByMarker(keyboard, "settings_page_captcha")
		if !ok {
			t.Fatal("no Captcha button on the Main page")
		}
		if !strings.Contains(captchaButton.Text, staffMarker("common_status_off")) {
			t.Errorf("Captcha button %q does not show the Off state of a fresh chat", captchaButton.Text)
		}

		if got := env.answerCount() - before; got != 1 {
			t.Fatalf("Open here answered the callback %d times, want exactly once", got)
		}
		if answer, alert := env.lastAnswer(); answer != "" || alert {
			t.Errorf("Open here answered %q (alert %v), want an empty answer", answer, alert)
		}
	})

	t.Run("Captcha page", func(t *testing.T) {
		env := newSettingsEnv(t)
		msgID := settingsOpenCaptchaPage(t, env)

		text, keyboard := env.lastEdit(env.chat.Id, msgID)
		for _, want := range []string{
			staffMarker("settings_header") + " " + staffMarker("settings_page_captcha"),
			"<b>Lock Chat</b> <code>" + fmt.Sprint(env.chat.Id) + "</code>",
			staffMarker("settings_captcha_time") + " 2",
		} {
			if !strings.Contains(text, want) {
				t.Errorf("Captcha page %q does not contain %q", text, want)
			}
		}

		stepper := settingsStepper(t, env, msgID)
		admin := fmt.Sprint(env.admin.Id)
		minus, value, plus := settingsButtonFields(t, stepper[0]), settingsButtonFields(t, stepper[1]), settingsButtonFields(t, stepper[2])
		wantMinus := map[string]string{"a": "st", "u": admin, "p": "c", "t": "t", "v": "1"}
		wantPlus := map[string]string{"a": "st", "u": admin, "p": "c", "t": "t", "v": "3"}
		wantValue := map[string]string{"a": "op", "u": admin, "p": "c"}
		for name, pair := range map[string][2]map[string]string{"minus": {minus, wantMinus}, "plus": {plus, wantPlus}, "value": {value, wantValue}} {
			if fmt.Sprint(pair[0]) != fmt.Sprint(pair[1]) {
				t.Errorf("%s button fields = %v, want %v", name, pair[0], pair[1])
			}
		}
		if !strings.Contains(stepper[1].Text, staffMarker("settings_btn_minutes")) || !strings.Contains(stepper[1].Text, "2") {
			t.Errorf("value button %q does not show 2 minutes", stepper[1].Text)
		}

		nav := keyboard[len(keyboard)-1]
		if len(nav) != 2 {
			t.Fatalf("navigation row = %+v, want [Back | Close]", nav)
		}
		back, closeButton := settingsButtonFields(t, nav[0]), settingsButtonFields(t, nav[1])
		if back["a"] != "bk" || back["u"] != admin || len(back) != 2 {
			t.Errorf("Back fields = %v, want {a: bk, u: %s}", back, admin)
		}
		if closeButton["a"] != "cl" || closeButton["u"] != admin || len(closeButton) != 2 {
			t.Errorf("Close fields = %v, want {a: cl, u: %s}", closeButton, admin)
		}
	})

	t.Run("a step writes through the shared apply function", func(t *testing.T) {
		env := newSettingsEnv(t)
		msgID := settingsOpenCaptchaPage(t, env)
		stepper := settingsStepper(t, env, msgID)
		before := env.answerCount()

		env.pressRaw(env.chat, env.admin, msgID, stepper[2].CallbackData)

		if got := settingsShownTimeout(t, env.chat.Id); got != 3 {
			t.Errorf("GetCaptchaSettings timeout = %d after +1, want 3", got)
		}
		if stored, ok := settingsStoredTimeout(t, env.chat.Id); !ok || stored != 3 {
			t.Errorf("stored timeout = %d (row %v), want 3", stored, ok)
		}
		text, _ := env.lastEdit(env.chat.Id, msgID)
		if !strings.Contains(text, staffMarker("settings_captcha_time")+" 3") {
			t.Errorf("redrawn page %q does not show 3", text)
		}
		redrawn := settingsStepper(t, env, msgID)
		if v := settingsButtonFields(t, redrawn[0])["v"]; v != "2" {
			t.Errorf("redrawn minus target = %q, want 2", v)
		}
		if v := settingsButtonFields(t, redrawn[2])["v"]; v != "4" {
			t.Errorf("redrawn plus target = %q, want 4", v)
		}
		if got := env.answerCount() - before; got != 1 {
			t.Errorf("the step answered the callback %d times, want once", got)
		}
	})

	t.Run("the command sees the menu's change", func(t *testing.T) {
		env := newSettingsEnvWith(t, settingsEnvOptions{RealLocale: true})
		msgID := settingsOpenCaptchaPage(t, env)
		stepper := settingsStepper(t, env, msgID)
		env.pressRaw(env.chat, env.admin, msgID, stepper[2].CallbackData)

		env.send(env.admin, "/captcha")
		want, err := i18n.MustNewTranslator("en").GetString("captcha_settings_timeout", i18n.TranslationParams{"d": 3})
		if err != nil || want == "" {
			t.Fatalf("real captcha_settings_timeout text: %q, %v", want, err)
		}
		env.wantReplyHas(want)
	})

	t.Run("the menu sees the command's change", func(t *testing.T) {
		env := newSettingsEnv(t)
		msgID := settingsOpenCaptchaPage(t, env)
		stepper := settingsStepper(t, env, msgID)

		env.send(env.admin, "/captchatime 7")
		env.wantReplyHas(staffMarker("captcha_timeout_set_success"))

		env.pressRaw(env.chat, env.admin, msgID, stepper[1].CallbackData)
		text, _ := env.lastEdit(env.chat.Id, msgID)
		if !strings.Contains(text, staffMarker("settings_captcha_time")+" 7") {
			t.Errorf("value button redraw %q does not show 7", text)
		}
	})

	t.Run("bounds", func(t *testing.T) {
		for _, bound := range []struct {
			name       string
			stored     string
			stepIndex  int
			wantTarget string
			toast      string
		}{
			{"upper", "10", 2, "10", "settings_max_value"},
			{"lower", "1", 0, "1", "settings_min_value"},
		} {
			t.Run(bound.name, func(t *testing.T) {
				env := newSettingsEnv(t)
				env.send(env.admin, "/captchatime "+bound.stored)
				msgID := settingsOpenCaptchaPage(t, env)
				stepper := settingsStepper(t, env, msgID)

				step := stepper[bound.stepIndex]
				if step.Text != "·" {
					t.Fatalf("step button at the bound is labelled %q, want ·", step.Text)
				}
				if v := settingsButtonFields(t, step)["v"]; v != bound.wantTarget {
					t.Fatalf("step button at the bound targets %q, want %s", v, bound.wantTarget)
				}
				var beforeRow models.CaptchaSettings
				if err := db.DB.Where("chat_id = ?", env.chat.Id).First(&beforeRow).Error; err != nil {
					t.Fatalf("read row: %v", err)
				}
				edits := len(env.edits(env.chat.Id, msgID))
				answers := env.answerCount()

				env.pressRaw(env.chat, env.admin, msgID, step.CallbackData)

				if got := env.answerCount() - answers; got != 1 {
					t.Fatalf("bound press answered %d times, want once", got)
				}
				answer, alert := env.lastAnswer()
				if alert || !strings.Contains(answer, staffMarker(bound.toast)) || !strings.Contains(answer, bound.wantTarget) {
					t.Errorf("bound answer = %q (alert %v), want the %s toast carrying %s", answer, alert, bound.toast, bound.wantTarget)
				}
				if got := len(env.edits(env.chat.Id, msgID)); got != edits {
					t.Errorf("bound press edited the menu (%d -> %d edits), want none", edits, got)
				}
				var afterRow models.CaptchaSettings
				if err := db.DB.Where("chat_id = ?", env.chat.Id).First(&afterRow).Error; err != nil {
					t.Fatalf("read row: %v", err)
				}
				if !afterRow.UpdatedAt.Equal(beforeRow.UpdatedAt) || afterRow.Timeout != beforeRow.Timeout {
					t.Errorf("bound press changed the row: %+v -> %+v", beforeRow, afterRow)
				}
			})
		}
	})

	t.Run("Back and Close", func(t *testing.T) {
		env := newSettingsEnv(t)
		msgID := settingsOpenCaptchaPage(t, env)
		_, keyboard := env.lastEdit(env.chat.Id, msgID)
		nav := keyboard[len(keyboard)-1]

		before := env.answerCount()
		env.pressRaw(env.chat, env.admin, msgID, nav[0].CallbackData)
		text, _ := env.lastEdit(env.chat.Id, msgID)
		if !strings.Contains(text, staffMarker("settings_header_main")) || !strings.Contains(text, staffMarker("settings_main_hint")) {
			t.Errorf("Back drew %q, want the Main page", text)
		}
		if got := env.answerCount() - before; got != 1 {
			t.Errorf("Back answered %d times, want once", got)
		}

		before = env.answerCount()
		env.pressRaw(env.chat, env.admin, msgID, nav[1].CallbackData)
		deletes := env.calls("deleteMessage")
		if len(deletes) != 1 || staffParamInt(deletes[0].Params, "message_id") != msgID {
			t.Fatalf("Close made deleteMessage calls %+v, want one for message %d", deletes, msgID)
		}
		if got := env.answerCount() - before; got != 1 {
			t.Errorf("Close answered %d times, want once", got)
		}
	})

	t.Run("Close falls back to an edit when the delete fails", func(t *testing.T) {
		env := newSettingsEnv(t)
		msgID := settingsOpenCaptchaPage(t, env)
		_, keyboard := env.lastEdit(env.chat.Id, msgID)
		nav := keyboard[len(keyboard)-1]
		env.fake.script("deleteMessage", env.chat.Id, staffFakeError(400, "Bad Request: message can't be deleted"))
		before := env.answerCount()

		env.pressRaw(env.chat, env.admin, msgID, nav[1].CallbackData)

		text, closed := env.lastEdit(env.chat.Id, msgID)
		if !strings.Contains(text, staffMarker("settings_closed")) {
			t.Errorf("Close fallback edited the menu to %q, want settings_closed", text)
		}
		if len(closed) != 0 {
			t.Errorf("Close fallback kept the keyboard %+v, want none", closed)
		}
		if got := env.answerCount() - before; got != 1 {
			t.Errorf("Close answered %d times, want once", got)
		}
	})
}

func TestSettingsParityCaptchaTime(t *testing.T) {
	env := newSettingsEnv(t)
	msgID := settingsOpenCaptchaPage(t, env)

	// (1) the command writes, the menu shows it.
	env.send(env.admin, "/captchatime 4")
	stepper := settingsStepper(t, env, msgID)
	env.pressRaw(env.chat, env.admin, msgID, stepper[1].CallbackData)
	text, _ := env.lastEdit(env.chat.Id, msgID)
	if !strings.Contains(text, staffMarker("settings_captcha_time")+" 4") {
		t.Errorf("after /captchatime 4 the page shows %q, want 4", text)
	}
	if stored, ok := settingsStoredTimeout(t, env.chat.Id); !ok || stored != 4 {
		t.Errorf("stored timeout = %d (row %v) after /captchatime 4, want 4", stored, ok)
	}

	// (2) the menu writes, the command shows it.
	stepper = settingsStepper(t, env, msgID)
	env.pressRaw(env.chat, env.admin, msgID, stepper[2].CallbackData)
	if got := settingsShownTimeout(t, env.chat.Id); got != 5 {
		t.Errorf("the command side reads %d after a menu step to 5", got)
	}
	// (3) one row holds the value both sides show.
	if stored, ok := settingsStoredTimeout(t, env.chat.Id); !ok || stored != 5 {
		t.Errorf("stored timeout = %d (row %v) after the menu step, want 5", stored, ok)
	}
}

func TestSettingsDriftCaptchaTime(t *testing.T) {
	for _, n := range []int{0, 1, 10, 11} {
		t.Run(fmt.Sprintf("N=%d", n), func(t *testing.T) {
			env := newSettingsEnv(t)
			commandChat := env.chat
			menuChat := env.addChat()
			valid := n >= 1 && n <= 10

			commandReplies := len(callsToChat(env.fake.staffBotClient, "sendMessage", commandChat.Id))
			env.send(env.admin, fmt.Sprintf("/captchatime %d", n))
			if !valid {
				env.wantReplyHas(staffMarker("captcha_timeout_invalid"))
			}
			if got := len(callsToChat(env.fake.staffBotClient, "sendMessage", commandChat.Id)); got != commandReplies+1 {
				t.Fatalf("the command sent %d messages, want one reply", got-commandReplies)
			}

			fields := map[string]string{"a": "st", "u": fmt.Sprint(env.admin.Id), "p": "c", "t": "t", "v": fmt.Sprint(n)}
			data := encodeCallbackData(settingsCallbackNamespace, fields)
			if data == "" {
				t.Fatalf("hand-built press %v did not encode", fields)
			}
			const menuMsgID = 4242
			env.pressRaw(menuChat, env.admin, menuMsgID, data)

			commandStored, commandHasRow := settingsStoredTimeout(t, commandChat.Id)
			menuStored, menuHasRow := settingsStoredTimeout(t, menuChat.Id)
			if commandHasRow != menuHasRow || commandStored != menuStored {
				t.Fatalf("command stored (%d, row %v), menu stored (%d, row %v): they disagree", commandStored, commandHasRow, menuStored, menuHasRow)
			}
			if valid && (!menuHasRow || menuStored != n) {
				t.Errorf("both sides stored (%d, row %v), want %d", menuStored, menuHasRow, n)
			}
			if !valid && menuHasRow {
				t.Errorf("an invalid value %d was stored (%d)", n, menuStored)
			}
			if !valid {
				text, _ := env.lastEdit(menuChat.Id, menuMsgID)
				if !strings.Contains(text, staffMarker("captcha_timeout_invalid")) {
					t.Errorf("the menu redrew %q, want a notice with the command's own captcha_timeout_invalid", text)
				}
			}
		})
	}
}

func TestSettingsCommand(t *testing.T) {
	t.Run("a plain member is refused", func(t *testing.T) {
		env := newSettingsEnv(t)
		member := gotgbot.User{Id: uniqueLinkOwnerID() + 11, FirstName: "Plain"}
		env.fake.setMember(env.chat.Id, member.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
		env.send(member, "/settings")
		env.wantReplyHas(staffMarker("chat_status_user_admin_cmd_error"))
		for _, sent := range env.replies() {
			if strings.Contains(fmt.Sprint(sent.Params["text"]), staffMarker("settings_choose")) {
				t.Errorf("a plain member got the choice message")
			}
		}
	})

	t.Run("a failed live lookup refuses", func(t *testing.T) {
		env := newSettingsEnv(t)
		env.fake.script("getChatMember", env.chat.Id, staffFakeError(500, "Internal Server Error"))
		env.send(env.admin, "/settings")
		env.wantReplyHas(staffMarker("lockdown_check_failed"))
		for _, sent := range env.replies() {
			if strings.Contains(fmt.Sprint(sent.Params["text"]), staffMarker("settings_choose")) {
				t.Errorf("an unverifiable sender got the choice message")
			}
		}
	})

	t.Run("a private chat is refused", func(t *testing.T) {
		env := newSettingsEnv(t)
		env.sendPrivate(env.admin, "/settings")
		sent := env.fake.sentTo(env.admin.Id)
		if len(sent) != 1 || !strings.Contains(fmt.Sprint(sent[0].Params["text"]), staffMarker("chat_status_group_only_error")) {
			t.Fatalf("private /settings got %+v, want the group-only refusal", sent)
		}
		if len(env.replies()) != 0 {
			t.Errorf("private /settings sent something to the group")
		}
	})

	t.Run("a basic group works", func(t *testing.T) {
		env := newSettingsEnv(t)
		env.chat.Type = "group"
		msgID := env.openMenu(env.admin)
		text, _ := env.lastEdit(env.chat.Id, msgID)
		if !strings.Contains(text, staffMarker("settings_header_main")) {
			t.Errorf("a basic group's Open here drew %q, want the Main page", text)
		}
	})

	t.Run("the admin cache is never consulted", func(t *testing.T) {
		env := newSettingsEnv(t)
		before := env.memberLookupsOf(env.chat.Id, env.admin.Id)
		env.send(env.admin, "/settings")
		if got := env.memberLookupsOf(env.chat.Id, env.admin.Id) - before; got < 1 {
			t.Errorf("/settings made %d live lookups of the sender, want at least 1", got)
		}
	})
}
