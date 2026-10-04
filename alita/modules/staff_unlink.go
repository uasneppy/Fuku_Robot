package modules

import (
	"errors"
	"strconv"
	"strings"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/db/staff"
	"github.com/divkix/Alita_Robot/alita/i18n"
	"github.com/divkix/Alita_Robot/alita/utils/chat_status"
	"github.com/divkix/Alita_Robot/alita/utils/formatting"
	"github.com/divkix/Alita_Robot/alita/utils/helpers"
)

// staffUserToken stands in for the issuer's mention while a notice is translated,
// for the same reason as staffGroupTitleToken: a display name is user-controlled
// and the translator runs a printf-style pass over interpolated text.
const staffUserToken = "<<staff-user>>"

// staffUnlinkRefusal says why an unlink attempt did not remove the link. The
// refusals before staffUnlinkNotGroupOwner happen before the issuer is proven the
// live creator of the Staff Group, so they are answered where the command came
// from and never in the Staff Group.
type staffUnlinkRefusal int

const (
	staffUnlinkOK staffUnlinkRefusal = iota
	staffUnlinkAnonymous
	staffUnlinkNotLinked
	staffUnlinkNotStaffOwner
	staffUnlinkCheckFailedInPlace
	staffUnlinkNotGroupOwner
	staffUnlinkCheckFailed
	staffUnlinkGone
)

var unlinkStaffDesc = helpers.CommandDescriptor{
	Name:           "unlinkstaff",
	RequiredChecks: []helpers.CheckFunc{helpers.RequireGroup()},
}

// checkUnlinkAuthority proves, live, that issuer is the creator of the link's
// Staff Group and then of the linked group. verifiedStaffOwner reports whether the
// first proof succeeded: it decides where a refusal may be shown. The recorded
// owner_user_id of the link is never consulted. Both the command and the Unlink
// button run this at every press.
func checkUnlinkAuthority(
	b *gotgbot.Bot,
	issuer *gotgbot.User,
	link models.StaffGroupLink,
) (refusal staffUnlinkRefusal, verifiedStaffOwner bool) {
	switch result, _, err := chat_status.CheckOwner(b, link.StaffChatID, issuer.Id); result {
	case chat_status.OwnerUnknown:
		log.Warnf("[Staff] unlink: owner check for Staff Group %d failed: %v", link.StaffChatID, err)
		return staffUnlinkCheckFailedInPlace, false
	case chat_status.OwnerMismatch:
		return staffUnlinkNotStaffOwner, false
	}
	switch result, _, err := chat_status.CheckOwner(b, link.GroupChatID, issuer.Id); result {
	case chat_status.OwnerUnknown:
		log.Warnf("[Staff] unlink: owner check for group %d failed: %v", link.GroupChatID, err)
		return staffUnlinkCheckFailed, true
	case chat_status.OwnerMismatch:
		return staffUnlinkNotGroupOwner, true
	}
	return staffUnlinkOK, true
}

// runUnlinkGroup is the one unlink flow behind /unlinkstaff and the Unlink
// button: live creator of the Staff Group, live creator of the linked group, then
// the delete. Only the caller whose delete actually removed the row posts the
// "unlinked by" notice, and it goes to the Staff Group alone (D-06).
func runUnlinkGroup(
	b *gotgbot.Bot,
	issuer *gotgbot.User,
	link models.StaffGroupLink,
) (refusal staffUnlinkRefusal, verifiedStaffOwner bool) {
	if refusal, verifiedStaffOwner = checkUnlinkAuthority(b, issuer, link); refusal != staffUnlinkOK {
		return refusal, verifiedStaffOwner
	}
	deleted, err := staff.DeleteLink(link.ID)
	if err != nil {
		return staffUnlinkCheckFailed, true
	}
	if !deleted {
		return staffUnlinkGone, true
	}
	postUnlinkNotice(b, issuer, link)
	return staffUnlinkOK, true
}

// postUnlinkNotice tells the Staff Group, in its own language, who unlinked which
// group. A failed post is logged by sendStaffNotice and nothing is sent anywhere
// else: the linked group must keep no trace of the unlink (D-07).
func postUnlinkNotice(b *gotgbot.Bot, issuer *gotgbot.User, link models.StaffGroupLink) {
	tr := staffChatTranslator(link.StaffChatID)
	text, _ := tr.GetString("staff_notice_unlinked_by", i18n.TranslationParams{
		"group": staffGroupTitleToken,
		"user":  staffUserToken,
	})
	text = strings.Replace(text, staffGroupTitleToken, staffDisplayTitle(link.GroupTitle), 1)
	text = strings.Replace(text, staffUserToken, formatting.MentionHtml(issuer.Id, issuer.FirstName), 1)
	_ = sendStaffNotice(b, link.StaffChatID, text)
}

