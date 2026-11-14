---
phase: 12-rules-media-media-under-text-and-greet-once
plan: 08
subsystem: greetings
tags: [greetings, welcome, goodbye, media, caption-above, apply-function]
requires:
  - phase: 12-rules-media-media-under-text-and-greet-once
    provides: 12-01 legacy request goldens, 12-03 SetWelcomeMediaBelow and SetGoodbyeMediaBelow, 12-04 media.SendGreetingMessage, media.GreetingMessage and CaptionAboveEligible
provides:
  - greetingWrites.WelcomeMediaBelow and GoodbyeMediaBelow seam fields
  - applyWelcomeMediaBelow and applyGoodbyeMediaBelow in fuku/modules/greetings.go
  - the join, the leave and both /welcome and /goodbye display sends (formatted and noformat) through media.SendGreetingMessage with the stored Media under text flag
affects: [12-09, 12-11, 12-14, 12-16]

actuals:
  tokens: 9500
  tasks: 2
  commits: 4

commits: 4
plan_head_before: 81f5536f0f21eb56534186f0bc2afc593b03fd11
plan_head_after: a75627c

tech-stack:
  added: []
  patterns:
    - "One setting, one code path: a column-targeted repository writer behind the greetingWrites seam, finished by finishGreetingApply (read-back, typed result)"
    - "The sender owns the eligibility rule: callers pass the stored flag and media.SendGreetingMessage applies CaptionAboveEligible"

key-files:
  created:
    - fuku/modules/greetings_media_below_test.go
  modified:
    - fuku/modules/greetings.go

key-decisions:
  - "The two apply functions have no production caller until 12-11 (the Welcome and Goodbye Media under text switch); TestGreetingsMediaBelowApply calls both so the unused check stays quiet"
  - "/welcome noformat and /goodbye noformat also pass the stored flag, so the owner sees the real placement (research Open Question 14)"
  - "applyGreetingReset and the Remove apply functions are untouched and never write the switch (UI-SPEC A3)"

patterns-established:
  - "Greeting sends pass media.GreetingMessage{CaptionAbove: stored flag}; no caller re-implements the eligibility check"

requirements-completed: []

coverage:
  - id: D1
    description: "A join with Media under text on sends the welcome photo with show_caption_above_media true, the caption and the stored keyboard; after the switch is turned off the key is absent"
    requirement: "EDIT-07"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestGreetingsMediaBelowEverywhere$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D2
    description: "The leave and the /welcome, /welcome noformat, /goodbye and /goodbye noformat displays follow the same flag; with a document, sticker, audio, voice or video note stored a stored on adds no show_caption_above_media"
    requirement: "EDIT-07"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestGreetingsMediaBelowEverywhere$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D3
    description: "applyWelcomeMediaBelow and applyGoodbyeMediaBelow store one column each, return the row read back, leave every other column unchanged, and a failed write is greetingSaveFailed with the error and an unchanged row"
    requirement: "EDIT-07"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestGreetingsMediaBelowApply$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D4
    description: "Welcomes and goodbyes saved before this phase send byte-identical requests (the 12-01 goldens, unedited)"
    requirement: "EDIT-07"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestLegacyGreetingsRequestsUnchanged|TestRulesLegacyRowSendsTodaysRequest)$' ./fuku/modules"
        status: pass
    human_judgment: false

duration: 15min
completed: 2026-11-05
status: complete
---

# Phase 12 Plan 08: Media under text in welcomes and goodbyes Summary

**A welcome or goodbye whose Media under text is on now shows its photo, video or GIF with the caption above, at the join, the leave and the /welcome and /goodbye displays, through the one send function and its eligibility rule, while every greeting saved before sends exactly today's request.**

## What changed

