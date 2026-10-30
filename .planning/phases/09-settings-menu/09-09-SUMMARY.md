---
phase: 09-settings-menu
plan: 09
subsystem: telegram-ui
tags: [settings-menu, antiflood, stepper, apply-functions, gotgbot]

requires:
  - phase: 09-settings-menu
    provides: "plan 09-01 page registry, press pipeline and settingsRightAdminFloodBot; plan 09-03 applyFloodLimit, applyFloodMode, applyFloodDelete; plan 09-05 page strings"
provides:
  - "Antiflood page (p=f) of the settings menu: limit stepper with an Off position, mute/ban/kick action row, delete flood messages switch"
  - "settingsFloodRead seam over GetAntifloodSettingsCachedContext"
  - "Main page Antiflood button state (Limit n or Off)"
affects: [09-13 docs]

plan_head_before: b3c964cbc69d121a2911ef0e7ac19cdd43864331
plan_head_after: 50fbea559c9f2e05e36495757ae10e6996857f80

actuals:
  tokens: 28000
  tasks: 2
  commits: 3

tech-stack:
  added: []
  patterns:
    - "page file registers itself from init with registerSettingsPage; no shared settings file edited"
    - "every press writes through the command's apply function and redraws from its database read-back"

key-files:
  created:
    - alita/modules/settings_antiflood.go
    - alita/modules/settings_antiflood_test.go
  modified: []

key-decisions:
  - "The limit stepper is hand-built (settingsFloodLimitRow) because the shared settingsStepperRow uses one value as both base and comparison; Off needs a base of 2 while its stored value is 0"
  - "A press that the apply function refuses redraws from the row read before the press; a write whose read-back failed re-reads through settingsFloodRead, and an unreadable page is the cannot-open edit"
  - "The unknown-action notice is the command's antiflood_setfloodmode_unknown_type with the word HTML-escaped (the command itself leaves it unescaped)"

requirements-completed: [MENU-03, MENU-07, MENU-08]

coverage:
  - id: D1
    description: "The Antiflood page shows the limit or the off line, the action, the delete switch and a limit stepper with Off, 3..100; Main shows Limit n or Off"
    requirement: "MENU-07"
    verification:
      - kind: unit
        ref: "alita/modules/settings_antiflood_test.go#TestSettingsAntifloodPage"
        status: pass
      - kind: unit
        ref: "alita/modules/settings_antiflood_test.go#TestSettingsAntifloodStepper"
        status: pass
    human_judgment: false
  - id: D2
    description: "Presses write through applyFloodLimit/applyFloodMode/applyFloodDelete and the redraw shows the stored row, including the first-write limit on a chat with no row"
    requirement: "MENU-08"
    verification:
      - kind: unit
        ref: "alita/modules/settings_antiflood_test.go#TestSettingsAntifloodFreshChat"
        status: pass
      - kind: unit
        ref: "alita/modules/settings_antiflood_test.go#TestSettingsParityAntiflood"
        status: pass
      - kind: unit
        ref: "alita/modules/settings_antiflood_test.go#TestSettingsDriftAntiflood"
        status: pass
    human_judgment: false
  - id: D3
    description: "A stored warn, tban or tmute is shown read-only with a note and no selected choice; an unreadable page is the cannot-open edit, never off"
    requirement: "MENU-07"
    verification:
      - kind: unit
        ref: "alita/modules/settings_antiflood_test.go#TestSettingsAntifloodStoredLegacyAction"
        status: pass
      - kind: unit
        ref: "alita/modules/settings_antiflood_test.go#TestSettingsAntifloodReadFailure"
        status: pass
    human_judgment: false
  - id: D4
    description: "Antiflood changes need the live admin and the bot as admin with the commands' own refusal; viewing needs only the live admin"
    requirement: "MENU-03"
    verification:
      - kind: unit
        ref: "alita/modules/settings_antiflood_test.go#TestSettingsAuthorityParityAntiflood"
        status: pass
    human_judgment: false
  - id: D5
    description: "Every Antiflood button fits 64 bytes in both menu variants and the page fits 3800 UTF-16 units in all 7 locales"
    verification:
      - kind: unit
        ref: "alita/modules/settings_antiflood_test.go#TestSettingsAntifloodCallbackBytes"
        status: pass
      - kind: unit
        ref: "alita/modules/settings_antiflood_test.go#TestSettingsAntifloodPageFitsEveryLocale"
        status: pass
    human_judgment: false
  - id: D6
    description: "How the page reads on real Telegram clients (label truncation in hi and ru, the toast text for the bounds)"
    verification: []
    human_judgment: true
    rationale: "Client rendering cannot be asserted by a fake; covered by the deferred real-device checklist 09-SPIKE.md"

duration: 20min
completed: 2026-10-30
status: complete
---

# Phase 9 Plan 09: Antiflood settings page Summary

**An admin sets the antiflood limit (with an Off position), action and message deletion from the menu; each press goes through the same apply function as /setflood, /setfloodmode and /delflood and the page is drawn from what the database stored.**

## Performance

- **Duration:** about 20 min
- **Tasks:** 2 of 2
- **Files:** 2 created (`alita/modules/settings_antiflood.go`, `alita/modules/settings_antiflood_test.go`)
- **Commits:** 3 (`4176c38` RED, `1f5ab0a` feature, `50fbea5` parity and limits tests), then this SUMMARY

## Accomplishments

