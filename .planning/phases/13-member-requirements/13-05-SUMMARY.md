---
phase: 13-member-requirements
plan: 05
subsystem: settings-menu
tags: [settings-menu, member-requirements, mreq, page-m, switches, harness]
requires:
  - phase: 13-member-requirements
    provides: memberreq.GetSettings / GetSettingsFresh / SetCheckOnJoin / SetDeleteMessages / ActionOf / AnyRuleOn (13-01), the memberReqRules table (13-02), the 61 locale keys (13-04)
provides:
  - page m (Order 80, icon 🛂, settings_page_mreq) registered from fuku/modules/settings_mreq.go, with its Main state line (Off, {n} on, unknown mark)
  - renderSettingsMreqPage (pure), memberReqActionIcon, settingsMreqActionWord
  - memberReqSettingsLookup (the one read seam of the page, Main and later the watcher), memberReqWrites, memberReqApplyResult, applyMemberReqJoin, applyMemberReqDelete
  - controls j (Check on join) and d (Delete messages), tg or st, v 0 or 1, settingsRightRestrict
  - both Phase 13 tables in the module test harness AutoMigrate list and in settingsCleanup
affects: [13-06, 13-07, 13-08, 13-09, 13-10, 13-11, 13-13, member-requirements]

actuals:
  tokens: 11700
  tasks: 3
  commits: 5

commits: 5
plan_head_before: 6cbcbf72356b8af04e989eec06bb599117a5ec0f
plan_head_after: 5cc8f9c7187553ef7432647b05a9da7af16dcf50

tech-stack:
  added: []
  patterns:
    - "A settings page with no command carries drift and authority tests instead of a parity test; its apply functions live beside the domain code (memberreq_settings.go)"
    - "An apply function reads the row back fresh after a failed write too, so the page redraws what is really stored"

key-files:
  created:
    - fuku/modules/settings_mreq.go
    - fuku/modules/memberreq_settings.go
    - fuku/modules/settings_mreq_test.go
  modified:
    - fuku/modules/test_harness_test.go
    - fuku/modules/settings_env_test.go
    - fuku/modules/settings_edit_test.go
    - fuku/modules/settings_staff_privacy_test.go
    - AGENTS.md

key-decisions:
  - "Both switches reuse settingsEditSwitchValue as the control Validate (exactly 0 or 1); it is generic, so no second validator was added"
  - "The apply functions read the row back with memberreq.GetSettingsFresh also after a failed write; a read-back failure leaves Row nil and the caller reloads through the cached loader, a failure there is the cannot-open edit"
  - "The page has no Refresh button in this plan (UI-SPEC lists one on the full page); the Main grid button carries the 37 and 33 byte check"
  - "No bot-rights line and no rule grid: the grid and rule screens are 13-06, the bot-rights lines Phase 14"

patterns-established:
  - "settingsMreqSeams restores the read and write seams; settingsMreqSeed replaces a chat's row directly and drops the cache on every layer"

requirements-completed: []

coverage:
  - id: D1
    description: "Requirements is on Main (r|m grid, the status line) and opens to every rule Off, both switches Off and the five hints in contract order, in a group menu and a private menu, with no row created and no Open in private row"
    requirement: "MREQ-01"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestSettingsMreqOpensFromMain|TestSettingsMreqMainState|TestSettingsStaffPrivacy)$' -v ./fuku/modules (RED at 887e3b7 on assertions, GREEN at 692d65e)"
        status: pass
    human_judgment: false
  - id: D2
    description: "Main reads Off whatever the switches say, {n} on for n = 1..5, and the unknown mark with the cannot-open edit and alert when the read fails"
    requirement: "MREQ-01"
    verification:
      - kind: unit
        ref: "TestSettingsMreqMainState (seven subtests pass)"
        status: pass
    human_judgment: false
  - id: D3
    description: "The two switches write one column each (a Go false written, the other switch and the five actions untouched), redraw from the read-back, drop the member_requirements key, refuse a bad v, control or action before any getChatMember, and redraw the old state with common_settings_save_failed on a failed write"
    requirement: "MREQ-02, MREQ-03"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestSettingsMreqSwitches|TestSettingsDriftMreq|TestSettingsAuthorityMreq)$' -v ./fuku/modules (RED at 32b1ef7 on assertions, GREEN at 58ac07a)"
        status: pass
    human_judgment: false
  - id: D4
    description: "An administrator without can_restrict_members opens the page and every switch press is the settings_no_restrict alert with no edit and no write; a plain member gets settings_not_admin; the owner gate answers settings_gate_owner_only before the right"
    requirement: "MREQ-02"
    verification:
      - kind: unit
        ref: "TestSettingsAuthorityMreq (four subtests pass)"
        status: pass
    human_judgment: false
  - id: D5
    description: "The page fits 3800 UTF-16 units with 64-rune ampersand, astral and multibyte titles in all 8 languages, with no unfilled placeholder and no button omitted; the Main grid button is 37 bytes private and 33 group, a switch press 45 and 41, every field a number or a code"
    requirement: "MREQ-13"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestSettingsMreqPageFitsEveryLocale|TestSettingsMreqCallbackBytes)$' -v ./fuku/modules. Negative proof: with the switch control code temporarily 'jjjj' TestSettingsMreqCallbackBytes failed on 48 and 44 bytes and on the code shape; restored"
        status: pass
    human_judgment: false
  - id: D6
    description: "A reader of a group menu or a private admin menu never sees the Staff Group on the page, rendered in 8 languages with every notice, and in a live private run"
    requirement: "MREQ-01"
    verification:
      - kind: unit
        ref: "TestSettingsStaffPrivacy (page m in the hand-built mainEntries, 2 states x 2 views x all notices x 8 languages, and the page plus a switch press in the private admin run)"
        status: pass
    human_judgment: false

