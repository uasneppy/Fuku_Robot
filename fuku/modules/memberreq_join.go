package modules

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/utils/tracing"
)

// The join side of the member requirements (D-01, D-02, D-08, D-11). memberReqJoinThenWelcome
// is the one entry of both welcome paths: the chat_member join and the join service message
// (through membershipDepsForJoin, after claimRecentJoinProcessing) and a captcha pass (through
// finishCaptchaPass, the button and the web verify alike), so the captcha always runs first and
// the paths cannot drift. Nothing here is reached while the enforcement switch is off.

// memberReqJoinTimeout bounds the whole check of one join: the settings read, the hold read and
// the executor with its one live getChatMember, its punishment and its notice.
const memberReqJoinTimeout = 15 * time.Second

// memberReqGuardTimeout bounds the captcha guard's hold read and its one re-mute call.
const memberReqGuardTimeout = 10 * time.Second

// errMemberReqHoldUnreadable is what the captcha guard returns when the member's hold could not
// be read. It carries IDs only and none of the database's own text, so nothing in it can read as
// a permanent unmute failure (isPermanentUnmuteError answers false for it explicitly).
var errMemberReqHoldUnreadable = errors.New("requirement hold unreadable")

// memberReqHoldRead reads a member's hold fresh. It is the one read the join entry and the
// captcha guard make of a hold, a variable so a test can count and fail it.
var memberReqHoldRead = memberreq.GetHoldFresh

// memberReqPunishedMute reports whether hold is a requirement mute that has taken effect: the
// member is muted by the requirements and their welcome is held until they fix it.
func memberReqPunishedMute(hold *models.ChatRequirementHold) bool {
	return hold != nil && hold.Action == models.RequirementActionMute && hold.PunishedAt != nil
}

// memberReqHoldWelcome records on the hold that a mute held the member's welcome, so the
// "I fixed it" handler sends it once the member is freed (D-02). A failed write is logged by IDs
// only: the welcome stays unsent either way.
func memberReqHoldWelcome(ctx context.Context, chatID, userID int64, hold *models.ChatRequirementHold) {
	recordCtx, cancel := memberReqRecordCtx(ctx)
	defer cancel()
	if _, err := memberreq.SetHoldWelcomeHeld(recordCtx, hold.ID); err != nil {
		log.Errorf("[MemberReq] recording the held welcome of user %d in chat %d failed: %s", userID, chatID, memberReqErrText(err))
	}
}

