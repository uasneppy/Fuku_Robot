---
phase: 01-staff-group-links
plan: 08
subsystem: staff
tags: [telegram, my_chat_member, bot-health, watchers, gorm, i18n]

requires:
  - phase: 01-staff-group-links
    provides: "staffHealthFromBot, staffWatchersModule at group -3, GetLinkOfGroup/GetLinkOfGroupFresh, sendStaffNotice, staffChatTranslator (plans 01-03, 01-05, 01-07)"
provides:
  - "staff.SetLinkHealth: the conditional UPDATE ... WHERE health <> new that decides who posts the heads-up"
  - "applyLinkHealth: the single link-health transition primitive, reused by the panel (01-09) and sweep (01-10)"
  - "onBotChatMember my_chat_member watcher at group -3, returning ext.ContinueGroups"
  - "Four heads-up strings (bot_missing, bot_not_admin, bot_cannot_restrict, ok) in all 7 locales"
affects: [01-09 panel, 01-10 hourly sweep, Phase 2 pre-action checks]

actuals:
  tokens: 14000
  tasks: 2
  commits: 2

tech-stack:
  added: []
  patterns:
    - "Conditional UPDATE ... WHERE health <> new, RowsAffected == 1 decides the single heads-up poster"
    - "Health derived from the my_chat_member payload itself (no extra API call)"
    - "Group title spliced into the translated heads-up after translation (staffGroupTitleToken)"

key-files:
  created:
    - alita/modules/staff_health_test.go
  modified:
    - alita/db/staff/repository.go
    - alita/db/staff/repository_test.go
    - alita/modules/staff_recheck.go
    - alita/modules/staff_watchers.go
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml

key-decisions:
  - "The heads-up is posted synchronously from onBotChatMember: the only Telegram call is the one SendMessage to the Staff Group, no live membership lookup is made"
  - "SetLinkHealth reads group_chat_id first (for cache invalidation) and then runs the conditional UPDATE; a missing link returns (false, nil)"
  - "applyLinkHealth with an unexpected health value still records it but posts nothing (empty text), so a future health value cannot send an empty message"

patterns-established:
  - "Only the caller whose conditional statement affected a row posts the Staff Group notice (now for removal and for health change)"
  - "Watchers at group -3 always return ext.ContinueGroups"

requirements-completed: [SETUP-06, SETUP-08, PLAT-03]

coverage:
  - id: D1
    description: "Bot removed from, demoted in, or stripped of the restrict right in a linked group keeps the link, records bot_missing / bot_not_admin / bot_cannot_restrict, and gives the Staff Group exactly one heads-up naming the group"
    requirement: SETUP-08
    verification:
      - kind: unit
        ref: "alita/modules/staff_health_test.go#TestStaffHealthTracer"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_health_test.go#TestStaffHealthTransitions"
        status: pass
    human_judgment: false
  - id: D2
    description: "Adding the bot back with the right permissions returns the same link row to ok with one healthy-again heads-up and no relinking"
    requirement: SETUP-08
    verification:
      - kind: unit
        ref: "alita/modules/staff_health_test.go#TestStaffHealthRecovery"
        status: pass
    human_judgment: false
  - id: D3
    description: "Repeated updates, repeated applyLinkHealth calls and 8 racing callers post exactly one heads-up (conditional UPDATE, RowsAffected == 1)"
    requirement: PLAT-03
    verification:
      - kind: unit
        ref: "alita/modules/staff_health_test.go#TestStaffHealthNoRepeat"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_health_test.go#TestStaffHealthConcurrentPostsOnce"
        status: pass
      - kind: unit
        ref: "alita/db/staff/repository_test.go#TestStaffRepoSetLinkHealthConcurrentOneWinner"
        status: pass
    human_judgment: false
  - id: D4
    description: "my_chat_member in an unlinked group or in the Staff Group itself changes nothing, posts nothing and returns ext.ContinueGroups"
    requirement: SETUP-06
    verification:
      - kind: unit
        ref: "alita/modules/staff_health_test.go#TestStaffHealthIgnoresUnrelatedChats"
        status: pass
    human_judgment: false
  - id: D5
    description: "SetLinkHealth: change returns true and invalidates the cached gate, same value returns false, unknown value is rejected by the CHECK, missing link is a no-op"
    requirement: SETUP-08
    verification:
      - kind: unit
        ref: "alita/db/staff/repository_test.go#TestStaffRepoSetLinkHealthChangesOnceAndInvalidates"
        status: pass
      - kind: unit
        ref: "alita/db/staff/repository_test.go#TestStaffRepoSetLinkHealthRejectsUnknownValue"
        status: pass
    human_judgment: false
  - id: D6
    description: "Real Telegram delivers my_chat_member for the bot being removed, demoted and re-added in a real linked supergroup, and the heads-up wording reads well in every language"
    verification: []
    human_judgment: true
    rationale: "Only a live run against Telegram shows the real update shapes (including Telegram skipping or merging updates); the non-English wording needs a native-speaker read"

