---
phase: 12-rules-media-media-under-text-and-greet-once
plan: 13
subsystem: settings-editor
tags: [rules, media, buttons, typed-reply-door, caption-limit, settings-menu]
requires:
  - phase: 12-rules-media-media-under-text-and-greet-once
    provides: 12-05 locale keys settings_edit_refuse_text_rules_long and settings_edit_refuse_media_rules_long, 12-07 the rules builder (heading plus normalized text is the caption), 12-12 Rules parts x m b, applyRulesMedia, applyRulesButtons, settingsEditRulesStateOf
provides:
  - the typed-reply door and the pure check accept Rules media and buttons (the Rules arm of settingsEditCheck is gone)
  - settingsInputReply.applyRules, a part switch over applyRulesText, applyRulesMedia and applyRulesButtons
  - settingsEditRulesLongest, the one heading-aware Rules measure shared by the Rules text part and the Rules media part
  - settingsEditMediaCaptionRefusal (Welcome and Rules caption refusals in one place)
  - settingsEditPromptMax: stored media gives 1024 on every page, then 3800 on Rules, else 4096
affects: [12-14, 12-16]

actuals:
  tokens: 14500
  tasks: 3
  commits: 5

commits: 5
plan_head_before: 1f1e7c64c656eed5655e0e79624e3b443dc1d4f4
plan_head_after: a3a646988161a21517616243da6dedef3f8e9c90

tech-stack:
  added: []
  patterns:
    - "the Rules text and media checks share settingsEditRulesLongest; the Welcome's caption predicates (settingsEditPartIsSet, the media-type-agnostic 1024 rule) are reused, never a Rules-only kind list"

key-files:
  created:
    - fuku/modules/settings_input_rules_test.go
  modified:
    - fuku/modules/settings_edit_check.go
    - fuku/modules/settings_edit_actions.go
    - fuku/modules/settings_input_door.go
    - fuku/modules/settings_edit_check_test.go
    - fuku/modules/settings_edit_actions_test.go
    - fuku/modules/settings_edit_screens_test.go
    - AGENTS.md

key-decisions:
  - "UI-SPEC row 3 versus check 8: with media Rules refuses with the two new keys, without media the existing settings_edit_refuse_too_long_filled stays (research Open Question 10); the existing 4097 no-media case keeps asserting it and the check-8 sentence is tested for with-media refusals only, as the plan resolved"
  - "The media side measures heading plus the stored rules text normalized for HTML; an empty text measures the heading alone, which over-counts by the heading's two newline units (conservative)"
  - "Rules buttons count no {rules} token of the stored text, because /rules adds no rules button (the stored text passed to settingsEditButtonRows is empty on Rules)"

patterns-established:
  - "A multi-part door test opens each person's private menu once (the deep link asserts exactly one private message), so a test that needs two waits uses a second owner"

requirements-completed: []

coverage:
  - id: D1
    description: "After Set media or Set buttons on the Rules page the owner's next private message becomes the Rules media or buttons, stored as the Welcome stores the same message (same file ID and type, a GIF as db.ANIMATION, same buttons), with the saved page; /rules then sends the photo with heading plus text as its caption and the button"
    requirement: "EDIT-08"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestSettingsEditRulesTypedParity|TestSettingsEditRulesParity|TestSettingsEditDoorStoresEveryPart|TestSettingsEditMediaParity)$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D2
    description: "One caption limit: with Rules media of any kind stored a Rules text whose heading plus text is over 1024 is refused with settings_edit_refuse_text_rules_long and its number; a Rules media over a stored text whose heading plus text is over 1024 is refused with settings_edit_refuse_media_rules_long; with no media the old 4096 limit and key stand; the Rules and Welcome checks agree at 1000, 1024 and 1025 for every stored kind"
    requirement: "EDIT-08"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestSettingsEditCheckRules|TestSettingsRulesCaptionParity|TestSettingsEditPromptMax|TestSettingsEditMediaTextLong)$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D3
    description: "A Rules wait keeps the wait with the reason, settings_edit_still_waiting and one Cancel button for a text or video note (media), a long stored text, a bad link, 101 buttons; an album stores one item and adds settings_edit_album_cut; a caption is ignored and not echoed; a bot that lost its administrator right ends the wait with chat_status_bot_not_admin and stores nothing; the two caption refusals fit 3800 units and 64-byte buttons in all 8 languages"
    requirement: "EDIT-08"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestSettingsEditRulesTypedRefusals|TestSettingsEditScreensFitEveryLocale|TestSettingsEditDoorAuthority|TestSettingsStaffPrivacy)$' ./fuku/modules"
        status: pass
    human_judgment: false