- The page registers itself (`settingsPageAntiflood` "f", order 40) from its own `init`; no shared settings file, `antiflood.go`, locale file or AGENTS.md was touched.
- Limit stepper `[-10 | -1 | value | +1 | +10]`: Off, then 3 to 100. Off sits below 3, so from 3 the minus steps give Off, from Off +1 gives 3 and +10 gives 12, and a step past 100 lands on 100. A step whose target equals the stored value (Off, 100) is labelled "·" and answers the `settings_min_value` (with the Off word) or `settings_max_value` toast, writing nothing.
- Action row Mute / Ban / Kick and the delete switch, all with `settingsRightAdminFloodBot`: a bot that is not an admin gets the command's `connections_is_user_connected_bot_not_admin` as a notice and nothing is written; viewing needs only the live admin.
- The redraw is the apply function's database read-back (`floodApplyResult.Stored`). A first Ban press on a chat with no row shows the limit the repository really stored (5, pinned by plan 09-03), identical to what `/setfloodmode ban` leaves.
- A stored `warn`, `tban` or `tmute` is shown by name with `settings_antiflood_action_note` and no selected choice; pressing Mute, Ban or Kick replaces it.
- The page reads through `settingsFloodRead` (`GetAntifloodSettingsCachedContext`); a read error is the cannot-open edit, and the Main button shows " · ❔".
- The menu refuses exactly what the commands refuse: limits 1, 2, 101 (`antiflood_errors_set_in_limit`) and mode warn (`antiflood_setfloodmode_unknown_type`); malformed forged values (non-digit or 4-digit limit, upper-case or over-long action, delete value other than 0/1) get the invalid-request alert and write nothing.

## Task Commits

1. Task 1 RED: `4176c38` test(09-09): show there is no Antiflood page
2. Task 1 GREEN: `1f5ab0a` feat(09-09): add the Antiflood page to the settings menu
3. Task 2: `50fbea5` test(09-09): prove the Antiflood page and the antiflood commands agree

## Verification

- `CGO_ENABLED=0 go build ./...`: ok
- Task 2 verify command (eleven named tests, run with `-v`, all `--- PASS`): `TestSettingsParityAntiflood`, `TestSettingsDriftAntiflood`, `TestSettingsAuthorityParityAntiflood`, `TestSettingsAntifloodCallbackBytes`, `TestSettingsAntifloodPageFitsEveryLocale`, `TestSettingsAntifloodPage`, `TestSettingsAntifloodStepper`, `TestSettingsAntifloodFreshChat`, `TestSettingsAntifloodStoredLegacyAction`, `TestSettingsAntifloodReadFailure`, `TestAntifloodCommandsUpdateSettingsAndDisplay`.
- Pinning set: `-run '^(TestSettings|TestApplyFlood|TestAntiflood|TestLoadAntiflood)'` on `./alita/modules`: ok (all earlier settings tests still pass with a fourth page registered).
- Mutation check: giving the limit control `settingsRightAdmin` instead of `settingsRightAdminFloodBot` makes `TestSettingsAuthorityParityAntiflood` fail (the step wrote a row with the bot not an admin); restored before commit.
- `go vet -tags testtools ./alita/modules/` clean, `gofmt -l alita/` empty, `make check-translations` and `make check-docs` clean. `make lint` was not run: golangci-lint is built with go1.25 against the go1.26.0 module in this sandbox, so `gofmt` and `go vet` stand in. The whole `make test` was not run (the orchestrator runs it between waves).
- Acceptance greps: one call each of `applyFloodLimit(`, `applyFloodMode(`, `applyFloodDelete(`; `settingsRightAdminFloodBot` on the three controls; `GetAntifloodSettingsCachedContext` is the seam default. `git diff --name-only b3c964c..HEAD` lists only the two plan files.

## Deviations from Plan

**1. [Rule 3 - Process] Plan-commit ledger file not written**
- The harness refuses compound git commands and a write into the shared `.git` path, as in plan 09-01. `commits: 3` and the two head SHAs were measured directly with `git rev-list --count b3c964c..HEAD` before this SUMMARY commit.

**2. [Test shape] `/flood` replies are checked by marker and numbers**
- With the marker locale the printf templates of `antiflood_flood_show_settings` lose their verbs, so the reply is checked for the key's marker plus the values (12, banned) that `fmt` appends; no literal English is asserted.

**Total deviations:** 2 (process/test shape, no behaviour or scope change). **Impact:** none.

## Known Stubs

None.

## Threat Flags

None. T-09-31 (bot-admin authority parity), T-09-32 (legacy actions read-only) and T-09-33 (read failure is cannot-open, never off) are covered by the tests named above. No endpoint, schema or secret was added.

## Follow-ups

- AGENTS.md: the first-write side effect (picking an action or the delete switch on a chat with no antiflood row also turns antiflood on at the default limit 5, a Mute press there writes nothing) goes into the docs with plan 09-13.
- Accepted skew, as for the commands: the `antiflood` cache key is not in `skipLocal`, so another replica can show an old value for up to `CACHE_LOCAL_TTL`.

## Pending human verification (deferred to 09-SPIKE.md, not blocking)

On a real bot and group (bot an administrator), as a group admin: send `/settings`, open Open here, tap Antiflood. Step the limit up from Off (+1 gives 3, +10 gives 12) and down to Off; check the toast at Off and at 100; pick Mute, Ban and Kick; toggle Delete flood messages; compare with `/flood` after each. Remove the bot's admin status and confirm a change shows the "I'm not an admin in this chat!" notice. Check that the stepper labels and the Action line are not cut in hi and ru.

## Self-Check: PASSED

- `alita/modules/settings_antiflood.go` and `alita/modules/settings_antiflood_test.go` exist; `4176c38`, `1f5ab0a` and `50fbea5` are in `git log` on this branch; the working tree was clean after the last code commit.
