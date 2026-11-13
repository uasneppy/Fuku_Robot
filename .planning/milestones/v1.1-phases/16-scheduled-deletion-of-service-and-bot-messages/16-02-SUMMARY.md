---
phase: 16-scheduled-deletion-of-service-and-bot-messages
plan: 02
subsystem: modules
tags: [deletion, service-messages, kind-table, apply-functions, reflect-guard, cleanservice]

requires:
  - phase: 16-scheduled-deletion-of-service-and-bot-messages
    provides: "plan 16-01: deletion kind registry, cached rules, SetRule/RemoveRule, ApplyJoinLeftRule, ApplyCleanServiceFlag, the apply-hook test seam"
provides:
  - "deletionRow and the two tables: twelve service rows (sj sl sp sh sn sv sf sb sa sk sg so) with predicates, five bot rows (bw bg br ba bm), presets and locale key names"
  - "deletionServiceKindOf, deletionRowOf, deletionRowOfKind, deletionEffectiveDelay"
  - "applyDeleteRule (every rule but joined and left), applyCleanServiceRule (joined and left, one transaction) and the seams deletionRulesLookup, deletionRuleWrites, deletionFlagLookup"
  - "applyCleanService: the one write path of /cleanservice on and off"
  - "TestServiceKindsCoverMessageFields: a gotgbot upgrade that adds a Message field fails until it is classified"
affects: [16-03, 16-04, 16-05, 16-06, 16-07, 16-08, 16-09]

plan_head_before: d49ee3b5c9ec6136e81c4aec5dcd01033da55151
plan_head_after: 4041fd9da2061eef3831b4f3c38564e5a920cbf0

actuals:
  tokens: 15500   # chars/4 over the added lines of d49ee3b..4041fd9 (62127 chars), tests included
  tasks: 3
  commits: 6      # MEASURED: git rev-list --count d49ee3b..HEAD (before this SUMMARY commit)

tech-stack:
  added: []
  patterns:
    - "one row type for service and bot tables; differences are fields, no branch on a code or kind"
    - "presence predicates as one deletionAnyOf call over plain comparisons (no branches, no dupl clones)"
    - "reflect test with three explicit field lists as the guard for every dependency upgrade"
    - "apply result with Outcome applied/unchanged/refused/save-failed, rules read back through the cached loader"

key-files:
  created:
    - fuku/modules/deletion_kinds.go
    - fuku/modules/deletion_kinds_test.go
    - fuku/modules/deletion_apply.go
    - fuku/modules/deletion_apply_test.go
  modified:
    - fuku/modules/greetings.go
    - fuku/modules/greetings_command_test.go

key-decisions:
  - "applyDeleteRule refuses joined and left with a package-local sentinel (errDeletionSharedKind): the repository had none and a new repository error was not needed"
  - "deletionApplyResult carries CleanService, Delay and On for applyCleanServiceRule so a caller gets the effective state without a second read; they are zero when Rules is nil"
  - "Keep counts as 'already stored' only when the flag is off AND both delay rows are gone; a stale row makes it a write (D-26)"
  - "predicates use len(slice) > 0 for NewChatMembers and NewChatPhoto, so an empty non-nil slice is not a service notice"

patterns-established:
  - "deletionCleanup(t, chats...) test helper: deletes rule and greetings rows, drops both cache keys, ResetLocalForTest, before and after"
  - "deletionCountHook(t, err): deletion.SetApplyHookForTest as a transaction counter or failure injector, no DDL"

requirements-completed: []

coverage:
  - id: D1
    description: "twelve service rows and five bot rows are the single table; no creation, migrate, topic-creation or giveaway-post message is ever a row; every gotgbot Message field is classified exactly once"
    requirement: "DEL-01"
    verification:
      - kind: unit
        ref: "TestServiceKindTable, TestServiceKindsCoverMessageFields, TestDeletionKindRegistryMatchesRepository, TestDeletionKindsNeverStaffLockdownCaptchaLog"
        status: pass
  - id: D2
    description: "one write path for every rule but joined and left, with validation before any read or write, no write for an unchanged state, a typed result"
    requirement: "DEL-02"
    verification:
      - kind: unit
        ref: "TestApplyDeleteRule (row lookups, set, zero, remove, unchanged, refused, save failed, unreadable state)"
        status: pass
  - id: D3
    description: "joined and left read the shared /cleanservice flag; one transaction writes flag and rows; /cleanservice on and off use it and remove both delay rows"
    requirement: "DEL-01"
    verification:
      - kind: unit
        ref: "TestDeletionEffectiveRule, TestApplyCleanServiceRule, TestCleanServiceCommandResetsDelayRows, the existing TestGreeting*/TestCleanService* tests unedited"
        status: pass
  - id: D4
    description: "no kind can name a staff, lockdown, captcha, log or raid message"
    requirement: "DEL-07"
    verification:
      - kind: unit
        ref: "TestDeletionKindsNeverStaffLockdownCaptchaLog"
        status: pass
