//go:build testtools

package modules

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db/lockdown"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/i18n"
)

// settingsLiftTestButton returns the first button of keyboard whose label carries the marker
// of key, failing the test when there is none.
func settingsLiftTestButton(t *testing.T, keyboard [][]gotgbot.InlineKeyboardButton, key string) gotgbot.InlineKeyboardButton {
	t.Helper()
	button, ok := settingsButtonByMarker(keyboard, key)
	if !ok {
		t.Fatalf("keyboard %+v has no button labelled %s", keyboard, key)
	}
	return button
}

// settingsLiftTestHasButton reports whether keyboard has a button labelled with key.
func settingsLiftTestHasButton(keyboard [][]gotgbot.InlineKeyboardButton, key string) bool {
	_, ok := settingsButtonByMarker(keyboard, key)
	return ok
}

// settingsLiftTestAsk presses the Lift button of the page on msgID as opener and returns the
// text and keyboard the message was edited to.
func settingsLiftTestAsk(t *testing.T, env *settingsEnv, opener gotgbot.User, msgID int64) (string, [][]gotgbot.InlineKeyboardButton) {
	t.Helper()
	_, keyboard := env.lastEdit(env.chat.Id, msgID)
	lift := settingsLiftTestButton(t, keyboard, "lockdown_alert_btn_lift")
	env.pressRaw(env.chat, opener, msgID, lift.CallbackData)
	return env.lastEdit(env.chat.Id, msgID)
}

// settingsLiftTestRestores counts the setChatPermissions calls made for the env's chat so far.
func settingsLiftTestRestores(env *settingsEnv) int {
	return len(env.calls("setChatPermissions"))
}

