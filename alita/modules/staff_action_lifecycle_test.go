//go:build testtools

package modules

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/db/staff"
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

// addLinkedGroup links one more group, with the same creator and the issuer an
// administrator in it, and returns its chat ID.
func (e *staffActionEnv) addLinkedGroup(title string) int64 {
	e.t.Helper()
	group := uniqueModuleChatID()
	staffCleanup(e.t, group)
	e.fake.setCreator(group, e.owner)
	e.fake.setMember(group, e.issuer.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusAdministrator, CanRestrictMembers: true})
	link := &models.StaffGroupLink{GroupChatID: group, StaffChatID: e.staffChat, OwnerUserID: e.owner, GroupTitle: title}
	if err := staff.CreateLink(link); err != nil {
		e.t.Fatalf("create link for %s: %v", title, err)
	}
	return group
}

// removeLink unlinks a group.
func (e *staffActionEnv) removeLink(group int64) {
	e.t.Helper()
	link, err := staff.GetLinkOfGroupFresh(group)
	if err != nil || link == nil {
		e.t.Fatalf("read link of group %d: %v (link %v)", group, err, link)
	}
	if deleted, err := staff.DeleteLink(link.ID); err != nil || !deleted {
		e.t.Fatalf("delete link of group %d: deleted=%v err=%v", group, deleted, err)
	}
}

// targetLockHolder reads the value of the target lock, "" when there is none.
func targetLockHolder(t *testing.T, target int64) string {
	t.Helper()
	value, err := cache.GetRedisClient().Get(cache.Context, fmt.Sprintf("%s%d", staffTargetLockPrefix, target)).Result()
	if err != nil {
		return ""
	}
	return value
}

