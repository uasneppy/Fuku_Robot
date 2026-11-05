//go:build testtools

package modules

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/connections"
	"github.com/uasneppy/fuku_robot/fuku/db/lang"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/db/rules"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// Fixed chat IDs of the member-facing rules tests, one per scenario, so no test reads another's row.
const (
	rulesSendPhotoChat int64 = -1009912007101
	rulesSendVideoChat int64 = -1009912007102
	rulesSendDocChat   int64 = -1009912007103
)

// rulesChatID gives each scenario of the member-facing rules tests its own chat ID.
func rulesChatID(n int64) int64 { return -1009912008000 - n }

// rulesTestClient answers every media send the rules tests make, so a send succeeds and no error path runs.
func rulesTestClient() *moduleBotClient {
	client := legacyGreetingClient()
	generic := json.RawMessage(`{"message_id":9006,"date":1,"chat":{"id":-1001,"type":"supergroup","title":"Test Chat"}}`)
	client.responses["sendAudio"] = generic
	client.responses["sendSticker"] = generic
	return client
}

// rulesCallParams decodes the parameters of a recorded call through JSON, the form Telegram receives, so
// a keyboard passed by value and one passed by pointer read the same.
func rulesCallParams(t *testing.T, call moduleBotCall) map[string]any {
	t.Helper()
	raw, err := json.Marshal(call.Params)
	if err != nil {
		t.Fatalf("encode %s params: %v", call.Method, err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode %s params: %v", call.Method, err)
	}
	return out
}

// rulesOnlyCall requires that the client made exactly one send call, of the given method, and returns its
// decoded parameters.
func rulesOnlyCall(t *testing.T, client *moduleBotClient, method string) map[string]any {
	t.Helper()
	for _, other := range []string{"sendMessage", "sendPhoto", "sendVideo", "sendDocument", "sendAnimation", "sendAudio", "sendSticker"} {
		want := 0
		if other == method {
			want = 1
		}
		if got := len(client.callsFor(other)); got != want {
			t.Fatalf("%s calls = %d, want %d", other, got, want)
		}
	}
	return rulesCallParams(t, client.callsFor(method)[0])
}

// rulesSeed stores a rules row: its text, an optional media and optional buttons.
func rulesSeed(t *testing.T, chatID int64, text, fileID string, mediaType int, buttons []models.Button) {
	t.Helper()
	legacyCleanup(t, chatID)
	mustLegacy(t, rules.SetChatRules(chatID, text))
	if fileID != "" {
		mustLegacy(t, rules.SetRulesMediaOnly(chatID, fileID, mediaType))
	}
	if len(buttons) > 0 {
		mustLegacy(t, rules.SetRulesButtonsOnly(chatID, buttons))
	}
}

// rulesGroupCommand runs /rules in a supergroup and returns the client that recorded the calls and the
// context the command saw.
func rulesGroupCommand(t *testing.T, chatID int64, title string) (*moduleBotClient, *ext.Context) {
	t.Helper()
	client := rulesTestClient()
	bot := newModuleTestBot(client)
	chat := gotgbot.Chat{Id: chatID, Type: "supergroup", Title: title}
	ctx := newModuleMessageContext(bot, chat, gotgbot.User{Id: 42, FirstName: "Member"}, "/rules")
	if err := rulesModule.sendRules(bot, ctx); err != ext.EndGroups {
		t.Fatalf("sendRules() error = %v, want EndGroups", err)
	}
	return client, ctx
}

func TestRulesMediaAndButtonsInGroup(t *testing.T) {
	withRealLocales(t)
	site := []models.Button{{Name: "Site", Url: "https://example.com"}}

	cases := []struct {
		name      string
		chatID    int64
		mediaType int
		method    string
		fileKey   string
	}{
		{"photo", rulesSendPhotoChat, db.PHOTO, "sendPhoto", "photo"},
		{"video", rulesSendVideoChat, db.VIDEO, "sendVideo", "video"},
		{"document", rulesSendDocChat, db.DOCUMENT, "sendDocument", "document"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rulesSeed(t, tc.chatID, "Be kind", "rules-file", tc.mediaType, site)
			client, ctx := rulesGroupCommand(t, tc.chatID, "Rules & Co")
			params := rulesOnlyCall(t, client, tc.method)

			tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
			if got, want := params["caption"], buildRulesText(tr, "Rules & Co", "Be kind"); got != want {
				t.Fatalf("caption = %q, want %q", got, want)
			}
			if got := params[tc.fileKey]; got != "rules-file" {
				t.Fatalf("%s = %v, want rules-file", tc.fileKey, got)
			}
			if got := params["parse_mode"]; got != "HTML" {
				t.Fatalf("parse_mode = %v, want HTML", got)
			}
			markup, _ := params["reply_markup"].(map[string]any)
			rows, _ := markup["inline_keyboard"].([]any)
			if len(rows) != 1 {
				t.Fatalf("keyboard rows = %v, want the one button", markup)
			}
			button := rows[0].([]any)[0].(map[string]any)
			if button["text"] != "Site" || button["url"] != "https://example.com" {
				t.Fatalf("button = %v, want Site https://example.com", button)
			}
			reply, _ := params["reply_parameters"].(map[string]any)
			if reply["message_id"] != float64(101) || reply["allow_sending_without_reply"] != true {
				t.Fatalf("reply_parameters = %v, want the /rules message with allow_sending_without_reply", reply)
			}
		})
	}
}

