//go:build testtools

package deletion

import "strings"

// botPrefix is the prefix of every bot kind. Production code never asks "is this a bot kind":
// it matches kinds exactly, so the prefix and the helpers below exist for tests only.
const botPrefix = "bot:"

// Kinds returns every kind in page order. The slice is a copy. It is a test helper: no
// production code lists the kinds (the registry is read through ValidKind).
func Kinds() []string {
	return append([]string(nil), kinds...)
}

// IsBotKind reports whether kind is a "bot:" kind. It is a test helper.
func IsBotKind(kind string) bool {
	return strings.HasPrefix(kind, botPrefix)
}

// SetApplyHookForTest installs fn as the hook that ApplyJoinLeftRule and
// ApplyCleanServiceFlag call inside their transaction, between the flag write and the rule
// writes, so a test can fail the second step without altering a table. It returns a
// function that puts the previous hook back.
func SetApplyHookForTest(fn func() error) (restore func()) {
	previous := applyHook
	applyHook = fn
	return func() { applyHook = previous }
}
