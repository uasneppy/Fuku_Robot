//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"
	logrustest "github.com/sirupsen/logrus/hooks/test"

	"github.com/uasneppy/fuku_robot/fuku/db/approvals"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/captcha"
	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// mreqBitOf is the hold-mask bit of the rule with the given callback code.
func mreqBitOf(t *testing.T, code string) int {
	t.Helper()
	rule, ok := memberReqRuleByCode(code)
	if !ok {
		t.Fatalf("no rule with code %q", code)
	}
	return rule.Bit
}

// mreqLookupMember mints a member whose names break no name rule and whose photo answer in the
// lookup fake is the given one. Its users row, cache keys and fuku:memberreq:photo key are
// deleted when the test ends.
func mreqLookupMember(t *testing.T, env *memberReqEnv, state *mreqLookupState, photo mreqPhotoAnswer) gotgbot.User {
	t.Helper()
	member := env.mreqMember("Alex", "Smith", "alexsmith")
	mreqLookupCleanup(t, []int64{member.Id}, nil)
	state.set(member.Id, photo)
	return member
}

// mreqHasPhoto is the photo answer of a member who has one.
var mreqHasPhoto = mreqPhotoAnswer{Total: 1, List: true}

func TestMemberReqLookupPunishes(t *testing.T) {
	photoBit := func(t *testing.T) int { return mreqBitOf(t, "ph") }

	t.Run("photo rule Mute mutes a member with no photo with one hold and one notice", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RulePhoto: models.RequirementActionMute})
		member := mreqLookupMember(t, env, state, mreqPhotoAnswer{})

		got := env.mreqEnforce(env.mreqCase(member))
		if got.Outcome != memberReqMuted || got.Hold == nil {
			t.Fatalf("outcome = %+v, want Muted with the hold", got)
		}
		restricts := env.calls("restrictChatMember")
		if len(restricts) != 1 {
			t.Fatalf("restrict calls = %d, want 1", len(restricts))
		}
		hold := env.mreqHold(member.Id)
		if hold == nil || hold.Rules != photoBit(t) || hold.Action != models.RequirementActionMute || hold.PunishedAt == nil {
			t.Fatalf("hold = %+v, want a punished mute hold with the photo bit %d", hold, photoBit(t))
		}
		if until := staffParamInt(restricts[0].Params, "until_date"); until != hold.MuteUntil {
			t.Errorf("until_date = %d, hold mute_until = %d, want the marker on both", until, hold.MuteUntil)
		}
		notices := env.mreqNotices()
		if len(notices) != 1 {
			t.Fatalf("notices = %d, want exactly one", len(notices))
		}
		text := mreqText(notices[0])
		for _, want := range []string{staffMarker("mreq_notice_mute"), staffMarker("mreq_rule_photo")} {
			if !strings.Contains(text, want) {
				t.Errorf("notice %q does not contain %q", text, want)
			}
		}
		if data := mreqKeyboardData(notices[0]); len(data) != 1 {
			t.Errorf("mute notice buttons = %v, want the one I fixed it button", data)
		}
		if n := mreqLookupPhotoCalls(env); n != 1 {
			t.Errorf("getUserProfilePhotos requests = %d, want 1", n)
		}
	})

	t.Run("photo rule Warn warns a member with no photo", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RulePhoto: models.RequirementActionWarn})
		member := mreqLookupMember(t, env, state, mreqPhotoAnswer{})

		got := env.mreqEnforce(env.mreqCase(member))
		if got.Outcome != memberReqWarned || got.Hold == nil {
			t.Fatalf("outcome = %+v, want Warned with the hold", got)
		}
		row := env.mreqWarnRow(member.Id)
		if row == nil || row.NumWarns != 1 || len(row.Reasons) != 1 || !strings.Contains(row.Reasons[0], staffMarker("mreq_rule_photo")) {
			t.Fatalf("warns row = %+v, want one warn naming the photo rule", row)
		}
		if hold := env.mreqHold(member.Id); hold == nil || hold.Rules != photoBit(t) || hold.Action != models.RequirementActionWarn {
			t.Errorf("hold = %+v, want a warn hold with the photo bit", hold)
		}
		notices := env.mreqNotices()
		if len(notices) != 1 || !strings.Contains(mreqText(notices[0]), staffMarker("mreq_notice_warn")) ||
			!strings.Contains(mreqText(notices[0]), staffMarker("mreq_rule_photo")) {
			t.Fatalf("notices = %+v, want one warn notice naming the photo rule", notices)
		}
	})

	t.Run("photo rule Kick removes a member with no photo", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RulePhoto: models.RequirementActionKick})
		member := mreqLookupMember(t, env, state, mreqPhotoAnswer{})

		got := env.mreqEnforce(env.mreqCase(member))
		if got.Outcome != memberReqRemoved {
			t.Fatalf("outcome = %+v, want Removed", got)
		}
		unbans := env.calls("unbanChatMember")
		if len(unbans) != 1 || staffParamBool(unbans[0].Params, "only_if_banned") {
			t.Fatalf("unban calls = %+v, want one member-removing unban", unbans)
		}
		if n := len(env.mreqWrites()); n != 1 {
			t.Errorf("ban, restrict and unban calls = %d, want only the unban", n)
		}
		if hold := env.mreqHold(member.Id); hold == nil || hold.Rules != photoBit(t) || hold.Action != models.RequirementActionKick {
			t.Errorf("hold = %+v, want a kick hold with the photo bit", hold)
		}
		if notices := env.mreqNotices(); len(notices) != 1 || !strings.Contains(mreqText(notices[0]), staffMarker("mreq_notice_kick")) {
			t.Errorf("notices = %+v, want one kick notice", notices)
		}
	})

	t.Run("photo rule Ban bans a member with no photo", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RulePhoto: models.RequirementActionBan})
		member := mreqLookupMember(t, env, state, mreqPhotoAnswer{})

		got := env.mreqEnforce(env.mreqCase(member))
		if got.Outcome != memberReqRemoved {
			t.Fatalf("outcome = %+v, want Removed", got)
		}
		if bans := env.calls("banChatMember"); len(bans) != 1 {
			t.Fatalf("ban calls = %d, want 1", len(bans))
		}
		if hold := env.mreqHold(member.Id); hold == nil || hold.Rules != photoBit(t) || hold.Action != models.RequirementActionBan {
			t.Errorf("hold = %+v, want a ban hold with the photo bit", hold)
		}
		if notices := env.mreqNotices(); len(notices) != 1 || !strings.Contains(mreqText(notices[0]), staffMarker("mreq_notice_ban")) {
			t.Errorf("notices = %+v, want one ban notice", notices)
		}
	})

	t.Run("channel rule", mreqPunishesChannelSubtests)
}

