//go:build testtools

package modules

import (
	"fmt"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

// staffState builds the target state of one live getChatMember answer.
func staffState(status string, isMember, muted bool, until int64) staffTargetState {
	return staffTargetState{Status: status, IsMember: isMember, Muted: muted, Until: until}
}

func TestStaffActionDecisionTable(t *testing.T) {
	const until = int64(2_000_000_000)
	var (
		creator  = staffState(gotgbot.ChatMemberStatusCreator, true, false, 0)
		admin    = staffState(gotgbot.ChatMemberStatusAdministrator, true, false, 0)
		member   = staffState(gotgbot.ChatMemberStatusMember, true, false, 0)
		left     = staffState(gotgbot.ChatMemberStatusLeft, false, false, 0)
		kicked   = staffState(gotgbot.ChatMemberStatusKicked, false, false, 0)
		kickedT  = staffState(gotgbot.ChatMemberStatusKicked, false, false, until)
		mutedIn  = staffState(gotgbot.ChatMemberStatusRestricted, true, true, 0)
		mutedInT = staffState(gotgbot.ChatMemberStatusRestricted, true, true, until)
		freeIn   = staffState(gotgbot.ChatMemberStatusRestricted, true, false, 0)
		mutedOut = staffState(gotgbot.ChatMemberStatusRestricted, false, true, 0)
		freeOut  = staffState(gotgbot.ChatMemberStatusRestricted, false, false, 0)
	)

	rows := []struct {
		name     string
		kind     staffActionKind
		state    staffTargetState
		newUntil int64
		want     staffVerdict
	}{
		// Creators and administrators are never touched, whatever the kind.
		{"ban creator", staffKindBan, creator, 0, staffVerdict{staffCallNone, staffReasonSkipTargetAdmin}},
		{"ban admin", staffKindBan, admin, 0, staffVerdict{staffCallNone, staffReasonSkipTargetAdmin}},
		{"mute creator", staffKindMute, creator, 0, staffVerdict{staffCallNone, staffReasonSkipTargetAdmin}},
		{"mute admin", staffKindMute, admin, 0, staffVerdict{staffCallNone, staffReasonSkipTargetAdmin}},
		{"kick creator", staffKindKick, creator, 0, staffVerdict{staffCallNone, staffReasonSkipTargetAdmin}},
		{"kick admin", staffKindKick, admin, 0, staffVerdict{staffCallNone, staffReasonSkipTargetAdmin}},
		{"unban creator", staffKindUnban, creator, 0, staffVerdict{staffCallNone, staffReasonSkipTargetAdmin}},
		{"unban admin", staffKindUnban, admin, 0, staffVerdict{staffCallNone, staffReasonSkipTargetAdmin}},
		{"unmute creator", staffKindUnmute, creator, 0, staffVerdict{staffCallNone, staffReasonSkipTargetAdmin}},
		{"unmute admin", staffKindUnmute, admin, 0, staffVerdict{staffCallNone, staffReasonSkipTargetAdmin}},

		// ban, unchanged from plan 02-01.
		{"ban kicked permanently", staffKindBan, kicked, 0, staffVerdict{staffCallNone, staffReasonSkipAlreadyBanned}},
		{"ban kicked until T with permanent", staffKindBan, kickedT, 0, staffVerdict{staffCallBan, staffReasonBanned}},
		{"ban kicked until T with the same T", staffKindBan, kickedT, until, staffVerdict{staffCallNone, staffReasonSkipAlreadyBanned}},
		{"ban left", staffKindBan, left, 0, staffVerdict{staffCallBan, staffReasonBannedNotInGroup}},
		{"ban restricted non-member", staffKindBan, mutedOut, 0, staffVerdict{staffCallBan, staffReasonBannedNotInGroup}},
		{"ban member", staffKindBan, member, 0, staffVerdict{staffCallBan, staffReasonBanned}},
		{"ban muted member is stricter", staffKindBan, mutedIn, 0, staffVerdict{staffCallBan, staffReasonBanned}},

		// mute.
		{"mute member", staffKindMute, member, 0, staffVerdict{staffCallMute, staffReasonMuted}},
		{"mute restricted member not muted", staffKindMute, freeIn, 0, staffVerdict{staffCallMute, staffReasonMuted}},
		{"mute already muted permanently", staffKindMute, mutedIn, 0, staffVerdict{staffCallNone, staffReasonSkipAlreadyMuted}},
		{"mute muted until T with permanent", staffKindMute, mutedInT, 0, staffVerdict{staffCallMute, staffReasonMuted}},
		{"mute muted until T with the same T", staffKindMute, mutedInT, until, staffVerdict{staffCallNone, staffReasonSkipAlreadyMuted}},
		{"mute muted until T with a later end", staffKindMute, mutedInT, until + 1, staffVerdict{staffCallMute, staffReasonMuted}},
		{"mute muted until T with an earlier end", staffKindMute, mutedInT, until - 1, staffVerdict{staffCallNone, staffReasonSkipAlreadyMuted}},
		{"mute restricted non-member", staffKindMute, mutedOut, 0, staffVerdict{staffCallNone, staffReasonSkipNotInGroup}},
		{"mute left", staffKindMute, left, 0, staffVerdict{staffCallNone, staffReasonSkipNotInGroup}},
		{"mute kicked", staffKindMute, kicked, 0, staffVerdict{staffCallNone, staffReasonSkipNotInGroup}},

		// kick.
		{"kick member", staffKindKick, member, 0, staffVerdict{staffCallKick, staffReasonKicked}},
		{"kick muted member", staffKindKick, mutedIn, 0, staffVerdict{staffCallKick, staffReasonKicked}},
		{"kick restricted member not muted", staffKindKick, freeIn, 0, staffVerdict{staffCallKick, staffReasonKicked}},
		{"kick restricted non-member", staffKindKick, mutedOut, 0, staffVerdict{staffCallNone, staffReasonSkipNotInGroup}},
		{"kick left", staffKindKick, left, 0, staffVerdict{staffCallNone, staffReasonSkipNotInGroup}},
		{"kick kicked", staffKindKick, kicked, 0, staffVerdict{staffCallNone, staffReasonSkipNotInGroup}},

		// unban.
		{"unban kicked", staffKindUnban, kicked, 0, staffVerdict{staffCallUnban, staffReasonUnbanned}},
		{"unban kicked until T", staffKindUnban, kickedT, 0, staffVerdict{staffCallUnban, staffReasonUnbanned}},
		{"unban member", staffKindUnban, member, 0, staffVerdict{staffCallNone, staffReasonSkipNotBanned}},
		{"unban muted member", staffKindUnban, mutedIn, 0, staffVerdict{staffCallNone, staffReasonSkipNotBanned}},
		{"unban restricted non-member", staffKindUnban, freeOut, 0, staffVerdict{staffCallNone, staffReasonSkipNotBanned}},
		{"unban left", staffKindUnban, left, 0, staffVerdict{staffCallNone, staffReasonSkipNotBanned}},

		// unmute.
		{"unmute muted member", staffKindUnmute, mutedIn, 0, staffVerdict{staffCallUnmute, staffReasonUnmuted}},
		{"unmute muted non-member", staffKindUnmute, mutedOut, 0, staffVerdict{staffCallUnmute, staffReasonUnmuted}},
		{"unmute member", staffKindUnmute, member, 0, staffVerdict{staffCallNone, staffReasonSkipNotMuted}},
		{"unmute restricted member not muted", staffKindUnmute, freeIn, 0, staffVerdict{staffCallNone, staffReasonSkipNotMuted}},
		{"unmute restricted non-member not muted", staffKindUnmute, freeOut, 0, staffVerdict{staffCallNone, staffReasonSkipNotMuted}},
		{"unmute left", staffKindUnmute, left, 0, staffVerdict{staffCallNone, staffReasonSkipNotMuted}},
		{"unmute kicked", staffKindUnmute, kicked, 0, staffVerdict{staffCallNone, staffReasonSkipNotInGroup}},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			if got := decideStaffAction(row.kind, row.state, row.newUntil); got != row.want {
				t.Fatalf("decideStaffAction(%s, %+v, %d) = %+v, want %+v", row.kind, row.state, row.newUntil, got, row.want)
			}
		})
	}

	t.Run("an unknown status fails the lookup for every kind", func(t *testing.T) {
		for _, kind := range []staffActionKind{staffKindBan, staffKindMute, staffKindKick, staffKindUnban, staffKindUnmute} {
			for _, status := range []string{"", "weird", "KICKED"} {
				want := staffVerdict{Call: staffCallNone, Reason: staffReasonFailLookup}
				if got := decideStaffAction(kind, staffState(status, false, false, 0), 0); got != want {
					t.Fatalf("decideStaffAction(%s, status %q) = %+v, want %+v", kind, status, got, want)
				}
			}
		}
	})
}

