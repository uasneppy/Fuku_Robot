//go:build testtools

package captchaweb

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// rebuild replaces the handler of o with one built from the same dependencies and the options
// mutate leaves, so a test can change the trusted proxies, the readiness or the backend.
func (o *outcomeHandler) rebuild(mutate func(*Options)) {
	o.t.Helper()
	opts := Options{
		BotToken:      outcomeBotToken,
		PublicBaseURL: "https://" + outcomeHost,
		Store:         o.Store,
		Siteverify:    o.Client,
		Backend:       o.Backend,
	}
	if mutate != nil {
		mutate(&opts)
	}
	handler, err := NewHandler(opts)
	if err != nil {
		o.t.Fatalf("NewHandler() error = %v", err)
	}
	o.Handler = handler
}

// serve drives the handler's Verify with a request from remote and returns the recorder.
func (o *outcomeHandler) serve(remote, contentType, body string, extra http.Header) *httptest.ResponseRecorder {
	o.t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/captcha/verify", strings.NewReader(body))
	req.RemoteAddr = remote
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for k, values := range extra {
		for _, v := range values {
			req.Header.Add(k, v)
		}
	}
	rec := httptest.NewRecorder()
	o.Handler.Verify(rec, req)
	return rec
}

// answerOf decodes the JSON answer of rec.
func answerOf(t *testing.T, rec *httptest.ResponseRecorder) Answer {
	t.Helper()
	var answer Answer
	if err := json.Unmarshal(rec.Body.Bytes(), &answer); err != nil {
		t.Fatalf("answer %q is not JSON: %v", rec.Body.String(), err)
	}
	return answer
}

// postFrom posts what a real page posts for userID and token from remote.
func (o *outcomeHandler) postFrom(remote string, userID int64, token string) *httptest.ResponseRecorder {
	o.t.Helper()
	return o.serve(remote, "application/json", o.body(o.initData(userID, token), TurnstileTokenForTest(token), token), nil)
}

func TestVerifyRouteWrongUserChangesNothing(t *testing.T) {
	const (
		target  = int64(42)
		griefer = int64(77)
	)
	o := newOutcomeHandler(t)
	token, _ := o.issue(target)
	before := o.Redis.HGet(o.key(token), "u")
	if before != strconv.FormatInt(target, 10) {
		t.Fatalf("challenge owner = %q before the attack", before)
	}

	notForYou, limited := 0, 0
	for i := 0; i < 100; i++ {
		rec := o.postFrom("198.51.100.20:5555", griefer, token)
		switch rec.Code {
		case http.StatusOK:
			if got := answerOf(t, rec).Result; got != ResultNotForYou {
				t.Fatalf("attempt %d: result = %q, want %q", i+1, got, ResultNotForYou)
			}
			notForYou++
		case http.StatusTooManyRequests:
			if got := answerOf(t, rec).Result; got != ResultRetry {
				t.Fatalf("attempt %d: 429 result = %q, want %q", i+1, got, ResultRetry)
			}
			if rec.Header().Get("Retry-After") == "" {
				t.Fatalf("attempt %d: 429 without Retry-After", i+1)
			}
			limited++
		default:
			t.Fatalf("attempt %d: status = %d", i+1, rec.Code)
		}
	}
	if notForYou == 0 || limited == 0 {
		t.Fatalf("notForYou=%d limited=%d, want both non-zero", notForYou, limited)
	}
	if notForYou > verifyTokenUserLimit {
		t.Fatalf("griefer got %d answers, the token window is %d", notForYou, verifyTokenUserLimit)
	}

	if calls := o.Backend.Calls(); len(calls) != 0 {
		t.Fatalf("backend called for the griefer: %v", calls)
	}
	if got := o.Redis.HGet(o.key(token), "u"); got != before {
		t.Fatalf("challenge changed by the griefer: owner %q, want %q", got, before)
	}

	rec := o.postFrom("203.0.113.5:4000", target, token)
	if rec.Code != http.StatusOK || answerOf(t, rec).Result != ResultPassed {
		t.Fatalf("target verify = %d %q, want 200 passed", rec.Code, rec.Body.String())
	}
}

