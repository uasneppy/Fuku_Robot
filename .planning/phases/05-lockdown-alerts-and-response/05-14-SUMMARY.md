---
phase: 05-lockdown-alerts-and-response
plan: 14
subsystem: lockdown
tags: [lockdown, docs, i18n, agents-md, postgres-integrity, phase-gate]
status: complete
requires:
  - phase: 05-lockdown-alerts-and-response
    provides: the alert, its Lift, Ban and Revoke buttons and the hourly reminders (05-01..05-13)
provides:
  - lockdown_help_msg and staff_help_msg describing the alert, the three buttons, who may press them and the reminders, in all 7 languages
  - regenerated lockdown and staff docs pages
  - test-postgres-integrity running TestBeginAlert and TestBanRunStart
  - AGENTS.md checked against the shipped Phase 5 code
  - the full Phase 5 gate run, including PostgreSQL 16
affects: []
tech-stack:
  added: []
  patterns:
    - "help text describes only what the call does: Ban is for good with no end date, nothing is said about the banned accounts' messages"
key-files:
  created: []
  modified:
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml
    - docs/src/content/docs/commands/lockdown/index.md
    - docs/src/content/docs/commands/staff/index.md
    - Makefile
    - AGENTS.md
    - alita/modules/greetings_command_test.go
key-decisions:
  - "The three button lines of lockdown_help_msg are plain lines led by the button's emoji, not x-bullets: TestLockdownStringsHaveBalancedTags requires exactly three x-bullets, each starting with a code-tagged command."
  - "Blank lines separate the alert paragraph and each button line, so the generated docs render them as separate paragraphs instead of one run-on block."
  - "AGENTS.md keeps tables, columns and Redis keys under Data; the alert, delivery, refresh and reminder behaviour moved to Subsystem traps."
requirements-completed: [LOCK-11, LOCK-12, LOCK-13, LOCK-14]
duration: 55m
completed: 2026-10-11
commits: 3
plan_head_before: 509dcaf42c84692e20f0957833321aeac657aa1a
plan_head_after: 6b5a24baa867e4abdfe768a1d85facdefd7ccfa1
actuals:
  tokens: 14500
  tasks: 2
  commits: 3
---

# Phase 5 Plan 14: Help text, AGENTS.md and the phase gate Summary

**Admins and staff can now read, in all 7 languages and in the generated docs, what a lockdown alert shows, what Lift, Ban recent joiners and Revoke link do and who may press each, and when reminders come and stop; AGENTS.md was checked against the shipped code and corrected; the PostgreSQL integrity run covers the exactly-once alert gate and the one-running-ban-run index; and the whole Phase 5 gate is green, including a real PostgreSQL 16 run.**

Note: `commits: 3` counts the two task commits and the gofmt commit made before this SUMMARY; the SUMMARY commit follows, so `plan_head_after` is the last code commit. `actuals.tokens` is chars/4 over the realized diff (58,211 chars).

## What was built

- **Help text (`lockdown_help_msg`, all 7 locales)**: a new block after the existing paragraphs. It says the alert goes to the Staff Group (if linked) and to the log channel (if the admin log category is on) and what it shows (trigger, joined just before and since, up to 10 accounts, the invite link most of them used); then one line per button. Lift lifts like `/unlockdown` and needs the owner or an admin who can restrict members, checked live, with Staff Group membership not enough. Ban recent joiners bans the burst's accounts for good so they can't rejoin, says it cannot be undone, leaves out admins, approved users and people an admin let in, and notes that a later press bans only new accounts. Revoke link revokes the link the raid used only if the bot created it, says other admins' links must be revoked in Telegram, and needs the owner or an admin who can invite users. A last line says every button asks for a confirmation only the presser can give, to press the buttons as yourself (not as an anonymous admin or a channel), and that a reminder comes every hour while the group stays locked and stops at the lift. Nothing is said about the banned accounts' messages: the ban call sends no `revoke_messages` (orchestrator note).
- **Help text (`staff_help_msg`, all 7 locales)**: one line after the `/staff` bullet saying a linked group's alert and hourly reminders appear in the Staff Group with the three buttons that only that group's admins can use.
- **Docs**: `make generate-docs` regenerated `commands/lockdown/index.md` and `commands/staff/index.md`; `make check-docs` reports no drift.
- **Makefile**: `TestBeginAlert` and `TestBanRunStart` added to the `-run` list of `test-postgres-integrity` (beside `TestStartLockdownOneActivePerChat`; `./alita/db/lockdown` was already in its package list).
- **AGENTS.md pass** (below).
- **`alita/modules/greetings_command_test.go`**: one stray blank line removed by `gofmt -w`, so the gate's `gofmt -l` is clean.

