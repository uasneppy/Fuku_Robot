---
phase: 12-rules-media-media-under-text-and-greet-once
plan: 03
subsystem: database
tags: [greet-once, claim, column-targeted-writers, rules-media, group-language]
requires:
  - phase: 12-rules-media-media-under-text-and-greet-once
    provides: 12-02 columns, models (GreetedMember, MediaBelow, Once, RulesSettings media fields) and HasContent
provides:
  - greetings.ClaimGreeting and greetings.ReleaseGreeting (greeted_members, one INSERT ... ON CONFLICT DO NOTHING)
  - greetings.SetWelcomeMediaBelow, SetGoodbyeMediaBelow and SetWelcomeOnce (column-targeted, cache key deleted)
  - rules.LoadRulesSettingsContext (whole row, creates nothing) with LoadRulesContext as a wrapper
  - rules.SetRulesMediaOnly, SetRulesButtonsOnly, SetRulesMediaBelow and ClearRules
  - lang.GetGroupLanguage (cached chat_lang read by chat ID)
affects: [12-08, 12-09, 12-10, 12-11, 12-12, 12-14]

actuals:
  tokens: 8100
  tasks: 3
  commits: 7

commits: 7
plan_head_before: e7d071696911147c9e991067a2df42a5068cbfdf
plan_head_after: 6ac15247073c56edb5473b66f5e73504ea6d3606

tech-stack:
  added: []
  patterns:
    - "A red commit carries a compile-only stub so the target test fails on its assertion, not on a build error"
    - "Cache-key deletion proved by seeding the key in miniredis and asserting it is gone after the write"

key-files:
  created:
    - fuku/db/greetings/greeted.go
    - fuku/db/greetings/greeted_test.go
  modified:
    - fuku/db/greetings/repository.go
    - fuku/db/greetings/editor_repository_test.go
    - fuku/db/rules/repository.go
    - fuku/db/rules/editor_repository_test.go
    - fuku/db/lang/repository.go
    - fuku/db/lang/repository_test.go

key-decisions:
  - "The four rules writers share one private helper, updateRulesColumns (checkRulesSetting, then UpdateRecordWithZeroValues), so they stay under the dupl threshold and each is a one-line call"
  - "TestGreetedMemberClaimConcurrent runs on whatever database internal/testdb.Run provides and has no t.Skip, the way the fuku/db/lockdown tests run: SQLite here, PostgreSQL in the integrity target"

patterns-established:
  - "useMiniredisCache (greetings tests) wires the real cache stack for one test so a write's DeleteCache can be observed"

requirements-completed: []

coverage:
  - id: D1
    description: "ClaimGreeting records a member once; of 16 concurrent claims exactly one wins; a release lets the member be claimed again and a claim in another chat is independent"
    requirement: "EDIT-09"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestGreetedMemberClaim|TestGreetedMemberClaimConcurrent)$' ./fuku/db/greetings (SQLite)"
        status: pass
      - kind: unit
        ref: "TestGreetedMemberClaimConcurrent on PostgreSQL (CI integrity run once 12-16 adds it)"
        status: not-run
    human_judgment: false
  - id: D2
    description: "The three switch writers change only their column, write a Go false, and delete the greetings cache key"
    requirement: "EDIT-07"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestSetMediaBelowColumnTargeted$' ./fuku/db/greetings"
        status: pass
    human_judgment: false
  - id: D3
    description: "The rules row is read whole without creating a row, and the media, buttons, media-below and clear writers are column-targeted and store zeroes"
    requirement: "EDIT-08"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestLoadRulesSettingsCreatesNothing|TestRulesMediaWriters|TestClearRulesStoresZeroes|TestSetMediaBelowColumnTargeted)$' ./fuku/db/rules"
        status: pass
    human_judgment: false
  - id: D4
    description: "GetGroupLanguage reads the group's language by chat ID like GetLanguage in a group, falls back to uk and creates no row"
    requirement: "EDIT-08"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestGetGroupLanguage$' ./fuku/db/lang"
        status: pass
    human_judgment: false

duration: 30min
completed: 2026-11-05
status: complete
---

# Phase 12 Plan 03: Greet-once claim, column-targeted writers, whole-row rules loader and group language Summary

**`ClaimGreeting` gives exactly one winner per first welcome across concurrent claims, every new switch and rules part has its own column-targeted writer, a rules row reads whole without being created, and `GetGroupLanguage` names a group's language by chat ID.**

## Performance

