//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/greetings"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/db/rules"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/formatting"
)

// settingsEditControl returns the button labelled labelKey on a page, checks its control is t, and returns
// its decoded fields.
func settingsEditControl(t *testing.T, keyboard [][]gotgbot.InlineKeyboardButton, labelKey, control string) (gotgbot.InlineKeyboardButton, map[string]string) {
	t.Helper()
	button, ok := settingsButtonByMarker(keyboard, labelKey)
	if !ok {
		t.Fatalf("the page has no button labelled %s: %+v", labelKey, keyboard)
	}
	fields := settingsButtonFields(t, button)
	if fields["t"] != control {
		t.Fatalf("button %s carries control %q, want %q", labelKey, fields["t"], control)
	}
	return button, fields
}

func TestSettingsEditWelcomeSwitch(t *testing.T) {
	env := newSettingsEditEnv(t)
	owner := env.newOwner(env.chat)

	// Main shows the Welcome button and its state line.
	msgID := env.openMenu(owner)
	mainText, mainKeyboard := env.lastEdit(env.chat.Id, msgID)
	if _, ok := settingsButtonByMarker(mainKeyboard, "settings_page_welcome"); !ok {
		t.Fatal("the Main page has no Welcome button")
	}
	line, ok := settingsMainStateLine(mainText, "settings_page_welcome")
	if !ok || !strings.Contains(line, staffMarker("common_status_on")) {
		t.Fatalf("Welcome status line = %q (found %v), want it to read On", line, ok)
	}

	// The page lists the three parts, both hints and the switch.
	env.pressRaw(env.chat, owner, msgID, mustSettingsPageButton(t, mainKeyboard, settingsPageWelcome).CallbackData)
	text, keyboard := env.lastEdit(env.chat.Id, msgID)
	for _, want := range []string{
		staffMarker("settings_edit_line_text"), staffMarker("settings_state_default"),
		staffMarker("settings_edit_line_media"), staffMarker("settings_edit_line_buttons"), staffMarker("settings_state_not_set"),
		staffMarker("settings_edit_hint_greeting"), staffMarker("settings_edit_clean_hint_welcome"),
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the Welcome page %q lacks %s", text, want)
		}
	}
	toggle, fields := settingsEditControl(t, keyboard, "settings_btn_edit_switch_welcome", settingsEditControlEnabled)
	if fields["a"] != settingsActToggle || fields["p"] != settingsPageWelcome || fields["v"] != "0" {
		t.Fatalf("the switch carries %v, want tg on page w with v=0", fields)
	}
	if !strings.Contains(toggle.Text, "✅") {
		t.Errorf("the switch label %q does not show On", toggle.Text)
	}
	if _, ok := settingsEditStored(t, env.chat.Id); ok {
		t.Fatal("opening the page created a greetings row")
	}

	// Pressing it stores what /welcome off stores.
	answers := env.answerCount()
	env.pressRaw(env.chat, owner, msgID, toggle.CallbackData)
	if got := env.answerCount() - answers; got != 1 {
		t.Errorf("the press was answered %d times, want once", got)
	}
	row, ok := settingsEditStored(t, env.chat.Id)
	if !ok || row.WelcomeSettings == nil || row.WelcomeSettings.ShouldWelcome {
		t.Fatalf("stored row = %+v (found %v), want welcome_enabled false", row, ok)
	}
	text, keyboard = env.lastEdit(env.chat.Id, msgID)
	toggle, fields = settingsEditControl(t, keyboard, "settings_btn_edit_switch_welcome", settingsEditControlEnabled)
	if fields["v"] != "1" || !strings.Contains(toggle.Text, "❌") {
		t.Errorf("after the press the switch is %q with %v, want Off and v=1", toggle.Text, fields)
	}
	if strings.Contains(text, settingsNoticeWarnIcon) {
		t.Errorf("a stored write redrew with a notice: %q", text)
	}

	twin := env.addChat()
	editCleanup(t, twin.Id)
	twinOwner := env.newOwner(twin)
	env.sendIn(twin, twinOwner, "/welcome off")
	requireSameGreetingRow(t, loadGreetingRow(t, twin.Id), loadGreetingRow(t, env.chat.Id))

	// A stale press of the same absolute value writes the same value again.
	stale := encodeCallbackData(settingsCallbackNamespace, env.groupFields(owner, "a", "tg", "p", "w", "t", "e", "v", "0"))
	env.pressRaw(env.chat, owner, msgID, stale)
	if row, _ := settingsEditStored(t, env.chat.Id); row.WelcomeSettings.ShouldWelcome {
		t.Error("a repeated v=0 press turned the welcome back on")
	}

	// v=1 turns it on.
	env.pressRaw(env.chat, owner, msgID, toggle.CallbackData)
	if row, _ := settingsEditStored(t, env.chat.Id); !row.WelcomeSettings.ShouldWelcome {
		t.Error("the v=1 press did not turn the welcome on")
	}
}

// mustSettingsPageButton is settingsPageButton that fails the test when the page has no button.
func mustSettingsPageButton(t *testing.T, keyboard [][]gotgbot.InlineKeyboardButton, pageKey string) gotgbot.InlineKeyboardButton {
	t.Helper()
	button, ok := settingsPageButton(t, keyboard, pageKey)
	if !ok {
		t.Fatalf("no button for page %q", pageKey)
	}
	return button
}

// settingsEditView is the view of a group menu or a private menu of the worst-case IDs.
func settingsEditView(tr *i18n.Translator, private bool) settingsView {
	if private {
		return settingsView{Tr: tr, Private: true, OwnerKey: "c", OwnerValue: "-1001234567890", GroupID: -1001234567890, GroupTitle: "G", OpenerName: "N"}
	}
	return settingsView{Tr: tr, OwnerKey: "u", OwnerValue: "1234567890", GroupID: -1001234567890, GroupTitle: "G", OpenerName: "N"}
}

// settingsEditDef is the definition of the page with key.
func settingsEditDef(t *testing.T, key string) settingsEditPageDef {
	t.Helper()
	def, ok := settingsEditPageByKey(key)
	if !ok {
		t.Fatalf("no editor page %q", key)
	}
	return def
}

// settingsEditHasNoticeLine reports whether some line of a page text starts with the notice icon.
// A switch's state line also shows the check mark, but it starts with its locale key, so only a
// notice line starts with the icon itself.
func settingsEditHasNoticeLine(text, icon string) bool {
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, icon) {
			return true
		}
	}
	return false
}

// settingsEditHasButton reports whether a keyboard has a button whose label carries the marker.
func settingsEditHasButton(keyboard [][]gotgbot.InlineKeyboardButton, labelKey string) bool {
	_, ok := settingsButtonByMarker(keyboard, labelKey)
	return ok
}

// settingsEditLabelName is the short name settingsEditRowNames gives a button whose label starts
// with the marker of the locale key: the key without its settings_btn_edit_ or settings_btn_
// prefix.
func settingsEditLabelName(key string) string {
	for _, prefix := range []string{"settings_btn_edit_", "settings_btn_"} {
		if name, ok := strings.CutPrefix(key, prefix); ok {
			return name
		}
	}
	return key
}

// settingsEditLabelMarker finds the first locale marker in a button label.
var settingsEditLabelMarker = regexp.MustCompile(`@@([a-z0-9-]+)@@`)

