# Phase 8: Turnstile Web Captcha - Pattern Map

**Mapped:** 2026-10-29
**Files analyzed:** 34 new or modified (grouped below)
**Analogs found:** 31 / 34 (all analog paths verified git-tracked; no gitignored mirror paths)

Decision IDs (D-xx), Pattern N, Pitfall N and line numbers for `captcha.go` refer to `08-CONTEXT.md` and `08-RESEARCH.md`. Line numbers below were read this session.

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|---|---|---|---|---|
| `alita/utils/captchaweb/store.go` (+ `limiter.go`) | service (Redis Lua) | request-response, event-driven (one-time token) | `alita/modules/raid_counters.go` | role-match (same Lua + `{key}` + miniredis idiom) |
| `alita/utils/captchaweb/siteverify.go` | client (stdlib leaf) | request-response | `alita/utils/gemini/client.go` | exact |
| `alita/utils/captchaweb/initdata.go`, `clientip.go`, `settings.go`, `stats.go` | utility | transform | `alita/utils/gemini/client.go` (leaf style, `ValidModel` regex helper); `alita/modules/raid_ai.go:375-424` (atomic counters) | partial |
| `alita/utils/captchaweb/handler.go` (+ `page/captcha.html`) | controller (HTTP) | request-response | `alita/utils/httpserver/server.go` (`webhookHandler`, `requireMetricsAuth`, `RegisterDBMetrics`) | role-match |
| `alita/utils/captchaweb/*_test.go` + fake-Cloudflare helper | test | request-response | `alita/utils/gemini/gemini_test.go:64-100`, `leaf_test.go` | exact |
| `alita/utils/httpserver/server.go` (add `Handle`, endpoint log) | controller/config | request-response | itself (`RegisterDBMetrics`, `Start` endpoints list) | exact |
| `alita/modules/captcha_web.go` | service (Backend impl) | CRUD + event-driven | `alita/modules/raid_counters.go` (`raidRedis`), `devs.go:362-379` (stats), `raid_ai.go:375-424` | role-match |
| `alita/modules/captcha.go` (edits) | controller | request-response | itself | exact |
| `alita/modules/devs.go` (stats section) | controller | request-response | `raidAIStatsSection` same file | exact |
| `alita/modules/captcha_web_test.go` | test | integration | `alita/modules/lockdown_fake_test.go`, `test_harness_test.go:42-74` | role-match |
| `main.go` (route registration, readiness mark) | config/wiring | request-response | itself (`:235-241`, `:434-440`) | exact |
| `alita/db/models/captcha.go`, `alita/db/captcha/repository.go` | model / repository | CRUD | themselves | exact |
| `alita/db/constraints_test.go`, new `TestSetCaptchaModeAcceptsTurnstile` | test | CRUD | `constraints_test.go:127-139` | exact |
| `migrations/20261030120000_allow_turnstile_captcha_mode.sql` | migration | batch | `migrations/20261029120000_add_raid_detection_ai.sql` (header style) + `migrations/20260412000000_add_missing_check_constraints.sql` (the constraint) | role-match |
| `alita/db/migrations/captcha_mode_test.go` (`TestCaptchaModeTurnstileConstraint`) | test (PG-only) | batch | `alita/db/migrations/raid_carry_over_test.go:47-77` | exact |
| `Makefile` `test-postgres-integrity` | config | batch | itself, line 41 `-run` regex | exact |
| `alita/config/config.go` | config | transform | `GeminiAPIKey` additions (`:196-207`, `:342-346`, `:480-490`) | exact |
| `alita/config/config_test.go` (`TestCaptchaWebConfig`) | test | transform | existing config tests (not read; planner to grep `ValidateConfig`) | role-match |
| `locales/{en,es,fr,hi,id,pt,ru}.yml` | config (i18n) | - | `lockdown_*` keys + `alita/i18n/lockdown_locale_test.go` | exact |
| `alita/i18n/captcha_web_locale_test.go` | test | transform | `alita/i18n/lockdown_locale_test.go:1-60` | exact |
| `docs/.../self-hosting/captcha-web.md` | docs | - | `docs/.../self-hosting/webhooks.md` (hand-written, no marker) | role-match |
| `docs/.../api-reference/environment.md`, `sample.env` | docs | - | `GEMINI_API_KEY` blocks (`environment.md:73-96`, `:592-593`; `sample.env:127-141`) | exact |
| `scripts/captcha-page/*.test.mjs` | test (node) | transform | none (`scripts/gemini-spike/` for dir layout only) | no analog |
| embedded `page/captcha.html` + inline JS | UI asset | request-response | none | no analog |
| `08-SPIKE.md` | docs | - | `.planning/phases/07-ai-assisted-raid-detection/07-SPIKE.md` | exact |
| `AGENTS.md` (same commit) | docs | - | its Phase 7 raid bullets | exact |

