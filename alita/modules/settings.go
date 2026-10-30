package modules

import (
	"context"
	"strconv"
	"strings"
	"time"

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

// settingsStartDeepLink opens the Main page in the user's private chat for the group named
// by a /start settings_<chat id> payload. The payload is attacker-controlled text and
// authorizes nothing: it must parse strictly (parseStaffChatIDArg: an optional minus,
// digits, at most 20 characters, a negative result), and a live getChatMember of the user
// in that group must show a creator or an administrator before any menu is sent. Every
// refusal is a plain reply with no keyboard.
func settingsStartDeepLink(b *gotgbot.Bot, ctx *ext.Context, user *gotgbot.User, arg string) error {
	if ctx == nil || ctx.EffectiveChat == nil || ctx.EffectiveMessage == nil || user == nil {
		return ext.EndGroups
	}
	tr := ctxTr(ctx)
	refuse := func(key string) error {
		replyHTML(b, ctx.EffectiveMessage, settingsText(tr, key))
		return ext.EndGroups
	}

	rest, _ := strings.CutPrefix(arg, settingsStartPrefix)
	chatID, ok := parseStaffChatIDArg(rest)
	if !ok {
		return refuse("settings_start_invalid")
	}

	member, err := lockdownLiveMember(b, chatID, user.Id)
	if err != nil {
		if chat_status.IsBotMissingError(err) {
			return refuse("settings_cannot_open")
		}
		log.Warnf("[Settings] live check of user %d in chat %d failed: %v", user.Id, chatID, err)
		return refuse("settings_check_failed")
	}
	if member.Status != gotgbot.ChatMemberStatusCreator && member.Status != gotgbot.ChatMemberStatusAdministrator {
		return refuse("settings_start_not_admin")
	}

	lookupCtx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()
	full, err := b.GetChatWithContext(lookupCtx, chatID, nil)
	if err != nil || full == nil {
		if err != nil && chat_status.IsBotMissingError(err) {
			return refuse("settings_cannot_open")
		}
		log.Warnf("[Settings] getChat of chat %d for the private entry failed: %v", chatID, err)
		return refuse("settings_check_failed")
	}
	group := full.ToChat()
	if group.Type != "group" && group.Type != "supergroup" {
		return refuse("settings_cannot_open")
	}

	view := settingsView{
		Tr:         tr,
		Private:    true,
		OwnerKey:   "c",
		OwnerValue: strconv.FormatInt(chatID, 10),
		GroupID:    chatID,
		GroupTitle: group.Title,
		OpenerName: staffFullName(user),
		Now:        time.Now(),
	}
	screen := renderSettingsMain(view, settingsMainEntries(b, tr, &group), "")
	opts := formatting.Shtml()
	opts.ReplyMarkup = screen.Keyboard
	if _, err := b.SendMessage(ctx.EffectiveChat.Id, screen.Text, opts); err != nil {
		log.Errorf("[Settings] private Main page for chat %d to user %d: %v", group.Id, user.Id, err)
	}
	return ext.EndGroups
}

// LoadSettings registers /settings and the settings| callback, and lists the module in
// /help (its text is settings_help_msg, its alt name comes from locales/config.yml).
func LoadSettings(dispatcher *ext.Dispatcher) {
	SetModuleEnabled(settingsModule.moduleName, true)

	helpers.WrapCommand(dispatcher, settingsDesc, pipelineHandler(settingsModule.settings))
	// No SetAllowChannel: a menu is never a channel message.
	dispatcher.AddHandler(handlers.NewCallback(callbackquery.Prefix("settings|"), settingsModule.settingsCallback))
}

func init() {
	RegisterLegacyModule("Settings", 239, LoadSettings)
	// The Open in private button of the choice message lands here through /start, which
	// is left untouched (D-29): the router picks the longest registered prefix.
	RegisterDeepLinkHandler(settingsStartPrefix, settingsStartDeepLink)
	// An anonymous admin proves who they are first; the proof re-enters here as the person
	// who tapped, and the same live check runs for them.
	RegisterAnonymousAdminHandler("settings", anonPipelineHandler(settingsDesc, settingsModule.settings))
}
