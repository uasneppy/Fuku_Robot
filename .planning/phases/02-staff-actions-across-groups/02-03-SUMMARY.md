---
phase: 02-staff-actions-across-groups
plan: 03
subsystem: staff-moderation
tags: [telegram, gotgbot, duration, parser, tban, tmute, i18n]

requires:
  - phase: 02-staff-actions-across-groups
    provides: "Plans 02-01 and 02-02: StaffActions interceptor, Redis confirm card, per-group live check chain, decideStaffAction decision table (never-shorten and upgrade rules keyed on newUntil)"
provides:
  - "extraction.ParseDurationToken: pure exported m/h/d/w parser with DurationSpec, ErrDurationInvalid, ErrDurationTooLong"
  - "Staff parser duration modes (none, optional, required) and the staffParseNeedDuration / staffParseBadDuration results"
  - "Timed staff /ban and /mute, plus /tban and /tmute in the Staff Group, with one until_date computed at Confirm and sent to every group"
  - "Card header duration label (as typed, permanent, or permanent (longer than 366 days)) in 7 locales"
affects: [02-04, 02-05, 02-06, 02-07, phase-03-audit-undo]

actuals:
  tokens: 15000
  tasks: 2
  commits: 4

plan_head_before: e4cb3c05da163c45e40b00314f8b53a438d9ab51
plan_head_after: 739bdc1a8c9f48b1b571656882ec2b92df604fe7
commits: 4

tech-stack:
  added: []
  patterns:
    - "One m/h/d/w grammar (extraction.ParseDurationToken) for per-group and staff commands; the per-group parser itself is untouched"
    - "The card stores the typed duration, never an end date; the end date is computed once at Confirm and handed to the fan-out as a single value"
    - "A duration field that is not a duration stays part of the reason, so the card shows 'permanent' and a misparse is visible before Confirm"

key-files:
  created:
    - alita/utils/extraction/duration_token_test.go
    - alita/modules/staff_action_parse_test.go
    - alita/modules/staff_action_timed_test.go
  modified:
    - alita/utils/extraction/extraction.go
    - alita/modules/staff_action_parse.go
    - alita/modules/staff_action.go
    - alita/modules/staff_action_card.go
    - alita/modules/staff_action_summary.go
    - alita/modules/staff_action_dispatch_test.go
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml
    - AGENTS.md

key-decisions:
  - "Only the staff parser maps an over-limit duration (above 366 days, or too large for int64) to permanent; per-group /tban keeps refusing it with its existing reply (STAFF-13). Telegram never receives a clamped value."
  - "The grammar is exactly ^[0-9]+[mhdw]$, lowercase; '2D', '12', '-1d', '+2d' and '1.5d' are not durations and land in the reason (D-01)."
  - "parseTemporaryDuration was left as it was rather than refactored onto shared code, so the per-group commands cannot change; a pinning test proves its answers."
  - "Cards saved before this plan have no duration fields; loadStaffActionCard reads them as permanent instead of failing."

patterns-established:
  - "Adding a duration-bearing staff command is one row in staffActionCommands with a staffDurationMode"

requirements-completed: [STAFF-01, STAFF-02, STAFF-03, STAFF-13]

