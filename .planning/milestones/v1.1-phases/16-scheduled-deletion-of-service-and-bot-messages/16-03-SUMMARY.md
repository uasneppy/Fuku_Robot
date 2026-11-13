---
phase: 16-scheduled-deletion-of-service-and-bot-messages
plan: 03
subsystem: modules
tags: [deletion, service-messages, watcher, pacer, latch, classifier, inline-delete, cleanservice, tracer]

requires:
  - phase: 16-scheduled-deletion-of-service-and-bot-messages
    provides: "plan 16-01: rules cache, queue (Schedule, ClaimDue, Complete, Release), ApplyJoinLeftRule; plan 16-02: kind tables, deletionServiceKindOf, deletionEffectiveDelay, applyDeleteRule, applyCleanServiceRule, the seams deletionRulesLookup and deletionFlagLookup"
provides:
  - "the ServiceDeletion module (priority 242) with serviceDeleteWatcher at handler group -5 (serviceDeleteGroup), SetAllowBot, a named result preset to ext.ContinueGroups"
  - "deletionPacer / deletionPaced (fuku:delete:pace:next and :block, 100 ms, 3 retries, 60 s), deletionNow, deletionContext / cancelDeletionContext"
  - "deletionDeleteNow (capped inline goroutine, panic guard, durable fallback), deletionInline tracker, deletionInlineMax"
  - "deletionClassify (table of predicates: done, retryFree, gone, refused, retry) and the scoped latch fuku:delete:norights:<chat> (gone, service)"
  - "deletionSchedule (the only caller of the queue writer, through deletionQueueWrite), deletionSentAt, wakeDeletionWorker / deletionWake, deletionAct, deletionRuleFor, deletionApplyMessage"
  - "cleanService deletes after join processing by the effective rule; the watcher handles bot-caused joins and leaves"
  - "the test toolkit deletionEnv (newDeletionEnv, sendService, deliver, deletes, queued, makeStaffGroup, withFastDeletionPacer, restoreDeletionWorker, deletionHangClient)"
affects: [16-04, 16-05, 16-06, 16-07, 16-08, 16-09]

plan_head_before: 5283de21f7dd17492be9e32193d9c1c8551d1f5a
plan_head_after: ddfa52785676a0da74b6b4c18272549b4b4fe543

actuals:
  tokens: 17000   # chars/4 over the added lines of 5283de2..ddfa527 (68157 chars), tests and AGENTS.md included
  tasks: 3
  commits: 6      # MEASURED: git rev-list --count 5283de2..HEAD (before this SUMMARY commit)

tech-stack:
  added: []
  patterns:
    - "named result preset to ext.ContinueGroups plus the panic guard, proved by a second handler of the same group that must still run"
    - "one decision function (deletionAct) for latch, Staff Group refusal, inline delete and queue row; one shared tail (deletionApplyMessage) for the watcher and cleanService"
    - "classifier as a table of predicates tried in order, so no function passes gocyclo"
    - "inline goroutines counted by a startBelow tracker with a durable queue-row fallback when the cap or the pacer refuses"

key-files:
  created:
    - fuku/modules/deletion_schedule.go
    - fuku/modules/deletion_service.go
    - fuku/modules/deletion_env_test.go
    - fuku/modules/deletion_service_test.go
    - fuku/modules/deletion_schedule_test.go
  modified:
    - fuku/modules/greetings.go
    - fuku/modules/greetings_command_test.go
    - fuku/modules/deletion_apply_test.go
    - AGENTS.md

key-decisions:
  - "The Staff Group refusal lives in deletionAct for every delay (a fresh deletionStaffLookup read, an error counts as a Staff Group), on top of deletion.Schedule's own refusal; the plan text had it only for delay 0"
  - "An inline delete the pacer shed (ratelimit.ErrRateLimited) and an inline call beyond deletionInlineMax both become a queue row due now plus a worker wake; a cancelled context, a retry-class error and a refused answer lose that one message quietly (D-08)"
  - "deletionRuleFor (rules plus the shared flag for joined and left) and deletionApplyMessage are one tail for the watcher and cleanService, so the two paths cannot drift and dupl stays quiet"
  - "A 429 that reaches the classifier is retryFree; a deadline error on a live context is retry; a cancelled context or context.Canceled is retryFree"

patterns-established:
  - "deletionAfterWatcher / deletionAfterCleanService: a counting handler registered after the handler under test shows whether the update was ended"
  - "env.deliver(id, ...) replays one update ID through the real dispatcher"

requirements-completed: []

