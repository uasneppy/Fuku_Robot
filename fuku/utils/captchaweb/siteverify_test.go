//go:build testtools

package captchaweb

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const (
	siteverifyTestHost  = "captcha.example.com"
	siteverifyTestCData = "cdata-of-this-challenge"
)

func siteverifyWant() Expect {
	return Expect{Hostname: siteverifyTestHost, Action: TurnstileAction, CData: siteverifyTestCData}
}

// successBody is a siteverify success echoing the given fields.
func successBody(hostname, action, cdata string) string {
	body, _ := json.Marshal(map[string]any{
		"success":  true,
		"hostname": hostname,
		"action":   action,
		"cdata":    cdata,
	})
	return string(body)
}

// failureBody is a siteverify failure carrying the given error codes.
func failureBody(codes ...string) string {
	body, _ := json.Marshal(map[string]any{"success": false, "error-codes": codes})
	return string(body)
}

// TestSiteverifyOutcome pins D-28: a member is failed only when the answer is provably about
// their token and nothing else; every other answer is a retry.
func TestSiteverifyOutcome(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		body       string
		wantOut    Outcome
		wantReason Reason
		wantConfig bool
	}{
		{"full match passes", 200, successBody(siteverifyTestHost, TurnstileAction, siteverifyTestCData), OutcomePass, ReasonOK, false},
		{"hostname case is ignored", 200, successBody("Captcha.Example.COM", TurnstileAction, siteverifyTestCData), OutcomePass, ReasonOK, false},
		{"another cdata fails", 200, successBody(siteverifyTestHost, TurnstileAction, "cdata-of-another-challenge"), OutcomeFailed, ReasonCData, false},
		{"another hostname is the owner's fault", 200, successBody("evil.example.net", TurnstileAction, siteverifyTestCData), OutcomeRetry, ReasonHostname, true},
		{"another action is the owner's fault", 200, successBody(siteverifyTestHost, "login", siteverifyTestCData), OutcomeRetry, ReasonAction, true},
		{"another hostname wins over another cdata", 200, successBody("evil.example.net", TurnstileAction, "other"), OutcomeRetry, ReasonHostname, true},
		{"invalid-input-response fails", 200, failureBody("invalid-input-response"), OutcomeFailed, ReasonInvalidToken, false},
		{"timeout-or-duplicate fails", 200, failureBody("timeout-or-duplicate"), OutcomeFailed, ReasonDuplicate, false},
		{"both failing codes fail", 200, failureBody("invalid-input-response", "timeout-or-duplicate"), OutcomeFailed, ReasonInvalidToken, false},
		{"a secret error beside a failing code is a retry", 200, failureBody("invalid-input-response", "invalid-input-secret"), OutcomeRetry, ReasonSecret, true},
		{"missing-input-secret is a retry", 200, failureBody("missing-input-secret"), OutcomeRetry, ReasonSecret, true},
		{"invalid-input-secret is a retry", 200, failureBody("invalid-input-secret"), OutcomeRetry, ReasonSecret, true},
		{"internal-error is a retry", 200, failureBody("internal-error"), OutcomeRetry, ReasonInternal, false},
		{"bad-request is a retry", 200, failureBody("bad-request"), OutcomeRetry, ReasonBadRequest, false},
		{"missing-input-response is a retry", 200, failureBody("missing-input-response"), OutcomeRetry, ReasonMissingResponse, false},
		{"an unknown code is a retry", 200, failureBody("something-new"), OutcomeRetry, ReasonUnknown, false},
		{"a failing code beside an unknown one is a retry", 200, failureBody("invalid-input-response", "something-new"), OutcomeRetry, ReasonUnknown, false},
		{"no code at all is a retry", 200, `{"success":false}`, OutcomeRetry, ReasonUnknown, false},
		{"an empty code list is a retry", 200, `{"success":false,"error-codes":[]}`, OutcomeRetry, ReasonUnknown, false},
		{"a 500 is a retry", 500, failureBody("invalid-input-response"), OutcomeRetry, ReasonStatus, false},
		{"a 400 with a body is classified by the body", 400, failureBody("invalid-input-response"), OutcomeFailed, ReasonInvalidToken, false},
		{"a 400 success is never a pass", 400, successBody(siteverifyTestHost, TurnstileAction, siteverifyTestCData), OutcomeRetry, ReasonStatus, false},
		{"a body that is not JSON is a retry", 200, `<html>gateway</html>`, OutcomeRetry, ReasonDecode, false},
		{"an empty body is a retry", 200, ``, OutcomeRetry, ReasonDecode, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := SiteverifyServerForTest(t, SiteverifyAnswerForTest(tc.status, tc.body))
			got := client.Verify(context.Background(), "token", siteverifyWant())
			if got.Outcome != tc.wantOut || got.Reason != tc.wantReason {
				t.Fatalf("Verify() = %+v, want outcome %d reason %q", got, tc.wantOut, tc.wantReason)
			}
			if got.ConfigError() != tc.wantConfig {
				t.Fatalf("ConfigError() = %v, want %v for %+v", got.ConfigError(), tc.wantConfig, got)
			}
		})
	}

	t.Run("a redirect is a retry and is never followed", func(t *testing.T) {
		var secondCalls atomic.Int64
		second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			secondCalls.Add(1)
			_, _ = w.Write([]byte(successBody(siteverifyTestHost, TurnstileAction, siteverifyTestCData)))
		}))
		t.Cleanup(second.Close)
		client := SiteverifyServerForTest(t, func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, second.URL, http.StatusFound)
		})
		// The transport of SiteverifyServerForTest follows redirects by default: the client
		// itself must refuse them, whatever transport it was given.
		got := client.Verify(context.Background(), "token", siteverifyWant())
		if got.Outcome != OutcomeRetry || got.Reason != ReasonStatus {
			t.Fatalf("Verify() = %+v, want retry/status", got)
		}
		if secondCalls.Load() != 0 {
			t.Fatalf("the redirect target was called %d times, want never", secondCalls.Load())
		}

		client.HTTP = nil
		if got := client.Verify(context.Background(), "token", siteverifyWant()); got.Outcome != OutcomeRetry {
			t.Fatalf("Verify() with the default transport = %+v, want retry", got)
		}
		if secondCalls.Load() != 0 {
			t.Fatalf("the redirect target was called %d times with the default transport, want never", secondCalls.Load())
		}
	})

	t.Run("a body over 64 KiB is a retry", func(t *testing.T) {
		pad := strings.Repeat("x", 70<<10)
		body := `{"success":true,"hostname":"` + siteverifyTestHost + `","action":"` + TurnstileAction +
			`","cdata":"` + siteverifyTestCData + `","pad":"` + pad + `"}`
		client := SiteverifyServerForTest(t, SiteverifyAnswerForTest(200, body))
		got := client.Verify(context.Background(), "token", siteverifyWant())
		if got.Outcome != OutcomeRetry || got.Reason != ReasonDecode {
			t.Fatalf("Verify() = %+v, want retry/decode", got)
		}
	})

	t.Run("a server slower than the client timeout is a retry", func(t *testing.T) {
		client := SiteverifyServerForTest(t, func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-time.After(500 * time.Millisecond):
			case <-r.Context().Done():
			}
			_, _ = w.Write([]byte(successBody(siteverifyTestHost, TurnstileAction, siteverifyTestCData)))
		})
		client.HTTP = &http.Client{Timeout: 60 * time.Millisecond}
		got := client.Verify(context.Background(), "token", siteverifyWant())
		if got.Outcome != OutcomeRetry || got.Reason != ReasonTransport {
			t.Fatalf("Verify() = %+v, want retry/transport", got)
		}
	})

	t.Run("a closed connection is a retry", func(t *testing.T) {
		client := SiteverifyServerForTest(t, func(w http.ResponseWriter, _ *http.Request) {
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Error("the test server cannot hijack")
				return
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				t.Errorf("hijack: %v", err)
				return
			}
			_ = conn.Close()
		})
		got := client.Verify(context.Background(), "token", siteverifyWant())
		if got.Outcome != OutcomeRetry || got.Reason != ReasonTransport {
			t.Fatalf("Verify() = %+v, want retry/transport", got)
		}
	})
}

// TestSiteverifyRequestShape pins what Cloudflare receives: one JSON POST with the secret and
// the token and nothing else (no remoteip, no idempotency_key).
func TestSiteverifyRequestShape(t *testing.T) {
	var (
		method      string
		contentType string
		raw         []byte
	)
	client := SiteverifyServerForTest(t, func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		contentType = r.Header.Get("Content-Type")
		raw, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(failureBody("invalid-input-response")))
	})
	client.Verify(context.Background(), "the-turnstile-token", siteverifyWant())

	if method != http.MethodPost {
		t.Fatalf("method = %s, want POST", method)
	}
	if contentType != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", contentType)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("request body %q is not JSON: %v", raw, err)
	}
	if len(fields) != 2 || fields["secret"] != client.Secret || fields["response"] != "the-turnstile-token" {
		t.Fatalf("request fields = %v, want exactly secret and response", fields)
	}
}
