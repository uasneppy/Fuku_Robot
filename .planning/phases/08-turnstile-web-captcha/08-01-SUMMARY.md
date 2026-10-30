---
phase: 08-turnstile-web-captcha
plan: 01
subsystem: captcha
tags: [turnstile, telegram-mini-app, redis, initdata-hmac, cloudflare-siteverify, go]

requires:
  - phase: 07-ai-assisted-raid-detection
    provides: "Redis operational-key conventions, the raid Lua script idiom, the stdlib-leaf client shape (gemini)"
provides:
  - "alita/utils/captchaweb: challenge store (alita:captcha:ch:<token>), initData validator, siteverify client, verify handler behind a Backend interface, testtools fixtures"
  - "alita/modules/captcha_web.go: CaptchaWebConfig, ConfigureCaptchaWeb, captchaWebAvailable (memoised PING), captchaWebBackend"
  - "captcha.go: claimCaptchaPass and finishCaptchaPass shared by the button callback and the web verify; the turnstile branch of SendCaptcha; the ts1: refusal in both callbacks"
  - "captcha_mode CHECK tag accepts turnstile (SQLite harness); locale keys captcha_web_welcome and captcha_web_button in 7 languages"
affects: [08-02, 08-03, 08-04, 08-05, 08-06, 08-07, 08-08, 08-09, 08-10]

actuals:
  tokens: 24000
  tasks: 3
  commits: 6
plan_head_before: c3cd9515ed551fedcfcea411661d6a30faf2c90f
plan_head_after: 66c026eebdd7019d6904572387a6d61b86ce3b0b

tech-stack:
  added: []
  patterns:
    - "User-compared Lua consume: a wrong user returns {2} and changes nothing, so a public token cannot be burned by another member"
    - "One shared pass path (claim then finish) for every way a captcha can be passed"
    - "Reserved unguessable answer (ts1: + 128 random bits) as the only marker of a Turnstile attempt, refused by the old callbacks"
    - "Backend interface in a leaf package, implemented in modules, so the verify handler is testable without a database"

key-files:
  created:
    - alita/utils/captchaweb/challenge.go
    - alita/utils/captchaweb/initdata.go
    - alita/utils/captchaweb/siteverify.go
    - alita/utils/captchaweb/verify.go
    - alita/utils/captchaweb/fixtures_testtools.go
    - alita/modules/captcha_web.go
    - alita/modules/captcha_web_test.go
  modified:
    - alita/modules/captcha.go
    - alita/db/models/captcha.go
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml
    - AGENTS.md

key-decisions:
  - "PassResult's zero value is PassLost and ConsumeResult's is ConsumeUnknown, so an unset value can never read as a pass or a consumed challenge"
  - "Result is a named string type; Action stays a plain string (kick, ban, mute)"
  - "A signed start_param that differs from the posted challenge answers invalid; a bad initData answers 200 invalid (a decided answer), a malformed body 400 invalid, an oversized body 413 invalid, a store or backend error 503 retry"
  - "The availability memo is keyed by runtime pointer as well as time, so replacing the runtime (a test, a reconfigure) never reuses an answer probed for another Redis"
  - "Nothing is re-armed after a consume in this plan: every non-pass Cloudflare answer is a retry that leaves the challenge consumed (plan 08-06 adds the re-arm)"

patterns-established:
  - "Turnstile attempt = captcha_attempts row with a ts1: answer; no second table or kind column"
  - "Web finish runs through startCaptchaLifecycleTask (drained by StopCaptchaLifecycle) or inline; post-claim work on context.WithoutCancel with its own deadline"

requirements-completed: [CAPT-02, CAPT-03, CAPT-04, CAPT-05]

