//go:build testtools

package modules

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"
	logrustest "github.com/sirupsen/logrus/hooks/test"

	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// The two Telegram budgets of the member requirements, as Redis keys: the lookups' (photo and
// channel questions) and Phase 13's (punishments, notices and the live member read).
const (
	mreqLookupPaceNext  = "fuku:memberreq:lkp:pace:next"
	mreqLookupPaceBlock = "fuku:memberreq:lkp:pace:block"
	mreqPunishPaceNext  = "fuku:memberreq:pace:next"
	mreqPunishPaceBlock = "fuku:memberreq:pace:block"
	mreqLongBlock       = 30 * time.Second
)

// mreqBlockPacer writes a retry_after block of 30 s (longer than the lookup pacer's MaxWait,
// 5 s) on the pacer key and removes it when the test ends.
func mreqBlockPacer(t *testing.T, env *memberReqEnv, key string) {
	t.Helper()
	mreqSetKey(t, env, key, mreqLongBlock)
	t.Cleanup(func() { env.redis.Del(key) })
}

// mreqWantQuick fails when an entry that should shed at once ran for longer than a small
// multiple of the fast interval would explain (a shed makes no Telegram request and never waits).
func mreqWantQuick(t *testing.T, what string, started time.Time) {
	t.Helper()
	if took := time.Since(started); took > 2*time.Second {
		t.Errorf("%s took %s under a 30 s lookup block, want it to shed at once", what, took)
	}
}

// mreqBudgetJob is the enforcement job's entry: a member who breaks the last name rule and has
// no photo, both rules Mute. Blocked, the photo lookup is shed as unknown while the name
// breach is muted and its notice delivered at once; unblocked, both rules are established.
func mreqBudgetJob(t *testing.T, blocked bool) {
	env, state := mreqLookupEnv(t)
	env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionMute, memberreq.RulePhoto: models.RequirementActionMute})
	member := env.mreqMember("Alex", "", "alex")
	mreqLookupCleanup(t, []int64{member.Id}, nil)
	state.set(member.Id, mreqPhotoAnswer{})
	if blocked {
		mreqBlockPacer(t, env, mreqLookupPaceBlock)
	}

	started := time.Now()
	got := env.mreqEnforce(env.mreqCase(member))

	if got.Outcome != memberReqMuted {
		t.Fatalf("outcome = %+v, want Muted for the last name rule", got)
	}
	if n := len(env.mreqNotices()); n != 1 {
		t.Errorf("notices = %d, want the one mute notice", n)
	}
	hold := env.mreqHold(member.Id)
	if hold == nil {
		t.Fatal("no hold after the mute")
	}
	if blocked {
		mreqWantQuick(t, "the enforcement job", started)
		if n := mreqLookupPhotoCalls(env); n != 0 {
			t.Errorf("getUserProfilePhotos requests = %d, want none while the lookup budget is blocked", n)
		}
		if hold.Rules != mreqBitOf(t, "ln") {
			t.Errorf("hold rules = %d, want the last name bit only (the shed photo lookup is unknown)", hold.Rules)
		}
		return
	}
	if n := mreqLookupPhotoCalls(env); n != 1 {
		t.Errorf("getUserProfilePhotos requests = %d, want 1", n)
	}
	if want := mreqBitOf(t, "ln") | mreqBitOf(t, "ph"); hold.Rules != want {
		t.Errorf("hold rules = %d, want %d (last name and photo)", hold.Rules, want)
	}
	if !env.redis.Exists(mreqLookupPaceNext) {
		t.Errorf("the job's photo lookup did not write %s", mreqLookupPaceNext)
	}
}

