---
phase: 04-manual-lockdown
plan: 08
subsystem: staff-actions
tags: [lockdown, staff-panel, staff-actions, telegram, locales, agents-md, phase-gate]

requires:
  - phase: 04-manual-lockdown
    provides: "chat_lockdowns / chat_lockdown_joiners repository, joiner ban_until marker, HasJoinerBanFresh, lockdown worker and lift (plans 04-01..04-07)"
provides:
  - "lockdown.ListActiveByChatsFresh: one-query batch of confirmed active lockdowns (chat -> locked_at)"
  - "/staff panel lockdown line on a locked linked group's row (staffLinkRow.LockedSince, markLockedRows)"
  - "staffTargetState.LockdownBan and the staffLockdownBanLookup seam: a staff ban replaces a lockdown's own ban"
  - "TestLockdownLocaleKeys: lockdown locale parity and placeholder guard for all 7 locales"
  - "AGENTS.md final pass matched to the shipped lockdown code"
affects: [phase-05-web-captcha, phase-06-antiraid, phase-09-settings-menu]

actuals:
  tokens: 14000
  tasks: 3
  commits: 5

plan_head_before: 608d321d561e67be99c8e91187e5f1241f35b69e
plan_head_after: dc84b223aeb716f422dcfda65582d9f6315f6aa3

tech-stack:
  added: []
  patterns:
    - "Panel decoration loaded as one batch query after the live checks, so the renderer stays pure"
    - "A per-group decision flag filled by an injectable lookup seam, with a failed lookup failing the group closed"

key-files:
  created:
    - alita/modules/lockdown_staff_test.go
    - alita/i18n/lockdown_locale_test.go
  modified:
    - alita/db/lockdown/repository.go
    - alita/db/lockdown/repository_test.go
    - alita/modules/staff_panel.go
    - alita/modules/staff_panel_render_test.go
    - alita/modules/staff_action_decide.go
    - alita/modules/staff_action_decide_test.go
    - alita/modules/staff_action_run.go
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml
    - docs/src/content/docs/commands/staff/index.md
    - AGENTS.md

key-decisions:
  - "The marker shows locked_at (confirmation time) in UTC as '2 Jan 15:04' on one extra line after the status line; one batch query per panel build keeps renderStaffPanel pure, and a failed query shows the panel unmarked"
  - "LockdownBan is the last field of staffTargetState and is looked up only for a staff ban on a kicked target with a non-zero end date, so no other kind pays a query"
  - "The staff ban that replaces a lockdown ban gets its own (possibly shorter) end date; that is what makes the lift keep it"

requirements-completed: [SETUP-08, LOCK-07]

duration: 18min
completed: 2026-10-07
status: complete
---

# Phase 4 Plan 08: Staff panel lockdown marker, staff ban over a lockdown ban, phase gate Summary

**The /staff panel now marks each confirmed-locked linked group with one UTC "🔒 in lockdown since" line (never the reason or starter), a staff /ban or /tban over a lockdown's own ban replaces it so the lift keeps it, and the phase gate is green except the two steps this environment cannot run (make lint, PostgreSQL 16).**

## Performance

- **Duration:** about 18 min wall clock
- **Completed:** 2026-10-07T03:48Z
- **Tasks:** 3
- **Files modified:** 17 (2 created, 15 modified)

## Accomplishments

- `/staff` shows `🔒 in lockdown since 5 Oct 12:04` (UTC) on exactly the rows of linked groups whose lockdown is active and confirmed. Unconfirmed lockdowns and lifts in progress show nothing. A locked group the bot was removed from shows both the bot-missing problem and the marker (D-22). The reason and starter never reach the panel (D-17).
- A staff `/ban` or `/tban` on someone whose live ban is a lockdown's own ban (its end date matches a joiner row's `ban_until`) is now sent even when it ends sooner. It carries its own end date, so the lockdown's lift sees a ban that is not its own and keeps it (research Pitfall 4, D-05). A failed lookup fails that group with an internal error and no write. All other ban decisions are unchanged.
- `TestLockdownLocaleKeys` guards every `lockdown_` key plus `greetings_join_request_lockdown` and `mutes_unmute_lockdown_note` across all 7 locales (parity both ways, non-empty, same placeholder set).
- AGENTS.md was re-read bullet by bullet against the code; four statements the code contradicted or left imprecise were fixed (see Deviations).

## Task Commits

1. **Task 1 (tracer): /staff marks locked groups**
   - RED: `cb8d741` test(04-08): show /staff does not mark locked groups
   - GREEN: `0083d06` feat(04-08): mark locked groups in the /staff panel
2. **Task 2: staff ban replaces a lockdown's ban**
   - RED: `634a6c8` test(04-08): show a staff ban on a lockdown joiner is skipped and later lifted
   - GREEN: `ccafe9b` fix(04-08): replace a lockdown's ban with a staff ban so the lift keeps it
