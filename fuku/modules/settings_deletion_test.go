//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/db/staff"
)

// settingsDelCase is one row of a table the page draws, written out here so the tests do not read the
// production tables they check.
type settingsDelCase struct {
	code, nameKey, descKey, kind string
}

var settingsDelServiceCases = []settingsDelCase{
	{"sj", "settings_del_svc_joined", "settings_del_desc_joined", deletion.KindJoined},
	{"sl", "settings_del_svc_left", "settings_del_desc_left", deletion.KindLeft},
	{"sp", "settings_del_svc_photo", "settings_del_desc_photo", deletion.KindPhoto},
	{"sh", "settings_del_svc_title", "settings_del_desc_title", deletion.KindTitle},
	{"sn", "settings_del_svc_pinned", "settings_del_desc_pinned", deletion.KindPinned},
	{"sv", "settings_del_svc_video", "settings_del_desc_video", deletion.KindVideo},
	{"sf", "settings_del_svc_forum", "settings_del_desc_forum", deletion.KindForum},
	{"sb", "settings_del_svc_boost", "settings_del_desc_boost", deletion.KindBoost},
	{"sa", "settings_del_svc_timer", "settings_del_desc_timer", deletion.KindTimer},
	{"sk", "settings_del_svc_background", "settings_del_desc_background", deletion.KindBackground},
	{"sg", "settings_del_svc_gifts", "settings_del_desc_gifts", deletion.KindGiveaway},
	{"so", "settings_del_svc_other", "settings_del_desc_other", deletion.KindOther},
}

var settingsDelBotCases = []settingsDelCase{
	{"bw", "settings_del_bot_welcome", "settings_del_desc_welcome", deletion.KindBotWelcome},
	{"bg", "settings_del_bot_goodbye", "settings_del_desc_goodbye", deletion.KindBotGoodbye},
	{"br", "settings_del_bot_rules", "settings_del_desc_rules", deletion.KindBotRules},
	{"ba", "settings_del_bot_auto", "settings_del_desc_auto", deletion.KindBotAuto},
	{"bm", "settings_del_bot_manual", "settings_del_desc_manual", deletion.KindBotManual},
}

var (
	settingsDelServicePresetValues = []string{"n", "0", "10", "60", "600", "3600", "21600", "86400"}
	settingsDelBotPresetValues     = []string{"n", "60", "600", "3600", "21600", "86400"}
)

// settingsDelNoRows fails the test when the chat has a rule row, a greetings row or a queue row: the
// page never creates one by being opened or drawn.
func settingsDelNoRows(t *testing.T, chatID int64) {
	t.Helper()
	for name, model := range map[string]any{
		"delete rule": &models.DeleteRule{}, "greetings": &models.GreetingSettings{},
		"scheduled deletion": &models.ScheduledDeletion{},
	} {
		var count int64
		if err := db.DB.Model(model).Where("chat_id = ?", chatID).Count(&count).Error; err != nil {
			t.Fatalf("count %s rows: %v", name, err)
		}
		if count != 0 {
			t.Errorf("the page left %d %s rows behind", count, name)
		}
	}
}

// settingsDelWantRowFields fails unless the button carries exactly a=action, t=code and the owner.
func settingsDelWantRowFields(t *testing.T, button gotgbot.InlineKeyboardButton, action, code string) {
	t.Helper()
	fields := settingsButtonFields(t, button)
	if fields["a"] != action || fields["t"] != code || len(fields) != 3 {
		t.Errorf("button %q fields = %v, want exactly a=%s, t=%s and the owner", button.Text, fields, action, code)
	}
}

// settingsDelWantBack fails unless the last row of the keyboard is Back with the given fields and Close.
func settingsDelWantBack(t *testing.T, keyboard [][]gotgbot.InlineKeyboardButton, want map[string]string) {
	t.Helper()
	if len(keyboard) == 0 {
		t.Fatal("empty keyboard")
	}
	nav := keyboard[len(keyboard)-1]
	if len(nav) != 2 || !containsMarker(nav[0].Text, "settings_btn_back") || !containsMarker(nav[1].Text, "settings_btn_close") {
		t.Fatalf("last row = %+v, want Back and Close", nav)
	}
	fields := settingsButtonFields(t, nav[0])
	for key, value := range want {
		if fields[key] != value {
			t.Errorf("Back fields = %v, want %s=%s", fields, key, value)
		}
	}
	if len(fields) != len(want)+1 {
		t.Errorf("Back fields = %v, want exactly %v and the owner", fields, want)
	}
}