// mreqBudgetJoin is the join entry: Check on join with the photo rule Mute and a joiner with no
// photo. Blocked, the lookup is shed and the joiner is welcomed at once; unblocked, it is
// muted and held.
func mreqBudgetJoin(t *testing.T, blocked bool) {
	env := mreqJoinEnv(t, map[memberreq.Rule]int{memberreq.RulePhoto: models.RequirementActionMute}, true)
	withFastLookupPacer(t)
	state := newMreqLookupState()
	mreqInstallLookupFake(env, state)
	member := mreqLookupMember(t, env, state, mreqPhotoAnswer{})
	if blocked {
		mreqBlockPacer(t, env, mreqLookupPaceBlock)
	}

	started := time.Now()
	env.mreqJoin(member)

	if blocked {
		mreqWantQuick(t, "the join entry", started)
		if n := mreqLookupPhotoCalls(env); n != 0 {
			t.Errorf("getUserProfilePhotos requests = %d, want none while the lookup budget is blocked", n)
		}
		mreqWantNothing(t, env, member, memberReqResult{})
		if got := mreqCountText(env.mreqSentTexts(), mreqWelcomeWord); got != 1 {
			t.Errorf("welcomes = %d, want 1 (a joiner whose lookup is shed is welcomed)", got)
		}
		return
	}
	if n := mreqLookupPhotoCalls(env); n != 1 {
		t.Errorf("getUserProfilePhotos requests = %d, want 1", n)
	}
	if hold := env.mreqHold(member.Id); hold == nil || hold.Rules != mreqBitOf(t, "ph") {
		t.Errorf("hold = %+v, want a hold with the photo bit", hold)
	}
	if !env.redis.Exists(mreqLookupPaceNext) {
		t.Errorf("the join entry's photo lookup did not write %s", mreqLookupPaceNext)
	}
}

// mreqBudgetFixed is the "I fixed it" press of a member muted for the photo rule. Blocked, its
// fresh lookup is shed and the press answers that it could not check; unblocked, the member
// still has no photo and the press says so.
func mreqBudgetFixed(t *testing.T, blocked bool) {
	env, _, member, hold, notice := mreqPhotoMuted(t)
	photoBefore := mreqLookupPhotoCalls(env)
	env.redis.Del(mreqLookupPaceNext)
	if blocked {
		mreqBlockPacer(t, env, mreqLookupPaceBlock)
	}

	started := time.Now()
	if blocked {
		mreqPressRefused(t, env, member, member, notice.MessageID, env.mreqFixedData(notice), "mreq_fixed_check_failed")
		mreqWantQuick(t, "the press", started)
		if n := mreqLookupPhotoCalls(env) - photoBefore; n != 0 {
			t.Errorf("getUserProfilePhotos requests by the press = %d, want none while the lookup budget is blocked", n)
		}
	} else {
		mreqPressRefused(t, env, member, member, notice.MessageID, env.mreqFixedData(notice), "mreq_fixed_still_broken", staffMarker("mreq_rule_photo"))
		if n := mreqLookupPhotoCalls(env) - photoBefore; n != 1 {
			t.Errorf("getUserProfilePhotos requests by the press = %d, want 1", n)
		}
		if !env.redis.Exists(mreqLookupPaceNext) {
			t.Errorf("the press's photo lookup did not write %s", mreqLookupPaceNext)
		}
	}
	mreqWantHeld(t, env, hold)
}

// mreqBudgetPage is the Requirements page with the channel rule on in a group whose linked
// channel the bot is only a member of. Blocked, its lookups are shed and the page draws the
// unknown line with no request; unblocked, it draws the not-administrator line.
func mreqBudgetPage(t *testing.T, blocked bool) {
	env := newSettingsMreqHereEnv(t)
	channel := mreqChannel(t, env.state, env.chat.Id, gotgbot.ChatMemberStatusMember)
	env.seed(settingsMreqChannelOn(), false, false)
	if blocked {
		mreqBlockPacer(t, env.memberReqEnv, mreqLookupPaceBlock)
	}
	msgID := env.openMenuOf(false)
	before := env.lookupCalls(channel)

	started := time.Now()
	text, _ := env.press(false, msgID, "a", "op", "p", settingsPageMreq)

	if blocked {
		mreqWantQuick(t, "the page open", started)
		if got := env.lookupCalls(channel) - before; got != 0 {
			t.Errorf("lookup requests by the page = %d, want none while the lookup budget is blocked", got)
		}
		if !strings.Contains(text, settingsMreqHereLine("❔ ", "settings_mreq_here_unknown")) {
			t.Errorf("the page %q lacks the unknown line", text)
		}
		return
	}
	if got := env.lookupCalls(channel) - before; got == 0 {
		t.Error("the page made no lookup request")
	}
	if !strings.Contains(text, settingsMreqHereLine("🚫 ", "settings_mreq_here_notadmin")) {
		t.Errorf("the page %q lacks the not-administrator line", text)
	}
	if !env.redis.Exists(mreqLookupPaceNext) {
		t.Errorf("the page's lookups did not write %s", mreqLookupPaceNext)
	}
	for _, key := range []string{mreqPunishPaceNext, mreqPunishPaceBlock} {
		if env.redis.Exists(key) {
			t.Errorf("the page's lookups wrote the punishment budget's key %s", key)
		}
	}
}

