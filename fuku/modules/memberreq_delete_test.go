//go:build testtools

package modules

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	logrustest "github.com/sirupsen/logrus/hooks/test"

	"github.com/uasneppy/fuku_robot/fuku/db/approvals"
	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
)

// mreqDeleteMessages switches Delete messages on or off for the environment's group through
// the real repository.
func (e *memberReqEnv) mreqDeleteMessages(on bool) {
	e.t.Helper()
	if err := memberreq.SetDeleteMessages(context.Background(), e.chat.Id, on); err != nil {
		e.t.Fatalf("SetDeleteMessages(%t) error = %v", on, err)
	}
}

// mreqDeleted returns the message IDs the bot asked Telegram to delete in the group, in order.
func (e *memberReqEnv) mreqDeleted() []int64 {
	e.t.Helper()
	var ids []int64
	for _, call := range e.calls("deleteMessage") {
		ids = append(ids, staffParamInt(call.Params, "message_id"))
	}
	return ids
}

// mreqAdminList loads the chat's admin list into the process-wide admin cache from the fake's
// getChatAdministrators answer, so the cache knows the list. The cache entry of the chat is
// dropped again when the test ends.
func (e *memberReqEnv) mreqAdminList() {
	e.t.Helper()
	cache.InvalidateAdminCache(e.chat.Id)
	e.t.Cleanup(func() { cache.InvalidateAdminCache(e.chat.Id) })
	if got := cache.LoadAdminCache(e.bot, e.chat.Id); !got.Cached {
		e.t.Fatalf("LoadAdminCache() = %+v, want a cached list", got)
	}
}

// mreqNoAdminList empties the chat's admin cache entry, so the cache cannot say who is an admin.
func (e *memberReqEnv) mreqNoAdminList() {
	e.t.Helper()
	cache.InvalidateAdminCache(e.chat.Id)
	e.t.Cleanup(func() { cache.InvalidateAdminCache(e.chat.Id) })
}

// mreqPromote makes user an administrator (or the creator) in the fake and drops the cached
// admin list, as the promotion's own chat_member update does through adminCacheAutoUpdate.
func (e *memberReqEnv) mreqPromote(user gotgbot.User, status string) {
	e.t.Helper()
	e.fake.setMember(e.chat.Id, user.Id, staffFakeMember{Status: status, CanRestrictMembers: true})
	if status == gotgbot.ChatMemberStatusAdministrator {
		e.listAdmin(e.chat.Id, user.Id)
	}
	cache.InvalidateAdminCache(e.chat.Id)
	e.t.Cleanup(func() { cache.InvalidateAdminCache(e.chat.Id) })
}

// mreqMemberLookups counts the getChatMember requests of the group that name userID.
func (e *memberReqEnv) mreqMemberLookups(userID int64) int {
	e.t.Helper()
	return e.mreqCallsAbout(userID, "getChatMember")
}

// mreqHoldMember puts a hold of a Mute rule on a new member of the group by one enforcement
// whose message is already deleted when Delete messages is on, and returns the member. The
// caller's later cases for the member are then the held path.
func mreqHoldMember(t *testing.T, env *memberReqEnv) gotgbot.User {
	t.Helper()
	member := env.mreqMember("Alex", "", "alex")
	if got := env.mreqEnforce(env.mreqCase(member)); got.Outcome != memberReqMuted {
		t.Fatalf("first enforcement = %+v, want Muted", got)
	}
	return member
}

