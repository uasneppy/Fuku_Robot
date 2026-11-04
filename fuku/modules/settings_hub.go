package modules

import (
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db/adminindex"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/error_handling"
)

// The private group list is the top level of the settings hub: /settings in the bot's
// private chat sends it, a tap on a row is a pick (settings|v1 a=pk) that turns the same
// message into that group's Main page. The list is built only from the presser's own user ID
// and the bot's roster record (chat_admin_index), with a database read and no Telegram call.
// The record is a hint: every pick is checked against a live getChatMember.

const (
	// settingsHubNamespace is the callback namespace of the list's own buttons
	// (shub|v1|...): paging and Close.
	settingsHubNamespace = "shub"
	// settingsHubActPage shows list page v (a whole number of 1 to settingsHubPageDigits digits).
	settingsHubActPage = "lp"
	// settingsHubActClose deletes the list message.
	settingsHubActClose = "cx"
	// settingsHubPageSize is the number of groups on one page (D-04).
	settingsHubPageSize = 8
	// settingsHubLabelMax is the longest title, in code points, a row label shows before the
	// ellipsis is appended.
	settingsHubLabelMax = 32
	// settingsHubPageDigits is the longest page value: 999 pages, so 7,992 groups.
	settingsHubPageDigits = 3
	// settingsHubMaxPages is the last page a list can reach.
	settingsHubMaxPages = 999
	// settingsHubEllipsis is appended to a label that was cut.
	settingsHubEllipsis = "…"
)

// settingsHubLookup reads the groups the roster record says a user may manage, fresh from
// the database. It is a variable only so a test can make the read fail; production code never
// reassigns it.
var settingsHubLookup = adminindex.ListForUserFresh

// settingsHubCollapse folds every run of whitespace, line breaks included, to one space and
// trims the ends.
func settingsHubCollapse(title string) string {
	return strings.Join(strings.Fields(title), " ")
}

// settingsHubLabel is the text of a list row: the group's title alone, collapsed and cut at
// settingsHubLabelMax code points with an ellipsis appended when cut (so never more than 33
// code points), or the chat ID when the title is empty. A button label is plain text that
// Telegram never parses, so the title is not HTML-escaped (D-01 as clarified).
func settingsHubLabel(title string, chatID int64) string {
	label := settingsHubCollapse(title)
	if label == "" {
		return strconv.FormatInt(chatID, 10)
	}
	runes := []rune(label)
	if len(runes) > settingsHubLabelMax {
		return string(runes[:settingsHubLabelMax]) + settingsHubEllipsis
	}
	return label
}

// settingsHubSortKey is what a row is ordered by: the lower-cased collapsed title, or the
// chat ID text for a group with no title, which is the text its row shows.
func settingsHubSortKey(group adminindex.ListedGroup) string {
	title := settingsHubCollapse(group.Title)
	if title == "" {
		return strconv.FormatInt(group.ChatID, 10)
	}
	return strings.ToLower(title)
}

// settingsHubSort returns the groups ordered by lower-cased title in code point order (a byte
// comparison of UTF-8, not locale collation), ties by chat ID ascending (D-04). The order is
// the same on every redraw.
func settingsHubSort(groups []adminindex.ListedGroup) []adminindex.ListedGroup {
	sorted := slices.Clone(groups)
	slices.SortStableFunc(sorted, func(a, b adminindex.ListedGroup) int {
		if c := strings.Compare(settingsHubSortKey(a), settingsHubSortKey(b)); c != 0 {
			return c
		}
		switch {
		case a.ChatID < b.ChatID:
			return -1
		case a.ChatID > b.ChatID:
			return 1
		}
		return 0
	})
	return sorted
}

// settingsHubPages is the number of pages of n groups: at least one, at most
// settingsHubMaxPages.
func settingsHubPages(n int) int {
	pages := (n + settingsHubPageSize - 1) / settingsHubPageSize
	return min(max(pages, 1), settingsHubMaxPages)
}

// settingsHubButton builds one callback button of the given namespace. An encode that does not
// fit 64 bytes is counted in k.omitted and logged, never shipped as a dead button.
func settingsHubButton(k *settingsKeyboard, label, namespace string, fields map[string]string) (gotgbot.InlineKeyboardButton, bool) {
	data := encodeCallbackData(namespace, fields)
	if data == "" {
		log.Errorf("[Settings] list button skipped: callback data does not fit (namespace %q, action %q)", namespace, fields["a"])
		k.omitted++
		return gotgbot.InlineKeyboardButton{}, false
	}
	return gotgbot.InlineKeyboardButton{Text: label, CallbackData: data}, true
}

