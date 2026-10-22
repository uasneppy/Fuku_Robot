//go:build testtools

package modules

import (
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/db/staff"
	"github.com/divkix/Alita_Robot/alita/utils/helpers"
)

// ranStaffCommand runs a staff command to its end and returns the card message and
// the audit record it left.
func (e *staffActionEnv) ranStaffCommand(command string) (msgID int64, action *models.StaffAction) {
	e.t.Helper()
	_, msgID = e.startRun(command)
	e.waitRuns()
	action, _ = recordOfCard(e.t, msgID)
	return msgID, action
}

// writeCounts counts the ban, restrict and unban calls addressed to each group, so a
// test can prove an undo made none in a group.
func (e *staffActionEnv) writeCounts(groups ...int64) map[int64]int {
	counts := make(map[int64]int, len(groups))
	for _, group := range groups {
		counts[group] = len(e.writes(group))
	}
	return counts
}

// wantUndoLine fails unless the undo summary has a line for title that starts with
// prefix and carries the marker of the locale key.
func wantUndoLine(t *testing.T, summary, title, prefix, key string) {
	t.Helper()
	line := summaryLine(t, summary, title)
	if !strings.HasPrefix(line, prefix) || !strings.Contains(line, staffMarker(key)) {
		t.Fatalf("undo line for %s = %q, want it to start with %q and say %s", title, line, prefix, key)
	}
}

// wantRestoreCall fails unless the last restrictChatMember call to chatID restored
// want until the end date, with use_independent_chat_permissions.
func wantRestoreCall(t *testing.T, env *staffActionEnv, chatID int64, want gotgbot.ChatPermissions, until int64) {
	t.Helper()
	calls := env.callsTo("restrictChatMember", chatID)
	if len(calls) == 0 {
		t.Fatalf("group %d got no restrictChatMember call", chatID)
	}
	call := calls[len(calls)-1]
	if !staffParamBool(call.Params, "use_independent_chat_permissions") {
		t.Fatalf("the restore in group %d was sent without use_independent_chat_permissions: %+v", chatID, call.Params)
	}
	if got, wantJSON := permsJSON(t, call.Params["permissions"]), permsJSON(t, want); got != wantJSON {
		t.Fatalf("the restore in group %d sent permissions %s, want %s", chatID, got, wantJSON)
	}
	if got := staffParamInt(call.Params, "until_date"); got != until {
		t.Fatalf("the restore in group %d sent until_date %d, want %d", chatID, got, until)
	}
}

// wantStoredPermissions fails unless the fake holds a restricted target with exactly
// the permission set and end date.
func wantStoredPermissions(t *testing.T, env *staffActionEnv, chatID int64, want gotgbot.ChatPermissions, until int64) {
	t.Helper()
	m := wantMember(t, env.fake, chatID, staffTestTarget, gotgbot.ChatMemberStatusRestricted)
	if m.Perms == nil {
		t.Fatalf("restricted target in group %d has no stored permissions: %+v", chatID, m)
	}
	if got, wantJSON := permsJSON(t, *m.Perms), permsJSON(t, want); got != wantJSON {
		t.Fatalf("target in group %d holds permissions %s, want %s", chatID, got, wantJSON)
	}
	if m.UntilDate != until {
		t.Fatalf("target in group %d is restricted until %d, want %d", chatID, m.UntilDate, until)
	}
}

// restrictedPerms builds a permission set the way a recorded prior state has it: the
// pointer fields are set, as staffPriorFromMember sets them.
func restrictedPerms(set func(p *gotgbot.ChatPermissions)) gotgbot.ChatPermissions {
	p := gotgbot.ChatPermissions{
		CanReactToMessages: helpers.Ptr(false),
		CanEditTag:         helpers.Ptr(false),
		CanManageTopics:    helpers.Ptr(false),
	}
	set(&p)
	return p
}

