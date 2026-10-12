//go:build testtools

package modules

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db/lockdown"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/utils/cache"
	"github.com/divkix/Alita_Robot/alita/utils/callbackcodec"
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

// askLiftAs has from press the alert's Lift button and returns the confirm card that
// resulted: its message ID and the data of its Confirm button.
func askLiftAs(t *testing.T, env *lockdownEnv, fix lockdownAlertFixture, from gotgbot.User) (msgID int64, confirm string) {
	t.Helper()
	before := len(env.fake.sentTo(fix.staffChat.Id))
	pressLockdownButton(env, from, fix.staffChat, fix.alert.MessageID, fix.liftData)
	posted := env.fake.sentTo(fix.staffChat.Id)
	if len(posted) != before+1 {
		t.Fatalf("%d messages in the Staff Group, want one new confirm card (were %d)", len(posted), before)
	}
	card := posted[len(posted)-1]
	return card.MessageID, lockdownButtonData(t, card.Params["reply_markup"], "lc")
}

// pressOnce presses data on message msgID of the Staff Group as from and fails unless
// the press produced exactly one answerCallbackQuery.
func pressOnce(t *testing.T, env *lockdownEnv, from gotgbot.User, fix lockdownAlertFixture, msgID int64, data string) {
	t.Helper()
	before := env.answerTotal()
	pressLockdownButton(env, from, fix.staffChat, msgID, data)
	if got := env.answerTotal() - before; got != 1 {
		t.Fatalf("the press made %d answers, want exactly 1", got)
	}
}

// copyIDOf is the alert copy row ID the alert's Lift button names.
func copyIDOf(t *testing.T, fix lockdownAlertFixture) string {
	t.Helper()
	decoded, ok := decodeCallbackData(fix.liftData, lockdownCallbackNamespace)
	if !ok {
		t.Fatalf("Lift data %q does not decode", fix.liftData)
	}
	return decoded.Fields["r"]
}

// lastCardEdit is the text of the last edit of message msgID in the Staff Group, or
// the empty string when it was never edited.
func (e *lockdownEnv) lastCardEdit(fix lockdownAlertFixture, msgID int64) string {
	edits := e.cardEdits(fix.staffChat.Id, msgID)
	if len(edits) == 0 {
		return ""
	}
	return edits[len(edits)-1]
}

