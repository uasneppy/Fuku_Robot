//go:build testtools

package modules

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// settingsEditPlainKeys are the message editor's replies and alerts that are one line of text and
// nothing else: the ended replies of the typed-reply door, the answers of Cancel, Remove and the
// previews, the closed prompt and the two notes of a saved page.
var settingsEditPlainKeys = []string{
	"settings_edit_refuse_access_lost", "settings_gate_owner_only", "settings_no_change_info",
	"chat_status_bot_not_admin", "settings_check_failed", "common_settings_save_failed",
	"settings_edit_cancelled", "settings_edit_cancel_nothing", "settings_edit_prompt_stale",
	"settings_edit_wait_expired", "settings_edit_unavailable", "settings_edit_prompt_closed",
	"settings_edit_nothing_to_remove", "settings_edit_group_note", "settings_edit_album_cut",
	"settings_edit_caption_ignored", "settings_edit_preview_sent", "settings_edit_preview_need_start",
	"settings_edit_preview_failed", "settings_edit_nothing_to_preview", "settings_edit_preview_header",
	"settings_edit_preview_header_rules", "settings_edit_preview_buttons_body",
}

// settingsEditRefusalReasons are the reasons the check refuses a typed message with, each with the
// worst-case numbers its parameters take.
func settingsEditRefusalReasons(tr *i18n.Translator) map[string]string {
	numbers := i18n.TranslationParams{"n": 100000, "max": settingsEditTextMax}
	reasons := map[string]string{}
	for _, key := range []string{
		"settings_edit_refuse_need_text", "settings_edit_refuse_need_media", "settings_edit_refuse_text_buttons",
		"settings_edit_refuse_buttons_syntax", "settings_edit_refuse_buttons_limit",
		"settings_edit_refuse_too_long_filled", "settings_edit_refuse_media_text_long", "settings_edit_refuse_button_url",
	} {
		reasons[key] = settingsText(tr, key, numbers)
	}
	return reasons
}

// settingsEditScreens renders every non-page screen of the message editor for view, through the
// functions the code sends them with: the waiting prompt of every part (replaced or not), each
// refusal reply of the typed-reply door with its Cancel button, the saved page with every note, the
// wait-ended reply of every part, the editor page under every notice an action can put on it, and the
// plain replies and alerts. The keys are descriptive and unique.
func settingsEditScreens(tr *i18n.Translator, view settingsView) map[string]settingsScreen {
	screens := map[string]settingsScreen{}
	plain := func(name, text string) { screens[name] = settingsScreen{Text: text} }
	opts := func(notice string) settingsEditScreenOpts {
		return settingsEditScreenOpts{Notice: notice, BotUsername: "FukuTestBot"}
	}

	for _, key := range settingsEditPlainKeys {
		plain("plain "+key, settingsText(tr, key))
	}
	for _, part := range []string{settingsEditPartText, settingsEditPartMedia, settingsEditPartButtons} {
		plain("wait ended "+part, settingsText(tr, "settings_edit_wait_ended_note", i18n.TranslationParams{
			"part": settingsText(tr, settingsInputPartWordKey(part)),
		}))
	}

	for _, def := range settingsEditPages {
		for stateName, st := range settingsEditCallbackStates(def) {
			// The page under each notice an action or the door puts on it.
			for _, key := range []string{
				"settings_edit_cancelled", "settings_edit_group_note", "settings_edit_nothing_to_remove",
				"common_settings_save_failed", "settings_edit_removed_media", "settings_edit_removed_buttons",
				"settings_edit_removed_text_default", "settings_edit_removed_text_rules",
			} {
				notice := settingsNoticeWarn(settingsText(tr, key))
				screens[fmt.Sprintf("page %s %s %s", def.Key, stateName, key)] = renderSettingsEditor(view, def, st, opts(notice))
			}
			// The saved page, with and without the caption and album notes.
			for _, part := range def.Parts {
				for _, caption := range []bool{false, true} {
					for _, album := range []bool{false, true} {
						notice := settingsInputSavedNotice(tr, part.Code, caption, album)
						name := fmt.Sprintf("saved %s %s %s caption=%v album=%v", def.Key, part.Code, stateName, caption, album)
						screens[name] = renderSettingsEditor(view, def, st, opts(notice))
					}
				}
			}
		}
		for _, part := range def.Parts {
			for _, replaced := range []bool{false, true} {
				for _, maxLen := range []int{settingsEditTextMax, settingsEditCaptionMax, settingsEditRulesPromptMax} {
					name := fmt.Sprintf("prompt %s %s replaced=%v max=%d", def.Key, part.Code, replaced, maxLen)
					screens[name] = renderSettingsEditPrompt(view, def, part.Code, time.Now(), maxLen, replaced)
				}
			}
			for key, reason := range settingsEditRefusalReasons(tr) {
				screens[fmt.Sprintf("refusal %s %s %s", def.Key, part.Code, key)] = renderSettingsInputRefusal(view, def, part.Code, reason)
			}
		}
	}
	return screens
}

func TestSettingsEditScreensFitEveryLocale(t *testing.T) {
	withRealLocales(t)
	titles := map[string]string{
		"ampersands": strings.Repeat("&", 64),
		"astral":     strings.Repeat("😀", 64),
		"multibyte":  strings.Repeat("Ж", 64),
	}
	for _, code := range []string{"uk", "en", "es", "fr", "hi", "id", "pt", "ru"} {
		t.Run(code, func(t *testing.T) {
			tr := i18n.MustNewTranslator(code)
			for titleName, title := range titles {
				for _, private := range []bool{false, true} {
					view := settingsEditView(tr, private)
					view.GroupTitle, view.OpenerName = title, title
					screens := settingsEditScreens(tr, view)
					if len(screens) < 100 {
						t.Fatalf("%s %s: only %d screens were rendered", code, titleName, len(screens))
					}
					for name, screen := range screens {
						label := fmt.Sprintf("%s %s private=%v %s", code, titleName, private, name)
						if units := panelUTF16Len(screen.Text); units == 0 || units > 3800 {
							t.Errorf("%s is %d UTF-16 units, want 1..3800", label, units)
						}
						for _, placeholder := range []string{"{n}", "{max}", "{time}", "{part}", "{state}", "{page}", "%!"} {
							if strings.Contains(screen.Text, placeholder) {
								t.Errorf("%s keeps the unfilled %s: %q", label, placeholder, screen.Text)
							}
						}
						if screen.Omitted != 0 {
							t.Errorf("%s omitted %d buttons for overflow", label, screen.Omitted)
						}
						for _, button := range settingsAllButtons(screen.Keyboard.InlineKeyboard) {
							if strings.TrimSpace(button.Text) == "" {
								t.Errorf("%s has a button with an empty label", label)
							}
							if button.Url == "" {
								if n := len(button.CallbackData); n < 1 || n > 64 {
									t.Errorf("%s button %q has %d bytes of callback data, want 1..64", label, button.Text, n)
								}
							}
						}
					}
				}
			}
		})
	}
}
