//go:build testtools

package modules

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/aispam"
	"github.com/uasneppy/fuku_robot/fuku/db/approvals"
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

// post runs a message with the given entities in the group as from, through the real dispatcher, and
// returns the message's ID.
func (e *phase18E2E) post(from gotgbot.User, text string, entities ...gotgbot.MessageEntity) int64 {
	e.t.Helper()
	id := e.updateID()
	update := &gotgbot.Update{
		UpdateId: id,
		Message: &gotgbot.Message{
			MessageId: id, Date: 1, Chat: e.chat, From: &from, Text: text, Entities: entities,
		},
	}
	if err := e.dispatcher.ProcessUpdate(e.bot, update, nil); err != nil {
		e.t.Fatalf("ProcessUpdate(%d) error = %v", id, err)
	}
	return id
}

// deleted counts the deleteMessage requests for message msgID of the group.
func (e *phase18E2E) deleted(msgID int64) int {
	count := 0
	for _, call := range callsToChat(e.fake.staffBotClient, "deleteMessage", e.chat.Id) {
		if staffParamInt(call.Params, "message_id") == msgID {
			count++
		}
	}
	return count
}

// repliesWith counts the messages the bot sent to the group that carry the marker of the locale key.
func (e *phase18E2E) repliesWith(key string) int {
	return settingsStaffTestMessagesWith(e.settingsEnv, e.chat.Id, key)
}

