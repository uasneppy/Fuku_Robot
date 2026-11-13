---
phase: 10-pm-settings-hub-and-owner-gate
plan: 11
subsystem: permissions
tags: [settings, deep-link, owner-gate, telegram-errors, privacy]
requires:
  - phase: 10-pm-settings-hub-and-owner-gate
    provides: settingsGate, isTelegramClientError and the gate test seams (10-05), the deep link's Owner flag (10-06), the settings fixtures newOwner, listAdmin, clearCreator, setAdminsAllowed (10-04)
provides:
  - settingsStartDeepLink with the order parse, live getChatMember, settingsGate, getChat, every Telegram error classified by isTelegramClientError
  - TestSettingsStartDeepLinkSameAnswer, the proof that the deep link is no oracle for the bot's membership
  - TestSettingsGateAllDoors with named door drivers, ready for plan 10-09 to append the pick door
  - AGENTS.md and the permissions reference describing the owner gate on every door
affects: [10-08, 10-09, 10-10]

status: complete
actuals:
  tokens: 5300
  tasks: 2
  commits: 2

commits: 2
plan_head_before: 6c0881de07522b6355f3708294556e088df70391
plan_head_after: 49daf3d74b1f5c4c065ecc50e479c73cbb9a0108

tech-stack:
  added: []
  patterns:
    - "Telegram errors of an attacker-driven lookup are classified by class (4xx except 429 is the client's answer), never by wording, so an unrecognised wording cannot leak a different reply"
    - "A door test is a table of fixtures by a slice of named door drivers, so a new door adds one entry and the failure message names the fixture and the door that disagrees"

key-files:
  created: []
  modified:
    - fuku/modules/settings.go
    - fuku/modules/settings_private_test.go
    - fuku/modules/settings_gate_test.go
    - AGENTS.md
    - docs/src/content/docs/api-reference/permissions.md

key-decisions:
  - "The deep link no longer answers settings_cannot_open: a 4xx at getChat and a chat that is not a group or supergroup answer settings_start_not_admin, the plain member's words (UI-SPEC R3)"
  - "settingsGate runs on the member record of the deep link's own getChatMember and before getChat, so a gated administrator costs no getChat call and a non-admin never sees settings_start_owner_only"
  - "The AGENTS.md deep link paragraph was rewritten in the task 1 commit, not task 2, because task 1's code change is what invalidated it; task 2 adds the Permissions bullet"

requirements-completed: []

coverage:
  - id: D1
    description: "A gated administrator (switch off or no row, a live owner) is refused at the deep link with settings_start_owner_only, no keyboard, no Main page and no getChat call; the owner, an allowed administrator and an administrator of a group with no live owner get the Main page"
    requirement: MENU-11
    verification:
      - kind: unit
        ref: "fuku/modules/settings_private_test.go#TestSettingsStartDeepLink/the_owner_gate"
        status: pass
    human_judgment: false
  - id: D2
    description: "An unreadable switch or an unknown owner state refuses with settings_check_failed at the deep link"
    requirement: MENU-11
    verification:
      - kind: unit
        ref: "fuku/modules/settings_private_test.go#TestSettingsStartDeepLink/the_owner_gate"
        status: pass
    human_judgment: false
  - id: D3
    description: "Telegram errors are classified by class: a 4xx other than 429 at getChatMember or getChat, a channel and every non-admin status answer settings_start_not_admin; 429, 5xx and transport errors answer settings_check_failed"
    requirement: MENU-13
    verification:
      - kind: unit
        ref: "fuku/modules/settings_private_test.go#TestSettingsStartDeepLink/the_refusals"
        status: pass
    human_judgment: false
  - id: D4
    description: "A plain member, a chat the bot is not in, a gone chat, an unrecognised 400 and an unrecognised 403, and a channel all get the same reply text and markup"
    requirement: MENU-13
    verification:
      - kind: unit
        ref: "fuku/modules/settings_private_test.go#TestSettingsStartDeepLinkSameAnswer"
        status: pass
    human_judgment: false
  - id: D5
    description: "Every door (group /settings, anonymous re-entry, deep link, press) gives the same decision over owner with the switch off (administrator and owner), switch on, no owner and owner state unknown"
    requirement: MENU-11
    verification:
      - kind: unit
        ref: "fuku/modules/settings_gate_test.go#TestSettingsGateAllDoors"
        status: pass
    human_judgment: false
  - id: D6
    description: "AGENTS.md and the permissions reference say that only the owner can open the menu by default through any door, how the owner lets admins in, and that the setting commands are unchanged"
    requirement: MENU-11
    verification:
      - kind: command
        ref: "make check-docs"
        status: pass
    human_judgment: true
    rationale: "Prose accuracy of the documentation is a reading judgment; only the docs drift check and the marker are automated"
---

# Phase 10 Plan 11: Deep link owner gate and same-answer rule Summary

**The `/start settings_<chat id>` deep link now runs the owner gate on its own live member record before `getChat`, answers every Telegram error by class instead of by wording, and gives a stranger the same reply whether or not the bot is in the named chat.**

## Performance

- **Duration:** about 15 min
- **Tasks:** 2 of 2
- **Files modified:** 5 (1 production, 2 test, AGENTS.md, 1 docs page)
- **Commits:** 2 (`1a75f96`, `49daf3d`)

## Accomplishments

