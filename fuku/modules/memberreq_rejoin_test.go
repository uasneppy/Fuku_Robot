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

// mreqJoinAt runs the member's join as a chat_member update with the given Unix date, as a
// self-join. The environment's own join stamps Date 1 (1970), which the rejoin reset would read
// as a join long before every hold, so the rejoin tests send their joins dated now (mreqJoinNow)
// or dated on purpose.
func (e *memberReqEnv) mreqJoinAt(user gotgbot.User, date int64) {
	e.t.Helper()
	id := e.updateID()
	update := &gotgbot.Update{
		UpdateId: id,
		ChatMember: &gotgbot.ChatMemberUpdated{
			Chat:          e.chat,
			From:          user,
			Date:          date,
			OldChatMember: gotgbot.ChatMemberLeft{User: user},
			NewChatMember: gotgbot.ChatMemberMember{User: user},
		},
	}
	if err := e.dispatcher.ProcessUpdate(e.bot, update, nil); err != nil {
		e.t.Fatalf("ProcessUpdate(%d) error = %v", id, err)
	}
}

// mreqJoinNow runs the member's join as a chat_member update dated now.
func (e *memberReqEnv) mreqJoinNow(user gotgbot.User) {
	e.t.Helper()
	e.mreqJoinAt(user, time.Now().Unix())
}

// mreqServiceJoinAt runs the member's join as the new_chat_members service message with the
// given Unix date.
func (e *memberReqEnv) mreqServiceJoinAt(user gotgbot.User, date int64) {
	e.t.Helper()
	id := e.updateID()
	update := &gotgbot.Update{
		UpdateId: id,
		Message: &gotgbot.Message{
			MessageId:      id,
			Date:           date,
			Chat:           e.chat,
			From:           &user,
			NewChatMembers: []gotgbot.User{user},
		},
	}
	if err := e.dispatcher.ProcessUpdate(e.bot, update, nil); err != nil {
		e.t.Fatalf("ProcessUpdate(%d) error = %v", id, err)
	}
}

// mreqServiceJoinNow runs the member's join as the new_chat_members service message dated now,
// the one delivery a requirement-muted member's rejoin produces.
func (e *memberReqEnv) mreqServiceJoinNow(user gotgbot.User) {
	e.t.Helper()
	e.mreqServiceJoinAt(user, time.Now().Unix())
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

// mreqAgeHold moves the hold's creation back a minute, so the rejoin reset (which deletes the
// holds created before the join's own date, a date with one second of resolution) finds it.
func (e *memberReqEnv) mreqAgeHold(holdID int64) {
	e.t.Helper()
	if err := db.DB.Model(&models.ChatRequirementHold{}).Where("id = ?", holdID).
		Update("created_at", time.Now().Add(-time.Minute)).Error; err != nil {
		e.t.Fatalf("age hold %d: %v", holdID, err)
	}
}

// mreqMutedByJoin joins a member of the rule's group (Check on join on, Mute on the last-name
// rule), returns them and their punished mute hold, aged so a rejoin finds it, and forgets the
// join's dedupe claim as the minutes between a leave and a rejoin do.
func mreqMutedByJoin(t *testing.T) (*memberReqEnv, gotgbot.User, *models.ChatRequirementHold) {
	t.Helper()
	env := mreqJoinEnv(t, map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionMute}, true)
	member := env.mreqMember("Alex", "", "alex")
	env.mreqJoinNow(member)
	hold := env.mreqHold(member.Id)
	if hold == nil || hold.Action != models.RequirementActionMute || hold.PunishedAt == nil || hold.MuteUntil == 0 {
		t.Fatalf("hold after the first join = %+v, want a punished mute hold with a marker", hold)
	}
	env.mreqAgeHold(hold.ID)
	clearRecentJoinProcessing(env.chat.Id, member.Id)
	return env, member, env.mreqHold(member.Id)
}

