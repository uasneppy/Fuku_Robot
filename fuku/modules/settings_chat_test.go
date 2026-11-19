//go:build testtools

package modules

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/config"
	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/chatconv"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// withChatProviders sets ENABLE_CHAT and the two provider keys for one test and restores them.
// The stand-in key values are named so gosec does not read them as credentials.
func withChatProviders(t *testing.T, enabled, anthropic, google bool) {
	t.Helper()
	cfg := config.AppConfig
	prevEnabled, prevAnthropic, prevGoogle := cfg.EnableChat, cfg.AnthropicAPIKey, cfg.GeminiAPIKey
	t.Cleanup(func() { cfg.EnableChat, cfg.AnthropicAPIKey, cfg.GeminiAPIKey = prevEnabled, prevAnthropic, prevGoogle })
	const anthropicProviderValue, googleProviderValue = "chat-test-anthropic-value", "chat-test-google-value"
	cfg.EnableChat, cfg.AnthropicAPIKey, cfg.GeminiAPIKey = enabled, "", ""
	if anthropic {
		cfg.AnthropicAPIKey = anthropicProviderValue
	}
	if google {
		cfg.GeminiAPIKey = googleProviderValue
	}
}

// settingsChatStored reads the conversation setting straight from the database.
func settingsChatStored(t *testing.T, chatID int64) chatconv.Settings {
	t.Helper()
	got, err := chatconv.GetFresh(context.Background(), chatID)
	if err != nil {
		t.Fatalf("GetFresh(%d) = %v", chatID, err)
	}
	return got
}

