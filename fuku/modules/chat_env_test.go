package modules

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/uasneppy/fuku_robot/fuku/db/chatconv"
	"github.com/uasneppy/fuku_robot/fuku/utils/ratelimit"
)

// chatFakeProvider is a scripted conversation provider: each call takes the next step, or answers
// chatFakeDefaultReply when the script is used up, and records the request it was sent.
type chatFakeProvider struct {
	mu       sync.Mutex
	steps    []func(ctx context.Context, req chatRequest) (chatAnswer, error)
	requests []chatRequest
}

const chatFakeDefaultReply = "Notice. Your request has been reviewed. Procedure complete."

func (f *chatFakeProvider) converse(ctx context.Context, req chatRequest) (chatAnswer, error) {
	f.mu.Lock()
	f.requests = append(f.requests, req)
	var step func(ctx context.Context, req chatRequest) (chatAnswer, error)
	if len(f.steps) > 0 {
		step, f.steps = f.steps[0], f.steps[1:]
	}
	f.mu.Unlock()
	if step != nil {
		return step(ctx, req)
	}
	return chatAnswer{Text: chatFakeDefaultReply}, nil
}

// script queues the next answers.
func (f *chatFakeProvider) script(steps ...func(ctx context.Context, req chatRequest) (chatAnswer, error)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.steps = append(f.steps, steps...)
}

func (f *chatFakeProvider) calls() []chatRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]chatRequest(nil), f.requests...)
}

func chatAnswerText(text string) func(context.Context, chatRequest) (chatAnswer, error) {
	return func(context.Context, chatRequest) (chatAnswer, error) { return chatAnswer{Text: text}, nil }
}

func chatAnswerErr(err error) func(context.Context, chatRequest) (chatAnswer, error) {
	return func(context.Context, chatRequest) (chatAnswer, error) { return chatAnswer{}, err }
}

// chatEnv is a group with conversations on, the real dispatcher with the Chat module loaded, a
// fake Telegram client, miniredis and a scripted provider. Every swapped variable is restored in
// t.Cleanup; the test never runs in parallel.
type chatEnv struct {
	t          *testing.T
	dispatcher *ext.Dispatcher
	client     *moduleBotClient
	bot        *gotgbot.Bot
	chatID     int64
	mr         *miniredis.Miniredis
	rc         *redis.Client
	provider   *chatFakeProvider
	passed     []int64
	nextUpdate int64
	nextMsg    int64
}

// newChatEnv builds the environment; conversations are on for the anthropic provider and every
// audience.
func newChatEnv(t *testing.T) *chatEnv {
	t.Helper()
	env := newChatEnvFor(t, uniqueModuleChatID())
	if err := chatconv.SetEnabled(context.Background(), env.chatID, true, 1); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	return env
}

// newChatEnvFor builds the environment over an existing group and leaves its setting as it is.
func newChatEnvFor(t *testing.T, chatID int64) *chatEnv {
	t.Helper()
	withRealLocales(t)
	mr, rc := withChatRedis(t)
	withChatProviders(t, true, true, true)
	withChatRuntime(t)

	env := &chatEnv{t: t, mr: mr, rc: rc, client: newModuleBotClient(), provider: &chatFakeProvider{}, nextUpdate: 1, nextMsg: 500}
	env.bot = newModuleTestBot(env.client)
	env.bot.BotClient = &chatTelegramFake{moduleBotClient: env.client, deleted: map[int64]bool{}}
	env.chatID = chatID
	chatConvCleanup(t, env.chatID)
	memberReqCleanup(t, []int64{env.chatID}, nil)
	chatSwap(t, &chatProviderOf, func(string) chatProvider { return env.provider })

	env.dispatcher = ext.NewDispatcher(&ext.DispatcherOpts{MaxRoutines: -1})
	LoadChat(env.dispatcher)
	env.dispatcher.AddHandlerToGroup(chatGroupRecorder{seen: &env.passed}, chatWatchGroup)
	return env
}

// withChatRuntime gives the test a fresh pool state, a fast pacer, fresh breakers and counters,
// and puts every one back afterwards.
func withChatRuntime(t *testing.T) {
	t.Helper()
	chatJobs.wait()
	chatSwap(t, &chatPacer, ratelimit.NewTelegramPacer(ratelimit.TelegramPacerOptions{
		NextKey: "fuku:chat:pace:test:next", BlockKey: "fuku:chat:pace:test:block",
		Interval: time.Millisecond, MaxRetries: 1, MaxWait: time.Second, RetryAfterUnit: 5 * time.Millisecond,
	}))
	chatSwap(t, &chatBreakers, map[string]*memberReqAIBreaker{
		chatconv.ProviderAnthropic: newMemberReqAIBreaker(),
		chatconv.ProviderGoogle:    newMemberReqAIBreaker(),
	})
	chatSwap(t, &chatCooldown, chatCooldown)
	chatSwap(t, &chatUserHourlyBudget, chatUserHourlyBudget)
	chatSwap(t, &chatChatHourlyBudget, chatChatHourlyBudget)
	chatSwap(t, &chatGlobalHourlyBudget, chatGlobalHourlyBudget)
	t.Cleanup(chatJobs.wait)
}

