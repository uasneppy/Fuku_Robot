//go:build testtools

package modules

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/uasneppy/fuku_robot/fuku/db"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/db/rules"
)

// Fixed chat IDs keep the JSON goldens below stable, one per subtest.
const (
	legacyRulesChat  int64 = -1009912001201
	legacyReplyChat  int64 = -1009912001202
	legacyPrivChat   int64 = -1009912001203
	legacyNoneChat   int64 = -1009912001204
	legacyLinkChat   int64 = -1009912001205
	legacyNoLinkChat int64 = -1009912001206
)

// legacyRequestJSON is the JSON encoding of the parameters the bot handed to Telegram.
// encoding/json sorts map keys, so the string is stable.
func legacyRequestJSON(t *testing.T, call moduleBotCall) string {
	t.Helper()
	raw, err := json.Marshal(call.Params)
	if err != nil {
		t.Fatalf("encode %s params: %v", call.Method, err)
	}
	return string(raw)
}

// legacyCleanup deletes the rules, greetings and chats rows of the chats and the cached
// greetings before the test and again when it ends.
func legacyCleanup(t *testing.T, chatIDs ...int64) {
	t.Helper()
	clean := func() {
		db.DB.Where("chat_id IN ?", chatIDs).Delete(&models.RulesSettings{})
		db.DB.Where("chat_id IN ?", chatIDs).Delete(&models.GreetingSettings{})
		db.DB.Where("chat_id IN ?", chatIDs).Delete(&models.Chat{})
		for _, id := range chatIDs {
			dbcache.DeleteCache(dbcache.CacheKey("greetings", id))
		}
		dbcache.ResetLocalForTest()
	}
	clean()
	t.Cleanup(clean)
}

// mustLegacy fails the test on a setup error.
func mustLegacy(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("legacy fixture setup: %v", err)
	}
}

// legacyExpectOne requires exactly one call of the method and compares its request with
// the golden recorded on the code as it was before Phase 12.
func legacyExpectOne(t *testing.T, client *moduleBotClient, method, golden string) {
	t.Helper()
	calls := client.callsFor(method)
	if len(calls) != 1 {
		t.Fatalf("%s calls = %d, want 1", method, len(calls))
	}
	if got := legacyRequestJSON(t, calls[0]); got != golden {
		t.Fatalf("%s request changed\n got %s\nwant %s", method, got, golden)
	}
}

func TestRulesLegacyRowSendsTodaysRequest(t *testing.T) {
	withRealLocales(t)
	const rulesText = `Read <b>this</b> first: https://example.com/rules`
	user := gotgbot.User{Id: 42, FirstName: "Member"}

	groupCommand := func(t *testing.T, chatID int64, replyTo int64) *moduleBotClient {
		t.Helper()
		client := newModuleBotClient()
		bot := newModuleTestBot(client)
		chat := gotgbot.Chat{Id: chatID, Type: "supergroup", Title: "Legacy & Rules"}
		ctx := newModuleMessageContext(bot, chat, user, "/rules")
		if replyTo != 0 {
			ctx.EffectiveMessage.ReplyToMessage = &gotgbot.Message{MessageId: replyTo, Date: 1, Chat: chat}
		}
		if err := rulesModule.sendRules(bot, ctx); err != ext.EndGroups {
			t.Fatalf("sendRules() error = %v, want EndGroups", err)
		}
		return client
	}
	deepLink := func(t *testing.T, chatID int64) *moduleBotClient {
		t.Helper()
		client := newModuleBotClient()
		bot := newModuleTestBot(client)
		chat := gotgbot.Chat{Id: user.Id, Type: "private", FirstName: "Member"}
		arg := "rules_" + strconv.FormatInt(chatID, 10)
		ctx := newModuleMessageContext(bot, chat, user, "/start "+arg)
		if err := rulesDeepLinkHandler(bot, ctx, &user, arg); err != ext.EndGroups {
			t.Fatalf("rulesDeepLinkHandler() error = %v, want EndGroups", err)
		}
		return client
	}

	t.Run("group_private_off", func(t *testing.T) {
		legacyCleanup(t, legacyRulesChat)
		mustLegacy(t, rules.SetChatRules(legacyRulesChat, rulesText))
		legacyExpectOne(t, groupCommand(t, legacyRulesChat, 0), "sendMessage", goldenRulesPrivateOff)
	})
	t.Run("group_reply", func(t *testing.T) {
		legacyCleanup(t, legacyReplyChat)
		mustLegacy(t, rules.SetChatRules(legacyReplyChat, rulesText))
		legacyExpectOne(t, groupCommand(t, legacyReplyChat, 55), "sendMessage", goldenRulesReply)
	})
	t.Run("group_private_on", func(t *testing.T) {
		legacyCleanup(t, legacyPrivChat)
		mustLegacy(t, rules.SetChatRules(legacyPrivChat, rulesText))
		mustLegacy(t, rules.SetPrivateRules(legacyPrivChat, true))
		mustLegacy(t, rules.SetChatRulesButton(legacyPrivChat, "Read rules"))
		legacyExpectOne(t, groupCommand(t, legacyPrivChat, 0), "sendMessage", goldenRulesPrivateOn)
	})
	t.Run("group_no_rules", func(t *testing.T) {
		legacyCleanup(t, legacyNoneChat)
		legacyExpectOne(t, groupCommand(t, legacyNoneChat, 0), "sendMessage", goldenRulesNone)
	})
	t.Run("deep_link_set", func(t *testing.T) {
		legacyCleanup(t, legacyLinkChat)
		mustLegacy(t, rules.SetChatRules(legacyLinkChat, rulesText))
		legacyExpectOne(t, deepLink(t, legacyLinkChat), "sendMessage", goldenRulesDeepLinkSet)
	})
	t.Run("deep_link_not_set", func(t *testing.T) {
		legacyCleanup(t, legacyNoLinkChat)
		legacyExpectOne(t, deepLink(t, legacyNoLinkChat), "sendMessage", goldenRulesDeepLinkNotSet)
	})
}

