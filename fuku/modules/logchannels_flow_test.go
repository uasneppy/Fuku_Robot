//go:build testtools

package modules

import (
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/chats"
	"github.com/uasneppy/fuku_robot/fuku/db/logchannels"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// flowUpdateIDs hands out update and message IDs for the dispatcher flow tests, far from the
// IDs other tests of the package use.
var flowUpdateIDs atomic.Int64

func init() { flowUpdateIDs.Store(910000) }

func nextFlowID() int64 { return flowUpdateIDs.Add(1) }

// commandEntity is the bot_command entity of a text that starts with the command word.
func commandEntity(text string) []gotgbot.MessageEntity {
	n := len(text)
	for i, r := range text {
		if r == ' ' {
			n = i
			break
		}
	}
	return []gotgbot.MessageEntity{{Type: gotgbot.MessageEntityTypeBotCommand, Offset: 0, Length: int64(n)}}
}

// channelPostUpdate is a post in a channel as Telegram delivers it: no from, the channel as
// sender_chat, the command entity at offset 0.
func channelPostUpdate(channel gotgbot.Chat, text string) *gotgbot.Update {
	id := nextFlowID()
	sender := channel
	return &gotgbot.Update{UpdateId: id, ChannelPost: &gotgbot.Message{
		MessageId:  id,
		Date:       1,
		Chat:       channel,
		SenderChat: &sender,
		Text:       text,
		Entities:   commandEntity(text),
	}}
}

// forwardedPostUpdate is the channel post forwarded by from into chat: it keeps the post's text
// and entities and names the channel and the post's message ID as its origin.
func forwardedPostUpdate(chat gotgbot.Chat, from gotgbot.User, post *gotgbot.Update, originMessageID int64) *gotgbot.Update {
	id := nextFlowID()
	text := post.ChannelPost.Text
	return &gotgbot.Update{UpdateId: id, Message: &gotgbot.Message{
		MessageId:     id,
		Date:          1,
		Chat:          chat,
		From:          &from,
		Text:          text,
		Entities:      commandEntity(text),
		ForwardOrigin: gotgbot.MessageOriginChannel{Date: 1, Chat: post.ChannelPost.Chat, MessageId: originMessageID},
	}}
}

// textUpdate is a typed message.
func textUpdate(chat gotgbot.Chat, from gotgbot.User, text string) *gotgbot.Update {
	id := nextFlowID()
	return &gotgbot.Update{UpdateId: id, Message: &gotgbot.Message{
		MessageId: id,
		Date:      1,
		Chat:      chat,
		From:      &from,
		Text:      text,
		Entities:  commandEntity(text),
	}}
}

// flowSentTexts is every text the bot sent to chatID, in order.
func flowSentTexts(client *moduleBotClient, chatID int64) []string {
	var out []string
	for _, call := range client.callsFor("sendMessage") {
		if fmt.Sprint(call.Params["chat_id"]) == fmt.Sprint(chatID) {
			out = append(out, fmt.Sprint(call.Params["text"]))
		}
	}
	return out
}

func flowLastText(client *moduleBotClient, chatID int64) string {
	texts := flowSentTexts(client, chatID)
	if len(texts) == 0 {
		return ""
	}
	return texts[len(texts)-1]
}

func ukText(t *testing.T, key string) string {
	t.Helper()
	text, err := i18n.MustNewTranslator("uk").GetString(key)
	if err != nil || text == "" {
		t.Fatalf("uk text of %s: %q, %v", key, text, err)
	}
	return text
}

type setLogFlowEnv struct {
	t          *testing.T
	client     *moduleBotClient
	bot        *gotgbot.Bot
	dispatcher *ext.Dispatcher
	group      gotgbot.Chat
	channel    gotgbot.Chat
	admin      gotgbot.User
	member     gotgbot.User
}

func newSetLogFlowEnv(t *testing.T) *setLogFlowEnv {
	t.Helper()
	client := newModuleBotClient()
	env := &setLogFlowEnv{
		t:          t,
		client:     client,
		bot:        newModuleTestBot(client),
		dispatcher: ext.NewDispatcher(&ext.DispatcherOpts{MaxRoutines: -1}),
		group:      gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Flow Group"},
		channel:    gotgbot.Chat{Id: uniqueModuleChatID(), Type: "channel", Title: "Flow Channel"},
		admin:      gotgbot.User{Id: 777000, FirstName: "Telegram"},
		member:     gotgbot.User{Id: 42, FirstName: "Member"},
	}
	LoadLogChannels(env.dispatcher)
	if err := chats.EnsureChatInDb(env.group.Id, env.group.Title); err != nil {
		t.Fatalf("EnsureChatInDb(group): %v", err)
	}
	if err := chats.EnsureChatInDb(env.channel.Id, env.channel.Title); err != nil {
		t.Fatalf("EnsureChatInDb(channel): %v", err)
	}
	t.Cleanup(func() {
		_ = logchannels.Unset(env.group.Id)
		cache.DeleteCache(cache.CacheKey("log_channel", env.group.Id))
	})
	return env
}

func (e *setLogFlowEnv) process(u *gotgbot.Update) {
	e.t.Helper()
	if err := e.dispatcher.ProcessUpdate(e.bot, u, nil); err != nil {
		e.t.Fatalf("ProcessUpdate(%d): %v", u.UpdateId, err)
	}
}

func (e *setLogFlowEnv) boundChannel() int64 {
	row := logchannels.Get(e.group.Id)
	if row == nil {
		return 0
	}
	return row.LogChannelID
}

// TestSetLogFlowThroughDispatcher runs the documented two steps of /setlog through the real
// dispatcher: a channel post, then its forward to the group by a live administrator. It runs on
// the in-memory marker store and on miniredis (the production GETDEL path).
func TestSetLogFlowThroughDispatcher(t *testing.T) {
	for _, store := range []string{"memory", "miniredis"} {
		t.Run(store, func(t *testing.T) {
			withRealLocales(t)
			if store == "miniredis" {
				withMiniredis(t)
			}

			t.Run("channel post then admin forward binds", func(t *testing.T) {
				env := newSetLogFlowEnv(t)
				post := channelPostUpdate(env.channel, "/setlog")
				env.process(post)
				if got := env.boundChannel(); got != 0 {
					t.Fatalf("after the post alone the group has log channel %d", got)
				}
				if got, want := flowLastText(env.client, env.channel.Id), ukText(t, "logs_forward_to_group"); got != want {
					t.Fatalf("channel reply = %q, want %q", got, want)
				}
				env.process(forwardedPostUpdate(env.group, env.admin, post, post.ChannelPost.MessageId))
				if got := env.boundChannel(); got != env.channel.Id {
					t.Fatalf("after the forward the log channel = %d, want %d", got, env.channel.Id)
				}
				texts := flowSentTexts(env.client, env.group.Id)
				if len(texts) != 1 || texts[0] != ukText(t, "logs_set_ok") {
					t.Fatalf("group replies = %q, want exactly logs_set_ok", texts)
				}
			})

			t.Run("member forward binds nothing and keeps the marker", func(t *testing.T) {
				env := newSetLogFlowEnv(t)
				post := channelPostUpdate(env.channel, "/setlog")
				env.process(post)
				env.process(forwardedPostUpdate(env.group, env.member, post, post.ChannelPost.MessageId))
				if got := env.boundChannel(); got != 0 {
					t.Fatalf("a member's forward bound %d", got)
				}
				env.process(forwardedPostUpdate(env.group, env.admin, post, post.ChannelPost.MessageId))
				if got := env.boundChannel(); got != env.channel.Id {
					t.Fatalf("the admin's forward after the member's bound %d, want %d", got, env.channel.Id)
				}
			})

			t.Run("unmarked forward and replay bind nothing", func(t *testing.T) {
				env := newSetLogFlowEnv(t)
				post := channelPostUpdate(env.channel, "/setlog")
				env.process(post)
				env.process(forwardedPostUpdate(env.group, env.admin, post, post.ChannelPost.MessageId+99))
				if got := env.boundChannel(); got != 0 {
					t.Fatalf("a forward of an unmarked post bound %d", got)
				}
				env.process(forwardedPostUpdate(env.group, env.admin, post, post.ChannelPost.MessageId))
				if got := env.boundChannel(); got != env.channel.Id {
					t.Fatalf("the marked forward bound %d, want %d", got, env.channel.Id)
				}
				if err := logchannels.Unset(env.group.Id); err != nil {
					t.Fatalf("Unset: %v", err)
				}
				env.process(forwardedPostUpdate(env.group, env.admin, post, post.ChannelPost.MessageId))
				if got := env.boundChannel(); got != 0 {
					t.Fatalf("a replayed forward bound %d", got)
				}
				if got, want := flowLastText(env.client, env.group.Id), ukText(t, "logs_need_channel"); got != want {
					t.Fatalf("replay reply = %q, want %q", got, want)
				}
			})

			t.Run("typed setlog in the group answers need channel", func(t *testing.T) {
				env := newSetLogFlowEnv(t)
				env.process(textUpdate(env.group, env.admin, "/setlog"))
				if got := env.boundChannel(); got != 0 {
					t.Fatalf("a typed /setlog bound %d", got)
				}
				if got, want := flowLastText(env.client, env.group.Id), ukText(t, "logs_need_channel"); got != want {
					t.Fatalf("reply = %q, want %q", got, want)
				}
			})

			t.Run("no marshaler fails closed", func(t *testing.T) {
				env := newSetLogFlowEnv(t)
				withNilCacheMarshal(t)
				post := channelPostUpdate(env.channel, "/setlog")
				env.process(post)
				if got, want := flowLastText(env.client, env.channel.Id), ukText(t, "common_settings_save_failed"); got != want {
					t.Fatalf("channel reply = %q, want %q", got, want)
				}
				env.process(forwardedPostUpdate(env.group, env.admin, post, post.ChannelPost.MessageId))
				if got := env.boundChannel(); got != 0 {
					t.Fatalf("a forward of a post whose marker was never stored bound %d", got)
				}
			})
		})
	}
}
