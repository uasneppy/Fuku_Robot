package modules

import (
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/db/lang"
	"github.com/divkix/Alita_Robot/alita/i18n"
	"github.com/divkix/Alita_Robot/alita/utils/error_handling"
	"github.com/divkix/Alita_Robot/alita/utils/formatting"
)

// staffSelfDeleteAfter is how long a refusal posted in the issuing group stays
// visible before the bot deletes it. A restart loses the timer, which leaves a
// short refusal visible; that is accepted.
var staffSelfDeleteAfter = 30 * time.Second

// staffChatTranslator returns the translator for a chat's own language. Notices
// that are posted into a Staff Group, possibly long after the command that caused
// them, use the Staff Group's language rather than the issuer's or English.
func staffChatTranslator(chatID int64) *i18n.Translator {
	return i18n.MustNewTranslator(lang.GetLanguage(&ext.Context{EffectiveChat: &gotgbot.Chat{Id: chatID}}))
}

// replySelfDeleting replies to msg in HTML and deletes the reply after
// staffSelfDeleteAfter. It is how refusals reach a user who has not yet proven
// they own the Staff Group they named: the answer stays in the chat the command
// came from and does not linger there. Send failures are logged at warn.
func replySelfDeleting(b *gotgbot.Bot, msg *gotgbot.Message, text string) {
	if msg == nil || text == "" {
		return
	}
	sent, err := msg.Reply(b, text, formatting.Shtml())
	if err != nil {
		log.Warnf("[Staff] self-deleting reply in chat %d failed: %v", msg.Chat.Id, err)
		return
	}
	chatID, messageID := msg.Chat.Id, sent.MessageId
	time.AfterFunc(staffSelfDeleteAfter, func() {
		defer error_handling.RecoverFromPanic("replySelfDeleting", "Staff")
		_, _ = b.DeleteMessage(chatID, messageID, nil)
	})
}

// sendStaffNotice posts an HTML notice into a Staff Group with link previews off.
// A failure is logged at warn and returned, so a caller can fall back to another
// way of telling the issuer.
func sendStaffNotice(b *gotgbot.Bot, staffChatID int64, text string) error {
	_, err := b.SendMessage(staffChatID, text, &gotgbot.SendMessageOpts{
		ParseMode:          formatting.HTML,
		LinkPreviewOptions: &gotgbot.LinkPreviewOptions{IsDisabled: true},
	})
	if err != nil {
		log.Warnf("[Staff] notice to Staff Group %d failed: %v", staffChatID, err)
	}
	return err
}