func TestSettingsLockdownLift(t *testing.T) {
	env := newSettingsEnv(t)
	row := env.lockAsAdmin("raid")
	if row.ChatTitle == "" {
		t.Fatal("setup: the lockdown stored no chat title")
	}
	msgID := settingsLockdownTestOpen(t, env)
	owner := fmt.Sprint(env.admin.Id)

	_, keyboard := env.lastEdit(env.chat.Id, msgID)
	if len(keyboard) == 0 || len(keyboard[0]) != 1 || !strings.Contains(keyboard[0][0].Text, staffMarker("lockdown_alert_btn_lift")) {
		t.Fatalf("first content row = %+v, want the single Lift button", keyboard)
	}
	if fields := settingsButtonFields(t, keyboard[0][0]); fmt.Sprint(fields) != fmt.Sprint(map[string]string{"a": "ls", "u": owner}) {
		t.Errorf("Lift button fields = %v, want a=ls and the opener", fields)
	}

	restoresBefore := settingsLiftTestRestores(env)
	cardText, card := settingsLiftTestAsk(t, env, env.admin, msgID)
	for _, want := range []string{staffMarker("lockdown_alert_lift_ask"), row.ChatTitle} {
		if !strings.Contains(cardText, want) {
			t.Errorf("Lift card %q does not contain %q", cardText, want)
		}
	}
	if len(card) != 2 || len(card[0]) != 2 || len(card[1]) != 2 {
		t.Fatalf("Lift card keyboard = %+v, want [Yes No] and [Back Close]", card)
	}
	yes, no := card[0][0], card[0][1]
	if !strings.Contains(yes.Text, staffMarker("settings_btn_yes")) || !strings.Contains(no.Text, staffMarker("settings_btn_no")) {
		t.Errorf("card buttons = %q, %q, want Yes then No", yes.Text, no.Text)
	}
	yesFields := settingsButtonFields(t, yes)
	if yesFields["a"] != "ly" || yesFields["u"] != owner || !regexp.MustCompile(`^\d+$`).MatchString(yesFields["e"]) || len(yesFields) != 3 {
		t.Errorf("Yes fields = %v, want a=ly, the opener and a numeric expiry", yesFields)
	}
	if fields := settingsButtonFields(t, no); fmt.Sprint(fields) != fmt.Sprint(map[string]string{"a": "no", "p": "l", "u": owner}) {
		t.Errorf("No fields = %v, want a=no back to page l", fields)
	}
	if got := settingsLiftTestRestores(env); got != restoresBefore {
		t.Errorf("asking made %d restore calls, want none", got-restoresBefore)
	}
	if active, err := lockdown.GetActiveFresh(env.chat.Id); err != nil || active == nil {
		t.Fatalf("GetActiveFresh = %v, %v, want the lockdown still active after the ask", active, err)
	}

	// No goes back to the Lockdown page and lifts nothing.
	env.pressRaw(env.chat, env.admin, msgID, no.CallbackData)
	pageText, pageKeyboard := env.lastEdit(env.chat.Id, msgID)
	if !strings.Contains(pageText, staffMarker("lockdown_status_active")) || !settingsLiftTestHasButton(pageKeyboard, "lockdown_alert_btn_lift") {
		t.Errorf("after No the message is %q with keyboard %+v, want the Lockdown page with its Lift button", pageText, pageKeyboard)
	}
	if got := settingsLiftTestRestores(env); got != restoresBefore {
		t.Errorf("No made %d restore calls, want none", got-restoresBefore)
	}

	// Yes answers first, then lifts through the path /unlockdown uses.
	_, card = settingsLiftTestAsk(t, env, env.admin, msgID)
	yes = settingsLiftTestButton(t, card, "settings_btn_yes")
	var mu sync.Mutex
	answersAtRestore := -1
	env.fake.setOnSetPermissions(func(int64) {
		n := env.answerCount()
		mu.Lock()
		defer mu.Unlock()
		answersAtRestore = n
	})
	answersBefore := env.answerCount()
	lockdownApplyDrainWake()
	env.pressRaw(env.chat, env.admin, msgID, yes.CallbackData)

	if got := env.answerCount() - answersBefore; got != 1 {
		t.Errorf("the Yes press was answered %d times, want exactly once", got)
	}
	mu.Lock()
	if answersAtRestore != answersBefore+1 {
		t.Errorf("answers at the restore call = %d, want %d: Yes answers before it lifts", answersAtRestore, answersBefore+1)
	}
	mu.Unlock()
	if got := settingsLiftTestRestores(env) - restoresBefore; got != 1 {
		t.Errorf("Yes made %d restore calls, want exactly the one liftLockdownRow makes", got)
	}
	if got := env.fake.chatPermsRaw(env.chat.Id); got != lockdownTestPrePermissions {
		t.Errorf("permissions after the lift = %q, want the stored pre-lockdown set", got)
	}
	lifted := env.freshRow(row.ID)
	if lifted.State != models.LockdownStateLifted && lifted.State != models.LockdownStateLifting {
		t.Errorf("State = %q, want lifting or lifted", lifted.State)
	}
	if lifted.LiftedBy == nil || *lifted.LiftedBy != env.admin.Id {
		t.Errorf("LiftedBy = %v, want the presser %d", lifted.LiftedBy, env.admin.Id)
	}
	// A menu opened in the group redraws in the group with the lift as its notice (checked
	// below), so a second public message would say it twice (WR-04). The worker is still woken.
	if got := env.repliesWith(staffMarker("lockdown_lifted")); len(got) != 0 {
		t.Errorf("%d public lift notices reached the group beside the redrawn menu, want none", len(got))
	}
	if !lockdownApplyDrainWake() {
		t.Error("the lockdown worker was not woken after a lift from a group menu")
	}
	finalText, finalKeyboard := env.lastEdit(env.chat.Id, msgID)
	if !strings.Contains(finalText, staffMarker("lockdown_lifted")) {
		t.Errorf("redrawn page %q does not carry the lift result as a notice", finalText)
	}
	if settingsLiftTestHasButton(finalKeyboard, "lockdown_alert_btn_lift") {
		t.Errorf("the page still offers Lift after the lockdown was lifted: %+v", finalKeyboard)
	}
}

