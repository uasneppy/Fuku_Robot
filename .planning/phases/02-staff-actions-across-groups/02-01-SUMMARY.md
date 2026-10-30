---
phase: 02-staff-actions-across-groups
plan: 01
subsystem: staff-moderation
tags: [telegram, gotgbot, redis, lua, confirm-card, fan-out, moderation, i18n]

# Dependency graph
requires:
  - phase: 01-staff-group
    provides: "Staff Group and link tables, cached gate and Fresh reads, recheckLink with shared staffOwnerPass, sendStaffNotice, staff| callback namespace, staffBotClient test fake"
provides:
  - "StaffActions module (priority 65): raw /ban interceptor ahead of Bans (70) and Mutes (80), pass-through outside Staff Groups"
  - "Redis confirm card (alita:staff:act:<token>) with a Lua compare-and-set state machine and issuer-only Confirm and Cancel"
  - "Per-group live check chain and fan-out run with the staffGroupResult outcome model and 13 reason codes"
  - "Pure decideStaffAction (ban column of the status table), parser, summary renderer and delivery"
  - "staff_act_* locale keys in all 7 locales"
  - "Stateful gotgbot.BotClient fake with Bot API restrict/unban semantics and the staffActionEnv test harness"
affects: [02-02, 02-03, 02-04, 02-05, 02-06, 02-07, phase-03-audit-undo]

# Actuals (#2632)
actuals:
  tokens: 32000
  tasks: 3
  commits: 5

# Commit measurement (#3968)
plan_head_before: 039c91525af5e38f159b5d5be1184a646440b004
plan_head_after: 50c2631d7a1f635bf053be067de94eee6c4d29b7
commits: 5

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Raw handlers.NewCommand interceptor at group 0 that returns ext.ContinueGroups with zero side effects outside its scope"
    - "Redis hash card with a redis.NewScript compare-and-set (pending to running, cancelled or expired) checking issuer and expiry inside the script"
    - "Per-group authority only from a live getChatMember answer; no cached admin predicate"
    - "User text spliced after translation through substitution tokens and escaped once"
    - "Stateful BotClient fake that applies writes to member records, so tests can prove a ban is never lifted"

key-files:
  created:
    - alita/modules/staff_action.go
    - alita/modules/staff_action_parse.go
    - alita/modules/staff_action_card.go
    - alita/modules/staff_action_decide.go
    - alita/modules/staff_action_run.go
    - alita/modules/staff_action_summary.go
    - alita/modules/staff_action_fake_test.go
    - alita/modules/staff_action_test.go
    - alita/modules/staff_action_gates_test.go
    - alita/modules/staff_action_dispatch_test.go
  modified:
    - alita/modules/staff.go
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml
    - AGENTS.md

key-decisions:
  - "StaffActions uses raw command handlers, not helpers.WrapCommand, because BuildCommandContext replies to sender-less updates and would double-reply in every non-staff chat; recorded in AGENTS.md as a documented exception"
  - "Staff actions need Redis: without a client the command is refused with staff_act_redis_unavailable and no card is created (fail closed)"
  - "The card shows the reason capped at 200 runes (cut with an ellipsis) so a cut is visible before Confirm"
  - "A numeric ID the users table has never seen is accepted; the card shows 'unknown name' next to the ID"
  - "abortStaffActionCard takes the translator as an argument so it can render the card header"

patterns-established:
  - "Interceptor gate: cached GetStaffGroup first, then GetStaffGroupFresh; a stale gate falls through to the per-group command"
  - "Every card tap answers the callback exactly once; a tap on a handled card never edits the message"
  - "Fan-out order per group: link recheck, live issuer, bot and service short-circuit, live target, decision, write"

requirements-completed: [STAFF-01, STAFF-02, STAFF-03, STAFF-04, STAFF-05, STAFF-06, STAFF-07, STAFF-08, STAFF-13, PLAT-01]

