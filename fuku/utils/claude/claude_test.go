//go:build testtools

package claude

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// testKey is shaped like a real key and obviously fake.
const testKey = "sk-ant-api03-FAKEFAKEFAKEFAKEFAKEFAKEFAKEFAKEFAKEFAKE0123456789"

const testModel = "test-vision-lite"

// recordedRequest is what the test server saw of one request.
type recordedRequest struct {
	Method   string
	Path     string
	RawQuery string
	Header   http.Header
	Body     []byte
}

// recorder collects every request a test server receives.
type recorder struct {
	mu   sync.Mutex
	reqs []recordedRequest
}

func (r *recorder) add(req *http.Request) {
	body, _ := io.ReadAll(req.Body)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reqs = append(r.reqs, recordedRequest{
		Method:   req.Method,
		Path:     req.URL.EscapedPath(),
		RawQuery: req.URL.RawQuery,
		Header:   req.Header.Clone(),
		Body:     body,
	})
}

func (r *recorder) all() []recordedRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]recordedRequest, len(r.reqs))
	copy(out, r.reqs)
	return out
}

func (r *recorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.reqs)
}

// newTestServer starts an httptest server that records every request and then
// hands it to handler. It is closed when the test ends.
func newTestServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *recorder) {
	t.Helper()
	rec := &recorder{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.add(r)
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	return server, rec
}

func testClient(server *httptest.Server, model string) *Client {
	return &Client{
		Endpoint: server.URL,
		APIKey:   testKey,
		Model:    model,
		HTTP:     server.Client(),
	}
}

func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

// toolAnswer builds a forced-tool answer whose input is input.
func toolAnswer(input string) string {
	return `{"id":"msg_01","type":"message","role":"assistant","model":"` + testModel + `",` +
		`"content":[{"type":"tool_use","id":"toolu_01","name":"` + ToolName + `","input":` + input + `}],` +
		`"stop_reason":"tool_use","stop_details":null,"usage":{"input_tokens":790,"output_tokens":21}}`
}

const goodAnswer = `{"spam_probability":0.93}`

func errorBody(errType, message, requestID string) string {
	return `{"type":"error","error":{"type":"` + errType + `","message":"` + message + `"},"request_id":"` + requestID + `"}`
}

var jpegBytes = []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0x01, 0x02}

func classifyOnce(client *Client) (*Verdict, error) {
	return client.Classify(context.Background(), Image{MIMEType: "image/jpeg", Data: jpegBytes})
}

