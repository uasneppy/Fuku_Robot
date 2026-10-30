//go:build testtools

package captchaweb

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// outcomeBackend is a hand-written Backend that records its calls and answers with scripted
// results. Set the fields before the handler is driven; they are read under mu.
type outcomeBackend struct {
	mu sync.Mutex

	CanRestrict    bool
	CanRestrictErr error
	Status         AttemptStatus
	AttemptErr     error
	PassResult     PassResult
	PassErr        error
	FailClaimed    bool
	FailErr        error
	ExpireErr      error

	calls []string
}

func newOutcomeBackend() *outcomeBackend {
	return &outcomeBackend{
		CanRestrict: true,
		Status:      AttemptStatus{Live: true, Action: "kick"},
		PassResult:  PassOK,
		FailClaimed: true,
	}
}

func (b *outcomeBackend) record(name string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls = append(b.calls, name)
}

// Calls returns the names of the Backend methods called so far, in order.
func (b *outcomeBackend) Calls() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.calls...)
}

func (b *outcomeBackend) BotCanRestrict(context.Context, int64) (bool, error) {
	b.record("bot_can_restrict")
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.CanRestrict, b.CanRestrictErr
}

func (b *outcomeBackend) Attempt(context.Context, Record) (AttemptStatus, error) {
	b.record("attempt")
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Status, b.AttemptErr
}

func (b *outcomeBackend) Pass(context.Context, Record, InitData) (PassResult, error) {
	b.record("pass")
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.PassResult, b.PassErr
}

func (b *outcomeBackend) Fail(context.Context, Record) (bool, error) {
	b.record("fail")
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.FailClaimed, b.FailErr
}

func (b *outcomeBackend) Expire(context.Context, Record) error {
	b.record("expire")
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.ExpireErr
}

const (
	outcomeHost     = "captcha.example.com"
	outcomeBotToken = "999:outcome-test-token"
)

// outcomeHandler is a verify Handler over a scripted outcomeBackend, a fresh miniredis and a
// fake Cloudflare. Every test builds its own, so the verify route's per-handler limits are
// never shared.
type outcomeHandler struct {
	t        *testing.T
	Handler  *Handler
	Backend  *outcomeBackend
	Store    *Store
	Redis    *miniredis.Miniredis
	Client   *Client
	Requests atomic.Int64

	siteverify atomic.Value // http.HandlerFunc
}

// newOutcomeHandler builds the handler. Cloudflare confirms every token made by
// TurnstileTokenForTest until setSiteverify says otherwise.
func newOutcomeHandler(t *testing.T) *outcomeHandler {
	t.Helper()
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rc.Close() })

	o := &outcomeHandler{t: t, Backend: newOutcomeBackend(), Store: NewStore(rc), Redis: mr}
	o.setSiteverify(SiteverifyPassForTest(outcomeHost))
	o.Client = SiteverifyServerForTest(t, func(w http.ResponseWriter, r *http.Request) {
		o.Requests.Add(1)
		o.siteverify.Load().(http.HandlerFunc)(w, r)
	})
	handler, err := NewHandler(Options{
		BotToken:      outcomeBotToken,
		PublicBaseURL: "https://" + outcomeHost,
		Store:         o.Store,
		Siteverify:    o.Client,
		Backend:       o.Backend,
	})
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}
	o.Handler = handler
	return o
}

func (o *outcomeHandler) setSiteverify(h http.HandlerFunc) {
	o.siteverify.Store(h)
}

// issue stores a challenge for the member and returns its token. The attempt ID is 1.
func (o *outcomeHandler) issue(userID int64) (string, Record) {
	o.t.Helper()
	token, err := NewToken()
	if err != nil {
		o.t.Fatalf("NewToken() error = %v", err)
	}
	rec := Record{AttemptID: 1, ChatID: -1001234, UserID: userID, ExpiresAt: time.Now().Add(10 * time.Minute)}
	if err := o.Store.Issue(context.Background(), token, rec); err != nil {
		o.t.Fatalf("Issue() error = %v", err)
	}
	return token, rec
}