func TestSettingsDeletionOpensFromMain(t *testing.T) {
	settingsDelMenus(t, func(t *testing.T, m *settingsDelMenu) {
		wantShape := "s|l / c|f / w|g / r|m / d / a / close"
		if m.private {
			wantShape = "s|l / c|f / w|g / r|m / d / a / groups|close"
		}
		if got := settingsMainShape(t, m.mainKeyboard); got != wantShape {
			t.Errorf("the owner's Main = %q, want %q", got, wantShape)
		}
		if rows := len(m.mainKeyboard); rows != 7 {
			t.Errorf("Main has %d rows, want 7", rows)
		}
		settingsDelWantLine(t, m.mainText, "settings_page_del", "🧹", staffMarker("settings_state_off_plain"))
		cell, ok := settingsPageButton(t, m.mainKeyboard, "d")
		if !ok || !strings.HasPrefix(cell.Text, "🧹 ") || !containsMarker(cell.Text, "settings_page_del") {
			t.Fatalf("Main has no 🧹 Deletion cell: %+v", m.mainKeyboard)
		}

		// The hub.
		m.pressButton(cell)
		text, keyboard := m.last()
		if !containsMarker(text, "settings_del_intro") || !containsMarker(text, "settings_del_hint_time") {
			t.Errorf("hub text = %q, want the intro and the time hint", text)
		}
		settingsDelWantLine(t, text, "settings_del_sec_service", "🔔", staffMarker("settings_state_off_plain"))
		settingsDelWantLine(t, text, "settings_del_sec_bot", "🤖", staffMarker("settings_state_off_plain"))
		if containsMarker(text, "settings_del_no_right") || containsMarker(text, "settings_del_right_unknown") {
			t.Errorf("hub of a bot that can delete carries a right line: %q", text)
		}
		if got := settingsDelWidths(keyboard); fmt.Sprint(got) != "[2 2]" {
			t.Errorf("hub row widths = %v, want [2 2]", got)
		}
		for i, code := range []string{"s", "b"} {
			settingsDelWantRowFields(t, keyboard[0][i], "dl", code)
		}
		if fields := settingsButtonFields(t, keyboard[1][0]); fields["a"] != "bk" {
			t.Errorf("the hub's Back = %v, want the core Back to Main", fields)
		}

		// The Service messages list.
		text, keyboard = m.open("dl", "s")
		if !containsMarker(text, "settings_del_list_svc") || !containsMarker(text, "settings_del_hint_cleanservice") ||
			!containsMarker(text, "settings_del_hint_time") {
			t.Errorf("service list text = %q, want its intro and both hints", text)
		}
		lastAt := -1
		for _, tc := range settingsDelServiceCases {
			settingsDelWantLine(t, text, tc.nameKey, "➖", staffMarker("settings_del_state_keep"))
			at := strings.Index(text, staffMarker(tc.nameKey))
			if at < lastAt {
				t.Errorf("line of %s is out of table order", tc.code)
			}
			lastAt = at
		}
		if got := settingsDelWidths(keyboard); fmt.Sprint(got) != "[2 2 2 2 2 2 2]" {
			t.Errorf("service list row widths = %v, want seven rows of two", got)
		}
		cells := settingsAllButtons(keyboard[:6])
		for i, tc := range settingsDelServiceCases {
			if !strings.HasPrefix(cells[i].Text, "➖ ") || !containsMarker(cells[i].Text, tc.nameKey) {
				t.Errorf("cell %d = %q, want ➖ and %s", i, cells[i].Text, tc.nameKey)
			}
			settingsDelWantRowFields(t, cells[i], "dr", tc.code)
		}
		settingsDelWantBack(t, keyboard, map[string]string{"a": "op", "p": "d"})

		// A row screen, and Back to its list.
		text, keyboard = m.open("dr", "sn")
		for _, key := range []string{"settings_del_svc_pinned", "settings_del_desc_pinned", "settings_del_currently", "settings_del_hint_time"} {
			if !containsMarker(text, key) {
				t.Errorf("pinned row screen lacks %s: %q", key, text)
			}
		}
		if containsMarker(text, "settings_del_hint_cleanservice") {
			t.Errorf("pinned row screen carries the /cleanservice hint: %q", text)
		}
		if !strings.Contains(text, "➖ "+staffMarker("settings_del_state_keep")) {
			t.Errorf("pinned row screen Currently line is not ➖ Keep: %q", text)
		}
		if got := settingsDelWidths(keyboard); fmt.Sprint(got) != "[2 3 3 2]" {
			t.Errorf("service row screen widths = %v, want [2 3 3 2]", got)
		}
		for i, button := range settingsAllButtons(keyboard[:3]) {
			fields := settingsButtonFields(t, button)
			if fields["a"] != "st" || fields["p"] != "d" || fields["t"] != "sn" || fields["v"] != settingsDelServicePresetValues[i] {
				t.Errorf("choice %d fields = %v, want st on sn with v=%s", i, fields, settingsDelServicePresetValues[i])
			}
		}
		if first := keyboard[0][0].Text; !strings.HasPrefix(first, settingsSelectedPrefix) {
			t.Errorf("Keep is not marked on an unset row: %q", first)
		}
		settingsDelWantBack(t, keyboard, map[string]string{"a": "dl", "t": "s"})
		m.pressButton(keyboard[len(keyboard)-1][0])
		text, keyboard = m.last()
		if !containsMarker(text, "settings_del_list_svc") {
			t.Errorf("Back from a row screen did not draw the service list: %q", text)
		}

		// Back from the list is the hub, Back from the hub is Main.
		m.pressButton(keyboard[len(keyboard)-1][0])
		text, keyboard = m.last()
		if !containsMarker(text, "settings_del_intro") {
			t.Errorf("Back from the list did not draw the hub: %q", text)
		}
		m.pressButton(keyboard[len(keyboard)-1][0])
		text, _ = m.last()
		if !containsMarker(text, "settings_main_hint") {
			t.Errorf("Back from the hub did not draw Main: %q", text)
		}

		// The Bot messages list and a bot row screen.
		text, keyboard = m.open("dl", "b")
		if !containsMarker(text, "settings_del_list_bot") || containsMarker(text, "settings_del_hint_cleanservice") {
			t.Errorf("bot list text = %q, want its intro and no /cleanservice hint", text)
		}
		for _, tc := range settingsDelBotCases {
			settingsDelWantLine(t, text, tc.nameKey, "➖", staffMarker("settings_state_off_plain"))
		}
		if got := settingsDelWidths(keyboard); fmt.Sprint(got) != "[2 2 1 2]" {
			t.Errorf("bot list row widths = %v, want [2 2 1 2]", got)
		}
		text, keyboard = m.open("dr", "ba")
		if !containsMarker(text, "settings_del_hint_punish_auto") || !containsMarker(text, "settings_del_desc_auto") {
			t.Errorf("auto punishments screen = %q, want its description and its mute-notice hint", text)
		}
		if got := settingsDelWidths(keyboard); fmt.Sprint(got) != "[3 3 2]" {
			t.Errorf("bot row screen widths = %v, want [3 3 2]", got)
		}
		for i, button := range settingsAllButtons(keyboard[:2]) {
			if fields := settingsButtonFields(t, button); fields["v"] != settingsDelBotPresetValues[i] {
				t.Errorf("bot choice %d v = %q, want %s", i, fields["v"], settingsDelBotPresetValues[i])
			}
		}
		settingsDelWantBack(t, keyboard, map[string]string{"a": "dl", "t": "b"})
		settingsDelNoRows(t, m.env.chat.Id)
		if m.env.botReads(m.env.chat.Id) == 0 {
			t.Error("the deletion screens never read the bot's right")
		}
	})
}

