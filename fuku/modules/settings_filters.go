package modules

import (
	"context"
	"errors"
	"strings"

	log "github.com/sirupsen/logrus"

	db_filters "github.com/uasneppy/fuku_robot/fuku/db/filters"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/content"
)

// The Filters page of the settings menu (page i, Phase 19) is the second table of the list engine
// (settings_list.go) on the engine's two-step add (settings_list_input.go): the page lists the chat's filter
// keywords eight a page, deletes one behind a two-tap card that binds the row, and adds one from two typed
// messages (the keyword, then the reply). It gives hooks only and writes no flow code: the engine runs both steps,
// the reply is extracted by content.ExtractTypedNoteOrFilter (the form /filter K replying to a message stores) and
// stored through applyFilterAdd, the function /filter stores through. It reads through the fresh, chat-scoped,
// non-creating readers of the filters repository. A keyword that already exists is refused, never overwritten
// (research C-09): overwriting is editing content, which the menu does not do. It is private-only: a menu opened
// in the group shows only the framework's note.

const (
	// settingsPageFilters is the p value of the Filters page.
	settingsPageFilters = "i"
)

// The reads of the page, variables only so a test can count or fail them. Each adapts the repository's row to
// the engine's entry through the engine's own adapters.
var (
	// settingsFiltersRead lists the chat's filters (ID and keyword).
	settingsFiltersRead = settingsListReader(db_filters.ListFilterEntriesFresh, settingsFiltersEntryOf)
	// settingsFiltersEntry reads one filter by ID, only when it belongs to the chat.
	settingsFiltersEntry = settingsListEntryReader(db_filters.GetFilterEntryFresh, settingsFiltersEntryOf)
	// settingsFiltersCount counts the chat's filters.
	settingsFiltersCount = settingsListCounter(db_filters.CountFiltersContext)
)

// settingsFiltersEntryOf is the engine's entry of a stored filter.
func settingsFiltersEntryOf(row models.ChatFilters) settingsListEntry {
	return settingsListEntry{ID: int64(row.ID), Text: row.KeyWord}
}

// settingsFiltersNormalise is the keyword the first typed message names: its words joined by one space and
// lower-cased, the form /filter stores.
func settingsFiltersNormalise(text string) string {
	return strings.ToLower(strings.Join(strings.Fields(text), " "))
}

// settingsFiltersPrompt is the lines of the add prompt before its footer (Screen 5): the keyword prompt and its rules
// for step 1, the reply prompt for step 2.
func settingsFiltersPrompt(v settingsView, part string) []string {
	if part == "y" {
		return []string{settingsText(v.Tr, "settings_flt_wait_reply")}
	}
	return []string{
		settingsText(v.Tr, "settings_flt_wait_keyword"),
		settingsText(v.Tr, "settings_flt_wait_keyword_rules", i18n.TranslationParams{"max": listKeywordMaxRunes}),
	}
}

// settingsFiltersExistsText is the refusal of a keyword the chat already has, naming it (escaped, cut at 100 code points).
func settingsFiltersExistsText(tr *i18n.Translator, keyword string) string {
	return settingsListEntryText(tr, "settings_flt_refuse_exists", "keyword", keyword)
}

// settingsFiltersCapText is the refusal of an Add at the cap of filtersCapMax filters.
func settingsFiltersCapText(tr *i18n.Translator) string {
	return settingsText(tr, "settings_flt_refuse_cap", i18n.TranslationParams{"max": filtersCapMax})
}

// settingsFiltersCheck is the step-1 check of a keyword, and the check step 2 runs again against a fresh read:
// checkFilterKeyword, the check /filter runs, so the command and the page refuse alike. It returns the finished
// refusal and, for a read that failed, an error that keeps the wait (nothing is stored on a guess).
func settingsFiltersCheck(ctx context.Context, chatID int64, norm string, tr *i18n.Translator) (string, error) {
	result := checkFilterKeyword(ctx, chatID, norm)
	switch result.Outcome {
	case filterAdded:
		return "", nil
	case filterKeywordBlank:
		return settingsText(tr, "settings_entries_refuse_blank"), nil
	case filterKeywordTooLong:
		return settingsText(tr, "filters_keyword_too_long"), nil
	case filterKeywordTooBroad:
		return settingsText(tr, "filters_keyword_too_broad"), nil
	case filterExists:
		return settingsFiltersExistsText(tr, norm), nil
	case filterCapReached:
		return settingsFiltersCapText(tr), nil
	}
	if result.Err != nil {
		return "", result.Err
	}
	return "", errSettingsFiltersCheck
}

// errSettingsFiltersCheck is the error of a check that refused for a reason it has no words for.
var errSettingsFiltersCheck = errors.New("the filter keyword check gave no answer")

