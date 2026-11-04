//go:build testtools

package modules

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	logrustest "github.com/sirupsen/logrus/hooks/test"

	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/greetings"
	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
	"github.com/uasneppy/fuku_robot/fuku/utils/content"
)

// privateUpdate builds the update of a private message of from to the bot (the chat, the sender,
// the ID and the date are filled in).
func privateUpdate(env *settingsEnv, from gotgbot.User, msg gotgbot.Message) *gotgbot.Update {
	id := env.updateID()
	msg.MessageId = id
	msg.Date = 1
	msg.Chat = settingsPrivateChat(from)
	msg.From = &from
	return &gotgbot.Update{UpdateId: id, Message: &msg}
}

// sendPrivateMessage runs msg through the real dispatcher as a private message of from to the
// bot and returns the message's ID.
func sendPrivateMessage(env *settingsEnv, from gotgbot.User, msg gotgbot.Message) int64 {
	env.t.Helper()
	update := privateUpdate(env, from, msg)
	if err := env.dispatcher.ProcessUpdate(env.bot, update, nil); err != nil {
		env.t.Fatalf("ProcessUpdate(%d) error = %v", update.UpdateId, err)
	}
	return update.UpdateId
}

// sendGroupMessage runs msg through the real dispatcher as a message of from in chat and returns
// the message's ID.
func sendGroupMessage(env *settingsEnv, chat gotgbot.Chat, from gotgbot.User, msg gotgbot.Message) int64 {
	env.t.Helper()
	id := env.updateID()
	msg.MessageId = id
	msg.Date = 1
	msg.Chat = chat
	msg.From = &from
	if err := env.dispatcher.ProcessUpdate(env.bot, &gotgbot.Update{UpdateId: id, Message: &msg}, nil); err != nil {
		env.t.Fatalf("ProcessUpdate(%d) error = %v", id, err)
	}
	return id
}

// settingsEditSetWait opens page pageKey of the env's group in user's private menu and presses
// Set of part, so user has a wait. It returns the ID of the menu message, which is now the
// waiting prompt.
func settingsEditSetWait(env *settingsEnv, user gotgbot.User, pageKey, part string) int64 {
	env.t.Helper()
	def, ok := settingsEditPageByKey(pageKey)
	if !ok {
		env.t.Fatalf("no editor page %q", pageKey)
	}
	p := settingsEditPartByCode(def, part)
	if p == nil {
		env.t.Fatalf("page %q has no part %q", pageKey, part)
	}
	menuID := openEditPagePrivate(env, user, pageKey)
	settingsEditPressButton(env, user, menuID, p.SetLabelKey)
	if !env.redis.Exists(settingsInputKey(user.Id)) {
		env.t.Fatalf("pressing Set of %s/%s left user %d with no wait", pageKey, part, user.Id)
	}
	return menuID
}

func TestSettingsEditTypedTextSaved(t *testing.T) {
	env := newSettingsEditEnv(t)
	owner := env.newOwner(env.chat)
	settingsEditSeedGreetings(t, env.chat.Id)
	before, _ := settingsEditStored(t, env.chat.Id)
	promptID := settingsEditSetWait(env, owner, settingsPageWelcome, settingsEditPartText)
	sentBefore := len(env.fake.sentTo(owner.Id))

	// "Hi {first} 😀 bold" with bold on the last word; the emoji is two UTF-16 units.
	const typed = "Hi {first} 😀 bold"
	entities := []gotgbot.MessageEntity{{Type: "bold", Offset: 14, Length: 4}}
	sendPrivateMessage(env, owner, gotgbot.Message{Text: typed, Entities: entities})

	want := content.ExtractWelcome(&gotgbot.Message{
		Text:     "/setwelcome " + typed,
		Entities: shiftEntities(entities, "/setwelcome "),
	}, "welcome", "en")
	if want.DataType == -1 {
		t.Fatalf("the command refused the text: %q", want.ErrorMsg)
	}
	row, ok := settingsEditStored(t, env.chat.Id)
	if !ok {
		t.Fatal("nothing was stored")
	}
	if row.WelcomeSettings.WelcomeText != want.Text {
		t.Errorf("welcome_text = %q, want what /setwelcome stores: %q", row.WelcomeSettings.WelcomeText, want.Text)
	}
	if row.WelcomeSettings.FileID != before.WelcomeSettings.FileID ||
		row.WelcomeSettings.WelcomeType != before.WelcomeSettings.WelcomeType ||
		len(row.WelcomeSettings.Button) != len(before.WelcomeSettings.Button) {
		t.Errorf("the media or buttons changed: before %+v, after %+v", before.WelcomeSettings, row.WelcomeSettings)
	}
	if row.GoodbyeSettings.GoodbyeText != before.GoodbyeSettings.GoodbyeText {
		t.Errorf("the goodbye changed to %q", row.GoodbyeSettings.GoodbyeText)
	}

	// The saved page is one new message in the owner's private chat, with the page's keyboard.
	sent := env.fake.sentTo(owner.Id)
	if len(sent) != sentBefore+1 {
		t.Fatalf("the bot sent %d new messages to the owner, want exactly 1: %+v", len(sent)-sentBefore, sent[sentBefore:])
	}
	page := sent[len(sent)-1]
	text := fmt.Sprint(page.Params["text"])
	for _, wantText := range []string{
		staffMarker("settings_header") + " " + staffMarker("settings_page_welcome"),
		fmt.Sprint(env.chat.Id),
		staffMarker("settings_edit_saved_text"),
	} {
		if !strings.Contains(text, wantText) {
			t.Errorf("the saved page %q lacks %q", text, wantText)
		}
	}
	if strings.Contains(text, staffMarker("settings_edit_caption_ignored")) {
		t.Errorf("a text reply carries the caption note: %q", text)
	}
	keyboard := staffKeyboardOf(page.Params["reply_markup"])
	if _, ok := settingsButtonByMarker(keyboard, "settings_btn_edit_set_text"); !ok {
		t.Errorf("the saved page has no Set text button: %+v", keyboard)
	}
	if _, ok := settingsButtonByMarker(keyboard, "settings_btn_edit_remove_text"); !ok {
		t.Errorf("the saved page does not offer Remove for the text it just saved: %+v", keyboard)
	}

	// The prompt is closed with no keyboard, and the wait and its marker are gone.
	closed, closedKeyboard := env.lastEdit(owner.Id, promptID)
	if !strings.Contains(closed, staffMarker("settings_edit_prompt_closed")) || len(closedKeyboard) != 0 {
		t.Errorf("the prompt was edited to %q with keyboard %+v, want the closed line and none", closed, closedKeyboard)
	}
	if keys := settingsEditInputKeys(env); len(keys) != 0 {
		t.Errorf("the wait left %v behind", keys)
	}
}

