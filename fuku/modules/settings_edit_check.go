package modules

import (
	"fmt"
	"html"
	"regexp"
	"strings"
	"unicode/utf8"

	tgmd2html "github.com/PaulSonOfLars/gotg_md2html"
	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/content"
	"github.com/uasneppy/fuku_robot/fuku/utils/formatting"
	"github.com/uasneppy/fuku_robot/fuku/utils/keyboard"
)

const (
	// settingsEditFillCount is the member count the dry run fills {count} with: the widest a count
	// can be (nine digits).
	settingsEditFillCount = "999999999"

	// settingsEditTextLimit and settingsEditCaptionLimit are Telegram's limits, in UTF-16 units, of
	// a message text and of a caption.
	settingsEditTextLimit    = 4096
	settingsEditCaptionLimit = 1024

	// settingsEditMaxButtons and settingsEditMaxRowButtons are Telegram's limits of the buttons of
	// one message and of the buttons in one row.
	settingsEditMaxButtons    = 100
	settingsEditMaxRowButtons = 8

	// settingsEditDefaultButtonName is the label a button written with no name gets, as the
	// commands give it.
	settingsEditDefaultButtonName = "Greeting"
)

// settingsEditRulesTokenPattern finds the {rules}, {rules:same} and {rules:up} token that adds the
// rules button at a join (the same pattern the join fill uses).
var settingsEditRulesTokenPattern = regexp.MustCompile(`(?s){rules(:(same|up))?}`)

// settingsEditRulesLanguages are the languages the Rules dry run takes the /rules heading in: the
// group's language can change later, so the longest of them is held to the limit.
var settingsEditRulesLanguages = []string{"uk", "en", "es", "fr", "hi", "id", "pt", "ru"}

// This file is the pure check of a message the owner typed for one part of a Welcome, Goodbye
// or Rules message. It converts the message exactly as the commands convert theirs, and it
// refuses, with the reason, whatever Telegram would refuse when the message is sent at a join.
// It sends nothing, writes nothing and reads no database: the typed-reply door calls it between
// its authority checks and its claim, so a refusal never consumes the wait.

// settingsEditCheckInput is what the check judges: the page, the part the owner is setting, the
// message the owner sent, the stored message as the page shows it, the owner (the first of the
// three members the dry run fills the placeholders for), the group's title and the language of
// the commands' own refusal text.
type settingsEditCheckInput struct {
	Def        settingsEditPageDef
	Part       string
	Msg        *gotgbot.Message
	State      settingsEditState
	Owner      gotgbot.User
	GroupTitle string
	Language   string
}

// settingsEditVerdict is the answer of the check. When OK is true the fields of the judged part
// carry what to store: Text the stored HTML of a text part, FileID and DataType the media of a
// media part (CaptionIgnored when the media message carried a caption, which is not used) and
// Buttons the buttons of a buttons part. When OK is false RefusalKey and
// RefusalParams name the locale line to show, except that RefusalText is the finished text of a
// refusal the commands already send (the over-long text), which wins when it is not empty.
type settingsEditVerdict struct {
	OK             bool
	Text           string
	FileID         string
	DataType       int
	Buttons        []db.Button
	CaptionIgnored bool
	RefusalKey     string
	RefusalParams  i18n.TranslationParams
	RefusalText    string
}

// settingsEditRefusal is the verdict that refuses with a locale key and its parameters.
func settingsEditRefusal(key string, params i18n.TranslationParams) settingsEditVerdict {
	return settingsEditVerdict{RefusalKey: key, RefusalParams: params}
}

// settingsEditConvert runs one conversion of the owner's message and recovers from a panic of
// the converter (an entity whose offset is past the end of the text), answering ok false. It
// logs no text.
func settingsEditConvert(convert func() string) (out string, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			log.Warn("[SettingsEdit][Convert]: a typed message could not be converted")
			out, ok = "", false
		}
	}()
	return convert(), true
}

// settingsEditCheck judges the message of in for the part of in. It is pure: no Telegram call,
// no database read, no write.
func settingsEditCheck(in settingsEditCheckInput) settingsEditVerdict {
	if in.Msg == nil {
		return settingsEditRefusal("settings_edit_refuse_need_text", nil)
	}
	switch {
	case in.Part == settingsEditPartText && in.Def.Kind == settingsEditRules:
		return settingsEditCheckRulesText(in)
	case in.Part == settingsEditPartText:
		return settingsEditCheckGreetingText(in)
	case in.Def.Kind == settingsEditRules:
		// Rules has a text part only.
	case in.Part == settingsEditPartMedia:
		return settingsEditCheckMedia(in)
	case in.Part == settingsEditPartButtons:
		return settingsEditCheckButtons(in)
	}
	return settingsEditRefusal("settings_edit_refuse_need_text", nil)
}

