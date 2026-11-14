//go:build testtools

package modules

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"

	"github.com/uasneppy/fuku_robot/fuku/db"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/utils/tracing"
)

// Command deletion (plan 17-03, DEL-03): a handled command is answered and then deleted, through
// the real dispatcher with the wrapper main.go installs.

// commandProcessors are the two Inner processors every ordering test runs under: the bare one and
// the production one (tracing), delivered polling-style (the env's deliver passes nil data).
var commandProcessors = []struct {
	name  string
	inner ext.Processor
}{
	{"base processor", ext.BaseProcessor{}},
	{"tracing processor", tracing.TracingProcessor{}},
}

func TestCommandDeletionAfterAnswer(t *testing.T) {
	for _, proc := range commandProcessors {
		t.Run(proc.name, func(t *testing.T) {
			env := newCommandDeletionEnv(t, proc.inner)
			target := env.mreqMember("Bad", "Actor", "bad")

			id := env.deliver(&env.del.owner, commandMessage(fmt.Sprintf("/ban %d", target.Id)))

			if got := env.deleted(env.chat.Id, id); got != 1 {
				t.Fatalf("deletes of the /ban command = %d, want exactly 1", got)
			}
			reply := env.replyIndexTo(env.chat.Id, id)
			del := env.deleteIndexOf(env.chat.Id, id)
			if reply < 0 || del < 0 {
				t.Fatalf("reply index = %d, delete index = %d, want both in the call log (methods %v)", reply, del, env.methods())
			}
			if reply >= del {
				t.Fatalf("the reply request is at %d and the delete at %d, want the answer first (methods %v)", reply, del, env.methods())
			}
			for _, sent := range env.sentIDs() {
				if got := env.deleted(env.chat.Id, sent); got != 0 {
					t.Errorf("the bot's own message %d was deleted %d times, want it kept", sent, got)
				}
			}
			if rows := env.rows(); len(rows) != 0 {
				t.Fatalf("queue rows after an inline delete = %+v, want none", rows)
			}
			if calls := env.fake.callsFor("banChatMember"); len(calls) != 1 {
				t.Errorf("banChatMember calls = %d, want the command's own effect", len(calls))
			}
		})
	}
}

func TestCommandDeletionRawCommand(t *testing.T) {
	env := newCommandDeletionEnv(t, ext.BaseProcessor{})
	botName := env.bot.Username
	cases := []struct {
		name  string
		apply func(*gotgbot.Message)
	}{
		{"bare", commandMessage("/rules")},
		{"upper case", commandMessage("/RULES")},
		{"addressed to this bot", commandMessage("/rules@" + botName)},
		{"addressed in another case", commandMessage("/rules@" + "FUKUTESTBOT")},
		{"caption on a photo", func(m *gotgbot.Message) {
			m.Caption = "/rules"
			m.CaptionEntities = []gotgbot.MessageEntity{{Type: gotgbot.MessageEntityTypeBotCommand, Offset: 0, Length: 6}}
			m.Photo = []gotgbot.PhotoSize{{FileId: "photo", FileUniqueId: "photo-u", Width: 8, Height: 8}}
		}},
		{"no entities", func(m *gotgbot.Message) { m.Text = "/rules" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			member := env.mreqMember("Rae", "", "rae")
			before := len(env.mreqNotices())

			id := env.deliver(&member, tc.apply)

			if got := env.deleted(env.chat.Id, id); got != 1 {
				t.Fatalf("deletes of the command = %d, want exactly 1", got)
			}
			if after := len(env.mreqNotices()); after <= before {
				t.Fatalf("messages to the group = %d before and %d after, want the command answered", before, after)
			}
			send, del := env.anySendIndex(env.chat.Id), env.deleteIndexOf(env.chat.Id, id)
			if send < 0 || del < 0 || send >= del {
				t.Fatalf("first send at %d, delete at %d, want the answer before the delete (methods %v)", send, del, env.methods())
			}
		})
	}
}

