---
phase: 09-settings-menu
plan: 03
subsystem: antiflood
tags: [antiflood, settings-menu, apply-functions, characterization-test, gorm, go]

requires:
  - phase: 06-raid-detection
    provides: antiflood commands and repository the apply functions wrap
provides:
  - applyFloodLimit, applyFloodMode and applyFloodDelete with an uncached database read-back
  - floodApplyOutcome, floodApplyResult, floodModes, floodOffWords
  - characterization of the first antiflood write on a chat with no row (stored flood_limit is 5)
affects: [09-09 Antiflood settings page, 09-13 docs]

actuals:
  tokens: 6000
  tasks: 2
  commits: 3
plan_head_before: 5df191f8683da0eca1c46c354afc11ad23532f77
plan_head_after: 3cc562a09a86cd87520784ccda3878a46c8dd19a

tech-stack:
  added: []
  patterns:
    - "apply function per setting: validate, repository write (does its own DeleteCache), uncached read-back, typed result"

key-files:
  created:
    - alita/modules/antiflood_apply_test.go
  modified:
    - alita/modules/antiflood.go
    - alita/db/antiflood/repository_test.go

key-decisions:
  - "The repository and the model are untouched (D-27); the first-write side effect is reported by a read-back, not fixed"
  - "A numeric 0 that is not an off word (00, -0) keeps its old reply, antiflood_errors_set_in_limit, so no command reply changes"
  - "Command replies stay keyed on the lower-cased argument; the apply functions take the caller's already normalised value"

patterns-established:
  - "floodApplyResult{Outcome, Stored, Err}: the page shows Stored, never an assumed value"

requirements-completed: [MENU-07, MENU-08]

coverage:
  - id: D1
    description: "A characterization test pins what the first mode or delete write on a chat with no antiflood row stores (flood_limit 5), and that a Mute write on a fresh chat writes no row"
    requirement: "MENU-07"
    verification:
      - kind: unit
        ref: "alita/db/antiflood/repository_test.go#TestSetFloodModeOnFreshChatStoredLimit"
        status: pass
      - kind: unit
        ref: "alita/db/antiflood/repository_test.go#TestSetFloodMsgDelOnFreshChatStoredLimit"
        status: pass
    human_judgment: false
  - id: D2
    description: "applyFloodLimit, applyFloodMode and applyFloodDelete validate, write, read the row back uncached and return a typed result; invalid input writes nothing"
    requirement: "MENU-08"
    verification:
      - kind: unit
        ref: "alita/modules/antiflood_apply_test.go#TestApplyFloodLimit"
        status: pass
      - kind: unit
        ref: "alita/modules/antiflood_apply_test.go#TestApplyFloodMode"
        status: pass
      - kind: unit
        ref: "alita/modules/antiflood_apply_test.go#TestApplyFloodDelete"
        status: pass
    human_judgment: false
  - id: D3
    description: "/setflood, /setfloodmode and /delflood (and /clearflood) call the apply functions and keep every reply key and return value"
    requirement: "MENU-08"
    verification:
      - kind: unit
        ref: "alita/modules/antiflood_apply_test.go#TestAntifloodCommandsUseApply"
        status: pass
      - kind: unit
        ref: "alita/modules/antiflood_command_test.go#TestAntifloodCommandsUpdateSettingsAndDisplay"
        status: pass
      - kind: unit
        ref: "alita/modules/antiflood_command_test.go#TestAntifloodCommandsHandleDisabledAndValidationBranches"
        status: pass
    human_judgment: false

duration: 25min
completed: 2026-10-30
status: complete
---

# Phase 09 Plan 03: Antiflood apply functions Summary

**One validating, read-back apply function per antiflood setting (limit, action, delete switch), shared by the commands now and by the 09-09 page later, with the repository's first-write behaviour (flood_limit 5) pinned by test.**

## Performance

- **Duration:** about 25 min
- **Tasks:** 2 of 2
- **Files:** 3 (1 created, 2 modified)
- **Commits:** 3 code commits (`fef0c15`, `593942e`, `3cc562a`), then this SUMMARY

## Accomplishments

- Task 1 settled research assumption A1: on a chat with no antiflood row, the first `SetFloodModeContext(ban)` or `SetFloodMsgDelContext(true)` stores `flood_limit` **5**, the model's `default:5`, because the `FirstOrCreate` insert leaves the limit at its Go zero value. An admin who only picks an action or turns on message deleting therefore also turns antiflood on at 5 messages. `SetFloodModeContext(mute)` on a fresh chat writes no row (the cached default is already mute), so a Mute press on an unconfigured chat changes nothing. The repository, `optimized.go` and the model are untouched (D-27).
- Task 2 added `applyFloodLimit` (0 off, 3..100 set, else out of range and no write), `applyFloodMode` (ban, kick, mute exactly; warn, tban, tmute, wrong case and empty are `floodUnknownMode`) and `applyFloodDelete`. Each writes through the repository (which does its own `DeleteCache`), then reads the row back with `antiflood.GetAntifloodSettingsContext` (uncached) and returns `floodApplyResult{Outcome, Stored, Err}`. A failed read-back is logged at Warn and leaves `Stored` nil; a repository error is `floodSaveFailed` with `Err` set.
- `setFlood`, `setFloodMode` and `setFloodDeleter` now call them. The `IsUserConnected(b, ctx, true, true)` gate (3 occurrences), every reply key, formatting and return value are unchanged, pinned by `TestAntifloodCommandsUseApply` with a per-key marker locale (`withStaffLocale`), plus the two pre-existing command tests, unedited and passing.