// settingsEditCheckGreetingText converts a Welcome or Goodbye text as /setwelcome and
// /setgoodbye do (the buttons parser over the message's markdown, trimmed as preFixes trims)
// and refuses what those commands refuse, then what a join would fail to send.
func settingsEditCheckGreetingText(in settingsEditCheckInput) settingsEditVerdict {
	if in.Msg.Text == "" {
		return settingsEditRefusal("settings_edit_refuse_need_text", nil)
	}
	var buttons []tgmd2html.ButtonV2
	stored, ok := settingsEditConvert(func() string {
		text, btns := tgmd2html.MD2HTMLButtonsV2(in.Msg.OriginalMDV2())
		buttons = btns
		return text
	})
	if !ok {
		return settingsEditRefusal("settings_edit_refuse_need_text", nil)
	}
	if len(buttons) > 0 {
		return settingsEditRefusal("settings_edit_refuse_text_buttons", nil)
	}
	dataType, limit := db.TEXT, settingsEditTextLimit
	if settingsEditPartIsSet(in.State, settingsEditPartMedia) {
		dataType, limit = in.State.MediaType, settingsEditCaptionLimit
	}
	// The commands count the stored HTML before they trim it, so the same count is made here.
	if refusal := content.TextLengthRefusal(stored, dataType, in.Language); refusal != "" {
		return settingsEditVerdict{RefusalText: refusal}
	}
	stored = strings.Trim(stored, "\n\t\r ")

	longest, minimalBlank := settingsEditFilledLongest(in, stored)
	if stored == "" || (dataType == db.TEXT && minimalBlank) {
		return settingsEditRefusal("settings_edit_refuse_need_text", nil)
	}
	if longest > limit {
		return settingsEditTooLong(longest, limit)
	}
	// The {rules} button this text adds at a join is counted against the stored buttons, as the
	// buttons part counts it against the stored text, so neither part can make a join's welcome
	// a keyboard Telegram refuses.
	if total, widest := settingsEditButtonRows(in.State.Buttons, stored); total > settingsEditMaxButtons || widest > settingsEditMaxRowButtons {
		return settingsEditRefusal("settings_edit_refuse_buttons_limit", nil)
	}
	return settingsEditVerdict{OK: true, Text: stored}
}

// settingsEditOverLimit is a refusal that names how long the message would be and the limit.
func settingsEditOverLimit(key string, n, limit int) settingsEditVerdict {
	return settingsEditRefusal(key, i18n.TranslationParams{"n": n, "max": limit})
}

// settingsEditTooLong is the refusal of a message that is too long once its placeholders or its
// heading are filled in.
func settingsEditTooLong(n, limit int) settingsEditVerdict {
	return settingsEditOverLimit("settings_edit_refuse_too_long_filled", n, limit)
}

// settingsEditWorstUser is the member whose names make a filled message longest: 64 letter first
// and last names, no username (so {username} and {mention} show the first name) and a 13 digit ID.
func settingsEditWorstUser() *gotgbot.User {
	return &gotgbot.User{
		Id:        9999999999999,
		FirstName: strings.Repeat("W", 64),
		LastName:  strings.Repeat("L", 64),
	}
}

// settingsEditMinimalUser is the member whose names make a filled message shortest: a first name
// of one letter and nothing else, so {last} is empty.
func settingsEditMinimalUser() *gotgbot.User {
	return &gotgbot.User{Id: 1, FirstName: "A"}
}

// settingsEditFilledLongest fills stored for the owner, the worst-case member and the minimal
// member, the way a real join fills it, and returns the longest visible length (UTF-16 units)
// and whether the minimal member's text is blank.
func settingsEditFilledLongest(in settingsEditCheckInput, stored string) (longest int, minimalBlank bool) {
	owner := in.Owner
	minimal := settingsEditMinimalUser()
	for _, user := range []*gotgbot.User{&owner, settingsEditWorstUser(), minimal} {
		filled := formatting.FillPlaceholders(stored, user, in.GroupTitle, settingsEditFillCount)
		if n := formatting.VisibleLength(filled); n > longest {
			longest = n
		}
		if user == minimal {
			minimalBlank = strings.TrimSpace(tgmd2html.StripHTMLV2(filled)) == ""
		}
	}
	return longest, minimalBlank
}

// settingsEditCheckRulesText converts a Rules text as /setrules does and holds the /rules
// message built from it to Telegram's limit. Placeholders are not filled in rules.
func settingsEditCheckRulesText(in settingsEditCheckInput) settingsEditVerdict {
	if in.Msg.Text == "" {
		return settingsEditRefusal("settings_edit_refuse_need_text", nil)
	}
	stored, ok := settingsEditConvert(func() string {
		return tgmd2html.MD2HTMLV2(in.Msg.OriginalMDV2())
	})
	if !ok || strings.TrimSpace(tgmd2html.StripHTMLV2(stored)) == "" {
		return settingsEditRefusal("settings_edit_refuse_need_text", nil)
	}
	normalized := normalizeRulesForHTML(stored)
	longest := 0
	for _, heading := range settingsEditRulesHeadings(in.GroupTitle) {
		if n := formatting.VisibleLength(heading + normalized); n > longest {
			longest = n
		}
	}
	if longest > settingsEditTextLimit {
		return settingsEditTooLong(longest, settingsEditTextLimit)
	}
	return settingsEditVerdict{OK: true, Text: stored}
}

