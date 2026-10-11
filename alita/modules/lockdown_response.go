package modules

import (
	"context"
	"errors"
	"slices"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/db/lockdown"
	"github.com/divkix/Alita_Robot/alita/db/models"
)

// "Ban N recent joiners" bans the accounts of a lockdown's burst for good. This file is
// the policy and safety core of that: one function decides and bans one account, so
// that no path to a ban with no end date skips the checks. The run, its card and its
// button sit on top of lockdownResponseBanOne.

const (
	// lockdownBanSkipAdmin is the reason of an account that is a live creator or
	// administrator.
	lockdownBanSkipAdmin = "admin"
	// lockdownBanSkipApproved is the reason of an approved user of the group.
	lockdownBanSkipApproved = "approved"
	// lockdownBanSkipBanned is the reason of an account someone else already banned.
	lockdownBanSkipBanned = "already_banned"
	// lockdownBanSkipSelf is the reason of the bot's own account.
	lockdownBanSkipSelf = "bot_self"
	// lockdownBanSkipLetIn is the reason of an account the lockdown recorded as exempt,
	// because a live creator or administrator (or an anonymous admin) let them in.
	lockdownBanSkipLetIn = models.BurstReasonLetIn
	// lockdownBanReasonRequest marks a banned burst row whose account was a join request
	// the lockdown declined, so the report can say so.
	lockdownBanReasonRequest = "request"
)

// lockdownResponseClaimStates are the states in which a joiner row may be claimed (to
// acting) before its account is banned for good. Exempt is never in the list: someone
// an admin let in is skipped, not claimed. Acting and unbanning belong to the worker and
// the lift, which the run waits for instead.
var lockdownResponseClaimStates = []string{
	models.JoinerStatePending,
	models.JoinerStateBanned,
	models.JoinerStateBanFailed,
	models.JoinerStateDeclined,
	models.JoinerStateDeclineFailed,
	models.JoinerStateKept,
	models.JoinerStateCancelled,
}

// decideLockdownResponseBan decides, from the account's live status alone, whether
// "Ban N recent joiners" bans it. It is pure so the whole table is tested without a
// bot. banUntil is the end date the lockdown's own ban carries (the account's joiner
// row), or 0 for an account the lockdown never recorded.
//
// A creator or administrator is never banned (skip admin). A kicked account is banned
// again only when it is under the lockdown's own ban (isLockdownBan), which the new ban
// with no end date upgrades; any other ban, with no end date or with another one, was
// placed by someone else and is left exactly as it is (skip already_banned), because
// banChatMember on a kicked account would replace that ban. Every other status (member,
// restricted, left) is banned.
func decideLockdownResponseBan(m gotgbot.MergedChatMember, banUntil int64) (ban bool, skip string) {
	switch m.Status {
	case gotgbot.ChatMemberStatusCreator, gotgbot.ChatMemberStatusAdministrator:
		return false, lockdownBanSkipAdmin
	case gotgbot.ChatMemberStatusKicked:
		if isLockdownBan(m, banUntil) {
			return true, ""
		}
		return false, lockdownBanSkipBanned
	}
	return true, ""
}

// moveLockdownBurst moves a burst row and logs a failed write; the row then stays in its
// claimed state until the stale-claim release returns it to the queue.
func moveLockdownBurst(id uint, from, to, reason, detail string, countAttempt bool) {
	if _, err := lockdown.MoveBurst(id, from, to, reason, detail, countAttempt); err != nil {
		log.Errorf("[Lockdown] burst row %d could not move from %s to %s: %v", id, from, to, err)
	}
}