func TestMemberReqDeleteMessages(t *testing.T) {
	t.Run("a violator's message is deleted with the warn and the later ones with nothing else", func(t *testing.T) {
		env := newMemberReqEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionWarn})
		env.mreqDeleteMessages(true)
		env.mreqAdminList()
		member := env.mreqMember("Alex", "", "alex")

		first := env.mreqCase(member)
		if got := env.mreqEnforce(first); got.Outcome != memberReqWarned {
			t.Fatalf("first enforcement = %+v, want Warned", got)
		}
		if deleted := env.mreqDeleted(); len(deleted) != 1 || deleted[0] != first.MessageID {
			t.Fatalf("deleted after the first message = %v, want only %d", deleted, first.MessageID)
		}
		if row := env.mreqWarnRow(member.Id); row == nil || row.NumWarns != 1 {
			t.Fatalf("warns row = %+v, want one warn", row)
		}
		notices, lookups := len(env.mreqNotices()), env.mreqMemberLookups(member.Id)

		want := []int64{first.MessageID}
		for i := 0; i < 3; i++ {
			next := env.mreqCase(member)
			if got := env.mreqEnforce(next); got.Outcome != memberReqHeld {
				t.Fatalf("later enforcement %d = %+v, want Held", i, got)
			}
			want = append(want, next.MessageID)
		}
		if deleted := env.mreqDeleted(); fmt.Sprint(deleted) != fmt.Sprint(want) {
			t.Errorf("deleted = %v, want every message of the member %v", deleted, want)
		}
		if row := env.mreqWarnRow(member.Id); row == nil || row.NumWarns != 1 {
			t.Errorf("warns row after held messages = %+v, want still one warn", row)
		}
		if after := len(env.mreqNotices()); after != notices {
			t.Errorf("held messages posted %d messages, want none", after-notices)
		}
		if writes := env.mreqWrites(); len(writes) != 0 {
			t.Errorf("held messages made %d restrict, ban or unban calls, want none", len(writes))
		}
		if after := env.mreqMemberLookups(member.Id); after != lookups {
			t.Errorf("held messages made %d getChatMember calls, want none while the cached list knows the member", after-lookups)
		}
	})

	t.Run("with Delete messages off nothing is deleted", func(t *testing.T) {
		env := newMemberReqEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionMute})
		env.mreqAdminList()
		member := mreqHoldMember(t, env)
		if got := env.mreqEnforce(env.mreqCase(member)); got.Outcome != memberReqHeld {
			t.Fatalf("second enforcement = %+v, want Held", got)
		}
		if deleted := env.mreqDeleted(); len(deleted) != 0 {
			t.Errorf("deleted = %v, want nothing with Delete messages off", deleted)
		}
	})

	t.Run("a refused delete is logged by IDs and changes nothing else", func(t *testing.T) {
		hook := logrustest.NewGlobal()
		t.Cleanup(hook.Reset)
		env := newMemberReqEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionWarn})
		env.mreqDeleteMessages(true)
		member := env.mreqMember("Alex", "", "alex")
		env.fake.script("deleteMessage", env.chat.Id, staffFakeError(400, "Bad Request: message can't be deleted"))

		c := env.mreqCase(member)
		if got := env.mreqEnforce(c); got.Outcome != memberReqWarned {
			t.Fatalf("enforcement = %+v, want Warned", got)
		}
		if hold := env.mreqHold(member.Id); hold == nil || hold.PunishedAt == nil || hold.NoticeMsgID == 0 {
			t.Errorf("hold = %+v, want a punished hold with its notice stored", hold)
		}
		if row := env.mreqWarnRow(member.Id); row == nil || row.NumWarns != 1 {
			t.Errorf("warns row = %+v, want one warn", row)
		}
		if n := len(env.mreqNotices()); n != 1 {
			t.Errorf("messages posted = %d, want the one warn notice", n)
		}
		var logged string
		for _, line := range mreqWarnLines(hook) {
			if strings.Contains(line, "deleting message") {
				logged = line
			}
		}
		for _, id := range []int64{env.chat.Id, member.Id, c.MessageID} {
			if !strings.Contains(logged, fmt.Sprint(id)) {
				t.Errorf("delete failure log %q lacks the ID %d", logged, id)
			}
		}
		if logged == "" {
			t.Error("the failed delete was not logged")
		}
		if strings.Contains(logged, "Bad Request") {
			t.Errorf("delete failure log %q carries Telegram's wording, want the code only", logged)
		}
	})

	t.Run("the message is deleted when the punishment is refused or unknown", func(t *testing.T) {
		cases := []struct {
			name   string
			action int
			method string
			err    error
		}{
			{"a mute refused", models.RequirementActionMute, "restrictChatMember", staffFakeError(400, "Bad Request: not enough rights")},
			{"a ban whose answer is unknown", models.RequirementActionBan, "banChatMember", staffFakeError(502, "Bad Gateway")},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				env := newMemberReqEnv(t)
				env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: tc.action})
				env.mreqDeleteMessages(true)
				member := env.mreqMember("Alex", "", "alex")
				env.fake.script(tc.method, env.chat.Id, tc.err)

				c := env.mreqCase(member)
				if got := env.mreqEnforce(c); got.Outcome != memberReqNotApplied {
					t.Fatalf("enforcement = %+v, want NotApplied", got)
				}
				if deleted := env.mreqDeleted(); len(deleted) != 1 || deleted[0] != c.MessageID {
					t.Errorf("deleted = %v, want only %d", deleted, c.MessageID)
				}
				if n := len(env.mreqNotices()); n != 0 {
					t.Errorf("messages posted = %d, want none after a punishment that did not take effect", n)
				}
			})
		}
	})

	t.Run("a clean member's message and a join are never deleted", func(t *testing.T) {
		env := newMemberReqEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionKick})
		env.mreqDeleteMessages(true)

		clean := env.mreqMember("Bo", "Li", "bo")
		if got := env.mreqEnforce(env.mreqCase(clean)); got.Outcome != memberReqNone {
			t.Fatalf("clean member = %+v, want None", got)
		}

		joiner := env.mreqMember("Cy", "", "cy")
		c := env.mreqCase(joiner)
		c.Source, c.MessageID, c.MessageDate = memberReqFromJoin, 0, 0
		if got := env.mreqEnforce(c); got.Outcome != memberReqRemoved {
			t.Fatalf("join case = %+v, want Removed", got)
		}
		if deleted := env.mreqDeleted(); len(deleted) != 0 {
			t.Errorf("deleted = %v, want nothing for a clean member and a join", deleted)
		}
	})

	t.Run("a held message is deleted only when the sender is established as a non-admin", func(t *testing.T) {
		newHeld := func(t *testing.T) (*memberReqEnv, gotgbot.User) {
			env := newMemberReqEnv(t)
			env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionMute})
			env.mreqDeleteMessages(true)
			return env, mreqHoldMember(t, env)
		}
		// heldCase enforces one more message of the member and returns what happened to it.
		heldCase := func(t *testing.T, env *memberReqEnv, member gotgbot.User) (deleted bool, lookups int) {
			t.Helper()
			before, lookupsBefore := len(env.mreqDeleted()), env.mreqMemberLookups(member.Id)
			if got := env.mreqEnforce(env.mreqCase(member)); got.Outcome != memberReqHeld {
				t.Fatalf("enforcement = %+v, want Held", got)
			}
			return len(env.mreqDeleted()) == before+1, env.mreqMemberLookups(member.Id) - lookupsBefore
		}

		t.Run("a known list that does not name them: deleted, no getChatMember", func(t *testing.T) {
			env, member := newHeld(t)
			env.mreqAdminList()
			if deleted, lookups := heldCase(t, env, member); !deleted || lookups != 0 {
				t.Errorf("deleted = %t with %d getChatMember calls, want deleted with none", deleted, lookups)
			}
		})
		t.Run("an invalidated list: one getChatMember, deleted", func(t *testing.T) {
			env, member := newHeld(t)
			env.mreqNoAdminList()
			if deleted, lookups := heldCase(t, env, member); !deleted || lookups != 1 {
				t.Errorf("deleted = %t with %d getChatMember calls, want deleted with one", deleted, lookups)
			}
		})
		t.Run("promoted to administrator since: one getChatMember, not deleted", func(t *testing.T) {
			env, member := newHeld(t)
			env.mreqPromote(member, gotgbot.ChatMemberStatusAdministrator)
			if deleted, lookups := heldCase(t, env, member); deleted || lookups != 1 {
				t.Errorf("deleted = %t with %d getChatMember calls, want not deleted after one", deleted, lookups)
			}
		})
		t.Run("promoted and the list reloaded: not deleted", func(t *testing.T) {
			env, member := newHeld(t)
			env.mreqPromote(member, gotgbot.ChatMemberStatusAdministrator)
			env.mreqAdminList()
			if deleted, _ := heldCase(t, env, member); deleted {
				t.Error("a member the reloaded list names as an administrator was deleted")
			}
		})
		t.Run("the creator: one getChatMember, not deleted", func(t *testing.T) {
			env, member := newHeld(t)
			env.mreqPromote(member, gotgbot.ChatMemberStatusCreator)
			if deleted, lookups := heldCase(t, env, member); deleted || lookups != 1 {
				t.Errorf("deleted = %t with %d getChatMember calls, want not deleted after one", deleted, lookups)
			}
		})
		t.Run("a failed lookup: not deleted", func(t *testing.T) {
			env, member := newHeld(t)
			env.mreqNoAdminList()
			env.fake.script("getChatMember", env.chat.Id, staffFakeError(500, "Internal Server Error"))
			if deleted, lookups := heldCase(t, env, member); deleted || lookups != 1 {
				t.Errorf("deleted = %t with %d getChatMember calls, want not deleted after the failed one", deleted, lookups)
			}
		})
	})
}

