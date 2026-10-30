package modules

import (
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/i18n"
)

// This file is the page registry and the shared renderer of the settings menu. Everything
// here is pure: it builds text and keyboards from a view and never reads a database or
// calls Telegram, except settingsMainEntries, which asks each page for its state.

const (
	// settingsBoundLabel is the label of a step button whose target equals the current
	// value, which only happens at a bound. It keeps valid callback data.
	settingsBoundLabel = "·"
	// settingsSelectedPrefix marks the selected option of a choice row.
	settingsSelectedPrefix = "✅ "
	// settingsUnknownState is appended to a Main page button whose state could not be read.
	settingsUnknownState = "❔"
	// settingsStateSeparator sits between a page name and its state on a Main page button.
	settingsStateSeparator = " · "
	// settingsNameToken stands for the opener's escaped name until after translation.
	settingsNameToken = "<<settings-name>>"
	// settingsGroupToken stands for the escaped group title until after translation.
	settingsGroupToken = "<<settings-group>>"
	// settingsNoticeWarnIcon and settingsNoticeOKIcon start the notice line of a refusal
	// and of a success.
	settingsNoticeWarnIcon = "⚠️ "
	settingsNoticeOKIcon   = "✅ "
)

// settingsView is everything a renderer needs to know about the menu it draws.
type settingsView struct {
	Tr *i18n.Translator
	// Private is true for a menu opened in a private chat.
	Private bool
	// OwnerKey is "c" (private menu) or "u" (group menu); OwnerValue is its decimal value.
	// Every button carries the pair.
	OwnerKey, OwnerValue string
	GroupID              int64
	GroupTitle           string
	// OpenerName is the opener's full name, escaped by the renderer.
	OpenerName string
	Now        time.Time
}

// settingsScreen is one drawn message: its HTML text and keyboard. Omitted counts buttons
// left out because their callback data did not fit; a worst-case test fails on any.
type settingsScreen struct {
	Text     string
	Keyboard gotgbot.InlineKeyboardMarkup
	Omitted  int
}

// settingsText translates key and logs a missing key; a typo'd key is an empty message.
func settingsText(tr *i18n.Translator, key string, params ...i18n.TranslationParams) string {
	if tr == nil {
		log.Errorf("[Settings] translate %s: no translator", key)
		return ""
	}
	text, err := tr.GetString(key, params...)
	if err != nil {
		log.Errorf("[Settings] translate %s: %v", key, err)
	}
	return text
}

// settingsNoticeWarn is the notice line of a refusal or a failure. Empty text gives no
// notice.
func settingsNoticeWarn(text string) string {
	if text == "" {
		return ""
	}
	return settingsNoticeWarnIcon + text
}

// settingsNoticeOK is the notice line of a success that needs a word (link, unlink, lift).
func settingsNoticeOK(text string) string {
	if text == "" {
		return ""
	}
	return settingsNoticeOKIcon + text
}

// settingsStateWord is "✅ On" or "❌ Off" in the view's language.
func settingsStateWord(tr *i18n.Translator, on bool) string {
	if on {
		return "✅ " + settingsText(tr, "common_status_on")
	}
	return "❌ " + settingsText(tr, "common_status_off")
}

// settingsParseNumber reads a non-negative whole number of 1 to maxDigits ASCII digits.
// A sign, a space, a letter or a non-ASCII digit is refused.
func settingsParseNumber(v string, maxDigits int) (int, bool) {
	if v == "" || len(v) > maxDigits {
		return 0, false
	}
	for i := 0; i < len(v); i++ {
		if v[i] < '0' || v[i] > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, false
	}
	return n, true
}

