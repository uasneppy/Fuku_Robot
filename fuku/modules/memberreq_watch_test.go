//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/redis/go-redis/v9"

	"github.com/uasneppy/fuku_robot/fuku/db/approvals"
	"github.com/uasneppy/fuku_robot/fuku/db/captcha"
	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
)

// mreqGroupSeen is a handler that records every update that reaches its group, so a test can
// prove the watcher at group -2 let the update go on to the groups before and after the
// commands.
type mreqGroupSeen struct {
	mu     sync.Mutex
	counts map[string]int
}

func (s *mreqGroupSeen) record(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counts[name]++
}

func (s *mreqGroupSeen) count(name string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.counts[name]
}

// mreqWatchGroups installs one recording handler at group -1 (right after the watcher) and one
// at group 3 (after the commands) on the environment's dispatcher.
func (e *memberReqEnv) mreqWatchGroups() *mreqGroupSeen {
	seen := &mreqGroupSeen{counts: make(map[string]int)}
	e.dispatcher.AddHandlerToGroup(recordingHandler{name: "g-1", record: seen.record}, -1)
	e.dispatcher.AddHandlerToGroup(recordingHandler{name: "g3", record: seen.record}, 3)
	return seen
}

// mreqWatcherCtx builds the update context the dispatcher would hand the watcher for msg.
func (e *memberReqEnv) mreqWatcherCtx(msg *gotgbot.Message) *ext.Context {
	id := e.updateID()
	msg.MessageId = id
	return ext.NewContext(e.bot, &gotgbot.Update{UpdateId: id, Message: msg}, nil)
}

// mreqCallsAbout counts the recorded requests of the named methods that name userID, so a
// request another handler makes (the admin cache's lookup of the bot) is not counted.
func (e *memberReqEnv) mreqCallsAbout(userID int64, methods ...string) int {
	e.t.Helper()
	count := 0
	for _, method := range methods {
		for _, call := range e.fake.callsFor(method) {
			if staffParamInt(call.Params, "user_id") == userID {
				count++
			}
		}
	}
	return count
}

func TestMemberReqWatcherPunishesOnce(t *testing.T) {
	env := newMemberReqEnv(t)
	env.loadMemberReq()
	env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionWarn})
	violator := env.mreqMember("Alex", "", "alex")
	fine := env.mreqMember("Bo", "Li", "bo")

	// The first message is judged alone: the warn module's transaction reads and then writes,
	// and on the SQLite of this suite a concurrent writer (here the /rules command creating
	// its rows) makes it fail with "database is locked" (PostgreSQL waits instead). The nine
	// that follow find the hold; the race to claim it is proved with a mute in
	// TestMemberReqOnePunishmentAcrossReplicas.
	env.mreqSend(violator, "plain text")
	memberReqJobs.wait()
	env.mreqSend(violator, "/rules")
	for i := 0; i < 8; i++ {
		env.mreqSend(violator, "plain text")
	}
	memberReqJobs.wait()

	if row := env.mreqWarnRow(violator.Id); row == nil || row.NumWarns != 1 {
		t.Fatalf("warns row after ten messages = %+v, want exactly one warn", row)
	}
	if hold := env.mreqHold(violator.Id); hold == nil || hold.PunishedAt == nil || hold.Action != models.RequirementActionWarn {
		t.Fatalf("hold = %+v, want one punished warn hold", hold)
	}
	var notices, rulesReplies int
	for _, sent := range env.mreqNotices() {
		text := mreqText(sent)
		if strings.Contains(text, staffMarker("mreq_notice_warn")) {
			notices++
		}
		if strings.Contains(text, staffMarker("rules_no_rules_set")) {
			rulesReplies++
		}
	}
	if notices != 1 {
		t.Errorf("warn notices = %d, want exactly 1", notices)
	}
	if rulesReplies != 1 {
		t.Errorf("answers to /rules = %d, want 1 (the watcher never ends the update)", rulesReplies)
	}

	before := len(env.mreqNotices())
	env.mreqSend(fine, "hello")
	memberReqJobs.wait()
	if env.mreqHold(fine.Id) != nil || env.mreqWarnRow(fine.Id) != nil {
		t.Error("a member with a last name was held or warned")
	}
	if n := env.mreqCallsAbout(fine.Id, "getChatMember", "restrictChatMember", "banChatMember", "unbanChatMember"); n != 0 {
		t.Errorf("a clean member's message made %d Telegram calls about them, want none", n)
	}
	if after := len(env.mreqNotices()); after != before {
		t.Errorf("a clean member's message posted %d messages, want none", after-before)
	}
}

