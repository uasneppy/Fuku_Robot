//go:build testtools

package modules

import (
	"testing"
	"time"

	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
)

// TestMemberDeletePinnedAnyKind holds I2 (D-11): a pinned message stays whatever kind of row
// queued it. A chat with only a bot rule (no self-destruct rule) whose rules message an admin
// pinned loses that message's pending row, through the real dispatcher.
func TestMemberDeletePinnedAnyKind(t *testing.T) {
	env := newMemberDeletionEnv(t)
	resetMemberDeleteState()
	t.Cleanup(resetMemberDeleteState)
	env.storeMemberRule(deletion.KindBotRules, 3600)
	due := env.now.Add(time.Hour)
	pinned := seedDeletionRow(t, env.chat.Id, 991201, due, env.now, deletion.KindBotRules)
	kept := seedDeletionRow(t, env.chat.Id, 991202, due, env.now, deletion.KindBotRules)

	env.sendPinOf(env.chat, &env.admin, pinned.MessageID)

	rows := env.queuedNoWait(env.chat.Id)
	if len(rows) != 1 || rows[0].MessageID != kept.MessageID {
		t.Fatalf("rows left = %+v, want only the unpinned message %d", rows, kept.MessageID)
	}
}
