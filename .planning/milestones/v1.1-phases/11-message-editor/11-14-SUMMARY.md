---
phase: 11-message-editor
plan: 14
subsystem: modules
tags: [settings-menu, message-editor, privacy-test, locale-fit, docs, phase-gate]
requires:
  - phase: 11-message-editor
    provides: plans 11-08 to 11-13 (editor pages, Set/Cancel/Remove, typed-reply door, previews, album gate)
provides:
  - TestSettingsEditScreensFitEveryLocale and the shared settingsEditScreens builder (every non-page editor screen)
  - TestSettingsStaffPrivacy extended to every editor screen and to a private typed editor run of a non-creator admin
  - renderSettingsInputRefusal and settingsInputSavedNotice, the pure builders the door sends with
  - AGENTS.md, manual docs and README describing the whole editor
  - 11-SPIKE.md, the owner's 11 non-blocking live checks and 3 follow-ups
affects: [phase-11-verification, settings-message-editor]

actuals:
  tokens: 11200
  tasks: 3
  commits: 3

commits: 3
plan_head_before: 99078e7
plan_head_after: 32907172397c6d2aa48736898897a980219a18ff

tech-stack:
  added: []
  patterns:
    - "A test renders exactly what the code sends by calling the same pure builder (renderSettingsInputRefusal, settingsInputSavedNotice, renderSettingsEditPrompt, renderSettingsEditor), never a copy of its text"
    - "One screen builder (settingsEditScreens) feeds both the privacy test and the fit test, so the two cannot drift"

key-files:
  created:
    - fuku/modules/settings_edit_screens_test.go
    - .planning/phases/11-message-editor/11-SPIKE.md
  modified:
    - fuku/modules/settings_input_door.go
    - fuku/modules/settings_staff_privacy_test.go
    - AGENTS.md
    - README.md
    - docs/src/content/docs/api-reference/callbacks.md
    - docs/src/content/docs/api-reference/commands.md
    - docs/src/content/docs/api-reference/permissions.md
    - docs/src/content/docs/architecture/handler-groups.md

key-decisions:
  - "The door's refusal reply and the saved-page notice were pulled out of the methods into pure functions (behaviour unchanged), so the privacy and fit tests render what is sent instead of copying it."
  - "The previews are described in the new Message editor bullet of AGENTS.md rather than spread over the older page bullet, so earlier plans' text is not duplicated."

patterns-established:
  - "Phase gate with a package slower than the 10 minute per-package timeout in this sandbox: run the rest through the real pipeline, run the slow package alone with a longer timeout, and feed the merged JSON through scripts/check_test_results"

requirements-completed: []

coverage:
  - id: D1
    description: "No editor screen (prompt, refusals, ended and wait-ended replies, saved page, cancel and group notes, preview headers) names the Staff Group, in 8 languages, for a group menu and a non-creator private menu, and a full private typed run of a non-creator admin leaks nothing"
    requirement: "EDIT-02"
    verification:
      - kind: unit
        ref: "fuku/modules/settings_staff_privacy_test.go#TestSettingsStaffPrivacy"
        status: pass
    human_judgment: false
  - id: D2
    description: "Every editor prompt, refusal, saved page and wait-ended reply fits 3800 UTF-16 units and 64-byte buttons in all 8 languages with 64-rune titles"
    requirement: "EDIT-06"
    verification:
      - kind: unit
        ref: "fuku/modules/settings_edit_screens_test.go#TestSettingsEditScreensFitEveryLocale"
        status: pass
    human_judgment: false
  - id: D3
    description: "AGENTS.md, the manual docs and the README describe the editor as built; generated docs show no drift"
    verification:
      - kind: other
        ref: "make generate-docs && make check-docs"
        status: pass
    human_judgment: true
    rationale: "Whether the prose is accurate and clear is a reader's judgment; only drift of the generated pages is checked by a tool"
  - id: D4
    description: "The owner's live checks and follow-ups are written down (real client rendering, two replicas, Telegram assumptions A1-A9, native-speaker read)"
    verification: []
    human_judgment: true
    rationale: "These checks need real Telegram clients, a deployed bot and native speakers; none can be automated"

duration: 55min
completed: 2026-11-04
status: complete
---

# Phase 11 Plan 14: Privacy and fit tests, rulebook and docs, live-check list and phase gate Summary

**Every screen of the message editor is now held to the Staff Group privacy rule and to Telegram's size limits in all 8 languages, a non-creator admin's whole private editor run (Set, a refused photo, a saved text, both previews) is proven to leak nothing, AGENTS.md and the manual docs describe the editor, and 11-SPIKE.md lists the owner's live checks.**

## Performance

- **Duration:** about 55 minutes (most of it the gate runs)
- **Tasks:** 3 (one tracer, two auto)
- **Files modified:** 10 (2 created)

## Accomplishments

