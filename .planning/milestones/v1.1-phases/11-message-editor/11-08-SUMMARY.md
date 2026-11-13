---
phase: 11-message-editor
plan: 08
subsystem: modules
tags: [settings-menu, message-editor, greetings, rules, pages, tdd]
requires:
  - phase: 11-message-editor
    provides: plan 11-03 LoadSettingsContext and LoadRulesContext, plan 11-05 applyWelcomeEnabled, applyGoodbyeEnabled, applyCleanWelcome and applyCleanGoodbye, plan 11-06 settingsRightChangeInfo and settingsRightRefusalFor
provides:
  - the Welcome (w), Goodbye (g) and Rules (r) settings pages, registered from settings_edit.go's own init
  - one parts table and the pure renderSettingsEditor in row blocks
  - controls e (on/off switch) and d (Delete last) of Welcome and Goodbye, through the commands' apply functions
  - three Main lines and three Main grid entries (s|l / c|f / w|g / r / a)
  - the editor test toolkit settings_edit_env_test.go
affects: [11-09, 11-10, 11-11, 11-12, 11-13, 11-14, settings-message-editor]

actuals:
  tokens: 18700
  tasks: 3
  commits: 4

commits: 4
plan_head_before: f584d1accfa0ddda6aadecfb38d3bee76aaeb8d2
plan_head_after: 9847e16120dd818db76089926b1da44c5fce7610

tech-stack:
  added: []
  patterns:
    - "One page-definition table (settingsEditPageDef) drives the registration, the Main state, the renderer and the controls of three pages, so a page or a part is a table row"
    - "A page's keyboard is built in row blocks (switches, Open in private, navigation) so later plans add a block without touching the others"
    - "A read seam that creates nothing and returns its error (settingsEditGreetingsRead, settingsEditRulesRead) instead of the creating getters"
    - "A BotClient wrapper in the test toolkit (settingsEditListClient) overrides one listed administrator's record in getChatAdministrators"

key-files:
  created:
    - fuku/modules/settings_edit.go
    - fuku/modules/settings_edit_test.go
    - fuku/modules/settings_edit_env_test.go
  modified:
    - fuku/modules/settings_main_test.go
    - fuku/modules/settings_staff_privacy_test.go
    - AGENTS.md
    - .planning/REQUIREMENTS.md

key-decisions:
  - "The three pages share one definition table and one set of functions; Welcome and Goodbye twins differ only by a Kind switch in settingsEditStateOf and settingsEditApplySwitch, so the dupl check has nothing to find."
  - "Control d is registered with settingsRightChangeInfo and control e with settingsRightAdmin, matching /cleanwelcome (CanUserChangeInfo) and /welcome on|off (RequireUserAdmin)."
  - "Rules' Main state is Set or Not set by a non-empty stored text; Welcome and Goodbye show common_status_on or common_status_off."
  - "The authority-parity test lists the administrator in the cached list with the record it wants (can_change_info on or off) through a BotClient wrapper in the toolkit, because the shared staff fake lists every administrator with can_restrict_members only."

requirements-completed: [EDIT-10]

duration: 55min
completed: 2026-11-04
status: complete

coverage:
  - id: D1
    description: "The owner opens Welcome from Main, sees which parts are set, and the on/off switch stores exactly what /welcome off and /welcome on store, with an absolute target value and one answer"
    requirement: EDIT-10
    verification:
      - kind: unit
        ref: "fuku/modules/settings_edit_test.go#TestSettingsEditWelcomeSwitch"
        status: pass
      - kind: unit
        ref: "fuku/modules/settings_edit_test.go#TestSettingsParityGreetingSwitches"
        status: pass
    human_judgment: false
  - id: D2
    description: "Delete last on Welcome and Goodbye stores what /cleanwelcome and /cleangoodbye store; an administrator without can_change_info is refused by menu and command alike, any administrator may flip the switch in both"
    requirement: EDIT-10
    verification:
      - kind: unit
        ref: "fuku/modules/settings_edit_test.go#TestSettingsAuthorityParityGreetings"
        status: pass
      - kind: unit
        ref: "fuku/modules/settings_edit_test.go#TestSettingsEditPages"
        status: pass
    human_judgment: false
  - id: D3
    description: "Pages read through reads that create nothing and fail honestly: no greetings, rules or chats row is created by opening them, a failed read is the cannot-open edit and the unknown Main mark, and a fresh chat shows what the bot then does (Goodbye On, Delete last Off)"
    requirement: EDIT-10
    verification:
      - kind: unit
        ref: "fuku/modules/settings_edit_test.go#TestSettingsEditReadsCreateNothing"
        status: pass
      - kind: unit
        ref: "fuku/modules/settings_edit_test.go#TestSettingsEditReadFailure"
        status: pass
      - kind: unit
        ref: "fuku/modules/settings_edit_test.go#TestSettingsEditDefaultsMatchRuntime"
        status: pass
      - kind: unit
        ref: "fuku/modules/settings_edit_test.go#TestSettingsDriftGreetings"
        status: pass
    human_judgment: false
  - id: D4
    description: "A failed write redraws the page with the save-failed notice and the old state"
    requirement: EDIT-10
    verification:
      - kind: unit
        ref: "fuku/modules/settings_edit_test.go#TestSettingsEditSwitchFailure"
        status: pass
    human_judgment: false
  - id: D5
    description: "Every page button fits 64 bytes at the worst-case IDs, every page fits 3800 UTF-16 units in all 8 languages with 64-rune multibyte titles, the Main grid is s|l / c|f / w|g / r / a, and no page names the Staff Group in a group menu or a private admin menu"
    requirement: EDIT-01
    verification:
      - kind: unit
        ref: "fuku/modules/settings_edit_test.go#TestSettingsEditCallbackBytes"
        status: pass
      - kind: unit
        ref: "fuku/modules/settings_edit_test.go#TestSettingsEditPagesFitEveryLocale"
        status: pass
      - kind: unit
        ref: "fuku/modules/settings_edit_test.go#TestSettingsEditRegistry"
        status: pass
      - kind: unit
        ref: "fuku/modules/settings_staff_privacy_test.go#TestSettingsStaffPrivacy"
        status: pass
    human_judgment: false
