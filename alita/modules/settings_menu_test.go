//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/lang"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/i18n"
)

// settingsMenuMark is a snapshot of the calls a press can make, taken before it.
type settingsMenuMark struct {
	answers int
	edits   int
	members int
	chats   int
}

func settingsMenuMarkOf(env *settingsEnv) settingsMenuMark {
	return settingsMenuMark{
		answers: env.answerCount(),
		edits:   len(env.fake.callsFor("editMessageText")),
		members: len(env.fake.callsFor("getChatMember")),
		chats:   len(env.fake.callsFor("getChat")),
	}
}

// settingsMenuWantRefused fails unless the press made since before was answered exactly
// once with the alert of the locale key, edited nothing and, when noLookup is set, made
// no getChatMember and no getChat call.
func settingsMenuWantRefused(t *testing.T, env *settingsEnv, before settingsMenuMark, key string, noLookup bool) {
	t.Helper()
	after := settingsMenuMarkOf(env)
	if got := after.answers - before.answers; got != 1 {
		t.Fatalf("the press was answered %d times, want exactly once", got)
	}
	answer, alert := env.lastAnswer()
	if !alert || !strings.Contains(answer, staffMarker(key)) {
		t.Errorf("answer = %q (alert %v), want the alert %s", answer, alert, key)
	}
	if after.edits != before.edits {
		t.Errorf("a refused press edited %d messages, want none", after.edits-before.edits)
	}
	if noLookup && (after.members != before.members || after.chats != before.chats) {
		t.Errorf("a refused press made %d getChatMember and %d getChat calls, want none",
			after.members-before.members, after.chats-before.chats)
	}
}

// settingsMenuCaptchaMenu opens the menu on the Captcha page and returns its message ID
// and the data of its +1 button.
func settingsMenuCaptchaMenu(t *testing.T, env *settingsEnv) (int64, string) {
	t.Helper()
	msgID := settingsOpenCaptchaPage(t, env)
	stepper := settingsStepper(t, env, msgID)
	return msgID, stepper[2].CallbackData
}

