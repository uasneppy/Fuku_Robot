//go:build testtools

package captchaweb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
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

	t.Run("request hygiene", func(t *testing.T) {
		const member = int64(42)
		page := func(o *outcomeHandler, token string) string {
			return o.body(o.initData(member, token), TurnstileTokenForTest(token), token)
		}

		t.Run("only JSON is accepted", func(t *testing.T) {
			o := newOutcomeHandler(t)
			token, _ := o.issue(member)
			body := page(o, token)
			for _, contentType := range []string{"text/plain", "application/x-www-form-urlencoded", "multipart/form-data", ""} {
				rec := o.serve("203.0.113.9:4000", contentType, body, nil)
				if rec.Code != http.StatusUnsupportedMediaType || answerOf(t, rec).Result != ResultInvalid {
					t.Fatalf("Content-Type %q: %d %q, want 415 invalid", contentType, rec.Code, rec.Body.String())
				}
			}
			if calls := o.Backend.Calls(); len(calls) != 0 {
				t.Fatalf("a refused content type reached the backend: %v", calls)
			}
			rec := o.serve("203.0.113.9:4000", "application/json; charset=utf-8", body, nil)
			if rec.Code != http.StatusOK || answerOf(t, rec).Result != ResultPassed {
				t.Fatalf("application/json with a charset: %d %q, want 200 passed", rec.Code, rec.Body.String())
			}
		})

		t.Run("the body is capped at 16 KiB", func(t *testing.T) {
			o := newOutcomeHandler(t)
			big := `{"init_data":"` + strings.Repeat("a", 17<<10) + `"}`
			rec := o.serve("203.0.113.9:4000", "application/json", big, nil)
			if rec.Code != http.StatusRequestEntityTooLarge || answerOf(t, rec).Result != ResultInvalid {
				t.Fatalf("17 KiB body: %d %q, want 413 invalid", rec.Code, rec.Body.String())
			}
		})

		t.Run("unknown fields and a second value are invalid", func(t *testing.T) {
			o := newOutcomeHandler(t)
			token, _ := o.issue(member)
			good := page(o, token)
			withExtra := strings.TrimSuffix(good, "}") + `,"extra":1}`
			for name, body := range map[string]string{
				"unknown field": withExtra,
				"second object": good + `{}`,
				"trailing text": good + ` x`,
			} {
				rec := o.serve("203.0.113.9:4000", "application/json", body, nil)
				if rec.Code != http.StatusBadRequest || answerOf(t, rec).Result != ResultInvalid {
					t.Fatalf("%s: %d %q, want 400 invalid", name, rec.Code, rec.Body.String())
				}
			}
			if calls := o.Backend.Calls(); len(calls) != 0 {
				t.Fatalf("an invalid body reached the backend: %v", calls)
			}
		})

		t.Run("every answer is uncacheable JSON with no CORS header", func(t *testing.T) {
			o := newOutcomeHandler(t)
			token, _ := o.issue(member)
			recs := map[string]*httptest.ResponseRecorder{
				"passed":       o.serve("203.0.113.9:4000", "application/json", page(o, token), nil),
				"bad body":     o.serve("203.0.113.9:4000", "application/json", "{", nil),
				"bad type":     o.serve("203.0.113.9:4000", "text/plain", "x", nil),
				"unknown":      o.serve("203.0.113.9:4000", "application/json", page(o, token), nil),
				"wrong verb":   httptest.NewRecorder(),
				"rate limited": nil,
			}
			req := httptest.NewRequest(http.MethodGet, "/captcha/verify", nil)
			o.Handler.Verify(recs["wrong verb"], req)
			for i := 0; i < verifyIPLimit; i++ {
				o.serve("203.0.113.77:4000", "application/json", "{}", nil)
			}
			recs["rate limited"] = o.serve("203.0.113.77:4000", "application/json", "{}", nil)

			if recs["wrong verb"].Code != http.StatusMethodNotAllowed || recs["wrong verb"].Header().Get("Allow") != http.MethodPost {
				t.Fatalf("GET: %d Allow=%q, want 405 with Allow: POST", recs["wrong verb"].Code, recs["wrong verb"].Header().Get("Allow"))
			}
			if recs["rate limited"].Code != http.StatusTooManyRequests {
				t.Fatalf("rate limited: status = %d, want 429", recs["rate limited"].Code)
			}
			for name, rec := range recs {
				h := rec.Header()
				if h.Get("Cache-Control") != "no-store" || h.Get("X-Content-Type-Options") != "nosniff" ||
					!strings.HasPrefix(h.Get("Content-Type"), "application/json") {
					t.Fatalf("%s: headers = %v, want no-store, nosniff and JSON", name, h)
				}
				for key := range h {
					if strings.HasPrefix(strings.ToLower(key), "access-control-") {
						t.Fatalf("%s: CORS header %q on the verify route", name, key)
					}
				}
			}
		})

		t.Run("the three routes behave as a real server serves them", func(t *testing.T) {
			o := newOutcomeHandler(t)
			en := map[string]string{}
			for _, key := range PageKeys {
				en[key] = key
			}
			page, err := NewPage(PageOptions{SiteKey: "site-key", Strings: map[string]map[string]string{"en": en}})
			if err != nil {
				t.Fatalf("NewPage() error = %v", err)
			}
			mux := http.NewServeMux()
			for pattern, handler := range o.Handler.Routes(page) {
				mux.Handle(pattern, handler)
			}
			if RoutePage != "/captcha" || RouteVerify != "/captcha/verify" || RouteHealth != "/captcha/health" {
				t.Fatalf("route patterns = %q %q %q", RoutePage, RouteVerify, RouteHealth)
			}
			server := httptest.NewServer(mux)
			t.Cleanup(server.Close)

			do := func(method, path string) *http.Response {
				req, err := http.NewRequest(method, server.URL+path, nil)
				if err != nil {
					t.Fatalf("NewRequest(%s %s): %v", method, path, err)
				}
				resp, err := server.Client().Do(req)
				if err != nil {
					t.Fatalf("%s %s: %v", method, path, err)
				}
				t.Cleanup(func() { _ = resp.Body.Close() })
				return resp
			}

			// A wrong method gets the handler's own 405: Allow, no-store and nosniff, as
			// AGENTS.md documents, and for the verify route the JSON body of every answer.
			wrong := []struct {
				method, path, allow string
				json                bool
			}{
				{http.MethodGet, "/captcha/verify", "POST", true},
				{http.MethodOptions, "/captcha/verify", "POST", true},
				{http.MethodPut, "/captcha/verify", "POST", true},
				{http.MethodPost, "/captcha", "GET, HEAD", false},
				{http.MethodDelete, "/captcha", "GET, HEAD", false},
				{http.MethodPost, "/captcha/health", "GET, HEAD", true},
			}
			for _, tc := range wrong {
				resp := do(tc.method, tc.path)
				if resp.StatusCode != http.StatusMethodNotAllowed {
					t.Fatalf("%s %s: status = %d, want 405", tc.method, tc.path, resp.StatusCode)
				}
				if got := resp.Header.Get("Allow"); got != tc.allow {
					t.Fatalf("%s %s: Allow = %q, want %q", tc.method, tc.path, got, tc.allow)
				}
				if got := resp.Header.Get("Cache-Control"); got != "no-store" {
					t.Fatalf("%s %s: Cache-Control = %q, want no-store", tc.method, tc.path, got)
				}
				if got := resp.Header.Get("X-Content-Type-Options"); got != "nosniff" {
					t.Fatalf("%s %s: X-Content-Type-Options = %q, want nosniff", tc.method, tc.path, got)
				}
				if tc.json {
					if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
						t.Fatalf("%s %s: Content-Type = %q, want JSON", tc.method, tc.path, got)
					}
				}
				for key := range resp.Header {
					if strings.HasPrefix(strings.ToLower(key), "access-control-") {
						t.Fatalf("%s %s: CORS header %q", tc.method, tc.path, key)
					}
				}
			}
			var answer Answer
			if err := json.NewDecoder(do(http.MethodGet, "/captcha/verify").Body).Decode(&answer); err != nil || answer.Result != ResultInvalid {
				t.Fatalf("GET /captcha/verify body = %+v, %v; want the JSON invalid answer", answer, err)
			}

			for _, method := range []string{http.MethodGet, http.MethodHead} {
				if resp := do(method, "/captcha"); resp.StatusCode != http.StatusOK {
					t.Fatalf("%s /captcha: status = %d, want 200", method, resp.StatusCode)
				}
				if resp := do(method, "/captcha/health"); resp.StatusCode != http.StatusOK {
					t.Fatalf("%s /captcha/health: status = %d, want 200", method, resp.StatusCode)
				}
			}
			if resp := do(http.MethodGet, "/captcha/"); resp.StatusCode != http.StatusNotFound {
				t.Fatalf("GET /captcha/: status = %d, want 404", resp.StatusCode)
			}
		})
	})

	t.Run("in flight", func(t *testing.T) {
		const held = verifyMaxInFlight
		o := newOutcomeHandler(t)
		blocker := &blockingBackend{
			outcomeBackend: o.Backend,
			entered:        make(chan struct{}, held+1),
			release:        make(chan struct{}),
		}
		o.rebuild(func(opts *Options) { opts.Backend = blocker })

		bodies := make([]string, held)
		for i := range bodies {
			token, _ := o.issue(int64(1000 + i))
			bodies[i] = o.body(o.initData(int64(1000+i), token), TurnstileTokenForTest(token), token)
		}
		results := make([]*httptest.ResponseRecorder, held)
		var wg sync.WaitGroup
		for i := range bodies {
			wg.Add(1)
			go func() {
				defer wg.Done()
				results[i] = o.serve(fmt.Sprintf("198.51.100.%d:4000", i+1), "application/json", bodies[i], nil)
			}()
		}
		for i := 0; i < held; i++ {
			select {
			case <-blocker.entered:
			case <-time.After(10 * time.Second):
				close(blocker.release)
				t.Fatalf("only %d of %d requests are in flight", i, held)
			}
		}

		// The slot is taken after the body is read and the initData verified (WR-06): a body that
		// does not parse and initData that is not signed are answered while every slot is held.
		if rec := o.serve("203.0.113.98:4000", "application/json", "{}", nil); rec.Code != http.StatusBadRequest {
			close(blocker.release)
			t.Fatalf("an empty body with every slot held = %d %q, want 400 (no slot needed)", rec.Code, rec.Body.String())
		}
		forged := o.body("hash=0&auth_date=1", TurnstileTokenForTest("tok"), "tok")
		if rec := o.serve("203.0.113.97:4000", "application/json", forged, nil); rec.Code != http.StatusOK || answerOf(t, rec).Result != ResultInvalid {
			close(blocker.release)
			t.Fatalf("forged initData with every slot held = %d %q, want 200 invalid (no slot needed)", rec.Code, rec.Body.String())
		}

		// A verified request beyond the cap is refused before the challenge is claimed.
		extra, _ := o.issue(3000)
		extraBody := o.body(o.initData(3000, extra), TurnstileTokenForTest(extra), extra)
		rec := o.serve("203.0.113.99:4000", "application/json", extraBody, nil)
		if rec.Code != http.StatusServiceUnavailable || answerOf(t, rec).Result != ResultRetry {
			close(blocker.release)
			t.Fatalf("request %d: %d %q, want 503 retry", held+1, rec.Code, rec.Body.String())
		}
		if got := o.Redis.HGet(o.key(extra), "u"); got != "3000" || o.Redis.HGet(o.key(extra), "k") != "" {
			close(blocker.release)
			t.Fatalf("the refused request touched its challenge: owner %q", got)
		}
		if calls := o.Backend.Calls(); len(calls) != 0 {
			close(blocker.release)
			t.Fatalf("the refused request reached the backend: %v", calls)
		}

		close(blocker.release)
		wg.Wait()
		for i, res := range results {
			if res.Code != http.StatusOK || answerOf(t, res).Result != ResultPassed {
				t.Fatalf("held request %d: %d %q, want 200 passed", i, res.Code, res.Body.String())
			}
		}
		// The slots are free again.
		token, _ := o.issue(2000)
		if rec := o.postFrom("203.0.113.100:4000", 2000, token); rec.Code != http.StatusOK {
			t.Fatalf("after the burst: status = %d, want 200", rec.Code)
		}
	})
}

