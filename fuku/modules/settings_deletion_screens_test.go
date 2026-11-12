//go:build testtools

package modules

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/helpers"
)

// The pure families of the Deletion page: they render the screens straight from a view and the data a
// draw read, with no bot and no database, so every row, state and language is cheap to cover.

// settingsDelDelayKeysOf is the label key of each preset, written out by the tests.
var settingsDelDelayKeysOf = map[int]string{
	10: "settings_del_delay_10s", 60: "settings_del_delay_1m", 600: "settings_del_delay_10m",
	3600: "settings_del_delay_1h", 21600: "settings_del_delay_6h", 86400: "settings_del_delay_24h",
}

// settingsDelPureView is the view of a menu at worst-case IDs: a 14-character chat ID and a 10-digit user ID.
func settingsDelPureView(tr *i18n.Translator, private bool, title string) settingsView {
	view := settingsView{
		Tr: tr, OwnerKey: "u", OwnerValue: "1234567890",
		GroupID: -1001234567890, GroupTitle: title, OpenerName: "Opener",
	}
	if private {
		view.Private, view.OwnerKey, view.OwnerValue = true, "c", "-1001234567890"
	}
	return view
}

// settingsDelEveryRuleSet is the data of a group with every rule on: the service rows cycle through
// their presets (Delete now included), the bot rows through theirs, and the shared flag is on.
func settingsDelEveryRuleSet() settingsDelData {
	delays := map[string]int{}
	presets := []int{0, 10, 60, 600, 3600, 21600, 86400}
	for i, tc := range settingsDelServiceCases {
		delays[tc.kind] = presets[i%len(presets)]
	}
	for i, tc := range settingsDelBotCases {
		delays[tc.kind] = presets[1+i%(len(presets)-1)]
	}
	return settingsDelData{Rules: &deletion.Rules{ChatID: 1, Delays: delays}, Flag: true}
}

// settingsDelNamedScreen is a rendered screen and what it is.
type settingsDelNamedScreen struct {
	name   string
	screen settingsScreen
}

// settingsDelEveryScreen renders the hub, both lists and the screen of every row.
func settingsDelEveryScreen(prep settingsDelPrepared, notice string) []settingsDelNamedScreen {
	screens := []settingsDelNamedScreen{{"hub", renderSettingsDelHub(prep, settingsDelSections, notice)}}
	for _, section := range settingsDelSections {
		screens = append(screens, settingsDelNamedScreen{"list " + section.Code, renderSettingsDelList(prep, section, notice)})
		for _, row := range section.Rows {
			screens = append(screens, settingsDelNamedScreen{"row " + row.Code, renderSettingsDelRow(prep, section, row, notice)})
		}
	}
	return screens
}

func TestSettingsDeletionRowBudget(t *testing.T) {
	withStaffLocale(t)
	tr := i18n.MustNewTranslator("en")
	wantWidths := map[string]string{
		"hub": "[2 1 2]", "list s": "[2 2 2 2 2 2 2]", "list b": "[2 2 1 2]",
		"list m": "[2 1 2]", "row mc": "[2 2]", "row me": "[3 3 2]", "row ms": "[1 3 3 2]",
	}
	for _, private := range []bool{false, true} {
		prep := settingsDelPrepared{View: settingsDelPureView(tr, private, "G"), Data: settingsDelEveryRuleSet()}
		for _, named := range settingsDelEveryScreen(prep, "") {
			want, ok := wantWidths[named.name]
			if !ok {
				want = "[3 3 2]"
				if strings.HasPrefix(named.name, "row s") {
					want = "[2 3 3 2]"
				}
			}
			if got := fmt.Sprint(settingsDelWidths(named.screen.Keyboard.InlineKeyboard)); got != want {
				t.Errorf("private=%v %s: row widths = %s, want %s", private, named.name, got, want)
			}
			for _, button := range settingsAllButtons(named.screen.Keyboard.InlineKeyboard) {
				if button.Url != "" {
					t.Errorf("private=%v %s: a URL button %q", private, named.name, button.Text)
				}
			}
		}
	}

	t.Run("a hub with five sections is four rows", func(t *testing.T) {
		sections := make([]settingsDelSection, 0, 5)
		for i := range 5 {
			section := settingsDelSections[i%len(settingsDelSections)]
			section.Code = fmt.Sprintf("x%d", i)
			sections = append(sections, section)
		}
		prep := settingsDelPrepared{View: settingsDelPureView(tr, true, "G"), Data: settingsDelEveryRuleSet()}
		screen := renderSettingsDelHub(prep, sections, "")
		if got := fmt.Sprint(settingsDelWidths(screen.Keyboard.InlineKeyboard)); got != "[2 2 1 2]" {
			t.Errorf("five-section hub widths = %s, want [2 2 1 2]", got)
		}
	})

	t.Run("Main keeps seven rows with every rule set", func(t *testing.T) {
		settingsDelMenus(t, func(t *testing.T, m *settingsDelMenu) {
			for _, tc := range settingsDelServiceCases {
				if tc.kind != deletion.KindJoined && tc.kind != deletion.KindLeft {
					m.env.setRule(tc.kind, 600)
				}
			}
			for _, tc := range settingsDelBotCases {
				m.env.setRule(tc.kind, 3600)
			}
			m.env.setJoinLeft(deletion.KindJoined, deletion.JoinLeftState{Mode: deletion.JoinLeftNow})
			m.press("a", "bk")
			if _, keyboard := m.last(); len(keyboard) != 7 {
				t.Errorf("Main has %d rows, want 7", len(keyboard))
			}
		})
	})
}

