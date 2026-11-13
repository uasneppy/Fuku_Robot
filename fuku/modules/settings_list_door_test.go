//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db/blacklists"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
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

// settingsListDoorTexts is the texts the bot sent to user's private chat after the first `before` messages.
func settingsListDoorTexts(env *settingsListEnv, user gotgbot.User, before int) []string {
	return settingsEditTextsSince(env.settingsEnv, user, before)
}

// settingsListDoorChat is a fresh chat of the env with the bot an administrator and the one word alpha on warn, its
// rows cleaned up with the test.
func settingsListDoorChat(env *settingsListEnv) gotgbot.Chat {
	env.t.Helper()
	return settingsBlacklistWithBot(env, staffRoleAdmin, "alpha")
}

// settingsListDoorReply fails the test unless exactly one message was sent to user after the first `before` and it
// carries the marker of every key in wantKeys; it returns the message.
func settingsListDoorReply(t *testing.T, env *settingsListEnv, user gotgbot.User, before int, wantKeys ...string) staffSentMessage {
	t.Helper()
	sent := env.fake.sentTo(user.Id)
	if len(sent) != before+1 {
		t.Fatalf("the bot sent %d messages, want exactly 1: %q", len(sent)-before, settingsListDoorTexts(env, user, before))
	}
	text := fmt.Sprint(sent[before].Params["text"])
	for _, key := range wantKeys {
		if !strings.Contains(text, staffMarker(key)) {
			t.Errorf("the reply %q lacks %s", text, key)
		}
	}
	return sent[before]
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

func TestSettingsListDoorRefusals(t *testing.T) {
	env := newSettingsListEnv(t)
	owner := env.admin
	chat := settingsListDoorChat(env)
	m := settingsListDoorWait(env, chat, owner)
	env.redis.FastForward(2 * time.Second)
	ttl := env.redis.TTL(settingsInputKey(owner.Id))
	if ttl >= settingsInputTTL {
		t.Fatalf("the wait TTL = %v after the fast forward, want less than %v", ttl, settingsInputTTL)
	}
	keepWait := func(name string) {
		t.Helper()
		settingsEditWaitKept(t, env.settingsEnv, owner)
		if after := env.redis.TTL(settingsInputKey(owner.Id)); after != ttl {
			t.Errorf("%s: the wait TTL = %v, want the unchanged %v (a refusal never extends the wait)", name, after, ttl)
		}
		if got := settingsBlacklistStored(t, chat.Id); len(got) != 1 || got["alpha"] != "warn" {
			t.Errorf("%s: the refusal stored %v", name, got)
		}
	}

	for _, kind := range []string{"sticker", "photo"} {
		before := settingsEditSentCount(env.settingsEnv, owner)
		sendPrivateMessage(env.settingsEnv, owner, settingsMediaItem(kind))
		settingsListDoorReply(t, env, owner, before, "settings_edit_refuse_need_text", "settings_edit_still_waiting")
		keepWait(kind)
	}

	// Too broad, too long and already listed, in one message: counts and the still-waiting line, no word.
	before := settingsEditSentCount(env.settingsEnv, owner)
	settingsListDoorSay(env, owner, "a "+strings.Repeat("x", 101)+" ALPHA")
	reply := settingsListDoorReply(t, env, owner, before, "settings_bl_refuse_none", "settings_bl_saved_too_broad",
		"settings_bl_saved_too_long", "settings_bl_saved_already", "settings_edit_still_waiting")
	if text := fmt.Sprint(reply.Params["text"]); strings.Contains(text, "alpha") || strings.Contains(text, "xxxx") {
		t.Errorf("the refusal names a word:\n%s", text)
	}
	keyboard := staffKeyboardOf(reply.Params["reply_markup"])
	if len(keyboard) != 1 || len(keyboard[0]) != 1 || !containsMarker(keyboard[0][0].Text, "settings_btn_edit_cancel") {
		t.Fatalf("the refusal keyboard = %+v, want one row with Cancel", keyboard)
	}
	if fields := settingsButtonFields(t, keyboard[0][0]); fields["a"] != "bc" || fields["t"] != "w" || len(fields) != 3 {
		t.Errorf("the refusal's Cancel carries %v, want bc, t=w and c", fields)
	}
	if reply.Params["reply_parameters"] == nil {
		t.Error("the refusal is not a reply to the message")
	}
	keepWait("a refusal with counts")
	if !env.redis.Exists(settingsInputKey(owner.Id)) || env.redis.HGet(settingsInputKey(owner.Id), "r"+strconv.FormatInt(reply.MessageID, 10)) == "" {
		t.Errorf("the refusal reply %d was not noted on the wait", reply.MessageID)
	}

	// The refusal's own Cancel ends this wait, while the prompt's stays answerable only for a live wait.
	env.pressRaw(settingsPrivateChat(owner), owner, reply.MessageID, keyboard[0][0].CallbackData)
	settingsEditWaitGone(t, env.settingsEnv, owner)
	if text, _ := m.last(); !strings.Contains(text, staffMarker("settings_page_blacklist")) {
		t.Errorf("the prompt message was not left as the page: %q", text)
	}

	settingsListDoorRecovery(t, env)
}

// settingsListDoorRecovery is the second half of TestSettingsListDoorRefusals: what a refused or failed add leaves.
func settingsListDoorRecovery(t *testing.T, env *settingsListEnv) {
	owner := env.admin
	chat := settingsListDoorChat(env)
	settingsListDoorWait(env, chat, owner)

	// A refused message does not end the wait: the next one is read.
	settingsListDoorSay(env, owner, "a")
	settingsEditWaitKept(t, env.settingsEnv, owner)
	settingsListDoorSay(env, owner, "spam")
	if got := settingsBlacklistStored(t, chat.Id); got["spam"] != "warn" || len(got) != 2 {
		t.Errorf("stored %v, want spam added after a refusal", got)
	}
	settingsEditWaitGone(t, env.settingsEnv, owner)

	t.Run("a failed write for every word releases the claim and the wait is read again", func(t *testing.T) {
		chat := settingsListDoorChat(env)
		settingsListDoorWait(env, chat, owner)
		add := blacklistWrites.Add
		blacklistWrites.Add = func(context.Context, int64, string, string) error { return errors.New("the database is down") }
		t.Cleanup(func() { blacklistWrites.Add = add })
		before := settingsEditSentCount(env.settingsEnv, owner)

		settingsListDoorSay(env, owner, "one two")

		settingsListDoorReply(t, env, owner, before, "common_settings_save_failed", "settings_edit_still_waiting")
		settingsEditWaitKept(t, env.settingsEnv, owner)
		blacklistWrites.Add = add
		settingsListDoorSay(env, owner, "one")
		if got := settingsBlacklistStored(t, chat.Id); got["one"] != "warn" {
			t.Errorf("stored %v, want the retried word", got)
		}
	})

	t.Run("one of two writes failing is saved with the failed count", func(t *testing.T) {
		chat := settingsListDoorChat(env)
		settingsListDoorWait(env, chat, owner)
		add := blacklistWrites.Add
		blacklistWrites.Add = func(ctx context.Context, chatID int64, word, action string) error {
			if word == "bad" {
				return errors.New("the database is down")
			}
			return add(ctx, chatID, word, action)
		}
		t.Cleanup(func() { blacklistWrites.Add = add })
		before := settingsEditSentCount(env.settingsEnv, owner)

		settingsListDoorSay(env, owner, "good bad")

		reply := settingsListDoorReply(t, env, owner, before, "settings_bl_saved_added", "settings_bl_saved_failed")
		if text := fmt.Sprint(reply.Params["text"]); !strings.Contains(text, staffMarker("settings_bl_saved_failed")+" 1") {
			t.Errorf("the saved page lacks the failed count:\n%s", text)
		}
		if got := settingsBlacklistStored(t, chat.Id); got["good"] != "warn" || got["bad"] != "" {
			t.Errorf("stored %v, want only good", got)
		}
		settingsEditWaitGone(t, env.settingsEnv, owner)
	})
}

// settingsListDoorClearAdminCache drops the chat's cached administrator list on every layer, so the commands' cached
// predicates read what the fake says now.
func settingsListDoorClearAdminCache(chatID int64) {
	cache.InvalidateAdminCache(chatID)
	dbcache.ResetLocalForTest()
}

func TestSettingsListDoorAuthority(t *testing.T) {
	type world struct {
		name string
		// setup returns the person who presses Add (nil is the env's administrator, the group's creator).
		setup  func(env *settingsListEnv, chat gotgbot.Chat) gotgbot.User
		change func(env *settingsListEnv, chat gotgbot.Chat, person gotgbot.User)
		want   string
		kept   bool
	}
	restrictAdmin := staffFakeMember{Status: gotgbot.ChatMemberStatusAdministrator, CanRestrictMembers: true}
	gatedAdmin := func(env *settingsListEnv, chat gotgbot.Chat) gotgbot.User {
		env.newOwner(chat)
		admin := gotgbot.User{Id: uniqueLinkOwnerID() + 8_400_000, FirstName: "Plain"}
		env.fake.setMember(chat.Id, admin.Id, restrictAdmin)
		env.fake.listAdministrator(chat.Id, admin.Id)
		return admin
	}
	worlds := []world{
		{
			name: "the person is a plain member now", want: "settings_edit_refuse_access_lost",
			change: func(env *settingsListEnv, chat gotgbot.Chat, p gotgbot.User) {
				env.fake.setMember(chat.Id, p.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
			},
		},
		{
			name: "getChatMember of the person answers 500", want: "settings_check_failed", kept: true,
			change: func(env *settingsListEnv, chat gotgbot.Chat, _ gotgbot.User) {
				env.fake.script("getChatMember", chat.Id, staffFakeError(500, "Internal Server Error"))
			},
		},
		{
			name: "an owner exists and the switch is off", setup: gatedAdmin, want: "settings_gate_owner_only",
			change: func(env *settingsListEnv, chat gotgbot.Chat, _ gotgbot.User) { env.setAdminsAllowed(chat.Id, false) },
		},
		{
			name: "the administrator lost can_restrict_members", want: "settings_no_restrict",
			change: func(env *settingsListEnv, chat gotgbot.Chat, p gotgbot.User) {
				env.fake.setMember(chat.Id, p.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusAdministrator})
			},
		},
		{
			name: "the bot is no administrator, cached and live", want: "connections_is_user_connected_bot_not_admin",
			change: func(env *settingsListEnv, chat gotgbot.Chat, _ gotgbot.User) {
				env.fake.setBotRole(chat.Id, staffRoleMember)
				settingsListDoorClearAdminCache(chat.Id)
			},
		},
		{
			// The person's own lookup passes (nil), then the bot's cache-miss lookup and the door's live check of the
			// bot both meet a 429: nothing is established, so the wait is kept.
			name: "the bot is no administrator in the cache and the live check answers 429", want: "settings_check_failed", kept: true,
			change: func(env *settingsListEnv, chat gotgbot.Chat, _ gotgbot.User) {
				env.fake.setBotRole(chat.Id, staffRoleMember)
				settingsListDoorClearAdminCache(chat.Id)
				env.fake.script("getChatMember", chat.Id, nil, staffFake429(1), staffFake429(1), staffFake429(1))
			},
		},
		{
			// The bot's cache-miss lookup fails once, so the predicate says false, but the door's live record says the
			// bot is an administrator that can restrict: a stale answer is not a loss, so the wait is kept.
			name: "the cached bot answer is false and the live record says administrator", want: "settings_check_failed", kept: true,
			change: func(env *settingsListEnv, chat gotgbot.Chat, _ gotgbot.User) {
				settingsListDoorClearAdminCache(chat.Id)
				env.fake.script("getChatMember", chat.Id, nil, staffFake429(1), staffFake429(1), nil)
			},
		},
		{
			name: "the bot is an administrator without can_restrict_members", want: "chat_status_bot_restrict_group_error",
			change: func(env *settingsListEnv, chat gotgbot.Chat, _ gotgbot.User) {
				env.fake.setBotRole(chat.Id, staffRoleAdminNoRestrict)
				settingsListDoorClearAdminCache(chat.Id)
			},
		},
		{
			name: "getChat answers 400", want: "settings_edit_refuse_access_lost",
			change: func(env *settingsListEnv, chat gotgbot.Chat, _ gotgbot.User) {
				env.fake.script("getChat", chat.Id, staffFakeError(400, "Bad Request: chat not found"))
			},
		},
	}
	env := newSettingsListEnv(t)
	for _, w := range worlds {
		t.Run(w.name, func(t *testing.T) {
			resetSettingsOwnerMemo(t)
			chat := settingsListDoorChat(env)
			person := env.admin
			if w.setup != nil {
				person = w.setup(env, chat)
			}
			settingsListDoorWait(env, chat, person)
			w.change(env, chat, person)
			before := settingsEditSentCount(env.settingsEnv, person)

			settingsListDoorSay(env, person, "spam")

			reply := settingsListDoorReply(t, env, person, before, w.want)
			text := fmt.Sprint(reply.Params["text"])
			stopped := staffMarker("settings_edit_wait_stopped")
			switch {
			case w.kept || w.want == "settings_edit_refuse_access_lost":
				if strings.Contains(text, stopped) {
					t.Errorf("the reply %q carries the wait-stopped line", text)
				}
			case !strings.HasSuffix(text, staffMarker(w.want)+"\n\n"+stopped):
				t.Errorf("the reply %q is not the reason, a blank line and the wait-stopped line", text)
			}
			if w.kept {
				settingsEditWaitKept(t, env.settingsEnv, person)
			} else {
				settingsEditWaitGone(t, env.settingsEnv, person)
			}
			if got := settingsBlacklistStored(t, chat.Id); len(got) != 1 || got["alpha"] != "warn" {
				t.Errorf("a refused typed reply wrote: %v", got)
			}
		})
	}
}

func TestSettingsListDoorReplaced(t *testing.T) {
	env := newSettingsListEnv(t)
	owner := env.newOwner(env.chat)
	settingsListCleanup(t, env.chat.Id)
	welcomeMenu := settingsEditSetWait(env.settingsEnv, owner, settingsPageWelcome, settingsEditPartText)

	// The Blacklist Add replaces the Welcome text wait of the same group and says so.
	m := settingsListDoorWait(env, env.chat, owner)
	if text, _ := m.last(); !strings.Contains(text, staffMarker("settings_edit_replaced_note")) {
		t.Errorf("the Add that replaced a Welcome wait lacks the replaced note:\n%s", text)
	}
	if got := env.redis.HGet(settingsInputKey(owner.Id), "p"); got != settingsPageBlacklist {
		t.Errorf("the wait is for page %q, want the Blacklist", got)
	}
	// The Welcome prompt's Cancel is stale: it ends nothing.
	settingsEditPressButton(env.settingsEnv, owner, welcomeMenu, "settings_btn_edit_cancel")
	if text, alert := env.lastAnswer(); !strings.Contains(text, staffMarker("settings_edit_prompt_stale")) || !alert {
		t.Errorf("the stale Welcome Cancel answered %q (alert %v), want settings_edit_prompt_stale", text, alert)
	}
	settingsEditWaitKept(t, env.settingsEnv, owner)

	// Add again on the Blacklist page only refreshes the ten minutes: no note, the prompt message moves.
	m2 := settingsBlacklistOpenMenuAs(env, env.chat, owner)
	add, _ := m2.markedButton("settings_btn_bl_add")
	m2.tap(add)
	text, _ := m2.last()
	if strings.Contains(text, staffMarker("settings_edit_replaced_note")) {
		t.Errorf("a second Add for the same page shows the replaced note:\n%s", text)
	}
	if got := env.redis.HGet(settingsInputKey(owner.Id), "m"); got != strconv.FormatInt(m2.msgID, 10) {
		t.Errorf("the wait's prompt is %q, want the newer message %d", got, m2.msgID)
	}
}

func TestSettingsListDoorExpired(t *testing.T) {
	env := newSettingsListEnv(t)
	owner := env.admin
	chat := settingsListDoorChat(env)
	settingsListDoorWait(env, chat, owner)
	env.redis.FastForward(settingsInputTTL + time.Second)
	if env.redis.Exists(settingsInputKey(owner.Id)) || !env.redis.Exists(settingsInputDoneKey(owner.Id)) {
		t.Fatalf("after ten minutes the keys are %v, want only the marker", settingsEditInputKeys(env.settingsEnv))
	}
	before := settingsEditSentCount(env.settingsEnv, owner)

	settingsListDoorSay(env, owner, "too late")

	settingsListDoorReply(t, env, owner, before, "settings_entries_wait_ended_note", "settings_entries_part_words")
	if got := settingsBlacklistStored(t, chat.Id); len(got) != 1 {
		t.Errorf("a late reply stored %v", got)
	}
	settingsEditWaitGone(t, env.settingsEnv, owner)

	// The second delivery is an ordinary private message.
	before = settingsEditSentCount(env.settingsEnv, owner)
	settingsListDoorSay(env, owner, "still too late")
	if texts := settingsListDoorTexts(env, owner, before); len(texts) != 0 {
		t.Errorf("a second late message was answered: %q", texts)
	}
}

func TestSettingsListDoorCancel(t *testing.T) {
	env := newSettingsListEnv(t)
	owner := env.admin
	chat := settingsListDoorChat(env)

	t.Run("the prompt's Cancel ends the wait and shows the list", func(t *testing.T) {
		m := settingsListDoorWait(env, chat, owner)
		_, keyboard := m.last()
		m.tap(keyboard[0][0])
		text, _ := m.last()
		for _, want := range []string{staffMarker("settings_page_blacklist"), settingsNoticeOKIcon + staffMarker("settings_edit_cancelled"), "<code>alpha</code>"} {
			if !strings.Contains(text, want) {
				t.Errorf("the cancelled prompt lacks %q:\n%s", want, text)
			}
		}
		settingsEditWaitGone(t, env.settingsEnv, owner)
	})

	t.Run("a stale prompt answers stale and leaves the live wait", func(t *testing.T) {
		old := settingsListDoorWait(env, chat, owner)
		_, oldKeyboard := old.last()
		oldCancel := oldKeyboard[0][0]
		live := settingsListDoorWait(env, chat, owner)

		env.pressRaw(settingsPrivateChat(owner), owner, old.msgID, oldCancel.CallbackData)

		if text, alert := env.lastAnswer(); !strings.Contains(text, staffMarker("settings_edit_prompt_stale")) || !alert {
			t.Errorf("the stale Cancel answered %q (alert %v), want settings_edit_prompt_stale", text, alert)
		}
		settingsEditWaitKept(t, env.settingsEnv, owner)
		if got := env.redis.HGet(settingsInputKey(owner.Id), "m"); got != strconv.FormatInt(live.msgID, 10) {
			t.Errorf("the live wait's prompt = %q, want %d", got, live.msgID)
		}
		if text, _ := old.last(); !strings.Contains(text, staffMarker("settings_page_blacklist")) || strings.Contains(text, staffMarker("settings_edit_wait_footer")) {
			t.Errorf("the stale prompt was not edited into the page:\n%s", text)
		}
	})

	t.Run("/cancel deletes the wait", func(t *testing.T) {
		settingsListDoorWait(env, chat, owner)
		before := settingsEditSentCount(env.settingsEnv, owner)
		sendPrivateMessage(env.settingsEnv, owner, gotgbot.Message{
			Text: "/cancel", Entities: []gotgbot.MessageEntity{{Type: "bot_command", Offset: 0, Length: 7}},
		})
		settingsListDoorReply(t, env, owner, before, "settings_edit_cancelled")
		settingsEditWaitGone(t, env.settingsEnv, owner)
	})
}

func TestSettingsListDoorAlbum(t *testing.T) {
	env := newSettingsListEnv(t)
	owner := env.admin
	chat := settingsListDoorChat(env)
	settingsListDoorWait(env, chat, owner)
	const group = "9100123"
	first, second := settingsMediaItemID("photo", "album-1"), settingsMediaItemID("photo", "album-2")
	first.Caption = "spam scam"
	first.MediaGroupId, second.MediaGroupId = group, group

	before := settingsEditSentCount(env.settingsEnv, owner)
	sendPrivateMessage(env.settingsEnv, owner, first)
	calls := func() int {
		total := 0
		for _, method := range []string{"getChatMember", "getChat", "sendMessage", "editMessageText"} {
			total += len(env.fake.callsFor(method))
		}
		return total
	}
	afterFirst := calls()
	sendPrivateMessage(env.settingsEnv, owner, second)

	if got := calls(); got != afterFirst {
		t.Errorf("the second item of the album made %d Telegram calls, want none", got-afterFirst)
	}
	reply := settingsListDoorReply(t, env, owner, before, "settings_bl_saved_added", "settings_edit_album_cut")
	if text := fmt.Sprint(reply.Params["text"]); !strings.Contains(text, staffMarker("settings_bl_saved_added")+" 2") {
		t.Errorf("the saved page lacks Added 2:\n%s", text)
	}
	if got := settingsBlacklistStored(t, chat.Id); len(got) != 3 || got["spam"] == "" || got["scam"] == "" {
		t.Errorf("stored %v, want spam and scam from the album's first caption, once", got)
	}
}

func TestSettingsListDoorNoRedis(t *testing.T) {
	env := newSettingsListEnv(t)
	owner := env.admin
	chat := settingsListDoorChat(env)
	m := settingsBlacklistOpenMenuAs(env, chat, owner)
	alphaID := settingsBlacklistIDOf(t, chat.Id, "alpha")
	add, _ := m.markedButton("settings_btn_bl_add")
	edits := len(env.edits(owner.Id, m.msgID))

	restore := cache.DisableRedisForTest()
	answers := env.answerCount()
	m.tap(add)
	text, alert := env.lastAnswer()
	gotAnswers := env.answerCount() - answers
	if gotAnswers != 1 || !alert || !strings.Contains(text, staffMarker("settings_entries_add_unavailable")) {
		t.Errorf("Add without Redis answered %q (alert %v, %d answers), want one alert settings_entries_add_unavailable", text, alert, gotAnswers)
	}
	if got := len(env.edits(owner.Id, m.msgID)); got != edits {
		t.Errorf("Add without Redis edited the menu %d times", got-edits)
	}

	// The list, the card with its Yes and the action keep working: they read the database and keep no state.
	m.press("a", "bp", "v", "1")
	m.press("a", "bx", "r", strconv.FormatInt(alphaID, 10), "v", "1")
	if card, _ := m.last(); !strings.Contains(card, staffMarker("settings_bl_delete_ask")) {
		t.Errorf("the delete card did not draw without Redis:\n%s", card)
	}
	m.press("a", settingsActBlacklistSet, "v", "ban")
	m.press("a", "by", "r", strconv.FormatInt(alphaID, 10), "e", settingsCardExpiry(time.Now()))
	restore()
	if got := settingsBlacklistStored(t, chat.Id); len(got) != 0 {
		t.Errorf("the delete without Redis left %v", got)
	}
	if keys := settingsEditInputKeys(env.settingsEnv); len(keys) != 0 {
		t.Errorf("Add without Redis left %v", keys)
	}
}