// settingsEditWaitKept fails the test unless user still has a wait with no claim held.
func settingsEditWaitKept(t *testing.T, env *settingsEnv, user gotgbot.User) {
	t.Helper()
	key := settingsInputKey(user.Id)
	if !env.redis.Exists(key) {
		t.Errorf("user %d lost the wait, want it kept", user.Id)
		return
	}
	if claim := env.redis.HGet(key, "k"); claim != "" {
		t.Errorf("the wait of user %d still holds the claim %q", user.Id, claim)
	}
}

// settingsEditWaitGone fails the test when user still has a wait or its marker.
func settingsEditWaitGone(t *testing.T, env *settingsEnv, user gotgbot.User) {
	t.Helper()
	if env.redis.Exists(settingsInputKey(user.Id)) || env.redis.Exists(settingsInputDoneKey(user.Id)) {
		t.Errorf("user %d still has a wait or its marker: %v", user.Id, settingsEditInputKeys(env))
	}
}

// settingsEditTextsSince returns the texts the bot sent to user's private chat after the first
// `before` messages.
func settingsEditTextsSince(env *settingsEnv, user gotgbot.User, before int) []string {
	return settingsGateNewTexts(env, user.Id, before)
}

// settingsEditSettingsKeys lists the Redis keys of the settings family.
func settingsEditSettingsKeys(env *settingsEnv) []string {
	var found []string
	for _, key := range env.redis.Keys() {
		if strings.HasPrefix(key, "fuku:settings:") {
			found = append(found, key)
		}
	}
	return found
}

// settingsEditSentCount is how many messages the bot has sent to user's private chat.
func settingsEditSentCount(env *settingsEnv, user gotgbot.User) int {
	return len(env.fake.sentTo(user.Id))
}

// settingsEditWantReply fails the test unless exactly one message was sent
// after `before` and it carries the marker of every key in wantKeys.
func settingsEditWantReply(t *testing.T, env *settingsEnv, user gotgbot.User, before int, wantKeys ...string) {
	t.Helper()
	texts := settingsEditTextsSince(env, user, before)
	if len(texts) != 1 {
		t.Fatalf("the bot sent %d messages, want exactly 1: %q", len(texts), texts)
	}
	for _, key := range wantKeys {
		if !strings.Contains(texts[0], staffMarker(key)) {
			t.Errorf("the reply %q lacks %s", texts[0], key)
		}
	}
}

// settingsEditCountingWrite replaces the welcome text write with one that counts its calls and
// then runs the real write (or fails with err when err is set). The real write is restored when
// the test ends.
func settingsEditCountingWrite(t *testing.T, err error) *atomic.Int32 {
	t.Helper()
	var calls atomic.Int32
	previous := greetingWrites.WelcomeTextOnly
	greetingWrites.WelcomeTextOnly = func(chatID int64, text string) error {
		calls.Add(1)
		if err != nil {
			return err
		}
		return previous(chatID, text)
	}
	t.Cleanup(func() { greetingWrites.WelcomeTextOnly = previous })
	return &calls
}

