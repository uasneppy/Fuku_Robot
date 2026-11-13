//go:build testtools

package modules

import (
	"fmt"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/db/warns"
)

// phase18E2E is the toolkit of TestSettingsPhase18EndToEnd: the Deletion settings environment (a
// supergroup, a creator who owns it, an administrator who can restrict members, a bot that is an
// administrator with the delete right, the real dispatcher, miniredis) with the module a subtest
// exercises loaded on that dispatcher, and helpers that open a page as the owner, press its real
// buttons and read what the fake Telegram saw. Only Telegram is faked.
type phase18E2E struct {
	*settingsDelEnv
}

// newPhase18E2E builds one environment for one subtest. load puts the modules the subtest needs on
// the dispatcher; the settings module is already there.
func newPhase18E2E(t *testing.T, load ...func(*phase18E2E)) *phase18E2E {
	t.Helper()
	e := &phase18E2E{settingsDelEnv: newSettingsDelEnv(t)}
	memberReqCleanup(t, []int64{e.chat.Id}, []int64{e.owner.Id, e.admin.Id})
	for _, fn := range load {
		fn(e)
	}
	return e
}

// member makes a plain member of the group: a fresh ID, in no administrator list, whose rows and cache
// keys are removed when the test ends.
func (e *phase18E2E) member(name string) gotgbot.User {
	e.t.Helper()
	user := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: name}
	e.fake.setProfile(user.Id, name, "", "")
	memberReqCleanup(e.t, nil, []int64{user.Id})
	return user
}

// openPage opens the group menu as the owner, presses the Main button of the page and returns the
// menu message's ID and the page's keyboard.
func (e *phase18E2E) openPage(page string) (int64, [][]gotgbot.InlineKeyboardButton) {
	e.t.Helper()
	msgID := e.openMenu(e.owner)
	_, main := e.lastEdit(e.chat.Id, msgID)
	button, ok := settingsPageButton(e.t, main, page)
	if !ok {
		e.t.Fatalf("Main has no button for page %q", page)
	}
	e.pressRaw(e.chat, e.owner, msgID, button.CallbackData)
	_, keyboard := e.lastEdit(e.chat.Id, msgID)
	return msgID, keyboard
}

// buttonFor returns the button of keyboard whose settings fields carry all of kv, and fails the test when
// the page draws none.
func (e *phase18E2E) buttonFor(keyboard [][]gotgbot.InlineKeyboardButton, kv ...string) gotgbot.InlineKeyboardButton {
	e.t.Helper()
	if len(kv)%2 != 0 {
		e.t.Fatalf("buttonFor takes key, value pairs, got %v", kv)
	}
	for _, button := range settingsAllButtons(keyboard) {
		decoded, ok := decodeCallbackData(button.CallbackData, settingsCallbackNamespace)
		if !ok {
			continue
		}
		matches := true
		for i := 0; i+1 < len(kv); i += 2 {
			if decoded.Fields[kv[i]] != kv[i+1] {
				matches = false
			}
		}
		if matches {
			return button
		}
	}
	e.t.Fatalf("the page has no button with fields %v: %+v", kv, keyboard)
	return gotgbot.InlineKeyboardButton{}
}

// tap presses the button of the page message msgID as the owner and returns the page's keyboard after
// the press.
func (e *phase18E2E) tap(msgID int64, button gotgbot.InlineKeyboardButton) [][]gotgbot.InlineKeyboardButton {
	e.t.Helper()
	e.pressRaw(e.chat, e.owner, msgID, button.CallbackData)
	_, keyboard := e.lastEdit(e.chat.Id, msgID)
	return keyboard
}

// callsAbout counts the requests of method addressed to the group that name userID.
func (e *phase18E2E) callsAbout(method string, userID int64) int {
	count := 0
	for _, call := range callsToChat(e.fake.staffBotClient, method, e.chat.Id) {
		if staffParamInt(call.Params, "user_id") == userID {
			count++
		}
	}
	return count
}

// warnCount is the member's stored warns in the group.
func (e *phase18E2E) warnCount(userID int64) int {
	e.t.Helper()
	dbcache.ResetLocalForTest()
	count, _ := warns.GetWarns(userID, e.chat.Id)
	return count
}

