---
phase: 16-scheduled-deletion-of-service-and-bot-messages
plan: 05
subsystem: modules
tags: [deletion, bot-messages, welcome, goodbye, rules, punishment-replies, warn-source, tracer]

requires:
  - phase: 16-scheduled-deletion-of-service-and-bot-messages
    provides: "plan 16-02: deletionRulesLookup, the kind constants; plan 16-03: deletionSchedule, deletionSentAt, deletionQueueWrite, the latch, deletionEnv; plan 16-04: the worker that carries out the rows this plan writes"
provides:
  - "scheduleBotMessage(sent, kind) and its shared body scheduleBotRow (optional delay override for plan 16-06)"
  - "hooks at the named sites: SendWelcomeMessage (bot:welcome), leftMember (bot:goodbye), /rules and the click-to-read message (bot:rules), moderationCtx.replySuccess at five sites of bans.go and three of mute.go (bot:punish_manual), warnThisUser by source (bot:punish_manual or bot:punish_auto)"
  - "sendRulesMessage returns the sent message; warnThisUser takes a warnSource"
  - "staffActionFake.sendDate seam (default 1) set from deletionNow() by the deletion environment"
affects: [16-06, 16-07, 16-08, 16-09]

plan_head_before: 9b0623a9602a18c71d65807d4aca00bf44e1aa48
plan_head_after: dc0fa5cb82390b5266ef65717877b83be6ddae04

actuals:
  tokens: 12100   # chars/4 over the added lines of 9b0623a..dc0fa5c (48401 chars), tests and AGENTS.md included
  tasks: 3
  commits: 6      # MEASURED: git rev-list --count 9b0623a..HEAD (before this SUMMARY commit)

tech-stack:
  added: []
  patterns:
    - "one opt-in helper with no result that reads the chat from the message Telegram answered, so a private chat or channel is refused by type and every failure is one Warn"
    - "a shared body (scheduleBotRow) that takes an optional delay override, so later callers add a variant without a second copy"
    - "site tests on a scripted moduleBotClient whose send answers carry the chat's own ID, type and the fixed clock's date (botSite)"

key-files:
  created:
    - fuku/modules/deletion_bot.go
    - fuku/modules/deletion_bot_test.go
    - fuku/modules/deletion_bot_sites_test.go
  modified:
    - fuku/modules/greetings.go
    - fuku/modules/rules.go
    - fuku/modules/settings_edit_preview.go
    - fuku/modules/moderation.go
    - fuku/modules/bans.go
    - fuku/modules/mute.go
    - fuku/modules/warns.go
    - fuku/modules/blacklists.go
    - fuku/modules/warns_command_test.go
    - fuku/modules/staff_action_fake_test.go
    - fuku/modules/deletion_env_test.go
    - AGENTS.md

key-decisions:
  - "The helper takes the chat ID and type from the message Telegram answered (sent.Chat), as the plan's signature says; the site tests therefore script their send answers with the chat's own ID and type"
  - "A /rules from a private chat connected to a group is refused by the helper's type check only (the message goes to the private chat), and TestBotRulesPrivateNeverScheduled drives that path with a real connection row"
  - "scheduleBotRow takes the base time (not a date) and an optional *int delay override; its override branch has no production caller yet and is exercised by TestScheduleBotMessage (plan 16-06 is its user)"
  - "The rules read in the helper treats a nil answer with no error as no rule, so no input can panic"

patterns-established:
  - "newBotSite(t, chatType) plus s.commandCtx / s.wantRow / s.wantNoRows for a command-handler site test with a queue assertion"
  - "env.joinMember / env.leaveMember deliver a real chat_member update to the deletion environment's dispatcher"

requirements-completed: []

