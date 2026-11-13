---
phase: 12-rules-media-media-under-text-and-greet-once
plan: 14
subsystem: settings-editor
tags: [rules, media, caption-above, preview, greet-once, group-language, settings-menu]
requires:
  - phase: 12-rules-media-media-under-text-and-greet-once
    provides: 12-03 lang.GetGroupLanguage, 12-04 media.SendGreetingMessage and CaptionAboveEligible, 12-07 buildRulesMessage and sendRulesMessage (degrade flag), 12-09 greet-once claim in SendWelcomeMessage, 12-11/12-12 Media under text switches and Rules parts, 12-13 Rules media and buttons through the door
provides:
  - Full preview of a Welcome or Goodbye through media.SendGreetingMessage with CaptionAbove from the stored Media under text switch
  - the Rules Full preview as the exact /rules group-form message (buildRulesMessage in the group's language read by lang.GetGroupLanguage, sendRulesMessage with degrade false)
  - Rules Preview media (media alone) and Preview buttons (body line with the rules keyboard) through the shared settingsEditPreviewMediaAlone and settingsEditPreviewButtonsAlone
  - proof that no preview claims a greet-once member or writes a greetings, rules, chats or greeted_members row
  - AGENTS.md previews paragraph rewritten, the EDIT-07 limit sentence, and 12-SPIKE.md created with the EDIT-07 live check
affects: [12-16]

actuals:
  tokens: 21000
  tasks: 3
  commits: 5

commits: 5
plan_head_before: e9bff30
plan_head_after: 5c63976

tech-stack:
  added: []
  patterns:
    - "A preview is compared with what members receive by decoding both sends the way Telegram receives them (method, text or caption, file, parse mode, caption placement, keyboard) and ignoring the chat and the reply target"
    - "Preview media and Preview buttons are two shared helpers used by Welcome, Goodbye and Rules, so no branch is a clone"

key-files:
  created:
    - .planning/phases/12-rules-media-media-under-text-and-greet-once/12-SPIKE.md
  modified:
    - fuku/modules/settings_edit_preview.go
    - fuku/modules/settings_edit_preview_test.go
    - AGENTS.md

key-decisions:
  - "The Rules Full preview keeps its old request options (parse mode HTML, no reply, no link-preview option) and passes degrade false, so a refused media shows as settings_edit_preview_failed and never as a different message (research S-03)"
  - "With private rules on, Full preview still shows the group form in the group's language (UI-SPEC Assumption A12, owner confirmation open)"
  - "Previews ignore Greet once and never write (UI-SPEC Assumption A9)"

patterns-established:
  - "settingsEditSeen / settingsEditSeenSince: the one-message view of a send, reused to assert 'the preview equals what /rules sends'"

requirements-completed: [EDIT-07, EDIT-08]

coverage:
  - id: D1
    description: "Full preview of a Welcome photo, a Goodbye video and a Rules animation with Media under text on carries show_caption_above_media true; with the switch on and a document, sticker or audio, or with the switch off, no call carries it; Preview media never carries it"
    requirement: "EDIT-07"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestSettingsEditFullPreviewMediaBelow$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D2
    description: "The Rules Full preview (text-only, media-only, empty, photo with buttons and Media under text, also while private rules is on) equals the message /rules sends in the group, its heading in the group's language and its header line in the presser's"
    requirement: "EDIT-08"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestSettingsEditRulesPreview|TestSettingsEditRulesPreviewGroupLanguage)$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D3
    description: "Rules Preview media sends the media alone, Preview buttons the body line with the rules keyboard as stored, Preview text the stored text; a stale Preview of an empty part answers settings_edit_nothing_to_preview and sends nothing; a group menu answers the toast and sends only to the presser"
    requirement: "EDIT-08"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestSettingsEditRulesPreviewParts$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D4
    description: "A preview never claims a greet-once member or writes a row: Full preview and Preview text of a Welcome with Greet once on leave no greeted_members row and the member's real join is welcomed and recorded; Rules previews and a group with no rows create no greetings, rules, chats or greeted_members row"
    requirement: "EDIT-09"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestSettingsEditPreviewNeverClaims|TestSettingsEditPreviewWritesNothing|TestGreetOnceFirstJoinOnly)$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D5
    description: "The EDIT-07 limit (a GIF stored before Phase 12 is a document and gets no switch until set again) holds on a real group"
    requirement: "EDIT-07"
    verification:
      - kind: human
        ref: ".planning/phases/12-rules-media-media-under-text-and-greet-once/12-SPIKE.md section 'EDIT-07 limit: a GIF stored before Phase 12'"
        status: deferred
    human_judgment: true

