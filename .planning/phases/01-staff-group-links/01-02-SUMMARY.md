---
phase: 01-staff-group-links
plan: 02
subsystem: staff-groups
tags: [go, gotgbot, gorm, i18n, telegram, callbacks, tdd]

requires:
  - phase: 01-staff-group-links
    provides: "staff_groups and staff_group_links tables, CreateStaffGroup, GetStaffGroupFresh, CheckOwner, staffBotClient test harness"
provides:
  - "/unsetstaff behind a Confirm/Cancel tap and a live creator check; removal of the status and all its links in one transaction (staff.DeleteStaffGroupWithLinks)"
  - "staff| callback dispatcher (actions uy, un) that takes the chat from the message and re-checks the presser live"
  - "helpers.IsAnonymousSender and helpers.RejectAnonymousSender (first RequiredChecks entry; reused by Phase 2 for STAFF-06)"
  - "chat_status.FetchBotMember: live, tri-state bot membership (found, missing, unknown)"
  - "/setstaff refusal matrix in a fixed order (anonymous, chat type, live creator, bot administrator, role conflict) and the in-transaction D-10 role check (staff.ErrRoleConflict)"
  - "staff.CountLinksByStaffFresh"
affects: [01-03, 01-04, 01-05, 01-06, phase-02]

actuals:
  tokens: 20500
  tasks: 3
  commits: 5

plan_head_before: 78fe65cca2f1c9e0700017f97590e3ff68edc7bf
plan_head_after: 190dcb0d42548626314005538b7fa81bde9a1501

tech-stack:
  added: []
  patterns:
    - "Claim-then-act delete: the staff_groups row is deleted first, so only the caller whose delete affected a row reports a removal"
    - "User-controlled text is spliced into a translated message after translation (sentinel token), because the translator runs a printf-style pass over interpolated text"
    - "Check order is documented and tested; each check lives in its own small method to keep gocyclo low"

key-files:
  created:
    - alita/modules/staff_unset_test.go
    - alita/modules/staff_setstaff_test.go
  modified:
    - alita/modules/staff.go
    - alita/db/staff/repository.go
    - alita/db/staff/repository_test.go
    - alita/utils/helpers/command_pipeline.go
    - alita/utils/helpers/command_pipeline_test.go
    - alita/utils/chat_status/owner.go
    - alita/utils/chat_status/owner_test.go
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml
    - locales/config.yml
    - docs/src/content/docs/commands/staff/index.md

key-decisions:
  - "DeleteStaffGroupWithLinks deletes the staff row before selecting and deleting its links, so a racing second Confirm sees zero rows affected and posts nothing"
  - "FetchBotMember maps a left or kicked status to Missing as well as the three 'bot is gone' error texts; only a definite answer is Missing, every other failure is Unknown"
  - "Group titles in the removal notice are inserted after translation, never through the translator"
  - "Only literal tr.GetString(\"staff_...\") calls are used so make check-translations can see every key"

patterns-established:
  - "Pipeline identity gate: RejectAnonymousSender() first in RequiredChecks, before anything that calls Telegram"
  - "Callback authority: chat from query.Message, presser checked live on every press, one answerCallbackQuery per press"

requirements-completed: [SETUP-01, SETUP-02, PLAT-03]

