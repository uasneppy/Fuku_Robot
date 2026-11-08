//go:build testtools

package modules

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"
	logrustest "github.com/sirupsen/logrus/hooks/test"

	"github.com/uasneppy/fuku_robot/fuku/db/approvals"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/captcha"
	"github.com/uasneppy/fuku_robot/fuku/db/lang"
	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/db/warns"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// mreqCallCount counts the requests of the named methods the fake has recorded for the group.
func (e *memberReqEnv) mreqCallCount(methods ...string) int {
	e.t.Helper()
	count := 0
	for _, method := range methods {
		count += len(e.calls(method))
	}
	return count
}

// mreqText is the text of a sendMessage the fake recorded.
func mreqText(sent staffSentMessage) string {
	return fmt.Sprint(sent.Params["text"])
}

// mreqKeyboardData returns the callback data of every button of a recorded message.
func mreqKeyboardData(sent staffSentMessage) []string {
	var data []string
	for _, row := range staffKeyboardOf(sent.Params["reply_markup"]) {
		for _, button := range row {
			data = append(data, button.CallbackData)
		}
	}
	return data
}

func TestMemberReqEnforceWarnOnce(t *testing.T) {
	env := newMemberReqEnv(t)
	env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionWarn})
	member := env.mreqMember("Alex", "", "alex")

	got := env.mreqEnforce(env.mreqCase(member))
	if got.Outcome != memberReqWarned || got.Hold == nil {
		t.Fatalf("first enforcement = %+v, want Warned with the hold", got)
	}

	row := env.mreqWarnRow(member.Id)
	if row == nil || row.NumWarns != 1 || len(row.Reasons) != 1 {
		t.Fatalf("warns row = %+v, want one warn with one reason", row)
	}
	if reason := row.Reasons[0]; !strings.Contains(reason, staffMarker("mreq_warn_reason")) ||
		!strings.Contains(reason, staffMarker("mreq_rule_lastname")) {
		t.Fatalf("stored warn reason = %q, want mreq_warn_reason naming the last name rule", reason)
	}

	notices := env.mreqNotices()
	if len(notices) != 1 {
		t.Fatalf("messages to the group = %d, want exactly one notice", len(notices))
	}
	text := mreqText(notices[0])
	for _, want := range []string{
		staffMarker("mreq_notice_warn"),
		fmt.Sprintf("tg://user?id=%d", member.Id),
		staffMarker("mreq_rule_lastname"),
	} {
		if !strings.Contains(text, want) {
			t.Errorf("notice %q does not contain %q", text, want)
		}
	}
	if !strings.HasSuffix(text, " 1 3") {
		t.Errorf("notice %q should end with the count 1 and the limit 3", text)
	}
	if _, set := notices[0].Params["reply_parameters"]; set {
		t.Errorf("notice carries reply_parameters %v, want a new message", notices[0].Params["reply_parameters"])
	}
	if opts, ok := notices[0].Params["link_preview_options"].(*gotgbot.LinkPreviewOptions); !ok || !opts.IsDisabled {
		t.Errorf("link_preview_options = %v, want previews disabled", notices[0].Params["link_preview_options"])
	}
	if data := mreqKeyboardData(notices[0]); len(data) != 0 {
		t.Errorf("warn notice has buttons %v, want none", data)
	}

	hold := env.mreqHold(member.Id)
	if hold == nil || hold.Action != models.RequirementActionWarn || hold.Rules != 1 ||
		hold.PunishedAt == nil || hold.NoticeMsgID != notices[0].MessageID {
		t.Fatalf("hold = %+v, want warn action, rules 1, punished, notice %d", hold, notices[0].MessageID)
	}

	writesBefore := env.mreqCallCount("sendMessage", "banChatMember", "restrictChatMember", "unbanChatMember")
	again := env.mreqEnforce(env.mreqCase(member))
	if again.Outcome != memberReqHeld {
		t.Fatalf("second enforcement = %+v, want Held", again)
	}
	if after := env.mreqCallCount("sendMessage", "banChatMember", "restrictChatMember", "unbanChatMember"); after != writesBefore {
		t.Errorf("a held member made %d new writes or messages, want none", after-writesBefore)
	}
	if row := env.mreqWarnRow(member.Id); row == nil || row.NumWarns != 1 {
		t.Errorf("warns after the held enforcement = %+v, want still 1", row)
	}

	for _, sent := range env.mreqNotices() {
		if strings.Contains(mreqText(sent), staffMarker("warns_user_warning")) {
			t.Errorf("the warn module's reply was sent: %q", mreqText(sent))
		}
		for _, data := range mreqKeyboardData(sent) {
			if strings.HasPrefix(data, "rmWarn|") {
				t.Errorf("the Remove warn button was sent: %q", data)
			}
		}
	}
}