func TestMemberReqWatcherOffUntilLive(t *testing.T) {
	// Plan 13-13 turned enforcement on by default (init() stores true once the "I fixed it"
	// handler is registered), so this is the one existing expectation that changed with it.
	if !memberReqLive() {
		t.Fatal("memberReqLive() is false outside any test helper, want the production default true")
	}

	t.Run("with the switch off ten messages start nothing", func(t *testing.T) {
		env := newMemberReqEnv(t)
		withMemberReqLive(t, false)
		env.loadMemberReq()
		seen := env.mreqWatchGroups()
		env.mreqRules(map[memberreq.Rule]int{
			memberreq.RuleLastName: models.RequirementActionMute,
			memberreq.RuleUsername: models.RequirementActionMute,
		})
		violator := env.mreqMember("Alex", "", "")

		for i := 0; i < 10; i++ {
			env.mreqSend(violator, "message")
		}
		memberReqJobs.wait()

		if env.mreqHold(violator.Id) != nil {
			t.Error("a hold was written while the switch is off")
		}
		if n := env.mreqCallsAbout(violator.Id, "getChatMember", "restrictChatMember", "banChatMember", "unbanChatMember"); n != 0 {
			t.Errorf("%d Telegram calls about the violator were made while the switch is off, want none", n)
		}
		if n := len(env.mreqNotices()); n != 0 {
			t.Errorf("%d messages were posted while the switch is off, want none", n)
		}
		if env.mreqWarnRow(violator.Id) != nil {
			t.Error("a warn was written while the switch is off")
		}
		if seen.count("g-1") != 10 || seen.count("g3") != 10 {
			t.Errorf("later groups saw %d and %d of 10 updates, want all", seen.count("g-1"), seen.count("g3"))
		}
	})

	t.Run("a helper restores the previous value and not a fixed false", func(t *testing.T) {
		withMemberReqLive(t, true)
		t.Run("inner", func(t *testing.T) {
			withMemberReqLive(t, false)
			if memberReqLive() {
				t.Error("the inner helper did not turn the switch off")
			}
		})
		if !memberReqLive() {
			t.Error("the switch is off after the inner test ended, want the previous value true restored")
		}
	})
}