// lockdownResponseBanOne decides and bans one account of the burst for good. The burst
// row is already claimed to acting by the caller. It reports whether the row reached a
// final state (banned, skipped or failed for good); false means it went back to the
// queue.
//
// The account is skipped, with a call to nothing, when it is an approved user (approved
// is read fresh by the caller), the bot itself, or a joiner the lockdown recorded as
// exempt (let in by an admin, D-22), also when that row turned exempt after the run was
// queued, because the joiner row is read fresh here. Otherwise the joiner row, if the
// lockdown recorded one, is claimed to acting before anything is sent: that keeps the
// worker's marker ban and the lift's unban away from the account for the whole call, so
// neither can overwrite or undo a ban with no end date. A joiner row held by the worker
// or the lift (acting, unbanning) or lost in a race sends the account back to the queue
// without an attempt.
//
// One paced unit then reads the live status and, when decideLockdownResponseBan says
// ban, sends banChatMember with empty options: no end date and no message removal. A
// ban moves the burst row to banned (reason request for a declined join request) and
// the joiner row to kept with detail response_ban, which the lift leaves banned. Every
// other outcome puts the joiner row back in the state and detail it came from. A rate
// limit or a shutdown sends the burst row back without an attempt; another failure
// costs one, and the third is final (failed).
//
// A crash between the claim and the move leaves the joiner row acting; ReleaseStaleClaims
// returns it to pending (active lockdown) or banned (lifting), and the worker's and the
// lift's live checks then keep a ban with no end date (Phase 4 rules). The burst row is
// returned to the queue by ReleaseStaleBurstClaims.
func lockdownResponseBanOne(
	ctx context.Context,
	b *gotgbot.Bot,
	ld models.ChatLockdown,
	row models.ChatLockdownBurst,
	approved map[int64]bool,
) bool {
	skip := func(reason string) bool {
		moveLockdownBurst(row.ID, models.BurstStateActing, models.BurstStateSkipped, reason, "", false)
		return true
	}
	requeue := func() bool {
		moveLockdownBurst(row.ID, models.BurstStateActing, models.BurstStateQueued, "", "", false)
		return false
	}

	if approved[row.UserID] {
		return skip(lockdownBanSkipApproved)
	}
	if row.UserID == b.Id {
		return skip(lockdownBanSkipSelf)
	}

	joiner, err := lockdown.GetJoinerOfUserFresh(ld.ID, row.UserID)
	if err != nil {
		return requeue()
	}

	var banUntil int64
	claimed := false
	request := false
	if joiner != nil {
		banUntil = joiner.BanUntil
		request = joiner.JoinPath == models.JoinPathRequest
		switch {
		case joiner.State == models.JoinerStateExempt:
			return skip(lockdownBanSkipLetIn)
		case joiner.State == models.JoinerStateActing || joiner.State == models.JoinerStateUnbanning:
			return requeue()
		case slices.Contains(lockdownResponseClaimStates, joiner.State):
			won, claimErr := lockdown.ClaimJoiner(joiner.ID, joiner.State, models.JoinerStateActing)
			if claimErr != nil || !won {
				return requeue()
			}
			claimed = true
		}
	}
	restoreJoiner := func() {
		if claimed {
			moveLockdownJoiner(joiner.ID, models.JoinerStateActing, joiner.State, joiner.Detail, false)
		}
	}

	var (
		ban        bool
		skipReason string
	)
	err = lockdownPaced(ctx, func(callCtx context.Context) error {
		call, cancel := context.WithTimeout(callCtx, lockdownCallTimeout)
		defer cancel()
		ban, skipReason = false, ""
		live, lookupErr := b.GetChatMemberWithContext(call, ld.ChatID, row.UserID, nil)
		if lookupErr != nil {
			return lookupErr
		}
		if live == nil {
			return errors.New("getChatMember returned no member")
		}
		ban, skipReason = decideLockdownResponseBan(live.MergeChatMember(), banUntil)
		if !ban {
			return nil
		}
		_, banErr := b.BanChatMemberWithContext(call, ld.ChatID, row.UserID, &gotgbot.BanChatMemberOpts{})
		return banErr
	})

	switch lockdownClassify(ctx, err) {
	case lockdownCallDone:
		if !ban {
			restoreJoiner()
			return skip(skipReason)
		}
		reason := ""
		if request {
			reason = lockdownBanReasonRequest
		}
		moveLockdownBurst(row.ID, models.BurstStateActing, models.BurstStateBanned, reason, "", false)
		if claimed {
			moveLockdownJoiner(joiner.ID, models.JoinerStateActing, models.JoinerStateKept, models.JoinerDetailResponseBan, false)
		}
		return true
	case lockdownCallRetryFree:
		restoreJoiner()
		return requeue()
	}

	log.Warnf("[Lockdown] ban for good of user %d in chat %d failed: %v", row.UserID, ld.ChatID, err)
	restoreJoiner()
	if row.Attempts+1 >= lockdownMaxAttempts {
		moveLockdownBurst(row.ID, models.BurstStateActing, models.BurstStateFailed, "", telegramErrorDetail(err), true)
		return true
	}
	moveLockdownBurst(row.ID, models.BurstStateActing, models.BurstStateQueued, "", telegramErrorDetail(err), true)
	return false
}
