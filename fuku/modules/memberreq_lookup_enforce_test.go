//go:build testtools

package modules

import (
	"fmt"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

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
