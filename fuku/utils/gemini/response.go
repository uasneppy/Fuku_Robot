package gemini

import (
	"encoding/json"
	"fmt"
	"strings"
)

const maxTokenRunes = 40

// Outcome is how an answer ended.
type Outcome int

const (
	// OutcomeAnswered is a complete answer.
	OutcomeAnswered Outcome = iota
	// OutcomeRefused is a blocked prompt or a safety stop; Reply.Text is empty.
	OutcomeRefused
	// OutcomeTruncated is an answer cut by maxOutputTokens that still has text.
	OutcomeTruncated
)

// Reply is what the bot reads from an answer.
type Reply struct {
	Text    string
	Outcome Outcome
	// Token counts from usageMetadata. InputTokens includes cached tokens.
	InputTokens     int
	OutputTokens    int
	ThoughtsTokens  int
	CacheReadTokens int
	// Model is the model version the answer reports.
	Model string
}

type responseBody struct {
	Candidates []struct {
		Content *struct {
			Parts []struct {
				Text    string `json:"text"`
				Thought bool   `json:"thought"`
			} `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	PromptFeedback *struct {
		BlockReason string `json:"blockReason"`
	} `json:"promptFeedback"`
	UsageMetadata *struct {
		PromptTokenCount        int `json:"promptTokenCount"`
		CandidatesTokenCount    int `json:"candidatesTokenCount"`
		ThoughtsTokenCount      int `json:"thoughtsTokenCount"`
		CachedContentTokenCount int `json:"cachedContentTokenCount"`
	} `json:"usageMetadata"`
	ModelVersion string `json:"modelVersion"`
}

// refusedFinish are the finish reasons that mean the answer was blocked.
var refusedFinish = map[string]bool{
	"SAFETY":             true,
	"PROHIBITED_CONTENT": true,
	"BLOCKLIST":          true,
	"SPII":               true,
	"RECITATION":         true,
}

// parseReply reads one answer. It is tolerant of letter case in the enum
// values and of both blocked-prompt shapes (no candidates, or an empty list).
// A blocked prompt or a refused finish is the refused outcome; STOP joins the
// non-thought text parts; MAX_TOKENS keeps text that is not blank; anything
// else, or a blank answer, is no reply.
func parseReply(data []byte) (*Reply, error) {
	var raw responseBody
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("%w: unreadable answer", ErrNoReply)
	}

	reply := &Reply{Model: truncateRunes(raw.ModelVersion, maxTokenRunes)}
	if u := raw.UsageMetadata; u != nil {
		reply.InputTokens = u.PromptTokenCount
		reply.OutputTokens = u.CandidatesTokenCount
		reply.ThoughtsTokens = u.ThoughtsTokenCount
		reply.CacheReadTokens = u.CachedContentTokenCount
	}

	blocked := raw.PromptFeedback != nil && strings.TrimSpace(raw.PromptFeedback.BlockReason) != ""
	if len(raw.Candidates) == 0 {
		if blocked {
			reply.Outcome = OutcomeRefused
			return reply, nil
		}
		return nil, fmt.Errorf("%w: no candidates", ErrNoReply)
	}

	candidate := raw.Candidates[0]
	finish := strings.ToUpper(strings.TrimSpace(candidate.FinishReason))
	switch {
	case refusedFinish[finish] || (blocked && finish != "STOP" && finish != "MAX_TOKENS"):
		reply.Outcome = OutcomeRefused
		return reply, nil
	case finish == "STOP":
		reply.Outcome = OutcomeAnswered
	case finish == "MAX_TOKENS":
		reply.Outcome = OutcomeTruncated
	default:
		return nil, fmt.Errorf("%w: finish %s", ErrNoReply, truncateRunes(finish, maxTokenRunes))
	}

	var text strings.Builder
	if candidate.Content != nil {
		for _, p := range candidate.Content.Parts {
			if !p.Thought {
				text.WriteString(p.Text)
			}
		}
	}
	reply.Text = strings.TrimSpace(text.String())
	if reply.Text == "" {
		return nil, fmt.Errorf("%w: blank answer", ErrNoReply)
	}
	return reply, nil
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
