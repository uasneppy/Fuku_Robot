---
phase: 01-staff-group-links
plan: 01
subsystem: staff-groups
tags: [go, gotgbot, gorm, postgres, migrations, i18n, telegram]

requires: []
provides:
  - "staff_groups and staff_group_links tables (append-only migration 20261004120000) with DB-enforced uniqueness, self-link and health constraints"
  - "models.StaffGroup, models.StaffGroupLink, models.StaffHealth* constants"
  - "staff repository: CreateStaffGroup (idempotent), GetStaffGroup (cached gate with not-found sentinel), GetStaffGroupFresh (authority read), invalidateStaffKeys"
  - "chat_status.CheckOwner: live, uncached, tri-state (OwnerMatch / OwnerMismatch / OwnerUnknown) creator check"
  - "Staff module with /setstaff (creator only) and /staff (silent outside a Staff Group), pure renderStaffPanel"
  - "staffBotClient test harness, withStaffLocale marker locale, runStaffCommand, staffCleanup, uniqueStaffChatID, cleanupStaffRows"
  - "TestStaffLocaleKeys parity test across en es fr hi id pt ru"
affects: [01-02, 01-03, 01-04, 01-05, 01-06, 01-09, phase-02, phase-03, phase-09]

actuals:
  tokens: 13500
  tasks: 3
  commits: 4

plan_head_before: 51ccb5849ae3066945cb2ea3917d158c75e34675
plan_head_after: 50eb7763f1b2d79c91b75f10732d08debb8988ee

tech-stack:
  added: []
  patterns:
    - "Authority via live getChatAdministrators, never the cached admin list; error maps to Unknown, never Mismatch"
    - "Cached gate with zero-value sentinel plus invalidate-after-write; gate prefixes listed in skipLocal"
    - "Marker locale (withStaffLocale) built from en.yml so tests assert which key a reply used"
    - "Hand-written per-chat BotClient fake with scripted TelegramErrors (staffBotClient)"

key-files:
  created:
    - migrations/20261004120000_add_staff_groups_and_links.sql
    - alita/db/models/staff.go
    - alita/db/staff/repository.go
    - alita/db/staff/testmain_test.go
    - alita/db/staff/repository_test.go
    - alita/utils/chat_status/owner.go
    - alita/utils/chat_status/owner_test.go
    - alita/modules/staff.go
    - alita/modules/staff_panel.go
    - alita/modules/staff_helpers_test.go
    - alita/modules/staff_test.go
    - alita/i18n/staff_locale_test.go
    - docs/src/content/docs/commands/staff/index.md
  modified:
    - alita/db/cache/local.go
    - alita/db/cache/local_test.go
    - alita/db/testmain_test.go
    - alita/modules/test_harness_test.go
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml
    - locales/config.yml

key-decisions:
  - "No foreign keys on chat IDs in the staff tables (federation_chats precedent) so SQLite tests match PostgreSQL"
  - "staff_groups.owner_user_id is indexed but not unique (D-09: one owner may run several Staff Groups)"
  - "/staff stays silent outside a Staff Group (D-16) and replies via renderStaffPanel with an empty keyboard until later plans add buttons"
  - "Only literal tr.GetString(\"staff_...\") keys are used, with TestStaffLocaleKeys covering what make check-translations cannot see"

patterns-established:
  - "CheckOwner tri-state: only OwnerMismatch may remove state; OwnerUnknown fails closed and writes nothing"
  - "Every staff repository write calls invalidateStaffKeys after the write; later plans reuse it for re-keys"

requirements-completed: [SETUP-01, SETUP-08, PLAT-03]

