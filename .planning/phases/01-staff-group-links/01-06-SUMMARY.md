---
phase: 01-staff-group-links
plan: 06
subsystem: staff-groups
tags: [go, gotgbot, gorm, i18n, telegram, callbacks, unlink, tdd]

requires:
  - phase: 01-staff-group-links
    provides: "staff| callback dispatcher, CheckOwner, IsAnonymousSender, sendStaffNotice, replySelfDeleting, staffChatTranslator, ListLinksByStaffFresh, GetLinkOfGroupFresh, renderStaffPanel, the staffBotClient harness, the PostgreSQL role-exclusivity trigger"
provides:
  - "runUnlinkGroup: the one live-checked unlink flow behind /unlinkstaff and the Unlink button (live creator of the Staff Group, then of the group, then a claim-by-delete)"
  - "/unlinkstaff: deletes the command message, answers refusals before Staff Group proof in the issuing group (self-deleting), posts later refusals and the 'unlinked by' notice in the Staff Group only"
  - "Per-link Unlink buttons in /staff with an 'Are you sure?' Confirm/Cancel step (callback actions ul / uc / ux), authority re-checked live on every press"
  - "staff.GetLinkByIDFresh and staff.DeleteLink (one row, RETURNING, cache invalidated for the group)"
  - "buildStaffPanel (shared by /staff and the in-place re-render), staffRerenderPanel, staffButtonTitle, staffUnlinkRefusalText"
affects: [01-07, 01-08, 01-09, 01-10, phase-02]

estimate:
  tokens: 70000
  raw_tokens: 70000
  tasks: 2
  confidence: low

actuals:
  tokens: 15500
  tasks: 2
  commits: 3

plan_head_before: f02ee6d464c4cdc5137c6c3a4fa04aca81244ed1
plan_head_after: 39d61c3c36a1cd194abb5740a01fff9c208d8263

tech-stack:
  added: []
  patterns:
    - "Claim-by-delete with RETURNING: the DELETE both claims the row and returns its group_chat_id, so only the caller whose delete affected a row posts the notice and the cache invalidation uses the exact key"
    - "Every button press loads its link fresh by row ID, requires the message chat to equal the link's Staff Group, then re-checks the presser live on both chats; nothing in the callback data is trusted"
    - "User-controlled text (group title, issuer display name) is spliced into translated text after translation via sentinel tokens, in notices, prompts and button labels"
    - "Refusal destination is a function of one fact (has the issuer been proven the Staff Group's live creator): before it, self-deleting in-place reply; after it, Staff Group notice with self-deleting fallback"

key-files:
  created:
    - alita/modules/staff_unlink.go
    - alita/modules/staff_unlink_test.go
    - alita/modules/staff_unlink_button_test.go
  modified:
    - alita/modules/staff.go
    - alita/modules/staff_panel.go
    - alita/db/staff/repository.go
    - alita/db/staff/repository_test.go
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
  - "Cancel needs the same live creator-of-both authority as Ask and Confirm, so a bystander in the Staff Group cannot dismiss the creator's confirm prompt"
  - "DeleteLink deletes with RETURNING instead of read-then-delete: it avoids the SQLite read-to-write lock upgrade that failed under contention in 01-05 and gives the exact group key to invalidate"
  - "A refusal after the Staff Group creator is proven falls back to a self-deleting reply in the issuing group when the Staff Group post fails (same rule as 01-05 for linking); a success notice never falls back"
  - "A new key staff_unlink_check_failed_group ('Not unlinked: ...') is used instead of reusing staff_link_check_failed_group, whose text says 'Not linked'"
  - "The 'unlinked by' mention passes the raw first name to formatting.MentionHtml, which escapes it once; the plan's html.EscapeString would double-escape"

patterns-established:
  - "Panel keyboard: row 0 is the Add group URL button, then one Unlink row per link; later plans append rows below"
  - "In-place panel refresh returns errStaffPanelGone (nothing touched) when the message is missing or its chat is no longer a Staff Group; the caller answers expired"

requirements-completed: [SETUP-05, PLAT-03]