func (o *outcomeHandler) key(token string) string { return KeyPrefix + token }

// initData is the signed initData the member's Mini App would hold.
func (o *outcomeHandler) initData(userID int64, token string) string {
	return InitDataForTest(outcomeBotToken, userID, "Ünïcode Name", "en", token, time.Now())
}

// body is the JSON a page posts.
func (o *outcomeHandler) body(initData, turnstileToken, challenge string) string {
	o.t.Helper()
	body, err := json.Marshal(map[string]string{
		"init_data":       initData,
		"turnstile_token": turnstileToken,
		"challenge":       challenge,
	})
	if err != nil {
		o.t.Fatalf("marshal body: %v", err)
	}
	return string(body)
}

// postBody sends body to the handler and returns the status and the decoded answer.
func (o *outcomeHandler) postBody(body string) (int, Answer) {
	o.t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/captcha/verify", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	o.Handler.Verify(rec, req)
	var answer Answer
	if err := json.Unmarshal(rec.Body.Bytes(), &answer); err != nil {
		o.t.Fatalf("answer %q is not JSON: %v", rec.Body.String(), err)
	}
	return rec.Code, answer
}

// post sends the body a real page posts for userID and token.
func (o *outcomeHandler) post(userID int64, token string) (int, Answer) {
	o.t.Helper()
	return o.postBody(o.body(o.initData(userID, token), TurnstileTokenForTest(token), token))
}

func hasCall(calls []string, name string) bool {
	for _, c := range calls {
		if c == name {
			return true
		}
	}
	return false
}