coverage:
  - id: D1
    description: "a pinned notice is deleted at once through the real dispatcher by one paced call, never without a rule, with another chat's rule, for content, or in a Staff Group"
    requirement: "DEL-01"
    verification:
      - kind: integration
        ref: "TestServiceWatcherDeletesNow, TestServiceWatcherSkipsStaffGroup"
        status: pass
  - id: D2
    description: "a rule of N writes one queue row at sent_at + N, keeping the earlier time; a rule of 0 writes none"
    requirement: "DEL-06"
    verification:
      - kind: integration
        ref: "TestServiceWatcherSchedulesAfterN, TestServiceWatcherZeroWritesNoRow, TestDeletionDelayCapped"
        status: pass
  - id: D3
    description: "the watcher never ends an update or returns nil for any sender, panic, unreadable rule, Staff Group, private chat or channel"
    requirement: "DEL-01"
    verification:
      - kind: integration
        ref: "TestServiceWatcherSenders, TestServiceWatcherNeverEndsUpdate (mutation-checked: result = nil fails six cases)"
        status: pass
  - id: D4
    description: "the inline path is capped and recovered, falls back to a durable row, the classifier and the scoped latch keep an unreachable chat from being hammered, the pacer has its own budget"
    requirement: "DEL-06"
    verification:
      - kind: integration
        ref: "TestDeletionInlineCapFallsBackToQueue (also -count=20), TestDeletionInlineShedFallsBackToQueue, TestDeletionClassify, TestDeletionLatch, TestDeletionPacerIsIndependent"
        status: pass
  - id: D5
    description: "joined and left are deleted after join processing by the effective rule; a bot-caused join or leave is the watcher's; a Staff Group is refused in three places"
    requirement: "DEL-07"
    verification:
      - kind: integration
        ref: "TestCleanServiceEffectiveRule, TestCleanServiceDeletesAfterJoin, TestServiceWatcherBotJoin (the lockdown-ended case fails without LoadLockdown, so it discriminates)"
        status: pass
---

# Phase 16 Plan 03: Deletion core and service watcher Summary

**A deletable service notice is deleted at once or queued for its time by the group's stored rule, through the real dispatcher, in every group but a Staff Group: a watcher at group -5 that never ends an update, a pacer of its own, a capped recovered inline delete with a durable fallback, a classifier and a scoped latch, and `cleanService` rewired to delete after join processing by the shared flag's effective rule.**

This plan's first task is the phase's tracer. One pinned notice goes from a rule stored by `applyDeleteRule`, through the real dispatcher, to one paced `deleteMessage` call, and it was proven green by `TestServiceWatcherDeletesNow` before any expansion; Tasks 2 and 3 extend that proven path.

## Performance

- **Duration:** about 22 minutes by the commit clock (00:06 to 00:28 UTC); one stop of a few minutes to free disk (see Deviations).
- **Tasks:** 3 of 3 (task 1 is the tracer)
- **Files:** 9 (5 new, 4 edited)

## Accomplishments

- `fuku/modules/deletion_service.go`: `serviceDeleteGroup = -5`, the `ServiceDeletion` module registered at priority 242 (not a help module), `serviceDeleteWatcher` with a named result preset to `ext.ContinueGroups`, `serviceDeleteCandidate` (joined and left only for a bot sender), `deletionRuleFor`, `deletionApplyMessage`.
- `fuku/modules/deletion_schedule.go`: the pacer, `deletionContext`, the inline tracker and `deletionDeleteNow`, `deletionClassify`, the latch, `deletionSentAt`, `deletionSchedule`, `deletionAct`, `wakeDeletionWorker`.
- `fuku/modules/greetings.go`: `cleanService` keeps join processing and replaces its unconditional, row-creating, error-returning delete with `deletionApplyMessage`; it always ends with `ext.EndGroups`.
- Tests: `TestServiceWatcherDeletesNow`, `TestServiceWatcherSkipsStaffGroup`, `TestServiceWatcherSchedulesAfterN`, `TestServiceWatcherZeroWritesNoRow`, `TestServiceWatcherSenders`, `TestServiceWatcherNeverEndsUpdate`, `TestDeletionClassify`, `TestDeletionLatch`, `TestDeletionInlineCapFallsBackToQueue`, `TestDeletionInlineShedFallsBackToQueue` (added), `TestDeletionPacerIsIndependent`, `TestDeletionDelayCapped`, `TestCleanServiceEffectiveRule`, `TestCleanServiceDeletesAfterJoin`, `TestServiceWatcherBotJoin`.
- AGENTS.md: group `-5` in the group list, a `ServiceDeletion` Handlers bullet, the retired-raid-mode sentence amended, the `fuku:delete:pace:*` and `fuku:delete:norights:<chat>` Data bullets, and a "Scheduled deletion core" Subsystem traps bullet.