coverage:
  - id: D1
    description: "A Staff Group member sends /ban <ID> [reason], gets one confirm card, and after the issuer's Confirm the target is banned in every linked group, with the card edited into a per-group summary and no write to the Staff Group"
    requirement: "STAFF-01"
    verification:
      - kind: integration
        ref: "alita/modules/staff_action_test.go#TestStaffActionTracer"
        status: pass
    human_judgment: false
  - id: D2
    description: "Only the issuer's own Confirm or Cancel acts, once; other members, repeat taps, wrong chats, malformed or unknown tokens and expired cards change nothing"
    requirement: "STAFF-03"
    verification:
      - kind: integration
        ref: "alita/modules/staff_action_gates_test.go#TestStaffActionCancel"
        status: pass
      - kind: integration
        ref: "alita/modules/staff_action_gates_test.go#TestStaffActionCardIssuerOnly"
        status: pass
      - kind: integration
        ref: "alita/modules/staff_action_gates_test.go#TestStaffActionCardTapAnswers"
        status: pass
    human_judgment: false
  - id: D3
    description: "Per-group live gates: link owner recheck, issuer must be live creator or administrator with can_restrict_members, bot and service IDs and group creators or admins are never targeted, lookup failures fail closed with no write"
    requirement: "STAFF-05"
    verification:
      - kind: integration
        ref: "alita/modules/staff_action_test.go#TestStaffActionSkipsWhereIssuerNotAdmin"
        status: pass
      - kind: integration
        ref: "alita/modules/staff_action_gates_test.go#TestStaffActionPerGroupGates"
        status: pass
      - kind: integration
        ref: "alita/modules/staff_action_gates_test.go#TestStaffActionStaffChatNeverActedOn"
        status: pass
    human_judgment: false
  - id: D4
    description: "Confirm-time aborts (Staff status removed, issuer left, every link removed) end the card without any ban"
    requirement: "STAFF-04"
    verification:
      - kind: integration
        ref: "alita/modules/staff_action_gates_test.go#TestStaffActionConfirmAborts"
        status: pass
    human_judgment: false
  - id: D5
    description: "Anonymous admins, channel identities, linked-channel posts and sender-less updates get 'post as yourself' before any parsing or Telegram lookup"
    requirement: "STAFF-06"
    verification:
      - kind: integration
        ref: "alita/modules/staff_action_dispatch_test.go#TestStaffActionAnonymous"
        status: pass
    human_judgment: false
  - id: D6
    description: "/ban outside a Staff Group makes the same ordered Telegram calls as a dispatcher without StaffActions; a stale cached gate falls through to the per-group command; module order and exact command-name matching hold"
    requirement: "STAFF-13"
    verification:
      - kind: integration
        ref: "alita/modules/staff_action_dispatch_test.go#TestStaffActionNonStaffUnchanged"
        status: pass
      - kind: integration
        ref: "alita/modules/staff_action_dispatch_test.go#TestStaffActionStaleGatePassesThrough"
        status: pass
      - kind: unit
        ref: "alita/modules/staff_action_dispatch_test.go#TestStaffActionModuleOrder"
        status: pass
      - kind: integration
        ref: "alita/modules/staff_action_dispatch_test.go#TestStaffActionCommandNamesExact"
        status: pass
    human_judgment: false
  - id: D7
    description: "Argument parsing and refusals: need-target and bad-target hints, 200-rune reason cap counted in runes, no links, Redis required"
    requirement: "STAFF-02"
    verification:
      - kind: integration
        ref: "alita/modules/staff_action_gates_test.go#TestStaffActionParseHints"
        status: pass
      - kind: integration
        ref: "alita/modules/staff_action_gates_test.go#TestStaffActionNoLinks"
        status: pass
      - kind: integration
        ref: "alita/modules/staff_action_gates_test.go#TestStaffActionRedisRequired"
        status: pass
    human_judgment: false
  - id: D8
    description: "Behaviour against the real Telegram Bot API (banChatMember on real supergroups, real getChatMember statuses, real edit and callback handling)"
    verification: []
    human_judgment: true
    rationale: "All Telegram behaviour is exercised through a hand-written fake BotClient; there is no bot token in the sandbox, so a live check on a throwaway Staff Group is still outstanding"

# Metrics
duration: 22min
completed: 2026-10-05
status: complete
---

# Phase 2 Plan 01: Staff /ban across linked groups Summary

**Staff Group `/ban <ID> [reason]` with an issuer-only Redis confirm card, live per-group admin checks and a card that becomes the per-group summary, behind a pass-through interceptor that leaves per-group `/ban` untouched everywhere else**

## Performance

- **Duration:** 22 min
- **Started:** 2026-10-05T05:16:40Z
- **Completed:** 2026-10-05T05:38:29Z
- **Tasks:** 3
- **Files modified:** 19 (10 created, 9 modified)

## Accomplishments

