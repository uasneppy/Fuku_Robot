---
phase: "8"
slug: "turnstile-web-captcha"
# status lifecycle: draft (seeded by plan-phase) → validated (set by validate-phase §6)
# audit-milestone §5.5 distinguishes NOT-VALIDATED (draft) from PARTIAL (validated + nyquist_compliant: true) (#2117)
status: validated
nyquist_compliant: true
wave_0_complete: true
created: "2026-10-29"
---

# Phase 8 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go `testing` with build tag `testtools`, testify assertions only; real fixtures: `internal/testdb.Run` and the module harness (SQLite), miniredis, `httptest` servers standing in for Cloudflare `siteverify`, hand-written `gotgbot.BotClient` fakes (`moduleBotClient`). No mock libraries. |
| **Config file** | none; `-tags testtools` is mandatory (AGENTS.md). Harness: `alita/modules/test_harness_test.go` |
| **Quick run command** | `go test -tags testtools -race -count=1 ./alita/utils/captchaweb ./alita/config ./alita/db/captcha` and `go test -tags testtools -race -count=1 -run '^(TestCaptchaWeb\|TestSendCaptchaTurnstile\|TestCaptchaMode\|TestCaptchaVerifyCallback\|TestHandleCaptchaTimeout)' ./alita/modules` |
| **Full suite command** | `make test` (then `make check-translations`, `make check-docs`, `CGO_ENABLED=0 go build ./...`) |
| **Estimated runtime** | ~60 seconds quick, ~6 minutes full |

---

## Sampling Rate

- **After every task commit:** Run the quick run command above for the packages touched
- **After every plan wave:** Run `make test` plus `go vet -tags testtools ./...` and `gofmt -l` (golangci-lint cannot lint Go 1.26 in this sandbox)
- **Before `/gsd-verify-work`:** `make test`, `make check-translations`, `make check-docs` and `CGO_ENABLED=0 go build ./...` green; the `node --test` result of the page script recorded in a summary
- **Max feedback latency:** 90 seconds
- PostgreSQL-only tests (`TestCaptchaModeTurnstileConstraint`) are run by CI through `make test-postgres-integrity`, never locally

---

## Per-Task Verification Map

