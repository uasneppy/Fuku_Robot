---
phase: 02-staff-actions-across-groups
plan: 07
subsystem: staff-moderation
status: complete
tags: [telegram, summary, rate-limit, shutdown, i18n, utf16]

requires:
  - phase: 02-staff-actions-across-groups
    provides: "staffActionHeader / staffResultLine / renderStaffActionSummary (02-01..02-05), the paced bounded fan-out and the pending sweep (02-06), card lifecycle with the per-target lock release (02-05), staffActionEnv harness and the stateful staffActionFake"
provides:
  - "Summary renderer with a tally line, fixed link order, done-line collapse, pending-line collapse in progress and continuation messages for overflow (STAFF-08, D-16..D-18)"
  - "A single coordinator goroutine per card that edits at most every staffActionEditEvery (2.5 s) and only when a result changed"
  - "deliverStaffActionFinal: final edit retried on 429 retry_after (up to 3 attempts), not-modified is success, new-message fallback, continuation messages in order (STAFF-08, STAFF-12)"
  - "StopStaffActions shutdown drain registered after the database-close handler: cancels fan-outs, marks unfinished groups 'failed: interrupted by restart', delivers the final summary, bounded by 30 s"
affects: [phase-03-audit-undo]

actuals:
  tokens: 11500
  tasks: 2
  commits: 4

plan_head_before: bc32be6f583ed3be3cdc4d6d4ea7c10ee0227e85
plan_head_after: 047e34b13b9296d4a6149b4473646ba73e6c3e30
commits: 4

tech-stack:
  added: []
  patterns:
    - "Whole-line packing under a UTF-16 cap: lines are built from escaped pieces and never sliced; a fit helper adds lines until the next would not fit"
    - "Only the lines that carry no failure information may be summarized: done lines collapse into a count, skipped and failed are always listed or moved to a continuation message"
    - "Coordinator owns the card: workers write results into a mutex-guarded progress struct with a dirty flag; one goroutine ticks, snapshots and edits"
    - "Final delivery on a fresh timeout context, never the run's own (possibly cancelled) one"

key-files:
  created:
    - alita/modules/staff_action_summary_test.go
    - alita/modules/staff_action_progress_test.go
  modified:
    - alita/modules/staff_action_summary.go
    - alita/modules/staff_action_run.go
    - alita/modules/staff_action_decide.go
    - main.go
    - AGENTS.md
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml

key-decisions:
  - "deliverStaffActionFinal takes a context as its first parameter (the plan listed no context). The coordinator passes the fresh 15 s delivery context the plan asks for, and the retry waits select on it, so a long retry_after cannot stall a shutdown; when the context ends the summary falls back to a new message"
  - "Continuation messages and the new-message fallback go through sendStaffSummaryPart, which also waits out a 429 (up to 3 attempts), because 11 or more continuation messages in one chat can themselves be rate limited"
  - "The pending sweep moved from runStaffActionFanOut to the coordinator, because only the coordinator knows whether the base context was cancelled (interrupted by restart) or a worker panicked (internal error)"
  - "Group lines in continuation messages carry the header and the continuation marker but not the tally; each non-final part ends with the continued marker"
  - "StopStaffActions reads the cancel func under staffActionsMu and waits without holding it, the staff_sweeper.go pattern; the run does Add(1) before reading the context so a shutdown never misses it"

patterns-established:
  - "withStaffActionTimers(t, editEvery, retryUnit) restores the three timing vars; env.startRun and env.addTitledGroups are the progress-test helpers"

requirements-completed: [STAFF-08, STAFF-12]

duration: 50min
completed: 2026-10-05
---

# Phase 2 Plan 07: Summary That Never Lies by Omission

**One Staff Group message fills in at a safe edit rate with a tally and fixed lines, collapses only done lines when long, spills skipped and failed lines into continuation messages, and still delivers when Telegram rate limits the edit, the card is gone or the bot shuts down.**

## Performance

