//go:build testtools

package modules

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/connections"
	"github.com/uasneppy/fuku_robot/fuku/db/greetings"
	"github.com/uasneppy/fuku_robot/fuku/db/locks"
	"github.com/uasneppy/fuku_robot/fuku/db/notes"
	"github.com/uasneppy/fuku_robot/fuku/db/warns"
)

// pmConnClient is the module fake with per-chat administrators: in group it lists the bot and
// admins, and like Telegram it refuses getChatAdministrators for any other chat (a private chat
// has none). getChat answers group.
type pmConnClient struct {
	*moduleBotClient
	group  int64
	admins []int64
}

func (c *pmConnClient) RequestWithContext(ctx context.Context, token, method string, params map[string]any, opts *gotgbot.RequestOpts) (json.RawMessage, error) {
	switch method {
	case "getChatAdministrators":
		if staffParamInt(params, "chat_id") != c.group {
			c.record(method, params)
			return nil, &gotgbot.TelegramError{Code: 400, Description: "Bad Request: there are no administrators in the private chat"}
		}
		entries := []string{`{"status":"administrator","user":{"id":999,"is_bot":true,"first_name":"Fuku"},"can_delete_messages":true,"can_restrict_members":true,"can_change_info":true,"can_invite_users":true}`}
		for _, id := range c.admins {
			entries = append(entries, fmt.Sprintf(`{"status":"administrator","user":{"id":%d,"is_bot":false,"first_name":"Admin"},"can_change_info":true,"can_restrict_members":true,"can_delete_messages":true}`, id))
		}
		c.record(method, params)
		return json.RawMessage("[" + strings.Join(entries, ",") + "]"), nil
	case "getChat":
		c.record(method, params)
		return json.RawMessage(fmt.Sprintf(`{"id":%d,"type":"supergroup","title":"Connected Group"}`, c.group)), nil
	}
	return c.moduleBotClient.RequestWithContext(ctx, token, method, params, opts)
}

func (c *pmConnClient) record(method string, params map[string]any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, moduleBotCall{Method: method, Params: params})
}

// pmConnEnv is a group with one real administrator (never 777000 or 1087968824, which
// tgAdminList makes an admin everywhere) and one plain member, both connected to it from
// their private chat with the bot.
type pmConnEnv struct {
	t      *testing.T
	client *pmConnClient
	bot    *gotgbot.Bot
	group  gotgbot.Chat
	admin  gotgbot.User
	member gotgbot.User
}

func newPMConnEnv(t *testing.T) *pmConnEnv {
	t.Helper()
	withStaffLocale(t)
	group := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Connected Group"}
	admin := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Real Admin"}
	member := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Plain Member"}
	client := &pmConnClient{moduleBotClient: newModuleBotClient(), group: group.Id, admins: []int64{admin.Id}}
	bot := newModuleTestBot(client.moduleBotClient)
	bot.BotClient = client
	for _, user := range []gotgbot.User{admin, member} {
		if err := connections.ConnectId(user.Id, group.Id); err != nil {
			t.Fatalf("ConnectId(%d) error = %v", user.Id, err)
		}
		id := user.Id
		t.Cleanup(func() { _ = connections.DisconnectId(id) })
	}
	return &pmConnEnv{t: t, client: client, bot: bot, group: group, admin: admin, member: member}
}

// inPrivate runs handler on text sent by from in their private chat with the bot.
func (e *pmConnEnv) inPrivate(from gotgbot.User, text string, handler func(*gotgbot.Bot, *ext.Context) error) {
	e.t.Helper()
	private := gotgbot.Chat{Id: from.Id, Type: "private", FirstName: from.FirstName}
	if err := handler(e.bot, newModuleMessageContext(e.bot, private, from, text)); err != nil && err != ext.EndGroups && err != ext.ContinueGroups {
		e.t.Fatalf("%q error = %v", text, err)
	}
}

// sentTexts is every text the bot sent, in order.
func (e *pmConnEnv) sentTexts() []string {
	var out []string
	for _, call := range e.client.callsFor("sendMessage") {
		out = append(out, fmt.Sprint(call.Params["text"]))
	}
	return out
}

func (e *pmConnEnv) wantNoAdminRefusal() {
	e.t.Helper()
	for _, text := range e.sentTexts() {
		if strings.Contains(text, staffMarker("chat_status_user_admin_cmd_error")) {
			e.t.Fatalf("the bot sent the admin refusal %q to a real admin", text)
		}
	}
}

