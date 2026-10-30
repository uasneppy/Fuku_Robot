//go:build testtools

package i18n

import (
	"embed"
	"strings"
	"testing"

	"github.com/divkix/Alita_Robot/alita/utils/captchaweb"
)

// newTableTestManager builds a manager from already parsed locale maps, the way
// OverrideManagerForTest builds one from inline YAML.
func newTableTestManager(maps map[string]map[string]any) *LocaleManager {
	var fs embed.FS
	return &LocaleManager{
		defaultLang: "en",
		localeMaps:  maps,
		localeFS:    &fs,
	}
}

func TestStringTable(t *testing.T) {
	t.Run("real locales", func(t *testing.T) {
		maps := make(map[string]map[string]any, len(staffLocales))
		for _, lang := range staffLocales {
			maps[lang] = loadStaffLocale(t, lang)
		}
		lm := newTableTestManager(maps)

		table, err := lm.StringTable(captchaweb.PageLanguages, captchaweb.PageKeys)
		if err != nil {
			t.Fatalf("StringTable: %v", err)
		}
		if len(table) != len(captchaweb.PageLanguages) {
			t.Fatalf("got %d language tables, want %d", len(table), len(captchaweb.PageLanguages))
		}
		for _, lang := range captchaweb.PageLanguages {
			strs, ok := table[lang]
			if !ok {
				t.Fatalf("no table for %s", lang)
			}
			if len(strs) != len(captchaweb.PageKeys) {
				t.Errorf("%s: %d strings, want %d", lang, len(strs), len(captchaweb.PageKeys))
			}
			for _, key := range captchaweb.PageKeys {
				if strings.TrimSpace(strs[key]) == "" {
					t.Errorf("%s: empty string for %s", lang, key)
				}
			}
		}
		if table["es"]["captcha_web_ready_title"] == table["en"]["captcha_web_ready_title"] {
			t.Error("the es ready title is the English one, so the table did not use the es locale")
		}
		wantClose, _ := maps["ru"]["captcha_web_close_button"].(string)
		if got := table["ru"]["captcha_web_close_button"]; got != wantClose || got == "" {
			t.Errorf("ru close button = %q, want the ru file's %q", got, wantClose)
		}
	})

	t.Run("English fallback", func(t *testing.T) {
		keys := []string{"captcha_web_a", "captcha_web_b"}
		lm := newTableTestManager(map[string]map[string]any{
			"en": {"captcha_web_a": "Alpha", "captcha_web_b": "Beta"},
			"es": {"captcha_web_a": "Alfa"},
		})

		table, err := lm.StringTable([]string{"en", "es"}, keys)
		if err != nil {
			t.Fatalf("StringTable: %v", err)
		}
		if got := table["es"]["captcha_web_a"]; got != "Alfa" {
			t.Errorf("es a = %q, want Alfa", got)
		}
		if got := table["es"]["captcha_web_b"]; got != "Beta" {
			t.Errorf("es b = %q, want the English Beta", got)
		}
	})

	t.Run("missing in English", func(t *testing.T) {
		lm := newTableTestManager(map[string]map[string]any{
			"en": {"captcha_web_a": "Alpha"},
			"es": {"captcha_web_a": "Alfa", "captcha_web_b": "Beta"},
		})

		table, err := lm.StringTable([]string{"en", "es"}, []string{"captcha_web_a", "captcha_web_b"})
		if err == nil {
			t.Fatal("StringTable accepted a key English lacks")
		}
		if table != nil {
			t.Errorf("got a table %v alongside the error", table)
		}
		if !strings.Contains(err.Error(), "captcha_web_b") {
			t.Errorf("error %q does not name the key", err)
		}
	})

	t.Run("empty in English", func(t *testing.T) {
		lm := newTableTestManager(map[string]map[string]any{
			"en": {"captcha_web_a": "  "},
		})

		table, err := lm.StringTable([]string{"en"}, []string{"captcha_web_a"})
		if err == nil || table != nil {
			t.Fatalf("StringTable = %v, %v; want an error and no table for a blank English value", table, err)
		}
	})

	t.Run("unknown language", func(t *testing.T) {
		lm := newTableTestManager(map[string]map[string]any{
			"en": {"captcha_web_a": "Alpha"},
		})

		table, err := lm.StringTable([]string{"de"}, []string{"captcha_web_a"})
		if err != nil {
			t.Fatalf("StringTable: %v", err)
		}
		if got := table["de"]["captcha_web_a"]; got != "Alpha" {
			t.Errorf("de a = %q, want the English Alpha", got)
		}
	})
}
