package modules

import (
	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/utils/extraction"
)

// withRealReply returns msg with its reply_to_message cut down to a real reply (extraction.ReplyTarget): in a
// forum topic Telegram sets reply_to_message to the topic's root on every message that replies to nothing, so
// a command that reads its content from the reply would take the topic's root instead of its own text. The
// copy is shallow and msg itself is not changed.
func withRealReply(msg *gotgbot.Message) *gotgbot.Message {
	if msg == nil || msg.ReplyToMessage == nil {
		return msg
	}
	reply := extraction.ReplyTarget(msg)
	if reply == msg.ReplyToMessage {
		return msg
	}
	scoped := *msg
	scoped.ReplyToMessage = reply
	return &scoped
}