func TestVerifyRouteLimitsAndHeaders(t *testing.T) {
	t.Run("windows", func(t *testing.T) {
		t.Run("per public address", func(t *testing.T) {
			o := newOutcomeHandler(t)
			for i := 1; i <= verifyIPLimit; i++ {
				rec := o.serve("203.0.113.9:4000", "application/json", "{}", nil)
				if rec.Code != http.StatusBadRequest {
					t.Fatalf("request %d: status = %d, want 400", i, rec.Code)
				}
			}
			rec := o.serve("203.0.113.9:4000", "application/json", "{}", nil)
			if rec.Code != http.StatusTooManyRequests || answerOf(t, rec).Result != ResultRetry {
				t.Fatalf("request %d: %d %q, want 429 retry", verifyIPLimit+1, rec.Code, rec.Body.String())
			}
			if secs, err := strconv.Atoi(rec.Header().Get("Retry-After")); err != nil || secs < 1 || secs > 60 {
				t.Fatalf("Retry-After = %q, want whole seconds in 1..60", rec.Header().Get("Retry-After"))
			}
			other := o.serve("203.0.113.10:4000", "application/json", "{}", nil)
			if other.Code != http.StatusBadRequest {
				t.Fatalf("another address: status = %d, want 400", other.Code)
			}
		})

		t.Run("per verified user", func(t *testing.T) {
			o := newOutcomeHandler(t)
			const member = int64(42)
			for i := 1; i <= verifyUserLimit; i++ {
				token, err := NewToken()
				if err != nil {
					t.Fatal(err)
				}
				rec := o.postFrom(fmt.Sprintf("198.51.100.%d:4000", i), member, token)
				if rec.Code != http.StatusOK || answerOf(t, rec).Result != ResultInvalid {
					t.Fatalf("request %d: %d %q, want 200 invalid", i, rec.Code, rec.Body.String())
				}
			}
			token, _ := NewToken()
			rec := o.postFrom("198.51.100.200:4000", member, token)
			if rec.Code != http.StatusTooManyRequests || answerOf(t, rec).Result != ResultRetry || rec.Header().Get("Retry-After") == "" {
				t.Fatalf("request %d: %d %q, want 429 retry with Retry-After", verifyUserLimit+1, rec.Code, rec.Body.String())
			}
		})

		t.Run("per token and verified user", func(t *testing.T) {
			o := newOutcomeHandler(t)
			const member = int64(42)
			token, err := NewToken()
			if err != nil {
				t.Fatal(err)
			}
			for i := 1; i <= verifyTokenUserLimit; i++ {
				rec := o.postFrom(fmt.Sprintf("198.51.100.%d:4000", i), member, token)
				if rec.Code != http.StatusOK || answerOf(t, rec).Result != ResultInvalid {
					t.Fatalf("request %d: %d %q, want 200 invalid", i, rec.Code, rec.Body.String())
				}
			}
			rec := o.postFrom("198.51.100.200:4000", member, token)
			if rec.Code != http.StatusTooManyRequests || answerOf(t, rec).Result != ResultRetry || rec.Header().Get("Retry-After") == "" {
				t.Fatalf("request %d: %d %q, want 429 retry with Retry-After", verifyTokenUserLimit+1, rec.Code, rec.Body.String())
			}
			// Another token of the same member is a different window.
			other, _ := NewToken()
			if rec := o.postFrom("198.51.100.201:4000", member, other); rec.Code != http.StatusOK {
				t.Fatalf("another token: status = %d, want 200", rec.Code)
			}
		})

		t.Run("a private peer without a trusted list is not address-limited", func(t *testing.T) {
			o := newOutcomeHandler(t)
			for i := 0; i < verifyIPLimit+40; i++ {
				rec := o.serve("10.1.2.3:4000", "application/json", "{}", nil)
				if rec.Code != http.StatusBadRequest {
					t.Fatalf("request %d: status = %d, want 400", i+1, rec.Code)
				}
			}
		})

		t.Run("behind a trusted proxy the forwarded client is the bucket", func(t *testing.T) {
			o := newOutcomeHandler(t)
			trusted, err := ParseTrustedProxies("10.0.0.0/8")
			if err != nil {
				t.Fatal(err)
			}
			o.rebuild(func(opts *Options) { opts.TrustedProxies = trusted })
			forwarded := http.Header{"X-Forwarded-For": {"198.51.100.7"}}
			for i := 1; i <= verifyIPLimit; i++ {
				if rec := o.serve("10.0.0.5:4000", "application/json", "{}", forwarded); rec.Code != http.StatusBadRequest {
					t.Fatalf("request %d: status = %d, want 400", i, rec.Code)
				}
			}
			if rec := o.serve("10.0.0.5:4000", "application/json", "{}", forwarded); rec.Code != http.StatusTooManyRequests {
				t.Fatalf("request %d: status = %d, want 429", verifyIPLimit+1, rec.Code)
			}
			other := http.Header{"X-Forwarded-For": {"198.51.100.8"}}
			if rec := o.serve("10.0.0.5:4000", "application/json", "{}", other); rec.Code != http.StatusBadRequest {
				t.Fatalf("another client behind the proxy: status = %d, want 400", rec.Code)
			}
		})

		t.Run("a Redis error refuses instead of allowing", func(t *testing.T) {
			o := newOutcomeHandler(t)
			o.Redis.Close()
			rec := o.serve("203.0.113.9:4000", "application/json", "{}", nil)
			if rec.Code != http.StatusServiceUnavailable || answerOf(t, rec).Result != ResultRetry {
				t.Fatalf("closed redis: %d %q, want 503 retry", rec.Code, rec.Body.String())
			}
		})
	})
}
