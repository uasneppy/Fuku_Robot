---
phase: "1"
slug: "staff-group-links"
# status lifecycle: draft (seeded by plan-phase) → validated (set by validate-phase §6)
# audit-milestone §5.5 distinguishes NOT-VALIDATED (draft) from PARTIAL (validated + nyquist_compliant: false) (#2117)
status: draft
nyquist_compliant: false
wave_0_complete: false
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
| TBD | TBD | TBD | SETUP-01 | — | Only live creator, bot admin, group/supergroup; anonymous sender refused with zero Telegram writes | module | `go test -tags testtools -race -count=1 -run '^TestSetStaff' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | SETUP-02 | — | Only live creator removes status; links removed in same tx | module | `go test -tags testtools -race -count=1 -run '^TestUnsetStaff' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | SETUP-03 | — | Payload never grants authority; no message to linked group | module | `go test -tags testtools -race -count=1 -run '^TestLinkStaff\|^TestStartGroupPayload' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | SETUP-04 | — | Live owner-of-both, fail closed on API error, no spam into foreign Staff Group | repo + module | `go test -tags testtools -race -count=1 -run '^TestCreateLink' ./alita/db/staff` and `-run '^TestLinkStaffRefusals' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | SETUP-05 | — | Unlink authority re-checked per press; stale/wrong-chat callback refused | module | `go test -tags testtools -race -count=1 -run '^TestUnlinkStaff\|^TestStaffCallback' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | SETUP-06 | — | Only definite mismatch unlinks; exactly-once notice; single sweeper | module + repo, miniredis | `go test -tags testtools -race -count=1 -run '^TestStaffOwnership\|^TestStaffSweep' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | SETUP-07 | — | Re-key idempotent in either order; constraints hold after re-key | repo + module | `go test -tags testtools -race -count=1 -run '^TestRekey' ./alita/db/staff` and `-run '^TestStaffMigrate' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | SETUP-08 | — | Panel only in Staff Group; titles escaped; under 4096 chars | pure + module | `go test -tags testtools -race -count=1 -run '^TestStaffPanel\|^TestStaffCommand' ./alita/modules` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | PLAT-03 | — | N/A | parity + checker | `go test -tags testtools -count=1 -run '^TestStaffLocaleKeys' ./alita/i18n` and `make check-translations` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | D-10 (DB) | — | DB rejects self-link, double link, role overlap | repo; Postgres-gated | `go test -tags testtools -race -count=1 -run '^TestStaffConstraints' ./alita/db/staff` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `alita/db/models/staff.go` and the migration (needed before any repository test compiles)
- [ ] `alita/db/staff/testmain_test.go` using `testdb.Run(m, &models.StaffGroup{}, &models.StaffGroupLink{})`
- [ ] Add `&models.StaffGroup{}`, `&models.StaffGroupLink{}` to `alita/modules/test_harness_test.go` and `alita/db/testmain_test.go` AutoMigrate lists
- [ ] `staffBotClient` fake (per-chat creators, bot roles, scripted `TelegramError` incl. `ResponseParams.MigrateToChatId` and 429), `//go:build testtools`
- [ ] Staff-specific `i18n.OverrideManagerForTest` YAML with marker strings for every key
- [ ] `TestStaffLocaleKeys` parity test (reads `locales/*.yml` with `yaml.v3`)
- [ ] Postgres-gated trigger test and its entry in the Makefile `test-postgres-integrity` regex
- [ ] Framework install: none (testify, miniredis already present)

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

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 60s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
