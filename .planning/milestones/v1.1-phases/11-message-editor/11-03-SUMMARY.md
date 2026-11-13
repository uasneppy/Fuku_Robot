---
phase: 11-message-editor
plan: 03
subsystem: database
tags: [greetings, rules, repository, message-editor, tdd, gorm]
requires:
  - phase: 11-message-editor
    provides: plan 11-RESEARCH S-05 (side-effect-free reads, column-targeted writes)
provides:
  - greetings.LoadSettingsContext, a read that creates nothing, returns errors and shows the defaults the bot acts on
  - rules.LoadRulesContext, a read that creates nothing and returns errors
  - SetWelcome/Goodbye TextOnly, MediaOnly and ButtonsOnly, column-targeted one-part writes
affects: [11-08, 11-09, 11-10, 11-13, settings-message-editor]

actuals:
  tokens: 5100
  tasks: 2
  commits: 3

commits: 3
plan_head_before: 09908a61a28c7842bcd85b4f407e6f576ca4b907
plan_head_after: c3517a2ce0f8ad3a6d2dece7450b0ef4b50f88b9

tech-stack:
  added: []
  patterns:
    - "A settings page reads through a context-taking Load*Context that never writes and returns every error; the commands keep their create-on-read getters"
    - "One-part writes go through the existing upsertGreetingSettings with a column-targeted updates map, so the cache key is invalidated in one place"

key-files:
  created:
    - fuku/db/greetings/editor_repository_test.go
    - fuku/db/rules/editor_repository_test.go
  modified:
    - fuku/db/greetings/repository.go
    - fuku/db/rules/repository.go

key-decisions:
  - "LoadSettingsContext checks the chats table with its own error-returning Count rather than db.ChatExistsContext, which turns a database error into 'not found' and would show Goodbye off after a failed read."
  - "The RED commit carries six no-op stub writers so the test compiles and fails on its assertions (a build failure is not a valid RED); the GREEN commit replaces them."

requirements-completed: []

duration: 12min
completed: 2026-11-04
status: complete

coverage:
  - id: D1
    description: "LoadSettingsContext reads without creating a row, returns read errors, and its no-row defaults (Goodbye on for a known chat, off for an unknown one) equal what GetGreetingSettings ends up with"
    requirement: EDIT-01
    verification:
      - kind: unit
        ref: "fuku/db/greetings/editor_repository_test.go#TestLoadGreetingSettingsCreatesNothing"
        status: pass
      - kind: unit
        ref: "fuku/db/greetings/editor_repository_test.go#TestLoadGreetingSettingsMatchesRuntime"
        status: pass
    human_judgment: false
  - id: D2
    description: "LoadRulesContext returns empty text for no row without creating a rules or chats row, and returns errors"
    requirement: EDIT-01
    verification:
      - kind: unit
        ref: "fuku/db/rules/editor_repository_test.go#TestLoadRulesCreatesNothing"
        status: pass
    human_judgment: false
  - id: D3
    description: "The six one-part writers change only their own columns, leave the other side alone and round-trip buttons, and a first write creates the row with the database defaults"
    requirement: EDIT-11
    verification:
      - kind: unit
        ref: "fuku/db/greetings/editor_repository_test.go#TestGreetingColumnWritersKeepOtherParts"
        status: pass
    human_judgment: false
---

# Phase 11 Plan 03: Editor reads and one-part writes Summary

**Side-effect-free, error-returning reads (`greetings.LoadSettingsContext`, `rules.LoadRulesContext`) and six column-targeted one-part greeting writers, so the editor can show and save a welcome, goodbye or rules part without creating rows, hiding read errors or touching the other parts.**

## Performance

- **Duration:** 12 min
- **Started:** 2026-11-04T16:47:00Z
- **Completed:** 2026-11-04T16:59:00Z
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments

- `LoadSettingsContext(ctx, chatID)` reads with the caller's context, never writes, and on no row returns the defaults the bot ends up with: Welcome on, Delete last off, default texts, Goodbye ON for a chat in the chats table (the database default the first write stores) and off for an unknown chat. Any other error, including a cancelled context, is returned with no settings. A stored empty text reads back as the default text.
- `LoadRulesContext(ctx, chatID)` returns `""` and no error for no row, creates neither a rules row nor a chats row, and returns every other error.
- `SetWelcomeTextOnly`, `SetWelcomeMediaOnly`, `SetWelcomeButtonsOnly` and the Goodbye twins each go through `upsertGreetingSettings` with only their own columns, so the other parts stay and the `greetings:<chat>` key is invalidated as the commands' writes do. `SetWelcomeText` and `SetGoodbyeText` are unchanged.

