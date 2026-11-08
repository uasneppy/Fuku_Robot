---
phase: 14-profile-photo-and-channel-rules
plan: 12
subsystem: member-requirements
tags: [end-to-end-test, rulebook, docs, spike, phase-gate]

requires:
  - phase: 14-profile-photo-and-channel-rules
    provides: plans 14-01 to 14-11 (storage, locales, the lookup layer, the executor, the watcher, "I fixed it", the page, the gate, the budget and privacy sweep)
provides:
  - TestMemberReqLookupEndToEnd (photo and channel subtests through the page, the real dispatcher, the executor and the button)
  - AGENTS.md read against the code and corrected; README, callbacks.md and permissions.md describe the seven rules
  - REQUIREMENTS.md traceability notes for MREQ-01 and MREQ-10 (status cells untouched)
  - 14-SPIKE.md (12 live checks, the pre-enable warning, owner confirmations, accepted limits)
  - the Phase 14 gate results
affects: [15, 16, 17]

plan_head_before: e3c67eb5cc5aee6b9d68ede4c080e7337e1f6600
plan_head_after: 5c4634fda69b34eae36e114e1c2bc2ebe40d97dd
actuals:
  tokens: 40000
  tasks: 3
  commits: 3

tech-stack:
  added: []
  patterns:
    - "A closing end-to-end test drives the owner's page press and card Yes, the real dispatcher and the button with one fake Telegram, and also covers the cases that must punish nobody"

key-files:
  created:
    - fuku/modules/memberreq_lookup_e2e_test.go
    - .planning/phases/14-profile-photo-and-channel-rules/14-SPIKE.md
  modified:
    - AGENTS.md
    - README.md
    - docs/src/content/docs/api-reference/callbacks.md
    - docs/src/content/docs/api-reference/permissions.md
    - .planning/REQUIREMENTS.md

key-decisions:
  - "The end-to-end test opens the menu in the group (the settings card tests' own path) rather than in a private chat: the press pipeline is the same code, and a private menu adds one getChat that the earlier plans' page tests already cover"
  - "Production code is unchanged: the end-to-end test and the whole gate passed without a fix commit"

requirements-completed: []

coverage:
  - id: D1
    description: "the whole slice works for the photo rule: an owner sets Mute from the page (card, Yes), a member with clean names and no photo is muted once with one notice ending with the photo remedy and the button, a second message punishes nobody again, the member adds a photo, taps the button and is freed by a fresh lookup, the photo positive is cached and the next message starts no job"
    requirement: "MREQ-11"
    verification:
      - kind: unit
        ref: "TestMemberReqLookupEndToEnd/photo"
        status: pass
    human_judgment: false
  - id: D2
    description: "the whole slice works for the channel rule: a member who left a channel the bot administers is muted with the channel phrase and no remedy, subscribes and is freed; with the bot not an administrator, and with the group unlinked, nobody is punished; the page shows the not-administrator line; a punishing press with no link is refused with nothing written"
    requirement: "MREQ-12"
    verification:
      - kind: unit
        ref: "TestMemberReqLookupEndToEnd/channel"
        status: pass
    human_judgment: false
  - id: D3
    description: "the rulebook, docs, README and requirement notes describe the phase as built (seven rules, page codes ph and ch, the status block, the live link check, step 4b, approved members freed at the tap), and MREQ-01 and MREQ-10 say where Phase 15 completes them"
    verification:
      - kind: other
        ref: "make generate-docs then make check-docs (no drift), the plan's grep acceptance criteria"
        status: pass
    human_judgment: false
  - id: D4
    description: "14-SPIKE.md lists the owner's live checks S1, S2, S3 and nine more, opens with the pre-enable warning and states that none blocks the phase"
    verification:
      - kind: other
        ref: "grep acceptance criteria of task 3 (12 numbered checks, 'Before you enable', 'basic group')"
        status: pass
    human_judgment: true
  - id: D5
    description: "the phase gate passes"
    verification:
      - kind: integration
        ref: "see Phase gate below"
        status: pass
    human_judgment: false

