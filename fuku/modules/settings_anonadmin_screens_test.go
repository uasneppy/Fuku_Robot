//go:build testtools

package modules

import (
	"fmt"
	"html"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

func TestSettingsAnonAdminCallbackBytes(t *testing.T) {
	newSettingsEnv(t) // installs the marker locale the translator reads
	tr := i18n.MustNewTranslator("en")
	for _, variant := range settingsWarnsViews(tr, "G") {
		t.Run(variant.name, func(t *testing.T) {
			largest := 0
			for _, on := range []bool{false, true} {
				name := fmt.Sprintf("mode on=%v", on)
				screen := renderSettingsAnonAdmin(variant.view, on, "")
				if screen.Omitted != 0 {
					t.Errorf("%s omitted %d buttons for overflow, want none", name, screen.Omitted)
				}
				if got := settingsSwWidths(screen.Keyboard.InlineKeyboard); !slices.Equal(got, []int{1, 2}) {
					t.Errorf("%s keyboard widths = %v, want [1 2]", name, got)
				}
				for _, button := range settingsAllButtons(screen.Keyboard.InlineKeyboard) {
					n := len(button.CallbackData)
					if n < 1 || n > 64 {
						t.Errorf("%s button %q has %d bytes of callback data, want 1..64", name, button.Text, n)
					}
					largest = max(largest, n)
				}
			}
			want := 45
			if !variant.view.Private {
				want = 41
			}
			if largest != want {
				t.Errorf("the largest button is %d bytes, want %d", largest, want)
			}
		})
	}
}

func TestSettingsAnonAdminPageFitsEveryLocale(t *testing.T) {
	withRealLocales(t)
	titles := map[string]string{
		"ampersands": strings.Repeat("&", 64),
		"angles":     strings.Repeat("<", 64),
		"astral":     strings.Repeat("😀", 64),
		"multibyte":  strings.Repeat("Ж", 64),
	}
	for _, code := range []string{"uk", "en", "es", "fr", "hi", "id", "pt", "ru"} {
		t.Run(code, func(t *testing.T) {
			tr := i18n.MustNewTranslator(code)
			notices := []string{
				"",
				settingsNoticeWarn(settingsText(tr, "chat_status_bot_not_admin")),
				settingsNoticeWarn(settingsText(tr, "admin_anon_admin_db_error")),
				settingsNoticeWarn(settingsText(tr, "common_settings_save_failed")),
			}
			for titleName, title := range titles {
				for _, variant := range settingsWarnsViews(tr, title) {
					for _, on := range []bool{false, true} {
						for _, notice := range notices {
							label := fmt.Sprintf("%s %s %s on=%v", code, titleName, variant.name, on)
							screen := renderSettingsAnonAdmin(variant.view, on, notice)
							if units := panelUTF16Len(screen.Text); units > 3800 || units == 0 {
								t.Errorf("%s is %d UTF-16 units, want 1..3800", label, units)
							}
							if screen.Omitted != 0 {
								t.Errorf("%s omitted %d buttons", label, screen.Omitted)
							}
							for _, placeholder := range []string{"{state}", "%!", "%s"} {
								if strings.Contains(screen.Text, placeholder) {
									t.Errorf("%s page keeps the unfilled %s", label, placeholder)
								}
							}
							// The title is in the group line and in the reused sentence, escaped both
							// times and never raw.
							if got := strings.Count(screen.Text, html.EscapeString(title)); got < 2 {
								t.Errorf("%s shows the escaped title %d times, want at least 2", label, got)
							}
							if title != html.EscapeString(title) && strings.Contains(screen.Text, title) {
								t.Errorf("%s shows the raw title", label)
							}
							if got := settingsSwWidths(screen.Keyboard.InlineKeyboard); !slices.Equal(got, []int{1, 2}) {
								t.Errorf("%s keyboard widths = %v, want [1 2]", label, got)
							}
							for _, button := range settingsAllButtons(screen.Keyboard.InlineKeyboard) {
								if strings.TrimSpace(button.Text) == "" {
									t.Errorf("%s has a button with an empty label", label)
								}
								if decoded, ok := decodeCallbackData(button.CallbackData, settingsCallbackNamespace); ok &&
									decoded.Fields["a"] == settingsActToggle && utf8.RuneCountInString(button.Text) > 40 {
									t.Errorf("%s toggle %q is %d code points, want at most 40", label, button.Text, utf8.RuneCountInString(button.Text))
								}
							}
						}
					}
				}
			}
		})
	}
}