coverage:
  - id: D1
    description: "/unlinkstaff in a linked group, sent by the live creator of both that group and its Staff Group, deletes that one link, tries to delete the command message, posts one 'unlinked by' notice (escaped title, mention of the issuer) in the Staff Group and sends nothing to the linked group; other links of the same Staff Group are untouched"
    requirement: "SETUP-05"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -run '^TestUnlinkStaffTracer$' ./alita/modules#TestUnlinkStaffTracer"
        status: pass
    human_judgment: false
  - id: D2
    description: "/unlinkstaff refusals: not linked, a stranger, a Staff Group whose live creator is someone else, an unverifiable Staff Group check and an anonymous sender all get a self-deleting in-place reply and nothing in the Staff Group; a Staff Group creator who no longer owns the group, or an unverifiable group check, is told in the Staff Group only (with a self-deleting fallback when that post fails); the link remains in every refusal"
    requirement: "SETUP-05"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -run '^TestUnlinkStaffRefusals$' ./alita/modules#TestUnlinkStaffRefusals"
        status: pass
      - kind: unit
        ref: "go test -tags testtools -race -run '^TestUnlinkStaffStrangerNeverPostsInStaffGroup$' ./alita/modules#TestUnlinkStaffStrangerNeverPostsInStaffGroup"
        status: pass
    human_judgment: false
  - id: D3
    description: "Each linked group in /staff has an Unlink button (callback staff|v1|a=ul&l=<link id>, at most 64 bytes, plain title capped at 20 runes); Unlink asks 'Are you sure?' with Confirm and Cancel, Confirm by the live creator of both groups deletes that one link, re-renders the panel in place and posts the notice to the Staff Group, Cancel restores the panel"
    requirement: "SETUP-05"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -run '^TestStaffUnlinkButtonRendered$|^TestStaffUnlinkButtonConfirmFlow$|^TestStaffUnlinkButtonCancel$|^TestStaffUnlinkButtonOnlyNamedLinkGoes$' ./alita/modules"
        status: pass
    human_judgment: false
  - id: D4
    description: "Only the live creator of both groups can use the buttons, at every press: a stranger, the creator of only one of the two groups, and a recorded owner whose ownership moved between Unlink and Confirm get an owner-only alert and nothing changes; a Telegram error during the check gives a could-not-verify alert and nothing changes"
    requirement: "SETUP-05"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -run '^TestStaffUnlinkButtonStranger$|^TestStaffUnlinkButtonOnlyCreatorOfBothGroups$|^TestStaffUnlinkButtonCheckFailure$|^TestStaffUnlinkButtonOwnerChangedBetweenPresses$' ./alita/modules"
        status: pass
    human_judgment: false
  - id: D5
    description: "Forged or stale presses are harmless: a callback whose message chat is not the link's Staff Group is denied before any Telegram lookup; a deleted link, a lost race (second Confirm) and malformed link IDs (empty, text, signed, float, overflow) are answered 'expired' and the panel is refreshed; the notice is posted once"
    requirement: "SETUP-05"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -run '^TestStaffUnlinkButtonWrongChat$|^TestStaffUnlinkButtonExpired$|^TestStaffUnlinkButtonBadLinkID$|^TestStaffUnlinkButtonConfirmTwiceRemovesOnce$' ./alita/modules"
        status: pass
    human_judgment: false
  - id: D6
    description: "DeleteLink removes only the named row, a second call or an unknown ID returns deleted=false, four concurrent callers produce exactly one winner, the cached link-of-group lookup is invalidated; GetLinkByIDFresh returns (nil, nil) for a missing row; all of it also passes on PostgreSQL with the migration chain and the exclusivity trigger"
    requirement: "SETUP-05"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -run '^TestStaffRepoDeleteLink|^TestStaffRepoGetLinkByIDFresh' ./alita/db/staff"
        status: pass
      - kind: integration
        ref: "ALITA_TEST_DATABASE=true go test -tags testtools -race -p 1 ./alita/db/staff (fresh database with the migration chain applied)"
        status: pass
    human_judgment: false
  - id: D7
    description: "All 8 new staff_ keys (plus the extended help text and the unlinkstaff alt name) exist in all 7 locales with identical placeholder sets; docs regenerated without drift"
    requirement: "PLAT-03"
    verification:
      - kind: unit
        ref: "go test -tags testtools -run '^TestStaffLocaleKeys$' ./alita/i18n#TestStaffLocaleKeys"
        status: pass
      - kind: other
        ref: "make check-translations && make check-docs"
        status: pass
    human_judgment: false
  - id: D8
    description: "In a real Telegram client the linked group's members see no trace of an unlink: /unlinkstaff is deleted, the self-deleting refusals disappear after 30 seconds, and the Unlink buttons only appear in the Staff Group's /staff reply"
    requirement: "SETUP-05"
    verification: []
    human_judgment: true
    rationale: "Telegram delivery (whether the bot may delete the command message, how an owner's anonymous posting appears, inline keyboards in a live Staff Group) cannot be reproduced by the test fake. The tests assert the calls the bot makes, not what a client shows."

