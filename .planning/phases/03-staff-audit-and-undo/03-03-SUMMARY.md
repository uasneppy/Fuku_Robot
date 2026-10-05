---
phase: 03-staff-audit-and-undo
plan: 03
subsystem: modules
tags: [telegram, staff-actions, log-channel, pacing, run-engine, i18n]

requires:
  - phase: 03-staff-audit-and-undo
    provides: saveStaffGroupResult, finalizeStaffActionRecord and the audit record written by plan 03-01
  - phase: 02-staff-actions-across-groups
    provides: staff action card, fan-out coordinator, staffPaced, staffChatTranslator
provides:
  - actionlog.Destination, the channel/category/header seam shared with actionlog.Log
  - staffRunSpec and startStaffRun, the one spec-driven run engine every staff run uses
  - staff_log.go (composeStaffActionLog, postStaffActionLog, staffLogHashtag, staffLogReason)
  - staff_log_admin_label, staff_log_user_label, staff_log_via_staff_group in all 7 locales
  - one paced post per applied group in that group's admin log channel
affects: [03-06 undo run (second staffRunSpec), 03-02, 03-04, 03-07, 03-08]

actuals:
  tokens: 7500
  tasks: 2
  commits: 4
plan_head_before: f48b9884c29cf9a1c8ac90d274a6072b9355acdc
plan_head_after: 28aaea79c0b6f86d47570f11a7f0d4c397f307e2

tech-stack:
  added: []
  patterns:
    - "Spec-driven run engine: per-group work and hooks are functions on staffRunSpec, the coordinator is shared"
    - "Log posts live in the worker after the result is set, outside the result path, and are warn-logged on failure"
    - "Labels are translated in the linked group's own language; user text is spliced in after translation"

key-files:
  created:
    - alita/modules/staff_log.go
    - alita/modules/staff_log_test.go
  modified:
    - alita/modules/staff_action_run.go
    - alita/utils/actionlog/actionlog.go
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml
    - AGENTS.md

key-decisions:
  - "The post is sent through staffPaced with b.SendMessageWithContext, not actionlog.Log, whose send is unpaced; actionlog.Destination is split out so the lookup, category check and header stay in one place"
  - "Post labels use the linked group's own language (staffChatTranslator(link.GroupChatID)); hashtags are not localized"
  - "Reason cap for posts is 300 runes, applied before HTML escaping"
  - "A run cancelled by StopStaffActions skips its remaining posts; accepted and written into AGENTS.md"

patterns-established:
  - "startStaffRun(b, staffRunSpec{Card, Links, ChatID, MsgID, Group, AfterGroup, Render, Finish})"
  - "AfterGroup hook order: progress.set, then record write, then log post"

requirements-completed: [STAFF-09]

coverage:
  - id: D1
    description: "Each linked group where a staff action was applied gets one post in its admin log channel with the #STAFF_<KIND> hashtag, the issuer mention, the target's name and ID, the duration for ban and mute, the reason (or 'no reason given') and 'via Staff Group', under the usual group header"
    requirement: "STAFF-09"
    verification:
      - kind: unit
        ref: "alita/modules/staff_log_test.go#TestStaffActionLogPosts"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_log_test.go#TestStaffActionLogNoneWhenNothingApplied"
        status: pass
    human_judgment: false
  - id: D2
    description: "Skipped or failed groups, groups without a log channel and groups with the admin category off get no post, nothing is sent into a linked group's own chat, and no post names the Staff Group's title or ID"
    requirement: "STAFF-09"
    verification:
      - kind: unit
        ref: "alita/modules/staff_log_test.go#TestStaffActionLogPosts"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_log_test.go#TestStaffActionLogCategoryOff"
        status: pass
    human_judgment: false
  - id: D3
    description: "A 403, a retried 429 or an exhausted 429 on the log channel never changes the group's done line, the tally or the recorded outcome; the post is paced through staffPaced"
    requirement: "STAFF-09"
    verification:
      - kind: unit
        ref: "alita/modules/staff_log_test.go#TestStaffActionLogFailureKeepsDone"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_log_test.go#TestStaffActionLogPaced"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_log_test.go#TestStaffActionLogRateLimitedKeepsDone"
        status: pass
    human_judgment: false
  - id: D4
    description: "The reason is cut to 300 runes before it is HTML-escaped, so no raw tag from it reaches Telegram"
    requirement: "STAFF-09"
    verification:
      - kind: unit
        ref: "alita/modules/staff_log_test.go#TestStaffActionLogReasonCapped"
        status: pass
    human_judgment: false
  - id: D5
    description: "Two groups sharing one log channel get one post each under their own headers; posts are sent after the group's moderation call"
    requirement: "STAFF-09"
    verification:
      - kind: unit
        ref: "alita/modules/staff_log_test.go#TestStaffActionLogSharedChannel"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_log_test.go#TestStaffActionLogPosts"
        status: pass
    human_judgment: false
  - id: D6
    description: "Every staff run goes through one spec-driven engine; Phase 2's progress, rate-limit, lock-renewal and shutdown tests pass unchanged, and actionlog.Log behaves exactly as before for existing callers"
    requirement: "STAFF-09"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestStaffAction|^TestStopStaffActions' ./alita/modules"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_log_test.go#TestActionLogDestination"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_log_test.go#TestActionLogAdminUnchanged"
        status: pass
    human_judgment: false

duration: 12min
completed: 2026-10-05
status: complete
---

# Phase 3 Plan 3: Staff action log posts Summary