func TestClaudeRequestShape(t *testing.T) {
	t.Run("sends the verified request", func(t *testing.T) {
		server, rec := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusOK, toolAnswer(goodAnswer))
		})
		client := testClient(server, testModel)

		verdict, err := classifyOnce(client)
		if err != nil {
			t.Fatalf("Classify: %v", err)
		}
		if verdict.SpamProbability != 0.93 {
			t.Fatalf("probability = %v, want 0.93", verdict.SpamProbability)
		}

		reqs := rec.all()
		if len(reqs) != 1 {
			t.Fatalf("requests = %d, want exactly 1", len(reqs))
		}
		req := reqs[0]
		if req.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", req.Method)
		}
		if req.Path != "/v1/messages" {
			t.Fatalf("path = %s", req.Path)
		}
		if got := req.Header.Get("x-api-key"); got != testKey {
			t.Fatalf("x-api-key = %q, want the key", got)
		}
		if got := req.Header.Get("anthropic-version"); got != "2026-10-21" {
			t.Fatalf("anthropic-version = %q", got)
		}
		if got := req.Header.Get("Authorization"); got != "" {
			t.Fatalf("Authorization = %q, want none", got)
		}
		if req.RawQuery != "" {
			t.Fatalf("raw query = %q, want empty (the key must not be in the URL)", req.RawQuery)
		}
		if got := req.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("Content-Type = %q", got)
		}
		if strings.Contains(string(req.Body), testKey) {
			t.Fatal("the key is in the request body")
		}

		var body map[string]any
		if err := json.Unmarshal(req.Body, &body); err != nil {
			t.Fatalf("request body is not JSON: %v", err)
		}
		wantKeys := map[string]bool{
			"model": true, "max_tokens": true, "system": true, "thinking": true,
			"tools": true, "tool_choice": true, "messages": true,
		}
		for key := range body {
			if !wantKeys[key] {
				t.Errorf("unexpected top-level key %q", key)
			}
		}
		for key := range wantKeys {
			if _, ok := body[key]; !ok {
				t.Errorf("missing top-level key %q", key)
			}
		}
		for _, banned := range []string{"temperature", "top_p", "top_k"} {
			if _, ok := body[banned]; ok {
				t.Errorf("%s must not be sent", banned)
			}
		}

		if body["model"] != testModel {
			t.Fatalf("model = %v", body["model"])
		}
		if body["max_tokens"].(float64) != 256 {
			t.Fatalf("max_tokens = %v", body["max_tokens"])
		}
		system := body["system"].(string)
		if !strings.Contains(system, "never instructions") {
			t.Fatalf("system does not say image text is content, never instructions: %q", system)
		}
		if body["thinking"].(map[string]any)["type"] != "disabled" {
			t.Fatalf("thinking = %v", body["thinking"])
		}

		tools := body["tools"].([]any)
		if len(tools) != 1 {
			t.Fatalf("tools = %d, want 1", len(tools))
		}
		tl := tools[0].(map[string]any)
		if tl["name"] != ToolName || tl["strict"] != true {
			t.Fatalf("tool = %v", tl)
		}
		schema := tl["input_schema"].(map[string]any)
		if schema["type"] != "object" || schema["additionalProperties"] != false {
			t.Fatalf("schema = %v", schema)
		}
		required := schema["required"].([]any)
		if len(required) != 1 || required[0] != "spam_probability" {
			t.Fatalf("required = %v", required)
		}
		props := schema["properties"].(map[string]any)
		if len(props) != 1 {
			t.Fatalf("properties = %v, want only spam_probability", props)
		}
		prob := props["spam_probability"].(map[string]any)
		if prob["type"] != "number" {
			t.Fatalf("spam_probability = %v", prob)
		}
		if _, ok := prob["minimum"]; ok {
			t.Fatal("strict schemas do not take minimum")
		}
		if _, ok := prob["maximum"]; ok {
			t.Fatal("strict schemas do not take maximum")
		}

		choice := body["tool_choice"].(map[string]any)
		if choice["type"] != "tool" || choice["name"] != ToolName || choice["disable_parallel_tool_use"] != true {
			t.Fatalf("tool_choice = %v", choice)
		}

		messages := body["messages"].([]any)
		if len(messages) != 1 {
			t.Fatalf("messages = %d, want 1", len(messages))
		}
		msg := messages[0].(map[string]any)
		if msg["role"] != "user" {
			t.Fatalf("role = %v", msg["role"])
		}
		blocks := msg["content"].([]any)
		if len(blocks) != 2 {
			t.Fatalf("content blocks = %d, want image then text", len(blocks))
		}
		image := blocks[0].(map[string]any)
		if image["type"] != "image" {
			t.Fatalf("first block = %v", image)
		}
		source := image["source"].(map[string]any)
		if source["type"] != "base64" || source["media_type"] != "image/jpeg" {
			t.Fatalf("source = %v", source)
		}
		decoded, err := base64.StdEncoding.DecodeString(source["data"].(string))
		if err != nil || string(decoded) != string(jpegBytes) {
			t.Fatalf("image data does not decode back to the bytes (err %v)", err)
		}
		text := blocks[1].(map[string]any)
		if text["type"] != "text" || text["text"] != "Classify this image." {
			t.Fatalf("second block = %v", text)
		}
	})

	t.Run("refuses bad input before any request", func(t *testing.T) {
		server, rec := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusOK, toolAnswer(goodAnswer))
		})
		good := Image{MIMEType: "image/jpeg", Data: jpegBytes}
		noKey := testClient(server, testModel)
		noKey.APIKey = ""
		cases := []struct {
			name   string
			client *Client
			img    Image
			want   error
		}{
			{"model with a slash", testClient(server, "x/../../y"), good, ErrInvalidModel},
			{"model over 64 characters", testClient(server, strings.Repeat("a", 65)), good, ErrInvalidModel},
			{"empty model", testClient(server, ""), good, ErrInvalidModel},
			{"empty key", noKey, good, ErrNotConfigured},
			{"gif", testClient(server, testModel), Image{MIMEType: "image/gif", Data: jpegBytes}, ErrUnsupportedImage},
			{"empty data", testClient(server, testModel), Image{MIMEType: "image/jpeg"}, ErrUnsupportedImage},
			{"over 5 MiB", testClient(server, testModel), Image{MIMEType: "image/jpeg", Data: make([]byte, MaxImageBytes+1)}, ErrUnsupportedImage},
		}
		for _, tc := range cases {
			verdict, err := tc.client.Classify(context.Background(), tc.img)
			if verdict != nil {
				t.Errorf("%s: got a verdict", tc.name)
			}
			if !errors.Is(err, tc.want) {
				t.Errorf("%s: err = %v, want %v", tc.name, err, tc.want)
			}
		}
		if n := rec.count(); n != 0 {
			t.Fatalf("a request was sent for refused input: %d", n)
		}
	})

	t.Run("accepts the 5 MiB boundary", func(t *testing.T) {
		server, rec := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusOK, toolAnswer(goodAnswer))
		})
		client := testClient(server, testModel)
		if _, err := client.Classify(context.Background(), Image{MIMEType: "image/png", Data: make([]byte, MaxImageBytes)}); err != nil {
			t.Fatalf("Classify at the limit: %v", err)
		}
		if rec.count() != 1 {
			t.Fatalf("requests = %d, want 1", rec.count())
		}
	})
}