// settingsEditRowNames draws a screen's keyboard as one string per row: the short names of the
// row's buttons joined by " | ", in order. A button's name comes from the first locale marker of
// its label, so a toggle's state word is not part of it.
func settingsEditRowNames(screen settingsScreen) []string {
	var rows []string
	for _, row := range screen.Keyboard.InlineKeyboard {
		var names []string
		for _, button := range row {
			match := settingsEditLabelMarker.FindStringSubmatch(button.Text)
			if match == nil {
				names = append(names, button.Text)
				continue
			}
			names = append(names, settingsEditLabelName(strings.ReplaceAll(match[1], "-", "_")))
		}
		rows = append(rows, strings.Join(names, " | "))
	}
	return rows
}

// settingsEditCountPreviews counts the preview buttons of a page: one Preview per part and one
// Full preview, found by their action codes.
func settingsEditCountPreviews(t *testing.T, screen settingsScreen, def settingsEditPageDef) (previews, full int) {
	t.Helper()
	for _, button := range settingsAllButtons(screen.Keyboard.InlineKeyboard) {
		if button.Url != "" {
			continue
		}
		switch settingsButtonFields(t, button)["a"] {
		case def.PreviewCode:
			previews++
		case def.FullCode:
			full++
		}
	}
	return previews, full
}

