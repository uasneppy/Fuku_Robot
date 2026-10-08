---
phase: "1"
slug: "staff-group-links"
# status lifecycle: draft (seeded by plan-phase) → validated (set by validate-phase §6)
# audit-milestone §5.5 distinguishes NOT-VALIDATED (draft) from PARTIAL (validated + nyquist_compliant: false) (#2117)
status: validated
nyquist_compliant: true
wave_0_complete: true
created: "2026-10-04"
---

# Phase 1 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.
> Source: `01-RESEARCH.md` §"Validation Architecture".

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go `testing` + testify v1.12.1 (assertions only), real SQLite via `internal/testdb.Run`, miniredis, hand-written `gotgbot.BotClient` fakes. No mock libraries (AGENTS.md) |
| **Config file** | none — build tag `testtools` is mandatory |
| **Quick run command** | `go test -tags testtools -race -count=1 -run '^TestStaff' ./alita/db/staff ./alita/utils/chat_status ./alita/utils/helpers ./alita/modules` |
| **Full suite command** | `make test` (dev/CI). Sandbox equivalent: `go test -tags testtools -race -count=1 -timeout 10m ./...` (`make test` fails here: toolchain lacks `covdata`) |
| **Phase gate extras** | `make check-translations`, `make check-docs`, `go vet -tags testtools ./...` (`make lint` in CI — sandbox golangci-lint refuses go 1.26) |
| **Postgres-only** | `DATABASE_URL=... ALITA_TEST_DATABASE=true go test -tags testtools -race -count=1 -run '^TestStaffExclusivityTrigger' ./alita/db/staff` (CI) |
| **Estimated runtime** | ~20 s quick, ~50 s full |

---

## Sampling Rate

- **After every task commit:** Run the package-level quick command for the touched package(s)
- **After every plan wave:** Run `go test -tags testtools -race -count=1 -timeout 10m ./...` plus `make check-translations`
- **Before `/gsd-verify-work`:** Full suite must be green, plus `make check-translations` and `make check-docs`
- **Max feedback latency:** 60 seconds

---

## Per-Task Verification Map