func (e *pmConnEnv) wantSentContains(want string) {
	e.t.Helper()
	for _, text := range e.sentTexts() {
		if strings.Contains(text, want) {
			return
		}
	}
	e.t.Fatalf("no sent text contains %q: %q", want, e.sentTexts())
}

func TestConnectedAdminCommandsInPrivateChat(t *testing.T) {
	t.Run("lock and unlock", func(t *testing.T) {
		env := newPMConnEnv(t)
		env.inPrivate(env.admin, "/lock url", locksModule.lockPerm)
		if !locks.IsPermLocked(env.group.Id, "url") {
			t.Fatal("/lock url through the connection stored nothing for the group")
		}
		env.inPrivate(env.admin, "/unlock url", locksModule.unlockPerm)
		if locks.IsPermLocked(env.group.Id, "url") {
			t.Fatal("/unlock url through the connection left the group's lock")
		}
		env.wantNoAdminRefusal()
		env.wantSentContains(staffMarker("locks_unlocked_successfully"))
	})

	t.Run("setwarnmode and setwarnlimit", func(t *testing.T) {
		env := newPMConnEnv(t)
		env.inPrivate(env.admin, "/setwarnmode ban", warnsModule.setWarnMode)
		env.inPrivate(env.admin, "/setwarnlimit 5", warnsModule.setWarnLimit)
		got := warns.GetWarnSetting(env.group.Id)
		if got.WarnMode != "ban" || got.WarnLimit != 5 {
			t.Fatalf("group warn settings = %s/%d, want ban/5", got.WarnMode, got.WarnLimit)
		}
		env.wantNoAdminRefusal()

		// A connected plain member is refused and changes nothing.
		env.inPrivate(env.member, "/setwarnmode kick", warnsModule.setWarnMode)
		if got := warns.GetWarnSetting(env.group.Id); got.WarnMode != "ban" {
			t.Fatalf("a plain member's /setwarnmode stored %q", got.WarnMode)
		}
	})

	t.Run("welcome off", func(t *testing.T) {
		env := newPMConnEnv(t)
		env.inPrivate(env.admin, "/welcome off", greetingsModule.welcome)
		if greetings.GetGreetingSettings(env.group.Id).WelcomeSettings.ShouldWelcome {
			t.Fatal("/welcome off through the connection left the group's welcome on")
		}
		env.wantNoAdminRefusal()
	})

	t.Run("notes lists an admin-only note", func(t *testing.T) {
		env := newPMConnEnv(t)
		if err := notes.AddNote(env.group.Id, "staffonly", "secret text", "", nil, db.TEXT, false, false, true, false, false, false); err != nil {
			t.Fatalf("AddNote() error = %v", err)
		}
		t.Cleanup(func() { _ = notes.RemoveAllNotes(env.group.Id) })
		env.inPrivate(env.admin, "/notes", notesModule.notesList)
		env.wantSentContains("staffonly")
	})

	t.Run("get noformat", func(t *testing.T) {
		env := newPMConnEnv(t)
		if err := notes.AddNote(env.group.Id, "plainnote", "plain note text", "", nil, db.TEXT, false, false, false, false, false, false); err != nil {
			t.Fatalf("AddNote() error = %v", err)
		}
		t.Cleanup(func() { _ = notes.RemoveAllNotes(env.group.Id) })
		env.inPrivate(env.admin, "/get plainnote noformat", notesModule.getNotes)
		env.wantNoAdminRefusal()
		env.wantSentContains("plain note text")

		// A plain member still gets the refusal.
		before := len(env.sentTexts())
		env.inPrivate(env.member, "/get plainnote noformat", notesModule.getNotes)
		texts := env.sentTexts()
		if len(texts) != before+1 || !strings.Contains(texts[len(texts)-1], staffMarker("chat_status_user_admin_cmd_error")) {
			t.Fatalf("a plain member's /get noformat sent %q, want the admin refusal", texts[before:])
		}
	})

	t.Run("in the group itself", func(t *testing.T) {
		env := newPMConnEnv(t)
		ctx := newModuleMessageContext(env.bot, env.group, env.admin, "/setwarnmode kick")
		if err := warnsModule.setWarnMode(env.bot, ctx); err != nil && err != ext.EndGroups {
			t.Fatalf("/setwarnmode in the group error = %v", err)
		}
		if got := warns.GetWarnSetting(env.group.Id); got.WarnMode != "kick" {
			t.Fatalf("group warn mode = %q, want kick", got.WarnMode)
		}
	})
}
