---
phase: 01-staff-group-links
plan: 03
subsystem: staff-groups
tags: [go, gotgbot, gorm, telegram, chat-migration]

requires:
  - phase: 01-staff-group-links
    provides: "staff_groups and staff_group_links tables, invalidateStaffKeys, GetStaffGroup / GetStaffGroupFresh, /staff panel, staffBotClient test harness"
provides:
  - "staff.RekeyChat: idempotent one-transaction re-key of staff_groups.chat_id, staff_group_links.staff_chat_id and staff_group_links.group_chat_id, invalidating both IDs"
  - "StaffWatchers module (non-help, priority 237, handler group -3) with the silent migrate watcher onMigrateMessage"
  - "rekeyFromTelegramError(oldChatID, err): re-keys from a Telegram error's ResponseParams.MigrateToChatId (for plans 01-09 and 01-10)"
  - "AGENTS.md documents handler group -3 and the staff chat-migration trap"
affects: [01-07, 01-08, 01-09, 01-10]

actuals:
  tokens: 6500
  tasks: 2
  commits: 3

plan_head_before: 78fe65cca2f1c9e0700017f97590e3ff68edc7bf
plan_head_after: f74833da8a7bb220ec5f2fc8f3327dc39244d27c

tech-stack:
  added: []
  patterns:
    - "Group -3 watchers are silent and always return ext.ContinueGroups so groups -2, -1 and 0 still run"
    - "Re-key = three UPDATEs in one transaction, sum RowsAffected, invalidate old and new IDs only after commit"

key-files:
  created:
    - alita/db/staff/rekey.go
    - alita/db/staff/rekey_test.go
    - alita/modules/staff_watchers.go
    - alita/modules/staff_migrate_test.go
  modified:
    - AGENTS.md

key-decisions:
  - "The re-key is silent: no notice is posted in any chat, only an info log"
  - "rekeyFromTelegramError reports (newID, true) even when RekeyChat itself fails (error logged), so a caller can still retry its Telegram call against the new ID"
  - "A re-key that would make a link's group equal its staff chat fails as a whole (CHECK violation) and rolls back every UPDATE"

patterns-established:
  - "migrate* and rekey* test-helper prefixes keep this plan's helpers disjoint from sibling plans in the same packages"

requirements-completed: [SETUP-07]

coverage:
  - id: D1
    description: "A migrate service message (either direction, either order, duplicated) moves the Staff Group and every link to the new chat ID, /staff answers on the new ID, and the watcher never replies and always returns ext.ContinueGroups"
    requirement: "SETUP-07"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestStaffMigrate' ./alita/modules#TestStaffMigrateTracer"
        status: pass
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestStaffMigrate' ./alita/modules#TestStaffMigrateEitherOrder"
        status: pass
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestStaffMigrate' ./alita/modules#TestStaffMigrateUnrelatedChat"
        status: pass
    human_judgment: false
  - id: D2
    description: "RekeyChat updates every role of the old ID in one transaction, is idempotent, reports changed=false when nothing matched, keeps the group_chat_id <> staff_chat_id CHECK, rolls back on a violation, and invalidates both the old and the new cache IDs; a linked group's own ID is re-keyed too"
    requirement: "SETUP-07"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestRekeyChat' ./alita/db/staff"
        status: pass
    human_judgment: false
  - id: D3
    description: "A Telegram 400 error carrying ResponseParams.MigrateToChatId re-keys old to new; nil, plain, 400-without-parameter and unrelated errors change nothing"
    requirement: "SETUP-07"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestStaffMigrate' ./alita/modules#TestStaffMigrateFromTelegramError"
        status: pass
    human_judgment: false
  - id: D4
    description: "The handler is actually dispatched for real Telegram migrate updates (group -3, bots allowed) and a real basic-group-to-supergroup upgrade keeps /staff working"
    requirement: "SETUP-07"
    verification: []
    human_judgment: true
    rationale: "Tests call onMigrateMessage directly with synthetic messages and never run the dispatcher. botJoinedGroup leaves basic groups, so a basic-group Staff Group is practically unreachable (RESEARCH C2, OQ1); the plan accepted no live upgrade UAT. The registration (message.Migrate filter, SetAllowBot(true), AddHandlerToGroup at -3) is verified by grep only."

duration: 25min
completed: 2026-10-04
status: complete
---

# Phase 1 Plan 03: Staff Group Chat Migration Summary

**`staff.RekeyChat` moves a Staff Group and all its links to a new chat ID in one idempotent transaction, triggered by either Telegram migrate service message (silent watcher at handler group -3) or by the 400 `migrate_to_chat_id` error parameter via `rekeyFromTelegramError`.**

## Performance

