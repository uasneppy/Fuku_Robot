---
phase: 08-turnstile-web-captcha
plan: 06
subsystem: captcha
tags: [turnstile, cloudflare-siteverify, redis, rearm, exactly-once, logging, go]

requires:
  - phase: 08-turnstile-web-captcha
    provides: "08-01 captchaweb package, Backend interface, claimCaptchaPass/finishCaptchaPass; 08-04 for AGENTS.md ordering"
provides:
  - "siteverify classification per D-28 (Reason constants for every Cloudflare code, Verdict.ConfigError)"
  - "Store.Rearm: gives a consumed challenge back with its remaining lifetime, only when the key is absent and time remains"
  - "Backend.Fail, Backend.Expire, Backend.BotCanRestrict and their captchaWebBackend implementation"
  - "Handler outcome branches: failed solve, expired, retry with re-arm, live bot-rights preflight"
  - "Reusable test helpers: outcomeBackend, newOutcomeHandler (captchaweb); outcomeReplica, outcomeRace, withCaptchaLifecycleForTest (modules)"
affects: [08-07, 08-08, 08-09, 08-10, 08-11]

actuals:
  tokens: 23000
  tasks: 3
  commits: 5
plan_head_before: 20d409392ff3c7f1e5c2e0b79ed5ea237c0aed21
plan_head_after: 8404ef9d8039c7827164568c8a1eb331d7cf3ab2

tech-stack:
  added: []
  patterns:
    - "Verdict classification where only provable token failures are OutcomeFailed; the zero value is OutcomeRetry"
    - "Re-arm only on retry: remaining = PTTL returned by the consume minus time since, never a fresh lifetime"
    - "A Backend method returns an error only when it changed nothing, which makes re-arming after it safe"
    - "Every outcome decided by the conditional delete of the attempt row; the loser runs expireCaptchaAttempt, idempotent through its own claim"

key-files:
  created:
    - alita/utils/captchaweb/siteverify_test.go
    - alita/utils/captchaweb/verify_outcomes_test.go
    - alita/modules/captcha_web_outcomes_test.go
  modified:
    - alita/utils/captchaweb/siteverify.go
    - alita/utils/captchaweb/challenge.go
    - alita/utils/captchaweb/verify.go
    - alita/modules/captcha_web.go
    - alita/modules/captcha_web_test.go
    - AGENTS.md

key-decisions:
  - "Hostname and action mismatches win over a cdata mismatch: an owner's configuration fault is never turned into a kick"
  - "A success answer carried on an HTTP status of 400 or more is a retry (status), never a pass"
  - "The Client now refuses redirects whatever transport it was given (it copies the *http.Client and sets CheckRedirect), so the secret in the body can never follow a redirect"
  - "A failed Expire after an expired/lost-claim answer is logged and still answered expired: nothing changed, and the timer and cleanup tick own the row"
  - "captchaWebBackend.Fail reads the attempt and the settings first, so a database error is an error (re-arm, 503) and not a lost claim"

requirements-completed: [CAPT-04, CAPT-06, CAPT-07]

