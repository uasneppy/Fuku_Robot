---
phase: 13-member-requirements
plan: 14
subsystem: modules
tags: [member-requirements, privacy-sweep, backup-exclusion, docs, phase-gate, live-checks]
requires:
  - phase: 13-member-requirements
    provides: the Requirements page and its renderers (13-05, 13-06), the notices (13-07), the mreq|v1 "I fixed it" answers (13-13), both tables (13-01), the integrity names in the Makefile (13-01)
provides:
  - the Phase 13 final sweep: a private non-creator run through page m with both switches, and every "I fixed it" answer measured at 200 code points in 8 languages
  - TestMemberRequirementsStayOutOfBackup and both models in the backup harness
  - AGENTS.md Testing paragraph for the Phase 13 families, the memberReqEnv toolkit and its hygiene rule
  - callbacks.md (page m, mreq|v1), permissions.md (rights of page m and of I fixed it), a README paragraph
  - the MREQ-01 traceability note (five of the eight rules)
  - 13-SPIKE.md, the owner's live checks, open confirmations and accepted limits
affects: [member-requirements, phase-14, phase-15]

actuals:
  tokens: 21000
  tasks: 3
  commits: 4

commits: 4
plan_head_before: ae05dc52c3fd0286223b175f4b6038bf6c62f737
plan_head_after: 153052d06e11a1cff4affe54914713dc8710da3e

tech-stack:
  added: []
  patterns:
    - "A pure measurement helper (assertMemberReqFixedAnswers) renders the answers through the same translator calls the handler uses"
    - "A backup-exclusion test copied from TestGreetedMembersStayOutOfBackup: export, import and reset against two fresh chats"

key-files:
  created:
    - fuku/db/backup/memberreq_backup_test.go
    - .planning/phases/13-member-requirements/13-SPIKE.md
  modified:
    - fuku/modules/memberreq_screens_test.go
    - fuku/modules/settings_staff_privacy_test.go
    - fuku/db/backup/testmain_test.go
    - AGENTS.md
    - README.md
    - docs/src/content/docs/api-reference/callbacks.md
    - docs/src/content/docs/api-reference/permissions.md
    - .planning/REQUIREMENTS.md

key-decisions:
  - "No production code changed: every check passed on the code of 13-13, so no fix was needed and no check was loosened"
  - "The answer measurement covers the eight answers of the button plus one still-broken answer per rule, and the two notices the button edits in (400 visible code points, like every notice)"
  - "AGENTS.md needed no correction beyond the new Testing paragraph: every Phase 13 identifier, constant and key it names was checked against the code"
  - "MREQ-01 keeps its Pending status cell; only the Phase cell gained the five-of-eight note"

requirements-completed: []

duration: about 40 min of work plus two whole-tree test runs of about 12 min each
completed: 2026-11-08
status: complete
---

# Phase 13 Plan 14: Final sweep, docs and the phase gate Summary

**The whole Member requirements phase is proven to leak nothing, fit every language and stay out of every backup; the rulebook, docs and README describe it; the owner has a 13-check live list; and the phase gate ran (see the table).**

## Commits

| # | Hash | Subject |
|---|------|---------|
| 1 | 1417b6e | test(13-14): no Phase 13 screen names the Staff Group, overflows or travels in a backup |
| 2 | 919375c | docs(13-14): describe member requirements in the rulebook, docs and README |
| 3 | da65612 | docs(13-14): name the memberReqEnv type in the rulebook |
| 4 | 153052d | docs(13-14): add the owner's live checks and open confirmations for member requirements |

## What was built

