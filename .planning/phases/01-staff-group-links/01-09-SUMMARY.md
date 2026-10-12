---
phase: 01-staff-group-links
plan: 09
subsystem: staff
tags: [telegram, staff-panel, errgroup, pagination, i18n, live-checks]

requires:
  - phase: 01-staff-group-links
    provides: "recheckLink / newStaffOwnerPass (01-07), applyLinkHealth and staffHealthFromBot (01-08), staffUnlinkButton and the unlink flow (01-06), staffAddGroupURL (01-05), chat_status.FetchBotMember / IsUserInChatWithError"
provides:
  - "buildStaffPanelRows: live per-link owner and bot checks, errgroup limit 4, at most 2N+1 Telegram calls, rows in link-id order"
  - "renderStaffPanel(tr, staffGroup, rows, botUsername, page, updatedAt): status icons, one reason line, legend, Updated time, paging at 8, 3800 UTF-16 cap"
  - "Refresh (rf) and Page (pg) callbacks: Staff Group chat plus live membership, answered once, edit in place"
  - "buildStaffPanel(b, tr, staffGroup, page): the one live builder behind /staff, Refresh, paging and the unlink re-render"
affects: [01-10 hourly sweep, Phase 2 staff actions, Phase 4 lockdown status, Phase 9 settings menu]

actuals:
  tokens: 16000
  tasks: 2
  commits: 3

tech-stack:
  added: []
  patterns:
    - "Live panel build: errgroup.Group with SetLimit(4), results written to per-index slots so output order never depends on completion order"
    - "Length cap measured in UTF-16 units on the HTML source: swap the help for a hint, then drop rows with a localized note"
    - "Presser authority for read-only panel buttons is Staff Group chat + live getChatMember, never the admin cache"

key-files:
  created:
    - alita/modules/staff_panel_test.go
    - alita/modules/staff_panel_render_test.go
  modified:
    - alita/modules/staff_panel.go
    - alita/modules/staff.go
    - alita/modules/staff_unlink.go
    - alita/modules/staff_unlink_button_test.go
    - alita/modules/staff_migrate_test.go
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml

key-decisions:
  - "The press is answered before the live checks run, because a panel of many groups can outlast a callback query's validity; a failed rebuild is then logged rather than shown as an alert"
  - "Rows whose live checks give no answer stay in the panel with unknown icons; only OwnerMismatch (via recheckLink) removes a link, matching D-15"
  - "A cancelled build context keeps every row as unknown and makes no Telegram calls, rather than silently dropping rows"
  - "Refresh and Page share one body (staffPanelRebuild) so the two buttons cannot drift apart on authority"
  - "Rows dropped to fit the length cap lose their Unlink button too; the cap only bites with worst-case titles (64 ampersands), so this is a safety net, not a normal path"

patterns-established:
  - "isMessageNotModified (case-insensitive) is the single check for Telegram's no-op edit answer; editStaffMessage now uses it too"
  - "Test-local helpers in this plan carry a panel prefix (panelEnv, panelGateClient, panelRows...) so they compile alongside plan 01-10's tests"

requirements-completed: [SETUP-08, SETUP-06, PLAT-03]