func TestMemberReqWatcherContinuesGroups(t *testing.T) {
	env := newMemberReqEnv(t)
	env.loadMemberReq()
	seen := env.mreqWatchGroups()
	env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionMute})

	type path struct {
		name  string
		setup func(t *testing.T) *gotgbot.Message
	}
	text := func(u gotgbot.User, s string) *gotgbot.Message {
		return &gotgbot.Message{Chat: env.chat, From: &u, Text: s, Date: time.Now().Unix()}
	}
	paths := []path{
		{"a violation handed to a job", func(t *testing.T) *gotgbot.Message {
			return text(env.mreqMember("Alex", "", "alex"), "violation")
		}},
		{"a clean member", func(t *testing.T) *gotgbot.Message {
			return text(env.mreqMember("Bo", "Li", "bo"), "clean")
		}},
		{"a clean member again (pass value)", func(t *testing.T) *gotgbot.Message {
			member := env.mreqMember("Cy", "Li", "cy")
			env.mreqSend(member, "first")
			return text(member, "second")
		}},
		{"a sender_chat message", func(t *testing.T) *gotgbot.Message {
			msg := text(env.mreqMember("Di", "", "di"), "as a channel")
			msg.SenderChat = &gotgbot.Chat{Id: -1009990001, Type: "channel", Title: "News"}
			return msg
		}},
		{"an automatic forward", func(t *testing.T) *gotgbot.Message {
			msg := text(env.mreqMember("Ed", "", "ed"), "forwarded")
			msg.IsAutomaticForward = true
			return msg
		}},
		{"a service message", func(t *testing.T) *gotgbot.Message {
			member := env.mreqMember("Fay", "", "fay")
			return &gotgbot.Message{Chat: env.chat, From: &member, NewChatTitle: "A new title", Date: time.Now().Unix()}
		}},
		{"a cached admin", func(t *testing.T) *gotgbot.Message {
			member := env.mreqMember("Gus", "", "gus")
			env.listAdmin(env.chat.Id, member.Id)
			cache.InvalidateAdminCache(env.chat.Id)
			t.Cleanup(func() { cache.InvalidateAdminCache(env.chat.Id) })
			cache.LoadAdminCache(env.bot, env.chat.Id)
			return text(member, "admin")
		}},
	}

	check := func(t *testing.T, name string, msg *gotgbot.Message) {
		t.Helper()
		err := memberReqModule.memberReqWatcher(env.bot, env.mreqWatcherCtx(msg))
		if !errors.Is(err, ext.ContinueGroups) {
			t.Errorf("%s: watcher returned %v, want ext.ContinueGroups", name, err)
		}
	}
	for _, p := range paths {
		msg := p.setup(t)
		check(t, p.name, msg)
		before, beforeLate := seen.count("g-1"), seen.count("g3")
		env.mreqProcess(msg)
		if seen.count("g-1") != before+1 || seen.count("g3") != beforeLate+1 {
			t.Errorf("%s: the update did not reach both group -1 and group 3", p.name)
		}
	}

	t.Run("the switch off", func(t *testing.T) {
		withMemberReqLive(t, false)
		check(t, "switch off", text(env.mreqMember("Hal", "", "hal"), "off"))
	})
	t.Run("no rule on", func(t *testing.T) {
		other := newMemberReqEnv(t)
		check(t, "no rule", &gotgbot.Message{Chat: other.chat, From: &gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Ivy"}, Text: "x", Date: time.Now().Unix()})
	})
	t.Run("a settings read error", func(t *testing.T) {
		mreqSwap(t, &memberReqSettingsLookup, func(context.Context, int64) (*models.ChatRequirements, error) {
			return nil, errors.New("database unavailable")
		})
		check(t, "settings error", text(env.mreqMember("Jo", "", "jo"), "error"))
	})
	memberReqJobs.wait()
}

// mreqPassKeyOf is the Redis key of a member's clean verdict in the environment's group.
func (e *memberReqEnv) mreqPassKeyOf(userID int64) string {
	return fmt.Sprintf("fuku:memberreq:pass:%d:%d", e.chat.Id, userID)
}

// mreqOldHold writes, straight through the repository, a hold of a warn that was claimed an
// hour ago and confirmed, so a clean message dated now deletes it when the clean predicate
// runs and leaves it when the predicate does not.
func (e *memberReqEnv) mreqOldHold(userID int64) {
	e.t.Helper()
	punished := time.Now().Add(-time.Hour)
	won, err := memberreq.ClaimHold(context.Background(), &models.ChatRequirementHold{
		ChatID: e.chat.Id, UserID: userID, Rules: 1, Action: models.RequirementActionWarn,
		CreatedAt: punished, PunishedAt: &punished,
	})
	if err != nil || !won {
		e.t.Fatalf("seed hold: won = %t, error = %v", won, err)
	}
}

func TestMemberReqPassCache(t *testing.T) {
	setup := func(t *testing.T) (*memberReqEnv, gotgbot.User) {
		env := newMemberReqEnv(t)
		env.loadMemberReq()
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionWarn})
		return env, env.mreqMember("Bo", "Li", "bo")
	}
	holdGone := func(env *memberReqEnv, userID int64) bool { return env.mreqHold(userID) == nil }

	t.Run("a clean first message writes the verdict and makes no Telegram call", func(t *testing.T) {
		env, member := setup(t)
		env.mreqSend(member, "hello")
		memberReqJobs.wait()

		key := env.mreqPassKeyOf(member.Id)
		got, err := env.redis.Get(key)
		if err != nil {
			t.Fatalf("pass key %s not written: %v", key, err)
		}
		settings, _ := memberreq.GetSettingsFresh(context.Background(), env.chat.Id)
		if want := memberReqPassValue(settings, memberReqNamesOf(member)); got != want || len(got) != 16 {
			t.Errorf("pass value = %q, want the 16-hex fingerprint %q", got, want)
		}
		if ttl := env.redis.TTL(key); ttl < 29*time.Minute || ttl > 30*time.Minute {
			t.Errorf("pass TTL = %v, want 30 minutes", ttl)
		}
		if strings.Contains(key, member.FirstName) || strings.Contains(got, member.FirstName) {
			t.Error("the pass key or value carries a name")
		}
		if n := env.mreqCallsAbout(member.Id, "getChatMember", "restrictChatMember", "banChatMember", "unbanChatMember"); n != 0 {
			t.Errorf("a clean message made %d Telegram calls about the member, want none", n)
		}
	})

	t.Run("the second message finds the value and runs no clean predicate", func(t *testing.T) {
		env, member := setup(t)
		env.mreqSend(member, "first")
		env.mreqOldHold(member.Id)
		env.mreqSend(member, "second")
		if holdGone(env, member.Id) {
			t.Error("the clean predicate ran on the second message, the pass value should have skipped it")
		}
	})

	t.Run("a rename misses and runs the predicate", func(t *testing.T) {
		env, member := setup(t)
		env.mreqSend(member, "first")
		env.mreqOldHold(member.Id)
		renamed := member
		renamed.LastName = "Lee"
		env.mreqSend(renamed, "after the rename")
		if !holdGone(env, member.Id) {
			t.Error("a renamed member's message did not run the clean predicate")
		}
		settings, _ := memberreq.GetSettingsFresh(context.Background(), env.chat.Id)
		if got, _ := env.redis.Get(env.mreqPassKeyOf(member.Id)); got != memberReqPassValue(settings, memberReqNamesOf(renamed)) {
			t.Errorf("pass value after the rename = %q, want the renamed member's fingerprint", got)
		}
	})

	t.Run("a settings change misses and runs the predicate", func(t *testing.T) {
		env, member := setup(t)
		env.mreqSend(member, "first")
		env.mreqOldHold(member.Id)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleUsername: models.RequirementActionKick})
		env.mreqSend(member, "after the change")
		if !holdGone(env, member.Id) {
			t.Error("a settings change did not make the next message run the clean predicate")
		}
	})

	t.Run("a violation never writes the verdict", func(t *testing.T) {
		env, _ := setup(t)
		violator := env.mreqMember("Alex", "", "alex")
		env.mreqSend(violator, "violation")
		memberReqJobs.wait()
		if env.redis.Exists(env.mreqPassKeyOf(violator.Id)) {
			t.Error("a pass value was written for a violator")
		}
	})

	t.Run("a message much older than now never writes the verdict", func(t *testing.T) {
		env, member := setup(t)
		env.mreqSendAt(member, "old", time.Now().Add(-time.Hour).Unix())
		if env.redis.Exists(env.mreqPassKeyOf(member.Id)) {
			t.Error("a delayed old message wrote a pass value that would hide a later hold from the predicate")
		}
	})

	t.Run("without Redis the predicate runs on every message and nobody is punished wrongly", func(t *testing.T) {
		env, member := setup(t)
		mreqSwap(t, &memberReqRedisClient, func() *redis.Client { return nil })
		env.mreqSend(member, "first")
		env.mreqOldHold(member.Id)
		env.mreqSend(member, "second")
		memberReqJobs.wait()
		if !holdGone(env, member.Id) {
			t.Error("with no pass cache the predicate did not run")
		}
		if env.mreqWarnRow(member.Id) != nil {
			t.Error("a clean member was warned")
		}
	})
}

