//go:build testtools

package modules

import (
	"context"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/greetings"
	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// mreqJoinEnv builds the member requirements environment with the greetings module loaded on
// its dispatcher and a welcome text a test can recognise ("Welcome <first name>"). The rules
// are stored first (none for a nil map) and Check on join is set as asked, through the real
// repository. Its greetings, greeted_members, captcha and join-claim rows are removed by
// memberReqCleanup.
func mreqJoinEnv(t *testing.T, rules map[memberreq.Rule]int, checkOnJoin bool) *memberReqEnv {
	t.Helper()
	env := newMemberReqEnv(t)
	LoadGreetings(env.dispatcher)
	if err := greetings.SetWelcomeText(env.chat.Id, "Welcome {first}", "", nil, db.TEXT); err != nil {
		t.Fatalf("SetWelcomeText error = %v", err)
	}
	if len(rules) > 0 {
		env.mreqRules(rules)
	}
	if err := memberreq.SetCheckOnJoin(context.Background(), env.chat.Id, checkOnJoin); err != nil {
		t.Fatalf("SetCheckOnJoin(%t) error = %v", checkOnJoin, err)
	}
	return env
}

// mreqJoin runs the member's join as a chat_member update through the real dispatcher, as a
// self-join (so the raid counter makes no performer lookup).
func (e *memberReqEnv) mreqJoin(user gotgbot.User) {
	e.t.Helper()
	e.join(user, user, "")
}

// mreqServiceJoin runs the member's join as the new_chat_members service message, the second
// delivery of the same join.
func (e *memberReqEnv) mreqServiceJoin(user gotgbot.User) {
	e.t.Helper()
	e.serviceJoin(user, nil, user)
}

// mreqSentTexts returns the texts of the messages the bot sent to the group, in order.
func (e *memberReqEnv) mreqSentTexts() []string {
	e.t.Helper()
	var texts []string
	for _, sent := range e.mreqNotices() {
		texts = append(texts, mreqText(sent))
	}
	return texts
}

// mreqCountText counts the texts that contain needle.
func mreqCountText(texts []string, needle string) int {
	count := 0
	for _, text := range texts {
		if strings.Contains(text, needle) {
			count++
		}
	}
	return count
}

// mreqIndexOfText is the index of the first text that contains needle, or -1.
func mreqIndexOfText(texts []string, needle string) int {
	for i, text := range texts {
		if strings.Contains(text, needle) {
			return i
		}
	}
	return -1
}

// mreqWelcomeWord is what the welcome text of mreqJoinEnv starts with.
const mreqWelcomeWord = "Welcome"

func TestMemberReqJoinNoCaptcha(t *testing.T) {
	t.Run("a mute on join holds the welcome and records it on the hold", func(t *testing.T) {
		env := mreqJoinEnv(t, map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionMute}, true)
		member := env.mreqMember("Alex", "", "alex")

		env.mreqJoin(member)
		env.mreqServiceJoin(member)

		texts := env.mreqSentTexts()
		if got := mreqCountText(texts, mreqWelcomeWord); got != 0 {
			t.Fatalf("welcomes = %d, want none for a requirement-muted joiner (texts %v)", got, texts)
		}
		if got := mreqCountText(texts, staffMarker("mreq_notice_mute")); got != 1 {
			t.Fatalf("mute notices = %d, want exactly one for the two deliveries of one join (texts %v)", got, texts)
		}
		if got := env.mreqCallsAbout(member.Id, "restrictChatMember"); got != 1 {
			t.Fatalf("restrictChatMember calls = %d, want the one requirement mute", got)
		}
		hold := env.mreqHold(member.Id)
		if hold == nil || hold.Action != models.RequirementActionMute || hold.PunishedAt == nil || !hold.WelcomeHeld {
			t.Fatalf("hold = %+v, want a punished mute hold with welcome_held true", hold)
		}
	})

	t.Run("a warn posts its notice and then the usual welcome", func(t *testing.T) {
		env := mreqJoinEnv(t, map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionWarn}, true)
		member := env.mreqMember("Alex", "", "alex")

		env.mreqJoin(member)

		texts := env.mreqSentTexts()
		notice, welcome := mreqIndexOfText(texts, staffMarker("mreq_notice_warn")), mreqIndexOfText(texts, mreqWelcomeWord)
		if notice < 0 || welcome < 0 || notice > welcome {
			t.Fatalf("notice at %d, welcome at %d, want the warn notice first and then the welcome (texts %v)", notice, welcome, texts)
		}
		if row := env.mreqWarnRow(member.Id); row == nil || row.NumWarns != 1 {
			t.Fatalf("warns row = %+v, want one warn", row)
		}
	})

	removals := []struct {
		name   string
		action int
		method string
	}{
		{"a kick on join sends no welcome", models.RequirementActionKick, "unbanChatMember"},
		{"a ban on join sends no welcome", models.RequirementActionBan, "banChatMember"},
	}
	for _, tc := range removals {
		t.Run(tc.name, func(t *testing.T) {
			env := mreqJoinEnv(t, map[memberreq.Rule]int{memberreq.RuleLastName: tc.action}, true)
			member := env.mreqMember("Alex", "", "alex")

			env.mreqJoin(member)

			if got := mreqCountText(env.mreqSentTexts(), mreqWelcomeWord); got != 0 {
				t.Fatalf("welcomes = %d, want none for a removed joiner", got)
			}
			if got := env.mreqCallsAbout(member.Id, tc.method); got != 1 {
				t.Fatalf("%s calls = %d, want 1", tc.method, got)
			}
		})
	}

	t.Run("a joiner with a last name is welcomed and not noticed", func(t *testing.T) {
		env := mreqJoinEnv(t, map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionMute}, true)
		member := env.mreqMember("Alex", "Smith", "alex")

		env.mreqJoin(member)

		texts := env.mreqSentTexts()
		if got := mreqCountText(texts, mreqWelcomeWord); got != 1 {
			t.Fatalf("welcomes = %d, want 1 (texts %v)", got, texts)
		}
		if got := mreqCountText(texts, "@@mreq-notice"); got != 0 {
			t.Fatalf("notices = %d, want none for a clean joiner", got)
		}
		if hold := env.mreqHold(member.Id); hold != nil {
			t.Fatalf("hold = %+v, want none", hold)
		}
	})

	t.Run("with Check on join off the joiner is welcomed and nobody looks them up", func(t *testing.T) {
		env := mreqJoinEnv(t, map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionMute}, false)
		member := env.mreqMember("Alex", "", "alex")

		env.mreqJoin(member)

		if got := mreqCountText(env.mreqSentTexts(), mreqWelcomeWord); got != 1 {
			t.Fatalf("welcomes = %d, want 1", got)
		}
		if got := env.mreqMemberLookups(member.Id); got != 0 {
			t.Fatalf("getChatMember calls about the joiner = %d, want none", got)
		}
		if hold := env.mreqHold(member.Id); hold != nil {
			t.Fatalf("hold = %+v, want none", hold)
		}
	})

	t.Run("with every rule off the joiner is welcomed and nobody looks them up", func(t *testing.T) {
		env := mreqJoinEnv(t, nil, true)
		member := env.mreqMember("Alex", "", "alex")

		env.mreqJoin(member)

		if got := mreqCountText(env.mreqSentTexts(), mreqWelcomeWord); got != 1 {
			t.Fatalf("welcomes = %d, want 1", got)
		}
		if got := env.mreqMemberLookups(member.Id); got != 0 {
			t.Fatalf("getChatMember calls about the joiner = %d, want none", got)
		}
	})
}

func TestMemberReqJoinOffUntilLive(t *testing.T) {
	rules := map[memberreq.Rule]int{
		memberreq.RuleLastName: models.RequirementActionMute,
		memberreq.RuleUsername: models.RequirementActionMute,
	}
	env := mreqJoinEnv(t, rules, true)
	withMemberReqLive(t, false)
	member := env.mreqMember("Alex", "", "")

	env.mreqJoin(member)

	texts := env.mreqSentTexts()
	if got := mreqCountText(texts, mreqWelcomeWord); got != 1 {
		t.Fatalf("welcomes = %d, want exactly the plain welcome while enforcement is off (texts %v)", got, texts)
	}
	if got := mreqCountText(texts, "@@mreq-notice"); got != 0 {
		t.Fatalf("notices = %d, want none", got)
	}
	if got := env.mreqMemberLookups(member.Id); got != 0 {
		t.Fatalf("getChatMember calls about the joiner = %d, want none", got)
	}
	if got := env.mreqCallCount("restrictChatMember", "banChatMember", "unbanChatMember"); got != 0 {
		t.Fatalf("punishing calls = %d, want none", got)
	}
	if hold := env.mreqHold(member.Id); hold != nil {
		t.Fatalf("hold = %+v, want none", hold)
	}
}
