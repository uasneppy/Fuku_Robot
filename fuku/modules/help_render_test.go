//go:build testtools

package modules

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

type helpFakeBotClient struct {
	response json.RawMessage
	err      error
}

func (f helpFakeBotClient) RequestWithContext(context.Context, string, string, map[string]any, *gotgbot.RequestOpts) (json.RawMessage, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.response, nil
}

func (f helpFakeBotClient) GetAPIURL(*gotgbot.RequestOpts) string {
	return "https://api.telegram.org"
}

func (f helpFakeBotClient) FileURL(token string, path string, _ *gotgbot.RequestOpts) string {
	return "https://api.telegram.org/file/bot" + token + "/" + path
}

func helpTestTranslator(t *testing.T) *i18n.Translator {
	t.Helper()

	tr, err := i18n.NewTestTranslator(`
help_info_about_header: "About Xenobot-72"
help_bot_intro: "Intro. "
help_pm_intro: "Hi %s. "
help_all_commands_usage: "Use commands."
help_button_about_me: "About me"
help_button_configuration: "Config"
common_back_arrow_alt: "Back"
help_button_about: "About"
help_button_add_to_chat: "Add"
help_button_commands_help: "Commands"
help_button_language: "Language"
`)
	if err != nil {
		t.Fatalf("NewTestTranslator() error = %v", err)
	}
	return tr
}

func TestHelpTextRendering(t *testing.T) {
	t.Parallel()

	tr := helpTestTranslator(t)
	if got := getAboutText(tr); got != "About Xenobot-72" {
		t.Fatalf("getAboutText() = %q", got)
	}
	if got := getStartHelp(tr); got != "Intro. " {
		t.Fatalf("getStartHelp() = %q", got)
	}
	if got := getMainHelp(tr, "Div"); got != "Hi Div. Use commands." {
		t.Fatalf("getMainHelp() = %q", got)
	}
}

func TestHelpKeyboardsUseCallbackCodecAndBotUsername(t *testing.T) {
	t.Parallel()

	tr := helpTestTranslator(t)
	aboutKb := getAboutKb(tr)
	if len(aboutKb.InlineKeyboard) != 3 {
		t.Fatalf("getAboutKb() rows = %d, want 3 (about, configuration, back)", len(aboutKb.InlineKeyboard))
	}
	if aboutKb.InlineKeyboard[0][0].Text != "About me" {
		t.Fatalf("about button text = %q", aboutKb.InlineKeyboard[0][0].Text)
	}
	if !strings.HasPrefix(aboutKb.InlineKeyboard[0][0].CallbackData, "about|v1|") {
		t.Fatalf("about callback = %q, want encoded about callback", aboutKb.InlineKeyboard[0][0].CallbackData)
	}

	startKb := getStartMarkup(tr, "FukuRobot")
	if len(startKb.InlineKeyboard) != 4 {
		t.Fatalf("getStartMarkup() rows = %d, want 4", len(startKb.InlineKeyboard))
	}
	if got := startKb.InlineKeyboard[1][0].Url; got != "https://t.me/FukuRobot?startgroup=botstart" {
		t.Fatalf("add-to-chat URL = %q", got)
	}
	if got := len(startKb.InlineKeyboard[1]); got != 1 {
		t.Fatalf("add-to-chat row has %d buttons, want the add button alone", got)
	}
	// No button of either keyboard may send members to a third-party channel or group: the only
	// URL button is the add-to-chat link built from the bot's own username.
	for name, kb := range map[string]gotgbot.InlineKeyboardMarkup{"about": aboutKb, "start": startKb} {
		for _, row := range kb.InlineKeyboard {
			for _, btn := range row {
				if btn.Url != "" && btn.Url != "https://t.me/FukuRobot?startgroup=botstart" {
					t.Fatalf("%s keyboard has a third-party URL button: %q", name, btn.Url)
				}
			}
		}
	}
	if !strings.HasPrefix(startKb.InlineKeyboard[2][0].CallbackData, "helpq|v1|") {
		t.Fatalf("commands callback = %q, want encoded help callback", startKb.InlineKeyboard[2][0].CallbackData)
	}
}