duration: 95min
completed: 2026-11-05
status: complete
---

# Phase 12 Plan 14: Previews show media under text and the exact Rules message Summary

**Full preview now sends what members receive: a Welcome or Goodbye through `media.SendGreetingMessage` with the stored Media under text, and the Rules page's Full preview through `buildRulesMessage` and `sendRulesMessage` in the group's language, with Rules Preview media and buttons each shown alone and no preview ever claiming a greet-once member or writing a row.**

## What changed

- `settings_edit_preview.go`: `settingsEditPreviewRules` builds the row from the page state and, for Full preview, calls `buildRulesMessage(i18n.MustNewTranslator(lang.GetGroupLanguage(ctx, p.group.Id)), p.group.Title, &row, rulesFormGroup)` and `sendRulesMessage(..., &gotgbot.SendMessageOpts{ParseMode: formatting.HTML}, false)`; Preview text is the stored rules, `m` and `b` use the shared helpers. `settingsEditPreviewGreeting` Full preview calls `media.SendGreetingMessage` with `CaptionAbove: st.MediaBelow`. `settingsEditPreviewMediaAlone` and `settingsEditPreviewButtonsAlone` are shared by all three pages. The header comment now says the heading is built in the group's language. The 403 classification and every earlier preview behaviour are untouched.
- `settings_edit_preview_test.go`: `TestSettingsEditRulesPreview` rewritten (Full preview equals what `/rules` sends, five subtests including the tracer), new `TestSettingsEditFullPreviewMediaBelow`, `TestSettingsEditRulesPreviewParts`, `TestSettingsEditRulesPreviewGroupLanguage`, `TestSettingsEditPreviewNeverClaims`, extended `TestSettingsEditPreviewWritesNothing` (now two subtests), and the helpers `settingsEditSeen`, `settingsEditSeenSince`, `settingsEditNewSends`, `settingsEditRulesPreviewEnv`, `settingsEditSetLanguages`, `settingsEditRowCounts`, `settingsEditCleanUsers`.
- `AGENTS.md` (Subsystem traps, "Message editor" bullet, plus the "from plan 12-14" wording of the Rules message bullet): Full preview of a Welcome or Goodbye is sent by `media.SendGreetingMessage` with the stored Media under text, Preview of one part by `media.SendGreeting`, previews never call `SendWelcomeMessage` or `ClaimGreeting`, the Rules Full preview is built by `buildRulesMessage` in the group's language read by `lang.GetGroupLanguage` (uk on a miss, no row) and sent without the degrade, and the EDIT-07 limit sentence.
- `12-SPIKE.md` created with the title line and one section, `## EDIT-07 limit: a GIF stored before Phase 12`, holding the sentence and a five-step live check; plan 12-16 adds its own checks around it.
- No locale key was added; the 12-01 goldens are untouched.

## Requirements

`requirements-completed: [EDIT-07, EDIT-08]`. Both are ticked in REQUIREMENTS.md (checkbox and traceability row); EDIT-09 was already ticked and was not touched.

EDIT-07 limit (D-07, D-08): a GIF stored before Phase 12 is still a document, shows exactly as before and gets no Media under text switch until the owner sets that GIF again, which stores it as an animation and makes the switch appear.

The limit does not weaken the completion claim: EDIT-07 is complete as D-07 and D-08 decided. The same sentence is in AGENTS.md and in `12-SPIKE.md` (with a live check, which needs a real group and is deferred to the owner / plan 12-16).

## TDD record

