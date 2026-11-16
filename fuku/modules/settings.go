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

	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/chat_status"
	"github.com/uasneppy/fuku_robot/fuku/utils/formatting"
	"github.com/uasneppy/fuku_robot/fuku/utils/helpers"
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
	// settingsRepairTimeout bounds the roster refresh that follows a group /settings.
	settingsRepairTimeout = 10 * time.Second
)

// settingsDesc is deliberately not Disableable. Its authority is a live getChatMember of
// the sender (a creator or an administrator), never the admin cache, and then the owner
// gate (requireSettingsAccess: by default only the owner, or an administrator the owner let
// in); an anonymous admin gets the proof first and re-enters as the person who tapped, who
// is checked and gated like anyone else.
var settingsDesc = helpers.CommandDescriptor{
	Name:           "settings",
	RequiredChecks: []helpers.CheckFunc{requireSettingsAccess()},
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

// settingsPrivateList sends the private group list, page 1, as a new message in the user's
// language. It is built from the sender's own ID with a database read and makes no Telegram
// call but the send. A failed read is one plain reply, never a half-drawn list.
func settingsPrivateList(b *gotgbot.Bot, ctx *ext.Context, chat *gotgbot.Chat, sender *gotgbot.User) error {
	tr := ctxTr(ctx)
	screen, err := settingsHubScreen(context.Background(), tr, sender.Id, 1)
	if err != nil {
		log.Errorf("[Settings] group list of user %d: %v", sender.Id, err)
		if _, sendErr := b.SendMessage(chat.Id, settingsText(tr, "settings_check_failed"), nil); sendErr != nil {
			log.Warnf("[Settings] list failure notice to chat %d: %v", chat.Id, sendErr)
		}
		return ext.EndGroups
	}
	opts := formatting.Shtml()
	opts.ReplyMarkup = screen.Keyboard
	if _, err := b.SendMessage(chat.Id, screen.Text, opts); err != nil {
		log.Errorf("[Settings] group list to chat %d: %v", chat.Id, err)
	}
	return ext.EndGroups
}

// settings replies to /settings: in a group the two-button choice message, in the bot's private
// chat the list of the sender's groups.
func (m moduleStruct) settings(b *gotgbot.Bot, ctx *ext.Context) error {
	chat := ctx.EffectiveChat
	msg := ctx.EffectiveMessage
	sender := chat_status.GetEffectiveUser(ctx)
	if chat == nil || msg == nil || sender == nil {
		return ext.EndGroups
	}
	if chat.Type == "private" {
		return settingsPrivateList(b, ctx, chat, sender)
	}
	screen := settingsChoiceScreen(ctxTr(ctx), getBotUsername(b), chat, sender.Id)
	opts := formatting.Shtml()
	opts.ReplyMarkup = screen.Keyboard
	if _, err := msg.Reply(b, screen.Text, opts); err != nil {
		log.Errorf("[Settings] choice message in chat %d: %v", chat.Id, err)
	}
	settingsRepairAdminIndex(b, chat.Id)
	return ext.EndGroups
}

// settingsRepairAdminIndex refreshes the group's roster record after /settings, the second
// repair path after /admincache (D-07, MENU-10). It runs after the choice message, so a slow or
// failed refresh never delays or changes what the sender sees, and only a sender the checks
// let in reaches it. It makes at most two Telegram calls under settingsRepairTimeout and only
// logs an error: the refresh changes rows on definitive facts alone.
func settingsRepairAdminIndex(b *gotgbot.Bot, chatID int64) {
	ctx, cancel := context.WithTimeout(context.Background(), settingsRepairTimeout)
	defer cancel()
	if _, err := refreshChatAdminIndex(ctx, b, chatID); err != nil {
		log.Debugf("[Settings] roster refresh of chat %d: %v", chatID, err)
	}
}

// settingsStartDeepLink opens the Main page in the user's private chat for the group named
// by a /start settings_<chat id> payload. The payload is attacker-controlled text and
// authorizes nothing, so the order is fixed and each step refuses before the next runs:
//
//  1. the payload must parse strictly (parseStaffChatIDArg: an optional minus, digits, at
//     most 20 characters, a negative result), with no Telegram call otherwise;
//  2. a live getChatMember of the user in that group must show a creator or an administrator;
//  3. settingsGate, on the member record that check returned, decides whether the owner gate
//     lets this administrator in (D-16), so a gated administrator costs no getChat;
//  4. one getChat of the group, and only a group or supergroup is opened.
//
// Telegram errors are classified by class, never by wording (UI-SPEC R3): a 4xx other than
// 429 at either lookup, and a chat that is not a group or supergroup, answer
// settings_start_not_admin, the words a plain member gets, so a forged payload learns nothing
// about whether the bot is in a chat; a 429, a 5xx, a transport error or a timeout answers
// settings_check_failed. Every refusal is a plain reply with no keyboard.
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
		if isTelegramClientError(err) {
			// The same words as a plain member gets: any user can send any chat ID, so a
			// different answer would tell them whether the bot is in that chat.
			return refuse("settings_start_not_admin")
		}
		log.Warnf("[Settings] live check of user %d in chat %d failed: %v", user.Id, chatID, err)
		return refuse("settings_check_failed")
	}
	if member.Status != gotgbot.ChatMemberStatusCreator && member.Status != gotgbot.ChatMemberStatusAdministrator {
		return refuse("settings_start_not_admin")
	}

	switch settingsGate(b, chatID, *user, member) {
	case settingsGateIn:
	case settingsGateOwnerOnly:
		return refuse("settings_start_owner_only")
	default:
		return refuse("settings_check_failed")
	}

	lookupCtx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()
	full, err := b.GetChatWithContext(lookupCtx, chatID, nil)
	if err != nil || full == nil {
		if err != nil && isTelegramClientError(err) {
			return refuse("settings_start_not_admin")
		}
		log.Warnf("[Settings] getChat of chat %d for the private entry failed: %v", chatID, err)
		return refuse("settings_check_failed")
	}
	group := full.ToChat()
	if group.Type != "group" && group.Type != "supergroup" {
		return refuse("settings_start_not_admin")
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
		Owner:      member.Status == gotgbot.ChatMemberStatusCreator,
	}
	screen := renderSettingsMain(view, settingsMainEntries(b, tr, &group, view.Owner, view.Private), "")
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
	// The private group list's own buttons (paging, Close); a list is never a channel message.
	dispatcher.AddHandler(handlers.NewCallback(callbackquery.Prefix("shub|"), settingsModule.settingsHubCallback))
	LoadSettingsInput(dispatcher)
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