// TestDecideStaffBanLockdownJoiner covers the one case where a ban that does not end
// later is still sent: the live ban is a lockdown's own ban, which the lockdown's lift
// would remove, so the staff ban has to replace it (D-05).
func TestDecideStaffBanLockdownJoiner(t *testing.T) {
	const until = int64(2_000_000_000)
	lockdownBanned := staffTargetState{Status: gotgbot.ChatMemberStatusKicked, Until: until, LockdownBan: true}
	deliberateBan := staffTargetState{Status: gotgbot.ChatMemberStatusKicked, Until: until}

	rows := []struct {
		name     string
		kind     staffActionKind
		state    staffTargetState
		newUntil int64
		want     staffVerdict
	}{
		{"a shorter staff ban replaces a lockdown ban", staffKindBan, lockdownBanned, until - 1000,
			staffVerdict{staffCallBan, staffReasonBanned}},
		{"the same shorter ban over a deliberate ban is skipped", staffKindBan, deliberateBan, until - 1000,
			staffVerdict{staffCallNone, staffReasonSkipAlreadyBanned}},
		{"a permanent staff ban replaces a lockdown ban", staffKindBan, lockdownBanned, 0,
			staffVerdict{staffCallBan, staffReasonBanned}},
		{"a mute on a lockdown-banned target is still skipped as not in group", staffKindMute, lockdownBanned, until - 1000,
			staffVerdict{staffCallNone, staffReasonSkipNotInGroup}},
		{"a kick on a lockdown-banned target is still skipped as not in group", staffKindKick, lockdownBanned, 0,
			staffVerdict{staffCallNone, staffReasonSkipNotInGroup}},
		{"an unmute on a lockdown-banned target is still skipped as not in group", staffKindUnmute, lockdownBanned, 0,
			staffVerdict{staffCallNone, staffReasonSkipNotInGroup}},
		{"an unban on a lockdown-banned target is unchanged", staffKindUnban, lockdownBanned, 0,
			staffVerdict{staffCallUnban, staffReasonUnbanned}},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			if got := decideStaffAction(row.kind, row.state, row.newUntil); got != row.want {
				t.Fatalf("decideStaffAction(%s, %+v, %d) = %+v, want %+v", row.kind, row.state, row.newUntil, got, row.want)
			}
		})
	}
}