coverage:
  - id: D1
    description: "The live creator removes Staff status with /unsetstaff after a Confirm tap; the status and every link of that Staff Group are deleted in one transaction, the message lists the unlinked groups (escaped titles, link-id order) and their count, links of another Staff Group of the same owner are untouched"
    requirement: "SETUP-02"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -run '^TestUnsetStaffConfirmRemovesStatusAndLinks$' ./alita/modules#TestUnsetStaffConfirmRemovesStatusAndLinks"
        status: pass
      - kind: unit
        ref: "go test -tags testtools -race -run '^TestStaffRepoDeleteStaffGroupWithLinks' ./alita/db/staff#TestStaffRepoDeleteStaffGroupWithLinksOnlyTarget"
        status: pass
    human_judgment: false
  - id: D2
    description: "Only the live creator can remove Staff status: a recorded owner who is no longer creator is refused, a non-creator's Confirm gets an owner-only alert and changes nothing, an unverifiable press changes nothing, cancel keeps everything, an unknown action expires"
    requirement: "SETUP-02"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -run '^TestUnsetStaff' ./alita/modules#TestUnsetStaffRecordedOwnerWhoIsNoLongerCreatorIsRefused"
        status: pass
      - kind: unit
        ref: "go test -tags testtools -race -run '^TestUnsetStaff' ./alita/modules#TestUnsetStaffConfirmByNonCreatorGetsAlert"
        status: pass
    human_judgment: false
  - id: D3
    description: "Pressing Confirm twice removes the status once: one edit with the removal notice, the second press answered 'already removed'; a second repository delete returns deleted=false with nothing removed; zero links reports 0"
    requirement: "SETUP-02"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -run '^TestUnsetStaffDoubleConfirmRemovesOnce$' ./alita/modules#TestUnsetStaffDoubleConfirmRemovesOnce"
        status: pass
      - kind: unit
        ref: "go test -tags testtools -race -run '^TestStaffRepoDeleteStaffGroupWithLinksSecondCallIsNoop$' ./alita/db/staff#TestStaffRepoDeleteStaffGroupWithLinksSecondCallIsNoop"
        status: pass
    human_judgment: false
  - id: D4
    description: "Anonymous admins, channel identities and the service accounts 1087968824 and 777000 get the 'post as yourself' reply from /setstaff and /unsetstaff; the handler never runs and no Telegram call other than that one reply is made"
    requirement: "SETUP-01"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -run '^TestRejectAnonymousSender' ./alita/utils/helpers#TestRejectAnonymousSenderBlocksAnonymousAdmin"
        status: pass
      - kind: unit
        ref: "go test -tags testtools -race -run '^TestSetStaffRejectsAnonymousAdmin$' ./alita/modules#TestSetStaffRejectsAnonymousAdmin"
        status: pass
      - kind: unit
        ref: "go test -tags testtools -race -run '^TestUnsetStaffRejectsAnonymousAdmin$' ./alita/modules#TestUnsetStaffRejectsAnonymousAdmin"
        status: pass
    human_judgment: false
  - id: D5
    description: "Anonymous-admin delivery from a real Telegram client (Remain anonymous on, then off) gets the 'post as yourself' reply and no Staff Group, then succeeds when not anonymous"
    requirement: "SETUP-01"
    verification: []
    human_judgment: true
    rationale: "Anonymous-admin delivery comes from real Telegram clients and cannot be produced by the test fake (plan human-check, research A4). The tests build the same Message shape (SenderChat equal to the chat, From 1087968824) but only a real group proves Telegram sends it that way."
  - id: D6
    description: "/setstaff refuses channels, private chats, a bot that is not a live administrator, an unverifiable bot check and a chat that is a linked group, each with its own reason, in the fixed order anonymous, chat type, live creator, bot administrator, role conflict; a re-run reports the existing status and leaves one row; arguments are ignored"
    requirement: "SETUP-01"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -run '^TestSetStaff' ./alita/modules#TestSetStaffRefusalOrder"
        status: pass
      - kind: unit
        ref: "go test -tags testtools -race -run '^TestSetStaff' ./alita/modules#TestSetStaffRejectsBotNotAdmin"
        status: pass
      - kind: unit
        ref: "go test -tags testtools -race -run '^TestSetStaff' ./alita/modules#TestSetStaffAlreadyStaffIsIdempotent"
        status: pass
    human_judgment: false
  - id: D7
    description: "CreateStaffGroup refuses a chat that is currently a link's group (D-10) inside its transaction and writes nothing; FetchBotMember separates found, missing and unknown (flood wait, timeout) outcomes"
    requirement: "SETUP-01"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -run '^TestStaffRepoCreateStaffGroupRoleConflict$' ./alita/db/staff#TestStaffRepoCreateStaffGroupRoleConflict"
        status: pass
      - kind: unit
        ref: "go test -tags testtools -race -run '^TestFetchBotMember' ./alita/utils/chat_status#TestFetchBotMemberUnknown"
        status: pass
    human_judgment: false
  - id: D8
    description: "All 13 new staff_ keys and the extended help text exist in all 7 locales with identical placeholder sets; docs page regenerated without drift"
    requirement: "PLAT-03"
    verification:
      - kind: unit
        ref: "go test -tags testtools -run '^TestStaffLocaleKeys$' ./alita/i18n#TestStaffLocaleKeys"
        status: pass
      - kind: other
        ref: "make check-translations && make check-docs"
        status: pass
    human_judgment: false
  - id: D9
    description: "Removal commits atomically: after an interruption either the staff_groups row and all its links remain, or none do (single db.DB.Transaction)"
    requirement: "SETUP-02"
    verification: []
    human_judgment: true
    rationale: "Plan marks this truth verification: backstop. The transaction is exercised on SQLite for correctness of outcome, but a mid-transaction crash or PostgreSQL rollback is not simulated by any test."
  - id: D10
    description: "Two Confirm presses racing on two replicas remove the status once"
    requirement: "SETUP-02"
    verification: []
    human_judgment: true
    rationale: "Sequential double-press and the repository's second-call no-op are tested, and PostgreSQL row locking makes the second delete affect 0 rows, but no concurrent multi-replica run was executed (Postgres-gated tests were intentionally not run in this wave)."

