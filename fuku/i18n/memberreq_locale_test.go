//go:build testtools

package i18n

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"testing"
	"unicode/utf8"
)

const memberReqKeyPrefix = "mreq_"

// memberReqLimit is the UI-SPEC length ceiling of one mreq_ template, counted in code
// points as written (placeholders included): English against max, every other language
// against other.
type memberReqLimit struct {
	max   int
	other int
}

// memberReqLocaleLimits is the "Max EN" and "Max other" columns of the UI-SPEC "New mreq_
// keys" table (rows 1 to 29). The test fails when it does not cover exactly the mreq_ keys
// of every locale file, so a plan that adds a key adds its row.
var memberReqLocaleLimits = map[string]memberReqLimit{
	"mreq_rule_lastname":           {18, 24},
	"mreq_rule_username":           {18, 24},
	"mreq_rule_russian":            {20, 27},
	"mreq_rule_arabic":             {20, 27},
	"mreq_rule_chinese":            {20, 27},
	"mreq_rule_photo":              {26, 32},
	"mreq_rule_channel":            {30, 38},
	"mreq_name_unknown":            {16, 24},
	"mreq_notice_warn":             {110, 154},
	"mreq_notice_warn_limit_kick":  {120, 168},
	"mreq_notice_warn_limit_ban":   {120, 168},
	"mreq_notice_warn_limit_mute":  {140, 170},
	"mreq_notice_kick":             {100, 140},
	"mreq_notice_ban":              {80, 112},
	"mreq_notice_mute":             {110, 154},
	"mreq_btn_fixed":               {20, 28},
	"mreq_notice_fixed":            {70, 98},
	"mreq_notice_fixed_lockdown":   {130, 182},
	"mreq_fixed_done":              {50, 70},
	"mreq_fixed_done_lockdown":     {110, 160},
	"mreq_remedy_photo":            {62, 84},
	"mreq_fixed_still_broken":      {60, 70},
	"mreq_fixed_still_broken_many": {80, 112},
	"mreq_fixed_not_yours":         {80, 112},
	"mreq_fixed_not_ours":          {120, 160},
	"mreq_fixed_check_failed":      {90, 126},
	"mreq_fixed_stale":             {70, 98},
	"mreq_warn_reason":             {40, 56},
	"mreq_notice_paused":           {190, 266},
}

// memberReqRulePhrases are the seven rule phrases that fill {rules}, in rule order.
var memberReqRulePhrases = []string{
	"mreq_rule_lastname", "mreq_rule_username", "mreq_rule_russian", "mreq_rule_arabic", "mreq_rule_chinese",
	"mreq_rule_photo", "mreq_rule_channel",
}

// memberReqRemedyNoticeKeys are the notices that carry the photo remedy sentence after one
// space when the photo rule is among the broken ones.
var memberReqRemedyNoticeKeys = []string{"mreq_notice_warn", "mreq_notice_kick", "mreq_notice_mute"}

// memberReqStillBrokenPhrases is how many phrases the still-broken alert lists before it
// names the count instead (five or more rules use mreq_fixed_still_broken_many).
const memberReqStillBrokenPhrases = 4

// memberReqAnswerKeys are shown as answerCallbackQuery text (the toast and the alerts):
// plain text, at most 160 code points as templates (UI-SPEC "Rules for this family").
var memberReqAnswerKeys = []string{
	"mreq_fixed_done", "mreq_fixed_done_lockdown", "mreq_fixed_still_broken", "mreq_fixed_not_yours",
	"mreq_fixed_not_ours", "mreq_fixed_check_failed", "mreq_fixed_stale", "mreq_fixed_still_broken_many",
}

// memberReqNoticeKeys are the group notices and their edits: one paragraph each, at most
// 520 code points of visible text once rendered with the worst case name (amendment M10).
var memberReqNoticeKeys = []string{
	"mreq_notice_warn", "mreq_notice_warn_limit_kick", "mreq_notice_warn_limit_ban",
	"mreq_notice_warn_limit_mute", "mreq_notice_kick", "mreq_notice_ban", "mreq_notice_mute",
	"mreq_notice_fixed", "mreq_notice_fixed_lockdown", "mreq_notice_paused",
}

const (
	memberReqAnswerMax        = 160
	memberReqStillBrokenEn    = 160
	memberReqStillBrokenOther = 200
	memberReqNoticeMax        = 520
	memberReqFixedPrefix      = "✅ "
	// memberReqNameRunes is the longest name a notice carries: 64 runes plus the two
	// Unicode isolates U+2068 and U+2069 the mention adds around it.
	memberReqNameRunes = 66
)

