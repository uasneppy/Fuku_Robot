package modules

import (
	"context"
	"errors"
	"strconv"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db/lang"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/error_handling"
	"github.com/uasneppy/fuku_robot/fuku/utils/formatting"
)

// settingsInputGroup is the handler group of the typed-reply door. Group 0 holds the commands,
// so a command is judged before the door sees the message, and group -1 the users tracker; no
// group between 0 and 2 is used, and every group that could end a private update comes later.
const settingsInputGroup = 1

// This file is the typed-reply door of the message editor (D-03 to D-07, D-12). After the owner
// presses Set, the bot waits for that person's next private message and saves it as the part.
// The door acts only for a person who has a wait (or, for one reply, a marker of a wait that
// ended); everyone else's private chat with the bot is untouched. A reply is judged like a
// press: a live getChatMember, the owner gate, the right of the part and a live getChat, and
// only then is the message checked, the wait claimed and the part written through the apply
// function of the command it mirrors. No log line carries the message text, a caption, a
// button name, a link or a file ID.

// settingsInputIsPrivate is the door's handler filter: a message of a private chat.
func settingsInputIsPrivate(msg *gotgbot.Message) bool {
	return msg != nil && msg.Chat.Type == "private"
}

// settingsInputLooksLikeCommand reports whether the message is a command: a bot_command entity
// at offset 0 of its text or caption, or a slash followed by a character that is not a space
// (which gotgbot's own command handler also runs). A command is never taken as content.
func settingsInputLooksLikeCommand(msg *gotgbot.Message) bool {
	for _, entity := range msg.GetEntities() {
		if entity.Type == "bot_command" && entity.Offset == 0 {
			return true
		}
	}
	text := msg.GetText()
	if len(text) < 2 || text[0] != '/' {
		return false
	}
	next, _ := utf8.DecodeRuneInString(text[1:])
	return !unicode.IsSpace(next)
}

// settingsInputCandidate reports whether the message may be the reply to a wait: a new private
// message whose sender is the chat itself (not the bot and not a Telegram service identity),
// that carries no rich message and is not a command.
func settingsInputCandidate(msg *gotgbot.Message) bool {
	return msg != nil && msg.From != nil &&
		msg.From.Id == msg.Chat.Id && !msg.From.IsBot && !staffServiceUserIDs[msg.From.Id] &&
		msg.RichMessage == nil && !settingsInputLooksLikeCommand(msg)
}

// settingsInputReply is one candidate message being judged against its sender's wait.
type settingsInputReply struct {
	b      *gotgbot.Bot
	ctx    *ext.Context
	msg    *gotgbot.Message
	user   *gotgbot.User
	tr     *i18n.Translator
	wait   *settingsInputWait
	def    settingsEditPageDef
	member gotgbot.MergedChatMember
	group  gotgbot.Chat
	// list is the table of the list page whose wait is being run (nil for an editor page).
	list *settingsListDef
}

// settingsInput is the typed-reply door. It returns ext.ContinueGroups for a message that is
// not its own (no wait and no marker, a Redis problem) and ext.EndGroups for everything it
// consumed, a recovered panic included.
func (m moduleStruct) settingsInput(b *gotgbot.Bot, ctx *ext.Context) (ret error) {
	defer error_handling.RecoverFromPanic("settingsInput", "Settings")
	msg := ctx.EffectiveMessage
	if !settingsInputCandidate(msg) {
		return ext.ContinueGroups
	}
	wait, marker, err := settingsInputRead(context.Background(), msg.From.Id)
	if err != nil {
		if !errors.Is(err, errSettingsInputUnavailable) {
			log.Warnf("[Settings] reading the typed-reply wait of user %d failed: %v", msg.From.Id, err)
		}
		return ext.ContinueGroups
	}
	if wait == nil && marker == "" {
		return ext.ContinueGroups
	}
	r := &settingsInputReply{b: b, ctx: ctx, msg: msg, user: msg.From, tr: ctxTr(ctx), wait: wait}
	if wait == nil {
		if !r.waitEnded() {
			return ext.ContinueGroups
		}
		return ext.EndGroups
	}
	// From here the message is consumed: a panic below must not hand it on, so the result a
	// recovered panic leaves behind is EndGroups.
	ret = ext.EndGroups
	if !r.albumFirst() {
		return ext.EndGroups
	}
	r.run()
	return ext.EndGroups
}