func TestSettingsEditPages(t *testing.T) {
	withStaffLocale(t)
	tr := i18n.MustNewTranslator("en")
	welcome := settingsEditDef(t, settingsPageWelcome)
	rulesDef := settingsEditDef(t, settingsPageRules)

	t.Run("a custom text shows its length and never itself", func(t *testing.T) {
		st := settingsEditState{
			Kind: settingsEditWelcome, Text: "<b>SECRETTEXT</b> hello {first}", Enabled: true,
			Buttons: models.ButtonArray{{Name: "SECRETBUTTON", Url: "https://secret.example/path"}, {Name: "Two", Url: "https://two.example"}},
		}
		screen := renderSettingsEditor(settingsEditView(tr, false), welcome, st, settingsEditScreenOpts{})
		want := fmt.Sprintf("%s %d", staffMarker("settings_edit_state_text_len"), formatting.VisibleLength(st.Text))
		if !strings.Contains(screen.Text, want) {
			t.Errorf("page %q lacks %q", screen.Text, want)
		}
		if want := staffMarker("settings_edit_state_buttons_n") + " 2"; !strings.Contains(screen.Text, want) {
			t.Errorf("page %q lacks %q", screen.Text, want)
		}
		for _, secret := range []string{"SECRETTEXT", "SECRETBUTTON", "secret.example", "two.example"} {
			if strings.Contains(screen.Text, secret) {
				t.Errorf("page %q echoes %q", screen.Text, secret)
			}
			for _, button := range settingsAllButtons(screen.Keyboard.InlineKeyboard) {
				if strings.Contains(button.Text, secret) || strings.Contains(button.CallbackData, secret) || strings.Contains(button.Url, secret) {
					t.Errorf("button %+v echoes %q", button, secret)
				}
			}
		}
	})

	t.Run("every media kind has its word, and no media is Not set", func(t *testing.T) {
		kinds := []struct {
			mediaType int
			key       string
		}{
			{db.PHOTO, "settings_edit_media_photo"}, {db.VIDEO, "settings_edit_media_video"},
			{db.DOCUMENT, "settings_edit_media_file"}, {db.STICKER, "settings_edit_media_sticker"},
			{db.AUDIO, "settings_edit_media_audio"}, {db.VOICE, "settings_edit_media_voice"},
			{db.VIDEO_NOTE, "settings_edit_media_video_note"},
		}
		for _, kind := range kinds {
			st := settingsEditState{Kind: settingsEditWelcome, TextIsDefault: true, MediaType: kind.mediaType, FileID: "f", Enabled: true}
			text := renderSettingsEditor(settingsEditView(tr, true), welcome, st, settingsEditScreenOpts{}).Text
			if want := staffMarker("settings_edit_line_media") + " " + staffMarker(kind.key); !strings.Contains(text, want) {
				t.Errorf("media type %d: page %q lacks %q", kind.mediaType, text, want)
			}
			wantNote := kind.mediaType == db.STICKER || kind.mediaType == db.VIDEO_NOTE
			if got := strings.Contains(text, staffMarker("settings_edit_note_no_caption")); got != wantNote {
				t.Errorf("media type %d: no-caption note shown = %v, want %v", kind.mediaType, got, wantNote)
			}
		}
		for name, st := range map[string]settingsEditState{
			"text type":    {Kind: settingsEditWelcome, MediaType: db.TEXT, FileID: "f"},
			"no file ID":   {Kind: settingsEditWelcome, MediaType: db.PHOTO},
			"unknown type": {Kind: settingsEditWelcome, MediaType: 99, FileID: "f"},
		} {
			text := renderSettingsEditor(settingsEditView(tr, true), welcome, st, settingsEditScreenOpts{}).Text
			if want := staffMarker("settings_edit_line_media") + " " + staffMarker("settings_state_not_set"); !strings.Contains(text, want) {
				t.Errorf("%s: page %q lacks %q", name, text, want)
			}
		}
	})

	t.Run("Rules has the text line only and no switch", func(t *testing.T) {
		for _, text := range []string{"", "the rules"} {
			screen := renderSettingsEditor(settingsEditView(tr, false), rulesDef,
				settingsEditState{Kind: settingsEditRules, Text: text}, settingsEditScreenOpts{BotUsername: "FukuTestBot"})
			if !strings.Contains(screen.Text, staffMarker("settings_edit_line_text")) {
				t.Errorf("Rules page %q has no text line", screen.Text)
			}
			for _, absent := range []string{"settings_edit_line_media", "settings_edit_line_buttons", "settings_edit_clean_hint_welcome", "settings_edit_clean_hint_goodbye"} {
				if strings.Contains(screen.Text, staffMarker(absent)) {
					t.Errorf("Rules page %q carries %s", screen.Text, absent)
				}
			}
			for _, absent := range []string{"settings_btn_edit_switch_welcome", "settings_btn_edit_switch_clean", "settings_btn_edit_switch_goodbye"} {
				if settingsEditHasButton(screen.Keyboard.InlineKeyboard, absent) {
					t.Errorf("Rules page has the %s button", absent)
				}
			}
		}
		set := renderSettingsEditor(settingsEditView(tr, true), rulesDef, settingsEditState{Kind: settingsEditRules, Text: "abc"}, settingsEditScreenOpts{}).Text
		if want := staffMarker("settings_edit_state_text_len") + " 3"; !strings.Contains(set, want) {
			t.Errorf("Rules page %q lacks %q", set, want)
		}
		unset := renderSettingsEditor(settingsEditView(tr, true), rulesDef, settingsEditState{Kind: settingsEditRules}, settingsEditScreenOpts{}).Text
		if want := staffMarker("settings_edit_line_text") + " " + staffMarker("settings_state_not_set"); !strings.Contains(unset, want) {
			t.Errorf("an empty Rules page %q lacks %q", unset, want)
		}
	})

	t.Run("Open in private only in a group menu, rows and navigation", func(t *testing.T) {
		for _, def := range settingsEditPages {
			st := settingsEditState{Kind: def.Kind, Enabled: true}
			group := renderSettingsEditor(settingsEditView(tr, false), def, st, settingsEditScreenOpts{BotUsername: "FukuTestBot"})
			private := renderSettingsEditor(settingsEditView(tr, true), def, st, settingsEditScreenOpts{BotUsername: "FukuTestBot"})

			var urls []string
			for _, button := range settingsAllButtons(group.Keyboard.InlineKeyboard) {
				if button.Url != "" {
					urls = append(urls, button.Url)
				}
			}
			if want := "https://t.me/FukuTestBot?start=settings_-1001234567890"; len(urls) != 1 || urls[0] != want {
				t.Errorf("%s group page URL buttons = %v, want [%s]", def.Key, urls, want)
			}
			for _, button := range settingsAllButtons(private.Keyboard.InlineKeyboard) {
				if button.Url != "" {
					t.Errorf("%s private page has a URL button %q", def.Key, button.Url)
				}
			}
			if rows := len(private.Keyboard.InlineKeyboard); rows > 8 {
				t.Errorf("%s private page has %d rows, want at most 8", def.Key, rows)
			}
			if rows := len(group.Keyboard.InlineKeyboard); rows > 9 {
				t.Errorf("%s group page has %d rows, want at most 9", def.Key, rows)
			}
			for name, screen := range map[string]settingsScreen{"group": group, "private": private} {
				rows := screen.Keyboard.InlineKeyboard
				last := rows[len(rows)-1]
				if len(last) != 2 || !strings.Contains(last[0].Text, staffMarker("settings_btn_back")) || !strings.Contains(last[1].Text, staffMarker("settings_btn_close")) {
					t.Errorf("%s %s page ends with %+v, want Back and Close", def.Key, name, last)
				}
			}
			// No username: no row at all, and the page still draws.
			noName := renderSettingsEditor(settingsEditView(tr, false), def, st, settingsEditScreenOpts{})
			for _, button := range settingsAllButtons(noName.Keyboard.InlineKeyboard) {
				if button.Url != "" {
					t.Errorf("%s page without a bot username has a URL button %q", def.Key, button.Url)
				}
			}
		}
	})

	t.Run("part rows: Set alone while a part is not set, Set and Remove while it is", func(t *testing.T) {
		rowOf := func(screen settingsScreen, i int) []string {
			var labels []string
			for _, button := range screen.Keyboard.InlineKeyboard[i] {
				labels = append(labels, button.Text)
			}
			return labels
		}
		view := settingsEditView(tr, true)
		for _, def := range []settingsEditPageDef{welcome, settingsEditDef(t, settingsPageGoodbye)} {
			none := renderSettingsEditor(view, def, settingsEditState{Kind: def.Kind, TextIsDefault: true, Enabled: true}, settingsEditScreenOpts{})
			for i, key := range []string{"settings_btn_edit_set_text", "settings_btn_edit_set_media", "settings_btn_edit_set_buttons"} {
				if got := rowOf(none, i); len(got) != 1 || !strings.Contains(got[0], staffMarker(key)) {
					t.Errorf("%s row %d with nothing set = %v, want [%s]", def.Key, i, got, key)
				}
			}
			all := renderSettingsEditor(view, def, settingsEditState{
				Kind: def.Kind, Text: "custom", MediaType: db.PHOTO, FileID: "f",
				Buttons: models.ButtonArray{{Name: "B", Url: "https://example.com"}}, Enabled: true,
			}, settingsEditScreenOpts{})
			for i, keys := range [][2]string{
				{"settings_btn_edit_set_text", "settings_btn_edit_remove_text"},
				{"settings_btn_edit_set_media", "settings_btn_edit_remove_media"},
				{"settings_btn_edit_set_buttons", "settings_btn_edit_remove_buttons"},
			} {
				got := rowOf(all, i)
				if len(got) != 2 || !strings.Contains(got[0], staffMarker(keys[0])) || !strings.Contains(got[1], staffMarker(keys[1])) {
					t.Errorf("%s row %d with everything set = %v, want [%s, %s]", def.Key, i, got, keys[0], keys[1])
				}
			}
		}
		empty := renderSettingsEditor(view, rulesDef, settingsEditState{Kind: settingsEditRules}, settingsEditScreenOpts{})
		if got := rowOf(empty, 0); len(got) != 1 || !strings.Contains(got[0], staffMarker("settings_btn_edit_set_text")) {
			t.Errorf("empty Rules row 0 = %v, want Set text alone", got)
		}
		set := renderSettingsEditor(view, rulesDef, settingsEditState{Kind: settingsEditRules, Text: "r"}, settingsEditScreenOpts{})
		if got := rowOf(set, 0); len(got) != 2 || !strings.Contains(got[1], staffMarker("settings_btn_edit_remove_text")) {
			t.Errorf("Rules row 0 with text = %v, want Set and Remove", got)
		}
		if settingsEditHasButton(set.Keyboard.InlineKeyboard, "settings_btn_edit_set_media") {
			t.Error("the Rules page has a media row")
		}
	})

	t.Run("the mock states draw exactly the contract's rows", func(t *testing.T) {
		buttons := models.ButtonArray{{Name: "B", Url: "https://example.com"}}
		for _, def := range []settingsEditPageDef{welcome, settingsEditDef(t, settingsPageGoodbye)} {
			states := []struct {
				name  string
				state settingsEditState
				rows  []string
			}{
				{
					"nothing set", settingsEditState{Kind: def.Kind, TextIsDefault: true, Enabled: true},
					[]string{
						"set_text", "set_media", "set_buttons", "preview_text | full_preview", "switches", "back | close",
					},
				},
				{
					"text and media set", settingsEditState{Kind: def.Kind, Text: "custom", MediaType: db.PHOTO, FileID: "f", Enabled: true},
					[]string{
						"set_text | remove_text", "set_media | remove_media", "set_buttons",
						"preview_text | preview_media", "full_preview", "switches", "back | close",
					},
				},
				{
					"everything set", settingsEditState{Kind: def.Kind, Text: "custom", MediaType: db.STICKER, FileID: "f", Buttons: buttons, Enabled: true},
					[]string{
						"set_text | remove_text", "set_media | remove_media", "set_buttons | remove_buttons",
						"preview_text | preview_media", "preview_buttons | full_preview", "switches", "back | close",
					},
				},
				{
					"only buttons set", settingsEditState{Kind: def.Kind, TextIsDefault: true, Buttons: buttons, Enabled: true},
					[]string{
						"set_text", "set_media", "set_buttons | remove_buttons",
						"preview_text | preview_buttons", "full_preview", "switches", "back | close",
					},
				},
			}
			for _, tc := range states {
				// The switches that are drawn for the state are a two-column grid, filled left to right.
				var names, grid []string
				for _, cell := range settingsEditSwitchCells(def, tc.state) {
					names = append(names, settingsEditLabelName(cell.LabelKey))
				}
				for i := 0; i < len(names); i += 2 {
					grid = append(grid, strings.Join(names[i:min(i+2, len(names))], " | "))
				}
				tc.rows = slices.Concat(tc.rows[:len(tc.rows)-2], grid, tc.rows[len(tc.rows)-1:])
				t.Run(def.Key+" "+tc.name, func(t *testing.T) {
					private := renderSettingsEditor(settingsEditView(tr, true), def, tc.state, settingsEditScreenOpts{})
					if got := settingsEditRowNames(private); !slices.Equal(got, tc.rows) {
						t.Errorf("private rows = %q, want %q", got, tc.rows)
					}
					if len(tc.rows) > 8 {
						t.Errorf("the mock has %d rows, want at most 8 in a private menu", len(tc.rows))
					}
					group := renderSettingsEditor(settingsEditView(tr, false), def, tc.state, settingsEditScreenOpts{BotUsername: "FukuTestBot"})
					wantGroup := slices.Concat(tc.rows[:len(tc.rows)-1], []string{"open_private", "back | close"})
					if got := settingsEditRowNames(group); !slices.Equal(got, wantGroup) {
						t.Errorf("group rows = %q, want %q", got, wantGroup)
					}
					if len(wantGroup) > 9 {
						t.Errorf("the group page has %d rows, want at most 9", len(wantGroup))
					}
				})
			}
		}

		rulesStates := []struct {
			name  string
			state settingsEditState
			rows  []string
		}{
			{"rules set", settingsEditState{Kind: settingsEditRules, Text: "the rules"}, []string{"set_text | remove_text", "preview_text | full_preview", "back | close"}},
			{"rules not set", settingsEditState{Kind: settingsEditRules}, []string{"set_text", "full_preview", "back | close"}},
		}
		for _, tc := range rulesStates {
			t.Run(tc.name, func(t *testing.T) {
				private := renderSettingsEditor(settingsEditView(tr, true), rulesDef, tc.state, settingsEditScreenOpts{})
				if got := settingsEditRowNames(private); !slices.Equal(got, tc.rows) {
					t.Errorf("private rows = %q, want %q", got, tc.rows)
				}
				group := renderSettingsEditor(settingsEditView(tr, false), rulesDef, tc.state, settingsEditScreenOpts{BotUsername: "FukuTestBot"})
				wantGroup := slices.Concat(tc.rows[:len(tc.rows)-1], []string{"open_private", "back | close"})
				if got := settingsEditRowNames(group); !slices.Equal(got, wantGroup) {
					t.Errorf("group rows = %q, want %q", got, wantGroup)
				}
			})
		}
	})

	t.Run("Delete last twice and the goodbye twins store their own columns", func(t *testing.T) {
		env := newSettingsEditEnv(t)
		owner := env.newOwner(env.chat)
		msgID := openEditPage(env, owner, settingsPageWelcome)

		env.press(owner, msgID, env.groupFields(owner, "a", "tg", "p", "w", "t", "d", "v", "1"))
		row, ok := settingsEditStored(t, env.chat.Id)
		if !ok || !row.WelcomeSettings.CleanWelcome || row.GoodbyeSettings.CleanGoodbye {
			t.Fatalf("after Delete last on Welcome: %+v (found %v), want welcome_clean_old only", row, ok)
		}
		_, keyboard := env.lastEdit(env.chat.Id, msgID)
		if _, fields := settingsEditControl(t, keyboard, "settings_btn_edit_switch_clean", settingsEditControlClean); fields["v"] != "0" {
			t.Errorf("the redrawn Delete last carries %v, want v=0", fields)
		}

		env.press(owner, msgID, env.groupFields(owner, "a", "op", "p", "g"))
		env.press(owner, msgID, env.groupFields(owner, "a", "tg", "p", "g", "t", "d", "v", "1"))
		env.press(owner, msgID, env.groupFields(owner, "a", "tg", "p", "g", "t", "e", "v", "0"))
		row, _ = settingsEditStored(t, env.chat.Id)
		if !row.GoodbyeSettings.CleanGoodbye || row.GoodbyeSettings.ShouldGoodbye || !row.WelcomeSettings.CleanWelcome {
			t.Errorf("after the Goodbye presses: %+v, want goodbye_clean_old on, goodbye off, welcome_clean_old still on", row)
		}
	})
}

