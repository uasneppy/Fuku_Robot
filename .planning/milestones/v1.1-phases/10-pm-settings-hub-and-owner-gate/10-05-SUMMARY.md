---
phase: 10-pm-settings-hub-and-owner-gate
plan: 05
subsystem: permissions
tags: [settings, owner-gate, telegram, singleflight, authority, agents-md]
requires:
  - phase: 10-pm-settings-hub-and-owner-gate
    provides: menuaccess.AdminsAllowedFresh (10-01), the settings test fixtures newOwner, setAdminsAllowed, listAdmin, setDeletedCreator, clearCreator (10-04)
provides:
  - settingsGate, the one owner-gate decision (creator, then the fresh switch, then the owner state)
  - settingsOwnerState, a getChatAdministrators read with a 3 s bound, per-chat singleflight and a positive-only 30 s memo
  - settingsCreatorGone and isTelegramClientError (the classifier plans 10-07, 10-09 and 10-11 call)
  - requireSettingsAccess on in-group /settings and its anonymous re-entry
  - press step 4b in settingsPress.verify
  - lockdownAuthorityMember, extracted from requireLockdownAuthority with no behaviour change
affects: [10-06, 10-07, 10-08, 10-09, 10-10, 10-11]

status: complete
actuals:
  tokens: 14700
  tasks: 2
  commits: 2

commits: 2
plan_head_before: 0475c669f633d7dbbfd25c6d25a9ddf8c7f6685e
plan_head_after: f2057c6e6809c5c49ae2c4aeac1d5651e3514830

tech-stack:
  added: []
  patterns:
    - "One gate function every settings door calls after its own live getChatMember; unknown always refuses"
    - "Zero values of the gate verdict and owner-state enums are the refusing ones (undecided, unknown)"
    - "Only 'an owner exists' is memoised, because it can only keep an administrator out"

key-files:
  created:
    - fuku/modules/settings_gate.go
    - fuku/modules/settings_gate_test.go
  modified:
    - fuku/modules/lockdown.go
    - fuku/modules/settings.go
    - fuku/modules/settings_menu.go
    - AGENTS.md

key-decisions:
  - "settingsGateUndecided and settingsOwnerUnknown are the iota zero values, so an uninitialised verdict fails closed"
  - "The owner-state memo drops expired entries once it holds 4096 chats, so it cannot grow without bound"
  - "A late getChatAdministrators answer (after the 3 s deadline) is discarded inside the flight, so it is neither used nor memoised"

requirements-completed: [MENU-11, MENU-13, MENU-14]

