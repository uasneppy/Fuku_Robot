---
phase: 09-settings-menu
plan: 06
subsystem: captcha
tags: [captcha, settings-menu, refactor, turnstile]
requires:
  - phase: 09-settings-menu
    provides: "captchaApplyResult / applyCaptchaTimeout pattern (09-01)"
provides:
  - "applyCaptchaEnabled, applyCaptchaMode, applyCaptchaAction apply functions"
  - "captchaWebInstalled no-I/O Turnstile precondition"
  - "captchaRefused and captchaCleanupFailed outcomes"
affects: [09-08 Captcha settings page]
tech-stack:
  added: []
  patterns: ["one apply function per setting, command maps the typed result to its unchanged reply"]
key-files:
  created:
    - alita/modules/captcha_apply_test.go
  modified:
    - alita/modules/captcha.go
key-decisions:
  - "The shared read-back helper captchaReadBack uses context.Background(), because the plan's signatures for the three apply functions carry no context"
  - "No Settings read-back after a failed cleanup (captchaCleanupFailed): the command never needed it"
patterns-established:
  - "captchaEnabledReply keeps /captcha on|off replies in one place; captchaModeCommand and captchaActionCommand switch on result.Outcome"
requirements-completed: [MENU-07, MENU-08]
status: complete
duration: 25min
completed: 2026-10-30
commits: 3
plan_head_before: e595cb83fa4a76a0e7ca611cac47c16bb11933c3
plan_head_after: f1cb094f4171a54fc773b9b400a3199636197334
actuals:
  tokens: 9000
  tasks: 2
  commits: 3
---

# Phase 9 Plan 06: Captcha apply functions Summary

**Captcha on/off, mode and failure action each have one apply function in `captcha.go`, and `/captcha`, `/captchamode`, `/captchaaction` call them with every reply and returned error unchanged; Turnstile keeps its three ordered refusals inside `applyCaptchaMode`.**

## Accomplishments
- `applyCaptchaEnabled(b, chatID, on)`: stores the switch; off also runs `disableCaptchaForChat`. A failed write is `captchaSaveFailed` (`captcha_enable_failed` / `captcha_disable_failed`), a failed cleanup is `captchaCleanupFailed` (`captcha_disable_failed`) with the switch already saved.
- `applyCaptchaAction(chatID, action)`: lower-cases, accepts kick/ban/mute only (`captcha_action_invalid`), maps repository errors to `captcha_invalid_action_error` / `captcha_action_failed`.
- `applyCaptchaMode(b, chatID, mode)`: lower-cases, accepts math/text/turnstile (`captcha_mode_invalid`), turnstile goes through `captchaWebModeRefusal` (`captchaRefused` with its key, nothing written), repository errors map to `captcha_invalid_mode_error` / `captcha_mode_failed`.
- `captchaWebInstalled(bot)`: runtime, bot username and Redis client, no I/O; it is now the first step of `captchaWebModeRefusal`, so the page and the command cannot disagree.
- New tests: `TestApplyCaptchaEnabled`, `TestApplyCaptchaAction`, `TestApplyCaptchaMode`, `TestCaptchaWebInstalled` (real SQLite harness, `newCaptchaWebEnv`, miniredis, the existing Telegram fakes; a database with no tables stands in for a failing repository).

## Task Commits
1. RED tests for all three apply functions and the helper: `bfcce2b`
2. Task 1 (enabled, action): `eaf1674`
3. Task 2 (mode, captchaWebInstalled): `f1cb094`

## Verification
- `CGO_ENABLED=0 go build ./...` clean; `go vet -tags testtools ./alita/modules/` clean; `gofmt -l alita/` empty.
- All 15 tests named by the two verify blocks ran and passed (`-v`, `-race`); the pre-existing 11 are unedited. `go test -run Captcha ./alita/modules` also passes (24 s).
- `make lint` was not run: golangci-lint is built with go1.25 and cannot lint the go1.26 module in this sandbox. `gofmt` and `go vet` were run instead.

## Deviations from Plan

**1. [Process] One RED commit instead of two.** The plan asked for a RED commit per task. The tests for all three apply functions were written together and committed once, so the Task 1 commit (`eaf1674`) builds (`go build ./...`) but the test package only compiles from `f1cb094`, when `applyCaptchaMode` and `captchaWebInstalled` exist. The focused tests were run once, after Task 2, on the final tree.

**2. [Minor] Read-back context.** The apply functions take no context in the plan's signatures, so the read-back uses `context.Background()` (unlike `applyCaptchaTimeout`, which receives the update's tracing context).

Otherwise the plan was executed as written. No auth gates.

## Known Stubs
None.

## Threat Flags
None. T-09-22 (Turnstile refusal order) is covered by `TestApplyCaptchaMode` and the unchanged `TestCaptchaModeTurnstileRefusals`; T-09-23 (failed cleanup reported, switch saved) by `TestApplyCaptchaEnabled`.

## Pending human verification
None. AGENTS.md was not edited: the `/captchamode turnstile` bullet stays true.

## Self-Check: PASSED
- `alita/modules/captcha_apply_test.go` exists; `applyCaptchaEnabled`, `applyCaptchaAction`, `applyCaptchaMode`, `captchaWebInstalled` are defined in `alita/modules/captcha.go`.
- Commits `bfcce2b`, `eaf1674`, `f1cb094` exist on the branch.
