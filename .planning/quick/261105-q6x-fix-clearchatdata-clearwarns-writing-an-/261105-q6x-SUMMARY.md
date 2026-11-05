---
phase: 261105-q6x-fix-clearchatdata-clearwarns-writing-an-
plan: 01
subsystem: backup
tags: [backup, reset, warns, postgres, check-constraint, test-postgres-integrity]
status: complete
provides:
  - clearWarns stores the default warn mode (mute) so an all-modules reset commits on PostgreSQL
  - TestClearWarnsRestoresFreshChatDefaults (new) and TestClearChatData_AllModules in test-postgres-integrity
affects: [backup, warns, Makefile, AGENTS.md]
key-files:
  modified:
    - fuku/db/backup/backup.go
    - fuku/db/backup/backup_test.go
    - Makefile
    - AGENTS.md
requirements-completed: []
---

# Quick task 261105-q6x: a reset must store a valid warn mode on PostgreSQL

## Outcome

`clearWarns` now writes `WarnMode: defaultWarnMode` (`mute`) next to `WarnLimit: 3`. The legacy `chk_warns_mode` no
longer rejects the reset row, so `ClearChatData` commits on PostgreSQL. Commit `1d433ec`.

## Evidence

Reproduced first, on PostgreSQL 16.15 (throwaway cluster, migration chain applied, 56 migrations):

- Both CHECKs live on `warns_settings`: `chk_warns_mode` (`NULL` or the five modes) and `chk_warn_mode` (also `''`).
- Before: `TestClearChatData_AllModules` and `TestClearModuleData_IndividualModules` failed with
  `new row for relation "warns_settings" violates check constraint "chk_warns_mode" (SQLSTATE 23514)`.
- The new `TestClearWarnsRestoresFreshChatDefaults` was red on PostgreSQL (23514) and on SQLite (stored mode `""`).
- After: all three pass on PostgreSQL and SQLite; the full `fuku/db/backup` and `fuku/db/warns` packages pass on both.
- CI-equivalent `make test-postgres-integrity` (CI env, `FUKU_TEST_DATABASE=true`): all 13 packages `ok`, and both new
  `-run` entries ran and passed.
- `gofmt` clean, `go vet -tags testtools ./fuku/db/backup` ok, `CGO_ENABLED=0 go build ./...` ok,
  `make check-docs` no drift, `make check-translations` complete.

## Findings worth knowing

- The model and migration define **no** default `warn_mode`; the default is the warns repository's `mute` (four
  places). A reset to an empty mode was also wrong at runtime: the warn handler logs "Unknown warn mode" and punishes
  nobody at the limit, on SQLite too.
- The old assertion `assert.Equal(t, "", ...WarnMode)` in `TestClearModuleData_IndividualModules` encoded the bug; it
  now expects `mute`.
- **Not fixed, same root cause:** `importWarns` writes a backup's `warn_settings` as it is, so a backup with no
  `warn_mode` fails on PostgreSQL with the same 23514 and rolls the import back (verified with a temporary test, then
  deleted). What an import should do with a missing mode is a product decision; recorded as a known gap in AGENTS.md.
- **`TestGreetedMembersStayOutOfBackup` is not on this branch** (nor on `main`): `greeted_backup_test.go` exists only
  on `origin/claude/ecstatic-goldberg-qsss05` (326 commits ahead of `main`, unmerged). It was therefore **not** added
  to the Makefile `-run` list here: a name that matches no test passes silently and would show CI a guarantee it does
  not prove. Checked in a throwaway detached worktree of that branch (discarded): as it stands the test fails with the
  same 23514 at `ClearChatData(srcChat, nil)`; with this commit's `backup.go` change it passes on PostgreSQL. That
  branch must take this fix, change the stale `""` assertion at `backup_test.go:520` to `mute`, and add the test name
  to its own `-run` list.

## Deviations

- The quick workflow's planner and executor subagents were not spawned. The task arrived fully specified and hinged on
  one stateful PostgreSQL cluster whose evidence had to be read directly, so the plan and the work were done inline.
  PLAN.md describes what was executed.
- The repo-local `gsd-tools` is missing its vendored `re2js.cjs`, so the global install was used for `init.quick`.
- The commit trailer names `Claude` without a model name or version, because the session rules forbid a model
  identifier in commit messages.

## Cleanup

The throwaway cluster (`/var/lib/postgresql/fuku_pg_261105_q6x`), its Redis, the extra database and the worktree are
removed; no secret was written to a file (trust authentication on a loopback port).
