//go:build testtools

package modules

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/uasneppy/fuku_robot/fuku/db"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/db/staff"
	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
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

// settingsDelDriftEnv is the producers' environment with the page's owner: the command wrapper, the edit
// handler and the self-destruct watcher on the real dispatcher, the admin cache primed (a member is a
// known non-administrator), the batcher's buffer emptied now and when the test ends, and no rule stored.
func settingsDelDriftEnv(t *testing.T) *commandDeletionEnv {
	t.Helper()
	env := newCommandDeletionEnv(t, ext.BaseProcessor{})
	env.setCommandsRule(false)
	LoadMemberDeletion(env.dispatcher)
	drainMemberDeleteQueue(t)
	restoreMemberDeleteBatcher(t)
	cache.InvalidateAdminCache(env.chat.Id)
	t.Cleanup(func() { cache.InvalidateAdminCache(env.chat.Id) })
	if got := cache.LoadAdminCache(env.bot, env.chat.Id); !got.Cached {
		t.Fatalf("LoadAdminCache() = %+v, want a cached list", got)
	}
	return env
}

// settingsDelDriftEdit delivers an edit of a message dated five minutes ago, edited gap seconds after
// its date, through the real dispatcher, and returns its ID.
func settingsDelDriftEdit(t *testing.T, env *commandDeletionEnv, from gotgbot.User, gap int64) int64 {
	t.Helper()
	id := env.updateID()
	date := env.now().Unix() - 300
	msg := &gotgbot.Message{MessageId: id, Date: date, EditDate: date + gap, Chat: env.chat, From: &from, Text: "edited"}
	if err := env.dispatcher.ProcessUpdate(env.bot, &gotgbot.Update{UpdateId: id, EditedMessage: msg}, nil); err != nil {
		t.Fatalf("ProcessUpdate(%d) error = %v", id, err)
	}
	return id
}

// settingsDelDriftFlush writes what the batcher holds and returns the queue rows of the env chat.
func settingsDelDriftFlush(t *testing.T, env *commandDeletionEnv) map[int64]models.ScheduledDeletion {
	t.Helper()
	memberDeleteTestFlush(t)
	return env.rows()
}

