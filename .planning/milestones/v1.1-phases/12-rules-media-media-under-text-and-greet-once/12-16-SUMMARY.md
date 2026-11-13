---
phase: 12-rules-media-media-under-text-and-greet-once
plan: 16
subsystem: settings-editor
tags: [privacy, fit-tests, postgres-integrity, agents-md, docs, spike, phase-gate]
requires:
  - phase: 12-rules-media-media-under-text-and-greet-once
    provides: 12-03 greeted_members claim test, 12-05 locale keys, 12-10 Greet once, 12-11 and 12-12 Media under text switches and Rules parts, 12-13 Rules media and buttons through the door, 12-14 previews and 12-SPIKE.md
provides:
  - TestSettingsStaffPrivacy covers every Phase 12 editor state, the below-unavailable notice and Main with Rules Set in 8 languages, plus a private Phase 12 run of a non-creator admin
  - Main with the Rules line Set in the 3800-unit and 64-byte fit test (settingsEditScreens)
  - make test-postgres-integrity runs TestGreetedMemberClaimConcurrent and TestRulesMediaMigrationDefaults over ./fuku/db/greetings and ./fuku/db/migrations
  - AGENTS.md, callbacks.md, permissions.md and README describe the phase as built
  - 12-SPIKE.md with nine numbered live checks and the follow-ups
affects: []

actuals:
  tokens: 26000
  tasks: 3
  commits: 3

commits: 3
plan_head_before: 1c76ec4f308637de797bb8e111497a6c40f2de6e
plan_head_after: 4daeb039a2115c9d8a0a38f1219e5c3eb5b42ff7

tech-stack:
  added: []
  patterns:
    - "The privacy test reuses settingsEditCallbackStates, so a state a page test adds is also rendered in 8 languages for a group menu and a non-creator private menu"

key-files:
  created: []
  modified:
    - fuku/modules/settings_staff_privacy_test.go
    - fuku/modules/settings_edit_screens_test.go
    - Makefile
    - AGENTS.md
    - README.md
    - docs/src/content/docs/api-reference/callbacks.md
    - docs/src/content/docs/api-reference/permissions.md
    - .planning/phases/12-rules-media-media-under-text-and-greet-once/12-SPIKE.md

key-decisions:
  - "12-15-SUMMARY.md is absent (plan 12-15 dropped by the orchestrator): the Makefile leaves TestSwapCleanMsgIDConcurrent out, AGENTS.md carries one sentence with 'Delete-last race' and names no swap function"
  - "The non-creator private run does the Rules photo first through the real Set flow and then drives the rest on that one menu message, because openEditPagePrivate sends /start once per user"

patterns-established: []

requirements-completed: []

coverage:
  - id: D1
    description: "No Phase 12 screen names the Staff Group or shows a greeted member: every editor state (switches on and off, Greet once, Media under text for photo, video and animation, the document hint, a Rules photo alone), the below-unavailable notice, the Rules refusals, the Rules Full preview header and Main with Rules Set, for a group menu and a non-creator private menu in 8 languages; and a private run by a non-creator admin (Rules photo typed, Rules Media under text, Rules Full preview, Welcome Greet once and Media under text, Welcome Full preview) that writes no greeted_members row"
    requirement: "EDIT-09"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestSettingsStaffPrivacy|TestSettingsEditScreensFitEveryLocale|TestSettingsMainPageFitsEveryLocale)$' -v ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D2
    description: "Every Phase 12 page, prompt, refusal, saved message and Main with the Rules line Set is at most 3800 UTF-16 units with 64-rune titles in 8 languages, every button at most 64 bytes, Omitted 0"
    requirement: "EDIT-07"
    verification:
      - kind: unit
        ref: "fuku/modules/settings_edit_screens_test.go#TestSettingsEditScreensFitEveryLocale"
        status: pass
    human_judgment: false
  - id: D3
    description: "CI's integrity target runs the Phase 12 PostgreSQL guarantees; AGENTS.md, the frozen docs pages and README describe the phase; generated docs show no drift"
    verification:
      - kind: other
        ref: "make generate-docs && make check-docs; grep gates of task 2"
        status: pass
    human_judgment: true
    rationale: "Whether the prose is accurate and clear is a reader's judgment; only drift of generated pages and the presence of the names are checked by a tool"
  - id: D4
    description: "The owner's live checks and open confirmations are written down (caption placement on real clients, old client, sendAnimation, 1025-unit caption, two replicas, /rules on clients, hi/ru/uk cells, native-speaker read, CI)"
    verification: []
    human_judgment: true
    rationale: "These checks need real Telegram clients, a deployed bot, two replicas and native speakers; none can be automated"

