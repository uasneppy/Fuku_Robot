package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func keysOf(entries []Entry) []string {
	keys := make([]string, 0, len(entries))
	for _, e := range entries {
		keys = append(keys, e.Key)
	}
	return keys
}

func TestLoadLocaleKeepsFileOrderAndShapes(t *testing.T) {
	entries, err := LoadLocale("testdata/en.yml")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"admin_ok", "misc_runs", "help_configuration_step-1", "multi_line", "plain_yes", "pseudo", "only_in_english"}
	if got := keysOf(entries); !reflect.DeepEqual(got, want) {
		t.Fatalf("key order = %v, want %v", got, want)
	}
	runs := entries[1]
	if !runs.List || len(runs.Parts) != 2 {
		t.Fatalf("misc_runs = %+v, want a list of 2", runs)
	}
	if entries[0].List || len(entries[0].Parts) != 1 || entries[0].Parts[0] != "Done <b>%s</b>" {
		t.Fatalf("admin_ok = %+v", entries[0])
	}
	if entries[3].Parts[0] != "First line\nSecond line\n" {
		t.Fatalf("multi_line = %q", entries[3].Parts[0])
	}
}

func TestWriteLocaleRoundTripsAwkwardScalars(t *testing.T) {
	awkward := []string{
		"yes", "null", "123", "~", "true", "a: b", "#x", " leading", "trailing ", "",
		"🇺🇦", "multi\nline", "multi\nline with trailing newline\n", "- dash", "'quoted'", `"double"`,
		"100% of {first}", "<b>%s</b>", "tab\tinside", "ends with colon:",
	}
	entries := []Entry{{Key: "misc_runs", Parts: []string{"one", "yes"}, List: true}}
	for i, v := range awkward {
		entries = append(entries, Entry{Key: "k_" + string(rune('a'+i)), Parts: []string{v}})
	}
	path := filepath.Join(t.TempDir(), "out.yml")
	if err := WriteLocale(path, entries); err != nil {
		t.Fatal(err)
	}
	got, err := LoadLocale(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, entries) {
		for i := range entries {
			if i >= len(got) || !reflect.DeepEqual(got[i], entries[i]) {
				t.Errorf("entry %d: wrote %#v, read back %#v", i, entries[i], got[min(i, len(got)-1)])
			}
		}
		t.FailNow()
	}
	if !got[0].List {
		t.Fatal("a list key must stay a YAML sequence")
	}

	// LoadLocale reads source text whatever the tag, so it cannot notice a "yes" that
	// lost its quotes. The bot's own loader decodes into any, where a bare yes is a
	// bool and 123 an int; check what that loader would see.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var asBot map[string]any
	if err := yaml.Unmarshal(raw, &asBot); err != nil {
		t.Fatal(err)
	}
	for _, e := range entries[1:] {
		if s, ok := asBot[e.Key].(string); !ok || s != e.Parts[0] {
			t.Errorf("key %s: the bot would read %#v (%T), want the string %q", e.Key, asBot[e.Key], asBot[e.Key], e.Parts[0])
		}
	}
	list, _ := asBot["misc_runs"].([]any)
	if len(list) != 2 || list[1] != "yes" {
		t.Errorf("misc_runs as the bot reads it = %#v, want [one yes]", asBot["misc_runs"])
	}
}

func TestWriteLocaleLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "uk.yml")
	if err := WriteLocale(path, []Entry{{Key: "a", Parts: []string{"b"}}}); err != nil {
		t.Fatal(err)
	}
	if err := WriteLocale(path, []Entry{{Key: "a", Parts: []string{"c"}}}); err != nil {
		t.Fatal(err)
	}
	files, _ := os.ReadDir(dir)
	if len(files) != 1 || files[0].Name() != "uk.yml" {
		t.Fatalf("directory holds %v, want only uk.yml", files)
	}
}

func TestLoadLocaleRejectsNonStringValues(t *testing.T) {
	cases := map[string]string{
		"mapping value":     "good: ok\nnested:\n  inner: x\n",
		"mapping in a list": "good: ok\nlisty:\n  - fine\n  - key: value\n",
		"list in a list":    "good: ok\nlisty:\n  - - deep\n",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "bad.yml")
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := LoadLocale(path)
			if err == nil {
				t.Fatal("want an error")
			}
			if !strings.Contains(err.Error(), "nested") && !strings.Contains(err.Error(), "listy") {
				t.Fatalf("error %q should name the key", err)
			}
		})
	}
}

func TestLoadLocaleReadsNumbersAsText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "n.yml")
	if err := os.WriteFile(path, []byte("count: 123\nflag: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, err := LoadLocale(path)
	if err != nil {
		t.Fatal(err)
	}
	if entries[0].Parts[0] != "123" || entries[1].Parts[0] != "true" {
		t.Fatalf("got %+v", entries)
	}
}

func TestRealLocalesHaveTheSameKeys(t *testing.T) {
	en, err := LoadLocale("../../locales/en.yml")
	if err != nil {
		t.Fatal(err)
	}
	ru, err := LoadLocale("../../locales/ru.yml")
	if err != nil {
		t.Fatal(err)
	}
	enSet := map[string]bool{}
	lists := 0
	for _, e := range en {
		enSet[e.Key] = true
		if e.List {
			lists++
		}
	}
	for _, e := range ru {
		if !enSet[e.Key] {
			t.Errorf("ru.yml has %q, en.yml does not", e.Key)
		}
		delete(enSet, e.Key)
	}
	for k := range enSet {
		t.Errorf("en.yml has %q, ru.yml does not", k)
	}
	if lists != 1 {
		t.Errorf("en.yml has %d list keys, want exactly 1", lists)
	}
}

func TestLoadLocaleRejectsDuplicateKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dup.yml")
	if err := os.WriteFile(path, []byte("same: one\nother: x\nsame: two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadLocale(path)
	if err == nil || !strings.Contains(err.Error(), "same") {
		t.Fatalf("want an error naming the duplicate key, got %v", err)
	}
}