func TestMemberReqLookupSpares(t *testing.T) {
	t.Run("a member with a photo is spared and the pass is cached", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RulePhoto: models.RequirementActionBan})
		member := mreqLookupMember(t, env, state, mreqHasPhoto)

		got := env.mreqEnforce(env.mreqCase(member))
		if got.Outcome != memberReqNone || got.Hold != nil {
			t.Fatalf("outcome = %+v, want None with no hold", got)
		}
		if hold := env.mreqHold(member.Id); hold != nil {
			t.Errorf("hold = %+v, want none", hold)
		}
		if n := env.mreqCallCount("restrictChatMember", "banChatMember", "unbanChatMember", "sendMessage"); n != 0 {
			t.Errorf("writes and messages = %d, want none", n)
		}
		if row := env.mreqWarnRow(member.Id); row != nil {
			t.Errorf("warns row = %+v, want none", row)
		}
		if !env.redis.Exists(memberReqPhotoKey(member.Id)) {
			t.Errorf("%s does not exist, want the cached pass", memberReqPhotoKey(member.Id))
		}

		before := mreqLookupPhotoCalls(env)
		again := env.mreqEnforce(env.mreqCase(member))
		if again.Outcome != memberReqNone {
			t.Fatalf("second outcome = %+v, want None", again)
		}
		if after := mreqLookupPhotoCalls(env); after != before {
			t.Errorf("getUserProfilePhotos requests after the second enforcement = %d, want still %d (the cached pass)", after, before)
		}
	})

	t.Run("a member with a photo is spared under every punishment", func(t *testing.T) {
		for _, action := range []int{models.RequirementActionWarn, models.RequirementActionKick, models.RequirementActionMute} {
			t.Run(fmt.Sprint("action ", action), func(t *testing.T) {
				env, state := mreqLookupEnv(t)
				env.mreqRules(map[memberreq.Rule]int{memberreq.RulePhoto: action})
				member := mreqLookupMember(t, env, state, mreqHasPhoto)
				if got := env.mreqEnforce(env.mreqCase(member)); got.Outcome != memberReqNone {
					t.Fatalf("outcome = %+v, want None", got)
				}
				if n := env.mreqCallCount("restrictChatMember", "banChatMember", "unbanChatMember", "sendMessage"); n != 0 {
					t.Errorf("writes and messages = %d, want none", n)
				}
			})
		}
	})
}

// mreqChannelUser mints a member (names that break no name rule, a photo) of a group linked to
// a fresh channel where the bot has botStatus and the member has memberStatus. It returns the
// member and the channel ID.
func mreqChannelUser(t *testing.T, env *memberReqEnv, state *mreqLookupState, botStatus, memberStatus string) (gotgbot.User, int64) {
	t.Helper()
	channel := mreqChannel(t, state, env.chat.Id, botStatus)
	member := mreqLookupMember(t, env, state, mreqHasPhoto)
	state.setMember(channel, member.Id, mreqChannelMember{Status: memberStatus})
	return member, channel
}

// mreqWantNothing fails the test when the executor left anything behind for the member: an
// outcome, a hold, a warn row, a Telegram write or a message to the group.
func mreqWantNothing(t *testing.T, env *memberReqEnv, member gotgbot.User, got memberReqResult) {
	t.Helper()
	if got.Outcome != memberReqNone || got.Hold != nil {
		t.Errorf("outcome = %+v, want None with no hold", got)
	}
	if hold := env.mreqHold(member.Id); hold != nil {
		t.Errorf("hold = %+v, want none", hold)
	}
	if row := env.mreqWarnRow(member.Id); row != nil {
		t.Errorf("warns row = %+v, want none", row)
	}
	if n := len(env.mreqWrites()); n != 0 {
		t.Errorf("ban, restrict and unban calls = %d, want none", n)
	}
	for _, text := range env.mreqSentTexts() {
		if strings.Contains(text, "mreq_notice") {
			t.Errorf("a requirement notice was sent: %q", text)
		}
	}
}

// mreqPunishesChannelSubtests are the channel half of TestMemberReqLookupPunishes.
func mreqPunishesChannelSubtests(t *testing.T) {
	channelBit := func(t *testing.T) int { return mreqBitOf(t, "ch") }
	for _, tc := range []struct {
		name    string
		action  int
		outcome memberReqOutcome
		method  string
	}{
		{"Warn", models.RequirementActionWarn, memberReqWarned, ""},
		{"Kick", models.RequirementActionKick, memberReqRemoved, "unbanChatMember"},
		{"Ban", models.RequirementActionBan, memberReqRemoved, "banChatMember"},
		{"Mute", models.RequirementActionMute, memberReqMuted, "restrictChatMember"},
	} {
		t.Run("channel rule "+tc.name+" punishes a member who left the channel", func(t *testing.T) {
			env, state := mreqLookupEnv(t)
			env.mreqRules(map[memberreq.Rule]int{memberreq.RuleChannel: tc.action})
			member, _ := mreqChannelUser(t, env, state, gotgbot.ChatMemberStatusAdministrator, gotgbot.ChatMemberStatusLeft)

			got := env.mreqEnforce(env.mreqCase(member))
			if got.Outcome != tc.outcome || got.Hold == nil {
				t.Fatalf("outcome = %+v, want %d with the hold", got, tc.outcome)
			}
			if hold := env.mreqHold(member.Id); hold == nil || hold.Rules != channelBit(t) || hold.Action != tc.action {
				t.Errorf("hold = %+v, want action %d with the channel bit %d", hold, tc.action, channelBit(t))
			}
			if tc.method != "" && env.mreqCallCount(tc.method) != 1 {
				t.Errorf("%s calls = %d, want 1", tc.method, env.mreqCallCount(tc.method))
			}
			notices := env.mreqNotices()
			if len(notices) != 1 || !strings.Contains(mreqText(notices[0]), staffMarker("mreq_rule_channel")) {
				t.Fatalf("notices = %+v, want one naming the channel rule", notices)
			}
		})
	}

	t.Run("a subscribed member is spared", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleChannel: models.RequirementActionBan})
		member, _ := mreqChannelUser(t, env, state, gotgbot.ChatMemberStatusAdministrator, gotgbot.ChatMemberStatusMember)
		mreqWantNothing(t, env, member, env.mreqEnforce(env.mreqCase(member)))
	})

	t.Run("a bot that does not administer the channel spares the member (D-22)", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleChannel: models.RequirementActionBan})
		member, _ := mreqChannelUser(t, env, state, gotgbot.ChatMemberStatusMember, gotgbot.ChatMemberStatusLeft)
		mreqWantNothing(t, env, member, env.mreqEnforce(env.mreqCase(member)))
	})

	t.Run("a group with no linked channel punishes nobody", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleChannel: models.RequirementActionBan})
		state.link(env.chat.Id, 0)
		mreqLookupCleanup(t, nil, []int64{env.chat.Id})
		member := mreqLookupMember(t, env, state, mreqPhotoAnswer{})
		mreqWantNothing(t, env, member, env.mreqEnforce(env.mreqCase(member)))
	})
}

