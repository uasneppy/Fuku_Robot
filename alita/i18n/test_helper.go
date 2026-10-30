//go:build testtools

package i18n

import (
	"embed"
	"errors"
)

func NewTestTranslator(yamlContent string) (*Translator, error) {
	data, err := parseYAML([]byte(yamlContent))
	if err != nil {
		return nil, err
	}
	lm := &LocaleManager{
		defaultLang: "en",
		localeMaps:  map[string]map[string]any{"en": data},
	}
	return &Translator{
		langCode: "en",
		manager:  lm,
		data:     data,
	}, nil
}

// OverrideManagerForTest replaces the global LocaleManager singleton with one backed
// by the provided inline YAML content. It returns a restore function that reverts
// managerInstance to its prior value; call it via t.Cleanup. This is intentionally
// NOT goroutine-safe across parallel sub-tests that also call this function — run
// the parent test without t.Parallel() or ensure sub-tests share the same override.
//
// It is guarded by the testtools build tag and is never compiled into production.
func OverrideManagerForTest(yamlContent string) (restore func(), err error) {
	data, err := parseYAML([]byte(yamlContent))
	if err != nil {
		return nil, err
	}
	// A non-nil *embed.FS is required to pass GetTranslator's localeFS nil-guard.
	// The value is never used for file I/O because localeMaps is already populated.
	var dummyFS embed.FS
	lm := &LocaleManager{
		defaultLang: "en",
		localeMaps:  map[string]map[string]any{"en": data},
		localeFS:    &dummyFS,
	}
	// Ensure managerOnce.Do has already fired before we overwrite managerInstance.
	// If we set managerInstance first and Then GetManager() is called, Once.Do would
	// fire and overwrite our value with the empty default singleton.
	_ = GetManager()
	prev := managerInstance
	managerInstance = lm
	return func() { managerInstance = prev }, nil
}

// OverrideManagerLanguagesForTest is OverrideManagerForTest for several languages at once:
// yamlByLang maps a language code to the full YAML text of its locale file, and "en" must be
// among them (it is the default). A test that needs the real words of a language, for
// example to prove a menu speaks the group's language, passes the real locale files. It
// returns a restore function to call from t.Cleanup, and is guarded by the testtools build
// tag like the helpers above.
func OverrideManagerLanguagesForTest(yamlByLang map[string]string) (restore func(), err error) {
	localeMaps := make(map[string]map[string]any, len(yamlByLang))
	for code, content := range yamlByLang {
		data, parseErr := parseYAML([]byte(content))
		if parseErr != nil {
			return nil, parseErr
		}
		localeMaps[code] = data
	}
	if _, ok := localeMaps["en"]; !ok {
		return nil, errors.New("OverrideManagerLanguagesForTest: the default language en is missing")
	}
	var dummyFS embed.FS
	lm := &LocaleManager{
		defaultLang: "en",
		localeMaps:  localeMaps,
		localeFS:    &dummyFS,
	}
	_ = GetManager()
	prev := managerInstance
	managerInstance = lm
	return func() { managerInstance = prev }, nil
}
