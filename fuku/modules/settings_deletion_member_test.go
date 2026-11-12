//go:build testtools

package modules

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// settingsDelWantCurrently fails unless the row screen text has the Currently line with the icon and the
// state words in it.
func settingsDelWantCurrently(t *testing.T, text, icon string, words ...string) {
	t.Helper()
	currently := ""
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, staffMarker("settings_del_currently")) {
			currently = line
		}
	}
	if currently == "" {
		t.Errorf("no Currently line in %q", text)
		return
	}
	for _, want := range append([]string{icon}, words...) {
		if !strings.Contains(currently, want) {
			t.Errorf("Currently line %q lacks %q", currently, want)
		}
	}
}

// TestSettingsDeletionMemberOpens is the tracer: the owner opens Member messages from Main and turns
// Commands on and off, stored and redrawn.
func TestSettingsDeletionMemberOpens(t *testing.T) {
	settingsDelMenus(t, func(t *testing.T, m *settingsDelMenu) {
		env := m.env
		hub, keyboard := m.openHub()
		settingsDelWantLine(t, hub, "settings_del_sec_member", "💬", staffMarker("settings_state_off_plain"))
		if got := settingsDelWidths(keyboard); len(got) != 3 || got[0] != 2 || got[1] != 1 || got[2] != 2 {
			t.Fatalf("hub row widths = %v, want [2 1 2]", got)
		}
		settingsDelWantRowFields(t, keyboard[1][0], "dl", "m")
		if !strings.HasPrefix(keyboard[1][0].Text, "💬 ") || !containsMarker(keyboard[1][0].Text, "settings_del_sec_member") {
			t.Errorf("the Member cell = %q, want 💬 and the section name", keyboard[1][0].Text)
		}

		list, keyboard := m.open("dl", "m")
		if !containsMarker(list, "settings_del_list_mem") || !containsMarker(list, "settings_del_hint_time") {
			t.Errorf("member list text = %q, want its intro and the time hint", list)
		}
		for _, key := range []string{"settings_del_mem_commands", "settings_del_mem_edited", "settings_del_mem_self"} {
			settingsDelWantLine(t, list, key, "➖", staffMarker("settings_state_off_plain"))
		}
		if got := settingsDelWidths(keyboard); len(got) != 3 || got[0] != 2 || got[1] != 1 || got[2] != 2 {
			t.Errorf("member list row widths = %v, want [2 1 2]", got)
		}
		settingsDelWantBack(t, keyboard, map[string]string{"a": "op", "p": "d"})

		text, keyboard := m.open("dr", "mc")
		for _, key := range []string{"settings_del_mem_commands", "settings_del_desc_commands", "settings_del_hint_commands", "settings_del_currently"} {
			if !containsMarker(text, key) {
				t.Errorf("Commands screen lacks %s: %q", key, text)
			}
		}
		if containsMarker(text, "settings_del_hint_time") {
			t.Errorf("the Commands screen carries the time hint, which is false for it: %q", text)
		}
		if got := settingsDelWidths(keyboard); len(got) != 2 || got[0] != 2 || got[1] != 2 {
			t.Errorf("Commands row widths = %v, want [2 2]", got)
		}
		settingsDelWantBack(t, keyboard, map[string]string{"a": "dl", "t": "m"})

		m.choose("mc", "0")
		if delay, ok := env.stored(deletion.KindMemberCommands); !ok || delay != 0 {
			t.Fatalf("stored member:commands = %d (%v), want a stored 0", delay, ok)
		}
		text, keyboard = m.last()
		settingsDelWantCurrently(t, text, "🗑", staffMarker("settings_del_state_on"))
		if marked := settingsAllButtons(keyboard[:1]); len(marked) != 2 ||
			!strings.HasPrefix(marked[1].Text, settingsSelectedPrefix) || !containsMarker(marked[1].Text, "settings_del_state_on") {
			t.Errorf("the choices after On = %+v, want On marked", marked)
		}

		hub, _ = m.openHub()
		settingsDelWantLine(t, hub, "settings_del_sec_member", "💬", staffMarker("settings_state_del_on")+" 1")
		list, _ = m.open("dl", "m")
		settingsDelWantLine(t, list, "settings_del_mem_commands", "🗑", staffMarker("settings_del_state_on"))
		settingsDelWantLine(t, list, "settings_del_mem_edited", "➖", staffMarker("settings_state_off_plain"))
		settingsDelWantLine(t, list, "settings_del_mem_self", "➖", staffMarker("settings_state_off_plain"))

		m.open("dr", "mc")
		m.choose("mc", "n")
		if _, ok := env.stored(deletion.KindMemberCommands); ok {
			t.Error("Off left the member:commands row stored")
		}
		text, _ = m.last()
		settingsDelWantCurrently(t, text, "➖", staffMarker("settings_state_off_plain"))
	})
}

