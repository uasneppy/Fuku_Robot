---
phase: 12-rules-media-media-under-text-and-greet-once
plan: 07
subsystem: rules
tags: [rules, media, buttons, sendPhoto, caption-above, deep-link, fallback]
requires:
  - phase: 12-rules-media-media-under-text-and-greet-once
    provides: 12-01 legacy request goldens, 12-02 RulesSettings.HasContent, 12-03 row loaders and writers, 12-04 media.Send CaptionAbove and CaptionAboveEligible
provides:
  - rulesForm (rulesFormGroup, rulesFormPrivate), rulesMessage, buildRulesMessage, sendRulesMessage, sendRulesText in fuku/modules/rules.go
  - /rules (group and connected private chat) and the rules deep link sent through the one builder and the one sender
  - the {rules} button gated on RulesSettings.HasContent
  - the member-path text fallback on a definite 4xx media refusal (deliberate deviation from D-03), logged at Warn
  - AGENTS.md "Rules message" bullet
affects: [12-12, 12-13, 12-14, 12-16]

actuals:
  tokens: 8800
  tasks: 3
  commits: 6

commits: 6
plan_head_before: ac28523472b86e71fadb9410be1c8e8da2da667d
plan_head_after: 00375037b98a23f4db5b6358d2e1dbbd47263f0e

tech-stack:
  added: []
  patterns:
    - "A text-only branch that is the caller's own old SendMessage call (options passed in) so a legacy row's request stays byte-identical, next to a media branch that is one media.Send"
    - "Heading form as a small enum with methods, so the group and private wordings share one builder"

key-files:
  created:
    - fuku/modules/rules_media_send_test.go
  modified:
    - fuku/modules/rules.go
    - fuku/utils/formatting/formatting.go
    - fuku/utils/formatting/formatting_test.go
    - AGENTS.md

key-decisions:
  - "Rules count as set by RulesSettings.HasContent everywhere a member sees them (UI-SPEC Assumption A1, a media-only row has the bare heading as its caption)"
  - "The member-path text fallback on a definite 4xx is a deliberate deviation from D-03, written into AGENTS.md; Full preview (12-14) passes degrade false"
  - "The fallback log line names the chat the message was sent to (the group for /rules, the member's private chat for the deep link), because sendRulesMessage's planned signature carries only that chat ID"

patterns-established:
  - "sendRulesMessage(b, chatID, m, textOpts, degrade): the caller owns the text options, the sender only adds the rules keyboard"

requirements-completed: []

coverage:
  - id: D1
    description: "/rules in the group sends one photo, video or document with the heading and rules text as its caption, the rules' URL buttons under it, parse mode HTML and the reply target of the /rules message"
    requirement: "EDIT-08"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestRulesMediaAndButtonsInGroup$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D2
    description: "A rules row saved before the phase (no media, no buttons) makes the same request as before in the group, as private rules and via the deep link; the 12-01 goldens are unedited"
    requirement: "EDIT-08"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestRulesLegacyRowSendsTodaysRequest|TestLegacyGreetingsRequestsUnchanged)$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D3
    description: "The deep link and /rules in a connected private chat send the media and buttons (private form and group form); media-only, buttons-only and empty rows answer as designed; the click-to-read message carries no media"
    requirement: "EDIT-08"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestPrivateRulesMediaAndButtons|TestRulesMediaOnlyAndButtonsOnly|TestRulesClickToReadHasNoMedia)$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D4
    description: "show_caption_above_media is sent only when Media under text is on and the media is a photo, video or animation, in /rules and the deep link"
    requirement: "EDIT-07"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestRulesMediaBelowEverywhere$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D5
    description: "A media send refused with a 4xx other than 429 is retried once as text with the buttons in /rules and the deep link and logged at Warn by chat ID with no content; a 429, a 5xx and a transport error are not retried"
    requirement: "EDIT-08"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestRulesDegradesToTextOnClientError$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D6
    description: "A welcome's {rules} button appears for a rules row that has only a file ID"
    requirement: "EDIT-08"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestFormattingRulesButtonMediaOnly|TestFormattingReplacerAddsRulesButtons)$' ./fuku/utils/formatting"
        status: pass
    human_judgment: false

