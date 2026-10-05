---
phase: "3"
slug: "staff-audit-and-undo"
# status lifecycle: draft (seeded by plan-phase) → validated (set by validate-phase §6)
# audit-milestone §5.5 distinguishes NOT-VALIDATED (draft) from PARTIAL (validated + nyquist_compliant: false) (#2117)
status: draft
nyquist_compliant: false
wave_0_complete: false
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

Seeded from `03-RESEARCH.md` § Validation Architecture at requirement level. Task IDs are bound by the plans and filled in by `/gsd-validate-phase`.

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| TBD | TBD | TBD | STAFF-10 | — | Parent + pending rows at Confirm; columns round-trip | repo (SQLite) | `go test -tags testtools -race -count=1 -run '^TestStaffActionRecord(Create\|RoundTrip)' ./alita/db/staff` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | STAFF-10 | — | Prior state written before the Telegram call; per-group outcomes; finalize; fresh read after "restart" | module + SQLite | `go test -tags testtools -race -count=1 -run '^TestStaffActionRecordsPriorState$\|^TestStaffActionRecordPerGroup$\|^TestStaffActionRecordFinalize$' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | STAFF-10 | — | Mid-run shutdown leaves a truthful record | module | `go test -tags testtools -race -count=1 -run '^TestStopStaffActionsRecordsInterrupted$' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | STAFF-10 | — | Audit write failure fails closed (no Telegram write) | module | `go test -tags testtools -race -count=1 -run '^TestStaffActionRecordFailureFailsClosed$' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | STAFF-10 | — | `RekeyChat` moves `staff_chat_id`, never `summary_chat_id` | repo | `go test -tags testtools -race -count=1 -run '^TestRekeyChat' ./alita/db/staff` | ✅ extend | ⬜ pending |
| TBD | TBD | TBD | STAFF-09 | — | Log post only to ✅ groups, admin category, paced, failure never changes ✅, no Staff Group title/ID, reason escaped + capped | module | `go test -tags testtools -race -count=1 -run '^TestStaffActionLog' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | STAFF-09 | — | `actionlog.Log` unchanged for existing callers | unit | `go test -tags testtools -race -count=1 ./alita/utils/actionlog` | check | ⬜ pending |
| TBD | TBD | TBD | STAFF-11 | — | Undo decision matrix and never-lift invariants | table + property (pure) | `go test -tags testtools -race -count=1 -run '^TestStaffUndoDecision(Table\|Invariants)$' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | STAFF-11 | — | Restore-prior end to end over member / left / restricted / kicked priors | module | `go test -tags testtools -race -count=1 -run '^TestStaffUndoRestores' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | STAFF-11 | — | Only ✅ groups touched; changed-since and unlinked skipped | module | `go test -tags testtools -race -count=1 -run '^TestStaffUndo(OnlyApplied\|ChangedSince\|LinkRemoved)$' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | STAFF-11 | — | Presser rights checked live per group; outsider refused | module | `go test -tags testtools -race -count=1 -run '^TestStaffUndo(PresserRights\|Outsider)$' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | STAFF-11 | — | Confirm card presser-only, expiry, cancel; shared target lock | module + miniredis | `go test -tags testtools -race -count=1 -run '^TestStaffUndo(Card\|TargetLock)' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | STAFF-11 | — | One undo per action across replicas | module, concurrent | `go test -tags testtools -race -count=1 -run '^TestStaffUndoOnce' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | STAFF-11 | — | Result is a reply; original edited; post-rekey plain message | module | `go test -tags testtools -race -count=1 -run '^TestStaffUndo(Reply\|OriginalEdit\|AfterRekey)' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | STAFF-11 | — | Undo button rules (no kick, no ✅, undone, unfinished) | module | `go test -tags testtools -race -count=1 -run '^TestStaffUndoButton' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | STAFF-11 | — | `#STAFF_UNDO` log per succeeded group | module | `go test -tags testtools -race -count=1 -run '^TestStaffUndoLog' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | STAFF-11 | — | Shutdown during an undo finalizes it | module | `go test -tags testtools -race -count=1 -run '^TestStopStaffActionsUndo' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | SETUP-09 | — | History list: newest first, 10/page, offset paging, this Staff Group only | module | `go test -tags testtools -race -count=1 -run '^TestStaffHistoryList' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | SETUP-09 | — | Detail view per-group lines, undo outcome, unlinked marker, Undo rules | module | `go test -tags testtools -race -count=1 -run '^TestStaffHistoryDetail' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | SETUP-09 | — | Members only; Back to panel | module | `go test -tags testtools -race -count=1 -run '^TestStaffHistoryAccess' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | SETUP-09 | — | Length cap and 64-byte callback budget | pure | `go test -tags testtools -race -count=1 -run '^TestStaffHistory(LengthCap\|CallbackBudget)' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | SETUP-09 | — | Panel renders the new button | pure | `go test -tags testtools -race -count=1 -run '^TestStaffPanelRender' ./alita/modules` | ✅ update | ⬜ pending |
| TBD | TBD | TBD | All | — | Phase 2 regression net | module | `go test -tags testtools -race -count=1 -run '^TestStaffAction\|^TestStopStaffActions' ./alita/modules` | ✅ | ⬜ pending |
| TBD | TBD | TBD | All | — | Locale parity; every reason has text | unit | `make check-translations` | ✅ | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `alita/db/staff/actions_test.go` — repository tests (create, prior, result, finalize, claim, list/paging/tally, rekey)
- [ ] Add `&models.StaffAction{}, &models.StaffActionGroup{}` to the three `AutoMigrate` lists (`alita/db/staff/testmain_test.go`, `alita/modules/test_harness_test.go`, `alita/db/testmain_test.go`)
- [ ] Extend `staffActionFake` (`alita/modules/staff_action_fake_test.go`) with full permission bits in `getChatMember` JSON and `restrictChatMember` storage; honour `use_independent_chat_permissions`
- [ ] Extend `staffCleanup` (`alita/modules/staff_helpers_test.go`) to delete audit rows
- [ ] `staffActionEnv` helpers: `undoCard()`, `setLogChannel(group, channelID)`, `recordOf(token)`
- [ ] Update `panelSplitKeyboard` for the new "Recent actions" action code

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

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 60s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
