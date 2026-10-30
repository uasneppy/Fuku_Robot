---
phase: 09-settings-menu
plan: 04
subsystem: lockdown
tags: [lockdown, autolockdown, refactor, gotgbot, settings-menu]

requires:
  - phase: 08-captcha-web
    provides: the lockdown commands, the alert Lift Confirm and the lockdown pacer this plan factors
provides:
  - applyAutoLockdown, the one write path of the automatic lockdown triggers (switch and threshold)
  - applyLockdownLift and lockdownLiftApplyText, the shared lift and its replies
  - lockdownAnnounceLift, the shared public lift notice plus worker wake
  - lockdownStatusHead, the pure head of /lockdownstatus
  - lockdownCurrentLookup seam for the current-lockdown read
affects: [09-10 settings Lockdown page, 09-12 settings Lockdown actions]

actuals:
  tokens: 9000
  tasks: 2
  commits: 4
plan_head_before: 5df191f8683da0eca1c46c354afc11ad23532f77
plan_head_after: 4020e4e08d3186b8f6c560440c5b8ac92ef031de

tech-stack:
  added: []
  patterns:
    - "Command body split into a pure apply function returning an outcome struct plus a text function that renders each outcome, so a button page can reuse the code path and the exact reply"

key-files:
  created:
    - alita/modules/lockdown_apply_test.go
  modified:
    - alita/modules/autolockdown.go
    - alita/modules/lockdown.go
    - alita/modules/lockdown_alert_callbacks.go
    - alita/modules/lockdown_status.go

key-decisions:
  - "applyAutoLockdown still refuses a basic group itself, but autolockdownApply keeps its own early basic-group check so the parse errors and the status view of a basic group reply exactly as before"
  - "lockdownStatusHead puts the cut mark inside the reason it splices (reason cut, then plus an ellipsis, then escaped by lockdownReasonLine), so the mark stays next to the reason whatever the locale's word order; a reason within the cap gets no mark"
  - "lockdownAnnounceLift has only the alert's Lift Confirm as a caller today and the settings menu (09-12) becomes the second, so AGENTS.md (which says the button path posts the public notice) stays true and is not edited here"

patterns-established:
  - "Seam variables (lockdownCurrentLookup) reassigned only by tests, like lockdownBeginLift and autolockdownWrite"

requirements-completed: [MENU-06, MENU-08]

coverage:
  - id: D1
    description: "applyAutoLockdown is the one trigger write path; /autolockdown and /autoantiraid reply as before"
    requirement: "MENU-06"
    verification:
      - kind: unit
        ref: "alita/modules/lockdown_apply_test.go#TestApplyAutoLockdown"
        status: pass
      - kind: unit
        ref: "alita/modules/autolockdown_test.go#TestAutoLockdown, TestAutolockdownAI, TestAutoLockdownArgs, TestAutoLockdownAuthority, TestAutoLockdownBadInput, TestAutoLockdownBasicGroup, TestAutoLockdownSaveFails, TestAutoLockdownReadsAreBounded"
        status: pass
    human_judgment: false
  - id: D2
    description: "applyLockdownLift lifts only through liftLockdownRow and lockdownLiftApplyText renders every outcome with the text /unlockdown sends"
    requirement: "MENU-06"
    verification:
      - kind: unit
        ref: "alita/modules/lockdown_apply_test.go#TestApplyLockdownLift"
        status: pass
      - kind: unit
        ref: "alita/modules/lockdown_lift_test.go#TestLockdownLiftManualChange, TestLockdownLiftRestoreFailure, TestUnlockdownLiftsOnce, TestUnlockdownWithoutLockdown, TestLockdownLiftRecordFailure"
        status: pass
    human_judgment: false
  - id: D3
    description: "lockdownAnnounceLift posts the public lift notice through the lockdown pacer and wakes the worker; the alert's Lift Confirm calls it"
    requirement: "MENU-08"
    verification:
      - kind: unit
        ref: "alita/modules/lockdown_apply_test.go#TestLockdownAnnounceLift"
        status: pass
      - kind: unit
        ref: "alita/modules/lockdown_alert_lift_test.go#TestLockdownAlertLift, TestLockdownAlertLiftOutcomes"
        status: pass
    human_judgment: false
  - id: D4
    description: "lockdownStatusHead is the head of /lockdownstatus with an optional reason cut; the command passes 0 and replies as before"
    requirement: "MENU-08"
    verification:
      - kind: unit
        ref: "alita/modules/lockdown_apply_test.go#TestLockdownStatusHead"
        status: pass
      - kind: unit
        ref: "alita/modules/lockdown_status_test.go#TestLockdownStatusCommand"
        status: pass
    human_judgment: false

