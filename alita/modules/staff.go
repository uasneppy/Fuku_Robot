package modules

import (
	"errors"
	"strconv"
	"strings"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/callbackquery"
	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/db/staff"
	"github.com/divkix/Alita_Robot/alita/i18n"
	"github.com/divkix/Alita_Robot/alita/utils/chat_status"
	"github.com/divkix/Alita_Robot/alita/utils/formatting"
	"github.com/divkix/Alita_Robot/alita/utils/helpers"
)

var staffModule = moduleStruct{
	moduleName: "Staff",
}

var (
	setStaffDesc = helpers.CommandDescriptor{
		Name:           "setstaff",
		RequiredChecks: []helpers.CheckFunc{helpers.RejectAnonymousSender(), helpers.RequireGroup()},
	}
	unsetStaffDesc = helpers.CommandDescriptor{
		Name:           "unsetstaff",
		RequiredChecks: []helpers.CheckFunc{helpers.RejectAnonymousSender(), helpers.RequireGroup()},
	}
	staffDesc = helpers.CommandDescriptor{
		Name: "staff",
	}
)

// staffCallbackNamespace is the callbackcodec namespace of every Staff Group
// button. Later plans add actions under it.
const staffCallbackNamespace = "staff"

// Actions carried in the "a" field of a staff callback.
const (
	staffActUnsetConfirm  = "uy"
	staffActUnsetCancel   = "un"
	staffActUnlinkAsk     = "ul"
	staffActUnlinkConfirm = "uc"
	staffActUnlinkCancel  = "ux"
	staffActRefresh       = "rf"
	staffActPage          = "pg"
	staffActRunConfirm    = "xc"
	staffActRunCancel     = "xn"
)

// staffGroupsToken stands in for the group list while the translation is
// interpolated. Group titles are user-controlled, and the translator runs a
// printf-style pass over the interpolated text, so a title such as "100%d" must
// be spliced in afterwards, never passed through the translator.
const staffGroupsToken = "<<staff-groups>>"

// replyStaff sends an HTML reply to the command message and logs a failure.
func replyStaff(c *helpers.CommandContext, text string) {
	if _, err := c.Msg.Reply(c.Bot, text, formatting.Shtml()); err != nil {
		log.Errorf("[Staff] reply: %v", err)
	}
}

// setStaff makes the current group a Staff Group. Authority comes from a live
// getChatAdministrators call (never the cached admin list): only the group's
// current creator may do it, and an API error refuses without writing anything.
//
// When several checks would fail the reason reported is fixed: anonymous sender
// (the pipeline's RejectAnonymousSender), chat type, live creator, bot
// administrator, role conflict. Arguments after the command are ignored.
func (m moduleStruct) setStaff(c *helpers.CommandContext) error {
	if c.Chat.Type != "group" && c.Chat.Type != "supergroup" {
		// RequireGroup only rejects private chats, so channels reach this point.
		text, _ := c.Tr.GetString("staff_refuse_not_group")
		replyStaff(c, text)
		return ext.EndGroups
	}
	if !m.requireLiveCreator(c) || !m.requireBotAdministrator(c) {
		return ext.EndGroups
	}
	m.createStaffGroup(c)
	return ext.EndGroups
}

// requireLiveCreator replies and returns false unless Telegram, asked live,
// lists the sender as the creator of the chat.
func (moduleStruct) requireLiveCreator(c *helpers.CommandContext) bool {
	result, _, err := chat_status.CheckOwner(c.Bot, c.Chat.Id, c.User.Id)
	switch result {
	case chat_status.OwnerUnknown:
		log.Warnf("[Staff] setStaff: owner check for chat %d failed: %v", c.Chat.Id, err)
		text, _ := c.Tr.GetString("staff_check_failed")
		replyStaff(c, text)
		return false
	case chat_status.OwnerMismatch:
		text, _ := c.Tr.GetString("staff_refuse_not_owner")
		replyStaff(c, text)
		return false
	}
	return true
}

