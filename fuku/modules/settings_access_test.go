//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db/menuaccess"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// settingsAccessStored reads the owner switch of chatID straight from the database.
func settingsAccessStored(t *testing.T, chatID int64) bool {
	t.Helper()
	allowed, err := menuaccess.AdminsAllowedFresh(context.Background(), chatID)
	if err != nil {
		t.Fatalf("AdminsAllowedFresh(%d) error = %v", chatID, err)
	}
	return allowed
}

// settingsAccessFind returns the first button whose decoded fields satisfy match.
func settingsAccessFind(t *testing.T, keyboard [][]gotgbot.InlineKeyboardButton, match func(map[string]string) bool) (gotgbot.InlineKeyboardButton, bool) {
	t.Helper()
	for _, button := range settingsAllButtons(keyboard) {
		if match(settingsButtonFields(t, button)) {
			return button, true
		}
	}
	return gotgbot.InlineKeyboardButton{}, false
}

// settingsAccessToggle finds the switch toggle of a drawn Access page and returns it with
// the target value it sends.
func settingsAccessToggle(t *testing.T, keyboard [][]gotgbot.InlineKeyboardButton) (gotgbot.InlineKeyboardButton, string) {
	t.Helper()
	button, ok := settingsAccessFind(t, keyboard, func(f map[string]string) bool {
		return f["a"] == "tg" && f["p"] == settingsPageAccess && f["t"] == settingsAccessControlSwitch
	})
	if !ok {
		t.Fatalf("the Access page has no toggle: %+v", keyboard)
	}
	return button, settingsButtonFields(t, button)["v"]
}

// settingsAccessOpenFromMain presses the Access button of the Main page drawn on msgID and
// returns once the page is drawn. press is the test's way to press a raw button.
func settingsAccessOpenFromMain(t *testing.T, keyboard [][]gotgbot.InlineKeyboardButton, press func(gotgbot.InlineKeyboardButton)) {
	t.Helper()
	button, ok := settingsPageButton(t, keyboard, settingsPageAccess)
	if !ok {
		t.Fatalf("the Main page has no Access button: %+v", keyboard)
	}
	press(button)
}