coverage:
  - id: D1
    description: "the welcome sent for a join through the real dispatcher is queued at its own send time plus the stored delay, and never with the kind off or in a Staff Group"
    requirement: "DEL-02"
    verification:
      - kind: integration
        ref: "TestBotWelcomeScheduled (mutation-checked: removing the hook fails it)"
        status: pass
  - id: D2
    description: "the helper refuses nil, private, channel and untyped chats, no rule, a gone latch, a Staff Group, a failed read and a failed write without an error; the date clamp never stretches a message"
    requirement: "DEL-07"
    verification:
      - kind: integration
        ref: "TestScheduleBotMessage, TestScheduleBotMessageClampsDate"
        status: pass
  - id: D3
    description: "the goodbye and the group /rules and click-to-read messages are queued; the private rules message, the deep link, a connected private /rules and the settings preview are not, and the Phase 12 goldens pass unedited"
    requirement: "DEL-02"
    verification:
      - kind: integration
        ref: "TestBotGoodbyeScheduled, TestBotRulesScheduled, TestBotRulesPrivateNeverScheduled, TestRulesLegacyRowSendsTodaysRequest, TestLegacyGreetingsRequestsUnchanged"
        status: pass
  - id: D4
    description: "the success reply of each of fourteen manual punishment commands is queued as bot:punish_manual (also after the anonymous admin's proof), and no error reply, list, card, /kickme, /banme or silent variant is"
    requirement: "DEL-02"
    verification:
      - kind: integration
        ref: "TestBotManualPunishmentReplies (42 subtests; mutation-checked: dropping the schedule call fails 21), TestBotManualRepliesNotCovered (21 cases)"
        status: pass
  - id: D5
    description: "the manual /warn and the blacklist's warn are told apart, the warn-limit text is scheduled too, and a rule for the other kind does not apply"
    requirement: "DEL-02"
    verification:
      - kind: integration
        ref: "TestWarnThisUserSource (mutation-checked: always using the manual kind fails the four automatic cases)"
        status: pass
---

# Phase 16 Plan 05: Scheduling the bot's own messages Summary

**The bot's welcome, goodbye, group rules message and the success replies of its manual punishment commands are queued for deletion after the delay stored for their kind, in groups only, through one opt-in helper called at named sites; the blacklist's automatic warn is told apart from a manual `/warn`.**

The first task is a tracer: a welcome sent for a join through the real dispatcher became one queue row at its own send time plus 60 s (and none with the kind off or in a Staff Group) before any other site was touched. Tasks 2 and 3 extend that proven path.

## Performance

- **Duration:** about 25 minutes by the commit clock (01:03 to 01:25 UTC), reading the earlier plans and the send sites first.
- **Tasks:** 3 of 3 (task 1 is the tracer)
- **Files:** 15 (3 new, 12 edited)

## Accomplishments

- `fuku/modules/deletion_bot.go`: `scheduleBotMessage(sent, kind)` (nil message and any chat type but `group` and `supergroup` return at once) over `scheduleBotRow` (cached rules read through `deletionRulesLookup`, optional delay override, `deletionSchedule` under a 3 s context, every failure one Warn with chat ID and kind).
- Welcome (`SendWelcomeMessage`) and goodbye (`leftMember`) schedule right after the send's error check and before the clean-old block.
- `sendRulesMessage` returns `(*gotgbot.Message, error)` with every request unchanged; `/rules` and the click-to-read message schedule `bot:rules`; the deep link and the settings preview discard the message.
- `moderationCtx.replySuccess` at the eight success sites (five in `bans.go`, three in `mute.go`); `warnThisUser` gains `source warnSource` (`warnSourceManual`, `warnSourceAuto`) with callers in `warns.go`, `blacklists.go` and `warns_command_test.go`.
- `staffActionFake.sendDate` (nil means 1) used by the `sendMessage` and edit answers; the deletion environment sets it from `deletionNow()`.
- AGENTS.md: a "Scheduling the bot's own messages" Subsystem traps bullet and a Testing bullet for the plan's families.

## Task Commits