func TestSettingsEditTextParity(t *testing.T) {
	cases := append([]struct {
		name     string
		text     string
		entities []gotgbot.MessageEntity
	}{}, settingsEditParityCases...)
	cases = append(cases, struct {
		name     string
		text     string
		entities []gotgbot.MessageEntity
	}{name: "mention placeholder", text: "Hello {mention}, read {rules}"})

	env := newSettingsEditEnv(t)
	commandChat, editorChat := env.addChat(), env.addChat()
	editCleanup(t, commandChat.Id, editorChat.Id)
	// One owner runs the commands in every case: the cached administrator list of the chat names
	// the first owner only.
	commandOwner := env.newOwner(commandChat)
	for _, page := range []string{settingsPageWelcome, settingsPageGoodbye} {
		command := "/setwelcome "
		if page == settingsPageGoodbye {
			command = "/setgoodbye "
		}
		for _, tc := range cases {
			t.Run(page+"/"+tc.name, func(t *testing.T) {
				editorOwner := env.newOwner(editorChat)
				sendGroupMessage(env, commandChat, commandOwner, gotgbot.Message{
					Text: command + tc.text, Entities: shiftEntities(tc.entities, command),
				})
				commandRow, ok := settingsEditStored(t, commandChat.Id)
				if !ok {
					t.Fatalf("%s stored nothing", command)
				}

				envChat := env.chat
				env.chat = editorChat
				defer func() { env.chat = envChat }()
				settingsEditSetWait(env, editorOwner, page, settingsEditPartText)
				sendPrivateMessage(env, editorOwner, gotgbot.Message{Text: tc.text, Entities: tc.entities})

				editorRow, ok := settingsEditStored(t, editorChat.Id)
				if !ok {
					t.Fatal("the editor stored nothing")
				}
				if page == settingsPageWelcome {
					if editorRow.WelcomeSettings.WelcomeText != commandRow.WelcomeSettings.WelcomeText {
						t.Errorf("welcome_text differs\n editor: %q\ncommand: %q",
							editorRow.WelcomeSettings.WelcomeText, commandRow.WelcomeSettings.WelcomeText)
					}
					return
				}
				if editorRow.GoodbyeSettings.GoodbyeText != commandRow.GoodbyeSettings.GoodbyeText {
					t.Errorf("goodbye_text differs\n editor: %q\ncommand: %q",
						editorRow.GoodbyeSettings.GoodbyeText, commandRow.GoodbyeSettings.GoodbyeText)
				}
			})
		}
	}
}

func TestSettingsEditRulesParity(t *testing.T) {
	env := newSettingsEditEnv(t)
	commandChat, editorChat := env.addChat(), env.addChat()
	editCleanup(t, commandChat.Id, editorChat.Id)
	const command = "/setrules "
	commandOwner := env.newOwner(commandChat)
	for _, tc := range settingsEditParityCases {
		t.Run(tc.name, func(t *testing.T) {
			editorOwner := env.newOwner(editorChat)
			sendGroupMessage(env, commandChat, commandOwner, gotgbot.Message{
				Text: command + tc.text, Entities: shiftEntities(tc.entities, command),
			})
			commandRow, ok := settingsEditRulesStored(t, commandChat.Id)
			if !ok {
				t.Fatalf("%s stored nothing", command)
			}

			envChat := env.chat
			env.chat = editorChat
			defer func() { env.chat = envChat }()
			settingsEditSetWait(env, editorOwner, settingsPageRules, settingsEditPartText)
			sendPrivateMessage(env, editorOwner, gotgbot.Message{Text: tc.text, Entities: tc.entities})

			editorRow, ok := settingsEditRulesStored(t, editorChat.Id)
			if !ok {
				t.Fatal("the editor stored nothing")
			}
			if editorRow.Rules != commandRow.Rules {
				t.Errorf("rules differ\n editor: %q\ncommand: %q", editorRow.Rules, commandRow.Rules)
			}
		})
	}
}