// TestSettingsDriftDeletionMember proves the page and the three producers read one rule: a rule set on
// the page acts at the producer's next message, Off stops it at the next one, a rule written straight to
// the repository is drawn by the next draw, a stored value a rule does not accept is Off on both sides,
// and opening writes nothing. The page's own presses are the real press pipeline; the producers are the
// real dispatcher.
func TestSettingsDriftDeletionMember(t *testing.T) {
	env := settingsDelDriftEnv(t)
	member := env.mreqMember("Rae", "", "rae")
	m := env.del.groupMenu()

	t.Run("opening every member screen writes no rule and no queue row", func(t *testing.T) {
		m.openHub()
		for _, kv := range [][]string{{"dl", "m"}, {"dr", "mc"}, {"dr", "me"}, {"dr", "ms"}} {
			m.open(kv[0], kv[1])
		}
		// The environment's welcome text is a greetings row of its own, so only the rule and queue rows count.
		for name, model := range map[string]any{"delete rule": &models.DeleteRule{}, "scheduled deletion": &models.ScheduledDeletion{}} {
			var count int64
			if err := db.DB.Model(model).Where("chat_id = ?", env.chat.Id).Count(&count).Error; err != nil || count != 0 {
				t.Errorf("opening the section left %d %s rows (error %v)", count, name, err)
			}
		}
		if rows := settingsDelDriftFlush(t, env); len(rows) != 0 {
			t.Errorf("opening the section queued rows: %+v", rows)
		}
	})

	t.Run("Commands set on the page deletes the next command, Off keeps it", func(t *testing.T) {
		m.open("dr", "mc")
		m.choose("mc", "0")
		id := env.deliver(&member, commandMessage("/rules"))
		if got := env.deleted(env.chat.Id, id); got != 1 {
			t.Fatalf("deletes of the command after Commands On = %d, want 1", got)
		}
		m.choose("mc", "n")
		id = env.deliver(&member, commandMessage("/rules"))
		if got := env.deleted(env.chat.Id, id); got != 0 {
			t.Errorf("deletes of the command after Commands Off = %d, want none", got)
		}
	})

	t.Run("Edited set on the page deletes the next late edit, Off keeps it", func(t *testing.T) {
		m.open("dr", "me")
		m.choose("me", "60")
		if id := settingsDelDriftEdit(t, env, member, 90); env.deleted(env.chat.Id, id) != 1 {
			t.Errorf("an edit 90 s late with a 1 minute grace was not deleted once")
		}
		if id := settingsDelDriftEdit(t, env, member, 20); env.deleted(env.chat.Id, id) != 0 {
			t.Errorf("an edit 20 s late with a 1 minute grace was deleted")
		}
		m.choose("me", "n")
		if id := settingsDelDriftEdit(t, env, member, 90); env.deleted(env.chat.Id, id) != 0 {
			t.Errorf("an edit 90 s late was deleted after Edited Off")
		}
	})

	t.Run("Self-destruct set through the card queues the next message, Off stops it", func(t *testing.T) {
		m.open("dr", "ms")
		m.choose("ms", "600")
		yes, _ := settingsDelCardOf(t, m)
		if rows := settingsDelDriftFlush(t, env); len(rows) != 0 {
			t.Fatalf("the card's draw queued rows before the Yes: %+v", rows)
		}
		m.pressButton(yes)
		id := env.deliver(&member, func(msg *gotgbot.Message) { msg.Text = "hello" })
		rows := settingsDelDriftFlush(t, env)
		if len(rows) != 1 {
			t.Fatalf("queue rows after one message with Self-destruct 10 min = %+v, want one", rows)
		}
		env.wantRow(id, deletion.KindMemberSelfDestruct, env.now(), env.now().Add(600*time.Second))
		m.choose("ms", "n")
		if rows := env.rows(); len(rows) != 0 {
			t.Errorf("Off left waiting rows: %+v", rows)
		}
		env.deliver(&member, func(msg *gotgbot.Message) { msg.Text = "again" })
		if rows := settingsDelDriftFlush(t, env); len(rows) != 0 {
			t.Errorf("a message after Self-destruct Off was queued: %+v", rows)
		}
	})

	t.Run("a rule written straight to the repository is drawn by the next draw and acts", func(t *testing.T) {
		if err := deletion.SetRule(t.Context(), env.chat.Id, deletion.KindMemberEdited, 300); err != nil {
			t.Fatalf("SetRule error = %v", err)
		}
		text, _ := m.open("dr", "me")
		settingsDelWantCurrently(t, text, "⏳", staffMarker("settings_del_state_grace"), staffMarker("settings_del_delay_5m"))
		if id := settingsDelDriftEdit(t, env, member, 400); env.deleted(env.chat.Id, id) != 1 {
			t.Error("an edit 400 s late with a 5 minute grace was not deleted once")
		}
		if id := settingsDelDriftEdit(t, env, member, 90); env.deleted(env.chat.Id, id) != 0 {
			t.Error("an edit 90 s late with a 5 minute grace was deleted")
		}
	})

	t.Run("a stored value a rule does not accept is Off on the page and in the producers", func(t *testing.T) {
		// SetRule and the page refuse such a value; only a hand-written row can hold it.
		for _, tc := range []struct {
			kind        string
			valid, hand int
		}{
			{deletion.KindMemberCommands, 0, 60},
			{deletion.KindMemberEdited, 60, 45},
			{deletion.KindMemberSelfDestruct, 60, 45},
		} {
			if err := deletion.SetRule(t.Context(), env.chat.Id, tc.kind, tc.valid); err != nil {
				t.Fatalf("SetRule(%s) error = %v", tc.kind, err)
			}
			if err := db.DB.Model(&models.DeleteRule{}).Where("chat_id = ? AND kind = ?", env.chat.Id, tc.kind).
				Update("delay_secs", tc.hand).Error; err != nil {
				t.Fatalf("write %s = %d by hand: %v", tc.kind, tc.hand, err)
			}
		}
		dbcache.DeleteCache(dbcache.CacheKey("delete_rules", env.chat.Id))
		id := env.deliver(&member, commandMessage("/rules"))
		if got := env.deleted(env.chat.Id, id); got != 0 {
			t.Errorf("a command was deleted under a hand-written commands value 60: %d deletes", got)
		}
		if id := settingsDelDriftEdit(t, env, member, 400); env.deleted(env.chat.Id, id) != 0 {
			t.Error("a late edit was deleted under a hand-written grace of 45 s")
		}
		env.deliver(&member, func(msg *gotgbot.Message) { msg.Text = "hello" })
		if rows := settingsDelDriftFlush(t, env); len(rows) != 0 {
			t.Errorf("a message was queued under a hand-written Self-destruct of 45 s: %+v", rows)
		}
		for _, code := range []string{"mc", "me", "ms"} {
			text, _ := m.open("dr", code)
			settingsDelWantCurrently(t, text, "➖", staffMarker("settings_state_off_plain"))
		}
		list, _ := m.open("dl", "m")
		for _, key := range []string{"settings_del_mem_commands", "settings_del_mem_edited", "settings_del_mem_self"} {
			settingsDelWantLine(t, list, key, "➖", staffMarker("settings_state_off_plain"))
		}
		hub, _ := m.openHub()
		settingsDelWantLine(t, hub, "settings_del_sec_member", "💬", staffMarker("settings_state_off_plain"))
	})
}