// linkEntity marks the first URL of a text that starts with "see ".
func linkEntity(url string) gotgbot.MessageEntity {
	return gotgbot.MessageEntity{Type: "url", Offset: 4, Length: int64(len(url))}
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

	t.Run("locks", func(t *testing.T) {
		e := newPhase18E2E(t, func(e *phase18E2E) { LoadLocks(e.dispatcher) })
		lockGoldenCleanup(t, e.chat.Id)
		settingsLocksWarmBotRight(e.settingsDelEnv)
		member, approved := e.member("Member"), e.member("Approved")
		if err := approvals.AddApprovedUser(e.chat.Id, approved.Id, e.owner.Id, ""); err != nil {
			t.Fatalf("approve: %v", err)
		}
		const link = "http://example.test/x"
		text := "see " + link

		// Nothing is locked yet: a link stays.
		if id := e.post(member, text, linkEntity(link)); e.deleted(id) != 0 {
			t.Fatal("a link was deleted before anything was locked")
		}

		msgID, keyboard := e.openPage(settingsPageLocks)
		keyboard = e.tap(msgID, e.buttonFor(keyboard, "a", "tg", "t", settingsLocksCode(t, "url"), "v", "1"))
		if got := settingsLocksStored(t, e.chat.Id); got != "url=true" {
			t.Fatalf("stored locks = %q, want url=true", got)
		}
		dbcache.ResetLocalForTest()

		if id := e.post(member, text, linkEntity(link)); e.deleted(id) != 1 {
			t.Errorf("a member's link was deleted %d times, want once", e.deleted(id))
		}
		if id := e.post(member, "no link here"); e.deleted(id) != 0 {
			t.Error("a member's plain text was deleted")
		}
		if id := e.post(e.admin, text, linkEntity(link)); e.deleted(id) != 0 {
			t.Error("an administrator's link was deleted")
		}
		if id := e.post(approved, text, linkEntity(link)); e.deleted(id) != 0 {
			t.Error("an approved member's link was deleted")
		}
		e.post(e.admin, "/locks")
		if reply := e.lastReply(); !strings.Contains(reply, "url = true") {
			t.Errorf("/locks answered %q, want url = true as the page shows", reply)
		}

		// Unlocking from the page stops it.
		e.tap(msgID, e.buttonFor(keyboard, "a", "tg", "t", settingsLocksCode(t, "url"), "v", "0"))
		dbcache.ResetLocalForTest()
		if id := e.post(member, text, linkEntity(link)); e.deleted(id) != 0 {
			t.Error("a link was deleted after the page unlocked it")
		}
		e.post(e.admin, "/locks")
		if reply := e.lastReply(); !strings.Contains(reply, "url = false") {
			t.Errorf("/locks answered %q after the unlock, want url = false", reply)
		}
	})

	t.Run("ai spam", func(t *testing.T) {
		withAISpamConfigured(t)
		e := newPhase18E2E(t, func(e *phase18E2E) { LoadAISpam(e.dispatcher) })
		settingsSwCleanup(t, e.chat.Id)
		msgID, keyboard := e.openPage(settingsPageSwitches)
		on := e.buttonFor(keyboard, "a", "tg", "t", settingsSwAIKey, "v", "1")

		// The filter becomes unavailable while the page is open: the command and the stale press refuse
		// the same way, for both causes, and nothing is stored.
		for _, how := range []aispamNotConfiguredHow{aispamProviderValueEmpty, aispamSwitchOff} {
			withAISpamNotConfigured(t, how)
			e.sendIn(e.chat, e.admin, "/aispam on")
			if reply := e.lastReply(); !strings.Contains(reply, staffMarker(aispamRefusalKey())) {
				t.Errorf("/aispam on answered %q, want %s", reply, aispamRefusalKey())
			}
			e.tap(msgID, on)
			if answer, alert := e.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("settings_sw_ai_unavailable")) {
				t.Errorf("the page's On answered %q (alert %v), want the unavailable alert", answer, alert)
			}
			if _, has := settingsSwAIRow(t, e.chat.Id); has {
				t.Fatal("a refused On stored an ai_spam_settings row")
			}
		}

		// Available again, the page's On is what the filter reads.
		withAISpamConfigured(t)
		_, keyboard = e.openPage(settingsPageSwitches)
		e.tap(msgID, e.buttonFor(keyboard, "a", "tg", "t", settingsSwAIKey, "v", "1"))
		dbcache.ResetLocalForTest()
		if !aispam.IsAISpamEnabled(e.chat.Id) {
			t.Error("the filter reads off after the page turned it on")
		}
	})

	t.Run("anonymous admins", func(t *testing.T) {
		e := newPhase18E2E(t, func(e *phase18E2E) { LoadWarns(e.dispatcher) })
		settingsAnonCleanup(t, e.chat.Id)
		// A mode-on anonymous sender skips the cached admin list, so the bot's own record is what the
		// gates read: it can restrict, as it must to warn at all.
		e.setBot(lockdownBotAdminJSON(true, true, false))
		target := e.member("Target")
		warnCmd := fmt.Sprintf("/warn %d spam", target.Id)

		// Mode off: the anonymous admin gets the proof prompt and the command does not run.
		e.sendAnonymous(warnCmd)
		if n := e.repliesWith("chat_status_anon_confirm"); n != 1 {
			t.Fatalf("an anonymous admin got %d proof prompts, want 1", n)
		}
		if got := e.warnCount(target.Id); got != 0 {
			t.Fatalf("the command ran before the proof: %d warns", got)
		}

		// A non-owner administrator's press changes nothing.
		adminMenu := e.openMenu(e.admin)
		_, main := e.lastEdit(e.chat.Id, adminMenu)
		button, ok := settingsPageButton(t, main, settingsPageAnon)
		if !ok {
			t.Fatal("Main has no Anonymous admins button")
		}
		e.pressRaw(e.chat, e.admin, adminMenu, button.CallbackData)
		_, adminPage := e.lastEdit(e.chat.Id, adminMenu)
		toggle, _ := settingsAnonToggleOf(t, adminPage)
		e.pressRaw(e.chat, e.admin, adminMenu, toggle.CallbackData)
		if on, err := settingsAnonRead(context.Background(), e.chat.Id); err != nil || on {
			t.Fatalf("anonymous-admin mode = %v (%v) after a non-owner's press, want off", on, err)
		}
		if answer, _ := e.lastAnswer(); !strings.Contains(answer, staffMarker("chat_status_owner_button_error")) {
			t.Errorf("the non-owner's press was answered %q, want the owner-only alert", answer)
		}

		// The creator turns the mode on from the page and the same command runs without the proof.
		msgID, keyboard := e.openPage(settingsPageAnon)
		toggle, _ = settingsAnonToggleOf(t, keyboard)
		e.tap(msgID, toggle)
		dbcache.ResetLocalForTest()
		if on, err := settingsAnonRead(context.Background(), e.chat.Id); err != nil || !on {
			t.Fatalf("anonymous-admin mode = %v (%v) after the owner's press, want on", on, err)
		}
		e.sendAnonymous(warnCmd)
		if n := e.repliesWith("chat_status_anon_confirm"); n != 1 {
			t.Errorf("with the mode on the bot sent %d proof prompts in all, want still 1", n)
		}
		if got := e.warnCount(target.Id); got != 1 {
			t.Errorf("the anonymous admin's /warn stored %d warns, want 1", got)
		}
	})
}
