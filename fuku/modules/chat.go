package modules

import (
	"strings"
	"unicode"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/message"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/utils/chatprompt"
	"github.com/uasneppy/fuku_robot/fuku/utils/helpers"
)

// The Chat module (Phase 20): Xenobot-72 answers members who address it, in groups whose owner
// turned conversations on from the bot's private chat. It never converses in a private chat.

var chatModule = moduleStruct{moduleName: "Chat"}

// LoadChat registers /ask (alias /xeno) and the conversation watcher, alone at its own handler
// group after every other group, with no setter (new messages of real users only).
func LoadChat(dispatcher *ext.Dispatcher) {
	helpers.WrapCommand(dispatcher, helpers.CommandDescriptor{
		Name:           "ask",
		Disableable:    true,
		RequiredChecks: []helpers.CheckFunc{helpers.CheckDisabled("ask")},
	}, chatModule.ask)
	// /xeno honours its own name and /ask's, so disabling /ask silences both.
	helpers.WrapCommand(dispatcher, helpers.CommandDescriptor{
		Name:           "xeno",
		Disableable:    true,
		RequiredChecks: []helpers.CheckFunc{helpers.CheckDisabled("xeno"), helpers.CheckDisabled("ask")},
	}, chatModule.ask)
	dispatcher.AddHandlerToGroup(handlers.NewMessage(message.All, chatModule.chatWatcher), chatWatchGroup)

	SetModuleEnabled(chatModule.moduleName, true)
	DefaultHelpRegistry().AltHelpOptions["Chat"] = []string{"conversations", "ask", "xeno"}
	log.Info("[Modules] Chat module loaded")
}

// ask answers /ask <text>: in a private chat it says conversations happen in groups; in a group
// whose conversations are off it says how the owner turns them on; with no text it shows the
// usage; otherwise it hands the text to the conversation pool like an addressed message, after
// the same checks. Anonymous admins and channels get nothing.
func (moduleStruct) ask(c *helpers.CommandContext) error {
	msg, chat := c.Msg, c.Chat
	if chat.Type == "private" {
		chatReplyText(c, "chat_private_refused")
		return ext.EndGroups
	}
	if chat.Type != "group" && chat.Type != "supergroup" {
		return ext.EndGroups
	}
	userID, ok := raidFloodSenderCounts(c.Bot, c.Ctx)
	if !ok || c.User.IsBot {
		return ext.EndGroups
	}
	gate, provider := chatAccept(c.Bot, chat.Id, userID)
	switch gate {
	case chatGateOff:
		chatReplyText(c, "chat_off_hint")
		return ext.EndGroups
	case chatGateSilent:
		return ext.EndGroups
	}
	text := chatCommandArgument(msg.Text)
	if text == "" {
		chatReplyText(c, "chat_ask_usage")
		return ext.EndGroups
	}
	job := chatJobOf(c.Bot, msg, userID, provider)
	job.turn.Text = strings.TrimSpace(chatprompt.StripMention(text, c.Bot.Username))
	chatEnqueue(job)
	return ext.EndGroups
}

// chatCommandArgument is the text after a command's first word, line breaks kept.
func chatCommandArgument(text string) string {
	i := strings.IndexFunc(text, unicode.IsSpace)
	if i < 0 {
		return ""
	}
	return strings.TrimSpace(text[i:])
}

// chatReplyText answers the command with one locale line, in HTML like every reply.
func chatReplyText(c *helpers.CommandContext, key string) {
	text, err := c.Tr.GetString(key)
	if err != nil || strings.TrimSpace(text) == "" {
		return
	}
	if _, err := c.Msg.Reply(c.Bot, text, &gotgbot.SendMessageOpts{ParseMode: "HTML"}); err != nil {
		log.WithField("chat_id", c.Chat.Id).Debugf("[Chat] the /ask answer could not be sent: %s", chatSendErrClass(err))
	}
}

func init() {
	RegisterLegacyModule("Chat", 245, LoadChat)
}
