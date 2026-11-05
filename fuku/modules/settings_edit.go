package modules

import (
	"context"
	"errors"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/greetings"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/db/rules"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/formatting"
	"github.com/uasneppy/fuku_robot/fuku/utils/media"
)

// This file is the message editor's pages: Welcome, Goodbye and Rules. Each page shows which
// parts of the message are set, one line per part from one parts table, and the Welcome and
// Goodbye pages carry the on/off switch and Delete last. The page never echoes the stored text,
// a button name or a link, and never reads the Staff Group. Later plans add their rows (Set,
// Cancel, Remove, the previews) as further blocks of renderSettingsEditor.

const (
	// settingsPageWelcome, settingsPageGoodbye and settingsPageRules are the p values of the
	// three editor pages.
	settingsPageWelcome = "w"
	settingsPageGoodbye = "g"
	settingsPageRules   = "r"

	// settingsEditPartText, settingsEditPartMedia and settingsEditPartButtons are the t values
	// of the three parts of a message.
	settingsEditPartText    = "x"
	settingsEditPartMedia   = "m"
	settingsEditPartButtons = "b"

	// settingsEditControlEnabled and settingsEditControlClean are the t values of the on/off
	// switch and of Delete last.
	settingsEditControlEnabled = "e"
	settingsEditControlClean   = "d"

	// settingsEditControlOnce is the t value of the Welcome page's Greet once switch.
	settingsEditControlOnce = "o"

	// settingsEditControlBelow is the t value of Media under text on the Welcome and Goodbye pages.
	settingsEditControlBelow = "u"
)

// settingsEditKind says which message an editor page edits.
type settingsEditKind int

const (
	settingsEditWelcome settingsEditKind = iota
	settingsEditGoodbye
	settingsEditRules
)

// settingsEditPart is one row of the parts table: the part's code (its t value), the locale key
// of the line that shows its state, the keys of its Set, Remove and Preview labels, the notice a
// Remove shows and the body of its waiting prompt.
type settingsEditPart struct {
	Code, LineKey                           string
	SetLabelKey, RemoveLabelKey, RemovedKey string
	WaitKey, PreviewLabelKey                string
}

// settingsEditSwitch is one switch of an editor page: its control (the t value), the key of the
// button label (the state icon alone, spliced into {state}), the key of the body line that says the
// state in words, the key of its hint paragraph (empty for none), the right a press needs, the
// state's value, whether the page draws it now (nil means always), the key of the warning a press
// shows, with nothing written, when the cell is no longer drawn (UnavailableKey) and the apply
// function that writes it. Apply returns the outcome and the state read back (nil when the caller
// must reload).
type settingsEditSwitch struct {
	Control, LabelKey, LineKey, HintKey string
	UnavailableKey                      string
	Right                               settingsRight
	Value                               func(settingsEditState) bool
	Drawn                               func(settingsEditState) bool
	Apply                               func(ctx context.Context, def settingsEditPageDef, chatID int64, on bool) (greetingApplyOutcome, *settingsEditState)
}

// settingsEditPageDef is everything that differs between the three pages. The renderer and
// the controls read only this table, so a page or a part is one entry and never new code.
type settingsEditPageDef struct {
	Key   string
	Kind  settingsEditKind
	Order int
	Icon  string
	// TitleKey is the page name; HintKey the hint under the part lines.
	TitleKey, HintKey string
	// Switches is the page's switch table in grid order (none on Rules).
	Switches []settingsEditSwitch
	// SetCode, RemoveCode and CancelCode are the page's action codes for Set, Remove and Cancel;
	// PreviewCode is its Preview of one part and FullCode its Full preview.
	SetCode, RemoveCode, CancelCode, PreviewCode, FullCode string
	// EditRight is the right Set and Remove need, the one the matching commands need.
	EditRight settingsRight
	Parts     []settingsEditPart
}

