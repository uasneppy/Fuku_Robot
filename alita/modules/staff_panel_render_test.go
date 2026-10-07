//go:build testtools

package modules

import (
	"fmt"
	"html"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/i18n"
	"github.com/divkix/Alita_Robot/alita/utils/chat_status"
	"github.com/divkix/Alita_Robot/alita/utils/formatting"
)

const panelRenderStaffChatID int64 = -1001234567890

// panelRows returns n healthy rows in link-id order. titleOf names row i (0-based).
func panelRows(n int, titleOf func(i int) string) []staffLinkRow {
	rows := make([]staffLinkRow, n)
	for i := range rows {
		rows[i] = staffLinkRow{
			Link: models.StaffGroupLink{
				ID:          uint(i + 1),
				GroupChatID: -1002000000000 - int64(i),
				StaffChatID: panelRenderStaffChatID,
				OwnerUserID: 42,
				GroupTitle:  titleOf(i),
			},
			Health:      models.StaffHealthOK,
			HealthKnown: true,
			OwnerState:  chat_status.OwnerMatch,
		}
	}
	return rows
}

func panelRenderGroup() models.StaffGroup {
	return models.StaffGroup{ChatID: panelRenderStaffChatID, OwnerUserID: 42}
}

// panelButtons splits a keyboard into its buttons by action: "ul" Unlink buttons
// (in keyboard order), "pg" paging buttons, "rf" Refresh buttons, "rc" Recent
// actions buttons and URL buttons.
type panelButtons struct {
	unlink  []gotgbot.InlineKeyboardButton
	paging  []gotgbot.InlineKeyboardButton
	refresh []gotgbot.InlineKeyboardButton
	recent  []gotgbot.InlineKeyboardButton
	urls    []gotgbot.InlineKeyboardButton
}

// panelSplitKeyboard classifies every button and fails when callback data is
// longer than Telegram's 64 bytes or does not decode.
func panelSplitKeyboard(t *testing.T, keyboard gotgbot.InlineKeyboardMarkup) (buttons panelButtons, decoded map[string]map[string]string) {
	t.Helper()
	decoded = make(map[string]map[string]string)
	for _, row := range keyboard.InlineKeyboard {
		for _, button := range row {
			if button.Url != "" {
				buttons.urls = append(buttons.urls, button)
				continue
			}
			if len(button.CallbackData) > 64 {
				t.Errorf("callback data %q is %d bytes, want at most 64", button.CallbackData, len(button.CallbackData))
			}
			fields, ok := decodeCallbackData(button.CallbackData, staffCallbackNamespace)
			if !ok {
				t.Errorf("button %q has undecodable callback data %q", button.Text, button.CallbackData)
				continue
			}
			decoded[button.CallbackData] = fields.Fields
			switch fields.Fields["a"] {
			case staffActUnlinkAsk:
				buttons.unlink = append(buttons.unlink, button)
			case "pg":
				buttons.paging = append(buttons.paging, button)
			case staffActRefresh:
				buttons.refresh = append(buttons.refresh, button)
			case "rc":
				buttons.recent = append(buttons.recent, button)
			default:
				t.Errorf("button %q carries unexpected action %q", button.Text, fields.Fields["a"])
			}
		}
	}
	return buttons, decoded
}

// panelMarkerTranslator returns an English translator whose strings are markers.
func panelMarkerTranslator(t *testing.T) *i18n.Translator {
	t.Helper()
	withStaffLocale(t)
	return i18n.MustNewTranslator("en")
}

func panelUTF16Len(text string) int { return len(utf16.Encode([]rune(text))) }