func TestSettingsOwnership(t *testing.T) {
	t.Run("another administrator is not the opener", func(t *testing.T) {
		env := newSettingsEnv(t)
		msgID, plus := settingsMenuCaptchaMenu(t, env)
		other := gotgbot.User{Id: env.admin.Id + 1, FirstName: "Other"}
		env.fake.setMember(env.chat.Id, other.Id, staffFakeMember{
			Status: gotgbot.ChatMemberStatusAdministrator, CanRestrictMembers: true,
		})
		lookups := env.memberLookupsOf(env.chat.Id, other.Id)
		before := settingsMenuMarkOf(env)

		env.pressRaw(env.chat, other, msgID, plus)

		settingsMenuWantRefused(t, env, before, "settings_not_your_menu", false)
		if got := env.memberLookupsOf(env.chat.Id, other.Id); got != lookups {
			t.Errorf("the presser was looked up %d times, want none before the opener check", got-lookups)
		}
		if got := settingsShownTimeout(t, env.chat.Id); got != 2 {
			t.Errorf("stored timeout = %d after a foreign press, want the default 2", got)
		}
	})

	t.Run("forged structure", func(t *testing.T) {
		env := newSettingsEnv(t)
		msgID, _ := settingsMenuCaptchaMenu(t, env)
		admin := fmt.Sprint(env.admin.Id)
		privateChat := gotgbot.Chat{Id: env.admin.Id, Type: "private", FirstName: env.admin.FirstName}

		cases := []struct {
			name   string
			inChat gotgbot.Chat
			fields map[string]string
		}{
			{"c on a group message", env.chat, map[string]string{"a": "bk", "c": fmt.Sprint(env.chat.Id)}},
			{"u on a private message", privateChat, map[string]string{"a": "bk", "u": admin}},
			{"both c and u", env.chat, map[string]string{"a": "bk", "c": fmt.Sprint(env.chat.Id), "u": admin}},
			{"neither c nor u", env.chat, map[string]string{"a": "bk"}},
			{"an unknown key", env.chat, map[string]string{"a": "bk", "u": admin, "x": "1"}},
			{"a non-numeric c", privateChat, map[string]string{"a": "bk", "c": "abc"}},
			{"a positive c", privateChat, map[string]string{"a": "bk", "c": "1234"}},
			{"a zero u", env.chat, map[string]string{"a": "bk", "u": "0"}},
			{"a negative u", env.chat, map[string]string{"a": "bk", "u": "-5"}},
			{"an unknown action", env.chat, map[string]string{"a": "zz", "u": admin}},
			{"a step without its fields", env.chat, map[string]string{"a": "st", "u": admin, "p": "c"}},
			{"a field the action does not take", env.chat, map[string]string{"a": "cl", "u": admin, "p": "c"}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				data := encodeCallbackData(settingsCallbackNamespace, tc.fields)
				if data == "" {
					t.Fatalf("fields %v did not encode", tc.fields)
				}
				before := settingsMenuMarkOf(env)
				env.pressRaw(tc.inChat, env.admin, msgID, data)
				settingsMenuWantRefused(t, env, before, "common_callback_invalid_request", true)
			})
		}

		t.Run("data of another namespace", func(t *testing.T) {
			before := settingsMenuMarkOf(env)
			env.pressRaw(env.chat, env.admin, msgID, "garbage")
			after := settingsMenuMarkOf(env)
			if after.edits != before.edits || after.members != before.members || after.chats != before.chats {
				t.Errorf("undecodable data made a call: %+v -> %+v", before, after)
			}
		})
	})

	t.Run("a forwarded menu is judged in the chat it sits in", func(t *testing.T) {
		env := newSettingsEnv(t)
		_, plus := settingsMenuCaptchaMenu(t, env)
		second := env.addChat()
		env.fake.setMember(second.Id, env.admin.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
		before := settingsMenuMarkOf(env)

		env.pressRaw(second, env.admin, 777, plus)

		settingsMenuWantRefused(t, env, before, "settings_not_admin", false)
		if got := env.memberLookupsOf(second.Id, env.admin.Id); got != 1 {
			t.Errorf("the second group looked the presser up %d times, want once", got)
		}
		if got := settingsShownTimeout(t, second.Id); got != 2 {
			t.Errorf("the second group's timeout = %d, want untouched 2", got)
		}
	})

	t.Run("a private press", func(t *testing.T) {
		env := newSettingsEnv(t)
		members := env.memberLookupsOf(env.chat.Id, env.admin.Id)
		chats := len(callsToChat(env.fake.staffBotClient, "getChat", env.chat.Id))

		const privateMsg = 5555
		env.pressPrivate(env.admin, privateMsg, env.privateFields(env.chat.Id, "a", "op", "p", "c"))

		if got := env.memberLookupsOf(env.chat.Id, env.admin.Id) - members; got != 1 {
			t.Errorf("the private press looked the presser up %d times, want once", got)
		}
		if got := len(callsToChat(env.fake.staffBotClient, "getChat", env.chat.Id)) - chats; got != 1 {
			t.Errorf("the private press made %d getChat calls, want once", got)
		}
		text, _ := env.lastEdit(env.admin.Id, privateMsg)
		if !strings.Contains(text, staffMarker("settings_captcha_time")) {
			t.Errorf("the private message was edited to %q, want the Captcha page", text)
		}
		if strings.Contains(text, staffMarker("settings_main_owner")) {
			t.Errorf("a private menu names an opener: %q", text)
		}
	})

	t.Run("a press with no message", func(t *testing.T) {
		env := newSettingsEnv(t)
		data := encodeCallbackData(settingsCallbackNamespace, env.groupFields(env.admin, "a", "bk"))
		before := settingsMenuMarkOf(env)
		env.pressNoMessage(env.admin, data)
		settingsMenuWantRefused(t, env, before, "common_callback_message_unavailable", true)
	})
}

