//go:build testtools

package chat_status

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

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

// memberBotClient answers getChatMember with canned JSON, a scripted error, or,
// when block is set, by waiting for the caller's context to end.
type memberBotClient struct {
	member json.RawMessage
	err    error
	block  bool
	asked  *[]any
}

func (c memberBotClient) RequestWithContext(ctx context.Context, _ string, method string, params map[string]any, _ *gotgbot.RequestOpts) (json.RawMessage, error) {
	if method != "getChatMember" {
		return json.RawMessage(`true`), nil
	}
	if c.asked != nil {
		*c.asked = append(*c.asked, params["user_id"])
	}
	if c.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if c.err != nil {
		return nil, c.err
	}
	return c.member, nil
}

func (memberBotClient) GetAPIURL(*gotgbot.RequestOpts) string {
	return "https://api.telegram.org"
}

func (memberBotClient) FileURL(token string, path string, _ *gotgbot.RequestOpts) string {
	return "https://api.telegram.org/file/bot" + token + "/" + path
}

func newMemberBot(client memberBotClient) *gotgbot.Bot {
	return &gotgbot.Bot{
		Token:     "999:test",
		BotClient: client,
		User:      gotgbot.User{Id: 999, IsBot: true, Username: "MemberTestBot"},
	}
}

func TestFetchBotMemberFound(t *testing.T) {
	var asked []any
	bot := newMemberBot(memberBotClient{member: json.RawMessage(botAdmin), asked: &asked})

	member, result, err := FetchBotMember(bot, -1001)
	if result != BotMemberFound || err != nil {
		t.Fatalf("FetchBotMember = (%v, %v), want (BotMemberFound, nil)", result, err)
	}
	if member.Status != "administrator" || !member.CanRestrictMembers {
		t.Fatalf("member = %+v, want administrator with can_restrict_members", member)
	}
	if len(asked) != 1 || fmt.Sprint(asked[0]) != "999" {
		t.Fatalf("getChatMember asked about %v, want the bot's own ID 999", asked)
	}
}

func TestFetchBotMemberFoundPlainMember(t *testing.T) {
	plain := `{"status":"member","user":{"id":999,"is_bot":true,"first_name":"Bot"}}`
	bot := newMemberBot(memberBotClient{member: json.RawMessage(plain)})

	member, result, err := FetchBotMember(bot, -1001)
	if result != BotMemberFound || err != nil || member.Status != "member" {
		t.Fatalf("FetchBotMember = (%+v, %v, %v), want a found plain member", member, result, err)
	}
}

func TestFetchBotMemberMissing(t *testing.T) {
	cases := map[string]error{
		"kicked":         &gotgbot.TelegramError{Method: "getChatMember", Code: 403, Description: "Forbidden: bot was kicked from the supergroup chat"},
		"not a member":   &gotgbot.TelegramError{Method: "getChatMember", Code: 403, Description: "Forbidden: bot is not a member of the supergroup chat"},
		"chat not found": &gotgbot.TelegramError{Method: "getChatMember", Code: 400, Description: "Bad Request: chat not found"},
	}
	for name, scripted := range cases {
		t.Run(name, func(t *testing.T) {
			bot := newMemberBot(memberBotClient{err: scripted})

			_, result, err := FetchBotMember(bot, -1001)
			if result != BotMemberMissing || err != nil {
				t.Fatalf("FetchBotMember = (%v, %v), want (BotMemberMissing, nil)", result, err)
			}
		})
	}
}

func TestFetchBotMemberMissingWhenStatusLeftOrKicked(t *testing.T) {
	for _, status := range []string{"left", "kicked"} {
		t.Run(status, func(t *testing.T) {
			body := `{"status":"` + status + `","user":{"id":999,"is_bot":true,"first_name":"Bot"}}`
			bot := newMemberBot(memberBotClient{member: json.RawMessage(body)})

			_, result, err := FetchBotMember(bot, -1001)
			if result != BotMemberMissing || err != nil {
				t.Fatalf("FetchBotMember = (%v, %v), want (BotMemberMissing, nil)", result, err)
			}
		})
	}
}

// Anything that is not a definite "the bot is not there" answer must stay
// unknown, so a flood wait or a timeout is never read as the bot being absent.
func TestFetchBotMemberUnknown(t *testing.T) {
	cases := map[string]error{
		"429 flood wait":  &gotgbot.TelegramError{Method: "getChatMember", Code: 429, Description: "Too Many Requests: retry after 5"},
		"other 400":       &gotgbot.TelegramError{Method: "getChatMember", Code: 400, Description: "Bad Request: user not found"},
		"plain error":     errors.New("connection reset"),
		"deadline passed": context.DeadlineExceeded,
	}
	for name, scripted := range cases {
		t.Run(name, func(t *testing.T) {
			bot := newMemberBot(memberBotClient{err: scripted})

			_, result, err := FetchBotMember(bot, -1001)
			if result != BotMemberUnknown || err == nil {
				t.Fatalf("FetchBotMember = (%v, %v), want (BotMemberUnknown, non-nil error)", result, err)
			}
		})
	}
}

func TestFetchBotMemberTimeoutIsUnknown(t *testing.T) {
	previous := liveCheckTimeout
	liveCheckTimeout = 20 * time.Millisecond
	t.Cleanup(func() { liveCheckTimeout = previous })
	bot := newMemberBot(memberBotClient{block: true})

	_, result, err := FetchBotMember(bot, -1001)
	if result != BotMemberUnknown || err == nil {
		t.Fatalf("FetchBotMember on a hung call = (%v, %v), want (BotMemberUnknown, non-nil error)", result, err)
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