coverage:
  - id: D1
    description: "/staff in a Staff Group lists every linked group in link-id order with live status icons and one reason line for a broken group (bot missing, not admin, cannot restrict, owner could not be checked)"
    requirement: SETUP-08
    verification:
      - kind: unit
        ref: "alita/modules/staff_panel_test.go#TestStaffPanelLiveStatuses"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_panel_test.go#TestStaffPanelLiveStableOrder"
        status: pass
    human_judgment: false
  - id: D2
    description: "An owner mismatch found while building the panel removes that link, posts the owner-changed notice once and leaves the group out; a health change posts one heads-up and a repeat Refresh posts none"
    requirement: SETUP-06
    verification:
      - kind: unit
        ref: "alita/modules/staff_panel_test.go#TestStaffPanelLiveStatuses"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_panel_test.go#TestStaffPanelRefreshInPlace"
        status: pass
    human_judgment: false
  - id: D3
    description: "Refresh edits the same message in place for a live Staff Group member, answers once, treats 'message is not modified' as success, and a non-member or a press outside a Staff Group changes nothing"
    requirement: SETUP-08
    verification:
      - kind: unit
        ref: "alita/modules/staff_panel_test.go#TestStaffPanelRefreshInPlace"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_panel_test.go#TestStaffPanelRefreshNonMember"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_panel_test.go#TestStaffPanelRefreshOutsideStaffGroup"
        status: pass
    human_judgment: false
  - id: D4
    description: "Building the panel makes at most 2N+1 Telegram lookups for N links with at most 4 in flight"
    requirement: PLAT-03
    verification:
      - kind: unit
        ref: "alita/modules/staff_panel_test.go#TestStaffPanelCallBudget"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_panel_test.go#TestStaffPanelConcurrencyLimit"
        status: pass
    human_judgment: false
  - id: D5
    description: "8 links are one page with no paging buttons; 9 give two pages with Next/Prev, Unlink buttons only for the current page, out-of-range pages clamped, paging callback gated on live membership"
    requirement: SETUP-08
    verification:
      - kind: unit
        ref: "alita/modules/staff_panel_render_test.go#TestStaffPanelRenderPaging"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_panel_render_test.go#TestStaffPanelPageCallback"
        status: pass
    human_judgment: false
  - id: D6
    description: "Panel text stays at or under 3800 UTF-16 units for 40 links with 64-rune titles in all 7 locales; titles are cut to 64 runes and HTML-escaped; the empty state shows the chat ID, Add group and Refresh"
    requirement: SETUP-08
    verification:
      - kind: unit
        ref: "alita/modules/staff_panel_render_test.go#TestStaffPanelRenderFitsInEveryLocale"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_panel_render_test.go#TestStaffPanelRenderDropsRowsWithANoteWhenTooLong"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_panel_render_test.go#TestStaffPanelRenderEscapesAndTruncatesTitles"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_panel_render_test.go#TestStaffPanelRenderEmpty"
        status: pass
    human_judgment: false
  - id: D7
    description: "The panel is readable in a real Telegram client: status icons, reason lines, the Updated time changing on Refresh, and the translated wording in all 7 languages"
    verification: []
    human_judgment: true
    rationale: "Visual layout and readability in real Telegram clients, and a native-speaker read of the 14 new strings, cannot be asserted by a test"

duration: 18min
completed: 2026-10-05
status: complete
plan_head_before: 5ce8c9124146793b493673e2fdbd7805b62e8194
plan_head_after: b6b1ed5
commits: 3
---

# Phase 1 Plan 09: Live /staff panel Summary

**/staff now checks every linked group live (owner via recheckLink, bot via FetchBotMember and applyLinkHealth, errgroup limit 4), renders status icons with one reason line per broken group, pages at 8 with a 3800 UTF-16 cap in all 7 locales, and Refresh/paging edit the same message in place for live Staff Group members.**

## Performance

- **Duration:** about 18 min (start time was not captured at spawn; estimated from commit times)
- **Started:** 2026-10-04T23:47:00Z (approximate)
- **Completed:** 2026-10-05T00:05:00Z (approximate)
- **Tasks:** 2
- **Files modified:** 14 (2 created)

## Accomplishments

- `buildStaffPanelRows` runs `recheckLink` and `FetchBotMember` plus `applyLinkHealth` for every link, writes results into per-index slots (so order is link id ascending whatever order the checks finish in), and shares one `newStaffOwnerPass` so N links cost at most 2N+1 lookups, 4 at a time. A link removed by the owner check is left out; a check with no answer keeps its row with an unknown icon.
- `renderStaffPanel` shows bot-is-admin, can-restrict and owner-matches icons, exactly one reason line for a broken row, a localized legend and an "Updated HH:MM:SS UTC" line. It pages at 8 (Prev/Next, one-based page line, Unlink buttons only for rows shown), clamps out-of-range pages, and holds the text to 3800 UTF-16 units by swapping the help for a hint and then dropping rows with a note.
- Refresh (`rf`) and Page (`pg`) are available to any current Staff Group member: the chat must be a Staff Group, membership is checked live with `IsUserInChatWithError`, the press is answered once, and the message is edited in place. "message is not modified" counts as success.
- The unlink re-render and `/staff` use the same live builder, so every panel reflects live status.

## Task Commits