func TestLockdownAlertConfirmCard(t *testing.T) {
	t.Run("expired", func(t *testing.T) {
		env := newLockdownEnv(t)
		fix := postLockdownAlert(t, env)
		msgID, _ := askLiftAs(t, env, fix, env.admin)
		sets := len(env.calls("setChatPermissions"))
		stale := encodeCallbackData(lockdownCallbackNamespace, map[string]string{
			"a": lockdownActLiftConfirm,
			"r": copyIDOf(t, fix),
			"u": fmt.Sprint(env.admin.Id),
			"e": fmt.Sprint(time.Now().Unix()/60 - 1),
		})

		pressOnce(t, env, env.admin, fix, msgID, stale)

		if got := env.lastCardEdit(fix, msgID); !strings.Contains(got, staffMarker("lockdown_alert_card_expired")) {
			t.Errorf("card reads %q, want %q", got, staffMarker("lockdown_alert_card_expired"))
		}
		if got := len(env.calls("setChatPermissions")); got != sets {
			t.Errorf("setChatPermissions calls = %d, were %d: an expired card lifts nothing", got, sets)
		}
		env.wantStillActive(fix.row)

		// Pressing Lift on the alert again gives a fresh card, and that one lifts.
		freshID, freshConfirm := askLiftAs(t, env, fix, env.admin)
		pressOnce(t, env, env.admin, fix, freshID, freshConfirm)
		if got := len(env.calls("setChatPermissions")); got != sets+1 {
			t.Errorf("setChatPermissions calls = %d, want %d: the fresh card lifts once", got, sets+1)
		}
		if got := env.lastCardEdit(fix, freshID); !strings.Contains(got, staffMarker("lockdown_alert_lift_done")) {
			t.Errorf("fresh card reads %q, want %q", got, staffMarker("lockdown_alert_lift_done"))
		}
	})

	t.Run("double confirm", func(t *testing.T) {
		env := newLockdownEnv(t)
		fix := postLockdownAlert(t, env)
		msgID, confirm := askLiftAs(t, env, fix, env.admin)
		sets := len(env.calls("setChatPermissions"))

		pressOnce(t, env, env.admin, fix, msgID, confirm)
		pressOnce(t, env, env.admin, fix, msgID, confirm)

		if got := len(env.calls("setChatPermissions")); got != sets+1 {
			t.Errorf("setChatPermissions calls = %d, want %d: exactly one lift", got, sets+1)
		}
		if got := env.lastCardEdit(fix, msgID); !strings.Contains(got, staffMarker("lockdown_alert_already_lifted")) {
			t.Errorf("card reads %q after the second Confirm, want %q", got, staffMarker("lockdown_alert_already_lifted"))
		}
	})

	t.Run("two askers", func(t *testing.T) {
		env := newLockdownEnv(t)
		fix := postLockdownAlert(t, env)
		second := env.newOutsider()
		env.fake.setMember(env.chat.Id, second.Id, staffFakeMember{
			Status:             gotgbot.ChatMemberStatusAdministrator,
			CanRestrictMembers: true,
		})
		firstCard, firstConfirm := askLiftAs(t, env, fix, env.admin)
		secondCard, secondConfirm := askLiftAs(t, env, fix, second)
		sets := len(env.calls("setChatPermissions"))

		pressOnce(t, env, env.admin, fix, firstCard, firstConfirm)
		pressOnce(t, env, second, fix, secondCard, secondConfirm)

		if got := len(env.calls("setChatPermissions")); got != sets+1 {
			t.Errorf("setChatPermissions calls = %d, want %d: exactly one lift", got, sets+1)
		}
		if got := env.lastCardEdit(fix, firstCard); !strings.Contains(got, staffMarker("lockdown_alert_lift_done")) {
			t.Errorf("first card reads %q, want %q", got, staffMarker("lockdown_alert_lift_done"))
		}
		if got := env.lastCardEdit(fix, secondCard); !strings.Contains(got, staffMarker("lockdown_alert_already_lifted")) {
			t.Errorf("second card reads %q, want %q", got, staffMarker("lockdown_alert_already_lifted"))
		}
	})

	t.Run("bad fields", func(t *testing.T) {
		env := newLockdownEnv(t)
		fix := postLockdownAlert(t, env)
		msgID, _ := askLiftAs(t, env, fix, env.admin)
		asker := fmt.Sprint(env.admin.Id)
		expiry := fmt.Sprint(time.Now().Unix()/60 + 5)
		copyID := copyIDOf(t, fix)

		cases := []struct {
			name   string
			fields map[string]string
		}{
			{"u missing", map[string]string{"a": "lc", "r": copyID, "e": expiry}},
			{"u not a number", map[string]string{"a": "lc", "r": copyID, "u": "abc", "e": expiry}},
			{"e missing", map[string]string{"a": "lc", "r": copyID, "u": asker}},
			{"e not a number", map[string]string{"a": "lc", "r": copyID, "u": asker, "e": "soon"}},
			{"r zero", map[string]string{"a": "lc", "r": "0", "u": asker, "e": expiry}},
			{"r negative", map[string]string{"a": "lc", "r": "-4", "u": asker, "e": expiry}},
			// Short u keeps the data inside Telegram's 64 bytes next to a 20-digit r.
			{"r 20 digits", map[string]string{"a": "lc", "r": "12345678901234567890", "u": "7", "e": expiry}},
		}
		for _, tc := range cases {
			data := encodeCallbackData(lockdownCallbackNamespace, tc.fields)
			if data == "" {
				t.Fatalf("%s: data did not encode", tc.name)
			}
			sets := len(env.calls("setChatPermissions"))
			lookups := env.pressedLookups(env.chat.Id, env.admin.Id)

			pressOnce(t, env, env.admin, fix, msgID, data)

			if text, _ := env.lastAnswer(); !strings.Contains(text, staffMarker("lockdown_alert_expired")) {
				t.Errorf("%s: answer = %q, want %q", tc.name, text, staffMarker("lockdown_alert_expired"))
			}
			if got := env.pressedLookups(env.chat.Id, env.admin.Id); got != lookups {
				t.Errorf("%s: %d lookups of the presser, want none", tc.name, got-lookups)
			}
			if got := len(env.calls("setChatPermissions")); got != sets {
				t.Errorf("%s: setChatPermissions calls = %d, were %d", tc.name, got, sets)
			}
		}
		env.wantStillActive(fix.row)
	})

	t.Run("no Redis", func(t *testing.T) {
		env := newLockdownEnv(t)
		t.Cleanup(cache.DisableRedisForTest())
		fix := postLockdownAlert(t, env)
		msgID, confirm := askLiftAs(t, env, fix, env.admin)

		pressOnce(t, env, env.admin, fix, msgID, confirm)
		env.cycle()

		if lifted := env.freshRow(fix.row.ID); lifted.State != models.LockdownStateLifting && lifted.State != models.LockdownStateLifted {
			t.Errorf("State = %q, want lifting or lifted without Redis", lifted.State)
		}
		if got := env.lastCardEdit(fix, msgID); !strings.Contains(got, staffMarker("lockdown_alert_lift_done")) {
			t.Errorf("card reads %q, want %q", got, staffMarker("lockdown_alert_lift_done"))
		}
		rows := env.alertRows()
		if len(rows) != 1 || rows[0].ClosedAt == nil || rows[0].ButtonsLive {
			t.Errorf("alert rows = %+v, want one closed copy with no live buttons", rows)
		}
	})
}