- `settingsEditScreens` (`settings_edit_screens_test.go`) renders every non-page editor screen through the functions the code sends them with: the prompt of every part (replaced or not, three limits), each typed-reply refusal with its Cancel button, the saved page with every caption and album note, the wait-ended reply of each part, the editor page under each notice an action puts on it, and every plain reply and alert. `TestSettingsEditScreensFitEveryLocale` holds all of them to 1..3800 UTF-16 units, no unfilled placeholder, no omitted button and 1..64 bytes of callback data in all 8 languages with 64-rune ampersand, astral and multibyte titles, for a group view and a private view.
- `TestSettingsStaffPrivacy` renders the same screens (group view and private non-creator view, in all 8 languages) plus an explicit prompt, and has a new subtest in which an administrator who is not the creator (can_change_info, switch on) presses Set text, sends a photo (refused), sends a valid text (saved), then presses Preview text and Full preview. Every message, edit, button and answer the bot produced is checked for the Staff Group's title (raw and escaped), its marker and its chat ID. The run is also checked to have really produced the refusal, the still-waiting line, the saved line and the preview header.
- `renderSettingsInputRefusal` and `settingsInputSavedNotice` were extracted from `settingsInputReply.refuse` and `.saved` (no behaviour change), so the tests render what the door sends.
- `AGENTS.md`: the editor follows "One setting, one code path"; a new "Message editor" bullet ties the pieces together and holds the preview rules (actions `wv wf gv gf rv rf`, header then content through `renderGreeting` or `buildRulesText` and `media.SendGreeting`, only to the presser's private chat, never written, the 403 / failure / stale answers); the Testing section lists the remaining families and files and that the typed reply is the sixth door of `TestSettingsGateAllDoors`. `commands.md` (`/cancel`), `callbacks.md` (the `settings` prefix and the editor codes), `permissions.md` (the editor rights and the typed reply) and `handler-groups.md` (group 1) were updated, and the README Greetings section says the three messages can be set from `/settings` in private. `make generate-docs` changed nothing further and `make check-docs` shows no drift.
- `11-SPIKE.md` lists 11 non-blocking live checks (A3 GIF, A4 album, A5 never-started admin, A6 fast taps, A1/A2/A7/A8 long texts and many buttons, A9 typed message, real clients and truncation in hi/ru/uk, two replicas, the late reply, a native-speaker read of the 83 strings, `make lint` on Go 1.26 and CI `make test`) and the follow-ups (the `/rules` double escaping, Goodbye On by default, the anonymous `/cancel` reply), stating that none blocks the phase.

## Task Commits

1. **Task 1 (tracer): privacy and size tests over every editor screen** - `df877d0` (test; also the behaviour-neutral extraction in `settings_input_door.go`)
2. **Task 2: rulebook, manual docs and README** - `7ff58af` (docs)
3. **Task 3: the live-check list** - `3290717` (docs)

**Plan metadata:** the commit of this SUMMARY, STATE.md and ROADMAP.md (docs).

## Files Created/Modified

- `fuku/modules/settings_edit_screens_test.go` - `settingsEditScreens`, `settingsEditRefusalReasons`, `settingsEditPlainKeys` and `TestSettingsEditScreensFitEveryLocale`
- `fuku/modules/settings_staff_privacy_test.go` - editor screens in the pure renders, new typed-run subtest
- `fuku/modules/settings_input_door.go` - `renderSettingsInputRefusal`, `settingsInputSavedNotice`
- `AGENTS.md`, `README.md`, four docs pages - as above
- `.planning/phases/11-message-editor/11-SPIKE.md` - the owner's checklist

## Decisions Made

See `key-decisions`. The tracer gate (`<verify>` re-run) passed first time, so nothing was fixed in code: no screen leaked and none was over a limit.

## Deviations from Plan

**1. [Rule 3 - Blocking] The door's refusal and saved notice were inline, so a test could not render exactly what is sent**
- **Found during:** Task 1
- **Issue:** the plan says to "render the door's replies through the same helpers the door uses", but `refuse` built its keyboard and text inside the method and `saved` built its notice inline.
- **Fix:** extracted `renderSettingsInputRefusal` and `settingsInputSavedNotice` (same text, same keyboard, same order) and made the methods call them.
- **Files modified:** `fuku/modules/settings_input_door.go`
- **Verification:** `TestSettingsEdit*`, `TestSettingsInput*`, `TestSettingsGate*` and the two new tests all pass.
- **Committed in:** `df877d0`

**2. [Process] The plan commit ledger was not written before the first commit**
- **Issue:** protocol 0c (`gsd-plan-head-before-11-14`) was skipped. `plan_head_before` is the orchestrator's stated baseline `99078e7`, which is the commit this plan started from; `commits: 3` is `git rev-list --count 99078e7..HEAD` before this SUMMARY's commit.

**Total deviations:** 1 auto-fixed (Rule 3), 1 process note. No scope creep.

## Phase gate

| Command | Result |
|---------|--------|
| `go test -tags testtools -race -count=1 -run '^(TestSettingsStaffPrivacy\|TestSettingsEditScreensFitEveryLocale)$' ./fuku/modules` | passed (both `--- PASS`, 8 language subtests each; the new typed-run subtest ran) |
| `go test ... -run '^(TestSettingsStaffPrivacy\|TestSettingsEditScreensFitEveryLocale\|TestSettingsEdit\|TestSettingsInput\|TestSettingsGate)' ./fuku/modules` after the door extraction | ok (112 s) |
| `make test` | could not exit 0 in this sandbox, for two environment reasons. (1) No `covdata` tool: `go test -coverpkg` reports a `build-fail` for `fuku/utils/actionlog` (no test files). (2) New in this phase: with the 10 minute per-package `-timeout` of the Makefile, `fuku/modules` was killed at 600.4 s by `panic: test timed out after 10m0s` (running test `TestStaffUndoCardPresserOnly`, no test failure; twice, once with other commands running at the same time and once on an otherwise idle machine, 4 CPUs). Fallback: the whole `./...` run through the real pipeline flags (`-json -race -coverprofile -coverpkg`, 10 m) for every package, then `./fuku/modules` alone with the same flags and `-timeout 40m`: `ok fuku/modules 639.377s`, exit 0, no test failure and no `fail` event. Merging both JSON streams with only the covdata `build-fail` event of `actionlog` removed and feeding them to `go run ./scripts/check_test_results` exits 0 (no unexpected skips; 75.7% coverage line). CI's `make test` is the authority; its runner is faster than this sandbox, but `fuku/modules` is now within about 6% of the 10 minute limit here, so the Makefile's `-timeout 10m` is worth raising or the package worth splitting (see Issues Encountered) |
| `make check-translations` | exit 0, all 8 locales complete |
| `make generate-docs`, then `make check-docs` | no drift (exit 0); the generator changed nothing beyond the hand edits of the manual pages |
| `make lint` | could not run: golangci-lint was built with go1.25 and refuses the repo's Go 1.26.0 target ("can't load config"). Fallback: `go vet -tags testtools ./...` exit 0; `gofmt -l` over the three Go files this plan touched prints nothing (it still lists seven older, untouched files in `fuku/modules`); `python3 .../lint-local.py 72a692b` (dupl, gocyclo, godox, staticcheck, errcheck on added lines) exit 0 |
| `git diff --exit-code 72a692b -- go.mod go.sum` | exit 0 |

Acceptance greps: `grep -c renderSettingsEditPrompt` in the privacy test 2 (baseline 0); `grep -c '/cancel'` in `commands.md` 2 (baseline 0); `grep -c renderGreeting AGENTS.md` 2 and `grep -c 'Message editor' AGENTS.md` 1 (baseline 0 each); `` grep -c '`ws`' `` in `callbacks.md` 1; `grep -cE '^\| *1 *\|'` in `handler-groups.md` 1; the `MANUALLY MAINTAINED` marker is still in the first 512 bytes of `commands.md`; `grep -ci 'private chat' README.md` 2 (HEAD 1); `11-SPIKE.md` has "double escaping" once and 19 matches of the heading/number pattern.

## Issues Encountered

- `fuku/modules` takes 639 s under `-race -coverpkg` on this 4-CPU sandbox, over the Makefile's `-timeout 10m`. Earlier gates in this phase (and plan 10-10) passed inside the limit, so the package crossed it as the Phase 11 tests were added; this plan adds about 10 s of it. CI on `b2817b0` was green, so it is not a failure there yet, but there is little margin. Nothing was changed about it here (out of scope: the Makefile is not in this plan's file list); it is recorded as a follow-up for the orchestrator or the owner.
- The first `make test` attempt ran while `check-translations`, `go vet ./...` and the lint stand-in were also running, which only made the timeout earlier; the second attempt was on an idle machine and timed out the same way.

