package modules

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// The Deletion page of the settings menu (Phase 16, D-15 to D-18): a hub of sections, one list per
// section and one screen per row, with preset choices only. Everything it draws comes from two
// tables, the sections below and the row tables of deletion_kinds.go, so no renderer, validator or
// gate branches on a row's code or kind. Opening a screen reads the rules through the cached loader
// the watcher and the send sites read, writes nothing and creates no row. Every deletion screen
// carries one status line about the bot's right to delete, from one live paced read of the bot's own
// record per draw; Main makes no Telegram call. The page does not exist inside a Staff Group (D-18).

const (
	// settingsPageDeletion is the p value of the Deletion page.
	settingsPageDeletion = "d"
	// settingsActDelList is the action code that opens a section's list (field t, a section code).
	settingsActDelList = "dl"
	// settingsActDelRow is the action code that opens a row's screen (field t, a row code).
	settingsActDelRow = "dr"
	// settingsDelNone is the v value of "no rule": Keep on a service row, Off on a bot row.
	settingsDelNone = "n"
)

// settingsDelHereTimeout bounds the one live read of the bot's right that a draw makes, the wait
// for the pacer slot included. A variable so a test can shorten it.
var settingsDelHereTimeout = 3 * time.Second

// errSettingsDelBlocked is returned by Open for a chat that is a Staff Group or whose Staff Group
// row cannot be read: the cannot-open edit, never a text that names the Staff Group.
var errSettingsDelBlocked = errors.New("the deletion page does not open here")

// settingsDelSection is one section of the hub: a list of rows. Phase 17 adds a section as an entry
// here and its rows; nothing else changes.
type settingsDelSection struct {
	// Code is the t value of the dl action.
	Code string
	Icon string
	// NameKey is the section's name, used for the hub cell and its state line.
	NameKey string
	// ListIntroKey is the first line of the section's list.
	ListIntroKey string
	// ListHintKeys are the hint lines of the section's list, before the time hint.
	ListHintKeys []string
	// NoneKey is the word of "no rule" in this section: Keep for service messages, Off for bot messages.
	NoneKey string
	// ChoiceRows is the width of each choice row of a row screen, laid over the "no rule" choice
	// followed by the row's presets.
	ChoiceRows []int
	Rows       []deletionRow
}

// settingsDelSections are the sections of the hub in page order. It is a variable only so a test can
// draw a hub with more sections; production code never reassigns it.
var settingsDelSections = []settingsDelSection{
	{
		Code: "s", Icon: "🔔", NameKey: "settings_del_sec_service", ListIntroKey: "settings_del_list_svc",
		ListHintKeys: []string{"settings_del_hint_cleanservice"},
		NoneKey:      "settings_del_state_keep", ChoiceRows: []int{2, 3, 3}, Rows: deletionServiceRows,
	},
	{
		Code: "b", Icon: "🤖", NameKey: "settings_del_sec_bot", ListIntroKey: "settings_del_list_bot",
		NoneKey: "settings_state_off_plain", ChoiceRows: []int{3, 3}, Rows: deletionBotRows,
	},
}

// settingsDelDelayKeys maps a delay in seconds to the locale key of its label, used by the choice
// buttons and by the {delay} of "After {delay}". 0 is the Delete now choice.
var settingsDelDelayKeys = map[int]string{
	0:     "settings_del_state_now",
	10:    "settings_del_delay_10s",
	60:    "settings_del_delay_1m",
	600:   "settings_del_delay_10m",
	3600:  "settings_del_delay_1h",
	21600: "settings_del_delay_6h",
	86400: "settings_del_delay_24h",
}