// memberReqKeys returns every mreq_ key of a locale.
func memberReqKeys(data map[string]any) map[string]struct{} {
	keys := make(map[string]struct{})
	for key := range data {
		if strings.HasPrefix(key, memberReqKeyPrefix) {
			keys[key] = struct{}{}
		}
	}
	return keys
}

// memberReqRender splices the worst case values into a template the way the bot does after
// translation: a 66 code point name, two digit numbers and the seven rule phrases of the
// language joined by ", ". A non-empty tail is appended after one space before the splice,
// as memberReqNoticeText does for the photo remedy.
func memberReqRender(t *testing.T, lang string, data map[string]any, key, tail string) string {
	t.Helper()
	phrases := make([]string, 0, len(memberReqRulePhrases))
	for _, phrase := range memberReqRulePhrases {
		phrases = append(phrases, settingsLocaleString(t, lang, data, phrase))
	}
	return memberReqSplice(settingsLocaleString(t, lang, data, key), tail, phrases)
}

// memberReqSplice fills the placeholders of one template with the worst case values and the
// given phrases.
func memberReqSplice(template, tail string, phrases []string) string {
	if tail != "" {
		template += " " + tail
	}
	replacer := strings.NewReplacer(
		"{name}", strings.Repeat("W", memberReqNameRunes),
		"{rules}", strings.Join(phrases, ", "),
		"{n}", "99",
		"{limit}", "99",
		"{minutes}", "99",
		"{id}", "9007199254740991",
	)
	return replacer.Replace(template)
}

// memberReqLongestPhrases returns the count longest rule phrases of a language.
func memberReqLongestPhrases(t *testing.T, lang string, data map[string]any, count int) []string {
	t.Helper()
	phrases := make([]string, 0, len(memberReqRulePhrases))
	for _, phrase := range memberReqRulePhrases {
		phrases = append(phrases, settingsLocaleString(t, lang, data, phrase))
	}
	sort.SliceStable(phrases, func(i, j int) bool {
		return utf8.RuneCountInString(phrases[i]) > utf8.RuneCountInString(phrases[j])
	})
	return phrases[:count]
}

// TestMemberReqLocaleKeys guards every mreq_ string across all 8 locales. make
// check-translations only sees keys that are literal GetString arguments, and these keys
// have no Go reader until the notice and button plans, so it cannot catch a key that is
// missing, empty, over its ceiling, carrying markup or a renamed placeholder. The rendered
// checks are the binding ones: the ceilings only make them satisfiable.
func TestMemberReqLocaleKeys(t *testing.T) {
	english := loadStaffLocale(t, "en")

	for _, lang := range staffLocales {
		t.Run(lang, func(t *testing.T) {
			data := loadStaffLocale(t, lang)
			keys := memberReqKeys(data)

			var failures []string
			for key := range keys {
				if _, ok := memberReqLocaleLimits[key]; !ok {
					failures = append(failures, fmt.Sprintf("%s: the limits table has no row for %s", lang, key))
				}
			}
			for key := range memberReqLocaleLimits {
				if _, ok := keys[key]; !ok {
					failures = append(failures, fmt.Sprintf("%s: missing key %s", lang, key))
				}
			}

			for key, limit := range memberReqLocaleLimits {
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

				if strings.ContainsAny(text, "<>&") {
					failures = append(failures, fmt.Sprintf("%s: %s holds markup or an ampersand", lang, key))
				}
				if settingsVerbPattern.MatchString(text) {
					failures = append(failures, fmt.Sprintf("%s: %s holds a printf verb", lang, key))
				}

				englishText, _ := english[key].(string)
				want := staffPlaceholders(englishText)
				if got := staffPlaceholders(text); !slices.Equal(got, want) {
					failures = append(failures, fmt.Sprintf("%s: %s has placeholders %v, en.yml has %v", lang, key, got, want))
				}
			}

			if _, ok := keys["mreq_btn_fixed"]; ok {
				if label, _ := data["mreq_btn_fixed"].(string); !strings.HasPrefix(label, memberReqFixedPrefix) {
					failures = append(failures, fmt.Sprintf("%s: mreq_btn_fixed %q does not start with %q", lang, label, memberReqFixedPrefix))
				}
			}

			for _, key := range memberReqAnswerKeys {
				text, _ := data[key].(string)
				if runes := utf8.RuneCountInString(text); runes > memberReqAnswerMax {
					failures = append(failures, fmt.Sprintf("%s: answer key %s has %d characters, ceiling %d", lang, key, runes, memberReqAnswerMax))
				}
			}

			failures = append(failures, memberReqRenderedFailures(t, lang, data)...)
			failures = append(failures, memberReqCrossFamilyFailures(lang, data)...)

			sort.Strings(failures)
			for _, failure := range failures {
				t.Error(failure)
			}
		})
	}
}

