//go:build testtools

package modules

import (
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// "I fixed it" with the AI rule (plan 15-07; D-15 as amended by D-23). Step 5 judges the rule with
// the live record's names and the AI row answers from the verdict cache whatever Fresh says: the
// press is deterministic for unchanged names and makes one call only for names it has no verdict
// for, through the breaker, the gate and the member's account budget.

// mreqAIMuted builds the environment with the AI rule on Mute and the real watcher loaded, and mutes
// a member whose names the stand-in rates 0.97, through the real dispatcher (the member is recorded
// as new, so the first message is judged). It returns the member, their hold and the mute notice.
// The stand-in's default answer is left at 0.97; a test sets the next answer itself.
func mreqAIMuted(t *testing.T) (*memberReqEnv, *mreqAIState, gotgbot.User, *models.ChatRequirementHold, staffSentMessage) {
	t.Helper()
	env, ai := mreqAIWatchEnv(t, models.RequirementActionMute)
	member := mreqAIMember(env, "Casino")
	mreqAICleanup(t, []int64{env.chat.Id}, []int64{member.Id})
	ai.setDefault(mreqAIProbability(0.97))
	mreqAIMarkNew(t, env.chat.Id, member.Id)
	notice := env.mreqMuteByMessage(member)
	hold := env.mreqHold(member.Id)
	if hold == nil || hold.Rules != mreqBitOf(t, "ai") || hold.PunishedAt == nil || hold.Action != models.RequirementActionMute {
		t.Fatalf("hold after the mute = %+v, want a punished mute hold with the AI bit", hold)
	}
	if n := ai.count(); n != 1 {
		t.Fatalf("provider requests after the mute = %d, want 1", n)
	}
	return env, ai, member, hold, notice
}

func TestMemberReqFixedAI(t *testing.T) {
	t.Run("frees a renamed member with one call for the new names", func(t *testing.T) {
		env, ai, member, _, notice := mreqAIMuted(t)
		ordinary := mreqAIRename(env, member, "")
		ordinary.FirstName = "Maria" + member.FirstName[len("Promo"):]
		ordinary.Username = "maria" + member.Username[len("promo"):]
		env.fake.setProfile(member.Id, ordinary.FirstName, ordinary.LastName, ordinary.Username)
		ai.setDefault(mreqAIProbability(0.05))
		requestsBefore := ai.count()
		restrictsBefore, answersBefore := len(env.mreqRestricts(member.Id)), len(env.mreqAnswers())

		env.mreqPressFixed(ordinary, notice)

		mreqWantFixedUnmute(t, env, ordinary, notice, restrictsBefore, answersBefore, lockdownSnapshotPermissions(t),
			mreqAnswer{Text: staffMarker("mreq_fixed_done")},
			mreqFixedNoticeText(t, env.translator(), "mreq_notice_fixed", ordinary))
		if n := ai.count() - requestsBefore; n != 1 {
			t.Errorf("provider requests for the press = %d, want exactly 1 for the new names", n)
		}
		mreqAIWantBaseline(t, member.Id, mreqAIHashOfUser(t, ordinary))
		mreqAIWantMarker(t, env.chat.Id, member.Id, false)

		starts := mreqCountJobStarts(t)
		env.mreqSend(ordinary, "hello again")
		memberReqJobs.wait()
		if n := starts(); n != 0 {
			t.Errorf("job starts for the freed member's next message = %d, want none", n)
		}
	})
}