func init() {
	controls := map[string]settingsControl{}
	for _, section := range settingsDelSections {
		for _, row := range section.Rows {
			// A row control takes st only: a tg naming it is refused in step 1.
			controls[row.Code] = settingsControl{
				Right: settingsRightChangeInfo, Actions: []string{settingsActSet},
				Validate: settingsDelPresetValidator(row.Presets), Set: settingsDelSet(section, row),
			}
		}
	}
	registerSettingsPage(settingsPage{
		Key:       settingsPageDeletion,
		Order:     85,
		Icon:      "🧹",
		TitleKey:  "settings_page_del",
		MainState: settingsDelMainState,
		Open:      settingsDelOpen,
		Hidden:    settingsDelHidden,
		Controls:  controls,
		Actions: map[string]settingsPageAction{
			settingsActDelList: {Fields: []string{"t"}, Right: settingsRightAdmin, Validate: settingsDelListFields, Run: settingsDelRunList},
			settingsActDelRow:  {Fields: []string{"t"}, Right: settingsRightAdmin, Validate: settingsDelRowFields, Run: settingsDelRunRow},
		},
	})
}

// settingsDelPresetValidator is the strict check of a control's v: the "no rule" value, or the
// canonical decimal seconds of one of the row's own presets. A sign, a space, a non-ASCII digit, a
// leading zero and any value the row does not offer are refused (0 is the one zero).
func settingsDelPresetValidator(presets []int) func(string) bool {
	return func(value string) bool {
		if value == settingsDelNone {
			return true
		}
		secs, ok := settingsParseNumber(value, 5)
		return ok && strconv.Itoa(secs) == value && slices.Contains(presets, secs)
	}
}

// settingsDelSectionByCode finds a section of the hub.
func settingsDelSectionByCode(code string) (settingsDelSection, bool) {
	for _, section := range settingsDelSections {
		if section.Code == code {
			return section, true
		}
	}
	return settingsDelSection{}, false
}

// settingsDelRowByCode finds a row and the section that lists it.
func settingsDelRowByCode(code string) (settingsDelSection, deletionRow, bool) {
	for _, section := range settingsDelSections {
		for _, row := range section.Rows {
			if row.Code == code {
				return section, row, true
			}
		}
	}
	return settingsDelSection{}, deletionRow{}, false
}

// settingsDelAllRows is every row of every section, in hub order.
func settingsDelAllRows() []deletionRow {
	var rows []deletionRow
	for _, section := range settingsDelSections {
		rows = append(rows, section.Rows...)
	}
	return rows
}

// settingsDelListFields is the Validate of dl: t is a section code (the generic check refuses any
// other key).
func settingsDelListFields(fields map[string]string) bool {
	_, ok := settingsDelSectionByCode(fields["t"])
	return ok
}

// settingsDelRowFields is the Validate of dr: t is a row code.
func settingsDelRowFields(fields map[string]string) bool {
	_, _, ok := settingsDelRowByCode(fields["t"])
	return ok
}

// settingsDelData is what a screen of the page knows about the group's rules: the rules read
// through the cached loader and the shared /cleanservice flag.
type settingsDelData struct {
	Rules *deletion.Rules
	Flag  bool
}

// settingsDelRead reads the rules and, only when one of the rows shares the /cleanservice flag, the
// flag, through the seams of the watcher and the send sites. It creates nothing; an error is the
// caller's cannot-open edit.
func settingsDelRead(chatID int64, rows []deletionRow) (settingsDelData, error) {
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()
	rules, err := deletionRulesLookup(ctx, chatID)
	if err != nil {
		return settingsDelData{}, err
	}
	data := settingsDelData{Rules: rules}
	for _, row := range rows {
		if !deletion.SharesCleanServiceFlag(row.Kind) {
			continue
		}
		settings, err := deletionFlagLookup(ctx, chatID)
		if err != nil {
			return settingsDelData{}, err
		}
		data.Flag = settings != nil && settings.ShouldCleanService
		break
	}
	return data, nil
}

// settingsDelCountOn counts the rows whose effective rule is on.
func settingsDelCountOn(rows []deletionRow, data settingsDelData) int {
	on := 0
	for _, row := range rows {
		if _, isOn := deletionEffectiveDelay(row.Kind, data.Rules, data.Flag); isOn {
			on++
		}
	}
	return on
}

// settingsDelCountWord is the state of a group of rows: Off when none is on, else "{n} on".
func settingsDelCountWord(tr *i18n.Translator, rows []deletionRow, data settingsDelData) string {
	if on := settingsDelCountOn(rows, data); on > 0 {
		return settingsText(tr, "settings_state_del_on", i18n.TranslationParams{"n": on})
	}
	return settingsText(tr, "settings_state_off_plain")
}