coverage:
  - id: D1
    description: "ParseDurationToken accepts only lowercase <digits><m|h|d|w>, treats 366d as timed, anything longer or overflowing as too long, zero as invalid, and everything else as not a duration"
    requirement: "STAFF-02"
    verification:
      - kind: unit
        ref: "alita/utils/extraction/duration_token_test.go#TestParseDurationToken"
        status: pass
      - kind: unit
        ref: "alita/utils/extraction/duration_token_test.go#TestParseDurationTokenAgreesWithExistingGrammar"
        status: pass
    human_judgment: false
  - id: D2
    description: "The staff parser reads an optional duration for ban and mute, requires one for tban and tmute, reads none for kick, unban and unmute, and keeps a non-duration word as the start of the reason"
    requirement: "STAFF-02"
    verification:
      - kind: unit
        ref: "alita/modules/staff_action_parse_test.go#TestStaffActionParseOptionalDuration"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_action_parse_test.go#TestStaffActionParseRequiredDuration"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_action_parse_test.go#TestStaffActionParseNoDuration"
        status: pass
    human_judgment: false
  - id: D3
    description: "Timed /ban, /mute, /tban and /tmute from the Staff Group send one until_date (Confirm time plus the duration) to every group; the card shows the duration as typed before Confirm"
    requirement: "STAFF-03"
    verification:
      - kind: integration
        ref: "alita/modules/staff_action_timed_test.go#TestStaffActionTimedBan"
        status: pass
      - kind: integration
        ref: "alita/modules/staff_action_timed_test.go#TestStaffActionTimedMute"
        status: pass
      - kind: integration
        ref: "alita/modules/staff_action_timed_test.go#TestStaffActionTimedLabelIsExact"
        status: pass
      - kind: integration
        ref: "alita/modules/staff_action_timed_test.go#TestStaffActionTbanTmuteWork"
        status: pass
    human_judgment: false
  - id: D4
    description: "A timed staff ban or mute never shortens a longer or permanent one and upgrades a shorter one; durations over 366 days are permanent with the over-limit wording, while exactly 366d is timed"
    requirement: "STAFF-01"
    verification:
      - kind: integration
        ref: "alita/modules/staff_action_timed_test.go#TestStaffActionTimedNeverShortens"
        status: pass
      - kind: integration
        ref: "alita/modules/staff_action_timed_test.go#TestStaffActionOverLimitIsPermanent"
        status: pass
      - kind: integration
        ref: "alita/modules/staff_action_timed_test.go#TestStaffActionLimitBoundary"
        status: pass
    human_judgment: false
  - id: D5
    description: "/tban and /tmute without a valid duration, and any command with a zero amount, get a hint and no card; 2D and unit-less numbers become part of the reason with a visible permanent label"
    requirement: "STAFF-02"
    verification:
      - kind: integration
        ref: "alita/modules/staff_action_timed_test.go#TestStaffActionTbanTmuteNeedDuration"
        status: pass
      - kind: integration
        ref: "alita/modules/staff_action_timed_test.go#TestStaffActionDurationTokenRules"
        status: pass
    human_judgment: false
  - id: D6
    description: "Per-group /tban, /tmute, /ban and /mute outside Staff Groups make the same ordered Telegram calls as before, extraction's per-group parser answers are unchanged, and anonymous senders of tban and tmute get only 'post as yourself'"
    requirement: "STAFF-13"
    verification:
      - kind: integration
        ref: "alita/modules/staff_action_dispatch_test.go#TestStaffActionNonStaffUnchanged"
        status: pass
      - kind: integration
        ref: "alita/modules/staff_action_dispatch_test.go#TestStaffActionAnonymous"
        status: pass
      - kind: unit
        ref: "alita/utils/extraction/duration_token_test.go#TestParseTemporaryDurationUnchanged"
        status: pass
    human_judgment: false
  - id: D7
    description: "Wording of the duration labels and hints in es, fr, hi, id, pt and ru, and how a real Telegram server treats an until_date on a restricted or kicked member"
    verification: []
    human_judgment: true
    rationale: "The non-English strings are my own translations without native-speaker review, and the fake models Telegram's until_date handling from the Bot API documentation; neither is observable by a test"

duration: 25min
completed: 2026-10-05
status: complete
---

# Phase 2 Plan 03: Timed staff actions Summary

**Staff can now write `/ban <ID> 2d spamming`, `/mute <ID> 30m`, `/tban` and `/tmute` in the Staff Group: one shared m/h/d/w grammar (`extraction.ParseDurationToken`), the parsed duration shown on the card before Confirm, anything over 366 days applied and shown as permanent, and a single `until_date` computed at Confirm and sent to every linked group.**

## Performance

- **Duration:** 25 min
- **Tasks:** 2 (both TDD)
- **Files modified:** 17 (3 created, 14 modified)

## Accomplishments

- `extraction.ParseDurationToken` is a pure, strict parser: only lowercase `<digits><m|h|d|w>` matches; zero is `ErrDurationInvalid`; more than 366 days, or an amount that overflows int64 or overflows once multiplied, is `ErrDurationTooLong` with the typed amount and unit kept when readable. It neither replies nor reads the clock.
- `parseStaffActionArgs` takes a `staffDurationMode` from the command table: `/ban` and `/mute` read an optional duration right after the target, `/tban` and `/tmute` require one (`staffParseNeedDuration`), a zero amount is `staffParseBadDuration`, and `/kick`, `/unban`, `/unmute` read none. A non-duration word such as `2D` stays the start of the reason, so the card reads "permanent" and the misparse is visible.
- The card carries `duration_s`, `dur_n`, `dur_u` and `over_limit` in its Redis hash. `staffActionConfirm` computes `extraction.TemporaryUntilDate(time.Now().Unix(), DurationSec)` exactly once and passes that one value to the fan-out, so every group (and later retries) gets the same end date. A failure there aborts the card with the check-failed text.
- `staffDurationLabel` renders the header segment: "{n} minute(s)" and the hour, day, week forms with the amount exactly as typed (90m stays "90 minute(s)"), "permanent", or "permanent (longer than 366 days)". Seven new keys in all 7 locales.
- The decision table from plan 02-02 needed no change: with real end dates, a 1d ban over a permanent ban is skipped, a 2d mute over a 1d mute is applied, and a 1h mute over a 1d mute is skipped.
- AGENTS.md gains a "Staff durations" bullet next to the never-lift-a-ban rule.

