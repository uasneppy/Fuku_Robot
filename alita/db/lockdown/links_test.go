//go:build testtools

package lockdown

import (
	"testing"
	"time"

	"github.com/divkix/Alita_Robot/alita/db/models"
)

// linkedPreRow is preRow for userID with the given invite link.
func linkedPreRow(ld *models.ChatLockdown, userID int64, link string) models.ChatLockdownBurst {
	row := preRow(ld, userID, "P", time.Now().UTC().Add(-time.Minute))
	row.InviteLink = link
	return row
}

// recordLinked records a joiner row for userID that came through link.
func recordLinked(t *testing.T, ld *models.ChatLockdown, userID int64, link string) {
	t.Helper()
	rec := newJoinRecord(ld, userID)
	rec.InviteLink = link
	mustRecord(t, rec)
}

func TestTopInviteLinks(t *testing.T) {
	const (
		linkA = "https://t.me/+AAAA"
		linkB = "https://t.me/+BBBB"
		linkC = "https://t.me/+CCCC"
	)

	t.Run("pre rows and joiner rows are merged and ordered", func(t *testing.T) {
		chat := uniqueLockdownChatID(t)
		cleanupBurstLockdowns(t, chat)
		ld := mustStart(t, chat)
		mustConfirm(t, ld.ID)

		plan := AlertPlan{Burst: []models.ChatLockdownBurst{
			linkedPreRow(ld, 9501, linkA),
			linkedPreRow(ld, 9502, linkA),
			linkedPreRow(ld, 9503, linkB),
			linkedPreRow(ld, 9504, ""),
		}}
		if won, err := BeginAlert(ld.ID, plan); err != nil || !won {
			t.Fatalf("BeginAlert = %v, %v, want true", won, err)
		}
		recordLinked(t, ld, 9511, linkB)
		recordLinked(t, ld, 9512, linkC)
		recordLinked(t, ld, 9513, "")

		got, err := TopInviteLinksFresh(ld.ID)
		if err != nil {
			t.Fatalf("TopInviteLinksFresh: %v", err)
		}
		want := []InviteLinkCount{{Link: linkA, Count: 2}, {Link: linkB, Count: 2}, {Link: linkC, Count: 1}}
		if len(got) != len(want) {
			t.Fatalf("links = %+v, want %+v (count descending, ties by link, empty links ignored)", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("links[%d] = %+v, want %+v (all: %+v)", i, got[i], want[i], got)
			}
		}
	})

	t.Run("a user with a pre row and a joiner row counts once, by the pre row", func(t *testing.T) {
		chat := uniqueLockdownChatID(t)
		cleanupBurstLockdowns(t, chat)
		ld := mustStart(t, chat)
		mustConfirm(t, ld.ID)

		plan := AlertPlan{Burst: []models.ChatLockdownBurst{linkedPreRow(ld, 9601, linkA)}}
		if won, err := BeginAlert(ld.ID, plan); err != nil || !won {
			t.Fatalf("BeginAlert = %v, %v, want true", won, err)
		}
		recordLinked(t, ld, 9601, linkC)

		got, err := TopInviteLinksFresh(ld.ID)
		if err != nil {
			t.Fatalf("TopInviteLinksFresh: %v", err)
		}
		if len(got) != 1 || got[0] != (InviteLinkCount{Link: linkA, Count: 1}) {
			t.Errorf("links = %+v, want only %s once: the pre row decides, the later joiner row is not counted", got, linkA)
		}
	})

	t.Run("no links", func(t *testing.T) {
		chat := uniqueLockdownChatID(t)
		cleanupBurstLockdowns(t, chat)
		ld := mustStart(t, chat)
		mustConfirm(t, ld.ID)
		recordLinked(t, ld, 9701, "")

		got, err := TopInviteLinksFresh(ld.ID)
		if err != nil || len(got) != 0 {
			t.Errorf("TopInviteLinksFresh = %+v, %v, want nothing", got, err)
		}
	})

	t.Run("another lockdown's links are not counted", func(t *testing.T) {
		chat, otherChat := uniqueLockdownChatID(t), uniqueLockdownChatID(t)
		cleanupBurstLockdowns(t, chat, otherChat)
		ld, other := mustStart(t, chat), mustStart(t, otherChat)
		mustConfirm(t, ld.ID)
		mustConfirm(t, other.ID)
		recordLinked(t, ld, 9801, linkA)
		recordLinked(t, other, 9802, linkB)

		got, err := TopInviteLinksFresh(ld.ID)
		if err != nil || len(got) != 1 || got[0].Link != linkA {
			t.Errorf("TopInviteLinksFresh = %+v, %v, want only %s", got, err, linkA)
		}
	})
}