// mreqUnanswerable is one way a lookup cannot be answered. prepare scripts the fake for a
// member who, answered definitely, would be a breach of the rule; hangs makes the case's
// budgets and flight timeout short because the lookup never answers.
type mreqUnanswerable struct {
	name    string
	rule    memberreq.Rule
	hangs   bool
	prepare func(t *testing.T, env *memberReqEnv, state *mreqLookupState, group, channel, user int64)
}

func mreqUnanswerableShapes() []mreqUnanswerable {
	errPlain := errors.New("connection reset by peer")
	telegram := func(code int) error { return staffFakeError(code, fmt.Sprintf("test error %d", code)) }
	persistent429 := []error{staffFake429(1), staffFake429(1), staffFake429(1), staffFake429(1)}
	lookupBlock := func(t *testing.T, env *memberReqEnv) {
		t.Helper()
		const blockKey = "fuku:memberreq:lkp:pace:block"
		mreqSetKey(t, env, blockKey, 30*time.Second)
		t.Cleanup(func() { env.redis.Del(blockKey) })
	}
	stopped := func(t *testing.T, _ *memberReqEnv, _ *mreqLookupState, _, _, _ int64) {
		t.Helper()
		restoreMemberReqRunner(t)
		StopMemberReqJobs()
	}
	photo := func(name string, prepare func(t *testing.T, env *memberReqEnv, state *mreqLookupState, user int64)) mreqUnanswerable {
		return mreqUnanswerable{name: "photo: " + name, rule: memberreq.RulePhoto,
			prepare: func(t *testing.T, env *memberReqEnv, state *mreqLookupState, _, _, user int64) {
				t.Helper()
				prepare(t, env, state, user)
			}}
	}
	channel := func(name string, hangs bool, prepare func(t *testing.T, env *memberReqEnv, state *mreqLookupState, group, channel, user int64)) mreqUnanswerable {
		return mreqUnanswerable{name: "channel: " + name, rule: memberreq.RuleChannel, hangs: hangs, prepare: prepare}
	}
	shapes := []mreqUnanswerable{
		photo("a plain error", func(_ *testing.T, _ *memberReqEnv, s *mreqLookupState, u int64) { s.script(u, errPlain) }),
		photo("a 400", func(_ *testing.T, _ *memberReqEnv, s *mreqLookupState, u int64) { s.script(u, telegram(400)) }),
		photo("a 403", func(_ *testing.T, _ *memberReqEnv, s *mreqLookupState, u int64) { s.script(u, telegram(403)) }),
		photo("a 429 past the retry", func(_ *testing.T, _ *memberReqEnv, s *mreqLookupState, u int64) { s.script(u, persistent429...) }),
		photo("a 500", func(_ *testing.T, _ *memberReqEnv, s *mreqLookupState, u int64) { s.script(u, telegram(500)) }),
		photo("a null body", func(_ *testing.T, _ *memberReqEnv, s *mreqLookupState, u int64) {
			s.set(u, mreqPhotoAnswer{Null: true})
		}),
		photo("a pacer shed by a long block", func(t *testing.T, e *memberReqEnv, _ *mreqLookupState, _ int64) { lookupBlock(t, e) }),
		{name: "photo: a timeout", rule: memberreq.RulePhoto, hangs: true,
			prepare: func(t *testing.T, _ *memberReqEnv, s *mreqLookupState, _, _, user int64) {
				t.Helper()
				t.Cleanup(s.hold(user))
			}},
		{name: "photo: the shutdown root cancelled", rule: memberreq.RulePhoto, prepare: stopped},

		channel("getChat plain error", false, func(_ *testing.T, _ *memberReqEnv, s *mreqLookupState, g, _, _ int64) { s.scriptChat(g, errPlain) }),
		channel("getChat 400", false, func(_ *testing.T, _ *memberReqEnv, s *mreqLookupState, g, _, _ int64) { s.scriptChat(g, telegram(400)) }),
		channel("getChat 403", false, func(_ *testing.T, _ *memberReqEnv, s *mreqLookupState, g, _, _ int64) { s.scriptChat(g, telegram(403)) }),
		channel("getChat 429 past the retry", false, func(_ *testing.T, _ *memberReqEnv, s *mreqLookupState, g, _, _ int64) {
			s.scriptChat(g, persistent429...)
		}),
		channel("getChat 500", false, func(_ *testing.T, _ *memberReqEnv, s *mreqLookupState, g, _, _ int64) { s.scriptChat(g, telegram(500)) }),
		channel("getChat null body", false, func(_ *testing.T, _ *memberReqEnv, s *mreqLookupState, g, _, _ int64) { s.nullGetChat(g) }),
		channel("getChat timeout", true, func(t *testing.T, _ *memberReqEnv, s *mreqLookupState, g, _, _ int64) {
			t.Helper()
			t.Cleanup(s.holdKey(mreqChannelKey("getChat", g, 0)))
		}),
		channel("member plain error", false, func(_ *testing.T, _ *memberReqEnv, s *mreqLookupState, _, c, u int64) { s.scriptMember(c, u, errPlain) }),
		channel("member 400", false, func(_ *testing.T, _ *memberReqEnv, s *mreqLookupState, _, c, u int64) {
			s.scriptMember(c, u, telegram(400))
		}),
		channel("member 403", false, func(_ *testing.T, _ *memberReqEnv, s *mreqLookupState, _, c, u int64) {
			s.scriptMember(c, u, telegram(403))
		}),
		channel("member 429 past the retry", false, func(_ *testing.T, _ *memberReqEnv, s *mreqLookupState, _, c, u int64) {
			s.scriptMember(c, u, persistent429...)
		}),
		channel("member 500", false, func(_ *testing.T, _ *memberReqEnv, s *mreqLookupState, _, c, u int64) {
			s.scriptMember(c, u, telegram(500))
		}),
		channel("member null body", false, func(_ *testing.T, _ *memberReqEnv, s *mreqLookupState, _, c, u int64) {
			s.setMember(c, u, mreqChannelMember{Null: true})
		}),
		channel("member unrecognised status", false, func(_ *testing.T, _ *memberReqEnv, s *mreqLookupState, _, c, u int64) {
			s.setMember(c, u, mreqChannelMember{Status: "something_new"})
		}),
		channel("member nil status", false, func(_ *testing.T, _ *memberReqEnv, s *mreqLookupState, _, c, u int64) {
			s.setMember(c, u, mreqChannelMember{Status: ""})
		}),
		channel("member timeout", true, func(t *testing.T, _ *memberReqEnv, s *mreqLookupState, _, c, u int64) {
			t.Helper()
			t.Cleanup(s.holdKey(mreqChannelKey("getChatMember", c, u)))
		}),
		channel("the bot's own standing cannot be read", false, func(_ *testing.T, _ *memberReqEnv, s *mreqLookupState, _, c, _ int64) {
			s.scriptMember(c, staffTestBotID, telegram(500))
		}),
		channel("a pacer shed by a long block", false, func(t *testing.T, e *memberReqEnv, _ *mreqLookupState, _, _, _ int64) { lookupBlock(t, e) }),
		channel("the shutdown root cancelled", false, stopped),
	}
	return shapes
}

