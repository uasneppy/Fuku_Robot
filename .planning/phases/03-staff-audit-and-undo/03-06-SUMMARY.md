---
phase: 03-staff-audit-and-undo
plan: 06
subsystem: staff-actions
tags: [telegram, staff-actions, undo, run-engine, inline-keyboard, i18n, tdd]
status: complete

requires:
  - phase: 03-staff-audit-and-undo
    provides: staff_actions and staff_action_groups with prior-state and undo columns, staffPriorFromRow, staffResultRow, finalizeStaffActionRecord (plan 03-01)
  - phase: 03-staff-audit-and-undo
    provides: decideStaffUndo, staffUndoVerdict, staffCallRestore and the undone_* and skip_* reasons (plan 03-02)
  - phase: 03-staff-audit-and-undo
    provides: staffRunSpec and startStaffRun, the one spec-driven run engine (plan 03-03)
  - phase: 03-staff-audit-and-undo
    provides: staffCardFromRecord, staffResultsFromRecord, staffGroupsBySeq, the undo-reason wording (plan 03-05)
provides:
  - "Undo everywhere": a button on a finished ban, mute, unban or unmute summary, a confirm card, and a per-group undo run with the presser as actor
  - staff_undo.go (staffKindUndo, staffActionUndoable, staffUndoAsk, staffUndoConfirm, editStaffUndoneOriginal, staffUndoTargets, startStaffUndoRun, runStaffUndoInGroup, executeStaffUndoCall)
  - staff.ClaimUndo, SaveUndoResult, FinalizeUndo, SetSummaryMessage
  - staffGroupPrechecks, shared by the action chain and the undo chain
  - composeStaffSummary, editStaffActionMessageWithMarkup, a markup parameter on the final delivery, staffRunSpec.Finish returning a keyboard and staffRunSpec.Delivered
  - undo cards in the staff action hash (undo_of, undo_kind) and the callback codes ya, yc, yn
affects: [03-07, 03-08, 03-09]

actuals:
  tokens: 23000
  tasks: 2
  commits: 4
plan_head_before: 490fbf5315037afbf7f25b5dcfe879d1cb7f4187
plan_head_after: 414b3458ff7e3412a9777ec5c2b9ba9cecd2c7b7

tech-stack:
  added: []
  patterns:
    - "Undo is a second staffRunSpec on the same engine, card model, Lua compare-and-set, target lock and pacer as a staff action; only the per-group function, the header and the finish hook differ"
    - "The card's issuer is the presser, so the existing issuer check gives 'only the presser confirms' and the shared per-group chain checks the presser's live rights"
    - "One-undo-per-record guarantee is a conditional UPDATE (undo_started_at IS NULL), the last check before the run; the Redis card only carries the confirm"
    - "Message IDs are used only in the chat they belong to: reply and original edit require summary_chat_id == the pressing chat"

key-files:
  created:
    - alita/modules/staff_undo.go
    - alita/modules/staff_undo_test.go
  modified:
    - alita/db/staff/actions.go
    - alita/db/staff/actions_test.go
    - alita/modules/staff_action_run.go
    - alita/modules/staff_action_summary.go
    - alita/modules/staff_action_card.go
    - alita/modules/staff_action_test.go
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
  - "The undo card's issuer is the presser; no separate 'confirmer' field exists"
  - "A staffClaimUndo package variable (test seam, like staffCreateActionRecord) wraps staff.ClaimUndo so a test can make another claimer win between Confirm's last read and its claim"
  - "A callback answer is plain text, so the 'already undone by <name>' alert splices the name cut but not HTML-escaped; the same text on a card (HTML) is escaped"
  - "A group no longer linked gets a link value holding only its ID and stored title; it is display-only because that group is skipped without a Telegram call"
  - "The Undo keyboard is built in Finish only when the record was finalized, the kind is not kick and at least one group is done"

patterns-established:
  - "Per-group prechecks shared by every staff run: staffGroupPrechecks(ctx, b, card, link, pass) with card.Issuer as the actor"
  - "Run spec Delivered hook: the engine reports which message ended up holding the final summary"

