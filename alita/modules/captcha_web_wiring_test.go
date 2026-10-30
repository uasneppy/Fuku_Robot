//go:build testtools

package modules

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/divkix/Alita_Robot/alita/db"
	dbcache "github.com/divkix/Alita_Robot/alita/db/cache"
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
		captchaWebProbe.Store(nil)

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

// statsDeltaWant checks that every counter moved by exactly the wanted amount since before.
func statsDeltaWant(t *testing.T, before, want captchaweb.Stats) {
	t.Helper()
	after := captchaweb.StatsSnapshot()
	got := captchaweb.Stats{
		Issued:  after.Issued - before.Issued,
		Passed:  after.Passed - before.Passed,
		Failed:  after.Failed - before.Failed,
		Expired: after.Expired - before.Expired,
		Invalid: after.Invalid - before.Invalid,
		Retry:   after.Retry - before.Retry,
		Limited: after.Limited - before.Limited,
		Errors:  after.Errors - before.Errors,
	}
	if got != want {
		t.Fatalf("counter deltas = %+v, want %+v", got, want)
	}
}

// expireAttemptNow makes the attempt's time run out without running any timer.
func expireAttemptNow(t *testing.T, attemptID uint) {
	t.Helper()
	if err := db.DB.Model(&db.CaptchaAttempts{}).Where("id = ?", attemptID).
		Updates(map[string]any{
			"created_at": time.Now().Add(-2 * time.Minute),
			"expires_at": time.Now().Add(-time.Minute),
		}).Error; err != nil {
		t.Fatalf("expire the attempt: %v", err)
	}
}

