//go:build testtools

package modules

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/utils/captchaweb"
)

// syntheticPageStrings builds a page string table with every key in every language.
func syntheticPageStrings() map[string]map[string]string {
	tables := make(map[string]map[string]string, len(captchaweb.PageLanguages))
	for _, lang := range captchaweb.PageLanguages {
		table := make(map[string]string, len(captchaweb.PageKeys))
		for _, key := range captchaweb.PageKeys {
			table[key] = lang + ":" + key
		}
		tables[lang] = table
	}
	return tables
}

// wiringConfig is a full production-shaped config over the env's Redis and fake Cloudflare.
func (e *captchaWebEnv) wiringConfig() CaptchaWebConfig {
	return CaptchaWebConfig{
		BaseURL:        "https://" + captchaWebTestHost,
		ShortName:      "verify",
		SiteKey:        "test-site-key",
		Secret:         e.secret,
		Redis:          e.redis,
		Siteverify:     e.rt.cfg.Siteverify,
		TrustedProxies: "10.0.0.0/8",
		PageStrings:    syntheticPageStrings(),
		StartNotReady:  true,
	}
}

// wiringMux mounts CaptchaWebRoutes() on a ServeMux the way main does through Server.Handle.
func wiringMux() *http.ServeMux {
	mux := http.NewServeMux()
	for pattern, handler := range CaptchaWebRoutes() {
		mux.Handle(pattern, handler)
	}
	return mux
}

func muxDo(mux *http.ServeMux, method, target, body string, header map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	for k, v := range header {
		req.Header.Set(k, v)
	}
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	return rr
}

