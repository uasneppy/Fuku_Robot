---
phase: 12-rules-media-media-under-text-and-greet-once
plan: 04
subsystem: media
tags: [animation, sendAnimation, caption-above, show_caption_above_media, greetings, sender]
requires:
  - phase: 12-rules-media-media-under-text-and-greet-once
    provides: 12-01 legacy request goldens, 12-05 locale key settings_edit_media_animation
provides:
  - db.ANIMATION (9), stored for a GIF set by /setwelcome, /setgoodbye or the editor's media part
  - content.extractGreetingMedia, called by ExtractWelcome and ExtractMedia (notes, filters, pins keep documents)
  - media sender case db.ANIMATION (sendAnimation) and one shared sendCaptioned for photo, video and animation
  - media.Content.CaptionAbove, media.CaptionAboveEligible, media.GreetingMessage and media.SendGreetingMessage (SendGreeting is its flag-off wrapper)
  - settingsEditMediaKey names a stored animation (settings_edit_media_animation)
affects: [12-08, 12-11, 12-12, 12-14, 12-16]

actuals:
  tokens: 8300
  tasks: 3
  commits: 5

commits: 5
plan_head_before: 0742b7fba7b20a4f1405b7d5ace3426ade05bf36
plan_head_after: 2d0fd6960e22b0d2e92dec5cf0f10a6e651036e6

tech-stack:
  added: []
  patterns:
    - "One Animation-first greeting extraction beside the legacy one, so a type change for one module cannot reach the others"
    - "Flag-carrying content field read only by the senders whose Telegram method takes the parameter, ANDed with one predicate at the single entry point"

key-files:
  created:
    - fuku/modules/greetings_gif_test.go
  modified:
    - fuku/db/db.go
    - fuku/utils/content/content.go
    - fuku/utils/content/content_test.go
    - fuku/utils/content/editor_helpers_test.go
    - fuku/utils/media/sender.go
    - fuku/utils/media/sender_test.go
    - fuku/modules/settings_edit.go
    - fuku/modules/settings_input_media_test.go
    - fuku/modules/settings_edit_check_test.go
    - AGENTS.md

key-decisions:
  - "sendPhoto, sendVideo and the new sendAnimation became one function, sendCaptioned, with a switch on the media type: three separate functions were a dupl clone cluster, and a closure-based helper was still one. The Send switch keeps its own case db.ANIMATION"
  - "The plan's ANIMATION constant is added in the tracer's red commit as the compile stub, so the red tests fail on their assertions and not on a build error"

patterns-established:
  - "A GIF saved from Phase 12 on is db.ANIMATION; a document GIF stays a document and sends sendDocument (12-01 golden still green)"

requirements-completed: []

coverage:
  - id: D1
    description: "A GIF set by /setwelcome by reply or the editor is stored as db.ANIMATION with the animation's file ID, a join sends one sendAnimation and no sendDocument, and the editor page names it"
    requirement: "EDIT-07"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestSetWelcomeGifIsAnimation|TestSettingsEditMediaParity|TestSettingsEditMediaKinds)$' ./fuku/modules"
        status: pass
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestExtractMedia$' ./fuku/utils/content"
        status: pass
    human_judgment: false
  - id: D2
    description: "Notes, filters and pins keep a GIF as a document; extractMediaFromReply and pins.go are untouched; legacy requests are byte-identical"
    requirement: "EDIT-07"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestExtractNoteAndFilterGifStaysDocument|TestExtractMediaFromReply|TestExtractGreetingMediaAnimation)$' ./fuku/utils/content"
        status: pass
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestRulesLegacyRowSendsTodaysRequest|TestLegacyGreetingsRequestsUnchanged)$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D3
    description: "show_caption_above_media is sent only by sendPhoto, sendVideo and sendAnimation, only when the flag is true, through the one predicate CaptionAboveEligible"
    requirement: "EDIT-07"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestCaptionAboveEligible|TestSendGreetingMessageCaptionAbove|TestSendAnimationUsesSendAnimation)$' ./fuku/utils/media"
        status: pass
    human_judgment: false

duration: 45min
completed: 2026-11-05
status: complete
---

# Phase 12 Plan 04: Animation type and caption-above sender Summary

**A GIF set as a welcome or goodbye from now on is stored as `db.ANIMATION` and sent with `sendAnimation`, and the sender can put a caption above a photo, video or GIF through one eligibility predicate, while every request of a message saved before is unchanged.**

## What changed

- `db.ANIMATION int = 9`. `extractGreetingMedia` (animation first, else `extractMediaFromReply`) is called by `ExtractWelcome` and `ExtractMedia`; `extractMediaFromReply`, `ExtractNoteAndFilter` and `fuku/modules/pins.go` are untouched (`git diff --quiet bd2f794 -- fuku/modules/pins.go` holds, and `return replyMsg.Animation.FileId, db.DOCUMENT` still appears once).
- Sender: `case db.ANIMATION`, `Content.CaptionAbove` (read only through `ShowCaptionAboveMedia` in the photo, video and animation calls), `CaptionAboveEligible`, `GreetingMessage` and `SendGreetingMessage`; `SendGreeting` keeps its signature and its nine call sites and is a one-call wrapper with the flag off.
- `settingsEditMediaKey` has `case db.ANIMATION: return "settings_edit_media_animation"` (key from 12-05).
- Tests flipped on purpose (research S-06): `TestExtractMedia`, `TestSettingsEditMediaParity`, `TestSettingsEditMediaKinds`. New: `TestSetWelcomeGifIsAnimation`, `TestExtractGreetingMediaAnimation`, `TestExtractNoteAndFilterGifStaysDocument`, `TestSendAnimationUsesSendAnimation`, `TestCaptionAboveEligible`, `TestSendGreetingMessageCaptionAbove`; the animation row was added to the sender's two table tests and `TestContentAndOptionsZeroValues` covers the new field.
- AGENTS.md (this commit): the two "GIF as a document" sentences rewritten and a "Greeting media types (Phase 12)" bullet added under Subsystem traps, with the rolling-deploy caveat.