requirements-completed: [STAFF-11]

coverage:
  - id: D1
    description: "The final summary of a finished ban, mute, unban or unmute with at least one applied group carries one Undo button holding only the record ID; a kick, an action applied nowhere and a run whose record was not finalized get none; /tban is recorded as a ban"
    requirement: "STAFF-11"
    verification:
      - kind: unit
        ref: "alita/modules/staff_undo_test.go#TestStaffUndoTracer"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_undo_test.go#TestStaffUndoButtonRules"
        status: pass
    human_judgment: false
  - id: D2
    description: "A final edit that falls back to a new message puts the button on that message and the record's summary_msg_id follows it; the Undo then replies to that message"
    requirement: "STAFF-11"
    verification:
      - kind: unit
        ref: "alita/modules/staff_undo_test.go#TestStaffUndoButtonOnFallback"
        status: pass
    human_judgment: false
  - id: D3
    description: "Undo posts a confirm card as a reply to the summary and nothing happens until Confirm; Confirm unbans each applied group once with only_if_banned, ends on a final summary with a tally, marks the original 'Undone by <name>' without a button and stores who, when and each group's undo outcome"
    requirement: "STAFF-11"
    verification:
      - kind: unit
        ref: "alita/modules/staff_undo_test.go#TestStaffUndoTracer"
        status: pass
    human_judgment: false
  - id: D4
    description: "Each group is rechecked live for the presser (creator, or administrator with can_restrict_members); a group where the presser lacks rights is skipped with the reason and gets no write call, whatever the original issuer's rights were"
    requirement: "STAFF-11"
    verification:
      - kind: unit
        ref: "alita/modules/staff_undo_test.go#TestStaffUndoAllSkipped"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_undo_test.go#TestStaffUndoPresserNeedsTheRestrictRight"
        status: pass
    human_judgment: false
  - id: D5
    description: "Outsiders cannot start an undo, only the presser can confirm, a presser who left cannot confirm, another Staff Group's record is refused, the loser of the claim runs nothing and names the winner, and an action is undone once"
    requirement: "STAFF-11"
    verification:
      - kind: unit
        ref: "alita/modules/staff_undo_test.go#TestStaffUndoAccess"
        status: pass
      - kind: unit
        ref: "alita/db/staff/actions_test.go#TestStaffActionClaimUndo"
        status: pass
    human_judgment: false
  - id: D6
    description: "The original summary is edited and replied to only when the record's summary_chat_id is the pressing chat, an unreachable original does not stop the undo, and the undo card and action card never run through each other's Confirm"
    requirement: "STAFF-11"
    verification:
      - kind: unit
        ref: "alita/modules/staff_undo_test.go#TestStaffUndoAfterRekey"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_undo_test.go#TestStaffUndoOriginalEditFails"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_undo_test.go#TestStaffUndoCardsDoNotCross"
        status: pass
    human_judgment: false
  - id: D7
    description: "An undo where every group is skipped still ends with a final summary and a record with undo_finished_at; the summary lists applied groups in the original's order and counts the others in one header line; unapplied rows are recorded skip_not_applied"
    requirement: "STAFF-11"
    verification:
      - kind: unit
        ref: "alita/modules/staff_undo_test.go#TestStaffUndoAllSkipped"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_undo_test.go#TestStaffUndoOrder"
        status: pass
      - kind: unit
        ref: "alita/db/staff/actions_test.go#TestStaffActionFinalizeUndo"
        status: pass
    human_judgment: false
  - id: D8
    description: "A prior restriction is put back with its recorded permission set and end date through restrictChatMember with use_independent_chat_permissions, not lifted"
    requirement: "STAFF-11"
    verification:
      - kind: unit
        ref: "alita/modules/staff_undo_test.go#TestStaffUndoRestoresPriorRestriction"
        status: pass
    human_judgment: false

duration: 25min
completed: 2026-10-05
---

