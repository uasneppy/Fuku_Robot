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
)

// settingsEditKind says which message an editor page edits.
type settingsEditKind int

const (
	settingsEditWelcome settingsEditKind = iota
	settingsEditGoodbye
	settingsEditRules
)

// settingsEditPart is one row of the parts table: the part's code (its t value), the locale key
// of the line that shows its state, the keys of its Set and Remove labels, the notice a Remove
// shows and the body of its waiting prompt.
type settingsEditPart struct {
	Code, LineKey                           string
	SetLabelKey, RemoveLabelKey, RemovedKey string
	WaitKey                                 string
}

// settingsEditPageDef is everything that differs between the three pages. The renderer and
// the controls read only this table, so a page or a part is one entry and never new code.
type settingsEditPageDef struct {
	Key   string
	Kind  settingsEditKind
	Order int
	Icon  string
	// TitleKey is the page name; HintKey the hint under the part lines; CleanHintKey the Delete
	// last hint (empty on Rules).
	TitleKey, HintKey, CleanHintKey string
	// ToggleLabelKey and CleanLabelKey are the labels of the on/off switch and of Delete last
	// (empty on Rules, which has neither).
	ToggleLabelKey, CleanLabelKey string
	// SetCode, RemoveCode and CancelCode are the page's action codes for Set, Remove and Cancel.
	SetCode, RemoveCode, CancelCode string
	// EditRight is the right Set and Remove need, the one the matching commands need.
	EditRight settingsRight
	Parts     []settingsEditPart
}

var (
	settingsEditTextPart = settingsEditPart{
		Code: settingsEditPartText, LineKey: "settings_edit_line_text",
		SetLabelKey: "settings_btn_edit_set_text", RemoveLabelKey: "settings_btn_edit_remove_text",
		RemovedKey: "settings_edit_removed_text_default", WaitKey: "settings_edit_wait_text",
	}
	settingsEditRulesTextPart = settingsEditPart{
		Code: settingsEditPartText, LineKey: "settings_edit_line_text",
		SetLabelKey: "settings_btn_edit_set_text", RemoveLabelKey: "settings_btn_edit_remove_text",
		RemovedKey: "settings_edit_removed_text_rules", WaitKey: "settings_edit_wait_text_rules",
	}
	settingsEditMediaPart = settingsEditPart{
		Code: settingsEditPartMedia, LineKey: "settings_edit_line_media",
		SetLabelKey: "settings_btn_edit_set_media", RemoveLabelKey: "settings_btn_edit_remove_media",
		RemovedKey: "settings_edit_removed_media", WaitKey: "settings_edit_wait_media",
	}
	settingsEditButtonsPart = settingsEditPart{
		Code: settingsEditPartButtons, LineKey: "settings_edit_line_buttons",
		SetLabelKey: "settings_btn_edit_set_buttons", RemoveLabelKey: "settings_btn_edit_remove_buttons",
		RemovedKey: "settings_edit_removed_buttons", WaitKey: "settings_edit_wait_buttons",
	}
)