duration: 40min
completed: 2026-11-05
status: complete
---

# Phase 12 Plan 16: Privacy and fit tests, PostgreSQL list, rulebook and docs, live checks and phase gate Summary

**Every Phase 12 screen is proven to leak no Staff Group name or greeted member and to fit 8 languages, CI runs the two new PostgreSQL guarantees, AGENTS.md, docs and README describe the phase as built, and the owner has nine live checks plus the open confirmations.**

## Performance

- **Duration:** about 40 min (the `fuku/modules` gate run took 622 s of it)
- **Tasks:** 3 (task 1 tracer)
- **Files modified:** 8

## Accomplishments

- `TestSettingsStaffPrivacy` now renders, for a group menu and a non-creator private menu in all 8 languages, every Phase 12 state of the three editor pages (it reuses `settingsEditCallbackStates`: switches on and off, Greet once, Media under text for a photo, video and animation, the document hint, plus a photo-alone row and a video with Greet once and Media under text), the `settings_edit_below_unavailable` notice, and Main with the Rules line Set. A new subtest runs the private Phase 12 flow of an administrator who is not the creator (typed Rules photo, Rules Media under text, Rules Full preview, Welcome Greet once and Media under text, Welcome Full preview) and checks that nothing produced names the Staff Group, that the Phase 12 markers appeared, that the switches were stored, and that no `greeted_members` row was written.
- `settingsEditScreens` gained "main with the Rules line Set" (every other page on its longest state), so `TestSettingsEditScreensFitEveryLocale` holds it to 3800 UTF-16 units and 64-byte buttons with 64-rune ampersand, astral and multibyte titles, and the privacy test renders it too.
- Makefile: `TestGreetedMemberClaimConcurrent|TestRulesMediaMigrationDefaults` inside the anchored `-run` list (the `$$` anchoring is untouched) and `./fuku/db/greetings` in the package list. `TestGreetedMemberClaimConcurrent` is in `./fuku/db/greetings` and passes locally on SQLite; `TestRulesMediaMigrationDefaults` is in `./fuku/db/migrations` and skips locally (PostgreSQL only).
- AGENTS.md (scoped edits): the editor summary bullet now says controls `e d o u`; the Testing section names the two PostgreSQL guarantees, the Phase 12 characterization tests that must never be re-recorded and the Phase 12 test families, and the test-hygiene rule from 12-09; the Delete-last text gains the one sentence with `Delete-last race`; the stale "joins `make test-postgres-integrity` in the Phase 12 gate plan" is replaced. I checked the rest of the Phase 12 passages (greet once, the new columns, the animation type, the shared Rules builder, the Rules parts and rights, previews) against the code the earlier summaries describe and found them consistent.
- callbacks.md and permissions.md (both still frozen by their MANUALLY MAINTAINED marker) list the controls `o` and `u`, Rules parts `x m b`, the `tg` rule (`t` a control of the page, `v` 0 or 1, refused before any lookup) and rows for Media under text and Greet once; README gets one sentence after the Greetings paragraph for the three features.
- `12-SPIKE.md`: the EDIT-07 limit section of 12-14 is kept word for word; nine numbered `### N.` checks and the follow-ups were appended (see the file).

## Task Commits

1. **Task 1 (tracer): privacy and fit tests** - `b9954f4` (test)
2. **Task 2: Makefile, AGENTS.md, docs, README** - `951d9ea` (docs)
3. **Task 3: the owner's live checks** - `4daeb03` (docs)

The SUMMARY and the STATE and ROADMAP updates are committed after these.

## Phase gate

