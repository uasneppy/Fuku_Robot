// Package deletion holds the scheduled deletion data: each chat's delay for one kind of
// message (chat_delete_rules, read through the cache) and the durable queue of messages
// waiting to be deleted (scheduled_deletions, read and written fresh).
//
// Kinds are validated here, never by a database CHECK, so a later message type needs no
// migration. A kind is "service:<type>" for a Telegram service message or "bot:<kind>" for
// a message the bot sent.
package deletion

import (
	"errors"
	"strings"
)

// The kinds of message a rule or a queue row can name.
const (
	// KindJoined is the "new members joined" service message. It shares the /cleanservice flag.
	KindJoined = "service:new_chat_members"
	// KindLeft is the "member left" service message. It shares the /cleanservice flag.
	KindLeft = "service:left_chat_member"
	// KindPhoto is the "group photo changed or removed" service message.
	KindPhoto = "service:chat_photo"
	// KindTitle is the "group title changed" service message.
	KindTitle = "service:chat_title"
	// KindPinned is the "message pinned" service message.
	KindPinned = "service:pinned_message"
	// KindVideo is the video chat service messages.
	KindVideo = "service:video_chat"
	// KindForum is the forum topic service messages.
	KindForum = "service:forum_topic"
	// KindBoost is the boost service messages.
	KindBoost = "service:boost"
	// KindTimer is the "auto-delete timer changed" service message.
	KindTimer = "service:auto_delete_timer"
	// KindBackground is the "chat background changed" service message.
	KindBackground = "service:chat_background"
	// KindGiveaway is the giveaway service messages.
	KindGiveaway = "service:giveaway"
	// KindOther is every other service message.
	KindOther = "service:other"

	// KindBotWelcome is a welcome message the bot sent.
	KindBotWelcome = "bot:welcome"
	// KindBotGoodbye is a goodbye message the bot sent.
	KindBotGoodbye = "bot:goodbye"
	// KindBotRules is a rules message the bot sent in the group.
	KindBotRules = "bot:rules"
	// KindBotAuto is the reply of an automatic punishment (warn, antiflood, member requirement).
	KindBotAuto = "bot:punish_auto"
	// KindBotManual is the reply of a manual punishment command (/ban, /mute, /warn and the rest).
	KindBotManual = "bot:punish_manual"
)

// MaxDelaySecs is the longest delay a rule or a queue row may carry: one day, well below
// Telegram's 48 hour window for deleting a message.
const MaxDelaySecs = 86400

const (
	servicePrefix = "service:"
	botPrefix     = "bot:"
)

var (
	// ErrUnknownKind is returned for a kind that is not in Kinds. Nothing is written.
	ErrUnknownKind = errors.New("unknown deletion kind")
	// ErrDelayOutOfRange is returned for a delay outside 0..MaxDelaySecs. Nothing is written.
	ErrDelayOutOfRange = errors.New("deletion delay out of range")
)

// kinds lists every kind in the order the settings page shows them.
var kinds = []string{
	KindJoined, KindLeft, KindPhoto, KindTitle, KindPinned, KindVideo, KindForum, KindBoost,
	KindTimer, KindBackground, KindGiveaway, KindOther,
	KindBotWelcome, KindBotGoodbye, KindBotRules, KindBotAuto, KindBotManual,
}

// Kinds returns every kind in page order. The slice is a copy.
func Kinds() []string {
	return append([]string(nil), kinds...)
}

// ValidKind reports whether kind is one of the registered kinds.
func ValidKind(kind string) bool {
	for _, k := range kinds {
		if k == kind {
			return true
		}
	}
	return false
}

// IsServiceKind reports whether kind is a "service:" kind.
func IsServiceKind(kind string) bool {
	return strings.HasPrefix(kind, servicePrefix)
}

// IsBotKind reports whether kind is a "bot:" kind.
func IsBotKind(kind string) bool {
	return strings.HasPrefix(kind, botPrefix)
}

// SharesCleanServiceFlag reports whether kind's on/off state is the shared /cleanservice
// flag of the greetings row: true for the joined and left service messages only.
func SharesCleanServiceFlag(kind string) bool {
	return kind == KindJoined || kind == KindLeft
}

// validateDelay returns ErrDelayOutOfRange for a delay outside 0..MaxDelaySecs.
func validateDelay(delay int) error {
	if delay < 0 || delay > MaxDelaySecs {
		return ErrDelayOutOfRange
	}
	return nil
}