## Pattern Assignments

### `alita/utils/captchaweb/store.go` and `limiter.go` (service, Lua + Redis)

**Analog:** `alita/modules/raid_counters.go`

Do not copy `raidRedis()` as is: it is package-private to `modules` and `captchaweb` must not import `alita/utils/cache` through `alita/db`. Inject a `redis.Cmdable` (or `*redis.Client`) into `NewStore`. `alita/utils/cache` is not under `alita/db`, but keep the leaf free of `alita/config` and `alita/db` (D-29); take the client from `main.go`.

**Imports / constants** (`raid_counters.go:1-48`):
```go
import (
	"context"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"
	"github.com/divkix/Alita_Robot/alita/utils/cache"
)
const (
	// raidKeyPrefix starts every key of the family. It sits outside alita:cache:, so
	// CLEAR_CACHE_ON_STARTUP keeps it.
	raidKeyPrefix = "alita:raid:"
	// raidRedisTimeout bounds the one Redis round trip the join hot path makes.
	raidRedisTimeout = 250 * time.Millisecond
)
```
Copy: key family constants outside `alita:cache:` (new family `alita:captcha:ch:`, `:done:`, `:rl:`; `alita:captcha:refresh:cooldown:*` already exists), 250 ms bound per round trip.

**Lua script pattern** (`raid_counters.go:84-121`):
```go
var raidJoinScript = redis.NewScript(`
local now = tonumber(ARGV[1])
...
return redis.call('ZCOUNT', KEYS[1], '(' .. (now - tonumber(ARGV[2])), '+inf')
`)
ctx, cancel := context.WithTimeout(cache.Context, raidRedisTimeout)
defer cancel()
return raidJoinScript.Run(ctx, client, []string{raidKey(chatID, raidPartJoined)}, args...).Int()
```
Copy: doc comment above each script naming `KEYS[1]` and `ARGV` order; `redis.NewScript`; `.Run(ctx, client, keys, args...)`. The three scripts (consume-if-user, rearm, window) are given verbatim in RESEARCH Pattern 10 lines 397-418; use them. Store ids as decimal strings in hash fields (Lua numbers lose exactness above 14 digits). The `{chat}` hash tag does not apply (single-key scripts); the test for "wrong user returns `{2}` and leaves the key" is the one to write first.

**Tests:** miniredis, as `alita/modules/lockdown_fake_test.go:16` (`github.com/alicebob/miniredis/v2`) and `alita/utils/cache/startup_test.go:13` (`miniredis.RunT(t)`). In `captchaweb` tests build `redis.NewClient(&redis.Options{Addr: mr.Addr()})` directly (no `cache.SetRedisClientForTest` needed since the store takes a client).

---

### `alita/utils/captchaweb/siteverify.go` (client, request-response)

**Analog:** `alita/utils/gemini/client.go` (stdlib-only leaf)

**Package doc and leaf rule** (`client.go:1-8`):
```go
// Package gemini is the Gemini generateContent client of the AI raid checks.
//
// It imports the standard library only, so the scripts/gemini-spike tool can
// use it without pulling in alita/config or alita/db, whose init functions
// connect to PostgreSQL before main. ... nothing in this package logs:
// callers log, and every error returned here is free of the key.
```
Copy the intent: `captchaweb` imports stdlib plus `github.com/redis/go-redis/v9` only (D-29 says no `alita/db`; the Gemini leaf test forbids all dotted paths, so for captchaweb the leaf test must allow go-redis but forbid `Alita_Robot/alita/db`, `alita/config`, `alita/modules`).