duration: 8min
completed: 2026-10-04
status: complete
plan_head_before: c481b9c1f0e9370f6daa35f15f98936ad6affb89
plan_head_after: 5f9977b21a277294e5a14a742acb15a3566dedae
commits: 2
---

# Phase 1 Plan 08: Bot-health tracking for links Summary

**A my_chat_member watcher at group -3 derives each linked group's bot health from the update itself and records it through one conditional UPDATE, so the Staff Group hears about every change, and about recovery, exactly once.**

## Performance

- **Duration:** 8 min
- **Started:** 2026-10-04T23:35:00Z
- **Completed:** 2026-10-04T23:43:00Z
- **Tasks:** 2
- **Files modified:** 12 (1 created)

## Accomplishments

- Removing the bot, demoting it, or taking away its restrict right in a linked group keeps the link, sets its health to `bot_missing`, `bot_not_admin` or `bot_cannot_restrict`, and posts one heads-up naming the group to the Staff Group only. Nothing is sent to the linked group.
- Adding the bot back as an admin with the restrict right returns the same link row to `ok` with one "healthy again" heads-up and no relinking.
- A repeated check that finds the same health, a repeated update, and 8 racing callers all post nothing extra: only the caller whose `UPDATE ... WHERE health <> new` affected a row posts.
- `my_chat_member` updates for unlinked chats, and for the Staff Group itself, are dropped by the cached link gate with no write and no message; the watcher always returns `ext.ContinueGroups`, so the admin cache (-2) and `botJoinedGroup` (-1) still run.
- `applyLinkHealth` is the single health-transition primitive for the panel (01-09) and the sweep (01-10) to reuse.

## Task Commits

1. **Task 1: tracer, bot removed from a linked group, health recorded, one heads-up** - `2efe567` (feat)
2. **Task 2: transition, recovery, no-repeat, concurrency and unrelated-chat tests** - `5f9977b` (test)

**Plan metadata:** committed separately (docs: complete plan).

## Tracer gate

`TestStaffHealthTracer` is the tracer verify, and it carries only `<automated>` evidence. Run mode is interactive with `human_verify_mode=end-of-phase`, so the verify was re-run end to end after the Task 1 commit, passed (`--- PASS: TestStaffHealthTracer`), and execution continued to Task 2 with no checkpoint.

## Files Created/Modified

- `alita/db/staff/repository.go` - `SetLinkHealth(id, health) (changed bool, err error)`: conditional UPDATE through a map, cache invalidation of the group's keys when a row changed
- `alita/modules/staff_recheck.go` - `applyLinkHealth` and `staffHealthNoticeText` (literal-key switch, title spliced in after translation)
- `alita/modules/staff_watchers.go` - `onBotChatMember` and its `handlers.NewMyChatMember` registration at group -3
- `alita/modules/staff_health_test.go` - tracer plus transitions, recovery, no-repeat, concurrency and unrelated-chat tests
- `alita/db/staff/repository_test.go` - `TestStaffRepoSetLinkHealth*`
- `locales/*.yml` (7) - `staff_notice_health_bot_missing`, `staff_notice_health_bot_not_admin`, `staff_notice_health_bot_cannot_restrict`, `staff_notice_health_ok`

