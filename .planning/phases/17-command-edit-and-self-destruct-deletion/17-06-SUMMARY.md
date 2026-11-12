---
phase: 17-command-edit-and-self-destruct-deletion
plan: 06
subsystem: modules
tags: [member-deletion, self-destruct, batcher, shutdown, deletion-worker, queue-ceiling]

requires:
  - phase: 17-command-edit-and-self-destruct-deletion
    provides: plan 17-01 (deletion.Entry, ScheduleBatch, CountPending, the member kinds and ValidateRuleDelay) and plan 17-05 (the member deletion toolkit)
provides:
  - the self-destruct batcher (member_deletion_batcher.go): memberDeleteOffer, memberDeleteFlush, the batcher goroutine, memberDeleteStart and memberDeleteStop, the shed counters and the hourly per-chat shed log gate
  - StartDeletionWorker and StopDeletionWorker now start and stop the batcher under the one existing 3 s wait (no new drain)
  - deletionBatch 1000 and the worker-time rule screen for member:selfdestruct rows (screenSelfDestruct)
  - the toolkit restoreMemberDeleteBatcher, withMemberDeleteQueue, drainMemberDeleteQueue, memberDeleteShedTotal
affects: [17-07, 17-10, 17-11, 17-12]

plan_head_before: 34fffb5
plan_head_after: 0ba9b9772aba9942ecdf4d90536a839c16e3b9a9

actuals:
  tokens: 13000   # chars/4 over the added lines of 34fffb5..0ba9b97 (51940 chars), tests and AGENTS.md included
  tasks: 3
  commits: 6      # MEASURED: git rev-list --count 34fffb5..HEAD before this SUMMARY commit

tech-stack:
  added: []
  patterns:
    - "a bounded channel fed by a non-blocking send, drained by one goroutine that flushes on a ticker or a kick"
    - "an in-process rate-limited log gate keyed by chat with a bounded map"
    - "a stop joined into an existing shutdown handler's single timer instead of a new drain"

key-files:
  created:
    - fuku/modules/member_deletion_batcher.go
    - fuku/modules/member_deletion_batcher_test.go
    - fuku/modules/deletion_worker_member_test.go
  modified:
    - fuku/modules/deletion_worker.go
    - fuku/modules/member_deletion_env_test.go
    - fuku/modules/deletion_env_test.go
    - AGENTS.md

key-decisions:
  - "The batcher is joined into StopDeletionWorker with a single time.Timer shared by the worker and the batcher done channels (a slice walk, not a map), so a stop that overruns warns once and the 68 s arithmetic and stop_grace_period 75s stand."
  - "StopDeletionWorker sets the draining flag before its early return, so an offer after a stop is refused even when the worker never ran."
  - "memberDeleteStart clears the draining flag (a start after a stop runs a new batcher); production never restarts, tests do."
  - "Lifecycle functions are named memberDeleteStart and memberDeleteStop, so the plan's `grep -c memberDelete deletion_worker.go` gate (at least 2) holds."

patterns-established:
  - "memberDeleteOfferN/selfDestructEntry test helpers and the shed counters read through memberDeleteShedTotal."

requirements-completed: []

coverage:
  - id: D1
    description: "An offered self-destruct entry is written by the next flush as one queue row at the message date plus the delay, and the real worker deletes it on time and not before; two offers of one message keep the earlier time"
    requirement: "DEL-05 (contribution only)"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -run '^(TestMemberSelfDestructBatcherQueues|TestMemberSelfDestructBatcherEarliest)$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D2
    description: "Every shed keeps its message and is counted (full, stopping, ceiling, rules unreadable, count unreadable, write failed); the ceiling is exact; one Warn per chat per hour with no message ID; Off, an unreadable rule and a Staff Group write nothing"
    requirement: "DEL-05, DEL-07 (contribution only)"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -run '^(TestMemberSelfDestructBatcherSheds|TestMemberSelfDestructBatcherOffRace|TestMemberDeleteWarnGateBound)$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D3
    description: "StopDeletionWorker ends the batcher inside the unchanged 3 s wait with a final flush bounded to 2 s on a detached context; a hanging writer loses only never-queued entries"
    requirement: "PLAT-06 (contribution only)"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=10 -run '^TestShutdownEndsMemberDeletionBatcher$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D4
    description: "The worker claims 1000 rows a cycle with at most one call per chunk of 100 per chat (40 chats of 25 rows are 40 calls; 3000 rows of one chat are 30 calls over three cycles) and screens claimed self-destruct rows against the current rule"
    requirement: "DEL-05 (contribution only)"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -run '^(TestDeletionWorkerManyChats|TestDeletionWorkerScreensSelfDestructOff|TestDeletionWorker.*)$' ./fuku/modules"
        status: pass
    human_judgment: false