**Constants and sentinel errors** (`client.go:22-57`): a `const` block with `DefaultEndpoint`, `Timeout = 8 * time.Second`, `maxResponseBytes = 1 << 20`; sentinel `errors.New("gemini: ...")`. Use `DefaultEndpoint = "https://challenges.cloudflare.com/turnstile/v0/siteverify"`, `Timeout = 5 * time.Second`, `maxResponseBytes = 64 << 10`.

**Swappable endpoint + no redirect** (`client.go:76-118`):
```go
type Client struct {
	Endpoint string // Empty means DefaultEndpoint; tests point it at an httptest server.
	APIKey   string
	HTTP     *http.Client // Nil uses a client with Timeout that follows no redirects.
}
var defaultHTTP = &http.Client{
	Timeout: Timeout,
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}
```
Copy as `Client{Endpoint, Secret, HTTP}`. Secret goes in the JSON body (Cloudflare contract), never a URL. Return a three-way `Outcome{Pass, Failed, Retry}` per RESEARCH Pattern 5 table; never wrap or `%v` a `*url.Error` (AGENTS.md Go rules: "never wrap or `%v` a request"; rebuild from the error class).

**Leaf test** (`leaf_test.go:14-56`): copy `TestGeminiPackageIsLeaf` as `TestCaptchaWebPackageIsLeaf` (parse imports of non-test files with `parser.ImportsOnly`; adapt the allow rule as above).

---

### `alita/utils/captchaweb/initdata.go`, `clientip.go`, `settings.go`, `stats.go` (utility, transform)

**Analog for validation helper style:** `alita/utils/gemini/client.go:59-66`
```go
var modelPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)
func ValidModel(name string) bool { return modelPattern.MatchString(name) }
```
Copy: pre-compiled `^[A-Za-z0-9_-]{1,64}$` for the challenge token and short name. HMAC code is given in RESEARCH Pattern 2 (`deriveKey`, `sign`, `hmac.Equal`); no repo analog.

**Analog for counters:** `alita/modules/raid_ai.go:375-424` (`atomic.Int64` vars, `GetRaidAIStats()` snapshot struct)
```go
raidAIChecked atomic.Int64
...
func GetRaidAIStats() RaidAIStats {
	return RaidAIStats{Checked: raidAIChecked.Load(), ...}
}
```
Copy as `stats.go` `Snapshot()` (issued, passed, failed, expired, invalid, retry, limited, errors). Counters are per process; numbers only.

---

### `alita/utils/captchaweb/handler.go` and `page/captcha.html` (controller, request-response)

**Analog:** `alita/utils/httpserver/server.go`

**Body cap, method check, generic errors** (`server.go:238-262`, `webhookHandler`):
```go
if r.Method != http.MethodPost {
	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	return
}
body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBodySize))
if err != nil {
	http.Error(w, "Bad request", http.StatusBadRequest)
	return
}
...
w.Header().Set("Retry-After", "1")
http.Error(w, "Update queue unavailable", http.StatusServiceUnavailable)
```
Differences for verify: use Go 1.22 method patterns (`POST /captcha/verify`) so `OPTIONS`/`GET` give 405 for free (RESEARCH Pattern 6); cap 16 KiB instead of 10 MiB (`maxRequestBodySize` is 10 MiB, `server.go:34`); JSON answers `{"result":...}` instead of `http.Error` text; add `Cache-Control: no-store`, `X-Content-Type-Options: nosniff`, no CORS.

**Do not copy** the tracing span attributes (`attribute.String("callback_query.id", ...)`): Pitfall 8, no token/initData in spans or logs.

**JSON health handler** (`server.go:110-139`, `RegisterHealth`): `Content-Type: application/json`, `json.NewEncoder(w).Encode(...)`, log encode failure with `log.Errorf("[HTTPServer] Failed to encode ...")`. Copy for `GET /captcha/health` (`{"status":"ok"|"unavailable"}`, 503 when Redis `PING` over 250 ms fails).

**Panic recovery:** every goroutine starts with `defer error_handling.RecoverFromPanic("<fn>", "<mod>")` (`server.go:222`, `alita/utils/error_handling/error_handling.go:16`). `captchaweb` cannot import `alita/utils/error_handling` if it imports db; check, otherwise recover locally. The detached finish task lives in `modules` where it can.

