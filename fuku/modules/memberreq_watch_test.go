//go:build testtools

package modules

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

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
	if memberReqLive() {
		t.Fatal("memberReqLive() is true outside any test helper, want the production default false")
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
