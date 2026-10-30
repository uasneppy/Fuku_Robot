package modules

import (
	"context"
	"strconv"
	"time"
	"unicode/utf16"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/db/staff"
	"github.com/divkix/Alita_Robot/alita/i18n"
)

// The Staff Group page of the settings menu (MENU-05). A group that is linked to a Staff
// Group shows the link's health and offers Link and Unlink, which run the code of
// /linkstaff and /unlinkstaff, so the menu can never do more than the commands. A group
// that is itself a Staff Group shows the /staff panel read-only. A menu opened in the group
// is readable by every member of it, so it never shows the Staff Group's title or chat ID
// (D-21): settingsStaffViewFor is the one place that decides whether those are passed on.

const (
	// settingsPageStaff is the p value of the Staff Group page.
	settingsPageStaff = "s"

	// settingsStaffActLink links the group (s, when present, is the staff_groups row ID of the
	// Staff Group picked), settingsStaffActUnlinkAsk asks for the second tap and
	// settingsStaffActUnlinkYes takes it (e is the card's expiry minute).
	settingsStaffActLink      = "lk"
	settingsStaffActUnlinkAsk = "ua"
	settingsStaffActUnlinkYes = "uy"
	// settingsStaffActPage pages the read-only panel (v is the page).
	settingsStaffActPage = "pg"

	// settingsStaffToken stands for the Staff Group's escaped title until after translation.
	settingsStaffToken = "<<settings-staff>>"
)

func init() {
	registerSettingsPage(settingsPage{
		Key:       settingsPageStaff,
		Order:     10,
		Icon:      "👥",
		TitleKey:  "settings_page_staff",
		MainState: settingsStaffMainState,
		Open:      settingsStaffOpen,
		Actions: map[string]settingsPageAction{
			settingsStaffActLink:      {Fields: []string{"s"}, Right: settingsRightAdmin, Run: settingsStaffLinkRun},
			settingsStaffActUnlinkAsk: {Right: settingsRightAdmin, Run: settingsStaffUnlinkAsk},
			settingsStaffActUnlinkYes: {Fields: []string{"e"}, Right: settingsRightAdmin, Run: settingsStaffUnlinkYes},
			settingsStaffActPage:      {Fields: []string{"v"}, Right: settingsRightAdmin, Run: settingsStaffPage},
		},
	})
}

// settingsStaffMainState is the Staff Group button's state: how many groups are linked when
// this group is itself a Staff Group, else Linked or Not linked. It reads the database only
// and makes no Telegram call.
func settingsStaffMainState(_ *gotgbot.Bot, tr *i18n.Translator, group *gotgbot.Chat) (string, bool) {
	staffGroup, err := staff.GetStaffGroupFresh(group.Id)
	if err != nil {
		return "", false
	}
	if staffGroup != nil {
		links, err := staff.ListLinksByStaffFresh(group.Id)
		if err != nil {
			return "", false
		}
		return settingsText(tr, "settings_state_staff_n", i18n.TranslationParams{"count": len(links)}), true
	}
	link, err := staff.GetLinkOfGroupFresh(group.Id)
	if err != nil {
		return "", false
	}
	if link == nil {
		return settingsText(tr, "settings_state_not_linked"), true
	}
	return settingsText(tr, "settings_state_linked"), true
}

// settingsStaffView is what the renderers of a linked group's page may know. The Staff
// Group's title and chat ID are set only when Full, and Full is set only for a menu nobody
// but the Staff Group's own people can read (a private menu, or a menu inside the Staff
// Group), so a renderer of a group menu never receives them.
type settingsStaffView struct {
	Full        bool
	StaffTitle  string
	StaffChatID int64
	// Row carries the link's health only: its Link is cleared, so it names no Staff Group.
	Row    staffLinkRow
	Linked bool
}

