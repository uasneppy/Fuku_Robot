package modules

import (
	"context"
	"fmt"
	"html"
	"strconv"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db/locks"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// The Locks page of the settings menu (page k, Phase 18) turns each of the 22 lock types of /lock on
// or off over two pages. Every cell writes exactly one lock through applyLock, the function /lock and
// /unlock use, and redraws the page the lock is on. The page reads the locks the way the watchers do,
// with the error returned, and a lock with no row reads Unlocked, as the enforcement does. Every draw
// carries the Phase 16 right line from one live read of the bot's own record, because a lock only
// works while the bot can delete the messages it targets; no button is ever disabled for it.

const (
	// settingsPageLocks is the p value of the Locks page.
	settingsPageLocks = "k"
	// settingsActLocksPage is the action code of the pager (field v, the absolute page).
	settingsActLocksPage = "kg"
	// settingsLocksPages is how many pages the table spreads over.
	settingsLocksPages = 2
	// settingsLocksPageDigits is the longest v of the pager.
	settingsLocksPageDigits = 2
)

// settingsLocksRead reads the chat's lock map through the loader the watchers' repository uses, with
// the error returned. A lock with no row is absent from the map. It is a variable only so a test can
// make the read fail.
var settingsLocksRead = locks.GetChatLocksCachedContext

func init() {
	controls := make(map[string]settingsControl, len(settingsLockRows))
	for _, row := range settingsLockRows {
		// /lock and /unlock ask IsUserConnected(true, true) first: an administrator and the bot as an
		// administrator. A press never writes without both.
		controls[row.Code] = settingsControl{
			Right:    settingsRightAdminFloodBot,
			Set:      settingsLocksControl(row),
			Actions:  []string{settingsActToggle},
			Validate: settingsEditSwitchValue,
		}
	}
	registerSettingsPage(settingsPage{
		Key:       settingsPageLocks,
		Order:     44,
		Icon:      "🔐",
		TitleKey:  "settings_page_locks",
		MainState: settingsLocksMainState,
		Open:      settingsLocksOpen,
		Controls:  controls,
		Actions: map[string]settingsPageAction{
			settingsActLocksPage: {
				Fields: []string{"v"}, Right: settingsRightAdmin,
				Validate: settingsLocksPageFields, Run: settingsLocksRunPage,
			},
		},
	})
}

// settingsLocksPageFields is the Validate of kg: v is one or two ASCII digits, and the generic check
// refuses any other key. A number outside the table's pages is clamped by Run.
func settingsLocksPageFields(fields map[string]string) bool {
	_, ok := settingsParseNumber(fields["v"], settingsLocksPageDigits)
	return ok
}

// settingsLockIcon is the lock of a locked kind and the open lock of an unlocked one.
func settingsLockIcon(on bool) string {
	if on {
		return "🔒"
	}
	return "🔓"
}

// settingsLocksRowsOf lists the rows of one page of the table, in table order.
func settingsLocksRowsOf(page int) []settingsLockRow {
	var rows []settingsLockRow
	for _, row := range settingsLockRows {
		if row.Page == page {
			rows = append(rows, row)
		}
	}
	return rows
}

// settingsLocksCount is how many of the table's kinds are stored locked.
func settingsLocksCount(state map[string]bool) int {
	n := 0
	for _, row := range settingsLockRows {
		if state[row.Kind] {
			n++
		}
	}
	return n
}

// settingsLocksMainState is the Locks line of the Main page: how many locks are on. A failed read
// answers unknown. It makes no Telegram call.
func settingsLocksMainState(_ *gotgbot.Bot, tr *i18n.Translator, group *gotgbot.Chat) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()
	state, err := settingsLocksRead(ctx, group.Id)
	if err != nil {
		return "", false
	}
	return settingsText(tr, "settings_state_locks_on", i18n.TranslationParams{"n": settingsLocksCount(state)}), true
}

// settingsLocksBuild reads the locks (an error is the caller's cannot-open edit), answers the press
// empty, then makes the one live read of the bot's right and draws the page, so the read never holds
// the spinner and a redraw after a write reads once too.
func settingsLocksBuild(p *settingsPress, page int, notice string) (settingsScreen, error) {
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()
	state, err := settingsLocksRead(ctx, p.group.Id)
	if err != nil {
		return settingsScreen{}, err
	}
	p.answer("", false)
	return renderSettingsLocks(p.view(), state, page, settingsDelReadRight(p), notice), nil
}