duration: 75min
completed: 2026-11-05
status: complete
---

# Phase 13 Plan 05: The Member requirements page and its switches Summary

**Page `m` (🛂 Requirements, Order 80) on the settings Main: every rule and both switches read through the cached loader the watcher will use, Check on join and Delete messages written one column each under `settingsRightRestrict`, and both Phase 13 tables in the module test harness.**

## Accomplishments

- Task 1 (tracer): tests first (RED), then `settings_mreq.go` and the `memberReqSettingsLookup` seam. Main now draws `s|l / c|f / w|g / r|m` then Access; the page shows the intro, the all-off line, one line per rule with icon and word, the two switch lines and the five hints, a switch row and the navigation row; a failed read is the cannot-open edit, Main shows the unknown mark. No row is created by opening Main or the page.
- Task 2: the controls `j` and `d` (`tg` or `st`, strict 0 or 1, `settingsRightRestrict`), `memberReqWrites`, `applyMemberReqJoin` and `applyMemberReqDelete` with a typed `memberReqApplyResult`. The page redraws from the fresh read-back; a failed write redraws the stored state with the save-failed notice.
- Task 3: the 8-language fit test, the worst-case callback test and the AGENTS.md updates (the `settings|v1` page list, the Main shape, a page bullet after Access, the Testing families).
- The harness `AutoMigrate` list and `settingsCleanup` carry both Phase 13 tables and the `member_requirements` cache key (done in the first RED commit, so every page test opens page m).
- `settings_pages.go` is untouched (`git diff --exit-code 8c0c827 -- fuku/modules/settings_pages.go` exits 0). Nothing here restricts, kicks, bans, warns or deletes, and the kill switch does not exist yet.

## Task Commits

1. Task 1 RED: `887e3b7` test(13-05): add failing tests for the Member requirements page on Main
2. Task 1 GREEN (tracer): `692d65e` feat(13-05): the Member requirements page on Main
3. Task 2 RED: `32b1ef7` test(13-05): add failing tests for the Check on join and Delete messages switches
4. Task 2 GREEN: `58ac07a` feat(13-05): Check on join and Delete messages switches
5. Task 3: `5cc8f9c` test(13-05): the Member requirements page fits every language (tests and AGENTS.md)

RED evidence. Task 1 RED carried compile-only stand-ins (the page constants, an empty `renderSettingsMreqPage`, the real read seam) and failed on assertions: `the owner's group Main = "s|l / c|f / w|g / r / a / close", want "... r|m ..."`, `Main has no 🛂 Requirements button`, and `member requirements ... : the screen has no text` in the privacy test. Task 2 RED carried the real write seam only and failed on assertions: `tg joins on: stored = {... CheckOnJoin:false ...}` and `answered "@@common-callback-invalid-request@@", want the settings_no_restrict alert` (the controls were not registered). Task 3 is a TDD task whose tests pass at once on the code of tasks 1 and 2 (the plan says so: "they should pass on Tasks 1 and 2"), so there is no RED commit for it; the fit and byte tests were instead shown to fail under a deliberate mutation (negative proof in D5). The `gsd_run check tdd-red-evidence` classifier was not run (the plan is `type: execute`).

The tracer gate: the tracer's verify tests were re-run end to end after the expansion tasks (they are part of the green runs below), so nothing was built on a broken slice.

## Verification (real results)