- `settingsStartDeepLink` order is now parse, live `getChatMember`, `settingsGate`, one `getChat`. A gated administrator gets `settings_start_owner_only` with no keyboard and costs no `getChat`; an undecided gate gets `settings_check_failed`. The Owner flag of plan 10-06 and everything after a passing `getChat` are unchanged.
- The three-phrase `chat_status.IsBotMissingError` mapping is gone. `isTelegramClientError` decides at both lookups, and the two `settings_cannot_open` answers of the deep link are gone (the in-menu press path, which plan 10-11 does not touch, still uses its own cannot-open edit).
- `TestSettingsStartDeepLink` rows were rewritten to the class rule and extended (unrecognised 400 and 403, 429 and a transport error at each step, the gated administrator, the missing switch row, the unreadable owner list, an unreadable switch, the owner, an allowed administrator, an ownerless group). `TestSettingsStartDeepLinkSameAnswer` compares text and reply markup of six non-admin outcomes with the plain member's.
- `TestSettingsGateAllDoors` runs five fixtures (administrator and owner with an owner and the switch off, switch on, no owner, owner state unknown) through four door drivers (`settingsGateDoorGroupCommand`, `settingsGateDoorAnonymous`, `settingsGateDoorDeepLink`, `settingsGateDoorPress`) and names the fixture and door that disagree. Plan 10-09 appends the pick door as one more entry of the `doors` slice.
- AGENTS.md: the deep link paragraph under Handlers describes the order and the class rule; "The settings owner gate" bullet under Permissions lists the deep link among the doors and names `TestSettingsGateAllDoors`. The permissions reference gained a "Who can open the menu" subsection under "Settings menu" (marker kept).

## Verification

| Check | Result |
|-------|--------|
| `go test -tags testtools -race -count=1 -run '^(TestSettingsStartDeepLink\|TestSettingsStartDeepLinkSameAnswer\|TestSettingsPrivateRoundTrip\|TestSettingsPrivateMenu)$' ./fuku/modules` | ok (12.8 s) |
| `go test -tags testtools -race -count=1 -run '^TestSettingsGateAllDoors$' -v ./fuku/modules` | `--- PASS: TestSettingsGateAllDoors`, five fixtures pass |
| `go test -tags testtools -race -count=1 -run '^(TestSettings\|TestLoadSettings)' ./fuku/modules ./fuku/i18n` (after both tasks) | ok modules (97.7 s), ok i18n (5.1 s) |
| `CGO_ENABLED=0 go build ./...` | ok |
| `make check-docs` | "No drift detected" |
| `git diff --exit-code 6ef6ad9 -- go.mod go.sum` | clean |
| `grep -c isTelegramClientError fuku/modules/settings.go` | 2 (baseline 0) |
| comment-stripped `grep -c settings_cannot_open fuku/modules/settings.go` | 0 (baseline 2) |
| line order in `settings.go` | `lockdownLiveMember(` 130, `settingsGate(b, chatID` 144, `GetChatWithContext(` 154 |
| `grep -c '"settings_start_owner_only"' fuku/modules/settings.go` | 1 (baseline 0) |
| AGENTS.md greps | `settings_start_owner_only` 1, `TestSettingsGateAllDoors` 1, `TestSettingsStartDeepLinkSameAnswer` 1 |
| permissions.md marker in the first 512 bytes | 1 |

## Task Commits

1. **Task 1 (tracer): the deep link refuses a gated admin and gives every non-admin outcome one answer** - `1a75f96` (feat)
2. **Task 2: every door decides the same, and the rulebook and the permissions reference describe the gate** - `49daf3d` (test)

The tracer feedback gate: the tracer's `<verify>` was re-run end to end (the four test names and the `settings_start_owner_only` grep) before task 2 started and passed, so expansion went ahead.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing critical: rulebook sync] AGENTS.md deep link paragraph rewritten in the task 1 commit**
- **Found during:** Task 1
- **Issue:** AGENTS.md says to update it in the same commit as any change that invalidates a rule. Task 1's code change invalidated the Handlers paragraph (the `IsBotMissingError` mapping and the two cannot-open answers), but the plan puts that edit in task 2.
- **Fix:** The Handlers paragraph was rewritten in the task 1 commit; task 2 added only the Permissions bullet and the docs page.
- **Files modified:** `AGENTS.md`
- **Commit:** `1a75f96`

**2. [Test changes the plan names] Old deep link rows changed their expected answer**
- The rows "the group is gone at getChat" and "a channel" of `TestSettingsStartDeepLink` expected `settings_cannot_open` and now expect `settings_start_not_admin`, as the plan says. Every other earlier deep link and press test (the whole settings set) passed unchanged.

**Total deviations:** 1 auto-fixed (Rule 2), 1 plan-directed test change. **Impact:** none on scope.

## Requirements

`requirements-completed` is empty on purpose. MENU-11 ("in the PM list, through in-group /settings, through the deep link and on every button press") and MENU-13 ("a regular admin does not see the group in their PM list") both include the private list, which plan 10-09 builds, so both stay Pending in `.planning/REQUIREMENTS.md` (checkbox and traceability row). MENU-09, MENU-10, MENU-15 and PLAT-05 are also untouched and stay Pending.

## Known Stubs

None.

## Threat Flags

None. The threat register's T-10-11, T-10-39 and T-10-40 are mitigated as planned; no new endpoint, auth path or schema was added.

## Self-Check: PASSED

- `fuku/modules/settings.go`, `settings_private_test.go`, `settings_gate_test.go`, `AGENTS.md` and `docs/src/content/docs/api-reference/permissions.md` exist and carry the changes above.
- Commits `1a75f96` and `49daf3d` are ancestors of HEAD; `git rev-list --count 6c0881d..HEAD` was 2 at the end of task 2.
- No file was deleted by either commit.
