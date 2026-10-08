//go:build testtools

package modules

import (
	"fmt"
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

	t.Run("unban", func(t *testing.T) {
		runUndoRows(t, []undoRow{
			{name: "prior shorter ban", kind: staffKindUnban, prior: priorKicked(undoNow + 7200), applied: noUntil, live: left,
				wantCall: staffCallBan, wantWhy: staffReasonUndoneBanRestored, wantUntil: undoNow + 7200},
			{name: "prior permanent ban", kind: staffKindUnban, prior: priorKicked(0), applied: noUntil, live: left,
				wantCall: staffCallBan, wantWhy: staffReasonUndoneBanRestored, wantUntil: 0},
			{name: "prior ban about to end", kind: staffKindUnban, prior: priorKicked(undoNow + 60), applied: noUntil, live: left,
				wantCall: staffCallNone, wantWhy: staffReasonSkipRestrictionEnded},
			{name: "prior ban ends at the margin", kind: staffKindUnban, prior: priorKicked(undoNow + staffUndoMinRemaining), applied: noUntil, live: left,
				wantCall: staffCallNone, wantWhy: staffReasonSkipRestrictionEnded},
			{name: "prior ban one second past the margin", kind: staffKindUnban, prior: priorKicked(undoNow + staffUndoMinRemaining + 1), applied: noUntil, live: left,
				wantCall: staffCallBan, wantWhy: staffReasonUndoneBanRestored, wantUntil: undoNow + staffUndoMinRemaining + 1},
			{name: "rejoined since", kind: staffKindUnban, prior: priorKicked(undoNow + 7200), applied: noUntil, live: member,
				wantCall: staffCallNone, wantWhy: staffReasonSkipChangedSince},
			{name: "banned again since", kind: staffKindUnban, prior: priorKicked(undoNow + 7200), applied: noUntil, live: kicked,
				wantCall: staffCallNone, wantWhy: staffReasonSkipChangedSince},
			{name: "prior was not a ban", kind: staffKindUnban, prior: priorMember, applied: noUntil, live: left,
				wantCall: staffCallNone, wantWhy: staffReasonSkipNoPriorState},
			{name: "no captured prior state", kind: staffKindUnban, prior: staffPriorState{}, applied: noUntil, live: left,
				wantCall: staffCallNone, wantWhy: staffReasonSkipNoPriorState},
			{name: "live administrator", kind: staffKindUnban, prior: priorKicked(0), applied: noUntil, live: admin,
				wantCall: staffCallNone, wantWhy: staffReasonSkipTargetAdmin},
		})
	})

	t.Run("unmute", func(t *testing.T) {
		allOff := &gotgbot.ChatPermissions{}
		runUndoRows(t, []undoRow{
			{name: "live member", kind: staffKindUnmute, prior: priorRestricted(undoNow+3600, allOff), applied: noUntil, live: member,
				wantCall: staffCallRestore, wantWhy: staffReasonUndoneRestrictionRestored, wantUntil: undoNow + 3600, wantPerms: allOff},
			{name: "live left", kind: staffKindUnmute, prior: priorRestricted(undoNow+3600, allOff), applied: noUntil, live: left,
				wantCall: staffCallRestore, wantWhy: staffReasonUndoneRestrictionRestored, wantUntil: undoNow + 3600, wantPerms: allOff},
			{name: "live restricted but able to send", kind: staffKindUnmute, prior: priorRestricted(undoNow+3600, allOff), applied: noUntil,
				live:     staffState(gotgbot.ChatMemberStatusRestricted, true, false, 0),
				wantCall: staffCallRestore, wantWhy: staffReasonUndoneRestrictionRestored, wantUntil: undoNow + 3600, wantPerms: allOff},
			{name: "prior permanent partial restriction", kind: staffKindUnmute, prior: priorRestricted(0, partialPerms()), applied: noUntil, live: member,
				wantCall: staffCallRestore, wantWhy: staffReasonUndoneRestrictionRestored, wantUntil: 0, wantPerms: partialPerms()},
			{name: "muted again since", kind: staffKindUnmute, prior: priorRestricted(undoNow+3600, allOff), applied: noUntil, live: mutedIn(0),
				wantCall: staffCallNone, wantWhy: staffReasonSkipChangedSince},
			{name: "banned since", kind: staffKindUnmute, prior: priorRestricted(undoNow+3600, allOff), applied: noUntil, live: kicked,
				wantCall: staffCallNone, wantWhy: staffReasonSkipChangedSince},
			{name: "prior restriction ends at the margin", kind: staffKindUnmute,
				prior: priorRestricted(undoNow+staffUndoMinRemaining, allOff), applied: noUntil, live: member,
				wantCall: staffCallNone, wantWhy: staffReasonSkipRestrictionEnded},
			{name: "prior restriction one second past the margin", kind: staffKindUnmute,
				prior: priorRestricted(undoNow+staffUndoMinRemaining+1, allOff), applied: noUntil, live: member,
				wantCall: staffCallRestore, wantWhy: staffReasonUndoneRestrictionRestored,
				wantUntil: undoNow + staffUndoMinRemaining + 1, wantPerms: allOff},
			{name: "prior restriction without a stored permission set", kind: staffKindUnmute,
				prior: priorRestricted(undoNow+3600, nil), applied: noUntil, live: member,
				wantCall: staffCallNone, wantWhy: staffReasonSkipNoPriorState},
			{name: "prior was not a restriction", kind: staffKindUnmute, prior: priorMember, applied: noUntil, live: member,
				wantCall: staffCallNone, wantWhy: staffReasonSkipNoPriorState},
			{name: "live creator", kind: staffKindUnmute, prior: priorRestricted(0, allOff), applied: noUntil, live: creator,
				wantCall: staffCallNone, wantWhy: staffReasonSkipTargetAdmin},
		})
	})

	t.Run("kick has no undo", func(t *testing.T) {
		for _, live := range []staffTargetState{creator, admin, member, left, kicked, mutedIn(0)} {
			for _, prior := range []staffPriorState{{}, priorMember, priorLeft, priorRestricted(0, partialPerms()), priorKicked(0)} {
				got := decideStaffUndo(staffKindKick, prior, noUntil, live, undoNow)
				want := staffVerdict{Call: staffCallNone, Reason: staffReasonFailLookup}
				if got.staffVerdict != want {
					t.Fatalf("decideStaffUndo(kick, prior %+v, live %+v) = %+v, want %+v", prior, live, got.staffVerdict, want)
				}
			}
		}
	})

	t.Run("an unknown kind has no undo", func(t *testing.T) {
		got := decideStaffUndo(staffActionKind("weird"), priorMember, noUntil, kicked, undoNow)
		if want := (staffVerdict{Call: staffCallNone, Reason: staffReasonFailLookup}); got.staffVerdict != want {
			t.Fatalf("decideStaffUndo(unknown kind) = %+v, want %+v", got.staffVerdict, want)
		}
	})
}

