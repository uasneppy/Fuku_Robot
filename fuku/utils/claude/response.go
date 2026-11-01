package claude

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
)

// ErrNoVerdict is wrapped by every answer that does not carry a usable spam
// probability: a refusal, a truncated or empty answer, unreadable JSON or a
// number outside 0..1. The caller treats all of them as "not checked".
var ErrNoVerdict = errors.New("claude: no verdict")

// ErrBlocked is a no verdict caused by the model declining to answer (a
// refusal stop). It wraps ErrNoVerdict, so errors.Is(err, ErrNoVerdict) stays
// true, and lets a caller count refusals apart from other failures.
var ErrBlocked = fmt.Errorf("%w: refused", ErrNoVerdict)

const maxTokenRunesVerdict = 40

// Verdict is the only thing the bot reads from the model.
type Verdict struct {
	// SpamProbability is finite and inside 0..1.
	SpamProbability float64
	// InputTokens and OutputTokens come from the answer's usage.
	InputTokens  int
	OutputTokens int
	// Model is the model id the answer reports.
	Model string
}

type response struct {
	Model       string         `json:"model"`
	Content     []contentReply `json:"content"`
	StopReason  string         `json:"stop_reason"`
	StopDetails *stopDetails   `json:"stop_details"`
	Usage       *usage         `json:"usage"`
}

type stopDetails struct {
	Category *string `json:"category"`
}

type usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type contentReply struct {
	Type  string          `json:"type"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// toolInput is the forced tool call's argument object.
type toolInput struct {
	SpamProbability *float64 `json:"spam_probability"`
}

// parseVerdict reads one Messages answer strictly: only a finite
// spam_probability in 0..1 from a forced tool call that ended with tool_use is
// a verdict, everything else is an error wrapping ErrNoVerdict.
func parseVerdict(data []byte) (*Verdict, error) {
	var raw response
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("%w: unreadable answer", ErrNoVerdict)
	}

	if raw.StopReason == "refusal" {
		if raw.StopDetails != nil && raw.StopDetails.Category != nil && *raw.StopDetails.Category != "" {
			return nil, fmt.Errorf("%w (category %s)", ErrBlocked, safeToken(*raw.StopDetails.Category))
		}
		return nil, ErrBlocked
	}
	if raw.StopReason != "tool_use" {
		return nil, fmt.Errorf("%w: stop %s", ErrNoVerdict, safeToken(raw.StopReason))
	}

	var call *contentReply
	for i := range raw.Content {
		if raw.Content[i].Type == "tool_use" && raw.Content[i].Name == ToolName {
			call = &raw.Content[i]
			break
		}
	}
	if call == nil {
		return nil, fmt.Errorf("%w: no tool call", ErrNoVerdict)
	}

	var in toolInput
	if err := json.Unmarshal(call.Input, &in); err != nil || in.SpamProbability == nil {
		return nil, fmt.Errorf("%w: unreadable answer", ErrNoVerdict)
	}
	p := *in.SpamProbability
	if math.IsNaN(p) || math.IsInf(p, 0) || p < 0 || p > 1 {
		return nil, fmt.Errorf("%w: probability out of range", ErrNoVerdict)
	}

	verdict := &Verdict{
		SpamProbability: p,
		Model:           truncateRunes(raw.Model, maxTokenRunes),
	}
	if raw.Usage != nil {
		verdict.InputTokens = raw.Usage.InputTokens
		verdict.OutputTokens = raw.Usage.OutputTokens
	}
	return verdict, nil
}

// safeToken bounds an enum-like string taken from the answer before it is put
// into an error or a verdict.
func safeToken(s string) string {
	return truncateRunes(s, maxTokenRunes)
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	n := 0
	for i := range s {
		if n == max {
			return s[:i]
		}
		n++
	}
	return s
}
