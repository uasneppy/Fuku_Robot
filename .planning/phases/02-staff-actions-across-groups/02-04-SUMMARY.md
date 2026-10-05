---
phase: 02-staff-actions-across-groups
plan: 04
subsystem: staff-moderation
tags: [telegram, gotgbot, username, text-mention, utf16, parser, i18n]

requires:
  - phase: 02-staff-actions-across-groups
    provides: "Plans 02-01 to 02-03: StaffActions interceptor, confirm card, decision table, duration parsing"
provides:
  - "user.FindUsersByUsername: case-insensitive, all-matches username lookup over the users table with last activity, returning database errors"
  - "Staff parser target forms: text_mention at the first argument (UTF-16 offsets), @username, numeric ID; staffParseBareReply and staffParseBadUsername"
  - "resolveStaffTarget: unknown, ambiguous and failed username lookups refused with a way forward, never guessed"
  - "Six refused variants (sban, dban, skick, dkick, smute, dmute) in a Staff Group via staffCommandSpec.Refused; 13 intercepted command names"
  - "Seven staff hint keys (one updated, six new) in all 7 locales"
affects: [02-05, 02-06, 02-07, phase-03-audit-undo]

actuals:
  tokens: 15800
  tasks: 2
  commits: 4

plan_head_before: 36f1e70192ec3177454e53de782d816a32c27d88
plan_head_after: b3c883cdb531aac6b43d406ac243ebbe788cad0e
commits: 4

tech-stack:
  added: []
  patterns:
    - "A package-level function variable (staffUserLookup) as the one test seam for a DB-backed lookup, instead of a mock library"
    - "User-controlled text (usernames, stored names, command names, dates) goes into a hint through a placeholder token spliced in after translation"
    - "Staff target parsing never calls the extraction package's user resolvers (case-sensitive, channel and getChat fallbacks)"

key-files:
  created:
    - alita/db/user/find_by_username_test.go
    - alita/modules/staff_action_target_test.go
  modified:
    - alita/db/user/repository.go
    - alita/modules/staff_action_parse.go
    - alita/modules/staff_action.go
    - alita/modules/staff_action_parse_test.go
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
  - "FindUsersByUsername returns its database error, unlike the exact-case GetUserIdByUserName left unchanged beside it, so a failed lookup can never read as 'never seen'."
  - "The refused variants are table rows with Refused set and no Kind; the refusal runs right after the anonymous check, before Redis, parsing and any lookup."
  - "The ambiguity list falls back from last_activity to updated_at to created_at for the date, and omits the 'last seen' part only when all three are zero."
  - "A text_mention is honoured only when its UTF-16 offset equals where the first argument starts; anywhere else it is ignored and the first field is parsed normally."

patterns-established:
  - "A new staff target form is one branch in parseStaffActionArgs plus one case in resolveStaffTarget"

requirements-completed: [STAFF-01, STAFF-03, STAFF-06, STAFF-13]

