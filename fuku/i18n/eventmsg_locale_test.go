//go:build testtools

package i18n

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"
)

const eventMsgKeyPrefix = "eventmsg_"

// eventMsgLimit is the length ceiling of one eventmsg_ template in code points as written
// (placeholders and markup included): English against max, every other language against other.
type eventMsgLimit struct {
	max   int
	other int
}

// eventMsgLocaleLimits has one row per eventmsg_ key of the locale files. The test fails when it does
// not cover exactly the eventmsg_ keys of every locale file, so a plan that adds a key adds its row.
var eventMsgLocaleLimits = map[string]eventMsgLimit{
	"eventmsg_admin_anon":         {20, 30},
	"eventmsg_pm_ban":             {140, 200},
	"eventmsg_pm_tban":            {140, 200},
	"eventmsg_pm_kick":            {140, 200},
	"eventmsg_pm_mute":            {140, 200},
	"eventmsg_pm_tmute":           {140, 200},
	"eventmsg_pm_unban":           {140, 200},
	"eventmsg_pm_unmute":          {140, 200},
	"eventmsg_pm_reason":          {40, 60},
	"eventmsg_pm_unmute_lockdown": {140, 200},
}

// eventMsgAllowedTags is the markup an eventmsg_ text may carry: <b> only, around {group} in the
// default private texts and around the label of the reason line.
var eventMsgAllowedTags = []string{"<b>", "</b>"}

func TestEventMsgLocaleKeys(t *testing.T) {
	english := loadStaffLocale(t, "en")

	for _, lang := range staffLocales {
		t.Run(lang, func(t *testing.T) {
			data := loadStaffLocale(t, lang)

			var failures []string
			for key := range data {
				if !strings.HasPrefix(key, eventMsgKeyPrefix) {
					continue
				}
				if _, ok := eventMsgLocaleLimits[key]; !ok {
					failures = append(failures, fmt.Sprintf("%s: the limits table has no row for %s", lang, key))
				}
			}

			for key, limit := range eventMsgLocaleLimits {
				text, isString := data[key].(string)
				if !isString || strings.TrimSpace(text) == "" {
					failures = append(failures, fmt.Sprintf("%s: missing, empty or non-string %s", lang, key))
					continue
				}
				ceiling := limit.other
				if lang == "en" {
					ceiling = limit.max
				}
				if runes := utf8.RuneCountInString(text); runes > ceiling {
					failures = append(failures, fmt.Sprintf("%s: %s has %d characters, ceiling %d", lang, key, runes, ceiling))
				}
				stripped := text
				for _, tag := range eventMsgAllowedTags {
					stripped = strings.ReplaceAll(stripped, tag, "")
				}
				if strings.ContainsAny(stripped, "<>&") {
					failures = append(failures, fmt.Sprintf("%s: %s holds markup other than <b> or an ampersand", lang, key))
				}
				if strings.Count(text, "<b>") != strings.Count(text, "</b>") {
					failures = append(failures, fmt.Sprintf("%s: %s has an unclosed <b>", lang, key))
				}
				if settingsVerbPattern.MatchString(text) {
					failures = append(failures, fmt.Sprintf("%s: %s holds a printf verb", lang, key))
				}
				englishText, _ := english[key].(string)
				if got, want := staffPlaceholders(text), staffPlaceholders(englishText); !slices.Equal(got, want) {
					failures = append(failures, fmt.Sprintf("%s: %s has placeholders %v, en.yml has %v", lang, key, got, want))
				}
				if strings.HasPrefix(key, "eventmsg_pm_") && key != "eventmsg_pm_reason" && key != "eventmsg_pm_unmute_lockdown" &&
					!strings.Contains(text, "<b>{group}</b>") {
					failures = append(failures, fmt.Sprintf("%s: %s does not name the group as <b>{group}</b>", lang, key))
				}
			}

			slices.Sort(failures)
			for _, failure := range failures {
				t.Error(failure)
			}
		})
	}
}
