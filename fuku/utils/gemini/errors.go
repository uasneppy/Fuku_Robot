package gemini

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	maxMessageRunes = 300
	maxParsedDelay  = 24 * time.Hour
	redacted        = "[REDACTED]"
)

// HTTPError is a non-2xx answer from the Gemini API. Its Error text holds the
// status, the status word and the gRPC status only; the server message is kept
// in Message, truncated and with the key replaced, and is never part of Error.
type HTTPError struct {
	// Status is the HTTP status code.
	Status int
	// GRPCStatus is the error's status, for example RESOURCE_EXHAUSTED.
	GRPCStatus string
	// Reason is the ErrorInfo reason, for example API_KEY_INVALID.
	Reason string
	// Message is the server's message, truncated to 300 runes with every
	// occurrence of the key replaced by [REDACTED].
	Message string
	// RetryDelay is the wait from a RetryInfo detail or a retry-after header,
	// else zero. Google does not document either, so it is never relied on.
	RetryDelay time.Duration
	// DailyQuota is set when a QuotaFailure names a per-day quota, which a
	// retry within the hour cannot clear.
	DailyQuota bool
}

// Error prints the status, its name and the gRPC status, never the message.
func (e *HTTPError) Error() string {
	text := "gemini: HTTP " + strconv.Itoa(e.Status)
	if word := http.StatusText(e.Status); word != "" {
		text += " " + word
	}
	if e.GRPCStatus != "" {
		text += " " + e.GRPCStatus
	}
	return text
}

// Retryable reports whether one more attempt is worth making: a 408, a 5xx, or
// a 429 that is not a per-day quota.
func (e *HTTPError) Retryable() bool {
	switch e.Status {
	case http.StatusTooManyRequests:
		return !e.DailyQuota
	case http.StatusRequestTimeout:
		return true
	default:
		return e.Status >= 500
	}
}

// KeyRejected reports whether the key or its account is not accepted: a 401,
// 402 or 403, or a 400 that says the key is invalid.
func (e *HTTPError) KeyRejected() bool {
	switch e.Status {
	case http.StatusUnauthorized, http.StatusPaymentRequired, http.StatusForbidden:
		return true
	case http.StatusBadRequest:
		return e.Reason == "API_KEY_INVALID" || strings.Contains(strings.ToLower(e.Message), "api key not valid")
	}
	return false
}

type errorEnvelope struct {
	Error *struct {
		Code    int               `json:"code"`
		Message string            `json:"message"`
		Status  string            `json:"status"`
		Details []json.RawMessage `json:"details"`
	} `json:"error"`
}

type errorDetail struct {
	Type       string `json:"@type"`
	Reason     string `json:"reason"`
	RetryDelay string `json:"retryDelay"`
	Violations []struct {
		QuotaID string `json:"quotaId"`
	} `json:"violations"`
}

// newHTTPError builds the error for a non-2xx answer. Everything taken from the
// body is scrubbed of the key before it is stored.
func newHTTPError(status int, header http.Header, body []byte, key string) *HTTPError {
	httpErr := &HTTPError{Status: status}
	var envelope errorEnvelope
	if json.Unmarshal(body, &envelope) == nil && envelope.Error != nil {
		httpErr.GRPCStatus = truncateRunes(scrub(envelope.Error.Status, key), maxTokenRunes)
		httpErr.Message = truncateRunes(scrub(envelope.Error.Message, key), maxMessageRunes)
		for _, raw := range envelope.Error.Details {
			var d errorDetail
			if json.Unmarshal(raw, &d) != nil {
				continue
			}
			switch {
			case strings.HasSuffix(d.Type, "google.rpc.ErrorInfo"):
				httpErr.Reason = truncateRunes(scrub(d.Reason, key), maxTokenRunes)
			case strings.HasSuffix(d.Type, "google.rpc.RetryInfo"):
				httpErr.RetryDelay = parseDelay(d.RetryDelay)
			case strings.HasSuffix(d.Type, "google.rpc.QuotaFailure"):
				for _, v := range d.Violations {
					if strings.Contains(v.QuotaID, "PerDay") {
						httpErr.DailyQuota = true
					}
				}
			}
		}
	}
	if httpErr.RetryDelay == 0 {
		httpErr.RetryDelay = parseRetryAfter(header.Get("retry-after"))
	}
	return httpErr
}

func scrub(s, key string) string {
	if key == "" {
		return s
	}
	return strings.ReplaceAll(s, key, redacted)
}

// parseDelay reads a protobuf Duration such as "27s" or "1.5s".
func parseDelay(value string) time.Duration {
	d, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil || d <= 0 {
		return 0
	}
	if d > maxParsedDelay {
		return maxParsedDelay
	}
	return d
}

// parseRetryAfter reads a retry-after header given in seconds.
func parseRetryAfter(value string) time.Duration {
	seconds, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || seconds <= 0 {
		return 0
	}
	if d := time.Duration(seconds) * time.Second; d < maxParsedDelay {
		return d
	}
	return maxParsedDelay
}