// settingsCompose writes a page: the header (the Main header when titleKey is empty, else
// the header naming the page whose title is titleKey), the group line, one blank line, the
// body lines, and when there is a notice or an updated line one blank line and the footer
// (the notice first). The group title is capped at 64 runes and escaped here, after
// translation, so a title never passes through the translator.
func settingsCompose(v settingsView, titleKey string, body []string, notice string, updated bool) string {
	var sb strings.Builder
	if titleKey == "" {
		sb.WriteString(settingsText(v.Tr, "settings_header_main"))
	} else {
		page := settingsText(v.Tr, titleKey)
		sb.WriteString(settingsText(v.Tr, "settings_header", i18n.TranslationParams{"page": page}))
	}
	sb.WriteString("\n<b>")
	sb.WriteString(staffDisplayTitle(v.GroupTitle))
	sb.WriteString("</b> <code>")
	sb.WriteString(strconv.FormatInt(v.GroupID, 10))
	sb.WriteString("</code>")

	if len(body) > 0 {
		sb.WriteString("\n\n")
		sb.WriteString(strings.Join(body, "\n"))
	}

	var footer []string
	if notice != "" {
		footer = append(footer, notice)
	}
	if updated {
		now := v.Now
		if now.IsZero() {
			now = time.Now()
		}
		footer = append(footer, settingsText(v.Tr, "staff_panel_updated", i18n.TranslationParams{
			"time": now.UTC().Format("15:04:05"),
		}))
	}
	if len(footer) > 0 {
		sb.WriteString("\n\n")
		sb.WriteString(strings.Join(footer, "\n"))
	}
	return sb.String()
}

// settingsKeyboard collects the rows of one screen. Every button it builds carries the
// view's owner field, and a button whose data does not fit 64 bytes is counted and logged,
// never dropped silently.
type settingsKeyboard struct {
	view    settingsView
	rows    [][]gotgbot.InlineKeyboardButton
	omitted int
}

// newSettingsKeyboard starts an empty keyboard for view.
func newSettingsKeyboard(view settingsView) *settingsKeyboard {
	return &settingsKeyboard{view: view}
}

// button builds a callback button: fields plus the owner field, encoded in the settings
// namespace. It returns false when the data does not fit.
func (k *settingsKeyboard) button(label string, fields map[string]string) (gotgbot.InlineKeyboardButton, bool) {
	all := make(map[string]string, len(fields)+1)
	maps.Copy(all, fields)
	if k.view.OwnerKey != "" {
		all[k.view.OwnerKey] = k.view.OwnerValue
	}
	data := encodeCallbackData(settingsCallbackNamespace, all)
	if data == "" {
		log.Errorf("[Settings] button skipped: callback data does not fit (action %q)", fields["a"])
		k.omitted++
		return gotgbot.InlineKeyboardButton{}, false
	}
	return gotgbot.InlineKeyboardButton{Text: label, CallbackData: data}, true
}

// row appends one keyboard row; an empty row is not added.
func (k *settingsKeyboard) row(buttons ...gotgbot.InlineKeyboardButton) {
	if len(buttons) == 0 {
		return
	}
	k.rows = append(k.rows, buttons)
}

// navRow appends the navigation row, which is always the last row: Close alone on the Main
// page, Back and Close everywhere else.
func (k *settingsKeyboard) navRow(main bool) {
	var buttons []gotgbot.InlineKeyboardButton
	if !main {
		if back, ok := k.button(settingsText(k.view.Tr, "settings_btn_back"), map[string]string{"a": settingsActBack}); ok {
			buttons = append(buttons, back)
		}
	}
	if closeButton, ok := k.button(settingsText(k.view.Tr, "settings_btn_close"), map[string]string{"a": settingsActClose}); ok {
		buttons = append(buttons, closeButton)
	}
	k.row(buttons...)
}

// screen finishes the keyboard into a screen with the given text.
func (k *settingsKeyboard) screen(text string) settingsScreen {
	return settingsScreen{
		Text:     text,
		Keyboard: gotgbot.InlineKeyboardMarkup{InlineKeyboard: k.rows},
		Omitted:  k.omitted,
	}
}

// settingsStepLabel is "+N" or "−N" (a true minus sign) for a step button.
func settingsStepLabel(step int) string {
	if step < 0 {
		return "−" + strconv.Itoa(-step)
	}
	return "+" + strconv.Itoa(step)
}