// Goldens of the requests rules rows saved before Phase 12 send, recorded on the code as
// it was at bd2f794 (rules.go, sender.go and greetings.go untouched) and never re-recorded.
const (
	goldenRulesPrivateOff     = `{"chat_id":-1009912001201,"parse_mode":"HTML","reply_markup":{},"reply_parameters":{"message_id":101,"allow_sending_without_reply":true},"text":"Правила для \u003cb\u003eLegacy \u0026amp; Rules\u003c/b\u003e:\n\nRead \u003cb\u003ethis\u003c/b\u003e first: https://example.com/rules"}`
	goldenRulesReply          = `{"chat_id":-1009912001202,"parse_mode":"HTML","reply_markup":{},"reply_parameters":{"message_id":55,"allow_sending_without_reply":true},"text":"Правила для \u003cb\u003eLegacy \u0026amp; Rules\u003c/b\u003e:\n\nRead \u003cb\u003ethis\u003c/b\u003e first: https://example.com/rules"}`
	goldenRulesPrivateOn      = `{"chat_id":-1009912001203,"parse_mode":"HTML","reply_markup":{"inline_keyboard":[[{"text":"Read rules","url":"t.me/FukuTestBot?start=rules_-1009912001203"}]]},"reply_parameters":{"message_id":101,"allow_sending_without_reply":true},"text":"Натисніть кнопку, щоб побачити правила чату!"}`
	goldenRulesNone           = `{"chat_id":-1009912001204,"parse_mode":"HTML","reply_markup":{},"reply_parameters":{"message_id":101,"allow_sending_without_reply":true},"text":"Схоже, у цьому чаті ще не встановлено жодних правил... Але не сприймайте це як запрошення."}`
	goldenRulesDeepLinkSet    = `{"chat_id":42,"link_preview_options":{"is_disabled":true},"parse_mode":"HTML","reply_parameters":{"message_id":101,"allow_sending_without_reply":true},"text":"Правила для \u003cb\u003eTest Chat\u003c/b\u003e:\n\nRead \u003cb\u003ethis\u003c/b\u003e first: https://example.com/rules"}`
	goldenRulesDeepLinkNotSet = `{"chat_id":42,"link_preview_options":{"is_disabled":true},"parse_mode":"HTML","reply_parameters":{"message_id":101,"allow_sending_without_reply":true},"text":"У цьому чаті немає жодних правил!"}`
)
