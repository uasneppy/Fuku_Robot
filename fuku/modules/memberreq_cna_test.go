//go:build testtools

package modules

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"
	logrustest "github.com/sirupsen/logrus/hooks/test"

	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// mreqCnaKey is the literal key of the channel-level "the bot is not an administrator" fact (WR-01).
// It names the channel only and never a user.
func mreqCnaKey(channel int64) string {
	return "fuku:memberreq:cna:" + strconv.FormatInt(channel, 10)
}

// mreqChannelMemberCalls counts the getChatMember requests the fake saw for any user in chat.
func mreqChannelMemberCalls(e *memberReqEnv, chat int64) int {
	n := 0
	for _, call := range e.fake.callsFor("getChatMember") {
		if staffParamInt(call.Params, "chat_id") == chat {
			n++
		}
	}
	return n
}

// TestMemberReqChannelNotAdminCache is WR-01: a group linked to a channel the bot cannot read
// costs one channel-level fact per minute and not a runner job and paced calls per message.
func TestMemberReqChannelNotAdminCache(t *testing.T) {
	t.Run("a bot that is no administrator is cached for the channel and the next member costs no channel request", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleChannel: models.RequirementActionBan})
		first, channel := mreqChannelUser(t, env, state, gotgbot.ChatMemberStatusMember, gotgbot.ChatMemberStatusLeft)
		second := mreqLookupMember(t, env, state, mreqHasPhoto)
		state.setMember(channel, second.Id, mreqChannelMember{Status: gotgbot.ChatMemberStatusLeft})

		mreqWantNothing(t, env, first, env.mreqEnforce(env.mreqCase(first)))
		key := mreqCnaKey(channel)
		if value, err := env.redis.Get(key); err != nil || value != "1" {
			t.Fatalf("%s = %q (%v), want the fixed marker 1", key, value, err)
		}
		if ttl := env.redis.TTL(key); ttl <= 0 || ttl > time.Minute {
			t.Errorf("TTL of %s = %s, want at most 60 s", key, ttl)
		}

		channelBefore, chatBefore := mreqChannelMemberCalls(env, channel), mreqChatCalls(env, env.chat.Id)
		mreqWantNothing(t, env, second, env.mreqEnforce(env.mreqCase(second)))
		if n := mreqChannelMemberCalls(env, channel); n != channelBefore {
			t.Errorf("getChatMember requests for the channel = %d, want still %d (the cached fact answers)", n, channelBefore)
		}
		if n := mreqChatCalls(env, env.chat.Id); n != chatBefore {
			t.Errorf("getChat requests for the group = %d, want still %d", n, chatBefore)
		}
	})

	t.Run("an unknown member lookup followed by a bot that is no administrator writes it too", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleChannel: models.RequirementActionBan})
		channel := mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusMember)
		member := mreqLookupMember(t, env, state, mreqHasPhoto)
		state.scriptMember(channel, member.Id, staffFakeError(403, "Forbidden: bot is not a member of the channel chat"))

		mreqWantNothing(t, env, member, env.mreqEnforce(env.mreqCase(member)))
		if !env.redis.Exists(mreqCnaKey(channel)) {
			t.Errorf("%s does not exist after an unknown member lookup of a channel the bot does not administer", mreqCnaKey(channel))
		}
	})

	t.Run("a bot standing that cannot be established writes nothing", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			errs []error
		}{
			{"a 429 that keeps coming", []error{staffFake429(1), staffFake429(1)}},
			{"a 500", []error{staffFakeError(500, "Internal Server Error"), staffFakeError(500, "Internal Server Error")}},
			{"a transport error", []error{errors.New("connection reset by peer"), errors.New("connection reset by peer")}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				env, state := mreqLookupEnv(t)
				env.mreqRules(map[memberreq.Rule]int{memberreq.RuleChannel: models.RequirementActionBan})
				member, channel := mreqChannelUser(t, env, state, gotgbot.ChatMemberStatusMember, gotgbot.ChatMemberStatusLeft)
				state.scriptMember(channel, staffTestBotID, tc.errs...)

				mreqWantNothing(t, env, member, env.mreqEnforce(env.mreqCase(member)))
				if env.redis.Exists(mreqCnaKey(channel)) {
					t.Errorf("%s was written for a standing that could not be established", mreqCnaKey(channel))
				}
			})
		}
	})

	t.Run("a bot that administers the channel never writes it and a member who left is still a breach", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleChannel: models.RequirementActionBan})
		member, channel := mreqChannelUser(t, env, state, gotgbot.ChatMemberStatusAdministrator, gotgbot.ChatMemberStatusLeft)

		if got := env.mreqEnforce(env.mreqCase(member)); got.Outcome != memberReqRemoved {
			t.Fatalf("outcome = %+v, want Removed", got)
		}
		if env.redis.Exists(mreqCnaKey(channel)) {
			t.Errorf("%s was written for a bot that administers the channel", mreqCnaKey(channel))
		}
	})

	t.Run("a cached fact turns the rule inert at once and a Fresh lookup bypasses it and drops it on an administrator", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleChannel: models.RequirementActionBan})
		member, channel := mreqChannelUser(t, env, state, gotgbot.ChatMemberStatusAdministrator, gotgbot.ChatMemberStatusLeft)
		mreqSetKey(t, env, mreqCnaKey(channel), 50*time.Second)

		if got := mreqChannelVerdict(env, member.Id, memberReqLookupMode{}); got != memberReqInert {
			t.Fatalf("verdict with the fact cached = %d, want inert", got)
		}
		if n := mreqChannelMemberCalls(env, channel); n != 0 {
			t.Errorf("getChatMember requests for the channel = %d, want none while the fact is cached", n)
		}
		if got := mreqChannelVerdict(env, member.Id, memberReqLookupMode{Fresh: true}); got != memberReqBreach {
			t.Fatalf("Fresh verdict = %d, want breach: the bot is an administrator now", got)
		}
		if env.redis.Exists(mreqCnaKey(channel)) {
			t.Errorf("%s survived a Fresh answer that showed the bot as an administrator", mreqCnaKey(channel))
		}
	})

	t.Run("the watcher's cached read counts the channel rule as satisfied while the fact is set", func(t *testing.T) {
		env, state := mreqWatchLookupEnv(t)
		settings := env.mreqRules(map[memberreq.Rule]int{memberreq.RuleChannel: models.RequirementActionBan})
		channel := mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusMember)
		member := mreqLookupMember(t, env, state, mreqHasPhoto)
		mreqSetValue(t, env, memberReqLinkedKey(env.chat.Id), strconv.FormatInt(channel, 10), time.Minute)

		if satisfied, _ := memberReqLookupCached(settings, env.chat.Id, member.Id, "x"); satisfied {
			t.Fatal("satisfied with the link cached and neither the subscription nor the fact: want a job")
		}
		mreqSetKey(t, env, mreqCnaKey(channel), 50*time.Second)
		if satisfied, _ := memberReqLookupCached(settings, env.chat.Id, member.Id, "x"); !satisfied {
			t.Error("not satisfied while the not-administrator fact is set: want no job")
		}

		starts := mreqCountJobStarts(t)
		env.mreqSend(member, "hello")
		memberReqJobs.wait()
		if n := starts(); n != 0 {
			t.Errorf("job starts while the fact is set = %d, want none", n)
		}
		if n := mreqLookupRequests(env, member) + mreqChannelMemberCalls(env, channel); n != 0 {
			t.Errorf("requests about the member and the channel = %d, want none", n)
		}
	})
}

