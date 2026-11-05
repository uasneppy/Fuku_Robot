//go:build testtools

package i18n

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
)

// wantLocaleCount is the number of interface languages the bot ships.
const wantLocaleCount = 8

// localeSetDiff compares the locale codes found on disk with the codes the
// guard tests enumerate, as sets (order never matters). It returns an empty
// string when they are equal and have exactly wantLocaleCount members, else a
// message naming every missing and every extra code.
func localeSetDiff(onDisk, listed []string) string {
	disk := make(map[string]struct{}, len(onDisk))
	for _, code := range onDisk {
		disk[code] = struct{}{}
	}
	want := make(map[string]struct{}, len(listed))
	for _, code := range listed {
		want[code] = struct{}{}
	}

	var problems []string
	var missing, extra []string
	for code := range want {
		if _, ok := disk[code]; !ok {
			missing = append(missing, code)
		}
	}
	for code := range disk {
		if _, ok := want[code]; !ok {
			extra = append(extra, code)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	if len(missing) > 0 {
		problems = append(problems, fmt.Sprintf("listed in staffLocales but no locales/<code>.yml file: %s", strings.Join(missing, ", ")))
	}
	if len(extra) > 0 {
		problems = append(problems, fmt.Sprintf("locales/<code>.yml file not listed in staffLocales: %s", strings.Join(extra, ", ")))
	}
	if len(disk) != wantLocaleCount {
		problems = append(problems, fmt.Sprintf("%d locale files on disk, want exactly %d", len(disk), wantLocaleCount))
	}
	if len(want) != wantLocaleCount {
		problems = append(problems, fmt.Sprintf("%d codes in staffLocales, want exactly %d", len(want), wantLocaleCount))
	}
	return strings.Join(problems, "; ")
}

// TestLocalesAreTheEight proves the locale set is exactly the 8 language files
// (config.yml is the help alt-name pseudo-locale, not a language) and that the
// list every locale guard test walks is that same set, so a new string can
// never ship missing from an unlisted file.
func TestLocalesAreTheEight(t *testing.T) {
	entries, err := os.ReadDir("../../locales")
	if err != nil {
		t.Fatalf("read locales directory: %v", err)
	}
	var onDisk []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".yml") || name == "config.yml" {
			continue
		}
		onDisk = append(onDisk, strings.TrimSuffix(name, ".yml"))
	}

	if diff := localeSetDiff(onDisk, staffLocales); diff != "" {
		t.Fatal(diff)
	}

	t.Run("detects a missing and an extra code", func(t *testing.T) {
		diff := localeSetDiff(
			[]string{"ru", "pt", "id", "hi", "fr", "es", "en", "de"},
			staffLocales,
		)
		if !strings.Contains(diff, "uk") {
			t.Errorf("failure does not name the missing code uk: %q", diff)
		}
		if !strings.Contains(diff, "de") {
			t.Errorf("failure does not name the extra code de: %q", diff)
		}
	})

	t.Run("order never matters", func(t *testing.T) {
		reversed := make([]string, len(staffLocales))
		for i, code := range staffLocales {
			reversed[len(staffLocales)-1-i] = code
		}
		if diff := localeSetDiff(reversed, staffLocales); diff != "" {
			t.Errorf("reordered set rejected: %s", diff)
		}
	})

	t.Run("a ninth code fails the count", func(t *testing.T) {
		nine := append(append([]string{}, staffLocales...), "de")
		if diff := localeSetDiff(nine, nine); !strings.Contains(diff, "want exactly 8") {
			t.Errorf("nine locales accepted: %q", diff)
		}
	})
}