1. **Task 1: tracer, /staff checks every link live and Refresh edits it in place** - `b7033c2` (feat)
2. **Task 2 RED: failing render, cap and page-callback tests** - `926a76f` (test)
3. **Task 2 GREEN: paging at 8, length cap, page callback, locale keys** - `b6b1ed5` (feat)

**Plan metadata:** committed separately (docs: complete plan).

## Tracer gate

Task 1 is `type="tracer"` with an automated-only verify. The run is interactive with `human_verify_mode=end-of-phase`, so after the Task 1 commit the verify (`go test -tags testtools -race -count=1 -run '^TestStaffPanel' ./alita/modules`) was re-run end to end, passed, and execution continued to Task 2 with no checkpoint.

## TDD Gate Compliance (Task 2)

- RED commit `926a76f` precedes the GREEN commit `b6b1ed5`. Target test `TestStaffPanelRenderPaging` failed on assertions for the planned behavior (page 0 had 9 Unlink buttons and no Next button). `TestStaffPanelRenderFitsInEveryLocale` and `TestStaffPanelPageCallback` also failed on assertions.
- `gsd-tools check tdd-red-evidence` returned `RED_EVIDENCE_OK` for `TestStaffPanelRenderPaging`, with one caveat: the verifier only parses TAP and Surefire output, and returned `INVALID_RED zero_tests_discovered` for raw `go test -v` output. I recorded the same failing run translated to TAP (target test `not ok`, `# tests 1 / # fail 1`) and noted the translation inside the record. The classification therefore rests on the Go run, not on a different run.
- Three RED-phase tests already passed because Task 1 had implemented their behavior: `TestStaffPanelRenderEmpty`, `TestStaffPanelRenderEscapesAndTruncatesTitles` (the title helper existed from plan 01-04) and `TestStaffPanelRenderStableOrder`. They are characterization tests for requirements the plan lists under Task 2.
- Non-vacuity checks (each reverted by editing the line back, never by a checkout): making `isMessageNotModified` return false failed `TestStaffPanelRefreshInPlace` on the error-level log assertion; raising the cap in `staffPanelTextFor` to `1<<30` made `TestStaffPanelRenderFitsInEveryLocale` fail for the ampersand-title case in all 7 locales.
- No refactor commit.

## Files Created/Modified

- `alita/modules/staff_panel.go` - live builder, final renderer (paging, cap, statuses), Refresh/Page handlers, `isMessageNotModified`
- `alita/modules/staff.go` - `staffActRefresh` (`rf`) and `staffActPage` (`pg`) constants and dispatch; `/staff` uses the live builder
- `alita/modules/staff_unlink.go` - unlink re-render uses the live builder; `editStaffMessage` uses the case-insensitive not-modified check
- `alita/modules/staff_panel_test.go` - live status, refresh, non-member, outside-Staff-Group, call budget, concurrency limit, stable order, cancelled context
- `alita/modules/staff_panel_render_test.go` - paging, empty, escaping and truncation, stable order, 7-locale fit, row dropping, page callback
- `alita/modules/staff_unlink_button_test.go`, `alita/modules/staff_migrate_test.go` - adjusted for the new behavior (see Deviations)
- `locales/*.yml` (7) - 14 new keys: `staff_panel_row_status`, `staff_panel_reason_bot_missing`, `staff_panel_reason_bot_not_admin`, `staff_panel_reason_bot_cannot_restrict`, `staff_panel_reason_owner_unknown`, `staff_panel_legend`, `staff_panel_updated`, `staff_panel_refresh_button`, `staff_cb_members_only`, `staff_panel_prev`, `staff_panel_next`, `staff_panel_page`, `staff_panel_help_hint`, `staff_panel_truncated`

## Decisions Made

- Answer the callback before the live checks (a large panel can take longer than a callback stays valid). The cost is that a failed rebuild after that point is logged, not shown as an alert.
- `errgroup.Group` with `SetLimit(4)` rather than `errgroup.WithContext`: the per-link goroutines never return an error (an unknown check is a row state, not a failure), so a derived cancel context would never fire. The build still honors a caller context by skipping the remaining checks and keeping their rows as unknown.
- Refresh and Page are two named methods over one shared body, so their authority cannot diverge.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Updated two earlier tests that the new live behavior invalidated**
- **Found during:** Task 1 (full staff regression run)
- **Issue:** `TestStaffUnlinkButtonRendered` counted every callback button as an Unlink button, and the Refresh button is now one too. `TestStaffMigrateTracer` ran `/staff` against links whose chats had no scripted creator, so the live panel (correctly) unlinked them and the reply was not the panel.
- **Fix:** The first test now counts only `a=ul` buttons and passes the new render arguments. The second scripts the links' maker as creator of the re-keyed chats, as a real migrated group would have.
- **Files modified:** `alita/modules/staff_unlink_button_test.go`, `alita/modules/staff_migrate_test.go`
- **Verification:** `go test -tags testtools -race -count=1 -run '^TestStaff|^TestLinkStaff|^TestUnlinkStaff|^TestSetStaff|^TestUnsetStaff' ./alita/modules` passes
- **Committed in:** `b7033c2`

