package modules

import (
	"context"
	"strconv"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/i18n"
	"github.com/divkix/Alita_Robot/alita/utils/chat_status"
	"github.com/divkix/Alita_Robot/alita/utils/error_handling"
	"github.com/divkix/Alita_Robot/alita/utils/formatting"
)

// The settings| callbacks carry no authority. Every press runs one pipeline, each step
// refusing before the next runs (settingsPress.verify): decode and structure with no I/O, a
// Telegram service identity, the opener of a group menu, one live getChatMember of the
// presser in the group being configured, and then the right of the control. The menu keeps
// no state of its own: no Redis key, so a press works on any replica with Redis down.

// The core action codes. A page declares its own codes through registerSettingsPage.
const (
	// settingsActOpenHere turns the choice message into the Main page.
	settingsActOpenHere = "oh"
	// settingsActOpen opens page p (also Refresh, and a stepper's value button).
	settingsActOpen = "op"
	// settingsActBack redraws the Main page.
	settingsActBack = "bk"
	// settingsActClose deletes the menu message.
	settingsActClose = "cl"
	// settingsActToggle sets switch t of page p to the absolute value v.
	settingsActToggle = "tg"
	// settingsActSet sets control t of page p to the absolute value v.
	settingsActSet = "st"
	// settingsActCancel cancels a confirm card and redraws page p.
	settingsActCancel = "no"
)

// settingsReadTimeout bounds one database read of a page.
const settingsReadTimeout = 5 * time.Second

// settingsRight is the right a control needs beyond being a live creator or administrator
// of the group, which every press already proved at step 4. Each value mirrors what the
// matching command checks, so the menu refuses exactly what the command refuses.
type settingsRight int

const (
	// settingsRightAdmin needs nothing more (captcha mode, time and action).
	settingsRightAdmin settingsRight = iota
	// settingsRightAdminBotAdmin also needs the bot to be an administrator, as /captcha does.
	settingsRightAdminBotAdmin
	// settingsRightAdminFloodBot also needs the bot to be an administrator by the check
	// /setflood, /setfloodmode and /delflood make.
	settingsRightAdminFloodBot
	// settingsRightRestrict needs the creator or an administrator with can_restrict_members.
	settingsRightRestrict
)

// settingsRightRefusal returns the locale key of the refusal when the press lacks the
// right, and whether it is an alert (the page is not edited) or a notice line on the
// redrawn page. It returns "" when the right is held. The bot checks use the commands' own
// cached predicates, never a live bot lookup, so the menu refuses exactly what the command
// refuses.
func settingsRightRefusal(p *settingsPress, right settingsRight) (key string, alert bool) {
	switch right {
	case settingsRightAdmin:
		return "", false
	case settingsRightAdminBotAdmin:
		if !chat_status.IsBotAdmin(p.b, nil, p.group) {
			return "chat_status_bot_not_admin", false
		}
		return "", false
	case settingsRightAdminFloodBot:
		if !chat_status.IsUserAdmin(p.b, p.group.Id, p.b.Id) {
			return "connections_is_user_connected_bot_not_admin", false
		}
		return "", false
	case settingsRightRestrict:
		switch staffIssuerSkipReason(p.member) {
		case "":
			return "", false
		case staffReasonSkipIssuerNoRight:
			return "settings_no_restrict", true
		}
		return "settings_not_admin", true
	}
	// An unknown right fails closed.
	return "settings_not_admin", true
}

// settingsPress is one verified press of a settings button.
type settingsPress struct {
	b      *gotgbot.Bot
	ctx    *ext.Context
	query  *gotgbot.CallbackQuery
	fields map[string]string
	action string
	// private is true for a menu opened in the presser's private chat.
	private bool
	// group is the group being configured: the message's chat for a group menu, the chat
	// named by c for a private one.
	group *gotgbot.Chat
	// opener is the user ID u of a group menu (0 for a private one).
	opener int64
	// member is the presser's live getChatMember answer in the group.
	member gotgbot.MergedChatMember
	tr     *i18n.Translator
	// answered is set once the callback query was answered.
	answered bool
}

