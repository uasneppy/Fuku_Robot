//go:build testtools

package modules

import (
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

// settingsPrivateChat is the private chat of from with the bot.
func settingsPrivateChat(from gotgbot.User) gotgbot.Chat {
	return gotgbot.Chat{Id: from.Id, Type: "private", FirstName: from.FirstName}
}

// settingsPrivateOnlySent returns the one message the bot sent to from's private chat, and
// fails the test when it sent anything else.
func settingsPrivateOnlySent(t *testing.T, env *settingsEnv, from gotgbot.User) staffSentMessage {
	t.Helper()
	sent := env.fake.sentTo(from.Id)
	if len(sent) != 1 {
		t.Fatalf("the bot sent %d messages to the private chat, want exactly 1: %+v", len(sent), sent)
	}
	return sent[0]
}

// settingsPrivateOpenURLStart returns the start query value of the Open in private button
// of the newest choice message in the env's chat.
func settingsPrivateOpenURLStart(t *testing.T, env *settingsEnv) string {
	t.Helper()
	sent := env.fake.sentTo(env.chat.Id)
	for i := len(sent) - 1; i >= 0; i-- {
		for _, button := range settingsAllButtons(staffKeyboardOf(sent[i].Params["reply_markup"])) {
			if button.Url == "" {
				continue
			}
			parsed, err := url.Parse(button.Url)
			if err != nil {
				t.Fatalf("Open in private url %q: %v", button.Url, err)
			}
			return parsed.Query().Get("start")
		}
	}
	t.Fatal("the choice message has no Open in private button")
	return ""
}

func TestSettingsStartDeepLink(t *testing.T) {
	t.Run("an admin gets the Main page in private for exactly that group", func(t *testing.T) {
		env := newSettingsEnv(t)
		env.sendPrivate(env.admin, fmt.Sprintf("/start settings_%d", env.chat.Id))

		msg := settingsPrivateOnlySent(t, env, env.admin)
		text := fmt.Sprint(msg.Params["text"])
		for _, want := range []string{
			staffMarker("settings_header_main"),
			"<code>" + fmt.Sprint(env.chat.Id) + "</code>",
			staffMarker("settings_main_hint"),
		} {
			if !strings.Contains(text, want) {
				t.Errorf("private Main page %q does not contain %q", text, want)
			}
		}
		if strings.Contains(text, staffMarker("settings_main_owner")) {
			t.Errorf("private Main page %q carries the group menu's owner line", text)
		}

		keyboard := staffKeyboardOf(msg.Params["reply_markup"])
		buttons := settingsAllButtons(keyboard)
		if len(buttons) == 0 {
			t.Fatal("the private Main page has no buttons")
		}
		for _, button := range buttons {
			fields := settingsButtonFields(t, button)
			if fields["c"] != fmt.Sprint(env.chat.Id) {
				t.Errorf("button %q has c = %q, want the group's chat ID %d", button.Text, fields["c"], env.chat.Id)
			}
			if _, hasU := fields["u"]; hasU {
				t.Errorf("button %q carries an opener field u in a private menu: %v", button.Text, fields)
			}
		}
		if lookups := env.memberLookupsOf(env.chat.Id, env.admin.Id); lookups < 1 {
			t.Errorf("the deep link made %d live lookups of the user in the group, want at least 1", lookups)
		}
	})

	t.Run("a basic group is accepted", func(t *testing.T) {
		env := newSettingsEnv(t)
		env.fake.setChatType(env.chat.Id, "group")
		env.sendPrivate(env.admin, fmt.Sprintf("/start settings_%d", env.chat.Id))

		msg := settingsPrivateOnlySent(t, env, env.admin)
		if !strings.Contains(fmt.Sprint(msg.Params["text"]), staffMarker("settings_header_main")) {
			t.Errorf("a basic group got %q, want the Main page", msg.Params["text"])
		}
	})

	t.Run("an invalid payload is refused before any lookup", func(t *testing.T) {
		for _, payload := range []string{
			"settings_",
			"settings_abc",
			"settings_12345",
			"settings_-",
			"settings_--1",
			"settings_-1001234567890123456789",
			"settings_-100abc",
		} {
			t.Run(payload, func(t *testing.T) {
				env := newSettingsEnv(t)
				lookups := len(env.fake.callsFor("getChatMember"))
				chats := len(env.fake.callsFor("getChat"))
				env.sendPrivate(env.admin, "/start "+payload)

				msg := settingsPrivateOnlySent(t, env, env.admin)
				if !strings.Contains(fmt.Sprint(msg.Params["text"]), staffMarker("settings_start_invalid")) {
					t.Errorf("payload %q got %q, want settings_start_invalid", payload, msg.Params["text"])
				}
				if keyboard := staffKeyboardOf(msg.Params["reply_markup"]); len(keyboard) != 0 {
					t.Errorf("payload %q got a keyboard %+v", payload, keyboard)
				}
				if got := len(env.fake.callsFor("getChatMember")) - lookups; got != 0 {
					t.Errorf("payload %q made %d getChatMember calls, want none", payload, got)
				}
				if got := len(env.fake.callsFor("getChat")) - chats; got != 0 {
					t.Errorf("payload %q made %d getChat calls, want none", payload, got)
				}
			})
		}
	})

	t.Run("a payload with a space never reaches the handler", func(t *testing.T) {
		// "/start settings_ -100" has three words, which /start treats as an unexpected
		// argument count: it sends nothing and the deep link handler never runs.
		env := newSettingsEnv(t)
		env.sendPrivate(env.admin, "/start settings_ -100")
		if sent := env.fake.sentTo(env.admin.Id); len(sent) != 0 {
			t.Errorf("a three-word /start sent %+v, want nothing", sent)
		}
		if got := len(env.fake.callsFor("getChatMember")); got != 0 {
			t.Errorf("a three-word /start made %d getChatMember calls, want none", got)
		}
	})

	t.Run("a plain member is refused", func(t *testing.T) {
		env := newSettingsEnv(t)
		member := gotgbot.User{Id: uniqueLinkOwnerID() + 21, FirstName: "Plain"}
		env.fake.setMember(env.chat.Id, member.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
		env.sendPrivate(member, fmt.Sprintf("/start settings_%d", env.chat.Id))

		msg := settingsPrivateOnlySent(t, env, member)
		if !strings.Contains(fmt.Sprint(msg.Params["text"]), staffMarker("settings_start_not_admin")) {
			t.Errorf("a plain member got %q, want settings_start_not_admin", msg.Params["text"])
		}
		if keyboard := staffKeyboardOf(msg.Params["reply_markup"]); len(keyboard) != 0 {
			t.Errorf("a refused member got a keyboard %+v", keyboard)
		}
	})

	t.Run("the refusals", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			script func(env *settingsEnv)
			want   string
		}{
			{
				name: "a failed member lookup",
				script: func(env *settingsEnv) {
					env.fake.script("getChatMember", env.chat.Id, staffFakeError(500, "Internal Server Error"))
				},
				want: "settings_check_failed",
			},
			{
				name: "the bot is not a member of the group",
				script: func(env *settingsEnv) {
					env.fake.script("getChatMember", env.chat.Id, staffFakeError(403, "Forbidden: bot is not a member of the supergroup chat"))
				},
				want: "settings_cannot_open",
			},
			{
				name: "the group is gone at getChat",
				script: func(env *settingsEnv) {
					env.fake.script("getChat", env.chat.Id, staffFakeError(400, "Bad Request: chat not found"))
				},
				want: "settings_cannot_open",
			},
			{
				name: "a failed getChat",
				script: func(env *settingsEnv) {
					env.fake.script("getChat", env.chat.Id, staffFakeError(500, "Internal Server Error"))
				},
				want: "settings_check_failed",
			},
			{
				name: "a channel",
				script: func(env *settingsEnv) {
					env.fake.setChatType(env.chat.Id, "channel")
				},
				want: "settings_cannot_open",
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				env := newSettingsEnv(t)
				tc.script(env)
				env.sendPrivate(env.admin, fmt.Sprintf("/start settings_%d", env.chat.Id))

				msg := settingsPrivateOnlySent(t, env, env.admin)
				if !strings.Contains(fmt.Sprint(msg.Params["text"]), staffMarker(tc.want)) {
					t.Errorf("got %q, want %s", msg.Params["text"], tc.want)
				}
				if keyboard := staffKeyboardOf(msg.Params["reply_markup"]); len(keyboard) != 0 {
					t.Errorf("a refusal sent a keyboard %+v", keyboard)
				}
			})
		}
	})
}

