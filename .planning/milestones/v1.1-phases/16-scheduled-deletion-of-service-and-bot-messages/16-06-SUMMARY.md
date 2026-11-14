---
phase: 16-scheduled-deletion-of-service-and-bot-messages
plan: 06
subsystem: modules
tags: [deletion, bot-messages, member-requirements, antiflood, blacklist, aispam, punish-auto, tracer]
status: complete

requires:
  - phase: 16-scheduled-deletion-of-service-and-bot-messages
    provides: "plan 16-05: scheduleBotMessage and its shared body scheduleBotRow (optional delay override), warnThisUser's source, the sendDate seam; plans 16-02/16-03: rules lookup, deletionSchedule, deletionNow, the queue's LEAST upsert"
provides:
  - "scheduleBotMessageCapped (a notice with a button the member needs, queued at the 24 hour cap whatever the timer says) and scheduleBotMessageFromNow (the stored delay counted from this moment)"
  - "memberReqScheduleNotice, called by memberReqPostNotice right after a successful send: every requirement notice and the per-chat cap's pause notice are bot:punish_auto, the mute notice at 86400 s"
  - "memberReqFixedEdit takes the chat type and reschedules the edited confirmation from the tap"
  - "hooks at the antiflood notice (checkFlood), the blacklist mute, ban and kick notices and the AI spam breaker's pause notice (aispamNotifyBreaker)"
affects: [16-07, 16-08, 16-09]

plan_head_before: 2e1cf1c4926c3b16eaa32dbdbce91d67c7af2f8c
plan_head_after: 929abdf81aa92404a50036fba37c454734ad32f6

actuals:
  tokens: 8000    # chars/4 over the added lines of 2e1cf1c..929abdf (32036 chars), tests and AGENTS.md included
  tasks: 3
  commits: 7      # MEASURED: git rev-list --count 2e1cf1c..HEAD (before this SUMMARY commit)

tech-stack:
  added: []
  patterns:
    - "thin variant callers of one shared body: the cap variant and the from-now variant add no second copy of the rules read, the refusals or the error swallowing"
    - "a clock-controlled member requirements environment (mreqSchedEnv: deletionNow and the fake's sendDate follow one settable instant) so a tap 20 minutes later is testable"
    - "a table of named notice sites (autoNoticeSite) run once per case family: queued, kind off, other kind, private or channel answer, failed send, helper nil"

key-files:
  created:
    - fuku/modules/memberreq_notice_schedule_test.go
    - fuku/modules/deletion_bot_auto_test.go
  modified:
    - fuku/modules/deletion_bot.go
    - fuku/modules/memberreq_notice.go
    - fuku/modules/memberreq_fixed.go
    - fuku/modules/antiflood.go
    - fuku/modules/blacklists.go
    - fuku/modules/aispam.go
    - AGENTS.md

key-decisions:
  - "scheduleBotRow now reads the rules first on every path, override included: a kind with no stored rule is Off and writes nothing, so the 24 hour cap override can never create a row for a chat whose owner left the kind off (research C-07 point 1). The override replaces a stored delay of any value, 0 included"
  - "memberReqScheduleNotice (a tiny wrapper in memberreq_notice.go) picks the cap variant when the notice carries the button and scheduleBotMessage otherwise, so memberReqPostNotice gains one call and its retry loop, pacing, return value and requests are untouched"
  - "The reschedule runs only on the successful edit path of memberReqFixedEdit; a failed edit returns before it, so the 24 hour row stays"
  - "The AI spam breaker's pause notice counts as an automatic punishment message (D-27); aispamMirrorDeletion (owner-log dump) is untouched"

patterns-established:
  - "newMreqSchedEnv(t) plus env.setRule / wantRowFor / wantNoRows / advance for a notice test with a queue assertion"

requirements-completed: []