// memberReqJoinThenWelcome checks a new member against the group's requirements and then
// welcomes them, or does not. While memberReqLive() is false it sends the plain welcome and
// nothing else, so no commit before the switch is turned on changes a real join. The same plain
// welcome goes to a chat that is not a supergroup, a bot, the bot itself and a Telegram service
// account (never checked, D-11), a group whose settings cannot be read and a group with every
// rule off.
//
// With a rule on, the member's hold is read fresh first: a punished requirement mute (a member
// who posted before their captcha attempt existed, the D-08 race) records the held welcome and
// sends none, whatever Check on join says. With Check on join off the member is welcomed with no
// further read, except that while the AI rule is live (memberReqAILive) the join entry first sets
// the member's new-member marker, so the rule judges a joiner whose join call was unanswered, or
// who joined with Check on join off, at their first message instead of baselining them (D-21).
// Otherwise memberReqEnforce runs synchronously with one live getChatMember: a kick
// or ban sends no welcome, a mute sends none and has stored welcome_held on its hold, a warn posts
// its notice and the usual welcome follows, and a clean, exempt or unanswerable member is welcomed.
// Logs carry IDs and outcomes only.
func memberReqJoinThenWelcome(bot *gotgbot.Bot, ctx *ext.Context, user gotgbot.User) error {
	welcome := func() error { return SendWelcomeMessage(bot, ctx, user.Id, user.FirstName) }
	if !memberReqLive() {
		return welcome()
	}
	chat := ctx.EffectiveChat
	if chat == nil || chat.Type != "supergroup" || memberReqNeverChecked(bot, user) {
		return welcome()
	}
	runCtx, cancel := context.WithTimeout(tracing.UpdateContext(ctx), memberReqJoinTimeout)
	defer cancel()

	settings, err := memberReqSettingsLookup(runCtx, chat.Id)
	if err != nil {
		log.Warnf("[MemberReq] the requirements of chat %d could not be read on the join of user %d: %s", chat.Id, user.Id, memberReqErrText(err))
		return welcome()
	}
	if settings == nil || !memberreq.AnyRuleOn(settings) {
		return welcome()
	}
	hold, err := memberReqHoldRead(runCtx, chat.Id, user.Id)
	if err != nil {
		log.Warnf("[MemberReq] the hold of user %d in chat %d could not be read on their join: %s", user.Id, chat.Id, memberReqErrText(err))
		return welcome()
	}
	if memberReqPunishedMute(hold) {
		memberReqHoldWelcome(runCtx, chat.Id, user.Id, hold)
		return nil
	}
	// A joiner is new whether or not Check on join is on: the marker makes the AI rule judge them at
	// this join or, when no answer came, at their first message, never baseline them (D-21).
	if memberReqAILive(settings) {
		memberReqAIMarkJoin(runCtx, chat.Id, user.Id)
	}
	if !settings.CheckOnJoin {
		return welcome()
	}

	snapshot := *settings
	result := memberReqEnforce(runCtx, bot, memberReqCase{
		Chat:     *chat,
		User:     user,
		Source:   memberReqFromJoin,
		Settings: &snapshot,
	})
	switch result.Outcome {
	case memberReqRemoved, memberReqMuted:
		return nil
	case memberReqHeld:
		if memberReqPunishedMute(result.Hold) {
			memberReqHoldWelcome(runCtx, chat.Id, user.Id, result.Hold)
			return nil
		}
	}
	return welcome()
}

// memberReqCaptchaGuard runs first in unmuteCaptchaUser, the one choke point of every captcha
// unmute (the pass, restoreCaptchaPermissions, restart recovery and the captcha_muted_users retry
// worker), so a captcha never frees a member the requirements muted (D-08, D-22). It reports
// handled true when the member is under a punished requirement mute: it re-applies
// MutedPermissions with until_date the hold's own marker through memberReqPaced instead of
// unmuting, which keeps the same absolute marker valid for "I fixed it", and the caller treats
// the unmute as done (a nil error) or as failed (the re-mute's error, retried like any failed
// unmute).
//
// While memberReqLive() is false it does nothing at all: no hold read, no call, no error, so no
// captcha unmute changes before enforcement is turned on. Once it is on, a hold that cannot be
// read fails closed: the error is returned with handled false and the member is not unmuted,
// because unmuting on an unknown hold could free a member the requirements muted. Every caller
// handles that error as a failed, non-permanent Telegram unmute (a captcha_muted_users row for
// the 5-minute worker, or a kept row), and the member is unmuted at the next retry that can read.
func memberReqCaptchaGuard(bot *gotgbot.Bot, chatID, userID int64) (handled bool, err error) {
	if !memberReqLive() {
		return false, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), memberReqGuardTimeout)
	defer cancel()
	hold, readErr := memberReqHoldRead(ctx, chatID, userID)
	if readErr != nil {
		log.Warnf("[MemberReq] the hold of user %d in chat %d could not be read, so their captcha unmute waits: %s", userID, chatID, memberReqErrText(readErr))
		return false, fmt.Errorf("%w: user %d in chat %d", errMemberReqHoldUnreadable, userID, chatID)
	}
	if !memberReqPunishedMute(hold) {
		return false, nil
	}
	err = memberReqPaced(ctx, func(callCtx context.Context) error {
		_, restrictErr := bot.RestrictChatMemberWithContext(callCtx, chatID, userID, MutedPermissions,
			&gotgbot.RestrictChatMemberOpts{UntilDate: hold.MuteUntil})
		return restrictErr
	})
	return true, err
}

// memberReqResetTimeout bounds the whole rejoin reset of one member: the hold statement, the
// live getChatMember, the group read and the one lift.
const memberReqResetTimeout = 20 * time.Second

