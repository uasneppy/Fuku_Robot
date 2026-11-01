package claude

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	// maxMessageRunes bounds the server message kept on an HTTPError.
	maxMessageRunes = 300
	// maxTokenRunes bounds the error type kept on an HTTPError.
	maxTokenRunes = 40
	// maxRequestIDRunes bounds the request id kept on an HTTPError.
	maxRequestIDRunes = 64
	// maxParsedDelay stops an absurd delay from overflowing a Duration.
	maxParsedDelay = 24 * time.Hour

	redacted = "[REDACTED]"
)

// HTTPError is a non-2xx answer from the Anthropic API. Its Error text holds
// the status, the status word and the error type only; the server message is
// kept in Message for a spike tool's probe output, truncated and with the API
// key replaced, and is never part of Error.
type HTTPError struct {
	// Status is the HTTP status code.
	Status int
	// Type is the error envelope's type, for example rate_limit_error.
	Type string
	// Message is the server's message, truncated to 300 runes with every
	// occurrence of the key replaced by [REDACTED].
	Message string
	// RequestID is the request id the server reported, bounded and scrubbed.
	RequestID string
	// RetryDelay is the wait the server asked for in retry-after, else zero.
	RetryDelay time.Duration
}

// Error prints the status, its name and the error type, never the server
// message or the request id.
func (e *HTTPError) Error() string {
	text := "claude: HTTP " + strconv.Itoa(e.Status)
	if word := http.StatusText(e.Status); word != "" {
		text += " " + word
	}
	if e.Type != "" {
		text += " " + e.Type
	}
	return text
}

// Retryable reports whether one more attempt is worth making: a rate limit or
// a server-side failure (529 overloaded included).
func (e *HTTPError) Retryable() bool {
	return e.Status == http.StatusTooManyRequests || e.Status >= 500
}

// KeyRejected reports whether the key or its account is not accepted: a 401,
// 402 or 403, or an authentication, billing or permission error type.
func (e *HTTPError) KeyRejected() bool {
	switch e.Status {
	case http.StatusUnauthorized, http.StatusPaymentRequired, http.StatusForbidden:
		return true
	}
	switch e.Type {
	case "authentication_error", "billing_error", "permission_error":
		return true
	}
	return false
}

// errorEnvelope is Anthropic's error body.
type errorEnvelope struct {
	Type  string `json:"type"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
	RequestID string `json:"request_id"`
}

// newHTTPError builds the error for a non-2xx answer. Everything taken from the
// body or headers is scrubbed of key before it is stored.
func newHTTPError(status int, header http.Header, body []byte, key string) *HTTPError {
	httpErr := &HTTPError{Status: status}

	var envelope errorEnvelope
	if json.Unmarshal(body, &envelope) == nil && envelope.Error != nil {
		httpErr.Type = truncateRunes(scrub(envelope.Error.Type, key), maxTokenRunes)
		httpErr.Message = truncateRunes(scrub(envelope.Error.Message, key), maxMessageRunes)
		httpErr.RequestID = truncateRunes(scrub(envelope.RequestID, key), maxRequestIDRunes)
	}
	if httpErr.RequestID == "" {
		httpErr.RequestID = truncateRunes(scrub(header.Get("request-id"), key), maxRequestIDRunes)
	}
	httpErr.RetryDelay = parseRetryAfter(header.Get("retry-after"))
	return httpErr
}

// scrub replaces every occurrence of key in s.
func scrub(s, key string) string {
	if key == "" {
		return s
	}
	return strings.ReplaceAll(s, key, redacted)
}

// parseRetryAfter reads the retry-after header in either of its two forms;
// anything unreadable is no delay.
func parseRetryAfter(value string) time.Duration {
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds <= 0 {
			return 0
		}
		delay := time.Duration(seconds) * time.Second
		if delay > maxParsedDelay || delay < 0 {
			return maxParsedDelay
		}
		return delay
	}
	if at, err := http.ParseTime(value); err == nil {
		if wait := time.Until(at); wait > 0 {
			if wait > maxParsedDelay {
				return maxParsedDelay
			}
			return wait
		}
	}
	return 0
}