func TestMemberReqWarnMarkInsideLock(t *testing.T) {
	shortenStale := func(t *testing.T) {
		t.Helper()
		previous := memberreq.StaleClaim
		memberreq.StaleClaim = time.Millisecond
		t.Cleanup(func() { memberreq.StaleClaim = previous })
	}
	swapApplyWarn := func(t *testing.T, replacement func(context.Context, *gotgbot.Bot, *gotgbot.Chat, int64, string, func()) (warnApplyResult, error)) {
		t.Helper()
		previous := memberReqApplyWarn
		memberReqApplyWarn = replacement
		t.Cleanup(func() { memberReqApplyWarn = previous })
	}

	t.Run("a recorded warn is a punished hold before the warn lock is released", func(t *testing.T) {
		shortenStale(t)
		env := newMemberReqEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionWarn})
		member := env.mreqMember("Alex", "", "alex")

		var lockHeld, punishedInside bool
		swapApplyWarn(t, func(ctx context.Context, b *gotgbot.Bot, chat *gotgbot.Chat, userID int64, reason string, afterRecord func()) (warnApplyResult, error) {
			return applyWarn(ctx, b, chat, userID, reason, func() {
				if afterRecord != nil {
					afterRecord()
				}
				if mu := warnPunishLock(chat.Id, userID); mu.TryLock() {
					mu.Unlock()
				} else {
					lockHeld = true
				}
				hold, err := memberreq.GetHoldFresh(context.Background(), chat.Id, userID)
				punishedInside = err == nil && hold != nil && hold.PunishedAt != nil
			})
		})

		if got := env.mreqEnforce(env.mreqCase(member)); got.Outcome != memberReqWarned {
			t.Fatalf("enforcement = %+v, want Warned", got)
		}
		if !lockHeld {
			t.Error("afterRecord ran outside the warn lock")
		}
		if !punishedInside {
			t.Error("the hold was not marked punished by the time the warn lock was released")
		}

		time.Sleep(10 * time.Millisecond)
		if got := env.mreqEnforce(env.mreqCase(member)); got.Outcome != memberReqHeld {
			t.Fatalf("enforcement after the stale age = %+v, want Held (a punished hold is never reclaimed)", got)
		}
		if row := env.mreqWarnRow(member.Id); row == nil || row.NumWarns != 1 {
			t.Errorf("warns = %+v, want exactly 1 after a reclaim attempt", row)
		}
		if n := len(env.mreqNotices()); n != 1 {
			t.Errorf("notices = %d, want 1", n)
		}
	})

	t.Run("a warn that cannot be recorded leaves a pending hold that one reclaim warns once", func(t *testing.T) {
		shortenStale(t)
		env := newMemberReqEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionWarn})
		member := env.mreqMember("Alex", "", "alex")

		calls := 0
		swapApplyWarn(t, func(ctx context.Context, b *gotgbot.Bot, chat *gotgbot.Chat, userID int64, reason string, afterRecord func()) (warnApplyResult, error) {
			calls++
			if calls > 1 {
				return applyWarn(ctx, b, chat, userID, reason, afterRecord)
			}
			restore := captchaApplyBrokenDB(t)
			defer restore()
			return applyWarn(ctx, b, chat, userID, reason, afterRecord)
		})

		if got := env.mreqEnforce(env.mreqCase(member)); got.Outcome != memberReqNotApplied {
			t.Fatalf("enforcement with a failing warn write = %+v, want NotApplied", got)
		}
		hold := env.mreqHold(member.Id)
		if hold == nil || hold.PunishedAt != nil {
			t.Fatalf("hold after the failed warn = %+v, want a pending hold", hold)
		}
		if row := env.mreqWarnRow(member.Id); row != nil {
			t.Errorf("warns row = %+v, want none", row)
		}
		if n := len(env.mreqNotices()); n != 0 {
			t.Errorf("notices after the failed warn = %d, want none", n)
		}

		time.Sleep(10 * time.Millisecond)
		if got := env.mreqEnforce(env.mreqCase(member)); got.Outcome != memberReqWarned {
			t.Fatalf("enforcement after the stale age = %+v, want Warned by the reclaim", got)
		}
		if row := env.mreqWarnRow(member.Id); row == nil || row.NumWarns != 1 {
			t.Errorf("warns = %+v, want exactly one", row)
		}
		if hold := env.mreqHold(member.Id); hold == nil || hold.PunishedAt == nil {
			t.Errorf("hold after the reclaim = %+v, want punished", hold)
		}
		if n := len(env.mreqNotices()); n != 1 {
			t.Errorf("notices = %d, want exactly one", n)
		}
	})
}

// mreqEnforceOne stores the rules, mints a member with the given names and enforces once.
func mreqEnforceOne(t *testing.T, env *memberReqEnv, rules map[memberreq.Rule]int, first, last, username string) (gotgbot.User, memberReqResult) {
	t.Helper()
	env.mreqRules(rules)
	member := env.mreqMember(first, last, username)
	return member, env.mreqEnforce(env.mreqCase(member))
}

// mreqLiveRecord reads the member's live record from the fake through the executor's own read.
func mreqLiveRecord(t *testing.T, env *memberReqEnv, userID int64) gotgbot.MergedChatMember {
	t.Helper()
	live, err := memberReqLiveMember(context.Background(), env.bot, env.chat.Id, userID)
	if err != nil {
		t.Fatalf("live record of %d: %v", userID, err)
	}
	return live
}

