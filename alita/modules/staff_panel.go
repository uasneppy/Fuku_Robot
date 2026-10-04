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

// renderStaffPanel builds the text and keyboard of the /staff panel. It is pure:
// it performs no I/O, so the Phase 9 settings menu can reuse it. The keyboard is
// empty for now; later plans add the Add group, Unlink, Refresh and paging
// buttons.
func renderStaffPanel(
	tr *i18n.Translator,
	staffGroup models.StaffGroup,
	rows []staffLinkRow,
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

	return sb.String(), gotgbot.InlineKeyboardMarkup{}
}
