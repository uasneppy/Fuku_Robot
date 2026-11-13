---
phase: 13-member-requirements
plan: 09
subsystem: modules
tags: [member-requirements, watcher, kill-switch, pass-cache, runner, shutdown-drain, skiplocal]
requires:
  - phase: 13-member-requirements
    provides: memberReqEnforce and its helpers (13-07, 13-08); memberReqBroken, memberReqPassValue (13-02); memberReqSettingsLookup (13-05); ClearHoldOnClean, StaleClaim (13-01); the memberReqEnv test environment (13-07)
provides:
  - the MemberRequirements module (priority 241) and its group -2 message watcher, the only message handler of that group
  - the enforcement kill switch memberReqEnforcementLive (an atomic.Bool, read only through memberReqLive(), false in production until 13-13)
  - the positive-only pass value fuku:memberreq:pass:<chat>:<user> and the clean path that ends a fixed warn hold
  - the bounded job runner (memberReqRunAsync, 32 in flight, memberReqJobTimeout 90 s) and its shutdown drain (StopMemberReqJobs, RegisterMemberReqDrain, main.go)
  - the member_requirements prefix in skipLocal
  - test helpers withMemberReqLive, loadMemberReq, mreqSend, mreqSendAt, mreqReplica, restoreMemberReqRunner
affects: [13-10, 13-11, 13-12, 13-13, 13-14, member-requirements]

actuals:
  tokens: 15000
  tasks: 3
  commits: 6

commits: 6
plan_head_before: 517cecabd9de3f20f92da215b47bb47af788a2ca
plan_head_after: f6407cf361fa4f05dbb268368dac9a04b70cdca0

tech-stack:
  added: []
  patterns:
    - "The watcher's result is a named value preset to ext.ContinueGroups, so a panic the deferred recover swallows still returns ContinueGroups and never nil"
    - "memberReqJobs.startBelow(limit) checks the cap and counts the job in one locked step; the draining flag is read after the job is counted"
    - "A kill switch as an atomic.Bool with one reader; tests write it only through a helper that restores the previous value"

key-files:
  created:
    - fuku/modules/memberreq.go
    - fuku/modules/memberreq_watch.go
    - fuku/modules/memberreq_watch_test.go
  modified:
    - fuku/modules/memberreq_env_test.go
    - fuku/modules/shutdown_drains.go
    - fuku/modules/shutdown_drains_test.go
    - main.go
    - fuku/db/cache/local.go
    - fuku/db/cache/local_test.go
    - AGENTS.md

key-decisions:
  - "The pure name rules run before the pass-value GET: a violator pays no Redis round trip, and only a clean member (the common case) reads the pass value. Cheaper than research S-02's order and equivalent, because a violator never has a pass value for the same names and settings"
  - "A clean verdict is stored only for a message not older than 2 minutes (memberReqPassMaxAge): a backlog message processed late must not hide a hold created afterwards from the clean predicate for 30 minutes"
  - "The watcher's result is a named, preset return value, because the deferred panic recover would otherwise return nil, which this gotgbot reads as leave the group"
  - "The three-replica test proves the claim race with a mute (fully concurrent); its warn variant judges the first message alone, because on SQLite a concurrent writer makes the warn module's read-then-write transaction fail with database is locked (PostgreSQL waits)"

requirements-completed: [MREQ-13]

duration: 3h (including the SQLite lock investigation and the 5-minute settings family)
completed: 2026-11-07
status: complete
---

# Phase 13 Plan 09: The group -2 message watcher behind the enforcement switch Summary

**Every supergroup message is now screened by one watcher at handler group -2: a clean member costs a cached settings read and, at most once per 30 minutes, one clean DELETE; a violator goes to a bounded, panic-recovering job that runs `memberReqEnforce`; and the whole path is inert in production, because the enforcement switch `memberReqEnforcementLive` is an `atomic.Bool` that nothing but a test helper writes until plan 13-13.**

## Performance

- **Tasks:** 3 of 3 (task 1 tracer, tasks 2 and 3 TDD)
- **Commits:** 6 (three RED `test(13-09)`, three GREEN `feat(13-09)`), measured from `plan_head_before` with `git rev-list --count`.

## How the kill switch was checked (T-13-29, T-13-56)

