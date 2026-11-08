//go:build testtools

package modules

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"
	logrustest "github.com/sirupsen/logrus/hooks/test"

	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// mreqPartialMember mints a member with the given names who has a profile photo and whose
// standing in channel is status, so a test decides which rules they break by the names and the
// status alone.
func mreqPartialMember(t *testing.T, env *memberReqEnv, state *mreqLookupState, channel int64, last, status string) gotgbot.User {
	t.Helper()
	member := env.mreqMember("Alex", last, "alexsmith")
	mreqLookupCleanup(t, []int64{member.Id}, nil)
	state.set(member.Id, mreqHasPhoto)
	state.setMember(channel, member.Id, mreqChannelMember{Status: status})
	return member
}

// TestMemberReqPartialEpisode is WR-02: a lookup that could not be answered must never leave a
// weaker punishment standing with a hold that keeps a stricter rule from being judged, while it
// still never blocks a same-or-stricter established breach.
func TestMemberReqPartialEpisode(t *testing.T) {
	boom := errors.New("connection reset by peer")

	t.Run("a stricter channel rule whose lookup was shed defers the weaker name punishment and is judged at the next message", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		env.mreqRules(map[memberreq.Rule]int{
			memberreq.RuleLastName: models.RequirementActionWarn,
			memberreq.RuleChannel:  models.RequirementActionBan,
		})
		channel := mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
		member := mreqPartialMember(t, env, state, channel, "", gotgbot.ChatMemberStatusLeft)
		state.scriptMember(channel, member.Id, boom)

		hook := logrustest.NewGlobal()
		previous := log.GetLevel()
		log.SetLevel(log.InfoLevel)
		t.Cleanup(func() { hook.Reset(); log.SetLevel(previous) })
		hook.Reset()

		mreqWantNothing(t, env, member, env.mreqEnforce(env.mreqCase(member)))
		informed := false
		for _, entry := range hook.AllEntries() {
			if entry.Level == log.InfoLevel && strings.Contains(entry.Message, strconv.FormatInt(member.Id, 10)) &&
				strings.Contains(entry.Message, strconv.FormatInt(env.chat.Id, 10)) {
				informed = true
				if strings.Contains(entry.Message, "Alex") {
					t.Errorf("the deferral line %q names the member", entry.Message)
				}
			}
		}
		if !informed {
			t.Error("no Info line with the chat and user IDs says the check was deferred")
		}

		got := env.mreqEnforce(env.mreqCase(member))
		if got.Outcome != memberReqRemoved || got.Hold == nil {
			t.Fatalf("second outcome = %+v, want Removed: the channel lookup answers a breach now", got)
		}
		if n := env.mreqCallCount("banChatMember"); n != 1 {
			t.Errorf("ban calls = %d, want exactly 1", n)
		}
		want := mreqBitOf(t, "ln") | mreqBitOf(t, "ch")
		if hold := env.mreqHold(member.Id); hold == nil || hold.Action != models.RequirementActionBan || hold.Rules != want {
			t.Errorf("hold = %+v, want a ban hold with the last name and channel bits (%d)", hold, want)
		}
		if row := env.mreqWarnRow(member.Id); row != nil {
			t.Errorf("warns row = %+v, want none: nobody was warned on the way", row)
		}
	})

	t.Run("a stricter photo rule whose lookup failed defers a weaker name punishment too", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		env.mreqRules(map[memberreq.Rule]int{
			memberreq.RuleLastName: models.RequirementActionWarn,
			memberreq.RulePhoto:    models.RequirementActionBan,
		})
		member := env.mreqMember("Alex", "", "alexsmith")
		mreqLookupCleanup(t, []int64{member.Id}, nil)
		state.set(member.Id, mreqPhotoAnswer{})
		state.script(member.Id, boom)

		mreqWantNothing(t, env, member, env.mreqEnforce(env.mreqCase(member)))
		if got := env.mreqEnforce(env.mreqCase(member)); got.Outcome != memberReqRemoved {
			t.Fatalf("second outcome = %+v, want Removed", got)
		}
	})

	t.Run("an unknown rule that is equal or weaker than the punishment does not defer it", func(t *testing.T) {
		for _, tc := range []struct {
			name          string
			nameAction    int
			channelAction int
			outcome       memberReqOutcome
			write         string
		}{
			{"a Ban name rule over an unknown Warn channel rule", models.RequirementActionBan, models.RequirementActionWarn, memberReqRemoved, "banChatMember"},
			{"a Ban name rule over an unknown Ban channel rule", models.RequirementActionBan, models.RequirementActionBan, memberReqRemoved, "banChatMember"},
			{"a Mute name rule over an unknown Mute channel rule", models.RequirementActionMute, models.RequirementActionMute, memberReqMuted, "restrictChatMember"},
			{"a Kick name rule over an unknown Mute channel rule", models.RequirementActionKick, models.RequirementActionMute, memberReqRemoved, "unbanChatMember"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				env, state := mreqLookupEnv(t)
				env.mreqRules(map[memberreq.Rule]int{
					memberreq.RuleLastName: tc.nameAction,
					memberreq.RuleChannel:  tc.channelAction,
				})
				channel := mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
				member := mreqPartialMember(t, env, state, channel, "", gotgbot.ChatMemberStatusLeft)
				state.scriptMember(channel, member.Id, boom)

				got := env.mreqEnforce(env.mreqCase(member))
				if got.Outcome != tc.outcome || got.Hold == nil {
					t.Fatalf("outcome = %+v, want %d with the hold", got, tc.outcome)
				}
				if n := env.mreqCallCount(tc.write); n != 1 {
					t.Errorf("%s calls = %d, want 1", tc.write, n)
				}
				if hold := env.mreqHold(member.Id); hold == nil || hold.Rules != mreqBitOf(t, "ln") || hold.Action != tc.nameAction {
					t.Errorf("hold = %+v, want the last name bit alone with action %d", hold, tc.nameAction)
				}
			})
		}
	})

	t.Run("an unknown rule with no established breach is simply not broken", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		env.mreqRules(map[memberreq.Rule]int{
			memberreq.RuleLastName: models.RequirementActionWarn,
			memberreq.RuleChannel:  models.RequirementActionBan,
		})
		channel := mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
		member := mreqPartialMember(t, env, state, channel, "Smith", gotgbot.ChatMemberStatusLeft)
		state.scriptMember(channel, member.Id, boom)

		mreqWantNothing(t, env, member, env.mreqEnforce(env.mreqCase(member)))
	})

	t.Run("a rule that is Off never defers anything", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionWarn})
		channel := mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
		member := mreqPartialMember(t, env, state, channel, "", gotgbot.ChatMemberStatusLeft)
		state.scriptMember(channel, member.Id, boom)

		if got := env.mreqEnforce(env.mreqCase(member)); got.Outcome != memberReqWarned {
			t.Fatalf("outcome = %+v, want Warned: the channel rule is Off", got)
		}
	})
}
