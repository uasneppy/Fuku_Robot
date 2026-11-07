//go:build testtools

package modules

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/greetings"
	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/utils/tracing"
)

// mreqJoinNow runs the member's join as a chat_member update dated now, as a self-join. The
// environment's own join stamps Date 1 (1970), which the rejoin reset would read as a join long
// before every hold, so the rejoin tests send their joins through this one.
func (e *memberReqEnv) mreqJoinNow(user gotgbot.User) {
	e.t.Helper()
	id := e.updateID()
	update := &gotgbot.Update{
		UpdateId: id,
		ChatMember: &gotgbot.ChatMemberUpdated{
			Chat:          e.chat,
			From:          user,
			Date:          time.Now().Unix(),
			OldChatMember: gotgbot.ChatMemberLeft{User: user},
			NewChatMember: gotgbot.ChatMemberMember{User: user},
		},
	}
	if err := e.dispatcher.ProcessUpdate(e.bot, update, nil); err != nil {
		e.t.Fatalf("ProcessUpdate(%d) error = %v", id, err)
	}
}

// mreqLeave runs the chat_member update of the member going from member to the given
// status (left or kicked), as Telegram reports a kick, a ban or a member leaving.
func (e *memberReqEnv) mreqLeave(user gotgbot.User, to gotgbot.ChatMember) {
	e.t.Helper()
	e.memberUpdate(e.admin, gotgbot.ChatMemberMember{User: user}, to, false)
}

// mreqRejoinable puts the member back into the fake group as a plain member, as Telegram has
// them after a kick and a new join.
func (e *memberReqEnv) mreqRejoinable(user gotgbot.User) {
	e.t.Helper()
	e.fake.setMember(e.chat.Id, user.Id, staffFakeMember{
		Status:          gotgbot.ChatMemberStatusMember,
		IsMember:        true,
		CanSendMessages: true,
	})
}