| Command | Result |
|---------|--------|
| `go test -tags testtools -race -count=1 -run '^(TestSettingsStaffPrivacy\|TestSettingsEditScreensFitEveryLocale\|TestSettingsMainPageFitsEveryLocale)$' -v ./fuku/modules` | passed, three `--- PASS`; the new subtest `a_private_Phase_12_run_by_an_admin_who_is_not_the_creator_never_names_the_Staff_Group_or_a_greeted_member` passed |
| `go test ... -run '^(TestGreetedMemberClaimConcurrent\|TestRulesMediaMigrationDefaults)$' -v ./fuku/db/greetings ./fuku/db/migrations` | `TestGreetedMemberClaimConcurrent` PASS on SQLite; `TestRulesMediaMigrationDefaults` SKIP (needs PostgreSQL, runs in CI through `make test-postgres-integrity`) |
| `make test` | not run as such: the sandbox has no `covdata` tool, so `go test -coverpkg` reports a `build-fail` for `fuku/utils/actionlog` (a package with no test files). Fallback as in 11-14-SUMMARY: every package except `fuku/modules` through the real pipeline flags (`-json -race -coverprofile -coverpkg`, `-timeout 15m`), then `./fuku/modules` alone with the same flags and `-timeout 40m`: `ok fuku/modules 621.973s`, exit 0, no `fail` event in either stream. The two JSON streams merged with only the three covdata events of `actionlog` (two `build-output`, one `build-fail`) removed and fed to `go run ./scripts/check_test_results`: exit 0, 6781 passing tests, 17 skips all expected, 76.0% coverage line. `fuku/modules` took 622 s here, inside the Makefile's 15 minute limit. CI's `make test` stays the authority |
| `make test-postgres-integrity` | not run: no PostgreSQL here. `make -n` shows the anchored `-run` list and the package list as edited. CI runs it |
| `make check-translations` | exit 0, all 8 locales complete |
| `make generate-docs`, then `make check-docs` | no drift (exit 0); the generator changed nothing beyond the hand edits |
| `make lint` | could not run: golangci-lint is built with Go 1.25 and refuses Go 1.26.0. Fallback: `go vet -tags testtools ./...` exit 0; `gofmt -l` over every Go file changed since 72a692b prints nothing; `python3 .../lint-local.py 72a692b` (dupl, gocyclo, godox, staticcheck, errcheck on added lines) exit 0 |
| `git diff --exit-code 72a692b -- go.mod go.sum` and `git diff --exit-code bd2f794 -- go.mod go.sum` | both exit 0 |
| `node --test scripts/captcha-page/*.test.mjs` | not run (the Turnstile page was not touched) |

Acceptance greps of task 2: `grep -c TestGreetedMemberClaimConcurrent Makefile` 1 (baseline 0), `fuku/db/greetings` in the Makefile, `TestRulesMediaMigrationDefaults` once in the Makefile, `TestGreetedMemberClaimConcurrent` in AGENTS.md, `Greet once` in README.md, both frozen pages keep their marker in the first 512 bytes. The third verify gate (the `test -f` of 12-15-SUMMARY.md, absent) passed: the Makefile has no `TestSwapCleanMsgIDConcurrent`, AGENTS.md has `Delete-last race` and no `SwapCleanWelcomeMsgID`. Task 3: `12-SPIKE.md` has 9 `### N.` headings, `show_caption_above_media` twice, and `a GIF stored before Phase 12` three times (the EDIT-07 limit section kept).

## Deviations from Plan

None - plan executed as written. The plan's lint fallback names `.planning/phases/11-message-editor/handoff/lint-local.py`; the brief's scratchpad copy was used instead, as the brief directs.

## Issues Encountered

- A first scripted edit of the privacy test failed on tab counts and wrote nothing (the script asserts before writing); it was redone with exact edits. No effect on the result.
- `TestGreetedMemberClaimConcurrent` carries `skipIfNoDb` rather than no skip; it passes on SQLite locally, which is how AGENTS.md now describes it ("also runs on SQLite in `make test`").

## Requirements

`requirements-completed: []`. EDIT-07 and EDIT-08 were completed by plan 12-14 and EDIT-09 by plan 12-10; this plan re-confirms them with the final privacy, size and PostgreSQL checks and changes no checkbox or traceability row in REQUIREMENTS.md.

## Known Stubs

None. No file this plan changed holds a placeholder, an empty default flowing to the UI or a TODO.

## Threat Flags

None. This plan adds tests, a CI list, docs and a checklist; no endpoint, auth path, file access or schema.

## User Setup Required

None. The owner's optional live checks are in `12-SPIKE.md`.

## Next Phase Readiness

All plans of Phase 12 except the dropped 12-15 have a SUMMARY; the orchestrator can verify and close the phase. Open items for the owner: the live checks and follow-ups in `12-SPIKE.md` (A1, A2, A3, A4 and A12 confirmations, the Delete-last race of the dropped plan 12-15), and CI's `make test` and `make test-postgres-integrity`.

## Self-Check: PASSED

- FOUND: fuku/modules/settings_staff_privacy_test.go, fuku/modules/settings_edit_screens_test.go, Makefile, AGENTS.md, README.md, docs/src/content/docs/api-reference/callbacks.md, docs/src/content/docs/api-reference/permissions.md, 12-SPIKE.md
- FOUND commits: b9954f4, 951d9ea, 4daeb03 (all ancestors of HEAD)
