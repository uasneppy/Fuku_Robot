package modules

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/uasneppy/fuku_robot/fuku/utils/chatprompt"
)

// A group's conversation memory, all in Redis (fuku:chat:*), outside the fuku:cache: prefix so
// CLEAR_CACHE_ON_STARTUP keeps it, and deleted when the owner turns conversations off
// (applyChatEnabled). It holds only what was addressed to the bot and the bot's own replies: the
// transcript keeps the newest 12 lines for 2 hours, the set of the feature's message IDs the
// newest 200 for 24 hours. It holds no user ID or username.

const (
	// chatRedisTimeout bounds one Redis round trip of the conversation feature.
	chatRedisTimeout = 500 * time.Millisecond
	// chatHistoryMax is how many lines the transcript keeps.
	chatHistoryMax = 12
	// chatHistoryTTL is how long the transcript lives after its last line.
	chatHistoryTTL = 2 * time.Hour
	// chatHistoryTextRunes is how much of a line's text the transcript keeps.
	chatHistoryTextRunes = 600
	// chatMineMax is how many of the feature's message IDs a chat keeps.
	chatMineMax = 200
	// chatMineTTL is how long the ID set lives after the last reply.
	chatMineTTL = 24 * time.Hour
)

// chatHistoryEntry is one line of the transcript, stored as compact JSON.
type chatHistoryEntry struct {
	// Role is "u" for a member's message and "b" for the bot's reply.
	Role string `json:"r"`
	// Name is the member's first name (empty for the bot).
	Name string `json:"n,omitempty"`
	Text string `json:"t"`
	// MessageID is the Telegram message ID of the line.
	MessageID int64 `json:"m"`
	At        int64 `json:"at"`
}

// chatReadHistory returns the chat's recent conversation, oldest first. A Redis error or an entry
// that does not parse is left out: the reply then has less context, never none.
func chatReadHistory(ctx context.Context, client *redis.Client, chatID int64) []chatprompt.Turn {
	ctx, cancel := context.WithTimeout(ctx, chatRedisTimeout)
	defer cancel()
	raw, err := client.LRange(ctx, chatHistoryKey(chatID), 0, chatHistoryMax-1).Result()
	if err != nil {
		return nil
	}
	turns := make([]chatprompt.Turn, 0, len(raw))
	// The list is newest first; the prompt wants oldest first.
	for i := len(raw) - 1; i >= 0; i-- {
		var entry chatHistoryEntry
		if json.Unmarshal([]byte(raw[i]), &entry) != nil {
			continue
		}
		turns = append(turns, chatprompt.Turn{Name: entry.Name, Text: entry.Text, Bot: entry.Role == "b"})
	}
	return turns
}

// chatRecordExchange stores one answered exchange: the member's line and the bot's reply in the
// transcript, and the reply's message ID in the set of the feature's own messages, in one
// transaction. It is best effort: a failure costs the next reply some context.
func chatRecordExchange(client *redis.Client, chatID int64, member chatHistoryEntry, reply chatHistoryEntry) error {
	ctx, cancel := context.WithTimeout(context.Background(), chatRedisTimeout)
	defer cancel()
	member.Role, reply.Role = "u", "b"
	member.Text = chatprompt.Clean(member.Text, chatHistoryTextRunes)
	reply.Text = chatprompt.Clean(reply.Text, chatHistoryTextRunes)
	reply.Name = ""
	memberJSON, err := json.Marshal(member)
	if err != nil {
		return err
	}
	replyJSON, err := json.Marshal(reply)
	if err != nil {
		return err
	}
	historyKey, mineKey := chatHistoryKey(chatID), chatMineKey(chatID)
	_, err = client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.LPush(ctx, historyKey, memberJSON, replyJSON)
		pipe.LTrim(ctx, historyKey, 0, chatHistoryMax-1)
		pipe.Expire(ctx, historyKey, chatHistoryTTL)
		chatRememberOwnPipe(ctx, pipe, mineKey, reply.MessageID)
		return nil
	})
	return err
}

// chatRememberOwn records a message the feature sent that is not an answered exchange (the canned
// refusal), so a reply to it is still an address.
func chatRememberOwn(client *redis.Client, chatID, messageID int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), chatRedisTimeout)
	defer cancel()
	key := chatMineKey(chatID)
	_, err := client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		chatRememberOwnPipe(ctx, pipe, key, messageID)
		return nil
	})
	return err
}

// chatRememberOwnPipe adds a message ID to the sorted set of the feature's messages, scored by the
// ID (Telegram's IDs grow within a chat), keeps the newest chatMineMax and renews the expiry.
func chatRememberOwnPipe(ctx context.Context, pipe redis.Pipeliner, key string, messageID int64) {
	pipe.ZAdd(ctx, key, redis.Z{Score: float64(messageID), Member: strconv.FormatInt(messageID, 10)})
	pipe.ZRemRangeByRank(ctx, key, 0, -chatMineMax-1)
	pipe.Expire(ctx, key, chatMineTTL)
}