func TestStaffUndoRestores(t *testing.T) {
	t.Run("ban over a partial restriction", func(t *testing.T) {
		env := newStaffActionEnv(t, 1)
		group := env.groups[0]
		presser := env.undoPresser("Bob")
		until := time.Now().Unix() + 3600
		// can_send_other_messages implies the seven send permissions unless the call
		// says the permissions are independent, so a restore sent without that flag
		// would widen this set.
		perms := restrictedPerms(func(p *gotgbot.ChatPermissions) {
			p.CanSendMessages = true
			p.CanSendOtherMessages = true
			p.CanInviteUsers = true
		})
		env.fake.setMember(group, staffTestTarget, staffFakeMember{
			Status: gotgbot.ChatMemberStatusRestricted, IsMember: true, CanSendMessages: true, UntilDate: until, Perms: &perms,
		})

		msgID, action := env.ranStaffCommand("/ban 4242")
		wantMember(t, env.fake, group, staffTestTarget, gotgbot.ChatMemberStatusKicked)
		cardMsgID := env.runUndo(presser, msgID, action.ID)

		wantRestoreCall(t, env, group, perms, until)
		wantStoredPermissions(t, env, group, perms, until)
		if len(env.callsTo("unbanChatMember", group)) != 0 {
			t.Fatalf("the restore was preceded by an unban call: %+v", env.callsTo("unbanChatMember", group))
		}
		wantUndoLine(t, env.lastEditText(env.staffChat, cardMsgID), "Group A", "✅ Group A", "staff_undo_restriction_restored")
	})

	t.Run("ban over a shorter ban", func(t *testing.T) {
		env := newStaffActionEnv(t, 1)
		group := env.groups[0]
		presser := env.undoPresser("Bob")
		until := time.Now().Unix() + 7200
		env.fake.setMember(group, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusKicked, UntilDate: until})

		msgID, action := env.ranStaffCommand("/ban 4242")
		if m := wantMember(t, env.fake, group, staffTestTarget, gotgbot.ChatMemberStatusKicked); m.UntilDate != 0 {
			t.Fatalf("the permanent ban left an end date %d, want 0", m.UntilDate)
		}
		cardMsgID := env.runUndo(presser, msgID, action.ID)

		bans := env.callsTo("banChatMember", group)
		if len(bans) != 2 {
			t.Fatalf("ban calls = %d, want the staff ban and the restored ban", len(bans))
		}
		if got := staffParamInt(bans[1].Params, "until_date"); got != until {
			t.Fatalf("the restored ban sent until_date %d, want the original %d", got, until)
		}
		if m := wantMember(t, env.fake, group, staffTestTarget, gotgbot.ChatMemberStatusKicked); m.UntilDate != until {
			t.Fatalf("the target is banned until %d, want the original %d", m.UntilDate, until)
		}
		wantUndoLine(t, env.lastEditText(env.staffChat, cardMsgID), "Group A", "✅ Group A", "staff_undo_ban_restored")
	})

	t.Run("ban over a restriction that ended", func(t *testing.T) {
		env := newStaffActionEnv(t, 1)
		group := env.groups[0]
		presser := env.undoPresser("Bob")
		env.fake.setMember(group, staffTestTarget, staffFakeMember{
			Status: gotgbot.ChatMemberStatusRestricted, IsMember: true, UntilDate: time.Now().Unix() + 60,
		})

		msgID, action := env.ranStaffCommand("/ban 4242")
		cardMsgID := env.runUndo(presser, msgID, action.ID)

		unbans := env.callsTo("unbanChatMember", group)
		if len(unbans) != 1 || !staffParamBool(unbans[0].Params, "only_if_banned") {
			t.Fatalf("unban calls = %+v, want exactly one with only_if_banned", unbans)
		}
		if got := len(env.callsTo("restrictChatMember", group)); got != 0 {
			t.Fatalf("a restriction that ended was put back: %d restrict calls", got)
		}
		wantMember(t, env.fake, group, staffTestTarget, gotgbot.ChatMemberStatusLeft)
		wantUndoLine(t, env.lastEditText(env.staffChat, cardMsgID), "Group A", "✅ Group A", "staff_undo_unbanned")
	})

	t.Run("mute of a member", func(t *testing.T) {
		env := newStaffActionEnv(t, 1)
		group := env.groups[0]
		presser := env.undoPresser("Bob")
		defaults := gotgbot.ChatPermissions{CanSendMessages: true, CanSendPhotos: true, CanInviteUsers: true}
		env.fake.chatPerms[group] = defaults

		msgID, action := env.ranStaffCommand("/mute 4242")
		if m := wantMember(t, env.fake, group, staffTestTarget, gotgbot.ChatMemberStatusRestricted); m.CanSendMessages {
			t.Fatalf("the target can still send after /mute: %+v", m)
		}
		cardMsgID := env.runUndo(presser, msgID, action.ID)

		m := wantMember(t, env.fake, group, staffTestTarget, gotgbot.ChatMemberStatusRestricted)
		if !m.CanSendMessages || !m.IsMember {
			t.Fatalf("the target after the undo = %+v, want a member who can send", m)
		}
		if m.Perms == nil || permsJSON(t, *m.Perms) != permsJSON(t, defaults) {
			t.Fatalf("the target holds %+v, want the group's default permissions %+v", m.Perms, defaults)
		}
		if got := len(env.callsTo("unbanChatMember", group)); got != 0 {
			t.Fatalf("unmuting a member made %d unban calls, want none", got)
		}
		wantUndoLine(t, env.lastEditText(env.staffChat, cardMsgID), "Group A", "✅ Group A", "staff_undo_unmuted")
	})

	t.Run("mute over a shorter mute", func(t *testing.T) {
		env := newStaffActionEnv(t, 1)
		group := env.groups[0]
		presser := env.undoPresser("Bob")
		until := time.Now().Unix() + 3600
		perms := restrictedPerms(func(*gotgbot.ChatPermissions) {})
		env.fake.setMember(group, staffTestTarget, staffFakeMember{
			Status: gotgbot.ChatMemberStatusRestricted, IsMember: true, UntilDate: until, Perms: &perms,
		})

		msgID, action := env.ranStaffCommand("/mute 4242 1d")
		if m := wantMember(t, env.fake, group, staffTestTarget, gotgbot.ChatMemberStatusRestricted); m.UntilDate <= until {
			t.Fatalf("the longer mute ends at %d, want later than %d", m.UntilDate, until)
		}
		cardMsgID := env.runUndo(presser, msgID, action.ID)

		wantRestoreCall(t, env, group, perms, until)
		wantStoredPermissions(t, env, group, perms, until)
		wantUndoLine(t, env.lastEditText(env.staffChat, cardMsgID), "Group A", "✅ Group A", "staff_undo_restriction_restored")
	})

	t.Run("unban", func(t *testing.T) {
		env := newStaffActionEnv(t, 1)
		group := env.groups[0]
		presser := env.undoPresser("Bob")
		until := time.Now().Unix() + 7200
		env.fake.setMember(group, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusKicked, UntilDate: until})

		msgID, action := env.ranStaffCommand("/unban 4242")
		wantMember(t, env.fake, group, staffTestTarget, gotgbot.ChatMemberStatusLeft)
		cardMsgID := env.runUndo(presser, msgID, action.ID)

		bans := env.callsTo("banChatMember", group)
		if len(bans) != 1 || staffParamInt(bans[0].Params, "until_date") != until {
			t.Fatalf("ban calls = %+v, want one that re-bans until the original %d", bans, until)
		}
		if m := wantMember(t, env.fake, group, staffTestTarget, gotgbot.ChatMemberStatusKicked); m.UntilDate != until {
			t.Fatalf("the target is banned until %d, want the original %d", m.UntilDate, until)
		}
		wantUndoLine(t, env.lastEditText(env.staffChat, cardMsgID), "Group A", "✅ Group A", "staff_undo_ban_restored")
	})

	t.Run("unban of a ban that ended", func(t *testing.T) {
		env := newStaffActionEnv(t, 1)
		group := env.groups[0]
		presser := env.undoPresser("Bob")
		env.fake.setMember(group, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusKicked, UntilDate: time.Now().Unix() + 60})

		msgID, action := env.ranStaffCommand("/unban 4242")
		before := env.writeCounts(group)
		cardMsgID := env.runUndo(presser, msgID, action.ID)

		if after := env.writeCounts(group); after[group] != before[group] {
			t.Fatalf("an ended ban was put back: %d write calls, want %d", after[group], before[group])
		}
		wantMember(t, env.fake, group, staffTestTarget, gotgbot.ChatMemberStatusLeft)
		summary := env.lastEditText(env.staffChat, cardMsgID)
		wantUndoLine(t, summary, "Group A", "⏭ Group A", "staff_undo_skip_restriction_ended")
	})

	t.Run("unmute", func(t *testing.T) {
		env := newStaffActionEnv(t, 1)
		group := env.groups[0]
		presser := env.undoPresser("Bob")
		until := time.Now().Unix() + 5400
		perms := restrictedPerms(func(p *gotgbot.ChatPermissions) { p.CanInviteUsers = true })
		env.fake.setMember(group, staffTestTarget, staffFakeMember{
			Status: gotgbot.ChatMemberStatusRestricted, IsMember: true, UntilDate: until, Perms: &perms,
		})

		msgID, action := env.ranStaffCommand("/unmute 4242")
		if m := wantMember(t, env.fake, group, staffTestTarget, gotgbot.ChatMemberStatusRestricted); !m.CanSendMessages {
			t.Fatalf("the target still cannot send after /unmute: %+v", m)
		}
		cardMsgID := env.runUndo(presser, msgID, action.ID)

		wantRestoreCall(t, env, group, perms, until)
		wantStoredPermissions(t, env, group, perms, until)
		wantUndoLine(t, env.lastEditText(env.staffChat, cardMsgID), "Group A", "✅ Group A", "staff_undo_restriction_restored")
	})
}

