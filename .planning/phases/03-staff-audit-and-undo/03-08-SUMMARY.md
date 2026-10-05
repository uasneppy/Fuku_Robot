---
phase: 03-staff-audit-and-undo
plan: 08
subsystem: staff-actions
tags: [telegram, staff-actions, undo, log-channel, i18n, tdd, fake]
status: complete

requires:
  - phase: 03-staff-audit-and-undo
    provides: decideStaffUndo and its verdict table (plan 03-02)
  - phase: 03-staff-audit-and-undo
    provides: postStaffActionLog, the log labels and the spec-driven run engine (plan 03-03)
  - phase: 03-staff-audit-and-undo
    provides: undo card, undo run, executeStaffUndoCall with the independent-permissions restore (plan 03-06)
  - phase: 03-staff-audit-and-undo
    provides: undo card lifecycle, shared target lock and drain (plan 03-07)
provides:
  - A stateful-fake extension that applies Telegram's implied-permission rule, which makes the restore's use_independent_chat_permissions flag observable
  - End-to-end proof that undo restores exactly the prior state in every decision-table row, skips changed, unapplied and unlinked groups, and acts only on the presser's live rights
  - "#STAFF_UNDO" log post per group where an undo succeeded, through a shared sendStaffLogPost
affects: [03-09]

actuals:
  tokens: 26000
  tasks: 2
  commits: 3
plan_head_before: e7e194e13d75b992c1bc54ba2f0a23c9f901c2bd
plan_head_after: 449bf9031a9a61936ac89f27405e3b7613dd9b94

tech-stack:
  added: []
  patterns:
    - "A shared sendStaffLogPost takes a body closure, so action and undo posts share the destination lookup, translator and paced send"
    - "Mutation-check a restore test that passes at once: drop the flag, see it fail, revert"

key-files:
  created:
    - alita/modules/staff_undo_restore_test.go
    - alita/modules/staff_undo_log_test.go
  modified:
    - alita/modules/staff_action_fake_test.go
    - alita/modules/staff_log.go
    - alita/modules/staff_undo.go
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml
    - AGENTS.md

key-decisions:
  - "Task 1 needed no production change: the restore, the not-applied filter, the link lookup and the presser rechecks built in 03-06 already satisfy every row, so the commit is test(03-08) and the tests were validated by mutation"
  - "The undo post names the presser as Admin and the original issuer only inside 'undoes <action> by <issuer>', after translation through tokens, with the name capped and HTML-escaped"
  - "staffUndoConfirm sets card.IssuerName to the presser before the run, because the undo card is created at Ask and its issuer is the presser"

patterns-established:
  - "Split a post sender into destination plus paced send (sendStaffLogPost) and let each post kind pass only its body"

requirements-completed: [STAFF-11, STAFF-09]

coverage:
  - id: D1
    description: "Undo puts back exactly the recorded prior state in each group: partial restriction with the same permission bits and end date, a shorter ban with its end date, a lifted ban when the old restriction ended, a re-ban after an unban, a re-restriction after an unmute, an unmute after a mute of a member"
    requirement: "STAFF-11"
    verification:
      - kind: unit
        ref: "alita/modules/staff_undo_restore_test.go#TestStaffUndoRestores"
        status: pass
    human_judgment: false
  - id: D2
    description: "Every restore is sent with use_independent_chat_permissions and the recorded permission struct, so Telegram's implied permissions cannot widen or narrow it"
    requirement: "STAFF-11"
    verification:
      - kind: unit
        ref: "alita/modules/staff_undo_restore_test.go#TestStaffUndoRestores"
        status: pass
    human_judgment: true
    rationale: "The fake models the implied-permission rule from the gotgbot doc comment; live Telegram behaviour (research A3) cannot be proven here and is a manual check in 03-VALIDATION.md and plan 03-09"
  - id: D3
    description: "A group changed since the staff action is skipped with no write call, an originally unapplied group gets no Telegram call and is recorded skip_not_applied, and an unlinked group is skipped as link removed with no call"
    requirement: "STAFF-11"
    verification:
      - kind: unit
        ref: "alita/modules/staff_undo_restore_test.go#TestStaffUndoChangedSince"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_undo_restore_test.go#TestStaffUndoNotAppliedOriginally"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_undo_restore_test.go#TestStaffUndoLinkRemoved"
        status: pass
    human_judgment: false
  - id: D4
    description: "Rights are the presser's, live, per group: creator and administrator with can_restrict_members undone, administrator without it and plain member skipped, the original issuer's rights playing no part"
    requirement: "STAFF-11"
    verification:
      - kind: unit
        ref: "alita/modules/staff_undo_restore_test.go#TestStaffUndoPresserRights"
        status: pass
    human_judgment: false
  - id: D5
    description: "Each group where the undo succeeded gets one #STAFF_UNDO post in its admin log channel naming the presser, the target, the undone action and 'via Staff Group', never the Staff Group; skipped groups get none; a failed post or an off category never changes the undo's result"
    requirement: "STAFF-09"
    verification:
      - kind: unit
        ref: "alita/modules/staff_undo_log_test.go#TestStaffUndoLogPosts"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_undo_log_test.go#TestStaffUndoLogFailureKeepsDone"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_undo_log_test.go#TestStaffUndoLogCategoryOff"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_undo_log_test.go#TestStaffReverseKind"
        status: pass
    human_judgment: false

