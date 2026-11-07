package modules

import (
	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
)

// memberReqWatchGroup is the handler group of the message watcher.
const memberReqWatchGroup = -2

// memberReqJobs counts the enforcement jobs in flight.
var memberReqJobs raidPostTracker

// memberReqWatcher is the group -2 message watcher.
func (m moduleStruct) memberReqWatcher(_ *gotgbot.Bot, _ *ext.Context) error {
	return ext.ContinueGroups
}
