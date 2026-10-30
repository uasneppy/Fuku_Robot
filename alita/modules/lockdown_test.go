//go:build testtools

package modules

import (
	"encoding/json"
	"sync"
	"testing"

	"github.com/divkix/Alita_Robot/alita/db/lockdown"
	"github.com/divkix/Alita_Robot/alita/db/models"
)

func TestLockdownCommandStarts(t *testing.T) {
	env := newLockdownEnv(t)

	env.send(env.admin, "/lockdown raid from spam bots")

	row, err := lockdown.GetActiveFresh(env.chat.Id)
	if err != nil {
		t.Fatalf("GetActiveFresh error = %v", err)
	}
	if row == nil {
		t.Fatal("no active lockdown row after /lockdown")
	}
	if row.State != models.LockdownStateActive {
		t.Errorf("State = %q, want active", row.State)
	}
	if row.TriggerKind != models.LockdownTriggerManual {
		t.Errorf("TriggerKind = %q, want manual", row.TriggerKind)
	}
	if row.LockedAt == nil {
		t.Error("LockedAt is nil, want the time Telegram confirmed the lock")
	}
	if row.StartedBy != env.admin.Id {
		t.Errorf("StartedBy = %d, want %d", row.StartedBy, env.admin.Id)
	}
	if row.StartedByName != "Ad<b>min" {
		t.Errorf("StartedByName = %q, want the raw name", row.StartedByName)
	}
	if row.Reason != "raid from spam bots" {
		t.Errorf("Reason = %q", row.Reason)
	}
	if row.PrePermissions != lockdownTestPrePermissions {
		t.Errorf("PrePermissions = %q, want the getChat permissions byte for byte", row.PrePermissions)
	}
	if row.LockedPermissions != lockdownLockedPermissions {
		t.Errorf("LockedPermissions = %q, want the locked set", row.LockedPermissions)
	}

	env.wantReplyHas(
		staffMarker("lockdown_started"),
		staffMarker("lockdown_note_approved_muted"),
		staffMarker("lockdown_started_by"),
		staffMarker("lockdown_reason_line"),
		"Ad&lt;b&gt;min",
		"raid from spam bots",
	)
}

func TestLockdownLockSendsLockedSet(t *testing.T) {
	env := newLockdownEnv(t)

	var mu sync.Mutex
	var sawRowAtLockCall bool
	env.fake.setOnSetPermissions(func(chatID int64) {
		row, err := lockdown.GetActiveFresh(chatID)
		mu.Lock()
		defer mu.Unlock()
		sawRowAtLockCall = err == nil && row != nil
	})

	env.send(env.admin, "/lockdown")

	sets := env.calls("setChatPermissions")
	if len(sets) != 1 {
		t.Fatalf("setChatPermissions calls to the chat = %d, want 1", len(sets))
	}
	if all := env.fake.callsFor("setChatPermissions"); len(all) != 1 {
		t.Errorf("setChatPermissions calls in any chat = %d, want 1", len(all))
	}
	if got, _ := sets[0].Params["use_independent_chat_permissions"].(bool); !got {
		t.Errorf("use_independent_chat_permissions = %v, want true", sets[0].Params["use_independent_chat_permissions"])
	}
	var sent map[string]bool
	if err := json.Unmarshal([]byte(lockdownParamText(sets[0].Params["permissions"])), &sent); err != nil {
		t.Fatalf("permissions parameter is not a bool map: %v", err)
	}
	var want map[string]bool
	if err := json.Unmarshal([]byte(lockdownLockedPermissions), &want); err != nil {
		t.Fatalf("locked set is not a bool map: %v", err)
	}
	if len(sent) != 16 || len(want) != 16 {
		t.Errorf("permission key counts = sent %d, locked set %d, want 16 each", len(sent), len(want))
	}
	for key, value := range sent {
		if value {
			t.Errorf("locked permission %s = true, want false", key)
		}
		if _, ok := want[key]; !ok {
			t.Errorf("sent permission key %s is not in the locked set", key)
		}
	}
	if restricts := env.fake.callsFor("restrictChatMember"); len(restricts) != 0 {
		t.Errorf("restrictChatMember calls = %d, want none: a lockdown makes no per-user call", len(restricts))
	}

	mu.Lock()
	defer mu.Unlock()
	if !sawRowAtLockCall {
		t.Error("no chat_lockdowns row existed when setChatPermissions arrived, the snapshot must be stored first")
	}
}

func TestLockdownLiftRestoresExactSnapshot(t *testing.T) {
	env := newLockdownEnv(t)

	env.send(env.admin, "/lockdown")
	started, err := lockdown.GetActiveFresh(env.chat.Id)
	if err != nil || started == nil {
		t.Fatalf("GetActiveFresh = %v, %v, want the active row", started, err)
	}

	env.send(env.admin, "/unlockdown")

	sets := env.calls("setChatPermissions")
	if len(sets) != 2 {
		t.Fatalf("setChatPermissions calls = %d, want 2 (lock, restore)", len(sets))
	}
	if got := lockdownParamText(sets[1].Params["permissions"]); got != lockdownTestPrePermissions {
		t.Errorf("restore sent %q, want the stored snapshot %q", got, lockdownTestPrePermissions)
	}
	if got, _ := sets[1].Params["use_independent_chat_permissions"].(bool); !got {
		t.Errorf("restore use_independent_chat_permissions = %v, want true", sets[1].Params["use_independent_chat_permissions"])
	}
	if got := env.fake.chatPermsRaw(env.chat.Id); got != lockdownTestPrePermissions {
		t.Errorf("chat permissions after the lift = %q, want the snapshot byte for byte", got)
	}

	lifted, err := lockdown.GetFresh(started.ID)
	if err != nil || lifted == nil {
		t.Fatalf("GetFresh = %v, %v", lifted, err)
	}
	if lifted.State != models.LockdownStateLifted {
		t.Errorf("State = %q, want lifted", lifted.State)
	}
	if lifted.LiftedBy == nil || *lifted.LiftedBy != env.admin.Id {
		t.Errorf("LiftedBy = %v, want %d", lifted.LiftedBy, env.admin.Id)
	}
	if lifted.LiftedByName != "Ad<b>min" {
		t.Errorf("LiftedByName = %q", lifted.LiftedByName)
	}
	if lifted.LiftStartedAt == nil || lifted.LiftedAt == nil {
		t.Errorf("LiftStartedAt = %v, LiftedAt = %v, want both set", lifted.LiftStartedAt, lifted.LiftedAt)
	}
	if active, err := lockdown.GetActiveFresh(env.chat.Id); err != nil || active != nil {
		t.Errorf("GetActiveFresh after the lift = %v, %v, want nil, nil", active, err)
	}
	env.wantReplyHas(staffMarker("lockdown_lifted"), "Ad&lt;b&gt;min")
}
