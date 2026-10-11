---
phase: 05-lockdown-alerts-and-response
plan: 07
subsystem: lockdown
tags: [lockdown, alerts, burst, join-tracker, migration, i18n]
status: complete
requires:
  - phase: 05-lockdown-alerts-and-response
    provides: alert decision and poster (05-01..05-06), recent-joins tracker and lockdownRecentLookup seam (05-05)
provides:
  - chat_lockdown_burst table (migration 20261011130000) with the whole row shape, including the per-account response columns
  - the tracker's pre-lockdown snapshot saved with the alert decision (AlertPlan.Burst, written only by the BeginAlert winner)
  - lockdown.BurstSampleFresh, BurstAccount, BurstCounts
  - the alert's counts line and a sample of up to 10 accounts, with the "and N more" line
affects: [05-08, 05-10, 05-11, 05-13]
tech-stack:
  added: []
  patterns:
    - "user text built outside the translator and spliced in after it through a token (lockdownSampleToken)"
    - "snapshot of a best-effort Redis structure saved to PostgreSQL in the transaction that decides the alert, so later steps never need Redis"
key-files:
  created:
    - migrations/20261011130000_add_lockdown_burst.sql
    - alita/db/lockdown/burst.go
    - alita/db/lockdown/burst_test.go
    - alita/modules/lockdown_alert_sample_test.go
  modified:
    - alita/db/models/lockdown.go
    - alita/db/lockdown/alerts.go
    - alita/db/lockdown/testmain_test.go
    - alita/modules/lockdown_alert.go
    - alita/modules/test_harness_test.go
    - AGENTS.md
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml
key-decisions:
  - "The snapshot window is (locked_at - 10 minutes, locked_at]; later joins are the lockdown's own joiner rows. A user with a pre row is listed once, as a pre-lockdown account, and is not counted in Since."
  - "chat_lockdown_burst is created with its full shape (state, reason, detail, run_id, attempts, claimed_at) so the ban run of plans 05-08 and 05-10 needs no further migration."
  - "The sample counts every joiner row the lockdown recorded, whatever its state (D-02); the counts and the 'and N more' line are computed from the whole burst, not from the shown lines."
  - "A failed tracker read or a missing Redis saves no burst rows and the alert still posts with before 0 (D-17); a failed sample read at post time leaves the sample out."
requirements-completed: [LOCK-11, LOCK-01]
duration: 45min
completed: 2026-10-11
commits: 3
plan_head_before: ee29884dcbe876c79d6aa7ab188529088003f4c2
plan_head_after: 69b11e3a77cc9866887531f7f383dcd43d52f42f
actuals:
  tokens: 14000
  tasks: 2
  commits: 3
---

# Phase 5 Plan 07: Burst sample on the lockdown alert Summary

**The lockdown alert now shows the raid: the counts behind the trigger (joined in the 10 minutes before the lock, joined since) and up to 10 accounts of the burst, pre-lockdown joiners first, saved in PostgreSQL when the alert is decided so nothing later needs Redis.**

Note: `commits: 3` counts the two test commits and the one implementation commit made before this SUMMARY; the SUMMARY commit follows, so `plan_head_after` is the last code commit.

## What was built

- **Migration `20261011130000_add_lockdown_burst.sql`**: `chat_lockdown_burst`, one row per account of a burst, with `uk_chat_lockdown_burst_user` on `(lockdown_id, user_id)`, source and state CHECK constraints, a SQL-only foreign key to `chat_lockdowns` with `ON DELETE CASCADE` and `idx_chat_lockdown_burst_lockdown_state`. Every statement is idempotent; no transaction statements, no `CONCURRENTLY`. The response columns are created now for plans 05-08 and 05-10.
- **Model (`models/lockdown.go`)**: `ChatLockdownBurst` (`TableName` `chat_lockdown_burst`) and the `BurstSource*` / `BurstState*` constants with GoDoc. Added to the AutoMigrate lists of `alita/modules/test_harness_test.go` and `alita/db/lockdown/testmain_test.go`.
- **`BeginAlert` (`alerts.go`)**: `AlertPlan.Burst`; the winner inserts the burst rows in the same transaction with `ON CONFLICT DO NOTHING`, setting the lockdown ID itself. The loser inserts nothing; an empty burst inserts nothing.
- **`burst.go`**: `BurstSampleFresh(lockdownID, limit)` returns pre rows (by `joined_at`, then ID) followed by the lockdown's joiner rows whose user has no pre row (by ID), cut to `limit`, plus the counts of the whole burst. A limit of 0 returns the counts alone.
- **`lockdown_alert.go`**: `lockdownBurstSnapshot` reads `lockdownRecentLookup(chat, locked_at - lockdownRecentWindow, locked_at)` in the plan step, skips the bot's own ID, and treats an error as empty (warn log). `lockdownRenderSample` / `lockdownRenderSampleAccount` render the counts line, title, up to `lockdownAlertSampleMax` (10) lines and the "and N more" line; names are capped (64 runes), usernames capped (32) and HTML-escaped, the ID is in code tags, a bot gets the localized mark. The account lines are spliced in through `lockdownSampleToken`, never through the translator. `lockdownRenderAlert` reads the sample fresh at post time; a read error leaves it out and the alert still posts.
- **Locale keys in all 7 files**: `lockdown_alert_counts`, `lockdown_alert_sample_title`, `lockdown_alert_sample_more`, `lockdown_alert_bot_mark`.
- **AGENTS.md**: Data bullet for `chat_lockdown_burst`.

