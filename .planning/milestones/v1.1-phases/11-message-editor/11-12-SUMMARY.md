---
phase: 11-message-editor
plan: 12
subsystem: modules
tags: [settings-menu, message-editor, preview, full-preview, tdd]
requires:
  - phase: 11-message-editor
    provides: plan 11-04 locale keys, plan 11-05 renderGreeting and buildRulesText, plan 11-08/11-09 the editor pages and part rows
provides:
  - Preview of one part (wv, gv, rv) and Full preview (wf, gf, rf) on the Welcome, Goodbye and Rules pages
  - the preview row block of renderSettingsEditor (text, media, buttons, Full preview, two to a row)
  - settingsEditPreview, settingsEditFullPreview, settingsEditPreviewSend, settingsEditPreviewAnswer, settingsEditIsForbidden
affects: [11-13, 11-14, settings-message-editor]

actuals:
  tokens: 21000
  tasks: 3
  commits: 5

commits: 5
plan_head_before: b2817b094cc9f63012b4d951dd88c7704a9df143
plan_head_after: a28d8b63ac2efb1e8076d10a349738977357c938

tech-stack:
  added: []
  patterns:
    - "A preview is built by the join's own builder (renderGreeting) and sent by the group's own sender (media.SendGreeting) to query.From.Id only, so it is byte for byte the member message and cannot reach the group"
    - "Preview and Full preview share one runner (settingsEditRunPreview, part \"\" is Full) so the stale-press refusal, the read-error edit and the answer are written once"
    - "A send failure is classified by Telegram's code (403 is need-start), never by wording; the wording goes to the log only"

key-files:
  created:
    - fuku/modules/settings_edit_preview.go
    - fuku/modules/settings_edit_preview_test.go
  modified:
    - fuku/modules/settings_edit.go
    - fuku/modules/settings_edit_actions.go
    - fuku/modules/settings_edit_test.go
    - .planning/REQUIREMENTS.md

key-decisions:
  - "A Welcome or Goodbye text is always previewable (the default text is a message too); media, buttons and the Rules text only while set, so a stale or forged press of an empty part answers settings_edit_nothing_to_preview with no send."
  - "Preview buttons renders the buttons from renderGreeting with an empty text, as the plan says, so the {rules} placeholder of the text does not add the Rules button and no member-count or rules lookup runs for a buttons-only preview."
  - "The preview action codes are registered for all three pages in settingsEditActions from the page definitions (PreviewCode, FullCode), not per page in init; Full preview takes no extra field and Preview takes t, checked by settingsEditValidatePart before any I/O."

requirements-completed: [EDIT-06]

coverage:
  - deliverable: "Full preview sends the header and the exact member message (renderGreeting, media.SendGreeting) to the owner in private, filled with the owner's name and the group's title"
    requirement: EDIT-06
    verification:
      - kind: test
        ref: "fuku/modules/settings_edit_preview_test.go#TestSettingsEditFullPreview"
        status: pass
  - deliverable: "Preview text, media and buttons send one part alone for Welcome and Goodbye (every media kind through its own send method); Rules rv and rf send what /rules shows, also with private rules on"
    requirement: EDIT-06
    verification:
      - kind: test
        ref: "fuku/modules/settings_edit_preview_test.go#TestSettingsEditPreview"
        status: pass
      - kind: test
        ref: "fuku/modules/settings_edit_preview_test.go#TestSettingsEditRulesPreview"
        status: pass
  - deliverable: "A preview writes nothing, deletes nothing and posts nothing into the group"
    requirement: EDIT-06
    verification:
      - kind: test
        ref: "fuku/modules/settings_edit_preview_test.go#TestSettingsEditPreviewWritesNothing"
        status: pass
  - deliverable: "Every outcome has its answer: toast from a group menu, empty from a private one, need-start on a 403, failed on a 400 or 429, nothing-to-preview on a stale press, invalid request before any lookup"
    requirement: EDIT-06
    verification:
      - kind: test
        ref: "fuku/modules/settings_edit_preview_test.go#TestSettingsEditPreviewErrors"
        status: pass
  - deliverable: "The pages with their preview rows match the contract's mock states row by row, keep every button within 64 bytes and fit 3800 UTF-16 units in all 8 languages; at most 8 rows private and 9 in a group menu"
    requirement: EDIT-06
    verification:
      - kind: test
        ref: "fuku/modules/settings_edit_test.go#TestSettingsEditPages"
        status: pass
      - kind: test
        ref: "fuku/modules/settings_edit_test.go#TestSettingsEditCallbackBytes"
        status: pass
      - kind: test
        ref: "fuku/modules/settings_edit_test.go#TestSettingsEditPagesFitEveryLocale"
        status: pass
  - deliverable: "On a real client a GIF stored as a document and resent through the group's send path renders as the owner expects in Full preview (research A3)"
    requirement: EDIT-06
    verification:
      - kind: backstop
        ref: "owner's live check in 11-SPIKE.md (written by plan 11-14)"
        status: not-run
    human_judgment: true
  - deliverable: "Fast taps on Preview are not retried and never write; a 429 on the second send answers preview_failed and the next press works (research A6)"
    requirement: EDIT-06
    verification:
      - kind: test
        ref: "fuku/modules/settings_edit_preview_test.go#TestSettingsEditPreviewErrors (the 429 case, as far as a fake can show it)"
        status: pass
      - kind: backstop
        ref: "owner's live check in 11-SPIKE.md (real rate limits)"
        status: not-run
    human_judgment: true

