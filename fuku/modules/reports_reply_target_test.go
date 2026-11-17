package modules

import (
	"sync/atomic"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/message"
)

// TestReportIgnoresForumTopicRoot holds M6: in a forum topic, a /report that is
// not an explicit reply carries the topic's forum_topic_created message as its
// reply_to_message; that is no report target, so no report card is posted.
func TestReportIgnoresForumTopicRoot(t *testing.T) {
	client := newModuleBotClient()
	bot := newModuleTestBot(client)
	chat := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Forum", IsForum: true}
	reporter := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Reporter"}
	creator := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Topic Creator"}

	ctx := newModuleMessageContext(bot, chat, reporter, "/report")
	ctx.EffectiveMessage.IsTopicMessage = true
	ctx.EffectiveMessage.MessageThreadId = 70
	ctx.EffectiveMessage.ReplyToMessage = &gotgbot.Message{
		MessageId:         70,
		Date:              1,
		Chat:              chat,
		From:              &creator,
		ForumTopicCreated: &gotgbot.ForumTopicCreated{Name: "topic"},
	}
	if err := reportsModule.report(bot, ctx); err != ext.EndGroups {
		t.Fatalf("report error = %v, want EndGroups", err)
	}
	for _, call := range client.callsFor("sendMessage") {
		if call.Params["reply_markup"] != nil {
			t.Fatalf("a report card was posted for the topic's creator: %v", call.Params)
		}
	}
}

// TestAdminMentionWatcherLetsLaterGroupsRun holds M6: the @admin watcher at
// group 8 reports and then lets groups 9 to 12 (filters, pins, the log channel,
// the late raid flood counter) see the message.
func TestAdminMentionWatcherLetsLaterGroupsRun(t *testing.T) {
	client := newModuleBotClient()
	bot := newModuleTestBot(client)
	chat := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Report Chat"}
	reporter := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Reporter"}
	target := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Target"}

	dispatcher := ext.NewDispatcher(&ext.DispatcherOpts{MaxRoutines: -1})
	LoadReports(dispatcher)
	var later atomic.Int32
	dispatcher.AddHandlerToGroup(handlers.NewMessage(message.All, func(*gotgbot.Bot, *ext.Context) error {
		later.Add(1)
		return ext.ContinueGroups
	}), 9)

	update := &gotgbot.Update{UpdateId: 1, Message: &gotgbot.Message{
		MessageId: 600,
		Date:      1,
		Chat:      chat,
		From:      &reporter,
		Text:      "@admin look at this",
		ReplyToMessage: &gotgbot.Message{
			MessageId: 599,
			Date:      1,
			Chat:      chat,
			From:      &target,
			Text:      "spam",
		},
	}}
	if err := dispatcher.ProcessUpdate(bot, update, nil); err != nil {
		t.Fatalf("ProcessUpdate error = %v", err)
	}
	reported := false
	for _, call := range client.callsFor("sendMessage") {
		if call.Params["reply_markup"] != nil {
			reported = true
		}
	}
	if !reported {
		t.Fatal("the @admin message was not reported")
	}
	if later.Load() != 1 {
		t.Fatalf("group 9 handler ran %d times, want 1", later.Load())
	}
}