func TestSettingsLockdownLiftOutcomes(t *testing.T) {
	t.Run("no lockdown redraws with the not-active notice and no card", func(t *testing.T) {
		env := newSettingsEnv(t)
		msgID := settingsLockdownTestOpen(t, env)
		env.press(env.admin, msgID, env.groupFields(env.admin, "a", "ls"))

		text, keyboard := env.lastEdit(env.chat.Id, msgID)
		if !strings.Contains(text, staffMarker("lockdown_not_active")) {
			t.Errorf("page %q does not carry %s", text, staffMarker("lockdown_not_active"))
		}
		if settingsLiftTestHasButton(keyboard, "settings_btn_yes") {
			t.Errorf("a card was drawn without a lockdown: %+v", keyboard)
		}
		if got := settingsLiftTestRestores(env); got != 0 {
			t.Errorf("%d restore calls with no lockdown", got)
		}
	})

	t.Run("a refused restore keeps the lockdown and posts no notice", func(t *testing.T) {
		env := newSettingsEnv(t)
		env.lockAsAdmin("raid")
		msgID := settingsLockdownTestOpen(t, env)
		_, card := settingsLiftTestAsk(t, env, env.admin, msgID)
		yes := settingsLiftTestButton(t, card, "settings_btn_yes")
		env.fake.script("setChatPermissions", env.chat.Id,
			staffFakeError(400, "Bad Request: not enough rights to change chat permissions"))

		env.pressRaw(env.chat, env.admin, msgID, yes.CallbackData)

		text, keyboard := env.lastEdit(env.chat.Id, msgID)
		if !strings.Contains(text, staffMarker("lockdown_restore_failed")) || strings.Contains(text, "not enough rights") {
			t.Errorf("page %q, want the plain restore failure", text)
		}
		if active, err := lockdown.GetActiveFresh(env.chat.Id); err != nil || active == nil {
			t.Errorf("GetActiveFresh = %v, %v, want the lockdown still active", active, err)
		}
		if got := env.repliesWith(staffMarker("lockdown_lifted")); len(got) != 0 {
			t.Errorf("%d public lift notices after a failed restore, want none", len(got))
		}
		if !settingsLiftTestHasButton(keyboard, "lockdown_alert_btn_lift") {
			t.Errorf("a lockdown that is still active lost its Lift button: %+v", keyboard)
		}
	})

	t.Run("a failed record keeps the lockdown and posts no notice", func(t *testing.T) {
		env := newSettingsEnv(t)
		env.lockAsAdmin("raid")
		msgID := settingsLockdownTestOpen(t, env)
		_, card := settingsLiftTestAsk(t, env, env.admin, msgID)
		yes := settingsLiftTestButton(t, card, "settings_btn_yes")
		previous := lockdownBeginLift
		lockdownBeginLift = func(uint, int64, string, bool) (bool, error) { return false, errors.New("database is down") }
		t.Cleanup(func() { lockdownBeginLift = previous })

		env.pressRaw(env.chat, env.admin, msgID, yes.CallbackData)

		text, _ := env.lastEdit(env.chat.Id, msgID)
		if !strings.Contains(text, staffMarker("lockdown_lift_record_failed")) {
			t.Errorf("page %q does not carry %s", text, staffMarker("lockdown_lift_record_failed"))
		}
		if active, err := lockdown.GetActiveFresh(env.chat.Id); err != nil || active == nil {
			t.Errorf("GetActiveFresh = %v, %v, want the lockdown still active", active, err)
		}
		if got := env.repliesWith(staffMarker("lockdown_lifted")); len(got) != 0 {
			t.Errorf("%d public lift notices after a failed record, want none", len(got))
		}
	})

	t.Run("a lockdown being lifted shows no Lift button and a hand-built ask says so", func(t *testing.T) {
		env := newSettingsEnv(t)
		row := env.lockAsAdmin("raid")
		if began, err := lockdown.BeginLift(row.ID, env.admin.Id, "Ad<b>min", false); err != nil || !began {
			t.Fatalf("setup: BeginLift = %v, %v", began, err)
		}
		seedLockdownJoiner(t, row.ID, env.chat.Id, 9701, models.JoinerStateBanned)
		restoresBefore := settingsLiftTestRestores(env)
		msgID := settingsLockdownTestOpen(t, env)

		_, keyboard := env.lastEdit(env.chat.Id, msgID)
		if settingsLiftTestHasButton(keyboard, "lockdown_alert_btn_lift") {
			t.Errorf("a lockdown being lifted offers Lift: %+v", keyboard)
		}
		env.press(env.admin, msgID, env.groupFields(env.admin, "a", "ls"))

		text, keyboard := env.lastEdit(env.chat.Id, msgID)
		if !strings.Contains(text, staffMarker("lockdown_lift_in_progress")) {
			t.Errorf("page %q does not carry %s", text, staffMarker("lockdown_lift_in_progress"))
		}
		if settingsLiftTestHasButton(keyboard, "settings_btn_yes") {
			t.Errorf("a card was drawn for a lockdown being lifted: %+v", keyboard)
		}
		if got := settingsLiftTestRestores(env); got != restoresBefore {
			t.Errorf("%d restore calls for a lockdown already being lifted", got-restoresBefore)
		}
	})
}