func TestStaffActionLinksChangedAborts(t *testing.T) {
	t.Run("a group was linked", func(t *testing.T) {
		env := newStaffActionEnv(t, 2)
		for _, group := range env.groups {
			env.fake.setMember(group, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
		}
		env.send(env.issuer, "/ban 4242")
		token, msgID := env.card()
		third := env.addLinkedGroup("Group C")
		env.fake.setMember(third, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})

		env.tap(env.issuer, staffActRunConfirm, token, msgID)
		env.waitRuns()

		text := env.lastEditText(env.staffChat, msgID)
		if !strings.Contains(text, staffMarker("staff_act_abort_links_changed")+" 2 3") {
			t.Fatalf("card = %q, want the links-changed abort with the old count 2 and the new count 3", text)
		}
		wantNoKeyboard(t, env.edits(env.staffChat, msgID)[len(env.edits(env.staffChat, msgID))-1])
		for _, group := range append(append([]int64{}, env.groups...), third) {
			if writes := env.writes(group); len(writes) != 0 {
				t.Fatalf("write calls in group %d = %+v, want none", group, writes)
			}
		}
		if state := cardState(t, token); state != staffCardAborted {
			t.Fatalf("card state = %q, want %q", state, staffCardAborted)
		}
	})

	t.Run("one group swapped for another", func(t *testing.T) {
		env := newStaffActionEnv(t, 2)
		for _, group := range env.groups {
			env.fake.setMember(group, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
		}
		env.send(env.issuer, "/ban 4242")
		token, msgID := env.card()
		env.removeLink(env.groups[0])
		swapped := env.addLinkedGroup("Group C")
		env.fake.setMember(swapped, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})

		env.tap(env.issuer, staffActRunConfirm, token, msgID)
		env.waitRuns()

		text := env.lastEditText(env.staffChat, msgID)
		if !strings.Contains(text, staffMarker("staff_act_abort_links_changed")+" 2 2") {
			t.Fatalf("card = %q, want the links-changed abort although the count is unchanged", text)
		}
		for _, group := range []int64{env.groups[0], env.groups[1], swapped} {
			if writes := env.writes(group); len(writes) != 0 {
				t.Fatalf("write calls in group %d = %+v, want none", group, writes)
			}
		}
		if state := cardState(t, token); state != staffCardAborted {
			t.Fatalf("card state = %q, want %q", state, staffCardAborted)
		}
	})

	t.Run("links unchanged", func(t *testing.T) {
		env := newStaffActionEnv(t, 2)
		for _, group := range env.groups {
			env.fake.setMember(group, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
		}
		env.send(env.issuer, "/ban 4242")
		token, msgID := env.card()

		env.tap(env.issuer, staffActRunConfirm, token, msgID)
		env.waitRuns()

		for _, group := range env.groups {
			wantBanned(t, env.fake, group, staffTestTarget)
		}
		if state := cardState(t, token); state != staffCardDone {
			t.Fatalf("card state = %q, want %q", state, staffCardDone)
		}
	})
}

func TestStaffActionTargetLock(t *testing.T) {
	env := newStaffActionEnv(t, 1)
	group := env.groups[0]
	env.fake.setMember(group, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
	env.send(env.issuer, "/ban 4242")
	token, msgID := env.card()

	// Another card's run holds the target.
	taken, err := acquireStaffTargetLock(staffTestTarget, "othertoken")
	if err != nil || !taken {
		t.Fatalf("acquireStaffTargetLock = %v, %v, want it taken", taken, err)
	}
	env.tap(env.issuer, staffActRunConfirm, token, msgID)
	env.waitRuns()

	text, alert := env.lastAnswer()
	if !strings.Contains(text, staffMarker("staff_act_target_busy")) || !alert {
		t.Fatalf("answer = %q (alert %v), want the target-busy alert", text, alert)
	}
	if writes := env.writes(group); len(writes) != 0 {
		t.Fatalf("write calls while the target was busy = %+v, want none", writes)
	}
	if state := cardState(t, token); state != staffCardPending {
		t.Fatalf("card state = %q, want it still %q", state, staffCardPending)
	}
	if edits := env.edits(env.staffChat, msgID); len(edits) != 0 {
		t.Fatalf("the busy answer edited the card %d times, want none", len(edits))
	}
	if holder := targetLockHolder(t, staffTestTarget); holder != "othertoken" {
		t.Fatalf("target lock holder = %q, want the other card to keep it", holder)
	}

	// The other run ends; the same card can be confirmed now.
	releaseStaffTargetLock(staffTestTarget, "othertoken")
	env.tap(env.issuer, staffActRunConfirm, token, msgID)
	env.waitRuns()
	wantBanned(t, env.fake, group, staffTestTarget)
	if state := cardState(t, token); state != staffCardDone {
		t.Fatalf("card state = %q, want %q", state, staffCardDone)
	}
}

func TestStaffActionTargetLockReleased(t *testing.T) {
	t.Run("after a completed run", func(t *testing.T) {
		env := newStaffActionEnv(t, 1)
		env.fake.setMember(env.groups[0], staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
		env.send(env.issuer, "/ban 4242")
		token, msgID := env.card()

		env.tap(env.issuer, staffActRunConfirm, token, msgID)
		env.waitRuns()

		wantBanned(t, env.fake, env.groups[0], staffTestTarget)
		if holder := targetLockHolder(t, staffTestTarget); holder != "" {
			t.Fatalf("target lock holder after the run = %q, want the key gone", holder)
		}
	})

	t.Run("after a Confirm aborted by changed links", func(t *testing.T) {
		env := newStaffActionEnv(t, 1)
		env.send(env.issuer, "/ban 4242")
		token, msgID := env.card()
		env.addLinkedGroup("Group B")

		env.tap(env.issuer, staffActRunConfirm, token, msgID)
		env.waitRuns()

		if state := cardState(t, token); state != staffCardAborted {
			t.Fatalf("card state = %q, want %q", state, staffCardAborted)
		}
		if holder := targetLockHolder(t, staffTestTarget); holder != "" {
			t.Fatalf("target lock holder after the abort = %q, want the key gone", holder)
		}
	})

	t.Run("after a Confirm refused for a late tap", func(t *testing.T) {
		env := newStaffActionEnv(t, 1)
		env.send(env.issuer, "/ban 4242")
		token, msgID := env.card()
		if err := cache.GetRedisClient().HSet(cache.Context, staffCardKey(token), "expires_at", time.Now().Add(-time.Second).UnixMilli()).Err(); err != nil {
			t.Fatalf("set expires_at: %v", err)
		}

		env.tap(env.issuer, staffActRunConfirm, token, msgID)

		if holder := targetLockHolder(t, staffTestTarget); holder != "" {
			t.Fatalf("target lock holder after an expired tap = %q, want the key gone", holder)
		}
	})
}

func TestStaffActionTargetLockForeignRelease(t *testing.T) {
	withMiniredis(t)

	taken, err := acquireStaffTargetLock(staffTestTarget, "holder")
	if err != nil || !taken {
		t.Fatalf("acquireStaffTargetLock = %v, %v, want it taken", taken, err)
	}
	if holder := targetLockHolder(t, staffTestTarget); holder != "holder" {
		t.Fatalf("target lock holder = %q, want %q", holder, "holder")
	}
	if again, err := acquireStaffTargetLock(staffTestTarget, "second"); err != nil || again {
		t.Fatalf("second acquire = %v, %v, want it refused", again, err)
	}

	releaseStaffTargetLock(staffTestTarget, "wrong")
	if holder := targetLockHolder(t, staffTestTarget); holder != "holder" {
		t.Fatalf("target lock holder after a foreign release = %q, want it kept by %q", holder, "holder")
	}

	releaseStaffTargetLock(staffTestTarget, "holder")
	if holder := targetLockHolder(t, staffTestTarget); holder != "" {
		t.Fatalf("target lock holder after its own release = %q, want the key gone", holder)
	}
}

// gatedBanClient holds every banChatMember until release is closed, so a test can
// tap a card while its run is still in flight.
type gatedBanClient struct {
	*staffActionFake
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (g *gatedBanClient) RequestWithContext(
	ctx context.Context,
	token, method string,
	params map[string]any,
	opts *gotgbot.RequestOpts,
) (json.RawMessage, error) {
	if method == "banChatMember" {
		g.once.Do(func() { close(g.entered) })
		<-g.release
	}
	return g.staffActionFake.RequestWithContext(ctx, token, method, params, opts)
}

func TestStaffActionOwnCardDoubleTap(t *testing.T) {
	env := newStaffActionEnv(t, 1)
	group := env.groups[0]
	env.fake.setMember(group, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
	gate := &gatedBanClient{staffActionFake: env.fake, entered: make(chan struct{}), release: make(chan struct{})}
	env.bot.BotClient = gate
	var releaseOnce sync.Once
	releaseRun := func() { releaseOnce.Do(func() { close(gate.release) }) }
	t.Cleanup(releaseRun)

	env.send(env.issuer, "/ban 4242")
	token, msgID := env.card()
	env.tap(env.issuer, staffActRunConfirm, token, msgID)
	select {
	case <-gate.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the run never reached its first ban")
	}
	editsBefore := len(env.edits(env.staffChat, msgID))

	// The run is in flight and holds the target lock; the issuer taps again.
	env.tap(env.issuer, staffActRunConfirm, token, msgID)

	if got := env.answersContaining("staff_act_card_handled"); got != 1 {
		text, _ := env.lastAnswer()
		t.Fatalf("already-handled toasts = %d (last answer %q), want 1", got, text)
	}
	if got := env.answersContaining("staff_act_target_busy"); got != 0 {
		t.Fatalf("target-busy answers = %d, want none for the issuer's own running card", got)
	}
	if edits := len(env.edits(env.staffChat, msgID)); edits != editsBefore {
		t.Fatalf("the second tap edited the card: %d edits, want %d", edits, editsBefore)
	}

	releaseRun()
	env.waitRuns()
	if got := env.bansOf(group, staffTestTarget); got != 1 {
		t.Fatalf("banChatMember calls = %d, want exactly 1", got)
	}
}

// targetLockPTTL is the remaining life of the target lock, 0 when there is none.
func targetLockPTTL(t *testing.T, target int64) time.Duration {
	t.Helper()
	ttl, err := cache.GetRedisClient().PTTL(cache.Context, staffTargetLockKey(target)).Result()
	if err != nil || ttl < 0 {
		return 0
	}
	return ttl
}

// heldStaffRun starts a ban run on a gated client and returns once its first ban
// call is in flight, so the run holds its target lock and its coordinator loop is
// alive. The renewal interval is shortened for the test. The returned function
// lets the run finish and waits for it.
func heldStaffRun(t *testing.T) (env *staffActionEnv, token string, finish func()) {
	t.Helper()
	previous := staffTargetLockRenewEvery
	staffTargetLockRenewEvery = 20 * time.Millisecond
	t.Cleanup(func() { staffTargetLockRenewEvery = previous })

	env = newStaffActionEnv(t, 1)
	group := env.groups[0]
	env.fake.setMember(group, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
	gate := &gatedBanClient{staffActionFake: env.fake, entered: make(chan struct{}), release: make(chan struct{})}
	env.bot.BotClient = gate
	var releaseOnce sync.Once
	releaseRun := func() { releaseOnce.Do(func() { close(gate.release) }) }
	t.Cleanup(releaseRun)

	env.send(env.issuer, "/ban 4242")
	token, msgID := env.card()
	env.tap(env.issuer, staffActRunConfirm, token, msgID)
	select {
	case <-gate.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the run never reached its first ban")
	}
	return env, token, func() {
		releaseRun()
		env.waitRuns()
	}
}

func TestStaffActionTargetLockRenewedDuringRun(t *testing.T) {
	t.Run("renews its own lock", func(t *testing.T) {
		_, token, finish := heldStaffRun(t)

		// The run's lock is about to expire; a live run must push it back out.
		if err := cache.GetRedisClient().PExpire(cache.Context, staffTargetLockKey(staffTestTarget), 2*time.Second).Err(); err != nil {
			t.Fatalf("shorten the lock: %v", err)
		}
		deadline := time.Now().Add(time.Second)
		for targetLockPTTL(t, staffTestTarget) <= 2*time.Second && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}

		if holder := targetLockHolder(t, staffTestTarget); holder != token {
			t.Fatalf("target lock holder = %q, want the running card %q", holder, token)
		}
		if ttl := targetLockPTTL(t, staffTestTarget); ttl <= staffTargetLockTTL-time.Minute {
			t.Fatalf("target lock TTL = %s, want it renewed above %s", ttl, staffTargetLockTTL-time.Minute)
		}

		finish()
		if holder := targetLockHolder(t, staffTestTarget); holder != "" {
			t.Fatalf("target lock holder after the run = %q, want the key gone", holder)
		}
	})

	t.Run("re-takes a lock that vanished", func(t *testing.T) {
		_, token, finish := heldStaffRun(t)

		// Redis lost the key mid-run; nobody else holds the target.
		if err := cache.GetRedisClient().Del(cache.Context, staffTargetLockKey(staffTestTarget)).Err(); err != nil {
			t.Fatalf("delete the lock: %v", err)
		}
		deadline := time.Now().Add(time.Second)
		for targetLockHolder(t, staffTestTarget) != token && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}

		if holder := targetLockHolder(t, staffTestTarget); holder != token {
			t.Fatalf("target lock holder = %q, want the running card %q to take it back", holder, token)
		}
		if ttl := targetLockPTTL(t, staffTestTarget); ttl <= staffTargetLockTTL-time.Minute {
			t.Fatalf("target lock TTL = %s, want it above %s", ttl, staffTargetLockTTL-time.Minute)
		}

		finish()
		if holder := targetLockHolder(t, staffTestTarget); holder != "" {
			t.Fatalf("target lock holder after the run = %q, want the key gone", holder)
		}
	})

	t.Run("leaves a foreign lock alone", func(t *testing.T) {
		const foreign = "ffffffffffffffff"
		_, _, finish := heldStaffRun(t)

		// Another card's token replaces the lock; the run must not extend or take it.
		if err := cache.GetRedisClient().Set(cache.Context, staffTargetLockKey(staffTestTarget), foreign, 2*time.Second).Err(); err != nil {
			t.Fatalf("overwrite the lock: %v", err)
		}
		t.Cleanup(func() { cache.GetRedisClient().Del(cache.Context, staffTargetLockKey(staffTestTarget)) })
		time.Sleep(100 * time.Millisecond)

		if holder := targetLockHolder(t, staffTestTarget); holder != foreign {
			t.Fatalf("target lock holder = %q, want the foreign card to keep %q", holder, foreign)
		}
		if ttl := targetLockPTTL(t, staffTestTarget); ttl > 2*time.Second {
			t.Fatalf("target lock TTL = %s, want the foreign lock left at 2s or less", ttl)
		}

		finish()
		if holder := targetLockHolder(t, staffTestTarget); holder != foreign {
			t.Fatalf("target lock holder after the run = %q, want the foreign lock untouched", holder)
		}
	})
}
