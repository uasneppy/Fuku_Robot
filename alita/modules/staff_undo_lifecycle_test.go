//go:build testtools

package modules

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/db/staff"
	"github.com/divkix/Alita_Robot/alita/utils/cache"
)

// undoMember returns a Staff Group member named name, an administrator with the
// restrict right in every linked group, whose ID is the issuer's plus offset. The
// ID differs from undoPresser's (offset 1) for every offset but 1.
func (e *staffActionEnv) undoMember(name string, offset int64) gotgbot.User {
	e.t.Helper()
	user := gotgbot.User{Id: e.issuer.Id + offset, FirstName: name}
	for _, group := range e.groups {
		e.fake.setMember(group, user.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusAdministrator, CanRestrictMembers: true})
	}
	return user
}

// undoFinishedBan runs "/ban 4242" to its end and returns the record and the
// message its summary ended on.
func (e *staffActionEnv) undoFinishedBan() (*models.StaffAction, int64) {
	e.t.Helper()
	_, msgID := e.startRun("/ban 4242")
	e.waitRuns()
	action, _ := recordOfCard(e.t, msgID)
	return action, msgID
}

// unbansIn counts the unbanChatMember calls made to every linked group.
func (e *staffActionEnv) unbansIn() int {
	total := 0
	for _, group := range e.groups {
		total += len(e.callsTo("unbanChatMember", group))
	}
	return total
}

// undoStartedAt reads the record's undo_started_at fresh.
func undoStartedAt(t *testing.T, id uint) *time.Time {
	t.Helper()
	action, err := staff.GetActionFresh(id)
	if err != nil || action == nil {
		t.Fatalf("read staff action %d: %v (record %v)", id, err, action)
	}
	return action.UndoStartedAt
}

func TestStaffUndoCardCancel(t *testing.T) {
	env := newStaffActionEnv(t, 2)
	bob := env.undoPresser("Bob")
	action, msgID := env.undoFinishedBan()

	token, cardMsgID := env.askUndo(bob, msgID, action.ID)
	env.tapUndoCard(bob, undoCancelCode, token, cardMsgID)

	text := env.lastEditText(env.staffChat, cardMsgID)
	for _, key := range []string{"staff_undo_label", "staff_act_card_cancelled"} {
		if !strings.Contains(text, staffMarker(key)) {
			t.Fatalf("cancelled undo card lacks %s:\n%s", key, text)
		}
	}
	if !strings.Contains(text, "Bob") {
		t.Fatalf("cancelled undo card does not name who cancelled it:\n%s", text)
	}
	edits := env.edits(env.staffChat, cardMsgID)
	wantNoKeyboard(t, edits[len(edits)-1])
	if state := cardState(t, token); state != staffCardCancelled {
		t.Fatalf("undo card state = %q, want %q", state, staffCardCancelled)
	}
	if got := env.unbansIn(); got != 0 {
		t.Fatalf("a cancelled undo made %d unban call(s), want none", got)
	}
	if started := undoStartedAt(t, action.ID); started != nil {
		t.Fatalf("a cancelled undo claimed the record at %v", started)
	}

	// Nothing was used up: Undo posts a new card, which runs.
	sentBefore := len(env.fake.sentTo(env.staffChat))
	secondToken, secondMsg := env.askUndo(bob, msgID, action.ID)
	if got := len(env.fake.sentTo(env.staffChat)); got != sentBefore+1 || secondToken == token {
		t.Fatalf("Undo after a Cancel posted %d card(s) (token %q), want one new card", got-sentBefore, secondToken)
	}
	env.tapUndoCard(bob, undoConfirmCode, secondToken, secondMsg)
	env.waitRuns()
	if got := env.unbansIn(); got != len(env.groups) {
		t.Fatalf("unban calls after the second card = %d, want one per group", got)
	}
}

func TestStaffUndoCardPresserOnly(t *testing.T) {
	env := newStaffActionEnv(t, 2)
	bob := env.undoPresser("Bob")
	carol := env.undoMember("Carol", 2)
	action, msgID := env.undoFinishedBan()
	token, cardMsgID := env.askUndo(bob, msgID, action.ID)
	editsBefore := len(env.edits(env.staffChat, cardMsgID))

	for _, code := range []string{undoConfirmCode, undoCancelCode} {
		env.tapUndoCard(carol, code, token, cardMsgID)
		if text, alert := env.lastAnswer(); text != staffMarker("staff_undo_card_presser_only") || !alert {
			t.Fatalf("answer to Carol's %s = %q alert=%v, want the presser-only alert", code, text, alert)
		}
	}
	if state := cardState(t, token); state != staffCardPending {
		t.Fatalf("undo card state = %q, want it still pending", state)
	}
	if got := len(env.edits(env.staffChat, cardMsgID)); got != editsBefore {
		t.Fatalf("another member's taps edited the card %d time(s)", got-editsBefore)
	}
	if got := env.unbansIn(); got != 0 {
		t.Fatalf("another member's taps made %d unban call(s)", got)
	}

	env.tapUndoCard(bob, undoConfirmCode, token, cardMsgID)
	env.waitRuns()
	if got := env.unbansIn(); got != len(env.groups) {
		t.Fatalf("unban calls after Bob's Confirm = %d, want one per group", got)
	}
}