func TestSettingsPrivateRoundTrip(t *testing.T) {
	env := newSettingsEnv(t)
	env.send(env.admin, "/settings")

	start := settingsPrivateOpenURLStart(t, env)
	if start != fmt.Sprintf("settings_%d", env.chat.Id) {
		t.Fatalf("the Open in private button carries start=%q, want settings_%d", start, env.chat.Id)
	}
	env.sendPrivate(env.admin, "/start "+start)

	main := settingsPrivateOnlySent(t, env, env.admin)
	if !strings.Contains(fmt.Sprint(main.Params["text"]), staffMarker("settings_header_main")) {
		t.Fatalf("the deep link sent %q, want the Main page", main.Params["text"])
	}
	button, ok := settingsPageButton(t, staffKeyboardOf(main.Params["reply_markup"]), settingsPageCaptcha)
	if !ok {
		t.Fatal("the private Main page has no Captcha button")
	}

	private := settingsPrivateChat(env.admin)
	env.pressRaw(private, env.admin, main.MessageID, button.CallbackData)
	text, keyboard := env.lastEdit(env.admin.Id, main.MessageID)
	if !strings.Contains(text, staffMarker("settings_page_captcha")) {
		t.Fatalf("pressing Captcha in private drew %q, want the Captcha page", text)
	}
	if len(keyboard) != 2 || len(keyboard[0]) != 3 {
		t.Fatalf("private Captcha page keyboard = %+v, want the stepper row and the navigation row", keyboard)
	}

	env.pressRaw(private, env.admin, main.MessageID, keyboard[0][2].CallbackData)
	if got := settingsShownTimeout(t, env.chat.Id); got != 3 {
		t.Errorf("the group's captcha time = %d after a private step, want 3", got)
	}
	if stored, ok := settingsStoredTimeout(t, env.chat.Id); !ok || stored != 3 {
		t.Errorf("stored timeout of the group = %d (row %v), want 3", stored, ok)
	}
	if _, ok := settingsStoredTimeout(t, env.admin.Id); ok {
		t.Errorf("a private menu step wrote captcha settings for the private chat %d", env.admin.Id)
	}
}