func TestMemberReqCleanEndsWarnHold(t *testing.T) {
	env := newMemberReqEnv(t)
	env.loadMemberReq()
	env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionWarn})
	member := env.mreqMember("Alex", "", "alex")

	env.mreqSend(member, "no last name yet")
	memberReqJobs.wait()
	hold := env.mreqHold(member.Id)
	if hold == nil || hold.PunishedAt == nil {
		t.Fatalf("hold after the first violation = %+v, want a punished warn hold", hold)
	}

	t.Run("a message dated before the hold never deletes it", func(t *testing.T) {
		fixed := member
		fixed.LastName = "Smith"
		env.mreqSendAt(fixed, "an old message processed late", hold.CreatedAt.Add(-time.Hour).Unix())
		if env.mreqHold(member.Id) == nil {
			t.Fatal("a message older than the hold deleted it")
		}
	})

	t.Run("a later clean message ends the episode and a new break is a new episode", func(t *testing.T) {
		fixed := member
		fixed.LastName = "Smith"
		env.mreqSendAt(fixed, "now with a last name", time.Now().Add(5*time.Second).Unix())
		if env.mreqHold(member.Id) != nil {
			t.Fatal("the hold is still there after a clean message more than 2 s later")
		}
		env.mreqSend(member, "no last name again")
		memberReqJobs.wait()

		row := env.mreqWarnRow(member.Id)
		if row == nil || row.NumWarns != 2 {
			t.Fatalf("warns after the second episode = %+v, want 2", row)
		}
		var notices int
		for _, sent := range env.mreqNotices() {
			if strings.Contains(mreqText(sent), staffMarker("mreq_notice_warn")) {
				notices++
			}
		}
		if notices != 2 {
			t.Errorf("warn notices = %d, want one per episode (2)", notices)
		}
	})
}