**2. [Rule 3 - Blocking] `buildStaffPanel` signature changed**
- **Found during:** Task 1
- **Issue:** The live builder needs the bot for Telegram calls, and `staffRerenderPanel`, `staffPanel` and the new Refresh handler all call it. The plan's interface sketch showed `staffRerenderPanel(b, msg)`, but the existing function also takes the translator and still does.
- **Fix:** `buildStaffPanel(b, tr, staffGroup, page)`; callers updated.
- **Committed in:** `b7033c2`

---

**Total deviations:** 2 auto-fixed (2 blocking)
**Impact on plan:** Both were direct consequences of making the panel live. No scope change.

## Issues Encountered

- I ran `git checkout -- alita/modules/staff_panel.go` to undo a mutation experiment while the file was still uncommitted, which reverted all of Task 1's work in that file. I rewrote the file from the content I had just written and re-ran the whole suite before committing; nothing was lost, but later mutation checks used `sed` to restore the line instead.
- `gofmt -l` reports `alita/modules/greetings_command_test.go`. It was already unformatted before this plan and is out of scope.

## Known Stubs

None.

## Threat Flags

None. The new surface (Refresh and Page callbacks, one live fan-out per panel) is covered by T-01-28, T-01-29, T-01-30 and T-01-31, and each has a test: `TestStaffPanelRefreshOutsideStaffGroup`, `TestStaffPanelCallBudget` and `TestStaffPanelConcurrencyLimit`, `TestStaffPanelRenderEscapesAndTruncatesTitles`, `TestStaffPanelRefreshNonMember` and `TestStaffPanelPageCallback`.

## Verification Results

- `go test -tags testtools -race -count=1 -timeout 10m ./...` passes
- `go vet -tags testtools ./...` clean; `gofmt -l` clean on every file touched by this plan
- `go test -tags testtools -race -count=1 -run '^TestStaffPanel' ./alita/modules` passes (17 test functions); `-run '^TestStaffPanelRender'` prints 9 PASS lines (parents and subtests)
- `go test -tags testtools -count=1 -run '^TestStaffLocaleKeys$' ./alita/i18n`, `make check-translations` and `make check-docs` pass
- `git diff --exit-code go.mod go.sum` clean (errgroup comes from the existing golang.org/x/sync v0.23.0)
- Acceptance greps: `SetLimit(4)`, `staffActRefresh` in `staff.go`, "message is not modified" in `staff_panel.go`, `staffPanelPageSize = 8`, `staffPanelMaxUTF16 = 3800` all match; no non-literal `GetString(` key and no `IsUserAdmin|LoadAdminCache|RequireUserOwner` in `staff_panel.go`

## Human check outstanding (end-of-phase)

In a real Staff Group with at least two linked groups (demote the bot to member in one), send `/staff`, press Refresh, then promote the bot back and press Refresh again. Expect one message with readable icons and a reason for the broken group; Refresh edits the same message and the Updated time changes; after promoting the bot the group shows healthy and the Staff Group got exactly one "healthy again" heads-up. Have a native speaker skim the non-English wording of the 14 new strings.

## Next Phase Readiness

`buildStaffPanelRows` and `buildStaffPanel` are ready for the hourly sweep (01-10) and Phase 4's in-lockdown status. `requirements-completed` lists SETUP-06 as the plan declares it; `REQUIREMENTS.md` was marked for SETUP-08 and PLAT-03 only, because plan 01-10 also declares SETUP-06 and had no summary yet when this plan finished.

## Self-Check: PASSED

---
*Phase: 01-staff-group-links*
*Completed: 2026-10-05*