duration: 50min
completed: 2026-11-05
status: complete
---

# Phase 12 Plan 07: Rules message builder and sender Summary

**/rules (in the group and in a connected private chat) and the private-rules deep link now go through one builder and one sender that shows the rules' media, buttons and Media under text as one message, while every rules row saved before sends exactly today's request and a media Telegram refuses still reaches members as text.**

## What changed

- `rules.go`: `rulesForm` (`rulesFormGroup`: `rules_for_chat_header` / `rules_no_rules_set`; `rulesFormPrivate`: `rules_for_chat` / `rules_not_set`), `rulesMessage`, `buildRulesMessage`, `sendRulesMessage` and its text branch `sendRulesText`. `Empty` is `!row.HasContent()`; a row with media or buttons but no text gets the heading alone (the private form with its trailing newlines trimmed); the keyboard exists only when the row has buttons.
- `sendRulesMessage`: without media it is the caller's own `SendMessage` call, its options copied from `textOpts` with `ReplyMarkup` replaced only when the rules have buttons (never `media.SendGreeting`, which turns link previews off and drops the reply target). With media it is one `media.Send` carrying the text as caption, the keyboard, the reply target from `textOpts` and `CaptionAbove: m.Below && media.CaptionAboveEligible(m.MsgType)`.
- `sendRules`: the click-to-read branch is unchanged (no media, no buttons, A8); every other path builds the group form and sends it to `msg.Chat.Id` with today's options (`ReplyMarkup` the zero markup, the reply parameters, HTML). `rulesDeepLinkHandler` keeps the parse, `GetChat` and `IsUserInChat` checks and builds the private form, sent with `formatting.Shtml()` whose reply target is the `/start` message.
- `formatting.go`: the `{rules}` gate is `!rulesDb.HasContent()`.
- Fallback (deliberate deviation from D-03): with `degrade` true, a media send whose error satisfies `isTelegramClientError` (a 4xx other than 429) is logged at Warn (`[Rules] the rules media of chat <id> was refused, sending the text alone`) and sent once as text with the buttons. `/rules` and the deep link pass true; Full preview (12-14) will pass false.
- AGENTS.md: a new "Rules message" bullet (builder, forms, `HasContent`, legacy text branch, media branch, click-to-read, the deviation from D-03 with the Warn log and "Full preview never falls back"), and the "One setting, one code path" sentence now names `buildRulesMessage`.
- Tests: `TestRulesMediaAndButtonsInGroup` (photo, video, document), `TestPrivateRulesMediaAndButtons` (deep link and connected chat), `TestRulesMediaOnlyAndButtonsOnly`, `TestRulesClickToReadHasNoMedia`, `TestRulesMediaBelowEverywhere` (six kinds, flag on and off, group and deep link), `TestRulesDegradesToTextOnClientError` (400 twice, 502, transport error and 429; group and deep link; asserts the Warn line names the chat and carries no text, file ID or link), `TestFormattingRulesButtonMediaOnly`.

## TDD record

- **Task 1 (tracer) RED** `b76bea6`: `TestRulesMediaAndButtonsInGroup` failed on `sendMessage calls = 1, want 0` for photo, video and document. No stub needed (the test only calls `sendRules`). GREEN `4e6b2d7`; the 12-01 goldens and the two existing rules tests stayed green.
- **Task 2 RED** `25d91f0`: failures on assertions only (deep link sends text instead of the photo, bare heading, the buttons-only deep link has no keyboard, the matrix of kinds, `{rules}` button for a media-only row); the connected-chat and click-to-read cases already passed (the tracer had covered /rules, and the click-to-read branch was unchanged), kept as regression guards. GREEN `2ab7f4e`.
- **Task 3 RED** `667ee97`: the 400 cases failed with the handler returning the send error instead of falling back; the 502, transport and 429 cases passed (guards against over-retrying). GREEN `0037503`.

## Commits

- b76bea6 test(12-07): add failing test for /rules sending rules media and buttons as one message
- 4e6b2d7 feat(12-07): /rules sends the rules media and buttons as one message
- 25d91f0 test(12-07): add failing tests for private rules, media-only rules and the {rules} button
- 2ab7f4e feat(12-07): private rules and media-only rules follow the one builder
- 667ee97 test(12-07): add failing test for the rules text fallback on a refused media send
- 0037503 fix(12-07): a refused rules media still reaches members as text

