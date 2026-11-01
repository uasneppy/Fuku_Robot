package captchaweb

import (
	"bytes"
	"crypto/rand"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
)

//go:embed page/captcha.html
var pageHTML string

// PageKeys are the locale keys the page reads, in the order of the UI contract. The page
// looks a key up by its full name; nothing builds a key from user input.
var PageKeys = []string{
	"captcha_web_page_title",
	"captcha_web_loading_title",
	"captcha_web_loading_body",
	"captcha_web_ready_title",
	"captcha_web_ready_body",
	"captcha_web_widget_label",
	"captcha_web_verifying_title",
	"captcha_web_verifying_body",
	"captcha_web_passed_title",
	"captcha_web_passed_body",
	"captcha_web_failed_title",
	"captcha_web_failed_body_kick",
	"captcha_web_failed_body_ban",
	"captcha_web_failed_body_mute",
	"captcha_web_expired_title",
	"captcha_web_expired_body_kick",
	"captcha_web_expired_body_ban",
	"captcha_web_expired_body_mute",
	"captcha_web_retry_title",
	"captcha_web_retry_body",
	"captcha_web_retry_button",
	"captcha_web_invalid_title",
	"captcha_web_invalid_body",
	"captcha_web_unsupported_title",
	"captcha_web_unsupported_body",
	"captcha_web_close_button",
}

// PageLanguages are the languages the page carries a string table for. The page picks one
// from the member's Telegram language, for display only.
var PageLanguages = []string{"en", "es", "fr", "hi", "id", "pt", "ru"}

// PageOptions configures NewPage.
type PageOptions struct {
	// SiteKey is the public Turnstile site key the widget is rendered with.
	SiteKey string
	// Strings maps a language to its page strings, keyed by PageKeys. The en table must
	// hold every key; another language may lack keys and then falls back to en in the page.
	Strings map[string]map[string]string
}

// Page serves the Mini App page. It is its own http.Handler and holds no per-request state.
type Page struct {
	tmpl    *template.Template
	siteKey string
	i18n    template.JS
}

// pageData is what the template receives. No field reaches the main script: that script
// reads the data-* attributes and the i18n block only.
type pageData struct {
	Nonce       string
	SiteKey     string
	Action      string
	CDataPrefix string
	I18N        template.JS
}

// NewPage parses the embedded page once and marshals the string tables once. It refuses an
// empty site key and an en table that is missing any of PageKeys (or holds an empty value).
func NewPage(opts PageOptions) (*Page, error) {
	if opts.SiteKey == "" {
		return nil, errors.New("captchaweb: page needs a Turnstile site key")
	}
	en := opts.Strings["en"]
	if en == nil {
		return nil, errors.New("captchaweb: page needs an en string table")
	}
	for _, key := range PageKeys {
		if en[key] == "" {
			return nil, fmt.Errorf("captchaweb: en page table lacks %s", key)
		}
	}

	// Only the known languages and keys are embedded, so the block stays bounded.
	tables := make(map[string]map[string]string, len(PageLanguages))
	for _, lang := range PageLanguages {
		src, ok := opts.Strings[lang]
		if !ok {
			continue
		}
		table := make(map[string]string, len(PageKeys))
		for _, key := range PageKeys {
			if v, ok := src[key]; ok && v != "" {
				table[key] = v
			}
		}
		tables[lang] = table
	}
	// json.Marshal escapes <, > and & and the line separators U+2028/U+2029, so the block
	// cannot close its own script element.
	raw, err := json.Marshal(tables)
	if err != nil {
		return nil, fmt.Errorf("captchaweb: marshal page strings: %w", err)
	}

	tmpl, err := template.New("captcha").Parse(pageHTML)
	if err != nil {
		return nil, fmt.Errorf("captchaweb: parse page: %w", err)
	}
	return &Page{tmpl: tmpl, siteKey: opts.SiteKey, i18n: template.JS(raw)}, nil // #nosec G203 -- output of json.Marshal
}

// CSPFor returns the Content-Security-Policy of one response whose scripts and style carry
// nonce. The test compares against this function, not a copy of the string.
func CSPFor(nonce string) string {
	return "default-src 'none'; " +
		"script-src 'nonce-" + nonce + "' https://telegram.org https://challenges.cloudflare.com; " +
		"style-src 'nonce-" + nonce + "'; " +
		"frame-src https://challenges.cloudflare.com; " +
		"connect-src 'self'; " +
		"img-src 'self' data:; " +
		"base-uri 'none'; " +
		"form-action 'none'; " +
		"frame-ancestors https://web.telegram.org https://webk.telegram.org https://webz.telegram.org"
}

// ServeHTTP answers GET and HEAD with the page and a fresh nonce; anything else gets 405.
// It never logs the request. It sends no X-Frame-Options (it would block Telegram Web's
// embedded Mini App) and no CORS header.
func (p *Page) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		w.Header().Set("Cache-Control", "no-store")
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	var nonceBytes [16]byte
	if _, err := rand.Read(nonceBytes[:]); err != nil {
		w.Header().Set("Cache-Control", "no-store")
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	nonce := base64.StdEncoding.EncodeToString(nonceBytes[:])

	var buf bytes.Buffer
	err := p.tmpl.Execute(&buf, pageData{
		Nonce:       nonce,
		SiteKey:     p.siteKey,
		Action:      TurnstileAction,
		CDataPrefix: CDataPrefix,
		I18N:        p.i18n,
	})
	if err != nil {
		w.Header().Set("Cache-Control", "no-store")
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Content-Security-Policy", CSPFor(nonce))
	h.Set("Cache-Control", "no-store")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Length", strconv.Itoa(buf.Len()))
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(buf.Bytes())
}
