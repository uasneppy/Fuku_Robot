//go:build testtools

package modules

import (
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
)

// TestDeletionEffectiveRule pins what a stored rule means for a message: the delay it gets and
// whether a rule exists at all. The subtests are the cases of the kinds that do not share the
// /cleanservice flag and, from the third task of plan 16-02, of the two that do.
func TestDeletionEffectiveRule(t *testing.T) {
	t.Run("stored kinds", func(t *testing.T) {
		rules := &deletion.Rules{Delays: map[string]int{deletion.KindPinned: 600, deletion.KindTitle: 0, deletion.KindBotRules: 3600}}
		for _, c := range []struct {
			name  string
			kind  string
			delay int
			on    bool
		}{
			{"a stored delay", deletion.KindPinned, 600, true},
			{"a stored zero is on with delay zero", deletion.KindTitle, 0, true},
			{"a bot kind", deletion.KindBotRules, 3600, true},
			{"a service kind with no row", deletion.KindPhoto, 0, false},
			{"a bot kind with no row", deletion.KindBotWelcome, 0, false},
		} {
			// The flag argument is irrelevant to a kind that does not share it.
			for _, flag := range []bool{false, true} {
				delay, on := deletionEffectiveDelay(c.kind, rules, flag)
				if delay != c.delay || on != c.on {
					t.Errorf("%s (flag %v): got (%d, %v), want (%d, %v)", c.name, flag, delay, on, c.delay, c.on)
				}
			}
		}
		if delay, on := deletionEffectiveDelay(deletion.KindPinned, nil, true); delay != 0 || on {
			t.Errorf("nil rules: got (%d, %v), want no rule", delay, on)
		}
	})

	t.Run("a pinned notice gets the stored delay", func(t *testing.T) {
		msg := &gotgbot.Message{PinnedMessage: gotgbot.Message{MessageId: 7}}
		row, ok := deletionServiceKindOf(msg)
		if !ok || row.Kind != deletion.KindPinned {
			t.Fatalf("a message holding a pinned message routed to %+v (%v), want %s", row, ok, deletion.KindPinned)
		}
	})
}