func TestSettingsDeletionMainState(t *testing.T) {
	env := newSettingsDelEnv(t)
	m := env.groupMenu()
	readsBefore := env.botReads(env.chat.Id)
	mainLine := func() string {
		t.Helper()
		m.press("a", "bk")
		text, _ := m.last()
		line, ok := settingsMainStateLine(text, "settings_page_del")
		if !ok {
			t.Fatalf("Main has no Deletion line: %q", text)
		}
		return line
	}
	wantOn := func(want int) {
		t.Helper()
		line := mainLine()
		if !strings.Contains(line, staffMarker("settings_state_del_on")+" "+fmt.Sprint(want)) {
			t.Errorf("Main line = %q, want %d on", line, want)
		}
	}

	if line := mainLine(); !strings.Contains(line, staffMarker("settings_state_off_plain")) {
		t.Errorf("no rule: line = %q, want the Off word", line)
	}
	env.setRule(deletion.KindPinned, 600)
	env.setRule(deletion.KindPhoto, 0)
	wantOn(2)
	env.setRule(deletion.KindBotWelcome, 3600)
	wantOn(3)
	// The shared flag turns joined and left on together, each by its effective state.
	env.setJoinLeft(deletion.KindJoined, deletion.JoinLeftState{Mode: deletion.JoinLeftNow})
	wantOn(5)
	env.setJoinLeft(deletion.KindJoined, deletion.JoinLeftState{Mode: deletion.JoinLeftAfter, DelaySecs: 60})
	wantOn(5)
	env.setJoinLeft(deletion.KindJoined, deletion.JoinLeftState{Mode: deletion.JoinLeftKeep})
	wantOn(3)
	// A stale delay row with the flag off is Keep, so it counts for nothing.
	if err := deletion.SetRule(context.Background(), env.chat.Id, deletion.KindLeft, 60); err != nil {
		t.Fatalf("SetRule(left) error = %v", err)
	}
	wantOn(3)

	for name, swap := range map[string]func(){
		"rules": func() {
			mreqSwap(t, &deletionRulesLookup, func(context.Context, int64) (*deletion.Rules, error) {
				return nil, errors.New("rules read failed")
			})
		},
		"flag": func() {
			mreqSwap(t, &deletionFlagLookup, func(context.Context, int64) (*models.GreetingSettings, error) {
				return nil, errors.New("flag read failed")
			})
		},
	} {
		swap()
		if line := mainLine(); !strings.Contains(line, settingsUnknownState) || strings.Contains(line, staffMarker("settings_state_del_on")) {
			t.Errorf("a failed %s read: line = %q, want the unknown mark", name, line)
		}
	}
	if reads := env.botReads(env.chat.Id) - readsBefore; reads != 0 {
		t.Errorf("Main made %d getChatMember calls about the bot, want none", reads)
	}
}

