//go:build testtools

package modules

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	logrustest "github.com/sirupsen/logrus/hooks/test"

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
			if strings.Contains(line, "delete") {
				logged = line
			}
		}
		for _, id := range []int64{env.chat.Id, member.Id, c.MessageID} {
			if !strings.Contains(logged, fmt.Sprint(id)) {
				t.Errorf("delete failure log %q lacks the ID %d", logged, id)
			}
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
