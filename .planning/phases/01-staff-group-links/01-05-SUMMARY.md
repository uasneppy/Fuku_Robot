---
phase: 01-staff-group-links
plan: 05
subsystem: staff-groups
tags: [go, gotgbot, gorm, i18n, telegram, deep-link, tdd]

requires:
  - phase: 01-staff-group-links
    provides: "staff_groups and staff_group_links tables, CheckOwner, FetchBotMember, RejectAnonymousSender/IsAnonymousSender, staffBotClient harness, the staff| callback dispatcher, the PostgreSQL role-exclusivity trigger"
provides:
  - "runLinkGroup: the one live-checked link flow behind /linkstaff and the group picker (anonymous sender, named or sole verified Staff Group, supergroup, not a Staff Group, live creator of both chats, bot health, insert)"
  - "/linkstaff [id] command and the Add group URL button in /staff (t.me startgroup picker asking for restrict_members + delete_messages)"
  - "Group /start payload route: RegisterGroupDeepLinkHandler / HandleGroupDeepLink, help.go start() group branch, stf_<digits> handler"
  - "staff.CreateLink (ErrAlreadyLinked, ErrStaffGroupMissing, ErrRoleConflict), ListLinksByStaffFresh, GetLinkOfGroup, GetLinkOfGroupFresh, ListStaffGroupsByOwner"
  - "staffChatTranslator, sendStaffNotice, replySelfDeleting (staffSelfDeleteAfter = 30s), staffLinkRefusalText"
  - "/staff lists linked groups under a count header"
affects: [01-06, 01-07, 01-08, 01-09, 01-10, phase-02]

actuals:
  tokens: 25500
  tasks: 3
  commits: 6

plan_head_before: a1aa732b01de98e30b3cf7d4c4dde3363623acba
plan_head_after: 774371e3f785561252fd7c672e323beac18f8eb0

tech-stack:
  added: []
  patterns:
    - "Refusal destination is a function of one fact: before the issuer is verified as the live creator of the named Staff Group the refusal is a self-deleting reply in the issuing group; after it, it is posted in the Staff Group (in the Staff Group's language)"
    - "User-controlled group titles are spliced into a translated message after translation (sentinel token), because the translator runs a printf-style pass"
    - "Insert-first transaction with the role checks repeated after it, plus the same checks before the transaction for a precise refusal reason under the PostgreSQL trigger"
    - "Group deep-link registry with longest-prefix match; a handler returns handled=false for payloads it does not recognise so /start falls back unchanged"

key-files:
  created:
    - alita/modules/staff_link.go
    - alita/modules/staff_notify.go
    - alita/modules/staff_link_test.go
    - alita/modules/staff_link_refusals_test.go
    - alita/modules/staff_picker_test.go
  modified:
    - alita/modules/staff.go
    - alita/modules/staff_panel.go
    - alita/modules/deeplink_router.go
    - alita/modules/help.go
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
  - "A refusal is routed by whether the issuer has been proven the live creator of the Staff Group they named, never by the wording of the case; a stranger gets one uniform text whether or not the ID is a Staff Group, and nothing is ever sent to the named chat"
  - "A successful link is never posted in the linked group, even when the Staff Group notice fails (logged only); the plan's self-deleting fallback applies to refusals only"
  - "/linkstaff without an argument fails closed when any recorded candidate cannot be verified, because it might be a second match; several verified matches ask for an explicit ID"
  - "CreateLink runs its role checks before the transaction as well as after the insert inside it, so the caller keeps the precise reason under the PostgreSQL trigger and concurrent writers take the write lock first"
  - "The picker payload is stf_ plus the decimal of the negated Staff Group chat ID (at most 23 characters, charset A-Za-z0-9_-); it names a Staff Group and grants nothing"

patterns-established:
  - "Pipeline-free commands that need a self-deleting refusal do the anonymous check inside the core, not as a RequiredChecks entry"
  - "Tests assert destinations by chat: callsToChat / textsToChat on the staffBotClient, with markers from withStaffLocale"