duration: 35min
completed: 2026-10-05
---

# Phase 3 Plan 8: Exact restore and the #STAFF_UNDO log Summary

**Eight restore rows, the changed-since, not-applied, link-removed and presser-rights cases are proven end to end against a fake that now applies Telegram's implied-permission rule, and every successful undo is posted to the group's admin log channel as #STAFF_UNDO without revealing the Staff Group.**

## Performance

- **Duration:** about 35 min
- **Tasks:** 2 (restore proof; undo log post and AGENTS.md)
- **Files modified:** 13 (2 created, 11 modified)
- **Commits:** 3 task commits (one test-only for Task 1, RED and GREEN for Task 2)

## Accomplishments

- **Fake extension.** `restrictChatMember` in `staffActionFake` runs `staffImpliedPermissions` unless `use_independent_chat_permissions` is true: `can_send_other_messages` or `can_add_web_page_previews` imply the seven send permissions, `can_send_polls` implies `can_send_messages`. Every existing test passed unchanged.
- **`TestStaffUndoRestores`, 8 subtests.** ban over a partial restriction, ban over a shorter ban, ban over a restriction that ended, mute of a member, mute over a shorter mute, unban, unban of a ban that ended (skipped, no write), unmute. Each runs the real command, Undo, Confirm, then reads the fake's record and the recorded calls (flag, permission JSON, `until_date`, summary line marker).
- **Four scenario tests.** `TestStaffUndoChangedSince` (group lifted by an admin, group re-banned longer, third group undone), `TestStaffUndoNotAppliedOriginally` (no `getChatMember`, no write, row `skipped` / `skip_not_applied`), `TestStaffUndoLinkRemoved` (no Telegram call at all), `TestStaffUndoPresserRights` (creator, admin with and without the restrict right, plain member; the original issuer demoted first).
- **Undo log post.** `sendStaffLogPost(ctx, b, link, body)` factors the destination, translator and paced send out of `postStaffActionLog`, which keeps its behaviour through it. `composeStaffUndoLog` and `postStaffUndoLog` build `#STAFF_UNDO · <reverse action> · Admin: <presser> · User: <target> · undoes <action> by <issuer> · via Staff Group`. The undo run's `AfterGroup` posts only for a done result, after `SaveUndoResult`.
- **Locale and docs.** `staff_log_undoes` in all 7 locales; the AGENTS.md log-post bullet records the undo posts.

## Task Commits

1. **Task 1: restore proof** - `b4a0d7b` (test; no production change)
2. **Task 2: undo log post** - `249bdb5` (test, RED) then `449bf90` (feat, GREEN)

**Plan metadata:** the commit holding this file follows. No refactor commit was needed.

## TDD Gate Compliance

- **Task 1:** all new tests passed against the existing code, which the plan anticipated ("fixing whatever they expose"; nothing was exposed). They were checked by mutation rather than trusted: removing `UseIndependentChatPermissions: true` from the restore fails "ban over a partial restriction", "mute over a shorter mute" and "unmute" (the flag assertion), and treating failed rows as undo targets fails `TestStaffUndoNotAppliedOriginally` ("group B got 2 getChatMember calls during the undo"). Both mutations were reverted. The commit is `test(03-08)`, as the plan allows when no production change was needed.
- **Task 2:** RED `249bdb5` failed on the planned assertions: `TestStaffUndoLogPosts` ("channel A has 0 #STAFF_UNDO posts, want exactly 1"), `TestStaffUndoLogFailureKeepsDone` ("log post attempts = 1, want 2") and `TestStaffReverseKind` (four wrong answers). `TestStaffUndoLogCategoryOff` passed in RED because nothing was posted yet, so it only became meaningful with GREEN. A placeholder `staffReverseKind` that returned its input was committed in RED so every commit compiles. GREEN `449bf90` passes all of them. A mutation (post for every outcome) fails `TestStaffUndoLogPosts` with channel B receiving a post.
- `gsd_run check tdd-red-evidence` was not run; RED evidence was read from the failing assertion output.

## Files Created/Modified