func TestSettingsPressPipeline(t *testing.T) {
	t.Run("service identities", func(t *testing.T) {
		env := newSettingsEnv(t)
		msgID, _ := settingsMenuCaptchaMenu(t, env)
		for _, id := range []int64{777000, 1087968824, 136817688} {
			service := gotgbot.User{Id: id, FirstName: "Service"}
			data := encodeCallbackData(settingsCallbackNamespace, map[string]string{"a": "bk", "u": fmt.Sprint(id)})
			before := settingsMenuMarkOf(env)
			env.pressRaw(env.chat, service, msgID, data)
			settingsMenuWantRefused(t, env, before, "lockdown_alert_post_as_yourself", true)
		}
	})

	t.Run("a demoted opener", func(t *testing.T) {
		env := newSettingsEnv(t)
		msgID, plus := settingsMenuCaptchaMenu(t, env)
		env.fake.setMember(env.chat.Id, env.admin.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
		before := settingsMenuMarkOf(env)

		env.pressRaw(env.chat, env.admin, msgID, plus)

		settingsMenuWantRefused(t, env, before, "settings_not_admin", false)
		if got := settingsShownTimeout(t, env.chat.Id); got != 2 {
			t.Errorf("a demoted opener changed the time limit to %d", got)
		}
	})

	t.Run("an unverifiable opener", func(t *testing.T) {
		env := newSettingsEnv(t)
		msgID, plus := settingsMenuCaptchaMenu(t, env)
		env.fake.script("getChatMember", env.chat.Id, staffFakeError(500, "Internal Server Error"))
		before := settingsMenuMarkOf(env)

		env.pressRaw(env.chat, env.admin, msgID, plus)

		settingsMenuWantRefused(t, env, before, "settings_check_failed", false)
		if _, ok := settingsStoredTimeout(t, env.chat.Id); ok {
			t.Error("a press whose rights could not be checked wrote a row")
		}
	})

	t.Run("every press is answered once", func(t *testing.T) {
		env := newSettingsEnv(t)
		msgID, plus := settingsMenuCaptchaMenu(t, env)
		_, keyboard := env.lastEdit(env.chat.Id, msgID)
		for _, button := range settingsAllButtons(keyboard) {
			before := env.answerCount()
			env.pressRaw(env.chat, env.admin, msgID, button.CallbackData)
			if got := env.answerCount() - before; got != 1 {
				t.Errorf("button %q answered %d times, want once", button.Text, got)
			}
			// Put the Captcha page back for the next button.
			_, current := env.lastEdit(env.chat.Id, msgID)
			if _, onPage := settingsButtonByMarker(current, "settings_btn_minutes"); !onPage {
				env.pressRaw(env.chat, env.admin, msgID, settingsMenuCaptchaButton(t, current))
			}
		}
		before := env.answerCount()
		env.pressRaw(env.chat, env.admin, msgID, plus)
		if got := env.answerCount() - before; got != 1 {
			t.Errorf("a step answered %d times, want once", got)
		}
	})
}

// settingsMenuCaptchaButton returns the data of the Captcha button on a Main page.
func settingsMenuCaptchaButton(t *testing.T, keyboard [][]gotgbot.InlineKeyboardButton) string {
	t.Helper()
	button, ok := settingsButtonByMarker(keyboard, "settings_page_captcha")
	if !ok {
		t.Fatal("no Captcha button on the page")
	}
	return button.CallbackData
}

func TestSettingsRights(t *testing.T) {
	env := newSettingsEnv(t)
	pressFor := func(chat gotgbot.Chat, member gotgbot.MergedChatMember) *settingsPress {
		return &settingsPress{b: env.bot, group: &chat, member: member}
	}

	t.Run("settingsRightAdmin allows a live administrator", func(t *testing.T) {
		chat := env.addChat()
		member, err := lockdownLiveMember(env.bot, chat.Id, env.admin.Id)
		if err != nil {
			t.Fatalf("live member: %v", err)
		}
		if key, _ := settingsRightRefusal(pressFor(chat, member), settingsRightAdmin); key != "" {
			t.Errorf("settingsRightAdmin refused an administrator with %q", key)
		}
	})

	t.Run("settingsRightAdminBotAdmin mirrors /captcha", func(t *testing.T) {
		member, _ := lockdownLiveMember(env.bot, env.chat.Id, env.admin.Id)
		for _, tc := range []struct {
			name    string
			role    string
			refused bool
		}{
			{"bot is an administrator", staffRoleAdmin, false},
			{"bot is a member", staffRoleMember, true},
		} {
			t.Run(tc.name, func(t *testing.T) {
				chat := env.addChat()
				env.fake.setBotRole(chat.Id, tc.role)
				key, alert := settingsRightRefusal(pressFor(chat, member), settingsRightAdminBotAdmin)
				env.sendIn(chat, env.admin, "/captcha on")
				commandRefused := false
				for _, sent := range env.fake.sentTo(chat.Id) {
					if strings.Contains(fmt.Sprint(sent.Params["text"]), staffMarker("chat_status_bot_not_admin")) {
						commandRefused = true
					}
				}
				if tc.refused {
					if key != "chat_status_bot_not_admin" || alert {
						t.Errorf("menu refusal = (%q, alert %v), want the notice chat_status_bot_not_admin", key, alert)
					}
				} else if key != "" {
					t.Errorf("menu refused with %q although the bot is an administrator", key)
				}
				if commandRefused != tc.refused {
					t.Errorf("/captcha on refused = %v, the menu refused = %v: they disagree", commandRefused, tc.refused)
				}
			})
		}
	})

	t.Run("settingsRightAdminFloodBot mirrors /setflood", func(t *testing.T) {
		member, _ := lockdownLiveMember(env.bot, env.chat.Id, env.admin.Id)
		for _, tc := range []struct {
			name    string
			role    string
			refused bool
		}{
			{"bot is an administrator", staffRoleAdmin, false},
			{"bot is a member", staffRoleMember, true},
		} {
			t.Run(tc.name, func(t *testing.T) {
				chat := env.addChat()
				env.fake.setBotRole(chat.Id, tc.role)
				key, alert := settingsRightRefusal(pressFor(chat, member), settingsRightAdminFloodBot)
				env.sendIn(chat, env.admin, "/setflood 5")
				commandRefused := false
				for _, sent := range env.fake.sentTo(chat.Id) {
					if strings.Contains(fmt.Sprint(sent.Params["text"]), staffMarker("connections_is_user_connected_bot_not_admin")) {
						commandRefused = true
					}
				}
				if tc.refused {
					if key != "connections_is_user_connected_bot_not_admin" || alert {
						t.Errorf("menu refusal = (%q, alert %v), want the notice connections_is_user_connected_bot_not_admin", key, alert)
					}
				} else if key != "" {
					t.Errorf("menu refused with %q although the bot is an administrator", key)
				}
				if commandRefused != tc.refused {
					t.Errorf("/setflood 5 refused = %v, the menu refused = %v: they disagree", commandRefused, tc.refused)
				}
			})
		}
	})

	t.Run("settingsRightRestrict mirrors /autolockdown", func(t *testing.T) {
		chat := env.addChat()
		noRestrict := gotgbot.User{Id: env.admin.Id + 21, FirstName: "NoRestrict"}
		withRestrict := gotgbot.User{Id: env.admin.Id + 22, FirstName: "WithRestrict"}
		creator := gotgbot.User{Id: env.admin.Id + 23, FirstName: "Creator"}
		plain := gotgbot.User{Id: env.admin.Id + 24, FirstName: "Plain"}
		env.fake.setMember(chat.Id, noRestrict.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusAdministrator})
		env.fake.setMember(chat.Id, withRestrict.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusAdministrator, CanRestrictMembers: true})
		env.fake.setMember(chat.Id, creator.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusCreator})
		env.fake.setMember(chat.Id, plain.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})

		for _, tc := range []struct {
			name      string
			user      gotgbot.User
			wantKey   string
			wantAlert bool
		}{
			{"an administrator without the right", noRestrict, "settings_no_restrict", true},
			{"an administrator with the right", withRestrict, "", false},
			{"the creator", creator, "", false},
			{"a plain member", plain, "settings_not_admin", true},
		} {
			t.Run(tc.name, func(t *testing.T) {
				member, err := lockdownLiveMember(env.bot, chat.Id, tc.user.Id)
				if err != nil {
					t.Fatalf("live member: %v", err)
				}
				key, alert := settingsRightRefusal(pressFor(chat, member), settingsRightRestrict)
				if key != tc.wantKey || alert != tc.wantAlert {
					t.Errorf("refusal = (%q, alert %v), want (%q, alert %v)", key, alert, tc.wantKey, tc.wantAlert)
				}
			})
		}

		// The command refuses the same person and writes nothing.
		before := len(env.fake.sentTo(chat.Id))
		env.sendIn(chat, noRestrict, "/autolockdown joins 20")
		sent := env.fake.sentTo(chat.Id)
		if len(sent) != before+1 || !strings.Contains(fmt.Sprint(sent[len(sent)-1].Params["text"]), staffMarker("chat_status_restrict_cmd_error")) {
			t.Fatalf("/autolockdown by an administrator without the right got %+v, want chat_status_restrict_cmd_error", sent)
		}
		var rows []models.ChatRaidDetection
		if err := db.DB.Where("chat_id = ?", chat.Id).Find(&rows).Error; err != nil || len(rows) != 0 {
			t.Errorf("the refused command wrote %d raid detection rows (err %v), want none", len(rows), err)
		}
	})
}