func TestSettingsAccessOwnerFlow(t *testing.T) {
	t.Run("in a group menu", func(t *testing.T) {
		resetSettingsOwnerMemo(t)
		env := newSettingsEnv(t)
		owner := env.newOwner(env.chat)
		env.setAdminsAllowed(env.chat.Id, false)

		msgID := env.openMenu(owner)
		_, main := env.lastEdit(env.chat.Id, msgID)
		settingsAccessOpenFromMain(t, main, func(b gotgbot.InlineKeyboardButton) {
			env.pressRaw(env.chat, owner, msgID, b.CallbackData)
		})

		text, keyboard := env.lastEdit(env.chat.Id, msgID)
		for _, want := range []string{
			staffMarker("settings_access_status") + " ❌ " + staffMarker("common_status_off"),
			staffMarker("settings_access_off_desc"),
		} {
			if !strings.Contains(text, want) {
				t.Fatalf("Access page %q does not contain %q", text, want)
			}
		}
		toggle, target := settingsAccessToggle(t, keyboard)
		if target != "1" {
			t.Fatalf("the off toggle sends v=%s, want 1", target)
		}

		// The first tap toward ON draws the card and writes nothing.
		env.pressRaw(env.chat, owner, msgID, toggle.CallbackData)
		text, keyboard = env.lastEdit(env.chat.Id, msgID)
		if !strings.Contains(text, staffMarker("settings_access_confirm")) {
			t.Fatalf("the first tap drew %q, want the confirm card", text)
		}
		if settingsAccessStored(t, env.chat.Id) {
			t.Fatal("the first tap toward ON wrote ON")
		}
		yes, ok := settingsAccessFind(t, keyboard, func(f map[string]string) bool { return f["a"] == settingsActAccessYes && f["e"] != "" })
		if !ok {
			t.Fatalf("the card has no Yes with an expiry: %+v", keyboard)
		}
		if _, ok := settingsAccessFind(t, keyboard, func(f map[string]string) bool { return f["a"] == "no" && f["p"] == settingsPageAccess }); !ok {
			t.Errorf("the card has no No back to the Access page: %+v", keyboard)
		}

		// The admin is still shut out until Yes.
		env.press(env.admin, msgID, env.groupFields(env.admin, "a", "bk"))
		if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("settings_gate_owner_only")) {
			t.Errorf("before Yes the admin's answer = %q (alert %v), want settings_gate_owner_only", answer, alert)
		}

		env.pressRaw(env.chat, owner, msgID, yes.CallbackData)
		if !settingsAccessStored(t, env.chat.Id) {
			t.Fatal("Yes did not write ON")
		}
		text, keyboard = env.lastEdit(env.chat.Id, msgID)
		if !strings.Contains(text, staffMarker("settings_access_on_desc")) {
			t.Errorf("after Yes the page drew %q, want the on description", text)
		}
		toggle, target = settingsAccessToggle(t, keyboard)
		if target != "0" {
			t.Fatalf("the on toggle sends v=%s, want 0", target)
		}

		// The admin's next press passes the gate.
		editsBefore := len(env.edits(env.chat.Id, msgID))
		env.press(env.admin, msgID, env.groupFields(env.admin, "a", "bk"))
		if got := len(env.edits(env.chat.Id, msgID)); got != editsBefore+1 {
			t.Errorf("the admin's press made %d edits, want 1", got-editsBefore)
		}

		// One tap turns it off at once and the admin is shut out again.
		env.pressRaw(env.chat, owner, msgID, toggle.CallbackData)
		if settingsAccessStored(t, env.chat.Id) {
			t.Fatal("the OFF tap did not write OFF")
		}
		env.press(env.admin, msgID, env.groupFields(env.admin, "a", "bk"))
		if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("settings_gate_owner_only")) {
			t.Errorf("after OFF the admin's answer = %q (alert %v), want settings_gate_owner_only", answer, alert)
		}
	})

	t.Run("in a private menu", func(t *testing.T) {
		resetSettingsOwnerMemo(t)
		env := newSettingsEnv(t)
		owner := env.newOwner(env.chat)
		env.setAdminsAllowed(env.chat.Id, false)

		env.sendPrivate(owner, fmt.Sprintf("/start settings_%d", env.chat.Id))
		msgID := settingsPrivateOnlySent(t, env, owner).MessageID
		private := settingsPrivateChat(owner)
		sent := env.fake.sentTo(owner.Id)
		mainKeyboard := staffKeyboardOf(sent[len(sent)-1].Params["reply_markup"])
		settingsAccessOpenFromMain(t, mainKeyboard, func(b gotgbot.InlineKeyboardButton) {
			env.pressRaw(private, owner, msgID, b.CallbackData)
		})

		text, keyboard := env.lastEdit(owner.Id, msgID)
		if !strings.Contains(text, staffMarker("settings_access_off_desc")) {
			t.Fatalf("private Access page %q does not describe the off state", text)
		}
		toggle, target := settingsAccessToggle(t, keyboard)
		if target != "1" || settingsButtonFields(t, toggle)["c"] != fmt.Sprint(env.chat.Id) {
			t.Fatalf("private toggle fields = %v, want v=1 and the group's c", settingsButtonFields(t, toggle))
		}

		env.pressRaw(private, owner, msgID, toggle.CallbackData)
		_, keyboard = env.lastEdit(owner.Id, msgID)
		if settingsAccessStored(t, env.chat.Id) {
			t.Fatal("the first private tap toward ON wrote ON")
		}
		yes, ok := settingsAccessFind(t, keyboard, func(f map[string]string) bool { return f["a"] == settingsActAccessYes })
		if !ok {
			t.Fatalf("the private card has no Yes: %+v", keyboard)
		}
		env.pressRaw(private, owner, msgID, yes.CallbackData)
		if !settingsAccessStored(t, env.chat.Id) {
			t.Fatal("the private Yes did not write ON")
		}
		_, keyboard = env.lastEdit(owner.Id, msgID)
		toggle, target = settingsAccessToggle(t, keyboard)
		if target != "0" {
			t.Fatalf("the private on toggle sends v=%s, want 0", target)
		}
		env.pressRaw(private, owner, msgID, toggle.CallbackData)
		if settingsAccessStored(t, env.chat.Id) {
			t.Fatal("the private OFF tap did not write OFF")
		}
	})
}

// settingsAccessSeams restores the two Access seams when the test ends.
func settingsAccessSeams(t *testing.T) {
	t.Helper()
	lookup, write := settingsAccessLookup, settingsAccessWrite
	t.Cleanup(func() { settingsAccessLookup, settingsAccessWrite = lookup, write })
}

// settingsAccessCountWrites wraps the write seam and returns the number of calls made.
func settingsAccessCountWrites(t *testing.T) *atomic.Int64 {
	t.Helper()
	settingsAccessSeams(t)
	var calls atomic.Int64
	inner := settingsAccessWrite
	settingsAccessWrite = func(ctx context.Context, chatID int64, allowed bool, by int64) error {
		calls.Add(1)
		return inner(ctx, chatID, allowed, by)
	}
	return &calls
}

// settingsAccessOwnerOpen opens the owner's group menu and its Access page.
func settingsAccessOwnerOpen(t *testing.T, env *settingsEnv, owner gotgbot.User) int64 {
	t.Helper()
	msgID := env.openMenu(owner)
	env.press(owner, msgID, env.groupFields(owner, "a", "op", "p", settingsPageAccess))
	return msgID
}

