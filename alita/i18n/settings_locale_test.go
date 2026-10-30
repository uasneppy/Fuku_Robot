//go:build testtools

package i18n

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
	"unicode/utf8"
)

const settingsKeyPrefix = "settings_"

// settingsLimit is the UI-SPEC length ceiling of one settings string, counted in runes of
// the template: English against max, every other language against other.
type settingsLimit struct {
	max   int
	other int
}

// settingsLocaleLimits is the "Max EN" and "Max other" columns of the UI-SPEC "New locale
// keys" table, for the keys that exist so far. The test fails when it does not cover
// exactly the settings_ keys of the locale files, so a plan that adds a key adds its row.
var settingsLocaleLimits = map[string]settingsLimit{
	"settings_header_main":       {24, 34},
	"settings_header":            {36, 50},
	"settings_page_captcha":      {14, 18},
	"settings_choose":            {90, 126},
	"settings_btn_open_here":     {20, 28},
	"settings_btn_open_private":  {24, 32},
	"settings_main_hint":         {100, 140},
	"settings_main_owner":        {48, 68},
	"settings_btn_back":          {12, 16},
	"settings_btn_close":         {12, 16},
	"settings_captcha_time":      {36, 50},
	"settings_btn_minutes":       {10, 12},
	"settings_not_your_menu":     {110, 160},
	"settings_not_admin":         {90, 130},
	"settings_no_restrict":       {110, 160},
	"settings_check_failed":      {110, 160},
	"settings_min_value":         {50, 70},
	"settings_max_value":         {50, 70},
	"settings_cannot_open_alert": {60, 84},
	"settings_cannot_open":       {140, 196},
	"settings_closed":            {24, 34},
	"settings_start_not_admin":   {90, 126},
	"settings_start_invalid":     {110, 154},
}

// settingsAlertKeys are shown as answerCallbackQuery alerts: plain text, no tag, at most
// 200 characters in every language (Telegram's cap).
var settingsAlertKeys = []string{
	"settings_not_your_menu",
	"settings_not_admin",
	"settings_no_restrict",
	"settings_check_failed",
	"settings_min_value",
	"settings_max_value",
	"settings_cannot_open_alert",
}

// settingsAlertMaxRunes is the length Telegram allows an alert.
const settingsAlertMaxRunes = 200

var (
	settingsTagPattern  = regexp.MustCompile(`</?([A-Za-z][A-Za-z0-9]*)[^>]*>`)
	settingsVerbPattern = regexp.MustCompile(`%[a-zA-Z]`)
)

// settingsKeys returns every settings_ key of a locale.
func settingsKeys(data map[string]any) map[string]struct{} {
	keys := make(map[string]struct{})
	for key := range data {
		if strings.HasPrefix(key, settingsKeyPrefix) {
			keys[key] = struct{}{}
		}
	}
	return keys
}

// TestSettingsLocaleKeys guards every settings_ string across all 7 locales. make
// check-translations only sees keys that are literal GetString arguments, so it cannot
// catch a key that is empty, too long for the layout, carrying markup it should not or a
// renamed placeholder.
func TestSettingsLocaleKeys(t *testing.T) {
	english := loadStaffLocale(t, "en")
	englishKeys := settingsKeys(english)
	if len(englishKeys) == 0 {
		t.Fatal("en.yml has no settings_ keys")
	}

	for _, lang := range staffLocales {
		t.Run(lang, func(t *testing.T) {
			data := loadStaffLocale(t, lang)
			keys := settingsKeys(data)

			var failures []string
			for key := range keys {
				if _, ok := settingsLocaleLimits[key]; !ok {
					failures = append(failures, fmt.Sprintf("%s: the limits table has no row for %s", lang, key))
				}
			}
			for key := range settingsLocaleLimits {
				if _, ok := keys[key]; !ok {
					failures = append(failures, fmt.Sprintf("%s: missing key %s", lang, key))
				}
			}

			for key, limit := range settingsLocaleLimits {
				value, ok := data[key]
				if !ok {
					continue
				}
				text, isString := value.(string)
				if !isString || strings.TrimSpace(text) == "" {
					failures = append(failures, fmt.Sprintf("%s: empty or non-string value for %s", lang, key))
					continue
				}

				ceiling := limit.other
				if lang == "en" {
					ceiling = limit.max
				}
				if runes := utf8.RuneCountInString(text); runes > ceiling {
					failures = append(failures, fmt.Sprintf("%s: %s has %d characters, ceiling %d", lang, key, runes, ceiling))
				}

				englishText, _ := english[key].(string)
				want := staffPlaceholders(englishText)
				if got := staffPlaceholders(text); !slices.Equal(got, want) {
					failures = append(failures, fmt.Sprintf("%s: %s has placeholders %v, en.yml has %v", lang, key, got, want))
				}

				if settingsVerbPattern.MatchString(text) {
					failures = append(failures, fmt.Sprintf("%s: %s holds a printf verb", lang, key))
				}

				open := map[string]int{}
				for _, match := range settingsTagPattern.FindAllStringSubmatch(text, -1) {
					tag := strings.ToLower(match[1])
					if tag != "b" && tag != "code" {
						failures = append(failures, fmt.Sprintf("%s: %s uses the tag <%s>, only b and code are allowed", lang, key, tag))
						continue
					}
					if strings.HasPrefix(match[0], "</") {
						open[tag]--
					} else {
						open[tag]++
					}
				}
				for tag, balance := range open {
					if balance != 0 {
						failures = append(failures, fmt.Sprintf("%s: %s has unbalanced <%s> tags", lang, key, tag))
					}
				}
			}

			for _, key := range settingsAlertKeys {
				text, _ := data[key].(string)
				if strings.ContainsAny(text, "<>") {
					failures = append(failures, fmt.Sprintf("%s: alert key %s holds markup", lang, key))
				}
				if runes := utf8.RuneCountInString(text); runes > settingsAlertMaxRunes {
					failures = append(failures, fmt.Sprintf("%s: alert key %s has %d characters, Telegram allows %d", lang, key, runes, settingsAlertMaxRunes))
				}
			}

			sort.Strings(failures)
			for _, failure := range failures {
				t.Error(failure)
			}
		})
	}
}