// commandKeepCase is one message that must stay.
type commandKeepCase struct {
	name string
	// setup changes the stored rule; nil keeps member:commands stored as 0.
	setup func(env *commandDeletionEnv)
	// send delivers the message and returns the chat and the message ID it carried.
	send func(env *commandDeletionEnv, member gotgbot.User) (int64, int64)
}

// sendCommandInGroup delivers a command text of from in the env chat with mutate applied.
func sendCommandInGroup(text string, mutate func(env *commandDeletionEnv, m *gotgbot.Message)) func(*commandDeletionEnv, gotgbot.User) (int64, int64) {
	return func(env *commandDeletionEnv, member gotgbot.User) (int64, int64) {
		id := env.deliver(&member, func(m *gotgbot.Message) {
			commandMessage(text)(m)
			if mutate != nil {
				mutate(env, m)
			}
		})
		return env.chat.Id, id
	}
}

func TestCommandDeletionKeeps(t *testing.T) {
	cases := []commandKeepCase{
		{name: "another bot's command", send: sendCommandInGroup("/rules@otherbot", nil)},
		{name: "an unregistered word", send: sendCommandInGroup("/nosuchcommand17", nil)},
		{name: "first entity is a mention", send: sendCommandInGroup("/rules", func(_ *commandDeletionEnv, m *gotgbot.Message) {
			m.Entities = []gotgbot.MessageEntity{{Type: gotgbot.MessageEntityTypeMention, Offset: 0, Length: 6}}
		})},
		{name: "a plain text", send: func(env *commandDeletionEnv, member gotgbot.User) (int64, int64) {
			return env.chat.Id, env.deliver(&member, func(m *gotgbot.Message) { m.Text = "hello rules" })
		}},
		{name: "an edited command", send: func(env *commandDeletionEnv, member gotgbot.User) (int64, int64) {
			id := env.updateID()
			msg := &gotgbot.Message{MessageId: id, Date: env.now().Unix(), Chat: env.chat, From: &member}
			commandMessage("/rules")(msg)
			if err := env.dispatcher.ProcessUpdate(env.bot, &gotgbot.Update{UpdateId: env.updateID(), EditedMessage: msg}, nil); err != nil {
				t.Fatalf("ProcessUpdate(edited) error = %v", err)
			}
			return env.chat.Id, id
		}},
		{name: "a private chat", send: func(env *commandDeletionEnv, member gotgbot.User) (int64, int64) {
			id := env.updateID()
			chat := gotgbot.Chat{Id: member.Id, Type: gotgbot.ChatTypePrivate}
			msg := &gotgbot.Message{MessageId: id, Date: env.now().Unix(), Chat: chat, From: &member}
			commandMessage("/rules")(msg)
			if err := env.dispatcher.ProcessUpdate(env.bot, &gotgbot.Update{UpdateId: env.updateID(), Message: msg}, nil); err != nil {
				t.Fatalf("ProcessUpdate(private) error = %v", err)
			}
			return chat.Id, id
		}},
		{name: "a bot user", send: func(env *commandDeletionEnv, _ gotgbot.User) (int64, int64) {
			robot := gotgbot.User{Id: uniqueLinkOwnerID(), IsBot: true, FirstName: "Robot"}
			return env.chat.Id, env.deliver(&robot, commandMessage("/rules"))
		}},
		{name: "a sender chat", send: sendCommandInGroup("/rules", func(_ *commandDeletionEnv, m *gotgbot.Message) {
			m.SenderChat = &gotgbot.Chat{Id: -1002000000001, Type: gotgbot.ChatTypeChannel, Title: "News"}
		})},
		{name: "an anonymous admin posting as the group", send: func(env *commandDeletionEnv, _ gotgbot.User) (int64, int64) {
			anonymous := gotgbot.User{Id: 1087968824, IsBot: true, FirstName: "Group"}
			id := env.deliver(&anonymous, func(m *gotgbot.Message) {
				commandMessage("/rules")(m)
				chat := env.chat
				m.SenderChat = &chat
			})
			return env.chat.Id, id
		}},
		{name: "a Telegram service account", send: func(env *commandDeletionEnv, _ gotgbot.User) (int64, int64) {
			service := gotgbot.User{Id: 777000, FirstName: "Telegram"}
			return env.chat.Id, env.deliver(&service, commandMessage("/rules"))
		}},
		{name: "no rule row", setup: func(env *commandDeletionEnv) { env.setCommandsRule(false) },
			send: sendCommandInGroup("/rules", nil)},
		{name: "a stored value of 60", setup: func(env *commandDeletionEnv) {
			if err := db.DB.Model(&models.DeleteRule{}).
				Where("chat_id = ? AND kind = ?", env.chat.Id, deletion.KindMemberCommands).
				Updates(map[string]any{"delay_secs": 60, "updated_at": time.Now().UTC()}).Error; err != nil {
				t.Fatalf("write a stored 60: %v", err)
			}
			dbcache.DeleteCache(dbcache.CacheKey("delete_rules", env.chat.Id))
			dbcache.ResetLocalForTest()
		}, send: sendCommandInGroup("/rules", nil)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newCommandDeletionEnv(t, ext.BaseProcessor{})
			if tc.setup != nil {
				tc.setup(env)
			}
			member := env.mreqMember("Kay", "", "kay")

			chatID, id := tc.send(env, member)

			env.wantNoCommandDelete(chatID, id)
		})
	}
}