// memberReqRenderedFailures runs the binding rendered-length checks of one language: the
// still-broken alert with the four longest phrases, the count answer at n 99, and every
// notice with a 66 code point name and all seven phrases, the three remedy notices also
// with the photo remedy after one space.
func memberReqRenderedFailures(t *testing.T, lang string, data map[string]any) []string {
	t.Helper()
	required := append([]string{"mreq_fixed_still_broken", "mreq_fixed_still_broken_many", "mreq_remedy_photo"}, memberReqNoticeKeys...)
	required = append(required, memberReqRulePhrases...)
	for _, key := range required {
		if _, ok := data[key].(string); !ok {
			// A missing key is already reported by the coverage check.
			return nil
		}
	}

	var failures []string
	stillBrokenMax := memberReqStillBrokenOther
	if lang == "en" {
		stillBrokenMax = memberReqStillBrokenEn
	}
	longest := memberReqLongestPhrases(t, lang, data, memberReqStillBrokenPhrases)
	stillBroken := memberReqSplice(settingsLocaleString(t, lang, data, "mreq_fixed_still_broken"), "", longest)
	if runes := utf8.RuneCountInString(stillBroken); runes > stillBrokenMax {
		failures = append(failures, fmt.Sprintf("%s: mreq_fixed_still_broken rendered with the four longest phrases has %d characters, ceiling %d", lang, runes, stillBrokenMax))
	}
	many := memberReqSplice(settingsLocaleString(t, lang, data, "mreq_fixed_still_broken_many"), "", nil)
	if runes := utf8.RuneCountInString(many); runes > memberReqAnswerMax {
		failures = append(failures, fmt.Sprintf("%s: mreq_fixed_still_broken_many rendered at n 99 has %d characters, ceiling %d", lang, runes, memberReqAnswerMax))
	}
	remedy := settingsLocaleString(t, lang, data, "mreq_remedy_photo")
	for _, key := range memberReqNoticeKeys {
		if runes := utf8.RuneCountInString(memberReqRender(t, lang, data, key, "")); runes > memberReqNoticeMax {
			failures = append(failures, fmt.Sprintf("%s: %s rendered with the worst case has %d characters, ceiling %d", lang, key, runes, memberReqNoticeMax))
		}
	}
	for _, key := range memberReqRemedyNoticeKeys {
		if runes := utf8.RuneCountInString(memberReqRender(t, lang, data, key, remedy)); runes > memberReqNoticeMax {
			failures = append(failures, fmt.Sprintf("%s: %s rendered with the worst case and the photo remedy has %d characters, ceiling %d", lang, key, runes, memberReqNoticeMax))
		}
	}
	return failures
}

// memberReqQuotingKeys are the texts that name the "I fixed it" button: in every language
// the words of mreq_btn_fixed (without its check mark prefix) must appear verbatim in each,
// so the text never names a button the member cannot find (UI-SPEC check 11).
var memberReqQuotingKeys = []string{
	"mreq_notice_mute", "settings_mreq_actions_hint", "settings_mreq_hint_mute", "settings_mreq_confirm_mute",
}

// memberReqLetterKeys are the texts that show the four Russian letters literally, in every
// language (MREQ-13).
var memberReqLetterKeys = []string{"settings_mreq_hint_letters", "settings_mreq_desc_russian"}

// memberReqCrossFamilyFailures reads the mreq_ and settings_mreq_ families together: the
// quote consistency of the button name and the literal letters of the Russian rule.
func memberReqCrossFamilyFailures(lang string, data map[string]any) []string {
	var failures []string

	label, _ := data["mreq_btn_fixed"].(string)
	words := strings.TrimSpace(strings.TrimPrefix(label, memberReqFixedPrefix))
	if words == "" {
		failures = append(failures, fmt.Sprintf("%s: mreq_btn_fixed has no words after its prefix", lang))
	} else {
		for _, key := range memberReqQuotingKeys {
			text, _ := data[key].(string)
			if !strings.Contains(text, words) {
				failures = append(failures, fmt.Sprintf("%s: %s does not contain the button words %q verbatim", lang, key, words))
			}
		}
	}

	for _, key := range memberReqLetterKeys {
		text, _ := data[key].(string)
		for _, letter := range []string{"ы", "ъ", "э", "ё"} {
			if !strings.Contains(text, letter) {
				failures = append(failures, fmt.Sprintf("%s: %s does not contain the letter %s literally", lang, key, letter))
			}
		}
	}
	return failures
}
