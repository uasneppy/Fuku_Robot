//go:build testtools

package modules

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/models"
)

// forGoodBansOf counts the banChatMember calls for userID that carry no end date, the
// bans of "Ban N recent joiners" (the worker's own marker ban always has one).
func (e *lockdownEnv) forGoodBansOf(userID int64) int {
	count := 0
	for _, call := range e.memberCallsOf("banChatMember", userID) {
		if _, hasEnd := call.Params["until_date"]; !hasEnd {
			count++
		}
	}
	return count
}

// banTotal counts every banChatMember call of the env's chat so far.
func (e *lockdownEnv) banTotal() int {
	return len(e.calls("banChatMember"))
}

// confirmBan asks for the ban as from and confirms the card, and returns the card.
func (e *lockdownEnv) confirmBan(a banAlert, from gotgbot.User) banCard {
	e.t.Helper()
	card := e.askBan(a, from)
	pressLockdownButton(e, from, a.staffChat, card.msg.MessageID, card.confirm)
	return card
}

func TestLockdownBanRun(t *testing.T) {
	t.Run("second press", func(t *testing.T) {
		env, staffChat := banEnv(t)
		j1, j2 := env.newJoiner("J1"), env.newJoiner("J2")
		env.join(j1, j1, "")
		env.join(j2, j2, "")
		env.send(env.admin, "/lockdown raid")
		env.cycle()
		a := lastAlert(t, env, staffChat)

		first := env.confirmBan(a, env.admin)
		env.cycleUntilIdle()
		env.wantBannedForGood(j1.Id, "J1")
		env.wantBannedForGood(j2.Id, "J2")
		if runs := env.banRuns(); len(runs) != 1 || runs[0].State != models.BanRunStateDone {
			t.Fatalf("ban runs = %+v, want the first run done", runs)
		}
		if text := env.lastCardText(staffChat.Id, first.msg.MessageID); !strings.Contains(text, " 2 0 0") {
			t.Fatalf("first card = %q, want banned 2, skipped 0, failed 0", text)
		}

		// The raid goes on: J3 joins, and the Ban button is still on the alert.
		j3 := env.newJoiner("J3")
		env.join(j3, j3, "")
		ban1, ban2 := env.forGoodBansOf(j1.Id), env.forGoodBansOf(j2.Id)
		lookups1, lookups2 := len(env.memberCallsOf("getChatMember", j1.Id)), len(env.memberCallsOf("getChatMember", j2.Id))

		card := env.askBan(a, env.admin)
		if text := fmt.Sprint(card.msg.Params["text"]); !strings.Contains(text, staffMarker("lockdown_ban_ask")+" 1 ") {
			t.Fatalf("second card = %q, want %q with 1: only J3 is new", text, staffMarker("lockdown_ban_ask"))
		}
		pressLockdownButton(env, env.admin, staffChat, card.msg.MessageID, card.confirm)
		env.cycleUntilIdle()

		if got := env.forGoodBansOf(j3.Id); got != 1 {
			t.Errorf("%d bans for good of J3, want 1", got)
		}
		env.wantBannedForGood(j3.Id, "J3")
		if got := env.forGoodBansOf(j1.Id); got != ban1 {
			t.Errorf("J1 got %d more bans for good, want none", got-ban1)
		}
		if got := env.forGoodBansOf(j2.Id); got != ban2 {
			t.Errorf("J2 got %d more bans for good, want none", got-ban2)
		}
		if got := len(env.memberCallsOf("getChatMember", j1.Id)); got != lookups1 {
			t.Errorf("J1 was looked up %d more times by the second press, want none", got-lookups1)
		}
		if got := len(env.memberCallsOf("getChatMember", j2.Id)); got != lookups2 {
			t.Errorf("J2 was looked up %d more times by the second press, want none", got-lookups2)
		}
		runs := env.banRuns()
		if len(runs) != 2 || runs[1].State != models.BanRunStateDone {
			t.Fatalf("ban runs = %+v, want two done runs", runs)
		}
		if text := env.lastCardText(staffChat.Id, card.msg.MessageID); !strings.Contains(text, staffMarker("lockdown_ban_result")) || !strings.Contains(text, " 1 0 0") {
			t.Errorf("second card = %q, want the result with banned 1 (J3 alone)", text)
		}
	})

	t.Run("restart", func(t *testing.T) {
		env, staffChat := banEnv(t)
		raider := env.newJoiner("R")
		env.join(raider, raider, "")
		env.send(env.admin, "/lockdown raid")
		env.cycle()
		a := lastAlert(t, env, staffChat)
		card := env.confirmBan(a, env.admin)
		if runs := env.banRuns(); len(runs) != 1 || runs[0].State != models.BanRunStateRunning {
			t.Fatalf("ban runs = %+v, want one running run", runs)
		}

		// The worker that took the account stopped: its row is acting, claimed 3 minutes ago.
		err := db.DB.Model(&models.ChatLockdownBurst{}).
			Where("chat_id = ? AND user_id = ?", env.chat.Id, raider.Id).
			Updates(map[string]any{
				"state":      models.BurstStateActing,
				"claimed_at": time.Now().UTC().Add(-3 * time.Minute),
			}).Error
		if err != nil {
			t.Fatalf("leave the burst row acting: %v", err)
		}

		env.cycleUntilIdle()

		env.wantBannedForGood(raider.Id, "the raider")
		if row := env.burstRowOf(raider.Id); row.State != models.BurstStateBanned {
			t.Errorf("burst row = %q, want banned: the next cycle released the stale claim and finished the account", row.State)
		}
		if runs := env.banRuns(); len(runs) != 1 || runs[0].State != models.BanRunStateDone {
			t.Fatalf("ban runs = %+v, want the run done", runs)
		}
		if text := env.lastCardText(staffChat.Id, card.msg.MessageID); !strings.Contains(text, staffMarker("lockdown_ban_result")) || !strings.Contains(text, " 1 0 0") {
			t.Errorf("card = %q, want the result with banned 1", text)
		}
	})

	t.Run("lifted mid-run", func(t *testing.T) {
		env, staffChat := banEnv(t)
		raiders := []gotgbot.User{env.newJoiner("R1"), env.newJoiner("R2"), env.newJoiner("R3")}
		for _, user := range raiders {
			env.join(user, user, "")
		}
		env.send(env.admin, "/lockdown raid")
		env.cycle()
		a := lastAlert(t, env, staffChat)
		card := env.confirmBan(a, env.admin)
		if runs := env.banRuns(); len(runs) != 1 || runs[0].State != models.BanRunStateRunning {
			t.Fatalf("ban runs = %+v, want one running run", runs)
		}
		forGood := make(map[int64]int)
		for _, user := range raiders {
			forGood[user.Id] = env.forGoodBansOf(user.Id)
		}

		env.send(env.admin, "/unlockdown")
		env.cycleUntilIdle()

		for _, user := range raiders {
			if got := env.forGoodBansOf(user.Id); got != forGood[user.Id] {
				t.Errorf("user %d got %d bans for good after the lift, want none", user.Id, got-forGood[user.Id])
			}
		}
		runs := env.banRuns()
		if len(runs) != 1 || runs[0].State != models.BanRunStateStopped || runs[0].FinishedAt == nil {
			t.Fatalf("ban runs = %+v, want the run stopped with a finish time", runs)
		}
		for _, row := range env.burstRows() {
			if row.State != models.BurstStateNone || row.RunID != nil {
				t.Errorf("burst row of %d = %q with run %v, want unhandled with no run", row.UserID, row.State, row.RunID)
			}
		}
		if text := env.lastCardText(staffChat.Id, card.msg.MessageID); !strings.Contains(text, staffMarker("lockdown_ban_stopped")) {
			t.Errorf("card = %q, want it to say %q", text, staffMarker("lockdown_ban_stopped"))
		}
	})

	t.Run("lift starts in the middle of a batch", func(t *testing.T) {
		env, staffChat := banEnv(t)
		raiders := []gotgbot.User{env.newJoiner("R1"), env.newJoiner("R2"), env.newJoiner("R3")}
		for _, user := range raiders {
			env.join(user, user, "")
		}
		env.send(env.admin, "/lockdown raid")
		env.cycle()
		a := lastAlert(t, env, staffChat)
		card := env.confirmBan(a, env.admin)
		forGood := make(map[int64]int)
		for _, user := range raiders {
			forGood[user.Id] = env.forGoodBansOf(user.Id)
		}

		// The lift is recorded after the worker read the lockdown and the approved users,
		// and before it reaches the first account.
		previous := lockdownApprovedLookup
		lockdownApprovedLookup = func(chatID int64) (map[int64]bool, error) {
			approved, err := previous(chatID)
			err2 := db.DB.Model(&models.ChatLockdown{}).
				Where("chat_id = ? AND state = ?", env.chat.Id, models.LockdownStateActive).
				Update("state", models.LockdownStateLifting).Error
			if err2 != nil {
				t.Errorf("start the lift: %v", err2)
			}
			return approved, err
		}
		t.Cleanup(func() { lockdownApprovedLookup = previous })

		env.cycle()
		lockdownApprovedLookup = previous

		for _, user := range raiders {
			if got := env.forGoodBansOf(user.Id); got != forGood[user.Id] {
				t.Errorf("user %d got %d bans for good after the lift started, want none", user.Id, got-forGood[user.Id])
			}
		}
		if runs := env.banRuns(); len(runs) != 1 || runs[0].State != models.BanRunStateStopped {
			t.Fatalf("ban runs = %+v, want the run stopped", runs)
		}
		if text := env.lastCardText(staffChat.Id, card.msg.MessageID); !strings.Contains(text, staffMarker("lockdown_ban_stopped")) {
			t.Errorf("card = %q, want it to say %q", text, staffMarker("lockdown_ban_stopped"))
		}
	})
}

