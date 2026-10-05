//go:build testtools

package modules

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/divkix/Alita_Robot/alita/utils/cache"
)

// shortenExpirySlack makes the expiry timer fire d after the card's own lifetime.
func shortenExpirySlack(t *testing.T, d time.Duration) {
	t.Helper()
	previous := staffActionExpirySlack
	staffActionExpirySlack = d
	t.Cleanup(func() { staffActionExpirySlack = previous })
}

// callbackUpdate builds a callback query update for a card button without
// processing it, so several taps can be dispatched from separate goroutines.
func (e *staffActionEnv) callbackUpdate(from gotgbot.User, action, token string, msgID int64) *gotgbot.Update {
	data := encodeCallbackData(staffCallbackNamespace, map[string]string{"a": action, "t": token})
	if data == "" {
		e.t.Fatalf("callback data for %s did not encode", action)
	}
	id := e.updateID()
	return &gotgbot.Update{
		UpdateId: id,
		CallbackQuery: &gotgbot.CallbackQuery{
			Id:           fmt.Sprintf("cb-%d", id),
			From:         from,
			Message:      gotgbot.Message{MessageId: msgID, Date: 1, Chat: e.staffChatObj()},
			Data:         data,
			ChatInstance: "staff-action-test",
		},
	}
}

// waitForEdits waits until the card message has been edited n times. The wait is
// only an upper bound for a timer to fire; it never decides a result.
func (e *staffActionEnv) waitForEdits(msgID int64, n int) {
	e.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if len(e.edits(e.staffChat, msgID)) >= n {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	e.t.Fatalf("message %d was edited %d times within 5s, want %d", msgID, len(e.edits(e.staffChat, msgID)), n)
}

// answersContaining counts the callback answers whose text carries the marker of
// the locale key.
func (e *staffActionEnv) answersContaining(key string) int {
	count := 0
	for _, call := range e.fake.callsFor("answerCallbackQuery") {
		if text, _ := call.Params["text"].(string); strings.Contains(text, staffMarker(key)) {
			count++
		}
	}
	return count
}

// bansOf counts the banChatMember calls about userID addressed to chatID.
func (e *staffActionEnv) bansOf(chatID, userID int64) int {
	count := 0
	for _, call := range e.callsTo("banChatMember", chatID) {
		if staffParamInt(call.Params, "user_id") == userID {
			count++
		}
	}
	return count
}

// otherReplica is a second bot instance for the same Staff Group: its own bot,
// fake Bot API and dispatcher, sharing with the first only what real replicas
// share, the Redis client and the database.
func (e *staffActionEnv) otherReplica() *staffActionEnv {
	e.t.Helper()
	fake := newStaffActionFake()
	bot := newModuleTestBot(fake.moduleBotClient)
	bot.BotClient = fake

	replica := &staffActionEnv{
		t:          e.t,
		fake:       fake,
		bot:        bot,
		staffChat:  e.staffChat,
		groups:     e.groups,
		owner:      e.owner,
		issuer:     e.issuer,
		nextUpdate: 50000,
	}
	fake.setCreator(e.staffChat, e.owner)
	for _, group := range e.groups {
		fake.setCreator(group, e.owner)
		fake.setMember(group, e.issuer.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusAdministrator, CanRestrictMembers: true})
	}
	replica.dispatcher = ext.NewDispatcher(&ext.DispatcherOpts{MaxRoutines: -1})
	LoadStaffActions(replica.dispatcher)
	LoadStaff(replica.dispatcher)
	return replica
}

