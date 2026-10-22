//go:build testtools

package modules

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/logchannels"
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

// logBanPosts is the text of every #LOCKDOWN_BAN message the log channel received.
func (e *lockdownEnv) logBanPosts(channelID int64) []string {
	var posts []string
	for _, sent := range e.fake.sentTo(channelID) {
		if text := fmt.Sprint(sent.Params["text"]); strings.Contains(text, "#LOCKDOWN_BAN") {
			posts = append(posts, text)
		}
	}
	return posts
}

// cyclesUntilRunEnds runs worker cycles, at most limit of them, until no run is running.
// A failing account costs one attempt per cycle, so a run with one can need several.
func (e *lockdownEnv) cyclesUntilRunEnds(limit int) {
	e.t.Helper()
	for i := 0; i < limit; i++ {
		e.cycle()
		running := false
		for _, run := range e.banRuns() {
			if run.State == models.BanRunStateRunning {
				running = true
			}
		}
		if !running {
			return
		}
	}
	e.t.Fatalf("a ban run was still running after %d cycles", limit)
}

func TestLockdownBanResult(t *testing.T) {
	t.Run("full report", func(t *testing.T) {
		env, staffChat := banEnv(t)

		// Joined before the lock: a member, an admin and an account someone else kicked.
		p1, p2, p3 := env.newJoiner("P1"), env.newJoiner("P2"), env.newJoiner("P3")
		env.join(p1, p1, "")
		env.join(p2, p2, "")
		env.join(p3, p3, "")
		env.fake.setMember(env.chat.Id, p2.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusAdministrator})
		env.fake.setMember(env.chat.Id, p3.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusKicked})

		env.send(env.admin, "/lockdown raid")
		time.Sleep(3 * time.Millisecond)
		// Ascending IDs in join order, so the run reaches them in the order they joined
		// whether the database scans the joiner rows by row or by user.
		base := uniqueLinkOwnerID()
		j1 := gotgbot.User{Id: base + 1, FirstName: "J1"}
		e := gotgbot.User{Id: base + 2, FirstName: "E"}
		r := gotgbot.User{Id: base + 3, FirstName: "R"}
		f := gotgbot.User{Id: base + 4, FirstName: "F<b>"}
		env.join(j1, j1, "")
		env.join(e, env.admin, "")
		env.joinRequest(r, "")
		env.join(f, f, "")
		env.cycle()
		a := lastAlert(t, env, staffChat)

		// The run reaches P1, J1, R and F in that order (burst row order); F's three
		// attempts are the only calls Telegram refuses.
		refusal := staffFakeError(400, "Bad Request: user not found")
		env.fake.script("banChatMember", env.chat.Id, nil, nil, nil, refusal, refusal, refusal)
		card := env.confirmBan(a, env.admin)
		env.cyclesUntilRunEnds(6)

		text := env.lastCardText(staffChat.Id, card.msg.MessageID)
		for _, want := range []string{
			staffMarker("lockdown_ban_result") + " ",
			" 3 3 1",
			staffMarker("lockdown_ban_result_requests") + " 1",
			staffMarker("lockdown_ban_skip_admin") + " 1",
			staffMarker("lockdown_ban_skip_banned") + " 1",
			staffMarker("lockdown_ban_skip_let_in") + " 1",
			staffMarker("lockdown_ban_failed_line") + " 1",
			"F&lt;b&gt;",
			fmt.Sprintf("<code>%d</code>", f.Id),
			"Bad Request: user not found",
		} {
			if !strings.Contains(text, want) {
				t.Errorf("result card = %q, want it to contain %q", text, want)
			}
		}
		for _, unwanted := range []string{
			staffMarker("lockdown_ban_skip_approved"),
			staffMarker("lockdown_ban_stopped"),
			"<<lockdown",
		} {
			if strings.Contains(text, unwanted) {
				t.Errorf("result card = %q, want it without %q", text, unwanted)
			}
		}
		env.wantBannedForGood(p1.Id, "P1")
		env.wantBannedForGood(j1.Id, "J1")
		env.wantBannedForGood(r.Id, "the declined requester")
		env.wantNotBanned(p2.Id, "the admin")
		env.wantNotBanned(e.Id, "the user an admin let in")
	})

	t.Run("failed list is cut", func(t *testing.T) {
		env, staffChat := banEnv(t)
		var failing []gotgbot.User
		for i := 0; i < 12; i++ {
			user := env.newJoiner(fmt.Sprintf("Failing%d", i))
			env.join(user, user, "")
			failing = append(failing, user)
		}
		env.send(env.admin, "/lockdown raid")
		env.cycle()
		a := lastAlert(t, env, staffChat)
		refusal := staffFakeError(400, "Bad Request: no")
		for i := 0; i < 12*3; i++ {
			env.fake.script("banChatMember", env.chat.Id, refusal)
		}
		card := env.confirmBan(a, env.admin)
		env.cyclesUntilRunEnds(6)

		text := env.lastCardText(staffChat.Id, card.msg.MessageID)
		if !strings.Contains(text, staffMarker("lockdown_ban_failed_line")+" 12") {
			t.Fatalf("result card = %q, want %q with 12", text, staffMarker("lockdown_ban_failed_line"))
		}
		listed := 0
		for _, user := range failing {
			if strings.Contains(text, fmt.Sprintf("<code>%d</code>", user.Id)) {
				listed++
			}
		}
		if listed != 10 {
			t.Errorf("%d failed accounts listed, want 10", listed)
		}
		if !strings.Contains(text, staffMarker("lockdown_lift_tally_more")+" 2") {
			t.Errorf("result card = %q, want %q with the 2 left out", text, staffMarker("lockdown_lift_tally_more"))
		}
	})

	t.Run("log post", func(t *testing.T) {
		env, staffChat := banEnv(t)
		logChannel := uniqueModuleChatID()
		setStaffLogChannel(t, env.chat.Id, logChannel)

		j1 := env.newJoiner("J1")
		env.join(j1, j1, "")
		env.send(env.admin, "/lockdown raid")
		env.cycle()
		a := lastAlert(t, env, staffChat)

		env.confirmBan(a, env.admin)
		env.cycleUntilIdle()
		posts := env.logBanPosts(logChannel)
		if len(posts) != 1 {
			t.Fatalf("%d #LOCKDOWN_BAN posts after the first run, want exactly 1", len(posts))
		}
		for _, want := range []string{
			staffMarker("lockdown_ban_log") + " 1 0 0",
			"Ad&lt;b&gt;min",
			fmt.Sprintf("tg://user?id=%d", env.admin.Id),
			staffMarker("staff_log_admin_label"),
		} {
			if !strings.Contains(posts[0], want) {
				t.Errorf("log post = %q, want it to contain %q", posts[0], want)
			}
		}

		// More cycles post nothing more; a second run posts its own.
		env.cycle()
		env.cycle()
		if got := len(env.logBanPosts(logChannel)); got != 1 {
			t.Fatalf("%d #LOCKDOWN_BAN posts after extra cycles, want still 1", got)
		}
		j2 := env.newJoiner("J2")
		env.join(j2, j2, "")
		env.confirmBan(a, env.admin)
		env.cycleUntilIdle()
		if got := len(env.logBanPosts(logChannel)); got != 2 {
			t.Errorf("%d #LOCKDOWN_BAN posts after the second run, want 2", got)
		}
	})

	t.Run("no audit", func(t *testing.T) {
		env, staffChat := banEnv(t)
		setStaffLogChannel(t, env.chat.Id, uniqueModuleChatID())
		raider := env.newJoiner("R")
		env.join(raider, raider, "")
		env.send(env.admin, "/lockdown raid")
		env.cycle()
		a := lastAlert(t, env, staffChat)

		var before, after int64
		if err := db.DB.Model(&models.StaffAction{}).Count(&before).Error; err != nil {
			t.Fatalf("count staff actions: %v", err)
		}
		env.confirmBan(a, env.admin)
		env.cycleUntilIdle()
		if err := db.DB.Model(&models.StaffAction{}).Count(&after).Error; err != nil {
			t.Fatalf("count staff actions: %v", err)
		}
		if after != before {
			t.Errorf("%d staff_actions rows after the run, were %d: a response ban writes no audit record", after, before)
		}
		env.wantBannedForGood(raider.Id, "the raider")
	})

	t.Run("category off", func(t *testing.T) {
		env, staffChat := banEnv(t)
		logChannel := uniqueModuleChatID()
		setStaffLogChannel(t, env.chat.Id, logChannel)
		if err := logchannels.SetCategory(env.chat.Id, logchannels.CategoryAdmin, false); err != nil {
			t.Fatalf("turn the admin category off: %v", err)
		}
		raider := env.newJoiner("R")
		env.join(raider, raider, "")
		env.send(env.admin, "/lockdown raid")
		env.cycle()
		a := lastAlert(t, env, staffChat)

		card := env.confirmBan(a, env.admin)
		env.cycleUntilIdle()

		if posts := env.logBanPosts(logChannel); len(posts) != 0 {
			t.Errorf("%d #LOCKDOWN_BAN posts with the admin category off, want none", len(posts))
		}
		if runs := env.banRuns(); len(runs) != 1 || runs[0].State != models.BanRunStateDone {
			t.Errorf("ban runs = %+v, want the run done", runs)
		}
		if text := env.lastCardText(staffChat.Id, card.msg.MessageID); !strings.Contains(text, staffMarker("lockdown_ban_result")) {
			t.Errorf("card = %q, want the result", text)
		}
	})

	t.Run("a stopped run that handled nothing posts nothing", func(t *testing.T) {
		env, staffChat := banEnv(t)
		logChannel := uniqueModuleChatID()
		setStaffLogChannel(t, env.chat.Id, logChannel)
		raider := env.newJoiner("R")
		env.join(raider, raider, "")
		env.send(env.admin, "/lockdown raid")
		env.cycle()
		a := lastAlert(t, env, staffChat)
		env.confirmBan(a, env.admin)

		env.send(env.admin, "/unlockdown")
		env.cycleUntilIdle()

		if runs := env.banRuns(); len(runs) != 1 || runs[0].State != models.BanRunStateStopped {
			t.Fatalf("ban runs = %+v, want the run stopped", runs)
		}
		if posts := env.logBanPosts(logChannel); len(posts) != 0 {
			t.Errorf("%d #LOCKDOWN_BAN posts for a run that handled nobody, want none", len(posts))
		}
	})
}