func TestMemberReqRejoinResetsMuteHold(t *testing.T) {
	t.Run("a muted member who leaves and rejoins is freed and checked again", func(t *testing.T) {
		env, member, old := mreqMutedByJoin(t)

		// The leave sent no update the greetings handlers act on (restricted to restricted), so
		// the rejoin's service message is the first thing that sees the member again.
		env.mreqServiceJoinNow(member)

		restricts := env.mreqRestricts(member.Id)
		if len(restricts) != 3 {
			t.Fatalf("restrictChatMember calls = %d, want the mute, its lift and the mute of the new check", len(restricts))
		}
		mreqWantUnmute(t, restricts[1])
		if until := staffParamInt(restricts[2].Params, "until_date"); until == 0 {
			t.Errorf("third restrict until_date = 0, want the marker of a new requirement mute")
		}
		hold := env.mreqHold(member.Id)
		if hold == nil || hold.ID == old.ID || hold.PunishedAt == nil || !hold.WelcomeHeld {
			t.Fatalf("hold = %+v, want a new punished mute hold (the old one was %d)", hold, old.ID)
		}
		texts := env.mreqSentTexts()
		if got := mreqCountText(texts, staffMarker("mreq_notice_mute")); got != 2 {
			t.Fatalf("mute notices = %d, want one per check (texts %v)", got, texts)
		}
		if got := mreqCountText(texts, mreqWelcomeWord); got != 0 {
			t.Fatalf("welcomes = %d, want none for a muted rejoiner", got)
		}
	})

	t.Run("a refused lift restores the hold with its ID and notice ID", func(t *testing.T) {
		for _, method := range []string{"restrictChatMember", "getChat", "getChatMember"} {
			t.Run(method, func(t *testing.T) {
				env, member, old := mreqMutedByJoin(t)
				env.fake.script(method, env.chat.Id, mreqRefusals(1)...)

				env.mreqServiceJoinNow(member)

				hold := env.mreqHold(member.Id)
				if hold == nil || hold.ID != old.ID || hold.NoticeMsgID != old.NoticeMsgID ||
					hold.MuteUntil != old.MuteUntil || hold.PunishedAt == nil || !hold.WelcomeHeld {
					t.Fatalf("hold = %+v, want the old hold %+v restored", hold, old)
				}
				live := env.fake.member(env.chat.Id, member.Id)
				if live == nil || live.Status != gotgbot.ChatMemberStatusRestricted || live.UntilDate != old.MuteUntil {
					t.Fatalf("member record = %+v, want the requirement mute untouched", live)
				}
				texts := env.mreqSentTexts()
				if got := mreqCountText(texts, staffMarker("mreq_notice_mute")); got != 1 {
					t.Fatalf("mute notices = %d, want only the first (texts %v)", got, texts)
				}
				if got := mreqCountText(texts, mreqWelcomeWord); got != 0 {
					t.Fatalf("welcomes = %d, want none while the member is still muted", got)
				}
			})
		}
	})

	t.Run("a hold created after the join date survives", func(t *testing.T) {
		env := mreqJoinEnv(t, map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionMute}, true)
		member := env.mreqMember("Alex", "", "alex")
		// The member posted before their join's service message was processed: the watcher
		// punished them, and that hold belongs to this very membership.
		if got := env.mreqEnforce(env.mreqCase(member)); got.Outcome != memberReqMuted {
			t.Fatalf("outcome = %+v, want Muted", got)
		}
		held := env.mreqHold(member.Id)

		env.mreqServiceJoinAt(member, time.Now().Add(-time.Minute).Unix())

		if hold := env.mreqHold(member.Id); hold == nil || hold.ID != held.ID {
			t.Fatalf("hold = %+v, want the hold %d kept", hold, held.ID)
		}
		if got := len(env.mreqRestricts(member.Id)); got != 1 {
			t.Fatalf("restrictChatMember calls = %d, want only the first mute", got)
		}
		if got := mreqCountText(env.mreqSentTexts(), mreqWelcomeWord); got != 0 {
			t.Fatalf("welcomes = %d, want none for a requirement-muted member", got)
		}
	})

	t.Run("an admin's mute is never lifted", func(t *testing.T) {
		env := mreqJoinEnv(t, map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionMute}, false)
		member := env.mreqMember("Alex", "", "alex")
		if got := env.mreqEnforce(env.mreqCase(member)); got.Outcome != memberReqMuted {
			t.Fatalf("outcome = %+v, want Muted", got)
		}
		old := env.mreqHold(member.Id)
		env.mreqAgeHold(old.ID)
		// An admin muted the member afterwards: their restriction replaced the marker date.
		env.fake.setMember(env.chat.Id, member.Id, staffFakeMember{
			Status:    gotgbot.ChatMemberStatusRestricted,
			IsMember:  true,
			UntilDate: time.Now().Add(24 * time.Hour).Unix(),
			Perms:     &gotgbot.ChatPermissions{},
		})
		clearRecentJoinProcessing(env.chat.Id, member.Id)

		env.mreqServiceJoinNow(member)

		if hold := env.mreqHold(member.Id); hold != nil {
			t.Fatalf("hold = %+v, want the old hold deleted", hold)
		}
		if got := len(env.mreqRestricts(member.Id)); got != 1 {
			t.Fatalf("restrictChatMember calls = %d, want only the first mute: the admin's mute is not ours to lift", got)
		}
		if got := env.mreqCallsAbout(member.Id, "unbanChatMember", "banChatMember"); got != 0 {
			t.Fatalf("ban or unban calls = %d, want none", got)
		}
	})

	t.Run("a warn, kick or ban hold is only deleted", func(t *testing.T) {
		for _, action := range []int{models.RequirementActionWarn, models.RequirementActionKick, models.RequirementActionBan} {
			t.Run(fmt.Sprint("action ", action), func(t *testing.T) {
				env := mreqJoinEnv(t, map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionMute}, false)
				member := env.mreqMember("Alex", "", "alex")
				punished := time.Now().Add(-time.Minute)
				old := &models.ChatRequirementHold{
					ChatID: env.chat.Id, UserID: member.Id, Rules: 1, Action: action,
					CreatedAt: punished, PunishedAt: &punished,
				}
				if won, err := memberreq.ClaimHold(context.Background(), old); err != nil || !won {
					t.Fatalf("ClaimHold = %v, %v", won, err)
				}
				env.mreqAgeHold(old.ID)

				env.mreqServiceJoinNow(member)

				if hold := env.mreqHold(member.Id); hold != nil {
					t.Fatalf("hold = %+v, want it deleted", hold)
				}
				if writes := env.mreqWrites(); len(writes) != 0 {
					t.Fatalf("ban, restrict or unban calls = %v, want none", writes)
				}
			})
		}
	})

	t.Run("during a lockdown the lift uses the stored pre-lockdown permissions", func(t *testing.T) {
		env, member, _ := mreqMutedByJoin(t)
		mreqStartLockdown(t, env.chat.Id, lockdownTestPrePermissions)
		// The group's live defaults are the locked set while the lockdown lasts.
		env.fake.setChatPermsRaw(env.chat.Id, `{"can_send_messages":false}`)

		memberReqJoinReset(env.bot, &env.chat, member, time.Now())

		restricts := env.mreqRestricts(member.Id)
		if len(restricts) != 2 {
			t.Fatalf("restrictChatMember calls = %d, want the mute and its lift", len(restricts))
		}
		wantSnapshotPermissions(t, permissionsOf(t, restricts[1]))
		if until := staffParamInt(restricts[1].Params, "until_date"); until != 0 {
			t.Errorf("lift until_date = %d, want none", until)
		}
		if hold := env.mreqHold(member.Id); hold != nil {
			t.Fatalf("hold = %+v, want it deleted", hold)
		}
	})

	t.Run("a join with no date is read as now", func(t *testing.T) {
		env := mreqJoinEnv(t, map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionWarn}, true)
		member := env.mreqMember("Alex", "", "alex")
		if got := env.mreqEnforce(env.mreqCase(member)); got.Outcome != memberReqWarned {
			t.Fatalf("outcome = %+v, want Warned", got)
		}
		old := env.mreqHold(member.Id)
		env.mreqAgeHold(old.ID)

		env.mreqJoinAt(member, 0)

		hold := env.mreqHold(member.Id)
		if hold == nil || hold.ID == old.ID {
			t.Fatalf("hold = %+v, want the old hold %d replaced by the new check's", hold, old.ID)
		}
		if row := env.mreqWarnRow(member.Id); row == nil || row.NumWarns != 2 {
			t.Fatalf("warns row = %+v, want a second warn for the new episode", row)
		}
	})
}