// mreqUnanswerableEnv builds the environment of one shape for the job path or the join entry,
// with the rule stored at action, the lookup fake installed and, for a channel rule, a linked
// channel the bot administers and a member who is not in it (so a definite answer is a breach).
func mreqUnanswerableEnv(t *testing.T, shape mreqUnanswerable, action int, join bool) (*memberReqEnv, gotgbot.User) {
	t.Helper()
	var env *memberReqEnv
	if join {
		env = mreqJoinEnv(t, map[memberreq.Rule]int{shape.rule: action}, true)
	} else {
		env = newMemberReqEnv(t)
		env.mreqRules(map[memberreq.Rule]int{shape.rule: action})
	}
	withFastLookupPacer(t)
	state := newMreqLookupState()
	mreqInstallLookupFake(env, state)
	if shape.hangs {
		mreqSwap(t, &memberReqLookupJobBudget, 150*time.Millisecond)
		mreqSwap(t, &memberReqLookupJoinBudget, 150*time.Millisecond)
		mreqSwap(t, &memberReqLookupFlightTimeout, 600*time.Millisecond)
		// Runs before the row cleanups: the abandoned flight has ended by its own timeout.
		t.Cleanup(func() { time.Sleep(700 * time.Millisecond) })
	}
	var member gotgbot.User
	var channel int64
	if shape.rule == memberreq.RuleChannel {
		member, channel = mreqChannelUser(t, env, state, gotgbot.ChatMemberStatusAdministrator, gotgbot.ChatMemberStatusLeft)
	} else {
		member = mreqLookupMember(t, env, state, mreqPhotoAnswer{})
	}
	shape.prepare(t, env, state, env.chat.Id, channel, member.Id)
	return env, member
}

func TestMemberReqLookupUnanswerable(t *testing.T) {
	actions := []int{models.RequirementActionMute, models.RequirementActionWarn, models.RequirementActionKick, models.RequirementActionBan}
	for i, shape := range mreqUnanswerableShapes() {
		action := actions[i%len(actions)]
		t.Run("job: "+shape.name, func(t *testing.T) {
			env, member := mreqUnanswerableEnv(t, shape, action, false)
			started := time.Now()
			got := env.mreqEnforce(env.mreqCase(member))
			mreqWantNothing(t, env, member, got)
			if shape.hangs {
				if took := time.Since(started); took > 3*time.Second {
					t.Errorf("a hung lookup held the job for %s, want about the 150 ms budget", took)
				}
			}
		})
		t.Run("join: "+shape.name, func(t *testing.T) {
			env, member := mreqUnanswerableEnv(t, shape, action, true)
			started := time.Now()
			env.mreqJoin(member)
			if shape.hangs {
				if took := time.Since(started); took > 3*time.Second {
					t.Errorf("a hung lookup held the join entry for %s, want about the 150 ms budget", took)
				}
			}
			mreqWantNothing(t, env, member, memberReqResult{})
			if got := mreqCountText(env.mreqSentTexts(), mreqWelcomeWord); got != 1 {
				t.Errorf("welcomes = %d, want 1 (a joiner whose lookup cannot be answered is welcomed)", got)
			}
		})
	}
}

func TestMemberReqLookupDoesNotBlockNameBreach(t *testing.T) {
	noLastName := func(t *testing.T, env *memberReqEnv, state *mreqLookupState, photo mreqPhotoAnswer) gotgbot.User {
		t.Helper()
		member := env.mreqMember("Alex", "", "alex")
		mreqLookupCleanup(t, []int64{member.Id}, nil)
		state.set(member.Id, photo)
		return member
	}
	noticeText := func(t *testing.T, env *memberReqEnv) string {
		t.Helper()
		notices := env.mreqNotices()
		if len(notices) != 1 {
			t.Fatalf("notices = %d, want exactly one", len(notices))
		}
		return mreqText(notices[0])
	}

	t.Run("an unanswerable photo lookup leaves the name breach punished and the episode partial", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionMute, memberreq.RulePhoto: models.RequirementActionWarn})
		member := noLastName(t, env, state, mreqPhotoAnswer{})
		state.script(member.Id, staffFakeError(500, "Internal Server Error"))

		got := env.mreqEnforce(env.mreqCase(member))
		if got.Outcome != memberReqMuted {
			t.Fatalf("outcome = %+v, want Muted for the last name rule", got)
		}
		if hold := env.mreqHold(member.Id); hold == nil || hold.Rules != mreqBitOf(t, "ln") || hold.Action != models.RequirementActionMute {
			t.Errorf("hold = %+v, want a mute hold with the last name bit only", hold)
		}
		text := noticeText(t, env)
		if !strings.Contains(text, staffMarker("mreq_rule_lastname")) || strings.Contains(text, staffMarker("mreq_rule_photo")) {
			t.Errorf("notice %q should name the last name rule and not the photo rule", text)
		}
		if row := env.mreqWarnRow(member.Id); row != nil {
			t.Errorf("warns row = %+v, want none (the photo Warn was never established)", row)
		}
	})

	t.Run("an unanswerable channel lookup leaves the name breach punished", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionKick, memberreq.RuleChannel: models.RequirementActionBan})
		channel := mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
		member := noLastName(t, env, state, mreqHasPhoto)
		state.setMember(channel, member.Id, mreqChannelMember{Status: gotgbot.ChatMemberStatusLeft})
		state.scriptChat(env.chat.Id, staffFakeError(500, "Internal Server Error"))

		got := env.mreqEnforce(env.mreqCase(member))
		if got.Outcome != memberReqRemoved {
			t.Fatalf("outcome = %+v, want Removed (kick) for the last name rule, not the channel ban", got)
		}
		if hold := env.mreqHold(member.Id); hold == nil || hold.Rules != mreqBitOf(t, "ln") || hold.Action != models.RequirementActionKick {
			t.Errorf("hold = %+v, want a kick hold with the last name bit only", hold)
		}
		if n := env.mreqCallCount("banChatMember"); n != 0 {
			t.Errorf("ban calls = %d, want none", n)
		}
	})

	t.Run("a name breach and a photo breach make one hold with both bits, the strictest action and both phrases in rule order", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionMute, memberreq.RulePhoto: models.RequirementActionBan})
		member := noLastName(t, env, state, mreqPhotoAnswer{})

		got := env.mreqEnforce(env.mreqCase(member))
		if got.Outcome != memberReqRemoved {
			t.Fatalf("outcome = %+v, want Removed: the photo rule's Ban is stricter than the last name Mute", got)
		}
		if hold := env.mreqHold(member.Id); hold == nil || hold.Rules != mreqBitOf(t, "ln")|mreqBitOf(t, "ph") || hold.Action != models.RequirementActionBan {
			t.Errorf("hold = %+v, want a ban hold with both bits", hold)
		}
		text := noticeText(t, env)
		name, photo := strings.Index(text, staffMarker("mreq_rule_lastname")), strings.Index(text, staffMarker("mreq_rule_photo"))
		if name < 0 || photo < 0 || name > photo {
			t.Errorf("notice %q should list the last name phrase and then the photo phrase", text)
		}
		if n := env.mreqCallCount("banChatMember", "restrictChatMember"); n != 1 {
			t.Errorf("ban and restrict calls = %d, want exactly the one ban", n)
		}
	})
}