func settingsChatRows(t *testing.T, chatID int64) int64 {
	t.Helper()
	var n int64
	if err := db.DB.Model(&models.ChatConversation{}).Where("chat_id = ?", chatID).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

// settingsChatOpenPrivate opens the private menu of env's group for user and presses the
// Conversations row; it returns the message ID.
func settingsChatOpenPrivate(t *testing.T, env *settingsEnv, user gotgbot.User) int64 {
	t.Helper()
	env.sendPrivate(user, fmt.Sprintf("/start settings_%d", env.chat.Id))
	msgID := settingsPrivateOnlySent(t, env, user).MessageID
	env.pressPrivate(user, msgID, env.privateFields(env.chat.Id, "a", "op", "p", settingsPageChat))
	return msgID
}

func settingsChatFind(t *testing.T, keyboard [][]gotgbot.InlineKeyboardButton, match func(map[string]string) bool) (gotgbot.InlineKeyboardButton, bool) {
	t.Helper()
	for _, button := range settingsAllButtons(keyboard) {
		if match(settingsButtonFields(t, button)) {
			return button, true
		}
	}
	return gotgbot.InlineKeyboardButton{}, false
}

func settingsChatEnv(t *testing.T) *settingsEnv {
	t.Helper()
	resetSettingsOwnerMemo(t)
	env := newSettingsEnv(t)
	chatConvCleanup(t, env.chat.Id)
	return env
}

func TestSettingsChatOwnerFlow(t *testing.T) {
	withChatProviders(t, true, true, true)
	env := settingsChatEnv(t)
	owner := env.newOwner(env.chat)
	private := settingsPrivateChat(owner)

	msgID := settingsChatOpenPrivate(t, env, owner)
	text, keyboard := env.lastEdit(owner.Id, msgID)
	for _, want := range []string{"settings_chat_intro", "settings_chat_status", "settings_chat_hint", "settings_chat_data_notice"} {
		if !strings.Contains(text, staffMarker(want)) {
			t.Fatalf("the page %q lacks %s", text, want)
		}
	}
	if !strings.Contains(text, "Anthropic (Claude)") {
		t.Errorf("the page does not name the stored provider: %q", text)
	}

	toggle, ok := settingsChatFind(t, keyboard, func(f map[string]string) bool { return f["a"] == "tg" && f["t"] == settingsChatControlSwitch })
	if !ok || settingsButtonFields(t, toggle)["v"] != "1" {
		t.Fatalf("no off toggle with v=1: %+v", keyboard)
	}
	env.pressRaw(private, owner, msgID, toggle.CallbackData)
	text, keyboard = env.lastEdit(owner.Id, msgID)
	if !strings.Contains(text, staffMarker("settings_chat_confirm")) || !strings.Contains(text, staffMarker("settings_chat_data_notice")) {
		t.Fatalf("the first tap drew %q, want the card with the data notice", text)
	}
	if settingsChatRows(t, env.chat.Id) != 0 {
		t.Fatal("the first tap wrote a row")
	}
	yes, ok := settingsChatFind(t, keyboard, func(f map[string]string) bool { return f["a"] == settingsActChatYes && f["e"] != "" })
	if !ok {
		t.Fatalf("the card has no Yes: %+v", keyboard)
	}
	env.pressRaw(private, owner, msgID, yes.CallbackData)
	if got := settingsChatStored(t, env.chat.Id); !got.Enabled || got.Provider != chatconv.ProviderAnthropic {
		t.Fatalf("after Yes: %+v", got)
	}

	_, keyboard = env.lastEdit(owner.Id, msgID)
	google, ok := settingsChatFind(t, keyboard, func(f map[string]string) bool { return f["t"] == settingsChatControlProvider && f["v"] == "g" })
	if !ok {
		t.Fatalf("no Google button: %+v", keyboard)
	}
	env.pressRaw(private, owner, msgID, google.CallbackData)
	if got := settingsChatStored(t, env.chat.Id); got.Provider != chatconv.ProviderGoogle || !got.Enabled {
		t.Fatalf("after Google: %+v", got)
	}
	text, keyboard = env.lastEdit(owner.Id, msgID)
	if !strings.Contains(text, "Google (Gemini)") {
		t.Errorf("the data notice does not name Google: %q", text)
	}
	admins, ok := settingsChatFind(t, keyboard, func(f map[string]string) bool { return f["t"] == settingsChatControlAudience && f["v"] == "a" })
	if !ok {
		t.Fatal("no audience button")
	}
	env.pressRaw(private, owner, msgID, admins.CallbackData)
	if got := settingsChatStored(t, env.chat.Id); got.Audience != chatconv.AudienceAdminsApproved {
		t.Fatalf("after audience: %+v", got)
	}

	_, keyboard = env.lastEdit(owner.Id, msgID)
	off, _ := settingsChatFind(t, keyboard, func(f map[string]string) bool { return f["a"] == "tg" && f["t"] == settingsChatControlSwitch })
	if settingsButtonFields(t, off)["v"] != "0" {
		t.Fatal("the on toggle does not send v=0")
	}
	env.pressRaw(private, owner, msgID, off.CallbackData)
	if got := settingsChatStored(t, env.chat.Id); got.Enabled {
		t.Fatal("one tap did not turn conversations off")
	}
}

func TestSettingsChatMainAndGroupMenu(t *testing.T) {
	withChatProviders(t, true, true, false)
	env := settingsChatEnv(t)
	owner := env.newOwner(env.chat)

	// In a group menu the owner's Main shows the row with In private, and opening it is the note.
	msgID := env.openMenu(owner)
	text, keyboard := env.lastEdit(env.chat.Id, msgID)
	if _, ok := settingsPageButton(t, keyboard, settingsPageChat); !ok {
		t.Fatal("the owner's group Main has no Conversations row")
	}
	if line, ok := settingsMainStateLine(text, "settings_page_chat"); !ok || !strings.Contains(line, staffMarker("settings_state_private_only")) {
		t.Errorf("the group Main line = %q, want In private", line)
	}
	env.press(owner, msgID, env.groupFields(owner, "a", "op", "p", settingsPageChat))
	text, _ = env.lastEdit(env.chat.Id, msgID)
	if !strings.Contains(text, staffMarker("settings_entries_group_note")) || strings.Contains(text, staffMarker("settings_chat_intro")) {
		t.Errorf("the group menu opened %q, want the private-only note", text)
	}

	// Forged presses from the group menu write nothing, whoever sends them.
	for _, fields := range []map[string]string{
		env.groupFields(owner, "a", "tg", "p", settingsPageChat, "t", settingsChatControlSwitch, "v", "0"),
		env.groupFields(owner, "a", "st", "p", settingsPageChat, "t", settingsChatControlAudience, "v", "a"),
		env.groupFields(owner, "a", settingsActChatYes, "e", "99999999"),
	} {
		env.press(owner, msgID, fields)
		if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("common_callback_invalid_request")) {
			t.Errorf("a group press %v answered %q (alert %v), want the invalid-request alert", fields, answer, alert)
		}
	}
	if settingsChatRows(t, env.chat.Id) != 0 {
		t.Fatal("a group-menu press wrote a row")
	}

	// The private Main reads the stored state.
	if err := chatconv.SetEnabled(context.Background(), env.chat.Id, true, owner.Id); err != nil {
		t.Fatal(err)
	}
	env.sendPrivate(owner, fmt.Sprintf("/start settings_%d", env.chat.Id))
	sent := env.fake.sentTo(owner.Id)
	privateMain := fmt.Sprint(sent[len(sent)-1].Params["text"])
	if line, ok := settingsMainStateLine(privateMain, "settings_page_chat"); !ok || !strings.Contains(line, "Anthropic") {
		t.Errorf("the private Main line = %q, want On and Anthropic", line)
	}
}