// settingsStaffViewFor builds the view of the linked page for a menu (D-21 enforcement
// point). It copies the Staff Group's title and chat ID only when v is a private menu or
// isStaffGroup holds; for a group menu they stay empty and no renderer can print them.
func settingsStaffViewFor(v settingsView, isStaffGroup bool, staffGroup *models.StaffGroup, row *staffLinkRow) settingsStaffView {
	sv := settingsStaffView{Full: v.Private || isStaffGroup}
	if row != nil {
		sv.Linked = true
		sv.Row = staffLinkRow{
			Health:      row.Health,
			HealthKnown: row.HealthKnown,
			OwnerState:  row.OwnerState,
			LockedSince: row.LockedSince,
		}
	}
	if sv.Full {
		switch {
		case staffGroup != nil:
			sv.StaffTitle = staffGroup.Title
			sv.StaffChatID = staffGroup.ChatID
		case row != nil:
			sv.StaffChatID = row.Link.StaffChatID
		}
	}
	return sv
}

// renderSettingsStaffLinked draws the page of a linked group: the line naming the Staff Group
// (or the hidden line), the health of the link, the legend, the time of the check, and
// Refresh and Unlink. The Staff Group's title is escaped and spliced in after translation.
func renderSettingsStaffLinked(v settingsView, sv settingsStaffView, notice string) settingsScreen {
	var first string
	if sv.Full {
		first = settingsText(v.Tr, "settings_staff_linked_to", i18n.TranslationParams{
			"staff": settingsStaffToken,
			"id":    sv.StaffChatID,
		})
		first = lockdownSplice(first, settingsStaffToken, staffDisplayTitle(sv.StaffTitle))
	} else {
		first = settingsText(v.Tr, "settings_staff_linked_hidden")
	}
	body := []string{first, renderStaffRowStatus(v.Tr, sv.Row), settingsText(v.Tr, "staff_panel_legend")}

	k := newSettingsKeyboard(v)
	if refresh, ok := k.button(settingsText(v.Tr, "staff_panel_refresh_button"),
		map[string]string{"a": settingsActOpen, "p": settingsPageStaff}); ok {
		k.row(refresh)
	}
	if unlink, ok := k.button(settingsText(v.Tr, "settings_btn_unlink"),
		map[string]string{"a": settingsStaffActUnlinkAsk}); ok {
		k.row(unlink)
	}
	k.navRow(false)
	return k.screen(settingsCompose(v, "settings_page_staff", body, notice, true))
}

// renderSettingsStaffNotLinked draws the page of a group that is not linked. It names no
// Staff Group in either variant.
func renderSettingsStaffNotLinked(v settingsView, notice string) settingsScreen {
	body := []string{
		settingsText(v.Tr, "settings_staff_not_linked"),
		settingsText(v.Tr, "settings_staff_link_hint"),
	}
	k := newSettingsKeyboard(v)
	if link, ok := k.button(settingsText(v.Tr, "settings_btn_link"),
		map[string]string{"a": settingsStaffActLink}); ok {
		k.row(link)
	}
	k.navRow(false)
	return k.screen(settingsCompose(v, "settings_page_staff", body, notice, false))
}

// settingsUTF16Len is the length of text in UTF-16 code units, the unit Telegram's limit and
// the panel cap are counted in.
func settingsUTF16Len(text string) int {
	return len(utf16.Encode([]rune(text)))
}