func TestMemberReqPacersAreIndependent(t *testing.T) {
	entries := []struct {
		name string
		run  func(t *testing.T, blocked bool)
	}{
		{"the enforcement job", mreqBudgetJob},
		{"the join entry", mreqBudgetJoin},
		{"the I fixed it press", mreqBudgetFixed},
		{"the page", mreqBudgetPage},
	}
	for _, entry := range entries {
		t.Run(entry.name+": a lookup block sheds its lookups at once", func(t *testing.T) {
			entry.run(t, true)
		})
		t.Run(entry.name+": its lookups write the lookup budget's keys", func(t *testing.T) {
			entry.run(t, false)
		})
	}

	t.Run("a block on the punishment budget does not delay a page open", func(t *testing.T) {
		env := newSettingsMreqHereEnv(t)
		channel := mreqChannel(t, env.state, env.chat.Id, gotgbot.ChatMemberStatusMember)
		env.seed(settingsMreqChannelOn(), false, false)
		mreqBlockPacer(t, env.memberReqEnv, mreqPunishPaceBlock)
		msgID := env.openMenuOf(false)

		started := time.Now()
		text, _ := env.press(false, msgID, "a", "op", "p", settingsPageMreq)

		if took := time.Since(started); took > 2*time.Second {
			t.Errorf("the page open took %s under a 30 s block of the punishment budget, want it unaffected", took)
		}
		if !strings.Contains(text, settingsMreqHereLine("🚫 ", "settings_mreq_here_notadmin")) {
			t.Errorf("the page %q lacks the status its lookups established", text)
		}
		if env.lookupCalls(channel) == 0 {
			t.Error("the page made no lookup request")
		}
		if env.redis.Exists(mreqPunishPaceNext) {
			t.Errorf("the page's lookups wrote %s", mreqPunishPaceNext)
		}
		if got, err := env.redis.Get(mreqPunishPaceBlock); err != nil || got != "1" {
			t.Errorf("%s = %q (%v), want the block this test wrote, untouched", mreqPunishPaceBlock, got, err)
		}
	})

	t.Run("a block on the lookup budget does not delay the punishment of a name breach", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionKick, memberreq.RuleChannel: models.RequirementActionBan})
		channel := mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
		member := env.mreqMember("Alex", "", "alex")
		mreqLookupCleanup(t, []int64{member.Id}, nil)
		state.setMember(channel, member.Id, mreqChannelMember{Status: gotgbot.ChatMemberStatusLeft})
		mreqBlockPacer(t, env, mreqLookupPaceBlock)

		started := time.Now()
		got := env.mreqEnforce(env.mreqCase(member))

		mreqWantQuick(t, "the removal of a name breach", started)
		if got.Outcome != memberReqRemoved {
			t.Errorf("outcome = %+v, want Removed (the kick of the last name rule)", got)
		}
		if n := len(env.calls("unbanChatMember")); n != 1 {
			t.Errorf("kick calls = %d, want 1", n)
		}
		if n := len(env.calls("banChatMember")); n != 0 {
			t.Errorf("ban calls = %d, want none (the channel lookup was shed as unknown)", n)
		}
	})
}