func TestClaudeValidModel(t *testing.T) {
	valid := []string{DefaultModel, "a.b-c_9", "A", "9", strings.Repeat("a", 64)}
	for _, name := range valid {
		if !ValidModel(name) {
			t.Errorf("ValidModel(%q) = false, want true", name)
		}
	}
	invalid := []string{"", ".", "..", "../x", "a/b", "a b", "a:b", "a?x=1", "a%2f", "-a", "_a", strings.Repeat("a", 65), "модель"}
	for _, name := range invalid {
		if ValidModel(name) {
			t.Errorf("ValidModel(%q) = true, want false", name)
		}
	}
}

// classifyWith runs Classify against a server answering status and body and
// requires a verdict.
func classifyWith(t *testing.T, status int, body string) *Verdict {
	t.Helper()
	server, _ := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, status, body)
	})
	verdict, err := classifyOnce(testClient(server, testModel))
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if verdict == nil {
		t.Fatal("Classify returned neither a verdict nor an error")
	}
	return verdict
}

// classifyError runs Classify and requires an error and no verdict.
func classifyError(t *testing.T, status int, body string) error {
	t.Helper()
	server, _ := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, status, body)
	})
	verdict, err := classifyOnce(testClient(server, testModel))
	if verdict != nil {
		t.Fatalf("got a verdict %+v, want none", verdict)
	}
	if err == nil {
		t.Fatal("Classify returned no error and no verdict")
	}
	return err
}