// settingsDelCardOf returns the card's buttons of the menu's last screen: Yes and No.
func settingsDelCardOf(t *testing.T, m *settingsDelMenu) (yes, no gotgbot.InlineKeyboardButton) {
	t.Helper()
	_, keyboard := m.last()
	if len(keyboard) != 2 || len(keyboard[0]) != 2 || len(keyboard[1]) != 2 {
		t.Fatalf("the last screen is not a card: widths %v", settingsDelWidths(keyboard))
	}
	return keyboard[0][0], keyboard[0][1]
}

// settingsDelWantNoCard fails when the menu's last screen is a card.
func settingsDelWantNoCard(t *testing.T, m *settingsDelMenu, what string) {
	t.Helper()
	text, keyboard := m.last()
	if containsMarker(text, "settings_del_confirm_self") || len(settingsDelWidths(keyboard)) < 3 {
		t.Errorf("%s drew a card: %q", what, text)
	}
}

func TestSettingsDeletionMemberWrites(t *testing.T) {
	settingsDelMenus(t, func(t *testing.T, m *settingsDelMenu) {
		env := m.env
		var counter settingsDelCounter
		counter.install(t)
		other := uniqueModuleChatID()
		deletionCleanup(t, other)

		// answered presses a choice and checks it was answered exactly once.
		answered := func(code, value string) {
			t.Helper()
			before := env.answerCount()
			m.choose(code, value)
			if got := env.answerCount() - before; got != 1 {
				t.Errorf("%s=%s was answered %d times, want once", code, value, got)
			}
		}

		// Edited messages writes at once, never draws a card and reads Grace.
		answered("me", "60")
		if delay, ok := env.stored(deletion.KindMemberEdited); !ok || delay != 60 {
			t.Fatalf("stored member:edited = %d (%v), want 60", delay, ok)
		}
		text, keyboard := m.last()
		settingsDelWantCurrently(t, text, "⏳", staffMarker("settings_del_state_grace"), staffMarker("settings_del_delay_1m"))
		if !containsMarker(text, "settings_del_hint_grace") || containsMarker(text, "settings_del_hint_time") {
			t.Errorf("the Edited screen time line is wrong: %q", text)
		}
		var marks []string
		for _, button := range settingsAllButtons(keyboard) {
			if strings.HasPrefix(button.Text, settingsSelectedPrefix) {
				marks = append(marks, settingsButtonFields(t, button)["v"])
			}
		}
		if len(marks) != 1 || marks[0] != "60" {
			t.Errorf("marked choices = %v, want [60]", marks)
		}
		answered("me", "60")
		if counter.sets != 1 {
			t.Errorf("a press of the stored value wrote: %d sets, want 1", counter.sets)
		}
		answered("me", "n")
		if _, ok := env.stored(deletion.KindMemberEdited); ok {
			t.Error("Off left the member:edited row stored")
		}
		text, _ = m.last()
		settingsDelWantCurrently(t, text, "➖", staffMarker("settings_state_off_plain"))

		// Commands: On stores a 0, twice is one write.
		answered("mc", "0")
		answered("mc", "0")
		if delay, ok := env.stored(deletion.KindMemberCommands); !ok || delay != 0 || counter.sets != 2 {
			t.Errorf("stored member:commands = %d (%v) after %d sets, want a stored 0 after 2", delay, ok, counter.sets)
		}

		// Self-destruct Off with a rule stored removes it and cancels this chat's waiting rows only.
		env.setRule(deletion.KindMemberSelfDestruct, 600)
		seedDeletionRow(t, env.chat.Id, 91001, time.Now().Add(time.Hour), time.Now(), deletion.KindMemberSelfDestruct)
		seedDeletionRow(t, env.chat.Id, 91002, time.Now().Add(time.Hour), time.Now(), deletion.KindBotWelcome)
		seedDeletionRow(t, other, 91003, time.Now().Add(time.Hour), time.Now(), deletion.KindMemberSelfDestruct)
		removes := counter.removes
		answered("ms", "n")
		if _, ok := env.stored(deletion.KindMemberSelfDestruct); ok {
			t.Error("Off left the member:selfdestruct row stored")
		}
		wantQueued(t, env.chat.Id, map[int64]string{91002: models.ScheduledDeletionPending})
		wantQueued(t, other, map[int64]string{91003: models.ScheduledDeletionPending})
		settingsDelWantNoCard(t, m, "Self-destruct Off")
		text, _ = m.last()
		settingsDelWantCurrently(t, text, "➖", staffMarker("settings_state_off_plain"))
		if counter.removes != removes+1 {
			t.Errorf("Off made %d removes, want 1", counter.removes-removes)
		}

		// Off with no rule writes nothing.
		sets, removes := counter.sets, counter.removes
		answered("ms", "n")
		if counter.sets != sets || counter.removes != removes {
			t.Error("Off with no rule wrote")
		}
		settingsDelWantNoCard(t, m, "Self-destruct Off with no rule")

		// A failed write redraws the stored state with the save-failed notice.
		real := deletionRuleWrites
		mreqSwap(t, &deletionRuleWrites, deletionRuleWriter{
			Set: func(context.Context, int64, string, int) error { return errors.New("write failed") }, Remove: real.Remove,
		})
		answered("me", "300")
		if _, ok := env.stored(deletion.KindMemberEdited); ok {
			t.Error("a failed write stored the rule")
		}
		text, _ = m.last()
		if !containsMarker(text, "common_settings_save_failed") {
			t.Errorf("a failed write drew %q, want the save-failed notice", text)
		}
		settingsDelWantCurrently(t, text, "➖", staffMarker("settings_state_off_plain"))
	})
}