func TestSettingsChatAuthority(t *testing.T) {
	withChatProviders(t, true, true, true)

	t.Run("an administrator never sees the row and a forged press is refused", func(t *testing.T) {
		env := settingsChatEnv(t)
		env.newOwner(env.chat)
		env.setAdminsAllowed(env.chat.Id, true)
		env.sendPrivate(env.admin, fmt.Sprintf("/start settings_%d", env.chat.Id))
		sent := settingsPrivateOnlySent(t, env, env.admin)
		msgID := sent.MessageID
		all := env.fake.sentTo(env.admin.Id)
		if _, ok := settingsPageButton(t, staffKeyboardOf(all[len(all)-1].Params["reply_markup"]), settingsPageChat); ok {
			t.Error("an administrator's Main has the Conversations row")
		}
		for _, fields := range []map[string]string{
			env.privateFields(env.chat.Id, "a", "op", "p", settingsPageChat),
			env.privateFields(env.chat.Id, "a", "tg", "p", settingsPageChat, "t", settingsChatControlSwitch, "v", "1"),
			env.privateFields(env.chat.Id, "a", settingsActChatYes, "e", "99999999"),
			env.privateFields(env.chat.Id, "a", "st", "p", settingsPageChat, "t", settingsChatControlProvider, "v", "g"),
		} {
			env.pressPrivate(env.admin, msgID, fields)
			if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("settings_access_owner_only")) {
				t.Errorf("the admin's press %v answered %q (alert %v), want the owner-only alert", fields, answer, alert)
			}
		}
		if settingsChatRows(t, env.chat.Id) != 0 {
			t.Fatal("an administrator's press wrote a row")
		}
	})

	t.Run("an administrator the owner gate refuses is refused by the gate", func(t *testing.T) {
		env := settingsChatEnv(t)
		env.newOwner(env.chat)
		env.setAdminsAllowed(env.chat.Id, false)
		env.pressPrivate(env.admin, 78, env.privateFields(env.chat.Id, "a", "tg", "p", settingsPageChat, "t", settingsChatControlSwitch, "v", "1"))
		if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("settings_gate_owner_only")) {
			t.Errorf("a gated administrator's press answered %q (alert %v)", answer, alert)
		}
		if settingsChatRows(t, env.chat.Id) != 0 {
			t.Fatal("a gated administrator's press wrote a row")
		}
	})

	t.Run("a plain member is refused", func(t *testing.T) {
		env := settingsChatEnv(t)
		env.newOwner(env.chat)
		member := gotgbot.User{Id: uniqueLinkOwnerID() + 8_000_000, FirstName: "Member"}
		env.fake.setMember(env.chat.Id, member.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
		env.pressPrivate(member, 77, env.privateFields(env.chat.Id, "a", "tg", "p", settingsPageChat, "t", settingsChatControlSwitch, "v", "1"))
		if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("settings_not_admin")) {
			t.Errorf("a member's press answered %q (alert %v)", answer, alert)
		}
		if settingsChatRows(t, env.chat.Id) != 0 {
			t.Fatal("a member's press wrote a row")
		}
	})

	t.Run("a demotion between the card's two taps is refused", func(t *testing.T) {
		env := settingsChatEnv(t)
		owner := env.newOwner(env.chat)
		private := settingsPrivateChat(owner)
		msgID := settingsChatOpenPrivate(t, env, owner)
		_, keyboard := env.lastEdit(owner.Id, msgID)
		toggle, _ := settingsChatFind(t, keyboard, func(f map[string]string) bool { return f["a"] == "tg" })
		env.pressRaw(private, owner, msgID, toggle.CallbackData)
		_, keyboard = env.lastEdit(owner.Id, msgID)
		yes, ok := settingsChatFind(t, keyboard, func(f map[string]string) bool { return f["a"] == settingsActChatYes })
		if !ok {
			t.Fatal("no Yes on the card")
		}
		env.fake.setMember(env.chat.Id, owner.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusAdministrator})
		env.pressRaw(private, owner, msgID, yes.CallbackData)
		if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("settings_access_owner_only")) {
			t.Errorf("the demoted Yes answered %q (alert %v)", answer, alert)
		}
		if settingsChatStored(t, env.chat.Id).Enabled {
			t.Fatal("a demoted owner's Yes turned conversations on")
		}
	})
}