func TestStaffActionCardExpiresByTimer(t *testing.T) {
	shortenCardLifetime(t, 50*time.Millisecond)
	shortenExpirySlack(t, 10*time.Millisecond)
	env := newStaffActionEnv(t, 1)
	group := env.groups[0]
	env.fake.setMember(group, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})

	env.send(env.issuer, "/ban 4242")
	token, msgID := env.card()

	env.waitForEdits(msgID, 1)
	edits := env.edits(env.staffChat, msgID)
	if len(edits) != 1 {
		t.Fatalf("edits of the card = %d, want exactly one", len(edits))
	}
	if text := fmt.Sprint(edits[0].Params["text"]); !strings.Contains(text, staffMarker("staff_act_card_expired_text")) {
		t.Fatalf("expiry edit = %q, want the expired text", text)
	}
	wantNoKeyboard(t, edits[0])
	if state := cardState(t, token); state != staffCardExpired {
		t.Fatalf("card state = %q, want %q", state, staffCardExpired)
	}

	env.tap(env.issuer, staffActRunConfirm, token, msgID)
	env.waitRuns()
	if got := env.answersContaining("staff_act_card_expired"); got != 1 {
		text, _ := env.lastAnswer()
		t.Fatalf("expired toasts = %d (last answer %q), want 1", got, text)
	}
	if edits := env.edits(env.staffChat, msgID); len(edits) != 1 {
		t.Fatalf("edits after the late Confirm = %d, want still one", len(edits))
	}
	if writes := env.writes(group); len(writes) != 0 {
		t.Fatalf("write calls after an expired Confirm = %+v, want none", writes)
	}
}