// mreqArrivalProbe watches the lookup fake: the order of its requests and, as each one
// arrives, whether the member already has a hold and what the chat's cap counter says.
type mreqArrivalProbe struct {
	mu       sync.Mutex
	events   []string
	holdSeen []bool
	capSeen  []int
}

func (p *mreqArrivalProbe) add(event string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, event)
}

func (p *mreqArrivalProbe) log() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.events...)
}

// mreqProbeLookups installs hooks that log every lookup request as "photo" or "getChat" or
// "getChatMember" and note the member's hold and the cap counter at each photo request.
func mreqProbeLookups(env *memberReqEnv, state *mreqLookupState) *mreqArrivalProbe {
	probe := &mreqArrivalProbe{}
	chat := env.chat.Id
	state.setOnPhoto(func(user int64) {
		hold, err := memberreq.GetHoldFresh(context.Background(), chat, user)
		counted := 0
		for _, key := range env.mreqCapKeys(chat) {
			if value, getErr := env.redis.Get(key); getErr == nil {
				n, _ := strconv.Atoi(value)
				counted += n
			}
		}
		probe.mu.Lock()
		probe.holdSeen = append(probe.holdSeen, err != nil || hold != nil)
		probe.capSeen = append(probe.capSeen, counted)
		probe.events = append(probe.events, "photo")
		probe.mu.Unlock()
	})
	state.setOnChannel(func(method string, _, _ int64) { probe.add(method) })
	return probe
}

