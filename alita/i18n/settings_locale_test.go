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

	// Page and card strings (plan 09-05). settings_lockdown_reason is not here on
	// purpose: the reason line reuses lockdown_reason_line (D-29).
	"settings_page_staff":              {14, 18},
	"settings_page_lockdown":           {14, 18},
	"settings_page_antiflood":          {14, 18},
	"settings_state_linked":            {14, 20},
	"settings_state_not_linked":        {14, 20},
	"settings_state_staff_n":           {14, 20},
	"settings_state_lock_none":         {14, 20},
	"settings_state_lock_active":       {14, 20},
	"settings_state_lock_lifting":      {14, 20},
	"settings_state_limit":             {14, 20},
	"settings_state_off_plain":         {10, 14},
	"settings_btn_yes":                 {12, 16},
	"settings_btn_no":                  {12, 16},
	"settings_staff_linked_to":         {60, 84},
	"settings_staff_linked_hidden":     {40, 56},
	"settings_btn_staff_pick_n":        {24, 32},
	"settings_staff_pick_private_hint": {80, 112},
	"settings_staff_not_linked":        {60, 84},
	"settings_staff_link_hint":         {100, 140},
	"settings_staff_readonly":          {110, 154},
	"settings_staff_pick":              {60, 84},
	"settings_btn_link":                {28, 38},
	"settings_btn_unlink":              {28, 38},
	"settings_notice_linked":           {70, 98},
	"settings_notice_unlinked":         {70, 98},
	"settings_lockdown_none":           {80, 112},
	"settings_lockdown_triggers":       {80, 112},
	"settings_trigger_joins":           {70, 98},
	"settings_trigger_flood":           {70, 98},
	"settings_trigger_ai":              {76, 106},
	"settings_btn_trigger_joins":       {24, 32},
	"settings_btn_trigger_flood":       {24, 32},
	"settings_btn_trigger_ai":          {24, 32},
	"settings_lockdown_basic":          {100, 140},
	"settings_lockdown_unreadable":     {100, 140},
	"settings_captcha_status":          {24, 34},
	"settings_captcha_mode":            {40, 56},
	"settings_captcha_action":          {40, 56},
	"settings_captcha_fallback_note":   {110, 154},
	"settings_btn_captcha_toggle":      {24, 32},
	"settings_opt_math":                {10, 12},
	"settings_opt_text":                {10, 12},
	"settings_opt_turnstile":           {10, 12},
	"settings_opt_kick":                {10, 12},
	"settings_opt_ban":                 {10, 12},
	"settings_opt_mute":                {10, 12},
	"settings_opt_warn":                {10, 12},
	"settings_opt_tban":                {14, 18},
	"settings_opt_tmute":               {14, 18},
	"settings_antiflood_limit":         {36, 50},
	"settings_antiflood_off":           {70, 98},
	"settings_antiflood_action":        {36, 50},
	"settings_antiflood_delete":        {40, 56},
	"settings_antiflood_action_note":   {110, 154},
	"settings_btn_antiflood_delete":    {36, 48},
	"settings_card_expired":            {80, 112},
	"settings_staff_no_group":          {120, 170},

	// The /help entry (plan 09-13).
	"settings_help_msg": {900, 1260},
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

// settingsLocaleString reads one string value of a locale and fails the test when it is
// missing or not a string.
func settingsLocaleString(t *testing.T, lang string, data map[string]any, key string) string {
	t.Helper()
	text, ok := data[key].(string)
	if !ok {
		t.Fatalf("%s: %s is missing or not a string", lang, key)
	}
	return text
}

// TestSettingsLabelLengths holds the composite buttons to the UI-SPEC sizing checks: a
// main-page button (icon, page name, separator and its longest state) is at most 34
// characters in English and 44 in the other languages, and a choice-row button (a check
// mark and the option) at most 13 and 15.
func TestSettingsLabelLengths(t *testing.T) {
	type mainPage struct {
		icon   string
		page   string
		states []string
	}
	pages := []mainPage{
		{"👥", "settings_page_staff", []string{"settings_state_linked", "settings_state_not_linked", "settings_state_staff_n"}},
		{"🔒", "settings_page_lockdown", []string{"settings_state_lock_none", "settings_state_lock_active", "settings_state_lock_lifting"}},
		{"🧩", "settings_page_captcha", []string{"common_status_on", "common_status_off"}},
		{"🌊", "settings_page_antiflood", []string{"settings_state_limit", "settings_state_off_plain"}},
	}
	options := []string{
		"settings_opt_math", "settings_opt_text", "settings_opt_turnstile",
		"settings_opt_kick", "settings_opt_ban", "settings_opt_mute",
	}

	for _, lang := range staffLocales {
		t.Run(lang, func(t *testing.T) {
			data := loadStaffLocale(t, lang)
			mainMax, choiceMax := 44, 15
			if lang == "en" {
				mainMax, choiceMax = 34, 13
			}

			for _, page := range pages {
				name := settingsLocaleString(t, lang, data, page.page)
				for _, stateKey := range page.states {
					state := settingsLocaleString(t, lang, data, stateKey)
					state = strings.ReplaceAll(state, "{count}", "100")
					state = strings.ReplaceAll(state, "{n}", "100")
					if strings.HasPrefix(stateKey, "common_status_") {
						icon := "❌ "
						if stateKey == "common_status_on" {
							icon = "✅ "
						}
						state = icon + state
					}
					label := page.icon + " " + name + " · " + state
					if runes := utf8.RuneCountInString(label); runes > mainMax {
						t.Errorf("%s: main-page label %q has %d characters, ceiling %d", lang, label, runes, mainMax)
					}
				}
			}

			for _, key := range options {
				label := "✅ " + settingsLocaleString(t, lang, data, key)
				if runes := utf8.RuneCountInString(label); runes > choiceMax {
					t.Errorf("%s: choice button %q (%s) has %d characters, ceiling %d", lang, label, key, runes, choiceMax)
				}
			}
		})
	}
}

