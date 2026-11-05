package modules

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db/approvals"
	"github.com/uasneppy/fuku_robot/fuku/db/captcha"
	"github.com/uasneppy/fuku_robot/fuku/db/lang"
	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/ratelimit"
)

// The member requirements executor. memberReqEnforce is the one place that decides whether a
// member who may be breaking a rule is punished, punishes once and posts the one notice. The
// message watcher and the join check call it; nothing here is reached by a handler yet.
//
// Everything that cannot be answered punishes nobody: an unreadable setting, an approvals or
// captcha read that failed, a getChatMember that failed. Logs carry chat and user IDs and the
// outcome only, never a name, a rule text or a notice text.

// memberReqPacer paces every Telegram call of the executor. Its slot reservation and
// retry_after block live in Redis, so every replica shares one budget, separate from the
// staff and lockdown budgets. It is a variable so tests can install a fast one.
var memberReqPacer = ratelimit.NewTelegramPacer(ratelimit.TelegramPacerOptions{
	NextKey:    "fuku:memberreq:pace:next",
	BlockKey:   "fuku:memberreq:pace:block",
	Interval:   100 * time.Millisecond,
	MaxRetries: 3,
	MaxWait:    60 * time.Second,
})

// memberReqCallTimeout bounds one Telegram request of the executor.
const memberReqCallTimeout = 10 * time.Second

// memberReqRecordTimeout bounds one database record write that must not be cancelled with
// the run: marking a hold punished and storing its notice ID.
const memberReqRecordTimeout = 5 * time.Second

// memberReqPaced runs one Telegram call under the requirement pacer: it waits for its slot,
// bounds the call and, on a 429, waits out Telegram's retry_after and repeats the same
// closure. A slot more than the pacer's MaxWait away fails at once as
// ratelimit.ErrRateLimited, with no Telegram request.
func memberReqPaced(ctx context.Context, call func(context.Context) error) error {
	return memberReqPacer.Do(ctx, func(callCtx context.Context) error {
		bounded, cancel := context.WithTimeout(callCtx, memberReqCallTimeout)
		defer cancel()
		return call(bounded)
	})
}

// memberReqSource says which entry reached the executor.
type memberReqSource int

const (
	// memberReqFromMessage is the message watcher.
	memberReqFromMessage memberReqSource = iota
	// memberReqFromJoin is the join check.
	memberReqFromJoin
)

// memberReqCase is what a caller knows about a member who may break a rule: the group, the
// member as the update showed them, which entry reached the executor, the message (zero for
// a join) and the group's stored settings.
type memberReqCase struct {
	Chat                   gotgbot.Chat
	User                   gotgbot.User
	Source                 memberReqSource
	MessageID, MessageDate int64
	Settings               *models.ChatRequirements
}

// memberReqOutcome is what the executor did.
type memberReqOutcome int

const (
	// memberReqNone: nobody was punished and nothing was recorded (an exemption, an
	// unknown state, a clean member, a failed lookup).
	memberReqNone memberReqOutcome = iota
	// memberReqHeld: the member already has a hold, so nothing was added.
	memberReqHeld
	// memberReqWarned: a warn was recorded (a warn-limit mute is still this outcome).
	memberReqWarned
	// memberReqRemoved: a kick or a ban took effect.
	memberReqRemoved
	// memberReqMuted: the requirement mute took effect.
	memberReqMuted
	// memberReqNotApplied: this call won the claim but the punishment did not take effect,
	// so the hold stays pending and nothing was posted.
	memberReqNotApplied
)

// memberReqResult is the outcome and the hold it concerns (nil when there is none).
type memberReqResult struct {
	Outcome memberReqOutcome
	Hold    *models.ChatRequirementHold
}

var (
	// memberReqApplyWarn is the warn module's core, a variable so a test can look inside the
	// warn lock or make the warn write fail.
	memberReqApplyWarn = applyWarn
	// memberReqApprovedLookup is the tri-state approvals read: an error is an unknown.
	memberReqApprovedLookup = approvals.IsUserApprovedContext
	// memberReqCaptchaPendingLookup and memberReqCaptchaAttemptLookup are the two pending
	// captcha reads, variables so a test can make one fail.
	memberReqCaptchaPendingLookup = captcha.HasPendingCaptchaAttemptsContext
	memberReqCaptchaAttemptLookup = captcha.GetCaptchaAttempt
)

