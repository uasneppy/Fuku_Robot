//go:build testtools

package captchaweb

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
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

		t.Run("the three routes behave as a mux serves them", func(t *testing.T) {
			o := newOutcomeHandler(t)
			mux := http.NewServeMux()
			pageHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
			for pattern, handler := range o.Handler.Routes(pageHandler) {
				mux.Handle(pattern, handler)
			}
			if RoutePage != "GET /captcha" || RouteVerify != "POST /captcha/verify" || RouteHealth != "GET /captcha/health" {
				t.Fatalf("route patterns = %q %q %q", RoutePage, RouteVerify, RouteHealth)
			}
			do := func(method, path string) *httptest.ResponseRecorder {
				req := httptest.NewRequest(method, path, nil)
				req.RemoteAddr = "203.0.113.9:4000"
				rec := httptest.NewRecorder()
				mux.ServeHTTP(rec, req)
				return rec
			}
			for _, method := range []string{http.MethodOptions, http.MethodGet, http.MethodPut} {
				rec := do(method, "/captcha/verify")
				if rec.Code != http.StatusMethodNotAllowed || !strings.Contains(rec.Header().Get("Allow"), http.MethodPost) {
					t.Fatalf("%s /captcha/verify: %d Allow=%q, want 405 with POST", method, rec.Code, rec.Header().Get("Allow"))
				}
			}
			if rec := do(http.MethodHead, "/captcha"); rec.Code != http.StatusOK {
				t.Fatalf("HEAD /captcha: status = %d, want 200", rec.Code)
			}
			if rec := do(http.MethodGet, "/captcha/"); rec.Code != http.StatusNotFound {
				t.Fatalf("GET /captcha/: status = %d, want 404", rec.Code)
			}
			if rec := do(http.MethodGet, "/captcha/health"); rec.Code != http.StatusOK {
				t.Fatalf("GET /captcha/health: status = %d, want 200", rec.Code)
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

		commands := o.Redis.CommandCount()
		rec := o.serve("203.0.113.99:4000", "application/json", "{}", nil)
		if rec.Code != http.StatusServiceUnavailable || answerOf(t, rec).Result != ResultRetry {
			close(blocker.release)
			t.Fatalf("request %d: %d %q, want 503 retry", held+1, rec.Code, rec.Body.String())
		}
		if got := o.Redis.CommandCount(); got != commands {
			close(blocker.release)
			t.Fatalf("the refused request made %d Redis commands, want none", got-commands)
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
