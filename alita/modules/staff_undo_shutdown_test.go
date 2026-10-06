//go:build testtools

package modules

import (
	"context"
	"strings"
	"testing"
	"time"
)

// resetStaffActionsContextAfter puts a live run context back once the test is over,
// because StopStaffActions cancels the shared one.
func resetStaffActionsContextAfter(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		staffActionsMu.Lock()
		defer staffActionsMu.Unlock()
		staffActionsCtx, staffActionsCancel = context.WithCancel(context.Background())
	})
}

func TestStaffUndoShutdownWindow(t *testing.T) {
	env := newStaffActionEnv(t, 2)
	bob := env.undoPresser("Bob")
	resetStaffActionsContextAfter(t)
	action, msgID := env.undoFinishedBan()
	token, cardMsgID := env.askUndo(bob, msgID, action.ID)

	// The delay holds the Confirm inside its pending-summary edit, which comes right
	// after the claim and before the run is started.
	env.fake.setDelay("editMessageText", env.staffChat, 500*time.Millisecond)
	confirmDone := make(chan struct{})
	update := env.callbackUpdate(bob, undoConfirmCode, token, cardMsgID)
	go func() {
		defer close(confirmDone)
		if err := env.dispatcher.ProcessUpdate(env.bot, update, nil); err != nil {
			t.Errorf("ProcessUpdate(%d) error = %v", update.UpdateId, err)
		}
	}()

	deadline := time.Now().Add(5 * time.Second)
	for undoStartedAt(t, action.ID) == nil {
		if time.Now().After(deadline) {
			t.Fatal("the undo Confirm did not claim the record within 5s")
		}
		time.Sleep(5 * time.Millisecond)
	}

	StopStaffActions()

	// The shutdown waited for the Confirm that had already claimed, so its run has
	// given the claim back by now.
	if started := undoStartedAt(t, action.ID); started != nil {
		t.Errorf("record still claimed at %v after StopStaffActions returned, want the claim given back", started)
	}
	if got := env.unbansIn(); got != 0 {
		t.Errorf("a run cut off by the shutdown made %d unban call(s), want none", got)
	}
	if edits := env.edits(env.staffChat, cardMsgID); len(edits) == 0 {
		t.Error("the undo card was never edited")
	} else {
		text := env.lastEditText(env.staffChat, cardMsgID)
		for _, key := range []string{"staff_act_fail_interrupted", "staff_undo_released_note"} {
			if !strings.Contains(text, staffMarker(key)) {
				t.Errorf("final undo card lacks %s:\n%s", key, text)
			}
		}
		if strings.Contains(text, "⏳") {
			t.Errorf("final undo card still has a pending line:\n%s", text)
		}
	}
	if holder := targetLockHolder(t, staffTestTarget); holder != "" {
		t.Errorf("target lock holder after the shutdown = %q, want the key gone", holder)
	}

	select {
	case <-confirmDone:
	case <-time.After(5 * time.Second):
		t.Error("the undo Confirm did not return within 5s")
	}
	env.waitRuns()
}