**Write timeout trap:** `Start()` sets `WriteTimeout: 10 * time.Second` (`server.go:381-384`); verify handler starts with `http.NewResponseController(w).SetWriteDeadline(time.Now().Add(20*time.Second))` and ignores the error (ResponseRecorder returns one).

**Template:** no repo analog for `html/template` + nonce. Follow RESEARCH Pattern 7 (`//go:embed page/captcha.html`, parse once, `template.JS(json.Marshal(...))` for the 7-language block).

---

### `alita/utils/httpserver/server.go` (add generic `Handle`)

**Analog:** same file, `RegisterDBMetrics` (`server.go:177-195`) and `Start` endpoint list (`:386-393`).
```go
s.mux.Handle("/db_metrics", s.requireMetricsAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { ... })))
log.Info("[HTTPServer] Registered /db_metrics endpoint")
...
endpoints := []string{"/health", "/metrics"}
if s.pprofEnabled { endpoints = append(endpoints, "/debug/pprof/*") }
```
Add `func (s *Server) Handle(pattern string, h http.Handler)` that calls `s.mux.Handle` and logs `[HTTPServer] Registered %s`; add a `captchaEnabled` flag so `Start` appends `/captcha*` to `endpoints`. Note `s.bot` is set only by `RegisterWebhook` (`:206`), so the captcha handler gets the bot by injection. Warn pattern for the shared port (D-31): copy `RegisterMetrics` (`:169-175`) `if s.metricsAuthToken == "" { log.Warn("[HTTPServer] METRICS_AUTH_TOKEN is not set ...") }`.

Test analog: `alita/utils/httpserver/server_test.go:28-60` (`New(0, time.Now())`, `httptest`) for `TestCaptchaWebRoutesRegistration`.

---

### Fake-Cloudflare helper and `captchaweb` tests (test)

**Analog:** `alita/utils/gemini/gemini_test.go:64-100`
```go
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
	return &Client{Endpoint: server.URL, APIKey: testKey, Model: model, HTTP: server.Client()}
}
func writeJSON(w http.ResponseWriter, status int, body string) { ... }
```
Copy the recorder, `writeJSON`, and an `Endpoint`-injected client. Fake switches per RESEARCH Wave 0: outcome, echo-the-secret-in-body, slow, 500, non-JSON, redirect. Also copy the `TestGeminiErrorsNeverContainKey` idea (`gemini_test.go:680`) for `TestCaptchaWebNeverLogsInputs`. Build tag: files start `//go:build testtools` (see `leaf_test.go:1`); run with `-tags testtools`.

---

### `alita/modules/captcha_web.go` (service, Backend impl) and `captcha.go` edits

**Analog:** `alita/modules/raid_counters.go:63-77` for Redis acquisition with a one-time "no Redis" notice:
```go
var raidNoRedisLogged atomic.Bool
func raidRedis() *redis.Client {
	if cache.IsRedisAvailable() {
		if client := cache.GetRedisClient(); client != nil { return client }
	}
	if raidNoRedisLogged.CompareAndSwap(false, true) {
		log.Info("[RaidDetection] Redis is not available, ...")
	}
	return nil
}
```
Copy for `captchaWebRedis()`. Remember `cache.IsRedisAvailable()` is only `redisClient != nil` (Pitfall in RESEARCH): availability must also use the memoised 250 ms `PING` (5 s memo, `atomic`).

