---
phase: 08-turnstile-web-captcha
plan: 10
subsystem: captcha
tags: [captcha, turnstile, httpserver, wiring, stats]
requires:
  - phase: 08-02
    provides: CaptchaWebConfigured and the config fields
  - phase: 08-05
    provides: captchaweb.NewPage, PageKeys, PageLanguages
  - phase: 08-07
    provides: i18n StringTable
  - phase: 08-08
    provides: the verify handler, SetReady, StartNotReady, Routes
  - phase: 08-09
    provides: /captchamode turnstile
provides:
  - "Server.Handle and the recorded endpoint list"
  - "CaptchaWebConfig.TrustedProxies, PageStrings, StartNotReady; CaptchaWebRoutes, MarkCaptchaWebReady, CaptchaWebStartupWarnings"
  - "main.go wiring (registerCaptchaWeb) and MarkCaptchaWebReady at the end of postInit"
  - "captchaweb/stats.go counters and the developer /stats section"
affects: [08-11]
tech-stack:
  added: []
  patterns:
    - "Closed-until-ready routes: the handler is built StartNotReady and opened at the end of postInit"
    - "Count once: writeAnswer counts, writeBody replays, writeLimited counts a refusal as its own event"
key-files:
  created:
    - alita/utils/captchaweb/stats.go
    - alita/modules/captcha_web_wiring_test.go
  modified:
    - main.go
    - alita/utils/httpserver/server.go
    - alita/utils/httpserver/server_test.go
    - alita/modules/captcha_web.go
    - alita/modules/captcha.go
    - alita/modules/devs.go
    - alita/utils/captchaweb/verify.go
    - AGENTS.md
key-decisions:
  - "Without Redis main installs no runtime and mounts no route (one warning), instead of the plan's runtime-without-Redis, because newCaptchaWebRuntime rejects a nil Redis client"
  - "A remembered answer replayed from the done marker is written with writeBody and counts nothing; an expired answer is not counted by writeAnswer because the expiry is counted where the attempt is claimed"
  - "Limited (429, in-flight 503) is its own event and is not also counted as Retry"
requirements-completed: [CAPT-02, CAPT-04, CAPT-08]
duration: 15 min
completed: 2026-10-30
status: complete
commits: 4
plan_head_before: 056826f0aa681a8666fb8b61a769cddac3b201e8
plan_head_after: 41507b41d8796a7ff585d80265574e0d90d56e1c
actuals:
  tokens: 18000
  tasks: 2
  commits: 4
coverage:
  - deliverable: "With the four settings set the page, verify and health routes are served on the bot's port through Server.Handle, the verify route closed until MarkCaptchaWebReady, and unset means no route"
    verification:
      - kind: test
        ref: "alita/utils/httpserver/server_test.go#TestCaptchaWebRoutesRegistration"
        status: pass
      - kind: test
        ref: "alita/modules/captcha_web_wiring_test.go#TestCaptchaWebWiring"
        status: pass
    human_judgment: false
  - deliverable: "Startup warns when METRICS_AUTH_TOKEN or CAPTCHA_WEB_TRUSTED_PROXIES is empty"
    verification:
      - kind: test
        ref: "alita/modules/captcha_web_wiring_test.go#TestCaptchaWebStartupWarnings"
        status: pass
    human_judgment: false
  - deliverable: "Developer /stats shows eight per-replica web captcha counters, each counted once, numbers only"
    verification:
      - kind: test
        ref: "alita/modules/captcha_web_wiring_test.go#TestCaptchaWebStatsSection"
        status: pass
    human_judgment: false
  - deliverable: "The production main.go path (registerCaptchaWeb with real locales, polling and webhook ordering) on a real bot, hostname and Cloudflare keys"
    verification: []
    human_judgment: true
    rationale: "main.go is wiring only and cannot run here (no bot token, no Cloudflare, no database); proven by the build, the --version smoke check and the tests of the functions it calls. The live check is deferred to 08-SPIKE.md"
---

# Phase 8 Plan 10: Wiring the Turnstile web captcha Summary

**Setting the four variables now mounts `GET /captcha`, `POST /captcha/verify` and `GET /captcha/health` on the bot's existing HTTP port through a new `Server.Handle`, with the verify route closed until the end of `postInit`, startup warnings about the shared port, and eight per-replica counters in the developer `/stats`.**