coverage:
  - id: D1
    description: "In-group /settings is owner only by default: a live creator gets the choice message, a non-owner administrator gets the plain settings_refuse_owner_only reply, an allowed administrator or one in an ownerless group gets in, unknown refuses with settings_check_failed, a plain member sees no settings text"
    requirement: MENU-11
    verification:
      - kind: unit
        ref: "fuku/modules/settings_gate_test.go#TestSettingsInGroupRefusal"
        status: pass
    human_judgment: false
  - id: D2
    description: "The anonymous re-entry decides on the tapper: an anonymous owner gets the choice message, an anonymous gated administrator gets settings_refuse_owner_only"
    requirement: MENU-14
    verification:
      - kind: unit
        ref: "fuku/modules/settings_gate_test.go#TestSettingsAnonymousOwnerGate"
        status: pass
    human_judgment: false
  - id: D3
    description: "Every button press runs the gate (step 4b): an open menu of a newly gated administrator is refused on its next press (control, Back, Close), edits and writes nothing, and a private press is refused before getChat"
    requirement: MENU-11
    verification:
      - kind: unit
        ref: "fuku/modules/settings_gate_test.go#TestSettingsGatePress"
        status: pass
    human_judgment: false
  - id: D4
    description: "Owner-state rules: creator, switch, deleted-account heuristic, fail-closed on error, timeout, late answer, empty list, missing presser and presser-as-creator"
    requirement: MENU-11
    verification:
      - kind: unit
        ref: "fuku/modules/settings_gate_test.go#TestSettingsGate"
        status: pass
      - kind: unit
        ref: "fuku/modules/settings_gate_test.go#TestSettingsOwnerState"
        status: pass
    human_judgment: false
  - id: D5
    description: "One getChatAdministrators call for concurrent readers of a chat, a positive-only memo that expires at exactly 30 s, and a switch read that precedes the memo"
    requirement: MENU-14
    verification:
      - kind: unit
        ref: "fuku/modules/settings_gate_test.go#TestSettingsOwnerStateSingleflight"
        status: pass
      - kind: unit
        ref: "fuku/modules/settings_gate_test.go#TestSettingsOwnerStateMemo"
        status: pass
    human_judgment: false
  - id: D6
    description: "isTelegramClientError is true for 4xx except 429 (also wrapped) and false for 429, 5xx and non-Telegram errors"
    requirement: MENU-11
    verification:
      - kind: unit
        ref: "fuku/modules/settings_gate_test.go#TestSettingsTelegramClientError"
        status: pass
    human_judgment: false
  - id: D7
    description: "The setting commands stay ungated (MENU-13): the gated administrator's /captchamode, /setflood and /autolockdown still work, and the parity carve-out is documented and tested"
    requirement: MENU-13
    verification:
      - kind: unit
        ref: "fuku/modules/settings_gate_test.go#TestSettingsCommandsUngated"
        status: pass
      - kind: unit
        ref: "fuku/modules/settings_gate_test.go#TestSettingsAuthorityParityOwnerGate"
        status: pass
    human_judgment: false
  - id: D8
    description: "The live behaviour against real Telegram (deleted-owner entry shape A1, a plain-member bot able to call getChatAdministrators A2, an anonymous creator reported as creator A4) is unconfirmed"
    requirement: MENU-11
    verification: []
    human_judgment: true
    rationale: "Planner assumptions A1, A2 and A4 describe Telegram's real answers and cannot be asserted against a hand-written fake; the gate fails closed if they are wrong. The manual, non-blocking confirmation is listed in the phase SPIKE file (plan 10-10)."
---

# Phase 10 Plan 05: Owner gate on in-group /settings and every press Summary

The settings menu is now owner only by default: `settingsGate` (creator, then the fresh switch, then a bounded and singleflighted `getChatAdministrators` owner-state read) guards in-group `/settings` with its anonymous re-entry and every button press, and refuses with `settings_check_failed` whenever the switch or the owner state cannot be established.

## Accomplishments

- **`lockdownAuthorityMember`** extracted from `requireLockdownAuthority` (`fuku/modules/lockdown.go`); the old function is a three-line wrapper and the whole lockdown, autolockdown and unlockdown test set passes unchanged.
- **`fuku/modules/settings_gate.go`**: `settingsGate`, `settingsOwnerState` (3 s bound, per-chat `singleflight`, a late answer discarded, a memo only for "an owner exists" that is expired at exactly 30 s and pruned past 4096 chats), `settingsEvaluateOwner`, `settingsCreatorGone` (reuses `isDeletedAccount`, plus no last name and no username), `isTelegramClientError`, and `requireSettingsAccess`. Seams: `settingsAccessLookup`, `settingsOwnerLookup`, `settingsOwnerFetch`, `settingsOwnerNow`, `settingsOwnerMemoTTL`, `settingsOwnerLookupTimeout`.
- **In-group door**: `settingsDesc.RequiredChecks` is `requireLockdownGroup()` then `requireSettingsAccess()`. A non-owner administrator with the switch off gets one plain reply (`settings_refuse_owner_only`, no keyboard); unknown gets `settings_check_failed`; a plain member still gets the lockdown check's own refusal.
- **Press step 4b** in `settingsPress.verify`, after the live member check and before the private `getChat`: `settings_gate_owner_only` or `settings_check_failed`, answered once, nothing edited or written (D-10), Close included.
- **Tests** (`fuku/modules/settings_gate_test.go`): the ten named tests of the plan. A mutation check (turning the owner-only refusal at step 4b into a no-op) made `TestSettingsGatePress` and `TestSettingsAuthorityParityOwnerGate` fail, then the file was restored.
- **AGENTS.md**: the Settings module paragraph (Handlers), press step 4b (Callbacks) and a new "settings owner gate" bullet (Permissions).