func TestClaudeVerdictParse(t *testing.T) {
	t.Run("a tool_use answer gives a verdict", func(t *testing.T) {
		verdict := classifyWith(t, http.StatusOK, toolAnswer(`{"spam_probability":0.9}`))
		if verdict.SpamProbability != 0.9 {
			t.Fatalf("verdict = %+v", verdict)
		}
		if verdict.InputTokens != 790 || verdict.OutputTokens != 21 {
			t.Fatalf("tokens = %d/%d", verdict.InputTokens, verdict.OutputTokens)
		}
		if verdict.Model != testModel {
			t.Fatalf("model = %q", verdict.Model)
		}
	})

	t.Run("a thinking block before the call is skipped", func(t *testing.T) {
		body := `{"model":"m","content":[{"type":"thinking","thinking":"hmm","signature":"x"},` +
			`{"type":"text","text":"ok"},` +
			`{"type":"tool_use","id":"t","name":"` + ToolName + `","input":{"spam_probability":0.02}}],"stop_reason":"tool_use"}`
		verdict := classifyWith(t, http.StatusOK, body)
		if verdict.SpamProbability != 0.02 {
			t.Fatalf("verdict = %+v", verdict)
		}
	})

	t.Run("the bounds 0 and 1 are valid", func(t *testing.T) {
		for _, p := range []string{"0", "1", "0.0", "1.0"} {
			if verdict := classifyWith(t, http.StatusOK, toolAnswer(`{"spam_probability":`+p+`}`)); verdict == nil {
				t.Fatalf("probability %s gave no verdict", p)
			}
		}
	})

	noVerdict := []struct {
		name string
		body string
	}{
		{"missing probability", toolAnswer(`{}`)},
		{"probability above 1", toolAnswer(`{"spam_probability":1.5}`)},
		{"negative probability", toolAnswer(`{"spam_probability":-0.1}`)},
		{"probability as a string", toolAnswer(`{"spam_probability":"0.9"}`)},
		{"probability beyond float range", toolAnswer(`{"spam_probability":1e999}`)},
		{"probability null", toolAnswer(`{"spam_probability":null}`)},
		{"input not an object", toolAnswer(`"0.9"`)},
		{"max_tokens", `{"content":[{"type":"tool_use","name":"` + ToolName + `","input":{"spam_probability":0.9}}],"stop_reason":"max_tokens"}`},
		{"end_turn with text only", `{"content":[{"type":"text","text":"spam"}],"stop_reason":"end_turn"}`},
		{"pause_turn", `{"content":[],"stop_reason":"pause_turn"}`},
		{"no stop reason", `{"content":[{"type":"tool_use","name":"` + ToolName + `","input":{"spam_probability":0.9}}]}`},
		{"another tool name", `{"content":[{"type":"tool_use","id":"t","name":"other_tool","input":{"spam_probability":0.9}}],"stop_reason":"tool_use"}`},
		{"empty content", `{"content":[],"stop_reason":"tool_use"}`},
		{"unreadable JSON", `<html>not json</html>`},
	}
	for _, tc := range noVerdict {
		t.Run("no verdict: "+tc.name, func(t *testing.T) {
			err := classifyError(t, http.StatusOK, tc.body)
			if !errors.Is(err, ErrNoVerdict) {
				t.Fatalf("err = %v, want ErrNoVerdict", err)
			}
			if errors.Is(err, ErrBlocked) {
				t.Fatalf("err = %v, a plain no-verdict is not a block", err)
			}
		})
	}

	blocked := []struct {
		name string
		body string
	}{
		{"refusal with a category", `{"content":[],"stop_reason":"refusal","stop_details":{"type":"refusal","category":"general_harms","explanation":"no"}}`},
		{"refusal with a null category", `{"content":[],"stop_reason":"refusal","stop_details":{"type":"refusal","category":null}}`},
		{"refusal without details", `{"content":[],"stop_reason":"refusal"}`},
	}
	for _, tc := range blocked {
		t.Run("blocked: "+tc.name, func(t *testing.T) {
			err := classifyError(t, http.StatusOK, tc.body)
			if !errors.Is(err, ErrBlocked) {
				t.Fatalf("err = %v, want ErrBlocked", err)
			}
			if !errors.Is(err, ErrNoVerdict) {
				t.Fatalf("err = %v, ErrBlocked must also be ErrNoVerdict", err)
			}
		})
	}

	t.Run("a long refusal category is bounded", func(t *testing.T) {
		body := `{"content":[],"stop_reason":"refusal","stop_details":{"category":"` + strings.Repeat("x", 500) + `"}}`
		err := classifyError(t, http.StatusOK, body)
		if len(err.Error()) > 200 {
			t.Fatalf("error text is %d bytes", len(err.Error()))
		}
	})
}

func rateLimitBody() string {
	return errorBody("rate_limit_error", "slow down", "req_01")
}