duration: 14min
completed: 2026-10-04
status: complete
---

# Phase 1 Plan 06: Unlink a Group with /unlinkstaff or the Unlink Button Summary

**One live-checked `runUnlinkGroup` flow now removes a single link from `/unlinkstaff` or from a per-group Unlink button (with an "Are you sure?" step) in `/staff`; both require the live creator of the group and of its Staff Group at every press, delete exactly one row by claim-by-delete, tell only the Staff Group, and leave nothing in the linked group.**

## Performance

- **Duration:** about 14 min (2026-10-04T22:44:53Z to 22:58:54Z)
- **Tasks:** 2 (1 tracer, 1 TDD expansion)
- **Commits:** 3 task commits (measured from the persisted ledger base `f02ee6d`)
- **Files:** 16 changed, 1467 insertions, 23 deletions (measured against the base commit)

## Accomplishments

- Tracer slice end to end: `GetLinkByIDFresh` and `DeleteLink`, `runUnlinkGroup`, `/unlinkstaff`, the Staff Group notice, 7-locale strings, help text, alt name and regenerated docs. The tracer verify (`TestUnlinkStaff*`, `make check-translations`, `make check-docs`, `TestStaffLocaleKeys`) passed before expansion (tracer verified end to end, expanding).
- Unlink buttons: `renderStaffPanel` adds one button per link under the Add group row. `ul` swaps the panel for a prompt, `uc` deletes the link, re-renders in place and posts the notice, `ux` restores the panel. Each press loads the link by row ID, requires the message chat to be the link's Staff Group, then runs a live creator-of-both check, so ownership that moved between Unlink and Confirm is caught.
- Harmless stale or forged presses: a wrong-chat press is denied before any Telegram call; a deleted link, a lost race and malformed IDs are answered "expired" and the panel is refreshed; the "unlinked by" notice is posted by the one caller whose delete hit a row.
- Plan-level verification: `make test` exit 0 (run twice), `go vet -tags testtools ./...` clean, golangci-lint v2.13.1 `--new-from-rev` 0 issues and the dupl pass 0 issues, `make check-translations` and `make check-docs` pass, `go build ./...` clean, `go.mod` and `go.sum` unchanged, `gofmt -l` empty on the changed files. On a throwaway PostgreSQL 16 database: `TestRepositoryMigrationChain`, `ALITA_TEST_DATABASE=true go test ./alita/db/staff` and `make test-postgres-integrity` pass (database dropped afterwards; `alita_test` untouched).

## Task Commits

1. **Task 1 (tracer): /unlinkstaff removes one link and tells only the Staff Group** - `6aaee7f` (feat)
2. **Task 2 RED: failing tests for the Unlink button, confirm step and live re-checks** - `3bb9288` (test)
3. **Task 2 GREEN: per-group Unlink buttons with a confirm step and live re-checks** - `39d61c3` (feat)

## TDD Gate Compliance

Task 2 has a `test(01-06)` commit followed by a `feat(01-06)` commit. `workflow.tdd_mode` was off, so no hard gate applied; recorded for transparency.

- **RED (`3bb9288`)** failed on the planned assertions, not on a build error: every `TestStaffUnlinkButton*` test except `TestStaffUnlinkButtonBadLinkID` failed (for example `edits = [], want the panel re-rendered`, `answer = ("@@staff-cb-expired@@", alert=false), want an alert with staff_cb_owner_only`, `the first press made 0 edits, want the prompt`, `staffButtonTitle ... got ""`). To compile, the RED commit carried the three action constants and a `staffButtonTitle` stub returning `""`. `TestStaffUnlinkButtonBadLinkID` passed immediately because the old default branch already answered "expired" (characterisation of a requirement that holds both before and after).
- The RED commit's keyboard assertions used `a=ul|l=<id>`, but the codec joins fields with `&`; the GREEN commit corrects the test separator. The behavioural assertions that drove the implementation were unaffected.
- The `TestStaffRepoDeleteLink*` and `TestStaffRepoGetLinkByIDFresh` tests were committed with the Task 1 repository code they test, not in Task 2's RED, so they do not appear as a RED failure.
- No REFACTOR commits.

