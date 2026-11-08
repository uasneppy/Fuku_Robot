//go:build testtools

package modules

import (
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// mreqPhotoMuted builds the lookup environment with the real watcher loaded, sets the photo
// rule to Mute and mutes a member whose names break nothing and whose photo is missing, through
// the real dispatcher. It returns the member, their hold and the mute notice, with the lookup
// fake still answering "no photo" for them.
func mreqPhotoMuted(t *testing.T) (*memberReqEnv, *mreqLookupState, gotgbot.User, *models.ChatRequirementHold, staffSentMessage) {
	t.Helper()
	env, state := mreqWatchLookupEnv(t)
	env.mreqRules(map[memberreq.Rule]int{memberreq.RulePhoto: models.RequirementActionMute})
	member := mreqLookupMember(t, env, state, mreqPhotoAnswer{})
	notice := env.mreqMuteByMessage(member)
	hold := env.mreqHold(member.Id)
	if hold == nil || hold.Rules != mreqBitOf(t, "ph") || hold.PunishedAt == nil {
		t.Fatalf("hold after the mute = %+v, want a punished hold with the photo bit", hold)
	}
	return env, state, member, hold, notice
}

func TestMemberReqFixedLookups(t *testing.T) {
	t.Run("a member who added a photo is freed by a fresh lookup that ignores a stale positive", func(t *testing.T) {
		env, state, member, _, notice := mreqPhotoMuted(t)
		state.set(member.Id, mreqHasPhoto)
		// A stale positive from before the member removed their photo: one minute left.
		mreqSetKey(t, env, memberReqPhotoKey(member.Id), time.Minute)
		photoBefore := mreqLookupPhotoCalls(env)
		restrictsBefore, answersBefore := len(env.mreqRestricts(member.Id)), len(env.mreqAnswers())

		env.mreqPressFixed(member, notice)

		mreqWantFixedUnmute(t, env, member, notice, restrictsBefore, answersBefore, lockdownSnapshotPermissions(t),
			mreqAnswer{Text: staffMarker("mreq_fixed_done")},
			mreqFixedNoticeText(t, env.translator(), "mreq_notice_fixed", member))
		if n := mreqLookupPhotoCalls(env); n != photoBefore+1 {
			t.Errorf("getUserProfilePhotos requests = %d, want %d: the press must ask again and not trust the cached positive", n, photoBefore+1)
		}
		if ttl := env.redis.TTL(memberReqPhotoKey(member.Id)); ttl < 10*time.Minute {
			t.Errorf("photo key ttl = %s, want the positive refreshed to %s", ttl, memberReqPositiveTTL)
		}
	})

	t.Run("a member whose photo is still missing is told so and the stale positive is deleted", func(t *testing.T) {
		env, _, member, hold, notice := mreqPhotoMuted(t)
		mreqSetKey(t, env, memberReqPhotoKey(member.Id), memberReqPositiveTTL)
		data := env.mreqFixedData(notice)

		mreqPressRefused(t, env, member, member, notice.MessageID, data, "mreq_fixed_still_broken", staffMarker("mreq_rule_photo"))

		if env.redis.Exists(memberReqPhotoKey(member.Id)) {
			t.Error("the stale photo positive is still cached after a definite breach")
		}
		mreqWantHeld(t, env, hold)
	})
}
