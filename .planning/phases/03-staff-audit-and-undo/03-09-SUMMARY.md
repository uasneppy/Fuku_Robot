---
phase: 03-staff-audit-and-undo
plan: 09
subsystem: staff-actions
tags: [telegram, staff-group, undo, history, i18n, docs, agents-md]

requires:
  - phase: 03-staff-audit-and-undo
    provides: staffActionUndoable, staffUndoKeyboard and staffUndoAsk (plan 03-06), the history detail view (plans 03-04, 03-05), undo lifecycle and log posts (plans 03-07, 03-08)
provides:
  - Undo everywhere button in the history detail view, gated by staffActionUndoable
  - staff_help_msg in 7 locales describing Recent actions, Undo everywhere and the log-channel posts
  - regenerated /staff command docs
  - final AGENTS.md pass over every Phase 3 rule
affects: [phase-04, verify-work]

plan_head_before: b2f4a6663c470a57a8c8cc652542fc73bf884bc0
plan_head_after: 20e57d1e4e0f937143e3b60b1345ffa258fbc42d

actuals:
  tokens: 14000
  tasks: 2
  commits: 4

tech-stack:
  added: []
  patterns:
    - "A second entry point to a dangerous flow reuses the first entry point's handler (a=ya, staffUndoAsk) and carries only the record ID, so no check is duplicated or skipped"

key-files:
  created:
    - alita/modules/staff_history_undo_test.go
  modified:
    - alita/modules/staff_history.go
    - alita/modules/staff_history_test.go
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml
    - docs/src/content/docs/commands/staff/index.md
    - AGENTS.md

key-decisions:
  - "The detail view's Undo button reuses staffUndoKeyboard and the existing a=ya press path instead of a new callback, so staffUndoAsk's live membership, record chat binding, undoable check and the presser-only Confirm apply unchanged"
  - "The new help paragraph goes after the two-line card paragraph and before the /sban refusal line; the localized help text quotes each locale's own button labels"

requirements-completed: [SETUP-09, STAFF-11]

coverage:
  - id: D1
    description: "The history detail view offers Undo everywhere exactly while the action can be undone, and pressing it runs the same confirm-card flow (reply to the original summary, presser-only Confirm)"
    requirement: STAFF-11
    verification:
      - kind: unit
        ref: "alita/modules/staff_history_undo_test.go#TestStaffHistoryDetailUndo"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_history_undo_test.go#TestStaffHistoryDetailUndoRules"
        status: pass
    human_judgment: false
  - id: D2
    description: "After an undo the detail view shows no Undo button, shows who undid it with each group's undo result, and the list line reads undone"
    requirement: STAFF-11
    verification:
      - kind: unit
        ref: "alita/modules/staff_history_undo_test.go#TestStaffHistoryDetailUndo"
        status: pass
    human_judgment: false
  - id: D3
    description: "The /staff help text in all 7 languages describes Recent actions, Undo everywhere and the log-channel posts, and the generated docs match"
    requirement: SETUP-09
    verification:
      - kind: other
        ref: "make check-translations && make check-docs"
        status: pass
    human_judgment: true
    rationale: "Translation quality and the readability of the help text need a native-speaker read; automation only proves the keys and the generated docs are consistent"
  - id: D4
    description: "AGENTS.md states every Phase 3 rule and matches the code"
    verification: []
    human_judgment: true
    rationale: "Accuracy of prose rules against the code was checked by hand (each named symbol was confirmed to exist) but no test asserts it"
  - id: D5
    description: "Live Telegram behavior of undo (restrictChatMember on banned or departed users, use_independent_chat_permissions) and the log-channel posts"
    verification: []
    human_judgment: true
    rationale: "Cannot be proven with the hand-written fake (research A2, A3); end-of-phase live check from 03-VALIDATION.md is pending"

duration: 28min
completed: 2026-10-05
status: complete
---

# Phase 3 Plan 09: Undo from Recent actions, help text and the phase gate Summary

**The history detail view now offers "Undo everywhere" through the same staffUndoAsk flow as the summary button, /staff help and docs describe Recent actions, Undo and the log posts in all 7 languages, and AGENTS.md carries every Phase 3 rule.**

## Performance

- **Duration:** 28 min
- **Started:** 2026-10-05T19:30:00Z
- **Completed:** 2026-10-05T19:58:12Z
- **Tasks:** 2
- **Files modified:** 12 (1 created, 11 modified)

## Accomplishments

- `renderStaffHistoryDetail` adds the Undo row above Back while `staffActionUndoable(a, groups)` holds. The button carries only `a=ya&r=<id>`; the press is handled by `staffUndoAsk`, so live membership, record chat binding, the undoable check and the presser-only Confirm all still apply (T-03-28, T-03-29).
- After an undo the detail view drops the button and shows who undid it with each group's undo result, and the list line reads undone.
- `staff_help_msg` in en, es, fr, hi, id, pt and ru gains the Recent actions sentence on the /staff line and a paragraph on Undo everywhere (who may press it, the presser confirms, each group is checked live, changed groups are left alone, a kick cannot be undone) and the log-channel posts. `make generate-docs` regenerated `commands/staff/index.md`.
- AGENTS.md pass: every Phase 3 rule named in the plan was confirmed present and accurate against the code (each named symbol was grepped), and one rule was added for the detail view's Undo button.