func TestStaffPanelRenderPaging(t *testing.T) {
	tr := panelMarkerTranslator(t)
	title := func(i int) string { return "Group" + string(rune('A'+i)) }
	now := time.Now()

	t.Run("eight rows are one page", func(t *testing.T) {
		text, keyboard := renderStaffPanel(tr, panelRenderGroup(), panelRows(8, title), "fukubot", 0, now)
		buttons, _ := panelSplitKeyboard(t, keyboard)
		if len(buttons.paging) != 0 {
			t.Errorf("%d paging buttons, want none for 8 rows", len(buttons.paging))
		}
		if len(buttons.unlink) != 8 {
			t.Errorf("%d Unlink buttons, want 8", len(buttons.unlink))
		}
		if strings.Contains(text, staffMarker("staff_panel_page")) {
			t.Errorf("text %q must not carry a page line for one page", text)
		}
	})

	t.Run("nine rows are two pages", func(t *testing.T) {
		text, keyboard := renderStaffPanel(tr, panelRenderGroup(), panelRows(9, title), "fukubot", 0, now)
		buttons, decoded := panelSplitKeyboard(t, keyboard)
		if len(buttons.unlink) != 8 {
			t.Errorf("page 0 has %d Unlink buttons, want 8", len(buttons.unlink))
		}
		if len(buttons.paging) != 1 || !strings.Contains(buttons.paging[0].Text, staffMarker("staff_panel_next")) ||
			decoded[buttons.paging[0].CallbackData]["p"] != "1" {
			t.Errorf("page 0 paging buttons = %+v, want one Next with p=1", buttons.paging)
		}
		if !strings.Contains(text, staffMarker("staff_panel_page")+" 1 2") {
			t.Errorf("text %q must carry the one-based page line 1/2", text)
		}
		if strings.Contains(text, "GroupI") || !strings.Contains(text, "GroupH") {
			t.Errorf("page 0 text %q must show rows 1-8 only", text)
		}

		text, keyboard = renderStaffPanel(tr, panelRenderGroup(), panelRows(9, title), "fukubot", 1, now)
		buttons, decoded = panelSplitKeyboard(t, keyboard)
		if len(buttons.unlink) != 1 {
			t.Errorf("page 1 has %d Unlink buttons, want 1", len(buttons.unlink))
		}
		if len(buttons.paging) != 1 || !strings.Contains(buttons.paging[0].Text, staffMarker("staff_panel_prev")) ||
			decoded[buttons.paging[0].CallbackData]["p"] != "0" {
			t.Errorf("page 1 paging buttons = %+v, want one Prev with p=0", buttons.paging)
		}
		if !strings.Contains(text, "GroupI") || strings.Contains(text, "GroupA") {
			t.Errorf("page 1 text %q must show only row 9", text)
		}
		if len(buttons.refresh) != 1 || decoded[buttons.refresh[0].CallbackData]["p"] != "1" {
			t.Errorf("Refresh on page 1 must carry p=1, got %+v", buttons.refresh)
		}
	})

	t.Run("out of range pages are clamped", func(t *testing.T) {
		_, keyboard := renderStaffPanel(tr, panelRenderGroup(), panelRows(9, title), "fukubot", 7, now)
		buttons, decoded := panelSplitKeyboard(t, keyboard)
		if len(buttons.unlink) != 1 || len(buttons.refresh) != 1 || decoded[buttons.refresh[0].CallbackData]["p"] != "1" {
			t.Errorf("page 7 of 2 must clamp to the last page, got %d Unlink buttons", len(buttons.unlink))
		}
		_, keyboard = renderStaffPanel(tr, panelRenderGroup(), panelRows(9, title), "fukubot", -3, now)
		buttons, decoded = panelSplitKeyboard(t, keyboard)
		if len(buttons.unlink) != 8 || len(buttons.refresh) != 1 || decoded[buttons.refresh[0].CallbackData]["p"] != "0" {
			t.Errorf("page -3 must clamp to the first page, got %d Unlink buttons", len(buttons.unlink))
		}
	})
}

func TestStaffPanelRenderEmpty(t *testing.T) {
	tr := panelMarkerTranslator(t)

	text, keyboard := renderStaffPanel(tr, panelRenderGroup(), nil, "fukubot", 0, time.Now())

	if !strings.Contains(text, staffMarker("staff_panel_no_links")) {
		t.Errorf("text %q must carry staff_panel_no_links", text)
	}
	if !strings.Contains(text, strconv.FormatInt(panelRenderStaffChatID, 10)) {
		t.Errorf("text %q must carry the Staff Group chat ID", text)
	}
	buttons, _ := panelSplitKeyboard(t, keyboard)
	if len(buttons.urls) != 1 || !strings.Contains(buttons.urls[0].Url, "startgroup=stf_") {
		t.Errorf("URL buttons = %+v, want the Add group button", buttons.urls)
	}
	if len(buttons.refresh) != 1 {
		t.Errorf("%d Refresh buttons, want 1", len(buttons.refresh))
	}
	if len(buttons.paging) != 0 || len(buttons.unlink) != 0 {
		t.Errorf("empty panel has %d paging and %d Unlink buttons, want none", len(buttons.paging), len(buttons.unlink))
	}
}

func TestStaffPanelRenderEscapesAndTruncatesTitles(t *testing.T) {
	tr := panelMarkerTranslator(t)
	title := "<b>x</b>&" + strings.Repeat("😀", 61)
	if n := utf8.RuneCountInString(title); n != 70 {
		t.Fatalf("test title has %d runes, want 70", n)
	}
	rows := panelRows(1, func(int) string { return title })

	text, _ := renderStaffPanel(tr, panelRenderGroup(), rows, "fukubot", 0, time.Now())

	if strings.Contains(text, "<b>x</b>") {
		t.Errorf("text %q must not contain the raw <b>x</b> from a title", text)
	}
	if !strings.Contains(text, "&lt;b&gt;") {
		t.Errorf("text %q must carry the escaped title", text)
	}
	codeAt := strings.Index(text, " <code>"+strconv.FormatInt(rows[0].Link.GroupChatID, 10))
	if codeAt < 0 {
		t.Fatalf("text %q has no row", text)
	}
	lineStart := strings.LastIndex(text[:codeAt], "\n") + 1
	if got := utf8.RuneCountInString(html.UnescapeString(text[lineStart:codeAt])); got != 64 {
		t.Errorf("rendered title has %d runes after unescaping, want exactly 64", got)
	}
}