// settingsAccessPressToggle presses the toggle of the Access page drawn on msgID.
func settingsAccessPressToggle(t *testing.T, env *settingsEnv, owner gotgbot.User, msgID int64) {
	t.Helper()
	_, keyboard := env.lastEdit(env.chat.Id, msgID)
	toggle, _ := settingsAccessToggle(t, keyboard)
	env.pressRaw(env.chat, owner, msgID, toggle.CallbackData)
}

// settingsAccessPressYes presses the Yes of the card drawn on msgID.
func settingsAccessPressYes(t *testing.T, env *settingsEnv, owner gotgbot.User, msgID int64) {
	t.Helper()
	_, keyboard := env.lastEdit(env.chat.Id, msgID)
	yes, ok := settingsAccessFind(t, keyboard, func(f map[string]string) bool { return f["a"] == settingsActAccessYes })
	if !ok {
		t.Fatalf("no Yes on the drawn card: %+v", keyboard)
	}
	env.pressRaw(env.chat, owner, msgID, yes.CallbackData)
}

// settingsAccessTurnOn runs the two taps that turn the switch on.
func settingsAccessTurnOn(t *testing.T, env *settingsEnv, owner gotgbot.User, msgID int64) {
	t.Helper()
	settingsAccessPressToggle(t, env, owner, msgID)
	settingsAccessPressYes(t, env, owner, msgID)
}

// settingsAccessHasText reports whether any text of the Access strings reached text.
func settingsAccessHasText(text string) bool {
	for _, key := range []string{
		"settings_page_access", "settings_access_status", "settings_access_on_desc",
		"settings_access_off_desc", "settings_state_access_on", "settings_state_access_off",
		"settings_btn_access_toggle", "settings_access_confirm",
	} {
		if strings.Contains(text, staffMarker(key)) {
			return true
		}
	}
	return false
}

func TestSettingsAccessHiddenFromAdmins(t *testing.T) {
	resetSettingsOwnerMemo(t)
	reads := &atomic.Int64{}
	page := settingsPages[settingsPageAccess]
	original := page.MainState
	page.MainState = func(b *gotgbot.Bot, tr *i18n.Translator, group *gotgbot.Chat) (string, bool) {
		reads.Add(1)
		return original(b, tr, group)
	}
	settingsPages[settingsPageAccess] = page
	t.Cleanup(func() {
		restored := settingsPages[settingsPageAccess]
		restored.MainState = original
		settingsPages[settingsPageAccess] = restored
	})

	t.Run("a group menu of an allowed admin", func(t *testing.T) {
		env := newSettingsEnv(t)
		reads.Store(0)
		msgID := env.openMenu(env.admin)
		text, keyboard := env.lastEdit(env.chat.Id, msgID)
		if _, ok := settingsPageButton(t, keyboard, settingsPageAccess); ok {
			t.Errorf("the admin's Main page has an Access button: %+v", keyboard)
		}
		if settingsAccessHasText(text) {
			t.Errorf("the admin's Main page %q carries Access text", text)
		}
		if got := reads.Load(); got != 0 {
			t.Errorf("the Access state was read %d times for an admin, want 0", got)
		}
	})

	t.Run("a private menu of an allowed admin", func(t *testing.T) {
		env := newSettingsEnv(t)
		reads.Store(0)
		env.sendPrivate(env.admin, fmt.Sprintf("/start settings_%d", env.chat.Id))
		sent := settingsPrivateOnlySent(t, env, env.admin)
		keyboard := staffKeyboardOf(sent.Params["reply_markup"])
		if _, ok := settingsPageButton(t, keyboard, settingsPageAccess); ok {
			t.Errorf("the admin's private Main page has an Access button: %+v", keyboard)
		}
		if settingsAccessHasText(fmt.Sprint(sent.Params["text"])) {
			t.Errorf("the admin's private Main page carries Access text")
		}
		if got := reads.Load(); got != 0 {
			t.Errorf("the Access state was read %d times for an admin, want 0", got)
		}
	})

	t.Run("the owner sees it in both", func(t *testing.T) {
		env := newSettingsEnv(t)
		owner := env.newOwner(env.chat)
		reads.Store(0)
		msgID := env.openMenu(owner)
		text, keyboard := env.lastEdit(env.chat.Id, msgID)
		if _, ok := settingsPageButton(t, keyboard, settingsPageAccess); !ok {
			t.Errorf("the owner's Main page has no Access button: %+v", keyboard)
		}
		if !strings.Contains(text, staffMarker("settings_page_access")) && !strings.Contains(fmt.Sprint(keyboard), staffMarker("settings_page_access")) {
			t.Errorf("the owner's Main page %q does not name Access", text)
		}
		if reads.Load() == 0 {
			t.Error("the owner's Main page never read the Access state")
		}

		env.sendPrivate(owner, fmt.Sprintf("/start settings_%d", env.chat.Id))
		sent := settingsPrivateOnlySent(t, env, owner)
		if _, ok := settingsPageButton(t, staffKeyboardOf(sent.Params["reply_markup"]), settingsPageAccess); !ok {
			t.Error("the owner's private Main page has no Access button")
		}
	})
}