// settingsEditRulesHeadings returns, for every language and both ways the bot words the rules
// ("rules_for_chat_header" as buildRulesText composes it, and the "rules_for_chat" template), what
// comes before the rules text, ending in the blank line. The title is escaped as /rules escapes it.
func settingsEditRulesHeadings(title string) []string {
	escaped := html.EscapeString(title)
	var headings []string
	for _, lang := range settingsEditRulesLanguages {
		tr := i18n.MustNewTranslator(lang)
		if header, _ := tr.GetString("rules_for_chat_header"); header != "" {
			headings = append(headings, fmt.Sprintf(header, escaped)+"\n\n")
		}
		if template, _ := tr.GetString("rules_for_chat"); template != "" {
			headings = append(headings, fmt.Sprintf(template, escaped, ""))
		}
	}
	return headings
}

// settingsEditCheckMedia accepts exactly one message with media of a kind the commands store (a
// video note, which cannot carry the text, and everything that is not media are refused) and
// holds the stored text to the caption limit, because the text is sent as its caption.
func settingsEditCheckMedia(in settingsEditCheckInput) settingsEditVerdict {
	if in.Msg.Text != "" {
		return settingsEditRefusal("settings_edit_refuse_need_media", nil)
	}
	fileID, dataType := content.ExtractMedia(in.Msg)
	if dataType == -1 || dataType == db.VIDEO_NOTE {
		return settingsEditRefusal("settings_edit_refuse_need_media", nil)
	}
	stored := in.State.Text
	if content.TextLengthRefusal(stored, dataType, in.Language) != "" {
		return settingsEditOverLimit("settings_edit_refuse_media_text_long", utf8.RuneCountInString(stored), settingsEditCaptionLimit)
	}
	if longest, _ := settingsEditFilledLongest(in, stored); longest > settingsEditCaptionLimit {
		return settingsEditOverLimit("settings_edit_refuse_media_text_long", longest, settingsEditCaptionLimit)
	}
	return settingsEditVerdict{OK: true, FileID: fileID, DataType: dataType, CaptionIgnored: in.Msg.Caption != ""}
}

// settingsEditCheckButtons reads the button lines of a typed message with the commands' parser
// and refuses what the commands would drop silently or Telegram would refuse at a join.
func settingsEditCheckButtons(in settingsEditCheckInput) settingsEditVerdict {
	if in.Msg.Text == "" {
		return settingsEditRefusal("settings_edit_refuse_need_text", nil)
	}
	var parsed []tgmd2html.ButtonV2
	if _, ok := settingsEditConvert(func() string {
		_, parsed = tgmd2html.MD2HTMLButtonsV2(in.Msg.OriginalMDV2())
		return ""
	}); !ok {
		return settingsEditRefusal("settings_edit_refuse_need_text", nil)
	}
	if len(parsed) == 0 {
		return settingsEditRefusal("settings_edit_refuse_buttons_syntax", nil)
	}
	for i := range parsed {
		if parsed[i].Name == "" {
			parsed[i].Name = settingsEditDefaultButtonName
		}
	}
	for i, button := range parsed {
		if !content.ValidButtonURL(button.Content) {
			return settingsEditRefusal("settings_edit_refuse_button_url", i18n.TranslationParams{"n": i + 1})
		}
	}
	buttons := content.ConvertButtonV2ToDbButton(parsed)
	total, widest := settingsEditButtonRows(buttons, in.State.Text)
	if total > settingsEditMaxButtons || widest > settingsEditMaxRowButtons {
		return settingsEditRefusal("settings_edit_refuse_buttons_limit", nil)
	}
	return settingsEditVerdict{OK: true, Buttons: buttons}
}

// settingsEditButtonRows counts what a join sends: the buttons in all and the widest row, rows
// grouped as keyboard.BuildKeyboard groups them. A {rules} token in the stored text adds the
// rules button where the join puts it (last, on the last row for {rules:same}, or first for
// {rules:up}), counted even though a group with no rules would not show it.
func settingsEditButtonRows(buttons []db.Button, storedText string) (total, widest int) {
	all := buttons
	if token := settingsEditRulesTokenPattern.FindStringSubmatch(storedText); token != nil {
		rulesButton := db.Button{Name: "Rules", Url: "https://t.me/bot", SameLine: token[2] == "same"}
		if token[2] == "up" {
			all = append([]db.Button{rulesButton}, buttons...)
		} else {
			all = append(append([]db.Button(nil), buttons...), rulesButton)
		}
	}
	for _, row := range keyboard.BuildKeyboard(all) {
		if len(row) > widest {
			widest = len(row)
		}
	}
	return len(all), widest
}
