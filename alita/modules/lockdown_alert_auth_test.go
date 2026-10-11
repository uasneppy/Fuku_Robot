//go:build testtools

package modules

import (
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db/lockdown"
	"github.com/divkix/Alita_Robot/alita/db/models"
)

// lockdownAlertFixture is a confirmed lockdown whose alert sits in a linked Staff Group,
// with the data of the alert's Lift button.
type lockdownAlertFixture struct {
	row       *models.ChatLockdown
	staffChat gotgbot.Chat
	alert     staffSentMessage
	liftData  string
}

// postLockdownAlert locks the env's chat as its admin and lets the worker post the
// alert into a new Staff Group linked to it.
func postLockdownAlert(t *testing.T, env *lockdownEnv) lockdownAlertFixture {
	t.Helper()
	staffChat := linkLockdownStaffGroup(env)
	row := env.lockAsAdmin("raid")
	env.cycle()

	alerts := env.fake.sentTo(staffChat.Id)
	if len(alerts) != 1 {
		t.Fatalf("%d messages reached the Staff Group, want exactly the alert", len(alerts))
	}
	return lockdownAlertFixture{
		row:       row,
		staffChat: staffChat,
		alert:     alerts[0],
		liftData:  lockdownButtonData(t, alerts[0].Params["reply_markup"], "lk"),
	}
}

// pressedLookups counts the getChatMember calls about userID addressed to chatID.
func (e *lockdownEnv) pressedLookups(chatID, userID int64) int {
	count := 0
	for _, call := range callsToChat(e.fake.staffBotClient, "getChatMember", chatID) {
		if staffParamInt(call.Params, "user_id") == userID {
			count++
		}
	}
	return count
}

// lockdownAlertSnapshot is what a refused press must leave exactly as it was.
type lockdownAlertSnapshot struct {
	sets     int
	messages int
	answers  int
}

func (e *lockdownEnv) snapshotAlert(fix lockdownAlertFixture) lockdownAlertSnapshot {
	return lockdownAlertSnapshot{
		sets:     len(e.calls("setChatPermissions")),
		messages: len(e.fake.sentTo(fix.staffChat.Id)),
		answers:  e.answerTotal(),
	}
}

// wantRefused fails unless exactly one answer was given since before, it contains want,
// the lockdown is still active, no permissions were written and no card was posted.
func (e *lockdownEnv) wantRefused(fix lockdownAlertFixture, before lockdownAlertSnapshot, want string) {
	e.t.Helper()
	if got := e.answerTotal() - before.answers; got != 1 {
		e.t.Fatalf("the press made %d answers, want exactly 1", got)
	}
	if text, _ := e.lastAnswer(); !strings.Contains(text, want) {
		e.t.Errorf("answer = %q, want it to contain %q", text, want)
	}
	if got := len(e.calls("setChatPermissions")); got != before.sets {
		e.t.Errorf("setChatPermissions calls = %d, were %d: a refused press writes nothing", got, before.sets)
	}
	if got := len(e.fake.sentTo(fix.staffChat.Id)); got != before.messages {
		e.t.Errorf("%d messages in the Staff Group, were %d: a refused press posts no card", got, before.messages)
	}
	e.wantStillActive(fix.row)
}

