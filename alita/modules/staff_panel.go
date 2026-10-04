package modules

import (
	"context"
	"html"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"

	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/db/staff"
	"github.com/divkix/Alita_Robot/alita/i18n"
	"github.com/divkix/Alita_Robot/alita/utils/chat_status"
	"github.com/divkix/Alita_Robot/alita/utils/error_handling"
	"github.com/divkix/Alita_Robot/alita/utils/formatting"
)

const (
	maxStaffDisplayTitleRunes = 64
	maxStaffButtonTitleRunes  = 20

	// staffPanelBuildTimeout bounds one whole live panel build.
	staffPanelBuildTimeout = 45 * time.Second
)

// staffLinkRow is one linked group as shown in the Staff Group panel. Later
// plans add per-row fields (member counts, recent actions).
type staffLinkRow struct {
	Link models.StaffGroupLink
	// Health is the bot health found by the live check; it is only meaningful
	// when HealthKnown is true.
	Health string
	// HealthKnown is false when the live bot lookup gave no answer.
	HealthKnown bool
	// OwnerState is OwnerMatch when the live owner check passed and OwnerUnknown
	// when it could not be completed.
	OwnerState chat_status.OwnerResult
}

// staffStatus is one yes / no / unknown answer shown as an icon in a panel row.
type staffStatus int

const (
	staffStatusYes staffStatus = iota
	staffStatusNo
	staffStatusUnknown
)

// staffStatusIcon maps a status to its icon. The icons are not localized; every
// word around them is.
func staffStatusIcon(state staffStatus) string {
	switch state {
	case staffStatusYes:
		return "✅"
	case staffStatusNo:
		return "❌"
	default:
		return "❔"
	}
}

// staffDisplayTitle caps a stored title at 64 runes and HTML-escapes it for
// safe interpolation into a Telegram HTML message.
func staffDisplayTitle(title string) string {
	if utf8.RuneCountInString(title) > maxStaffDisplayTitleRunes {
		title = string([]rune(title)[:maxStaffDisplayTitleRunes])
	}
	return html.EscapeString(title)
}

// staffButtonTitle shortens a stored title to 20 runes for a button label. Button
// text is plain, so unlike staffDisplayTitle it is not HTML-escaped.
func staffButtonTitle(title string) string {
	title = strings.TrimSpace(title)
	if utf8.RuneCountInString(title) > maxStaffButtonTitleRunes {
		title = string([]rune(title)[:maxStaffButtonTitleRunes])
	}
	return title
}

// staffAddGroupURL builds the t.me link behind the Add group button. It opens
// Telegram's group picker and asks for the restrict-members and delete-messages
// admin rights in the same step. The payload is stf_ plus the decimal of the
// negated Staff Group chat ID, at most 23 characters of A-Za-z0-9_-. URL buttons
// cannot be limited to one presser, so authority is enforced when /start@bot
// arrives in the picked group (D-17), never from this link.
func staffAddGroupURL(botUsername string, staffChatID int64) string {
	return "https://t.me/" + botUsername + "?startgroup=stf_" +
		strconv.FormatInt(-staffChatID, 10) + "&admin=restrict_members+delete_messages"
}

// staffRowStatuses derives the three icons of a row: the bot is an admin of the
// group, the bot can restrict members there, and the owner still matches.
func staffRowStatuses(row staffLinkRow) (bot, restrict, owner staffStatus) {
	owner = staffStatusYes
	if row.OwnerState != chat_status.OwnerMatch {
		owner = staffStatusUnknown
	}
	if !row.HealthKnown {
		return staffStatusUnknown, staffStatusUnknown, owner
	}
	switch row.Health {
	case models.StaffHealthOK:
		return staffStatusYes, staffStatusYes, owner
	case models.StaffHealthBotCannotRestrict:
		return staffStatusYes, staffStatusNo, owner
	default:
		return staffStatusNo, staffStatusNo, owner
	}
}