// answer answers the callback query, once. A second call is ignored, so a press is
// answered exactly once whatever path it takes.
func (p *settingsPress) answer(text string, alert bool) {
	if p.answered {
		return
	}
	p.answered = true
	answerStaffCallback(p.b, p.query, text, alert)
}

// edit replaces the text and keyboard of the menu message. Telegram's "message is not
// modified" is success; any other failure is logged once and never retried or undone.
func (p *settingsPress) edit(s settingsScreen) {
	opts := &gotgbot.EditMessageTextOpts{
		Text:               s.Text,
		ParseMode:          formatting.HTML,
		LinkPreviewOptions: &gotgbot.LinkPreviewOptions{IsDisabled: true},
	}
	if len(s.Keyboard.InlineKeyboard) > 0 {
		opts.ReplyMarkup = s.Keyboard
	}
	if _, _, err := p.query.Message.EditText(p.b, opts); err != nil && !isMessageNotModified(err) {
		chat := p.query.Message.GetChat()
		log.Warnf("[Settings] edit of message %d in chat %d failed: %v", p.query.Message.GetMessageId(), chat.Id, err)
	}
}

// cannotOpen answers the alert and edits the menu to the cannot-open note with no
// keyboard, so a menu that cannot render never leaves a dead button.
func (p *settingsPress) cannotOpen() {
	p.answer(settingsText(p.tr, "settings_cannot_open_alert"), true)
	p.edit(settingsScreen{Text: settingsText(p.tr, "settings_cannot_open")})
}

// ownerKV is the owner field of this menu's buttons and its value.
func (p *settingsPress) ownerKV() (key, value string) {
	if p.private {
		return "c", strconv.FormatInt(p.group.Id, 10)
	}
	return "u", strconv.FormatInt(p.opener, 10)
}

// owner is the owner field of this menu's buttons as a field map.
func (p *settingsPress) owner() map[string]string {
	key, value := p.ownerKV()
	return map[string]string{key: value}
}

// view is the settingsView of the menu this press belongs to.
func (p *settingsPress) view() settingsView {
	key, value := p.ownerKV()
	return settingsView{
		Tr:         p.tr,
		Private:    p.private,
		OwnerKey:   key,
		OwnerValue: value,
		GroupID:    p.group.Id,
		GroupTitle: p.group.Title,
		OpenerName: staffFullName(&p.query.From),
		Now:        time.Now(),
		Creator:    p.isGroupCreator(),
	}
}

// isGroupCreator reports whether this is a private menu whose presser is, by the live member
// record every press already fetched, the creator of the group being configured. It makes no
// call of its own.
func (p *settingsPress) isGroupCreator() bool {
	return p.private && p.member.Status == gotgbot.ChatMemberStatusCreator
}

// settingsMainScreen draws the Main page for a press.
func settingsMainScreen(p *settingsPress, notice string) settingsScreen {
	return renderSettingsMain(p.view(), settingsMainEntries(p.b, p.tr, p.group), notice)
}

// settingsCheckFields reports whether the fields of a callback are exactly those the action
// takes: the action code, exactly one of c and u, and the action's own fields. Anything
// else, an unknown key included, is refused before any I/O.
func settingsCheckFields(action string, fields map[string]string) bool {
	_, hasC := fields["c"]
	_, hasU := fields["u"]
	if hasC == hasU {
		return false
	}
	owner := "u"
	if hasC {
		owner = "c"
	}
	allowed := map[string]bool{"a": true, owner: true}
	required := []string{"a", owner}
	switch action {
	case settingsActOpenHere:
		if owner != "u" {
			return false
		}
	case settingsActOpen, settingsActCancel:
		allowed["p"] = true
		required = append(required, "p")
	case settingsActBack, settingsActClose:
	case settingsActToggle, settingsActSet:
		for _, key := range []string{"p", "t", "v"} {
			allowed[key] = true
			required = append(required, key)
		}
	default:
		entry, ok := settingsActions[action]
		if !ok {
			return false
		}
		for _, key := range entry.action.Fields {
			allowed[key] = true
		}
	}
	for key := range fields {
		if !allowed[key] {
			return false
		}
	}
	for _, key := range required {
		if fields[key] == "" {
			return false
		}
	}
	return true
}