// finishedRun reads the one ban run of the env's chat.
func (e *lockdownEnv) finishedRun() models.ChatLockdownBanRun {
	e.t.Helper()
	runs := e.banRuns()
	if len(runs) != 1 {
		e.t.Fatalf("ban runs = %+v, want exactly one", runs)
	}
	return runs[0]
}

// cyclesUntilReported runs worker cycles, at most limit of them, until the env's one ban
// run has its report settled. A delivery that fails costs an attempt per cycle and makes
// no progress, so waiting for an idle cycle would stop too early.
func (e *lockdownEnv) cyclesUntilReported(limit int) {
	e.t.Helper()
	for i := 0; i < limit; i++ {
		if e.finishedRun().ReportedAt != nil {
			return
		}
		e.cycle()
	}
	if e.finishedRun().ReportedAt == nil {
		e.t.Fatalf("the report of the ban run was still open after %d cycles", limit)
	}
}

func TestLockdownBanReportResumes(t *testing.T) {
	// finishedUnreported runs a ban run whose card edit is rate limited, so the run ends
	// with its report undelivered, and returns the env, the card and the log channel.
	finishedUnreported := func(t *testing.T) (*lockdownEnv, gotgbot.Chat, banCard, int64) {
		t.Helper()
		env, staffChat := banEnv(t)
		logChannel := uniqueModuleChatID()
		setStaffLogChannel(t, env.chat.Id, logChannel)
		raider := env.newJoiner("R")
		env.join(raider, raider, "")
		env.send(env.admin, "/lockdown raid")
		env.cycle()
		a := lastAlert(t, env, staffChat)
		card := env.confirmBan(a, env.admin)

		limited := staffFake429(1)
		env.fake.script("editMessageText", staffChat.Id, limited, limited, limited, limited)
		env.cycle()
		return env, staffChat, card, logChannel
	}

	t.Run("a rate limited result is delivered by a later cycle", func(t *testing.T) {
		env, staffChat, card, logChannel := finishedUnreported(t)

		run := env.finishedRun()
		if run.State != models.BanRunStateDone || run.ReportedAt != nil || run.ReportClaimedAt != nil || run.ReportAttempts != 0 {
			t.Fatalf("run = %+v, want it done, unreported, unclaimed, with no attempt counted for a rate limit", run)
		}
		if run.ReportCardDoneAt != nil || run.ReportLogDoneAt != nil {
			t.Fatalf("run = %+v, want no step settled while the card edit is rate limited", run)
		}

		env.cyclesUntilReported(6)

		run = env.finishedRun()
		if run.ReportedAt == nil || run.ReportClaimedAt != nil {
			t.Fatalf("run = %+v, want it reported with the claim cleared", run)
		}
		if text := env.lastCardText(staffChat.Id, card.msg.MessageID); !strings.Contains(text, staffMarker("lockdown_ban_result")) || !strings.Contains(text, " 1 0 0") {
			t.Errorf("card = %q, want the result with banned 1", text)
		}
		if got := len(env.logBanPosts(logChannel)); got != 1 {
			t.Errorf("%d #LOCKDOWN_BAN posts, want exactly 1", got)
		}
		env.cycle()
		env.cycle()
		if got := len(env.logBanPosts(logChannel)); got != 1 {
			t.Errorf("%d #LOCKDOWN_BAN posts after more cycles, want still 1", got)
		}
	})

	t.Run("a worker that stopped mid-delivery is replaced", func(t *testing.T) {
		env, staffChat, card, logChannel := finishedUnreported(t)
		run := env.finishedRun()
		err := db.DB.Model(&models.ChatLockdownBanRun{}).Where("id = ?", run.ID).
			Update("report_claimed_at", time.Now().UTC().Add(-3*time.Minute)).Error
		if err != nil {
			t.Fatalf("leave the report claimed: %v", err)
		}

		env.cyclesUntilReported(6)

		if run = env.finishedRun(); run.ReportedAt == nil {
			t.Fatalf("run = %+v, want the stale claim released and the report delivered", run)
		}
		if text := env.lastCardText(staffChat.Id, card.msg.MessageID); !strings.Contains(text, staffMarker("lockdown_ban_result")) {
			t.Errorf("card = %q, want the result", text)
		}
		if got := len(env.logBanPosts(logChannel)); got != 1 {
			t.Errorf("%d #LOCKDOWN_BAN posts, want exactly 1", got)
		}
	})

	t.Run("a post that fails is retried and the delivered card edit is not repeated", func(t *testing.T) {
		env, staffChat := banEnv(t)
		logChannel := uniqueModuleChatID()
		setStaffLogChannel(t, env.chat.Id, logChannel)
		raider := env.newJoiner("R")
		env.join(raider, raider, "")
		env.send(env.admin, "/lockdown raid")
		env.cycle()
		a := lastAlert(t, env, staffChat)
		card := env.confirmBan(a, env.admin)
		refusal := staffFakeError(400, "Bad Request: chat not found")
		env.fake.script("sendMessage", logChannel, refusal, refusal)
		edits := len(env.cardEdits(staffChat.Id, card.msg.MessageID))

		env.cycle()
		run := env.finishedRun()
		if run.ReportedAt != nil || run.ReportCardDoneAt == nil || run.ReportLogDoneAt != nil || run.ReportAttempts != 1 {
			t.Fatalf("run = %+v, want the card delivered, the post failed once and the report open", run)
		}

		env.cyclesUntilReported(6)

		if run = env.finishedRun(); run.ReportedAt == nil {
			t.Fatalf("run = %+v, want the report delivered", run)
		}
		if got := len(env.logBanPosts(logChannel)); got != 1 {
			t.Errorf("%d #LOCKDOWN_BAN posts, want exactly 1", got)
		}
		if got := len(env.cardEdits(staffChat.Id, card.msg.MessageID)) - edits; got != 1 {
			t.Errorf("%d edits of the card for the result, want exactly 1: a delivered step is not repeated", got)
		}
	})

	t.Run("a post that keeps failing is given up after three attempts", func(t *testing.T) {
		env, staffChat := banEnv(t)
		logChannel := uniqueModuleChatID()
		setStaffLogChannel(t, env.chat.Id, logChannel)
		raider := env.newJoiner("R")
		env.join(raider, raider, "")
		env.send(env.admin, "/lockdown raid")
		env.cycle()
		a := lastAlert(t, env, staffChat)
		env.confirmBan(a, env.admin)
		refusal := staffFakeError(400, "Bad Request: chat not found")
		env.fake.script("sendMessage", logChannel, refusal, refusal, refusal)

		env.cyclesUntilReported(6)

		run := env.finishedRun()
		if run.ReportedAt == nil || run.ReportClaimedAt != nil {
			t.Fatalf("run = %+v, want the report settled after three failed posts", run)
		}
		if got := len(env.logBanPosts(logChannel)); got != 0 {
			t.Errorf("%d #LOCKDOWN_BAN posts, want none: all three attempts failed", got)
		}
		before := len(env.fake.sentTo(logChannel))
		env.cycle()
		env.cycle()
		if got := len(env.fake.sentTo(logChannel)); got != before {
			t.Errorf("%d sends to the log channel after the give-up, were %d: a settled report is not tried again", got, before)
		}
	})

	t.Run("a stopped run reports too", func(t *testing.T) {
		env, staffChat := banEnv(t)
		raider := env.newJoiner("R")
		env.join(raider, raider, "")
		env.send(env.admin, "/lockdown raid")
		env.cycle()
		a := lastAlert(t, env, staffChat)
		card := env.confirmBan(a, env.admin)
		limited := staffFake429(1)
		env.fake.script("editMessageText", staffChat.Id, limited, limited, limited, limited)

		env.send(env.admin, "/unlockdown")
		env.cycle()
		if run := env.finishedRun(); run.State != models.BanRunStateStopped || run.ReportedAt != nil {
			t.Fatalf("run = %+v, want it stopped with its report undelivered", run)
		}

		env.cyclesUntilReported(6)

		if run := env.finishedRun(); run.ReportedAt == nil {
			t.Fatalf("run = %+v, want the stopped report delivered", run)
		}
		if text := env.lastCardText(staffChat.Id, card.msg.MessageID); !strings.Contains(text, staffMarker("lockdown_ban_stopped")) {
			t.Errorf("card = %q, want it to say %q", text, staffMarker("lockdown_ban_stopped"))
		}
	})
}
