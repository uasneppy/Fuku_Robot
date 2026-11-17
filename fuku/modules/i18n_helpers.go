package modules

import (
	log "github.com/sirupsen/logrus"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/uasneppy/fuku_robot/fuku/db/lang"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/chat_status"
	"github.com/uasneppy/fuku_robot/fuku/utils/formatting"
)

// trS returns the translation for key and discards the error: GetString falls back to
// English for a key the language lacks, and returns "" when English lacks it too (or the
// translator has no manager), so a typo'd key gives an empty label, not an error. It
// replaces the repeated `func() string { t, _ := tr.GetString(key); return t }()` inline
// closures used to set single button labels; use it only for keys that exist in en.yml.
func trS(tr *i18n.Translator, key string) string {
	s, _ := tr.GetString(key)
	return s
}

func ctxTr(ctx *ext.Context) *i18n.Translator {
	return i18n.MustNewTranslator(lang.GetLanguage(ctx))
}

func replyHTML(b *gotgbot.Bot, msg *gotgbot.Message, text string) {
	if msg == nil || text == "" {
		return
	}
	if _, err := msg.Reply(b, text, formatting.Shtml()); err != nil {
		log.Error(err)
	}
}

func requireUser(b *gotgbot.Bot, ctx *ext.Context) *gotgbot.User {
	u := chat_status.RequireUser(b, ctx)
	if u == nil {
		chat_status.NewPermissionResponder(b).Respond(ctx, "common_cannot_identify_user", "", chat_status.WithReply())
	}
	return u
}