---

# Phase 16 Plan 02: Kind tables and apply functions Summary

**The twelve service rows and five bot rows as one table with presence predicates, the effective-rule function that reads the shared /cleanservice flag for joined and left, `applyDeleteRule` and `applyCleanServiceRule` as the two write paths, `/cleanservice` moved onto one apply function that also clears the delay rows, and a reflect test that fails when a gotgbot upgrade adds an unclassified Message field.**

## Performance

- **Duration:** about 12 minutes by the commit clock (09:28 to 09:36 UTC; the start time was not recorded with `date`)
- **Tasks:** 3 of 3
- **Files modified:** 6 (4 new, 2 edited)

## Accomplishments

- `fuku/modules/deletion_kinds.go`: `deletionRow` (Code, Kind, NameKey, DescKey, HintKey, Presets, Match), `deletionServiceRows` (12, page order), `deletionBotRows` (5), the two preset lists (service 0, 10, 60, 600, 3600, 21600, 86400; bot 60, 600, 3600, 21600, 86400), `deletionRowOf`, `deletionRowOfKind`, `deletionServiceKindOf`, `deletionEffectiveDelay`. The `so` row lists the 26 remaining deletable fields of the pinned gotgbot (`v2.0.0-rc.36.0.20260919140833-240296efadb4`) in one `deletionAnyOf` call. `ForumTopicCreated`, `Giveaway`, `GroupChatCreated`, `SupergroupChatCreated`, `ChannelChatCreated`, `MigrateToChatId` and `MigrateFromChatId` are in no predicate.
- `fuku/modules/deletion_apply.go`: `applyDeleteRule`, `applyCleanServiceRule`, `deletionApplyResult`, the seams `deletionRulesLookup`, `deletionRuleWrites`, `deletionFlagLookup`.
- `fuku/modules/greetings.go`: `applyCleanService`, and `delJoinedToggleConfig.setPref` now calls it under `greetingApplyContext()`; getPref, message keys, rights, `connectBotAdmin` and replies are unchanged.
- Tests: `TestDeletionEffectiveRule`, `TestApplyDeleteRule`, `TestApplyCleanServiceRule`, `TestServiceKindTable`, `TestServiceKindsCoverMessageFields`, `TestDeletionKindRegistryMatchesRepository`, `TestDeletionKindsNeverStaffLockdownCaptchaLog`, `TestCleanServiceCommandResetsDelayRows`.

## Task Commits

1. Task 1 RED `29f7037` test(16-02): a stored rule reads back as the effective delay
2. Task 1 GREEN `6229dcf` feat(16-02): one apply function for every deletion rule
3. Task 2 RED `f3f4b1a` test(16-02): the service table covers every gotgbot service field once
4. Task 2 GREEN `3d615ca` feat(16-02): the twelve service rows and their predicates
5. Task 3 RED `67620c6` test(16-02): joined and left share the /cleanservice flag
6. Task 3 GREEN `4041fd9` feat(16-02): joined and left share the /cleanservice flag through one apply function

The Task 1 and Task 3 RED commits do not compile (symbols missing); the Task 2 RED commit compiles and fails on the eleven missing rows.

## Verification (real results)

All run from /home/user/Fuku_Robot, SQLite (the `fuku/modules` harness does not run on PostgreSQL).

