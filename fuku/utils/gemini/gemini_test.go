//go:build testtools

package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

const testKey = "AIzaFAKEFAKEFAKEFAKEFAKEFAKEFAKEFAKE012345"

type recorded struct {
	Path     string
	RawQuery string
	Header   http.Header
	Body     []byte
}

type recorder struct {
	mu   sync.Mutex
	reqs []recorded
}

func (r *recorder) all() []recorded {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]recorded(nil), r.reqs...)
}

func newServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *recorder) {
	t.Helper()
	rec := &recorder{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		rec.mu.Lock()
		rec.reqs = append(rec.reqs, recorded{Path: r.URL.EscapedPath(), RawQuery: r.URL.RawQuery, Header: r.Header.Clone(), Body: body})
		rec.mu.Unlock()
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	return server, rec
}

func client(server *httptest.Server) *Client {
	return &Client{Endpoint: server.URL, APIKey: testKey, Model: DefaultModel, HTTP: server.Client()}
}

func respond(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

func answer(finish, parts string) string {
	return `{"candidates":[{"content":{"parts":[` + parts + `],"role":"model"},"finishReason":"` + finish + `","index":0}],` +
		`"usageMetadata":{"promptTokenCount":11500,"cachedContentTokenCount":9800,"candidatesTokenCount":90,"thoughtsTokenCount":200,"totalTokenCount":11790},` +
		`"modelVersion":"gemini-4-hydrogen","responseId":"r1"}`
}

func request() Request {
	return Request{Static: "STATIC", Dynamic: "Group: Test", User: "Resident: hello", MaxTokens: 2000}
}

func TestGenerateRequestShape(t *testing.T) {
	server, rec := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		respond(w, http.StatusOK, answer("STOP", `{"text":"Access granted."}`))
	})
	reply, err := client(server).Generate(context.Background(), request())
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if reply.Text != "Access granted." || reply.Outcome != OutcomeAnswered || reply.CacheReadTokens != 9800 || reply.ThoughtsTokens != 200 {
		t.Fatalf("reply = %+v", reply)
	}
	req := rec.all()[0]
	if req.Path != "/v1beta/models/gemini-4-hydrogen:generateContent" {
		t.Errorf("path = %s", req.Path)
	}
	if req.RawQuery != "" {
		t.Errorf("query = %q, the key must never be in the URL", req.RawQuery)
	}
	if req.Header.Get("x-goog-api-key") != testKey {
		t.Error("key header missing")
	}

	var body map[string]any
	if err := json.Unmarshal(req.Body, &body); err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"systemInstruction": true, "contents": true, "generationConfig": true, "safetySettings": true}
	for k := range body {
		if !want[k] {
			t.Errorf("unexpected field %q", k)
		}
	}
	parts := body["systemInstruction"].(map[string]any)["parts"].([]any)
	if len(parts) != 2 || parts[0].(map[string]any)["text"] != "STATIC" || parts[1].(map[string]any)["text"] != "Group: Test" {
		t.Errorf("system parts = %v", parts)
	}
	contents := body["contents"].([]any)
	if len(contents) != 1 || contents[0].(map[string]any)["role"] != "user" {
		t.Errorf("contents = %v", contents)
	}
	gen := body["generationConfig"].(map[string]any)
	if gen["maxOutputTokens"] != float64(2000) || gen["thinkingConfig"].(map[string]any)["thinkingLevel"] != "low" || len(gen) != 2 {
		t.Errorf("generationConfig = %v", gen)
	}
	safety := body["safetySettings"].([]any)
	if len(safety) != 4 {
		t.Fatalf("safety settings = %d, want 4", len(safety))
	}
	for _, s := range safety {
		if s.(map[string]any)["threshold"] != "BLOCK_ONLY_HIGH" {
			t.Errorf("threshold = %v", s)
		}
	}
	for _, banned := range []string{"temperature", "topP", "topK", "candidateCount", "thinkingBudget", "system_instruction"} {
		if bytes.Contains(req.Body, []byte(`"`+banned+`"`)) {
			t.Errorf("request carries %q", banned)
		}
	}
	if !strings.HasPrefix(string(req.Body), `{"systemInstruction":{"parts":[{"text":"STATIC"}`) {
		t.Error("the static text is not the start of the request")
	}
}

