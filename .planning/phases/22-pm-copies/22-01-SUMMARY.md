---
phase: 21-pm-copies
plan: 01
subsystem: database
status: complete
tags: [migration, repository, cache, skipLocal, backup, pm-copies]
requires: [21-02]
provides:
  - chat_event_message_options table (migration 20261120120000)
  - models.ChatEventMessageOptions
  - eventmsg.DestPM, eventmsg.DefaultSilent
  - eventmsg.Options, Messages.Options, (*Messages).Resolve
  - eventmsg.SetPMDetails, eventmsg.SetPMLog
  - the pm insert default of the part writers
affects: [22-02, 22-03, 22-04, 22-05, 22-06, 22-07, 22-08, 22-09, 22, 23, 29]
tech-stack:
  added: []
  patterns:
    - "a destination-dependent default of a switch, applied by the one upsert helper on insert only (the conflict update never lists the column)"
    - "a second table read into the same cached struct, under the same skipLocal key, with both writers dropping it"
    - "an error path proved by renaming the table for one subtest and back in t.Cleanup (works on SQLite and PostgreSQL)"
key-files:
  created:
    - migrations/20261120120000_add_event_message_options.sql
    - fuku/db/migrations/event_message_options_migration_test.go
  modified:
    - fuku/db/models/event_message.go
    - fuku/db/eventmsg/events.go
    - fuku/db/eventmsg/repository.go
    - fuku/db/eventmsg/repository_test.go
    - fuku/db/eventmsg/testmain_test.go
    - fuku/db/backup/event_messages_backup_test.go
    - fuku/db/backup/testmain_test.go
    - fuku/modules/test_harness_test.go
    - fuku/modules/event_message_env_test.go
    - fuku/modules/settings_messages_env_test.go
    - Makefile
    - AGENTS.md
decisions:
  - "The pm default lives in upsert: a writer whose columns do not include silent inserts row.Silent = DefaultSilent(dest) (false for group, so the group path is unchanged); the conflict update lists only the writer's columns, so an existing row keeps its switch."
  - "GetFresh reads the options with Limit(1).Find instead of First, so a chat with no options row is no error at all (nothing for the GORM logger, no ErrRecordNotFound branch); any other error is returned wrapped with the chat ID."
  - "Options writers share one helper, upsertOptions, mirroring upsert; each writer drops event_messages:<chat> itself, so the six drops are visible per writer."
  - "The AGENTS.md Data bullets (pm destination, DefaultSilent, Resolve, the options table) went into the task 2 commit, the one that made the old bullet false, not task 3."
metrics:
  duration: ~35 min
  completed: 2026-11-19
actuals:
  tokens: 9700
  tasks: 3
  commits: 3
plan_head_before: d0fae0e98bfb7adeeac458af94f1db2b0da25d31
plan_head_after: 8993f7ba4951bc8082d35d143a18101decc29b96
---

# Phase 22 Plan 01: Private copy and options storage Summary

A private copy of an event is a `chat_event_messages` row with `dest = 'pm'` that is off until its switch is written
(`DefaultSilent("pm")` is true, a part writer that inserts a new `pm` row stores `silent = true`, `Resolve` reads a missing
row as off), and the group's Details and log options live in the new `chat_event_message_options` table, read by `GetFresh`
into `Messages.Options` under the same cached `event_messages:<chat>` key and written by two column-targeted upserts that
drop it. Nothing of it reaches a backup.

## What later plans use

| Kind | Name |
|------|------|
| Table | `chat_event_message_options`, PK `pk_chat_event_message_options (chat_id)`; columns `pm_details`, `pm_log` (BOOLEAN, default false), `updated_by` (default 0), `updated_at` (default now()), all `NOT NULL`, no CHECK, no foreign key |
| Model | `models.ChatEventMessageOptions` (`ChatID`, `PMDetails`, `PMLog`, `UpdatedBy`, `UpdatedAt`; no `default:` tag), `TableName()` = `chat_event_message_options` |
| Destination | `eventmsg.DestPM = "pm"`; `eventmsg.Dests` is now `[group, pm]`; `eventmsg.DefaultSilent(dest string) bool` (true only for `DestPM`) |
| Read | `eventmsg.Options{PMDetails, PMLog bool}`; field `Messages.Options`; `(*Messages).Resolve(event, dest string) Message` (stored row, else `Message{Event, Dest, Silent: DefaultSilent(dest)}`; nil-safe). `Get` and `GetFresh` signatures unchanged |
| Write | `eventmsg.SetPMDetails(ctx, chatID int64, on bool, by int64) error`, `eventmsg.SetPMLog(ctx, chatID int64, on bool, by int64) error`; `SetText`, `SetMedia`, `SetButtons`, `SetSilent` accept `DestPM` unchanged |
| Cache key | unchanged: `eventmsg.CacheKey(chatID)`, in `skipLocal`; six writers drop it |

Rules a later plan must keep:

- The send path reads a message with `Resolve`, never `Lookup` plus its own default: `Lookup` reports "no row" for a fresh
  `pm` key, and treating that as "not silent" would send a copy the owner never switched on.
- `SetSilent(chat, event, DestPM, false, by)` is the only way to turn a copy on. The page's switch (`hp`, plan 22-04) writes
  it; a typed text, media or buttons write never does.
- A value cached by the previous build has no `Options` and decodes with both options off (the safe direction).

## Tasks