func TestSettingsDeletionMemberCard(t *testing.T) {
	settingsDelMenus(t, func(t *testing.T, m *settingsDelMenu) {
		env := m.env
		var counter settingsDelCounter
		counter.install(t)

		// From Off a delay writes nothing and draws the card in place of the row screen.
		m.open("dr", "ms")
		m.choose("ms", "600")
		if _, ok := env.stored(deletion.KindMemberSelfDestruct); ok || counter.sets != 0 {
			t.Fatalf("the choice wrote before the card was confirmed: %d sets", counter.sets)
		}
		text, keyboard := m.last()
		if !containsMarker(text, "settings_del_confirm_self") || !containsMarker(text, "settings_del_delay_10m") {
			t.Errorf("the card text = %q, want the question with the chosen delay", text)
		}
		if containsMarker(text, "settings_del_currently") || containsMarker(text, "settings_del_hint_time") {
			t.Errorf("the card carries a Currently or time line: %q", text)
		}
		yes, no := settingsDelCardOf(t, m)
		yesFields := settingsButtonFields(t, yes)
		if yesFields["a"] != "dy" || yesFields["t"] != "ms" || yesFields["v"] != "600" || yesFields["e"] == "" {
			t.Errorf("Yes fields = %v, want a=dy, t=ms, v=600 and an expiry", yesFields)
		}
		if noFields := settingsButtonFields(t, no); noFields["a"] != "no" || noFields["p"] != "d" {
			t.Errorf("No fields = %v, want a=no, p=d", noFields)
		}
		settingsDelWantBack(t, keyboard, map[string]string{"a": "bk"})

		// Yes writes, redraws the row with no notice, and a second Yes writes nothing more.
		m.pressButton(yes)
		if delay, ok := env.stored(deletion.KindMemberSelfDestruct); !ok || delay != 600 || counter.sets != 1 {
			t.Fatalf("after Yes stored = %d (%v) with %d sets, want 600 and 1", delay, ok, counter.sets)
		}
		text, _ = m.last()
		settingsDelWantCurrently(t, text, "⏳", staffMarker("settings_del_state_after"), staffMarker("settings_del_delay_10m"))
		if containsMarker(text, "settings_card_expired") || strings.Contains(text, "⚠️") {
			t.Errorf("a confirmed Yes drew a notice: %q", text)
		}
		m.pressButton(yes)
		if counter.sets != 1 {
			t.Errorf("a second Yes wrote: %d sets, want 1", counter.sets)
		}

		// Another delay draws the card again, the stored delay does not.
		m.choose("ms", "3600")
		settingsDelCardOf(t, m)
		if delay, _ := env.stored(deletion.KindMemberSelfDestruct); delay != 600 || counter.sets != 1 {
			t.Errorf("the card of a new delay wrote: stored %d, %d sets", delay, counter.sets)
		}
		m.choose("ms", "600")
		settingsDelWantNoCard(t, m, "the stored delay")
		if counter.sets != 1 {
			t.Errorf("a press of the stored delay wrote: %d sets", counter.sets)
		}

		// No redraws the hub and writes nothing; Back opens Main.
		m.choose("ms", "3600")
		_, no = settingsDelCardOf(t, m)
		m.pressButton(no)
		if hub, _ := m.last(); !containsMarker(hub, "settings_del_intro") || counter.sets != 1 {
			t.Errorf("No drew %q with %d sets, want the hub and no write", hub, counter.sets)
		}
		m.open("dr", "ms")
		m.choose("ms", "3600")
		_, keyboard = m.last()
		m.pressButton(keyboard[len(keyboard)-1][0])
		if main, _ := m.last(); !containsMarker(main, "settings_main_hint") {
			t.Errorf("Back from the card drew %q, want Main", main)
		}
		if counter.sets != 1 {
			t.Errorf("leaving the card wrote: %d sets", counter.sets)
		}
	})

	t.Run("an expired Yes writes nothing and says so", func(t *testing.T) {
		mreqSwap(t, &settingsCardLifetime, -2*time.Minute)
		env := newSettingsDelEnv(t)
		m := env.groupMenu()
		var counter settingsDelCounter
		counter.install(t)
		m.choose("ms", "600")
		yes, _ := settingsDelCardOf(t, m)
		m.pressButton(yes)
		text, keyboard := m.last()
		if !containsMarker(text, "settings_card_expired") {
			t.Errorf("an expired Yes drew %q, want the expired notice", text)
		}
		settingsDelWantCurrently(t, text, "➖", staffMarker("settings_state_off_plain"))
		if got := settingsDelWidths(keyboard); len(got) != 4 {
			t.Errorf("an expired Yes drew widths %v, want the row screen", got)
		}
		if _, ok := env.stored(deletion.KindMemberSelfDestruct); ok || counter.sets != 0 {
			t.Errorf("an expired Yes wrote: %d sets", counter.sets)
		}
	})

	t.Run("a Yes after the presser lost can_change_info is refused", func(t *testing.T) {
		env := newSettingsDelEnv(t)
		listEditAdmin(env.settingsEnv, env.chat, env.admin, true)
		m := env.groupMenuOf(env.admin)
		var counter settingsDelCounter
		counter.install(t)
		m.choose("ms", "600")
		yes, _ := settingsDelCardOf(t, m)
		listEditAdmin(env.settingsEnv, env.chat, env.admin, false)
		edits, answers := m.editCount(), env.answerCount()
		m.pressButton(yes)
		if text, alert := env.lastAnswer(); text != staffMarker("settings_no_change_info") || !alert {
			t.Errorf("a demoted Yes answered %q alert=%v, want settings_no_change_info", text, alert)
		}
		if m.editCount() != edits || env.answerCount()-answers != 1 {
			t.Error("a refused Yes edited the menu or was not answered exactly once")
		}
		if _, ok := env.stored(deletion.KindMemberSelfDestruct); ok || counter.sets != 0 {
			t.Errorf("a refused Yes wrote: %d sets", counter.sets)
		}
	})

	t.Run("a Yes in a Staff Group is the generic invalid alert", func(t *testing.T) {
		env := newSettingsDelEnv(t)
		m := env.groupMenu()
		m.choose("ms", "600")
		yes, _ := settingsDelCardOf(t, m)
		env.makeStaff()
		var counter settingsDelCounter
		counter.install(t)
		edits, reads := m.editCount(), env.botReads(env.chat.Id)
		m.pressButton(yes)
		if text, alert := env.lastAnswer(); text != staffMarker("common_callback_invalid_request") || !alert {
			t.Errorf("a Yes in a Staff Group answered %q alert=%v, want the invalid-request alert", text, alert)
		}
		if m.editCount() != edits || env.botReads(env.chat.Id) != reads || counter.sets != 0 {
			t.Error("a Yes in a Staff Group edited, read the bot's right or wrote")
		}
	})
}

