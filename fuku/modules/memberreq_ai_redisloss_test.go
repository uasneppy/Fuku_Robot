//go:build testtools

package modules

import (
	"context"
	"testing"
	"time"

	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// Phase 15 review WR-01. A hold that carries bit 128 is the durable proof that the member's names
// were judged spam, so a baseline-only pass (the rule's step for an established member it has never
// seen) must never release it or free a mute. When Redis has lost the verdict, the baseline and the
// marker, the held release and "I fixed it" must answer from the cache or from one real call.

// mreqAIFlushState deletes every AI key of the member and of the verdict cache, as a Redis flush, an
// eviction or thirty silent days do.
func mreqAIFlushState(t *testing.T, chatID, userID int64) {
	t.Helper()
	mreqAIDeleteVerdicts()
	mreqAIDeleteBaselines([]int64{userID})
	client := memberReqRedisClient()
	if client == nil {
		t.Fatal("no Redis client")
	}
	if err := client.Del(context.Background(), memberReqAIMarkerKey(chatID, userID)).Err(); err != nil {
		t.Fatalf("delete the marker: %v", err)
	}
}

func TestMemberReqAIHeldRedisLoss(t *testing.T) {
	t.Run("unchanged spam names keep the hold after Redis lost every AI key, with one request", func(t *testing.T) {
		env, ai := mreqAIWatchEnv(t, models.RequirementActionWarn)
		ai.setDefault(mreqAIProbability(0.97))
		member := mreqAIMember(env, "Casino")
		mreqSeedHeld(t, env, member, mreqBitOf(t, "ai"), models.RequirementActionWarn, 10*time.Minute)
		mreqAIFlushState(t, env.chat.Id, member.Id)

		got := env.mreqEnforce(env.mreqCase(member))

		if got.Outcome != memberReqHeld {
			t.Errorf("outcome = %+v, want Held", got)
		}
		if n := ai.count(); n != 1 {
			t.Errorf("provider requests = %d, want 1: no baseline step may stand in for a judgement", n)
		}
		if env.mreqHold(member.Id) == nil {
			t.Error("the hold was released although the names are still judged spam")
		}
		mreqAIWantNoBaseline(t, member.Id)
		mreqAIWantMarker(t, env.chat.Id, member.Id, true)
		mreqHeldNoPunishment(t, env, member)
	})

	t.Run("a changed ordinary name is released with one request", func(t *testing.T) {
		env, ai := mreqAIWatchEnv(t, models.RequirementActionWarn)
		member := mreqAIMember(env, "Casino")
		mreqSeedHeld(t, env, member, mreqBitOf(t, "ai"), models.RequirementActionWarn, 10*time.Minute)
		mreqAIFlushState(t, env.chat.Id, member.Id)
		renamed := mreqAIOrdinary(env)
		env.fake.setProfile(member.Id, renamed.FirstName, renamed.LastName, renamed.Username)
		member.FirstName, member.LastName, member.Username = renamed.FirstName, renamed.LastName, renamed.Username

		got := env.mreqEnforce(env.mreqCase(member))

		if got.Outcome != memberReqNone || got.Hold != nil {
			t.Errorf("outcome = %+v, want the empty result of a release", got)
		}
		if n := ai.count(); n != 1 {
			t.Errorf("provider requests = %d, want 1 for the new names", n)
		}
		if hold := env.mreqHold(member.Id); hold != nil {
			t.Errorf("hold = %+v, want it released", hold)
		}
		mreqAIWantBaseline(t, member.Id, mreqAIHashOfUser(t, member))
	})
}

func TestMemberReqAIFixedRedisLoss(t *testing.T) {
	t.Run("unchanged spam names keep the mute after Redis lost every AI key, with one request", func(t *testing.T) {
		env, ai, member, hold, notice := mreqAIMuted(t)
		mreqAIFlushState(t, env.chat.Id, member.Id)
		requestsBefore := ai.count()

		mreqPressRefused(t, env, member, member, notice.MessageID, env.mreqFixedData(notice), "mreq_fixed_still_broken", staffMarker("mreq_rule_ai"))

		if n := ai.count() - requestsBefore; n != 1 {
			t.Errorf("provider requests for the press = %d, want 1: no baseline step may free a mute", n)
		}
		mreqWantHeld(t, env, hold)
		if live := mreqLiveRecord(t, env, member.Id); !isMemberReqMute(live, hold.MuteUntil) {
			t.Errorf("the member after the refusal = %+v, want still muted with the marker", live)
		}
		mreqAIWantNoBaseline(t, member.Id)
	})

	t.Run("a changed ordinary name is freed with one request", func(t *testing.T) {
		env, ai, member, _, notice := mreqAIMuted(t)
		mreqAIFlushState(t, env.chat.Id, member.Id)
		ordinary := mreqAIOrdinaryRename(env, member)
		ai.setDefault(mreqAIProbability(0.05))
		requestsBefore := ai.count()
		restrictsBefore, answersBefore := len(env.mreqRestricts(member.Id)), len(env.mreqAnswers())

		env.mreqPressFixed(ordinary, notice)

		mreqWantFixedUnmute(t, env, ordinary, notice, restrictsBefore, answersBefore, lockdownSnapshotPermissions(t),
			mreqAnswer{Text: staffMarker("mreq_fixed_done")},
			mreqFixedNoticeText(t, env.translator(), "mreq_notice_fixed", ordinary))
		if n := ai.count() - requestsBefore; n != 1 {
			t.Errorf("provider requests for the press = %d, want 1 for the new names", n)
		}
	})
}