// settingsInputIsAlbum reports whether the message belongs to an album: a media group ID of 1 to
// 20 ASCII digits. Anything else is a single item and never forms an album key.
func settingsInputIsAlbum(msg *gotgbot.Message) bool {
	return settingsInputAlbumIDRe.MatchString(msg.MediaGroupId)
}

// albumFirst is the album gate (D-13). Telegram delivers the items of an album as separate
// updates, so with a live wait only the first item to take the album memory (SET NX, 30 s) goes
// on to the live checks and the claim: the others get no reply and no Telegram call, which also
// keeps a refused album to one refusal. A message that is no album, and a first item, go on; a
// Redis error ends the update with a log line and no second write.
func (r *settingsInputReply) albumFirst() bool {
	if !settingsInputIsAlbum(r.msg) {
		return true
	}
	first, err := settingsInputAlbumFirst(context.Background(), r.user.Id, r.msg.MediaGroupId)
	if err != nil {
		log.Warnf("[Settings] the album memory of user %d could not be written: %v", r.user.Id, err)
		return false
	}
	return first
}

// settingsInputPartWordKey is the locale key of the word for a part, as the wait-ended note names
// it.
func settingsInputPartWordKey(part string) string {
	switch part {
	case settingsEditPartMedia:
		return "settings_edit_part_media"
	case settingsEditPartButtons:
		return "settings_edit_part_buttons"
	}
	return "settings_edit_part_text"
}

// waitEnded answers a reply that came after the wait ended: the marker is taken with GETDEL, so
// two deliveries answer once, and a taken marker that reads as a wait is told with one plain reply
// naming the part. Nothing is saved. It reports whether the message was answered; a marker
// someone else took first, or one that does not parse, leaves the message an ordinary one.
func (r *settingsInputReply) waitEnded() bool {
	taken, ok, err := settingsInputTakeMarker(context.Background(), r.user.Id)
	if err != nil {
		if !errors.Is(err, errSettingsInputUnavailable) {
			log.Warnf("[Settings] taking the wait marker of user %d failed: %v", r.user.Id, err)
		}
		return false
	}
	if !ok {
		return false
	}
	_, page, part, valid := settingsInputParseMarker(taken)
	if !valid {
		return false
	}
	if listPart := settingsListPartByCode(page, part); listPart != nil {
		r.plain(r.text("settings_entries_wait_ended_note", i18n.TranslationParams{"part": r.text(listPart.WordKey)}))
	} else {
		r.plain(r.text("settings_edit_wait_ended_note", i18n.TranslationParams{"part": r.text(settingsInputPartWordKey(part))}))
	}
	log.Debugf("[Settings] user %d replied after the wait for the %s part ended", r.user.Id, part)
	return true
}

