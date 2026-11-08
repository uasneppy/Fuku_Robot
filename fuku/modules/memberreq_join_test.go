//go:build testtools

package modules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/approvals"
	"github.com/uasneppy/fuku_robot/fuku/db/captcha"
	"github.com/uasneppy/fuku_robot/fuku/db/greetings"
	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/utils/captchaweb"
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

// mreqPressCaptcha presses the captcha button of member in the environment's group with the
// given answer ("" is the right one), through the real verify callback.
func (e *memberReqEnv) mreqPressCaptcha(member gotgbot.User, answer string) {
	e.t.Helper()
	attempt, err := captcha.GetCaptchaAttempt(member.Id, e.chat.Id)
	if err != nil || attempt == nil {
		e.t.Fatalf("GetCaptchaAttempt(%d) = %v, %v, want an attempt", member.Id, attempt, err)
	}
	if answer == "" {
		answer = attempt.Answer
	}
	data := encodeCallbackData("captcha_verify",
		map[string]string{"a": strconv.FormatInt(int64(attempt.ID), 10), "u": strconv.FormatInt(member.Id, 10), "s": answer})
	ctx := newModuleCallbackContext(e.bot, e.chat, member, data)
	if err := captchaModule.captchaVerifyCallback(e.bot, ctx); err != nil {
		e.t.Fatalf("captchaVerifyCallback(%d) error = %v", member.Id, err)
	}
}

// mreqCaptchaOn turns the captcha on for the environment's group.
func (e *memberReqEnv) mreqCaptchaOn() {
	e.t.Helper()
	if err := captcha.SetCaptchaEnabled(e.chat.Id, true); err != nil {
		e.t.Fatalf("SetCaptchaEnabled error = %v", err)
	}
}

// mreqRestricts returns the restrictChatMember calls about userID, in order.
func (e *memberReqEnv) mreqRestricts(userID int64) []moduleBotCall {
	e.t.Helper()
	var matched []moduleBotCall
	for _, call := range e.fake.callsFor("restrictChatMember") {
		if staffParamInt(call.Params, "user_id") == userID {
			matched = append(matched, call)
		}
	}
	return matched
}