duration: 35min
completed: 2026-10-30
status: complete
---

# Phase 9 Plan 04: Lockdown apply functions Summary

**The lockdown trigger write, the lift, the lift announcement and the status head are now shared functions (applyAutoLockdown, applyLockdownLift, lockdownAnnounceLift, lockdownStatusHead), with /autolockdown, /autoantiraid, /unlockdown, the alert's Lift and /lockdownstatus behaving exactly as before.**

## Performance

- **Duration:** about 35 min
- **Completed:** 2026-10-30
- **Tasks:** 2
- **Files modified:** 5 (four refactored, one new test file)

## Accomplishments

- `applyAutoLockdown(chat, change)` refuses a non-supergroup, writes through the `autolockdownWrite` seam, maps `raiddetection.ErrThresholdOutOfRange` to a range refusal and any other error to a save failure, and reads the settings back under `raidSettingsTimeout` (a failed readback is kept in `ReadErr`). `autolockdownApplyText` renders each refusal with the exact existing key and params.
- `applyLockdownLift(ctx, b, chatID, actor)` reads the current lockdown through the new `lockdownCurrentLookup` seam and lifts only through `liftLockdownRow`; `lockdownLiftApplyText` gives each outcome the text `/unlockdown` replied. `/unlockdown` is now those two calls. `lockdownBeginLift(` is still called only inside `liftLockdownRow` (count 1 in lockdown.go).
- `lockdownAnnounceLift` is the public `lockdown_lifted` notice through `lockdownPostLiftNotice` (lockdown pacer, failure only logged) plus `wakeLockdownWorker`; the alert's Lift Confirm calls it in place of its two lines.
- `lockdownStatusHead(tr, row, reasonMaxRunes)` holds the head lines of `/lockdownstatus`; the command passes 0 and appends its remaining lines unchanged.
- New `lockdown_apply_test.go`: `TestApplyAutoLockdown`, `TestApplyLockdownLift`, `TestLockdownAnnounceLift`, `TestLockdownStatusHead`, on the existing lockdown env (SQLite, miniredis, hand-written fake, fast pacer), no mocks and no skips.

## Task Commits

1. **Task 1 RED:** `1431d02` test(09-04): pin the trigger apply function
2. **Task 1 GREEN:** `0a7c80d` refactor(09-04): extract applyAutoLockdown from /autolockdown
3. **Task 2 RED:** `a03ca72` test(09-04): pin the lift and status pieces
4. **Task 2 GREEN:** `4020e4e` refactor(09-04): extract the lift, its announcement and the status head

## Verification

- `CGO_ENABLED=0 go build ./...`: clean.
- Task 1 command: nine named tests, all `--- PASS` (eight pre-existing, unedited).
- Task 2 command: twelve named tests, all `--- PASS` (nine pre-existing, unedited).
- Wider pass over `LockdownStatus|LockdownTrigger|AutoAntiRaid|AntiRaid|LockdownAlert|LockdownAlready|Unlockdown` in `./alita/modules`: ok.
- `go vet -tags testtools ./alita/modules/` clean; `gofmt -l` empty on the five touched files. `make lint` was not run: golangci-lint in this sandbox is built with go1.25 against the module's go1.26.0, so gofmt and go vet stand in for it.
- The whole `./alita/modules` package and `make test` were not run (about 5 minutes; the orchestrator runs `make test` between waves).

## Deviations from Plan

None - plan executed exactly as written.

Two small choices inside the plan's latitude, not deviations:
- The commit trailer names the harness-supplied model line rather than the one printed in the plan text, per the project notes.
- `lockdownStatusHead` appends the cut mark to the reason before it is escaped and spliced, so it sits beside the reason in every locale; the plan only said "plus '…'".

## Known Stubs

None.

## Threat Flags

None. No new endpoint, auth path, file access or schema; the lift order (restore, then the one conditional record, then `FinishLift`) is untouched (T-09-16), the shared announcement posts for every button lift (T-09-17) and basic groups and out-of-range thresholds are refused before any write (T-09-18).

## Pending human verification

None for this plan. The live check of the menu's Lift and trigger buttons belongs to plans 09-10 and 09-12 and 09-SPIKE.md.

## Self-Check: PASSED

- Files exist: lockdown_apply_test.go, autolockdown.go, lockdown.go, lockdown_alert_callbacks.go, lockdown_status.go.
- Commits `1431d02`, `0a7c80d`, `a03ca72`, `4020e4e` exist on the branch.
- Acceptance greps: each of the six new functions found once; `lockdownBeginLift(` count in lockdown.go is 1.