// settingsEditPages are the three definitions in page order.
var settingsEditPages = []settingsEditPageDef{
	{
		Key: settingsPageWelcome, Kind: settingsEditWelcome, Order: 50, Icon: "🎉",
		TitleKey: "settings_page_welcome", HintKey: "settings_edit_hint_greeting",
		CleanHintKey:   "settings_edit_clean_hint_welcome",
		ToggleLabelKey: "settings_btn_edit_welcome_toggle", CleanLabelKey: "settings_btn_edit_clean_welcome",
		SetCode: "ws", CancelCode: "wc", EditRight: settingsRightChangeInfo,
		Parts: []settingsEditPart{settingsEditTextPart, settingsEditMediaPart, settingsEditButtonsPart},
	},
	{
		Key: settingsPageGoodbye, Kind: settingsEditGoodbye, Order: 60, Icon: "👋",
		TitleKey: "settings_page_goodbye", HintKey: "settings_edit_hint_greeting",
		CleanHintKey:   "settings_edit_clean_hint_goodbye",
		ToggleLabelKey: "settings_btn_edit_goodbye_toggle", CleanLabelKey: "settings_btn_edit_clean_goodbye",
		SetCode: "gs", CancelCode: "gc", EditRight: settingsRightChangeInfo,
		Parts: []settingsEditPart{settingsEditTextPart, settingsEditMediaPart, settingsEditButtonsPart},
	},
	{
		Key: settingsPageRules, Kind: settingsEditRules, Order: 70, Icon: "📜",
		TitleKey: "settings_page_rules", HintKey: "settings_edit_hint_rules",
		SetCode: "rs", CancelCode: "rc", EditRight: settingsRightAdminBotAdmin,
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
	// Enabled and Clean are the switch and Delete last (false on Rules).
	Enabled, Clean bool
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
		st.Enabled, st.Clean = w.ShouldWelcome, w.CleanWelcome
	case settingsEditGoodbye:
		g := row.GoodbyeSettings
		if g == nil {
			return st, errors.New("goodbye settings: row without a goodbye part")
		}
		st.Text, st.TextIsDefault = g.GoodbyeText, g.GoodbyeText == "" || g.GoodbyeText == db.DefaultGoodbye
		st.MediaType, st.FileID, st.Buttons = g.GoodbyeType, g.FileID, g.Button
		st.Enabled, st.Clean = g.ShouldGoodbye, g.CleanGoodbye
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
// no media. A GIF is stored as a document, exactly as the commands store it.
func settingsEditMediaKey(mediaType int) string {
	switch mediaType {
	case db.PHOTO:
		return "settings_edit_media_photo"
	case db.VIDEO:
		return "settings_edit_media_video"
	case db.DOCUMENT:
		return "settings_edit_media_file"
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

// isGreeting reports whether the page has the on/off switch and Delete last.
func (def settingsEditPageDef) isGreeting() bool {
	return def.Kind != settingsEditRules
}

// settingsEditBody is the page's text lines: one line per part, the note for a media kind that
// carries no caption, a blank line, the hint, and the Delete last hint on Welcome and Goodbye.
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
	body = append(body, "", settingsText(v.Tr, def.HintKey))
	if def.isGreeting() {
		body = append(body, "", settingsText(v.Tr, def.CleanHintKey))
	}
	return body
}

// settingsEditPartRows is the block of the part rows, one per part in the parts table's order:
// [Set]. The buttons are drawn in a group
// menu too, so the page looks the same in both places; there their press is the group note.
func settingsEditPartRows(k *settingsKeyboard, def settingsEditPageDef, st settingsEditState) {
	tr := k.view.Tr
	for _, part := range def.Parts {
		var row []gotgbot.InlineKeyboardButton
		if set, ok := k.button(settingsText(tr, part.SetLabelKey), map[string]string{"a": def.SetCode, "t": part.Code}); ok {
			row = append(row, set)
		}
		k.row(row...)
	}
}

// settingsEditSwitchRows is the block of the on/off switch and Delete last. Each button sends
// the absolute target value, so a double tap writes the same value.
func settingsEditSwitchRows(k *settingsKeyboard, def settingsEditPageDef, st settingsEditState) {
	if !def.isGreeting() {
		return
	}
	tr := k.view.Tr
	k.row(settingsToggleButton(k, def.Key, settingsEditControlEnabled,
		settingsText(tr, def.ToggleLabelKey, i18n.TranslationParams{"state": settingsStateWord(tr, st.Enabled)}),
		st.Enabled)...)
	k.row(settingsToggleButton(k, def.Key, settingsEditControlClean,
		settingsText(tr, def.CleanLabelKey, i18n.TranslationParams{"state": settingsStateWord(tr, st.Clean)}),
		st.Clean)...)
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
	settingsEditSwitchRows(k, def, st)
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

// settingsEditApplySwitch calls the apply function of the command the control mirrors:
// /welcome on|off, /goodbye on|off, /cleanwelcome and /cleangoodbye.
func settingsEditApplySwitch(
	ctx context.Context, def settingsEditPageDef, control string, chatID int64, on bool,
) greetingApplyResult {
	switch {
	case def.Kind == settingsEditWelcome && control == settingsEditControlEnabled:
		return applyWelcomeEnabled(ctx, chatID, on)
	case def.Kind == settingsEditGoodbye && control == settingsEditControlEnabled:
		return applyGoodbyeEnabled(ctx, chatID, on)
	case def.Kind == settingsEditWelcome && control == settingsEditControlClean:
		return applyCleanWelcome(ctx, chatID, on)
	case def.Kind == settingsEditGoodbye && control == settingsEditControlClean:
		return applyCleanGoodbye(ctx, chatID, on)
	}
	return greetingApplyResult{Outcome: greetingSaveFailed, Err: errors.New("settings editor: no such switch")}
}

// settingsEditSetSwitch serves a switch control of a Welcome or Goodbye page: v is "0" or "1"
// (anything else is an invalid request), the press calls the command's apply function and the
// page is redrawn from the row the function read back, with no notice. A failed write redraws
// the freshly loaded page, with the old state and the save-failed notice.
func settingsEditSetSwitch(def settingsEditPageDef, control string) func(*settingsPress, string) *settingsScreen {
	return func(p *settingsPress, value string) *settingsScreen {
		if value != "0" && value != "1" {
			p.answer(settingsText(p.tr, "common_callback_invalid_request"), true)
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
		defer cancel()

		result := settingsEditApplySwitch(ctx, def, control, p.group.Id, value == "1")
		notice := ""
		var st settingsEditState
		var err error
		if result.Outcome == greetingSaveFailed {
			log.Errorf("[Settings] %s switch %s of chat %d failed: %v", def.Key, control, p.group.Id, result.Err)
			notice = settingsNoticeWarn(settingsText(p.tr, "common_settings_save_failed"))
			st, err = settingsEditLoad(ctx, def, p.group.Id)
		} else if result.Settings != nil {
			st, err = settingsEditStateOf(def, result.Settings)
		} else {
			st, err = settingsEditLoad(ctx, def, p.group.Id)
		}
		if err != nil {
			log.Warnf("[Settings] %s page of chat %d could not be read after a press: %v", def.Key, p.group.Id, err)
			p.cannotOpen()
			return nil
		}
		screen := renderSettingsEditor(p.view(), def, st, settingsEditScreenOpts{
			Notice: notice, BotUsername: getBotUsername(p.b),
		})
		return &screen
	}
}

// settingsEditSetEnabled serves control e, the on/off switch (as /welcome on|off and
// /goodbye on|off).
func settingsEditSetEnabled(def settingsEditPageDef) func(*settingsPress, string) *settingsScreen {
	return settingsEditSetSwitch(def, settingsEditControlEnabled)
}

// settingsEditSetClean serves control d, Delete last (as /cleanwelcome and /cleangoodbye).
func settingsEditSetClean(def settingsEditPageDef) func(*settingsPress, string) *settingsScreen {
	return settingsEditSetSwitch(def, settingsEditControlClean)
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
		if def.isGreeting() {
			pg.Controls = map[string]settingsControl{
				settingsEditControlEnabled: {Right: settingsRightAdmin, Set: settingsEditSetEnabled(def)},
				settingsEditControlClean:   {Right: settingsRightChangeInfo, Set: settingsEditSetClean(def)},
			}
		}
		registerSettingsPage(pg)
	}
}