// TestSettingsDeletionMemberStale proves a drawn screen is a snapshot and every press judges the stored
// state again: after another admin's write the old message shows the old state until the next press, a
// card writes the absolute value it named, and a press after the bot lost its right still stores.
func TestSettingsDeletionMemberStale(t *testing.T) {
	t.Run("a row screen drawn before another admin's write shows the old state until the next press", func(t *testing.T) {
		env := newSettingsDelEnv(t)
		m := env.groupMenu()
		text, _ := m.open("dr", "me")
		settingsDelWantCurrently(t, text, "➖", staffMarker("settings_state_off_plain"))
		env.setRule(deletion.KindMemberEdited, 300) // another admin's press: the repository drops the shared key
		if old, _ := m.last(); strings.Contains(old, staffMarker("settings_del_delay_5m")) {
			t.Errorf("the drawn message changed without a press: %q", old)
		}
		var counter settingsDelCounter
		counter.install(t)
		m.choose("me", "300") // the value now stored: judged against the new state, nothing to write
		if counter.sets != 0 {
			t.Errorf("a press of the value another admin stored wrote %d times", counter.sets)
		}
		text, _ = m.last()
		settingsDelWantCurrently(t, text, "⏳", staffMarker("settings_del_state_grace"), staffMarker("settings_del_delay_5m"))
		m.choose("me", "60")
		if delay, ok := env.stored(deletion.KindMemberEdited); !ok || delay != 60 || counter.sets != 1 {
			t.Errorf("the next change stored %d (%v) with %d sets, want 60 and 1", delay, ok, counter.sets)
		}
	})

	t.Run("a card writes the absolute value it named, whatever another admin set meanwhile", func(t *testing.T) {
		env := newSettingsDelEnv(t)
		m := env.groupMenu()
		m.choose("ms", "600")
		yes, _ := settingsDelCardOf(t, m)
		env.setRule(deletion.KindMemberSelfDestruct, 3600)
		m.pressButton(yes)
		if delay, ok := env.stored(deletion.KindMemberSelfDestruct); !ok || delay != 600 {
			t.Errorf("the Yes stored %d (%v), want the 600 the card named", delay, ok)
		}
		text, _ := m.last()
		settingsDelWantCurrently(t, text, "⏳", staffMarker("settings_del_state_after"), staffMarker("settings_del_delay_10m"))
	})

	t.Run("a Yes after another admin set the same delay writes nothing more", func(t *testing.T) {
		env := newSettingsDelEnv(t)
		m := env.groupMenu()
		m.choose("ms", "600")
		yes, _ := settingsDelCardOf(t, m)
		env.setRule(deletion.KindMemberSelfDestruct, 600)
		var counter settingsDelCounter
		counter.install(t)
		m.pressButton(yes)
		if counter.sets != 0 || counter.removes != 0 {
			t.Errorf("a Yes for the stored delay wrote: %d sets, %d removes", counter.sets, counter.removes)
		}
		text, _ := m.last()
		settingsDelWantCurrently(t, text, "⏳", staffMarker("settings_del_state_after"), staffMarker("settings_del_delay_10m"))
	})

	t.Run("a press after the bot lost its right redraws with the line once and still stores", func(t *testing.T) {
		noRight := staffMarker("settings_del_no_right")
		env := newSettingsDelEnv(t)
		m := env.groupMenu()
		m.open("dr", "mc")
		env.setBot(settingsDelBotJSON("member", false))
		m.choose("mc", "0")
		if delay, ok := env.stored(deletion.KindMemberCommands); !ok || delay != 0 {
			t.Errorf("stored member:commands = %d (%v), want a stored 0", delay, ok)
		}
		if text, _ := m.last(); strings.Count(text, noRight) != 1 {
			t.Errorf("the redraw = %q, want the no-right line once", text)
		}
		m.choose("ms", "3600")
		if text, _ := m.last(); strings.Count(text, noRight) != 1 {
			t.Errorf("the card = %q, want the no-right line once", text)
		}
		yes, _ := settingsDelCardOf(t, m)
		m.pressButton(yes)
		if delay, ok := env.stored(deletion.KindMemberSelfDestruct); !ok || delay != 3600 {
			t.Errorf("stored member:selfdestruct = %d (%v), want 3600 with the line showing", delay, ok)
		}
		if text, _ := m.last(); strings.Count(text, noRight) != 1 {
			t.Errorf("the redraw after the Yes = %q, want the no-right line once", text)
		}
	})
}