var (
	settingsEditTextPart = settingsEditPart{
		Code: settingsEditPartText, LineKey: "settings_edit_line_text",
		SetLabelKey: "settings_btn_edit_set_text", RemoveLabelKey: "settings_btn_edit_remove_text",
		RemovedKey: "settings_edit_removed_text_default", WaitKey: "settings_edit_wait_text",
		PreviewLabelKey: "settings_btn_edit_preview_text",
	}
	settingsEditRulesTextPart = settingsEditPart{
		Code: settingsEditPartText, LineKey: "settings_edit_line_text",
		SetLabelKey: "settings_btn_edit_set_text", RemoveLabelKey: "settings_btn_edit_remove_text",
		RemovedKey: "settings_edit_removed_text_rules", WaitKey: "settings_edit_wait_text_rules",
		PreviewLabelKey: "settings_btn_edit_preview_text",
	}
	settingsEditMediaPart = settingsEditPart{
		Code: settingsEditPartMedia, LineKey: "settings_edit_line_media",
		SetLabelKey: "settings_btn_edit_set_media", RemoveLabelKey: "settings_btn_edit_remove_media",
		RemovedKey: "settings_edit_removed_media", WaitKey: "settings_edit_wait_media",
		PreviewLabelKey: "settings_btn_edit_preview_media",
	}
	settingsEditButtonsPart = settingsEditPart{
		Code: settingsEditPartButtons, LineKey: "settings_edit_line_buttons",
		SetLabelKey: "settings_btn_edit_set_buttons", RemoveLabelKey: "settings_btn_edit_remove_buttons",
		RemovedKey: "settings_edit_removed_buttons", WaitKey: "settings_edit_wait_buttons",
		PreviewLabelKey: "settings_btn_edit_preview_buttons",
	}
)

// The switch cells of the pages. Each calls the apply function of the command it mirrors
// (/welcome on|off, /goodbye on|off, /cleanwelcome, /cleangoodbye); Greet once and Media under text
// have no command, so their one write paths are applyWelcomeOnce(ctx, chatID, on),
// applyWelcomeMediaBelow(ctx, chatID, on) and applyGoodbyeMediaBelow(ctx, chatID, on). Media under
// text needs the page's EditRight.
var (
	settingsEditWelcomeSwitch = settingsEditSwitch{
		Control: settingsEditControlEnabled, LabelKey: "settings_btn_edit_switch_welcome",
		LineKey: "settings_edit_line_welcome", Right: settingsRightAdmin,
		Value: func(st settingsEditState) bool { return st.Enabled },
		Apply: settingsEditGreetingApply(applyWelcomeEnabled),
	}
	settingsEditGoodbyeSwitch = settingsEditSwitch{
		Control: settingsEditControlEnabled, LabelKey: "settings_btn_edit_switch_goodbye",
		LineKey: "settings_edit_line_goodbye", Right: settingsRightAdmin,
		Value: func(st settingsEditState) bool { return st.Enabled },
		Apply: settingsEditGreetingApply(applyGoodbyeEnabled),
	}
	settingsEditCleanWelcomeSwitch = settingsEditSwitch{
		Control: settingsEditControlClean, LabelKey: "settings_btn_edit_switch_clean",
		LineKey: "settings_edit_line_clean_welcome", HintKey: "settings_edit_clean_hint_welcome",
		Right: settingsRightChangeInfo,
		Value: func(st settingsEditState) bool { return st.Clean },
		Apply: settingsEditGreetingApply(applyCleanWelcome),
	}
	settingsEditCleanGoodbyeSwitch = settingsEditSwitch{
		Control: settingsEditControlClean, LabelKey: "settings_btn_edit_switch_clean",
		LineKey: "settings_edit_line_clean_goodbye", HintKey: "settings_edit_clean_hint_goodbye",
		Right: settingsRightChangeInfo,
		Value: func(st settingsEditState) bool { return st.Clean },
		Apply: settingsEditGreetingApply(applyCleanGoodbye),
	}
	settingsEditWelcomeBelowSwitch = settingsEditSwitch{
		Control: settingsEditControlBelow, LabelKey: "settings_btn_edit_switch_below",
		LineKey: "settings_edit_line_below", UnavailableKey: "settings_edit_below_unavailable",
		Right: settingsRightChangeInfo,
		Value: func(st settingsEditState) bool { return st.MediaBelow },
		Drawn: settingsEditMediaBelowEligible,
		Apply: settingsEditGreetingApply(applyWelcomeMediaBelow),
	}
	settingsEditGoodbyeBelowSwitch = settingsEditSwitch{
		Control: settingsEditControlBelow, LabelKey: "settings_btn_edit_switch_below",
		LineKey: "settings_edit_line_below", UnavailableKey: "settings_edit_below_unavailable",
		Right: settingsRightChangeInfo,
		Value: func(st settingsEditState) bool { return st.MediaBelow },
		Drawn: settingsEditMediaBelowEligible,
		Apply: settingsEditGreetingApply(applyGoodbyeMediaBelow),
	}
	settingsEditOnceSwitch = settingsEditSwitch{
		Control: settingsEditControlOnce, LabelKey: "settings_btn_edit_switch_once",
		LineKey: "settings_edit_line_once", HintKey: "settings_edit_hint_once",
		Right: settingsRightAdmin,
		Value: func(st settingsEditState) bool { return st.Once },
		Apply: settingsEditGreetingApply(applyWelcomeOnce),
	}
)

