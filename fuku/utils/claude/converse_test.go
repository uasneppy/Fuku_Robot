//go:build testtools

package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

const testConverseModel = "claude-haiku-5-5"

func converseRequest() ConverseRequest {
	return ConverseRequest{
		Static:    "STATIC PERSONA AND ARCHIVE",
		Dynamic:   "Group: Test group\nLanguage: Ukrainian",
		User:      "Resident: hello",
		MaxTokens: 2000,
	}
}

func converseAnswer(stop string, content string) string {
	return `{"id":"msg_01","type":"message","role":"assistant","model":"` + testConverseModel + `",` +
		`"content":[` + content + `],"stop_reason":"` + stop + `",` +
		`"usage":{"input_tokens":1500,"output_tokens":120,"cache_read_input_tokens":10000,"cache_creation_input_tokens":0}}`
}

func TestConverseRequestShape(t *testing.T) {
	server, rec := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, converseAnswer("end_turn", `{"type":"text","text":"Request denied."}`))
	})
	client := testClient(server, testConverseModel)

	reply, err := client.Converse(context.Background(), converseRequest())
	if err != nil {
		t.Fatalf("Converse: %v", err)
	}
	if reply.Text != "Request denied." || reply.Outcome != OutcomeAnswered {
		t.Fatalf("reply = %+v", reply)
	}
	if reply.InputTokens != 1500 || reply.OutputTokens != 120 || reply.CacheReadTokens != 10000 {
		t.Fatalf("usage = %+v", reply)
	}

	reqs := rec.all()
	if len(reqs) != 1 {
		t.Fatalf("requests = %d, want 1", len(reqs))
	}
	req := reqs[0]
	if req.Method != http.MethodPost || req.Path != "/v1/messages" || req.RawQuery != "" {
		t.Fatalf("request line = %s %s?%s", req.Method, req.Path, req.RawQuery)
	}
	if req.Header.Get("x-api-key") != testKey || req.Header.Get("anthropic-version") != APIVersion {
		t.Fatal("key or version header missing")
	}
	if req.Header.Get("anthropic-beta") != "" {
		t.Fatal("a Haiku model must get no beta header")
	}

	var body map[string]any
	if err := json.Unmarshal(req.Body, &body); err != nil {
		t.Fatal(err)
	}
	wantKeys := map[string]bool{"model": true, "max_tokens": true, "system": true, "messages": true, "output_config": true}
	for k := range body {
		if !wantKeys[k] {
			t.Errorf("unexpected request field %q", k)
		}
	}
	for k := range wantKeys {
		if _, ok := body[k]; !ok {
			t.Errorf("missing request field %q", k)
		}
	}
	if body["model"] != testConverseModel || body["max_tokens"] != float64(2000) {
		t.Errorf("model or max_tokens wrong: %v %v", body["model"], body["max_tokens"])
	}
	if effort := body["output_config"].(map[string]any)["effort"]; effort != "low" {
		t.Errorf("effort = %v", effort)
	}
	system := body["system"].([]any)
	if len(system) != 2 {
		t.Fatalf("system blocks = %d, want 2", len(system))
	}
	first, second := system[0].(map[string]any), system[1].(map[string]any)
	if first["text"] != "STATIC PERSONA AND ARCHIVE" || first["cache_control"].(map[string]any)["type"] != "ephemeral" {
		t.Errorf("first system block = %v", first)
	}
	if _, marked := second["cache_control"]; marked || second["text"] != converseRequest().Dynamic {
		t.Errorf("second system block = %v", second)
	}
	messages := body["messages"].([]any)
	if len(messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(messages))
	}
	msg := messages[0].(map[string]any)
	content := msg["content"].([]any)
	if msg["role"] != "user" || len(content) != 1 || content[0].(map[string]any)["text"] != "Resident: hello" {
		t.Errorf("message = %v", msg)
	}
	for _, banned := range []string{"thinking", "temperature", "top_p", "top_k", "tools", "tool_choice", "fallbacks"} {
		if bytes.Contains(req.Body, []byte(`"`+banned+`"`)) {
			t.Errorf("request carries %q", banned)
		}
	}
}

