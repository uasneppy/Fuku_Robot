//go:build testtools

package modules

import "testing"

func TestCanonicalPermissions(t *testing.T) {
	t.Run("key order does not matter", func(t *testing.T) {
		same, err := samePermissions(`{"can_send_messages":true,"can_invite_users":true}`, `{"can_invite_users":true,"can_send_messages":true}`)
		if err != nil || !same {
			t.Errorf("samePermissions = %v, %v, want true", same, err)
		}
	})
	t.Run("a missing key equals false", func(t *testing.T) {
		same, err := samePermissions(`{"can_send_messages":true,"can_pin_messages":false}`, `{"can_send_messages":true}`)
		if err != nil || !same {
			t.Errorf("samePermissions = %v, %v, want true", same, err)
		}
	})
	t.Run("a different grant differs", func(t *testing.T) {
		same, err := samePermissions(`{"can_send_messages":true}`, `{"can_send_messages":true,"can_pin_messages":true}`)
		if err != nil || same {
			t.Errorf("samePermissions = %v, %v, want false", same, err)
		}
	})
	t.Run("a non-boolean value is ignored", func(t *testing.T) {
		granted, err := canonicalPermissions(`{"can_send_messages":"yes","can_pin_messages":1,"can_invite_users":true}`)
		if err != nil {
			t.Fatalf("canonicalPermissions error = %v", err)
		}
		if len(granted) != 1 || !granted["can_invite_users"] {
			t.Errorf("granted = %v, want only can_invite_users", granted)
		}
	})
	t.Run("invalid JSON is an error", func(t *testing.T) {
		if _, err := canonicalPermissions(`{"can_send_messages":`); err == nil {
			t.Error("canonicalPermissions accepted invalid JSON")
		}
		if _, err := samePermissions(`{}`, `not json`); err == nil {
			t.Error("samePermissions accepted invalid JSON")
		}
	})
	t.Run("the locked set grants nothing", func(t *testing.T) {
		same, err := samePermissions(lockdownLockedPermissions, "{}")
		if err != nil || !same {
			t.Errorf("samePermissions(locked set, {}) = %v, %v, want true", same, err)
		}
	})
}