// rulesDeepLinkCommand opens the private-rules deep link for the chat as the member 42 and returns the
// client and the context of the /start message. The fake's getChat answers the title "Test Chat".
func rulesDeepLinkCommand(t *testing.T, chatID int64) (*moduleBotClient, *ext.Context) {
	t.Helper()
	client := rulesTestClient()
	bot := newModuleTestBot(client)
	user := gotgbot.User{Id: 42, FirstName: "Member"}
	chat := gotgbot.Chat{Id: user.Id, Type: "private", FirstName: "Member"}
	arg := "rules_" + strconv.FormatInt(chatID, 10)
	ctx := newModuleMessageContext(bot, chat, user, "/start "+arg)
	if err := rulesDeepLinkHandler(bot, ctx, &user, arg); err != ext.EndGroups {
		t.Fatalf("rulesDeepLinkHandler() error = %v, want EndGroups", err)
	}
	return client, ctx
}

// rulesPrivateText is the private form's text spelled out from the locale, so a test does not lean on the builder.
func rulesPrivateText(t *testing.T, ctx *ext.Context, title, body string) string {
	t.Helper()
	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
	text, err := tr.GetString("rules_for_chat", i18n.TranslationParams{"first": title, "second": body})
	if err != nil {
		t.Fatalf("rules_for_chat: %v", err)
	}
	return text
}

// rulesKeyboardButtons returns the rows of the decoded reply_markup, or nil when it has none.
func rulesKeyboardButtons(params map[string]any) []any {
	markup, _ := params["reply_markup"].(map[string]any)
	rows, _ := markup["inline_keyboard"].([]any)
	return rows
}