**Shared pass extraction** (`captcha.go:1499-1577`; read-before-claim ordering matters because the claim deletes stored messages):
```go
storedMessages, storedErr := captcha.GetStoredMessagesForAttempt(attempt.ID)
claimed, claimErr := captcha.CompleteCaptchaAttemptAtomic(attempt.ID, targetUserID, chat.Id, selectedAnswer, attempt.MessageID, expectedRefreshCount)
if claimErr != nil || !claimed { ... captcha_expired_or_not_found ... }
if err = restoreCaptchaPermissions(bot, chat.Id, targetUserID); err != nil { ...DeleteMessageWithErrorHandling; captcha_failed_verify... }
// summary: captcha_stored_messages_summary {"user": formatting.MentionHtml(...), "count", "types"}; time.AfterFunc(30s) delete
_ = helpers.DeleteMessageWithErrorHandling(bot, chat.Id, attempt.MessageID)
// greetings_captcha_verified_success; time.AfterFunc(5s) delete
SendWelcomeMessage(bot, ctx, targetUserID, user.FirstName)
```
Split into `claimCaptchaPass` (through unmute) and `finishCaptchaPass` (summary, delete challenge, success note, welcome) exactly as RESEARCH Pattern 3. Callback-only `query.Answer(...)` calls stay in the callback. Regression net: `TestCaptchaVerifyCallbackCorrectAnswerUnmutesAndCleansAttempt` (`captcha_command_test.go:1180`) and siblings must stay green unchanged.

**Tracked detached task** (`captcha.go:185-200`, `startCaptchaLifecycleTask`):
```go
func startCaptchaLifecycleTask(task func(context.Context)) bool {
	captchaLifecycleMu.Lock()
	if captchaLifecycleCtx == nil || captchaLifecycleDone { captchaLifecycleMu.Unlock(); return false }
	ctx := captchaLifecycleCtx
	captchaLifecycleWG.Add(1)
	...
```
Use it for `finishCaptchaPass`, with `defer error_handling.RecoverFromPanic("captchaWebFinish", "captcha")` first; fall back to inline when it returns false. `StopCaptchaLifecycle` (`:174-183`) already waits for it, so no new drain (RESEARCH Pattern 11).

**`SendCaptcha` mode branch** (`captcha.go:1032-1062`): current `if settings.CaptchaMode == "math" { ... } else { text }`. Introduce `mode := settings.CaptchaMode`; when `turnstile` and `!captchaWebAvailable()`, `mode = "math"`; add a `turnstile` arm that sets `answer = "ts1:" + hex(16 random bytes)` and skips image generation; the rest of `SendCaptcha` (GetChatMember validation, attempt row, mute, `UpdateCaptchaAttemptMessageID`, `scheduleCaptchaTimeout`) stays. Rollback on Issue/send failure via existing `rollbackCaptchaAttempt`.

**Forged-callback guard** (D-25): insert after the attempt load in `captchaVerifyCallback` (after `attempt.RefreshCount != expectedRefreshCount`, `captcha.go:1485-1490`) and in `captchaRefreshCallback` (`1703-1721`), copying the existing refusal shape:
```go
if attempt.ID != uint(attemptID64) {
	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
	text, _ := tr.GetString("captcha_attempt_not_valid")
	_, err = query.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: text})
	return err
}
```
with `strings.HasPrefix(attempt.Answer, "ts1:")`.

**`/captchamode` command** (`captcha.go:585-633`): extend `mode != "math" && mode != "text"` to include `turnstile`, add the three ordered refusals before `captcha.SetCaptchaMode`, add `captcha_mode_turnstile_desc` branch beside `if mode == "text"` at `:624-627`. Keep `captchaAdminGate(bot, ctx, false)` (`:588`): the per-group admin check is never skipped. Rights refusal pattern: `lockdown_start.go:94-97` and `chat_status.FetchBotMember` (`owner.go:71`).

**Stats section** (`devs.go:362-379`, appended at `:407`):
```go
func raidAIStatsSection() string {
	raid := GetRaidAIStats()
	return "\n<b>AI checks (this replica, since its last restart):</b>" +
		fmt.Sprintf("\n    <b>Raid path:</b> checked %d, ...", ...)
}
...
stats := devs.LoadAllStats() + raidAIStatsSection()
```
Add `captchaWebStatsSection()` with the same heading shape and numbers only; append `+ captchaWebStatsSection()` at `devs.go:407`.

**Module tests:** `alita/modules/test_harness_test.go:49-59` (`newModuleTestBot` with `Username: "AlitaTestBot"`, `Token: "999:test"`) and `lockdown_fake_test.go:268-300` (`lockdownEnv` with miniredis) for a second "replica" test. Redis: `cache.SetRedisClientForTest(t, client)` (`alita/utils/cache/test_setup_testtools.go:144`).

---

### `alita/db/models/captcha.go` and `alita/db/captcha/repository.go` (CRUD)