func TestLockdownBanAuthority(t *testing.T) {
	// raidEnv is a locked chat with one raider in it and the alert posted.
	raidEnv := func(t *testing.T) (*lockdownEnv, gotgbot.Chat, banAlert) {
		t.Helper()
		env, staffChat := banEnv(t)
		raider := env.newJoiner("R")
		env.join(raider, raider, "")
		env.send(env.admin, "/lockdown raid")
		env.cycle()
		return env, staffChat, lastAlert(t, env, staffChat)
	}

	t.Run("admin without restrict", func(t *testing.T) {
		env, staffChat, a := raidEnv(t)
		weak := env.newOutsider()
		env.fake.setMember(env.chat.Id, weak.Id, staffFakeMember{
			Status:             gotgbot.ChatMemberStatusAdministrator,
			CanRestrictMembers: false,
		})
		sent := len(env.fake.sentTo(staffChat.Id))

		pressLockdownButton(env, weak, staffChat, a.alert.MessageID, a.banData)

		if text, _ := env.lastAnswer(); !strings.Contains(text, staffMarker("lockdown_alert_no_restrict")) {
			t.Errorf("answer = %q, want %q", text, staffMarker("lockdown_alert_no_restrict"))
		}
		if got := len(env.fake.sentTo(staffChat.Id)); got != sent {
			t.Errorf("%d messages in the Staff Group, were %d: a refused press posts no card", got, sent)
		}
		if runs := env.banRuns(); len(runs) != 0 {
			t.Errorf("%d ban runs, want none", len(runs))
		}
	})

	t.Run("demoted between Ask and Confirm", func(t *testing.T) {
		env, staffChat, a := raidEnv(t)
		card := env.askBan(a, env.admin)
		env.fake.setMember(env.chat.Id, env.admin.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
		bans := env.banTotal()

		pressLockdownButton(env, env.admin, staffChat, card.msg.MessageID, card.confirm)

		if text := env.lastCardText(staffChat.Id, card.msg.MessageID); !strings.Contains(text, staffMarker("lockdown_alert_not_admin")) {
			t.Errorf("card = %q, want %q", text, staffMarker("lockdown_alert_not_admin"))
		}
		if runs := env.banRuns(); len(runs) != 0 {
			t.Errorf("%d ban runs, want none: a demoted admin bans nobody", len(runs))
		}
		env.cycleUntilIdle()
		if got := env.banTotal(); got != bans {
			t.Errorf("%d bans after the refused Confirm, want none", got-bans)
		}
	})

	t.Run("another admin's Confirm and Cancel", func(t *testing.T) {
		env, staffChat, a := raidEnv(t)
		other := env.newOutsider()
		env.fake.setMember(env.chat.Id, other.Id, staffFakeMember{
			Status:             gotgbot.ChatMemberStatusAdministrator,
			CanRestrictMembers: true,
		})
		card := env.askBan(a, env.admin)

		for name, data := range map[string]string{"Confirm": card.confirm, "Cancel": card.cancel} {
			pressLockdownButton(env, other, staffChat, card.msg.MessageID, data)
			if text, _ := env.lastAnswer(); !strings.Contains(text, staffMarker("lockdown_alert_not_yours")) {
				t.Errorf("%s by another admin answered %q, want %q", name, text, staffMarker("lockdown_alert_not_yours"))
			}
		}
		if edits := env.cardEdits(staffChat.Id, card.msg.MessageID); len(edits) != 0 {
			t.Errorf("card edits = %q, want none: another admin changes nothing", edits)
		}
		if runs := env.banRuns(); len(runs) != 0 {
			t.Errorf("%d ban runs, want none", len(runs))
		}

		// The asker's own Cancel still works.
		pressLockdownButton(env, env.admin, staffChat, card.msg.MessageID, card.cancel)
		if text := env.lastCardText(staffChat.Id, card.msg.MessageID); !strings.Contains(text, staffMarker("lockdown_alert_cancelled")) {
			t.Errorf("card = %q, want %q", text, staffMarker("lockdown_alert_cancelled"))
		}
	})

	t.Run("expired card", func(t *testing.T) {
		env, staffChat, a := raidEnv(t)
		card := env.askBan(a, env.admin)
		decoded, ok := decodeCallbackData(card.confirm, lockdownCallbackNamespace)
		if !ok {
			t.Fatalf("Confirm data %q does not decode", card.confirm)
		}
		stale := encodeCallbackData(lockdownCallbackNamespace, map[string]string{
			"a": lockdownActBanConfirm,
			"r": decoded.Fields["r"],
			"u": fmt.Sprint(env.admin.Id),
			"e": fmt.Sprint(time.Now().Unix()/60 - 1),
		})

		pressLockdownButton(env, env.admin, staffChat, card.msg.MessageID, stale)

		if text := env.lastCardText(staffChat.Id, card.msg.MessageID); !strings.Contains(text, staffMarker("lockdown_alert_card_expired")) {
			t.Errorf("card = %q, want %q", text, staffMarker("lockdown_alert_card_expired"))
		}
		if runs := env.banRuns(); len(runs) != 0 {
			t.Errorf("%d ban runs, want none: an expired card bans nobody", len(runs))
		}
	})

	t.Run("service identities", func(t *testing.T) {
		env, staffChat, a := raidEnv(t)
		card := env.askBan(a, env.admin)
		for _, id := range []int64{777000, 1087968824, 136817688} {
			service := gotgbot.User{Id: id, FirstName: "Service"}
			sent := len(env.fake.sentTo(staffChat.Id))
			for name, target := range map[string]struct {
				msgID int64
				data  string
			}{
				"Ban":     {a.alert.MessageID, a.banData},
				"Confirm": {card.msg.MessageID, card.confirm},
			} {
				pressLockdownButton(env, service, staffChat, target.msgID, target.data)
				if text, _ := env.lastAnswer(); !strings.Contains(text, staffMarker("lockdown_alert_post_as_yourself")) {
					t.Errorf("%s by service identity %d answered %q, want %q", name, id, text, staffMarker("lockdown_alert_post_as_yourself"))
				}
			}
			if got := len(env.fake.sentTo(staffChat.Id)); got != sent {
				t.Errorf("%d messages in the Staff Group, were %d: a service identity gets no card", got, sent)
			}
			if got := env.pressedLookups(env.chat.Id, id); got != 0 {
				t.Errorf("service identity %d was looked up %d times, want none", id, got)
			}
		}
		if runs := env.banRuns(); len(runs) != 0 {
			t.Errorf("%d ban runs, want none", len(runs))
		}
	})
}
