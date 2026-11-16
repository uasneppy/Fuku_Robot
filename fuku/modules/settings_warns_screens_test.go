//go:build testtools

package modules

import (
	"fmt"
	"strings"
	"testing"

	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// settingsWarnsStoredModes are the stored actions a page can meet: the three the commands
// write and the three the table's CHECK also allows.
var settingsWarnsStoredModes = []string{"ban", "kick", "mute", "tban", "tmute", ""}

// settingsWarnsViews is a private and a group menu view with the longest identifiers a button
// carries: a 14-character chat ID and a 10-digit user ID.
func settingsWarnsViews(tr *i18n.Translator, title string) []struct {
	name string
	view settingsView
} {
	return []struct {
		name string
		view settingsView
	}{
		{"private", settingsView{
			Tr: tr, Private: true, OwnerKey: "c", OwnerValue: "-1001234567890",
			GroupID: -1001234567890, GroupTitle: title, OpenerName: title,
		}},
		{"group", settingsView{
			Tr: tr, OwnerKey: "u", OwnerValue: "9999999999",
			GroupID: -1001234567890, GroupTitle: title, OpenerName: title,
		}},
	}
}

func TestSettingsWarnsCallbackBytes(t *testing.T) {
	newSettingsEnv(t) // installs the marker locale the translator reads
	tr := i18n.MustNewTranslator("en")
	for _, variant := range settingsWarnsViews(tr, "G") {
		t.Run(variant.name, func(t *testing.T) {
			largest := 0
			for _, limit := range []int{1, 3, 100} {
				for _, mode := range settingsWarnsStoredModes {
					name := fmt.Sprintf("limit %d mode %q", limit, mode)
					screen := renderSettingsWarns(variant.view, &models.WarnSettings{WarnLimit: limit, WarnMode: mode}, "")
					if screen.Omitted != 0 {
						t.Errorf("%s omitted %d buttons for overflow, want none", name, screen.Omitted)
					}
					if widths := settingsWarnsWidths(screen.Keyboard.InlineKeyboard); fmt.Sprint(widths) != "[5 3 2]" {
						t.Errorf("%s keyboard widths = %v, want [5 3 2]", name, widths)
					}
					for _, button := range settingsAllButtons(screen.Keyboard.InlineKeyboard) {
						n := len(button.CallbackData)
						if n < 1 || n > 64 {
							t.Errorf("%s button %q has %d bytes of callback data, want 1..64", name, button.Text, n)
						}
						largest = max(largest, n)
					}
				}
			}
			want := 48
			if !variant.view.Private {
				want = 44
			}
			if largest != want {
				t.Errorf("the largest button is %d bytes, want %d", largest, want)
			}
		})
	}
}

func TestSettingsWarnsPageFitsEveryLocale(t *testing.T) {
	withRealLocales(t)
	titles := map[string]string{
		"ampersands": strings.Repeat("&", 64),
		"astral":     strings.Repeat("😀", 64),
		"multibyte":  strings.Repeat("Ж", 64),
	}
	for _, code := range []string{"uk", "en", "es", "fr", "hi", "id", "pt", "ru"} {
		t.Run(code, func(t *testing.T) {
			tr := i18n.MustNewTranslator(code)
			notices := []string{
				settingsNoticeWarn(fmt.Sprintf(settingsText(tr, "warns_mode_unknown"), strings.Repeat("a", settingsFloodModeMaxLen))),
				settingsNoticeWarn(settingsText(tr, "connections_is_user_connected_bot_not_admin")),
				settingsNoticeWarn(settingsText(tr, "common_settings_save_failed")),
			}
			for titleName, title := range titles {
				for _, variant := range settingsWarnsViews(tr, title) {
					for _, mode := range []string{"tmute", ""} {
						for _, notice := range notices {
							label := fmt.Sprintf("%s %s %s mode %q", code, titleName, variant.name, mode)
							screen := renderSettingsWarns(variant.view, &models.WarnSettings{WarnLimit: 100, WarnMode: mode}, notice)
							if units := panelUTF16Len(screen.Text); units > 3800 || units == 0 {
								t.Errorf("%s is %d UTF-16 units, want 1..3800", label, units)
							}
							if widths := settingsWarnsWidths(screen.Keyboard.InlineKeyboard); fmt.Sprint(widths) != "[5 3 2]" {
								t.Errorf("%s keyboard widths = %v, want [5 3 2]", label, widths)
							}
							if screen.Omitted != 0 {
								t.Errorf("%s omitted %d buttons", label, screen.Omitted)
							}
							for _, placeholder := range []string{"{n}", "{action}", "%!", "%s"} {
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
