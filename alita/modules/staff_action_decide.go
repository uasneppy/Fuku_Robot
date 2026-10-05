package modules

import "github.com/PaulSonOfLars/gotgbot/v2"

// staffActionKind names a staff moderation action. Later plans add mute, kick,
// unban and unmute next to ban.
type staffActionKind string

const (
	// staffKindBan bans the target, permanently or until a date.
	staffKindBan staffActionKind = "ban"
)

// staffOutcome is how one linked group ended for a staff action.
type staffOutcome int

const (
	// staffOutcomePending means the group has not been finished yet.
	staffOutcomePending staffOutcome = iota
	// staffOutcomeDone means the action was applied in the group.
	staffOutcomeDone
	// staffOutcomeSkipped means the group was deliberately left alone.
	staffOutcomeSkipped
	// staffOutcomeFailed means the group could not be judged or acted on.
	staffOutcomeFailed
)

// staffReason is the code behind a group's result line. The zero value means no
// reason, which staffIssuerSkipReason uses for "the issuer may act here".
type staffReason string

const (
	staffReasonBanned             staffReason = "banned"
	staffReasonBannedNotInGroup   staffReason = "banned_not_in_group"
	staffReasonSkipIssuerNotAdmin staffReason = "skip_issuer_not_admin"
	staffReasonSkipIssuerNoRight  staffReason = "skip_issuer_no_right"
	staffReasonSkipTargetAdmin    staffReason = "skip_target_admin"
	staffReasonSkipTargetBot      staffReason = "skip_target_bot"
	staffReasonSkipTargetService  staffReason = "skip_target_service"
	staffReasonSkipAlreadyBanned  staffReason = "skip_already_banned"
	staffReasonSkipLinkRemoved    staffReason = "skip_link_removed"
	staffReasonSkipStaffGroup     staffReason = "skip_staff_group"
	staffReasonFailOwnerUnknown   staffReason = "fail_owner_unknown"
	staffReasonFailLookup         staffReason = "fail_lookup"
	staffReasonFailTelegram       staffReason = "fail_telegram"
)

// staffReasonOutcome maps a reason to the outcome it stands for.
func staffReasonOutcome(r staffReason) staffOutcome {
	switch r {
	case staffReasonBanned, staffReasonBannedNotInGroup:
		return staffOutcomeDone
	case staffReasonSkipIssuerNotAdmin, staffReasonSkipIssuerNoRight, staffReasonSkipTargetAdmin,
		staffReasonSkipTargetBot, staffReasonSkipTargetService, staffReasonSkipAlreadyBanned,
		staffReasonSkipLinkRemoved, staffReasonSkipStaffGroup:
		return staffOutcomeSkipped
	case staffReasonFailOwnerUnknown, staffReasonFailLookup, staffReasonFailTelegram:
		return staffOutcomeFailed
	}
	return staffOutcomeFailed
}

// staffTargetState is what one live getChatMember answer says about the target
// of an action in one group.
type staffTargetState struct {
	Status   string
	IsMember bool
	// Muted is true for a restricted member who cannot send messages.
	Muted bool
	// Until is the end of the ban or restriction as a Unix time; 0 is permanent.
	Until int64
}

// staffTargetStateFrom reads the fields the decision table needs from a merged
// chat member.
func staffTargetStateFrom(m gotgbot.MergedChatMember) staffTargetState {
	st := staffTargetState{Status: m.Status, Until: m.UntilDate}
	switch m.Status {
	case gotgbot.ChatMemberStatusRestricted:
		st.IsMember = m.IsMember
		st.Muted = !m.CanSendMessages
	case gotgbot.ChatMemberStatusMember, gotgbot.ChatMemberStatusAdministrator, gotgbot.ChatMemberStatusCreator:
		st.IsMember = true
	}
	return st
}

// staffAPICall is the one Telegram write a verdict asks for.
type staffAPICall int

const (
	// staffCallNone means no write call is made.
	staffCallNone staffAPICall = iota
	// staffCallBan is banChatMember.
	staffCallBan
)

// staffVerdict is the decision for one group: the write to make, if any, and the
// reason shown on the group's line.
type staffVerdict struct {
	Call   staffAPICall
	Reason staffReason
}

// staffEndsLater reports whether a new end date outlasts the current one. Zero
// means permanent, which is the latest possible end (D-13).
func staffEndsLater(newUntil, curUntil int64) bool {
	switch {
	case curUntil == 0:
		return false
	case newUntil == 0:
		return true
	default:
		return newUntil > curUntil
	}
}

// decideStaffAction is the pure per-group decision. The target's creator or
// administrator status always wins (D-15); after that each kind has its own
// column of the status table.
func decideStaffAction(kind staffActionKind, st staffTargetState, newUntil int64) staffVerdict {
	if st.Status == gotgbot.ChatMemberStatusCreator || st.Status == gotgbot.ChatMemberStatusAdministrator {
		return staffVerdict{Call: staffCallNone, Reason: staffReasonSkipTargetAdmin}
	}
	switch kind {
	case staffKindBan:
		return decideStaffBan(st, newUntil)
	}
	return staffVerdict{Call: staffCallNone, Reason: staffReasonFailLookup}
}

// decideStaffBan is the ban column: a ban reaches a group the target is not in
// (D-10), upgrades a shorter ban (D-13) and never shortens one (D-12).
func decideStaffBan(st staffTargetState, newUntil int64) staffVerdict {
	switch st.Status {
	case gotgbot.ChatMemberStatusKicked:
		if staffEndsLater(newUntil, st.Until) {
			return staffVerdict{Call: staffCallBan, Reason: staffReasonBanned}
		}
		return staffVerdict{Call: staffCallNone, Reason: staffReasonSkipAlreadyBanned}
	case gotgbot.ChatMemberStatusLeft:
		return staffVerdict{Call: staffCallBan, Reason: staffReasonBannedNotInGroup}
	case gotgbot.ChatMemberStatusRestricted:
		if !st.IsMember {
			return staffVerdict{Call: staffCallBan, Reason: staffReasonBannedNotInGroup}
		}
		return staffVerdict{Call: staffCallBan, Reason: staffReasonBanned}
	case gotgbot.ChatMemberStatusMember:
		return staffVerdict{Call: staffCallBan, Reason: staffReasonBanned}
	}
	return staffVerdict{Call: staffCallNone, Reason: staffReasonFailLookup}
}

// staffServiceUserIDs are Telegram's own accounts: the service account, the
// anonymous-admin placeholder and the replies bot. They are never a target.
var staffServiceUserIDs = map[int64]bool{777000: true, 1087968824: true, 136817688: true}