// settingsEditPages are the three definitions in page order.
var settingsEditPages = []settingsEditPageDef{
	{
		Key: settingsPageWelcome, Kind: settingsEditWelcome, Order: 50, Icon: "🎉",
		TitleKey: "settings_page_welcome", HintKey: "settings_edit_hint_greeting",
		Switches: []settingsEditSwitch{settingsEditWelcomeSwitch, settingsEditCleanWelcomeSwitch, settingsEditWelcomeBelowSwitch, settingsEditOnceSwitch},
		SetCode:  "ws", RemoveCode: "wr", CancelCode: "wc", PreviewCode: "wv", FullCode: "wf", EditRight: settingsRightChangeInfo,
		Parts: []settingsEditPart{settingsEditTextPart, settingsEditMediaPart, settingsEditButtonsPart},
	},
	{
		Key: settingsPageGoodbye, Kind: settingsEditGoodbye, Order: 60, Icon: "👋",
		TitleKey: "settings_page_goodbye", HintKey: "settings_edit_hint_greeting",
		Switches: []settingsEditSwitch{settingsEditGoodbyeSwitch, settingsEditCleanGoodbyeSwitch, settingsEditGoodbyeBelowSwitch},
		SetCode:  "gs", RemoveCode: "gr", CancelCode: "gc", PreviewCode: "gv", FullCode: "gf", EditRight: settingsRightChangeInfo,
		Parts: []settingsEditPart{settingsEditTextPart, settingsEditMediaPart, settingsEditButtonsPart},
	},
	{
		Key: settingsPageRules, Kind: settingsEditRules, Order: 70, Icon: "📜",
		TitleKey: "settings_page_rules", HintKey: "settings_edit_hint_rules",
		SetCode: "rs", RemoveCode: "rr", CancelCode: "rc", PreviewCode: "rv", FullCode: "rf", EditRight: settingsRightAdminBotAdmin,
		Parts: []settingsEditPart{settingsEditRulesTextPart},
	},
}

// The editor pages read through the repository's reads that create nothing and return their
// errors. They are variables only so a test can make a read fail.
var (
	settingsEditGreetingsRead = greetings.LoadSettingsContext
	settingsEditRulesRead     = rules.LoadRulesContext
)

// settingsEditPageByKey finds an editor page definition.
func settingsEditPageByKey(key string) (settingsEditPageDef, bool) {
	for _, def := range settingsEditPages {
		if def.Key == key {
			return def, true
		}
	}
	return settingsEditPageDef{}, false
}