func TestMemberReqEnforcePunishments(t *testing.T) {
	t.Run("kick is the member-removing unban", func(t *testing.T) {
		env := newMemberReqEnv(t)
		member, got := mreqEnforceOne(t, env, map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionKick}, "Alex", "", "alex")
		if got.Outcome != memberReqRemoved {
			t.Fatalf("outcome = %+v, want Removed", got)
		}
		unbans := env.calls("unbanChatMember")
		if len(unbans) != 1 || staffParamBool(unbans[0].Params, "only_if_banned") {
			t.Fatalf("unban calls = %+v, want one with only_if_banned false", unbans)
		}
		if n := len(env.mreqWrites()); n != 1 {
			t.Errorf("ban, restrict and unban calls = %d, want only the unban", n)
		}
		notices := env.mreqNotices()
		if len(notices) != 1 || !strings.Contains(mreqText(notices[0]), staffMarker("mreq_notice_kick")) {
			t.Fatalf("notices = %+v, want one mreq_notice_kick", notices)
		}
		if data := mreqKeyboardData(notices[0]); len(data) != 0 {
			t.Errorf("kick notice has buttons %v, want none", data)
		}
		if hold := env.mreqHold(member.Id); hold == nil || hold.Action != models.RequirementActionKick || hold.PunishedAt == nil {
			t.Errorf("hold = %+v, want a punished kick hold", hold)
		}
	})

	t.Run("ban has no end date and no message revocation", func(t *testing.T) {
		env := newMemberReqEnv(t)
		member, got := mreqEnforceOne(t, env, map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionBan}, "Alex", "", "alex")
		if got.Outcome != memberReqRemoved {
			t.Fatalf("outcome = %+v, want Removed", got)
		}
		bans := env.calls("banChatMember")
		if len(bans) != 1 {
			t.Fatalf("ban calls = %d, want 1", len(bans))
		}
		if _, set := bans[0].Params["until_date"]; set {
			t.Errorf("ban carries until_date %v, want permanent", bans[0].Params["until_date"])
		}
		if staffParamBool(bans[0].Params, "revoke_messages") {
			t.Error("ban revokes messages, want it off")
		}
		notices := env.mreqNotices()
		if len(notices) != 1 || !strings.Contains(mreqText(notices[0]), staffMarker("mreq_notice_ban")) {
			t.Fatalf("notices = %+v, want one mreq_notice_ban", notices)
		}
		if hold := env.mreqHold(member.Id); hold == nil || hold.Action != models.RequirementActionBan || hold.PunishedAt == nil {
			t.Errorf("hold = %+v, want a punished ban hold", hold)
		}
	})

	t.Run("mute carries the marker end date and one button", func(t *testing.T) {
		env := newMemberReqEnv(t)
		member, got := mreqEnforceOne(t, env, map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionMute}, "Alex", "", "alex")
		if got.Outcome != memberReqMuted || got.Hold == nil {
			t.Fatalf("outcome = %+v, want Muted with the hold", got)
		}
		restricts := env.calls("restrictChatMember")
		if len(restricts) != 1 {
			t.Fatalf("restrict calls = %d, want 1", len(restricts))
		}
		if perms, ok := restricts[0].Params["permissions"].(gotgbot.ChatPermissions); !ok || !reflect.DeepEqual(perms, MutedPermissions) {
			t.Errorf("permissions = %+v, want MutedPermissions", restricts[0].Params["permissions"])
		}
		hold := env.mreqHold(member.Id)
		if hold == nil || hold.Action != models.RequirementActionMute || hold.PunishedAt == nil {
			t.Fatalf("hold = %+v, want a punished mute hold", hold)
		}
		until := staffParamInt(restricts[0].Params, "until_date")
		if until != hold.MuteUntil {
			t.Errorf("until_date = %d, hold mute_until = %d, want equal", until, hold.MuteUntil)
		}
		want := time.Now().Add(lockdownBanSpan).Unix()
		if diff := want - until; diff < -2 || diff > 2 {
			t.Errorf("until_date = %d, want now plus 330 days (%d)", until, want)
		}

		notices := env.mreqNotices()
		if len(notices) != 1 || !strings.Contains(mreqText(notices[0]), staffMarker("mreq_notice_mute")) {
			t.Fatalf("notices = %+v, want one mreq_notice_mute", notices)
		}
		rows := staffKeyboardOf(notices[0].Params["reply_markup"])
		if len(rows) != 1 || len(rows[0]) != 1 {
			t.Fatalf("keyboard = %+v, want one row of one button", rows)
		}
		decoded, ok := decodeCallbackData(rows[0][0].CallbackData, "mreq")
		if !ok || len(decoded.Fields) != 3 || decoded.Fields["a"] != "f" ||
			decoded.Fields["h"] != strconv.FormatInt(hold.ID, 10) || decoded.Fields["u"] != strconv.FormatInt(member.Id, 10) {
			t.Fatalf("button data %q decoded to %+v, want exactly a=f, h=%d, u=%d", rows[0][0].CallbackData, decoded, hold.ID, member.Id)
		}
		if notices[0].Params["reply_parameters"] != nil {
			t.Error("the mute notice is a reply, want a new message")
		}

		if !isMemberReqMute(mreqLiveRecord(t, env, member.Id), hold.MuteUntil) {
			t.Error("isMemberReqMute = false for the record the requirement mute left")
		}
	})

	t.Run("the mute is stored on the hold before the call", func(t *testing.T) {
		env := newMemberReqEnv(t)
		env.fake.script("restrictChatMember", env.chat.Id, staffFakeError(400, "Bad Request: not enough rights"))
		member, got := mreqEnforceOne(t, env, map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionMute}, "Alex", "", "alex")
		if got.Outcome != memberReqNotApplied {
			t.Fatalf("outcome = %+v, want NotApplied", got)
		}
		hold := env.mreqHold(member.Id)
		if hold == nil || hold.PunishedAt != nil || hold.MuteUntil == 0 {
			t.Fatalf("hold = %+v, want a pending hold that already carries the marker", hold)
		}
		if n := len(env.mreqNotices()); n != 0 {
			t.Errorf("notices = %d, want none after a refused mute", n)
		}
	})

	t.Run("only the marker proves a requirement mute", func(t *testing.T) {
		marker := memberReqMuteUntil(time.Now())
		restricted := func(until int64, isMember bool) gotgbot.MergedChatMember {
			return gotgbot.MergedChatMember{Status: gotgbot.ChatMemberStatusRestricted, IsMember: isMember, UntilDate: until}
		}
		cases := []struct {
			name   string
			member gotgbot.MergedChatMember
			want   bool
		}{
			{"the marker", restricted(marker, true), true},
			{"the marker two seconds off", restricted(marker+2, true), true},
			{"the marker three seconds off", restricted(marker+3, true), false},
			{"a /mute (until 0)", restricted(0, true), false},
			{"a /tmute of another date", restricted(time.Now().Add(time.Hour).Unix(), true), false},
			{"the marker but left the group", restricted(marker, false), false},
			{"a plain member", gotgbot.MergedChatMember{Status: gotgbot.ChatMemberStatusMember, UntilDate: marker}, false},
		}
		for _, tc := range cases {
			if got := isMemberReqMute(tc.member, marker); got != tc.want {
				t.Errorf("%s: isMemberReqMute = %t, want %t", tc.name, got, tc.want)
			}
		}
		if isMemberReqMute(restricted(marker, true), 0) {
			t.Error("isMemberReqMute is true when the hold has no marker")
		}
	})

	t.Run("no other mute path leaves the marker on the fake", func(t *testing.T) {
		env := newMemberReqEnv(t)
		member := env.mreqMember("Alex", "", "alex")
		marker := memberReqMuteUntil(time.Now())
		env.fake.setMember(env.chat.Id, member.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusRestricted, IsMember: true})
		if isMemberReqMute(mreqLiveRecord(t, env, member.Id), marker) {
			t.Error("a /mute record (until 0) was taken for the requirement mute")
		}
		env.fake.setMember(env.chat.Id, member.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusRestricted, IsMember: true, UntilDate: marker + 3600})
		if isMemberReqMute(mreqLiveRecord(t, env, member.Id), marker) {
			t.Error("a /tmute record of another date was taken for the requirement mute")
		}
		env.fake.setMember(env.chat.Id, member.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember, IsMember: true})
		if isMemberReqMute(mreqLiveRecord(t, env, member.Id), marker) {
			t.Error("a member record was taken for the requirement mute")
		}
	})
}