requirements-completed: [SETUP-03, SETUP-04, PLAT-03]

coverage:
  - id: D1
    description: "/linkstaff <id> from the live creator of both a supergroup and the named Staff Group creates a link owned by that user, deletes the command message from the linked group, posts one confirmation in the Staff Group and nothing in the linked group; /staff then lists the group (escaped title, chat ID, count header)"
    requirement: "SETUP-03"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -run '^TestLinkStaffTracer$' ./alita/modules#TestLinkStaffTracer"
        status: pass
    human_judgment: false
  - id: D2
    description: "When the bot is not an admin, cannot restrict, or is missing in the target group, the link is still created with health bot_not_admin / bot_cannot_restrict / bot_missing and the Staff Group notice carries exactly one matching warning line (D-04)"
    requirement: "SETUP-03"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -run '^TestLinkStaffWarns' ./alita/modules#TestLinkStaffWarnsWhenBotLacksRights"
        status: pass
      - kind: unit
        ref: "go test -tags testtools -race -run '^TestStaffHealthFromBot$' ./alita/modules#TestStaffHealthFromBot"
        status: pass
    human_judgment: false
  - id: D3
    description: "Every D-03 refusal states its reason in the right place: invalid ID, stranger and non-Staff-Group IDs (one identical text, nothing sent to the named chat), a recorded owner who is no longer creator, an unverifiable Staff Group check, all as self-deleting replies in the issuing group; basic group, target is a Staff Group (including itself), not the target's creator, already linked and an unverifiable target check, all posted in the Staff Group only"
    requirement: "SETUP-04"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -run '^TestLinkStaffRefusals' ./alita/modules#TestLinkStaffRefusalsUniformForStranger"
        status: pass
      - kind: unit
        ref: "go test -tags testtools -race -run '^TestLinkStaffRefusals' ./alita/modules#TestLinkStaffRefusalsToStaffGroup"
        status: pass
      - kind: unit
        ref: "go test -tags testtools -race -run '^TestLinkStaffSelfDeletingReply$' ./alita/modules#TestLinkStaffSelfDeletingReply"
        status: pass
    human_judgment: false
  - id: D4
    description: "Anonymous admins, channel identities and Telegram service accounts using /linkstaff or the picker get a self-deleting 'post as yourself' reply, with no getChatAdministrators or getChatMember call and no row"
    requirement: "SETUP-04"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -run '^TestLinkStaffRefusalsAnonymous$' ./alita/modules#TestLinkStaffRefusalsAnonymous"
        status: pass
      - kind: unit
        ref: "go test -tags testtools -race -run '^TestStaffPickerRefusesAnonymous$' ./alita/modules#TestStaffPickerRefusesAnonymous"
        status: pass
    human_judgment: false
  - id: D5
    description: "/linkstaff without an argument links to the issuer's only live-verified Staff Group; none gives 'you don't own a Staff Group'; several verified give the 'use /linkstaff <ID>' hint without picking by order; a recorded Staff Group whose live creator changed is not a candidate; an unverifiable candidate fails closed"
    requirement: "SETUP-03"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -run '^TestLinkStaffNoArgument$' ./alita/modules#TestLinkStaffNoArgument"
        status: pass
    human_judgment: false
  - id: D6
    description: "Concurrent link attempts for the same group (same or different Staff Groups) leave exactly one row: one nil error and one ErrAlreadyLinked, on SQLite and on PostgreSQL with the exclusivity trigger; CreateLink reports ErrStaffGroupMissing and ErrRoleConflict precisely"
    requirement: "SETUP-04"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -run '^TestStaffRepoCreateLink' ./alita/db/staff#TestStaffRepoCreateLinkConcurrentSameGroupMakesOneRow"
        status: pass
      - kind: integration
        ref: "ALITA_TEST_DATABASE=true go test -tags testtools -race -p 1 ./alita/db/staff (fresh database with the migration chain applied)"
        status: pass
    human_judgment: false
  - id: D7
    description: "The Add group button is a URL button https://t.me/<bot>?startgroup=stf_<digits>&admin=restrict_members+delete_messages whose payload is at most 64 characters of A-Za-z0-9_-; /start@bot stf_<digits> in a group deletes the message and runs the same live-checked flow as /linkstaff; junk payloads (empty, non-digit, negative, over-long) keep the old help_pm_questions reply and create no link"
    requirement: "SETUP-03"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -run '^TestStaffPicker|^TestStartGroupPayloadIgnoresJunk$|^TestStaffPanelHasAddGroupButton$|^TestStartCommandRepliesInPrivateAndGroup$' ./alita/modules#TestStaffPickerLinksGroup"
        status: pass
      - kind: unit
        ref: "go test -tags testtools -race -run '^TestStaffPickerRefusesStranger$' ./alita/modules#TestStaffPickerRefusesStranger"
        status: pass
    human_judgment: false
  - id: D8
    description: "Linking leaves no trace in the linked group: no link confirmation is ever posted there, including when the Staff Group notice fails; a refusal after verification falls back to a self-deleting reply in the issuing group only when the Staff Group post fails"
    requirement: "SETUP-04"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -run '^TestLinkStaffNoticeFailure$' ./alita/modules#TestLinkStaffNoticeFailure"
        status: pass
    human_judgment: false
  - id: D9
    description: "All 15 new staff_ keys (plus the extended help text) exist in all 7 locales with identical placeholder sets; docs regenerated without drift; /linkstaff added to the Staff alt names"
    requirement: "PLAT-03"
    verification:
      - kind: unit
        ref: "go test -tags testtools -run '^TestStaffLocaleKeys$' ./alita/i18n#TestStaffLocaleKeys"
        status: pass
      - kind: other
        ref: "make check-translations && make check-docs"
        status: pass
    human_judgment: false
  - id: D10
    description: "In a real Telegram client the Add group picker accepts a supergroup the bot is already in and one it is not in, keeps the bot's existing admin rights (combined with restrict + delete), delivers /start@bot stf_... with the owner as sender, and an owner with Remain anonymous on gets a self-deleting 'post as yourself' and no link"
    requirement: "SETUP-03"
    verification: []
    human_judgment: true
    rationale: "Plan Task 3 human-check (research A3, A12, A4). The group picker, the rights-combination dialog and anonymous posting happen in real Telegram clients and the test fake cannot reproduce them. The tests build the same message shapes (From the owner; SenderChat equal to the chat with From 1087968824) but only a real group proves Telegram sends them that way. /linkstaff is the fallback if the picker misbehaves. Not run in this environment."
  - id: D11
    description: "A link with an unknown bot status (the bot-membership lookup itself failed) is stored with health ok and no warning until the live panel or sweep corrects it"
    requirement: "SETUP-03"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -run '^TestStaffHealthFromBot$' ./alita/modules#TestStaffHealthFromBot"
        status: pass
    human_judgment: false