func TestMemberReqWatcherExemptions(t *testing.T) {
	env := newMemberReqEnv(t)
	env.loadMemberReq()
	env.mreqRules(map[memberreq.Rule]int{
		memberreq.RuleLastName: models.RequirementActionMute,
		memberreq.RuleUsername: models.RequirementActionMute,
	})
	text := func(u gotgbot.User) *gotgbot.Message {
		return &gotgbot.Message{Chat: env.chat, From: &u, Text: "hello", Date: time.Now().Unix()}
	}
	violator := func() gotgbot.User { return env.mreqMember("Alex", "", "") }

	// exempt runs send and asserts that the user was neither checked nor held nor noticed.
	exempt := func(name string, userID int64, send func()) {
		t.Run(name, func(t *testing.T) {
			notices := len(env.mreqNotices())
			send()
			memberReqJobs.wait()
			if env.mreqHold(userID) != nil {
				t.Error("a hold was written")
			}
			if n := env.mreqCallsAbout(userID, "getChatMember", "restrictChatMember", "banChatMember", "unbanChatMember"); n != 0 {
				t.Errorf("%d Telegram calls were made about the member, want none", n)
			}
			if after := len(env.mreqNotices()); after != notices {
				t.Errorf("%d messages were posted, want none", after-notices)
			}
		})
	}

	admin := violator()
	env.listAdmin(env.chat.Id, admin.Id)
	cache.InvalidateAdminCache(env.chat.Id)
	t.Cleanup(func() { cache.InvalidateAdminCache(env.chat.Id) })
	cache.LoadAdminCache(env.bot, env.chat.Id)
	exempt("a member the cached admin list knows as admin", admin.Id, func() { env.mreqProcess(text(admin)) })

	approved := violator()
	if err := approvals.AddApprovedUser(env.chat.Id, approved.Id, env.admin.Id, "trusted"); err != nil {
		t.Fatalf("approve: %v", err)
	}
	exempt("an approved member", approved.Id, func() { env.mreqProcess(text(approved)) })

	pending := violator()
	if _, err := captcha.CreateCaptchaAttemptPreMessage(pending.Id, env.chat.Id, "answer", 5); err != nil {
		t.Fatalf("pending captcha: %v", err)
	}
	exempt("a member with a pending captcha", pending.Id, func() { env.mreqProcess(text(pending)) })

	anon := violator()
	exempt("a message sent as the group", anon.Id, func() {
		msg := text(anon)
		msg.SenderChat = &gotgbot.Chat{Id: env.chat.Id, Type: "supergroup"}
		env.mreqProcess(msg)
	})
	channel := violator()
	exempt("a message sent as a channel", channel.Id, func() {
		msg := text(channel)
		msg.SenderChat = &gotgbot.Chat{Id: -1009990002, Type: "channel"}
		env.mreqProcess(msg)
	})
	forward := violator()
	exempt("an automatic forward", forward.Id, func() {
		msg := text(forward)
		msg.IsAutomaticForward = true
		env.mreqProcess(msg)
	})
	for _, id := range []int64{777000, 1087968824, 136817688} {
		exempt(fmt.Sprintf("service account %d", id), id, func() {
			env.mreqProcess(text(gotgbot.User{Id: id, FirstName: "Telegram"}))
		})
	}
	edited := violator()
	exempt("an edited message", edited.Id, func() {
		id := env.updateID()
		msg := text(edited)
		msg.MessageId = id
		msg.EditDate = time.Now().Unix()
		if err := env.dispatcher.ProcessUpdate(env.bot, &gotgbot.Update{UpdateId: id, EditedMessage: msg}, nil); err != nil {
			t.Errorf("ProcessUpdate error = %v", err)
		}
	})
	private := violator()
	exempt("a message in a private chat", private.Id, func() {
		msg := text(private)
		msg.Chat = gotgbot.Chat{Id: private.Id, Type: "private", FirstName: private.FirstName}
		env.mreqProcess(msg)
	})
	basic := violator()
	exempt("a message in a basic group", basic.Id, func() {
		msg := text(basic)
		msg.Chat = gotgbot.Chat{Id: env.chat.Id, Type: "group", Title: "Basic"}
		env.mreqProcess(msg)
	})
	robot := violator()
	exempt("a bot sender", robot.Id, func() {
		bot := robot
		bot.IsBot = true
		env.mreqProcess(text(bot))
	})
	joiner := violator()
	exempt("a new_chat_members service message", joiner.Id, func() {
		msg := &gotgbot.Message{Chat: env.chat, From: &joiner, NewChatMembers: []gotgbot.User{joiner}, Date: time.Now().Unix()}
		if err := memberReqModule.memberReqWatcher(env.bot, env.mreqWatcherCtx(msg)); !errors.Is(err, ext.ContinueGroups) {
			t.Errorf("watcher returned %v", err)
		}
	})
	unreadable := violator()
	exempt("an unreadable setting", unreadable.Id, func() {
		mreqSwap(t, &memberReqSettingsLookup, func(context.Context, int64) (*models.ChatRequirements, error) {
			return nil, errors.New("database unavailable")
		})
		env.mreqProcess(text(unreadable))
	})
}

