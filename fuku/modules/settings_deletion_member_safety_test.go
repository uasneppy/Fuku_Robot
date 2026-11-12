//go:build testtools

package modules

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/db/staff"
)

// Safeguards of the Member messages section (plan 17-10, D-16, D-17, D-18). The section has no
// command, so each rule carries an authority test and a drift test in place of a parity test.

// settingsDelMemberStaffTitle is a Staff Group title no screen of the page may carry.
const settingsDelMemberStaffTitle = "Zed Member Staff Title"

// yesFields are the fields of the Self-destruct card's Yes for delay v, expiring in the card lifetime,
// as this menu would draw them.
func (m *settingsDelMenu) yesFields(v string) map[string]string {
	return m.fields("a", "dy", "t", "ms", "v", v, "e", settingsCardExpiry(time.Now()))
}

// wantAlert presses a button built from fields and fails unless the press was answered once with the
// alert key, edited nothing and made no getChatMember call about the bot. It returns nothing else:
// the callers check what was written.
func (m *settingsDelMenu) wantAlert(t *testing.T, name string, fields map[string]string, key string) {
	t.Helper()
	env := m.env
	data := encodeCallbackData(settingsCallbackNamespace, fields)
	if data == "" {
		t.Fatalf("%s: the callback data did not encode", name)
	}
	edits, answers, reads := m.editCount(), env.answerCount(), env.botReads(env.chat.Id)
	env.pressRaw(m.chat, m.actor, m.msgID, data)
	if text, alert := env.lastAnswer(); text != staffMarker(key) || !alert {
		t.Errorf("%s: answered %q alert=%v, want the %s alert", name, text, alert, key)
	}
	if got := env.answerCount() - answers; got != 1 {
		t.Errorf("%s: answered %d times, want once", name, got)
	}
	if m.editCount() != edits {
		t.Errorf("%s: edited the menu", name)
	}
	if env.botReads(env.chat.Id) != reads {
		t.Errorf("%s: read the bot's right", name)
	}
}

// memberChanges are the member rows' changing presses: every choice of the three rows
// (Off included, which removes) and the card's Yes.
func (m *settingsDelMenu) memberChanges() []struct {
	name   string
	fields map[string]string
} {
	return []struct {
		name   string
		fields map[string]string
	}{
		{"Commands On", m.fields("a", "st", "p", "d", "t", "mc", "v", "0")},
		{"Commands Off", m.fields("a", "st", "p", "d", "t", "mc", "v", "n")},
		{"Edited 60", m.fields("a", "st", "p", "d", "t", "me", "v", "60")},
		{"Edited Off", m.fields("a", "st", "p", "d", "t", "me", "v", "n")},
		{"Self-destruct 600", m.fields("a", "st", "p", "d", "t", "ms", "v", "600")},
		{"Self-destruct Off", m.fields("a", "st", "p", "d", "t", "ms", "v", "n")},
		{"the card's Yes", m.yesFields("600")},
	}
}