func TestSettingsNavigation(t *testing.T) {
	t.Run("Back draws the Main page", func(t *testing.T) {
		env := newSettingsEnv(t)
		msgID := settingsOpenCaptchaPage(t, env)
		_, keyboard := env.lastEdit(env.chat.Id, msgID)
		back, ok := settingsButtonByMarker(keyboard, "settings_btn_back")
		if !ok {
			t.Fatal("no Back button on the Captcha page")
		}
		env.pressRaw(env.chat, env.admin, msgID, back.CallbackData)
		text, _ := env.lastEdit(env.chat.Id, msgID)
		if !strings.Contains(text, staffMarker("settings_header_main")) {
			t.Errorf("Back drew %q, want the Main page", text)
		}
	})

	t.Run("not modified is success", func(t *testing.T) {
		env := newSettingsEnv(t)
		msgID := settingsOpenCaptchaPage(t, env)
		_, keyboard := env.lastEdit(env.chat.Id, msgID)
		back, _ := settingsButtonByMarker(keyboard, "settings_btn_back")
		env.pressRaw(env.chat, env.admin, msgID, back.CallbackData)

		env.fake.script("editMessageText", env.chat.Id, staffFakeError(400, "Bad Request: message is not modified"))
		answers := env.answerCount()
		edits := len(env.edits(env.chat.Id, msgID))
		env.pressRaw(env.chat, env.admin, msgID, back.CallbackData)

		if got := env.answerCount() - answers; got != 1 {
			t.Errorf("the repeated press was answered %d times, want once", got)
		}
		if answer, alert := env.lastAnswer(); answer != "" || alert {
			t.Errorf("the repeated press answered %q (alert %v), want an empty answer", answer, alert)
		}
		if got := len(env.edits(env.chat.Id, msgID)) - edits; got != 1 {
			t.Errorf("the repeated press made %d edits, want one", got)
		}
	})

	t.Run("a rate limited edit is not retried and keeps the write", func(t *testing.T) {
		env := newSettingsEnv(t)
		msgID, plus := settingsMenuCaptchaMenu(t, env)
		env.fake.script("editMessageText", env.chat.Id, staffFake429(1))
		edits := len(env.edits(env.chat.Id, msgID))
		answers := env.answerCount()

		env.pressRaw(env.chat, env.admin, msgID, plus)

		if got := len(env.edits(env.chat.Id, msgID)) - edits; got != 1 {
			t.Errorf("the press made %d edit attempts, want exactly one", got)
		}
		if got := env.answerCount() - answers; got != 1 {
			t.Errorf("the press was answered %d times, want once", got)
		}
		if stored, ok := settingsStoredTimeout(t, env.chat.Id); !ok || stored != 3 {
			t.Errorf("stored timeout = %d (row %v), want the written 3", stored, ok)
		}
	})
}

