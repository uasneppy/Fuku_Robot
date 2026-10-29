package main

import (
	"bytes"
	"errors"
	"image"
	"image/jpeg"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/divkix/Alita_Robot/alita/utils/gemini"
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
	methods  []string
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
	r.methods = append(r.methods, req.Method)
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

const keyInvalidBody = `{"error":{"code":400,"message":"API key not valid. Please pass a valid API key.","status":"INVALID_ARGUMENT",` +
	`"details":[{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"API_KEY_INVALID","domain":"googleapis.com"}]}}`

func TestSpikeProbeVerdicts(t *testing.T) {
	rejected := &gemini.HTTPError{Status: 400, Code: "INVALID_ARGUMENT", Message: `Invalid JSON payload received. Unknown name "responseFormat" at 'generation_config': Cannot find field.`}
	keyRefused := &gemini.HTTPError{Status: 400, Code: "INVALID_ARGUMENT", Reason: "API_KEY_INVALID", Message: "API key not valid. Please pass a valid API key."}
	notFound := &gemini.HTTPError{Status: 404, Code: "NOT_FOUND", Message: "models/nope is not found"}
	modelSide := &gemini.HTTPError{Status: 400, Code: "INVALID_ARGUMENT", Message: "Thinking level is not supported for this model."}

	cases := []struct {
		name string
		err  error
		v    *gemini.Verdict
		want []string
	}{
		{"field rejected", rejected, nil, []string{"field rejected", "Invalid JSON payload"}},
		{"shape accepted, key refused", keyRefused, nil, []string{"shape accepted, key refused"}},
		{"accepted and answered", nil, &gemini.Verdict{SpamProbability: 0.42}, []string{"accepted and answered", "0.42"}},
		{"other status", notFound, nil, []string{"404", "NOT_FOUND"}},
		{"model-side 400 is not a rejected field", modelSide, nil, []string{"400", "Thinking level"}},
		{"blocked", gemini.ErrBlocked, nil, []string{"accepted", "blocked"}},
		{"unusable answer", gemini.ErrNoVerdict, nil, []string{"accepted", "not a usable verdict"}},
		{"transport", errors.New("connection reset"), nil, []string{"request error"}},
	}
	for _, tc := range cases {
		got := probeVerdict(tc.err, tc.v)
		for _, want := range tc.want {
			if !strings.Contains(got, want) {
				t.Errorf("%s: probeVerdict = %q; want it to contain %q", tc.name, got, want)
			}
		}
	}
	if got := probeVerdict(modelSide, nil); strings.Contains(got, "field rejected") || strings.Contains(got, "key refused") {
		t.Errorf("a model-side 400 was read as %q", got)
	}

	variants := probeVariants()
	names := map[string]bool{}
	mimes := map[string]bool{}
	schemas := map[string]bool{}
	resolutions := map[string]bool{}
	stores := map[bool]bool{}
	var thinkingOn, thinkingOff, safety bool
	for _, v := range variants {
		if names[v.Name] {
			t.Errorf("two probe variants are named %q", v.Name)
		}
		names[v.Name] = true
		if len(v.Image.Data) == 0 {
			t.Errorf("variant %q has no image", v.Name)
		}
		mimes[v.Image.MIMEType] = true
		if v.Options.SchemaField != "" {
			schemas[v.Options.SchemaField] = true
		}
		if v.Options.MediaResolution != "" {
			resolutions[v.Options.MediaResolution] = true
		}
		if v.Options.Store != nil {
			stores[*v.Options.Store] = true
		}
		thinkingOn = thinkingOn || (v.Options.ThinkingLevel != "" && v.Options.ThinkingLevel != "off")
		thinkingOff = thinkingOff || v.Options.ThinkingLevel == "off"
		safety = safety || v.Options.SafetyThreshold != ""
	}
	for _, mime := range []string{"image/jpeg", "image/png", "image/webp"} {
		if !mimes[mime] {
			t.Errorf("no probe variant sends %s", mime)
		}
	}
	for _, field := range []string{"responseJsonSchema", "responseSchema", "responseFormat"} {
		if !schemas[field] {
			t.Errorf("no probe variant uses the %s schema field", field)
		}
	}
	if len(resolutions) != 3 {
		t.Errorf("probe variants cover %d media resolutions; want 3", len(resolutions))
	}
	if !stores[true] || !stores[false] || !thinkingOn || !thinkingOff || !safety {
		t.Errorf("probe variants miss an option: store %v thinking on %v off %v safety %v", stores, thinkingOn, thinkingOff, safety)
	}
}

func TestSpikeProbeRefusesWithoutAnyKey(t *testing.T) {
	for _, args := range [][]string{{"-probe"}, {"-fake-key"}, {"-models-get", "-fake-key"}} {
		tr := &recordingTransport{t: t, forbid: true}
		var stdout, stderr bytes.Buffer
		if code := run(args, envOf(), &stdout, &stderr, tr); code == 0 {
			t.Errorf("run %v without a key exited 0", args)
		}
		if tr.count() != 0 {
			t.Errorf("run %v sent %d requests without a key", args, tr.count())
		}
		if !strings.Contains(stderr.String(), "GEMINI_API_KEY") {
			t.Errorf("run %v: usage does not name GEMINI_API_KEY: %q", args, stderr.String())
		}
	}

	tr := &recordingTransport{t: t, status: http.StatusBadRequest, body: keyInvalidBody}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-probe", "-fake-key"}, envOf(), &stdout, &stderr, tr); code != 0 {
		t.Fatalf("-probe -fake-key exited %d; stderr: %s", code, stderr.String())
	}
	variants := probeVariants()
	if tr.count() != len(variants) {
		t.Fatalf("%d requests; want one per probe variant (%d)", tr.count(), len(variants))
	}
	if got := strings.Count(stdout.String(), "shape accepted, key refused"); got != len(variants) {
		t.Errorf("stdout reports %d variants as 'shape accepted, key refused'; want %d:\n%s", got, len(variants), stdout.String())
	}
	for _, v := range variants {
		if !strings.Contains(stdout.String(), v.Name) {
			t.Errorf("stdout does not name the variant %q", v.Name)
		}
	}
	for _, key := range tr.keys {
		if key == "" || key == testKey {
			t.Errorf("a probe request used the key %q; want the built-in fake key", key)
		}
	}

	// With a real key in the environment the probe uses it, and never prints it.
	tr = &recordingTransport{t: t, status: http.StatusBadRequest, body: keyInvalidBody}
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"-probe"}, envOf("GEMINI_API_KEY", testKey), &stdout, &stderr, tr); code != 0 {
		t.Fatalf("-probe with a key exited %d; stderr: %s", code, stderr.String())
	}
	if tr.keys[0] != testKey {
		t.Error("the probe did not use the key from GEMINI_API_KEY")
	}
	if strings.Contains(stdout.String()+stderr.String(), testKey) {
		t.Error("the probe printed the API key")
	}
}