func TestSettingsEditDoorPassThrough(t *testing.T) {
	t.Run("a stranger with no wait and no marker is left alone", func(t *testing.T) {
		env := newSettingsEditEnv(t)
		stranger := gotgbot.User{Id: uniqueLinkOwnerID() + 8_100_000, FirstName: "Stranger"}
		keys := len(settingsEditSettingsKeys(env))
		lookups := len(env.fake.callsFor("getChatMember"))
		sends := len(env.fake.callsFor("sendMessage"))

		sendPrivateMessage(env, stranger, gotgbot.Message{Text: "hello bot"})

		if got := settingsEditSentCount(env, stranger); got != 0 {
			t.Errorf("the bot answered a stranger %d times", got)
		}
		if got := len(env.fake.callsFor("getChatMember")); got != lookups {
			t.Errorf("getChatMember was called %d more times", got-lookups)
		}
		if got := len(env.fake.callsFor("sendMessage")); got != sends {
			t.Errorf("sendMessage was called %d more times", got-sends)
		}
		if got := len(settingsEditSettingsKeys(env)); got != keys {
			t.Errorf("Redis holds %d settings keys, want the %d it held before: %v", got, keys, settingsEditSettingsKeys(env))
		}
	})

	t.Run("a message that cannot be a reply leaves the wait alone", func(t *testing.T) {
		photo := []gotgbot.PhotoSize{{FileId: "photo-id", FileUniqueId: "photo-unique", Width: 1, Height: 1}}
		command := []gotgbot.MessageEntity{{Type: "bot_command", Offset: 0, Length: 4}}
		cases := []struct {
			name string
			// who is the person with the wait; nil is the group's owner.
			who  *gotgbot.User
			send func(env *settingsEnv, who gotgbot.User)
		}{
			{
				name: "a Telegram service identity",
				who:  &gotgbot.User{Id: 777000, FirstName: "Telegram"},
				send: func(env *settingsEnv, who gotgbot.User) { sendPrivateMessage(env, who, gotgbot.Message{Text: "hello"}) },
			},
			{
				name: "a bot sender",
				who:  &gotgbot.User{Id: uniqueLinkOwnerID() + 8_200_000, FirstName: "Robot", IsBot: true},
				send: func(env *settingsEnv, who gotgbot.User) { sendPrivateMessage(env, who, gotgbot.Message{Text: "hello"}) },
			},
			{
				name: "an edited message",
				send: func(env *settingsEnv, who gotgbot.User) {
					update := privateUpdate(env, who, gotgbot.Message{Text: "edited text"})
					update.EditedMessage, update.Message = update.Message, nil
					if err := env.dispatcher.ProcessUpdate(env.bot, update, nil); err != nil {
						env.t.Fatalf("ProcessUpdate error = %v", err)
					}
				},
			},
			{
				name: "a message with a rich message",
				send: func(env *settingsEnv, who gotgbot.User) {
					sendPrivateMessage(env, who, gotgbot.Message{Text: "hello", RichMessage: &gotgbot.RichMessage{}})
				},
			},
			{
				name: "a command",
				send: func(env *settingsEnv, who gotgbot.User) {
					sendPrivateMessage(env, who, gotgbot.Message{Text: "/foo", Entities: command})
				},
			},
			{
				name: "a photo captioned with a command",
				send: func(env *settingsEnv, who gotgbot.User) {
					sendPrivateMessage(env, who, gotgbot.Message{Photo: photo, Caption: "/foo", CaptionEntities: command})
				},
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				env := newSettingsEditEnv(t)
				var who gotgbot.User
				if tc.who != nil {
					who = *tc.who
					settingsInputSeed(t, who.Id, env.chat.Id, settingsPageWelcome, settingsEditPartText)
				} else {
					who = env.newOwner(env.chat)
					settingsEditSetWait(env, who, settingsPageWelcome, settingsEditPartText)
				}
				before := settingsEditSentCount(env, who)

				tc.send(env, who)

				settingsEditWaitKept(t, env, who)
				if texts := settingsEditTextsSince(env, who, before); len(texts) != 0 {
					t.Errorf("the door answered: %q", texts)
				}
				if row, ok := settingsEditStored(t, env.chat.Id); ok {
					t.Errorf("a greetings row was stored: %+v", row)
				}
			})
		}
	})

	t.Run("with Redis down nothing happens", func(t *testing.T) {
		env := newSettingsEditEnv(t)
		owner := env.newOwner(env.chat)
		settingsEditSetWait(env, owner, settingsPageWelcome, settingsEditPartText)
		before := settingsEditSentCount(env, owner)

		restore := cache.DisableRedisForTest()
		sendPrivateMessage(env, owner, gotgbot.Message{Text: "typed while Redis is down"})
		restore()

		if texts := settingsEditTextsSince(env, owner, before); len(texts) != 0 {
			t.Errorf("the door answered without Redis: %q", texts)
		}
		settingsEditWaitKept(t, env, owner)
		if row, ok := settingsEditStored(t, env.chat.Id); ok {
			t.Errorf("a greetings row was stored: %+v", row)
		}
	})

	t.Run("a #name text is saved as the text, the notes watcher does not take it", func(t *testing.T) {
		env := newSettingsEditEnv(t)
		LoadNotes(env.dispatcher)
		owner := env.newOwner(env.chat)
		settingsEditSetWait(env, owner, settingsPageWelcome, settingsEditPartText)

		sendPrivateMessage(env, owner, gotgbot.Message{Text: "#rules"})

		row, ok := settingsEditStored(t, env.chat.Id)
		if !ok || row.WelcomeSettings.WelcomeText != "#rules" {
			t.Errorf("the welcome is %+v (row %v), want the typed #rules", row.WelcomeSettings, ok)
		}
		settingsEditWaitGone(t, env, owner)
	})
}

