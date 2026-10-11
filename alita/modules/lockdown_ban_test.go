//go:build testtools

package modules

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/approvals"
	"github.com/divkix/Alita_Robot/alita/db/models"
)

// banEnv is a lockdown env linked to a Staff Group that also deletes the burst rows
// and the approvals of its chat when the test ends.
func banEnv(t *testing.T) (*lockdownEnv, gotgbot.Chat) {
	t.Helper()
	env := newLockdownEnv(t)
	staffChat := lockdownBurstEnv(env)
	t.Cleanup(func() {
		_ = approvals.RemoveAllApprovedUsers(env.chat.Id)
	})
	return env, staffChat
}

// banAlert is the lockdown alert in the Staff Group with the data of its Ban button.
type banAlert struct {
	staffChat gotgbot.Chat
	alert     staffSentMessage
	banData   string
}

// lastAlert reads the one alert the Staff Group received and its Ban button.
func lastAlert(t *testing.T, env *lockdownEnv, staffChat gotgbot.Chat) banAlert {
	t.Helper()
	sent := env.fake.sentTo(staffChat.Id)
	if len(sent) < 1 {
		t.Fatal("the Staff Group received no alert")
	}
	alert := sent[0]
	return banAlert{
		staffChat: staffChat,
		alert:     alert,
		banData:   lockdownButtonData(t, alert.Params["reply_markup"], "bn"),
	}
}

// banCard is the Ban confirm card the admin was shown.
type banCard struct {
	msg     staffSentMessage
	confirm string
	cancel  string
}

// askBan presses the alert's Ban button as from and returns the confirm card it
// opened, failing when no card was posted.
func (e *lockdownEnv) askBan(a banAlert, from gotgbot.User) banCard {
	e.t.Helper()
	before := len(e.fake.sentTo(a.staffChat.Id))
	pressLockdownButton(e, from, a.staffChat, a.alert.MessageID, a.banData)
	posted := e.fake.sentTo(a.staffChat.Id)
	if len(posted) != before+1 {
		e.t.Fatalf("%d messages in the Staff Group after the press, want %d (one confirm card)", len(posted), before+1)
	}
	card := posted[len(posted)-1]
	return banCard{
		msg:     card,
		confirm: lockdownButtonData(e.t, card.Params["reply_markup"], "bc"),
		cancel:  lockdownButtonData(e.t, card.Params["reply_markup"], "x"),
	}
}

// banRuns reads every ban run of the env's chat, oldest first.
func (e *lockdownEnv) banRuns() []models.ChatLockdownBanRun {
	e.t.Helper()
	var runs []models.ChatLockdownBanRun
	if err := db.DB.Where("chat_id = ?", e.chat.Id).Order("id").Find(&runs).Error; err != nil {
		e.t.Fatalf("read ban runs: %v", err)
	}
	return runs
}

// cycleUntilIdle runs worker cycles until one makes no progress.
func (e *lockdownEnv) cycleUntilIdle() {
	e.t.Helper()
	for i := 0; i < 12; i++ {
		if !e.cycle() {
			return
		}
	}
	e.t.Fatal("the worker still made progress after 12 cycles")
}

// wantNotBanned fails when a banChatMember was sent for userID.
func (e *lockdownEnv) wantNotBanned(userID int64, who string) {
	e.t.Helper()
	if got := len(e.memberCallsOf("banChatMember", userID)); got != 0 {
		e.t.Errorf("%d banChatMember calls for %s (%d), want none", got, who, userID)
	}
}

// wantBannedForGood fails unless the last banChatMember for userID carries no end
// date and the fake shows the account kicked with none.
func (e *lockdownEnv) wantBannedForGood(userID int64, who string) {
	e.t.Helper()
	bans := e.memberCallsOf("banChatMember", userID)
	if len(bans) == 0 {
		e.t.Errorf("no banChatMember for %s (%d)", who, userID)
		return
	}
	wantEmptyBan(e.t, bans[len(bans)-1])
	member := e.fake.member(e.chat.Id, userID)
	if member == nil || member.Status != gotgbot.ChatMemberStatusKicked || member.UntilDate != 0 {
		e.t.Errorf("%s (%d) = %+v in the fake, want kicked with until_date 0", who, userID, member)
	}
}