func TestSettingsAccessForgedPresses(t *testing.T) {
	resetSettingsOwnerMemo(t)
	env := newSettingsEnv(t)
	writes := settingsAccessCountWrites(t)
	env.setAdminsAllowed(env.chat.Id, true)
	msgID := env.openMenu(env.admin)

	forged := []struct {
		name   string
		fields []string
	}{
		{"open", []string{"a", "op", "p", "a"}},
		{"cancel", []string{"a", "no", "p", "a"}},
		{"toggle off", []string{"a", "tg", "p", "a", "t", "s", "v", "0"}},
		{"set on", []string{"a", "st", "p", "a", "t", "s", "v", "1"}},
		{"set off", []string{"a", "st", "p", "a", "t", "s", "v", "0"}},
		{"toggle on", []string{"a", "tg", "p", "a", "t", "s", "v", "1"}},
		{"yes", []string{"a", "ay", "e", settingsCardExpiry(time.Now())}},
	}
	for _, press := range forged {
		t.Run(press.name, func(t *testing.T) {
			editsBefore := len(env.edits(env.chat.Id, msgID))
			answersBefore := env.answerCount()

			env.press(env.admin, msgID, env.groupFields(env.admin, press.fields...))

			if got := env.answerCount() - answersBefore; got != 1 {
				t.Errorf("the press was answered %d times, want once", got)
			}
			if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("settings_access_owner_only")) {
				t.Errorf("answer = %q (alert %v), want the alert settings_access_owner_only", answer, alert)
			}
			if got := len(env.edits(env.chat.Id, msgID)); got != editsBefore {
				t.Errorf("the forged press edited the menu (%d edits, was %d)", got, editsBefore)
			}
			if !settingsAccessStored(t, env.chat.Id) {
				t.Error("the forged press changed the stored switch")
			}
		})
	}
	if got := writes.Load(); got != 0 {
		t.Errorf("forged presses made %d writes, want 0", got)
	}
}

