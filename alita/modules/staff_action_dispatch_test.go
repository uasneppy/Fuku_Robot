//go:build testtools

package modules

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/db/staff"
)

// staffDispatcher loads the per-group moderation modules, and the StaffActions
// interceptors when withStaff is set, in registry (priority) order.
func staffDispatcher(t *testing.T, withStaff bool) *ext.Dispatcher {
	t.Helper()
	wanted := map[string]bool{"Bans": true, "Mutes": true, "StaffActions": withStaff}
	var mods []registeredModule
	for _, m := range registry {
		if wanted[m.name] {
			mods = append(mods, m)
		}
	}
	slices.SortStableFunc(mods, func(a, b registeredModule) int { return a.priority - b.priority })
	wantMods := 2
	if withStaff {
		wantMods++
	}
	if len(mods) != wantMods {
		t.Fatalf("found %d modules in the registry, want Bans, Mutes and StaffActions only when asked (%d)", len(mods), wantMods)
	}
	dispatcher := ext.NewDispatcher(&ext.DispatcherOpts{MaxRoutines: -1})
	for _, m := range mods {
		m.load(dispatcher)
	}
	return dispatcher
}

// staffCommandUpdate builds a command message in chat from from; reply, when not
// nil, is the message being replied to.
func staffCommandUpdate(id int64, chat gotgbot.Chat, from gotgbot.User, text string, reply *gotgbot.User) *gotgbot.Update {
	msg := &gotgbot.Message{MessageId: id, Date: 1, Chat: chat, From: &from, Text: text}
	if reply != nil {
		msg.ReplyToMessage = &gotgbot.Message{MessageId: id - 1, Date: 1, Chat: chat, From: reply, Text: "moderated"}
	}
	return &gotgbot.Update{UpdateId: id, Message: msg}
}

// normalizedCalls renders the calls of a client as method, user_id, until_date
// and the reply text, with chatID replaced by a placeholder.
func normalizedCalls(client *moduleBotClient, chatID int64) []string {
	client.mu.Lock()
	defer client.mu.Unlock()
	placeholder := strconv.FormatInt(chatID, 10)
	var out []string
	for _, call := range client.calls {
		text := strings.ReplaceAll(fmt.Sprint(call.Params["text"]), placeholder, "<chat>")
		out = append(out, fmt.Sprintf("%s|user=%v|until=%v|text=%s", call.Method, call.Params["user_id"], call.Params["until_date"], text))
	}
	return out
}

func TestStaffActionNonStaffUnchanged(t *testing.T) {
	withStaffLocale(t)
	admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	target := gotgbot.User{Id: 42, FirstName: "Member"}

	commands := []struct {
		name  string
		text  string
		reply *gotgbot.User
	}{
		{name: "ban an ID", text: "/ban 4242"},
		{name: "ban without an argument", text: "/ban"},
		{name: "ban as a reply", text: "/ban spam", reply: &target},
		{name: "mute an ID", text: "/mute 4242"},
		{name: "kick an ID", text: "/kick 4242"},
		{name: "unban an ID", text: "/unban 4242"},
		{name: "unmute an ID", text: "/unmute 4242"},
		{name: "mute as a reply", text: "/mute spam", reply: &target},
		{name: "tban an ID", text: "/tban 4242 2d"},
		{name: "tban with a reason", text: "/tban 4242 2d spamming"},
		{name: "tban without a duration", text: "/tban 4242"},
		{name: "tban as a reply", text: "/tban 2d spam", reply: &target},
		{name: "tmute an ID", text: "/tmute 4242 1h"},
		{name: "tmute without a duration", text: "/tmute 4242"},
		{name: "tmute as a reply", text: "/tmute 1h spam", reply: &target},
		{name: "ban with a duration", text: "/ban 4242 2d spam"},
		{name: "mute with a duration", text: "/mute 4242 30m"},
	}
	for _, tc := range commands {
		t.Run(tc.name, func(t *testing.T) {
			var runs [2][]string
			for i, withStaff := range []bool{false, true} {
				client := newModuleBotClient()
				bot := newModuleTestBot(client)
				chat := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Plain Chat"}
				dispatcher := staffDispatcher(t, withStaff)
				if err := dispatcher.ProcessUpdate(bot, staffCommandUpdate(900, chat, admin, tc.text, tc.reply), nil); err != nil {
					t.Fatalf("ProcessUpdate (staff modules loaded: %t) error = %v", withStaff, err)
				}
				runs[i] = normalizedCalls(client, chat.Id)
			}
			if len(runs[0]) == 0 {
				t.Fatal("the baseline dispatcher made no Telegram call, so the comparison proves nothing")
			}
			if !slices.Equal(runs[0], runs[1]) {
				t.Fatalf("Telegram calls differ with StaffActions loaded:\nwithout: %q\nwith:    %q", runs[0], runs[1])
			}
		})
	}
}

