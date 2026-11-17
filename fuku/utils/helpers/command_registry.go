package helpers

import (
	"strings"
	"sync"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
)

var (
	botCommands   = make(map[string]struct{})
	botCommandsMu sync.RWMutex
)

// NewBotCommand builds the command handler for name and records the lower-cased name in
// the registry that IsBotCommand reads. It returns the library's own command handler for
// the same arguments, so matching (case, entities, the @bot suffix) is exactly the
// library's, and a chained setter such as SetAllowEdited keeps working.
//
// This is the only place in the repository that calls the library's command
// constructor: every other command handler, wrapped or raw, is built here, so the
// registry holds exactly the names the bot routes. TestNoRawNewCommand fails on any other
// construction.
//
// The response is wrapped so that an automatic forward (IsAutomaticForward) never runs a command:
// the wrapper returns ext.ContinueGroups at once, with no reply and no I/O, because the Telegram
// service account in such a message's from field passes the admin predicates.
func NewBotCommand(name string, resp handlers.Response) handlers.Command {
	botCommandsMu.Lock()
	botCommands[strings.ToLower(name)] = struct{}{}
	botCommandsMu.Unlock()
	return handlers.NewCommand(name, skipAutomaticForward(resp))
}

// skipAutomaticForward wraps a command response so an automatic forward is passed on untouched.
func skipAutomaticForward(resp handlers.Response) handlers.Response {
	return func(b *gotgbot.Bot, ctx *ext.Context) error {
		if ctx != nil && IsAutomaticForward(ctx.EffectiveMessage) {
			return ext.ContinueGroups
		}
		return resp(b, ctx)
	}
}

// IsAutomaticForward reports whether msg is a linked channel's post that Telegram forwarded into
// the channel's discussion group, or any other message the Telegram service account (777000)
// sent on behalf of a chat. Such a message is nobody's command: the chat_status admin predicates
// count 777000 as an administrator of every chat, so a channel post must never reach them.
func IsAutomaticForward(msg *gotgbot.Message) bool {
	if msg == nil {
		return false
	}
	if msg.IsAutomaticForward {
		return true
	}
	return msg.SenderChat != nil && msg.From != nil && msg.From.Id == telegramServiceUserID
}

// IsBotCommand reports whether the bot registers a handler for the command name, ignoring
// case. A name only another bot handles, or one nobody registers, is false.
func IsBotCommand(name string) bool {
	botCommandsMu.RLock()
	_, ok := botCommands[strings.ToLower(name)]
	botCommandsMu.RUnlock()
	return ok
}