// settingsEditState is what a page shows: the stored parts and switches of one message.
type settingsEditState struct {
	Kind settingsEditKind
	// Text is the stored text; TextIsDefault is true when it is the built-in default text.
	Text          string
	TextIsDefault bool
	MediaType     int
	FileID        string
	Buttons       models.ButtonArray
	// Enabled and Clean are the switch and Delete last (false on Rules); Once is Greet once
	// (Welcome only).
	Enabled, Clean, Once bool
	// MediaBelow is Media under text (Welcome and Goodbye); it is kept while the stored media is of a
	// kind that cannot take it, and then neither drawn nor sent.
	MediaBelow bool
}

// settingsEditScreenOpts are the extras of one drawing: a notice line and the bot's username
// for the Open in private row of a group menu.
type settingsEditScreenOpts struct {
	Notice, BotUsername string
}

// settingsEditStateOf turns a stored greetings row into the state of a Welcome or Goodbye page.
func settingsEditStateOf(def settingsEditPageDef, row *models.GreetingSettings) (settingsEditState, error) {
	st := settingsEditState{Kind: def.Kind}
	switch def.Kind {
	case settingsEditWelcome:
		w := row.WelcomeSettings
		if w == nil {
			return st, errors.New("welcome settings: row without a welcome part")
		}
		st.Text, st.TextIsDefault = w.WelcomeText, w.WelcomeText == "" || w.WelcomeText == db.DefaultWelcome
		st.MediaType, st.FileID, st.Buttons = w.WelcomeType, w.FileID, w.Button
		st.Enabled, st.Clean, st.Once, st.MediaBelow = w.ShouldWelcome, w.CleanWelcome, w.Once, w.MediaBelow
	case settingsEditGoodbye:
		g := row.GoodbyeSettings
		if g == nil {
			return st, errors.New("goodbye settings: row without a goodbye part")
		}
		st.Text, st.TextIsDefault = g.GoodbyeText, g.GoodbyeText == "" || g.GoodbyeText == db.DefaultGoodbye
		st.MediaType, st.FileID, st.Buttons = g.GoodbyeType, g.FileID, g.Button
		st.Enabled, st.Clean, st.MediaBelow = g.ShouldGoodbye, g.CleanGoodbye, g.MediaBelow
	default:
		return st, errors.New("settings editor: not a greetings page")
	}
	return st, nil
}

// settingsEditLoad reads the stored message of a page, creating nothing. The read error is
// returned unchanged, so a failed read is never drawn as a default.
func settingsEditLoad(ctx context.Context, def settingsEditPageDef, chatID int64) (settingsEditState, error) {
	if def.Kind == settingsEditRules {
		text, err := settingsEditRulesRead(ctx, chatID)
		if err != nil {
			return settingsEditState{}, err
		}
		return settingsEditState{Kind: def.Kind, Text: text}, nil
	}
	row, err := settingsEditGreetingsRead(ctx, chatID)
	if err != nil {
		return settingsEditState{}, err
	}
	if row == nil {
		return settingsEditState{}, errors.New("greeting settings: no row and no default")
	}
	return settingsEditStateOf(def, row)
}

// settingsEditMediaKey is the locale key of the kind word of a stored media type, or "" for
// no media. A GIF saved from Phase 12 on is an animation, exactly as the commands store it; one saved earlier
// is a document and reads File or GIF.
func settingsEditMediaKey(mediaType int) string {
	switch mediaType {
	case db.PHOTO:
		return "settings_edit_media_photo"
	case db.VIDEO:
		return "settings_edit_media_video"
	case db.DOCUMENT:
		return "settings_edit_media_file"
	case db.ANIMATION:
		return "settings_edit_media_animation"
	case db.STICKER:
		return "settings_edit_media_sticker"
	case db.AUDIO:
		return "settings_edit_media_audio"
	case db.VOICE:
		return "settings_edit_media_voice"
	case db.VIDEO_NOTE:
		return "settings_edit_media_video_note"
	}
	return ""
}

