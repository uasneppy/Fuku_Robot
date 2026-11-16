---
phase: 18-remaining-settings-pages
plan: 04
subsystem: settings-menu
tags: [telegram, settings-menu, reports, auto-approve, apply-function, golden, rights, docs]
status: complete

requires:
  - phase: 18-remaining-settings-pages
    provides: "plans 18-01 to 18-03 (the Warnings page and the Phase 18 string keys); the one-setting-one-path rule"
provides:
  - "applyReportsEnabled (group branch of /reports, enabled and status together, uncached) and applyAutoApprove (greetingWrites.AutoApprove seam), with typed results the commands map to their unchanged replies"
  - "reports.LoadChatStatusContext, the non-creating, error-returning reports reader"
  - "goldens TestLegacyReportsRequests, TestLegacyReportsPrivateRequests and TestLegacyAutoApproveRequests (whole greetings row of a first write pinned), each committed alone before its extraction"
  - "settingsRightChangeInfoBotAdmin and settingsRightOwnerBotAdmin, bot part judged first (the only framework edit of Phase 18)"
affects: [18-06, 18-10, 18-15]

plan_head_before: 22d64539f8be275658c1eea6526eb39802880495
plan_head_after: fe84b3e7f3159c76c68ea52fb7a5ff29a7f32b41

actuals:
  tokens: 12300   # chars/4 over the added non-planning lines of 22d6453..fe84b3e (49290 chars), tests and docs included
  tasks: 3
  commits: 6      # MEASURED: git rev-list --count 22d6453..HEAD (before this SUMMARY commit)

key-files:
  created:
    - fuku/modules/reports_legacy_request_test.go
    - fuku/modules/autoapprove_legacy_request_test.go
    - fuku/modules/switches_apply_test.go
    - fuku/db/reports/load_status_test.go
  modified:
    - fuku/modules/reports.go
    - fuku/modules/greetings.go
    - fuku/db/reports/repository.go
    - fuku/modules/settings_menu.go
    - fuku/modules/settings_menu_test.go
    - fuku/modules/settings_edit_hooks_test.go
    - AGENTS.md

key-decisions:
  - "The owner right refuses a non-owner with the alert chat_status_owner_button_error (planner deviation note 2): a press is a callback and /anonadmin's own permission responder picks that key for a button."
  - "The bot part of the two composite rights is judged first in settingsRightRefusal through settingsRightBotFirstRefusal; the six older rights keep their order; settingsRightRefusalFor (pure, shared with the typed reply) gains only the two member parts."
  - "applyReportsEnabled reads the status back through reports.LoadChatStatusContext under greetingApplyTimeout and falls back to the requested value when only the read-back fails; it does not reuse applyReadBack, whose bare value cannot tell a failed read from a stored false."

requirements-completed: []

coverage:
  - id: D1
    description: "/reports and /autoapprove send and store exactly what they did before, through apply functions a page can call"
    requirement: "PAGE-02, PAGE-10 (contribution only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^(TestLegacyReportsRequests|TestLegacyReportsPrivateRequests|TestLegacyAutoApproveRequests)$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D2
    description: "The menu refuses exactly what /autoapprove and /anonadmin refuse, bot first"
    requirement: "PAGE-05 (contribution only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^(TestSettingsRights|TestSettingsRightRefusalForParity)$' ./fuku/modules"
        status: pass
    human_judgment: false
---

# Phase 18 Plan 04: /reports and /autoapprove apply functions, the reports reader and the two composite rights Summary

`/reports` (group branch) and `/autoapprove` now write through `applyReportsEnabled` and `applyAutoApprove` behind goldens recorded first, the report status reads without creating rows, and the menu has the two composite rights (`settingsRightChangeInfoBotAdmin`, `settingsRightOwnerBotAdmin`) that judge the bot before the member.

## What changed

- **Goldens first.** `TestLegacyReportsRequests` (15 group cases: on/yes/true/ON, off/no/false, status both ways, a status on a chat with no row, an invalid word, a save failure, a non-admin and a bot that is not an administrator), `TestLegacyReportsPrivateRequests` (the private-chat branch with the `connections.ConnectId` fixture: on, off, both statuses, a save failure; no group row is written) and `TestLegacyAutoApproveRequests` (14 cases including both first writes with the whole greetings row as JSON, the status in Markdown, a failed first write that still leaves the default row, and both refusals). Each was committed alone as a `test(18-04):` commit before its production edit.
- **`/reports`.** `reportsWrites.ChatStatus` (default `reports.SetChatReportStatus`), `reportsApplyResult`, `applyReportsEnabled` and the small `reportsGroupApplyKey` wrapper; only the group branch maps onto it. The private branch, the gates and the status text are untouched. `reports.LoadChatStatusContext` is one plain `SELECT status`, no `EnsureChatInDb`, no create, a missing row reads on, the error returned.
- **`/autoapprove`.** `greetingWrites.AutoApprove` (default `greetings.SetShouldAutoApprove`) and `applyAutoApprove` through `finishGreetingApply`; `autoApproveToggleConfig.setPref` calls it as `cleanWelcomeToggleConfig` calls `applyCleanWelcome`.
- **Rights.** Two values appended after `settingsRightChangeInfo`; `settingsRightRefusalFor` answers their member parts; `settingsRightRefusal` calls `settingsRightBotFirstRefusal` first (`chat_status.IsUserAdmin` on the bot with the notice `connections_is_user_connected_bot_not_admin`, or `chat_status.IsBotAdmin` with `chat_status_bot_not_admin`). `settings_pages.go` and `settings_card.go` are untouched.
- **Tests.** `TestLoadChatStatusContext`, `TestApplyReportsEnabled`, `TestApplyAutoApprove`, the two `TestSettingsRights` subtests ("mirrors /autoapprove" and "mirrors /anonadmin", each running the real command in the same chat over bot role and member right), and 14 new `TestSettingsRightRefusalForParity` rows.
- **AGENTS.md.** The "One setting, one code path" bullet gains the reports and auto-approve paragraphs; the Permissions bullet gains the two rights and the one framework edit.