func TestStaffPanelRenderStableOrder(t *testing.T) {
	tr := panelMarkerTranslator(t)
	rows := panelRows(8, func(i int) string { return fmt.Sprintf("Order%02d", i) })

	text, keyboard := renderStaffPanel(tr, panelRenderGroup(), rows, "fukubot", 0, time.Now())

	last := -1
	for i := range rows {
		at := strings.Index(text, fmt.Sprintf("Order%02d", i))
		if at < 0 || at < last {
			t.Fatalf("row %d is out of order in %q", i, text)
		}
		last = at
	}
	buttons, decoded := panelSplitKeyboard(t, keyboard)
	if len(buttons.unlink) != len(rows) {
		t.Fatalf("%d Unlink buttons, want %d", len(buttons.unlink), len(rows))
	}
	for i, button := range buttons.unlink {
		if want := strconv.Itoa(i + 1); decoded[button.CallbackData]["l"] != want {
			t.Errorf("Unlink button %d names link %q, want %s", i, decoded[button.CallbackData]["l"], want)
		}
	}
}

func TestRenderStaffRowLockdownMarker(t *testing.T) {
	tr := panelMarkerTranslator(t)
	marker := staffMarker("staff_panel_row_lockdown")
	since := time.Date(2026, time.October, 5, 12, 4, 33, 0, time.UTC)

	t.Run("a locked row carries one lockdown line with the UTC time", func(t *testing.T) {
		row := panelRows(1, func(int) string { return "Alpha" })[0]
		row.LockedSince = since
		text := renderStaffRow(tr, row)
		if strings.Count(text, marker) != 1 {
			t.Fatalf("row %q must carry the lockdown line exactly once", text)
		}
		if !strings.Contains(text, "5 Oct 12:04") {
			t.Errorf("row %q must show the confirmation time as 5 Oct 12:04", text)
		}
	})

	t.Run("a row's local zone does not change the printed time", func(t *testing.T) {
		row := panelRows(1, func(int) string { return "Alpha" })[0]
		row.LockedSince = since.In(time.FixedZone("UTC+9", 9*60*60))
		if text := renderStaffRow(tr, row); !strings.Contains(text, "5 Oct 12:04") {
			t.Errorf("row %q must print the UTC time 5 Oct 12:04", text)
		}
	})

	t.Run("a row that is not locked has no lockdown line", func(t *testing.T) {
		row := panelRows(1, func(int) string { return "Alpha" })[0]
		if text := renderStaffRow(tr, row); strings.Contains(text, marker) {
			t.Errorf("row %q must carry no lockdown line when LockedSince is zero", text)
		}
	})

	t.Run("a locked row the bot left shows both the problem and the lockdown", func(t *testing.T) {
		row := panelRows(1, func(int) string { return "Alpha" })[0]
		row.Health = models.StaffHealthBotMissing
		row.HealthKnown = true
		row.LockedSince = since
		text := renderStaffRow(tr, row)
		if !strings.Contains(text, staffMarker("staff_panel_reason_bot_missing")) {
			t.Errorf("row %q must carry the bot-missing reason", text)
		}
		if !strings.Contains(text, marker) {
			t.Errorf("row %q must carry the lockdown line too", text)
		}
	})
}