# Phase 3 Plan 6: Undo everywhere Summary

**A staff member presses "Undo everywhere" on a finished ban, mute, unban or unmute summary, confirms a card that replies to it, and the bot puts each applied group back to its recorded prior state, checking the presser's own live rights in every group; the result is its own summary and the original gains an "Undone by" line.**

## Performance

- **Duration:** about 25 min
- **Tasks:** 2 (tracer, then the edge cases and AGENTS.md)
- **Files modified:** 17 (2 created, 15 modified)
- **Lines:** about 2000 added across code, tests and locales

## Accomplishments

- **Undo is a second run on the same machinery.** The undo card is `action=undo` with `undo_of` and `undo_kind` in the same Redis hash, with the same Lua compare-and-set, per-target lock, pacer and summary rules. The card's issuer is the presser, so "only the member who pressed Undo can confirm" needed no new code. The per-group chain is `staffGroupPrechecks`, extracted unchanged from `runStaffActionInGroup` and now shared, with `card.Issuer` as the actor.
- **Every call is chosen by `decideStaffUndo` and made by `executeStaffUndoCall`,** which switches only on the verdict: ban, unban and unmute go through `executeStaffCall`, a restore is one paced `restrictChatMember` with the recorded permissions, `use_independent_chat_permissions` and the recorded end date.
- **One undo per record is a conditional update.** `ClaimUndo` (`undo_started_at IS NULL`, `RowsAffected == 1`) is the last check before the run, outlives the Redis card, and the loser aborts naming the winner.
- **The Undo button follows the summary.** `Finish` returns the keyboard only for a finalized record of a non-kick action with a done group; `Delivered` reports which message holds the final summary and the action run points `summary_msg_id` at a fallback message.
- **Messages stay in their own chat.** The reply and the original-summary edit both require `summary_chat_id` to be the pressing chat, so after a Staff Group migration the card is posted without a reply and nothing old is edited.
- **Seven locale keys in all 7 locales,** three callback codes (`ya`, `yc`, `yn`), and the AGENTS.md rules.

## Task Commits

1. **Task 1: tracer, undo a ban end to end** - `7943a94` (test, RED) then `ec40cae` (feat, GREEN)
2. **Task 2: button rules, fallback, re-key, empty and ordering edges, AGENTS.md** - `0ac4c05` (test) then `414b345` (feat, AGENTS.md)

The plan-metadata commit with this file follows. No refactor commit was needed.

## Tracer feedback gate

The tracer's `<verify>` was run end to end after the GREEN commit and again as part of the full package run; it passed (`TestStaffUndoTracer`, the Phase 2/3 regression run, locale checks). Tracer verified end-to-end, expansion (Task 2) went ahead.

## TDD Gate Compliance

- **Task 1:** RED `7943a94` failed on the planned assertion ("the final summary has 0 buttons, want exactly one Undo button"), not on a compile error; the test is written against the wire codes `ya`, `yc` and `yn` as string literals so it compiled before the implementation existed. GREEN `ec40cae` passes.
- **Task 2:** the plan allowed that some tests pass at once. All of them passed against Task 1's implementation, because the guards (summary chat, claim, presser rights, kind check) were built in Task 1. That is an expected GREEN, and it was checked by mutation instead of trusted: removing the `summary_chat_id` guard on the reply fails `TestStaffUndoAfterRekey`; removing it on the original edit fails the same test on a different assertion; dropping the `Delivered` hook fails `TestStaffUndoButtonOnFallback`; acting as the original issuer instead of the presser fails `TestStaffUndoAllSkipped` and `TestStaffUndoPresserNeedsTheRestrictRight`; dropping the Staff Group membership check fails `TestStaffUndoAccess/an_outsider_cannot_start_an_undo`. Each mutation was reverted. The `feat` commit of Task 2 holds only the AGENTS.md change, as in earlier plans of this phase.
- `gsd_run check tdd-red-evidence` was not run; RED evidence was verified by reading the failing assertion output.

