---
phase: 11-message-editor
plan: 06
subsystem: modules
tags: [settings-menu, press-pipeline, rights, message-editor, tdd]
requires:
  - phase: 11-message-editor
    provides: plan 11-01 (the settings_no_change_info locale key and its limits row)
provides:
  - settingsPageAction.Validate (strict pre-I/O check of a page action's own fields)
  - settingsRightChangeInfo (creator or administrator with can_change_info, judged on the press's live member record)
  - settingsRightRefusalFor (the one pure judgement of the member-record part of every right)
affects: [11-08, 11-09, 11-11, 11-13, settings-message-editor]

actuals:
  tokens: 9000
  tasks: 2
  commits: 3

commits: 3
plan_head_before: 9849dbd90856d23944c5218f62899476ea665289
plan_head_after: 4be989283edc6ee7babb1e7e5cf8e125762dd390

tech-stack:
  added: []
  patterns:
    - "A page action may carry Validate, run in settingsCheckFields after the allow-list is known and before any I/O"
    - "The member part of a right is one pure function shared by the button press and, later, the typed-reply door"

key-files:
  created:
    - fuku/modules/settings_edit_hooks_test.go
  modified:
    - fuku/modules/settings_pages.go
    - fuku/modules/settings_menu.go

key-decisions:
  - "settingsRightChangeInfo is appended after settingsRightOwner, so the zero settingsRight stays settingsRightAdmin and every existing control keeps its right."
  - "settingsRightRefusalFor answers no refusal for the two bot rights; settingsRightRefusal keeps their cached-predicate bot checks after the member part, as before."
  - "AGENTS.md is not edited here (the hooks add a capability, invalidate no rule); plan 11-08, the first user, documents Validate and the right."

requirements-completed: []

duration: 20min
completed: 2026-11-04
status: complete

coverage:
  - id: D1
    description: "A page action's Validate refuses a forged field set (missing part, unknown part, non-ASCII part, extra key) as common_callback_invalid_request before any getChatMember, runs and edits nothing; an action with no Validate behaves as before"
    requirement: EDIT-01
    verification:
      - kind: unit
        ref: "fuku/modules/settings_edit_hooks_test.go#TestSettingsPageActionValidate"
        status: pass
  - id: D2
    description: "settingsRightChangeInfo: creator and administrator with can_change_info pass, administrator without it gets the alert settings_no_change_info with nothing edited or run, any other status fails closed"
    requirement: EDIT-11
    verification:
      - kind: unit
        ref: "fuku/modules/settings_edit_hooks_test.go#TestSettingsRightChangeInfo"
        status: pass
  - id: D3
    description: "Every existing right answers exactly as before and settingsRightRefusal agrees with settingsRightRefusalFor"
    requirement: EDIT-11
    verification:
      - kind: unit
        ref: "fuku/modules/settings_edit_hooks_test.go#TestSettingsRightRefusalForParity"
        status: pass
      - kind: unit
        ref: "fuku/modules/settings_menu_test.go#TestSettingsRights"
        status: pass
---

# Phase 11 Plan 06: Settings pipeline hooks Summary

**Two additive hooks in the shared settings press pipeline: `settingsPageAction.Validate` refuses a forged editor button before any Telegram call, and `settingsRightChangeInfo` plus the pure `settingsRightRefusalFor` give the "change group info" right one shared judgement.**

## What changed

- `fuku/modules/settings_pages.go`: `settingsPageAction` gains `Validate func(fields map[string]string) bool`; nil keeps the old "allowed, not required" behaviour.
- `fuku/modules/settings_menu.go`: `settingsCheckFields` calls `entry.action.Validate(fields)` in its default branch and refuses on false (step 1, no I/O). `settingsRightChangeInfo` is appended after `settingsRightOwner`. `settingsRightRefusalFor(member, right)` holds the member-record part of Admin, Restrict, Owner and ChangeInfo (the two bot rights give no refusal; an unknown right fails closed). `settingsRightRefusal` calls it first and then keeps the two bot checks.
- `fuku/modules/settings_edit_hooks_test.go`: `TestSettingsPageActionValidate`, `TestSettingsRightRefusalForParity`, `TestSettingsRightChangeInfo`, driven through the real dispatcher with test pages that clean themselves up.

## Task commits

| Task | Commit | Subject |
|------|--------|---------|
| 1 (tracer) | 35bc6c4 | feat(11-06): let a settings page action validate its own fields before any I/O |
| 2 RED | 2885437 | test(11-06): pin the change-info right and the member part of every settings right |
| 2 GREEN | 4be9892 | feat(11-06): add the change-info settings right and judge every right's member part in one place |

## TDD notes

RED evidence (Task 2): with inert stubs for `settingsRightChangeInfo` and `settingsRightRefusalFor` (committed with the tests so the package compiles), `go test -run '^(TestSettingsRightChangeInfo|TestSettingsRightRefusalForParity)$'` failed on assertions: `TestSettingsRightRefusalForParity` (restrict administrator-without-right, member, restricted, left, kicked; owner administrator and member; unknown right) and `TestSettingsRightChangeInfo` (pure judgement, refusal alert, with-right run, creator run). Semantic assessment: both target tests executed and failed on the planned assertions for the intended reason (a stub that grants every right), not on build, fixture or setup faults. The format-based `gsd_run check tdd-red-evidence` classifier was not run (plan is `type: execute`, not `type: tdd`). Task 1 is a tracer and was committed test and implementation together, as the plan describes.

## Verification (real results)

- `go test -tags testtools -race -count=1 -run '^TestSettingsPageActionValidate$' ./fuku/modules`: ok.
- `go test -tags testtools -race -count=1 -run '^(TestSettingsRightChangeInfo|TestSettingsRightRefusalForParity|TestSettingsPageActionValidate|TestSettingsRights)$' ./fuku/modules`: ok.
- Whole settings set `go test -tags testtools -race -count=1 -run '^(TestSettings|TestLoadSettings)' ./fuku/modules ./fuku/i18n`: `ok fuku/modules 120.947s`, `ok fuku/i18n 6.737s` (every earlier press, page, gate and privacy test unchanged and green).
- `CGO_ENABLED=0 go build ./...`: ok. `go vet -tags testtools ./fuku/modules`: clean.
- `git diff --exit-code 72a692b -- go.mod go.sum`: exit 0.
- Acceptance greps: `Validate func(fields map[string]string) bool` 1 in settings_pages.go; `action.Validate` 1 and `settingsRightRefusalFor(p.member` 1 and `settingsRightChangeInfo` 3 in settings_menu.go; `settingsRightAdmin settingsRight = iota` still on one line.
- `gofmt -l` on the three touched files prints nothing (it lists seven other, untouched files that were already unformatted).
- Not run: `make lint` (installed golangci-lint is built with Go 1.25, refuses Go 1.26.0), `make test`, PostgreSQL targets (nothing here needs them).

## Deviations from Plan

None - plan executed as written. The locale key `settings_no_change_info` and its limits row already existed from plan 11-01, so no locale file changed.

## Requirements

EDIT-01 and EDIT-11 are left Pending: this plan only contributes (EDIT-01 completes in 11-09, EDIT-11 in 11-13). `requirements-completed: []`.

## Known Stubs

None.

## Threat Flags

None. Mitigations T-11-16 (Validate before any lookup, asserted by `memberLookupsOf`), T-11-17 (change-info from the press's own live record, fails closed) and T-11-18 (Restrict and Owner mappings moved verbatim, parity test plus the whole suite) are in place; T-11-SC: no package installed.

## Self-Check: PASSED

- FOUND: fuku/modules/settings_edit_hooks_test.go, fuku/modules/settings_pages.go, fuku/modules/settings_menu.go
- FOUND commits 35bc6c4, 2885437, 4be9892 (ancestors of HEAD)