coverage:
  - id: D1
    description: "A provably failed check (invalid-input-response or timeout-or-duplicate as the only codes, or a confirmed token with another challenge's cdata) applies the group's failure action once through handleCaptchaTimeout, deletes the challenge message, posts the failure notice and answers failed with the action; a second verify answers invalid"
    requirement: CAPT-06
    verification:
      - kind: integration
        ref: "alita/modules/captcha_web_outcomes_test.go#TestCaptchaWebFailedSolveAppliesAction"
        status: pass
      - kind: unit
        ref: "alita/utils/captchaweb/siteverify_test.go#TestSiteverifyOutcome"
        status: pass
    human_judgment: false
  - id: D2
    description: "A Cloudflare, network or owner-configuration fault never kicks: 500, internal-error, a secret error, a hostname mismatch and a closed connection answer retry, the member stays muted with the attempt intact, the same token is given back and passes once Cloudflare is back"
    requirement: CAPT-04
    verification:
      - kind: integration
        ref: "alita/modules/captcha_web_outcomes_test.go#TestCaptchaWebCloudflareErrorIsNotAFailure"
        status: pass
      - kind: unit
        ref: "alita/utils/captchaweb/verify_outcomes_test.go#TestVerifyRouteRearmsOnlyOnRetry"
        status: pass
    human_judgment: false
  - id: D3
    description: "A check after the attempt expired, or one that lost the claim, expires the attempt at once with the group's action and answers expired; it never unmutes. A claimed pass whose unmute failed answers retry and leaves the durable captcha_muted_users row for the 5-minute worker"
    requirement: CAPT-07
    verification:
      - kind: integration
        ref: "alita/modules/captcha_web_outcomes_test.go#TestCaptchaWebPassAfterExpiryExpiresAtOnce"
        status: pass
      - kind: integration
        ref: "alita/modules/captcha_web_outcomes_test.go#TestCaptchaWebUnmuteFailureSchedulesRetry"
        status: pass
    human_judgment: false
  - id: D4
    description: "Pass, failed solve and timeout are decided once on any replica: a challenge issued on A passes on B, and 20 timeout-versus-check races on one and on two replicas each end in exactly one of unmute or kick"
    requirement: CAPT-07
    verification:
      - kind: integration
        ref: "alita/modules/captcha_web_outcomes_test.go#TestCaptchaWebTimeoutAndVerifyOnDifferentReplicas"
        status: pass
      - kind: integration
        ref: "alita/modules/captcha_web_outcomes_test.go#TestCaptchaWebPassVersusTimeoutRace"
        status: pass
    human_judgment: false
  - id: D5
    description: "Before any Cloudflare call the bot's own Restrict right is read live (3 s); without it, or when unknown, the answer is retry and the token is given back. The web finish joins the captcha lifecycle drain and runs inline after the stop"
    requirement: CAPT-04
    verification:
      - kind: integration
        ref: "alita/modules/captcha_web_outcomes_test.go#TestCaptchaWebBotWithoutRightsIsRetry"
        status: pass
      - kind: integration
        ref: "alita/modules/captcha_web_outcomes_test.go#TestCaptchaWebFinishJoinsLifecycleDrain"
        status: pass
    human_judgment: false
  - id: D6
    description: "No log entry on any verify path contains the secret, the Turnstile token, the challenge token, the raw initData, the member's name or Cloudflare's text, with the redactor off and a Cloudflare double that echoes them"
    requirement: CAPT-04
    verification:
      - kind: unit
        ref: "alita/utils/captchaweb/verify_outcomes_test.go#TestCaptchaWebNeverLogsInputs"
        status: pass
    human_judgment: false

duration: 35min
completed: 2026-10-30
status: complete
---

# Phase 8 Plan 06: Turnstile Check Outcomes Summary

**A Turnstile check now ends in exactly one of pass, kick-once or retry: only a provably failed token kicks, every Cloudflare, network or setup fault re-arms the same challenge with its remaining lifetime, and a late or racing check expires the attempt through the same conditional delete the timer uses, on any replica.**

## Performance

- **Duration:** about 35 min
- **Tasks:** 3 (all `tdd="true"`)
- **Files:** 9 (3 new test files, 6 modified)
- **Commits:** 5 before this SUMMARY

## Accomplishments

