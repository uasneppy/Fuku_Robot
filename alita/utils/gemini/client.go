// Package gemini is the Gemini generateContent client of the AI raid checks.
//
// It imports the standard library only, so the scripts/gemini-spike tool can
// use it without pulling in alita/config or alita/db, whose init functions
// connect to PostgreSQL before main. The API key travels only in the
// x-goog-api-key header, never in a URL, and nothing in this package logs:
// callers log, and every error returned here is free of the key.
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
	// DefaultModel is the model used when GEMINI_MODEL is not set.
	DefaultModel = "gemini-3.5-flash-lite"
	// Timeout bounds one whole Classify or GetModel call, retry included.
	Timeout = 8 * time.Second
	// MaxImageBytes is the largest image sent inline.
	MaxImageBytes = 5 << 20

	maxResponseBytes = 1 << 20
	apiKeyHeader     = "x-goog-api-key"

	// retryWait is the wait before the one retry when the server names none.
	retryWait = 500 * time.Millisecond
	// maxRetryWait drops the retry rather than hold a worker on a long
	// backoff. A shed check is "not checked", which is the safe side.
	maxRetryWait = 2 * time.Second
)

var (
	// ErrNotConfigured means the client has no API key.
	ErrNotConfigured = errors.New("gemini: no API key configured")
	// ErrInvalidModel means the model name is not a plain model id.
	ErrInvalidModel = errors.New("gemini: invalid model name")
	// ErrUnsupportedImage means the image type or size cannot be sent.
	ErrUnsupportedImage = errors.New("gemini: unsupported image")
)

var modelPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// ValidModel reports whether name is safe to put in the request path: 1 to 64
// letters, digits, dots, underscores or hyphens.
func ValidModel(name string) bool {
	return modelPattern.MatchString(name)
}

// Options changes the request for the spike tool's comparisons. Its zero value
// is the production request.
type Options struct {
	// SchemaField picks the structured-output spelling: "" or
	// "responseJsonSchema" (default), "responseSchema" or "responseFormat".
	SchemaField string
	// ThinkingLevel is empty for MINIMAL on gemini-3 models and nothing on
	// others, "off" to send none, or a level name to send as given.
	ThinkingLevel string
	// OmitTemperature leaves the temperature out instead of sending 0.
	OmitTemperature bool
	// MediaResolution is empty for MEDIA_RESOLUTION_MEDIUM.
	MediaResolution string
	// SafetyThreshold, when set, adds safetySettings with this threshold for
	// the four adjustable harm categories. Production sends none.
	SafetyThreshold string
	// Store, when not nil, is sent as the request's store flag.
	Store *bool
}

// Image is one picture sent inline.
type Image struct {
	// MIMEType is image/jpeg, image/png or image/webp.
	MIMEType string
	Data     []byte
}

// Client calls the Gemini API. Build one per use; it holds no state besides
// its fields and is safe for concurrent use.
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

func (c *Client) modelURL(suffix string) string {
	return c.baseURL() + "/v1beta/models/" + url.PathEscape(c.Model) + suffix
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
// failure is an error and no verdict: a block, a truncated answer, an
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
		return nil, errors.New("gemini: encode request")
	}

	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()

	body, err := c.exec(ctx, http.MethodPost, c.modelURL(":generateContent"), payload)
	if err != nil {
		return nil, err
	}
	return parseVerdict(body)
}

// ModelInfo is the part of models.get the spike tool reports.
type ModelInfo struct {
	Name                       string   `json:"name"`
	DisplayName                string   `json:"displayName"`
	InputTokenLimit            int      `json:"inputTokenLimit"`
	OutputTokenLimit           int      `json:"outputTokenLimit"`
	SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
}

// GetModel reads the configured model's metadata (models.get). The spike tool
// uses it to check that the model exists and takes images; the bot does not.
func (c *Client) GetModel(ctx context.Context) (*ModelInfo, error) {
	if c.APIKey == "" {
		return nil, ErrNotConfigured
	}
	if !ValidModel(c.Model) {
		return nil, ErrInvalidModel
	}

	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()

	body, err := c.exec(ctx, http.MethodGet, c.modelURL(""), nil)
	if err != nil {
		return nil, err
	}
	var info ModelInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, errors.New("gemini: unreadable model metadata")
	}
	return &info, nil
}

// exec makes the request and, for a 429 or 5xx, one more attempt when the wait
// is at most maxRetryWait. The wait is the server's retryDelay or Retry-After,
// else retryWait. Any other failure is final.
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
		return nil, errors.New("gemini: build request")
	}
	req.Header.Set(apiKeyHeader, c.APIKey)
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
		return errors.New("gemini: request failed")
	}
	return fmt.Errorf("gemini: request failed: %w", err)
}