func TestStaffUndoChangedSince(t *testing.T) {
	env := newStaffActionEnv(t, 3)
	presser := env.undoPresser("Bob")
	groupA, groupB, groupC := env.groups[0], env.groups[1], env.groups[2]

	msgID, action := env.ranStaffCommand("/ban 4242 1d")
	// A group admin lifted the ban in A, and in B banned the target for longer.
	env.fake.setMember(groupA, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusLeft})
	env.fake.setMember(groupB, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusKicked, UntilDate: time.Now().Unix() + 3*86400})
	before := env.writeCounts(groupA, groupB)

	cardMsgID := env.runUndo(presser, msgID, action.ID)

	after := env.writeCounts(groupA, groupB)
	for _, group := range []int64{groupA, groupB} {
		if after[group] != before[group] {
			t.Fatalf("group %d got %d write calls during the undo, want none: it changed since the action", group, after[group]-before[group])
		}
	}
	summary := env.lastEditText(env.staffChat, cardMsgID)
	wantUndoLine(t, summary, "Group A", "⏭ Group A", "staff_undo_skip_changed_since")
	wantUndoLine(t, summary, "Group B", "⏭ Group B", "staff_undo_skip_changed_since")
	wantUndoLine(t, summary, "Group C", "✅ Group C", "staff_undo_unbanned")
	if m := wantMember(t, env.fake, groupB, staffTestTarget, gotgbot.ChatMemberStatusKicked); m.UntilDate == 0 {
		t.Fatalf("the later ban in group B was lifted: %+v", m)
	}
	wantMember(t, env.fake, groupC, staffTestTarget, gotgbot.ChatMemberStatusLeft)
}

