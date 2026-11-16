---
phase: 10-pm-settings-hub-and-owner-gate
plan: 06
subsystem: settings-menu
tags: [settings, access, owner-switch, telegram, authority, agents-md]
requires:
  - phase: 10-pm-settings-hub-and-owner-gate
    provides: menuaccess.AdminsAllowedFresh and SetAdminsAllowed (10-01), settings test fixtures newOwner, otherReplica, setAdminsAllowed (10-04), settingsGate, settingsAccessLookup and press step 4b (10-05)
provides:
  - the Access page (key a, control s, action ay), OwnerOnly, drawn and opened only for the live creator
  - applyAdminsAllowed, the one write path of the owner switch, behind the settingsAccessWrite seam
  - settingsRightOwner, settingsPage.OwnerOnly, settingsView.Owner and settingsMainEntries(..., viewerIsOwner)
affects: [10-07, 10-08, 10-09, 10-10, 10-11]

status: complete
actuals:
  tokens: 21000
  tasks: 2
  commits: 2

commits: 2
plan_head_before: 68d0af8375c7679937cfe39393436f51707803f7
plan_head_after: 1fe2991be2e16a26b15509e667009c9b664ee587

tech-stack:
  added: []
  patterns:
    - "An OwnerOnly page is hidden from Main without reading its state and refused in openPage, so no route (open, cancel, refresh) reaches it"
    - "The apply function refuses a non-creator again behind the control's right (defence in depth)"
    - "A failed write redraws from the apply result's unchanged value, never a guess"

key-files:
  created:
    - fuku/modules/settings_access.go
    - fuku/modules/settings_access_test.go
  modified:
    - fuku/modules/settings_pages.go
    - fuku/modules/settings_menu.go
    - fuku/modules/settings.go
    - fuku/modules/settings_test.go
    - AGENTS.md

key-decisions:
  - "settingsRightOwner is appended after settingsRightRestrict, so the zero value of settingsRight stays settingsRightAdmin"
  - "An expired Yes reads the switch once to redraw the page; an unreadable switch there is the cannot-open edit"
  - "Until plan 10-10 the Access entry is drawn as the last one-column Main button (Order 90)"

requirements-completed: [MENU-11, MENU-12, MENU-14]

coverage:
  - id: D1
    description: "The live owner opens Access from Main (group and private menu) and turns admin access ON with two taps and OFF with one; the admin's next press follows"
    requirement: MENU-12
    verification:
      - kind: unit
        ref: "fuku/modules/settings_access_test.go#TestSettingsAccessOwnerFlow"
        status: pass
    human_judgment: false
  - id: D2
    description: "Access is invisible to an allowed admin (no Main button or text, no MainState read) in a group and a private menu"
    requirement: MENU-12
    verification:
      - kind: unit
        ref: "fuku/modules/settings_access_test.go#TestSettingsAccessHiddenFromAdmins"
        status: pass
    human_judgment: false
  - id: D3
    description: "An allowed admin's forged op, no, tg, st and ay presses answer settings_access_owner_only once, edit nothing and write nothing"
    requirement: MENU-11
    verification:
      - kind: unit
        ref: "fuku/modules/settings_access_test.go#TestSettingsAccessForgedPresses"
        status: pass
      - kind: unit
        ref: "fuku/modules/settings_access_test.go#TestSettingsOwnerAuthorityAccess"
        status: pass
    human_judgment: false
  - id: D4
    description: "The ON card: st v=1 never writes, expired Yes writes nothing, bad e is an invalid request, a Yes by a demoted owner is refused, repeated taps are harmless"
    requirement: MENU-12
    verification:
      - kind: unit
        ref: "fuku/modules/settings_access_test.go#TestSettingsAccessCard"
        status: pass
    human_judgment: false
  - id: D5
    description: "A failed write redraws with common_settings_save_failed and the stored value unchanged; an unreadable switch is the cannot-open edit on the page and the unknown mark on Main"
    requirement: MENU-12
    verification:
      - kind: unit
        ref: "fuku/modules/settings_access_test.go#TestSettingsAccessWriteFailure"
        status: pass
      - kind: unit
        ref: "fuku/modules/settings_access_test.go#TestSettingsAccessReadFailure"
        status: pass
    human_judgment: false
  - id: D6
    description: "A flip on one replica is seen by the very next press on another in both directions, a memoised owner never hides a flip to ON, and a chat with no row is owner only on every replica; nothing is announced"
    requirement: MENU-14
    verification:
      - kind: unit
        ref: "fuku/modules/settings_access_test.go#TestSettingsAccessAcrossReplicas"
        status: pass
      - kind: unit
        ref: "fuku/modules/settings_access_test.go#TestSettingsAccessNoRowIsOwnerOnly"
        status: pass
      - kind: unit
        ref: "fuku/modules/settings_access_test.go#TestSettingsAccessNoAnnouncement"
        status: pass
    human_judgment: false
  - id: D7
    description: "Callback data of every Access button is at most 64 bytes with Omitted 0, and the page and card fit 3800 UTF-16 units in all 8 languages with a 64-rune multibyte title; the toggle is at most 40 code points"
    requirement: MENU-12
    verification:
      - kind: unit
        ref: "fuku/modules/settings_access_test.go#TestSettingsAccessCallbackBytes"
        status: pass
      - kind: unit
        ref: "fuku/modules/settings_access_test.go#TestSettingsAccessPageFitsEveryLocale"
        status: pass
    human_judgment: false