// Recognisable texts of the log test: if a log line of a lookup path ever carries one of them,
// it carries something the logs must never hold (D-18, T-13-23).
const (
	mreqLogFileID      = "AgACAgSentinelPhotoFileID"
	mreqLogChannelName = "ZZ Channel Sentinel Title"
	mreqLogFirstName   = "ZZMemberFirstSentinel"
	mreqLogLastName    = "ZZMemberLastSentinel"
	mreqLogUserName    = "zzmembersentinel"
	// mreqLogDescription is a Telegram description: only the error code may reach a log.
	mreqLogDescription = "Bad Request: SentinelDescription"
	// mreqLogBotCredential stands in for the bot's API credential, which Telegram's file and
	// method URLs carry and a transport error can embed.
	mreqLogBotCredential = "1234567:AAHSentinelBotCredential"
)

// mreqLogCapture installs a hook for the test's duration, at debug level (the lookup layer
// logs routine failures there), and returns it with nothing captured yet.
func mreqLogCapture(t *testing.T) *logrustest.Hook {
	t.Helper()
	previous := log.GetLevel()
	log.SetLevel(log.DebugLevel)
	hook := logrustest.NewGlobal()
	hook.Reset()
	t.Cleanup(func() {
		hook.Reset()
		log.SetLevel(previous)
	})
	return hook
}

// mreqLogLines returns every captured entry as one string: its message and its fields.
func mreqLogLines(hook *logrustest.Hook) []string {
	var lines []string
	for _, entry := range hook.AllEntries() {
		line := entry.Message
		for key, value := range entry.Data {
			line += " " + key + "=" + fmt.Sprint(value)
		}
		lines = append(lines, line)
	}
	return lines
}

// mreqLogForbidden is what no captured line may contain whatever the path: the descriptions
// and names of the fakes' answers and the requirement notice texts (the marker locale writes the
// key of a message into its text, so a notice or a rule name in a log line shows up here).
var mreqLogForbidden = []string{
	mreqLogFileID, mreqLogChannelName, mreqLogFirstName, mreqLogLastName, mreqLogUserName,
	mreqLogDescription, mreqLogBotCredential, "test error", "Smith", "alexsmith", "Test Chat",
	"mreq_notice", "mreq_rule_", "mreq_remedy",
}

// mreqWantCleanLogs fails when a captured line holds any forbidden text, and when none of the
// lines holds one of the texts of mustSee (so the test cannot pass by capturing nothing).
func mreqWantCleanLogs(t *testing.T, hook *logrustest.Hook, forbidden []string, mustSee ...string) {
	t.Helper()
	lines := mreqLogLines(hook)
	for _, line := range lines {
		for _, text := range forbidden {
			if strings.Contains(line, text) {
				t.Errorf("a log line carries %q: %q", text, line)
			}
		}
	}
	all := strings.Join(lines, "\n")
	for _, want := range mustSee {
		if !strings.Contains(all, want) {
			t.Errorf("no log line holds %q: the failure shape it names was not logged (%d lines)", want, len(lines))
		}
	}
}

// mreqFailingTransport is a transport that always fails the way a dropped connection does.
type mreqFailingTransport struct{}

func (mreqFailingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("dial tcp 192.0.2.1:443: connect: connection refused")
}