// requireBotAdministrator replies and returns false unless the bot is already an
// administrator of the chat (D-08). A failed lookup refuses as unverifiable.
func (moduleStruct) requireBotAdministrator(c *helpers.CommandContext) bool {
	member, result, err := chat_status.FetchBotMember(c.Bot, c.Chat.Id)
	switch result {
	case chat_status.BotMemberUnknown:
		log.Warnf("[Staff] setStaff: bot membership check for chat %d failed: %v", c.Chat.Id, err)
		text, _ := c.Tr.GetString("staff_check_failed")
		replyStaff(c, text)
		return false
	case chat_status.BotMemberMissing:
		text, _ := c.Tr.GetString("staff_refuse_bot_not_admin")
		replyStaff(c, text)
		return false
	}
	if member.Status != gotgbot.ChatMemberStatusAdministrator {
		text, _ := c.Tr.GetString("staff_refuse_bot_not_admin")
		replyStaff(c, text)
		return false
	}
	return true
}

// createStaffGroup records the chat as a Staff Group and reports the outcome:
// refused (the chat is a linked group), already a Staff Group, or newly made.
func (moduleStruct) createStaffGroup(c *helpers.CommandContext) {
	created, err := staff.CreateStaffGroup(c.Chat.Id, c.User.Id, c.Chat.Title)
	switch {
	case errors.Is(err, staff.ErrRoleConflict):
		text, _ := c.Tr.GetString("staff_refuse_group_is_linked")
		replyStaff(c, text)
	case err != nil:
		log.Errorf("[Staff] setStaff: %v", err)
		text, _ := c.Tr.GetString("staff_check_failed")
		replyStaff(c, text)
	case !created:
		text, _ := c.Tr.GetString("staff_set_already")
		replyStaff(c, text)
	default:
		text, _ := c.Tr.GetString("staff_set_done", i18n.TranslationParams{
			"chat_id": c.Chat.Id,
		})
		replyStaff(c, text)
	}
}

// staffPanel shows the Staff Group help text and chat ID. Outside a Staff Group
// it stays silent so the command does not reveal that the feature exists.
func (moduleStruct) staffPanel(c *helpers.CommandContext) error {
	group := staff.GetStaffGroup(c.Chat.Id)
	if group == nil {
		return ext.EndGroups
	}
	text, keyboard, err := buildStaffPanel(c.Bot, c.Tr, *group, 0)
	if err != nil {
		failed, _ := c.Tr.GetString("staff_check_failed")
		replyStaff(c, failed)
		return ext.EndGroups
	}
	opts := formatting.Shtml()
	if len(keyboard.InlineKeyboard) > 0 {
		opts.ReplyMarkup = keyboard
	}
	if _, err := c.Msg.Reply(c.Bot, text, opts); err != nil {
		log.Errorf("[Staff] staffPanel: %v", err)
	}
	return ext.EndGroups
}

