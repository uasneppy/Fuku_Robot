---
phase: 16-scheduled-deletion-of-service-and-bot-messages
plan: 04
subsystem: modules
tags: [deletion, worker, queue, pacer, latch, classifier, shutdown, tracer]

requires:
  - phase: 16-scheduled-deletion-of-service-and-bot-messages
    provides: "plan 16-01: queue (ClaimDue, Complete, Release, ReleaseStale, DropChatPending, NewClaimToken); plan 16-03: deletionPaced, deletionClassify, the scoped latch, deletionContext, deletionWake, deletionStaffLookup, the inline delete"
provides:
  - "StartDeletionWorker / StopDeletionWorker and runDeletionCycle (tick plus wake, recovered cycle, stale release, 200-row claim by token, per-chat chunks of 100, per-ID fallback, scoped latch, 47 hour drop, 90 s cycle guard)"
  - "deletionRight, deletionClassifyBotRight, deletionReadBotRight, deletionDecideServiceLatch (the delete-right reader the page of plan 16-08 reuses)"
  - "main.go: worker started after StartLockdownWorker, stop handler registered after the admin index sweeper's stop and before the member requirements drain"
  - "AGENTS.md: the 68 s worst case against the enforced 60 s, the worker's cycle order, the latch scope and the Data additions"
  - "test toolkit additions on deletionEnv: script (deletionScriptClient), replica, cycle, pass, seedDeletionRow, seedDue, batchCalls, singleCalls, freshDeletionRoot"
affects: [16-05, 16-06, 16-07, 16-08, 16-09]

plan_head_before: a6f664a5601b91c0c79b429c9802cc91560f6a1f
plan_head_after: 3c40c91c7fe436bcade9add93694e2c168ade628

actuals:
  tokens: 18500   # chars/4 over the added lines of a6f664a..3c40c91 (74250 chars), tests and AGENTS.md included
  tasks: 3
  commits: 5      # MEASURED: git rev-list --count a6f664a..HEAD (before this SUMMARY commit)

tech-stack:
  added: []
  patterns:
    - "one cycle state (deletionCycle) with a settled set: whatever a cycle did not settle (a cut call, a panic, the guard) is released free by one deferred call, so no row is lost"
    - "the cycle guard is a timeout on the cycle context, so every Telegram call and the classifier see a cut-off as retry-free"
    - "one writeContext helper detaches every row write from the cancelled run and bounds it at 5 s"
    - "a script hook on the bot client (deletionScriptClient) models a hung, failing or per-message answer on top of the shared fake, which still records every request"

key-files:
  created:
    - fuku/modules/deletion_worker.go
    - fuku/modules/deletion_right.go
    - fuku/modules/deletion_worker_test.go
    - fuku/modules/deletion_right_test.go
  modified:
    - fuku/modules/deletion_schedule.go
    - fuku/modules/deletion_env_test.go
    - fuku/modules/shutdown_drains_test.go
    - main.go
    - AGENTS.md

key-decisions:
  - "A delete-now after the worker's stop starts no goroutine: deletionDeleteNow returns false once the root context is cancelled, so deletionAct takes the durable path (a row due now) and the message resumes after a restart. This gives cancelDeletionContext and the root check their production users"
  - "StopDeletionWorker cancels the root even when no worker is running (the stop is a shutdown, and inline deletes derive from the same root); tests that call it restore a fresh root"
  - "deletionReadBotRight and deletionDecideServiceLatch take the chat ID (int64), not a chat object: the worker has only the row's chat ID, and the page has chat.Id"
  - "The service latch drops the chat's other claimed service rows of the same cycle as well as its pending ones, so a latched chat is not asked again within the cycle"
  - "A cycle's progress means a row was removed (deleted or dropped); a row only put back is not progress, so the loop waits for the tick instead of spinning"

patterns-established:
  - "env.script.set(func(ctx, method, params) error): return an error to answer with, block on ctx.Done() to hang, nil to let the fake answer"
  - "env.replica(): a second bot process over the same database and Redis with its own fake"

requirements-completed: []