// memberReqJoinDate is the date of a join as the reset reads it: Telegram's Unix date, or now
// when the update carries none.
func memberReqJoinDate(unix int64) time.Time {
	if unix <= 0 {
		return time.Now()
	}
	return time.Unix(unix, 0)
}

// memberReqJoinReset settles the requirement holds an earlier membership of user left in chat,
// at the join that won its dedupe claim (MembershipDeps.Reset; D-07, D-22). A member who was
// muted by the requirements and left and rejoined is invisible to the chat_member handlers
// (restricted to restricted is neither a leave nor a join), so the rejoin's service message is
// the one place that ends that episode: without it the member would stay muted for the marker's
// 330 days with no button.
//
// It deletes only the holds created before joinDate (memberreq.TakeHoldsBefore), so a hold the
// watcher created for this very membership, because the member posted before their join was
// processed, survives. For a mute hold, punished or still pending (the restrict may have taken
// effect before the hold was marked), it reads the member's live record and lifts the
// mute only while that record still carries the hold's marker (isMemberReqMute): an admin's,
// staff's or captcha's mute replaced the end date and is never ours to lift. The lift uses only
// memberReqUnmutePermissions (the group's live defaults, or an active lockdown's stored
// permissions) and memberReqLiftMute. When any step of the lift fails the hold is put back with
// its ID and notice ID (memberreq.RestoreHold), so the next join retries and the member still
// has a hold. Nothing here ever unbans, and every log line carries IDs only.
//
// It is deliberately not behind the enforcement switch: it only deletes holds or lifts a
// requirement mute, and while the switch is off no hold exists.
func memberReqJoinReset(bot *gotgbot.Bot, chat *gotgbot.Chat, user gotgbot.User, joinDate time.Time) {
	ctx, cancel := context.WithTimeout(context.Background(), memberReqResetTimeout)
	defer cancel()
	holds, err := memberreq.TakeHoldsBefore(ctx, chat.Id, user.Id, joinDate)
	if err != nil {
		log.Errorf("[MemberReq] resetting the holds of user %d in chat %d on their join failed: %s", user.Id, chat.Id, memberReqErrText(err))
		return
	}
	for i := range holds {
		// A pending mute hold too (WR-03): the restrict may have taken effect before the hold was
		// marked punished, and this reset deletes the only record of its marker. The lift checks the
		// live marker, so a pending hold whose restrict never landed is a no-op.
		if holds[i].Action != models.RequirementActionMute {
			continue
		}
		liftErr := memberReqLiftOldMute(ctx, bot, chat.Id, user.Id, &holds[i])
		if liftErr == nil {
			continue
		}
		restoreCtx, restoreCancel := memberReqRecordCtx(ctx)
		if restoreErr := memberreq.RestoreHold(restoreCtx, &holds[i]); restoreErr != nil {
			log.Errorf("[MemberReq] restoring the hold of user %d in chat %d after a failed rejoin lift failed: %s", user.Id, chat.Id, memberReqErrText(restoreErr))
		}
		restoreCancel()
		log.Warnf("[MemberReq] the requirement mute of user %d in chat %d could not be lifted at their rejoin, so the hold was kept: %s",
			user.Id, chat.Id, memberReqErrText(liftErr))
	}
}

// memberReqLiftOldMute lifts a requirement mute at the rejoin, only while the member's
// live record still carries the hold's marker. A record that does not carry it (an admin's mute,
// no restriction left) is nothing to lift and no error.
func memberReqLiftOldMute(ctx context.Context, bot *gotgbot.Bot, chatID, userID int64, hold *models.ChatRequirementHold) error {
	member, err := memberReqLiveMember(ctx, bot, chatID, userID)
	if err != nil {
		return err
	}
	if !isMemberReqMute(member, hold.MuteUntil) {
		return nil
	}
	perms, _, err := memberReqUnmutePermissions(ctx, bot, chatID)
	if err != nil {
		return err
	}
	return memberReqLiftMute(ctx, bot, chatID, userID, perms)
}
