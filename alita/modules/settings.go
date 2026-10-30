package modules

import (
	"strconv"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/callbackquery"
	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/i18n"
	"github.com/divkix/Alita_Robot/alita/utils/chat_status"
	"github.com/divkix/Alita_Robot/alita/utils/formatting"
	"github.com/divkix/Alita_Robot/alita/utils/helpers"
)

// settingsModule is the Settings module: /settings and the one-message button menu behind
// it. It adds no handler group; its callbacks are matched by prefix.
var settingsModule = moduleStruct{moduleName: "Settings"}

const (
	// settingsCallbackNamespace is the callback namespace of the menu (settings|v1|...).
	settingsCallbackNamespace = "settings"
	// settingsStartPrefix starts the /start payload of the private entry:
	// settings_<chat id>.
	settingsStartPrefix = "settings_"
)

// settingsDesc is deliberately not Disableable. Its authority is a live getChatMember of
// the sender (a creator or an administrator), never the admin cache; an anonymous admin
// gets the proof first and re-enters as the person who tapped.
var settingsDesc = helpers.CommandDescriptor{
	Name:           "settings",
	RequiredChecks: []helpers.CheckFunc{requireLockdownGroup(), requireLockdownAuthority(false)},
}

// settingsPrivateURL is the Open in private link: a deep link into the bot's /start with
// the settings_<chat id> payload, at most 23 characters of [A-Za-z0-9_-]. Anyone can tap a
// URL button, so authority is decided only when /start arrives.
func settingsPrivateURL(botUsername string, chatID int64) string {
	return "https://t.me/" + botUsername + "?start=" + settingsStartPrefix + strconv.FormatInt(chatID, 10)
}

// settingsChoiceScreen is the reply to /settings: where to open the menu. Open here carries
// the opener's ID u; Open in private is a URL button, left out (and logged) when the bot has
// no username. The group's title is escaped and spliced in after translation.
func settingsChoiceScreen(tr *i18n.Translator, botUsername string, chat *gotgbot.Chat, openerID int64) settingsScreen {
	view := settingsView{
		Tr:         tr,
		OwnerKey:   "u",
		OwnerValue: strconv.FormatInt(openerID, 10),
		GroupID:    chat.Id,
		GroupTitle: chat.Title,
	}
	text := settingsText(tr, "settings_choose", i18n.TranslationParams{"group": settingsGroupToken})
	text = lockdownSplice(text, settingsGroupToken, staffDisplayTitle(chat.Title))

	k := newSettingsKeyboard(view)
	var row []gotgbot.InlineKeyboardButton
	if here, ok := k.button(settingsText(tr, "settings_btn_open_here"), map[string]string{"a": settingsActOpenHere}); ok {
		row = append(row, here)
	}
	if botUsername != "" {
		row = append(row, gotgbot.InlineKeyboardButton{
			Text: settingsText(tr, "settings_btn_open_private"),
			Url:  settingsPrivateURL(botUsername, chat.Id),
		})
	} else {
		log.Error("[Settings] the bot has no username, so the Open in private button is left out")
	}
	k.row(row...)
	return k.screen(text)
}

// settings replies to /settings with the two-button choice message.
func (m moduleStruct) settings(b *gotgbot.Bot, ctx *ext.Context) error {
	chat := ctx.EffectiveChat
	msg := ctx.EffectiveMessage
	sender := chat_status.GetEffectiveUser(ctx)
	if chat == nil || msg == nil || sender == nil {
		return ext.EndGroups
	}
	screen := settingsChoiceScreen(ctxTr(ctx), getBotUsername(b), chat, sender.Id)
	opts := formatting.Shtml()
	opts.ReplyMarkup = screen.Keyboard
	if _, err := msg.Reply(b, screen.Text, opts); err != nil {
		log.Errorf("[Settings] choice message in chat %d: %v", chat.Id, err)
	}
	return ext.EndGroups
}

// LoadSettings registers /settings and the settings| callback. The help text, the alt names
// and the module switch arrive with the docs, so /help never lists a module without text.
func LoadSettings(dispatcher *ext.Dispatcher) {
	helpers.WrapCommand(dispatcher, settingsDesc, pipelineHandler(settingsModule.settings))
	// No SetAllowChannel: a menu is never a channel message.
	dispatcher.AddHandler(handlers.NewCallback(callbackquery.Prefix("settings|"), settingsModule.settingsCallback))
}

func init() {
	RegisterLegacyModule("Settings", 239, LoadSettings)
	// An anonymous admin proves who they are first; the proof re-enters here as the person
	// who tapped, and the same live check runs for them.
	RegisterAnonymousAdminHandler("settings", anonPipelineHandler(settingsDesc, settingsModule.settings))
}