func TestSettingsEditReadsCreateNothing(t *testing.T) {
	env := newSettingsEditEnv(t)
	owner := env.newOwner(env.chat)
	msgID := env.openMenu(owner)
	chats := func() int64 {
		var n int64
		if err := db.DB.Model(&models.Chat{}).Where("chat_id = ?", env.chat.Id).Count(&n).Error; err != nil {
			t.Fatalf("count chats: %v", err)
		}
		return n
	}
	before := chats()
	for _, page := range []string{"w", "g", "r"} {
		env.press(owner, msgID, env.groupFields(owner, "a", "op", "p", page))
		env.press(owner, msgID, env.groupFields(owner, "a", "bk"))
	}
	if _, ok := settingsEditStored(t, env.chat.Id); ok {
		t.Error("opening Main and the pages created a greetings row")
	}
	if _, ok := settingsEditRulesStored(t, env.chat.Id); ok {
		t.Error("opening Main and the pages created a rules row")
	}
	if after := chats(); after != before {
		t.Errorf("chats rows for the chat went from %d to %d", before, after)
	}
}

func TestSettingsEditReadFailure(t *testing.T) {
	cases := []struct {
		name    string
		page    string
		titles  []string
		healthy []string
		fail    func()
		restore func()
	}{
		{
			name: "greetings read", page: "w",
			titles: []string{"settings_page_welcome", "settings_page_goodbye"}, healthy: []string{"settings_page_rules"},
			fail: func() {
				settingsEditGreetingsRead = func(context.Context, int64) (*models.GreetingSettings, error) {
					return nil, errors.New("database unavailable")
				}
			},
			restore: func() { settingsEditGreetingsRead = greetings.LoadSettingsContext },
		},
		{
			name: "rules read", page: "r",
			titles: []string{"settings_page_rules"}, healthy: []string{"settings_page_welcome", "settings_page_goodbye"},
			fail: func() {
				settingsEditRulesRead = func(context.Context, int64) (string, error) { return "", errors.New("database unavailable") }
			},
			restore: func() { settingsEditRulesRead = rules.LoadRulesContext },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newSettingsEditEnv(t)
			tc.fail()
			t.Cleanup(tc.restore)

			msgID := env.openMenu(env.admin)
			mainText, keyboard := env.lastEdit(env.chat.Id, msgID)
			for _, key := range tc.titles {
				if line, found := settingsMainStateLine(mainText, key); !found || !strings.HasSuffix(line, settingsUnknownState) {
					t.Errorf("%s status line %q (found %v) does not end with the unknown mark", key, line, found)
				}
			}
			for _, key := range tc.healthy {
				if line, found := settingsMainStateLine(mainText, key); !found || strings.HasSuffix(line, settingsUnknownState) {
					t.Errorf("%s status line %q (found %v) shows the unknown mark for a page that reads fine", key, line, found)
				}
			}

			before := env.answerCount()
			env.pressRaw(env.chat, env.admin, msgID, mustSettingsPageButton(t, keyboard, tc.page).CallbackData)
			if got := env.answerCount() - before; got != 1 {
				t.Errorf("the press was answered %d times, want once", got)
			}
			answer, alert := env.lastAnswer()
			if !alert || !strings.Contains(answer, staffMarker("settings_cannot_open_alert")) {
				t.Errorf("answer = %q (alert %v), want the alert settings_cannot_open_alert", answer, alert)
			}
			text, kb := env.lastEdit(env.chat.Id, msgID)
			if !strings.Contains(text, staffMarker("settings_cannot_open")) || len(kb) != 0 {
				t.Errorf("the menu was edited to %q with keyboard %+v, want the cannot-open edit without a keyboard", text, kb)
			}
		})
	}
}