- **Duration:** about 25 min (start time was not captured at spawn, so this is approximate)
- **Tasks:** 2 (1 tracer, 1 TDD expansion)
- **Files:** 5 changed, 634 insertions, 1 deletion (measured against the base commit)

## Accomplishments

- Tracer slice end to end: `RekeyChat`, the new non-help `StaffWatchers` module (priority 237, group -3, bots allowed), `onMigrateMessage` with panic recovery, and the AGENTS.md group -3 entry plus the chat-migration trap bullet. The tracer verify passed before expansion (`⚡ Tracer verified end-to-end — expanding`).
- Re-key hardened: eight `TestRekeyChat*` repository tests cover moving the staff row and links, idempotency, no-match, zero and equal IDs, a linked group's own ID, the CHECK constraint, rollback of the whole transaction when a link would collapse into a self-link, and cache invalidation of both IDs (stale "not found" sentinel on the new ID is cleared).
- Delivery order is irrelevant: migrate_from then migrate_to, the reverse, and duplicates of either all reach the same end state; an unrelated chat's migrate message changes no rows and sends nothing.
- `rekeyFromTelegramError` detects the 400 parameter through `errors.As` (including a wrapped error), ready for plans 01-09 and 01-10.
- Verification: `make test` exits 0 with no failures, `golangci-lint` (v2.13.1, `--new-from-rev` base) and the dupl pass report 0 issues, `make check-docs` reports no drift, `go build ./...` and `go vet -tags testtools` are clean, `gofmt -l` is empty on the changed files, `go.mod` and `go.sum` are unchanged.

## Task Commits

1. **Task 1: tracer, migrate message to RekeyChat to /staff on the new ID** - `711cbb8` (feat)
2. **Task 2 RED: failing detector test plus RekeyChat characterisation tests** - `8d606cf` (test)
3. **Task 2 GREEN: rekeyFromTelegramError** - `f74833d` (feat)

## TDD Gate Compliance

RED commit `8d606cf` (test) precedes GREEN commit `f74833d` (feat). The RED run failed intentionally on the target assertion: `TestStaffMigrateFromTelegramError/migrate_parameter` reported `rekeyFromTelegramError = (0, false), want (<newID>, true)`. To make that an assertion failure rather than a compile error, the RED commit carries a stub `rekeyFromTelegramError` returning `(0, false)` that GREEN replaces. The eight `TestRekeyChat*` tests and the other `TestStaffMigrate*` tests passed immediately because they characterise the Task 1 code; they have no failing-first record. `workflow.tdd_mode` was off, so no hard gate applied. No REFACTOR commit was needed.

## Deviations from Plan

None - plan executed exactly as written. Two additions beyond the plan's behaviour list: `TestRekeyChatRollsBackOnConstraintViolation` (pins the T-01-09 atomicity mitigation) and `TestRekeyChatIgnoresZeroAndEqualIDs` (pins the guard clauses the plan specified for `RekeyChat`).

## Known Stubs

None.

## Threat Flags

None beyond the plan's threat model. T-01-09 (partial update) is mitigated by one transaction and pinned by the rollback test; T-01-10 (watcher swallowing updates) is mitigated by `onMigrateMessage` always returning `ext.ContinueGroups`, recovering from panics and never replying, pinned by `TestStaffMigrateUnrelatedChat` and the "no bare `return nil`" grep; T-01-SC holds (`go.mod` and `go.sum` unchanged).

## Issues Encountered

- The dispatcher wiring (filter, `SetAllowBot(true)`, group -3) is not exercised by a test; see coverage D4. A live basic-group-to-supergroup upgrade was not run (plan-accepted assumption OQ1).
- `RekeyChat` returns the error and changes nothing when the new ID is already registered (UNIQUE violation); that path is logged but only the CHECK-violation rollback is covered by a test.

## Next Phase Readiness

Plans 01-07 and 01-08 can add the ownership and bot-health watchers to `staffWatchersModule` / `LoadStaffWatchers` (group -3, return `ext.ContinueGroups`). Plans 01-09 and 01-10 can call `rekeyFromTelegramError(oldChatID, err)` after a failed Telegram call on a Staff Group chat and retry against the returned ID.

## Self-Check: PASSED

- Created files exist: `alita/db/staff/rekey.go`, `alita/db/staff/rekey_test.go`, `alita/modules/staff_watchers.go`, `alita/modules/staff_migrate_test.go`.
- Commits `711cbb8`, `8d606cf`, `f74833d` are on the branch.
- Task 1 and Task 2 acceptance criteria re-run and passing (greps match, no bare `return nil`, 10 `TestStaffMigrate` passes, 8 `TestRekeyChat` passes).
- Plan-level verification passing: `make test`, golangci-lint and dupl (0 issues), `make check-docs`, `go.mod` and `go.sum` unchanged.