func TestStaffUndoNotAppliedOriginally(t *testing.T) {
	env := newStaffActionEnv(t, 2)
	presser := env.undoPresser("Bob")
	groupA, groupB := env.groups[0], env.groups[1]
	env.fake.script("banChatMember", groupB, staffFakeError(400, "Bad Request: not enough rights"))

	msgID, action := env.ranStaffCommand("/ban 4242")
	if _, rows := recordOfCard(t, msgID); rows[1].Outcome != models.StaffActionOutcomeFailed {
		t.Fatalf("group B outcome = %q, want the ban to have failed there", rows[1].Outcome)
	}
	lookups := env.callsTo("getChatMember", groupB)
	writes := env.writeCounts(groupB)

	env.runUndo(presser, msgID, action.ID)

	if got := len(env.callsTo("getChatMember", groupB)); got != len(lookups) {
		t.Fatalf("group B got %d getChatMember calls during the undo, want none", got-len(lookups))
	}
	if after := env.writeCounts(groupB); after[groupB] != writes[groupB] {
		t.Fatalf("group B got %d write calls during the undo, want none", after[groupB]-writes[groupB])
	}
	wantMember(t, env.fake, groupA, staffTestTarget, gotgbot.ChatMemberStatusLeft)

	_, rows := recordOfCard(t, msgID)
	for _, row := range rows {
		if row.GroupChatID != groupB {
			continue
		}
		if row.UndoOutcome != models.StaffActionOutcomeSkipped || row.UndoReason != "skip_not_applied" {
			t.Fatalf("group B undo = %q / %q, want skipped / skip_not_applied", row.UndoOutcome, row.UndoReason)
		}
	}
}

