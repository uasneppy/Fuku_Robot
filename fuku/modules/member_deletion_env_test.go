//go:build testtools

package modules

import (
	"context"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
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

// coldAdminCache empties the chat's admin cache entry, so the cache cannot say who is an administrator
// and the handler has to ask Telegram.
func (e *memberDeletionEnv) coldAdminCache() {
	e.t.Helper()
	cache.InvalidateAdminCache(e.chat.Id)
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
