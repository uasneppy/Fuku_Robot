---
phase: 11-message-editor
plan: 07
subsystem: modules
tags: [settings-menu, message-editor, redis, lua, cancel-command, tdd]
requires:
  - phase: 11-message-editor
    provides: plan 11-01 (the settings_edit_cancelled and settings_edit_cancel_nothing locale keys)
provides:
  - the Redis wait store of the message editor (set, read, claim, finish, release, cancel, cancel-any, album memory, marker)
  - /cancel in the bot's private chat (private only, silent in groups, not Disableable)
  - LoadSettingsInput, called from LoadSettings
affects: [11-09, 11-11, 11-13, settings-message-editor]

actuals:
  tokens: 11400
  tasks: 3
  commits: 4

commits: 4
plan_head_before: 325e8dbf14c6be2b47fc85272bdba6b6870addf2
plan_head_after: c7227a181acc504a5b2568adb46bdcac45eaf6b2

tech-stack:
  added: []
  patterns:
    - "Claim, work, finish or release by nonce in Lua (the captchaweb pattern), with finish and release on context.WithoutCancel and their own bound"
    - "Every deliberate end deletes the wait and its marker in one script, so the marker means only 'the wait expired'"
    - "A private-only CheckFunc that never replies makes a WrapCommand command silent in groups"

key-files:
  created:
    - fuku/modules/settings_input.go
    - fuku/modules/settings_input_test.go
  modified:
    - fuku/modules/settings.go
    - AGENTS.md

key-decisions:
  - "settingsInputCancel extends research's 0/1/2/3 cancel script with outcome 4 (no wait, marker present: the wait expired, marker deleted), as the plan assumed."
  - "settingsInputSet refuses a wait no reader would accept (non-negative group, unknown page or part, bad nonce) before any write, and a zero ExpiresAt means ten minutes from now."
  - "/cancel while a claim is held answers settings_edit_cancel_nothing and leaves the wait to the write in flight."
  - "The page and part allow-lists (w/g/r, x/m/b) are literals in settings_input.go; plan 11-11 switches the two validators to the editor's page table."

requirements-completed: []

duration: 35min
completed: 2026-11-04
status: complete

coverage:
  - id: D1
    description: "A wait is one Redis hash (fields c p t m n x) with a 10 minute TTL plus a 15 minute marker c|p|t, written by one script that returns the replaced wait; /cancel in private deletes both and answers cancelled, then nothing to cancel; /cancel in a group is silent and writes nothing"
    requirement: EDIT-02
    verification:
      - kind: unit
        ref: "fuku/modules/settings_input_test.go#TestSettingsInputStore"
        status: pass
      - kind: unit
        ref: "fuku/modules/settings_input_test.go#TestSettingsEditCancelCommand"
        status: pass
      - kind: unit
        ref: "fuku/modules/settings_input_test.go#TestSettingsEditCancelCommandIsSilentInAGroup"
        status: pass
      - kind: unit
        ref: "fuku/modules/settings_input_test.go#TestSettingsEditCancelCommandTouchesOnlyTheSender"
        status: pass
  - id: D2
    description: "Claim succeeds only for the holder of the current nonce and only once; finish and release act for the claim holder only; the Cancel script gives Deleted, Other, Busy, None and Expired and never touches another part's live wait"
    requirement: EDIT-02
    verification:
      - kind: unit
        ref: "fuku/modules/settings_input_test.go#TestSettingsInputCancelOutcomes"
        status: pass
  - id: D3
    description: "The read is one pipeline and a bad hash is no wait; the marker is taken once; the album memory is first-caller-only and refuses a bad media group ID without a write"
    requirement: EDIT-02
    verification:
      - kind: unit
        ref: "fuku/modules/settings_input_test.go#TestSettingsInputRead"
        status: pass
      - kind: unit
        ref: "fuku/modules/settings_input_test.go#TestSettingsInputAlbumAndMarker"
        status: pass
  - id: D4
    description: "Without Redis every store function returns errSettingsInputUnavailable and /cancel answers settings_edit_cancel_nothing"
    requirement: EDIT-02
    verification:
      - kind: unit
        ref: "fuku/modules/settings_input_test.go#TestSettingsInputWithoutRedis"
        status: pass
---

# Phase 11 Plan 07: Message editor wait store and /cancel Summary

**The bot can now remember, on any replica, that it is waiting for one person's next private message for one part of one group's message: one Redis hash per user with its own 10 minute expiry, six atomic Lua scripts (set, claim, finish, release, cancel, cancel-any), an album memory and an "expired" marker, plus a private-only `/cancel` that ends the wait.**

## What changed