func TestSettingsEditDoorAuthority(t *testing.T) {
	type world struct {
		name string
		page string
		// setup returns the person who presses Set; the wait is set while the world still lets
		// them in. change then moves the world.
		setup  func(env *settingsEnv) gotgbot.User
		change func(env *settingsEnv, person gotgbot.User)
		want   string
		kept   bool
	}
	owner := func(env *settingsEnv) gotgbot.User { return env.newOwner(env.chat) }
	gatedAdmin := func(env *settingsEnv) gotgbot.User {
		env.newOwner(env.chat)
		admin := gotgbot.User{Id: uniqueLinkOwnerID() + 8_300_000, FirstName: "Plain"}
		listEditAdmin(env, env.chat, admin, true)
		return admin
	}
	worlds := []world{
		{
			name: "the person is a plain member now", page: settingsPageWelcome, setup: owner, want: "settings_edit_refuse_access_lost",
			change: func(env *settingsEnv, p gotgbot.User) {
				env.fake.setMember(env.chat.Id, p.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
			},
		},
		{
			name: "getChatMember answers 403", page: settingsPageWelcome, setup: owner, want: "settings_edit_refuse_access_lost",
			change: func(env *settingsEnv, _ gotgbot.User) {
				env.fake.script("getChatMember", env.chat.Id, staffFakeError(403, "Forbidden: bot was kicked from the supergroup chat"))
			},
		},
		{
			name: "getChatMember answers 429", page: settingsPageWelcome, setup: owner, want: "settings_check_failed", kept: true,
			change: func(env *settingsEnv, _ gotgbot.User) { env.fake.script("getChatMember", env.chat.Id, staffFake429(1)) },
		},
		{
			name: "getChatMember answers 500", page: settingsPageWelcome, setup: owner, want: "settings_check_failed", kept: true,
			change: func(env *settingsEnv, _ gotgbot.User) {
				env.fake.script("getChatMember", env.chat.Id, staffFakeError(500, "Internal Server Error"))
			},
		},
		{
			name: "an owner exists and the switch is off", page: settingsPageWelcome, setup: gatedAdmin, want: "settings_gate_owner_only",
			change: func(env *settingsEnv, _ gotgbot.User) { env.setAdminsAllowed(env.chat.Id, false) },
		},
		{
			name: "the owner state cannot be read", page: settingsPageWelcome, setup: gatedAdmin, want: "settings_check_failed", kept: true,
			change: func(env *settingsEnv, _ gotgbot.User) {
				env.setAdminsAllowed(env.chat.Id, false)
				env.fake.setFailure("getChatAdministrators", env.chat.Id, staffFakeError(500, "Internal Server Error"))
			},
		},
		{
			name: "the administrator lost can_change_info", page: settingsPageWelcome, setup: gatedAdmin, want: "settings_no_change_info",
			change: func(env *settingsEnv, p gotgbot.User) {
				env.fake.setMemberRaw(env.chat.Id, p.Id, settingsEditAdminJSON(p.Id, false))
			},
		},
		{
			name: "the bot is no administrator and the page is Rules", page: settingsPageRules, setup: owner, want: "chat_status_bot_not_admin",
			change: func(env *settingsEnv, _ gotgbot.User) {
				env.fake.setBotRole(env.chat.Id, staffRoleMember)
				dbcache.DeleteCache(dbcache.CacheKey("admin_list", env.chat.Id))
				dbcache.ResetLocalForTest()
			},
		},
		{
			name: "getChat answers 400", page: settingsPageWelcome, setup: owner, want: "settings_edit_refuse_access_lost",
			change: func(env *settingsEnv, _ gotgbot.User) {
				env.fake.script("getChat", env.chat.Id, staffFakeError(400, "Bad Request: chat not found"))
			},
		},
		{
			name: "getChat answers 500", page: settingsPageWelcome, setup: owner, want: "settings_check_failed", kept: true,
			change: func(env *settingsEnv, _ gotgbot.User) {
				env.fake.script("getChat", env.chat.Id, staffFakeError(500, "Internal Server Error"))
			},
		},
		{
			name: "the chat is not a group", page: settingsPageWelcome, setup: owner, want: "settings_edit_refuse_access_lost",
			change: func(env *settingsEnv, _ gotgbot.User) { env.fake.setChatType(env.chat.Id, "channel") },
		},
	}
	for _, w := range worlds {
		t.Run(w.name, func(t *testing.T) {
			resetSettingsOwnerMemo(t)
			env := newSettingsEditEnv(t)
			person := w.setup(env)
			settingsEditSetWait(env, person, w.page, settingsEditPartText)
			w.change(env, person)
			before := settingsEditSentCount(env, person)

			sendPrivateMessage(env, person, gotgbot.Message{Text: "a new text"})

			settingsEditWantReply(t, env, person, before, w.want)
			if w.kept {
				settingsEditWaitKept(t, env, person)
			} else {
				settingsEditWaitGone(t, env, person)
			}
			if row, ok := settingsEditStored(t, env.chat.Id); ok {
				t.Errorf("a greetings row was stored: %+v", row)
			}
			if row, ok := settingsEditRulesStored(t, env.chat.Id); ok && row.Rules != "" {
				t.Errorf("rules were stored: %+v", row)
			}
		})
	}
}

func TestSettingsEditRefusalKeepsWait(t *testing.T) {
	env := newSettingsEditEnv(t)
	owner := env.newOwner(env.chat)
	settingsEditSetWait(env, owner, settingsPageWelcome, settingsEditPartText)
	env.redis.FastForward(2 * time.Second)
	ttl := env.redis.TTL(settingsInputKey(owner.Id))
	if ttl >= settingsInputTTL {
		t.Fatalf("the wait TTL = %v after the fast forward, want less than %v", ttl, settingsInputTTL)
	}
	before := settingsEditSentCount(env, owner)

	photo := []gotgbot.PhotoSize{{FileId: "photo-id", FileUniqueId: "photo-unique", Width: 1, Height: 1}}
	sendPrivateMessage(env, owner, gotgbot.Message{Photo: photo})

	settingsEditWantReply(t, env, owner, before, "settings_edit_refuse_need_text", "settings_edit_still_waiting")
	sent := env.fake.sentTo(owner.Id)
	reply := sent[len(sent)-1]
	keyboard := staffKeyboardOf(reply.Params["reply_markup"])
	if len(keyboard) != 1 || len(keyboard[0]) != 1 || !containsMarker(keyboard[0][0].Text, "settings_btn_edit_cancel") {
		t.Fatalf("the refusal keyboard = %+v, want one row with Cancel", keyboard)
	}
	fields := settingsButtonFields(t, keyboard[0][0])
	if fields["a"] != "wc" || fields["t"] != "x" || fields["c"] != fmt.Sprint(env.chat.Id) || len(fields) != 3 {
		t.Errorf("Cancel carries %v, want wc, t=x and c", fields)
	}
	if reply.Params["reply_parameters"] == nil {
		t.Error("the refusal is not a reply to the message")
	}
	settingsEditWaitKept(t, env, owner)
	if after := env.redis.TTL(settingsInputKey(owner.Id)); after != ttl {
		t.Errorf("the wait TTL = %v after the refusal, want the unchanged %v (a refusal never extends the wait)", after, ttl)
	}

	// An over-long text is refused with the commands' own words, and the wait stays.
	before = settingsEditSentCount(env, owner)
	sendPrivateMessage(env, owner, gotgbot.Message{Text: strings.Repeat("x", 4097)})
	settingsEditWantReply(t, env, owner, before, "content_text_too_long", "settings_edit_still_waiting")
	settingsEditWaitKept(t, env, owner)

	// A valid text sent next is saved.
	sendPrivateMessage(env, owner, gotgbot.Message{Text: "a valid welcome"})
	if row, ok := settingsEditStored(t, env.chat.Id); !ok || row.WelcomeSettings.WelcomeText != "a valid welcome" {
		t.Errorf("the welcome is %+v (row %v), want the valid text", row.WelcomeSettings, ok)
	}
	settingsEditWaitGone(t, env, owner)
}

func TestSettingsEditWriteFailureReleases(t *testing.T) {
	env := newSettingsEditEnv(t)
	owner := env.newOwner(env.chat)
	settingsEditSetWait(env, owner, settingsPageWelcome, settingsEditPartText)
	calls := settingsEditCountingWrite(t, errors.New("the database is down"))
	before := settingsEditSentCount(env, owner)

	sendPrivateMessage(env, owner, gotgbot.Message{Text: "a welcome that cannot be stored"})

	if got := calls.Load(); got != 1 {
		t.Fatalf("the write was tried %d times, want once", got)
	}
	settingsEditWantReply(t, env, owner, before, "common_settings_save_failed", "settings_edit_still_waiting")
	settingsEditWaitKept(t, env, owner)
	if row, ok := settingsEditStored(t, env.chat.Id); ok {
		t.Errorf("a failed write stored %+v", row)
	}

	// With the write back, the same text is saved.
	greetingWrites.WelcomeTextOnly = greetings.SetWelcomeTextOnly
	sendPrivateMessage(env, owner, gotgbot.Message{Text: "a welcome that cannot be stored"})
	if row, ok := settingsEditStored(t, env.chat.Id); !ok || row.WelcomeSettings.WelcomeText != "a welcome that cannot be stored" {
		t.Errorf("the welcome is %+v (row %v), want the retried text", row.WelcomeSettings, ok)
	}
	settingsEditWaitGone(t, env, owner)
}

func TestSettingsEditWaitAcrossReplicas(t *testing.T) {
	env := newSettingsEditEnv(t)
	owner := env.newOwner(env.chat)
	promptID := settingsEditSetWait(env, owner, settingsPageWelcome, settingsEditPartText)

	replica := env.otherReplica()
	replica.fake.setMember(env.chat.Id, owner.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusCreator})
	replica.fake.setCreator(env.chat.Id, owner.Id)
	baseSent := settingsEditSentCount(env, owner)

	sendPrivateMessage(replica, owner, gotgbot.Message{Text: "typed on the other replica"})

	if row, ok := settingsEditStored(t, env.chat.Id); !ok || row.WelcomeSettings.WelcomeText != "typed on the other replica" {
		t.Fatalf("the welcome is %+v (row %v), want the text typed on the other replica", row.WelcomeSettings, ok)
	}
	pages := replica.fake.sentTo(owner.Id)
	if len(pages) != 1 || !strings.Contains(fmt.Sprint(pages[0].Params["text"]), staffMarker("settings_edit_saved_text")) {
		t.Errorf("the second replica sent %+v, want the one saved page", pages)
	}
	if got := settingsEditSentCount(env, owner); got != baseSent {
		t.Errorf("the first replica sent %d more messages, want none", got-baseSent)
	}
	if len(replica.edits(owner.Id, promptID)) != 1 {
		t.Errorf("the second replica edited the prompt %d times, want once", len(replica.edits(owner.Id, promptID)))
	}
	settingsEditWaitGone(t, env, owner)
}