func TestStaffUndoCardExpires(t *testing.T) {
	env := newStaffActionEnv(t, 2)
	bob := env.undoPresser("Bob")
	action, msgID := env.undoFinishedBan()

	// The ban has run on the normal timers; only the undo card is short-lived.
	shortenCardLifetime(t, 50*time.Millisecond)
	shortenExpirySlack(t, 10*time.Millisecond)
	token, cardMsgID := env.askUndo(bob, msgID, action.ID)

	env.waitForEdits(cardMsgID, 1)
	text := env.lastEditText(env.staffChat, cardMsgID)
	for _, key := range []string{"staff_undo_label", "staff_act_card_expired_text"} {
		if !strings.Contains(text, staffMarker(key)) {
			t.Fatalf("expired undo card lacks %s:\n%s", key, text)
		}
	}
	wantNoKeyboard(t, env.edits(env.staffChat, cardMsgID)[0])
	if state := cardState(t, token); state != staffCardExpired {
		t.Fatalf("undo card state = %q, want %q", state, staffCardExpired)
	}

	env.tapUndoCard(bob, undoConfirmCode, token, cardMsgID)
	env.waitRuns()
	if got := env.answersContaining("staff_act_card_expired"); got != 1 {
		answer, _ := env.lastAnswer()
		t.Fatalf("expired toasts = %d (last answer %q), want 1", got, answer)
	}
	if started := undoStartedAt(t, action.ID); started != nil {
		t.Fatalf("a late Confirm claimed the record at %v", started)
	}
	if got := env.unbansIn(); got != 0 {
		t.Fatalf("a late Confirm made %d unban call(s)", got)
	}
}

func TestStaffUndoCardDoubleTap(t *testing.T) {
	env := newStaffActionEnv(t, 2)
	bob := env.undoPresser("Bob")
	action, msgID := env.undoFinishedBan()
	token, cardMsgID := env.askUndo(bob, msgID, action.ID)

	env.tapUndoCard(bob, undoConfirmCode, token, cardMsgID)
	env.tapUndoCard(bob, undoConfirmCode, token, cardMsgID)
	env.waitRuns()

	if text, _ := env.lastAnswer(); text != staffMarker("staff_act_card_handled") {
		t.Fatalf("answer to the second Confirm = %q, want the already-handled toast", text)
	}
	for _, group := range env.groups {
		if got := len(env.callsTo("unbanChatMember", group)); got != 1 {
			t.Fatalf("group %d unban calls = %d, want exactly one", group, got)
		}
	}
}

