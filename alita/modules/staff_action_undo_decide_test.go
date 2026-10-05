//go:build testtools

package modules

import (
	"reflect"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

// undoNow is the fixed clock of the undo decision tests.
const undoNow = int64(1_800_000_000)

// undoRow is one row of the undo decision table. wantPerms nil means the verdict
// carries no permission set.
type undoRow struct {
	name      string
	kind      staffActionKind
	prior     staffPriorState
	applied   staffAppliedState
	live      staffTargetState
	wantCall  staffAPICall
	wantWhy   staffReason
	wantUntil int64
	wantPerms *gotgbot.ChatPermissions
}

// partialPerms is a restriction that still lets the member send messages but
// not photos.
func partialPerms() *gotgbot.ChatPermissions {
	return &gotgbot.ChatPermissions{CanSendMessages: true, CanSendPhotos: false}
}

// priorOf builds a recorded prior state.
func priorOf(status string, isMember bool, until int64, perms *gotgbot.ChatPermissions) staffPriorState {
	return staffPriorState{Status: status, IsMember: isMember, Until: until, Perms: perms}
}

// runUndoRows checks every row of a decision table against decideStaffUndo.
func runUndoRows(t *testing.T, rows []undoRow) {
	t.Helper()
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			got := decideStaffUndo(row.kind, row.prior, row.applied, row.live, undoNow)
			if got.Call != row.wantCall || got.Reason != row.wantWhy {
				t.Fatalf("decideStaffUndo(%s, prior %+v, applied %+v, live %+v) = {%v %s}, want {%v %s}",
					row.kind, row.prior, row.applied, row.live, got.Call, got.Reason, row.wantCall, row.wantWhy)
			}
			if got.Until != row.wantUntil {
				t.Fatalf("verdict Until = %d, want %d", got.Until, row.wantUntil)
			}
			var wantPerms gotgbot.ChatPermissions
			if row.wantPerms != nil {
				wantPerms = *row.wantPerms
			}
			if !reflect.DeepEqual(got.Perms, wantPerms) {
				t.Fatalf("verdict Perms = %+v, want %+v", got.Perms, wantPerms)
			}
		})
	}
}