coverage:
  - id: D1
    description: "a warn, kick, ban or mute notice sent by the real executor, and the cap's pause notice, are queued as bot:punish_auto from the date of the message that went out (600 s for a notice with no button), and nothing is queued with the kind off"
    requirement: "MREQ-15"
    verification:
      - kind: integration
        ref: "TestMemberReqNoticeScheduled (mutation-checked: dropping the cap fails the mute case)"
        status: pass
  - id: D2
    description: "a mute notice with its I fixed it button is queued at 86400 s whether the timer is 60 s, 600 s, 86400 s or a stored 0, writes no row with the kind off or when the send failed, and counts from the date of the message that finally went out after a retry"
    requirement: "MREQ-15"
    verification:
      - kind: integration
        ref: "TestMemberReqMuteNoticeAtCap (mutation-checked: scheduling at the timer instead of the cap fails five subtests)"
        status: pass
  - id: D3
    description: "the tap moves the 24 hour row up to the timer counted from the tap (20 minutes after a 10 minute timer), a longer timer changes nothing, the kind off or turned off schedules nothing, a failed edit, a refused tap and a still-broken member leave the 24 hour row"
    requirement: "MREQ-15"
    verification:
      - kind: integration
        ref: "TestMemberReqFixedRescheduleMovesUp (mutation-checked: shifting the base fails the first two subtests)"
        status: pass
  - id: D4
    description: "the antiflood notice (mute, ban, kick), the blacklist mute, ban, kick and warn notices and the AI spam breaker's pause notice are queued as bot:punish_auto at their sites; kind off, the manual kind only, a private or channel answer, a failed send and a helper that answers no message write nothing; a delete verdict and the owner-log dump write nothing"
    requirement: "DEL-02"
    verification:
      - kind: integration
        ref: "TestBotAutoPunishmentNotices (mutation-checked: removing the blacklist mute and aispam hooks fails their cases)"
        status: pass
---

# Phase 16 Plan 06: Automatic punishment notices Summary

**Every member requirement notice (the pause notice included), the antiflood notice, the blacklist's mute, ban and kick notices and the AI spam breaker's pause notice are queued for deletion by the automatic-punishments timer, a mute notice is held up to a day at the 24 hour cap for the member's "I fixed it" button, and the tap reschedules the confirmation from that moment.**

## Interrupted execution, who did what

The first executor of this plan was killed by a container restart after it had made all seven task commits and before it wrote this SUMMARY, updated STATE.md or ROADMAP.md. A second sequential executor finished it. It did not redo or rewrite anything: it re-read the diff `2e1cf1c..HEAD` against every task of the plan, re-ran every verify command and acceptance grep, ran mutation checks, found no gap, and wrote this SUMMARY and the STATE and ROADMAP updates.

- Commits 1 to 7 below are the first executor's. None are the second executor's except the SUMMARY commit itself and the STATE/ROADMAP bookkeeping.

## Task commits

