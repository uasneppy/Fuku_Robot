//go:build testtools

package modules

import (
	"context"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
	"github.com/uasneppy/fuku_robot/fuku/utils/helpers"
	"github.com/uasneppy/fuku_robot/fuku/utils/ratelimit"
)

// Member deletion toolkit (plan 17-05). The environment is the Phase 16 deletion environment (one
// supergroup where the bot is an administrator with the delete right, the real dispatcher, the fixed
// deletion clock, the fast pacer, deletionInline.wait registered last) with the MemberDeletion
// module loaded on its dispatcher. Phase 16's environments are unchanged, so no Phase 16 test sees
// the new handlers.

// memberDeletionEnv is the deletion environment with the member deletion handlers loaded.
type memberDeletionEnv struct {
	*deletionEnv
}

// newMemberDeletionEnv builds the environment and lists its administrator in the fake's
// getChatAdministrators answer; the admin cache itself starts cold (see primeAdminCache).
func newMemberDeletionEnv(t *testing.T) *memberDeletionEnv {
	t.Helper()
	env := &memberDeletionEnv{deletionEnv: newDeletionEnv(t)}
	LoadMemberDeletion(env.dispatcher)
	env.fake.listAdministrator(env.chat.Id, env.admin.Id)
	memberReqCleanup(t, []int64{env.chat.Id}, []int64{env.admin.Id})
	t.Cleanup(func() { cache.InvalidateAdminCache(env.chat.Id) })
	return env
}

// member returns a plain member with a fresh ID, whose users row and cache entry are removed when
// the test ends.
func (e *memberDeletionEnv) member() gotgbot.User {
	e.t.Helper()
	user := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Member"}
	memberReqCleanup(e.t, nil, []int64{user.Id})
	return user
}

// primeAdminCache loads the chat's administrators into the admin cache, so the cache knows who is
// an administrator (the env's admin is listed) and a member is a known non-administrator.
func (e *memberDeletionEnv) primeAdminCache() {
	e.t.Helper()
	cache.InvalidateAdminCache(e.chat.Id)
	if got := cache.LoadAdminCache(e.bot, e.chat.Id); !got.Cached {
		e.t.Fatalf("LoadAdminCache() = %+v, want a cached list", got)
	}
}

// coldAdminCache empties the chat's admin cache entry and keeps it empty (the warm of the shared loader
// is a no-op for the test), so the cache cannot say who is an administrator and the handler has to ask
// Telegram about the member. warmableAdminCache is the cache that can be warmed.
func (e *memberDeletionEnv) coldAdminCache() {
	e.t.Helper()
	cache.InvalidateAdminCache(e.chat.Id)
	mreqSwap(e.t, &memberDeletionAdminWarm, func(*gotgbot.Bot, int64) {})
}

// warmableAdminCache empties the chat's admin cache entry and leaves the loader as it is, so the first
// check of a message loads the list once (WR-01).
func (e *memberDeletionEnv) warmableAdminCache() {
	e.t.Helper()
	cache.InvalidateAdminCache(e.chat.Id)
}

// resetMemberDeletionFailureState clears the failure log gate and the warm backoff now and when the
// test ends, so no line or backoff of one test is seen by another.
func resetMemberDeletionFailureState(t *testing.T) {
	t.Helper()
	reset := func() {
		for i := range memberDeletionWarnGates {
			memberDeletionWarnGates[i].mu.Lock()
			memberDeletionWarnGates[i].last = nil
			memberDeletionWarnGates[i].capWarned = false
			memberDeletionWarnGates[i].mu.Unlock()
		}
		memberDeletionWarmBackoff.mu.Lock()
		memberDeletionWarmBackoff.until = nil
		memberDeletionWarmBackoff.mu.Unlock()
	}
	reset()
	t.Cleanup(reset)
}

// storeMemberRule stores a member rule of the env chat through the real apply function.
func (e *memberDeletionEnv) storeMemberRule(kind string, delaySecs int) {
	e.t.Helper()
	res := applyDeleteRule(context.Background(), e.chat.Id, kind, deletionSet(delaySecs))
	if res.Outcome != deletionApplied && res.Outcome != deletionUnchanged {
		e.t.Fatalf("store %s = %d: outcome = %v, err = %v", kind, delaySecs, res.Outcome, res.Err)
	}
}

// sendEdit runs an edited_message update of the env chat through the real dispatcher: from sent the
// message at date and edited it at editDate, apply sets any further field. It returns the message ID.
func (e *memberDeletionEnv) sendEdit(from *gotgbot.User, date, editDate int64, apply func(*gotgbot.Message)) int64 {
	e.t.Helper()
	return e.sendEditIn(e.chat, from, date, editDate, apply)
}