// settingsLiftTestCard locks the env's chat as the administrator, opens the Lockdown page and
// asks for the Lift card. It returns the lockdown row, the menu message and the card's Yes
// button.
func settingsLiftTestCard(t *testing.T, env *settingsEnv) (*models.ChatLockdown, int64, gotgbot.InlineKeyboardButton) {
	t.Helper()
	row := env.lockAsAdmin("raid")
	msgID := settingsLockdownTestOpen(t, env)
	_, card := settingsLiftTestAsk(t, env, env.admin, msgID)
	return row, msgID, settingsLiftTestButton(t, card, "settings_btn_yes")
}

func TestSettingsLockdownLiftTwice(t *testing.T) {
	for _, tc := range []struct {
		name   string
		joiner bool
		want   string
	}{
		{"nobody to unban", false, "lockdown_not_active"},
		{"joiners still to unban", true, "lockdown_lift_in_progress"},
	} {
		t.Run("two Yes presses lift once: "+tc.name, func(t *testing.T) {
			env := newSettingsEnv(t)
			row := env.lockAsAdmin("raid")
			if tc.joiner {
				seedLockdownJoiner(t, row.ID, env.chat.Id, 9801, models.JoinerStateBanned)
			}
			msgID := settingsLockdownTestOpen(t, env)
			_, card := settingsLiftTestAsk(t, env, env.admin, msgID)
			yes := settingsLiftTestButton(t, card, "settings_btn_yes")
			restoresBefore := settingsLiftTestRestores(env)

			env.pressRaw(env.chat, env.admin, msgID, yes.CallbackData)
			env.pressRaw(env.chat, env.admin, msgID, yes.CallbackData)

			if got := settingsLiftTestRestores(env) - restoresBefore; got != 1 {
				t.Errorf("two Yes presses made %d restore calls, want 1", got)
			}
			if got := env.repliesWith(staffMarker("lockdown_lifted")); len(got) != 0 {
				t.Errorf("%d public lift notices beside the menu's own notice, want none", len(got))
			}
			lifted := env.freshRow(row.ID)
			if lifted.LiftedBy == nil || *lifted.LiftedBy != env.admin.Id {
				t.Errorf("LiftedBy = %v, want %d", lifted.LiftedBy, env.admin.Id)
			}
			text, _ := env.lastEdit(env.chat.Id, msgID)
			if !strings.Contains(text, staffMarker(tc.want)) {
				t.Errorf("the second press redrew %q, want %s", text, staffMarker(tc.want))
			}
		})
	}

	t.Run("a Yes after /unlockdown posts no second notice", func(t *testing.T) {
		env := newSettingsEnv(t)
		_, msgID, yes := settingsLiftTestCard(t, env)
		env.send(env.admin, "/unlockdown")
		restoresBefore := settingsLiftTestRestores(env)
		noticesBefore := len(env.repliesWith(staffMarker("lockdown_lifted")))
		if noticesBefore != 1 {
			t.Fatalf("setup: /unlockdown left %d lifted replies, want 1", noticesBefore)
		}

		env.pressRaw(env.chat, env.admin, msgID, yes.CallbackData)

		if got := settingsLiftTestRestores(env); got != restoresBefore {
			t.Errorf("a Yes after the lift made %d restore calls, want none", got-restoresBefore)
		}
		if got := len(env.repliesWith(staffMarker("lockdown_lifted"))); got != noticesBefore {
			t.Errorf("%d lifted messages after the Yes, want still %d", got, noticesBefore)
		}
		text, _ := env.lastEdit(env.chat.Id, msgID)
		if !strings.Contains(text, staffMarker("lockdown_not_active")) {
			t.Errorf("page %q does not say the group is not in lockdown", text)
		}
	})
}

func TestSettingsLockdownLiftExpired(t *testing.T) {
	env := newSettingsEnv(t)
	previous := settingsCardLifetime
	settingsCardLifetime = -2 * time.Minute
	t.Cleanup(func() { settingsCardLifetime = previous })
	row, msgID, yes := settingsLiftTestCard(t, env)
	restoresBefore := settingsLiftTestRestores(env)

	env.pressRaw(env.chat, env.admin, msgID, yes.CallbackData)

	text, keyboard := env.lastEdit(env.chat.Id, msgID)
	if !strings.Contains(text, staffMarker("settings_card_expired")) {
		t.Errorf("page %q does not carry %s", text, staffMarker("settings_card_expired"))
	}
	if settingsLiftTestHasButton(keyboard, "settings_btn_yes") {
		t.Errorf("an expired Yes left a card on screen: %+v", keyboard)
	}
	if got := settingsLiftTestRestores(env); got != restoresBefore {
		t.Errorf("an expired Yes made %d restore calls, want none", got-restoresBefore)
	}
	if still := env.freshRow(row.ID); still.State != models.LockdownStateActive || still.LiftedBy != nil {
		t.Errorf("row = %+v, want it active with no lifter", still)
	}
	if got := env.repliesWith(staffMarker("lockdown_lifted")); len(got) != 0 {
		t.Errorf("%d public lift notices after an expired Yes, want none", len(got))
	}
}

