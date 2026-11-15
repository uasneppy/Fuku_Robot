---
phase: 12-rules-media-media-under-text-and-greet-once
plan: 06
subsystem: settings-menu
tags: [settings, callback-validation, press-pipeline, step-1, controls]
requires:
  - phase: 12-rules-media-media-under-text-and-greet-once
    provides: 12-04 edits to fuku/modules/settings_edit.go (only a file-ordering dependency)
provides:
  - settingsControl.Validate func(value string) bool (nil keeps today's behaviour)
  - settingsControlFieldsValid, called from the tg and st case of settingsCheckFields in step 1
  - settingsEditSwitchValue, the 0-or-1 Validate of the Welcome and Goodbye e and d controls
affects: [12-10, 12-11, 12-12]

actuals:
  tokens: 2100
  tasks: 2
  commits: 4

commits: 4
plan_head_before: 123fc2585eb1bfe98505c081b61b19f9517c2ec2
plan_head_after: 9ba9082058014dd8b59725254420d99d8021730f

tech-stack:
  added: []
  patterns:
    - "Control-level Validate beside the page-action Validate: a strict, I/O-free check in step 1 of the press pipeline"

key-files:
  created:
    - fuku/modules/settings_control_validate_test.go
  modified:
    - fuku/modules/settings_pages.go
    - fuku/modules/settings_menu.go
    - fuku/modules/settings_edit.go

key-decisions:
  - "The existence check refuses an unregistered page too (runControl refused it after the lookup before): same alert, only earlier"
  - "settingsEditSetSwitch keeps its own 0-or-1 guard as a second line; AGENTS.md is untouched because no rule it states became false (plan 12-10 documents the control Validate with the new controls)"

patterns-established:
  - "A new control that takes a value declares Validate; a control without it keeps any non-empty v allowed"

requirements-completed: []

coverage:
  - id: D1
    description: "A tg or st press naming an unregistered page, or a control the page does not have, is answered once with common_callback_invalid_request in step 1: no getChatMember, no getChat, no edit, no write, in a group menu and a private menu"
    requirement: "EDIT-07"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestSettingsControlValidate$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D2
    description: "The Welcome and Goodbye switches e and d accept only v 0 or 1 before any lookup; every other page's controls still run"
    requirement: "EDIT-09"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestSettingsControlValidate|TestSettingsEditWelcomeSwitch|TestSettingsParityGreetingSwitches|TestSettingsAuthorityParityGreetings)$' ./fuku/modules"
        status: pass
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestSettingsCaptchaToggle|TestSettingsCaptchaTurnstile|TestSettingsAntifloodStepper|TestSettingsAntifloodFirstWriteNotice|TestSettingsLockdownTriggers|TestSettingsAccessOwnerFlow|TestSettingsAccessForgedPresses|TestSettingsParityCaptcha|TestSettingsParityCaptchaTime|TestSettingsParityAntiflood|TestSettingsParityLockdownTriggers)$' ./fuku/modules"
        status: pass
    human_judgment: false

duration: 20min
completed: 2026-11-05
status: complete
---

# Phase 12 Plan 06: Strict control validation Summary

**A forged settings switch press (an unknown page, a control the page does not have, or a value that is not 0 or 1) is now refused in step 1, before the bot makes any Telegram call, which the new Media under text and Greet once switches of later plans rely on.**

## What changed

- `settingsControl.Validate func(value string) bool` in `settings_pages.go`, the control-level twin of `settingsPageAction.Validate` (nil keeps the old behaviour).
- `settingsControlFieldsValid(fields)` in `settings_menu.go`: false when `settingsPageByKey(p)` finds no page, when the page has no control `t`, or when the control's `Validate` rejects `v`. It is called from the `settingsActToggle, settingsActSet` case of `settingsCheckFields` once the allow-list is built, so `settingsCheckFields` gained one `if` and no new branch of its own logic.
- `settingsEditSwitchValue` in `settings_edit.go` (`"0"` or `"1"`), set as `Validate` on the Welcome and Goodbye `e` and `d` controls.
- `TestSettingsControlValidate` (`settings_control_validate_test.go`): for `tg` and `st`, in a group menu and a private menu, ten forged presses (unknown control `z`, `e` and `d` on Rules, unregistered page `q`, `t` missing, an extra key, `v` of 2, -1, +1 and empty) must be answered once with the alert `common_callback_invalid_request` with no `getChatMember`, no `getChat`, no edit and no greetings row written; a valid `p=w t=e v=0` press still stores welcome off.

## TDD record

- **Task 1 (tracer) RED** `33d806c`: the test with only the unknown-control cases. 16 subtests failed (four forged presses x `tg`/`st` x two menus), each on an assertion (`the refused press made 1 getChatMember calls`); the `t missing` and `extra key` cases passed because step 1 already refuses them (regression guards). No stub was needed: the test uses only existing symbols. Semantic assessment: the target ran and failed on the planned behavior (a lookup happens before the refusal), not on setup. GREEN `477e752`.
- **Task 2 RED** `a177d59`: the value cases added. 12 subtests failed on `made 1 getChatMember calls` and `made 1 getChat calls` (v=2, -1, +1); the empty `v` case already passed (required field). GREEN `9ba9082`.

## Commits

- 33d806c test(12-06): add failing test for refusing a forged settings control before any lookup
- 477e752 feat(12-06): refuse a forged settings control before any lookup
- a177d59 test(12-06): add failing test for the editor switches accepting only 0 or 1
- 9ba9082 feat(12-06): the editor's switches accept only 0 or 1

## Verification (real results)

- `go test -tags testtools -race -count=1 -run '^TestSettingsControlValidate$' ./fuku/modules`: ok.
- Task 2 verify commands (editor switch family; captcha, antiflood, lockdown, access and parity families): ok, all listed tests PASS.
- Settings family `-run '^(TestSettings|TestLoadSettings)' ./fuku/modules ./fuku/i18n`: ok (199 s and 6 s).
- `CGO_ENABLED=0 go build ./...`: ok. `go vet -tags testtools ./fuku/modules`: clean. `gofmt -l` on the touched files: empty.
- `python3 .../lint-local.py 72a692b` (dupl, gocyclo, godox, staticcheck unused, errcheck): exit 0 before the last code commit and again before this summary.
- `git diff --exit-code 72a692b -- go.mod go.sum`: clean. Acceptance greps: `settingsControlFieldsValid(` 2 hits, `Validate func(value string) bool` 1 hit, `Validate: settingsEditSwitchValue` 2 hits.

## Deviations from Plan

None - plan executed exactly as written. The plan's verification line names `lint-local.py bd2f794` under the phase 11 handoff directory; the brief's stand-in with base `72a692b` was used instead.

## Known Stubs

None.

## Not run / notes

- `make test` and `make lint` were not run (sandbox limits; the plan does not ask for `make test`). No PostgreSQL work in this plan.
- EDIT-07 and EDIT-09 are not completed here: they stay Pending (`requirements-completed: []`); the new switches arrive in 12-10 to 12-12. AGENTS.md is unchanged on purpose (plan 12-10 documents the control `Validate` and the existence check).

## Self-Check: PASSED

`settings_control_validate_test.go` exists and the three modified files are changed; commits 33d806c, 477e752, a177d59 and 9ba9082 are ancestors of HEAD.