- `greetings.go`: seam fields `WelcomeMediaBelow` and `GoodbyeMediaBelow` (= `greetings.SetWelcomeMediaBelow` / `SetGoodbyeMediaBelow`); `applyWelcomeMediaBelow` and `applyGoodbyeMediaBelow` return `finishGreetingApply(...)`, the typed result with the row read back. They are the one write path of the menu's switch (no command exists).
- `SendWelcomeMessage`, `leftMember` and both sends of `displayGreeting` (noformat with the empty keyboard, formatted with `kb`) now call `media.SendGreetingMessage` with `CaptionAbove` from the stored column; `media.SendGreeting` has no caller left in `greetings.go`. `displayGreeting` gained one assignment per arm and no branch. Nothing in `SendWelcomeMessage` besides the send call changed (the clean-old block and ordering are as before; the greet-once claim is 12-09's).
- Tests in `greetings_media_below_test.go`: `TestGreetingsMediaBelowEverywhere` (join photo on and back off, leave video, the four display commands on and off, five ineligible kinds with the switch on) and `TestGreetingsMediaBelowApply` (welcome column, goodbye column, failed write via the seam).

## TDD record

- **Task 1 (tracer) RED** `3027210`: `join_photo_switch_on` failed on `show_caption_above_media = <nil>, want true`; `applyWelcomeMediaBelow` was a compile-only stub returning the zero result, so the failure is the planned assertion. GREEN `a51e44d` (real apply functions for both switches as the plan's Task 1 action says, seam fields, `SendWelcomeMessage`). The first GREEN run failed once on a bug in my own test (the `photo` request parameter is an `InputFileByID`, not a string); fixed by comparing the request JSON, in the GREEN commit.
- **Task 2 RED** `c32412a`: `leave_video_switch_on` and the four `displays_follow_the_flag/*_on` subtests failed on `show_caption_above_media = <nil>, want true`. The off cases, the ineligible-kind cases and `TestGreetingsMediaBelowApply` passed at once: the off and ineligible cases are regression guards, and the apply functions already existed from Task 1 (the plan builds both in Task 1), so `TestGreetingsMediaBelowApply` is a characterization of finished code, not a RED test. GREEN `a75627c`.

## Commits

- 3027210 test(12-08): add failing test for a welcome photo shown with its caption above
- a51e44d feat(12-08): a welcome can show its media under its text
- c32412a test(12-08): add failing tests for goodbye and display sends honouring Media under text
- a75627c feat(12-08): goodbyes and the greeting displays honour Media under text

## Verification (real results)

- `-run '^(TestGreetingsMediaBelowEverywhere|TestLegacyGreetingsRequestsUnchanged|TestMemberJoinAndLeaveSendConfiguredGreetings)$' ./fuku/modules`: ok (Task 1).
- `-run '^(TestGreetingsMediaBelowEverywhere|TestGreetingsMediaBelowApply|TestLegacyGreetingsRequestsUnchanged)$' -v ./fuku/modules`: every subtest PASS; second Task 2 command (`TestMemberJoinAndLeaveSendConfiguredGreetings|TestLeftMemberDeletesPendingCaptchaAttemptAndCleanGoodbye|TestWelcomeDisplaySendsStatusAndGreeting|TestWelcomeAndGoodbyeToggleInvalidAndDisplayBranches|TestGreetingDisplayPropagatesMediaSendErrors|TestGreetingCommandsPropagateGotgbotRequestErrors`): ok.
- Plan-level family `-run '^(TestGreeting|TestWelcome|TestSetGreeting|TestSetWelcome|TestMemberJoin|TestLeftMember|TestLegacyGreetings|TestRulesLegacy|TestSettingsEdit)' ./fuku/modules`: ok (89.5 s).
- `CGO_ENABLED=0 go build ./...`: ok. `go vet -tags testtools ./fuku/modules`: clean. `gofmt -l` on `greetings.go` and the test file: empty. `git diff --exit-code 72a692b -- go.mod go.sum`: clean.
- `python3 .../lint-local.py 72a692b`: exit 0 (no dupl, gocyclo, godox, staticcheck or errcheck findings) before the last code commit and again before this summary.
- Acceptance: one `func applyWelcomeMediaBelow(`; one `CaptionAbove: greetPrefs.WelcomeSettings.MediaBelow`; `media.SendGreetingMessage(` 4 hits and `media.SendGreeting(` 0 in `greetings.go`; `rules_legacy_request_test.go` unchanged and its last commit a 12-01 commit.

## Deviations from Plan

None - plan executed as written. The AGENTS.md was not edited (the plan names no change; 12-10 and 12-16 document).

## Known Stubs

None. `applyWelcomeMediaBelow` and `applyGoodbyeMediaBelow` have no production caller yet: plan 12-11 (the Welcome and Goodbye Media under text switch) is their caller; until then only `TestGreetingsMediaBelowApply` and `TestGreetingsMediaBelowEverywhere` use them.

## Not run / notes

- `make test` and `make lint` were not run (sandbox limits and not asked by the plan). The `TestSettings|TestLoadSettings` family was not run (plan does not ask). No PostgreSQL-only guarantee belongs to this plan.
- Requirements: this plan completes none (`requirements-completed: []`). EDIT-07 stays Pending: no switch exists until 12-11 and Full preview follows in 12-14.

## Self-Check: PASSED

`greetings_media_below_test.go` exists and `greetings.go` is changed; commits 3027210, a51e44d, c32412a and a75627c are ancestors of HEAD.