coverage:
  - id: D1
    description: "A member who joins a turnstile group (web captcha configured, Redis answering, bot has a username) is muted, gets a ts1: attempt and exactly one URL button https://t.me/<bot>/<short>?startapp=<22-char token>; the token is stored in Redis with the attempt's time left plus 5 minutes"
    requirement: CAPT-02
    verification:
      - kind: integration
        ref: "alita/modules/captcha_web_test.go#TestCaptchaWebTracerJoinVerifyUnmute/join_posts_a_Mini_App_button"
        status: pass
    human_judgment: false
  - id: D2
    description: "A POST with initData signed for the member, the Turnstile token Cloudflare confirms (hostname, captcha_join, cData) and the challenge answers passed; the member is unmuted, the held-message summary is posted, the challenge deleted and the attempt row gone, through the same pass path as the button"
    requirement: CAPT-05
    verification:
      - kind: integration
        ref: "alita/modules/captcha_web_test.go#TestCaptchaWebTracerJoinVerifyUnmute/the_target_passes"
        status: pass
      - kind: integration
        ref: "alita/modules/captcha_web_test.go#TestCaptchaWebTracerJoinVerifyUnmute/held_messages_are_summarised"
        status: pass
      - kind: integration
        ref: "alita/modules/captcha_web_test.go#TestCaptchaPassSharedByCallbackAndWeb"
        status: pass
    human_judgment: false
  - id: D3
    description: "Only the target can pass: another member's verify answers not_for_you and changes nothing, an unconfirmed Turnstile token, stale initData or a malformed body never unmute anyone"
    requirement: CAPT-03
    verification:
      - kind: integration
        ref: "alita/modules/captcha_web_test.go#TestCaptchaWebTracerJoinVerifyUnmute/another_member_cannot_pass_or_burn_it"
        status: pass
      - kind: integration
        ref: "alita/modules/captcha_web_test.go#TestCaptchaWebTracerJoinVerifyUnmute/a_token_Cloudflare_rejects_never_unmutes"
        status: pass
      - kind: integration
        ref: "alita/modules/captcha_web_test.go#TestCaptchaWebTracerJoinVerifyUnmute/a_malformed_body_is_invalid"
        status: pass
      - kind: integration
        ref: "alita/modules/captcha_web_test.go#TestCaptchaWebTracerJoinVerifyUnmute/stale_initData_is_invalid"
        status: pass
    human_judgment: false
  - id: D4
    description: "A hand-built captcha_verify (with the stored answer or a wrong one) or captcha_refresh callback for a Turnstile attempt is answered captcha_attempt_not_valid and changes nothing"
    requirement: CAPT-03
    verification:
      - kind: integration
        ref: "alita/modules/captcha_web_test.go#TestCaptchaWebAttemptRefusesCallbacks"
        status: pass
    human_judgment: false
  - id: D5
    description: "A turnstile group whose web captcha is unavailable (no runtime, Redis not answering, no bot username) gives the math captcha; an issue or send failure rolls back and unmutes; an old token after a re-join answers expired; restart recovery treats a Turnstile attempt like any other"
    requirement: CAPT-04
    verification:
      - kind: integration
        ref: "alita/modules/captcha_web_test.go#TestSendCaptchaTurnstileFallsBackToMath"
        status: pass
      - kind: integration
        ref: "alita/modules/captcha_web_test.go#TestSendCaptchaTurnstileRollsBackOnFailure"
        status: pass
      - kind: integration
        ref: "alita/modules/captcha_web_test.go#TestCaptchaWebRejoinInvalidatesOldChallenge"
        status: pass
      - kind: integration
        ref: "alita/modules/captcha_web_test.go#TestRunOrphanedCaptchaRecoveryResumesTurnstileAttempt"
        status: pass
    human_judgment: false

duration: 18min
completed: 2026-10-30
status: complete
---

# Phase 8 Plan 01: Turnstile Web Captcha Tracer Summary

**A member who joins a Turnstile group gets one Mini App button and is unmuted only after the bot's server checks Telegram's signed initData and Cloudflare's siteverify answer, through a pass path shared with the button captcha; forged callbacks cannot pass or convert a Turnstile attempt.**

## Performance

- **Duration:** 18 min
- **Started:** 2026-10-30T00:26:34Z
- **Completed:** 2026-10-30T00:45:00Z
- **Tasks:** 3 (1 tracer, 2 auto/tdd)
- **Files modified:** 17 (7 new)

## Accomplishments

- Tracer: join, Redis challenge handle, URL button, signed POST, fake Cloudflare, unmute, all green end to end (`TestCaptchaWebTracerJoinVerifyUnmute`, 7 subtests). The tracer verify was re-run before expansion: "Tracer verified end-to-end - expanding".
- The pass path of `captchaVerifyCallback` is extracted into `claimCaptchaPass` and `finishCaptchaPass`; the existing callback tests pass unchanged and `TestCaptchaPassSharedByCallbackAndWeb` pins that both passes make the same ordered Telegram calls.
- D-25 closed: both `captcha_verify` and `captcha_refresh` refuse an attempt with a `ts1:` answer before any comparison, count, cooldown write or image.
- D-09: a turnstile group falls back to math when the runtime is missing, the bot has no username or Redis does not answer a PING within 250 ms (memoised for 5 s); an issue or send failure rolls the attempt back and unmutes.
- 7-language locale keys `captcha_web_welcome` and `captcha_web_button`; `make check-translations` and `make check-docs` clean.

## Task Commits

