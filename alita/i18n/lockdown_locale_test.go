//go:build testtools

package i18n

import (
	"fmt"
	"sort"
	"strings"
	"testing"
)

const lockdownKeyPrefix = "lockdown_"

// lockdownExtraKeys are the lockdown strings that live under another module's prefix:
// the greetings Accept button alert and the /unmute reply note.
var lockdownExtraKeys = []string{
	"greetings_join_request_lockdown",
	"mutes_unmute_lockdown_note",
}

// lockdownKeys returns every lockdown_ key of a locale plus the two extra keys the
// locale has.
func lockdownKeys(data map[string]any) map[string]struct{} {
	keys := make(map[string]struct{})
	for key := range data {
		if strings.HasPrefix(key, lockdownKeyPrefix) {
			keys[key] = struct{}{}
		}
	}
	for _, key := range lockdownExtraKeys {
		if _, ok := data[key]; ok {
			keys[key] = struct{}{}
		}
	}
	return keys
}

// TestLockdownLocaleKeys guards every lockdown string across all 7 locales: parity
// with en.yml in both directions, non-empty values, and identical placeholder sets.
// make check-translations only sees literal GetString keys, so it cannot catch a
// lockdown_ key that is missing, empty or has a renamed placeholder.
func TestLockdownLocaleKeys(t *testing.T) {
	english := loadStaffLocale(t, "en")
	englishKeys := lockdownKeys(english)
	for _, key := range lockdownExtraKeys {
		if _, ok := englishKeys[key]; !ok {
			t.Fatalf("en.yml has no %s key", key)
		}
	}
	if len(englishKeys) <= len(lockdownExtraKeys) {
		t.Fatal("en.yml has no lockdown_ keys")
	}

	var failures []string
	for _, lang := range staffLocales {
		data := loadStaffLocale(t, lang)
		keys := lockdownKeys(data)

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
			want := fmt.Sprint(staffPlaceholders(fmt.Sprint(english[key])))
			got := fmt.Sprint(staffPlaceholders(text))
			if want != got {
				failures = append(failures, fmt.Sprintf("%s: placeholders of %s are %s, want %s", lang, key, got, want))
			}
		}
		for key := range keys {
			if _, ok := englishKeys[key]; !ok {
				failures = append(failures, fmt.Sprintf("%s: key %s is not in en.yml", lang, key))
			}
		}
	}

	sort.Strings(failures)
	for _, failure := range failures {
		t.Error(failure)
	}
}

// lockdownWarningMarker is U+26A0 U+FE0F, the mark lockdown_status_manual_change
// already starts with.
const lockdownWarningMarker = "\u26a0\ufe0f"

// lockdownWarningKeys are the lines that report something the bot failed to do. A
// refusal where nothing changed carries no marker.
var lockdownWarningKeys = []string{
	"lockdown_restore_failed",
	"lockdown_lock_unknown",
	"lockdown_lift_record_failed",
	"lockdown_lift_tally_failed",
	"lockdown_status_ban_failed",
}

// TestLockdownWarningLines checks that every failure line starts with the warning
// marker in all 7 locales, so a failure reads differently from a routine line.
func TestLockdownWarningLines(t *testing.T) {
	var failures []string
	for _, lang := range staffLocales {
		data := loadStaffLocale(t, lang)
		for _, key := range lockdownWarningKeys {
			text, _ := data[key].(string)
			if !strings.HasPrefix(text, lockdownWarningMarker) {
				failures = append(failures, fmt.Sprintf("%s: %s does not start with the warning marker", lang, key))
			}
		}
	}
	sort.Strings(failures)
	for _, failure := range failures {
		t.Error(failure)
	}
}

// TestLockdownRepliesCarryNoTelegramDetail checks that no lockdown string has a
// {detail} slot in any locale: Telegram's own error text goes to the log, never to
// the group.
func TestLockdownRepliesCarryNoTelegramDetail(t *testing.T) {
	var failures []string
	for _, lang := range staffLocales {
		data := loadStaffLocale(t, lang)
		for key := range lockdownKeys(data) {
			text, _ := data[key].(string)
			for _, name := range staffPlaceholders(text) {
				if name == "detail" {
					failures = append(failures, fmt.Sprintf("%s: %s has a {detail} placeholder", lang, key))
				}
			}
		}
	}
	sort.Strings(failures)
	for _, failure := range failures {
		t.Error(failure)
	}
}