// settingsLocksOpen draws page 1 for the dispatcher. A read error is returned, which the dispatcher
// turns into the cannot-open edit.
func settingsLocksOpen(p *settingsPress, notice string) (settingsScreen, error) {
	return settingsLocksBuild(p, 1, notice)
}

// settingsLocksRunPage handles kg: the page v, clamped to the table's pages.
func settingsLocksRunPage(p *settingsPress) *settingsScreen {
	page, _ := settingsParseNumber(p.fields["v"], settingsLocksPageDigits)
	page = min(max(page, 1), settingsLocksPages)
	screen, err := settingsLocksBuild(p, page, "")
	if err != nil {
		log.Warnf("[Settings] Locks page of chat %d could not be read: %v", p.group.Id, err)
		p.cannotOpen()
		return nil
	}
	return &screen
}

// settingsLocksPager is the pager row: Next only before the last page and Prev only after the first,
// each sending the absolute page, so there is no dead button.
func settingsLocksPager(k *settingsKeyboard, page int) {
	var buttons []gotgbot.InlineKeyboardButton
	if page > 1 {
		if prev, ok := k.button(settingsText(k.view.Tr, "staff_panel_prev"), map[string]string{"a": settingsActLocksPage, "v": strconv.Itoa(page - 1)}); ok {
			buttons = append(buttons, prev)
		}
	}
	if page < settingsLocksPages {
		if next, ok := k.button(settingsText(k.view.Tr, "staff_panel_next"), map[string]string{"a": settingsActLocksPage, "v": strconv.Itoa(page + 1)}); ok {
			buttons = append(buttons, next)
		}
	}
	k.row(buttons...)
}

// renderSettingsLocks draws one page of the Locks page: the intro, one line per lock of the page with
// its state in words, the page line, the status slot with the right line (empty when the bot can
// delete), the notice, the toggle grid, the pager and the navigation row. It is pure.
func renderSettingsLocks(v settingsView, state map[string]bool, page int, right, notice string) settingsScreen {
	rows := settingsLocksRowsOf(page)
	body := []string{settingsText(v.Tr, "settings_locks_intro"), ""}
	cells := make([]settingsToggleCell, 0, len(rows))
	for _, row := range rows {
		on := state[row.Kind]
		name := settingsText(v.Tr, row.NameKey)
		word := settingsText(v.Tr, "settings_locks_state_off")
		if on {
			word = settingsText(v.Tr, "settings_locks_state_on")
		}
		body = append(body, settingsText(v.Tr, "settings_main_state_line", i18n.TranslationParams{
			"icon": settingsLockIcon(on), "page": name, "state": word,
		}))
		cells = append(cells, settingsToggleCell{Control: row.Code, Icon: settingsLockIcon(on), Name: name, On: on})
	}
	body = append(body, "", settingsText(v.Tr, "staff_panel_page", i18n.TranslationParams{"page": page, "pages": settingsLocksPages}))
	body = settingsDelSlot(body, right)

	k := newSettingsKeyboard(v)
	settingsToggleGrid(k, settingsPageLocks, cells)
	settingsLocksPager(k, page)
	k.navRow(false)
	return k.screen(settingsCompose(v, "settings_page_locks", body, notice, false))
}

// settingsLocksFailureNotice is the command's own failure sentence for the lock named, escaped
// because the locale text is the command's printf template and the page is sent as HTML.
func settingsLocksFailureNotice(p *settingsPress, key string, row settingsLockRow) string {
	name := html.EscapeString(settingsText(p.tr, row.NameKey))
	return settingsNoticeWarn(fmt.Sprintf(settingsText(p.tr, key), name))
}

// settingsLocksApply is the apply step of one lock's cell: it writes exactly that lock through
// applyLock. A failed save reports the command's failure key for the unlock or the lock, which the
// caller words with the lock's name.
func settingsLocksApply(ctx context.Context, chatID int64, row settingsLockRow, on bool) settingsToggleResult[map[string]bool] {
	result := applyLock(ctx, chatID, row.Kind, on)
	switch {
	case result.Outcome != lockApplied:
		if on {
			return settingsToggleResult[map[string]bool]{Key: "locks_lock_failed"}
		}
		return settingsToggleResult[map[string]bool]{Key: "locks_unlock_failed"}
	case result.Locks == nil:
		// The write went through and the read-back failed: the helper reads the page again.
		return settingsToggleResult[map[string]bool]{}
	}
	return settingsToggleResult[map[string]bool]{State: result.Locks, Stored: true}
}