func TestPrivateRulesMediaAndButtons(t *testing.T) {
	withRealLocales(t)
	site := []models.Button{{Name: "Site", Url: "https://example.com"}}

	t.Run("deep link", func(t *testing.T) {
		chatID := rulesChatID(1)
		rulesSeed(t, chatID, "Be kind", "rules-photo", db.PHOTO, site)
		client, ctx := rulesDeepLinkCommand(t, chatID)
		params := rulesOnlyCall(t, client, "sendPhoto")

		if got, want := params["caption"], rulesPrivateText(t, ctx, "Test Chat", normalizeRulesForHTML("Be kind")); got != want {
			t.Fatalf("caption = %q, want the private form %q", got, want)
		}
		if params["chat_id"] != float64(42) || params["photo"] != "rules-photo" || params["parse_mode"] != "HTML" {
			t.Fatalf("photo request = %v, want the member's private chat, rules-photo, HTML", params)
		}
		rows := rulesKeyboardButtons(params)
		if len(rows) != 1 {
			t.Fatalf("keyboard = %v, want the one button", params["reply_markup"])
		}
		if button := rows[0].([]any)[0].(map[string]any); button["text"] != "Site" || button["url"] != "https://example.com" {
			t.Fatalf("button = %v, want Site https://example.com", button)
		}
		reply, _ := params["reply_parameters"].(map[string]any)
		if reply["message_id"] != float64(101) || reply["allow_sending_without_reply"] != true {
			t.Fatalf("reply_parameters = %v, want a reply to the /start message", reply)
		}
	})

	t.Run("connected private chat", func(t *testing.T) {
		chatID := rulesChatID(2)
		rulesSeed(t, chatID, "Be kind", "rules-photo", db.PHOTO, site)
		user := gotgbot.User{Id: 7702, FirstName: "Member"}
		mustLegacy(t, connections.ConnectId(user.Id, chatID))
		t.Cleanup(func() { _ = connections.DisconnectId(user.Id) })

		client := rulesTestClient()
		client.responses["getChat"] = json.RawMessage(fmt.Sprintf(`{"id":%d,"type":"supergroup","title":"Connected Co"}`, chatID))
		client.responses["getChatMember"] = json.RawMessage(`{"status":"member","user":{"id":7702,"is_bot":false,"first_name":"Member"}}`)
		bot := newModuleTestBot(client)
		private := gotgbot.Chat{Id: user.Id, Type: "private", FirstName: "Member"}
		ctx := newModuleMessageContext(bot, private, user, "/rules")
		if err := rulesModule.sendRules(bot, ctx); err != ext.EndGroups {
			t.Fatalf("sendRules() error = %v, want EndGroups", err)
		}
		params := rulesOnlyCall(t, client, "sendPhoto")

		tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
		if got, want := params["caption"], buildRulesText(tr, "Connected Co", "Be kind"); got != want {
			t.Fatalf("caption = %q, want the group form %q", got, want)
		}
		if params["chat_id"] != float64(user.Id) || params["photo"] != "rules-photo" {
			t.Fatalf("photo request = %v, want rules-photo in the private chat", params)
		}
		if rows := rulesKeyboardButtons(params); len(rows) != 1 {
			t.Fatalf("keyboard = %v, want the one button", params["reply_markup"])
		}
	})
}

func TestRulesMediaOnlyAndButtonsOnly(t *testing.T) {
	withRealLocales(t)
	site := []models.Button{{Name: "Site", Url: "https://example.com"}}
	groupHeading := func(ctx *ext.Context, title string) string {
		tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
		header, _ := tr.GetString("rules_for_chat_header")
		return fmt.Sprintf(header, title)
	}
	noRules := func(ctx *ext.Context, key string) string {
		tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
		text, _ := tr.GetString(key)
		return text
	}

	t.Run("media only", func(t *testing.T) {
		chatID := rulesChatID(11)
		rulesSeed(t, chatID, "", "rules-photo", db.PHOTO, nil)
		client, ctx := rulesGroupCommand(t, chatID, "Rules Co")
		params := rulesOnlyCall(t, client, "sendPhoto")
		if got, want := params["caption"], groupHeading(ctx, "Rules Co"); got != want {
			t.Fatalf("group caption = %q, want the bare heading %q", got, want)
		}

		linkChat := rulesChatID(12)
		rulesSeed(t, linkChat, "", "rules-photo", db.PHOTO, nil)
		client, ctx = rulesDeepLinkCommand(t, linkChat)
		params = rulesOnlyCall(t, client, "sendPhoto")
		want := strings.TrimRight(rulesPrivateText(t, ctx, "Test Chat", ""), "\n")
		if got := params["caption"]; got != want {
			t.Fatalf("private caption = %q, want the bare private heading %q", got, want)
		}
	})

	t.Run("buttons only", func(t *testing.T) {
		chatID := rulesChatID(13)
		rulesSeed(t, chatID, "", "", 0, site)
		client, ctx := rulesGroupCommand(t, chatID, "Rules Co")
		params := rulesOnlyCall(t, client, "sendMessage")
		if got, want := params["text"], groupHeading(ctx, "Rules Co"); got != want {
			t.Fatalf("group text = %q, want the heading %q", got, want)
		}
		if rows := rulesKeyboardButtons(params); len(rows) != 1 {
			t.Fatalf("keyboard = %v, want the one button", params["reply_markup"])
		}

		linkChat := rulesChatID(14)
		rulesSeed(t, linkChat, "", "", 0, site)
		client, _ = rulesDeepLinkCommand(t, linkChat)
		params = rulesOnlyCall(t, client, "sendMessage")
		if rows := rulesKeyboardButtons(params); len(rows) != 1 {
			t.Fatalf("deep link keyboard = %v, want the one button", params["reply_markup"])
		}
	})

	t.Run("nothing", func(t *testing.T) {
		chatID := rulesChatID(15)
		rulesSeed(t, chatID, "", "", 0, nil)
		client, ctx := rulesGroupCommand(t, chatID, "Rules Co")
		params := rulesOnlyCall(t, client, "sendMessage")
		if got, want := params["text"], noRules(ctx, "rules_no_rules_set"); got != want {
			t.Fatalf("group text = %q, want rules_no_rules_set %q", got, want)
		}

		linkChat := rulesChatID(16)
		rulesSeed(t, linkChat, "", "", 0, nil)
		client, ctx = rulesDeepLinkCommand(t, linkChat)
		params = rulesOnlyCall(t, client, "sendMessage")
		if got, want := params["text"], noRules(ctx, "rules_not_set"); got != want {
			t.Fatalf("deep link text = %q, want rules_not_set %q", got, want)
		}
	})
}

