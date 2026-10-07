package modules

import "github.com/PaulSonOfLars/gotgbot/v2"

// staffActionKind names a staff moderation action.
type staffActionKind string

const (
	// staffKindBan bans the target, permanently or until a date.
	staffKindBan staffActionKind = "ban"
	// staffKindMute restricts a member so they cannot send messages.
	staffKindMute staffActionKind = "mute"
	// staffKindKick removes a current member, who may rejoin.
	staffKindKick staffActionKind = "kick"
	// staffKindUnban lifts a ban and never removes a member.
	staffKindUnban staffActionKind = "unban"
	// staffKindUnmute gives a muted target the group's default permissions back.
	staffKindUnmute staffActionKind = "unmute"
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
	staffReasonMuted              staffReason = "muted"
	staffReasonKicked             staffReason = "kicked"
	staffReasonUnbanned           staffReason = "unbanned"
	staffReasonUnmuted            staffReason = "unmuted"
	staffReasonSkipIssuerNotAdmin staffReason = "skip_issuer_not_admin"
	staffReasonSkipIssuerNoRight  staffReason = "skip_issuer_no_right"
	staffReasonSkipTargetAdmin    staffReason = "skip_target_admin"
	staffReasonSkipTargetBot      staffReason = "skip_target_bot"
	staffReasonSkipTargetService  staffReason = "skip_target_service"
	staffReasonSkipAlreadyBanned  staffReason = "skip_already_banned"
	staffReasonSkipNotInGroup     staffReason = "skip_not_in_group"
	staffReasonSkipAlreadyMuted   staffReason = "skip_already_muted"
	staffReasonSkipNotBanned      staffReason = "skip_not_banned"
	staffReasonSkipNotMuted       staffReason = "skip_not_muted"
	staffReasonSkipLinkRemoved    staffReason = "skip_link_removed"
	staffReasonSkipStaffGroup     staffReason = "skip_staff_group"
	staffReasonFailOwnerUnknown   staffReason = "fail_owner_unknown"
	staffReasonFailLookup         staffReason = "fail_lookup"
	staffReasonFailTelegram       staffReason = "fail_telegram"
	staffReasonFailRateLimited    staffReason = "fail_rate_limited"
	staffReasonFailBotNotAdmin    staffReason = "fail_bot_not_admin"
	staffReasonFailBotNoRights    staffReason = "fail_bot_no_rights"
	staffReasonFailGroupNotFound  staffReason = "fail_group_not_found"
	staffReasonFailInternal       staffReason = "fail_internal"
	staffReasonFailInterrupted    staffReason = "fail_interrupted"

	// Undo reasons (Phase 3). The undone_* reasons stand for a call that was made;
	// the skip_* reasons leave the group alone.
	staffReasonUndoneUnbanned            staffReason = "undone_unbanned"
	staffReasonUndoneUnmuted             staffReason = "undone_unmuted"
	staffReasonUndoneBanRestored         staffReason = "undone_ban_restored"
	staffReasonUndoneRestrictionRestored staffReason = "undone_restriction_restored"
	staffReasonSkipNotApplied            staffReason = "skip_not_applied"
	staffReasonSkipChangedSince          staffReason = "skip_changed_since"
	staffReasonSkipRestrictionEnded      staffReason = "skip_restriction_ended"
	staffReasonSkipNoPriorState          staffReason = "skip_no_prior_state"
)

