package modules

import (
	"context"
	"html"
	"slices"
	"strings"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db/blacklists"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// The Blacklist page of the settings menu (page b, Phase 19) is a table of the list engine
// (settings_list.go) plus the action screen. It lists the chat's blacklisted words eight a page, deletes one
// behind a two-tap card that binds the row, and sets the action every word carries through
// applyBlacklistAction, the function /blacklistaction uses. It reads through the fresh, chat-scoped,
// non-creating readers of the blacklists repository, never through the command's cached getter, which hides
// its read error. It is private-only: a menu opened in the group shows only the framework's note.

const (
	// settingsPageBlacklist is the p value of the Blacklist page.
	settingsPageBlacklist = "b"
	// settingsActBlacklistOpen opens the action screen (no field besides the owner), settingsActBlacklistSet sets the
	// action of every word (the field v, one of blacklistActionValues).
	settingsActBlacklistOpen = "bo"
	settingsActBlacklistSet  = "bs"
	// settingsBlacklistActionMaxRunes caps a stored action value that has no option word when it is shown.
	settingsBlacklistActionMaxRunes = 16
	// settingsBlacklistApplyTimeout bounds the writes of one typed message: a message holds at most about two thousand
	// words, each one insert.
	settingsBlacklistApplyTimeout = 30 * time.Second
)

// The reads of the page, variables only so a test can count or fail them. Each adapts the repository's
// row to the engine's entry.
var (
	// settingsBlacklistRead lists the chat's words with their actions.
	settingsBlacklistRead = func(ctx context.Context, chatID int64) ([]settingsListEntry, error) {
		rows, err := blacklists.ListBlacklistEntriesFresh(ctx, chatID)
		if err != nil {
			return nil, err
		}
		entries := make([]settingsListEntry, 0, len(rows))
		for _, row := range rows {
			entries = append(entries, settingsBlacklistEntryOf(row))
		}
		return entries, nil
	}
	// settingsBlacklistEntry reads one word by ID, only when it belongs to the chat.
	settingsBlacklistEntry = func(ctx context.Context, chatID, id int64) (*settingsListEntry, error) {
		row, err := blacklists.GetBlacklistEntryFresh(ctx, chatID, id)
		if err != nil || row == nil {
			return nil, err
		}
		entry := settingsBlacklistEntryOf(*row)
		return &entry, nil
	}
	// settingsBlacklistCount counts the chat's words.
	settingsBlacklistCount = func(ctx context.Context, chatID int64) (int, error) {
		n, err := blacklists.CountBlacklistFresh(ctx, chatID)
		return int(n), err
	}
)

// settingsBlacklistEntryOf is the engine's entry of a stored word.
func settingsBlacklistEntryOf(row models.BlacklistSettings) settingsListEntry {
	return settingsListEntry{ID: int64(row.ID), Text: row.Word, Action: row.Action}
}

// settingsBlacklistActionWord is the word of a stored action: the four option words of the commands, the
// Delete only word for none, and any other stored value (tban, tmute, delete) as stored, capped and escaped.
func settingsBlacklistActionWord(tr *i18n.Translator, action string) string {
	switch action {
	case "warn":
		return settingsText(tr, "settings_opt_warn")
	case "mute":
		return settingsText(tr, "settings_opt_mute")
	case "kick":
		return settingsText(tr, "settings_opt_kick")
	case "ban":
		return settingsText(tr, "settings_opt_ban")
	case "none":
		return settingsText(tr, "settings_bl_opt_none")
	}
	runes := []rune(action)
	if len(runes) > settingsBlacklistActionMaxRunes {
		action = string(runes[:settingsBlacklistActionMaxRunes])
	}
	return html.EscapeString(action)
}

// settingsBlacklistUniformAction returns the one action every word has, and false when the words differ or
// there are none: the page claims a single action only when it is true of every word (C-02).
func settingsBlacklistUniformAction(entries []settingsListEntry) (string, bool) {
	if len(entries) == 0 {
		return "", false
	}
	for _, e := range entries[1:] {
		if e.Action != entries[0].Action {
			return "", false
		}
	}
	return entries[0].Action, true
}

// settingsBlacklistActionLine is the action line of the list page and of the action screen: the action when
// every word has it, Mixed when they differ, no line for an empty list.
func settingsBlacklistActionLine(v settingsView, entries []settingsListEntry) []string {
	if len(entries) == 0 {
		return nil
	}
	word := settingsText(v.Tr, "settings_bl_action_mixed")
	if action, ok := settingsBlacklistUniformAction(entries); ok {
		word = settingsBlacklistActionWord(v.Tr, action)
	}
	return []string{settingsText(v.Tr, "settings_bl_action_line", i18n.TranslationParams{"action": word})}
}

// settingsBlacklistActionCounts counts the words by stored action.
func settingsBlacklistActionCounts(entries []settingsListEntry) map[string]int {
	counts := make(map[string]int)
	for _, e := range entries {
		counts[e.Action]++
	}
	return counts
}

// settingsBlacklistActionsSorted lists the stored actions of the words, most words first, ties by value.
func settingsBlacklistActionsSorted(counts map[string]int) []string {
	actions := make([]string, 0, len(counts))
	for action := range counts {
		actions = append(actions, action)
	}
	slices.SortFunc(actions, func(a, b string) int {
		if counts[a] != counts[b] {
			return counts[b] - counts[a]
		}
		switch {
		case a < b:
			return -1
		case a > b:
			return 1
		}
		return 0
	})
	return actions
}

// settingsBlacklistActionBody is the action screen's body: the intro, the action line, one count line per stored
// value (most words first) and the mixed hint when the words differ, the inert line for a uniform stored value the
// command never writes, a blank line and the hint. An empty list has the intro and the no-words line only (U-10).
func settingsBlacklistActionBody(v settingsView, entries []settingsListEntry) []string {
	body := []string{settingsText(v.Tr, "settings_bl_action_intro")}
	if len(entries) == 0 {
		return append(body, "", settingsText(v.Tr, "settings_bl_action_no_words"))
	}
	body = append(body, settingsBlacklistActionLine(v, entries)...)
	counts := settingsBlacklistActionCounts(entries)
	if len(counts) > 1 {
		for _, action := range settingsBlacklistActionsSorted(counts) {
			body = append(body, settingsText(v.Tr, "settings_bl_action_count", i18n.TranslationParams{
				"action": settingsBlacklistActionWord(v.Tr, action), "n": counts[action],
			}))
		}
		body = append(body, settingsText(v.Tr, "settings_bl_action_mixed_hint"))
	} else if action, _ := settingsBlacklistUniformAction(entries); !slices.Contains(blacklistActionValues, action) {
		body = append(body, settingsText(v.Tr, "settings_bl_action_inert"))
	}
	return append(body, "", settingsText(v.Tr, "settings_bl_action_hint"))
}

// settingsBlacklistChoiceRow builds one row of the action choices: each button sets its own value for every word, the
// one every word already has is marked, and the labels are the option words.
func settingsBlacklistChoiceRow(k *settingsKeyboard, selected string, values ...string) []gotgbot.InlineKeyboardButton {
	row := make([]gotgbot.InlineKeyboardButton, 0, len(values))
	for _, value := range values {
		label := settingsBlacklistActionWord(k.view.Tr, value)
		if value == selected {
			label = settingsSelectedPrefix + label
		}
		if button, ok := k.button(label, map[string]string{"a": settingsActBlacklistSet, "v": value}); ok {
			row = append(row, button)
		}
	}
	return row
}

// renderSettingsBlacklistAction draws the action screen (Screen 3): the body, the two choice rows with a check
// mark only on the action every word has, and Back to the list (page 1) and Close. An empty list has no choices. It
// is pure.
func renderSettingsBlacklistAction(v settingsView, entries []settingsListEntry, notice string) settingsScreen {
	k := newSettingsKeyboard(v)
	if len(entries) > 0 {
		selected, _ := settingsBlacklistUniformAction(entries)
		k.row(settingsBlacklistChoiceRow(k, selected, "warn", "mute", "kick")...)
		k.row(settingsBlacklistChoiceRow(k, selected, "ban", "none")...)
	}
	var nav []gotgbot.InlineKeyboardButton
	if back, ok := k.button(settingsText(v.Tr, "settings_btn_back"), map[string]string{"a": settingsActOpen, "p": settingsPageBlacklist}); ok {
		nav = append(nav, back)
	}
	if closeButton, ok := k.button(settingsText(v.Tr, "settings_btn_close"), map[string]string{"a": settingsActClose}); ok {
		nav = append(nav, closeButton)
	}
	k.row(nav...)
	return k.screen(settingsCompose(v, "settings_page_blacklist", settingsBlacklistActionBody(v, entries), notice, false))
}

// settingsBlacklistDrawAction reads the list fresh and draws the action screen with the notice.
func settingsBlacklistDrawAction(p *settingsPress, notice string) (settingsScreen, error) {
	def, _ := settingsListByPage(settingsPageBlacklist)
	entries, err := settingsListLoad(p, def)
	if err != nil {
		return settingsScreen{}, err
	}
	return renderSettingsBlacklistAction(p.view(), entries, notice), nil
}

// settingsBlacklistRunOpen handles bo: a fresh read and the action screen.
func settingsBlacklistRunOpen(p *settingsPress) *settingsScreen {
	screen, err := settingsBlacklistDrawAction(p, "")
	if err != nil {
		log.Warnf("[Settings] the blacklist action screen of chat %d could not be read: %v", p.group.Id, err)
		p.cannotOpen()
		return nil
	}
	return &screen
}

// settingsBlacklistRunSet handles bs: the action of every word through applyBlacklistAction, the function
// /blacklistaction uses, then the action screen from a fresh read. A press of the action every word has writes the
// same rows again and shows no notice; an empty list answers the no-words notice; a failed save redraws the stored
// state with the save-failed notice.
func settingsBlacklistRunSet(p *settingsPress) *settingsScreen {
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()
	notice := ""
	switch applyBlacklistAction(ctx, p.group.Id, p.fields["v"]).Outcome {
	case blacklistActionNoWords:
		notice = settingsNoticeWarn(settingsText(p.tr, "settings_bl_action_no_words"))
	case blacklistActionSaveFailed:
		notice = settingsNoticeWarn(settingsText(p.tr, "common_settings_save_failed"))
	case blacklistActionUnknown:
		// Unreachable: step 1 only lets the command's own values through.
		p.answer(settingsText(p.tr, "common_callback_invalid_request"), true)
		return nil
	}
	p.answer("", false)
	screen, err := settingsBlacklistDrawAction(p, notice)
	if err != nil {
		log.Warnf("[Settings] the blacklist action screen of chat %d could not be read: %v", p.group.Id, err)
		p.cannotOpen()
		return nil
	}
	return &screen
}

// settingsBlacklistAddRow is the part of the row after the pager that follows the Add button: the Action button on a
// list with words, nothing on an empty one (U-10).
func settingsBlacklistAddRow(k *settingsKeyboard, entries []settingsListEntry) []gotgbot.InlineKeyboardButton {
	if len(entries) == 0 {
		return nil
	}
	button, ok := k.button(settingsText(k.view.Tr, "settings_btn_bl_action"), map[string]string{"a": settingsActBlacklistOpen})
	if !ok {
		return nil
	}
	return []gotgbot.InlineKeyboardButton{button}
}

// settingsBlacklistActions are the page's own actions: the action screen and the action set.
func settingsBlacklistActions() map[string]settingsPageAction {
	return map[string]settingsPageAction{
		settingsActBlacklistOpen: {
			Right:    settingsRightAdmin,
			Validate: settingsListPrivateField,
			Run:      settingsBlacklistRunOpen,
		},
		settingsActBlacklistSet: {
			Fields: []string{"v"},
			Right:  settingsRightRestrictBotRestrict,
			Validate: func(fields map[string]string) bool {
				return settingsListPrivateField(fields) && slices.Contains(blacklistActionValues, fields["v"])
			},
			Run: settingsBlacklistRunSet,
		},
	}
}

// settingsBlacklistRemove deletes one word by its row ID through applyBlacklistRemoveByID, the function the
// delete of the command's rows shares with the page: one conditional DELETE by (id, chat) and the cache drop.
func settingsBlacklistRemove(ctx context.Context, chatID, id int64) settingsListRemoval {
	result := applyBlacklistRemoveByID(ctx, chatID, id)
	switch result.Outcome {
	case blacklistRemovedByID:
		return settingsListRemoval{Outcome: settingsListRemoved, Text: result.Word}
	case blacklistRemoveGone:
		return settingsListRemoval{Outcome: settingsListRemoveGone}
	}
	return settingsListRemoval{Outcome: settingsListRemoveFailed}
}

// settingsBlacklistPrompt is the lines of the add prompt before its footer (Screen 5): what the owner is asked to send
// and the rules a word must meet.
func settingsBlacklistPrompt(v settingsView, _ string) []string {
	return []string{
		settingsText(v.Tr, "settings_bl_wait"),
		settingsText(v.Tr, "settings_bl_wait_rules", i18n.TranslationParams{"max": listKeywordMaxRunes}),
	}
}

// settingsBlacklistCounts is how many of the typed words fell into each category: the page words only counts, never a
// word (U-7).
type settingsBlacklistCounts struct {
	Added, Already, TooLong, TooBroad, Failed int
}

// settingsBlacklistCountsOf is the counts of an apply result.
func settingsBlacklistCountsOf(result blacklistAddResult) settingsBlacklistCounts {
	return settingsBlacklistCounts{
		Added:    len(result.Added),
		Already:  len(result.Already),
		TooLong:  len(result.TooLong),
		TooBroad: len(result.TooBroad),
		Failed:   result.Failed,
	}
}

// settingsBlacklistCountLines is one line per non-zero category, in the order Added, Already, Too long, Too broad,
// Failed.
func settingsBlacklistCountLines(tr *i18n.Translator, c settingsBlacklistCounts) []string {
	var lines []string
	for _, line := range []struct {
		key    string
		n      int
		params i18n.TranslationParams
	}{
		{"settings_bl_saved_added", c.Added, i18n.TranslationParams{"n": c.Added}},
		{"settings_bl_saved_already", c.Already, i18n.TranslationParams{"n": c.Already}},
		{"settings_bl_saved_too_long", c.TooLong, i18n.TranslationParams{"n": c.TooLong, "max": listKeywordMaxRunes}},
		{"settings_bl_saved_too_broad", c.TooBroad, i18n.TranslationParams{"n": c.TooBroad}},
		{"settings_bl_saved_failed", c.Failed, i18n.TranslationParams{"n": c.Failed}},
	} {
		if line.n > 0 {
			lines = append(lines, settingsText(tr, line.key, line.params))
		}
	}
	return lines
}

// settingsBlacklistSavedNotice is the notice of the saved page: the success mark on the first count line, the other
// non-zero categories on their own lines and, for a message cut from an album, the album note.
func settingsBlacklistSavedNotice(tr *i18n.Translator, c settingsBlacklistCounts, album bool) string {
	lines := settingsBlacklistCountLines(tr, c)
	if len(lines) == 0 {
		return ""
	}
	notice := settingsNoticeOK(lines[0])
	for _, line := range lines[1:] {
		notice += "\n" + line
	}
	if album {
		notice += "\n" + settingsText(tr, "settings_edit_album_cut")
	}
	return notice
}

// settingsBlacklistRefusalText is the reply to a message with no addable word: the first line, then one line per
// non-zero category.
func settingsBlacklistRefusalText(tr *i18n.Translator, c settingsBlacklistCounts) string {
	text := settingsText(tr, "settings_bl_refuse_none")
	for _, line := range settingsBlacklistCountLines(tr, c) {
		text += "\n" + line
	}
	return text
}

// settingsBlacklistTypedWords splits the typed text like /addblacklist's arguments (strings.Fields), keeping each word as
// typed and counting a repeat once (words that differ only in case are one word, as they are stored lower-cased).
func settingsBlacklistTypedWords(text string) []string {
	seen := make(map[string]struct{})
	var words []string
	for _, word := range strings.Fields(text) {
		key := strings.ToLower(word)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		words = append(words, word)
	}
	return words
}

// settingsBlacklistRows is the engine's fresh read as the rows applyBlacklistAdd takes.
func settingsBlacklistRows(entries []settingsListEntry) models.BlacklistSettingsSlice {
	rows := make(models.BlacklistSettingsSlice, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, &models.BlacklistSettings{Word: e.Text, Action: e.Action})
	}
	return rows
}