**Model tag** (`models/captcha.go:9`): change `check:chk_captcha_mode,captcha_mode IN ('math','text')` to include `'turnstile'`. GORM builds the CHECK from this tag in SQLite tests, so no `AutoMigrate` list changes (model already in every list).

**Repository** (`repository.go:73-91`): change `if mode != "math" && mode != "text" { return ErrInvalidCaptchaMode }` to accept `turnstile`. Keep `cache.DeleteCache(cache.CacheKey("captcha_settings", chatID))` after the write (already present).

**Test:** extend `alita/db/constraints_test.go:135` `validModes := []string{"math", "text"}` with `"turnstile"`; add `TestSetCaptchaModeAcceptsTurnstile` in `alita/db/captcha` (add its name to nothing; SQLite).

---

### `migrations/20261030120000_allow_turnstile_captcha_mode.sql` (migration)

**Header-comment analog:** `migrations/20261029120000_add_raid_detection_ai.sql` (prose header, then closing sentence "Every statement is idempotent; the Go runner applies each file once inside a transaction and records its sha256 in schema_migrations.").

**Constraint analog:** `migrations/20260412000000_add_missing_check_constraints.sql:13` drops with `ALTER TABLE captcha_settings DROP CONSTRAINT IF EXISTS chk_captcha_mode;` then adds `ADD CONSTRAINT chk_captcha_mode CHECK (captcha_mode IN ('math', 'text')) NOT VALID;` (`:24-26`).

Statements (no `BEGIN`/`COMMIT`; `DROP IF EXISTS` first because `alita/db/migrations/runner.go:530-575` wraps `ALTER TABLE ... ADD CONSTRAINT ... ;` in a `DO` block that swallows `duplicate_object`, and `scripts/migrate_psql.sh:75` does the same; keep the ADD multi-line so the regex matches as for the old file):
```sql
ALTER TABLE captcha_settings DROP CONSTRAINT IF EXISTS chk_captcha_mode;
ALTER TABLE captcha_settings
ADD CONSTRAINT chk_captcha_mode
CHECK (captcha_mode IN ('math', 'text', 'turnstile')) NOT VALID;
```
No change to `runner.go` or `migrate_psql.sh` is needed. Name must sort after `20261029120000`.

---

### `alita/db/migrations/captcha_mode_test.go` (PG-only test) and `Makefile`

**Analog:** `alita/db/migrations/raid_carry_over_test.go:47-77`: skips unless `ALITA_TEST_DATABASE=true` and `DATABASE_URL` set, opens `gorm.Open(postgres.Open(dsn), ...)`, checks tables exist ("the migration chain must run before this test"), executes the migration file via `cleanSupabaseSQL` + `splitSQLStatements`. Copy for `TestCaptchaModeTurnstileConstraint` (turnstile accepted, `bogus` rejected). Never run PostgreSQL locally.

**Makefile** line 41: append `|TestCaptchaModeTurnstileConstraint` inside the `(...)$$` alternation of the `-run` regex (anchored with `$$`; `./alita/db/migrations` is already in the package list at line 44).

---

### `alita/config/config.go` (config)

**Analog:** the Gemini additions.

Struct fields with doc comment (`config.go:196-207`):
```go
// Image checks of the AI raid trigger (Google Gemini). ... The key is
// a secret and is registered with logredact.RegisterSecret. ...
GeminiAPIKey string
GeminiModel  string
```
Env loading (`:342-346`): `GeminiAPIKey: os.Getenv("GEMINI_API_KEY"),`. Add `CaptchaWebBaseURL`, `TelegramMiniAppShortName`, `TurnstileSiteKey`, `TurnstileSecretKey`, `CaptchaWebTrustedProxies`.

Secrets (`:480-490`):
```go
func registerSecrets(cfg *Config) {
	logredact.RegisterSecret(cfg.BotToken, ..., cfg.TypeSafeAPIKey, cfg.GeminiAPIKey)
}
```
Append `cfg.TurnstileSecretKey` (≥6 chars is enforced inside `RegisterSecret`, `logredact.go:85-97`). The site key is public, not registered.

