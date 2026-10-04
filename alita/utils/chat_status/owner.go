package chat_status

import (
	"context"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

// OwnerResult is the outcome of a live ownership check.
type OwnerResult int

const (
	// OwnerMatch means Telegram lists the wanted user as the chat's creator.
	OwnerMatch OwnerResult = iota
	// OwnerMismatch means Telegram answered and the wanted user is not the
	// creator: either a different creator is listed or none is.
	OwnerMismatch
	// OwnerUnknown means the answer could not be obtained (API error, timeout).
	OwnerUnknown
)

// BotMemberResult is the outcome of a live lookup of the bot's own membership.
type BotMemberResult int

const (
	// BotMemberFound means Telegram answered with a member record for the bot.
	BotMemberFound BotMemberResult = iota
	// BotMemberMissing means Telegram says the bot is not in the chat.
	BotMemberMissing
	// BotMemberUnknown means the answer could not be obtained.
	BotMemberUnknown
)

// FetchBotMember is a RED-phase stub.
func FetchBotMember(b *gotgbot.Bot, chatID int64) (gotgbot.MergedChatMember, BotMemberResult, error) {
	return gotgbot.MergedChatMember{}, BotMemberFound, nil
}

// liveCheckTimeout bounds the getChatAdministrators call made by CheckOwner.
var liveCheckTimeout = 8 * time.Second

// CheckOwner asks Telegram, live and uncached, whether wantUserID is the
// creator of chatID. It returns the creator's user ID when one is listed.
//
// It is tri-state on purpose: any API error gives OwnerUnknown with a non-nil
// error, never OwnerMismatch. Only OwnerMismatch may ever remove state;
// OwnerUnknown must fail closed for actions and must never delete anything.
// Nothing is cached, so the 30 minute admin cache is never an authority.
func CheckOwner(b *gotgbot.Bot, chatID, wantUserID int64) (OwnerResult, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), liveCheckTimeout)
	defer cancel()

	members, err := b.GetChatAdministratorsWithContext(ctx, chatID, nil)
	if err != nil {
		return OwnerUnknown, 0, err
	}
	for _, member := range members {
		if member.GetStatus() != gotgbot.ChatMemberStatusCreator {
			continue
		}
		creatorID := member.GetUser().Id
		if creatorID == wantUserID {
			return OwnerMatch, creatorID, nil
		}
		return OwnerMismatch, creatorID, nil
	}
	return OwnerMismatch, 0, nil
}