// settingsFiltersApply stores the typed reply through applyFilterAdd, the function /filter stores through. A keyword
// stored by another admin between the steps (filterExists, filterRaced) and a cap reached meanwhile are taken: the wait
// ends with the reason, and nothing is overwritten.
func settingsFiltersApply(ctx context.Context, chatID int64, norm string, ex content.ExtractResult, tr *i18n.Translator) settingsListApplyResult {
	result := applyFilterAdd(ctx, chatID, filterAddInput{
		Keyword:  norm,
		Text:     ex.Text,
		FileID:   ex.FileID,
		Buttons:  ex.Buttons,
		DataType: ex.DataType,
	})
	switch result.Outcome {
	case filterAdded:
		return settingsListApplyResult{}
	case filterExists, filterRaced:
		return settingsListApplyResult{Outcome: settingsListApplyTaken, Text: settingsFiltersExistsText(tr, norm)}
	case filterCapReached:
		return settingsListApplyResult{Outcome: settingsListApplyTaken, Text: settingsFiltersCapText(tr)}
	}
	return settingsListApplyResult{Outcome: settingsListApplyFailed}
}

// settingsFiltersRemove deletes one filter by its row ID through applyFilterRemoveByID, the function the command's
// delete shares with the page: one conditional DELETE by (id, chat) and the drop of both filter caches.
func settingsFiltersRemove(ctx context.Context, chatID, id int64) settingsListRemoval {
	result := applyFilterRemoveByID(ctx, chatID, id)
	switch result.Outcome {
	case filterRemovedByID:
		return settingsListRemoval{Outcome: settingsListRemoved, Text: result.Keyword}
	case filterRemoveByIDGone:
		return settingsListRemoval{Outcome: settingsListRemoveGone}
	}
	return settingsListRemoval{Outcome: settingsListRemoveFailed}
}

// settingsFiltersAddGate is consulted before the Add press writes a wait: at the cap of filtersCapMax filters (a fresh
// count) page 1 is redrawn with the cap notice and no wait is created. A count that cannot be read is the cannot-open
// edit. It returns nil when Add may go on.
func settingsFiltersAddGate(p *settingsPress) *settingsScreen {
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()
	n, err := settingsFiltersCount(ctx, p.group.Id)
	if err == nil && n < filtersCapMax {
		return nil
	}
	def, _ := settingsListByPage(settingsPageFilters)
	var screen settingsScreen
	if err == nil {
		screen, err = settingsListDraw(p, def, 1, settingsNoticeWarn(settingsFiltersCapText(p.tr)))
	}
	if err != nil {
		log.Warnf("[Settings] the filters of chat %d could not be counted for Add: %v", p.group.Id, err)
		p.answer(settingsText(p.tr, "settings_cannot_open_alert"), true)
		return &settingsScreen{Text: settingsText(p.tr, "settings_cannot_open")}
	}
	return &screen
}

// settingsFiltersDef is the table of the Filters page.
func settingsFiltersDef() settingsListDef {
	return settingsListDef{
		Key:       settingsPageFilters,
		Order:     47,
		Icon:      "🔁",
		TitleKey:  "settings_page_filters",
		PagerCode: "ig",
		AskCode:   "ix",
		YesCode:   "iy",
		// Every filter write declares the right /filter asks for (D-03).
		Right:        settingsRightChangeInfo,
		CountMax:     filtersCapMax,
		Remove:       settingsFiltersRemove,
		DeleteAskKey: "settings_flt_delete_ask",
		RemovedKey:   "settings_flt_removed",
		Read: func(ctx context.Context, chatID int64) ([]settingsListEntry, error) {
			return settingsFiltersRead(ctx, chatID)
		},
		Entry: func(ctx context.Context, chatID, id int64) (*settingsListEntry, error) {
			return settingsFiltersEntry(ctx, chatID, id)
		},
		Count:      func(ctx context.Context, chatID int64) (int, error) { return settingsFiltersCount(ctx, chatID) },
		EntryParam: "keyword",
		IntroKey:   "settings_flt_intro",
		CountKey:   "settings_flt_count",
		EmptyKey:   "settings_flt_empty",
		AddCode:    "ia",
		CancelCode: "ic",
		AddKey:     "settings_btn_flt_add",
		Prompt:     settingsFiltersPrompt,
		AddGate:    settingsFiltersAddGate,
		TwoStep: &settingsListTwoStep{
			Normalise:  settingsFiltersNormalise,
			Check:      settingsFiltersCheck,
			IsFilter:   true,
			Apply:      settingsFiltersApply,
			KeyLineKey: "settings_flt_keyword_line",
			SavedKey:   "settings_flt_saved",
		},
	}
}

func init() {
	registerSettingsList(settingsFiltersDef())
}
