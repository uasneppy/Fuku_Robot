package modules

import (
	"context"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

// The unmute code of the member requirements, shared by the lift of a mute whose notice could
// not be delivered, the rejoin reset and the "I fixed it" button. It never restricts a member
// with permissions that did not come from resolveUnmutePermissionsWithLockdown, so an active
// lockdown's stored pre-lockdown permissions apply and nobody gets rights back early.

// memberReqUnmutePermissions reads the group once through the requirement pacer and resolves
// the permissions an unmuted member gets: the live defaults, or the lockdown's stored
// pre-lockdown permissions (true) while a lockdown is active. An error means nothing may be
// restricted or announced.
func memberReqUnmutePermissions(ctx context.Context, b *gotgbot.Bot, chatID int64) (gotgbot.ChatPermissions, bool, error) {
	var chat *gotgbot.ChatFullInfo
	err := memberReqPaced(ctx, func(callCtx context.Context) error {
		var getErr error
		chat, getErr = b.GetChatWithContext(callCtx, chatID, nil)
		return getErr
	})
	if err != nil {
		return gotgbot.ChatPermissions{}, false, err
	}
	return resolveUnmutePermissionsWithLockdown(chat)
}

// memberReqLiftMute restricts the member to perms with no end date, through one paced
// restrictChatMember call. perms must come from memberReqUnmutePermissions.
func memberReqLiftMute(ctx context.Context, b *gotgbot.Bot, chatID, userID int64, perms gotgbot.ChatPermissions) error {
	return memberReqPaced(ctx, func(callCtx context.Context) error {
		_, err := b.RestrictChatMemberWithContext(callCtx, chatID, userID, perms, nil)
		return err
	})
}
