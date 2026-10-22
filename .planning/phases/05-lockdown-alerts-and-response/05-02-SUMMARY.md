---
phase: 05-lockdown-alerts-and-response
plan: 02
subsystem: lockdown
tags: [lockdown, alerts, staff-group, callbacks, lift, i18n]
status: complete
requires:
  - phase: 05-lockdown-alerts-and-response
    provides: chat_lockdown_alerts copy rows, lockdownPostAlerts, lockdownRenderAlert (plan 05-01)
provides:
  - liftLockdownRow, the one lift path shared by /unlockdown and the alert's Lift Confirm
  - lockdown|v1 callback namespace with stateless confirm cards
  - "Lift lockdown" button on every posted alert copy (LOCK-11, LOCK-06)
affects: [05-03, 05-04, 05-06, 05-13]
tech-stack:
  added: []
  patterns:
    - "button authority is a live getChatMember of the chat stored on the lockdown row, never the pressed chat or the admin cache"
    - "stateless confirm card: asker u and expiry e travel in callback data; every fact is re-checked live at Confirm"
key-files:
  created:
    - alita/modules/lockdown_alert_callbacks.go
    - alita/modules/lockdown_alert_lift_test.go
  modified:
    - alita/modules/lockdown.go
    - alita/modules/lockdown_alert.go
    - AGENTS.md
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml
key-decisions:
  - "Confirm cards are stateless (u and e in the callback data), so the lift button works with Redis down; e is written here and enforced by plan 05-04."
  - "A press is judged against ld.ChatID from the lockdown row, loaded fresh through the alert copy row named by r; the chat the button sits in grants nothing."
  - "The public lift notice goes through lockdownPaced into the locked group; the card, its edits and the toasts are sent directly like the staff cards."
requirements-completed: [LOCK-11, LOCK-06]
duration: 45m
completed: 2026-10-11
commits: 4
plan_head_before: 571cc3b12dc0ab56632fe055ff26e11234ee683a
plan_head_after: 7ac5c98ff22f04d34f8277f85dd725be7b1e0055
actuals:
  tokens: 14500
  tasks: 2
  commits: 4
---

# Phase 5 Plan 02: Lift a lockdown from its alert Summary

**The Staff Group alert now carries a "Lift lockdown" button: a live admin (with the restrict right) of the locked group gets a confirm card only they can use, and Confirm runs the same restore-then-record lift as `/unlockdown` through one shared `liftLockdownRow`, then posts the public "Lockdown lifted by X" notice in the locked group.**

Note: `commits: 4` counts the two RED test commits and the two implementation commits made before this SUMMARY; the SUMMARY commit follows it.

## What was built

- **`liftLockdownRow` (`alita/modules/lockdown.go`)**: the body of `/unlockdown` from the manual-change read to `FinishLift`, moved unchanged, returning a `lockdownLiftResult` (outcome done / restore failed / record failed / lost, plus manual change, finished and unbanning count). It is the only caller of the `lockdownBeginLift` seam. `lockdownLiftedText` builds the public notice. `/unlockdown` keeps its reads and its lifting-state reply and maps the outcome to the same replies as before.
- **Alert keyboard (`lockdown_alert.go`)**: `lockdownAlertKeyboard` puts one `a=lk&r=<copy row id>` button on every copy; an encoding overflow is logged and the button is left out. Constants for the namespace and actions, and `lockdownCardLifetime` (5 min).
- **Callbacks (`lockdown_alert_callbacks.go`)**: `lockdownAlertCallback` decodes `lockdown|v1`, loads the copy row and its lockdown fresh, and dispatches `lk` / `lc` / `x`. `lockdownButtonAuthority` is a live `getChatMember` of `ld.ChatID` (creator, or administrator with `can_restrict_members`); a failed lookup refuses. Ask posts a card replying to the alert (Confirm carries asker `u` and expiry `e`, Cancel carries `u`). Confirm requires `u == presser`, re-checks the lockdown and the presser live, runs `liftLockdownRow`, edits the card to its final text and posts the notice through `lockdownPaced`. Every outcome is told on the card: lifted, restore failed, record failed, already lifted (fresh lifter and time); Cancel belongs to the asker.
- **Registration**: `callbackquery.Prefix("lockdown|")` in `LoadLockdown`.
- **Locale keys** (14, all 7 locales): `lockdown_alert_btn_lift`, `_lift_ask`, `_btn_confirm`, `_btn_cancel`, `_not_admin`, `_check_failed`, `_not_yours`, `_working`, `_lift_done`, `_already_lifted`, `_expired`, `_restore_failed`, `_record_failed`, `_cancelled`.
- **AGENTS.md**: a Callbacks bullet for `lockdown|v1`, and the `BeginLift` rule now names `liftLockdownRow`.

## Tests

