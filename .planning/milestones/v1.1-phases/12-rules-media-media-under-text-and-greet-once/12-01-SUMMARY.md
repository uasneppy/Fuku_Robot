---
phase: 12-rules-media-media-under-text-and-greet-once
plan: 01
subsystem: testing
tags: [characterization-test, golden-requests, rules, greetings, telegram-requests]
requires:
  - phase: 11-message-editor
    provides: the greetings and rules apply functions, the module test harness (moduleBotClient, withRealLocales)
provides:
  - TestRulesLegacyRowSendsTodaysRequest, the golden sendMessage requests of /rules (private rules off, as a reply, private rules on, no rules) and of the private-rules deep link (rules set, not set)
  - TestLegacyGreetingsRequestsUnchanged, the golden requests of a legacy text welcome, photo welcome, video goodbye, /welcome showing a photo welcome and a document GIF, none carrying show_caption_above_media
affects: [12-04, 12-07, 12-08, 12-09, rules, greetings, media-sender]

actuals:
  tokens: 3150
  tasks: 2
  commits: 2

commits: 2
plan_head_before: fc1ba4cc93fd84f3eb7ff0aea2667e93dab3a8b5
plan_head_after: 26c5b198f0a40aacb9c609e45b174b4418ad61c9

tech-stack:
  added: []
  patterns:
    - "Golden requests are json.Marshal of the Params the bot handed to the Telegram client (sorted map keys), compared byte for byte, recorded on untouched code and never re-recorded"

key-files:
  created:
    - fuku/modules/rules_legacy_request_test.go
  modified: []

key-decisions:
  - "Goldens record what the code does today, including the quirks the plan flagged: the deep link goes through formatting.Shtml() (link_preview_options is_disabled, reply_parameters allow_sending_without_reply true), and /rules sends an empty reply_markup object even with private rules off and no link_preview_options"
  - "The tests install the real locale files (withRealLocales), so the goldens include the translated lines; with no language row the bot answers in uk, the default, so the recorded texts are Ukrainian"

patterns-established:
  - "legacyExpectOne(t, client, method, golden): exactly one call of the method, request JSON equal to the golden"
  - "legacyNoCaptionAbove(t, client): no recorded call of any method has show_caption_above_media"

requirements-completed: []

coverage:
  - id: D1
    description: "The requests /rules and the private-rules deep link send for a rules row saved before Phase 12 are pinned byte for byte"
    requirement: "EDIT-07"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestRulesLegacyRowSendsTodaysRequest$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D2
    description: "The requests a legacy welcome, goodbye, /welcome display and document GIF send are pinned, and none carries show_caption_above_media"
    requirement: "EDIT-08"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestLegacyGreetingsRequestsUnchanged$' ./fuku/modules"
        status: pass
    human_judgment: false

duration: 15min
completed: 2026-11-05
status: complete
---

# Phase 12 Plan 01: Legacy request characterization Summary

**Eleven golden Telegram requests (six rules, five greetings) recorded on untouched rules.go, sender.go and greetings.go, so every later refactor of the send paths must leave old rows byte-identical.**

## Performance

- **Duration:** about 15 min
- **Tasks:** 2 (a tracer and an auto task)
- **Files created:** 1 (`fuku/modules/rules_legacy_request_test.go`)

## Accomplishments

- `TestRulesLegacyRowSendsTodaysRequest` drives the real `sendRules` and `rulesDeepLinkHandler` through `moduleBotClient` for six cases and compares the one `sendMessage` request of each with a golden string.
- `TestLegacyGreetingsRequestsUnchanged` drives `newMember`, `leftMember` and `welcome` for five cases (`join_text`, `join_photo`, `leave_video`, `welcome_display_photo`, `join_document_gif`) and compares the one `sendMessage`, `sendPhoto`, `sendVideo` or `sendDocument` request with a golden; every subtest also asserts that no recorded call of any method has `show_caption_above_media`, and the document GIF case asserts there is no `sendAnimation` call.
- Both tests passed on the first run against the untouched code once the goldens were pasted in; `rules.go`, `sender.go` and `greetings.go` are byte-identical to `bd2f794` and to `HEAD` at both commits.

## What the goldens record (real behaviour, not an expectation)