## AGENTS.md final pass

Every sentence naming a Phase 5 identifier, key, table, callback field or worker step was read against the code (`lockdown_alert_callbacks.go`, `lockdown_alert.go`, `lockdown_worker.go` cycle order, `lockdown_recent.go`, `lockdown_response.go`, the alert and burst models, the Makefile and the test harness lists). Fixed:

- **Contradiction**: the `lockdown|v1` bullet said the card expiry `e` "is written now and not yet checked". Plan 05-04 enforces it; the bullet now describes the lazy expiry (a Confirm with `e` before the current unix minute edits the card to `lockdown_alert_card_expired` and does nothing else, no timer, no Redis key).
- **Plan-number leftovers**: "Plan 05-07 saves its snapshot" in the tracker bullet now names `chat_lockdown_burst`.
- **Testing**: the line "the Phase 5 models are added to the harness lists ... today `alita/modules/test_harness_test.go`" was stale. It now says `ChatLockdownAlert`, `ChatLockdownBurst` and `ChatLockdownBanRun` are in `alita/modules/test_harness_test.go` and `alita/db/lockdown/testmain_test.go` only, that the `alita/db` package's own tests never touch them, and what `make test-postgres-integrity` covers.
- **Callbacks, added** (what 05-04 did not document, as the orchestrator asked): the single bullet is now five. The dispatcher's check order (service identity answered "post as yourself" before any read; copy id parse; copy row honoured only on its own chat, answered "no longer valid" with no lockdown read and no presser lookup; lockdown; stale press; action), `SetAllowChannel(true)` (moved here from Data), the strict field parsing (`lockdownParseCopyID`, `lockdownParseAsker`, `lockdownParseCardFields`), the confirm card's expiry minute `e`, `lockdown_alert_no_restrict` for an administrator without `can_restrict_members`, the invite-right authority for Revoke, and `lockdownRevokeRefusalKey` (Telegram's raw text goes to the log only).
- **Structure**: the Data bullet for `chat_lockdown_alerts` now holds the table facts only (row per copy, slot key, `message_id`, `body`, `link`, the claim and state columns, fresh reads, never in backup). The behaviour that sat in Data moved to Subsystem traps, once each: alert decision and destinations, copy delivery and stale-claim resume, the `edit_pending` refresh step, and reminders (with one added sentence on what a reminder shows).
- **Checked and unchanged**: ban decision table and claim-before-call rules, the Ban button's fresh-approvals rule, the stop at the lift, `#LOCKDOWN_BAN`, no staff audit, close at the lift, `liftLockdownRow` as the one lift path, the Redis key list, the migrations and `chat_lockdown_ban_runs` rules.

All acceptance greps hold: `TestBeginAlert` and `TestBanRunStart` once each in the Makefile, `lockdown|v1`, `chat_lockdown_alerts`, `chat_lockdown_burst`, `chat_lockdown_ban_runs`, `alita:lockdown:recent:`, `liftLockdownRow`, `#LOCKDOWN_BAN` and `lockdownReminderEvery` all present in AGENTS.md, `git diff --exit-code go.mod go.sum` exits 0, and no plan number remains in AGENTS.md.

## Task commits

| Task | Commit | Message |
| ---- | ------ | ------- |
| 1 | ea11d0b | docs(05-14): describe lockdown alerts, their buttons and reminders |
| 2 | 0161ed6 | docs(05-14): bring AGENTS.md in line with lockdown alerts and responses |
| gate | 6b5a24b | style(05-14): gofmt the greetings command test |

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] The plan's button lines as x-bullets would have failed an existing guard**
- **Found during:** Task 1
- **Issue:** the plan writes the Lift, Ban and Revoke lines as `×` bullets. `TestLockdownStringsHaveBalancedTags` requires exactly three `×` bullets in `lockdown_help_msg`, each starting with a `<code>/` command, in every locale.
- **Fix:** the three button lines are plain lines led by the button's emoji (🔓, 🚫, ⛔) and its label; the three command bullets are unchanged. A blank line separates each, so the generated docs show separate paragraphs.
- **Files modified:** the 7 locale files
- **Commit:** ea11d0b

