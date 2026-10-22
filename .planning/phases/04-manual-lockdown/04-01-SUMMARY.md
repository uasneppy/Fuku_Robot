---
phase: 04-manual-lockdown
plan: 01
subsystem: lockdown
tags: [telegram, postgres, gorm, lockdown, permissions, raw-json, gotgbot]

requires:
  - phase: 03-staff-actions
    provides: "live getChatMember authority (staffIssuerSkipReason), telegramErrorDetail, staffFullName, the staffActionFake test fixture, the fresh-read audit-record pattern"
provides:
  - "Migration 20261006120000: chat_lockdowns, chat_lockdown_joiners and the uk_chat_lockdowns_active partial unique index"
  - "alita/db/lockdown repository: Start, GetActiveFresh, GetFresh, ConfirmLocked, DeleteUnconfirmed, BeginLift, FinishLift"
  - "Raw getChat / setChatPermissions helpers and the 16-key locked set (lockdown_perms.go)"
  - "/lockdown [reason] and /unlockdown with a live getChatMember authority check"
  - "lockdownFake, newLockdownEnv and lockdownCleanup test fixtures"
affects: [04-02 anonymous admins, 04-03 join guard and worker, 04-04 lift report, 04-05 lockdownstatus, 04-06 LOCK-08 unmute, 04-07 staff panel marker, 04-08 UAT]

actuals:
  tokens: 27800
  tasks: 2
  commits: 5

tech-stack:
  added: []
  patterns:
    - "Group permissions are read and written as raw JSON (json.RawMessage through bot.RequestWithContext), never gotgbot.ChatPermissions"
    - "One active row per chat from a partial unique index plus ON CONFLICT DO NOTHING and RowsAffected"
    - "User text spliced into a translated string after translation through tokens"

key-files:
  created:
    - migrations/20261006120000_add_chat_lockdowns.sql
    - alita/db/models/lockdown.go
    - alita/db/lockdown/repository.go
    - alita/db/lockdown/repository_test.go
    - alita/db/lockdown/testmain_test.go
    - alita/modules/lockdown.go
    - alita/modules/lockdown_perms.go
    - alita/modules/lockdown_perms_test.go
    - alita/modules/lockdown_fake_test.go
    - alita/modules/lockdown_test.go
    - alita/modules/lockdown_refusals_test.go
    - docs/src/content/docs/commands/lockdown/index.md
  modified:
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
    - Makefile
    - AGENTS.md

key-decisions:
  - "Lockdown state is never cached: every read is a fresh query, so a new lockdown is enforced by every replica at once and a Redis flush cannot lose it"
  - "The snapshot is the getChat permissions member kept as raw bytes and replayed verbatim; the lock sends a constant with all 16 keys explicit false"
  - "Both tables are created by this one migration, with the joiner columns Phase 5 needs, so later plans add no migration"
  - "/lockdown is refused in a basic group, because a ban there does not keep anyone out"
  - "/lockdown and /unlockdown are not Disableable"

patterns-established:
  - "requireLockdownAuthority(needRestrict): a helpers.CheckFunc that never reads the admin cache"
  - "Refusal replies carry Telegram's own (escaped) error text through a token, and a refusal writes nothing"

requirements-completed: [LOCK-02, LOCK-03, LOCK-06, LOCK-07, LOCK-09]

plan_head_before: c0e9a579d15af9d42cad76f845a07297b53a9124
plan_head_after: 7ca9bd1e539e2f2bfa866482a7d2b8e365d54636