## Verification (real results)

- Task 1: `-run '^(TestRulesMediaAndButtonsInGroup|TestRulesLegacyRowSendsTodaysRequest|TestLegacyGreetingsRequestsUnchanged|TestBuildRulesTextMatchesSendRules|TestSendRulesUsesPrivateButtonWhenEnabled)$' ./fuku/modules`: ok.
- Task 2: the five-test modules run (`TestPrivateRulesMediaAndButtons|TestRulesMediaOnlyAndButtonsOnly|TestRulesClickToReadHasNoMedia|TestRulesMediaBelowEverywhere|TestRulesLegacyRowSendsTodaysRequest`): ok; `-run '^(TestFormattingRulesButtonMediaOnly|TestFormattingReplacerAddsRulesButtons|TestFormattingReplacerWithoutRulesDoesNotRequireDatabase)$' ./fuku/utils/formatting`: ok.
- Task 3: `-run '^(TestRulesDegradesToTextOnClientError|TestRulesMediaAndButtonsInGroup|TestRulesLegacyRowSendsTodaysRequest)$' ./fuku/modules`: ok. `grep -c buildRulesMessage AGENTS.md` prints 2 (baseline 0); both required AGENTS.md phrases present.
- Plan-level family run `-run '^(TestRules|TestSendRules|TestSetRules|TestClearRules|TestPrivateRules|TestBuildRules|TestLegacyGreetings|TestSettingsEditRulesPreview|TestFormatting)' ./fuku/modules ./fuku/utils/formatting`: ok (8.1 s and 1.0 s).
- `CGO_ENABLED=0 go build ./...`: ok. `go vet -tags testtools ./fuku/modules ./fuku/utils/formatting`: clean. `gofmt -l` on the touched files: empty. `git diff --exit-code 72a692b -- go.mod go.sum`: clean.
- `python3 .../lint-local.py 72a692b`: exit 0 before the last code commit and again before this summary.
- Acceptance greps: one `func buildRulesMessage(` and one `func sendRulesMessage(`; no `SendGreeting` on a code line of `rules.go`; the 12-01 goldens file unchanged and its last commit a 12-01 commit; `HasContent()` once in `formatting.go` and no `rulesDb.Rules == ""`; `sendRulesMessage(` 3 hits (definition and two callers); `isTelegramClientError(` in `rules.go`.

## Deviations from Plan

### Deliberate (planned)

**1. [D-03] Member-path text fallback.** The plan itself deviates from D-03 (one message with the media) on purpose, as its section "Deliberate deviation from D-03" says; implemented exactly (`degrade` true only for `/rules` and the deep link, one retry, Warn log with chat ID only) and documented in AGENTS.md.

### Auto-fixed Issues

None.

### Notes

- The fallback log line carries the chat the message was sent to, as `sendRulesMessage`'s planned signature has only that chat ID: for the deep link that is the member's private chat, not the rules group. The test asserts that behaviour. If operators need the group ID for deep-link fallbacks, 12-14 or a later plan can add it to `rulesMessage`.
- `{rules}` button gate: `HasContent` trims whitespace, so a rules row whose text is whitespace only and has no media or buttons no longer gets the button (it used to, by `Rules == ""`). That row shows the no-rules line in `/rules` anyway.

## Known Stubs

None.

## Not run / notes

- `make test` and `make lint` were not run (sandbox limits; the plan does not ask for `make test`). The settings test family was not run (the plan does not ask for it; no settings file was touched). No PostgreSQL-only guarantee is part of this plan.
- Requirements: this plan completes none (`requirements-completed: []`). EDIT-07 and EDIT-08 stay Pending in `.planning/REQUIREMENTS.md`; the owner can set rules media and buttons only from 12-12 and 12-13, and Full preview follows in 12-14.

## Self-Check: PASSED

`rules_media_send_test.go` exists and `rules.go`, `formatting.go`, `formatting_test.go` and `AGENTS.md` are changed; commits b76bea6, 4e6b2d7, 25d91f0, 2ab7f4e, 667ee97 and 0037503 are ancestors of HEAD.