## Requirements

`requirements-completed: []`. EDIT-02 and EDIT-06 were completed by plans 11-11 and 11-12 and are already ticked; this plan re-confirms them with the final privacy and size checks and changes no checkbox or traceability row. EDIT-01, EDIT-03, EDIT-04, EDIT-05, EDIT-10 and EDIT-11 were completed by plans 11-09, 11-11, 11-13, 11-13, 11-08 and 11-13.

## Known Stubs

None. No file this plan changed holds a placeholder, an empty default flowing to the UI or a TODO.

## Threat Flags

None. This plan adds no endpoint, auth path, file access or schema; the only production change is a behaviour-neutral extraction in the typed-reply door.

## User Setup Required

None - no external service configuration required. The owner's optional live checks are in `11-SPIKE.md`.

## Next Phase Readiness

All 14 plans of Phase 11 have a SUMMARY. The orchestrator can verify the phase. Open items for the owner: the live checks and follow-ups in `11-SPIKE.md`, and the `fuku/modules` test time against the 10 minute `make test` timeout.

## Self-Check: PASSED

- `fuku/modules/settings_edit_screens_test.go`, `11-SPIKE.md`: present.
- Commits `df877d0`, `7ff58af`, `3290717`: ancestors of HEAD.

---
*Phase: 11-message-editor*
*Completed: 2026-11-04*