// mreqRestrictCalls counts the restrictChatMember requests about userID that the given
// environments' fakes recorded, together.
func mreqRestrictCalls(userID int64, envs ...*memberReqEnv) int {
	total := 0
	for _, env := range envs {
		total += env.mreqCallsAbout(userID, "restrictChatMember")
	}
	return total
}

// mreqNoticeCount counts the sent messages of the given environments that contain marker.
func mreqNoticeCount(marker string, envs ...*memberReqEnv) int {
	total := 0
	for _, env := range envs {
		for _, sent := range env.mreqNotices() {
			if strings.Contains(mreqText(sent), marker) {
				total++
			}
		}
	}
	return total
}

func TestMemberReqOnePunishmentAcrossReplicas(t *testing.T) {
	// sendAtOnce sends n messages of user at the same time, spread over the three processes.
	sendAtOnce := func(user gotgbot.User, n int, envs ...*memberReqEnv) {
		var wg sync.WaitGroup
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func(env *memberReqEnv) {
				defer wg.Done()
				env.mreqSend(user, "same message")
			}(envs[i%len(envs)])
		}
		wg.Wait()
		memberReqJobs.wait()
	}

	t.Run("a mute: ten messages at once give one restrict with the marker and one notice with the button", func(t *testing.T) {
		env := newMemberReqEnv(t)
		env.loadMemberReq()
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionMute})
		violator := env.mreqMember("Alex", "", "alex")
		first, second := env.mreqReplica(violator), env.mreqReplica(violator)

		sendAtOnce(violator, 10, env, first, second)

		if n := mreqRestrictCalls(violator.Id, env, first, second); n != 1 {
			t.Fatalf("restrictChatMember calls in all three processes = %d, want exactly 1", n)
		}
		hold := env.mreqHold(violator.Id)
		if hold == nil || hold.PunishedAt == nil || hold.Action != models.RequirementActionMute || hold.MuteUntil == 0 {
			t.Fatalf("hold = %+v, want one punished mute hold with its marker", hold)
		}
		var until int64
		for _, e := range []*memberReqEnv{env, first, second} {
			for _, call := range e.fake.callsFor("restrictChatMember") {
				if staffParamInt(call.Params, "user_id") == violator.Id {
					until = staffParamInt(call.Params, "until_date")
				}
			}
		}
		if until != hold.MuteUntil {
			t.Errorf("the restrict call's until_date = %d, want the hold's marker %d", until, hold.MuteUntil)
		}
		if n := mreqNoticeCount(staffMarker("mreq_notice_mute"), env, first, second); n != 1 {
			t.Errorf("mute notices in all three processes = %d, want exactly 1", n)
		}
		var buttons int
		for _, e := range []*memberReqEnv{env, first, second} {
			for _, sent := range e.mreqNotices() {
				for _, data := range mreqKeyboardData(sent) {
					if strings.HasPrefix(data, "mreq|") {
						buttons++
					}
				}
			}
		}
		if buttons != 1 {
			t.Errorf("I fixed it buttons = %d, want exactly 1", buttons)
		}
	})

	t.Run("a warn: the first message warns once and nine more at once across the processes add nothing", func(t *testing.T) {
		env := newMemberReqEnv(t)
		env.loadMemberReq()
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionWarn})
		violator := env.mreqMember("Alex", "", "alex")
		first, second := env.mreqReplica(violator), env.mreqReplica(violator)

		// One warn is judged alone first: the warn module's transaction reads and then writes,
		// and on SQLite a concurrent writer makes it fail with "database is locked" where
		// PostgreSQL waits. The claim race itself is the mute subtest above.
		env.mreqSend(violator, "first")
		memberReqJobs.wait()
		sendAtOnce(violator, 9, env, first, second)

		if row := env.mreqWarnRow(violator.Id); row == nil || row.NumWarns != 1 {
			t.Fatalf("warns row = %+v, want exactly one warn", row)
		}
		if n := mreqNoticeCount(staffMarker("mreq_notice_warn"), env, first, second); n != 1 {
			t.Errorf("warn notices in all three processes = %d, want exactly 1", n)
		}
	})
}

