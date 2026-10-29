---
phase: "8"
slug: "turnstile-web-captcha"
# status lifecycle: draft (seeded by plan-phase) → validated (set by validate-phase §6)
# audit-milestone §5.5 distinguishes NOT-VALIDATED (draft) from PARTIAL (validated + nyquist_compliant: false) (#2117)
status: draft
nyquist_compliant: false
wave_0_complete: false
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

Seeded from the research's requirement-to-test map. The planner and executors replace each row with the real task IDs; the Nyquist audit fills the status.

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 08-XX-XX | XX | N | CAPT-01 | T-08 (access control) | `/captchamode turnstile` stored only when the web captcha is configured, Redis answers and the bot has Restrict; refusals come in order and leave the mode unchanged; non-admin refused | integration | `... -run '^(TestCaptchaModeTurnstile\|TestCaptchaModeTurnstileRefusals)$' ./alita/modules` | ❌ W0 | ⬜ pending |
| 08-XX-XX | XX | N | CAPT-01 | T-08 (input validation) | Repository and model accept `turnstile` and reject anything else; the widened CHECK holds on PostgreSQL | repo and PG only (CI) | `... -run '^TestSetCaptchaModeAcceptsTurnstile$' ./alita/db/captcha`, extended `TestCaptchaSettingsConstraint_ValidModes`, `TestCaptchaModeTurnstileConstraint` in `./alita/db/migrations` (added to `make test-postgres-integrity`) | ❌ W0 | ⬜ pending |
| 08-XX-XX | XX | N | CAPT-02 | T-08 (spoofing) | A join in a Turnstile group mutes the member, writes an attempt row with the reserved `ts1:` answer, invalidates the pending flag, stores the token in Redis and posts one message with exactly one URL button; falls back to math when unconfigured, Redis is down or the bot has no username; a send failure rolls back and unmutes | integration | `... -run '^(TestSendCaptchaTurnstileMutesAndPostsMiniAppButton\|TestSendCaptchaTurnstileFallsBackToMath\|TestSendCaptchaTurnstileRollsBackOnFailure)$' ./alita/modules` | ❌ W0 | ⬜ pending |
| 08-XX-XX | XX | N | CAPT-03 | T-08 (spoofing, elevation) | A wrong user's verify changes nothing and does not consume; a reused or parallel token proceeds once (50 concurrent verifies); forged `captcha_verify` / `captcha_refresh` callbacks cannot pass or replace a Turnstile attempt; an old token never touches a re-joiner's new attempt | route, miniredis, integration | `... -run '^(TestVerifyRouteWrongUserChangesNothing\|TestChallengeStoreConsumeOnce)$' ./alita/utils/captchaweb` and `... -run '^(TestCaptchaWebAttemptRefusesCallbacks\|TestCaptchaWebRejoinInvalidatesOldChallenge)$' ./alita/modules` | ❌ W0 | ⬜ pending |
| 08-XX-XX | XX | N | CAPT-04 | T-08 (tampering, spoofing) | `initData` known-answer vectors, tampered fields, missing or duplicate hash, stale, future, bad user JSON and the bot user are all rejected; the page can never report a pass; `siteverify` outcomes and request shape; a retryable outcome re-arms only the same user's token; size, method, rate, in-flight and readiness limits; client IP resolution | unit, route, miniredis | `... -run '^(TestInitDataKnownAnswer\|TestInitDataRejects\|TestVerifyRouteNeverTrustsThePage\|TestSiteverifyOutcome\|TestSiteverifyRequestShape\|TestVerifyRouteRearmsOnlyOnRetry\|TestVerifyRouteLimitsAndHeaders\|TestClientIP)$' ./alita/utils/captchaweb` | ❌ W0 | ⬜ pending |
| 08-XX-XX | XX | N | CAPT-05 | T-08 (integrity) | A web pass unmutes with the chat's default permissions, posts the held-message summary, deletes the challenge, sends the greeting and clears the rows exactly like the button pass (shared function); an unmute failure after the claim schedules the durable retry and never reports `passed` | integration | `... -run '^(TestCaptchaWebPassUnmutesSummarisesAndCleansUp\|TestCaptchaPassSharedByCallbackAndWeb\|TestCaptchaVerifyCallback\|TestCaptchaWebUnmuteFailureSchedulesRetry)' ./alita/modules` | ❌ W0 | ⬜ pending |
| 08-XX-XX | XX | N | CAPT-06 | T-08 (self-inflicted DoS) | A provable failed solve applies the group's failure action once (kick lets the member rejoin) and returns `failed`; a Cloudflare outage, secret error or hostname mismatch never kicks | integration | `... -run '^(TestCaptchaWebFailedSolveAppliesAction\|TestCaptchaWebCloudflareErrorIsNotAFailure)$' ./alita/modules` | ❌ W0 | ⬜ pending |
| 08-XX-XX | XX | N | CAPT-07 | T-08 (repudiation, tampering) | Timeout on one replica while another serves verify gives exactly one outcome; pass and timeout racing give exactly one of unmute or kick (`-race`); a pass after expiry applies the failure at once; a restart resumes a Turnstile attempt like any other | integration | `... -run '^(TestCaptchaWebTimeoutAndVerifyOnDifferentReplicas\|TestCaptchaWebPassVersusTimeoutRace\|TestCaptchaWebPassAfterExpiryExpiresAtOnce\|TestRunOrphanedCaptchaRecoveryResumesValidAttempts)$' ./alita/modules` | ❌ W0 | ⬜ pending |
| 08-XX-XX | XX | N | CAPT-08 | T-08 (XSS, clickjacking) | Embedded string table has all 7 languages and every page key, English fallback, length limits; one unique CSP nonce per response, the header values, no `X-Frame-Options`, a safe i18n block; the page script maps languages, `cData` and states correctly | unit, route, `node --test` (executor-run) | `go test -tags testtools -race -count=1 -run '^TestCaptchaWebLocaleKeys$' ./alita/i18n`, `... -run '^(TestCaptchaWebPageStrings\|TestPageHeadersAndNonce)$' ./alita/utils/captchaweb`, `node --test scripts/captcha-page/` | ❌ W0 | ⬜ pending |
| 08-XX-XX | XX | N | PLAT-02 | T-08 (information disclosure) | Config is none-or-all, names variables not values, requires https; the Turnstile secret is registered and scrubbed; secrets and inputs never reach a log even with redaction off and a Cloudflare double that echoes the secret | unit, integration | `... -run '^TestCaptchaWebConfig$' ./alita/config`, `... -run '^TestCaptchaWebNeverLogsInputs$' ./alita/utils/captchaweb` | ❌ W0 | ⬜ pending |
| 08-XX-XX | XX | N | CAPT-08 (D-17, D-10) | — | Counters appear in the developer `/stats`; routes are absent without settings and present with them; `/captcha/health` is 200 with Redis and 503 without; a finish task joins the lifecycle drain | integration, route | `... -run '^(TestCaptchaWebStatsSection\|TestCaptchaWebFinishJoinsLifecycleDrain)$' ./alita/modules`, `... -run '^(TestCaptchaWebRoutesRegistration\|TestHealthRoute)$' ./alita/utils/captchaweb ./alita/utils/httpserver` | ❌ W0 | ⬜ pending |
| 08-XX-XX | XX | N | PLAT-04 | — | The self-hosting page, `environment.md`, `sample.env` and the regenerated captcha command page are consistent and drift-free | docs | `make generate-docs && make check-docs` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `alita/utils/captchaweb/*_test.go` and a fake-Cloudflare `httptest` helper (switches: outcome, echo the secret, slow, 500, non-JSON, redirect)
- [ ] A modules-level harness helper: miniredis wired into `cache` (the raid tests already do it) and a `moduleBotClient` per replica; the default client already answers `getChatMember` for the bot as an administrator with `can_restrict_members`, so only the no-rights case needs a response override; `getChat`, `sendMessage`, `deleteMessage`, `restrictChatMember`, `banChatMember` and `unbanChatMember` responses must exist for the pass, failure and timeout tests
- [ ] `alita/i18n` locale test for the `captcha_web_` keys (pattern: `lockdown_locale_test.go`)
- [ ] `scripts/captcha-page/` `node --test` file (executor-run, not part of `make test`)
- [ ] `TestCaptchaModeTurnstileConstraint` added to the `-run` regex of `make test-postgres-integrity` with its package
- [ ] Known-answer `initData` vectors from the published sources stored as test constants (`#nosec G101` if flagged)
- [ ] No harness `AutoMigrate` list change is needed (`CaptchaSettings` and `CaptchaAttempts` are already in every list)

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| The direct link posted by the bot in a group opens the Mini App in Android, iOS, Desktop and Web, delivers the tapper's `initData`, and a second member's tap is rejected | CAPT-02, CAPT-03 | Needs a real bot, a public hostname, BotFather and real Telegram clients | Follow the live checklist in `08-SPIKE.md` |
| The Turnstile widget renders and returns a token inside each WebView; real `siteverify` hostname for the owner's domain; CSP and `Referrer-Policy` behave in every client; whole-check latency stays inside the 12 s client timeout | CAPT-04, CAPT-08 | Needs the owner's Cloudflare keys and real clients | `08-SPIKE.md` |
| Whether `start_param` is signed in `initData`, the `startapp` length limit, the first-launch dialog, compact mode, theme and safe-area rendering, `crypto.subtle` availability | CAPT-02, CAPT-08 | Telegram client behaviour that official pages could not be read for | `08-SPIKE.md` |
| BotFather accepts the owner's short name and URL | PLAT-04 | Needs the owner's BotFather chat | Self-hosting guide steps |
| Native-speaker read of the new `captcha_web_*` strings in es, fr, hi, id, pt and ru | CAPT-08 | Translation quality | Review the new keys |
| The self-hosting guide reads correctly against the nine steps of D-15 | PLAT-04 | Prose | Manual read-through |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 90s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
