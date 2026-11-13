//go:build testtools

package modules

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db/blacklists"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
)

// settingsListDoorWait opens the Blacklist page of chat in a private menu of user and presses Add, so user has a wait
// for the words part. It returns the menu, whose message is now the waiting prompt.
func settingsListDoorWait(env *settingsListEnv, chat gotgbot.Chat, user gotgbot.User) *settingsBlacklistMenu {
	env.t.Helper()
	m := settingsBlacklistOpenMenuAs(env, chat, user)
	add, ok := m.markedButton("settings_btn_bl_add")
	if !ok {
		env.t.Fatalf("the Blacklist page has no Add button")
	}
	m.tap(add)
	if !env.redis.Exists(settingsInputKey(user.Id)) {
		env.t.Fatalf("pressing Add left user %d with no wait", user.Id)
	}
	return m
}

// settingsListDoorSay sends text as a private message of user through the real dispatcher.
func settingsListDoorSay(env *settingsListEnv, user gotgbot.User, text string) int64 {
	env.t.Helper()
	return sendPrivateMessage(env.settingsEnv, user, gotgbot.Message{Text: text})
}

func TestSettingsListDoorOneStep(t *testing.T) {
	env := newSettingsListEnv(t)
	settingsListSeedBlacklistAction(t, env.chat.Id, "alpha", "ban")
	// The command's cached read is primed, so the typed add must drop it.
	blacklists.GetBlacklistSettingsContext(context.Background(), env.chat.Id)
	cacheKey := dbcache.CacheKey("blacklist", env.chat.Id)
	if !env.redis.Exists(cacheKey) {
		t.Fatalf("the command's cache key %s was not primed", cacheKey)
	}

	m := settingsListDoorWait(env, env.chat, env.admin)
	prompt, keyboard := m.last()
	for _, want := range []string{
		staffMarker("settings_header") + " " + staffMarker("settings_page_blacklist"),
		staffMarker("settings_bl_wait"), staffMarker("settings_bl_wait_rules") + " 100", staffMarker("settings_edit_wait_footer"),
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("the prompt lacks %q:\n%s", want, prompt)
		}
	}
	if len(keyboard) != 1 || len(keyboard[0]) != 1 || !strings.Contains(keyboard[0][0].Text, staffMarker("settings_btn_edit_cancel")) {
		t.Fatalf("the prompt keyboard = %+v, want one row with Cancel", keyboard)
	}
	if fields := settingsButtonFields(t, keyboard[0][0]); fields["a"] != "bc" || fields["t"] != "w" || len(fields) != 3 {
		t.Errorf("Cancel carries %v, want bc, t=w and c", fields)
	}
	wait := env.redis.HGet(settingsInputKey(env.admin.Id), "p") + "/" + env.redis.HGet(settingsInputKey(env.admin.Id), "t") + "/" +
		env.redis.HGet(settingsInputKey(env.admin.Id), "m")
	if want := fmt.Sprintf("b/w/%d", m.msgID); wait != want {
		t.Errorf("the wait reads page/part/prompt %q, want %q", wait, want)
	}

	sentBefore := len(env.fake.sentTo(env.admin.Id))
	settingsListDoorSay(env, env.admin, "Spam scam spam")

	stored := settingsBlacklistStored(t, env.chat.Id)
	if len(stored) != 3 || stored["spam"] != "ban" || stored["scam"] != "ban" || stored["alpha"] != "ban" {
		t.Errorf("stored %v, want spam and scam on ban beside alpha (the repeat counted once)", stored)
	}
	if env.redis.Exists(cacheKey) {
		t.Errorf("the command's cache key %s survived the add", cacheKey)
	}

	sent := env.fake.sentTo(env.admin.Id)
	if len(sent) != sentBefore+1 {
		t.Fatalf("the bot sent %d new messages, want exactly the saved page: %+v", len(sent)-sentBefore, sent[sentBefore:])
	}
	page := fmt.Sprint(sent[len(sent)-1].Params["text"])
	for _, want := range []string{
		staffMarker("settings_page_blacklist"), "<code>scam</code>", "<code>spam</code>",
		settingsNoticeOKIcon + staffMarker("settings_bl_saved_added") + " 2",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the saved page lacks %q:\n%s", want, page)
		}
	}
	if strings.Contains(page, staffMarker("settings_bl_saved_already")) {
		t.Errorf("the saved page has an already line for a repeat:\n%s", page)
	}
	saved := staffKeyboardOf(sent[len(sent)-1].Params["reply_markup"])
	if got := settingsListRowWidths(saved); fmt.Sprint(got) != fmt.Sprint([]int{3, 2, 2}) {
		t.Errorf("saved page rows = %v, want the delete row, [Add | Action] and the navigation row", got)
	}
	if _, ok := settingsButtonByMarker(saved, "settings_btn_bl_add"); !ok {
		t.Errorf("the saved page has no Add button: %+v", saved)
	}

	closed, closedKeyboard := m.last()
	if !strings.Contains(closed, staffMarker("settings_edit_prompt_closed")) || len(closedKeyboard) != 0 {
		t.Errorf("the prompt was edited to %q with keyboard %+v, want the closed line and none", closed, closedKeyboard)
	}
	if keys := settingsEditInputKeys(env.settingsEnv); len(keys) != 0 {
		t.Errorf("the wait left %v behind", keys)
	}

	t.Run("an empty list draws the Add button alone before the navigation", func(t *testing.T) {
		chat := env.addChat()
		settingsListCleanup(t, chat.Id)
		empty := settingsBlacklistOpenMenu(env, chat)
		_, emptyKeyboard := empty.last()
		if got := settingsListRowWidths(emptyKeyboard); fmt.Sprint(got) != fmt.Sprint([]int{1, 2}) {
			t.Errorf("empty list rows = %v, want [Add] and the navigation row", got)
		}
	})
}