func TestSettingsChatNotConfigured(t *testing.T) {
	cases := []struct {
		name                        string
		enabled, anthropic, google  bool
		wantLine                    string
		providerButtons, wantToggle int
	}{
		{"ENABLE_CHAT off", false, true, true, "settings_chat_not_set_up", 0, 0},
		{"no key", true, false, false, "settings_chat_not_set_up", 0, 0},
		{"only Google, Anthropic stored", true, false, true, "settings_chat_provider_missing", 1, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withChatProviders(t, tc.enabled, tc.anthropic, tc.google)
			env := settingsChatEnv(t)
			owner := env.newOwner(env.chat)
			msgID := settingsChatOpenPrivate(t, env, owner)
			text, keyboard := env.lastEdit(owner.Id, msgID)
			if !strings.Contains(text, "🚫 "+staffMarker(tc.wantLine)) {
				t.Errorf("the page %q lacks the %s line", text, tc.wantLine)
			}
			providers, toggles := 0, 0
			for _, button := range settingsAllButtons(keyboard) {
				f := settingsButtonFields(t, button)
				if f["t"] == settingsChatControlProvider {
					providers++
				}
				if f["t"] == settingsChatControlSwitch {
					toggles++
				}
			}
			if providers != tc.providerButtons || toggles != tc.wantToggle {
				t.Errorf("provider buttons %d, toggles %d, want %d and %d", providers, toggles, tc.providerButtons, tc.wantToggle)
			}

			// A forged On is refused with the reason and writes nothing.
			env.pressPrivate(owner, msgID, env.privateFields(env.chat.Id, "a", "tg", "p", settingsPageChat, "t", settingsChatControlSwitch, "v", "1"))
			if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker(tc.wantLine)) {
				t.Errorf("a forged On answered %q (alert %v), want %s", answer, alert, tc.wantLine)
			}
			if !tc.anthropic {
				env.pressPrivate(owner, msgID, env.privateFields(env.chat.Id, "a", "st", "p", settingsPageChat, "t", settingsChatControlProvider, "v", "a"))
				if _, alert := env.lastAnswer(); !alert {
					t.Error("a forged Anthropic press was not refused")
				}
			}
			if settingsChatRows(t, env.chat.Id) != 0 {
				t.Fatal("a refused press wrote a row")
			}
		})
	}

	t.Run("stored on with nothing configured can still be turned off", func(t *testing.T) {
		withChatProviders(t, false, false, false)
		env := settingsChatEnv(t)
		owner := env.newOwner(env.chat)
		if err := chatconv.SetEnabled(context.Background(), env.chat.Id, true, owner.Id); err != nil {
			t.Fatal(err)
		}
		msgID := settingsChatOpenPrivate(t, env, owner)
		_, keyboard := env.lastEdit(owner.Id, msgID)
		off, ok := settingsChatFind(t, keyboard, func(f map[string]string) bool { return f["t"] == settingsChatControlSwitch && f["v"] == "0" })
		if !ok {
			t.Fatal("no Off toggle while stored on")
		}
		env.pressRaw(settingsPrivateChat(owner), owner, msgID, off.CallbackData)
		if settingsChatStored(t, env.chat.Id).Enabled {
			t.Fatal("Off did not write")
		}
	})
}

