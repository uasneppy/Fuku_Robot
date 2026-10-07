//go:build testtools

package modules

import (
	"errors"
	"testing"

	"github.com/divkix/Alita_Robot/alita/db/greetings"
	"github.com/divkix/Alita_Robot/alita/db/models"
)

// requestCallsOf returns the recorded calls of method (approveChatJoinRequest or
// declineChatJoinRequest) for userID in the env's chat.
func (e *lockdownEnv) requestCallsOf(method string, userID int64) []moduleBotCall {
	var matched []moduleBotCall
	for _, call := range e.calls(method) {
		if staffParamInt(call.Params, "user_id") == userID {
			matched = append(matched, call)
		}
	}
	return matched
}

func (e *lockdownEnv) approvesOf(userID int64) []moduleBotCall {
	return e.requestCallsOf("approveChatJoinRequest", userID)
}

func (e *lockdownEnv) declinesOf(userID int64) []moduleBotCall {
	return e.requestCallsOf("declineChatJoinRequest", userID)
}

func TestLockdownGuardDeclinesJoinRequest(t *testing.T) {
	env := newLockdownEnv(t)
	env.loadJoinModules()
	if err := greetings.SetShouldAutoApprove(env.chat.Id, true); err != nil {
		t.Fatalf("enable auto-approve: %v", err)
	}

	// Control: without a lockdown the request is approved, so everything below is the
	// guard's doing.
	control := env.newJoiner("Control")
	env.joinRequest(control, "")
	if got := len(env.approvesOf(control.Id)); got != 1 {
		t.Fatalf("control: approveChatJoinRequest calls = %d, want 1: auto-approve should admit the request before a lockdown", got)
	}

	env.lockAsAdmin("raid")
	messages := len(env.replies())

	raider := env.newJoiner("Raider")
	env.joinRequest(raider, "https://t.me/+abc")

	if got := len(env.approvesOf(raider.Id)); got != 0 {
		t.Errorf("approveChatJoinRequest calls for the raider = %d, want none during a lockdown", got)
	}
	if got := len(env.replies()); got != messages {
		t.Errorf("the bot sent %d message(s) for the request, want none: no approve card", got-messages)
	}
	row := env.joinerRow(raider.Id)
	if row.State != models.JoinerStatePending || row.JoinPath != models.JoinPathRequest || row.InviteLink != "https://t.me/+abc" {
		t.Errorf("row = %+v, want pending, request path, the invite link", row)
	}
	if got := len(env.declinesOf(raider.Id)); got != 0 {
		t.Errorf("the guard made %d decline call(s), want none: the worker declines, never the handler", got)
	}

	env.cycle()
	if got := len(env.declinesOf(raider.Id)); got != 1 {
		t.Errorf("declineChatJoinRequest calls after one cycle = %d, want exactly 1", got)
	}
	if row := env.joinerRow(raider.Id); row.State != models.JoinerStateDeclined {
		t.Errorf("row after the cycle = %+v, want declined", row)
	}

	// The declined person asks again at once: the row is reclaimed and declined again.
	env.joinRequest(raider, "")
	if row := env.joinerRow(raider.Id); row.State != models.JoinerStatePending {
		t.Fatalf("row after the second request = %+v, want the declined row reclaimed to pending", row)
	}
	env.cycle()
	if got := len(env.declinesOf(raider.Id)); got != 2 {
		t.Errorf("declineChatJoinRequest calls after the second request = %d, want 2", got)
	}
	if got := len(env.approvesOf(raider.Id)); got != 0 {
		t.Errorf("approveChatJoinRequest calls for the raider = %d, want none", got)
	}

	t.Run("a request Telegram reports as gone is declined", func(t *testing.T) {
		gone := env.newJoiner("Gone")
		env.fake.script("declineChatJoinRequest", env.chat.Id,
			staffFakeError(400, "Bad Request: HIDE_REQUESTER_MISSING"))
		env.joinRequest(gone, "")
		env.cycle()

		if got := len(env.declinesOf(gone.Id)); got != 1 {
			t.Errorf("declineChatJoinRequest calls = %d, want 1: a gone request is not retried", got)
		}
		if row := env.joinerRow(gone.Id); row.State != models.JoinerStateDeclined {
			t.Errorf("row = %+v, want declined: a request that is already gone counts as declined", row)
		}
	})

	t.Run("a request still pending at the lift is cancelled", func(t *testing.T) {
		late := env.newJoiner("Late")
		env.joinRequest(late, "")
		if row := env.joinerRow(late.Id); row.State != models.JoinerStatePending {
			t.Fatalf("setup: row = %+v, want pending", row)
		}
		env.send(env.admin, "/unlockdown")
		env.cycle()

		if got := len(env.declinesOf(late.Id)); got != 0 {
			t.Errorf("declineChatJoinRequest calls = %d, want none: the lift cancels a pending request", got)
		}
		if row := env.joinerRow(late.Id); row.State != models.JoinerStateCancelled {
			t.Errorf("row = %+v, want cancelled", row)
		}
	})
}