## Task Commits

1. Task 1 (tracer): in-group `/settings` gate - `5b05c60`
2. Task 2: press step 4b, owner-state rules, tests and AGENTS.md - `f2057c6`

Tracer gate: the tracer `<verify>` is automated only (`end-of-phase` mode), so it was re-run end to end before expansion and passed (`TestSettingsInGroupRefusal`, `TestSettingsCommandsUngated`, `TestSettingsAnonymousOwnerGate`, `TestSettingsAnonymousAdmin`, and the lockdown set).

## Verification Results

- `go test -tags testtools -race -count=1 -run '^(TestSettingsInGroupRefusal|TestSettingsCommandsUngated|TestSettingsAnonymousOwnerGate|TestSettingsAnonymousAdmin)$' ./fuku/modules` - PASS.
- `go test -tags testtools -race -count=1 -run '^(TestLockdown|TestAutoLockdown|TestUnlockdown)' ./fuku/modules` - `ok 90.331s`.
- `go test -tags testtools -race -count=1 -run '^(TestSettingsGatePress|TestSettingsGate|TestSettingsOwnerState|TestSettingsOwnerStateSingleflight|TestSettingsOwnerStateMemo|TestSettingsTelegramClientError|TestSettingsAuthorityParityOwnerGate)$' ./fuku/modules` - all PASS.
- `go test -tags testtools -race -count=1 -run '^(TestSettings|TestLoadSettings|TestLockdown|TestAutoLockdown|TestUnlockdown)' ./fuku/modules ./fuku/i18n` - `ok fuku/modules 165.336s`, `ok fuku/i18n 9.526s` (every Phase 8/9 press and lockdown-authority test green).
- `go test ... -run '^TestSettingsAuthorityParity' -v` - PASS for Antiflood, Captcha, OwnerGate, LockdownTriggers and Staff.
- Acceptance greps: `lockdownAuthorityMember` 1, `requireSettingsAccess()` in settings.go 1, `singleflight` 2, `!= 429` 1, `isDeletedAccount(` outside comments 1 (and 1 definition in misc_extras.go), `MergeChatMember()` outside comments 1, `settingsGate(b, groupID, query.From, member)` 1 at line 326 before `GetChatWithContext(` at line 340, `settingsGate` in AGENTS.md 2, `settings_refuse_owner_only` 2, `TestSettingsAuthorityParityOwnerGate` 1.
- `CGO_ENABLED=0 go build ./...` passes; `go vet -tags testtools ./fuku/modules` is clean; `git diff --exit-code 6ef6ad9 -- go.mod go.sum` is clean.
- `make lint` could not run: the installed golangci-lint was built with Go 1.25 and refuses the repository's Go 1.26.0 target. `make test` (the full run) was not executed; the plan's own scoped commands above were.

## Deviations from Plan

None - plan executed exactly as written.

Small choices inside the plan's freedom: the two enums put the refusing value at iota zero (the plan listed the constants in another order, which it did not make binding), and the memo prunes expired chats at 4096 entries.

## Known Stubs

None.

## Threat Flags

None. The new surface (a `getChatAdministrators` call per gated press) is covered by the plan's T-10-10, T-10-13 and T-10-14.

## Self-Check: PASSED

- Created files present: `fuku/modules/settings_gate.go`, `fuku/modules/settings_gate_test.go`.
- Commits `5b05c60` and `f2057c6` are ancestors of HEAD.
- All task acceptance criteria and plan-level verification commands above re-run and passing (except the lint and full `make test` runs noted above).