1. **Task 1: tracer** - `8d1b52d` (test, RED: does not build until the package exists), `feef68c` (feat)
2. **Task 2: forged buttons** - `d06aaa4` (test, RED: the forged `captcha_verify` passed), `744ed93` (fix)
3. **Task 3: join edge cases** - `6f35bcd` (test), `66c026e` (docs: AGENTS.md rules)

**Plan metadata:** the commit that adds this SUMMARY (docs: complete plan).

## Files Created/Modified

- `alita/utils/captchaweb/challenge.go` - token, reserved answer, cData, Mini App link, Record, Store.Issue, Store.ConsumeIfUser (user-compared Lua)
- `alita/utils/captchaweb/initdata.go` - ValidateInitData (HMAC with `WebAppData`, signature field included, 5-minute window, `user.id > 0`)
- `alita/utils/captchaweb/siteverify.go` - Client.Verify with Outcome/Reason/Verdict; no redirect, 64 KiB cap, 5 s timeout, nothing wrapped or logged
- `alita/utils/captchaweb/verify.go` - Handler, Options, Backend, Answer/Result, PassResult, Verify route logic
- `alita/utils/captchaweb/fixtures_testtools.go` - signer, fake Cloudflare handlers (testtools tag only)
- `alita/modules/captcha_web.go` - runtime, availability probe, backend (Attempt, Pass)
- `alita/modules/captcha.go` - turnstile branch, shared pass functions, callback guards
- `alita/modules/captcha_web_test.go` - env helpers and the seven named tests
- `alita/db/models/captcha.go` - `chk_captcha_mode` tag includes `turnstile`
- `locales/*.yml` (7), `AGENTS.md` - two keys each; Data bullet for `alita:captcha:ch:<token>` and the "Turnstile captcha" trap bullet

## Decisions Made

See `key-decisions` above. Redis key layout, the consume script, the verify JSON contract and the `Backend` interface are the stable surface plans 08-03 to 08-10 build on.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Plan wrong about existing code] A kick is one `unbanChatMember`, not ban then unban**
- **Found during:** Task 3 (`TestRunOrphanedCaptchaRecoveryResumesTurnstileAttempt`)
- **Issue:** The plan says an expired attempt is expired "with the group's failure action (banChatMember then unbanChatMember for kick)". In this codebase `kickMember` is a single `UnbanChatMember` with `OnlyIfBanned: false`; the existing recovery test also expects one unban and no ban for a kick.
- **Fix:** The test asserts no `banChatMember`, exactly one `unbanChatMember` for the expired member with `only_if_banned` false, the expired challenge message deleted and the live one kept. No production change.
- **Files modified:** `alita/modules/captcha_web_test.go`
- **Verification:** the test passes
- **Committed in:** `6f35bcd`

**2. [Rule 3 - Blocking] Commit trailer uses the harness attribution, not the plan's**
- **Found during:** Task 1 commit
- **Issue:** The plan text names a different model in the `Co-Authored-By` trailer than the attribution the harness gave this session, and the orchestrator notes say to use the harness trailer.
- **Fix:** Every commit ends with the harness `Co-Authored-By` and `Claude-Session` lines (the first commit was amended before anything else was built on it).
- **Files modified:** none (commit messages only)