// renderSettingsStaffPanel draws the read-only /staff panel of a Staff Group, one page of
// staffPanelPageSize rows: the panel's own text fitted into what is left of the 3800 units
// after the settings header, group line, footer and notice, then the read-only line. The
// page is clamped into range. Refresh keeps the page; Prev and Next carry the absolute
// target page.
func renderSettingsStaffPanel(
	v settingsView, staffGroup models.StaffGroup, rows []staffLinkRow, page int, now time.Time, notice string,
) settingsScreen {
	pages := max((len(rows)+staffPanelPageSize-1)/staffPanelPageSize, 1)
	page = min(max(page, 0), pages-1)
	start := page * staffPanelPageSize
	end := min(start+staffPanelPageSize, len(rows))

	readonly := settingsText(v.Tr, "settings_staff_readonly")
	// The frame is the page without the panel text: the panel text is inserted at the start
	// of the body, so the room left for it is the cap minus the frame.
	frame := settingsCompose(v, "settings_page_staff", []string{"\n\n" + readonly}, notice, false)
	limit := staffPanelMaxUTF16 - settingsUTF16Len(frame)

	panel, _ := staffPanelTextFor(v.Tr, staffPanelView{
		staffGroup: staffGroup,
		total:      len(rows),
		shown:      rows[start:end],
		page:       page,
		pages:      pages,
		updatedAt:  now,
		helpAsHint: true,
		limit:      limit,
	})

	k := newSettingsKeyboard(v)
	pageValue := func(n int) map[string]string {
		return map[string]string{"a": settingsStaffActPage, "v": strconv.Itoa(n)}
	}
	if refresh, ok := k.button(settingsText(v.Tr, "staff_panel_refresh_button"), pageValue(page)); ok {
		k.row(refresh)
	}
	var paging []gotgbot.InlineKeyboardButton
	if page > 0 {
		if prev, ok := k.button(settingsText(v.Tr, "staff_panel_prev"), pageValue(page-1)); ok {
			paging = append(paging, prev)
		}
	}
	if page < pages-1 {
		if next, ok := k.button(settingsText(v.Tr, "staff_panel_next"), pageValue(page+1)); ok {
			paging = append(paging, next)
		}
	}
	k.row(paging...)
	k.navRow(false)
	return k.screen(settingsCompose(v, "settings_page_staff", []string{panel + "\n\n" + readonly}, notice, false))
}

// settingsStaffData is what the page reads from the database before any Telegram call.
type settingsStaffData struct {
	// staffGroup is set when this group is itself a Staff Group, with its linked groups.
	staffGroup *models.StaffGroup
	links      []models.StaffGroupLink
	// link is this group's own link, and linkStaff the Staff Group it names (read for a
	// private menu only, which is the one linked page that shows it).
	link      *models.StaffGroupLink
	linkStaff *models.StaffGroup
}

// settingsStaffRead reads the page's data. A database error is returned and becomes the
// cannot-open edit.
func settingsStaffRead(p *settingsPress) (settingsStaffData, error) {
	var data settingsStaffData
	var err error
	if data.staffGroup, err = staff.GetStaffGroupFresh(p.group.Id); err != nil {
		return data, err
	}
	if data.staffGroup != nil {
		data.links, err = staff.ListLinksByStaffFresh(p.group.Id)
		return data, err
	}
	if data.link, err = staff.GetLinkOfGroupFresh(p.group.Id); err != nil {
		return data, err
	}
	if data.link != nil && p.private {
		data.linkStaff, err = staff.GetStaffGroupFresh(data.link.StaffChatID)
	}
	return data, err
}

// settingsStaffRender runs the page's live health checks (at most 2N+1 Telegram calls for N
// links, bounded by staffPanelBuildTimeout) and draws it. A link the owner check removed
// draws as not linked.
func settingsStaffRender(p *settingsPress, data settingsStaffData, notice string, page int) settingsScreen {
	v := p.view()
	ctx, cancel := context.WithTimeout(context.Background(), staffPanelBuildTimeout)
	defer cancel()

	switch {
	case data.staffGroup != nil:
		rows := buildStaffPanelRows(ctx, p.b, data.links)
		markLockedRows(rows)
		return renderSettingsStaffPanel(v, *data.staffGroup, rows, page, v.Now, notice)
	case data.link != nil:
		rows := buildStaffPanelRows(ctx, p.b, []models.StaffGroupLink{*data.link})
		markLockedRows(rows)
		if len(rows) == 0 {
			return renderSettingsStaffNotLinked(v, notice)
		}
		return renderSettingsStaffLinked(v, settingsStaffViewFor(v, false, data.linkStaff, &rows[0]), notice)
	}
	return renderSettingsStaffNotLinked(v, notice)
}

// settingsStaffBuild reads the page, answers the press empty (the live checks can outlast
// the callback's validity, D-31) and draws it. A database error comes back before the
// answer, so the cannot-open alert and edit are the press's one answer.
func settingsStaffBuild(p *settingsPress, notice string, page int) (settingsScreen, error) {
	data, err := settingsStaffRead(p)
	if err != nil {
		return settingsScreen{}, err
	}
	p.answer("", false)
	return settingsStaffRender(p, data, notice, page), nil
}