func TestSettingsDeletionCallbackBytes(t *testing.T) {
	withStaffLocale(t)
	tr := i18n.MustNewTranslator("en")
	for _, private := range []bool{false, true} {
		prep := settingsDelPrepared{View: settingsDelPureView(tr, private, "G"), Data: settingsDelEveryRuleSet()}
		longest := 0
		for _, named := range settingsDelEveryScreen(prep, "") {
			if named.screen.Omitted != 0 {
				t.Errorf("private=%v %s omitted %d buttons", private, named.name, named.screen.Omitted)
			}
			for _, button := range settingsAllButtons(named.screen.Keyboard.InlineKeyboard) {
				if size := len(button.CallbackData); size > 64 {
					t.Errorf("private=%v %s: %q is %d bytes", private, named.name, button.CallbackData, size)
				} else {
					longest = max(longest, size)
				}
			}
		}
		want := 46
		if private {
			want = 50
		}
		if longest != want {
			t.Errorf("private=%v: the longest button is %d bytes, want %d", private, longest, want)
		}
	}
}

func TestSettingsDeletionNoCard(t *testing.T) {
	withStaffLocale(t)
	tr := i18n.MustNewTranslator("en")
	allowedActions := map[string]bool{"dl": true, "dr": true, "op": true, "bk": true, "cl": true, "st": true}
	for _, private := range []bool{false, true} {
		prep := settingsDelPrepared{View: settingsDelPureView(tr, private, "G"), Data: settingsDelEveryRuleSet()}
		for _, named := range settingsDelEveryScreen(prep, "") {
			for _, button := range settingsAllButtons(named.screen.Keyboard.InlineKeyboard) {
				if strings.HasPrefix(button.CallbackData, settingsHubNamespace+"|") {
					continue
				}
				fields := settingsButtonFields(t, button)
				if _, hasExpiry := fields["e"]; hasExpiry {
					t.Errorf("%s: button %q has an expiry field: %v", named.name, button.Text, fields)
				}
				if !allowedActions[fields["a"]] {
					t.Errorf("%s: button %q has the action %q, want no card, Yes or No", named.name, button.Text, fields["a"])
				}
			}
		}
	}

	// The one card is Self-destruct's (a row with a ConfirmKey): pressing every choice of every row
	// draws a card, with an expiry, a Yes and a No, only for that row and only for a delay.
	t.Run("only a Self-destruct delay draws a card", func(t *testing.T) {
		env := newSettingsDelEnv(t)
		m := env.groupMenu()
		for _, row := range settingsDelAllRows() {
			values := append([]string{settingsDelNone}, settingsDelRowValues(row)...)
			for _, value := range values {
				m.choose(row.Code, value)
				text, keyboard := m.last()
				cards := 0
				for _, button := range settingsAllButtons(keyboard) {
					if fields := settingsButtonFields(t, button); fields["a"] == "dy" || fields["a"] == "no" || fields["e"] != "" {
						cards++
					}
				}
				wantCard := row.ConfirmKey != "" && value != settingsDelNone
				switch {
				case wantCard && (cards != 2 || !containsMarker(text, row.ConfirmKey)):
					t.Errorf("%s=%s: %d card buttons in %q, want the card", row.Code, value, cards, text)
				case !wantCard && (cards != 0 || (row.ConfirmKey != "" && containsMarker(text, row.ConfirmKey))):
					t.Errorf("%s=%s: %d card buttons in %q, want no card", row.Code, value, cards, text)
				}
			}
		}
	})
}