func TestSettingsEditWaitExpiry(t *testing.T) {
	env := newSettingsEditEnv(t)
	owner := env.newOwner(env.chat)
	settingsEditSetWait(env, owner, settingsPageWelcome, settingsEditPartText)
	env.redis.FastForward(settingsInputTTL + time.Second)
	if env.redis.Exists(settingsInputKey(owner.Id)) || !env.redis.Exists(settingsInputDoneKey(owner.Id)) {
		t.Fatalf("after ten minutes the keys are %v, want only the marker", settingsEditInputKeys(env))
	}
	before := settingsEditSentCount(env, owner)

	sendPrivateMessage(env, owner, gotgbot.Message{Text: "too late"})

	settingsEditWantReply(t, env, owner, before, "settings_edit_wait_ended_note", "settings_edit_part_text")
	if row, ok := settingsEditStored(t, env.chat.Id); ok {
		t.Errorf("a late reply stored %+v", row)
	}
	settingsEditWaitGone(t, env, owner)

	// The second late message is an ordinary private message.
	before = settingsEditSentCount(env, owner)
	sendPrivateMessage(env, owner, gotgbot.Message{Text: "still too late"})
	if texts := settingsEditTextsSince(env, owner, before); len(texts) != 0 {
		t.Errorf("a second late message was answered: %q", texts)
	}
}