func TestMemberReqJoinAfterCaptcha(t *testing.T) {
	muteRules := map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionMute}

	t.Run("the button pass, then the check", func(t *testing.T) {
		env := mreqJoinEnv(t, muteRules, true)
		env.mreqCaptchaOn()
		member := env.mreqMember("Alex", "", "alex")

		env.mreqJoin(member)
		if hold := env.mreqHold(member.Id); hold != nil {
			t.Fatalf("hold before the captcha pass = %+v, want none: nothing is checked before the pass", hold)
		}
		if got := mreqCountText(env.mreqSentTexts(), "@@mreq-notice"); got != 0 {
			t.Fatalf("requirement notices before the captcha pass = %d, want none", got)
		}

		env.mreqPressCaptcha(member, "")

		texts := env.mreqSentTexts()
		success, notice := mreqIndexOfText(texts, staffMarker("greetings_captcha_verified_success")), mreqIndexOfText(texts, staffMarker("mreq_notice_mute"))
		if success < 0 || notice < 0 || success > notice {
			t.Fatalf("success note at %d, mute notice at %d, want the captcha's note first and then the notice (texts %v)", success, notice, texts)
		}
		if got := mreqCountText(texts, mreqWelcomeWord); got != 0 {
			t.Fatalf("welcomes = %d, want none for a requirement-muted member (texts %v)", got, texts)
		}
		hold := env.mreqHold(member.Id)
		if hold == nil || hold.Action != models.RequirementActionMute || hold.PunishedAt == nil || !hold.WelcomeHeld {
			t.Fatalf("hold = %+v, want a punished mute hold with welcome_held true", hold)
		}
		restricts := env.mreqRestricts(member.Id)
		last := restricts[len(restricts)-1]
		if perms, ok := last.Params["permissions"].(gotgbot.ChatPermissions); !ok || !reflect.DeepEqual(perms, MutedPermissions) ||
			staffParamInt(last.Params, "until_date") != hold.MuteUntil {
			t.Fatalf("the last restrict of the member = %+v, want MutedPermissions until the hold's marker %d (all %d restricts)", last.Params, hold.MuteUntil, len(restricts))
		}
	})

	t.Run("the web pass, then the check", func(t *testing.T) {
		web := newCaptchaWebEnv(t)
		withMemberReqLive(t, true)
		withFastMemberReqPacer(t)
		member := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Alex"}
		memberReqCleanup(t, []int64{web.chat.Id}, []int64{member.Id})
		web.client.responses["getChatMember"] = json.RawMessage(fmt.Sprintf(
			`{"status":"member","user":{"id":%d,"is_bot":false,"first_name":"Alex"}}`, member.Id))
		if err := greetings.SetWelcomeText(web.chat.Id, "Welcome {first}", "", nil, db.TEXT); err != nil {
			t.Fatalf("SetWelcomeText error = %v", err)
		}
		if err := memberreq.SetRuleAction(context.Background(), web.chat.Id, memberreq.RuleLastName, models.RequirementActionMute); err != nil {
			t.Fatalf("SetRuleAction error = %v", err)
		}
		if err := memberreq.SetCheckOnJoin(context.Background(), web.chat.Id, true); err != nil {
			t.Fatalf("SetCheckOnJoin error = %v", err)
		}

		token := web.join(member)
		if hold, err := memberreq.GetHoldFresh(context.Background(), web.chat.Id, member.Id); err != nil || hold != nil {
			t.Fatalf("hold before the web pass = %+v, %v, want none", hold, err)
		}
		status, answer := web.verify(member, token)
		if status != http.StatusOK || answer.Result != captchaweb.ResultPassed {
			t.Fatalf("verify = %d %q, want 200 passed", status, answer.Result)
		}

		var texts []string
		for _, call := range web.client.callsFor("sendMessage") {
			texts = append(texts, fmt.Sprint(call.Params["text"]))
		}
		if got := mreqCountText(texts, staffMarker("mreq_notice_mute")); got != 1 {
			t.Fatalf("mute notices = %d, want 1 (texts %v)", got, texts)
		}
		if got := mreqCountText(texts, mreqWelcomeWord); got != 0 {
			t.Fatalf("welcomes = %d, want none for a requirement-muted member (texts %v)", got, texts)
		}
		hold, err := memberreq.GetHoldFresh(context.Background(), web.chat.Id, member.Id)
		if err != nil || hold == nil || hold.Action != models.RequirementActionMute || hold.PunishedAt == nil || !hold.WelcomeHeld {
			t.Fatalf("hold = %+v, %v, want a punished mute hold with welcome_held true", hold, err)
		}
		restricts := web.client.callsFor("restrictChatMember")
		last := restricts[len(restricts)-1]
		if perms, ok := last.Params["permissions"].(gotgbot.ChatPermissions); !ok || !reflect.DeepEqual(perms, MutedPermissions) ||
			staffParamInt(last.Params, "until_date") != hold.MuteUntil {
			t.Fatalf("the last restrict = %+v, want MutedPermissions until the hold's marker %d", last.Params, hold.MuteUntil)
		}
	})

	t.Run("a joiner who fails the captcha is never checked", func(t *testing.T) {
		env := mreqJoinEnv(t, muteRules, true)
		env.mreqCaptchaOn()
		if err := captcha.SetCaptchaFailureAction(env.chat.Id, "ban"); err != nil {
			t.Fatalf("SetCaptchaFailureAction error = %v", err)
		}
		if err := captcha.SetCaptchaMaxAttempts(env.chat.Id, 1); err != nil {
			t.Fatalf("SetCaptchaMaxAttempts error = %v", err)
		}
		member := env.mreqMember("Alex", "", "alex")

		env.mreqJoin(member)
		env.mreqPressCaptcha(member, "not-the-answer")

		if got := env.mreqCallsAbout(member.Id, "banChatMember"); got != 1 {
			t.Fatalf("banChatMember calls = %d, want the captcha's failure action once", got)
		}
		if hold := env.mreqHold(member.Id); hold != nil {
			t.Fatalf("hold = %+v, want none: a captcha-failed joiner is never checked", hold)
		}
		texts := env.mreqSentTexts()
		if got := mreqCountText(texts, "@@mreq-notice") + mreqCountText(texts, mreqWelcomeWord); got != 0 {
			t.Fatalf("requirement notices and welcomes = %d, want none (texts %v)", got, texts)
		}
	})

	t.Run("a joiner the lockdown guard records for a ban is never checked", func(t *testing.T) {
		env := mreqJoinEnv(t, muteRules, true)
		env.lockAsAdmin("raid")
		member := env.mreqMember("Alex", "", "alex")

		env.mreqJoin(member)

		if row := env.joinerRow(member.Id); row.State != models.JoinerStatePending {
			t.Fatalf("joiner row state = %q, want pending: the guard recorded the ban", row.State)
		}
		if hold := env.mreqHold(member.Id); hold != nil {
			t.Fatalf("hold = %+v, want none: a lockdown-banned joiner is never checked", hold)
		}
		if got := env.mreqMemberLookups(member.Id); got != 0 {
			t.Fatalf("getChatMember calls about the joiner = %d, want none", got)
		}
		texts := env.mreqSentTexts()
		if got := mreqCountText(texts, "@@mreq-notice") + mreqCountText(texts, mreqWelcomeWord); got != 0 {
			t.Fatalf("requirement notices and welcomes = %d, want none (texts %v)", got, texts)
		}
	})
}