// run judges the reply: authority first (as a press), then the check, the claim and the write.
func (r *settingsInputReply) run() {
	// A wait whose page is a list page is run by the list engine (M43); the editor's path below is unchanged.
	if listDef, ok := settingsListByPage(r.wait.Page); ok {
		part := settingsListPartByCode(r.wait.Page, r.wait.Part)
		if part == nil {
			r.end("settings_check_failed")
			return
		}
		r.runList(listDef, *part)
		return
	}
	def, ok := settingsEditPageByKey(r.wait.Page)
	if !ok {
		r.end("settings_check_failed")
		return
	}
	r.def = def
	if !r.authorize(def.EditRight) {
		return
	}
	loadCtx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()
	state, err := settingsEditLoad(loadCtx, def, r.wait.GroupID)
	if err != nil {
		log.Warnf("[Settings] %s page of chat %d could not be read for a typed reply: %v", def.Key, r.wait.GroupID, err)
		r.keep("settings_check_failed")
		return
	}
	verdict := settingsEditCheck(settingsEditCheckInput{
		Def:        def,
		Part:       r.wait.Part,
		Msg:        r.msg,
		State:      state,
		Owner:      *r.user,
		GroupTitle: r.group.Title,
		Language:   lang.GetLanguage(r.ctx),
	})
	if !verdict.OK {
		r.refuse(r.refusalText(verdict))
		return
	}

	// A claim lost to another delivery of the same message (or a Cancel) answers nothing.
	claim, claimed := r.claimWait()
	if !claimed {
		return
	}
	if !r.apply(verdict) {
		log.Errorf("[Settings] the typed %s of chat %d could not be saved for user %d", r.wait.Part, r.wait.GroupID, r.user.Id)
		r.releaseWait(claim)
		r.refuse(r.text("common_settings_save_failed"))
		return
	}
	r.finishWait(claim)
	log.Infof("[Settings] user %d saved the typed %s of the %s page of chat %d", r.user.Id, r.wait.Part, def.Key, r.wait.GroupID)
	r.saved(verdict)
	r.closePrompt()
}

// authorize makes the live checks of a press, in order: the sender's member record, the owner
// gate, the right of the part (the bot's first half, the member's part, the bot's second half, as a
// press judges it) and the group itself. It answers the refusal and returns false when the reply may
// not go on. A loss ends the wait; an answer that cannot be established keeps it.
func (r *settingsInputReply) authorize(right settingsRight) bool {
	member, err := lockdownLiveMember(r.b, r.wait.GroupID, r.user.Id)
	if err != nil {
		log.Warnf("[Settings] live check of user %d in chat %d for a typed reply failed: %v", r.user.Id, r.wait.GroupID, err)
		if isTelegramClientError(err) {
			r.end("settings_edit_refuse_access_lost")
		} else {
			r.keep("settings_check_failed")
		}
		return false
	}
	if member.Status != gotgbot.ChatMemberStatusCreator && member.Status != gotgbot.ChatMemberStatusAdministrator {
		r.end("settings_edit_refuse_access_lost")
		return false
	}
	r.member = member

	switch settingsGate(r.b, r.wait.GroupID, *r.user, member) {
	case settingsGateIn:
	case settingsGateOwnerOnly:
		r.end("settings_gate_owner_only")
		return false
	default:
		r.keep("settings_check_failed")
		return false
	}

	botGroup := &gotgbot.Chat{Id: r.wait.GroupID, Type: "supergroup"}
	if key := settingsRightBotFirst(r.b, botGroup, right); key != "" {
		r.botCheck(key)
		return false
	}
	if key, _ := settingsRightRefusalFor(member, right); key != "" {
		r.end(key)
		return false
	}
	if key := settingsRightBotAfter(r.b, botGroup, right, true); key != "" {
		r.botCheck(key)
		return false
	}

	lookupCtx, cancel := context.WithTimeout(context.Background(), lockdownLiveCheckTimeout)
	defer cancel()
	full, err := r.b.GetChatWithContext(lookupCtx, r.wait.GroupID, nil)
	if err != nil || full == nil {
		log.Warnf("[Settings] getChat of chat %d for a typed reply failed: %v", r.wait.GroupID, err)
		if err != nil && isTelegramClientError(err) {
			r.end("settings_edit_refuse_access_lost")
		} else {
			r.keep("settings_check_failed")
		}
		return false
	}
	group := full.ToChat()
	if group.Type != "group" && group.Type != "supergroup" {
		r.end("settings_edit_refuse_access_lost")
		return false
	}
	r.group = group
	return true
}