## Task Commits

1. **Task 1 RED:** `6125223` (test) - detail-view Undo tests; `TestStaffHistoryDetailUndo` failed on the missing Undo row (keyboard had only the Back row)
2. **Task 1 GREEN:** `06b4e61` (feat) - Undo button in the detail view, plus the `TestStaffHistoryDetail` expectation update
3. **Task 2 help and docs:** `c86008c` (docs) - 7 locales and regenerated docs
4. **Task 2 AGENTS.md:** `20e57d1` (docs) - final rules pass

No refactor commit was needed.

## TDD Gate Compliance

RED (`test(03-09)`, `6125223`) precedes GREEN (`feat(03-09)`, `06b4e61`). The RED run failed on the target test for the planned reason (assertion on the missing Undo button). `TestStaffHistoryDetailUndoRules` passed in RED because it only asserts the absence of Undo, which was already true; it guards the gating after GREEN.

## Phase gate results

| Check | Result |
|-------|--------|
| `make test` | pass (exit 0, no FAIL line) |
| `make check-translations` | pass, all translations present |
| `make check-docs` | pass, no drift |
| `go vet -tags testtools ./...` | pass, no output |
| `CGO_ENABLED=0 go build ./...` | pass |
| `gofmt -l alita main.go` | only the pre-existing `alita/modules/greetings_command_test.go` |
| `git diff --exit-code go.mod go.sum` | unchanged |
| `make lint` | cannot run here: `Error: can't load config: the Go language version (go1.25) used to build golangci-lint is lower than the targeted Go version (1.26.0)`. Relied on gofmt and go vet as plans 02-06 and 02-07 did |
| Migration chain on PostgreSQL 16 (`TestRepositoryMigrationChain`) | **delegated to the orchestrator post-merge**: the worktree guard refuses `su`/`runuser`, so the throwaway cluster cannot be started from here. Not run by this agent; not claimed as passed |

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Existing detail-view test asserted exactly one button**
- **Found during:** Task 1 GREEN
- **Issue:** `TestStaffHistoryDetail` and `TestStaffHistoryDetailUnlinked` (plan 03-05) use `historyKeyboardOf`, which treats any action other than dt, rc and refresh as an error and expected the detail keyboard to be a single Back button. A finished ban now legitimately carries an Undo button.
- **Fix:** `historyKeyboardOf` classifies `a=ya` into a new `undo` field; `TestStaffHistoryDetail` now expects the Undo and the Back button (total 2).
- **Files modified:** `alita/modules/staff_history_test.go`
- **Verification:** `go test -tags testtools -race -count=1 -run '^TestStaffHistory|^TestStaffUndo' ./alita/modules` passes
- **Committed in:** `06b4e61`

---

**Total deviations:** 1 auto-fixed (1 test expectation updated for the planned behavior change)
**Impact on plan:** None on scope; the plan's own change made the old assertion obsolete.

## Issues Encountered

None beyond the delegated and toolchain-limited gate steps listed above.

## Pending live verification (end-of-phase, for the verifier and UAT)

The Task 2 human-check (03-VALIDATION.md Manual-Only Verifications) is not run here and is pending live verification. Link two test supergroups, each with a log channel, to a test Staff Group. (1) In group A restrict a test account to text-only for 1 day; run a staff /ban from the Staff Group, Confirm, then Undo everywhere and Confirm. (2) Ban the account in group B for 2 hours with the per-group /ban, run a staff /unban, then Undo it. (3) Inspect both log channels. (4) Open /staff, Recent actions and the entry's detail. Expected: (1) restricted again in group A, text allowed and media blocked, ending at the original time; (2) banned again in group B until the original end time; (3) each channel has a #STAFF_BAN or #STAFF_UNBAN post and a #STAFF_UNDO post naming the issuer and presser and "via Staff Group", never the Staff Group's title or ID; (4) the list shows both actions as undone and the detail shows each group's result and undo result. This covers research A2 and A3, which the hand-written fake cannot prove.

## Known Stubs

None.

## Threat Flags

None. No new endpoints, auth paths or schema; the new button reuses the existing `a=ya` handler.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

Phase 3 plans are all executed. Remaining before the phase can close: the orchestrator's PostgreSQL migration-chain run after merge, `make lint` on a toolchain built with go1.26, and the live Telegram check above.

## Self-Check: PASSED

- `alita/modules/staff_history_undo_test.go` exists; commits `6125223`, `06b4e61`, `c86008c`, `20e57d1` exist in the branch history.
- Acceptance criteria: grep for `staffActionUndoable(a, groups)` and `staffUndoKeyboard(tr, a.ID)` in `staff_history.go` match; `↩` present in `staff_help_msg` of all 7 locales; docs contain "Recent actions"; AGENTS.md contains `executeStaffUndoCall` and `ClaimUndo`; go.mod and go.sum unchanged.

---
*Phase: 03-staff-audit-and-undo*
*Completed: 2026-10-05*