// mreqHoldReadCounter replaces memberReqHoldRead for the test with a function that counts its
// calls and, when fail is set, always answers with an error whose own text would read as a
// permanent unmute failure.
func mreqHoldReadCounter(t *testing.T, fail bool) *atomic.Int64 {
	t.Helper()
	var calls atomic.Int64
	mreqSwap(t, &memberReqHoldRead, func(ctx context.Context, chatID, userID int64) (*models.ChatRequirementHold, error) {
		calls.Add(1)
		if fail {
			return nil, errors.New("database is down: user not found")
		}
		return memberreq.GetHoldFresh(ctx, chatID, userID)
	})
	return &calls
}

// mreqDueMutedRow stores a captcha_muted_users row that is already due for the member.
func (e *memberReqEnv) mreqDueMutedRow(userID int64) {
	e.t.Helper()
	if err := captcha.CreateMutedUser(userID, e.chat.Id, time.Now().Add(-time.Second)); err != nil {
		e.t.Fatalf("CreateMutedUser error = %v", err)
	}
}

// mreqMutedRowExists reports whether the member still has a captcha_muted_users row.
func (e *memberReqEnv) mreqMutedRowExists(userID int64) bool {
	e.t.Helper()
	row, err := captcha.GetMutedUser(userID, e.chat.Id)
	if err != nil {
		e.t.Fatalf("GetMutedUser error = %v", err)
	}
	return row != nil
}

// mreqWantUnmute fails the test unless call is a restrict that gives the member back the
// group's permissions (it can send messages and carries no end date).
func mreqWantUnmute(t *testing.T, call moduleBotCall) {
	t.Helper()
	if perms := permissionsOf(t, call); !perms.CanSendMessages {
		t.Fatalf("restrict permissions = %+v, want an unmute", perms)
	}
	if until := staffParamInt(call.Params, "until_date"); until != 0 {
		t.Fatalf("restrict until_date = %d, want none", until)
	}
}

