---
phase: 20-message-engine-messages-page-and-manual-punishments
plan: 05
subsystem: moderation
status: complete
tags: [event-messages, moderation, anonymous-admin, EVT-01, MSG-03, MSG-09, MSG-11]
requires: [20-01, 20-04]
provides:
  - kickDefaultReply, tbanDefaultReply, unbanDefaultReply (bans.go)
  - muteDefaultReply, tmuteDefaultReply, unmuteDefaultReply (mute.go)
  - eventUntil (moderation.go)
  - the anonymous mark of the proof tap (bot_updates.go)
  - TestEventMessageSites, TestEventMessageSilentVariants, TestEventMessageAfterAnonymousProof
affects: [20-06, 20-07, 20-08, 20-11, 21]
tech-stack:
  added: []
  patterns: ["default built at the site by an eventDefaultReply, handed to replyEvent"]
key-files:
  created:
    - fuku/modules/event_message_sites_test.go
  modified:
    - fuku/modules/bans.go
    - fuku/modules/mute.go
    - fuku/modules/moderation.go
    - fuku/modules/bot_updates.go
    - fuku/modules/event_message.go
    - AGENTS.md
decisions:
  - "eventUntil(untilDate) turns a timed punishment's until_date into the {until} value, and a zero or negative one into the zero time, so {until} is empty rather than 1970 for a permanent punishment."
  - "moderationReasonSuffix lost its last caller (every site now builds its default with moderationReasonLine through a builder) and was removed rather than kept as dead code; its doc moved to moderationReasonLine."
  - "The unmute default builder returns the text without the lockdown note; the site appends \"\\n\" + note as before and passes the same note in eventReply.Note."
metrics:
  duration: ~25 min
  completed: 2026-11-19
actuals:
  tokens: 8800
  tasks: 3
  commits: 3
plan_head_before: a0230f00b0b36cfd6479ad8e080988ae78bcf63b
plan_head_after: 53adedba32992dcfa96dc592c8585fdb48ed76eb
---

# Phase 20 Plan 05: The six other manual events and the anonymous mark Summary

All seven manual punishment events of a group now post the group's stored message: `/tban`, `/kick`,
`/dkick`, `/unban`, `/mute`, `/dmute`, `/tmute` and `/unmute` reply through `replyEvent` exactly as
`/ban` does, `/tban` and `/tmute` fill `{duration}` and `{until}`, a custom `/unmute` text keeps the
lockdown note, and an anonymous admin stays "an admin" after the proof tap; a group with nothing
stored sends every golden reply of plan 20-01 byte for byte.

## Tasks

| Task | Name | Commit |
|------|------|--------|
| 1 | /tban, /kick, /dkick and /unban post their stored event message | 7e48d23 |
| 2 | /mute, /dmute, /tmute and /unmute post their stored event message, the lockdown note included | 1f4a8b5 |
| 3 | the anonymous mark on the proof tap, the silent variants, AGENTS.md | 53adedb |

## Event per command

| Command | Event | Values |
|---------|-------|--------|
| `/tban` | `tban` | reason, `{duration}` = `t.timeVal`, `{until}` = `eventUntil(t.untilDate)` |
| `/kick`, `/dkick` | `kick` (both through `kickReply`) | reason |
| `/unban` of a member | `unban` | none |
| `/mute`, `/dmute` | `mute` (both through `muteReplyWithButton`) | reason |
| `/tmute` | `tmute` | reason, `{duration}`, `{until}` |
| `/unmute` | `unmute` | none; `Note` = the lockdown note during an active lockdown |

The channel `/ban` and `/unban` keep `replySuccess` (`grep -c 'c.replySuccess(' bans.go` = 2), the
silent variants are untouched (`moderationSmute` not in the diff).

## Verification