// TestSettingsDriftChat proves the page draws what the repository stores, written by anything,
// and that opening Main and the page creates no row.
func TestSettingsDriftChat(t *testing.T) {
	withChatProviders(t, true, true, true)
	env := settingsChatEnv(t)
	owner := env.newOwner(env.chat)
	msgID := settingsChatOpenPrivate(t, env, owner)
	if settingsChatRows(t, env.chat.Id) != 0 {
		t.Fatal("opening Main and the page created a row")
	}
	ctx := context.Background()
	if err := chatconv.SetProvider(ctx, env.chat.Id, chatconv.ProviderGoogle, 1); err != nil {
		t.Fatal(err)
	}
	if err := chatconv.SetAudience(ctx, env.chat.Id, chatconv.AudienceAdminsApproved, 1); err != nil {
		t.Fatal(err)
	}
	env.pressPrivate(owner, msgID, env.privateFields(env.chat.Id, "a", "op", "p", settingsPageChat))
	text, keyboard := env.lastEdit(owner.Id, msgID)
	if !strings.Contains(text, "Google (Gemini)") || !strings.Contains(text, staffMarker("settings_chat_audience_admins")) {
		t.Errorf("the page %q does not draw the stored provider and audience", text)
	}
	selected, ok := settingsChatFind(t, keyboard, func(f map[string]string) bool { return f["t"] == settingsChatControlProvider && f["v"] == "g" })
	if !ok || !strings.HasPrefix(selected.Text, settingsSelectedPrefix) {
		t.Errorf("the stored provider is not marked: %+v", selected)
	}
}

func TestSettingsChatCallbackBytes(t *testing.T) {
	withChatProviders(t, true, true, true)
	tr := i18n.MustNewTranslator("en")
	view := settingsView{Tr: tr, Private: true, OwnerKey: "c", OwnerValue: "-1001234567890", GroupID: -1001234567890, GroupTitle: "G", OpenerName: "N", Owner: true}
	screens := map[string]settingsScreen{"card": renderSettingsChatCard(view, chatconv.ProviderGoogle)}
	for _, on := range []bool{false, true} {
		for _, provider := range chatProviders {
			screens[fmt.Sprintf("page %t %s", on, provider)] = renderSettingsChat(view, chatconv.Settings{Enabled: on, Provider: provider, Audience: 1}, "")
		}
	}
	for label, screen := range screens {
		if screen.Omitted != 0 {
			t.Errorf("%s omitted %d buttons", label, screen.Omitted)
		}
		for _, button := range settingsAllButtons(screen.Keyboard.InlineKeyboard) {
			if n := len(button.CallbackData); n < 1 || n > 64 {
				t.Errorf("%s button %q has %d bytes", label, button.Text, n)
			}
		}
	}
}

func TestSettingsChatPageFitsEveryLocale(t *testing.T) {
	withChatProviders(t, true, true, true)
	withRealLocales(t)
	title := strings.Repeat("&<ж😀", 16)
	for _, code := range []string{"uk", "en", "es", "fr", "hi", "id", "pt", "ru"} {
		t.Run(code, func(t *testing.T) {
			tr := i18n.MustNewTranslator(code)
			notice := settingsNoticeWarn(settingsText(tr, "common_settings_save_failed"))
			view := settingsView{Tr: tr, Private: true, OwnerKey: "c", OwnerValue: "-1001234567890", GroupID: -1001234567890, GroupTitle: title, OpenerName: "N", Owner: true}
			for _, screen := range []settingsScreen{
				renderSettingsChatCard(view, chatconv.ProviderAnthropic),
				renderSettingsChat(view, chatconv.Settings{Enabled: true, Provider: chatconv.ProviderGoogle, Audience: 1}, notice),
			} {
				if got := panelUTF16Len(screen.Text); got > 3800 {
					t.Errorf("a screen is %d UTF-16 units, want at most 3800", got)
				}
				for _, button := range settingsAllButtons(screen.Keyboard.InlineKeyboard) {
					if n := utf8.RuneCountInString(button.Text); n == 0 || n > 40 {
						t.Errorf("button %q is %d code points", button.Text, n)
					}
				}
			}
		})
	}
}
