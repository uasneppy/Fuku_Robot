package main

import (
	"bytes"
	"image"
	"image/jpeg"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const testKey = "test-key-9f3a7c1d-not-a-real-key"

const cannedVerdict = `{"candidates":[{"content":{"parts":[{"text":"{\"category\":\"scam\",\"spam_probability\":0.93}"}]},"finishReason":"STOP"}],` +
	`"usageMetadata":{"promptTokenCount":300,"candidatesTokenCount":20,"thoughtsTokenCount":4},"modelVersion":"gemini-3.5-flash-lite"}`

// recordingTransport counts requests and answers each one with a canned body.
// With forbid set, any request fails the test: it is how the tests prove that
// nothing was sent.
type recordingTransport struct {
	t      *testing.T
	forbid bool
	status int
	body   string

	mu       sync.Mutex
	calls    int
	keys     []string
	paths    []string
	requests []string
}

func (r *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if r.forbid {
		r.t.Errorf("unexpected request to %s", req.URL.Path)
	}
	var sent string
	if req.Body != nil {
		data, _ := io.ReadAll(req.Body)
		sent = string(data)
	}
	r.mu.Lock()
	r.calls++
	r.keys = append(r.keys, req.Header.Get("x-goog-api-key"))
	r.paths = append(r.paths, req.URL.Path)
	r.requests = append(r.requests, sent)
	r.mu.Unlock()

	status := r.status
	if status == 0 {
		status = http.StatusOK
	}
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(r.body)),
		Request:    req,
	}, nil
}

func (r *recordingTransport) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func writeJPEG(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 2, 2)), nil); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func envOf(pairs ...string) func(string) string {
	m := map[string]string{}
	for i := 0; i+1 < len(pairs); i += 2 {
		m[pairs[i]] = pairs[i+1]
	}
	return func(k string) string { return m[k] }
}

func TestSpikeNoKeyMakesNoRequest(t *testing.T) {
	dir := t.TempDir()
	writeJPEG(t, filepath.Join(dir, "spam", "a.jpg"))

	t.Run("no key sends nothing", func(t *testing.T) {
		tr := &recordingTransport{t: t, forbid: true}
		var stdout, stderr bytes.Buffer
		code := run([]string{"-dir", dir}, envOf(), &stdout, &stderr, tr)
		if code == 0 {
			t.Fatal("run without GEMINI_API_KEY exited 0")
		}
		if !strings.Contains(stderr.String(), "GEMINI_API_KEY") {
			t.Errorf("usage does not name GEMINI_API_KEY: %q", stderr.String())
		}
		if tr.count() != 0 {
			t.Errorf("%d requests were sent without a key", tr.count())
		}
	})

	t.Run("with a key it classifies and reports", func(t *testing.T) {
		tr := &recordingTransport{t: t, body: cannedVerdict}
		var stdout, stderr bytes.Buffer
		code := run([]string{"-dir", dir}, envOf("GEMINI_API_KEY", testKey), &stdout, &stderr, tr)
		if code != 0 {
			t.Fatalf("run exited %d; stderr: %s", code, stderr.String())
		}
		if tr.count() != 1 {
			t.Fatalf("%d requests; want one per image", tr.count())
		}
		if tr.keys[0] != testKey {
			t.Error("the key from GEMINI_API_KEY did not reach the request header")
		}
		if !strings.Contains(tr.paths[0], "/models/gemini-3.5-flash-lite:generateContent") {
			t.Errorf("request path %q is not the default model's generateContent", tr.paths[0])
		}
		out := stdout.String()
		if !strings.Contains(out, "spam/a.jpg,spam,0.9300,scam,ok,") {
			t.Errorf("stdout has no CSV row for the image:\n%s", out)
		}
		if !strings.Contains(out, "accuracy") || !strings.Contains(out, "0.85") {
			t.Errorf("stdout has no summary at the 0.85 bar:\n%s", out)
		}
		if !strings.Contains(out, "ai.google.dev/gemini-api/docs/pricing") {
			t.Errorf("the summary does not tell the owner to verify the prices:\n%s", out)
		}
		if strings.Contains(out, testKey) || strings.Contains(stderr.String(), testKey) {
			t.Error("the API key appears in the output")
		}
	})

	t.Run("a provider failure is counted and the key stays out", func(t *testing.T) {
		tr := &recordingTransport{
			t:      t,
			status: http.StatusForbidden,
			body:   `{"error":{"code":403,"message":"key ` + testKey + ` is not allowed","status":"PERMISSION_DENIED"}}`,
		}
		var stdout, stderr bytes.Buffer
		code := run([]string{"-dir", dir, "-concurrency", "1"}, envOf("GEMINI_API_KEY", testKey), &stdout, &stderr, tr)
		if code != 0 {
			t.Fatalf("run exited %d; stderr: %s", code, stderr.String())
		}
		if !strings.Contains(stdout.String(), "failed HTTP 403 PERMISSION_DENIED") {
			t.Errorf("the CSV row does not carry the status and code:\n%s", stdout.String())
		}
		if strings.Contains(stdout.String(), testKey) || strings.Contains(stderr.String(), testKey) {
			t.Error("the API key appears in the output")
		}
	})

	t.Run("a folder with nothing labelled sends nothing", func(t *testing.T) {
		empty := t.TempDir()
		writeJPEG(t, filepath.Join(empty, "unlabelled.jpg"))
		tr := &recordingTransport{t: t, forbid: true}
		var stdout, stderr bytes.Buffer
		code := run([]string{"-dir", empty}, envOf("GEMINI_API_KEY", testKey), &stdout, &stderr, tr)
		if code == 0 {
			t.Error("a folder without labelled images exited 0")
		}
		if tr.count() != 0 {
			t.Errorf("%d requests for a folder without labelled images", tr.count())
		}
	})
}