## Decisions Made

See `key-decisions` above. In short: Cancel is authority-gated; DeleteLink is delete-with-RETURNING; a refusal after Staff Group proof falls back in place only when the Staff Group post fails; the check-failed refusal has its own "Not unlinked" text; the mention is escaped once.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Wrong wording for an unlink that could not be verified**
- **Found during:** Task 1 (mapping `staffUnlinkCheckFailed` to text)
- **Issue:** The plan reuses `staff_link_check_failed_group` for both an unverifiable group check and a failed delete. That text begins "Not linked: ...", which in the unlink flow would tell the Staff Group the opposite of what happened.
- **Fix:** Added `staff_unlink_check_failed_group` ("Not unlinked: I could not verify who created {group} ... Nothing was changed") in all 7 locales and used it for both cases.
- **Files modified:** alita/modules/staff_unlink.go, locales/*.yml
- **Commit:** 6aaee7f

**2. [Rule 1 - Bug] Plan's `html.EscapeString(issuer.FirstName)` would double-escape the mention**
- **Found during:** Task 1
- **Issue:** `formatting.MentionHtml` already HTML-escapes the name, so passing a pre-escaped name would show `&amp;lt;` for a name containing `<`.
- **Fix:** Pass the raw first name; `TestUnlinkStaffTracer` uses the name `Sender <i>` and asserts it appears escaped exactly once. Both the title and the mention are spliced in after translation because the translator runs a printf-style pass.
- **Files modified:** alita/modules/staff_unlink.go
- **Commit:** 6aaee7f

**3. [Rule 2 - Missing critical] Cancel is authority-gated**
- **Found during:** Task 2 design
- **Issue:** The plan's behaviour list tests only Unlink and Confirm for strangers. An ungated Cancel would let any Staff Group member dismiss the creator's confirm prompt and trigger a panel edit.
- **Fix:** `staffUnlinkCancel` runs the same load-link, same-chat and live creator-of-both checks as the other two. `TestStaffUnlinkButtonStranger` covers all three actions; the wrong-chat and expired tests cover all three too.
- **Files modified:** alita/modules/staff_unlink.go
- **Commit:** 39d61c3

**4. [Rule 2 - Missing critical] Refusals after Staff Group proof fall back in place when the Staff Group post fails**
- **Found during:** Task 1
- **Issue:** The plan sends those refusals to the Staff Group only; if the post fails the verified creator would get no answer at all.
- **Fix:** Same rule as 01-05: fall back to a self-deleting reply in the issuing group (refusals only; the success notice never falls back, only logged). Covered by a subtest.
- **Files modified:** alita/modules/staff_unlink.go
- **Commit:** 6aaee7f

**5. [Rule 2 - Missing critical] Extra tests beyond the plan's list**
- **Found during:** Tasks 1 and 2
- **Fix:** `TestUnlinkStaffStrangerNeverPostsInStaffGroup`, `TestStaffUnlinkRefusalTextSplicesTitleAfterTranslation`, `TestStaffUnlinkButtonTitle`, `TestStaffUnlinkButtonRenderedByStaffCommand`, `TestStaffUnlinkButtonConfirmTwiceRemovesOnce`, `TestStaffUnlinkButtonOnlyCreatorOfBothGroups`, `TestStaffUnlinkButtonCheckFailure`, `TestStaffUnlinkButtonBadLinkID`, `TestStaffUnlinkButtonOnlyNamedLinkGoes`, and `TestStaffRepoDeleteLinkConcurrentOneWinner` / `TestStaffRepoGetLinkByIDFresh` in the repository.
- **Commits:** 6aaee7f, 3bb9288

### Small choices where the plan was silent or differed

- `unlinkStaffDesc` lives in `staff_unlink.go` next to its handler (as `linkStaffDesc` does in `staff_link.go`); the `WrapCommand` registration is in `staff.go` as planned.
- `staffRerenderPanel(b, msg, tr)` takes the translator as a third parameter and returns `errStaffPanelGone` rather than answering the callback itself, so each caller answers exactly once. It treats Telegram's "message is not modified" as success.
- `staffCallback` passes `decoded.Fields` to the three unlink methods, which keeps it a thin switch.
- `/staff` and the in-place re-render share one `buildStaffPanel(tr, group, botUsername)`; `staffPanel` in `staff.go` was reduced to use it (the dupl linter would otherwise flag the copy).
- `DeleteLink` uses `DELETE ... RETURNING` rather than "load the row's group_chat_id inside the transaction, then delete"; same outcome, no read-to-write lock upgrade.
- Panel keyboard shape is Add group (row 0, only when the bot has a username) plus one Unlink button per row; with the planned layout later plans add Refresh and paging below.

**Total deviations:** 5 auto-fixed (2 bugs, 3 missing-critical), 6 small choices. **Impact:** no scope change; one extra locale key (8 new keys instead of 7), and a stricter Cancel.

## Authentication Gates

None.

## Known Stubs

None. The compile-only RED stubs (`staffButtonTitle`) were replaced in the GREEN commit.

## Threat Flags

None beyond the plan's threat model. T-01-19 (forged link ID or replay in another chat: link loaded fresh, message chat must equal `link.StaffChatID`, denied before any lookup, `TestStaffUnlinkButtonWrongChat`, `TestStaffUnlinkButtonExpired`, `TestStaffUnlinkButtonBadLinkID`), T-01-20 (non-owner or ownership moved between presses: `CheckOwner` on both chats at Ask, Confirm and Cancel, `TestStaffUnlinkButtonOwnerChangedBetweenPresses`, `TestStaffUnlinkButtonOnlyCreatorOfBothGroups`) and T-01-21 (notice spam: refusals before Staff Group proof are self-deleting in-place replies, the notice is posted only by the caller whose delete affected a row, `TestUnlinkStaffStrangerNeverPostsInStaffGroup`, `TestStaffUnlinkButtonConfirmTwiceRemovesOnce`, `TestStaffRepoDeleteLinkConcurrentOneWinner`) each have a mitigation and a passing test. T-01-SC: `go.mod` and `go.sum` unchanged.

One residual to note for the verifier, not a new surface: in a linked group `/unlinkstaff` from a stranger answers "only the creator ... can unlink it" while in an unlinked group it answers "not linked", so a member who tries the command can learn whether the group is linked. This follows the plan's explicit wording (the not-linked reply is a stated truth); hiding it would need a creator-of-this-group check before the link lookup, which conflicts with the plan's "Staff Group creator who no longer owns the group is told in the Staff Group" behaviour. Both replies delete themselves after 30 seconds.

## Issues Encountered

- The human check (real Telegram client, coverage item D8) was not run; the fake client cannot show what members of a linked group see.
- `gofmt -l` still reports `alita/modules/greetings_command_test.go`, a pre-existing file outside this plan; not touched.
- The module tests were not run against PostgreSQL (pre-existing: the modules test setup calls AutoMigrate, which fails on a migration-chain schema); the repository tests, which are what touch PostgreSQL behaviour here, ran against the chain database.
- `.planning/WINDOWS.md` does not exist, so no broken-windows entries were appended; there are no stubs or skipped tests to record.

## Next Phase Readiness

Plans 01-07 to 01-10 can build on `renderStaffPanel` (keyboard rows: Add group, then one Unlink row per link), `buildStaffPanel` and `staffRerenderPanel` for refresh and paging buttons, and on `GetLinkByIDFresh` / `DeleteLink` for the recheck sweep (the sweep must call `DeleteLink` and post nothing if `deleted` is false). Every later plan that adds `staff_` keys must re-run `TestStaffLocaleKeys`.

## Self-Check: PASSED

- All created files exist on disk (`staff_unlink.go`, `staff_unlink_test.go`, `staff_unlink_button_test.go`) and modified files are tracked; the working tree was clean before this SUMMARY.
- Commits 6aaee7f, 3bb9288 and 39d61c3 are on the branch; `git rev-list --count` from the ledger base gives 3.
- Acceptance criteria of both tasks re-run and passing (greps for `WrapCommand`, `DeleteLink` signature, no `IsUserAdmin`/`LoadAdminCache`/`RequireUserOwner`, no non-literal `GetString`, `staffActUnlink` count 6, 37 passing `TestStaffUnlinkButton*` results, `TestStaffLocaleKeys`, `make check-translations`, `make check-docs`); plan-level verification (`make test`, vet, lint, dupl, go.mod/go.sum, PostgreSQL chain) passing.
