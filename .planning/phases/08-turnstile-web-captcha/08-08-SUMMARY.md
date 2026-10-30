---
phase: 08-turnstile-web-captcha
plan: 08
subsystem: captcha
status: complete
tags: [turnstile, rate-limit, redis, client-ip, readiness, health, idempotency, go]

requires:
  - phase: 08-turnstile-web-captcha
    provides: "08-06 verify outcomes, Store.Rearm, outcomeBackend and newOutcomeHandler test helpers"
provides:
  - "Limiter.Allow: Redis fixed windows shared by every replica (60 per address, 20 per verified user, 8 per token and verified user)"
  - "ClientIP and ParseTrustedProxies: RemoteAddr unless the peer is a trusted proxy, rightmost untrusted X-Forwarded-For"
  - "Request hygiene on the verify route: JSON only (415), 16 KiB (413), strict body (400), 64 in flight (503), no-store and nosniff on every answer"
  - "Handler.SetReady, Options.StartNotReady, Handler.Health, Handler.Routes and the RoutePage, RouteVerify and RouteHealth patterns"
  - "Store.MarkDone, Store.Done, Decision, Store.Ping: a decided answer is replayed for 10 minutes to the same verified user"
affects: [08-09, 08-10, 08-11]

actuals:
  tokens: 17000
  tasks: 3
  commits: 6
plan_head_before: 6c8f10545a998fd59a40b991477f8bbb06a9ffc3
plan_head_after: 165c9eeba1a69f08c0a02b38bfba90479eea8593

tech-stack:
  added: []
  patterns:
    - "Fixed-window limiter in one Lua script (INCR, PEXPIRE on first, PTTL), a Redis error refuses"
    - "Per-user budgets keyed by the signed initData user, never by the public token alone"
    - "Answer remembered before it is written, so a lost response can be asked again"

key-files:
  created:
    - alita/utils/captchaweb/limiter.go
    - alita/utils/captchaweb/clientip.go
    - alita/utils/captchaweb/limiter_test.go
    - alita/utils/captchaweb/clientip_test.go
    - alita/utils/captchaweb/verify_limits_test.go
  modified:
    - alita/utils/captchaweb/verify.go
    - alita/utils/captchaweb/challenge.go
    - alita/modules/captcha_web_outcomes_test.go
    - AGENTS.md

key-decisions:
  - "An IPv6 client is counted by its /64 in the per-address window, so one host cannot mint 2^64 buckets"
  - "A trusted proxy's own address, or an all-trusted or unparsable forwarding chain, is not a bucket (bucket false): the per-address window is skipped and the per-user windows still apply"
  - "The done marker is written with SET NX, so the first decision of a token wins"
  - "A retry answer (including a claimed pass whose unmute failed) writes no marker"
  - "Store.Done returns (Decision, found, error) and treats an unparsable value as no decision"

requirements-completed: [CAPT-03, CAPT-04]

coverage:
  - id: D1
    description: "A griefer using another member's public token spends only their own budgets: 100 attempts end in 429 retry with Retry-After, the target's challenge is untouched, the backend saw nothing for the griefer, and the target then passes from its own address"
    requirement: CAPT-03
    verification:
      - kind: integration
        ref: "alita/utils/captchaweb/verify_limits_test.go#TestVerifyRouteWrongUserChangesNothing"
        status: pass
    human_judgment: false
  - id: D2
    description: "Floods are refused cheapest first and counted in Redis: 64 in flight, 60 per address, 20 per user, 8 per token and user; a Redis error refuses; a spoofed X-Forwarded-For only counts behind a trusted proxy"
    requirement: CAPT-04
    verification:
      - kind: unit
        ref: "alita/utils/captchaweb/limiter_test.go#TestWindowLimiter"
        status: pass
      - kind: unit
        ref: "alita/utils/captchaweb/clientip_test.go#TestClientIP"
        status: pass
      - kind: integration
        ref: "alita/utils/captchaweb/verify_limits_test.go#TestVerifyRouteLimitsAndHeaders"
        status: pass
    human_judgment: false
  - id: D3
    description: "Only JSON under 16 KiB with known fields reaches the logic; other methods get 405 with Allow: POST; every answer is no-store, nosniff and has no CORS header"
    requirement: CAPT-04
    verification:
      - kind: integration
        ref: "alita/utils/captchaweb/verify_limits_test.go#TestVerifyRouteLimitsAndHeaders"
        status: pass
    human_judgment: false
  - id: D4
    description: "The route answers 503 retry, touching neither Redis nor the backend, until SetReady(true); GET /captcha/health reports ok only when ready and Redis answers; a slow check still delivers its answer past the server's 10 s WriteTimeout"
    requirement: CAPT-04
    verification:
      - kind: integration
        ref: "alita/utils/captchaweb/verify_limits_test.go#TestVerifyRouteReadinessGate"
        status: pass
      - kind: integration
        ref: "alita/utils/captchaweb/verify_limits_test.go#TestHealthRoute"
        status: pass
      - kind: integration
        ref: "alita/utils/captchaweb/verify_limits_test.go#TestVerifyRouteOutlivesServerWriteTimeout"
        status: pass
    human_judgment: false
  - id: D5
    description: "A reused or parallel token acts once (50 concurrent consumes give one ConsumeOK, 8 parallel verifies make one Attempt call and one Cloudflare request) and a repeat from the same verified user replays the decided answer with no side effect while another user gets not_for_you"
    requirement: CAPT-03
    verification:
      - kind: integration
        ref: "alita/utils/captchaweb/verify_limits_test.go#TestChallengeStoreConsumeOnce"
        status: pass
      - kind: integration
        ref: "alita/utils/captchaweb/verify_limits_test.go#TestVerifyRouteReplaysDecision"
        status: pass
    human_judgment: false