## Task Commits

1. **Task 1: duration-token parser and staff duration modes, test-first** - `c79e810` (test, RED), `5e762a5` (feat, GREEN)
2. **Task 2: timed staff actions end to end, test-first** - `e09160f` (test, RED), `739bdc1` (feat, GREEN)

**Plan metadata:** committed with this SUMMARY (docs: complete plan)

## TDD Gate Compliance

Both tasks are `tdd="true"`; each has a RED commit before its GREEN commit.

- **Task 1 RED (`c79e810`):** `go test -tags testtools -count=1 -run 'ParseDurationToken|ParseTemporaryDuration' ./alita/utils/extraction` exited 1: `TestParseDurationToken` failed on 10 sub-tests (every valid and error case) with "matched = false, want true", and `TestParseDurationTokenAgreesWithExistingGrammar` failed on "want a valid duration". `go test -tags testtools -race -count=1 -run '^TestStaffActionParse' ./alita/modules` exited 1 on 16 sub-tests, each on the planned behaviour, for example `parse "/ban 42 2d spam" = {sec 0, ... reason "2d spam"}, want {sec 172800, amount 2, unit "d", ... reason "spam"}` and `parse "/tban 42 spam" result = 0, want 3`. `TestParseTemporaryDurationUnchanged` and the pure no-duration and "stays in the reason" cases passed at once, which is expected: they are regression guards that must hold before and after. The RED commit declares the new types, constants, error values, spec field and a stub `ParseDurationToken` returning not-matched so the tests compile; nothing in it parses a duration.
- **Task 1 GREEN (`5e762a5`):** the same commands pass, plus `TestTemporaryUntilDate` and all `^TestStaff` tests.
- **Task 2 RED (`e09160f`):** `go test -tags testtools -race -count=1 -run '^TestStaffAction(TimedBan|TimedLabelIsExact|TimedMute|TimedNeverShortens|OverLimitIsPermanent|LimitBoundary|TbanTmuteNeedDuration|TbanTmuteWork|DurationTokenRules|KickIgnoresDuration|NonStaffUnchanged|Anonymous)$' ./alita/modules` exited 1 on 25 failing tests/sub-tests, all on planned behaviour: the card read "permanent" where "2 day(s)" was wanted, `until_date = 0, want Confirm time + 86400` on both upgrade cases, "write calls = [banChatMember...] want none" where a shorter timed action must be skipped, "no confirm card was sent" for `/tban` and `/tmute` (not registered), "messages to the Staff Group = [], want exactly one with staff_act_hint_need_duration", and "staff commands = 1, want ..." for the widened command-count check. The permanent-ban-skip case, `TestStaffActionDurationTokenRules`, `TestStaffActionKickIgnoresDuration` and `TestStaffActionNonStaffUnchanged` passed at once (already-true behaviour or regression guards).
- **Task 2 GREEN (`739bdc1`):** the set above passes, plus `TestStaffActionHeaderDuration`, `TestStaffLocaleKeys`, `make check-translations` and `make check-docs`.
- **REFACTOR:** none.
- `gsd_run check tdd-red-evidence` was not run (no `gsd_run` resolver in this sandbox); the evidence above records command, exit code, failing tests and assertion text instead.

## Files Created/Modified