// staffRowReason returns the one reason line of a broken row, or "" for a healthy
// one. A bot problem outranks an unverifiable owner. Every case reads a literal
// locale key so make check-translations sees it.
func staffRowReason(tr *i18n.Translator, row staffLinkRow) string {
	if row.HealthKnown {
		var reason string
		switch row.Health {
		case models.StaffHealthBotMissing:
			reason, _ = tr.GetString("staff_panel_reason_bot_missing")
		case models.StaffHealthBotNotAdmin:
			reason, _ = tr.GetString("staff_panel_reason_bot_not_admin")
		case models.StaffHealthBotCannotRestrict:
			reason, _ = tr.GetString("staff_panel_reason_bot_cannot_restrict")
		}
		if reason != "" {
			return reason
		}
	}
	if row.OwnerState != chat_status.OwnerMatch {
		reason, _ := tr.GetString("staff_panel_reason_owner_unknown")
		return reason
	}
	return ""
}

// renderStaffRow renders one linked group: the escaped title and ID, its status
// line, and a reason line when it is broken. The title is written straight into
// the text, never through the translator.
func renderStaffRow(tr *i18n.Translator, row staffLinkRow) string {
	bot, restrict, owner := staffRowStatuses(row)
	status, _ := tr.GetString("staff_panel_row_status", i18n.TranslationParams{
		"bot":      staffStatusIcon(bot),
		"restrict": staffStatusIcon(restrict),
		"owner":    staffStatusIcon(owner),
	})
	var sb strings.Builder
	sb.WriteString(staffDisplayTitle(row.Link.GroupTitle))
	sb.WriteString(" <code>")
	sb.WriteString(strconv.FormatInt(row.Link.GroupChatID, 10))
	sb.WriteString("</code>\n")
	sb.WriteString(status)
	if reason := staffRowReason(tr, row); reason != "" {
		sb.WriteString("\n")
		sb.WriteString(reason)
	}
	return sb.String()
}

// renderStaffPanel builds the text and keyboard of the /staff panel. It is pure:
// it performs no I/O, so the Phase 9 settings menu can reuse it. The text has the
// help, the chat ID, each linked group with its live status, a legend and the
// time of the check. The keyboard has the Add group button first (when the bot
// has a username), then Refresh, then one Unlink button per linked group.
func renderStaffPanel(
	tr *i18n.Translator,
	staffGroup models.StaffGroup,
	rows []staffLinkRow,
	botUsername string,
	page int,
	updatedAt time.Time,
) (string, gotgbot.InlineKeyboardMarkup) {
	var sb strings.Builder

	help, _ := tr.GetString("staff_help_msg")
	sb.WriteString(formatting.ToTelegramHTML(help))

	chatLine, _ := tr.GetString("staff_panel_chat_id", i18n.TranslationParams{
		"chat_id": staffGroup.ChatID,
	})
	sb.WriteString("\n\n")
	sb.WriteString(chatLine)

	if len(rows) == 0 {
		noLinks, _ := tr.GetString("staff_panel_no_links")
		sb.WriteString("\n")
		sb.WriteString(noLinks)
	}
	if len(rows) > 0 {
		header, _ := tr.GetString("staff_panel_links_header", i18n.TranslationParams{
			"count": len(rows),
		})
		sb.WriteString("\n")
		sb.WriteString(header)
	}
	for _, row := range rows {
		sb.WriteString("\n\n")
		sb.WriteString(renderStaffRow(tr, row))
	}

	if len(rows) > 0 {
		legend, _ := tr.GetString("staff_panel_legend")
		sb.WriteString("\n\n")
		sb.WriteString(legend)
	}
	updated, _ := tr.GetString("staff_panel_updated", i18n.TranslationParams{
		"time": updatedAt.UTC().Format("15:04:05"),
	})
	sb.WriteString("\n")
	sb.WriteString(updated)

	var keyboard gotgbot.InlineKeyboardMarkup
	if botUsername != "" {
		addLabel, _ := tr.GetString("staff_panel_add_group_button")
		keyboard.InlineKeyboard = [][]gotgbot.InlineKeyboardButton{{
			{Text: addLabel, Url: staffAddGroupURL(botUsername, staffGroup.ChatID)},
		}}
	}
	if refresh, ok := staffRefreshButton(tr, page); ok {
		keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, []gotgbot.InlineKeyboardButton{refresh})
	}
	for _, row := range rows {
		if button, ok := staffUnlinkButton(tr, row.Link); ok {
			keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, []gotgbot.InlineKeyboardButton{button})
		}
	}
	return sb.String(), keyboard
}