func TestMemberReqEnforceStrictest(t *testing.T) {
	// "محمد 王" breaks the Arabic and the Chinese rule; with no last name and no username it
	// also breaks the last name and the username rules.
	const name = "محمد 王"
	order := func(phrases ...string) string {
		keys := make([]string, len(phrases))
		for i, phrase := range phrases {
			keys[i] = staffMarker(phrase)
		}
		return strings.Join(keys, ", ")
	}

	t.Run("ban over mute over warn, one notice naming the rules in rule order", func(t *testing.T) {
		env := newMemberReqEnv(t)
		member, got := mreqEnforceOne(t, env, map[memberreq.Rule]int{
			memberreq.RuleLastName: models.RequirementActionWarn,
			memberreq.RuleUsername: models.RequirementActionMute,
			memberreq.RuleArabic:   models.RequirementActionBan,
			memberreq.RuleChinese:  models.RequirementActionOff,
		}, name, "", "")
		if got.Outcome != memberReqRemoved {
			t.Fatalf("outcome = %+v, want Removed (a ban)", got)
		}
		if n := len(env.calls("banChatMember")); n != 1 {
			t.Errorf("ban calls = %d, want 1", n)
		}
		if n := len(env.calls("restrictChatMember")) + len(env.calls("unbanChatMember")); n != 0 {
			t.Errorf("mute and kick calls = %d, want none", n)
		}
		if row := env.mreqWarnRow(member.Id); row != nil {
			t.Errorf("warns row = %+v, want none", row)
		}
		notices := env.mreqNotices()
		if len(notices) != 1 {
			t.Fatalf("notices = %d, want one", len(notices))
		}
		text := mreqText(notices[0])
		if !strings.Contains(text, order("mreq_rule_lastname", "mreq_rule_username", "mreq_rule_arabic")) {
			t.Errorf("notice %q does not list no last name, no username and Arabic letters in rule order", text)
		}
		if strings.Contains(text, staffMarker("mreq_rule_chinese")) {
			t.Errorf("notice %q names the Chinese rule, which is Off", text)
		}
		if hold := env.mreqHold(member.Id); hold == nil || hold.Rules != 1|2|8 || hold.Action != models.RequirementActionBan {
			t.Errorf("hold = %+v, want rules 11 and a ban", hold)
		}
	})

	t.Run("kick over mute", func(t *testing.T) {
		env := newMemberReqEnv(t)
		_, got := mreqEnforceOne(t, env, map[memberreq.Rule]int{
			memberreq.RuleLastName: models.RequirementActionKick,
			memberreq.RuleUsername: models.RequirementActionMute,
		}, "Alex", "", "")
		if got.Outcome != memberReqRemoved || len(env.calls("unbanChatMember")) != 1 || len(env.calls("restrictChatMember")) != 0 {
			t.Fatalf("outcome = %+v, want one kick and no restrict", got)
		}
	})

	t.Run("mute over warn", func(t *testing.T) {
		env := newMemberReqEnv(t)
		member, got := mreqEnforceOne(t, env, map[memberreq.Rule]int{
			memberreq.RuleLastName: models.RequirementActionWarn,
			memberreq.RuleUsername: models.RequirementActionMute,
		}, "Alex", "", "")
		if got.Outcome != memberReqMuted || len(env.calls("restrictChatMember")) != 1 {
			t.Fatalf("outcome = %+v, want one mute", got)
		}
		if row := env.mreqWarnRow(member.Id); row != nil {
			t.Errorf("warns row = %+v, want none when a mute is chosen", row)
		}
	})
}

