---
phase: 06-automatic-raid-detection
plan: 10
subsystem: raid-detection
tags: [antiraid, lockdown, redis, go, telegram, i18n]

requires:
  - phase: 06-automatic-raid-detection
    provides: autolockdownApply and requireAutoLockdownAuthority (06-05), join-surge and flood triggers (06-06 to 06-09), the migrated join threshold (06-02)
provides:
  - "/antiraid, /raidtime and /raidactiontime as one pointer to /lockdown and /autolockdown"
  - "/autoantiraid as an alias of the join-surge threshold with /autolockdown's live authority"
  - "no join handler at group -5, no expiry poller, no raid-mode temp-bans"
  - "a one-time startup sweep of the retired alita:antiraid:* Redis keys"
affects: [06-13 comment pass and docs regeneration, 06-11, 06-12]

actuals:
  tokens: 56000
  tasks: 2
  commits: 4

plan_head_before: d1ab55c1beee7a89271f564a6bb0167ec3f5d7bb
plan_head_after: 4160d06648c495c125afd5f8888173fd19b66c9b

tech-stack:
  added: []
  patterns:
    - "retired feature = one static pointer command + one-time key sweep + stale-button answer"
    - "sync.Once guarded recover-protected startup goroutine bounded by a context timeout"

key-files:
  created:
    - alita/modules/antiraid_retired_test.go
  modified:
    - alita/modules/antiraid.go
    - alita/modules/callback_nil_handlers_test.go
    - alita/modules/lockdown_isolation_test.go
    - main.go
    - AGENTS.md
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml
  deleted:
    - alita/modules/antiraid_test.go

key-decisions:
  - "An old antiraid button is answered with show_alert and the pointer text with HTML tags stripped and cut to Telegram's 200-character limit, because a callback answer is plain text"
  - "The sweep is started through a package sync.Once; tests mark it done (quietAntiRaidSweep) so a background sweep never touches another test's Redis"
  - "The 20 unused antiraid_ locale keys were removed from all 7 files after a grep showed no Go code reads them"

patterns-established:
  - "Retired commands: one WrapCommand descriptor with no checks and Disableable kept as before"

requirements-completed: [LOCK-15]

coverage:
  - id: D1
    description: "/antiraid, /raidtime and /raidactiontime reply with the pointer and change nothing"
    requirement: LOCK-15
    verification:
      - kind: integration
        ref: "alita/modules/antiraid_retired_test.go#TestAntiRaidRetired/pointers"
        status: pass
    human_judgment: false
  - id: D2
    description: "/autoantiraid sets, turns off and shows the join surge, with /autolockdown's range, live authority and anonymous-admin re-entry"
    requirement: LOCK-15
    verification:
      - kind: integration
        ref: "alita/modules/antiraid_retired_test.go#TestAntiRaidRetired (alias sets the surge, alias authority, alias anonymous admin)"
        status: pass
    human_judgment: false
  - id: D3
    description: "Nobody is temp-banned or restricted by the old raid mode, even with its Redis keys present"
    requirement: LOCK-15
    verification:
      - kind: integration
        ref: "alita/modules/antiraid_retired_test.go#TestAntiRaidRetiredNoBan"
        status: pass
    human_judgment: false
  - id: D4
    description: "An old antiraid button is answered once with the pointer and changes nothing; a missing callback query is ignored"
    requirement: LOCK-15
    verification:
      - kind: integration
        ref: "alita/modules/antiraid_retired_test.go#TestAntiRaidRetired/old_button"
        status: pass
      - kind: unit
        ref: "alita/modules/callback_nil_handlers_test.go#TestCallbackHandlersIgnoreMissingCallbackQuery"
        status: pass
    human_judgment: false
  - id: D5
    description: "The retired alita:antiraid:* keys are removed once at startup, bounded and recover-protected"
    requirement: LOCK-15
    verification:
      - kind: integration
        ref: "alita/modules/antiraid_retired_test.go#TestAntiRaidKeySweep"
        status: pass
      - kind: integration
        ref: "alita/modules/antiraid_retired_test.go#TestAntiRaidSweepRunsOnceOnLoad"
        status: pass
    human_judgment: false
  - id: D6
    description: "The translated wording of the pointer and of the rewritten AntiRaid help page reads well in the six non-English languages"
    verification: []
    human_judgment: true
    rationale: "Translations were written by the executor; a native speaker should review them"

duration: 50min
completed: 2026-10-18
status: complete
---

# Phase 6 Plan 10: Retire the old raid mode Summary

