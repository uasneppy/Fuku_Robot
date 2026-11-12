//go:build testtools

package modules

import (
	"strings"
	"testing"

	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
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