func TestSettingsLockdownLiftAuthority(t *testing.T) {
	// withoutRestrict makes user an administrator who may not restrict members.
	withoutRestrict := func(env *settingsEnv, user gotgbot.User) {
		env.fake.setMember(env.chat.Id, user.Id, staffFakeMember{
			Status:             gotgbot.ChatMemberStatusAdministrator,
			CanRestrictMembers: false,
		})
	}

	t.Run("an administrator without can_restrict_members gets the alert and no card", func(t *testing.T) {
		env := newSettingsEnv(t)
		row := env.lockAsAdmin("raid")
		helper := env.newOutsider()
		withoutRestrict(env, helper)
		msgID := settingsLockdownTestOpenAs(t, env, helper)
		_, keyboard := env.lastEdit(env.chat.Id, msgID)
		lift := settingsLiftTestButton(t, keyboard, "lockdown_alert_btn_lift")
		restoresBefore := settingsLiftTestRestores(env)
		edits := len(env.edits(env.chat.Id, msgID))
		answers := env.answerCount()

		env.pressRaw(env.chat, helper, msgID, lift.CallbackData)

		if got := env.answerCount() - answers; got != 1 {
			t.Errorf("the press was answered %d times, want once", got)
		}
		if text, alert := env.lastAnswer(); !alert || !strings.Contains(text, staffMarker("settings_no_restrict")) {
			t.Errorf("answer = %q (alert %v), want the settings_no_restrict alert", text, alert)
		}
		if got := len(env.edits(env.chat.Id, msgID)); got != edits {
			t.Errorf("the refused press edited the menu")
		}

		// The command refuses the same administrator, and neither restored anything.
		env.send(helper, "/unlockdown")
		env.wantReplyHas(staffMarker("chat_status_restrict_cmd_error"))
		if got := settingsLiftTestRestores(env); got != restoresBefore {
			t.Errorf("%d restore calls from a refused administrator, want none", got-restoresBefore)
		}
		if still := env.freshRow(row.ID); still.State != models.LockdownStateActive {
			t.Errorf("State = %q, want active", still.State)
		}
	})

	t.Run("a card asked by a restricting admin is refused at Yes after a demotion", func(t *testing.T) {
		env := newSettingsEnv(t)
		row, msgID, yes := settingsLiftTestCard(t, env)
		withoutRestrict(env, env.admin)
		restoresBefore := settingsLiftTestRestores(env)
		edits := len(env.edits(env.chat.Id, msgID))

		env.pressRaw(env.chat, env.admin, msgID, yes.CallbackData)

		if text, alert := env.lastAnswer(); !alert || !strings.Contains(text, staffMarker("settings_no_restrict")) {
			t.Errorf("answer = %q (alert %v), want the settings_no_restrict alert", text, alert)
		}
		if got := len(env.edits(env.chat.Id, msgID)); got != edits {
			t.Errorf("the refused Yes edited the menu")
		}
		if got := settingsLiftTestRestores(env); got != restoresBefore {
			t.Errorf("a demoted administrator's Yes made %d restore calls, want none", got-restoresBefore)
		}
		if still := env.freshRow(row.ID); still.State != models.LockdownStateActive || still.LiftedBy != nil {
			t.Errorf("row = %+v, want it active with no lifter", still)
		}
		if got := env.repliesWith(staffMarker("lockdown_lifted")); len(got) != 0 {
			t.Errorf("%d public lift notices, want none", len(got))
		}
	})

	t.Run("another restricting administrator cannot use the opener's card", func(t *testing.T) {
		env := newSettingsEnv(t)
		row, msgID, yes := settingsLiftTestCard(t, env)
		other := env.newOutsider()
		env.fake.setMember(env.chat.Id, other.Id, staffFakeMember{
			Status:             gotgbot.ChatMemberStatusAdministrator,
			CanRestrictMembers: true,
		})
		restoresBefore := settingsLiftTestRestores(env)

		env.pressRaw(env.chat, other, msgID, yes.CallbackData)

		if text, alert := env.lastAnswer(); !alert || !strings.Contains(text, staffMarker("settings_not_your_menu")) {
			t.Errorf("answer = %q (alert %v), want the settings_not_your_menu alert", text, alert)
		}
		if got := settingsLiftTestRestores(env); got != restoresBefore {
			t.Errorf("another administrator's Yes made %d restore calls, want none", got-restoresBefore)
		}
		if still := env.freshRow(row.ID); still.State != models.LockdownStateActive {
			t.Errorf("State = %q, want active", still.State)
		}
	})
}

