//go:build testtools

package modules

import (
	"fmt"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/uasneppy/fuku_robot/fuku/db/approvals"
)

// automaticForwardUpdate is a post of channel as Telegram forwards it automatically into the
// channel's discussion group: from the Telegram service account (777000), the channel as
// sender_chat, is_automatic_forward set and the post as its channel origin, with the post's text.
func automaticForwardUpdate(group, channel gotgbot.Chat, text string, originMessageID int64) *gotgbot.Update {
	id := nextFlowID()
	sender := channel
	return &gotgbot.Update{UpdateId: id, Message: &gotgbot.Message{
		MessageId:          id,
		Date:               1,
		Chat:               group,
		From:               &gotgbot.User{Id: 777000, FirstName: "Telegram"},
		SenderChat:         &sender,
		IsAutomaticForward: true,
		Text:               text,
		Entities:           commandEntity(text),
		ForwardOrigin:      gotgbot.MessageOriginChannel{Date: 1, Chat: channel, MessageId: originMessageID},
	}}
}

// TestAutomaticForwardRunsNoCommand holds that a post of a linked channel forwarded automatically
// into its discussion group never runs a command, however it is worded: the Telegram service
// account that Telegram puts in its from field passes the admin checks, so the command handler
// built by helpers.NewBotCommand passes it on with no reply and no write.
func TestAutomaticForwardRunsNoCommand(t *testing.T) {
	withRealLocales(t)

	t.Run("approve", func(t *testing.T) {
		client := newModuleBotClient()
		bot := newModuleTestBot(client)
		dispatcher := ext.NewDispatcher(&ext.DispatcherOpts{MaxRoutines: -1})
		LoadApprovals(dispatcher)
		group := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Forward Group"}
		channel := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "channel", Title: "Forward Channel"}
		target := uniqueLinkOwnerID()
		t.Cleanup(func() { _ = approvals.RemoveApprovedUser(group.Id, target) })

		update := automaticForwardUpdate(group, channel, fmt.Sprintf("/approve %d", target), 7)
		if err := dispatcher.ProcessUpdate(bot, update, nil); err != nil {
			t.Fatalf("ProcessUpdate: %v", err)
		}
		if approvals.IsUserApproved(group.Id, target) {
			t.Fatal("an automatic forward approved a user")
		}
		if texts := flowSentTexts(client, group.Id); len(texts) != 0 {
			t.Fatalf("an automatic forward got replies %q", texts)
		}
	})

	for _, store := range []string{"memory", "miniredis"} {
		t.Run("setlog "+store, func(t *testing.T) {
			if store == "miniredis" {
				withMiniredis(t)
			}
			env := newSetLogFlowEnv(t)
			post := channelPostUpdate(env.channel, "/setlog")
			env.process(post)
			env.process(automaticForwardUpdate(env.group, env.channel, "/setlog", post.ChannelPost.MessageId))
			if got := env.boundChannel(); got != 0 {
				t.Fatalf("an automatic forward bound log channel %d", got)
			}
			if texts := flowSentTexts(env.client, env.group.Id); len(texts) != 0 {
				t.Fatalf("an automatic forward got replies %q", texts)
			}
			// The marker is still there: the administrator's own forward binds.
			env.process(forwardedPostUpdate(env.group, env.admin, post, post.ChannelPost.MessageId))
			if got := env.boundChannel(); got != env.channel.Id {
				t.Fatalf("the admin's forward after the automatic one bound %d, want %d", got, env.channel.Id)
			}
		})
	}

	t.Run("setfedlog", func(t *testing.T) {
		env := newSetFedLogFlowEnv(t)
		post := channelPostUpdate(env.channel, "/setfedlog "+env.fedID)
		env.process(post)
		env.process(automaticForwardUpdate(env.group, env.channel, "/setfedlog "+env.fedID, post.ChannelPost.MessageId))
		env.process(automaticForwardUpdate(env.group, env.channel, "/setfedlog", 0))
		if got := env.logChat(); got != 0 {
			t.Fatalf("an automatic forward set the fed log to %d", got)
		}
		if texts := flowSentTexts(env.client, env.group.Id); len(texts) != 0 {
			t.Fatalf("an automatic forward got replies %q", texts)
		}
	})
}