Validation (`ValidateConfig`, `:212`): style is `return fmt.Errorf("X is required")`. For the "all four or none" rule, name missing variables only, never values. Use stdlib `net/url`; `alita/config` must not import anything reaching `alita/db`. The `setDefaults` note at `:467-474` is the model for "never log the rejected value; `setDefaults` runs before `registerSecrets`".

Tests: new `TestCaptchaWebConfig` (none, partial, bad scheme, query/fragment, secret scrubbed through `logredact`). `alita/utils/logredact/reset_testtools.go` offers the test reset.

---

### `locales/*.yml` and `alita/i18n/captcha_web_locale_test.go`

**Analog:** `alita/i18n/lockdown_locale_test.go` (`TestLockdownLocaleKeys`, lines 38-60+): gathers keys by prefix (`lockdownKeyPrefix = "lockdown_"`) plus an extras list, then per locale (`staffLocales`, `loadStaffLocale(t, lang)`) checks parity both directions, non-empty, identical placeholders. Copy as `captchaWebKeys` (prefix `captcha_web_`, extras `captcha_mode_turnstile_desc`), and add the UI-SPEC max lengths and the "no placeholders/format verbs in page keys" rule. Reason: `make check-translations` only sees literal `GetString("key")` (Pitfall 16); the 26 page keys are read dynamically. Challenge-message and refusal keys are read with literal `GetString`, so the existing check covers them.

Edited keys in all 7 files: `captcha_mode_specify`, `captcha_mode_invalid`, `captcha_help_msg`, `captcha_invalid_mode_error` (en.yml lines ~976, 977, 289, 1561). `captcha_mode_set_formatted` (`en.yml:1734`) takes `(%s mode, %s description)`.

---

### Docs: `self-hosting/captcha-web.md`, `environment.md`, `sample.env`

**New page analog:** `docs/src/content/docs/self-hosting/webhooks.md` (front matter `title`/`description`, no MANUALLY MAINTAINED marker, hand-written pages persist under `make check-docs`). Directory metadata is `self-hosting/meta.ts` (`defineMeta`, no per-page list to update). Cover D-15's nine items plus the D-31 proxy path allow-list (`/captcha`, `/captcha/*` only).

**`environment.md` analog:** carries `<!-- MANUALLY MAINTAINED: do not regenerate -->` (line 6), edited by hand. Per-variable `### \`GEMINI_API_KEY\`` section (`:73`) with the Type/Required property table; plus the flat env listing near `:592-593` (`GEMINI_API_KEY=# ...`). Add the four settings (and the optional trusted-proxies variable) in both places.

**`sample.env` analog:** `sample.env:127-141` (commented `#GEMINI_API_KEY=` with explanatory comment block).

`docs/src/content/docs/commands/captcha/index.md` is generated from `captcha_help_msg`: run `make generate-docs`, then `make check-docs`.

---

### `.planning/phases/08-turnstile-web-captcha/08-SPIKE.md`

**Analog:** `.planning/phases/07-ai-assisted-raid-detection/07-SPIKE.md`. Section skeleton to copy: `# Phase N: ... spike guide` / `## Why this exists` (says the owner deferred it, D-18) / `## Before you start` / `## What to run, in order` / `## What to read in the output` / `## Decisions` / `## Which constants to change afterwards` / `## Checks only a live run can do` / `## What the tool never does`. Phase 8 differs: a checklist instead of a tool run. Each step states what a result means and what to change (CSP `frame-ancestors` hosts, `Referrer-Policy`, `start_param` source, `&mode=compact`, WebView widget rendering).

---

### `AGENTS.md` (same commit)

**Analog:** the Phase 7 raid bullets under "Data" and "Subsystem traps". Add: the `alita:captcha:ch:*`/`done`/`rl` key families outside `alita:cache:`; the `ts1:` reserved answer and the callback guard; the readiness gate (`MarkCaptchaWebReady`); the shutdown rule (post-pass tasks join `captchaLifecycleWG`, no new drain); the shared-port note (D-31); `captchaweb` leaf rule. Also update the "Callbacks" and test-harness bullets if their facts change.

---

## Shared Patterns

