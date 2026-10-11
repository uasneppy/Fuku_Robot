//go:build testtools

package modules

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db/lockdown"
	"github.com/divkix/Alita_Robot/alita/db/models"
)

// pressLockdownButton presses a button of the lockdown| namespace on message msgID of
// chat, as from, through the real dispatcher. The message is a post of the bot, so the
// callback carries the real presser in from.
func pressLockdownButton(env *lockdownEnv, from gotgbot.User, chat gotgbot.Chat, msgID int64, data string) {
	env.t.Helper()
	id := env.updateID()
	update := &gotgbot.Update{
		UpdateId: id,
		CallbackQuery: &gotgbot.CallbackQuery{
			Id:           fmt.Sprintf("cb-%d", id),
			From:         from,
			Message:      gotgbot.Message{MessageId: msgID, Date: 1, Chat: chat},
			Data:         data,
			ChatInstance: "lockdown-alert-test",
		},
	}
	if err := env.dispatcher.ProcessUpdate(env.bot, update, nil); err != nil {
		env.t.Fatalf("ProcessUpdate(%d) error = %v", id, err)
	}
}

// lockdownButtonData is the callback data of the button of markup whose decoded field
// a equals act, or a fatal error when the keyboard has none.
func lockdownButtonData(t *testing.T, markup any, act string) string {
	t.Helper()
	for _, row := range staffKeyboardOf(markup) {
		for _, button := range row {
			decoded, ok := decodeCallbackData(button.CallbackData, "lockdown")
			if ok && decoded.Fields["a"] == act {
				return button.CallbackData
			}
		}
	}
	t.Fatalf("keyboard %+v has no lockdown button for action %q", staffKeyboardOf(markup), act)
	return ""
}

// staffCalls returns the recorded calls of method addressed to chatID.
func (e *lockdownEnv) staffCalls(method string, chatID int64) []moduleBotCall {
	return callsToChat(e.fake.staffBotClient, method, chatID)
}

// answerTotal counts the answerCallbackQuery calls so far.
func (e *lockdownEnv) answerTotal() int {
	return len(e.fake.callsFor("answerCallbackQuery"))
}