// memberReqMuteUntil is the marker end date, in Unix seconds, of a requirement mute placed at
// now: 330 days ahead, inside Telegram's finite-restriction window, so no deliberate mute
// shares it (D-08). It is the lockdown's own span.
func memberReqMuteUntil(now time.Time) int64 {
	return lockdownBanUntil(now)
}

// memberReqErrText is an error as a log line may carry it: a Telegram error by its code
// alone, anything else as it is. Telegram's own description is left out because it can echo
// request text.
func memberReqErrText(err error) string {
	var tgErr *gotgbot.TelegramError
	if errors.As(err, &tgErr) {
		return "telegram error " + strconv.Itoa(tgErr.Code)
	}
	return err.Error()
}

// memberReqTranslator is the translator of the group's language, uk when it is unknown.
func memberReqTranslator(ctx context.Context, chatID int64) *i18n.Translator {
	return i18n.MustNewTranslator(lang.GetGroupLanguage(ctx, chatID))
}

// memberReqLiveMember reads the member's live record through the requirement pacer. It is
// the authoritative admin check and the source of the current names.
func memberReqLiveMember(ctx context.Context, b *gotgbot.Bot, chatID, userID int64) (gotgbot.MergedChatMember, error) {
	var member gotgbot.ChatMember
	err := memberReqPaced(ctx, func(callCtx context.Context) error {
		var getErr error
		member, getErr = b.GetChatMemberWithContext(callCtx, chatID, userID, nil)
		return getErr
	})
	if err != nil {
		return gotgbot.MergedChatMember{}, err
	}
	if member == nil {
		return gotgbot.MergedChatMember{}, errors.New("empty chat member answer")
	}
	return member.MergeChatMember(), nil
}

// memberReqLiveEligible reports whether a live record is of someone the requirements may
// act on: a plain member, or a restricted one who is still in the group. A creator, an
// administrator, someone who left or was kicked, a bot and an answer it does not know are not.
func memberReqLiveEligible(m gotgbot.MergedChatMember) bool {
	if m.User.IsBot {
		return false
	}
	switch m.Status {
	case gotgbot.ChatMemberStatusMember:
		return true
	case gotgbot.ChatMemberStatusRestricted:
		return m.IsMember
	}
	return false
}

// memberReqNeverChecked is true for the bot itself, bots and Telegram's service accounts.
func memberReqNeverChecked(b *gotgbot.Bot, u gotgbot.User) bool {
	return u.Id <= 0 || u.IsBot || u.Id == b.Id || staffServiceUserIDs[u.Id]
}

// memberReqPending reports whether the member has a pending captcha attempt, or whether that
// cannot be known: either read failing is "cannot be known", and the caller punishes nobody.
func memberReqPending(ctx context.Context, c memberReqCase) (pending bool, err error) {
	anyPending, err := memberReqCaptchaPendingLookup(ctx, c.Chat.Id)
	if err != nil {
		return false, err
	}
	if !anyPending {
		return false, nil
	}
	attempt, err := memberReqCaptchaAttemptLookup(c.User.Id, c.Chat.Id)
	if err != nil {
		return false, err
	}
	return attempt != nil, nil
}

// memberReqExempt reports whether the member must not be punished now: approved, with a
// pending captcha, or a read of either that failed. Each is yes, no or unknown, and an
// unknown is exempt, so nobody is punished on a state that could not be read. Approvals go
// through the tri-state reader, never the cached predicate that hides a failed load.
func memberReqExempt(ctx context.Context, c memberReqCase) bool {
	approved, err := memberReqApprovedLookup(ctx, c.Chat.Id, c.User.Id)
	if err != nil {
		log.Warnf("[MemberReq] approval of user %d in chat %d is unknown: %s", c.User.Id, c.Chat.Id, memberReqErrText(err))
		return true
	}
	if approved {
		return true
	}
	pending, err := memberReqPending(ctx, c)
	if err != nil {
		log.Warnf("[MemberReq] captcha state of user %d in chat %d is unknown: %s", c.User.Id, c.Chat.Id, memberReqErrText(err))
		return true
	}
	return pending
}

