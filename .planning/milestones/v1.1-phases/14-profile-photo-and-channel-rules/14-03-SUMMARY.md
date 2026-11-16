---
phase: 14-profile-photo-and-channel-rules
plan: 03
subsystem: member-requirements
tags: [lookup, singleflight, pacer, photo, redis, member-requirements]

requires:
  - phase: 14-profile-photo-and-channel-rules
    provides: plans 14-01 (storage) and 14-02 (locale strings)
provides:
  - memberReqVerdict (zero value memberReqUnknown, then Pass, Breach, Inert) and memberReqLookupMode{Fresh}
  - the lookup pacer (memberReqLookupPacerOptions, memberReqLookupPacer, memberReqLookupPaced) on its own Redis keys
  - memberReqFlight, a singleflight per key with internal panic recovery and a flight context derived from memberReqContext()
  - the positive-only cache helpers (memberReqCacheHas, memberReqCacheSet, memberReqCacheDel) and memberReqPhotoKey
  - memberReqPhotoRule, the profile photo lookup
  - the test fake (mreqLookupFake over a shared mreqLookupState), withFastLookupPacer and mreqLookupCleanup
affects: [14-04, 14-05, 14-06, 14-08, 14-09, 14-11]

plan_head_before: a4308f363df01eef8f7019541e8f2a9a382ece70
plan_head_after: 76b46f52f2e95d8766f0433b5406745410118b38
actuals:
  tokens: 11600
  tasks: 3
  commits: 5

tech-stack:
  added: []
  patterns:
    - "A DoChan flight function recovers its own panics first (the library re-raises a leader's panic in a goroutine nothing can recover) and returns the zero verdict"
    - "A shared lookup runs on a context derived from the shutdown root, never from a caller; every caller selects on its own deadline"

key-files:
  created:
    - fuku/modules/memberreq_lookup.go
    - fuku/modules/memberreq_lookup_fake_test.go
    - fuku/modules/memberreq_lookup_test.go
  modified:
    - AGENTS.md

key-decisions:
  - "memberReqLookupLogFailure logs 'rate limited' for a ratelimit.ErrRateLimited instead of memberReqErrText, because ErrRateLimited's text carries Telegram's description (D-18)"
  - "The Fresh stale-positive delete runs inside the flight keyed on the flight leader's mode; a Fresh caller that joins a non-Fresh leader's flight misses the delete, which is harmless because a non-Fresh leader only starts a flight when the key was absent"
  - "The two-replica pacer tests drive two ratelimit.TelegramPacer values built from memberReqLookupPacerOptions through two bots over one shared state, because the lookup pacer is one package variable"

requirements-completed: []

coverage:
  - id: D1
    description: "the verdict zero value is unknown and every failure shape of the photo lookup is unknown, never a breach, never cached (T-14-08, T-14-11)"
    requirement: "MREQ-10, MREQ-11 (contribution only)"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestMemberReqVerdictZeroValue|TestMemberReqPhotoRule)$' ./fuku/modules"
        status: pass
  - id: D2
    description: "one request for 100 callers, a panicking flight function contained, a caller deadline never cancels a shared lookup, shutdown ends a hung lookup (T-14-09)"
    requirement: "PLAT-06 (contribution only)"
    verification:
      - kind: unit
        ref: "TestMemberReqLookupSingleflight, -race -count=20"
        status: pass
  - id: D3
    description: "a separate fleet-wide lookup pacer on its own keys that sheds a far slot with no request, and the no-Redis fallback (T-14-10)"
    requirement: "PLAT-06 (contribution only)"
    verification:
      - kind: unit
        ref: "TestMemberReqLookupPacer (-race -count=20), TestMemberReqLookupRedisDown"
        status: pass

duration: 50min
completed: 2026-11-08
status: complete
---

# Phase 14 Plan 03: The lookup layer and the photo lookup Summary

**A paced, coalesced, positive-cached profile photo lookup whose verdict zero value is "unknown", with a flight that recovers its own panics and ends at shutdown, and a lookup pacer that shares nothing with Phase 13's.**

