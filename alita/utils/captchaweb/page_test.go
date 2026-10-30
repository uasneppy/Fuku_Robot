//go:build testtools

package captchaweb

import (
	"encoding/json"
	"html"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// testPageTables builds a complete string table for every page language from PageKeys.
func testPageTables() map[string]map[string]string {
	tables := make(map[string]map[string]string, len(PageLanguages))
	for _, lang := range PageLanguages {
		t := make(map[string]string, len(PageKeys))
		for _, key := range PageKeys {
			t[key] = lang + ":" + key
		}
		tables[lang] = t
	}
	return tables
}

func newTestPage(t *testing.T, siteKey string, tables map[string]map[string]string) *Page {
	t.Helper()
	p, err := NewPage(PageOptions{SiteKey: siteKey, Strings: tables})
	if err != nil {
		t.Fatalf("NewPage: %v", err)
	}
	return p
}

func servePage(p *Page, method string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, httptest.NewRequest(method, "/captcha", nil))
	return rec
}

var (
	nonceAttrRe  = regexp.MustCompile(`nonce="([^"]*)"`)
	openScriptRe = regexp.MustCompile(`<script[\s>]`)
	openStyleRe  = regexp.MustCompile(`<style[\s>]`)
	closeScriptR = regexp.MustCompile(`</script>`)
	scriptBodyRe = regexp.MustCompile(`(?s)<script[^>]*>.*?</script>`)
	styleBodyRe  = regexp.MustCompile(`(?s)<style[^>]*>.*?</style>`)
)

// markupOnly drops script and style element bodies so tag-level checks do not read code.
func markupOnly(body string) string {
	body = scriptBodyRe.ReplaceAllString(body, "")
	return styleBodyRe.ReplaceAllString(body, "")
}

func TestPageHeadersAndNonce(t *testing.T) {
	p := newTestPage(t, "0x4AAAAAAA-test", testPageTables())

	nonces := make([]string, 0, 2)
	for i := 0; i < 2; i++ {
		rec := servePage(p, http.MethodGet)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET status = %d, want 200", rec.Code)
		}
		body := rec.Body.String()

		// Every script and style element carries the same nonce, and it is the one in the CSP.
		matches := nonceAttrRe.FindAllStringSubmatch(body, -1)
		want := len(openScriptRe.FindAllString(body, -1)) + len(openStyleRe.FindAllString(body, -1))
		if want < 5 {
			t.Fatalf("found %d script and style tags, want at least 5", want)
		}
		if len(matches) != want {
			t.Fatalf("%d nonce attributes for %d script and style tags", len(matches), want)
		}
		nonce := html.UnescapeString(matches[0][1])
		for _, m := range matches {
			if got := html.UnescapeString(m[1]); got != nonce {
				t.Fatalf("nonce %q differs from %q inside one response", got, nonce)
			}
		}
		if len(nonce) < 22 {
			t.Fatalf("nonce %q is too short to be 16 random bytes", nonce)
		}
		nonces = append(nonces, nonce)

		if got, want := rec.Header().Get("Content-Security-Policy"), CSPFor(nonce); got != want {
			t.Errorf("CSP = %q, want %q", got, want)
		}
		wantHeaders := map[string]string{
			"Cache-Control":          "no-store",
			"Referrer-Policy":        "no-referrer",
			"X-Content-Type-Options": "nosniff",
			"Content-Type":           "text/html; charset=utf-8",
		}
		for k, v := range wantHeaders {
			if got := rec.Header().Get(k); got != v {
				t.Errorf("%s = %q, want %q", k, got, v)
			}
		}
		if _, ok := rec.Header()["X-Frame-Options"]; ok {
			t.Error("X-Frame-Options must not be sent")
		}
		for k := range rec.Header() {
			if strings.HasPrefix(strings.ToLower(k), "access-control-") {
				t.Errorf("CORS header %s must not be sent", k)
			}
		}
	}
	if nonces[0] == nonces[1] {
		t.Fatalf("two responses shared the nonce %q", nonces[0])
	}

	get := servePage(p, http.MethodGet)
	head := servePage(p, http.MethodHead)
	if head.Code != http.StatusOK {
		t.Fatalf("HEAD status = %d, want 200", head.Code)
	}
	if head.Body.Len() != 0 {
		t.Errorf("HEAD wrote %d body bytes", head.Body.Len())
	}
	for _, k := range []string{"Cache-Control", "Referrer-Policy", "X-Content-Type-Options", "Content-Type"} {
		if head.Header().Get(k) != get.Header().Get(k) {
			t.Errorf("HEAD %s = %q, GET has %q", k, head.Header().Get(k), get.Header().Get(k))
		}
	}
	if head.Header().Get("Content-Security-Policy") == "" {
		t.Error("HEAD carries no Content-Security-Policy")
	}

	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		if rec := servePage(p, m); rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s status = %d, want 405", m, rec.Code)
		}
	}
}

