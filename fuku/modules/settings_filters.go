package modules

import (
	"context"
	"errors"
	"strings"

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
// the engine's entry.
var (
	// settingsFiltersRead lists the chat's filters (ID and keyword).
	settingsFiltersRead = func(ctx context.Context, chatID int64) ([]settingsListEntry, error) {
		rows, err := db_filters.ListFilterEntriesFresh(ctx, chatID)
		if err != nil {
			return nil, err
		}
		entries := make([]settingsListEntry, 0, len(rows))
		for _, row := range rows {
			entries = append(entries, settingsFiltersEntryOf(row))
		}
		return entries, nil
	}
	// settingsFiltersEntry reads one filter by ID, only when it belongs to the chat.
	settingsFiltersEntry = func(ctx context.Context, chatID, id int64) (*settingsListEntry, error) {
		row, err := db_filters.GetFilterEntryFresh(ctx, chatID, id)
		if err != nil || row == nil {
			return nil, err
		}
		entry := settingsFiltersEntryOf(*row)
		return &entry, nil
	}
	// settingsFiltersCount counts the chat's filters.
	settingsFiltersCount = func(ctx context.Context, chatID int64) (int, error) {
		n, err := db_filters.CountFiltersContext(ctx, chatID)
		return int(n), err
	}
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

// settingsFiltersCheck is the step-1 check of a keyword, and the check step 2 runs again against a fresh read:
// checkFilterKeyword, the check /filter runs, so the command and the page refuse alike. It returns the finished
// refusal and, for a read that failed, an error that keeps the wait.
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
	}
	if result.Err != nil {
		return "", result.Err
	}
	// The keyword exists or the cap is reached: refused through the read error path until the page words them.
	return "", errors.New("filter keyword refused")
}

// settingsFiltersApply stores the typed reply through applyFilterAdd, the function /filter stores through. A keyword
// stored by another admin between the steps (filterExists, filterRaced) is taken, never overwritten.
func settingsFiltersApply(ctx context.Context, chatID int64, norm string, ex content.ExtractResult, _ *i18n.Translator) settingsListApplyResult {
	result := applyFilterAdd(ctx, chatID, filterAddInput{
		Keyword:  norm,
		Text:     ex.Text,
		FileID:   ex.FileID,
		Buttons:  ex.Buttons,
		DataType: ex.DataType,
	})
	if result.Outcome == filterAdded {
		return settingsListApplyResult{}
	}
	return settingsListApplyResult{Outcome: settingsListApplyFailed}
}

// settingsFiltersDef is the table of the Filters page.
func settingsFiltersDef() settingsListDef {
	return settingsListDef{
		Key:       settingsPageFilters,
		Order:     47,
		Icon:      "🔁",
		TitleKey:  "settings_page_filters",
		PagerCode: "ig",
		// Every filter write declares the right /filter asks for (D-03).
		Right:    settingsRightChangeInfo,
		CountMax: filtersCapMax,
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