func TestSettingsAuthorityDeletionMember(t *testing.T) {
	t.Run("an administrator without can_change_info opens the section but changes nothing", func(t *testing.T) {
		env := newSettingsDelEnv(t)
		listEditAdmin(env.settingsEnv, env.chat, env.admin, false)
		m := env.groupMenuOf(env.admin)
		var counter settingsDelCounter
		counter.install(t)
		// A rule is stored, so Off would be a real removal.
		env.setRule(deletion.KindMemberSelfDestruct, 600)
		env.setRule(deletion.KindMemberEdited, 60)
		env.setRule(deletion.KindMemberCommands, 0)
		sets := counter.sets

		for _, kv := range [][]string{
			{"a", "op", "p", "d"}, {"a", "dl", "t", "m"}, {"a", "dr", "t", "mc"}, {"a", "dr", "t", "me"}, {"a", "dr", "t", "ms"},
		} {
			m.press(kv...)
			if text, _ := m.last(); containsMarker(text, "settings_no_change_info") || containsMarker(text, "settings_cannot_open") {
				t.Errorf("an administrator without can_change_info could not open %v: %q", kv, text)
			}
		}
		for _, tc := range m.memberChanges() {
			m.wantAlert(t, tc.name, tc.fields, "settings_no_change_info")
		}
		if counter.sets != sets || counter.removes != 0 {
			t.Errorf("refused presses wrote: %d sets (was %d), %d removes", counter.sets, sets, counter.removes)
		}
		for kind, want := range map[string]int{deletion.KindMemberCommands: 0, deletion.KindMemberEdited: 60, deletion.KindMemberSelfDestruct: 600} {
			if delay, ok := env.stored(kind); !ok || delay != want {
				t.Errorf("%s = %d (%v) after refused presses, want the stored %d untouched", kind, delay, ok, want)
			}
		}
	})

	t.Run("the right is judged again at the card's Yes", func(t *testing.T) {
		env := newSettingsDelEnv(t)
		listEditAdmin(env.settingsEnv, env.chat, env.admin, true)
		m := env.groupMenuOf(env.admin)
		var counter settingsDelCounter
		counter.install(t)
		m.choose("ms", "600")
		yes, _ := settingsDelCardOf(t, m)
		if counter.sets != 0 {
			t.Fatalf("the card's draw wrote %d times", counter.sets)
		}
		listEditAdmin(env.settingsEnv, env.chat, env.admin, false)
		m.wantAlert(t, "a Yes after the right was lost", settingsButtonFields(t, yes), "settings_no_change_info")
		if _, ok := env.stored(deletion.KindMemberSelfDestruct); ok || counter.sets != 0 {
			t.Errorf("a refused Yes wrote: %d sets", counter.sets)
		}
		// With the right back the same card works.
		listEditAdmin(env.settingsEnv, env.chat, env.admin, true)
		m.pressButton(yes)
		if delay, ok := env.stored(deletion.KindMemberSelfDestruct); !ok || delay != 600 {
			t.Errorf("a Yes with the right stored %d (%v), want 600", delay, ok)
		}
	})

	t.Run("with the owner's switch off a non-owner administrator is refused at the gate", func(t *testing.T) {
		env := newSettingsDelEnv(t)
		listEditAdmin(env.settingsEnv, env.chat, env.admin, true)
		m := env.groupMenuOf(env.admin)
		env.setAdminsAllowed(env.chat.Id, false)
		var counter settingsDelCounter
		counter.install(t)
		for _, tc := range append(m.memberChanges(), struct {
			name   string
			fields map[string]string
		}{"opening the section", m.fields("a", "dl", "t", "m")}) {
			m.wantAlert(t, tc.name, tc.fields, "settings_gate_owner_only")
		}
		if counter.sets != 0 || counter.removes != 0 {
			t.Errorf("a gated administrator wrote: %d sets, %d removes", counter.sets, counter.removes)
		}
		settingsDelNoRows(t, env.chat.Id)
	})

	t.Run("the creator's presses write in a group menu and in a private one", func(t *testing.T) {
		settingsDelMenus(t, func(t *testing.T, m *settingsDelMenu) {
			env := m.env
			m.choose("mc", "0")
			m.choose("me", "60")
			m.choose("ms", "600")
			yes, _ := settingsDelCardOf(t, m)
			m.pressButton(yes)
			for kind, want := range map[string]int{deletion.KindMemberCommands: 0, deletion.KindMemberEdited: 60, deletion.KindMemberSelfDestruct: 600} {
				if delay, ok := env.stored(kind); !ok || delay != want {
					t.Errorf("the creator stored %s = %d (%v), want %d", kind, delay, ok, want)
				}
			}
			m.choose("ms", "n")
			if _, ok := env.stored(deletion.KindMemberSelfDestruct); ok {
				t.Error("the creator's Self-destruct Off left the row stored")
			}
		})
	})
}