duration: 50min
completed: 2026-10-04
status: complete
---

# Phase 1 Plan 02: /unsetstaff and the /setstaff Refusal Matrix Summary

**`/unsetstaff` removes a Staff Group and every link in one transaction behind a Confirm tap and a live creator check, and `/setstaff` now refuses anonymous or service senders, channels, a bot that is not admin, unverifiable lookups and linked groups, each with its own reason in a fixed order, built on a reusable `RejectAnonymousSender` check and a tri-state `FetchBotMember`.**

## Performance

- **Duration:** about 50 min (start time was not captured at spawn, so this is approximate)
- **Tasks:** 3 (1 tracer, 2 TDD expansion)
- **Commits:** 5 task commits (measured from the persisted ledger base `78fe65c`)
- **Files:** 18 changed, 1615 insertions, 24 deletions (measured against the base commit)

## Accomplishments

- Tracer slice end to end: `DeleteStaffGroupWithLinks` and `CountLinksByStaffFresh`, `/unsetstaff` with Confirm/Cancel, the `staff|` callback dispatcher, 7-locale strings, regenerated docs. Both tracer verify commands (nine `TestUnsetStaff*` tests, four `TestStaffRepoDeleteStaffGroupWithLinks*` tests) plus `make check-translations`, `make check-docs` and `TestStaffLocaleKeys` passed before expansion.
- Authority stays live: neither the command nor the Confirm press ever reads `owner_user_id` as authority. A recorded owner who lost ownership is refused, the new creator succeeds, a non-creator gets an owner-only alert, an unverifiable press changes nothing.
- Shared primitives for later plans: `helpers.IsAnonymousSender` / `RejectAnonymousSender` (Phase 2 STAFF-06 and plan 01-05 link flows) and `chat_status.FetchBotMember`.
- `/setstaff` refusal order is a documented, tested contract: anonymous, chat type, live creator, bot administrator, role conflict.
- D-10 is enforced inside the `CreateStaffGroup` transaction with `ErrRoleConflict`.
- Plan-level verification: `make test` exit 0, `go vet -tags testtools ./...` clean, golangci-lint v2.13.1 with `--new-from-rev` reports 0 issues, the dupl pass reports 0 issues, `make check-translations` and `make check-docs` pass, `go.mod` and `go.sum` unchanged, `go build ./...` clean.