func TestPageEmbedsSafeStrings(t *testing.T) {
	tables := testPageTables()
	tables["en"]["captcha_web_page_title"] = "</script><script>alert(1)</script>  & more"
	siteKey := `0x"quoted" key`
	p := newTestPage(t, siteKey, tables)

	body := servePage(p, http.MethodGet).Body.String()

	if open, closing := len(openScriptRe.FindAllString(body, -1)), len(closeScriptR.FindAllString(body, -1)); open != closing {
		t.Fatalf("%d script elements but %d closing tags: a string broke out of its block", open, closing)
	}
	if strings.Contains(body, "alert(1)</script>") {
		t.Fatal("the injected script text reached the page unescaped")
	}

	blockRe := regexp.MustCompile(`(?s)<script[^>]*id="i18n"[^>]*>(.*?)</script>`)
	m := blockRe.FindStringSubmatch(body)
	if m == nil {
		t.Fatal("no i18n script block found")
	}
	var got map[string]map[string]string
	if err := json.Unmarshal([]byte(m[1]), &got); err != nil {
		t.Fatalf("i18n block is not JSON: %v", err)
	}
	if len(got) != len(tables) {
		t.Fatalf("i18n block holds %d languages, want %d", len(got), len(tables))
	}
	for lang, table := range tables {
		for k, v := range table {
			if got[lang][k] != v {
				t.Errorf("i18n[%s][%s] = %q, want %q", lang, k, got[lang][k], v)
			}
		}
	}

	keyRe := regexp.MustCompile(`data-sitekey="([^"]*)"`)
	km := keyRe.FindStringSubmatch(body)
	if km == nil {
		t.Fatal("no data-sitekey attribute found")
	}
	if strings.Contains(km[1], `"`) {
		t.Fatal("the site key's double quote ended its attribute")
	}
	if html.UnescapeString(km[1]) != siteKey {
		t.Fatalf("data-sitekey decodes to %q, want %q", html.UnescapeString(km[1]), siteKey)
	}
}

func TestPageStructure(t *testing.T) {
	p := newTestPage(t, "0x4AAAAAAA-test", testPageTables())
	body := servePage(p, http.MethodGet).Body.String()
	markup := markupOnly(body)

	for _, want := range []string{`data-action="captcha_join"`, `data-cdata-prefix="cd1:"`} {
		if !strings.Contains(body, want) {
			t.Errorf("page lacks %s", want)
		}
	}
	if n := strings.Count(body, `role="status"`); n != 1 {
		t.Errorf("%d status regions, want 1", n)
	}
	if !strings.Contains(body, `aria-live="polite"`) || !strings.Contains(body, `aria-atomic="true"`) {
		t.Error("status region lacks aria-live polite or aria-atomic true")
	}
	if n := strings.Count(body, `role="group"`); n != 1 {
		t.Errorf("%d widget containers, want 1", n)
	}
	if n := strings.Count(markup, "<button"); n != 1 {
		t.Errorf("%d buttons, want 1", n)
	}
	if !strings.Contains(markup, "<noscript") {
		t.Error("page has no noscript block")
	}

	iconRe := regexp.MustCompile(`<svg[^>]*\sdata-icon="([a-z]+)"`)
	var icons []string
	for _, m := range iconRe.FindAllStringSubmatch(markup, -1) {
		icons = append(icons, m[1])
	}
	sort.Strings(icons)
	wantIcons := []string{"check", "clock", "cross", "info", "retry", "shield", "slash", "spinner"}
	if strings.Join(icons, ",") != strings.Join(wantIcons, ",") {
		t.Errorf("icons = %v, want %v", icons, wantIcons)
	}
	if n := strings.Count(markup, "<svg"); n != 8 {
		t.Errorf("%d svg elements, want 8", n)
	}

	srcRe := regexp.MustCompile(`<script[^>]*\ssrc="([^"]+)"`)
	var srcs []string
	for _, m := range srcRe.FindAllStringSubmatch(body, -1) {
		srcs = append(srcs, html.UnescapeString(m[1]))
	}
	sort.Strings(srcs)
	wantSrcs := []string{
		"https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit&onload=captchaTurnstileReady",
		"https://telegram.org/js/telegram-web-app.js",
	}
	if strings.Join(srcs, "\n") != strings.Join(wantSrcs, "\n") {
		t.Errorf("external scripts = %v, want %v", srcs, wantSrcs)
	}

	// No inline style attribute and no inline event handler anywhere in the markup.
	inlineRe := regexp.MustCompile(`(?i)<[a-z][^>]*\s(style|on[a-z]+)\s*=`)
	if m := inlineRe.FindString(markup); m != "" {
		t.Errorf("inline style or handler attribute in %q", m)
	}
	// The script holds no way to run a string as code or to build markup from text.
	for _, banned := range []string{"eval(", "new Function", ".innerHTML", ".outerHTML", "document.write", "insertAdjacentHTML"} {
		if strings.Contains(body, banned) {
			t.Errorf("page contains %q", banned)
		}
	}

	viewport := regexp.MustCompile(`<meta[^>]*name="viewport"[^>]*>`).FindString(markup)
	if viewport == "" {
		t.Fatal("no viewport meta")
	}
	if !strings.Contains(viewport, "viewport-fit=cover") {
		t.Errorf("viewport %q lacks viewport-fit=cover", viewport)
	}
	if strings.Contains(viewport, "maximum-scale") || strings.Contains(viewport, "user-scalable") {
		t.Errorf("viewport %q blocks zoom", viewport)
	}
	if !regexp.MustCompile(`<meta[^>]*name="robots"[^>]*content="noindex"`).MatchString(markup) {
		t.Error("no robots noindex meta")
	}
}

