---
phase: 03-staff-audit-and-undo
plan: 04
subsystem: staff-actions
tags: [telegram, staff-history, inline-keyboard, paging, gorm, i18n]

requires:
  - phase: 03-staff-audit-and-undo
    provides: staff_actions and staff_action_groups audit tables, models.StaffAction, staff.CreateAction / FinalizeAction / SaveGroupResult (plan 03-01)
  - phase: 02-staff-actions-across-groups
    provides: /staff panel, staffCallback dispatcher, editStaffMessage, fitStaffSummaryLines, staffActionIcon / staffActionName
provides:
  - "Recent actions" button on the /staff panel and a paged history view of this Staff Group's staff actions
  - staff.ListActionsFresh, staff.TallyActionGroups, staff.ActionTally
  - staffActRecent ("rc") callback action with an offset cursor (a=rc&o=<offset>)
  - renderStaffHistory / staffHistoryLine / staffRecentButton / moduleStruct.staffHistoryList
  - eight staff_history_* and staff_panel_recent_button locale keys in all 7 locale files
affects: [03-05 entry detail view, 03-06, 03-07, 03-08, 03-09]

actuals:
  tokens: 12000
  tasks: 2
  commits: 4
plan_head_before: 9b3937af33b87918fe91cdc94d1e7f81d3070e32
plan_head_after: cdecc6375922a02f1e71f2f2b289e4ff79ad4731

tech-stack:
  added: []
  patterns:
    - "Offset-cursor paging: Next starts at offset + entries shown, so a page shrunk to the 3800-unit cap hides nothing"
    - "One page query (limit+1 to detect a next page, no COUNT) plus one GROUP BY tally query per history view"
    - "User text cut, whitespace-collapsed and escaped per field; issuer spliced in after translation through staffUserToken"

key-files:
  created:
    - alita/modules/staff_history.go
    - alita/modules/staff_history_test.go
  modified:
    - alita/db/staff/actions.go
    - alita/db/staff/actions_test.go
    - alita/modules/staff.go
    - alita/modules/staff_panel.go
    - alita/modules/staff_panel_render_test.go
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml
    - AGENTS.md

key-decisions:
  - "Paging uses an offset cursor (a=rc&o=<offset>) and Next starts at offset + entries shown, never at a page number"
  - "Field caps per line: names 16 runes, reasons 24 runes, each plus an ellipsis, escaped after cutting"
  - "Running versus interrupted for an unfinished record is decided at render time from updated_at against staffTargetLockTTL (30 min); no reconcile job"
  - "The page query asks for 11 rows to know whether Next is needed, so no COUNT query is made"
  - "The panel's Recent actions button sits right after Refresh; Back from the history reuses the existing Refresh action (a=rf, p=0), so no new rebuild handler exists"

patterns-established:
  - "History view access = the panel's access: GetStaffGroupFresh on the chat of the pressed message, live IsUserInChatWithError, answer once, then read; queries scoped by staff_chat_id"

requirements-completed: [SETUP-09]

coverage:
  - id: D1
    description: "The /staff panel has a Recent actions button; pressing it edits the same message into the history view, and Back returns to the links panel"
    requirement: "SETUP-09"
    verification:
      - kind: unit
        ref: "alita/modules/staff_history_test.go#TestStaffPanelRenderRecentButton"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_history_test.go#TestStaffHistoryList"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_history_test.go#TestStaffHistoryAccess"
        status: pass
    human_judgment: false
  - id: D2
    description: "The history lists this Staff Group's actions newest first, 10 per page, each as one line with icon, action, target name and ID, duration, reason, issuer, UTC date, the done/skipped/failed tally and an undone marker"
    requirement: "SETUP-09"
    verification:
      - kind: unit
        ref: "alita/modules/staff_history_test.go#TestStaffHistoryLine"
        status: pass
      - kind: unit
        ref: "alita/db/staff/actions_test.go#TestStaffActionListFresh"
        status: pass
      - kind: unit
        ref: "alita/db/staff/actions_test.go#TestStaffActionTally"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_history_test.go#TestStaffHistorySameSecond"
        status: pass
    human_judgment: false
  - id: D3
    description: "Prev and Next page with an offset cursor: exactly 10 records show no Next, 11 show Next to a page holding the 11th alone, an empty history shows the empty text with Back, and a page shrunk to the length cap hides no entry"
    requirement: "SETUP-09"
    verification:
      - kind: unit
        ref: "alita/modules/staff_history_test.go#TestStaffHistoryPageBoundary"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_history_test.go#TestStaffHistoryEmpty"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_history_test.go#TestStaffHistoryShrunkPageHidesNothing"
        status: pass
    human_judgment: false
  - id: D4
    description: "Only a live member of this Staff Group can open or page the history, and it shows only that chat's actions; a non-member gets the members-only alert, a chat that is not a Staff Group or a forged offset gets the expired answer, each with no edit"
    requirement: "SETUP-09"
    verification:
      - kind: unit
        ref: "alita/modules/staff_history_test.go#TestStaffHistoryAccess"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_history_test.go#TestStaffHistoryList"
        status: pass
    human_judgment: false
  - id: D5
    description: "The view text never exceeds 3800 UTF-16 units on the escaped HTML with hostile names and reasons, every button's callback data is 1 to 64 bytes, and unfinished records read running (under 30 minutes) or interrupted (after)"
    requirement: "SETUP-09"
    verification:
      - kind: unit
        ref: "alita/modules/staff_history_test.go#TestStaffHistoryLengthCap"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_history_test.go#TestStaffHistoryCallbackBudget"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_history_test.go#TestStaffHistoryUnfinished"
        status: pass
    human_judgment: false
  - id: D6
    description: "The history reads naturally in a real Telegram client in each of the 7 languages (wording of the eight new strings, one-line entries on a phone screen)"
    requirement: "SETUP-09"
    verification: []
    human_judgment: true
    rationale: "Translations were written by the executor and the entry layout was only asserted through marker strings; no native-speaker or real-client review was possible here."