// TestSettingsDeletionMemberCodesUnique holds the codes of the new section, its rows and its action
// apart from every other section code, row code and action code (registerSettingsPage already panics
// at init on a duplicate action code; the section and row tables have no such guard).
func TestSettingsDeletionMemberCodesUnique(t *testing.T) {
	seen := map[string]string{}
	add := func(code, what string) {
		t.Helper()
		if code == "" {
			t.Errorf("%s has an empty code", what)
			return
		}
		if other, dup := seen[code]; dup {
			t.Errorf("the code %q is both %s and %s", code, other, what)
		}
		seen[code] = what
	}
	for _, section := range settingsDelSections {
		add(section.Code, "section "+section.Code)
		for _, row := range section.Rows {
			add(row.Code, "row "+row.Code)
			if row.Code[0] != section.Code[0] {
				t.Errorf("row %s does not start with its section code %s", row.Code, section.Code)
			}
		}
	}
	for _, core := range settingsCoreActions {
		add(core, "core action "+core)
	}
	for code := range settingsActions {
		add(code, "action "+code)
	}
	for _, code := range []string{"m", "mc", "me", "ms", "dy"} {
		if _, ok := seen[code]; !ok {
			t.Errorf("the code %q is not registered", code)
		}
	}
	if seen["dy"] != "action dy" || seen["m"] != "section m" {
		t.Errorf("dy is %q and m is %q, want the action and the section", seen["dy"], seen["m"])
	}
}