// settingsStaffOpen draws the page for the dispatcher.
func settingsStaffOpen(p *settingsPress, notice string) (settingsScreen, error) {
	return settingsStaffBuild(p, notice, 0)
}

// settingsStaffRedraw draws the page again after an action, or makes the cannot-open edit.
func settingsStaffRedraw(p *settingsPress, notice string) *settingsScreen {
	screen, err := settingsStaffBuild(p, notice, 0)
	if err != nil {
		log.Warnf("[Settings] Staff Group page of chat %d could not be read: %v", p.group.Id, err)
		p.cannotOpen()
		return nil
	}
	return &screen
}

// settingsStaffPage handles pg: the read-only panel at the page v, clamped. A group that is
// no longer a Staff Group gets its normal page.
func settingsStaffPage(p *settingsPress) *settingsScreen {
	page, ok := settingsParseNumber(p.fields["v"], 3)
	if !ok {
		p.answer(settingsText(p.tr, "common_callback_invalid_request"), true)
		return nil
	}
	screen, err := settingsStaffBuild(p, "", page)
	if err != nil {
		log.Warnf("[Settings] Staff Group page of chat %d could not be read: %v", p.group.Id, err)
		p.cannotOpen()
		return nil
	}
	return &screen
}

// settingsStaffLinkRefusalText is the text of a Link refusal: the command's own words
// (staffLinkRefusalText), except that a presser with no Staff Group of their own gets the
// menu's line, because the command's ends "then send this command again", which is wrong in
// a button flow.
func settingsStaffLinkRefusalText(p *settingsPress, refusal staffRefusal) string {
	if refusal == staffRefusalNoStaffGroup {
		return settingsText(p.tr, "settings_staff_no_group")
	}
	return staffLinkRefusalText(p.tr, refusal, p.group.Title)
}

// settingsStaffPickMax is how many Staff Groups the picker lists.
const settingsStaffPickMax = 8

// settingsStaffLinkRun handles lk. Without s it is the /linkstaff flow without an argument:
// the presser's Staff Groups verified live (listVerifiedOwnStaffGroups), none or an unknown
// answer is a notice, exactly one links at once, and several make the picker. With s it is a
// pick (settingsStaffPick). Every press is answered empty first, because the live checks can
// outlast the callback.
func settingsStaffLinkRun(p *settingsPress) *settingsScreen {
	if raw, picked := p.fields["s"]; picked {
		return settingsStaffPick(p, raw)
	}
	p.answer("", false)

	matched, unknown, err := listVerifiedOwnStaffGroups(p.b, &p.query.From)
	if err != nil {
		return settingsStaffRedraw(p, settingsNoticeWarn(settingsStaffLinkRefusalText(p, staffRefusalCheckFailedInPlace)))
	}
	// The decision is /linkstaff's own (decideOwnStaffGroup): none, or one match beside a
	// group that could not be verified, is a refusal, and one match links at once. Only
	// several matches differ: the command asks for an explicit ID, the menu shows a picker.
	only, refusal := decideOwnStaffGroup(matched, unknown)
	switch refusal {
	case staffRefusalNone:
		return settingsStaffLinkTo(p, only)
	case staffRefusalNeedStaffID:
		screen := renderSettingsStaffPicker(p.view(), matched, "")
		return &screen
	}
	return settingsStaffRedraw(p, settingsNoticeWarn(settingsStaffLinkRefusalText(p, refusal)))
}