func TestSettingsLockdownLiftPrivate(t *testing.T) {
	env := newSettingsEnv(t)
	row := env.lockAsAdmin("raid")
	env.sendPrivate(env.admin, fmt.Sprintf("/start settings_%d", env.chat.Id))
	privateMsgID := settingsPrivateOnlySent(t, env, env.admin).MessageID
	private := settingsPrivateChat(env.admin)
	env.pressPrivate(env.admin, privateMsgID, env.privateFields(env.chat.Id, "a", "op", "p", settingsPageLockdown))

	_, keyboard := env.lastEdit(env.admin.Id, privateMsgID)
	lift := settingsLiftTestButton(t, keyboard, "lockdown_alert_btn_lift")
	if fields := settingsButtonFields(t, lift); fields["c"] != fmt.Sprint(env.chat.Id) || fields["a"] != "ls" {
		t.Fatalf("private Lift fields = %v, want a=ls for the group", fields)
	}
	env.pressRaw(private, env.admin, privateMsgID, lift.CallbackData)
	_, card := env.lastEdit(env.admin.Id, privateMsgID)
	yes := settingsLiftTestButton(t, card, "settings_btn_yes")
	restoresBefore := settingsLiftTestRestores(env)

	env.pressRaw(private, env.admin, privateMsgID, yes.CallbackData)

	if got := settingsLiftTestRestores(env) - restoresBefore; got != 1 {
		t.Errorf("a private Yes made %d restore calls on the group, want 1", got)
	}
	if got := env.fake.chatPermsRaw(env.chat.Id); got != lockdownTestPrePermissions {
		t.Errorf("permissions after the lift = %q, want the stored set", got)
	}
	lifted := env.freshRow(row.ID)
	if lifted.LiftedBy == nil || *lifted.LiftedBy != env.admin.Id {
		t.Errorf("LiftedBy = %v, want %d", lifted.LiftedBy, env.admin.Id)
	}
	// The group has not been told anywhere else, so the private menu posts the one notice
	// (D-26), after the page was redrawn (WR-04).
	if got := env.repliesWith(staffMarker("lockdown_lifted")); len(got) != 1 {
		t.Errorf("%d public lift notices reached the group, want exactly 1", len(got))
	}
	if !lockdownApplyDrainWake() {
		t.Error("the lockdown worker was not woken after a lift from a private menu")
	}
	if sent := env.fake.sentTo(env.admin.Id); len(sent) != 1 {
		t.Errorf("the bot sent %d messages to the private chat, want only the menu itself", len(sent))
	}
	text, _ := env.lastEdit(env.admin.Id, privateMsgID)
	if !strings.Contains(text, staffMarker("lockdown_lifted")) {
		t.Errorf("private page %q does not carry the lift result", text)
	}
}