// staffUnlinkRefusalText maps a refusal to its message. Every case reads a
// literal locale key so make check-translations sees it. groupTitle is
// user-controlled, so it is escaped and spliced in after translation rather than
// passed through the translator. staffUnlinkOK and staffUnlinkGone have no text.
func staffUnlinkRefusalText(tr *i18n.Translator, r staffUnlinkRefusal, groupTitle string) string {
	params := i18n.TranslationParams{"group": staffGroupTitleToken}
	var text string
	switch r {
	case staffUnlinkAnonymous:
		text, _ = tr.GetString("staff_post_as_yourself")
	case staffUnlinkNotLinked:
		text, _ = tr.GetString("staff_unlink_not_linked")
	case staffUnlinkNotStaffOwner:
		text, _ = tr.GetString("staff_unlink_refuse_not_owner")
	case staffUnlinkNotGroupOwner:
		text, _ = tr.GetString("staff_unlink_refuse_not_group_owner", params)
	case staffUnlinkCheckFailed:
		text, _ = tr.GetString("staff_unlink_check_failed_group", params)
	case staffUnlinkCheckFailedInPlace:
		text, _ = tr.GetString("staff_check_failed")
	default:
		return ""
	}
	return strings.Replace(text, staffGroupTitleToken, staffDisplayTitle(groupTitle), 1)
}

// deliverUnlinkRefusal puts a refusal of the command where it belongs. Before the
// issuer is proven the Staff Group's creator it is a self-deleting reply in the
// issuing group. After that proof it is posted in the Staff Group, in its
// language; if that post fails, a self-deleting reply in the issuing group takes
// its place. A lost race (gone) says nothing.
func deliverUnlinkRefusal(
	b *gotgbot.Bot,
	c *helpers.CommandContext,
	link models.StaffGroupLink,
	refusal staffUnlinkRefusal,
	verifiedStaffOwner bool,
) {
	if refusal == staffUnlinkOK || refusal == staffUnlinkGone {
		return
	}
	if !verifiedStaffOwner {
		replySelfDeleting(b, c.Msg, staffUnlinkRefusalText(c.Tr, refusal, link.GroupTitle))
		return
	}
	staffTr := staffChatTranslator(link.StaffChatID)
	if err := sendStaffNotice(b, link.StaffChatID, staffUnlinkRefusalText(staffTr, refusal, link.GroupTitle)); err != nil {
		replySelfDeleting(b, c.Msg, staffUnlinkRefusalText(c.Tr, refusal, link.GroupTitle))
	}
}

// unlinkStaff handles /unlinkstaff in the group being unlinked. The command
// message is removed first, best effort and never gated on the result (D-07).
// Authority is the live creator of both groups; every refusal before the Staff
// Group's creator is proven stays in the issuing group and deletes itself.
func (moduleStruct) unlinkStaff(c *helpers.CommandContext) error {
	_, _ = c.Msg.Delete(c.Bot, nil)

	if helpers.IsAnonymousSender(c.Ctx, c.User) {
		replySelfDeleting(c.Bot, c.Msg, staffUnlinkRefusalText(c.Tr, staffUnlinkAnonymous, ""))
		return ext.EndGroups
	}
	link, err := staff.GetLinkOfGroupFresh(c.Chat.Id)
	if err != nil {
		replySelfDeleting(c.Bot, c.Msg, staffUnlinkRefusalText(c.Tr, staffUnlinkCheckFailedInPlace, ""))
		return ext.EndGroups
	}
	if link == nil {
		replySelfDeleting(c.Bot, c.Msg, staffUnlinkRefusalText(c.Tr, staffUnlinkNotLinked, ""))
		return ext.EndGroups
	}

	refusal, verifiedStaffOwner := runUnlinkGroup(c.Bot, c.User, *link)
	deliverUnlinkRefusal(c.Bot, c, *link, refusal, verifiedStaffOwner)
	return ext.EndGroups
}

// errStaffPanelGone means the message a button sits on is no longer a Staff
// Group panel: the message is missing, or its chat stopped being a Staff Group.
var errStaffPanelGone = errors.New("staff: no panel to render")

// parseStaffLinkID parses the "l" field of an Unlink button strictly: decimal
// digits only (no sign, no spaces), non-zero, and within the range of a uint.
func parseStaffLinkID(raw string) (uint, bool) {
	id, err := strconv.ParseUint(raw, 10, 0)
	if err != nil || id == 0 {
		return 0, false
	}
	return uint(id), true
}