- **Duration:** about 30 min
- **Tasks:** 3 (a tracer and two TDD tasks)
- **Commits:** 7 task commits (3 red, 3 green, 1 test follow-up) plus the docs commit
- **Files:** 2 created, 6 modified

## Accomplishments

- `greeted.go`: `ClaimGreeting` is `db.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&GreetedMember{...})` and reports `RowsAffected == 1`; an error is wrapped and returns false. `ReleaseGreeting` deletes the one row and treats an absent row as success. Both read and write the database fresh: no cache, no Redis, no `claimRecentJoinProcessing`.
- `greetings/repository.go`: `SetWelcomeMediaBelow`, `SetGoodbyeMediaBelow`, `SetWelcomeOnce`, each one map through `upsertGreetingSettings` (so a Go false is written and `greetings:<chat>` is deleted).
- `rules/repository.go`: `LoadRulesSettingsContext` (zero row for a chat with none, creates nothing, returns read errors), `LoadRulesContext` now a wrapper over it, `SetRulesMediaOnly`, `SetRulesButtonsOnly`, `SetRulesMediaBelow` and `ClearRules` (text, file ID, media type and buttons together, leaving `rules_btn`, `private` and `media_below` alone, Assumption A3). Rules reads are uncached, so no key exists to delete; the GoDoc of each writer says so.
- `lang/repository.go`: `GetGroupLanguage(ctx, groupID)` returns `getGroupLanguageContext`, the cached `chat_lang` read `GetLanguage` makes in a group.

## TDD record

Each of tasks 1, 2 and 3 went red then green in separate commits. A Go test cannot compile against a symbol that does not exist, and a build failure is an INVALID_RED by `tdd.md`, so every red commit also carries a compile-only stub that returns the zero value (`false, nil`, `nil`, an empty row, `""`). The target tests then execute and fail on their planned assertions, and the green commit replaces the stub.

- **Task 1 red (`4bfd152`):** `TestGreetedMemberClaim` failed with "first claim of a member should win" and `TestGreetedMemberClaimConcurrent` with "winners = 0, losers = 16, want 1 and 15". Semantic assessment: both targets ran and failed on the planned assertion because the stub never claims. Green: `297a88d`, both PASS.
- **Task 2 red (`4b6f0c1`):** the three `TestSetMediaBelowColumnTargeted/*` writer subtests failed with "write(true) left the greetings cache key behind" and "expected one greetings row, got 0"; in rules `TestLoadRulesSettingsCreatesNothing` (stored row, cancelled context), all five `TestRulesMediaWriters` subtests, `TestClearRulesStoresZeroes` and rules `TestSetMediaBelowColumnTargeted` failed on their assertions. The "no row reads a zero row" and "LoadRulesContext still returns the text" subtests passed against the stub, as they characterise behaviour the stub also has. Semantic assessment: the failures are the planned assertions (the stub writes nothing). Green: `249f3b9`.
- **Task 3 red (`b9a0093`):** all five `TestGetGroupLanguage` subtests failed on `GetGroupLanguage() = "", want ...` (the stub returns an empty string). Green: `5621811`.
- **Follow-up (`6ac1524`):** the lint stand-in's errcheck found an unchecked `mr.Set` in the greetings test; the error is now checked.
- `gsd_run check tdd-red-evidence` was not run (the plan is `type: execute`, no gate is enforced, and the evidence is a console report in this session); the semantic check above was done by reading the `--- FAIL` lines.

## Task Commits

1. **Task 1 (tracer) red:** `4bfd152` (test)
2. **Task 1 green:** `297a88d` (feat) - claim a member's first welcome exactly once
3. **Task 2 red:** `4b6f0c1` (test)
4. **Task 2 green:** `249f3b9` (feat) - write each switch and rules part on its own
5. **Task 3 red:** `b9a0093` (test)
6. **Task 3 green:** `5621811` (feat) - read a group's language by its chat ID
7. **Follow-up:** `6ac1524` (test) - check the error of seeding the greetings cache key

## Verification (real results)

