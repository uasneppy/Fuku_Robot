package gemini

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	// maxMessageRunes bounds the server message kept on an HTTPError.
	maxMessageRunes = 300
	// maxTokenRunes bounds the status and reason words kept on an HTTPError.
	maxTokenRunes = 40
	// maxParsedDelay stops an absurd delay from overflowing a Duration.
	maxParsedDelay = 24 * time.Hour

	redacted = "[REDACTED]"
)

// HTTPError is a non-2xx answer from the Gemini API. Its Error text holds the
// status, the status word and the reason only; the server message is kept in
// Message for the spike tool's probe output, truncated and with the API key
// replaced, and is never part of Error.
type HTTPError struct {
	// Status is the HTTP status code.
	Status int
	// Code is the error envelope's status word, for example RESOURCE_EXHAUSTED.
	Code string
	// Reason is the ErrorInfo reason, for example API_KEY_INVALID.
	Reason string
	// Message is the server's message, truncated to 300 runes with every
	// occurrence of the key replaced by [REDACTED].
	Message string
	// RetryDelay is the wait the server asked for: RetryInfo.retryDelay, else
	// the Retry-After header, else zero.
	RetryDelay time.Duration
}

// Error prints the status, its name and the reason, never the server message.
func (e *HTTPError) Error() string {
	text := fmt.Sprintf("gemini: HTTP %d %s", e.Status, http.StatusText(e.Status))
	if e.Code != "" {
		text += " " + e.Code
	}
	if e.Reason != "" && e.Reason != e.Code {
		text += " " + e.Reason
	}
	return text
}

// Retryable reports whether one more attempt is worth making: a rate limit or
// a server-side failure.
func (e *HTTPError) Retryable() bool {
	return e.Status == http.StatusTooManyRequests || e.Status >= 500
}

// KeyRejected reports whether the answer says the key itself is not accepted:
// an API_KEY_INVALID reason, or a 401 or 403.
func (e *HTTPError) KeyRejected() bool {
	return e.Reason == "API_KEY_INVALID" || e.Status == http.StatusUnauthorized || e.Status == http.StatusForbidden
}

// errorEnvelope is Google's error body.
type errorEnvelope struct {
	Error *struct {
		Message string `json:"message"`
		Status  string `json:"status"`
		Details []struct {
			Type       string `json:"@type"`
			RetryDelay string `json:"retryDelay"`
			Reason     string `json:"reason"`
		} `json:"details"`
	} `json:"error"`
}

// newHTTPError builds the error for a non-2xx answer. Everything taken from the
// body is scrubbed of key before it is stored.
func newHTTPError(status int, header http.Header, body []byte, key string) *HTTPError {
	httpErr := &HTTPError{Status: status}

	var envelope errorEnvelope
	if json.Unmarshal(body, &envelope) == nil && envelope.Error != nil {
		httpErr.Code = truncateRunes(scrub(envelope.Error.Status, key), maxTokenRunes)
		httpErr.Message = truncateRunes(scrub(envelope.Error.Message, key), maxMessageRunes)
		for _, detail := range envelope.Error.Details {
			switch {
			case strings.HasSuffix(detail.Type, "RetryInfo"):
				if delay, ok := parseDelay(detail.RetryDelay); ok {
					httpErr.RetryDelay = delay
				}
			case strings.HasSuffix(detail.Type, "ErrorInfo"):
				if httpErr.Reason == "" {
					httpErr.Reason = truncateRunes(scrub(detail.Reason, key), maxTokenRunes)
				}
			}
		}
	}
	if httpErr.RetryDelay <= 0 {
		httpErr.RetryDelay = parseRetryAfter(header.Get("Retry-After"))
	}
	return httpErr
}

// scrub replaces every occurrence of key in s.
func scrub(s, key string) string {
	if key == "" {
		return s
	}
	return strings.ReplaceAll(s, key, redacted)
}

// parseDelay reads a protobuf Duration in its JSON form, a decimal number of
// seconds followed by "s", such as "34s" or "15.002899939s".
func parseDelay(value string) (time.Duration, bool) {
	if !strings.HasSuffix(value, "s") {
		return 0, false
	}
	seconds, err := strconv.ParseFloat(strings.TrimSuffix(value, "s"), 64)
	if err != nil || math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 {
		return 0, false
	}
	delay := time.Duration(seconds * float64(time.Second))
	if seconds > maxParsedDelay.Seconds() || delay > maxParsedDelay {
		delay = maxParsedDelay
	}
	return delay, true
}

// parseRetryAfter reads the Retry-After header in either of its two forms;
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