- **Task 1 (tracer, tests only).** `TestSettingsStaffPrivacy`'s private run by an administrator who is not the creator already opened page `m`, a rule screen, a Mute card, its Yes and the Check on join switch; it now also presses Delete messages. The page, the five rule screens and the fifteen cards were already swept in 8 languages by 13-05 and 13-06. `TestMemberReqScreensFitEveryLocale` now also renders every "I fixed it" answer (done, done during a lockdown, still broken with all five rules and with each single rule, not yours, not ours, check failed, stale, invalid request) and checks each is at most 200 code points, and the two notices the button edits in (400 visible code points), and that none names the chat ID, the Staff Group, the admin who set the rule or holds an unfilled placeholder. `TestMemberRequirementsStayOutOfBackup` (`fuku/db/backup/memberreq_backup_test.go`, both models added to the backup AutoMigrate list) proves a full export holds only backup modules, exporting `chat_requirements`, `chat_requirement_holds` or `member_requirements` by name is an error, an import creates no row in either table, and a reset of the source chat leaves both rows.
- **Task 2 (docs).** AGENTS.md Testing gained one paragraph: the Phase 13 families by file, the `memberReqEnv` toolkit, its hygiene rule, the size and privacy sweep and the backup exclusion test. `callbacks.md` documents page `m` of `settings|v1` (codes `mo` and `my`, the five rule controls with `st` only, the switches `j` and `d`, the `Actions` allow-list) and the `mreq` namespace (fields `a=f`, `h`, `u`, presser-only); its overview counts went from 26 to 27 callbacks and 18 to 19 modules. `permissions.md` gained three rows (open the page, every punishment choice and switch, I fixed it). README gained one paragraph. The MREQ-01 row's Phase cell now reads "Phase 13 (five of the eight rules; the other three through MREQ-11, MREQ-12, MREQ-14 in Phases 14 and 15)". Both frozen pages keep their marker.
- **Task 3 (live checks).** `13-SPIKE.md` has 13 numbered checks (research A1 to A8, the bidi isolates, the hi/ru/uk cells, a native-speaker read, two live replicas, CI), a prominent "Before you enable Mute in a real group" section, the UI contract's open confirmations (A1, A4, A8, A14, A15, A17, A18) and the accepted limits.

**MREQ-01 scope, in plain words:** Phase 13 delivers five of the eight rules the requirement names (last name, username, Russian, Arabic, Chinese). The profile-photo, channel-subscription and AI spam-name rules arrive through MREQ-11, MREQ-12 and MREQ-14 of Phases 14 and 15. MREQ-01 therefore stays Pending.

## Before you enable Mute in a real group

Enforcement is on in code since 13-13 (every rule still defaults to Off, so a group screens nobody until its owner sets a rule). Run the live checks for the marker `until_date` (A2), the leave and rejoin behaviour (A3, A4), the restricted-member behaviour (A4) and the lockdown unmute path (A5, A8) in `13-SPIKE.md` before setting any rule to Mute. Warn, Kick and Ban do not depend on the marker.

## Accepted limits flagged for the code review

- `memberReqLiveEligible` lets a restricted member who is still in the group through, so a requirement Mute can replace a partial admin restriction, and "I fixed it" would then restore the group's default permissions. Recorded in `13-SPIKE.md`; not changed here (no production code is in this plan).
- Also listed there: a basic group is never screened (D-20); an unknown-status violator's message is kept during a pause; two Telegram refusals in a row leave a mute with no button (an admin's `/unmute` or a rejoin ends it); a crash between the rejoin reset's delete and its lift can strand a mute; the SQLite "database is locked" artifact for concurrent warn writes; a Latin ë; Unicode 15 tables; the goodbye after a kick or ban; the runner's shedding; the rolling-deploy caveats.

## AGENTS.md read-through