func TestMemberReqRunnerSheds(t *testing.T) {
	t.Run("the job deadline ends before a reclaim can start", func(t *testing.T) {
		if memberReqJobTimeout != 90*time.Second {
			t.Errorf("memberReqJobTimeout = %v, want 90s", memberReqJobTimeout)
		}
		if margin := memberreq.StaleClaim - memberReqJobTimeout; margin < 30*time.Second {
			t.Errorf("StaleClaim - memberReqJobTimeout = %v, want at least 30s", margin)
		}
	})

	t.Run("a full runner sheds the job with one log line and the next message is punished", func(t *testing.T) {
		env := newMemberReqEnv(t)
		env.loadMemberReq()
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionMute})
		first := env.mreqMember("Alex", "", "alex")
		second := env.mreqMember("Bea", "", "bea")
		mreqSwap(t, &memberReqMaxInFlight, 1)
		hook := mreqShedHook(t)
		t.Cleanup(hook.Reset)

		started, release := make(chan struct{}), make(chan struct{})
		var startOnce, freeOnce sync.Once
		freed := func() { freeOnce.Do(func() { close(release) }) }
		t.Cleanup(freed)
		env.fake.setOnGetMember(func(_, userID int64) {
			if userID == first.Id {
				startOnce.Do(func() { close(started) })
				<-release
			}
		})

		env.mreqSend(first, "blocks the only slot")
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("the first job never reached its lookup")
		}
		env.mreqSend(second, "shed")
		if env.mreqHold(second.Id) != nil {
			t.Error("the shed member got a hold")
		}
		var shed []string
		for _, entry := range hook.AllEntries() {
			if strings.Contains(entry.Message, "was shed") {
				shed = append(shed, entry.Message)
			}
		}
		if len(shed) != 1 {
			t.Fatalf("shed log lines = %d (%q), want 1", len(shed), shed)
		}
		for _, want := range []string{fmt.Sprint(second.Id), fmt.Sprint(env.chat.Id)} {
			if !strings.Contains(shed[0], want) {
				t.Errorf("shed line %q does not name %s", shed[0], want)
			}
		}
		for _, name := range []string{"Alex", "Bea"} {
			if strings.Contains(shed[0], name) {
				t.Errorf("shed line %q carries a name", shed[0])
			}
		}

		freed()
		memberReqJobs.wait()
		env.mreqSend(second, "checked again")
		memberReqJobs.wait()
		if hold := env.mreqHold(second.Id); hold == nil || hold.PunishedAt == nil {
			t.Errorf("hold of the shed member after their next message = %+v, want a punished mute", hold)
		}
		if hold := env.mreqHold(first.Id); hold == nil || hold.PunishedAt == nil {
			t.Errorf("hold of the first member = %+v, want a punished mute", hold)
		}
	})
}