// settingsBotKeyPasses is what a live record of the bot must show for the bot part that refused with key to have been
// a stale answer: the restrict refusal needs an administrator with can_restrict_members, every other bot refusal an
// administrator.
func settingsBotKeyPasses(key string) func(gotgbot.MergedChatMember) bool {
	if key == "chat_status_bot_restrict_group_error" {
		return func(bot gotgbot.MergedChatMember) bool {
			return bot.Status == gotgbot.ChatMemberStatusAdministrator && bot.CanRestrictMembers
		}
	}
	return func(bot gotgbot.MergedChatMember) bool { return bot.Status == gotgbot.ChatMemberStatusAdministrator }
}

// botCheck classifies a bot part that the commands' own cached predicate refused (key is its refusal), never looser
// than the command. Those predicates return a bare bool, so a false is either a bot that lacks the right or a lookup
// that failed; one live getChatMember of the bot tells the two apart. A definite answer that the bot lacks the right,
// or a Telegram client error, ends the wait with key; an error that cannot be established (429, 5xx, transport,
// timeout) and a live record that shows the bot can (the predicate's answer was stale) keep it with
// settings_check_failed. It answers the refusal itself, so the caller returns false afterwards.
func (r *settingsInputReply) botCheck(key string) {
	bot, err := lockdownLiveMember(r.b, r.wait.GroupID, r.b.Id)
	if err != nil {
		log.Warnf("[Settings] live check of the bot in chat %d for a typed reply failed: %v", r.wait.GroupID, err)
		if isTelegramClientError(err) {
			r.end(key)
		} else {
			r.keep("settings_check_failed")
		}
		return
	}
	if settingsBotKeyPasses(key)(bot) {
		r.keep("settings_check_failed")
		return
	}
	r.end(key)
}

// text translates key in the sender's language.
func (r *settingsInputReply) text(key string, params ...i18n.TranslationParams) string {
	return settingsText(r.tr, key, params...)
}

// refusalText is the finished text of a refused check: the commands' own words when the verdict
// carries them, else the translated reason with its parameters.
func (r *settingsInputReply) refusalText(verdict settingsEditVerdict) string {
	if verdict.RefusalText != "" {
		return verdict.RefusalText
	}
	return r.text(verdict.RefusalKey, verdict.RefusalParams)
}

// plain replies to the message with text and nothing else.
func (r *settingsInputReply) plain(text string) {
	r.send(text, nil, true)
}

// send posts text to the private chat, as a reply to the message when reply is set. It returns
// the ID of the message sent, 0 when nothing was sent.
func (r *settingsInputReply) send(text string, keyboard *gotgbot.InlineKeyboardMarkup, reply bool) int64 {
	if text == "" {
		return 0
	}
	opts := &gotgbot.SendMessageOpts{
		ParseMode:          formatting.HTML,
		LinkPreviewOptions: &gotgbot.LinkPreviewOptions{IsDisabled: true},
	}
	if reply {
		opts.ReplyParameters = &gotgbot.ReplyParameters{MessageId: r.msg.MessageId, AllowSendingWithoutReply: true}
	}
	if keyboard != nil && len(keyboard.InlineKeyboard) > 0 {
		opts.ReplyMarkup = *keyboard
	}
	sent, err := r.b.SendMessage(r.msg.Chat.Id, text, opts)
	if err != nil {
		log.Warnf("[Settings] answering the typed reply of user %d failed: %v", r.user.Id, err)
		return 0
	}
	if sent == nil {
		return 0
	}
	return sent.MessageId
}

// keep answers key and leaves the wait as it is: the answer could not be established, so
// nothing is lost and the person may send the message again.
func (r *settingsInputReply) keep(key string) {
	r.plain(r.text(key))
}

// settingsInputEndKeysSayStopped are the end reasons whose own text already says the bot stopped
// waiting; every other reason is followed by settings_edit_wait_stopped, so the person knows the
// next message will not be read.
var settingsInputEndKeysSayStopped = map[string]bool{"settings_edit_refuse_access_lost": true}

// end deletes the wait and its marker (only this wait: a newer wait for another group, page or
// part is left alone) and answers key, then a blank line and the line that says the wait ended
// unless the reason already does. The person has been told, so the marker goes too.
func (r *settingsInputReply) end(key string) {
	r.cancelWait()
	text := r.text(key)
	if !settingsInputEndKeysSayStopped[key] {
		text += "\n\n" + r.text("settings_edit_wait_stopped")
	}
	r.plain(text)
}

