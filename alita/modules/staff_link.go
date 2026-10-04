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
	"github.com/divkix/Alita_Robot/alita/utils/helpers"
)

// maxStaffChatIDArgLen caps the /linkstaff argument: a minus sign plus the 19
// digits of the largest int64.
const maxStaffChatIDArgLen = 20

// staffGroupTitleToken stands in for a group title while a notice is translated.
// Titles are user-controlled and the translator runs a printf-style pass over the
// interpolated text, so the escaped title is spliced in after translation.
const staffGroupTitleToken = "<<staff-group-title>>"

// staffRefusal says why a link attempt was refused. Refusals up to
// staffRefusalCheckFailedInPlace happen before the issuer is verified as the live
// creator of the named Staff Group and are answered in the issuing group; the
// rest are posted in the Staff Group.
type staffRefusal int

const (
	staffRefusalNone staffRefusal = iota
	staffRefusalAnonymous
	staffRefusalInvalidID
	staffRefusalNoStaffGroup
	staffRefusalNeedStaffID
	staffRefusalNotStaffOwner
	staffRefusalCheckFailedInPlace
	staffRefusalNotSupergroup
	staffRefusalTargetIsStaff
	staffRefusalNotTargetOwner
	staffRefusalAlreadyLinked
	staffRefusalCheckFailed
)

// staffLinkOutcome is the result of runLinkGroup. StaffGroup is set only once the
// issuer is verified live as its creator; it decides where a refusal is posted.
// Link is set only when the link was created.
type staffLinkOutcome struct {
	Refusal    staffRefusal
	StaffGroup *models.StaffGroup
	Link       *models.StaffGroupLink
}

var linkStaffDesc = helpers.CommandDescriptor{
	Name:           "linkstaff",
	RequiredChecks: []helpers.CheckFunc{helpers.RequireGroup()},
}