duration: 22min
completed: 2026-10-04
status: complete
---

# Phase 1 Plan 05: Link a Group with /linkstaff or the Add Group Picker Summary

**One live-checked `runLinkGroup` flow now links a supergroup to a Staff Group from `/linkstaff [id]` or from the Add group picker's `/start@bot stf_<id>`, requires the issuer to be the live creator of both chats, stores the bot's health with a warning instead of refusing, tells only the Staff Group, deletes the command message and leaves no trace in the linked group.**

## Performance

- **Duration:** about 22 min (started 2026-10-04T22:18:12Z)
- **Tasks:** 3 (1 tracer, 2 TDD expansion)
- **Commits:** 6 task commits (measured from the persisted ledger base `a1aa732`)
- **Files:** 20 changed, 1877 insertions, 14 deletions (measured against the base commit)

## Accomplishments

- Tracer slice end to end: `CreateLink` and the link read functions, `runLinkGroup`, the `/linkstaff` handler, `sendStaffNotice`, the panel's linked-group list, 7-locale strings, help text, alt name and regenerated docs. The tracer verify (`TestLinkStaffTracer`, `TestLinkStaffWarnsWhenBotLacksRights`, `make check-translations`, `make check-docs`) passed before expansion (tracer verified end to end, expanding).
- The SETUP-04 ordering edge is closed and tested: anonymous sender, named or sole verified Staff Group and its live creator, then target type, target not a Staff Group, target live creator, bot health, insert. Until the Staff Group's creator is proven, every refusal is a self-deleting reply in the issuing group and the named chat gets zero messages; a stranger sees the same text for a real Staff Group and for any other ID.
- `/linkstaff` without an argument verifies each recorded Staff Group live and never picks by order.
- The Add group button opens Telegram's picker with the restrict and delete rights requested; `/start@bot stf_<digits>` in a group runs the same flow. The group deep-link registry is new, `start()` routes to it only when there is a payload, and junk payloads keep the old reply.
- Plan-level verification: `make test` exit 0 (twice, the second after the last code change), `go vet -tags testtools ./...` clean, golangci-lint v2.13.1 `--new-from-rev` 0 issues and the dupl pass 0 issues, `make check-translations` and `make check-docs` pass, `go build ./...` clean, `go.mod` and `go.sum` unchanged, `gofmt -l` empty on the changed files. `ALITA_TEST_DATABASE=true go test ./alita/db/staff` and `make test-postgres-integrity` pass against a fresh PostgreSQL 16 database with the migration chain applied (dropped afterwards).

