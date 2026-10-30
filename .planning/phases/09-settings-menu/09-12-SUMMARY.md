---
phase: 09-settings-menu
plan: 12
subsystem: telegram-ui
tags: [settings-menu, lockdown, lift, two-tap-card, callbacks, gotgbot]

requires:
  - phase: 09-settings-menu
    provides: the press pipeline, page registry and settingsRightRestrict (09-01), applyLockdownLift, lockdownLiftApplyText and lockdownAnnounceLift (09-04), the Lockdown page (09-10), the two-tap card helper settings_card.go (09-11)
provides:
  - "Lift on the Lockdown page: a Lift row while the lockdown is active, ls asks in place with a card, ly lifts through applyLockdownLift"
  - "renderSettingsLockdownLiftCard, settingsLockdownLiftAsk, settingsLockdownLiftYes"
  - "AGENTS.md rules naming the settings menu as a caller of the lift path and of lockdownAnnounceLift"
affects: [09-13, 09-14]

requirements-completed: [MENU-03, MENU-06, MENU-08]

plan_head_before: 3d179a6c8c42dfe638e48fda55edf92e2416896f
plan_head_after: 6c5fb019f9e53189f7b90c04f77d5a950b8eb63d
commits: 3

actuals:
  tokens: 10500
  tasks: 2
  commits: 3

key-files:
  created:
    - alita/modules/settings_lockdown_lift_test.go
  modified:
    - alita/modules/settings_lockdown.go
    - AGENTS.md

key-decisions:
  - "The Lift row is drawn for any lockdown in state active, confirmed or not, because /unlockdown lifts an unconfirmed row the same way; a lifting or lifted lockdown, or none, draws no Lift."
  - "Ask refusals (none, lifting, unreadable) reuse lockdownLiftApplyText with a synthetic lockdownLiftApply, so the menu says exactly what /unlockdown says."
  - "Yes redraws through settingsLockdownRedraw (a fresh load) so the page after a lift shows the real state, not a guess."

duration: 40min
completed: 2026-10-30
status: complete

coverage:
  - id: D1
    description: "An active lockdown's Lockdown page has the Lift button as its first content row; a lifting or lifted lockdown or none shows no Lift"
    requirement: "MENU-06"
    verification:
      - kind: unit
        ref: "alita/modules/settings_lockdown_lift_test.go#TestSettingsLockdownLift"
        status: pass
      - kind: unit
        ref: "alita/modules/settings_lockdown_lift_test.go#TestSettingsLockdownLiftOutcomes"
        status: pass
    human_judgment: false
  - id: D2
    description: "Lift asks a second tap in place (card naming the group, Yes with e, No back to the page, Back and Close); Yes answers first, lifts once through applyLockdownLift, posts the public lockdown_lifted notice and redraws with the result"
    requirement: "MENU-06"
    verification:
      - kind: unit
        ref: "alita/modules/settings_lockdown_lift_test.go#TestSettingsLockdownLift"
        status: pass
      - kind: unit
        ref: "alita/modules/settings_lockdown_lift_test.go#TestSettingsLockdownLiftPrivate"
        status: pass
    human_judgment: false
  - id: D3
    description: "Two Yes presses, or a Yes after /unlockdown, restore once and post one public notice; a failed restore or record keeps the lockdown active and posts nothing"
    requirement: "MENU-08"
    verification:
      - kind: unit
        ref: "alita/modules/settings_lockdown_lift_test.go#TestSettingsLockdownLiftTwice"
        status: pass
      - kind: unit
        ref: "alita/modules/settings_lockdown_lift_test.go#TestSettingsLockdownLiftOutcomes"
        status: pass
    human_judgment: false
  - id: D4
    description: "The restrict right is judged live at the ask and at Yes; an expired Yes lifts nothing; another administrator cannot use the opener's card"
    requirement: "MENU-03"
    verification:
      - kind: unit
        ref: "alita/modules/settings_lockdown_lift_test.go#TestSettingsLockdownLiftAuthority"
        status: pass
      - kind: unit
        ref: "alita/modules/settings_lockdown_lift_test.go#TestSettingsLockdownLiftExpired"
        status: pass
    human_judgment: false
  - id: D5
    description: "The menu's lift ends in the same state as /unlockdown (state, lifter, restored permissions, /lockdownstatus text); every Lift button fits 64 bytes and the card fits 3800 units in 7 languages"
    requirement: "MENU-08"
    verification:
      - kind: unit
        ref: "alita/modules/settings_lockdown_lift_test.go#TestSettingsParityLockdownLift"
        status: pass
      - kind: unit
        ref: "alita/modules/settings_lockdown_lift_test.go#TestSettingsLockdownLiftCallbackBytes"
        status: pass
      - kind: unit
        ref: "alita/modules/settings_lockdown_lift_test.go#TestSettingsLockdownLiftCardFitsEveryLocale"
        status: pass
    human_judgment: false
  - id: D6
    description: "How the Lift card and the redrawn page read on real Telegram clients, and the public lift notice appearing in the group when lifted from a private menu"
    verification: []
    human_judgment: true
    rationale: "Client rendering and real Bot API behaviour cannot be asserted by a fake; covered by the deferred real-device checklist 09-SPIKE.md"
