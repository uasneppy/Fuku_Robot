---
phase: 05-lockdown-alerts-and-response
plan: 03
subsystem: lockdown
tags: [lockdown, alerts, staff-group, callbacks, lift, i18n]
status: complete
requires:
  - phase: 05-lockdown-alerts-and-response
    provides: alert copy rows and lockdownPostAlerts (05-01), liftLockdownRow and the lockdown| callbacks (05-02)
provides:
  - worker step that edits every posted alert copy to "Lifted by X at T" with no buttons at any lift
  - close claims in the lockdown repository (list, claim, record, release)
  - stale-button path: a press on a lifting or lifted lockdown says so and changes nothing
  - repository tests for the alert gate, copy claims and close claims
affects: [05-04, 05-06, 05-13]
tech-stack:
  added: []
  patterns:
    - "closing is a worker step driven by the lockdown's state, so it covers /unlockdown, the alert button and a restart alike"
    - "a posted copy's claimed_at doubles as the close claim; a conditional update decides the winner"
key-files:
  created:
    - alita/db/lockdown/alerts_test.go
    - alita/modules/lockdown_alert_close_test.go
  modified:
    - alita/db/lockdown/alerts.go
    - alita/db/lockdown/testmain_test.go
    - alita/modules/lockdown_alert.go
    - alita/modules/lockdown_alert_callbacks.go
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
  - "Closing runs after lockdownProcessLifts in every cycle and lists copies as soon as the lockdown is not active (lifting or lifted), newest first, at most lockdownAlertBatch per cycle."
  - "The close claim reuses claimed_at on a posted copy; RecordAlertClosed stamps closed_at and clears buttons_live and edit_pending."
  - "'message to edit not found', 'message can't be edited' and MESSAGE_ID_INVALID close the copy (detail gone); 'message is not modified' is success."
  - "The stale-button check sits in lockdownAlertCallback right after the copy row and its lockdown are loaded, ahead of the action switch, so no action and no lookup of the chat's current lockdown can follow."
requirements-completed: [LOCK-06, LOCK-11]
duration: 40m
completed: 2026-10-11
commits: 4
plan_head_before: f5803d4dfec670643e4c15abaa36ee940e1b191d
plan_head_after: fa21a8231fe9a4acfacaa1834fd83037c04656c0
actuals:
  tokens: 17000
  tasks: 2
  commits: 4
---

# Phase 5 Plan 03: Close every alert copy at the lift Summary

**At any lift, `/unlockdown` or the alert's button, the worker edits each posted alert copy to its stored text plus "Lifted by X at T" and strips its buttons exactly once; a press on a button of a lifting or lifted lockdown answers "Already lifted by X at T", closes that copy and never touches a newer lockdown.**

Note: `commits: 4` counts the two RED test commits and the two implementation commits made before this SUMMARY; the SUMMARY commit follows it.

## What was built

- **Close claims (`alita/db/lockdown/alerts.go`)**: `ListAlertSlotsToCloseFresh` (posted, unclosed, unclaimed copies of lockdowns that are not active, newest ID first), `ClaimAlertClose` (conditional update of `claimed_at`), `RecordAlertClosed` (`closed_at`, claim cleared, `buttons_live` and `edit_pending` false, detail), `ReleaseAlertClose` (claim cleared, an attempt counted only when asked). Rows of an active lockdown are never listed. No cache is involved, as for the rest of the alert tables.
- **Closing (`alita/modules/lockdown_alert.go`)**: `lockdownLiftedFooter` (lifter mention and `lift_started_at` spliced in after translation), `lockdownEditGone`, `lockdownCloseAlerts` (the worker step), `lockdownCloseSlotNow` (one copy at once) and a shared `lockdownCloseClaimed`. One paced `EditMessageTextWithContext` of the stored body plus the footer, HTML, link previews off, no reply markup. Success, "message is not modified" and a gone message close the copy; a rate limit or shutdown releases the claim without an attempt and ends the step for that cycle; any other failure costs an attempt and the third closes the copy with Telegram's reason.
- **Worker hook (`lockdown_worker.go`)**: `runLockdownCycle` runs `closed := lockdownCloseAlerts(ctx, b)` after `lockdownProcessLifts` and ORs it into the progress result.
- **Stale buttons (`lockdown_alert_callbacks.go`)**: `lockdownStalePress`, called before the action switch when the copy's lockdown is not active. It answers `lockdown_alert_already_lifted` (plain text), then closes the copy through `lockdownCloseSlotNow` when the pressed message is the copy itself (chat and message ID match the row), or edits a confirm card to the same text. It posts no card, lifts nothing and never reads the chat's current lockdown.
- **Locale key** `lockdown_alert_lifted_footer` in all 7 locale files ("🔓 Lifted by {name} at {time}." and translations).
- **AGENTS.md**: a Subsystem traps bullet for the close step and an addition to the `lockdown|v1` bullet for the stale-button rule.

## Tests

