package gemini

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
)

// ErrNoVerdict is wrapped by every answer that does not carry a usable spam
// probability: a block, a truncated or empty answer, unreadable JSON or a
// number outside 0..1. The caller treats all of them as "not checked".
var ErrNoVerdict = errors.New("gemini: no verdict")

// ErrBlocked is a no verdict caused by Google's safety filters. It wraps
// ErrNoVerdict, so errors.Is(err, ErrNoVerdict) stays true, and lets a caller
// count blocks apart from other failures.
var ErrBlocked = fmt.Errorf("%w: blocked", ErrNoVerdict)

// maxCategoryRunes bounds the audit-only category string.
const maxCategoryRunes = 64

// blockingFinishReasons are the candidate finish reasons that mean a safety
// filter stopped the answer.
var blockingFinishReasons = map[string]bool{
	"SAFETY":             true,
	"IMAGE_SAFETY":       true,
	"PROHIBITED_CONTENT": true,
	"BLOCKLIST":          true,
	"SPII":               true,
}

// Verdict is the only thing the bot reads from the model.
type Verdict struct {
	// SpamProbability is finite and inside 0..1.
	SpamProbability float64
	// Category is for the audit trail only; it never drives an action.
	Category string
	// PromptTokens, OutputTokens and ThoughtTokens come from usageMetadata.
	// Thought tokens are billed as output but reported separately.
	PromptTokens  int
	OutputTokens  int
	ThoughtTokens int
	// Model is the modelVersion the answer came from.
	Model string
	// ModelStage is modelStatus.modelStage (for example STABLE or DEPRECATED).
	ModelStage string
}

type response struct {
	Candidates []struct {
		Content      *responseContent `json:"content"`
		FinishReason string           `json:"finishReason"`
	} `json:"candidates"`
	PromptFeedback *promptFeedback `json:"promptFeedback"`
	UsageMetadata  *usageMetadata  `json:"usageMetadata"`
	ModelVersion   string          `json:"modelVersion"`
	ModelStatus    *modelStatus    `json:"modelStatus"`
}

type promptFeedback struct {
	BlockReason string `json:"blockReason"`
}

type usageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	ThoughtsTokenCount   int `json:"thoughtsTokenCount"`
}

type modelStatus struct {
	ModelStage string `json:"modelStage"`
}

// responsePart is a part of an answer; thought marks the model's own reasoning.
type responsePart struct {
	Text    string `json:"text"`
	Thought bool   `json:"thought"`
}

type responseContent struct {
	Parts []responsePart `json:"parts"`
}

// answer is the schema-constrained JSON the model writes into its text part.
type answer struct {
	Category any      `json:"category"`
	Prob     *float64 `json:"spam_probability"`
}

// parseVerdict reads one generateContent answer strictly: only a finite
// spam_probability in 0..1 from a candidate that finished with STOP is a
// verdict, everything else is an error wrapping ErrNoVerdict.
func parseVerdict(data []byte) (*Verdict, error) {
	var raw response
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("%w: unreadable answer", ErrNoVerdict)
	}

	if raw.PromptFeedback != nil && raw.PromptFeedback.BlockReason != "" {
		return nil, fmt.Errorf("%w (prompt %s)", ErrBlocked, safeToken(raw.PromptFeedback.BlockReason))
	}
	if len(raw.Candidates) == 0 {
		return nil, fmt.Errorf("%w: no candidates", ErrNoVerdict)
	}
	first := raw.Candidates[0]
	if first.FinishReason != "" && first.FinishReason != "STOP" {
		if blockingFinishReasons[first.FinishReason] {
			return nil, fmt.Errorf("%w (finish %s)", ErrBlocked, first.FinishReason)
		}
		return nil, fmt.Errorf("%w: finish %s", ErrNoVerdict, safeToken(first.FinishReason))
	}

	text, ok := firstAnswerText(first.Content)
	if !ok {
		return nil, fmt.Errorf("%w: no answer text", ErrNoVerdict)
	}
	var out answer
	if err := json.Unmarshal([]byte(text), &out); err != nil || out.Prob == nil {
		return nil, fmt.Errorf("%w: unreadable answer", ErrNoVerdict)
	}
	p := *out.Prob
	if math.IsNaN(p) || math.IsInf(p, 0) || p < 0 || p > 1 {
		return nil, fmt.Errorf("%w: probability out of range", ErrNoVerdict)
	}

	verdict := &Verdict{
		SpamProbability: p,
		Model:           raw.ModelVersion,
	}
	if category, ok := out.Category.(string); ok {
		verdict.Category = truncateRunes(category, maxCategoryRunes)
	}
	if raw.UsageMetadata != nil {
		verdict.PromptTokens = raw.UsageMetadata.PromptTokenCount
		verdict.OutputTokens = raw.UsageMetadata.CandidatesTokenCount
		verdict.ThoughtTokens = raw.UsageMetadata.ThoughtsTokenCount
	}
	if raw.ModelStatus != nil {
		verdict.ModelStage = safeToken(raw.ModelStatus.ModelStage)
	}
	return verdict, nil
}

// firstAnswerText returns the first non-empty text part that is not a thought.
func firstAnswerText(c *responseContent) (string, bool) {
	if c == nil {
		return "", false
	}
	for _, p := range c.Parts {
		if p.Thought || p.Text == "" {
			continue
		}
		return p.Text, true
	}
	return "", false
}

// safeToken bounds an enum-like string taken from the answer before it is put
// into an error or a verdict.
func safeToken(s string) string {
	return truncateRunes(s, 40)
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