- A Staff Group member sends `/ban <numeric ID> [reason]` and gets one confirm card as a reply, showing the action, the target's stored name (or "unknown name") with the ID, "permanent", the reason (or "no reason given") and "Applies to N linked group(s)". Nothing is applied before Confirm. The buttons carry only `staff|v1|a=xc|xn&t=<16 hex>` (32 bytes); everything else lives in the Redis hash `alita:staff:act:<token>`.
- Only the issuer's own tap moves the card, and only once, through a Lua compare-and-set that also checks expiry. Other members get the issuer-only alert; repeat taps get "already handled" without any edit; Cancel edits the card to "Cancelled by <name>" and never deletes it.
- After Confirm the bot re-checks the Staff Group, the issuer's membership and the link list, then visits each linked group in link-id order: owner recheck through `recheckLink`, live `getChatMember` for the issuer (creator, or administrator with `can_restrict_members`), bot and Telegram service ID short-circuit, live `getChatMember` for the target, then the decision table. A group that fails any step gets no write and a line saying why. The Staff Group is never written to, and nothing is posted into linked groups.
- The ban column of the decision table bans members, upgrades a temporary ban to permanent, bans a target who left or never joined ("banned (not in group)"), and skips already-permanently-banned targets and group creators or administrators.
- `/ban` in any chat that is not a Staff Group makes the same ordered Telegram calls as before (compared against a dispatcher without the module); anonymous admins, channel identities and linked-channel posts in a Staff Group are told to post as themselves before any lookup.
- A stateful test fake models the Bot API server's replace-the-status semantics for `restrictChatMember` and `unbanChatMember`, which later plans (mute, kick, unban) need to prove no ban is ever lifted.

## Task Commits

1. **Task 1: End-to-end `/ban <ID>` tracer** - `52635fe` (feat)
2. **Task 2: Cancel and every refusal, abort and per-group gate** - `e66317e` (test, RED), `764f47e` (feat, GREEN)
3. **Task 3: Per-group `/ban` unchanged outside Staff Groups, anonymous senders refused, AGENTS.md rules** - `719ba3b` (test), `50c2631` (docs)

**Plan metadata:** committed with this SUMMARY (docs: complete plan)

## TDD Gate Compliance

Task 2 and Task 3 are `tdd="true"`, but Task 1 was a tracer that already implemented most of their contract, so only part of the work had a genuine RED step.

- **Task 2 RED:** `e66317e` adds `staff_action_gates_test.go`. Run before the Cancel code: `go test -tags testtools -race -count=1 -run '^TestStaffAction(Cancel|CardIssuerOnly|...)$' ./alita/modules` exited 1 with exactly two failures, both on assertions for the planned behaviour: `TestStaffActionCancel` ("card button actions = [xc], want [xc xn]") and `TestStaffActionCardIssuerOnly` (an `xn` tap answered `staff_cb_expired` instead of the issuer-only alert). All other tests in the file passed at once because Task 1 already built those branches.
- **Task 2 GREEN:** `764f47e` adds the Cancel button, `staffActionCancel` and `staff_act_card_cancelled`; the same command then passed.
- **Task 3:** every test passed on first run (unexpected GREEN), because the interceptor, gate and anonymous refusal were Task 1 work. To show the new tests can fail, two mutations were applied and reverted: returning `ext.EndGroups` for a non-staff chat made `TestStaffActionNonStaffUnchanged` and `TestStaffActionStaleGatePassesThrough` fail; accepting any issuer made `TestStaffActionSkipsWhereIssuerNotAdmin` fail. The cached-gate pre-check on its own is not observable (the fresh read falls through to the same behaviour), so it is a performance guard, not a tested contract.
- **REFACTOR:** none.
- `gsd_run check tdd-red-evidence` was not run: the `gsd_run` resolver is not available in this sandbox. The RED evidence above (command, exit code, failing tests, assertion text) is recorded here instead.

## Files Created/Modified

- `alita/modules/staff_action.go` - StaffActions module registration (priority 65), `/ban` interceptor, cached gate, `handleStaffAction` up to the card
- `alita/modules/staff_action_parse.go` - `parseStaffActionArgs`, `capStaffReason` (200 runes), request types
- `alita/modules/staff_action_card.go` - Redis card, token, Lua scripts, Confirm and Cancel callbacks, abort and answer helpers
- `alita/modules/staff_action_decide.go` - kinds, outcomes, 13 reasons, target state, `decideStaffAction` (ban column)
- `alita/modules/staff_action_run.go` - per-group check chain, fan-out run, `fetchLiveMember`, `executeStaffCall`
- `alita/modules/staff_action_summary.go` - header, result lines, summary render, edit and fallback delivery
- `alita/modules/staff.go` - `xc` and `xn` actions and their callback cases
- `alita/modules/staff_action_fake_test.go` - stateful `BotClient` fake and `staffActionEnv` harness
- `alita/modules/staff_action_test.go`, `staff_action_gates_test.go`, `staff_action_dispatch_test.go` - 17 named tests
- `locales/*.yml` (7) - 31 `staff_act_*` keys each
- `AGENTS.md` - interceptor exception, card key family, staff actions authority rule