// renderSettingsHubList draws list page page (clamped to the pages that exist) of groups, which
// must already be sorted. No title appears in the message text, only on the row buttons, so
// nothing here needs escaping. Zero groups is the one neutral empty text, with Close only.
func renderSettingsHubList(tr *i18n.Translator, groups []adminindex.ListedGroup, page int) settingsScreen {
	header := settingsText(tr, "settings_header", i18n.TranslationParams{
		"page": settingsText(tr, "settings_page_groups"),
	})
	hint := settingsText(tr, "settings_list_hint")

	k := newSettingsKeyboard(settingsView{Tr: tr})
	closeButton := func() {
		if button, ok := settingsHubButton(k, settingsText(tr, "settings_btn_close"), settingsHubNamespace,
			map[string]string{"a": settingsHubActClose}); ok {
			k.row(button)
		}
	}

	if len(groups) == 0 {
		closeButton()
		return k.screen(header + "\n\n" + settingsText(tr, "settings_list_empty") + "\n\n" + hint)
	}

	pages := settingsHubPages(len(groups))
	page = min(max(page, 1), pages)
	text := header + "\n\n" + settingsText(tr, "settings_list_intro") + "\n\n" + hint
	if pages > 1 {
		text += "\n\n" + settingsText(tr, "staff_panel_page", i18n.TranslationParams{"page": page, "pages": pages})
	}

	start := (page - 1) * settingsHubPageSize
	end := min(start+settingsHubPageSize, len(groups))
	for _, group := range groups[start:end] {
		fields := map[string]string{"a": settingsActPick, "c": strconv.FormatInt(group.ChatID, 10)}
		if button, ok := settingsHubButton(k, settingsHubLabel(group.Title, group.ChatID), settingsCallbackNamespace, fields); ok {
			k.row(button)
		}
	}

	var paging []gotgbot.InlineKeyboardButton
	if page > 1 {
		if button, ok := settingsHubButton(k, settingsText(tr, "staff_panel_prev"), settingsHubNamespace,
			map[string]string{"a": settingsHubActPage, "v": strconv.Itoa(page - 1)}); ok {
			paging = append(paging, button)
		}
	}
	if page < pages {
		if button, ok := settingsHubButton(k, settingsText(tr, "staff_panel_next"), settingsHubNamespace,
			map[string]string{"a": settingsHubActPage, "v": strconv.Itoa(page + 1)}); ok {
			paging = append(paging, button)
		}
	}
	k.row(paging...)
	closeButton()
	return k.screen(text)
}

// settingsHubScreen reads the user's groups, sorts them and renders page of the list. The
// read error is returned for the caller to answer; nothing is drawn for it.
func settingsHubScreen(ctx context.Context, tr *i18n.Translator, userID int64, page int) (settingsScreen, error) {
	readCtx, cancel := context.WithTimeout(ctx, settingsReadTimeout)
	defer cancel()
	groups, err := settingsHubLookup(readCtx, userID)
	if err != nil {
		return settingsScreen{}, err
	}
	screen := renderSettingsHubList(tr, settingsHubSort(groups), page)
	if screen.Omitted > 0 {
		log.Errorf("[Settings] the group list left out %d button(s) whose callback data did not fit", screen.Omitted)
	}
	return screen, nil
}

// settingsHubCallback answers the shub| buttons of the private list. The list is always
// rebuilt from the presser's own user ID and never from the callback data, so a replayed or
// forwarded button shows the presser their own groups. It answers the query exactly once and
// returns ext.EndGroups.
func (m moduleStruct) settingsHubCallback(b *gotgbot.Bot, ctx *ext.Context) error {
	defer error_handling.RecoverFromPanic("settingsHubCallback", "Settings")

	query, ok := callbackQueryFromContext(ctx)
	if !ok {
		return ext.EndGroups
	}
	press := newSettingsPress(b, ctx, query)
	// Deferred after the recover above, so it runs first: a panic below still answers once.
	defer press.answer("", false)
	refuse := func(key string) error {
		press.answer(settingsText(press.tr, key), true)
		return ext.EndGroups
	}

	decoded, ok := decodeCallbackData(query.Data, settingsHubNamespace)
	if !ok {
		return refuse("common_callback_invalid_request")
	}
	if query.Message == nil || query.Message.GetDate() == 0 {
		return refuse("common_callback_message_unavailable")
	}
	if query.Message.GetChat().Type != "private" {
		return refuse("common_callback_invalid_request")
	}
	if staffServiceUserIDs[query.From.Id] {
		return refuse("lockdown_alert_post_as_yourself")
	}

	switch decoded.Fields["a"] {
	case settingsHubActClose:
		if len(decoded.Fields) != 1 {
			return refuse("common_callback_invalid_request")
		}
		press.closeMenu()
	default:
		return refuse("common_callback_invalid_request")
	}
	return ext.EndGroups
}