- `go test -tags testtools -race -count=1 -run '^(TestSettingsMreqOpensFromMain|TestSettingsMreqMainState|TestSettingsStaffPrivacy)$' -v ./fuku/modules`: PASS at `692d65e`.
- `go test -tags testtools -race -count=1 -run '^(TestSettingsMreqSwitches|TestSettingsDriftMreq|TestSettingsAuthorityMreq)$' -v ./fuku/modules`: PASS at `58ac07a`.
- `go test -tags testtools -race -count=1 -run '^(TestSettingsMreqPageFitsEveryLocale|TestSettingsMreqCallbackBytes)$' -v ./fuku/modules`: PASS at `5cc8f9c`.
- The settings family at plan end, run once on SQLite in the background to `/tmp/claude-0/-home-user-Fuku-Robot/4ce545b5-6832-57a3-a863-43c972932c7c/scratchpad/13-05-family.txt`: `go test -tags testtools -race -count=1 -run '^(TestSettings|TestLoadSettings)' ./fuku/modules ./fuku/i18n` gave `ok fuku/modules 280.104s`, `ok fuku/i18n 5.404s`, `exit=0`, no `--- FAIL`. Page m is in every registry-driven test (Back and Close on every page, the Main grid, the label lengths).
- `gofmt -l` on every file I touched: empty (the repository has seven older files that gofmt lists; none is mine). `go vet -tags testtools ./fuku/modules`: clean. `CGO_ENABLED=0 go build ./...`: ok. `make check-translations`: "All translations are present!". `git diff --exit-code 8c0c827 -- go.mod go.sum`: exit 0.
- Lint stand-in `python3 .../lint-local.py 8c0c827`: exit 0 ("no dupl, gocyclo or godox findings on lines added since 8c0c827"), run before the last code commit and again before this SUMMARY.
- Acceptance greps: `models.ChatRequirements{}` once and `models.ChatRequirementHold{}` present in `test_harness_test.go`; `r|m / a / close` once in `settings_edit_test.go`; `settings_page_mreq` in `settings_staff_privacy_test.go`; `func applyMemberReqJoin(` and `func applyMemberReqDelete(` present; `settingsRightRestrict` appears twice in `settings_mreq.go`; `grep -c 'settings_mreq.go' AGENTS.md` prints 1.
- Not run: `make lint` (golangci-lint is built with Go 1.25 here and refuses Go 1.26.0), `make test` (not this plan's task), PostgreSQL (nothing in this plan rests on it; the `fuku/modules` harness runs on SQLite here).

## Deviations from Plan

- **The lint stand-in path.** The plan names `.planning/phases/11-message-editor/handoff/lint-local.py`, which does not exist; the scratchpad stand-in from the executor brief was used instead (exit 0).
- **No RED commit for Task 3** (see above): its tests are guards that pass at once, as the plan states; proven by a mutation instead.
- **Changed existing tests**, all as the plan lists: `settings_edit_test.go` (the owner's group Main shape now `s|l / c|f / w|g / r|m / a / close`) and `settings_staff_privacy_test.go` (page `m` in the hand-built `mainEntries`, the page in its pure 8-language renders, and the page plus a switch press in the private admin run). No other existing test needed a changed expectation; the family run is green.
- **No Refresh button on the page**, so "the Main grid button and Refresh are 37 and 33 bytes" is checked on the Main grid button alone (the plan's page draws only the switch row and the navigation row; a Refresh arrives only if a later plan adds one).

No Rule 1 to 3 auto-fixes were needed.

## Known Stubs

None. The page's rule grid is deliberately absent until 13-06 (a planned boundary, not a stub): the switch row and the navigation row are complete, and every button drawn works. No broken-windows ledger entry was needed.

## Threat Flags

None. No endpoint, auth path, file access or schema was added. T-13-12 is mitigated by `TestSettingsAuthorityMreq` (restrict right, gate order), T-13-13 by `TestSettingsMreqSwitches` (strict 0 or 1 and the control check before any lookup), T-13-14 by `TestSettingsStaffPrivacy` with page m (the page reads only `chat_requirements`), T-13-SC by the unchanged `go.mod` and `go.sum`.

## Requirements

`requirements-completed: []`. The plan contributes to MREQ-01, MREQ-02, MREQ-03 and MREQ-13 and completes none, as its coverage note says (MREQ-01 is completed by 13-06, MREQ-02 by 13-11, MREQ-03 by 13-10, MREQ-13 by 13-09). All four stay Pending in REQUIREMENTS.md.

## Notes for the owner

- `settingsRightRestrict` for both switches is UI-SPEC Assumption A4 (one right for the whole page); "Delete messages" could need `can_delete_messages` instead. Recorded for the owner's confirmation in 13-SPIKE.md.
- The `skipLocal` clause for the `member_requirements:*` key is deliberately not added yet (it comes with the watcher), so another replica can show an old value for up to `CACHE_LOCAL_TTL`, like the Captcha and Antiflood pages.

## Self-Check: PASSED

- FOUND: fuku/modules/settings_mreq.go, fuku/modules/memberreq_settings.go, fuku/modules/settings_mreq_test.go.
- FOUND commits (all ancestors of HEAD): 887e3b7, 692d65e, 32b1ef7, 58ac07a, 5cc8f9c.