func TestSettingsAccessCard(t *testing.T) {
	t.Run("set to 1 draws the card like toggle and writes nothing", func(t *testing.T) {
		resetSettingsOwnerMemo(t)
		env := newSettingsEnv(t)
		owner := env.newOwner(env.chat)
		env.setAdminsAllowed(env.chat.Id, false)
		writes := settingsAccessCountWrites(t)
		msgID := settingsAccessOwnerOpen(t, env, owner)

		env.press(owner, msgID, env.groupFields(owner, "a", "st", "p", "a", "t", "s", "v", "1"))

		text, _ := env.lastEdit(env.chat.Id, msgID)
		if !strings.Contains(text, staffMarker("settings_access_confirm")) {
			t.Errorf("st v=1 drew %q, want the card", text)
		}
		if settingsAccessStored(t, env.chat.Id) || writes.Load() != 0 {
			t.Error("st v=1 wrote ON")
		}

		env.press(owner, msgID, env.groupFields(owner, "a", "st", "p", "a", "t", "s", "v", "2"))
		if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("common_callback_invalid_request")) {
			t.Errorf("v=2 answer = %q (alert %v), want an invalid request", answer, alert)
		}
	})

	t.Run("No returns to the page", func(t *testing.T) {
		resetSettingsOwnerMemo(t)
		env := newSettingsEnv(t)
		owner := env.newOwner(env.chat)
		env.setAdminsAllowed(env.chat.Id, false)
		msgID := settingsAccessOwnerOpen(t, env, owner)
		settingsAccessPressToggle(t, env, owner, msgID)
		_, keyboard := env.lastEdit(env.chat.Id, msgID)
		no, ok := settingsAccessFind(t, keyboard, func(f map[string]string) bool { return f["a"] == "no" })
		if !ok {
			t.Fatalf("the card has no No: %+v", keyboard)
		}
		env.pressRaw(env.chat, owner, msgID, no.CallbackData)
		text, _ := env.lastEdit(env.chat.Id, msgID)
		if !strings.Contains(text, staffMarker("settings_access_off_desc")) || settingsAccessStored(t, env.chat.Id) {
			t.Errorf("No drew %q or wrote ON", text)
		}
	})

	t.Run("an expired Yes redraws the page and writes nothing", func(t *testing.T) {
		resetSettingsOwnerMemo(t)
		env := newSettingsEnv(t)
		owner := env.newOwner(env.chat)
		env.setAdminsAllowed(env.chat.Id, false)
		writes := settingsAccessCountWrites(t)
		previous := settingsCardLifetime
		settingsCardLifetime = -10 * time.Minute
		t.Cleanup(func() { settingsCardLifetime = previous })
		msgID := settingsAccessOwnerOpen(t, env, owner)
		settingsAccessPressToggle(t, env, owner, msgID)

		settingsAccessPressYes(t, env, owner, msgID)

		text, _ := env.lastEdit(env.chat.Id, msgID)
		if !strings.Contains(text, staffMarker("settings_card_expired")) || !strings.Contains(text, staffMarker("settings_access_off_desc")) {
			t.Errorf("an expired Yes drew %q, want the page with settings_card_expired", text)
		}
		if settingsAccessStored(t, env.chat.Id) || writes.Load() != 0 {
			t.Error("an expired Yes wrote ON")
		}
	})

	t.Run("a Yes with a bad e is an invalid request", func(t *testing.T) {
		resetSettingsOwnerMemo(t)
		env := newSettingsEnv(t)
		owner := env.newOwner(env.chat)
		env.setAdminsAllowed(env.chat.Id, false)
		writes := settingsAccessCountWrites(t)
		msgID := settingsAccessOwnerOpen(t, env, owner)
		for _, fields := range [][]string{
			{"a", "ay"},
			{"a", "ay", "e", "12a"},
			{"a", "ay", "e", "0"},
			{"a", "ay", "e", "99999999999999999999"},
		} {
			editsBefore := len(env.edits(env.chat.Id, msgID))
			env.press(owner, msgID, env.groupFields(owner, fields...))
			if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("common_callback_invalid_request")) {
				t.Errorf("%v: answer = %q (alert %v), want an invalid request", fields, answer, alert)
			}
			if got := len(env.edits(env.chat.Id, msgID)); got != editsBefore {
				t.Errorf("%v edited the menu", fields)
			}
		}
		if settingsAccessStored(t, env.chat.Id) || writes.Load() != 0 {
			t.Error("a bad Yes wrote ON")
		}
	})

	t.Run("a Yes after the owner was demoted writes nothing", func(t *testing.T) {
		resetSettingsOwnerMemo(t)
		env := newSettingsEnv(t)
		owner := env.newOwner(env.chat)
		env.setAdminsAllowed(env.chat.Id, false)
		writes := settingsAccessCountWrites(t)
		msgID := settingsAccessOwnerOpen(t, env, owner)
		settingsAccessPressToggle(t, env, owner, msgID)

		// Another user becomes the creator and the old owner an administrator.
		env.newOwner(env.chat)
		env.fake.setMember(env.chat.Id, owner.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusAdministrator})
		env.listAdmin(env.chat.Id, owner.Id)
		editsBefore := len(env.edits(env.chat.Id, msgID))

		settingsAccessPressYes(t, env, owner, msgID)

		answer, alert := env.lastAnswer()
		refused := false
		for _, key := range []string{"settings_access_owner_only", "settings_gate_owner_only", "settings_not_admin"} {
			if strings.Contains(answer, staffMarker(key)) {
				refused = true
			}
		}
		if !alert || !refused {
			t.Errorf("answer = %q (alert %v), want a refusal alert", answer, alert)
		}
		if got := len(env.edits(env.chat.Id, msgID)); got != editsBefore {
			t.Error("the refused Yes edited the menu")
		}
		if settingsAccessStored(t, env.chat.Id) || writes.Load() != 0 {
			t.Error("a Yes of a demoted owner wrote ON")
		}
	})

	t.Run("repeated taps write the same value again", func(t *testing.T) {
		resetSettingsOwnerMemo(t)
		env := newSettingsEnv(t)
		owner := env.newOwner(env.chat)
		env.setAdminsAllowed(env.chat.Id, false)
		msgID := settingsAccessOwnerOpen(t, env, owner)
		settingsAccessPressToggle(t, env, owner, msgID)
		_, keyboard := env.lastEdit(env.chat.Id, msgID)
		yes, _ := settingsAccessFind(t, keyboard, func(f map[string]string) bool { return f["a"] == settingsActAccessYes })

		env.pressRaw(env.chat, owner, msgID, yes.CallbackData)
		env.pressRaw(env.chat, owner, msgID, yes.CallbackData)
		if !settingsAccessStored(t, env.chat.Id) {
			t.Fatal("a double Yes left the switch off")
		}
		off := map[string]string{"a": "tg", "p": "a", "t": "s", "v": "0"}
		env.press(owner, msgID, env.groupFields(owner, "a", off["a"], "p", off["p"], "t", off["t"], "v", off["v"]))
		env.press(owner, msgID, env.groupFields(owner, "a", off["a"], "p", off["p"], "t", off["t"], "v", off["v"]))
		if settingsAccessStored(t, env.chat.Id) {
			t.Fatal("a double OFF left the switch on")
		}
		text, _ := env.lastEdit(env.chat.Id, msgID)
		if !strings.Contains(text, staffMarker("settings_access_off_desc")) {
			t.Errorf("after a double OFF the page drew %q", text)
		}
	})
}

