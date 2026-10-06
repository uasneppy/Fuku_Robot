//go:build testtools

package modules

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db"
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

func TestStaffUndoOnce(t *testing.T) {
	env := newStaffActionEnv(t, 2)
	bob := env.undoPresser("Bob")
	carol := env.undoMember("Carol", 2)
	action, msgID := env.undoFinishedBan()

	// Two cards for one record: each press is allowed, only one confirm can win.
	bobToken, bobMsg := env.askUndo(bob, msgID, action.ID)
	carolToken, carolMsg := env.askUndo(carol, msgID, action.ID)
	if bobToken == carolToken || bobMsg == carolMsg {
		t.Fatal("two presses shared one card")
	}

	env.tapUndoCard(bob, undoConfirmCode, bobToken, bobMsg)
	env.waitRuns()
	unbansAfterBob := env.unbansIn()
	if unbansAfterBob != len(env.groups) {
		t.Fatalf("unban calls after Bob's undo = %d, want one per group", unbansAfterBob)
	}
	writesOf := func() int {
		total := len(env.writes(env.staffChat))
		for _, group := range env.groups {
			total += len(env.writes(group))
		}
		return total
	}
	writesBefore := writesOf()

	env.tapUndoCard(carol, undoConfirmCode, carolToken, carolMsg)
	env.waitRuns()

	text := env.lastEditText(env.staffChat, carolMsg)
	if !strings.Contains(text, staffMarker("staff_undo_already")) || !strings.Contains(text, "Bob") {
		t.Fatalf("Carol's card did not say the action was already undone by Bob:\n%s", text)
	}
	if state := cardState(t, carolToken); state != staffCardAborted {
		t.Fatalf("Carol's card state = %q, want %q", state, staffCardAborted)
	}
	if got := env.unbansIn(); got != unbansAfterBob {
		t.Fatalf("Carol's card made %d more unban call(s), want none", got-unbansAfterBob)
	}
	if got := writesOf(); got != writesBefore {
		t.Fatalf("Carol's card made %d write call(s), want none", got-writesBefore)
	}
	done, _ := recordOfCard(t, msgID)
	if done.UndoBy == nil || *done.UndoBy != bob.Id {
		t.Fatalf("record undo_by = %v, want Bob %d", done.UndoBy, bob.Id)
	}

	// The Undo button of the original answers the same.
	before := len(env.fake.sentTo(env.staffChat))
	env.tapData(carol, env.staffChatObj(), msgID,
		encodeCallbackData(staffCallbackNamespace, map[string]string{"a": undoAskCode, "r": fmt.Sprint(action.ID)}))
	if answer, _ := env.lastAnswer(); !strings.Contains(answer, staffMarker("staff_undo_already")) {
		t.Fatalf("answer to Undo on an undone action = %q, want the already-undone text", answer)
	}
	if got := len(env.fake.sentTo(env.staffChat)); got != before {
		t.Fatalf("Undo on an undone action posted %d message(s), want none", got-before)
	}
}

func TestStaffUndoOnceAcrossReplicas(t *testing.T) {
	a := newStaffActionEnv(t, 2)
	b := a.otherReplica()
	bob := a.undoPresser("Bob")
	carol := a.undoMember("Carol", 2)
	// Replica B sees the same live state as A: the target banned, Bob and Carol
	// administrators with the restrict right in every group.
	for _, group := range a.groups {
		b.fake.setMember(group, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusKicked})
	}
	b.undoMember("Bob", 1)
	b.undoMember("Carol", 2)
	action, msgID := a.undoFinishedBan()

	// Both cards are created through replica A and live in the shared Redis.
	bobToken, bobMsg := a.askUndo(bob, msgID, action.ID)
	carolToken, carolMsg := a.askUndo(carol, msgID, action.ID)

	updates := []struct {
		env    *staffActionEnv
		update *gotgbot.Update
	}{
		{a, a.callbackUpdate(bob, undoConfirmCode, bobToken, bobMsg)},
		{b, b.callbackUpdate(carol, undoConfirmCode, carolToken, carolMsg)},
	}
	var ready, finished sync.WaitGroup
	start := make(chan struct{})
	ready.Add(len(updates))
	finished.Add(len(updates))
	for _, u := range updates {
		go func() {
			defer finished.Done()
			ready.Done()
			<-start
			if err := u.env.dispatcher.ProcessUpdate(u.env.bot, u.update, nil); err != nil {
				t.Errorf("ProcessUpdate(%d) error = %v", u.update.UpdateId, err)
			}
		}()
	}
	ready.Wait()
	close(start)
	finished.Wait()
	a.waitRuns()

	done, _ := recordOfCard(t, msgID)
	if done.UndoBy == nil || (*done.UndoBy != bob.Id && *done.UndoBy != carol.Id) {
		t.Fatalf("record undo_by = %v, want one of the two pressers", done.UndoBy)
	}
	if done.UndoFinishedAt == nil {
		t.Fatal("the record has no undo_finished_at")
	}

	// Each card is read on the replica that handled its Confirm.
	cards := []struct {
		env   *staffActionEnv
		msgID int64
	}{{a, bobMsg}, {b, carolMsg}}
	summaries, losers := 0, 0
	for _, card := range cards {
		edits := card.env.edits(card.env.staffChat, card.msgID)
		text := ""
		if len(edits) > 0 {
			text = fmt.Sprint(edits[len(edits)-1].Params["text"])
		}
		switch {
		case summaryTallyRe.MatchString(text):
			summaries++
		case strings.Contains(text, staffMarker("staff_undo_already")):
			losers++
		case len(edits) == 0 && card.env.answersContaining("staff_act_target_busy") == 1:
			losers++
		default:
			t.Fatalf("a card ended neither on a summary, nor aborted as already undone, nor refused as busy (edits %d):\n%s", len(edits), text)
		}
	}
	if summaries != 1 || losers != 1 {
		t.Fatalf("cards ended on %d summary(ies) and %d loser(s), want exactly one of each", summaries, losers)
	}
	total := 0
	for _, env := range []*staffActionEnv{a, b} {
		total += env.unbansIn()
	}
	if total != len(a.groups) {
		t.Fatalf("unban calls across both replicas = %d, want one per group (%d)", total, len(a.groups))
	}
}