coverage:
  - id: D1
    description: "a queued row whose time has come is claimed, deleted in batch calls of at most 100 IDs through the pacer by the real worker and removed; rows not yet due are left alone"
    requirement: "DEL-06"
    verification:
      - kind: integration
        ref: "TestDeletionWorkerBatches"
        status: pass
  - id: D2
    description: "an undeletable message is isolated by the per-ID fallback and dropped quietly; gone, refused, 5xx, transport, 429 and cancel are each read by class; three counted failures drop a row; a 429 and a shutdown cost no attempt; a row over 47 hours old is dropped with no call and no log line; a delay never exceeds 24 hours"
    requirement: "DEL-06"
    verification:
      - kind: integration
        ref: "TestDeletionWorkerFallsBackPerID, TestDeletionWorkerClassifiesErrors (9 error shapes x 2 modes), TestDeletionWorkerRateLimitedFree, TestDeletionWorkerDropsOldRows"
        status: pass
  - id: D3
    description: "the latch is scoped: gone stops every kind, service only after one live right check (administrator flag not trusted), never for bot kinds; a Staff Group gets no call; a stale-claim guard keeps a live cycle's rows from a second replica"
    requirement: "DEL-07"
    verification:
      - kind: integration
        ref: "TestDeletionServiceLatchNeedsLiveCheck (mutation-checked: trusting the flag fails the administrator case, removing the once-per-cycle guard fails two cases), TestDeletionWorkerSkipsStaffGroup, TestDeletionWorkerLatchedChat, TestDeletionWorkerStaleClaimNotDoubleProcessed"
        status: pass
  - id: D4
    description: "rows survive a restart and a crash and run once across replicas; the worker logs counts only"
    requirement: "DEL-06"
    verification:
      - kind: integration
        ref: "TestDeletionWorkerResumesAfterRestart, TestDeletionWorkerOneDeleteAcrossReplicas (also -count=20, and both replicas took rows in 10 of 10 runs), TestDeletionWorkerLogsNoMessageIDs"
        status: pass
  - id: D5
    description: "the stop ends a hung cycle and a hung inline delete inside the manager's 50 ms default allowance, the rows are pending when the database-close probe runs, a delete-now after the stop takes the durable path, a restart resumes every row; the worker is wired in main.go in the required order"
    requirement: "PLAT-06"
    verification:
      - kind: integration
        ref: "TestShutdownEndsDeletionWorker, TestDeletionWorkerStartStopIdempotent, the main.go order acceptance command"
        status: pass
  - id: D6
    description: "the delete-right classifier maps status and code to can, cannot and unknown, and the live reader goes through the pacer under the caller's timeout"
    requirement: "DEL-08"
    verification:
      - kind: unit
        ref: "TestDeletionBotRightClassifier (19 cases), TestDeletionReadBotRight"
        status: pass
---

# Phase 16 Plan 04: The deletion worker Summary

**Every queued row is now carried out: deleted in paced batches of at most 100 at its time, isolated per message when a batch fails, dropped quietly when the message is gone, refused or too old, retried three times on a 5xx, released free on a 429 or a shutdown, resumed after a restart, done once across replicas, and stopped with the process inside the shutdown budget, with the bot's right to delete classified in one place behind a live check.**

The first task is a tracer: 250 due rows became three batch calls (100, 100, 50) through the pacer, a second chat got its own call and rows due in an hour stayed pending, before any failure handling existed. Tasks 2 and 3 extend that proven path.

## Performance

