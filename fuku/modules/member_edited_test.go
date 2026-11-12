//go:build testtools

package modules

import (
	"testing"

	"github.com/uasneppy/fuku_robot/fuku/db/approvals"
	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
)

// TestMemberEditedDeletesLateEdit drives edited_message updates through the real dispatcher with
// member:edited stored as a grace of 60 seconds (plan 17-05, D-04, D-05, D-33): an edit 90 s after
// the message was sent is deleted at once, one 20 s after is kept, a group with no rule keeps both,
// and an administrator's and an approved member's late edits are kept.
func TestMemberEditedDeletesLateEdit(t *testing.T) {
	t.Run("a late edit is deleted at once", func(t *testing.T) {
		env := newMemberDeletionEnv(t)
		env.storeMemberRule(deletion.KindMemberEdited, memberEditedRuleDelay)
		env.primeAdminCache()
		member := env.member()
		env.wantDeletedOnce(env.lateEdit(&member, 90, nil))
	})
	t.Run("an edit within the grace is kept", func(t *testing.T) {
		env := newMemberDeletionEnv(t)
		env.storeMemberRule(deletion.KindMemberEdited, memberEditedRuleDelay)
		env.primeAdminCache()
		member := env.member()
		env.wantKept(env.lateEdit(&member, 20, nil))
	})
	t.Run("a group with no rule keeps a late edit", func(t *testing.T) {
		env := newMemberDeletionEnv(t)
		env.primeAdminCache()
		member := env.member()
		env.wantKept(env.lateEdit(&member, 90, nil))
	})
	t.Run("an administrator's late edit is kept", func(t *testing.T) {
		env := newMemberDeletionEnv(t)
		env.storeMemberRule(deletion.KindMemberEdited, memberEditedRuleDelay)
		env.primeAdminCache()
		env.wantKept(env.lateEdit(&env.admin, 90, nil))
	})
	t.Run("an approved member's late edit is kept", func(t *testing.T) {
		env := newMemberDeletionEnv(t)
		env.storeMemberRule(deletion.KindMemberEdited, memberEditedRuleDelay)
		env.primeAdminCache()
		member := env.member()
		if err := approvals.AddApprovedUser(env.chat.Id, member.Id, env.admin.Id, "trusted"); err != nil {
			t.Fatalf("AddApprovedUser error = %v", err)
		}
		env.wantKept(env.lateEdit(&member, 90, nil))
	})
}
