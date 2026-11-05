package modules

import (
	"context"

	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
)

// memberReqSettingsLookup is the one read of a group's requirement settings, shared by the
// settings page, the Main status line and, later, the message watcher (D-06). It is the cached
// loader, so every reader sees the same value; it is a variable only so a test can make it fail.
var memberReqSettingsLookup = memberreq.GetSettings

// memberReqWrites holds the repository writes of the two switches. It is a variable only so a test
// can make a write fail.
var memberReqWrites = struct {
	CheckOnJoin    func(ctx context.Context, chatID int64, on bool) error
	DeleteMessages func(ctx context.Context, chatID int64, on bool) error
}{
	CheckOnJoin:    memberreq.SetCheckOnJoin,
	DeleteMessages: memberreq.SetDeleteMessages,
}
