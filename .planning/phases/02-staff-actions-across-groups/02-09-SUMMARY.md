---
phase: 02-staff-actions-across-groups
plan: 09
subsystem: staff-moderation
status: complete
tags: [telegram, rate-limit, pacer, redis, lua, gap-closure]

requires:
  - phase: 02-staff-actions-across-groups
    provides: "The fleet-wide TelegramPacer (alita:staff:pace:next / alita:staff:pace:block) and staffPaced (02-06), the error classifiers that map ErrRateLimited to staff_act_fail_rate_limited (02-06, 02-07), the staffActionEnv harness"
provides:
  - "A paced call whose slot is more than MaxWait away fails at once with an error wrapping ratelimit.ErrRateLimited: no sleep, no Telegram request, no slot taken (Gap 2 pacer half, WR-02, T-02-36)"
  - "reserveSlotScript takes the caller's max wait (ARGV[3]) and returns before its SET when the slot is beyond it, so refused callers leave alita:staff:pace:next untouched (T-02-37)"
  - "reserveLocal (Redis-down fallback) refuses an over-cap slot the same way and does not move localNext"
  - "retry_after is clamped to pacerMaxRetryAfterSeconds (86400) before it is multiplied by RetryAfterUnit, so math.MaxInt64 gives a positive bounded block (T-02-38)"
affects: [02-10-target-lock-heartbeat, phase-03-audit-undo]

actuals:
  tokens: 9000
  tasks: 2
  commits: 4

plan_head_before: 12eb21ab3ba85ba462100cf534fbee12125bf79d
plan_head_after: d40521178631691dcfe118caae8025f7ebeb813d
commits: 4

tech-stack:
  added: []
  patterns:
    - "Refuse, do not sleep: any wait beyond the cap is an immediate ErrRateLimited failure, so a long shared block turns into fast per-group 'rate limited' lines instead of a frozen fleet"
    - "A refused reservation is side-effect free in both the Redis script and the local fallback"

key-files:
  created:
    - alita/modules/staff_action_flood_test.go
  modified:
    - alita/utils/ratelimit/telegram_pacer.go
    - alita/utils/ratelimit/telegram_pacer_test.go

key-decisions:
  - "The shared block is not clamped to MaxWait. The full retry_after is still recorded (research Q2, plan 02-06, T-02-27) so no replica sends into a flood Telegram asked it to wait out; wait now turns the block into a fast refusal. TestTelegramPacerSharedBlock and TestTelegramPacerRetryAfterCap pass unchanged."
  - "A refused reservation takes no slot (Lua returns before SET, reserveLocal returns before moving localNext), otherwise refused callers would push the next slot and keep refusing callers after the block ended."
  - "retry_after clamp is one day (86400 s): Telegram never sends more, the clamp only prevents Duration overflow."

requirements-completed: [STAFF-12, PLAT-01]

duration: 15min
completed: 2026-10-05
---

# Phase 2 Plan 09: Pacer MaxWait refusal Summary

A Telegram block longer than the pacer's 60 s cap now fails every paced staff call at once as "rate limited", with no request sent and no slot taken, instead of freezing every run across the fleet.

## Performance

- **Tasks:** 2 of 2 (Task 1 tracer, Task 2 auto, both TDD)
- **Files:** 1 new test file, 2 modified
- **Commits:** 4 (RED, GREEN, RED, GREEN)

## Accomplishments

- Tracer: `TestStaffActionBlockAboveMaxWaitFailsFast` sets `alita:staff:pace:block` for 2 minutes against a 60 s MaxWait. A staff `/ban` over 2 linked groups ends in about 0.3 s with both groups "failed: rate limited"; no `getChatAdministrators`, `getChatMember` or write request reaches either group. After the block key is deleted the next `/ban 4242` bans in both groups, which proves refused callers did not push `alita:staff:pace:next`.
- `wait` returns a wrapped `ErrRateLimited` when the reserved slot is more than MaxWait away. A slot exactly at MaxWait is still waited (boundary pinned).
- `reserveSlotScript` gains `ARGV[3]` (MaxWait in ms) and returns `slot - now` without the SET when the slot is beyond it. Subtest "a refused caller takes no slot" checks NextKey value and TTL are unchanged.
- `reserveLocal` refuses over-cap slots without moving `localNext`. `TestTelegramPacerLocalRefusesAboveMaxWait` shows a refused call does not stop the next call once the local block is within the cap.
- `retry_after` clamped to 86400 before the multiplication; `math.MaxInt64` fails the call after exactly 1 invocation with a positive block of at most 86400 x RetryAfterUnit.
- The package stays a leaf: no import from `alita/modules`.