func TestCaptchaWebWiring(t *testing.T) {
	member := gotgbot.User{Id: 42, FirstName: "Member"}

	t.Run("no runtime installed", func(t *testing.T) {
		captchaWebCurrent.Store(nil)
		if routes := CaptchaWebRoutes(); routes != nil {
			t.Fatalf("CaptchaWebRoutes() = %v, want nil without a runtime", routes)
		}
		MarkCaptchaWebReady() // must not panic
	})

	t.Run("a bad trusted proxies value installs nothing", func(t *testing.T) {
		env := newCaptchaWebEnv(t)
		captchaWebCurrent.Store(nil)
		cfg := env.wiringConfig()
		cfg.TrustedProxies = "not-an-address"
		if err := ConfigureCaptchaWeb(env.bot, cfg); err == nil {
			t.Fatal("ConfigureCaptchaWeb accepted a bad CAPTCHA_WEB_TRUSTED_PROXIES")
		}
		if captchaWebCurrent.Load() != nil {
			t.Fatal("a failed ConfigureCaptchaWeb installed a runtime")
		}
		if CaptchaWebRoutes() != nil {
			t.Fatal("CaptchaWebRoutes() is not nil after a failed configuration")
		}
	})

	t.Run("routes, readiness and a full join through the mux", func(t *testing.T) {
		env := newCaptchaWebEnv(t)
		if err := ConfigureCaptchaWeb(env.bot, env.wiringConfig()); err != nil {
			t.Fatalf("ConfigureCaptchaWeb() error = %v", err)
		}
		// Point the env at the runtime production would have installed.
		env.rt = captchaWebCurrent.Load()
		captchaWebProbeAt.Store(0)

		var patterns []string
		for pattern := range CaptchaWebRoutes() {
			patterns = append(patterns, pattern)
		}
		sort.Strings(patterns)
		want := []string{captchaweb.RouteHealth, captchaweb.RoutePage, captchaweb.RouteVerify}
		sort.Strings(want)
		if strings.Join(patterns, "|") != strings.Join(want, "|") {
			t.Fatalf("CaptchaWebRoutes() patterns = %v, want %v", patterns, want)
		}
		mux := wiringMux()

		page := muxDo(mux, http.MethodGet, "/captcha", "", nil)
		if page.Code != http.StatusOK || !strings.HasPrefix(page.Header().Get("Content-Type"), "text/html") {
			t.Fatalf("GET /captcha = %d %q, want 200 text/html", page.Code, page.Header().Get("Content-Type"))
		}
		if page.Header().Get("Content-Security-Policy") == "" {
			t.Fatal("GET /captcha carries no Content-Security-Policy")
		}
		if !strings.Contains(page.Body.String(), "en:captcha_web_page_title") {
			t.Fatal("the page does not carry the configured string tables")
		}

		// Before MarkCaptchaWebReady nothing is decided and health says unavailable.
		token := env.join(member)
		initData := captchaweb.InitDataForTest(env.bot.Token, member.Id, member.FirstName, "en", token, time.Now())
		body := env.verifyJSON(initData, captchaweb.TurnstileTokenForTest(token), token)
		jsonHeader := map[string]string{"Content-Type": "application/json"}

		early := muxDo(mux, http.MethodPost, "/captcha/verify", body, jsonHeader)
		var earlyAnswer captchaweb.Answer
		if err := json.Unmarshal(early.Body.Bytes(), &earlyAnswer); err != nil {
			t.Fatalf("early verify body %q: %v", early.Body.String(), err)
		}
		if early.Code != http.StatusServiceUnavailable || earlyAnswer.Result != captchaweb.ResultRetry {
			t.Fatalf("verify before ready = %d %q, want 503 retry", early.Code, earlyAnswer.Result)
		}
		if health := muxDo(mux, http.MethodGet, "/captcha/health", "", nil); health.Code != http.StatusServiceUnavailable {
			t.Fatalf("health before ready = %d, want 503", health.Code)
		}
		if env.attempt(member.Id) == nil {
			t.Fatal("a verify before ready decided the attempt")
		}

		MarkCaptchaWebReady()

		if health := muxDo(mux, http.MethodGet, "/captcha/health", "", nil); health.Code != http.StatusOK {
			t.Fatalf("health after ready = %d, want 200", health.Code)
		}
		pass := muxDo(mux, http.MethodPost, "/captcha/verify", body, jsonHeader)
		var answer captchaweb.Answer
		if err := json.Unmarshal(pass.Body.Bytes(), &answer); err != nil {
			t.Fatalf("verify body %q: %v", pass.Body.String(), err)
		}
		if pass.Code != http.StatusOK || answer.Result != captchaweb.ResultPassed {
			t.Fatalf("verify after ready = %d %q, want 200 passed", pass.Code, answer.Result)
		}
		if env.attempt(member.Id) != nil {
			t.Fatal("the attempt is still pending after a pass")
		}
		if len(env.client.callsFor("restrictChatMember")) == 0 {
			t.Fatal("a pass through the mux did not unmute the member")
		}
	})
}

func TestCaptchaWebStartupWarnings(t *testing.T) {
	t.Run("nothing set returns both warnings", func(t *testing.T) {
		warnings := CaptchaWebStartupWarnings(false, "")
		if len(warnings) != 2 {
			t.Fatalf("got %d warnings %q, want 2", len(warnings), warnings)
		}
		first, second := warnings[0], warnings[1]
		if !strings.Contains(first, "METRICS_AUTH_TOKEN") || !strings.Contains(first, "/captcha") {
			t.Fatalf("first warning %q must name METRICS_AUTH_TOKEN and /captcha", first)
		}
		if !strings.Contains(second, "CAPTCHA_WEB_TRUSTED_PROXIES") {
			t.Fatalf("second warning %q must name CAPTCHA_WEB_TRUSTED_PROXIES", second)
		}
	})
	t.Run("both set returns none", func(t *testing.T) {
		if warnings := CaptchaWebStartupWarnings(true, "10.0.0.1"); len(warnings) != 0 {
			t.Fatalf("got warnings %q, want none", warnings)
		}
	})
	t.Run("only the proxy list missing", func(t *testing.T) {
		warnings := CaptchaWebStartupWarnings(true, "")
		if len(warnings) != 1 || !strings.Contains(warnings[0], "CAPTCHA_WEB_TRUSTED_PROXIES") {
			t.Fatalf("got %q, want only the proxy warning", warnings)
		}
	})
}
