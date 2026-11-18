// Package eventmsg stores the custom event messages of a group: one message per event (a manual
// punishment such as ban or mute) and destination (today only the group itself), each with a
// text, at most one media item, URL buttons and a Silent switch. A chat's messages are read as
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
	// EventTimedBan is the reply of /tban, or of /ban with a duration.
	EventTimedBan = "tban"
	// EventKick is the reply of /kick.
	EventKick = "kick"
	// EventMute is the reply of a permanent /mute.
	EventMute = "mute"
	// EventTimedMute is the reply of /tmute, or of /mute with a duration.
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
)

// Events lists every event a message can be stored for, in the order the settings page shows
// them. Phases 22 to 24 append the automatic, lockdown and requirement events.
var Events = []string{
	EventBan,
	EventTimedBan,
	EventKick,
	EventMute,
	EventTimedMute,
	EventUnban,
	EventUnmute,
}

// Dests lists every destination a message can be stored for. Phase 21 appends the private copy.
var Dests = []string{
	DestGroup,
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
