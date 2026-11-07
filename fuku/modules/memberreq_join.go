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
// further read. Otherwise memberReqEnforce runs synchronously with one live getChatMember: a kick
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