func TestMemberReqLookupLogsCarryIDsOnly(t *testing.T) {
	actions := []int{models.RequirementActionMute, models.RequirementActionWarn, models.RequirementActionKick, models.RequirementActionBan}
	seen := []string{"telegram error 400", "telegram error 403", "telegram error 500", "rate limited", "had an empty answer"}

	t.Run("the job and the flights under every failure shape", func(t *testing.T) {
		hook := mreqLogCapture(t)
		for i, shape := range mreqUnanswerableShapes() {
			t.Run(shape.name, func(t *testing.T) {
				env, member := mreqUnanswerableEnv(t, shape, actions[i%len(actions)], false)
				env.mreqEnforce(env.mreqCase(member))
			})
		}
		mreqWantCleanLogs(t, hook, mreqLogForbidden, seen...)
	})

	t.Run("the I fixed it press under every failure shape", func(t *testing.T) {
		hook := mreqLogCapture(t)
		for _, shape := range mreqUnanswerableShapes() {
			t.Run(shape.name, func(t *testing.T) {
				env, member, notice, _ := mreqUnknownShape(t, shape)
				mreqPressRefused(t, env, member, member, notice.MessageID, env.mreqFixedData(notice), "mreq_fixed_check_failed")
			})
		}
		mreqWantCleanLogs(t, hook, mreqLogForbidden, seen...)
	})

	t.Run("the held release, with and without an answer", func(t *testing.T) {
		hook := mreqLogCapture(t)
		release := func(t *testing.T, rules map[memberreq.Rule]int, bits int, setup func(env *memberReqEnv, state *mreqLookupState, member gotgbot.User)) {
			t.Helper()
			env, state := mreqHeldEnv(t)
			env.loadMemberReq()
			env.mreqRules(rules)
			member := mreqLookupMember(t, env, state, mreqHasPhoto)
			setup(env, state, member)
			mreqSeedHeld(t, env, member, bits, models.RequirementActionWarn, time.Hour)
			env.mreqSend(member, "a message")
			memberReqJobs.wait()
		}
		t.Run("the photo answer is a server error", func(t *testing.T) {
			release(t, map[memberreq.Rule]int{memberreq.RulePhoto: models.RequirementActionWarn}, mreqBitOf(t, "ph"),
				func(_ *memberReqEnv, state *mreqLookupState, member gotgbot.User) {
					state.script(member.Id, staffFakeError(500, mreqLogDescription))
				})
		})
		t.Run("the photo is there and the hold is released", func(t *testing.T) {
			release(t, map[memberreq.Rule]int{memberreq.RulePhoto: models.RequirementActionWarn}, mreqBitOf(t, "ph"),
				func(*memberReqEnv, *mreqLookupState, gotgbot.User) {})
		})
		t.Run("the channel lookups are refused with 403 and the bot is no administrator", func(t *testing.T) {
			var channel int64
			release(t, map[memberreq.Rule]int{memberreq.RuleChannel: models.RequirementActionWarn}, mreqBitOf(t, "ch"),
				func(env *memberReqEnv, state *mreqLookupState, member gotgbot.User) {
					channel = mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
					state.scriptMember(channel, member.Id, staffFakeError(403, mreqLogDescription))
					state.scriptMember(channel, staffTestBotID, staffFakeError(403, mreqLogDescription))
				})
		})
		mreqWantCleanLogs(t, hook, mreqLogForbidden, "telegram error 500", "telegram error 403", "was released")
	})

	t.Run("the page's status read and the gate of a punishing channel press", func(t *testing.T) {
		hook := mreqLogCapture(t)
		errPlain := fmt.Errorf("connection reset by peer")
		for _, tc := range []struct {
			name   string
			script func(env *settingsMreqHereEnv, channel int64)
		}{
			{"getChat fails with a plain error", func(env *settingsMreqHereEnv, _ int64) {
				env.state.scriptChat(env.chat.Id, errPlain, errPlain)
			}},
			{"getChat fails with 400", func(env *settingsMreqHereEnv, _ int64) {
				env.state.scriptChat(env.chat.Id, staffFakeError(400, mreqLogDescription), staffFakeError(400, mreqLogDescription))
			}},
			{"getChat fails with 500", func(env *settingsMreqHereEnv, _ int64) {
				env.state.scriptChat(env.chat.Id, staffFakeError(500, mreqLogDescription), staffFakeError(500, mreqLogDescription))
			}},
			{"getChat answers null", func(env *settingsMreqHereEnv, _ int64) { env.state.nullGetChat(env.chat.Id) }},
			{"the bot's own status fails with 500", func(env *settingsMreqHereEnv, channel int64) {
				env.state.scriptMember(channel, staffTestBotID, staffFakeError(500, mreqLogDescription))
			}},
			{"the bot's own status answers an unknown status", func(env *settingsMreqHereEnv, channel int64) {
				env.state.setMember(channel, staffTestBotID, mreqChannelMember{Status: "something_new"})
			}},
			{"the lookup pacer is blocked", func(env *settingsMreqHereEnv, _ int64) {
				mreqBlockPacer(env.t, env.memberReqEnv, mreqLookupPaceBlock)
			}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				env, channel := settingsMreqCostEnv(t)
				env.seed(settingsMreqChannelOn(), false, false)
				tc.script(env, channel)
				msgID := env.openMenuOf(false)
				env.press0(false, msgID, "a", "op", "p", settingsPageMreq)
				env.press0(false, msgID, settingsMreqChannelStrictKV(models.RequirementActionMute)...)
				env.press0(false, msgID, "a", settingsActMreqOpen, "t", "ch")
			})
		}
		mreqWantCleanLogs(t, hook, mreqLogForbidden, "telegram error 500", "telegram error 400", "rate limited")
	})

	t.Run("answers that carry a file ID, a title and names are never logged", func(t *testing.T) {
		hook := mreqLogCapture(t)
		env, state := mreqLookupEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RulePhoto: models.RequirementActionMute, memberreq.RuleChannel: models.RequirementActionMute})
		channel := mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
		state.setTitle(channel, mreqLogChannelName)
		fine := env.mreqMember(mreqLogFirstName, mreqLogLastName, mreqLogUserName)
		breaker := env.mreqMember(mreqLogFirstName+"2", mreqLogLastName+"2", mreqLogUserName+"2")
		mreqLookupCleanup(t, []int64{fine.Id, breaker.Id}, nil)
		state.set(fine.Id, mreqPhotoAnswer{Total: 1, List: true, FileID: mreqLogFileID})
		state.setMember(channel, fine.Id, mreqChannelMember{Status: gotgbot.ChatMemberStatusMember})
		state.set(breaker.Id, mreqPhotoAnswer{})
		state.setMember(channel, breaker.Id, mreqChannelMember{Status: gotgbot.ChatMemberStatusLeft})

		mreqWantNothing(t, env, fine, env.mreqEnforce(env.mreqCase(fine)))
		if got := env.mreqEnforce(env.mreqCase(breaker)); got.Outcome != memberReqMuted {
			t.Errorf("outcome for the member who breaks both rules = %+v, want Muted", got)
		}
		// A fresh lookup of both, as the press and the page make them.
		ctx := context.Background()
		memberReqPhotoRule(ctx, env.bot, env.chat.Id, fine.Id, memberReqLookupMode{Fresh: true})
		memberReqChannelRule(ctx, env.bot, env.chat.Id, fine.Id, memberReqLookupMode{Fresh: true, Classify: true})
		memberReqBotChannelStatus(ctx, env.bot, channel, memberReqLookupMode{Fresh: true})

		mreqWantCleanLogs(t, hook, mreqLogForbidden)
		for _, line := range mreqLogLines(hook) {
			if strings.Contains(line, mreqLogFirstName) || strings.Contains(line, mreqLogLastName) {
				t.Errorf("a log line carries a member's name: %q", line)
			}
		}
	})

	t.Run("a panic in a flight is recovered with one line that names no ID", func(t *testing.T) {
		hook := mreqLogCapture(t)
		const userID = int64(7340011223)
		key := memberReqFlightKey(fmt.Sprintf("p:%d", userID), memberReqLookupMode{})
		verdict := memberReqFlight(context.Background(), key, func(context.Context) memberReqVerdict {
			panic("flight failed")
		})
		if verdict != memberReqUnknown {
			t.Errorf("verdict after a panic = %d, want unknown (%d): a crash punishes nobody", verdict, memberReqUnknown)
		}
		recovered := 0
		for _, line := range mreqLogLines(hook) {
			if !strings.Contains(line, "Recovered from panic") {
				continue
			}
			recovered++
			if strings.Contains(line, fmt.Sprint(userID)) || !strings.Contains(line, "[MemberReq][memberReqFlight]") {
				t.Errorf("the recovery line %q must name the flight and no user ID", line)
			}
		}
		if recovered != 1 {
			t.Errorf("recovery lines = %d, want exactly 1", recovered)
		}
	})

	t.Run("a transport error and a Telegram description never put the bot's credential or the description in a line", func(t *testing.T) {
		hook := mreqLogCapture(t)
		mreqLookupEnv(t)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			if _, err := fmt.Fprintf(w, `{"ok":false,"error_code":400,"description":%q}`, mreqLogDescription); err != nil {
				t.Errorf("write the refusal: %v", err)
			}
		}))
		t.Cleanup(server.Close)
		clients := map[string]*gotgbot.BaseBotClient{
			"a dropped connection": {Client: http.Client{Transport: mreqFailingTransport{}}, DefaultRequestOpts: &gotgbot.RequestOpts{Timeout: 5 * time.Second}},
			"a refused request":    {Client: *server.Client(), DefaultRequestOpts: &gotgbot.RequestOpts{APIURL: server.URL, Timeout: 5 * time.Second}},
		}
		for name, client := range clients {
			bot := &gotgbot.Bot{Token: mreqLogBotCredential, User: gotgbot.User{Id: staffTestBotID, IsBot: true}, BotClient: client}
			chat, user, channel := uniqueModuleChatID(), uniqueLinkOwnerID(), uniqueModuleChatID()
			mreqLookupCleanup(t, []int64{user}, []int64{chat, channel})
			ctx := context.Background()
			if got := memberReqPhotoRule(ctx, bot, chat, user, memberReqLookupMode{}); got != memberReqUnknown {
				t.Errorf("%s: photo verdict = %d, want unknown", name, got)
			}
			if _, known := memberReqLinkedChannel(ctx, bot, chat, memberReqLookupMode{}); known {
				t.Errorf("%s: the link of a group whose getChat failed is known", name)
			}
			if got := memberReqBotChannelStatus(ctx, bot, channel, memberReqLookupMode{}); got != memberReqBotUnknown && name == "a dropped connection" {
				t.Errorf("%s: bot status = %d, want unknown", name, got)
			}
		}
		mreqWantCleanLogs(t, hook, []string{mreqLogBotCredential, "AAHSentinel", mreqLogDescription}, "not answered")
	})
}