func TestGenerateAnswers(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		outcome Outcome
		text    string
		noReply bool
	}{
		{"stop skips thoughts", answer("STOP", `{"text":"plan","thought":true},{"text":"Request "},{"text":"denied."}`), OutcomeAnswered, "Request denied.", false},
		{"lowercase finish", answer("stop", `{"text":"ok"}`), OutcomeAnswered, "ok", false},
		{"max tokens with text", answer("MAX_TOKENS", `{"text":"Half"}`), OutcomeTruncated, "Half", false},
		{"max tokens blank", answer("MAX_TOKENS", `{"text":"x","thought":true}`), 0, "", true},
		{"safety", answer("SAFETY", ``), OutcomeRefused, "", false},
		{"prohibited", answer("PROHIBITED_CONTENT", ``), OutcomeRefused, "", false},
		{"blocklist", answer("BLOCKLIST", ``), OutcomeRefused, "", false},
		{"spii", answer("SPII", ``), OutcomeRefused, "", false},
		{"recitation", answer("RECITATION", `{"text":"x"}`), OutcomeRefused, "", false},
		{"blocked prompt, no candidates", `{"promptFeedback":{"blockReason":"PROHIBITED_CONTENT"},"usageMetadata":{"promptTokenCount":10}}`, OutcomeRefused, "", false},
		{"blocked prompt, empty candidates", `{"candidates":[],"promptFeedback":{"blockReason":"safety"}}`, OutcomeRefused, "", false},
		{"blocked prompt, candidate without content", `{"candidates":[{"index":0}],"promptFeedback":{"blockReason":"OTHER"}}`, OutcomeRefused, "", false},
		{"no candidates, no block", `{"usageMetadata":{}}`, 0, "", true},
		{"candidate without content", `{"candidates":[{"finishReason":"STOP"}]}`, 0, "", true},
		{"blank stop", answer("STOP", `{"text":"  "}`), 0, "", true},
		{"other", answer("OTHER", `{"text":"x"}`), 0, "", true},
		{"language", answer("LANGUAGE", `{"text":"x"}`), 0, "", true},
		{"unknown", answer("SOMETHING_NEW", `{"text":"x"}`), 0, "", true},
		{"unreadable", `nope`, 0, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server, _ := newServer(t, func(w http.ResponseWriter, _ *http.Request) { respond(w, http.StatusOK, tc.body) })
			reply, err := client(server).Generate(context.Background(), request())
			if tc.noReply {
				if !errors.Is(err, ErrNoReply) {
					t.Fatalf("err = %v, want ErrNoReply", err)
				}
				return
			}
			if err != nil || reply.Outcome != tc.outcome || reply.Text != tc.text {
				t.Fatalf("reply %+v err %v", reply, err)
			}
		})
	}
}

func geminiError(code int, status, message, details string) string {
	return `{"error":{"code":` + jsonInt(code) + `,"message":"` + message + `","status":"` + status + `","details":[` + details + `]}}`
}

func jsonInt(n int) string { b, _ := json.Marshal(n); return string(b) }

