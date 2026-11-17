//go:build testtools

package modules

import (
	"fmt"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/uasneppy/fuku_robot/fuku/db/federations"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

type setFedLogFlowEnv struct {
	t          *testing.T
	client     *moduleBotClient
	bot        *gotgbot.Bot
	dispatcher *ext.Dispatcher
	owner      gotgbot.User
	fedID      string
	channel    gotgbot.Chat
	group      gotgbot.Chat
	pm         gotgbot.Chat
}

func newSetFedLogFlowEnv(t *testing.T) *setFedLogFlowEnv {
	t.Helper()
	client := newModuleBotClient()
	owner := gotgbot.User{Id: uniquePositiveUserID(), FirstName: "Owner"}
	env := &setFedLogFlowEnv{
		t:          t,
		client:     client,
		bot:        newModuleTestBot(client),
		dispatcher: ext.NewDispatcher(&ext.DispatcherOpts{MaxRoutines: -1}),
		owner:      owner,
		fedID:      newFlowFed(t, owner.Id),
		channel:    gotgbot.Chat{Id: uniqueModuleChatID(), Type: "channel", Title: "Fed Log Channel"},
		group:      gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Fed Flow Group"},
		pm:         gotgbot.Chat{Id: owner.Id, Type: "private", FirstName: "Owner"},
	}
	LoadFederations(env.dispatcher)
	return env
}

func newFlowFed(t *testing.T, ownerID int64) string {
	t.Helper()
	fed, err := federations.CreateFederation(ownerID, fmt.Sprintf("Flow Fed %d", ownerID))
	if err != nil {
		t.Fatalf("CreateFederation: %v", err)
	}
	t.Cleanup(func() { _ = federations.DeleteFederation(fed.FedID) })
	return fed.FedID
}

// makeGroupAdmin lists user as an administrator of the env's group in the admin list the bot
// loads.
func (e *setFedLogFlowEnv) makeGroupAdmin(user gotgbot.User) {
	e.client.mu.Lock()
	defer e.client.mu.Unlock()
	e.client.responses["getChatAdministrators"] = []byte(fmt.Sprintf(
		`[{"status":"administrator","user":{"id":999,"is_bot":true,"first_name":"Fuku"},"can_restrict_members":true},`+
			`{"status":"administrator","user":{"id":%d,"is_bot":false,"first_name":"Owner"},"can_restrict_members":true}]`,
		user.Id))
}

func (e *setFedLogFlowEnv) process(u *gotgbot.Update) {
	e.t.Helper()
	if err := e.dispatcher.ProcessUpdate(e.bot, u, nil); err != nil {
		e.t.Fatalf("ProcessUpdate(%d): %v", u.UpdateId, err)
	}
}

func (e *setFedLogFlowEnv) logChat() int64 {
	e.t.Helper()
	fed := federations.GetFed(e.fedID)
	if fed == nil {
		e.t.Fatalf("federation %s is gone", e.fedID)
	}
	return fed.LogChatID
}

func (e *setFedLogFlowEnv) resetLog() {
	e.t.Helper()
	if err := federations.SetFedLogChat(e.fedID, 0); err != nil {
		e.t.Fatalf("SetFedLogChat(0): %v", err)
	}
}

func (e *setFedLogFlowEnv) wantLast(chatID int64, key string) {
	e.t.Helper()
	if got, want := flowLastText(e.client, chatID), ukText(e.t, key); got != want {
		e.t.Fatalf("last reply in %d = %q, want %s %q", chatID, got, key, want)
	}
}

// TestSetFedLogFlowThroughDispatcher runs the two steps of the federation log channel flow
// through the real dispatcher: a channel post /setfedlog <fed id>, then its forward by the
// federation owner. It also holds the typed /setfedlog of a group (item 2b of the audit batch:
// the sender must be an administrator of that group).
func TestSetFedLogFlowThroughDispatcher(t *testing.T) {
	for _, store := range []string{"memory", "miniredis"} {
		t.Run(store, func(t *testing.T) {
			withRealLocales(t)
			if store == "miniredis" {
				withMiniredis(t)
			}

			t.Run("post alone binds nothing, owner forward in private binds the channel", func(t *testing.T) {
				env := newSetFedLogFlowEnv(t)
				post := channelPostUpdate(env.channel, "/setfedlog "+env.fedID)
				env.process(post)
				if got := env.logChat(); got != 0 {
					t.Fatalf("after the post alone the fed log is %d", got)
				}
				env.wantLast(env.channel.Id, "feds_setfedlog_forward_to_owner")
				env.process(forwardedPostUpdate(env.pm, env.owner, post, post.ChannelPost.MessageId))
				if got := env.logChat(); got != env.channel.Id {
					t.Fatalf("fed log = %d, want the channel %d", got, env.channel.Id)
				}
				env.wantLast(env.pm.Id, "feds_setfedlog_ok")
			})

			t.Run("owner forward in a group binds the channel, not the group", func(t *testing.T) {
				env := newSetFedLogFlowEnv(t)
				post := channelPostUpdate(env.channel, "/setfedlog "+env.fedID)
				env.process(post)
				env.process(forwardedPostUpdate(env.group, env.owner, post, post.ChannelPost.MessageId))
				if got := env.logChat(); got != env.channel.Id {
					t.Fatalf("fed log = %d, want the channel %d (group %d)", got, env.channel.Id, env.group.Id)
				}
				env.wantLast(env.group.Id, "feds_setfedlog_ok")
			})

			t.Run("stranger forward binds nothing and keeps the marker", func(t *testing.T) {
				env := newSetFedLogFlowEnv(t)
				post := channelPostUpdate(env.channel, "/setfedlog "+env.fedID)
				env.process(post)
				stranger := gotgbot.User{Id: uniquePositiveUserID(), FirstName: "Stranger"}
				strangerPM := gotgbot.Chat{Id: stranger.Id, Type: "private", FirstName: "Stranger"}
				env.process(forwardedPostUpdate(strangerPM, stranger, post, post.ChannelPost.MessageId))
				if got := env.logChat(); got != 0 {
					t.Fatalf("a stranger's forward set the fed log to %d", got)
				}
				env.wantLast(strangerPM.Id, "feds_not_owner")
				env.process(forwardedPostUpdate(env.pm, env.owner, post, post.ChannelPost.MessageId))
				if got := env.logChat(); got != env.channel.Id {
					t.Fatalf("the owner's forward after the stranger's set %d, want %d", got, env.channel.Id)
				}
			})

			t.Run("unmarked forward and replay bind nothing", func(t *testing.T) {
				env := newSetFedLogFlowEnv(t)
				post := channelPostUpdate(env.channel, "/setfedlog "+env.fedID)
				env.process(post)
				env.process(forwardedPostUpdate(env.pm, env.owner, post, post.ChannelPost.MessageId+99))
				if got := env.logChat(); got != 0 {
					t.Fatalf("a forward of an unmarked post set %d", got)
				}
				env.wantLast(env.pm.Id, "feds_setfedlog_no_pending")
				env.process(forwardedPostUpdate(env.pm, env.owner, post, post.ChannelPost.MessageId))
				if got := env.logChat(); got != env.channel.Id {
					t.Fatalf("the marked forward set %d, want %d", got, env.channel.Id)
				}
				env.resetLog()
				env.process(forwardedPostUpdate(env.pm, env.owner, post, post.ChannelPost.MessageId))
				if got := env.logChat(); got != 0 {
					t.Fatalf("a replayed forward set %d", got)
				}
				env.wantLast(env.pm.Id, "feds_setfedlog_no_pending")
			})

			t.Run("marker of another owner's federation binds nothing", func(t *testing.T) {
				env := newSetFedLogFlowEnv(t)
				otherFed := newFlowFed(t, uniquePositiveUserID())
				post := channelPostUpdate(env.channel, "/setfedlog "+otherFed)
				env.process(post)
				env.process(forwardedPostUpdate(env.pm, env.owner, post, post.ChannelPost.MessageId))
				if got := env.logChat(); got != 0 {
					t.Fatalf("the owner's own fed log changed to %d", got)
				}
				if got := federations.GetFed(otherFed).LogChatID; got != 0 {
					t.Fatalf("another owner's fed log changed to %d", got)
				}
				env.wantLast(env.pm.Id, "feds_not_owner")
			})

			t.Run("post without a valid fed id stores no marker", func(t *testing.T) {
				env := newSetFedLogFlowEnv(t)
				for _, text := range []string{"/setfedlog", "/setfedlog not-a-fed-id"} {
					post := channelPostUpdate(env.channel, text)
					env.process(post)
					env.wantLast(env.channel.Id, "feds_setfedlog_need_id")
					env.process(forwardedPostUpdate(env.pm, env.owner, post, post.ChannelPost.MessageId))
					env.wantLast(env.pm.Id, "feds_setfedlog_no_pending")
				}
				if got := env.logChat(); got != 0 {
					t.Fatalf("fed log = %d, want 0", got)
				}
			})

			t.Run("typed setfedlog by an admin owner sets the group", func(t *testing.T) {
				env := newSetFedLogFlowEnv(t)
				env.makeGroupAdmin(env.owner)
				env.process(textUpdate(env.group, env.owner, "/setfedlog"))
				if got := env.logChat(); got != env.group.Id {
					t.Fatalf("fed log = %d, want the group %d", got, env.group.Id)
				}
				env.resetLog()
				// A forward whose origin is a user is not the channel flow.
				fwd := textUpdate(env.group, env.owner, "/setfedlog")
				fwd.Message.ForwardOrigin = gotgbot.MessageOriginUser{Date: 1, SenderUser: env.owner}
				env.process(fwd)
				if got := env.logChat(); got != env.group.Id {
					t.Fatalf("after a user-origin forward the fed log = %d, want the group %d", got, env.group.Id)
				}
			})

			t.Run("typed setfedlog by an owner who is a plain member is refused", func(t *testing.T) {
				env := newSetFedLogFlowEnv(t)
				env.process(textUpdate(env.group, env.owner, "/setfedlog"))
				if got := env.logChat(); got != 0 {
					t.Fatalf("a plain member set the fed log to %d", got)
				}
				env.wantLast(env.group.Id, "chat_status_user_admin_cmd_error")
			})
		})
	}
}

// TestSetFedLogLocaleKeys holds the two reply strings of the channel flow and the corrected help
// text in every language.
func TestSetFedLogLocaleKeys(t *testing.T) {
	withRealLocales(t)
	for _, code := range []string{"uk", "en", "es", "fr", "hi", "id", "pt", "ru"} {
		tr := i18n.MustNewTranslator(code)
		for _, key := range []string{"feds_setfedlog_forward_to_owner", "feds_setfedlog_no_pending"} {
			if text, err := tr.GetString(key); err != nil || strings.TrimSpace(text) == "" {
				t.Errorf("%s: %s = %q, %v", code, key, text, err)
			}
		}
		help, err := tr.GetString("federations_help_msg")
		if err != nil {
			t.Fatalf("%s: federations_help_msg: %v", code, err)
		}
		if !strings.Contains(help, "/setfedlog") || !strings.Contains(help, "/unsetfedlog") {
			t.Errorf("%s: help does not name /setfedlog and /unsetfedlog", code)
		}
		if strings.Contains(help, "/setfedlog /unsetfedlog") {
			t.Errorf("%s: help still lists /setfedlog with the private-chat commands", code)
		}
	}
}