func TestClaudeRetryAndErrors(t *testing.T) {
	t.Run("429 with a short retry-after is retried once", func(t *testing.T) {
		var calls int
		server, rec := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
			calls++
			if calls == 1 {
				w.Header().Set("retry-after", "1")
				writeJSON(w, http.StatusTooManyRequests, rateLimitBody())
				return
			}
			writeJSON(w, http.StatusOK, toolAnswer(goodAnswer))
		})
		verdict, err := classifyOnce(testClient(server, testModel))
		if err != nil {
			t.Fatalf("Classify: %v", err)
		}
		if verdict.SpamProbability != 0.93 {
			t.Fatalf("verdict = %+v", verdict)
		}
		if rec.count() != 2 {
			t.Fatalf("requests = %d, want 2", rec.count())
		}
	})

	t.Run("429 with a long retry-after is not retried", func(t *testing.T) {
		server, rec := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("retry-after", "30")
			writeJSON(w, http.StatusTooManyRequests, rateLimitBody())
		})
		_, err := classifyOnce(testClient(server, testModel))
		var httpErr *HTTPError
		if !errors.As(err, &httpErr) {
			t.Fatalf("err = %v, want *HTTPError", err)
		}
		if httpErr.Status != 429 || httpErr.Type != "rate_limit_error" || httpErr.RetryDelay != 30*time.Second {
			t.Fatalf("HTTPError = %+v", httpErr)
		}
		if rec.count() != 1 {
			t.Fatalf("requests = %d, want 1", rec.count())
		}
		if !httpErr.Retryable() {
			t.Fatal("429 must be retryable")
		}
	})

	t.Run("429 and 529 without retry-after are retried once after the default wait", func(t *testing.T) {
		for _, status := range []int{429, 529} {
			var calls int
			var firstAt time.Time
			var gap time.Duration
			server, rec := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
				calls++
				if calls == 1 {
					firstAt = time.Now()
					writeJSON(w, status, errorBody("overloaded_error", "busy", "req_02"))
					return
				}
				gap = time.Since(firstAt)
				writeJSON(w, http.StatusOK, toolAnswer(goodAnswer))
			})
			if _, err := classifyOnce(testClient(server, testModel)); err != nil {
				t.Fatalf("status %d: Classify: %v", status, err)
			}
			if rec.count() != 2 {
				t.Fatalf("status %d: requests = %d, want 2", status, rec.count())
			}
			if gap < 400*time.Millisecond {
				t.Fatalf("status %d: retried after %v, want about 500ms", status, gap)
			}
		}
	})

	t.Run("two 529s make two requests and an overloaded HTTPError", func(t *testing.T) {
		server, rec := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, 529, errorBody("overloaded_error", "busy", "req_03"))
		})
		_, err := classifyOnce(testClient(server, testModel))
		var httpErr *HTTPError
		if !errors.As(err, &httpErr) || httpErr.Status != 529 || httpErr.Type != "overloaded_error" {
			t.Fatalf("err = %v, want a 529 overloaded_error HTTPError", err)
		}
		if rec.count() != 2 {
			t.Fatalf("requests = %d, want 2 (one retry only)", rec.count())
		}
		if !httpErr.Retryable() || httpErr.KeyRejected() {
			t.Fatalf("a 529 is retryable and not a key problem: %+v", httpErr)
		}
		if strings.Contains(httpErr.Error(), "busy") || strings.Contains(httpErr.Error(), "req_03") {
			t.Fatalf("Error() holds the message or request id: %q", httpErr.Error())
		}
	})

	t.Run("a long retry-after on a 5xx is not retried", func(t *testing.T) {
		server, rec := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("retry-after", "60")
			writeJSON(w, http.StatusServiceUnavailable, errorBody("api_error", "down", "req_04"))
		})
		_, err := classifyOnce(testClient(server, testModel))
		var httpErr *HTTPError
		if !errors.As(err, &httpErr) || httpErr.Status != 503 {
			t.Fatalf("err = %v", err)
		}
		if rec.count() != 1 {
			t.Fatalf("requests = %d, want 1", rec.count())
		}
	})

	for _, tc := range []struct {
		status  int
		errType string
	}{
		{401, "authentication_error"},
		{402, "billing_error"},
		{403, "permission_error"},
	} {
		t.Run(fmt.Sprintf("a %d is final and rejects the key", tc.status), func(t *testing.T) {
			server, rec := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
				writeJSON(w, tc.status, errorBody(tc.errType, "nope", "req_05"))
			})
			_, err := classifyOnce(testClient(server, testModel))
			var httpErr *HTTPError
			if !errors.As(err, &httpErr) {
				t.Fatalf("err = %v", err)
			}
			if !httpErr.KeyRejected() || httpErr.Retryable() || httpErr.Type != tc.errType {
				t.Fatalf("HTTPError = %+v", httpErr)
			}
			if rec.count() != 1 {
				t.Fatalf("requests = %d, want 1", rec.count())
			}
		})
	}

	t.Run("an error type alone rejects the key", func(t *testing.T) {
		for _, errType := range []string{"authentication_error", "billing_error", "permission_error"} {
			if !(&HTTPError{Status: 400, Type: errType}).KeyRejected() {
				t.Errorf("%s did not reject the key", errType)
			}
		}
	})

	t.Run("a plain 400 does not reject the key", func(t *testing.T) {
		server, rec := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusBadRequest, errorBody("invalid_request_error", "bad field", "req_06"))
		})
		_, err := classifyOnce(testClient(server, testModel))
		var httpErr *HTTPError
		if !errors.As(err, &httpErr) {
			t.Fatalf("err = %v", err)
		}
		if httpErr.KeyRejected() || httpErr.Retryable() {
			t.Fatalf("a 400 must be final and not a key problem: %+v", httpErr)
		}
		if httpErr.Message != "bad field" || httpErr.RequestID != "req_06" {
			t.Fatalf("HTTPError = %+v", httpErr)
		}
		if rec.count() != 1 {
			t.Fatalf("requests = %d, want 1", rec.count())
		}
	})

	t.Run("the request-id header is the fallback id", func(t *testing.T) {
		server, _ := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("request-id", "req_hdr")
			writeJSON(w, http.StatusNotFound, `{"type":"error","error":{"type":"not_found_error","message":"no model"}}`)
		})
		_, err := classifyOnce(testClient(server, testModel))
		var httpErr *HTTPError
		if !errors.As(err, &httpErr) || httpErr.Status != 404 || httpErr.Type != "not_found_error" || httpErr.RequestID != "req_hdr" {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("an answer that is not an error envelope is still an HTTPError", func(t *testing.T) {
		server, _ := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusBadGateway, `<html>bad gateway</html>`)
		})
		_, err := classifyOnce(testClient(server, testModel))
		var httpErr *HTTPError
		if !errors.As(err, &httpErr) || httpErr.Status != 502 {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("a redirect is not followed", func(t *testing.T) {
		var other int
		target, _ := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
			other++
			writeJSON(w, http.StatusOK, toolAnswer(goodAnswer))
		})
		server, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, target.URL+"/v1/messages", http.StatusFound)
		})
		// A nil HTTP field uses the package client; point it at the redirecting
		// server through the endpoint.
		client := &Client{Endpoint: server.URL, APIKey: testKey, Model: testModel}
		_, err := classifyOnce(client)
		var httpErr *HTTPError
		if !errors.As(err, &httpErr) || httpErr.Status != http.StatusFound {
			t.Fatalf("err = %v, want a 302 HTTPError", err)
		}
		if other != 0 {
			t.Fatalf("the redirect target was called %d times", other)
		}
	})

	t.Run("a hung server ends at the caller's deadline", func(t *testing.T) {
		server, _ := newTestServer(t, func(_ http.ResponseWriter, r *http.Request) {
			<-r.Context().Done()
		})
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()

		start := time.Now()
		verdict, err := testClient(server, testModel).Classify(ctx, Image{MIMEType: "image/jpeg", Data: jpegBytes})
		if verdict != nil || err == nil {
			t.Fatalf("verdict = %v err = %v, want an error", verdict, err)
		}
		if elapsed := time.Since(start); elapsed > time.Second {
			t.Fatalf("took %v, want under 1s", elapsed)
		}
	})

	t.Run("Retryable is true for 429 and 5xx only", func(t *testing.T) {
		for status, want := range map[int]bool{429: true, 500: true, 503: true, 529: true, 400: false, 401: false, 402: false, 403: false, 404: false} {
			if got := (&HTTPError{Status: status}).Retryable(); got != want {
				t.Errorf("Retryable(%d) = %v, want %v", status, got, want)
			}
		}
	})

	t.Run("retry-after as an HTTP date is read", func(t *testing.T) {
		at := time.Now().Add(90 * time.Second).UTC().Format(http.TimeFormat)
		if d := parseRetryAfter(at); d < 60*time.Second || d > 91*time.Second {
			t.Fatalf("delay = %v", d)
		}
		if parseRetryAfter("soon") != 0 || parseRetryAfter("-3") != 0 || parseRetryAfter("") != 0 {
			t.Fatal("unreadable values must be no delay")
		}
	})
}

