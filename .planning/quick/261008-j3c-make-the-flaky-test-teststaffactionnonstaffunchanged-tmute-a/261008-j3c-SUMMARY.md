---
phase: quick-261008-j3c
plan: 01
subsystem: testing
tags: [flaky-test, staff-actions, go-test]
key-files:
  modified:
    - alita/modules/staff_action_dispatch_test.go
status: complete
commits: 1
plan_head_before: dab6c0b426af8b4d3e5bc19f1a4b2a2d36a2e411
plan_head_after: 077ee2e
actuals:
  tasks: 2
  commits: 1
---

# Quick 261008-j3c: Deterministic TestStaffActionNonStaffUnchanged

Test-only fix: `until_date` is now compared through each run's measured Unix-second window instead of as an absolute string, so a wall-clock second boundary between the two runs no longer fails the test.

## What changed

`alita/modules/staff_action_dispatch_test.go` only:

- New helper `comparableCalls` renders calls like `normalizedCalls` but prints `until=set` / `until=none` and returns the `until_date` values separately. `normalizedCalls` is unchanged (still used by a diagnostic elsewhere).
- New type `staffDispatchRun` (lines, until, from, to).
- `TestStaffActionNonStaffUnchanged`: five cases marked `timed: true` (tban an ID, tban with a reason, tban as a reply, tmute an ID, tmute as a reply). Each `ProcessUpdate` is bracketed by `time.Now().Unix()`. Method, user_id, until presence and reply text are still compared exactly for all 25 cases. For `until_date`, the delta between runs must lie in `[with.from - without.to, with.to - without.from]`. A guard fails a timed case whose baseline sent no `until_date`.
- No production change, no skip, no sleep in the committed file.

## Verification

- RED (before fix): forced-straddle overlay (wait for next whole second before the StaffActions run) failed 5 of 5 timed subtests with "Telegram calls differ with StaffActions loaded".
- Task 1 gate: same overlay passes after the fix. Normal run passes. 25 cases, 5 timed.
- `go test -tags testtools -race -count=20 -run '^TestStaffActionNonStaffUnchanged$' ./alita/modules`: ok.
- Forced straddle, `-race -count=3`: ok.
- Negative control (adds 120 s to the StaffActions run's until_date): exactly 5 subtests fail (tban an ID, tban with a reason, tban as a reply, tmute an ID, tmute as a reply), 5 "until_date of call" messages.
- `CGO_ENABLED=0 go build ./...`: ok. `go vet -tags testtools ./alita/modules`: ok. `gofmt -l`: clean.
- `go test -tags testtools -race -count=1 ./alita/modules`: ok (192 s).
- Scope gate: the only file changed versus the base commit is `alita/modules/staff_action_dispatch_test.go`; added lines contain no `Skip(` or `Sleep(`.

## Deviations from Plan

**1. [Not run] golangci-lint**
- `golangci-lint run ./alita/modules/...` could not run: the installed binary was built with Go 1.25 and the repo targets Go 1.26.0 ("can't load config"). This is an environment limitation, not a code finding. `go vet` and `gofmt` are clean. Recommend running `make lint` in CI.

**2. [Environment] Disk full during verification**
- The disk filled (81 MB free) and the first test link failed with "no space left on device". Freed space by deleting Go build cache entries not accessed for over a day (about 17 GB, regenerable). No repository files were touched.

## Known Stubs

None.

## Self-Check: PASSED

- alita/modules/staff_action_dispatch_test.go exists and is committed in 077ee2e.
