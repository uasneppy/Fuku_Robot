package modules

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/utils/extraction"
)

// staffReasonMaxRunes caps the reason of a staff action. The card shows exactly
// the reason that will be used, so a cut is visible before Confirm.
const staffReasonMaxRunes = 200

// staffNumericTargetRe matches a numeric user ID: digits only, so a negative chat
// ID or a word never passes for a user.
var staffNumericTargetRe = regexp.MustCompile(`^[0-9]{1,16}$`)

// staffTargetRef is who a staff action is aimed at. Only UserID is filled for a
// numeric ID; later plans add @username and text_mention targets.
type staffTargetRef struct {
	UserID      int64
	Username    string
	MentionName string
}

// staffDurationMode says whether a staff command reads a duration after the target.
type staffDurationMode int

const (
	// staffDurationNone never reads a duration: kick, unban and unmute.
	staffDurationNone staffDurationMode = iota
	// staffDurationOptional reads a duration when the next field is one: ban and mute.
	staffDurationOptional
	// staffDurationRequired needs a duration right after the target: tban and tmute.
	staffDurationRequired
)

// staffActionRequest is a parsed staff command.
type staffActionRequest struct {
	Kind   staffActionKind
	Target staffTargetRef
	Reason string
	// DurationSec is the length in seconds; 0 means permanent.
	DurationSec int64
	// DurationAmount and DurationUnit are the amount and unit exactly as typed.
	DurationAmount int64
	DurationUnit   string
	// OverLimit is set when the typed duration was longer than 366 days; the action
	// is then permanent.
	OverLimit bool
}

// staffParseResult says whether a staff command could be parsed.
type staffParseResult int

const (
	// staffParseOK means Kind, Target and Reason are filled.
	staffParseOK staffParseResult = iota
	// staffParseNoTarget means the command had no argument at all.
	staffParseNoTarget
	// staffParseBadTarget means the first argument is not a usable target.
	staffParseBadTarget
	// staffParseNeedDuration means a command that requires a duration had none.
	staffParseNeedDuration
	// staffParseBadDuration means the duration had an amount of zero.
	staffParseBadDuration
)

// staffSplitField returns the first whitespace-separated field of s and what
// follows it with leading whitespace removed.
func staffSplitField(s string) (field, rest string) {
	s = strings.TrimLeftFunc(s, unicode.IsSpace)
	i := strings.IndexFunc(s, unicode.IsSpace)
	if i < 0 {
		return s, ""
	}
	return s[:i], strings.TrimLeftFunc(s[i:], unicode.IsSpace)
}

// parseStaffActionArgs reads a staff command: the command word (with or without
// @bot) is dropped, the next field must be the target, and everything after it is
// the reason. The target is always the first argument, so "/ban spam 123" is
// refused instead of guessed at.
func parseStaffActionArgs(msg *gotgbot.Message, spec staffCommandSpec) (staffActionRequest, staffParseResult) {
	req := staffActionRequest{Kind: spec.Kind}
	if msg == nil {
		return req, staffParseNoTarget
	}
	_, rest := staffSplitField(msg.GetText())
	if rest == "" {
		return req, staffParseNoTarget
	}
	targetField, reason := staffSplitField(rest)
	if !staffNumericTargetRe.MatchString(targetField) {
		return req, staffParseBadTarget
	}
	id, err := strconv.ParseInt(targetField, 10, 64)
	if err != nil || id <= 0 {
		return req, staffParseBadTarget
	}
	req.Target = staffTargetRef{UserID: id}

	if spec.Duration != staffDurationNone {
		var result staffParseResult
		reason, result = readStaffDuration(&req, reason, spec.Duration)
		if result != staffParseOK {
			return req, result
		}
	}
	req.Reason = capStaffReason(reason)
	return req, staffParseOK
}

// readStaffDuration looks at the first field after the target and fills the
// duration of req when it is one, returning what is left as the reason. The
// grammar is extraction.ParseDurationToken, shared with the per-group /tban and
// /tmute. Under staffDurationOptional a field that is not a duration stays part of
// the reason, so the card shows "permanent" and a misparse is visible before
// Confirm; under staffDurationRequired it is an error.
func readStaffDuration(req *staffActionRequest, rest string, mode staffDurationMode) (string, staffParseResult) {
	field, after := staffSplitField(rest)
	spec, matched, err := extraction.ParseDurationToken(field)
	switch {
	case !matched:
		if mode == staffDurationRequired {
			return rest, staffParseNeedDuration
		}
		return rest, staffParseOK
	case errors.Is(err, extraction.ErrDurationInvalid):
		return rest, staffParseBadDuration
	case errors.Is(err, extraction.ErrDurationTooLong):
		// Longer than Telegram's 366-day window: permanent, with what was typed kept
		// for the card. Telegram never gets a clamped value.
		req.OverLimit = true
		req.DurationAmount = spec.Amount
		req.DurationUnit = string(spec.Unit)
		return after, staffParseOK
	case err != nil:
		return rest, staffParseBadDuration
	}
	req.DurationSec = spec.Seconds
	req.DurationAmount = spec.Amount
	req.DurationUnit = string(spec.Unit)
	return after, staffParseOK
}

// capStaffReason trims a reason and cuts it to staffReasonMaxRunes runes, adding
// an ellipsis when it was cut. It counts runes, not bytes, and does no escaping:
// that happens when the reason is rendered.
func capStaffReason(s string) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= staffReasonMaxRunes {
		return s
	}
	return string([]rune(s)[:staffReasonMaxRunes]) + "…"
}