// settingsDelRowValues are the v values of a row's presets in canonical digits.
func settingsDelRowValues(row deletionRow) []string {
	values := make([]string, 0, len(row.Presets))
	for _, preset := range row.Presets {
		values = append(values, strconv.Itoa(preset))
	}
	return values
}

// settingsDelWantState is what the contract says a stored rule draws: the icon and the words (marker
// strings) of the state, and the value of the choice that is marked ("" for none).
func settingsDelWantState(noneKey string, presets []int, delay int, on bool) (icon string, words []string, marked string) {
	switch {
	case !on:
		return "➖", []string{staffMarker(noneKey)}, "n"
	case delay == 0:
		marked = ""
		if len(presets) > 0 && presets[0] == 0 {
			marked = "0"
		}
		return "🗑", []string{staffMarker("settings_del_state_now")}, marked
	}
	key, preset := settingsDelDelayKeysOf[delay]
	if !preset {
		return "⏳", []string{staffMarker("settings_del_state_after"), staffMarker("settings_del_delay_custom") + " " + fmt.Sprint(delay)}, ""
	}
	// A preset that this row does not offer (10 s on a bot row) is drawn but marks nothing.
	if !slices.Contains(presets, delay) {
		return "⏳", []string{staffMarker("settings_del_state_after"), staffMarker(key)}, ""
	}
	return "⏳", []string{staffMarker("settings_del_state_after"), staffMarker(key)}, fmt.Sprint(delay)
}

// settingsDelCheckState renders the list and the row screen of a row for the data and fails the test
// when the line, the cell, the Currently line or the marked choice differs from the contract.
func settingsDelCheckState(t *testing.T, tr *i18n.Translator, tc settingsDelCase, data settingsDelData, delay int, on bool, label string) {
	t.Helper()
	section, row, ok := settingsDelRowByCode(tc.code)
	if !ok {
		t.Fatalf("no row %s", tc.code)
	}
	prep := settingsDelPrepared{View: settingsDelPureView(tr, true, "G"), Data: data}
	icon, words, marked := settingsDelWantState(section.NoneKey, row.Presets, delay, on)

	list := renderSettingsDelList(prep, section, "")
	settingsDelWantLine(t, list.Text, tc.nameKey, icon, words...)
	var cellFound bool
	for _, button := range settingsAllButtons(list.Keyboard.InlineKeyboard) {
		if containsMarker(button.Text, tc.nameKey) {
			cellFound = true
			if !strings.HasPrefix(button.Text, icon+" ") {
				t.Errorf("%s %s: cell %q lacks the icon %s", tc.code, label, button.Text, icon)
			}
		}
	}
	if !cellFound {
		t.Errorf("%s %s: no cell", tc.code, label)
	}

	screen := renderSettingsDelRow(prep, section, row, "")
	currently := ""
	for _, line := range strings.Split(screen.Text, "\n") {
		if strings.HasPrefix(line, staffMarker("settings_del_currently")) {
			currently = line
		}
	}
	for _, want := range append([]string{icon}, words...) {
		if !strings.Contains(currently, want) {
			t.Errorf("%s %s: Currently line %q lacks %q", tc.code, label, currently, want)
		}
	}
	var marks []string
	for _, button := range settingsAllButtons(screen.Keyboard.InlineKeyboard) {
		if strings.HasPrefix(button.Text, settingsSelectedPrefix) {
			marks = append(marks, settingsButtonFields(t, button)["v"])
		}
	}
	switch {
	case marked == "" && len(marks) != 0:
		t.Errorf("%s %s: choices %v are marked, want none", tc.code, label, marks)
	case marked != "" && (len(marks) != 1 || marks[0] != marked):
		t.Errorf("%s %s: marked choices = %v, want [%s]", tc.code, label, marks, marked)
	}
}