// TestVerifyRouteRearmsOnlyOnRetry pins D-21: the token is given back only when the member
// can still try again (a retry verdict, a preflight refusal, a backend error before any
// claim), with the time it had left, and never after a decision.
func TestVerifyRouteRearmsOnlyOnRetry(t *testing.T) {
	const member = int64(42)

	cases := []struct {
		name        string
		setup       func(o *outcomeHandler)
		wantStatus  int
		wantResult  Result
		wantAction  string
		wantRearmed bool
		wantCalls   []string
	}{
		{
			name:        "a retry verdict re-arms",
			setup:       func(o *outcomeHandler) { o.setSiteverify(SiteverifyAnswerForTest(200, failureBody("internal-error"))) },
			wantStatus:  200,
			wantResult:  ResultRetry,
			wantRearmed: true,
			wantCalls:   []string{"bot_can_restrict", "attempt"},
		},
		{
			name:        "a configuration fault re-arms",
			setup:       func(o *outcomeHandler) { o.setSiteverify(SiteverifyPassForTest("other.example.net")) },
			wantStatus:  200,
			wantResult:  ResultRetry,
			wantRearmed: true,
			wantCalls:   []string{"bot_can_restrict", "attempt"},
		},
		{
			name: "a failed solve does not re-arm",
			setup: func(o *outcomeHandler) {
				o.setSiteverify(SiteverifyAnswerForTest(200, failureBody("invalid-input-response")))
			},
			wantStatus: 200,
			wantResult: ResultFailed,
			wantAction: "kick",
			wantCalls:  []string{"bot_can_restrict", "attempt", "fail"},
		},
		{
			name: "a failed solve that lost the claim expires and does not re-arm",
			setup: func(o *outcomeHandler) {
				o.setSiteverify(SiteverifyAnswerForTest(200, failureBody("timeout-or-duplicate")))
				o.Backend.FailClaimed = false
			},
			wantStatus: 200,
			wantResult: ResultExpired,
			wantAction: "kick",
			wantCalls:  []string{"bot_can_restrict", "attempt", "fail", "expire"},
		},
		{
			name: "a failed solve whose claim errored re-arms",
			setup: func(o *outcomeHandler) {
				o.setSiteverify(SiteverifyAnswerForTest(200, failureBody("invalid-input-response")))
				o.Backend.FailClaimed = false
				o.Backend.FailErr = errors.New("database down")
			},
			wantStatus:  503,
			wantResult:  ResultRetry,
			wantRearmed: true,
			wantCalls:   []string{"bot_can_restrict", "attempt", "fail"},
		},
		{
			name:       "a pass does not re-arm",
			setup:      func(*outcomeHandler) {},
			wantStatus: 200,
			wantResult: ResultPassed,
			wantCalls:  []string{"bot_can_restrict", "attempt", "pass"},
		},
		{
			name:       "a pass that lost the claim expires and does not re-arm",
			setup:      func(o *outcomeHandler) { o.Backend.PassResult = PassLost },
			wantStatus: 200,
			wantResult: ResultExpired,
			wantAction: "kick",
			wantCalls:  []string{"bot_can_restrict", "attempt", "pass", "expire"},
		},
		{
			name:       "a claimed pass whose unmute failed is a retry and does not re-arm",
			setup:      func(o *outcomeHandler) { o.Backend.PassResult = PassUnmuteFailed },
			wantStatus: 200,
			wantResult: ResultRetry,
			wantCalls:  []string{"bot_can_restrict", "attempt", "pass"},
		},
		{
			name: "a pass that errored before the claim re-arms",
			setup: func(o *outcomeHandler) {
				o.Backend.PassResult = PassLost
				o.Backend.PassErr = errors.New("telegram down")
			},
			wantStatus:  503,
			wantResult:  ResultRetry,
			wantRearmed: true,
			wantCalls:   []string{"bot_can_restrict", "attempt", "pass"},
		},
		{
			name:       "an expired attempt does not re-arm",
			setup:      func(o *outcomeHandler) { o.Backend.Status = AttemptStatus{Live: false, Action: "ban"} },
			wantStatus: 200,
			wantResult: ResultExpired,
			wantAction: "ban",
			wantCalls:  []string{"bot_can_restrict", "attempt", "expire"},
		},
		{
			name:        "a bot without the Restrict right re-arms and asks nothing",
			setup:       func(o *outcomeHandler) { o.Backend.CanRestrict = false },
			wantStatus:  200,
			wantResult:  ResultRetry,
			wantRearmed: true,
			wantCalls:   []string{"bot_can_restrict"},
		},
		{
			name:        "an unknown bot right re-arms and asks nothing",
			setup:       func(o *outcomeHandler) { o.Backend.CanRestrictErr = errors.New("timeout") },
			wantStatus:  200,
			wantResult:  ResultRetry,
			wantRearmed: true,
			wantCalls:   []string{"bot_can_restrict"},
		},
		{
			name:        "an attempt lookup error re-arms",
			setup:       func(o *outcomeHandler) { o.Backend.AttemptErr = errors.New("database down") },
			wantStatus:  503,
			wantResult:  ResultRetry,
			wantRearmed: true,
			wantCalls:   []string{"bot_can_restrict", "attempt"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := newOutcomeHandler(t)
			tc.setup(o)
			token, rec := o.issue(member)
			before := o.Redis.TTL(o.key(token))

			status, answer := o.post(member, token)
			if status != tc.wantStatus || answer.Result != tc.wantResult || answer.Action != tc.wantAction {
				t.Fatalf("answer = %d %+v, want %d %q action %q", status, answer, tc.wantStatus, tc.wantResult, tc.wantAction)
			}
			if got := o.Backend.Calls(); strings.Join(got, ",") != strings.Join(tc.wantCalls, ",") {
				t.Fatalf("backend calls = %v, want %v", got, tc.wantCalls)
			}

			key := o.key(token)
			if o.Redis.Exists(key) != tc.wantRearmed {
				t.Fatalf("challenge key present = %v, want %v", o.Redis.Exists(key), tc.wantRearmed)
			}
			if !tc.wantRearmed {
				return
			}
			for field, want := range map[string]string{
				"u": "42",
				"a": "1",
				"c": "-1001234",
				"x": strconv.FormatInt(rec.ExpiresAt.UnixMilli(), 10),
			} {
				if got := o.Redis.HGet(key, field); got != want {
					t.Fatalf("re-armed field %s = %q, want %q", field, got, want)
				}
			}
			after := o.Redis.TTL(key)
			if after <= 0 || after > before {
				t.Fatalf("re-armed ttl = %v, want in (0, %v]", after, before)
			}

			// The same token can be tried again and, once Cloudflare is back, passes.
			o.setSiteverify(SiteverifyPassForTest(outcomeHost))
			o.Backend.mu.Lock()
			o.Backend.CanRestrict, o.Backend.CanRestrictErr, o.Backend.AttemptErr = true, nil, nil
			o.Backend.PassResult, o.Backend.PassErr = PassOK, nil
			o.Backend.mu.Unlock()
			if status, answer := o.post(member, token); status != 200 || answer.Result != ResultPassed {
				t.Fatalf("second post = %d %+v, want 200 passed", status, answer)
			}
		})
	}

	t.Run("another member's verify and an unknown token never create a key", func(t *testing.T) {
		o := newOutcomeHandler(t)
		token, _ := o.issue(member)

		status, answer := o.post(43, token)
		if status != 200 || answer.Result != ResultNotForYou {
			t.Fatalf("post(other) = %d %+v, want 200 not_for_you", status, answer)
		}
		if got := o.Redis.HGet(o.key(token), "u"); got != "42" {
			t.Fatalf("challenge owner after another member's verify = %q, want 42", got)
		}
		if calls := o.Backend.Calls(); len(calls) != 0 {
			t.Fatalf("backend calls after another member's verify = %v, want none", calls)
		}

		unknown, err := NewToken()
		if err != nil {
			t.Fatal(err)
		}
		status, answer = o.post(member, unknown)
		if status != 200 || answer.Result != ResultInvalid {
			t.Fatalf("post(unknown) = %d %+v, want 200 invalid", status, answer)
		}
		if o.Redis.Exists(o.key(unknown)) {
			t.Fatal("an unknown token was turned into a challenge")
		}
		if got := o.Requests.Load(); got != 0 {
			t.Fatalf("Cloudflare requests = %d, want none", got)
		}
	})

	t.Run("a spent lifetime is not re-armed", func(t *testing.T) {
		o := newOutcomeHandler(t)
		token, rec := o.issue(member)
		if ok, err := o.Store.Rearm(context.Background(), token, rec, 0); ok || err != nil {
			t.Fatalf("Rearm(0) = %v, %v; want false, nil", ok, err)
		}
		// consume, then try with the remaining time of a key that is gone
		if _, _, _, err := o.Store.ConsumeIfUser(context.Background(), token, member); err != nil {
			t.Fatal(err)
		}
		if ok, err := o.Store.Rearm(context.Background(), token, rec, -time.Second); ok || err != nil {
			t.Fatalf("Rearm(negative) = %v, %v; want false, nil", ok, err)
		}
		if ok, err := o.Store.Rearm(context.Background(), token, rec, time.Minute); !ok || err != nil {
			t.Fatalf("Rearm(1m) = %v, %v; want true, nil", ok, err)
		}
		// an existing key is never overwritten
		other := Record{AttemptID: 2, ChatID: -5, UserID: 99, ExpiresAt: rec.ExpiresAt}
		if ok, err := o.Store.Rearm(context.Background(), token, other, time.Minute); ok || err != nil {
			t.Fatalf("Rearm over an existing key = %v, %v; want false, nil", ok, err)
		}
		if got := o.Redis.HGet(o.key(token), "u"); got != "42" {
			t.Fatalf("the existing challenge was overwritten: owner %q", got)
		}
	})
}