// mreqShedHook captures the log at debug level for the test and returns the hook with nothing
// captured yet. A shed is logged at debug (WR-01), so a test that counts or reads shed lines
// needs the level raised, and the level goes back when the test ends.
func mreqShedHook(t *testing.T) *logrustest.Hook {
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

// mreqSetValue stores value under key in the environment's Redis with a TTL.
func mreqSetValue(t *testing.T, e *memberReqEnv, key, value string, ttl time.Duration) {
	t.Helper()
	if err := e.redis.Set(key, value); err != nil {
		t.Fatalf("set %s: %v", key, err)
	}
	e.redis.SetTTL(key, ttl)
}

// TestMemberReqShedLogIsDebug is the log half of WR-01: a shed storm must not flood the Warn
// log, and the line still names the IDs only.
func TestMemberReqShedLogIsDebug(t *testing.T) {
	env := newMemberReqEnv(t)
	env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionWarn})
	member := env.mreqMember("Alex", "", "alex")
	mreqSwap(t, &memberReqMaxInFlight, 0)

	previous := log.GetLevel()
	t.Cleanup(func() { log.SetLevel(previous) })

	log.SetLevel(log.InfoLevel)
	hook := logrustest.NewGlobal()
	t.Cleanup(hook.Reset)
	hook.Reset()
	if memberReqRunAsync(env.bot, env.mreqCase(member)) {
		t.Fatal("a full runner started a job")
	}
	for _, entry := range hook.AllEntries() {
		if strings.Contains(entry.Message, "was shed") {
			t.Errorf("a shed was logged at %s, want debug only: %q", entry.Level, entry.Message)
		}
	}

	log.SetLevel(log.DebugLevel)
	hook.Reset()
	memberReqRunAsync(env.bot, env.mreqCase(member))
	lines := 0
	for _, entry := range hook.AllEntries() {
		if !strings.Contains(entry.Message, "was shed") {
			continue
		}
		lines++
		if entry.Level != log.DebugLevel {
			t.Errorf("shed line level = %s, want debug", entry.Level)
		}
		if !strings.Contains(entry.Message, strconv.FormatInt(member.Id, 10)) || strings.Contains(entry.Message, "Alex") {
			t.Errorf("shed line %q must name the IDs and no name", entry.Message)
		}
	}
	if lines != 1 {
		t.Errorf("shed lines at debug level = %d, want 1", lines)
	}
}