## Task Commits

1. Task 1 RED `e437762` test(16-03): a pinned notice is deleted through the real dispatcher
2. Task 1 GREEN (tracer) `a063d10` feat(16-03): delete a service notice at once by its stored rule
3. Task 2 RED `d950bdc` test(16-03): service notices are queued, latched and never end an update
4. Task 2 GREEN `2a4a266` feat(16-03): queue a service notice after N seconds and latch an unreachable chat
5. Task 3 RED `85e5366` test(16-03): joined and left follow the shared flag after join processing
6. Task 3 GREEN `ddfa527` feat(16-03): joined and left deletion follows the shared flag after join processing

The Task 1 and Task 2 RED commits do not compile (symbols missing); the Task 3 RED commit compiles and fails on 10 cases.

## Verification (real results)

All run from /home/user/Fuku_Robot on SQLite (the `fuku/modules` harness does not run on PostgreSQL).

| Gate | Result |
|------|--------|
| Task 1 verify `-run '^(TestServiceWatcherDeletesNow\|TestServiceWatcherSkipsStaffGroup)$' -v` | PASS, 0 SKIP |
| Task 2 verify `-run '^(TestServiceWatcher\|TestDeletionClassify\|TestDeletionLatch\|TestDeletionInlineCapFallsBackToQueue\|TestDeletionPacerIsIndependent\|TestDeletionDelayCapped)' -v` | all PASS, 0 SKIP |
| `-count=20 -run '^TestDeletionInlineCapFallsBackToQueue$'` | ok |
| Task 3 verify `-run '^(TestCleanService\|TestServiceWatcher\|TestDeletion)' -v` | all PASS, 0 SKIP, 0 FAIL |
| Task 3 neighbours `-run '^(TestGreeting\|TestDelJoined\|TestGreetings\|TestGreetOnce\|TestLegacyGreetingsRequestsUnchanged\|TestLockdown)' ./fuku/modules` | ok (94.5 s) |
| Plan verification `-run '^(TestCleanService\|TestServiceWatcher\|TestDeletion\|TestApplyDeleteRule\|TestApplyCleanServiceRule\|TestServiceKind\|TestStaff)' ./fuku/modules` (the plan's `TestGreeting\|TestGreetOnce\|TestLockdown\|TestStaff` neighbours are covered by this run and the one above) | ok (151.6 s) |
| `-run '^(TestLoadAllModules\|TestRegistry\|TestHelp\|TestSettingsMain\|TestSettingsHelp)'` | ok |
| `make check-docs` | no drift |
| `go vet -tags testtools ./fuku/modules`, `CGO_ENABLED=0 go build ./...` | clean |
| `gofmt -l` on every file touched | nothing |
| `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827` (exit read directly) | exit 0, "no dupl, gocyclo or godox findings" (the first run reported two errcheck findings on `miniredis.Set` in my test, fixed, re-run exit 0) |
| `grep -n 'reflect.Ptr\|ioutil\.' fuku/modules/deletion_*.go` | nothing |
| `git diff --exit-code 8c0c827 -- go.mod go.sum` | exit 0 |
| Mutation checks | `result = nil` in the watcher fails six cases of `TestServiceWatcherNeverEndsUpdate`; removing `LoadLockdown` from the lockdown-ended case fails `TestServiceWatcherBotJoin` |
| Acceptance greps | Task 1: `serviceDeleteGroup` 2 (and 1 for `= -5`), `fuku:delete:pace:next` 1, `:block` 1, panic guard in both files; Task 2: `deletion.Schedule` outside `deletion_schedule.go` 0, `deletionQueueWrite` 2, `fuku:delete:norights:` 1, `math/rand` 0; Task 3: `if greetPrefs.ShouldCleanService {` 0, `service message deletion watcher` 3, `fuku:delete:norights` 1, `greetings_command_test.go` numstat vs `dd61b2c` 0 deletions |

**Not run:** `make lint` (golangci-lint is built with Go 1.25; the stand-in ran), `make test`, the whole `fuku/modules` package, the settings test family (no page touched), `make check-translations` (no locale change), gosec, PostgreSQL (this plan has no migration).

## Decisions Made

See `key-decisions`. The one refinement of D-08 under overload (research S-05): an inline delete that the pacer sheds, and a delete-now call beyond `deletionInlineMax`, are not lost but become a queue row due now plus a worker wake. A cancelled context, a retry-class error (5xx, transport) and a refused answer still lose that one message quietly, as D-08 accepts.

## Deviations from Plan

**1. [Gate text] `deletionAct(` in both `greetings.go` and `deletion_service.go` (Task 3 acceptance).** `cleanService` and the watcher share one tail, `deletionApplyMessage`, which calls `deletionAct`; two copies of the read-rule-and-act sequence would be a dupl clone and a drift risk. Corrected gate text: `for f in fuku/modules/greetings.go fuku/modules/deletion_service.go; do test "$(grep -v '^\s*//' $f | grep -c 'deletionApplyMessage(')" -ge 1 || echo MISSING $f; done` prints nothing, and `grep -c 'deletionAct(' fuku/modules/deletion_service.go` is 1. The plan's own wording ("one shared decision") is met.

**2. [Rule 2 - Missing critical functionality] Staff Group refusal for every delay.** The plan's `deletionAct` text refuses a Staff Group for delay 0 and leaves delay N to `deletion.Schedule`'s own read. With the `deletionStaffLookup` seam only covering the first, an unreadable Staff Group read could still write a row through the second. `deletionAct` now makes the one fresh `deletionStaffLookup` read for both (an error counts as a Staff Group), so the three places of D-18 (watcher and `cleanService` through `deletionAct`, and `deletion.Schedule`) agree and `TestServiceWatcherNeverEndsUpdate` and `TestServiceWatcherSkipsStaffGroup` hold for both delays. Cost: one indexed read per scheduled message, doubled by `Schedule`'s own.

**3. [Plan text] `deletionCleanup` lives in `deletion_apply_test.go`, not `deletion_env_test.go`.** Plan 16-02 already defined it there. I extended it in place (it also deletes `scheduled_deletions` rows) and the environment calls it plus `staffCleanup`, instead of moving or duplicating it.

**4. [Added] Extra tests and helpers.** `TestDeletionInlineShedFallsBackToQueue` (the plan's `TestDeletionInlineCapFallsBackToQueue` behaviour lists the shed case; a block longer than the pacer's `MaxWait` makes it a separate, cleaner test), `deletionHangClient` (a bot client wrapper that holds `deleteMessage` until its context ends, because the shared fake's `setDelay` sleeps and ignores the context), `deletionAfterWatcher` and `deletionAfterCleanService`.

**5. [Plan text] `restoreDeletionWorker` does not clear a draining flag.** The inline tracker has no draining state in this plan (a call after the root is cancelled fails at once inside the pacer), so the helper only puts a fresh root context back, as `restoreMemberReqRunner` does for the root.

**6. [Existing tests edited as the plan says] `TestCleanServiceProcessesJoinAndDeletesServiceMessage` and `TestCleanServiceProcessesMultipleNewMembersWithoutCaptcha`** gain one `deletionInline.wait()` line each before their delete assertion (research C-08); the numstat against `dd61b2c` shows 0 deleted lines. They are behavioural tests, not Phase 12 characterization tests.

**7. [Environment] Disk full mid-plan.** The Go build cache had grown to 25 GB and the link step failed with "no space left on device". I deleted cache files not accessed for 180 minutes under `/root/.cache/go-build` (a regenerable cache; nothing in the repository was touched). No code or result was affected.

**8. [Process] Commit ledger.** The `gsd-plan-head-before-16-03` ledger was written before the first commit from `5283de2`; `commits:` is measured (6).

## Known Stubs

None. `deletionSetLatch` with the `service` scope, `wakeDeletionWorker`'s reader (`deletionWake`) and `cancelDeletionContext` have no production caller yet: plan 16-04 (the worker and its stop, and the live right check that sets the `service` scope) is their only user. Each is exercised by a test of this plan (`TestDeletionLatch`, `TestDeletionInlineCapFallsBackToQueue`), and the lint stand-in reports no unused finding. Rows written by this plan are carried out only by the worker of plan 16-04.

## Threat Flags

None beyond the plan's register: T-16-09 (Staff Group refused in the watcher, `cleanService` and `Schedule`; a failing read deletes nothing), T-16-10 (named preset result, panic guard, group -5 hosts only this watcher), T-16-11 (inline cap with a durable fallback, a pacer of its own, a root context the stop will cancel), T-16-12 (scoped latch set only by the classifier's `gone`, never by a message-level refusal), T-16-13 (an unreadable rule or flag deletes nothing), T-16-14 (logs carry the chat ID only), T-16-SC (`go.mod` and `go.sum` unchanged).

## Requirements

`requirements-completed: []`. DEL-01, DEL-06 and DEL-07 stay Pending; the closing plan 16-09 completes them.

## Self-Check: PASSED

Created files exist (`deletion_schedule.go`, `deletion_service.go`, `deletion_env_test.go`, `deletion_service_test.go`, `deletion_schedule_test.go`) and commits `e437762`, `a063d10`, `d950bdc`, `2a4a266`, `85e5366`, `ddfa527` are ancestors of HEAD.
