package modules

import (
	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
)

// memberReqSettingsLookup is the one read of a group's requirement settings, shared by the
// settings page, the Main status line and, later, the message watcher (D-06). It is the cached
// loader, so every reader sees the same value; it is a variable only so a test can make it fail.
var memberReqSettingsLookup = memberreq.GetSettings