func TestSettingsEditDefaultsMatchRuntime(t *testing.T) {
	withStaffLocale(t)
	env := newSettingsEditEnv(t)
	owner := env.newOwner(env.chat)
	// The bot knows every group it works in, which is what makes the first write store Goodbye On.
	if err := db.EnsureChatInDb(env.chat.Id, env.chat.Title); err != nil {
		t.Fatalf("EnsureChatInDb: %v", err)
	}

	msgID := openEditPage(env, owner, settingsPageGoodbye)
	text, keyboard := env.lastEdit(env.chat.Id, msgID)
	toggle, fields := settingsEditControl(t, keyboard, "settings_btn_edit_switch_goodbye", settingsEditControlEnabled)
	if !strings.Contains(toggle.Text, "✅") || fields["v"] != "0" {
		t.Errorf("a fresh chat's Goodbye switch is %q with %v, want On", toggle.Text, fields)
	}
	clean, cleanFields := settingsEditControl(t, keyboard, "settings_btn_edit_switch_clean", settingsEditControlClean)
	if !strings.Contains(clean.Text, "❌") || cleanFields["v"] != "1" {
		t.Errorf("a fresh chat's Delete last is %q with %v, want Off", clean.Text, cleanFields)
	}
	if strings.Contains(text, settingsNoticeWarnIcon) {
		t.Errorf("a fresh page carries a notice: %q", text)
	}

	// Welcome's Greet once is off for a chat with no row, which is what the bot does.
	welcomeMsgID := openEditPage(env, owner, settingsPageWelcome)
	welcomeText, welcomeKeyboard := env.lastEdit(env.chat.Id, welcomeMsgID)
	once, onceFields := settingsEditControl(t, welcomeKeyboard, "settings_btn_edit_switch_once", settingsEditControlOnce)
	if !strings.Contains(once.Text, "❌") || onceFields["v"] != "1" ||
		!strings.Contains(welcomeText, staffMarker("settings_edit_line_once")+" ❌ "+staffMarker("common_status_off")) {
		t.Errorf("a fresh chat's Greet once is %q with %v in %q, want Off", once.Text, onceFields, welcomeText)
	}

	runtime := greetings.GetGreetingSettings(env.chat.Id)
	if runtime.WelcomeSettings == nil || runtime.WelcomeSettings.Once {
		t.Errorf("the runtime reads %+v, want Greet once off, as the page showed", runtime.WelcomeSettings)
	}
	if runtime.GoodbyeSettings == nil || !runtime.GoodbyeSettings.ShouldGoodbye || runtime.GoodbyeSettings.CleanGoodbye {
		t.Errorf("the runtime reads %+v, want goodbye on and delete last off, as the page showed", runtime.GoodbyeSettings)
	}
}

func TestSettingsEditSwitchFailure(t *testing.T) {
	env := newSettingsEditEnv(t)
	owner := env.newOwner(env.chat)
	msgID := openEditPage(env, owner, settingsPageWelcome)

	previous := greetingWrites.WelcomeToggle
	greetingWrites.WelcomeToggle = func(int64, bool) error { return errors.New("write failed") }
	t.Cleanup(func() { greetingWrites.WelcomeToggle = previous })

	env.press(owner, msgID, env.groupFields(owner, "a", "tg", "p", "w", "t", "e", "v", "0"))

	text, keyboard := env.lastEdit(env.chat.Id, msgID)
	if !strings.Contains(text, settingsNoticeWarnIcon+staffMarker("common_settings_save_failed")) {
		t.Errorf("a failed write drew %q, want the save-failed notice", text)
	}
	toggle, fields := settingsEditControl(t, keyboard, "settings_btn_edit_switch_welcome", settingsEditControlEnabled)
	if !strings.Contains(toggle.Text, "✅") || fields["v"] != "0" {
		t.Errorf("the switch after a failed write is %q with %v, want the old state On", toggle.Text, fields)
	}
	if row, ok := settingsEditStored(t, env.chat.Id); ok && !row.WelcomeSettings.ShouldWelcome {
		t.Errorf("a failed write left %+v", row)
	}
}

// settingsEditPressAs hand-builds a press of an editor control in chat as user in a group menu.
func settingsEditPressAs(env *settingsEnv, chat gotgbot.Chat, user gotgbot.User, msgID int64, page, control, value string) {
	env.t.Helper()
	data := encodeCallbackData(settingsCallbackNamespace, env.groupFields(user, "a", "tg", "p", page, "t", control, "v", value))
	if data == "" {
		env.t.Fatalf("press %s %s %s did not encode", page, control, value)
	}
	env.pressRaw(chat, user, msgID, data)
}

func TestSettingsParityGreetingSwitches(t *testing.T) {
	const menuMsgID = 6161
	cases := []struct {
		name, page, control, value, command string
	}{
		{"welcome off", "w", "e", "0", "/welcome off"},
		{"welcome on after off", "w", "e", "1", "/welcome on"},
		{"goodbye off", "g", "e", "0", "/goodbye off"},
		{"goodbye on", "g", "e", "1", "/goodbye on"},
		{"delete last welcome on", "w", "d", "1", "/cleanwelcome on"},
		{"delete last goodbye on", "g", "d", "1", "/cleangoodbye on"},
		{"delete last welcome off", "w", "d", "0", "/cleanwelcome off"},
	}
	env := newSettingsEditEnv(t)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			menuChat, commandChat := env.addChat(), env.addChat()
			editCleanup(t, menuChat.Id, commandChat.Id)
			menuOwner, commandOwner := env.newOwner(menuChat), env.newOwner(commandChat)
			// Warm the cached read of both chats, so a missing invalidation shows as a stale value.
			greetings.GetGreetingSettings(menuChat.Id)
			greetings.GetGreetingSettings(commandChat.Id)

			settingsEditPressAs(env, menuChat, menuOwner, menuMsgID, tc.page, tc.control, tc.value)
			env.sendIn(commandChat, commandOwner, tc.command)

			requireSameGreetingRow(t, loadGreetingRow(t, commandChat.Id), loadGreetingRow(t, menuChat.Id))
			menuStored, _ := settingsEditStored(t, menuChat.Id)
			commandStored, _ := settingsEditStored(t, commandChat.Id)
			if menuStored.WelcomeSettings.ShouldWelcome != commandStored.WelcomeSettings.ShouldWelcome ||
				menuStored.GoodbyeSettings.ShouldGoodbye != commandStored.GoodbyeSettings.ShouldGoodbye ||
				menuStored.WelcomeSettings.CleanWelcome != commandStored.WelcomeSettings.CleanWelcome ||
				menuStored.GoodbyeSettings.CleanGoodbye != commandStored.GoodbyeSettings.CleanGoodbye {
				t.Errorf("stored rows differ: menu %+v, command %+v", menuStored, commandStored)
			}
			menuCached, commandCached := greetings.GetGreetingSettings(menuChat.Id), greetings.GetGreetingSettings(commandChat.Id)
			if menuCached.WelcomeSettings.ShouldWelcome != commandCached.WelcomeSettings.ShouldWelcome ||
				menuCached.GoodbyeSettings.ShouldGoodbye != commandCached.GoodbyeSettings.ShouldGoodbye ||
				menuCached.WelcomeSettings.CleanWelcome != commandCached.WelcomeSettings.CleanWelcome ||
				menuCached.GoodbyeSettings.CleanGoodbye != commandCached.GoodbyeSettings.CleanGoodbye {
				t.Errorf("cached reads differ: menu %+v/%+v, command %+v/%+v",
					menuCached.WelcomeSettings, menuCached.GoodbyeSettings, commandCached.WelcomeSettings, commandCached.GoodbyeSettings)
			}
			// The value really changed: the row is not the untouched default.
			wantOn := tc.value == "1"
			var got bool
			switch tc.page + tc.control {
			case "we":
				got = menuCached.WelcomeSettings.ShouldWelcome
			case "ge":
				got = menuCached.GoodbyeSettings.ShouldGoodbye
			case "wd":
				got = menuCached.WelcomeSettings.CleanWelcome
			default:
				got = menuCached.GoodbyeSettings.CleanGoodbye
			}
			if got != wantOn {
				t.Errorf("the stored switch is %v, want %v", got, wantOn)
			}
		})
	}
}