- Task 1: `go test -tags testtools -race -count=1 -run '^(TestEventMessageSites|TestLegacyModerationRequests|TestBotManualPunishmentReplies)$' -v ./fuku/modules`: PASS.
- Task 2: the same plus `TestUnmuteDuringLockdownUsesSnapshot`, `TestUnmuteLockdownLookupFails` and `TestResolveUnmutePermissionsLockdown` (the plan's `TestLockdownUnmute.*` pattern names no test; these are the lockdown unmute tests): PASS.
- Task 3: `-run '^(TestEventMessageAfterAnonymousProof|TestEventMessageSilentVariants|TestEventMessageSites|TestLegacyModerationRequests|TestVerifyAnonymousAdmin.*|TestBotManualPunishmentReplies|TestEventMessage.*|TestRenderEventText|TestBotManualRepliesNotCovered|TestUnmuteDuringLockdownUsesSnapshot|TestUnmuteLockdownLookupFails)$' -v`: PASS, no SKIP.
- Wider: every test of `bans*`, `mute*`, `mutes*`, `moderation*`, `lockdown_unmute` and `deletion_bot*` test files, and the anonymous-admin families (`TestAnonAdmin.*`, `TestAnonymousAdminBanAfterProof`, `TestCommandDeletionAnonymousAdmin`, `TestLockCommandsAnonymousAdmin`, `TestLockdownAnonymousAdminProof`, `TestSettingsAnonymousAdmin`, `TestSettingsAnonymousOwnerGate`): ok.
- Mutation check: with the `markAnonymousActor(ctx)` call removed, `TestEventMessageAfterAnonymousProof` fails ("by Telegram"); restored before the commit.
- `git diff --exit-code 31e7e18 -- fuku/modules/moderation_legacy_request_test.go` exits 0: the golden is unedited and passes.
- Acceptance greps: builders 3 in bans.go and 3 in mute.go; `replyEvent(eventReply{` 4 in bans.go and 3 in mute.go; `c.replySuccess(` 2 in bans.go and 0 in mute.go; `markAnonymousActor(ctx)` 1 in bot_updates.go; `tmuteDefaultReply` in AGENTS.md 1.
- `CGO_ENABLED=0 go build ./...` ok; `go vet -tags testtools ./fuku/modules` clean; `gofmt -l` clean on every touched file.
- `make lint` could not run: the installed golangci-lint 2.5.0 was built with Go 1.25 and refuses the module's Go 1.26.0 target. `go vet` and `gofmt` were run instead, and every new production identifier has a production caller.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] `moderationReasonSuffix` removed**
- **Found during:** Task 2
- **Issue:** once the mute sites built their defaults through builders, `moderationReasonSuffix` had no caller left; the `unused` linter (run in CI without the test tag) would fail on it.
- **Fix:** removed it; its documentation moved to `moderationReasonLine`, which every builder calls. AGENTS.md says so.
- **Files modified:** fuku/modules/moderation.go
- **Commit:** 1f4a8b5

**2. [Rule 2 - Correctness] `eventUntil` helper**
- **Found during:** Task 1
- **Issue:** the plan's `time.Unix(t.untilDate, 0)` renders 1970 for a zero until_date.
- **Fix:** `eventUntil` (moderation.go) returns the zero time for a non-positive until_date, so `{until}` is empty for a permanent punishment (D-09); `/tban` and `/tmute` use it.
- **Commit:** 7e48d23

**3. Verify pattern** the plan's `TestLockdownUnmute.*` matches no test; the real lockdown unmute tests were run by name instead (see Verification).

## nolint:unused

- Removed: `markAnonymousActor` in `fuku/modules/event_message.go` (its production caller is now `verifyAnonymousAdmin`).
- Added: none.

## For plans 20-06 onwards

- Every manual event key in `eventmsg.Events` now has a live send site; the page can store a message for any of the seven and it is posted.
- `/unmute`'s custom text gets the lockdown note through `eventReply.Note`; a preview of the unmute event on the page should not add it (it is a fact of the moment, not of the message).
- The anonymous mark is `ctx.Data[eventAnonActorKey]`, set only in `verifyAnonymousAdmin`.
- `TestEventMessageSites` is table-driven (`eventMsgSiteCases`, custom text `{first}|{reason}|{duration}|{until}|{admin}`); `eventMsgProofTap(t, s, text)` drives a real proof tap on a golden site; `eventMsgWantSiteGolden` compares a reply with any moderation golden. Each of the three new top-level tests loads the real locales once (about 1.5 s under `-race`).

## Known Stubs

None.

## Threat Flags

None: T-20-14 mitigated (`TestEventMessageAfterAnonymousProof`), T-20-16 mitigated (the goldens and the "nothing stored" rows of `TestEventMessageSites`).

## Self-Check: PASSED

- FOUND: fuku/modules/event_message_sites_test.go
- FOUND: 7e48d23, 1f4a8b5, 53adedb (ancestors of HEAD)
