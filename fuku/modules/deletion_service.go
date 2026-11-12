package modules

import (
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
// of a group or supergroup, other than joined and left, which cleanService handles after join
// processing.
func serviceDeleteCandidate(msg *gotgbot.Message) (deletionRow, bool) {
	if msg == nil || (msg.Chat.Type != "group" && msg.Chat.Type != "supergroup") {
		return deletionRow{}, false
	}
	row, ok := deletionServiceKindOf(msg)
	if !ok || deletion.SharesCleanServiceFlag(row.Kind) {
		return deletionRow{}, false
	}
	return row, true
}

// serviceDeleteDelay reads the group's rules and returns the effective delay of the row, and
// whether a rule is on. An unreadable rule is no rule: nothing is deleted on a guess.
func serviceDeleteDelay(ctx *ext.Context, chatID int64, row deletionRow) (int, bool) {
	rules, err := deletionRulesLookup(tracing.UpdateContext(ctx), chatID)
	if err != nil {
		log.Warnf("[ServiceDeletion] could not read the deletion rules of chat %d: %v", chatID, err)
		return 0, false
	}
	return deletionEffectiveDelay(row.Kind, rules, false)
}

// serviceDeleteWatcher deletes a service message by the group's stored rule. It never ends an
// update, never replies and never returns nil: the result is a named value preset to
// ext.ContinueGroups, so a recovered panic cannot leave group -5 and kill a later handler.
func (m moduleStruct) serviceDeleteWatcher(b *gotgbot.Bot, ctx *ext.Context) (result error) {
	result = ext.ContinueGroups
	defer error_handling.RecoverFromPanic("serviceDeleteWatcher", "ServiceDeletion")

	msg := ctx.EffectiveMessage
	row, ok := serviceDeleteCandidate(msg)
	if !ok {
		return result
	}
	delay, on := serviceDeleteDelay(ctx, msg.Chat.Id, row)
	if !on {
		return result
	}
	deletionAct(b, deletionAction{
		ChatID:    msg.Chat.Id,
		MessageID: msg.MessageId,
		Kind:      row.Kind,
		SentAt:    deletionSentAt(msg.Date),
		Delay:     delay,
	})
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