**The old raid mode no longer temp-bans anyone: its commands point to /lockdown and /autolockdown, /autoantiraid stays as a join-surge alias behind live authority, and a one-time startup sweep clears the retired alita:antiraid:* Redis keys.**

## Performance

- **Duration:** about 50 min
- **Tasks:** 2 (both TDD)
- **Commits:** 4 task commits (2 test, 2 feat)
- **Files:** 13 changed, 1 deleted (`antiraid_test.go`), 1 created (`antiraid_retired_test.go`)

## Accomplishments

- `antiraid.go` shrank from about 960 lines to about 170: the group -5 join handler, the expiry poller, the Lua scripts, the raid state helpers, `banRaidMember`, the duration parsers and the old commands and callback are gone. The poller's shutdown handler is removed from `main.go`; the LIFO order of the remaining handlers is unchanged.
- `/antiraid`, `/raidtime`, `/raidactiontime` are one `helpers.WrapCommand` pointer (`antiraidRetiredDesc`, Disableable, no checks, no Telegram call). `/autoantiraid` is `autoantiraidDesc` with `/autolockdown`'s checks and anonymous-admin re-entry (`RegisterAnonymousAdminHandler` in `init`); with words it runs `autolockdownApply` with `joins` in front.
- An old `antiraid|` button gets one answer and changes nothing.
- `sweepRetiredAntiRaidKeys` (SCAN `COUNT 100` + `UNLINK`, no Redis returns 0 and no error) starts once per process from `LoadAntiRaid` in a goroutine that begins with `defer error_handling.RecoverFromPanic("sweepRetiredAntiRaidKeys", "AntiRaid")`, under a 30 s context.
- `antiraid_help_msg` rewritten in all 7 locales; `lockdown_antiraid_retired` added; 20 unused `antiraid_*` keys removed from all 7 locales.
- AGENTS.md: group -5 entry gone, `alita:antiraid:*` marked retired, the antiraid join-list and step-aside rules removed, the antiflood/antiraid trap now describes the Redis-only raid counters, and a new bullet states the retirement, the rolling-deploy caveat and that `antiraid_settings` stays for the legacy backup module.

## Task Commits

1. **Task 1 RED** - `8890e4b` (test) - TestAntiRaidRetired and TestAntiRaidRetiredNoBan failed on assertions: 2 `banChatMember` calls from the old handler, wrong replies, no proof prompt for the alias.
2. **Task 1 GREEN** - `813a1d9` (feat)
3. **Task 2 RED** - `d465ca9` (test) - TestAntiRaidKeySweep (3 subtests) and TestAntiRaidSweepRunsOnceOnLoad failed on assertions (sweep removed 0 keys).
4. **Task 2 GREEN** - `4160d06` (feat)

**Plan metadata:** the SUMMARY commit follows this file (`docs(06-10)`).

## TDD Gate Compliance

RED then GREEN commits exist for both tasks. Notes:

- The Task 2 RED commit contains compile scaffolding in `antiraid.go` (a stub `sweepRetiredAntiRaidKeys` returning `0, nil` and the `antiraidSweepOnce` variable), which Go needs for the test to compile. Because of the stub, the "without Redis it returns at once" subtest passed in RED; the other sweep subtests and the once-on-load test failed on their assertions.
- RED evidence was read from the `go test -v` output (the failing lines are quoted above). The `gsd_run check tdd-red-evidence` record was not produced because `gsd_run` is not on PATH in this worktree.

## Files Created/Modified

- `alita/modules/antiraid.go` - pointer commands, alias, retired-button answer, startup sweep.
- `alita/modules/antiraid_retired_test.go` - TestAntiRaidRetired, TestAntiRaidRetiredNoBan, TestAntiRaidKeySweep, TestAntiRaidSweepRunsOnceOnLoad and their helpers.
- `alita/modules/antiraid_test.go` - deleted with the code it tested.
- `alita/modules/lockdown_isolation_test.go` - TestAntiRaidStepsAsideDuringLockdown and the imports only it used removed.
- `alita/modules/callback_nil_handlers_test.go` - the antiraid row now calls `antiraidRetiredCallback`.
- `main.go` - old poller shutdown handler removed.
- `locales/*.yml` (7) - `lockdown_antiraid_retired` added, `antiraid_help_msg` rewritten, 20 keys removed.
- `AGENTS.md` - rules updated in the same commit as the change.

## Decisions Made

