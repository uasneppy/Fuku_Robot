---
phase: 03-staff-audit-and-undo
plan: 05
subsystem: staff-actions
tags: [telegram, staff-history, audit-record, inline-keyboard, i18n, undo-outcome]
status: complete

requires:
  - phase: 03-staff-audit-and-undo
    provides: staff_actions and staff_action_groups audit tables with undo columns, the eight undo reason codes (plans 03-01, 03-02)
  - phase: 03-staff-audit-and-undo
    provides: Recent actions list, offset cursor, staffCallback dispatcher, renderStaffHistory (plan 03-04)
provides:
  - "A detail button per history entry that edits the message into the action's full per-group record in the run summary's format"
  - renderStaffHistoryDetail, moduleStruct.staffHistoryDetail, staffHistoryGate (shared access sequence), parseStaffHistoryOffset
  - staffCardFromRecord, staffResultsFromRecord, staffUndoResultsFromRecord, staffGroupsBySeq (record-to-summary mapping that plan 03-06 reuses)
  - staffReasonText wording for the eight undo reasons
  - staffActDetail ("dt") callback action
  - twelve locale keys in all 7 locale files
affects: [03-06 undo run and summary, 03-07, 03-08, 03-09]

actuals:
  tokens: 26000
  tasks: 2
  commits: 4
plan_head_before: 196c7b43bde5ae3cfefeb160577fd9a83cec0b02
plan_head_after: d05aac28a1e3c0e753d209f8ac496309bbae288c

tech-stack:
  added: []
  patterns:
    - "Shared history access sequence (staffHistoryGate): Staff Group row, then live membership, each refusal answered once, before any read"
    - "Chat-bound record read: a record loaded fresh by ID is refused unless its staff_chat_id is the pressed message's chat"
    - "Overflow fitting that never drops a group: collapse done lines per block, then list what fits and end with a tail sized with the largest possible count"
    - "User text (names, titles, reasons) cut and escaped, spliced in after translation through tokens"

key-files:
  created: []
  modified:
    - alita/modules/staff_history.go
    - alita/modules/staff_history_test.go
    - alita/modules/staff_action_record.go
    - alita/modules/staff_action_summary.go
    - alita/modules/staff.go
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml
    - AGENTS.md

key-decisions:
  - "The detail view is one edited message; overflow ends with an explicit 'N more group(s) not shown' note, never continuation messages"
  - "Entry buttons are labelled with the entry's list number, five per row, above the paging row"
  - "A record ID is parsed as 63-bit: a larger value cannot be a row ID and Postgres would reject it as a driver error, so it is answered as expired instead"
  - "The undo header travels with the first line under it during fitting; an undo with no applied group puts its header in the head so fitting can never cut it"
  - "A nil linked map marks no group as unlinked (pure callers without link data), a non-nil empty map marks all"
  - "The detail header caps the reason at 300 runes: the stored reason may be as long as Telegram allows, and the header must leave room for the group lines"

patterns-established:
  - "Detail items carry the number of groups they stand for (a collapsed done line stands for N), so the left-out count is exact"

requirements-completed: [SETUP-09]

coverage:
  - id: D1
    description: "Each history entry has a detail button; pressing it edits the message into the action's header, issuer, time, tally and one line per linked group in original order, in the run summary's format"
    requirement: "SETUP-09"
    verification:
      - kind: unit
        ref: "alita/modules/staff_history_test.go#TestStaffHistoryDetail"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_history_test.go#TestStaffResultsFromRecord"
        status: pass
    human_judgment: false
  - id: D2
    description: "Back returns to the list at the offset the detail was opened from"
    requirement: "SETUP-09"
    verification:
      - kind: unit
        ref: "alita/modules/staff_history_test.go#TestStaffHistoryDetailBack"
        status: pass
    human_judgment: false
  - id: D3
    description: "A group no longer linked keeps its stored title and is marked; the undo block shows who and when and each applied group's undo result (running groups as hourglass); every reason code except the five plain successes has words"
    requirement: "SETUP-09"
    verification:
      - kind: unit
        ref: "alita/modules/staff_history_test.go#TestStaffHistoryDetailUnlinked"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_history_test.go#TestStaffHistoryDetailUndoOutcome"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_history_test.go#TestStaffReasonText"
        status: pass
    human_judgment: false
  - id: D4
    description: "A run that never finished shows pending groups as running, then as failed interrupted after 30 minutes, without writing; long texts collapse then list with an explicit count and stay under the cap"
    requirement: "SETUP-09"
    verification:
      - kind: unit
        ref: "alita/modules/staff_history_test.go#TestStaffHistoryDetailUnfinished"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_history_test.go#TestStaffHistoryDetailLong"
        status: pass
    human_judgment: false
  - id: D5
    description: "Only a live Staff Group member can open a detail, only for a record of this chat; forged, missing and malformed IDs are refused with one answer and no edit; callback data stays within 64 bytes"
    requirement: "SETUP-09"
    verification:
      - kind: unit
        ref: "alita/modules/staff_history_test.go#TestStaffHistoryDetailAccess"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_history_test.go#TestStaffHistoryDetailCallbackBudget"
        status: pass
    human_judgment: false