func TestSettingsCallbackBytes(t *testing.T) {
	tr := i18n.MustNewTranslator("en")
	check := func(t *testing.T, name string, screen settingsScreen) {
		t.Helper()
		if screen.Omitted != 0 {
			t.Errorf("%s omitted %d buttons for overflow, want none", name, screen.Omitted)
		}
		buttons := settingsAllButtons(screen.Keyboard.InlineKeyboard)
		if len(buttons) == 0 {
			t.Errorf("%s has no buttons", name)
		}
		for _, button := range buttons {
			if button.Url != "" {
				continue
			}
			if n := len(button.CallbackData); n < 1 || n > 64 {
				t.Errorf("%s button %q has %d bytes of callback data, want 1..64", name, button.Text, n)
			}
		}
	}

	entries := []settingsMainEntry{
		{Key: "s", Icon: "👥", Title: "Staff Group", State: "Not linked", Known: true},
		{Key: "l", Icon: "🔒", Title: "Lockdown", State: "Not active", Known: true},
		{Key: "c", Icon: "🧩", Title: "Captcha", State: "On", Known: true},
		{Key: "f", Icon: "🌊", Title: "Antiflood", State: "Limit 100", Known: false},
	}
	for _, variant := range []struct {
		name string
		view settingsView
	}{
		{"private", settingsView{Tr: tr, Private: true, OwnerKey: "c", OwnerValue: "-1001234567890", GroupID: -1001234567890, GroupTitle: "G", OpenerName: "N"}},
		{"group", settingsView{Tr: tr, OwnerKey: "u", OwnerValue: "9999999999", GroupID: -1001234567890, GroupTitle: "G", OpenerName: "N"}},
	} {
		t.Run(variant.name, func(t *testing.T) {
			check(t, "Main", renderSettingsMain(variant.view, entries, ""))
			for _, minutes := range []int{1, 5, 10} {
				check(t, fmt.Sprintf("Captcha %d", minutes), renderSettingsCaptcha(variant.view, &models.CaptchaSettings{Timeout: minutes}, settingsCaptchaWeb{}, ""))
			}
		})
	}

	t.Run("choice message", func(t *testing.T) {
		chat := gotgbot.Chat{Id: -1001234567890, Type: "supergroup", Title: "G"}
		screen := settingsChoiceScreen(tr, "AlitaTestBot", &chat, 9999999999)
		check(t, "choice", screen)
		buttons := settingsAllButtons(screen.Keyboard.InlineKeyboard)
		var urlButton *gotgbot.InlineKeyboardButton
		for i := range buttons {
			if buttons[i].Url != "" {
				urlButton = &buttons[i]
			}
		}
		if urlButton == nil {
			t.Fatal("the choice message has no URL button")
		}
		_, payload, found := strings.Cut(urlButton.Url, "?start=")
		if !found || payload == "" || len(payload) > 64 {
			t.Fatalf("start payload %q is empty or longer than 64 characters", payload)
		}
		for _, r := range payload {
			if !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
				t.Errorf("start payload %q holds %q, outside [A-Za-z0-9_-]", payload, r)
			}
		}
	})
}