**2. [Rule 3 - Blocking] The gate's `gofmt -l alita main.go` listed a file that predates Phase 5**
- **Issue:** `alita/modules/greetings_command_test.go` has one extra blank line (also noted by plans 05-01, 05-04, 05-07, 05-10..05-13). The gate requires an empty list.
- **Fix:** `gofmt -w` on that file, one line removed, committed on its own.
- **Commit:** 6b5a24b

### Other differences

- The plan's own sentence "Revoke link ... Only the owner or an admin who can invite users" became "Only the group owner or an admin who can invite users", and "I are left out" in the plan wording became "Admins, approved users and people an admin let in are left out" (the bot's own exclusion is true but not worth a sentence for admins).
- AGENTS.md also lists `SetAllowChannel(true)` under Callbacks now instead of Data.

## Verification

- `make check-docs`: no drift. `make check-translations`: all translations present.
- `go test -tags testtools -count=1 -run '^TestLockdownLocaleKeys$|^TestStaffLocaleKeys$|^TestLockdownStringsHaveBalancedTags$' -v ./alita/i18n`: pass; whole `./alita/i18n`: pass. `TestStaffPanel*` (including `TestStaffPanelRenderFitsInEveryLocale`, which proves the longer staff help still fits or falls back to the hint): pass.
- Acceptance: `grep -c 'Revoke link'` and `grep -c "can't rejoin"` on the generated lockdown page print 1 each.
- **`make test` (full suite, race, coverage, skip gate): exit 0**, 4 min 44 s, 61 `ok` package lines, no `--- FAIL`. The skips it lists are the PostgreSQL-only tests (`TestRepositoryMigrationChain`, `TestRunMigrations_*`, `TestApplyMigration_*`, `TestUpdateChat`, ...), which ran on PostgreSQL below. No intermittent failure appeared in this run.
- `CGO_ENABLED=0 go build ./...`: ok. `go vet -tags testtools ./...`: no diagnostics. `gofmt -l alita main.go`: empty after the gofmt commit. `git diff --exit-code go.mod go.sum`: unchanged.
- **PostgreSQL 16 (run locally this time).** `/usr/lib/postgresql/16/bin` was available, and `initdb` as the `postgres` user worked through `setpriv`, in a scratch cluster under `/var/tmp` with a local Redis. Against it:
  - `ALITA_TEST_MIGRATION_CHAIN=true go test -tags testtools -run '^TestRepositoryMigrationChain$' ./alita/db/migrations`: pass; all 50 migrations apply, including the four Phase 5 files (`20261011120000`, `130000`, `140000`, `150000`), and index verification reports 0 missing.
  - `ALITA_TEST_DATABASE=true make test-postgres-integrity`: exit 0, every listed test passes, including `TestBeginAlert` (4 subtests), `TestBanRunStart` (6 subtests) and `TestStartLockdownOneActivePerChat`.
  - Bonus: the whole `./alita/db/lockdown` package under `-race -p 1` against PostgreSQL: pass.
  The cluster and Redis were stopped afterwards.
- `golangci-lint` was not run: the installed build targets Go 1.25 and cannot lint Go 1.26 code; `go vet -tags testtools` and `gofmt -l` stand in for it.

## Threat surface

No surface beyond the plan's threat model. T-05-40 (a help text that misstates an irreversible action) is covered: the Ban line says for good, can't rejoin, can't be undone, names who is left out and who may press, and claims nothing about messages; `TestLockdownLocaleKeys` and `TestStaffLocaleKeys` keep all 7 languages in parity. No package installs.

## Known Stubs

None.

## UAT still open (needs real Telegram groups; not runnable here)

1. Lock a linked group that has a log channel; check both alert copies render and press Lift from the Staff Group and from the log channel (research A1, A2).
2. During a lockdown, join through a bot-created link and through another admin's link, press Revoke and record Telegram's exact answers (A4, A7). `lockdownRevokeRefusalKey` maps an unrecognised answer to the generic line, so add the real wording there if it differs.
3. Press Ban recent joiners; confirm the accounts are banned with no end date, stay banned after the lift, and that someone an admin added during the lockdown is left out.
4. Keep a test group locked for over an hour; check the reminder arrives, the previous one loses its buttons, and nothing comes after the lift.
5. The PostgreSQL items in the plan's human check (migrations on 16, `make test-postgres-integrity`) were done here; repeat them on the production database before deploy if desired.

## Self-Check: PASSED

Commits ea11d0b, 0161ed6 and 6b5a24b are on the branch. Files modified exist: the 7 locale files, the two generated docs pages, `Makefile`, `AGENTS.md`, `alita/modules/greetings_command_test.go`. `STATE.md` and `ROADMAP.md` were not touched.