func TestMemberReqNoticeWarnLimit(t *testing.T) {
	cases := []struct {
		mode        string
		key         string
		wantOutcome memberReqOutcome
		call        string
	}{
		{"kick", "mreq_notice_warn_limit_kick", memberReqRemoved, "unbanChatMember"},
		{"ban", "mreq_notice_warn_limit_ban", memberReqRemoved, "banChatMember"},
		{"mute", "mreq_notice_warn_limit_mute", memberReqWarned, "restrictChatMember"},
	}
	for _, tc := range cases {
		t.Run("limit "+tc.mode, func(t *testing.T) {
			env := newMemberReqEnv(t)
			member := env.mreqMember("Alex", "", "alex")
			env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionWarn})
			if err := warns.SetWarnLimit(env.chat.Id, 1); err != nil {
				t.Fatal(err)
			}
			if err := warns.SetWarnMode(env.chat.Id, tc.mode); err != nil {
				t.Fatal(err)
			}
			got := env.mreqEnforce(env.mreqCase(member))
			if got.Outcome != tc.wantOutcome {
				t.Fatalf("outcome = %+v, want %v", got, tc.wantOutcome)
			}
			if n := len(env.calls(tc.call)); n != 1 {
				t.Errorf("%s calls = %d, want the warn module's limit action once", tc.call, n)
			}
			notices := env.mreqNotices()
			if len(notices) != 1 {
				t.Fatalf("notices = %d, want one", len(notices))
			}
			text := mreqText(notices[0])
			if !strings.Contains(text, staffMarker(tc.key)) || !strings.HasSuffix(text, " 1 1") {
				t.Errorf("notice %q, want %s with n 1 and limit 1", text, tc.key)
			}
			if data := mreqKeyboardData(notices[0]); len(data) != 0 {
				t.Errorf("limit notice has buttons %v, want none (a warn-limit mute is the warn module's)", data)
			}
		})
	}

	t.Run("a refused limit action is the plain warn notice", func(t *testing.T) {
		env := newMemberReqEnv(t)
		member := env.mreqMember("Alex", "", "alex")
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionWarn})
		if err := warns.SetWarnLimit(env.chat.Id, 1); err != nil {
			t.Fatal(err)
		}
		if err := warns.SetWarnMode(env.chat.Id, "kick"); err != nil {
			t.Fatal(err)
		}
		env.fake.script("unbanChatMember", env.chat.Id, staffFakeError(400, "Bad Request: not enough rights"))
		got := env.mreqEnforce(env.mreqCase(member))
		if got.Outcome != memberReqWarned {
			t.Fatalf("outcome = %+v, want Warned", got)
		}
		notices := env.mreqNotices()
		if len(notices) != 1 {
			t.Fatalf("notices = %d, want one", len(notices))
		}
		text := mreqText(notices[0])
		if !strings.Contains(text, staffMarker("mreq_notice_warn")) || strings.Contains(text, staffMarker("mreq_notice_warn_limit_kick")) {
			t.Errorf("notice %q, want the plain warn notice and no removal claimed", text)
		}
	})
}

func TestMemberReqNoticeLanguage(t *testing.T) {
	env := newMemberReqEnvWith(t, settingsEnvOptions{RealLocale: true})
	if err := lang.ChangeGroupLanguage(env.chat.Id, "uk"); err != nil {
		t.Fatal(err)
	}
	// A name that would be a template parameter or markup if it ever met the translator.
	member, got := mreqEnforceOne(t, env, map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionWarn}, "{rules}<b>", "", "alex")
	if got.Outcome != memberReqWarned {
		t.Fatalf("outcome = %+v, want Warned", got)
	}
	uk := i18n.MustNewTranslator("uk")
	phrase, err := uk.GetString("mreq_rule_lastname")
	if err != nil {
		t.Fatal(err)
	}
	wantReason, err := uk.GetString("mreq_warn_reason", i18n.TranslationParams{"rules": phrase})
	if err != nil {
		t.Fatal(err)
	}
	row := env.mreqWarnRow(member.Id)
	if row == nil || len(row.Reasons) != 1 || row.Reasons[0] != wantReason {
		t.Fatalf("stored warn reason = %+v, want the uk text %q", row, wantReason)
	}
	notices := env.mreqNotices()
	if len(notices) != 1 {
		t.Fatalf("notices = %d, want one", len(notices))
	}
	text := mreqText(notices[0])
	if !strings.Contains(text, phrase) {
		t.Errorf("notice %q does not name the rule in the group's language (%q)", text, phrase)
	}
	if !strings.Contains(text, "\u2068{rules}&lt;b&gt;\u2069") {
		t.Errorf("notice %q does not print the name literally, escaped and isolated", text)
	}
}

