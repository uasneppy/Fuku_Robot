// Package deletion holds the scheduled deletion data: each chat's delay for one kind of
// message (chat_delete_rules, read through the cache) and the durable queue of messages
// waiting to be deleted (scheduled_deletions, read and written fresh).
//
// Kinds are validated here, never by a database CHECK, so a later message type needs no
// migration. A kind is "service:<type>" for a Telegram service message, "bot:<kind>" for
// a message the bot sent or "member:<rule>" for one of the three rules about members' own
// messages (their commands, their edits and self-destruct).
package deletion

import (
	"errors"
	"slices"
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

	// KindMemberCommands is the switch that deletes a member's command message once it was handled.
	// Its rule carries a stored 0 only: a stored 0 means On.
	KindMemberCommands = "member:commands"
	// KindMemberEdited is the grace after which a member's edited message is deleted. Its rule
	// carries one of the grace presets; its queue entries are the due-now fallback of an inline
	// delete, so they carry 0.
	KindMemberEdited = "member:edited"
	// KindMemberSelfDestruct is the delay after which every member message is deleted. Its rule and
	// its queue entries carry one of the self-destruct presets.
	KindMemberSelfDestruct = "member:selfdestruct"
)

// MaxDelaySecs is the longest delay a rule or a queue row may carry: one day, well below
// Telegram's 48 hour window for deleting a message.
const MaxDelaySecs = 86400

// servicePrefix is the prefix of every service kind.
const servicePrefix = "service:"

// memberPrefix is the prefix of every member kind.
const memberPrefix = "member:"

// editedGraceDelays are the delays, in seconds, the member:edited rule accepts: 30 s, 1, 5 and 15
// minutes and 1 hour.
var editedGraceDelays = []int{30, 60, 300, 900, 3600}

// selfDestructDelays are the delays, in seconds, a member:selfdestruct rule or queue entry
// accepts: 1, 5 and 10 minutes, 1, 6 and 24 hours.
var selfDestructDelays = []int{60, 300, 600, 3600, 21600, 86400}

var (
	// ErrUnknownKind is returned for a kind that is not in Kinds. Nothing is written.
	ErrUnknownKind = errors.New("unknown deletion kind")
	// ErrDelayOutOfRange is returned for a delay a kind does not accept: outside 0..MaxDelaySecs,
	// or, for a member kind, not one of its presets. Nothing is written.
	ErrDelayOutOfRange = errors.New("deletion delay out of range")
)

// kinds lists every kind in the order the settings page shows them.
var kinds = []string{
	KindJoined, KindLeft, KindPhoto, KindTitle, KindPinned, KindVideo, KindForum, KindBoost,
	KindTimer, KindBackground, KindGiveaway, KindOther,
	KindBotWelcome, KindBotGoodbye, KindBotRules, KindBotAuto, KindBotManual,
	KindMemberCommands, KindMemberEdited, KindMemberSelfDestruct,
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

// IsMemberKind reports whether kind is a "member:" kind.
func IsMemberKind(kind string) bool {
	return strings.HasPrefix(kind, memberPrefix)
}

// NeedsDeleteRight reports whether deleting a message of kind needs the bot's right to delete other
// people's messages: true for a service kind and for a member kind, false for a bot kind, because a
// plain-member bot can always delete its own messages.
func NeedsDeleteRight(kind string) bool {
	return IsServiceKind(kind) || IsMemberKind(kind)
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

// ValidateRuleDelay checks the delay a rule of kind may store, before any write. An unknown kind is
// ErrUnknownKind. A member kind is checked by value, not by range, so a database edit cannot create
// a grace that no page offers: member:commands accepts 0 only (a stored 0 means On), member:edited
// 30, 60, 300, 900 or 3600 and member:selfdestruct 60, 300, 600, 3600, 21600 or 86400. Every
// other kind keeps the range 0..MaxDelaySecs. A refused delay is ErrDelayOutOfRange.
func ValidateRuleDelay(kind string, secs int) error {
	switch kind {
	case KindMemberCommands:
		return requireDelayIn(secs, 0)
	case KindMemberEdited:
		return requireDelayIn(secs, editedGraceDelays...)
	case KindMemberSelfDestruct:
		return requireDelayIn(secs, selfDestructDelays...)
	}
	if !ValidKind(kind) {
		return ErrUnknownKind
	}
	return validateDelay(secs)
}

// ValidateEntryDelay checks the delay a queue entry of kind may carry, before any write. An unknown
// kind is ErrUnknownKind. member:commands and member:edited entries take 0 only, because such an
// entry is the due-now fallback of an inline delete, never a delayed deletion (the grace of an edit
// is waited out before the entry is made); member:selfdestruct entries take the six self-destruct
// presets; every other kind keeps the range 0..MaxDelaySecs. A refused delay is ErrDelayOutOfRange.
func ValidateEntryDelay(kind string, secs int) error {
	switch kind {
	case KindMemberCommands, KindMemberEdited:
		return requireDelayIn(secs, 0)
	case KindMemberSelfDestruct:
		return requireDelayIn(secs, selfDestructDelays...)
	}
	if !ValidKind(kind) {
		return ErrUnknownKind
	}
	return validateDelay(secs)
}

// requireDelayIn returns ErrDelayOutOfRange unless secs is one of the allowed values.
func requireDelayIn(secs int, allowed ...int) error {
	if slices.Contains(allowed, secs) {
		return nil
	}
	return ErrDelayOutOfRange
}
