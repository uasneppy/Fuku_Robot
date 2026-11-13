---
phase: 11-message-editor
plan: 13
subsystem: modules
tags: [settings-menu, message-editor, typed-reply-door, album-gate, media, buttons, tdd]
requires:
  - phase: 11-message-editor
    provides: plan 11-07 settingsInputAlbumFirst and the album key, plan 11-10 settingsEditCheck media and buttons branches, plan 11-11 the typed-reply door
provides:
  - the album gate of the typed-reply door (settingsInputReply.albumFirst, settingsInputIsAlbum)
  - the settings_edit_album_cut line on the saved page of a message that came from an album
  - media, album, button and whole-message parity tests through the door (settings_input_media_test.go)
affects: [11-14, settings-message-editor]

actuals:
  tokens: 6200
  tasks: 3
  commits: 2

commits: 2
plan_head_before: 52258b807b68c6a81535a1263a09b16b1d8b10bd
plan_head_after: af8cdfe9cc43361910da2c7719e56a7b6cb2c9ac

tech-stack:
  added: []
  patterns:
    - "Album items arrive as separate updates, so the door lets only the first one to take fuku:settings:inputalbum:<user>:<media_group_id> (SET NX EX 30) reach the live checks and the claim; the rest end silently, which keeps one write, one saved page and one refusal per album"
    - "A parity test builds a command chat and an editor chat with their own creators (settingsEditPair) and compares what the two leave stored"

key-files:
  created:
    - fuku/modules/settings_input_media_test.go
  modified:
    - fuku/modules/settings_input_door.go
    - AGENTS.md
    - .planning/REQUIREMENTS.md

key-decisions:
  - "The album gate sits in settingsInput right after the wait is known to be live and before run(), so it precedes lockdownLiveMember and the claim; the expired-wait path is untouched (its GETDEL marker already answers once)."
  - "A media group ID is an album only when it matches settingsInputAlbumIDRe; the door tests that before calling settingsInputAlbumFirst, because the store function answers false both for a lost gate and for a non-album ID."
  - "settings_edit_album_cut is added by the saved page for any saved message with a valid media group ID, whichever part it was (the plan's wording: the saved message carried a valid media group ID)."

requirements-completed: [EDIT-04, EDIT-05, EDIT-11]

coverage:
  - deliverable: "A photo, video, GIF, document, sticker, audio or voice message sent for the media part is stored with the file ID and type /setwelcome and /setgoodbye store for the same item, text and buttons kept, caption not used and said so"
    requirement: EDIT-04
    verification:
      - kind: test
        ref: "fuku/modules/settings_input_media_test.go#TestSettingsEditMediaParity"
        status: pass
      - kind: test
        ref: "fuku/modules/settings_input_media_test.go#TestSettingsEditCaptionIgnored"
        status: pass
  - deliverable: "A video note, contact, poll, location or plain text for the media part is refused with settings_edit_refuse_need_media and media over a 1100-character text with settings_edit_refuse_media_text_long, the wait stays"
    requirement: EDIT-04
    verification:
      - kind: test
        ref: "fuku/modules/settings_input_media_test.go#TestSettingsEditMediaKindsDoor"
        status: pass
      - kind: test
        ref: "fuku/modules/settings_input_media_test.go#TestSettingsEditMediaTextLongDoor"
        status: pass
  - deliverable: "From an album one item is kept and the saved page says so; an album is written once even when its items arrive in parallel; a media group ID that is not digits is one item"
    requirement: EDIT-04
    verification:
      - kind: test
        ref: "fuku/modules/settings_input_media_test.go#TestSettingsEditAlbumWritesOnce"
        status: pass
      - kind: test
        ref: "fuku/modules/settings_input_media_test.go#TestSettingsEditAlbumNotice"
        status: pass
  - deliverable: "A refused album gets one refusal and a late album one wait-ended reply, not one per item"
    requirement: EDIT-04
    verification:
      - kind: test
        ref: "fuku/modules/settings_input_media_test.go#TestSettingsEditRefusedAlbumOneRefusal"
        status: pass
      - kind: test
        ref: "fuku/modules/settings_input_media_test.go#TestSettingsEditExpiredAlbumOneReply"
        status: pass
  - deliverable: "Button lines are stored as /setwelcome and /setgoodbye store the same lines (:same, the Greeting default label), replacing the set; a bad link, unreadable lines, over 100 buttons or over 8 in a row are refused with their reason and the wait stays"
    requirement: EDIT-05
    verification:
      - kind: test
        ref: "fuku/modules/settings_input_media_test.go#TestSettingsEditButtonsParity"
        status: pass
      - kind: test
        ref: "fuku/modules/settings_input_media_test.go#TestSettingsEditButtonRefusalsDoor"
        status: pass
  - deliverable: "Text, media and buttons saved one after another leave the same greetings row as one command replying to a captioned photo; Rules text equals /setrules"
    requirement: EDIT-11
    verification:
      - kind: test
        ref: "fuku/modules/settings_input_media_test.go#TestSettingsEditWholeMessageParity"
        status: pass
  - deliverable: "A sticker saved through the editor redraws the page with the no-caption note"
    requirement: EDIT-04
    verification:
      - kind: test
        ref: "fuku/modules/settings_input_media_test.go#TestSettingsEditStickerNote"
        status: pass
  - deliverable: "A GIF resent as a document still renders as a GIF on a real client (research A3), and real album delivery order and replicas (A4)"
    requirement: EDIT-04
    verification:
      - kind: backstop
        ref: "owner's live check in 11-SPIKE.md (written by plan 11-14)"
        status: not-run
    human_judgment: true