// blockingBackend holds every request at the bot-rights check until release is closed.
type blockingBackend struct {
	*outcomeBackend
	entered chan struct{}
	release chan struct{}
}

func (b *blockingBackend) BotCanRestrict(ctx context.Context, chatID int64) (bool, error) {
	b.entered <- struct{}{}
	<-b.release
	return b.outcomeBackend.BotCanRestrict(ctx, chatID)
}

// slowPassBackend takes longer to pass a member than the test server's write timeout.
type slowPassBackend struct {
	*outcomeBackend
	delay time.Duration
}

func (b *slowPassBackend) Pass(ctx context.Context, rec Record, user InitData) (PassResult, error) {
	time.Sleep(b.delay)
	return b.outcomeBackend.Pass(ctx, rec, user)
}

func TestVerifyRouteReadinessGate(t *testing.T) {
	const member = int64(42)
	o := newOutcomeHandler(t)
	o.rebuild(func(opts *Options) { opts.StartNotReady = true })
	token, _ := o.issue(member)

	commands := o.Redis.CommandCount()
	rec := o.postFrom("203.0.113.9:4000", member, token)
	if rec.Code != http.StatusServiceUnavailable || answerOf(t, rec).Result != ResultRetry {
		t.Fatalf("not ready: %d %q, want 503 retry", rec.Code, rec.Body.String())
	}
	if got := o.Redis.CommandCount(); got != commands {
		t.Fatalf("a request before readiness made %d Redis commands, want none", got-commands)
	}
	if calls := o.Backend.Calls(); len(calls) != 0 {
		t.Fatalf("a request before readiness reached the backend: %v", calls)
	}
	if o.Redis.HGet(o.key(token), "u") == "" {
		t.Fatalf("a request before readiness consumed the challenge")
	}

	o.Handler.SetReady(true)
	rec = o.postFrom("203.0.113.9:4000", member, token)
	if rec.Code != http.StatusOK || answerOf(t, rec).Result != ResultPassed {
		t.Fatalf("ready: %d %q, want 200 passed", rec.Code, rec.Body.String())
	}

	o.Handler.SetReady(false)
	token, _ = o.issue(member)
	if rec := o.postFrom("203.0.113.9:4000", member, token); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("not ready again: status = %d, want 503", rec.Code)
	}
}