## Task Commits

1. **Task 1 (tracer): /linkstaff <id> creates the link, the Staff Group is told, /staff lists it** - `a98c1c2` (feat)
2. **Task 2 RED: failing tests for the refusal matrix and no-argument resolution** - `b67d230` (test)
3. **Task 2 GREEN: full refusal matrix, no-argument resolution, self-deleting replies** - `5435159` (feat)
4. **Task 3 RED: failing tests for the picker route and URL button** - `1dcaeb0` (test)
5. **Task 3 GREEN: Add group button and the group /start payload route** - `ff15edb` (feat)
6. **Fix found during verification: CreateLink precise reason under the PostgreSQL trigger, SQLite lock, shared cached loader** - `774371e` (fix)

## TDD Gate Compliance

Both TDD tasks have a `test(01-05)` commit followed by a `feat(01-05)` commit. `workflow.tdd_mode` was off, so no hard gate applied; recorded for transparency.

- **Task 2 RED (`b67d230`)** failed on the planned assertions, not on a build error: 24 of 27 `TestLinkStaff*` tests and subtests reported missing replies (for example `replies in the issuing group = [], want exactly one staff_link_invalid_id`) and `ListStaffGroupsByOwner(owner) = ([], <nil>), want two groups`. To compile, the RED commit carried a stub `ListStaffGroupsByOwner` returning `nil, nil` and the `staffSelfDeleteAfter` variable. Tests that characterise Task 1 code passed immediately: the four `TestStaffRepoCreateLink*` error tests, `TestStaffRepoListLinksByStaffFresh` and `TestLinkStaffNoticeFailure/success_stays_out_of_the_linked_group`. The concurrent-insert test failed on SQLite with `database is locked`; that was fixed in `774371e` (see Deviations), not in GREEN, because the RED commit deliberately carried the original `CreateLink`.
- **Task 3 RED (`1dcaeb0`)** failed on the planned assertions: the picker tests got the old `help_pm_questions` reply, `parseStaffPickerPayload` and `staffAddGroupURL` returned zero values, the panel had no keyboard, and the registry stub handled nothing. The RED commit carried compile-only stubs for `GroupDeepLinkHandler`, `RegisterGroupDeepLinkHandler`, `HandleGroupDeepLink`, `parseStaffPickerPayload` and `staffAddGroupURL`. `TestStartGroupPayloadIgnoresJunk` passed immediately (old behaviour, characterisation).
- No REFACTOR commits.