// memberReqPriorHold reads the member's hold. A hold that is a pending one older than
// memberreq.StaleClaim comes back as stale (a failed episode the reclaim may retry); any
// other hold comes back as held. An unreadable hold is an error.
func memberReqPriorHold(ctx context.Context, c memberReqCase) (stale, held *models.ChatRequirementHold, err error) {
	hold, err := memberreq.GetHoldFresh(ctx, c.Chat.Id, c.User.Id)
	if err != nil || hold == nil {
		return nil, nil, err
	}
	if hold.PunishedAt == nil && hold.CreatedAt.Before(time.Now().Add(-memberreq.StaleClaim)) {
		return hold, nil, nil
	}
	return nil, hold, nil
}

// memberReqLiveViolation makes the one live getChatMember and returns the member's current
// record and the mask of rules the live names break. ok is false when the lookup failed, the
// member is exempt by the live record, or nothing is broken any more.
func memberReqLiveViolation(ctx context.Context, b *gotgbot.Bot, c memberReqCase) (member gotgbot.MergedChatMember, broken int, ok bool) {
	member, err := memberReqLiveMember(ctx, b, c.Chat.Id, c.User.Id)
	if err != nil {
		log.Warnf("[MemberReq] live record of user %d in chat %d is unknown: %s", c.User.Id, c.Chat.Id, memberReqErrText(err))
		return member, 0, false
	}
	if !memberReqLiveEligible(member) {
		return member, 0, false
	}
	broken = memberReqBroken(c.Settings, memberReqNamesOf(member.User))
	return member, broken, broken != 0
}

// memberReqTakeHold claims the member's hold for this violation (insert-first), or reclaims
// the stale pending one it was given. It returns the hold and true only for the one caller
// that won. keepUntil is the marker of a mute that is already live (zero for none): the hold
// keeps it instead of a new one, so the marker on the member and on the hold stay equal.
func memberReqTakeHold(ctx context.Context, c memberReqCase, stale *models.ChatRequirementHold, broken, action int, keepUntil int64) (*models.ChatRequirementHold, bool) {
	now := time.Now()
	hold := &models.ChatRequirementHold{
		ChatID:      c.Chat.Id,
		UserID:      c.User.Id,
		Rules:       broken,
		Action:      action,
		WelcomeHeld: c.Source == memberReqFromJoin && action == models.RequirementActionMute,
		CreatedAt:   now,
	}
	if action == models.RequirementActionMute {
		hold.MuteUntil = memberReqMuteUntil(now)
		if keepUntil != 0 {
			hold.MuteUntil = keepUntil
		}
	}
	if stale != nil {
		won, id, err := memberreq.ReclaimStaleHold(ctx, hold)
		if err != nil {
			log.Errorf("[MemberReq] reclaim of the hold of user %d in chat %d failed: %s", c.User.Id, c.Chat.Id, memberReqErrText(err))
			return nil, false
		}
		hold.ID = id
		return hold, won
	}
	won, err := memberreq.ClaimHold(ctx, hold)
	if err != nil {
		log.Errorf("[MemberReq] claim of the hold of user %d in chat %d failed: %s", c.User.Id, c.Chat.Id, memberReqErrText(err))
		return nil, false
	}
	return hold, won
}

// memberReqRecordCtx is a context for a record write that must survive the run's
// cancellation, with its own bound.
func memberReqRecordCtx(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), memberReqRecordTimeout)
}

// memberReqMarkPunished stamps the hold punished. A failure is logged and leaves the hold
// pending, which the two-minute reclaim retries.
func memberReqMarkPunished(ctx context.Context, c memberReqCase, hold *models.ChatRequirementHold) {
	recordCtx, cancel := memberReqRecordCtx(ctx)
	defer cancel()
	if _, err := memberreq.MarkHoldPunished(recordCtx, hold.ID); err != nil {
		log.Errorf("[MemberReq] marking the hold of user %d in chat %d punished failed: %s", c.User.Id, c.Chat.Id, memberReqErrText(err))
	}
}

