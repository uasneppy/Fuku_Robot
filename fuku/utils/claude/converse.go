package claude

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	// ConverseTimeout bounds one whole Converse call, retry included. A
	// conversation thinks before it answers, so it needs far more than a
	// classification's Timeout.
	ConverseTimeout = 40 * time.Second
	// ConverseEffort is the output_config effort of a conversation: low is
	// enough for a short chat reply on every 5.x model.
	ConverseEffort = "low"

	// fallbackBeta is the anthropic-beta value of the server-side refusal
	// fallback.
	fallbackBeta = "server-side-fallback-2026-07-01"
)

// ErrNoReply is wrapped by every conversation answer that carries no usable
// text: a stop reason other than end_turn, max_tokens or refusal, a blank
// answer, or an unreadable body.
var ErrNoReply = errors.New("claude: no reply")

// Outcome is how a conversation answer ended.
type Outcome int

const (
	// OutcomeAnswered is a complete answer.
	OutcomeAnswered Outcome = iota
	// OutcomeRefused is the model declining to answer; Reply.Text is empty.
	OutcomeRefused
	// OutcomeTruncated is an answer cut by max_tokens that still has text.
	OutcomeTruncated
)

// ConverseRequest is one conversation turn. Static is the cached persona and
// archive and must be the same bytes for every chat; Dynamic is the short
// per-chat block after it; User is the one user message.
type ConverseRequest struct {
	Static    string
	Dynamic   string
	User      string
	MaxTokens int
}

// Reply is what the bot reads from a conversation answer.
type Reply struct {
	Text    string
	Outcome Outcome
	// Token counts from the answer's usage.
	InputTokens         int
	OutputTokens        int
	CacheReadTokens     int
	CacheCreationTokens int
	// Model is the model id the answer reports.
	Model string
}

// conversationHTTP is the transport of a conversation: a longer limit than a
// classification, and no redirect followed.
var conversationHTTP = &http.Client{
	Timeout: ConverseTimeout,
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

// serverFallbackPrefixes are the model families that take the server-side
// refusal fallback. A Haiku model has none, and a fallback field on one is a
// 400, so it is never sent there.
var serverFallbackPrefixes = []string{"claude-opus-5", "claude-sonnet-5-5", "claude-fable-5"}

// ServerFallback reports whether a conversation with model sends
// "fallbacks": "default" and the beta header.
func ServerFallback(model string) bool {
	for _, prefix := range serverFallbackPrefixes {
		if strings.HasPrefix(model, prefix) {
			return true
		}
	}
	return false
}

type converseSystemBlock struct {
	Type         string        `json:"type"`
	Text         string        `json:"text"`
	CacheControl *cacheControl `json:"cache_control,omitempty"`
}

type cacheControl struct {
	Type string `json:"type"`
}

type converseTextBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type converseMessage struct {
	Role    string              `json:"role"`
	Content []converseTextBlock `json:"content"`
}

// converseBody is the conversation request. It carries no thinking field (Opus
// 5.5 rejects disabled and budgets), no sampling field, no tools and no
// assistant prefill.
type converseBody struct {
	Model        string                `json:"model"`
	MaxTokens    int                   `json:"max_tokens"`
	System       []converseSystemBlock `json:"system"`
	Messages     []converseMessage     `json:"messages"`
	OutputConfig outputConfig          `json:"output_config"`
	Fallbacks    string                `json:"fallbacks,omitempty"`
}

// buildConverseBody assembles the request. The static block comes first and
// carries the cache marker; everything that varies comes after it, so the
// cached prefix is the same for every chat.
func buildConverseBody(model string, in ConverseRequest) *converseBody {
	body := &converseBody{
		Model:     model,
		MaxTokens: in.MaxTokens,
		System: []converseSystemBlock{
			{Type: "text", Text: in.Static, CacheControl: &cacheControl{Type: "ephemeral"}},
			{Type: "text", Text: in.Dynamic},
		},
		Messages:     []converseMessage{{Role: "user", Content: []converseTextBlock{{Type: "text", Text: in.User}}}},
		OutputConfig: outputConfig{Effort: ConverseEffort},
	}
	if ServerFallback(model) {
		body.Fallbacks = "default"
	}
	return body
}

// Converse sends one conversation turn and returns the reply. A refusal is a
// Reply with OutcomeRefused and no error; every answer without usable text is
// an error wrapping ErrNoReply; an HTTP failure is an *HTTPError.
func (c *Client) Converse(ctx context.Context, in ConverseRequest) (*Reply, error) {
	if c.APIKey == "" {
		return nil, ErrNotConfigured
	}
	if !ValidModel(c.Model) {
		return nil, ErrInvalidModel
	}
	if in.MaxTokens <= 0 || strings.TrimSpace(in.Static) == "" || strings.TrimSpace(in.User) == "" {
		return nil, errors.New("claude: incomplete conversation request")
	}

	payload, err := json.Marshal(buildConverseBody(c.Model, in))
	if err != nil {
		return nil, errors.New("claude: encode request")
	}

	ctx, cancel := context.WithTimeout(ctx, ConverseTimeout)
	defer cancel()

	opt := sendOptions{http: c.HTTP}
	if opt.http == nil {
		opt.http = conversationHTTP
	}
	if ServerFallback(c.Model) {
		opt.beta = fallbackBeta
	}
	body, err := c.exec(ctx, opt, http.MethodPost, c.baseURL()+"/v1/messages", payload)
	if err != nil {
		return nil, err
	}
	return parseConverseReply(body)
}

type converseResponse struct {
	Model      string `json:"model"`
	StopReason string `json:"stop_reason"`
	Content    []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Usage *struct {
		InputTokens              int `json:"input_tokens"`
		OutputTokens             int `json:"output_tokens"`
		CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	} `json:"usage"`
}

// parseConverseReply reads the stop reason before the content: end_turn joins
// the text blocks (thinking and fallback blocks are skipped), max_tokens keeps
// text that is not blank, refusal is the refused outcome, and anything else,
// or a blank answer, is no reply.
func parseConverseReply(data []byte) (*Reply, error) {
	var raw converseResponse
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("%w: unreadable answer", ErrNoReply)
	}

	reply := &Reply{Model: truncateRunes(raw.Model, maxTokenRunes)}
	if raw.Usage != nil {
		reply.InputTokens = raw.Usage.InputTokens
		reply.OutputTokens = raw.Usage.OutputTokens
		reply.CacheReadTokens = raw.Usage.CacheReadInputTokens
		reply.CacheCreationTokens = raw.Usage.CacheCreationInputTokens
	}

	switch raw.StopReason {
	case "refusal":
		reply.Outcome = OutcomeRefused
		return reply, nil
	case "end_turn":
		reply.Outcome = OutcomeAnswered
	case "max_tokens":
		reply.Outcome = OutcomeTruncated
	default:
		return nil, fmt.Errorf("%w: stop %s", ErrNoReply, safeToken(raw.StopReason))
	}

	var text strings.Builder
	for _, block := range raw.Content {
		if block.Type == "text" {
			text.WriteString(block.Text)
		}
	}
	reply.Text = strings.TrimSpace(text.String())
	if reply.Text == "" {
		return nil, fmt.Errorf("%w: blank answer", ErrNoReply)
	}
	return reply, nil
}