## Task Commits

1. **Task 1 (tracer): /unsetstaff with confirm tap and transactional unlink** - `e90ffff` (feat)
2. **Task 2 RED: failing tests for RejectAnonymousSender and FetchBotMember** - `f615152` (test)
3. **Task 2 GREEN: implement RejectAnonymousSender and FetchBotMember** - `066f331` (feat)
4. **Task 3 RED: failing tests for the /setstaff refusal matrix and role conflict** - `6c7e295` (test)
5. **Task 3 GREEN: refusal matrix in fixed order plus the D-10 role check** - `190dcb0` (feat)

## TDD Gate Compliance

Both TDD tasks have a `test(01-02)` commit followed by a `feat(01-02)` commit. Each RED commit fails on the planned assertions, not on a build error:
- Task 2 RED used compile-only stubs with deliberately wrong behaviour (`IsAnonymousSender` returning false, `RejectAnonymousSender` returning true, `FetchBotMember` returning Found). 7 helpers tests and 11 chat_status tests failed on assertions; only `TestRejectAnonymousSenderAllowsRealUser` passed (it is correct under the stub by design).
- Task 3 RED failed on assertions because the new locale keys did not exist and `CreateStaffGroup` did not check roles (for example `replies = [], want exactly one staff_refuse_not_group`, and `CreateStaffGroup(linked group) = (true, <nil>), want (false, ErrRoleConflict)`). Three tests (private chat, bot admin without restrict rights, idempotent re-run) and one order case passed immediately because they characterise behaviour that already worked. `workflow.tdd_mode` was off, so no hard gate applied.
- The `ErrRoleConflict` variable declaration was included in the Task 3 RED commit so the tests compile; nothing returned it until GREEN.

## Decisions Made

See `key-decisions` above. In short: claim-then-act delete order, a stricter `FetchBotMember` Missing definition, translator-safe splicing of group titles, literal `GetString` keys only.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Translator printf pass would mangle group titles in the removal notice**
- **Found during:** Task 1
- **Issue:** The i18n interpolation runs a printf-style pass (`%s`, `%d`, ...) over the text after substituting `{groups}`. A linked group titled `100%d` (titles are user-controlled) would be consumed as a format verb and corrupt the notice, or be swapped for the count.
- **Fix:** `staff_unset_done` is translated with a sentinel token for `{groups}` and the escaped group list is spliced in afterwards. `TestUnsetStaffConfirmRemovesStatusAndLinks` uses the titles `Alpha <b>` and `100%d` and asserts both survive.
- **Files modified:** alita/modules/staff.go, alita/modules/staff_unset_test.go
- **Commit:** e90ffff

**2. [Rule 2 - Missing critical] Typo'd i18n keys would have been invisible to make check-translations**
- **Found during:** Task 1 acceptance check
- **Issue:** My first draft used a `trS(tr, "key")` helper. `make check-translations` only sees literal `GetString("...")` calls, so a mistyped key would have shipped as an empty reply (AGENTS.md anti-pattern, and plan requirement "use only literal GetString calls").
- **Fix:** Replaced every `trS` use in staff.go with literal `tr.GetString("staff_...")` calls before the first commit.
- **Files modified:** alita/modules/staff.go
- **Commit:** e90ffff

**3. [Rule 2 - Missing critical] Race-safe delete order**
- **Found during:** Task 1
- **Issue:** The plan listed select links, delete links, delete the staff row. With two racing confirms, both could read the links before either commits, and the loser would still hold a non-empty list.
- **Fix:** `DeleteStaffGroupWithLinks` deletes the staff row first (claiming it), and only on `RowsAffected == 1` selects and deletes the links. The loser gets `(nil, false, nil)`. Same transaction, same outcome for a single caller.
- **Files modified:** alita/db/staff/repository.go
- **Commit:** e90ffff