---

# Phase 8 Plan 08: Verify Route Abuse Limits Summary

**The public Turnstile verify route now refuses floods with Redis windows shared by every replica, speaks only small strict JSON, waits for readiness, has a health check, outlives the server write timeout, and replays a decided answer once to the same member.**

## Accomplishments

- Three fixed windows (`alita:captcha:rl:ip:<ip>` 60, `rl:u:<user>` 20, `rl:t:<token>:<user>` 8, per 60 s) in one Lua script; a Redis error answers 503 retry, a spent budget 429 retry with `Retry-After`. The token and user windows are keyed by the signed initData user, so a griefer hammering a target's token spends only their own budget and the target still passes.
- `ClientIP` reads `RemoteAddr`, or the rightmost untrusted `X-Forwarded-For` entry behind `CAPTCHA_WEB_TRUSTED_PROXIES`; `X-Real-IP` and `CF-Connecting-IP` are never read.
- Verify order: write deadline extended, method, ready flag, 64-in-flight cap, address window, content type (415), 16 KiB cap (413), strict JSON (400), initData, user window, start_param, token-and-user window, consume.
- `Handler.SetReady`, `Options.StartNotReady`, `Handler.Health` (`{"status":"ok"|"unavailable"}` only), `Handler.Routes` and the three route constants for plan 08-10 to mount.
- `alita:captcha:done:<token>` (`<user>:<result>[:<action>]`, 10 minutes): written before the answer for passed, failed and expired, replayed to the same verified user, `not_for_you` for another, never written for a retry.
- AGENTS.md: rate-window Data bullet, done-marker Data bullet, client-address rule, and the verify route rules under the Turnstile captcha bullet.

## Task Commits

1. Task 1 RED `f5b4692` (test), GREEN `592e167` (feat): windows and client address
2. Task 2 RED `11eb785` (test), GREEN `ebb96bc` (feat): hygiene, readiness, health, write deadline
3. Task 3 RED `74a1640` (test), GREEN `165c9ee` (feat): consume once and replay

Each RED commit does not compile on its own (the symbols it tests do not exist yet); that is the failing state. Verified by a mutation: removing the write-deadline extension makes `TestVerifyRouteOutlivesServerWriteTimeout` fail with `EOF`.

## Deviations from Plan

**1. [Rule 1 - Bug] Plan 08-06 module tests expected `invalid` on a repeat**
- **Found during:** Task 3
- **Issue:** `TestCaptchaWebPassAfterExpiryExpiresAtOnce` and `TestCaptchaWebFailedSolveAppliesAction` asserted a second verify of a decided token answers `invalid`. D-03 (this plan) makes it replay the decided answer with no side effect.
- **Fix:** both now expect the replayed `expired` and `failed` with the same action; their existing assertions that nothing was kicked, banned or sent again stay.
- **Files modified:** `alita/modules/captcha_web_outcomes_test.go`
- **Commit:** `165c9ee`

**2. [Rule 2 - Security] IPv6 clients counted by /64** (not in the plan): a single host owns a whole /64, so a per-/128 key would give an attacker unlimited buckets. `limitKeyForAddr` masks IPv6 to /64. Commit `592e167`.

**Total deviations:** 2 auto-fixed (1 bug in an earlier plan's test, 1 missing hardening). **Impact:** none on scope.

## Verification

- `CGO_ENABLED=1 go test -tags testtools -race -count=1 ./alita/utils/captchaweb`: ok; `TestChallengeStoreConsumeOnce` also ran 20 times under `-race` clean.
- `go test ... -run 'Captcha' ./alita/modules`: ok (includes every `TestCaptchaWeb` and `TestSendCaptchaTurnstile` test).
- `CGO_ENABLED=0 go build ./...`, `go vet -tags testtools ./alita/utils/captchaweb ./alita/modules`, `gofmt -l alita/utils/captchaweb` all clean; `go.mod` and `go.sum` unchanged.
- `make lint` could not run in this sandbox (golangci-lint is built with go1.25, the module is go1.26.0); `gofmt` and `go vet` were run instead. The whole `make test` was not run (the orchestrator runs it between waves).
- Acceptance greps: `SetWriteDeadline` 1, route constants 3, `SetReady` 1, `StartNotReady` in AGENTS.md present, `alita:captcha:rl:` and `alita:captcha:done:` in AGENTS.md present, no `X-Real-IP` or `CF-Connecting-IP` outside comments in `clientip.go`.

## Known Stubs

None.

## Threat Flags

None beyond the plan's register: T-08-31 to T-08-37 are each covered by the tests named above. The routes are not mounted yet (plan 08-10), so nothing new is publicly reachable from this plan.

## Pending Human Verification

None for this plan. The reverse-proxy and live Cloudflare checks stay with 08-SPIKE.md.

## Self-Check: PASSED

All created files exist, all six commits are in `git log`, and the focused and package test runs above passed.