func TestSettingsDriftGreetings(t *testing.T) {
	withStaffLocale(t)
	env := newSettingsEditEnv(t)
	owner := env.newOwner(env.chat)
	chatID := env.chat.Id
	buttons := []models.Button{{Name: "SECRETBUTTON", Url: "https://secret.example"}, {Name: "Second", Url: "https://second.example"}}
	if err := greetings.SetWelcomeText(chatID, "a custom welcome", "photo-file", buttons, db.PHOTO); err != nil {
		t.Fatalf("SetWelcomeText: %v", err)
	}
	if err := greetings.SetGoodbyeText(chatID, "a custom goodbye", "", nil, db.TEXT); err != nil {
		t.Fatalf("SetGoodbyeText: %v", err)
	}
	for _, write := range []error{
		greetings.SetWelcomeToggle(chatID, false), greetings.SetCleanWelcomeSetting(chatID, true),
		greetings.SetGoodbyeToggle(chatID, true), greetings.SetCleanGoodbyeSetting(chatID, false),
	} {
		if write != nil {
			t.Fatalf("seeding a switch: %v", write)
		}
	}

	msgID := openEditPage(env, owner, settingsPageWelcome)
	text, keyboard := env.lastEdit(env.chat.Id, msgID)
	for _, want := range []string{
		staffMarker("settings_edit_state_text_len") + " 16", staffMarker("settings_edit_media_photo"), staffMarker("settings_edit_state_buttons_n") + " 2",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the Welcome page %q lacks %q", text, want)
		}
	}
	if toggle, _ := settingsEditControl(t, keyboard, "settings_btn_edit_switch_welcome", settingsEditControlEnabled); !strings.Contains(toggle.Text, "❌") {
		t.Errorf("the Welcome switch %q does not show Off", toggle.Text)
	}
	if clean, _ := settingsEditControl(t, keyboard, "settings_btn_edit_switch_clean", settingsEditControlClean); !strings.Contains(clean.Text, "✅") {
		t.Errorf("Delete last welcome %q does not show On", clean.Text)
	}
	if strings.Contains(text, "SECRETBUTTON") || strings.Contains(text, "custom welcome") {
		t.Errorf("the page echoes stored content: %q", text)
	}

	env.press(owner, msgID, env.groupFields(owner, "a", "op", "p", "g"))
	text, keyboard = env.lastEdit(env.chat.Id, msgID)
	if want := staffMarker("settings_edit_state_text_len") + " 16"; !strings.Contains(text, want) {
		t.Errorf("the Goodbye page %q lacks %q", text, want)
	}
	if want := staffMarker("settings_edit_line_media") + " " + staffMarker("settings_state_not_set"); !strings.Contains(text, want) {
		t.Errorf("the Goodbye page %q lacks %q", text, want)
	}
	if toggle, _ := settingsEditControl(t, keyboard, "settings_btn_edit_switch_goodbye", settingsEditControlEnabled); !strings.Contains(toggle.Text, "✅") {
		t.Errorf("the Goodbye switch %q does not show On", toggle.Text)
	}

	if err := rules.SetChatRules(chatID, "the rules text"); err != nil {
		t.Fatalf("SetChatRules: %v", err)
	}
	env.press(owner, msgID, env.groupFields(owner, "a", "op", "p", "r"))
	text, _ = env.lastEdit(env.chat.Id, msgID)
	if want := staffMarker("settings_edit_state_text_len") + " 14"; !strings.Contains(text, want) {
		t.Errorf("the Rules page %q lacks %q", text, want)
	}
}

