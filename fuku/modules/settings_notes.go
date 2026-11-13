package modules

import (
	"context"
	"errors"
	"strings"

	"github.com/uasneppy/fuku_robot/fuku/db/models"
	db_notes "github.com/uasneppy/fuku_robot/fuku/db/notes"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/content"
)

// The Notes page of the settings menu (page e, Phase 19) is the third table of the list engine (settings_list.go)
// on the engine's two-step add (settings_list_input.go): the page lists the chat's notes eight a page, tagging an
// admin-only note, and adds one from two typed messages (the name, then the content). It gives hooks only and
// writes no flow code: the engine runs both steps, the content is extracted by content.ExtractTypedNoteOrFilter
// (the form /save NAME replying to a message stores) and stored through applyNoteAdd, the function /save stores
// through. It reads through the fresh, chat-scoped, non-creating readers of the notes repository and never reads
// notes_settings (the private-notes switch is /privnote's), so opening the page creates no row. A name that already
// exists is refused, never overwritten (research C-09). It is private-only: a menu opened in the group shows only
// the framework's note.

const (
	// settingsPageNotes is the p value of the Notes page.
	settingsPageNotes = "e"
)

// The reads of the page, variables only so a test can count or fail them. Each adapts the repository's row to the
// engine's entry through the engine's own adapters.
var (
	// settingsNotesRead lists the chat's notes (ID, name and admin-only flag).
	settingsNotesRead = settingsListReader(db_notes.ListNoteEntriesFresh, settingsNotesEntryOf)
	// settingsNotesEntry reads one note by ID, only when it belongs to the chat.
	settingsNotesEntry = settingsListEntryReader(db_notes.GetNoteEntryFresh, settingsNotesEntryOf)
	// settingsNotesCount counts the chat's notes.
	settingsNotesCount = settingsListCounter(db_notes.CountNotesFresh)
)

// settingsNotesEntryOf is the engine's entry of a stored note.
func settingsNotesEntryOf(row models.Notes) settingsListEntry {
	return settingsListEntry{ID: int64(row.ID), Text: row.NoteName, AdminOnly: row.AdminOnly}
}

// settingsNotesNormalise is the name the first typed message gives: trimmed and lower-cased, the form /save stores.
// White space inside the name is not removed: checkNoteName refuses it, as the command does.
func settingsNotesNormalise(text string) string {
	return strings.ToLower(strings.TrimSpace(text))
}

// settingsNotesPrompt is the lines of the add prompt before its footer (Screen 5): the name prompt and its rules for
// step 1, the content prompt for step 2.
func settingsNotesPrompt(v settingsView, part string) []string {
	if part == "c" {
		return []string{settingsText(v.Tr, "settings_notes_wait_content")}
	}
	return []string{
		settingsText(v.Tr, "settings_notes_wait_name"),
		settingsText(v.Tr, "settings_notes_wait_name_rules", i18n.TranslationParams{"max": listKeywordMaxRunes}),
	}
}

// settingsNotesCheck is the step-1 check of a name, and the check step 2 runs again against a fresh read:
// checkNoteName, the check /save runs, so the command and the page refuse alike. It returns the finished refusal
// and, for a read that failed, an error that keeps the wait (nothing is stored on a guess).
func settingsNotesCheck(ctx context.Context, chatID int64, norm string, tr *i18n.Translator) (string, error) {
	result := checkNoteName(ctx, chatID, norm)
	switch result.Outcome {
	case noteAdded:
		return "", nil
	case noteNameBlank:
		return settingsText(tr, "settings_entries_refuse_blank"), nil
	case noteNameBadShape:
		return settingsText(tr, "notes_name_invalid"), nil
	}
	if result.Err != nil {
		return "", result.Err
	}
	return "", errSettingsNotesCheck
}

// errSettingsNotesCheck is the error of a check that refused for a reason it has no words for.
var errSettingsNotesCheck = errors.New("the note name check gave no answer")

// settingsNotesApply stores the typed content through applyNoteAdd, the function /save stores through, after the
// same private-flag rule /save applies: a note with both {private} and {noprivate} keeps neither and the saved notice
// says so with /save's own warning text.
func settingsNotesApply(ctx context.Context, chatID int64, norm string, ex content.ExtractResult, tr *i18n.Translator) settingsListApplyResult {
	pvtOnly, grpOnly, conflict := resolveNotePrivateConflict(ex.PvtOnly, ex.GrpOnly)
	result := applyNoteAdd(ctx, chatID, noteAddInput{
		Name:        norm,
		Text:        ex.Text,
		FileID:      ex.FileID,
		Buttons:     ex.Buttons,
		DataType:    ex.DataType,
		PvtOnly:     pvtOnly,
		GrpOnly:     grpOnly,
		AdminOnly:   ex.AdminOnly,
		WebPrev:     ex.WebPreview,
		IsProtected: ex.IsProtected,
		NoNotif:     ex.NoNotif,
	})
	if result.Outcome != noteAdded {
		return settingsListApplyResult{Outcome: settingsListApplyFailed}
	}
	if !conflict {
		return settingsListApplyResult{}
	}
	// The warning is /save's own text, which starts with the blank lines that set it off in a reply.
	return settingsListApplyResult{Notice: strings.TrimLeft(settingsText(tr, "notes_private_conflict_warning"), "\n")}
}

// settingsNotesDef is the table of the Notes page.
func settingsNotesDef() settingsListDef {
	return settingsListDef{
		Key:       settingsPageNotes,
		Order:     72,
		Icon:      "📝",
		TitleKey:  "settings_page_notes",
		PagerCode: "eg",
		// Every note write declares the right /save and /clear ask for (D-03): can_change_info and the bot an
		// administrator.
		Right: settingsRightChangeInfoBotAdmin,
		Read: func(ctx context.Context, chatID int64) ([]settingsListEntry, error) {
			return settingsNotesRead(ctx, chatID)
		},
		Entry: func(ctx context.Context, chatID, id int64) (*settingsListEntry, error) {
			return settingsNotesEntry(ctx, chatID, id)
		},
		Count:       func(ctx context.Context, chatID int64) (int, error) { return settingsNotesCount(ctx, chatID) },
		EntryParam:  "name",
		IntroKey:    "settings_notes_intro",
		CountKey:    "settings_notes_count",
		EmptyKey:    "settings_notes_empty",
		AdminTagKey: "settings_notes_admin_tag",
		AddCode:     "ea",
		CancelCode:  "ec",
		AddKey:      "settings_btn_notes_add",
		Prompt:      settingsNotesPrompt,
		TwoStep: &settingsListTwoStep{
			Normalise:  settingsNotesNormalise,
			Check:      settingsNotesCheck,
			Apply:      settingsNotesApply,
			KeyLineKey: "settings_notes_name_line",
			SavedKey:   "settings_notes_saved",
		},
	}
}

func init() {
	registerSettingsList(settingsNotesDef())
}