## Files Created/Modified

- `alita/modules/staff_undo.go` - the undo feature (button, card, Ask, Confirm, original-summary edit, targets, run, per-group chain, call execution) and the `staffClaimUndo` seam
- `alita/modules/staff_undo_test.go` - eleven tests and their helpers
- `alita/db/staff/actions.go` and `actions_test.go` - `SetSummaryMessage`, `ClaimUndo`, `SaveUndoResult`, `FinalizeUndo` and three tests
- `alita/modules/staff_action_run.go` - `staffGroupPrechecks`, `Finish` returning a keyboard, `Delivered`, the action spec's Undo keyboard and summary-message follow-up
- `alita/modules/staff_action_summary.go` - `composeStaffSummary`, the undo header branch, `editStaffActionMessageWithMarkup`, markup on the final edit and the fallback send, the message ID returned by the delivery
- `alita/modules/staff_action_card.go` - `UndoOf`, `UndoKind`, their storage, and the action Confirm's refusal of an undo card
- `alita/modules/staff.go` - `ya`, `yc`, `yn` and their dispatch
- `alita/modules/staff_action_test.go` - one Phase 2 assertion updated (see Deviations)
- `locales/*.yml` (7 files) - `staff_undo_button`, `staff_undo_label`, `staff_undo_card_applies`, `staff_undo_marker`, `staff_undo_not_applied_note`, `staff_undo_already`, `staff_undo_not_available`
- `AGENTS.md` - the undo rules in the decision, data, trap and callback sections

## Decisions Made

