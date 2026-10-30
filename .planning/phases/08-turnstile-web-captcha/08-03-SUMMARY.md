---
phase: 08-turnstile-web-captcha
plan: 03
subsystem: captcha
tags: [turnstile, telegram-mini-app, initdata, hmac, security]
requires:
  - phase: 08-01
    provides: captchaweb package, ValidateInitData, SignInitDataForTest
provides:
  - strict ValidateInitData (size cap, lowercase 64-hex hash) with unchanged signature
  - known-answer test against two published Telegram vectors
  - leaf-package test that keeps captchaweb free of the database init
affects: [08-06]
tech-stack:
  added: []
  patterns: ["go/parser ImportsOnly leaf test with a named allow-list"]
key-files:
  created:
    - alita/utils/captchaweb/initdata_test.go
    - alita/utils/captchaweb/leaf_test.go
  modified:
    - alita/utils/captchaweb/initdata.go
key-decisions:
  - "Raw initData over 4096 bytes is refused before parsing"
  - "hash must be exactly 64 lowercase hex characters before it is decoded"
requirements-completed: [CAPT-03, CAPT-04]
status: complete
commits: 3
plan_head_before: f9560b78b4854f75cca428dbf59882850f236056
plan_head_after: fdfa318cafde2a18fed1c6ac5c487aa07153a9df
actuals:
  tokens: 3500
  tasks: 2
  commits: 3
duration: 12 min
completed: 2026-10-30
---

# Phase 8 Plan 3: initData hardening Summary

**Strict Telegram initData validation (size cap, lowercase 64-hex hash) pinned by both published known-answer vectors, plus a test that keeps `captchaweb` a database-free leaf.**

## Accomplishments

- `ValidateInitData` (same signature) now refuses a raw string over 4096 bytes and a hash that is not 64 lowercase hex characters, before the HMAC is compared. Plan 08-01 already had the HMAC with `hmac.Equal`, the 5 minute window, the 60 s future skew, repeated-key rejection and the user checks; this plan found and closed the two gaps (uppercase hex hash accepted, no size cap).
- `TestInitDataKnownAnswer` validates the init-data-golang vector (proves `signature` stays in the data-check string) and the Telegram-Mini-Apps docs vector (no signature), and checks the removed-signature, one-character-hash and swapped-token cases.
- `TestInitDataRejects` covers 19 rejections (changed, added, removed field, two hashes, repeated key, uppercase hash, 63-char hash, other bot token, 5m01s old, 61 s ahead, missing and non-numeric auth_date, bad user JSON, id 0 and -5, bot user, no user, 4097 bytes, empty) each returning `ErrInvalidInitData` and an empty `InitData`, plus the accepted boundaries (exactly 5 minutes old, exactly 60 s ahead).
- `TestCaptchaWebPackageIsLeaf` fails if any non-test file imports beyond the standard library, go-redis and logrus. Checked against a deliberate `alita/config` import (fails as expected, probe file removed).

## Task Commits

| Task | Commit | Message |
| ---- | ------ | ------- |
| 1 RED | e9ab177 | test(08-03): show initData accepts an uppercase hash and an oversized string |
| 1 GREEN | 2dff605 | fix(08-03): make the initData check strict |
| 2 | fdfa318 | test(08-03): keep the captchaweb package a leaf |

## Verification

- `go test -tags testtools -race -count=1 -run '^(TestInitDataKnownAnswer|TestInitDataRejects)$' -v ./alita/utils/captchaweb`: both PASS (RED run before the fix showed `hash_in_uppercase_hex` and `raw_string_of_4097_bytes` failing).
- `go test -tags testtools -race -count=1 ./alita/utils/captchaweb`: ok.
- `go test -tags testtools -race -count=1 -run '^TestCaptchaWeb' -v ./alita/modules`: `TestCaptchaWebTracerJoinVerifyUnmute`, `TestCaptchaWebAttemptRefusesCallbacks`, `TestCaptchaWebRejoinInvalidatesOldChallenge` PASS.
- `CGO_ENABLED=0 go build ./...`, `go vet -tags testtools ./alita/utils/captchaweb`, `gofmt -l alita/utils/captchaweb`: clean. `make lint` was not run (golangci-lint is built with go1.25 against module go1.26.0 in this sandbox); gofmt and go vet stand in for it.
- Acceptance greps: `hmac.Equal` 1, `ValidateInitData` signature 1, each vector hash 1 line in the test, `parser.ImportsOnly` 1, go-redis path in leaf test 1.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] initData accepted an uppercase hex hash and had no size cap**
- **Found during:** Task 1 (RED run)
- **Issue:** `hex.DecodeString` accepts uppercase, so a hash re-cased by an attacker or proxy still verified; no cap on the raw string.
- **Fix:** `isLowerHex64` check before decoding; `initDataMaxBytes = 4096` check at the top.
- **Files modified:** alita/utils/captchaweb/initdata.go
- **Commit:** 2dff605

### Process notes

- The RED commit message names the two cases that actually failed (uppercase hash, oversized string) instead of the plan's wording (repeated keys, future dates, bot users), because 08-01 already rejected those and the test passed for them from the start.
- Commit trailers use the harness-supplied attribution line (Claude Sonnet 5.5) rather than the "Opus 5.5" text in the plan.
- The per-plan commit ledger file under the worktree git dir could not be written from the sandbox, so `plan_head_before` is the base SHA given in the task prompt; `git rev-list --count f9560b78..HEAD` gave 3 (3 code commits, before this SUMMARY commit).

**Total deviations:** 1 auto-fixed (1 bug). **Impact:** closes two small gaps the plan expected to find; no scope change.

## Known Stubs

None.

## Threat Flags

None. Mitigations for T-08-11 to T-08-14 are in place and covered by the tests named above.

## Pending Human Verification

None for this plan.

## Self-Check: PASSED

- initdata_test.go, leaf_test.go, initdata.go present; commits e9ab177, 2dff605, fdfa318 present on the branch.