// staffRefreshButton builds the Refresh button. Its data names the page to
// rebuild; ok is false when the data does not fit Telegram's 64 bytes.
func staffRefreshButton(tr *i18n.Translator, page int) (gotgbot.InlineKeyboardButton, bool) {
	data := encodeCallbackData(staffCallbackNamespace, map[string]string{
		"a": staffActRefresh,
		"p": strconv.Itoa(page),
	})
	if data == "" {
		log.Warnf("[Staff] refresh button for page %d skipped: callback data does not fit", page)
		return gotgbot.InlineKeyboardButton{}, false
	}
	label, _ := tr.GetString("staff_panel_refresh_button")
	return gotgbot.InlineKeyboardButton{Text: label, CallbackData: data}, true
}

// staffUnlinkButton builds the Unlink button of one linked group. The callback
// carries only the link's row ID; the handler loads the row, so nothing in the
// data is trusted. When the data does not fit Telegram's 64 bytes the button is
// left out and the failure logged, so no dead button ships.
func staffUnlinkButton(tr *i18n.Translator, link models.StaffGroupLink) (gotgbot.InlineKeyboardButton, bool) {
	data := encodeCallbackData(staffCallbackNamespace, map[string]string{
		"a": staffActUnlinkAsk,
		"l": strconv.FormatUint(uint64(link.ID), 10),
	})
	if data == "" {
		log.Warnf("[Staff] unlink button for link %d skipped: callback data does not fit", link.ID)
		return gotgbot.InlineKeyboardButton{}, false
	}
	// The title is user-controlled and the translator runs a printf-style pass
	// over interpolated text, so it is spliced in after translation.
	label, _ := tr.GetString("staff_panel_unlink_button", i18n.TranslationParams{"group": staffGroupTitleToken})
	label = strings.Replace(label, staffGroupTitleToken, staffButtonTitle(link.GroupTitle), 1)
	return gotgbot.InlineKeyboardButton{Text: label, CallbackData: data}, true
}

// buildStaffPanelRows checks every link live and returns the rows to show, in
// the order of links (link id ascending) whatever order the checks finish in.
//
// For each link the owner side goes through recheckLink, which removes the link
// and posts its notice when the owner changed (such a row is dropped), and the bot
// side goes through chat_status.FetchBotMember and applyLinkHealth, which records
// the health and posts one heads-up when it changed. Both share one owner pass, so
// a panel of N links makes at most 2N+1 Telegram calls, with at most 4 in flight.
// A check that gets no answer keeps its row
// and shows it as unknown; a cancelled ctx skips the remaining checks the same
// way.
func buildStaffPanelRows(ctx context.Context, b *gotgbot.Bot, links []models.StaffGroupLink) []staffLinkRow {
	pass := newStaffOwnerPass()
	slots := make([]*staffLinkRow, len(links))

	var group errgroup.Group
	group.SetLimit(4) // at most 4 live Telegram checks in flight (T-01-29)
	for i, link := range links {
		group.Go(func() error {
			// A panic leaves this link out of the panel until the next refresh.
			defer error_handling.RecoverFromPanic("buildStaffPanelRows", "Staff")
			row := staffLinkRow{Link: link, OwnerState: chat_status.OwnerUnknown}
			if ctx.Err() == nil {
				switch recheckLink(b, link, pass) {
				case staffRecheckRemoved, staffRecheckGone:
					return nil
				case staffRecheckOK:
					row.OwnerState = chat_status.OwnerMatch
				}
				member, result, err := chat_status.FetchBotMember(b, link.GroupChatID)
				if result == chat_status.BotMemberUnknown {
					log.Warnf("[Staff] panel: bot lookup in group %d failed: %v", link.GroupChatID, err)
				} else {
					row.Health = staffHealthFromBot(member, result)
					row.HealthKnown = true
					applyLinkHealth(b, link, row.Health)
				}
			}
			slots[i] = &row
			return nil
		})
	}
	_ = group.Wait()

	rows := make([]staffLinkRow, 0, len(links))
	for _, slot := range slots {
		if slot != nil {
			rows = append(rows, *slot)
		}
	}
	return rows
}