---

# Phase 3 Plan 05: Staff Action Detail View Summary

**Each Recent actions entry opens its full record: every linked group's outcome in the run summary's own format, the undo's per-group outcome once there is one, all read from the audit tables with no Telegram call to linked groups.**

## Performance

- **Duration:** about 15 min
- **Completed:** 2026-10-05
- **Tasks:** 2 (both TDD: RED test commit then GREEN feature commit)
- **Files modified:** 13

## Accomplishments

- Per-entry detail buttons on the history list (`a=dt&r=<record id>&o=<offset>`), five per row, labelled with the entry number. Pressing one edits the same message into the record: header (icon, action, target, duration, reason), issuer and UTC time, tally line, then one line per group in the original order via `staffResultLine`. A single Back button returns to the list at the offset it came from.
- Access is chat-bound: a malformed, zero or over-63-bit record ID is answered as expired before any read; then the Staff Group row and live membership are checked (shared `staffHistoryGate`, now also used by the list handler); the record is loaded fresh and refused with the "does not belong to this chat" alert unless `staff_chat_id` equals the pressed message's chat. Every refusal is answered exactly once with no edit.
- Truthful rendering: groups since unlinked keep the stored title plus a "no longer linked" mark; once an undo was claimed an "Undone by name, time UTC" block lists each applied group's undo result and reason (running groups as the hourglass); a record that never finished shows pending groups as running, then as "failed: interrupted by restart" after 30 minutes, display only.
- Length handling: over 3800 UTF-16 units the done lines of each block collapse into a count, then as many lines as fit are listed and the text ends with an exact "N more group(s) are not shown" note, sized so the final text always fits.
- Reason wording: `staffReasonText` now words the eight undo reasons, so no line reads "skipped: " with nothing after it (plan 03-06 reuses this). Twelve locale keys translated in all 7 files.

## Task Commits

1. **Task 1 RED:** `59e91e0` test(03-05): show history entries cannot be opened
2. **Task 1 GREEN:** `a01556c` feat(03-05): open each recent staff action's per-group record
3. **Task 2 RED:** `f97ad0a` test(03-05): cover unlinked groups, undo outcomes, unfinished records and forged IDs in the detail view
4. **Task 2 GREEN:** `d05aac2` feat(03-05): show unlinked groups, undo outcomes and unfinished runs in the staff action detail

The plan-metadata commit with this SUMMARY follows. No refactor commit was needed.

## TDD Gate Compliance

RED and GREEN commits exist for both tasks, in order. RED was verified as intentional: Task 1's RED ran with a temporary, uncommitted stub for `staffResultsFromRecord` so the target tests failed on assertions (no detail buttons, zero results) rather than on a compile error; Task 2's RED needed no stub and failed on `TestStaffReasonText` (eight undo reasons empty), `...Unlinked`, `...UndoOutcome`, `...Unfinished` and `...Long`. The Task 1 RED commit alone does not compile (it references the then-missing `staffResultsFromRecord`), as with earlier plans.

## Files Created/Modified

- `alita/modules/staff_history.go` - detail buttons in `renderStaffHistory`, `renderStaffHistoryDetail`, `fitStaffHistoryDetail`, `staffHistoryDetail` handler, shared `staffHistoryGate` and `parseStaffHistoryOffset`
- `alita/modules/staff_action_record.go` - `staffCardFromRecord`, `staffResultsFromRecord`, `staffUndoResultsFromRecord`, `staffGroupsBySeq`
- `alita/modules/staff_action_summary.go` - `staffReasonText` arms for the eight undo reasons
- `alita/modules/staff.go` - `staffActDetail = "dt"` and its dispatch case
- `alita/modules/staff_history_test.go` - ten new tests plus helper updates
- `locales/*.yml` (7 files) - `staff_history_back_list`, `staff_history_unlinked`, `staff_history_undone_by`, `staff_history_more_groups`, `staff_undo_unbanned`, `staff_undo_unmuted`, `staff_undo_ban_restored`, `staff_undo_restriction_restored`, `staff_undo_skip_not_applied`, `staff_undo_skip_changed_since`, `staff_undo_skip_restriction_ended`, `staff_undo_skip_no_prior_state`
- `AGENTS.md` - Callbacks bullet for the detail view