- D-28 classification in `Client.Verify` (`classifyFailure`): a failed solve needs a non-empty code list made only of `invalid-input-response` and `timeout-or-duplicate`, or a confirmed token with another cdata; secret, internal, bad-request, missing-response and unknown codes, hostname and action mismatches, transport errors, redirects, 5xx, oversized or undecodable bodies are retries. `Verdict.ConfigError` marks secret, hostname and action; the handler logs those once at Error as `[CaptchaWeb] siteverify configuration fault: <reason>`.
- `Store.Rearm` (one Lua script): remaining lifetime only, nothing when the key exists or time is spent. The handler re-arms only after a retry verdict, a bot-rights refusal or a Backend error before any claim; never after a wrong user, unknown token, failed solve, expiry or pass.
- `Backend` gained `BotCanRestrict`, `Fail` and `Expire` with the "an error means nothing changed" contract. `captchaWebBackend.Fail` reuses `handleCaptchaTimeout`; `Expire` reuses `expireCaptchaAttempt`; `BotCanRestrict` reads `getChatMember` for the bot live (creator, or administrator with `can_restrict_members`).
- `Handler.Verify` was split into `challengeRun` steps (preflight, attempt, siteverify, pass/fail/expire/retry), which keeps each function small.
- 20-iteration races (single replica, and timer on A against a check on B, with a staggered start so both outcomes occur: observed 16-17 passes and 3-4 expiries per run) pass under `-race` and stayed stable over repeated runs.
- `TestCaptchaWebNeverLogsInputs` covers 26 scenarios; a deliberate mutation (logging the Turnstile token) made it fail, then was reverted.
- AGENTS.md: outcome, re-arm, preflight and exactly-once rules; the `alita:captcha:ch:<token>` bullet describes `Rearm`.

## Task Commits

1. **Task 1** - `eed6bf0` (test, RED: the package did not build), `2f2cdec` (feat)
2. **Task 2** - `a974f9e` (test), `6a2a1a3` (docs: AGENTS.md exactly-once rules)
3. **Task 3** - `8404ef9` (test)

**Plan metadata:** the commit that adds this SUMMARY.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Plan wrong about existing code] A kick is one `unbanChatMember`, not ban then unban**
- **Found during:** Task 1 (test design); already recorded in the 08-01 summary
- **Issue:** The plan describes the kick as `banChatMember` then `unbanChatMember`. `kickMember` is a single `UnbanChatMember` with `only_if_banned=false`.
- **Fix:** The tests assert exactly one member-removing unban (`kicksOf`) and no `banChatMember` for a kick, and one `banChatMember` with no unban for a ban.
- **Files modified:** `alita/modules/captcha_web_outcomes_test.go`

**2. [Rule 1 - Existing test contradicts the new behavior] Two 08-01 test expectations changed**
- **Found during:** Task 1 and the Captcha-wide run
- **Issue:** `TestCaptchaWebTracerJoinVerifyUnmute/a_token_Cloudflare_rejects_never_unmutes` expected `retry` for `invalid-input-response`, which D-28 now makes a failed solve; `TestCaptchaPassSharedByCallbackAndWeb` compared Telegram call sequences and the new preflight adds one `getChatMember`.
- **Fix:** The first now expects `failed` with action kick, the attempt gone and still no unmute; the second filters `getChatMember` like `getChat` and `answerCallbackQuery`. `captcha_web_test.go` is outside the plan's `files_modified`, but both edits are forced by the plan's own behavior.
- **Files modified:** `alita/modules/captcha_web_test.go`
- **Commit:** `eed6bf0`, `a974f9e`

**3. [Rule 1 - Hardening] The Client refuses redirects for any transport**
- **Found during:** Task 1
- **Issue:** The 302 test with `SiteverifyServerForTest` (whose `*http.Client` follows redirects) would send the secret to the redirect target; the no-redirect rule only covered the default transport.
- **Fix:** `httpClient()` copies the given client and sets `CheckRedirect` to `ErrUseLastResponse`.
- **Files modified:** `alita/utils/captchaweb/siteverify.go`
- **Commit:** `2f2cdec`

**4. [Rule 3 - Blocking] Commit trailer and plan ledger**
- The plan names a different model in the `Co-Authored-By` line than the harness attribution; commits use the harness trailer. The sandbox refuses writes under the worktree's git directory (as in 08-01), so `commits` is measured with `git rev-list --count 20d4093...HEAD` against the dispatch base: 5.

### TDD notes (not deviations)

- Task 1's RED commit does not compile (`ReasonInvalidToken`, `ConfigError`, `Store.Rearm` and the new Backend methods do not exist yet), as in 08-01.
- Task 2's tests were green when first run: Task 1's Backend design already contained the expiry, lost-claim and unmute-failure branches. They are characterization tests, plus one real test bug found on the way (the expiry fixture violated `chk_captcha_expires_at`, so `created_at` is moved with `expires_at`; and the first race run was vacuous because the timer always won, so the timer now starts a staggered 0 to 11 ms late). There is no separate Task 2 `feat` commit because no production code was needed; the AGENTS.md rules are the `docs` commit.
- Task 3 needed no code change: no leak was found.
- `alita/modules/captcha.go` is unchanged (acceptance criterion `git diff --exit-code`).

