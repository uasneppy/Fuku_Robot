//go:build testtools

package modules

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/lockdown"
	"github.com/divkix/Alita_Robot/alita/db/models"
)

// startRow runs startLockdownRow for the env's chat the way an automatic join-surge
// trigger would: the bot is the starter and the trigger numbers are 15 joins in 60 s.
func (e *lockdownEnv) startRow(chat gotgbot.Chat) lockdownStartResult {
	e.t.Helper()
	return startLockdownRow(context.Background(), e.bot, lockdownStartParams{
		Chat:          &chat,
		TriggerKind:   models.LockdownTriggerJoinSurge,
		StarterID:     staffTestBotID,
		StarterName:   "Alita",
		TriggerCount:  15,
		TriggerWindow: 60,
	})
}

// wantStartOutcome fails unless res carries the wanted outcome.
func wantStartOutcome(t *testing.T, res lockdownStartResult, want lockdownStartOutcome) {
	t.Helper()
	if res.Outcome != want {
		t.Fatalf("Outcome = %d, want %d", res.Outcome, want)
	}
}

// lockdownRowCount counts every lockdown row of the chat, whatever its state.
func lockdownRowCount(t *testing.T, chatID int64) int64 {
	t.Helper()
	var rows int64
	if err := db.DB.Model(&models.ChatLockdown{}).Where("chat_id = ?", chatID).Count(&rows).Error; err != nil {
		t.Fatalf("count lockdown rows: %v", err)
	}
	return rows
}