duration: ~25min
completed: 2026-10-05
status: complete
---

# Phase 3 Plan 4: Recent staff actions view Summary

**The /staff panel gets a Recent actions button that edits the same message into a paged, members-only history of this Staff Group's staff actions, newest first, ten per page, each entry one line with who did what to whom, when, why and the done/skipped/failed tally, paged with an offset cursor so a page shrunk to Telegram's length cap hides no entry.**

## Performance

- **Duration:** about 25 min (start time was not recorded, so this is approximate)
- **Completed:** 2026-10-05
- **Tasks:** 2
- **Files modified:** 15 (2 created, 13 modified)

## Accomplishments

- `staff.ListActionsFresh` (WHERE staff_chat_id, ORDER BY id DESC, offset and limit) and `staff.TallyActionGroups` (one GROUP BY query for the whole page, an empty ID list makes no query). Both are uncached fresh reads, like the rest of the audit record, so no `DeleteCache` applies.
- `renderStaffHistory` is pure. Each entry reads `N. 🔨 Ban · Name (<code>123</code>) · 2d · spamming · by Alice · 5 Oct 12:04 · ✅4 ⏭1 ❌1 · ↩ undone`. Names are cut to 16 runes, reasons to 24, then escaped. The text is fitted with `fitStaffSummaryLines`, and Next starts at `offset + shown`.
- `staffHistoryList` follows the panel's access sequence: the chat comes from the pressed message, `GetStaffGroupFresh` must find a Staff Group, the presser must be a live member (`IsUserInChatWithError`), the press is answered once, and only then are rows read. A non-member gets the members-only alert, anything else gets the expired answer, each with no edit. An offset outside 0..1,000,000 is refused as expired.
- A record whose run has not finished reads "running" while its last update is under 30 minutes old and "interrupted, no final result recorded" after that.
- Eight locale keys in all 7 files; `AGENTS.md` records the `a=rc&o=<offset>` callback and the offset cursor.

## Task Commits

1. **Task 1 RED:** `aefb31d` test(03-04): show /staff has no recent staff actions view
2. **Task 1 GREEN:** `9cc525a` feat(03-04): list recent staff actions from the /staff panel
3. **Task 2 tests:** `6b15a37` test(03-04): cover history access, length cap, callback budget and unfinished records
4. **Task 2 docs:** `cdecc63` docs(03-04): record the staff history callback and its offset cursor in AGENTS.md

**Plan metadata:** the `docs(03-04)` commit that holds this file.

## Files Created/Modified

- `alita/db/staff/actions.go` - `ActionTally`, `ListActionsFresh`, `TallyActionGroups`
- `alita/modules/staff_history.go` - constants, `staffHistoryCut`, `staffHistoryDuration`, `staffHistoryState`, `staffHistoryLine`, `renderStaffHistory`, `staffRecentButton`, `staffHistoryList`
- `alita/modules/staff.go` - `staffActRecent = "rc"` and its dispatch case
- `alita/modules/staff_panel.go` - Recent actions row after Refresh; GoDoc keyboard order
- `alita/modules/staff_history_test.go`, `alita/db/staff/actions_test.go`, `alita/modules/staff_panel_render_test.go` - the tests listed in the plan plus `seedStaffActions` and `panelButtons.recent`
- `locales/*.yml` (7 files) - eight new keys each
- `AGENTS.md` - history callback rule in the Callbacks section

## Decisions Made

- The five decisions recorded in the plan stood as written (offset cursor, field caps, compact duration, render-time running/interrupted, limit+1 probe).
- Back from the history reuses the existing Refresh action (`a=rf`, `p=0`), which already rebuilds the links panel live with the same authority, instead of adding a second rebuild path.
- The issuer in an entry is plain escaped text, not a mention link, so reading the history never produces a notification-style link.

## TDD Gate Compliance

