//go:build testtools

package modules

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// settingsLocksPagerCode is the action code of the Locks page's pager, which a control code must
// never equal.
const settingsLocksPagerCode = "kg"

// TestSettingsLocksTableMatchesCommand holds settingsLockRows equal to what /lock and /unlock
// accept: a lock added to lockMap or restrMap without a row fails here, and so does a row that
// names no lock, so the page and the commands can never split.
func TestSettingsLocksTableMatchesCommand(t *testing.T) {
	commandKinds := locksModule.getLockMapAsArray()

	tableKinds := make([]string, 0, len(settingsLockRows))
	for _, row := range settingsLockRows {
		tableKinds = append(tableKinds, row.Kind)
	}
	for _, kind := range commandKinds {
		if !slices.Contains(tableKinds, kind) {
			t.Errorf("the command accepts %q and the table has no row for it", kind)
		}
	}
	for _, kind := range tableKinds {
		if !slices.Contains(commandKinds, kind) {
			t.Errorf("the table names %q, which /lock does not accept", kind)
		}
	}
	if len(settingsLockRows) != 22 {
		t.Errorf("table has %d rows, want 22", len(settingsLockRows))
	}
	if len(tableKinds) != len(commandKinds) {
		t.Errorf("table lists %d kinds and the command %d: a kind is repeated", len(tableKinds), len(commandKinds))
	}

	shape := regexp.MustCompile(`^[a-z]{2}$`)
	seen := make(map[string]string, len(settingsLockRows))
	for _, row := range settingsLockRows {
		if !shape.MatchString(row.Code) {
			t.Errorf("row %q has code %q, want two lower-case letters", row.Kind, row.Code)
		}
		if other, dup := seen[row.Code]; dup {
			t.Errorf("code %q is used by %q and %q", row.Code, other, row.Kind)
		}
		seen[row.Code] = row.Kind
		if row.Code == settingsLocksPagerCode {
			t.Errorf("row %q uses the pager's code %q", row.Kind, row.Code)
		}
		if slices.Contains(settingsCoreActions, row.Code) {
			t.Errorf("row %q uses the core action code %q", row.Kind, row.Code)
		}
		if row.Code == row.Kind {
			t.Errorf("row %q carries the lock's own name as its code", row.Kind)
		}
		if want := "settings_lock_" + row.Kind; row.NameKey != want {
			t.Errorf("row %q has name key %q, want %q", row.Kind, row.NameKey, want)
		}
		if row.Page != 1 && row.Page != 2 {
			t.Errorf("row %q is on page %d, want 1 or 2", row.Kind, row.Page)
		}
	}

	// 12 rows on page 1 then 10 on page 2, in the UI-SPEC's order, with "all" last.
	var page1, page2 []string
	for i, row := range settingsLockRows {
		switch row.Page {
		case 1:
			page1 = append(page1, row.Code)
			if i >= settingsLockPageSize {
				t.Errorf("row %q on page 1 comes after a page 2 row", row.Kind)
			}
		case 2:
			page2 = append(page2, row.Code)
		}
	}
	if want := []string{"sk", "gf", "ph", "vd", "vn", "vo", "au", "dc", "ct", "lc", "gm", "ur"}; !slices.Equal(page1, want) {
		t.Errorf("page 1 = %v, want %v", page1, want)
	}
	if want := []string{"fw", "rt", "ac", "bt", "ms", "cm", "md", "ot", "pv", "al"}; !slices.Equal(page2, want) {
		t.Errorf("page 2 = %v, want %v", page2, want)
	}
	if len(page1) != settingsLockPageSize {
		t.Errorf("page 1 holds %d rows, want the page size %d", len(page1), settingsLockPageSize)
	}
	if last := settingsLockRows[len(settingsLockRows)-1]; last.Kind != "all" {
		t.Errorf("the last row is %q, want all", last.Kind)
	}
}

// TestSettingsLocksKeysExist is the guard of every locale key the Locks page reads through a table
// field or a table of the phase: make check-translations only sees keys that are literal GetString
// arguments, so a lock name, a page title or a log category name missing from one language would
// ship an empty button. Every key must translate to a non-empty string in all 8 languages.
func TestSettingsLocksKeysExist(t *testing.T) {
	withRealLocales(t)

	keys := []string{"settings_locks_state_on", "settings_locks_state_off", "settings_locks_intro"}
	for _, row := range settingsLockRows {
		keys = append(keys, row.NameKey)
	}
	// The six page titles of Phase 18: Warnings, Locks, Switches, Language, Log channel and
	// Anonymous admins.
	keys = append(keys,
		"settings_page_warns", "settings_page_locks", "settings_page_switches",
		"settings_page_lang", "settings_page_log", "settings_page_anon",
	)
	for _, cat := range settingsLogCategories {
		keys = append(keys, cat.NameKey)
	}

	for _, code := range memberReqScreenLanguages {
		tr := i18n.MustNewTranslator(code)
		for _, key := range keys {
			text, err := tr.GetString(key)
			if err != nil || strings.TrimSpace(text) == "" {
				t.Errorf("%s: key %q does not translate: %q, %v", code, key, text, err)
			}
		}
	}
}
