//go:build testtools

package deletion

// SetApplyHookForTest installs fn as the hook that ApplyJoinLeftRule and
// ApplyCleanServiceFlag call inside their transaction, between the flag write and the rule
// writes, so a test can fail the second step without altering a table. It returns a
// function that puts the previous hook back.
func SetApplyHookForTest(fn func() error) (restore func()) {
	previous := applyHook
	applyHook = fn
	return func() { applyHook = previous }
}