func TestLockdownBanRecentJoiners(t *testing.T) {
	t.Run("burst", func(t *testing.T) {
		env, staffChat := banEnv(t)

		p1, p2, p3 := env.newJoiner("P1"), env.newJoiner("P2"), env.newJoiner("P3")
		env.join(p1, p1, "")
		env.join(p2, p2, "")
		env.join(p3, p3, "")
		env.fake.setMember(env.chat.Id, p3.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusAdministrator})
		if err := approvals.AddApprovedUser(env.chat.Id, p2.Id, env.admin.Id, ""); err != nil {
			t.Fatalf("approve P2: %v", err)
		}

		env.send(env.admin, "/lockdown raid")
		time.Sleep(3 * time.Millisecond)
		j1 := env.newJoiner("J1")
		j2 := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "J2", IsBot: true}
		letIn := env.newJoiner("LetIn")
		env.join(j1, j1, "")
		env.join(j2, j2, "")
		env.join(letIn, env.admin, "")
		env.cycle()

		a := lastAlert(t, env, staffChat)
		keyboard := staffKeyboardOf(a.alert.Params["reply_markup"])
		if len(keyboard) != 2 || len(keyboard[1]) != 1 {
			t.Fatalf("alert keyboard = %+v, want a second row with the Ban button", keyboard)
		}
		if label := keyboard[1][0].Text; !strings.Contains(label, staffMarker("lockdown_alert_btn_ban")+" 4") {
			t.Errorf("Ban button label = %q, want it to carry %q with 4", label, staffMarker("lockdown_alert_btn_ban"))
		}

		// Being in the Staff Group grants nothing: a member of the locked group is refused.
		outsider := env.newOutsider()
		sentBefore := len(env.fake.sentTo(staffChat.Id))
		pressLockdownButton(env, outsider, staffChat, a.alert.MessageID, a.banData)
		if text, _ := env.lastAnswer(); !strings.Contains(text, staffMarker("lockdown_alert_not_admin")) {
			t.Errorf("refusal answer = %q, want %q", text, staffMarker("lockdown_alert_not_admin"))
		}
		if got := len(env.fake.sentTo(staffChat.Id)); got != sentBefore {
			t.Errorf("%d messages in the Staff Group, were %d: a refused press posts no card", got, sentBefore)
		}
		if runs := env.banRuns(); len(runs) != 0 {
			t.Fatalf("%d ban runs after a refused press, want none", len(runs))
		}

		// A live admin gets the card, a reply to the alert, with N and the two buttons.
		card := env.askBan(a, env.admin)
		text := fmt.Sprint(card.msg.Params["text"])
		if !strings.Contains(text, staffMarker("lockdown_ban_ask")+" 4 ") {
			t.Errorf("card text = %q, want %q with 4", text, staffMarker("lockdown_ban_ask"))
		}
		reply, ok := card.msg.Params["reply_parameters"].(*gotgbot.ReplyParameters)
		if !ok || reply.MessageId != a.alert.MessageID {
			t.Errorf("card reply_parameters = %+v, want a reply to the alert %d", card.msg.Params["reply_parameters"], a.alert.MessageID)
		}
		confirm, _ := decodeCallbackData(card.confirm, "lockdown")
		if confirm.Fields["u"] != fmt.Sprint(env.admin.Id) {
			t.Errorf("Confirm carries asker %q, want %d", confirm.Fields["u"], env.admin.Id)
		}
		if runs := env.banRuns(); len(runs) != 0 {
			t.Fatalf("%d ban runs before Confirm, want none", len(runs))
		}

		// Only the asker may confirm.
		pressLockdownButton(env, outsider, staffChat, card.msg.MessageID, card.confirm)
		if text, _ := env.lastAnswer(); !strings.Contains(text, staffMarker("lockdown_alert_not_yours")) {
			t.Errorf("another member's Confirm answered %q, want %q", text, staffMarker("lockdown_alert_not_yours"))
		}
		if runs := env.banRuns(); len(runs) != 0 {
			t.Fatalf("%d ban runs after another member's Confirm, want none", len(runs))
		}

		pressLockdownButton(env, env.admin, staffChat, card.msg.MessageID, card.confirm)
		edits := env.cardEdits(staffChat.Id, card.msg.MessageID)
		if len(edits) == 0 || !strings.Contains(edits[len(edits)-1], staffMarker("lockdown_ban_running")) {
			t.Fatalf("card edits = %q, want the last to read %q", edits, staffMarker("lockdown_ban_running"))
		}
		runs := env.banRuns()
		if len(runs) != 1 || runs[0].State != models.BanRunStateRunning {
			t.Fatalf("ban runs = %+v, want exactly one running run", runs)
		}
		if runs[0].StartedBy != env.admin.Id || runs[0].CardChatID != staffChat.Id || runs[0].CardMsgID != card.msg.MessageID {
			t.Errorf("run = %+v, want started by the admin with the card %d/%d", runs[0], staffChat.Id, card.msg.MessageID)
		}

		env.cycleUntilIdle()

		env.wantBannedForGood(p1.Id, "P1")
		env.wantBannedForGood(j1.Id, "J1")
		env.wantBannedForGood(j2.Id, "J2")
		env.wantNotBanned(p2.Id, "the approved user P2")
		env.wantNotBanned(p3.Id, "the admin P3")
		env.wantNotBanned(letIn.Id, "the user an admin let in")
		env.wantNotBanned(staffTestBotID, "the bot itself")
		if got := len(env.memberCallsOf("getChatMember", letIn.Id)); got != 0 {
			t.Errorf("%d getChatMember calls for the user an admin let in, want none", got)
		}

		if row := env.burstRowOf(p3.Id); row.State != models.BurstStateSkipped || row.Reason != lockdownBanSkipAdmin {
			t.Errorf("P3 burst row = %s/%s, want skipped/%s", row.State, row.Reason, lockdownBanSkipAdmin)
		}
		if row := env.burstRowOf(letIn.Id); row.State != models.BurstStateSkipped || row.Reason != lockdownBanSkipLetIn {
			t.Errorf("let-in burst row = %s/%s, want skipped/%s", row.State, row.Reason, lockdownBanSkipLetIn)
		}
		if row := env.joinerRow(letIn.Id); row.State != models.JoinerStateExempt {
			t.Errorf("let-in joiner row = %s, want it still exempt", row.State)
		}
		for _, user := range []gotgbot.User{j1, j2} {
			row := env.joinerRow(user.Id)
			if row.State != models.JoinerStateKept || row.Detail != models.JoinerDetailResponseBan {
				t.Errorf("joiner row of %d = %s/%s, want kept/%s", user.Id, row.State, row.Detail, models.JoinerDetailResponseBan)
			}
		}

		runs = env.banRuns()
		if len(runs) != 1 || runs[0].State != models.BanRunStateDone {
			t.Fatalf("ban runs = %+v, want the run done", runs)
		}
		edits = env.cardEdits(staffChat.Id, card.msg.MessageID)
		last := edits[len(edits)-1]
		if !strings.Contains(last, staffMarker("lockdown_ban_result")) || !strings.Contains(last, " 3 2 0") {
			t.Errorf("last card edit = %q, want %q with banned 3, skipped 2 and failed 0", last, staffMarker("lockdown_ban_result"))
		}
	})

	t.Run("request", func(t *testing.T) {
		env, staffChat := banEnv(t)

		env.send(env.admin, "/lockdown raid")
		requester := env.newJoiner("Requester")
		env.joinRequest(requester, "")
		env.cycle()
		if got := len(env.declinesOf(requester.Id)); got != 1 {
			t.Fatalf("%d declines of the join request, want 1", got)
		}

		a := lastAlert(t, env, staffChat)
		card := env.askBan(a, env.admin)
		pressLockdownButton(env, env.admin, staffChat, card.msg.MessageID, card.confirm)
		env.cycleUntilIdle()

		env.wantBannedForGood(requester.Id, "the requester")
		if row := env.burstRowOf(requester.Id); row.State != models.BurstStateBanned || row.Reason != lockdownBanReasonRequest {
			t.Errorf("requester burst row = %s/%s, want banned/%s", row.State, row.Reason, lockdownBanReasonRequest)
		}
		if runs := env.banRuns(); len(runs) != 1 || runs[0].State != models.BanRunStateDone {
			t.Errorf("ban runs = %+v, want one done run", runs)
		}
	})
}