func TestSettingsKeyboardHelpers(t *testing.T) {
	view := settingsView{Tr: i18n.MustNewTranslator("en"), OwnerKey: "u", OwnerValue: "42", GroupID: -1001, GroupTitle: "G"}
	steps := []int{-10, -1, 1, 10}
	clamp := func(v int) int { return min(max(v, 3), 500) }

	t.Run("stepper bounds", func(t *testing.T) {
		for _, tc := range []struct {
			current int
			labels  []string
			targets []string
		}{
			{3, []string{"·", "·", "+1", "+10"}, []string{"3", "3", "4", "13"}},
			{4, []string{"−10", "−1", "+1", "+10"}, []string{"3", "3", "5", "14"}},
			{495, []string{"−10", "−1", "+1", "+10"}, []string{"485", "494", "496", "500"}},
			{500, []string{"−10", "−1", "·", "·"}, []string{"490", "499", "500", "500"}},
		} {
			k := newSettingsKeyboard(view)
			row := settingsStepperRow(k, "l", "j", tc.current, steps, clamp, fmt.Sprint(tc.current))
			if len(row) != 5 {
				t.Fatalf("stepper at %d has %d buttons, want 5", tc.current, len(row))
			}
			stepButtons := []gotgbot.InlineKeyboardButton{row[0], row[1], row[3], row[4]}
			for i, button := range stepButtons {
				if button.Text != tc.labels[i] {
					t.Errorf("at %d step %d label = %q, want %q", tc.current, i, button.Text, tc.labels[i])
				}
				fields := settingsButtonFields(t, button)
				if fields["v"] != tc.targets[i] || fields["a"] != "st" || fields["p"] != "l" || fields["t"] != "j" || fields["u"] != "42" {
					t.Errorf("at %d step %d fields = %v, want target %s", tc.current, i, fields, tc.targets[i])
				}
			}
			value := settingsButtonFields(t, row[2])
			if value["a"] != "op" || value["p"] != "l" || value["v"] != "" {
				t.Errorf("value button fields = %v, want {a: op, p: l, u}", value)
			}
			if k.omitted != 0 {
				t.Errorf("stepper omitted %d buttons", k.omitted)
			}
		}
	})

	t.Run("choice row", func(t *testing.T) {
		k := newSettingsKeyboard(view)
		row := settingsChoiceRow(k, "c", "m", []settingsOption{{"math", "Math"}, {"text", "Text"}, {"turnstile", "Turnstile"}}, "text")
		if len(row) != 3 {
			t.Fatalf("choice row has %d buttons, want 3", len(row))
		}
		for i, want := range []string{"Math", "✅ Text", "Turnstile"} {
			if row[i].Text != want {
				t.Errorf("option %d label = %q, want %q", i, row[i].Text, want)
			}
		}
		fields := settingsButtonFields(t, row[1])
		if fields["a"] != "st" || fields["p"] != "c" || fields["t"] != "m" || fields["v"] != "text" {
			t.Errorf("choice fields = %v", fields)
		}
	})

	t.Run("toggle sends the absolute target", func(t *testing.T) {
		k := newSettingsKeyboard(view)
		for _, tc := range []struct {
			on   bool
			want string
		}{{true, "0"}, {false, "1"}} {
			row := settingsToggleButton(k, "c", "e", "Captcha", tc.on)
			if len(row) != 1 {
				t.Fatalf("toggle row has %d buttons, want 1", len(row))
			}
			fields := settingsButtonFields(t, row[0])
			if fields["a"] != "tg" || fields["v"] != tc.want {
				t.Errorf("toggle on=%v fields = %v, want v %s", tc.on, fields, tc.want)
			}
		}
	})

	t.Run("parse number", func(t *testing.T) {
		for value, want := range map[string]int{"1": 1, "500": 500, "0": 0} {
			if got, ok := settingsParseNumber(value, 3); !ok || got != want {
				t.Errorf("settingsParseNumber(%q) = (%d, %v), want (%d, true)", value, got, ok, want)
			}
		}
		for _, bad := range []string{"", "-1", "+1", "1a", " 1", "1234", "١"} {
			if got, ok := settingsParseNumber(bad, 3); ok {
				t.Errorf("settingsParseNumber(%q) = (%d, true), want a refusal", bad, got)
			}
		}
	})

	t.Run("an overflowing button is counted, never silent", func(t *testing.T) {
		huge := settingsView{Tr: view.Tr, OwnerKey: "c", OwnerValue: strings.Repeat("9", 70), GroupID: -1}
		k := newSettingsKeyboard(huge)
		if _, ok := k.button("x", map[string]string{"a": "bk"}); ok {
			t.Fatal("a button over 64 bytes was built")
		}
		if k.omitted != 1 {
			t.Errorf("omitted = %d, want 1", k.omitted)
		}
	})
}