// settingsDelCounter wraps the real rule writes and counts the calls.
type settingsDelCounter struct{ sets, removes int }

func (c *settingsDelCounter) install(t *testing.T) {
	t.Helper()
	real := deletionRuleWrites
	mreqSwap(t, &deletionRuleWrites, deletionRuleWriter{
		Set: func(ctx context.Context, chatID int64, kind string, delay int) error {
			c.sets++
			return real.Set(ctx, chatID, kind, delay)
		},
		Remove: func(ctx context.Context, chatID int64, kind string) error {
			c.removes++
			return real.Remove(ctx, chatID, kind)
		},
	})
}

func TestSettingsDeletionWrites(t *testing.T) {
	settingsDelMenus(t, func(t *testing.T, m *settingsDelMenu) {
		env := m.env
		var counter settingsDelCounter
		counter.install(t)
		m.openHub()
		m.open("dl", "s")
		m.open("dr", "sn")

		// A preset stores, drops the cache key, and the screen shows the stored state with the choice marked.
		m.choose("sn", "600")
		if delay, ok := env.stored(deletion.KindPinned); !ok || delay != 600 {
			t.Fatalf("stored pinned = %d (%v), want 600", delay, ok)
		}
		rules, err := deletion.GetRules(context.Background(), env.chat.Id)
		if err != nil || rules.Delays[deletion.KindPinned] != 600 {
			t.Errorf("a read through the cache shows %+v (%v), want 600 for pinned", rules, err)
		}
		text, keyboard := m.last()
		if !strings.Contains(text, "⏳ "+staffMarker("settings_del_state_after")+" "+staffMarker("settings_del_delay_10m")) {
			t.Errorf("after 10 min the Currently line = %q", text)
		}
		marked := 0
		for _, button := range settingsAllButtons(keyboard) {
			if strings.HasPrefix(button.Text, settingsSelectedPrefix) {
				marked++
				if !containsMarker(button.Text, "settings_del_delay_10m") {
					t.Errorf("the marked choice is %q, want 10 min", button.Text)
				}
			}
		}
		if marked != 1 {
			t.Errorf("%d choices are marked, want exactly one", marked)
		}

		// The stored state again writes nothing.
		m.choose("sn", "600")
		if counter.sets != 1 || counter.removes != 0 {
			t.Errorf("writes after the repeated press: %d sets, %d removes, want 1 and 0", counter.sets, counter.removes)
		}

		// Delete now stores a Go 0, Keep removes the row.
		m.choose("sn", "0")
		if delay, ok := env.stored(deletion.KindPinned); !ok || delay != 0 {
			t.Errorf("stored pinned after Delete now = %d (%v), want a stored 0", delay, ok)
		}
		text, _ = m.last()
		if !strings.Contains(text, "🗑 "+staffMarker("settings_del_state_now")) {
			t.Errorf("Delete now Currently line = %q", text)
		}
		m.choose("sn", "n")
		if _, ok := env.stored(deletion.KindPinned); ok {
			t.Error("Keep left the pinned row stored")
		}
		text, _ = m.last()
		if !strings.Contains(text, "➖ "+staffMarker("settings_del_state_keep")) {
			t.Errorf("Keep Currently line = %q", text)
		}

		// A bot row stores minutes and reads Off when removed.
		m.open("dr", "bg")
		m.choose("bg", "3600")
		if delay, ok := env.stored(deletion.KindBotGoodbye); !ok || delay != 3600 {
			t.Errorf("stored goodbye = %d (%v), want 3600", delay, ok)
		}
		m.choose("bg", "n")
		if _, ok := env.stored(deletion.KindBotGoodbye); ok {
			t.Error("Off left the goodbye row stored")
		}

		// Joined goes through the shared flag, as /cleanservice does.
		m.open("dr", "sj")
		m.choose("sj", "0")
		settings, err := deletionFlagLookup(context.Background(), env.chat.Id)
		if err != nil || !settings.ShouldCleanService {
			t.Errorf("after joined Delete now the flag = %+v (%v), want on", settings, err)
		}
		if _, ok := env.stored(deletion.KindJoined); ok {
			t.Error("joined Delete now left a delay row")
		}
		m.choose("sj", "600")
		if delay, ok := env.stored(deletion.KindJoined); !ok || delay != 600 {
			t.Errorf("stored joined = %d (%v), want 600", delay, ok)
		}
		m.choose("sj", "n")
		settings, err = deletionFlagLookup(context.Background(), env.chat.Id)
		if err != nil || settings.ShouldCleanService {
			t.Errorf("after joined Keep the flag = %+v (%v), want off", settings, err)
		}
	})
}