func TestCaptchaWebStatsSection(t *testing.T) {
	member := gotgbot.User{Id: 42, FirstName: "Member"}
	other := gotgbot.User{Id: 43, FirstName: "Other"}
	upstream500 := captchaweb.SiteverifyAnswerForTest(http.StatusInternalServerError, `{"error":"upstream"}`)

	t.Run("a join and a pass", func(t *testing.T) {
		env := newCaptchaWebEnv(t)
		before := captchaweb.StatsSnapshot()
		token := env.join(member)
		statsDeltaWant(t, before, captchaweb.Stats{Issued: 1})

		mid := captchaweb.StatsSnapshot()
		if status, answer := env.verify(member, token); status != http.StatusOK || answer.Result != captchaweb.ResultPassed {
			t.Fatalf("verify = %d %+v, want passed", status, answer)
		}
		statsDeltaWant(t, mid, captchaweb.Stats{Passed: 1})

		// The remembered answer of a repeated tap is the same event: it is not counted again.
		mid = captchaweb.StatsSnapshot()
		if status, answer := env.verify(member, token); status != http.StatusOK || answer.Result != captchaweb.ResultPassed {
			t.Fatalf("second verify = %d %+v, want the remembered pass", status, answer)
		}
		statsDeltaWant(t, mid, captchaweb.Stats{})
	})

	t.Run("a failed solve", func(t *testing.T) {
		env := newCaptchaWebEnv(t)
		token := env.join(member)
		env.setSiteverify(captchaweb.SiteverifyAnswerForTest(http.StatusOK, `{"success":false,"error-codes":["invalid-input-response"]}`))
		before := captchaweb.StatsSnapshot()
		if status, answer := env.verify(member, token); status != http.StatusOK || answer.Result != captchaweb.ResultFailed {
			t.Fatalf("verify = %d %+v, want failed", status, answer)
		}
		statsDeltaWant(t, before, captchaweb.Stats{Failed: 1})
	})

	t.Run("a Cloudflare 500 is a retry and no error", func(t *testing.T) {
		env := newCaptchaWebEnv(t)
		token := env.join(member)
		env.setSiteverify(upstream500)
		before := captchaweb.StatsSnapshot()
		if status, answer := env.verify(member, token); status != http.StatusOK || answer.Result != captchaweb.ResultRetry {
			t.Fatalf("verify = %d %+v, want retry", status, answer)
		}
		statsDeltaWant(t, before, captchaweb.Stats{Retry: 1})
	})

	t.Run("a wrong user is invalid", func(t *testing.T) {
		env := newCaptchaWebEnv(t)
		token := env.join(member)
		before := captchaweb.StatsSnapshot()
		if status, answer := env.verify(other, token); status != http.StatusOK || answer.Result != captchaweb.ResultNotForYou {
			t.Fatalf("verify = %d %+v, want not_for_you", status, answer)
		}
		statsDeltaWant(t, before, captchaweb.Stats{Invalid: 1})
	})

	t.Run("a hostname mismatch is an error and a retry", func(t *testing.T) {
		env := newCaptchaWebEnv(t)
		token := env.join(member)
		env.setSiteverify(captchaweb.SiteverifyPassForTest("not-our-host.example.net"))
		before := captchaweb.StatsSnapshot()
		if status, answer := env.verify(member, token); status != http.StatusOK || answer.Result != captchaweb.ResultRetry {
			t.Fatalf("verify = %d %+v, want retry", status, answer)
		}
		statsDeltaWant(t, before, captchaweb.Stats{Errors: 1, Retry: 1})
	})

	t.Run("a spent budget is limited and no retry", func(t *testing.T) {
		env := newCaptchaWebEnv(t)
		token := env.join(member)
		env.setSiteverify(upstream500)
		// The per-token budget is 8 a minute: eight retries, then the ninth is refused.
		for i := 0; i < 8; i++ {
			if status, answer := env.verify(member, token); status != http.StatusOK || answer.Result != captchaweb.ResultRetry {
				t.Fatalf("verify %d = %d %+v, want retry", i+1, status, answer)
			}
		}
		before := captchaweb.StatsSnapshot()
		if status, answer := env.verify(member, token); status != http.StatusTooManyRequests || answer.Result != captchaweb.ResultRetry {
			t.Fatalf("ninth verify = %d %+v, want 429 retry", status, answer)
		}
		statsDeltaWant(t, before, captchaweb.Stats{Limited: 1})
	})

	t.Run("an expired Turnstile attempt counts once, a math attempt not at all", func(t *testing.T) {
		env := newCaptchaWebEnv(t)
		token := env.join(member)
		attempt := env.attempt(member.Id)
		expireAttemptNow(t, attempt.ID)

		// The member's late check expires it through the backend: one Expired, and the
		// expired answer itself is not counted again.
		before := captchaweb.StatsSnapshot()
		if status, answer := env.verify(member, token); status != http.StatusOK || answer.Result != captchaweb.ResultExpired {
			t.Fatalf("verify = %d %+v, want expired", status, answer)
		}
		statsDeltaWant(t, before, captchaweb.Stats{Expired: 1})

		// A timer on a second Turnstile attempt: the same function, one more Expired.
		token2 := env.join(other)
		_ = token2
		second := env.attempt(other.Id)
		expireAttemptNow(t, second.ID)
		before = captchaweb.StatsSnapshot()
		if _, err := expireCaptchaAttempt(env.bot, second); err != nil {
			t.Fatalf("expireCaptchaAttempt() error = %v", err)
		}
		statsDeltaWant(t, before, captchaweb.Stats{Expired: 1})

		// A math attempt expiring is not a web captcha event.
		if err := db.DB.Model(&db.CaptchaSettings{}).Where("chat_id = ?", env.chat.Id).
			Update("captcha_mode", "math").Error; err != nil {
			t.Fatalf("set math mode: %v", err)
		}
		dbcache.DeleteCache(dbcache.CacheKey("captcha_settings", env.chat.Id))
		third := gotgbot.User{Id: 44, FirstName: "Third"}
		joinWithoutToken(t, env, third)
		mathAttempt := env.attempt(third.Id)
		if mathAttempt == nil || captchaweb.IsReservedAnswer(mathAttempt.Answer) {
			t.Fatalf("attempt = %+v, want a math attempt", mathAttempt)
		}
		expireAttemptNow(t, mathAttempt.ID)
		before = captchaweb.StatsSnapshot()
		if _, err := expireCaptchaAttempt(env.bot, mathAttempt); err != nil {
			t.Fatalf("expireCaptchaAttempt(math) error = %v", err)
		}
		statsDeltaWant(t, before, captchaweb.Stats{})
	})

	t.Run("the owner's stats text carries the section after the AI checks", func(t *testing.T) {
		text := ownerStatsText(t)
		ai := strings.Index(text, "AI checks (this replica")
		web := strings.Index(text, "Web captcha (this replica, since its last restart)")
		if ai < 0 || web < 0 || web < ai {
			t.Fatalf("stats text has the AI section at %d and the web captcha section at %d, want both with the web one last:\n%s", ai, web, text)
		}
	})

	t.Run("the section prints eight numbers and nothing else", func(t *testing.T) {
		env := newCaptchaWebEnv(t)
		token := env.join(member)
		if status, answer := env.verify(member, token); status != http.StatusOK || answer.Result != captchaweb.ResultPassed {
			t.Fatalf("verify = %d %+v, want passed", status, answer)
		}

		section := captchaWebStatsSection()
		if !strings.Contains(section, "Web captcha (this replica, since its last restart)") {
			t.Fatalf("section %q lacks its heading", section)
		}
		for _, word := range []string{"issued", "passed", "failed", "expired", "invalid", "retry", "limited", "errors"} {
			if !regexp.MustCompile(word + ` \d+`).MatchString(section) {
				t.Fatalf("section %q lacks %q followed by a number", section, word)
			}
		}
		lower := strings.ToLower(section)
		for _, banned := range []string{token, "init", "cloudflare", "turnstile", "token", fmt.Sprint(env.chat.Id), env.secret} {
			if banned != "" && strings.Contains(lower, strings.ToLower(banned)) {
				t.Fatalf("section %q contains %q", section, banned)
			}
		}
	})
}