// sendEditIn is sendEdit for any chat.
func (e *memberDeletionEnv) sendEditIn(
	chat gotgbot.Chat, from *gotgbot.User, date, editDate int64, apply func(*gotgbot.Message),
) int64 {
	e.t.Helper()
	id := e.updateID()
	msg := &gotgbot.Message{MessageId: id, Date: date, EditDate: editDate, Chat: chat, From: from}
	if apply != nil {
		apply(msg)
	}
	update := &gotgbot.Update{UpdateId: id, EditedMessage: msg}
	if err := e.dispatcher.ProcessUpdate(e.bot, update, nil); err != nil {
		e.t.Fatalf("ProcessUpdate(%d) error = %v", id, err)
	}
	return id
}

// lateEdit sends a message dated five minutes ago and edited gap seconds after its date.
func (e *memberDeletionEnv) lateEdit(from *gotgbot.User, gap int64, apply func(*gotgbot.Message)) int64 {
	e.t.Helper()
	date := deletionNow().Unix() - 300
	return e.sendEdit(from, date, date+gap, apply)
}

// deletedMessage counts the deleteMessage calls of message id in chatID.
func (e *memberDeletionEnv) deletedMessage(chatID, id int64) int {
	e.t.Helper()
	count := 0
	for _, call := range e.deletes(chatID) {
		if staffParamInt(call.Params, "message_id") == id {
			count++
		}
	}
	return count
}

// wantKept fails when message id of the env chat was deleted or a queue row exists.
func (e *memberDeletionEnv) wantKept(id int64) {
	e.t.Helper()
	if got := e.deletedMessage(e.chat.Id, id); got != 0 {
		e.t.Fatalf("deletes of message %d = %d, want none", id, got)
	}
	if rows := e.queued(e.chat.Id); len(rows) != 0 {
		e.t.Fatalf("queue rows = %+v, want none", rows)
	}
}

// wantDeletedOnce fails unless message id of the env chat was deleted exactly once, at once and not
// through the queue.
func (e *memberDeletionEnv) wantDeletedOnce(id int64) {
	e.t.Helper()
	if got := e.deletedMessage(e.chat.Id, id); got != 1 {
		e.t.Fatalf("deletes of message %d = %d, want 1", id, got)
	}
	if rows := e.queued(e.chat.Id); len(rows) != 0 {
		e.t.Fatalf("queue rows = %+v, want none", rows)
	}
}

// memberEditedRuleDelay is the grace of the member:edited rule most tests store.
const memberEditedRuleDelay = 60

// Member deletion batcher toolkit (plan 17-06).

// emptyMemberDeleteQueue drops every buffered entry and any pending kick.
func emptyMemberDeleteQueue() {
	for {
		select {
		case <-memberDeleteQueue:
		case <-memberDeleteKick:
		default:
			return
		}
	}
}

// drainMemberDeleteQueue empties the batcher's buffer now and again when the test ends, so no entry
// offered by one test is flushed by another. Register it before the test's row cleanup.
func drainMemberDeleteQueue(t *testing.T) {
	t.Helper()
	emptyMemberDeleteQueue()
	t.Cleanup(emptyMemberDeleteQueue)
}

// withMemberDeleteQueue swaps the buffer for one of the given size for the test. Nothing may be
// offering or flushing while it is called, and the batcher goroutine must not be running.
func withMemberDeleteQueue(t *testing.T, size int) {
	t.Helper()
	mreqSwap(t, &memberDeleteQueue, make(chan deletion.Entry, size))
}

// resetMemberDeleteState zeroes the counters, the shed log gate and the draining flag.
func resetMemberDeleteState() {
	memberDeleteDraining.Store(false)
	for i := range memberDeleteShedCounts {
		memberDeleteShedCounts[i].Store(0)
	}
	memberDeleteWritten.Store(0)
	memberDeleteDroppedOff.Store(0)
	memberDeletePinnedDropped.Store(0)
	memberPinnedNotes.mu.Lock()
	memberPinnedNotes.at = nil
	memberPinnedNotes.mu.Unlock()
	memberDeleteShedWarn.mu.Lock()
	memberDeleteShedWarn.last = nil
	memberDeleteShedWarn.capWarned = false
	memberDeleteShedWarn.mu.Unlock()
}

// restoreMemberDeleteBatcher resets the batcher's state now and when the test ends: draining off,
// the buffer emptied, the counters and the shed log gate cleared, a running batcher stopped and
// waited for, and a fresh deletion root in place. Register it after the test's mreqSwaps, so its
// clean-up (the stop) runs before they are put back and no batcher goroutine reads a variable
// while it is restored.
func restoreMemberDeleteBatcher(t *testing.T) {
	t.Helper()
	resetMemberDeleteState()
	emptyMemberDeleteQueue()
	t.Cleanup(func() {
		if done := memberDeleteStop(); done != nil {
			select {
			case <-done:
			case <-time.After(10 * time.Second):
				t.Error("the member deletion batcher did not stop")
			}
		}
		emptyMemberDeleteQueue()
		resetMemberDeleteState()
		freshDeletionRoot()
	})
}

// memberDeleteShedTotal returns how many entries were shed for one reason.
func memberDeleteShedTotal(reason int) int64 {
	return memberDeleteShedCounts[reason].Load()
}