// newSettingsPress is the press of a callback before any of its checks ran.
func newSettingsPress(b *gotgbot.Bot, ctx *ext.Context, query *gotgbot.CallbackQuery) *settingsPress {
	return &settingsPress{b: b, ctx: ctx, query: query, tr: ctxTr(ctx)}
}

// verify runs steps 1 to 4 of the press pipeline in that order and returns p, or nil after
// answering the refusal. A refusal of steps 1 to 3 makes no Telegram call; step 4 is the live
// getChatMember of the presser in the group being configured. Nothing is read or written for
// a press that does not pass.
func (p *settingsPress) verify() *settingsPress {
	b, query := p.b, p.query
	refuse := func(key string) *settingsPress {
		p.answer(settingsText(p.tr, key), true)
		return nil
	}

	// 1. Decode and structure, no I/O.
	decoded, ok := decodeCallbackData(query.Data, settingsCallbackNamespace)
	if !ok {
		return refuse("common_callback_invalid_request")
	}
	if query.Message == nil || query.Message.GetDate() == 0 {
		return refuse("common_callback_message_unavailable")
	}
	p.fields = decoded.Fields
	p.action = p.fields["a"]
	if !settingsCheckFields(p.action, p.fields) {
		return refuse("common_callback_invalid_request")
	}
	messageChat := query.Message.GetChat()
	var privateGroupID int64
	if raw, isPrivate := p.fields["c"]; isPrivate {
		id, valid := parseStaffChatIDArg(raw)
		if !valid || strconv.FormatInt(id, 10) != raw || messageChat.Type != "private" {
			return refuse("common_callback_invalid_request")
		}
		p.private = true
		privateGroupID = id
	} else {
		asker, valid := lockdownParseAsker(p.fields)
		if !valid || strconv.FormatInt(asker, 10) != p.fields["u"] || messageChat.Type == "private" {
			return refuse("common_callback_invalid_request")
		}
		p.opener = asker
	}

	// 2. A Telegram service identity is refused before any lookup.
	if staffServiceUserIDs[query.From.Id] {
		return refuse("lockdown_alert_post_as_yourself")
	}

	// 3. A group menu answers only the admin who opened it.
	if !p.private && query.From.Id != p.opener {
		return refuse("settings_not_your_menu")
	}

	// 4. One live getChatMember of the presser in the group being configured. The admin
	// cache never authorizes, and a lookup that fails refuses.
	groupID := messageChat.Id
	if p.private {
		groupID = privateGroupID
	}
	member, err := lockdownLiveMember(b, groupID, query.From.Id)
	if err != nil {
		log.Warnf("[Settings] live check of user %d in chat %d failed: %v", query.From.Id, groupID, err)
		if p.private && chat_status.IsBotMissingError(err) {
			// The bot left the group, or the group is gone: this private menu can never
			// work again, so it closes itself instead of failing on every press (D-11).
			p.cannotOpen()
			return nil
		}
		return refuse("settings_check_failed")
	}
	if member.Status != gotgbot.ChatMemberStatusCreator && member.Status != gotgbot.ChatMemberStatusAdministrator {
		return refuse("settings_not_admin")
	}
	p.member = member

	if !p.private {
		p.group = &messageChat
		return p
	}
	lookupCtx, cancel := context.WithTimeout(context.Background(), lockdownLiveCheckTimeout)
	defer cancel()
	full, err := b.GetChatWithContext(lookupCtx, groupID, nil)
	if err != nil || full == nil {
		log.Warnf("[Settings] getChat of chat %d for a private menu failed: %v", groupID, err)
		if err != nil && chat_status.IsBotMissingError(err) {
			p.cannotOpen()
			return nil
		}
		return refuse("settings_check_failed")
	}
	group := full.ToChat()
	// Callback data is not authenticated by Telegram, so c can name any chat the presser
	// administers. Only a group or supergroup is configured here, as the deep link does.
	if group.Type != "group" && group.Type != "supergroup" {
		p.cannotOpen()
		return nil
	}
	p.group = &group
	return p
}