// chatSwap sets *target to value and restores the old value in t.Cleanup.
func chatSwap[T any](t *testing.T, target *T, value T) {
	t.Helper()
	previous := *target
	*target = value
	t.Cleanup(func() { *target = previous })
}

// member returns a fresh ordinary member.
func (e *chatEnv) member(name string) *gotgbot.User {
	id := uniqueLinkOwnerID()
	memberReqCleanup(e.t, nil, []int64{id})
	return &gotgbot.User{Id: id, FirstName: name}
}

// message builds a message in the env's group.
func (e *chatEnv) message(from *gotgbot.User, text string) *gotgbot.Message {
	e.nextMsg++
	return &gotgbot.Message{
		MessageId: e.nextMsg, Date: time.Now().Unix(), From: from, Text: text,
		Chat: gotgbot.Chat{Id: e.chatID, Type: "supergroup", Title: "Facility 7"},
	}
}

// mention builds a message that mentions the bot first.
func (e *chatEnv) mention(from *gotgbot.User, text string) *gotgbot.Message {
	handle := "@" + e.bot.Username
	msg := e.message(from, handle+" "+text)
	msg.Entities = []gotgbot.MessageEntity{{Type: "mention", Offset: 0, Length: int64(len(handle))}}
	return msg
}

// deliver runs one message through the dispatcher and waits for the jobs it started.
func (e *chatEnv) deliver(msg *gotgbot.Message) {
	e.t.Helper()
	e.nextUpdate++
	if err := e.dispatcher.ProcessUpdate(e.bot, &gotgbot.Update{UpdateId: e.nextUpdate, Message: msg}, nil); err != nil {
		e.t.Fatalf("ProcessUpdate: %v", err)
	}
	chatJobs.wait()
}

// sends returns the texts the bot sent to the group, in order.
func (e *chatEnv) sends() []string {
	var texts []string
	for _, call := range e.client.callsFor("sendMessage") {
		if fmt.Sprint(call.Params["chat_id"]) == fmt.Sprint(e.chatID) {
			texts = append(texts, fmt.Sprint(call.Params["text"]))
		}
	}
	return texts
}

// lastSend returns the parameters of the last sendMessage.
func (e *chatEnv) lastSend() map[string]any {
	e.t.Helper()
	calls := e.client.callsFor("sendMessage")
	if len(calls) == 0 {
		e.t.Fatalf("no sendMessage")
	}
	return calls[len(calls)-1].Params
}

// replyTo reads the reply_parameters of a send.
func chatReplyTo(t *testing.T, params map[string]any) gotgbot.ReplyParameters {
	t.Helper()
	var reply gotgbot.ReplyParameters
	raw, err := json.Marshal(params["reply_parameters"])
	if err != nil {
		t.Fatalf("marshal reply_parameters: %v", err)
	}
	if err := json.Unmarshal(raw, &reply); err != nil {
		t.Fatalf("reply_parameters %s: %v", raw, err)
	}
	return reply
}

// cannedText is a canned line in Ukrainian, the default group language.
func chatCanned(t *testing.T, key string) string {
	t.Helper()
	text := chatCannedText(0, key)
	if strings.TrimSpace(text) == "" {
		t.Fatalf("no text for %s", key)
	}
	return text
}

// callCount is how many Telegram calls the fake has seen.
func (e *chatEnv) callCount() int {
	e.client.mu.Lock()
	defer e.client.mu.Unlock()
	return len(e.client.calls)
}

// chatTelegramFake is the module fake with one more behaviour of the real Bot API: once a message
// is deleted, a send that replies to it without allow_sending_without_reply is refused with a 400.
type chatTelegramFake struct {
	*moduleBotClient
	mu      sync.Mutex
	deleted map[int64]bool
}

func (f *chatTelegramFake) RequestWithContext(ctx context.Context, token, method string, params map[string]any, opts *gotgbot.RequestOpts) (json.RawMessage, error) {
	switch method {
	case "deleteMessage":
		if id, ok := params["message_id"].(int64); ok {
			f.mu.Lock()
			f.deleted[id] = true
			f.mu.Unlock()
		}
	case "sendMessage":
		if reply, ok := params["reply_parameters"].(*gotgbot.ReplyParameters); ok && !reply.AllowSendingWithoutReply {
			f.mu.Lock()
			gone := f.deleted[reply.MessageId]
			f.mu.Unlock()
			if gone {
				f.moduleBotClient.mu.Lock()
				f.calls = append(f.calls, moduleBotCall{Method: "sendMessage (refused)", Params: params})
				f.moduleBotClient.mu.Unlock()
				return nil, &gotgbot.TelegramError{Method: method, Code: 400, Description: "Bad Request: message to be replied not found"}
			}
		}
	}
	return f.moduleBotClient.RequestWithContext(ctx, token, method, params, opts)
}