func TestMemberReqJoinResetOrder(t *testing.T) {
	user := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Alex"}
	var order []string
	deps := func(claim bool) MembershipDeps {
		return MembershipDeps{
			Claim: func(chatID, userID int64) bool {
				order = append(order, "claim")
				return claim
			},
			Reset: func(u gotgbot.User) {
				if u.Id != user.Id {
					t.Errorf("Reset got user %d, want %d", u.Id, user.Id)
				}
				order = append(order, "reset")
			},
			Welcome: func(gotgbot.User) error {
				order = append(order, "welcome")
				return nil
			},
		}
	}

	if _, err := ProcessSingleJoin(-1001, 9, user, false, deps(true)); err != nil {
		t.Fatalf("ProcessSingleJoin error = %v", err)
	}
	if got := strings.Join(order, ","); got != "claim,reset,welcome" {
		t.Errorf("order of a won claim = %s, want claim,reset,welcome", got)
	}

	order = nil
	if _, err := ProcessSingleJoin(-1001, 9, user, false, deps(false)); err != nil {
		t.Fatalf("ProcessSingleJoin error = %v", err)
	}
	if got := strings.Join(order, ","); got != "claim" {
		t.Errorf("order of a lost claim = %s, want claim only: a second delivery resets nothing", got)
	}

	order = nil
	if _, err := ProcessSingleJoin(-1001, user.Id, user, false, deps(true)); err != nil {
		t.Fatalf("ProcessSingleJoin error = %v", err)
	}
	if len(order) != 0 {
		t.Errorf("calls for the bot itself = %v, want none", order)
	}
}