// memberReqWarnStep records the warn through the warn module's own path (so it counts toward
// the group's limit and its action) with the hold marked punished inside the warn lock, right
// after the warn is stored. It sends no message of the warn module's.
func memberReqWarnStep(ctx context.Context, b *gotgbot.Bot, c memberReqCase, tr *i18n.Translator, hold *models.ChatRequirementHold) (memberReqOutcome, *warnApplyResult) {
	reason := memberReqText(tr, "mreq_warn_reason", i18n.TranslationParams{"rules": memberReqRulesText(tr, hold.Rules)})
	chat := c.Chat
	res, err := memberReqApplyWarn(ctx, b, &chat, c.User.Id, reason, func() {
		memberReqMarkPunished(ctx, c, hold)
	})
	if err != nil {
		log.Errorf("[MemberReq] warn of user %d in chat %d did not complete (recorded %t): %s", c.User.Id, c.Chat.Id, res.Recorded, memberReqErrText(err))
		memberReqBackoff(ctx, c, hold)
		return memberReqNotApplied, nil
	}
	if res.LimitApplied && (res.Mode == "kick" || res.Mode == "ban") {
		return memberReqRemoved, &res
	}
	return memberReqWarned, &res
}

// memberReqRefused reports whether a failed Telegram call is a definite refusal: a 4xx other
// than 429 (by code, never by wording), or the pacer giving up on a 429 or a long block
// (ratelimit.ErrRateLimited, where no request may have been made at all). Anything else (a 5xx,
// a timeout, a dropped connection) says nothing about whether the request took effect.
func memberReqRefused(err error) bool {
	return isTelegramClientError(err) || errors.Is(err, ratelimit.ErrRateLimited)
}

// memberReqBackoff refreshes created_at of the pending hold, so the member's next messages are
// the held path (no Telegram call) until memberreq.StaleClaim has passed, and then one winner
// reclaims it. A failed write is logged; the hold is then reclaimable earlier, never lost.
func memberReqBackoff(ctx context.Context, c memberReqCase, hold *models.ChatRequirementHold) {
	recordCtx, cancel := memberReqRecordCtx(ctx)
	defer cancel()
	if _, err := memberreq.BackoffHold(recordCtx, hold.ID); err != nil {
		log.Errorf("[MemberReq] backing off the hold of user %d in chat %d failed: %s", c.User.Id, c.Chat.Id, memberReqErrText(err))
	}
}

// memberReqSendFailed decides what a punishment call that did not answer success means (research
// S-05). A definite refusal backs the hold off with no notice. Any other answer of a mute is
// checked with one live getChatMember against the stored marker: present means the mute took
// effect (the answer was lost), absent backs off, and an unreadable record leaves the hold
// pending for the reclaim, which checks the marker first. A kick or ban with an unknown answer
// leaves the hold pending and posts nothing. It returns the outcome only when the punishment
// proved applied.
func memberReqSendFailed(ctx context.Context, b *gotgbot.Bot, c memberReqCase, hold *models.ChatRequirementHold, applied memberReqOutcome, err error) memberReqOutcome {
	log.Warnf("[MemberReq] punishment %d of user %d in chat %d was not confirmed: %s", hold.Action, c.User.Id, c.Chat.Id, memberReqErrText(err))
	switch {
	case memberReqRefused(err):
		memberReqBackoff(ctx, c, hold)
	case hold.Action == models.RequirementActionMute:
		live, liveErr := memberReqLiveMember(ctx, b, c.Chat.Id, c.User.Id)
		switch {
		case liveErr != nil:
			log.Warnf("[MemberReq] the mute of user %d in chat %d could not be verified: %s", c.User.Id, c.Chat.Id, memberReqErrText(liveErr))
		case isMemberReqMute(live, hold.MuteUntil):
			memberReqMarkPunished(ctx, c, hold)
			return applied
		default:
			memberReqBackoff(ctx, c, hold)
		}
	}
	return memberReqNotApplied
}