// TestCaptchaWebAvailabilityMemoIsOneUnit pins IN-04: the memo of the last Redis probe holds the
// runtime, the time and the answer together. With the runtime swapped back and forth under
// concurrent callers, a caller must never be told the answer of the other runtime's probe.
func TestCaptchaWebAvailabilityMemoIsOneUnit(t *testing.T) {
	env := newCaptchaWebEnv(t)
	dead := miniredis.RunT(t)
	deadClient := redis.NewClient(&redis.Options{Addr: dead.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = deadClient.Close() })
	dead.Close()
	downRT, err := newCaptchaWebRuntime(env.bot, CaptchaWebConfig{
		BaseURL:   "https://" + captchaWebTestHost,
		ShortName: "verify",
		SiteKey:   "test-site-key",
		Secret:    env.secret,
		Redis:     deadClient,
	})
	if err != nil {
		t.Fatalf("newCaptchaWebRuntime() error = %v", err)
	}
	upRT := env.rt

	var (
		wg   sync.WaitGroup
		stop atomic.Bool
		bad  atomic.Int64
	)
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for !stop.Load() {
				// A caller that read this runtime must get this runtime's own answer. The
				// installed runtime is swapped by the other goroutine, so the answer is
				// checked against the runtime the call returned.
				got, ok := captchaWebAvailable(env.bot)
				if got == nil {
					continue
				}
				if want := got == upRT; ok != want {
					bad.Add(1)
				}
			}
		}()
	}
	swapper := make(chan struct{})
	go func() {
		defer close(swapper)
		for i := 0; i < 200; i++ {
			if i%2 == 0 {
				captchaWebCurrent.Store(upRT)
			} else {
				captchaWebCurrent.Store(downRT)
			}
			time.Sleep(time.Millisecond)
		}
	}()
	<-swapper
	stop.Store(true)
	wg.Wait()
	if n := bad.Load(); n != 0 {
		t.Fatalf("%d callers were given the answer of another runtime's probe", n)
	}
}
