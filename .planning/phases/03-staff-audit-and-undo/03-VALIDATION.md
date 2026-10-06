---
phase: "3"
slug: "staff-audit-and-undo"
# status lifecycle: draft (seeded by plan-phase) → validated (set by validate-phase §6)
# audit-milestone §5.5 distinguishes NOT-VALIDATED (draft) from PARTIAL (validated + nyquist_compliant: true) (#2117)
status: validated
nyquist_compliant: false
wave_0_complete: true
created: "2026-10-05"
---

# Phase 3 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test + testify assertions; real fixtures (SQLite via `internal/testdb.Run`, miniredis, hand-written `gotgbot.BotClient` fakes `staffActionFake` / `staffBotClient`) |
| **Config file** | none; every test file starts with `//go:build testtools` |
| **Quick run command** | `go test -tags testtools -race -count=1 -run '^TestStaff(Undo\|History\|Audit\|ActionRecord\|ActionLog)' ./alita/modules ./alita/db/staff` |
| **Full suite command** | `make test` (sandbox: `go test -tags testtools -race -count=1 ./alita/modules ./alita/db/staff ./alita/utils/actionlog ./alita/utils/ratelimit`) |
| **Estimated runtime** | `make test` ~180 s; `alita/modules` package ~120 s; first compile of `alita/modules` ~77 s; each narrow `-run` < 60 s once compiled |

---

## Sampling Rate

- **After every task commit:** Run the narrow `-run` for the touched area (see the map below)
- **After every plan wave:** Run the full suite command
- **Before `/gsd-verify-work`:** `make test`, `make lint`, `make check-translations`, `make generate-docs && make check-docs` must be green
- **Max feedback latency:** 60 seconds

---

## Per-Task Verification Map

