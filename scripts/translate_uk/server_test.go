package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// do sends one request straight to the handler. httptest.NewRequest gives the
// request the Host of its URL, so tests that need another Host set req.Host.
func do(t *testing.T, h http.Handler, method, path string, body []byte, hdr map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, "http://127.0.0.1:8080"+path, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range hdr {
		if k == "Host" {
			req.Host = v
		} else {
			req.Header.Set(k, v)
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func saveBody(key string, uk ...string) []byte {
	b, _ := json.Marshal(map[string]any{"key": key, "uk": uk})
	return b
}

func TestStateReflectsDiskOnEveryRequest(t *testing.T) {
	p := mustInit(t)
	h := newServer(p)
	decode := func() State {
		rec := do(t, h, "GET", "/api/state", nil, nil)
		if rec.Code != 200 {
			t.Fatalf("status %d: %s", rec.Code, rec.Body)
		}
		var s State
		if err := json.Unmarshal(rec.Body.Bytes(), &s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	if keyOf(t, ptr(decode()), "plain_yes").UK[0] == "змінено ззовні" {
		t.Fatal("test setup is wrong")
	}
	editUK(t, p, "plain_yes", []string{"змінено ззовні"})
	if got := keyOf(t, ptr(decode()), "plain_yes").UK[0]; got != "змінено ззовні" {
		t.Fatalf("the server served a stale copy: %q", got)
	}
}

func ptr[T any](v T) *T { return &v }

func TestSaveAndConfirmRoundTrip(t *testing.T) {
	p := mustInit(t)
	h := newServer(p)

	rec := do(t, h, "POST", "/api/confirm", saveBody("admin_ok", "Готово <b>%s</b>"), nil)
	if rec.Code != 200 {
		t.Fatalf("confirm: %d %s", rec.Code, rec.Body)
	}
	var k KeyState
	if err := json.Unmarshal(rec.Body.Bytes(), &k); err != nil || !k.Confirmed {
		t.Fatalf("confirm response %+v %v", k, err)
	}
	if !keyOf(t, mustLoad(t, p), "admin_ok").Confirmed {
		t.Fatal("the confirmation was not stored")
	}

	rec = do(t, h, "POST", "/api/save", saveBody("admin_ok", "Зроблено <b>%s</b>"), nil)
	if rec.Code != 200 || keyOf(t, mustLoad(t, p), "admin_ok").Confirmed {
		t.Fatalf("a changed save must drop the confirmation: %d %s", rec.Code, rec.Body)
	}
}

func TestRefusedSaveIs422WithIssuesAndWritesNothing(t *testing.T) {
	p := mustInit(t)
	h := newServer(p)
	before := readFile(t, p.UkPath) + readFile(t, p.ProgressPath)
	rec := do(t, h, "POST", "/api/confirm", saveBody("admin_ok", "Готово"), nil)
	if rec.Code != 422 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var resp struct{ Issues []Issue }
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || !hasCode(resp.Issues, "placeholders") {
		t.Fatalf("body %s", rec.Body)
	}
	if readFile(t, p.UkPath)+readFile(t, p.ProgressPath) != before {
		t.Fatal("a refused save changed a file")
	}
}

func TestUnknownKeyIs404(t *testing.T) {
	h := newServer(mustInit(t))
	if rec := do(t, h, "POST", "/api/save", saveBody("no_such_key", "x"), nil); rec.Code != 404 {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestMalformedBodiesAre400(t *testing.T) {
	h := newServer(mustInit(t))
	for name, body := range map[string]string{
		"not json":      "{",
		"unknown field": `{"key":"admin_ok","uk":["x"],"extra":1}`,
		"missing key":   `{"uk":["x"]}`,
		"missing uk":    `{"key":"admin_ok"}`,
		"trailing data": `{"key":"admin_ok","uk":["x"]} {}`,
	} {
		t.Run(name, func(t *testing.T) {
			if rec := do(t, h, "POST", "/api/save", []byte(body), nil); rec.Code != 400 {
				t.Fatalf("status %d: %s", rec.Code, rec.Body)
			}
		})
	}
}

func TestForeignHostIsRefused(t *testing.T) {
	p := mustInit(t)
	h := newServer(p)
	before := readFile(t, p.UkPath) + readFile(t, p.ProgressPath)
	evil := map[string]string{"Host": "evil.example"}
	for _, c := range []struct {
		method, path string
		body         []byte
	}{
		{"GET", "/", nil},
		{"GET", "/api/state", nil},
		{"POST", "/api/confirm", saveBody("admin_ok", "Готово <b>%s</b>")},
	} {
		if rec := do(t, h, c.method, c.path, c.body, evil); rec.Code != 403 {
			t.Errorf("%s %s with a foreign Host: status %d, want 403", c.method, c.path, rec.Code)
		}
	}
	if readFile(t, p.UkPath)+readFile(t, p.ProgressPath) != before {
		t.Fatal("a refused request wrote something")
	}
	for _, host := range []string{"127.0.0.1:8080", "localhost:9999", "LOCALHOST", "[::1]:8080", "[::1]"} {
		if rec := do(t, h, "GET", "/api/state", nil, map[string]string{"Host": host}); rec.Code != 200 {
			t.Errorf("Host %q should be allowed, got %d", host, rec.Code)
		}
	}
}

func TestForeignOriginIsRefused(t *testing.T) {
	p := mustInit(t)
	h := newServer(p)
	body := saveBody("admin_ok", "Готово <b>%s</b>")
	for _, origin := range []string{"https://evil.example", "null", "http://127.0.0.1.evil.example"} {
		if rec := do(t, h, "POST", "/api/confirm", body, map[string]string{"Origin": origin}); rec.Code != 403 {
			t.Errorf("Origin %q: status %d, want 403", origin, rec.Code)
		}
	}
	if keyOf(t, mustLoad(t, p), "admin_ok").Confirmed {
		t.Fatal("a refused request confirmed a key")
	}
	for _, origin := range []string{"http://127.0.0.1:8080", "http://localhost:3000", ""} {
		if rec := do(t, h, "POST", "/api/save", body, map[string]string{"Origin": origin}); rec.Code != 200 {
			t.Errorf("Origin %q should be allowed, got %d: %s", origin, rec.Code, rec.Body)
		}
	}
}

func TestNonJSONContentTypeIsRefused(t *testing.T) {
	h := newServer(mustInit(t))
	body := saveBody("admin_ok", "Готово <b>%s</b>")
	for _, ct := range []string{"text/plain", "application/x-www-form-urlencoded", "multipart/form-data"} {
		if rec := do(t, h, "POST", "/api/save", body, map[string]string{"Content-Type": ct}); rec.Code != 415 {
			t.Errorf("Content-Type %q: status %d, want 415", ct, rec.Code)
		}
	}
	if rec := do(t, h, "POST", "/api/save", body, map[string]string{"Content-Type": "application/json; charset=utf-8"}); rec.Code != 200 {
		t.Errorf("a charset parameter must be accepted, got %d", rec.Code)
	}
}

func TestRequestBodyIsCapped(t *testing.T) {
	h := newServer(mustInit(t))
	big := saveBody("admin_ok", strings.Repeat("я", 2<<20))
	if rec := do(t, h, "POST", "/api/save", big, nil); rec.Code != 413 {
		t.Fatalf("status %d, want 413", rec.Code)
	}
}

func TestWrongMethodsAreRefused(t *testing.T) {
	h := newServer(mustInit(t))
	if rec := do(t, h, "GET", "/api/save", nil, nil); rec.Code != 405 || rec.Header().Get("Allow") != "POST" {
		t.Errorf("GET /api/save: %d %q", rec.Code, rec.Header().Get("Allow"))
	}
	if rec := do(t, h, "POST", "/api/state", []byte("{}"), nil); rec.Code != 405 {
		t.Errorf("POST /api/state: %d", rec.Code)
	}
}

func TestIndexPageIsServedWithProtectiveHeaders(t *testing.T) {
	h := newServer(mustInit(t))
	rec := do(t, h, "GET", "/", nil, nil)
	if rec.Code != 200 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") || rec.Body.Len() == 0 {
		t.Fatalf("GET /: %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	if rec.Header().Get("Content-Security-Policy") == "" || rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("missing protective headers: %v", rec.Header())
	}
	if rec := do(t, h, "GET", "/missing", nil, nil); rec.Code != 404 {
		t.Errorf("GET /missing: %d", rec.Code)
	}
}

func TestServeRefusesNonLoopbackAddress(t *testing.T) {
	p := mustInit(t)
	for _, addr := range []string{"0.0.0.0:8080", ":8080", "192.168.1.5:8080", "example.com:8080"} {
		err := serve(addr, p, io.Discard)
		if err == nil || !strings.Contains(err.Error(), "loopback") {
			t.Errorf("serve(%q) = %v, want a loopback refusal", addr, err)
		}
	}
}

func TestRunRefusesNonLoopbackAddress(t *testing.T) {
	p := mustInit(t)
	code, _, errOut := runIn(t, p, "-addr", "0.0.0.0:0")
	if code != 2 || !strings.Contains(errOut, "loopback") {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
}