duration: 15min
completed: 2026-11-04
status: complete
---

# Phase 11 Plan 13: The album gate, and media and buttons through the typed-reply door Summary

**The typed-reply door now gates albums: with a live wait only the first item of an album reaches the live checks and the claim, so an album is saved once, says "I kept only one item" on the saved page, gets one refusal when refused and one wait-ended reply when late; and tests prove that media, buttons and a whole message saved part by part are stored exactly as `/setwelcome`, `/setgoodbye` and `/setrules` store them.**

## What changed

- `fuku/modules/settings_input_door.go`: `settingsInputIsAlbum` (media group ID of 1 to 20 ASCII digits), `settingsInputReply.albumFirst` (calls `settingsInputAlbumFirst`; a lost gate or a Redis error ends the update with no reply, no Telegram call and no write; the Redis error logs the user ID only) called in `settingsInput` before `run()`, and the saved page adds `settings_edit_album_cut` for a message from an album. Nothing else in the door needed a fix: the media and buttons paths of plans 11-10 and 11-11 already matched the commands.
- `fuku/modules/settings_input_media_test.go` (new): helpers `settingsMediaItemID`, `settingsMediaItem`, `sendPrivateMedia`, `sendPrivateAlbum` (sequential or from goroutines), `settingsMediaAlbumOf`, `settingsEditPair` with `newSettingsEditPair` and `onEditor` (a command chat and an editor chat, each with a creator), `settingsEditCommandName`, `settingsEditWelcomeOf`, `settingsEditCommandReplyTo`, `settingsMediaCountingWrite`, `settingsEditAlbumKeys`, `settingsEditPageText`; and the twelve tests of the plan.
- `AGENTS.md`: the typed-reply door text describes the album gate (key, first wins, before the live checks and the claim, siblings silent, one refusal, one late reply, Redis error ends the update), `settings_edit_album_cut` on the saved page, what the media and buttons parts store, and that a bad link is refused instead of dropped (EDIT-05). The Data bullet of plan 11-07 already named `fuku:settings:inputalbum`; the grep now counts 2.
- `.planning/REQUIREMENTS.md`: EDIT-04, EDIT-05 and EDIT-11 ticked (checkbox and traceability row).

## Task commits

| Task | Commit | Subject |
|------|--------|---------|
| 1-3 RED | 8367de0 | test(11-13): add failing tests for the album gate and the media and buttons door |
| 2 GREEN | af8cdfe | feat(11-13): gate albums in the settings typed-reply door and say when one was cut |

## TDD notes (RED, GREEN, REFACTOR)

- RED (8367de0): the three album tests failed on the planned assertions with a clean build, the target test named in each `--- FAIL` line: `TestSettingsEditAlbumWritesOnce` ("the reply lacks settings_edit_album_cut", "the album memory ... is missing"), `TestSettingsEditAlbumNotice/an_album_keeps_one_item_and_says_so` ("lacks settings_edit_album_cut") and `TestSettingsEditRefusedAlbumOneRefusal` (both subtests: "the bot sent 3 messages, want exactly 1", three identical refusals). Semantic assessment: each failed because the door had no album gate, which is exactly the planned behaviour that is missing.
- The other nine tests of the file passed in RED: `TestSettingsEditMediaParity`, `TestSettingsEditCaptionIgnored`, `TestSettingsEditMediaKindsDoor`, `TestSettingsEditMediaTextLongDoor`, `TestSettingsEditStickerNote`, `TestSettingsEditExpiredAlbumOneReply`, `TestSettingsEditButtonsParity`, `TestSettingsEditButtonRefusalsDoor` and `TestSettingsEditWholeMessageParity`. They pin behaviour that plans 11-10 and 11-11 already built (the media and buttons checks, the apply functions, the marker taken with GETDEL), so they are parity and regression proof, not a failing step; the tracer task (Task 1) therefore needed no door fix. One test-side bug showed up before the RED commit: the caption sample "ignored" matched the substring of the locale marker `caption-ignored`, so the caption is now "SECRETCAPTION".
- GREEN (af8cdfe): the album gate; all twelve tests pass, and the three concurrent tests passed six runs in a row under `-race`.
- REFACTOR: none needed (no refactor commit).
- `gsd_run check tdd-red-evidence` was not run (the plan is `type: execute`, as for 11-09 and 11-12); the RED run was read by hand against the target test names.