- `fuku/modules/settings_input.go` (new): the key builders for `fuku:settings:input:<user>`, `fuku:settings:inputdone:<user>` and `fuku:settings:inputalbum:<user>:<media_group_id>`, `settingsInputWait`, the six scripts, `settingsInputSet`, `settingsInputRead` (one `HGETALL` plus `GET` pipeline), `settingsInputTakeMarker` (`GETDEL`), `settingsInputClaim`, `settingsInputFinish` / `settingsInputRelease` (run on `context.WithoutCancel` with their own bound), `settingsInputCancel` and `settingsInputCancelAny` (one shared runner), `settingsInputAlbumFirst`, `settingsInputParseMarker`, the page and part validators, `errSettingsInputUnavailable`, the `/cancel` command and `LoadSettingsInput`.
- `fuku/modules/settings.go`: `LoadSettings` ends with `LoadSettingsInput(dispatcher)`, so `settingsEnv.otherReplica()` gets `/cancel` too.
- `fuku/modules/settings_input_test.go` (new): the tests above, on miniredis and the real dispatcher.
- `AGENTS.md`: a Data bullet for the three key families, their TTLs, the scripts and the no-Redis answer, and a Handlers sentence for `LoadSettingsInput` and `/cancel`.
- `docs/src/content/docs/commands/settings/index.md` did not change: `make generate-docs` produced no difference and `make check-docs` reports no drift (the generator reads help text from `locales/en.yml`, and `/cancel` has no help entry in this plan).

## Task commits

| Task | Commit | Subject |
|------|--------|---------|
| 1 (tracer) | 4c4940a | feat(11-07): keep the editor's wait in Redis and end it with /cancel |
| 2 RED | 30de1a7 | test(11-07): add failing tests for the wait's claim, finish, release, cancel, album and marker |
| 2 GREEN | 42cf93c | feat(11-07): claim, finish, release and cancel the wait atomically, with the album memory and the marker |
| 3 | c7227a1 | docs(11-07): describe the editor's wait keys and /cancel in AGENTS.md |

## TDD notes (Task 2)

RED: with inert placeholder functions committed beside the tests so the package compiles, `go test -tags testtools -race -count=1 -run '^(TestSettingsInputCancelOutcomes|TestSettingsInputRead|TestSettingsInputAlbumAndMarker|TestSettingsInputWithoutRedis)$' ./fuku/modules` exited 1 with 4 failing top-level tests and 13 failing subtests, all on assertions. `gsd_run check tdd-red-evidence` returned `RED_EVIDENCE_OK` (`target_test_failed`, target `TestSettingsInputCancelOutcomes`). Semantic assessment: the targets executed and failed on the planned assertions (a placeholder that never claims, deletes or reads), not on build, setup or fixture faults. One subtest (claim after the wait expired) passes against the placeholder by construction, since it expects `false`; the other claim, finish, release, cancel, read, album, marker and no-Redis subtests fail in RED and pass in GREEN. GREEN: the implementation made all five plan tests plus the extra ones pass. No REFACTOR commit. Task 1 is a tracer, committed test and implementation together, as the plan says.

## Verification (real results)

- `go test -tags testtools -race -count=1 -run '^(TestSettingsInput.*|TestSettingsEditCancel.*)$' ./fuku/modules`: ok.
- Whole settings set `go test -tags testtools -race -count=1 -run '^(TestSettings|TestLoadSettings)' ./fuku/modules ./fuku/i18n`: `ok fuku/modules 127.786s`, `ok fuku/i18n 5.982s` (every earlier test green).
- `go test ... -run 'Registry|Help|Disable|WrapCommand|Descriptor' ./fuku/modules ./fuku/utils/helpers`: ok (the new command breaks no registry, help or disable test).
- `make generate-docs` then `make check-docs`: "No drift detected". `make check-translations`: all translations present.
- `CGO_ENABLED=0 go build ./...`: ok. `go vet -tags testtools ./fuku/modules`: clean. `git diff --exit-code 72a692b -- go.mod go.sum`: exit 0.
- `python3 .../lint-local.py 72a692b`: "no dupl, gocyclo or godox findings" (run before the GREEN commit and again at the end). `gofmt -l` on the touched Go files prints nothing.
- Acceptance greps: `LoadSettingsInput(dispatcher)` 1 in settings.go; `Name: *"cancel"` 1 and `Disableable` 0 in settings_input.go; `sync.Map|time.AfterFunc|map[int64]` 0; `redis.NewScript` 6; `context.WithoutCancel` 1; AGENTS.md has `fuku:settings:inputdone` 1, `LoadSettingsInput` 1, `fuku:settings:inputalbum` 1.
- Not run: `make lint` (installed golangci-lint is built with Go 1.25 and refuses Go 1.26.0, the local stand-in ran instead), `make test`, PostgreSQL targets (nothing here needs them).

## Deviations from Plan

None - plan executed as written. The `docs/.../settings/index.md` page was listed as a modified file but needed no change (no drift, see above).

## Requirements

EDIT-02 is left Pending: this plan only contributes (the wait, its expiry, `/cancel`, any replica). Plan 11-11 completes it when a typed reply is read. `requirements-completed: []`.

## Known Stubs

None. (The RED-step placeholders were replaced in the GREEN commit.)

## Threat Flags

None. T-11-19 (keys built from the sender's own ID, `TestSettingsEditCancelCommandTouchesOnlyTheSender`), T-11-20 (claim by nonce, finish and release by claim holder, album SET NX), T-11-21 (the Cancel script compares c, p, t and refuses while a claim is held), T-11-22 (the hash holds IDs, codes, a nonce and an expiry only), T-11-23 (finish and release on `context.WithoutCancel` with their own bound) are in place; T-11-SC: no package installed, go.mod and go.sum unchanged.

## Self-Check: PASSED

- FOUND: fuku/modules/settings_input.go, fuku/modules/settings_input_test.go, fuku/modules/settings.go, AGENTS.md
- FOUND commits 4c4940a, 30de1a7, 42cf93c, c7227a1 (ancestors of HEAD)
