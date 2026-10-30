package i18n

import (
	"fmt"
	"strings"
)

// StringTable returns one map per language holding exactly the given keys, for a page
// that carries its own strings (the Turnstile web captcha page, D-20). Each language
// resolves through its translator, so a key a language lacks falls back to English, and
// a language the manager does not know gets the English values.
//
// It fails when a key is absent from English or empty there (or blank in the result),
// naming the language and the key but never a value, so a page can never ship an empty
// string. The page picks a table client-side for display only; nothing
// security-relevant depends on which language it shows.
func (lm *LocaleManager) StringTable(langs, keys []string) (map[string]map[string]string, error) {
	tables := make(map[string]map[string]string, len(langs))
	for _, lang := range langs {
		translator, err := lm.GetTranslator(lang)
		if err != nil {
			return nil, fmt.Errorf("i18n string table: translator for language %q: %w", lang, err)
		}
		table := make(map[string]string, len(keys))
		for _, key := range keys {
			value, err := translator.GetString(key)
			if err != nil {
				return nil, fmt.Errorf("i18n string table: language %q key %q: %w", lang, key, err)
			}
			if strings.TrimSpace(value) == "" {
				return nil, fmt.Errorf("i18n string table: language %q key %q is empty", lang, key)
			}
			table[key] = value
		}
		tables[lang] = table
	}
	return tables, nil
}