func TestNewPageRefuses(t *testing.T) {
	if _, err := NewPage(PageOptions{SiteKey: "", Strings: testPageTables()}); err == nil {
		t.Error("an empty site key was accepted")
	}
	if _, err := NewPage(PageOptions{SiteKey: "k", Strings: nil}); err == nil {
		t.Error("a nil table was accepted")
	}
	for _, missing := range PageKeys {
		tables := testPageTables()
		delete(tables["en"], missing)
		if _, err := NewPage(PageOptions{SiteKey: "k", Strings: tables}); err == nil {
			t.Errorf("an en table without %s was accepted", missing)
		}
	}
}

func TestPageNoScriptFallback(t *testing.T) {
	p := newTestPage(t, "0x4AAAAAAA-test", testPageTables())
	body := servePage(p, http.MethodGet).Body.String()

	nsRe := regexp.MustCompile(`(?s)<noscript[^>]*>(.*?)</noscript>`)
	m := nsRe.FindStringSubmatch(body)
	if m == nil {
		t.Fatal("no noscript block")
	}
	tagRe := regexp.MustCompile(`<[^>]*>`)
	caption := strings.Join(strings.Fields(tagRe.ReplaceAllString(m[1], " ")), " ")
	if caption == "" || !strings.Contains(caption, "JavaScript") {
		t.Errorf("noscript caption = %q, want a hard-coded English line about JavaScript", caption)
	}
	if strings.Contains(caption, "en:captcha_web_") {
		t.Errorf("noscript caption %q was read from the string table", caption)
	}

	statusRe := regexp.MustCompile(`(?s)<section[^>]*role="status"[^>]*>(.*?)</section>`)
	sm := statusRe.FindStringSubmatch(body)
	if sm == nil {
		t.Fatal("no status section")
	}
	text := strings.Join(strings.Fields(tagRe.ReplaceAllString(sm[1], " ")), " ")
	for _, want := range []string{"Getting ready", "Setting up the check. This takes a moment."} {
		if !strings.Contains(text, want) {
			t.Errorf("first-paint status %q lacks %q", text, want)
		}
	}
	if strings.Contains(text, "captcha_web_") {
		t.Errorf("first-paint status %q was read from the string table", text)
	}
}

// TestPageLanguagesMatchScript pins IN-05: the language list lives in PageLanguages (the string
// tables the server embeds) and in the LANGS array of the page script (the languages the page
// picks from). A language in one and not the other is a table the page never reads or a pick
// with no strings, so the two must be the same list.
func TestPageLanguagesMatchScript(t *testing.T) {
	m := regexp.MustCompile(`(?m)^\s*var LANGS = \[([^\]]*)\];`).FindStringSubmatch(pageHTML)
	if m == nil {
		t.Fatal("the page script has no LANGS array")
	}
	var got []string
	for _, item := range strings.Split(m[1], ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if len(item) < 2 || item[0] != '\'' || item[len(item)-1] != '\'' {
			t.Fatalf("LANGS entry %q is not a single-quoted string", item)
		}
		got = append(got, item[1:len(item)-1])
	}
	if strings.Join(got, ",") != strings.Join(PageLanguages, ",") {
		t.Fatalf("LANGS = %v, PageLanguages = %v: the lists must be the same", got, PageLanguages)
	}
}