func TestSpikeRefusesBadFlags(t *testing.T) {
	dir := t.TempDir()
	writeJPEG(t, filepath.Join(dir, "spam", "a.jpg"))

	for _, args := range [][]string{
		{"-dir", dir, "-model", "bad model/../x"},
		{"-dir", dir, "-temperature", "0.7"},
		{"-dir", dir, "-store", "maybe"},
		{"-dir", dir, "-schema-field", "responseNonsense"},
		{"-dir", dir, "-bar", "1.5"},
		{"-dir", dir, "-concurrency", "0"},
		{"-dir", dir, "-no-such-flag"},
		{},
	} {
		tr := &recordingTransport{t: t, forbid: true}
		var stdout, stderr bytes.Buffer
		code := run(args, envOf("GEMINI_API_KEY", testKey), &stdout, &stderr, tr)
		if code == 0 {
			t.Errorf("run %v exited 0", args)
		}
		if tr.count() != 0 {
			t.Errorf("run %v sent %d requests", args, tr.count())
		}
		if strings.Contains(stdout.String()+stderr.String(), testKey) {
			t.Errorf("run %v printed the key", args)
		}
	}
}

func TestSpikeOptionsReachTheRequest(t *testing.T) {
	dir := t.TempDir()
	writeJPEG(t, filepath.Join(dir, "ham", "b.jpg"))

	tr := &recordingTransport{t: t, body: cannedVerdict}
	var stdout, stderr bytes.Buffer
	args := []string{"-dir", dir, "-temperature", "default", "-media-res", "MEDIA_RESOLUTION_LOW", "-thinking", "off", "-store", "false", "-safety", "BLOCK_NONE"}
	if code := run(args, envOf("GEMINI_API_KEY", testKey), &stdout, &stderr, tr); code != 0 {
		t.Fatalf("run exited %d; stderr: %s", code, stderr.String())
	}
	if tr.count() != 1 {
		t.Fatalf("%d requests; want 1", tr.count())
	}
	sent := tr.requests[0]
	for _, want := range []string{`"MEDIA_RESOLUTION_LOW"`, `"store":false`, `"BLOCK_NONE"`} {
		if !strings.Contains(sent, want) {
			t.Errorf("request body lacks %s", want)
		}
	}
	for _, unwanted := range []string{`"temperature"`, `"thinkingConfig"`} {
		if strings.Contains(sent, unwanted) {
			t.Errorf("request body has %s although the flags turned it off", unwanted)
		}
	}
}