## Tests

- `TestLockdownAlertSample` (module): pre-lockdown joiner listed (name escaped, @username, ID in code tags, counts line, burst row with source pre and invite link), joiners after pre (order and counts 1/2), more than ten (exactly 10 lines, "and 2 more", counts 12/0, under 4096 characters, all 12 rows saved), bot marked (bot line only), escaped (names, usernames and splice-token look-alikes never appear raw), no redis (recorded joiner only, before 0, no burst rows), lookup fails (same).
- `TestBurstSample` and `TestBeginAlertBurst` (repository): ordering with B stored before A, B once, counts 2/2, limit 2 / 3 / 0, empty lockdown, another lockdown's rows not read; the winner inserts, the loser inserts nothing, an empty burst inserts nothing, a repeated user is stored once.
- Mutation check: removing the name escaping, the "and N more" count or the bot mark makes the "escaped", "more than ten", "bot marked" and first cases fail.

## Deviations from Plan

### Auto-fixed Issues

None.

### Process notes

- The plan expected a RED-then-fix commit pair for Task 2. The implementation written in Task 1 (ordering, total-based "more" count, escaping, bot mark, failed-lookup path) already satisfied every Task 2 case, so those cases were committed as a single `test(05-07)` commit and there is no `fix(05-07)` commit. The tests were shown to fail when the escaping, the "more" count and the bot mark are removed. This is the same shape as plans 05-05 and 05-06.
- The Task 1 RED commit does not compile (the symbols it tests do not exist yet), which is its failing state.
- `lockdownPlanAlerts` now uses its `*gotgbot.Bot` parameter (it was ignored before) to skip the bot's own ID in the snapshot; the tracker already skips the bot's own join, so this is a second guard.

## Verification

- `go test -tags testtools -race -count=1 -run '^TestLockdownAlertSample$' -v ./alita/modules`: pass (7 subtests).
- `go test -tags testtools -race -count=1 -run '^TestBurstSample$|^TestBeginAlert$' -v ./alita/db/lockdown`: pass (`TestBeginAlertBurst` also passes).
- `go test -tags testtools -race -count=1 -run 'Lockdown|Unlockdown' ./alita/modules ./alita/db/lockdown`: pass (49 s).
- `go test -tags testtools -count=1 -run '^TestLockdownLocaleKeys$' ./alita/i18n`, `make check-translations`, `make check-docs`, `CGO_ENABLED=0 go build ./...`, `go vet -tags testtools ./alita/modules ./alita/db/lockdown ./alita/db/models`, `go test -tags testtools ./alita/db/migrations`: clean.
- Acceptance greps pass: last migration is `20261011130000_add_lockdown_burst.sql`; no `BEGIN/COMMIT/ROLLBACK/CONCURRENTLY`; `models.ChatLockdownBurst{}` once in each AutoMigrate list; `lockdownRecentLookup(` and `lockdownAlertSampleMax = 10` present; `go.mod` and `go.sum` unchanged. `gofmt -l` on the touched files prints nothing (it lists `alita/modules/greetings_command_test.go`, which this plan did not touch).
- `golangci-lint` was not run: the installed build targets Go 1.25 and cannot lint Go 1.26 code (go vet and gofmt used instead).
- Not run: the whole `./alita/modules` package under `-race`, any PostgreSQL check (including the human-check of applying the migration on PostgreSQL 16 after `20261011120000`; the SQL was checked only by reading and by the SQLite AutoMigrate of the same model), and a real Telegram post.

## Threat surface

No surface beyond the plan's threat model. T-05-20 is covered by the "escaped" case (names, usernames and token look-alikes escaped, spliced after the translator); T-05-21 by "more than ten" (10 lines, names 64 runes, usernames 32, reason 300; the alert stays under 4096 characters).

## Known Stubs

None. The response columns of `chat_lockdown_burst` (state, reason, detail, run_id, attempts, claimed_at) are unused by this plan on purpose; plans 05-08 and 05-10 move them.

## Self-Check: PASSED

`migrations/20261011130000_add_lockdown_burst.sql`, `alita/db/lockdown/burst.go`, `alita/db/lockdown/burst_test.go` and `alita/modules/lockdown_alert_sample_test.go` exist; commits 300cc45, e6e457b and 69b11e3 are on the branch.
