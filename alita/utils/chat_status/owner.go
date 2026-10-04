package chat_status

import (
	"context"
	"errors"
	"strings"
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

// liveCheckTimeout bounds the live Telegram calls made by CheckOwner and
// FetchBotMember.
var liveCheckTimeout = 8 * time.Second

// botMissingMarkers are the lower-cased fragments of the Telegram error
// descriptions that say the bot is not in the chat.
var botMissingMarkers = []string{"bot was kicked", "bot is not a member", "chat not found"}

// isBotMissingError reports whether err is a Telegram answer saying the bot is
// no longer in the chat (kicked, never a member, or the chat is gone).
func isBotMissingError(err error) bool {
	var tgErr *gotgbot.TelegramError
	if !errors.As(err, &tgErr) {
		return false
	}
	description := strings.ToLower(tgErr.Description)
	for _, marker := range botMissingMarkers {
		if strings.Contains(description, marker) {
			return true
		}
	}
	return false
}

// FetchBotMember asks Telegram, live and uncached, for the bot's own membership
// record in chatID.
//
// It is tri-state like CheckOwner: BotMemberFound carries the merged member
// (Status, CanRestrictMembers and the other rights); BotMemberMissing means
// Telegram definitely says the bot is not there, either through an error such as
// "bot was kicked" or through a left or kicked status, and returns a nil error;
// BotMemberUnknown covers every other failure (flood wait, timeout, network) with
// a non-nil error. Callers must fail closed on Unknown and never read it as
// Missing.
func FetchBotMember(b *gotgbot.Bot, chatID int64) (gotgbot.MergedChatMember, BotMemberResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), liveCheckTimeout)
	defer cancel()

	member, err := b.GetChatMemberWithContext(ctx, chatID, b.Id, nil)
	if err != nil {
		if isBotMissingError(err) {
			return gotgbot.MergedChatMember{}, BotMemberMissing, nil
		}
		return gotgbot.MergedChatMember{}, BotMemberUnknown, err
	}
	if member == nil {
		return gotgbot.MergedChatMember{}, BotMemberUnknown, errors.New("getChatMember returned no member")
	}

	merged := member.MergeChatMember()
	switch merged.Status {
	case gotgbot.ChatMemberStatusLeft, gotgbot.ChatMemberStatusKicked:
		return merged, BotMemberMissing, nil
	}
	return merged, BotMemberFound, nil
}

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
