//go:build testtools

package modules

import (
	"slices"
	"strings"
	"testing"

	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// settingsListKeys are the locale keys a list table reads through its fields, which make check-translations cannot
// see because they are not literal arguments of a GetString call.
func settingsListKeys(def settingsListDef) []string {
	keys := []string{def.TitleKey, def.IntroKey, def.CountKey, def.EmptyKey, def.DeleteAskKey, def.RemovedKey, def.AdminTagKey, def.AddKey}
	if def.TwoStep != nil {
		keys = append(keys, def.TwoStep.KeyLineKey, def.TwoStep.SavedKey)
	}
	return slices.DeleteFunc(keys, func(key string) bool { return key == "" })
}

// settingsListExtraKeys are the keys the list pages' typed add reads through code the engine does not enumerate: the
// part words of the static part table and the prompt lines of each table's Prompt.
func settingsListExtraKeys() []string {
	keys := []string{
		"settings_bl_wait", "settings_bl_wait_rules",
		// The Filters page's two prompts, its two refusals and its cap notice (Phase 19, plan 19-07).
		"settings_flt_wait_keyword", "settings_flt_wait_keyword_rules", "settings_flt_wait_reply",
		"settings_flt_refuse_exists", "settings_flt_refuse_cap",
		// The Notes page's two prompts (Phase 19, plan 19-09).
		"settings_notes_wait_name", "settings_notes_wait_name_rules", "settings_notes_wait_content", "settings_notes_refuse_exists",
		// The Blocked sources page's prompt lines, the action screen's intro and hint, the saved notice and its refusal.
		"settings_fwd_wait", "settings_fwd_wait_rules", "settings_fwd_action_hint", "settings_fwd_saved", "settings_fwd_refuse_one",
		"settings_bl_action_intro", "settings_btn_bl_action",
	}
	for _, part := range settingsListParts {
		keys = append(keys, part.WordKey)
	}
	return keys
}

// TestSettingsListKeysExist is the guard of every locale key a registered list table reads through a field: a key
// missing from one language would ship an empty line, button or card. Every key must translate to a non-empty
// string in all 8 languages, and a card or notice key must keep the placeholder the engine splices the entry into.
func TestSettingsListKeysExist(t *testing.T) {
	withRealLocales(t)
	if _, ok := settingsListByPage(settingsPageBlacklist); !ok {
		t.Fatal("the Blacklist table is not registered")
	}
	for _, def := range settingsLists {
		for _, code := range memberReqScreenLanguages {
			tr := i18n.MustNewTranslator(code)
			for _, key := range settingsListKeys(def) {
				text, err := tr.GetString(key)
				if err != nil || strings.TrimSpace(text) == "" {
					t.Errorf("%s: page %s key %q does not translate: %q, %v", code, def.Key, key, text, err)
				}
			}
			for _, key := range settingsListExtraKeys() {
				text, err := tr.GetString(key)
				if err != nil || strings.TrimSpace(text) == "" {
					t.Errorf("%s: typed-add key %q does not translate: %q, %v", code, key, text, err)
				}
			}
			for _, key := range settingsListEntryKeys(def) {
				if key == "" {
					continue
				}
				if text, _ := tr.GetString(key); !strings.Contains(text, "{"+def.EntryParam+"}") {
					t.Errorf("%s: page %s key %q lost the {%s} placeholder: %q", code, def.Key, key, def.EntryParam, text)
				}
			}
		}
	}
}

// settingsListEntryKeys are the keys of a table that splice the entry under its EntryParam: the delete card, the
// removed notice and, for a two-step page, the step-2 key line and the saved notice.
func settingsListEntryKeys(def settingsListDef) []string {
	keys := []string{def.DeleteAskKey, def.RemovedKey}
	if def.TwoStep != nil {
		keys = append(keys, def.TwoStep.KeyLineKey, def.TwoStep.SavedKey)
	}
	return keys
}
