//go:build testtools

package captchaweb

import (
	"strings"
	"testing"
)

// TestVerifyRouteActionSpared pins C5: an attempt someone else decided while the member is
// still in the group (released, passed, or an action that did not take effect) is answered
// with the neutral invalid and no action, never a removal; the replay marker stores the same,
// and it is not counted as an error.
func TestVerifyRouteActionSpared(t *testing.T) {
	const member = int64(42)
	o := newOutcomeHandler(t)
	o.Backend.Status = AttemptStatus{Live: false, Action: "kick"}
	o.Backend.ExpireResult = ActionSpared
	token, _ := o.issue(member)
	errorsBefore := StatsSnapshot().Errors

	status, answer := o.post(member, token)
	if status != 200 || answer.Result != ResultInvalid || answer.Action != "" {
		t.Fatalf("answer = %d %+v, want 200 invalid with no action", status, answer)
	}
	want := []string{"bot_can_restrict", "attempt", "expire"}
	if got := o.Backend.Calls(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("backend calls = %v, want %v", got, want)
	}
	status, answer = o.post(member, token)
	if status != 200 || answer.Result != ResultInvalid || answer.Action != "" {
		t.Fatalf("replay = %d %+v, want 200 invalid with no action", status, answer)
	}
	if got := o.Backend.Calls(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("the replay reached the backend: %v", got)
	}
	if got := StatsSnapshot().Errors - errorsBefore; got != 0 {
		t.Fatalf("errors counted = %d, want none for a member someone else spared", got)
	}
}
