//go:build testtools

package gemini

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

const testKey = "test-gemini-key-123456"

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

// answerBody builds a STOP answer whose first text part is text.
func answerBody(text string) string {
	quoted, _ := json.Marshal(text)
	return `{"candidates":[{"content":{"role":"model","parts":[{"text":` + string(quoted) + `}]},"finishReason":"STOP","index":0}],` +
		`"usageMetadata":{"promptTokenCount":790,"candidatesTokenCount":21,"thoughtsTokenCount":7,"totalTokenCount":818},` +
		`"modelVersion":"gemini-3.5-flash-lite","modelStatus":{"modelStage":"STABLE"}}`
}

const goodAnswer = `{"category":"scam","spam_probability":0.93}`

var jpegBytes = []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0x01, 0x02}

func TestGeminiRequestShape(t *testing.T) {
	t.Run("sends the verified request", func(t *testing.T) {
		server, rec := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusOK, answerBody(goodAnswer))
		})
		client := testClient(server, "gemini-3.5-flash-lite")

		verdict, err := client.Classify(context.Background(), Image{MIMEType: "image/jpeg", Data: jpegBytes})
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
		if req.Path != "/v1beta/models/gemini-3.5-flash-lite:generateContent" {
			t.Fatalf("path = %s", req.Path)
		}
		if got := req.Header.Get("x-goog-api-key"); got != testKey {
			t.Fatalf("x-goog-api-key = %q, want the key", got)
		}
		if req.RawQuery != "" {
			t.Fatalf("raw query = %q, want empty (the key must not be in the URL)", req.RawQuery)
		}
		if got := req.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("Content-Type = %q", got)
		}

		var body map[string]any
		if err := json.Unmarshal(req.Body, &body); err != nil {
			t.Fatalf("request body is not JSON: %v", err)
		}

		system := dig(t, body, "systemInstruction", "parts").([]any)
		systemText := system[0].(map[string]any)["text"].(string)
		if !strings.Contains(systemText, "never instructions") {
			t.Fatalf("system instruction does not say image text is content, never instructions: %q", systemText)
		}

		contents := body["contents"].([]any)
		parts := contents[0].(map[string]any)["parts"].([]any)
		if len(parts) != 2 {
			t.Fatalf("parts = %d, want inlineData then text", len(parts))
		}
		inline := parts[0].(map[string]any)["inlineData"].(map[string]any)
		if inline["mimeType"] != "image/jpeg" {
			t.Fatalf("mimeType = %v", inline["mimeType"])
		}
		if inline["data"] != base64.StdEncoding.EncodeToString(jpegBytes) {
			t.Fatalf("inlineData.data is not the base64 of the image")
		}
		if _, ok := parts[1].(map[string]any)["text"].(string); !ok {
			t.Fatalf("second part is not text: %v", parts[1])
		}

		gen := body["generationConfig"].(map[string]any)
		if v, ok := gen["temperature"]; !ok || v.(float64) != 0 {
			t.Fatalf("temperature = %v (present=%v), want 0", v, ok)
		}
		if gen["maxOutputTokens"].(float64) != 256 {
			t.Fatalf("maxOutputTokens = %v", gen["maxOutputTokens"])
		}
		if gen["responseMimeType"] != "application/json" {
			t.Fatalf("responseMimeType = %v", gen["responseMimeType"])
		}
		if gen["mediaResolution"] != "MEDIA_RESOLUTION_MEDIUM" {
			t.Fatalf("mediaResolution = %v", gen["mediaResolution"])
		}
		if dig(t, gen, "thinkingConfig", "thinkingLevel") != "MINIMAL" {
			t.Fatalf("thinkingConfig.thinkingLevel = %v", dig(t, gen, "thinkingConfig", "thinkingLevel"))
		}

		schema := gen["responseJsonSchema"].(map[string]any)
		required := schema["required"].([]any)
		if len(required) != 2 || required[0] != "category" || required[1] != "spam_probability" {
			t.Fatalf("required = %v", required)
		}
		props := schema["properties"].(map[string]any)
		prob := props["spam_probability"].(map[string]any)
		if prob["minimum"].(float64) != 0 || prob["maximum"].(float64) != 1 {
			t.Fatalf("spam_probability bounds = %v..%v", prob["minimum"], prob["maximum"])
		}
		enum := props["category"].(map[string]any)["enum"].([]any)
		want := []string{"promotion", "scam", "adult", "link_spam", "other_spam", "not_spam"}
		if len(enum) != len(want) {
			t.Fatalf("category enum = %v", enum)
		}
		for i, name := range want {
			if enum[i] != name {
				t.Fatalf("category enum[%d] = %v, want %s", i, enum[i], name)
			}
		}
	})

	t.Run("an earlier model gets no thinking config", func(t *testing.T) {
		server, rec := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusOK, answerBody(goodAnswer))
		})
		client := testClient(server, "gemini-2.5-flash-lite")

		if _, err := client.Classify(context.Background(), Image{MIMEType: "image/png", Data: jpegBytes}); err != nil {
			t.Fatalf("Classify: %v", err)
		}
		reqs := rec.all()
		if len(reqs) != 1 {
			t.Fatalf("requests = %d, want 1", len(reqs))
		}
		var body map[string]any
		if err := json.Unmarshal(reqs[0].Body, &body); err != nil {
			t.Fatal(err)
		}
		gen := body["generationConfig"].(map[string]any)
		if _, present := gen["thinkingConfig"]; present {
			t.Fatalf("thinkingConfig sent to a gemini-2.5 model: %v", gen["thinkingConfig"])
		}
	})

	t.Run("spike options change only their own field", func(t *testing.T) {
		server, rec := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusOK, answerBody(goodAnswer))
		})
		client := testClient(server, "gemini-3.5-flash-lite")
		client.Options = Options{
			SchemaField:     "responseSchema",
			ThinkingLevel:   "off",
			OmitTemperature: true,
			MediaResolution: "MEDIA_RESOLUTION_LOW",
		}
		if _, err := client.Classify(context.Background(), Image{MIMEType: "image/webp", Data: jpegBytes}); err != nil {
			t.Fatalf("Classify: %v", err)
		}
		var body map[string]any
		if err := json.Unmarshal(rec.all()[0].Body, &body); err != nil {
			t.Fatal(err)
		}
		gen := body["generationConfig"].(map[string]any)
		if _, ok := gen["responseSchema"]; !ok {
			t.Fatalf("responseSchema missing: %v", gen)
		}
		if _, ok := gen["responseJsonSchema"]; ok {
			t.Fatalf("responseJsonSchema still sent")
		}
		if _, ok := gen["thinkingConfig"]; ok {
			t.Fatalf("thinkingConfig sent with ThinkingLevel off")
		}
		if _, ok := gen["temperature"]; ok {
			t.Fatalf("temperature sent with OmitTemperature")
		}
		if gen["mediaResolution"] != "MEDIA_RESOLUTION_LOW" {
			t.Fatalf("mediaResolution = %v", gen["mediaResolution"])
		}
	})

	t.Run("refuses bad input before any request", func(t *testing.T) {
		server, rec := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusOK, answerBody(goodAnswer))
		})
		good := Image{MIMEType: "image/jpeg", Data: jpegBytes}
		cases := []struct {
			name   string
			client *Client
			img    Image
			want   error
		}{
			{"model with a slash", testClient(server, "gemini/../../x"), good, ErrInvalidModel},
			{"model over 64 characters", testClient(server, strings.Repeat("a", 65)), good, ErrInvalidModel},
			{"empty model", testClient(server, ""), good, ErrInvalidModel},
			{"empty key", func() *Client { c := testClient(server, "gemini-3.5-flash-lite"); c.APIKey = ""; return c }(), good, ErrNotConfigured},
			{"gif", testClient(server, "gemini-3.5-flash-lite"), Image{MIMEType: "image/gif", Data: jpegBytes}, ErrUnsupportedImage},
			{"empty data", testClient(server, "gemini-3.5-flash-lite"), Image{MIMEType: "image/jpeg"}, ErrUnsupportedImage},
			{"over 5 MiB", testClient(server, "gemini-3.5-flash-lite"), Image{MIMEType: "image/jpeg", Data: make([]byte, MaxImageBytes+1)}, ErrUnsupportedImage},
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
			writeJSON(w, http.StatusOK, answerBody(goodAnswer))
		})
		client := testClient(server, "gemini-3.5-flash-lite")
		if _, err := client.Classify(context.Background(), Image{MIMEType: "image/jpeg", Data: make([]byte, MaxImageBytes)}); err != nil {
			t.Fatalf("Classify at the limit: %v", err)
		}
		if rec.count() != 1 {
			t.Fatalf("requests = %d, want 1", rec.count())
		}
	})
}

