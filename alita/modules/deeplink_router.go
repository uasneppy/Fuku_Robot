package modules

import (
	"strings"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/divkix/Alita_Robot/alita/db/lang"
	"github.com/divkix/Alita_Robot/alita/i18n"
	"github.com/divkix/Alita_Robot/alita/utils/formatting"
	log "github.com/sirupsen/logrus"
)

// DeepLinkHandler processes a deep link argument.
type DeepLinkHandler func(b *gotgbot.Bot, ctx *ext.Context, user *gotgbot.User, arg string) error

var deepLinkRegistry = make(map[string]DeepLinkHandler)
var exactDeepLinkRegistry = make(map[string]DeepLinkHandler)

// RegisterDeepLinkHandler registers a handler for a deep link prefix.
func RegisterDeepLinkHandler(prefix string, handler DeepLinkHandler) {
	deepLinkRegistry[prefix] = handler
}

// RegisterExactDeepLinkHandler registers a handler for an exact deep link match.
func RegisterExactDeepLinkHandler(arg string, handler DeepLinkHandler) {
	exactDeepLinkRegistry[arg] = handler
}

// HandleDeepLink routes a deep link argument to the appropriate handler.
func HandleDeepLink(b *gotgbot.Bot, ctx *ext.Context, user *gotgbot.User, arg string) error {
	if handler, ok := exactDeepLinkRegistry[arg]; ok {
		return handler(b, ctx, user, arg)
	}

	var matchedPrefix string
	var handler DeepLinkHandler
	for prefix, h := range deepLinkRegistry {
		if strings.HasPrefix(arg, prefix) && len(prefix) > len(matchedPrefix) {
			matchedPrefix = prefix
			handler = h
		}
	}

	if handler != nil {
		return handler(b, ctx, user, arg)
	}

	return sendDefaultHelp(b, ctx, user)
}

// GroupDeepLinkHandler processes a /start payload that arrives in a group chat,
// for example the "Add group" picker's /start@bot stf_<id>. It reports handled =
// false when the payload is not one it recognises, so /start falls back to its
// normal reply.
//
// A group payload is attacker-controlled text that anyone in any group can send:
// it never grants authority. A handler must treat it as a hint about what was
// asked for and decide with live checks.
type GroupDeepLinkHandler func(b *gotgbot.Bot, ctx *ext.Context, user *gotgbot.User, arg string) (handled bool, err error)

var groupDeepLinkRegistry = make(map[string]GroupDeepLinkHandler)

// RegisterGroupDeepLinkHandler registers a handler for a group deep link prefix.
func RegisterGroupDeepLinkHandler(prefix string, handler GroupDeepLinkHandler) {
	groupDeepLinkRegistry[prefix] = handler
}

// HandleGroupDeepLink routes a group /start payload to the handler with the
// longest matching prefix, like HandleDeepLink. It returns (false, nil) when no
// prefix matches, and passes through a handler's own handled flag, so the caller
// can keep its existing behaviour for anything unhandled.
func HandleGroupDeepLink(b *gotgbot.Bot, ctx *ext.Context, user *gotgbot.User, arg string) (bool, error) {
	var matchedPrefix string
	var handler GroupDeepLinkHandler
	for prefix, h := range groupDeepLinkRegistry {
		if strings.HasPrefix(arg, prefix) && len(prefix) > len(matchedPrefix) {
			matchedPrefix = prefix
			handler = h
		}
	}
	if handler == nil {
		return false, nil
	}
	return handler(b, ctx, user, arg)
}

func sendDefaultHelp(b *gotgbot.Bot, ctx *ext.Context, user *gotgbot.User) error {
	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
	startHelpText := getStartHelp(tr)
	startMarkupKb := getStartMarkup(tr, b.Username)
	_, err := b.SendMessage(ctx.EffectiveChat.Id,
		startHelpText,
		&gotgbot.SendMessageOpts{
			ParseMode: formatting.HTML,
			LinkPreviewOptions: &gotgbot.LinkPreviewOptions{
				IsDisabled: true,
			},
			ReplyMarkup: &startMarkupKb,
		},
	)
	if err != nil {
		log.Error(err)
		return err
	}
	return ext.EndGroups
}