// failApprovedLookup makes the approved users unreadable until the returned function is
// called (or the test ends).
func failApprovedLookup(t *testing.T) (restore func()) {
	t.Helper()
	previous := lockdownApprovedLookup
	lockdownApprovedLookup = func(int64) (map[int64]bool, error) {
		return nil, errors.New("approvals unreachable")
	}
	restored := false
	restore = func() {
		if !restored {
			lockdownApprovedLookup = previous
			restored = true
		}
	}
	t.Cleanup(restore)
	return restore
}

// lastCardText is the text of the last edit of a card, or "" when it was never edited.
func (e *lockdownEnv) lastCardText(chatID, msgID int64) string {
	edits := e.cardEdits(chatID, msgID)
	if len(edits) == 0 {
		return ""
	}
	return edits[len(edits)-1]
}

func TestLockdownBanCount(t *testing.T) {
	t.Run("excluded from N", func(t *testing.T) {
		env, staffChat := banEnv(t)

		raiders := []gotgbot.User{env.newJoiner("R1"), env.newJoiner("R2"), env.newJoiner("R3")}
		for _, user := range raiders {
			env.join(user, user, "")
		}
		approved := env.newJoiner("Approved")
		env.join(approved, approved, "")
		if err := approvals.AddApprovedUser(env.chat.Id, approved.Id, env.admin.Id, ""); err != nil {
			t.Fatalf("approve: %v", err)
		}

		env.send(env.admin, "/lockdown raid")
		rows := env.lockdownRows()
		if len(rows) != 1 {
			t.Fatalf("%d lockdown rows, want 1", len(rows))
		}
		// The bot's own ID in the burst, as the tracker could hold it.
		botRow := models.ChatLockdownBurst{
			LockdownID: rows[0].ID,
			ChatID:     env.chat.Id,
			UserID:     staffTestBotID,
			FirstName:  "Bot",
			Source:     models.BurstSourcePre,
		}
		if err := db.DB.Create(&botRow).Error; err != nil {
			t.Fatalf("seed the bot's burst row: %v", err)
		}
		time.Sleep(3 * time.Millisecond)
		letIn := env.newJoiner("LetIn")
		env.join(letIn, env.admin, "")
		env.cycle()

		a := lastAlert(t, env, staffChat)
		keyboard := staffKeyboardOf(a.alert.Params["reply_markup"])
		if len(keyboard) != 2 {
			t.Fatalf("alert keyboard = %+v, want two rows", keyboard)
		}
		if label := keyboard[1][0].Text; !strings.Contains(label, staffMarker("lockdown_alert_btn_ban")+" 3") {
			t.Errorf("Ban button label = %q, want N = 3", label)
		}
		card := env.askBan(a, env.admin)
		if text := fmt.Sprint(card.msg.Params["text"]); !strings.Contains(text, staffMarker("lockdown_ban_ask")+" 3 ") {
			t.Errorf("card text = %q, want N = 3", text)
		}
		pressLockdownButton(env, env.admin, staffChat, card.msg.MessageID, card.confirm)
		if text := env.lastCardText(staffChat.Id, card.msg.MessageID); !strings.Contains(text, staffMarker("lockdown_ban_running")+" 3 ") {
			t.Errorf("running card = %q, want N = 3", text)
		}
		env.cycleUntilIdle()
		for _, user := range raiders {
			env.wantBannedForGood(user.Id, "a raider")
		}
		env.wantNotBanned(approved.Id, "the approved user")
		env.wantNotBanned(letIn.Id, "the user an admin let in")
		env.wantNotBanned(staffTestBotID, "the bot")
	})

	t.Run("approved read fails at Ask", func(t *testing.T) {
		env, staffChat := banEnv(t)
		raider := env.newJoiner("R")
		env.join(raider, raider, "")
		env.send(env.admin, "/lockdown raid")
		env.cycle()
		a := lastAlert(t, env, staffChat)

		failApprovedLookup(t)
		sent := len(env.fake.sentTo(staffChat.Id))
		pressLockdownButton(env, env.admin, staffChat, a.alert.MessageID, a.banData)

		if text, _ := env.lastAnswer(); !strings.Contains(text, staffMarker("lockdown_ban_check_failed")) {
			t.Errorf("answer = %q, want %q", text, staffMarker("lockdown_ban_check_failed"))
		}
		if got := len(env.fake.sentTo(staffChat.Id)); got != sent {
			t.Errorf("%d messages in the Staff Group, were %d: no card may be posted", got, sent)
		}
		if runs := env.banRuns(); len(runs) != 0 {
			t.Errorf("%d ban runs, want none", len(runs))
		}
	})

	t.Run("approved read fails at Confirm", func(t *testing.T) {
		env, staffChat := banEnv(t)
		raider := env.newJoiner("R")
		env.join(raider, raider, "")
		env.send(env.admin, "/lockdown raid")
		env.cycle()
		a := lastAlert(t, env, staffChat)
		card := env.askBan(a, env.admin)

		failApprovedLookup(t)
		pressLockdownButton(env, env.admin, staffChat, card.msg.MessageID, card.confirm)

		if text := env.lastCardText(staffChat.Id, card.msg.MessageID); !strings.Contains(text, staffMarker("lockdown_ban_check_failed")) {
			t.Errorf("card = %q, want %q", text, staffMarker("lockdown_ban_check_failed"))
		}
		if runs := env.banRuns(); len(runs) != 0 {
			t.Errorf("%d ban runs, want none", len(runs))
		}
	})

	t.Run("approved read fails during the run", func(t *testing.T) {
		env, staffChat := banEnv(t)
		first, second := env.newJoiner("R1"), env.newJoiner("R2")
		env.join(first, first, "")
		env.join(second, second, "")
		env.send(env.admin, "/lockdown raid")
		env.cycle()
		a := lastAlert(t, env, staffChat)
		card := env.askBan(a, env.admin)
		pressLockdownButton(env, env.admin, staffChat, card.msg.MessageID, card.confirm)

		restore := failApprovedLookup(t)
		env.cycleUntilIdle()
		for _, user := range []gotgbot.User{first, second} {
			got := len(env.memberCallsOf("getChatMember", user.Id)) + len(env.memberCallsOf("banChatMember", user.Id))
			if got != 0 {
				t.Errorf("%d Telegram calls for %d while the approved users were unreadable, want none", got, user.Id)
			}
		}
		for _, row := range env.burstRows() {
			if row.State != models.BurstStateQueued {
				t.Errorf("burst row of %d = %s, want it still queued", row.UserID, row.State)
			}
		}
		if runs := env.banRuns(); len(runs) != 1 || runs[0].State != models.BanRunStateRunning {
			t.Fatalf("ban runs = %+v, want the run still running", runs)
		}

		restore()
		env.cycleUntilIdle()
		env.wantBannedForGood(first.Id, "R1")
		env.wantBannedForGood(second.Id, "R2")
		if runs := env.banRuns(); len(runs) != 1 || runs[0].State != models.BanRunStateDone {
			t.Errorf("ban runs = %+v, want the run done", runs)
		}
	})

	t.Run("nothing left", func(t *testing.T) {
		env, staffChat := banEnv(t)
		raider := env.newJoiner("R")
		env.join(raider, raider, "")
		env.send(env.admin, "/lockdown raid")
		env.cycle()
		a := lastAlert(t, env, staffChat)
		card := env.askBan(a, env.admin)
		pressLockdownButton(env, env.admin, staffChat, card.msg.MessageID, card.confirm)
		env.cycleUntilIdle()
		env.wantBannedForGood(raider.Id, "the raider")

		sent := len(env.fake.sentTo(staffChat.Id))
		pressLockdownButton(env, env.admin, staffChat, a.alert.MessageID, a.banData)
		if text, _ := env.lastAnswer(); !strings.Contains(text, staffMarker("lockdown_ban_nothing")) {
			t.Errorf("answer = %q, want %q", text, staffMarker("lockdown_ban_nothing"))
		}
		if got := len(env.fake.sentTo(staffChat.Id)); got != sent {
			t.Errorf("%d messages in the Staff Group, were %d: no card may be posted", got, sent)
		}
		if runs := env.banRuns(); len(runs) != 1 {
			t.Errorf("%d ban runs, want only the first", len(runs))
		}
	})

	t.Run("already running", func(t *testing.T) {
		env, staffChat := banEnv(t)
		second := env.newOutsider()
		env.fake.setMember(env.chat.Id, second.Id, staffFakeMember{
			Status:             gotgbot.ChatMemberStatusAdministrator,
			CanRestrictMembers: true,
		})
		raider := env.newJoiner("R")
		env.join(raider, raider, "")
		env.send(env.admin, "/lockdown raid")
		env.cycle()
		a := lastAlert(t, env, staffChat)

		cardA := env.askBan(a, env.admin)
		cardB := env.askBan(a, second)
		pressLockdownButton(env, env.admin, staffChat, cardA.msg.MessageID, cardA.confirm)
		pressLockdownButton(env, second, staffChat, cardB.msg.MessageID, cardB.confirm)

		if text := env.lastCardText(staffChat.Id, cardB.msg.MessageID); !strings.Contains(text, staffMarker("lockdown_ban_busy")) {
			t.Errorf("second admin's card = %q, want %q", text, staffMarker("lockdown_ban_busy"))
		}
		if runs := env.banRuns(); len(runs) != 1 || runs[0].StartedBy != env.admin.Id {
			t.Errorf("ban runs = %+v, want exactly the first admin's run", runs)
		}
		env.cycleUntilIdle()
		env.wantBannedForGood(raider.Id, "the raider")
		if got := len(env.memberCallsOf("banChatMember", raider.Id)); got != 1 {
			t.Errorf("%d banChatMember calls for the raider, want 1", got)
		}
	})
}
