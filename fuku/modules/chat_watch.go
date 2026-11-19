package modules

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db/approvals"
	"github.com/uasneppy/fuku_robot/fuku/db/chatconv"
	"github.com/uasneppy/fuku_robot/fuku/utils/chat_status"
	"github.com/uasneppy/fuku_robot/fuku/utils/chatprompt"
	"github.com/uasneppy/fuku_robot/fuku/utils/error_handling"
	"github.com/uasneppy/fuku_robot/fuku/utils/extraction"
)

// The conversation watcher (Phase 20). It answers only a message addressed to the bot (a mention
// of the bot, or a reply to a message the conversation feature itself sent) by an ordinary member
// of a group whose owner switched conversations on. It never calls a provider and never sends: it
// hands one job to the conversation pool (chat_run.go) and returns ext.ContinueGroups on every
// path, so it ends no update.

// chatWatchGroup is the handler group of the conversation watcher: after aispam (3),
// antiflood (4), locks (5, 6) and blacklists (7), so a message those delete is never
// answered, and after the late raid flood counter (12). The watcher handles no command, ends
// no update and ignores slash texts, so group 12 still sees every slash text no command took.
const chatWatchGroup = 13

// chatWatchTimeout bounds the watcher's reads (the reply check, the settings, the audience).
const chatWatchTimeout = 3 * time.Second

// chatSettingsLookup reads a chat's conversation setting through the cache. A variable only so a
// test can make it fail.
var chatSettingsLookup = chatconv.Get

// chatAudienceAdmin reports whether the sender is an administrator of the chat. A failed lookup is
// "no", which is the safe direction for this feature: unknown never answers. A variable only so a
// test can swap it.
var chatAudienceAdmin = func(b *gotgbot.Bot, chatID, userID int64) bool {
	return chat_status.IsUserAdmin(b, chatID, userID)
}

// chatAudienceApproved reads the approvals; an error is returned for the caller to read as "no".
var chatAudienceApproved = approvals.IsUserApprovedContext

// chatWatcher is the handler of group 13.
func (moduleStruct) chatWatcher(b *gotgbot.Bot, ctx *ext.Context) (result error) {
	result = ext.ContinueGroups
	defer error_handling.RecoverFromPanic("chatWatcher", "Chat")
	chatWatch(b, ctx)
	return result
}

// chatWatch runs the watcher's checks, cheapest first, with no I/O before the message is known to
// be addressed to the bot.
func chatWatch(b *gotgbot.Bot, ctx *ext.Context) {
	msg := ctx.EffectiveMessage
	chat := ctx.EffectiveChat
	if msg == nil || chat == nil || (chat.Type != "group" && chat.Type != "supergroup") {
		return
	}
	if raidFloodIsSlashText(msg) {
		return
	}
	userID, ok := raidFloodSenderCounts(b, ctx)
	if !ok || ctx.EffectiveSender.User.IsBot {
		return
	}
	if addressed, _ := chatAddressed(b, msg); !addressed {
		return
	}
	gate, provider := chatAccept(b, chat.Id, userID)
	if gate != chatGateOpen {
		return
	}
	chatEnqueue(chatJobOf(b, msg, userID, provider))
}

// chatGate is what the group's settings and the server say about one addressed message.
type chatGate int

const (
	// chatGateOpen means the message gets a job.
	chatGateOpen chatGate = iota
	// chatGateOff means conversations are off in the group or cannot run on this server: /ask
	// answers chat_off_hint, a mention or a reply gets nothing.
	chatGateOff
	// chatGateSilent means no reply and no line: an unreadable setting, no Redis or an audience
	// the sender is not in.
	chatGateSilent
)

// chatAccept applies the checks after the address: the server can converse, the cached setting
// (an error answers nothing), the switch and its provider, Redis, and the audience.
func chatAccept(b *gotgbot.Bot, chatID, userID int64) (chatGate, string) {
	if !chatAnyProviderConfigured() {
		return chatGateOff, ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), chatWatchTimeout)
	defer cancel()
	settings, err := chatSettingsLookup(ctx, chatID)
	if err != nil {
		log.WithField("chat_id", chatID).Debug("[Chat] the conversation setting could not be read, no reply")
		return chatGateSilent, ""
	}
	if !settings.Enabled || !chatProviderConfigured(settings.Provider) {
		return chatGateOff, ""
	}
	if chatRedisClient() == nil {
		return chatGateSilent, ""
	}
	if !chatAudienceAllows(ctx, b, settings.Audience, chatID, userID) {
		return chatGateSilent, ""
	}
	return chatGateOpen, settings.Provider
}