- `go test -tags testtools -race -count=1 -run '^(TestGreetedMemberClaim|TestGreetedMemberClaimConcurrent)$' -v ./fuku/db/greetings`: both `--- PASS`, `ok`.
- `go test -tags testtools -race -count=1 -run '^TestSetMediaBelowColumnTargeted$' -v ./fuku/db/greetings ./fuku/db/rules`: PASS in both packages.
- `go test -tags testtools -race -count=1 -run '^(TestLoadRulesSettingsCreatesNothing|TestRulesMediaWriters|TestClearRulesStoresZeroes|TestLoadRulesCreatesNothing|TestRulesCRUD)$' -v ./fuku/db/rules`: all five PASS.
- `go test -tags testtools -race -count=1 -run '^(TestGetGroupLanguage|TestGetLanguageFromPrivateAndGroupContexts|TestLanguageCRUD)$' -v ./fuku/db/lang`: all three PASS.
- `go test -tags testtools -race -count=1 ./fuku/db/greetings ./fuku/db/rules ./fuku/db/lang`: `ok` for all three.
- `CGO_ENABLED=0 go build ./...`: exit 0 (so every existing caller of `LoadRulesContext` still compiles).
- `go vet -tags testtools` on the three packages: clean. `gofmt -l` on the three packages: nothing (it lists unrelated untouched files elsewhere under `fuku/db`, pre-existing).
- `python3 .../lint-local.py 72a692b`: exit 0 ("no dupl, gocyclo or godox findings"), run before the last code commit and again before this SUMMARY. The first run found one errcheck (fixed in `6ac1524`).
- `git diff --exit-code 72a692b -- go.mod go.sum` and against `bd2f794`: exit 0.
- Acceptance greps: `OnConflict{DoNothing: true}` appears once in `greeted.go` and no code line mentions redis or `DeleteCache(`; no `t.Skip` in `greeted_test.go`; all eight new functions exist.
- Not run: `make lint` and `make test` (cannot run here, the plan does not ask for them), the settings test family (not asked), `TestGreetedMemberClaimConcurrent` on PostgreSQL (CI only).

## Deviations from Plan

- **[Rule 3 - process] Compile-only stubs in the red commits**, as described under the TDD record. Each stub was replaced in its green commit; no stub ships.
- **PostgreSQL-only skip not used for `TestGreetedMemberClaimConcurrent`.** The caller's note asked for a local skip "like the repo's other PostgreSQL tests". The plan (and `scripts/check_test_results`, which refuses a skip outside `fuku/db/migrations`) say the opposite, and `fuku/db/lockdown`'s equivalent tests have no skip: they run on SQLite here and on PostgreSQL in `make test-postgres-integrity`. I followed the plan and the lockdown pattern: the test runs and passes here on SQLite (WAL, `_busy_timeout=10000`), and the PostgreSQL run happens in CI once 12-16 adds it and `./fuku/db/greetings` to the integrity target. It has not run on PostgreSQL.
- **Rules writers share `updateRulesColumns`** instead of repeating the `checkRulesSetting` plus `UpdateRecordWithZeroValues` pair four times, to stay under `dupl`.

## Known Stubs

None. The stubs of the red commits were replaced in the green commits.

## Unused-function note

The eight new exported functions have no production caller yet; later plans of this phase are their only users (12-09: `ClaimGreeting`, `ReleaseGreeting`; 12-08, 12-10, 12-11, 12-12: the writers; 12-12 and 12-14: `LoadRulesSettingsContext`, `GetGroupLanguage`). Each is called by a test of this plan, and the lint stand-in's `unused` check reports nothing.

## Threat Flags

None beyond the plan's register. Mitigations in place: T-12-07 (primary key plus ON CONFLICT DO NOTHING and RowsAffected == 1, `TestGreetedMemberClaimConcurrent`; PostgreSQL run pending in CI), T-12-08 (absent-row release returns nil, `TestGreetedMemberClaim`), T-12-09 (`TestGetGroupLanguage` asserts no chats row is created), T-12-10 (column-targeted map writes, tests compare every other column and the cache key), T-12-SC (go.mod and go.sum unchanged).

## Requirement coverage

`requirements-completed: []`. The plan contributes the switch writers to EDIT-07, the rules loader and writers to EDIT-08 and the claim to EDIT-09 and completes none; all three stay Pending in `.planning/REQUIREMENTS.md`.

## Notes for later plans

- 12-09 owns the 429 and refusal rules around the claim; this repository only inserts and deletes. Its AGENTS.md bullet for `greeted_members` still says the claim rule arrives with 12-09 and should be extended in place.
- 12-16 must add `TestGreetedMemberClaimConcurrent` to the `-run` list of `make test-postgres-integrity` and `./fuku/db/greetings` to its package list.
- `ClearRules` leaves `media_below` as it was (Assumption A3); 12-12 wires `/clearrules` to it.

## Self-Check: PASSED

- `fuku/db/greetings/greeted.go` and `greeted_test.go` exist.
- Commits `4bfd152`, `297a88d`, `4b6f0c1`, `249f3b9`, `b9a0093`, `5621811` and `6ac1524` are ancestors of HEAD.