func TestSettingsAccessWriteFailure(t *testing.T) {
	resetSettingsOwnerMemo(t)
	env := newSettingsEnv(t)
	owner := env.newOwner(env.chat)
	env.setAdminsAllowed(env.chat.Id, false)
	settingsAccessSeams(t)
	settingsAccessWrite = func(context.Context, int64, bool, int64) error { return errors.New("database is down") }
	msgID := settingsAccessOwnerOpen(t, env, owner)

	settingsAccessTurnOn(t, env, owner, msgID)

	text, keyboard := env.lastEdit(env.chat.Id, msgID)
	if !strings.Contains(text, settingsNoticeWarnIcon+staffMarker("common_settings_save_failed")) {
		t.Errorf("a failed ON drew %q, want the save-failed notice", text)
	}
	if !strings.Contains(text, staffMarker("settings_access_off_desc")) || settingsAccessStored(t, env.chat.Id) {
		t.Errorf("a failed ON drew %q or changed the stored switch", text)
	}
	if _, target := settingsAccessToggle(t, keyboard); target != "1" {
		t.Errorf("after a failed ON the toggle sends v=%s, want 1", target)
	}

	// A failed OFF keeps the switch on.
	settingsAccessLookup, settingsAccessWrite = menuaccess.AdminsAllowedFresh, menuaccess.SetAdminsAllowed
	env.setAdminsAllowed(env.chat.Id, true)
	settingsAccessWrite = func(context.Context, int64, bool, int64) error { return errors.New("database is down") }
	env.press(owner, msgID, env.groupFields(owner, "a", "op", "p", settingsPageAccess))
	settingsAccessPressToggle(t, env, owner, msgID)
	text, _ = env.lastEdit(env.chat.Id, msgID)
	if !strings.Contains(text, staffMarker("common_settings_save_failed")) || !strings.Contains(text, staffMarker("settings_access_on_desc")) {
		t.Errorf("a failed OFF drew %q, want the notice and the on state", text)
	}
	if !settingsAccessStored(t, env.chat.Id) {
		t.Error("a failed OFF changed the stored switch")
	}
}

func TestSettingsAccessReadFailure(t *testing.T) {
	resetSettingsOwnerMemo(t)
	env := newSettingsEnv(t)
	owner := env.newOwner(env.chat)
	settingsAccessSeams(t)
	settingsAccessLookup = func(context.Context, int64) (bool, error) { return false, errors.New("database is down") }

	msgID := env.openMenu(owner)
	_, main := env.lastEdit(env.chat.Id, msgID)
	button, ok := settingsPageButton(t, main, settingsPageAccess)
	if !ok {
		t.Fatalf("the Main page has no Access button: %+v", main)
	}
	if !strings.Contains(button.Text, settingsUnknownState) {
		t.Errorf("the Access button %q does not show the unknown mark", button.Text)
	}

	env.pressRaw(env.chat, owner, msgID, button.CallbackData)
	if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("settings_cannot_open_alert")) {
		t.Errorf("answer = %q (alert %v), want settings_cannot_open_alert", answer, alert)
	}
	text, keyboard := env.lastEdit(env.chat.Id, msgID)
	if !strings.Contains(text, staffMarker("settings_cannot_open")) || len(keyboard) != 0 {
		t.Errorf("the page drew %q with %d rows, want the cannot-open edit with no keyboard", text, len(keyboard))
	}
}

func TestSettingsAccessNoAnnouncement(t *testing.T) {
	resetSettingsOwnerMemo(t)
	env := newSettingsEnv(t)
	owner := env.newOwner(env.chat)
	env.setAdminsAllowed(env.chat.Id, false)
	msgID := settingsAccessOwnerOpen(t, env, owner)
	sentBefore := len(env.fake.callsFor("sendMessage"))

	settingsAccessTurnOn(t, env, owner, msgID)
	if !settingsAccessStored(t, env.chat.Id) {
		t.Fatal("the flip to ON did not happen")
	}
	settingsAccessPressToggle(t, env, owner, msgID)
	if settingsAccessStored(t, env.chat.Id) {
		t.Fatal("the flip to OFF did not happen")
	}

	if got := len(env.fake.callsFor("sendMessage")); got != sentBefore {
		t.Errorf("flipping the switch sent %d messages, want none", got-sentBefore)
	}
}