// settingsPrivateOpen opens the private Main page of the env's group through the deep link
// and returns the ID of the message.
func settingsPrivateOpen(t *testing.T, env *settingsEnv) int64 {
	t.Helper()
	env.sendPrivate(env.admin, fmt.Sprintf("/start settings_%d", env.chat.Id))
	return settingsPrivateOnlySent(t, env, env.admin).MessageID
}

func TestSettingsPrivateMenu(t *testing.T) {
	captchaPage := func(env *settingsEnv) map[string]string {
		return env.privateFields(env.chat.Id, "a", "op", "p", settingsPageCaptcha)
	}

	t.Run("a bot that was kicked closes the menu with the cannot-open note", func(t *testing.T) {
		env := newSettingsEnv(t)
		msgID := settingsPrivateOpen(t, env)
		env.fake.script("getChatMember", env.chat.Id, staffFakeError(403, "Forbidden: bot was kicked from the supergroup chat"))
		answers := env.answerCount()

		env.pressPrivate(env.admin, msgID, captchaPage(env))

		if got := env.answerCount() - answers; got != 1 {
			t.Fatalf("the press answered the callback %d times, want once", got)
		}
		if text, alert := env.lastAnswer(); !alert || !strings.Contains(text, staffMarker("settings_cannot_open_alert")) {
			t.Errorf("answer = %q (alert %v), want the settings_cannot_open_alert alert", text, alert)
		}
		if got := len(env.edits(env.admin.Id, msgID)); got != 1 {
			t.Fatalf("the menu was edited %d times, want once", got)
		}
		text, keyboard := env.lastEdit(env.admin.Id, msgID)
		if !strings.Contains(text, staffMarker("settings_cannot_open")) || len(keyboard) != 0 {
			t.Errorf("edit = %q with keyboard %+v, want settings_cannot_open and no keyboard", text, keyboard)
		}
	})

	t.Run("a group that is gone closes the menu the same way", func(t *testing.T) {
		env := newSettingsEnv(t)
		msgID := settingsPrivateOpen(t, env)
		env.fake.script("getChat", env.chat.Id, staffFakeError(400, "Bad Request: chat not found"))
		answers := env.answerCount()

		env.pressPrivate(env.admin, msgID, captchaPage(env))

		if got := env.answerCount() - answers; got != 1 {
			t.Fatalf("the press answered the callback %d times, want once", got)
		}
		if text, alert := env.lastAnswer(); !alert || !strings.Contains(text, staffMarker("settings_cannot_open_alert")) {
			t.Errorf("answer = %q (alert %v), want the settings_cannot_open_alert alert", text, alert)
		}
		if got := len(env.edits(env.admin.Id, msgID)); got != 1 {
			t.Fatalf("the menu was edited %d times, want once", got)
		}
		text, keyboard := env.lastEdit(env.admin.Id, msgID)
		if !strings.Contains(text, staffMarker("settings_cannot_open")) || len(keyboard) != 0 {
			t.Errorf("edit = %q with keyboard %+v, want settings_cannot_open and no keyboard", text, keyboard)
		}
	})

	t.Run("any other lookup error is a plain alert and edits nothing", func(t *testing.T) {
		for name, script := range map[string]func(env *settingsEnv){
			"getChatMember": func(env *settingsEnv) {
				env.fake.script("getChatMember", env.chat.Id, staffFakeError(500, "Internal Server Error"))
			},
			"getChat": func(env *settingsEnv) {
				env.fake.script("getChat", env.chat.Id, staffFakeError(500, "Internal Server Error"))
			},
		} {
			t.Run(name, func(t *testing.T) {
				env := newSettingsEnv(t)
				msgID := settingsPrivateOpen(t, env)
				script(env)
				answers := env.answerCount()

				env.pressPrivate(env.admin, msgID, captchaPage(env))

				if got := env.answerCount() - answers; got != 1 {
					t.Fatalf("the press answered the callback %d times, want once", got)
				}
				if text, alert := env.lastAnswer(); !alert || !strings.Contains(text, staffMarker("settings_check_failed")) {
					t.Errorf("answer = %q (alert %v), want the settings_check_failed alert", text, alert)
				}
				if got := len(env.edits(env.admin.Id, msgID)); got != 0 {
					t.Errorf("a failed lookup edited the menu %d times, want none", got)
				}
			})
		}
	})

	t.Run("a plain member's press is refused", func(t *testing.T) {
		env := newSettingsEnv(t)
		member := gotgbot.User{Id: uniqueLinkOwnerID() + 31, FirstName: "Plain"}
		env.fake.setMember(env.chat.Id, member.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
		const menuMsgID = 4343
		answers := env.answerCount()

		env.pressPrivate(member, menuMsgID, captchaPage(env))

		if got := env.answerCount() - answers; got != 1 {
			t.Fatalf("the press answered the callback %d times, want once", got)
		}
		if text, alert := env.lastAnswer(); !alert || !strings.Contains(text, staffMarker("settings_not_admin")) {
			t.Errorf("answer = %q (alert %v), want the settings_not_admin alert", text, alert)
		}
		if got := len(env.edits(member.Id, menuMsgID)); got != 0 {
			t.Errorf("a refused press edited the menu %d times, want none", got)
		}
	})

	t.Run("a step writes only the group's settings", func(t *testing.T) {
		env := newSettingsEnv(t)
		msgID := settingsPrivateOpen(t, env)
		env.pressPrivate(env.admin, msgID, captchaPage(env))
		_, keyboard := env.lastEdit(env.admin.Id, msgID)
		if len(keyboard) != 2 || len(keyboard[0]) != 3 {
			t.Fatalf("private Captcha page keyboard = %+v, want the stepper row and the navigation row", keyboard)
		}

		env.pressRaw(settingsPrivateChat(env.admin), env.admin, msgID, keyboard[0][2].CallbackData)

		if stored, ok := settingsStoredTimeout(t, env.chat.Id); !ok || stored != 3 {
			t.Errorf("stored timeout of the group = %d (row %v), want 3", stored, ok)
		}
		if _, ok := settingsStoredTimeout(t, env.admin.Id); ok {
			t.Errorf("a step wrote captcha settings for the presser's private chat %d", env.admin.Id)
		}
	})
}