- **Duration:** about 50 min
- **Tasks:** 2 (both TDD, RED then GREEN each)
- **Files modified:** 14 (2 new test files)

## Accomplishments

- The summary now shows a tally line (`✅ d · ⏭ s · ❌ f`, non-localized) under the header. Group lines keep their link order and only change icon.
- Length is counted in UTF-16 code units on the final escaped HTML. Over 3800 units, done lines collapse into one `✅ N groups done` line (D-18). While running, pending lines collapse too. Skipped and failed lines are never summarized. If they alone overflow, the first message ends with the continued marker and the rest go out as continuation messages that each start with the header and the continuation marker, so every group appears exactly once (STAFF-08).
- The run's goroutine became the coordinator: workers write into `staffActionProgress`, the coordinator ticks every `staffActionEditEvery`, edits only when something changed, then sweeps any group that never reported (to "interrupted by restart" when the base context was cancelled, otherwise "internal error") and delivers the final summary.
- The final edit is retried after `retry_after` (3 attempts, each wait capped at 60 s), "message is not modified" counts as success, and a card that cannot be edited gets the summary as a new message in the Staff Group (STAFF-12).
- `StopStaffActions` cancels running fan-outs, workers stop between steps and report `staffReasonFailInterrupted`, the final summaries are delivered and the call returns within `staffActionStopWait` (30 s). `main.go` registers it right after the staff sweeper handler, which is after the DB-close handler, so LIFO runs it before the database closes.
- Five locale keys added to all 7 files; AGENTS.md has the shutdown drain rule and the single-writer / no-hidden-group rule.

## Task Commits

1. **Task 1 RED:** `e0cdc73` test(02-07): add failing tests for the summary tally, collapse and continuation
2. **Task 1 GREEN:** `34640ef` feat(02-07): show a tally and collapse long staff summaries without hiding a group
3. **Task 2 RED:** `67d676b` test(02-07): add failing tests for batched edits, final delivery and the shutdown drain
4. **Task 2 GREEN:** `047e34b` feat(02-07): batch staff card edits, always deliver the final summary and drain on shutdown

## TDD Gate Compliance

Both tasks have a `test(02-07)` commit before their `feat(02-07)` commit. RED commits carry compile-only stubs (the renderer functions, `staffReasonFailInterrupted`, the timing vars, an empty `StopStaffActions`) so the target tests fail on assertions, not on compilation. Observed RED failures: tally/collapse/overflow/escape tests failed on missing tally and missing collapse; ProgressBatched, FinalEditRetriesOn429, OverflowContinuation and StopStaffActionsFinalizes failed on their assertions. Four tests passed already at RED because the earlier plans already satisfied them (FixedOrder, FinalFallsBackToNewMessage, NotModifiedIsSuccess, AllSkippedTally); they are regression guards for behavior this plan must keep. No refactor commit was needed.

## Verification

- `go test -tags testtools -race -count=1 -run '^TestStaffActionSummary' ./alita/modules`: pass
- The six progress tests plus `TestStopStaffActionsFinalizes`: pass, and pass 4 times in a row with `-count=4`
- `go test -tags testtools -race -count=1 ./alita/modules` (full package, about 121 s): pass on the last run (see Deferred Issues for a pre-existing flaky race seen on one earlier run)
- `./alita/utils/extraction`, `./alita/db/user`, `./alita/i18n` (including `TestStaffLocaleKeys`): pass
- `make check-translations`, `make check-docs`: pass
- `CGO_ENABLED=0 go build ./...`, `go vet -tags testtools ./alita/... .`: clean
- `gofmt -l alita main.go`: only `alita/modules/greetings_command_test.go`, which is not gofmt-clean on the base commit (pre-existing, left alone)
- `--version` smoke check: built the binary to the scratchpad and ran it from `/tmp`, printed `v2.23.14`
- `make lint` was NOT run: golangci-lint in this sandbox is built with go1.25 against module go1.26.0. `gofmt` and `go vet` stood in for it.
- `make test` (full suite with coverage and skip gate) was not run; the targeted package runs above were.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] deliverStaffActionFinal takes a context**
- **Found during:** Task 2
- **Issue:** The plan gives the signature without a context, but also requires delivery under a fresh 15 s timeout context. Without a context parameter that timeout has nothing to bound.
- **Fix:** `deliverStaffActionFinal(ctx, b, chatID, msgID, text, continuation)`. The ctx only bounds the waits between retries; on expiry the summary falls back to a new message.
- **Files modified:** alita/modules/staff_action_summary.go, alita/modules/staff_action_run.go
- **Commit:** 047e34b