func TestMemberReqCaptchaGuard(t *testing.T) {
	muteRules := map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionMute}

	t.Run("a punished requirement mute is re-applied and never lifted", func(t *testing.T) {
		env := newMemberReqEnv(t)
		env.mreqRules(muteRules)
		member := mreqHoldMember(t, env)
		hold := env.mreqHold(member.Id)
		before := len(env.mreqRestricts(member.Id))

		if err := unmuteCaptchaUser(env.bot, env.chat.Id, member.Id); err != nil {
			t.Fatalf("unmuteCaptchaUser error = %v, want nil", err)
		}

		restricts := env.mreqRestricts(member.Id)
		if len(restricts) != before+1 {
			t.Fatalf("restricts = %d, want one more than the %d before", len(restricts), before)
		}
		last := restricts[len(restricts)-1]
		if perms, ok := last.Params["permissions"].(gotgbot.ChatPermissions); !ok || !reflect.DeepEqual(perms, MutedPermissions) ||
			staffParamInt(last.Params, "until_date") != hold.MuteUntil {
			t.Fatalf("the guard's restrict = %+v, want MutedPermissions until the hold's marker %d", last.Params, hold.MuteUntil)
		}
		if got := len(env.calls("getChat")); got != 0 {
			t.Fatalf("getChat calls = %d, want none: nothing is unmuted", got)
		}
		if !isMemberReqMute(mreqLiveRecord(t, env, member.Id), hold.MuteUntil) {
			t.Error("the member no longer carries the requirement marker after the captcha's unmute")
		}
	})

	t.Run("no hold and a warn hold unmute exactly as before", func(t *testing.T) {
		env := newMemberReqEnv(t)
		plain := env.mreqMember("Alex", "Smith", "alex")
		if err := unmuteCaptchaUser(env.bot, env.chat.Id, plain.Id); err != nil {
			t.Fatalf("unmuteCaptchaUser(no hold) error = %v", err)
		}
		if restricts := env.mreqRestricts(plain.Id); len(restricts) != 1 {
			t.Fatalf("restricts of the member with no hold = %d, want the unmute", len(restricts))
		} else {
			mreqWantUnmute(t, restricts[0])
		}

		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionWarn})
		warned := env.mreqMember("Alex", "", "alex")
		got := env.mreqEnforce(env.mreqCase(warned))
		if got.Outcome != memberReqWarned {
			t.Fatalf("enforcement = %+v, want Warned", got)
		}
		if err := unmuteCaptchaUser(env.bot, env.chat.Id, warned.Id); err != nil {
			t.Fatalf("unmuteCaptchaUser(warn hold) error = %v", err)
		}
		if restricts := env.mreqRestricts(warned.Id); len(restricts) != 1 {
			t.Fatalf("restricts of the warned member = %d, want the unmute", len(restricts))
		} else {
			mreqWantUnmute(t, restricts[0])
		}
	})

	t.Run("a hold read error fails closed and the retry unmutes once it works", func(t *testing.T) {
		env := newMemberReqEnv(t)
		member := env.mreqMember("Alex", "Smith", "alex")
		calls := mreqHoldReadCounter(t, true)

		err := unmuteCaptchaUser(env.bot, env.chat.Id, member.Id)
		if err == nil {
			t.Fatal("unmuteCaptchaUser error = nil, want the unreadable hold to stop the unmute")
		}
		if isPermanentUnmuteError(err) {
			t.Fatalf("isPermanentUnmuteError(%v) = true, want false: a permanent answer would delete the retry row", err)
		}
		if msg := err.Error(); strings.Contains(msg, "database is down") ||
			!strings.Contains(msg, strconv.FormatInt(member.Id, 10)) || !strings.Contains(msg, strconv.FormatInt(env.chat.Id, 10)) {
			t.Fatalf("error text %q, want the chat and user IDs and nothing of the database's own text", msg)
		}
		if got := len(env.calls("getChat")) + len(env.mreqRestricts(member.Id)); got != 0 {
			t.Fatalf("getChat and restrict calls = %d, want none", got)
		}

		if err := restoreCaptchaPermissions(env.bot, env.chat.Id, member.Id); err == nil {
			t.Fatal("restoreCaptchaPermissions error = nil, want the failure reported")
		}
		if !env.mreqMutedRowExists(member.Id) {
			t.Fatal("no captcha_muted_users row after a failed unmute, want the retry scheduled")
		}

		env.mreqDueMutedRow(member.Id)
		unmuteExpiredCaptchaUsers(env.bot)
		if !env.mreqMutedRowExists(member.Id) {
			t.Fatal("the retry worker deleted the row of a member it could not unmute")
		}
		if got := len(env.mreqRestricts(member.Id)); got != 0 {
			t.Fatalf("restricts after a worker run with an unreadable hold = %d, want none", got)
		}
		if calls.Load() < 3 {
			t.Fatalf("hold reads = %d, want one per attempt", calls.Load())
		}

		memberReqHoldRead = memberreq.GetHoldFresh
		unmuteExpiredCaptchaUsers(env.bot)
		restricts := env.mreqRestricts(member.Id)
		if len(restricts) != 1 {
			t.Fatalf("restricts after the read works = %d, want the unmute", len(restricts))
		}
		mreqWantUnmute(t, restricts[0])
		if env.mreqMutedRowExists(member.Id) {
			t.Fatal("the row survived a worker run that unmuted the member")
		}
	})
}

func TestMemberReqCaptchaGuardOffUntilLive(t *testing.T) {
	env := newMemberReqEnv(t)
	env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionMute})
	member := mreqHoldMember(t, env)
	before := len(env.mreqRestricts(member.Id))
	withMemberReqLive(t, false)
	calls := mreqHoldReadCounter(t, true)

	wantOneMore := func(step string, n int) {
		t.Helper()
		restricts := env.mreqRestricts(member.Id)
		if len(restricts) != before+n {
			t.Fatalf("%s: restricts = %d, want %d", step, len(restricts), before+n)
		}
		mreqWantUnmute(t, restricts[len(restricts)-1])
		if got := len(env.calls("getChat")); got != n {
			t.Fatalf("%s: getChat calls = %d, want %d", step, got, n)
		}
	}

	if err := unmuteCaptchaUser(env.bot, env.chat.Id, member.Id); err != nil {
		t.Fatalf("unmuteCaptchaUser error = %v, want the plain unmute while enforcement is off", err)
	}
	wantOneMore("unmuteCaptchaUser", 1)

	if err := restoreCaptchaPermissions(env.bot, env.chat.Id, member.Id); err != nil {
		t.Fatalf("restoreCaptchaPermissions error = %v", err)
	}
	wantOneMore("restoreCaptchaPermissions", 2)

	env.mreqDueMutedRow(member.Id)
	unmuteExpiredCaptchaUsers(env.bot)
	wantOneMore("the retry worker", 3)
	if env.mreqMutedRowExists(member.Id) {
		t.Fatal("the retry worker left the row of a member it unmuted")
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("hold reads = %d, want none while enforcement is off", got)
	}
}