---

# Phase 9 Plan 12: Lift from the settings menu Summary

**The Lockdown page lifts an active lockdown with two taps through `applyLockdownLift` (the path `/unlockdown` uses), checks the restrict right live at the ask and at Yes, and announces the lift in the group through `lockdownAnnounceLift` in both menu variants.**

## Accomplishments

- A Lift row, the first content row while the chat's lockdown is active; `ls` asks in place with a card (`lockdown_alert_lift_ask` naming the group, Yes with the expiry minute, No back to the page, Back and Close); `ly` answers the press empty first and lifts.
- Every lift outcome redraws the page with the command's own words (`lockdown_lifted`, `lockdown_already_lifted`, `lockdown_lift_in_progress`, `lockdown_restore_failed`, `lockdown_lift_record_failed`, `lockdown_not_active`, `lockdown_state_failed`) as a notice; a recorded lift also posts the public notice in the group and wakes the worker.
- Eight plan tests plus a locale-fit test for the card; the existing `TestUnlockdownLiftsOnce`, `TestLockdownAlertLift` and the `TestSettings*` / `TestLockdownAlert*` / `TestUnlockdown*` families still pass (92 s run).
- AGENTS.md: the lift path now lists three callers and the public notice is `lockdownAnnounceLift`; the settings Callbacks section documents `ls` / `ly`.

## Task Commits

1. RED tests for the Lift row and outcomes: `8eaac14` (`test(09-12)`)
2. Lift row, ask, Yes, card renderer: `64c0a95` (`feat(09-12)`)
3. Double submit, expiry, authority, private, parity, bytes tests and AGENTS.md: `6c5fb01` (`test(09-12)`)

## Deviations from Plan

None - plan executed exactly as written. Two small additions beyond the named tests: `TestSettingsLockdownLiftCardFitsEveryLocale` (the card text stays within 3800 UTF-16 units with an escaped 128-rune title in all 7 locales), and the parity test also covers a lockdown with a joiner still to unban.

The plan's text says the alert's Lift Confirm and `/unlockdown` both reach the lift through `applyLockdownLift`; in the code the alert's Confirm calls `liftLockdownRow` directly. The AGENTS.md wording states this precisely (`/unlockdown` and the menu through `applyLockdownLift`, the alert directly) rather than repeating the plan's phrase.

**Total deviations:** 0 auto-fixed.

## Verification

- `CGO_ENABLED=0 go build ./...` passes; `go vet -tags testtools ./alita/modules/` passes; `gofmt -l alita/` prints nothing. `make lint` was not run: golangci-lint in this sandbox is built with go1.25 against a go1.26.0 module, so gofmt and go vet stand in for it.
- Task 2's verify command (the ten named tests, with `-v`) passes with every `--- PASS` line seen.
- No migration, model, Redis key, shutdown drain or locale key was added; PostgreSQL was not started.

## Known Stubs

None.

## Threat Flags

None. The new surface (`ls`, `ly`) is covered by T-09-44 to T-09-48 of the plan.

## Pending human verification (deferred to 09-SPIKE.md)

On a real test group with the bot as an administrator who may restrict members:
1. Run `/lockdown`, open `/settings`, press Lockdown. The first row is the Lift button.
2. Press Lift: the page becomes the confirm card naming the group. Press No: the Lockdown page returns, the group is still locked.
3. Press Lift then Yes: the group's message permissions come back, the group shows one "lockdown lifted" message, and the page shows the result and no Lift button.
4. Repeat from the private entry (`/start settings_<chat id>` through Open in private): the lift notice appears in the group and nothing extra in the private chat.
5. With a second administrator who may not restrict members: Lift shows the "needs the right to restrict members" alert and no card.

## Self-Check: PASSED

- FOUND: alita/modules/settings_lockdown_lift_test.go, alita/modules/settings_lockdown.go, AGENTS.md
- FOUND commits: 8eaac14, 64c0a95, 6c5fb01