// mreqSwap installs replacement in *target for the test.
func mreqSwap[T any](t *testing.T, target *T, replacement T) {
	t.Helper()
	previous := *target
	*target = replacement
	t.Cleanup(func() { *target = previous })
}

func TestMemberReqEnforceExemptions(t *testing.T) {
	env := newMemberReqEnv(t)
	// Mute is the punishment that would show most: a hold, a restrict and a notice.
	env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionMute})

	// wantNobody fails when the executor did anything: a hold, a warn, a Telegram write or a notice.
	wantNobody := func(t *testing.T, user gotgbot.User, got memberReqResult) {
		t.Helper()
		if got.Outcome != memberReqNone || got.Hold != nil {
			t.Errorf("outcome = %+v, want None with no hold", got)
		}
		if hold := env.mreqHold(user.Id); hold != nil {
			t.Errorf("hold = %+v, want none", hold)
		}
		if row := env.mreqWarnRow(user.Id); row != nil {
			t.Errorf("warns row = %+v, want none", row)
		}
	}
	// writesAndNotices is how many writes and messages the executor sent in all so far.
	writesAndNotices := func() int {
		return len(env.mreqWrites()) + len(env.mreqNotices())
	}
	before := writesAndNotices()
	t.Cleanup(func() {
		if after := writesAndNotices(); after != before {
			t.Errorf("exempt members caused %d Telegram writes or messages, want none", after-before)
		}
	})

	liveStates := []struct {
		name   string
		record staffFakeMember
	}{
		{"a live creator", staffFakeMember{Status: gotgbot.ChatMemberStatusCreator}},
		{"a live administrator", staffFakeMember{Status: gotgbot.ChatMemberStatusAdministrator, CanRestrictMembers: true}},
		{"a member who left", staffFakeMember{Status: gotgbot.ChatMemberStatusLeft}},
		{"a kicked member", staffFakeMember{Status: gotgbot.ChatMemberStatusKicked}},
		{"a restricted non-member", staffFakeMember{Status: gotgbot.ChatMemberStatusRestricted, IsMember: false}},
	}
	for _, tc := range liveStates {
		t.Run(tc.name, func(t *testing.T) {
			member := env.mreqMember("Alex", "", "alex")
			env.fake.setMember(env.chat.Id, member.Id, tc.record)
			wantNobody(t, member, env.mreqEnforce(env.mreqCase(member)))
		})
	}

	t.Run("an approved member", func(t *testing.T) {
		member := env.mreqMember("Alex", "", "alex")
		if err := approvals.AddApprovedUser(env.chat.Id, member.Id, env.admin.Id, "trusted"); err != nil {
			t.Fatal(err)
		}
		wantNobody(t, member, env.mreqEnforce(env.mreqCase(member)))
	})

	t.Run("a member with a pending captcha attempt", func(t *testing.T) {
		member := env.mreqMember("Alex", "", "alex")
		if _, err := captcha.CreateCaptchaAttemptPreMessage(member.Id, env.chat.Id, "answer", 5); err != nil {
			t.Fatal(err)
		}
		dbcache.DeleteCache(dbcache.CacheKey("captcha_pending", env.chat.Id))
		wantNobody(t, member, env.mreqEnforce(env.mreqCase(member)))
	})

	t.Run("a bot, the bot itself and the service accounts", func(t *testing.T) {
		for _, id := range []int64{env.bot.Id, 777000, 1087968824, 136817688} {
			user := gotgbot.User{Id: id, FirstName: "Service"}
			wantNobody(t, user, env.mreqEnforce(env.mreqCase(user)))
		}
		other := env.mreqMember("Robo", "", "robot")
		other.IsBot = true
		wantNobody(t, other, env.mreqEnforce(env.mreqCase(other)))
	})

	t.Run("an approvals read error", func(t *testing.T) {
		member := env.mreqMember("Alex", "", "alex")
		mreqSwap(t, &memberReqApprovedLookup, func(context.Context, int64, int64) (bool, error) {
			return false, context.DeadlineExceeded
		})
		wantNobody(t, member, env.mreqEnforce(env.mreqCase(member)))
	})

	t.Run("a captcha read error", func(t *testing.T) {
		member := env.mreqMember("Alex", "", "alex")
		mreqSwap(t, &memberReqCaptchaPendingLookup, func(context.Context, int64) (bool, error) {
			return false, context.DeadlineExceeded
		})
		wantNobody(t, member, env.mreqEnforce(env.mreqCase(member)))
	})

	t.Run("a captcha attempt read error", func(t *testing.T) {
		member := env.mreqMember("Alex", "", "alex")
		mreqSwap(t, &memberReqCaptchaPendingLookup, func(context.Context, int64) (bool, error) { return true, nil })
		mreqSwap(t, &memberReqCaptchaAttemptLookup, func(int64, int64) (*models.CaptchaAttempts, error) {
			return nil, context.DeadlineExceeded
		})
		wantNobody(t, member, env.mreqEnforce(env.mreqCase(member)))
	})

	lookupFailures := []struct {
		name string
		errs []error
	}{
		{"a 400", []error{staffFakeError(400, "Bad Request: chat not found")}},
		{"a 403", []error{staffFakeError(403, "Forbidden: bot was kicked")}},
		{"a 429 that never clears", []error{staffFake429(1), staffFake429(1), staffFake429(1), staffFake429(1), staffFake429(1)}},
		{"a 502", []error{staffFakeError(502, "Bad Gateway")}},
		{"a timeout", []error{context.DeadlineExceeded}},
	}
	for _, tc := range lookupFailures {
		t.Run("getChatMember answering "+tc.name, func(t *testing.T) {
			member := env.mreqMember("Alex", "", "alex")
			env.fake.script("getChatMember", env.chat.Id, tc.errs...)
			wantNobody(t, member, env.mreqEnforce(env.mreqCase(member)))
		})
	}

	t.Run("a member renamed since the update", func(t *testing.T) {
		member := env.mreqMember("Alex", "", "alex")
		stale := env.mreqCase(member)
		env.fake.setProfile(member.Id, "Alex", "Smith", "alex")
		wantNobody(t, member, env.mreqEnforce(stale))
	})
}