// memberReqSendStep makes one paced Telegram call of a punishment. When Telegram answered
// success the hold is marked punished and the outcome is returned; otherwise
// memberReqSendFailed decides, and nothing is recorded as punished unless the marker proves it.
func memberReqSendStep(ctx context.Context, b *gotgbot.Bot, c memberReqCase, hold *models.ChatRequirementHold, outcome memberReqOutcome, call func(context.Context) error) memberReqOutcome {
	if err := memberReqPaced(ctx, call); err != nil {
		return memberReqSendFailed(ctx, b, c, hold, outcome, err)
	}
	memberReqMarkPunished(ctx, c, hold)
	return outcome
}

// memberReqApplyStep applies the hold's punishment. A warn is marked punished inside its own
// lock; every other punishment is marked here once it took effect. alreadyMuted says the live
// record already carries this hold's mute marker, so a mute is confirmed and not sent again.
func memberReqApplyStep(ctx context.Context, b *gotgbot.Bot, c memberReqCase, tr *i18n.Translator, hold *models.ChatRequirementHold, alreadyMuted bool) (memberReqOutcome, *warnApplyResult) {
	switch hold.Action {
	case models.RequirementActionWarn:
		return memberReqWarnStep(ctx, b, c, tr, hold)
	case models.RequirementActionKick:
		return memberReqSendStep(ctx, b, c, hold, memberReqRemoved, func(callCtx context.Context) error {
			return kickMemberContext(callCtx, b, c.Chat.Id, c.User.Id)
		}), nil
	case models.RequirementActionBan:
		return memberReqSendStep(ctx, b, c, hold, memberReqRemoved, func(callCtx context.Context) error {
			_, err := b.BanChatMemberWithContext(callCtx, c.Chat.Id, c.User.Id, nil)
			return err
		}), nil
	case models.RequirementActionMute:
		if alreadyMuted {
			// A reclaimed mute whose marker is live: the earlier answer was lost, never mute again.
			memberReqMarkPunished(ctx, c, hold)
			return memberReqMuted, nil
		}
		return memberReqSendStep(ctx, b, c, hold, memberReqMuted, func(callCtx context.Context) error {
			_, err := b.RestrictChatMemberWithContext(callCtx, c.Chat.Id, c.User.Id, MutedPermissions,
				&gotgbot.RestrictChatMemberOpts{UntilDate: hold.MuteUntil})
			return err
		}), nil
	}
	return memberReqNotApplied, nil
}

// memberReqAnnounce posts the one notice of the episode and stores its message ID on the hold.
// A failure is logged by IDs only; the punishment stays.
func memberReqAnnounce(ctx context.Context, b *gotgbot.Bot, c memberReqCase, tr *i18n.Translator, user gotgbot.User, hold *models.ChatRequirementHold, warn *warnApplyResult) {
	text := memberReqNoticeText(tr, user, hold.Rules, hold.Action, warn)
	msgID, err := memberReqPostNotice(ctx, b, c.Chat.Id, text, memberReqNoticeKeyboard(tr, hold.Action, hold.ID, user.Id))
	if err != nil {
		log.Warnf("[MemberReq] the notice for user %d in chat %d was not delivered: %s", c.User.Id, c.Chat.Id, memberReqErrText(err))
		return
	}
	recordCtx, cancel := memberReqRecordCtx(ctx)
	defer cancel()
	if _, err := memberreq.SetHoldNotice(recordCtx, hold.ID, msgID); err != nil {
		log.Warnf("[MemberReq] storing the notice of user %d in chat %d failed: %s", c.User.Id, c.Chat.Id, memberReqErrText(err))
	}
}

