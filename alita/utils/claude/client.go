// Package claude is the Anthropic Messages client of the raid AI image checks.
//
// It imports the standard library only, so a spike tool can use it without
// pulling in alita/config or alita/db, whose init functions connect to
// PostgreSQL before main. The API key travels only in the x-api-key header,
// never in a URL, and nothing in this package logs: callers log, and every
// error returned here is free of the key.
package claude

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
	// DefaultEndpoint is the Anthropic API host.
	DefaultEndpoint = "https://api.anthropic.com"
	// DefaultModel is the model used when ANTHROPIC_MODEL is not set.
	DefaultModel = "claude-haiku-5-5"
	// APIVersion is the anthropic-version header value.
	APIVersion = "2026-10-21"
	// Timeout bounds one whole Classify call, retry included.
	Timeout = 8 * time.Second
	// MaxImageBytes is the largest image sent inline.
	MaxImageBytes = 5 << 20

	maxResponseBytes = 1 << 20
	apiKeyHeader     = "x-api-key"
	versionHeader    = "anthropic-version"

	// retryWait is the wait before the one retry when the server names none.
	retryWait = 500 * time.Millisecond
	// maxRetryWait drops the retry rather than hold a worker on a long
	// backoff. A shed check is "not checked", which is the safe side.
	maxRetryWait = 2 * time.Second
)

var (
	// ErrNotConfigured means the client has no API key.
	ErrNotConfigured = errors.New("claude: no API key configured")
	// ErrInvalidModel means the model name is not a plain model id.
	ErrInvalidModel = errors.New("claude: invalid model name")
	// ErrUnsupportedImage means the image type or size cannot be sent.
	ErrUnsupportedImage = errors.New("claude: unsupported image")
)

var modelPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// ValidModel reports whether name is a plain model id: 1 to 64 letters,
// digits, dots, underscores or hyphens, the first a letter or digit.
func ValidModel(name string) bool {
	return modelPattern.MatchString(name)
}

// Image is one picture sent inline.
type Image struct {
	// MIMEType is image/jpeg, image/png or image/webp.
	MIMEType string
	Data     []byte
}

// Client calls the Anthropic Messages API. Build one per use; it holds no
// state besides its fields and is safe for concurrent use.
type Client struct {
	// Endpoint is the API host, without a trailing path. Empty means
	// DefaultEndpoint; tests point it at an httptest server.
	Endpoint string
	// APIKey is sent in the x-api-key header only.
	APIKey string
	// Model is the model id, for example DefaultModel.
	Model string
	// HTTP is the transport. Nil uses a client with Timeout that follows no
	// redirects.
	HTTP *http.Client
	// Options is for the spike tool; leave it zero in production.
	Options Options
}

// defaultHTTP follows no redirect: an answer is never read from another host.
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

func imageSupported(img Image) bool {
	switch img.MIMEType {
	case "image/jpeg", "image/png", "image/webp":
	default:
		return false
	}
	return len(img.Data) > 0 && len(img.Data) <= MaxImageBytes
}

// Classify sends one image and returns the model's spam probability. Every
// failure is an error and no verdict: a refusal, a truncated answer, an
// unreadable or out-of-range number all wrap ErrNoVerdict.
func (c *Client) Classify(ctx context.Context, img Image) (*Verdict, error) {
	if c.APIKey == "" {
		return nil, ErrNotConfigured
	}
	if !ValidModel(c.Model) {
		return nil, ErrInvalidModel
	}
	if !imageSupported(img) {
		return nil, ErrUnsupportedImage
	}

	payload, err := json.Marshal(buildRequest(c.Model, c.Options, img))
	if err != nil {
		return nil, errors.New("claude: encode request")
	}

	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()

	body, err := c.exec(ctx, http.MethodPost, c.baseURL()+"/v1/messages", payload)
	if err != nil {
		return nil, err
	}
	return parseVerdict(body)
}

// ModelInfo is the metadata the models endpoint reports for a model. The spike
// tool prints it so the owner can check the model id and its limits.
type ModelInfo struct {
	ID             string `json:"id"`
	DisplayName    string `json:"display_name"`
	MaxInputTokens int    `json:"max_input_tokens"`
	MaxTokens      int    `json:"max_tokens"`
}

// GetModel reads the configured model's metadata (GET /v1/models/{id}). The
// spike tool uses it to check the model id before a run.
func (c *Client) GetModel(ctx context.Context) (*ModelInfo, error) {
	if c.APIKey == "" {
		return nil, ErrNotConfigured
	}
	if !ValidModel(c.Model) {
		return nil, ErrInvalidModel
	}

	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()

	body, err := c.exec(ctx, http.MethodGet, c.baseURL()+"/v1/models/"+url.PathEscape(c.Model), nil)
	if err != nil {
		return nil, err
	}
	var info ModelInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, errors.New("claude: unreadable model metadata")
	}
	return &info, nil
}

// exec makes the request and, for a 429 or 5xx, one more attempt when the wait
// is at most maxRetryWait. The wait is the server's retry-after, else
// retryWait. Any other failure is final.
func (c *Client) exec(ctx context.Context, method, target string, payload []byte) ([]byte, error) {
	body, err := c.send(ctx, method, target, payload)
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
	return c.send(ctx, method, target, payload)
}

// sleepContext waits, reporting false if the context ended first.
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
func (c *Client) send(ctx context.Context, method, target string, payload []byte) ([]byte, error) {
	var reader io.Reader
	if payload != nil {
		reader = bytes.NewReader(payload)
	}
	// The request error is not wrapped: it would carry the URL.
	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return nil, errors.New("claude: build request")
	}
	req.Header.Set(apiKeyHeader, c.APIKey)
	req.Header.Set(versionHeader, APIVersion)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

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
		return errors.New("claude: request failed")
	}
	return fmt.Errorf("claude: request failed: %w", err)
}