Seeded per requirement from research; the planner/executor refines Task IDs as plans land.

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 01-01-T1 | 01-01 | 1 | SETUP-01, SETUP-08 | T-01-01, T-01-03 | Only the live creator can set; /staff silent outside a Staff Group | module (tracer) | `go test -tags testtools -race -count=1 -run '^TestStaffTracer' ./alita/modules` | ✅ | ✅ green |
| 01-01-T2 | 01-01 | 1 | D-10 (DB), SETUP-01 | T-01-02 | DB rejects duplicate staff chat, double link, self-link, bad health; skipLocal covers staff keys | repo | `go test -tags testtools -race -count=1 ./alita/db/staff ./alita/db/cache ./alita/db` | ✅ | ✅ green |
| 01-01-T3 | 01-01 | 1 | PLAT-03 | T-01-04 | CheckOwner errors never count as mismatch | unit + parity | `go test -tags testtools -race -count=1 -run '^TestCheckOwner' ./alita/utils/chat_status` and `go test -tags testtools -count=1 -run '^TestStaffLocaleKeys$' ./alita/i18n` | ✅ | ✅ green |
| 01-02-T1 | 01-02 | 2 | SETUP-02 | T-01-06, T-01-07 | Only the live creator removes status; links go in the same tx; double confirm posts once | module + repo | `go test -tags testtools -race -count=1 -run '^TestUnsetStaff' ./alita/modules` | ✅ | ✅ green |
| 01-02-T2 | 01-02 | 2 | SETUP-01 | T-01-05 | Anonymous and service senders rejected; bot status tri-state | unit | `go test -tags testtools -race -count=1 -run '^TestRejectAnonymousSender' ./alita/utils/helpers` and `-run '^TestFetchBotMember' ./alita/utils/chat_status` | ✅ | ✅ green |
| 01-02-T3 | 01-02 | 2 | SETUP-01 | T-01-05, T-01-08 | Refusal matrix in a fixed order; linked group refused (D-10) | module | `go test -tags testtools -race -count=1 -run '^TestSetStaff' ./alita/modules` | ✅ | ✅ green |
| 01-03-T1/T2 | 01-03 | 2 | SETUP-07 | T-01-09, T-01-10 | Re-key idempotent in either order; constraints hold; watcher never swallows updates | repo + module | `go test -tags testtools -race -count=1 -run '^TestRekeyChat' ./alita/db/staff` and `-run '^TestStaffMigrate' ./alita/modules` | ✅ | ✅ green |
| 01-04-T2 | 01-04 | 2 | SETUP-04 (D-10) | T-01-11, T-01-12 | PostgreSQL trigger rejects role overlap, including concurrent | Postgres-gated (skips on SQLite) | `go test -tags testtools -count=1 -run '^TestStaffExclusivityTrigger$' -v ./alita/db/staff` (must SKIP here); CI: `ALITA_TEST_DATABASE=true make test-postgres-integrity` | ✅ | ✅ green |
| 01-05-T1/T2/T3 | 01-05 | 3 | SETUP-03, SETUP-04 | T-01-13..T-01-18 | Payload never grants authority; nothing posted to the linked group; no spam or probing of foreign Staff Groups; one row under races | module + repo | `go test -tags testtools -race -count=1 -run '^TestLinkStaff\|^TestStaffPicker\|^TestStartGroupPayload' ./alita/modules` and `-run '^TestStaffRepoCreateLink' ./alita/db/staff` | ✅ | ✅ green |
| 01-06-T1/T2 | 01-06 | 4 | SETUP-05 | T-01-19..T-01-21 | Unlink authority re-checked on both presses; stale or wrong-chat callback refused | module | `go test -tags testtools -race -count=1 -run '^TestUnlinkStaff\|^TestStaffUnlinkButton' ./alita/modules` | ✅ | ✅ green |
| 01-07-T1/T2 | 01-07 | 5 | SETUP-06 | T-01-22..T-01-25 | Only a definite mismatch unlinks; exactly-once notice; payload is a hint | module + repo | `go test -tags testtools -race -count=1 -run '^TestStaffOwnership' ./alita/modules` | ✅ | ✅ green |
| 01-08-T1/T2 | 01-08 | 6 | SETUP-06, SETUP-08 (D-14) | T-01-26, T-01-27 | One heads-up per health change; recovery without relink | module + repo | `go test -tags testtools -race -count=1 -run '^TestStaffHealth' ./alita/modules` | ✅ | ✅ green |
| 01-09-T1/T2 | 01-09 | 7 | SETUP-08 | T-01-28..T-01-31 | Panel live per open and Refresh; members only; at most 3800 UTF-16 units in every locale; titles escaped | module + pure | `go test -tags testtools -race -count=1 -run '^TestStaffPanel' ./alita/modules` | ✅ | ✅ green |
| 01-10-T1/T2/T3 | 01-10 | 7 | SETUP-06, SETUP-07 | T-01-32..T-01-34 | Errors never delete; one sweeper across replicas (miniredis); clean lifecycle | module + repo, miniredis | `go test -tags testtools -race -count=1 -run '^TestStaffSweep' ./alita/modules` | ✅ | ✅ green |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [x] `alita/db/models/staff.go` and the migration (needed before any repository test compiles)
- [x] `alita/db/staff/testmain_test.go` using `testdb.Run(m, &models.StaffGroup{}, &models.StaffGroupLink{})`
- [x] Add `&models.StaffGroup{}`, `&models.StaffGroupLink{}` to `alita/modules/test_harness_test.go` and `alita/db/testmain_test.go` AutoMigrate lists
- [x] `staffBotClient` fake (per-chat creators, bot roles, scripted `TelegramError` incl. `ResponseParams.MigrateToChatId` and 429), `//go:build testtools`
- [x] Staff-specific `i18n.OverrideManagerForTest` YAML with marker strings for every key
- [x] `TestStaffLocaleKeys` parity test (reads `locales/*.yml` with `yaml.v3`)
- [x] Postgres-gated trigger test and its entry in the Makefile `test-postgres-integrity` regex
- [x] Framework install: none (testify, miniredis already present)

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Picker works when bot already member/admin; rights not reduced; `/start@bot stf_...` arrives with picker as sender | SETUP-03 | Needs real Telegram clients (A3, A12) | Use "Add group" in `/staff` on a group where bot is already admin; inspect rights and the arriving message |
| `chat_owner_changed` and/or `chat_member` arrive on ownership transfer (bot admin) | SETUP-06 | Telegram behaviour undocumented (A1, A5) | Transfer ownership of a throwaway supergroup; capture raw updates; confirm link removed and one notice posted |
| Same transfer with bot as plain member; `getChatAdministrators` still lists creator | SETUP-06 | Telegram behaviour (A2, A5) | Repeat with bot demoted to member |
| Owner with "remain anonymous" runs picker and `/setstaff` | SETUP-01, SETUP-03 | Client behaviour (A4) | Enable anonymity, run both flows, confirm refusal text |

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references
- [x] No watch-mode flags
- [x] Feedback latency < 60s
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** approved 2026-10-05 (validate-phase audit, State A)

---

## Validation Audit 2026-10-05

Every row's automated command was run on this branch (SQLite, `-race`): 169 top-level tests passed, 0 failed. The only skip was `TestStaffExclusivityTrigger`, which is PostgreSQL-only. It was then run against a local PostgreSQL 16 using the CI sequence (`TestRepositoryMigrationChain`, then `ALITA_TEST_DATABASE=true make test-postgres-integrity`). All 32 top-level tests passed, 0 skipped. That includes all 5 subtests of `TestStaffExclusivityTrigger`, among them the concurrent race. Both staff migrations were recorded once in `schema_migrations` with checksums. `make check-translations` passed. PR #1 had no CI check runs at audit time, so the CI run is still outstanding.

| Metric | Count |
|--------|-------|
| Gaps found | 0 |
| Resolved | 0 |
| Escalated | 0 |