// settingsDelMainState is the Deletion line of Main: database reads only, no Telegram call. The
// Staff Group row is read fresh (the page is not drawn there) and an unreadable one, or the rules,
// is the unknown mark. A stored rule counts as on whether or not the bot can delete.
func settingsDelMainState(_ *gotgbot.Bot, tr *i18n.Translator, group *gotgbot.Chat) (string, bool) {
	staffRow, err := deletionStaffLookup(group.Id)
	if err != nil || staffRow != nil {
		return "", false
	}
	rows := settingsDelAllRows()
	data, err := settingsDelRead(group.Id, rows)
	if err != nil {
		return "", false
	}
	return settingsDelCountWord(tr, rows, data), true
}

// settingsDelHidden is the page's Hidden: true only for a definite Staff Group row. An unreadable
// row hides nothing here; Main's state and Open fail closed instead.
func settingsDelHidden(group *gotgbot.Chat) bool {
	staffRow, err := deletionStaffLookup(group.Id)
	return err == nil && staffRow != nil
}

// settingsDelGate is what a fresh Staff Group read says about a chat.
type settingsDelGate int

const (
	// settingsDelFree means the chat is not a Staff Group.
	settingsDelFree settingsDelGate = iota
	// settingsDelStaff means the chat is a Staff Group.
	settingsDelStaff
	// settingsDelUnreadable means the Staff Group row could not be read.
	settingsDelUnreadable
)

// settingsDelStaffCheck reads the Staff Group row of a chat fresh.
func settingsDelStaffCheck(chatID int64) settingsDelGate {
	staffRow, err := deletionStaffLookup(chatID)
	switch {
	case err != nil:
		log.Warnf("[Settings] the Staff Group row of chat %d could not be read for the deletion page: %v", chatID, err)
		return settingsDelUnreadable
	case staffRow != nil:
		return settingsDelStaff
	}
	return settingsDelFree
}

// settingsDelStaffGate is the defence in depth of D-18, at the top of dl, dr and every control, after
// the press pipeline and before any read or write of the page. A Staff Group answers the generic
// invalid-request alert as the press's one answer, an unreadable row is the cannot-open edit, and
// both return blocked with nothing written. No text says the group is a Staff Group.
func settingsDelStaffGate(p *settingsPress) (blocked bool) {
	switch settingsDelStaffCheck(p.group.Id) {
	case settingsDelStaff:
		p.answer(settingsText(p.tr, "common_callback_invalid_request"), true)
		return true
	case settingsDelUnreadable:
		p.cannotOpen()
		return true
	case settingsDelFree:
	}
	return false
}

// settingsDelReadRight is the right line of a draw: nothing when the bot can delete, the 🚫 line when
// it definitely cannot and the ❔ line when that could not be established. It makes one live paced
// read of the bot's own record under settingsDelHereTimeout, trusting an administrator's flag
// outside basic groups only, and never an error text.
func settingsDelReadRight(p *settingsPress) string {
	switch deletionReadBotRight(context.Background(), p.b, p.group.Id, settingsDelHereTimeout, p.group.Type != "group") {
	case deletionRightCan:
		return ""
	case deletionRightCannot:
		return "🚫 " + settingsText(p.tr, "settings_del_no_right")
	case deletionRightUnknown:
	}
	return settingsUnknownState + " " + settingsText(p.tr, "settings_del_right_unknown")
}

// settingsDelPrepared is what a draw gathered before it renders: the view, the rules and the right
// line.
type settingsDelPrepared struct {
	View  settingsView
	Data  settingsDelData
	Right string
}

// settingsDelPrepare is the one draw order of the page: the database is read first (an error is the
// caller's cannot-open edit), the press is answered empty, then the one live right read is made, so
// the read never holds the spinner and a redraw after a write reads once too.
func settingsDelPrepare(p *settingsPress, rows []deletionRow) (settingsDelPrepared, error) {
	data, err := settingsDelRead(p.group.Id, rows)
	if err != nil {
		return settingsDelPrepared{}, err
	}
	p.answer("", false)
	return settingsDelPrepared{View: p.view(), Data: data, Right: settingsDelReadRight(p)}, nil
}