- `TestLockdownAlertClosedAtLift` (command, button, gone, not modified, rate limit, other failure given up after three attempts): the alert is edited exactly once to its body plus the footer naming the lifter (escaped), with no keyboard; the row ends closed with no live buttons and no claim; further cycles make no edit; a gone message is not retried; a rate limit leaves the row open with 0 attempts and the next cycle closes it.
- `TestLockdownAlertStaleButtons`: the Lift button of a lifted lockdown answers already lifted, posts no card, makes no `setChatPermissions` and closes the copy (a later cycle makes no second edit); a Confirm on a card of a lockdown lifted meanwhile edits the card to already lifted with no write to the group and no second lift notice; an old alert's Lift after `/lockdown` again posts no card, leaves lockdown 2 active, does not edit alert B or close its copy and makes no `setChatPermissions`.
- `TestBeginAlert`, `TestAlertSlots`, `TestAlertClose` (repository, SQLite): gate once / empty plan / unconfirmed / lifting; claim, post, release with and without an attempt, final fail, pending list limited to active lockdowns and oldest first, unique slot; close list newest first and only for non-active lockdowns, claim won once, record clears everything, release counting.

## Task commits

| Task | Commit | Message |
| ---- | ------ | ------- |
| 1 (RED) | 07a3827 | test(05-03): show a lifted lockdown's alert keeps its live buttons |
| 1 (GREEN) | 108fbf0 | feat(05-03): mark every alert copy lifted and remove its buttons |
| 2 (RED) | 5c79cd0 | test(05-03): show a lifted lockdown's button still acts |
| 2 (GREEN) | fa21a82 | fix(05-03): make a lifted lockdown's buttons do nothing but say so |

## Deviations from Plan

**1. [Task 1 - RED shape] The repository tests failed to compile instead of failing at runtime.** `ListAlertSlotsToCloseFresh` and the other close functions did not exist at the RED commit, so `./alita/db/lockdown` did not build. The module test `TestLockdownAlertClosedAtLift` failed at runtime in all six subtests. `TestBeginAlert` and `TestAlertSlots` exercise code from 05-01/05-02, so they passed as soon as the package built; only `TestAlertClose` depends on the new functions.

**2. [Task 2 - two subtests passed at RED] "confirm card of a lockdown lifted meanwhile" and "an old alert never touches a newer lockdown" already passed before the fix**, because 05-02's Lift Ask and Confirm already refuse a non-active lockdown and the copy row already names its own lockdown. Only "lift button of a lifted lockdown" failed (the alert was never edited). The new stale path moves the check ahead of every action and adds the close.

**3. [Extra subtest] "other failure is given up after three attempts"** was added to `TestLockdownAlertClosedAtLift` to cover the plan's rule that a copy that keeps failing is given up after the third attempt. The closing attempt is recorded with Telegram's reason but, as the plan specifies for `RecordAlertClosed`, does not add to the attempt counter (2 counted attempts, third closes).

Otherwise the plan was executed as written.

## Known limitations

- A press that cannot take the close claim (the worker owns it) edits nothing itself; the worker closes the copy in the same or the next cycle.
- A closing edit that exceeds Telegram's 4096-character limit (a very long reason in the stored body) fails like any other failure and the copy is given up after three attempts.
- `lockdown_alert_close_test.go` and `alerts_test.go` run on SQLite only; no PostgreSQL server is available in the sandbox. No migration was added.

## Verification

- `go test -tags testtools -race -count=1 -run 'Lockdown|Unlockdown' ./alita/modules ./alita/db/lockdown`: pass (modules 36 s, repository 1 s).
- The named tests pass with `-v`: `TestLockdownAlertClosedAtLift`, `TestLockdownAlertStaleButtons`, `TestLockdownAlertLift`, `TestLockdownAlertLiftOutcomes`, `TestBeginAlert`, `TestAlertSlots`, `TestAlertClose`.
- `TestLockdownLocaleKeys`, `make check-translations` and `make check-docs` (no drift): pass.
- `CGO_ENABLED=0 go build ./...` and `go vet -tags testtools ./alita/modules ./alita/db/lockdown`: clean. `gofmt -l` on all touched Go files prints nothing. `git diff --exit-code go.mod go.sum`: unchanged. golangci-lint was not run: the installed one is built with Go 1.25 and cannot lint Go 1.26 code, so `go vet` and `gofmt` were used instead.
- Not run: the whole `./alita/modules` package under `-race` (only the `Lockdown|Unlockdown` selection), any PostgreSQL check, and a manual UAT press from a real Staff Group.

## Threat surface

No surface beyond the plan's threat model. T-05-08 (a stale button acting on a newer lockdown) is covered by `TestLockdownAlertStaleButtons`; T-05-09 (many copies edited at one lift) by the per-cycle `lockdownAlertBatch` bound, the pacer and the stop on a rate limit.

## Known Stubs

None.

## Self-Check: PASSED

`alita/db/lockdown/alerts_test.go` and `alita/modules/lockdown_alert_close_test.go` exist, and commits 07a3827, 108fbf0, 5c79cd0 and fa21a82 are on the branch.