func TestStaffActionStaffGroupRoutesToCard(t *testing.T) {
	env := newStaffActionEnv(t, 1)
	full := staffDispatcher(t, true)
	plain := gotgbot.User{Id: 555001, FirstName: "Plain"}

	if err := full.ProcessUpdate(env.bot, env.messageUpdate(env.staffChatObj(), &plain, "/ban 4242"), nil); err != nil {
		t.Fatalf("ProcessUpdate error = %v", err)
	}

	env.wantCardOnly()
	if bans := env.fake.callsFor("banChatMember"); len(bans) != 0 {
		t.Fatalf("banChatMember calls = %d, want 0", len(bans))
	}
}

// wantCardOnly expects exactly one message to the Staff Group: the confirm card.
func (e *staffActionEnv) wantCardOnly() {
	e.t.Helper()
	texts := textsToChat(e.fake.staffBotClient, e.staffChat)
	if len(texts) != 1 || !strings.Contains(texts[0], staffMarker("staff_act_card_applies")) {
		e.t.Fatalf("messages to the Staff Group = %q, want exactly the card", texts)
	}
	if len(e.cardActions()) == 0 {
		e.t.Fatal("the card has no buttons")
	}
}

func TestStaffActionAnonymous(t *testing.T) {
	staffChatOf := func(env *staffActionEnv) gotgbot.Chat { return env.staffChatObj() }
	for _, command := range []string{"ban", "mute", "kick", "unban", "unmute", "tban", "tmute"} {
		text := "/" + command + " 4242"
		if command == "tban" || command == "tmute" {
			text += " 2d"
		}
		cases := []struct {
			name  string
			build func(env *staffActionEnv) *gotgbot.Update
		}{
			{
				name: "anonymous admin",
				build: func(env *staffActionEnv) *gotgbot.Update {
					chat := staffChatOf(env)
					from := gotgbot.User{Id: 1087968824, IsBot: true, FirstName: "Group"}
					update := env.messageUpdate(chat, &from, text)
					update.Message.SenderChat = &chat
					return update
				},
			},
			{
				name: "linked channel post",
				build: func(env *staffActionEnv) *gotgbot.Update {
					from := gotgbot.User{Id: 777000, FirstName: "Telegram"}
					update := env.messageUpdate(staffChatOf(env), &from, text)
					update.Message.SenderChat = &gotgbot.Chat{Id: -1009999999999, Type: "channel", Title: "News"}
					update.Message.IsAutomaticForward = true
					return update
				},
			},
			{
				name: "channel identity",
				build: func(env *staffActionEnv) *gotgbot.Update {
					from := gotgbot.User{Id: 136817688, IsBot: true, FirstName: "Channel"}
					update := env.messageUpdate(staffChatOf(env), &from, text)
					update.Message.SenderChat = &gotgbot.Chat{Id: -1008888888888, Type: "channel", Title: "Other"}
					return update
				},
			},
		}
		for _, tc := range cases {
			t.Run(command+" "+tc.name, func(t *testing.T) {
				env := newStaffActionEnv(t, 1)
				env.process(tc.build(env))
				env.wantPostAsYourself()
			})
		}
	}

	t.Run("sender without a user", func(t *testing.T) {
		if len(staffActionCommands) != 7 {
			t.Fatalf("staff commands = %d, want ban, mute, kick, unban, unmute, tban and tmute", len(staffActionCommands))
		}
		for _, spec := range staffActionCommands {
			env := newStaffActionEnv(t, 1)
			ctx := ext.NewContext(env.bot, env.messageUpdate(env.staffChatObj(), &gotgbot.User{Id: 555001}, "/"+spec.Name+" 4242"), nil)
			ctx.EffectiveSender = &gotgbot.Sender{ChatId: env.staffChat}

			if err := staffModule.handleStaffAction(env.bot, ctx, spec); err != ext.EndGroups {
				t.Fatalf("handleStaffAction(%s) error = %v, want ext.EndGroups", spec.Name, err)
			}
			env.wantPostAsYourself()
		}
	})
}