1. Task 1 RED `7cf2249` test(16-05): a welcome is queued by its kind (the helper does not exist in this commit)
2. Task 1 GREEN (tracer) `0c08de1` feat(16-05): queue the welcome by its stored delay
3. Task 2 RED `e3729bc` test(16-05): the goodbye and the group rules message are queued
4. Task 2 GREEN `7ba29b8` feat(16-05): queue the goodbye and the group rules message
5. Task 3 RED `83393c1` test(16-05): manual punishment replies are queued, errors and cards are not (does not compile: no warn source yet)
6. Task 3 GREEN `dc0fa5c` feat(16-05): queue the success replies of manual punishments (AGENTS.md in the same commit)

## Verification (real results)

All run from /home/user/Fuku_Robot on SQLite (the `fuku/modules` harness does not run on PostgreSQL).

| Gate | Result |
|------|--------|
| Task 1 verify `-run '^(TestScheduleBotMessage\|TestScheduleBotMessageClampsDate\|TestBotWelcomeScheduled)$' -v` | PASS, 0 SKIP |
| Task 1 neighbours `-run '^(TestGreetOnce\|TestLegacyGreetingsRequestsUnchanged\|TestGreetingsMediaBelow\|TestSendWelcome\|TestStaffAction)'` | ok (78.5 s) |
| Task 2 verify `-run '^(TestBotGoodbyeScheduled\|TestBotRulesScheduled\|TestBotRulesPrivateNeverScheduled\|TestBotWelcomeScheduled)$' -v` | all PASS, 0 SKIP |
| Task 2 neighbours `-run '^(TestRules\|TestRulesLegacyRowSendsTodaysRequest\|TestLegacyGreetingsRequestsUnchanged\|TestGreeting\|TestSettingsEditPreview\|TestSettingsEditFullPreview\|TestPrivateRules\|TestSendRules\|TestSetRules)'` | ok (16.2 s) |
| Task 3 verify (the nine new tests plus Welcome, Goodbye, Rules) `-v` | all PASS, 0 SKIP |
| Task 3 neighbours `-run '^(TestBan\|TestKick\|TestMute\|TestUnmute\|TestWarn\|TestApplyWarn\|TestBlacklist\|TestModeration\|TestTban\|TestTmute\|TestDban\|TestDmute\|TestMigratedDescriptors\|TestTemporary\|TestSilent\|TestDelete)'` | ok |
| Plan verification `-run '^(TestScheduleBot\|TestBot\|TestWarnThisUserSource\|TestRules\|TestGreet\|TestLegacyGreetings\|TestWarn\|TestBan\|TestMute\|TestBlacklist\|TestDeletion\|TestServiceWatcher\|TestCleanService\|TestSendRules\|TestPrivateRules\|TestSetRules)' ./fuku/modules` | ok (43.1 s) |
| The nine new tests, `-count=3` | ok |
| `go vet -tags testtools ./fuku/modules`, `CGO_ENABLED=0 go build ./...` | clean |
| `gofmt -l` on every file touched | nothing |
| `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827` (exit read directly) | exit 0, "no dupl, gocyclo or godox findings" |
| `grep -n 'reflect.Ptr\|ioutil\.' fuku/modules/deletion_bot*.go` | nothing |
| `git diff --exit-code 8c0c827 -- go.mod go.sum` | exit 0 |
| `git diff --exit-code dd61b2c -- fuku/modules/rules_legacy_request_test.go` | exit 0 |
| Mutation checks | removing the welcome hook fails `TestBotWelcomeScheduled`; replacing `source.deletionKind()` by the manual kind fails four cases of `TestWarnThisUserSource`; dropping the schedule call in `replySuccess` fails 21 subtests of `TestBotManualPunishmentReplies` |
| Acceptance greps | `func scheduleBotMessage(` 1; `scheduleBotMessage(` in greetings.go 2, in `fuku/utils` 0, in rules.go 2, in settings_edit_preview.go 0; `sendDate` in the fake 5; `sendRulesMessage` returning the message 1; `replySuccess(` bans.go 5, mute.go 3, moderation.go definition 1; `warnSourceAuto` in blacklists.go 1; `warnSourceManual` in warns.go 2 and in the test 1; "Scheduling the bot" in AGENTS.md 1 |