I read the whole file once (and checked every `memberReq*`, `settingsMreq*`, `memberreq.*` and `mreq_*` identifier it names against the code and the `en.yml` keys; the numbers it states: 2 s stop wait, 90 s job timeout, 32 in flight, 30 minute pass TTL, 2 minute pass age and stale claim, cap 10 and pause 10 minutes, join timeout 15 s, priority 241, page order 80 and icon, the drain's registration order in `main.go`). Corrections needed: none; no Phase 13 sentence was inconsistent with the code. The only edits are the new Testing paragraph and the `memberReqEnv` type name inside it (a follow-up commit, because the acceptance grep wants the type name).

## Phase gate (real results)

| Command | Result |
|---------|--------|
| `go test -tags testtools -race -count=1 -run '^(TestSettingsStaffPrivacy\|TestMemberReqScreensFitEveryLocale\|TestSettingsMainPageFitsEveryLocale\|TestSettingsMreqPageFitsEveryLocale)$' -v ./fuku/modules` | exit 0, all four `--- PASS` (every language subtest PASS) |
| `go test -tags testtools -race -count=1 -run '^(TestMemberRequirementsStayOutOfBackup\|TestGreetedMembersStayOutOfBackup)$' -v ./fuku/db/backup` | exit 0, both `--- PASS` |
| `make test` | **ran, exit 2, only because of the missing `covdata` tool**: 67 packages `ok` (including `fuku/modules` 688 s, 75.4% coverage), no `FAIL` or `--- FAIL` line; `go: no such tool "covdata"` is a `build-fail` for `fuku/utils/actionlog` (a package with no test files), which `check_test_results` reports as "test failure". Fallback as in 12-16-SUMMARY: the same command with `-json` written to a file (`go test -tags testtools -json -race -coverprofile -coverpkg=... -count=1 -timeout 30m ./...`, `fuku/modules` 690 s, exit 1 from the covdata build-fail only), the three `actionlog` covdata events (two `build-output`, one `build-fail`) removed, the rest fed to `go run ./scripts/check_test_results`: **exit 0**, 7229 passing tests, 17 skips, all expected (the checker fails on an unexpected skip), 75.4% coverage. CI's `make test` stays the authority |
| `make check-translations` | exit 0, all translations present |
| `make generate-docs`, then `make check-docs` | no drift (exit 0); the generator changed nothing beyond the hand edits |
| `make lint` | could not run: golangci-lint is built with Go 1.25 and refuses Go 1.26.0. Fallback: `go vet -tags testtools ./...` exit 0; `gofmt -l` over the four Go files I touched prints nothing (`main.go`, which Phase 13 touched, is unformatted at base 8c0c827 too, a pre-existing alignment in `SetMyCommandsOpts`); `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827` exit 0 (read directly, before the first commit and again before this SUMMARY) |
| `git diff --exit-code 8c0c827 -- go.mod go.sum` | exit 0, no output |
| `make -n test-postgres-integrity` | shows `TestChatRequirementsSettings`, `TestRequirementHoldClaimConcurrent`, `TestRequirementHolds` and `./fuku/db/memberreq` |
| `make test-postgres-integrity` | not run: no PostgreSQL started here (the plan allowed repeating 13-01's throwaway cluster proof "if time allows"; this plan adds no migration, `CHECK` or partial index). CI runs it |
| `node --test scripts/captcha-page/*.test.mjs` | not run (the Turnstile page was not touched) |

Task acceptance greps: `grep -n '^| MREQ-01 ' .planning/REQUIREMENTS.md | grep -c MREQ-14` 1 (baseline 0); `grep -c TestRequirementHolds Makefile AGENTS.md` 1 and 2; `grep -c 'Member requirements' README.md` 1; `grep -c 'mreq|v1\|`mreq`' callbacks.md` 2; both frozen pages keep `MANUALLY MAINTAINED` in their first 512 bytes; `grep -c memberReqEnv AGENTS.md` 1; `grep -c 'models.ChatRequirementHold{}' fuku/db/backup/testmain_test.go` 1 and `grep -c chat_requirement fuku/db/backup/types.go` 0; `13-SPIKE.md` has 13 `### N.` headings, mentions `until_date` and `basic group`.

## Deviations from Plan

None - plan executed as written, with these notes:

- The plan's lint fallback names `.planning/phases/11-message-editor/handoff/lint-local.py`; the brief's copy under the phase-13 handoff path was used, as the brief directs.
- The privacy test's private run already covered most of what the plan lists (earlier plans added them); the only new line is the Delete messages switch press. The new assertions are in the answer measurement and the backup test.
- The "admin who set the rule" check in the answers is by a distinctive name constant that no answer is built from; it guards a future change that would splice an admin name into an answer.
- Plan-head ledger (protocol 0c) was written after the first task's commit, from its parent `ae05dc5`, the same value the brief gave.
- The first `make test` run could not be fed to the checker (its JSON went through the pipe), so the whole tree was run a second time with the JSON saved for the fallback; that is why `fuku/modules` ran twice (688 s and 690 s).

## Auth gates

None.

## Known Stubs

None. No UI or data source was touched.

## Threat Flags

None. The plan adds tests and documents only; no endpoint, file access or schema change.

## Requirements

`requirements-completed: []`. This plan completes none and re-confirms what earlier plans completed: MREQ-01 (13-06, still Pending because of its scope note above), MREQ-13 (13-09), MREQ-03 (13-10), MREQ-02, MREQ-05 and MREQ-09 (13-11), MREQ-04 and MREQ-08 (13-12), MREQ-06 and MREQ-07 (13-13). `.planning/REQUIREMENTS.md` changed only in the Phase cell of the MREQ-01 row. `phase.complete` was not run and Phase 13 is not marked Complete.

## Self-Check: PASSED

- Created files exist: `fuku/db/backup/memberreq_backup_test.go`, `.planning/phases/13-member-requirements/13-SPIKE.md`.
- Commits exist in HEAD's history: 1417b6e, 919375c, da65612, 153052d.
