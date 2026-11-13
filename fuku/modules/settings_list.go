package modules

import (
	"context"
	"html"
	"slices"
	"strconv"
	"strings"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// The list engine of the settings menu (Phase 19). The three list pages (Blacklist, Filters, Notes) are
// one engine and three tables: a settingsListDef names the page's codes, strings, rights and the seams
// that read and delete its entries, and settingsListPage builds the ordinary settingsPage from it. No
// renderer, validator or gate in this file asks which page it serves; the table drives everything, so
// adding a list is one table and one registration.
//
// A list page is private-only (D-01): it is registered with PrivateOnly, so a menu opened in the group
// shows only the framework's note and never runs any code here, and every list action requires the
// owner field c in step 1, so a press of a group menu is an invalid request before any I/O. An entry
// is a numbered body line and is never a button label (M44); its delete button carries the entry's
// row ID and its page, never any user text. Every draw reads the chat's list fresh, chat-scoped and
// with the error returned, sorts it and cuts the page in Go.

const (
	// settingsListPageSize is how many entries one page shows (D-05).
	settingsListPageSize = 8
	// settingsListLineMax is the longest entry, in code points, a list line shows before the ellipsis.
	settingsListLineMax = 32
	// settingsListCardMax is the longest entry a delete card or a notice shows before the ellipsis.
	settingsListCardMax = 100
	// settingsListIDMaxDigits is the longest row ID a delete button carries: a Yes with 17 digits is
	// exactly 64 bytes, so a longer ID would be a dead button (U-5).
	settingsListIDMaxDigits = 17
	// settingsListPageMaxDigits is the longest pager cursor v (U-4, F-6).
	settingsListPageMaxDigits = 4
	// settingsListEntryToken stands for the escaped entry text until after translation.
	settingsListEntryToken = "<<settings-entry>>"
	// settingsListDeleteIcon starts the label of a delete button, followed by the entry's number.
	settingsListDeleteIcon = "🗑 "
	// settingsListAdminTagSep separates an entry from its admin tag on its line.
	settingsListAdminTagSep = " · "
)

// settingsListEntry is one row of a list as the engine sees it. Text is the stored word, keyword or name;
// Action is the blacklist word's stored action; AdminOnly is a note's admin-only flag.
type settingsListEntry struct {
	ID        int64
	Text      string
	AdminOnly bool
	Action    string
}

// settingsListRemoveOutcome is what a table's Remove did; its zero value is removed.
type settingsListRemoveOutcome int

const (
	// settingsListRemoved means the chat's row was deleted.
	settingsListRemoved settingsListRemoveOutcome = iota
	// settingsListRemoveGone means no row of the chat has the ID (already removed, never existed or another chat's).
	settingsListRemoveGone
	// settingsListRemoveFailed means the delete failed.
	settingsListRemoveFailed
)

// settingsListRemoval is what a table's Remove returns: the outcome and the stored text of the row that went.
type settingsListRemoval struct {
	Outcome settingsListRemoveOutcome
	Text    string
}

// settingsListDef is the table of one list page.
type settingsListDef struct {
	// Key is the page's p value; Order, Icon and TitleKey place it on Main.
	Key      string
	Order    int
	Icon     string
	TitleKey string
	// PagerCode, AskCode and YesCode are the page's action codes: the pager (field v), the delete ask (r, v)
	// and the delete Yes (r, e). An empty AskCode or YesCode draws and registers no delete.
	PagerCode, AskCode, YesCode string
	// Right is the right of the delete ask and Yes: the right of the command the delete mirrors.
	Right settingsRight
	// Read lists the chat's entries, fresh, chat-scoped and with the error returned; it creates nothing.
	Read func(ctx context.Context, chatID int64) ([]settingsListEntry, error)
	// Entry reads one entry by ID, only when it belongs to the chat: a missing and a foreign ID are nil, nil.
	Entry func(ctx context.Context, chatID, id int64) (*settingsListEntry, error)
	// Count is the Main state's count, one fresh chat-scoped count.
	Count func(ctx context.Context, chatID int64) (int, error)
	// CountMax, when positive, is the list's cap: the count reads n/CountMax on Main and {max} in the count line.
	CountMax int
	// Remove deletes one entry by ID, only when it belongs to the chat, through the command's apply function.
	Remove func(ctx context.Context, chatID, id int64) settingsListRemoval
	// IntroKey, CountKey and EmptyKey are the list screen's body keys; DeleteAskKey and RemovedKey the card's and
	// the notice's, each taking the entry under EntryParam; AdminTagKey, when set, tags an admin-only entry's line.
	IntroKey, CountKey, EmptyKey, DeleteAskKey, RemovedKey, AdminTagKey string
	EntryParam                                                          string
	// BodyExtra adds lines after the count line of a non-empty list (the blacklist's action line).
	BodyExtra func(v settingsView, entries []settingsListEntry) []string
	// AddRowExtra returns the buttons of the row after the pager that follow the Add button (the Action button); none
	// draws no extra button.
	AddRowExtra func(k *settingsKeyboard, entries []settingsListEntry) []gotgbot.InlineKeyboardButton
	// AddCode, CancelCode and AddKey are the typed add's actions and the Add button's label key. An empty AddCode
	// draws and registers no Add. Prompt gives the lines of the add prompt before its footer for a part, and Typed is
	// the door's handler of a typed reply to the page's wait; both are set with AddCode.
	AddCode, CancelCode, AddKey string
	Prompt                      func(v settingsView, part string) []string
	Typed                       func(r *settingsInputReply, part settingsListPart)
	// Actions are the page's own action codes besides the engine's (the blacklist's action screen).
	Actions map[string]settingsPageAction
	// Redraw maps an action code of the page to the screen a notice refusal of that action redraws, which
	// is page 1 unless the action says otherwise.
	Redraw map[string]func(p *settingsPress, notice string) (settingsScreen, error)
}

var (
	// settingsLists records every registered list table by page key.
	settingsLists = map[string]settingsListDef{}
)

// settingsListByPage finds the table of a list page.
func settingsListByPage(key string) (settingsListDef, bool) {
	def, ok := settingsLists[key]
	return def, ok
}

// registerSettingsList registers the page of a table and records the table.
func registerSettingsList(def settingsListDef) {
	registerSettingsPage(settingsListPage(def))
	settingsLists[def.Key] = def
}

// settingsListSortKey is what an entry is ordered by: its lower-cased text.
func settingsListSortKey(e settingsListEntry) string {
	return strings.ToLower(e.Text)
}

// settingsListCompare orders entries by lower-cased text in code point order (a byte comparison of
// UTF-8, not locale collation), ties by row ID.
func settingsListCompare(a, b settingsListEntry) int {
	if c := strings.Compare(settingsListSortKey(a), settingsListSortKey(b)); c != 0 {
		return c
	}
	switch {
	case a.ID < b.ID:
		return -1
	case a.ID > b.ID:
		return 1
	}
	return 0
}

// settingsListSort returns the entries in the order a list draws them. The same entries give the
// same order on every redraw and on every replica.
func settingsListSort(entries []settingsListEntry) []settingsListEntry {
	sorted := slices.Clone(entries)
	slices.SortStableFunc(sorted, settingsListCompare)
	return sorted
}

// settingsListPages is the number of pages of n entries: at least one.
func settingsListPages(n int) int {
	return max((n+settingsListPageSize-1)/settingsListPageSize, 1)
}

// settingsListClampPage keeps a requested page inside 1..pages of n entries.
func settingsListClampPage(page, n int) int {
	return min(max(page, 1), settingsListPages(n))
}

// settingsListLabel is the text of an entry in a line, a card or a notice: bidirectional controls
// stripped, line breaks and runs of whitespace collapsed to one space, cut by code points at max with an
// ellipsis appended when cut, then HTML-escaped, so an entity is never split. The result is spliced in
// after translation and never reaches a button label, callback data or a log line.
func settingsListLabel(text string, maxRunes int) string {
	label := strings.Join(strings.Fields(memberReqStripBidi(text)), " ")
	runes := []rune(label)
	if len(runes) > maxRunes {
		label = string(runes[:maxRunes]) + settingsHubEllipsis
	}
	return html.EscapeString(label)
}

// settingsListParseID reads a row ID of 1 to settingsListIDMaxDigits ASCII digits with no leading zero
// and no sign, positive. Anything else is refused.
func settingsListParseID(raw string) (int64, bool) {
	if len(raw) > settingsListIDMaxDigits || strings.HasPrefix(raw, "0") {
		return 0, false
	}
	return settingsCardNumber(raw)
}

// settingsListPrivateField reports whether a press carries the private owner field c: every list action
// requires it, so the same press of a group menu (which carries u) is refused in step 1.
func settingsListPrivateField(fields map[string]string) bool {
	return fields["c"] != ""
}

// settingsListPageField reads the pager cursor v of 1 to settingsListPageMaxDigits digits.
func settingsListPageField(fields map[string]string) (int, bool) {
	return settingsParseNumber(fields["v"], settingsListPageMaxDigits)
}

// settingsListCountText is the entry count a list's Main state shows: the digits, or n/max for a capped list.
func settingsListCountText(def settingsListDef, n int) string {
	if def.CountMax > 0 {
		return strconv.Itoa(n) + "/" + strconv.Itoa(def.CountMax)
	}
	return strconv.Itoa(n)
}

// settingsListMainState is the page's line of the Main page in a private menu: the fresh count in digits,
// or the unknown mark when it could not be read. It makes no Telegram call.
func settingsListMainState(def settingsListDef) func(*gotgbot.Bot, *i18n.Translator, *gotgbot.Chat) (string, bool) {
	return func(_ *gotgbot.Bot, _ *i18n.Translator, group *gotgbot.Chat) (string, bool) {
		ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
		defer cancel()
		n, err := def.Count(ctx, group.Id)
		if err != nil {
			return "", false
		}
		return settingsListCountText(def, n), true
	}
}

// settingsListEntryText translates key with the entry spliced in after translation: the entry is cut at
// settingsListCardMax, escaped and put where the template's param stood, so user text never passes through the translator.
func settingsListEntryText(tr *i18n.Translator, key, param, text string) string {
	raw := settingsText(tr, key, i18n.TranslationParams{param: settingsListEntryToken})
	return lockdownSplice(raw, settingsListEntryToken, settingsListLabel(text, settingsListCardMax))
}

// renderSettingsList draws one page of a list for a private menu from the whole sorted list: the intro, the
// count line and the table's extra lines, one numbered line per entry of the page, the page line when there is
// more than one page, the notice, the delete rows, the pager, the table's add row and the navigation row. An
// empty list has the intro and the empty line only. A page outside the list is clamped. It is pure.
func renderSettingsList(v settingsView, def settingsListDef, entries []settingsListEntry, page int, notice string) settingsScreen {
	k := newSettingsKeyboard(v)
	body := []string{settingsText(v.Tr, def.IntroKey)}
	if len(entries) == 0 {
		body = append(body, "", settingsText(v.Tr, def.EmptyKey))
	} else {
		page = settingsListClampPage(page, len(entries))
		pages := settingsListPages(len(entries))
		body = append(body, settingsText(v.Tr, def.CountKey, i18n.TranslationParams{"n": len(entries), "max": def.CountMax}))
		if def.BodyExtra != nil {
			body = append(body, def.BodyExtra(v, entries)...)
		}
		body = append(body, "")
		first := (page - 1) * settingsListPageSize
		shown := entries[first:min(first+settingsListPageSize, len(entries))]
		for i, e := range shown {
			body = append(body, settingsListLine(v, def, first+i+1, e))
		}
		if pages > 1 {
			body = append(body, "", settingsText(v.Tr, "staff_panel_page", i18n.TranslationParams{"page": page, "pages": pages}))
		}
		settingsListDeleteRows(k, def, first, shown, page)
		settingsListPagerRow(k, def, page, pages)
	}
	k.row(settingsListAddRow(k, def, entries)...)
	k.navRow(false)
	return k.screen(settingsCompose(v, def.TitleKey, body, notice, false))
}

// settingsListLine is one entry's body line: its global number, its escaped text in code and, for an
// admin-only entry of a table that tags them, the tag.
func settingsListLine(v settingsView, def settingsListDef, number int, e settingsListEntry) string {
	line := strconv.Itoa(number) + ". <code>" + settingsListLabel(e.Text, settingsListLineMax) + "</code>"
	if e.AdminOnly && def.AdminTagKey != "" {
		line += settingsListAdminTagSep + settingsText(v.Tr, def.AdminTagKey)
	}
	return line
}

// settingsListDeleteRows adds the delete buttons of the page, four to a row, each labelled with the entry's
// global number and carrying its row ID and the page. A row whose ID has more digits than a Yes can carry
// gets no button and is counted in Omitted.
func settingsListDeleteRows(k *settingsKeyboard, def settingsListDef, first int, shown []settingsListEntry, page int) {
	if def.AskCode == "" {
		return
	}
	var row []gotgbot.InlineKeyboardButton
	for i, e := range shown {
		id := strconv.FormatInt(e.ID, 10)
		if len(id) > settingsListIDMaxDigits {
			log.Errorf("[Settings] delete button of a %s list skipped: the row ID has %d digits", def.Key, len(id))
			k.omitted++
			continue
		}
		label := settingsListDeleteIcon + strconv.Itoa(first+i+1)
		if button, ok := k.button(label, map[string]string{"a": def.AskCode, "r": id, "v": strconv.Itoa(page)}); ok {
			row = append(row, button)
		}
		if len(row) == 4 {
			k.row(row...)
			row = nil
		}
	}
	k.row(row...)
}

// settingsListPagerRow adds Prev only after page 1 and Next only before the last page, each carrying the
// absolute page, so there is no dead button.
func settingsListPagerRow(k *settingsKeyboard, def settingsListDef, page, pages int) {
	var buttons []gotgbot.InlineKeyboardButton
	if page > 1 {
		if prev, ok := k.button(settingsText(k.view.Tr, "staff_panel_prev"), map[string]string{"a": def.PagerCode, "v": strconv.Itoa(page - 1)}); ok {
			buttons = append(buttons, prev)
		}
	}
	if page < pages {
		if next, ok := k.button(settingsText(k.view.Tr, "staff_panel_next"), map[string]string{"a": def.PagerCode, "v": strconv.Itoa(page + 1)}); ok {
			buttons = append(buttons, next)
		}
	}
	k.row(buttons...)
}

// renderSettingsListCard draws the delete card of an entry: the question naming the entry (escaped, cut at
// settingsListCardMax), Yes carrying the entry's row ID and a fresh expiry, No returning to the page the ask
// came from, and the navigation row (Screen 4). It is pure.
func renderSettingsListCard(v settingsView, def settingsListDef, entry settingsListEntry, page int) settingsScreen {
	k := newSettingsKeyboard(v)
	settingsCardRowsTo(k,
		map[string]string{"a": def.YesCode, "r": strconv.FormatInt(entry.ID, 10)},
		map[string]string{"a": def.PagerCode, "v": strconv.Itoa(page)},
	)
	body := []string{settingsListEntryText(v.Tr, def.DeleteAskKey, def.EntryParam, entry.Text)}
	return k.screen(settingsCompose(v, def.TitleKey, body, "", false))
}

// settingsListLoad reads the chat's list, sorted. The error is the caller's cannot-open edit.
func settingsListLoad(p *settingsPress, def settingsListDef) ([]settingsListEntry, error) {
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()
	entries, err := def.Read(ctx, p.group.Id)
	if err != nil {
		return nil, err
	}
	return settingsListSort(entries), nil
}

// settingsListDraw reads the list and draws page with the notice.
func settingsListDraw(p *settingsPress, def settingsListDef, page int, notice string) (settingsScreen, error) {
	entries, err := settingsListLoad(p, def)
	if err != nil {
		return settingsScreen{}, err
	}
	return renderSettingsList(p.view(), def, entries, page, notice), nil
}

// settingsListDrawHolding draws the page that holds the entry id (page 1 when it is gone).
func settingsListDrawHolding(p *settingsPress, def settingsListDef, id int64, notice string) (settingsScreen, error) {
	entries, err := settingsListLoad(p, def)
	if err != nil {
		return settingsScreen{}, err
	}
	page := 1
	if i := slices.IndexFunc(entries, func(e settingsListEntry) bool { return e.ID == id }); i >= 0 {
		page = i/settingsListPageSize + 1
	}
	return renderSettingsList(p.view(), def, entries, page, notice), nil
}

// settingsListDrawBefore draws the page where an entry with the given text and ID stood: the number of
// entries that sort before it, divided by the page size, plus one.
func settingsListDrawBefore(p *settingsPress, def settingsListDef, text string, id int64, notice string) (settingsScreen, error) {
	entries, err := settingsListLoad(p, def)
	if err != nil {
		return settingsScreen{}, err
	}
	gone := settingsListEntry{ID: id, Text: text}
	before := 0
	for _, e := range entries {
		if settingsListCompare(e, gone) < 0 {
			before++
		}
	}
	return renderSettingsList(p.view(), def, entries, before/settingsListPageSize+1, notice), nil
}

// settingsListShow turns a draw into the screen of a page action: a read error is the cannot-open edit and
// draws nothing more.
func settingsListShow(p *settingsPress, def settingsListDef, screen settingsScreen, err error) *settingsScreen {
	if err != nil {
		log.Warnf("[Settings] list page %q of chat %d could not be read: %v", def.Key, p.group.Id, err)
		p.cannotOpen()
		return nil
	}
	return &screen
}

// settingsListOpen is the page's Open. A notice refusal of a page action redraws through Open too, so the
// screen is chosen from the press: the ask's page for the ask, the page holding the entry for the Yes, the
// table's own screen for an action it names, page 1 for everything else.
func settingsListOpen(def settingsListDef) func(*settingsPress, string) (settingsScreen, error) {
	return func(p *settingsPress, notice string) (settingsScreen, error) {
		if redraw, ok := def.Redraw[p.action]; ok {
			return redraw(p, notice)
		}
		switch {
		case def.AskCode != "" && p.action == def.AskCode:
			page, _ := settingsListPageField(p.fields)
			return settingsListDraw(p, def, page, notice)
		case def.YesCode != "" && p.action == def.YesCode:
			id, _ := settingsListParseID(p.fields["r"])
			return settingsListDrawHolding(p, def, id, notice)
		}
		return settingsListDraw(p, def, 1, notice)
	}
}

// settingsListRunPager handles the pager: page v, clamped to the list.
func settingsListRunPager(def settingsListDef) func(*settingsPress) *settingsScreen {
	return func(p *settingsPress) *settingsScreen {
		page, _ := settingsListPageField(p.fields)
		screen, err := settingsListDraw(p, def, page, "")
		return settingsListShow(p, def, screen, err)
	}
}

// settingsListRunAsk handles the delete ask: the entry is read fresh by (id, chat); a gone or foreign
// entry redraws page v with the already-removed notice and no card, else the card names it.
func settingsListRunAsk(def settingsListDef) func(*settingsPress) *settingsScreen {
	return func(p *settingsPress) *settingsScreen {
		id, _ := settingsListParseID(p.fields["r"])
		page, _ := settingsListPageField(p.fields)
		ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
		defer cancel()
		entry, err := def.Entry(ctx, p.group.Id, id)
		if err != nil {
			return settingsListShow(p, def, settingsScreen{}, err)
		}
		if entry == nil {
			screen, err := settingsListDraw(p, def, page, settingsNoticeWarn(settingsText(p.tr, "settings_entries_already_removed")))
			return settingsListShow(p, def, screen, err)
		}
		screen := renderSettingsListCard(p.view(), def, *entry, page)
		return &screen
	}
}

// settingsListRunYes handles the delete Yes: an expired minute deletes nothing, else the table's Remove
// deletes exactly the row the card carries, and the list is redrawn at the page where it stood.
func settingsListRunYes(def settingsListDef) func(*settingsPress) *settingsScreen {
	return func(p *settingsPress) *settingsScreen {
		id, _ := settingsListParseID(p.fields["r"])
		expired, ok := settingsCardExpired(p.fields)
		if !ok {
			p.answer(settingsText(p.tr, "common_callback_invalid_request"), true)
			return nil
		}
		if expired {
			screen, err := settingsListDrawHolding(p, def, id, settingsNoticeWarn(settingsText(p.tr, "settings_card_expired")))
			return settingsListShow(p, def, screen, err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
		defer cancel()
		removal := def.Remove(ctx, p.group.Id, id)
		var (
			screen settingsScreen
			err    error
		)
		switch removal.Outcome {
		case settingsListRemoved:
			notice := settingsNoticeOK(settingsListEntryText(p.tr, def.RemovedKey, def.EntryParam, removal.Text))
			screen, err = settingsListDrawBefore(p, def, removal.Text, id, notice)
		case settingsListRemoveGone:
			screen, err = settingsListDraw(p, def, 1, settingsNoticeWarn(settingsText(p.tr, "settings_entries_already_removed")))
		case settingsListRemoveFailed:
			fallthrough
		default:
			screen, err = settingsListDrawHolding(p, def, id, settingsNoticeWarn(settingsText(p.tr, "common_settings_save_failed")))
		}
		p.answer("", false)
		return settingsListShow(p, def, screen, err)
	}
}

// settingsListActions builds the page's action table: the pager, the delete ask and Yes of the table and the
// table's own actions. Every action requires the owner field c, so a group menu never reaches it, and checks its own
// fields strictly in step 1.
func settingsListActions(def settingsListDef) map[string]settingsPageAction {
	actions := map[string]settingsPageAction{
		def.PagerCode: {
			Fields: []string{"v"},
			Right:  settingsRightAdmin,
			Validate: func(fields map[string]string) bool {
				_, ok := settingsListPageField(fields)
				return settingsListPrivateField(fields) && ok
			},
			Run: settingsListRunPager(def),
		},
	}
	if def.AskCode != "" {
		actions[def.AskCode] = settingsPageAction{
			Fields: []string{"r", "v"},
			Right:  def.Right,
			Validate: func(fields map[string]string) bool {
				_, idOK := settingsListParseID(fields["r"])
				_, pageOK := settingsListPageField(fields)
				return settingsListPrivateField(fields) && idOK && pageOK
			},
			Run: settingsListRunAsk(def),
		}
	}
	if def.YesCode != "" {
		actions[def.YesCode] = settingsPageAction{
			Fields: []string{"r", "e"},
			Right:  def.Right,
			Validate: func(fields map[string]string) bool {
				_, idOK := settingsListParseID(fields["r"])
				_, minuteOK := settingsCardNumber(fields["e"])
				return settingsListPrivateField(fields) && idOK && minuteOK
			},
			Run: settingsListRunYes(def),
		}
	}
	if def.AddCode != "" {
		for code, action := range settingsListAddActions(def) {
			actions[code] = action
		}
	}
	for code, action := range def.Actions {
		actions[code] = action
	}
	return actions
}

// settingsListPage builds the settings page of a table. It is pure: it registers nothing, so a test can
// install a page of its own table by swapping registry entries.
func settingsListPage(def settingsListDef) settingsPage {
	return settingsPage{
		Key:         def.Key,
		Order:       def.Order,
		Icon:        def.Icon,
		TitleKey:    def.TitleKey,
		PrivateOnly: true,
		MainState:   settingsListMainState(def),
		Open:        settingsListOpen(def),
		Actions:     settingsListActions(def),
	}
}