- The retired-button answer uses `show_alert` and strips the HTML tags from the pointer text, cutting it to 200 characters, because Telegram shows a callback answer as plain text and rejects one over 200 characters. The plan said only "answered once with the pointer text".
- The sweep goes through a package `sync.Once`; tests mark it done so a background sweep cannot delete keys another test is seeding. `TestAntiRaidSweepRunsOnceOnLoad` resets the Once to prove one sweep per process.
- Old locale keys were removed from all 7 files (the plan allowed removing or keeping) after a grep over `alita/` and `scripts/` showed no Go code or script reads them; only `antiraid_help_msg` is read, by the help system.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing Critical] Pointer text in a callback answer**
- **Found during:** Task 1
- **Issue:** `lockdown_antiraid_retired` contains `<code>` tags and is over 150 characters; a callback answer is plain text and limited to 200 characters, so the raw HTML would show as tags or the answer would fail for a long translation.
- **Fix:** strip tags and cut to 200 runes before answering.
- **Files modified:** `alita/modules/antiraid.go`
- **Committed in:** `813a1d9`

**2. [Rule 2 - Missing Critical] Test isolation for the background sweep**
- **Found during:** Task 2
- **Issue:** `LoadAntiRaid` is called from many tests; the once-per-process goroutine would sweep whichever test's miniredis happened to be live and could delete keys a test had just seeded.
- **Fix:** `quietAntiRaidSweep` in the tests marks the Once done; the only test that wants the sweep resets it.
- **Files modified:** `alita/modules/antiraid_retired_test.go`
- **Committed in:** `d465ca9`

### Additions beyond the listed behaviors

- An extra anonymous-admin subtest for the alias and an extra "more keys than one scan batch" and "Redis failure is returned" subtest for the sweep, plus `TestAntiRaidSweepRunsOnceOnLoad`. They test behavior the plan states (anonymous-admin re-entry, batches, wrapped errors, once per process).

---

**Total deviations:** 2 auto-fixed (both Rule 2). **Impact on plan:** none on scope; both protect correctness.

## Issues Encountered

- The worktree sandbox refused one heredoc and a few commands that use shell variables as arguments; those steps were redone with the Write/Edit tools and plain commands.
- The plan's commit-ledger file (`gsd-plan-head-before-06-10` in the worktree git dir) could not be written from inside the worktree. `plan_head_before` is the spawn base `d1ab55c1...`, which the orchestrator also recorded, and `commits: 4` was measured with `git rev-list --count` against it before the SUMMARY commit.
- `make lint` cannot run here (golangci-lint cannot lint Go 1.26). `gofmt -l alita main.go` prints nothing and `go vet -tags testtools ./alita/modules .` is clean; they stand in for it.
- `alita/modules/greetings_command_test.go` is not gofmt-clean on the base commit; it was not touched.

## Verification Results

- `go test -tags testtools -race -count=1 ./alita/modules ./alita/db/... ./alita/utils/...` passes (the full `alita/modules` package took about 4.5 minutes). The full `make test` was not run, per instructions.
- `CGO_ENABLED=0 go build ./...`, `go vet -tags testtools ./alita/modules .`, `gofmt -l alita main.go`: clean.
- `make check-translations`: all translations present. `make check-docs`: no drift, so plan 06-13's regeneration finds nothing from these locale changes. `go test ./alita/i18n` passes.
- `--version` smoke check: binary built to the scratchpad and run from `/tmp`, prints `v2.23.14`.
- `git diff --exit-code go.mod go.sum`: no change. All acceptance criteria greps of both tasks pass.

## Known Stubs

None.

## Threat Flags

None. No new endpoint, auth path or schema; the only new Redis access is the bounded SCAN/UNLINK of `alita:antiraid:*`, which is in the plan's threat register (T-06-33).

## Notes for Plan 06-13

- Three code comments still name the removed handler or poller: `alita/modules/lockdown.go` (the group -7/-5 comment and the flood watcher comment), `alita/modules/lockdown_guard.go:599`, `alita/modules/raid_flood.go:37` and `alita/modules/staff_sweeper.go:117` (`StopAntiRaidExpiryPoller`). They change no behaviour.
- The docs pages are untouched by this plan; `make check-docs` shows no drift from the locale changes.

## Next Phase Readiness

Plans 06-11 and 06-12 run in parallel in other worktrees and touch no file of this plan. Ready for 06-13.

## Self-Check: PASSED

- Files exist: `alita/modules/antiraid.go`, `alita/modules/antiraid_retired_test.go`, `.planning/phases/06-automatic-raid-detection/06-10-SUMMARY.md`; `alita/modules/antiraid_test.go` is gone.
- Commits exist: `8890e4b`, `813a1d9`, `d465ca9`, `4160d06`.
- STATE.md and ROADMAP.md were not modified.

---
*Phase: 06-automatic-raid-detection*
*Completed: 2026-10-18*
