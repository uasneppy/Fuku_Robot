package modules

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db/lang"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/ratelimit"
)

// The send side of a conversation: the output filter, the fleet-wide pacer of the feature's own
// Telegram calls, the reply and the canned lines.

// chatPacer paces every Telegram call of the conversation feature (the typing action and the
// replies) on keys of its own, so a busy group's conversation never spends the budget of staff,
// lockdown, deletion or member-requirement calls. A variable so a test can install a fast one.
var chatPacer = ratelimit.NewTelegramPacer(ratelimit.TelegramPacerOptions{
	NextKey:    "fuku:chat:pace:next",
	BlockKey:   "fuku:chat:pace:block",
	Interval:   100 * time.Millisecond,
	MaxRetries: 2,
	MaxWait:    30 * time.Second,
})

// chatCallTimeout bounds one Telegram request of the feature.
const chatCallTimeout = 10 * time.Second

// chatPaced runs one Telegram call under the conversation pacer.
func chatPaced(ctx context.Context, call func(context.Context) error) error {
	return chatPacer.Do(ctx, func(callCtx context.Context) error {
		bounded, cancel := context.WithTimeout(callCtx, chatCallTimeout)
		defer cancel()
		return call(bounded)
	})
}

// chatReplyMaxRunes is the hard cap of a reply; the persona asks for far less.
const chatReplyMaxRunes = 1200

// chatLinkRemoved replaces every link-like token of a reply.
const chatLinkRemoved = "[link removed]"

var (
	// chatLinkPattern matches the tokens Telegram would turn into a link in plain text.
	chatLinkPattern = regexp.MustCompile(`(?i)(?:https?://|tg://|www\.|t\.me/|telegram\.me/)\S*`)
	// chatHandlePattern matches an @ that starts a username-shaped token.
	chatHandlePattern = regexp.MustCompile(`@([A-Za-z0-9_])`)
)

// chatFilterReply makes a provider's text safe to post: link-like tokens are replaced, every @
// that starts a username is followed by a zero-width space so nobody is pinged, and the text is
// cut to chatReplyMaxRunes at the last sentence end before the cap. It returns "" for a text that
// is blank after filtering, which is never sent.
func chatFilterReply(text string) string {
	text = chatLinkPattern.ReplaceAllString(text, chatLinkRemoved)
	text = chatHandlePattern.ReplaceAllString(text, "@\u200b$1")
	text = strings.TrimSpace(text)
	if utf8.RuneCountInString(text) <= chatReplyMaxRunes {
		return text
	}
	runes := []rune(text)[:chatReplyMaxRunes]
	cut := -1
	for i := len(runes) - 1; i > 0; i-- {
		if strings.ContainsRune(".!?…", runes[i]) {
			cut = i + 1
			break
		}
	}
	if cut <= 0 {
		return strings.TrimSpace(string(runes[:chatReplyMaxRunes-1])) + "…"
	}
	return strings.TrimSpace(string(runes[:cut]))
}

// chatSendReply posts text as a plain reply to the addressed message: no parse mode, link
// previews off. It is never sent without its reply: a message that aispam, antiflood, a lock or the
// blacklist deleted (they delete and pass the update on, so it still reaches the watcher), or that
// its sender deleted, makes Telegram refuse the reply, and the bot then stays silent
// (chatReplyTargetGone).
func chatSendReply(ctx context.Context, b *gotgbot.Bot, job chatJob, text string) (*gotgbot.Message, error) {
	var sent *gotgbot.Message
	err := chatPaced(ctx, func(callCtx context.Context) error {
		msg, err := b.SendMessageWithContext(callCtx, job.chatID, text, &gotgbot.SendMessageOpts{
			MessageThreadId:    job.threadID,
			LinkPreviewOptions: &gotgbot.LinkPreviewOptions{IsDisabled: true},
			ReplyParameters: &gotgbot.ReplyParameters{
				MessageId:                job.messageID,
				AllowSendingWithoutReply: false,
			},
		})
		sent = msg
		return err
	})
	return sent, err
}

// chatSendTyping shows "typing" in the chat once, best effort.
func chatSendTyping(ctx context.Context, b *gotgbot.Bot, job chatJob) {
	_ = chatPaced(ctx, func(callCtx context.Context) error {
		_, err := b.SendChatActionWithContext(callCtx, job.chatID, "typing", &gotgbot.SendChatActionOpts{
			MessageThreadId: job.threadID,
		})
		return err
	})
}

// The canned lines and their gates.
const (
	chatSaidMalfunction = "malfunction"
	chatSaidAllowance   = "allowance"
	// chatMalfunctionGate is how often a chat may see the malfunction line.
	chatMalfunctionGate = 5 * time.Minute
	// chatAllowanceGate is how often a chat may see the allowance line.
	chatAllowanceGate = time.Hour
)

// chatSaidKey gates one canned line in one chat.
func chatSaidKey(chatID int64, kind string) string {
	return chatKeyPrefix + "{" + strconv.FormatInt(chatID, 10) + "}:said:" + kind
}

// chatCannedText is a canned line in the group's language.
func chatCannedText(chatID int64, key string) string {
	tr := i18n.MustNewTranslator(lang.GetGroupLanguage(context.Background(), chatID))
	text, err := tr.GetString(key)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(text)
}

// chatSayGated posts a canned line as a reply when its gate is free: at most once per chat per
// window, across replicas. A Redis error or a lost gate posts nothing.
func chatSayGated(ctx context.Context, b *gotgbot.Bot, client *redis.Client, job chatJob, kind, key string, window time.Duration) {
	gateCtx, cancel := context.WithTimeout(ctx, chatRedisTimeout)
	won, err := client.SetNX(gateCtx, chatSaidKey(job.chatID, kind), "1", window).Result()
	cancel()
	if err != nil || !won {
		return
	}
	text := chatCannedText(job.chatID, key)
	if text == "" {
		return
	}
	if _, err := chatSendReply(ctx, b, job, text); err != nil {
		log.WithFields(log.Fields{"chat_id": job.chatID, "user_id": job.userID}).Debugf("[Chat] the %s line could not be sent: %s", kind, chatSendErrClass(err))
	}
}

// chatSendErrClass names a Telegram send error by its code only.
func chatSendErrClass(err error) string {
	var tgErr *gotgbot.TelegramError
	if errors.As(err, &tgErr) {
		return "Telegram " + strconv.Itoa(tgErr.Code)
	}
	if errors.Is(err, ratelimit.ErrRateLimited) {
		return "rate limited"
	}
	return "transport error"
}

// chatReplyTargetGone reports whether Telegram refused a send because the message it replies to is
// gone: a 400 whose description names the replied message. It is read by code first.
func chatReplyTargetGone(err error) bool {
	var tgErr *gotgbot.TelegramError
	if !errors.As(err, &tgErr) || tgErr.Code != 400 {
		return false
	}
	description := strings.ToLower(tgErr.Description)
	return strings.Contains(description, "replied") || strings.Contains(description, "reply message not found")
}
