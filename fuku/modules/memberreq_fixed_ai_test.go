//go:build testtools

package modules

import (
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/config"
	"github.com/uasneppy/fuku_robot/fuku/db/lang"
	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// "I fixed it" with the AI rule (plan 15-07; D-15 as amended by D-23). Step 5 judges the rule with
// the live record's names and the AI row answers from the verdict cache whatever Fresh says: the
// press is deterministic for unchanged names and makes one call only for names it has no verdict
// for, through the breaker, the gate and the member's account budget.

// mreqAIMuteMember mutes a member whose names the stand-in rates 0.97, through the real dispatcher
// (the member is recorded as new, so the first message is judged), in an environment whose rules
// are stored and whose watcher is loaded. last is the member's last name (blank breaks the last
// name rule when it is on). wantBits is the broken mask the hold must carry. The stand-in's default
// answer is left at 0.97; a test sets the next answer itself.
func mreqAIMuteMember(t *testing.T, env *memberReqEnv, ai *mreqAIState, last string, wantBits int) (gotgbot.User, *models.ChatRequirementHold, staffSentMessage) {
	t.Helper()
	member := mreqAIMember(env, last)
	mreqAICleanup(t, []int64{env.chat.Id}, []int64{member.Id})
	ai.setDefault(mreqAIProbability(0.97))
	mreqAIMarkNew(t, env.chat.Id, member.Id)
	notice := env.mreqMuteByMessage(member)
	hold := env.mreqHold(member.Id)
	if hold == nil || hold.Rules != wantBits || hold.PunishedAt == nil || hold.Action != models.RequirementActionMute {
		t.Fatalf("hold after the mute = %+v, want a punished mute hold with the mask %d", hold, wantBits)
	}
	return member, hold, notice
}

// mreqAIMuted builds the environment with the AI rule on Mute and the real watcher loaded, and mutes
// a member whose spam-like names carry no other broken rule. It returns the member, their hold and
// the mute notice; the stand-in has answered exactly one request.
func mreqAIMuted(t *testing.T) (*memberReqEnv, *mreqAIState, gotgbot.User, *models.ChatRequirementHold, staffSentMessage) {
	t.Helper()
	env, ai := mreqAIWatchEnv(t, models.RequirementActionMute)
	member, hold, notice := mreqAIMuteMember(t, env, ai, "Casino", mreqBitOf(t, "ai"))
	if n := ai.count(); n != 1 {
		t.Fatalf("provider requests after the mute = %d, want 1", n)
	}
	return env, ai, member, hold, notice
}

// mreqAIOrdinaryRename gives the member ordinary names that carry its user ID (so the verdict key
// is the test's own), with no last name, in the fake's live record and in the returned user.
func mreqAIOrdinaryRename(env *memberReqEnv, member gotgbot.User) gotgbot.User {
	env.t.Helper()
	member.FirstName = fmt.Sprintf("Maria%d", member.Id)
	member.LastName = ""
	member.Username = fmt.Sprintf("maria%d", member.Id)
	env.fake.setProfile(member.Id, member.FirstName, member.LastName, member.Username)
	return member
}

// mreqAIWantFreed fails unless the press freed the member with the done toast and edited the notice,
// with the group's permissions and no provider request beyond requestsBefore.
func mreqAIWantFreed(t *testing.T, env *memberReqEnv, ai *mreqAIState, member gotgbot.User, notice staffSentMessage, requestsBefore int, press func()) {
	t.Helper()
	restrictsBefore, answersBefore := len(env.mreqRestricts(member.Id)), len(env.mreqAnswers())
	press()
	mreqWantFixedUnmute(t, env, member, notice, restrictsBefore, answersBefore, lockdownSnapshotPermissions(t),
		mreqAnswer{Text: staffMarker("mreq_fixed_done")},
		mreqFixedNoticeText(t, env.translator(), "mreq_notice_fixed", member))
	if n := ai.count() - requestsBefore; n != 0 {
		t.Errorf("provider requests for the press = %d, want none", n)
	}
}

// mreqAIPressAnswer is the text of the last answer.
func mreqAIPressAnswer(env *memberReqEnv) string {
	answers := env.mreqAnswers()
	return answers[len(answers)-1].Text
}

func TestMemberReqFixedAI(t *testing.T) {
	t.Run("frees a renamed member with one call for the new names", func(t *testing.T) {
		env, ai, member, _, notice := mreqAIMuted(t)
		ordinary := mreqAIOrdinaryRename(env, member)
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

	t.Run("unchanged names with a cached breach are still broken, with no request and nothing changed", func(t *testing.T) {
		env, ai, member, hold, notice := mreqAIMuted(t)
		requestsBefore := ai.count()

		mreqPressRefused(t, env, member, member, notice.MessageID, env.mreqFixedData(notice), "mreq_fixed_still_broken", staffMarker("mreq_rule_ai"))

		if n := ai.count() - requestsBefore; n != 0 {
			t.Errorf("provider requests for the press = %d, want none: the cached verdict answers for unchanged names (D-23)", n)
		}
		mreqWantHeld(t, env, hold)
		if live := mreqLiveRecord(t, env, member.Id); !isMemberReqMute(live, hold.MuteUntil) {
			t.Errorf("the member after the refusal = %+v, want still muted with the marker", live)
		}
		mreqAIWantMarker(t, env.chat.Id, member.Id, true)

		// The press is deterministic: ten more taps are ten more refusals and no request.
		for i := 0; i < 10; i++ {
			env.mreqPressFixed(member, notice)
		}
		if n := ai.count() - requestsBefore; n != 0 {
			t.Errorf("provider requests after eleven presses = %d, want none", n)
		}
		mreqWantHeld(t, env, hold)
	})

	t.Run("a member renamed to another spam-like string is judged once and still broken", func(t *testing.T) {
		env, ai, member, hold, notice := mreqAIMuted(t)
		again := member
		again.FirstName, again.Username = fmt.Sprintf("Casino%d", member.Id), fmt.Sprintf("casino%d", member.Id)
		env.fake.setProfile(member.Id, again.FirstName, again.LastName, again.Username)
		requestsBefore := ai.count()

		mreqPressRefused(t, env, again, again, notice.MessageID, env.mreqFixedData(notice), "mreq_fixed_still_broken", staffMarker("mreq_rule_ai"))
		env.mreqPressFixed(again, notice)

		if n := ai.count() - requestsBefore; n != 1 {
			t.Errorf("provider requests for two presses = %d, want 1 (the second reads the verdict the first cached)", n)
		}
		mreqWantHeld(t, env, hold)
	})

	t.Run("an answer the AI cannot give changes nothing", mreqFixedAIUnknown)
	t.Run("approval, configuration and ownership", mreqFixedAIExemptions)
	t.Run("several rules at once", mreqFixedAISeveral)
}

// mreqAIAwaitHungFlight waits until the abandoned flight of a request that outlived the press's
// budget has ended: the stand-in has no request in flight, and the gate slot the flight held is
// taken and given back, which orders the flight's last reads of the swapped settings before the
// cleanups that put them back (the race detector cannot see a sleep).
func mreqAIAwaitHungFlight(ai *mreqAIState) {
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		ai.mu.Lock()
		idle := ai.inflight == 0
		ai.mu.Unlock()
		if idle {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond)
	for time.Now().Before(deadline) {
		if memberReqAIGate.TryAcquire(1) {
			memberReqAIGate.Release(1)
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// mreqAIPickShapes returns the shapes with the given names, in that order.
func mreqAIPickShapes(t *testing.T, shapes []mreqAIShape, names ...string) []mreqAIShape {
	t.Helper()
	var picked []mreqAIShape
	for _, name := range names {
		found := false
		for _, shape := range shapes {
			if shape.name == name {
				picked, found = append(picked, shape), true
				break
			}
		}
		if !found {
			t.Fatalf("no unanswerable shape named %q", name)
		}
	}
	return picked
}

// mreqFixedAIUnknown runs every shape the provider's answer or the guards can take: with nothing
// else broken the press is check failed and changes nothing, and with the last name rule definitely
// broken it names only that rule.
func mreqFixedAIUnknown(t *testing.T) {
	var shapes []mreqAIShape
	for _, shape := range mreqAIShapes() {
		if !shape.stops {
			shapes = append(shapes, shape)
		}
	}
	shapes = append(shapes, mreqAIStateShapes()...)

	// arm puts the stand-in and the guards in the state of the shape, after the member was muted.
	arm := func(t *testing.T, shape mreqAIShape, ai *mreqAIState, chatID int64) {
		t.Helper()
		ai.setDefault(shape.step)
		if shape.hangs {
			mreqSwap(t, &memberReqFixedLookupBudget, 300*time.Millisecond)
			mreqSwap(t, &memberReqLookupFlightTimeout, 600*time.Millisecond)
			// Runs before the swaps are put back: the abandoned flight has ended by its own timeout.
			t.Cleanup(func() { mreqAIAwaitHungFlight(ai) })
		}
		if shape.setup != nil {
			shape.setup(t, chatID)
		}
	}

	for _, shape := range shapes {
		t.Run("nothing else broken: "+shape.name, func(t *testing.T) {
			env, ai, member, hold, notice := mreqAIMuted(t)
			renamed := mreqAIOrdinaryRename(env, member)
			arm(t, shape, ai, env.chat.Id)
			started := time.Now()

			mreqPressRefused(t, env, renamed, renamed, notice.MessageID, env.mreqFixedData(notice), "mreq_fixed_check_failed")

			if took := time.Since(started); took >= memberReqFixedTimeout {
				t.Errorf("the press took %s, want it inside %s", took, memberReqFixedTimeout)
			}
			mreqWantHeld(t, env, hold)
			if live := mreqLiveRecord(t, env, member.Id); !isMemberReqMute(live, hold.MuteUntil) {
				t.Errorf("the member after the refusal = %+v, want still muted with the marker", live)
			}
		})
	}

	t.Run("the last name rule definitely broken beats an unknown AI answer", func(t *testing.T) {
		for _, shape := range mreqAIPickShapes(t, shapes, "a 500", "a request that outlives the budget", "a paused group breaker") {
			t.Run(shape.name, func(t *testing.T) {
				env, ai := mreqAIEnvOf(t, map[memberreq.Rule]int{
					memberreq.RuleLastName: models.RequirementActionMute,
					memberreq.RuleAI:       models.RequirementActionMute,
				}, false)
				env.loadMemberReq()
				member, hold, notice := mreqAIMuteMember(t, env, ai, "", mreqRuleMask(t, "ln", "ai"))
				// The first name changes (no verdict for the new names) and the last name stays blank.
				renamed := mreqAIOrdinaryRename(env, member)
				arm(t, shape, ai, env.chat.Id)

				mreqPressRefused(t, env, renamed, renamed, notice.MessageID, env.mreqFixedData(notice), "mreq_fixed_still_broken", staffMarker("mreq_rule_lastname"))

				if answer := mreqAIPressAnswer(env); strings.Contains(answer, staffMarker("mreq_rule_ai")) {
					t.Errorf("answer %q names the AI rule, whose check could not be answered", answer)
				}
				mreqWantHeld(t, env, hold)
			})
		}
	})
}

// mreqFixedAIExemptions covers the cases where the AI is never asked or never decides: an approved
// member, an AI that is not configured, and a mute the requirements did not place.
func mreqFixedAIExemptions(t *testing.T) {
	t.Run("an approved member is freed at the tap with no request", func(t *testing.T) {
		env, ai, member, _, notice := mreqAIMuted(t)
		mreqApprove(t, env, member)
		requestsBefore := ai.count()

		mreqAIWantFreed(t, env, ai, member, notice, requestsBefore, func() { env.mreqPressFixed(member, notice) })
	})

	t.Run("an AI that is not configured is inert and the member is freed", func(t *testing.T) {
		env, ai, member, _, notice := mreqAIMuted(t)
		previous := config.AppConfig.EnableAISpam
		config.AppConfig.EnableAISpam = false
		t.Cleanup(func() { config.AppConfig.EnableAISpam = previous })
		requestsBefore := ai.count()

		mreqAIWantFreed(t, env, ai, member, notice, requestsBefore, func() { env.mreqPressFixed(member, notice) })
	})

	t.Run("a mute that is not the requirements' is never freed and the provider is not asked", func(t *testing.T) {
		env, ai, member, hold, notice := mreqAIMuted(t)
		if _, err := env.bot.RestrictChatMember(env.chat.Id, member.Id, MutedPermissions, nil); err != nil {
			t.Fatal(err)
		}
		requestsBefore := ai.count()

		mreqPressRefused(t, env, member, member, notice.MessageID, env.mreqFixedData(notice), "mreq_fixed_not_ours")

		if n := ai.count() - requestsBefore; n != 0 {
			t.Errorf("provider requests for the press = %d, want none for a mute that is not ours", n)
		}
		mreqWantHeld(t, env, hold)
	})

	t.Run("the answer is in the group's language", func(t *testing.T) {
		withAIConfigured(t)
		ai := mreqInstallAIFake(t)
		mreqAICleanup(t, nil, nil)
		env := newMemberReqEnvWith(t, settingsEnvOptions{RealLocale: true})
		if err := lang.ChangeGroupLanguage(env.chat.Id, "uk"); err != nil {
			t.Fatal(err)
		}
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleAI: models.RequirementActionMute})
		env.loadMemberReq()
		member, hold, notice := mreqAIMuteMember(t, env, ai, "Casino", mreqBitOf(t, "ai"))
		answersBefore := len(env.mreqAnswers())

		env.mreqPressFixed(member, notice)

		uk := i18n.MustNewTranslator("uk")
		want, err := uk.GetString("mreq_fixed_still_broken", i18n.TranslationParams{"rules": memberReqRulesText(uk, mreqBitOf(t, "ai"))})
		if err != nil {
			t.Fatal(err)
		}
		if got := env.mreqOnlyAnswer(answersBefore); got != (mreqAnswer{Text: want, Alert: true}) {
			t.Errorf("answer = %+v, want the Ukrainian still-broken alert %q", got, want)
		}
		mreqWantHeld(t, env, hold)
	})
}

// mreqAILookupEnv is the lookup environment (photo and channel fakes) with the AI check configured
// and the stand-in installed, the rules stored and the watcher loaded.
func mreqAILookupEnv(t *testing.T, rules map[memberreq.Rule]int) (*memberReqEnv, *mreqLookupState, *mreqAIState) {
	t.Helper()
	withAIConfigured(t)
	ai := mreqInstallAIFake(t)
	mreqAICleanup(t, nil, nil)
	env, state := mreqWatchLookupEnv(t)
	env.mreqRules(rules)
	return env, state, ai
}

// mreqFixedAISeveral covers a hold placed by another rule while the AI rule has a cached pass, and the
// count answer that now runs to eight rules.
func mreqFixedAISeveral(t *testing.T) {
	t.Run("names unchanged with a cached pass: no request and the AI part is clear", func(t *testing.T) {
		env, state, ai := mreqAILookupEnv(t, map[memberreq.Rule]int{
			memberreq.RulePhoto: models.RequirementActionMute,
			memberreq.RuleAI:    models.RequirementActionMute,
		})
		member := mreqAIOrdinary(env)
		mreqAICleanup(t, []int64{env.chat.Id}, []int64{member.Id})
		mreqLookupCleanup(t, []int64{member.Id}, nil)
		state.set(member.Id, mreqPhotoAnswer{})
		mreqAIPlant(t, env, mreqAIKeyOf(t, mreqAINamesOfUser(member)), "0.2")
		notice := env.mreqMuteByMessage(member)
		if hold := env.mreqHold(member.Id); hold == nil || hold.Rules != mreqBitOf(t, "ph") {
			t.Fatalf("hold = %+v, want a hold with the photo bit only", hold)
		}
		state.set(member.Id, mreqHasPhoto)
		requestsBefore := ai.count()

		mreqAIWantFreed(t, env, ai, member, notice, requestsBefore, func() { env.mreqPressFixed(member, notice) })
		if requestsBefore != 0 {
			t.Errorf("provider requests before the press = %d, want none: the cached pass served the job too", requestsBefore)
		}
	})

	t.Run("all eight rules broken answer with the count, in at most 160 code points", func(t *testing.T) {
		env, state, ai := mreqAILookupEnv(t, map[memberreq.Rule]int{
			memberreq.RuleLastName: models.RequirementActionMute,
			memberreq.RuleUsername: models.RequirementActionMute,
			memberreq.RuleRussian:  models.RequirementActionMute,
			memberreq.RuleArabic:   models.RequirementActionMute,
			memberreq.RuleChinese:  models.RequirementActionMute,
			memberreq.RulePhoto:    models.RequirementActionMute,
			memberreq.RuleChannel:  models.RequirementActionMute,
			memberreq.RuleAI:       models.RequirementActionMute,
		})
		channel := mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
		member := env.mreqMember("x", "", "")
		mreqLookupCleanup(t, []int64{member.Id}, nil)
		member.FirstName = fmt.Sprintf("Ыا中%d", member.Id)
		env.fake.setProfile(member.Id, member.FirstName, "", "")
		mreqAICleanup(t, []int64{env.chat.Id}, []int64{member.Id})
		state.set(member.Id, mreqPhotoAnswer{})
		state.setMember(channel, member.Id, mreqChannelMember{Status: gotgbot.ChatMemberStatusLeft})
		mreqAIPlant(t, env, mreqAIKeyOf(t, mreqAINamesOfUser(member)), "0.97")
		notice := env.mreqMuteByMessage(member)
		hold := env.mreqHold(member.Id)
		if hold == nil || hold.Rules != mreqAllRulesMask() {
			t.Fatalf("hold = %+v, want a hold with all eight bits", hold)
		}
		requestsBefore := ai.count()

		mreqPressRefused(t, env, member, member, notice.MessageID, env.mreqFixedData(notice), "mreq_fixed_still_broken_many", " 8")

		answer := mreqAIPressAnswer(env)
		if got := utf8.RuneCountInString(answer); got > 160 {
			t.Errorf("count answer %q has %d code points, want at most 160 in English", answer, got)
		}
		for _, rule := range memberReqRules {
			if strings.Contains(answer, staffMarker(rule.PhraseKey)) {
				t.Errorf("count answer %q names %s", answer, rule.PhraseKey)
			}
		}
		if n := ai.count() - requestsBefore; n != 0 {
			t.Errorf("provider requests for the press = %d, want none (the cached breach answers)", n)
		}
		mreqWantHeld(t, env, hold)
	})
}
