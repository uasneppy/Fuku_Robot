//go:build testtools

package i18n

import (
	"strings"
	"testing"
)

// TestInstallManagerForTestSharesOneManager proves the contract the settings and staff test
// fixtures rest on: one manager built once can be installed by test after test, serves the
// languages it was built with (and the default for any other), and puts the previous
// manager back on restore.
func TestInstallManagerForTestSharesOneManager(t *testing.T) {
	shared, err := NewManagerForTest(map[string]string{
		"en": "greeting: Hello\n",
		"fr": "greeting: Bonjour\n",
	})
	if err != nil {
		t.Fatalf("NewManagerForTest() error = %v", err)
	}
	before := GetManager()

	for round := 1; round <= 2; round++ {
		restore := InstallManagerForTest(shared)
		if GetManager() != shared {
			t.Fatalf("round %d: the installed manager is not the shared one", round)
		}
		for lang, want := range map[string]string{"fr": "Bonjour", "en": "Hello", "xx": "Hello"} {
			tr, trErr := GetManager().GetTranslator(lang)
			if trErr != nil {
				t.Fatalf("round %d: GetTranslator(%q) error = %v", round, lang, trErr)
			}
			got, getErr := tr.GetString("greeting")
			if getErr != nil || got != want {
				t.Fatalf("round %d: %s greeting = %q, %v; want %q", round, lang, got, getErr, want)
			}
		}
		restore()
		if GetManager() != before {
			t.Fatalf("round %d: restore did not put the previous manager back", round)
		}
	}
}

func TestNewManagerForTestRequiresEnglishAndValidYAML(t *testing.T) {
	if _, err := NewManagerForTest(map[string]string{"fr": "greeting: Bonjour\n"}); err == nil ||
		!strings.Contains(err.Error(), "en is missing") {
		t.Fatalf("NewManagerForTest(no en) error = %v, want the missing default language", err)
	}
	if _, err := NewManagerForTest(map[string]string{"en": "greeting: [unclosed\n"}); err == nil {
		t.Fatal("NewManagerForTest(bad yaml) error = nil, want a parse error")
	}
}

// TestOverrideManagerLanguagesForTestStillInstalls keeps the original one-call helper
// behaving as before now that it is built on the two new ones.
func TestOverrideManagerLanguagesForTestStillInstalls(t *testing.T) {
	before := GetManager()
	restore, err := OverrideManagerLanguagesForTest(map[string]string{
		"en": "greeting: Hello\n",
		"es": "greeting: Hola\n",
	})
	if err != nil {
		t.Fatalf("OverrideManagerLanguagesForTest() error = %v", err)
	}
	tr, err := GetManager().GetTranslator("es")
	if err != nil {
		t.Fatalf("GetTranslator(es) error = %v", err)
	}
	if got, _ := tr.GetString("greeting"); got != "Hola" {
		t.Fatalf("es greeting = %q, want Hola", got)
	}
	restore()
	if GetManager() != before {
		t.Fatal("restore did not put the previous manager back")
	}
}