- **Task 1 (tracer)** RED `9d80647`: the rewritten `TestSettingsEditRulesPreview` failed on assertions (Full preview sent `sendMessage` of the bare no-rules line where `/rules` sent a photo, and the heading was in the presser's language). GREEN `c13277d`.
- **Task 2** RED `3cf7b15`: `TestSettingsEditFullPreviewMediaBelow` failed (a Welcome photo and a Goodbye video carried no `show_caption_above_media`; Rules Preview media sent the whole message with a caption and keyboard) and `TestSettingsEditRulesPreviewParts` failed (Preview buttons sent no body line). GREEN `c55c09d`. `TestSettingsEditRulesPreviewGroupLanguage` passed at once because the Task 1 commit already built the group-language behaviour; it is a regression guard.
- **Task 3** the tests (`TestSettingsEditPreviewNeverClaims`, the extended `TestSettingsEditPreviewWritesNothing`) passed on first run, because no earlier task made a preview claim or write; they are regression guards. Commit `5c63976` (tests, AGENTS.md and `12-SPIKE.md` together, as the plan says). There is no separate RED commit for Task 3.

## Commits

- 9d80647 test(12-14): the Rules Full preview must equal the message /rules sends
- c13277d feat(12-14): the Rules Full preview is the message members receive
- 3cf7b15 test(12-14): Full preview shows media under text and Rules previews each part alone
- c55c09d feat(12-14): previews show media under text and every Rules part
- 5c63976 test(12-14): previews never claim a member or write a row

## Verification (real results)

- Task 1: `-run '^(TestSettingsEditRulesPreview|TestRulesMediaAndButtonsInGroup)$' -v ./fuku/modules` ok. Acceptance greps: `lang.GetGroupLanguage(` 1, `buildRulesMessage(` 1, "presser's translator" 0.
- Task 2: `-run '^(TestSettingsEditFullPreviewMediaBelow|TestSettingsEditRulesPreviewParts|TestSettingsEditRulesPreviewGroupLanguage|TestSettingsEditFullPreview|TestSettingsEditPreview|TestSettingsEditPreviewErrors|TestSettingsEditRulesPreview)$' ./fuku/modules` ok. `SendGreetingMessage(` appears once in `settings_edit_preview.go`.
- Task 3: `-run '^(TestSettingsEditPreviewNeverClaims|TestSettingsEditPreviewWritesNothing|TestGreetOnceFirstJoinOnly)$' ./fuku/modules` ok; the superseded fragment "presser's language (`buildRulesText`" is gone from AGENTS.md (0 matches); `GetGroupLanguage` is in AGENTS.md; "a GIF stored before Phase 12" is in AGENTS.md and `12-SPIKE.md`.
- Plan end: `-run '^(TestSettings|TestLoadSettings)' ./fuku/modules ./fuku/i18n` ok (modules 262.4 s, i18n 5.4 s), run once after the last code commit; `-run '^(TestRules|TestLegacyGreetings|TestGreetOnce|TestGreetingsMediaBelow)' ./fuku/modules` ok.
- `CGO_ENABLED=0 go build ./...` ok; `go vet -tags testtools ./fuku/modules` clean; `gofmt -l` on the touched Go files empty; `make generate-docs` then `make check-docs` no drift; `make check-translations` all present; `git diff --exit-code 72a692b -- go.mod go.sum` exit 0.
- `python3 .../lint-local.py 72a692b`: exit 0 before the last code commit and again before this SUMMARY (the plan names `bd2f794` and the phase-11 script path; the brief's base `72a692b` and script were used).
- Not run: `make lint` (the installed golangci-lint is built with Go 1.25), `make test`, any PostgreSQL-only test (this plan adds none; `TestGreetedMemberClaimConcurrent` and `TestRulesMediaMigrationDefaults` stay with `make test-postgres-integrity` in CI), the live check of `12-SPIKE.md` (needs a real group).

## Deviations from Plan

### Auto-fixed Issues

None - plan executed as written, with these documented adjustments:

**1. [Documented] Test hygiene beyond the plan text.** Every test of this plan that drives a press or a join with a user ID cleans the `users` rows and cache entries (`settingsEditCleanUsers`, `settingsEditSetLanguages`, `greetOnceCleanup`), as the brief asks after the 12-09 CI failure; the greetings, rules and greeted_members rows go through `editCleanup`.

**2. [Documented] The tracer's Full preview of a Rules `m` or `b` part** briefly sent the whole message between commits `c13277d` and `c55c09d`; Task 2 replaced it with the part previews.

**3. [Documented] Task 3 has no separate failing-test commit** (see the TDD record).

## Known Stubs

None.

## Threat Flags

None. No new endpoint, auth path, file access or schema change; every preview still sends to `p.query.From.Id` only (T-12-45, `TestSettingsEditRulesPreviewParts` group-menu case), claims and writes nothing (T-12-46, `TestSettingsEditPreviewNeverClaims`, `TestSettingsEditPreviewWritesNothing`), and the group language read creates no chats row (T-12-47, `TestSettingsEditRulesPreviewGroupLanguage`). No log line was added. `go.mod` and `go.sum` are unchanged (T-12-SC).

## Self-Check: PASSED

The created `12-SPIKE.md` and the five commits above exist and are ancestors of HEAD.
