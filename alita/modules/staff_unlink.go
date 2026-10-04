package modules

import (
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