// TestConverseStaticBlockIsStable proves the cached prefix is byte-identical
// for two different chats and users, which is what makes the cache hit.
func TestConverseStaticBlockIsStable(t *testing.T) {
	server, rec := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, converseAnswer("end_turn", `{"type":"text","text":"ok"}`))
	})
	client := testClient(server, testConverseModel)
	a := converseRequest()
	b := ConverseRequest{Static: a.Static, Dynamic: "Group: Other\nLanguage: English", User: "Olena: hi", MaxTokens: 2000}
	for _, in := range []ConverseRequest{a, b} {
		if _, err := client.Converse(context.Background(), in); err != nil {
			t.Fatal(err)
		}
	}
	firstBlock := func(raw []byte) string {
		var body struct {
			System []json.RawMessage `json:"system"`
		}
		if err := json.Unmarshal(raw, &body); err != nil || len(body.System) == 0 {
			t.Fatalf("bad body: %v", err)
		}
		return string(body.System[0])
	}
	reqs := rec.all()
	if firstBlock(reqs[0].Body) != firstBlock(reqs[1].Body) {
		t.Fatal("the first system block differs between two chats")
	}
	if !strings.HasPrefix(string(reqs[0].Body), `{"model":"`+testConverseModel+`","max_tokens":2000,"system":[`+firstBlock(reqs[0].Body)) {
		t.Fatal("the static block is not the start of the system text")
	}
}

func TestConverseServerFallback(t *testing.T) {
	cases := map[string]bool{
		"claude-haiku-5-5":       false,
		"claude-haiku-4-5":       false,
		"claude-opus-5-5":        true,
		"claude-opus-5":          true,
		"claude-sonnet-5-5":      true,
		"claude-sonnet-5":        false,
		"claude-fable-5-2":       true,
		"claude-fable-5":         true,
		"gemini-4-hydrogen":      false,
		"some-claude-opus-5-alt": false,
	}
	for model, want := range cases {
		if got := ServerFallback(model); got != want {
			t.Errorf("ServerFallback(%q) = %v, want %v", model, got, want)
		}
	}

	server, rec := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, converseAnswer("end_turn",
			`{"type":"fallback","from":"claude-opus-5-5"},{"type":"text","text":"Fine."}`))
	})
	client := testClient(server, "claude-opus-5-5")
	reply, err := client.Converse(context.Background(), converseRequest())
	if err != nil || reply.Text != "Fine." {
		t.Fatalf("reply = %+v, err %v", reply, err)
	}
	req := rec.all()[0]
	if req.Header.Get("anthropic-beta") != fallbackBeta {
		t.Errorf("beta header = %q", req.Header.Get("anthropic-beta"))
	}
	if !bytes.Contains(req.Body, []byte(`"fallbacks":"default"`)) {
		t.Error("fallbacks field missing for an Opus model")
	}
}

func TestConverseStopReasons(t *testing.T) {
	cases := []struct {
		name    string
		answer  string
		outcome Outcome
		text    string
		noReply bool
	}{
		{"end_turn joins text and skips thinking", converseAnswer("end_turn",
			`{"type":"thinking","thinking":"secret"},{"type":"text","text":"Procedure "},{"type":"text","text":"complete."}`),
			OutcomeAnswered, "Procedure complete.", false},
		{"max_tokens with text", converseAnswer("max_tokens", `{"type":"text","text":"Half a sent"}`), OutcomeTruncated, "Half a sent", false},
		{"max_tokens blank", converseAnswer("max_tokens", `{"type":"thinking","thinking":"..."}`), 0, "", true},
		{"refusal", converseAnswer("refusal", `{"type":"text","text":"partial"}`), OutcomeRefused, "", false},
		{"blank end_turn", converseAnswer("end_turn", `{"type":"text","text":"   "}`), 0, "", true},
		{"no content", converseAnswer("end_turn", ``), 0, "", true},
		{"tool_use", converseAnswer("tool_use", `{"type":"text","text":"x"}`), 0, "", true},
		{"pause_turn", converseAnswer("pause_turn", `{"type":"text","text":"x"}`), 0, "", true},
		{"unknown", converseAnswer("something_new", `{"type":"text","text":"x"}`), 0, "", true},
		{"unreadable", `{not json`, 0, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server, _ := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
				writeJSON(w, http.StatusOK, tc.answer)
			})
			reply, err := testClient(server, testConverseModel).Converse(context.Background(), converseRequest())
			if tc.noReply {
				if !errors.Is(err, ErrNoReply) {
					t.Fatalf("err = %v, want ErrNoReply", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Converse: %v", err)
			}
			if reply.Outcome != tc.outcome || reply.Text != tc.text {
				t.Fatalf("reply = %+v", reply)
			}
		})
	}
}