func TestStaffPanelRenderFitsInEveryLocale(t *testing.T) {
	titles := map[string]func(int) string{
		// The worst case for escaping: every rune becomes a 5-unit entity.
		"ampersands": func(int) string { return strings.Repeat("&", 64) },
		// Astral runes take two UTF-16 units each.
		"emoji": func(int) string { return strings.Repeat("😀", 64) },
	}
	healths := []string{
		models.StaffHealthBotMissing, models.StaffHealthBotNotAdmin, models.StaffHealthBotCannotRestrict, models.StaffHealthOK,
	}

	for _, lang := range []string{"en", "es", "fr", "hi", "id", "pt", "ru"} {
		raw, err := os.ReadFile("../../locales/" + lang + ".yml")
		if err != nil {
			t.Fatalf("read locale %s: %v", lang, err)
		}
		tr, err := i18n.NewTestTranslator(string(raw))
		if err != nil {
			t.Fatalf("parse locale %s: %v", lang, err)
		}
		help, _ := tr.GetString("staff_help_msg")
		helpHTML := formatting.ToTelegramHTML(help)
		hint, _ := tr.GetString("staff_panel_help_hint")
		if strings.TrimSpace(hint) == "" {
			t.Errorf("%s: staff_panel_help_hint is missing or empty", lang)
		}

		for name, titleOf := range titles {
			rows := panelRows(40, titleOf)
			for i := range rows {
				rows[i].Health = healths[i%len(healths)]
				if i%5 == 0 {
					rows[i].OwnerState = chat_status.OwnerUnknown
				}
				if i%3 == 0 {
					rows[i].LockedSince = time.Date(2026, time.October, 5, 12, 4, 0, 0, time.UTC)
				}
			}
			for page := 0; page < 5; page++ {
				text, keyboard := renderStaffPanel(tr, panelRenderGroup(), rows, "fukubot", page, time.Now())
				if got := panelUTF16Len(text); got > 3800 {
					t.Errorf("%s/%s page %d: text is %d UTF-16 units, want at most 3800", lang, name, page, got)
				}
				if !strings.Contains(text, helpHTML) && !strings.Contains(text, hint) {
					t.Errorf("%s/%s page %d: help was dropped without the hint taking its place", lang, name, page)
				}
				buttons, _ := panelSplitKeyboard(t, keyboard)
				if len(buttons.unlink) > 8 {
					t.Errorf("%s/%s page %d: %d Unlink buttons, want at most 8", lang, name, page, len(buttons.unlink))
				}
			}
		}
	}
}

func TestStaffPanelPageCallback(t *testing.T) {
	env := newPanelEnv(t)
	for i := 0; i < 9; i++ {
		env.addGroup(t, env.ownerID, "Group "+string(rune('A'+i)))
	}

	env.press(t, env.staffChat(), env.owner(), "pg", "1")

	edits := env.edits()
	if len(edits) != 1 {
		t.Fatalf("%d edits after paging, want exactly one", len(edits))
	}
	text := panelEditText(edits[0])
	if !strings.Contains(text, "Group I") || strings.Contains(text, "Group A") {
		t.Errorf("page 1 text %q must show only the ninth group", text)
	}
	if sent := env.client.callsFor("sendMessage"); len(sent) != 0 {
		t.Errorf("paging sent %d messages, want none", len(sent))
	}
	if answers := env.answers(); len(answers) != 1 {
		t.Fatalf("paging answered %d times, want exactly once", len(answers))
	}

	const strangerID int64 = 6262
	env.setMember(env.staffID, strangerID, "left")
	env.press(t, env.staffChat(), gotgbot.User{Id: strangerID, FirstName: "Stranger"}, "pg", "1")

	if got := len(env.edits()); got != 1 {
		t.Fatalf("a non-member's page press edited the panel (%d edits total), want no further edit", got)
	}
	answers := env.answers()
	if len(answers) != 2 {
		t.Fatalf("%d answers in total, want 2", len(answers))
	}
	alert, _ := answers[1].Params["show_alert"].(bool)
	if !alert || !strings.Contains(fmt.Sprint(answers[1].Params["text"]), staffMarker("staff_cb_members_only")) {
		t.Fatalf("non-member answer = %+v, want a members-only alert", answers[1].Params)
	}
}

func TestStaffPanelRenderDropsRowsWithANoteWhenTooLong(t *testing.T) {
	raw, err := os.ReadFile("../../locales/en.yml")
	if err != nil {
		t.Fatalf("read locale en: %v", err)
	}
	tr, err := i18n.NewTestTranslator(string(raw))
	if err != nil {
		t.Fatalf("parse locale en: %v", err)
	}
	rows := panelRows(8, func(int) string { return strings.Repeat("&", 64) })
	for i := range rows {
		rows[i].Health = models.StaffHealthBotCannotRestrict
	}

	text, keyboard := renderStaffPanel(tr, panelRenderGroup(), rows, "fukubot", 0, time.Now())

	if got := panelUTF16Len(text); got > 3800 {
		t.Fatalf("text is %d UTF-16 units, want at most 3800", got)
	}
	hint, _ := tr.GetString("staff_panel_help_hint")
	if !strings.Contains(text, hint) {
		t.Errorf("text must swap the help for %q before dropping rows", hint)
	}
	buttons, _ := panelSplitKeyboard(t, keyboard)
	shown := strings.Count(text, "<code>-1002")
	if shown >= len(rows) || shown == 0 {
		t.Fatalf("%d rows shown, want some but not all of %d", shown, len(rows))
	}
	if len(buttons.unlink) != shown {
		t.Errorf("%d Unlink buttons for %d rows shown, want one per row shown", len(buttons.unlink), shown)
	}
	note, _ := tr.GetString("staff_panel_truncated", i18n.TranslationParams{"count": len(rows) - shown})
	if !strings.Contains(text, note) {
		t.Errorf("text must carry the note %q for the %d dropped rows", note, len(rows)-shown)
	}
}