## Decisions Made

- Raw command handlers instead of `WrapCommand`, recorded in AGENTS.md as a documented exception (see key-decisions).
- Redis is required for staff actions; the command is refused without it rather than falling back to in-process card state.
- A summary is delivered by editing the card; if the edit fails the summary is posted as a new message through `sendStaffNotice`.
- The card is expired lazily on the next tap; the 5-minute timer and continuation messages belong to later plans.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] `abortStaffActionCard` takes the translator**
- **Found during:** Task 1
- **Issue:** the plan's signature `abortStaffActionCard(b, card, chatID, msgID, text)` cannot render the header the plan says it must prepend without a translator.
- **Fix:** added a `tr *i18n.Translator` parameter.
- **Files modified:** `alita/modules/staff_action_card.go`
- **Committed in:** `52635fe`

**2. [Rule 1 - Bug] Test helper treated any keyboard as a staff card**
- **Found during:** Task 3 (`TestStaffActionStaleGatePassesThrough`)
- **Issue:** the per-group `/ban` that the stale gate falls through to replies with an Unban button, so `wantNoCard` failed on a correct result.
- **Fix:** `wantNoCard` now fails only on staff Confirm or Cancel buttons.
- **Files modified:** `alita/modules/staff_action_gates_test.go`
- **Committed in:** `719ba3b`

**3. [Rule 3 - Blocking] Dispatch test needed the sender to be a live creator**
- **Found during:** Task 3
- **Issue:** the fake's default `getChatMember` answer for 777000 is "member", so the per-group `CanUserRestrict` check refused.
- **Fix:** the test stores a creator record for 777000 in that chat.
- **Files modified:** `alita/modules/staff_action_dispatch_test.go`
- **Committed in:** `719ba3b`

**4. [Rule 2 - Missing critical] Wording of the unused-var lint**
- `staffActionsCancel` has no caller until plan 02-07 adds the shutdown stop, so it carries `//nolint:unused` with the reason.
- **Files modified:** `alita/modules/staff_action_run.go`
- **Committed in:** `52635fe`

---

**Total deviations:** 4 auto-fixed (1 bug in a test helper, 2 blocking, 1 lint guard)
**Impact on plan:** None on behaviour or scope. All four are small and local.

## Deferred Issues

- `alita/modules/greetings_command_test.go` is not gofmt-clean on the base commit (found by `gofmt -l`). Pre-existing and unrelated, left alone.
- `make lint` cannot run in this sandbox: the installed golangci-lint was built with go1.25 and the module targets 1.26.0 (the same toolchain limit recorded in Phase 1). `go vet -tags testtools ./alita/...`, `gofmt -l` on the changed files, `make check-translations` and `make check-docs` all pass.

## Issues Encountered

None beyond the deviations above.

## Known Stubs

None. No placeholder values flow to rendering; "unknown name" is a real, translated state for a target the bot has never seen.

## Threat Flags

None. The new surface (Staff Group command intake, callback buttons, Redis card, per-group moderation calls) is exactly what the plan's threat model covers; each `mitigate` entry has a named test.

## User Setup Required

None - no external service configuration required.

## Verification Run

- `go test -tags testtools -race -count=1 ./alita/modules ./alita/i18n` - pass
- `go test -tags testtools -race -count=1 -run '^TestStaff' ./alita/modules` - pass (Phase 1 staff suites included)
- `CGO_ENABLED=0 go build ./...` - pass
- `go vet -tags testtools ./alita/...` - pass
- `make check-translations`, `make check-docs` - pass
- `git diff --exit-code go.mod go.sum` - unchanged
- Comment-filtered grep for `IsUserAdmin|CanUserRestrict|GetAdminCacheUser|RequireUserAdmin` over `staff_action*.go` - no matches

## Next Phase Readiness

- Plans 02-02 to 02-07 can extend `staffActionKind`, `decideStaffAction` and `staffActionCommands`; the `staffGroupResult` model, reason codes and per-group check order are fixed here.
- The test fake already models restrict and unban status replacement for the mute, kick and unban plans.
- Live-Telegram verification of the whole flow is still outstanding (coverage item D8).

---
*Phase: 02-staff-actions-across-groups*
*Completed: 2026-10-05*

## Self-Check: PASSED

- All 10 created files and 9 modified files are present in `git diff --stat 039c915..HEAD`.
- Task commits `52635fe`, `e66317e`, `764f47e`, `719ba3b`, `50c2631` are present in `git log`.
- All task acceptance criteria and the plan-level verification commands were re-run and pass (see Verification Run).