duration: 55min
completed: 2026-11-12
status: complete
---

# Phase 17 Plan 06: The self-destruct batcher and the 1000-row worker Summary

A self-destruct entry offered to a bounded in-process batcher becomes a durable queue row without the dispatcher ever waiting on the database, every shed keeps the message, the batcher stops inside the unchanged 3 s wait of `StopDeletionWorker`, and the worker drains 1000 rows a cycle while screening self-destruct rows against the current rule.

## What was built

- `fuku/modules/member_deletion_batcher.go`: `memberDeleteOffer` (one non-blocking send into a 4096-entry channel; a full channel, a draining process and a wrong kind refuse), `memberDeleteFlush` (take what is buffered, merge to the earliest time per message, per chat in ascending order one rules read, one exact `deletion.CountPending` bounded by the 20,000 ceiling, entries past it shed latest first, one `deletion.ScheduleBatch` write over everything kept, wake the worker only when a written entry is already due), the goroutine (`memberDeleteStart`, a 300 ms ticker plus a kick at 100 buffered entries, each flush under its own panic recovery on `context.WithoutCancel(deletionContext())` bounded by `deletionRecordTimeout`), `memberDeleteStop` and the final flush (repeats until empty, bounded to 2 s). Shed reasons have their own atomic counters; `memberDeleteShedWarn` allows one Warn per chat per hour (chat ID and reason name only), tracks at most 10,000 chats, forgets entries older than an hour and, when none can be forgotten, logs one process-wide line and nothing more for a new chat. A rule that is absent at flush drops the chat's entries as correct (`memberDeleteDroppedOff`), an unreadable rule or count sheds.
- `fuku/modules/deletion_worker.go`: `StartDeletionWorker` starts the batcher; `StopDeletionWorker` cancels the root, stops the batcher (draining flag first, so it works with no worker running) and waits for both done channels under one `time.Timer`; `deletionBatch` 200 to 1000 with its comment; `screenSelfDestruct` in the per-chat run after the Staff Group read: one cached rules read per chat per cycle, rows completed without a call when the rule is absent or refused by `ValidateRuleDelay`, put back free (no attempt) when the read fails, other kinds untouched.
- Tests: `member_deletion_batcher_test.go` (`TestMemberSelfDestructBatcherQueues`, `...Earliest`, `...Sheds` with six subtests, `TestMemberDeleteWarnGateBound`, `TestMemberSelfDestructBatcherOffRace`, `TestShutdownEndsMemberDeletionBatcher`), `deletion_worker_member_test.go` (`TestDeletionWorkerManyChats`, `TestDeletionWorkerScreensSelfDestructOff`), toolkit additions in `member_deletion_env_test.go`.
- `AGENTS.md` (same commits): the `StopDeletionWorker` bullet says it also stops the batcher with no new drain and the 68 s figure unchanged; a new Subsystem traps bullet "Member deletion, the self-destruct batcher" with the order, shed reasons, log gate and the ceiling table (1 min 333/s ... 24 h 0.23/s); BOTH claim-size sentences now say 1000 (`grep -c 'at most 200 due rows\|claim up to 200' AGENTS.md` went from 2 to 0), plus the drain arithmetic and the rule screen in the Deletion worker bullet.

## Verification results (real)