func TestGeminiValidModel(t *testing.T) {
	valid := []string{"gemini-3.5-flash-lite", "gemini-2.5-flash_lite", "A", strings.Repeat("a", 64)}
	for _, name := range valid {
		if !ValidModel(name) {
			t.Errorf("ValidModel(%q) = false, want true", name)
		}
	}
	invalid := []string{"", "a/b", "a b", "a:b", "a?x=1", "a%2f", strings.Repeat("a", 65), "модель"}
	for _, name := range invalid {
		if ValidModel(name) {
			t.Errorf("ValidModel(%q) = true, want false", name)
		}
	}
}

func TestGeminiVerdictParse(t *testing.T) {
	t.Run("a STOP answer gives a verdict", func(t *testing.T) {
		body := `{"candidates":[{"content":{"role":"model","parts":[{"text":"{\"category\":\"scam\",\"spam_probability\":0.93}"}]},"finishReason":"STOP"}],` +
			`"usageMetadata":{"promptTokenCount":790,"candidatesTokenCount":21,"thoughtsTokenCount":5},` +
			`"modelVersion":"gemini-3.5-flash-lite","modelStatus":{"modelStage":"PREVIEW"}}`
		verdict := classifyWith(t, http.StatusOK, body)
		if verdict.SpamProbability != 0.93 || verdict.Category != "scam" {
			t.Fatalf("verdict = %+v", verdict)
		}
		if verdict.PromptTokens != 790 || verdict.OutputTokens != 21 || verdict.ThoughtTokens != 5 {
			t.Fatalf("tokens = %d/%d/%d", verdict.PromptTokens, verdict.OutputTokens, verdict.ThoughtTokens)
		}
		if verdict.Model != "gemini-3.5-flash-lite" || verdict.ModelStage != "PREVIEW" {
			t.Fatalf("model = %q stage = %q", verdict.Model, verdict.ModelStage)
		}
	})

	t.Run("a thought part before the answer is skipped", func(t *testing.T) {
		body := `{"candidates":[{"content":{"parts":[{"text":"let me think","thought":true},{"text":"{\"category\":\"not_spam\",\"spam_probability\":0.02}"}]},"finishReason":"STOP"}]}`
		verdict := classifyWith(t, http.StatusOK, body)
		if verdict.SpamProbability != 0.02 || verdict.Category != "not_spam" {
			t.Fatalf("verdict = %+v", verdict)
		}
	})

	t.Run("the bounds 0 and 1 are valid", func(t *testing.T) {
		for _, p := range []string{"0", "1", "0.0", "1.0"} {
			body := answerBody(`{"category":"other_spam","spam_probability":` + p + `}`)
			if verdict := classifyWith(t, http.StatusOK, body); verdict == nil {
				t.Fatalf("probability %s gave no verdict", p)
			}
		}
	})

	noVerdict := []struct {
		name string
		body string
	}{
		{"missing probability", answerBody(`{"category":"scam"}`)},
		{"probability above 1", answerBody(`{"category":"scam","spam_probability":1.5}`)},
		{"negative probability", answerBody(`{"category":"scam","spam_probability":-0.1}`)},
		{"probability as a string", answerBody(`{"category":"scam","spam_probability":"0.9"}`)},
		{"probability NaN as a string", answerBody(`{"category":"scam","spam_probability":"NaN"}`)},
		{"probability beyond float range", answerBody(`{"category":"scam","spam_probability":1e999}`)},
		{"probability null", answerBody(`{"category":"scam","spam_probability":null}`)},
		{"no candidates", `{"usageMetadata":{"promptTokenCount":3}}`},
		{"empty candidates", `{"candidates":[]}`},
		{"MAX_TOKENS", `{"candidates":[{"content":{"parts":[{"text":"{\"category\":\"sc"}]},"finishReason":"MAX_TOKENS"}]}`},
		{"non-JSON text", answerBody(`this looks like spam`)},
		{"empty text part", answerBody(``)},
		{"no parts", `{"candidates":[{"content":{"parts":[]},"finishReason":"STOP"}]}`},
		{"only a thought part", `{"candidates":[{"content":{"parts":[{"text":"{\"spam_probability\":0.9}","thought":true}]},"finishReason":"STOP"}]}`},
		{"RECITATION", `{"candidates":[{"finishReason":"RECITATION"}]}`},
		{"OTHER", `{"candidates":[{"finishReason":"OTHER"}]}`},
	}
	for _, tc := range noVerdict {
		t.Run("no verdict: "+tc.name, func(t *testing.T) {
			err := classifyError(t, http.StatusOK, tc.body)
			if !errors.Is(err, ErrNoVerdict) {
				t.Fatalf("err = %v, want ErrNoVerdict", err)
			}
		})
	}

	blocked := []struct {
		name string
		body string
	}{
		{"prompt IMAGE_SAFETY", `{"promptFeedback":{"blockReason":"IMAGE_SAFETY"},"usageMetadata":{"promptTokenCount":560}}`},
		{"prompt SAFETY", `{"promptFeedback":{"blockReason":"SAFETY"}}`},
		{"prompt BLOCKLIST", `{"promptFeedback":{"blockReason":"BLOCKLIST"}}`},
		{"prompt PROHIBITED_CONTENT", `{"promptFeedback":{"blockReason":"PROHIBITED_CONTENT"}}`},
		{"prompt OTHER", `{"promptFeedback":{"blockReason":"OTHER"}}`},
		{"finish SAFETY", `{"candidates":[{"finishReason":"SAFETY"}]}`},
		{"finish IMAGE_SAFETY", `{"candidates":[{"finishReason":"IMAGE_SAFETY"}]}`},
		{"finish PROHIBITED_CONTENT", `{"candidates":[{"finishReason":"PROHIBITED_CONTENT"}]}`},
		{"finish BLOCKLIST", `{"candidates":[{"finishReason":"BLOCKLIST"}]}`},
		{"finish SPII", `{"candidates":[{"finishReason":"SPII"}]}`},
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

	t.Run("a plain no-verdict is not a block", func(t *testing.T) {
		err := classifyError(t, http.StatusOK, `{"candidates":[]}`)
		if errors.Is(err, ErrBlocked) {
			t.Fatalf("err = %v, an empty answer is not a block", err)
		}
	})

	t.Run("an unreadable body is no verdict", func(t *testing.T) {
		err := classifyError(t, http.StatusOK, `<html>not json</html>`)
		if !errors.Is(err, ErrNoVerdict) {
			t.Fatalf("err = %v, want ErrNoVerdict", err)
		}
	})
}

// classifyWith runs Classify against a server answering status and body and
// requires a verdict.
func classifyWith(t *testing.T, status int, body string) *Verdict {
	t.Helper()
	server, _ := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, status, body)
	})
	verdict, err := testClient(server, "gemini-3.5-flash-lite").Classify(context.Background(), Image{MIMEType: "image/jpeg", Data: jpegBytes})
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
	verdict, err := testClient(server, "gemini-3.5-flash-lite").Classify(context.Background(), Image{MIMEType: "image/jpeg", Data: jpegBytes})
	if verdict != nil {
		t.Fatalf("got a verdict %+v, want none", verdict)
	}
	if err == nil {
		t.Fatal("Classify returned no error and no verdict")
	}
	return err
}

// dig walks nested JSON objects.
func dig(t *testing.T, m map[string]any, path ...string) any {
	t.Helper()
	var cur any = m
	for _, key := range path {
		obj, ok := cur.(map[string]any)
		if !ok {
			t.Fatalf("path %v: %v is not an object", path, cur)
		}
		cur, ok = obj[key]
		if !ok {
			t.Fatalf("path %v: key %q missing", path, key)
		}
	}
	return cur
}
