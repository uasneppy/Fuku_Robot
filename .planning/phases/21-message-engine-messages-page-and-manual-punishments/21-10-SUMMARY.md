---
phase: 20-message-engine-messages-page-and-manual-punishments
plan: 10
subsystem: settings menu / Messages page / safeguards
status: complete
tags: [settings, messages-page, event-messages, safeguards, privacy, MSG-05, MSG-06, MSG-08]
requires: [21-06, 21-07, 21-08, 21-09]
provides:
  - settingsMsgScreens (every Messages screen as pure renders, with the row widths each must have)
  - TestSettingsMessagesCallbackBytes and TestSettingsMessagesPageFitsEveryLocale
  - TestSettingsMessagesForged, TestSettingsMessagesReadsCreateNothing, TestSettingsMessagesCost, TestSettingsMessagesLogsCarryIDsOnly
  - settingsPrivacyPhase21Screens in TestSettingsStaffPrivacy
  - the Phase 21 test families bullet of AGENTS.md
affects: [21-11]
tech-stack:
  added: []
  patterns:
    - "a page's screens are built once by one pure helper (settingsMsgScreens) that the byte test, the length test and the privacy sweep all read, each screen carrying the row shape it must have"
    - "the forged-press table is built from one valid press per code, so a new action code adds one row and gets every forged variant"
key-files:
  created:
    - fuku/modules/settings_messages_screens_test.go
    - fuku/modules/settings_messages_safety_test.go
  modified:
    - fuku/modules/settings_staff_privacy_test.go
    - AGENTS.md
decisions:
  - "No production change: every safeguard passed against the code of plans 21-06 to 21-09, and every size and row shape the planner assumed was exact."
  - "A refused preview's 403 logs nothing by design (settingsEditPreviewAnswer: the presser never started the bot, the alert says so), so the log test proves the line of a non-403 refusal (a 400 on the content) names the chat and that no line of any path carries content; this is not a defect."
  - "The hub and family list are drawn from a hand-built eventmsg.Messages whose map key is the repository's own event|dest form; the helper looks each event up and fails loudly if that form ever changes."
  - "The group note is part of settingsMsgScreens only for a group view (its Open in private row exists only there); the other screens are drawn in both views so the privacy sweep searches them in both."
metrics:
  duration: ~30 min
  completed: 2026-11-19
actuals:
  tokens: 10300
  tasks: 3
  commits: 3
plan_head_before: 0d5de7e478c6252f53237eccaaf10e116420c3a1
plan_head_after: 540a2a957e4da4f25dddcf897edb5832c6a8393c
---

# Phase 21 Plan 10: Messages page safeguards Summary

The Messages page is now held to every settings rule of v1.1 by measurement: every button of every screen is at most 64
bytes with the exact sizes the plans fixed, every screen fits 3800 UTF-16 units in all 8 languages with a 64-rune title
and the longest notices in the row shapes the plans fixed, every forged press of the eight action codes is refused in
step 1 before any lookup, no draw creates a row or a cache key, a draw costs only the press pipeline's calls, no log line
carries a stored or typed text, caption, button or file ID, and no screen names the Staff Group in any language. No
safeguard found a defect, so no production code changed.

## Tasks