func TestMemberReqJoinAndMessageOnce(t *testing.T) {
	t.Run("a warn at the join and the member's next message give one warn and one notice", func(t *testing.T) {
		env := mreqJoinEnv(t, map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionWarn}, true)
		member := env.mreqMember("Alex", "", "alex")
		replica := env.mreqReplica(member)

		env.mreqJoinNow(member)
		replica.mreqSend(member, "hello")
		memberReqJobs.wait()

		if row := env.mreqWarnRow(member.Id); row == nil || row.NumWarns != 1 {
			t.Fatalf("warns row = %+v, want one warn for the join and the message together", row)
		}
		all := append(env.mreqSentTexts(), replica.mreqSentTexts()...)
		if got := mreqCountText(all, staffMarker("mreq_notice_warn")); got != 1 {
			t.Fatalf("warn notices = %d, want 1 (texts %v)", got, all)
		}
	})

	t.Run("a message and then the join give one warn and one notice", func(t *testing.T) {
		env := mreqJoinEnv(t, map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionWarn}, true)
		member := env.mreqMember("Alex", "", "alex")
		replica := env.mreqReplica(member)

		replica.mreqSend(member, "hello")
		memberReqJobs.wait()
		env.mreqJoinNow(member)

		if row := env.mreqWarnRow(member.Id); row == nil || row.NumWarns != 1 {
			t.Fatalf("warns row = %+v, want one warn", row)
		}
		all := append(env.mreqSentTexts(), replica.mreqSentTexts()...)
		if got := mreqCountText(all, staffMarker("mreq_notice_warn")); got != 1 {
			t.Fatalf("warn notices = %d, want 1 (texts %v)", got, all)
		}
	})

	t.Run("a mute at the join and a message on another replica at once give one mute", func(t *testing.T) {
		env := mreqJoinEnv(t, map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionMute}, true)
		member := env.mreqMember("Alex", "", "alex")
		replica := env.mreqReplica(member)

		done := make(chan struct{})
		go func() {
			defer close(done)
			replica.mreqSend(member, "hello")
		}()
		env.mreqJoinNow(member)
		<-done
		memberReqJobs.wait()

		if got := env.mreqCallsAbout(member.Id, "restrictChatMember") + replica.mreqCallsAbout(member.Id, "restrictChatMember"); got != 1 {
			t.Fatalf("restrictChatMember calls on both replicas = %d, want one mute", got)
		}
		all := append(env.mreqSentTexts(), replica.mreqSentTexts()...)
		if got := mreqCountText(all, staffMarker("mreq_notice_mute")); got != 1 {
			t.Fatalf("mute notices = %d, want 1 (texts %v)", got, all)
		}
		if hold := env.mreqHold(member.Id); hold == nil || hold.Action != models.RequirementActionMute {
			t.Fatalf("hold = %+v, want one mute hold", hold)
		}
	})
}
