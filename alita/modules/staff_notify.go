package modules

import (
	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/db/lang"
	"github.com/divkix/Alita_Robot/alita/i18n"
	"github.com/divkix/Alita_Robot/alita/utils/formatting"
)

// staffChatTranslator returns the translator for a chat's own language. Notices
// that are posted into a Staff Group, possibly long after the command that caused
// them, use the Staff Group's language rather than the issuer's or English.
func staffChatTranslator(chatID int64) *i18n.Translator {
	return i18n.MustNewTranslator(lang.GetLanguage(&ext.Context{EffectiveChat: &gotgbot.Chat{Id: chatID}}))
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