// settingsEditPartIsSet reports whether the part holds something a Remove would take away.
func settingsEditPartIsSet(st settingsEditState, part string) bool {
	switch part {
	case settingsEditPartText:
		if st.Kind == settingsEditRules {
			return st.Text != ""
		}
		return !st.TextIsDefault
	case settingsEditPartMedia:
		return st.FileID != "" && settingsEditMediaKey(st.MediaType) != ""
	case settingsEditPartButtons:
		return len(st.Buttons) > 0
	}
	return false
}

// settingsEditStateWord is the state of one part in words: never the stored text, a button
// name or a link, only a kind, a length or a count.
func settingsEditStateWord(tr *i18n.Translator, st settingsEditState, part string) string {
	notSet := settingsText(tr, "settings_state_not_set")
	switch part {
	case settingsEditPartText:
		switch {
		case st.Kind != settingsEditRules && st.TextIsDefault:
			return settingsText(tr, "settings_state_default")
		case !settingsEditPartIsSet(st, part):
			return notSet
		}
		return settingsText(tr, "settings_edit_state_text_len", i18n.TranslationParams{
			"n": formatting.VisibleLength(st.Text),
		})
	case settingsEditPartMedia:
		if !settingsEditPartIsSet(st, part) {
			return notSet
		}
		return settingsText(tr, settingsEditMediaKey(st.MediaType))
	case settingsEditPartButtons:
		if !settingsEditPartIsSet(st, part) {
			return notSet
		}
		return settingsText(tr, "settings_edit_state_buttons_n", i18n.TranslationParams{"n": len(st.Buttons)})
	}
	return notSet
}

// settingsEditMediaBelowEligible reports whether Media under text is drawn for the state: media is
// stored and it is a photo, a video or an animation. The kind test is the sender's own predicate, so
// the page, the stale-press check and the send cannot disagree.
func settingsEditMediaBelowEligible(st settingsEditState) bool {
	return st.FileID != "" && media.CaptionAboveEligible(st.MediaType)
}

// isGreeting reports whether the page has the on/off switch and Delete last.
func (def settingsEditPageDef) isGreeting() bool {
	return def.Kind != settingsEditRules
}

// settingsEditBody is the page's text lines: one line per part, the note for a media kind that
// carries no caption, then one blank line and a state line per drawn switch (none when no switch
// is drawn), a blank line and the hint, a blank line and the hint of each drawn switch that
// has one, in the grid's order, and last, while the stored media is a document (a GIF saved before
// Phase 12 reads File or GIF), the hint that Media under text needs a photo, a video or a GIF.
func settingsEditBody(v settingsView, def settingsEditPageDef, st settingsEditState) []string {
	var body []string
	for _, part := range def.Parts {
		body = append(body, settingsText(v.Tr, part.LineKey, i18n.TranslationParams{
			"state": settingsEditStateWord(v.Tr, st, part.Code),
		}))
	}
	if settingsEditPartIsSet(st, settingsEditPartMedia) && (st.MediaType == db.STICKER || st.MediaType == db.VIDEO_NOTE) {
		body = append(body, settingsText(v.Tr, "settings_edit_note_no_caption"))
	}
	cells := settingsEditSwitchCells(def, st)
	if len(cells) > 0 {
		body = append(body, "")
	}
	for _, cell := range cells {
		body = append(body, settingsText(v.Tr, cell.LineKey, i18n.TranslationParams{
			"state": settingsStateWord(v.Tr, cell.Value(st)),
		}))
	}
	body = append(body, "", settingsText(v.Tr, def.HintKey))
	for _, cell := range cells {
		if cell.HintKey != "" {
			body = append(body, "", settingsText(v.Tr, cell.HintKey))
		}
	}
	if settingsEditPartIsSet(st, settingsEditPartMedia) && st.MediaType == db.DOCUMENT {
		body = append(body, "", settingsText(v.Tr, "settings_edit_hint_below_file"))
	}
	return body
}