func TestStaffUndoDecisionTable(t *testing.T) {
	var (
		creator = staffState(gotgbot.ChatMemberStatusCreator, true, false, 0)
		admin   = staffState(gotgbot.ChatMemberStatusAdministrator, true, false, 0)
		member  = staffState(gotgbot.ChatMemberStatusMember, true, false, 0)
		left    = staffState(gotgbot.ChatMemberStatusLeft, false, false, 0)
		kicked  = staffState(gotgbot.ChatMemberStatusKicked, false, false, 0)

		priorMember = priorOf(gotgbot.ChatMemberStatusMember, true, 0, nil)
		priorLeft   = priorOf(gotgbot.ChatMemberStatusLeft, false, 0, nil)

		noUntil = staffAppliedState{Until: 0, AppliedAt: undoNow - 100}
	)
	kickedUntil := func(until int64) staffTargetState {
		return staffState(gotgbot.ChatMemberStatusKicked, false, false, until)
	}
	mutedIn := func(until int64) staffTargetState {
		return staffState(gotgbot.ChatMemberStatusRestricted, true, true, until)
	}
	priorRestricted := func(until int64, perms *gotgbot.ChatPermissions) staffPriorState {
		return priorOf(gotgbot.ChatMemberStatusRestricted, true, until, perms)
	}
	priorKicked := func(until int64) staffPriorState {
		return priorOf(gotgbot.ChatMemberStatusKicked, false, until, nil)
	}

	t.Run("ban", func(t *testing.T) {
		runUndoRows(t, []undoRow{
			// The shared guards come first.
			{name: "live creator", kind: staffKindBan, prior: priorMember, applied: noUntil, live: creator,
				wantCall: staffCallNone, wantWhy: staffReasonSkipTargetAdmin},
			{name: "live administrator with no prior state", kind: staffKindBan, prior: staffPriorState{}, applied: noUntil, live: admin,
				wantCall: staffCallNone, wantWhy: staffReasonSkipTargetAdmin},
			{name: "no captured prior state", kind: staffKindBan, prior: staffPriorState{}, applied: noUntil, live: kicked,
				wantCall: staffCallNone, wantWhy: staffReasonSkipNoPriorState},

			// A ban over a member or a departed target is lifted.
			{name: "prior member", kind: staffKindBan, prior: priorMember, applied: noUntil, live: kicked,
				wantCall: staffCallUnban, wantWhy: staffReasonUndoneUnbanned},
			{name: "prior left", kind: staffKindBan, prior: priorLeft, applied: noUntil, live: kicked,
				wantCall: staffCallUnban, wantWhy: staffReasonUndoneUnbanned},

			// Anything that differs from what the ban left is someone else's decision.
			{name: "someone already unbanned", kind: staffKindBan, prior: priorMember, applied: noUntil, live: left,
				wantCall: staffCallNone, wantWhy: staffReasonSkipChangedSince},
			{name: "banned for a different length since", kind: staffKindBan, prior: priorMember, applied: noUntil,
				live:     kickedUntil(undoNow + 86400),
				wantCall: staffCallNone, wantWhy: staffReasonSkipChangedSince},
			{name: "member again", kind: staffKindBan, prior: priorMember, applied: noUntil, live: member,
				wantCall: staffCallNone, wantWhy: staffReasonSkipChangedSince},

			// A restriction that is still running is put back as it was.
			{name: "prior restriction still running", kind: staffKindBan,
				prior: priorRestricted(undoNow+3600, partialPerms()), applied: noUntil, live: kicked,
				wantCall: staffCallRestore, wantWhy: staffReasonUndoneRestrictionRestored,
				wantUntil: undoNow + 3600, wantPerms: partialPerms()},
			{name: "prior restriction ends at the margin", kind: staffKindBan,
				prior: priorRestricted(undoNow+staffUndoMinRemaining, partialPerms()), applied: noUntil, live: kicked,
				wantCall: staffCallUnban, wantWhy: staffReasonUndoneUnbanned},
			{name: "prior restriction one second past the margin", kind: staffKindBan,
				prior: priorRestricted(undoNow+staffUndoMinRemaining+1, partialPerms()), applied: noUntil, live: kicked,
				wantCall: staffCallRestore, wantWhy: staffReasonUndoneRestrictionRestored,
				wantUntil: undoNow + staffUndoMinRemaining + 1, wantPerms: partialPerms()},
			{name: "prior permanent restriction", kind: staffKindBan,
				prior: priorRestricted(0, partialPerms()), applied: noUntil, live: kicked,
				wantCall: staffCallRestore, wantWhy: staffReasonUndoneRestrictionRestored,
				wantUntil: 0, wantPerms: partialPerms()},
			{name: "prior restriction without a stored permission set", kind: staffKindBan,
				prior: priorRestricted(undoNow+3600, nil), applied: noUntil, live: kicked,
				wantCall: staffCallNone, wantWhy: staffReasonSkipNoPriorState},

			// A shorter ban that the staff ban upgraded is put back with its end date.
			{name: "prior shorter ban", kind: staffKindBan,
				prior: priorKicked(undoNow + 7200), applied: noUntil, live: kicked,
				wantCall: staffCallBan, wantWhy: staffReasonUndoneBanRestored, wantUntil: undoNow + 7200},
			{name: "prior shorter ban already about to end", kind: staffKindBan,
				prior: priorKicked(undoNow + 60), applied: noUntil, live: kicked,
				wantCall: staffCallUnban, wantWhy: staffReasonUndoneUnbanned},

			// The recorded end date must match the live one exactly.
			{name: "dated ban left as sent", kind: staffKindBan, prior: priorMember,
				applied:  staffAppliedState{Until: undoNow + 86400, AppliedAt: undoNow - 100},
				live:     kickedUntil(undoNow + 86400),
				wantCall: staffCallUnban, wantWhy: staffReasonUndoneUnbanned},
			{name: "dated ban one second off", kind: staffKindBan, prior: priorMember,
				applied:  staffAppliedState{Until: undoNow + 86400, AppliedAt: undoNow - 100},
				live:     kickedUntil(undoNow + 86401),
				wantCall: staffCallNone, wantWhy: staffReasonSkipChangedSince},
			{name: "ban under 30 seconds is stored as permanent", kind: staffKindBan, prior: priorMember,
				applied:  staffAppliedState{Until: undoNow + 20, AppliedAt: undoNow},
				live:     kicked,
				wantCall: staffCallUnban, wantWhy: staffReasonUndoneUnbanned},
			{name: "ban of exactly 30 seconds is not stored as permanent", kind: staffKindBan, prior: priorMember,
				applied:  staffAppliedState{Until: undoNow + 30, AppliedAt: undoNow},
				live:     kicked,
				wantCall: staffCallNone, wantWhy: staffReasonSkipChangedSince},

			// A ban that has run out needs no undo.
			{name: "the staff ban already ran out", kind: staffKindBan, prior: priorMember,
				applied:  staffAppliedState{Until: undoNow - 10, AppliedAt: undoNow - 3600},
				live:     kicked,
				wantCall: staffCallNone, wantWhy: staffReasonSkipRestrictionEnded},
		})
	})

	t.Run("mute", func(t *testing.T) {
		runUndoRows(t, []undoRow{
			{name: "prior member", kind: staffKindMute, prior: priorMember, applied: noUntil, live: mutedIn(0),
				wantCall: staffCallUnmute, wantWhy: staffReasonUndoneUnmuted},
			{name: "muted member who left since keeps the same restriction", kind: staffKindMute, prior: priorMember, applied: noUntil,
				live:     staffState(gotgbot.ChatMemberStatusRestricted, false, true, 0),
				wantCall: staffCallUnmute, wantWhy: staffReasonUndoneUnmuted},
			{name: "someone already unmuted", kind: staffKindMute, prior: priorMember, applied: noUntil, live: member,
				wantCall: staffCallNone, wantWhy: staffReasonSkipChangedSince},
			{name: "banned since", kind: staffKindMute, prior: priorMember, applied: noUntil, live: kicked,
				wantCall: staffCallNone, wantWhy: staffReasonSkipChangedSince},
			{name: "restricted but able to send", kind: staffKindMute, prior: priorMember, applied: noUntil,
				live:     staffState(gotgbot.ChatMemberStatusRestricted, true, false, 0),
				wantCall: staffCallNone, wantWhy: staffReasonSkipChangedSince},
			{name: "live administrator", kind: staffKindMute, prior: priorMember, applied: noUntil, live: admin,
				wantCall: staffCallNone, wantWhy: staffReasonSkipTargetAdmin},
			{name: "no captured prior state", kind: staffKindMute, prior: staffPriorState{}, applied: noUntil, live: mutedIn(0),
				wantCall: staffCallNone, wantWhy: staffReasonSkipNoPriorState},

			{name: "prior restriction still running", kind: staffKindMute,
				prior: priorRestricted(undoNow+3600, partialPerms()), applied: noUntil, live: mutedIn(0),
				wantCall: staffCallRestore, wantWhy: staffReasonUndoneRestrictionRestored,
				wantUntil: undoNow + 3600, wantPerms: partialPerms()},
			{name: "prior restriction about to end", kind: staffKindMute,
				prior: priorRestricted(undoNow+60, partialPerms()), applied: noUntil, live: mutedIn(0),
				wantCall: staffCallUnmute, wantWhy: staffReasonUndoneUnmuted},
			{name: "prior restriction without a stored permission set", kind: staffKindMute,
				prior: priorRestricted(undoNow+3600, nil), applied: noUntil, live: mutedIn(0),
				wantCall: staffCallNone, wantWhy: staffReasonSkipNoPriorState},
			{name: "prior left is not a state a mute can have followed", kind: staffKindMute, prior: priorLeft, applied: noUntil, live: mutedIn(0),
				wantCall: staffCallNone, wantWhy: staffReasonSkipNoPriorState},

			{name: "dated mute left as sent", kind: staffKindMute, prior: priorMember,
				applied:  staffAppliedState{Until: undoNow + 3600, AppliedAt: undoNow - 100},
				live:     mutedIn(undoNow + 3600),
				wantCall: staffCallUnmute, wantWhy: staffReasonUndoneUnmuted},
			{name: "dated mute extended since", kind: staffKindMute, prior: priorMember,
				applied:  staffAppliedState{Until: undoNow + 3600, AppliedAt: undoNow - 100},
				live:     mutedIn(undoNow + 7200),
				wantCall: staffCallNone, wantWhy: staffReasonSkipChangedSince},
			{name: "the staff mute already ran out", kind: staffKindMute, prior: priorMember,
				applied:  staffAppliedState{Until: undoNow - 10, AppliedAt: undoNow - 3600},
				live:     member,
				wantCall: staffCallNone, wantWhy: staffReasonSkipRestrictionEnded},
		})
	})
}
