package modules

import (
	"context"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/config"
	"github.com/uasneppy/fuku_robot/fuku/db/chatconv"
	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
)

// chatProviders is the order the providers are offered in.
var chatProviders = []string{chatconv.ProviderAnthropic, chatconv.ProviderGoogle}

// chatProviderConfigured reports whether this server can run conversations on provider: the
// ENABLE_CHAT switch is on and the provider's key is set. It reads config.AppConfig only, so it
// makes no network call.
func chatProviderConfigured(provider string) bool {
	cfg := config.AppConfig
	if cfg == nil || !cfg.EnableChat {
		return false
	}
	switch provider {
	case chatconv.ProviderAnthropic:
		return cfg.AnthropicAPIKey != ""
	case chatconv.ProviderGoogle:
		return cfg.GeminiAPIKey != ""
	}
	return false
}

// chatAnyProviderConfigured reports whether this server can run conversations at all.
func chatAnyProviderConfigured() bool {
	for _, provider := range chatProviders {
		if chatProviderConfigured(provider) {
			return true
		}
	}
	return false
}

// chatProviderName is the provider's name as the page and the data notice show it. Brand
// names are not translated.
func chatProviderName(provider string) string {
	if provider == chatconv.ProviderGoogle {
		return "Google (Gemini)"
	}
	return "Anthropic (Claude)"
}

// chatProviderShortName is the provider's name on the Main line.
func chatProviderShortName(provider string) string {
	if provider == chatconv.ProviderGoogle {
		return "Google"
	}
	return "Anthropic"
}

// chatKeyPrefix is the family of every conversation key in Redis. It sits outside the
// fuku:cache: prefix, so CLEAR_CACHE_ON_STARTUP keeps it.
const chatKeyPrefix = "fuku:chat:"

// chatHistoryKey is a chat's rolling transcript (the chat ID is a hash tag).
func chatHistoryKey(chatID int64) string {
	return chatKeyPrefix + "{" + strconv.FormatInt(chatID, 10) + "}:h"
}

// chatMineKey is the set of message IDs the conversation feature sent in a chat.
func chatMineKey(chatID int64) string {
	return chatKeyPrefix + "{" + strconv.FormatInt(chatID, 10) + "}:mine"
}

// chatNoRedisLogged makes the no-Redis line appear once per process.
var chatNoRedisLogged atomic.Bool

// chatRedisClient returns the Redis client, or nil without Redis, in which case conversations
// are inert on this replica. The first call without Redis logs one line. It is a variable only
// so a test can swap it.
var chatRedisClient = func() *redis.Client {
	if cache.IsRedisAvailable() {
		if client := cache.GetRedisClient(); client != nil {
			return client
		}
	}
	if chatNoRedisLogged.CompareAndSwap(false, true) {
		log.Info("[Chat] Redis is not available, so conversations are off on this replica")
	}
	return nil
}

// chatWipeTimeout bounds the transcript wipe after an Off write.
const chatWipeTimeout = 2 * time.Second

// chatConvWrites stores the conversation setting. Its fields are variables only so a test can
// make a write fail.
var chatConvWrites = struct {
	Enabled  func(ctx context.Context, chatID int64, enabled bool, by int64) error
	Provider func(ctx context.Context, chatID int64, provider string, by int64) error
	Audience func(ctx context.Context, chatID int64, audience int, by int64) error
}{
	Enabled:  chatconv.SetEnabled,
	Provider: chatconv.SetProvider,
	Audience: chatconv.SetAudience,
}

// chatConvRead reads the setting back after a write; a variable so a test can make it fail.
var chatConvRead = chatconv.GetFresh

// chatApplyOutcome is what an apply function did.
type chatApplyOutcome int

const (
	// chatApplied means the setting holds the requested value.
	chatApplied chatApplyOutcome = iota
	// chatInvalidValue means the value is not a stored one; nothing was written.
	chatInvalidValue
	// chatSaveFailed means the write failed; the stored value is unchanged.
	chatSaveFailed
)

// chatApplyResult is the typed result of an apply function. Settings is the row read back
// after a write, or nil when the read-back failed (the write still landed) or nothing was
// written.
type chatApplyResult struct {
	Outcome  chatApplyOutcome
	Settings *chatconv.Settings
	Err      error
}

// applyChatEnabled switches conversations on or off for a chat. It judges no person's rights:
// the settings page does that before it calls. Turning conversations off also deletes the
// chat's transcript and the set of the feature's own message IDs, so nothing members said to
// the bot outlives the switch.
func applyChatEnabled(ctx context.Context, chatID int64, enabled bool, by int64) chatApplyResult {
	if err := chatConvWrites.Enabled(ctx, chatID, enabled, by); err != nil {
		log.Warnf("[Chat] saving the on/off switch of chat %d failed: %v", chatID, err)
		return chatApplyResult{Outcome: chatSaveFailed, Err: err}
	}
	if !enabled {
		chatWipeMemory(chatID)
	}
	return chatReadBack(ctx, chatID)
}

// applyChatProvider stores the provider a chat's conversations ask.
func applyChatProvider(ctx context.Context, chatID int64, provider string, by int64) chatApplyResult {
	if !chatconv.ValidProvider(provider) {
		return chatApplyResult{Outcome: chatInvalidValue}
	}
	if err := chatConvWrites.Provider(ctx, chatID, provider, by); err != nil {
		log.Warnf("[Chat] saving the provider of chat %d failed: %v", chatID, err)
		return chatApplyResult{Outcome: chatSaveFailed, Err: err}
	}
	return chatReadBack(ctx, chatID)
}

// applyChatAudience stores who may talk to the bot in a chat.
func applyChatAudience(ctx context.Context, chatID int64, audience int, by int64) chatApplyResult {
	if !chatconv.ValidAudience(audience) {
		return chatApplyResult{Outcome: chatInvalidValue}
	}
	if err := chatConvWrites.Audience(ctx, chatID, audience, by); err != nil {
		log.Warnf("[Chat] saving the audience of chat %d failed: %v", chatID, err)
		return chatApplyResult{Outcome: chatSaveFailed, Err: err}
	}
	return chatReadBack(ctx, chatID)
}

func chatReadBack(ctx context.Context, chatID int64) chatApplyResult {
	settings, err := chatConvRead(ctx, chatID)
	if err != nil {
		log.Warnf("[Chat] reading back the setting of chat %d failed: %v", chatID, err)
		return chatApplyResult{Outcome: chatApplied}
	}
	return chatApplyResult{Outcome: chatApplied, Settings: &settings}
}

// chatWipeMemory deletes a chat's transcript and its set of the feature's message IDs. It is
// best effort: without Redis there is nothing to delete, and a failed delete leaves keys that
// expire within two hours (the transcript) and a day (the ID set).
func chatWipeMemory(chatID int64) {
	client := chatRedisClient()
	if client == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), chatWipeTimeout)
	defer cancel()
	if err := client.Del(ctx, chatHistoryKey(chatID), chatMineKey(chatID)).Err(); err != nil {
		log.Warnf("[Chat] deleting the conversation memory of chat %d failed: %v", chatID, err)
	}
}