func TestStaffActionCardExpiryBoundary(t *testing.T) {
	env := newStaffActionEnv(t, 1)
	group := env.groups[0]
	env.fake.setMember(group, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
	client := cache.GetRedisClient()

	setExpiry := func(token string, at time.Time) {
		t.Helper()
		if err := client.HSet(cache.Context, staffCardKey(token), "expires_at", at.UnixMilli()).Err(); err != nil {
			t.Fatalf("set expires_at: %v", err)
		}
	}

	// A tap at or after expires_at expires the card.
	env.send(env.issuer, "/ban 4242")
	token, msgID := env.card()
	setExpiry(token, time.Now().Add(-time.Millisecond))
	env.tap(env.issuer, staffActRunConfirm, token, msgID)
	env.waitRuns()
	if got := env.answersContaining("staff_act_card_expired"); got != 1 {
		t.Fatalf("expired toasts = %d, want 1", got)
	}
	if text := env.lastEditText(env.staffChat, msgID); !strings.Contains(text, staffMarker("staff_act_card_expired_text")) {
		t.Fatalf("card after the late tap = %q, want the expired text", text)
	}
	wantNoKeyboard(t, env.edits(env.staffChat, msgID)[0])
	if writes := env.writes(group); len(writes) != 0 {
		t.Fatalf("write calls after an expired Confirm = %+v, want none", writes)
	}
	if state := cardState(t, token); state != staffCardExpired {
		t.Fatalf("card state = %q, want %q", state, staffCardExpired)
	}

	// A tap a second before expires_at still runs.
	env.send(env.issuer, "/ban 4242")
	token, msgID = env.card()
	setExpiry(token, time.Now().Add(time.Second))
	env.tap(env.issuer, staffActRunConfirm, token, msgID)
	env.waitRuns()
	wantBanned(t, env.fake, group, staffTestTarget)
	if state := cardState(t, token); state != staffCardDone {
		t.Fatalf("card state = %q, want %q", state, staffCardDone)
	}
}

func TestStaffActionConfirmExactlyOnce(t *testing.T) {
	env := newStaffActionEnv(t, 2)

	for i := 0; i < 20; i++ {
		target := staffTestTarget + int64(i)
		for _, group := range env.groups {
			env.fake.setMember(group, target, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
		}
		env.send(env.issuer, fmt.Sprintf("/ban %d", target))
		token, msgID := env.card()
		answersBefore := len(env.fake.callsFor("answerCallbackQuery"))
		handledBefore := env.answersContaining("staff_act_card_handled")

		updates := []*gotgbot.Update{
			env.callbackUpdate(env.issuer, staffActRunConfirm, token, msgID),
			env.callbackUpdate(env.issuer, staffActRunConfirm, token, msgID),
		}
		var ready, finished sync.WaitGroup
		start := make(chan struct{})
		ready.Add(len(updates))
		finished.Add(len(updates))
		for _, update := range updates {
			go func() {
				defer finished.Done()
				ready.Done()
				<-start
				if err := env.dispatcher.ProcessUpdate(env.bot, update, nil); err != nil {
					t.Errorf("ProcessUpdate(%d) error = %v", update.UpdateId, err)
				}
			}()
		}
		ready.Wait()
		close(start)
		finished.Wait()
		env.waitRuns()

		for _, group := range env.groups {
			if got := env.bansOf(group, target); got != 1 {
				t.Fatalf("round %d: banChatMember calls for %d in group %d = %d, want exactly 1", i, target, group, got)
			}
		}
		if answers := len(env.fake.callsFor("answerCallbackQuery")) - answersBefore; answers != 2 {
			t.Fatalf("round %d: callback answers = %d, want one per tap", i, answers)
		}
		if got := env.answersContaining("staff_act_card_handled") - handledBefore; got != 1 {
			t.Fatalf("round %d: already-handled toasts = %d, want exactly one of the two taps", i, got)
		}
		if state := cardState(t, token); state != staffCardDone {
			t.Fatalf("round %d: card state = %q, want %q", i, state, staffCardDone)
		}
	}
}

func TestStaffActionConfirmAcrossReplicas(t *testing.T) {
	a := newStaffActionEnv(t, 2)
	b := a.otherReplica()
	for _, group := range a.groups {
		b.fake.setMember(group, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
	}

	// Replica A creates the card; replica B, which has never seen it, handles Confirm.
	a.send(a.issuer, "/ban 4242")
	token, msgID := a.card()
	b.tap(b.issuer, staffActRunConfirm, token, msgID)
	b.waitRuns()

	for _, group := range a.groups {
		if got := b.bansOf(group, staffTestTarget); got != 1 {
			t.Fatalf("replica B: banChatMember calls in group %d = %d, want 1", group, got)
		}
		wantBanned(t, b.fake, group, staffTestTarget)
		if writes := a.writes(group); len(writes) != 0 {
			t.Fatalf("replica A: write calls in group %d = %+v, want none", group, writes)
		}
	}
	if state := cardState(t, token); state != staffCardDone {
		t.Fatalf("card state = %q, want %q", state, staffCardDone)
	}

	// Replica A's expiry timer fires afterwards: the card is done, so it edits nothing.
	expireStaffActionCard(a.bot, token, a.staffChat, msgID)
	if edits := a.edits(a.staffChat, msgID); len(edits) != 0 {
		t.Fatalf("replica A edited the finished card %d times, want none", len(edits))
	}
	if state := cardState(t, token); state != staffCardDone {
		t.Fatalf("card state after A's timer = %q, want %q", state, staffCardDone)
	}
}

func TestStaffActionTimerSkipsCancelledCard(t *testing.T) {
	env := newStaffActionEnv(t, 1)
	env.send(env.issuer, "/ban 4242")
	token, msgID := env.card()

	env.tap(env.issuer, staffActRunCancel, token, msgID)
	if edits := env.edits(env.staffChat, msgID); len(edits) != 1 {
		t.Fatalf("edits after Cancel = %d, want 1", len(edits))
	}

	expireStaffActionCard(env.bot, token, env.staffChat, msgID)

	edits := env.edits(env.staffChat, msgID)
	if len(edits) != 1 {
		t.Fatalf("edits after the timer = %d, want only the cancel edit", len(edits))
	}
	if text := fmt.Sprint(edits[0].Params["text"]); !strings.Contains(text, staffMarker("staff_act_card_cancelled")) {
		t.Fatalf("the one edit = %q, want the cancel text", text)
	}
	if state := cardState(t, token); state != staffCardCancelled {
		t.Fatalf("card state = %q, want %q", state, staffCardCancelled)
	}
}