Seeded from `03-RESEARCH.md` § Validation Architecture at requirement level. Task IDs were bound to plans by `/gsd-validate-phase` on 2026-10-06 (Task ID = phase-plan).

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 3-01 | 01 | 1 | STAFF-10 | — | Parent + pending rows at Confirm; columns round-trip | repo (SQLite) | `go test -tags testtools -race -count=1 -run '^TestStaffActionRecord(Create\|RoundTrip)' ./alita/db/staff` | ✅ | ✅ green |
| 3-01 | 01 | 1 | STAFF-10 | — | Prior state written before the Telegram call; per-group outcomes; finalize; fresh read after "restart" | module + SQLite | `go test -tags testtools -race -count=1 -run '^TestStaffActionRecordsPriorState$\|^TestStaffActionRecordPerGroup$\|^TestStaffActionRecordFinalize$' ./alita/modules` | ✅ | ✅ green |
| 3-01 | 01 | 1 | STAFF-10 | — | Mid-run shutdown leaves a truthful record | module | `go test -tags testtools -race -count=1 -run '^TestStopStaffActionsRecordsInterrupted$' ./alita/modules` | ✅ | ✅ green |
| 3-01 | 01 | 1 | STAFF-10 | — | Audit write failure fails closed (no Telegram write) | module | `go test -tags testtools -race -count=1 -run '^TestStaffActionRecordFailureFailsClosed$' ./alita/modules` | ✅ | ✅ green |
| 3-01 | 01 | 1 | STAFF-10 | — | `RekeyChat` moves `staff_chat_id`, never `summary_chat_id` | repo | `go test -tags testtools -race -count=1 -run '^TestRekeyChat' ./alita/db/staff` | ✅ | ✅ green |
| 3-03 | 03 | 2 | STAFF-09 | — | Log post only to ✅ groups, admin category, paced, failure never changes ✅, no Staff Group title/ID, reason escaped + capped | module | `go test -tags testtools -race -count=1 -run '^TestStaffActionLog' ./alita/modules` | ✅ | ✅ green |
| 3-03 | 03 | 2 | STAFF-09 | — | `actionlog.Log` unchanged for existing callers | unit | `go test -tags testtools -race -count=1 -run '^TestActionLog' ./alita/modules` | ✅ | ✅ green |
| 3-02 | 02 | 2 | STAFF-11 | — | Undo decision matrix and never-lift invariants | table + property (pure) | `go test -tags testtools -race -count=1 -run '^TestStaffUndoDecision(Table\|Invariants)$' ./alita/modules` | ✅ | ✅ green |
| 3-08 | 08 | 7 | STAFF-11 | — | Restore-prior end to end over member / left / restricted / kicked priors | module | `go test -tags testtools -race -count=1 -run '^TestStaffUndoRestores' ./alita/modules` | ✅ | ✅ green |
| 3-08 | 08 | 7 | STAFF-11 | — | Only ✅ groups touched; changed-since and unlinked skipped | module | `go test -tags testtools -race -count=1 -run '^TestStaffUndo(OnlyApplied\|ChangedSince\|LinkRemoved)$' ./alita/modules` | ✅ | ✅ green |
| 3-08 | 08 | 7 | STAFF-11 | — | Presser rights checked live per group; outsider refused | module | `go test -tags testtools -race -count=1 -run '^TestStaffUndo(PresserRights\|Outsider)$' ./alita/modules` | ✅ | ✅ green |
| 3-06/07 | 06, 07 | 5-6 | STAFF-11 | — | Confirm card presser-only, expiry, cancel; shared target lock | module + miniredis | `go test -tags testtools -race -count=1 -run '^TestStaffUndo(Card\|TargetLock)' ./alita/modules` | ✅ | ✅ green |
| 3-07 | 07 | 6 | STAFF-11 | — | One undo per action across replicas | module, concurrent | `go test -tags testtools -race -count=1 -run '^TestStaffUndoOnce' ./alita/modules` | ✅ | ✅ green |
| 3-06 | 06 | 5 | STAFF-11 | — | Result is a reply; original edited; post-rekey plain message | module | `go test -tags testtools -race -count=1 -run '^TestStaffUndo(Reply\|OriginalEdit\|AfterRekey)' ./alita/modules` | ✅ | ✅ green |
| 3-06 | 06 | 5 | STAFF-11 | — | Undo button rules (no kick, no ✅, undone, unfinished) | module | `go test -tags testtools -race -count=1 -run '^TestStaffUndoButton' ./alita/modules` | ✅ | ✅ green |
| 3-08 | 08 | 7 | STAFF-11 | — | `#STAFF_UNDO` log per succeeded group | module | `go test -tags testtools -race -count=1 -run '^TestStaffUndoLog' ./alita/modules` | ✅ | ✅ green |
| 3-07 | 07 | 6 | STAFF-11 | — | Shutdown during an undo finalizes it | module | `go test -tags testtools -race -count=1 -run '^TestStopStaffActionsUndo' ./alita/modules` | ✅ | ✅ green |
| 3-04 | 04 | 3 | SETUP-09 | — | History list: newest first, 10/page, offset paging, this Staff Group only | module | `go test -tags testtools -race -count=1 -run '^TestStaffHistoryList' ./alita/modules` | ✅ | ✅ green |
| 3-05 | 05 | 4 | SETUP-09 | — | Detail view per-group lines, undo outcome, unlinked marker, Undo rules | module | `go test -tags testtools -race -count=1 -run '^TestStaffHistoryDetail' ./alita/modules` | ✅ | ✅ green |
| 3-04 | 04 | 3 | SETUP-09 | — | Members only; Back to panel | module | `go test -tags testtools -race -count=1 -run '^TestStaffHistoryAccess' ./alita/modules` | ✅ | ✅ green |
| 3-04 | 04 | 3 | SETUP-09 | — | Length cap and 64-byte callback budget | pure | `go test -tags testtools -race -count=1 -run '^TestStaffHistory(LengthCap\|CallbackBudget)' ./alita/modules` | ✅ | ✅ green |
| 3-04 | 04 | 3 | SETUP-09 | — | Panel renders the new button | pure | `go test -tags testtools -race -count=1 -run '^TestStaffPanelRender' ./alita/modules` | ✅ | ✅ green |
| all | all | all | All | — | Phase 2 regression net | module | `go test -tags testtools -race -count=1 -run '^TestStaffAction\|^TestStopStaffActions' ./alita/modules` | ✅ | ✅ green |
| all | all | all | All | — | Locale parity; every reason has text | unit | `make check-translations` | ✅ | ✅ green |
| 3-10 | 10 | 9 | STAFF-11 (G-03-4) | T-03-30..33 | An undo that reached no Telegram write gives the claim back; any reached write keeps it (D-09); "changed nothing" marker | module + SQLite | `go test -tags testtools -race -count=1 -run '^TestStaffUndo(ClaimReleased\|NothingChanged\|PanicKeepsClaim)$' ./alita/modules` | ✅ | ✅ green |
| 3-11 | 11 | 10 | STAFF-11 (G-03-4) | T-03-34..35 | Shutdown waits for a claimed undo Confirm; a Confirm after the shutdown began claims nothing | module | `go test -tags testtools -race -count=1 -run '^TestStaffUndo(ShutdownWindow\|ConfirmAfterStop)$' ./alita/modules` | ✅ | ✅ green |
| 3-12 | 12 | 11 | SETUP-09, STAFF-10, STAFF-11 (G-03-4) | T-03-36..37 | Undo states running / interrupted / undone / changed nothing in history, detail and Ask/Confirm answers | module | `go test -tags testtools -race -count=1 -run '^TestStaffUndoClaimedTexts$\|^TestStaffHistoryDetailUndoOutcome$' ./alita/modules` | ✅ | ✅ green |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [x] `alita/db/staff/actions_test.go` — repository tests (create, prior, result, finalize, claim, list/paging/tally, rekey)
- [x] Add `&models.StaffAction{}, &models.StaffActionGroup{}` to the three `AutoMigrate` lists (`alita/db/staff/testmain_test.go`, `alita/modules/test_harness_test.go`, `alita/db/testmain_test.go`)
- [x] Extend `staffActionFake` (`alita/modules/staff_action_fake_test.go`) with full permission bits in `getChatMember` JSON and `restrictChatMember` storage; honour `use_independent_chat_permissions`
- [x] Extend `staffCleanup` (`alita/modules/staff_helpers_test.go`) to delete audit rows
- [x] `staffActionEnv` helpers: `undoCard()`, `setLogChannel(group, channelID)`, `recordOf(token)`
- [x] Update `panelSplitKeyboard` for the new "Recent actions" action code

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Restore of a restricted prior state via `restrictChatMember` with `use_independent_chat_permissions=true` keeps the exact permission bits | STAFF-11 | Fakes cannot prove live Telegram semantics | In a test group: restrict user with partial permissions, staff `/ban` them, Undo; check the user's permissions match the originals |
| `restrictChatMember` on a kicked / left target during undo behaves as the decision table assumes | STAFF-11 | Live Telegram behaviour only | Staff `/unban` a kicked user, Undo, confirm the user is banned again with the original end date |
| Log-channel post appears in each linked group's channel without the Staff Group's title or ID | STAFF-09 | End-to-end with real channels | Link two groups with log channels, run `/ban`, inspect both channels |
| New migration applies on PostgreSQL with checksum recorded | STAFF-10 | Needs a PG server | `ALITA_TEST_MIGRATION_CHAIN=true DATABASE_URL=<empty pg> go test -tags testtools -v -count=1 -run '^TestRepositoryMigrationChain$' ./alita/db/migrations` (CI runs it) |

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references
- [x] No watch-mode flags
- [x] Feedback latency < 60s
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** validated 2026-10-06 (validate-phase, after all 12 plans merged)

## Validation Audit 2026-10-06

Audited after all 12 plans merged, including gap-closure plans 03-10 to 03-12 for UAT gap G-03-4. Every row was bound to its plan and its command was run against the merged head under `-race`. All 306 top-level `TestStaff*`, `TestStop*` and `TestRekeyChat*` tests in `alita/modules` and `alita/db/staff` passed, with 0 failures. Each row's pattern matched at least one real test. The `actionlog` row's command pointed at a package with no test files; it now targets `TestActionLogAdminUnchanged` and `TestActionLogDestination` in `alita/modules`, which pass. `make check-translations` passes.

| Metric | Count |
|--------|-------|
| Gaps found | 0 |
| Resolved | 0 |
| Escalated | 0 |

Notes:
- `make lint` was confirmed by the owner on a go1.26 toolchain (03-UAT test 5); the sandbox's golangci-lint is built with go1.25 and cannot run on this module.
- The manual-only rows were covered by the live UAT (03-UAT tests 1, 2 and 6), all passed. The PostgreSQL migration-chain row runs in CI.
