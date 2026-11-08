//go:build testtools

package modules

import (
	"errors"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// mreqNoPhotoMember mints a member whose names break nothing and who has no profile photo, with
// the photo rule set to action. The photo lookup is the slow step the tests act during.
func mreqNoPhotoMember(t *testing.T, env *memberReqEnv, state *mreqLookupState, action int) gotgbot.User {
	t.Helper()
	env.mreqRules(map[memberreq.Rule]int{memberreq.RulePhoto: action})
	return mreqLookupMember(t, env, state, mreqPhotoAnswer{})
}

// TestMemberReqLiveRecordAfterLookups is WR-03: the live record that gates a punishment is read
// again after a lookup phase that made a request, so a member who became an administrator or was
// restricted by an admin meanwhile is judged as they are now.
func TestMemberReqLiveRecordAfterLookups(t *testing.T) {
	t.Run("an administrator promoted during the lookup phase is never punished", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			action int
			write  string
		}{
			{"Ban", models.RequirementActionBan, "banChatMember"},
			{"Kick", models.RequirementActionKick, "unbanChatMember"},
			{"Mute", models.RequirementActionMute, "restrictChatMember"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				env, state := mreqLookupEnv(t)
				member := mreqNoPhotoMember(t, env, state, tc.action)
				state.setOnPhoto(func(user int64) {
					if user == member.Id {
						env.fake.setMember(env.chat.Id, member.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusAdministrator, IsMember: true})
					}
				})

				mreqWantNothing(t, env, member, env.mreqEnforce(env.mreqCase(member)))
				if n := env.mreqCallCount(tc.write); n != 0 {
					t.Errorf("%s calls = %d, want none", tc.write, n)
				}
			})
		}
	})

	t.Run("a member an admin restricted during the lookup phase is not muted over", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		member := mreqNoPhotoMember(t, env, state, models.RequirementActionMute)
		state.setOnPhoto(func(user int64) {
			if user == member.Id {
				env.mreqRestrictedBySomeoneElse(member, 0)
			}
		})

		mreqWantNothing(t, env, member, env.mreqEnforce(env.mreqCase(member)))
		if n := env.mreqCallCount("restrictChatMember"); n != 0 {
			t.Errorf("restrict calls = %d, want none: the admin's restriction must stand", n)
		}
	})

	t.Run("a live record that cannot be read again punishes nobody", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		member := mreqNoPhotoMember(t, env, state, models.RequirementActionBan)
		state.setOnPhoto(func(user int64) {
			if user == member.Id {
				env.fake.script("getChatMember", env.chat.Id, errors.New("connection reset by peer"))
			}
		})

		mreqWantNothing(t, env, member, env.mreqEnforce(env.mreqCase(member)))
	})

	t.Run("a lookup that made a request costs one more live record and a breach still punishes", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		member := mreqNoPhotoMember(t, env, state, models.RequirementActionBan)

		got := env.mreqEnforce(env.mreqCase(member))
		if got.Outcome != memberReqRemoved || got.Hold == nil {
			t.Fatalf("outcome = %+v, want Removed", got)
		}
		if n := mreqMemberCalls(env, env.chat.Id, member.Id); n != 2 {
			t.Errorf("live getChatMember requests for the member = %d, want 2 (before and after the lookup phase)", n)
		}
	})

	t.Run("a lookup phase that was all cache hits and fast costs no extra request", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		env.mreqRules(map[memberreq.Rule]int{
			memberreq.RuleLastName: models.RequirementActionMute,
			memberreq.RulePhoto:    models.RequirementActionBan,
		})
		member := env.mreqMember("Alex", "", "alexsmith")
		mreqLookupCleanup(t, []int64{member.Id}, nil)
		state.set(member.Id, mreqHasPhoto)
		mreqSetKey(t, env, memberReqPhotoKey(member.Id), memberReqPositiveTTL)

		got := env.mreqEnforce(env.mreqCase(member))
		if got.Outcome != memberReqMuted {
			t.Fatalf("outcome = %+v, want Muted by the name rule", got)
		}
		if n := mreqMemberCalls(env, env.chat.Id, member.Id); n != 1 {
			t.Errorf("live getChatMember requests for the member = %d, want 1: a cached lookup phase needs no second read", n)
		}
		if n := mreqLookupPhotoCalls(env); n != 0 {
			t.Errorf("getUserProfilePhotos requests = %d, want none (the cached pass)", n)
		}
	})
}
