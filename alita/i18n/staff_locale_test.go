//go:build testtools

package i18n

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const staffKeyPrefix = "staff_"

var staffLocales = []string{"en", "es", "fr", "hi", "id", "pt", "ru"}

// staffPlaceholderPattern matches {name} tokens exactly and case-sensitively.
var staffPlaceholderPattern = regexp.MustCompile(`\{(\w+)\}`)

func loadStaffLocale(t *testing.T, lang string) map[string]any {
	t.Helper()
	path := fmt.Sprintf("../../locales/%s.yml", lang)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var data map[string]any
	if err := yaml.Unmarshal(raw, &data); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	if len(data) == 0 {
		t.Fatalf("%s parsed to an empty map", path)
	}
	return data
}

func staffKeys(data map[string]any) map[string]struct{} {
	keys := make(map[string]struct{})
	for key := range data {
		if strings.HasPrefix(key, staffKeyPrefix) {
			keys[key] = struct{}{}
		}
	}
	return keys
}

// staffPlaceholders returns the sorted, de-duplicated {name} tokens of text;
// order and repetition do not matter, only the set does.
func staffPlaceholders(text string) []string {
	seen := make(map[string]struct{})
	for _, match := range staffPlaceholderPattern.FindAllStringSubmatch(text, -1) {
		seen[match[1]] = struct{}{}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// TestStaffLocaleKeys guards every staff_ key across all 7 locales: parity with
// en.yml in both directions, non-empty values, and identical placeholder sets.
// make check-translations only sees literal GetString keys, so it cannot catch
// a staff_ key that is missing, empty or has a renamed placeholder.
func TestStaffLocaleKeys(t *testing.T) {
	english := loadStaffLocale(t, "en")
	englishKeys := staffKeys(english)
	if len(englishKeys) == 0 {
		t.Fatal("en.yml has no staff_ keys")
	}

	var failures []string
	for _, lang := range staffLocales {
		data := loadStaffLocale(t, lang)
		keys := staffKeys(data)

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