// endWith is end for a reason that is already finished text (a refusal the page worded, with the user's entry
// spliced in): it cancels this wait, then answers the text, a blank line and the line that says the wait ended.
func (r *settingsInputReply) endWith(text string) {
	r.cancelWait()
	r.plain(r.endText(text))
}

// endText is the answer of a wait that ended: the reason, a blank line and settings_edit_wait_stopped.
func (r *settingsInputReply) endText(reason string) string {
	return reason + "\n\n" + r.text("settings_edit_wait_stopped")
}

// cancelWait deletes this wait and its marker, logging a failure only.
func (r *settingsInputReply) cancelWait() {
	outcome, err := settingsInputCancel(context.Background(), r.user.Id, r.wait.GroupID, r.wait.Page, r.wait.Part, r.wait.PromptMsgID)
	if err != nil && !errors.Is(err, errSettingsInputUnavailable) {
		log.Warnf("[Settings] ending the wait of user %d failed: %v", r.user.Id, err)
	}
	log.Debugf("[Settings] the wait of user %d ended with outcome %d", r.user.Id, int(outcome))
}

// view is the private menu of the group being configured, as a press builds it.
func (r *settingsInputReply) view() settingsView {
	owner := r.member.Status == gotgbot.ChatMemberStatusCreator
	return settingsView{
		Tr:         r.tr,
		Private:    true,
		OwnerKey:   "c",
		OwnerValue: strconv.FormatInt(r.group.Id, 10),
		GroupID:    r.group.Id,
		GroupTitle: r.group.Title,
		OpenerName: staffFullName(r.user),
		Now:        time.Now(),
		Creator:    owner,
		Owner:      owner,
	}
}

// renderSettingsInputRefusal draws the reply to a message the check refused: the reason, a blank
// line, the still-waiting line and one Cancel button. It is pure, so a test can render exactly
// what the door sends.
func renderSettingsInputRefusal(v settingsView, def settingsEditPageDef, part, reason string) settingsScreen {
	return renderSettingsInputRefusalCode(v, def.CancelCode, part, reason)
}

// renderSettingsInputRefusalCode is renderSettingsInputRefusal for a page that is named by its Cancel action code
// alone (a list page).
func renderSettingsInputRefusalCode(v settingsView, cancelCode, part, reason string) settingsScreen {
	k := newSettingsKeyboard(v)
	if cancel, ok := k.button(settingsText(v.Tr, "settings_btn_edit_cancel"), map[string]string{"a": cancelCode, "t": part}); ok {
		k.row(cancel)
	}
	return k.screen(reason + "\n\n" + settingsText(v.Tr, "settings_edit_still_waiting"))
}

// refuse answers a reply the check refused, as a reply to the message. The wait is untouched, its
// time not extended.
func (r *settingsInputReply) refuse(reason string) {
	cancelCode := r.def.CancelCode
	if r.list != nil {
		cancelCode = r.list.CancelCode
	}
	screen := renderSettingsInputRefusalCode(r.view(), cancelCode, r.wait.Part, reason)
	id := r.send(screen.Text, &screen.Keyboard, true)
	// The refusal carries a Cancel button of its own: note it on the wait, so a press on it ends
	// this wait while a stale prompt's button still cannot.
	if err := settingsInputNoteRefusal(context.Background(), r.user.Id, r.wait.Nonce, id); err != nil {
		log.Warnf("[Settings] noting the refusal reply of user %d failed: %v", r.user.Id, err)
	}
}

