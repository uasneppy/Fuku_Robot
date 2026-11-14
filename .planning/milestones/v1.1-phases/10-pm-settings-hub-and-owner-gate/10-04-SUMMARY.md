---
phase: 10-pm-settings-hub-and-owner-gate
plan: 04
subsystem: testing
tags: [tests, harness, settings, postgres-integrity, agents-md, plat-05]
requires:
  - phase: 10-pm-settings-hub-and-owner-gate
    provides: menuaccess and adminindex repositories and models from plan 10-01
provides:
  - default settings test environment seeds admins_allowed true through the real menuaccess repository
  - fixtures newOwner, setAdminsAllowed, listAdmin, otherReplica on settingsEnv and listAdministrator, setDeletedCreator on the bot fake
  - TestSettingsEnvSeedsAdminAccess
  - make test-postgres-integrity runs TestAdminIndexReplaceConcurrent and TestMenuAccessUpsert
  - AGENTS.md documents both new tables, the new fixtures and says 8 locales
affects: [10-05, 10-06, 10-07, 10-08, 10-09, 10-10]

status: complete
actuals:
  tokens: 6500
  tasks: 2
  commits: 2

commits: 2
plan_head_before: ff71605
plan_head_after: d1ee0a4cf31b9352f25937fd2234f5dd43fc64cd

tech-stack:
  added: []
  patterns:
    - "The default settings environment is an admin the owner let in (switch seeded on); gate tests set the switch explicitly"
    - "A second replica is a second lockdownFake, bot and dispatcher over the same database and miniredis, update IDs from 50000"

key-files:
  created:
    - fuku/modules/settings_env_harness_test.go
  modified:
    - fuku/modules/test_harness_test.go
    - fuku/modules/settings_env_test.go
    - fuku/modules/staff_helpers_test.go
    - Makefile
    - AGENTS.md

key-decisions:
  - "clearCreator already existed in staff_ownership_test.go with the exact behaviour the plan asks for, so the existing method is reused and no second one was added to staff_helpers_test.go"
  - "AGENTS.md also lists the new fixtures and the two Phase 10 PostgreSQL guarantees in the Testing section, beside the Data bullets and locale counts the plan named"

requirements-completed: [PLAT-05, MENU-10, MENU-14]

coverage:
  - id: D1
    description: "Every existing settings and staff test passes unchanged after the harness seeds admins_allowed true"
    requirement: MENU-10
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestSettings|TestLoadSettings|TestStaff)' ./fuku/modules ./fuku/i18n"
        status: pass
    human_judgment: false
  - id: D2
    description: "Fixtures for the seed, cleanup, listed administrator, deleted creator, cleared creator, owner and second replica are proven"
    requirement: MENU-14
    verification:
      - kind: unit
        ref: "fuku/modules/settings_env_harness_test.go#TestSettingsEnvSeedsAdminAccess"
        status: pass
    human_judgment: false
  - id: D3
    description: "The PostgreSQL integrity run lists plan 10-01's two guarantees with their packages and keeps every earlier name"
    requirement: MENU-14
    verification:
      - kind: other
        ref: "make -n test-postgres-integrity prints TestAdminIndexReplaceConcurrent, TestMenuAccessUpsert, ./fuku/db/adminindex ./fuku/db/menuaccess and TestCaptchaModeTurnstileConstraint"
        status: pass
    human_judgment: false
  - id: D4
    description: "AGENTS.md describes chat_settings_access and chat_admin_index and states the locale count as 8 everywhere"
    requirement: PLAT-05
    verification:
      - kind: other
        ref: "! grep -nE '\\b7 (locale|language)|all 7 (files|locales|languages)|the 7 languages|all 7$' AGENTS.md"
        status: pass
    human_judgment: false
---

# Phase 10 Plan 04: Settings test harness, integrity list and AGENTS.md rules Summary

The default settings environment now holds an allowed admin through the real `menuaccess` switch, the owner, listed-admin, deleted-creator and second-replica fixtures exist for plans 10-05 onward, CI's PostgreSQL run covers plan 10-01's two guarantees, and AGENTS.md documents both tables and says 8 locales.

## Accomplishments

