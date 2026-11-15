package modules

import (
	"sync/atomic"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/callbackquery"
)

// The MemberRequirements module screens members against the rules a group's owner set on the
// Member requirements page (D-12). This file holds the module, its registration and the
// enforcement switch; the message watcher is memberreq_watch.go and the executor it hands a
// violator to is memberreq_enforce.go.

// memberReqModule carries the module's name and the handler group of its message watcher.
var memberReqModule = moduleStruct{
	moduleName:   "MemberRequirements",
	handlerGroup: memberReqWatchGroup,
}

// memberReqEnforcementLive is the enforcement switch. It is true by default, stored by this
// file's init(), since the "I fixed it" handler is registered by LoadMemberRequirements: a
// mute always has a working button, so enforcement may run. It stays the one switch that turns
// the message watcher, the join entry and the captcha guard off everywhere, and it is read
// only through memberReqLive and nowhere else. The store is in init() and not in
// LoadMemberRequirements, because a test loads the module after turning the switch off with
// withMemberReqLive and a store at load would turn it back on under that test.
//
// It is an atomic.Bool and not a plain bool because goroutines a test does not own read it
// (a dispatcher goroutine, and the captcha lifecycle goroutines that reach the join entry
// can outlive the test that started them while the next test writes it), and the test
// suite runs with -race. Tests write it only through withMemberReqLive, which restores the
// previous value.
var memberReqEnforcementLive atomic.Bool

// memberReqLive reports whether the member requirements may punish anyone. It is the one
// reader of memberReqEnforcementLive.
func memberReqLive() bool {
	return memberReqEnforcementLive.Load()
}

// LoadMemberRequirements registers the message watcher and the "I fixed it" button. The
// watcher is the only message handler of handler group -2, which is what lets its
// ext.ContinueGroups be read as the "keep going" it is: no other matching handler may be added
// to that group. The button's namespace is registered with no channel permission, because a
// requirement notice is never a channel message.
func LoadMemberRequirements(dispatcher *ext.Dispatcher) {
	dispatcher.AddHandlerToGroup(handlers.NewMessage(func(m *gotgbot.Message) bool {
		return m.Chat.Type == "supergroup"
	}, memberReqModule.memberReqWatcher), memberReqWatchGroup)
	dispatcher.AddHandler(handlers.NewCallback(callbackquery.Prefix("mreq|"), memberReqModule.memberReqFixedCallback))
}

func init() {
	// The one production write of the switch: the button handler is registered by the module
	// below, so enforcement is on from here.
	memberReqEnforcementLive.Store(true)
	RegisterLegacyModule("MemberRequirements", 241, LoadMemberRequirements)
}