func TestHealthRoute(t *testing.T) {
	health := func(o *outcomeHandler) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/captcha/health", nil)
		rec := httptest.NewRecorder()
		o.Handler.Health(rec, req)
		return rec
	}
	check := func(t *testing.T, rec *httptest.ResponseRecorder, wantCode int, wantStatus string) {
		t.Helper()
		if rec.Code != wantCode {
			t.Fatalf("status = %d, want %d", rec.Code, wantCode)
		}
		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("body %q is not JSON: %v", rec.Body.String(), err)
		}
		if len(body) != 1 || body["status"] != wantStatus {
			t.Fatalf("body = %v, want only status=%q", body, wantStatus)
		}
		if rec.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("Cache-Control = %q, want no-store", rec.Header().Get("Cache-Control"))
		}
	}

	t.Run("ready and Redis up", func(t *testing.T) {
		check(t, health(newOutcomeHandler(t)), http.StatusOK, "ok")
	})
	t.Run("not ready", func(t *testing.T) {
		o := newOutcomeHandler(t)
		o.rebuild(func(opts *Options) { opts.StartNotReady = true })
		check(t, health(o), http.StatusServiceUnavailable, "unavailable")
		o.Handler.SetReady(true)
		check(t, health(o), http.StatusOK, "ok")
	})
	t.Run("Redis down", func(t *testing.T) {
		o := newOutcomeHandler(t)
		o.Redis.Close()
		check(t, health(o), http.StatusServiceUnavailable, "unavailable")
	})
}