func TestConverseErrors(t *testing.T) {
	t.Run("retries once on a short retryable wait", func(t *testing.T) {
		calls := 0
		server, _ := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
			calls++
			if calls == 1 {
				w.Header().Set("retry-after", "1")
				writeJSON(w, http.StatusTooManyRequests, errorBody("rate_limit_error", "slow down", "req_1"))
				return
			}
			writeJSON(w, http.StatusOK, converseAnswer("end_turn", `{"type":"text","text":"ok"}`))
		})
		reply, err := testClient(server, testConverseModel).Converse(context.Background(), converseRequest())
		if err != nil || reply.Text != "ok" || calls != 2 {
			t.Fatalf("reply %+v err %v calls %d", reply, err, calls)
		}
	})
	t.Run("long wait is not retried", func(t *testing.T) {
		server, rec := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("retry-after", "30")
			writeJSON(w, http.StatusTooManyRequests, errorBody("rate_limit_error", "slow down", "req_1"))
		})
		_, err := testClient(server, testConverseModel).Converse(context.Background(), converseRequest())
		var httpErr *HTTPError
		if !errors.As(err, &httpErr) || httpErr.RetryDelay != 30*time.Second || rec.count() != 1 {
			t.Fatalf("err %v count %d", err, rec.count())
		}
	})
	t.Run("rejected key", func(t *testing.T) {
		server, rec := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusUnauthorized, errorBody("authentication_error", "bad key "+testKey, "req_2"))
		})
		_, err := testClient(server, testConverseModel).Converse(context.Background(), converseRequest())
		var httpErr *HTTPError
		if !errors.As(err, &httpErr) || !httpErr.KeyRejected() || httpErr.Retryable() || rec.count() != 1 {
			t.Fatalf("err %v", err)
		}
		if strings.Contains(err.Error(), testKey) || strings.Contains(httpErr.Message, testKey) {
			t.Fatal("the key leaked into the error")
		}
	})
	t.Run("transport error carries no key or URL", func(t *testing.T) {
		client := &Client{Endpoint: "http://127.0.0.1:1", APIKey: testKey, Model: testConverseModel}
		_, err := client.Converse(context.Background(), converseRequest())
		if err == nil || strings.Contains(err.Error(), testKey) || strings.Contains(err.Error(), "127.0.0.1:1/v1") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("not configured and invalid input make no request", func(t *testing.T) {
		server, rec := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusOK, converseAnswer("end_turn", `{"type":"text","text":"ok"}`))
		})
		noKey := testClient(server, testConverseModel)
		noKey.APIKey = ""
		if _, err := noKey.Converse(context.Background(), converseRequest()); !errors.Is(err, ErrNotConfigured) {
			t.Errorf("no key: %v", err)
		}
		if _, err := testClient(server, "bad model!").Converse(context.Background(), converseRequest()); !errors.Is(err, ErrInvalidModel) {
			t.Errorf("bad model: %v", err)
		}
		empty := converseRequest()
		empty.User = " "
		if _, err := testClient(server, testConverseModel).Converse(context.Background(), empty); err == nil {
			t.Error("blank user text accepted")
		}
		if rec.count() != 0 {
			t.Errorf("requests = %d, want 0", rec.count())
		}
	})
	t.Run("redirect is not followed", func(t *testing.T) {
		server, rec := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/elsewhere", http.StatusFound)
		})
		client := &Client{Endpoint: server.URL, APIKey: testKey, Model: testConverseModel}
		if _, err := client.Converse(context.Background(), converseRequest()); err == nil {
			t.Fatal("redirect accepted")
		}
		if rec.count() != 1 {
			t.Fatalf("requests = %d, want 1", rec.count())
		}
	})
}
