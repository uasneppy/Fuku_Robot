package modules

import (
	"html"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/db/staff"
	"github.com/divkix/Alita_Robot/alita/i18n"
	"github.com/divkix/Alita_Robot/alita/utils/formatting"
)

const (
	maxStaffDisplayTitleRunes = 64
	maxStaffButtonTitleRunes  = 20
)

// staffLinkRow is one linked group as shown in the Staff Group panel. Later
// plans add per-row fields (member counts, recent actions).
type staffLinkRow struct {
	Link models.StaffGroupLink
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

// renderStaffPanel builds the text and keyboard of the /staff panel. It is pure:
// it performs no I/O, so the Phase 9 settings menu can reuse it. The keyboard has
// the Add group button first (when the bot has a username), then one Unlink
// button per linked group; later plans add the Refresh and paging buttons.
func renderStaffPanel(
	tr *i18n.Translator,
	staffGroup models.StaffGroup,
	rows []staffLinkRow,
	botUsername string,
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
		sb.WriteString("\n")
		sb.WriteString(staffDisplayTitle(row.Link.GroupTitle))
		sb.WriteString(" <code>")
		sb.WriteString(strconv.FormatInt(row.Link.GroupChatID, 10))
		sb.WriteString("</code>")
	}

	var keyboard gotgbot.InlineKeyboardMarkup
	if botUsername != "" {
		addLabel, _ := tr.GetString("staff_panel_add_group_button")
		keyboard.InlineKeyboard = [][]gotgbot.InlineKeyboardButton{{
			{Text: addLabel, Url: staffAddGroupURL(botUsername, staffGroup.ChatID)},
		}}
	}
	for _, row := range rows {
		if button, ok := staffUnlinkButton(tr, row.Link); ok {
			keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, []gotgbot.InlineKeyboardButton{button})
		}
	}
	return sb.String(), keyboard
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

// buildStaffPanel loads the linked groups of staffGroup straight from the
// database and renders the panel. It is shared by /staff and by the buttons that
// re-render the panel in place.
func buildStaffPanel(
	tr *i18n.Translator,
	staffGroup models.StaffGroup,
	botUsername string,
) (string, gotgbot.InlineKeyboardMarkup, error) {
	links, err := staff.ListLinksByStaffFresh(staffGroup.ChatID)
	if err != nil {
		return "", gotgbot.InlineKeyboardMarkup{}, err
	}
	rows := make([]staffLinkRow, 0, len(links))
	for _, link := range links {
		rows = append(rows, staffLinkRow{Link: link})
	}
	text, keyboard := renderStaffPanel(tr, staffGroup, rows, botUsername)
	return text, keyboard, nil
}