func TestClaudeErrorsNeverContainKey(t *testing.T) {
	assertClean := func(t *testing.T, name string, err error, extra ...string) {
		t.Helper()
		if err == nil {
			t.Fatalf("%s: no error", name)
		}
		texts := []string{err.Error(), fmt.Sprintf("%v", err), fmt.Sprintf("%+v", err), fmt.Sprintf("%#v", err)}
		var httpErr *HTTPError
		if errors.As(err, &httpErr) {
			texts = append(texts, httpErr.Message, httpErr.Type, httpErr.RequestID, fmt.Sprintf("%+v", *httpErr))
		}
		for _, text := range texts {
			if strings.Contains(text, testKey) {
				t.Errorf("%s: error text contains the key: %q", name, text)
			}
			for _, banned := range extra {
				if strings.Contains(text, banned) {
					t.Errorf("%s: error text contains %q: %q", name, banned, text)
				}
			}
		}
	}

	t.Run("a 400 whose message echoes the key", func(t *testing.T) {
		server, _ := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusBadRequest, errorBody("invalid_request_error", "key "+testKey+" bad, pass "+testKey, "req_07"))
		})
		_, err := classifyOnce(testClient(server, testModel))
		assertClean(t, "echoed key", err)
		var httpErr *HTTPError
		if !errors.As(err, &httpErr) || !strings.Contains(httpErr.Message, "[REDACTED]") {
			t.Fatalf("Message should keep the text with the key replaced, got %+v", httpErr)
		}
	})

	t.Run("a 500 whose body is the key repeated", func(t *testing.T) {
		server, _ := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusInternalServerError, strings.Repeat(testKey, 20))
		})
		_, err := classifyOnce(testClient(server, testModel))
		assertClean(t, "key body", err)
	})

	t.Run("a key echoed in the error type and request id", func(t *testing.T) {
		server, _ := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("request-id", testKey)
			writeJSON(w, http.StatusBadRequest, errorBody(testKey, "x", testKey))
		})
		_, err := classifyOnce(testClient(server, testModel))
		assertClean(t, "key in type and id", err)
	})

	t.Run("a key echoed in the request-id header only", func(t *testing.T) {
		server, _ := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("request-id", testKey)
			writeJSON(w, http.StatusBadRequest, `<html>no envelope</html>`)
		})
		_, err := classifyOnce(testClient(server, testModel))
		assertClean(t, "key in header id", err)
	})

	t.Run("a closed server", func(t *testing.T) {
		server, _ := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusOK, toolAnswer(goodAnswer))
		})
		client := testClient(server, testModel)
		serverURL := server.URL
		server.Close()

		_, err := classifyOnce(client)
		assertClean(t, "closed server", err, serverURL, "/v1/messages")
	})

	t.Run("a hung server", func(t *testing.T) {
		server, _ := newTestServer(t, func(_ http.ResponseWriter, r *http.Request) {
			<-r.Context().Done()
		})
		ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
		defer cancel()
		_, err := testClient(server, testModel).Classify(ctx, Image{MIMEType: "image/jpeg", Data: jpegBytes})
		assertClean(t, "hung server", err, server.URL, "/v1/messages")
	})

	t.Run("an endpoint that cannot be parsed", func(t *testing.T) {
		client := &Client{Endpoint: "http://bad host\x7f/" + testKey, APIKey: testKey, Model: testModel}
		_, err := classifyOnce(client)
		assertClean(t, "unbuildable request", err, "bad host")
	})

	t.Run("a key echoed in a transport error", func(t *testing.T) {
		client := &Client{
			Endpoint: "http://example.invalid",
			APIKey:   testKey,
			Model:    testModel,
			HTTP: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, errors.New("dial failed for " + testKey)
			})},
		}
		_, err := classifyOnce(client)
		assertClean(t, "transport echo", err, "example.invalid")
	})
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// requestBodyWith classifies one image with options and returns the decoded
// request body the server saw.
func requestBodyWith(t *testing.T, opts Options) map[string]any {
	t.Helper()
	server, rec := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, toolAnswer(goodAnswer))
	})
	client := testClient(server, testModel)
	client.Options = opts
	if _, err := classifyOnce(client); err != nil {
		t.Fatalf("Classify with %+v: %v", opts, err)
	}
	reqs := rec.all()
	if len(reqs) != 1 {
		t.Fatalf("requests = %d, want 1", len(reqs))
	}
	var body map[string]any
	if err := json.Unmarshal(reqs[0].Body, &body); err != nil {
		t.Fatalf("request body is not JSON: %v", err)
	}
	return body
}