func TestVerifyRouteOutlivesServerWriteTimeout(t *testing.T) {
	const member = int64(42)
	o := newOutcomeHandler(t)
	o.rebuild(func(opts *Options) {
		opts.Backend = &slowPassBackend{outcomeBackend: o.Backend, delay: 800 * time.Millisecond}
	})
	token, _ := o.issue(member)

	mux := http.NewServeMux()
	for pattern, handler := range o.Handler.Routes(http.NotFoundHandler()) {
		mux.Handle(pattern, handler)
	}
	server := httptest.NewUnstartedServer(mux)
	server.Config.WriteTimeout = 500 * time.Millisecond
	server.Start()
	t.Cleanup(server.Close)

	resp, err := server.Client().Post(server.URL+"/captcha/verify", "application/json",
		strings.NewReader(o.body(o.initData(member, token), TurnstileTokenForTest(token), token)))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var answer Answer
	if err := json.NewDecoder(resp.Body).Decode(&answer); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.StatusCode != http.StatusOK || answer.Result != ResultPassed {
		t.Fatalf("answer = %d %q, want 200 passed", resp.StatusCode, answer.Result)
	}
}

func TestChallengeStoreConsumeOnce(t *testing.T) {
	const member = int64(42)

	t.Run("50 goroutines consume one token once", func(t *testing.T) {
		o := newOutcomeHandler(t)
		token, _ := o.issue(member)

		const workers = 50
		var (
			wg      sync.WaitGroup
			start   = make(chan struct{})
			mu      sync.Mutex
			results = map[ConsumeResult]int{}
			errs    []error
		)
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				got, _, _, err := o.Store.ConsumeIfUser(context.Background(), token, member)
				mu.Lock()
				defer mu.Unlock()
				results[got]++
				if err != nil {
					errs = append(errs, err)
				}
			}()
		}
		close(start)
		wg.Wait()

		if len(errs) != 0 {
			t.Fatalf("ConsumeIfUser errors: %v", errs)
		}
		if results[ConsumeOK] != 1 || results[ConsumeUnknown] != workers-1 {
			t.Fatalf("results = %v, want exactly one ConsumeOK and %d ConsumeUnknown", results, workers-1)
		}
	})

	t.Run("parallel verifies of one token act once", func(t *testing.T) {
		o := newOutcomeHandler(t)
		token, _ := o.issue(member)
		body := o.body(o.initData(member, token), TurnstileTokenForTest(token), token)

		const parallel = verifyTokenUserLimit
		var (
			wg      sync.WaitGroup
			start   = make(chan struct{})
			results = make([]*httptest.ResponseRecorder, parallel)
		)
		for i := 0; i < parallel; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				results[i] = o.serve("203.0.113.9:4000", "application/json", body, nil)
			}()
		}
		close(start)
		wg.Wait()

		attempts := 0
		for _, call := range o.Backend.Calls() {
			if call == "attempt" {
				attempts++
			}
		}
		if attempts != 1 {
			t.Fatalf("backend.Attempt called %d times, want 1 (calls: %v)", attempts, o.Backend.Calls())
		}
		if got := o.Requests.Load(); got != 1 {
			t.Fatalf("Cloudflare was asked %d times, want 1", got)
		}
		passes := 0
		for i, rec := range results {
			if rec.Code != http.StatusOK {
				t.Fatalf("request %d: status = %d, want 200", i, rec.Code)
			}
			switch got := answerOf(t, rec).Result; got {
			case ResultPassed:
				passes++
			case ResultInvalid:
			default:
				t.Fatalf("request %d: result = %q, want passed or invalid", i, got)
			}
		}
		if passes == 0 {
			t.Fatalf("no request was answered passed")
		}
		passCalls := 0
		for _, call := range o.Backend.Calls() {
			if call == "pass" {
				passCalls++
			}
		}
		if passCalls != 1 {
			t.Fatalf("backend.Pass called %d times, want 1", passCalls)
		}
	})
}