func TestLockdownAlertLift(t *testing.T) {
	env := newLockdownEnv(t)
	staffChat := linkLockdownStaffGroup(env)
	member := env.newOutsider()
	other := env.newOutsider()
	env.fake.setMember(env.chat.Id, other.Id, staffFakeMember{
		Status:             gotgbot.ChatMemberStatusAdministrator,
		CanRestrictMembers: true,
	})

	row := env.lockAsAdmin("raid")
	env.cycle()

	alerts := env.fake.sentTo(staffChat.Id)
	if len(alerts) != 1 {
		t.Fatalf("%d messages reached the Staff Group, want exactly the alert", len(alerts))
	}
	alert := alerts[0]
	keyboard := staffKeyboardOf(alert.Params["reply_markup"])
	if len(keyboard) != 2 || len(keyboard[0]) != 1 || len(keyboard[1]) != 1 {
		t.Fatalf("alert keyboard = %+v, want the Lift button and, on a second row, the Ban button", keyboard)
	}
	if !strings.Contains(keyboard[0][0].Text, staffMarker("lockdown_alert_btn_lift")) {
		t.Errorf("button label = %q, want %q", keyboard[0][0].Text, staffMarker("lockdown_alert_btn_lift"))
	}
	liftData := lockdownButtonData(t, alert.Params["reply_markup"], "lk")
	decoded, _ := decodeCallbackData(liftData, "lockdown")
	copies := env.alertRows()
	if len(copies) != 1 || decoded.Fields["r"] != fmt.Sprint(copies[0].ID) {
		t.Fatalf("button names copy %q, want the alert's copy row %+v", decoded.Fields["r"], copies)
	}

	// Being in the Staff Group grants nothing: a plain member of the locked group is refused.
	answers := env.answerTotal()
	pressLockdownButton(env, member, staffChat, alert.MessageID, liftData)
	if got := env.answerTotal() - answers; got != 1 {
		t.Fatalf("a refused press made %d answers, want exactly 1", got)
	}
	if text, _ := env.lastAnswer(); !strings.Contains(text, staffMarker("lockdown_alert_not_admin")) {
		t.Errorf("refusal answer = %q, want %q", text, staffMarker("lockdown_alert_not_admin"))
	}
	if got := len(env.fake.sentTo(staffChat.Id)); got != 1 {
		t.Errorf("%d messages in the Staff Group after a refused press, want only the alert", got)
	}
	if active, err := lockdown.GetActiveFresh(env.chat.Id); err != nil || active == nil {
		t.Fatalf("GetActiveFresh = %v, %v, want the lockdown still active", active, err)
	}

	// A live admin of the locked group gets the confirm card, replying to the alert.
	answers = env.answerTotal()
	pressLockdownButton(env, env.admin, staffChat, alert.MessageID, liftData)
	if got := env.answerTotal() - answers; got != 1 {
		t.Fatalf("an accepted press made %d answers, want exactly 1", got)
	}
	posted := env.fake.sentTo(staffChat.Id)
	if len(posted) != 2 {
		t.Fatalf("%d messages in the Staff Group, want the alert and one confirm card", len(posted))
	}
	card := posted[1]
	if text := fmt.Sprint(card.Params["text"]); !strings.Contains(text, staffMarker("lockdown_alert_lift_ask")) {
		t.Errorf("card text = %q, want %q", text, staffMarker("lockdown_alert_lift_ask"))
	}
	reply, ok := card.Params["reply_parameters"].(*gotgbot.ReplyParameters)
	if !ok || reply.MessageId != alert.MessageID {
		t.Fatalf("card reply_parameters = %+v, want a reply to the alert %d", card.Params["reply_parameters"], alert.MessageID)
	}
	confirmData := lockdownButtonData(t, card.Params["reply_markup"], "lc")
	cancelData := lockdownButtonData(t, card.Params["reply_markup"], "x")
	confirm, _ := decodeCallbackData(confirmData, "lockdown")
	if confirm.Fields["u"] != fmt.Sprint(env.admin.Id) {
		t.Errorf("Confirm carries asker %q, want %d", confirm.Fields["u"], env.admin.Id)
	}
	if _, ok := decodeCallbackData(cancelData, "lockdown"); !ok {
		t.Errorf("Cancel data %q does not decode", cancelData)
	}

	// Another live admin cannot confirm the card.
	setsBefore := len(env.calls("setChatPermissions"))
	pressLockdownButton(env, other, staffChat, card.MessageID, confirmData)
	if text, _ := env.lastAnswer(); !strings.Contains(text, staffMarker("lockdown_alert_not_yours")) {
		t.Errorf("other admin's answer = %q, want %q", text, staffMarker("lockdown_alert_not_yours"))
	}
	if got := len(env.calls("setChatPermissions")); got != setsBefore {
		t.Fatalf("setChatPermissions calls = %d, were %d: only the asker may confirm", got, setsBefore)
	}

	// The asker confirms: the stored permissions go back before the lift is recorded.
	var mu sync.Mutex
	var stateAtRestore string
	var liftedByAtRestore *int64
	env.fake.setOnSetPermissions(func(int64) {
		current, err := lockdown.GetFresh(row.ID)
		mu.Lock()
		defer mu.Unlock()
		if err == nil && current != nil {
			stateAtRestore = current.State
			liftedByAtRestore = current.LiftedBy
		}
	})
	pressLockdownButton(env, env.admin, staffChat, card.MessageID, confirmData)

	sets := env.calls("setChatPermissions")
	if len(sets) != setsBefore+1 {
		t.Fatalf("setChatPermissions calls = %d, want %d: one restore", len(sets), setsBefore+1)
	}
	if got := lockdownParamText(sets[len(sets)-1].Params["permissions"]); got != lockdownTestPrePermissions {
		t.Errorf("restore sent %q, want the stored permissions", got)
	}
	mu.Lock()
	if stateAtRestore != models.LockdownStateActive || liftedByAtRestore != nil {
		t.Errorf("at the restore call the row was %q with lifted_by %v, want active with none", stateAtRestore, liftedByAtRestore)
	}
	mu.Unlock()

	lifted := env.freshRow(row.ID)
	if lifted.State != models.LockdownStateLifting && lifted.State != models.LockdownStateLifted {
		t.Errorf("State = %q, want lifting or lifted", lifted.State)
	}
	if lifted.LiftedBy == nil || *lifted.LiftedBy != env.admin.Id || lifted.LiftedByName != "Ad<b>min" {
		t.Errorf("lifted by %v %q, want %d %q", lifted.LiftedBy, lifted.LiftedByName, env.admin.Id, "Ad<b>min")
	}
	if got := env.repliesWith(staffMarker("lockdown_lifted")); len(got) != 1 {
		t.Errorf("%d lift notices reached the locked group, want 1", len(got))
	}
	edits := env.staffCalls("editMessageText", staffChat.Id)
	if len(edits) == 0 || !strings.Contains(fmt.Sprint(edits[len(edits)-1].Params["text"]), staffMarker("lockdown_alert_lift_done")) {
		t.Errorf("card edits = %+v, want the card edited to %q", edits, staffMarker("lockdown_alert_lift_done"))
	}
}