// mreqPause sets the chat's pause latch the way the cap does, for the test.
func (e *memberReqEnv) mreqPause() {
	e.t.Helper()
	key := memberReqPausedKey(e.chat.Id)
	if err := e.redis.Set(key, "1"); err != nil {
		e.t.Fatalf("set the pause latch: %v", err)
	}
	e.redis.SetTTL(key, memberReqPause)
}

// mreqResume removes the chat's pause latch.
func (e *memberReqEnv) mreqResume() {
	e.t.Helper()
	e.redis.Del(memberReqPausedKey(e.chat.Id))
}

func TestMemberReqDeleteDuringPause(t *testing.T) {
	newPaused := func(t *testing.T, deleteOn bool) *memberReqEnv {
		env := newMemberReqEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionMute})
		env.mreqDeleteMessages(deleteOn)
		return env
	}
	// quiet asserts that a case made no hold, no write, no notice and no lookup about the member.
	quiet := func(t *testing.T, env *memberReqEnv, member gotgbot.User, notices int) {
		t.Helper()
		if n := env.mreqCallsAbout(member.Id, "getChatMember", "restrictChatMember", "banChatMember", "unbanChatMember"); n != 0 {
			t.Errorf("%d Telegram calls about the member were made during the pause, want none", n)
		}
		if after := len(env.mreqNotices()); after != notices {
			t.Errorf("%d messages were posted during the pause, want none", after-notices)
		}
	}

	t.Run("a new violator the cached list knows as a non-admin is deleted and nothing else", func(t *testing.T) {
		env := newPaused(t, true)
		env.mreqAdminList()
		env.mreqPause()
		member := env.mreqMember("Alex", "", "alex")
		notices := len(env.mreqNotices())

		c := env.mreqCase(member)
		if got := env.mreqEnforce(c); got.Outcome != memberReqNone {
			t.Fatalf("enforcement = %+v, want None: nothing is punished during a pause", got)
		}
		if deleted := env.mreqDeleted(); len(deleted) != 1 || deleted[0] != c.MessageID {
			t.Errorf("deleted = %v, want only %d", deleted, c.MessageID)
		}
		if env.mreqHold(member.Id) != nil {
			t.Error("a hold was written during the pause")
		}
		quiet(t, env, member, notices)
	})

	t.Run("a held member the cached list knows as a non-admin keeps being deleted", func(t *testing.T) {
		env := newPaused(t, true)
		env.mreqAdminList()
		member := mreqHoldMember(t, env)
		env.mreqPause()
		lookups, notices := env.mreqMemberLookups(member.Id), len(env.mreqNotices())

		c := env.mreqCase(member)
		if got := env.mreqEnforce(c); got.Outcome != memberReqHeld {
			t.Fatalf("enforcement = %+v, want Held", got)
		}
		if deleted := env.mreqDeleted(); len(deleted) != 2 || deleted[1] != c.MessageID {
			t.Errorf("deleted = %v, want the held member's message too", deleted)
		}
		if after := env.mreqMemberLookups(member.Id); after != lookups {
			t.Errorf("%d getChatMember calls during the pause, want none", after-lookups)
		}
		if after := len(env.mreqNotices()); after != notices {
			t.Errorf("%d messages were posted, want none", after-notices)
		}
	})

	t.Run("with the cached list unknown nothing is deleted and no call is made", func(t *testing.T) {
		env := newPaused(t, true)
		env.mreqNoAdminList()
		held := mreqHoldMember(t, env)
		promoted := mreqHoldMember(t, env)
		env.mreqPromote(promoted, gotgbot.ChatMemberStatusAdministrator)
		env.mreqPause()
		fresh := env.mreqMember("Bo", "", "bo")
		deletedBefore, notices := len(env.mreqDeleted()), len(env.mreqNotices())
		lookups := map[int64]int{held.Id: env.mreqMemberLookups(held.Id), promoted.Id: env.mreqMemberLookups(promoted.Id)}

		for name, c := range map[string]memberReqCase{
			"a new violator":          env.mreqCase(fresh),
			"a held member":           env.mreqCase(held),
			"a held member promoted":  env.mreqCase(promoted),
			"a held member, again":    env.mreqCase(held),
			"a promoted member again": env.mreqCase(promoted),
		} {
			env.mreqEnforce(c)
			if after := len(env.mreqDeleted()); after != deletedBefore {
				t.Errorf("%s: a message was deleted while the list is unknown and the chat is paused", name)
			}
		}
		for id, before := range lookups {
			if after := env.mreqMemberLookups(id); after != before {
				t.Errorf("user %d: %d getChatMember calls during the pause, want none", id, after-before)
			}
		}
		quiet(t, env, fresh, notices)
		if env.mreqHold(fresh.Id) != nil {
			t.Error("a hold was written for a new violator during the pause")
		}

		// Once the pause is over the same promoted member is checked live, and still not deleted.
		env.mreqResume()
		before, lookupsBefore := len(env.mreqDeleted()), env.mreqMemberLookups(promoted.Id)
		if got := env.mreqEnforce(env.mreqCase(promoted)); got.Outcome != memberReqHeld {
			t.Fatalf("promoted member after the pause = %+v, want Held", got)
		}
		if after := len(env.mreqDeleted()); after != before {
			t.Error("a promoted administrator's message was deleted after the pause")
		}
		if after := env.mreqMemberLookups(promoted.Id); after != lookupsBefore+1 {
			t.Errorf("getChatMember calls after the pause = %d, want one", after-lookupsBefore)
		}
	})

	t.Run("with Delete messages off nothing is deleted during the pause", func(t *testing.T) {
		env := newPaused(t, false)
		env.mreqAdminList()
		held := mreqHoldMember(t, env)
		env.mreqPause()
		fresh := env.mreqMember("Bo", "", "bo")

		env.mreqEnforce(env.mreqCase(held))
		env.mreqEnforce(env.mreqCase(fresh))
		if deleted := env.mreqDeleted(); len(deleted) != 0 {
			t.Errorf("deleted = %v, want nothing with Delete messages off", deleted)
		}
	})

	t.Run("an approved member is never deleted during the pause", func(t *testing.T) {
		env := newPaused(t, true)
		env.mreqAdminList()
		env.mreqPause()
		member := env.mreqMember("Alex", "", "alex")
		if err := approvals.AddApprovedUser(env.chat.Id, member.Id, env.admin.Id, "trusted"); err != nil {
			t.Fatalf("approve: %v", err)
		}
		env.mreqEnforce(env.mreqCase(member))
		if deleted := env.mreqDeleted(); len(deleted) != 0 {
			t.Errorf("deleted = %v, want nothing for an approved member", deleted)
		}
	})
}
