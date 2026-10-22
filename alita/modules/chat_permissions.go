package modules

import (
	"encoding/json"
	"fmt"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db/lockdown"
	"github.com/divkix/Alita_Robot/alita/utils/helpers"
)

// MutedPermissions represents a fully restricted user - all sending capabilities disabled
var MutedPermissions = gotgbot.ChatPermissions{
	CanSendMessages:       false,
	CanSendPhotos:         false,
	CanSendVideos:         false,
	CanSendAudios:         false,
	CanSendDocuments:      false,
	CanSendVideoNotes:     false,
	CanSendVoiceNotes:     false,
	CanAddWebPagePreviews: false,
	CanChangeInfo:         false,
	CanInviteUsers:        false,
	CanPinMessages:        false,
	CanManageTopics:       helpers.Ptr(false),
	CanSendPolls:          false,
	CanSendOtherMessages:  false,
}

// defaultUnmutePermissions represents a safe fallback when chat defaults are unavailable.
func defaultUnmutePermissions() gotgbot.ChatPermissions {
	return gotgbot.ChatPermissions{
		CanSendMessages:       true,
		CanSendPhotos:         true,
		CanSendVideos:         true,
		CanSendAudios:         true,
		CanSendDocuments:      true,
		CanSendVideoNotes:     true,
		CanSendVoiceNotes:     true,
		CanAddWebPagePreviews: true,
		CanChangeInfo:         false,
		CanInviteUsers:        true,
		CanPinMessages:        false,
		CanManageTopics:       helpers.Ptr(false),
		CanSendPolls:          true,
		CanSendOtherMessages:  true,
	}
}

// lockdownSnapshotLookup reads a chat's active lockdown fresh from the database for
// resolveUnmutePermissions and the /unmute reply. It is a package variable so tests
// can force a lookup error.
var lockdownSnapshotLookup = lockdown.GetActiveFresh

// resolveUnmutePermissions is the single choke point of every unmute (D-23): /unmute,
// the unrestrict button, a captcha pass and staff /unmute, whose undo of a staff mute
// uses it too. It returns the permissions to give the unmuted user.
//
// During an active lockdown the group's live defaults are the locked set, so copying
// them into the user's own restriction would leave them muted after the lift. The
// lockdown's stored pre-lockdown permissions are returned instead, and a failed
// lookup is an error so no caller restricts or announces an unmute that did not
// happen. Without a lockdown the live defaults are used, else the built-in fallback.
// A chatInfo with Id 0 is never looked up.
func resolveUnmutePermissions(chatInfo *gotgbot.ChatFullInfo) (gotgbot.ChatPermissions, error) {
	if chatInfo != nil && chatInfo.Id != 0 {
		active, err := lockdownSnapshotLookup(chatInfo.Id)
		if err != nil {
			return gotgbot.ChatPermissions{}, fmt.Errorf("read the lockdown of chat %d before unmuting: %w", chatInfo.Id, err)
		}
		if active != nil {
			var snapshot gotgbot.ChatPermissions
			if err := json.Unmarshal([]byte(active.PrePermissions), &snapshot); err != nil {
				return gotgbot.ChatPermissions{}, fmt.Errorf("decode the pre-lockdown permissions of chat %d: %w", chatInfo.Id, err)
			}
			return snapshot, nil
		}
	}
	if chatInfo != nil && chatInfo.Permissions != nil {
		return *chatInfo.Permissions, nil
	}
	return defaultUnmutePermissions(), nil
}