- **Duration:** about 20 minutes by the commit clock (00:33 to 00:43 UTC for the commits, plus reading the earlier plans and the lockdown worker's frame first).
- **Tasks:** 3 of 3 (task 1 is the tracer)
- **Files:** 9 (4 new, 5 edited)

## Accomplishments

- `fuku/modules/deletion_worker.go`: the eight timing variables, `StartDeletionWorker` / `StopDeletionWorker` in the lockdown worker's shape (context derived from `deletionContext()`), `deletionWorkerLoop` (a pass at start, then per tick or wake from `deletionWake`), `runDeletionCycle` under its own `RecoverFromPanic`, `deletionCycle` (claimed rows, settled set, a deferred free release of whatever is unsettled, `writeContext` on `context.WithoutCancel` with a 5 s bound) and `deletionChatRun` (latch screen, Staff Group read, chunks, batch classification, per-ID fallback, `maybeLatchService`, `gone`).
- `fuku/modules/deletion_right.go`: `deletionRight` (unknown is the zero value), `deletionClassifyBotRight`, `deletionReadBotRight` (paced, under the caller's timeout, never cached) and `deletionDecideServiceLatch` (one live read with the administrator flag not trusted; only `cannot` latches `service` and drops the chat's pending `service:*` rows).
- `fuku/modules/deletion_schedule.go`: `deletionDeleteNow` refuses once the root is cancelled (see decisions).
- `main.go`: `modules.StartDeletionWorker(b)` right after `modules.StartLockdownWorker(b)`; the stop handler right after the admin index sweeper's stop and before `RegisterMemberReqDrain`; no `RegisterHandlerWithTimeout`.
- `AGENTS.md` (same commit as the registration): a `StopDeletionWorker` Startup and shutdown bullet with the 68 s arithmetic (HTTP stop 10, AI spam 10, raid AI 2, raid posts 3, lockdown 6, staff 31, member requirements 3, deletion worker 3) against the enforced 60 s and the exit inside `stop_grace_period: 75s`; the member requirements bullet's 65 s kept with a clause that the deletion stop follows it in LIFO order; the `scheduled_deletions` Data bullet gains the stale release, the guard, the 47 hour drop and the three tries; a "Deletion worker" Subsystem traps bullet.
- Test toolkit: `deletionScriptClient` (`env.script.set`), `env.replica()`, `env.cycle()`, `env.pass()`, `seedDeletionRow`, `env.seedDue`, `batchCalls`, `singleCalls`, `freshDeletionRoot`, `deletionWorkerStopped`.

## Task Commits

1. Task 1 RED `391cdb8` test(16-04): the worker deletes due rows in batches (does not compile: no worker yet)
2. Task 1 GREEN (tracer) `e39850e` feat(16-04): the deletion worker deletes due rows in batches
3. Task 2 RED `05fdaa1` test(16-04): the worker isolates, drops and latches by class (does not compile: no right reader yet)
4. Task 2 GREEN `71a950c` feat(16-04): classify, latch and drop by class in the deletion worker
5. Task 3 `3c40c91` feat(16-04): start the deletion worker and stop it inside the shutdown budget (tests, main.go and AGENTS.md in one commit)

## Verification (real results)

All run from /home/user/Fuku_Robot on SQLite (the `fuku/modules` harness does not run on PostgreSQL).

| Gate | Result |
|------|--------|
| Task 1 verify `-run '^TestDeletionWorkerBatches$' -v` | PASS |
| Task 2 verify `-run '^(TestDeletionWorker\|TestDeletionBotRightClassifier\|TestDeletionServiceLatchNeedsLiveCheck)' -v` (with `TestDeletionReadBotRight`) | 40 PASS lines, 0 FAIL, 0 SKIP |
| Task 2 neighbours `-run '^(TestDeletionWorker\|TestServiceWatcher\|TestDeletionLatch\|TestDeletionClassify)'` | ok (19.3 s) |
| Task 3 verify `-run '^(TestDeletionWorkerResumesAfterRestart\|TestDeletionWorkerOneDeleteAcrossReplicas\|TestShutdownEndsDeletionWorker)$' -v` (with `TestDeletionWorkerStartStopIdempotent`) | all PASS, 0 SKIP |
| `-count=20 -run '^TestDeletionWorkerOneDeleteAcrossReplicas$'` | ok (7.5 s); a `-count=10 -v` run showed both replicas took rows every time |
| `-run '^TestShutdown' ./fuku/modules && CGO_ENABLED=0 go build ./...` | ok, build ok |
| Plan verification `-run '^(TestDeletion\|TestServiceWatcher\|TestCleanService\|TestShutdown\|TestApplyDeleteRule\|TestApplyCleanServiceRule)' ./fuku/modules` | ok (30.8 s) |
| Plan verification `-run '^(TestStaff\|TestLockdown\|TestMemberReqOnePunishmentAcrossReplicas)' ./fuku/modules` | ok (229.8 s) |
| `go vet -tags testtools ./fuku/modules .` | clean |
| `gofmt -l` on every file touched | only `main.go`, which is not clean at `8c0c827` either (an alignment in the `SetMyCommandsOpts` literal at line 530, not my lines) |
| `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827` (exit read directly) | exit 0, "no dupl, gocyclo or godox findings" |
| `grep -n 'reflect.Ptr\|ioutil\.' fuku/modules/deletion_*.go` | nothing |
| `git diff --exit-code 8c0c827 -- go.mod go.sum` | exit 0 |
| Mutation checks | trusting the administrator flag in `deletionDecideServiceLatch` fails the administrator case of `TestDeletionServiceLatchNeedsLiveCheck`; removing the `r.decided` guard fails it and the unknown-answer case |
| Acceptance greps | Task 1: 3 functions, 2 non-comment `RecoverFromPanic`; Task 2: 3 right functions, 0 log lines above debug naming a message ID; Task 3: the `main.go` order command exits 0, `68 s` 1, `StopDeletionWorker` in AGENTS.md 2, `RegisterHandlerWithTimeout` in `main.go` 0 |

**Not run:** `make lint` (golangci-lint is built with Go 1.25; the stand-in ran), `make test`, the whole `fuku/modules` package, the settings test family (no page touched), `make check-translations` and `make check-docs` (no locale or help change), gosec, PostgreSQL (this plan has no migration and no PostgreSQL-only guarantee).

## Decisions Made

See `key-decisions`. The one behaviour change outside the new files is `deletionDeleteNow` returning false after the stop; it makes the plan's test line "a delete-now started after the stop ... takes the durable fallback" true and is the production user of `cancelDeletionContext`. The inline delete that was already hung when the stop arrives is still lost (a cancelled call is no verdict, D-08).

## Deviations from Plan

**1. [Gate text] `context.WithoutCancel` count in `deletion_worker.go` (Task 2 acceptance).** The gate asks for at least 2; the file has exactly 1, in `deletionCycle.writeContext`, which every row write (`Complete`, `Release`, `DropChatPending`, the latch) goes through. Two textual copies would be a dupl clone and a drift risk, so the one helper is the design; `deletion_right.go` has the second use (`deletionDecideServiceLatch`). Corrected gate text: `grep -rv '^\s*//' fuku/modules/deletion_worker.go fuku/modules/deletion_right.go | grep -c 'context.WithoutCancel'` prints at least 2, and `grep -c 'writeContext()' fuku/modules/deletion_worker.go` is at least 4.

**2. [Process] Task 3 has no separate RED commit.** The plan asks for the three tests first and committed red. `StartDeletionWorker`, `StopDeletionWorker` and the loop had existed since Task 1, so the three tests (and the two I added) passed the first time they ran, apart from one test bug of mine (the call the stop cut off is recorded too, so the resume test counts only what the restarted worker sends). One green commit carries the tests, `main.go` and AGENTS.md.

**3. [Added] Extra tests and helpers.** `TestDeletionWorkerStartStopIdempotent` (the plan's behaviour list: a stop with nothing running, a second stop, a second start), `TestDeletionReadBotRight` (the live reader's paced call and its four answers), the `deletionScriptClient`, `replica`, `pass`, `seedDue` and `batchCalls`/`singleCalls` helpers, `deletionWorkerStopped`.

**4. [Plan text] `deletionReadBotRight` takes a chat ID.** The plan wrote `deletionReadBotRight(ctx, b, chat, timeout, trustAdminFlag)`; the worker has only an ID and the page has `chat.Id`, so the parameter is `chatID int64`. Plan 16-08 passes `chat.Id` and `chat.Type != "group"`.

**5. [Plan text] A mid-cycle service latch also drops claimed rows.** The plan names the chat's pending `service:*` rows; the cycle also completes the chat's other unsettled claimed service rows once `cannot` is established, so the chat is not asked again within the cycle. `TestDeletionServiceLatchNeedsLiveCheck` asserts one service call then the bot calls.

**6. [Plan text] `restoreDeletionWorker` now calls `freshDeletionRoot`.** The helper that plan 16-03 added registers a clean-up only; a test that stops and restarts the worker mid-test needs a fresh root at once, so the body became `freshDeletionRoot` and `restoreDeletionWorker` registers it.

## Known Stubs

None. The three items plan 16-03 left without a production caller now have one: `deletionSetLatch(..., deletionScopeService)` (in `deletionDecideServiceLatch`), the reader of `deletionWake` (`deletionWorkerLoop`) and `cancelDeletionContext` (`StopDeletionWorker`).

## Threat Flags

None beyond the plan's register: T-16-15 (a fresh Staff Group read per chat per cycle, rows dropped with no call, an unreadable read releases them free), T-16-16 (retry-free ends the cycle and the shared block makes the next first call fail with no request), T-16-17 (scoped latch behind a live check, never for `bot:*`), T-16-18 (token-guarded completion, the 90 s guard under the 2 minute stale release), T-16-19 (3 s internal wait, root cancel, 68 s against 60 s stated in AGENTS.md), T-16-20 (debug counts only, `TestDeletionWorkerLogsNoMessageIDs` over every failure shape), T-16-21 (the worker never trusts the administrator flag), T-16-SC (`go.mod` and `go.sum` unchanged).

## Requirements

`requirements-completed: []`. DEL-06, DEL-07, DEL-08 and DEL-01, DEL-02, MREQ-15 stay Pending in REQUIREMENTS.md; the closing plan 16-09 completes them. PLAT-06 stays ticked as it was.

## Self-Check: PASSED

Created files exist (`deletion_worker.go`, `deletion_right.go`, `deletion_worker_test.go`, `deletion_right_test.go`) and commits `391cdb8`, `e39850e`, `05fdaa1`, `71a950c`, `3c40c91` are ancestors of HEAD.
