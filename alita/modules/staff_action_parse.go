package modules

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/PaulSonOfLars/gotgbot/v2"
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

// staffActionRequest is a parsed staff command.
type staffActionRequest struct {
	Kind   staffActionKind
	Target staffTargetRef
	Reason string
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
	req.Reason = capStaffReason(reason)
	return req, staffParseOK
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