func resetCachedBotUsername() {
	cachedBotUsernameMu.Lock()
	cachedBotUsername = ""
	cachedBotUsernameMu.Unlock()
}

func TestGetBotUsernameCachesStructAndGetMeFallbacks(t *testing.T) {
	resetCachedBotUsername()
	t.Cleanup(resetCachedBotUsername)

	if got := getBotUsername(&gotgbot.Bot{User: gotgbot.User{Username: "StructBot"}}); got != "StructBot" {
		t.Fatalf("getBotUsername(struct) = %q", got)
	}
	if got := getBotUsername(nil); got != "StructBot" {
		t.Fatalf("getBotUsername(cached) = %q", got)
	}

	resetCachedBotUsername()
	bot := &gotgbot.Bot{
		Token: "123:test",
		BotClient: helpFakeBotClient{response: json.RawMessage(
			`{"id":123,"is_bot":true,"first_name":"Fuku","username":"GetMeBot"}`,
		)},
	}
	if got := getBotUsername(bot); got != "GetMeBot" {
		t.Fatalf("getBotUsername(getMe) = %q", got)
	}

	resetCachedBotUsername()
	if got := getBotUsername(nil); got != "" {
		t.Fatalf("getBotUsername(nil) = %q, want empty", got)
	}
}

func TestRenderModuleHelpHTMLBodyKeepsBoldTags(t *testing.T) {
	t.Parallel()

	header := "Here is the help for the *%s* module:\n\n"
	body := `<b>🎭 Auto-Reactions</b>

Automatically react to messages with specific keywords!

<b>Admin Commands:</b>
• /addreaction <keyword> <emoji> - Add an auto-reaction
<b>Example:</b>
• /addreaction hello 👋`

	// A page that opens with its own bold title line gets no generic header, so the title is shown once.
	got := renderModuleHelp(header, "Reactions", body)
	if strings.Contains(got, "&lt;b&gt;") || strings.Contains(got, "*Reactions*") || strings.Contains(got, "module:") {
		t.Fatalf("renderModuleHelp() escaped HTML, left markdown or added the header: %q", got)
	}
	if !strings.HasPrefix(got, "<b>🎭 Auto-Reactions</b>\n\n") {
		t.Fatalf("renderModuleHelp() does not open with the page's own title: %q", got)
	}
	if !strings.Contains(got, "<b>Admin Commands:</b>") {
		t.Fatalf("renderModuleHelp() missing Admin Commands heading: %q", got)
	}
	if !strings.Contains(got, "<b>Example:</b>") {
		t.Fatalf("renderModuleHelp() missing Example heading: %q", got)
	}
	// A page without its own title still gets the header, with a blank line before the body.
	untitled := renderModuleHelp(header, "Reactions", strings.SplitN(body, "\n\n", 2)[1])
	if !strings.Contains(untitled, "<b>Reactions</b>") || !strings.Contains(untitled, "module:\n\nAutomatically react") &&
		!strings.Contains(untitled, "module:</b>\n\nAutomatically react") {
		t.Fatalf("renderModuleHelp() of an untitled page = %q, want the header and a blank line", untitled)
	}
	if !strings.Contains(got, "&lt;keyword&gt;") || !strings.Contains(got, "&lt;emoji&gt;") {
		t.Fatalf("renderModuleHelp() did not escape placeholders: %q", got)
	}
}

func TestRenderModuleHelpMarkdownBodyConvertsBold(t *testing.T) {
	t.Parallel()

	header := "Here is the help for the *%s* module:\n\n"
	body := "*Admin Commands:*\n× /promote `<reply/username>`: Promote a user."
	got := renderModuleHelp(header, "Admin", body)
	if !strings.Contains(got, "<b>Admin</b>") {
		t.Fatalf("renderModuleHelp(markdown) missing header bold: %q", got)
	}
	if !strings.Contains(got, "<b>Admin Commands:</b>") && !strings.Contains(got, "<b>Admin Commands</b>") {
		t.Fatalf("renderModuleHelp(markdown) missing converted section heading: %q", got)
	}
	if strings.Contains(got, "<reply/username>") {
		t.Fatalf("renderModuleHelp(markdown) left raw placeholder: %q", got)
	}
}