func TestSettingsPhase18EndToEnd(t *testing.T) {
	t.Run("warnings", func(t *testing.T) {
		e := newPhase18E2E(t, func(e *phase18E2E) { LoadWarns(e.dispatcher) })
		msgID, keyboard := e.openPage(settingsPageWarns)

		// The owner presses the stepper's -1 button (3 to 2) and the Ban choice.
		keyboard = e.tap(msgID, e.buttonFor(keyboard, "t", settingsWarnsLimitKey, "v", "2"))
		keyboard = e.tap(msgID, e.buttonFor(keyboard, "t", settingsWarnsModeKey, "v", "ban"))
		if row, ok := settingsWarnsRow(t, e.chat.Id); !ok || row.WarnLimit != 2 || row.WarnMode != "ban" {
			t.Fatalf("stored warn settings = %+v (row %v), want limit 2 and ban", row, ok)
		}

		warnTwice := func(member gotgbot.User) {
			t.Helper()
			e.sendIn(e.chat, e.admin, fmt.Sprintf("/warn %d spam", member.Id))
			if got := e.warnCount(member.Id); got != 1 {
				t.Fatalf("after one /warn the member has %d warns, want 1", got)
			}
			if bans := e.callsAbout("banChatMember", member.Id) + e.callsAbout("restrictChatMember", member.Id) +
				e.callsAbout("unbanChatMember", member.Id); bans != 0 {
				t.Fatalf("one warn below the limit made %d limit action calls, want none", bans)
			}
			e.sendIn(e.chat, e.admin, fmt.Sprintf("/warn %d spam", member.Id))
		}

		banned := e.member("Banned")
		warnTwice(banned)
		if n := e.callsAbout("banChatMember", banned.Id); n != 1 {
			t.Errorf("the second warn made %d banChatMember calls, want 1", n)
		}
		if n := e.callsAbout("restrictChatMember", banned.Id) + e.callsAbout("unbanChatMember", banned.Id); n != 0 {
			t.Errorf("a Ban limit also made %d restrict or unban calls", n)
		}
		if got := e.warnCount(banned.Id); got != 0 {
			t.Errorf("the banned member's warns = %d, want them cleared", got)
		}

		// Mute: the page replaces the action, and the next member who reaches the limit is restricted.
		e.tap(msgID, e.buttonFor(keyboard, "t", settingsWarnsModeKey, "v", "mute"))
		muted := e.member("Muted")
		warnTwice(muted)
		restricts := callsToChat(e.fake.staffBotClient, "restrictChatMember", e.chat.Id)
		if n := e.callsAbout("restrictChatMember", muted.Id); n != 1 {
			t.Fatalf("a Mute limit made %d restrictChatMember calls, want 1 (all %d)", n, len(restricts))
		}
		if n := e.callsAbout("banChatMember", muted.Id) + e.callsAbout("unbanChatMember", muted.Id); n != 0 {
			t.Errorf("a Mute limit also made %d ban or unban calls", n)
		}
		if rec := e.fake.member(e.chat.Id, muted.Id); rec == nil || rec.Perms == nil || rec.Perms.CanSendMessages {
			t.Errorf("the muted member's stored restriction = %+v, want one with no rights", rec)
		}
		if got := e.warnCount(muted.Id); got != 0 {
			t.Errorf("the muted member's warns = %d, want them cleared", got)
		}

		// Kick: one member-removing unban.
		e.tap(msgID, e.buttonFor(keyboard, "t", settingsWarnsModeKey, "v", "kick"))
		kicked := e.member("Kicked")
		warnTwice(kicked)
		unbans := callsToChat(e.fake.staffBotClient, "unbanChatMember", e.chat.Id)
		var removals int
		for _, call := range unbans {
			if staffParamInt(call.Params, "user_id") == kicked.Id && !staffParamBool(call.Params, "only_if_banned") {
				removals++
			}
		}
		if removals != 1 {
			t.Errorf("a Kick limit made %d member-removing unbanChatMember calls, want 1 (%v)", removals, unbans)
		}
		if n := e.callsAbout("banChatMember", kicked.Id) + e.callsAbout("restrictChatMember", kicked.Id); n != 0 {
			t.Errorf("a Kick limit also made %d ban or restrict calls", n)
		}
		if got := e.warnCount(kicked.Id); got != 0 {
			t.Errorf("the kicked member's warns = %d, want them cleared", got)
		}

		// A stored tban, which the table allows and the page never writes, applies nothing at the limit.
		if err := db.DB.Model(&models.WarnSettings{}).Where("chat_id = ?", e.chat.Id).
			Update("warn_mode", "tban").Error; err != nil {
			t.Fatalf("write tban by hand: %v", err)
		}
		dbcache.DeleteCache(dbcache.CacheKey("warn_settings", e.chat.Id))
		dbcache.ResetLocalForTest()
		inert := e.member("Inert")
		e.sendIn(e.chat, e.admin, fmt.Sprintf("/warn %d spam", inert.Id))
		e.sendIn(e.chat, e.admin, fmt.Sprintf("/warn %d spam", inert.Id))
		if n := e.callsAbout("banChatMember", inert.Id) + e.callsAbout("restrictChatMember", inert.Id) +
			e.callsAbout("unbanChatMember", inert.Id); n != 0 {
			t.Errorf("a stored tban made %d limit action calls, want none", n)
		}
		if got := e.warnCount(inert.Id); got != 2 {
			t.Errorf("the member's warns under tban = %d, want 2 (not cleared)", got)
		}
	})
}
