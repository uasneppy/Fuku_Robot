---
phase: 05-lockdown-alerts-and-response
plan: 13
subsystem: lockdown
tags: [lockdown, alerts, reminders, worker, i18n]
status: complete
requires:
  - phase: 05-lockdown-alerts-and-response
    provides: alert copies, claims, retries and stale release (05-01, 05-06), the close step (05-10), the sync step and Revoke (05-12), the Ban and Lift buttons (05-03, 05-11)
provides:
  - lockdown.ListRemindableFresh, AlertDestKinds, ReminderPlanned, PlanReminderSlots, SupersedeReminders
  - lockdownReminderEvery, lockdownReminderSeq, lockdownPlanReminders, lockdownRenderReminder, superseded handling in lockdownPostAlertSlot
  - RecordAlertSynced(id, buttonsLive), a conditional clear of edit_pending
affects: [05-14]
tech-stack:
  added: []
  patterns:
    - "a reminder is an ordinary alert copy row (kind reminder, seq from alert_started_at), so the poster, claims, retries, stale release and close step handle it with no new state"
    - "a refresh clears its edit mark only while buttons_live still has the value the edit was made with"
key-files:
  created:
    - alita/modules/lockdown_reminder_test.go
  modified:
    - alita/db/lockdown/alerts.go
    - alita/db/lockdown/alerts_test.go
    - alita/modules/lockdown_alert.go
    - alita/modules/lockdown_worker.go
    - AGENTS.md
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml
key-decisions:
  - "Reminder seq = floor((now - alert_started_at) / lockdownReminderEvery); only the current seq is ever planned, so downtime or an alert older than this feature never produces a backlog."
  - "A pending reminder behind the current seq is ended as failed with detail superseded instead of posted late."
  - "Destinations are the alert's destination kinds (AlertDestKinds) resolved again by lockdownAlertDestinations at planning time."
  - "A reminder posted after a revoke is sent with the revoked footer, but its stored body is without it, because the sync and close steps append the footers to the stored body."
  - "RecordAlertSynced takes the buttons_live value the edit used and clears edit_pending only while it still matches; the sync step re-reads a copy after claiming it."
patterns-established:
  - "Reuse the alert's own machinery for every new kind of copy: add rows, not state."
requirements-completed: [LOCK-14]
duration: 75min
completed: 2026-10-11
commits: 4
plan_head_before: a74396717f49eea10cf5c8f721f4be71a921ffe3
plan_head_after: 0610a60f49e9083519f37e303295bbff5b95828f
actuals:
  tokens: 14400
  tasks: 2
  commits: 4
---

# Phase 5 Plan 13: Hourly reminders while a group stays locked Summary

**While a group stays locked, the worker posts one reminder an hour (counted from the alert) into each destination the alert went to: "still locked since T", the reason, joiners removed so far (response bans included), the number still pending and the alert's link line, with working Lift, Ban (fresh count) and Revoke buttons and no sample; the previous reminder loses its buttons, the original alert keeps them until the lift, nothing is back-filled after downtime, and reminders stop at the lift, which closes every copy.**

Note: `commits: 4` counts the two RED test commits and the two implementation commits made before this SUMMARY; the SUMMARY commit follows, so `plan_head_after` is the last code commit. `actuals.tokens` is chars/4 over the realized diff (57,651 chars).

## What was built

- **Repository (`alerts.go`)**:
  - `ListRemindableFresh(limit)`: active lockdowns with `locked_at` and `alert_started_at` set.
  - `AlertDestKinds(lockdownID)`: the distinct, sorted destinations of the lockdown's alert copies (kind alert, any state).
  - `ReminderPlanned(lockdownID, seq)`: whether any reminder copy of that seq exists.
  - `PlanReminderSlots(lockdownID, seq, slots)`: one transaction; inserts nothing unless the lockdown is still active; each slot is forced to kind reminder, the given seq, pending, and inserted `ON CONFLICT DO NOTHING` on the slot key; returns how many rows it inserted.
  - `SupersedeReminders(lockdownID, dest, seq)`: posted, open reminder copies of that destination with a lower seq and live buttons get `buttons_live` false and `edit_pending` true. Alert copies, the same seq and other destinations are untouched.
  - `RecordAlertSynced(id, buttonsLive)`: see "Rule chosen" below.
- **Modules (`lockdown_alert.go`)**:
  - `lockdownReminderEvery` (a variable, `time.Hour`), `lockdownReminderSeq`.
  - `lockdownPlanReminders`: for each remindable lockdown, skips seq below 1 or already planned, reads the alert's kinds, resolves `lockdownAlertDestinations`, keeps the kinds the alert went to, and plans.
  - `lockdownPostAlertSlot` branches on kind: a reminder behind the current seq is failed with detail `superseded` with no send; otherwise it is rendered with `lockdownRenderReminder`, sent with the same keyboard and pacing as the alert, and after `RecordAlertPosted` calls `SupersedeReminders`.
  - `lockdownRenderReminder`: title (group spliced through its token, since time), reason, counts (`lockdownRemovedSoFar`, the same as `/lockdownstatus`, and pending plus acting), the alert's link line from `TopInviteLinksFresh` (its top link stored on the copy so Revoke works), no sample. A failed read leaves that line out (warn).
  - `lockdownSyncAlertCopies` re-reads a copy after claiming it.