3. **Task 3: locale parity test, final AGENTS.md pass, phase gate** - `dc84b22` docs(04-08): final AGENTS.md pass and lockdown locale parity test

**Plan metadata:** committed separately as `docs(04-08): complete ...` (this file).

## TDD Gate Compliance

- Task 1: RED commit precedes GREEN. RED was run with the minimum compile stubs (the `LockedSince` field and an empty `ListActiveByChatsFresh`) uncommitted in the working tree, so the target tests failed on their behavior assertions and not on a compile error. The stubs were committed with the GREEN commit.
  - `TestListActiveByChats`: "ListActiveByChatsFresh = map[], want only the confirmed active chat"
  - `TestStaffPanelShowsLockedGroup`: "panel carries 0 lockdown lines, want 2 (Alpha and Bravo)"
  - `TestRenderStaffRowLockdownMarker`: three of four subtests failed on the missing line
- Task 2: same method (stubs: the `LockdownBan` field and the unused `staffLockdownBanLookup` seam). RED failures were `TestDecideStaffBanLockdownJoiner` (got skip_already_banned, want banned), `TestStaffActionDecisionInvariants` ("a staff ban over a lockdown ban was not sent"), `TestStaffBanOnLockdownJoinerSurvivesLift` (no banChatMember to the locked group) and `TestStaffBanLockdownLookupFails` (group skipped as already banned, not failed).
- Task 3 added only a guard test (`TestLockdownLocaleKeys`); all lockdown locale keys were already complete, so it passed on its first run. There is no RED for it.

## Files Created/Modified

- `alita/db/lockdown/repository.go` - `ListActiveByChatsFresh`
- `alita/modules/staff_panel.go` - `LockedSince`, the lockdown line in `renderStaffRow`, `markLockedRows` called from `buildStaffPanel`
- `alita/modules/staff_action_decide.go` - `staffTargetState.LockdownBan` and its branch in `decideStaffBan`
- `alita/modules/staff_action_run.go` - `staffLockdownBanLookup` seam and its use in `runStaffActionInGroup`
- `alita/modules/lockdown_staff_test.go` - panel integration test, survive-the-lift test, failed-lookup test (new)
- `alita/i18n/lockdown_locale_test.go` - locale parity test (new)
- `locales/*.yml` - `staff_panel_row_lockdown` and the extended `staff_help_msg` in all 7 files
- `docs/src/content/docs/commands/staff/index.md` - regenerated
- `AGENTS.md` - final pass

## Decisions Made

Followed the plan's recorded decisions (marker = `locked_at` in UTC; one batch query; `LockdownBan` appended last and set only for a staff ban on a kicked target with an end date). One extra: the panel marker is also documented as its own AGENTS.md bullet, because nothing described it.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] AGENTS.md contradicted the shipped code in four places**
- **Found during:** Task 3 (final AGENTS.md pass)
- **Issue:** (a) antiraid's step-aside was described as "while a lockdown is active", but the code requires a confirmed one (`LockedAt != nil`); (b) the join guard bullet did not say it acts only on a confirmed lockdown, which every guard path does; (c) `decideLockdownJoin` was described as exempting only a live creator or administrator, but it also exempts an anonymous admin; (d) the Testing bullet said the same three harness lists carry the lockdown models, but the staff package's `testmain_test.go` does not, while `alita/db/lockdown/testmain_test.go` does.
- **Fix:** Corrected each statement; added the panel marker bullet; cross-referenced the staff exception from the lift bullet.
- **Files modified:** AGENTS.md
- **Commit:** `dc84b22`

**Total deviations:** 1 auto-fixed (documentation accuracy). **Impact:** none on code.

## Authentication Gates

None.

## Verification Results

| Check | Result |
| --- | --- |
| `TestRenderStaffRowLockdownMarker`, `TestStaffPanelShowsLockedGroup`, `TestStaffPanelRenderFitsInEveryLocale`, `^TestStaffPanel` (-race) | PASS |
| `TestListActiveByChats` (-race) | PASS |
| `TestDecideStaffBanLockdownJoiner`, `^TestStaffActionDecision`, `TestStaffBanOnLockdownJoinerSurvivesLift`, `TestStaffBanLockdownLookupFails` (-race) | PASS |
| `^TestStaffAction`, `^TestStopStaffActions`, `^TestStaffUndo` (-race, 63 s) | PASS (the known-flaky `tmute_as_a_reply` did not fail) |
| `TestLockdownLocaleKeys`, `TestStaffLocaleKeys` | PASS |
| `make test` (full, with race, coverage and skip gate) | PASS, exit 0, no FAIL lines (modules 140 s, 70.1% coverage) |
| `make check-translations` | PASS ("All translations are present!") |
| `make generate-docs` then `make check-docs` | PASS, no drift, `git status --porcelain docs/` empty |
| `CGO_ENABLED=0 go build ./...` | PASS |
| `go vet -tags testtools ./alita/modules ./alita/db/lockdown ./alita/i18n` | PASS, no output |
| `git diff --exit-code go.mod go.sum` | PASS, unchanged |
| `gofmt -l` on touched files | clean (see note below) |
| `make lint` | NOT RUN: `Error: can't load config: the Go language version (go1.25) used to build golangci-lint is lower than the targeted Go version (1.26.0)` (installed golangci-lint 2.5.0 was built with go1.25) |
| PostgreSQL 16 migration chain + `TestStartLockdownOneActivePerChat` | NOT RUN: the harness refused `su postgres` ("this command runs su in a plain command ... Refusing to run it"). The `TestStartLockdownOneActivePerChat` SQLite run is part of the green `make test`. |