// staffReasonOutcome maps a reason to the outcome it stands for.
func staffReasonOutcome(r staffReason) staffOutcome {
	switch r {
	case staffReasonBanned, staffReasonBannedNotInGroup, staffReasonMuted, staffReasonKicked,
		staffReasonUnbanned, staffReasonUnmuted, staffReasonUndoneUnbanned, staffReasonUndoneUnmuted,
		staffReasonUndoneBanRestored, staffReasonUndoneRestrictionRestored:
		return staffOutcomeDone
	case staffReasonSkipIssuerNotAdmin, staffReasonSkipIssuerNoRight, staffReasonSkipTargetAdmin,
		staffReasonSkipTargetBot, staffReasonSkipTargetService, staffReasonSkipAlreadyBanned,
		staffReasonSkipLinkRemoved, staffReasonSkipStaffGroup, staffReasonSkipNotInGroup,
		staffReasonSkipAlreadyMuted, staffReasonSkipNotBanned, staffReasonSkipNotMuted,
		staffReasonSkipNotApplied, staffReasonSkipChangedSince, staffReasonSkipRestrictionEnded,
		staffReasonSkipNoPriorState:
		return staffOutcomeSkipped
	case staffReasonFailOwnerUnknown, staffReasonFailLookup, staffReasonFailTelegram,
		staffReasonFailRateLimited, staffReasonFailBotNotAdmin, staffReasonFailBotNoRights,
		staffReasonFailGroupNotFound, staffReasonFailInternal, staffReasonFailInterrupted:
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
	// LockdownBan is true when the target's current ban is a lockdown's own ban: its
	// end date is a lockdown joiner's ban_until. A staff ban must then replace it even
	// when it ends sooner, or the lockdown's lift would lift the staff ban too (D-05).
	// It is set only for a staff ban on a kicked target with an end date.
	LockdownBan bool
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
	// staffCallMute is restrictChatMember with MutedPermissions.
	staffCallMute
	// staffCallKick is unbanChatMember with only_if_banned=false, which removes a
	// current member without banning them.
	staffCallKick
	// staffCallUnban is unbanChatMember with only_if_banned=true.
	staffCallUnban
	// staffCallUnmute is restrictChatMember with the group's default permissions.
	staffCallUnmute
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

// staffDo is a verdict that makes a call.
func staffDo(call staffAPICall, reason staffReason) staffVerdict {
	return staffVerdict{Call: call, Reason: reason}
}

// staffSkip is a verdict that leaves the group alone for the given reason.
func staffSkip(reason staffReason) staffVerdict {
	return staffVerdict{Call: staffCallNone, Reason: reason}
}

// decideStaffAction is the pure per-group decision, and the only place that
// chooses which Telegram write a group gets. The target's creator or
// administrator status always wins (D-15); after that each kind has its own
// column of the status table.
//
// The table is built around one rule: restrictChatMember and unbanChatMember
// REPLACE the target's status on the server, so a restrict sent to a banned or
// departed target would lift the ban. Restrict goes only to a member or a
// restricted target, member-removing unban only to a current member, and every
// unban is only_if_banned=true.
func decideStaffAction(kind staffActionKind, st staffTargetState, newUntil int64) staffVerdict {
	if st.Status == gotgbot.ChatMemberStatusCreator || st.Status == gotgbot.ChatMemberStatusAdministrator {
		return staffSkip(staffReasonSkipTargetAdmin)
	}
	switch kind {
	case staffKindBan:
		return decideStaffBan(st, newUntil)
	case staffKindMute:
		return decideStaffMute(st, newUntil)
	case staffKindKick:
		return decideStaffKick(st)
	case staffKindUnban:
		return decideStaffUnban(st)
	case staffKindUnmute:
		return decideStaffUnmute(st)
	}
	return staffSkip(staffReasonFailLookup)
}

// decideStaffBan is the ban column: a ban reaches a group the target is not in
// (D-10), upgrades a shorter ban (D-13) and never shortens one (D-12). The one
// exception to never shortening is a lockdown's own ban (LockdownBan): it is
// replaced even by a shorter or equal staff ban, which then carries its own end date,
// so the lockdown's lift leaves it alone (D-05).
func decideStaffBan(st staffTargetState, newUntil int64) staffVerdict {
	switch st.Status {
	case gotgbot.ChatMemberStatusKicked:
		if st.LockdownBan {
			return staffVerdict{Call: staffCallBan, Reason: staffReasonBanned}
		}
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

// decideStaffMute is the mute column. Only a current member is muted (D-11); a
// muted member is muted again only when the new mute ends later (D-12, D-13), and
// a banned target is never touched, because restrictChatMember would replace the
// ban.
func decideStaffMute(st staffTargetState, newUntil int64) staffVerdict {
	switch st.Status {
	case gotgbot.ChatMemberStatusMember:
		return staffDo(staffCallMute, staffReasonMuted)
	case gotgbot.ChatMemberStatusRestricted:
		if !st.IsMember {
			return staffSkip(staffReasonSkipNotInGroup)
		}
		if st.Muted && !staffEndsLater(newUntil, st.Until) {
			return staffSkip(staffReasonSkipAlreadyMuted)
		}
		return staffDo(staffCallMute, staffReasonMuted)
	case gotgbot.ChatMemberStatusLeft, gotgbot.ChatMemberStatusKicked:
		return staffSkip(staffReasonSkipNotInGroup)
	}
	return staffSkip(staffReasonFailLookup)
}

// decideStaffKick is the kick column. Only a current member is removed; the
// member-removing unban on anyone else would lift a ban. A muted member is kicked
// too, which drops the mute exactly as the per-group /kick does.
func decideStaffKick(st staffTargetState) staffVerdict {
	switch st.Status {
	case gotgbot.ChatMemberStatusMember:
		return staffDo(staffCallKick, staffReasonKicked)
	case gotgbot.ChatMemberStatusRestricted:
		if st.IsMember {
			return staffDo(staffCallKick, staffReasonKicked)
		}
		return staffSkip(staffReasonSkipNotInGroup)
	case gotgbot.ChatMemberStatusLeft, gotgbot.ChatMemberStatusKicked:
		return staffSkip(staffReasonSkipNotInGroup)
	}
	return staffSkip(staffReasonFailLookup)
}

// decideStaffUnban is the unban column: it acts on a banned target and on nobody
// else (D-14), so it can never remove a member.
func decideStaffUnban(st staffTargetState) staffVerdict {
	switch st.Status {
	case gotgbot.ChatMemberStatusKicked:
		return staffDo(staffCallUnban, staffReasonUnbanned)
	case gotgbot.ChatMemberStatusMember, gotgbot.ChatMemberStatusRestricted, gotgbot.ChatMemberStatusLeft:
		return staffSkip(staffReasonSkipNotBanned)
	}
	return staffSkip(staffReasonFailLookup)
}

// decideStaffUnmute is the unmute column. A muted restricted target gets the
// default permissions back, member or not, without becoming a member. A banned
// target is never unmuted, because restrictChatMember would lift the ban.
func decideStaffUnmute(st staffTargetState) staffVerdict {
	switch st.Status {
	case gotgbot.ChatMemberStatusRestricted:
		if st.Muted {
			return staffDo(staffCallUnmute, staffReasonUnmuted)
		}
		return staffSkip(staffReasonSkipNotMuted)
	case gotgbot.ChatMemberStatusMember, gotgbot.ChatMemberStatusLeft:
		return staffSkip(staffReasonSkipNotMuted)
	case gotgbot.ChatMemberStatusKicked:
		return staffSkip(staffReasonSkipNotInGroup)
	}
	return staffSkip(staffReasonFailLookup)
}

// staffCallRestore is restrictChatMember with a recorded permission set and end
// date, sent with use_independent_chat_permissions. Only decideStaffUndo returns
// it.
const staffCallRestore staffAPICall = staffCallUnmute + 1

// staffUndoMinRemaining is how many seconds a recorded end date must still have
// before undo re-applies it. Telegram turns a ban or restriction that ends less
// than 30 seconds from the moment it is applied into a permanent one, and a pacing
// wait or a 429 retry can delay the call, so a closer end date counts as ended.
const staffUndoMinRemaining int64 = 120

// staffAppliedState is what the staff action sent to one group.
type staffAppliedState struct {
	// Until is the until_date the action sent; 0 is permanent.
	Until int64
	// AppliedAt is when the group's write was recorded, as a Unix time.
	AppliedAt int64
}

// staffUndoVerdict is the decision for one group's undo. It embeds staffVerdict
// instead of adding fields to it: the Phase 2 tests build staffVerdict with
// unkeyed literals, so that type must not change.
type staffUndoVerdict struct {
	staffVerdict
	// Until is the end date of a restored ban or restriction; 0 is permanent.
	Until int64
	// Perms is the permission set of a staffCallRestore.
	Perms gotgbot.ChatPermissions
}

// staffUndoSkip is an undo verdict that leaves the group alone.
func staffUndoSkip(reason staffReason) staffUndoVerdict {
	return staffUndoVerdict{staffVerdict: staffSkip(reason)}
}

// staffUndoDo is an undo verdict that makes a call with an end date.
func staffUndoDo(call staffAPICall, reason staffReason, until int64) staffUndoVerdict {
	return staffUndoVerdict{staffVerdict: staffDo(call, reason), Until: until}
}

// staffUndoRestore is the verdict that puts a recorded restriction back.
func staffUndoRestore(prior staffPriorState) staffUndoVerdict {
	return staffUndoVerdict{
		staffVerdict: staffDo(staffCallRestore, staffReasonUndoneRestrictionRestored),
		Until:        prior.Until,
		Perms:        *prior.Perms,
	}
}

// staffUndoEnded reports whether a recorded end date is gone or too close to be
// re-applied safely. Zero is permanent and never ended.
func staffUndoEnded(until, now int64) bool {
	return until != 0 && until <= now+staffUndoMinRemaining
}

// staffUndoSameUntil reports whether a live end date is the one the staff action
// sent. Telegram stores an end date less than 30 seconds after the call as a
// permanent ban, so a live permanent state also matches such a recorded date.
func staffUndoSameUntil(live int64, applied staffAppliedState) bool {
	if live == applied.Until {
		return true
	}
	return applied.Until != 0 && live == 0 && applied.Until-applied.AppliedAt < 30
}

// staffUndoLeftBehind reports whether the live state is exactly what the staff
// action left in the group. Undo acts only on that state: any difference is a later
// decision by someone else (D-04). A mute does not compare membership, because a
// muted member who left keeps the same restriction and membership is not a staff
// decision. Kick has no undo.
func staffUndoLeftBehind(orig staffActionKind, applied staffAppliedState, live staffTargetState) bool {
	switch orig {
	case staffKindBan:
		return live.Status == gotgbot.ChatMemberStatusKicked && staffUndoSameUntil(live.Until, applied)
	case staffKindMute:
		return live.Status == gotgbot.ChatMemberStatusRestricted && live.Muted && staffUndoSameUntil(live.Until, applied)
	case staffKindUnban:
		return live.Status == gotgbot.ChatMemberStatusLeft
	case staffKindUnmute:
		switch live.Status {
		case gotgbot.ChatMemberStatusMember, gotgbot.ChatMemberStatusLeft:
			return true
		case gotgbot.ChatMemberStatusRestricted:
			return !live.Muted
		}
	}
	return false
}

// decideStaffUndo is the pure per-group decision for undoing a staff action, and
// the only place that chooses an undo's Telegram call. It puts the target's
// recorded prior state back and never just lifts (D-02), and it leaves the group
// alone whenever the live state is not exactly what the staff action left behind
// (D-04).
//
// It makes the one sanctioned exception to the rule decideStaffAction is built
// on: a restore or a re-ban may be sent to a kicked or left target, which a
// restrict or ban must never reach otherwise because it replaces that status. That
// is safe only because the live state equals the state the staff action itself left
// (staffUndoLeftBehind), so the call replaces nothing but the staff's own work.
// A prior end date within staffUndoMinRemaining is never re-applied.
func decideStaffUndo(orig staffActionKind, prior staffPriorState, applied staffAppliedState, live staffTargetState, now int64) staffUndoVerdict {
	switch orig {
	case staffKindBan, staffKindMute, staffKindUnban, staffKindUnmute:
	default:
		// Kick removes a member who may rejoin and has nothing to put back; like
		// decideStaffAction's default, anything else is a failed lookup.
		return staffUndoSkip(staffReasonFailLookup)
	}
	if live.Status == gotgbot.ChatMemberStatusCreator || live.Status == gotgbot.ChatMemberStatusAdministrator {
		return staffUndoSkip(staffReasonSkipTargetAdmin)
	}
	if prior.Status == "" {
		return staffUndoSkip(staffReasonSkipNoPriorState)
	}
	if (orig == staffKindBan || orig == staffKindMute) && applied.Until != 0 && applied.Until <= now {
		return staffUndoSkip(staffReasonSkipRestrictionEnded)
	}
	if !staffUndoLeftBehind(orig, applied, live) {
		return staffUndoSkip(staffReasonSkipChangedSince)
	}
	switch orig {
	case staffKindBan:
		return decideStaffUndoBan(prior, now)
	case staffKindMute:
		return decideStaffUndoMute(prior, now)
	case staffKindUnban:
		return decideStaffUndoUnban(prior, now)
	case staffKindUnmute:
		return decideStaffUndoUnmute(prior, now)
	}
	return staffUndoSkip(staffReasonFailLookup)
}

// decideStaffUndoUnban is the unban column: it puts a ban back only when the ban
// the unban lifted still has time to run, with its original end date.
func decideStaffUndoUnban(prior staffPriorState, now int64) staffUndoVerdict {
	if prior.Status != gotgbot.ChatMemberStatusKicked {
		return staffUndoSkip(staffReasonSkipNoPriorState)
	}
	if staffUndoEnded(prior.Until, now) {
		return staffUndoSkip(staffReasonSkipRestrictionEnded)
	}
	return staffUndoDo(staffCallBan, staffReasonUndoneBanRestored, prior.Until)
}

// decideStaffUndoUnmute is the unmute column: it puts the recorded restriction
// back, with its permission set and end date, only while it still has time to run.
func decideStaffUndoUnmute(prior staffPriorState, now int64) staffUndoVerdict {
	if prior.Status != gotgbot.ChatMemberStatusRestricted {
		return staffUndoSkip(staffReasonSkipNoPriorState)
	}
	if staffUndoEnded(prior.Until, now) {
		return staffUndoSkip(staffReasonSkipRestrictionEnded)
	}
	if prior.Perms == nil {
		return staffUndoSkip(staffReasonSkipNoPriorState)
	}
	return staffUndoRestore(prior)
}

// decideStaffUndoBan is the ban column: a ban over a member or a departed target
// is lifted, a ban over a restriction or a shorter ban still running puts that
// back.
func decideStaffUndoBan(prior staffPriorState, now int64) staffUndoVerdict {
	switch prior.Status {
	case gotgbot.ChatMemberStatusMember, gotgbot.ChatMemberStatusLeft:
		return staffUndoDo(staffCallUnban, staffReasonUndoneUnbanned, 0)
	case gotgbot.ChatMemberStatusRestricted:
		if staffUndoEnded(prior.Until, now) {
			return staffUndoDo(staffCallUnban, staffReasonUndoneUnbanned, 0)
		}
		if prior.Perms == nil {
			return staffUndoSkip(staffReasonSkipNoPriorState)
		}
		return staffUndoRestore(prior)
	case gotgbot.ChatMemberStatusKicked:
		if staffUndoEnded(prior.Until, now) {
			return staffUndoDo(staffCallUnban, staffReasonUndoneUnbanned, 0)
		}
		return staffUndoDo(staffCallBan, staffReasonUndoneBanRestored, prior.Until)
	}
	return staffUndoSkip(staffReasonSkipNoPriorState)
}

// decideStaffUndoMute is the mute column: a mute over a member is lifted, a mute
// over a restriction still running puts that restriction back.
func decideStaffUndoMute(prior staffPriorState, now int64) staffUndoVerdict {
	switch prior.Status {
	case gotgbot.ChatMemberStatusMember:
		return staffUndoDo(staffCallUnmute, staffReasonUndoneUnmuted, 0)
	case gotgbot.ChatMemberStatusRestricted:
		if staffUndoEnded(prior.Until, now) {
			return staffUndoDo(staffCallUnmute, staffReasonUndoneUnmuted, 0)
		}
		if prior.Perms == nil {
			return staffUndoSkip(staffReasonSkipNoPriorState)
		}
		return staffUndoRestore(prior)
	}
	return staffUndoSkip(staffReasonSkipNoPriorState)
}

// staffServiceUserIDs are Telegram's own accounts: the service account, the
// anonymous-admin placeholder and the replies bot. They are never a target.
var staffServiceUserIDs = map[int64]bool{777000: true, 1087968824: true, 136817688: true}