// mreqRestrictedBySomeoneElse puts the member in the fake group as an admin's restriction left
// them: still in the group, restricted, with the given end date and no requirement marker.
func (e *memberReqEnv) mreqRestrictedBySomeoneElse(user gotgbot.User, until int64) {
	e.t.Helper()
	e.fake.setMember(e.chat.Id, user.Id, staffFakeMember{
		Status:          gotgbot.ChatMemberStatusRestricted,
		IsMember:        true,
		CanSendMessages: true,
		UntilDate:       until,
	})
}

// TestMemberReqMuteNeverReplacesRestriction is CR-01: a requirement Mute never replaces a
// restriction someone else placed, while Warn, Kick and Ban still act on a restricted member.
func TestMemberReqMuteNeverReplacesRestriction(t *testing.T) {
	for _, tc := range []struct {
		name  string
		until int64
	}{
		{"a partial restriction with no end date", 0},
		{"a restriction an admin gave an end date", time.Now().Add(48 * time.Hour).Unix()},
	} {
		t.Run("a Mute rule places no mute over "+tc.name, func(t *testing.T) {
			hook := logrustest.NewGlobal()
			t.Cleanup(hook.Reset)
			env := newMemberReqEnv(t)
			env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionMute})
			env.mreqDeleteMessages(true)
			member := env.mreqMember("Alex", "", "alex")
			env.mreqRestrictedBySomeoneElse(member, tc.until)

			got := env.mreqEnforce(env.mreqCase(member))
			if got.Outcome != memberReqNone || got.Hold != nil {
				t.Fatalf("outcome = %+v, want None with no hold", got)
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
			for _, secret := range []string{"Alex", "alex"} {
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
		t.Run(tc.name+" still acts on a restricted member", func(t *testing.T) {
			env := newMemberReqEnv(t)
			env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: tc.action})
			member := env.mreqMember("Alex", "", "alex")
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
					t.Errorf("a warn made %d restriction calls, want none (the restriction stays)", n)
				}
			}
		})
	}

	t.Run("a restriction that only equals the group's defaults is not someone else's", func(t *testing.T) {
		// A group whose defaults are restrictive keeps a member the captcha, an admin's /unmute or a
		// requirement lift restored to those defaults as "restricted", yet nothing is taken away.
		env := newMemberReqEnv(t)
		env.fake.setChatPermsRaw(env.chat.Id, `{"can_send_messages":true,"can_pin_messages":true}`)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionMute})
		member := env.mreqMember("Alex", "", "alex")
		defaults := gotgbot.ChatPermissions{CanSendMessages: true, CanPinMessages: true}
		env.fake.setMember(env.chat.Id, member.Id, staffFakeMember{
			Status: gotgbot.ChatMemberStatusRestricted, IsMember: true, CanSendMessages: true, Perms: &defaults,
		})

		if got := env.mreqEnforce(env.mreqCase(member)); got.Outcome != memberReqMuted {
			t.Fatalf("outcome = %+v, want Muted (a member at the group's defaults loses nothing to the mute)", got)
		}
	})

	t.Run("a restriction below the group's defaults is someone else's", func(t *testing.T) {
		env := newMemberReqEnv(t)
		env.fake.setChatPermsRaw(env.chat.Id, `{"can_send_messages":true,"can_pin_messages":true}`)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionMute})
		member := env.mreqMember("Alex", "", "alex")
		partial := gotgbot.ChatPermissions{CanSendMessages: true}
		env.fake.setMember(env.chat.Id, member.Id, staffFakeMember{
			Status: gotgbot.ChatMemberStatusRestricted, IsMember: true, CanSendMessages: true, Perms: &partial,
		})

		if got := env.mreqEnforce(env.mreqCase(member)); got.Outcome != memberReqNone {
			t.Fatalf("outcome = %+v, want None (the pin right the admin took away is theirs)", got)
		}
		if n := env.mreqCallCount("restrictChatMember"); n != 0 {
			t.Errorf("restrict calls = %d, want none", n)
		}
	})

	t.Run("a group whose defaults cannot be read gets no mute over a restriction", func(t *testing.T) {
		env := newMemberReqEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionMute})
		member := env.mreqMember("Alex", "", "alex")
		env.mreqRestrictedBySomeoneElse(member, 0)
		env.fake.script("getChat", env.chat.Id, staffFakeError(502, "Bad Gateway"))

		if got := env.mreqEnforce(env.mreqCase(member)); got.Outcome != memberReqNone {
			t.Fatalf("outcome = %+v, want None (an unknown default fails closed)", got)
		}
		if n := env.mreqCallCount("restrictChatMember"); n != 0 {
			t.Errorf("restrict calls = %d, want none", n)
		}
		if hold := env.mreqHold(member.Id); hold != nil {
			t.Errorf("hold = %+v, want none", hold)
		}
	})

	t.Run("a stale mute hold whose marker is live on the member is adopted", func(t *testing.T) {
		mreqShortenStale(t, 50*time.Millisecond)
		env := newMemberReqEnv(t)
		member := env.mreqMember("Alex", "", "alex")
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionMute})
		env.loseAnswers("restrictChatMember", 1)
		env.fake.script("getChatMember", env.chat.Id, nil, staffFakeError(502, "Bad Gateway"))
		if got := env.mreqEnforce(env.mreqCase(member)); got.Outcome != memberReqNotApplied {
			t.Fatalf("first case = %+v, want NotApplied", got)
		}
		first := env.mreqHold(member.Id)
		time.Sleep(80 * time.Millisecond)

		got := env.mreqEnforce(env.mreqCase(member))
		if got.Outcome != memberReqMuted {
			t.Fatalf("reclaim = %+v, want the live marker adopted (Muted)", got)
		}
		if n := len(env.calls("restrictChatMember")); n != 1 {
			t.Errorf("restrict calls = %d, want only the first", n)
		}
		if hold := env.mreqHold(member.Id); hold == nil || hold.PunishedAt == nil || hold.MuteUntil != first.MuteUntil {
			t.Errorf("hold = %+v, want punished with marker %d", hold, first.MuteUntil)
		}
	})
}

