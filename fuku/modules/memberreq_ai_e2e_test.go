//go:build testtools

package modules

import (
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// The AI spam-name rule end to end (plan 15-11; MREQ-14, MREQ-10, MREQ-01). Everything is real
// but Telegram and the provider: the settings page, the card, the dispatcher's join and message
// paths, the executor, the hold and the "I fixed it" button run through the registered handlers,
// and the Anthropic stand-in answers over HTTP.

// mreqAIE2EEnv builds the whole slice: the AI configured (or not), the stand-in installed, the
// requirements, warns, rules and greetings modules loaded on the dispatcher, Check on join on, no
// rule stored and a live owner who can press the page.
func mreqAIE2EEnv(t *testing.T, configured bool) (*memberReqEnv, *mreqAIState, gotgbot.User) {
	t.Helper()
	if configured {
		withAIConfigured(t)
	} else {
		withAINotConfigured(t, mreqAINoKey)
	}
	ai := mreqInstallAIFake(t)
	mreqAICleanup(t, nil, nil)
	env := mreqJoinEnv(t, nil, true)
	env.loadMemberReq()
	return env, ai, env.newOwner(env.chat)
}

// mreqAIE2ENoNewJobs runs fn with the runner refusing every job and fails the test when the watcher
// tried to start one, then puts the runner's limit back, so the test can go on with real jobs.
func mreqAIE2ENoNewJobs(t *testing.T, what string, fn func()) {
	t.Helper()
	previous := memberReqMaxInFlight
	starts := mreqCountJobStarts(t)
	fn()
	memberReqJobs.wait()
	if n := starts(); n != 0 {
		t.Errorf("%s tried to start %d jobs, want none", what, n)
	}
	memberReqMaxInFlight = previous
}

// mreqAIE2ENewNotice returns the one message the group received since before, and fails unless it is
// exactly one.
func mreqAIE2ENewNotice(t *testing.T, env *memberReqEnv, before int) staffSentMessage {
	t.Helper()
	notices := env.mreqNotices()
	if len(notices) != before+1 {
		t.Fatalf("messages to the group = %d, want %d (one new notice): %q", len(notices), before+1, env.mreqSentTexts())
	}
	return notices[len(notices)-1]
}

// mreqAIE2EWantAINotice fails unless text names the AI phrase and ends with the AI remedy.
func mreqAIE2EWantAINotice(t *testing.T, notice staffSentMessage) {
	t.Helper()
	aiRule, ok := memberReqRuleByCode("ai")
	if !ok {
		t.Fatal("no AI rule")
	}
	text := mreqText(notice)
	if !strings.Contains(text, staffMarker(aiRule.PhraseKey)) || !strings.HasSuffix(text, " "+staffMarker("mreq_remedy_ai")) {
		t.Errorf("notice %q must name the AI phrase and end with the AI remedy", text)
	}
}

func TestMemberReqAIEndToEnd(t *testing.T) {
	t.Run("owner, joiner, rename, button", func(t *testing.T) {
		env, ai, owner := mreqAIE2EEnv(t, true)
		msgID := settingsMreqOpenPage(t, env.settingsEnv, owner)
		mreqE2ESetRule(t, env, owner, msgID, "ai", models.RequirementActionMute)
		if got := settingsMreqStoredActions(t, env.chat.Id); got != [8]int{7: models.RequirementActionMute} {
			t.Fatalf("stored punishments = %v, want only the AI rule on Mute", got)
		}
		if n := ai.count(); n != 0 {
			t.Fatalf("provider requests while the owner set the rule = %d, want none", n)
		}

		// A joiner whose names the provider rates 0.97 is muted once, with one notice.
		ai.setDefault(mreqAIProbability(0.97))
		member := mreqAIMember(env, "Casino")
		noticesBefore := len(env.mreqNotices())
		env.mreqJoin(member)
		notice := mreqAIE2ENewNotice(t, env, noticesBefore)
		mreqAIE2EWantAINotice(t, notice)
		if got := mreqAIWelcomes(env); got != 0 {
			t.Errorf("welcomes = %d, want none for a muted joiner", got)
		}
		hold := env.mreqHold(member.Id)
		if hold == nil || hold.Rules != mreqBitOf(t, "ai") || hold.PunishedAt == nil || hold.Action != models.RequirementActionMute || !hold.WelcomeHeld {
			t.Fatalf("hold = %+v, want a punished AI mute hold that holds the welcome", hold)
		}
		restricts := env.mreqRestricts(member.Id)
		if len(restricts) != 1 {
			t.Fatalf("restrictChatMember calls about the member = %d, want one", len(restricts))
		}
		if _, marker := restricts[0].Params["until_date"]; !marker {
			t.Errorf("the mute %+v carries no marker end date", restricts[0].Params)
		}
		if n := ai.count(); n != 1 {
			t.Errorf("provider requests = %d, want 1", n)
		}
		mreqAIWantMarker(t, env.chat.Id, member.Id, true)

		// The second delivery of the same join punishes nobody again.
		env.mreqServiceJoin(member)
		if n := len(env.mreqRestricts(member.Id)); n != 1 {
			t.Errorf("restrictChatMember calls after the second delivery = %d, want still one", n)
		}
		if n := len(env.mreqNotices()); n != noticesBefore+1 {
			t.Errorf("messages after the second delivery = %d, want %d: %q", n, noticesBefore+1, env.mreqSentTexts())
		}
		if n := ai.count(); n != 1 {
			t.Errorf("provider requests after the second delivery = %d, want still 1", n)
		}

		// The member renames and taps the button: one request for the new string frees them.
		ordinary := mreqAIOrdinaryRename(env, member)
		ai.setDefault(mreqAIProbability(0.05))
		requestsBefore := ai.count()
		restrictsBefore, answersBefore := len(env.mreqRestricts(member.Id)), len(env.mreqAnswers())
		env.mreqPressFixed(ordinary, notice)
		mreqWantFixedUnmute(t, env, ordinary, notice, restrictsBefore, answersBefore, lockdownSnapshotPermissions(t),
			mreqAnswer{Text: staffMarker("mreq_fixed_done")},
			mreqFixedNoticeText(t, env.translator(), "mreq_notice_fixed", ordinary))
		if n := ai.count() - requestsBefore; n != 1 {
			t.Errorf("provider requests for the press = %d, want exactly 1 for the new string", n)
		}
		mreqAIWantBaseline(t, member.Id, mreqAIHashOfUser(t, ordinary))
		mreqAIWantMarker(t, env.chat.Id, member.Id, false)
		if got := mreqAIWelcomes(env); got != 1 {
			t.Errorf("welcomes after the member was freed = %d, want the held welcome once", got)
		}

		// Their next message starts no job.
		mreqAIE2ENoNewJobs(t, "a message of the freed member", func() { env.mreqSend(ordinary, "hello again") })
		if hold := env.mreqHold(member.Id); hold != nil {
			t.Errorf("hold after the member's next message = %+v, want none", hold)
		}
		if n := ai.count() - requestsBefore; n != 1 {
			t.Errorf("provider requests in all after the fix = %d, want still 1", n)
		}
	})

	t.Run("established member: recorded, then judged on a rename", func(t *testing.T) {
		env, ai := mreqAIWatchEnv(t, models.RequirementActionWarn)
		ai.setDefault(mreqAIProbability(0.95))
		member := mreqAIOrdinary(env)
		hash := mreqAIHashOfUser(t, member)

		// No join record: the first message records a baseline and asks nobody.
		env.mreqSend(member, "hello")
		memberReqJobs.wait()
		if n := ai.count(); n != 0 {
			t.Fatalf("provider requests for an established member = %d, want none", n)
		}
		mreqAIWantBaseline(t, member.Id, hash)
		mreqAIWantMarker(t, env.chat.Id, member.Id, false)
		if hold := env.mreqHold(member.Id); hold != nil || len(env.mreqNotices()) != 0 {
			t.Fatalf("hold = %+v, notices = %q, want nobody punished", hold, env.mreqSentTexts())
		}
		mreqAIE2ENoNewJobs(t, "the second message of a recorded member", func() { env.mreqSend(member, "hello again") })

		// The rename to spam-like names is judged: one request, one warn, one notice with the remedy.
		renamed := mreqAIRename(env, member, "Casino")
		env.mreqSend(renamed, "buy now")
		memberReqJobs.wait()
		if n := ai.count(); n != 1 {
			t.Errorf("provider requests for the rename = %d, want 1", n)
		}
		row := env.mreqWarnRow(member.Id)
		if row == nil || row.NumWarns != 1 {
			t.Fatalf("warns = %+v, want one warn", row)
		}
		hold := env.mreqHold(member.Id)
		if hold == nil || hold.Rules != mreqBitOf(t, "ai") || hold.Action != models.RequirementActionWarn || hold.PunishedAt == nil {
			t.Fatalf("hold = %+v, want a punished AI warn hold", hold)
		}
		mreqAIE2EWantAINotice(t, mreqAIE2ENewNotice(t, env, 0))
		mreqAIWantMarker(t, env.chat.Id, member.Id, true)

		// Renamed back and written again, the hold is released: one request for the string, no new warn.
		ordinary := mreqAIOrdinaryRename(env, renamed)
		ai.setDefault(mreqAIProbability(0.05))
		env.mreqSendAt(ordinary, "sorry about the name", time.Now().Unix()+5)
		memberReqJobs.wait()
		if hold := env.mreqHold(member.Id); hold != nil {
			t.Errorf("hold after the rename back = %+v, want it released", hold)
		}
		if n := ai.count(); n != 2 {
			t.Errorf("provider requests in all = %d, want 2", n)
		}
		if row := env.mreqWarnRow(member.Id); row == nil || row.NumWarns != 1 {
			t.Errorf("warns after the release = %+v, want still one", row)
		}
		if n := len(env.mreqNotices()); n != 1 {
			t.Errorf("messages to the group = %d, want the one warn notice: %q", n, env.mreqSentTexts())
		}
	})

	t.Run("approved members and administrators are never judged", func(t *testing.T) {
		env, ai, _ := mreqAIE2EEnv(t, true)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleAI: models.RequirementActionMute})
		ai.setDefault(mreqAIProbability(0.99))

		approved := mreqAIMember(env, "Casino")
		mreqApprove(t, env, approved)
		administrator := mreqAIMember(env, "Casino")
		env.fake.setMember(env.chat.Id, administrator.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusAdministrator, IsMember: true})

		for name, member := range map[string]gotgbot.User{"approved": approved, "administrator": administrator} {
			welcomes := mreqAIWelcomes(env)
			env.mreqJoin(member)
			env.mreqSend(member, "plain text")
			memberReqJobs.wait()
			if got := mreqAIWelcomes(env) - welcomes; got != 1 {
				t.Errorf("%s: welcomes = %d, want 1", name, got)
			}
			if hold := env.mreqHold(member.Id); hold != nil {
				t.Errorf("%s: hold = %+v, want none", name, hold)
			}
			if n := len(env.mreqRestricts(member.Id)); n != 0 {
				t.Errorf("%s: restrictChatMember calls = %d, want none", name, n)
			}
		}
		if n := ai.count(); n != 0 {
			t.Errorf("provider requests = %d, want none for an approved member and an administrator", n)
		}
	})

	t.Run("the AI not configured", func(t *testing.T) {
		env, ai, owner := mreqAIE2EEnv(t, false)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleAI: models.RequirementActionMute})
		msgID := settingsMreqOpenPage(t, env.settingsEnv, owner)

		// The page says so in words, and the rule's screen is the Off-only form with the stored Mute shown.
		text, _ := env.lastEdit(env.chat.Id, msgID)
		if got := strings.Count(text, settingsMreqAILine()); got != 1 {
			t.Errorf("the page %q lists the AI line %d times, want once", text, got)
		}
		env.press(owner, msgID, env.groupFields(owner, "a", "mo", "t", "ai"))
		text, keyboard := env.lastEdit(env.chat.Id, msgID)
		if want := settingsMreqAIScreenHead(settingsMreqAILine(), models.RequirementActionMute); !strings.Contains(text, want) {
			t.Errorf("the AI screen %q lacks %q", text, want)
		}
		if rows := settingsMreqRowWidths(keyboard); !settingsMreqSameInts(rows, []int{1, 2}) {
			t.Errorf("the AI screen rows = %v, want the Off-only [1 2]", rows)
		}

		// A punishing press is refused with the alert and nothing is written.
		wantAlert := mreqAnswer{Text: staffMarker("settings_mreq_here_ai") + " " + staffMarker("settings_mreq_rule_ai"), Alert: true}
		refused := func(what string, data func()) {
			t.Helper()
			answersBefore := len(env.mreqAnswers())
			data()
			if got := env.mreqOnlyAnswer(answersBefore); got != wantAlert {
				t.Errorf("%s: answer = %+v, want %+v", what, got, wantAlert)
			}
			if text, _ := env.lastEdit(env.chat.Id, msgID); strings.Contains(text, staffMarker("settings_mreq_confirm")) {
				t.Errorf("%s drew a card: %q", what, text)
			}
		}
		refused("Warn", func() {
			env.press(owner, msgID, env.groupFields(owner, settingsMreqAIStrictKV(models.RequirementActionWarn)...))
		})
		if got := settingsMreqStoredActions(t, env.chat.Id); got != [8]int{7: models.RequirementActionMute} {
			t.Errorf("stored after the refused Warn = %v, want the Mute from before", got)
		}

		// Off is accepted and clears the stored value.
		answersBefore := len(env.mreqAnswers())
		env.press(owner, msgID, env.groupFields(owner, settingsMreqAIStrictKV(models.RequirementActionOff)...))
		if got := env.mreqOnlyAnswer(answersBefore); got.Alert {
			t.Errorf("Off answered with an alert %+v, want it accepted", got)
		}
		if got := settingsMreqStoredActions(t, env.chat.Id); got != [8]int{} {
			t.Fatalf("stored after Off = %v, want every rule off", got)
		}

		// Mute and a forged Yes of its card are refused too.
		refused("Mute", func() {
			env.press(owner, msgID, env.groupFields(owner, settingsMreqAIStrictKV(models.RequirementActionMute)...))
		})
		refused("a forged Yes", func() {
			data := encodeCallbackData(settingsCallbackNamespace, env.groupFields(owner, "a", "my", "t", "ai", "v", "4", "e", settingsMreqExpiryMinute()))
			if data == "" {
				t.Fatal("the forged Yes did not encode")
			}
			env.pressRaw(env.chat, owner, msgID, data)
		})
		if got := settingsMreqStoredActions(t, env.chat.Id); got != [8]int{} {
			t.Errorf("stored after the refused Mute and Yes = %v, want every rule off", got)
		}

		// With a Mute stored behind the settings' back, a joiner is welcomed: no request, no punishment.
		// The same holds when ENABLE_AISPAM is off and the key is set.
		for _, how := range []mreqAIOff{mreqAINoKey, mreqAISwitchOff} {
			withAINotConfigured(t, how)
			env.mreqRules(map[memberreq.Rule]int{memberreq.RuleAI: models.RequirementActionMute})
			member := mreqAIMember(env, "Casino")
			welcomes := mreqAIWelcomes(env)
			env.mreqJoin(member)
			env.mreqSend(member, "plain text")
			memberReqJobs.wait()
			if got := mreqAIWelcomes(env) - welcomes; got != 1 {
				t.Errorf("welcomes for the joiner = %d, want 1", got)
			}
			if hold := env.mreqHold(member.Id); hold != nil {
				t.Errorf("hold = %+v, want none", hold)
			}
			if n := len(env.mreqRestricts(member.Id)); n != 0 {
				t.Errorf("restrictChatMember calls = %d, want none", n)
			}
			mreqAIWantMarker(t, env.chat.Id, member.Id, false)
			mreqAIWantNoBaseline(t, member.Id)
		}
		if n := ai.count(); n != 0 {
			t.Errorf("provider requests = %d, want none", n)
		}
	})

	t.Run("a rejected key and the breaker", func(t *testing.T) {
		env, ai, _ := mreqAIE2EEnv(t, true)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleAI: models.RequirementActionMute})
		clock := mreqAIInstallClock(t)

		// A rejected key gives no punishment: the joiner is welcomed and keeps the marker.
		ai.setDefault(mreqAIStatus(401, "authentication_error", ""))
		first := mreqAIMember(env, "Casino")
		welcomes := mreqAIWelcomes(env)
		env.mreqJoin(first)
		if got := mreqAIWelcomes(env) - welcomes; got != 1 {
			t.Errorf("welcomes after a rejected key = %d, want 1", got)
		}
		if hold := env.mreqHold(first.Id); hold != nil || len(env.mreqRestricts(first.Id)) != 0 {
			t.Errorf("hold = %+v, restricts = %d, want nobody punished by a rejected key", hold, len(env.mreqRestricts(first.Id)))
		}
		mreqAIWantMarker(t, env.chat.Id, first.Id, true)
		if n := ai.count(); n != 1 {
			t.Fatalf("provider requests for the rejected key = %d, want 1", n)
		}

		// The provider-wide pause makes the next join make no request.
		ai.setDefault(mreqAIProbability(0.97))
		second := mreqAIMember(env, "Casino")
		welcomes = mreqAIWelcomes(env)
		env.mreqJoin(second)
		if n := ai.count(); n != 1 {
			t.Errorf("provider requests during the pause = %d, want still 1", n)
		}
		if got := mreqAIWelcomes(env) - welcomes; got != 1 {
			t.Errorf("welcomes during the pause = %d, want 1", got)
		}
		if hold := env.mreqHold(second.Id); hold != nil {
			t.Errorf("hold during the pause = %+v, want none", hold)
		}

		// When the pause ends, a working answer punishes the next spam-named joiner.
		clock.advance(memberReqAIKeyPause + time.Minute)
		third := mreqAIMember(env, "Casino")
		env.mreqJoin(third)
		if n := ai.count(); n != 2 {
			t.Errorf("provider requests after the pause = %d, want 2", n)
		}
		if hold := env.mreqHold(third.Id); hold == nil || hold.Rules != mreqBitOf(t, "ai") || hold.PunishedAt == nil {
			t.Errorf("hold after the pause = %+v, want a punished AI hold", hold)
		}

		// A 500 at join welcomes the joiner and leaves the marker; the first message is judged once
		// the provider answers.
		ai.setDefault(mreqAIStatus(500, "api_error", ""))
		fourth := mreqAIMember(env, "Casino")
		welcomes = mreqAIWelcomes(env)
		env.mreqJoin(fourth)
		if got := mreqAIWelcomes(env) - welcomes; got != 1 {
			t.Errorf("welcomes after a 500 = %d, want 1", got)
		}
		if hold := env.mreqHold(fourth.Id); hold != nil {
			t.Fatalf("hold after a 500 = %+v, want none", hold)
		}
		mreqAIWantMarker(t, env.chat.Id, fourth.Id, true)
		ai.setDefault(mreqAIProbability(0.97))
		requestsBefore := ai.count()
		env.mreqSend(fourth, "hello")
		memberReqJobs.wait()
		if n := ai.count() - requestsBefore; n != 1 {
			t.Errorf("provider requests for the first message = %d, want 1", n)
		}
		if hold := env.mreqHold(fourth.Id); hold == nil || hold.Rules != mreqBitOf(t, "ai") {
			t.Errorf("hold after the first message = %+v, want the AI hold", hold)
		}
	})
}
