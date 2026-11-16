---
phase: 18-remaining-settings-pages
plan: 06
subsystem: settings-menu
tags: [telegram, settings-menu, switches, reports, aispam, auto-approve, toggle-helper, docs]
status: complete

requires:
  - phase: 18-remaining-settings-pages
    provides: "plans 18-02 (the Phase 18 string keys), 18-04 (applyReportsEnabled, applyAutoApprove, reports.LoadChatStatusContext, the two composite rights) and 18-05 (aispamConfigured, applyAISpamEnabled, aispam.LoadEnabledContext, the AI test helpers)"
provides:
  - "The Switches page (t, Order 86, icon 🔘): Reports, AI spam and Auto-approve toggles, each tg-only with v exactly 0 or 1, writing through the commands' apply functions behind the commands' rights"
  - "settingsToggleSet, the one generic boolean press of the phase (read before, apply with the absolute target, redraw from the read-back or the stored state with the warn notice), reused by 18-09, 18-10 and 18-12"
  - "The AI status line and Off-only form: the data notice while the filter can run, a code-side stop sign and settings_sw_ai_unavailable while it cannot, no AI toggle when unavailable and off, an alert plus Off-only redraw for a stale On press"
  - "Main reads Switches as {n} on or Off, and the Main-shape constants are s|l / c|f / n|w / g|r / m|d / t / a / close, 8 owner rows"
affects: [18-07, 18-09, 18-10, 18-12, 18-13, 18-15]

plan_head_before: 18e9e92f46c1952c7dc40246549e23fd8717842d
plan_head_after: 0bb736cae3a45db80e023cf7b9e7c2f345c33cf3

actuals:
  tokens: 19600   # chars/4 over the added non-planning lines of 18e9e92..0bb736c (78378 chars), tests and docs included
  tasks: 3
  commits: 4   # MEASURED: git rev-list --count 18e9e92..HEAD (before the SUMMARY and state commits)

key-files:
  created:
    - fuku/modules/settings_switches.go
    - fuku/modules/settings_toggle.go
    - fuku/modules/settings_switches_test.go
    - fuku/modules/settings_switches_screens_test.go
  modified:
    - fuku/modules/settings_main_test.go
    - AGENTS.md
    - README.md
    - docs/src/content/docs/api-reference/callbacks.md
    - docs/src/content/docs/api-reference/permissions.md
    - .planning/phases/18-remaining-settings-pages/18-SPIKE.md

key-decisions:
  - "A refused AI On press follows UI-SPEC check C18-11 and the Phase 15 settingsMreqAIGate precedent (deviation note 1): the alert is the press's one answer, nothing is written, and the page is redrawn from the stored rows (the Off-only form); the mock's 'nothing edited' is superseded because the redraw repairs a stale screen."
  - "settingsToggleSet is generic over the page's state type and takes read, apply and draw as functions, so it knows no page. The apply step reports the state to draw (Stored), a notice key for a refusal or failed save, or neither (the helper reads the page again). The AI gate is inside the AI control's apply step, so the alert and the redraw use the same sequence as every other press."
  - "settingsSwState carries AIAvailable (aispamConfigured() read with the three switches), so renderSettingsSwitches stays pure and a test renders every combination without touching config."

patterns-established:
  - "A boolean control is settingsToggleSet plus one small apply adapter around the command's apply function; the page's own control table adds the right, tg-only Actions and settingsEditSwitchValue."

requirements-completed: []

coverage:
  - id: D1
    description: "Reports, the AI spam filter and Auto-approve of join requests are switched from one page through the commands' own apply functions, leaving the same row and cache"
    requirement: "PAGE-02, PAGE-10 (contribution only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^(TestSettingsParityReports|TestSettingsParityAISpam|TestSettingsParityAutoApprove|TestSettingsDriftReports|TestSettingsDriftAISpam|TestSettingsDriftAutoApprove)$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D2
    description: "The menu refuses what /reports, /aispam and /autoapprove refuse (bot first for Auto-approve), forged presses are refused before any lookup"
    requirement: "PAGE-10 (contribution only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^(TestSettingsAuthorityParitySwitches|TestSettingsSwitchesForged)$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D3
    description: "The AI line states availability in words with no cause, the Off-only form and the stale On gate, in a group and a private menu"
    requirement: "PAGE-02 (contribution only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^(TestSettingsSwitchesAIStatus|TestSettingsSwitchesAIGate)$' ./fuku/modules"
        status: pass
    human_judgment: true
  - id: D4
    description: "The page fits 64-byte buttons and 3800 UTF-16 units in 8 languages, and reads create no row"
    requirement: "PLAT-05 (contribution only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^(TestSettingsSwitchesCallbackBytes|TestSettingsSwitchesPageFitsEveryLocale|TestSettingsSwitchesReadsCreateNothing)$' ./fuku/modules"
        status: pass
    human_judgment: false