// buildStaffPanel loads the linked groups of staffGroup straight from the
// database, checks each live and renders the given page. It is shared by /staff,
// Refresh and the buttons that re-render the panel in place.
func buildStaffPanel(
	b *gotgbot.Bot,
	tr *i18n.Translator,
	staffGroup models.StaffGroup,
	page int,
) (string, gotgbot.InlineKeyboardMarkup, error) {
	links, err := staff.ListLinksByStaffFresh(staffGroup.ChatID)
	if err != nil {
		return "", gotgbot.InlineKeyboardMarkup{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), staffPanelBuildTimeout)
	defer cancel()
	rows := buildStaffPanelRows(ctx, b, links)
	text, keyboard := renderStaffPanel(tr, staffGroup, rows, b.Username, page, time.Now())
	return text, keyboard, nil
}

// isMessageNotModified reports whether err is Telegram's answer to an edit that
// would change nothing. The message already shows what was asked for, so callers
// treat it as success. The text is matched case-insensitively.
func isMessageNotModified(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "message is not modified")
}

// staffPanelRefresh handles the Refresh button: any current member of the Staff
// Group may press it. The chat is the one the button sits in, which must be a
// Staff Group, and the presser's membership is checked live. The press is
// answered exactly once, then the panel is rebuilt live and the same message is
// edited in place.
func (moduleStruct) staffPanelRefresh(
	b *gotgbot.Bot,
	query *gotgbot.CallbackQuery,
	tr *i18n.Translator,
	fields map[string]string,
) error {
	chat := query.Message.GetChat()
	group, err := staff.GetStaffGroupFresh(chat.Id)
	if err != nil {
		text, _ := tr.GetString("staff_check_failed")
		answerStaffCallback(b, query, text, true)
		return ext.EndGroups
	}
	if group == nil {
		text, _ := tr.GetString("staff_cb_expired")
		answerStaffCallback(b, query, text, false)
		return ext.EndGroups
	}

	member, err := chat_status.IsUserInChatWithError(b, &chat, query.From.Id)
	if err != nil {
		log.Warnf("[Staff] panel press: membership check of user %d in chat %d failed: %v", query.From.Id, chat.Id, err)
		text, _ := tr.GetString("staff_check_failed")
		answerStaffCallback(b, query, text, true)
		return ext.EndGroups
	}
	if !member {
		text, _ := tr.GetString("staff_cb_members_only")
		answerStaffCallback(b, query, text, true)
		return ext.EndGroups
	}

	// Answer before the live checks: they can outlast the callback's validity.
	answerStaffCallback(b, query, "", false)

	page, _ := strconv.Atoi(fields["p"])
	text, keyboard, err := buildStaffPanel(b, tr, *group, page)
	if err != nil {
		log.Errorf("[Staff] panel press: build panel of chat %d: %v", chat.Id, err)
		return ext.EndGroups
	}
	_ = editStaffMessage(b, query.Message, text, keyboard)
	return ext.EndGroups
}