- `alita/utils/extraction/extraction.go` - `DurationSpec`, `ErrDurationInvalid`, `ErrDurationTooLong`, `ParseDurationToken`; `ExtractTime` and `parseTemporaryDuration` untouched
- `alita/utils/extraction/duration_token_test.go` - token grammar, agreement with the per-group grammar, pinned per-group answers
- `alita/modules/staff_action_parse.go` - `staffDurationMode`, duration fields on the request, two new parse results, `readStaffDuration`
- `alita/modules/staff_action.go` - `Duration` on the command spec, `tban` and `tmute` rows, hint replies, duration copied to the card
- `alita/modules/staff_action_card.go` - duration fields in the hash (tolerant load), `newUntil` computed once at Confirm
- `alita/modules/staff_action_summary.go` - `staffDurationLabel`, used by the header
- `alita/modules/staff_action_parse_test.go`, `staff_action_timed_test.go` - new tests; `staff_action_dispatch_test.go` - non-staff and anonymous suites widened to tban and tmute
- `locales/*.yml` (7) - 7 keys each; `AGENTS.md` - staff durations rule

## Decisions Made

- Over-limit handling lives only in the staff parser (see key-decisions); per-group `/tban` still refuses with its reply.
- The per-group parser was not refactored onto shared code; "must stay unchanged" is easier to prove by leaving it alone and pinning its answers.
- The new translations for es, fr, pt, id, ru and hi are my own and have not had a native-speaker review. Plural handling is "(s)" or an invariant abbreviation, as the plan scoped.

## Deviations from Plan

None - plan executed exactly as written.

Small additions inside the planned scope: `TestStaffActionTimedLabelIsExact`, `TestStaffActionLimitBoundary`, `TestStaffActionTbanTmuteWork` and `TestStaffActionKickIgnoresDuration` in the timed test file (the 366d/367d boundary and exact-label requirements in the plan's truths), tolerant loading of cards saved without duration fields, and the AGENTS.md bullet that the repo rule "update AGENTS.md in the same change" asks for.

## Deferred Issues

- `alita/modules/greetings_command_test.go` is not gofmt-clean on the base commit. Pre-existing and unrelated, left alone.
- `make lint` cannot run in this sandbox (installed golangci-lint is built with go1.25, module targets 1.26.0). `gofmt -l` on all changed Go files (clean), `go vet -tags testtools ./alita/...`, `CGO_ENABLED=0 go build ./...`, `make check-translations` and `make check-docs` all pass.
- Human check, outstanding: in a throwaway Staff Group with two linked supergroups, run `/ban <test id> 2d test` and Confirm; each group should show the member as restricted-out until about 48 hours later (Telegram's own banned-until display), and a following `/ban <id> 1d` should read "skipped: already banned" in both.

## Issues Encountered

None.

## Known Stubs

None. (The temporary `ParseDurationToken` stub that exists only in the RED commit `c79e810` is replaced in `5e762a5`.)

## Threat Flags

None. No new network endpoint, auth path or schema change. T-02-13 (free-text duration) is mitigated by the strict lowercase grammar, the always-visible duration on the card and `TestStaffActionDurationTokenRules`; T-02-14 (overflow) by the pre-multiplication checks and the `99999999999999999999d` and `9223372036854775807w` cases.

## User Setup Required

None - no external service configuration required.

## Verification Run

- `go test -tags testtools -race -count=1 ./alita/modules ./alita/utils/extraction ./alita/i18n` - pass
- `go test -tags testtools -count=1 -run '^TestStaffLocaleKeys$' ./alita/i18n` - pass
- `CGO_ENABLED=0 go build ./...` - pass
- `go vet -tags testtools ./alita/...` - pass
- `make check-translations`, `make check-docs` - pass
- Acceptance greps: `func ParseDurationToken(token string) (spec DurationSpec, matched bool, err error)`, `ErrDurationTooLong` and `extraction.TemporaryUntilDate(` and `"duration_s"` in the card, `extraction.ParseDurationToken(` in the parser, `Name: "tban"` in `staff_action.go` - all match
- `go test -tags testtools -count=1 -run '^TestTemporaryUntilDate$' ./alita/utils/extraction` - pass
- go.mod and go.sum untouched

## Next Phase Readiness

- Later plans can send timed actions through `startStaffActionRun`; `newUntil` is now real and identical for every group and retry.
- Live-Telegram verification of timed bans and mutes is still outstanding (see Deferred Issues).

---
*Phase: 02-staff-actions-across-groups*
*Completed: 2026-10-05*

## Self-Check: PASSED

- Created files `alita/utils/extraction/duration_token_test.go`, `alita/modules/staff_action_parse_test.go`, `alita/modules/staff_action_timed_test.go` exist.
- Task commits `c79e810`, `5e762a5`, `e09160f`, `739bdc1` are present in `git log`, and `git rev-list --count e4cb3c0..HEAD` was 4 when this file was written.
- All task acceptance criteria and the plan-level verification commands were re-run and pass (see Verification Run).