- `TestLockdownAlertLift`: the alert has exactly one button (a=lk, r = copy row ID); a plain member of the locked group is refused with one answer and no new message; the admin gets a card replying to the alert with Confirm (u = admin) and Cancel; another live admin's Confirm is answered "not yours" with no `setChatPermissions`; the asker's Confirm sends the stored permissions byte for byte while the row is still active, records the presser as lifter (raw name `Ad<b>min`), posts the lift notice in the locked group and edits the card to the done text.
- `TestLockdownAlertLiftOutcomes`: restore fails (card says so, lockdown active, no notice, group stays locked); record fails (card says so, lockdown active); someone lifted first via `/unlockdown` (card says already lifted, exactly one restore in total); lost race during the press (another admin records between restore and record: card says already lifted with the winner recorded, no second notice); Cancel by another admin (answered "not yours", no edit) and by the asker (card cancelled, nothing changed).
- Unchanged and still passing: `TestLockdownLiftManualChange`, `TestUnlockdownLiftsOnce`, `TestLockdownLiftRecordFailure`, `TestLockdownLiftRestoreFailure`, `TestUnlockdownWithoutLockdown`.

## Task commits

| Task | Commit | Message |
| ---- | ------ | ------- |
| 1 (RED) | 82ccb3d | test(05-02): show a lockdown alert has no way to lift it |
| 1 (GREEN) | b8c71ce | feat(05-02): lift a lockdown from its alert |
| 2 (RED) | 03784b6 | test(05-02): show lift failures from the alert are not reported |
| 2 (GREEN) | 7ac5c98 | fix(05-02): report every lift outcome on the alert's confirm card |

## Deviations from Plan

**1. [Task 1 - intermediate placeholder] Non-done outcomes and Cancel had a stand-in text in the Task 1 commit.** To keep Task 2 a real RED-then-GREEN step, the Task 1 commit edits the card to `lockdown_alert_check_failed` for restore failed, record failed, lost and Cancel. Task 2 replaced every one of them with its specific text. The plan is complete only with both commits.

**2. [Task 2 - one subtest passed on its first run] "someone lifted first" (the lockdown already lifted when Confirm is pressed) passed before Task 2's fix**, because Task 1's Confirm already edits the card to `lockdown_alert_already_lifted` for a non-active lockdown. The other four subtests failed as expected. An extra subtest, "lost the race", covers the `lockdownLiftLost` outcome (the lift recorded by another admin between this lift's restore and record), which the plan's list did not name separately.

Otherwise the plan was executed as written.

## Known limitations

- The expiry `e` is written into Confirm but not checked yet; plan 05-04 enforces it. AGENTS.md says so.
- The button is judged by the lockdown row's chat, not by the pressed message's chat. A forged `lk` for a known copy ID pressed from any chat the bot can post in would at most post a confirm card there for a presser who is a live admin of the locked group; nothing changes without that live check.

## Verification

- `go test -tags testtools -race -count=1 -run '^TestLockdownAlertLift$'` and `-run '^TestLockdownAlertLiftOutcomes$|...'` (with the four unchanged lift tests): pass.
- `go test -tags testtools -race -count=1 -run 'Lockdown|Unlockdown' ./alita/modules`: pass. The whole `./alita/modules` package under `-race`: pass (180 s).
- `TestLockdownLocaleKeys`, `make check-translations`, `make check-docs` (no drift): pass.
- `CGO_ENABLED=0 go build ./...` and `go vet -tags testtools ./alita/modules`: clean. `gofmt -l` on the four touched Go files prints nothing. golangci-lint was not run: the installed one is built with Go 1.25 and cannot lint Go 1.26 code, so `go vet` and `gofmt` were used instead.
- `go.mod` and `go.sum` unchanged. `grep 'lockdownBeginLift('` outside tests: one line, in `lockdown.go`.
- Not run: any PostgreSQL check (no server in the sandbox; this plan adds no migration). Not run: a manual UAT press from a real Staff Group (assumption A1: a callback from a button on a bot post carries the real presser in `from`).

## Threat surface

No surface beyond the plan's threat model. T-05-04 (live `getChatMember` of the locked chat at Ask and Confirm; failed lookup refuses), T-05-05 (only IDs in data, rows loaded fresh, Confirm/Cancel need `u == presser`), T-05-06 (one shared `liftLockdownRow`, restore asserted before record through the `setChatPermissions` hook) and T-05-07 (presser recorded as `lifted_by` / `lifted_by_name`, named in the notice) are each covered by a test above.

## Known Stubs

None.

## Self-Check: PASSED

All created files exist (`lockdown_alert_callbacks.go`, `lockdown_alert_lift_test.go`), and commits 82ccb3d, b8c71ce, 03784b6 and 7ac5c98 are on the branch.
