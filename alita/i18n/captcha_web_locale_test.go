//go:build testtools

package i18n

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/divkix/Alita_Robot/alita/utils/captchaweb"
)

const captchaWebKeyPrefix = "captcha_web_"

// captchaWebLimit is the UI-SPEC length ceiling of one page string, counted in runes:
// English against max, every other language against other.
type captchaWebLimit struct {
	max   int
	other int
}

// captchaWebLimits is the "Max EN" and "Max other" columns of the UI-SPEC page strings
// table. The test fails when it does not cover exactly captchaweb.PageKeys.
var captchaWebLimits = map[string]captchaWebLimit{
	"captcha_web_page_title":        {24, 32},
	"captcha_web_loading_title":     {24, 34},
	"captcha_web_loading_body":      {100, 140},
	"captcha_web_ready_title":       {24, 34},
	"captcha_web_ready_body":        {100, 140},
	"captcha_web_widget_label":      {24, 32},
	"captcha_web_verifying_title":   {24, 34},
	"captcha_web_verifying_body":    {100, 140},
	"captcha_web_passed_title":      {24, 34},
	"captcha_web_passed_body":       {100, 140},
	"captcha_web_failed_title":      {24, 34},
	"captcha_web_failed_body_kick":  {100, 140},
	"captcha_web_failed_body_ban":   {100, 140},
	"captcha_web_failed_body_mute":  {100, 140},
	"captcha_web_expired_title":     {24, 34},
	"captcha_web_expired_body_kick": {100, 140},
	"captcha_web_expired_body_ban":  {100, 140},
	"captcha_web_expired_body_mute": {100, 140},
	"captcha_web_retry_title":       {24, 34},
	"captcha_web_retry_body":        {100, 140},
	"captcha_web_retry_button":      {16, 22},
	"captcha_web_invalid_title":     {24, 34},
	"captcha_web_invalid_body":      {100, 140},
	"captcha_web_unsupported_title": {24, 34},
	"captcha_web_unsupported_body":  {100, 140},
	"captcha_web_close_button":      {16, 22},
}

// captchaWebKickBodies are the only page strings that promise a rejoin.
var captchaWebKickBodies = map[string]struct{}{
	"captcha_web_failed_body_kick":  {},
	"captcha_web_expired_body_kick": {},
}

var captchaWebVerbPattern = regexp.MustCompile(`%[sd]`)

// captchaWebKeys returns every captcha_web_ key of a locale.
func captchaWebKeys(data map[string]any) map[string]struct{} {
	keys := make(map[string]struct{})
	for key := range data {
		if strings.HasPrefix(key, captchaWebKeyPrefix) {
			keys[key] = struct{}{}
		}
	}
	return keys
}

// captchaWebHasEmoji reports a rune in the emoji planes or the dingbat and symbol blocks.
func captchaWebHasEmoji(text string) bool {
	for _, r := range text {
		if r >= 0x1F000 || (r >= 0x2600 && r <= 0x27BF) {
			return true
		}
	}
	return false
}

// TestCaptchaWebLocaleKeys guards every captcha_web_ string across all 7 locales.
// make check-translations only sees literal GetString keys, and the page keys are read
// from captchaweb.PageKeys, so it cannot catch a page key that is missing, empty, too
// long for the layout or carrying markup.
func TestCaptchaWebLocaleKeys(t *testing.T) {
	english := loadStaffLocale(t, "en")
	englishKeys := captchaWebKeys(english)
	if len(englishKeys) == 0 {
		t.Fatal("en.yml has no captcha_web_ keys")
	}

	// The length table and the page's key list must be the same set.
	pageKeys := make(map[string]struct{}, len(captchaweb.PageKeys))
	for _, key := range captchaweb.PageKeys {
		pageKeys[key] = struct{}{}
		if _, ok := captchaWebLimits[key]; !ok {
			t.Errorf("the length table has no entry for page key %s", key)
		}
		if _, ok := englishKeys[key]; !ok {
			t.Errorf("en.yml has no page key %s", key)
		}
	}
	for key := range captchaWebLimits {
		if _, ok := pageKeys[key]; !ok {
			t.Errorf("the length table lists %s, which is not in captchaweb.PageKeys", key)
		}
	}

	var failures []string
	for _, lang := range staffLocales {
		data := loadStaffLocale(t, lang)
		keys := captchaWebKeys(data)

		for key := range englishKeys {
			value, ok := data[key]
			if !ok {
				failures = append(failures, fmt.Sprintf("%s: missing key %s", lang, key))
				continue
			}
			text, isString := value.(string)
			if !isString || strings.TrimSpace(text) == "" {
				failures = append(failures, fmt.Sprintf("%s: empty or non-string value for %s", lang, key))
				continue
			}

			if _, isPage := pageKeys[key]; isPage {
				if strings.ContainsAny(text, "<&%{") {
					failures = append(failures, fmt.Sprintf("%s: page key %s holds markup, a format verb or a placeholder", lang, key))
				}
				if captchaWebHasEmoji(text) {
					failures = append(failures, fmt.Sprintf("%s: page key %s holds an emoji", lang, key))
				}
				limit := captchaWebLimits[key]
				ceiling := limit.other
				if lang == "en" {
					ceiling = limit.max
				}
				if n := utf8.RuneCountInString(strings.TrimSpace(text)); n > ceiling {
					failures = append(failures, fmt.Sprintf("%s: %s is %d characters, over its %d", lang, key, n, ceiling))
				}
				continue
			}

			want := fmt.Sprint(captchaWebVerbPattern.FindAllString(fmt.Sprint(english[key]), -1))
			got := fmt.Sprint(captchaWebVerbPattern.FindAllString(text, -1))
			if want != got {
				failures = append(failures, fmt.Sprintf("%s: format verbs of %s are %s, want %s", lang, key, got, want))
			}
		}
		for key := range keys {
			if _, ok := englishKeys[key]; !ok {
				failures = append(failures, fmt.Sprintf("%s: key %s is not in en.yml", lang, key))
			}
		}
	}

	// Only the two kick bodies promise a rejoin (English only; other languages are a
	// native-speaker review item).
	for key := range englishKeys {
		text, _ := english[key].(string)
		if _, kick := captchaWebKickBodies[key]; kick {
			if !strings.Contains(strings.ToLower(text), "rejoin") {
				failures = append(failures, fmt.Sprintf("en: kick body %s does not say rejoin", key))
			}
			continue
		}
		if _, isPage := pageKeys[key]; isPage && strings.Contains(strings.ToLower(text), "rejoin") {
			failures = append(failures, fmt.Sprintf("en: %s promises a rejoin but is not a kick body", key))
		}
	}

	sort.Strings(failures)
	for _, failure := range failures {
		t.Error(failure)
	}
}