// liftCard is a confirm card the env's admin asked for: the lockdown, the card's message
// and the data of its two buttons.
type liftCard struct {
	row       *models.ChatLockdown
	staffChat gotgbot.Chat
	msgID     int64
	confirm   string
	cancel    string
}

// askToLiftLockdown locks the env's chat, lets the worker post the alert in a new Staff
// Group, and has the env's admin press Lift. It returns the confirm card that resulted.
func askToLiftLockdown(t *testing.T, env *lockdownEnv) liftCard {
	t.Helper()
	staffChat := linkLockdownStaffGroup(env)
	row := env.lockAsAdmin("raid")
	env.cycle()

	alerts := env.fake.sentTo(staffChat.Id)
	if len(alerts) != 1 {
		t.Fatalf("%d messages reached the Staff Group, want exactly the alert", len(alerts))
	}
	liftData := lockdownButtonData(t, alerts[0].Params["reply_markup"], "lk")
	pressLockdownButton(env, env.admin, staffChat, alerts[0].MessageID, liftData)

	posted := env.fake.sentTo(staffChat.Id)
	if len(posted) != 2 {
		t.Fatalf("%d messages in the Staff Group, want the alert and one confirm card", len(posted))
	}
	card := posted[1]
	return liftCard{
		row:       row,
		staffChat: staffChat,
		msgID:     card.MessageID,
		confirm:   lockdownButtonData(t, card.Params["reply_markup"], "lc"),
		cancel:    lockdownButtonData(t, card.Params["reply_markup"], "x"),
	}
}

// cardEdits returns the text of every edit of message msgID of chatID, in order.
func (e *lockdownEnv) cardEdits(chatID, msgID int64) []string {
	var texts []string
	for _, call := range e.staffCalls("editMessageText", chatID) {
		if staffParamInt(call.Params, "message_id") == msgID {
			texts = append(texts, fmt.Sprint(call.Params["text"]))
		}
	}
	return texts
}

// wantCardEdited fails unless the last edit of the card contains want.
func (e *lockdownEnv) wantCardEdited(card liftCard, want string) {
	e.t.Helper()
	edits := e.cardEdits(card.staffChat.Id, card.msgID)
	if len(edits) == 0 {
		e.t.Fatalf("the confirm card was never edited, want a text with %q", want)
	}
	if last := edits[len(edits)-1]; !strings.Contains(last, want) {
		e.t.Errorf("confirm card reads %q, want %q", last, want)
	}
}

// wantStillActive fails unless the lockdown row is still active with no lifter.
func (e *lockdownEnv) wantStillActive(row *models.ChatLockdown) {
	e.t.Helper()
	current := e.freshRow(row.ID)
	if current.State != models.LockdownStateActive || current.LiftedBy != nil {
		e.t.Errorf("row = %+v, want it still active with no lifter", current)
	}
}