| Task | Name | Commit |
|------|------|--------|
| 1 | TestSettingsMessagesCallbackBytes and TestSettingsMessagesPageFitsEveryLocale over settingsMsgScreens | 6b76658 |
| 2 | TestSettingsMessagesForged, TestSettingsMessagesReadsCreateNothing, TestSettingsMessagesCost, TestSettingsMessagesLogsCarryIDsOnly | 478fcfe |
| 3 | settingsPrivacyPhase21Screens in TestSettingsStaffPrivacy; AGENTS.md (the Messages page bullet's limits, the Phase 21 test families) | 540a2a9 |

## Measured limits

| What | Measured |
|------|----------|
| Bytes at chat ID `-1001234567890` | `hl` (hub cell, family Back) 37, `he` 38, `hs hr hv hc hq` 42, `hf` 38, `op` (the family list's Back to the hub) 37, Back to Main 33, Close 33, Main cell 37 private and 33 group, group note buttons 29 |
| Fields | no button carries a field outside `a c u p t k v` |
| Rows | hub `[1 2]`, family list `[2 2 2 1 2]`, event screen `[1 1 1 1 1 2]` empty and `[2 2 2 2 2 1 2]` full (photo or sticker, Silent on), prompt and refusal `[1]`, group note `[1 2]` and `[2]` without a bot username |
| Length | every screen 1..3800 UTF-16 units in 8 languages with a 64-rune title and the two longest of the page's notices; no unfilled placeholder |
| Cells | half-width at most 20 code points, full-width at most 40 |

## What plan 21-11 uses

| Kind | Name |
|------|------|
| Screens | `settingsMsgScreens(t, view, notices)` (map of `settingsMsgNamedScreen{screen, widths}`), `settingsMsgAllCustom(t, silent)`, `settingsMsgFullState(mediaType)`, `settingsMsgEmptyState()` |
| Forged presses | `settingsMsgValidPresses` (one valid press per code), `settingsMsgForgedPresses()`, `(*settingsMsgEnv).pressForged(chat, user, msgID, f)` |
| Counters | `settingsMsgCountReads(t)` (wraps `settingsMsgRead` with an atomic counter), `settingsMsgSafetyCounts(t, chatID)` (event message, chats and rules rows) |
| Sentinels | `settingsMsgSentinelText`, `File`, `Caption`, `Button`, `Link` (no digits) |

### Notes for plan 21-11 (end-to-end and phase gates)

- `TestSettingsStaffPrivacy`'s pure-render subtest takes about 28 s in all; the Phase 21 addition is about 0.15 s per
  language (about 1.2 s). Plan 21-11 should not add another language sweep; the end-to-end test needs one environment
  per subtest and no language cross product.
- The `{rules}` side effect stays documented, not tested here: a stored event text with `{rules}` makes a preview (and the
  send path) read the group's rules through the join path's reader, which creates an empty rules row. The reads test
  stores no `{rules}`, so its "no rules row" assertion holds; an end-to-end test that stores `{rules}` must not assert
  that no rules row exists.
- `TestSettingsGateAllDoors` still has no Messages column (as 21-09 noted, the page's presses run the same pipeline, step
  4b included); the safety test proves the forged side, not the owner gate worlds.
- `make lint` has not been run on any plan of the phase (see below); plan 21-11's gate should run it where the toolchain
  can load Go 1.26.

## Verification

- `go test -tags testtools -race -count=1 -run '^(TestSettingsMessagesCallbackBytes|TestSettingsMessagesPageFitsEveryLocale)$' -v ./fuku/modules`: PASS (0.48 s and 3.2 s, 8 language subtests, no SKIP).
- `go test -tags testtools -race -count=1 -run '^(TestSettingsMessagesForged|TestSettingsMessagesReadsCreateNothing|TestSettingsMessagesCost|TestSettingsMessagesLogsCarryIDsOnly)$' -v ./fuku/modules`: PASS (each about 0.4 s, package 2.7 s), no SKIP.
- `go test -tags testtools -race -count=1 -timeout 30m -run '^(TestSettingsStaffPrivacy|TestSettingsMessages.*|TestSettingsDriftMessages.*|TestSettingsAuthorityMessages.*)$' ./fuku/modules`: ok (51.6 s).
- `go vet -tags testtools ./fuku/modules ./fuku/i18n` clean; `CGO_ENABLED=0 go build ./...` ok; `gofmt -l` clean on the three test files; `make check-docs`: no drift.
- Mutation checks (reverted): a `log.Warnf` of the typed text and file ID in `settingsMsgWrite` fails `TestSettingsMessagesLogsCarryIDsOnly` on both sentinels; `settingsMsgSilentFields` without the `v` check fails `TestSettingsMessagesForged` (a `v=2` press reached `getChatMember` and `getChat`).
- Acceptance greps: each of the six test functions once in its file; `[2 2 2 2 2 1 2]` in the screens test; `settingsPrivacyPhase21Screens` 3 times in the privacy test; `TestLegacyModerationRequests` and `TestSettingsMessages` in AGENTS.md.

## Not verified

- `make lint` was not run (the installed golangci-lint cannot load the Go 1.26 module), as instructed; `ineffassign` was checked by hand on the new tests, and `unused` does not apply (no production identifier was added).
- The whole `fuku/modules` package was not run, as instructed.
- No PostgreSQL run: nothing here rests on a constraint.
- `make check-translations` was not run: no locale file changed.

## Deviations from Plan

### Auto-fixed Issues

None in production code.

**1. [Plan behavior] The preview's 403 logs no line**
- **Found during:** Task 2
- **Issue:** the plan's behavior said every failure line of the preview's 403 names the chat; the 403 path
  (`settingsEditPreviewAnswer`) logs nothing by design, so there is no line to name the chat.
- **Resolution:** the subtest asserts the 403's need-start alert, then scripts a 400 on the content (`sendPhoto`) and asserts the
  failed alert and a log line naming the chat; the content sweep covers both. Recorded under decisions; no production change.

**2. Forged presses also from a private menu carrying u**
- Besides the plan's group menu press (`u` in a group chat), each code is also pressed in a private chat with `u` in place of `c`;
  both are refused in step 1.

**3. Commit split**
- The plan named one commit (`test(21-10): hold the Messages page to the settings limits`); each task is its own commit, the last
  one carrying that subject with AGENTS.md beside the test change it describes.

## Known Stubs

None.

## Threat Flags

None: T-21-31 (`TestSettingsMessagesForged`), T-21-32 (`TestSettingsMessagesCallbackBytes`,
`TestSettingsMessagesPageFitsEveryLocale`) and T-21-33 (`settingsPrivacyPhase21Screens`,
`TestSettingsMessagesLogsCarryIDsOnly`) are mitigated as planned.

## Self-Check: PASSED

- FOUND: fuku/modules/settings_messages_screens_test.go, fuku/modules/settings_messages_safety_test.go
- FOUND commits: 6b76658, 478fcfe, 540a2a9, all ancestors of HEAD