func TestGenerateErrors(t *testing.T) {
	cases := []struct {
		name      string
		status    int
		body      string
		retryable bool
		rejected  bool
		calls     int
	}{
		{"invalid key 400", 400, geminiError(400, "INVALID_ARGUMENT", "API key not valid. Please pass a valid API key.",
			`{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"API_KEY_INVALID","domain":"googleapis.com"}`), false, true, 1},
		{"malformed 400", 400, geminiError(400, "INVALID_ARGUMENT", "bad field", ``), false, false, 1},
		{"401", 401, geminiError(401, "UNAUTHENTICATED", "no", ``), false, true, 1},
		{"402", 402, geminiError(402, "RESOURCE_EXHAUSTED", "credit", ``), false, true, 1},
		{"403 leaked", 403, geminiError(403, "PERMISSION_DENIED", "Your API key was reported as leaked.", ``), false, true, 1},
		{"404", 404, geminiError(404, "NOT_FOUND", "model", ``), false, false, 1},
		{"429 long delay", 429, geminiError(429, "RESOURCE_EXHAUSTED", "retry",
			`{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"27s"}`), true, false, 1},
		{"429 daily quota", 429, geminiError(429, "RESOURCE_EXHAUSTED", "quota",
			`{"@type":"type.googleapis.com/google.rpc.QuotaFailure","violations":[{"quotaId":"GenerateRequestsPerDayPerProjectPerModel-FreeTier"}]},`+
				`{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"1s"}`), false, false, 1},
		{"429 bare retried once", 429, ``, true, false, 2},
		{"503 retried once", 503, geminiError(503, "UNAVAILABLE", "overloaded", ``), true, false, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server, rec := newServer(t, func(w http.ResponseWriter, _ *http.Request) { respond(w, tc.status, tc.body) })
			_, err := client(server).Generate(context.Background(), request())
			var httpErr *HTTPError
			if !errors.As(err, &httpErr) {
				t.Fatalf("err = %v, want *HTTPError", err)
			}
			if httpErr.Status != tc.status || httpErr.Retryable() != tc.retryable || httpErr.KeyRejected() != tc.rejected {
				t.Errorf("status %d retryable %v rejected %v", httpErr.Status, httpErr.Retryable(), httpErr.KeyRejected())
			}
			if got := len(rec.all()); got != tc.calls {
				t.Errorf("requests = %d, want %d", got, tc.calls)
			}
		})
	}

	t.Run("retry delay read from RetryInfo", func(t *testing.T) {
		e := newHTTPError(429, http.Header{}, []byte(geminiError(429, "RESOURCE_EXHAUSTED", "x",
			`{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"27.5s"}`)), testKey)
		if e.RetryDelay != 27500*time.Millisecond {
			t.Fatalf("delay = %v", e.RetryDelay)
		}
	})
	t.Run("short retry then success", func(t *testing.T) {
		calls := 0
		server, _ := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
			calls++
			if calls == 1 {
				respond(w, 429, geminiError(429, "RESOURCE_EXHAUSTED", "x", `{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"0.1s"}`))
				return
			}
			respond(w, 200, answer("STOP", `{"text":"ok"}`))
		})
		reply, err := client(server).Generate(context.Background(), request())
		if err != nil || reply.Text != "ok" || calls != 2 {
			t.Fatalf("reply %+v err %v calls %d", reply, err, calls)
		}
	})
	t.Run("the key never reaches an error", func(t *testing.T) {
		server, _ := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
			respond(w, 400, geminiError(400, "INVALID_ARGUMENT", "API key not valid: "+testKey, ``))
		})
		_, err := client(server).Generate(context.Background(), request())
		var httpErr *HTTPError
		if !errors.As(err, &httpErr) || strings.Contains(err.Error(), testKey) || strings.Contains(httpErr.Message, testKey) {
			t.Fatalf("key leaked: %v / %q", err, httpErr.Message)
		}
	})
	t.Run("transport error carries no URL or key", func(t *testing.T) {
		c := &Client{Endpoint: "http://127.0.0.1:1", APIKey: testKey, Model: DefaultModel}
		_, err := c.Generate(context.Background(), request())
		if err == nil || strings.Contains(err.Error(), testKey) || strings.Contains(err.Error(), "generateContent") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("redirect is not followed", func(t *testing.T) {
		server, rec := newServer(t, func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/x", http.StatusFound) })
		c := &Client{Endpoint: server.URL, APIKey: testKey, Model: DefaultModel}
		if _, err := c.Generate(context.Background(), request()); err == nil || len(rec.all()) != 1 {
			t.Fatalf("err %v requests %d", err, len(rec.all()))
		}
	})
	t.Run("oversized answer is cut and unreadable", func(t *testing.T) {
		server, _ := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
			respond(w, 200, `{"candidates":[{"content":{"parts":[{"text":"`+strings.Repeat("a", maxResponseBytes)+`"}]},"finishReason":"STOP"}]}`)
		})
		if _, err := client(server).Generate(context.Background(), request()); !errors.Is(err, ErrNoReply) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("not configured and invalid input make no request", func(t *testing.T) {
		server, rec := newServer(t, func(w http.ResponseWriter, _ *http.Request) { respond(w, 200, answer("STOP", `{"text":"ok"}`)) })
		c := client(server)
		c.APIKey = ""
		if _, err := c.Generate(context.Background(), request()); !errors.Is(err, ErrNotConfigured) {
			t.Error(err)
		}
		c = client(server)
		c.Model = "bad/model"
		if _, err := c.Generate(context.Background(), request()); !errors.Is(err, ErrInvalidModel) {
			t.Error(err)
		}
		in := request()
		in.MaxTokens = 0
		if _, err := client(server).Generate(context.Background(), in); err == nil {
			t.Error("zero max tokens accepted")
		}
		if n := len(rec.all()); n != 0 {
			t.Errorf("requests = %d", n)
		}
	})
	t.Run("spike thinking level option", func(t *testing.T) {
		server, rec := newServer(t, func(w http.ResponseWriter, _ *http.Request) { respond(w, 200, answer("STOP", `{"text":"ok"}`)) })
		c := client(server)
		c.Options.ThinkingLevel = "LOW"
		if _, err := c.Generate(context.Background(), request()); err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(rec.all()[0].Body, []byte(`"thinkingLevel":"LOW"`)) {
			t.Error("option not applied")
		}
	})
}