func TestLockdownAlertLiftAuthority(t *testing.T) {
	t.Run("staff member, not an admin", func(t *testing.T) {
		env := newLockdownEnv(t)
		fix := postLockdownAlert(t, env)
		member := env.newOutsider()
		before := env.snapshotAlert(fix)

		pressLockdownButton(env, member, fix.staffChat, fix.alert.MessageID, fix.liftData)

		env.wantRefused(fix, before, staffMarker("lockdown_alert_not_admin"))
	})

	t.Run("admin without restrict", func(t *testing.T) {
		env := newLockdownEnv(t)
		fix := postLockdownAlert(t, env)
		weak := env.newOutsider()
		env.fake.setMember(env.chat.Id, weak.Id, staffFakeMember{
			Status:             gotgbot.ChatMemberStatusAdministrator,
			CanRestrictMembers: false,
		})
		before := env.snapshotAlert(fix)

		pressLockdownButton(env, weak, fix.staffChat, fix.alert.MessageID, fix.liftData)

		env.wantRefused(fix, before, staffMarker("lockdown_alert_no_restrict"))
	})

	t.Run("lookup fails", func(t *testing.T) {
		env := newLockdownEnv(t)
		fix := postLockdownAlert(t, env)
		before := env.snapshotAlert(fix)
		env.fake.script("getChatMember", env.chat.Id, staffFakeError(500, "Internal Server Error"))

		pressLockdownButton(env, env.admin, fix.staffChat, fix.alert.MessageID, fix.liftData)

		env.wantRefused(fix, before, staffMarker("lockdown_alert_check_failed"))
	})

	t.Run("service identities", func(t *testing.T) {
		env := newLockdownEnv(t)
		fix := postLockdownAlert(t, env)
		for _, id := range []int64{1087968824, 136817688, 777000} {
			before := env.snapshotAlert(fix)

			pressLockdownButton(env, gotgbot.User{Id: id, FirstName: "Service"}, fix.staffChat, fix.alert.MessageID, fix.liftData)

			env.wantRefused(fix, before, staffMarker("lockdown_alert_post_as_yourself"))
			if got := env.pressedLookups(env.chat.Id, id); got != 0 {
				t.Errorf("service identity %d was looked up %d times, want no lookup", id, got)
			}
		}
	})

	t.Run("forged chat", func(t *testing.T) {
		env := newLockdownEnv(t)
		fix := postLockdownAlert(t, env)
		elsewhere := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Elsewhere"}
		for _, chat := range []gotgbot.Chat{env.chat, elsewhere} {
			before := env.snapshotAlert(fix)
			lookups := env.pressedLookups(env.chat.Id, env.admin.Id)

			pressLockdownButton(env, env.admin, chat, fix.alert.MessageID, fix.liftData)

			env.wantRefused(fix, before, staffMarker("lockdown_alert_expired"))
			if got := env.pressedLookups(env.chat.Id, env.admin.Id); got != lookups {
				t.Errorf("a press on chat %d made %d lookups of the presser, want none", chat.Id, got-lookups)
			}
			if got := len(callsToChat(env.fake.staffBotClient, "getChatMember", chat.Id)); chat.Id == elsewhere.Id && got != 0 {
				t.Errorf("%d lookups in the forged chat, want none", got)
			}
			if got := len(env.fake.sentTo(chat.Id)); chat.Id == elsewhere.Id && got != 0 {
				t.Errorf("%d messages posted in the forged chat, want none", got)
			}
		}
	})

	t.Run("creator allowed", func(t *testing.T) {
		env := newLockdownEnv(t)
		fix := postLockdownAlert(t, env)
		creator := env.newOutsider()
		env.fake.setMember(env.chat.Id, creator.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusCreator})
		before := env.snapshotAlert(fix)

		pressLockdownButton(env, creator, fix.staffChat, fix.alert.MessageID, fix.liftData)

		if got := env.answerTotal() - before.answers; got != 1 {
			t.Fatalf("the press made %d answers, want exactly 1", got)
		}
		posted := env.fake.sentTo(fix.staffChat.Id)
		if len(posted) != before.messages+1 {
			t.Fatalf("%d messages in the Staff Group, want the alert and one confirm card", len(posted))
		}
		confirm := lockdownButtonData(t, posted[len(posted)-1].Params["reply_markup"], "lc")
		decoded, _ := decodeCallbackData(confirm, "lockdown")
		if decoded.Fields["u"] == "" {
			t.Errorf("Confirm data %q carries no asker", confirm)
		}
	})

	t.Run("demoted before Confirm", func(t *testing.T) {
		env := newLockdownEnv(t)
		card := askToLiftLockdown(t, env)
		env.fake.setMember(env.chat.Id, env.admin.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
		sets := len(env.calls("setChatPermissions"))

		pressLockdownButton(env, env.admin, card.staffChat, card.msgID, card.confirm)

		env.wantCardEdited(card, staffMarker("lockdown_alert_not_admin"))
		env.wantStillActive(card.row)
		if got := len(env.calls("setChatPermissions")); got != sets {
			t.Errorf("setChatPermissions calls = %d, were %d: a demoted admin lifts nothing", got, sets)
		}
		if active, err := lockdown.GetActiveFresh(env.chat.Id); err != nil || active == nil {
			t.Errorf("GetActiveFresh = %v, %v, want the lockdown still active", active, err)
		}
	})
}
