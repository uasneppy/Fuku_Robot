//go:build testtools

package modules

import (
	"strings"
	"testing"

	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// TestDeletionKindKeysExist is the guard of every locale key the Deletion page reads through a
// table field: make check-translations only sees keys that are literal GetString arguments, so
// the NameKey, DescKey and HintKey of the service and bot tables, the two section names and the
// delay words would ship an empty message if a language lacked one. Every key must translate to
// a non-empty string in all 8 languages, and the two templates that take a parameter must splice
// it in.
func TestDeletionKindKeysExist(t *testing.T) {
	withRealLocales(t)

	fixed := []string{
		"settings_del_sec_service", "settings_del_sec_bot", "settings_del_delay_10s", "settings_del_delay_1m",
		"settings_del_delay_10m", "settings_del_delay_1h", "settings_del_delay_6h", "settings_del_delay_24h",
		"settings_del_state_keep", "settings_del_state_now",
	}

	for _, code := range memberReqScreenLanguages {
		tr := i18n.MustNewTranslator(code)
		translates := func(what, key string, params i18n.TranslationParams) string {
			text, err := tr.GetString(key, params)
			if err != nil || strings.TrimSpace(text) == "" {
				t.Errorf("%s: %s %q does not translate: %q, %v", code, what, key, text, err)
			}
			return text
		}

		for _, key := range fixed {
			translates("page key", key, nil)
		}

		for _, table := range []struct {
			name string
			rows []deletionRow
		}{{"service", deletionServiceRows}, {"bot", deletionBotRows}} {
			for _, row := range table.rows {
				translates(table.name+" row "+row.Code+" NameKey", row.NameKey, nil)
				translates(table.name+" row "+row.Code+" DescKey", row.DescKey, nil)
				if row.HintKey != "" {
					translates(table.name+" row "+row.Code+" HintKey", row.HintKey, nil)
				}
			}
		}

		after := translates("state template", "settings_del_state_after", i18n.TranslationParams{"delay": "DELAYMARK"})
		if !strings.Contains(after, "DELAYMARK") {
			t.Errorf("%s: settings_del_state_after does not carry {delay}: %q", code, after)
		}
		currently := translates("state template", "settings_del_currently", i18n.TranslationParams{"state": "STATEMARK"})
		if !strings.Contains(currently, "STATEMARK") {
			t.Errorf("%s: settings_del_currently does not carry {state}: %q", code, currently)
		}
		custom := translates("delay template", "settings_del_delay_custom", i18n.TranslationParams{"n": "37"})
		if !strings.Contains(custom, "37") {
			t.Errorf("%s: settings_del_delay_custom does not carry {n}: %q", code, custom)
		}
		on := translates("state template", "settings_state_del_on", i18n.TranslationParams{"n": "3"})
		if !strings.Contains(on, "3") {
			t.Errorf("%s: settings_state_del_on does not carry {n}: %q", code, on)
		}
	}
}