func TestSettingsEditMarkerReplyOnce(t *testing.T) {
	expire := func(env *settingsEnv) { env.redis.FastForward(settingsInputTTL + time.Second) }

	t.Run("two deliveries answer once", func(t *testing.T) {
		env := newSettingsEditEnv(t)
		owner := env.newOwner(env.chat)
		settingsEditSetWait(env, owner, settingsPageWelcome, settingsEditPartText)
		expire(env)
		before := settingsEditSentCount(env, owner)

		update := privateUpdate(env, owner, gotgbot.Message{Text: "late"})
		var wg sync.WaitGroup
		for i := 0; i < 6; i++ {
			copied := *update
			message := *update.Message
			copied.Message = &message
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := env.dispatcher.ProcessUpdate(env.bot, &copied, nil); err != nil {
					t.Errorf("ProcessUpdate error = %v", err)
				}
			}()
		}
		wg.Wait()

		settingsEditWantReply(t, env, owner, before, "settings_edit_wait_ended_note")
	})

	t.Run("after a save an ordinary message is not answered", func(t *testing.T) {
		env := newSettingsEditEnv(t)
		owner := env.newOwner(env.chat)
		settingsEditSetWait(env, owner, settingsPageWelcome, settingsEditPartText)
		sendPrivateMessage(env, owner, gotgbot.Message{Text: "saved text"})
		before := settingsEditSentCount(env, owner)

		sendPrivateMessage(env, owner, gotgbot.Message{Text: "just chatting"})

		if texts := settingsEditTextsSince(env, owner, before); len(texts) != 0 {
			t.Errorf("an ordinary message after a save was answered: %q", texts)
		}
	})

	t.Run("after the Cancel button an ordinary message is not answered", func(t *testing.T) {
		env := newSettingsEditEnv(t)
		owner := env.newOwner(env.chat)
		menuID := settingsEditSetWait(env, owner, settingsPageWelcome, settingsEditPartText)
		settingsEditPressButton(env, owner, menuID, "settings_btn_edit_cancel")
		before := settingsEditSentCount(env, owner)

		sendPrivateMessage(env, owner, gotgbot.Message{Text: "just chatting"})

		if texts := settingsEditTextsSince(env, owner, before); len(texts) != 0 {
			t.Errorf("an ordinary message after Cancel was answered: %q", texts)
		}
	})

	t.Run("after /cancel an ordinary message is not answered", func(t *testing.T) {
		env := newSettingsEditEnv(t)
		owner := env.newOwner(env.chat)
		settingsEditSetWait(env, owner, settingsPageWelcome, settingsEditPartText)
		before := settingsEditSentCount(env, owner)
		sendPrivateMessage(env, owner, gotgbot.Message{
			Text: "/cancel", Entities: []gotgbot.MessageEntity{{Type: "bot_command", Offset: 0, Length: 7}},
		})
		settingsEditWantReply(t, env, owner, before, "settings_edit_cancelled")
		before = settingsEditSentCount(env, owner)

		sendPrivateMessage(env, owner, gotgbot.Message{Text: "just chatting"})

		if texts := settingsEditTextsSince(env, owner, before); len(texts) != 0 {
			t.Errorf("an ordinary message after /cancel was answered: %q", texts)
		}
	})
}

