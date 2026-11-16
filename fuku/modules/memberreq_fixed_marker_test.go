//go:build testtools

package modules

import (
	"errors"
	"testing"
	"time"
)

// TestMemberReqFixedMarkerRecheck is WR-04: the live marker is proved again right before the hold
// is taken whenever the fresh lookups of step 5 made a request, so an admin's /mute that landed
// during them is never lifted by a press.
func TestMemberReqFixedMarkerRecheck(t *testing.T) {
	t.Run("an admin mute that lands during the fresh lookups is not undone", func(t *testing.T) {
		env, state, member, hold, notice := mreqPhotoMuted(t)
		state.set(member.Id, mreqHasPhoto)
		data := env.mreqFixedData(notice)
		state.setOnPhoto(func(user int64) {
			if user == member.Id {
				env.mreqRestrictedBySomeoneElse(member, time.Now().Add(48*time.Hour).Unix())
			}
		})

		mreqPressRefused(t, env, member, member, notice.MessageID, data, "mreq_fixed_not_ours")
		mreqWantHeld(t, env, hold)
	})

	t.Run("a marker that is still there frees the member after one more live read", func(t *testing.T) {
		env, state, member, _, notice := mreqPhotoMuted(t)
		state.set(member.Id, mreqHasPhoto)
		restrictsBefore, answersBefore := len(env.mreqRestricts(member.Id)), len(env.mreqAnswers())
		liveBefore := env.mreqMemberLookups(member.Id)

		env.mreqPressFixed(member, notice)

		mreqWantFixedUnmute(t, env, member, notice, restrictsBefore, answersBefore, lockdownSnapshotPermissions(t),
			mreqAnswer{Text: staffMarker("mreq_fixed_done")},
			mreqFixedNoticeText(t, env.translator(), "mreq_notice_fixed", member))
		if n := env.mreqMemberLookups(member.Id) - liveBefore; n != 2 {
			t.Errorf("live getChatMember requests of the press = %d, want 2 (the marker check and the one before the take)", n)
		}
	})

	t.Run("a press that made no lookup request costs no extra live read", func(t *testing.T) {
		env := newMemberReqEnv(t)
		member, _, notice := mreqMutedMember(t, env)
		fixed := env.mreqFix(member)
		liveBefore := env.mreqMemberLookups(member.Id)

		env.mreqPressFixed(fixed, notice)

		if n := env.mreqMemberLookups(member.Id) - liveBefore; n != 1 {
			t.Errorf("live getChatMember requests of a name-only press = %d, want 1", n)
		}
		if hold := env.mreqHold(member.Id); hold != nil {
			t.Errorf("hold after the press = %+v, want none", hold)
		}
	})

	t.Run("a live read before the take that cannot be answered changes nothing", func(t *testing.T) {
		env, state, member, hold, notice := mreqPhotoMuted(t)
		state.set(member.Id, mreqHasPhoto)
		data := env.mreqFixedData(notice)
		state.setOnPhoto(func(user int64) {
			if user == member.Id {
				env.fake.script("getChatMember", env.chat.Id, errors.New("connection reset by peer"))
			}
		})

		mreqPressRefused(t, env, member, member, notice.MessageID, data, "mreq_fixed_check_failed")
		mreqWantHeld(t, env, hold)
	})
}