func TestLockdownAlertLiftOutcomes(t *testing.T) {
	t.Run("restore fails", func(t *testing.T) {
		env := newLockdownEnv(t)
		card := askToLiftLockdown(t, env)
		env.fake.script("setChatPermissions", env.chat.Id,
			staffFakeError(400, "Bad Request: not enough rights to change chat permissions"))

		pressLockdownButton(env, env.admin, card.staffChat, card.msgID, card.confirm)

		env.wantCardEdited(card, staffMarker("lockdown_alert_restore_failed"))
		env.wantStillActive(card.row)
		if got := env.repliesWith(staffMarker("lockdown_lifted")); len(got) != 0 {
			t.Errorf("%d lift notices reached the locked group after a failed restore, want none", len(got))
		}
		if got := env.fake.chatPermsRaw(env.chat.Id); got != lockdownLockedPermissions {
			t.Errorf("permissions = %q, want the group to stay locked", got)
		}
	})

	t.Run("record fails", func(t *testing.T) {
		env := newLockdownEnv(t)
		card := askToLiftLockdown(t, env)
		previous := lockdownBeginLift
		t.Cleanup(func() { lockdownBeginLift = previous })
		lockdownBeginLift = func(uint, int64, string, bool) (bool, error) {
			return false, errors.New("database is down")
		}

		pressLockdownButton(env, env.admin, card.staffChat, card.msgID, card.confirm)

		env.wantCardEdited(card, staffMarker("lockdown_alert_record_failed"))
		env.wantStillActive(card.row)
		if got := env.repliesWith(staffMarker("lockdown_lifted")); len(got) != 0 {
			t.Errorf("%d lift notices reached the locked group though nothing was recorded, want none", len(got))
		}
	})

	t.Run("someone lifted first", func(t *testing.T) {
		env := newLockdownEnv(t)
		card := askToLiftLockdown(t, env)
		env.send(env.admin, "/unlockdown")

		pressLockdownButton(env, env.admin, card.staffChat, card.msgID, card.confirm)

		env.wantCardEdited(card, staffMarker("lockdown_alert_already_lifted"))
		restores := 0
		for _, call := range env.calls("setChatPermissions") {
			if lockdownParamText(call.Params["permissions"]) == lockdownTestPrePermissions {
				restores++
			}
		}
		if restores != 1 {
			t.Errorf("%d restores of the stored permissions in total, want exactly 1", restores)
		}
		if got := env.repliesWith(staffMarker("lockdown_lifted")); len(got) != 1 {
			t.Errorf("%d lift notices in the locked group, want only /unlockdown's", len(got))
		}
	})

	t.Run("lost the race", func(t *testing.T) {
		env := newLockdownEnv(t)
		card := askToLiftLockdown(t, env)
		// Another admin records the lift between this lift's restore and its record.
		var once sync.Once
		env.fake.setOnSetPermissions(func(int64) {
			once.Do(func() {
				if won, err := lockdown.BeginLift(card.row.ID, 4242, "Bea", false); err != nil || !won {
					t.Errorf("setup: BeginLift = %v, %v, want true", won, err)
				}
			})
		})

		pressLockdownButton(env, env.admin, card.staffChat, card.msgID, card.confirm)

		env.wantCardEdited(card, staffMarker("lockdown_alert_already_lifted"))
		lifted := env.freshRow(card.row.ID)
		if lifted.LiftedBy == nil || *lifted.LiftedBy != 4242 {
			t.Errorf("LiftedBy = %v, want 4242, the admin who recorded first", lifted.LiftedBy)
		}
		if got := env.repliesWith(staffMarker("lockdown_lifted")); len(got) != 0 {
			t.Errorf("%d lift notices from the losing lift, want none", len(got))
		}
	})

	t.Run("cancel", func(t *testing.T) {
		env := newLockdownEnv(t)
		card := askToLiftLockdown(t, env)
		other := env.newOutsider()
		env.fake.setMember(env.chat.Id, other.Id, staffFakeMember{
			Status:             gotgbot.ChatMemberStatusAdministrator,
			CanRestrictMembers: true,
		})
		setsBefore := len(env.calls("setChatPermissions"))

		pressLockdownButton(env, other, card.staffChat, card.msgID, card.cancel)

		if text, _ := env.lastAnswer(); !strings.Contains(text, staffMarker("lockdown_alert_not_yours")) {
			t.Errorf("other admin's answer = %q, want %q", text, staffMarker("lockdown_alert_not_yours"))
		}
		if edits := env.cardEdits(card.staffChat.Id, card.msgID); len(edits) != 0 {
			t.Fatalf("the card was edited %d times by someone who did not ask, want none", len(edits))
		}

		pressLockdownButton(env, env.admin, card.staffChat, card.msgID, card.cancel)

		env.wantCardEdited(card, staffMarker("lockdown_alert_cancelled"))
		env.wantStillActive(card.row)
		if got := len(env.calls("setChatPermissions")); got != setsBefore {
			t.Errorf("setChatPermissions calls = %d, were %d: Cancel changes nothing", got, setsBefore)
		}
	})
}