func TestMemberReqJoinExemptions(t *testing.T) {
	allMute := map[memberreq.Rule]int{
		memberreq.RuleLastName: models.RequirementActionMute,
		memberreq.RuleUsername: models.RequirementActionMute,
		memberreq.RuleRussian:  models.RequirementActionMute,
		memberreq.RuleArabic:   models.RequirementActionMute,
		memberreq.RuleChinese:  models.RequirementActionMute,
	}
	wantWelcomedNotPunished := func(t *testing.T, env *memberReqEnv, member gotgbot.User) {
		t.Helper()
		texts := env.mreqSentTexts()
		if got := mreqCountText(texts, mreqWelcomeWord); got != 1 {
			t.Fatalf("welcomes = %d, want 1 (texts %v)", got, texts)
		}
		if got := mreqCountText(texts, "@@mreq-notice"); got != 0 {
			t.Fatalf("notices = %d, want none", got)
		}
		if hold := env.mreqHold(member.Id); hold != nil {
			t.Fatalf("hold = %+v, want none", hold)
		}
		if got := len(env.mreqRestricts(member.Id)) + env.mreqCallsAbout(member.Id, "banChatMember", "unbanChatMember"); got != 0 {
			t.Fatalf("punishing calls about the member = %d, want none", got)
		}
	}

	t.Run("a bot that joins", func(t *testing.T) {
		env := mreqJoinEnv(t, allMute, true)
		bot := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "HelperBot", IsBot: true}
		memberReqCleanup(t, nil, []int64{bot.Id})

		env.mreqJoin(bot)

		wantWelcomedNotPunished(t, env, bot)
		if got := env.mreqMemberLookups(bot.Id); got != 0 {
			t.Fatalf("getChatMember calls about the bot = %d, want none", got)
		}
	})

	t.Run("an approved member who skips the captcha", func(t *testing.T) {
		env := mreqJoinEnv(t, allMute, true)
		env.mreqCaptchaOn()
		member := env.mreqMember("Alex", "", "")
		if err := approvals.AddApprovedUser(env.chat.Id, member.Id, env.admin.Id, "trusted"); err != nil {
			t.Fatalf("AddApprovedUser error = %v", err)
		}

		env.mreqJoin(member)

		wantWelcomedNotPunished(t, env, member)
	})

	t.Run("a member whose live record is an administrator", func(t *testing.T) {
		env := mreqJoinEnv(t, allMute, true)
		member := env.mreqMember("Alex", "", "")
		env.fake.setMember(env.chat.Id, member.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusAdministrator, CanRestrictMembers: true})

		env.mreqJoin(member)

		wantWelcomedNotPunished(t, env, member)
	})

	t.Run("a joiner whose live lookup fails", func(t *testing.T) {
		env := mreqJoinEnv(t, allMute, true)
		member := env.mreqMember("Alex", "", "")
		env.fake.script("getChatMember", env.chat.Id, staffFakeError(500, "Internal Server Error"))

		env.mreqJoin(member)

		wantWelcomedNotPunished(t, env, member)
	})
}

// TestMemberReqJoinNeverReplacesRestriction is CR-01 on the join path: a member an admin muted
// for good, who leaves and rejoins (restricted to restricted, so only the service message
// arrives), is not muted again by a Mute rule, and no notice or hold is made.
func TestMemberReqJoinNeverReplacesRestriction(t *testing.T) {
	env := mreqJoinEnv(t, map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionMute}, true)
	member := env.mreqMember("Alex", "", "alex")
	env.mreqRestrictedBySomeoneElse(member, 0)

	env.mreqServiceJoinNow(member)

	if n := env.mreqCallsAbout(member.Id, "restrictChatMember", "banChatMember", "unbanChatMember"); n != 0 {
		t.Errorf("restriction calls about the member = %d, want none", n)
	}
	if got := mreqCountText(env.mreqSentTexts(), staffMarker("mreq_notice_mute")); got != 0 {
		t.Errorf("mute notices = %d, want none", got)
	}
	if hold := env.mreqHold(member.Id); hold != nil {
		t.Errorf("hold = %+v, want none", hold)
	}
	rec := env.fake.member(env.chat.Id, member.Id)
	if rec == nil || rec.Status != gotgbot.ChatMemberStatusRestricted || rec.UntilDate != 0 {
		t.Errorf("member after the join = %+v, want the admin's restriction left as it was", rec)
	}
}