---

# Phase 11 Plan 08: The Welcome, Goodbye and Rules editor pages Summary

**The settings menu now has Welcome, Goodbye and Rules pages that show which parts of each message are set (text, media, buttons) without ever echoing the stored content, and the Welcome and Goodbye pages carry the on/off switch and Delete last, stored through the very apply functions the commands call.**

## What changed

- `fuku/modules/settings_edit.go` (new): `settingsEditPageDef` and `settingsEditPages` (Welcome 🎉 order 50, Goodbye 👋 order 60, Rules 📜 order 70), the parts table (`x` text, `m` media, `b` buttons; Rules `x` only), `settingsEditLoad` through the `settingsEditGreetingsRead` and `settingsEditRulesRead` seams (`greetings.LoadSettingsContext`, `rules.LoadRulesContext`: they create nothing and return their errors), `settingsEditStateWord` (Default or Set with `formatting.VisibleLength`, kind words for the seven media types, Not set, Set with a count), the pure `renderSettingsEditor` in row blocks (body lines, switch rows, Open in private row, navigation), `settingsEditMainState`, `settingsEditOpen`, and the switch controls: `e` (`settingsRightAdmin`) and `d` (`settingsRightChangeInfo`) go through `settingsEditApplySwitch`, which calls `applyWelcomeEnabled`, `applyGoodbyeEnabled`, `applyCleanWelcome` or `applyCleanGoodbye`, then redraw from the read-back with no notice (a failed write redraws the freshly loaded page with `common_settings_save_failed`). The init registers all three pages; `settings_pages.go` is untouched.
- `fuku/modules/settings_edit_env_test.go` (new, the toolkit): `newSettingsEditEnv`, `editCleanup`, `settingsEditAdminJSON`, `settingsEditStored`, `settingsEditRulesStored`, `openEditPage`, `openEditPagePrivate`, and `listEditAdmin` with the `settingsEditListClient` wrapper.
- `fuku/modules/settings_edit_test.go` (new): the twelve plan tests (see Verification).
- `fuku/modules/settings_main_test.go`: `settingsMainWorstState` gains `settings_state_set` and `settings_state_not_set`.
- `fuku/modules/settings_staff_privacy_test.go`: the "pure renders in every language" screens gain every editor page in four states, two views and every notice, the Main entries gain the three pages, and the private-admin end-to-end run opens the three pages and presses the Welcome switch (with `editCleanup`).
- `AGENTS.md`: page keys `w`, `g`, `r` and the `Validate` hook in the `settings|v1` paragraph, a Callbacks bullet for the editor pages, `settingsRightChangeInfo` and `settingsRightRefusalFor` under Permissions, the "reads that create nothing, a fresh chat reads Goodbye On" trap, and the editor test families and toolkit under Testing.
- `.planning/REQUIREMENTS.md`: EDIT-10 ticked (checkbox and traceability row).

## Task commits

| Task | Commit | Subject |
|------|--------|---------|
| 1 (tracer) | da81993 | feat(11-08): add the Welcome, Goodbye and Rules editor pages with the welcome and goodbye switch |
| 2 RED | 22b3163 | test(11-08): add failing tests for the editor pages, Delete last and the greetings switch parity |
| 2 GREEN | 8f7845d | feat(11-08): add Delete last and every media kind to the editor pages |
| 3 | 9847e16 | test(11-08): hold the editor pages to the byte cap, every language and the Staff Group privacy rule |

## TDD notes (Task 2)