func TestSpikeModelsGet(t *testing.T) {
	const body = `{"name":"models/gemini-3.5-flash-lite","displayName":"Gemini 3.5 Flash-Lite","inputTokenLimit":1048576,"outputTokenLimit":65536,` +
		`"supportedGenerationMethods":["generateContent","countTokens"]}`

	tr := &recordingTransport{t: t, body: body}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-models-get"}, envOf("GEMINI_API_KEY", testKey), &stdout, &stderr, tr); code != 0 {
		t.Fatalf("-models-get exited %d; stderr: %s", code, stderr.String())
	}
	if tr.count() != 1 || tr.methods[0] != http.MethodGet || !strings.HasSuffix(tr.paths[0], "/v1beta/models/gemini-3.5-flash-lite") {
		t.Errorf("requests = %d %v %v; want one GET of the model", tr.count(), tr.methods, tr.paths)
	}
	for _, want := range []string{"models/gemini-3.5-flash-lite", "1048576", "65536", "generateContent", "countTokens"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("stdout lacks %q:\n%s", want, stdout.String())
		}
	}
	if strings.Contains(stdout.String()+stderr.String(), testKey) {
		t.Error("the API key appears in the output")
	}

	// A model the API does not know is a failure, reported without the key.
	missing := &recordingTransport{
		t:      t,
		status: http.StatusNotFound,
		body:   `{"error":{"code":404,"message":"models/nope is not found for key ` + testKey + `","status":"NOT_FOUND"}}`,
	}
	stdout.Reset()
	stderr.Reset()
	code := run([]string{"-models-get", "-model", "nope"}, envOf("GEMINI_API_KEY", testKey), &stdout, &stderr, missing)
	if code == 0 {
		t.Error("-models-get on an unknown model exited 0")
	}
	if !strings.Contains(stderr.String(), "404") {
		t.Errorf("stderr does not report the 404: %q", stderr.String())
	}
	if strings.Contains(stdout.String()+stderr.String(), testKey) {
		t.Error("the API key appears in the output of a failed -models-get")
	}
}