- `grep -rn memberReqEnforcementLive --include=*.go .` finds the declaration and `Load()` in `fuku/modules/memberreq.go` and, outside it, only `withMemberReqLive` in `memberreq_env_test.go` (`Load` and `Store`, restoring the previous value in `t.Cleanup`). No production line calls `Store`. `main.go` does not touch it.
- `TestMemberReqWatcherOffUntilLive` asserts `memberReqLive()` is false outside any helper (the production default of this plan), that ten messages of a violator with every rule on Mute start no job, write no hold or warn, make no `getChatMember`, `restrictChatMember`, `banChatMember` or `unbanChatMember` about the member and post nothing while the update still reaches groups -1 and 3, and that `withMemberReqLive(t, false)` inside a test that set true leaves the switch true afterwards (the previous value is restored, not a fixed false).
- After this plan a real group cannot be punished by any production path: the only caller of `memberReqEnforce` is the watcher's job, started only behind `memberReqLive()`.

## Accomplishments

- **Module and switch (`memberreq.go`).** `memberReqModule`, `memberReqEnforcementLive atomic.Bool`, `memberReqLive()`, `LoadMemberRequirements` (a `handlers.NewMessage` with a supergroup filter, no `SetAllow*`, at `memberReqWatchGroup` -2) and `RegisterLegacyModule("MemberRequirements", 241, ...)`.
- **Watcher (`memberreq_watch.go`).** Order, cheapest first: switch, item (`raidFloodItemKey`), sender shape (`raidFloodSenderCounts`), cached settings (error or no rule on ends it), pure rules, then clean member: pass value and clean predicate; violator: cached admin list, then `memberReqRunAsync`. Every return is `ext.ContinueGroups` (a grep of the watcher body finds no `return nil` and no `ext.EndGroups`).
- **Pass cache and clean path.** `memberReqPassKey`, `memberReqPassHit`, `memberReqClean`: a 16-hex fingerprint under `fuku:memberreq:pass:<chat>:<user>`, 30 minutes, written only after a successful `ClearHoldOnClean`; a Redis error or no Redis (the `memberReqRedisClient` seam) is a miss.
- **Runner and drain.** `memberReqRunAsync` (cap 32 via `startBelow`, one shed log line with IDs only, `RecoverFromPanic`, a context under `memberReqJobTimeout` 90 s from the cancellable root `memberReqContext`), `StopMemberReqJobs` (draining flag, cancel root first, wait at most `memberReqStopWait` 2 s), `RegisterMemberReqDrain` (wait plus `shutdownDrainGrace`), registered in `main.go` at line 226, before `RegisterStaffActionsDrain` at line 231.
- **`skipLocal`.** `memberRequirementsPrefix` added with its reason; `TestSkipLocalMatchesFreshnessCriticalKeys` extended and `TestSkipLocalMemberRequirementsKey` added.
- **AGENTS.md** (task 3 commit): the Handlers group list and a module bullet (group -2 rule, the switch and `withMemberReqLive`), the `StopMemberReqJobs` drain bullet (3 s, LIFO place, 65 s webhook worst case against 60 s, `stop_grace_period: 75s`, the cut handlers, why it costs no correctness, the 90 s job deadline), the pass-key Data bullet, the `skipLocal` clause on the `chat_requirements` and settings-page bullets, the runner and basic-group limits under Subsystem traps, the Testing families, and the executor bullet's "nothing calls it" sentence corrected.

## Task Commits

1. Task 1 (tracer): `5ad1102` test RED, `c8ba945` feat GREEN
2. Task 2: `9a51e42` test RED, `81bd161` feat GREEN
3. Task 3: `ff28d16` test RED, `f6407cf` feat GREEN

## TDD Gate Compliance

| Gate | Commit | Evidence |
|------|--------|----------|
| RED task 1 | 5ad1102 | Compile-only stand-ins (the module, the atomic switch and accessor, a watcher that only returns `ext.ContinueGroups`, `memberReqJobs`). `TestMemberReqWatcherPunishesOnce` failed on its assertion (`warns row after ten messages = <nil>`); `TestMemberReqWatcherOffUntilLive` and `TestMemberReqWatcherContinuesGroups` passed against the stand-in, as guards of behaviour a stub already has |
| GREEN task 1 | c8ba945 | all three pass; `TestRegisteredModulesRespectGotgbotDefaultGroupSemantics` still passes |
| RED task 2 | 9a51e42 | Compile-only stand-in: the `memberReqRedisClient` seam. Failing on assertions: `TestSkipLocalMemberRequirementsKey`, `TestSkipLocalMatchesFreshnessCriticalKeys`, four `TestMemberReqPassCache` subtests (verdict not written, rename, settings change, no Redis) and the second subtest of `TestMemberReqCleanEndsWarnHold`. `TestMemberReqWatcherExemptions` and three pass-cache subtests passed against the tracer's code (guards) |
| GREEN task 2 | 81bd161 | all pass |
| RED task 3 | ff28d16 | Compile-only stand-ins: `StopMemberReqJobs` returning true, an empty `RegisterMemberReqDrain`, `memberReqStopWait`. `TestShutdownWaitsForMemberReqJobs` failed on both subtests (a job still running at the database-close probe; a message after the drain started a job; `StopMemberReqJobs` reported idle). `TestMemberReqOnePunishmentAcrossReplicas` and `TestMemberReqRunnerSheds` already held at the earlier code (the runner and the claim existed): guards |
| GREEN task 3 | f6407cf | all pass |