duration: 65min
completed: 2026-11-08
status: complete
---

# Phase 14 Plan 12: Close the profile photo and channel rules Summary

**One end-to-end test proves both new rules through the real dispatcher (including the cases that punish nobody), AGENTS.md and the docs match the code, 14-SPIKE.md hands the owner the live checks no fake can make, and the phase gate passes.**

## What was done

**Task 1 (tracer, `098e0f3`):** `TestMemberReqLookupEndToEnd` in `fuku/modules/memberreq_lookup_e2e_test.go`.

- Subtest `photo`: the settings environment's creator opens the menu and the Requirements page, presses the photo cell to Mute and the card's Yes (stored `photo_action` 4, nothing written by the card). A member with clean names and no photo sends a message: one `restrictChatMember` with the marker, one hold with the photo bit, one notice that names the photo phrase, ends with `mreq_remedy_photo` and carries the button. A second message adds no punishment and no notice. The fake then gives the member a photo and the member taps the button: one restrict with the group's permissions, the hold gone, the notice edited to `mreq_notice_fixed`, one toast. The photo key is then cached, and the member's next message starts no job and makes no lookup.
- Subtest `channel`: the group is linked to a channel the bot administers; the owner sets Channel to Mute from the page; a member who left is muted with the channel phrase and no remedy tail; after subscribing, the button frees them. With the bot changed to a plain member (its cached `ba:` key dropped), an unsubscribed member is not punished and the page shows the not-administrator line. With the group unlinked (cached `lc:` dropped, bot an administrator again), an unsubscribed member is not punished and a punishing Ban press is refused with the `settings_mreq_here_nolink` alert, no card and nothing written (the stored Mute stays).
- `-count=1 -v`: both subtests PASS; `-count=10`: ok (7.9 s). It never skips (CI's `check_test_results` rejects an unexpected skip), uses fresh IDs from the earlier toolkits and the cleanups of `mreqLookupCleanup`/`mreqChannel`.

**Task 2 (`e15a8dd`):** AGENTS.md was read against the code in the member-requirements regions (Startup and shutdown drain bullet, Handlers watcher bullet, Callbacks `mreq|v1` and page `m` bullets, Permissions, the Data keys, the lookups, rule-table, executor, join, held-release and runner Subsystem traps, and the Testing section) and grepped whole for the stale phrases of research S-10. The plans 14-01 to 14-11 had already corrected almost every phrase; the file now holds 2,568 lines. Corrections made:

1. The runner bullet still said "Phase 14's 'cannot work here' line will say so on the page" (a future-tense promise); it now says the page says so with its basic-group line (`settings_mreq_here_basic`), a stated limit.
2. The lookups bullet gained the accepted limit that a member can be stuck behind "I fixed it" only while a channel lookup stays unanswerable AND the bot is neither a definite non-administrator nor bot-gone (an admin's `/unmute`, the 330-day marker mute the worst case), the note that no log of a non-gotgbot client's transport errors is scrubbed (the log test relies on gotgbot's), and the statement that three facts rest on live Telegram behaviour with the `14-SPIKE.md` pointer.
3. The Testing section's Phase 14 families now list `TestMemberReqLookupEndToEnd` and say why the channel unmute is asserted against `defaultUnmutePermissions()`.

Checked and found already true, no edit: the drain bullet (lookups derive from `memberReqContext()`, Redis and Telegram only, no new drain, 65 s worst case unchanged), the watcher and executor sentences (the lookups sit after the exemptions, the hold read, the pause and the live `getChatMember`, before the mute-over-restriction check, the cap and the claim; the plan's "step 6b" is a label the rulebook does not use and needs not), the held-release, notice-tail and runner sentences, the `mreq|v1` bullet (step 4b, fresh lookups, broken first, the classifier), the page `m` bullet (seven cells, six rows, the status block, the Off-only screen, the refused press), the Permissions bullets, the Data keys (`photo`, `lc`, `sub`, `ba`, `lkp:pace:*`, seven columns, the 247 `settings_` and 29 `mreq_` key counts, verified against `locales/en.yml`), and the Testing counts. The constants the prose cites (lookup pacer 100 ms, 1 retry, 5 s; `lc` 10 minutes, `ba` 2 minutes, fixed-lookup budget 5 s, status read 3 s, the five-or-more count answer) were checked against the code. Zero hits remain for "fifteen cards" and "five rule actions".

README: the Member requirements paragraph says seven rules and adds the two lookup rules (a photo the bot cannot see counts as missing; the channel rule needs a linked channel and an administrator bot; a check that cannot be answered punishes nobody). `callbacks.md`: page `m` codes `ph` and `ch`, the status block, the Off-only channel screen, the live link check of a punishing channel press, and `mreq` step 4b, the fresh lookups and the broken-first answers. `permissions.md`: rows for the two rules, the channel link condition and approved members freed at the tap. Both frozen pages keep their MANUALLY MAINTAINED marker. `REQUIREMENTS.md`: the Phase cells of MREQ-01 and MREQ-10 carry the notes; every status cell is unchanged.

**Task 3 (`5c4634f`):** `14-SPIKE.md`, in the 13-SPIKE.md format: the "Before you enable Mute, Kick or Ban for the photo or channel rule in a real group" section (enforcement is ON in code, rules default Off, both rules rest on live Telegram behaviour no fake proves, Warn and Off need no spike, a basic group is not screened), what the tests prove, twelve numbered checks (1 S1 hidden photo, 2 S2 non-administrator bot answers and whether `IsBotGoneError` can be narrowed, 3 S3 `linked_chat_id` for a plain-member bot, 4 the public-photo fallback, 5 member statuses, 6 the cells in hi, ru and uk on a phone, 7 two replicas, 8 a raid with the pacer shedding, 9 a restart during lookups, 10 link and unlink, 11 the native-speaker read of the 16 new and 7 reworded keys with the grammar notes, 12 CI), the follow-ups with recommended defaults (A3, A4, A7 and M10, M11, M12, M13, the pacer interval, and the MREQ-10 and MREQ-01 carry-over to Phase 15) and the accepted limits.

## Phase gate (real results)

| Gate | Result |
|------|--------|
| `make test`, as is | ran to completion in the background (output in the session scratchpad, `maketest.txt`): every package `ok`, `fuku/modules` `ok` in 1145 s, and the target exit 2 only because of `go: no such tool "covdata"` for `fuku/utils/actionlog` (a package with no test files: a `build-fail` event). The documented sandbox fallback was then run |
| `make test` fallback | the same flags (`-json -race -coverprofile -coverpkg -count=1`), every package except `fuku/modules` with `-timeout 30m`, then `./fuku/modules` alone with `-timeout 40m`, JSON saved to files; the three covdata events of `fuku/utils/actionlog` (two `build-output` lines and the `build-fail`) removed from the first file; merged and fed to `go run ./scripts/check_test_results`: **exit 0**, no unexpected skip, no failure (`fuku/modules` ok in 1124 s, 76.9% coverage). The `modules` run itself exited 0 and the first run exited 1 only for the covdata event. CI's `make test` stays the authority |
| `make check-translations` | exit 0, all translations present |
| `make generate-docs` then `make check-docs` | exit 0, "No drift detected" (also re-run on the final tree) |
| `make lint` | could not run: the installed golangci-lint is built with Go 1.25 and refuses Go 1.26.0. Fallback: `go vet -tags testtools ./...` exit 0; `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827` exit 0 (read directly, "no dupl, gocyclo or godox findings on lines added since 8c0c827"; staticcheck and errcheck included); `gofmt -l` over every Go file changed since 8c0c827 prints only `main.go`, which is not formatted at 8c0c827 either (a `Scope:` alignment from before Phase 13; see Concerns) |
| `git diff --exit-code 8c0c827 -- go.mod go.sum` | exit 0 |
| appliers | `fuku/db/migrations/runner.go` and `scripts/migrate_psql.sh`: `git diff 8c0c827` has 0 lines |
| `make -n test-postgres-integrity` | lists `TestChatRequirementsSettings`, `TestMemberRequirementLookupMigrationDefaults`, `./fuku/db/memberreq` and `./fuku/db/migrations`, each once |
| PostgreSQL 16 throwaway-cluster proof (14-01's, repeated) | done: cluster in `/var/lib/postgresql/fuku-gate14-pg`, trust auth on 127.0.0.1:55436, database `fuku_test`. `TestRepositoryMigrationChain` PASS (the chain applies the Phase 14 migration); with `FUKU_TEST_DATABASE=true`, `TestChatRequirementsSettings`, `TestRequirementHolds`, `TestRequirementHoldClaimConcurrent` (`./fuku/db/memberreq`) and `TestMemberRequirementLookupMigrationDefaults` (`./fuku/db/migrations`) all PASS (not skipped). The cluster was stopped and its data directory deleted afterwards (no DSN with credentials was written anywhere) |
| `node --test scripts/captcha-page/*.test.mjs` | not run (the Turnstile page is untouched) |

## Requirement bookkeeping

`requirements-completed: []`. This plan completes nothing and ticks nothing; `requirements.mark-complete` and `phase.complete` were not run and Phase 14 is not marked Complete in ROADMAP.md (the orchestrator does that after verification). MREQ-11 was completed by 14-08, MREQ-12 by 14-10 and PLAT-06 by 14-11 and stay ticked. **MREQ-01 is seven of the eight rules** after this phase (Phase 13: last name, username, Russian, Arabic, Chinese; Phase 14: profile photo and channel); the AI spam-name rule is MREQ-14 in Phase 15, so its checkbox stays Pending and its traceability note says so. **MREQ-10's three conditions of Phase 14's success criterion 3 are delivered**; the requirement text's "AI not configured" is Phase 15's `settings_mreq_here_ai` line, so it stays Pending with the note "AI line through MREQ-14 in Phase 15". Both carry-overs are repeated in `14-SPIKE.md`.

## Deviations from Plan

None - plan executed as written, with these notes: (1) the end-to-end test opens the menu in the group rather than in a private chat (same press pipeline, see key decisions); (2) no `fix(14-12)` commit was needed, the test and the gate passed on the code of plans 14-01 to 14-11; (3) `make test` was run exactly as the Makefile defines it first and reported only the sandbox's covdata build-fail, so the documented fallback was added, not substituted.

## Known Stubs

None.

## Threat Flags

None: a test, documentation and a checklist only; no production code, endpoint, schema or secret was touched.

## Concerns (named, not fixed)

- `main.go` is not `gofmt`-clean (a struct-literal alignment of `Scope:` in the `SetMyCommands` call, last touched by commit `f6407cf` of plan 13-09; the file at baseline 8c0c827 is unformatted too). The pre-commit hooks evidently do not format it. CI's lint reports only new issues, so it has not failed; a one-line `gofmt -w main.go` would settle it and is out of this plan's scope.
- The gate's `make test` takes about 19 minutes for `fuku/modules` alone and CI's 45-minute job budget holds, but the margin shrank with every Phase 14 plan; worth a look at the slowest tests before Phase 15 adds more.

## Self-Check: PASSED

- `fuku/modules/memberreq_lookup_e2e_test.go` and `.planning/phases/14-profile-photo-and-channel-rules/14-SPIKE.md` exist; `098e0f3`, `e15a8dd` and `5c4634f` are ancestors of HEAD; `git rev-list --count e3c67eb..5c4634f` is 3 (the ledger's measured count).
- The acceptance greps: `grep -c 'func TestMemberReqLookupEndToEnd'` 1, `mreqPressFixed` at least 1, `grep -c 'fifteen cards' AGENTS.md` 0, `step 4b` in callbacks.md, `profile photo` in README.md and permissions.md, MREQ-01 row with `seven of the eight`, both frozen markers present, the SPIKE has 12 numbered checks, "Before you enable" and "basic group".