// TestCommandDeletionPollingContext proves the delete does not depend on the context the tracing
// processor cancels when it returns: with tracing.TracingProcessor inside the wrapper and the
// update delivered polling-style (nil data), a wrapped and a raw command are each answered and
// deleted once.
func TestCommandDeletionPollingContext(t *testing.T) {
	env := newCommandDeletionEnv(t, tracing.TracingProcessor{})
	target := env.mreqMember("Bad", "Actor", "bad")
	member := env.mreqMember("Rae", "", "rae")

	wrapped := env.deliver(&env.del.owner, commandMessage(fmt.Sprintf("/ban %d", target.Id)))
	raw := env.deliver(&member, commandMessage("/rules"))

	for name, id := range map[string]int64{"wrapped /ban": wrapped, "raw /rules": raw} {
		if got := env.deleted(env.chat.Id, id); got != 1 {
			t.Errorf("deletes of the %s command = %d, want exactly 1", name, got)
		}
		if reply, del := env.anySendIndex(env.chat.Id), env.deleteIndexOf(env.chat.Id, id); reply < 0 || del < 0 || reply >= del {
			t.Errorf("%s: first send at %d, delete at %d, want the answer before the delete", name, reply, del)
		}
	}
	if rows := env.rows(); len(rows) != 0 {
		t.Fatalf("queue rows = %+v, want none (the inline deletes succeeded)", rows)
	}
}