func TestSettingsParityLockdownLift(t *testing.T) {
	for _, withJoiner := range []bool{false, true} {
		t.Run(fmt.Sprintf("joiners to unban %v", withJoiner), func(t *testing.T) {
			env := newSettingsEnv(t)
			commandChat := env.chat
			menuChat := env.addChat()
			commandRow := env.lockAsAdmin("raid")
			menuRow := env.lockIn(menuChat)
			if withJoiner {
				seedLockdownJoiner(t, commandRow.ID, commandChat.Id, 9811, models.JoinerStateBanned)
				seedLockdownJoiner(t, menuRow.ID, menuChat.Id, 9811, models.JoinerStateBanned)
			}

			env.send(env.admin, "/unlockdown")
			yes := map[string]string{"a": "ly", "u": fmt.Sprint(env.admin.Id), "e": settingsCardExpiry(time.Now())}
			data := encodeCallbackData(settingsCallbackNamespace, yes)
			if data == "" {
				t.Fatalf("hand-built Yes %v did not encode", yes)
			}
			const menuMsgID = 4545
			env.pressRaw(menuChat, env.admin, menuMsgID, data)

			commandAfter := env.freshRow(commandRow.ID)
			menuAfter := env.freshRow(menuRow.ID)
			if commandAfter.State != menuAfter.State || commandAfter.ManualChange != menuAfter.ManualChange {
				t.Errorf("command left state %q (manual change %v), menu left %q (%v)",
					commandAfter.State, commandAfter.ManualChange, menuAfter.State, menuAfter.ManualChange)
			}
			if commandAfter.LiftedBy == nil || menuAfter.LiftedBy == nil ||
				*commandAfter.LiftedBy != env.admin.Id || *menuAfter.LiftedBy != env.admin.Id ||
				commandAfter.LiftedByName != menuAfter.LiftedByName {
				t.Errorf("lifters differ: command %v %q, menu %v %q",
					commandAfter.LiftedBy, commandAfter.LiftedByName, menuAfter.LiftedBy, menuAfter.LiftedByName)
			}
			for name, chat := range map[string]gotgbot.Chat{"command": commandChat, "menu": menuChat} {
				if got := env.fake.chatPermsRaw(chat.Id); got != lockdownTestPrePermissions {
					t.Errorf("%s chat permissions = %q, want the stored set", name, got)
				}
				if got := len(env.callsIn(chat.Id, "setChatPermissions")); got != 2 {
					t.Errorf("%s chat saw %d permission writes, want the lock and one restore", name, got)
				}
			}

			env.sendIn(commandChat, env.admin, "/lockdownstatus")
			env.sendIn(menuChat, env.admin, "/lockdownstatus")
			statusOf := func(chat gotgbot.Chat) string {
				sent := env.fake.sentTo(chat.Id)
				if len(sent) == 0 {
					t.Fatalf("the bot sent nothing to chat %d", chat.Id)
				}
				return fmt.Sprint(sent[len(sent)-1].Params["text"])
			}
			commandStatus, menuStatus := statusOf(commandChat), statusOf(menuChat)
			if commandStatus != menuStatus {
				t.Errorf("/lockdownstatus after the command lift says %q, after the menu lift %q", commandStatus, menuStatus)
			}
			want := "lockdown_not_active"
			if withJoiner {
				want = "lockdown_status_lifting"
			}
			if !strings.Contains(menuStatus, staffMarker(want)) {
				t.Errorf("/lockdownstatus says %q, want %s", menuStatus, staffMarker(want))
			}
		})
	}
}

func TestSettingsLockdownLiftCallbackBytes(t *testing.T) {
	withRealLocales(t)
	tr := i18n.MustNewTranslator("en")
	active := settingsLockdownSnapshot{
		Row: &models.ChatLockdown{
			ChatID: -1001234567890, ChatTitle: "G", State: models.LockdownStateActive, TriggerKind: models.LockdownTriggerManual,
		},
		Triggers:    &models.ChatRaidDetection{JoinEnabled: true, JoinThreshold: 500, FloodEnabled: true, FloodThreshold: 500, AIEnabled: true, AIThreshold: 500},
		AIAvailable: true,
	}
	for _, variant := range []struct {
		name string
		view settingsView
	}{
		{"private", settingsView{Tr: tr, Private: true, OwnerKey: "c", OwnerValue: "-1001234567890", GroupID: -1001234567890, GroupTitle: "G", OpenerName: "N"}},
		{"group", settingsView{Tr: tr, OwnerKey: "u", OwnerValue: "9999999999", GroupID: -1001234567890, GroupTitle: "G", OpenerName: "N"}},
	} {
		t.Run(variant.name, func(t *testing.T) {
			page := renderSettingsLockdown(variant.view, active, "")
			card := renderSettingsLockdownLiftCard(variant.view, active.Row)
			for name, screen := range map[string]settingsScreen{"page": page, "card": card} {
				if screen.Omitted != 0 {
					t.Errorf("%s omitted %d buttons, want none", name, screen.Omitted)
				}
				for _, button := range settingsAllButtons(screen.Keyboard.InlineKeyboard) {
					if size := len(button.CallbackData); size < 1 || size > 64 {
						t.Errorf("%s button %q has %d bytes of callback data, want 1..64", name, button.Text, size)
					}
				}
			}
			if got := len(settingsAllButtons(page.Keyboard.InlineKeyboard)); got > 21 {
				t.Errorf("the active page has %d buttons, want at most 21", got)
			}
		})
	}

	// The private Yes is the longest button of the card: a=ly, the 14-character chat ID and
	// an 8-digit expiry minute.
	private := settingsView{Tr: tr, Private: true, OwnerKey: "c", OwnerValue: "-1001234567890", GroupID: -1001234567890, GroupTitle: "G"}
	card := renderSettingsLockdownLiftCard(private, active.Row)
	var yesData string
	for _, button := range settingsAllButtons(card.Keyboard.InlineKeyboard) {
		if settingsButtonFields(t, button)["a"] == "ly" {
			yesData = button.CallbackData
		}
	}
	if len(yesData) != 44 {
		t.Errorf("the private Yes is %d bytes (%q), want 44", len(yesData), yesData)
	}
}

