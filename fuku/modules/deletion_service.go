package modules

import (
	"context"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/utils/error_handling"
	"github.com/uasneppy/fuku_robot/fuku/utils/tracing"
)

// serviceDeleteGroup is the handler group of the service message deletion watcher. -5 has
// hosted no handler since the old raid mode was retired: it runs after the guards that may end
// an update (-10 captcha, -7 lockdown guard, -6 fed-ban) and before the flood counter (-4), and
// only this watcher lives there.
const serviceDeleteGroup = -5

// serviceDeleteModule holds the service message deletion watcher. It is not a help module, so
// it never calls SetModuleEnabled and /help and the docs generator see no new module.
var serviceDeleteModule = moduleStruct{
	moduleName:   "ServiceDeletion",
	handlerGroup: serviceDeleteGroup,
}

// serviceDeleteFilter is the watcher's filter: a message that is one of the deletable service
// types. Everything else never reaches the watcher body.
func serviceDeleteFilter(msg *gotgbot.Message) bool {
	_, ok := deletionServiceKindOf(msg)
	return ok
}

// serviceDeleteCandidate returns the row of a message the watcher may act on: a service message
// of a group or supergroup. Joined and left are the watcher's only when a bot caused them
// (the sender is a bot, so cleanService, which does not accept bot senders, never sees them);
// for every other sender cleanService handles them after join processing, so no message is
// handled twice. A message with no sender is not a bot's and is handled by neither path.
func serviceDeleteCandidate(ctx *ext.Context, msg *gotgbot.Message) (deletionRow, bool) {
	if msg == nil || (msg.Chat.Type != "group" && msg.Chat.Type != "supergroup") {
		return deletionRow{}, false
	}
	row, ok := deletionServiceKindOf(msg)
	if !ok {
		return deletionRow{}, false
	}
	if deletion.SharesCleanServiceFlag(row.Kind) && (ctx.EffectiveSender == nil || !ctx.EffectiveSender.IsBot()) {
		return deletionRow{}, false
	}
	return row, true
}

// deletionRuleFor reads the effective rule of a kind in a chat: whether a rule is on and its
// delay in seconds. Joined and left read the shared /cleanservice flag through
// deletionFlagLookup (D-02); every other kind is the stored row alone. A read that fails is an
// error, never a guess: the callers delete nothing then.
func deletionRuleFor(ctx context.Context, chatID int64, kind string) (delay int, on bool, err error) {
	rules, err := deletionRulesLookup(ctx, chatID)
	if err != nil {
		return 0, false, err
	}
	flagOn := false
	if deletion.SharesCleanServiceFlag(kind) {
		settings, flagErr := deletionFlagLookup(ctx, chatID)
		if flagErr != nil {
			return 0, false, flagErr
		}
		flagOn = settings != nil && settings.ShouldCleanService
	}
	delay, on = deletionEffectiveDelay(kind, rules, flagOn)
	return delay, on, nil
}

// deletionApplyMessage deletes or queues one service message by the group's effective rule for
// its kind. An unreadable rule or flag deletes nothing. It is the tail of the watcher and of
// cleanService, which call it after their own checks, and it logs the chat ID only.
func deletionApplyMessage(ctx context.Context, b *gotgbot.Bot, msg *gotgbot.Message, row deletionRow) {
	delay, on, err := deletionRuleFor(ctx, msg.Chat.Id, row.Kind)
	if err != nil {
		log.Warnf("[ServiceDeletion] could not read the deletion rule of chat %d: %v", msg.Chat.Id, err)
		return
	}
	if !on {
		return
	}
	deletionAct(b, deletionAction{
		ChatID:    msg.Chat.Id,
		MessageID: msg.MessageId,
		Kind:      row.Kind,
		SentAt:    deletionSentAt(msg.Date),
		Delay:     delay,
	})
}

// serviceDeleteWatcher deletes a service message by the group's stored rule. It never ends an
// update, never replies and never returns nil: the result is a named value preset to
// ext.ContinueGroups, so a recovered panic cannot leave group -5 and kill a later handler.
func (m moduleStruct) serviceDeleteWatcher(b *gotgbot.Bot, ctx *ext.Context) (result error) {
	result = ext.ContinueGroups
	defer error_handling.RecoverFromPanic("serviceDeleteWatcher", "ServiceDeletion")

	msg := ctx.EffectiveMessage
	if row, ok := serviceDeleteCandidate(ctx, msg); ok {
		deletionApplyMessage(tracing.UpdateContext(ctx), b, msg, row)
	}
	return result
}

// LoadServiceDeletion registers the service message deletion watcher at handler group -5. It
// accepts bot senders (a service message caused by a bot is still the group's notice).
func LoadServiceDeletion(dispatcher *ext.Dispatcher) {
	dispatcher.AddHandlerToGroup(
		handlers.NewMessage(serviceDeleteFilter, serviceDeleteModule.serviceDeleteWatcher).SetAllowBot(true),
		serviceDeleteModule.handlerGroup,
	)
}

func init() {
	RegisterLegacyModule("ServiceDeletion", 242, LoadServiceDeletion)
}