- **Worker**: `runLockdownCycle` runs `reminded := lockdownPlanReminders(ctx, b)` right before `lockdownPostAlerts` and ORs it into the progress result.
- **Locale keys in all 7 files**: `lockdown_reminder_title`, `lockdown_reminder_counts`.
- **AGENTS.md**: a Reminders bullet (planning, seq, superseded, destinations, the stale-claim rule, supersede) and the `RecordAlertSynced` rule, placed next to the sync bullet in Data where the other alert-copy rules live (as plan 05-12 did), not in Subsystem traps.

## Rules chosen, as the orchestrator asked

1. **Stale `posting` claim (05-01's open rule).** `ReleaseStaleAlertClaims` (05-06) does cover `posting`: it moves a copy claimed for over 2 minutes back to `pending` without counting an attempt, and the lockdown cycle calls it first. Reminder copies reuse that path unchanged. The rule is the alert's: **duplicate over lost**, so a crash between the Telegram send and `RecordAlertPosted` can post that reminder twice. The one reminder-specific twist: if by the time the released copy is retried a newer reminder is due, the released copy is ended as `superseded` and never sent, so a long outage turns into one current reminder, not a stale one plus a duplicate. This is written in AGENTS.md.
2. **Sync reuse and a second `edit_pending` mark (05-12's note).** The case was reachable here: a revoke marks a live reminder, a sync claims it and builds its keyboard, the hourly `SupersedeReminders` flips `buttons_live` to false and sets the mark again, then the in-flight sync finishes and cleared the mark, leaving the previous reminder with Lift and Ban for ever. Fixed two ways: `RecordAlertSynced(id, buttonsLive)` clears `edit_pending` with `CASE WHEN buttons_live = <value the edit used> THEN false ELSE edit_pending END`, so a mark set by a supersede during the edit survives and the next cycle edits the copy again; and the sync step reads the copy again after claiming it, so it normally edits with the current `buttons_live` anyway. The other direction cannot lose an edit that matters: `RecordLinkRevoked` marks only copies with live buttons, and a copy whose buttons are already off is not refreshed on a revoke (its revoked footer appears at the lift, by the close step), so a revoke arriving during a supersede-triggered sync has no mark to lose. Pinned by `TestAlertSyncKeepsAMarkSetDuringTheEdit`.

## Tests

- `TestLockdownReminders`: first (61 min: one reminder per destination, seq 1, Lift and Ban, log header on the log copy; further cycles post nothing), second (121 min: seq 2; the seq 1 copies are edited once without a keyboard; the original alerts and seq 2 are not edited and keep `buttons_live`), no catch-up (5 h 30 min: only seq 5), concurrent (two cycles at once: one reminder per destination), stops at the lift (no reminder after `/unlockdown`; every alert and reminder copy closed with the lifted footer and no keyboard), stale pending (seq 1 pending when seq 2 is due: seq 1 failed `superseded`, only seq 2 sent).
- `TestLockdownReminderContent`: counts (3 banned, 1 pending, 2 kept response bans give removed 5, pending 1, the escaped reason, no sample), buttons (Lift, Ban with a fresh count of 3 where the alert said 2, Revoke from the stored link; Revoke from the reminder works; the refreshed reminder drops Revoke and gains the revoked footer; the next reminder has no Revoke and the footer exactly once, with no edit), lift from a reminder (plain member refused with `lockdown_alert_not_admin`; the admin's card replies to the reminder; the lift closes every copy), ban from a reminder (a run starts and finishes with the reminder's card).
- `TestLockdownReminderDestinations`: unlinked (reminder to the log channel only), linked later (log channel only), category off later (Staff Group only).
- `TestReminderSlots` (once per seq and destination, none for a non-active lockdown, supersede scope, listing and destination kinds) and `TestAlertSyncKeepsAMarkSetDuringTheEdit`.
- Mutation check: disabling the `SupersedeReminders` call makes `TestLockdownReminders/second` fail ("first reminder was edited 0 times, want once"). The existing `TestAlertSync` calls were updated to the new `RecordAlertSynced` signature.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] A sync in flight could clear the mark a supersede set meanwhile**
- **Found during:** Task 1 (the orchestrator's 05-12 note, applied to the reminder design)
- **Issue:** `RecordAlertSynced` cleared `edit_pending` unconditionally, so the previous reminder could keep its buttons.
- **Fix:** `RecordAlertSynced(id, buttonsLive)` conditional clear plus a re-read after the claim; existing tests updated, one test added.
- **Files modified:** `alita/db/lockdown/alerts.go`, `alita/modules/lockdown_alert.go`, `alita/db/lockdown/alerts_test.go`
- **Commit:** 0d32147

**2. [Rule 1 - Bug] A reminder posted after a revoke would have shown its footer twice**
- **Found during:** Task 1 design
- **Issue:** the plan stores the posted text as the body, and every later sync or close appends the footers to the stored body, so a body that already held the revoked footer would show it twice.
- **Fix:** a reminder is sent as body plus footers but stored as the body alone; a later refresh of that copy reproduces the sent text (verified by the "second reminder: footer exactly once, no edit" assertion).
- **Commit:** 0d32147

### Other differences

- `lockdownRenderReminder` returns `(text, link)` rather than a string, because the copy must store the link its Revoke button revokes. The footers are added by the post, not by the renderer.
- `PlanReminderSlots` also refuses to insert for a lockdown that is no longer active (one extra count in its transaction), so a reminder planned in a race with a lift leaves no pending row.
- The RED commits: the Task 1 one does not compile (new symbols), as in earlier plans. In the Task 2 RED commit `counts` and `buttons` failed; `lift from a reminder`, `ban from a reminder` and the three destination cases already passed against Task 1's code and are kept as pinned tests.

## Verification

- `go test -tags testtools -race -count=1 -run '^TestLockdownReminders$' -v ./alita/modules`: pass.
- `go test -tags testtools -race -count=1 -run '^TestLockdownReminderContent$|^TestLockdownReminderDestinations$|^TestLockdownReminders$' -v ./alita/modules`: pass.
- `go test -tags testtools -race -count=1 -run '^TestReminderSlots$|^TestAlertSync|^TestAlertClose$' -v ./alita/db/lockdown`: pass.
- `go test -tags testtools -race -count=1 -run 'Lockdown|Unlockdown' ./alita/modules ./alita/db/lockdown`: pass (modules 68 s), including `TestLockdownNeverLiftsOnItsOwn` and `TestLockdownStopDuringCall`. Run twice (after each task), no failure.
- `go test -tags testtools -count=1 -run '^TestLockdownLocaleKeys$' ./alita/i18n`, `make check-translations`, `make check-docs`, `CGO_ENABLED=0 go build ./...`, `go vet -tags testtools ./alita/modules ./alita/db/lockdown`: clean. `gofmt -l` on the touched files prints nothing (`greetings_command_test.go` is listed by `gofmt -l alita/modules` but is not touched by this plan). `git diff --exit-code go.mod go.sum` exits 0.
- Acceptance greps pass: `lockdownReminderEvery = time.Hour`, `reminded := lockdownPlanReminders(ctx, b)` before the poster call, `lockdown_reminder_title` and `lockdown_reminder_counts` once in each of the 7 locale files, `lockdownReminderEvery` in AGENTS.md, `func lockdownRenderReminder(`, `lockdownRemovedSoFar(`.
- `golangci-lint` was not run: the installed build targets Go 1.25 and cannot lint Go 1.26 code (go vet and gofmt used instead).
- The full `./alita/modules` package under -race was not re-run end to end; the Lockdown and Unlockdown subset (all lockdown code and tests) was.
- Not run: PostgreSQL (SQLite only here) and a real Telegram edit that drops a keyboard.

## Observations

- On SQLite, the "concurrent" subtest logs a `database is locked` warning when both goroutines write at once; the losing cycle's plan is retried by the next cycle and the subtest passes. This is SQLite's single-writer behaviour, not a PostgreSQL concern.

## Threat surface

No surface beyond the plan's threat model. T-05-37 is covered by `TestLockdownReminders` ("concurrent", "no catch-up", "stale pending"); T-05-38 by `TestLockdownReminderDestinations` and by every button on a reminder going through the same authority checks as the alert (`lift from a reminder` refuses a plain member); T-05-39 by the existing `lockdownAlertBatch` bound on the sync and close steps (a reminder adds at most one copy per destination per hour per lockdown). No package installs.

## Known Stubs

None.

## Known limits

- If `SupersedeReminders` fails after a reminder was posted (a database error between two statements), the previous reminder keeps its buttons; the failure is logged at warn and nothing retries it.
- A reminder copy that stays in `posting` for more than an hour (not possible with a 2 minute stale release) could be posted after a newer one and keep its buttons.
- A lockdown whose alert went to no destination, or whose destinations no longer apply, is looked at every worker tick (2 s) for its due seq: a few indexed reads, no writes.
- The previous reminder is not edited to show a later revoke (it has no buttons); the revoked footer is added to it at the lift by the close step.

## Self-Check: PASSED

`alita/modules/lockdown_reminder_test.go` exists; commits c529ae3, 0d32147, 2cfbdac and 0610a60 are on the branch.