func TestMemberReqLookupAlertsFitEveryLocale(t *testing.T) {
	withRealLocales(t)
	const chatDigits, channelDigits = "1001234567890", "1002468013579"
	for _, code := range memberReqScreenLanguages {
		tr := i18n.MustNewTranslator(code)
		alerts := map[string]string{
			"refused channel press": settingsMreqNoLinkAlert(tr),
			"could not check":       memberReqText(tr, "mreq_fixed_check_failed"),
		}
		for name, text := range alerts {
			if text == "" {
				t.Errorf("%s %s: empty alert", code, name)
				continue
			}
			if got := utf8.RuneCountInString(text); got > 200 {
				t.Errorf("%s %s: %d code points, want at most 200 (Telegram's limit for an answer): %q", code, name, got, text)
			}
			for _, leak := range []string{chatDigits, channelDigits, settingsStaffPrivacyDigits, mreqLogChannelName, mreqLogFirstName} {
				if strings.Contains(text, leak) {
					t.Errorf("%s %s: the alert names %q: %q", code, name, leak, text)
				}
			}
			if strings.Contains(strings.ToLower(text), "staff") || strings.Contains(text, "{") || strings.Contains(text, memberReqNamePlaceholder) {
				t.Errorf("%s %s: the alert names the Staff Group or holds an unfilled token: %q", code, name, text)
			}
		}
		rule := settingsText(tr, "settings_mreq_rule_channel")
		if !strings.Contains(alerts["refused channel press"], rule) {
			t.Errorf("%s: the refused-press alert %q does not name the channel rule %q", code, alerts["refused channel press"], rule)
		}
	}
}