// settingsLocksControl builds the Set of one lock's cell: the shared boolean press with the page's
// read, the lock's apply step and a draw of the page the lock is on, with the right line read once
// after the press is answered.
func settingsLocksControl(row settingsLockRow) func(*settingsPress, string) *settingsScreen {
	return func(p *settingsPress, value string) *settingsScreen {
		var failedKey string
		return settingsToggleSet(p, value,
			func(ctx context.Context) (map[string]bool, error) { return settingsLocksRead(ctx, p.group.Id) },
			func(ctx context.Context, _ map[string]bool, on bool) settingsToggleResult[map[string]bool] {
				result := settingsLocksApply(ctx, p.group.Id, row, on)
				failedKey = result.Key
				return result
			},
			func(state map[string]bool, notice string) settingsScreen {
				if failedKey != "" {
					notice = settingsLocksFailureNotice(p, failedKey, row)
				}
				p.answer("", false)
				return renderSettingsLocks(p.view(), state, row.Page, settingsDelReadRight(p), notice)
			})
	}
}

// settingsLockPageSize is how many locks one page of the Locks page shows (A18-A11): the smallest
// size that keeps the tallest page inside the private menu's row budget.
const settingsLockPageSize = 12

// settingsLockRow is one lock the Locks page shows.
type settingsLockRow struct {
	// Code is the control key a button carries, two lower-case letters. It is never the lock's name,
	// and it differs from the pager's action code and from every core action code. A later lock is
	// one row here and one entry in lockMap or restrMap.
	Code string
	// Kind is the lock's name in /locktypes and the key applyLock stores, one entry of lockMap or
	// restrMap.
	Kind string
	// NameKey is the locale key of the lock's name, which labels its button and its state line.
	NameKey string
	// Page is the page of the Locks page the row is drawn on: 1 or 2.
	Page int
}

// settingsLockRows lists the 22 locks in the order the page draws them: the 16 locks of lockMap
// and the six restrictions of restrMap, 12 on page 1 and 10 on page 2.
var settingsLockRows = []settingsLockRow{
	{Code: "sk", Kind: "sticker", NameKey: "settings_lock_sticker", Page: 1},
	{Code: "gf", Kind: "gif", NameKey: "settings_lock_gif", Page: 1},
	{Code: "ph", Kind: "photo", NameKey: "settings_lock_photo", Page: 1},
	{Code: "vd", Kind: "video", NameKey: "settings_lock_video", Page: 1},
	{Code: "vn", Kind: "videonote", NameKey: "settings_lock_videonote", Page: 1},
	{Code: "vo", Kind: "voice", NameKey: "settings_lock_voice", Page: 1},
	{Code: "au", Kind: "audio", NameKey: "settings_lock_audio", Page: 1},
	{Code: "dc", Kind: "document", NameKey: "settings_lock_document", Page: 1},
	{Code: "ct", Kind: "contact", NameKey: "settings_lock_contact", Page: 1},
	{Code: "lc", Kind: "location", NameKey: "settings_lock_location", Page: 1},
	{Code: "gm", Kind: "game", NameKey: "settings_lock_game", Page: 1},
	{Code: "ur", Kind: "url", NameKey: "settings_lock_url", Page: 1},
	{Code: "fw", Kind: "forward", NameKey: "settings_lock_forward", Page: 2},
	{Code: "rt", Kind: "rtl", NameKey: "settings_lock_rtl", Page: 2},
	{Code: "ac", Kind: "anonchannel", NameKey: "settings_lock_anonchannel", Page: 2},
	{Code: "bt", Kind: "bots", NameKey: "settings_lock_bots", Page: 2},
	{Code: "ms", Kind: "messages", NameKey: "settings_lock_messages", Page: 2},
	{Code: "cm", Kind: "comments", NameKey: "settings_lock_comments", Page: 2},
	{Code: "md", Kind: "media", NameKey: "settings_lock_media", Page: 2},
	{Code: "ot", Kind: "other", NameKey: "settings_lock_other", Page: 2},
	{Code: "pv", Kind: "previews", NameKey: "settings_lock_previews", Page: 2},
	{Code: "al", Kind: "all", NameKey: "settings_lock_all", Page: 2},
}