## Verification (real results)

- `go test -tags testtools -race -count=1 -run '^(TestLegacyReportsRequests|TestLegacyReportsPrivateRequests|TestApplyReportsEnabled)$' -v ./fuku/modules`: PASS (all subtests). `... ./fuku/db/reports -run '^(TestLoadChatStatusContext|TestReportSettingsCRUD)$'`: PASS.
- `-run '^(TestLegacyAutoApproveRequests|TestApplyAutoApprove|TestLegacyGreetingsRequestsUnchanged|TestAutoApproveJoinRequestSkipsAdminNotice)$'`: PASS.
- `-run '^(TestSettingsRights|TestSettingsRightRefusalForParity|TestSettingsGateAllDoors|TestSettingsAuthorityParityOwnerGate|TestSettingsRightChangeInfo)$'`: PASS (`ok ... 17.663s`).
- Whole `fuku/modules` package, `go test -tags testtools -race -count=1 -timeout 30m -v ./fuku/modules` (background, log in the scratchpad `modules-18-04.log`): `ok github.com/uasneppy/fuku_robot/fuku/modules 1566.602s`, `exit 0`, `grep -cE -- '--- (FAIL|SKIP)|^FAIL'` = 0, 1683 top-level PASS lines, 1694 passing `TestSettings*` lines. This covers the settings family (`TestSettings|TestLoadSettings`) inside it; `./fuku/i18n` run separately: `ok 24.087s`.
- Acceptance greps: `TestLegacyReportsRequests` 1, `TestApplyReportsEnabled` 1, `TestLoadChatStatusContext` 1, `applyReportsEnabled` 1, `LoadChatStatusContext` 1, `TestLegacyAutoApproveRequests` 1, `TestApplyAutoApprove` 1, `func applyAutoApprove(` 1 and `applyAutoApprove(` 2, "mirrors /autoapprove" 1, "mirrors /anonadmin" 1, the two rights in `settings_menu.go` 8, `chat_status_owner_button_error` 2. `git log --reverse` order gates: the first commit touching each golden file or its command file starts with `test(18-04):`.
- `git diff --exit-code 29aca33 -- fuku/modules/settings_pages.go fuku/modules/settings_card.go`: exit 0. `git diff --exit-code 8c0c827 -- go.mod go.sum`: exit 0.
- `go vet -tags testtools ./fuku/modules ./fuku/db/reports` clean; `CGO_ENABLED=0 go build ./...` ok; `gofmt -l` on every file this plan touched prints nothing; `python3 .../lint-local.py 8c0c827` exits 0 ("no dupl, gocyclo or godox findings"), read directly.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] The first /reports refactor commit did not wire the command**
- **Found during:** Task 3, the lint stand-in ("func reportsGroupApplyKey is unused").
- **Issue:** the scripted edit of the two group branches of `reports()` did not match the file's indentation, so commit `a26564a` added `applyReportsEnabled` and the reader but left the command calling `reports.SetChatReportStatus` directly. The goldens passed either way, which is why only the lint stand-in noticed.
- **Fix:** commit `fe84b3e` replaces both group branches with `reportsGroupApplyKey`; the goldens pass unchanged on the wired command. The same commit adds error checks to the `connections.ConnectId`/`DisconnectId` fixture of the golden file (errcheck); no expectation changed.
- **Files modified:** fuku/modules/reports.go, fuku/modules/reports_legacy_request_test.go

### Notes

- The plan's filter on `TestLegacyReportsRequests` named one test; the private-chat branch lives in its own `TestLegacyReportsPrivateRequests` (same file, same commit) so its connection fixture is isolated.
- The reader's "no cache call" claim is structural (the table is uncached and the reader issues one plain `SELECT`); `TestLoadChatStatusContext` asserts what it can observe: no `chats` or report row created, stored on/off, and an error returned for a cancelled context.
- Disk space: the sandbox volume ran out mid-plan (link step "No space left on device"). I freed it only by deleting stale test binaries, coverage files and old logs from this session's scratchpad directory; a broader sweep of `/tmp` was refused by the permission system and was not attempted again. Remaining free space was about 300 MB at the end.

## Known Stubs

None. The two rights and the two apply functions have no production caller until 18-06 and 18-10, as the plan states; each is exercised by tests of this plan.

## Threat Flags

None. T-18-11 and T-18-12 are mitigated by the two rights and their mirror tests; T-18-13 by the goldens recorded first; T-18-14 by `TestLoadChatStatusContext`.

## Requirements

PAGE-02, PAGE-05 and PAGE-10 are contributed to and **left pending**; `requirements-completed: []`, `requirements.mark-complete` not run (plan 18-15 completes them).

## Self-Check: PASSED

- Files: reports_legacy_request_test.go, autoapprove_legacy_request_test.go, switches_apply_test.go, load_status_test.go, reports.go, greetings.go, repository.go, settings_menu.go all present.
- Commits fbf6241, a26564a, 0ab2728, 6588203, 11971ae, fe84b3e are ancestors of HEAD.