## Task Commits

1. Task 1: `fef0c15` test(09-03): pin what the first antiflood write stores
2. Task 2 RED: `593942e` test(09-03): pin the antiflood apply functions
3. Task 2 GREEN: `3cc562a` refactor(09-03): one antiflood apply function per setting, with a database read-back

## Verification

- `CGO_ENABLED=0 go build ./...`: pass
- Task 1 verify (six named tests in `./alita/db/antiflood`): all six `--- PASS`
- Task 2 verify (six named tests in `./alita/modules`, run with `-v`): all six `--- PASS`, sub-tests included
- `go test -run 'Antiflood|Flood' ./alita/modules`: ok (every pre-existing antiflood watcher and command test)
- `go vet -tags testtools ./alita/modules/ ./alita/db/antiflood/`: clean; `gofmt -l` on the touched files: empty; `make check-docs`: no drift
- `make lint` was not run: golangci-lint is built with go1.25 against the go1.26.0 module in this sandbox, so `gofmt` and `go vet` stand in for it.
- Acceptance greps: three `func applyFlood*(` definitions; 6 lines mention the three functions; `IsUserConnected(b, ctx, true, true)` count 3; `git diff` of `repository.go`, `optimized.go` and `models/antiflood.go` is empty.

## Deviations from Plan

**1. [Rule 3 - Blocking] Plan `<automated>` commands name the main checkout**
- **Found during:** Task 1 and 2 verify
- **Issue:** both `<automated>` lines start `cd /home/user/Fuku_Robot && ...`, an absolute path outside this worktree; run as written they would test the main tree and pass without seeing this branch's code (worktree-path-safety step 0c).
- **Fix:** ran the same `go build` / `go test -tags testtools -race -count=1 -v -run '<same pattern>' <same package>` commands from the worktree root, with no `cd`. Not rewritten in the plan file.
- **Commit:** none (execution detail). The plan's `<automated>` text should be made root-relative before it is reused.

**2. [Rule 1 - Bug] The command-reply test needed a real locale**
- **Found during:** Task 2 GREEN run
- **Issue:** in the module test binary the locale manager is not initialised, so `tr.GetString` returned an error and an empty string; asserting reply keys against `""` would have pinned nothing.
- **Fix:** `TestAntifloodCommandsUseApply` installs `withStaffLocale(t)` (every en.yml string replaced by a marker of its own key), so the reply text proves which key was used. Test-only.
- **Files modified:** `alita/modules/antiflood_apply_test.go`
- **Commit:** `3cc562a` (the test file edit landed with the GREEN commit; the RED commit had the same test without the locale line)

**3. Plan trailer vs harness trailer.** Commits end with the harness-provided `Co-Authored-By` and `Claude-Session` lines as the orchestrator instructed, not the model name printed in the plan text.

**Total deviations:** 3 (1 blocking-path, 1 test-only bug, 1 trailer note). **Impact:** none on behaviour or scope.

## Issues Encountered

None.

## Follow-ups

- **First-write side effect (carry into STATE.md):** the repository's first write for a chat with no antiflood row (mode or delete switch) stores `flood_limit` 5, turning antiflood on. This is pinned, not fixed (D-27). A fix (insert with limit 0 on first write) would change `/setfloodmode` and `/delflood` too and is a separate follow-up. Plan 09-09's page must show the `Stored` row from the apply result and may want to say that picking an action on an unconfigured chat enables the default limit. Also note the Mute press on an unconfigured chat is a true no-op.
- **AGENTS.md:** not edited here by design; the fresh-row note is written with the docs in plan 09-13.
- **Pre-existing, not touched:** `alita/modules/greetings_command_test.go` is not gofmt-clean on the base commit.

## Known Stubs

None.

## Threat Flags

None. No endpoint, auth path, file access or schema change was added. T-09-14 (one validation rule for the command and the menu) and T-09-15 (the real stored limit pinned and read back) are mitigated as planned.

## Pending human verification

None. No live Telegram call is involved; the live checks stay deferred to 09-SPIKE.md.

## Next

Ready for the remaining Wave 1 plans; plan 09-09 consumes `applyFloodLimit`, `applyFloodMode` and `applyFloodDelete`.

## Self-Check: PASSED

- `alita/modules/antiflood_apply_test.go` exists; `fef0c15`, `593942e` and `3cc562a` are in `git log` on this branch; the working tree was clean after the last code commit.
