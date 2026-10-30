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
			settingsStaffActPage: {Fields: []string{"v"}, Right: settingsRightAdmin, Run: settingsStaffPage},
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