Real task IDs from the 11 plans. `GT` stands for `CGO_ENABLED=1 go test -tags testtools -race -count=1 -v`. Every `-run` pattern names tests that exist once the task is done; each task's `<fails_when>` treats "no tests to run" as a failure. The Nyquist audit fills the status.

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 08-01-01 | 01 | 1 | CAPT-02, CAPT-03, CAPT-04, CAPT-05 | T-08-01, T-08-02, T-08-06 | A join in a Turnstile group mutes the member, writes a `ts1:` attempt, stores `alita:captcha:ch:<token>` and posts one message with one Mini App URL button; a signed verify confirmed by the fake Cloudflare unmutes through the shared pass path; another member gets not_for_you and changes nothing; an unconfirmed token, stale initData or a malformed body never unmutes | integration (tracer) | `GT -run '^TestCaptchaWebTracerJoinVerifyUnmute$' ./alita/modules` | ✅ | ✅ green |
| 08-01-02 | 01 | 1 | CAPT-03, CAPT-05 | T-08-03 | Forged `captcha_verify` / `captcha_refresh` callbacks cannot pass, count against or refresh a Turnstile attempt; the button pass and the web pass make the same Telegram calls | integration | `GT -run '^(TestCaptchaWebAttemptRefusesCallbacks\|TestCaptchaPassSharedByCallbackAndWeb)$' ./alita/modules` and the existing `TestCaptchaVerifyCallback*` tests | ✅ | ✅ green |
| 08-01-03 | 01 | 1 | CAPT-02, CAPT-07 | T-08-04, T-08-05 | Math fallback when the web captcha is unavailable (no runtime, no Redis PING, no bot username); issue or send failure rolls back and unmutes; an old token after a re-join never touches the new attempt; restart recovery resumes a Turnstile attempt | integration | `GT -run '^(TestSendCaptchaTurnstileFallsBackToMath\|TestSendCaptchaTurnstileRollsBackOnFailure\|TestCaptchaWebRejoinInvalidatesOldChallenge\|TestRunOrphanedCaptchaRecoveryResumesTurnstileAttempt)$' ./alita/modules` | ✅ | ✅ green |
| 08-02-01 | 02 | 1 | PLAT-04 (PLAT-02 half) | T-08-07, T-08-08, T-08-09, T-08-10 | Four settings all-or-nothing, errors name variables never values, https base URL, short secret refused, the secret registered and scrubbed | unit | `GT -run '^TestCaptchaWebConfig$' ./alita/config` | ✅ | ✅ green |
| 08-02-02 | 02 | 1 | PLAT-04 | — | sample.env and environment.md document the five variables with no value | docs | the grep loop in 08-02 Task 2 `<verify>` | ✅ | ✅ green |
| 08-03-01 | 03 | 2 | CAPT-03, CAPT-04 | T-08-11, T-08-12, T-08-13, T-08-14 | The published init-data-golang vector passes (signature kept); tampered, added, removed, repeated, wrong-token, stale, future, oversized, bad-user and bot-user initData are refused with one error | unit | `GT -run '^(TestInitDataKnownAnswer\|TestInitDataRejects)$' ./alita/utils/captchaweb` | ✅ | ✅ green |
| 08-03-02 | 03 | 2 | CAPT-04 (D-29) | — | captchaweb imports only the standard library, go-redis and logrus | unit | `GT -run '^TestCaptchaWebPackageIsLeaf$' ./alita/utils/captchaweb` | ✅ | ✅ green |
| 08-04-01 | 04 | 2 | CAPT-01 | T-08-15 | The migration drops `chk_captcha_mode` and the legacy inline check, re-adds it with `turnstile` NOT VALID, in the order the appliers need; PostgreSQL accepts turnstile, refuses bogus, keeps one captcha_mode check | unit (local) + PG only (CI) | `CGO_ENABLED=1 go test -tags testtools -count=1 -run '^(TestCaptchaModeMigrationStatements\|TestCaptchaModeTurnstileConstraint)$' -v ./alita/db/migrations` (the PostgreSQL test skips locally and runs in `make test-postgres-integrity`) | ✅ | ✅ green (statements); PG half CI-only |
| 08-04-02 | 04 | 2 | CAPT-01 | T-08-16, T-08-17 | SetCaptchaMode stores turnstile and invalidates the cache; the SQLite constraint accepts it | repo | `GT -run '^TestSetCaptchaModeAcceptsTurnstile$' ./alita/db/captcha` and `GT -run '^TestCaptchaSettingsConstraint_ValidModes$' ./alita/db` | ✅ | ✅ green |
| 08-05-01 | 05 | 2 | CAPT-04, CAPT-08 | T-08-18, T-08-19 | One fresh nonce per response on every script and style, the exact CSP, no frame-blocking or CORS header, a string table that cannot break out, the UI-contract markup | route | `GT -run '^(TestPageHeadersAndNonce\|TestPageEmbedsSafeStrings\|TestPageStructure\|TestNewPageRefuses)$' ./alita/utils/captchaweb` | ✅ | ✅ green |
| 08-05-02 | 05 | 2 | CAPT-08 | T-08-22 | The page script maps languages (in to id, unknown to en), the challenge (start_param then tgWebAppStartParam), cData, every server answer and widget size, with one POST per token | `node --test` (executor-run) | `node --test scripts/captcha-page/*.test.mjs` | ✅ | ✅ green |
| 08-05-03 | 05 | 2 | CAPT-04, CAPT-08 | T-08-21 | The boot path renders once, posts once, closes after a pass, resets on Try again, never logs or shows a relayed value; the noscript fallback | `node --test` + route | `node --test scripts/captcha-page/*.test.mjs` and `GT -run '^TestPageNoScriptFallback$' ./alita/utils/captchaweb` | ✅ | ✅ green |
| 08-06-01 | 06 | 3 | CAPT-04, CAPT-06 | T-08-23, T-08-24, T-08-28 | Only invalid-input-response, timeout-or-duplicate and a cdata mismatch fail a member (the group's action once); every Cloudflare or setup fault is retry with the token re-armed; nothing else re-arms | unit, route, integration | `GT -run '^(TestSiteverifyOutcome\|TestSiteverifyRequestShape\|TestVerifyRouteRearmsOnlyOnRetry)$' ./alita/utils/captchaweb` and `GT -run '^(TestCaptchaWebFailedSolveAppliesAction\|TestCaptchaWebCloudflareErrorIsNotAFailure)$' ./alita/modules` | ✅ | ✅ green |
| 08-06-02 | 06 | 3 | CAPT-07 | T-08-25, T-08-27 | A late check expires at once; timeout and check decide once on any replica (`-race`); a failed unmute is retried durably and never reported as passed; no Restrict right is retry; the finish joins the lifecycle drain | integration | `GT -run '^(TestCaptchaWebPassAfterExpiryExpiresAtOnce\|TestCaptchaWebTimeoutAndVerifyOnDifferentReplicas\|TestCaptchaWebPassVersusTimeoutRace\|TestCaptchaWebUnmuteFailureSchedulesRetry\|TestCaptchaWebBotWithoutRightsIsRetry\|TestCaptchaWebFinishJoinsLifecycleDrain)$' ./alita/modules` | ✅ | ✅ green |
| 08-06-03 | 06 | 3 | CAPT-04 (PLAT-02) | T-08-26 | No log line on any verify path holds the secret, a token, initData, the name or a Cloudflare body, with the redactor off and an echoing Cloudflare double | integration | `GT -run '^TestCaptchaWebNeverLogsInputs$' ./alita/utils/captchaweb` | ✅ | ✅ green |
| 08-07-01 | 07 | 3 | CAPT-08 | T-08-29 | All captcha_web_ keys in all 7 locales; the 26 page keys plain and within their UI-SPEC lengths | unit | `CGO_ENABLED=1 go test -tags testtools -count=1 -run '^TestCaptchaWebLocaleKeys$' -v ./alita/i18n` and `make check-translations` | ✅ | ✅ green |
| 08-07-02 | 07 | 3 | CAPT-08 | T-08-30 | Seven per-language tables with English fallback; a key missing in English fails at startup | unit | `CGO_ENABLED=1 go test -tags testtools -count=1 -run '^TestStringTable$' -v ./alita/i18n` | ✅ | ✅ green |
| 08-08-01 | 08 | 4 | CAPT-03, CAPT-04 | T-08-31, T-08-32, T-08-33 | Per-IP, per-user and per-(token, user) windows in Redis; a griefer on another member's link spends only their own budget; spoofed X-Forwarded-For ignored | unit, route | `GT -run '^(TestWindowLimiter\|TestClientIP\|TestVerifyRouteWrongUserChangesNothing\|TestVerifyRouteLimitsAndHeaders)$' ./alita/utils/captchaweb` | ✅ | ✅ green |
| 08-08-02 | 08 | 4 | CAPT-04 | T-08-34, T-08-36, T-08-37 | JSON only (415, 413, 400, 405), 64 in flight, no-store and no CORS, readiness gate, health route, the answer outlives the server's WriteTimeout | route | `GT -run '^(TestVerifyRouteLimitsAndHeaders\|TestVerifyRouteReadinessGate\|TestHealthRoute\|TestVerifyRouteOutlivesServerWriteTimeout)$' ./alita/utils/captchaweb` | ✅ | ✅ green |
| 08-08-03 | 08 | 4 | CAPT-03 | T-08-35 | 50 concurrent consumes give one winner; a decision is replayed only to the same verified user | route, miniredis | `GT -run '^(TestChallengeStoreConsumeOnce\|TestVerifyRouteReplaysDecision)$' ./alita/utils/captchaweb` | ✅ | ✅ green |
| 08-09-01 | 09 | 5 | CAPT-01 | T-08-38, T-08-39 | `/captchamode turnstile` stored only when configured, Redis answers and the bot has Restrict; refusals in order with nothing written; non-admin refused first | integration | `GT -run '^(TestCaptchaModeTurnstile\|TestCaptchaModeTurnstileRefusals)$' ./alita/modules` | ✅ | ✅ green |
| 08-09-02 | 09 | 5 | CAPT-01 | T-08-40 | The four mode texts list math, text and turnstile in all 7 locales; the captcha commands page documents Turnstile | integration, docs | `GT -run '^TestCaptchaModeListsTurnstileInEveryLanguage$' ./alita/modules` and `make check-translations && make check-docs` | ✅ | ✅ green |
| 08-10-01 | 10 | 6 | CAPT-02, CAPT-04, CAPT-08 | T-08-41, T-08-42, T-08-43, T-08-44 | Routes registered only with the settings, through `Server.Handle`; verify 503 until `MarkCaptchaWebReady`; an end-to-end pass through the real mux; startup warnings for the shared port and the proxy list | route, integration | `GT -run '^TestCaptchaWebRoutesRegistration$' ./alita/utils/httpserver` and `GT -run '^(TestCaptchaWebWiring\|TestCaptchaWebStartupWarnings)$' ./alita/modules` | ✅ | ✅ green |
| 08-10-02 | 10 | 6 | CAPT-08 (D-17) | T-08-45 | Eight per-replica counters, each counted once, shown in the developer /stats with numbers only | integration | `GT -run '^TestCaptchaWebStatsSection$' ./alita/modules` | ✅ | ✅ green |
| 08-11-01 | 11 | 6 | PLAT-04 | T-08-46, T-08-47 | The self-hosting page covers the D-15 steps and forwards only /captcha and /captcha/*; the docs check is clean | docs | `make generate-docs && make check-docs` plus the grep check in 08-11 Task 1 `<verify>` | ✅ | ✅ green |
| 08-11-02 | 11 | 6 | PLAT-04 (D-18) | T-08-48 | The owner's real-device checklist covers every live-only behaviour with its pass signal and the change to make | docs | the grep check in 08-11 Task 2 `<verify>` | ✅ | ✅ green |
| RF-WR-01 | review | - | CAPT-07 | - | A lost claim of a live attempt, or a DB blip on a failed solve, is retry with the challenge re-armed, never expired | integration | `GT -run '^(TestCaptchaWebLostClaimOfLiveAttemptIsRetry\|TestCaptchaWebFailedSolveDatabaseBlipIsRetry)$' ./alita/modules` | ✅ | ✅ green |
| RF-WR-02 | review | - | CAPT-07 | - | A pass whose unmute failed is replayed as `passed` with no side effect | route, integration | `GT -run '^TestVerifyRoutePassWhoseUnmuteFailedIsRemembered$' ./alita/utils/captchaweb` and `TestCaptchaWebUnmuteFailureSchedulesRetry` (modules) | ✅ | ✅ green |
| RF-WR-03 | review | - | CAPT-03 | - | Claim-and-release consume: an error after the script ran releases the claim | route, miniredis | `GT -run '^(TestChallengeStoreConsumeIsUndoneByAnError\|TestChallengeStoreConsumeOnce)$' ./alita/utils/captchaweb` | ✅ | ✅ green |
| RF-WR-04 | review | - | CAPT-04 | - | Key-mismatch streak breaker holds the sixth failed solve as retry; fails open on Redis error | route, integration | `GT -run '^TestVerifyRouteKeyMismatchBreaker$' ./alita/utils/captchaweb` and `GT -run '^TestCaptchaWebKeyMismatchCannotMassKick$' ./alita/modules` | ✅ | ✅ green |
| RF-WR-05 | review | - | CAPT-06 | - | Shared settle decision: approved or disabled-captcha members are released, not kicked | integration | `GT -run '^TestCaptchaWebFailedSolveSparesApprovedAndDisabled$' ./alita/modules` | ✅ | ✅ green |
| RF-WR-06 | review | - | CAPT-04 | - | In-flight slot is taken after the body: empty body 400 and forged initData invalid while all 64 slots are held | route | `GT -run '^TestVerifyRouteLimitsAndHeaders$' ./alita/utils/captchaweb` | ✅ | ✅ green |
| RF-WR-07 | review | - | CAPT-06 | - | Applied vs not-applied action result: a refused ban answers neutral `invalid`, unmutes, no removal text | route, integration | `GT -run '^TestVerifyRouteActionNotApplied$' ./alita/utils/captchaweb` and `GT -run '^TestCaptchaWebFailedActionNotAppliedIsNeutral$' ./alita/modules` | ✅ | ✅ green |
| RF-IN-01 | review | - | CAPT-04 | - | Method-less patterns through a real `http.ServeMux`: wrong method gets 405 with Allow, no-store, nosniff, no CORS | route | `GT -run '^TestVerifyRouteLimitsAndHeaders$' ./alita/utils/captchaweb` and `GT -run '^TestCaptchaWebRoutesRegistration$' ./alita/utils/httpserver` | ✅ | ✅ green |
| RF-IN-02 | review | - | CAPT-04 | - | A private or loopback address from X-Forwarded-For is no bucket | unit | `GT -run '^TestClientIP$' ./alita/utils/captchaweb` | ✅ | ✅ green |
| RF-IN-03 | review | - | PLAT-04 | - | One table of trusted-proxy inputs holds in config and captchaweb alike | unit | `GT -run '^TestValidCaptchaWebProxiesTable$' ./alita/config` and `GT -run '^TestParseTrustedProxiesTable$' ./alita/utils/captchaweb` | ✅ | ✅ green |
| RF-IN-04 | review | - | CAPT-02 | - | The availability probe memo is one unit; no caller gets another runtime's answer under `-race` | integration | `GT -run '^TestCaptchaWebAvailabilityMemoIsOneUnit$' ./alita/modules` | ✅ | ✅ green |
| RF-IN-05 | review | - | CAPT-08 | - | Page `LANGS` equals `PageLanguages`; CI `test` job runs Node 22 and `node --test scripts/captcha-page/*.test.mjs` | unit + CI config | `GT -run '^TestPageLanguagesMatchScript$' ./alita/utils/captchaweb`; `.github/workflows/ci.yml` lines 338-344 | ✅ | ✅ green |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

Each test file is created RED-first by the task that needs it (no separate Wave 0 plan):

- [x] `alita/utils/captchaweb/fixtures_testtools.go` (plan 08-01 Task 1): signed initData, the test Turnstile token and a fake-Cloudflare `httptest` server with pass and scripted answers; plans 08-03 to 08-10 reuse it
- [x] `alita/modules/captcha_web_test.go` (plan 08-01): the `captchaWebEnv` harness (miniredis wired into `cache`, the runtime installed, the fake Cloudflare); `captcha_web_outcomes_test.go` adds a second replica with its own `moduleBotClient` (plan 08-06)
- [x] `alita/utils/captchaweb/verify_outcomes_test.go` (plan 08-06): `outcomeBackend` and `newOutcomeHandler`, reused by plan 08-08
- [x] `alita/i18n/captcha_web_locale_test.go` (plan 08-07): parity of every `captcha_web_` key, which `make check-translations` cannot see
- [x] `scripts/captcha-page/captcha-page.test.mjs` (plan 08-05): `node --test`, executor-run, not part of `make test`
- [x] `TestCaptchaModeTurnstileConstraint` added to the `-run` regex of `make test-postgres-integrity` (plan 08-04; `./alita/db/migrations` is already in its package list)
- [x] The published init-data-golang known-answer vector stored as test constants (plan 08-03; `#nosec G101`, a public sample token)
- [x] No harness `AutoMigrate` list change is needed (`CaptchaSettings` and `CaptchaAttempts` are already in every list)

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| The direct link posted by the bot in a group opens the Mini App in Android, iOS, Desktop and Web, delivers the tapper's `initData`, and a second member's tap is rejected | CAPT-02, CAPT-03 | Needs a real bot, a public hostname, BotFather and real Telegram clients | Follow the live checklist in `08-SPIKE.md` |
| The Turnstile widget renders and returns a token inside each WebView; real `siteverify` hostname for the owner's domain; CSP and `Referrer-Policy` behave in every client; whole-check latency stays inside the 12 s client timeout | CAPT-04, CAPT-08 | Needs the owner's Cloudflare keys and real clients | `08-SPIKE.md` |
| Whether `start_param` is signed in `initData`, the `startapp` length limit, the first-launch dialog, compact mode, theme and safe-area rendering, `crypto.subtle` availability | CAPT-02, CAPT-08 | Telegram client behaviour that official pages could not be read for | `08-SPIKE.md` |
| BotFather accepts the owner's short name and URL | PLAT-04 | Needs the owner's BotFather chat | Self-hosting guide steps |
| Native-speaker read of the new `captcha_web_*` strings in es, fr, hi, id, pt and ru | CAPT-08 | Translation quality | Review the new keys |
| The `frame-ancestors` host list and CSP against a live managed widget; a solve after signed data older than 5 minutes (shows the "invalid" copy, IN-06) | CAPT-04, CAPT-08 | Needs a live widget and Telegram Web K/A/Z | `08-SPIKE.md` checks 6 and 14 |
| Site key and secret come from one widget, so the key-mismatch breaker never trips | CAPT-04 | Needs the owner's Cloudflare keys | `08-SPIKE.md` check 5 |
| `TestCaptchaModeTurnstileConstraint` against PostgreSQL | CAPT-01 | Never run locally; CI-only | `make test-postgres-integrity` in CI |
| The self-hosting guide reads correctly against the nine steps of D-15 | PLAT-04 | Prose | Manual read-through |

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references
- [x] No watch-mode flags
- [x] Feedback latency < 90s
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** validated 2026-10-30

## Validation Audit 2026-10-30

| Metric | Count |
|--------|-------|
| Gaps found | 0 |
| Resolved | 0 (all named tests exist and pass; 12 review-fix rows added to the map, each already backed by a test) |
| Escalated | 0 |

Every map pattern ran with `-race -v` (captchaweb, config, db/captcha, db, i18n, httpserver, db/migrations, and the Captcha patterns of modules) with no pattern matching zero tests; `node --test scripts/captcha-page/*.test.mjs` passed 24/24; `make check-translations` and `make check-docs` are clean; the doc rows (sample.env, environment.md, self-hosting page, 08-SPIKE.md) exist. CI-only: `TestCaptchaModeTurnstileConstraint` skips locally (no PostgreSQL started) and is in the `make test-postgres-integrity` `-run` list. Manual-only items are those above (the live checks of 08-VERIFICATION.md `human_verification`).