// TestBotCommandNameMatchesGotgbot is the differential proof: for every message of the table and
// every command name of the universe, botCommandName accepts the message for that name exactly
// when the library's own Command.CheckUpdate built for that name does, and whatever name it
// returns is one the library accepts the message for.
func TestBotCommandNameMatchesGotgbot(t *testing.T) {
	bot := &gotgbot.Bot{User: gotgbot.User{Id: 999, IsBot: true, Username: "FukuTestBot"}}
	botCmd := func(length int64) []gotgbot.MessageEntity {
		return []gotgbot.MessageEntity{{Type: gotgbot.MessageEntityTypeBotCommand, Offset: 0, Length: length}}
	}
	paragraph := func(text gotgbot.RichText) *gotgbot.RichMessage {
		return &gotgbot.RichMessage{Blocks: gotgbot.RichBlockArray{gotgbot.RichBlockParagraph{Text: text}}}
	}
	universe := []string{"rules", "ban", "start", "x", "a", "ünï", "rules extra", "rules arg"}
	messages := []struct {
		name string
		msg  gotgbot.Message
	}{
		{"bare", gotgbot.Message{Text: "/rules", Entities: botCmd(6)}},
		{"this bot", gotgbot.Message{Text: "/rules@FukuTestBot", Entities: botCmd(18)}},
		{"this bot lower case", gotgbot.Message{Text: "/rules@fukutestbot", Entities: botCmd(18)}},
		{"this bot mixed case", gotgbot.Message{Text: "/RULES@FUKUtestbot", Entities: botCmd(18)}},
		{"another bot", gotgbot.Message{Text: "/rules@otherbot", Entities: botCmd(15)}},
		{"upper case", gotgbot.Message{Text: "/RULES", Entities: botCmd(6)}},
		{"trailing argument", gotgbot.Message{Text: "/ban 42", Entities: botCmd(4)}},
		{"newline after", gotgbot.Message{Text: "/rules\nmore", Entities: botCmd(6)}},
		{"tab after", gotgbot.Message{Text: "/start\targ", Entities: botCmd(6)}},
		{"leading spaces", gotgbot.Message{Text: "  /rules"}},
		{"no entities", gotgbot.Message{Text: "/rules"}},
		{"first entity mention", gotgbot.Message{Text: "/rules", Entities: []gotgbot.MessageEntity{{Type: gotgbot.MessageEntityTypeMention, Offset: 0, Length: 6}}}},
		{"first entity after offset 0", gotgbot.Message{Text: "/rules", Entities: []gotgbot.MessageEntity{{Type: gotgbot.MessageEntityTypeBold, Offset: 3, Length: 3}}}},
		{"empty text", gotgbot.Message{}},
		{"only a slash", gotgbot.Message{Text: "/", Entities: botCmd(1)}},
		{"double slash", gotgbot.Message{Text: "//rules"}},
		{"slash then at", gotgbot.Message{Text: "/rules@"}},
		{"plain word", gotgbot.Message{Text: "rules"}},
		{"other trigger", gotgbot.Message{Text: "!rules"}},
		{"unicode letter", gotgbot.Message{Text: "/ünï", Entities: botCmd(5)}},
		{"unicode upper", gotgbot.Message{Text: "/ÜNÏ"}},
		{"caption", gotgbot.Message{Caption: "/rules", CaptionEntities: botCmd(6)}},
		{"caption wins over text", gotgbot.Message{Text: "/ban", Caption: "/rules", Entities: botCmd(4)}},
		{"rich string", gotgbot.Message{RichMessage: paragraph(gotgbot.RichTextString("/rules"))}},
		{"rich string with argument", gotgbot.Message{RichMessage: paragraph(gotgbot.RichTextString("/rules arg"))}},
		{"rich array", gotgbot.Message{RichMessage: paragraph(gotgbot.RichTextArray{gotgbot.RichTextString("/start")})}},
		{"rich empty array", gotgbot.Message{RichMessage: paragraph(gotgbot.RichTextArray{})}},
		{"rich bot command", gotgbot.Message{RichMessage: paragraph(gotgbot.RichTextBotCommand{
			Text: gotgbot.RichTextString("/ban@FukuTestBot"), BotCommand: "ban"})}},
		{"rich bot command upper case", gotgbot.Message{RichMessage: paragraph(gotgbot.RichTextBotCommand{
			Text: gotgbot.RichTextString("/Ban"), BotCommand: "Ban"})}},
		{"rich no blocks", gotgbot.Message{RichMessage: &gotgbot.RichMessage{}}},
	}
	for _, tc := range messages {
		t.Run(tc.name, func(t *testing.T) {
			msg := tc.msg
			ctx := &ext.Context{Update: &gotgbot.Update{Message: &msg}}
			accepts := func(name string) bool {
				return handlers.NewCommand(name, nil).CheckUpdate(bot, ctx)
			}
			got, ok := botCommandName(bot, &msg)
			if ok && !accepts(got) {
				t.Fatalf("botCommandName = %q, true, but the library handler for %q refuses the message", got, got)
			}
			for _, name := range universe {
				if want := accepts(name); want != (ok && got == strings.ToLower(name)) {
					t.Errorf("name %q: library accepts = %v, botCommandName = %q (%v)", name, want, got, ok)
				}
			}
		})
	}
}