// chatAddressed reports whether the message is addressed to the bot: a mention (pure), else a
// reply to a message the conversation feature sent (one Redis read, only for a reply to the bot).
// It returns the Redis client when it had to read it.
func chatAddressed(b *gotgbot.Bot, msg *gotgbot.Message) (bool, *redis.Client) {
	if chatMentionsBot(b, msg) {
		return true, nil
	}
	reply := extraction.ReplyTarget(msg)
	if reply == nil || reply.From == nil || reply.From.Id != b.Id {
		return false, nil
	}
	client := chatRedisClient()
	if client == nil || !chatIsOwnMessage(client, msg.Chat.Id, reply.MessageId) {
		return false, nil
	}
	return true, client
}

// chatMentionsBot reports whether the message mentions the bot: an @username mention entity of
// the bot (compared without regard to case) or a text_mention of the bot, in the text or the
// caption. It is pure.
func chatMentionsBot(b *gotgbot.Bot, msg *gotgbot.Message) bool {
	username := strings.ToLower(b.Username)
	check := func(text string, entities []gotgbot.MessageEntity) bool {
		for _, entity := range entities {
			switch entity.Type {
			case "text_mention":
				if entity.User != nil && entity.User.Id == b.Id {
					return true
				}
			case "mention":
				if username == "" {
					continue
				}
				handle := strings.TrimPrefix(extractEntityText(text, entity.Offset, entity.Length), "@")
				if strings.ToLower(handle) == username {
					return true
				}
			}
		}
		return false
	}
	return check(msg.Text, msg.Entities) || check(msg.Caption, msg.CaptionEntities)
}

// chatAudienceAllows applies the group's audience: everyone, or administrators and approved
// members. A failed admin or approvals read means no reply.
func chatAudienceAllows(ctx context.Context, b *gotgbot.Bot, audience int, chatID, userID int64) bool {
	if audience != chatconv.AudienceAdminsApproved {
		return true
	}
	if chatAudienceAdmin(b, chatID, userID) {
		return true
	}
	approved, err := chatAudienceApproved(ctx, chatID, userID)
	if err != nil {
		log.WithFields(log.Fields{"chat_id": chatID, "user_id": userID}).Debug("[Chat] the approvals could not be read, no reply")
		return false
	}
	return approved
}

// chatMessageText is the text or the caption of a message.
func chatMessageText(msg *gotgbot.Message) string {
	if msg.Text != "" {
		return msg.Text
	}
	return msg.Caption
}

// chatJobOf builds the job of an addressed message: the speaker's first name, the text with the
// bot's mention removed, and the message it replies to when that is not the bot's.
func chatJobOf(b *gotgbot.Bot, msg *gotgbot.Message, userID int64, provider string) chatJob {
	job := chatJob{
		bot:       b,
		chatID:    msg.Chat.Id,
		chatTitle: msg.Chat.Title,
		userID:    userID,
		messageID: msg.MessageId,
		provider:  provider,
		turn: chatprompt.Turn{
			Name: msg.From.FirstName,
			Text: strings.TrimSpace(chatprompt.StripMention(chatMessageText(msg), b.Username)),
		},
	}
	if msg.IsTopicMessage {
		job.threadID = msg.MessageThreadId
	}
	if reply := extraction.ReplyTarget(msg); reply != nil && (reply.From == nil || reply.From.Id != b.Id) {
		quoted := chatprompt.Turn{Text: chatMessageText(reply)}
		if reply.From != nil {
			quoted.Name = reply.From.FirstName
		}
		if strings.TrimSpace(quoted.Text) != "" {
			job.quoted = &quoted
		}
	}
	return job
}

// chatIsOwnMessage reports whether messageID is one the conversation feature sent in the chat. A
// Redis error is "no".
func chatIsOwnMessage(client *redis.Client, chatID, messageID int64) bool {
	ctx, cancel := context.WithTimeout(context.Background(), chatRedisTimeout)
	defer cancel()
	_, err := client.ZScore(ctx, chatMineKey(chatID), strconv.FormatInt(messageID, 10)).Result()
	return err == nil
}