| Gate | Result |
|------|--------|
| `go test -tags testtools -race -count=1 -run '^(TestApplyDeleteRule\|TestDeletionEffectiveRule)$' -v ./fuku/modules` (Task 1 verify) | all PASS, 8 + 3 subtests, no SKIP |
| `-run '^(TestServiceKindTable\|TestServiceKindsCoverMessageFields\|TestDeletionKindRegistryMatchesRepository\|TestDeletionKindsNeverStaffLockdownCaptchaLog\|TestDeletionEffectiveRule)$' -v` (Task 2 verify, run together with TestApplyDeleteRule) | all PASS |
| `-run '^(TestApplyCleanServiceRule\|TestCleanServiceCommandResetsDelayRows\|TestDeletionEffectiveRule\|TestApplyDeleteRule)$' -v` (Task 3 verify, first command) | all PASS, 0 SKIP, 0 FAIL |
| `-run '^(TestGreeting\|TestCleanService\|TestDelJoined\|TestGreetings)' ./fuku/modules` (second command) | ok |
| `go test -tags testtools -race -count=1 ./fuku/db/deletion ./fuku/db/greetings` | ok, ok |
| Plan verification `-run '^(TestServiceKind\|TestDeletion\|TestApply\|TestCleanService)' ./fuku/modules` | ok (7.7 s) |
| `CGO_ENABLED=0 go build ./...`; `go vet -tags testtools ./fuku/modules ./fuku/db/...` | clean |
| `gofmt -l` on every file touched | nothing (the tree has older unformatted files in `fuku/db` and `fuku/modules` that this plan did not touch) |
| `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827` (exit code read directly) | exit 0, "no dupl, gocyclo or godox findings on lines added since 8c0c827" |
| `git diff --exit-code 8c0c827 -- go.mod go.sum` | exit 0 |
| `git diff --exit-code dd61b2c -- fuku/modules/raid_flood.go` | exit 0 |
| Task 1 acceptance: `grep -c 'func applyDeleteRule(\|func deletionServiceKindOf(\|func deletionEffectiveDelay('` | 1 in `deletion_apply.go`, 2 in `deletion_kinds.go` (two files non-zero: the gate's "2"); `grep -c 'deletion\.Kind' deletion_kinds.go` = 7 (needs at least 6) |
| Task 2 acceptance: `Code: "s[a-z]"` 12, `Code: "b[a-z]"` 5, creation/migrate/ForumTopicCreated grep over non-comment lines 0 | pass |
| Task 3 acceptance: `grep -c applyCleanService greetings.go` 3 (needs 2), old `setPref: greetings.SetShouldCleanService` 0; `greetings_command_test.go` numstat vs `dd61b2c` is 67 additions, 0 deletions (the awk gate exits 0) | pass |

**Not run:** `make lint` (cannot run here; the stand-in ran), `make test`, the whole `fuku/modules` package, the settings family (the plan did not ask for it and touches no page), `make check-translations` (no locale change; the keys are added by plan 16-07), gosec.

## Decisions Made

See `key-decisions`. The D-02 coupling is asserted, not hidden: after Keep both kinds read Keep, after a Now or After N on either kind the OTHER kind reads Delete now unless it has its own row (`TestApplyCleanServiceRule` "after turns the flag on and the other kind reads Delete now"). The planner's one refinement inside D-02, that Keep removes BOTH delay rows (so a stale delay never comes back at a later Now), is implemented and pinned; it should go into 16-SPIKE.md for the owner.

## Deviations from Plan

**1. [Rule 3 - Blocking] No `ErrSharedKind` in the repository.** The plan says `applyDeleteRule` refuses "any kind for which `deletion.SharesCleanServiceFlag` is true" but plan 16-01 defined no sentinel for it. I added the package-local `errDeletionSharedKind` in `deletion_apply.go` rather than widening the repository for one refusal. Files: `fuku/modules/deletion_apply.go`.

**2. [Added] Result fields.** `deletionApplyResult` also carries `CleanService`, `Delay` and `On` (filled by `applyCleanServiceRule` only), so the page of plan 16-08 gets the effective state of the pressed kind without a second read. Not in the plan's text; the plan asks for "the effective state read back".

**3. [Added] Extra cases.** `TestApplyDeleteRule` has an "unreadable state" subtest (a failed pre-write read is a save failure with no write, per the plan's assumptions), `TestApplyCleanServiceRule` pins that Keep with a stale row is a write and Keep on a clean chat is not, and `TestServiceKindsCoverMessageFields` also fails on a listed name that is no longer a Message field.

**4. [Process] Commit ledger.** The `gsd-plan-head-before-16-02` file was created after the first commit (from `d49ee3b`, the HEAD at plan start), so `commits:` is still measured from the real base (6).

**5. [STATE.md counter] The brief says the `Plan:` line equals (plans with a SUMMARY) + 1, but the line 16-01 committed read `Plan: 1 of 9`, and the prompt for this plan asked for `Plan: 2 of 9` after it.** I kept the committed convention and the prompt: `Plan: 2 of 9` (two plans done), which is also what `state.advance-plan` printed. Flagged for the orchestrator.

No gate had to be corrected: every gate in the plan passed as written.

## Known Stubs

None. `deletionServiceKindOf`, `deletionRowOf`, `deletionRowOfKind`, `applyDeleteRule`, `applyCleanServiceRule` and the `Presets` and key fields have no production caller yet; the plan's stated end state is that nothing acts on an update until 16-03 (watcher), 16-05/16-06 (hooks) and 16-08 (page) call them. Each is exercised by a test of this plan, and the lint stand-in reports no unused finding.

## Threat Flags

None beyond the plan's register: T-16-06 (no creation, migrate, topic-creation or giveaway-post message is ever a row: `TestServiceKindTable` and the per-field build of `TestServiceKindsCoverMessageFields`), T-16-07 (kind, mode and delay validated before any read or write; joined and left only through the transaction), T-16-08 (a stale delay never revives: `TestCleanServiceCommandResetsDelayRows`), T-16-SC (`go.mod` and `go.sum` unchanged).

## Requirements

`requirements-completed: []`. DEL-01, DEL-02 and DEL-07 stay Pending; the closing plan 16-09 completes them.

## Self-Check: PASSED

Created files exist (`deletion_kinds.go`, `deletion_kinds_test.go`, `deletion_apply.go`, `deletion_apply_test.go`) and commits `29f7037`, `6229dcf`, `f3f4b1a`, `3d615ca`, `67620c6`, `4041fd9` are ancestors of HEAD.
