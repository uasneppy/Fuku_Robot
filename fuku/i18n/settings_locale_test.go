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
	"settings_antiflood_now_on":        {100, 140},
	"settings_btn_antiflood_delete":    {36, 48},
	"settings_card_expired":            {80, 112},
	"settings_staff_no_group":          {120, 170},
	"settings_staff_pick_more":         {120, 170},

	// The settings hub and the owner gate (plan 10-02). The ceilings are the UI-SPEC "New
	// locale keys" columns; the alert and reply rows are held lower than Telegram's 200 so a
	// long translation still fits.
	"settings_page_groups":       {14, 18},
	"settings_page_access":       {14, 18},
	"settings_btn_groups":        {12, 18},
	"settings_main_state_line":   {60, 84},
	"settings_state_access_on":   {20, 28},
	"settings_state_access_off":  {20, 28},
	"settings_list_intro":        {80, 112},
	"settings_list_hint":         {200, 280},
	"settings_list_empty":        {140, 200},
	"settings_access_status":     {50, 70},
	"settings_access_on_desc":    {100, 140},
	"settings_access_off_desc":   {130, 182},
	"settings_btn_access_toggle": {36, 40},
	"settings_access_confirm":    {240, 336},
	"settings_gate_owner_only":   {120, 160},
	"settings_access_owner_only": {120, 160},
	"settings_pick_unavailable":  {120, 160},
	"settings_refuse_owner_only": {70, 98},
	"settings_start_owner_only":  {70, 98},

	// The /help entry (plan 09-13, widened by plan 10-02).
	"settings_help_msg": {1100, 1540},

	// The message editor (plan 11-01). The ceilings are the UI-SPEC "New locale keys" columns.
	"settings_page_welcome":              {12, 18},
	"settings_page_goodbye":              {12, 18},
	"settings_page_rules":                {12, 18},
	"settings_btn_edit_set_text":         {18, 20},
	"settings_state_set":                 {10, 14},
	"settings_state_not_set":             {12, 18},
	"settings_state_default":             {12, 18},
	"settings_edit_line_text":            {40, 56},
	"settings_edit_line_media":           {40, 56},
	"settings_edit_line_buttons":         {40, 56},
	"settings_edit_state_text_len":       {30, 42},
	"settings_edit_state_buttons_n":      {30, 42},
	"settings_edit_media_photo":          {14, 20},
	"settings_edit_media_video":          {14, 20},
	"settings_edit_media_file":           {16, 24},
	"settings_edit_media_sticker":        {14, 20},
	"settings_edit_media_audio":          {14, 20},
	"settings_edit_media_voice":          {14, 20},
	"settings_edit_media_video_note":     {16, 24},
	"settings_edit_note_no_caption":      {90, 126},
	"settings_edit_hint_greeting":        {170, 240},
	"settings_edit_hint_rules":           {140, 196},
	"settings_edit_clean_hint_welcome":   {150, 210},
	"settings_edit_clean_hint_goodbye":   {150, 210},
	"settings_btn_edit_welcome_toggle":   {20, 28},
	"settings_btn_edit_goodbye_toggle":   {20, 28},
	"settings_btn_edit_clean_welcome":    {28, 40},
	"settings_btn_edit_clean_goodbye":    {28, 40},
	"settings_btn_edit_set_media":        {18, 20},
	"settings_btn_edit_set_buttons":      {18, 20},
	"settings_btn_edit_remove_text":      {18, 20},
	"settings_btn_edit_remove_media":     {18, 20},
	"settings_btn_edit_remove_buttons":   {18, 20},
	"settings_btn_edit_preview_text":     {18, 20},
	"settings_btn_edit_preview_media":    {18, 20},
	"settings_btn_edit_preview_buttons":  {18, 20},
	"settings_btn_edit_full_preview":     {18, 20},
	"settings_btn_edit_cancel":           {12, 20},
	"settings_edit_wait_text":            {420, 590},
	"settings_edit_wait_text_rules":      {240, 340},
	"settings_edit_wait_media":           {230, 320},
	"settings_edit_wait_buttons":         {340, 480},
	"settings_edit_wait_footer":          {110, 154},
	"settings_edit_replaced_note":        {80, 112},
	"settings_edit_removed_text_default": {70, 98},
	"settings_edit_removed_text_rules":   {50, 70},
	"settings_edit_removed_media":        {40, 56},
	"settings_edit_removed_buttons":      {40, 56},
	"settings_edit_nothing_to_remove":    {60, 84},
	"settings_edit_cancelled":            {50, 70},
	"settings_edit_cancel_nothing":       {50, 70},
	"settings_edit_group_note":           {110, 154},
	"settings_edit_prompt_closed":        {40, 56},
	"settings_edit_unavailable":          {120, 160},
	"settings_edit_prompt_stale":         {120, 160},
	"settings_edit_wait_expired":         {120, 160},
	"settings_edit_saved_text":           {40, 56},
	"settings_edit_still_waiting":        {70, 98},
	"settings_no_change_info":            {120, 160},
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
	"settings_gate_owner_only",
	"settings_access_owner_only",
	"settings_pick_unavailable",
	"settings_edit_unavailable",
	"settings_edit_prompt_stale",
	"settings_edit_wait_expired",
	"settings_no_change_info",
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

