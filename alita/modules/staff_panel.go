package modules

import (
	"html"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/i18n"
	"github.com/divkix/Alita_Robot/alita/utils/formatting"
)

const maxStaffDisplayTitleRunes = 64

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
// the Add group button for now; later plans add the Unlink, Refresh and paging
// buttons.
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
	return sb.String(), keyboard
}

// staffButtonTitle is a compile-only stub for the RED commit.
func staffButtonTitle(title string) string {
	return ""
}