// wantPostAsYourself expects one keyboard-less refusal and no lookup or write.
func (e *staffActionEnv) wantPostAsYourself() {
	e.t.Helper()
	e.wantReplyMarker("staff_post_as_yourself")
	for _, method := range []string{"getChatMember", "getChatAdministrators", "banChatMember", "restrictChatMember", "unbanChatMember"} {
		if calls := e.fake.callsFor(method); len(calls) != 0 {
			e.t.Fatalf("%s calls = %d, want 0 for an anonymous sender", method, len(calls))
		}
	}
}

func TestStaffActionModuleOrder(t *testing.T) {
	priority := map[string]int{}
	for _, m := range registry {
		priority[m.name] = m.priority
	}
	staffActions, ok := priority["StaffActions"]
	if !ok || staffActions != 65 {
		t.Fatalf("StaffActions priority = (%d, %t), want 65", staffActions, ok)
	}
	if staffActions >= priority["Bans"] || staffActions >= priority["Mutes"] {
		t.Fatalf("StaffActions (%d) must sort before Bans (%d) and Mutes (%d)", staffActions, priority["Bans"], priority["Mutes"])
	}
}

func TestStaffActionStaleGatePassesThrough(t *testing.T) {
	env := newStaffActionEnv(t, 1)
	full := staffDispatcher(t, true)
	if staff.GetStaffGroup(env.staffChat) == nil {
		t.Fatal("the cached gate does not know the Staff Group")
	}
	// Remove the row behind the cache's back: the gate now reads stale.
	if err := db.DB.Where("chat_id = ?", env.staffChat).Delete(&models.StaffGroup{}).Error; err != nil {
		t.Fatalf("delete Staff Group row: %v", err)
	}
	if staff.GetStaffGroup(env.staffChat) == nil {
		t.Fatal("the gate is not stale, so the test would prove nothing")
	}
	admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	env.fake.setMember(env.staffChat, admin.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusCreator})

	if err := full.ProcessUpdate(env.bot, staffCommandUpdate(901, env.staffChatObj(), admin, "/ban 4242", nil), nil); err != nil {
		t.Fatalf("ProcessUpdate error = %v", err)
	}

	if bans := env.callsTo("banChatMember", env.staffChat); len(bans) != 1 {
		t.Fatalf("banChatMember calls in the chat = %d, want the per-group /ban to run; calls: %q",
			len(bans), normalizedCalls(env.fake.moduleBotClient, env.staffChat))
	}
	env.wantNoCard()
}

func TestStaffActionCommandNamesExact(t *testing.T) {
	t.Run("banme and unbanall are not intercepted", func(t *testing.T) {
		env := newStaffActionEnv(t, 1)
		full := staffDispatcher(t, true)
		member := gotgbot.User{Id: 555001, FirstName: "Plain"}
		for _, command := range []string{"/banme", "/unbanall"} {
			if err := full.ProcessUpdate(env.bot, env.messageUpdate(env.staffChatObj(), &member, command), nil); err != nil {
				t.Fatalf("ProcessUpdate(%s) error = %v", command, err)
			}
		}
		env.wantNoCard()
		for _, text := range textsToChat(env.fake.staffBotClient, env.staffChat) {
			if strings.Contains(text, staffMarker("staff_act_card_applies")) {
				t.Fatalf("a staff card was sent for banme or unbanall: %q", text)
			}
		}
	})

	t.Run("a command for another bot reaches nobody", func(t *testing.T) {
		env := newStaffActionEnv(t, 1)
		full := staffDispatcher(t, true)
		member := gotgbot.User{Id: 555001, FirstName: "Plain"}
		if err := full.ProcessUpdate(env.bot, env.messageUpdate(env.staffChatObj(), &member, "/ban@OtherBot 4242"), nil); err != nil {
			t.Fatalf("ProcessUpdate error = %v", err)
		}
		if sent := env.fake.callsFor("sendMessage"); len(sent) != 0 {
			t.Fatalf("sendMessage calls = %d, want 0", len(sent))
		}
		if bans := env.fake.callsFor("banChatMember"); len(bans) != 0 {
			t.Fatalf("banChatMember calls = %d, want 0", len(bans))
		}
	})
}