- `newSettingsEnvWith` and `settingsEnv.addChat` call `setAdminsAllowed(chatID, true)`, so every Phase 9 press test stays green once the owner gate lands. `settingsCleanup` deletes the `chat_settings_access` and `chat_admin_index` rows of its chats; both models joined the modules AutoMigrate list.
- `staffBotClient` gained `listAdministrator` (an administrator appears in `getChatAdministrators` only on opt-in, after the creator and before the bot) and `setDeletedCreator` (creator entry "Deleted Account", no last name, no username). A chat that opts in to neither gets the same admin list as before, byte for byte.
- `settingsEnv` gained `setAdminsAllowed`, `newOwner`, `listAdmin` and `otherReplica` (own fake, bot and dispatcher over the shared database and miniredis, update IDs from 50000).
- `TestSettingsEnvSeedsAdminAccess` pins the seed, the cleanup, the unchanged admin list and every fixture.
- `make test-postgres-integrity` lists `TestAdminIndexReplaceConcurrent` and `TestMenuAccessUpsert` and the packages `./fuku/db/adminindex ./fuku/db/menuaccess`.
- AGENTS.md: two Data bullets (fresh-read policy, per-chat advisory lock of `ReplaceChat`, `ListForUserFresh` rule, backup status), a Testing bullet for the fixtures, the Phase 10 integrity guarantees, and all six locale counts changed from 7 to 8 with step 7 of "Adding a module" listing `en, es, fr, hi, id, pt, ru, uk`.

## Task Commits

1. Task 1 (tracer): harness seed and fixtures - `8862ff8`
2. Task 2: integrity list and AGENTS.md - `d1ee0a4`

Tracer gate: the tracer `<verify>` is automated only (`end-of-phase` mode), so it was re-run end to end before expansion and passed.

## Verification Results

- `go test -tags testtools -race -count=1 -run '^TestSettingsEnvSeedsAdminAccess$' -v ./fuku/modules` - PASS, all 10 subtests.
- `go test -tags testtools -race -count=1 -run '^(TestSettings|TestLoadSettings|TestStaff)' ./fuku/modules ./fuku/i18n` - `ok fuku/modules 190.758s`, `ok fuku/i18n 6.033s` (every Phase 9 press test green).
- `go test -tags testtools -race -count=1 -run '^(TestAdminIndexReplaceConcurrent|TestMenuAccessUpsert)$' ./fuku/db/adminindex ./fuku/db/menuaccess` - both PASS on SQLite, so the names in the Makefile list are real.
- Acceptance greps: `models.ChatSettingsAccess{}` 1, `models.ChatAdminIndex{}` 1, `menuaccess.SetAdminsAllowed` 1; Makefile `fuku/db/adminindex ./fuku/db/menuaccess` 1, both new names on one line 1, `TestCaptchaModeTurnstileConstraint` still 1; AGENTS.md `chat_settings_access` 1, `chat_admin_index` 2, `TestAdminIndexReplaceConcurrent` 1, `en, es, fr, hi, id, pt, ru, uk` 1; the old-locale-count grep prints nothing.
- `make -n test-postgres-integrity` shows both new names and packages; `CGO_ENABLED=0 go build ./...` passes; `git diff --exit-code 6ef6ad9 -- go.mod go.sum` is clean.
- The PostgreSQL run itself was not executed here (no PostgreSQL in this environment); it runs in CI.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] `clearCreator` already declared**
- **Found during:** Task 1
- **Issue:** `staffBotClient.clearCreator` already exists in `fuku/modules/staff_ownership_test.go` with the behaviour the plan describes (remove the chat from `creators`); a second declaration did not compile.
- **Fix:** Dropped the duplicate from `staff_helpers_test.go` and reuse the existing method. `TestSettingsEnvSeedsAdminAccess` covers it.
- **Files modified:** fuku/modules/staff_helpers_test.go
- **Commit:** 8862ff8

**Total deviations:** 1 auto-fixed (1 blocking). **Impact:** none on behaviour; one fewer helper than the plan listed in `staff_helpers_test.go`.

## Known Stubs

None.

## Self-Check: PASSED

- Created file present: `fuku/modules/settings_env_harness_test.go`.
- Commits `8862ff8` and `d1ee0a4` are ancestors of HEAD.
- All task acceptance criteria and plan-level verification commands above re-run and passing.