func TestLockdownJoinRequestFailsClosed(t *testing.T) {
	env := newLockdownEnv(t)
	env.loadJoinModules()
	if err := greetings.SetShouldAutoApprove(env.chat.Id, true); err != nil {
		t.Fatalf("enable auto-approve: %v", err)
	}
	env.lockAsAdmin("raid")

	previous := lockdownActiveLookup
	lockdownActiveLookup = func(int64) (*models.ChatLockdown, error) {
		return nil, errors.New("database unavailable")
	}
	t.Cleanup(func() { lockdownActiveLookup = previous })

	applicant := env.newJoiner("Applicant")
	env.joinRequest(applicant, "")

	if got := len(env.approvesOf(applicant.Id)); got != 0 {
		t.Errorf("approveChatJoinRequest calls = %d, want none: a request the guard could not judge is left pending", got)
	}
	if got := len(env.declinesOf(applicant.Id)); got != 0 {
		t.Errorf("declineChatJoinRequest calls = %d, want none: the request is left pending, not declined", got)
	}
}

func TestLockdownAcceptButtonRefuses(t *testing.T) {
	env := newLockdownEnv(t)
	env.loadJoinModules()
	owner := env.newJoiner("Owner")
	env.fake.setCreator(env.chat.Id, owner.Id)

	// Control: before the lockdown a request posts the approve card.
	applicant := env.newJoiner("Applicant")
	env.joinRequest(applicant, "")
	sent := env.replies()
	if len(sent) == 0 {
		t.Fatal("control: the request posted no approve card")
	}
	cardID := sent[len(sent)-1].MessageID

	env.lockAsAdmin("raid")
	env.pressJoinRequest(owner, cardID, "accept", applicant.Id)

	text, alert := env.lastAnswer()
	if !alert || text != staffMarker("greetings_join_request_lockdown") {
		t.Errorf("answer = %q (alert %v), want the lockdown alert", text, alert)
	}
	if got := len(env.approvesOf(applicant.Id)); got != 0 {
		t.Errorf("approveChatJoinRequest calls = %d, want none: Accept refuses during a lockdown", got)
	}
	if got := len(env.calls("editMessageText")); got != 0 {
		t.Errorf("editMessageText calls = %d, want none: the card stays as it is", got)
	}

	env.pressJoinRequest(owner, cardID, "decline", applicant.Id)
	if got := len(env.declinesOf(applicant.Id)); got != 1 {
		t.Errorf("declineChatJoinRequest calls after Decline = %d, want 1: Decline still works", got)
	}

	env.send(env.admin, "/unlockdown")
	later := env.newJoiner("Later")
	env.joinRequest(later, "")
	sent = env.replies()
	laterCard := sent[len(sent)-1].MessageID
	env.pressJoinRequest(owner, laterCard, "accept", later.Id)

	if got := len(env.approvesOf(later.Id)); got != 1 {
		t.Errorf("approveChatJoinRequest calls after the lift = %d, want 1: Accept works again", got)
	}
	if text, alert := env.lastAnswer(); alert || text == staffMarker("greetings_join_request_lockdown") {
		t.Errorf("answer after the lift = %q (alert %v), want the normal acceptance", text, alert)
	}
}
