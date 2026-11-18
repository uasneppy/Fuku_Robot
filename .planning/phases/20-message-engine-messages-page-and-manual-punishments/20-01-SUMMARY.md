---
phase: 20-message-engine-messages-page-and-manual-punishments
plan: 01
subsystem: testing
status: complete
tags: [golden, characterization, moderation, bans, mutes, MSG-11]
requires: []
provides:
  - TestLegacyModerationRequests (17 subtests)
  - goldenModeration* constants (14, one per replying case)
  - moderationGoldenTarget, moderationTargetClient, newModerationGoldenSite, withModerationGoldenLocales
affects: [20-04, 20-05, 20-11]
tech-stack:
  added: []
  patterns: [golden request JSON with the <chat> token, scripted client wrapper answering getChat of the target]
key-files:
  created:
    - fuku/modules/moderation_legacy_request_test.go
  modified: []
decisions:
  - "The real locales are installed once per top-level test (withModerationGoldenLocales); newModerationGoldenSite installs them itself only when no enclosing test did, because loading the eight locale files costs about one second per site under -race."
requirements: [MSG-11]
metrics:
  duration: ~20 min
  completed: 2026-11-18
actuals:
  tokens: 4300
  tasks: 2
  commits: 1
plan_head_before: 8e929a9
plan_head_after: 31e7e18
---

# Phase 20 Plan 01: Manual punishment reply goldens Summary

Characterization golden of every success reply of the ten manual punishment commands, recorded on
the unchanged code and committed alone, so later plans can prove a group with nothing stored keeps
byte-identical replies.

## What was built

`fuku/modules/moderation_legacy_request_test.go` (`//go:build testtools`), committed alone in
`31e7e18`:

- `TestLegacyModerationRequests`, one subtest per case: `ban_reason`, `ban_no_reason`, `ban_channel`,
  `dban`, `tban`, `kick_reason`, `dkick`, `mute_reason`, `dmute`, `tmute`, `unban`, `unban_channel`,
  `unmute`, `unmute_lockdown`, `sban`, `skick`, `smute`. Each case calls the module handler directly
  with `s.commandCtx(...)`, expects `ext.EndGroups`, asserts the punishment call by method and
  `user_id` (or `sender_chat_id` for the channel cases), for `dban` the `deleteMessage` of 303, and:
  - a replying case: exactly one `sendMessage`, whose `legacyRequestJSON` with the chat ID replaced
    by `<chat>` equals its golden byte for byte, and exactly one `bot:punish_manual` row for message
    9001 due 600 s after the fixed clock (`wantRow`);
  - a silent case (`/sban 42`, `/skick` by reply, `/smute 42`): no `sendMessage` and no queue row.
- The 14 golden constants `goldenModerationBanReason` ... `goldenModerationUnmuteLockdown`.
- Toolkit for later plans: `moderationGoldenTarget` (ID 42, `Ann & <Bo>`, `Lee`, `ann_lee`, as a
  private chat), `moderationTargetClient` (embeds `*moduleBotClient`, records every call, answers
  `getChat` of chat 42 with the target, delegates everything else), `newModerationGoldenSite(t)` (a
  `botSite` in a fresh supergroup with the wrapped client, real locales, a 600 s
  `bot:punish_manual` rule and `botSiteWarnsCleanup`), and `withModerationGoldenLocales(t)`.
- The lockdown case swaps `lockdownSnapshotLookup` with `mreqSwap` to return an active
  `models.ChatLockdown` with a valid `PrePermissions` JSON.

What the goldens show about today's code (useful for plans 20-04 and 20-05):

- `/ban`, `/dban`, `/mute`, `/dmute` send the Unban or Unmute button row
  (`unrestrict|v1|a=unban&u=42`, `...a=unmute&u=42`), `reply_parameters` `{"message_id":101}` with no
  `allow_sending_without_reply`, and no `link_preview_options`.
- `/tban`, `/tmute`, `/kick`, `/dkick`, `/unban`, `/unmute` and both channel replies use
  `formatting.Shtml()`: `link_preview_options.is_disabled`, `allow_sending_without_reply`, no keyboard.
- The mention escapes the name (`Ann &amp; &lt;Bo&gt;`) and uses the first name only; the reason line
  is escaped (`spam &lt;b&gt;`); `/tban` and `/tmute` show the duration in English (`2 hours`,
  `30 minutes`) inside the Ukrainian text.
- The channel replies name the channel by the replied post's sender chat title (`Spam Channel`).

## Verification

```
$ go test -tags testtools -race -count=1 -run '^TestLegacyModerationRequests$' -v ./fuku/modules
--- PASS: TestLegacyModerationRequests (1.18s)
    --- PASS: TestLegacyModerationRequests/ban_reason ... /smute   (17 subtests, all PASS)
ok  	github.com/uasneppy/fuku_robot/fuku/modules	2.287s

$ go test -tags testtools -race -count=1 -run '^(TestLegacyModerationRequests|TestBotManualPunishmentReplies)$' ./fuku/modules
ok  	github.com/uasneppy/fuku_robot/fuku/modules	2.845s
```

- `gofmt -l` on the file: empty. `go vet -tags testtools ./fuku/modules`: clean.
- `CGO_ENABLED=0 go build ./...`: OK.
- `git diff --exit-code c491b48 -- fuku/modules/moderation.go fuku/modules/bans.go fuku/modules/mute.go go.mod go.sum`: exit 0.
- `git show --stat HEAD` (the golden commit): one file. No commit since c491b48 touches the three send-site files.
- `grep -c '<chat>'` on the file: 16 (14 constants plus two comments).

## Commits

- `31e7e18` test(20-01): pin the manual punishment replies before the event messages

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking/performance] Real locales installed once per test, not once per site**
- **Found during:** Task 1
- **Issue:** `withRealLocales` takes about 1.1 s under `-race`; called from every site it made the
  test 19 s, while AGENTS.md says `fuku/modules` is near its CI time limit and new tests must stay light.
- **Fix:** `withModerationGoldenLocales(t)` installs the locales once in the parent test and sets a
  package counter; `newModerationGoldenSite(t)` still installs them itself when no enclosing test did,
  so its contract for later plans is unchanged. The test now runs in about 1.2 s.
- **Files modified:** fuku/modules/moderation_legacy_request_test.go
- **Commit:** 31e7e18

No production code was changed.

## Notes for later plans

- Reuse `newModerationGoldenSite(t)`; in a test with many subtests call `withModerationGoldenLocales(t)`
  in the parent first to avoid one second per site.
- `moderationGoldenCases()` holds the case table (command text, flags, handler, punishment method);
  later plans can range over it to compare a default reply with `tc.golden`.
- Never re-record or edit this file; compare against commit `31e7e18`.

## Self-Check: PASSED

- FOUND: fuku/modules/moderation_legacy_request_test.go
- FOUND: 31e7e18 (ancestor of HEAD)
