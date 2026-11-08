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
