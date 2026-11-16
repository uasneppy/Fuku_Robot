package modules

import (
	"fmt"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/utils/formatting"
)

// The punishment notice of the forward blocklist watcher lives in its own file, outside the staff*
// files, because it is a message of the linked group (an automatic punishment message, like the
// blacklist's and the member requirements') and so is queued for deletion, while the never-delete
// guard (TestMemberDeletionExcludedMessages) forbids any deletion producer in a staff file: a Staff
// Group card or summary must never reach one. The notice is never posted into a Staff Group.

// forwardBlockNotice posts the existing blacklist punishment notice in the group's language and
// queues it as an automatic punishment message. The mention and the reason are spliced after
// translation.
func forwardBlockNotice(b *gotgbot.Bot, ctx *ext.Context, userID int64, key string) error {
	tr := ctxTr(ctx)
	template, _ := tr.GetString(key)
	text := fmt.Sprintf(template, formatting.MentionHtml(userID, ctx.EffectiveSender.Name()), forwardBlockReason(tr))
	sent, err := b.SendMessage(ctx.EffectiveMessage.Chat.Id, text, formatting.Shtml())
	if err != nil {
		return err
	}
	scheduleBotMessage(sent, deletion.KindBotAuto)
	return nil
}