func TestMemberReqLookupOrder(t *testing.T) {
	t.Run("the photo request comes before the channel requests and both bits are on the hold", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RulePhoto: models.RequirementActionMute, memberreq.RuleChannel: models.RequirementActionMute})
		channel := mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
		member := mreqLookupMember(t, env, state, mreqPhotoAnswer{})
		state.setMember(channel, member.Id, mreqChannelMember{Status: gotgbot.ChatMemberStatusLeft})
		probe := mreqProbeLookups(env, state)

		if got := env.mreqEnforce(env.mreqCase(member)); got.Outcome != memberReqMuted {
			t.Fatalf("outcome = %+v, want Muted", got)
		}
		events := probe.log()
		if len(events) < 3 || events[0] != "photo" {
			t.Fatalf("lookup requests in order = %v, want the photo request first and then the channel requests", events)
		}
		for _, event := range events[1:] {
			if event == "photo" {
				t.Errorf("lookup requests in order = %v, want one photo request", events)
			}
		}
		if hold := env.mreqHold(member.Id); hold == nil || hold.Rules != mreqBitOf(t, "ph")|mreqBitOf(t, "ch") {
			t.Errorf("hold = %+v, want both lookup bits", hold)
		}
		text := mreqText(env.mreqNotices()[0])
		photo, ch := strings.Index(text, staffMarker("mreq_rule_photo")), strings.Index(text, staffMarker("mreq_rule_channel"))
		if photo < 0 || ch < 0 || photo > ch {
			t.Errorf("notice %q should name the photo rule and then the channel rule", text)
		}
	})

	t.Run("a rule whose action is Off makes no request of its own", func(t *testing.T) {
		t.Run("photo on, channel Off", func(t *testing.T) {
			env, state := mreqLookupEnv(t)
			env.mreqRules(map[memberreq.Rule]int{memberreq.RulePhoto: models.RequirementActionWarn})
			channel := mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
			member := mreqLookupMember(t, env, state, mreqPhotoAnswer{})
			state.setMember(channel, member.Id, mreqChannelMember{Status: gotgbot.ChatMemberStatusLeft})

			if got := env.mreqEnforce(env.mreqCase(member)); got.Outcome != memberReqWarned {
				t.Fatalf("outcome = %+v, want Warned", got)
			}
			if n := mreqChatCalls(env, env.chat.Id); n != 0 {
				t.Errorf("getChat requests of the group = %d, want 0 with the channel rule Off", n)
			}
			if n := mreqMemberCalls(env, channel, member.Id) + mreqMemberCalls(env, channel, staffTestBotID); n != 0 {
				t.Errorf("channel getChatMember requests = %d, want 0 with the channel rule Off", n)
			}
		})
		t.Run("channel on, photo Off", func(t *testing.T) {
			env, state := mreqLookupEnv(t)
			env.mreqRules(map[memberreq.Rule]int{memberreq.RuleChannel: models.RequirementActionWarn})
			member, _ := mreqChannelUser(t, env, state, gotgbot.ChatMemberStatusAdministrator, gotgbot.ChatMemberStatusLeft)
			state.set(member.Id, mreqPhotoAnswer{})

			if got := env.mreqEnforce(env.mreqCase(member)); got.Outcome != memberReqWarned {
				t.Fatalf("outcome = %+v, want Warned", got)
			}
			if n := mreqLookupPhotoCalls(env); n != 0 {
				t.Errorf("getUserProfilePhotos requests = %d, want 0 with the photo rule Off", n)
			}
		})
	})

	t.Run("an exempt member makes no lookup request", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RulePhoto: models.RequirementActionMute, memberreq.RuleChannel: models.RequirementActionMute})
		channel := mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
		probe := mreqProbeLookups(env, state)
		fresh := func() gotgbot.User {
			member := mreqLookupMember(t, env, state, mreqPhotoAnswer{})
			state.setMember(channel, member.Id, mreqChannelMember{Status: gotgbot.ChatMemberStatusLeft})
			return member
		}
		want := func(t *testing.T, name string, user gotgbot.User) {
			t.Helper()
			mreqWantNothing(t, env, user, env.mreqEnforce(env.mreqCase(user)))
			if events := probe.log(); len(events) != 0 {
				t.Errorf("%s: lookup requests = %v, want none", name, events)
			}
		}

		approved := fresh()
		if err := approvals.AddApprovedUser(env.chat.Id, approved.Id, env.admin.Id, "trusted"); err != nil {
			t.Fatal(err)
		}
		want(t, "an approved member", approved)

		pending := fresh()
		if _, err := captcha.CreateCaptchaAttemptPreMessage(pending.Id, env.chat.Id, "answer", 5); err != nil {
			t.Fatal(err)
		}
		dbcache.DeleteCache(dbcache.CacheKey("captcha_pending", env.chat.Id))
		want(t, "a member with a pending captcha", pending)

		admin := fresh()
		env.fake.setMember(env.chat.Id, admin.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusAdministrator, CanRestrictMembers: true})
		want(t, "a live administrator", admin)

		for _, id := range []int64{env.bot.Id, 777000, 1087968824, 136817688} {
			want(t, fmt.Sprintf("the bot or a service account %d", id), gotgbot.User{Id: id, FirstName: "Service"})
		}
	})

	t.Run("a held member whose hold carries name bits only makes no lookup request", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionMute})
		member := env.mreqMember("Alex", "", "alex")
		mreqLookupCleanup(t, []int64{member.Id}, nil)
		if got := env.mreqEnforce(env.mreqCase(member)); got.Outcome != memberReqMuted {
			t.Fatalf("first outcome = %+v, want Muted", got)
		}
		env.mreqRules(map[memberreq.Rule]int{memberreq.RulePhoto: models.RequirementActionMute})
		probe := mreqProbeLookups(env, state)

		if got := env.mreqEnforce(env.mreqCase(member)); got.Outcome != memberReqHeld {
			t.Fatalf("second outcome = %+v, want Held", got)
		}
		if events := probe.log(); len(events) != 0 {
			t.Errorf("lookup requests for a held member = %v, want none", events)
		}
	})

	t.Run("the lookups run before the claim and the cap, which follow them", func(t *testing.T) {
		mreqFixedClock(t)
		env, state := mreqLookupEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RulePhoto: models.RequirementActionMute})
		member := mreqLookupMember(t, env, state, mreqPhotoAnswer{})
		probe := mreqProbeLookups(env, state)

		if got := env.mreqEnforce(env.mreqCase(member)); got.Outcome != memberReqMuted {
			t.Fatalf("outcome = %+v, want Muted", got)
		}
		probe.mu.Lock()
		holdSeen, capSeen := append([]bool(nil), probe.holdSeen...), append([]int(nil), probe.capSeen...)
		probe.mu.Unlock()
		if len(holdSeen) != 1 || holdSeen[0] {
			t.Errorf("a hold existed when the photo request arrived: %v, want none", holdSeen)
		}
		if len(capSeen) != 1 || capSeen[0] != 0 {
			t.Errorf("the cap counter at the photo request = %v, want 0 (counted after the lookups)", capSeen)
		}
		if keys := env.mreqCapKeys(env.chat.Id); len(keys) != 1 {
			t.Errorf("cap counters after the punishment = %v, want one", keys)
		}
	})

	t.Run("a paused chat makes no lookup request", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RulePhoto: models.RequirementActionMute})
		member := mreqLookupMember(t, env, state, mreqPhotoAnswer{})
		mreqSetKey(t, env, memberReqPausedKey(env.chat.Id), memberReqPause)
		probe := mreqProbeLookups(env, state)

		mreqWantNothing(t, env, member, env.mreqEnforce(env.mreqCase(member)))
		if events := probe.log(); len(events) != 0 {
			t.Errorf("lookup requests in a paused chat = %v, want none", events)
		}
	})
}

func TestMemberReqLookupNeverReplacesRestriction(t *testing.T) {
	for _, tc := range []struct {
		name  string
		until int64
	}{
		{"a partial restriction with no end date", 0},
		{"a restriction an admin gave an end date", time.Now().Add(48 * time.Hour).Unix()},
	} {
		t.Run("a Mute for the photo rule places no mute over "+tc.name, func(t *testing.T) {
			hook := logrustest.NewGlobal()
			t.Cleanup(hook.Reset)
			env, state := mreqLookupEnv(t)
			env.mreqRules(map[memberreq.Rule]int{memberreq.RulePhoto: models.RequirementActionMute})
			member := mreqLookupMember(t, env, state, mreqPhotoAnswer{})
			env.mreqRestrictedBySomeoneElse(member, tc.until)

			got := env.mreqEnforce(env.mreqCase(member))
			if got.Outcome != memberReqNone || got.Hold != nil {
				t.Fatalf("outcome = %+v, want None with no hold", got)
			}
			if n := mreqLookupPhotoCalls(env); n != 1 {
				t.Errorf("photo requests = %d, want 1 (the lookup comes before the restriction check)", n)
			}
			if n := env.mreqCallCount("restrictChatMember", "banChatMember", "unbanChatMember", "deleteMessage", "sendMessage"); n != 0 {
				t.Errorf("writes, deletes and messages = %d, want none", n)
			}
			if hold := env.mreqHold(member.Id); hold != nil {
				t.Errorf("hold = %+v, want none", hold)
			}
			rec := env.fake.member(env.chat.Id, member.Id)
			if rec == nil || rec.Status != gotgbot.ChatMemberStatusRestricted || rec.UntilDate != tc.until {
				t.Errorf("member after the case = %+v, want the restriction left as it was (until %d)", rec, tc.until)
			}
			var infos []string
			for _, entry := range hook.AllEntries() {
				if entry.Level == log.InfoLevel && strings.Contains(entry.Message, "[MemberReq]") {
					infos = append(infos, entry.Message)
				}
			}
			if len(infos) != 1 {
				t.Fatalf("Info lines = %q, want exactly one", infos)
			}
			for _, want := range []string{fmt.Sprint(env.chat.Id), fmt.Sprint(member.Id)} {
				if !strings.Contains(infos[0], want) {
					t.Errorf("Info line %q does not hold %s", infos[0], want)
				}
			}
			for _, secret := range []string{"Alex", "alexsmith"} {
				if strings.Contains(infos[0], secret) {
					t.Errorf("Info line %q holds the member's name", infos[0])
				}
			}
		})
	}

	for _, tc := range []struct {
		name    string
		action  int
		outcome memberReqOutcome
	}{
		{"Warn", models.RequirementActionWarn, memberReqWarned},
		{"Kick", models.RequirementActionKick, memberReqRemoved},
		{"Ban", models.RequirementActionBan, memberReqRemoved},
	} {
		t.Run(tc.name+" for the photo rule still acts on a restricted member", func(t *testing.T) {
			env, state := mreqLookupEnv(t)
			env.mreqRules(map[memberreq.Rule]int{memberreq.RulePhoto: tc.action})
			member := mreqLookupMember(t, env, state, mreqPhotoAnswer{})
			env.mreqRestrictedBySomeoneElse(member, 0)

			got := env.mreqEnforce(env.mreqCase(member))
			if got.Outcome != tc.outcome || got.Hold == nil {
				t.Fatalf("outcome = %+v, want %d with a hold", got, tc.outcome)
			}
			if n := len(env.mreqNotices()); n != 1 {
				t.Errorf("notices = %d, want one", n)
			}
			if tc.action == models.RequirementActionWarn {
				if n := env.mreqCallCount("restrictChatMember", "banChatMember", "unbanChatMember"); n != 0 {
					t.Errorf("a warn made %d restriction calls, want none", n)
				}
			}
		})
	}
}