// TestSettingsDeletionMemberNavigation walks the section's Back buttons and the card's exits.
func TestSettingsDeletionMemberNavigation(t *testing.T) {
	settingsDelMenus(t, func(t *testing.T, m *settingsDelMenu) {
		// The hub's Back is the core Back to Main, the list's is the hub, a row screen's is the list.
		_, keyboard := m.openHub()
		if fields := settingsButtonFields(t, keyboard[len(keyboard)-1][0]); fields["a"] != "bk" {
			t.Errorf("the hub's Back = %v, want the core Back", fields)
		}
		_, keyboard = m.open("dl", "m")
		settingsDelWantBack(t, keyboard, map[string]string{"a": "op", "p": "d"})
		for _, code := range []string{"mc", "me", "ms"} {
			_, keyboard = m.open("dr", code)
			settingsDelWantBack(t, keyboard, map[string]string{"a": "dl", "t": "m"})
			m.pressButton(keyboard[len(keyboard)-1][0])
			if text, _ := m.last(); !containsMarker(text, "settings_del_list_mem") {
				t.Errorf("Back from the %s screen drew %q, want the Member list", code, text)
			}
		}
		_, keyboard = m.last()
		m.pressButton(keyboard[len(keyboard)-1][0])
		if text, _ := m.last(); !containsMarker(text, "settings_del_intro") {
			t.Errorf("Back from the list drew %q, want the hub", text)
		}

		// Every cell of the list opens its row.
		_, keyboard = m.open("dl", "m")
		for i, code := range []string{"mc", "me", "ms"} {
			settingsDelWantRowFields(t, settingsAllButtons(keyboard[:2])[i], "dr", code)
		}

		// The card: No is the hub, Back is Main, Close is the core Close.
		m.choose("ms", "600")
		_, no := settingsDelCardOf(t, m)
		if fields := settingsButtonFields(t, no); fields["a"] != "no" || fields["p"] != "d" {
			t.Errorf("the card's No = %v, want a=no and p=d", fields)
		}
		_, keyboard = m.last()
		settingsDelWantBack(t, keyboard, map[string]string{"a": "bk"})
		if fields := settingsButtonFields(t, keyboard[len(keyboard)-1][1]); fields["a"] != "cl" {
			t.Errorf("the card's Close = %v, want the core Close", fields)
		}
	})
}
