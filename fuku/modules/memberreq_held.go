package modules

import (
	"context"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// memberReqHeldReleased is the held release (D-21): the one re-check a held member's message
// gets before it is treated as a held one. A hold ends by a leave, a rejoin, the clean predicate
// and "I fixed it", and the clean predicate runs only in the watcher's clean path, which a group
// with a lookup rule on reaches only for a member whose lookups are cached positive. A held
// member's messages never ask Telegram, so a Warn hold with a photo or channel bit would never be
// cleared after the member fixed the photo, and the watcher would hand a job to the runner for
// every message of theirs for as long as the hold lives. This closes that loop.
//
// It reports true, and the caller then treats the message as a clean one (no Held outcome, no
// delete), only when all of these hold: the case is a message with a message ID and a date (a join
// has neither and is never released); the group's settings read again (memberReqSettingsLookup)
// without an error; the names of the sender break no rule that is on (a member who still breaks a
// name rule stays held, and no lookup is spent on them); every lookup rule the hold carries a bit
// of, and that is still on, now answers memberReqPass or memberReqInert (an unknown, a breach or a
// lookup that was cut off keeps the hold; a rule switched Off counts as released); and
// memberreq.ClearHoldOnClean, the Phase 13 predicate with its two second margin, deleted the row.
// A hold with name bits only needs no lookup, so a member who fixed their name is released here
// exactly as the watcher's clean path released them before the lookup rules existed.
//
// The lookups run with memberReqLookupMode{Classify: true} (an unanswerable channel lookup is
// inert when the bot is shown to be no administrator of the channel), under
// memberReqLookupJobBudget, and a pass is cached by the lookup itself; a missing photo or
// subscription is never cached, so a held member still lacking one costs one paced lookup per
// message (D-24, accepted).
//
// It only deletes the hold row. It never punishes, claims, warns, posts or restricts, and it
// never lifts a mute: for a punished mute the predicate deletes the row only when the mute was
// confirmed more than two seconds before the message, and a member the bot muted cannot send, so
// that message shows an administrator lifted the mute. A mute is otherwise freed only by "I fixed
// it" or by an administrator. Logs carry chat and user IDs and the outcome only.
func memberReqHeldReleased(ctx context.Context, b *gotgbot.Bot, c memberReqCase, held *models.ChatRequirementHold) bool {
	if held == nil || c.Source != memberReqFromMessage || c.MessageID <= 0 || c.MessageDate <= 0 {
		return false
	}
	settings, err := memberReqSettingsLookup(ctx, c.Chat.Id)
	if err != nil || settings == nil {
		log.Debugf("[MemberReq] the requirements of chat %d could not be read for the hold of user %d, so it is kept", c.Chat.Id, c.User.Id)
		return false
	}
	if memberReqBroken(settings, memberReqNamesOf(c.User)) != 0 {
		return false
	}
	if !memberReqHeldLookupsClear(ctx, b, c, settings, held.Rules) {
		return false
	}
	cleared, err := memberreq.ClearHoldOnClean(ctx, c.Chat.Id, c.User.Id, time.Unix(c.MessageDate, 0))
	if err != nil {
		log.Warnf("[MemberReq] releasing the hold of user %d in chat %d failed: %s", c.User.Id, c.Chat.Id, memberReqErrText(err))
		return false
	}
	if cleared {
		log.Infof("[MemberReq] the hold of user %d in chat %d was released: the member now meets what it was for", c.User.Id, c.Chat.Id)
	}
	return cleared
}

// memberReqHeldLookupsClear reports whether every lookup rule that the hold's mask carries a bit
// of, and that is still on in settings, now answers pass or inert. Only the hold's own lookup bits
// are asked (a lookup rule the member broke nothing of is never spent on a held member), in table
// order, and the first answer that is not a pass or inert ends it.
func memberReqHeldLookupsClear(ctx context.Context, b *gotgbot.Bot, c memberReqCase, settings *models.ChatRequirements, holdRules int) bool {
	lookupCtx, cancel := context.WithTimeout(ctx, memberReqLookupJobBudget)
	defer cancel()
	for _, rule := range memberReqRules {
		if rule.Lookup == nil || holdRules&rule.Bit == 0 || memberreq.ActionOf(settings, rule.Store) == models.RequirementActionOff {
			continue
		}
		switch rule.Lookup(lookupCtx, b, c.Chat.Id, c.User.Id, memberReqLookupMode{Classify: true}) {
		case memberReqPass, memberReqInert:
		default:
			return false
		}
	}
	return true
}