// settingsEditPartRows is the block of the part rows, one per part in the parts table's order:
// [Set] while the part is not set, [Set | Remove] while it is. The buttons are drawn in a group
// menu too, so the page looks the same in both places; there their press is the group note.
func settingsEditPartRows(k *settingsKeyboard, def settingsEditPageDef, st settingsEditState) {
	tr := k.view.Tr
	for _, part := range def.Parts {
		var row []gotgbot.InlineKeyboardButton
		if set, ok := k.button(settingsText(tr, part.SetLabelKey), map[string]string{"a": def.SetCode, "t": part.Code}); ok {
			row = append(row, set)
		}
		if settingsEditPartIsSet(st, part.Code) {
			if remove, ok := k.button(settingsText(tr, part.RemoveLabelKey), map[string]string{"a": def.RemoveCode, "t": part.Code}); ok {
				row = append(row, remove)
			}
		}
		k.row(row...)
	}
}

// settingsEditPreviewable reports whether a part has something to preview. A Welcome or Goodbye
// text always has, because the default text is a message too; every other part only while it is
// set.
func settingsEditPreviewable(def settingsEditPageDef, st settingsEditState, part string) bool {
	return (def.isGreeting() && part == settingsEditPartText) || settingsEditPartIsSet(st, part)
}

// settingsEditPreviewRows is the block of the preview buttons, right after the part rows: Preview
// for each part that has something to show, in the parts table's order, then Full preview, always
// last, two buttons to a row.
func settingsEditPreviewRows(k *settingsKeyboard, def settingsEditPageDef, st settingsEditState) {
	tr := k.view.Tr
	var buttons []gotgbot.InlineKeyboardButton
	for _, part := range def.Parts {
		if !settingsEditPreviewable(def, st, part.Code) {
			continue
		}
		if button, ok := k.button(settingsText(tr, part.PreviewLabelKey), map[string]string{"a": def.PreviewCode, "t": part.Code}); ok {
			buttons = append(buttons, button)
		}
	}
	if full, ok := k.button(settingsText(tr, "settings_btn_edit_full_preview"), map[string]string{"a": def.FullCode}); ok {
		buttons = append(buttons, full)
	}
	for i := 0; i < len(buttons); i += 2 {
		k.row(buttons[i:min(i+2, len(buttons))]...)
	}
}

// settingsEditSwitchCells is the page's switches that are drawn for the state, in grid order.
func settingsEditSwitchCells(def settingsEditPageDef, st settingsEditState) []settingsEditSwitch {
	var cells []settingsEditSwitch
	for _, cell := range def.Switches {
		if cell.Drawn == nil || cell.Drawn(st) {
			cells = append(cells, cell)
		}
	}
	return cells
}

// settingsEditSwitchGrid is the block of the switches: a two-column grid of half-width cells
// filled left to right, an odd last cell alone on its row. Each button is the state icon and the
// name, and sends the absolute target value, so a double tap writes the same value.
func settingsEditSwitchGrid(k *settingsKeyboard, def settingsEditPageDef, st settingsEditState) {
	tr := k.view.Tr
	var buttons []gotgbot.InlineKeyboardButton
	for _, cell := range settingsEditSwitchCells(def, st) {
		on := cell.Value(st)
		label := settingsText(tr, cell.LabelKey, i18n.TranslationParams{"state": settingsStateIcon(on)})
		buttons = append(buttons, settingsToggleButton(k, def.Key, cell.Control, label, on)...)
	}
	for i := 0; i < len(buttons); i += 2 {
		k.row(buttons[i:min(i+2, len(buttons))]...)
	}
}

// settingsEditOpenPrivateRow is the block of the Open in private URL row of a group menu: the
// existing /start settings_<chat id> deep link. It is left out, and logged, when the bot has no
// username; a private menu has none.
func settingsEditOpenPrivateRow(k *settingsKeyboard, opts settingsEditScreenOpts) {
	if k.view.Private {
		return
	}
	if opts.BotUsername == "" {
		log.Error("[Settings] the bot has no username, so the Open in private button is left out")
		return
	}
	k.row(gotgbot.InlineKeyboardButton{
		Text: settingsText(k.view.Tr, "settings_btn_open_private"),
		Url:  settingsPrivateURL(opts.BotUsername, k.view.GroupID),
	})
}