func TestSettingsDeletionHidden(t *testing.T) {
	t.Run("a Staff Group has no Deletion cell and no line", func(t *testing.T) {
		env := newSettingsDelEnv(t)
		env.makeStaff()
		var lookups int
		real := deletionStaffLookup
		mreqSwap(t, &deletionStaffLookup, func(chatID int64) (*models.StaffGroup, error) {
			lookups++
			return real(chatID)
		})
		m := env.groupMenu()
		if _, ok := settingsPageButton(t, m.mainKeyboard, "d"); ok {
			t.Errorf("Main in a Staff Group has a Deletion cell: %+v", m.mainKeyboard)
		}
		if _, ok := settingsMainStateLine(m.mainText, "settings_page_del"); ok {
			t.Errorf("Main in a Staff Group has a Deletion line: %q", m.mainText)
		}
		// Hidden read the Staff Group row once; MainState, which reads it too, never ran.
		if lookups != 1 {
			t.Errorf("the Main draw made %d Staff Group reads, want 1 (Hidden only, MainState skipped)", lookups)
		}

		// A forged open of the page is refused and edits nothing.
		edits, botReads, answers := m.editCount(), env.botReads(env.chat.Id), env.answerCount()
		m.press("a", "op", "p", "d")
		if text, alert := env.lastAnswer(); text != staffMarker("common_callback_invalid_request") || !alert {
			t.Errorf("a forged open answered %q alert=%v, want the invalid-request alert", text, alert)
		}
		if env.answerCount() != answers+1 {
			t.Errorf("the forged open was answered %d times, want once", env.answerCount()-answers)
		}
		if m.editCount() != edits {
			t.Error("a forged open of the page edited the menu")
		}
		if env.botReads(env.chat.Id) != botReads {
			t.Error("a forged open made a getChatMember call about the bot")
		}
	})

	t.Run("an ordinary group draws it and reads the Staff Group row twice", func(t *testing.T) {
		env := newSettingsDelEnv(t)
		var lookups int
		real := deletionStaffLookup
		mreqSwap(t, &deletionStaffLookup, func(chatID int64) (*models.StaffGroup, error) {
			lookups++
			return real(chatID)
		})
		m := env.groupMenu()
		if _, ok := settingsPageButton(t, m.mainKeyboard, "d"); !ok {
			t.Errorf("Main has no Deletion cell: %+v", m.mainKeyboard)
		}
		if lookups != 2 {
			t.Errorf("the Main draw made %d Staff Group reads, want 2 (Hidden and MainState)", lookups)
		}
	})
}