func TestMemberReqLookupCapAndPause(t *testing.T) {
	mreqFixedClock(t)
	env, state := mreqLookupEnv(t)
	env.mreqRules(map[memberreq.Rule]int{memberreq.RulePhoto: models.RequirementActionKick})
	probe := mreqProbeLookups(env, state)
	limit := memberReqCapPerMinute

	members := make([]gotgbot.User, limit+2)
	for i := range members {
		members[i] = mreqLookupMember(t, env, state, mreqPhotoAnswer{})
	}
	for i := 0; i < limit; i++ {
		if got := env.mreqEnforce(env.mreqCase(members[i])); got.Outcome != memberReqRemoved {
			t.Fatalf("member %d = %+v, want Removed", i, got)
		}
	}
	if env.redis.Exists(memberReqPausedKey(env.chat.Id)) {
		t.Fatal("the pause latch is set before the cap was exceeded")
	}

	over := members[limit]
	if got := env.mreqEnforce(env.mreqCase(over)); got.Outcome != memberReqNone {
		t.Fatalf("the member over the limit = %+v, want None", got)
	}
	if env.mreqHold(over.Id) != nil || env.mreqCallsAbout(over.Id, "unbanChatMember") != 0 {
		t.Error("the member over the cap was held or removed")
	}
	if !env.redis.Exists(memberReqPausedKey(env.chat.Id)) {
		t.Fatal("the cap did not start the pause")
	}
	if n := mreqPausedNotices(env); n != 1 {
		t.Errorf("pause notices = %d, want exactly one", n)
	}
	probe.mu.Lock()
	capSeen := append([]int(nil), probe.capSeen...)
	probe.mu.Unlock()
	if len(capSeen) != limit+1 {
		t.Fatalf("photo requests = %d, want %d (the member over the cap was looked up too)", len(capSeen), limit+1)
	}
	for i, seen := range capSeen {
		if seen != i {
			t.Errorf("the cap counter at photo request %d = %d, want %d (a member is counted after their lookups)", i, seen, i)
		}
	}

	if got := env.mreqEnforce(env.mreqCase(members[limit+1])); got.Outcome != memberReqNone {
		t.Fatalf("a member during the pause = %+v, want None", got)
	}
	if after := len(probe.log()); after != limit+1 {
		t.Errorf("lookup requests after the pause = %d, want still %d (a paused chat makes none)", after, limit+1)
	}
	if env.mreqHold(members[limit+1].Id) != nil {
		t.Error("a member during the pause was held")
	}
}

// mreqLookupJoinEnv is a join environment (Check on join on, a recognisable welcome) whose bot
// answers the lookups from a fresh shared state, with the lookup pacer made fast.
func mreqLookupJoinEnv(t *testing.T, rules map[memberreq.Rule]int) (*memberReqEnv, *mreqLookupState) {
	t.Helper()
	env := mreqJoinEnv(t, rules, true)
	withFastLookupPacer(t)
	state := newMreqLookupState()
	mreqInstallLookupFake(env, state)
	return env, state
}