func TestLockdownCallbackData(t *testing.T) {
	minute := fmt.Sprint(time.Now().Unix() / 60)
	for name, fields := range map[string]map[string]string{
		"ask":     {"a": lockdownActLiftAsk, "r": "9999999999"},
		"confirm": {"a": lockdownActLiftConfirm, "r": "9999999999", "u": "9999999999999999", "e": minute},
		"cancel":  {"a": lockdownActCancel, "r": "9999999999", "u": "9999999999999999"},
	} {
		data := encodeCallbackData(lockdownCallbackNamespace, fields)
		if data == "" || len(data) > callbackcodec.MaxCallbackDataLen {
			t.Errorf("%s: data %q has %d bytes, want 1 to %d", name, data, len(data), callbackcodec.MaxCallbackDataLen)
			continue
		}
		decoded, ok := decodeCallbackData(data, lockdownCallbackNamespace)
		if !ok {
			t.Errorf("%s: data %q does not decode", name, data)
			continue
		}
		for key, want := range fields {
			if got := decoded.Fields[key]; got != want {
				t.Errorf("%s: field %s = %q, want %q", name, key, got, want)
			}
		}
	}

	t.Run("unknown action", func(t *testing.T) {
		env := newLockdownEnv(t)
		fix := postLockdownAlert(t, env)
		data := encodeCallbackData(lockdownCallbackNamespace, map[string]string{"a": "zz", "r": copyIDOf(t, fix)})

		pressOnce(t, env, env.admin, fix, fix.alert.MessageID, data)

		if text, _ := env.lastAnswer(); !strings.Contains(text, staffMarker("lockdown_alert_expired")) {
			t.Errorf("answer = %q, want %q", text, staffMarker("lockdown_alert_expired"))
		}
		env.wantStillActive(fix.row)
	})

	t.Run("undecodable", func(t *testing.T) {
		env := newLockdownEnv(t)
		fix := postLockdownAlert(t, env)

		pressOnce(t, env, env.admin, fix, fix.alert.MessageID, "lockdown|v9|a=lk")

		if text, _ := env.lastAnswer(); !strings.Contains(text, staffMarker("common_callback_invalid_request")) {
			t.Errorf("answer = %q, want %q", text, staffMarker("common_callback_invalid_request"))
		}
		env.wantStillActive(fix.row)
	})
}
