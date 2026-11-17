package i18n

import (
	"fmt"
	"strings"
)

// StringTable returns one map per language holding exactly the given keys, for a page
// that carries its own strings (the Turnstile web captcha page, D-20). Each language is read
// from its own locale only, with no fallback to English (I10): a page that says it is in a
// language must not show English text, so a key a language lacks or holds empty, and a
// language the manager has no locale for, are errors.
//
// The error names the language and the key but never a value, so a page can never ship an
// empty or a borrowed string. The page picks a table client-side for display only; nothing
// security-relevant depends on which language it shows.
func (lm *LocaleManager) StringTable(langs, keys []string) (map[string]map[string]string, error) {
	tables := make(map[string]map[string]string, len(langs))
	for _, lang := range langs {
		translator, err := lm.GetTranslator(lang)
		if err != nil {
			return nil, fmt.Errorf("i18n string table: translator for language %q: %w", lang, err)
		}
		if translator.langCode != lang {
			return nil, fmt.Errorf("i18n string table: language %q has no locale", lang)
		}
		table := make(map[string]string, len(keys))
		for _, key := range keys {
			value := translator.lookupString(key)
			if value == "<nil>" || strings.TrimSpace(value) == "" {
				return nil, fmt.Errorf("i18n string table: language %q key %q is missing or empty", lang, key)
			}
			table[key] = value
		}
		tables[lang] = table
	}
	return tables, nil
}