func TestSettingsLockdownLiftCardFitsEveryLocale(t *testing.T) {
	withRealLocales(t)
	title := string([]rune(strings.Repeat("&<ü✓", 32)))
	row := &models.ChatLockdown{ChatID: -1001234567890, ChatTitle: title, State: models.LockdownStateActive}
	for _, code := range []string{"en", "es", "fr", "hi", "id", "pt", "ru"} {
		t.Run(code, func(t *testing.T) {
			view := settingsView{
				Tr: i18n.MustNewTranslator(code), OwnerKey: "u", OwnerValue: "9999999999",
				GroupID: -1001234567890, GroupTitle: title, OpenerName: title,
			}
			card := renderSettingsLockdownLiftCard(view, row)
			if units := panelUTF16Len(card.Text); units > 3800 {
				t.Errorf("card is %d UTF-16 units, want at most 3800", units)
			}
			if strings.Contains(card.Text, "&<ü") {
				t.Errorf("card %q carries the unescaped title", card.Text)
			}
			if !strings.Contains(card.Text, "&amp;&lt;ü✓") {
				t.Errorf("card %q does not name the group, escaped", card.Text)
			}
			for _, button := range settingsAllButtons(card.Keyboard.InlineKeyboard) {
				if strings.TrimSpace(button.Text) == "" {
					t.Error("a card button has an empty label")
				}
			}
		})
	}
}

// settingsLiftTestCallIndex returns the position in the bot's call log of the first call of
// method to chatID whose text carries the marker, or -1.
func settingsLiftTestCallIndex(env *settingsEnv, method string, chatID int64, marker string) int {
	env.fake.moduleBotClient.mu.Lock()
	defer env.fake.moduleBotClient.mu.Unlock()
	for i, call := range env.fake.moduleBotClient.calls {
		if call.Method != method || staffParamInt(call.Params, "chat_id") != chatID {
			continue
		}
		if text, _ := call.Params["text"].(string); strings.Contains(text, marker) {
			return i
		}
	}
	return -1
}

// TestSettingsLockdownLiftRedrawsBeforeAnnouncing pins WR-04: the announcement waits for a
// pacer slot, so the menu is redrawn (and its live Yes is gone) before the public notice is
// sent.
func TestSettingsLockdownLiftRedrawsBeforeAnnouncing(t *testing.T) {
	env := newSettingsEnv(t)
	env.lockAsAdmin("raid")
	env.sendPrivate(env.admin, fmt.Sprintf("/start settings_%d", env.chat.Id))
	privateMsgID := settingsPrivateOnlySent(t, env, env.admin).MessageID
	private := settingsPrivateChat(env.admin)
	env.pressPrivate(env.admin, privateMsgID, env.privateFields(env.chat.Id, "a", "op", "p", settingsPageLockdown))
	_, keyboard := env.lastEdit(env.admin.Id, privateMsgID)
	env.pressRaw(private, env.admin, privateMsgID, settingsLiftTestButton(t, keyboard, "lockdown_alert_btn_lift").CallbackData)
	_, card := env.lastEdit(env.admin.Id, privateMsgID)
	yes := settingsLiftTestButton(t, card, "settings_btn_yes")

	env.pressRaw(private, env.admin, privateMsgID, yes.CallbackData)

	redraw := settingsLiftTestCallIndex(env, "editMessageText", env.admin.Id, staffMarker("lockdown_lifted"))
	announce := settingsLiftTestCallIndex(env, "sendMessage", env.chat.Id, staffMarker("lockdown_lifted"))
	if redraw < 0 || announce < 0 {
		t.Fatalf("redraw call at %d, announcement at %d: both must exist", redraw, announce)
	}
	if redraw > announce {
		t.Errorf("the menu was redrawn at call %d, after the announcement at call %d: the Yes card stayed on screen while the notice waited", redraw, announce)
	}
}