func TestSettingsAnonymousAdmin(t *testing.T) {
	env := newSettingsEnv(t)
	creator := gotgbot.User{Id: uniqueLinkOwnerID() + 23, FirstName: "Tap<per"}
	env.fake.setCreator(env.chat.Id, creator.Id)
	env.fake.setMember(env.chat.Id, creator.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusCreator})

	env.sendAnonymous("/settings")
	env.wantProofPrompt()
	for _, sent := range env.replies() {
		if strings.Contains(fmt.Sprint(sent.Params["text"]), staffMarker("settings_choose")) {
			t.Fatal("an anonymous admin got the choice message before the proof")
		}
	}

	env.tapProof(creator)
	msgID, here := env.newestChoice()
	if fields := settingsButtonFields(t, here); fields["u"] != fmt.Sprint(creator.Id) {
		t.Errorf("Open here carries u = %q, want the tapper %d", fields["u"], creator.Id)
	}

	env.pressRaw(env.chat, creator, msgID, here.CallbackData)
	text, _ := env.lastEdit(env.chat.Id, msgID)
	if !strings.Contains(text, staffMarker("settings_header_main")) {
		t.Errorf("Open here by the tapper drew %q, want the Main page", text)
	}
}

func TestSettingsGroupLanguage(t *testing.T) {
	env := newSettingsEnvWith(t, settingsEnvOptions{RealLocale: true})
	if err := lang.ChangeGroupLanguage(env.chat.Id, "es"); err != nil {
		t.Fatalf("ChangeGroupLanguage: %v", err)
	}
	spanish, err := i18n.MustNewTranslator("es").GetString("settings_header_main")
	if err != nil || spanish == "" {
		t.Fatalf("Spanish settings_header_main: %q, %v", spanish, err)
	}
	english, _ := i18n.MustNewTranslator("en").GetString("settings_header_main")
	if spanish == english {
		t.Fatal("the Spanish and English headers are identical, the test cannot tell them apart")
	}

	msgID := env.openMenu(env.admin)
	text, _ := env.lastEdit(env.chat.Id, msgID)
	if !strings.HasPrefix(text, spanish) {
		t.Errorf("Main page %q does not start with the Spanish header %q", text, spanish)
	}
	if strings.HasPrefix(text, english) {
		t.Errorf("Main page %q starts with the English header", text)
	}
}

