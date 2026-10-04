package modules

import (
	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/message"
	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/db/staff"
	"github.com/divkix/Alita_Robot/alita/utils/error_handling"
)

// staffWatchersModule holds the silent Staff Group watchers. It is not a help
// module, so it never calls SetModuleEnabled. Its handler group is -3, which
// runs before the admin cache (-2) and the users tracker (-1); every watcher
// here must return ext.ContinueGroups so those groups still run.
var staffWatchersModule = moduleStruct{
	moduleName:   "StaffWatchers",
	handlerGroup: -3,
}

// onMigrateMessage re-keys a Staff Group and its links when Telegram announces
// that a chat changed its ID. Telegram sends migrate_to_chat_id in the old chat
// and migrate_from_chat_id in the new one, in an unspecified order, so both are
// handled by the idempotent staff.RekeyChat. The watcher never replies and
// always returns ext.ContinueGroups.
func (moduleStruct) onMigrateMessage(_ *gotgbot.Bot, ctx *ext.Context) error {
	defer error_handling.RecoverFromPanic("onMigrateMessage", "StaffWatchers")

	msg := ctx.EffectiveMessage
	if msg == nil {
		return ext.ContinueGroups
	}

	if msg.MigrateToChatId != 0 {
		if _, err := staff.RekeyChat(msg.Chat.Id, msg.MigrateToChatId); err != nil {
			log.Errorf("[StaffWatchers] onMigrateMessage: rekey %d -> %d: %v",
				msg.Chat.Id, msg.MigrateToChatId, err)
		}
	}
	if msg.MigrateFromChatId != 0 {
		if _, err := staff.RekeyChat(msg.MigrateFromChatId, msg.Chat.Id); err != nil {
			log.Errorf("[StaffWatchers] onMigrateMessage: rekey %d -> %d: %v",
				msg.MigrateFromChatId, msg.Chat.Id, err)
		}
	}
	return ext.ContinueGroups
}

// rekeyFromTelegramError is a RED-phase stub; the real detector follows.
func rekeyFromTelegramError(oldChatID int64, err error) (newChatID int64, rekeyed bool) {
	return 0, false
}

// LoadStaffWatchers registers the Staff Group watchers at handler group -3.
func LoadStaffWatchers(dispatcher *ext.Dispatcher) {
	dispatcher.AddHandlerToGroup(
		handlers.NewMessage(message.Migrate, staffWatchersModule.onMigrateMessage).SetAllowBot(true),
		staffWatchersModule.handlerGroup,
	)
}

func init() {
	RegisterLegacyModule("StaffWatchers", 237, LoadStaffWatchers)
}
