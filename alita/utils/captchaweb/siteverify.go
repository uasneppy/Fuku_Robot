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
	// HTTP is the transport. Nil uses a client with SiteverifyTimeout. Whatever the
	// transport, a redirect is never followed.
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

// httpClient returns the transport with redirects refused whatever it was given: the secret
// is in the request body and an answer is never read from another host.
func (c *Client) httpClient() *http.Client {
	if c.HTTP == nil {
		return defaultSiteverifyHTTP
	}
	hc := *c.HTTP
	hc.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &hc
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
	// member's: a Cloudflare, network or configuration fault. It is the zero value, so an
	// unset Verdict can never kick anyone.
	OutcomeRetry Outcome = iota
	// OutcomePass means Cloudflare confirmed the token for the expected hostname, action
	// and cData.
	OutcomePass
	// OutcomeFailed means the answer is provably about this member's token: Cloudflare said
	// it is invalid or already used, or confirmed a token minted for another challenge.
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
	ReasonHostname  Reason = "hostname"
	ReasonAction    Reason = "action"
	ReasonCData     Reason = "cdata"

	// The reasons of a failed siteverify, one per Cloudflare error code (D-28).
	ReasonInvalidToken    Reason = "invalid_token"    // invalid-input-response
	ReasonDuplicate       Reason = "duplicate"        // timeout-or-duplicate
	ReasonSecret          Reason = "secret"           // missing-input-secret, invalid-input-secret
	ReasonInternal        Reason = "internal"         // internal-error
	ReasonBadRequest      Reason = "bad_request"      // bad-request
	ReasonMissingResponse Reason = "missing_response" // missing-input-response
	ReasonUnknown         Reason = "unknown"          // any other code, or none
)

// Verdict is the result of one Verify call.
type Verdict struct {
	Outcome Outcome
	Reason  Reason
}

// ConfigError reports whether the verdict is the owner's configuration fault: a rejected
// secret, or an answer for another hostname or action. The handler logs those at Error level
// by reason name only; the member is never blamed for them.
func (v Verdict) ConfigError() bool {
	switch v.Reason {
	case ReasonSecret, ReasonHostname, ReasonAction:
		return true
	}
	return false
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

// Verify asks Cloudflare whether token is a valid solve and classifies the answer (D-28).
// It posts exactly secret and response (never remoteip or idempotency_key).
//
// A member is failed (OutcomeFailed) only when the answer is provably about their token and
// nothing else: success false whose error codes are all invalid-input-response or
// timeout-or-duplicate, or a success that echoes another challenge's cdata. A success for
// another hostname or action, any other error code, an empty code list, a transport error, a
// status of 500 or more, a redirect or a body it cannot decode is OutcomeRetry with its
// Reason, because none of them is the member's fault.
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
	// still carries a JSON answer that is classified like any other, but it is never a pass.
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

	if !answer.Success {
		return classifyFailure(answer.ErrorCodes)
	}
	switch {
	case resp.StatusCode >= 400:
		return Verdict{OutcomeRetry, ReasonStatus}
	case !strings.EqualFold(answer.Hostname, want.Hostname):
		return Verdict{OutcomeRetry, ReasonHostname}
	case answer.Action != want.Action:
		return Verdict{OutcomeRetry, ReasonAction}
	case answer.CData != want.CData:
		// Cloudflare confirmed a real token, but one minted for another challenge: replayed
		// or forged for this one, so it is this member's failure.
		return Verdict{OutcomeFailed, ReasonCData}
	}
	return Verdict{OutcomePass, ReasonOK}
}

// classifyFailure maps the error codes of a success=false answer (D-28). Only a non-empty
// list made entirely of invalid-input-response and timeout-or-duplicate fails the member.
// Any other code makes it a retry, with the reason of the highest-priority code present:
// secret, internal-error, bad-request, missing-input-response, then anything else.
func classifyFailure(codes []string) Verdict {
	if len(codes) == 0 {
		return Verdict{OutcomeRetry, ReasonUnknown}
	}
	var first Reason
	provable := true
	has := map[Reason]bool{}
	for _, code := range codes {
		reason := ReasonUnknown
		switch code {
		case "invalid-input-response":
			reason = ReasonInvalidToken
		case "timeout-or-duplicate":
			reason = ReasonDuplicate
		case "missing-input-secret", "invalid-input-secret":
			reason = ReasonSecret
		case "internal-error":
			reason = ReasonInternal
		case "bad-request":
			reason = ReasonBadRequest
		case "missing-input-response":
			reason = ReasonMissingResponse
		}
		if first == "" {
			first = reason
		}
		has[reason] = true
		if reason != ReasonInvalidToken && reason != ReasonDuplicate {
			provable = false
		}
	}
	if provable {
		return Verdict{OutcomeFailed, first}
	}
	for _, reason := range []Reason{ReasonSecret, ReasonInternal, ReasonBadRequest, ReasonMissingResponse} {
		if has[reason] {
			return Verdict{OutcomeRetry, reason}
		}
	}
	return Verdict{OutcomeRetry, ReasonUnknown}
}