**2. [Rule 2 - Missing critical functionality] Continuation and fallback messages wait out a 429**
- **Found during:** Task 2
- **Issue:** A pathological list produces many continuation messages in one chat; one 429 would silently drop a group list, contradicting STAFF-08.
- **Fix:** `sendStaffSummaryPart` retries up to 3 times after `retry_after`; a part that still cannot be sent is logged at error level.
- **Commit:** 047e34b

**3. [Rule 1 - Test strictness] ProgressBatched asserts on group lines, not on the tally**
- The plan's check "an intermediate edit contains both ✅ and ⏳" is trivially true because the tally line always contains ✅. The test looks for a line starting `✅ Group ` and a line starting `⏳ Group ` in the same intermediate edit.

Otherwise the plan was executed as written.

## Pending Human Verification (not faked)

The plan's `<human-check>` needs a real bot, real groups and real Telegram clients, which this sandbox does not have. It is NOT done. Exact steps:

1. Link two or three test groups to a test Staff Group and be a restrict-admin in all but one.
2. From the Staff Group run `/ban @testaccount 1d spam test` and press Confirm. Expect: the card shows name, ID, 1 day, reason and group count; after Confirm one message fills in with a tally and every group marked; the group where you are not an admin reads "skipped: you're not an admin there"; the Staff Group itself is untouched.
3. Run `/mute` with the test account's numeric ID and Confirm, then `/unmute` and `/unban` with the same ID. Expect each to apply only where allowed.
4. Run `/ban` as an anonymous admin. Expect "post as yourself".
5. Let one card sit for 5 minutes. Expect it to turn into "Expired".
6. Run `/ban` in a normal, unlinked group. Expect it to behave exactly as before.
7. Optional, covers the new drain: start a ban across several groups and stop the bot mid-run. Expect the card to end with the unfinished groups reading "failed: interrupted by restart" and no ⏳ line.

## Deferred Issues

Both are pre-existing and out of scope; neither touches this plan's code.

- `alita/modules` `TestLogUsersPersistsSenderChatAndReplyUsers` showed a data race once in three full-package `-race` runs (the test reassigns `userUpdateCache`, `chatUpdateCache` and `channelUpdateCache` while the one-minute `usersThrottleSweepLoop` goroutine in `users.go` reads them). It passes alone and passed on the next full run with identical code.
- `alita/utils/ratelimit` `TestTelegramPacerFleetSpacing` failed once with a gap of 34.9 ms against a 35 ms bound (timing flake, package not touched).

## Known Stubs

None. No placeholder values flow to the UI.

## Threat Flags

None. No new network endpoint, auth path, file access or schema surface. T-02-28 to T-02-31 are mitigated as planned: collapse/overflow tests, batched single-writer edits with 429 retry and new-message fallback, the shutdown drain test, and the coordinator sweep alongside `TestStaffActionWorkerPanicReported`.

## Self-Check: PASSED

- FOUND: alita/modules/staff_action_summary_test.go, alita/modules/staff_action_progress_test.go
- FOUND commits: e0cdc73, 34640ef, 67d676b, 047e34b
- Acceptance greps: `func renderStaffActionSummaryFinal`, `utf16.Encode`, `staffReasonFailInterrupted`, `func StopStaffActions()`, `modules.StopStaffActions()` after `return closeDBConnections()` in main.go, `RecoverFromPanic("staffActionFanOut", "StaffActions")`, `StopStaffActions` in AGENTS.md all match.