coverage:
  - id: D1
    description: "FindUsersByUsername matches case-insensitively by equality (underscore literal), strips a leading @, returns newest last_activity first, honours the limit, returns (nil, nil) for an empty name without querying, carries id, username, name and last activity, and returns a database error"
    requirement: "STAFF-01"
    verification:
      - kind: integration
        ref: "alita/db/user/find_by_username_test.go#TestFindUsersByUsernameCaseInsensitiveNewestFirst"
        status: pass
      - kind: integration
        ref: "alita/db/user/find_by_username_test.go#TestFindUsersByUsernameReturnsDatabaseError"
        status: pass
      - kind: integration
        ref: "alita/db/user/find_by_username_test.go#TestFindUsersByUsernameUnderscoreIsLiteral"
        status: pass
    human_judgment: false
  - id: D2
    description: "The staff parser reads @username (4-32 of A-Za-z0-9_), a text_mention at the first argument with UTF-16 offsets (also in captions), keeps a later mention in the reason, and gives a bare reply its own result while a reply plus an explicit target uses the explicit one"
    requirement: "STAFF-03"
    verification:
      - kind: unit
        ref: "alita/modules/staff_action_parse_test.go#TestStaffActionParseUsername"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_action_parse_test.go#TestStaffActionParseTextMention"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_action_parse_test.go#TestStaffActionParseBareReply"
        status: pass
    human_judgment: false
  - id: D3
    description: "In a Staff Group an @username resolves case-insensitively to one stored user whose name and ID show on the card and Confirm bans that ID; a text_mention card shows the mention name and entity user ID with the reason intact"
    requirement: "STAFF-01"
    verification:
      - kind: integration
        ref: "alita/modules/staff_action_target_test.go#TestStaffActionUsernameTarget"
        status: pass
      - kind: integration
        ref: "alita/modules/staff_action_target_test.go#TestStaffActionTextMention"
        status: pass
      - kind: integration
        ref: "alita/modules/staff_action_target_test.go#TestStaffActionUsernameCaseInsensitive"
        status: pass
    human_judgment: false
  - id: D4
    description: "An unseen username (even one that exists only in the channels table), an ambiguous username (every match listed, escaped, newest first) and a failed lookup are each refused with no card and no write; a failure reads 'could not check', never 'never seen'"
    requirement: "STAFF-03"
    verification:
      - kind: integration
        ref: "alita/modules/staff_action_target_test.go#TestStaffActionUsernameUnknown"
        status: pass
      - kind: integration
        ref: "alita/modules/staff_action_target_test.go#TestStaffActionUsernameNeverFallsBackToChannels"
        status: pass
      - kind: integration
        ref: "alita/modules/staff_action_target_test.go#TestStaffActionUsernameAmbiguous"
        status: pass
      - kind: integration
        ref: "alita/modules/staff_action_target_test.go#TestStaffActionUsernameLookupError"
        status: pass
    human_judgment: false
  - id: D5
    description: "A bare reply gets the hint with no lookup of the replied-to sender; a reply plus an explicit target acts on the explicit target only; a malformed @name gets the bad-username hint"
    requirement: "STAFF-03"
    verification:
      - kind: integration
        ref: "alita/modules/staff_action_target_test.go#TestStaffActionBareReplyHint"
        status: pass
      - kind: integration
        ref: "alita/modules/staff_action_target_test.go#TestStaffActionReplyWithExplicitTarget"
        status: pass
      - kind: integration
        ref: "alita/modules/staff_action_target_test.go#TestStaffActionBadUsername"
        status: pass
    human_judgment: false
  - id: D6
    description: "In a Staff Group /sban, /dban, /skick, /dkick, /smute and /dmute are refused with a hint naming the command, even with the per-group Bans and Mutes modules loaded, and never write; an anonymous sender of any of the 13 names gets only 'post as yourself'; outside Staff Groups all of them make the same ordered Telegram calls as before"
    requirement: "STAFF-13"
    verification:
      - kind: integration
        ref: "alita/modules/staff_action_target_test.go#TestStaffActionRefusedVariants"
        status: pass
      - kind: integration
        ref: "alita/modules/staff_action_dispatch_test.go#TestStaffActionNonStaffUnchanged"
        status: pass
      - kind: integration
        ref: "alita/modules/staff_action_dispatch_test.go#TestStaffActionAnonymous"
        status: pass
    human_judgment: false
  - id: D7
    description: "Wording of the new and updated hints in es, fr, hi, id, pt and ru, and how a real Telegram server delivers a text_mention entity on a user without a username"
    verification: []
    human_judgment: true
    rationale: "The non-English strings are my own translations without native-speaker review, and the entity shape is built by hand from the Bot API documentation; neither is observable by a test"

duration: 11min
completed: 2026-10-05
status: complete
---

# Phase 2 Plan 04: @username and mention targets Summary

**Staff can now name a target by `@username` (case-insensitive over the users table, never guessed), by a text mention, or by numeric ID; unseen, ambiguous and failed lookups are refused with a way forward, bare replies get a hint, and `/sban`, `/dban`, `/skick`, `/dkick`, `/smute` and `/dmute` are refused in a Staff Group.**

## Performance

- **Duration:** 11 min
- **Started:** 2026-10-05T06:11:56Z
- **Completed:** 2026-10-05T06:23:17Z
- **Tasks:** 2 (both TDD)
- **Files modified:** 15 (2 created, 13 modified)

## Accomplishments