1. Task 1 RED `0ab233c` test(16-06): requirement notices are queued by the automatic-punishments timer (does not compile until the cap variant exists)
2. Task 1 GREEN (tracer) `36c3210` feat(16-06): queue requirement notices by the automatic-punishments timer
3. Task 2 RED `6d21896` test(16-06): the fixed notice follows the timer from the tap
4. Task 2 GREEN `93567fe` feat(16-06): reschedule the fixed notice from the tap
5. Task 3 RED `215009e` test(16-06): the other automatic notices are queued
6. Task 3 GREEN `27b0f93` feat(16-06): queue the antiflood, blacklist and AI spam notices (AGENTS.md in the same commit)
7. Chore `929abdf` chore(16-06): keep the aispam dump function name out of the hook comment (restores the plan's `grep -c aispamMirrorDeletion` invariant of 2)

## Accomplishments

- `deletion_bot.go`: `scheduleBotRow` reads the rules first on every path and returns for a kind with no rule, so the override only replaces a stored delay; `deletionIsGroupType`; `scheduleBotMessageCapped` (override 86400, `deletion.MaxDelaySecs`); `scheduleBotMessageFromNow(chatID, chatType, messageID, kind)` (base `deletionNow().UTC()`).
- `memberreq_notice.go`: `memberReqScheduleNotice(sent, withButton)` called at the successful-send return of `memberReqPostNotice` (16 lines added, none removed), so `memberReqAnnounce` and the cap's pause notice are both covered.
- `memberreq_fixed.go`: `memberReqFixedEdit` gains `chatType` (its one caller passes `chat.Type`) and calls `scheduleBotMessageFromNow(..., deletion.KindBotAuto)` only after a successful edit.
- `antiflood.go`, `blacklists.go` (three sites), `aispam.go`: the sent message is kept where it was discarded and scheduled after the error check; no request, error path or return changed. `aispamMirrorDeletion` and the warn action (plan 16-05's wiring) are untouched.
- AGENTS.md: the Subsystem traps bullet "Scheduling the bot's own messages" lists the automatic sites, the cap, the reschedule base and the Off rule, and describes the shared body's new always-read-rules behaviour; the member requirements executor bullet states that `memberReqPostNotice` queues the notice and the "I fixed it" edit reschedules it.

## Verification (real results)

All run from /home/user/Fuku_Robot on SQLite (the `fuku/modules` harness does not run on PostgreSQL).

| Gate | Result |
|------|--------|
| Plan verify, 5 named tests `-run '^(TestBotAutoPunishmentNotices\|TestWarnThisUserSource\|TestMemberReqNoticeScheduled\|TestMemberReqMuteNoticeAtCap\|TestMemberReqFixedRescheduleMovesUp)$' -v` | all PASS, 0 SKIP (15.8 s) |
| Task 2 family `-run '^(TestMemberReqFixed\|TestMemberReqChannelInert\|TestSettingsMreqChannelRefusal)'` | ok (78.2 s) |
| Task 3 family `-run '^(TestAntiflood\|TestBlacklist\|TestAISpam\|TestAispam\|TestCheckFlood\|TestSetFlood)'` | ok (13.2 s) |
| Plan broad run `-run '^(TestMemberReq\|TestBotAutoPunishmentNotices\|TestAntiflood\|TestBlacklist\|TestAISpam\|TestAispam\|TestScheduleBot\|TestBot\|TestWarnThisUserSource\|TestDeletion)' ./fuku/modules` (re-run by the second executor in the background after the restart lost the first one) | ok, exit 0, 456.4 s |
| `go vet -tags testtools ./fuku/modules` | exit 0 |
| `CGO_ENABLED=0 go build ./...` (output to /dev/null) | exit 0 |
| `gofmt -l` on the 8 touched Go files | no output |
| `python3 .../lint-local.py 8c0c827` (exit read directly) | exit 0, "no dupl, gocyclo or godox findings" |
| `git diff --exit-code 8c0c827 -- go.mod go.sum` | exit 0 |
| Legacy-spelling grep over the added lines (`reflect.Ptr`, `ioutil.`, `strings.Title`, `interface{}`, `t.Skip`, `nolint`, TODO, FIXME) | no match |

Acceptance greps at HEAD:

| Check | Result |
|-------|--------|
| T1: non-comment `scheduleBotMessage` / `deletion.KindBotAuto` in memberreq_notice.go | 2 / 2 (baseline 0) |
| T1: `git diff --numstat dd61b2c -- memberreq_notice.go` removed-lines awk gate | exit 0 (16 added, 0 removed) |
| T2: `func scheduleBotMessageFromNow(` in deletion_bot.go / non-comment call in memberreq_fixed.go | 1 / 1 |
| T2: `git diff --exit-code dd61b2c -- memberreq_fixed_test.go` | exit 0 |
| T3: antiflood.go and aispam.go each have at least one `scheduleBotMessage(`; blacklists.go | no MISSING; 3 |
| T3: `grep -c aispamMirrorDeletion aispam.go` | 2 |
| T3: `grep -c 'bot:punish_auto' AGENTS.md` | 3 (baseline 0) |

Mutation checks (the second executor, each reverted, tree clean afterwards):

- Drop the 24 hour cap (cap variant passes `nil` override): `TestMemberReqNoticeScheduled/mute`, four `TestMemberReqMuteNoticeAtCap` subtests and `TestMemberReqFixedRescheduleMovesUp` fail.
- Shift the reschedule's base by minus one hour: the first two `TestMemberReqFixedRescheduleMovesUp` subtests fail.
- Remove the blacklist mute hook and the aispam breaker hook: `TestBotAutoPunishmentNotices/blacklist mute` and `/aispam breaker pause` fail.

## Deviations from Plan

None in behaviour. Two notes:

1. `scheduleBotRow` was changed from plan 16-05's shape (the override branch skipped the rules read) to read the rules first on every path. The plan's assumption section requires exactly this ("writes nothing when the kind has none", research C-07 point 1); the 16-05 SUMMARY already named the override as having no production caller. `TestScheduleBotMessage` of 16-05 passes unedited.
2. The plan's T3 invariant `grep -c 'aispamMirrorDeletion' aispam.go` prints 2 only because commit `929abdf` removed the function name from the hook's comment (the first draft had made it 3); this is the first executor's own fix and the gate text is correct as written.

No plan gate text was broken. The Phase 13 to 15 member requirements families and the antiflood, blacklist and aispam families pass unedited; no existing test file was edited.

## Auth gates

None.

## Known Stubs

None.

## Threat Flags

None. No new endpoint, auth path, file access or schema; every hook is additive around an existing send and logs nothing new (the helper logs chat ID and kind only).

## Requirements

`requirements-completed: []`. DEL-01, DEL-02, DEL-06, DEL-07, DEL-08 and MREQ-15 stay Pending in REQUIREMENTS.md; only plan 16-09 completes them after its end-to-end test.

## Not run

`make test`, `make lint` (cannot run here: golangci-lint built with Go 1.25), the settings test family (the plan does not call for it) and the whole `fuku/modules` package; PostgreSQL proofs (the plan has no migration or PostgreSQL-only guarantee).

## Self-Check: PASSED

Files created exist (`fuku/modules/memberreq_notice_schedule_test.go`, `fuku/modules/deletion_bot_auto_test.go`), the seven commits are ancestors of HEAD (`git rev-list --count 2e1cf1c..HEAD` printed 7), and the working tree was clean after every mutation check.