## TDD record

- **Task 1 (tracer) RED** `8d74a5a`: the three flipped tests and `TestSetWelcomeGifIsAnimation`, with `db.ANIMATION` as the compile stub. Real failures, all on assertions: `TestExtractMedia: got ("doc", 3), want ("an", 9)`, `TestSetWelcomeGifIsAnimation: the welcome is stored as ("gif-doc", type 3)`, `TestSettingsEditMediaKinds/a_GIF_is_stored_as_an_animation`, `TestSettingsEditMediaParity/{w,g}/animation`. Semantic assessment: the target tests executed and failed on the planned behavior (a GIF still stored as a document), not on setup. GREEN `222029d`.
- **Task 2** `6c5a536` (single test commit): the plan says these tests pass on Task 1's code, and they did on first run; no RED is claimed for it.
- **Task 3 RED** `292ef91`: `Content.CaptionAbove`, `CaptionAboveEligible`, `GreetingMessage` and `SendGreetingMessage` as compile-only stubs. `TestCaptionAboveEligible` failed for 4, 7 and 9 and `TestSendGreetingMessageCaptionAbove` for photo, video and animation (`show_caption_above_media with the flag on = <nil>, want true`). The "SendGreeting is the flag-off call" subtest passed against the stub by construction. GREEN `2d0fd69`.

## Commits

- 8d74a5a test(12-04): a GIF greeting must be stored as an animation
- 222029d feat(12-04): store a new GIF greeting as an animation
- 6c5a536 test(12-04): pin the GIF rules of notes, filters, pins and greetings
- 292ef91 test(12-04): add failing test for the caption above photo, video and GIF
- 2d0fd69 feat(12-04): let the sender put a caption above a photo, video or GIF

## Verification (real results)

- `go test -tags testtools -race -count=1 ./fuku/utils/content ./fuku/utils/media`: ok, ok.
- `go test -tags testtools -race -count=1 -run '^(TestRulesLegacyRowSendsTodaysRequest|TestLegacyGreetingsRequestsUnchanged|TestSetWelcomeGifIsAnimation|TestMemberJoin|TestSetWelcome)' ./fuku/modules`: ok (the 12-01 goldens are unchanged and green).
- Settings family `-run '^(TestSettings|TestLoadSettings)' ./fuku/modules ./fuku/i18n`: ok (194 s and 5 s).
- `CGO_ENABLED=0 go build ./...`: ok. `go vet -tags testtools` on `fuku/utils/media`, `fuku/utils/content`, `fuku/db`, `fuku/modules`: clean. `gofmt -l` on the touched files: empty.
- `python3 .../lint-local.py 72a692b` (dupl, gocyclo, godox, staticcheck unused, errcheck): exit 0, before the last code commit and again before this summary.
- `git diff --exit-code 72a692b -- go.mod go.sum`: clean.

## Deviations from Plan

**1. [Rule 3 - Blocking issue] dupl cluster in sender.go**
- **Found during:** Task 1 (lint stand-in).
- **Issue:** `sendAnimation` as a copy of `sendPhoto` made a six-function clone cluster with `sendDocument`, `sendPhoto`, `sendAudio`, `sendVoice` and `sendVideo`; a shared helper taking a closure left the three as clones of each other.
- **Fix:** `sendPhoto`, `sendVideo` and `sendAnimation` are one function, `sendCaptioned`, with a type switch; `Send` keeps its separate `case db.PHOTO`, `case db.VIDEO` and `case db.ANIMATION` that all call it. Requests, log text (`Empty FileID for PHOTO ...`) and error text are unchanged for photo and video (the existing sender tests and the 12-01 goldens pass).
- **Files modified:** `fuku/utils/media/sender.go`.
- **Commit:** 222029d.

No other deviations. The plan's `python3 .planning/phases/11-message-editor/handoff/lint-local.py bd2f794` was replaced by the brief's stand-in with base `72a692b`.

## Known Stubs

None.

## Not run / notes

- No PostgreSQL work in this plan. `make test` and `make lint` were not run (sandbox limits, and the plan does not ask for `make test`).
- EDIT-07 is not completed here (no switch and no send site passes the flag yet; 12-08, 12-11, 12-14). `requirements-completed: []`, EDIT-07 stays Pending.
- Rolling-deploy caveat (accepted): a replica on older code sends a type-9 welcome as text only until it is replaced; recorded in AGENTS.md. Real-client rendering of `sendAnimation` with a captured file ID is research assumption A7, a live check for 12-16's `12-SPIKE.md`.

## Self-Check: PASSED

Created and modified files exist; commits 8d74a5a, 222029d, 6c5a536, 292ef91 and 2d0fd69 are ancestors of HEAD.
