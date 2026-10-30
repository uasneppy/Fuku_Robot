package captchaweb

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	// DefaultSiteverifyEndpoint is Cloudflare's Turnstile verification endpoint.
	DefaultSiteverifyEndpoint = "https://challenges.cloudflare.com/turnstile/v0/siteverify"
	// SiteverifyTimeout bounds one siteverify call (D-11).
	SiteverifyTimeout = 5 * time.Second

	maxSiteverifyBytes = 64 << 10
)

// Client calls Cloudflare's siteverify. It holds no state besides its fields and is safe
// for concurrent use. The secret travels only in the request body, and no error, request or
// body is ever wrapped or logged.
type Client struct {
	// Endpoint is the siteverify URL. Empty means DefaultSiteverifyEndpoint; tests point it
	// at an httptest server.
	Endpoint string
	// Secret is the Turnstile secret key.
	Secret string
	// HTTP is the transport. Nil uses a client with SiteverifyTimeout that follows no
	// redirect.
	HTTP *http.Client
}

// defaultSiteverifyHTTP follows no redirect: the secret is in the body, and an answer is
// never read from another host.
var defaultSiteverifyHTTP = &http.Client{
	Timeout: SiteverifyTimeout,
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return defaultSiteverifyHTTP
}

func (c *Client) endpoint() string {
	if c.Endpoint == "" {
		return DefaultSiteverifyEndpoint
	}
	return c.Endpoint
}

// Expect is what a pass must echo: the page's hostname, the widget action and the cData.
type Expect struct {
	Hostname string
	Action   string
	CData    string
}

// Outcome is what the server does with a siteverify answer.
type Outcome int

const (
	// OutcomeRetry means the check did not pass for a reason that is not proven to be the
	// member's. It is the zero value. Plan 08-06 adds the classification of a failed solve.
	OutcomeRetry Outcome = iota
	// OutcomePass means Cloudflare confirmed the token for the expected hostname, action
	// and cData.
	OutcomePass
	// OutcomeFailed means the member failed the check.
	OutcomeFailed
)

// Reason says why a Verdict is not a pass. It is for counters and logs only.
type Reason string

// The reasons of a Verdict.
const (
	ReasonOK        Reason = "ok"
	ReasonTransport Reason = "transport"
	ReasonStatus    Reason = "status"
	ReasonDecode    Reason = "decode"
	ReasonRejected  Reason = "rejected"
	ReasonHostname  Reason = "hostname"
	ReasonAction    Reason = "action"
	ReasonCData     Reason = "cdata"
)

// Verdict is the result of one Verify call.
type Verdict struct {
	Outcome Outcome
	Reason  Reason
}

type siteverifyRequest struct {
	Secret   string `json:"secret"`
	Response string `json:"response"`
}

type siteverifyResponse struct {
	Success    bool     `json:"success"`
	Hostname   string   `json:"hostname"`
	Action     string   `json:"action"`
	CData      string   `json:"cdata"`
	ErrorCodes []string `json:"error-codes"`
}

// Verify asks Cloudflare whether token is a valid solve. It posts exactly secret and
// response (never remoteip or idempotency_key). It returns OutcomePass only when success is
// true and hostname, action and cdata equal want; every other answer, a transport error, a
// status of 500 or more, a redirect or a body it cannot decode is OutcomeRetry with its
// Reason.
func (c *Client) Verify(ctx context.Context, token string, want Expect) Verdict {
	ctx, cancel := context.WithTimeout(ctx, SiteverifyTimeout)
	defer cancel()

	body, err := json.Marshal(siteverifyRequest{Secret: c.Secret, Response: token})
	if err != nil {
		return Verdict{OutcomeRetry, ReasonTransport}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(), bytes.NewReader(body))
	if err != nil {
		return Verdict{OutcomeRetry, ReasonTransport}
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return Verdict{OutcomeRetry, ReasonTransport}
	}
	defer func() { _ = resp.Body.Close() }()

	// A 3xx is a redirect we refuse to follow and a 5xx is Cloudflare's trouble. A 4xx
	// still carries a JSON answer, but it is never a pass.
	if resp.StatusCode >= 500 || (resp.StatusCode >= 300 && resp.StatusCode < 400) {
		return Verdict{OutcomeRetry, ReasonStatus}
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxSiteverifyBytes+1))
	if err != nil || len(raw) > maxSiteverifyBytes {
		return Verdict{OutcomeRetry, ReasonDecode}
	}
	var answer siteverifyResponse
	if err := json.Unmarshal(raw, &answer); err != nil {
		return Verdict{OutcomeRetry, ReasonDecode}
	}

	switch {
	case !answer.Success || resp.StatusCode >= 400:
		return Verdict{OutcomeRetry, ReasonRejected}
	case !strings.EqualFold(answer.Hostname, want.Hostname):
		return Verdict{OutcomeRetry, ReasonHostname}
	case answer.Action != want.Action:
		return Verdict{OutcomeRetry, ReasonAction}
	case answer.CData != want.CData:
		return Verdict{OutcomeRetry, ReasonCData}
	}
	return Verdict{OutcomePass, ReasonOK}
}