- `alita/modules/staff_undo_restore_test.go` - the restore, changed-since, not-applied, link-removed and presser-rights tests and their helpers
- `alita/modules/staff_undo_log_test.go` - the undo log tests
- `alita/modules/staff_action_fake_test.go` - `staffImpliedPermissions` and its use in `restrictChatMember`
- `alita/modules/staff_log.go` - `sendStaffLogPost`, `staffReverseKind`, `composeStaffUndoLog`, `postStaffUndoLog`
- `alita/modules/staff_undo.go` - `AfterGroup` posts for done groups; `card.IssuerName` is the presser at Confirm
- `locales/*.yml` (7 files) - `staff_log_undoes`
- `AGENTS.md` - undo log-post rule

## Decisions Made

See `key-decisions`. The one worth repeating: the restore needed no code change, because 03-06 already sends the recorded struct with the independent-permissions flag; the value of this plan is that dropping the flag now fails a named test.

## Deviations from Plan

None - plan executed exactly as written. (Task 1 ended as `test(03-08)` with no production change, the branch the plan names; the RED commit of Task 2 carries a one-line placeholder so the tree compiles, replaced in GREEN.)

## Live-check items

- **Research A2, restrict on a kicked or left target.** The restore and the re-ban rely on `restrictChatMember` / `banChatMember` being accepted for a target whose live status is `left` or `kicked` (the state the staff action left). The fake accepts it and replaces the status; live Telegram has not been checked and cannot be from this environment.
- **Research A3, use_independent_chat_permissions.** The fake's implied-permission rule is modelled from the gotgbot doc comment. Live confirmation that the stored set equals the sent set belongs in plan 03-09's end-of-phase human checks and `03-VALIDATION.md` "Manual-Only Verifications".

## Issues Encountered

- `make lint` is unavailable here (golangci-lint is built with go1.25 and refuses the go1.26.0 module). `gofmt -l` on the touched files, `go vet -tags testtools ./alita/modules` and `CGO_ENABLED=0 go build ./...` are clean. `gofmt -l alita/modules/` also lists `greetings_command_test.go`, which this plan does not touch.
- Tests ran on SQLite only; nothing in this plan adds SQL.
- The worktree command guard refuses chained git commands; git calls were issued one per command. While mutation-testing, a `git checkout -- alita/modules/staff_undo.go` also discarded the uncommitted GREEN edits to that file; they were re-applied before the commit and the full suite was run after.
- Carried over: `TestStaffActionNonStaffUnchanged/tmute_as_a_reply` is flaky across a second boundary (STATE.md). It did not fail in the full package run here.

## Verification Results

- `go test -tags testtools -race -count=1 -run '^TestStaffUndo(Restores|ChangedSince|NotAppliedOriginally|LinkRemoved|PresserRights)' -v ./alita/modules` - all PASS (including `TestStaffUndoRestores`, 8 subtests)
- `go test -tags testtools -race -count=1 -run '^TestStaffAction|^TestStopStaffActions|^TestStaffUndo' ./alita/modules` - ok
- `go test -tags testtools -race -count=1 -run '^TestStaffUndoLog|^TestStaffReverseKind$|^TestStaffActionLog' -v ./alita/modules` - all PASS (including `TestStaffUndoLogPosts`)
- `go test -tags testtools -race -count=1 ./alita/modules ./alita/db/staff` - ok (modules 138 s)
- `go test -tags testtools -count=1 -run '^TestStaffLocaleKeys$' ./alita/i18n`, `make check-translations`, `make check-docs` - pass, no drift
- Acceptance criteria re-run and passing: the flag name in the fake, 8 `t.Run(` in the restore test, `composeStaffUndoLog` and `"#STAFF_UNDO"` in `staff_log.go`, `postStaffUndoLog(` in `staff_undo.go`, the key once in each of es fr hi id pt ru, `STAFF_UNDO` in AGENTS.md, `gofmt -l` clean on the touched files, `go.mod` and `go.sum` unchanged

## Known Stubs

None.

## Threat Flags

None. The surface touched is the plan's own: T-03-25 (the flag assertion fails when dropped), T-03-26 (presser, not-applied and link tests) and T-03-27 (posts scanned for the Staff Group's title and ID).

## User Setup Required

None.

## Next Phase Readiness

- Plan 03-09 can rely on undo that restores exactly and logs each success. The two live checks above (A2, A3) belong in its end-of-phase human checks.

## Self-Check: PASSED

- `alita/modules/staff_undo_restore_test.go` and `alita/modules/staff_undo_log_test.go` exist; the modified files exist.
- Commits `b4a0d7b`, `249bdb5` and `449bf90` exist; `test(03-08)` precedes `feat(03-08)` for Task 2.

---
*Phase: 03-staff-audit-and-undo*
*Completed: 2026-10-05*