// unsetStaff asks the live creator to confirm removing Staff status. Nothing is
// deleted here: the Confirm callback re-checks the creator and does the removal.
func (moduleStruct) unsetStaff(c *helpers.CommandContext) error {
	group, err := staff.GetStaffGroupFresh(c.Chat.Id)
	if err != nil {
		text, _ := c.Tr.GetString("staff_check_failed")
		replyStaff(c, text)
		return ext.EndGroups
	}
	if group == nil {
		text, _ := c.Tr.GetString("staff_unset_not_staff")
		replyStaff(c, text)
		return ext.EndGroups
	}

	result, _, ownerErr := chat_status.CheckOwner(c.Bot, c.Chat.Id, c.User.Id)
	switch result {
	case chat_status.OwnerUnknown:
		log.Warnf("[Staff] unsetStaff: owner check for chat %d failed: %v", c.Chat.Id, ownerErr)
		text, _ := c.Tr.GetString("staff_check_failed")
		replyStaff(c, text)
		return ext.EndGroups
	case chat_status.OwnerMismatch:
		text, _ := c.Tr.GetString("staff_refuse_not_owner")
		replyStaff(c, text)
		return ext.EndGroups
	}

	count, err := staff.CountLinksByStaffFresh(c.Chat.Id)
	if err != nil {
		text, _ := c.Tr.GetString("staff_check_failed")
		replyStaff(c, text)
		return ext.EndGroups
	}

	text, _ := c.Tr.GetString("staff_unset_confirm_prompt", i18n.TranslationParams{"count": count})
	confirmLabel, _ := c.Tr.GetString("staff_btn_confirm")
	cancelLabel, _ := c.Tr.GetString("staff_btn_cancel")
	opts := formatting.Shtml()
	opts.ReplyMarkup = gotgbot.InlineKeyboardMarkup{
		InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{
			{
				Text:         confirmLabel,
				CallbackData: encodeCallbackData(staffCallbackNamespace, map[string]string{"a": staffActUnsetConfirm}),
			},
			{
				Text:         cancelLabel,
				CallbackData: encodeCallbackData(staffCallbackNamespace, map[string]string{"a": staffActUnsetCancel}),
			},
		}},
	}
	if _, err := c.Msg.Reply(c.Bot, text, opts); err != nil {
		log.Errorf("[Staff] unsetStaff: %v", err)
	}
	return ext.EndGroups
}

// staffCallback dispatches the staff| buttons. The chat always comes from the
// message the button sits on, never from the callback data, and every action
// answers the query exactly once.
func (m moduleStruct) staffCallback(b *gotgbot.Bot, ctx *ext.Context) error {
	query, ok := callbackQueryFromContext(ctx)
	if !ok {
		return ext.EndGroups
	}
	decoded, ok := decodeCallbackData(query.Data, staffCallbackNamespace)
	if !ok || query.Message == nil {
		return answerInvalidCallback(b, ctx, query)
	}
	tr := ctxTr(ctx)

	switch decoded.Fields["a"] {
	case staffActUnsetConfirm:
		return m.staffUnsetConfirm(b, query, tr)
	case staffActUnsetCancel:
		return m.staffUnsetCancel(b, query, tr)
	case staffActUnlinkAsk:
		return m.staffUnlinkAsk(b, query, tr, decoded.Fields)
	case staffActUnlinkConfirm:
		return m.staffUnlinkConfirm(b, query, tr, decoded.Fields)
	case staffActUnlinkCancel:
		return m.staffUnlinkCancel(b, query, tr, decoded.Fields)
	case staffActRefresh:
		return m.staffPanelRefresh(b, query, tr, decoded.Fields)
	case staffActPage:
		return m.staffPanelPage(b, query, tr, decoded.Fields)
	case staffActRunConfirm:
		return m.staffActionConfirm(b, query, tr, decoded.Fields)
	default:
		text, _ := tr.GetString("staff_cb_expired")
		answerStaffCallback(b, query, text, false)
		return ext.EndGroups
	}
}

// answerStaffCallback answers the callback query once, as a toast or an alert.
func answerStaffCallback(b *gotgbot.Bot, query *gotgbot.CallbackQuery, text string, alert bool) {
	if _, err := query.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: text, ShowAlert: alert}); err != nil {
		log.Errorf("[Staff] answer callback: %v", err)
	}
}

// editStaffCallbackMessage replaces the text of the message a button sits on,
// which also removes its keyboard.
func editStaffCallbackMessage(b *gotgbot.Bot, query *gotgbot.CallbackQuery, text string) {
	if _, _, err := query.Message.EditText(b, &gotgbot.EditMessageTextOpts{
		Text:      text,
		ParseMode: formatting.HTML,
	}); err != nil {
		log.Errorf("[Staff] edit callback message: %v", err)
	}
}