func TestMemberReqKickBan(t *testing.T) {
	rule := func(action int) map[memberreq.Rule]int {
		return map[memberreq.Rule]int{memberreq.RuleLastName: action}
	}

	t.Run("a member kicked by a rule is checked and kicked again when they rejoin", func(t *testing.T) {
		env := mreqJoinEnv(t, rule(models.RequirementActionKick), true)
		member := env.mreqMember("Alex", "", "alex")

		env.mreqJoinNow(member)
		if got := env.mreqCallsAbout(member.Id, "unbanChatMember"); got != 1 {
			t.Fatalf("unbanChatMember calls after the first join = %d, want the one kick", got)
		}
		if env.mreqHold(member.Id) == nil {
			t.Fatal("no hold after the kick, want the episode recorded")
		}

		env.mreqLeave(member, gotgbot.ChatMemberLeft{User: member})
		if hold := env.mreqHold(member.Id); hold != nil {
			t.Fatalf("hold after the kick's leave update = %+v, want it cleared", hold)
		}

		env.mreqRejoinable(member)
		env.mreqJoinNow(member)

		if got := env.mreqCallsAbout(member.Id, "unbanChatMember"); got != 2 {
			t.Fatalf("unbanChatMember calls after the rejoin = %d, want the member kicked again", got)
		}
		texts := env.mreqSentTexts()
		if got := mreqCountText(texts, staffMarker("mreq_notice_kick")); got != 2 {
			t.Fatalf("kick notices = %d, want one per join (texts %v)", got, texts)
		}
		if got := mreqCountText(texts, mreqWelcomeWord); got != 0 {
			t.Fatalf("welcomes = %d, want none for a member removed twice", got)
		}
	})

	t.Run("a member banned by a rule stays banned and nothing unbans them", func(t *testing.T) {
		env := mreqJoinEnv(t, rule(models.RequirementActionBan), true)
		member := env.mreqMember("Alex", "", "alex")

		env.mreqJoinNow(member)
		if got := env.mreqCallsAbout(member.Id, "banChatMember"); got != 1 {
			t.Fatalf("banChatMember calls = %d, want the one ban", got)
		}

		env.mreqLeave(member, gotgbot.ChatMemberBanned{User: member})

		if hold := env.mreqHold(member.Id); hold != nil {
			t.Fatalf("hold after the ban's leave update = %+v, want it cleared", hold)
		}
		if got := env.mreqCallsAbout(member.Id, "unbanChatMember"); got != 0 {
			t.Fatalf("unbanChatMember calls = %d, want none: nothing of the requirements ever unbans", got)
		}
		if live := env.fake.member(env.chat.Id, member.Id); live == nil || live.Status != gotgbot.ChatMemberStatusKicked {
			t.Fatalf("member record = %+v, want the member still banned", live)
		}
	})

	t.Run("a member who leaves on their own has their warn hold cleared", func(t *testing.T) {
		env := mreqJoinEnv(t, rule(models.RequirementActionWarn), true)
		member := env.mreqMember("Alex", "", "alex")

		if got := env.mreqEnforce(env.mreqCase(member)); got.Outcome != memberReqWarned {
			t.Fatalf("outcome = %+v, want Warned", got)
		}
		if env.mreqHold(member.Id) == nil {
			t.Fatal("no warn hold, want the episode recorded")
		}

		env.mreqLeave(member, gotgbot.ChatMemberLeft{User: member})

		if hold := env.mreqHold(member.Id); hold != nil {
			t.Fatalf("hold after the member left = %+v, want it cleared", hold)
		}
		env.mreqRejoinable(member)
		if got := env.mreqEnforce(env.mreqCase(member)); got.Outcome != memberReqWarned {
			t.Fatalf("outcome of the next episode = %+v, want a new warn, not a held member", got)
		}
		if row := env.mreqWarnRow(member.Id); row == nil || row.NumWarns != 2 {
			t.Fatalf("warns row = %+v, want two warns, one per episode", row)
		}
	})

	t.Run("a hold that cannot be cleared is logged by IDs and the goodbye is still sent", func(t *testing.T) {
		env := mreqJoinEnv(t, rule(models.RequirementActionWarn), true)
		member := env.mreqMember("Alex", "", "alex")
		if err := greetings.SetGoodbyeText(env.chat.Id, "Bye {first}", "", nil, db.TEXT); err != nil {
			t.Fatalf("SetGoodbyeText error = %v", err)
		}
		if err := greetings.SetGoodbyeToggle(env.chat.Id, true); err != nil {
			t.Fatalf("SetGoodbyeToggle error = %v", err)
		}
		if got := env.mreqEnforce(env.mreqCase(member)); got.Outcome != memberReqWarned {
			t.Fatalf("outcome = %+v, want Warned", got)
		}
		hook := captureLogs(t, log.WarnLevel)

		// The update's own context is cancelled, so the hold statement fails while every other
		// read of the handler (greetings, captcha) still works.
		cancelled, cancel := context.WithCancel(context.Background())
		cancel()
		update := &gotgbot.Update{
			UpdateId: env.updateID(),
			ChatMember: &gotgbot.ChatMemberUpdated{
				Chat:          env.chat,
				From:          env.admin,
				Date:          time.Now().Unix(),
				OldChatMember: gotgbot.ChatMemberMember{User: member},
				NewChatMember: gotgbot.ChatMemberLeft{User: member},
			},
		}
		ctx := ext.NewContext(env.bot, update, nil)
		ctx.Data = map[string]any{tracing.ContextDataKey: cancelled}
		if err := greetingsModule.leftMember(env.bot, ctx); err != ext.EndGroups {
			t.Fatalf("leftMember error = %v, want EndGroups", err)
		}

		if got := mreqCountText(env.mreqSentTexts(), "Bye"); got != 1 {
			t.Fatalf("goodbyes = %d, want 1 although the hold could not be cleared", got)
		}
		if env.mreqHold(member.Id) == nil {
			t.Fatal("hold is gone, want it kept when its delete failed")
		}
		logged := false
		for _, entry := range hook.AllEntries() {
			if entry.Level <= log.ErrorLevel && strings.Contains(entry.Message, fmt.Sprint(member.Id)) &&
				strings.Contains(entry.Message, fmt.Sprint(env.chat.Id)) {
				logged = true
			}
		}
		if !logged {
			t.Fatalf("no error line naming user %d and chat %d (entries %v)", member.Id, env.chat.Id, hook.AllEntries())
		}
	})
}