## Task Commits

1. **Task 1 (tracer): side-effect-free reads** - `e1eb482` (feat)
2. **Task 2 (TDD): one-part writers**
   - RED - `0ca95b2` (test)
   - GREEN - `c3517a2` (feat)

No REFACTOR commit (nothing to clean up).

## TDD Gate Compliance

- **RED:** `TestGreetingColumnWritersKeepOtherParts` with no-op stub writers. It failed on its assertions (text stayed `<b>a</b>` after `SetWelcomeTextOnly(chat, "x")`, for both sides, and a first write stored no text). `gsd_run check tdd-red-evidence` returned `RED_EVIDENCE_OK` (`target_test_failed`). Semantic assessment: the target ran and failed on the planned "only the targeted columns change" assertion, not on a build, fixture or import fault.
- **GREEN:** the six real writers; the same test passes.
- Commit order: `test(11-03)` `0ca95b2` precedes `feat(11-03)` `c3517a2`.

Task 1 is a `type="tracer"` task: its tests were written together with its code (new symbols, so a separate RED would be a build failure, which is not valid RED). Tracer gate: `<verify>` re-run end to end, passed, expansion continued.

## Files Created/Modified

- `fuku/db/greetings/repository.go` - `LoadSettingsContext`, `defaultGreetingSettingsView`, six `Set*Only` writers
- `fuku/db/greetings/editor_repository_test.go` - the three greetings tests
- `fuku/db/rules/repository.go` - `LoadRulesContext`
- `fuku/db/rules/editor_repository_test.go` - `TestLoadRulesCreatesNothing`

## Decisions Made

- The chats-table check inside `LoadSettingsContext` is its own `Count` on `models.Chat` under the caller's context, because `db.ChatExistsContext` returns false on any error; using it would turn a failed read into "Goodbye off" (the plan's Pitfall 1).
- `checkGreetingSettings` is left alone (the commands keep their exact behaviour); `LoadSettingsContext` has its own small defaults builder instead of refactoring it.

## Deviations from Plan

None - plan executed exactly as written. (The RED stubs are the plan's own "write it first and see it fail" made compilable; documented under TDD Gate Compliance.)

**Total deviations:** 0.

## Verification (real results)

- `go test -tags testtools -race -count=1 -v -run '^(TestLoadGreetingSettingsCreatesNothing|TestLoadGreetingSettingsMatchesRuntime)$' ./fuku/db/greetings`: both PASS.
- `go test -tags testtools -race -count=1 -v -run '^TestLoadRulesCreatesNothing$' ./fuku/db/rules`: PASS.
- `go test -tags testtools -race -count=1 ./fuku/db/greetings ./fuku/db/rules`: ok, ok (includes `TestSetWelcomeText`, `TestSetGoodbyeText`, `TestResetWelcomeText`, `TestGetGreetingSettings_Defaults`, all PASS).
- `grep -cE 'func Set(Welcome|Goodbye)(Text|Media|Buttons)Only\(' fuku/db/greetings/repository.go` = 6 (baseline 0); `grep -c 'upsertGreetingSettings(chatID'` = 15 (72a692b: 9, so +6).
- `grep -c 'func LoadSettingsContext('` = 1; `grep -c 'func LoadRulesContext('` = 1; no `CreateRecord`, `EnsureChatInDb` or `FirstOrCreate` inside either function.
- `CGO_ENABLED=0 go build ./...`: ok. `git diff --exit-code 72a692b -- go.mod go.sum`: exit 0. `gofmt -l` on touched dirs: clean. `go vet -tags testtools ./fuku/db/greetings ./fuku/db/rules`: clean.
- Not run: `make lint` (installed golangci-lint is built with Go 1.25 and refuses Go 1.26.0), `make test` (not asked by this plan), PostgreSQL (nothing here needs it).

## Requirements

`requirements-completed: []`. The plan contributes to EDIT-01 and EDIT-11 but completes neither (EDIT-01 is completed by 11-09, EDIT-11 by 11-13), so both stay Pending in REQUIREMENTS.md.

## Issues Encountered

None.

## Known Stubs

None. (The RED-phase stubs were replaced in the GREEN commit.)

## Threat Flags

None. No new endpoint, auth path or schema; T-11-07, T-11-08 and T-11-09 are mitigated by the tests named above.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

Plans 11-08 and later can read a group's welcome, goodbye and rules for a page without writes and can save one part at a time. The reads are uncached and nothing reads `greetings:<chat>`, so no `skipLocal` entry is needed.

---
*Phase: 11-message-editor*
*Completed: 2026-11-04*