## Performance

- **Duration:** about 15 min (2026-10-30)
- **Tasks:** 2 of 2
- **Commits:** 4 (two RED test commits, two feature commits)

## Accomplishments

- `Server.Handle(pattern, handler)` mounts a Go 1.22 method pattern on the mux, logs `[HTTPServer] Registered <pattern>` and records it; `Start` now logs those patterns through `endpointList()` with the built-in ones.
- `CaptchaWebConfig` gained `TrustedProxies` (parsed with `ParseTrustedProxies`; a bad entry fails configuration and installs nothing), `PageStrings` (builds `captchaweb.NewPage` with the site key) and `StartNotReady`. New `CaptchaWebRoutes()` (nil without a runtime with a page), `MarkCaptchaWebReady()` (no-op without a runtime) and `CaptchaWebStartupWarnings(metricsTokenSet, trustedProxies)` (takes no secret).
- `main.go`: `registerCaptchaWeb` runs right after `RegisterDBMetrics` when `CaptchaWebConfigured()`: it builds the strings from `localeManager.StringTable(PageLanguages, PageKeys)` (fatal on error), calls `ConfigureCaptchaWeb` with `cache.GetRedisClient()`, `StartNotReady` true and the trusted proxies, mounts each route with `httpServer.Handle` and logs each warning. `modules.MarkCaptchaWebReady()` is the last statement of `postInit`. No handler group and no shutdown drain were added.
- `captchaweb/stats.go`: `Stats`, `StatsSnapshot`, `CountIssued`, `CountExpired` plus the internal answer/limited/error counters. Hooks: `writeAnswer` (by result), `writeLimited` (429 and in-flight 503), `countError` at configuration faults, Redis/backend errors and a failed unmute, `SendCaptcha` (Issued, after the message ID is stored) and `expireCaptchaAttempt` (Expired, when `handleCaptchaTimeout` claimed an attempt with a reserved answer).
- `captchaWebStatsSection()` prints the heading "Web captcha (this replica, since its last restart)" and one line with the eight numbers; `getStats` appends it after `raidAIStatsSection()`.
- AGENTS.md records the startup and readiness rule (Startup and shutdown), the no-handler-group note, the page script test command, the CSP and no-inline rules, `TestCaptchaWebLocaleKeys`/`TestStringTable`, and the counter rules.

## Task Commits

1. RED `test(08-10)`: `6e1f5bd` (TestCaptchaWebRoutesRegistration, TestCaptchaWebWiring, TestCaptchaWebStartupWarnings)
2. Task 1 `feat(08-10)`: `a9d571e` (Server.Handle, runtime config, routes, readiness, warnings, main.go, AGENTS.md)
3. RED `test(08-10)`: `119d70e` (TestCaptchaWebStatsSection)
4. Task 2 `feat(08-10)`: `41507b4` (stats.go, counter hooks, /stats section, AGENTS.md)

## Verification