func TestStaffUndoTargetLock(t *testing.T) {
	t.Run("action in flight blocks undo", func(t *testing.T) {
		env := newStaffActionEnv(t, 2)
		bob := env.undoPresser("Bob")
		action, msgID := env.undoFinishedBan()

		// An unban of the same person is running: it holds the target lock.
		for _, group := range env.groups {
			env.fake.setDelay("getChatMember", group, 300*time.Millisecond)
		}
		env.startRun("/unban 4242")
		token, cardMsgID := env.askUndo(bob, msgID, action.ID)
		env.tapUndoCard(bob, undoConfirmCode, token, cardMsgID)

		if answer, alert := env.lastAnswer(); answer != staffMarker("staff_act_target_busy") || !alert {
			t.Fatalf("answer to Confirm while an action runs = %q alert=%v, want the target-busy alert", answer, alert)
		}
		if started := undoStartedAt(t, action.ID); started != nil {
			t.Fatalf("a busy Confirm claimed the record at %v", started)
		}
		if state := cardState(t, token); state != staffCardPending {
			t.Fatalf("undo card state = %q, want it still pending", state)
		}
		env.waitRuns()
	})

	t.Run("undo in flight blocks an action", func(t *testing.T) {
		env := newStaffActionEnv(t, 2)
		bob := env.undoPresser("Bob")
		action, msgID := env.undoFinishedBan()
		token, cardMsgID := env.askUndo(bob, msgID, action.ID)

		for _, group := range env.groups {
			env.fake.setDelay("unbanChatMember", group, 300*time.Millisecond)
		}
		env.tapUndoCard(bob, undoConfirmCode, token, cardMsgID)

		// While the undo runs, a new action on the same person cannot be confirmed.
		env.send(env.issuer, "/ban 4242")
		banToken, banMsg := env.card()
		env.tap(env.issuer, staffActRunConfirm, banToken, banMsg)

		if answer, alert := env.lastAnswer(); answer != staffMarker("staff_act_target_busy") || !alert {
			t.Fatalf("answer to a new action's Confirm while an undo runs = %q alert=%v, want the target-busy alert", answer, alert)
		}
		if state := cardState(t, banToken); state != staffCardPending {
			t.Fatalf("new action card state = %q, want it still pending", state)
		}
		env.waitRuns()
		for _, group := range env.groups {
			if got := env.bansOf(group, staffTestTarget); got != 1 {
				t.Fatalf("group %d ban calls = %d, want only the first ban's", group, got)
			}
		}
		if holder := targetLockHolder(t, staffTestTarget); holder != "" {
			t.Fatalf("target lock holder after the undo = %q, want the key gone", holder)
		}
	})
}