// settingsStepperRow builds a stepper row: the negative steps, the value button, then the
// positive steps. Each step button sends the absolute target value clamp(current+step), so
// a double tap is harmless; a target equal to the current value (only possible at a bound)
// is labelled "·" and keeps valid data. The value button redraws the page.
func settingsStepperRow(
	k *settingsKeyboard, page, control string, current int, steps []int, clamp func(int) int, valueLabel string,
) []gotgbot.InlineKeyboardButton {
	var row []gotgbot.InlineKeyboardButton
	placed := false
	place := func() {
		placed = true
		if button, ok := k.button(valueLabel, map[string]string{"a": settingsActOpen, "p": page}); ok {
			row = append(row, button)
		}
	}
	for _, step := range steps {
		if step > 0 && !placed {
			place()
		}
		target := clamp(current + step)
		label := settingsStepLabel(step)
		if target == current {
			label = settingsBoundLabel
		}
		fields := map[string]string{
			"a": settingsActSet, "p": page, "t": control, "v": strconv.Itoa(target),
		}
		if button, ok := k.button(label, fields); ok {
			row = append(row, button)
		}
	}
	if !placed {
		place()
	}
	return row
}

// settingsOption is one choice of a choice row: the value sent in v and the label shown.
type settingsOption struct {
	Value, Label string
}

// settingsChoiceRow builds a row with one button per option. The selected option's label is
// prefixed "✅ "; each button sends its own value as the absolute target.
func settingsChoiceRow(
	k *settingsKeyboard, page, control string, options []settingsOption, selected string,
) []gotgbot.InlineKeyboardButton {
	row := make([]gotgbot.InlineKeyboardButton, 0, len(options))
	for _, option := range options {
		label := option.Label
		if option.Value == selected {
			label = settingsSelectedPrefix + label
		}
		fields := map[string]string{
			"a": settingsActSet, "p": page, "t": control, "v": option.Value,
		}
		if button, ok := k.button(label, fields); ok {
			row = append(row, button)
		}
	}
	return row
}

// settingsToggleButton builds a one-button row (empty when the data does not fit) whose
// label is the caller's, showing the current state. It sends the absolute target, "0" when
// the switch is on and "1" when it is off, so a double tap is harmless.
func settingsToggleButton(k *settingsKeyboard, page, control, label string, on bool) []gotgbot.InlineKeyboardButton {
	target := "1"
	if on {
		target = "0"
	}
	fields := map[string]string{"a": settingsActToggle, "p": page, "t": control, "v": target}
	if button, ok := k.button(label, fields); ok {
		return []gotgbot.InlineKeyboardButton{button}
	}
	return nil
}

// settingsPage is one entry of the page registry. A page file registers itself from its
// own init with registerSettingsPage; adding a page never edits this file.
type settingsPage struct {
	// Key is the p value in callback data.
	Key string
	// Order sorts the Main page buttons (Staff Group 10, Lockdown 20, Captcha 30,
	// Antiflood 40).
	Order int
	Icon  string
	// TitleKey is the locale key of the page name (a literal at the registration).
	TitleKey string
	// MainState is the state word shown on the page's Main button. It makes database reads
	// only, no Telegram call, and takes no press so the private entry can draw the same
	// Main page. ok is false when the state could not be read.
	MainState func(b *gotgbot.Bot, tr *i18n.Translator, group *gotgbot.Chat) (state string, ok bool)
	// Open draws the page. An error is the cannot-open edit.
	Open func(p *settingsPress, notice string) (settingsScreen, error)
	// Controls are the tg and st targets, by t.
	Controls map[string]settingsControl
	// Actions are the page-specific action codes, by a.
	Actions map[string]settingsPageAction
}

// settingsControl is one toggle or setting of a page.
type settingsControl struct {
	Right settingsRight
	// Set applies the absolute value v and returns the screen to draw. A nil screen means
	// the handler already answered (a bound toast) and nothing is edited.
	Set func(p *settingsPress, value string) *settingsScreen
}

// settingsPageAction is a page-specific action. Fields are the extra keys the callback may
// carry besides a and the owner; they are allowed, not required, and Run parses them
// strictly.
type settingsPageAction struct {
	Fields []string
	Right  settingsRight
	Run    func(p *settingsPress) *settingsScreen
}