coverage:
  - id: D1
    description: "/lockdown stores the raw getChat permissions, sets all 16 default permissions off with use_independent_chat_permissions, and announces only after Telegram answered true"
    requirement: "LOCK-02"
    verification:
      - kind: integration
        ref: "alita/modules/lockdown_test.go#TestLockdownCommandStarts"
        status: pass
      - kind: integration
        ref: "alita/modules/lockdown_test.go#TestLockdownLockSendsLockedSet"
        status: pass
    human_judgment: true
    rationale: "The fake proves what the bot sends; whether a real supergroup ends up muted needs the live check in 04-VERIFICATION (tracer human-check, not run here)"
  - id: D2
    description: "/unlockdown replays the stored permissions byte for byte and records who lifted"
    requirement: "LOCK-07"
    verification:
      - kind: integration
        ref: "alita/modules/lockdown_test.go#TestLockdownLiftRestoresExactSnapshot"
        status: pass
    human_judgment: true
    rationale: "Exactness against a real getChat answer (research A1: does Telegram write every key, false included) can only be shown with a captured live fixture"
  - id: D3
    description: "The lock notice names who locked it, gives the reason, says approved users are muted too, and the lock makes no per-user call"
    requirement: "LOCK-03"
    verification:
      - kind: integration
        ref: "alita/modules/lockdown_refusals_test.go#TestLockdownNoticeAndNoPerUserCalls"
        status: pass
    human_judgment: false
  - id: D4
    description: "Only a live creator or administrator with can_restrict_members can lock or lift; the admin cache never authorizes; a failed lookup refuses"
    requirement: "LOCK-06"
    verification:
      - kind: integration
        ref: "alita/modules/lockdown_refusals_test.go#TestLockdownCommandsRefuseNonAuthority"
        status: pass
    human_judgment: false
  - id: D5
    description: "Refusals (basic group, bot cannot restrict or cannot be checked, permissions unreadable, lock refused) record nothing; a second /lockdown reports the first"
    requirement: "LOCK-09"
    verification:
      - kind: integration
        ref: "alita/modules/lockdown_refusals_test.go#TestLockdownRefusals"
        status: pass
      - kind: integration
        ref: "alita/modules/lockdown_refusals_test.go#TestLockdownCommandReportsExisting"
        status: pass
    human_judgment: false
  - id: D6
    description: "At most one active lockdown per chat, on SQLite and on PostgreSQL 16 with the migration applied"
    requirement: "LOCK-09"
    verification:
      - kind: unit
        ref: "alita/db/lockdown/repository_test.go#TestStartLockdownOneActivePerChat (SQLite)"
        status: pass
      - kind: integration
        ref: "alita/db/lockdown/repository_test.go#TestStartLockdownOneActivePerChat (PostgreSQL 16) and alita/db/migrations#TestRepositoryMigrationChain"
        status: unknown
    human_judgment: true
    rationale: "The PostgreSQL run was blocked in this worktree (see Issues Encountered); the orchestrator must run it"

duration: ~45min
completed: 2026-10-07
status: complete
---

# Phase 4 Plan 01: Lock and lift a group, tracer Summary

**/lockdown stores a group's raw getChat permissions, mutes everyone but admins with an all-false 16-key set, and /unlockdown replays the stored JSON verbatim, backed by a PostgreSQL state machine with one active lockdown per chat.**

## Performance

- **Duration:** about 45 min (start time was not captured, so this is an estimate)
- **Completed:** 2026-10-07T01:15Z
- **Tasks:** 2 (a tracer and an auto task, both with tdd="true")
- **Files:** 24 changed (12 created, 12 modified), 2205 insertions
- **Commits:** 5 (measured: `git rev-list --count c0e9a579..7ca9bd1`)

## Accomplishments

- Migration 20261006120000 creates `chat_lockdowns` and `chat_lockdown_joiners`, the `uk_chat_lockdowns_active` partial unique index, and every joiner column Phase 5 needs, so no later plan adds a migration.
- The snapshot is the permissions member of a raw `getChat` answer, stored before the lock call and replayed verbatim with `use_independent_chat_permissions=true`; it never goes through `gotgbot.ChatPermissions`.
- `/lockdown [reason]` announces only after `setChatPermissions` returned `true`, makes no per-user restrict call, and says approved users are muted too. A second `/lockdown` makes no Telegram write and reports the first one.
- `/lockdown` refuses, recording nothing, in a basic group, when the bot cannot restrict members or cannot be checked, when the group's permissions are unreadable or missing, and when Telegram refuses the lock (the unconfirmed row is deleted). A missing delete or invite right only adds a note.
- Authority is `requireLockdownAuthority`, a live `getChatMember` (creator, or administrator with `can_restrict_members`); the admin cache is never read and a failed lookup refuses.
- 17 new locale keys (the 16 planned plus `lockdown_state_failed`) in all 7 locale files, a generated docs page, an `alt_names` entry, the Makefile PostgreSQL list and the AGENTS.md rules.

## Task Commits

1. **Task 1 (tracer): lock and lift end to end**
   - RED `c19b9a1` test(04-01): three tests fail on their first assertion (compile scaffolding only)
   - GREEN `25a0ed2` feat(04-01): migration, repository, raw helpers, commands, locales
2. **Task 2: authority, refusals, one active lockdown per chat**
   - RED `30d21a5` test(04-01): `TestLockdownRefusals` fails on all nine subtests
   - GREEN `c55f14f` feat(04-01): refusals, notes, help text, docs, Makefile, AGENTS.md
3. **Follow-up** `7ca9bd1` fix(04-01): never slice the arguments of a command message that has no words

## Files Created/Modified

