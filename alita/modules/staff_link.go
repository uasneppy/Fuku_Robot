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

// staffPickerPrefix starts the start payload of the Add group picker, followed by
// the decimal of the negated Staff Group chat ID: at most 4 + 19 characters, all
// from the charset Telegram allows in a start payload (A-Za-z0-9_-).
const (
	staffPickerPrefix    = "stf_"
	maxStaffPickerDigits = 19
)

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

// parseStaffPickerPayload parses the start payload of the Add group picker: "stf_"
// followed by 1 to 19 ASCII digits, the decimal of the negated Staff Group chat
// ID. Anything else, including a zero value, is not ours. The payload names the
// Staff Group only; it never authorizes anything.
func parseStaffPickerPayload(arg string) (staffChatID int64, ok bool) {
	digits, found := strings.CutPrefix(arg, staffPickerPrefix)
	if !found || digits == "" || len(digits) > maxStaffPickerDigits {
		return 0, false
	}
	for _, r := range digits {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	id, err := strconv.ParseInt(digits, 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return -id, true
}

// staffPickerDeepLink handles /start@bot stf_<id> in a group: the picker's message.
// A payload that is not stf_ plus digits is not handled, so /start keeps its
// normal reply. Otherwise it removes the message (D-07), runs the same live-checked
// link flow as /linkstaff, and always ends the update.
func staffPickerDeepLink(b *gotgbot.Bot, ctx *ext.Context, user *gotgbot.User, arg string) (bool, error) {
	staffChatID, ok := parseStaffPickerPayload(arg)
	if !ok || ctx == nil || ctx.EffectiveChat == nil {
		return false, nil
	}
	if msg := ctx.EffectiveMessage; msg != nil {
		_, _ = msg.Delete(b, nil)
	}
	out := runLinkGroup(b, ctx, user, ctx.EffectiveChat, strconv.FormatInt(staffChatID, 10))
	return true, staffModule.deliverLinkOutcome(b, ctx, user, ctx.EffectiveChat, out)
}

func init() {
	RegisterGroupDeepLinkHandler("stf_", staffPickerDeepLink)
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

// resolveLinkStaffGroup finds the Staff Group to link to and verifies live that
// the issuer is its creator. With an argument it is the named Staff Group; without
// one it is the issuer's only live-verified Staff Group.
func resolveLinkStaffGroup(b *gotgbot.Bot, issuer *gotgbot.User, staffArg string) (*models.StaffGroup, staffRefusal) {
	if strings.TrimSpace(staffArg) == "" {
		return resolveOwnStaffGroup(b, issuer)
	}
	return resolveNamedStaffGroup(b, issuer, staffArg)
}

// resolveOwnStaffGroup picks the Staff Group for /linkstaff without an argument.
// The issuer's recorded Staff Groups are only candidates: each is verified live,
// and one is used only when it is the single match. Several matches ask for an
// explicit ID, never an ordering guess. A candidate that could not be verified
// also fails closed, because it might be a second match.
func resolveOwnStaffGroup(b *gotgbot.Bot, issuer *gotgbot.User) (*models.StaffGroup, staffRefusal) {
	candidates, err := staff.ListStaffGroupsByOwner(issuer.Id)
	if err != nil {
		return nil, staffRefusalCheckFailedInPlace
	}
	if len(candidates) == 0 {
		return nil, staffRefusalNoStaffGroup
	}
	var matched []*models.StaffGroup
	unknown := 0
	for i := range candidates {
		result, _, ownerErr := chat_status.CheckOwner(b, candidates[i].ChatID, issuer.Id)
		switch result {
		case chat_status.OwnerMatch:
			matched = append(matched, &candidates[i])
		case chat_status.OwnerUnknown:
			log.Warnf("[Staff] link: owner check for Staff Group %d failed: %v", candidates[i].ChatID, ownerErr)
			unknown++
		}
	}
	switch {
	case len(matched) > 1:
		return nil, staffRefusalNeedStaffID
	case unknown > 0:
		return nil, staffRefusalCheckFailedInPlace
	case len(matched) == 1:
		return matched[0], staffRefusalNone
	default:
		return nil, staffRefusalNoStaffGroup
	}
}

// resolveNamedStaffGroup verifies the Staff Group named by staffArg. Anything that
// does not prove the issuer is its live creator, including a chat that is not a
// Staff Group at all, gives the same refusal.
func resolveNamedStaffGroup(b *gotgbot.Bot, issuer *gotgbot.User, staffArg string) (*models.StaffGroup, staffRefusal) {
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

// checkLinkTarget verifies, live, that the target is a supergroup that is not a
// Staff Group and that the issuer created.
func checkLinkTarget(b *gotgbot.Bot, issuer *gotgbot.User, target *gotgbot.Chat) staffRefusal {
	if target.Type != "supergroup" {
		return staffRefusalNotSupergroup
	}
	targetStaff, err := staff.GetStaffGroupFresh(target.Id)
	if err != nil {
		return staffRefusalCheckFailed
	}
	if targetStaff != nil {
		return staffRefusalTargetIsStaff
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

// staffLinkRefusalText maps a refusal to its message. Every case reads a literal
// locale key so make check-translations sees it. groupTitle is the title of the
// group being linked; it is user-controlled, so it is escaped and spliced in after
// translation rather than passed through the translator.
func staffLinkRefusalText(tr *i18n.Translator, r staffRefusal, groupTitle string) string {
	params := i18n.TranslationParams{"group": staffGroupTitleToken}
	var text string
	switch r {
	case staffRefusalAnonymous:
		text, _ = tr.GetString("staff_post_as_yourself")
	case staffRefusalInvalidID:
		text, _ = tr.GetString("staff_link_invalid_id")
	case staffRefusalNoStaffGroup:
		text, _ = tr.GetString("staff_link_no_staff_group")
	case staffRefusalNeedStaffID:
		text, _ = tr.GetString("staff_link_need_staff_id")
	case staffRefusalNotStaffOwner:
		text, _ = tr.GetString("staff_link_refuse_not_staff_owner")
	case staffRefusalNotSupergroup:
		text, _ = tr.GetString("staff_link_refuse_not_supergroup", params)
	case staffRefusalTargetIsStaff:
		text, _ = tr.GetString("staff_link_refuse_target_is_staff", params)
	case staffRefusalNotTargetOwner:
		text, _ = tr.GetString("staff_link_refuse_not_target_owner", params)
	case staffRefusalAlreadyLinked:
		text, _ = tr.GetString("staff_link_refuse_already_linked", params)
	case staffRefusalCheckFailed:
		text, _ = tr.GetString("staff_link_check_failed_group", params)
	default:
		// staffRefusalCheckFailedInPlace and anything unexpected: could not verify.
		text, _ = tr.GetString("staff_check_failed")
	}
	return strings.Replace(text, staffGroupTitleToken, staffDisplayTitle(groupTitle), 1)
}

// deliverLinkRefusal tells the right place about a refusal. Before the issuer is
// verified as the Staff Group's creator, the refusal is a self-deleting reply in
// the issuing group, in that chat's language, and nothing is ever sent to the
// named chat. After verification it is posted in the Staff Group, in its language;
// if that post fails, a self-deleting reply in the issuing group takes its place.
func deliverLinkRefusal(b *gotgbot.Bot, ctx *ext.Context, target *gotgbot.Chat, out staffLinkOutcome) {
	issuingTr := ctxTr(ctx)
	if out.StaffGroup == nil {
		replySelfDeleting(b, ctx.EffectiveMessage, staffLinkRefusalText(issuingTr, out.Refusal, target.Title))
		return
	}
	staffTr := staffChatTranslator(out.StaffGroup.ChatID)
	if err := sendStaffNotice(b, out.StaffGroup.ChatID, staffLinkRefusalText(staffTr, out.Refusal, target.Title)); err != nil {
		replySelfDeleting(b, ctx.EffectiveMessage, staffLinkRefusalText(issuingTr, out.Refusal, target.Title))
	}
}

// deliverLinkOutcome reports the outcome of runLinkGroup. A created link is
// announced in the Staff Group only, with one warning line when the bot lacks
// rights in the new group. Nothing about a successful link is ever sent to the
// linked group, even when the Staff Group cannot be reached: the failure is only
// logged, because the group's members must not learn that it is linked (D-06).
func (moduleStruct) deliverLinkOutcome(
	b *gotgbot.Bot,
	ctx *ext.Context,
	_ *gotgbot.User,
	target *gotgbot.Chat,
	out staffLinkOutcome,
) error {
	if out.Refusal != staffRefusalNone {
		deliverLinkRefusal(b, ctx, target, out)
		return ext.EndGroups
	}
	if out.Link == nil || out.StaffGroup == nil {
		log.Errorf("[Staff] link outcome without a link for group %d", target.Id)
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