func TestSettingsAccessAcrossReplicas(t *testing.T) {
	resetSettingsOwnerMemo(t)
	a := newSettingsEnv(t)
	owner := a.newOwner(a.chat)
	a.setAdminsAllowed(a.chat.Id, false)

	b := a.otherReplica()
	b.fake.setMember(b.chat.Id, owner.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusCreator})
	b.fake.setCreator(b.chat.Id, owner.Id)
	b.fake.listAdministrator(b.chat.Id, b.admin.Id)

	const bMsg = 9001
	adminPassesOnB := func() bool {
		before := len(b.edits(b.chat.Id, bMsg))
		b.press(b.admin, bMsg, b.groupFields(b.admin, "a", "bk"))
		return len(b.edits(b.chat.Id, bMsg)) == before+1
	}

	msgID := settingsAccessOwnerOpen(t, a, owner)
	settingsAccessTurnOn(t, a, owner, msgID)
	if !adminPassesOnB() {
		t.Fatal("after the owner turned ON on replica A, the admin's next press on B was refused")
	}

	settingsAccessPressToggle(t, a, owner, msgID)
	if adminPassesOnB() {
		t.Fatal("after the owner turned OFF on replica A, the admin's very next press on B passed")
	}
	if answer, alert := b.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("settings_gate_owner_only")) {
		t.Errorf("replica B answered %q (alert %v), want settings_gate_owner_only", answer, alert)
	}

	// B has now memoised "an owner exists"; a flip to ON must still be seen at once, with no
	// owner-state read on B.
	listCalls := settingsAdminListCalls(b, b.chat.Id)
	settingsAccessPressToggle(t, a, owner, msgID)
	settingsAccessPressYes(t, a, owner, msgID)
	if !adminPassesOnB() {
		t.Fatal("a memoised owner on replica B hid a flip to ON")
	}
	if got := settingsAdminListCalls(b, b.chat.Id); got != listCalls {
		t.Errorf("replica B made %d getChatAdministrators calls after the flip, want none", got-listCalls)
	}
}

func TestSettingsAccessNoRowIsOwnerOnly(t *testing.T) {
	resetSettingsOwnerMemo(t)
	env := newSettingsEnv(t)
	owner := env.newOwner(env.chat)
	settingsDropAccessRow(t, env.chat.Id)
	other := env.otherReplica()
	other.fake.setMember(other.chat.Id, owner.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusCreator})
	other.fake.setCreator(other.chat.Id, owner.Id)
	other.fake.listAdministrator(other.chat.Id, other.admin.Id)

	for name, e := range map[string]*settingsEnv{"replica A": env, "replica B": other} {
		before := len(e.edits(e.chat.Id, 9100))
		e.press(e.admin, 9100, e.groupFields(e.admin, "a", "bk"))
		if answer, alert := e.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("settings_gate_owner_only")) {
			t.Errorf("%s: answer = %q (alert %v), want settings_gate_owner_only", name, answer, alert)
		}
		if got := len(e.edits(e.chat.Id, 9100)); got != before {
			t.Errorf("%s: a chat without a row let the admin in", name)
		}
	}
}

func TestSettingsDriftAccess(t *testing.T) {
	resetSettingsOwnerMemo(t)
	env := newSettingsEnv(t)
	owner := env.newOwner(env.chat)
	msgID := env.openMenu(owner)

	for _, allowed := range []bool{true, false, true} {
		if err := menuaccess.SetAdminsAllowed(context.Background(), env.chat.Id, allowed, owner.Id); err != nil {
			t.Fatalf("SetAdminsAllowed(%t) error = %v", allowed, err)
		}
		env.press(owner, msgID, env.groupFields(owner, "a", "op", "p", settingsPageAccess))
		text, keyboard := env.lastEdit(env.chat.Id, msgID)
		wantDesc, wantTarget := "settings_access_off_desc", "1"
		if allowed {
			wantDesc, wantTarget = "settings_access_on_desc", "0"
		}
		if !strings.Contains(text, staffMarker(wantDesc)) {
			t.Errorf("stored %t: the page drew %q, want %s", allowed, text, wantDesc)
		}
		if _, target := settingsAccessToggle(t, keyboard); target != wantTarget {
			t.Errorf("stored %t: the toggle sends v=%s, want %s", allowed, target, wantTarget)
		}
		state, ok := settingsAccessMainState(env.bot, ctxTrForTest(), &env.chat)
		if !ok || state == "" {
			t.Errorf("stored %t: the Main state is %q, ok %v", allowed, state, ok)
		}
	}
}

// ctxTrForTest is the marker-locale translator the menu tests draw with.
func ctxTrForTest() *i18n.Translator {
	return i18n.MustNewTranslator("en")
}