// Self-destruct toolkit (plan 17-07).

// newSelfDestructEnv builds the member deletion environment with member:selfdestruct stored as
// delay and the admin cache primed (the env's admin is listed, a member is a known non-administrator),
// the batcher's buffer emptied now and when the test ends before the env's row cleanups run, and the
// batcher's state restored. No batcher goroutine runs: a test flushes with memberDeleteTestFlush and
// runs the worker with pass.
func newSelfDestructEnv(t *testing.T, delay int) *memberDeletionEnv {
	t.Helper()
	env := newMemberDeletionEnv(t)
	drainMemberDeleteQueue(t)
	restoreMemberDeleteBatcher(t)
	env.storeMemberRule(deletion.KindMemberSelfDestruct, delay)
	env.primeAdminCache()
	return env
}

// sendMember runs a new message of the env chat through the real dispatcher: from sent it, apply sets
// the content a test wants, and its date is deletionNow(). It returns the message ID.
func (e *memberDeletionEnv) sendMember(from *gotgbot.User, apply func(*gotgbot.Message)) int64 {
	e.t.Helper()
	return e.sendService(from, apply)
}

// sendMemberIn is sendMember for another chat, whose rules and state the test set up itself.
func (e *memberDeletionEnv) sendMemberIn(chat gotgbot.Chat, from *gotgbot.User, apply func(*gotgbot.Message)) int64 {
	e.t.Helper()
	return e.sendServiceTo(chat, from, apply)
}

// sendPinOf runs a pinned_message service message of chat sent by from that names messageID.
func (e *memberDeletionEnv) sendPinOf(chat gotgbot.Chat, from *gotgbot.User, messageID int64) int64 {
	e.t.Helper()
	return e.sendServiceTo(chat, from, func(m *gotgbot.Message) {
		m.PinnedMessage = gotgbot.Message{MessageId: messageID, Chat: chat}
	})
}

// selfDestructRows returns the chat's queue rows of the self-destruct kind, by message ID.
func (e *memberDeletionEnv) selfDestructRows(chatID int64) []models.ScheduledDeletion {
	e.t.Helper()
	var rows []models.ScheduledDeletion
	for _, row := range e.queuedNoWait(chatID) {
		if row.Kind == deletion.KindMemberSelfDestruct {
			rows = append(rows, row)
		}
	}
	return rows
}

// deletionPaceKeys are the two Redis keys of the deletion pacer.
var deletionPaceKeys = []string{"fuku:delete:pace:next", "fuku:delete:pace:block"}

// withRealDeletionPacer installs the production pacer options (100 ms apart, 3 retries, 60 s cap, a
// retry_after second of one second) as deletionPacer for the test, over the env's miniredis, and
// deletes its two keys when the test ends. The fast pacer of the deletion environment is put back by
// the swap's own clean-up.
func (e *memberDeletionEnv) withRealDeletionPacer(t *testing.T) {
	t.Helper()
	mreqSwap(t, &deletionPacer, ratelimit.NewTelegramPacer(ratelimit.TelegramPacerOptions{
		NextKey:    deletionPaceKeys[0],
		BlockKey:   deletionPaceKeys[1],
		Interval:   100 * time.Millisecond,
		MaxRetries: 3,
		MaxWait:    60 * time.Second,
	}))
	t.Cleanup(func() {
		for _, key := range deletionPaceKeys {
			e.redis.Del(key)
		}
	})
}

// paceKeyState returns the value and the remaining life of each deletion pacer key that exists.
func (e *memberDeletionEnv) paceKeyState() map[string]string {
	e.t.Helper()
	state := map[string]string{}
	for _, key := range deletionPaceKeys {
		if !e.redis.Exists(key) {
			continue
		}
		value, err := e.redis.Get(key)
		if err != nil {
			e.t.Fatalf("read %s: %v", key, err)
		}
		state[key] = value + "|" + e.redis.TTL(key).String()
	}
	return state
}

// registerReplyCommand registers a command on the env's dispatcher that answers with one message in
// the chat it was sent in, and returns its name. It is a command any member can use, so a test can
// see whether the bot still answers.
func (e *memberDeletionEnv) registerReplyCommand() string {
	e.t.Helper()
	const name = "selfdestructping"
	e.dispatcher.AddHandler(helpers.NewBotCommand(name, func(b *gotgbot.Bot, ctx *ext.Context) error {
		if _, err := ctx.EffectiveMessage.Reply(b, "pong", nil); err != nil {
			return err
		}
		return ext.EndGroups
	}))
	return name
}

// repliesTo counts the sendMessage calls addressed to chatID.
func (e *memberDeletionEnv) repliesTo(chatID int64) int {
	e.t.Helper()
	count := 0
	for _, call := range e.fake.callsFor("sendMessage") {
		if staffParamInt(call.Params, "chat_id") == chatID {
			count++
		}
	}
	return count
}
