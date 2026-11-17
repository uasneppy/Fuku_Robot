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

// TestGreetingsNotConfiguredKeysExist proves the two keys the greetings module reads through a
// struct field (so check-translations cannot see them) exist and are plain text in every locale.
func TestGreetingsNotConfiguredKeysExist(t *testing.T) {
	for _, code := range staffLocales {
		data := loadStaffLocale(t, code)
		for _, key := range []string{"greetings_welcome_not_configured", "greetings_goodbye_not_configured"} {
			value, ok := data[key].(string)
			if !ok || strings.TrimSpace(value) == "" {
				t.Errorf("%s: %s is missing or empty", code, key)
				continue
			}
			if strings.ContainsAny(value, "%{}") {
				t.Errorf("%s: %s holds a verb or a brace: %q", code, key, value)
			}
		}
	}
}

// TestBackupHelpExportIsAdminCommand proves /export, which needs an admin, is listed under its own
// heading above the owner commands and no longer under the old "user commands" heading.
func TestBackupHelpExportIsAdminCommand(t *testing.T) {
	oldHeading := map[string]string{
		"en": "User Commands:", "uk": "Команди для користувачів:", "es": "Comandos de Usuario:",
		"fr": "Commandes Utilisateur :", "hi": "उपयोगकर्ता कमांड:", "id": "Perintah Pengguna:",
		"pt": "Comandos de Usuário:", "ru": "Команды пользователя:",
	}
	headingAbove := func(lines []string, command string) (string, int) {
		heading := ""
		for i, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "<b>") {
				heading = strings.TrimSpace(line)
			}
			if strings.Contains(line, command) {
				return heading, i
			}
		}
		return "", -1
	}
	for _, code := range staffLocales {
		help, _ := loadStaffLocale(t, code)["backup_help_msg"].(string)
		if help == "" {
			t.Fatalf("%s: backup_help_msg is missing", code)
		}
		if strings.Contains(help, oldHeading[code]) {
			t.Errorf("%s: the help still lists a user commands heading %q", code, oldHeading[code])
		}
		lines := strings.Split(help, "\n")
		exportHeading, exportAt := headingAbove(lines, "/export")
		importHeading, importAt := headingAbove(lines, "/import")
		if exportAt < 0 || importAt < 0 || exportAt > importAt {
			t.Errorf("%s: /export must be listed before /import (%d, %d)", code, exportAt, importAt)
			continue
		}
		if exportHeading == "" || exportHeading == importHeading {
			t.Errorf("%s: /export and /import share the heading %q", code, exportHeading)
		}
	}
}

// TestCaptchaMutedActionStatesTheDuration proves the captcha failure mute, which lasts 24 hours,
// is no longer described as permanent in any locale.
func TestCaptchaMutedActionStatesTheDuration(t *testing.T) {
	permanent := map[string]string{
		"en": "permanent", "es": "permanente", "fr": "définitiv", "hi": "स्थायी",
		"id": "permanen", "pt": "permanente", "ru": "навсегда", "uk": "назавжди",
	}
	for _, code := range staffLocales {
		value, _ := loadStaffLocale(t, code)["captcha_action_muted"].(string)
		if !strings.Contains(value, "24") {
			t.Errorf("%s: captcha_action_muted does not say 24 hours: %q", code, value)
		}
		if strings.Contains(strings.ToLower(value), permanent[code]) {
			t.Errorf("%s: captcha_action_muted still says permanent: %q", code, value)
		}
	}
}