// settingsCallback dispatches the settings| buttons. Every path answers the query exactly
// once and returns ext.EndGroups.
func (m moduleStruct) settingsCallback(b *gotgbot.Bot, ctx *ext.Context) error {
	defer error_handling.RecoverFromPanic("settingsCallback", "Settings")

	query, ok := callbackQueryFromContext(ctx)
	if !ok {
		return ext.EndGroups
	}
	press := newSettingsPress(b, ctx, query)
	// Deferred after the recover above, so it runs first: a panic anywhere below, in the
	// checks or in a page, still answers the query once instead of leaving the client's
	// spinner running until Telegram times it out. A press that already answered is a no-op.
	defer press.answer("", false)
	p := press.verify()
	if p == nil {
		return ext.EndGroups
	}
	p.dispatch()
	// A handler that did not answer (a normal success) answers empty now.
	p.answer("", false)
	return ext.EndGroups
}

// dispatch runs the verified press.
func (p *settingsPress) dispatch() {
	switch p.action {
	case settingsActOpenHere, settingsActBack:
		p.edit(settingsMainScreen(p, ""))
	case settingsActOpen, settingsActCancel:
		pg, ok := settingsPageByKey(p.fields["p"])
		if !ok {
			p.answer(settingsText(p.tr, "common_callback_invalid_request"), true)
			return
		}
		p.openPage(pg, "")
	case settingsActClose:
		p.closeMenu()
	case settingsActToggle, settingsActSet:
		p.runControl()
	default:
		p.runPageAction()
	}
}

// openPage draws a page, or the cannot-open edit when its data cannot be read.
func (p *settingsPress) openPage(pg settingsPage, notice string) {
	screen, err := pg.Open(p, notice)
	if err != nil {
		log.Warnf("[Settings] page %q of chat %d could not be read: %v", pg.Key, p.group.Id, err)
		p.cannotOpen()
		return
	}
	p.edit(screen)
}

// closeMenu deletes the menu message and, when that fails (a private message older than 48
// hours, a missing right), edits it to the closed note with no keyboard.
func (p *settingsPress) closeMenu() {
	deleted, err := p.query.Message.Delete(p.b, nil)
	if err == nil && deleted {
		return
	}
	log.Warnf("[Settings] delete of the menu message in chat %d failed (%v), editing it instead", p.query.Message.GetChat().Id, err)
	p.edit(settingsScreen{Text: settingsText(p.tr, "settings_closed")})
}

// checkRight judges the right of a control or action. A refusal that is an alert answers
// it and edits nothing; one that is a notice answers empty and redraws page with the
// warning line. It returns true when the right is held.
func (p *settingsPress) checkRight(right settingsRight, pageKey string) bool {
	key, alert := settingsRightRefusal(p, right)
	if key == "" {
		return true
	}
	text := settingsText(p.tr, key)
	if alert {
		p.answer(text, true)
		return false
	}
	p.answer("", false)
	if pg, ok := settingsPageByKey(pageKey); ok {
		p.openPage(pg, settingsNoticeWarn(text))
	}
	return false
}

// runControl handles tg and st: it finds the page and control, checks the control's right
// and applies the absolute value.
func (p *settingsPress) runControl() {
	pg, ok := settingsPageByKey(p.fields["p"])
	if !ok {
		p.answer(settingsText(p.tr, "common_callback_invalid_request"), true)
		return
	}
	control, ok := pg.Controls[p.fields["t"]]
	if !ok || control.Set == nil {
		p.answer(settingsText(p.tr, "common_callback_invalid_request"), true)
		return
	}
	if !p.checkRight(control.Right, pg.Key) {
		return
	}
	if screen := control.Set(p, p.fields["v"]); screen != nil {
		p.edit(*screen)
	}
}

// runPageAction handles an action code a page declared.
func (p *settingsPress) runPageAction() {
	entry, ok := settingsActions[p.action]
	if !ok || entry.action.Run == nil {
		p.answer(settingsText(p.tr, "common_callback_invalid_request"), true)
		return
	}
	if !p.checkRight(entry.action.Right, entry.pageKey) {
		return
	}
	if screen := entry.action.Run(p); screen != nil {
		p.edit(*screen)
	}
}