func TestStaffUndoOutsider(t *testing.T) {
	// pressUndo presses the Undo button of record id as from and returns the answer
	// and how many messages the press posted to the Staff Group.
	pressUndo := func(env *staffActionEnv, from gotgbot.User, msgID int64, id uint) (string, bool, int) {
		before := len(env.fake.sentTo(env.staffChat))
		env.tapData(from, env.staffChatObj(), msgID,
			encodeCallbackData(staffCallbackNamespace, map[string]string{"a": undoAskCode, "r": fmt.Sprint(id)}))
		text, alert := env.lastAnswer()
		return text, alert, len(env.fake.sentTo(env.staffChat)) - before
	}

	t.Run("non-member", func(t *testing.T) {
		env := newStaffActionEnv(t, 2)
		action, msgID := env.undoFinishedBan()
		eve := gotgbot.User{Id: env.issuer.Id + 50, FirstName: "Eve"}
		env.fake.setMember(env.staffChat, eve.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusLeft})

		text, alert, posted := pressUndo(env, eve, msgID, action.ID)

		if text != staffMarker("staff_cb_members_only") || !alert || posted != 0 {
			t.Fatalf("non-member press: answer %q alert=%v posted=%d, want the members-only alert and no card", text, alert, posted)
		}
	})

	t.Run("service identity", func(t *testing.T) {
		env := newStaffActionEnv(t, 2)
		bob := env.undoPresser("Bob")
		action, msgID := env.undoFinishedBan()
		for id := range staffServiceUserIDs {
			service := gotgbot.User{Id: id, FirstName: "Telegram"}
			// Even as a live Staff Group member, a service identity gets no card.
			env.fake.setMember(env.staffChat, id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})

			text, alert, posted := pressUndo(env, service, msgID, action.ID)

			if text != staffMarker("staff_post_as_yourself") || !alert || posted != 0 {
				t.Fatalf("service identity %d: answer %q alert=%v posted=%d, want post-as-yourself and no card", id, text, alert, posted)
			}
		}

		// A card made by a real member cannot be confirmed by one either.
		token, cardMsgID := env.askUndo(bob, msgID, action.ID)
		env.tapUndoCard(gotgbot.User{Id: 1087968824, FirstName: "GroupAnonymousBot"}, undoConfirmCode, token, cardMsgID)
		if text, alert := env.lastAnswer(); text != staffMarker("staff_post_as_yourself") || !alert {
			t.Fatalf("service identity at Confirm: answer %q alert=%v, want post-as-yourself", text, alert)
		}
		if state := cardState(t, token); state != staffCardPending {
			t.Fatalf("undo card state = %q, want it untouched (pending)", state)
		}
		if started := undoStartedAt(t, action.ID); started != nil {
			t.Fatalf("a service identity claimed the record at %v", started)
		}
	})

	t.Run("other Staff Group's record", func(t *testing.T) {
		env := newStaffActionEnv(t, 2)
		bob := env.undoPresser("Bob")
		otherStaff := uniqueModuleChatID()
		staffCleanup(t, otherStaff)
		now := time.Now()
		foreign := &models.StaffAction{
			StaffChatID: otherStaff, IssuerUserID: 1, TargetUserID: staffTestTarget, Action: "ban",
			GroupCount: 1, FinishedAt: &now,
		}
		if err := staff.CreateAction(foreign, []models.StaffActionGroup{
			{Seq: 0, GroupChatID: env.groups[0], Outcome: models.StaffActionOutcomeDone},
		}); err != nil {
			t.Fatalf("create foreign record: %v", err)
		}

		text, alert, posted := pressUndo(env, bob, 5, foreign.ID)

		if text != staffMarker("staff_cb_denied") || !alert || posted != 0 {
			t.Fatalf("foreign record: answer %q alert=%v posted=%d, want the denied alert and no card", text, alert, posted)
		}
	})

	t.Run("missing record", func(t *testing.T) {
		env := newStaffActionEnv(t, 2)
		bob := env.undoPresser("Bob")

		text, _, posted := pressUndo(env, bob, 5, 987654321)

		if text != staffMarker("staff_cb_expired") || posted != 0 {
			t.Fatalf("missing record: answer %q posted=%d, want the expired answer and no card", text, posted)
		}
	})

	t.Run("kick", func(t *testing.T) {
		env := newStaffActionEnv(t, 2)
		bob := env.undoPresser("Bob")
		_, msgID := env.startRun("/kick 4242")
		env.waitRuns()
		action, _ := recordOfCard(t, msgID)

		text, alert, posted := pressUndo(env, bob, msgID, action.ID)

		if text != staffMarker("staff_undo_not_available") || !alert || posted != 0 {
			t.Fatalf("kick record: answer %q alert=%v posted=%d, want the not-available alert and no card", text, alert, posted)
		}
	})

	t.Run("already undone", func(t *testing.T) {
		env := newStaffActionEnv(t, 2)
		bob := env.undoPresser("Bob")
		action, msgID := env.undoFinishedBan()
		env.runUndo(bob, msgID, action.ID)

		text, alert, posted := pressUndo(env, bob, msgID, action.ID)

		if !strings.Contains(text, staffMarker("staff_undo_already")) || !strings.Contains(text, "Bob") || !alert || posted != 0 {
			t.Fatalf("undone record: answer %q alert=%v posted=%d, want the already-undone alert naming Bob and no card", text, alert, posted)
		}
	})

	t.Run("Redis down", func(t *testing.T) {
		env := newStaffActionEnv(t, 2)
		bob := env.undoPresser("Bob")
		action, msgID := env.undoFinishedBan()
		cache.SetRedisClientForTest(t, nil)

		text, alert, posted := pressUndo(env, bob, msgID, action.ID)

		if text != staffMarker("staff_act_redis_unavailable") || !alert || posted != 0 {
			t.Fatalf("Redis down: answer %q alert=%v posted=%d, want the shared-cache alert and no card", text, alert, posted)
		}
		if started := undoStartedAt(t, action.ID); started != nil {
			t.Fatalf("an undo without Redis claimed the record at %v", started)
		}
	})

	t.Run("presser left before Confirm", func(t *testing.T) {
		env := newStaffActionEnv(t, 2)
		bob := env.undoPresser("Bob")
		action, msgID := env.undoFinishedBan()
		token, cardMsgID := env.askUndo(bob, msgID, action.ID)
		env.fake.setMember(env.staffChat, bob.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusLeft})

		env.tapUndoCard(bob, undoConfirmCode, token, cardMsgID)
		env.waitRuns()

		if text := env.lastEditText(env.staffChat, cardMsgID); !strings.Contains(text, staffMarker("staff_act_abort_issuer_left")) {
			t.Fatalf("undo card after the presser left:\n%s", text)
		}
		if state := cardState(t, token); state != staffCardAborted {
			t.Fatalf("undo card state = %q, want %q", state, staffCardAborted)
		}
		if started := undoStartedAt(t, action.ID); started != nil {
			t.Fatalf("a presser who left claimed the record at %v", started)
		}
		if got := env.unbansIn(); got != 0 {
			t.Fatalf("a presser who left made %d unban call(s)", got)
		}
	})
}