Note: `gofmt -l alita/ main.go` lists `alita/modules/greetings_command_test.go`. That file is not in this plan's diff (pre-existing), so it was left alone.

### Acceptance criteria re-run

- Task 1: `LockedSince time.Time` present at `staff_panel.go:56`; `lockdown.ListActiveByChatsFresh(` present at `staff_panel.go:472`; the `renderStaffRow` body contains 0 occurrences of `StartedBy`; `staff_panel_row_lockdown:` appears once in each of es, fr, hi, id, pt, ru; `git log` shows the `test(04-08)` commit before the `feat(04-08)` commit.
- Task 2: `LockdownBan bool` and `var staffLockdownBanLookup = lockdown.HasJoinerBanFresh` present; `LockdownBan` appears in AGENTS.md; gofmt clean on the four files.
- Task 3: `make test` exit 0; `make check-docs` exit 0 with no docs drift; `` `-7` lockdown join guard `` and `alita:lockdown:*` each present in AGENTS.md; `go.mod`/`go.sum` unchanged. `make lint` exit 0 could not be met (environment, see above).

## Items for the orchestrator

**Unrun commands (phase gate, need a shell where `su postgres` is allowed):**

```
PGBIN=/usr/lib/postgresql/16/bin; D=$(mktemp -d); chown postgres "$D"; su postgres -c "$PGBIN/initdb -D $D/data -A trust >/dev/null && $PGBIN/pg_ctl -D $D/data -o '-p 55432 -k $D -c listen_addresses=' -l $D/log start -w >/dev/null && $PGBIN/createdb -h $D -p 55432 chain" && DSN="host=$D port=55432 user=postgres dbname=chain sslmode=disable" && ALITA_TEST_MIGRATION_CHAIN=true DATABASE_URL="$DSN" go test -tags testtools -v -count=1 -run '^TestRepositoryMigrationChain$' ./alita/db/migrations && ALITA_TEST_DATABASE=true DATABASE_URL="$DSN" go test -tags testtools -v -count=1 -run '^TestStartLockdownOneActivePerChat$' ./alita/db/lockdown; R=$?; su postgres -c "$PGBIN/pg_ctl -D $D/data stop -m fast" >/dev/null 2>&1; rm -rf "$D"; exit $R
```

Expected: a `--- PASS: TestRepositoryMigrationChain` line, a `lockdown repository backend: postgres` line, and no `--- SKIP`.

**`make lint`:** unrun here for the toolchain reason above; run it where golangci-lint v2 is built with Go 1.26. New code was kept to the existing patterns and is gofmt- and vet-clean.

**End-of-phase UAT (human checks that need a live Telegram supergroup):**

1. Tracer check: lock a linked group, open `/staff` in the Staff Group and confirm the 🔒 line appears on that group's row only, with no reason and no name; lift the lockdown, press Refresh, and confirm the line is gone.
2. Everything in `04-VALIDATION.md` manual-only list: joins on all three paths handled once; an admin approval of a pre-lockdown request; the getChat permissions round trip and the `until_date` echo saved as fixtures.
3. Staff ban over a lockdown ban in a real group: lock a group, let a test account be banned by the lockdown, send `/tban <id> 1d` from the Staff Group, lift the lockdown and confirm the account is still banned in that group.

## Known Stubs

None.

## Threat Flags

None. No new network endpoint, auth path or schema change; the only new reads are two fresh queries over existing lockdown tables (T-04-25 and T-04-26 from the plan's threat model are mitigated by the tests named above).

## Next Phase Readiness

Phase 4 plan 08 is the last plan of the phase. Ready for the orchestrator's merge, the PostgreSQL run above and phase verification.

## Self-Check: PASSED

- Created files exist: `alita/modules/lockdown_staff_test.go`, `alita/i18n/lockdown_locale_test.go`.
- Commits exist: `cb8d741`, `0083d06`, `634a6c8`, `ccafe9b`, `dc84b22` (`git rev-list --count 608d321..HEAD` = 5, measured from the persisted ledger).