// TestSettingsReusedKeyPlaceholders is the privacy check of D-21: a menu opened in a group
// is readable by every member, so the Staff texts the menu reuses must never carry a
// placeholder that could name the Staff Group. They carry none, or {group} (this group).
// Only settings_staff_linked_to may hold {staff} or {id}, and it is rendered in a private
// menu only.
func TestSettingsReusedKeyPlaceholders(t *testing.T) {
	noPlaceholder := []string{
		"staff_link_no_staff_group",
		"staff_link_refuse_not_staff_owner",
		"staff_check_failed",
		"staff_unlink_not_linked",
		"staff_unlink_refuse_not_owner",
	}
	groupOnly := []string{
		"staff_link_refuse_not_supergroup",
		"staff_link_refuse_target_is_staff",
		"staff_link_refuse_not_target_owner",
		"staff_link_refuse_already_linked",
		"staff_link_check_failed_group",
		"staff_unlink_refuse_not_group_owner",
		"staff_unlink_check_failed_group",
		"staff_unlink_confirm_prompt",
		"lockdown_alert_lift_ask",
	}

	for _, lang := range staffLocales {
		t.Run(lang, func(t *testing.T) {
			data := loadStaffLocale(t, lang)

			for _, key := range noPlaceholder {
				text := settingsLocaleString(t, lang, data, key)
				if got := staffPlaceholders(text); len(got) != 0 {
					t.Errorf("%s: %s has placeholders %v, want none", lang, key, got)
				}
			}
			for _, key := range groupOnly {
				text := settingsLocaleString(t, lang, data, key)
				if got := staffPlaceholders(text); !slices.Equal(got, []string{"group"}) {
					t.Errorf("%s: %s has placeholders %v, want only [group]", lang, key, got)
				}
			}

			for key, value := range data {
				if !strings.HasPrefix(key, settingsKeyPrefix) || key == "settings_staff_linked_to" {
					continue
				}
				text, _ := value.(string)
				for _, name := range staffPlaceholders(text) {
					if name == "staff" || name == "id" {
						t.Errorf("%s: %s holds {%s}, only settings_staff_linked_to may", lang, key, name)
					}
				}
			}
		})
	}
}

// settingsHelpCommands are the commands settings_help_msg must name: /settings itself and
// the eleven commands every menu change is the same as.
var settingsHelpCommands = []string{
	"/settings", "/linkstaff", "/unlinkstaff", "/unlockdown", "/autolockdown", "/captcha",
	"/captchamode", "/captchatime", "/captchaaction", "/setflood", "/setfloodmode", "/delflood",
}

// TestSettingsHelpFitsOneMessage holds the /help entry of the settings menu to the UI-SPEC
// ceiling (900 runes in English, 1260 in the others), to the b tag only, to naming every
// command it stands for, and, with the module help header, to one Telegram message.
func TestSettingsHelpFitsOneMessage(t *testing.T) {
	var failures []string
	for _, lang := range staffLocales {
		data := loadStaffLocale(t, lang)
		help, _ := data["settings_help_msg"].(string)
		header, _ := data["helpers_module_help_header"].(string)
		if strings.TrimSpace(help) == "" {
			failures = append(failures, fmt.Sprintf("%s: settings_help_msg is missing or empty", lang))
			continue
		}

		ceiling := 1260
		if lang == "en" {
			ceiling = 900
		}
		if runes := utf8.RuneCountInString(help); runes > ceiling {
			failures = append(failures, fmt.Sprintf("%s: settings_help_msg has %d characters, ceiling %d", lang, runes, ceiling))
		}

		for _, match := range settingsTagPattern.FindAllStringSubmatch(help, -1) {
			if tag := strings.ToLower(match[1]); tag != "b" {
				failures = append(failures, fmt.Sprintf("%s: settings_help_msg uses the tag <%s>, only b is allowed", lang, tag))
			}
		}

		for _, command := range settingsHelpCommands {
			if !strings.Contains(help, command) {
				failures = append(failures, fmt.Sprintf("%s: settings_help_msg does not name %s", lang, command))
			}
		}

		total := lockdownHelpLength(strings.ReplaceAll(header, "%s", "Settings")) + lockdownHelpLength(help)
		t.Logf("%s: help %d of %d UTF-16 units, headroom %d", lang, total, lockdownHelpTelegramLimit, lockdownHelpTelegramLimit-total)
		if total > lockdownHelpTelegramLimit {
			failures = append(failures, fmt.Sprintf("%s: help header and settings_help_msg are %d characters, over Telegram's %d", lang, total, lockdownHelpTelegramLimit))
		}
	}
	sort.Strings(failures)
	for _, failure := range failures {
		t.Error(failure)
	}
}