// editStaffMessage replaces the text and keyboard of the message a button sits
// on. Telegram's "message is not modified" answer is not a failure: the message
// already shows what was asked for.
func editStaffMessage(
	b *gotgbot.Bot,
	msg gotgbot.MaybeInaccessibleMessage,
	text string,
	keyboard gotgbot.InlineKeyboardMarkup,
) error {
	opts := &gotgbot.EditMessageTextOpts{
		Text:               text,
		ParseMode:          formatting.HTML,
		LinkPreviewOptions: &gotgbot.LinkPreviewOptions{IsDisabled: true},
	}
	if len(keyboard.InlineKeyboard) > 0 {
		opts.ReplyMarkup = keyboard
	}
	_, _, err := msg.EditText(b, opts)
	if err != nil && !strings.Contains(err.Error(), "message is not modified") {
		log.Errorf("[Staff] edit staff panel message: %v", err)
		return err
	}
	return nil
}

// staffRerenderPanel rebuilds the /staff panel of the chat msg sits in, from the
// database, and edits msg to show it. It returns errStaffPanelGone, touching
// nothing, when there is no message or its chat is not a Staff Group; the caller
// answers the callback.
func staffRerenderPanel(b *gotgbot.Bot, msg gotgbot.MaybeInaccessibleMessage, tr *i18n.Translator) error {
	if msg == nil {
		return errStaffPanelGone
	}
	group, err := staff.GetStaffGroupFresh(msg.GetChat().Id)
	if err != nil {
		return err
	}
	if group == nil {
		return errStaffPanelGone
	}
	text, keyboard, err := buildStaffPanel(tr, *group, b.Username)
	if err != nil {
		return err
	}
	return editStaffMessage(b, msg, text, keyboard)
}

// staffUnlinkExpired answers a press whose link no longer exists (or whose button
// data is unusable) with the "expired" toast and refreshes the panel, so the stale
// button disappears.
func staffUnlinkExpired(b *gotgbot.Bot, query *gotgbot.CallbackQuery, tr *i18n.Translator) {
	if err := staffRerenderPanel(b, query.Message, tr); err != nil && !errors.Is(err, errStaffPanelGone) {
		log.Warnf("[Staff] refresh after expired unlink press: %v", err)
	}
	text, _ := tr.GetString("staff_cb_expired")
	answerStaffCallback(b, query, text, false)
}

// answerUnlinkRefusal answers a refused Unlink press once: an owner-only alert for
// either ownership mismatch, the expired toast for a lost race, and a could-not-
// verify alert for everything else. Nothing is ever posted to a chat from here.
func answerUnlinkRefusal(
	b *gotgbot.Bot,
	query *gotgbot.CallbackQuery,
	tr *i18n.Translator,
	refusal staffUnlinkRefusal,
) {
	switch refusal {
	case staffUnlinkNotStaffOwner, staffUnlinkNotGroupOwner:
		text, _ := tr.GetString("staff_cb_owner_only")
		answerStaffCallback(b, query, text, true)
	case staffUnlinkGone:
		staffUnlinkExpired(b, query, tr)
	default:
		text, _ := tr.GetString("staff_check_failed")
		answerStaffCallback(b, query, text, true)
	}
}

// loadUnlinkTarget resolves the link an Unlink button names, from the row ID
// alone: the data is client-supplied, so the row is loaded fresh and the message
// must sit in that link's own Staff Group. On any failure it has already answered
// the press and returns nil.
func loadUnlinkTarget(
	b *gotgbot.Bot,
	query *gotgbot.CallbackQuery,
	tr *i18n.Translator,
	fields map[string]string,
) *models.StaffGroupLink {
	id, ok := parseStaffLinkID(fields["l"])
	if !ok {
		staffUnlinkExpired(b, query, tr)
		return nil
	}
	link, err := staff.GetLinkByIDFresh(id)
	if err != nil {
		text, _ := tr.GetString("staff_check_failed")
		answerStaffCallback(b, query, text, true)
		return nil
	}
	if link == nil {
		staffUnlinkExpired(b, query, tr)
		return nil
	}
	if query.Message.GetChat().Id != link.StaffChatID {
		text, _ := tr.GetString("staff_cb_denied")
		answerStaffCallback(b, query, text, true)
		return nil
	}
	return link
}