// settingsStaffPick handles lk with s, the staff_groups row ID of the Staff Group picked.
// The ID is never trusted: the presser's own Staff Groups are listed again, the row whose ID
// equals s is taken (a position is never used), and resolveNamedStaffGroup verifies the
// presser is its live creator before the link is made. An s that is not a positive number is
// an invalid request; an s that names no row of the presser is refused with the command's own
// "you can only link a Staff Group you created" text and links nothing.
func settingsStaffPick(p *settingsPress, raw string) *settingsScreen {
	id, ok := settingsParseNumber(raw, 10)
	if !ok || id <= 0 {
		p.answer(settingsText(p.tr, "common_callback_invalid_request"), true)
		return nil
	}
	p.answer("", false)

	owned, err := staff.ListStaffGroupsByOwner(p.query.From.Id)
	if err != nil {
		return settingsStaffRedraw(p, settingsNoticeWarn(settingsStaffLinkRefusalText(p, staffRefusalCheckFailedInPlace)))
	}
	var picked *models.StaffGroup
	for i := range owned {
		if owned[i].ID == uint(id) {
			picked = &owned[i]
			break
		}
	}
	if picked == nil {
		return settingsStaffRedraw(p, settingsNoticeWarn(settingsStaffLinkRefusalText(p, staffRefusalNotStaffOwner)))
	}
	staffGroup, refusal := resolveNamedStaffGroup(p.b, &p.query.From, strconv.FormatInt(picked.ChatID, 10))
	if refusal != staffRefusalNone {
		return settingsStaffRedraw(p, settingsNoticeWarn(settingsStaffLinkRefusalText(p, refusal)))
	}
	return settingsStaffLinkTo(p, staffGroup)
}

// settingsStaffLinkTo links this group to a Staff Group the presser was just verified to
// create, through linkToStaffGroup (the target checks, the bot's health, the insert) and
// announceStaffLink (the notice to the Staff Group only), the code /linkstaff runs. A refusal
// is the command's own text; a success redraws the page as linked.
func settingsStaffLinkTo(p *settingsPress, staffGroup *models.StaffGroup) *settingsScreen {
	out := linkToStaffGroup(p.b, &p.query.From, p.group, staffGroup)
	if out.Refusal != staffRefusalNone {
		return settingsStaffRedraw(p, settingsNoticeWarn(settingsStaffLinkRefusalText(p, out.Refusal)))
	}
	if out.Link == nil || out.StaffGroup == nil {
		log.Errorf("[Settings] link outcome without a link for group %d", p.group.Id)
		return settingsStaffRedraw(p, settingsNoticeWarn(settingsStaffLinkRefusalText(p, staffRefusalCheckFailedInPlace)))
	}
	announceStaffLink(p.b, out)
	return settingsStaffRedraw(p, settingsNoticeOK(settingsText(p.tr, "settings_notice_linked")))
}

// renderSettingsStaffPicker draws the list of the presser's Staff Groups, at most
// settingsStaffPickMax, one button each, carrying a=lk and s, the staff_groups row ID. A
// private menu labels a button with the title; a group menu is readable by every member, so
// its labels are positions and it never reads a title (D-21).
func renderSettingsStaffPicker(v settingsView, groups []models.StaffGroup, notice string) settingsScreen {
	type choice struct {
		id    uint
		title string
	}
	choices := make([]choice, 0, min(len(groups), settingsStaffPickMax))
	for i := 0; i < len(groups) && i < settingsStaffPickMax; i++ {
		c := choice{id: groups[i].ID}
		if v.Private {
			c.title = groups[i].Title
		}
		choices = append(choices, c)
	}

	body := []string{settingsText(v.Tr, "settings_staff_pick")}
	if more := len(groups) - len(choices); more > 0 {
		// The command can link any of them by its ID; the menu lists no more than fit.
		body = append(body, settingsText(v.Tr, "settings_staff_pick_more", i18n.TranslationParams{"n": more}))
	}
	if !v.Private {
		body = append(body, settingsText(v.Tr, "settings_staff_pick_private_hint"))
	}
	k := newSettingsKeyboard(v)
	for i, c := range choices {
		label := "👥 " + staffButtonTitle(c.title)
		if !v.Private {
			label = settingsText(v.Tr, "settings_btn_staff_pick_n", i18n.TranslationParams{"n": i + 1})
		}
		fields := map[string]string{"a": settingsStaffActLink, "s": strconv.FormatUint(uint64(c.id), 10)}
		if button, ok := k.button(label, fields); ok {
			k.row(button)
		}
	}
	k.navRow(false)
	return k.screen(settingsCompose(v, "settings_page_staff", body, notice, false))
}

