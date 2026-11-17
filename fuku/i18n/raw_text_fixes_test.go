//go:build testtools

package i18n

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// installRealLocales installs the eight real locale files as the global manager for the
// length of the test. It swaps a global, so the caller never runs with t.Parallel.
func installRealLocales(t *testing.T) {
	t.Helper()
	yamlByLang := make(map[string]string, len(staffLocales))
	for _, code := range staffLocales {
		path := fmt.Sprintf("../../locales/%s.yml", code)
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		yamlByLang[code] = string(raw)
	}
	restore, err := OverrideManagerLanguagesForTest(yamlByLang)
	if err != nil {
		t.Fatalf("install the real locales: %v", err)
	}
	t.Cleanup(restore)
}

// TestRawPlaceholderKeysRender renders every key the handlers fill with named params through
// the real locale files, with the exact param names the Go code passes. A %s or %d verb is
// only filled from keys 0 to 9 or a fixed list of common names, so any other name printed
// the raw verb: each output must hold every value and no verb or brace.
func TestRawPlaceholderKeysRender(t *testing.T) {
	installRealLocales(t)

	listParams := TranslationParams{"modules": "7SENT", "list": "• LISTSENT"}
	cases := []struct {
		key    string
		params TranslationParams
		want   []string
	}{
		{
			key:    "backup_export_success",
			params: TranslationParams{"chat": "CHATSENT", "modules": "7SENT", "time": "TIMESENT", "list": "• LISTSENT"},
			want:   []string{"CHATSENT", "7SENT", "TIMESENT", "LISTSENT"},
		},
		{key: "backup_import_confirm", params: listParams, want: []string{"7SENT", "LISTSENT"}},
		{key: "backup_import_success", params: listParams, want: []string{"7SENT", "LISTSENT"}},
		{key: "backup_reset_confirm", params: listParams, want: []string{"7SENT", "LISTSENT"}},
		{key: "backup_reset_success", params: listParams, want: []string{"7SENT", "LISTSENT"}},
		{
			key:    "captcha_stored_messages_summary",
			params: TranslationParams{"user": "USERSENT", "count": 4321, "types": "TYPESENT"},
			want:   []string{"USERSENT", "4321", "TYPESENT"},
		},
		{key: "captcha_max_attempts_current", params: TranslationParams{"attempts": 9876}, want: []string{"9876"}},
		{key: "captcha_max_attempts_set", params: TranslationParams{"attempts": 9876}, want: []string{"9876"}},
	}

	for _, code := range staffLocales {
		tr, err := NewTranslator(code)
		if err != nil {
			t.Fatalf("translator %s: %v", code, err)
		}
		for _, tc := range cases {
			got, err := tr.GetString(tc.key, tc.params)
			if err != nil {
				t.Errorf("%s %s: %v", code, tc.key, err)
				continue
			}
			if strings.ContainsAny(got, "%{}") {
				t.Errorf("%s %s still holds a verb or a brace: %q", code, tc.key, got)
			}
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Errorf("%s %s lost the value %q: %q", code, tc.key, w, got)
				}
			}
		}
	}
}