// staffUnsetConfirm removes Staff status after a live creator check of the
// person who pressed Confirm. The recorded owner_user_id is never the authority.
func (moduleStruct) staffUnsetConfirm(b *gotgbot.Bot, query *gotgbot.CallbackQuery, tr *i18n.Translator) error {
	chatID := query.Message.GetChat().Id

	group, err := staff.GetStaffGroupFresh(chatID)
	if err != nil {
		text, _ := tr.GetString("staff_check_failed")
		answerStaffCallback(b, query, text, true)
		return ext.EndGroups
	}
	if group == nil {
		text, _ := tr.GetString("staff_unset_already_removed")
		answerStaffCallback(b, query, text, false)
		return ext.EndGroups
	}

	result, _, ownerErr := chat_status.CheckOwner(b, chatID, query.From.Id)
	switch result {
	case chat_status.OwnerUnknown:
		log.Warnf("[Staff] unset confirm: owner check for chat %d failed: %v", chatID, ownerErr)
		text, _ := tr.GetString("staff_check_failed")
		answerStaffCallback(b, query, text, true)
		return ext.EndGroups
	case chat_status.OwnerMismatch:
		text, _ := tr.GetString("staff_cb_owner_only")
		answerStaffCallback(b, query, text, true)
		return ext.EndGroups
	}

	removed, deleted, err := staff.DeleteStaffGroupWithLinks(chatID)
	if err != nil {
		text, _ := tr.GetString("staff_check_failed")
		answerStaffCallback(b, query, text, true)
		return ext.EndGroups
	}
	if !deleted {
		text, _ := tr.GetString("staff_unset_already_removed")
		answerStaffCallback(b, query, text, false)
		return ext.EndGroups
	}

	text, _ := tr.GetString("staff_unset_done", i18n.TranslationParams{
		"count":  len(removed),
		"groups": staffGroupsToken,
	})
	text = strings.TrimSpace(strings.Replace(text, staffGroupsToken, renderUnlinkedGroups(removed), 1))
	editStaffCallbackMessage(b, query, text)
	answerStaffCallback(b, query, "", false)
	return ext.EndGroups
}

// staffUnsetCancel leaves Staff status untouched and closes the prompt.
func (moduleStruct) staffUnsetCancel(b *gotgbot.Bot, query *gotgbot.CallbackQuery, tr *i18n.Translator) error {
	text, _ := tr.GetString("staff_unset_cancelled")
	editStaffCallbackMessage(b, query, text)
	answerStaffCallback(b, query, "", false)
	return ext.EndGroups
}

// renderUnlinkedGroups lists removed links one per line, in the order given
// (link id ascending): the escaped title and the chat ID in a code tag.
func renderUnlinkedGroups(links []models.StaffGroupLink) string {
	var sb strings.Builder
	for i, link := range links {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(staffDisplayTitle(link.GroupTitle))
		sb.WriteString(" <code>")
		sb.WriteString(strconv.FormatInt(link.GroupChatID, 10))
		sb.WriteString("</code>")
	}
	return sb.String()
}

// LoadStaff registers the Staff Group commands.
func LoadStaff(dispatcher *ext.Dispatcher) {
	SetModuleEnabled(staffModule.moduleName, true)

	helpers.WrapCommand(dispatcher, setStaffDesc, staffModule.setStaff)
	helpers.WrapCommand(dispatcher, unsetStaffDesc, staffModule.unsetStaff)
	helpers.WrapCommand(dispatcher, staffDesc, staffModule.staffPanel)
	helpers.WrapCommand(dispatcher, linkStaffDesc, staffModule.linkStaff)
	helpers.WrapCommand(dispatcher, unlinkStaffDesc, staffModule.unlinkStaff)
	dispatcher.AddHandler(handlers.NewCallback(callbackquery.Prefix("staff|"), staffModule.staffCallback))
}

func init() {
	RegisterLegacyModule("Staff", 236, LoadStaff)
}