- Plan verification run: `go test -tags testtools -race -count=1 -run '^(TestMemberSelfDestruct|TestShutdownEnds|TestDeletionWorker|TestDeletionEndToEnd)' ./fuku/modules` printed `ok ... 26.088s`.
- Task 2 command: `-run '^(TestMemberSelfDestructBatcher.*|TestShutdownEndsMemberDeletionBatcher|TestShutdownEndsDeletionWorker|TestMemberDeleteWarnGateBound)$' -v`: every test and subtest PASS, no SKIP; `-count=10 -run '^TestShutdownEndsMemberDeletionBatcher$'` printed `ok ... 24.355s`.
- Task 3 command: `-run '^(TestDeletionWorkerManyChats|TestDeletionWorkerScreensSelfDestructOff)$' -v`: both PASS with their five subtests, no SKIP.
- Wider regression `-run '^(TestDeletion|TestMemberDeletion|TestCommandDeletion|TestMemberEdited|TestBot|TestScheduleBotMessage)' ./fuku/modules`: `ok ... 76.154s` (every Phase 16 worker test, the end-to-end test and the earlier Phase 17 tests unchanged and green).
- `CGO_ENABLED=0 go build ./...` ok; `go vet -tags testtools ./fuku/modules` ok; `gofmt -l` on the six touched Go files prints nothing; `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827` exit 0 (read directly): "no dupl, gocyclo or godox findings on lines added since 8c0c827".
- Invariants: `git diff --exit-code 8c0c827 -- go.mod go.sum` exit 0; the main.go registration count is 18 at HEAD and at `42c48c4`, and `git diff --exit-code 42c48c4 -- fuku/utils/shutdown` exit 0 (invariant ok); `grep -c 'func memberDeleteOffer(\|func memberDeleteFlush(' ...` 2; code lines naming `deletionAct(` or `deletionSchedule(` in the batcher 0; `grep -c memberDelete deletion_worker.go` 2; `0\.23/s` in AGENTS.md 1; `deletionBatch = 1000` line 1; `KindMemberSelfDestruct` in the worker 3; `at most 1000 due rows` 1.
- Not run: `make lint`, `make test` (not this plan's task), gosec (cannot run here), the settings test family (this plan touches no settings code), PostgreSQL (the plan adds no new SQL; `ScheduleBatch` and `CountPending` are already in the integrity list from plan 17-01).

## Deviations from Plan

1. **[Rule 3 - blocking] `freshDeletionRoot` (`fuku/modules/deletion_env_test.go`, not in the plan's file list) also clears the batcher's draining flag.** A stop now sets `memberDeleteDraining`, and every Phase 16 test that stops the worker would otherwise leave it set for all later tests in the package (the next plan's watcher tests offer entries), turning offers into sheds. The edit is one line plus a comment; no assertion changed.
2. **Lifecycle names.** The functions are `memberDeleteStart` and `memberDeleteStop` (not `...Batcher`), so the plan's acceptance grep for `memberDelete` in `deletion_worker.go` holds.
3. **The Staff Group read failure test uses a wrapped batch writer.** The deletion package's `staffGroupLookup` is not reachable from `fuku/modules` and the plan lists no test hook, so `TestMemberSelfDestructBatcherOffRace` swaps `memberDeleteBatchWrite` for a writer that drops the unreadable chat and returns an error joined to the real `ScheduleBatch` result, as `ScheduleBatch` does. The silent Staff Group refusal inside `ScheduleBatch` is tested for real (`makeStaffGroup`). No production hook was added.
4. Added a `member:edited` row to the worker screen test (never screened) and `TestMemberSelfDestructBatcherEarliest` and `TestMemberDeleteWarnGateBound` as extra small tests; the plan's six names all exist.
5. The task commits are test-first in order but the production batcher file was already written when the first test commit was made, so "red" commits do not exist as failing runs (the test commits are not compilable alone, as in earlier plans, because they name symbols the following feat commit adds).

No claim ledger file existed for this plan (the protocol 0c ledger was not created); `plan_head_before` is the parent of the first commit.

## Known limits (stated in AGENTS.md)

- A regular flush in flight when the stop is called finishes or is cut off by the 3 s timer; the final flush then runs only if time remains. Entries not written are never queued and their messages stay.
- The ceiling counts queued rows of the kind (pending and acting), so a message already queued and offered again is counted twice for that one flush (a harmless underuse of at most the batch).

## Requirements

`requirements-completed: []`. DEL-05, DEL-07 and PLAT-06 are contributed to; only plan 17-12 completes DEL-03, DEL-04 and DEL-05. `requirements.mark-complete` was not called.

## Known Stubs

None. `memberDeleteOffer` has tests as its only caller until plan 17-07's watcher offers to it (the brief's rule for a function a later plan of this phase is the only production user of).

## Threat Flags

None. No new endpoint, Redis key, schema or drain; the batcher writes only through `deletion.ScheduleBatch` (Staff Group refused inside it, T-17-03 inherited) and mitigates T-17-23 to T-17-27 as the plan's register says (bounded channel and exact ceiling, non-blocking offer, flush- and worker-time rule screens, one 3 s timer, one hourly Warn per chat with no message ID).

## Self-Check: PASSED

- Created files `fuku/modules/member_deletion_batcher.go`, `fuku/modules/member_deletion_batcher_test.go`, `fuku/modules/deletion_worker_member_test.go`: FOUND.
- Commits 215a89e, af73617, fcc1013, 3f43bbf, 1f57c3e, 0ba9b97: all reachable from HEAD.