- `/rules`, private rules off: `parse_mode HTML`, `reply_markup {}` (an empty keyboard object), `reply_parameters {message_id 101, allow_sending_without_reply true}`, no `link_preview_options`. As a reply to message 55 only the reply target changes to 55.
- `/rules`, private rules on: the click-to-read line and one URL button `t.me/FukuTestBot?start=rules_<chat id>` with the custom label.
- Deep link: `chat_id` is the private chat, `link_preview_options {is_disabled true}`, `parse_mode HTML`, `reply_parameters {message_id 101, allow_sending_without_reply true}`, no `reply_markup`.
- Welcome and goodbye: the text welcome carries `link_preview_options {is_disabled true}` and `reply_markup {}`; the photo, video and document sends carry `caption`, `parse_mode HTML`, `reply_markup {}`, the file ID and (video and document) `thumbnail null`. No request has `show_caption_above_media` and no `message_thread_id` (thread 0).

## Task Commits

1. **Task 1 (tracer): rules requests pinned** - `5b8978c` (test)
2. **Task 2: welcome, goodbye and document GIF requests pinned** - `26c5b19` (test)

The plan's docs commit follows these two.

## Verification (real results)

- `go test -tags testtools -race -count=1 -run '^TestRulesLegacyRowSendsTodaysRequest$' -v ./fuku/modules`: `--- PASS: TestRulesLegacyRowSendsTodaysRequest` plus six subtest PASS lines (`ok`).
- `go test -tags testtools -race -count=1 -run '^(TestRulesLegacyRowSendsTodaysRequest|TestLegacyGreetingsRequestsUnchanged)$' -v ./fuku/modules`: both tests and all eleven subtests PASS (`ok`).
- Family run `go test -tags testtools -race -count=1 -run '^(TestSendRules|TestMemberJoin|TestRulesApply|TestBuildRulesText)' ./fuku/modules`: `ok`, five top-level tests passed.
- `git diff --quiet bd2f794 -- fuku/modules/rules.go fuku/utils/media/sender.go fuku/modules/greetings.go`: exit 0 at both commits. `git diff --exit-code bd2f794 -- go.mod go.sum`: exit 0.
- `gofmt -l` on the test file: clean. `go vet -tags testtools ./fuku/modules`: clean.
- `python3 .../lint-local.py 72a692b`: exit 0, "no dupl, gocyclo or godox findings on lines added since 72a692b", run before the last code commit.
- The tracer gate (re-run of Task 1's verify before Task 2) passed.
- Not run: `make lint` and `make test` (they cannot run in this sandbox and the plan does not ask for them; 12-16 is the phase gate).

## Deviations from Plan

None - plan executed as written. Two small notes:

- The plan's acceptance invariant diffs against `bd2f794`; the brief's baseline `72a692b` is the Phase 11 base and differs from the three files (Phase 11 work), so the invariant was checked against `bd2f794` and `HEAD` as the plan says.
- Task 1's first version held the chat-ID constants and helper that only Task 2 uses; they were added in Task 2 so that no commit carries an unused helper.

## Known Stubs

None.

## Threat Flags

None. The plan adds a test file only.

## Requirement coverage

`requirements-completed: []`. The plan contributes to EDIT-07 and EDIT-08 (success criterion 4: old rows look exactly as before) and completes neither; both stay Pending in `.planning/REQUIREMENTS.md`.

## Notes for later plans

- The goldens must stay green through 12-04, 12-07, 12-08 and 12-09 and must never be re-recorded to make a refactor pass. A deliberate change to a legacy request is a deviation to be argued, not a golden to be edited.
- The goldens contain the Ukrainian default-language texts (`rules_for_chat`, `rules_click_for_rules`, `rules_not_set`, `rules_no_rules_set` of the no-rules case). A later edit of those `uk.yml` strings would fail the test on purpose.
- PostgreSQL-only guarantees of this phase (`TestGreetedMemberClaimConcurrent`, `TestRulesMediaMigrationDefaults`) belong to later plans; this plan adds none.

## Self-Check: PASSED

- `fuku/modules/rules_legacy_request_test.go` exists.
- Commits `5b8978c` and `26c5b19` are ancestors of HEAD.
