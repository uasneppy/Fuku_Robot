package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/divkix/Alita_Robot/alita/utils/claude"
)

const testKey = "sk-ant-api03-SPIKEFAKESPIKEFAKESPIKEFAKESPIKEFAKE0123456789"

// cannedVerdict is a Messages answer carrying a forced tool call.
const cannedVerdict = `{"id":"msg_01","type":"message","role":"assistant","model":"` + claude.DefaultModel + `",` +
	`"content":[{"type":"tool_use","id":"toolu_01","name":"` + claude.ToolName + `","input":{"spam_probability":0.93}}],` +
	`"stop_reason":"tool_use","stop_details":null,"usage":{"input_tokens":300,"output_tokens":20}}`

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
	r.keys = append(r.keys, req.Header.Get("x-api-key"))
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

func errorEnvelope(errType, message string) string {
	return `{"type":"error","error":{"type":"` + errType + `","message":"` + message + `"},"request_id":"req_01"}`
}

func TestSpikeNoKeyMakesNoRequest(t *testing.T) {
	dir := t.TempDir()
	writeJPEG(t, filepath.Join(dir, "spam", "a.jpg"))

	t.Run("no key sends nothing", func(t *testing.T) {
		tr := &recordingTransport{t: t, forbid: true}
		var stdout, stderr bytes.Buffer
		code := run([]string{"-dir", dir}, envOf(), &stdout, &stderr, tr)
		if code != exitUsage {
			t.Fatalf("run without ANTHROPIC_API_KEY exited %d, want %d", code, exitUsage)
		}
		if !strings.Contains(stderr.String(), "ANTHROPIC_API_KEY") {
			t.Errorf("usage does not name ANTHROPIC_API_KEY: %q", stderr.String())
		}
		if tr.count() != 0 {
			t.Errorf("%d requests were sent without a key", tr.count())
		}
	})

	t.Run("with a key it classifies and reports", func(t *testing.T) {
		tr := &recordingTransport{t: t, body: cannedVerdict}
		var stdout, stderr bytes.Buffer
		code := run([]string{"-dir", dir}, envOf("ANTHROPIC_API_KEY", testKey), &stdout, &stderr, tr)
		if code != 0 {
			t.Fatalf("run exited %d; stderr: %s", code, stderr.String())
		}
		if tr.count() != 1 {
			t.Fatalf("%d requests; want one per image", tr.count())
		}
		if tr.keys[0] != testKey {
			t.Error("the key from ANTHROPIC_API_KEY did not reach the x-api-key header")
		}
		if tr.methods[0] != http.MethodPost || tr.paths[0] != "/v1/messages" {
			t.Errorf("request = %s %s, want POST /v1/messages", tr.methods[0], tr.paths[0])
		}
		if !strings.Contains(tr.requests[0], `"model":"`+claude.DefaultModel+`"`) {
			t.Errorf("request body does not name the default model: %s", tr.requests[0])
		}
		out := stdout.String()
		if !strings.Contains(out, "spam/a.jpg,spam,0.9300,ok,") {
			t.Errorf("stdout has no CSV row for the image:\n%s", out)
		}
		if !strings.Contains(out, "accuracy") || !strings.Contains(out, "0.85") {
			t.Errorf("stdout has no summary at the 0.85 bar:\n%s", out)
		}
		if !strings.Contains(out, "Tokens: input 300, output 20") || !strings.Contains(out, "Estimated cost: $") {
			t.Errorf("stdout has no token and cost figures:\n%s", out)
		}
		if !strings.Contains(out, "pricing page") {
			t.Errorf("the summary does not tell the owner to verify the prices:\n%s", out)
		}
		if strings.Contains(out, testKey) || strings.Contains(stderr.String(), testKey) {
			t.Error("the API key appears in the output")
		}
		if !strings.Contains(stderr.String(), "to Anthropic") {
			t.Errorf("stderr does not say the images go to Anthropic: %q", stderr.String())
		}
	})

	t.Run("a provider failure is counted and the key stays out", func(t *testing.T) {
		tr := &recordingTransport{
			t:      t,
			status: http.StatusForbidden,
			body:   errorEnvelope("permission_error", "key "+testKey+" is not allowed"),
		}
		var stdout, stderr bytes.Buffer
		code := run([]string{"-dir", dir, "-concurrency", "1"}, envOf("ANTHROPIC_API_KEY", testKey), &stdout, &stderr, tr)
		if code != exitFailure {
			t.Fatalf("run exited %d, want %d when every request failed; stderr: %s", code, exitFailure, stderr.String())
		}
		if !strings.Contains(stderr.String(), "Every request failed") {
			t.Errorf("stderr does not say every request failed: %q", stderr.String())
		}
		if !strings.Contains(stdout.String(), "failed HTTP 403 permission_error") {
			t.Errorf("the CSV row does not carry the status and error type:\n%s", stdout.String())
		}
		if strings.Contains(stdout.String(), testKey) || strings.Contains(stderr.String(), testKey) {
			t.Error("the API key appears in the output")
		}
	})

	t.Run("a refusal is blocked, not failed", func(t *testing.T) {
		tr := &recordingTransport{t: t, body: `{"content":[],"stop_reason":"refusal","stop_details":{"type":"refusal","category":"general_harms"}}`}
		var stdout, stderr bytes.Buffer
		code := run([]string{"-dir", dir}, envOf("ANTHROPIC_API_KEY", testKey), &stdout, &stderr, tr)
		if code != 0 {
			t.Fatalf("run exited %d, want 0 (a refusal is not a failed request); stderr: %s", code, stderr.String())
		}
		if !strings.Contains(stdout.String(), "spam/a.jpg,spam,,blocked,") {
			t.Errorf("the CSV row is not a blocked row:\n%s", stdout.String())
		}
	})

	t.Run("a folder with nothing labelled sends nothing", func(t *testing.T) {
		empty := t.TempDir()
		writeJPEG(t, filepath.Join(empty, "unlabelled.jpg"))
		tr := &recordingTransport{t: t, forbid: true}
		var stdout, stderr bytes.Buffer
		code := run([]string{"-dir", empty}, envOf("ANTHROPIC_API_KEY", testKey), &stdout, &stderr, tr)
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
		{"-dir", dir, "-model", ".."},
		{"-dir", dir, "-tool-choice", "required"},
		{"-dir", dir, "-thinking", "enabled"},
		{"-dir", dir, "-effort", "extreme"},
		{"-dir", dir, "-effort", "xhigh"},
		{"-dir", dir, "-effort", "max", "-thinking", "disabled"},
		{"-dir", dir, "-max-tokens", "-1"},
		{"-dir", dir, "-max-tokens", "4097"},
		{"-dir", dir, "-bar", "1.5"},
		{"-dir", dir, "-concurrency", "0"},
		{"-dir", dir, "-concurrency", "9"},
		{"-dir", dir, "-endpoint", "http://api.anthropic.com"},
		{"-dir", dir, "-endpoint", "http://localhost.evil.example"},
		{"-dir", dir, "-endpoint", "ftp://127.0.0.1"},
		{"-dir", dir, "-endpoint", "https://"},
		{"-dir", dir, "-endpoint", "https://user:pass@api.anthropic.com"},
		{"-dir", dir, "-endpoint", "api.anthropic.com"},
		{"-probe", "-models-get"},
		{"-dir", dir, "-no-such-flag"},
		{},
	} {
		tr := &recordingTransport{t: t, forbid: true}
		var stdout, stderr bytes.Buffer
		code := run(args, envOf("ANTHROPIC_API_KEY", testKey), &stdout, &stderr, tr)
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

func TestSpikeAcceptsHTTPSAndLoopbackEndpoints(t *testing.T) {
	dir := t.TempDir()
	writeJPEG(t, filepath.Join(dir, "spam", "a.jpg"))

	for _, endpoint := range []string{
		"https://api.anthropic.com",
		"http://localhost:8080",
		"http://127.0.0.1:9",
		"http://[::1]:9",
	} {
		tr := &recordingTransport{t: t, body: cannedVerdict}
		var stdout, stderr bytes.Buffer
		code := run([]string{"-dir", dir, "-endpoint", endpoint}, envOf("ANTHROPIC_API_KEY", testKey), &stdout, &stderr, tr)
		if code != 0 {
			t.Errorf("-endpoint %s exited %d; stderr: %s", endpoint, code, stderr.String())
		}
		if tr.count() != 1 {
			t.Errorf("-endpoint %s sent %d requests, want 1", endpoint, tr.count())
		}
	}
}

// spikeBody runs one labelled image through the tool with extra flags and
// returns the decoded request body.
func spikeBody(t *testing.T, dir string, extra ...string) map[string]any {
	t.Helper()
	tr := &recordingTransport{t: t, body: cannedVerdict}
	var stdout, stderr bytes.Buffer
	args := append([]string{"-dir", dir}, extra...)
	if code := run(args, envOf("ANTHROPIC_API_KEY", testKey), &stdout, &stderr, tr); code != 0 {
		t.Fatalf("run %v exited %d; stderr: %s", args, code, stderr.String())
	}
	if tr.count() != 1 {
		t.Fatalf("run %v: %d requests; want 1", args, tr.count())
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(tr.requests[0]), &body); err != nil {
		t.Fatalf("request body is not JSON: %v", err)
	}
	return body
}

func TestSpikeOptionsReachTheRequest(t *testing.T) {
	dir := t.TempDir()
	writeJPEG(t, filepath.Join(dir, "ham", "b.jpg"))

	production := spikeBody(t, dir)
	changed := func(body map[string]any) []string {
		var keys []string
		seen := map[string]bool{}
		for k, v := range production {
			seen[k] = true
			if fmt.Sprint(v) != fmt.Sprint(body[k]) {
				keys = append(keys, k)
			}
		}
		for k := range body {
			if !seen[k] {
				keys = append(keys, k)
			}
		}
		return keys
	}

	cases := []struct {
		flags []string
		field string
		check func(map[string]any) bool
	}{
		{[]string{"-tool-choice", "auto"}, "tool_choice", func(b map[string]any) bool {
			return b["tool_choice"].(map[string]any)["type"] == "auto"
		}},
		{[]string{"-thinking", "adaptive"}, "thinking", func(b map[string]any) bool {
			return b["thinking"].(map[string]any)["type"] == "adaptive"
		}},
		{[]string{"-thinking", "omit"}, "thinking", func(b map[string]any) bool {
			_, ok := b["thinking"]
			return !ok
		}},
		{[]string{"-effort", "low"}, "output_config", func(b map[string]any) bool {
			return b["output_config"].(map[string]any)["effort"] == "low"
		}},
		{[]string{"-effort", "max", "-thinking", "adaptive"}, "", nil},
		{[]string{"-max-tokens", "1024"}, "max_tokens", func(b map[string]any) bool {
			return b["max_tokens"].(float64) == 1024
		}},
	}
	for _, tc := range cases {
		body := spikeBody(t, dir, tc.flags...)
		if tc.field == "" {
			// A combination: both its fields differ and nothing else.
			keys := changed(body)
			if len(keys) != 2 {
				t.Errorf("flags %v changed %v; want exactly thinking and output_config", tc.flags, keys)
			}
			continue
		}
		keys := changed(body)
		if len(keys) != 1 || keys[0] != tc.field {
			t.Errorf("flags %v changed %v; want only %s", tc.flags, keys, tc.field)
		}
		if !tc.check(body) {
			t.Errorf("flags %v did not reach the request: %v", tc.flags, body)
		}
	}

	if got := production["tool_choice"].(map[string]any)["type"]; got != "tool" {
		t.Errorf("default tool_choice type = %v, want the forced tool", got)
	}
}

const keyRefusedBody = `{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"},"request_id":"req_01"}`

func TestSpikeProbeVerdicts(t *testing.T) {
	rejected := &claude.HTTPError{Status: 400, Type: "invalid_request_error", Message: "thinking.type: Input tag 'bogus' found using 'type' does not match any of the expected tags"}
	keyRefused := &claude.HTTPError{Status: 401, Type: "authentication_error", Message: "invalid x-api-key"}
	notFound := &claude.HTTPError{Status: 404, Type: "not_found_error", Message: "model: nope"}
	overloaded := &claude.HTTPError{Status: 529, Type: "overloaded_error"}

	cases := []struct {
		name string
		err  error
		v    *claude.Verdict
		want []string
	}{
		{"field rejected", rejected, nil, []string{"field rejected", "thinking.type"}},
		{"key refused", keyRefused, nil, []string{"key refused"}},
		{"accepted and answered", nil, &claude.Verdict{SpamProbability: 0.42}, []string{"accepted and answered", "0.42"}},
		{"other status", notFound, nil, []string{"404", "not_found_error"}},
		{"status without a message", overloaded, nil, []string{"529", "overloaded_error"}},
		{"refusal", claude.ErrBlocked, nil, []string{"accepted", "refused by the provider"}},
		{"unusable answer", claude.ErrNoVerdict, nil, []string{"accepted", "not a usable verdict"}},
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
	if got := probeVerdict(notFound, nil); strings.Contains(got, "field rejected") || strings.Contains(got, "key refused") {
		t.Errorf("a 404 was read as %q", got)
	}
	if got := probeVerdict(rejected, nil); strings.Contains(got, "key refused") {
		t.Errorf("a rejected field was read as %q", got)
	}

	variants := probeVariants()
	names := map[string]bool{}
	mimes := map[string]bool{}
	var auto, adaptive, omitted, effort bool
	for _, v := range variants {
		if names[v.Name] {
			t.Errorf("two probe variants are named %q", v.Name)
		}
		names[v.Name] = true
		if len(v.Image.Data) == 0 {
			t.Errorf("variant %q has no image", v.Name)
		}
		if got := http.DetectContentType(v.Image.Data); got != v.Image.MIMEType {
			t.Errorf("variant %q claims %s but its bytes sniff as %s", v.Name, v.Image.MIMEType, got)
		}
		mimes[v.Image.MIMEType] = true
		auto = auto || v.Options.ToolChoice == "auto"
		adaptive = adaptive || v.Options.Thinking == "adaptive"
		omitted = omitted || v.Options.Thinking == "omit"
		effort = effort || v.Options.Effort == "low"
		if (v.Options.Thinking == "adaptive" || v.Options.Thinking == "omit") && v.Options.MaxTokens != 1024 {
			t.Errorf("variant %q has max tokens %d; thinking variants use 1024", v.Name, v.Options.MaxTokens)
		}
	}
	for _, mime := range []string{"image/jpeg", "image/png", "image/webp"} {
		if !mimes[mime] {
			t.Errorf("no probe variant sends %s", mime)
		}
	}
	if !auto || !adaptive || !omitted || !effort {
		t.Errorf("probe variants miss an option: auto %v adaptive %v omitted %v effort %v", auto, adaptive, omitted, effort)
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
		if !strings.Contains(stderr.String(), "ANTHROPIC_API_KEY") {
			t.Errorf("run %v: usage does not name ANTHROPIC_API_KEY: %q", args, stderr.String())
		}
	}

	tr := &recordingTransport{t: t, status: http.StatusUnauthorized, body: keyRefusedBody}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-probe", "-fake-key"}, envOf(), &stdout, &stderr, tr); code != 0 {
		t.Fatalf("-probe -fake-key exited %d; stderr: %s", code, stderr.String())
	}
	variants := probeVariants()
	if tr.count() != len(variants) {
		t.Fatalf("%d requests; want one per probe variant (%d)", tr.count(), len(variants))
	}
	if got := strings.Count(stdout.String(), "key refused"); got != len(variants) {
		t.Errorf("stdout reports %d variants as 'key refused'; want %d:\n%s", got, len(variants), stdout.String())
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
	tr = &recordingTransport{t: t, status: http.StatusUnauthorized, body: keyRefusedBody}
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"-probe"}, envOf("ANTHROPIC_API_KEY", testKey), &stdout, &stderr, tr); code != 0 {
		t.Fatalf("-probe with a key exited %d; stderr: %s", code, stderr.String())
	}
	if tr.keys[0] != testKey {
		t.Error("the probe did not use the key from ANTHROPIC_API_KEY")
	}
	if strings.Contains(stdout.String()+stderr.String(), testKey) {
		t.Error("the probe printed the API key")
	}

	// A 400 invalid_request_error on a variant is reported and fails the probe.
	tr = &recordingTransport{t: t, status: http.StatusBadRequest, body: errorEnvelope("invalid_request_error", "effort: not supported "+testKey)}
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"-probe"}, envOf("ANTHROPIC_API_KEY", testKey), &stdout, &stderr, tr); code != exitFailure {
		t.Fatalf("-probe with rejected fields exited %d, want %d", code, exitFailure)
	}
	if !strings.Contains(stdout.String(), "field rejected: effort: not supported") {
		t.Errorf("the rejected field is not quoted:\n%s", stdout.String())
	}
	if strings.Contains(stdout.String()+stderr.String(), testKey) {
		t.Error("the probe printed the API key from a server message")
	}
}

func TestSpikeModelsGet(t *testing.T) {
	const body = `{"id":"` + claude.DefaultModel + `","type":"model","display_name":"Claude Spike Test","max_input_tokens":200000,"max_tokens":64000}`

	tr := &recordingTransport{t: t, body: body}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-models-get"}, envOf("ANTHROPIC_API_KEY", testKey), &stdout, &stderr, tr); code != 0 {
		t.Fatalf("-models-get exited %d; stderr: %s", code, stderr.String())
	}
	if tr.count() != 1 || tr.methods[0] != http.MethodGet || tr.paths[0] != "/v1/models/"+claude.DefaultModel {
		t.Errorf("requests = %d %v %v; want one GET of the model", tr.count(), tr.methods, tr.paths)
	}
	for _, want := range []string{claude.DefaultModel, "Claude Spike Test", "200000", "64000"} {
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
		body:   errorEnvelope("not_found_error", "model: nope for key "+testKey),
	}
	stdout.Reset()
	stderr.Reset()
	code := run([]string{"-models-get", "-model", "nope"}, envOf("ANTHROPIC_API_KEY", testKey), &stdout, &stderr, missing)
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