// settingsDelState is how a stored rule is drawn: the state icon, the state word and the value of
// the choice to mark (empty when none is stored, so nothing is marked only by the caller's options).
type settingsDelState struct {
	Icon, Word, Value string
}

// settingsDelDelayLabel is the label of a delay: a preset's own, else the custom "{n} s".
func settingsDelDelayLabel(tr *i18n.Translator, secs int) string {
	if key, ok := settingsDelDelayKeys[secs]; ok {
		return settingsText(tr, key)
	}
	return settingsText(tr, "settings_del_delay_custom", i18n.TranslationParams{"n": secs})
}

// settingsDelStateOf is the one function that maps a stored delay, or none, to its icon, word and
// selected value; the list line, the cell, the Currently line and the selected mark all call it.
// A stored 0 reads Delete now whatever the row offers, and a delay that is no preset reads After
// {n} s and marks nothing because no choice carries its value.
func settingsDelStateOf(tr *i18n.Translator, noneKey string, delay int, on bool) settingsDelState {
	switch {
	case !on:
		return settingsDelState{Icon: "➖", Word: settingsText(tr, noneKey), Value: settingsDelNone}
	case delay == 0:
		return settingsDelState{Icon: "🗑", Word: settingsText(tr, "settings_del_state_now"), Value: "0"}
	}
	return settingsDelState{
		Icon:  "⏳",
		Word:  settingsText(tr, "settings_del_state_after", i18n.TranslationParams{"delay": settingsDelDelayLabel(tr, delay)}),
		Value: strconv.Itoa(delay),
	}
}

// settingsDelRowState is the drawn state of a row in a section from the data a draw read.
func settingsDelRowState(tr *i18n.Translator, section settingsDelSection, row deletionRow, data settingsDelData) settingsDelState {
	delay, on := deletionEffectiveDelay(row.Kind, data.Rules, data.Flag)
	return settingsDelStateOf(tr, section.NoneKey, delay, on)
}

// settingsDelSlot adds the status slot: one blank line and the right line, or nothing.
func settingsDelSlot(body []string, right string) []string {
	if right == "" {
		return body
	}
	return append(body, "", right)
}

// settingsDelGrid fills buttons two per row, left to right; an odd last one stays alone.
func settingsDelGrid(k *settingsKeyboard, buttons []gotgbot.InlineKeyboardButton) {
	for len(buttons) > 0 {
		width := min(2, len(buttons))
		k.row(buttons[:width]...)
		buttons = buttons[width:]
	}
}

// settingsDelBackRow ends a screen with Back (to the given fields) and Close.
func settingsDelBackRow(k *settingsKeyboard, back map[string]string) {
	var nav []gotgbot.InlineKeyboardButton
	if button, ok := k.button(settingsText(k.view.Tr, "settings_btn_back"), back); ok {
		nav = append(nav, button)
	}
	if button, ok := k.button(settingsText(k.view.Tr, "settings_btn_close"), map[string]string{"a": settingsActClose}); ok {
		nav = append(nav, button)
	}
	k.row(nav...)
}

// renderSettingsDelHub draws the hub: the intro, one state line per section, the right line, the time
// hint, the section grid and Back to Main. It is pure.
func renderSettingsDelHub(prep settingsDelPrepared, sections []settingsDelSection, notice string) settingsScreen {
	tr := prep.View.Tr
	body := []string{settingsText(tr, "settings_del_intro"), ""}
	k := newSettingsKeyboard(prep.View)
	var cells []gotgbot.InlineKeyboardButton
	for _, section := range sections {
		name := settingsText(tr, section.NameKey)
		body = append(body, settingsText(tr, "settings_main_state_line", i18n.TranslationParams{
			"icon": section.Icon, "page": name, "state": settingsDelCountWord(tr, section.Rows, prep.Data),
		}))
		if button, ok := k.button(section.Icon+" "+name, map[string]string{"a": settingsActDelList, "t": section.Code}); ok {
			cells = append(cells, button)
		}
	}
	body = settingsDelSlot(body, prep.Right)
	body = append(body, "", settingsText(tr, "settings_del_hint_time"))
	settingsDelGrid(k, cells)
	k.navRow(false)
	return k.screen(settingsCompose(prep.View, "settings_page_del", body, notice, false))
}

