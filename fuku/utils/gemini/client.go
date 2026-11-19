// Package gemini is the Google Gemini generateContent client of the
// conversation feature.
//
// It imports the standard library only, so a spike tool can use it without
// pulling in fuku/config or fuku/db, whose init functions connect to
// PostgreSQL before main. The API key travels only in the x-goog-api-key
// header, never in a URL (the ?key= form Google also documents is not used),
// no redirect is followed, and nothing in this package logs: callers log, and
// every error returned here is free of the key and of the URL.
package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	// DefaultEndpoint is the Gemini API host.
	DefaultEndpoint = "https://generativelanguage.googleapis.com"
	// DefaultModel is Google's recommended default text model.
	DefaultModel = "gemini-4-hydrogen"
	// Timeout bounds one whole Generate call, retry included.
	Timeout = 40 * time.Second
	// DefaultThinkingLevel is the thinking level of a conversation. Gemini 4
	// cannot turn thinking off; low is its lowest level.
	DefaultThinkingLevel = "low"

	maxResponseBytes = 1 << 20
	apiKeyHeader     = "x-goog-api-key"

	// retryWait is the wait before the one retry when the server names none.
	retryWait = 500 * time.Millisecond
	// maxRetryWait drops the retry rather than hold a worker on a long wait.
	maxRetryWait = 2 * time.Second
)

var (
	// ErrNotConfigured means the client has no API key.
	ErrNotConfigured = errors.New("gemini: no API key configured")
	// ErrInvalidModel means the model name is not a plain model id.
	ErrInvalidModel = errors.New("gemini: invalid model name")
	// ErrNoReply is wrapped by every answer that carries no usable text.
	ErrNoReply = errors.New("gemini: no reply")
)

var modelPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// ValidModel reports whether name is a plain model id: 1 to 64 letters,
// digits, dots, underscores or hyphens, the first a letter or digit.
func ValidModel(name string) bool {
	return modelPattern.MatchString(name)
}

// Options changes the request for the spike tool's comparisons. Its zero value
// is the production request; production code leaves Client.Options zero.
type Options struct {
	// ThinkingLevel is "" for DefaultThinkingLevel, else the value sent.
	ThinkingLevel string
}

// Client calls the Gemini generateContent API. It holds no state besides its
// fields and is safe for concurrent use.
type Client struct {
	// Endpoint is the API host, without a trailing path. Empty means
	// DefaultEndpoint; tests point it at an httptest server.
	Endpoint string
	// APIKey is sent in the x-goog-api-key header only.
	APIKey string
	// Model is the model id, for example DefaultModel.
	Model string
	// HTTP is the transport. Nil uses a client with Timeout that follows no
	// redirects.
	HTTP *http.Client
	// Options is for the spike tool; leave it zero in production.
	Options Options
}

var defaultHTTP = &http.Client{
	Timeout: Timeout,
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return defaultHTTP
}

func (c *Client) baseURL() string {
	if c.Endpoint == "" {
		return DefaultEndpoint
	}
	return strings.TrimRight(c.Endpoint, "/")
}

// Generate sends one conversation turn and returns the reply. A blocked prompt
// or a safety stop is a Reply with OutcomeRefused and no error; every answer
// without usable text is an error wrapping ErrNoReply; an HTTP failure is an
// *HTTPError.
func (c *Client) Generate(ctx context.Context, in Request) (*Reply, error) {
	if c.APIKey == "" {
		return nil, ErrNotConfigured
	}
	if !ValidModel(c.Model) {
		return nil, ErrInvalidModel
	}
	if in.MaxTokens <= 0 || strings.TrimSpace(in.Static) == "" || strings.TrimSpace(in.User) == "" {
		return nil, errors.New("gemini: incomplete request")
	}

	payload, err := json.Marshal(buildBody(in, c.Options))
	if err != nil {
		return nil, errors.New("gemini: encode request")
	}

	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()

	target := c.baseURL() + "/v1beta/models/" + url.PathEscape(c.Model) + ":generateContent"
	body, err := c.exec(ctx, target, payload)
	if err != nil {
		return nil, err
	}
	return parseReply(body)
}

// exec makes the request and, for a retryable failure, one more attempt when
// the wait is at most maxRetryWait. The wait is the server's retry delay, else
// retryWait. Any other failure is final.
func (c *Client) exec(ctx context.Context, target string, payload []byte) ([]byte, error) {
	body, err := c.send(ctx, target, payload)
	if err == nil {
		return body, nil
	}
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || !httpErr.Retryable() {
		return nil, err
	}
	wait := httpErr.RetryDelay
	if wait <= 0 {
		wait = retryWait
	}
	if wait > maxRetryWait || !sleepContext(ctx, wait) {
		return nil, err
	}
	return c.send(ctx, target, payload)
}

func sleepContext(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// send makes one request and returns the body of a 2xx answer.
func (c *Client) send(ctx context.Context, target string, payload []byte) ([]byte, error) {
	// The request error is not wrapped: it would carry the URL.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(payload))
	if err != nil {
		return nil, errors.New("gemini: build request")
	}
	req.Header.Set(apiKeyHeader, c.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, c.transportError(err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes))
		_ = resp.Body.Close()
	}()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, c.transportError(err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, newHTTPError(resp.StatusCode, resp.Header, data, c.APIKey)
	}
	return data, nil
}

// transportError rebuilds a failed request's error without the URL that
// *url.Error carries.
func (c *Client) transportError(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) && ue.Err != nil {
		err = ue.Err
	}
	if c.APIKey != "" && strings.Contains(err.Error(), c.APIKey) {
		return errors.New("gemini: request failed")
	}
	return fmt.Errorf("gemini: request failed: %w", err)
}