**4. [Rule 2 - Missing critical] Extra tests beyond the plan's list**
- **Found during:** Tasks 1 and 3
- **Issue:** Threat T-01-05 covers `/unsetstaff` as well as `/setstaff`, but the plan only listed an anonymous-admin test for `/setstaff`; the `FetchBotMember` left/kicked status mapping, a failing live check on Confirm, an unknown callback action, and a bot that is admin without restrict rights (must be accepted, D-08 only needs admin) had no tests.
- **Fix:** Added `TestUnsetStaffRejectsAnonymousAdmin`, `TestUnsetStaffConfirmWithFailingOwnerCheckChangesNothing`, `TestUnsetStaffUnknownActionExpires`, `TestFetchBotMemberMissingWhenStatusLeftOrKicked`, `TestFetchBotMemberTimeoutIsUnknown`, `TestSetStaffAcceptsBotAdminWithoutRestrictRights`, `TestSetStaffPrivateChatGetsGroupOnlyReply`, `TestSetStaffRejectsServiceAccountAndMissingUser`.
- **Commits:** e90ffff, f615152, 6c7e295, 190dcb0

### Observation (not changed)

**A sender with no identifiable user in the live pipeline is stopped by `BuildCommandContext`**, which replies `common_cannot_identify_user` before `RejectAnonymousSender` runs. The plan's truth says such a sender gets "post as yourself". `RejectAnonymousSender` itself does return false and send "post as yourself" for a nil user (tested directly on a `CommandContext`), but through `WrapCommand` the earlier, pre-existing reply wins. Nothing else runs and no Telegram lookup happens, so the safety property holds. I did not change `BuildCommandContext` because every command shares it. `TestSetStaffRejectsServiceAccountAndMissingUser` accepts either refusal for the no-user case.

**Total deviations:** 4 auto-fixed (1 bug, 3 missing-critical), 1 observation. **Impact:** none on scope; extra tests and a safer delete order.

## Authentication Gates

None.

## Known Stubs

None. The panel's empty keyboard from plan 01-01 is unchanged and still intentional.

## Threat Flags

None beyond the plan's threat model. T-01-05 (anonymous senders, tested for both commands), T-01-06 (callback chat from the message, live presser check, tested), T-01-07 (claim-then-act delete, sequential double press tested, concurrent run is a human-judgment item D10) and T-01-08 (in-transaction `ErrRoleConflict`; the cross-replica trigger is plan 01-04's) each have a mitigation in place. No package installs (`go.mod` and `go.sum` unchanged).

## Issues Encountered

- `gofmt -l` still reports `alita/modules/greetings_command_test.go`, a pre-existing file outside this plan's scope; not touched.
- The Task 3 human check (real anonymous-admin message from a Telegram client) was not run; recorded as coverage item D5 for the verifier.
- Postgres-gated tests were not run (plan 01-04 was using the database); this plan's tests all run on SQLite.

## Next Phase Readiness

Plan 01-05 (link flows) can call `helpers.IsAnonymousSender` directly and reuse `FetchBotMember` and the `staff|` callback dispatcher (add actions as new methods, `staffCallback` stays under the complexity limit). Plan 01-04's PostgreSQL trigger complements the in-transaction `ErrRoleConflict` check without changing behaviour. Phase 2 can put `helpers.RejectAnonymousSender()` first in its RequiredChecks for STAFF-06.

## Self-Check: PASSED

- All created files exist on disk (`staff_unset_test.go`, `staff_setstaff_test.go`) and modified files are tracked.
- Commits e90ffff, f615152, 066f331, 6c7e295, 190dcb0 are on the branch; `git rev-list --count` from the ledger base gives 5.
- Acceptance criteria of all three tasks re-run and passing; plan-level verification (`make test`, vet, lint, dupl, check-translations, check-docs, locale parity, go.mod/go.sum) passing.