duration: 50min
completed: 2026-11-04
status: complete
---

# Phase 11 Plan 12: Preview and Full preview Summary

**Every editor page now has Preview buttons for the parts that are set and a Full preview that sends the owner, in private, the header and exactly the message members get (the join's builder and sender, filled with the owner's name and the group's title), from either menu and without writing anything.**

## What changed

- `fuku/modules/settings_edit_preview.go` (new): `settingsEditIsForbidden` (403 by code, also wrapped), `settingsEditPreviewSend` (header through `SendMessage`, then the Rules or the greeting content), `settingsEditPreviewRules` (`buildRulesText` for Full, `normalizeRulesForHTML` for the text part), `settingsEditPreviewGreeting` (`renderGreeting` plus `media.SendGreeting`: Full as the join sends it, text with no keyboard, media with no caption and no keyboard, buttons under `settings_edit_preview_buttons_body`), `settingsEditPreviewAnswer`, `settingsEditRunPreview`, `settingsEditPreview`, `settingsEditFullPreview`.
- `fuku/modules/settings_edit.go`: parts gained `PreviewLabelKey`, page definitions gained `PreviewCode` and `FullCode` (wv wf, gv gf, rv rf), `settingsEditPreviewable` and the `settingsEditPreviewRows` block right after the part rows (text, media, buttons, Full preview last, two to a row).
- `fuku/modules/settings_edit_actions.go`: `settingsEditActions` registers Preview (`t`, `Validate`, `settingsRightAdmin`) and Full preview (no extra field, `settingsRightAdmin`) for all three pages.
- `fuku/modules/settings_edit_preview_test.go` (new): `settingsEditFakeSends`, a getChat title wrapper, and `TestSettingsEditFullPreview`, `TestSettingsEditPreview`, `TestSettingsEditRulesPreview`, `TestSettingsEditPreviewWritesNothing`, `TestSettingsEditPreviewErrors`.
- `fuku/modules/settings_edit_test.go`: `TestSettingsEditPages` gained one subtest per mock state (rows compared by short button names, private and group), `TestSettingsEditCallbackBytes` and `TestSettingsEditPagesFitEveryLocale` assert the all-set page draws every Preview button and Full preview, and the callback-bytes test also holds the 8 and 9 row limits for every state.
- `.planning/REQUIREMENTS.md`: EDIT-06 ticked (checkbox and traceability row).

## Task commits

| Task | Commit | Subject |
|------|--------|---------|
| 1 (tracer) RED | acefe67 | test(11-12): add failing test for Full preview of a Welcome sent to the owner in private |
| 1 (tracer) GREEN | a50660e | feat(11-12): add Full preview to the Welcome and Goodbye editor pages |
| 2 RED | 1f40d15 | test(11-12): add failing tests for one-part previews, the Rules preview, no writes and honest errors |
| 2 GREEN | 7376b59 | feat(11-12): add Preview of one part and the Rules previews to the message editor |
| 3 | a28d8b6 | test(11-12): hold the editor pages with their preview rows to the contract's mocks, byte cap and every language |

## TDD notes

- Task 1 RED (acefe67): `TestSettingsEditFullPreview` ran and failed on its planned assertion, "the Welcome page has no Full preview button", with the build clean and the target test named in the `--- FAIL` line. GREEN (a50660e) passes it. In GREEN I fixed a test-side bug the RED could not show: my getChat title wrapper round-tripped the answer through `map[string]any`, which rounds the huge test chat IDs, so it now keeps every other field as raw JSON.
- Task 2 RED (1f40d15): on the planned assertions, no build failure: `TestSettingsEditPreview` (no Preview button on either page, no media-kind call), `TestSettingsEditRulesPreview` (both subtests), `TestSettingsEditPreviewWritesNothing` (the wv press answered invalid request) and `TestSettingsEditPreviewErrors` (nothing-to-preview and the part cases). The 403/400/429 and group-toast subtests passed already, because they press Full preview from Task 1; they are kept as the honest-error contract. GREEN (7376b59) passes all. A test-side fix in GREEN: the recorded media parameter is a struct pointer, so the test checks it contains the file ID.
- Task 3 had no failing step: the renderer already met the mocks (the preview block was written to them in Task 2), so the new row tests passed first run. I checked they are not vacuous by mutating the renderer to one preview button per row: `TestSettingsEditPages` failed; the mutation was restored.
- `gsd_run check tdd-red-evidence` was not run (the plan is `type: execute`, the same as 11-09); the RED runs were read by hand against the target test names.
- Tracer gate: after Task 1 the verify command was re-run (pass) before expanding.

## Verification (real results)

- `go test -tags testtools -race -count=1 -run '^TestSettingsEditFullPreview$' ./fuku/modules`: ok (Task 1).
- `... -run '^(TestSettingsEditPreview|TestSettingsEditRulesPreview|TestSettingsEditPreviewWritesNothing|TestSettingsEditPreviewErrors|TestSettingsEditFullPreview)$'`: ok, every subtest PASS (Task 2).
- `... -run '^(TestSettingsEditPages|TestSettingsEditCallbackBytes|TestSettingsEditPagesFitEveryLocale)$' -v`: PASS with a subtest per mock state (`w`/`g` nothing set, text and media set, everything set, only buttons set; Rules set and not set) and per language (Task 3).
- Whole settings set `go test -tags testtools -race -count=1 -run '^(TestSettings|TestLoadSettings)' ./fuku/modules ./fuku/i18n` at the final code state: `ok fuku/modules 203.127s`, `ok fuku/i18n 6.126s`.
- `CGO_ENABLED=0 go build ./...`: ok. `go vet -tags testtools ./fuku/modules`: clean. `git diff --exit-code 72a692b -- go.mod go.sum`: exit 0. `gofmt -l` on the settings_edit files prints nothing.
- `python3 .../lint-local.py 72a692b` (before the Task 1 GREEN, the Task 2 GREEN and at the end): "no dupl, gocyclo or godox findings".
- Acceptance greps: `renderGreeting(` and `media.SendGreeting(` present in the preview file; the forbidden-names grep (`SendWelcomeMessage|SetCleanWelcomeMsgId|SetCleanGoodbyeMsgId|DeleteMessage` outside comments) prints 0; the six codes `wv gv rv wf gf rf` in settings_edit.go count 6; `buildRulesText(` count 1.
- Not run: `make lint` (golangci-lint here is built with Go 1.25), `make test`, `make check-translations` and `make check-docs` (no locale or generated-docs source changed; the 11-04 keys already exist), PostgreSQL targets (nothing here touches the schema).

## Deviations from Plan

None in behaviour. Notes:

- Task 1 draws no Preview row and registers no Preview action yet (and Rules had no Full preview code until Task 2), so no commit ships a dead button; the renderer's temporary empty-code guards were removed in Task 2 once every page had both codes.
- No existing test's expectation changed: the existing page tests passed unchanged with the preview rows, and Task 3 only added to them.
- AGENTS.md is not edited by this plan (the plan lists none); plan 11-14 writes the editor's AGENTS.md text, including the previews, as its plan says.

## Requirements

- EDIT-06 is COMPLETE and ticked (checkbox and traceability row): each part has a Preview (Welcome and Goodbye text always, the other parts while set; Rules text while set), and Full preview sends the whole message through `renderGreeting` and `media.SendGreeting`, the join path's own builder and sender (and Rules through `buildRulesText`, which `/rules` uses). The owner's live checks of how a GIF and real rate limits behave stay in 11-SPIKE.md (plan 11-14) as non-blocking backstops.

## Known Stubs

None.

## Threat Flags

None beyond the plan's register. T-11-47 (a preview posted into the group) and T-11-48 (a preview deleting or rewriting the last welcome) are covered by `TestSettingsEditPreviewWritesNothing` and the grep gate; T-11-49 by `TestSettingsEditPreviewErrors`; T-11-50 by `TestSettingsEditCallbackBytes`.

## Self-Check: PASSED

- Created files exist: `fuku/modules/settings_edit_preview.go`, `fuku/modules/settings_edit_preview_test.go`.
- Commits acefe67, a50660e, 1f40d15, 7376b59 and a28d8b6 are ancestors of HEAD; `git rev-list --count b2817b0..HEAD` printed 5 before this SUMMARY.