func TestSettingsDeletionMemberStaffGroup(t *testing.T) {
	// forged are the section's presses a Staff Group must refuse, whichever menu kind carries them.
	forged := func(m *settingsDelMenu) []struct {
		name   string
		fields map[string]string
	} {
		return append([]struct {
			name   string
			fields map[string]string
		}{
			{"the member list", m.fields("a", "dl", "t", "m")},
			{"the Commands row", m.fields("a", "dr", "t", "mc")},
			{"the Edited row", m.fields("a", "dr", "t", "me")},
			{"the Self-destruct row", m.fields("a", "dr", "t", "ms")},
		}, m.memberChanges()...)
	}

	t.Run("a Staff Group has no Deletion cell and no line", func(t *testing.T) {
		env := newSettingsDelEnv(t)
		env.makeStaff()
		m := env.groupMenu()
		if _, ok := settingsPageButton(t, m.mainKeyboard, "d"); ok {
			t.Errorf("Main in a Staff Group has a Deletion cell: %+v", m.mainKeyboard)
		}
		if _, ok := settingsMainStateLine(m.mainText, "settings_page_del"); ok {
			t.Errorf("Main in a Staff Group has a Deletion line: %q", m.mainText)
		}
	})

	t.Run("a forged press in a Staff Group is invalid and writes nothing", func(t *testing.T) {
		settingsDelMenus(t, func(t *testing.T, m *settingsDelMenu) {
			env := m.env
			// Menus opened while the group was ordinary; the group then becomes a Staff Group.
			env.makeStaff()
			var counter settingsDelCounter
			counter.install(t)
			for _, tc := range forged(m) {
				m.wantAlert(t, tc.name, tc.fields, "common_callback_invalid_request")
			}
			if counter.sets != 0 || counter.removes != 0 {
				t.Errorf("a Staff Group press wrote: %d sets, %d removes", counter.sets, counter.removes)
			}
			settingsDelNoRows(t, env.chat.Id)
		})
	})

	t.Run("an unreadable Staff Group row is the cannot-open edit with no write", func(t *testing.T) {
		settingsDelMenus(t, func(t *testing.T, m *settingsDelMenu) {
			env := m.env
			mreqSwap(t, &deletionStaffLookup, func(int64) (*models.StaffGroup, error) { return nil, errors.New("staff read failed") })
			var counter settingsDelCounter
			counter.install(t)
			for _, tc := range forged(m) {
				reads, answers := env.botReads(env.chat.Id), env.answerCount()
				env.pressRaw(m.chat, m.actor, m.msgID, encodeCallbackData(settingsCallbackNamespace, tc.fields))
				text, keyboard := m.last()
				if text != staffMarker("settings_cannot_open") || len(keyboard) != 0 {
					t.Errorf("%s drew %q (%d rows), want the cannot-open edit", tc.name, text, len(keyboard))
				}
				if answer, alert := env.lastAnswer(); answer != staffMarker("settings_cannot_open_alert") || !alert {
					t.Errorf("%s answered %q alert=%v, want the cannot-open alert", tc.name, answer, alert)
				}
				if got := env.answerCount() - answers; got != 1 {
					t.Errorf("%s was answered %d times, want once", tc.name, got)
				}
				if env.botReads(env.chat.Id) != reads {
					t.Errorf("%s read the bot's right", tc.name)
				}
			}
			if counter.sets != 0 || counter.removes != 0 {
				t.Errorf("an unreadable Staff Group row let a write through: %d sets, %d removes", counter.sets, counter.removes)
			}
			settingsDelNoRows(t, env.chat.Id)
		})
	})

	t.Run("no text or alert of a Staff Group's refusals names it", func(t *testing.T) {
		env := newSettingsDelEnv(t)
		m := env.groupMenu()
		// The Main page drawn before the group became a Staff Group names the group itself; only what
		// the refusals produce afterwards is judged.
		before := len(env.edits(env.chat.Id, m.msgID))
		if _, err := staff.CreateStaffGroup(env.chat.Id, env.owner.Id, settingsDelMemberStaffTitle); err != nil {
			t.Fatalf("CreateStaffGroup error = %v", err)
		}
		for _, tc := range forged(m) {
			m.wantAlert(t, tc.name, tc.fields, "common_callback_invalid_request")
		}
		var seen []string
		for _, edit := range env.edits(env.chat.Id, m.msgID)[before:] {
			seen = append(seen, fmt.Sprint(edit.Params["text"]))
		}
		for _, answer := range env.fake.callsFor("answerCallbackQuery") {
			seen = append(seen, fmt.Sprint(answer.Params["text"]))
		}
		if len(seen) < len(forged(m)) {
			t.Fatalf("only %d texts were produced by %d refusals", len(seen), len(forged(m)))
		}
		everything := strings.Join(seen, "\n")
		for _, leak := range []string{settingsDelMemberStaffTitle, strings.TrimPrefix(strconv.FormatInt(env.chat.Id, 10), "-")} {
			if strings.Contains(everything, leak) {
				t.Errorf("a refusal in a Staff Group names it (%q): %q", leak, everything)
			}
		}
	})
}