// apply writes the checked message through the apply function of the part's command and
// reports whether it was stored.
func (r *settingsInputReply) apply(verdict settingsEditVerdict) bool {
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()
	groupID := r.wait.GroupID
	if r.def.Kind == settingsEditRules {
		return r.applyRules(ctx, groupID, verdict)
	}
	isWelcome := r.def.Kind == settingsEditWelcome
	var result greetingApplyResult
	switch r.wait.Part {
	case settingsEditPartText:
		result = applyGreetingText(ctx, groupID, isWelcome, verdict.Text)
	case settingsEditPartMedia:
		result = applyGreetingMedia(ctx, groupID, isWelcome, verdict.FileID, verdict.DataType)
	case settingsEditPartButtons:
		result = applyGreetingButtons(ctx, groupID, isWelcome, verdict.Buttons)
	default:
		return false
	}
	return result.Outcome != greetingSaveFailed
}

// applyRules writes a checked Rules message through the apply function of its part, the ones the
// /setrules command and the Remove buttons use, and reports whether it was stored.
func (r *settingsInputReply) applyRules(ctx context.Context, groupID int64, verdict settingsEditVerdict) bool {
	var result rulesApplyResult
	switch r.wait.Part {
	case settingsEditPartText:
		result = applyRulesText(ctx, groupID, verdict.Text)
	case settingsEditPartMedia:
		result = applyRulesMedia(ctx, groupID, verdict.FileID, verdict.DataType)
	case settingsEditPartButtons:
		result = applyRulesButtons(ctx, groupID, verdict.Buttons)
	default:
		return false
	}
	return result.Outcome != greetingSaveFailed
}

// settingsInputSavedKey is the confirmation line of a saved part.
func settingsInputSavedKey(part string) string {
	switch part {
	case settingsEditPartMedia:
		return "settings_edit_saved_media"
	case settingsEditPartButtons:
		return "settings_edit_saved_buttons"
	}
	return "settings_edit_saved_text"
}

// settingsInputSavedNotice is the notice of the saved page: the confirmation of the part, the
// caption note when a caption was not used and the album note when the saved message was cut from
// an album.
func settingsInputSavedNotice(tr *i18n.Translator, part string, captionIgnored, album bool) string {
	notice := settingsNoticeOK(settingsText(tr, settingsInputSavedKey(part)))
	if captionIgnored {
		notice += "\n" + settingsText(tr, "settings_edit_caption_ignored")
	}
	if album {
		notice += "\n" + settingsText(tr, "settings_edit_album_cut")
	}
	return notice
}

// saved sends the saved page as a new message in the private chat: the page as it now stands,
// with the confirmation (and the caption and album notes) as its notice. When the page cannot be
// read the confirmation alone is sent.
func (r *settingsInputReply) saved(verdict settingsEditVerdict) {
	notice := settingsInputSavedNotice(r.tr, r.wait.Part, verdict.CaptionIgnored, settingsInputIsAlbum(r.msg))
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()
	state, err := settingsEditLoad(ctx, r.def, r.wait.GroupID)
	if err != nil {
		log.Warnf("[Settings] the %s page of chat %d could not be read after a typed save: %v", r.def.Key, r.wait.GroupID, err)
		r.send(notice, nil, false)
		return
	}
	screen := renderSettingsEditor(r.view(), r.def, state, settingsEditScreenOpts{Notice: notice})
	r.send(screen.Text, &screen.Keyboard, false)
}

// closePrompt edits the waiting prompt to the closed line with no keyboard, best effort.
func (r *settingsInputReply) closePrompt() {
	r.editPrompt("settings_edit_prompt_closed")
}

// editPrompt edits the waiting prompt message to the text of key with no keyboard, best effort.
func (r *settingsInputReply) editPrompt(key string) {
	if r.wait.PromptMsgID == 0 {
		return
	}
	_, _, err := r.b.EditMessageText(&gotgbot.EditMessageTextOpts{
		ChatId:    r.msg.Chat.Id,
		MessageId: r.wait.PromptMsgID,
		Text:      r.text(key),
		ParseMode: formatting.HTML,
	})
	if err != nil && !isMessageNotModified(err) {
		log.Warnf("[Settings] closing the prompt %d of user %d failed: %v", r.wait.PromptMsgID, r.user.Id, err)
	}
}
