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
	"github.com/uasneppy/fuku_robot/fuku/db/greetings"
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
	legacyJoinChat   int64 = -1009912001207
	legacyPhotoChat  int64 = -1009912001208
	legacyLeaveChat  int64 = -1009912001209
	legacyShowChat   int64 = -1009912001210
	legacyDocChat    int64 = -1009912001211
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

// legacyNoCaptionAbove fails when any recorded call of any method has the key
// show_caption_above_media.
func legacyNoCaptionAbove(t *testing.T, client *moduleBotClient) {
	t.Helper()
	client.mu.Lock()
	defer client.mu.Unlock()
	for _, call := range client.calls {
		if _, ok := call.Params["show_caption_above_media"]; ok {
			t.Fatalf("%s carries show_caption_above_media: a legacy row must not", call.Method)
		}
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

// legacyGreetingClient answers the media sends too, so a send succeeds and no error path runs.
func legacyGreetingClient() *moduleBotClient {
	client := newModuleBotClient()
	chat := `"chat":{"id":-1001,"type":"supergroup","title":"Test Chat"}`
	client.responses["sendVideo"] = json.RawMessage(
		`{"message_id":9004,"date":1,` + chat + `,"video":{"file_id":"video-1","file_unique_id":"video-u1","width":160,"height":80,"duration":1}}`)
	client.responses["sendAnimation"] = json.RawMessage(
		`{"message_id":9005,"date":1,` + chat + `,"animation":{"file_id":"anim-1","file_unique_id":"anim-u1","width":160,"height":80,"duration":1}}`)
	return client
}

func TestLegacyGreetingsRequestsUnchanged(t *testing.T) {
	withRealLocales(t)
	admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	member := gotgbot.User{Id: 4242, FirstName: "Newbie"}
	groupOf := func(id int64) gotgbot.Chat {
		return gotgbot.Chat{Id: id, Type: "supergroup", Title: "Greeting Chat"}
	}
	join := func(t *testing.T, client *moduleBotClient, chat gotgbot.Chat) {
		t.Helper()
		bot := newModuleTestBot(client)
		ctx := newChatMemberContext(bot, chat, admin,
			gotgbot.ChatMemberLeft{User: member}, gotgbot.ChatMemberMember{User: member})
		clearRecentJoinProcessing(chat.Id, member.Id)
		if err := greetingsModule.newMember(bot, ctx); err != ext.EndGroups {
			t.Fatalf("newMember error = %v, want EndGroups", err)
		}
	}

	t.Run("join_text", func(t *testing.T) {
		legacyCleanup(t, legacyJoinChat)
		mustLegacy(t, greetings.SetWelcomeText(legacyJoinChat, "Welcome {first}", "", nil, db.TEXT))
		client := legacyGreetingClient()
		join(t, client, groupOf(legacyJoinChat))
		legacyExpectOne(t, client, "sendMessage", goldenJoinText)
		legacyNoCaptionAbove(t, client)
	})
	t.Run("join_photo", func(t *testing.T) {
		legacyCleanup(t, legacyPhotoChat)
		mustLegacy(t, greetings.SetWelcomeText(legacyPhotoChat, "Hi {first}", "photo-legacy", nil, db.PHOTO))
		client := legacyGreetingClient()
		join(t, client, groupOf(legacyPhotoChat))
		legacyExpectOne(t, client, "sendPhoto", goldenJoinPhoto)
		legacyNoCaptionAbove(t, client)
	})
	t.Run("leave_video", func(t *testing.T) {
		legacyCleanup(t, legacyLeaveChat)
		mustLegacy(t, greetings.SetGoodbyeText(legacyLeaveChat, "Bye {first}", "video-legacy", nil, db.VIDEO))
		mustLegacy(t, greetings.SetGoodbyeToggle(legacyLeaveChat, true))
		client := legacyGreetingClient()
		bot := newModuleTestBot(client)
		chat := groupOf(legacyLeaveChat)
		ctx := newChatMemberContext(bot, chat, admin,
			gotgbot.ChatMemberMember{User: member}, gotgbot.ChatMemberLeft{User: member})
		if err := greetingsModule.leftMember(bot, ctx); err != ext.EndGroups {
			t.Fatalf("leftMember error = %v, want EndGroups", err)
		}
		legacyExpectOne(t, client, "sendVideo", goldenLeaveVideo)
		legacyNoCaptionAbove(t, client)
	})
	t.Run("welcome_display_photo", func(t *testing.T) {
		legacyCleanup(t, legacyShowChat)
		mustLegacy(t, greetings.SetWelcomeText(legacyShowChat, "Hi {first}", "photo-legacy", nil, db.PHOTO))
		client := legacyGreetingClient()
		bot := newModuleTestBot(client)
		ctx := newGreetingMessageContext(bot, groupOf(legacyShowChat), admin, "/welcome")
		if err := greetingsModule.welcome(bot, ctx); err != ext.EndGroups {
			t.Fatalf("welcome display error = %v, want EndGroups", err)
		}
		legacyExpectOne(t, client, "sendPhoto", goldenWelcomeShowPhoto)
		legacyNoCaptionAbove(t, client)
	})
	t.Run("join_document_gif", func(t *testing.T) {
		legacyCleanup(t, legacyDocChat)
		mustLegacy(t, greetings.SetWelcomeText(legacyDocChat, "Gif {first}", "gif-as-document", nil, db.DOCUMENT))
		client := legacyGreetingClient()
		join(t, client, groupOf(legacyDocChat))
		legacyExpectOne(t, client, "sendDocument", goldenJoinDocument)
		if calls := client.callsFor("sendAnimation"); len(calls) != 0 {
			t.Fatalf("sendAnimation calls = %d, want 0: a GIF saved as a document stays a document", len(calls))
		}
		legacyNoCaptionAbove(t, client)
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
	goldenJoinText            = `{"chat_id":-1009912001207,"link_preview_options":{"is_disabled":true},"parse_mode":"HTML","reply_markup":{},"text":"Welcome Newbie"}`
	goldenJoinPhoto           = `{"caption":"Hi Newbie","chat_id":-1009912001208,"parse_mode":"HTML","photo":"photo-legacy","reply_markup":{}}`
	goldenLeaveVideo          = `{"caption":"Bye Newbie","chat_id":-1009912001209,"parse_mode":"HTML","reply_markup":{},"thumbnail":null,"video":"video-legacy"}`
	goldenWelcomeShowPhoto    = `{"caption":"Hi Telegram","chat_id":-1009912001210,"parse_mode":"HTML","photo":"photo-legacy","reply_markup":{}}`
	goldenJoinDocument        = `{"caption":"Gif Newbie","chat_id":-1009912001211,"document":"gif-as-document","parse_mode":"HTML","reply_markup":{},"thumbnail":null}`
)