func TestVerifyRouteReplaysDecision(t *testing.T) {
	const (
		member = int64(42)
		other  = int64(77)
	)
	counts := func(o *outcomeHandler) (calls int, cloudflare int64) {
		return len(o.Backend.Calls()), o.Requests.Load()
	}

	t.Run("a passed answer is replayed to the same member only", func(t *testing.T) {
		o := newOutcomeHandler(t)
		token, _ := o.issue(member)
		if rec := o.postFrom("203.0.113.9:4000", member, token); answerOf(t, rec).Result != ResultPassed {
			t.Fatalf("first verify: %q, want passed", rec.Body.String())
		}
		calls, cloudflare := counts(o)
		if cloudflare != 1 {
			t.Fatalf("Cloudflare requests = %d, want 1", cloudflare)
		}

		rec := o.postFrom("203.0.113.9:4000", member, token)
		if rec.Code != http.StatusOK || answerOf(t, rec).Result != ResultPassed {
			t.Fatalf("repeat: %d %q, want 200 passed", rec.Code, rec.Body.String())
		}
		rec = o.postFrom("203.0.113.10:4000", other, token)
		if rec.Code != http.StatusOK || answerOf(t, rec).Result != ResultNotForYou {
			t.Fatalf("another member: %d %q, want 200 not_for_you", rec.Code, rec.Body.String())
		}
		if gotCalls, gotCloudflare := counts(o); gotCalls != calls || gotCloudflare != cloudflare {
			t.Fatalf("a repeat acted again: backend calls %d -> %d, Cloudflare %d -> %d", calls, gotCalls, cloudflare, gotCloudflare)
		}

		if ttl := o.Redis.TTL(DoneKeyPrefix + token); ttl <= 9*time.Minute || ttl > 10*time.Minute {
			t.Fatalf("done marker TTL = %v, want about 10 minutes", ttl)
		}
		o.Redis.FastForward(10*time.Minute + time.Second)
		rec = o.postFrom("203.0.113.9:4000", member, token)
		if rec.Code != http.StatusOK || answerOf(t, rec).Result != ResultInvalid {
			t.Fatalf("after the marker expired: %d %q, want 200 invalid", rec.Code, rec.Body.String())
		}
	})

	t.Run("a failed answer keeps its action", func(t *testing.T) {
		o := newOutcomeHandler(t)
		o.Backend.Status = AttemptStatus{Live: true, Action: "ban"}
		o.setSiteverify(SiteverifyAnswerForTest(200, failureBody("invalid-input-response")))
		token, _ := o.issue(member)
		first := answerOf(t, o.postFrom("203.0.113.9:4000", member, token))
		if first.Result != ResultFailed || first.Action != "ban" {
			t.Fatalf("first verify: %+v, want failed with ban", first)
		}
		calls, cloudflare := counts(o)

		repeat := answerOf(t, o.postFrom("203.0.113.9:4000", member, token))
		if repeat != first {
			t.Fatalf("repeat: %+v, want %+v", repeat, first)
		}
		if gotCalls, gotCloudflare := counts(o); gotCalls != calls || gotCloudflare != cloudflare {
			t.Fatalf("a repeat acted again: backend calls %d -> %d, Cloudflare %d -> %d", calls, gotCalls, cloudflare, gotCloudflare)
		}
	})

	t.Run("an expired answer keeps its action", func(t *testing.T) {
		o := newOutcomeHandler(t)
		o.Backend.Status = AttemptStatus{Live: false, Action: "mute"}
		token, _ := o.issue(member)
		first := answerOf(t, o.postFrom("203.0.113.9:4000", member, token))
		if first.Result != ResultExpired || first.Action != "mute" {
			t.Fatalf("first verify: %+v, want expired with mute", first)
		}
		calls, _ := counts(o)

		repeat := answerOf(t, o.postFrom("203.0.113.9:4000", member, token))
		if repeat != first {
			t.Fatalf("repeat: %+v, want %+v", repeat, first)
		}
		if gotCalls, _ := counts(o); gotCalls != calls {
			t.Fatalf("a repeat acted again: backend calls %d -> %d", calls, gotCalls)
		}
	})

	t.Run("a retry leaves no marker and the token is re-armed", func(t *testing.T) {
		o := newOutcomeHandler(t)
		o.setSiteverify(SiteverifyAnswerForTest(200, failureBody("internal-error")))
		token, _ := o.issue(member)
		if rec := o.postFrom("203.0.113.9:4000", member, token); answerOf(t, rec).Result != ResultRetry {
			t.Fatalf("first verify: %q, want retry", rec.Body.String())
		}
		if o.Redis.Exists(DoneKeyPrefix + token) {
			t.Fatalf("a retry wrote a done marker")
		}
		if !o.Redis.Exists(o.key(token)) {
			t.Fatalf("a retry did not re-arm the challenge")
		}
		o.setSiteverify(SiteverifyPassForTest(outcomeHost))
		if rec := o.postFrom("203.0.113.9:4000", member, token); answerOf(t, rec).Result != ResultPassed {
			t.Fatalf("second verify: %q, want passed", rec.Body.String())
		}
	})

	t.Run("an unknown token with no marker stays invalid", func(t *testing.T) {
		o := newOutcomeHandler(t)
		token, _ := NewToken()
		rec := o.postFrom("203.0.113.9:4000", member, token)
		if rec.Code != http.StatusOK || answerOf(t, rec).Result != ResultInvalid {
			t.Fatalf("unknown token: %d %q, want 200 invalid", rec.Code, rec.Body.String())
		}
	})

	t.Run("the store round-trips a decision", func(t *testing.T) {
		o := newOutcomeHandler(t)
		ctx := context.Background()
		if err := o.Store.MarkDone(ctx, "tok1", Decision{UserID: member, Result: ResultFailed, Action: "kick"}); err != nil {
			t.Fatalf("MarkDone() error = %v", err)
		}
		got, found, err := o.Store.Done(ctx, "tok1")
		want := Decision{UserID: member, Result: ResultFailed, Action: "kick"}
		if err != nil || !found || got != want {
			t.Fatalf("Done() = (%+v, %v, %v), want (%+v, true, nil)", got, found, err, want)
		}
		if err := o.Store.MarkDone(ctx, "tok2", Decision{UserID: member, Result: ResultPassed}); err != nil {
			t.Fatalf("MarkDone() error = %v", err)
		}
		got, found, err = o.Store.Done(ctx, "tok2")
		if err != nil || !found || got != (Decision{UserID: member, Result: ResultPassed}) {
			t.Fatalf("Done() = (%+v, %v, %v), want a passed decision with no action", got, found, err)
		}
		if _, found, err := o.Store.Done(ctx, "missing"); err != nil || found {
			t.Fatalf("Done(missing) = (%v, %v), want not found", found, err)
		}
		// A decision that is not a decided answer is never stored or read back.
		if err := o.Store.MarkDone(ctx, "tok3", Decision{UserID: member, Result: ResultRetry}); err == nil {
			t.Fatalf("MarkDone() accepted a retry")
		}
		o.Redis.Set(DoneKeyPrefix+"tok4", "garbage")
		if _, found, err := o.Store.Done(ctx, "tok4"); err != nil || found {
			t.Fatalf("Done(garbage) = (%v, %v), want not found", found, err)
		}
	})
}