`gsd_run check tdd-red-evidence` was not run (the plan is `type: execute`); the evidence is the real test output.

## Verification (real results)

- **Per task:** the task 1 command (four tests) `ok`; the task 2 cache command `ok` and the three module tests `ok`; the task 3 command `ok`. The three AGENTS.md greps: `member requirements watcher` prints 2; the `StopMemberReqJobs` bullet contains `65 s`, `RegisterMemberReqDrain`, `stop_grace_period: 75s`, `database close`, `costs no correctness` and `memberReqJobTimeout`; `memberReqEnforcementLive` 2, `memberReqLive()` 1, `basic group is never screened` 1.
- **Family run at plan end** (background, output in the scratchpad): `go test -tags testtools -race -count=1 -run '^(TestMemberReq|TestShutdown|TestRegistered|TestLoadAllModules|TestRaidFlood|TestAISpam|TestAntiflood|TestBlacklist)' ./fuku/modules` gave `ok fuku/modules 55.335s`, `family exit=0`. `go test -tags testtools -race -count=1 ./fuku/db/cache` gave `ok`.
- **Settings family, run once** because the plan adds a handler to the update pipeline and edits the shared `local.go`: `go test -tags testtools -race -count=1 -run '^(TestSettings|TestLoadSettings)' ./fuku/modules ./fuku/i18n` gave `ok fuku/modules 299.835s`, `ok fuku/i18n 5.495s`, `settings exit=0`.
- **Flake loop:** the new concurrent and timing tests (`TestMemberReqOnePunishmentAcrossReplicas`, `TestMemberReqRunnerSheds`, `TestShutdownWaitsForMemberReqJobs`, `TestMemberReqWatcher*`, `TestMemberReqPassCache`, `TestMemberReqCleanEndsWarnHold`) ran 25 times in a row from the compiled race-enabled test binary: 25 of 25 passed (50 of 50 for the three tracer tests after the warn-ordering fix).
- `gofmt -l` on every file I touched: empty (`main.go`, `error_logging.go`, `formatting.go`, `help.go`, `overwrite.go`, `blacklists_command_test.go`, `callback_codec.go`, `deeplink_router.go` are unformatted at base; `main.go`'s only finding is a pre-existing `Scope:` alignment at line 517, not mine). `go vet -tags testtools ./fuku/modules ./fuku/db/cache`: clean. `CGO_ENABLED=0 go build ./...`: ok (this builds `main.go`). `make check-translations`: "All translations are present!". `make check-docs`: "No drift detected". `git diff --exit-code 8c0c827 -- go.mod go.sum`: exit 0.
- **Lint stand-in:** the committed `lint-local.py` crashes with `FileNotFoundError: ''` as soon as `main.go` (a root-level file) has added lines (`os.listdir('')`). I ran a scratchpad copy with only `os.listdir(d or ".")` changed (`.../scratchpad/lint-local-fixed.py 8c0c827`): exit 0 after every GREEN commit, "no dupl, gocyclo or godox findings on lines added since 8c0c827". The shared script was left untouched; 13-10 onward will hit the same crash when they edit a root file and may want the one-line fix. `make lint` and gosec cannot run here; no new code assigns a string literal to a credential-like name and none uses math/rand.
- Not run: `make lint`, `make test` (only 13-14), PostgreSQL (nothing here rests on a PostgreSQL-only guarantee; the `fuku/modules` harness runs on SQLite).

## Deviations from Plan

### Auto-fixed Issues

None needed for rules 1 to 3 beyond the points below.

### Judgment calls

- **Rule order: pure rules before the pass GET.** The plan's text and research S-02 read the pass value before the rules; I compute `memberReqBroken` first (microseconds) and read the pass value only for a clean member, so a violator makes no Redis call in the watcher. Behaviour is identical.
- **Rule 2: stale-message guard on the pass value.** `memberReqPassMaxAge` (2 minutes): a message older than that still runs the clean predicate but writes no pass value, so a backlog message processed after downtime cannot hide a later hold from the predicate for the whole 30 minutes (`TestMemberReqPassCache`, "a message much older than now never writes the verdict"). Documented in AGENTS.md.
- **Named, preset result of the watcher.** `RecoverFromPanic` swallows a panic and the function would return nil, which this gotgbot reads as "leave the group"; the preset named result keeps `ext.ContinueGroups`. The acceptance grep (no `return nil` or `ext.EndGroups` in the body) holds.
- **`memberReqMaxInFlight` and `memberReqStopWait` are variables** (tests set them); `memberReqJobTimeout` stays a constant. The cap check uses a new method `raidPostTracker.startBelow` (in `memberreq_watch.go`) so the check and the increment are one locked step; `raid_trigger.go` is untouched.
- **SQLite artifact in the warn tests.** The warn module's transaction reads and then writes; on this suite's SQLite a concurrent writer (a concurrent `/rules` command, or another job's no-op claim) made it fail with `database is locked` in about 1 run of 12. `TestMemberReqWatcherPunishesOnce` therefore judges its first message alone and sends `/rules` and the rest afterwards; the warn variant of `TestMemberReqOnePunishmentAcrossReplicas` does the same, and the claim race itself is proved fully concurrently with a Mute rule (no warn transaction). PostgreSQL waits instead, so production is not affected. Recorded in AGENTS.md under Testing.
- **Tests enumerating modules or handler groups:** none needed an edit (`TestRegisteredModulesRespectGotgbotDefaultGroupSemantics`, `TestLoadAllModules`, the help registry and docs generation all pass unchanged; `make generate-docs` was not needed, `check-docs` is clean).
- **`memberReqCleanup`** gained one line deleting the rules rows of its chats (the `/rules` command in the watcher test creates one); the `fuku:memberreq:pass:*` keys were already covered by its substring match on the chat ID.
- **Plan's lint path** under phase 11 does not exist; the Phase 13 stand-in (with the fix above) was used.

No Rule 4 changes were needed.

## Requirement coverage

`requirements-completed: [MREQ-13]`. MREQ-13 is complete: the Russian, Arabic and Chinese rules check first and last names at every message (13-02's classifier, 13-05's page hints, this plan's watcher), an invisible last name counts as none. The live switch stays off until 13-13, which the SUMMARY states so the requirement reads as "implemented and proven behind the switch". MREQ-13's checkbox and traceability row are ticked in `.planning/REQUIREMENTS.md` (`git diff` shows only those two lines). MREQ-02, MREQ-04, MREQ-05 and MREQ-09 stay Pending: 13-11 completes MREQ-02, MREQ-05 and MREQ-09 and 13-12 completes MREQ-04. `requirements.mark-complete` was not run.

## First callers (what a later plan wires)

- `memberReqLive()`: this watcher; the join entry of 13-11 and the captcha guard of 13-12 check it too. **13-13 is the only plan that sets `memberReqEnforcementLive`.**
- `memberReqClean` / `memberReqPassHit`: only the watcher. 13-10 adds the delete and the per-chat cap and pause on the job path.
- `memberReqRunAsync`: only the watcher; `StopMemberReqJobs` through `RegisterMemberReqDrain` in `main.go`.

## Known Stubs

None.

## Threat Flags

None beyond the plan's register. T-13-29 and T-13-56 (the switch) by the atomic switch and `TestMemberReqWatcherOffUntilLive`; T-13-57 by `TestMemberReqRunnerSheds` (90 s deadline, 30 s margin to `StaleClaim`); T-13-30 by `TestMemberReqWatcherContinuesGroups`; T-13-31 by the cap of 32 and the shed log; T-13-32 by `TestMemberReqOnePunishmentAcrossReplicas` and `TestSkipLocalMemberRequirementsKey`; T-13-33 by `TestMemberReqWatcherExemptions`; T-13-34 by the documented 65 s worst case; T-13-SC by the unchanged `go.mod` and `go.sum`.

## Notes for later plans

- STATE.md "Plan:" reads `10 of 14`; ROADMAP.md was not marked complete.
- A test that starts a job through the dispatcher must end with the runner idle: `newMemberReqEnv` registers `memberReqJobs.wait` as a cleanup. A test that needs a job to be "in flight" holds it in `fake.setOnGetMember` for that member and waits for the hook to be entered before acting (a chat-wide `setDelay` also stalls the update goroutine inside other handlers' own lookups).

## Self-Check: PASSED

- Files exist: `fuku/modules/memberreq.go`, `fuku/modules/memberreq_watch.go`, `fuku/modules/memberreq_watch_test.go`.
- Commits `5ad1102`, `c8ba945`, `9a51e42`, `81bd161`, `ff28d16`, `f6407cf` are ancestors of HEAD.