// settingsDelForged are presses of the page's buttons that a Staff Group must refuse.
var settingsDelForged = [][]string{
	{"a", "dl", "t", "s"},
	{"a", "dr", "t", "sn"},
	{"a", "st", "p", "d", "t", "sn", "v", "600"},
	{"a", "st", "p", "d", "t", "sj", "v", "0"},
	{"a", "st", "p", "d", "t", "bw", "v", "n"},
}

func TestSettingsDeletionStaffGroup(t *testing.T) {
	t.Run("a forged press in a Staff Group is invalid and writes nothing", func(t *testing.T) {
		env := newSettingsDelEnv(t)
		m := env.groupMenu()
		env.makeStaff()
		var counter settingsDelCounter
		counter.install(t)
		for _, kv := range settingsDelForged {
			edits, reads, answers := m.editCount(), env.botReads(env.chat.Id), env.answerCount()
			m.press(kv...)
			if text, alert := env.lastAnswer(); text != staffMarker("common_callback_invalid_request") || !alert {
				t.Errorf("%v answered %q alert=%v, want the invalid-request alert", kv, text, alert)
			}
			if got := env.answerCount() - answers; got != 1 {
				t.Errorf("%v was answered %d times, want once", kv, got)
			}
			if m.editCount() != edits {
				t.Errorf("%v edited the menu", kv)
			}
			if env.botReads(env.chat.Id) != reads {
				t.Errorf("%v read the bot's right", kv)
			}
		}
		if counter.sets != 0 || counter.removes != 0 {
			t.Errorf("a Staff Group press wrote: %d sets, %d removes", counter.sets, counter.removes)
		}
		settingsDelNoRows(t, env.chat.Id)
	})

	t.Run("an unreadable Staff Group row is the cannot-open edit with no write", func(t *testing.T) {
		env := newSettingsDelEnv(t)
		m := env.groupMenu()
		mreqSwap(t, &deletionStaffLookup, func(int64) (*models.StaffGroup, error) { return nil, errors.New("staff read failed") })
		var counter settingsDelCounter
		counter.install(t)
		for _, kv := range append([][]string{{"a", "op", "p", "d"}}, settingsDelForged...) {
			reads := env.botReads(env.chat.Id)
			m.press(kv...)
			text, keyboard := m.last()
			if text != staffMarker("settings_cannot_open") || len(keyboard) != 0 {
				t.Errorf("%v drew %q (%d rows), want the cannot-open edit", kv, text, len(keyboard))
			}
			if answer, alert := env.lastAnswer(); answer != staffMarker("settings_cannot_open_alert") || !alert {
				t.Errorf("%v answered %q alert=%v, want the cannot-open alert", kv, answer, alert)
			}
			if env.botReads(env.chat.Id) != reads {
				t.Errorf("%v read the bot's right", kv)
			}
		}
		if counter.sets != 0 || counter.removes != 0 {
			t.Errorf("an unreadable Staff Group row let a write through: %d sets, %d removes", counter.sets, counter.removes)
		}
		settingsDelNoRows(t, env.chat.Id)
	})

	t.Run("a group linked to a Staff Group draws the page and never names it", func(t *testing.T) {
		env := newSettingsDelEnv(t)
		staffChat := uniqueModuleChatID()
		const staffTitle = "Zed Staff Title"
		staffCleanup(t, staffChat, env.chat.Id)
		if _, err := staff.CreateStaffGroup(staffChat, env.owner.Id, staffTitle); err != nil {
			t.Fatalf("CreateStaffGroup error = %v", err)
		}
		if err := staff.CreateLink(&models.StaffGroupLink{
			GroupChatID: env.chat.Id, StaffChatID: staffChat, OwnerUserID: env.owner.Id, GroupTitle: "Linked",
		}); err != nil {
			t.Fatalf("CreateLink error = %v", err)
		}
		m := env.groupMenu()
		if _, ok := settingsPageButton(t, m.mainKeyboard, "d"); !ok {
			t.Fatalf("a linked group's Main has no Deletion cell: %+v", m.mainKeyboard)
		}
		screens := []string{m.mainText}
		hub, _ := m.openHub()
		list, _ := m.open("dl", "s")
		row, _ := m.open("dr", "sn")
		botList, _ := m.open("dl", "b")
		screens = append(screens, hub, list, row, botList)
		for i, text := range screens {
			if strings.Contains(text, staffTitle) || strings.Contains(text, fmt.Sprint(staffChat)) {
				t.Errorf("screen %d names the Staff Group: %q", i, text)
			}
		}
		for _, text := range screens[1:] {
			if containsMarker(text, "settings_cannot_open") {
				t.Errorf("a linked group could not open the page: %q", text)
			}
		}
	})
}