## Task Commits

| Task | Commit | Type |
|------|--------|------|
| 1 RED | 9ac135e | test(02-09): show a long shared block freezes staff runs |
| 1 GREEN | fb7572c | fix(02-09): fail a paced staff call fast when its slot is beyond the wait cap instead of freezing the fleet |
| 2 RED | 3569dce | test(02-09): pin the pacer's wait-cap boundary, local fallback and retry_after overflow |
| 2 GREEN | d405211 | fix(02-09): refuse over-cap slots in the local pacer fallback and clamp retry_after before it can overflow |

## TDD evidence

- Task 1 RED: `TestStaffActionBlockAboveMaxWaitFailsFast` alone failed with "staff action runs did not finish within 10s" (the run slept through the 2 minute block). GREEN: passes in 0.29 s.
- Task 2 RED: `TestTelegramPacerLocalRefusesAboveMaxWait` failed ("Do() #3 error = ... next slot is 69ms away, over the 50ms cap" because the refused Do #2 had moved the local next slot) and `TestTelegramPacerRetryAfterOverflow` failed ("invocations = 4, want exactly 1"). The three Redis-path subtests of `TestTelegramPacerRefusesSlotAboveMaxWait` already passed after Task 1, as the plan expected. GREEN: all pass, `-count=3` clean.

## Verification

- `go test -tags testtools -race -count=3 -run '^TestTelegramPacer' ./alita/utils/ratelimit`: pass
- `go test -tags testtools -race -count=1 ./alita/utils/ratelimit`: pass
- `go test -tags testtools -race -count=1 -run 'Staff|staff' ./alita/modules`: pass (109 s)
- `go vet -tags testtools ./alita/utils/ratelimit ./alita/modules` and `CGO_ENABLED=0 go build ./...`: clean
- `git diff --exit-code go.mod go.sum`: unchanged
- `make lint` could not run in this sandbox (golangci-lint built with go1.25, module targets go1.26.0). `gofmt -l` on the touched files prints nothing and `go vet` is clean instead. `make test` (full suite) was not run; the ratelimit package and every Staff/staff test in `alita/modules` were.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Task 1's comment edit at `p.block(d)` did not apply**
- **Found during:** Task 2 (the overflow test showed the clamp edit was also missing from `Do`)
- **Issue:** A scripted replace for the `p.block(d)` comment and the Task 2 clamp used the wrong indentation, so both silently did not apply in Task 1's GREEN commit. The Task 1 behavior (refusal in `wait`, Lua ARGV[3]) was unaffected.
- **Fix:** Added the "full retry_after is recorded on purpose" comment together with the clamp in the Task 2 GREEN commit.
- **Files modified:** alita/utils/ratelimit/telegram_pacer.go
- **Commit:** d405211

Otherwise, the plan executed as written.

## Known Stubs

None.

## Threat Flags

None. No new network endpoints, auth paths or schema changes; the change only narrows when the pacer sleeps.

## Notes for plan 02-10

- AGENTS.md wording for the pacer refusal (a slot beyond MaxWait fails the call with ErrRateLimited, no request, no slot taken, retry_after clamped to one day) is left to plan 02-10, as the plan states. AGENTS.md was not touched here.

## Self-Check: PASSED

- FOUND: alita/modules/staff_action_flood_test.go, alita/utils/ratelimit/telegram_pacer.go, alita/utils/ratelimit/telegram_pacer_test.go
- FOUND commits: 9ac135e, fb7572c, 3569dce, d405211
- Acceptance greps: `if delay > p.opts.MaxWait`, `ARGV[3]`, `p.opts.MaxWait.Milliseconds()`, `const pacerMaxRetryAfterSeconds = 86400`, `min(seconds, pacerMaxRetryAfterSeconds)` all match; no `alita/modules` import in telegram_pacer.go