func TestStaffUndoLinkRemoved(t *testing.T) {
	env := newStaffActionEnv(t, 2)
	presser := env.undoPresser("Bob")
	groupA, groupB := env.groups[0], env.groups[1]

	msgID, action := env.ranStaffCommand("/ban 4242")
	link, err := staff.GetLinkOfGroupFresh(groupB)
	if err != nil || link == nil {
		t.Fatalf("link of group B = %v, %v", link, err)
	}
	if deleted, err := staff.DeleteLink(link.ID); err != nil || !deleted {
		t.Fatalf("DeleteLink = %v, %v, want the link deleted", deleted, err)
	}
	callsBefore := env.callsAddressedTo(groupB)

	cardMsgID := env.runUndo(presser, msgID, action.ID)

	if got := env.callsAddressedTo(groupB); got != callsBefore {
		t.Fatalf("group B received %d Telegram calls during the undo, want none: its link is gone", got-callsBefore)
	}
	summary := env.lastEditText(env.staffChat, cardMsgID)
	wantUndoLine(t, summary, "Group B", "⏭ Group B", "staff_act_skip_link_removed")
	wantUndoLine(t, summary, "Group A", "✅ Group A", "staff_undo_unbanned")
	wantMember(t, env.fake, groupB, staffTestTarget, gotgbot.ChatMemberStatusKicked)
	wantMember(t, env.fake, groupA, staffTestTarget, gotgbot.ChatMemberStatusLeft)
}

func TestStaffUndoPresserRights(t *testing.T) {
	env := newStaffActionEnv(t, 4)
	groupA, groupB, groupC, groupD := env.groups[0], env.groups[1], env.groups[2], env.groups[3]
	bob := gotgbot.User{Id: env.issuer.Id + 1, FirstName: "Bob"}
	env.fake.setMember(groupA, bob.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusCreator})
	env.fake.setMember(groupB, bob.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusAdministrator, CanRestrictMembers: true})
	env.fake.setMember(groupC, bob.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusAdministrator, CanRestrictMembers: false})
	env.fake.setMember(groupD, bob.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})

	msgID, action := env.ranStaffCommand("/ban 4242")
	// The original issuer has lost every right since; only Bob's rights count.
	for _, group := range env.groups {
		env.fake.setMember(group, env.issuer.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
	}
	lookups := map[int64]int{groupC: env.memberLookups(groupC, staffTestTarget), groupD: env.memberLookups(groupD, staffTestTarget)}
	writes := env.writeCounts(groupC, groupD)

	cardMsgID := env.runUndo(bob, msgID, action.ID)

	summary := env.lastEditText(env.staffChat, cardMsgID)
	wantUndoLine(t, summary, "Group A", "✅ Group A", "staff_undo_unbanned")
	wantUndoLine(t, summary, "Group B", "✅ Group B", "staff_undo_unbanned")
	wantUndoLine(t, summary, "Group C", "⏭ Group C", "staff_act_skip_issuer_no_right")
	wantUndoLine(t, summary, "Group D", "⏭ Group D", "staff_act_skip_issuer_not_admin")
	for _, group := range []int64{groupC, groupD} {
		if got := env.memberLookups(group, staffTestTarget); got != lookups[group] {
			t.Fatalf("group %d looked up the target %d time(s) during the undo, want none", group, got-lookups[group])
		}
		if after := env.writeCounts(group); after[group] != writes[group] {
			t.Fatalf("group %d got %d write calls during the undo, want none", group, after[group]-writes[group])
		}
		wantMember(t, env.fake, group, staffTestTarget, gotgbot.ChatMemberStatusKicked)
	}
	wantMember(t, env.fake, groupA, staffTestTarget, gotgbot.ChatMemberStatusLeft)
	wantMember(t, env.fake, groupB, staffTestTarget, gotgbot.ChatMemberStatusLeft)
}