## Verification (real results)

- RED run, the twelve target tests: `FAIL` with only the three album tests failing (above).
- GREEN run of the same twelve with `-race -count=1`: `ok fuku/modules 10.732s`.
- `-race -count=6` of `TestSettingsEditAlbumWritesOnce`, `TestSettingsEditRefusedAlbumOneRefusal`, `TestSettingsEditExpiredAlbumOneReply`: `ok 9.565s`.
- `go test -tags testtools -race -count=1 -run '^(TestSettings|TestLoadSettings)' ./fuku/modules ./fuku/i18n`: `ok fuku/modules 222.203s`, `ok fuku/i18n 6.214s`.
- `go test -tags testtools -race -count=1 -run '^(TestGreeting|TestWelcome|TestSetGreeting|TestMemberJoin|TestLeftMember|TestSetRules|TestSendRules)' ./fuku/modules`: `ok 1.395s`.
- `CGO_ENABLED=0 go build ./...`: ok. `go vet -tags testtools ./fuku/modules`: clean. `git diff --exit-code 72a692b -- go.mod go.sum`: exit 0. `gofmt -l` on the two Go files I touched prints nothing (it lists seven older files that this plan did not touch).
- `python3 .../lint-local.py 72a692b`: "no dupl, gocyclo or godox findings", exit 0, run after the GREEN code.
- Acceptance greps: `settingsInputAlbumFirst(` occurs once in the door (line 131) before `lockdownLiveMember(` (line 239); `settings_edit_album_cut` occurs once in the door; `fuku:settings:inputalbum` occurs twice in AGENTS.md.
- Not run: `make lint` (golangci-lint here is built with Go 1.25), `make test`, `make check-translations` and `make check-docs` (no locale or generated-docs source changed; `settings_edit_album_cut` already exists in all 8 locales from plan 11-04), PostgreSQL targets (nothing here touches the schema).

## Deviations from Plan

None in behaviour. Notes:

- The plan's test helper `sendPrivateAlbum(env, from, items, concurrent bool)` returns the media group ID, which a test uses to check the album key; `sendPrivateMedia` builds, sends and returns the message. The button-limit cases reuse the existing `settingsEditButtonLines` of `settings_edit_check_test.go`; the file's own sample lines are `settingsMediaButtonLines` (a name clash).
- No test outside this plan's file list changed.
- `TestSettingsEditDoorStoresEveryPart` (11-11) already stores a typed photo and a typed button line; this plan's tests do not repeat it but add parity with the commands, the kinds, albums and the whole message.

## Requirements

- EDIT-04 is COMPLETE and ticked: one item of photo, video, GIF (stored as a document), document, sticker, audio or voice is stored as the commands store it, the refused kinds say why, and an album keeps one item and tells the owner.
- EDIT-05 is COMPLETE and ticked: the bot's button syntax is parsed by the commands' own parser, and a bad link, unreadable lines or a count over Telegram's limits are refused with the reason while the wait stays.
- EDIT-11 is COMPLETE and ticked: text, media, buttons and Rules text saved through the editor equal the commands' stored rows (`TestSettingsEditTextParity`, `TestSettingsEditRulesParity`, `TestSettingsEditMediaParity`, `TestSettingsEditButtonsParity`, `TestSettingsEditWholeMessageParity`), and the check of plan 11-10 refuses what Telegram would refuse before the wait is claimed. Rules media and buttons belong to Phase 12 (EDIT-08) and are not part of this requirement's text.
- The owner's live checks (A3 a GIF as a document, A4 real album delivery) stay in 11-SPIKE.md (plan 11-14) as non-blocking backstops.

## Known Stubs

None.

## Threat Flags

None beyond the plan's register. T-11-51 (an album writing several times, two replicas) is covered by the album memory plus the nonce claim and `TestSettingsEditAlbumWritesOnce` under `-race`; T-11-52 by `TestSettingsEditRefusedAlbumOneRefusal`; T-11-53 by the digit test before any key is built and `TestSettingsEditAlbumNotice` ("abc" writes no album key); T-11-54 by `TestSettingsEditMediaKindsDoor`; T-11-55 by the three parity tests.

## Self-Check: PASSED

- Created file exists: `fuku/modules/settings_input_media_test.go`.
- Commits 8367de0 and af8cdfe are ancestors of HEAD; `git rev-list --count 52258b8..HEAD` printed 2 before this SUMMARY.