See the frontmatter `key-files`. The behaviour lives in `alita/modules/lockdown.go` (commands, authority check, notice composition), `alita/modules/lockdown_perms.go` (raw JSON helpers) and `alita/db/lockdown/repository.go` (state machine).

## Decisions Made

- Fresh reads only, no cache (the staff audit-record precedent), so no `DeleteCache` and no `skipLocal` entry; stated in AGENTS.md.
- Basic groups are refused (research A6, Pitfall 9).
- The reason is cut to 300 runes and names to 64, stored raw, HTML-escaped at render and spliced in after translation through tokens.
- `/unlockdown` finishes the lift at once when no joiner row is unfinished (`FinishLift`); later plans' worker finishes the rest.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing critical functionality] Generic failure reply and defensive argument slicing**
- **Found during:** Task 1 and the final review
- **Issue:** The plan's key list has no text for "a database read or write failed, or Telegram refused the restore", and `ctx.Args()[1:]` panics on a message with no words.
- **Fix:** Added the locale key `lockdown_state_failed` (all 7 files), used when the state cannot be read or written and when a restore call fails. Added `lockdownCommandArgs`, `lockdownHasPermissions` (a permissions member must be a JSON object) and the `lockdownDetailToken` splice for Telegram's error text.
- **Files modified:** `alita/modules/lockdown.go`, `locales/*.yml`
- **Commits:** `25a0ed2`, `c55f14f`, `7ca9bd1`

**2. [Process] Tracer human-check not run as a blocking checkpoint**
- **Found during:** Task 1 tracer feedback gate
- **Issue:** The tracer's `<verify>` carries a `<human-check>` (set unusual permissions in a real supergroup, lock, lift, compare, capture the raw `getChat` JSON as a fixture). Under `end-of-phase` mode the precedence chain says STOP with a `checkpoint:human-verify`. This executor was dispatched to finish the whole plan and write the SUMMARY in a parallel worktree, and the check needs a live Telegram supergroup this agent cannot reach.
- **Fix:** Re-ran every automated tracer verify command end to end (all passed) and carried on. The human-check was not run and is not claimed as passed: it is listed below for the verifier's UAT.
- **Impact:** LOCK-07 exactness is proven against the fake only. Research assumptions A1 (does Telegram write every permission key) stays open.

**3. [Process] PostgreSQL verification could not be run in this worktree**
- See "Issues Encountered".

### TDD note

Task 2's RED commit contains five test groups; only `TestLockdownRefusals` failed before the implementation. The authority, already-active, notice, `TestCanonicalPermissions` and repository tests describe behaviour the Task 1 tracer had already built (authority check, `Start`, `canonicalPermissions`), so they passed on first run. They are characterisation tests of the tracer, not RED tests. The Task 1 and Task 2 RED commits also carry compile scaffolding (models, no-op repository functions, the locked-set constant, an empty `LoadLockdown`) that the GREEN commits replace.

**Total deviations:** 1 auto-fixed (Rule 2), 2 process deviations. **Impact:** no scope creep; the two process items need an owner or orchestrator action.

## Issues Encountered

- **PostgreSQL 16 tests were not run.** The plan's last verify command starts a throwaway cluster with `su postgres -c ...`. The execution harness refused `su` and `runuser` in this worktree-isolated agent, and no PostgreSQL server is running. I did not use the sandbox override. Not run: `TestRepositoryMigrationChain` and `TestStartLockdownOneActivePerChat` on PostgreSQL. The migration SQL was reviewed by hand against `runner.go`'s statement splitter and the plan's schema, and the SQLite run of the same test passes, but the partial unique index, `ON CONFLICT DO NOTHING` without a target (research A9) and the migration checksum have not been exercised on PostgreSQL. Run from the main checkout:
  `PGBIN=/usr/lib/postgresql/16/bin; D=$(mktemp -d); chown postgres "$D"; su postgres -c "$PGBIN/initdb -D $D/data -A trust >/dev/null && $PGBIN/pg_ctl -D $D/data -o '-p 55432 -k $D -c listen_addresses=' -l $D/log start -w >/dev/null && $PGBIN/createdb -h $D -p 55432 chain" && DSN="host=$D port=55432 user=postgres dbname=chain sslmode=disable" && ALITA_TEST_MIGRATION_CHAIN=true DATABASE_URL="$DSN" go test -tags testtools -v -count=1 -run '^TestRepositoryMigrationChain$' ./alita/db/migrations && ALITA_TEST_DATABASE=true DATABASE_URL="$DSN" go test -tags testtools -v -count=1 -run '^TestStartLockdownOneActivePerChat$' ./alita/db/lockdown; R=$?; su postgres -c "$PGBIN/pg_ctl -D $D/data stop -m fast" >/dev/null 2>&1; rm -rf "$D"; exit $R`
  Expect `--- PASS: TestRepositoryMigrationChain` and `lockdown repository backend: postgres`.
