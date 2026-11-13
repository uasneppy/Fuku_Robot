//go:build testtools

package modules

import (
	"fmt"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db/approvals"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/warns"
)

// phase19E2E is the toolkit of TestSettingsPhase19EndToEnd: the list settings environment (a supergroup, the bot an
// administrator, the real dispatcher with the Blacklists, Filters, Notes and Warns modules loaded, miniredis) plus a live
// creator of the group who works the private menu. Every press is a real button of the page the bot drew and every typed
// step is a private message through the dispatcher; only Telegram is faked.
type phase19E2E struct {
	*settingsListEnv
	owner gotgbot.User
}

// newPhase19E2E builds one environment for one subtest.
func newPhase19E2E(t *testing.T) *phase19E2E {
	t.Helper()
	env := newSettingsListEnv(t)
	LoadFilters(env.dispatcher)
	LoadNotes(env.dispatcher)
	LoadWarns(env.dispatcher)
	owner := settingsFiltersOwner(env, env.chat)
	memberReqCleanup(t, []int64{env.chat.Id}, []int64{env.admin.Id})
	return &phase19E2E{settingsListEnv: env, owner: owner}
}

// member makes a plain member of the group: a fresh ID in no administrator list, whose rows and cache keys go with the test.
func (e *phase19E2E) member(name string) gotgbot.User {
	e.t.Helper()
	user := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: name}
	e.fake.setProfile(user.Id, name, "", "")
	memberReqCleanup(e.t, nil, []int64{user.Id})
	return user
}

// approvedMember is a member the owner approved through the approvals repository.
func (e *phase19E2E) approvedMember(name string) gotgbot.User {
	e.t.Helper()
	user := e.member(name)
	if err := approvals.AddApprovedUser(e.chat.Id, user.Id, e.owner.Id, ""); err != nil {
		e.t.Fatalf("approve %s: %v", name, err)
	}
	return user
}

// openList opens page of the group in a fresh private menu of the owner.
func (e *phase19E2E) openList(page string) *settingsBlacklistMenu {
	e.t.Helper()
	return settingsListOpenMenuPage(e.settingsListEnv, e.chat, e.owner, page)
}

// tap presses the drawn button whose label carries the marker of the locale key.
func (e *phase19E2E) tap(m *settingsBlacklistMenu, key string) {
	e.t.Helper()
	button, ok := m.markedButton(key)
	if !ok {
		text, keyboard := m.last()
		e.t.Fatalf("the page has no button %s:\n%s\n%+v", key, text, keyboard)
	}
	m.tap(button)
}

// deleteEntry presses the numbered delete button and the card's Yes.
func (e *phase19E2E) deleteEntry(m *settingsBlacklistMenu, label string) {
	e.t.Helper()
	m.tap(m.button(label))
	e.tap(m, "settings_btn_yes")
}

// typeInPrivate sends a private message of the owner through the dispatcher.
func (e *phase19E2E) typeInPrivate(text string) int64 {
	e.t.Helper()
	return settingsListDoorSay(e.settingsListEnv, e.owner, text)
}

// post sends a group message of from through the dispatcher and returns its ID.
func (e *phase19E2E) post(from gotgbot.User, text string) int64 {
	e.t.Helper()
	return sendGroupMessage(e.settingsEnv, e.chat, from, gotgbot.Message{Text: text})
}

// deleted counts the deleteMessage requests of message msgID the bot made in the group.
func (e *phase19E2E) deleted(msgID int64) int {
	return deletedIn(e.settingsEnv, e.chat.Id, msgID)
}

// deletedIn counts the deleteMessage requests of message msgID in chatID on env's bot.
func deletedIn(env *settingsEnv, chatID, msgID int64) int {
	count := 0
	for _, call := range callsToChatFake(env, "deleteMessage", chatID) {
		if staffParamInt(call.Params, "message_id") == msgID {
			count++
		}
	}
	return count
}

// warned is the member's stored warns in the group.
func (e *phase19E2E) warned(user gotgbot.User) int {
	e.t.Helper()
	dbcache.ResetLocalForTest()
	count, _ := warns.GetWarns(user.Id, e.chat.Id)
	return count
}

// sentTo counts the messages the bot sent to the group whose text contains text.
func (e *phase19E2E) sentTo(text string) int {
	count := 0
	for _, sent := range e.fake.sentTo(e.chat.Id) {
		if strings.Contains(fmt.Sprint(sent.Params["text"]), text) {
			count++
		}
	}
	return count
}

// lastSentTo is the newest message to the group that contains text.
func (e *phase19E2E) lastSentTo(text string) (staffSentMessage, bool) {
	sent := e.fake.sentTo(e.chat.Id)
	for i := len(sent) - 1; i >= 0; i-- {
		if strings.Contains(fmt.Sprint(sent[i].Params["text"]), text) {
			return sent[i], true
		}
	}
	return staffSentMessage{}, false
}

// addBlacklistWord works the Blacklist page's Add flow for one word.
func (e *phase19E2E) addBlacklistWord(word string) {
	e.t.Helper()
	m := e.openList(settingsPageBlacklist)
	e.tap(m, "settings_btn_bl_add")
	e.typeInPrivate(word)
}

// wantDeleted fails the test unless the member's message was deleted want times.
func (e *phase19E2E) wantDeleted(msgID int64, want int, what string) {
	e.t.Helper()
	if got := e.deleted(msgID); got != want {
		e.t.Errorf("%s: the message was deleted %d times, want %d", what, got, want)
	}
}

func TestSettingsPhase19EndToEnd(t *testing.T) {
	t.Run("blacklist", func(t *testing.T) {
		e := newPhase19E2E(t)
		member, approved := e.member("Member"), e.approvedMember("Approved")

		// A word added from the page: Add, then typed in private.
		e.addBlacklistWord("spam")
		if got := settingsBlacklistStored(t, e.chat.Id); len(got) != 1 || got["spam"] != "warn" {
			t.Fatalf("stored words = %v, want spam on warn", got)
		}

		// The next member message with the word is deleted and warned once; an admin's and an approved member's stay.
		id := e.post(member, "buy SPAMMER now")
		e.wantDeleted(id, 1, "a member's message with the word")
		if got := e.warned(member); got != 1 {
			t.Errorf("the member has %d warns, want 1", got)
		}
		e.wantDeleted(e.post(e.admin, "spam is bad"), 0, "an admin's message")
		e.wantDeleted(e.post(approved, "spam"), 0, "an approved member's message")
		if got := e.warned(approved) + e.warned(e.admin); got != 0 {
			t.Errorf("an admin or an approved member was warned %d times", got)
		}

		// Delete only on the action screen: the next message is deleted and nobody is warned again.
		m := e.openList(settingsPageBlacklist)
		e.tap(m, "settings_btn_bl_action")
		m.tap(m.button(staffMarker("settings_bl_opt_none")))
		if got := settingsBlacklistStored(t, e.chat.Id); got["spam"] != "none" {
			t.Fatalf("stored words = %v, want spam on none", got)
		}
		id = e.post(member, "more spam")
		e.wantDeleted(id, 1, "a message after Delete only")
		if got := e.warned(member); got != 1 {
			t.Errorf("Delete only added a warn: the member has %d, want 1", got)
		}

		// The page deletes the word: the next message stays.
		e.deleteEntry(e.openList(settingsPageBlacklist), "🗑 1")
		if got := settingsBlacklistStored(t, e.chat.Id); len(got) != 0 {
			t.Fatalf("stored words after the delete = %v, want none", got)
		}
		e.wantDeleted(e.post(member, "spam again"), 0, "a message after the word was deleted")
	})

}
