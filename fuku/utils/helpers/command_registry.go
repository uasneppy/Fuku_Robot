package helpers

import (
	"strings"
	"sync"

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
func NewBotCommand(name string, resp handlers.Response) handlers.Command {
	botCommandsMu.Lock()
	botCommands[strings.ToLower(name)] = struct{}{}
	botCommandsMu.Unlock()
	return handlers.NewCommand(name, resp)
}

// IsBotCommand reports whether the bot registers a handler for the command name, ignoring
// case. A name only another bot handles, or one nobody registers, is false.
func IsBotCommand(name string) bool {
	botCommandsMu.RLock()
	_, ok := botCommands[strings.ToLower(name)]
	botCommandsMu.RUnlock()
	return ok
}
