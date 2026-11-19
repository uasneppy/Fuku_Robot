package modules

import (
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/message"
)

// The Chat module (Phase 20): Xenobot-72 answers members who address it, in groups whose owner
// turned conversations on from the bot's private chat. It never converses in a private chat.

var chatModule = moduleStruct{moduleName: "Chat"}

// LoadChat registers the conversation watcher alone at its own handler group, after every other
// group, with no setter (new messages of real users only).
func LoadChat(dispatcher *ext.Dispatcher) {
	dispatcher.AddHandlerToGroup(handlers.NewMessage(message.All, chatModule.chatWatcher), chatWatchGroup)
}

func init() {
	RegisterLegacyModule("Chat", 245, LoadChat)
}
