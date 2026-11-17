//go:build testtools

package modules

import (
	"fmt"
	"sync"
	"testing"

	"github.com/uasneppy/fuku_robot/fuku/db/lockdown"
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