// settingsBlacklistPrecheck sorts the typed words against the fresh list the way applyBlacklistAdd does and reports the
// counts of what would be refused and how many words are addable, so a message with nothing to add is refused before
// the wait is claimed.
func settingsBlacklistPrecheck(words []string, existing models.BlacklistSettingsSlice) (settingsBlacklistCounts, int) {
	var counts settingsBlacklistCounts
	listed := make(map[string]struct{}, len(existing))
	for _, word := range existing.Triggers() {
		listed[word] = struct{}{}
	}
	valid, tooLong := blacklistSortWords(words)
	counts.TooLong = len(tooLong)
	addable := 0
	for _, word := range valid {
		switch _, isListed := listed[strings.ToLower(word)]; {
		case keywordTooBroad(word):
			counts.TooBroad++
		case isListed:
			counts.Already++
		default:
			addable++
		}
	}
	return counts, addable
}

// settingsBlacklistTyped is the door's handler of a typed reply to the Blacklist wait (UI-SPEC section 6, one step):
// the words are the text or caption of the message, split and de-duplicated, sorted against a fresh read, and, when at
// least one is addable, claimed and stored through applyBlacklistAdd, the function /addblacklist stores through. A message
// with no word, or none addable, is refused with the wait kept; a write that fails for every word releases the claim.
func settingsBlacklistTyped(r *settingsInputReply, _ settingsListPart) {
	words := settingsBlacklistTypedWords(r.msg.GetText())
	if len(words) == 0 {
		r.refuse(r.text("settings_edit_refuse_need_text"))
		return
	}
	readCtx, cancelRead := context.WithTimeout(context.Background(), settingsReadTimeout)
	entries, err := settingsBlacklistRead(readCtx, r.wait.GroupID)
	cancelRead()
	if err != nil {
		log.Warnf("[Settings] the blacklist of chat %d could not be read for a typed reply: %v", r.wait.GroupID, err)
		r.keep("settings_check_failed")
		return
	}
	existing := settingsBlacklistRows(entries)
	counts, addable := settingsBlacklistPrecheck(words, existing)
	if addable == 0 {
		r.refuse(settingsBlacklistRefusalText(r.tr, counts))
		return
	}
	claim, claimed := r.claimWait()
	if !claimed {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), settingsBlacklistApplyTimeout)
	defer cancel()
	result := applyBlacklistAdd(ctx, r.wait.GroupID, words, existing)
	counts = settingsBlacklistCountsOf(result)
	if result.Outcome != blacklistAdded {
		r.releaseWait(claim)
		if result.Outcome == blacklistAddSaveFailed {
			log.Errorf("[Settings] the typed blacklist words of chat %d could not be saved for user %d", r.wait.GroupID, r.user.Id)
			r.refuse(r.text("common_settings_save_failed"))
			return
		}
		// Another admin listed every word between the read and the write: nothing is left to add.
		r.refuse(settingsBlacklistRefusalText(r.tr, counts))
		return
	}
	r.finishWait(claim)
	log.Infof("[Settings] user %d added %d typed words to the blacklist of chat %d", r.user.Id, counts.Added, r.wait.GroupID)
	r.sendSavedList(*r.list, result.Added[0], settingsBlacklistSavedNotice(r.tr, counts, settingsInputIsAlbum(r.msg)))
	r.closePrompt()
}