// dropReplyHook runs a Redis script for real and then reports an error to the caller, as a
// connection that drops after Redis executed the command does: the effect happened, the reply
// did not arrive.
type dropReplyHook struct{ drop atomic.Bool }

func (h *dropReplyHook) DialHook(next redis.DialHook) redis.DialHook { return next }

func (h *dropReplyHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		err := next(ctx, cmd)
		name := strings.ToLower(cmd.Name())
		if err == nil && (name == "eval" || name == "evalsha") && h.drop.CompareAndSwap(true, false) {
			return errors.New("connection reset by peer")
		}
		return err
	}
}

func (h *dropReplyHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}

// TestChallengeStoreConsumeIsUndoneByAnError pins WR-03: a ConsumeIfUser that returns an error
// used up nothing. The key is claimed first and deleted only once the reply was parsed, so a
// reply that is lost after Redis ran the script, or one that does not parse, leaves the
// challenge for the member's next tap.
func TestChallengeStoreConsumeIsUndoneByAnError(t *testing.T) {
	const member = int64(42)

	t.Run("a reply lost after the script ran", func(t *testing.T) {
		o := newOutcomeHandler(t)
		hook := &dropReplyHook{}
		o.Store.client.AddHook(hook)
		token, rec := o.issue(member)

		hook.drop.Store(true)
		if _, _, _, err := o.Store.ConsumeIfUser(context.Background(), token, member); err == nil {
			t.Fatal("ConsumeIfUser() = nil error, want the lost reply")
		}
		if got := o.Redis.HGet(o.key(token), "u"); got != "42" {
			t.Fatalf("challenge owner after the lost reply = %q, want the challenge intact", got)
		}
		if o.Redis.HGet(o.key(token), "k") != "" {
			t.Fatal("the claim marker was left behind")
		}

		got, gotRec, left, err := o.Store.ConsumeIfUser(context.Background(), token, member)
		if err != nil || got != ConsumeOK || gotRec.AttemptID != rec.AttemptID || left <= 0 {
			t.Fatalf("second ConsumeIfUser() = %v, %+v, %v, %v; want ConsumeOK with the record", got, gotRec, left, err)
		}
		if o.Redis.Exists(o.key(token)) {
			t.Fatal("a consumed challenge is still stored")
		}
	})

	t.Run("a record that does not parse", func(t *testing.T) {
		o := newOutcomeHandler(t)
		token, _ := NewToken()
		o.Redis.HSet(o.key(token), "u", "42", "a", "not-a-number", "c", "-1001234", "x", "1")
		o.Redis.SetTTL(o.key(token), 5*time.Minute)

		if got, _, _, err := o.Store.ConsumeIfUser(context.Background(), token, member); err == nil || got == ConsumeOK {
			t.Fatalf("ConsumeIfUser() = %v, %v; want an error and never ConsumeOK", got, err)
		}
		if o.Redis.HGet(o.key(token), "u") != "42" || o.Redis.HGet(o.key(token), "k") != "" {
			t.Fatal("the unparsable record was consumed or left claimed")
		}
	})

	t.Run("a claimed key is used up for everyone until it is released", func(t *testing.T) {
		o := newOutcomeHandler(t)
		token, rec := o.issue(member)
		// A claim whose delete never ran: another caller must see the challenge as used.
		o.Redis.HSet(o.key(token), "k", "some-claim")
		if got, _, _, err := o.Store.ConsumeIfUser(context.Background(), token, member); err != nil || got != ConsumeUnknown {
			t.Fatalf("ConsumeIfUser() over a claim = %v, %v; want ConsumeUnknown", got, err)
		}
		// A re-arm overwrites the husk, with the record intact.
		if ok, err := o.Store.Rearm(context.Background(), token, rec, time.Minute); !ok || err != nil {
			t.Fatalf("Rearm() over a husk = %v, %v; want true", ok, err)
		}
		if o.Redis.HGet(o.key(token), "k") != "" || o.Redis.HGet(o.key(token), "u") != "42" {
			t.Fatal("the re-armed challenge still carries the claim")
		}
	})
}