- Task 1: RED `aefb31d` (every new test failed on an assertion of the planned behavior: empty line text, no Recent button, no edit on press, zero rows from the repository), GREEN `9cc525a`. No refactor commit.
- Task 2: the plan expected to "record which already pass": all of its tests passed on arrival because Task 1's implementation already enforces members-only, the offset range, escaping and the 3800 cap. There was no failing code to fix, so Task 2 has a `test` commit and a `docs` commit and no second `feat`.
- `gsd_run check tdd-red-evidence` was not run; RED was verified by reading the failing test output.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Signature stubs went into the Task 1 RED commit**
- **Found during:** Task 1 (RED)
- **Issue:** The new tests call `renderStaffHistory`, `staffHistoryLine`, `staffRecentButton`, `ListActionsFresh` and `TallyActionGroups`, so the test packages do not compile without them, and a compile error is INVALID_RED, not a failing assertion.
- **Fix:** `aefb31d` carries stubs that return zero values, so every new test failed on its first behavior assertion. The real bodies came in `9cc525a`.
- **Files modified:** `alita/db/staff/actions.go`, `alita/modules/staff_history.go`
- **Committed in:** `aefb31d`

**2. [Rule 2 - Missing critical functionality] Whitespace is collapsed in cut names and reasons**
- **Found during:** Task 1 (design of `staffHistoryCut`)
- **Issue:** A reason typed over several lines would put a newline inside an entry. That breaks the one-line-per-entry layout and a line could be mistaken for the start of the next entry.
- **Fix:** `staffHistoryCut` joins `strings.Fields` before cutting, so every entry stays on one line.
- **Files modified:** `alita/modules/staff_history.go`
- **Committed in:** `9cc525a`

**3. [Rule 1 - Bug in plan] The hostile-input length test cannot reach the shrink path**
- **Found during:** Task 2
- **Issue:** `TestStaffHistoryLengthCap` as specified (10 records, 64-rune names, 200-rune reasons) fits in the cap, because the plan's own field caps (16 and 24 runes) bound a line to roughly 350 units. The test's "when fewer than 10 lines are shown" branch is never taken, so the cursor logic on a shrunk page was untested.
- **Fix:** The test keeps the plan's assertions (length, escaping, conditional Next offset) and a new `TestStaffHistoryShrunkPageHidesNothing` forces the shrink with a 600-character translation: the page shows fewer than 10 entries, Next offset equals the number shown, and the next page numbers on from it.
- **Files modified:** `alita/modules/staff_history_test.go`
- **Committed in:** `6b15a37`

**4. Task 2's closing commit is `docs`, not `feat`**
- The plan names `feat(03-04): keep the staff history members-only...` for the last commit, but Task 2 needed no production change, and a `feat` holding only an AGENTS.md bullet would put a false entry in the changelog. It is `docs(03-04)`.

---

**Total deviations:** 4 (1 blocking, 1 missing critical, 1 plan inconsistency, 1 commit-type choice)
**Impact on plan:** No scope change.

## Issues Encountered

- A throwaway mutation check (disabling the member check to see `TestStaffHistoryAccess` fail) was refused by the auto-mode classifier as weakening a security check. The edit was reverted at once (`git diff` showed no change to `staff_history.go`) and the check was not pursued by another route. The non-member subtest asserts both the alert and the absence of any edit against the real code.
- The new repository queries ran on SQLite only. They use plain `WHERE`, `ORDER BY id DESC`, `OFFSET/LIMIT` and `GROUP BY ... COUNT(*)`, which behave the same on PostgreSQL; no PostgreSQL server was started here.
- `make lint` is unavailable here (golangci-lint is built with go1.25 and refuses the go1.26.0 module). `gofmt`, `go vet -tags testtools` on the touched packages and `CGO_ENABLED=0 go build ./...` are clean.

## Verification Results

- `go test -tags testtools -race -count=1 ./alita/modules ./alita/db/staff` - ok (modules 115 s)
- `-run '^TestStaffHistory|^TestStaffPanelRender' -v ./alita/modules` - all PASS, including `TestStaffHistoryList`
- `-run '^TestStaffAction(ListFresh|Tally)$' -v ./alita/db/staff` - both PASS
- `go test -tags testtools -count=1 -run '^TestStaffLocaleKeys$' ./alita/i18n` - ok; `make check-translations` - all present; `make check-docs` - no drift
- Task 1 and Task 2 acceptance criteria (greps, `gofmt -l`, `git diff --exit-code go.mod go.sum`) - all pass

## Known Stubs

None.

## Threat Flags

None. The new surface is the plan's own (T-03-11 to T-03-13): a callback read scoped to the pressed message's Staff Group, a strictly parsed offset and escaped, capped fields.

## User Setup Required

None.

## Next Phase Readiness

- Plan 03-05 can add each entry's detail view on top of `staffHistoryLine`, `staffActRecent` and the page keyboard; `renderStaffHistory` returns how many entries it showed.
- The history is read-only. Nothing in it sends a Telegram call to a linked group.
- A native-speaker pass over the eight new strings in es, fr, hi, id, pt and ru is still open (coverage D6).

## Self-Check: PASSED

- Created files exist: `alita/modules/staff_history.go`, `alita/modules/staff_history_test.go`.
- Commits `aefb31d`, `9cc525a`, `6b15a37` and `cdecc63` exist; `test(03-04)` precedes `feat(03-04)` for the code task.

---
*Phase: 03-staff-audit-and-undo*
*Completed: 2026-10-05*