coverage:
  - id: D1
    description: "A group creator's /setstaff creates a staff_groups row owned by the sender and confirms; /staff in that group shows help text, chat ID and the empty-list line; /staff elsewhere (other group, private chat) sends nothing"
    requirement: "SETUP-01"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -run '^TestStaffTracerSetStaffThenStaffShowsPanel$' ./alita/modules#TestStaffTracerSetStaffThenStaffShowsPanel"
        status: pass
    human_judgment: false
  - id: D2
    description: "A non-creator's /setstaff and a creator lookup that hits a Telegram error both create no row and send the matching refusal"
    requirement: "SETUP-08"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -run '^TestStaffTracerSetStaffRefusesNonOwnerAndUnknown$' ./alita/modules#TestStaffTracerSetStaffRefusesNonOwnerAndUnknown"
        status: pass
      - kind: unit
        ref: "go test -tags testtools -race -run '^TestCheckOwner' ./alita/utils/chat_status#TestCheckOwnerUnknownOnError"
        status: pass
    human_judgment: false
  - id: D3
    description: "staff_groups and staff_group_links schema: the database rejects duplicate staff chat_id, second link per group, self-link and unknown health; CreateStaffGroup is idempotent; the cached gate honours invalidation; staff cache prefixes bypass the local layer; backup never includes staff data"
    requirement: "SETUP-08"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -run '^TestStaffConstraints|^TestStaffRepo|^TestStaffTablesStayOutOfBackup' ./alita/db/staff"
        status: pass
      - kind: unit
        ref: "go test -tags testtools -race -run '^TestSkipLocalMatchesFreshnessCriticalKeys$' ./alita/db/cache"
        status: pass
    human_judgment: false
  - id: D4
    description: "All 7 locales carry every staff_ key with identical placeholder sets; generated docs page and translation check are clean"
    requirement: "PLAT-03"
    verification:
      - kind: unit
        ref: "go test -tags testtools -run '^TestStaffLocaleKeys$' ./alita/i18n"
        status: pass
      - kind: other
        ref: "make check-translations && make check-docs"
        status: pass
    human_judgment: false
  - id: D5
    description: "The migration applies cleanly on PostgreSQL through AUTO_MIGRATE at deploy time"
    requirement: "SETUP-01"
    verification: []
    human_judgment: true
    rationale: "No PostgreSQL server could be started inside the sandbox; the SQL was only exercised through GORM check tags on SQLite and the migration runner unit tests. First deploy (AUTO_MIGRATE=true) or a local Postgres run is the real proof."

duration: 15min
completed: 2026-10-04
status: complete
---

# Phase 1 Plan 01: Staff Group Walking Skeleton Summary

**An owner turns a supergroup into a Staff Group with `/setstaff` (live creator check via getChatAdministrators, uncached, tri-state) and `/staff` shows its help text and chat ID, backed by a new append-only migration, cached-gate repository, 7-locale strings and a generated docs page.**

## Performance

- **Duration:** about 15 min (start time was not captured at spawn, so this is approximate)
- **Tasks:** 3 (1 tracer, 2 TDD expansion)
- **Files:** 25 changed, 1445 insertions, 6 deletions (measured against the base commit)

## Accomplishments

- Tracer slice end to end: migration, models, repository, `CheckOwner`, `Staff` module registered at priority 236, pure panel renderer, test harness, 7 locales, generated docs. Tracer verify (both tracer tests, `make check-translations`, `make check-docs`) passed before expansion.
- Data layer hardened and pinned by tests: duplicate staff chat, second link per group, self-link and invalid health are all rejected by the database; `CreateStaffGroup` is idempotent; the cached `staff_group:` gate caches a not-found sentinel and is invalidated by the write; both `staff_group:` and `staff_link_of:` prefixes now bypass the in-process cache layer.
- Authority and PLAT-03 guarded: `CheckOwner` match, mismatch (other creator, no creator) and unknown (429, 400, plain error) are pinned, and `TestStaffLocaleKeys` checks key parity both ways, non-empty values and placeholder sets across all 7 locales (fail-first proven by deleting a fr key and by renaming a ru placeholder, both reverted).
- Backup untouched; `TestStaffTablesStayOutOfBackup` pins that no backup module names Staff data.
- Full suite `go test -tags testtools -race -count=1 -timeout 10m ./...` passes, `go vet -tags testtools ./...` is clean, `make check-translations` and `make check-docs` pass, `go.mod` and `go.sum` are unchanged.