// renderSettingsDelList draws one section's list: the intro, one state line per row, the right line,
// the section's hints and the time hint, the row grid and Back to the hub. It is pure.
func renderSettingsDelList(prep settingsDelPrepared, section settingsDelSection, notice string) settingsScreen {
	tr := prep.View.Tr
	body := []string{settingsText(tr, section.ListIntroKey), ""}
	k := newSettingsKeyboard(prep.View)
	var cells []gotgbot.InlineKeyboardButton
	for _, row := range section.Rows {
		state := settingsDelRowState(tr, section, row, prep.Data)
		name := settingsText(tr, row.NameKey)
		body = append(body, settingsText(tr, "settings_main_state_line", i18n.TranslationParams{
			"icon": state.Icon, "page": name, "state": state.Word,
		}))
		if button, ok := k.button(state.Icon+" "+name, map[string]string{"a": settingsActDelRow, "t": row.Code}); ok {
			cells = append(cells, button)
		}
	}
	body = settingsDelSlot(body, prep.Right)
	body = append(body, "")
	for _, key := range section.ListHintKeys {
		body = append(body, settingsText(tr, key))
	}
	body = append(body, settingsText(tr, "settings_del_hint_time"))
	settingsDelGrid(k, cells)
	settingsDelBackRow(k, map[string]string{"a": settingsActOpen, "p": settingsPageDeletion})
	return k.screen(settingsCompose(prep.View, "settings_page_del", body, notice, false))
}

// settingsDelOptions are the choices of a row: the "no rule" choice, then one per preset.
func settingsDelOptions(tr *i18n.Translator, section settingsDelSection, row deletionRow) []settingsOption {
	options := []settingsOption{{Value: settingsDelNone, Label: settingsText(tr, section.NoneKey)}}
	for _, preset := range row.Presets {
		options = append(options, settingsOption{Value: strconv.Itoa(preset), Label: settingsDelDelayLabel(tr, preset)})
	}
	return options
}

// renderSettingsDelRow draws one row's screen: its name and description, the row's own hint, the right
// line, the Currently line, the time hint, the choice rows laid out by the section and Back to the
// section's list. No card, no expiry field. It is pure.
func renderSettingsDelRow(prep settingsDelPrepared, section settingsDelSection, row deletionRow, notice string) settingsScreen {
	tr := prep.View.Tr
	state := settingsDelRowState(tr, section, row, prep.Data)
	body := []string{settingsText(tr, row.NameKey), settingsText(tr, row.DescKey)}
	if row.HintKey != "" {
		body = append(body, settingsText(tr, row.HintKey))
	}
	if prep.Right != "" {
		body = append(body, prep.Right)
	}
	body = append(body, "",
		settingsText(tr, "settings_del_currently", i18n.TranslationParams{"state": state.Icon + " " + state.Word}),
		"", settingsText(tr, "settings_del_hint_time"))

	k := newSettingsKeyboard(prep.View)
	options := settingsDelOptions(tr, section, row)
	for _, width := range section.ChoiceRows {
		if len(options) == 0 {
			break
		}
		width = min(width, len(options))
		k.row(settingsChoiceRow(k, settingsPageDeletion, row.Code, options[:width], state.Value)...)
		options = options[width:]
	}
	settingsDelBackRow(k, map[string]string{"a": settingsActDelList, "t": section.Code})
	return k.screen(settingsCompose(prep.View, "settings_page_del", body, notice, false))
}

// settingsDelOpen draws the hub. A chat that is a Staff Group, or whose row cannot be read, is the
// cannot-open edit (openPage has already refused a definite Staff Group; this fails closed on an
// unreadable one).
func settingsDelOpen(p *settingsPress, notice string) (settingsScreen, error) {
	if settingsDelStaffCheck(p.group.Id) != settingsDelFree {
		return settingsScreen{}, errSettingsDelBlocked
	}
	prep, err := settingsDelPrepare(p, settingsDelAllRows())
	if err != nil {
		return settingsScreen{}, err
	}
	return renderSettingsDelHub(prep, settingsDelSections, notice), nil
}

