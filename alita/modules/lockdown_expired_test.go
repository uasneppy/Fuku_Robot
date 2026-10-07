//go:build testtools

package modules

import (
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/models"
)

// setJoinerBanUntil rewrites the end date of a joiner row's ban, so a test can show a
// lockdown that has outlasted its own bans without waiting 330 days.
func setJoinerBanUntil(t *testing.T, rowID uint, until int64) {
	t.Helper()
	if err := db.DB.Model(&models.LockdownJoiner{}).Where("id = ?", rowID).UpdateColumn("ban_until", until).Error; err != nil {
		t.Fatalf("set ban_until of joiner row %d: %v", rowID, err)
	}
}

// A lockdown never ends on its own, but its bans do (330 days after each join). At the
// lift a joiner whose ban ran out is reported as that, and never as someone who had
// "already unbanned them or banned them on purpose".
func TestLockdownLiftReportsExpiredBans(t *testing.T) {
	env := newLockdownEnv(t)
	_, users := env.lockAndBan("Ann", "Bob", "Cid")
	expiredUser, liveUser, unbannedEarly := users[0], users[1], users[2]

	// Ann's ban ended a day ago and she is not in the group (what a lapsed ban leaves).
	past := time.Now().Add(-24 * time.Hour).Unix()
	setJoinerBanUntil(t, env.joinerRow(expiredUser.Id).ID, past)
	env.fake.setMember(env.chat.Id, expiredUser.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusLeft})
	// Cid was unbanned by an admin while the ban still had months to run.
	env.fake.setMember(env.chat.Id, unbannedEarly.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusLeft})

	env.send(env.admin, "/unlockdown")
	env.cycle()

	if unbans := env.unbansOf(expiredUser.Id); len(unbans) != 0 {
		t.Errorf("unbanChatMember calls for the expired ban = %v, want none: nothing was banned any more", unbans)
	}
	wantStates := map[int64]string{
		expiredUser.Id:   models.JoinerStateKept,
		liveUser.Id:      models.JoinerStateUnbanned,
		unbannedEarly.Id: models.JoinerStateKept,
	}
	for id, want := range wantStates {
		if row := env.joinerRow(id); row.State != want {
			t.Errorf("joiner %d state = %q, want %q", id, row.State, want)
		}
	}
	tallies := env.repliesWith(staffMarker("lockdown_lift_tally"))
	if len(tallies) != 1 {
		t.Fatalf("tally messages = %d, want 1", len(tallies))
	}
	tally := tallies[0]
	for _, want := range []string{
		staffMarker("lockdown_lift_tally") + " 1",
		staffMarker("lockdown_lift_tally_kept") + " 1",
		staffMarker("lockdown_lift_tally_expired") + " 1",
	} {
		if !strings.Contains(tally, want) {
			t.Errorf("tally %q does not contain %q", tally, want)
		}
	}
}