// staffUnlinkConfirmPrompt builds the "Are you sure?" text and its Confirm and
// Cancel buttons, which carry the same link ID. ok is false when the button data
// cannot be encoded.
func staffUnlinkConfirmPrompt(
	tr *i18n.Translator,
	link models.StaffGroupLink,
) (text string, keyboard gotgbot.InlineKeyboardMarkup, ok bool) {
	id := strconv.FormatUint(uint64(link.ID), 10)
	confirmData := encodeCallbackData(staffCallbackNamespace, map[string]string{"a": staffActUnlinkConfirm, "l": id})
	cancelData := encodeCallbackData(staffCallbackNamespace, map[string]string{"a": staffActUnlinkCancel, "l": id})
	if confirmData == "" || cancelData == "" {
		return "", keyboard, false
	}
	text, _ = tr.GetString("staff_unlink_confirm_prompt", i18n.TranslationParams{"group": staffGroupTitleToken})
	text = strings.Replace(text, staffGroupTitleToken, staffDisplayTitle(link.GroupTitle), 1)
	confirmLabel, _ := tr.GetString("staff_btn_confirm")
	cancelLabel, _ := tr.GetString("staff_btn_cancel")
	keyboard.InlineKeyboard = [][]gotgbot.InlineKeyboardButton{{
		{Text: confirmLabel, CallbackData: confirmData},
		{Text: cancelLabel, CallbackData: cancelData},
	}}
	return text, keyboard, true
}

// staffUnlinkAsk handles the Unlink button: after the live creator-of-both check
// it swaps the panel for an "Are you sure?" prompt. Nothing is deleted yet.
func (moduleStruct) staffUnlinkAsk(
	b *gotgbot.Bot,
	query *gotgbot.CallbackQuery,
	tr *i18n.Translator,
	fields map[string]string,
) error {
	link := loadUnlinkTarget(b, query, tr, fields)
	if link == nil {
		return ext.EndGroups
	}
	if refusal, _ := checkUnlinkAuthority(b, &query.From, *link); refusal != staffUnlinkOK {
		answerUnlinkRefusal(b, query, tr, refusal)
		return ext.EndGroups
	}
	text, keyboard, ok := staffUnlinkConfirmPrompt(tr, *link)
	if !ok {
		failed, _ := tr.GetString("staff_check_failed")
		answerStaffCallback(b, query, failed, true)
		return ext.EndGroups
	}
	_ = editStaffMessage(b, query.Message, text, keyboard)
	answerStaffCallback(b, query, "", false)
	return ext.EndGroups
}

// staffUnlinkConfirm handles Confirm. The live creator-of-both check runs again
// here, inside runUnlinkGroup, because ownership may have moved since the prompt
// was shown. On success the Staff Group gets the "unlinked by" notice and the panel
// is re-rendered in place.
func (moduleStruct) staffUnlinkConfirm(
	b *gotgbot.Bot,
	query *gotgbot.CallbackQuery,
	tr *i18n.Translator,
	fields map[string]string,
) error {
	link := loadUnlinkTarget(b, query, tr, fields)
	if link == nil {
		return ext.EndGroups
	}
	if refusal, _ := runUnlinkGroup(b, &query.From, *link); refusal != staffUnlinkOK {
		answerUnlinkRefusal(b, query, tr, refusal)
		return ext.EndGroups
	}
	finishStaffPanelPress(b, query, tr)
	return ext.EndGroups
}

// staffUnlinkCancel handles Cancel: it needs the same authority as the other two
// buttons, so a bystander cannot dismiss the creator's prompt, and then restores
// the panel.
func (moduleStruct) staffUnlinkCancel(
	b *gotgbot.Bot,
	query *gotgbot.CallbackQuery,
	tr *i18n.Translator,
	fields map[string]string,
) error {
	link := loadUnlinkTarget(b, query, tr, fields)
	if link == nil {
		return ext.EndGroups
	}
	if refusal, _ := checkUnlinkAuthority(b, &query.From, *link); refusal != staffUnlinkOK {
		answerUnlinkRefusal(b, query, tr, refusal)
		return ext.EndGroups
	}
	finishStaffPanelPress(b, query, tr)
	return ext.EndGroups
}

// finishStaffPanelPress re-renders the panel in place and answers the press once:
// with the expired toast when there is no Staff Group panel any more, otherwise
// with an empty answer (a failed edit is logged and does not undo the change).
func finishStaffPanelPress(b *gotgbot.Bot, query *gotgbot.CallbackQuery, tr *i18n.Translator) {
	if err := staffRerenderPanel(b, query.Message, tr); errors.Is(err, errStaffPanelGone) {
		text, _ := tr.GetString("staff_cb_expired")
		answerStaffCallback(b, query, text, false)
		return
	}
	answerStaffCallback(b, query, "", false)
}