// settingsDelDrawList draws a section's list for the press, or the cannot-open edit and nil.
func settingsDelDrawList(p *settingsPress, section settingsDelSection, notice string) *settingsScreen {
	prep, err := settingsDelPrepare(p, section.Rows)
	if err != nil {
		log.Warnf("[Settings] the deletion list of chat %d could not be read: %v", p.group.Id, err)
		p.cannotOpen()
		return nil
	}
	screen := renderSettingsDelList(prep, section, notice)
	return &screen
}

// settingsDelDrawRow draws a row's screen for the press, or the cannot-open edit and nil.
func settingsDelDrawRow(p *settingsPress, section settingsDelSection, row deletionRow, notice string) *settingsScreen {
	prep, err := settingsDelPrepare(p, []deletionRow{row})
	if err != nil {
		log.Warnf("[Settings] the deletion screen of chat %d could not be read: %v", p.group.Id, err)
		p.cannotOpen()
		return nil
	}
	screen := renderSettingsDelRow(prep, section, row, notice)
	return &screen
}

// settingsDelRunList serves dl.
func settingsDelRunList(p *settingsPress) *settingsScreen {
	if settingsDelStaffGate(p) {
		return nil
	}
	section, ok := settingsDelSectionByCode(p.fields["t"])
	if !ok {
		p.answer(settingsText(p.tr, "common_callback_invalid_request"), true)
		return nil
	}
	return settingsDelDrawList(p, section, "")
}

// settingsDelRunRow serves dr.
func settingsDelRunRow(p *settingsPress) *settingsScreen {
	if settingsDelStaffGate(p) {
		return nil
	}
	section, row, ok := settingsDelRowByCode(p.fields["t"])
	if !ok {
		p.answer(settingsText(p.tr, "common_callback_invalid_request"), true)
		return nil
	}
	return settingsDelDrawRow(p, section, row, "")
}

// settingsDelApplyChoice stores the absolute value v of a row through the row's own apply function:
// joined and left through the shared /cleanservice path (Keep, Delete now, After N), every other row
// through the rules write (a removed row, or the delay). The control's Validate already judged v.
func settingsDelApplyChoice(chatID int64, row deletionRow, value string) deletionApplyResult {
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()
	remove := value == settingsDelNone
	secs, ok := settingsParseNumber(value, 5)
	if !remove && !ok {
		return deletionApplyResult{Outcome: deletionRefused, Err: errors.New("deletion choice is not a delay")}
	}
	if deletion.SharesCleanServiceFlag(row.Kind) {
		state := deletion.JoinLeftState{Mode: deletion.JoinLeftAfter, DelaySecs: secs}
		switch {
		case remove:
			state = deletion.JoinLeftState{Mode: deletion.JoinLeftKeep}
		case secs == 0:
			state = deletion.JoinLeftState{Mode: deletion.JoinLeftNow}
		}
		return applyCleanServiceRule(ctx, chatID, row.Kind, state)
	}
	target := deletionSet(secs)
	if remove {
		target = deletionRemove
	}
	return applyDeleteRule(ctx, chatID, row.Kind, target)
}

// settingsDelSet makes the Set of a row's control. The Staff Group gate runs first, then the write,
// then the row's screen is redrawn from the stored state: a press of the stored state writes nothing
// and a failed write redraws the old state with the save-failed notice.
func settingsDelSet(section settingsDelSection, row deletionRow) func(*settingsPress, string) *settingsScreen {
	return func(p *settingsPress, value string) *settingsScreen {
		if settingsDelStaffGate(p) {
			return nil
		}
		notice := ""
		switch result := settingsDelApplyChoice(p.group.Id, row, value); result.Outcome {
		case deletionRefused:
			log.Warnf("[Settings] a deletion choice of chat %d was refused: %v", p.group.Id, result.Err)
			p.answer(settingsText(p.tr, "common_callback_invalid_request"), true)
			return nil
		case deletionSaveFailed:
			notice = settingsNoticeWarn(settingsText(p.tr, "common_settings_save_failed"))
		case deletionApplied, deletionUnchanged:
		}
		return settingsDelDrawRow(p, section, row, notice)
	}
}