func TestSettingsAuthorityParityGreetings(t *testing.T) {
	const menuMsgID = 7171
	nextAdmin := int64(5_000_000)
	newAdmin := func(env *settingsEnv, chat gotgbot.Chat, canChangeInfo bool) gotgbot.User {
		nextAdmin++
		admin := gotgbot.User{Id: uniqueLinkOwnerID() + nextAdmin, FirstName: "Plain"}
		listEditAdmin(env, chat, admin, canChangeInfo)
		return admin
	}

	t.Run("an administrator without can_change_info", func(t *testing.T) {
		env := newSettingsEditEnv(t)
		menuChat, commandChat := env.addChat(), env.addChat()
		editCleanup(t, menuChat.Id, commandChat.Id)
		menuAdmin, commandAdmin := newAdmin(env, menuChat, false), newAdmin(env, commandChat, false)

		answers := env.answerCount()
		settingsEditPressAs(env, menuChat, menuAdmin, menuMsgID, "w", "d", "1")
		if got := env.answerCount() - answers; got != 1 {
			t.Errorf("the press was answered %d times, want once", got)
		}
		answer, alert := env.lastAnswer()
		if !alert || !strings.Contains(answer, staffMarker("settings_no_change_info")) {
			t.Errorf("answer = %q (alert %v), want the alert settings_no_change_info", answer, alert)
		}
		if _, ok := settingsEditStored(t, menuChat.Id); ok {
			t.Error("a refused Delete last press wrote a row")
		}

		env.sendIn(commandChat, commandAdmin, "/cleanwelcome on")
		if !settingsAntifloodSentHas(env, commandChat.Id, staffMarker("chat_status_change_info_cmd_error")) {
			t.Error("/cleanwelcome on was not refused for an administrator without can_change_info")
		}
		if row, ok := settingsEditStored(t, commandChat.Id); ok && row.WelcomeSettings.CleanWelcome {
			t.Error("/cleanwelcome on wrote although it was refused")
		}

		// The switch is allowed to any administrator, in the menu as /welcome off is.
		settingsEditPressAs(env, menuChat, menuAdmin, menuMsgID, "w", "e", "0")
		if row, ok := settingsEditStored(t, menuChat.Id); !ok || row.WelcomeSettings.ShouldWelcome {
			t.Errorf("the switch press left %+v (found %v), want welcome off", row, ok)
		}
		env.sendIn(commandChat, commandAdmin, "/welcome off")
		if row, ok := settingsEditStored(t, commandChat.Id); !ok || row.WelcomeSettings.ShouldWelcome {
			t.Errorf("/welcome off left %+v (found %v), want welcome off", row, ok)
		}
	})

	t.Run("an administrator with can_change_info", func(t *testing.T) {
		env := newSettingsEditEnv(t)
		menuChat, commandChat := env.addChat(), env.addChat()
		editCleanup(t, menuChat.Id, commandChat.Id)
		menuAdmin, commandAdmin := newAdmin(env, menuChat, true), newAdmin(env, commandChat, true)

		settingsEditPressAs(env, menuChat, menuAdmin, menuMsgID, "g", "d", "1")
		env.sendIn(commandChat, commandAdmin, "/cleangoodbye on")
		for name, chatID := range map[string]int64{"menu": menuChat.Id, "command": commandChat.Id} {
			if row, ok := settingsEditStored(t, chatID); !ok || !row.GoodbyeSettings.CleanGoodbye {
				t.Errorf("%s left %+v (found %v), want goodbye_clean_old on", name, row, ok)
			}
		}
	})

	t.Run("a plain member's press is refused", func(t *testing.T) {
		env := newSettingsEditEnv(t)
		member := gotgbot.User{Id: uniqueLinkOwnerID() + 6_000_000, FirstName: "Member"}
		env.fake.setMember(env.chat.Id, member.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
		settingsEditPressAs(env, env.chat, member, menuMsgID, "w", "e", "0")
		answer, alert := env.lastAnswer()
		if !alert || !strings.Contains(answer, staffMarker("settings_not_admin")) {
			t.Errorf("answer = %q (alert %v), want the alert settings_not_admin", answer, alert)
		}
		if _, ok := settingsEditStored(t, env.chat.Id); ok {
			t.Error("a plain member's press wrote a row")
		}
	})
}

func TestSettingsEditRegistry(t *testing.T) {
	want := []struct {
		key, icon, titleKey string
		order               int
	}{
		{"w", "🎉", "settings_page_welcome", 50},
		{"g", "👋", "settings_page_goodbye", 60},
		{"r", "📜", "settings_page_rules", 70},
	}
	for _, w := range want {
		pg, ok := settingsPageByKey(w.key)
		if !ok {
			t.Errorf("page %q is not registered", w.key)
			continue
		}
		if pg.Order != w.order || pg.Icon != w.icon || pg.TitleKey != w.titleKey || pg.OwnerOnly {
			t.Errorf("page %q = order %d icon %q title %q owner-only %v, want %d %q %q false",
				w.key, pg.Order, pg.Icon, pg.TitleKey, pg.OwnerOnly, w.order, w.icon, w.titleKey)
		}
		// Welcome e, d, u, o; Goodbye e, d, u; Rules none (plan 12-12 adds the Media under text switch
		// of Rules).
		wantControls := map[string]int{"w": 4, "g": 3, "r": 0}[w.key]
		if len(pg.Controls) != wantControls {
			t.Errorf("page %q declares %d controls, want %d", w.key, len(pg.Controls), wantControls)
		}
	}
	if pg, _ := settingsPageByKey("w"); pg.Controls[settingsEditControlOnce].Right != settingsRightAdmin {
		t.Errorf("Welcome control o declares right %d, want settingsRightAdmin", pg.Controls[settingsEditControlOnce].Right)
	}
	if pg, _ := settingsPageByKey("g"); len(pg.Controls) != 3 || pg.Controls[settingsEditControlOnce].Set != nil {
		t.Error("Greet once must be on Welcome only")
	}
	for _, key := range []string{"w", "g"} {
		pg, _ := settingsPageByKey(key)
		if got := pg.Controls[settingsEditControlEnabled].Right; got != settingsRightAdmin {
			t.Errorf("page %q control e declares right %d, want settingsRightAdmin", key, got)
		}
		if got := pg.Controls[settingsEditControlClean].Right; got != settingsRightChangeInfo {
			t.Errorf("page %q control d declares right %d, want settingsRightChangeInfo", key, got)
		}
		// Media under text needs the page's own edit right (UI-SPEC assumption A2).
		def := settingsEditDef(t, key)
		if got := pg.Controls[settingsEditControlBelow]; got.Right != def.EditRight || got.Set == nil || got.Validate == nil {
			t.Errorf("page %q control u declares right %d (Set nil %v), want the page's EditRight %d", key, got.Right, got.Set == nil, def.EditRight)
		}
	}

	env := newSettingsEditEnv(t)
	owner := env.newOwner(env.chat)
	msgID := env.openMenu(owner)
	text, keyboard := env.lastEdit(env.chat.Id, msgID)
	if got, wantShape := settingsMainShape(t, keyboard), "s|l / c|f / w|g / r / a / close"; got != wantShape {
		t.Errorf("the owner's group Main = %q, want %q", got, wantShape)
	}
	for _, key := range []string{"settings_page_welcome", "settings_page_goodbye", "settings_page_rules"} {
		if line, found := settingsMainStateLine(text, key); !found || strings.HasSuffix(line, settingsUnknownState) {
			t.Errorf("Main status line for %s = %q (found %v), want a state", key, line, found)
		}
	}
	// The status block lists the new pages before the Access line.
	if iw, ia := strings.Index(text, staffMarker("settings_page_rules")), strings.Index(text, staffMarker("settings_page_access")); iw < 0 || ia < iw {
		t.Errorf("the Rules line is not before the Access line in %q", text)
	}
}

// settingsEditCallbackStates are the states the byte and fit tests draw every page in.
func settingsEditCallbackStates(def settingsEditPageDef) map[string]settingsEditState {
	return map[string]settingsEditState{
		"default": {Kind: def.Kind, TextIsDefault: def.Kind != settingsEditRules, Enabled: true},
		"all set": {
			Kind: def.Kind, Text: strings.Repeat("x", 3000), MediaType: db.VIDEO_NOTE, FileID: "f",
			Buttons: models.ButtonArray{{Name: "B", Url: "https://example.com"}}, Enabled: true, Clean: true, Once: true, MediaBelow: true,
		},
		"off":      {Kind: def.Kind, Text: "t", MediaType: db.PHOTO, FileID: "f"},
		"sticker":  {Kind: def.Kind, Text: "t", MediaType: db.STICKER, FileID: "f", Clean: true},
		"once on":  {Kind: def.Kind, TextIsDefault: def.Kind != settingsEditRules, Enabled: true, Once: true},
		"once off": {Kind: def.Kind, TextIsDefault: def.Kind != settingsEditRules, Enabled: true},
		// Media under text is drawn for a photo (on or off) and an animation, and the file hint
		// for a document.
		"photo below on": {
			Kind: def.Kind, Text: strings.Repeat("x", 3000), MediaType: db.PHOTO, FileID: "f",
			Buttons: models.ButtonArray{{Name: "B", Url: "https://example.com"}}, Enabled: true, Clean: true, Once: true, MediaBelow: true,
		},
		"photo below off": {Kind: def.Kind, Text: "t", MediaType: db.PHOTO, FileID: "f", Enabled: true},
		"animation below": {Kind: def.Kind, Text: "t", MediaType: db.ANIMATION, FileID: "f", Enabled: true, MediaBelow: true},
		"document":        {Kind: def.Kind, Text: "t", MediaType: db.DOCUMENT, FileID: "f", Enabled: true, MediaBelow: true},
	}
}

func TestSettingsEditCallbackBytes(t *testing.T) {
	tr := i18n.MustNewTranslator("en")
	for _, private := range []bool{true, false} {
		view := settingsEditView(tr, private)
		for _, def := range settingsEditPages {
			for name, st := range settingsEditCallbackStates(def) {
				label := fmt.Sprintf("%s private=%v %s", def.Key, private, name)
				screen := renderSettingsEditor(view, def, st, settingsEditScreenOpts{BotUsername: "FukuTestBot"})
				if screen.Omitted != 0 {
					t.Errorf("%s omitted %d buttons for overflow, want none", label, screen.Omitted)
				}
				var callbacks int
				for _, button := range settingsAllButtons(screen.Keyboard.InlineKeyboard) {
					if button.Url != "" {
						continue
					}
					callbacks++
					if n := len(button.CallbackData); n < 1 || n > 64 {
						t.Errorf("%s button %q has %d bytes of callback data, want 1..64", label, button.Text, n)
					}
				}
				if callbacks == 0 {
					t.Errorf("%s has no callback buttons", label)
				}
				var once gotgbot.InlineKeyboardButton
				hasOnce := false
				for _, button := range settingsAllButtons(screen.Keyboard.InlineKeyboard) {
					if decoded, ok := decodeCallbackData(button.CallbackData, settingsCallbackNamespace); ok && decoded.Fields["t"] == settingsEditControlOnce {
						once, hasOnce = button, true
					}
				}
				if hasOnce != (def.Key == settingsPageWelcome) {
					t.Errorf("%s: the Greet once cell is drawn = %v, want it on Welcome only", label, hasOnce)
				}
				if hasOnce && (len(once.CallbackData) < 1 || len(once.CallbackData) > 64) {
					t.Errorf("%s: the Greet once button has %d bytes, want 1..64", label, len(once.CallbackData))
				}
				// Media under text is on the two greeting pages, drawn exactly for the kinds a caption can go
				// above, and its button is within 64 bytes (45 in a private menu and 41 in a group menu).
				var below gotgbot.InlineKeyboardButton
				hasBelow := false
				for _, button := range settingsAllButtons(screen.Keyboard.InlineKeyboard) {
					if decoded, ok := decodeCallbackData(button.CallbackData, settingsCallbackNamespace); ok && decoded.Fields["t"] == settingsEditControlBelow {
						below, hasBelow = button, true
					}
				}
				if want := def.isGreeting() && settingsEditMediaBelowEligible(st); hasBelow != want {
					t.Errorf("%s: the Media under text cell is drawn = %v, want %v", label, hasBelow, want)
				}
				if limit := map[bool]int{true: 45, false: 41}[private]; hasBelow && len(below.CallbackData) > limit {
					t.Errorf("%s: the Media under text button has %d bytes, want at most %d", label, len(below.CallbackData), limit)
				}
				wantRows := 8
				if !private {
					wantRows = 9
				}
				if rows := len(screen.Keyboard.InlineKeyboard); rows > wantRows {
					t.Errorf("%s has %d rows, want at most %d", label, rows, wantRows)
				}
				if name == "all set" {
					if previews, full := settingsEditCountPreviews(t, screen, def); previews != len(def.Parts) || full != 1 {
						t.Errorf("%s draws %d Preview buttons and %d Full preview, want %d and 1", label, previews, full, len(def.Parts))
					}
				}
			}
		}
		for _, def := range settingsEditPages {
			for _, part := range def.Parts {
				for _, replaced := range []bool{false, true} {
					label := fmt.Sprintf("prompt %s %s private=%v replaced=%v", def.Key, part.Code, private, replaced)
					screen := renderSettingsEditPrompt(view, def, part.Code, time.Now(), settingsEditTextMax, replaced)
					if screen.Omitted != 0 {
						t.Errorf("%s omitted %d buttons for overflow", label, screen.Omitted)
					}
					buttons := settingsAllButtons(screen.Keyboard.InlineKeyboard)
					if len(buttons) != 1 {
						t.Errorf("%s has %d buttons, want only Cancel", label, len(buttons))
					}
					for _, button := range buttons {
						if n := len(button.CallbackData); n < 1 || n > 64 {
							t.Errorf("%s button %q has %d bytes of callback data, want 1..64", label, button.Text, n)
						}
					}
				}
			}
		}
	}
}

func TestSettingsEditPagesFitEveryLocale(t *testing.T) {
	withRealLocales(t)
	titles := map[string]string{
		"ampersands": strings.Repeat("&", 64),
		"astral":     strings.Repeat("😀", 64),
		"multibyte":  strings.Repeat("Ж", 64),
	}
	for _, code := range []string{"uk", "en", "es", "fr", "hi", "id", "pt", "ru"} {
		t.Run(code, func(t *testing.T) {
			tr := i18n.MustNewTranslator(code)
			notice := settingsNoticeWarn(settingsText(tr, "common_settings_save_failed"))
			for titleName, title := range titles {
				for _, private := range []bool{false, true} {
					view := settingsEditView(tr, private)
					view.GroupTitle, view.OpenerName = title, title
					for _, def := range settingsEditPages {
						for stateName, st := range settingsEditCallbackStates(def) {
							label := fmt.Sprintf("%s %s %s private=%v", code, def.Key, titleName, private)
							screen := renderSettingsEditor(view, def, st, settingsEditScreenOpts{Notice: notice, BotUsername: "FukuTestBot"})
							if units := panelUTF16Len(screen.Text); units > 3800 || units == 0 {
								t.Errorf("%s %s is %d UTF-16 units, want 1..3800", label, stateName, units)
							}
							for _, placeholder := range []string{"{n}", "{state}", "{page}", "%!"} {
								if strings.Contains(screen.Text, placeholder) {
									t.Errorf("%s %s keeps the unfilled %s: %q", label, stateName, placeholder, screen.Text)
								}
							}
							for _, button := range settingsAllButtons(screen.Keyboard.InlineKeyboard) {
								if strings.TrimSpace(button.Text) == "" {
									t.Errorf("%s %s has a button with an empty label", label, stateName)
								}
							}
							if screen.Omitted != 0 {
								t.Errorf("%s %s omitted %d buttons", label, stateName, screen.Omitted)
							}
							if stateName == "all set" {
								if previews, full := settingsEditCountPreviews(t, screen, def); previews != len(def.Parts) || full != 1 {
									t.Errorf("%s %s draws %d Preview buttons and %d Full preview, want %d and 1", label, stateName, previews, full, len(def.Parts))
								}
							}
						}
						for _, part := range def.Parts {
							for _, replaced := range []bool{false, true} {
								label := fmt.Sprintf("%s prompt %s %s %s private=%v replaced=%v", code, def.Key, part.Code, titleName, private, replaced)
								screen := renderSettingsEditPrompt(view, def, part.Code, time.Now(), settingsEditTextMax, replaced)
								if units := panelUTF16Len(screen.Text); units > 3800 || units == 0 {
									t.Errorf("%s is %d UTF-16 units, want 1..3800", label, units)
								}
								for _, placeholder := range []string{"{max}", "{time}", "{n}", "%!"} {
									if strings.Contains(screen.Text, placeholder) {
										t.Errorf("%s keeps the unfilled %s: %q", label, placeholder, screen.Text)
									}
								}
								if screen.Omitted != 0 || len(settingsAllButtons(screen.Keyboard.InlineKeyboard)) != 1 {
									t.Errorf("%s has %d omitted buttons and %d buttons, want none omitted and Cancel only", label, screen.Omitted, len(settingsAllButtons(screen.Keyboard.InlineKeyboard)))
								}
							}
						}
					}
				}
			}
		})
	}
}