// renderSettingsEditor draws an editor page. It is pure. The keyboard is built in row blocks
// (part rows, switches, Open in private, navigation) so a later plan adds a block without
// touching the others.
func renderSettingsEditor(v settingsView, def settingsEditPageDef, st settingsEditState, opts settingsEditScreenOpts) settingsScreen {
	body := settingsEditBody(v, def, st)
	k := newSettingsKeyboard(v)
	settingsEditPartRows(k, def, st)
	settingsEditPreviewRows(k, def, st)
	settingsEditSwitchGrid(k, def, st)
	settingsEditOpenPrivateRow(k, opts)
	k.navRow(false)
	return k.screen(settingsCompose(v, def.TitleKey, body, opts.Notice, false))
}

// settingsEditMainState is the page's state on Main: On or Off for Welcome and Goodbye, Set or
// Not set for Rules. ok is false when the read failed.
func settingsEditMainState(def settingsEditPageDef) func(*gotgbot.Bot, *i18n.Translator, *gotgbot.Chat) (string, bool) {
	return func(_ *gotgbot.Bot, tr *i18n.Translator, group *gotgbot.Chat) (string, bool) {
		ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
		defer cancel()
		st, err := settingsEditLoad(ctx, def, group.Id)
		if err != nil {
			return "", false
		}
		if def.isGreeting() {
			if st.Enabled {
				return settingsText(tr, "common_status_on"), true
			}
			return settingsText(tr, "common_status_off"), true
		}
		if settingsEditPartIsSet(st, settingsEditPartText) {
			return settingsText(tr, "settings_state_set"), true
		}
		return settingsText(tr, "settings_state_not_set"), true
	}
}

// settingsEditOpen reads the stored message and draws the page. A read error is returned,
// which the dispatcher turns into the cannot-open edit.
func settingsEditOpen(def settingsEditPageDef) func(*settingsPress, string) (settingsScreen, error) {
	return func(p *settingsPress, notice string) (settingsScreen, error) {
		ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
		defer cancel()
		st, err := settingsEditLoad(ctx, def, p.group.Id)
		if err != nil {
			return settingsScreen{}, err
		}
		return renderSettingsEditor(p.view(), def, st, settingsEditScreenOpts{
			Notice: notice, BotUsername: getBotUsername(p.b),
		}), nil
	}
}

// settingsEditGreetingApply adapts a greeting apply function (the one beside its command) to a
// switch cell: the outcome, and the state read back from the row the function returned, nil when
// the write failed or the read-back did, in which case the caller reloads the page.
func settingsEditGreetingApply(
	apply func(context.Context, int64, bool) greetingApplyResult,
) func(context.Context, settingsEditPageDef, int64, bool) (greetingApplyOutcome, *settingsEditState) {
	return func(ctx context.Context, def settingsEditPageDef, chatID int64, on bool) (greetingApplyOutcome, *settingsEditState) {
		result := apply(ctx, chatID, on)
		if result.Outcome == greetingSaveFailed {
			log.Errorf("[Settings] %s switch of chat %d failed: %v", def.Key, chatID, result.Err)
			return greetingSaveFailed, nil
		}
		if result.Settings == nil {
			return result.Outcome, nil
		}
		st, err := settingsEditStateOf(def, result.Settings)
		if err != nil {
			log.Warnf("[Settings] %s page of chat %d could not be read back after a press: %v", def.Key, chatID, err)
			return result.Outcome, nil
		}
		return result.Outcome, &st
	}
}

// settingsEditSwitchByControl finds the switch of a page by its control.
func settingsEditSwitchByControl(def settingsEditPageDef, control string) (settingsEditSwitch, bool) {
	for _, cell := range def.Switches {
		if cell.Control == control {
			return cell, true
		}
	}
	return settingsEditSwitch{}, false
}