See `key-decisions` above. The ones worth repeating: the presser is the card's issuer (so the existing compare-and-set does the work), and the claim, not the card, is what guarantees a single undo.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Test made obsolete by the new behavior] Phase 2 tracer asserted no keyboard on the final edit**
- **Found during:** Task 1 regression run
- **Issue:** `TestStaffActionTracer` ended with `wantNoKeyboard` on the last edit of a ban summary. The plan's own must-have is that this summary now carries the Undo button.
- **Fix:** it now asserts that no Confirm or Cancel button remains and that any button left is the Undo button, keeping its intent (the card's buttons are gone).
- **Files modified:** `alita/modules/staff_action_test.go`
- **Commit:** `ec40cae`

**2. [Rule 1 - Correctness] "Already undone by <name>" alert uses a plain name, not an escaped one**
- **Found during:** Task 1 (writing `staffUndoAsk`)
- **Issue:** the plan says the name is spliced in escaped. An `answerCallbackQuery` text is plain text, so escaping would show `&lt;` and `&amp;` to the user.
- **Fix:** the alert splices the name collapsed and cut to 64 runes without escaping; the same sentence on a card (HTML) is escaped. `staffUndoAlreadyText` takes a flag for which one.
- **Files modified:** `alita/modules/staff_undo.go`
- **Commit:** `ec40cae`

### Additions beyond the plan text

- **`staffClaimUndo` seam** in `staff_undo.go` (a package variable wrapping `staff.ClaimUndo`, the `staffCreateActionRecord` precedent). Without it the lost-claim path, where Confirm's pre-check saw no undo and its claim then loses, cannot be reached on purpose: every sequence of taps hits the earlier "already undone" check first. It is committed with the Task 2 tests (`0ac4c05`).
- **Extra tests** for the plan's two prohibitions and for paths the plan's list leaves to plan 03-08: `TestStaffUndoAccess` (outsider, foreign record, wrong confirmer, presser who left, lost claim, repeat undo), `TestStaffUndoPresserNeedsTheRestrictRight`, `TestStaffUndoRestoresPriorRestriction` (the restore call, its permissions and end date), plus `/tban` recorded as a ban and a forged Undo press on a kick record.
- **Unlinked group's link value** holds only the group ID and stored title, not a Staff Group ID: `staffUndoTargets` has no staff chat parameter and that value is only read for display, because that group is skipped with no Telegram call.

**Total deviations:** 2 auto-fixed (1 obsolete test, 1 plain-text correctness), 3 additions. **Impact:** none on scope; every must-have truth holds.

## Issues Encountered

- `make lint` is unavailable (golangci-lint is built with go1.25 and refuses the go1.26.0 module); `gofmt -l` on the touched files, `go vet -tags testtools ./alita/modules ./alita/db/staff` and `CGO_ENABLED=0 go build ./...` are clean.
- Tests ran on SQLite only. The new SQL is plain (conditional `UPDATE ... WHERE ... IS NULL`, `outcome <> 'done' AND undo_outcome = ''`) and portable, but it has not run against PostgreSQL here.
- The worktree command guard refuses some shell shapes (heredocs holding non-ASCII text, `git` inside `&&` chains); locale edits went through a script file in the scratchpad and git calls were issued one per command.
- `gofmt -l alita/modules` still lists `greetings_command_test.go`, unformatted at the base commit and out of scope.

## Verification Results

- `go test -tags testtools -race -count=1 -run '^TestStaffUndoTracer$' -v ./alita/modules` - PASS
- `go test -tags testtools -race -count=1 -run '^TestStaffUndo' -v ./alita/modules` - all PASS (decision tests and the eleven new tests, including `TestStaffUndoButtonOnFallback` and `TestStaffUndoOrder`)
- `go test -tags testtools -race -count=1 -run '^TestStaffAction(ClaimUndo|FinalizeUndo|SetSummaryMessage)$' -v ./alita/db/staff` - all PASS
- `go test -tags testtools -race -count=1 ./alita/modules ./alita/db/staff ./alita/i18n` - ok (modules 127 s)
- `go test -tags testtools -count=1 -run '^TestStaffLocaleKeys$' ./alita/i18n`, `make check-translations`, `make check-docs` - pass, no drift
- Acceptance criteria of both tasks re-run and passing: `func executeStaffUndoCall(`, `UseIndependentChatPermissions: true`, `decideStaffUndo(card.UndoKind`, `staff.ClaimUndo(`, `undo_started_at IS NULL`, `func staffGroupPrechecks(`, `func composeStaffSummary(tr *i18n.Translator, header string`, `staffActUndoAsk = "ya"`, `"undo_of"`, zero admin-cache calls in `staff_undo.go`, `staff_undo_button` once in each of es fr hi id pt ru, AGENTS.md mentions `decideStaffUndo` and `undo_kind`, `a.SummaryChatID != staffChatID`, `gofmt -l` clean on the touched files, `go.mod` and `go.sum` unchanged

## Known Stubs

None.

## Threat Flags

None. The new buttons and cards are the plan's own surface (T-03-16 to T-03-20): the presser's live rights per group (tested, grep gate clean), Staff Group membership and a chat-bound record at Ask, the issuer check and a membership recheck at Confirm, calls only through `decideStaffUndo`, new codes plus mutual refusal of cards, and message IDs used only in their own chat (tested).

## User Setup Required

None.

## Next Phase Readiness

- Plans 03-07 to 03-09 can build on a complete undo: the record carries `undo_by`, `undo_by_name`, `undo_started_at`, `undo_finished_at` and each row's `undo_outcome`, `undo_reason` and `undo_detail`, and the history detail view already renders them.
- Plan 03-08 can prove the remaining restores (ban over a shorter ban, unban re-banned, unmute re-restricted) end to end; this plan proves the unban and the restriction restore. Research assumption A2 (restrict on a left target) still needs a live check.
- `TestRepositoryMigrationChain` against PostgreSQL still has to run in CI or from the main checkout (carried over from plan 03-01).

## Self-Check: PASSED

- Created and modified files exist: `staff_undo.go`, `staff_undo_test.go`, `actions.go`, `actions_test.go` (confirmed by listing).
- Commits `7943a94`, `ec40cae`, `0ac4c05` and `414b345` exist; `test(03-06)` precedes `feat(03-06)` for both tasks.

---
*Phase: 03-staff-audit-and-undo*
*Completed: 2026-10-05*