**Total deviations:** 4 (3 Rule 1, 1 Rule 3). **Impact:** none on scope; two older test expectations were updated to the new contract.

## Issues Encountered

- `make lint` cannot run in this sandbox (golangci-lint is built with an older Go than the module). Used `gofmt -l alita` (clean apart from the pre-existing `alita/modules/greetings_command_test.go`) and `go vet -tags testtools ./alita/modules ./alita/utils/captchaweb` (clean).
- The sandbox refuses heredocs and compound commands, so files were written with the editor tools and git commands were run one at a time.

## Known Stubs

None.

## Threat Flags

None beyond the plan's threat model. T-08-23 to T-08-28 are covered by the tests listed under `coverage`.

## User Setup Required

None. No live Cloudflare or Telegram call was made; the fake Cloudflare is an `httptest` server. Pending human verification: none for this plan (real-device and real-Cloudflare checks stay in `08-SPIKE.md`).

## Verification Results

| Check | Result |
|-------|--------|
| `go test -tags testtools -race -count=1 -run '^(TestSiteverifyOutcome\|TestSiteverifyRequestShape\|TestVerifyRouteRearmsOnlyOnRetry)$' -v ./alita/utils/captchaweb` | PASS |
| `... -run '^(TestCaptchaWebFailedSolveAppliesAction\|TestCaptchaWebCloudflareErrorIsNotAFailure\|TestCaptchaWebTracerJoinVerifyUnmute)$' -v ./alita/modules` | PASS (3 + 5 + 7 subtests) |
| `... -run` the six Task 2 tests `-v ./alita/modules` | PASS; `-count=6` on the race and drain tests also ok, no `DATA RACE` |
| `... -run '^TestCaptchaWebNeverLogsInputs$' -v ./alita/utils/captchaweb` | PASS; fails when a token is added to a log line |
| `go test -tags testtools -race -count=1 -run 'Captcha' ./alita/modules ./alita/utils/captchaweb` | ok |
| `go test -tags testtools -race -count=1 ./alita/utils/captchaweb` (whole package) | ok |
| `CGO_ENABLED=0 go build ./...`, `go vet -tags testtools` on both packages, `gofmt -l` | clean |
| `node --test scripts/captcha-page/*.test.mjs` | pass |
| Acceptance greps (`timeout-or-duplicate`, `ConfigError`, `Rearm`, `BotCanRestrict(ctx context.Context, chatID int64) (bool, error)`, `handleCaptchaTimeout(`, `expireCaptchaAttempt(`, `captcha_muted_users` in AGENTS.md, no `Errorf(...%w` in siteverify.go) | as the plan states |
| `git diff --exit-code alita/modules/captcha.go go.mod go.sum` | clean |

The whole `./alita/modules` package was not run (the orchestrator runs `make test` between waves); only the `Captcha` pattern was.

## Next Phase Readiness

- Plan 08-08 can reuse `outcomeBackend` and `newOutcomeHandler`; the tests here stay inside its limits (one fresh env or handler per test, at most 21 verifies per race handler, at most 3 per token).
- Plan 08-09 owns `captcha.go`; nothing here needed a change there.
- Known accepted limit: a replica crash between the consume and a re-arm loses that member's challenge until the group's time limit (research Pitfall 9), recorded in AGENTS.md.

## Self-Check: PASSED

- Created files exist: `alita/utils/captchaweb/siteverify_test.go`, `alita/utils/captchaweb/verify_outcomes_test.go`, `alita/modules/captcha_web_outcomes_test.go`.
- Commits found: `eed6bf0`, `2f2cdec`, `a974f9e`, `6a2a1a3`, `8404ef9`.

---
*Phase: 08-turnstile-web-captcha*
*Completed: 2026-10-30*