// changedKeys lists the top-level keys whose value differs between two bodies,
// including a key present in only one of them.
func changedKeys(a, b map[string]any) []string {
	seen := map[string]bool{}
	var out []string
	for key := range a {
		seen[key] = true
		if fmt.Sprint(a[key]) != fmt.Sprint(b[key]) {
			out = append(out, key)
		}
	}
	for key := range b {
		if !seen[key] {
			out = append(out, key)
		}
	}
	return out
}

func TestClaudeSpikeOptionsChangeOnlyTheirOwnField(t *testing.T) {
	production := requestBodyWith(t, Options{})

	cases := []struct {
		name   string
		opts   Options
		field  string
		verify func(t *testing.T, body map[string]any)
	}{
		{"auto tool choice", Options{ToolChoice: "auto"}, "tool_choice", func(t *testing.T, body map[string]any) {
			choice := body["tool_choice"].(map[string]any)
			if choice["type"] != "auto" || len(choice) != 1 {
				t.Errorf("tool_choice = %v, want only type auto", choice)
			}
			if len(body["tools"].([]any)) != 1 {
				t.Error("the auto request must still carry the tool")
			}
		}},
		{"adaptive thinking", Options{Thinking: "adaptive"}, "thinking", func(t *testing.T, body map[string]any) {
			if body["thinking"].(map[string]any)["type"] != "adaptive" {
				t.Errorf("thinking = %v", body["thinking"])
			}
		}},
		{"thinking omitted", Options{Thinking: "omit"}, "thinking", func(t *testing.T, body map[string]any) {
			if _, ok := body["thinking"]; ok {
				t.Errorf("thinking = %v, want no field", body["thinking"])
			}
		}},
		{"effort", Options{Effort: "low"}, "output_config", func(t *testing.T, body map[string]any) {
			if body["output_config"].(map[string]any)["effort"] != "low" {
				t.Errorf("output_config = %v", body["output_config"])
			}
		}},
		{"max tokens", Options{MaxTokens: 1024}, "max_tokens", func(t *testing.T, body map[string]any) {
			if body["max_tokens"].(float64) != 1024 {
				t.Errorf("max_tokens = %v", body["max_tokens"])
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := requestBodyWith(t, tc.opts)
			changed := changedKeys(production, body)
			if len(changed) != 1 || changed[0] != tc.field {
				t.Errorf("changed keys = %v, want only %s", changed, tc.field)
			}
			tc.verify(t, body)
			for _, banned := range []string{"temperature", "top_p", "top_k"} {
				if _, ok := body[banned]; ok {
					t.Errorf("%s must not be sent", banned)
				}
			}
		})
	}

	t.Run("zero options are the production request", func(t *testing.T) {
		if got := production["tool_choice"].(map[string]any); got["type"] != "tool" || got["name"] != ToolName || got["disable_parallel_tool_use"] != true {
			t.Errorf("tool_choice = %v", got)
		}
		if production["thinking"].(map[string]any)["type"] != "disabled" {
			t.Errorf("thinking = %v", production["thinking"])
		}
		if _, ok := production["output_config"]; ok {
			t.Error("output_config must not be sent by default")
		}
		if production["max_tokens"].(float64) != 256 {
			t.Errorf("max_tokens = %v", production["max_tokens"])
		}
	})
}

func TestClaudeGetModel(t *testing.T) {
	t.Run("reads the metadata", func(t *testing.T) {
		server, rec := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusOK, `{"id":"`+testModel+`","type":"model","display_name":"Test Vision Lite","max_input_tokens":200000,"max_tokens":64000}`)
		})
		info, err := testClient(server, testModel).GetModel(context.Background())
		if err != nil {
			t.Fatalf("GetModel: %v", err)
		}
		if info.ID != testModel || info.DisplayName != "Test Vision Lite" || info.MaxInputTokens != 200000 || info.MaxTokens != 64000 {
			t.Errorf("info = %+v", info)
		}
		reqs := rec.all()
		if len(reqs) != 1 {
			t.Fatalf("requests = %d, want 1", len(reqs))
		}
		req := reqs[0]
		if req.Method != http.MethodGet || req.Path != "/v1/models/"+testModel {
			t.Errorf("request = %s %s", req.Method, req.Path)
		}
		if len(req.Body) != 0 {
			t.Errorf("body = %q, want none", req.Body)
		}
		if got := req.Header.Get("Content-Type"); got != "" {
			t.Errorf("Content-Type = %q, want none on a GET", got)
		}
		if req.Header.Get("x-api-key") != testKey || req.Header.Get("anthropic-version") != APIVersion {
			t.Error("the key and version headers are missing")
		}
		if req.RawQuery != "" {
			t.Errorf("raw query = %q", req.RawQuery)
		}
	})

	t.Run("a 404 is an HTTPError", func(t *testing.T) {
		server, _ := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusNotFound, errorBody("not_found_error", "model: nope", "req_1"))
		})
		_, err := testClient(server, testModel).GetModel(context.Background())
		var httpErr *HTTPError
		if !errors.As(err, &httpErr) || httpErr.Status != http.StatusNotFound || httpErr.Type != "not_found_error" {
			t.Fatalf("err = %v, want a 404 not_found_error HTTPError", err)
		}
	})

	t.Run("unreadable metadata is a fixed error", func(t *testing.T) {
		server, _ := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusOK, `not json `+testKey)
		})
		_, err := testClient(server, testModel).GetModel(context.Background())
		if err == nil || strings.Contains(err.Error(), testKey) {
			t.Fatalf("err = %v, want a keyless error", err)
		}
	})

	t.Run("no key or a bad model makes no request", func(t *testing.T) {
		server, rec := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusOK, `{}`)
		})
		noKey := testClient(server, testModel)
		noKey.APIKey = ""
		if _, err := noKey.GetModel(context.Background()); !errors.Is(err, ErrNotConfigured) {
			t.Errorf("no key: err = %v", err)
		}
		for _, model := range []string{"", ".", "..", "a/b", "a b"} {
			if _, err := testClient(server, model).GetModel(context.Background()); !errors.Is(err, ErrInvalidModel) {
				t.Errorf("model %q: err = %v", model, err)
			}
		}
		if rec.count() != 0 {
			t.Errorf("%d requests were sent, want none", rec.count())
		}
	})
}