func TestSettingsCaptchaReadFailure(t *testing.T) {
	env := newSettingsEnv(t)
	previous := settingsCaptchaRead
	settingsCaptchaRead = func(context.Context, int64) (*models.CaptchaSettings, error) {
		return nil, errors.New("database unavailable")
	}
	t.Cleanup(func() { settingsCaptchaRead = previous })

	msgID := env.openMenu(env.admin)
	_, keyboard := env.lastEdit(env.chat.Id, msgID)
	button, ok := settingsButtonByMarker(keyboard, "settings_page_captcha")
	if !ok {
		t.Fatal("no Captcha button on the Main page")
	}
	if !strings.HasSuffix(button.Text, " · ❔") {
		t.Errorf("Captcha button %q does not end with the unreadable state ' · ❔'", button.Text)
	}

	before := env.answerCount()
	env.pressRaw(env.chat, env.admin, msgID, button.CallbackData)

	if got := env.answerCount() - before; got != 1 {
		t.Errorf("the press was answered %d times, want once", got)
	}
	answer, alert := env.lastAnswer()
	if !alert || !strings.Contains(answer, staffMarker("settings_cannot_open_alert")) {
		t.Errorf("answer = %q (alert %v), want the alert settings_cannot_open_alert", answer, alert)
	}
	text, kb := env.lastEdit(env.chat.Id, msgID)
	if !strings.Contains(text, staffMarker("settings_cannot_open")) || strings.Contains(text, staffMarker("settings_cannot_open_alert")) {
		t.Errorf("the menu was edited to %q, want settings_cannot_open", text)
	}
	if len(kb) != 0 {
		t.Errorf("the cannot-open edit kept the keyboard %+v", kb)
	}
}

// settingsMenuTitleCap is the rune cap of group titles and names (D-10).
const settingsMenuTitleCap = 64

func TestSettingsComposeCapsAndEscapes(t *testing.T) {
	view := settingsView{
		Tr:         i18n.MustNewTranslator("en"),
		OwnerKey:   "u",
		OwnerValue: "1",
		GroupID:    -1001,
		GroupTitle: strings.Repeat("<", settingsMenuTitleCap+10),
		OpenerName: "N",
		Now:        time.Unix(0, 0),
	}
	text := settingsCompose(view, "", []string{"body"}, "", false)
	if strings.Contains(text, "<<") {
		t.Errorf("the title is not escaped: %q", text)
	}
	if got := strings.Count(text, "&lt;"); got != settingsMenuTitleCap {
		t.Errorf("the title keeps %d characters, want it cut at %d", got, settingsMenuTitleCap)
	}
}