// TestStaffActionDecisionInvariants walks every kind against every live status
// and checks the rules that keep a staff action from lifting a ban or touching an
// administrator, whatever the table says.
func TestStaffActionDecisionInvariants(t *testing.T) {
	now := time.Now().Unix()
	kinds := []staffActionKind{staffKindBan, staffKindMute, staffKindKick, staffKindUnban, staffKindUnmute}
	currentUntils := []int64{0, now + 3600}
	newUntils := []int64{0, now + 60, now + 3600, now + 7200}

	var states []staffTargetState
	for _, status := range []string{
		gotgbot.ChatMemberStatusCreator,
		gotgbot.ChatMemberStatusAdministrator,
		gotgbot.ChatMemberStatusMember,
		gotgbot.ChatMemberStatusLeft,
		gotgbot.ChatMemberStatusKicked,
	} {
		for _, until := range currentUntils {
			states = append(states, staffTargetState{
				Status:   status,
				IsMember: status == gotgbot.ChatMemberStatusCreator || status == gotgbot.ChatMemberStatusAdministrator || status == gotgbot.ChatMemberStatusMember,
				Until:    until,
			})
		}
	}
	for _, isMember := range []bool{true, false} {
		for _, muted := range []bool{true, false} {
			for _, until := range currentUntils {
				states = append(states, staffTargetState{
					Status: gotgbot.ChatMemberStatusRestricted, IsMember: isMember, Muted: muted, Until: until,
				})
			}
		}
	}

	// A kicked target with an end date may also be under a lockdown's own ban.
	for _, st := range append([]staffTargetState(nil), states...) {
		if st.Status == gotgbot.ChatMemberStatusKicked && st.Until != 0 {
			st.LockdownBan = true
			states = append(states, st)
		}
	}

	checked := 0
	for _, kind := range kinds {
		for _, st := range states {
			for _, newUntil := range newUntils {
				checked++
				v := decideStaffAction(kind, st, newUntil)
				label := fmt.Sprintf("%s on %+v until %d gave %+v", kind, st, newUntil, v)
				isAdmin := st.Status == gotgbot.ChatMemberStatusCreator || st.Status == gotgbot.ChatMemberStatusAdministrator
				memberOrRestricted := st.Status == gotgbot.ChatMemberStatusMember || st.Status == gotgbot.ChatMemberStatusRestricted
				currentMember := st.Status == gotgbot.ChatMemberStatusMember ||
					(st.Status == gotgbot.ChatMemberStatusRestricted && st.IsMember)

				// (1) restrictChatMember replaces a kicked or left status.
				if (v.Call == staffCallMute || v.Call == staffCallUnmute) && !memberOrRestricted {
					t.Fatalf("restrict sent to a target who is neither member nor restricted: %s", label)
				}
				// (2) unbanChatMember(only_if_banned=false) removes a current member only.
				if v.Call == staffCallKick && !currentMember {
					t.Fatalf("member-removing unban sent to a target who is not a current member: %s", label)
				}
				// (3) unban is for a ban.
				if v.Call == staffCallUnban && st.Status != gotgbot.ChatMemberStatusKicked {
					t.Fatalf("unban sent to a target who is not banned: %s", label)
				}
				// (4) creators and administrators get no call at all.
				if isAdmin && v.Call != staffCallNone {
					t.Fatalf("a call was made against a creator or administrator: %s", label)
				}
				// (5) a banned target can only be banned again (later end) or unbanned.
				if st.Status == gotgbot.ChatMemberStatusKicked &&
					v.Call != staffCallNone && v.Call != staffCallBan && v.Call != staffCallUnban {
					t.Fatalf("a kicked target got a call that is not ban or unban: %s", label)
				}
				if st.Status == gotgbot.ChatMemberStatusKicked && v.Call == staffCallBan &&
					!st.LockdownBan && !staffEndsLater(newUntil, st.Until) {
					t.Fatalf("a ban that does not end later was sent over an existing ban: %s", label)
				}
				// (5b) a ban over a lockdown's own ban is always sent, so the lift keeps it.
				if kind == staffKindBan && st.Status == gotgbot.ChatMemberStatusKicked && st.LockdownBan &&
					v.Call != staffCallBan {
					t.Fatalf("a staff ban over a lockdown ban was not sent: %s", label)
				}
				// (6) a done reason stands exactly for a call that was made.
				if done := staffReasonOutcome(v.Reason) == staffOutcomeDone; done != (v.Call != staffCallNone) {
					t.Fatalf("outcome %v does not match call %v: %s", staffReasonOutcome(v.Reason), v.Call, label)
				}
			}
		}
	}
	if want := len(kinds) * len(states) * len(newUntils); checked != want {
		t.Fatalf("checked %d combinations, want %d", checked, want)
	}
}

func TestStaffActionEndsLater(t *testing.T) {
	cases := []struct {
		name           string
		newUntil, cur  int64
		wantEndsLatest bool
	}{
		{"permanent over permanent", 0, 0, false},
		{"dated over permanent", 100, 0, false},
		{"permanent over dated", 0, 100, true},
		{"equal dates", 100, 100, false},
		{"later date", 101, 100, true},
		{"earlier date", 99, 100, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := staffEndsLater(tc.newUntil, tc.cur); got != tc.wantEndsLatest {
				t.Fatalf("staffEndsLater(%d, %d) = %t, want %t", tc.newUntil, tc.cur, got, tc.wantEndsLatest)
			}
		})
	}
}
