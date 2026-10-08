---
phase: 01-staff-group-links
plan: 07
subsystem: staff
tags: [telegram, ownership, recheck, watchers, gorm, i18n]

requires:
  - phase: 01-staff-group-links
    provides: "CheckOwner tri-state, staffWatchersModule at group -3, Fresh repo reads, sendStaffNotice (plans 01-03, 01-06)"
provides:
  - "recheckLink and recheckStaffGroup: the single live-checked authority recheck (D-15)"
  - "chat_owner_changed / chat_owner_left and chat_member creator-transition watchers at group -3"
  - "staff.DeleteLinkIfOwner and staff.UpdateStaffGroupOwner"
  - "Removal notices to the Staff Group only (D-13) in all 7 locales"
affects: [01-09 panel recheck, 01-10 hourly sweep, Phase 2 pre-action recheck]

actuals:
  tokens: 33000
  tasks: 2
  commits: 3

tech-stack:
  added: []
  patterns:
    - "Conditional DELETE ... RETURNING decides the single notice poster"
    - "Per-pass memoised CheckOwner (staffOwnerPass) with sync.Once per (chat, user) key"
    - "Notice titles spliced in after translation via placeholder tokens"

key-files:
  created:
    - alita/modules/staff_recheck.go
    - alita/modules/staff_ownership_test.go
  modified:
    - alita/modules/staff_watchers.go
    - alita/db/staff/repository.go
    - alita/db/staff/repository_test.go
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml
    - AGENTS.md

key-decisions:
  - "recheckStaffGroup calls pace before every group-side check, including the first, and a false return stops the run with Unknown set; nothing is removed for a stopped run"
  - "A Staff Group recheck seeds the pass with the live creator so group-side rechecks do not ask Telegram about the Staff Group again"
  - "recheckChatOwnership handles both roles from the cached gates, so the service-message and chat_member watchers share one entry point"

patterns-established:
  - "Only OwnerMismatch removes; any Telegram error is Unknown and changes nothing"
  - "Watchers at group -3 always return ext.ContinueGroups"

requirements-completed: [SETUP-06, PLAT-03]

coverage:
  - id: D1
    description: "Owner change of a linked group removes the link after a live check and notifies only the Staff Group; the payload is only a hint"
    requirement: SETUP-06
    verification:
      - kind: unit
        ref: "alita/modules/staff_ownership_test.go#TestStaffOwnershipTracer"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_ownership_test.go#TestStaffOwnershipPayloadIsOnlyAHint"
        status: pass
    human_judgment: false
  - id: D2
    description: "Staff Group owner change removes every link made by the previous owner, posts one combined notice, keeps Staff status and refreshes owner_user_id"
    requirement: SETUP-06
    verification:
      - kind: unit
        ref: "alita/modules/staff_ownership_test.go#TestStaffOwnershipStaffGroupChanged"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_ownership_test.go#TestStaffOwnershipStaffGroupHasNoCreatorRemovesAllLinks"
        status: pass
    human_judgment: false
  - id: D3
    description: "Owner-left, same-new-owner, Telegram errors (fail closed) and racing triggers (exactly one notice)"
    requirement: PLAT-03
    verification:
      - kind: unit
        ref: "alita/modules/staff_ownership_test.go#TestStaffOwnershipOwnerLeft"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_ownership_test.go#TestStaffOwnershipSameNewOwnerStillUnlinks"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_ownership_test.go#TestStaffOwnershipUnknownKeepsLink"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_ownership_test.go#TestStaffOwnershipConcurrentRechecksPostOnce"
        status: pass
    human_judgment: false
  - id: D4
    description: "chat_member creator transitions trigger the recheck; non-creator transitions and unrelated chats cost no Telegram call"
    requirement: SETUP-06
    verification:
      - kind: unit
        ref: "alita/modules/staff_ownership_test.go#TestStaffOwnershipChatMemberCreatorTransition"
        status: pass
    human_judgment: false
  - id: D5
    description: "Real Telegram emits chat_owner_changed (and possibly a creator chat_member update) on an actual ownership transfer"
    verification: []
    human_judgment: true
    rationale: "Research A1/A5: Telegram's behavior on a real ownership transfer is undocumented; only a live transfer in a throwaway supergroup shows it"

duration: 5min
completed: 2026-10-04
status: complete
plan_head_before: d97ee58abae1d8a2e8ce4fd5cc1fdc0d22a9ec98
plan_head_after: 9e5d03311613242edc65b053fc9558895d376817
commits: 3
---

# Phase 1 Plan 07: Automatic unlinking on ownership change Summary

**Live-checked recheckLink/recheckStaffGroup core with chat_owner_changed, chat_owner_left and chat_member creator watchers: a link vanishes by itself when its maker stops owning either group, and the Staff Group gets exactly one notice.**

## Performance

- **Duration:** 5 min in this attempt (a prior executor attempt did Task 1 and the Task 2 tests before a container restart)
- **Started:** 2026-10-04T23:27:00Z
- **Completed:** 2026-10-04T23:32:25Z
- **Tasks:** 2
- **Files modified:** 13