func TestSettingsOwnerAuthorityAccess(t *testing.T) {
	for status, wantOwner := range map[string]bool{
		gotgbot.ChatMemberStatusCreator:       true,
		gotgbot.ChatMemberStatusAdministrator: false,
		gotgbot.ChatMemberStatusMember:        false,
		gotgbot.ChatMemberStatusRestricted:    false,
		gotgbot.ChatMemberStatusLeft:          false,
		gotgbot.ChatMemberStatusKicked:        false,
		"":                                    false,
	} {
		t.Run("status "+status, func(t *testing.T) {
			press := &settingsPress{member: gotgbot.MergedChatMember{Status: status}}
			key, alert := settingsRightRefusal(press, settingsRightOwner)
			if wantOwner && key != "" {
				t.Errorf("the right refused a creator with %q", key)
			}
			if !wantOwner && (key != "settings_access_owner_only" || !alert) {
				t.Errorf("the right gave (%q, alert %v) for %q, want the owner-only alert", key, alert, status)
			}

			writes := settingsAccessCountWrites(t)
			env := newSettingsEnv(t)
			result := applyAdminsAllowed(context.Background(), status, env.chat.Id, true, 7)
			if wantOwner {
				if result.Outcome != accessApplied || !result.Allowed || writes.Load() != 1 || !settingsAccessStored(t, env.chat.Id) {
					t.Errorf("a creator's apply = %+v with %d writes", result, writes.Load())
				}
				return
			}
			if result.Outcome != accessRefusedNotOwner || result.Allowed || writes.Load() != 0 {
				t.Errorf("a %q apply = %+v with %d writes, want a refusal and no write", status, result, writes.Load())
			}
		})
	}
}

func TestSettingsAccessCallbackBytes(t *testing.T) {
	tr := i18n.MustNewTranslator("en")
	for _, view := range []settingsView{
		{Tr: tr, Private: true, OwnerKey: "c", OwnerValue: "-1001234567890", GroupID: -1001234567890, GroupTitle: "G", OpenerName: "N", Owner: true},
		{Tr: tr, OwnerKey: "u", OwnerValue: "1234567890", GroupID: -1001234567890, GroupTitle: "G", OpenerName: "N", Owner: true},
	} {
		name := "group"
		if view.Private {
			name = "private"
		}
		t.Run(name, func(t *testing.T) {
			screens := map[string]settingsScreen{
				"card": renderSettingsAccessCard(view),
			}
			for _, allowed := range []bool{false, true} {
				screens[fmt.Sprintf("page allowed %t", allowed)] = renderSettingsAccessPage(view, allowed, "")
			}
			for label, screen := range screens {
				if screen.Omitted != 0 {
					t.Errorf("%s omitted %d buttons", label, screen.Omitted)
				}
				buttons := settingsAllButtons(screen.Keyboard.InlineKeyboard)
				if len(buttons) == 0 {
					t.Errorf("%s has no buttons", label)
				}
				for _, button := range buttons {
					if n := len(button.CallbackData); n < 1 || n > 64 {
						t.Errorf("%s button %q has %d bytes of callback data, want 1..64", label, button.Text, n)
					}
				}
			}
		})
	}
}

func TestSettingsAccessPageFitsEveryLocale(t *testing.T) {
	withRealLocales(t)
	// 64 runes of characters that grow when escaped or take two UTF-16 units.
	title := strings.Repeat("&<ж😀", 16)
	for _, code := range []string{"uk", "en", "es", "fr", "hi", "id", "pt", "ru"} {
		t.Run(code, func(t *testing.T) {
			tr := i18n.MustNewTranslator(code)
			notice := settingsNoticeWarn(settingsText(tr, "common_settings_save_failed"))
			if expired := settingsNoticeWarn(settingsText(tr, "settings_card_expired")); len(expired) > len(notice) {
				notice = expired
			}
			if notice == "" {
				t.Fatalf("%s: no notice text", code)
			}
			for _, view := range []settingsView{
				{Tr: tr, OwnerKey: "u", OwnerValue: "1234567890", GroupID: -1001234567890, GroupTitle: title, OpenerName: "N", Owner: true},
				{Tr: tr, Private: true, OwnerKey: "c", OwnerValue: "-1001234567890", GroupID: -1001234567890, GroupTitle: title, OpenerName: "N", Owner: true},
			} {
				screens := []settingsScreen{
					renderSettingsAccessCard(view),
					renderSettingsAccessPage(view, false, notice),
					renderSettingsAccessPage(view, true, notice),
				}
				for _, screen := range screens {
					if got := panelUTF16Len(screen.Text); got > 3800 {
						t.Errorf("%s: a screen is %d UTF-16 units, want at most 3800", code, got)
					}
					for _, button := range settingsAllButtons(screen.Keyboard.InlineKeyboard) {
						if strings.TrimSpace(button.Text) == "" {
							t.Errorf("%s: a button has an empty label (data %q)", code, button.CallbackData)
						}
					}
				}
				for _, allowed := range []bool{false, true} {
					label := settingsText(tr, "settings_btn_access_toggle", i18n.TranslationParams{"state": settingsStateWord(tr, allowed)})
					if n := utf8.RuneCountInString(label); n > 40 {
						t.Errorf("%s: the toggle %q is %d code points, want at most 40", code, label, n)
					}
				}
			}
		})
	}
}