**3. [Rule 3 - Blocking] Plan commit ledger (protocol 0c) could not be written**
- **Found during:** before the first commit
- **Issue:** the sandbox refuses writes under the worktree's git directory, so the on-disk ledger file was not created.
- **Fix:** `commits`, `plan_head_before` and `plan_head_after` are measured with `git rev-list --count c3cd9515...HEAD` against the base SHA the dispatch prompt gave (it equals the worktree's starting HEAD, asserted first). Result: 6 commits before this SUMMARY.

### TDD notes (not deviations)

- The Task 1 RED commit (`8d1b52d`) does not compile: it references the `captchaweb` package and `newCaptchaWebRuntime`, which Task 1's GREEN commit adds. The plan asks for exactly this.
- `TestCaptchaPassSharedByCallbackAndWeb` and the four Task 3 tests were green when first run, because Task 1's code already contains the shared functions, the fallback, the rollback and the attempt-ID matching (the plan says so for Task 3). They are characterization tests; the one true RED of Tasks 2 and 3 is `TestCaptchaWebAttemptRefusesCallbacks`, which failed with the forged `captcha_verify` answering `captcha_verified_success_msg` before `744ed93`.
- The test fixture sets the fake `getChat` answer to echo the env chat's ID. The web pass reads the chat from Telegram and claims on that chat's ID, so a fixed `-1001` would make every claim miss.

---

**Total deviations:** 3 auto-fixed (1 Rule 1, 2 Rule 3)
**Impact on plan:** none on behavior or scope; one test expectation corrected to the real kick call.

## Issues Encountered

- `make lint` cannot run in this sandbox (golangci-lint is built with an older Go than the module). `gofmt -l` over `alita/modules alita/utils/captchaweb alita/db/models` prints nothing and `go vet -tags testtools ./alita/modules ./alita/utils/captchaweb ./alita/db/models` is clean.
- The sandbox refuses compound git commands, so `git add` and `git commit` were run as separate plain commands.

## Known Limitations (accepted, owned by later plans)

- **No re-arm yet.** The verify handler consumes the challenge before it asks Cloudflare; every non-pass Cloudflare answer, and a backend error after the consume, answers `retry` with the challenge gone, so that member cannot try again and times out like any unverified member. Plan 08-06 adds the re-arm, failed-solve handling and immediate expiry. Nothing is reachable in production before plan 08-10 mounts the routes.
- `/captchamode turnstile` is not accepted yet (`SetCaptchaMode` is unchanged; the tests set the stored mode directly) and there is no PostgreSQL migration for `chk_captcha_mode` yet; both are plan 08-04. The SQLite CHECK comes from the model tag.
- No rate windows, content-type check or readiness gate on the verify route (plan 08-08); `initData` has no published known-answer vectors yet (plan 08-03).
- No unit tests inside `alita/utils/captchaweb` itself (the leaf package is exercised through the module tests, as the plan specifies); the strict initData parser, siteverify classification and limiter tests belong to plans 08-03, 08-06 and 08-08.

## Known Stubs

None. No placeholder values flow to any user-facing surface.

## User Setup Required

None - no external service configuration required. No live Cloudflare or Telegram call was made; the fake Cloudflare is an `httptest` server. Pending human verification: none for this plan (the real-device checks stay in `08-SPIKE.md`, plan 08-10 era).

## Verification Results

| Check | Result |
|-------|--------|
| `go test -tags testtools -race -count=1 -run '^TestCaptchaWebTracerJoinVerifyUnmute$' -v ./alita/modules` | PASS (7 subtests) |
| `... -run '^(TestCaptchaWebAttemptRefusesCallbacks\|TestCaptchaPassSharedByCallbackAndWeb)$'` | PASS |
| `... -run '^(TestSendCaptchaTurnstileFallsBackToMath\|TestSendCaptchaTurnstileRollsBackOnFailure\|TestCaptchaWebRejoinInvalidatesOldChallenge\|TestRunOrphanedCaptchaRecoveryResumesTurnstileAttempt)$'` | PASS |
| Existing callback regression net (`TestCaptchaVerifyCallback*`, `TestCaptchaRefreshCallback*`, `TestCaptchaVerifyRejectsButtonFromPreviousRefresh`, `TestCaptchaCallbacksPropagateGotgbotRequestErrors`) | PASS, unchanged |
| `go test -tags testtools -race -count=1 ./alita/modules ./alita/db/captcha ./alita/utils/captchaweb ./alita/i18n ./alita/db/models` (whole packages) | ok (modules 308 s) |
| `CGO_ENABLED=0 go build ./...` | ok |
| `go vet -tags testtools` on the three touched packages | clean |
| `gofmt -l` on the touched directories | empty |
| `make check-translations`, `make check-docs` | clean |
| `git diff --exit-code go.mod go.sum` (against the base) | clean |
| Acceptance greps (tag, function definitions, 14 locale keys, AGENTS.md mentions, `math captcha`, reserved-answer guard count 2, no module import in captchaweb) | all as the plan states |

## Threat Flags

None beyond the plan's threat model (T-08-01 to T-08-06 are mitigated by the tests listed under `coverage`). The verify handler is not mounted on any route in this plan.

## Next Phase Readiness

- Plans 08-02 onward can build on `captchaweb.Backend`, `Store`, the verify JSON contract and the `captchaWebEnv` helpers (`newCaptchaWebEnv`, `setSiteverify`, `join`, `verify`, `verifyBody`, `attempt`).
- Wave-1 siblings run on disjoint files; the orchestrator should run `make test` between waves.

## Self-Check: PASSED

- Created files exist: `alita/utils/captchaweb/{challenge,initdata,siteverify,verify,fixtures_testtools}.go`, `alita/modules/captcha_web.go`, `alita/modules/captcha_web_test.go`.
- Commits found: `8d1b52d`, `feef68c`, `d06aaa4`, `744ed93`, `6f35bcd`, `66c026e`.

---
*Phase: 08-turnstile-web-captcha*
*Completed: 2026-10-30*
