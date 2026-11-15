//go:build testtools

package modules

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/keyboard"
)

// settingsLangStoredCodes is every stored value the page can draw: the eight languages and a code
// the bot has no locale for.
var settingsLangStoredCodes = []string{"uk", "en", "es", "fr", "hi", "id", "pt", "ru", "xx"}

func TestSettingsLanguageCallbackBytes(t *testing.T) {
	withRealLocales(t)
	settingsLangWithCodes(t, settingsLangDefaultCodes)
	tr := i18n.MustNewTranslator("en")
	for _, variant := range settingsWarnsViews(tr, "G") {
		t.Run(variant.name, func(t *testing.T) {
			largest := 0
			for _, stored := range settingsLangStoredCodes {
				screen := renderSettingsLanguage(variant.view, stored, "")
				if screen.Omitted != 0 {
					t.Errorf("stored %s omitted %d buttons for overflow, want none", stored, screen.Omitted)
				}
				if got := settingsLangRowWidths(screen.Keyboard.InlineKeyboard); !slices.Equal(got, []int{2, 2, 2, 2, 2}) {
					t.Errorf("stored %s keyboard widths = %v, want [2 2 2 2 2]", stored, got)
				}
				for _, button := range settingsAllButtons(screen.Keyboard.InlineKeyboard) {
					n := len(button.CallbackData)
					if n < 1 || n > 64 {
						t.Errorf("stored %s button %q has %d bytes of callback data, want 1..64", stored, button.Text, n)
					}
					largest = max(largest, n)
					if fields := settingsButtonFields(t, button); fields["a"] == settingsActSet {
						if label := utf8.RuneCountInString(button.Text); label > 20 {
							t.Errorf("stored %s cell %q is %d code points, want at most 20", stored, button.Text, label)
						}
					}
				}
			}
			want := 46
			if !variant.view.Private {
				want = 42
			}
			if largest != want {
				t.Errorf("the largest button is %d bytes, want %d", largest, want)
			}
		})
	}
}

func TestSettingsLanguagePageFitsEveryLocale(t *testing.T) {
	withRealLocales(t)
	settingsLangWithCodes(t, settingsLangDefaultCodes)
	titles := map[string]string{
		"ampersands": strings.Repeat("&", 64),
		"astral":     strings.Repeat("😀", 64),
		"multibyte":  strings.Repeat("Ж", 64),
	}
	for _, code := range settingsLangDefaultCodes {
		t.Run(code, func(t *testing.T) {
			tr := i18n.MustNewTranslator(code)
			// The longest success notice is the callback's own text naming any of the eight languages.
			notices := []string{
				"",
				settingsNoticeWarn(settingsText(tr, "language_invalid_selection")),
				settingsNoticeWarn(settingsText(tr, "common_settings_save_failed")),
			}
			for _, named := range settingsLangDefaultCodes {
				notices = append(notices, settingsNoticeOK(settingsText(tr, "language_changed_group", i18n.TranslationParams{"s": keyboard.GetLangFormat(named)})))
			}
			for titleName, title := range titles {
				for _, variant := range settingsWarnsViews(tr, title) {
					for _, stored := range settingsLangStoredCodes {
						for _, notice := range notices {
							label := fmt.Sprintf("%s %s %s stored %s", code, titleName, variant.name, stored)
							screen := renderSettingsLanguage(variant.view, stored, notice)
							if units := panelUTF16Len(screen.Text); units > 3800 || units == 0 {
								t.Errorf("%s is %d UTF-16 units, want 1..3800", label, units)
							}
							if screen.Omitted != 0 {
								t.Errorf("%s omitted %d buttons", label, screen.Omitted)
							}
							if got := settingsLangRowWidths(screen.Keyboard.InlineKeyboard); !slices.Equal(got, []int{2, 2, 2, 2, 2}) {
								t.Errorf("%s keyboard widths = %v, want [2 2 2 2 2]", label, got)
							}
							for _, placeholder := range []string{"{lang}", "{page}", "%!", "%s"} {
								if strings.Contains(screen.Text, placeholder) {
									t.Errorf("%s page keeps the unfilled %s", label, placeholder)
								}
							}
							for _, button := range settingsAllButtons(screen.Keyboard.InlineKeyboard) {
								if strings.TrimSpace(button.Text) == "" {
									t.Errorf("%s has a button with an empty label", label)
								}
							}
						}
					}
				}
			}
		})
	}
}