## Accomplishments

- `fuku/modules/memberreq_lookup.go`: `memberReqVerdict` (`memberReqUnknown` is the iota zero), `memberReqLookupMode{Fresh}`, the pacer options variable (`fuku:memberreq:lkp:pace:next` and `:block`, Interval 100 ms, MaxRetries 1, MaxWait 5 s) and `memberReqLookupPaced` (each call bounded by `memberReqCallTimeout`), the cache helpers on `memberReqRedisClient()` (250 ms each; a nil client or an error is a miss on read and a silent no-op on write), `memberReqFlight` and `memberReqPhotoRule`.
- `memberReqFlight` returns unknown at once for a done context, runs `DoChan` with a function whose first deferred statement is `error_handling.RecoverFromPanic` and whose body derives its context from `memberReqContext()` bounded by `memberReqLookupFlightTimeout`, and waits with `select` on the result or the caller's `ctx.Done()`. It adds one goroutine per distinct key in flight, no pool and no drain.
- `memberReqPhotoRule`: cache read unless Fresh, then flight `p:<user>` around `getUserProfilePhotos(user, limit 1)` through the lookup pacer. Total above zero or a non-empty list is a pass cached 30 minutes; total 0 with no photos is a breach, never cached; an error, 400, 403, 429, 500, a timeout, a shed, a cancelled context and a null body are unknown. Fresh writes a pass and deletes a stale positive key on a definite breach.
- AGENTS.md: three Data bullets (`fuku:memberreq:photo:<user>`, the two lookup pacer keys, the flights), one sentence in the member requirements shutdown bullet (lookups derive from `memberReqContext()`, Redis and Telegram only, no new drain, the 65 s worst case unchanged), one Go rules bullet (a `DoChan` leader that panics crashes the process). No channel key is described yet (plan 14-04).

## Task Commits