func TestLockdownStartRow(t *testing.T) {
	t.Run("not a supergroup", func(t *testing.T) {
		env := newLockdownEnv(t)
		chat := env.chat
		chat.Type = "group"

		res := env.startRow(chat)

		wantStartOutcome(t, res, lockdownStartNotSupergroup)
		for _, method := range []string{"getChatMember", "getChat", "setChatPermissions"} {
			if calls := env.calls(method); len(calls) != 0 {
				t.Errorf("%s calls = %d, want 0: a basic group is refused before any Telegram call", method, len(calls))
			}
		}
		if rows := lockdownRowCount(t, env.chat.Id); rows != 0 {
			t.Errorf("lockdown rows = %d, want 0", rows)
		}
	})

	t.Run("bot check fails", func(t *testing.T) {
		env := newLockdownEnv(t)
		env.fake.script("getChatMember", env.chat.Id, staffFakeError(500, "Internal Server Error"))

		res := env.startRow(env.chat)

		wantStartOutcome(t, res, lockdownStartBotCheckFailed)
		if calls := env.calls("setChatPermissions"); len(calls) != 0 {
			t.Errorf("setChatPermissions calls = %d, want 0", len(calls))
		}
		if rows := lockdownRowCount(t, env.chat.Id); rows != 0 {
			t.Errorf("lockdown rows = %d, want 0", rows)
		}
	})

	t.Run("bot cannot restrict", func(t *testing.T) {
		env := newLockdownEnv(t)
		env.fake.setBotRole(env.chat.Id, staffRoleAdminNoRestrict)

		res := env.startRow(env.chat)

		wantStartOutcome(t, res, lockdownStartBotCannotRestrict)
		if calls := env.calls("getChat"); len(calls) != 0 {
			t.Errorf("getChat calls = %d, want 0: the bot's rights are judged before the permissions are read", len(calls))
		}
		if rows := lockdownRowCount(t, env.chat.Id); rows != 0 {
			t.Errorf("lockdown rows = %d, want 0", rows)
		}
	})

	t.Run("permissions unreadable", func(t *testing.T) {
		env := newLockdownEnv(t)
		env.fake.clearChatPerms(env.chat.Id)

		res := env.startRow(env.chat)

		wantStartOutcome(t, res, lockdownStartPermissionsUnreadable)
		if calls := env.calls("setChatPermissions"); len(calls) != 0 {
			t.Errorf("setChatPermissions calls = %d, want 0", len(calls))
		}
		if rows := lockdownRowCount(t, env.chat.Id); rows != 0 {
			t.Errorf("lockdown rows = %d, want 0", rows)
		}
	})

	t.Run("lock call refused", func(t *testing.T) {
		env := newLockdownEnv(t)
		env.fake.script("setChatPermissions", env.chat.Id, staffFakeError(400, "Bad Request: not enough rights"))

		res := env.startRow(env.chat)

		wantStartOutcome(t, res, lockdownStartLockFailed)
		if rows := lockdownRowCount(t, env.chat.Id); rows != 0 {
			t.Errorf("lockdown rows = %d, want 0: a refused lock leaves no row", rows)
		}
		if got := env.fake.chatPermsRaw(env.chat.Id); got != lockdownTestPrePermissions {
			t.Errorf("group permissions = %q, want them unchanged", got)
		}
	})

	t.Run("lock answer lost", func(t *testing.T) {
		env := newLockdownEnv(t)
		env.fake.setFailAfterSetPermissions(errors.New("connection reset by peer"))

		res := env.startRow(env.chat)

		wantStartOutcome(t, res, lockdownStartLockUnknown)
		row, err := lockdown.GetActiveFresh(env.chat.Id)
		if err != nil || row == nil {
			t.Fatalf("GetActiveFresh = %v, %v, want the unconfirmed row kept", row, err)
		}
		if row.LockedAt != nil {
			t.Errorf("LockedAt = %v, want nil: Telegram never confirmed the lock", row.LockedAt)
		}
		if row.PrePermissions != lockdownTestPrePermissions {
			t.Errorf("PrePermissions = %q, want the snapshot kept for the restore", row.PrePermissions)
		}
	})

	t.Run("already active", func(t *testing.T) {
		env := newLockdownEnv(t)
		env.send(env.admin, "/lockdown first")
		first, err := lockdown.GetActiveFresh(env.chat.Id)
		if err != nil || first == nil {
			t.Fatalf("GetActiveFresh = %v, %v, want the admin's lockdown", first, err)
		}

		res := env.startRow(env.chat)

		wantStartOutcome(t, res, lockdownStartAlreadyActive)
		if res.Row == nil || res.Row.ID != first.ID {
			t.Fatalf("Row = %+v, want the lockdown that already runs (id %d)", res.Row, first.ID)
		}
		if calls := env.calls("setChatPermissions"); len(calls) != 1 {
			t.Errorf("setChatPermissions calls = %d, want 1: only the admin's lock", len(calls))
		}
		if rows := lockdownRowCount(t, env.chat.Id); rows != 1 {
			t.Errorf("lockdown rows = %d, want 1", rows)
		}
	})

	t.Run("started", func(t *testing.T) {
		env := newLockdownEnv(t)
		env.fake.setBotMember(env.chat.Id, lockdownBotAdminJSON(true, true, true))

		res := env.startRow(env.chat)

		wantStartOutcome(t, res, lockdownStartStarted)
		if res.Row == nil {
			t.Fatal("Row = nil, want the started lockdown")
		}
		row, err := lockdown.GetActiveFresh(env.chat.Id)
		if err != nil || row == nil {
			t.Fatalf("GetActiveFresh = %v, %v, want the started lockdown", row, err)
		}
		if row.ID != res.Row.ID {
			t.Errorf("stored row id = %d, want the returned row %d", row.ID, res.Row.ID)
		}
		if row.LockedAt == nil {
			t.Error("LockedAt = nil, want the lock confirmed")
		}
		if row.TriggerKind != models.LockdownTriggerJoinSurge || row.TriggerCount != 15 ||
			row.TriggerWindow != 60 || row.TriggerPercent != 0 {
			t.Errorf("trigger = %q %d/%ds/%d%%, want join_surge 15/60s/0%%",
				row.TriggerKind, row.TriggerCount, row.TriggerWindow, row.TriggerPercent)
		}
		if row.StartedBy != staffTestBotID || row.StartedByName != "Alita" {
			t.Errorf("started by %d %q, want the bot", row.StartedBy, row.StartedByName)
		}
		if row.Reason != "" {
			t.Errorf("Reason = %q, want empty: the numbers are columns, never part of the reason", row.Reason)
		}
		if row.PrePermissions != lockdownTestPrePermissions {
			t.Errorf("PrePermissions = %q, want the raw getChat permissions", row.PrePermissions)
		}
		if row.LockedPermissions != lockdownLockedPermissions {
			t.Errorf("LockedPermissions = %q, want the locked set", row.LockedPermissions)
		}
		if !res.BotMember.CanDeleteMessages || !res.BotMember.CanInviteUsers {
			t.Errorf("BotMember = %+v, want the delete and invite rights carried", res.BotMember)
		}
		if calls := env.calls("setChatPermissions"); len(calls) != 1 {
			t.Errorf("setChatPermissions calls = %d, want 1", len(calls))
		}
	})

	t.Run("concurrent", func(t *testing.T) {
		env := newLockdownEnv(t)

		starts := []lockdownStartParams{
			{
				Chat: &env.chat, TriggerKind: models.LockdownTriggerJoinSurge,
				StarterID: staffTestBotID, StarterName: "Alita", TriggerCount: 15, TriggerWindow: 60,
			},
			{
				Chat: &env.chat, TriggerKind: models.LockdownTriggerFlood,
				StarterID: staffTestBotID, StarterName: "Alita", TriggerCount: 41, TriggerWindow: 30, TriggerPercent: 87,
			},
		}
		results := make([]lockdownStartResult, len(starts))
		var wg sync.WaitGroup
		for i := range starts {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				results[i] = startLockdownRow(context.Background(), env.bot, starts[i])
			}(i)
		}
		wg.Wait()

		started, already := 0, 0
		for _, res := range results {
			switch res.Outcome {
			case lockdownStartStarted:
				started++
			case lockdownStartAlreadyActive:
				already++
			}
		}
		if started != 1 || already != 1 {
			t.Fatalf("outcomes = %d started and %d already active, want one of each", started, already)
		}
		if rows := lockdownRowCount(t, env.chat.Id); rows != 1 {
			t.Errorf("lockdown rows = %d, want 1", rows)
		}
		if calls := env.calls("setChatPermissions"); len(calls) != 1 {
			t.Errorf("setChatPermissions calls = %d, want 1", len(calls))
		}
	})

	t.Run("manual row", func(t *testing.T) {
		env := newLockdownEnv(t)

		env.send(env.admin, "/lockdown raid wave")

		env.wantReplyHas(staffMarker("lockdown_started"))
		row, err := lockdown.GetActiveFresh(env.chat.Id)
		if err != nil || row == nil {
			t.Fatalf("GetActiveFresh = %v, %v, want the admin's lockdown", row, err)
		}
		if row.TriggerKind != models.LockdownTriggerManual {
			t.Errorf("TriggerKind = %q, want manual", row.TriggerKind)
		}
		if row.TriggerCount != 0 || row.TriggerWindow != 0 || row.TriggerPercent != 0 {
			t.Errorf("trigger numbers = %d/%d/%d, want zeros for a manual lockdown",
				row.TriggerCount, row.TriggerWindow, row.TriggerPercent)
		}
		if row.StartedBy != env.admin.Id {
			t.Errorf("StartedBy = %d, want the admin %d", row.StartedBy, env.admin.Id)
		}
	})
}