duration: 70min
completed: 2026-11-05
status: complete
---

# Phase 12 Plan 13: Rules media and buttons through the typed-reply door Summary

**The owner now sends a photo (or any of the Welcome's media kinds) and button lines to a Rules wait in the bot's private chat, they are stored by `applyRulesMedia` and `applyRulesButtons` exactly as the Welcome stores the same message, and the editor refuses, with the true number, a rules message Telegram could not send as one message.**

## What changed

- `settings_edit_check.go`: the "Rules has a text part only" arm of `settingsEditCheck` is removed, so Rules media and buttons reach `settingsEditCheckMedia` and `settingsEditCheckButtons`; `settingsEditRulesLongest(title, normalized)` is the one heading-aware measure (max over both heading forms and 8 languages); `settingsEditCheckRulesText` holds heading plus text to 1024 while any media is stored (`settings_edit_refuse_text_rules_long`) and keeps the old 4096 check and `settings_edit_refuse_too_long_filled` without media; the media caption test moved into `settingsEditMediaCaptionRefusal`, whose Rules branch measures with `settingsEditRulesLongest` (`settings_edit_refuse_media_rules_long`, no placeholder fill) and whose Welcome branch is unchanged; the Rules buttons check passes an empty stored text to `settingsEditButtonRows`, so a `{rules}` token adds no button.
- `settings_input_door.go`: `settingsInputReply.applyRules` switches by part over `applyRulesText`, `applyRulesMedia` and `applyRulesButtons`; the door's order, the bot-admin re-check and every refusal path are untouched.
- `settings_edit_actions.go`: `settingsEditPromptMax` gives 1024 with stored media on every page (Rules too), then 3800 on Rules, else 4096.
- Tests: new `settings_input_rules_test.go` (`TestSettingsEditRulesTypedParity`, `TestSettingsEditRulesTypedRefusals`, helper `settingsRulesCleanUsers`); `settings_edit_check_test.go` (extended `TestSettingsEditCheckRules`, new `TestSettingsRulesCaptionParity`, helpers `settingsEditRulesCheckState`, `settingsEditRulesMediaCheck`); `settings_edit_actions_test.go` (`TestSettingsEditPromptMax` with Rules media, including the private prompt showing Up to 1024); `settings_edit_screens_test.go` (the two refusals with n 1180 and max 1024).
- `AGENTS.md`: the Set sentence (prompt limit), the typed-reply door paragraph (Rules parts through `applyRules`) and the Message editor trap bullet (shared predicates, the 1024 rule, `settingsEditRulesLongest`, the kept 4096 key without media, no `{rules}` button count on Rules).

## TDD record

- **Task 1 (tracer)** RED `2abee31`: `TestSettingsEditRulesTypedParity` failed on assertions for every subtest (the reply was `settings_edit_refuse_need_text`, "the Rules media was not stored", "the rules hold 0 buttons"). GREEN `1882d14`. The test file was corrected inside the GREEN commit: the RED version opened a second private menu for one owner, which the deep link helper forbids, so the end-to-end `/rules` subtest uses a second owner, and `settingsRulesCleanUsers` was added there.
- **Task 2** RED `6d40903`: the new `TestSettingsEditCheckRules` subtests, `TestSettingsRulesCaptionParity` and the Rules cases of `TestSettingsEditPromptMax` failed on assertions (no with-media text refusal, a Rules media over a long stored text accepted, with-media 4097 still `too_long_filled`, Rules prompt 3800 with media). Semantic assessment: the target tests executed and failed on the planned assertions for the intended reason (no build failure, no zero-test run). `gsd_run check tdd-red-evidence` was not run. GREEN `c84a450`.
- **Task 3** the tests of `TestSettingsEditRulesTypedRefusals` and the screens rows passed on first run, because the behaviour they pin arrived with the Task 1 and Task 2 commits; they are regression guards and the commit is a single `test(12-13)` commit with the AGENTS.md edits (`a3a6469`). There is no separate RED for Task 3.

## Commits

- 2abee31 test(12-13): the owner types the rules media and buttons
- 1882d14 feat(12-13): the owner types the rules media and buttons
- 6d40903 test(12-13): the rules caption limit holds at the Welcome's boundary for every media kind
- c84a450 feat(12-13): hold the rules to one message's caption limit
- a3a6469 test(12-13): the door handles Rules media and buttons like every part

## Verification (real results)

- Task 1: `-run '^(TestSettingsEditRulesTypedParity|TestSettingsEditRulesParity|TestSettingsEditDoorStoresEveryPart|TestSettingsEditMediaParity)$' ./fuku/modules` ok. Acceptance greps: no "Rules has a text part only" in the check; `applyRulesMedia(`/`applyRulesButtons(` appear 2 times in the door.
- Task 2: `-run '^(TestSettingsEditCheckRules|TestSettingsRulesCaptionParity|TestSettingsEditPromptMax|TestSettingsEditMediaTextLong|...)$' ./fuku/modules` ok (85 s, the check tests install the real locales per call). Acceptance greps: 2 lines name the two new keys, 3 occurrences of `settingsEditRulesLongest(`.
- Task 3: `-run '^(TestSettingsEditRulesTypedRefusals|TestSettingsEditScreensFitEveryLocale|TestSettingsEditDoorAuthority|TestSettingsStaffPrivacy)$' ./fuku/modules` ok; `grep -c settings_edit_refuse_text_rules_long AGENTS.md` is 1.
- Plan end: `-run '^(TestSettings|TestLoadSettings)' ./fuku/modules ./fuku/i18n` ok (modules 259.1 s, i18n 5.1 s), run once after the last code commit with no failure; `-run '^(TestRules|TestLegacyGreetings)' ./fuku/modules` ok.
- `CGO_ENABLED=0 go build ./...` ok; `go vet -tags testtools ./fuku/modules` clean; `gofmt -l` on the touched files empty; `make generate-docs` then `make check-docs` no drift; `make check-translations` all present; `git diff --exit-code 72a692b -- go.mod go.sum` clean.
- `python3 .../lint-local.py 72a692b`: exit 0 before the last code commit and again before this SUMMARY (the plan names `bd2f794` and the phase-11 script path; the brief's base `72a692b` and script were used).
- Not run: `make lint` (the installed golangci-lint is built with Go 1.25), `make test`, any PostgreSQL-only test (this plan adds none).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] The RED version of the end-to-end `/rules` subtest could not run as written**
- **Found during:** Task 1 GREEN
- **Issue:** `openEditPagePrivate` asserts that the bot sent exactly one private message to the person, so a test cannot open the private menu twice for one owner.
- **Fix:** the subtest takes the buttons from a second owner of the same group; the test file change is in the GREEN commit `1882d14`.
- **Files modified:** `fuku/modules/settings_input_rules_test.go`

**2. [Documented] `TestSettingsEditDoorAuthority` was not extended.** The plan lists it as extended; its worlds fix the text part, and the bot-lost-administrator case for the Rules media part is covered by `TestSettingsEditRulesTypedRefusals`, which asserts the reason, the blank line, `settings_edit_wait_stopped`, the wait gone and nothing stored. The test still runs in the Task 3 and family runs.

**3. [Documented] AGENTS.md prompt-limit sentence changed in the Task 2 GREEN commit** (the change that invalidates it); the door paragraph and trap bullet followed in the Task 3 commit, one commit after the Task 1 change that made the door paragraph incomplete.

**4. [Documented] Task 3 has no separate failing-test commit** (see the TDD record).

## Known Stubs

None. Until 12-14 the Preview of Rules media and buttons still sends the old text preview; `settings_edit_preview.go` was not touched.

## Requirements

EDIT-08 and EDIT-07 stay Pending (12-14 completes EDIT-08); the EDIT-09 tick is untouched. `requirements-completed: []`.

## Threat Flags

None. No new endpoint, auth path, file access or schema change; no log line was added (T-12-44), and the button links go through `content.ValidButtonURL` as before (T-12-41, `TestSettingsEditRulesTypedRefusals`).

## Self-Check: PASSED

The created test file and the five commits above exist and are ancestors of HEAD.