## Decisions Made

See `key-decisions` above. The two worth repeating: record IDs are parsed as 63-bit so an absurd ID is a clean "expired" instead of a database driver error, and the undo header is kept together with its first line so the fitting can never leave an undo block headerless.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing critical functionality] Cap the reason in the detail header**
- **Found during:** Task 1 (threat T-03-15, oversize detail text)
- **Issue:** `staffActionHeader` escapes the stored reason in full. A reason typed at Telegram's message limit would make the header alone exceed the 3800-unit cap, and the edit would fail.
- **Fix:** `renderStaffHistoryDetail` cuts the reason to 300 runes plus an ellipsis in its own copy of the card (`staffHistoryDetailReasonRunes`). `TestStaffHistoryDetailLong` uses a 5000-character reason to prove the text stays under the cap.
- **Files modified:** alita/modules/staff_history.go
- **Commit:** a01556c

**2. [Rule 1 - Bug prevention] Record ID parsed as 63-bit**
- **Found during:** Task 1 (design of the forged-ID path)
- **Issue:** `strconv.ParseUint(..., 64)` accepts values up to 2^64-1, which `database/sql` rejects for a bigint column. A forged ID would surface as a "check failed" alert and a logged error instead of the expired answer.
- **Fix:** parse with bit size 63; `TestStaffHistoryDetailAccess/bad id` includes 18446744073709551615.
- **Files modified:** alita/modules/staff_history.go
- **Commit:** a01556c

**3. [Plan-adjacent] Existing 03-04 test helpers updated**
- `historyKeyboardOf` failed on any unknown button action, so it learned the detail and Back-to-list buttons, and `TestStaffHistoryCallbackBudget` now expects the extra detail button (4 buttons, not 3). Both edits were part of the Task 1 RED commit.

### Additions beyond the plan text

- `staffUndoResultsFromRecord` and `staffGroupsBySeq` (undo-column mapping and shared seq ordering) live in `staff_action_record.go` next to the other mapping helpers, so plan 03-06 can reuse them. An empty `undo_outcome` maps to pending there, because `staffOutcomeFromName("")` would read it as failed.
- The gate shared by the list and detail handlers was extracted (`staffHistoryGate`); the list handler's behavior is unchanged and its tests pass.

**Total deviations:** 2 auto-fixed (1 missing-critical, 1 bug prevention), 1 helper adjustment. **Impact:** none on scope; both fixes strengthen the plan's own T-03-14 and T-03-15 mitigations.

## Issues Encountered

- `make lint` is unavailable here (golangci-lint is built with go1.25, the module is go1.26.0); `go vet -tags testtools ./alita/modules` is clean instead.
- `gofmt -l alita/modules` lists `greetings_command_test.go`, which this plan does not touch. Left alone as out of scope.

## Verification Results

- `go test -tags testtools -race -count=1 ./alita/modules` - pass (123 s)
- `go test -tags testtools -count=1 -run '^TestStaffLocaleKeys$' ./alita/i18n` - pass
- `make check-translations` - all 7 locales complete (1086 keys)
- `make check-docs` - no drift
- `CGO_ENABLED=0 go build ./...` - pass
- `git diff --exit-code go.mod go.sum` - unchanged
- All acceptance criteria of both tasks re-run and passing: `staffActDetail = "dt"`, `func renderStaffHistoryDetail(`, `func staffCardFromRecord(a *models.StaffAction) *staffActionCard`, `a.StaffChatID != chat.Id`, `staff_history_back_list` and `staff_undo_skip_changed_since` once in each of es fr hi id pt ru, 8 `staff_undo_` references in staff_action_summary.go, `a=dt&r=<record id>` in AGENTS.md, gofmt clean on the touched files.

## Known Stubs

None. The detail view is wired to real audit data end to end.

## Threat Flags

None. The new callback action is covered by the plan's T-03-14 (chat-bound fresh load, tested) and T-03-15 (escaped, capped, collapsed, counted; tested). No new endpoint, auth path, file access or schema change.

## Next Phase Readiness

Plan 03-06 can reuse `staffReasonText` for the undo summary, `staffResultsFromRecord` / `staffUndoResultsFromRecord` / `staffCardFromRecord` for rendering a record, and the detail view already renders an undo's outcome, so a running or finished undo is visible there as soon as it writes the undo columns.

## Self-Check: PASSED

- Modified files exist: staff_history.go, staff_history_test.go, staff_action_record.go, staff_action_summary.go, staff.go, AGENTS.md, 7 locale files (all confirmed by `git status` and the verification greps above).
- Commits found: 59e91e0, a01556c, f97ad0a, d05aac2 (`git log --grep` on `(03-05)`).