1. `f702e59` test(14-03): the photo lookup layer (RED: the package's tests do not compile without the layer; the production file was set aside, not deleted, while the test commit was made)
2. `9acc438` feat(14-03): look up a member's profile photo through the lookup pacer (GREEN)
3. `be19592` feat(14-03): coalesce lookups per key and end them at shutdown (tests of task 2, see deviations)
4. `f8421a7` test(14-03): check the miniredis write errors of the lookup tests (errcheck findings of the lint stand-in)
5. `76b46f5` docs(14-03): describe the lookup pacer, the photo cache and the flight rule

## Verification (real results)

- RED confirmed: with the production file absent `go vet -tags testtools ./fuku/modules` failed on `undefined: memberReqLookupMode`; the test commit was made in that state, the tests and their commit were separate commands.
- Task 1 GREEN: `go test -tags testtools -race -count=1 -run '^(TestMemberReqVerdictZeroValue|TestMemberReqPhotoRule)$' -v ./fuku/modules` passes (15 subtests).
- Task 2: `go test -tags testtools -race -count=1 -run '^(TestMemberReqLookupSingleflight|TestMemberReqLookupPacer|TestMemberReqLookupRedisDown)$' -v ./fuku/modules` passes. `-race -count=20` over the same three tests: `ok ... 94.518s`, no failure in any run.
- Mutation checks on the production file (restored afterwards, `git diff` showed only test files): without the internal recovery the panic subtest kills the test binary (`panic: scripted lookup panic`, FAIL); with `context.Background()` instead of `memberReqContext()` the shutdown subtest fails (`no verdict within 2s`).
- Phase 13 regression run: `go test -tags testtools -race -count=1 -run '^(TestMemberReqRunnerSheds|TestShutdownWaitsForMemberReqJobs|TestMemberReqWatcherContinuesGroups)$' ./fuku/modules`: ok (SQLite).
- `CGO_ENABLED=0 go build ./...`: ok. `gofmt -l` on the three Go files: nothing. `go vet -tags testtools ./fuku/modules`: clean.
- Lint stand-in `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827`: first run exit 1 on three errcheck findings (discarded miniredis `Set` errors in the new test), fixed with `mreqSetKey` and committed; second run exit 0 read directly ("no dupl, gocyclo or godox findings on lines added since 8c0c827"). `make lint` and gosec cannot run in this sandbox.
- `git diff --exit-code 8c0c827 -- go.mod go.sum`: clean. Acceptance greps: `func memberReqPhotoRule(` 1, `fuku:memberreq:lkp:pace:next` in the Go file 1, `DoChan` 2, `memberReqContext()` 2, `ErrRateLimited` in the test 2, `fuku:memberreq:photo` in AGENTS.md 1, `DoChan` in AGENTS.md 2.
- Not run: `make test` (the gate plan 14-12 runs it), the full settings family (this plan changes no settings code), PostgreSQL (the layer reads and writes Redis only).

## Deviations from Plan

**1. [Process] Task 2 had no RED commit.** The flight, pacer and cache code of Task 1 already honoured every Task 2 behaviour, so the Task 2 tests passed on first run and one commit holds them (`be19592`, subject as the plan names it though it adds only tests). The mutation checks above stand in for the red state and show the tests do detect a missing recovery and a caller-derived context.

**2. [Rule 1 - lint] errcheck on the tests.** Three discarded miniredis `Set` errors, fixed in `f8421a7` (a separate test commit, not part of the docs commit).

**3. [Judgment] Failure log text.** `memberReqErrText` returns `err.Error()` for a non-Telegram error, and `ratelimit.ErrRateLimited` wraps Telegram's description with `%v`, so `memberReqLookupLogFailure` logs the fixed text "rate limited" for it (D-18, T-14-12).

## Known Stubs

None in the sense of placeholder data. **Staged symbols with no production caller yet** (plan 14-05 puts the rule in the table, plan 14-06 folds it into the executor, 14-04 adds the channel lookups): `memberReqVerdict` and its four values (`memberReqInert` is used only by `TestMemberReqVerdictZeroValue`), `memberReqLookupMode`, `memberReqLookupPacerOptions`, `memberReqLookupPacer`, `memberReqLookupPaced`, `memberReqPhotoKey`, `memberReqCacheHas`, `memberReqCacheSet`, `memberReqCacheDel`, `memberReqFlight` and `memberReqPhotoRule`. Each is called by a test of this plan (`TestMemberReqPhotoRule`, `TestMemberReqLookupSingleflight`, `TestMemberReqLookupPacer`, `TestMemberReqLookupRedisDown`, `TestMemberReqVerdictZeroValue`), and the lint stand-in's unused check passes. `memberReqCacheDel` is reached only by the Fresh-breach path, which `TestMemberReqPhotoRule` drives.

## Known limits

- A caller that reads a cache miss and enters `DoChan` after another flight has finished and deregistered makes a second request; the cache write inside the flight closes most of that window, and the 100-caller test holds the call open so it does not depend on the race.
- The tests that measure pacer spacing (50 ms and 40 ms intervals, gaps of at least 25 ms) rely on timers under `-race`; they were stable over 20 runs.

## Threat Flags

None. The new surface is Redis keys `fuku:memberreq:photo:<user>` and `fuku:memberreq:lkp:pace:*` (documented), and one Telegram method (`getUserProfilePhotos`) that sends only the user ID. The photo response is never stored or logged (T-14-12; the log-content test is plan 14-11's `TestMemberReqLookupLogsCarryIDsOnly`).

## Self-Check: PASSED

- Files exist: `fuku/modules/memberreq_lookup.go`, `fuku/modules/memberreq_lookup_fake_test.go`, `fuku/modules/memberreq_lookup_test.go`, `AGENTS.md` change.
- Commits `f702e59`, `9acc438`, `be19592`, `f8421a7` and `76b46f5` are ancestors of HEAD; `git rev-list --count a4308f3..HEAD` was 5 before this SUMMARY.
