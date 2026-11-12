//go:build testtools

package modules

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// settingsLogStates are every stored combination of the six categories with a channel linked, and
// the all-off state of a chat with none.
func settingsLogStates() []settingsLogState {
	var states []settingsLogState
	for bits := range 1 << settingsLogCategoryCount {
		state := settingsLogState{Linked: true}
		for i := range settingsLogCategoryCount {
			state.On[i] = bits&(1<<i) != 0
		}
		states = append(states, state)
	}
	return append(states, settingsLogState{})
}

func TestSettingsLogChannelCallbackBytes(t *testing.T) {
	newSettingsEnv(t) // installs the marker locale the translator reads
	tr := i18n.MustNewTranslator("en")
	for _, variant := range settingsWarnsViews(tr, "G") {
		t.Run(variant.name, func(t *testing.T) {
			largest := 0
			for _, state := range settingsLogStates() {
				name := fmt.Sprintf("%+v", state)
				screen := renderSettingsLogChannel(variant.view, state, "")
				if screen.Omitted != 0 {
					t.Errorf("%s omitted %d buttons for overflow, want none", name, screen.Omitted)
				}
				if got := settingsSwWidths(screen.Keyboard.InlineKeyboard); !slices.Equal(got, []int{2, 2, 2, 2}) {
					t.Errorf("%s keyboard widths = %v, want [2 2 2 2]", name, got)
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

func TestSettingsLogChannelPageFitsEveryLocale(t *testing.T) {
	withRealLocales(t)
	titles := map[string]string{
		"ampersands": strings.Repeat("&", 64),
		"astral":     strings.Repeat("😀", 64),
		"multibyte":  strings.Repeat("Ж", 64),
	}
	const cellMax = 20
	for _, code := range []string{"uk", "en", "es", "fr", "hi", "id", "pt", "ru"} {
		t.Run(code, func(t *testing.T) {
			tr := i18n.MustNewTranslator(code)
			notices := []string{
				"",
				settingsNoticeWarn(settingsText(tr, "logs_not_set")),
				settingsNoticeWarn(settingsText(tr, "common_settings_save_failed")),
				settingsNoticeWarn(settingsText(tr, "logs_unknown_category")),
			}
			for titleName, title := range titles {
				for _, variant := range settingsWarnsViews(tr, title) {
					for _, state := range settingsLogStates() {
						for _, notice := range notices {
							label := fmt.Sprintf("%s %s %s %+v", code, titleName, variant.name, state)
							screen := renderSettingsLogChannel(variant.view, state, notice)
							if units := panelUTF16Len(screen.Text); units > 3800 || units == 0 {
								t.Errorf("%s is %d UTF-16 units, want 1..3800", label, units)
							}
							if screen.Omitted != 0 {
								t.Errorf("%s omitted %d buttons", label, screen.Omitted)
							}
							for _, placeholder := range []string{"{state}", "{icon}", "{page}", "%!", "%s"} {
								if strings.Contains(screen.Text, placeholder) {
									t.Errorf("%s page keeps the unfilled %s", label, placeholder)
								}
							}
							if got := settingsSwWidths(screen.Keyboard.InlineKeyboard); !slices.Equal(got, []int{2, 2, 2, 2}) {
								t.Errorf("%s keyboard widths = %v, want [2 2 2 2]", label, got)
							}
							for _, button := range settingsAllButtons(screen.Keyboard.InlineKeyboard) {
								if strings.TrimSpace(button.Text) == "" {
									t.Errorf("%s has a button with an empty label", label)
								}
								if decoded, ok := decodeCallbackData(button.CallbackData, settingsCallbackNamespace); ok &&
									decoded.Fields["a"] == settingsActToggle && utf8.RuneCountInString(button.Text) > cellMax {
									t.Errorf("%s cell %q is %d code points, want at most %d", label, button.Text, utf8.RuneCountInString(button.Text), cellMax)
								}
							}
						}
					}
				}
			}
		})
	}
}