// settingsEditSetSwitch serves a switch control of an editor page: v is "0" or "1" (anything else
// is an invalid request), the press calls the cell's apply function and the page is redrawn from
// the row the function read back, with no notice. A failed write redraws the freshly loaded page,
// with the old state and the save-failed notice.
func settingsEditSetSwitch(def settingsEditPageDef, control string) func(*settingsPress, string) *settingsScreen {
	cell, found := settingsEditSwitchByControl(def, control)
	return func(p *settingsPress, value string) *settingsScreen {
		if !found || !settingsEditSwitchValue(value) {
			p.answer(settingsText(p.tr, "common_callback_invalid_request"), true)
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
		defer cancel()

		if cell.Drawn != nil {
			if screen, stop := settingsEditStaleSwitch(ctx, p, def, cell); stop {
				return screen
			}
		}
		outcome, st := cell.Apply(ctx, def, p.group.Id, value == "1")
		notice := ""
		if outcome == greetingSaveFailed {
			notice = settingsNoticeWarn(settingsText(p.tr, "common_settings_save_failed"))
		}
		if st == nil {
			loaded, err := settingsEditLoad(ctx, def, p.group.Id)
			if err != nil {
				log.Warnf("[Settings] %s page of chat %d could not be read after a press: %v", def.Key, p.group.Id, err)
				p.cannotOpen()
				return nil
			}
			st = &loaded
		}
		screen := renderSettingsEditor(p.view(), def, *st, settingsEditScreenOpts{
			Notice: notice, BotUsername: getBotUsername(p.b),
		})
		return &screen
	}
}

// settingsEditStaleSwitch is the check of a press of a switch that is drawn only for some states:
// the page is loaded fresh and, when the cell is not drawn for it (the media changed since the page
// was drawn), nothing is written and the page is redrawn with the cell's unavailable warning. It
// stop is false when the press may go on. When it is true the press is over: the screen is the
// redrawn page, or nil when the page could not be read and was already answered as the cannot-open
// edit.
func settingsEditStaleSwitch(ctx context.Context, p *settingsPress, def settingsEditPageDef, cell settingsEditSwitch) (screen *settingsScreen, stop bool) {
	loaded, err := settingsEditLoad(ctx, def, p.group.Id)
	if err != nil {
		log.Warnf("[Settings] %s page of chat %d could not be read before a press: %v", def.Key, p.group.Id, err)
		p.cannotOpen()
		return nil, true
	}
	if cell.Drawn(loaded) {
		return nil, false
	}
	redrawn := renderSettingsEditor(p.view(), def, loaded, settingsEditScreenOpts{
		Notice: settingsNoticeWarn(settingsText(p.tr, cell.UnavailableKey)), BotUsername: getBotUsername(p.b),
	})
	return &redrawn, true
}

// settingsEditSwitchValue is the strict check of an editor switch's target value: exactly "0" or
// "1". It is the Validate of every switch control, so any other value is an invalid request in
// step 1, before any lookup.
func settingsEditSwitchValue(value string) bool {
	return value == "0" || value == "1"
}

func init() {
	for _, def := range settingsEditPages {
		pg := settingsPage{
			Key:       def.Key,
			Order:     def.Order,
			Icon:      def.Icon,
			TitleKey:  def.TitleKey,
			MainState: settingsEditMainState(def),
			Open:      settingsEditOpen(def),
			Actions:   settingsEditActions(def),
		}
		if len(def.Switches) > 0 {
			pg.Controls = make(map[string]settingsControl, len(def.Switches))
			for _, cell := range def.Switches {
				pg.Controls[cell.Control] = settingsControl{
					Right: cell.Right, Validate: settingsEditSwitchValue, Set: settingsEditSetSwitch(def, cell.Control),
				}
			}
		}
		registerSettingsPage(pg)
	}
}