func TestMemberReqJoinLookups(t *testing.T) {
	t.Run("a joiner with no photo is muted, the welcome is held and the hold records it", func(t *testing.T) {
		env, state := mreqLookupJoinEnv(t, map[memberreq.Rule]int{memberreq.RulePhoto: models.RequirementActionMute})
		member := mreqLookupMember(t, env, state, mreqPhotoAnswer{})

		env.mreqJoin(member)

		texts := env.mreqSentTexts()
		if got := mreqCountText(texts, mreqWelcomeWord); got != 0 {
			t.Fatalf("welcomes = %d, want none for a muted joiner (texts %v)", got, texts)
		}
		if got := mreqCountText(texts, staffMarker("mreq_notice_mute")); got != 1 {
			t.Fatalf("mute notices = %d, want exactly one (texts %v)", got, texts)
		}
		hold := env.mreqHold(member.Id)
		if hold == nil || hold.Action != models.RequirementActionMute || hold.PunishedAt == nil || !hold.WelcomeHeld || hold.Rules != mreqBitOf(t, "ph") {
			t.Fatalf("hold = %+v, want a punished mute hold with welcome_held true and the photo bit", hold)
		}
	})

	t.Run("a joiner with a photo is welcomed", func(t *testing.T) {
		env, state := mreqLookupJoinEnv(t, map[memberreq.Rule]int{memberreq.RulePhoto: models.RequirementActionBan})
		member := mreqLookupMember(t, env, state, mreqHasPhoto)

		env.mreqJoin(member)

		if got := mreqCountText(env.mreqSentTexts(), mreqWelcomeWord); got != 1 {
			t.Fatalf("welcomes = %d, want 1", got)
		}
		mreqWantNothing(t, env, member, memberReqResult{})
	})

	t.Run("a Warn rule warns the joiner and the usual welcome follows", func(t *testing.T) {
		env, state := mreqLookupJoinEnv(t, map[memberreq.Rule]int{memberreq.RulePhoto: models.RequirementActionWarn})
		member := mreqLookupMember(t, env, state, mreqPhotoAnswer{})

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

	for _, tc := range []struct {
		name    string
		action  int
		method  string
		welcome int
	}{
		{"Kick", models.RequirementActionKick, "unbanChatMember", 0},
		{"Ban", models.RequirementActionBan, "banChatMember", 0},
		{"Mute", models.RequirementActionMute, "restrictChatMember", 0},
	} {
		t.Run("a joiner not subscribed to the linked channel is handled by "+tc.name, func(t *testing.T) {
			env, state := mreqLookupJoinEnv(t, map[memberreq.Rule]int{memberreq.RuleChannel: tc.action})
			member, _ := mreqChannelUser(t, env, state, gotgbot.ChatMemberStatusAdministrator, gotgbot.ChatMemberStatusLeft)

			env.mreqJoin(member)

			if got := env.mreqCallsAbout(member.Id, tc.method); got != 1 {
				t.Fatalf("%s calls = %d, want 1", tc.method, got)
			}
			if got := mreqCountText(env.mreqSentTexts(), mreqWelcomeWord); got != tc.welcome {
				t.Errorf("welcomes = %d, want %d", got, tc.welcome)
			}
			if hold := env.mreqHold(member.Id); hold == nil || hold.Rules != mreqBitOf(t, "ch") {
				t.Errorf("hold = %+v, want the channel bit", hold)
			}
		})
	}

	t.Run("a joiner subscribed to the channel is welcomed", func(t *testing.T) {
		env, state := mreqLookupJoinEnv(t, map[memberreq.Rule]int{memberreq.RuleChannel: models.RequirementActionBan})
		member, _ := mreqChannelUser(t, env, state, gotgbot.ChatMemberStatusAdministrator, gotgbot.ChatMemberStatusMember)

		env.mreqJoin(member)

		if got := mreqCountText(env.mreqSentTexts(), mreqWelcomeWord); got != 1 {
			t.Fatalf("welcomes = %d, want 1", got)
		}
		mreqWantNothing(t, env, member, memberReqResult{})
	})

	t.Run("a hung photo lookup is cut at the join budget, the joiner is welcomed and the first message is still checked", func(t *testing.T) {
		env, state := mreqLookupJoinEnv(t, map[memberreq.Rule]int{memberreq.RulePhoto: models.RequirementActionMute})
		member := mreqLookupMember(t, env, state, mreqPhotoAnswer{})
		mreqSwap(t, &memberReqLookupJoinBudget, 300*time.Millisecond)
		release := state.hold(member.Id)
		t.Cleanup(release)

		started := time.Now()
		env.mreqJoin(member)
		took := time.Since(started)
		if took < 250*time.Millisecond || took > 3*time.Second {
			t.Errorf("the join took %s, want about the 300 ms budget", took)
		}
		if got := mreqCountText(env.mreqSentTexts(), mreqWelcomeWord); got != 1 {
			t.Fatalf("welcomes = %d, want 1 (a joiner whose lookup did not finish is welcomed)", got)
		}
		if hold := env.mreqHold(member.Id); hold != nil {
			t.Fatalf("hold = %+v, want none after a cut-off lookup", hold)
		}
		if n := env.mreqCallCount("restrictChatMember", "banChatMember", "unbanChatMember"); n != 0 {
			t.Errorf("writes = %d, want none", n)
		}

		// The lookup answers now. The watcher starts no job for a member whose names are clean
		// until plan 14-07, so the member's first message is the executor run directly, dated
		// now, with the job budget.
		release()
		later := env.mreqCase(member)
		if got := env.mreqEnforce(later); got.Outcome != memberReqMuted {
			t.Fatalf("the first message after the join = %+v, want Muted (the member is caught then)", got)
		}
	})
}

func TestMemberReqLookupOnePunishmentAcrossReplicas(t *testing.T) {
	for _, tc := range []struct {
		name   string
		action int
		method string
		notice string
	}{
		{"Mute", models.RequirementActionMute, "restrictChatMember", "mreq_notice_mute"},
		{"Kick", models.RequirementActionKick, "unbanChatMember", "mreq_notice_kick"},
	} {
		t.Run("a photo-less member enforced at once on three processes is punished once with "+tc.name, func(t *testing.T) {
			env, state := mreqLookupEnv(t)
			env.mreqRules(map[memberreq.Rule]int{memberreq.RulePhoto: tc.action})
			member := mreqLookupMember(t, env, state, mreqPhotoAnswer{})
			first, second := env.mreqReplica(member), env.mreqReplica(member)
			mreqInstallLookupFake(first, state)
			mreqInstallLookupFake(second, state)
			processes := []*memberReqEnv{env, first, second}

			// Hold the one shared lookup open until every caller has joined it, so all of them
			// get the breach together and race to claim.
			release := state.hold(member.Id)
			t.Cleanup(release)
			const callers = 9
			c := env.mreqCase(member)
			results := make(chan memberReqResult, callers)
			var wg sync.WaitGroup
			for i := 0; i < callers; i++ {
				wg.Add(1)
				go func(p *memberReqEnv) {
					defer wg.Done()
					results <- memberReqEnforce(context.Background(), p.bot, c)
				}(processes[i%len(processes)])
			}
			deadline := time.Now().Add(5 * time.Second)
			for len(state.arrivals()) < 1 {
				if time.Now().After(deadline) {
					t.Fatal("no lookup request arrived")
				}
				time.Sleep(2 * time.Millisecond)
			}
			time.Sleep(150 * time.Millisecond)
			release()
			wg.Wait()
			close(results)

			punished := 0
			for result := range results {
				if result.Outcome == memberReqMuted || result.Outcome == memberReqRemoved {
					punished++
				}
			}
			if punished != 1 {
				t.Errorf("callers that punished = %d, want exactly 1", punished)
			}
			total := 0
			for _, p := range processes {
				total += p.mreqCallsAbout(member.Id, tc.method)
			}
			if total != 1 {
				t.Errorf("%s requests in all three processes = %d, want exactly 1", tc.method, total)
			}
			if n := mreqNoticeCount(staffMarker(tc.notice), processes...); n != 1 {
				t.Errorf("notices in all three processes = %d, want exactly 1", n)
			}
			if hold := env.mreqHold(member.Id); hold == nil || hold.PunishedAt == nil || hold.Rules != mreqBitOf(t, "ph") {
				t.Errorf("hold = %+v, want one punished hold with the photo bit", hold)
			}
		})
	}
}