**Not run:** `make lint` (golangci-lint is built with Go 1.25; the stand-in ran), `make test`, the whole `fuku/modules` package, the settings test family (no settings page touched), `make check-translations` and `make check-docs` (no locale or help change), gosec, PostgreSQL (no migration, no PostgreSQL-only guarantee).

## Decisions Made

See `key-decisions`. One consequence worth knowing for plan 16-06: the helper reads the chat from the message Telegram answered. In production that is the chat the bot sent to; in a test with a scripted client it must be scripted (the shared `moduleBotClient` answers chat `-1001` for every send, so the site tests set the answer per chat in `botSite.answerAs`).

## Deviations from Plan

**1. [Plan text] The "private chat" case of `TestBotWelcomeScheduled` is covered in `TestScheduleBotMessage`, not through the dispatcher.** A welcome is only sent into a group by the join paths, and the shared staff fake answers every send with a supergroup chat, so a private chat cannot be reached through the real dispatcher. The helper's refusal of a private chat is proved by the table in `TestScheduleBotMessage` (private, channel, empty type) and, for a real private-chat send, by the connected `/rules` case of `TestBotRulesPrivateNeverScheduled`.

**2. [Plan text] `/rules` "from a private chat connected to a group" is driven with a real connection row.** The test connects the member (`connections.ConnectId`) so the command takes the connected path, asserts the group's rules text was sent to the private chat, and that no row is written for either chat. The helper's type check is the only defence there, as the plan says.

**3. [Process] Task 3's production code was written before its RED commit.** The tests were committed first as a separate commit (it does not compile: `warnSourceManual` does not exist in it), then the production commit followed; the mutation checks above show the tests discriminate.

**4. [Added] Extra coverage.** `TestScheduleBotMessage` also covers a nil rules answer and the delay override of `scheduleBotRow`; `TestBotManualPunishmentReplies` runs three variants per command (kind on, kind off with another kind's rule stored, anonymous re-entry through the registered handler after `LoadBans`, `LoadMutes` and `LoadWarns`); `TestWarnThisUserSource` adds the warn-limit text and a rule for the other kind.

**5. [Test correction] A no-reply case of `TestBotManualRepliesNotCovered`.** My first "warn of an admin" case sent no message for the reasons of the extraction path; it was replaced by "warn without a target" (one reply, no row) before the green commit. Nothing outside this plan's files changed.

No other deviations: every plan gate text was valid as written.

## Known Stubs

None. The delay-override branch of `scheduleBotRow` has no production caller yet (plan 16-06's mute notice and its reschedule after "I fixed it" are its users); it is exercised by `TestScheduleBotMessage` and the lint stand-in reports no unused finding.

## Threat Flags

None beyond the plan's register: T-16-22 (named sites only, no hook in the shared send helpers, `grep -rn 'scheduleBotMessage(' fuku/utils` finds nothing, private chats refused by type and Staff Groups by the queue writer), T-16-23 (the helper has no result, one Warn per failure, the Phase 12 request goldens pass unedited), T-16-24 (the date clamp), T-16-25 (the explicit `warnSource` at both callers), T-16-SC (`go.mod` and `go.sum` unchanged).

## Requirements

`requirements-completed: []`. DEL-02 and DEL-07 are contributed to and stay Pending; DEL-01, DEL-06, DEL-08 and MREQ-15 stay Pending too. The closing plan 16-09 completes them. PLAT-06 stays ticked as it was.

## Self-Check: PASSED

Created files exist (`deletion_bot.go`, `deletion_bot_test.go`, `deletion_bot_sites_test.go`) and commits `7cf2249`, `0c08de1`, `e3729bc`, `7ba29b8`, `83393c1`, `dc0fa5c` are ancestors of HEAD.
