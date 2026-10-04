package modules

import (
	"errors"

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

// recheckChatOwnership runs the live ownership recheck for chatID when the cached
// gates say it is a linked group. The cached gate only filters out unrelated
// chats cheaply; the link is reloaded fresh before it is judged, and the decision
// itself always comes from recheckLink's live Telegram check.
func recheckChatOwnership(b *gotgbot.Bot, chatID int64) {
	if staff.GetLinkOfGroup(chatID) != nil {
		link, err := staff.GetLinkOfGroupFresh(chatID)
		if err != nil || link == nil {
			return
		}
		recheckLink(b, *link, newStaffOwnerPass())
	}
}

// onOwnershipMessage reacts to the chat_owner_changed and chat_owner_left service
// messages. They need no bot admin rights, but their payload is only a hint that
// something changed: the recheck always asks Telegram live who the creator is.
// The watcher never replies to the chat and always returns ext.ContinueGroups.
func (moduleStruct) onOwnershipMessage(b *gotgbot.Bot, ctx *ext.Context) error {
	defer error_handling.RecoverFromPanic("onOwnershipMessage", "StaffWatchers")

	msg := ctx.EffectiveMessage
	if msg == nil {
		return ext.ContinueGroups
	}
	recheckChatOwnership(b, msg.Chat.Id)
	return ext.ContinueGroups
}

// rekeyFromTelegramError re-keys oldChatID when err is a Telegram error that
// carries ResponseParameters.MigrateToChatId, which Telegram returns (with a
// 400) when a request targets a group that has since become a supergroup. It
// returns the new chat ID and true once staff.RekeyChat has run; for any other
// error, including nil, it changes nothing and returns (0, false). A RekeyChat
// failure is logged and still reports the new ID, so the caller can retry its
// Telegram call against it.
//
// Plans 01-09 (panel) and 01-10 (sweeper) call it after a Telegram call on a
// Staff Group chat fails.
func rekeyFromTelegramError(oldChatID int64, err error) (newChatID int64, rekeyed bool) {
	var tgErr *gotgbot.TelegramError
	if !errors.As(err, &tgErr) || tgErr.ResponseParams == nil || tgErr.ResponseParams.MigrateToChatId == 0 {
		return 0, false
	}
	newChatID = tgErr.ResponseParams.MigrateToChatId
	if _, rekeyErr := staff.RekeyChat(oldChatID, newChatID); rekeyErr != nil {
		log.Errorf("[StaffWatchers] rekeyFromTelegramError: rekey %d -> %d: %v", oldChatID, newChatID, rekeyErr)
	}
	return newChatID, true
}

// LoadStaffWatchers registers the Staff Group watchers at handler group -3.
func LoadStaffWatchers(dispatcher *ext.Dispatcher) {
	dispatcher.AddHandlerToGroup(
		handlers.NewMessage(message.Migrate, staffWatchersModule.onMigrateMessage).SetAllowBot(true),
		staffWatchersModule.handlerGroup,
	)
	dispatcher.AddHandlerToGroup(
		handlers.NewMessage(func(m *gotgbot.Message) bool {
			return message.ChatOwnerChanged(m) || message.ChatOwnerLeft(m)
		}, staffWatchersModule.onOwnershipMessage).SetAllowBot(true),
		staffWatchersModule.handlerGroup,
	)
}

func init() {
	RegisterLegacyModule("StaffWatchers", 237, LoadStaffWatchers)
}