## Task Commits

1. **Task 1: tracer, /setstaff then /staff** - `25044bf` (feat)
2. **Task 2 RED: data-layer tests** - `2d7eca5` (test)
3. **Task 2 GREEN: staff prefixes in skipLocal plus backup test** - `296388f` (feat)
4. **Task 3: CheckOwner and locale parity tests** - `50eb776` (test)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Migration comment tripped an acceptance criterion**
- **Found during:** Task 1 acceptance check
- **Issue:** `grep -c "VARCHAR(24)"` expected 1, but my explanatory comment repeated the literal and made it 2.
- **Fix:** Reworded the comment to "24 characters wide". Done before the Task 1 commit.
- **Files modified:** migrations/20261004120000_add_staff_groups_and_links.sql
- **Commit:** 25044bf

**2. [Rule 2 - Missing critical] Test for the backup prohibition**
- **Found during:** Task 2
- **Issue:** The plan's prohibition P5 is marked `verification: test`, but the plan only listed `git diff --exit-code -- alita/db/backup/`, which is a check at commit time and not a regression guard.
- **Fix:** Added `TestStaffTablesStayOutOfBackup` in `alita/db/staff/repository_test.go` (uses `backup.IsValidModule` and `AllExportableModules`, so it asserts behaviour, not source text).
- **Commit:** 296388f

### TDD note

Tasks 2 and 3 are `tdd="true"`, but Task 1 (the tracer) had already created the repository, constraints and `CheckOwner`. In the RED step only `TestSkipLocalMatchesFreshnessCriticalKeys` failed on the planned assertion (staff_group and staff_link_of not excluded). The seven repository and constraint tests and the `CheckOwner` tests passed immediately because they characterise Task 1 code, so they have no failing-first record. The locale parity test has fail-first proof by temporary mutation of locale files (reverted, working tree clean). `workflow.tdd_mode` was off, so no hard gate applied; recorded here for transparency.

**Total deviations:** 2 auto-fixed (1 blocking, 1 missing-critical). **Impact:** none on scope; one extra test.

## Known Stubs

None. The panel's empty keyboard and the always-empty `rows` argument in `staffPanel` are intentional and are filled by plans 01-05, 01-06 and 01-09 (documented in the plan and in `renderStaffPanel`'s GoDoc).

## Threat Flags

None beyond the plan's threat model. T-01-01 (live `CheckOwner` before any write), T-01-02 (`skipLocal` plus invalidate-after-write), T-01-03 (silent `/staff` outside a Staff Group) and T-01-04 (Unknown never mismatches, writes nothing) are each covered by a passing test.

## Issues Encountered

- `gofmt -l` reports `alita/modules/greetings_command_test.go`, a pre-existing file outside this plan's scope; not touched.
- Could not start PostgreSQL in the sandbox to run the migration for real (see coverage D5). `make test` and `make lint` were not run (toolchain limits noted in the plan); the equivalent `go test -tags testtools -race` and `go vet -tags testtools` were.

## Next Phase Readiness

Plan 01-02 can build on `CheckOwner`, `staff.CreateStaffGroup`, `staffBotClient` (creators, botRole, members, failures) and `withStaffLocale`. Every later plan that adds `staff_` keys must re-run `TestStaffLocaleKeys`.

## Self-Check: PASSED

- All created files exist on disk (migration, models/staff.go, db/staff/*, chat_status/owner*.go, modules/staff*.go, i18n/staff_locale_test.go, docs staff page).
- Commits 25044bf, 2d7eca5, 296388f, 50eb776 are on the branch.
- Task acceptance criteria re-run and passing; plan-level verification (full suite, vet, check-translations, check-docs, go.mod/go.sum unchanged) passing.