func TestSettingsDeletionReadsAndWritesFail(t *testing.T) {
	t.Run("a failed rules read is the cannot-open edit and creates no row", func(t *testing.T) {
		env := newSettingsDelEnv(t)
		m := env.groupMenu()
		mreqSwap(t, &deletionRulesLookup, func(context.Context, int64) (*deletion.Rules, error) {
			return nil, errors.New("rules read failed")
		})
		for _, kv := range [][]string{{"a", "op", "p", "d"}, {"a", "dl", "t", "s"}, {"a", "dl", "t", "b"}, {"a", "dr", "t", "sn"}, {"a", "dr", "t", "bw"}} {
			m.press(kv...)
			text, keyboard := m.last()
			if text != staffMarker("settings_cannot_open") || len(keyboard) != 0 {
				t.Errorf("%v drew %q (%d rows), want the cannot-open edit", kv, text, len(keyboard))
			}
		}
		settingsDelNoRows(t, env.chat.Id)
	})

	t.Run("a failed flag read stops the screens that show joined and left only", func(t *testing.T) {
		env := newSettingsDelEnv(t)
		m := env.groupMenu()
		mreqSwap(t, &deletionFlagLookup, func(context.Context, int64) (*models.GreetingSettings, error) {
			return nil, errors.New("flag read failed")
		})
		for _, kv := range [][]string{{"a", "op", "p", "d"}, {"a", "dl", "t", "s"}, {"a", "dr", "t", "sj"}} {
			m.press(kv...)
			if text, _ := m.last(); text != staffMarker("settings_cannot_open") {
				t.Errorf("%v drew %q, want the cannot-open edit", kv, text)
			}
		}
		for _, kv := range [][]string{{"a", "dl", "t", "b"}, {"a", "dr", "t", "sn"}} {
			m.press(kv...)
			if text, _ := m.last(); containsMarker(text, "settings_cannot_open") {
				t.Errorf("%v needs no flag but drew the cannot-open edit", kv)
			}
		}
		settingsDelNoRows(t, env.chat.Id)
	})

	t.Run("a failed write redraws the stored state with the notice and answers once", func(t *testing.T) {
		env := newSettingsDelEnv(t)
		m := env.groupMenu()
		m.open("dr", "sn")
		mreqSwap(t, &deletionRuleWrites, deletionRuleWriter{
			Set:    func(context.Context, int64, string, int) error { return errors.New("write failed") },
			Remove: func(context.Context, int64, string) error { return errors.New("write failed") },
		})
		answers := env.answerCount()
		m.choose("sn", "600")
		text, _ := m.last()
		if !strings.Contains(text, "⚠️ "+staffMarker("common_settings_save_failed")) {
			t.Errorf("a failed write drew %q, want the save-failed notice", text)
		}
		if !strings.Contains(text, "➖ "+staffMarker("settings_del_state_keep")) {
			t.Errorf("a failed write drew %q, want the old state (Keep)", text)
		}
		if got := env.answerCount() - answers; got != 1 {
			t.Errorf("a failed write was answered %d times, want once", got)
		}
		if _, ok := env.stored(deletion.KindPinned); ok {
			t.Error("a failed write stored a row")
		}
	})
}