func TestSettingsDeletionStates(t *testing.T) {
	withStaffLocale(t)
	tr := i18n.MustNewTranslator("en")
	cases := append(append([]settingsDelCase{}, settingsDelServiceCases...), settingsDelBotCases...)
	stored := []struct {
		name  string
		delay int
		on    bool
	}{
		{"none", 0, false}, {"zero", 0, true}, {"10 s", 10, true}, {"1 min", 60, true}, {"10 min", 600, true},
		{"1 h", 3600, true}, {"6 h", 21600, true}, {"24 h", 86400, true}, {"37 s", 37, true},
	}
	for _, tc := range cases {
		if tc.kind == deletion.KindJoined || tc.kind == deletion.KindLeft {
			continue
		}
		for _, st := range stored {
			data := settingsDelData{Rules: &deletion.Rules{ChatID: 1, Delays: map[string]int{}}}
			if st.on {
				data.Rules.Delays[tc.kind] = st.delay
			}
			settingsDelCheckState(t, tr, tc, data, st.delay, st.on, st.name)
		}
	}

	// Joined and left read the effective state of the shared flag.
	shared := []struct {
		name  string
		flag  bool
		row   *int
		delay int
		on    bool
	}{
		{"flag off, no row", false, nil, 0, false},
		{"flag off, a stale row", false, helpers.Ptr(60), 0, false},
		{"flag on, no row", true, nil, 0, true},
		{"flag on, a row of 60 s", true, helpers.Ptr(60), 60, true},
		{"flag on, a row of 0", true, helpers.Ptr(0), 0, true},
		{"flag on, a row of 37 s", true, helpers.Ptr(37), 37, true},
	}
	for _, tc := range settingsDelServiceCases[:2] {
		for _, st := range shared {
			data := settingsDelData{Rules: &deletion.Rules{ChatID: 1, Delays: map[string]int{}}, Flag: st.flag}
			if st.row != nil {
				data.Rules.Delays[tc.kind] = *st.row
			}
			settingsDelCheckState(t, tr, tc, data, st.delay, st.on, st.name)
		}
	}

	t.Run("a stored 0 on a bot row reads Delete now and marks nothing", func(t *testing.T) {
		data := settingsDelData{Rules: &deletion.Rules{ChatID: 1, Delays: map[string]int{deletion.KindBotRules: 0}}}
		settingsDelCheckState(t, tr, settingsDelBotCases[2], data, 0, true, "bot zero")
	})
}

func TestSettingsDeletionPageFitsEveryLocale(t *testing.T) {
	withRealLocales(t)
	title := strings.Repeat("Жé語🙂", 16) // 64 runes of mixed scripts, 4-byte included
	for _, code := range []string{"uk", "en", "es", "fr", "hi", "id", "pt", "ru"} {
		tr := i18n.MustNewTranslator(code)
		rights := map[string]string{
			"can":     "",
			"cannot":  "🚫 " + settingsText(tr, "settings_del_no_right"),
			"unknown": settingsUnknownState + " " + settingsText(tr, "settings_del_right_unknown"),
		}
		notice := settingsNoticeWarn(settingsText(tr, "common_settings_save_failed"))
		for _, private := range []bool{false, true} {
			for rightName, right := range rights {
				for _, withNotice := range []string{"", notice} {
					prep := settingsDelPrepared{View: settingsDelPureView(tr, private, title), Data: settingsDelEveryRuleSet(), Right: right}
					for _, named := range settingsDelEveryScreen(prep, withNotice) {
						label := fmt.Sprintf("%s private=%v right=%s notice=%v %s", code, private, rightName, withNotice != "", named.name)
						if units := len(utf16.Encode([]rune(named.screen.Text))); units > 3800 {
							t.Errorf("%s: %d UTF-16 units, want at most 3800", label, units)
						}
						if named.screen.Omitted != 0 {
							t.Errorf("%s: omitted %d buttons", label, named.screen.Omitted)
						}
						checkButtons(t, label, named.screen.Keyboard.InlineKeyboard)
						if strings.Contains(named.screen.Text, "{") {
							t.Errorf("%s: an unreplaced placeholder in %q", label, named.screen.Text)
						}
					}
				}
			}
		}
	}
}

// checkButtons fails the test for a button whose callback data is over 64 bytes or empty.
func checkButtons(t *testing.T, label string, keyboard [][]gotgbot.InlineKeyboardButton) {
	t.Helper()
	for _, button := range settingsAllButtons(keyboard) {
		if size := len(button.CallbackData); size == 0 || size > 64 {
			t.Errorf("%s: button %q has %d bytes of callback data", label, button.Text, size)
		}
	}
}