## Provenance: prior attempt versus this run

- **Cherry-picked from the prior attempt:** commit `8ce997f` (now `78b2cff` here), the whole of Task 1: `DeleteLinkIfOwner`, `staff_recheck.go` (`staffOwnerPass`, `recheckLink`, notice helpers), `onOwnershipMessage`, the two notice keys in all 7 locales, and the tracer plus payload tests. I inspected it against Task 1 and re-ran the tracer verify (both tests pass) before keeping it.
- **Ported from the prior attempt's uncommitted working tree:** `alita/modules/staff_ownership_test.go` (all Task 2 behavior tests, plus the `clearCreator` fake helper) and `alita/db/staff/repository_test.go` (the `DeleteLinkIfOwner*` and `UpdateStaffGroupOwner*` tests), copied verbatim after reading them against the plan's `<behavior>` list.
- **Written in this run:** `UpdateStaffGroupOwner`, `recheckStaffGroup`/`staffRecheckSummary`, `staffCreatorTransition`, `onCreatorChatMember`, the `recheckChatOwnership` extension to Staff Groups, the `NewChatMember` registration, and the AGENTS.md trap.

## Accomplishments

- Owner change in a linked group (service message) removes the link after a live `getChatAdministrators` check and posts one notice to the Staff Group only; nothing goes to the linked group or a private chat.
- Staff Group owner change removes every link made by the previous owner (D-12), posts one combined notice in id order, keeps the `staff_groups` row, and refreshes `owner_user_id` to the live creator.
- `chat_member` updates with a creator old/new status trigger the same recheck; other transitions and unrelated chats make zero Telegram calls.
- Telegram errors and rate limits leave links and the Staff Group untouched; a response with no creator counts as a mismatch.
- Racing triggers (8 concurrent `recheckLink` calls) produce exactly one notice because only the conditional DELETE's winner posts.

## Task Commits

1. **Task 1: tracer, owner of a linked group changes** - `78b2cff` (feat; cherry-picked from prior attempt `8ce997f`)
2. **Task 2 RED: failing tests for Staff Group side and watchers** - `9dce777` (test; compile stubs so tests fail on assertions, 9 of the new tests failed)
3. **Task 2 GREEN: recheckStaffGroup, creator watcher, UpdateStaffGroupOwner, AGENTS.md** - `9e5d033` (feat)

**Plan metadata:** committed separately (docs: complete plan).

## Files Created/Modified

- `alita/modules/staff_recheck.go` - `staffOwnerPass`, `recheckLink`, `recheckStaffGroup`, removal notices
- `alita/modules/staff_watchers.go` - ownership message and creator `chat_member` watchers at group -3, `recheckChatOwnership`
- `alita/db/staff/repository.go` - `DeleteLinkIfOwner`, `UpdateStaffGroupOwner`
- `alita/modules/staff_ownership_test.go`, `alita/db/staff/repository_test.go` - tests
- `locales/*.yml` (7) - `staff_notice_unlinked_group_owner_changed`, `staff_notice_unlinked_staff_owner_changed`
- `AGENTS.md` - staff authority trap

## Decisions Made

- `pace` runs before every group-side check, including the first, and a false return stops the run (required by `TestStaffOwnershipRecheckStaffGroupPaceStopsLoop`).
- The combined Staff Group notice is posted from a `defer`, so a run stopped early by `pace` or a cancelled context still reports removals it already made.
- `DeleteLinkIfOwner` uses `DELETE ... RETURNING` (as `DeleteLink` does) so the group chat ID for cache invalidation comes from the same statement.

## Deviations from Plan

None - plan executed exactly as written. The RED commit carries minimal compile stubs (returning zero values) so the new tests fail on behavior assertions rather than compile errors; the GREEN commit replaces them.

## Issues Encountered

None. `gofmt -l` flags `alita/modules/greetings_command_test.go`, which this plan did not touch (pre-existing, out of scope).

## Known Stubs

None. The RED-phase stubs were replaced in `9e5d033`.

## Threat Flags

None. The new surface (watchers on existing update types, a conditional UPDATE/DELETE) is covered by T-01-22 to T-01-25.

## Verification Results

- `go test -tags testtools -race -count=1 -timeout 10m ./...` passes; `go vet -tags testtools ./...` clean
- `TestStaffOwnership*`: 21 PASS lines (parents and subtests); repository tests pass
- `make check-translations` and `make check-docs` pass; `go.mod`/`go.sum` unchanged

## Human check outstanding (end-of-phase)

Transfer ownership of a throwaway supergroup linked to a Staff Group, log the raw updates, and confirm: a `chat_owner_changed` arrives (note whether a creator `chat_member` update also arrives), the link disappears within seconds, the Staff Group gets exactly one "its owner changed" notice, and nothing appears in the linked group (research A1, A5).

## Next Phase Readiness

`recheckLink` and `recheckStaffGroup(ctx, b, id, pace)` are ready for the panel (01-09) and the hourly sweep (01-10).

## Self-Check: PASSED

---
*Phase: 01-staff-group-links*
*Completed: 2026-10-04*
