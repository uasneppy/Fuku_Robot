package modules

import (
	"context"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db/chatconv"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// The Conversations page (page p, Phase 20) is where a group's owner turns Xenobot-72's
// conversations on or off, picks the provider and says who may talk to the bot. It is drawn and
// opened only for the live creator (OwnerOnly) and only in the bot's private chat (PrivateOnly):
// in a menu opened in the group the owner sees the framework's note and no page code runs, and
// every control and action refuses a press from a group menu again before any read or write.
// Every control declares settingsRightOwner, judged on the press's own live getChatMember record.
// Turning conversations on takes two taps through the shared confirm card, whose text is the data
// notice; turning them off takes one. There is no command to compare with.

const (
	// settingsPageChat is the p value of the page.
	settingsPageChat = "p"
	// settingsChatControlSwitch is the on/off toggle (tg only, v 0 or 1).
	settingsChatControlSwitch = "e"
	// settingsChatControlProvider is the provider choice (st only, v a or g).
	settingsChatControlProvider = "m"
	// settingsChatControlAudience is the audience choice (st only, v e or a).
	settingsChatControlAudience = "w"
	// settingsActChatYes is the On card's Yes (fields: e).
	settingsActChatYes = "py"
)

// settingsChatProviderCodes maps a button's v to the stored provider.
var settingsChatProviderCodes = map[string]string{"a": chatconv.ProviderAnthropic, "g": chatconv.ProviderGoogle}

// settingsChatAudienceCodes maps a button's v to the stored audience.
var settingsChatAudienceCodes = map[string]int{"e": chatconv.AudienceEveryone, "a": chatconv.AudienceAdminsApproved}

// settingsChatRead reads the setting fresh; a variable only so a test can make it fail.
var settingsChatRead = chatconv.GetFresh

func init() {
	registerSettingsPage(settingsPage{
		Key:         settingsPageChat,
		Order:       89,
		Icon:        "🗨",
		TitleKey:    "settings_page_chat",
		OwnerOnly:   true,
		PrivateOnly: true,
		MainState:   settingsChatMainState,
		Open:        settingsChatOpen,
		Controls: map[string]settingsControl{
			settingsChatControlSwitch: {
				Right: settingsRightOwner, Set: settingsChatSetEnabled,
				Actions: []string{settingsActToggle}, Validate: settingsEditSwitchValue,
			},
			settingsChatControlProvider: {
				Right: settingsRightOwner, Set: settingsChatSetProvider,
				Actions: []string{settingsActSet}, Validate: func(v string) bool { _, ok := settingsChatProviderCodes[v]; return ok },
			},
			settingsChatControlAudience: {
				Right: settingsRightOwner, Set: settingsChatSetAudience,
				Actions: []string{settingsActSet}, Validate: func(v string) bool { _, ok := settingsChatAudienceCodes[v]; return ok },
			},
		},
		Actions: map[string]settingsPageAction{
			settingsActChatYes: {
				Fields: []string{"e"}, Right: settingsRightOwner,
				// The owner field c exists only on a private menu's buttons, so a group menu's
				// press is an invalid request before any lookup.
				Validate: func(fields map[string]string) bool { return fields["c"] != "" && fields["e"] != "" },
				Run:      settingsChatYes,
			},
		},
	})
}

func settingsChatReadSettings(chatID int64) (chatconv.Settings, error) {
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()
	return settingsChatRead(ctx, chatID)
}

// settingsChatMainState is the page's Main line: Off, or On and the provider's short name. While the
// server cannot run the stored provider the line still shows the stored state; the page explains.
func settingsChatMainState(_ *gotgbot.Bot, tr *i18n.Translator, group *gotgbot.Chat) (string, bool) {
	stored, err := settingsChatReadSettings(group.Id)
	if err != nil {
		return "", false
	}
	if !stored.Enabled {
		return settingsText(tr, "settings_state_off_plain"), true
	}
	return settingsText(tr, "settings_state_chat_on", i18n.TranslationParams{"provider": chatProviderShortName(stored.Provider)}), true
}

// settingsChatOpen is the page's Open. A read error is the cannot-open edit.
func settingsChatOpen(p *settingsPress, notice string) (settingsScreen, error) {
	stored, err := settingsChatReadSettings(p.group.Id)
	if err != nil {
		return settingsScreen{}, err
	}
	return renderSettingsChat(p.view(), stored, notice), nil
}

// settingsChatDataNotice is the data notice for provider, the same text on the page and the card.
func settingsChatDataNotice(tr *i18n.Translator, provider string) string {
	return settingsText(tr, "settings_chat_data_notice", i18n.TranslationParams{"provider": chatProviderName(provider)})
}

// renderSettingsChat draws the page. It is pure: the server's configuration is read through
// chatProviderConfigured, which reads config.AppConfig only. The page names no Staff Group, no
// member and no count.
func renderSettingsChat(v settingsView, stored chatconv.Settings, notice string) settingsScreen {
	audienceKey := "settings_chat_audience_everyone"
	if stored.Audience == chatconv.AudienceAdminsApproved {
		audienceKey = "settings_chat_audience_admins"
	}
	state := settingsStateWord(v.Tr, stored.Enabled)
	body := []string{
		settingsText(v.Tr, "settings_chat_intro"),
		"",
		settingsText(v.Tr, "settings_chat_status", i18n.TranslationParams{"state": state}),
		settingsText(v.Tr, "settings_chat_provider_line", i18n.TranslationParams{"provider": chatProviderName(stored.Provider)}),
		settingsText(v.Tr, "settings_chat_audience_line", i18n.TranslationParams{"audience": settingsText(v.Tr, audienceKey)}),
	}
	anyConfigured := chatAnyProviderConfigured()
	switch {
	case !anyConfigured:
		body = append(body, "🚫 "+settingsText(v.Tr, "settings_chat_not_set_up"))
	case !chatProviderConfigured(stored.Provider):
		body = append(body, "🚫 "+settingsText(v.Tr, "settings_chat_provider_missing"))
	}
	body = append(body, "", settingsText(v.Tr, "settings_chat_hint"), "", settingsChatDataNotice(v.Tr, stored.Provider))

	k := newSettingsKeyboard(v)
	if anyConfigured || stored.Enabled {
		label := settingsText(v.Tr, "settings_btn_chat_toggle", i18n.TranslationParams{"state": state})
		k.row(settingsToggleButton(k, settingsPageChat, settingsChatControlSwitch, label, stored.Enabled)...)
	}
	var providers []gotgbot.InlineKeyboardButton
	for _, code := range []string{"a", "g"} {
		provider := settingsChatProviderCodes[code]
		if !chatProviderConfigured(provider) {
			continue
		}
		label := chatProviderName(provider)
		if provider == stored.Provider {
			label = settingsSelectedPrefix + label
		}
		if button, ok := k.button(label, map[string]string{"a": settingsActSet, "p": settingsPageChat, "t": settingsChatControlProvider, "v": code}); ok {
			providers = append(providers, button)
		}
	}
	k.row(providers...)
	var audience []gotgbot.InlineKeyboardButton
	for _, choice := range []struct{ code, key string }{{"e", "settings_btn_chat_everyone"}, {"a", "settings_btn_chat_admins"}} {
		label := settingsText(v.Tr, choice.key)
		if settingsChatAudienceCodes[choice.code] == stored.Audience {
			label = settingsSelectedPrefix + label
		}
		if button, ok := k.button(label, map[string]string{"a": settingsActSet, "p": settingsPageChat, "t": settingsChatControlAudience, "v": choice.code}); ok {
			audience = append(audience, button)
		}
	}
	k.row(audience...)
	k.navRow(false)
	return k.screen(settingsCompose(v, "settings_page_chat", body, notice, false))
}

// renderSettingsChatCard draws the On card: the question, the data notice and [Yes | No].
func renderSettingsChatCard(v settingsView, provider string) settingsScreen {
	k := newSettingsKeyboard(v)
	settingsCardRows(k, map[string]string{"a": settingsActChatYes}, settingsPageChat)
	body := []string{settingsText(v.Tr, "settings_chat_confirm"), "", settingsChatDataNotice(v.Tr, provider)}
	return k.screen(settingsCompose(v, "settings_page_chat", body, "", false))
}

// settingsChatGroupRefused refuses a press from a menu opened in the group: conversations are
// turned on only in the bot's private chat. It answers the invalid-request alert and edits
// nothing, before any read or write.
func settingsChatGroupRefused(p *settingsPress) bool {
	if p.private {
		return false
	}
	p.answer(settingsText(p.tr, "common_callback_invalid_request"), true)
	return true
}

// settingsChatRedraw reads the setting and redraws the page with notice; a read error is the
// cannot-open edit.
func settingsChatRedraw(p *settingsPress, notice string) *settingsScreen {
	stored, err := settingsChatReadSettings(p.group.Id)
	if err != nil {
		log.Warnf("[Settings] the conversation setting of chat %d could not be read: %v", p.group.Id, err)
		p.cannotOpen()
		return nil
	}
	screen := renderSettingsChat(p.view(), stored, notice)
	return &screen
}

// settingsChatFromResult draws the page from an apply result: the read-back row, or a fresh read
// with the save-failed notice when the write failed.
func settingsChatFromResult(p *settingsPress, result chatApplyResult) *settingsScreen {
	if result.Outcome != chatApplied {
		return settingsChatRedraw(p, settingsNoticeWarn(settingsText(p.tr, "common_settings_save_failed")))
	}
	if result.Settings == nil {
		return settingsChatRedraw(p, "")
	}
	screen := renderSettingsChat(p.view(), *result.Settings, "")
	return &screen
}

// settingsChatUnavailable refuses a press that needs a provider this server cannot run (a stale
// or forged button): the alert names the reason, nothing is written, and the page is redrawn so
// the stale screen is repaired.
func settingsChatUnavailable(p *settingsPress) *settingsScreen {
	key := "settings_chat_provider_missing"
	if !chatAnyProviderConfigured() {
		key = "settings_chat_not_set_up"
	}
	p.answer(settingsText(p.tr, key), true)
	if screen := settingsChatRedraw(p, ""); screen != nil {
		p.edit(*screen)
	}
	return nil
}

// settingsChatSetEnabled serves the toggle. 0 writes off at once; 1 draws the card when the
// stored provider can run, and writes nothing.
func settingsChatSetEnabled(p *settingsPress, value string) *settingsScreen {
	if settingsChatGroupRefused(p) {
		return nil
	}
	if value == "0" {
		ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
		defer cancel()
		return settingsChatFromResult(p, applyChatEnabled(ctx, p.group.Id, false, p.query.From.Id))
	}
	stored, err := settingsChatReadSettings(p.group.Id)
	if err != nil {
		p.cannotOpen()
		return nil
	}
	if !chatProviderConfigured(stored.Provider) {
		return settingsChatUnavailable(p)
	}
	if stored.Enabled {
		screen := renderSettingsChat(p.view(), stored, "")
		return &screen
	}
	screen := renderSettingsChatCard(p.view(), stored.Provider)
	return &screen
}

// settingsChatYes writes on after the card's expiry check and the provider check again. The press
// already ran the whole pipeline, so a presser who is no longer the live creator never gets here.
func settingsChatYes(p *settingsPress) *settingsScreen {
	if settingsChatGroupRefused(p) {
		return nil
	}
	expired, ok := settingsCardExpired(p.fields)
	if !ok {
		p.answer(settingsText(p.tr, "common_callback_invalid_request"), true)
		return nil
	}
	if expired {
		return settingsChatRedraw(p, settingsNoticeWarn(settingsText(p.tr, "settings_card_expired")))
	}
	stored, err := settingsChatReadSettings(p.group.Id)
	if err != nil {
		p.cannotOpen()
		return nil
	}
	if !chatProviderConfigured(stored.Provider) {
		return settingsChatUnavailable(p)
	}
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()
	return settingsChatFromResult(p, applyChatEnabled(ctx, p.group.Id, true, p.query.From.Id))
}

// settingsChatSetProvider stores the provider. A provider this server cannot run is refused before
// any write; the stored one writes nothing.
func settingsChatSetProvider(p *settingsPress, value string) *settingsScreen {
	if settingsChatGroupRefused(p) {
		return nil
	}
	provider := settingsChatProviderCodes[value]
	if !chatProviderConfigured(provider) {
		return settingsChatUnavailable(p)
	}
	stored, err := settingsChatReadSettings(p.group.Id)
	if err != nil {
		p.cannotOpen()
		return nil
	}
	if stored.Provider == provider {
		screen := renderSettingsChat(p.view(), stored, "")
		return &screen
	}
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()
	return settingsChatFromResult(p, applyChatProvider(ctx, p.group.Id, provider, p.query.From.Id))
}

// settingsChatSetAudience stores who may talk to the bot; the stored value writes nothing.
func settingsChatSetAudience(p *settingsPress, value string) *settingsScreen {
	if settingsChatGroupRefused(p) {
		return nil
	}
	audience := settingsChatAudienceCodes[value]
	stored, err := settingsChatReadSettings(p.group.Id)
	if err != nil {
		p.cannotOpen()
		return nil
	}
	if stored.Audience == audience {
		screen := renderSettingsChat(p.view(), stored, "")
		return &screen
	}
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()
	return settingsChatFromResult(p, applyChatAudience(ctx, p.group.Id, audience, p.query.From.Id))
}