// memberReqPunish applies the claimed hold's punishment and, once it took effect, posts the
// notice. A punishment that did not take effect leaves the pending hold and posts nothing.
func memberReqPunish(ctx context.Context, b *gotgbot.Bot, c memberReqCase, user gotgbot.User, hold *models.ChatRequirementHold, alreadyMuted bool) memberReqResult {
	tr := memberReqTranslator(ctx, c.Chat.Id)
	outcome, warn := memberReqApplyStep(ctx, b, c, tr, hold, alreadyMuted)
	if outcome == memberReqNotApplied {
		return memberReqResult{Outcome: outcome, Hold: hold}
	}
	memberReqAnnounce(ctx, b, c, tr, user, hold, warn)
	return memberReqResult{Outcome: outcome, Hold: hold}
}

// memberReqEnforce is the executor. Its order is fixed and every step that cannot be
// answered returns memberReqNone with no hold:
//
//  1. the bot itself, bots and Telegram service accounts;
//  2. an approved member or a pending captcha attempt (a read that failed counts as exempt);
//  3. the member's hold, read fresh: one that is not a stale pending one is Held;
//  4. one live getChatMember through the pacer, the admin check and the source of the names;
//  5. the rules on the live names, the strictest punishment of the broken ones;
//  6. the claim of the hold, before any punishment (one winner across replicas); a reclaimed
//     mute whose marker is already live is adopted, not applied again;
//  7. the punishment, then the one notice. A punishment Telegram refuses backs the hold off
//     and posts nothing (memberReqSendFailed).
func memberReqEnforce(ctx context.Context, b *gotgbot.Bot, c memberReqCase) memberReqResult {
	if memberReqNeverChecked(b, c.User) || memberReqExempt(ctx, c) {
		return memberReqResult{}
	}
	stale, held, err := memberReqPriorHold(ctx, c)
	if err != nil {
		log.Warnf("[MemberReq] hold of user %d in chat %d is unreadable: %s", c.User.Id, c.Chat.Id, memberReqErrText(err))
		return memberReqResult{}
	}
	if held != nil {
		return memberReqResult{Outcome: memberReqHeld, Hold: held}
	}
	member, broken, ok := memberReqLiveViolation(ctx, b, c)
	if !ok {
		return memberReqResult{}
	}
	action := memberReqStrictest(c.Settings, broken)
	keepUntil := memberReqLiveMarker(stale, member, action)
	hold, won := memberReqTakeHold(ctx, c, stale, broken, action, keepUntil)
	if !won {
		if hold == nil {
			return memberReqResult{}
		}
		return memberReqResult{Outcome: memberReqHeld}
	}
	return memberReqPunish(ctx, b, c, member.User, hold, keepUntil != 0)
}

// memberReqLiveMarker returns the marker of a stale pending mute hold when the member's live
// record still carries it and the new action is a mute too, else zero. A pending mute may have
// taken effect with its answer lost, so the reclaim checks the marker before muting again and
// adopts it (the hold keeps the marker that is live).
func memberReqLiveMarker(stale *models.ChatRequirementHold, live gotgbot.MergedChatMember, action int) int64 {
	if stale == nil || stale.Action != models.RequirementActionMute || action != models.RequirementActionMute {
		return 0
	}
	if isMemberReqMute(live, stale.MuteUntil) {
		return stale.MuteUntil
	}
	return 0
}

// memberReqUntilMatches reports whether a live restriction end date is the stored marker,
// within lockdownBanUntilTolerance seconds. A zero on either side never matches.
func memberReqUntilMatches(live, stored int64) bool {
	if live == 0 || stored == 0 {
		return false
	}
	diff := live - stored
	if diff < 0 {
		diff = -diff
	}
	return diff <= lockdownBanUntilTolerance
}

// isMemberReqMute reports whether a live chat member is under the requirement mute: still in
// the group, restricted, with an end date within two seconds of the hold's mute_until. It is
// the only proof that the requirements placed a mute (D-08): every other mute path writes an
// end date of 0 or a date the person chose, so an admin's, a staff, the captcha's, the
// antiflood's or the warn limit's mute is never taken for this one, and "I fixed it" never
// lifts it. With no marker stored nothing is ours.
func isMemberReqMute(m gotgbot.MergedChatMember, muteUntil int64) bool {
	return m.Status == gotgbot.ChatMemberStatusRestricted && m.IsMember && memberReqUntilMatches(m.UntilDate, muteUntil)
}