- `make lint` could not run (golangci-lint 2.5.0 was built with go1.25 and refuses the go1.26.0 module). `gofmt -l` on every touched Go file prints nothing and `go vet -tags testtools` is clean.
- `gsd_run` is not available in this agent, so the broken-windows ledger (`.planning/WINDOWS.md`) was not appended and STATE/ROADMAP/REQUIREMENTS were not touched (the orchestrator owns them). Requirement IDs LOCK-02, LOCK-03, LOCK-06, LOCK-07 and LOCK-09 are shared with later plans, so none should read complete from this plan alone.

## Verification Results

| Check | Result |
|-------|--------|
| `TestLockdownCommandStarts`, `TestLockdownLockSendsLockedSet`, `TestLockdownLiftRestoresExactSnapshot` (`-race -count=1`) | pass |
| `TestLockdownCommandsRefuseNonAuthority`, `TestLockdownRefusals`, `TestLockdownCommandReportsExisting`, `TestLockdownNoticeAndNoPerUserCalls`, `TestCanonicalPermissions` | pass |
| `TestStartLockdownOneActivePerChat`, `TestLockdownRepositoryTransitions` on SQLite | pass |
| `go test -tags testtools -race -count=1 ./alita/modules ./alita/db/lockdown ./alita/db` | pass |
| `make test` (whole repository, run before the final one-line fix) | exit 0, no `--- FAIL`; PostgreSQL-only tests skipped as the result checker allows |
| Lockdown tests re-run after the final fix | pass |
| `CGO_ENABLED=0 go build ./...`, `go vet -tags testtools` on touched packages | clean |
| `make check-translations`, `make check-docs` | pass, no drift |
| `git diff --exit-code go.mod go.sum` | clean |
| PostgreSQL 16 migration chain and `TestStartLockdownOneActivePerChat` | not run (see above) |
| `make lint` | cannot run in this environment |
| Tracer human-check (real supergroup) | not run, deferred to UAT |

Task acceptance greps all pass: migration is the newest file, two `CREATE TABLE IF NOT EXISTS chat_lockdown*`, the partial unique index text matches, no top-level `BEGIN`/`COMMIT`, no cache calls in the repository, no `gotgbot.ChatPermissions` in `lockdown_perms.go`, no admin-cache calls in `lockdown.go`, the model name in all three AutoMigrate lists, 17 `lockdown_` keys in every locale file, and the `test(04-01)` commits precede their `feat(04-01)` commits.

## Known Stubs

None in code. Two things read ahead of behaviour that later plans add, and should not ship alone:
- The lock notice, `lockdown_started`, and the help text say new members are removed until the lift and unbanned at the lift. The join guard and the lift worker arrive in plan 04-03; until then `/lockdown` only mutes.
- `locales/config.yml` lists the alias `lockdownstatus`, which plan 04-05 creates, and the generated docs page lists it under "Module Aliases".
- A failed restore at `/unlockdown` replies with the generic `lockdown_state_failed` and leaves the lockdown active; the D-21 explanation with Telegram's reason is a later plan's.

## Threat Flags

None. No new network endpoint, auth path, file access or trust-boundary schema beyond the plan's threat model (T-04-01 to T-04-05 are mitigated and tested).

## Human Check Deferred (tracer)

In a test supergroup set unusual default permissions (for example text on, photos off, reactions off, invite on), run `/lockdown` then `/unlockdown`, and compare the Permissions screen before and after. Save the raw `getChat` permissions JSON seen during the run as a test fixture (research A1) and a banned `getChatMember` (research A2).

## Next Phase Readiness

- Ready: the state machine, joiner table and raw permission helpers 04-02 to 04-08 build on; `lockdownModule.handlerGroup` is already `-7` for the join guard.
- Needs action before relying on it: run the PostgreSQL commands above on a machine that allows `su`.

## Self-Check: PASSED

All 12 created files exist on disk and the five commits (`c19b9a1`, `25a0ed2`, `30d21a5`, `c55f14f`, `7ca9bd1`) are present in `git log c0e9a579..HEAD`. The PostgreSQL and live-Telegram items listed under Issues Encountered and Human Check Deferred remain open; this self-check covers only files and commits.

---
*Phase: 04-manual-lockdown*
*Completed: 2026-10-07*