// TestSettingsLocaleKeys guards every settings_ string across all 8 locales. make
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

// TestSettingsLabelLengths holds the composite buttons to the UI-SPEC sizing checks, in
// code points. A two-column grid button (the page's icon, a space and its name) is at most
// 20; the Access toggle rendered with its state (a check or cross icon and the language's
// On or Off word, the longer of the two) is at most 40; the Groups button is at most 18; and a
// choice-row button (a check mark and the option) is at most 13 in English and 15 in the
// others. The Phase 9 ceiling for state inside the Main button no longer applies: state
// moved to the status block of the Main page (UI-SPEC "Changes to Phase 9 rules"). The icons
// are the code constants of the modules, written literally here because fuku/i18n cannot
// import fuku/modules.
func TestSettingsLabelLengths(t *testing.T) {
	const (
		gridMax        = 20
		accessToggle   = 40
		groupsButton   = 18
		choiceMaxOther = 15
		choiceMaxEn    = 13
		// editorSwitchMax is the full-width ceiling of the editor's switch buttons.
		editorSwitchMax = 40
	)
	type gridPage struct {
		icon string
		page string
	}
	pages := []gridPage{
		{"👥", "settings_page_staff"},
		{"🔒", "settings_page_lockdown"},
		{"🧩", "settings_page_captcha"},
		{"🌊", "settings_page_antiflood"},
		{"🔑", "settings_page_access"},
		{"🎉", "settings_page_welcome"},
		{"👋", "settings_page_goodbye"},
		{"📜", "settings_page_rules"},
	}
	options := []string{
		"settings_opt_math", "settings_opt_text", "settings_opt_turnstile",
		"settings_opt_kick", "settings_opt_ban", "settings_opt_mute",
	}
	// The editor's two-column buttons (UI-SPEC rows 28 to 38): at most 20 code points as
	// written, icon included.
	editorButtons := []string{
		"settings_btn_edit_set_text", "settings_btn_edit_set_media", "settings_btn_edit_set_buttons",
		"settings_btn_edit_remove_text", "settings_btn_edit_remove_media", "settings_btn_edit_remove_buttons",
		"settings_btn_edit_preview_text", "settings_btn_edit_preview_media", "settings_btn_edit_preview_buttons",
		"settings_btn_edit_full_preview", "settings_btn_edit_cancel",
	}
	// The editor's four full-width switches (rows 24 to 27): at most 40 rendered with the
	// longer state of the language.
	editorSwitches := []string{
		"settings_btn_edit_welcome_toggle", "settings_btn_edit_goodbye_toggle",
		"settings_btn_edit_clean_welcome", "settings_btn_edit_clean_goodbye",
	}

	for _, lang := range staffLocales {
		t.Run(lang, func(t *testing.T) {
			data := loadStaffLocale(t, lang)
			choiceMax := choiceMaxOther
			if lang == "en" {
				choiceMax = choiceMaxEn
			}

			for _, page := range pages {
				label := page.icon + " " + settingsLocaleString(t, lang, data, page.page)
				if runes := utf8.RuneCountInString(label); runes > gridMax {
					t.Errorf("%s: grid label %q has %d characters, ceiling %d", lang, label, runes, gridMax)
				}
			}

			toggle := settingsLocaleString(t, lang, data, "settings_btn_access_toggle")
			for _, state := range []struct{ icon, key string }{
				{"✅ ", "common_status_on"},
				{"❌ ", "common_status_off"},
			} {
				rendered := strings.ReplaceAll(toggle, "{state}", state.icon+settingsLocaleString(t, lang, data, state.key))
				if runes := utf8.RuneCountInString(rendered); runes > accessToggle {
					t.Errorf("%s: Access toggle %q has %d characters, ceiling %d", lang, rendered, runes, accessToggle)
				}
			}

			groups := settingsLocaleString(t, lang, data, "settings_btn_groups")
			if runes := utf8.RuneCountInString(groups); runes > groupsButton {
				t.Errorf("%s: Groups button %q has %d characters, ceiling %d", lang, groups, runes, groupsButton)
			}

			for _, key := range editorButtons {
				label := settingsLocaleString(t, lang, data, key)
				if runes := utf8.RuneCountInString(label); runes > gridMax {
					t.Errorf("%s: editor button %q (%s) has %d characters, ceiling %d", lang, label, key, runes, gridMax)
				}
			}

			for _, key := range editorSwitches {
				template := settingsLocaleString(t, lang, data, key)
				for _, state := range []struct{ icon, key string }{
					{"✅ ", "common_status_on"},
					{"❌ ", "common_status_off"},
				} {
					rendered := strings.ReplaceAll(template, "{state}", state.icon+settingsLocaleString(t, lang, data, state.key))
					if runes := utf8.RuneCountInString(rendered); runes > editorSwitchMax {
						t.Errorf("%s: editor switch %q (%s) has %d characters, ceiling %d", lang, rendered, key, runes, editorSwitchMax)
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
					// The editor's text prompt lists the literal {id} placeholder of a welcome
					// text as copyable code; it is never filled in (TestSettingsEditLocaleTokens
					// holds it byte-identical), so it cannot name anyone.
					if name == "id" && key == settingsEditWaitTextKey {
						continue
					}
					if name == "staff" || name == "id" {
						t.Errorf("%s: %s holds {%s}, only settings_staff_linked_to may", lang, key, name)
					}
				}
			}
		})
	}
}

// settingsHelpCommands are the commands settings_help_msg must name: /settings itself and
// the commands every menu change is the same as.
var settingsHelpCommands = []string{
	"/settings", "/linkstaff", "/unlinkstaff", "/unlockdown", "/autolockdown", "/captcha",
	"/captchamode", "/captchatime", "/captchaaction", "/setflood", "/setfloodmode", "/delflood",
	"/setwelcome", "/setgoodbye", "/setrules",
}

// TestSettingsHelpFitsOneMessage holds the /help entry of the settings menu to the UI-SPEC
// ceiling (1100 runes in English, 1540 in the others), to the b tag only, to naming every
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

		ceiling := 1540
		if lang == "en" {
			ceiling = 1100
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

// settingsEditWaitTextKey is the Welcome and Goodbye text prompt, the one settings_ string
// that carries the literal {id} placeholder.
const settingsEditWaitTextKey = "settings_edit_wait_text"

// settingsEditLiteralTokens are the welcome-text placeholders and the button syntax the
// editor's prompts show as copyable code. They are never translated, in any language, and
// the translator leaves unknown {name} tokens alone, so each must reach the owner unchanged.
var settingsEditLiteralTokens = []string{
	"<code>{first}</code>", "<code>{last}</code>", "<code>{fullname}</code>", "<code>{username}</code>",
	"<code>{mention}</code>", "<code>{id}</code>", "<code>{count}</code>", "<code>{chatname}</code>",
	"<code>{rules}</code>",
}

const (
	settingsEditButtonExample1 = "<code>[Rules](buttonurl://https://example.com/rules)</code>"
	settingsEditButtonExample2 = "<code>[Chat](buttonurl://https://t.me/example:same)</code>"
)

// TestSettingsEditLocaleTokens holds the literal tokens of the editor's prompts byte-identical
// in every language, and the real parameters ({max}, {time}, {state}) where the English has
// them. A translated placeholder would ship a prompt that tells the owner to type a token the
// bot never replaces.
func TestSettingsEditLocaleTokens(t *testing.T) {
	type tokenKey struct {
		key    string
		tokens []string
	}
	literal := []tokenKey{
		{"settings_edit_wait_text", settingsEditLiteralTokens},
		{"settings_edit_wait_text_rules", []string{"<code>{first}</code>"}},
		{"settings_edit_wait_buttons", []string{
			settingsEditButtonExample1, settingsEditButtonExample2, "<code>:same</code>",
		}},
		{"settings_edit_hint_rules", []string{"<code>/rules</code>"}},
	}

	parameters := []tokenKey{
		{"settings_edit_wait_text", []string{"{max}"}},
		{"settings_edit_wait_text_rules", []string{"{max}"}},
		{"settings_edit_wait_footer", []string{"{time}"}},
		{"settings_edit_state_text_len", []string{"{n}"}},
		{"settings_edit_state_buttons_n", []string{"{n}"}},
		{"settings_edit_line_text", []string{"{state}"}},
		{"settings_edit_line_media", []string{"{state}"}},
		{"settings_edit_line_buttons", []string{"{state}"}},
		{"settings_btn_edit_welcome_toggle", []string{"{state}"}},
		{"settings_btn_edit_goodbye_toggle", []string{"{state}"}},
		{"settings_btn_edit_clean_welcome", []string{"{state}"}},
		{"settings_btn_edit_clean_goodbye", []string{"{state}"}},
	}

	english := loadStaffLocale(t, "en")
	for _, group := range [][]tokenKey{literal, parameters} {
		for _, entry := range group {
			text := settingsLocaleString(t, "en", english, entry.key)
			for _, token := range entry.tokens {
				if !strings.Contains(text, token) {
					t.Fatalf("en: %s does not carry %q, the test table is out of date", entry.key, token)
				}
			}
		}
	}

	for _, lang := range staffLocales {
		t.Run(lang, func(t *testing.T) {
			data := loadStaffLocale(t, lang)
			for _, group := range [][]tokenKey{literal, parameters} {
				for _, entry := range group {
					text := settingsLocaleString(t, lang, data, entry.key)
					for _, token := range entry.tokens {
						if got, want := strings.Count(text, token), strings.Count(settingsLocaleString(t, "en", english, entry.key), token); got != want {
							t.Errorf("%s: %s carries %q %d times, en.yml %d", lang, entry.key, token, got, want)
						}
					}
				}
			}

			// The prompts for the Welcome and Goodbye text name every placeholder once.
			text := settingsLocaleString(t, lang, data, settingsEditWaitTextKey)
			for _, token := range settingsEditLiteralTokens {
				if strings.Count(text, token) != 1 {
					t.Errorf("%s: %s must carry %q exactly once", lang, settingsEditWaitTextKey, token)
				}
			}
		})
	}
}