- `go test -tags testtools -race -count=1 -run '^TestCaptchaWebRoutesRegistration$' -v ./alita/utils/httpserver`: PASS (seven subtests).
- `-run '^(TestCaptchaWebWiring|TestCaptchaWebStartupWarnings)$' -v ./alita/modules`: PASS (the wiring test drives a join, the page, a 503 verify and health before ready, then a pass and unmute through a real `ServeMux`).
- `-run '^TestCaptchaWebStatsSection$' -v ./alita/modules`: PASS (eight subtests, deltas of `StatsSnapshot`).
- `-run 'Captcha|Stats|Devs' ./alita/modules ./alita/utils/captchaweb ./alita/utils/httpserver`: PASS. Whole `./alita/utils/httpserver`, `./alita/utils/captchaweb` and `./alita/i18n`: PASS.
- `CGO_ENABLED=0 go build ./...`, `go vet -tags testtools . ./alita/...`, `gofmt -l main.go alita`: clean. `go.mod`/`go.sum` unchanged.
- `make check-translations`: pass. `node --test scripts/captcha-page/*.test.mjs`: 24 pass.
- `--version` smoke: binary built to the scratchpad and run from `/tmp`, printed `v2.23.14`.
- Not run: `make lint` (golangci-lint is built with go1.25 against module go1.26.0 in this sandbox; `go vet` and `gofmt` stood in), the whole `./alita/modules` package and `make test` (the orchestrator runs them between waves), `make check-docs` (no en.yml or docs change).
- Acceptance greps: Handle 1, `MarkCaptchaWebReady()` 1, `CaptchaWebConfigured()` 1, `CaptchaWebRoutes()` 1, AGENTS.md `MarkCaptchaWebReady` 1 and `node --test scripts/captcha-page/` 1, `AddHandlerToGroup` in captcha_web.go 0, `StatsSnapshot` 1, `captchaWebStatsSection()` in devs.go 1, `CountIssued()` 1, `CountExpired()` 1.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Plan symbol] A nil Redis client cannot build a runtime**
- **Found during:** Task 1
- **Issue:** The plan says "nil without Redis: the runtime exists, the mode falls back to math and health says unavailable". `newCaptchaWebRuntime` (plan 08-01) returns an error for a nil Redis client, so passing nil would make `main` fatal at startup, and the store would dereference nil.
- **Fix:** `registerCaptchaWeb` checks `cache.GetRedisClient()`; with none it logs one warning and installs no runtime and mounts no route. Turnstile groups then get the math captcha (D-09) and `/captchamode turnstile` refuses with `captcha_web_refuse_not_configured`. The only difference from the plan is that `/captcha/health` is absent instead of answering 503.
- **Files modified:** main.go
- **Commit:** a9d571e

**2. [Rule 2 - Counting rule] Replays and refusals must not be double counted**
- **Found during:** Task 2
- **Issue:** "Counted once per event" is not met if `writeAnswer` counts every write: a remembered pass replayed from `alita:captcha:done:<token>` would add a second Passed, and a 429 would add both Limited and Retry.
- **Fix:** `writeAnswer` counts then calls `writeBody`; the replay path uses `writeBody` and a refusal uses `writeLimited`. An `expired` answer is not counted in `writeAnswer` because `CountExpired` counts the expiry where the attempt is claimed.
- **Files modified:** alita/utils/captchaweb/verify.go
- **Commit:** 41507b4

**3. [Test scope] One extra assertion in the wiring test file**
- The `/stats` ordering check (web section after the AI section) lives in `captcha_web_wiring_test.go` and reuses `ownerStatsText` from `raid_ai_stats_test.go`, so no file outside `files_modified` was touched.

**Total deviations:** 2 auto-fixed (1 Rule 1, 1 Rule 2), 1 test-scope note. **Impact:** none on the contract; the first only drops `/captcha/health` for a bot with the settings set but no Redis.

The commit trailer uses the attribution lines the harness supplied for this session, not the model line printed in the plan's constraints.

## Pending Human Verification

These need a real bot, hostname and Cloudflare keys, which the sandbox cannot reach, and are deferred to `08-SPIKE.md` (plan 08-11):

1. Set `CAPTCHA_WEB_BASE_URL`, `TELEGRAM_MINI_APP_SHORT_NAME`, `TURNSTILE_SITE_KEY` and `TURNSTILE_SECRET_KEY` on a deployed bot behind the owner's proxy, restart, and check the log shows `[HTTPServer] Registered GET /captcha`, `POST /captcha/verify` and `GET /captcha/health`, plus the two `[CaptchaWeb]` warnings when `METRICS_AUTH_TOKEN` and `CAPTCHA_WEB_TRUSTED_PROXIES` are empty.
2. `curl -i https://<host>/captcha/health` answers `200 {"status":"ok"}` once the bot reports started; in polling mode, a request made during startup answers 503.
3. In a Turnstile group, join with a second account, tap the button, solve the widget and see the member unmuted; then run `/stats` as the owner and see the Web captcha line with issued 1 and passed 1.
4. Confirm the public proxy forwards only `/captcha` and `/captcha/*`.

## Known Stubs

None.

## Threat Flags

None: the routes and the shared port are in the plan's threat model (T-08-41 to T-08-44), and the `/stats` section prints numbers only (T-08-45).

## Self-Check: PASSED

Created files exist (`alita/utils/captchaweb/stats.go`, `alita/modules/captcha_web_wiring_test.go`), the four commits are on the branch (`6e1f5bd`, `a9d571e`, `119d70e`, `41507b4`), and every acceptance command above printed the expected value.
