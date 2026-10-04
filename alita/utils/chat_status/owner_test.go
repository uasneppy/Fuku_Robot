//go:build testtools

package chat_status

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

// ownerBotClient answers getChatAdministrators with canned JSON or a scripted error.
type ownerBotClient struct {
	admins json.RawMessage
	err    error
}

func (c ownerBotClient) RequestWithContext(_ context.Context, _ string, method string, _ map[string]any, _ *gotgbot.RequestOpts) (json.RawMessage, error) {
	if method != "getChatAdministrators" {
		return json.RawMessage(`true`), nil
	}
	if c.err != nil {
		return nil, c.err
	}
	return c.admins, nil
}

func (ownerBotClient) GetAPIURL(*gotgbot.RequestOpts) string {
	return "https://api.telegram.org"
}

func (ownerBotClient) FileURL(token string, path string, _ *gotgbot.RequestOpts) string {
	return "https://api.telegram.org/file/bot" + token + "/" + path
}

func newOwnerBot(client ownerBotClient) *gotgbot.Bot {
	return &gotgbot.Bot{
		Token:     "999:test",
		BotClient: client,
		User:      gotgbot.User{Id: 999, IsBot: true, Username: "OwnerTestBot"},
	}
}

const (
	creatorTen    = `{"status":"creator","user":{"id":10,"is_bot":false,"first_name":"Ten"}}`
	creatorEleven = `{"status":"creator","user":{"id":11,"is_bot":false,"first_name":"Eleven"}}`
	adminTwelve   = `{"status":"administrator","user":{"id":12,"is_bot":false,"first_name":"Twelve"},"can_restrict_members":true}`
	botAdmin      = `{"status":"administrator","user":{"id":999,"is_bot":true,"first_name":"Bot"},"can_restrict_members":true}`
)

func TestCheckOwnerMatch(t *testing.T) {
	bot := newOwnerBot(ownerBotClient{admins: json.RawMessage("[" + botAdmin + "," + creatorTen + "," + adminTwelve + "]")})

	result, creatorID, err := CheckOwner(bot, -1001, 10)
	if result != OwnerMatch || creatorID != 10 || err != nil {
		t.Fatalf("CheckOwner = (%v, %d, %v), want (OwnerMatch, 10, nil)", result, creatorID, err)
	}
}

func TestCheckOwnerMismatchDifferentCreator(t *testing.T) {
	bot := newOwnerBot(ownerBotClient{admins: json.RawMessage("[" + botAdmin + "," + creatorEleven + "]")})

	result, creatorID, err := CheckOwner(bot, -1001, 10)
	if result != OwnerMismatch || creatorID != 11 || err != nil {
		t.Fatalf("CheckOwner = (%v, %d, %v), want (OwnerMismatch, 11, nil)", result, creatorID, err)
	}
}

func TestCheckOwnerMismatchNoCreator(t *testing.T) {
	bot := newOwnerBot(ownerBotClient{admins: json.RawMessage("[" + botAdmin + "," + adminTwelve + "]")})

	result, creatorID, err := CheckOwner(bot, -1001, 10)
	if result != OwnerMismatch || creatorID != 0 || err != nil {
		t.Fatalf("CheckOwner = (%v, %d, %v), want (OwnerMismatch, 0, nil)", result, creatorID, err)
	}
}

// An API error must never read as a mismatch: only a mismatch may remove state.
func TestCheckOwnerUnknownOnError(t *testing.T) {
	cases := map[string]error{
		"429 flood wait": &gotgbot.TelegramError{Method: "getChatAdministrators", Code: 429, Description: "Too Many Requests: retry after 5"},
		"400 bad chat":   &gotgbot.TelegramError{Method: "getChatAdministrators", Code: 400, Description: "Bad Request: chat not found"},
		"plain error":    errors.New("connection reset"),
	}
	for name, scripted := range cases {
		t.Run(name, func(t *testing.T) {
			bot := newOwnerBot(ownerBotClient{err: scripted})

			result, creatorID, err := CheckOwner(bot, -1001, 10)
			if result != OwnerUnknown {
				t.Fatalf("result = %v, want OwnerUnknown (an error must not read as a mismatch)", result)
			}
			if creatorID != 0 || err == nil {
				t.Fatalf("CheckOwner = (%v, %d, %v), want creator 0 and a non-nil error", result, creatorID, err)
			}
		})
	}
}