func TestRulesClickToReadHasNoMedia(t *testing.T) {
	withRealLocales(t)
	chatID := rulesChatID(21)
	rulesSeed(t, chatID, "Be kind", "rules-photo", db.PHOTO, []models.Button{{Name: "Site", Url: "https://example.com"}})
	mustLegacy(t, rules.SetPrivateRules(chatID, true))

	client, _ := rulesGroupCommand(t, chatID, "Rules Co")
	params := rulesOnlyCall(t, client, "sendMessage")
	rows := rulesKeyboardButtons(params)
	if len(rows) != 1 || len(rows[0].([]any)) != 1 {
		t.Fatalf("keyboard = %v, want only the deep-link button", params["reply_markup"])
	}
	button := rows[0].([]any)[0].(map[string]any)
	if want := fmt.Sprintf("t.me/FukuTestBot?start=rules_%d", chatID); button["url"] != want {
		t.Fatalf("button url = %v, want %s", button["url"], want)
	}
}

func TestRulesMediaBelowEverywhere(t *testing.T) {
	withRealLocales(t)
	kinds := []struct {
		name      string
		mediaType int
		method    string
		eligible  bool
	}{
		{"photo", db.PHOTO, "sendPhoto", true},
		{"video", db.VIDEO, "sendVideo", true},
		{"animation", db.ANIMATION, "sendAnimation", true},
		{"document", db.DOCUMENT, "sendDocument", false},
		{"audio", db.AUDIO, "sendAudio", false},
		{"sticker", db.STICKER, "sendSticker", false},
	}
	for i, kind := range kinds {
		for _, below := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s below=%v", kind.name, below), func(t *testing.T) {
				want := kind.eligible && below
				groupChat := rulesChatID(int64(100 + 2*i))
				linkChat := rulesChatID(int64(101 + 2*i))
				for _, chatID := range []int64{groupChat, linkChat} {
					rulesSeed(t, chatID, "Be kind", "rules-file", kind.mediaType, nil)
					mustLegacy(t, rules.SetRulesMediaBelow(chatID, below))
				}
				client, _ := rulesGroupCommand(t, groupChat, "Rules Co")
				_, got := rulesOnlyCall(t, client, kind.method)["show_caption_above_media"]
				if got != want {
					t.Fatalf("/rules: show_caption_above_media present = %v, want %v", got, want)
				}
				client, _ = rulesDeepLinkCommand(t, linkChat)
				_, got = rulesOnlyCall(t, client, kind.method)["show_caption_above_media"]
				if got != want {
					t.Fatalf("deep link: show_caption_above_media present = %v, want %v", got, want)
				}
			})
		}
	}
}