// settingsActionEntry maps a page action code back to its page.
type settingsActionEntry struct {
	pageKey string
	action  settingsPageAction
}

var (
	settingsPages   = map[string]settingsPage{}
	settingsActions = map[string]settingsActionEntry{}
)

// settingsCoreActions are the action codes the dispatcher itself owns.
var settingsCoreActions = []string{
	settingsActOpenHere, settingsActOpen, settingsActBack, settingsActClose,
	settingsActToggle, settingsActSet, settingsActCancel,
}

// registerSettingsPage adds a page to the registry. It panics at init on an empty or
// duplicate page key, a duplicate page action code, or a page action code that equals a
// core code: a clash would route a press to the wrong handler.
func registerSettingsPage(pg settingsPage) {
	if pg.Key == "" {
		panic("settings: page registered without a key")
	}
	if _, dup := settingsPages[pg.Key]; dup {
		panic("settings: duplicate page key " + pg.Key)
	}
	for code, action := range pg.Actions {
		if slices.Contains(settingsCoreActions, code) {
			panic("settings: page " + pg.Key + " action code " + code + " is a core code")
		}
		if owner, dup := settingsActions[code]; dup {
			panic("settings: page action code " + code + " of page " + pg.Key + " is already used by page " + owner.pageKey)
		}
		settingsActions[code] = settingsActionEntry{pageKey: pg.Key, action: action}
	}
	settingsPages[pg.Key] = pg
}

// settingsPagesInOrder returns the registered pages sorted by Order, then Key.
func settingsPagesInOrder() []settingsPage {
	pages := make([]settingsPage, 0, len(settingsPages))
	for _, pg := range settingsPages {
		pages = append(pages, pg)
	}
	slices.SortFunc(pages, func(a, b settingsPage) int {
		if a.Order != b.Order {
			return a.Order - b.Order
		}
		return strings.Compare(a.Key, b.Key)
	})
	return pages
}

// settingsPageByKey finds a registered page.
func settingsPageByKey(key string) (settingsPage, bool) {
	pg, ok := settingsPages[key]
	return pg, ok
}

// settingsMainEntry is one button of the Main page.
type settingsMainEntry struct {
	Key, Icon, Title, State string
	// Known is false when the state could not be read.
	Known bool
}

// settingsMainEntries asks every registered page for its Main state, in page order. It
// makes the database reads of the pages; the renderer stays pure.
func settingsMainEntries(b *gotgbot.Bot, tr *i18n.Translator, group *gotgbot.Chat) []settingsMainEntry {
	pages := settingsPagesInOrder()
	entries := make([]settingsMainEntry, 0, len(pages))
	for _, pg := range pages {
		entry := settingsMainEntry{Key: pg.Key, Icon: pg.Icon, Title: settingsText(tr, pg.TitleKey)}
		if pg.MainState != nil {
			entry.State, entry.Known = pg.MainState(b, tr, group)
		}
		entries = append(entries, entry)
	}
	return entries
}

// renderSettingsMain draws the Main page: the header, the group line, the hint, the
// opener's name in a group menu, one button per page and Close.
func renderSettingsMain(v settingsView, entries []settingsMainEntry, notice string) settingsScreen {
	body := []string{settingsText(v.Tr, "settings_main_hint")}
	if !v.Private {
		owner := settingsText(v.Tr, "settings_main_owner", i18n.TranslationParams{"name": settingsNameToken})
		body = append(body, lockdownSplice(owner, settingsNameToken, staffDisplayTitle(v.OpenerName)))
	}

	k := newSettingsKeyboard(v)
	for _, entry := range entries {
		label := entry.Icon + " " + entry.Title + settingsStateSeparator
		if entry.Known {
			label += entry.State
		} else {
			label += settingsUnknownState
		}
		if button, ok := k.button(label, map[string]string{"a": settingsActOpen, "p": entry.Key}); ok {
			k.row(button)
		}
	}
	k.navRow(true)
	return k.screen(settingsCompose(v, "", body, notice, false))
}