// TestMemberReqEnforceReadsSettingsAfterLookup is WR-02 (D-20): a job holds the settings its
// watcher read, but a rule switched off, a rule changed or Delete messages turned off before the
// job's live lookup is honoured, because the executor reads the settings again before the claim.
func TestMemberReqEnforceReadsSettingsAfterLookup(t *testing.T) {
	t.Run("a rule switched Off after the message punishes nobody", func(t *testing.T) {
		env := newMemberReqEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionMute})
		env.mreqDeleteMessages(true)
		member := env.mreqMember("Alex", "", "alex")
		c := env.mreqCase(member)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionOff})

		got := env.mreqEnforce(c)
		if got.Outcome != memberReqNone || got.Hold != nil {
			t.Fatalf("outcome = %+v, want None with no hold", got)
		}
		if n := env.mreqCallCount("restrictChatMember", "banChatMember", "unbanChatMember", "deleteMessage", "sendMessage"); n != 0 {
			t.Errorf("writes, deletes and messages = %d, want none", n)
		}
		if hold := env.mreqHold(member.Id); hold != nil {
			t.Errorf("hold = %+v, want none", hold)
		}
	})

	t.Run("Delete messages switched off after the message deletes nothing", func(t *testing.T) {
		env := newMemberReqEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionWarn})
		env.mreqDeleteMessages(true)
		member := env.mreqMember("Alex", "", "alex")
		c := env.mreqCase(member)
		env.mreqDeleteMessages(false)

		if got := env.mreqEnforce(c); got.Outcome != memberReqWarned {
			t.Fatalf("outcome = %+v, want Warned (only the delete switch changed)", got)
		}
		if ids := env.mreqDeleted(); len(ids) != 0 {
			t.Errorf("deleted messages = %v, want none", ids)
		}
	})

	t.Run("a rule changed from Mute to Warn punishes with the fresh action", func(t *testing.T) {
		env := newMemberReqEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionMute})
		member := env.mreqMember("Alex", "", "alex")
		c := env.mreqCase(member)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionWarn})

		got := env.mreqEnforce(c)
		if got.Outcome != memberReqWarned || got.Hold == nil || got.Hold.Action != models.RequirementActionWarn {
			t.Fatalf("outcome = %+v, want a Warn hold", got)
		}
		if n := env.mreqCallCount("restrictChatMember"); n != 0 {
			t.Errorf("restrict calls = %d, want none", n)
		}
	})

	t.Run("Delete messages switched on after the message deletes the message", func(t *testing.T) {
		env := newMemberReqEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionWarn})
		member := env.mreqMember("Alex", "", "alex")
		c := env.mreqCase(member)
		env.mreqDeleteMessages(true)

		if got := env.mreqEnforce(c); got.Outcome != memberReqWarned {
			t.Fatalf("outcome = %+v, want Warned", got)
		}
		if ids := env.mreqDeleted(); len(ids) != 1 || ids[0] != c.MessageID {
			t.Errorf("deleted messages = %v, want only %d", ids, c.MessageID)
		}
	})

	t.Run("an unreadable setting punishes nobody", func(t *testing.T) {
		env := newMemberReqEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionBan})
		env.mreqDeleteMessages(true)
		member := env.mreqMember("Alex", "", "alex")
		c := env.mreqCase(member)
		mreqSwap(t, &memberReqSettingsLookup, func(context.Context, int64) (*models.ChatRequirements, error) {
			return nil, fmt.Errorf("settings unreadable")
		})

		got := env.mreqEnforce(c)
		if got.Outcome != memberReqNone || got.Hold != nil {
			t.Fatalf("outcome = %+v, want None with no hold", got)
		}
		if n := env.mreqCallCount("restrictChatMember", "banChatMember", "unbanChatMember", "deleteMessage", "sendMessage"); n != 0 {
			t.Errorf("writes, deletes and messages = %d, want none", n)
		}
		if hold := env.mreqHold(member.Id); hold != nil {
			t.Errorf("hold = %+v, want none", hold)
		}
	})
}