| Task | Name | Commit |
|------|------|--------|
| 1 | options table, model, migration tests | fdd6c3d |
| 2 | pm destination, its default, Resolve, options in the one read, writers, AGENTS.md Data | a71de84 |
| 3 | backup exclusion, harness lists, cleanups, PostgreSQL list, AGENTS.md Testing | 8993f7b |

## Verification

- `go test -tags testtools -race -count=1 -run '^(TestEventMessageOptionsMigrationStatements|TestEventMessageOptionsMigrationDefaults|TestChatEventMessagesMigrationStatements)$' -v ./fuku/db/migrations`: Statements PASS, Defaults SKIP on SQLite (the allowed skip).
- `go test -tags testtools -race -count=1 -run '^(TestEventMessages|TestEventMessagesUpsert|TestEventMessagesPM)$' -v ./fuku/db/eventmsg`: PASS; `TestEventMessagesPM` 7 subtests, `TestEventMessagesUpsert` 2 new subtests, no SKIP.
- `go test -tags testtools -race -count=1 ./fuku/db/eventmsg ./fuku/db/cache ./fuku/db/backup ./fuku/db/migrations`: all ok.
- `go test -tags testtools -race -count=1 -run 'TestEventMessage.*|TestLegacyModerationRequests|TestMessagesEndToEnd|TestSettingsMessages.*' ./fuku/modules`: ok, 25 top-level tests PASS, no FAIL or SKIP (31 s).
- PostgreSQL 16 (a throwaway local cluster): `TestRepositoryMigrationChain` with `FUKU_TEST_MIGRATION_CHAIN=true` ok, then the whole `make test-postgres-integrity` exit 0 (19 packages ok, no SKIP), including `TestEventMessageOptionsMigrationDefaults`, `TestEventMessagesUpsert` and its two new subtests; `TestEventMessagesPM` (its `errors` subtest renames the table on PostgreSQL too) and `TestEventMessagesStayOutOfBackup` also PASS on PostgreSQL.
- Two mutations fail the tests that guard them: removing the pm insert default fails `a text alone does not switch it on` and `pm default under concurrent writers`; removing `SetPMDetails`'s cache drop fails `one key`.
- golangci-lint v2.13.1 (`go install` with go1.26.11, `CGO_ENABLED=0`): `run --new-from-rev=9940d97 ./...` 0 issues, and with `--build-tags=notesttools` 0 issues.
- `CGO_ENABLED=0 go build ./...` ok; `gofmt -l` empty on every touched Go file; `make check-docs` no drift.
- `git diff --exit-code 9940d97 -- go.mod go.sum main.go fuku/db/backup/backup.go fuku/db/backup/types.go migrations/20261119120000_add_chat_event_messages.sql fuku/db/migrations/runner.go scripts/migrate_psql.sh` exits 0. `TestLegacyModerationRequests` and `TestSettingsDeletionPhase16ScreensUnchanged` untouched.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Test assumption] Phase 21 table test expected exactly one destination**
- **Found during:** Task 2
- **Issue:** `TestEventMessages/the_event_and_destination_tables` asserted `Dests` is exactly `[group]` and `ValidDest("pm")` false, and `an_unknown_event_or_destination_writes_nothing` used `"pm"` as its unknown destination; both are false by design once `DestPM` exists.
- **Fix:** the table test now expects `[group, pm]` (and refuses `PM`, `private`, `log`, ...), and the unknown-destination test uses `"log"`. Neither is a golden.
- **Commit:** a71de84

**2. [Ordering] AGENTS.md Data bullets in the task 2 commit**
- The plan put every AGENTS.md edit in task 3; the repo rule says AGENTS.md changes go in the commit that invalidates the rule, and task 2 made the `chat_event_messages` bullet ("the destination `group`") false. The Data bullets (the `pm` default and the new `chat_event_message_options` bullet) went into a71de84; task 3 added the Testing paragraph and the cleanup note.

**3. [Naming] Task 3's commit type**
- The plan suggested `feat(22-01): store private copies and their group options` for task 3; its diff is tests, the Makefile and AGENTS.md only, so it is `test(22-01): keep private copies and options out of backups`. The `feat` commits are tasks 1 and 2.

**4. [Choice] Options read with `Limit(1).Find`**
- The plan's assumption named `First` with `ErrRecordNotFound` read as zero; `Find` gives the same result with no error branch. Behaviour is as planned.

## Not verified

- Nothing of the send path, the page or the deep link: they are later plans.

## Notes for plan 22-02

- Read through `eventmsg.Get` and `msgs.Resolve(event, eventmsg.DestPM)`; `Silent` true means "do not send". A failed `Get` returns an error and the plan's rule (D-11) is "default group reply, no private copy".
- `msgs.Options.PMDetails` and `msgs.Options.PMLog` are in the same struct, so the send path needs no second read.
- A test that stores a copy and wants it on must call `SetSilent(..., DestPM, false, ...)` after the part writes; `eventMsgCleanup` and `settingsMsgCleanup` already delete the options row.
- The modules harness migrates `ChatEventMessageOptions`; the `fuku/db/eventmsg` testmain does too.

## Known Stubs

None.

## Threat Flags

None: no endpoint or trust boundary beyond the plan's threat model; T-22-01 to T-22-04 are mitigated and tested as planned (`TestEventMessagesPM` "a text alone does not switch it on", "one key", "errors"; `TestEventMessagesStayOutOfBackup`).

## Self-Check: PASSED

- Files: the 2 created files exist; the 12 modified files carry their edits.
- Commits: fdd6c3d, a71de84, 8993f7b are ancestors of HEAD.
