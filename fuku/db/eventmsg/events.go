// Package eventmsg stores the custom event messages of a group: one message per event (a manual
// punishment such as ban or mute) and destination (the group itself, or the private copy sent to
// the punished person), each with a text, at most one media item, URL buttons and a Silent switch,
// plus the group's two options of the private copies. A chat's messages and options are read as
// one struct per chat through the cache key event_messages:<chat>, which is listed in skipLocal,
// and every write is a column-targeted upsert that drops that key.
//
// Events and destinations are validated here, never by a database rule, so a later phase adds
// an event or a destination with no migration.
package eventmsg

import (
	"errors"
	"slices"
)

// The events a message can be stored for.
const (
	// EventBan is the reply of a permanent /ban.
	EventBan = "ban"
	// EventTimedBan is the reply of /tban.
	EventTimedBan = "tban"
	// EventKick is the reply of /kick.
	EventKick = "kick"
	// EventMute is the reply of a permanent /mute.
	EventMute = "mute"
	// EventTimedMute is the reply of /tmute.
	EventTimedMute = "tmute"
	// EventUnban is the reply of /unban.
	EventUnban = "unban"
	// EventUnmute is the reply of /unmute.
	EventUnmute = "unmute"
)

// The destinations a message can be stored for.
const (
	// DestGroup is the message posted in the group itself.
	DestGroup = "group"
	// DestPM is the private copy of the event sent to the punished person in the bot's private
	// chat. It is off until its Silent switch is written false (see DefaultSilent).
	DestPM = "pm"
)

// Events lists every event a message can be stored for, in the order the settings page shows
// them. Phases 23 to 25 append the automatic, lockdown and requirement events.
var Events = []string{
	EventBan,
	EventTimedBan,
	EventKick,
	EventMute,
	EventTimedMute,
	EventUnban,
	EventUnmute,
}

// Dests lists every destination a message can be stored for.
var Dests = []string{
	DestGroup,
	DestPM,
}

var (
	// ErrUnknownEvent is returned, before any write, for an event that is not in Events.
	ErrUnknownEvent = errors.New("unknown event message event")
	// ErrUnknownDest is returned, before any write, for a destination that is not in Dests.
	ErrUnknownDest = errors.New("unknown event message destination")
)

// ValidEvent reports whether event is one of Events.
func ValidEvent(event string) bool {
	return slices.Contains(Events, event)
}

// ValidDest reports whether dest is one of Dests.
func ValidDest(dest string) bool {
	return slices.Contains(Dests, dest)
}

// DefaultSilent is the Silent switch of a destination that has no stored row. Silent means
// nothing is sent to that destination: the group message is sent by default (false), and a
// private copy is off until its switch is written (true). A part writer that inserts a new row
// stores this value too, so storing a text alone never turns a private copy on.
func DefaultSilent(dest string) bool {
	return dest == DestPM
}

// validate checks an event and a destination and returns the matching error for the first that
// is unknown.
func validate(event, dest string) error {
	if !ValidEvent(event) {
		return ErrUnknownEvent
	}
	if !ValidDest(dest) {
		return ErrUnknownDest
	}
	return nil
}