---

# Phase 18 Plan 06: The Switches page Summary

The Switches page turns Reports, the AI spam filter and Auto-approve of join requests on or off through the apply functions `/reports`, `/aispam` and `/autoapprove` call, behind the rights those commands need, states in words when the AI filter cannot run on this bot, and brings the shared `settingsToggleSet` that the later toggle pages reuse.

## What changed

- **Tracer (commit 46ace48).** `settings_toggle.go`: the generic `settingsToggleSet[S]` (read before, apply with the absolute target, redraw from the read-back, from the stored state with a warn notice on a refusal or failed save, or from a second read when the write succeeded and its read-back failed; an unreadable page is the cannot-open edit). `settings_switches.go`: page `t` (`Order` 86, icon 🔘, no `Hidden`), the three read seams (`settingsSwReportsRead`, `settingsSwAIRead`, `settingsSwApproveRead`), `settingsSwState`, `settingsSwMainState` (`{n} on` or `settings_state_off_plain`, unknown on any read error), the pure `renderSettingsSwitches`, `settingsSwControl` and the Reports apply adapter with `settingsRightAdminFloodBot`. The Main-shape constants in `settings_main_test.go` become `... / m|d / t / a / close` (private `... / groups|close`) and 8 owner rows. AGENTS.md: the Callbacks bullet lists page `t`, a Switches bullet, the Main order sentence, and `settingsToggleSet` in the "One setting, one code path" bullet.
- **AI spam and Auto-approve (commit 55a82e2).** Controls `a` (`settingsRightAdmin`, `/aispam` has no bot check) and `j` (`settingsRightChangeInfoBotAdmin`), the AI block (`aispam_status_enabled` or `settings_sw_ai_off`, then the existing data notice, or a code-side `🚫 ` and `settings_sw_ai_unavailable` while `aispamConfigured()` is false), the AI toggle drawn only when available or stored on (`{1,1,1,2}` or `{1,1,2}`), the gate for a stale On press inside the AI apply step, and the Auto-approve adapter. AGENTS.md: the Switches bullet is completed and the Permissions bullet names the three rights.
- **Docs (commit 920d26a).** README sentence, `callbacks.md` (page `t`, the three codes, `v`, largest button 45/41), `permissions.md` (three rows), `18-SPIKE.md` checks 4 to 6 (the AI line in each configuration and a stale On press; Auto-approve's first press on PostgreSQL; reports with `/report`).
- **Proofs (commit 0bb736c).** Drift, authority parity, forged, bytes, fit and reads-create-nothing tests (see Verification).

## Verification (real results)

- Task 1 verify: `go test -tags testtools -race -count=1 -run '^(TestSettingsSwitchesOpens|TestSettingsParityReports|TestLegacyReportsRequests|TestSettingsMainGrid|TestSettingsDeletionOpensFromMain|TestSettingsDeletionRowBudget|TestSettingsEditRegistry|TestSettingsMreqOpensFromMain|TestSettingsControlActionAllowList)$' -v ./fuku/modules`: nine `--- PASS`, `ok ... 10.091s`.
- Task 2 verify: `-run '^(TestSettingsSwitchesAIStatus|TestSettingsSwitchesAIGate|TestSettingsParityAISpam|TestSettingsParityAutoApprove|TestSettingsSwitchesOpens|TestLegacyAISpamRequests|TestLegacyAutoApproveRequests|TestSettingsMainGrid)$'`: all `--- PASS`, `ok ... 14.827s`; `make check-docs`: "No drift detected".
- Task 3 verify: `-run '^(TestSettingsDriftReports|TestSettingsDriftAISpam|TestSettingsDriftAutoApprove|TestSettingsAuthorityParitySwitches|TestSettingsSwitchesForged|TestSettingsSwitchesCallbackBytes|TestSettingsSwitchesPageFitsEveryLocale|TestSettingsSwitchesReadsCreateNothing)$' -v`: eight `--- PASS`, `ok ... 15.465s`. The largest button is 45 bytes (private) and 41 (group), as the plan says; the data-notice page fits 3800 UTF-16 units in all 8 languages with 64-rune ampersand, astral and multibyte titles.
- A mutation check: changing the Auto-approve control's right to `settingsRightAdmin` made `TestSettingsAuthorityParitySwitches` fail; restored (no diff left).
- Whole `fuku/modules` package, `go test -tags testtools -race -count=1 -timeout 30m -v ./fuku/modules` (background, log in the scratchpad `modules-18-06.log`): `ok github.com/uasneppy/fuku_robot/fuku/modules 1592.640s`, `exit 0`, `grep -cE -- '--- (FAIL|SKIP)|^FAIL'` = 0, 1700 top-level PASS lines, 288 of them `TestSettings*` (the settings family runs inside it); `./fuku/i18n` `-run '^(TestSettings|TestLoadSettings)'`: `ok 7.571s`; `make check-translations`: all translations present.
- Acceptance greps: the eleven counts of the three tasks print `1` (and `3` for `aispamConfigured()`, `1` for `settingsRightChangeInfoBotAdmin` and for `/autoapprove` in `permissions.md`), as required.
- `go vet -tags testtools ./fuku/modules` clean; `CGO_ENABLED=0 go build ./...` ok; `gofmt -l` on the five Go files prints nothing; `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827` exit 0 ("no dupl, gocyclo or godox findings"), read directly; `git diff --exit-code 8c0c827 -- go.mod go.sum` exit 0; `git diff --exit-code 29aca33 -- fuku/modules/settings_pages.go migrations fuku/db/models main.go` exit 0.
- Not run: `make lint` (the installed golangci-lint is built with Go 1.25, a documented sandbox fact), gosec, and the live checks of `18-SPIKE.md` 4 to 6 (owner steps).

## Deviations from Plan

### Auto-fixed Issues

None needing a production fix. Notes:

1. **[Test fixture] Auto-approve tests give the administrator `can_change_info`.** The settings environment's administrator holds no `can_change_info`, which `settingsRightChangeInfoBotAdmin` and `/autoapprove` both need, so the parity, drift and gate tests mark `env.admin` with `listEditAdmin(env, chat, env.admin, true)`; the without-it case is its own test.
2. **[Finding, C-09] The Welcome and Goodbye pages draw the same before and after Auto-approve's first press only for a chat the bot knows.** For a chat missing from the `chats` table the Goodbye page reads Off before any first greetings write and On after it, because the first write creates the whole greetings row with the database default `goodbye_enabled` true; `/autoapprove on` and `/welcome on` do the same. No served group is in that state (every one has a `chats` row), so no first-write notice was added (the plan asks for one only if the pages differ for a real chat); the test fixture registers its chats with `chats.EnsureChatInDb`, and the finding is recorded in AGENTS.md and in `18-SPIKE.md` check 5 for the PostgreSQL run.
3. **Refused AI On press (deviation note 1 of the plan), applied as the plan recommended:** the alert plus the Off-only redraw, nothing written.

## Known Stubs

None. Every control writes through its command's apply function and every reader reads the real table.

## Threat Flags

None. T-18-18 is mitigated by the gate before any write, the Off-only form and `applyAISpamEnabled`'s own refusal (`TestSettingsSwitchesAIGate`); T-18-19 by `settingsRightChangeInfoBotAdmin` with the bot part first (`TestSettingsAuthorityParitySwitches`, `TestSettingsParityAutoApprove`); T-18-20 because the line names no cause, key or provider (`TestSettingsSwitchesAIStatus` asserts the four cause keys never appear; 18-13 adds the privacy sweep); T-18-21 by `Actions` and `Validate` in step 1 (`TestSettingsSwitchesForged`); T-18-SC no package installed.

## Requirements

PAGE-02, PAGE-10 and PLAT-05 are contributed to and **left pending**; `requirements-completed: []`, `requirements.mark-complete` not run (plan 18-15 completes them).

## Self-Check: PASSED

- Files: settings_switches.go, settings_toggle.go, settings_switches_test.go and settings_switches_screens_test.go present.
- Commits 46ace48, 55a82e2, 920d26a and 0bb736c are ancestors of HEAD.
