//go:build testtools

package modules

import (
	"encoding/json"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/uasneppy/fuku_robot/fuku/db"
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
	client := legacyGreetingClient()
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
