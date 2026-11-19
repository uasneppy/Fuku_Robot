package modules

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/uasneppy/fuku_robot/fuku/config"
	"github.com/uasneppy/fuku_robot/fuku/db/chatconv"
	"github.com/uasneppy/fuku_robot/fuku/db/lang"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/chatprompt"
	"github.com/uasneppy/fuku_robot/fuku/utils/claude"
	"github.com/uasneppy/fuku_robot/fuku/utils/gemini"
)

// The request of one conversation turn and the one interface both providers sit behind, so the
// runtime never branches on the provider.

// chatMaxOutputTokens is the output cap of a conversation request on both providers. Thinking
// tokens count against it, which is why it is far above the visible reply.
const chatMaxOutputTokens = 2000

// chatOutcome is how a provider answer ended.
type chatOutcome int

const (
	// chatAnswered is a complete answer.
	chatAnswered chatOutcome = iota
	// chatRefused is the provider declining to answer.
	chatRefused
	// chatTruncated is an answer cut by the output cap that still has text.
	chatTruncated
)

// chatRequest is one turn as both providers take it.
type chatRequest struct {
	Static  string
	Dynamic string
	User    string
}

// chatAnswer is what the runtime reads from a provider.
type chatAnswer struct {
	Text         string
	Outcome      chatOutcome
	InputTokens  int
	OutputTokens int
}

// chatProvider is one provider's conversation call.
type chatProvider interface {
	converse(ctx context.Context, req chatRequest) (chatAnswer, error)
}

var (
	// chatAnthropicEndpoint and chatGeminiEndpoint are the provider hosts; variables only so a
	// test can point them at an httptest server.
	chatAnthropicEndpoint = claude.DefaultEndpoint
	chatGeminiEndpoint    = gemini.DefaultEndpoint
	// chatProviderHTTP is the transport of both providers; nil is each client's own.
	chatProviderHTTP *http.Client
)

// chatProviderFor returns the adapter of a provider with this server's key and model.
func chatProviderFor(provider string) chatProvider {
	cfg := config.AppConfig
	if provider == chatconv.ProviderGoogle {
		return chatGeminiProvider{client: &gemini.Client{
			Endpoint: chatGeminiEndpoint, APIKey: cfg.GeminiAPIKey, Model: cfg.ChatGeminiModel, HTTP: chatProviderHTTP,
		}}
	}
	return chatAnthropicProvider{client: &claude.Client{
		Endpoint: chatAnthropicEndpoint, APIKey: cfg.AnthropicAPIKey, Model: cfg.ChatAnthropicModel, HTTP: chatProviderHTTP,
	}}
}

type chatAnthropicProvider struct{ client *claude.Client }

func (p chatAnthropicProvider) converse(ctx context.Context, req chatRequest) (chatAnswer, error) {
	reply, err := p.client.Converse(ctx, claude.ConverseRequest{
		Static: req.Static, Dynamic: req.Dynamic, User: req.User, MaxTokens: chatMaxOutputTokens,
	})
	if err != nil {
		return chatAnswer{}, err
	}
	return chatAnswerOf(reply.Text, reply.Outcome == claude.OutcomeRefused, reply.Outcome == claude.OutcomeTruncated,
		reply.InputTokens, reply.OutputTokens), nil
}

type chatGeminiProvider struct{ client *gemini.Client }

func (p chatGeminiProvider) converse(ctx context.Context, req chatRequest) (chatAnswer, error) {
	reply, err := p.client.Generate(ctx, gemini.Request{
		Static: req.Static, Dynamic: req.Dynamic, User: req.User, MaxTokens: chatMaxOutputTokens,
	})
	if err != nil {
		return chatAnswer{}, err
	}
	return chatAnswerOf(reply.Text, reply.Outcome == gemini.OutcomeRefused, reply.Outcome == gemini.OutcomeTruncated,
		reply.InputTokens, reply.OutputTokens), nil
}

// chatAnswerOf builds the runtime's answer from a provider's reply.
func chatAnswerOf(text string, refused, truncated bool, input, output int) chatAnswer {
	answer := chatAnswer{Text: text, Outcome: chatAnswered, InputTokens: input, OutputTokens: output}
	switch {
	case refused:
		answer.Outcome = chatRefused
	case truncated:
		answer.Outcome = chatTruncated
	}
	return answer
}

// chatErrorClass names a provider error for a log line and the breaker: the HTTP status and the
// error type or status, never the message, which can quote the request.
func chatErrorClass(err error) string {
	var claudeErr *claude.HTTPError
	if errors.As(err, &claudeErr) {
		return "HTTP " + strconv.Itoa(claudeErr.Status) + " " + claudeErr.Type
	}
	var geminiErr *gemini.HTTPError
	if errors.As(err, &geminiErr) {
		return "HTTP " + strconv.Itoa(geminiErr.Status) + " " + geminiErr.GRPCStatus
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case errors.Is(err, claude.ErrNoReply), errors.Is(err, gemini.ErrNoReply):
		return "no reply"
	}
	return "transport error"
}

// chatProviderSpan says whether err calls for a pause of the provider for every group, and for
// how long: a rejected key or account, a 429 whose retry delay is over 2 s (at most 5 minutes) and
// Gemini's per-day quota.
func chatProviderSpan(err error) time.Duration {
	var claudeErr *claude.HTTPError
	if errors.As(err, &claudeErr) {
		if claudeErr.KeyRejected() {
			return memberReqAIKeyPause
		}
		if claudeErr.Status == http.StatusTooManyRequests && claudeErr.RetryDelay > memberReqAILongRateLimit {
			return min(claudeErr.RetryDelay, memberReqAIMaxPause)
		}
		return 0
	}
	var geminiErr *gemini.HTTPError
	if errors.As(err, &geminiErr) {
		if geminiErr.KeyRejected() {
			return memberReqAIKeyPause
		}
		if geminiErr.Status == http.StatusTooManyRequests {
			if geminiErr.DailyQuota {
				return memberReqAIMaxPause
			}
			if geminiErr.RetryDelay > memberReqAILongRateLimit {
				return min(geminiErr.RetryDelay, memberReqAIMaxPause)
			}
		}
	}
	return 0
}

// chatLanguageName is the group language's own name, for the dynamic system text.
func chatLanguageName(ctx context.Context, chatID int64) (code, name string) {
	code = lang.GetGroupLanguage(ctx, chatID)
	name, err := i18n.MustNewTranslator(code).GetString("language_name")
	if err != nil || strings.TrimSpace(name) == "" {
		name = code
	}
	return code, name
}

// chatBuildRequest assembles the three strings of a turn. The static text is the same bytes for
// every chat; the dynamic text holds the group's title, language, today's date in the group's zone
// and the bot's username; the user text holds the transcript, the quoted message and the message
// to answer. No user ID, username or chat ID is in any of them.
func chatBuildRequest(ctx context.Context, job chatJob, botUsername string, history []chatprompt.Turn) chatRequest {
	_, language := chatLanguageName(ctx, job.chatID)
	date := time.Now().In(chatClock(ctx, job.chatID)).Format("2006-01-02")
	return chatRequest{
		Static: chatprompt.Static(),
		Dynamic: chatprompt.Dynamic(chatprompt.Chat{
			Title: job.chatTitle, Language: language, Date: date, BotUsername: botUsername,
		}),
		User: chatprompt.User(history, job.quoted, job.turn),
	}
}