- `user.FindUsersByUsername(username, limit)` strips a leading `@`, matches `LOWER(username) = LOWER(?)` by equality (an underscore is literal), orders by `last_activity DESC`, and returns the database error. It is uncached and never touches the channels table or Telegram. `GetUserIdByUserName` is untouched and a test pins its exact-case behaviour.
- `parseStaffActionArgs` has three target forms in order: a `text_mention` entity whose UTF-16 offset equals where the first argument starts (name from first plus last name, reason taken with `extractEntityText` over the UTF-16 range after the entity, so a multi-word or emoji name never shifts it; caption entities work the same), an `@username` matching `^@[A-Za-z0-9_]{4,32}$`, and the numeric ID rule. No argument plus a reply is `staffParseBareReply`; a reply plus an explicit target never reads the replied-to sender.
- `resolveStaffTarget` sits between parsing and the link check. One stored match gives the ID and stored name; none gives `staff_act_username_unknown`; several give `staff_act_username_ambiguous` with one line per user (escaped name, ID, last-seen date, newest first); a lookup error gives `staff_act_abort_check_failed`. Usernames, names, the command name and dates reach the hint through placeholder tokens spliced in after translation.
- Six refused variants are rows in `staffActionCommands` with `Refused: true`. The refusal runs right after the anonymous check and before Redis, parsing and any lookup, so a variant makes no Telegram call, creates no card and never reaches the per-group `Bans` and `Mutes` handlers.
- Seven keys in all 7 locales: `staff_act_hint_need_target` updated, and `staff_act_hint_bad_username`, `staff_act_hint_no_reply`, `staff_act_hint_variant`, `staff_act_username_unknown`, `staff_act_username_ambiguous`, `staff_act_username_last_seen` added.
- AGENTS.md gains a "Staff targets" bullet next to the durations rule (the repo rule asks for it in the same change; it is not in the plan's `files_modified`).

## Task Commits

1. **Task 1: case-insensitive all-matches username lookup, test-first** - `85535e0` (test, RED), `2cd9997` (feat, GREEN)
2. **Task 2: @username, mention, bare-reply hint and refused variants, test-first** - `e9e70aa` (test, RED), `b3c883c` (feat, GREEN)

**Plan metadata:** committed with this SUMMARY (docs: complete plan)

## TDD Gate Compliance

Both tasks are `tdd="true"`; each has a RED commit before its GREEN commit.

- **Task 1 RED (`85535e0`):** `go test -tags testtools -race -count=1 -run '^TestFindUsersByUsername|^TestGetUserIdByUserNameUnchanged' ./alita/db/user` exited 1. Five target tests failed on the planned behaviour, for example `FindUsersByUsername("SPAMBOT1") ids = [], want [9102 9101] (newest last_activity first)`, `limit 1 ids = [], want [9112]`, `FindUsersByUsername = ([], <nil>), want one row` and `FindUsersByUsername error = nil, want the database error`. The no-match, empty-input and `GetUserIdByUserName` guards passed at once, as regression guards should. The RED commit declares a stub `FindUsersByUsername` returning `(nil, nil)` so the tests compile.
- **Task 1 GREEN (`2cd9997`):** the same command and `go test -tags testtools -race -count=1 ./alita/db/user` pass.
- **Task 2 RED (`e9e70aa`):** `go test -tags testtools -race -count=1 -run '^TestStaffAction(Parse|Username|BadUsername|TextMention|BareReplyHint|ReplyWithExplicitTarget|RefusedVariants|NonStaffUnchanged|Anonymous)' ./alita/modules` exited 1 on about 60 failing tests and sub-tests, all on planned behaviour: `parse "/ban @SpamBot1 2d x" result = 2, want 0`, `result = 1, want BareReply`, `messages to the Staff Group = ["@@staff-act-hint-bad-target@@"], want exactly one with staff_act_username_unknown`, `messages ... = ["@@staff-act-hint-need-target@@"], want exactly one with staff_act_hint_no_reply`, the variant cases answering `@@chat-status-user-admin-cmd-error@@` from the per-group handler instead of the variant hint, 18 anonymous-variant sub-tests with no reply, and `staff commands = 7, want the 7 staff commands and the 6 refused variants`. `TestStaffActionNonStaffUnchanged`, `TestStaffActionReplyWithExplicitTarget` and the parser cases that pin already-true behaviour passed at once. The RED commit declares the two new parse result constants, the `Refused` field, `staffUsernameMatchLimit` and `staffUserLookup` so the tests compile; nothing in it parses a username or mention.
- **Task 2 GREEN (`b3c883c`):** the same command passes (21 s), plus the full `./alita/modules ./alita/db/user ./alita/i18n` race run, `TestStaffLocaleKeys`, `make check-translations` and `make check-docs`.
- **REFACTOR:** none.
- `gsd_run check tdd-red-evidence` was not run (no resolver in this sandbox); the evidence above records command, exit code, failing tests and assertion text instead.

## Files Created/Modified

- `alita/db/user/repository.go` - `FindUsersByUsername`
- `alita/db/user/find_by_username_test.go` - lookup tests, including a failing database
- `alita/modules/staff_action_parse.go` - `staffUsernameRe`, `utf16Len`, `staffMentionAt`, the three target forms, two new parse results
- `alita/modules/staff_action.go` - `Refused`, six variant rows, `staffUserLookup`, `staffUsernameMatchLimit`, `resolveStaffTarget`, the ambiguity list, hint mappings
- `alita/modules/staff_action_target_test.go` - end-to-end target, refusal and variant tests
- `alita/modules/staff_action_parse_test.go`, `staff_action_dispatch_test.go` - parser cases; non-staff and anonymous suites widened to the new commands
- `locales/*.yml` (7), `AGENTS.md`

## Decisions Made

- `staffUserLookup` is a package variable, the plan's named test seam, and the only one; the lookup-failure path is the only thing it is replaced for. The ambiguity, unknown and case tests run against real SQLite rows.
- The test locale is the repo's marker locale, so tests assert which key a reply used and that its placeholders were spliced, not literal English.
- The new translations for es, fr, hi, id, pt and ru are my own and have not had a native-speaker review.

## Deviations from Plan

None - plan executed exactly as written.

Small additions inside the planned scope: a `staffLastSeen` fallback (last activity, then updated, then created) so an old row with no `last_activity` still shows a date, extra tests (`TestStaffActionUsernameCaseInsensitive`, `TestStaffActionUsernameNeverFallsBackToChannels`, `TestStaffActionUsernameAmbiguousSameCaseDuplicates`, `TestStaffActionUsernameLookupAsksForMoreThanOne`, `TestStaffActionBadUsername`, `TestStaffActionReplyWithExplicitTarget`, `TestStaffActionRefusedVariantsWithoutArguments`, `TestStaffActionParseRefusedTable`), and the AGENTS.md bullet.

## Deferred Issues

- `alita/modules/greetings_command_test.go` is not gofmt-clean on the base commit. Pre-existing and unrelated, left alone.
- `make lint` cannot run in this sandbox (the installed golangci-lint is built with go1.25, the module targets 1.26.0). `gofmt -l` on all changed Go files (clean), `go vet -tags testtools ./alita/...`, `CGO_ENABLED=0 go build ./...`, `make check-translations` and `make check-docs` all pass.
- Human check, outstanding: in a throwaway Staff Group, mention a user who has no username with `/ban <mention> 1h test` (pick the person from Telegram's suggestion list so a real `text_mention` entity is sent) and confirm the card shows their name and ID; then try an `@username` shared by two test accounts to see the ambiguity list.
- Not done and not in scope: a functional index on `LOWER(username)`. The plain-index scan is acceptable at this deployment size (RESEARCH Q6) and no migration was added.

## Issues Encountered

None.

## Known Stubs

None. (The temporary `FindUsersByUsername` stub exists only in RED commit `85535e0` and is replaced in `2cd9997`; the RED commit `e9e70aa` constants and fields are used in `b3c883c`.)

## Threat Flags

None. No new network endpoint, auth path or schema change. T-02-15 (spoofing) is mitigated by the case-insensitive multi-match refusal, refusal of unseen usernames, no channels or live-Telegram fallback (`TestStaffActionUsernameNeverFallsBackToChannels`), and the resolved name and ID on the card before Confirm. T-02-17 by UTF-16 offsets and the first-argument rule (`TestStaffActionParseTextMention`, `TestStaffActionTextMention`). T-02-18 by `TestStaffActionRefusedVariants`, which loads the per-group modules too. T-02-16 is accepted as planned; stored names in the ambiguity list are HTML-escaped and capped at 64 runes.

## User Setup Required

None - no external service configuration required.

## Verification Run

- `go test -tags testtools -race -count=1 ./alita/modules ./alita/db/user ./alita/i18n` - pass
- `go test -tags testtools -count=1 ./alita/...` - pass
- `CGO_ENABLED=0 go build ./...` - pass
- `go vet -tags testtools ./alita/...` - pass
- `make check-translations`, `make check-docs` - pass
- Acceptance greps: `func FindUsersByUsername(username string, limit int) ([]models.User, error)`, `LOWER(username) = LOWER(?)`, `staffUserLookup = user.FindUsersByUsername`, `Name: "dmute", Refused: true`, `extractEntityText(` in the parser all match; the count of `extraction.(GetUserId|ExtractUserAndText|ExtractUser)(` calls in the two staff files is 0
- go.mod and go.sum untouched

## Next Phase Readiness

- Staff targets now arrive as a resolved ID plus display name for every form, so later plans (log-channel posts, audit record, undo) can rely on `card.Target` and `card.TargetName` without caring how the target was typed.
- Live-Telegram verification of text mentions and of ambiguity lists is still outstanding (see Deferred Issues).

---
*Phase: 02-staff-actions-across-groups*
*Completed: 2026-10-05*

## Self-Check: PASSED

- Created files `alita/db/user/find_by_username_test.go` and `alita/modules/staff_action_target_test.go` exist.
- Task commits `85535e0`, `2cd9997`, `e9e70aa`, `b3c883c` are present in `git log`, and `git rev-list --count 36f1e70..HEAD` was 4 when this file was written.
- All task acceptance criteria and the plan-level verification commands were re-run and pass (see Verification Run).