## Decisions Made

See `key-decisions` above. In short: refusal destination is decided by proof of ownership of the named Staff Group; a successful link is never echoed into the linked group; an unverifiable no-argument candidate fails closed; `CreateLink` checks roles before and after the insert; the picker payload is `stf_` plus the negated chat ID.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] CreateLink lost its precise refusal reason under the PostgreSQL trigger, and failed under SQLite contention**
- **Found during:** Task 2 verification (SQLite concurrency test: `database is locked`), then plan-level verification on a trigger-bearing PostgreSQL database (`CreateLink(group is a Staff Group)` returned the trigger's generic `staff role conflict ... SQLSTATE 23514` instead of `ErrRoleConflict`).
- **Issue:** Task 1's check-then-insert transaction read first, then wrote: SQLite refuses that lock upgrade when another connection committed in between, and on PostgreSQL the 01-04 trigger rejected the insert before the in-transaction check could name the reason. My first attempted fix (insert first, checks after) fixed SQLite but made the trigger error pre-empt the precise reason.
- **Fix:** `checkLinkRoles` runs once before the transaction (precise `ErrStaffGroupMissing` / `ErrRoleConflict`) and again inside it after the insert (holds at the moment of write); the insert is first inside the transaction so it takes its write lock up front. `GetStaffGroup` and `GetLinkOfGroup` now share a generic `getCachedRow` loader (the dupl linter flagged the copy).
- **Files modified:** alita/db/staff/repository.go
- **Verification:** `TestStaffRepoCreateLink*` on SQLite (three repeated runs) and on PostgreSQL with the trigger; `make test`; dupl and default lint 0 issues.
- **Commit:** 774371e

**2. [Rule 2 - Missing critical] A successful link is never posted into the linked group**
- **Found during:** Task 2 design, against prohibition P2 ("no link confirmation ... is ever posted in the linked group").
- **Issue:** The plan says that if the Staff Group notice fails, fall back to a self-deleting reply in the issuing group. For a successful link that reply would be a link confirmation visible to the group's members.
- **Fix:** The fallback applies to refusals only. A failed success notice is logged at warn by `sendStaffNotice` and nothing is sent to the linked group. `TestLinkStaffNoticeFailure` covers both halves.
- **Files modified:** alita/modules/staff_link.go
- **Commit:** 5435159

**3. [Rule 2 - Missing critical] One match plus an unverifiable candidate fails closed for /linkstaff without an argument**
- **Found during:** Task 2.
- **Issue:** The plan specifies zero matches with an unknown, zero matches, and several matches. With exactly one match next to a candidate whose check failed, the unknown one could be a second match, so picking the first would be picking by luck.
- **Fix:** Any unverifiable candidate with fewer than two matches gives `staff_check_failed` in place. `TestLinkStaffNoArgument` has a subtest for it.
- **Files modified:** alita/modules/staff_link.go
- **Commit:** 5435159

**4. [Rule 2 - Missing critical] Extra tests beyond the plan's list**
- **Found during:** Tasks 1 to 3.
- **Fix:** `TestStaffHealthFromBot`, `TestParseStaffChatIDArg`, `TestParseStaffPickerPayload`, `TestStaffAddGroupURLFitsTelegramPayloadRules` (largest chat ID still fits Telegram's payload rules), `TestHandleGroupDeepLinkUsesLongestPrefix`, `TestStaffPickerAndTypedCommandMakeTheSameLink`, `TestLinkStaffRefusalsRecordedOwnerNoLongerCreator`, `TestLinkStaffNoticeFailure`, `TestStaffRepoListLinksByStaffFresh`, and the title-trim and cache-invalidation checks in `TestStaffRepoCreateLinkStoresTrimmedTitleAndInvalidatesGate`.
- **Commits:** a98c1c2, b67d230, 1dcaeb0

### Small choices where the plan was silent or differed

- `replySelfDeleting` deletes with `msg.Chat.Id` (the issuing chat) rather than `sent.Chat.Id`; they are the same chat on real Telegram, and the test fake returns a fixed chat in every sendMessage answer.
- The help text writes the optional argument as `/linkstaff [ID]` (square brackets) so the markdown-to-HTML pass and the docs generator never see an unescaped angle bracket.
- A database error while loading the linked groups for `/staff` replies `staff_check_failed` rather than showing a false "No groups are linked yet".
- The Add group button is omitted when the bot's username is empty, since the URL would be invalid.
- `staffAddGroupURL` lives in `staff_panel.go` (where the plan's grep criterion looks for `startgroup=stf_`) and the payload prefix constant in `staff_link.go`.

**Total deviations:** 4 auto-fixed (1 bug, 3 missing-critical), 5 small choices. **Impact:** no scope change; `CreateLink` is now correct under contention and under the trigger, and the linked group is quieter than the plan literally required.

## Authentication Gates

None.

## Known Stubs

None. The compile-only RED stubs were replaced in the GREEN commits.

## Threat Flags

None beyond the plan's threat model. T-01-13 (forged payload or `/linkstaff <id>`: live `CheckOwner` of the issuer on both chats, `TestStaffPickerRefusesStranger`), T-01-14 (uniform refusal, `TestLinkStaffRefusalsUniformForStranger`), T-01-15 (refusals before verification are self-deleting replies in the issuing chat, zero sendMessage to the named chat asserted), T-01-16 (`UNIQUE(group_chat_id)` plus `ON CONFLICT DO NOTHING` plus RowsAffected, concurrent test on SQLite and PostgreSQL), T-01-17 (`staffDisplayTitle` on every rendered title, spliced after translation), T-01-18 (digits-only parsers with length caps, junk falls through) each have a mitigation and a passing test. T-01-SC: `go.mod` and `go.sum` unchanged.

## Issues Encountered

- The Task 3 human check (real Telegram picker with the bot already in the group, not in it, and with Remain anonymous on) was not run; it is coverage item D10 for the verifier. `/linkstaff` is the fallback if the picker misbehaves.
- `gofmt -l` still reports `alita/modules/greetings_command_test.go`, a pre-existing file outside this plan; not touched.
- The module tests were not run against PostgreSQL: with `ALITA_TEST_DATABASE=true` the modules test setup calls AutoMigrate, which fails on a migration-chain schema (`uni_users_user_id` does not exist). This is pre-existing and CI runs the module tests on SQLite; the repository tests, which are what touch PostgreSQL behaviour here, were run against the chain database.
- `.planning/WINDOWS.md` does not exist, so no broken-windows entries were appended; there are no stubs or skipped tests to record.

## Next Phase Readiness

Plan 01-06 (unlink) can reuse `GetLinkOfGroupFresh`, `ListLinksByStaffFresh`, `staffChatTranslator`, `sendStaffNotice`, `replySelfDeleting` and the `staffLinkRow` panel rows. Plans 01-07 to 01-10 can build the live panel on `renderStaffPanel(tr, staffGroup, rows, botUsername)` (keyboard row 0 is the Add group button) and use `staffHealthFromBot` for the health refresh. Every later plan that adds `staff_` keys must re-run `TestStaffLocaleKeys`.

## Self-Check: PASSED

- All created files exist on disk (`staff_link.go`, `staff_notify.go`, `staff_link_test.go`, `staff_link_refusals_test.go`, `staff_picker_test.go`) and modified files are tracked; the working tree was clean before this SUMMARY.
- Commits a98c1c2, b67d230, 5435159, 1dcaeb0, ff15edb and 774371e are on the branch; `git rev-list --count` from the ledger base gives 6.
- Acceptance criteria of all three tasks re-run and passing (greps, `TestStaffLocaleKeys`, `make check-translations`, 27 passing `TestLinkStaffRefusals|NoArgument|SelfDeleting` results); plan-level verification (`make test`, vet, lint, dupl, check-docs, go.mod and go.sum) passing.