// renderSettingsStaffUnlinkCard draws the Unlink card: the confirm prompt naming only this
// group (its stored title, escaped and spliced in after translation), Yes and No, and the
// navigation row.
func renderSettingsStaffUnlinkCard(v settingsView, link models.StaffGroupLink) settingsScreen {
	prompt := settingsText(v.Tr, "staff_unlink_confirm_prompt", i18n.TranslationParams{"group": staffGroupTitleToken})
	prompt = lockdownSplice(prompt, staffGroupTitleToken, staffDisplayTitle(link.GroupTitle))

	k := newSettingsKeyboard(v)
	settingsCardRows(k, map[string]string{"a": settingsStaffActUnlinkYes}, settingsPageStaff)
	return k.screen(settingsCompose(v, "settings_page_staff", []string{prompt}, "", false))
}

// settingsStaffUnlinkAsk handles ua, the first tap of Unlink. The presser must be, live, the
// creator of the Staff Group and of this group (checkUnlinkAuthority, the check /unlinkstaff
// makes); a refusal is the command's own text on the redrawn page and no card is drawn.
func settingsStaffUnlinkAsk(p *settingsPress) *settingsScreen {
	p.answer("", false)
	link, err := staff.GetLinkOfGroupFresh(p.group.Id)
	if err != nil {
		return settingsStaffRedraw(p, settingsNoticeWarn(staffUnlinkRefusalText(p.tr, staffUnlinkCheckFailedInPlace, "")))
	}
	if link == nil {
		return settingsStaffRedraw(p, settingsNoticeWarn(staffUnlinkRefusalText(p.tr, staffUnlinkNotLinked, "")))
	}
	if refusal, _ := checkUnlinkAuthority(p.b, &p.query.From, *link); refusal != staffUnlinkOK {
		return settingsStaffRedraw(p, settingsNoticeWarn(staffUnlinkRefusalText(p.tr, refusal, link.GroupTitle)))
	}
	screen := renderSettingsStaffUnlinkCard(p.view(), *link)
	return &screen
}

// settingsStaffUnlinkYes handles uy, the second tap. A Yes whose expiry minute has passed
// redraws the page with settings_card_expired and unlinks nothing. Otherwise runUnlinkGroup
// runs, which checks the live creator of both groups again and deletes the link with a
// conditional delete, so of two Yes presses one wins and posts the one notice to the Staff
// Group.
func settingsStaffUnlinkYes(p *settingsPress) *settingsScreen {
	expired, ok := settingsCardExpired(p.fields)
	if !ok {
		p.answer(settingsText(p.tr, "common_callback_invalid_request"), true)
		return nil
	}
	if expired {
		return settingsStaffRedraw(p, settingsNoticeWarn(settingsText(p.tr, "settings_card_expired")))
	}
	p.answer("", false)

	link, err := staff.GetLinkOfGroupFresh(p.group.Id)
	if err != nil {
		return settingsStaffRedraw(p, settingsNoticeWarn(staffUnlinkRefusalText(p.tr, staffUnlinkCheckFailedInPlace, "")))
	}
	if link == nil {
		return settingsStaffRedraw(p, settingsNoticeWarn(staffUnlinkRefusalText(p.tr, staffUnlinkNotLinked, "")))
	}
	switch refusal, _ := runUnlinkGroup(p.b, &p.query.From, *link); refusal {
	case staffUnlinkOK:
		return settingsStaffRedraw(p, settingsNoticeOK(settingsText(p.tr, "settings_notice_unlinked")))
	case staffUnlinkGone:
		return settingsStaffRedraw(p, settingsNoticeWarn(staffUnlinkRefusalText(p.tr, staffUnlinkNotLinked, "")))
	default:
		return settingsStaffRedraw(p, settingsNoticeWarn(staffUnlinkRefusalText(p.tr, refusal, link.GroupTitle)))
	}
}