// waitForStaffRequest polls until the fake has seen more requests of method to the
// given groups than counts says (one count per group), or fails after 5 s.
func (e *staffActionEnv) waitForStaffRequest(method string, groups []int64, counts []int) {
	e.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for i, group := range groups {
			if len(e.fake.requestTimes(method, group)) > counts[i] {
				return
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	e.t.Fatalf("no %s request reached the linked groups within 5s", method)
}

// requestCounts is how many requests of method each group has seen so far.
func (e *staffActionEnv) requestCounts(method string, groups []int64) []int {
	counts := make([]int, len(groups))
	for i, group := range groups {
		counts[i] = len(e.fake.requestTimes(method, group))
	}
	return counts
}

func TestStopStaffActionsUndo(t *testing.T) {
	// resetStaffActionsContext puts a live context back after StopStaffActions
	// cancelled the shared one.
	resetStaffActionsContext := func(t *testing.T) {
		t.Helper()
		t.Cleanup(func() {
			staffActionsMu.Lock()
			defer staffActionsMu.Unlock()
			staffActionsCtx, staffActionsCancel = context.WithCancel(context.Background())
		})
	}

	t.Run("cut off before any write gives the claim back", func(t *testing.T) {
		env := newStaffActionEnv(t, 8)
		seedMembers(env)
		bob := env.undoPresser("Bob")
		withStaffActionTimers(t, time.Hour, 5*time.Millisecond)
		resetStaffActionsContext(t)
		action, msgID := env.undoFinishedBan()
		token, cardMsgID := env.askUndo(bob, msgID, action.ID)

		for _, group := range env.groups {
			env.fake.setDelay("getChatMember", group, 200*time.Millisecond)
		}
		lookups := env.requestCounts("getChatMember", env.groups)
		env.tapUndoCard(bob, undoConfirmCode, token, cardMsgID)
		env.waitForStaffRequest("getChatMember", env.groups, lookups)
		stopped := time.Now()
		StopStaffActions()
		if took := time.Since(stopped); took > 5*time.Second {
			t.Fatalf("StopStaffActions took %s, want under 5s", took)
		}
		env.waitRuns()

		summary := env.lastEditText(env.staffChat, cardMsgID)
		if !strings.Contains(summary, staffMarker("staff_act_fail_interrupted")) {
			t.Fatalf("final undo summary has no interrupted group:\n%s", summary)
		}
		if strings.Contains(summary, "⏳") {
			t.Fatalf("final undo summary still has a pending line:\n%s", summary)
		}
		done, skipped, failed := summaryTallyOf(t, summary)
		if done+skipped+failed != len(env.groups) {
			t.Fatalf("tally %d+%d+%d, want %d groups", done, skipped, failed, len(env.groups))
		}

		// No group reached its write, so the action's one undo was given back.
		wantUndoClaimGivenBack(t, msgID)
		if holder := targetLockHolder(t, staffTestTarget); holder != "" {
			t.Fatalf("target lock holder after the shutdown = %q, want the key gone", holder)
		}
	})

	t.Run("a write reached keeps the claim", func(t *testing.T) {
		env := newStaffActionEnv(t, 8)
		seedMembers(env)
		bob := env.undoPresser("Bob")
		withStaffActionTimers(t, time.Hour, 5*time.Millisecond)
		resetStaffActionsContext(t)
		action, msgID := env.undoFinishedBan()
		token, cardMsgID := env.askUndo(bob, msgID, action.ID)

		for _, group := range env.groups {
			env.fake.setDelay("unbanChatMember", group, 200*time.Millisecond)
		}
		unbans := env.requestCounts("unbanChatMember", env.groups)
		env.tapUndoCard(bob, undoConfirmCode, token, cardMsgID)
		env.waitForStaffRequest("unbanChatMember", env.groups, unbans)
		StopStaffActions()
		env.waitRuns()

		summary := env.lastEditText(env.staffChat, cardMsgID)
		if strings.Contains(summary, "⏳") {
			t.Fatalf("final undo summary still has a pending line:\n%s", summary)
		}
		// A group reached its write before the shutdown, so the undo is spent and its
		// record is finalized.
		record, rows := recordOfCard(t, msgID)
		if record.UndoStartedAt == nil || record.UndoFinishedAt == nil {
			t.Fatalf("record undo_started_at = %v, undo_finished_at = %v, want both set", record.UndoStartedAt, record.UndoFinishedAt)
		}
		undone := 0
		for _, row := range rows {
			if row.UndoOutcome == "" {
				t.Fatalf("group %d has no undo outcome: the shutdown dropped it from the record", row.GroupChatID)
			}
			if row.UndoOutcome == "done" {
				undone++
			}
		}
		if undone == 0 {
			t.Fatal("no group was recorded as undone, although one reached its write")
		}
		original := env.lastEditText(env.staffChat, msgID)
		if !strings.Contains(original, staffMarker("staff_undo_marker")) {
			t.Fatalf("the original summary was not marked as undone:\n%s", original)
		}
		if holder := targetLockHolder(t, staffTestTarget); holder != "" {
			t.Fatalf("target lock holder after the shutdown = %q, want the key gone", holder)
		}
	})
}

func TestStaffUndoNoTimeLimit(t *testing.T) {
	env := newStaffActionEnv(t, 2)
	bob := env.undoPresser("Bob")
	action, msgID := env.undoFinishedBan()

	past := time.Now().Add(-90 * 24 * time.Hour)
	if err := db.DB.Model(&models.StaffAction{}).Where("id = ?", action.ID).
		Updates(map[string]any{"created_at": past, "finished_at": past}).Error; err != nil {
		t.Fatalf("age the record: %v", err)
	}

	sentBefore := len(env.fake.sentTo(env.staffChat))
	token, cardMsgID := env.askUndo(bob, msgID, action.ID)
	if got := len(env.fake.sentTo(env.staffChat)); got != sentBefore+1 {
		t.Fatalf("Undo on a 90-day-old record posted %d message(s), want the one undo card", got-sentBefore)
	}

	env.tapUndoCard(bob, undoConfirmCode, token, cardMsgID)
	env.waitRuns()
	for _, group := range env.groups {
		wantMember(t, env.fake, group, staffTestTarget, gotgbot.ChatMemberStatusLeft)
	}
	if started := undoStartedAt(t, action.ID); started == nil {
		t.Fatal("the old record was not claimed by its undo")
	}
}
