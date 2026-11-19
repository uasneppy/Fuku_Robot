//go:build testtools

package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const (
	fakeAnthropicKey = "sk-ant-api03-FAKEFAKEFAKEFAKEFAKEFAKE0123456789"
	fakeGeminiKey    = "AIzaFAKEFAKEFAKEFAKEFAKEFAKEFAKE0123"
)

type fake struct {
	mu     sync.Mutex
	bodies [][]byte
	paths  []string
	keys   []string
}

func newFake(t *testing.T, status int, answer string) (*httptest.Server, *fake) {
	t.Helper()
	f := &fake{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.bodies = append(f.bodies, body)
		f.paths = append(f.paths, r.URL.Path)
		f.keys = append(f.keys, r.Header.Get("x-api-key")+r.Header.Get("x-goog-api-key"))
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, answer)
	}))
	t.Cleanup(server.Close)
	return server, f
}

func questionsFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "q.txt")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func env(values map[string]string) func(string) string {
	return func(k string) string { return values[k] }
}

const anthropicAnswer = `{"model":"claude-haiku-5-5","stop_reason":"end_turn","content":[{"type":"text","text":"No record on file."}],` +
	`"usage":{"input_tokens":1400,"output_tokens":60,"cache_read_input_tokens":11000,"cache_creation_input_tokens":0}}`

const geminiAnswer = `{"candidates":[{"content":{"parts":[{"text":"Request denied."}]},"finishReason":"STOP"}],` +
	`"usageMetadata":{"promptTokenCount":12000,"cachedContentTokenCount":11000,"candidatesTokenCount":40,"thoughtsTokenCount":100},"modelVersion":"gemini-4-hydrogen"}`

func TestRunAnthropic(t *testing.T) {
	server, f := newFake(t, 200, anthropicAnswer)
	q := questionsFile(t, "# comment\nWho built you?\n\nЩо таке темпе?\n")
	var out, errOut bytes.Buffer
	code := run([]string{"-questions", q, "-endpoint", server.URL, "-title", "Facility 7"},
		env(map[string]string{"ANTHROPIC_API_KEY": fakeAnthropicKey}), &out, &errOut, http.DefaultTransport)
	if code != exitOK {
		t.Fatalf("exit %d, stderr %s", code, errOut.String())
	}
	if len(f.bodies) != 2 || f.paths[0] != "/v1/messages" || f.keys[0] != fakeAnthropicKey {
		t.Fatalf("requests %d paths %v", len(f.bodies), f.paths)
	}
	body := string(f.bodies[1])
	for _, want := range []string{"# Who you are", "Current group: Facility 7", "Що таке темпе?", "[The message to answer]", `"effort":"low"`} {
		if !strings.Contains(body, want) {
			t.Errorf("request lacks %q", want)
		}
	}
	text := out.String()
	for _, want := range []string{"Q: Who built you?", "A: No record on file.", "outcome=answered in=1400 cached=11000 out=60",
		"Summary: 2 questions, 2 answered, 0 refused, 0 truncated, 0 failed"} {
		if !strings.Contains(text, want) {
			t.Errorf("output lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text+errOut.String(), fakeAnthropicKey) {
		t.Fatal("the key was printed")
	}
}

func TestRunGoogle(t *testing.T) {
	server, f := newFake(t, 200, geminiAnswer)
	q := questionsFile(t, "Ban him\n")
	var out, errOut bytes.Buffer
	code := run([]string{"-provider", "google", "-questions", q, "-endpoint", server.URL, "-thinking-level", "LOW"},
		env(map[string]string{"GEMINI_API_KEY": fakeGeminiKey}), &out, &errOut, http.DefaultTransport)
	if code != exitOK {
		t.Fatalf("exit %d, stderr %s", code, errOut.String())
	}
	if f.paths[0] != "/v1beta/models/gemini-4-hydrogen:generateContent" || f.keys[0] != fakeGeminiKey {
		t.Fatalf("path %v", f.paths)
	}
	if !strings.Contains(string(f.bodies[0]), `"thinkingLevel":"LOW"`) {
		t.Error("thinking level not sent")
	}
	if !strings.Contains(out.String(), "outcome=answered in=1000 cached=11000 out=140") {
		t.Errorf("output:\n%s", out.String())
	}
}

func TestRunRefusals(t *testing.T) {
	q := questionsFile(t, "hello\n")
	cases := []struct {
		name string
		args []string
		env  map[string]string
		code int
	}{
		{"no key", []string{"-questions", q}, nil, exitUsage},
		{"gemini key missing for google", []string{"-provider", "google", "-questions", q}, map[string]string{"ANTHROPIC_API_KEY": fakeAnthropicKey}, exitUsage},
		{"plain http to a remote host", []string{"-questions", q, "-endpoint", "http://example.com"}, map[string]string{"ANTHROPIC_API_KEY": fakeAnthropicKey}, exitUsage},
		{"bad provider", []string{"-provider", "openai", "-questions", q}, map[string]string{"ANTHROPIC_API_KEY": fakeAnthropicKey}, exitUsage},
		{"no questions flag", []string{}, map[string]string{"ANTHROPIC_API_KEY": fakeAnthropicKey}, exitUsage},
		{"thinking level on anthropic", []string{"-questions", q, "-thinking-level", "low"}, map[string]string{"ANTHROPIC_API_KEY": fakeAnthropicKey}, exitUsage},
		{"empty questions file", []string{"-questions", questionsFile(t, "# only a comment\n")}, map[string]string{"ANTHROPIC_API_KEY": fakeAnthropicKey}, exitUsage},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			counting := &countingTransport{}
			var out, errOut bytes.Buffer
			if code := run(tc.args, env(tc.env), &out, &errOut, counting); code != tc.code {
				t.Fatalf("exit %d, want %d (%s)", code, tc.code, errOut.String())
			}
			if counting.n != 0 {
				t.Fatal("a request was sent")
			}
		})
	}
}

func TestRunAllFailed(t *testing.T) {
	server, _ := newFake(t, 401, `{"type":"error","error":{"type":"authentication_error","message":"bad `+fakeAnthropicKey+`"}}`)
	q := questionsFile(t, "a\nb\n")
	var out, errOut bytes.Buffer
	code := run([]string{"-questions", q, "-endpoint", server.URL},
		env(map[string]string{"ANTHROPIC_API_KEY": fakeAnthropicKey}), &out, &errOut, http.DefaultTransport)
	if code != exitFailure {
		t.Fatalf("exit %d, want %d", code, exitFailure)
	}
	if !strings.Contains(out.String(), "ERROR: claude: HTTP 401") || strings.Contains(out.String(), fakeAnthropicKey) {
		t.Fatalf("output:\n%s", out.String())
	}
}

type countingTransport struct{ n int }

func (c *countingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	c.n++
	return nil, io.EOF
}