// TestStaffUndoDecisionInvariants walks every kind against every recorded prior
// state, live state and applied end date, and checks the rules that keep an undo
// from overruling a later decision, touching an administrator or re-applying an
// end date Telegram could turn permanent, whatever the table says.
func TestStaffUndoDecisionInvariants(t *testing.T) {
	now := undoNow
	kinds := []staffActionKind{staffKindBan, staffKindMute, staffKindKick, staffKindUnban, staffKindUnmute}

	var priors []staffPriorState
	for _, status := range []string{
		"",
		gotgbot.ChatMemberStatusMember,
		gotgbot.ChatMemberStatusLeft,
		gotgbot.ChatMemberStatusRestricted,
		gotgbot.ChatMemberStatusKicked,
	} {
		for _, perms := range []*gotgbot.ChatPermissions{nil, partialPerms()} {
			for _, until := range []int64{0, now + 60, now + 120, now + 121, now + 7200} {
				priors = append(priors, staffPriorState{
					Status:   status,
					IsMember: status == gotgbot.ChatMemberStatusMember || status == gotgbot.ChatMemberStatusRestricted,
					Until:    until,
					Perms:    perms,
				})
			}
		}
	}

	var liveBase []staffTargetState
	for _, status := range []string{
		gotgbot.ChatMemberStatusCreator,
		gotgbot.ChatMemberStatusAdministrator,
		gotgbot.ChatMemberStatusMember,
		gotgbot.ChatMemberStatusLeft,
		gotgbot.ChatMemberStatusKicked,
	} {
		liveBase = append(liveBase, staffTargetState{
			Status: status,
			IsMember: status == gotgbot.ChatMemberStatusCreator || status == gotgbot.ChatMemberStatusAdministrator ||
				status == gotgbot.ChatMemberStatusMember,
		})
	}
	for _, isMember := range []bool{true, false} {
		for _, muted := range []bool{true, false} {
			liveBase = append(liveBase, staffTargetState{
				Status: gotgbot.ChatMemberStatusRestricted, IsMember: isMember, Muted: muted,
			})
		}
	}

	applieds := []staffAppliedState{
		{Until: 0, AppliedAt: now - 100},
		{Until: now + 86400, AppliedAt: now - 100},
		{Until: now - 10, AppliedAt: now - 100},
	}

	checked := 0
	calls := 0
	for _, kind := range kinds {
		for _, prior := range priors {
			for _, applied := range applieds {
				for _, base := range liveBase {
					for _, liveUntil := range []int64{0, applied.Until, applied.Until + 1} {
						live := base
						live.Until = liveUntil
						checked++
						v := decideStaffUndo(kind, prior, applied, live, now)
						label := fmt.Sprintf("%s, prior %+v, applied %+v, live %+v gave %+v until %d", kind, prior, applied, live, v.staffVerdict, v.Until)
						isAdmin := live.Status == gotgbot.ChatMemberStatusCreator || live.Status == gotgbot.ChatMemberStatusAdministrator
						goneOrLeft := live.Status == gotgbot.ChatMemberStatusKicked || live.Status == gotgbot.ChatMemberStatusLeft
						if v.Call != staffCallNone {
							calls++
						}

						// (a) a kicked or left target is touched only when it is exactly
						// what the staff action left behind (D-04).
						if v.Call != staffCallNone && goneOrLeft && !staffUndoLeftBehind(kind, applied, live) {
							t.Fatalf("a call reached a kicked or left target that is not what the action left: %s", label)
						}
						// (b) creators and administrators get no call at all.
						if v.Call != staffCallNone && isAdmin {
							t.Fatalf("a call was made against a creator or administrator: %s", label)
						}
						// (c) unban goes only to a banned target.
						if v.Call == staffCallUnban && live.Status != gotgbot.ChatMemberStatusKicked {
							t.Fatalf("unban sent to a target who is not banned: %s", label)
						}
						// (d) a restore or re-ban never carries an end date Telegram's
						// 30-second rule or a pacing delay could turn permanent.
						if (v.Call == staffCallRestore || v.Call == staffCallBan) && v.Until != 0 && v.Until <= now+staffUndoMinRemaining {
							t.Fatalf("an end date within the margin was re-applied: %s", label)
						}
						// (e) a done reason stands exactly for a call that was made.
						if done := staffReasonOutcome(v.Reason) == staffOutcomeDone; done != (v.Call != staffCallNone) {
							t.Fatalf("outcome %v does not match call %v: %s", staffReasonOutcome(v.Reason), v.Call, label)
						}
						// (f) undo never mutes with the fixed muted set and never removes a member.
						if v.Call == staffCallMute || v.Call == staffCallKick {
							t.Fatalf("undo chose a mute or kick call: %s", label)
						}
						// (g) a restore carries exactly the recorded permission set.
						if v.Call == staffCallRestore && (prior.Perms == nil || !reflect.DeepEqual(v.Perms, *prior.Perms)) {
							t.Fatalf("a restore did not carry the recorded permission set: %s", label)
						}
						// (h) a ban or restore only ever follows a recorded prior state.
						if v.Call != staffCallNone && prior.Status == "" {
							t.Fatalf("a call was made with no captured prior state: %s", label)
						}
					}
				}
			}
		}
	}
	if want := len(kinds) * len(priors) * len(applieds) * len(liveBase) * 3; checked != want {
		t.Fatalf("checked %d combinations, want %d", checked, want)
	}
	if calls == 0 {
		t.Fatal("no combination produced a call, so the invariants proved nothing")
	}
}

func TestStaffUndoReasonOutcomes(t *testing.T) {
	for _, r := range []staffReason{
		staffReasonUndoneUnbanned, staffReasonUndoneUnmuted,
		staffReasonUndoneBanRestored, staffReasonUndoneRestrictionRestored,
	} {
		if got := staffReasonOutcome(r); got != staffOutcomeDone {
			t.Fatalf("staffReasonOutcome(%s) = %v, want done", r, got)
		}
	}
	for _, r := range []staffReason{
		staffReasonSkipNotApplied, staffReasonSkipChangedSince,
		staffReasonSkipRestrictionEnded, staffReasonSkipNoPriorState,
	} {
		if got := staffReasonOutcome(r); got != staffOutcomeSkipped {
			t.Fatalf("staffReasonOutcome(%s) = %v, want skipped", r, got)
		}
	}
}