// parseStaffChatIDArg parses the Staff Group chat ID given to /linkstaff. It is
// strict: an optional leading minus sign, digits only, at most 20 characters, and
// a negative result.
func parseStaffChatIDArg(arg string) (int64, bool) {
	if arg == "" || len(arg) > maxStaffChatIDArgLen {
		return 0, false
	}
	digits := strings.TrimPrefix(arg, "-")
	if digits == "" {
		return 0, false
	}
	for _, r := range digits {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	id, err := strconv.ParseInt(arg, 10, 64)
	if err != nil || id >= 0 {
		return 0, false
	}
	return id, true
}

// staffHealthFromBot turns a live bot membership lookup into the health value
// stored on a link. Unknown is stored as ok: the live panel and the sweep correct
// it later.
func staffHealthFromBot(m gotgbot.MergedChatMember, res chat_status.BotMemberResult) string {
	switch res {
	case chat_status.BotMemberMissing:
		return models.StaffHealthBotMissing
	case chat_status.BotMemberUnknown:
		return models.StaffHealthOK
	}
	switch m.Status {
	case gotgbot.ChatMemberStatusLeft, gotgbot.ChatMemberStatusKicked:
		return models.StaffHealthBotMissing
	case gotgbot.ChatMemberStatusAdministrator:
		if m.CanRestrictMembers {
			return models.StaffHealthOK
		}
		return models.StaffHealthBotCannotRestrict
	default:
		return models.StaffHealthBotNotAdmin
	}
}

// resolveLinkStaffGroup finds the Staff Group named by staffArg and verifies live
// that the issuer is its creator. Anything that does not prove it, including a
// chat that is not a Staff Group at all, gives the same refusal.
func resolveLinkStaffGroup(b *gotgbot.Bot, issuer *gotgbot.User, staffArg string) (*models.StaffGroup, staffRefusal) {
	staffChatID, ok := parseStaffChatIDArg(staffArg)
	if !ok {
		return nil, staffRefusalInvalidID
	}
	group, err := staff.GetStaffGroupFresh(staffChatID)
	if err != nil {
		return nil, staffRefusalCheckFailedInPlace
	}
	if group == nil {
		return nil, staffRefusalNotStaffOwner
	}
	switch result, _, ownerErr := chat_status.CheckOwner(b, group.ChatID, issuer.Id); result {
	case chat_status.OwnerUnknown:
		log.Warnf("[Staff] link: owner check for Staff Group %d failed: %v", group.ChatID, ownerErr)
		return nil, staffRefusalCheckFailedInPlace
	case chat_status.OwnerMismatch:
		return nil, staffRefusalNotStaffOwner
	}
	return group, staffRefusalNone
}

// checkLinkTarget verifies, live, that the target is a supergroup the issuer
// created.
func checkLinkTarget(b *gotgbot.Bot, issuer *gotgbot.User, target *gotgbot.Chat) staffRefusal {
	if target.Type != "supergroup" {
		return staffRefusalNotSupergroup
	}
	switch result, _, ownerErr := chat_status.CheckOwner(b, target.Id, issuer.Id); result {
	case chat_status.OwnerUnknown:
		log.Warnf("[Staff] link: owner check for group %d failed: %v", target.Id, ownerErr)
		return staffRefusalCheckFailed
	case chat_status.OwnerMismatch:
		return staffRefusalNotTargetOwner
	}
	return staffRefusalNone
}

// storeLink writes the link and maps the repository's answer to a refusal.
func storeLink(link *models.StaffGroupLink) staffRefusal {
	err := staff.CreateLink(link)
	switch {
	case err == nil:
		return staffRefusalNone
	case errors.Is(err, staff.ErrAlreadyLinked):
		return staffRefusalAlreadyLinked
	case errors.Is(err, staff.ErrRoleConflict):
		return staffRefusalTargetIsStaff
	default:
		return staffRefusalCheckFailed
	}
}

// runLinkGroup is the one link flow behind both /linkstaff and the group picker.
// Every check is live and none comes from a cache or from the start payload. The
// order is fixed: anonymous sender, the named Staff Group and its live creator
// (refusals up to here are answered in the issuing group), then the target's type
// and live creator, the bot's health in the target, and finally the insert.
func runLinkGroup(
	b *gotgbot.Bot,
	ctx *ext.Context,
	issuer *gotgbot.User,
	target *gotgbot.Chat,
	staffArg string,
) staffLinkOutcome {
	if helpers.IsAnonymousSender(ctx, issuer) {
		return staffLinkOutcome{Refusal: staffRefusalAnonymous}
	}
	staffGroup, refusal := resolveLinkStaffGroup(b, issuer, staffArg)
	if refusal != staffRefusalNone {
		return staffLinkOutcome{Refusal: refusal}
	}
	out := staffLinkOutcome{StaffGroup: staffGroup}
	if out.Refusal = checkLinkTarget(b, issuer, target); out.Refusal != staffRefusalNone {
		return out
	}

	member, res, err := chat_status.FetchBotMember(b, target.Id)
	if err != nil {
		log.Warnf("[Staff] link: bot membership lookup in group %d failed: %v", target.Id, err)
	}
	link := &models.StaffGroupLink{
		GroupChatID: target.Id,
		StaffChatID: staffGroup.ChatID,
		OwnerUserID: issuer.Id,
		GroupTitle:  target.Title,
		Health:      staffHealthFromBot(member, res),
	}
	if out.Refusal = storeLink(link); out.Refusal == staffRefusalNone {
		out.Link = link
	}
	return out
}

// linkHealthWarning returns the one warning line for a link whose bot lacks
// rights, or "" when the health is ok (D-04).
func linkHealthWarning(tr *i18n.Translator, health string) string {
	var text string
	switch health {
	case models.StaffHealthBotMissing:
		text, _ = tr.GetString("staff_link_warn_bot_missing")
	case models.StaffHealthBotNotAdmin:
		text, _ = tr.GetString("staff_link_warn_bot_not_admin")
	case models.StaffHealthBotCannotRestrict:
		text, _ = tr.GetString("staff_link_warn_bot_cannot_restrict")
	}
	return text
}

// deliverLinkOutcome reports the outcome of runLinkGroup. A created link is
// announced in the Staff Group only, with one warning line when the bot lacks
// rights in the new group. Nothing is ever sent to the linked group.
func (moduleStruct) deliverLinkOutcome(
	b *gotgbot.Bot,
	_ *ext.Context,
	_ *gotgbot.User,
	_ *gotgbot.Chat,
	out staffLinkOutcome,
) error {
	if out.Refusal != staffRefusalNone || out.Link == nil || out.StaffGroup == nil {
		log.Debugf("[Staff] link refused: %d", out.Refusal)
		return ext.EndGroups
	}
	tr := staffChatTranslator(out.StaffGroup.ChatID)
	text, _ := tr.GetString("staff_link_done", i18n.TranslationParams{
		"group": staffGroupTitleToken,
		"id":    out.Link.GroupChatID,
	})
	text = strings.Replace(text, staffGroupTitleToken, staffDisplayTitle(out.Link.GroupTitle), 1)
	if warning := linkHealthWarning(tr, out.Link.Health); warning != "" {
		text += "\n" + warning
	}
	_ = sendStaffNotice(b, out.StaffGroup.ChatID, text)
	return ext.EndGroups
}

// linkStaff handles /linkstaff [Staff Group chat ID] in the group being linked.
// The command message is removed first, best effort and never gated on the result
// (D-07): the bot's delete right is not checked through the admin cache.
func (m moduleStruct) linkStaff(c *helpers.CommandContext) error {
	_, _ = c.Msg.Delete(c.Bot, nil)
	staffArg := ""
	if args := c.Ctx.Args(); len(args) > 1 {
		staffArg = strings.Join(args[1:], " ")
	}
	out := runLinkGroup(c.Bot, c.Ctx, c.User, c.Chat, staffArg)
	return m.deliverLinkOutcome(c.Bot, c.Ctx, c.User, c.Chat, out)
}
