//go:build testtools

package modules

import (
	"fmt"
	"sync"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db/lockdown"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// TestLockdownStartLiftedDuringLock covers a /unlockdown that restores and records the
// lift of the unconfirmed row while the lock call is still on its way: the lock lands
// after the restore, so the group would stay locked with no active lockdown. The start
// must see that its confirmation lost, put the stored permissions back and report the
// lift, never a started lockdown.
func TestLockdownStartLiftedDuringLock(t *testing.T) {
	env := newLockdownEnv(t)

	var once sync.Once
	var hookErr error
	env.fake.setOnSetPermissions(func(chatID int64) {
		once.Do(func() {
			row, err := lockdown.GetActiveFresh(chatID)
			if err != nil || row == nil {
				hookErr = fmt.Errorf("no active row at the lock call: %v", err)
				return
			}
			// The lift restores the stored permissions first, then records itself.
			env.fake.setChatPermsRaw(chatID, row.PrePermissions)
			if won, err := lockdown.BeginLift(row.ID, env.admin.Id, "Admin", false); err != nil || !won {
				hookErr = fmt.Errorf("BeginLift = %v, %v", won, err)
			}
		})
	})

	res := env.startRow(env.chat)
	if hookErr != nil {
		t.Fatal(hookErr)
	}

	wantStartOutcome(t, res, lockdownStartLiftedMeanwhile)
	same, err := samePermissions(env.fake.chatPermsRaw(env.chat.Id), lockdownTestPrePermissions)
	if err != nil {
		t.Fatalf("compare permissions: %v", err)
	}
	if !same {
		t.Errorf("chat permissions = %s, want the stored pre-lockdown set put back", env.fake.chatPermsRaw(env.chat.Id))
	}
	if active, err := lockdown.GetActiveFresh(env.chat.Id); err != nil || active != nil {
		t.Errorf("active lockdown = %v, %v, want none", active, err)
	}
}

// TestLockdownLiftChecksBansWhoseAnswerFailed covers a ban call that answered a 5xx
// but may have taken effect: the row is left pending with an attempt counted, or ends
// ban_failed after three. The lift must still look at those joiners live and remove
// a ban that carries the row's marker, and leave a joiner whose ban never landed
// out of the tally.
func TestLockdownLiftChecksBansWhoseAnswerFailed(t *testing.T) {
	serverError := staffFakeError(500, "Internal Server Error")
	cases := []struct {
		name      string
		failures  int
		landed    bool
		wantState string
	}{
		{"pending after one failed answer, ban in place", 1, true, models.JoinerStateUnbanned},
		{"ban_failed after three failed answers, ban in place", 3, true, models.JoinerStateUnbanned},
		{"ban_failed and the ban never landed", 3, false, models.JoinerStateCancelled},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			env := newLockdownEnv(t)
			env.lockAsAdmin("raid")
			raider := env.newJoiner("Raider")
			env.join(raider, raider, "https://t.me/+abc")
			for i := 0; i < tt.failures; i++ {
				env.fake.script("banChatMember", env.chat.Id, serverError)
				env.cycle()
			}
			row := env.joinerRow(raider.Id)
			wantBefore := models.JoinerStatePending
			if tt.failures >= lockdownMaxAttempts {
				wantBefore = models.JoinerStateBanFailed
			}
			if row.State != wantBefore {
				t.Fatalf("setup: joiner is %q, want %q", row.State, wantBefore)
			}
			if tt.landed {
				// The ban took effect although its answer was lost.
				env.fake.setMember(env.chat.Id, raider.Id, staffFakeMember{
					Status:    gotgbot.ChatMemberStatusKicked,
					UntilDate: row.BanUntil,
				})
			}

			env.send(env.admin, "/unlockdown")
			env.cycle()

			if got := env.joinerRow(raider.Id); got.State != tt.wantState {
				t.Errorf("joiner state after the lift = %q, want %q", got.State, tt.wantState)
			}
			unbans := env.unbansOf(raider.Id)
			if tt.landed && len(unbans) != 1 {
				t.Errorf("unbanChatMember calls = %d, want 1: the lockdown's ban must be lifted", len(unbans))
			}
			if !tt.landed && len(unbans) != 0 {
				t.Errorf("unbanChatMember calls = %d, want 0: there was no ban to lift", len(unbans))
			}
			if !tt.landed {
				if got := env.messagesWith(staffMarker("lockdown_lift_tally_kept")); got != 0 {
					t.Errorf("tally messages with a kept line = %d, want 0", got)
				}
			}
		})
	}
}
