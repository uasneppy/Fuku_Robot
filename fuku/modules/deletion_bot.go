package modules

import (
	"context"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
)

// The bot's own messages are scheduled for deletion by explicit calls at named send sites only
// (D-12): the welcome, the goodbye, the group rules message and the replies of the punishment
// commands. Nothing hooks helpers.SendMessageWithErrorHandling, media.Send or a bot client, so a
// captcha challenge, a staff, lockdown or log post, a preview and a private message are never
// reached by accident (D-18, DEL-07).

// deletionBotTimeout bounds the rules read and the queue write of one scheduled bot message. The
// call is synchronous (a few milliseconds) and independent of the update's context, which a
// shutdown may cancel.
const deletionBotTimeout = 3 * time.Second

// scheduleBotRow is the shared body of every send site's hook. It reads the chat's cached rules
// (a read error schedules nothing), returns when the kind has no rule, takes the delay from the
// rule unless delayOverride is given, and queues the message at base plus the delay through
// deletionSchedule, so the latch and the Staff Group refusal apply. A stored 0 is a row due now.
// It never returns an error and never panics to its caller: a failure is one Warn naming the
// chat and the kind, never a message ID or any text.
func scheduleBotRow(chatID, messageID int64, base time.Time, kind string, delayOverride *int) {
	ctx, cancel := context.WithTimeout(context.Background(), deletionBotTimeout)
	defer cancel()

	delay := 0
	if delayOverride != nil {
		delay = *delayOverride
	} else {
		rules, err := deletionRulesLookup(ctx, chatID)
		if err != nil {
			log.Warnf("[Deletion] could not read the rules of chat %d to schedule a %s message: %v", chatID, kind, err)
			return
		}
		if rules == nil {
			return
		}
		stored, ok := rules.Delays[kind]
		if !ok {
			return
		}
		delay = stored
	}

	_, err := deletionSchedule(ctx, deletion.Entry{
		ChatID: chatID, MessageID: messageID, Kind: kind, SentAt: base, DelaySecs: delay,
	})
	if err != nil {
		log.Warnf("[Deletion] could not schedule a %s message in chat %d: %v", kind, chatID, err)
	}
}

// scheduleBotMessage queues a message the bot just sent in a group or supergroup for deletion
// after the delay stored for its kind. It returns at once for a nil message and for any chat
// that is not a group or a supergroup, so a private chat or a channel is never scheduled. The
// delay counts from the message's own date; a zero, negative or future date means now.
func scheduleBotMessage(sent *gotgbot.Message, kind string) {
	if sent == nil || (sent.Chat.Type != "group" && sent.Chat.Type != "supergroup") {
		return
	}
	scheduleBotRow(sent.Chat.Id, sent.MessageId, deletionSentAt(sent.Date), kind, nil)
}