func TestSettingsEditDoubleDelivery(t *testing.T) {
	env := newSettingsEditEnv(t)
	owner := env.newOwner(env.chat)
	settingsEditSetWait(env, owner, settingsPageWelcome, settingsEditPartText)
	calls := settingsEditCountingWrite(t, nil)
	before := settingsEditSentCount(env, owner)

	update := privateUpdate(env, owner, gotgbot.Message{Text: "delivered twice"})
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		copied := *update
		message := *update.Message
		copied.Message = &message
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := env.dispatcher.ProcessUpdate(env.bot, &copied, nil); err != nil {
				t.Errorf("ProcessUpdate error = %v", err)
			}
		}()
	}
	wg.Wait()

	if got := calls.Load(); got != 1 {
		t.Errorf("the write ran %d times, want once", got)
	}
	settingsEditWantReply(t, env, owner, before, "settings_edit_saved_text")
	if row, ok := settingsEditStored(t, env.chat.Id); !ok || row.WelcomeSettings.WelcomeText != "delivered twice" {
		t.Errorf("the welcome is %+v (row %v), want the delivered text", row.WelcomeSettings, ok)
	}
}

func TestSettingsEditDoorLogsNoText(t *testing.T) {
	hook := logrustest.NewGlobal()
	t.Cleanup(hook.Reset)
	env := newSettingsEditEnv(t)
	owner := env.newOwner(env.chat)
	settingsEditSetWait(env, owner, settingsPageWelcome, settingsEditPartText)

	const (
		sampleText = "SECRETSAMPLETEXT"
		sampleLink = "secret.example/private-link"
		sampleName = "SecretButtonName"
		sampleFile = "SECRETFILEID"
	)
	// A refusal: a photo (its file ID) and a text with a button line (a name and a link).
	sendPrivateMessage(env, owner, gotgbot.Message{Photo: []gotgbot.PhotoSize{{FileId: sampleFile, FileUniqueId: sampleFile + "U", Width: 1, Height: 1}}})
	sendPrivateMessage(env, owner, gotgbot.Message{Text: sampleText + " [" + sampleName + "](buttonurl://https://" + sampleLink + ")"})
	// A failed write, then a save.
	greetingWrites.WelcomeTextOnly = func(int64, string) error { return errors.New("the database is down") }
	t.Cleanup(func() { greetingWrites.WelcomeTextOnly = greetings.SetWelcomeTextOnly })
	sendPrivateMessage(env, owner, gotgbot.Message{Text: sampleText + " https://" + sampleLink})
	greetingWrites.WelcomeTextOnly = greetings.SetWelcomeTextOnly
	sendPrivateMessage(env, owner, gotgbot.Message{Text: sampleText + " https://" + sampleLink})
	if row, ok := settingsEditStored(t, env.chat.Id); !ok || !strings.Contains(row.WelcomeSettings.WelcomeText, sampleText) {
		t.Fatalf("the full save did not happen: %+v (row %v)", row.WelcomeSettings, ok)
	}

	for _, entry := range hook.AllEntries() {
		line := entry.Message + " " + fmt.Sprint(entry.Data)
		for _, secret := range []string{sampleText, sampleLink, sampleName, sampleFile} {
			if strings.Contains(line, secret) {
				t.Errorf("a log line carries %q: %s", secret, line)
			}
		}
	}
}