RED: `settings_edit.go` was reset to the Task 1 version and only the test files were committed (22b3163). With that code the run of the plan's eight Task 2 tests failed on assertions: `TestSettingsEditPages` (the media kind words and the no-caption note for video, document, sticker, audio, voice and video note were missing, and Delete last stored nothing), `TestSettingsParityGreetingSwitches/delete_last_welcome_on` (the menu wrote nothing), and `TestSettingsAuthorityParityGreetings` (control d answered `common_callback_invalid_request` instead of `settings_no_change_info`). `TestSettingsEditReadsCreateNothing`, `TestSettingsEditReadFailure`, `TestSettingsEditSwitchFailure`, `TestSettingsEditDefaultsMatchRuntime` and `TestSettingsDriftGreetings` passed in RED because Task 1's renderer and switch already satisfied them. These are failures on planned assertions, not on build or fixture faults. `gsd_run check tdd-red-evidence` was not run. GREEN (8f7845d): the media kind map and control d made all nine tests pass. No REFACTOR commit. Task 1 is a tracer, committed test and implementation together.

## Verification (real results)

- `go test -tags testtools -race -count=1 -run '^TestSettingsEditWelcomeSwitch$' ./fuku/modules`: ok.
- Task 2 command (eight tests): all `--- PASS`, ok.
- Task 3 command (`TestSettingsEditRegistry|TestSettingsEditCallbackBytes|TestSettingsEditPagesFitEveryLocale|TestSettingsStaffPrivacy|TestSettingsMainPageFitsEveryLocale|TestSettingsNavigation|TestSettingsCallbackBytes`): ok.
- Whole settings set `go test -tags testtools -race -count=1 -run '^(TestSettings|TestLoadSettings)' ./fuku/modules ./fuku/i18n`, run after Task 1, after Task 2 and at the final code state: `ok fuku/modules 133.236s`, `ok fuku/i18n 6.812s` (final run).
- Plan verification line 2, `-run '^(TestGreeting|TestWelcome|TestSetGreeting|TestMemberJoin|TestLeftMember)' ./fuku/modules`: ok.
- `CGO_ENABLED=0 go build ./...`: ok. `go vet -tags testtools ./fuku/modules`: clean. `git diff --exit-code 72a692b -- go.mod go.sum`: exit 0. `gofmt -l` on every file I touched prints nothing.
- `python3 .../lint-local.py 72a692b`: "no dupl, gocyclo or godox findings on lines added since 72a692b" (before the last code commit, and again before this SUMMARY).
- Acceptance greps: `registerSettingsPage(` 1 in settings_edit.go, no commit of this plan touches `settings_pages.go`, `GetGreetingSettings|GetChatRulesInfo` 0, `applyWelcomeEnabled(|applyGoodbyeEnabled(` 2, `settingsRightChangeInfo` 1 in settings_edit.go and 2 in AGENTS.md, `LoadSettingsContext` 1 and `settings_edit.go` 1 in AGENTS.md, `renderSettingsEditor` in the privacy test.
- Not run: `make lint` (the installed golangci-lint is built with Go 1.25 and refuses Go 1.26.0; the local stand-in ran instead), `make test`, `make check-translations`, `make check-docs` (no locale or docs source changed), PostgreSQL targets (nothing here touches the database schema).

## Deviations from Plan

None in behaviour. Notes:

- No earlier test's expectation changed. The plan expected the Main grid tests to pick up the three pages; `TestSettingsMainGrid`'s end-to-end subtest builds its expected shape from the registry, so it needed no edit. The two edits in `settings_main_test.go` (the worst-state candidates) and `settings_staff_privacy_test.go` (new screens) are the ones the plan lists.
- The plan's toolkit list names `settingsEditAdminJSON` for the authority tests. The staff fake lists every administrator with `can_restrict_members` and no `can_change_info`, so the cached command check could never see the right; I added `listEditAdmin` and its `BotClient` wrapper `settingsEditListClient` to `settings_edit_env_test.go` (my own file) rather than touching the shared fake.

## Requirements

- EDIT-10 is COMPLETE and ticked in `.planning/REQUIREMENTS.md` (checkbox and traceability row): the menu has the Welcome and Goodbye switch and Delete last, stored exactly as the four commands store them (parity, drift, authority and failure tests).
- EDIT-01 is left Pending: the pages show which parts are set, but the Set and Remove buttons arrive in plan 11-09, which completes it.

## Known Stubs

None. The pages draw no Set, Remove or Preview rows yet by design; plans 11-09 and 11-12 add those row blocks to `renderSettingsEditor`.

## Threat Flags

None. The pages add no network endpoint, file access or schema change; they reuse the settings press pipeline and the commands' apply functions.

## Self-Check: PASSED

- Created files exist: `fuku/modules/settings_edit.go`, `fuku/modules/settings_edit_test.go`, `fuku/modules/settings_edit_env_test.go`.
- Commits da81993, 22b3163, 8f7845d and 9847e16 are ancestors of HEAD; `git rev-list --count f584d1a..HEAD` printed 4 before this SUMMARY.