### Secrets and logging
**Source:** `alita/config/config.go:480-490`, `alita/utils/logredact/logredact.go:85`
**Apply to:** `config.go`, `siteverify.go`, `handler.go`
Register the Turnstile secret via `registerSecrets`. Never log or wrap request bodies, tokens, `initData`, Cloudflare responses or `*url.Error` (Pitfall 8; AGENTS.md "A file the bot downloads itself never puts its URL in an error"). Log categories use `log.Errorf("[Category][Function]: %v", err)` (CLAUDE.md).

### Fire-and-forget goroutines
**Source:** `alita/utils/error_handling/error_handling.go:16`, used at `server.go:222`, `captcha.go` `time.AfterFunc` blocks (`defer error_handling.RecoverFromPanic("captchaSummaryMsgDelete", "captcha")`)
**Apply to:** every new goroutine and `time.AfterFunc` in `captcha_web.go` and `captcha.go`.

### Redis operational keys
**Source:** `alita/modules/raid_counters.go:20-47`
**Apply to:** `captchaweb` store and limiters. Outside `alita:cache:`, 250 ms bounds, best effort, exactly-once from conditional DB writes (`CompleteCaptchaAttemptAtomic` / `ReleaseCaptchaAttemptAtomic`, `repository.go:380-453`), Redis only holds short-lived tokens.

### Cache invalidation after attempt insert
**Source:** `alita/db/captcha/repository.go:204-226` (`createCaptchaAttemptPreMessage` invalidates `alita:cache:captcha_pending:<chat>` after commit)
**Apply to:** `SendCaptcha` Turnstile path. No new insert path is added, so reuse the existing call; do not add a second insert.

### Failure and timeout reuse
**Source:** `handleCaptchaTimeout` (`captcha.go:1259`), `expireCaptchaAttempt` (`:1231`), `restoreCaptchaPermissions` (`:1404`)
**Apply to:** the Backend `Fail` and `Expire` methods. Call, do not copy. Failed solve = `handleCaptchaTimeout`; pass after expiry = `expireCaptchaAttempt` at once.

### Tests
**Apply to:** all Go tests: `//go:build testtools`, real fixtures only (`internal/testdb.Run`/module harness SQLite, miniredis, `httptest`, hand-written `gotgbot.BotClient` fakes such as `moduleBotClient` in `test_harness_test.go`), assert observable behaviour; run with `-tags testtools`. No mock libraries, no `t.Skip` (the skip gate in `scripts/check_test_results` fails `make test`).

## No Analog Found

| File | Role | Data Flow | Reason |
|---|---|---|---|
| `alita/utils/captchaweb/page/captcha.html` and inline JS | UI asset | request-response | No frontend exists in the repo (docs site is Astro/Blume under `docs/`, unrelated). Use `08-UI-SPEC.md` and RESEARCH Pattern 7. Embed with `embed.FS` and `html/template`; nonce on every `<script>`/`<style>`; no inline `style=` attributes (Pitfall 18). |
| `scripts/captcha-page/*.test.mjs` | test (node `--test` + `vm`) | transform | No JS test tooling in the repo; node is not a project dependency and not part of `make test` (Pitfall 23). For directory convention only, see `scripts/gemini-spike/` (`main.go`, `main_test.go`, one folder per tool). Record the executor-run result in the summary and list unautomated checks in `08-SPIKE.md`. |
| `initData` HMAC validator internals | utility | transform | No HMAC/signature validation exists in the repo. Use RESEARCH Pattern 2 (key `WebAppData`, `signature` kept in the check string, `hmac.Equal`, injectable clock for the published known-answer vectors). |
| Trusted-proxy `ClientIP` | utility | transform | `net/netip` rightmost-untrusted walk is new; spec in RESEARCH Pattern 6 ("Client address and the proxy"). |

## Metadata

**Analog search scope:** `alita/modules/`, `alita/utils/{gemini,httpserver,ratelimit,cache,logredact}`, `alita/db/{captcha,models,migrations}`, `alita/config`, `alita/i18n`, `migrations/`, `main.go`, `Makefile`, `docs/src/content/docs/{self-hosting,api-reference}`, `sample.env`, `scripts/`, `.planning/phases/07-*`
**Files scanned/read:** about 30
**Tracked-source gate:** `git ls-files` confirmed tracked for the 13 key analog files checked; no capability mirror paths used
**Pattern extraction date:** 2026-10-29