**Each linked group where a staff action was applied now gets one paced post in its admin log channel (issuer, target, duration, reason, "via Staff Group", never the Staff Group's name or ID), and every staff run goes through one spec-driven engine, `startStaffRun`, that the undo run can reuse.**

## Performance

- **Duration:** about 12 min
- **Started:** 2026-10-05T17:44Z (approximate)
- **Completed:** 2026-10-05T17:57Z
- **Tasks:** 2
- **Files modified:** 12 (2 created, 10 modified)

## Accomplishments

- `startStaffRun` moves Phase 2's coordinator body verbatim behind a `staffRunSpec` (`Card`, `Links`, `ChatID`, `MsgID`, `Group`, `AfterGroup`, `Render`, `Finish`); `runStaffFanOut` replaces `runStaffActionFanOut`. `startStaffActionRun` keeps its signature and is the first spec. All Phase 2 progress, 429, lock-renewal and shutdown tests pass unchanged.
- `actionlog.Destination(chat, category)` holds `Log`'s checks and header; `Log` calls it and sends exactly the same text as before.
- `postStaffActionLog` runs in the worker right after the result is set and the record is saved, only for a done result, through `staffPaced` with the 8 s call timeout. A failure is warn-logged and never changes the result.
- Three locale keys in all 7 locales; `make check-translations` and `TestStaffLocaleKeys` pass.
- AGENTS.md records the log-post rule, the run-engine rule and the shutdown caveat, replacing the Phase 2 "nothing is posted" sentence.

## Task Commits

1. **Task 1: run engine takes a spec, applied groups get one paced post** - `4d5a68a` (test, RED) then `8663acf` (feat, GREEN)
2. **Task 2: failure, pacing, category, escaping, actionlog contract, AGENTS.md** - `223d168` (test) then `28aaea7` (feat)

**Plan metadata:** the `docs(03-03)` commit that holds this file.

## Decisions Made

- Post sent with `b.SendMessageWithContext` inside `staffPaced`, not `actionlog.Log` (its send is unpaced, D-12).
- Labels in the linked group's own language; hashtags not localized; names and reason spliced in after translation.
- Reason cap 300 runes before escaping. A run cancelled by a shutdown skips its remaining posts (research Open Question 4, written into AGENTS.md).

## TDD Gate Compliance

- Task 1: RED `4d5a68a` failed on assertions for the planned behavior (`channel of group A received 0 posts, want 1`, the shared-channel count, and the no-reason post), GREEN `8663acf` passes.
- Task 2: the plan allowed that some tests pass at once. All seven new tests passed against Task 1's implementation, because Task 1's design already keeps the post outside the result path and caps the reason before escaping. This is an expected GREEN, not a wrong test: `TestStaffActionLogReasonCapped` was checked by mutation (cap set to 250 fails it on the 301-rune assertion) after changing it to assert the literal 300 instead of the constant. The Task 2 `feat` commit holds only the AGENTS.md change the plan assigns to it.
- `gsd_run check tdd-red-evidence` was not run (the worktree copy of gsd-tools is missing `vendor/re2js.cjs`, as in plan 03-01); RED evidence was verified from the failing test output.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug in test] Reason-cap test asserted the constant, not the literal**
- **Found during:** Task 2 (mutation check of an unexpected GREEN)
- **Issue:** `TestStaffActionLogReasonCapped` computed its expectation from `staffLogReasonMaxRunes`, so changing the cap would not fail it.
- **Fix:** It asserts 300 runes plus the ellipsis as literals. Mutation to 250 now fails it.
- **Files modified:** `alita/modules/staff_log_test.go`
- **Commit:** `223d168`

**Total deviations:** 1 auto-fixed (test strength). **Impact:** none on scope.

## Issues Encountered

- `make lint` is unavailable (golangci-lint built with go1.25 refuses the go1.26.0 module); `gofmt`, `go vet -tags testtools` on `./alita/modules` and `./alita/utils/actionlog`, and `CGO_ENABLED=0 go build ./...` are clean.
- Commit trailers: the dispatcher's project notes named one model in the trailer and the session attribution reminder named another; the attribution reminder (`Claude Sonnet 5.5`) was used since it matches the model that did the work.
- `runStaffFanOut` keeps the `b *gotgbot.Bot` parameter the plan specifies although the engine no longer uses it there; it is harmless and keeps the plan's signature.

## Verification Results

- `go test -tags testtools -race -count=1 -run '^TestStaffActionLog|^TestActionLog' -v ./alita/modules` - all 10 pass
- `go test -tags testtools -race -count=1 -run '^TestStaffAction|^TestStopStaffActions' ./alita/modules` - ok
- `go test -tags testtools -race -count=1 ./alita/modules ./alita/utils/actionlog` - ok (modules 115 s; actionlog has no test files)
- `go test -tags testtools -count=1 -run '^TestStaffLocaleKeys$' ./alita/i18n` - ok
- `make check-translations` - all present; `make check-docs` - no drift
- Task 1 and Task 2 acceptance criteria (greps, `gofmt -l`, `git diff --exit-code go.mod go.sum`) - all pass

## Known Stubs

None.

## Threat Flags

None. The only new outward surface is the log post the plan's threat model covers (T-03-07 to T-03-10): `composeStaffActionLog` never reads the card's Staff Group chat, the reason is capped then escaped, every post is paced, and a failed post never changes a result.

## User Setup Required

None.

## Next Phase Readiness

- Plan 03-06's undo run can be the second `staffRunSpec`; it supplies its own `Group`, `AfterGroup`, `Render` and `Finish`.
- `actionlog.Destination` is available for any other post that must share the group header and category checks.

## Self-Check: PASSED

- Created files exist: `alita/modules/staff_log.go`, `alita/modules/staff_log_test.go`.
- Commits `4d5a68a`, `8663acf`, `223d168` and `28aaea7` exist; `test(03-03)` precedes `feat(03-03)` for Task 1.

---
*Phase: 03-staff-audit-and-undo*
*Completed: 2026-10-05*
