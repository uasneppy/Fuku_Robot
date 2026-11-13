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
	return slices.DeleteFunc(keys, func(key string) bool { return key == "" })
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
			for _, key := range []string{def.DeleteAskKey, def.RemovedKey} {
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