---

# Phase 10 Plan 06: Access page and owner-only right Summary

The live owner of a group can now open an Access page from the settings menu and let admins in with two taps (a confirm card) or shut them out with one tap, and the change governs the very next press on every replica; nobody but the live creator can see or press any of it.

## Accomplishments

- **Registry and pipeline** (`settings_pages.go`, `settings_menu.go`, `settings.go`): `settingsPage.OwnerOnly`, `settingsView.Owner`, `settingsMainEntries(..., viewerIsOwner)` (skips an owner-only page without calling its `MainState`), `settingsRightOwner` (creator from the press's own member record, alert `settings_access_owner_only`), an `openPage` guard that refuses an owner-only page for a non-creator, and the deep link setting `Owner`.
- **`fuku/modules/settings_access.go`**: page `a` (Order 90, icon key), control `s` for `tg` and `st` alike, action `ay`. `applyAdminsAllowed` holds the non-creator refusal, the write through the `settingsAccessWrite` seam and the value to draw; there is no cache layer import because the table is read fresh. The page and the gate read through the same `settingsAccessLookup` seam.
- **Tests** (`settings_access_test.go`): the twelve tests named in the plan plus `TestSettingsAccessNoRowIsOwnerOnly`. A mutation check (owner right always passing, `openPage` guard disabled) made `TestSettingsAccessForgedPresses` and `TestSettingsOwnerAuthorityAccess` fail, then the file was restored. The forged `ay`, `tg v=0` and `st v=0` presses still failed to write under that mutation because `applyAdminsAllowed` refuses a non-creator on its own.
- **AGENTS.md**: the page key list, a new Access page bullet under Callbacks, `settingsRightOwner` under Permissions, and the new tests under Testing.

## Task Commits

1. Task 1 (tracer): Access page with two-tap ON and one-tap OFF - `6ec88f5`
2. Task 2: forgery, failure and cross-replica tests plus AGENTS.md - `1fe2991`

Tracer gate: the tracer `<verify>` is automated only (`end-of-phase` mode), so the whole settings suite was re-run end to end before expansion and passed.

## Verification Results

- `go test -tags testtools -race -count=1 -run '^TestSettingsAccessOwnerFlow$' ./fuku/modules` - PASS.
- `go test -tags testtools -race -count=1 -run '^(TestSettingsAccess|TestSettingsDriftAccess|TestSettingsOwnerAuthorityAccess)' ./fuku/modules` - `ok` (8.6 s).
- `-v` run of `TestSettingsAccessAcrossReplicas`, `TestSettingsAccessForgedPresses`, `TestSettingsAccessCallbackBytes`, `TestSettingsAccessPageFitsEveryLocale` - `--- PASS` for all four.
- `go test -tags testtools -race -count=1 -run '^(TestSettings|TestLoadSettings)' ./fuku/modules ./fuku/i18n` - `ok fuku/modules 84.7s`, `ok fuku/i18n 4.8s`.
- `CGO_ENABLED=0 go build ./...` and `go vet -tags testtools ./fuku/modules` clean; `git diff --exit-code 6ef6ad9 -- go.mod go.sum` clean; `make check-translations` all present.
- Acceptance greps: `OwnerOnly` in settings_pages.go at least 2, `settingsRightOwner` in settings_menu.go at least 2, one `Key: settingsPageAccess` and one `OwnerOnly: true` in settings_access.go, no `db/cache` or `utils/cache` import there, `settingsRightOwner` and `TestSettingsAccessAcrossReplicas` in AGENTS.md.
- Not run: `make lint` (the installed golangci-lint is built with Go 1.25, as noted in 10-05) and the full `make test`.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] An existing Main page test counted every registered page**
- **Found during:** Task 1 verification (`TestSettingsTracer/Open_here_becomes_the_Main_page`)
- **Issue:** the test expected one Main row per registered page, but the admin it opens the menu as is not the creator, so the owner-only Access page is now correctly absent.
- **Fix:** the test counts the registered pages that are not `OwnerOnly`.
- **Files modified:** fuku/modules/settings_test.go
- **Commit:** 6ec88f5

Otherwise the plan was executed as written.

## Known Stubs

None.

## Threat Flags

None. T-10-16, T-10-17 and T-10-18 are mitigated and tested as planned (`TestSettingsAccessForgedPresses`, `TestSettingsOwnerAuthorityAccess`, `TestSettingsAccessCard`, `TestSettingsAccessAcrossReplicas`).

## Self-Check: PASSED

- Created files present: `fuku/modules/settings_access.go`, `fuku/modules/settings_access_test.go`.
- Commits `6ec88f5` and `1fe2991` are ancestors of HEAD.
- All task acceptance criteria and plan-level verification commands above re-run and passing (except the lint and full `make test` runs noted above).