## Decisions Made

- The heads-up is sent synchronously inside `onBotChatMember`. "Never block on Telegram calls" is read as: no live membership lookup in the watcher (health comes from the payload). The single `SendMessage` is bounded by the client timeout; an async post would need a goroutine drained on shutdown and would make the tests racy for no real gain.
- `applyLinkHealth` records an unexpected health value but posts nothing for it (empty text guard), so a future value cannot ship an empty message.
- `SetLinkHealth` reads `group_chat_id` before the update, as the plan specified, so invalidation uses the right key; a vanished link gives `(false, nil)`.

## Deviations from Plan

None - plan executed as written. One note on process, not behavior: see TDD Gate Compliance.

## TDD Gate Compliance

Task 2 is `tdd="true"`, but Task 1 (a production-quality tracer) already implemented the full behavior, so every Task 2 test passed on first run. The RED gate (a failing test committed before implementation) could therefore not be satisfied for Task 2, and the fail-fast "unexpected GREEN" rule applies: the feature already exists, and the tests are characterization tests. There is a `feat(01-08)` commit (`2efe567`) and a `test(01-08)` commit (`5f9977b`) but no `test` commit that precedes the implementation, and no `refactor` commit.

To show the tests are not vacuous, I mutated `SetLinkHealth` to drop the `health <> ?` condition (uncommitted, reverted straight after). That made `TestStaffHealthNoRepeat` (both subtests), `TestStaffHealthConcurrentPostsOnce`, `TestStaffRepoSetLinkHealthChangesOnceAndInvalidates` and `TestStaffRepoSetLinkHealthConcurrentOneWinner` fail, which is exactly the duplicate heads-up behavior T-01-26 guards against. After the revert `git diff` on `repository.go` was empty and all pass.

## Issues Encountered

None.

## Known Stubs

None.

## Threat Flags

None. The new surface (one `my_chat_member` watcher, one conditional UPDATE) is covered by T-01-26 and T-01-27.

## Verification Results

- `go test -tags testtools -race -count=1 -timeout 10m ./...` passes
- `go vet -tags testtools ./...` clean; `gofmt -l` clean on touched files
- `go test -tags testtools -race -count=1 -run '^TestStaffHealth' ./alita/modules`: 21 PASS lines (parents and subtests, including the pre-existing `TestStaffHealthFromBot` that shares the prefix)
- `go test -tags testtools -race -count=1 -run '^TestStaffRepoSetLinkHealth' ./alita/db/staff`: 4 PASS lines
- `TestStaffLocaleKeys`, `make check-translations`, `make check-docs` pass; `git diff --exit-code go.mod go.sum` clean
- Acceptance greps: `handlers.NewMyChatMember` present in `staff_watchers.go`; `SetLinkHealth(id uint, health string) (changed bool, err error)` present; no bare `return nil` in `staff_watchers.go`; no non-literal `GetString(` key in `staff_recheck.go`

## Human check outstanding (end-of-phase)

In a throwaway supergroup linked to a Staff Group, remove the bot, then add it back as an admin with the ban right, and take the ban right away from it once. Confirm the Staff Group gets one heads-up per change (and one "healthy again"), nothing appears in the linked group, and the link is still listed afterward. Have a native speaker skim the non-English wording of the four new strings.

## Next Phase Readiness

`applyLinkHealth(b, link, health)` and `staff.SetLinkHealth` are ready for the `/staff` panel (01-09) and the hourly sweep (01-10).

## Self-Check: PASSED

---
*Phase: 01-staff-group-links*
*Completed: 2026-10-04*
