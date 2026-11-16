//go:build testtools

package modules

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
)

// TestMemberDeletePinnedNeedsRule holds IN-03: a pin in a chat with no self-destruct rule costs no
// database write, a chat with the rule drops, and an unreadable rule keeps the old behaviour.
func TestMemberDeletePinnedNeedsRule(t *testing.T) {
	t.Run("no rule, no database call", func(t *testing.T) {
		env := newMemberDeletionEnv(t)
		var calls atomic.Int32
		mreqSwap(t, &memberDeleteDropPinned,
			func(context.Context, int64, int64, string) (int64, error) {
				calls.Add(1)
				return 0, nil
			})
		member := env.member()
		env.sendPinOf(env.chat, &member, 881901)
		memberDeletePinned(env.chat.Id, 881902)
		if got := calls.Load(); got != 0 {
			t.Fatalf("drop calls for a chat without the rule = %d, want 0", got)
		}
		if got := memberPinnedSize(); got != 0 {
			t.Fatalf("pinned notes for a chat without the rule = %d, want 0", got)
		}
	})

	t.Run("a rule on drops", func(t *testing.T) {
		env := newSelfDestructEnv(t, memberSelfDestructRuleDelay)
		var calls atomic.Int32
		mreqSwap(t, &memberDeleteDropPinned,
			func(context.Context, int64, int64, string) (int64, error) {
				calls.Add(1)
				return 0, nil
			})
		member := env.member()
		env.sendPinOf(env.chat, &member, 881911)
		if got := calls.Load(); got != 1 {
			t.Fatalf("drop calls for a chat with the rule = %d, want 1", got)
		}
	})

	t.Run("an unreadable rule keeps the old behaviour", func(t *testing.T) {
		env := newMemberDeletionEnv(t)
		var calls atomic.Int32
		mreqSwap(t, &memberDeleteDropPinned,
			func(context.Context, int64, int64, string) (int64, error) {
				calls.Add(1)
				return 0, nil
			})
		mreqSwap(t, &deletionRulesLookup,
			func(context.Context, int64) (*deletion.Rules, error) { return nil, errors.New("rules unreadable") })
		memberDeletePinned(env.chat.Id, 881921)
		if got := calls.Load(); got != 1 {
			t.Fatalf("drop calls for an unreadable rule = %d, want 1", got)
		}
	})
}