// settingsBlacklistDef is the table of the Blacklist page.
func settingsBlacklistDef() settingsListDef {
	return settingsListDef{
		Key:       settingsPageBlacklist,
		Order:     46,
		Icon:      "🔤",
		TitleKey:  "settings_page_blacklist",
		PagerCode: "bp",
		AskCode:   "bx",
		YesCode:   "by",
		// Every blacklist write declares the right /addblacklist asks for (D-03, M42).
		Right:        settingsRightRestrictBotRestrict,
		Remove:       settingsBlacklistRemove,
		DeleteAskKey: "settings_bl_delete_ask",
		RemovedKey:   "settings_bl_removed",
		EntryParam:   "word",
		Read: func(ctx context.Context, chatID int64) ([]settingsListEntry, error) {
			return settingsBlacklistRead(ctx, chatID)
		},
		Entry: func(ctx context.Context, chatID, id int64) (*settingsListEntry, error) {
			return settingsBlacklistEntry(ctx, chatID, id)
		},
		Count:       func(ctx context.Context, chatID int64) (int, error) { return settingsBlacklistCount(ctx, chatID) },
		AddCode:     "bn",
		CancelCode:  "bc",
		AddKey:      "settings_btn_bl_add",
		Prompt:      settingsBlacklistPrompt,
		Typed:       settingsBlacklistTyped,
		IntroKey:    "settings_bl_intro",
		CountKey:    "settings_bl_count",
		EmptyKey:    "settings_bl_empty",
		BodyExtra:   settingsBlacklistActionLine,
		AddRowExtra: settingsBlacklistAddRow,
		Actions:     settingsBlacklistActions(),
		// A notice refusal of the action set (the bot cannot restrict) redraws the action screen it came from.
		Redraw: map[string]func(*settingsPress, string) (settingsScreen, error){settingsActBlacklistSet: settingsBlacklistDrawAction},
	}
}

func init() {
	registerSettingsList(settingsBlacklistDef())
}
