---
phase: 22-pm-copies
plan: 06
subsystem: event-messages-pm-copies
status: complete
tags: [pm-copies, details-button, deep-link, settings-menu]
requires: [22-03, 22-05]
provides:
  - the Details record fuku:pmcopy:det:<token> (48 h, pmCopyDetailsPut, pmCopyDetailsGet, pmCopyNewToken)
  - pmCopyAddDetails(ctx, o, tried), the paced editMessageReplyMarkup that keeps the group message's rows and adds one URL row
  - the pmcopy_ deep link pmCopyDetailsDeepLink (strict parse, one neutral reply, the copy only for the record's user)
  - eventPMOffer.Options, GroupMsgID, GroupRows, DetailsLabel; replyEventGroup returns the posted message and its rows
  - applyEventPMDetails, eventMsgWrites.PMDetails, eventMsgOptionsResult{Outcome; Messages; Err}
  - the hub's private copies block, settingsMsgPMOptions (row d), settingsMsgPMOptionByCode, the ho action (t, v, settingsRightChangeInfo)
  - strings eventmsg_btn_details, eventmsg_pm_details_none, settings_msg_line_pm_details, settings_btn_msg_pm_details, settings_msg_hint_pm_options
  - tests TestEventPMDetails, TestPMCopyDetailsDeepLink, TestSettingsMessagesPMOptions, TestSettingsDriftMessagesPMOptions, TestSettingsAuthorityMessagesPMOptions
  - toolkit messagesE2EClient.refuse, settingsMsgDetailsLine, settingsMsgDetailsToggle, settingsMsgStoredOptions, settingsMsgSendPathOptions
affects: [22-07, 22-08, 22-09]
tech-stack:
  added: []
  patterns:
    - "a record behind a 128-bit random token, read by the token alone and shown only on a user-ID match, with one neutral reply for every refusal"
    - "an option of the hub is one table row (code, keys, reader, apply), so a later option needs no renderer change"
key-files:
  created:
    - fuku/modules/event_message_pm_details.go
    - fuku/modules/event_message_pm_details_test.go
    - fuku/modules/settings_messages_pm_options_test.go
  modified:
    - fuku/modules/event_message_pm.go
    - fuku/modules/event_message.go
    - fuku/modules/event_message_apply.go
    - fuku/modules/settings_messages.go
    - fuku/modules/event_message_pm_env_test.go
    - fuku/modules/messages_e2e_test.go
    - fuku/modules/settings_messages_test.go
    - fuku/modules/settings_messages_screens_test.go
    - fuku/i18n/settings_locale_test.go
    - fuku/i18n/eventmsg_locale_test.go
    - locales/en.yml, es.yml, fr.yml, hi.yml, id.yml, pt.yml, ru.yml, uk.yml
    - AGENTS.md
decisions:
  - "pmCopyAddDetails takes the copy the job actually tried to send as a third argument (the composed copy, or the default text after a refused custom copy), so the record holds exactly what the person would have received."
  - "The Details label is translated in the handler (eventPMOffer.DetailsLabel, read only while the option is on), not by the job, so the job needs no translator."
  - "eventMsgOptionsResult carries the chat's fresh Messages (whose Options hold the stored options) rather than the options alone, so the hub redraws from the read-back with its event lines intact."
  - "The hub toggle writes with no same-value shortcut: the value is absolute, so a stale hub's press stores the same value again."
  - "The drift test drives the real dispatcher (newMessagesE2E) with the member's private sends refused by a small hook on messagesE2EClient, and proves the cache drop by banning once with the option off (the send path caches the off) before the hub press."
metrics:
  duration: ~1 h over two sessions (paused after task 2's production code)
  completed: 2026-11-21
actuals:
  tokens: 19964
  tasks: 3
  commits: 3
plan_head_before: 6f370ea720ae8b1cbf1967c0fec2476ee16b0ae2
plan_head_after: 14a74b0d7bd6f8b3c36443b382887e2fb87ef514
---

# Phase 22 Plan 06: The Details button Summary

When a group has its Details option on and a private copy could not reach the punished person (blocked, refused, busy or unknown),
the job stores the copy for 48 hours under `fuku:pmcopy:det:<token>` (a 22-character base64url token from 16 random bytes) and adds
one URL row, `https://t.me/<bot>?start=pmcopy_<token>`, to the group message through the private copies' pacer, keeping the rows the
message was sent with. `/start pmcopy_<token>` shows the stored copy to the punished person only, again on every open until it
expires; anyone else, a malformed or unknown token and a Redis error get one neutral reply naming nothing. The owner switches the
option from a new private copies block on the Messages hub (toggle `ho`, `can_change_info`), and the send path reads it at the next
command on every replica.

## What later plans use

| Kind | Name |
|------|------|
| Record | `pmCopyDetailsKey(token)`, `pmCopyDetailsTTL` (48 h), `pmCopyDetailsRecord` (`u c e t f y b`), `pmCopyDetailsPut`, `pmCopyDetailsGet`, `pmCopyNewToken`, `pmCopyTokenLen` |
| Edit | `pmCopyAddDetails(ctx, o, tried)` at the end of `runEventPM` for a final outcome other than delivered; `pmCopyBotUsername` (a variable so a test can clear it); `pmCopyMaxButtons` (100) |
| Deep link | `parsePMCopyDetails`, `pmCopyDetailsDeepLink`, registered from the file's `init` (`help.go` and `deeplink_router.go` untouched) |
| Offer | `eventPMOffer.Options`, `.GroupMsgID`, `.GroupRows`, `.DetailsLabel`; `replyEventGroup` returns an `eventGroupPost` |
| Option apply | `applyEventPMDetails(ctx, chatID, on, by)`, `eventMsgWrites.PMDetails`, `eventMsgOptionsReadBack`, `eventMsgOptionsResult` |
| Hub | `settingsMsgPMOptions` (one row, `d`), `settingsMsgPMOption{Code, LineKey, LabelKey, Value, Apply}`, `settingsMsgPMOptionByCode`, `settingsMsgPMOptionFields`, `settingsMsgAnyPM`, `settingsMsgRunPMOption`, `settingsActMsgPMOption` = `ho` (41 bytes) |
| Toolkit | `eventPMDetailsTokens`, `eventPMDetailsRecord`, `eventPMDetailsTokenOf` (`event_message_pm_env_test.go`); `eventPMDetailsOn`, `eventPMDetailsEdits`, `eventPMKeyboardRows` (`event_message_pm_details_test.go`); `messagesE2EClient.refuse` (`messages_e2e_test.go`); `settingsMsgDetailsLine`, `settingsMsgDetailsToggle`, `settingsMsgStoredOptions`, `settingsMsgSendPathOptions` (`settings_messages_pm_options_test.go`) |

## Tasks

| Task | Name | Commit |
|------|------|--------|
| 1 | the Details record, the paced edit, the `pmcopy_` deep link, the two strings in 8 languages, `TestEventPMDetails`, `TestPMCopyDetailsDeepLink` and AGENTS.md | 4cf6afb |
| 2 | `applyEventPMDetails`, the option table, `ho` and the hub block with its strings in 8 languages and the widened hub expectations (a `wip` commit at the pause) | fabf813 |
| 2, 3 | the three option tests, the `refuse` hook and the AGENTS.md text of task 2 (Messages page bullet, Callbacks field list, Permissions, sizes and rows, test families) | 14a74b0 |

The branch was paused after fabf813 (handoff fd4fe72); main (Conversations, Phase 20) was then merged in (16fbe95) and the
milestone's phases renumbered (2dcb0c7), so the first two commits carry the subject `21-06`, which is this plan under its old number.

## Verification

- `go test -tags testtools -race -count=1 -run '^(TestSettingsMessagesPMOptions|TestSettingsDriftMessagesPMOptions|TestSettingsAuthorityMessagesPMOptions)$' -v ./fuku/modules`: ok, every subtest PASS.
- `go test -tags testtools -race -count=1 -run '^(TestSettingsMessages.*|TestSettings(Authority|Drift)Messages.*|TestMessagesEndToEnd|TestEventPM.*|TestPMCopy.*|TestEventMessage.*|TestSettingsPhase1[89]Codes|TestSettingsControlActionAllowList|TestSettingsStaffPrivacy|TestSettingsMain.*|TestLegacyModerationRequests|TestCommandRepliesStayInsideTheHandler|TestSettingsGateAllDoors)$' ./fuku/modules`: ok (134 s), no FAIL or SKIP. `TestMessagesEndToEnd` needed no widening: it never asserts the hub's rows.
- `go test -tags testtools -race -count=1 -run '^(TestSettingsLocaleKeys|TestSettingsLabelLengths|TestSettingsEditLocaleTokens|TestLocaleStyle|TestEventMsgLocaleKeys|TestLocalePlaceholdersMatchOtherLanguages|TestSettings.*)$' ./fuku/i18n`: ok.
- `make check-translations`: all present; `make check-docs`: no drift.
- golangci-lint v2.13.1: `run --new-from-rev=origin/main ./...` 0 issues; with `--build-tags=notesttools` 0 issues.
- `CGO_ENABLED=0 go build ./...` ok; `gofmt -l` empty on the files this session changed.
- Acceptance: `RegisterDeepLinkHandler(pmCopyDetailsPrefix` once in `event_message_pm_details.go`; `pmCopyAddDetails(` once in `event_message_pm.go`; `func applyEventPMDetails(` once; `settingsActMsgPMOption` 4 times in `settings_messages.go`; the three test functions once each in `settings_messages_pm_options_test.go`; `fuku:pmcopy:det:` 3 times and `pmCopyDetailsDeepLink` once in AGENTS.md. None of the three plan commits touches `go.mod`, `go.sum`, `main.go`, `fuku/modules/help.go` or `fuku/modules/deeplink_router.go`; `go.mod`, `go.sum`, `help.go` and `deeplink_router.go` are unchanged since 9940d97, and `main.go` differs from 9940d97 only by main's merged Conversations and version-bump commits. `moderation_legacy_request_test.go` is unchanged since its last commit (the renumbering, not this plan).
- Mutation proof (task 1, T-22-22): with `rec.UserID != user.Id` removed from `pmCopyDetailsDeepLink`, `TestPMCopyDetailsDeepLink/someone_else` failed; reverted, the tree clean.
- Mutation proof (task 2, T-22-26): with the `ho` action's right set to `settingsRightAdmin`, `TestSettingsAuthorityMessagesPMOptions` failed (no alert, one edit, the options row written by the administrator without `can_change_info`); reverted.

## Deviations from Plan

### Auto-fixed Issues

None: no bug was found in existing code.

### Other deviations

**1. [Signature] `pmCopyAddDetails` takes the attempted copy**
- The plan's signature was `pmCopyAddDetails(ctx, o eventPMOffer)`. It takes a third argument, the copy the job actually tried to send, because after a refused custom copy the job sent the default text and the record must hold that.

**2. [Design] the Details label is translated in the handler**
- `eventPMOffer.DetailsLabel` is read by `offerReplyPM` (the group translator, only while the option is on), not by the job, which has no translator.

**3. [Design] `eventMsgOptionsResult` carries the fresh `Messages`**
- The plan named `Options *eventmsg.Options`; the result carries the chat's `*eventmsg.Messages` read back fresh (their `Options` are the stored options), so the hub redraws its event lines from the same read.

**4. [Test] an exception list in `TestEventMsgLocaleKeys`**
- `eventmsg_btn_details` and `eventmsg_pm_details_none` do not name the group, so the test gained an exception list for keys that need not carry `{group}`.

**5. [Test] "no username" through `pmCopyBotUsername`**
- The plan proposed clearing the site bot's username; the branch is tested by swapping the variable `pmCopyBotUsername` (default `getBotUsername`), which the edit reads for the URL.

**6. [Test] a refusal hook on `messagesE2EClient`**
- The drift test needs the member's private send to answer 403 through the real dispatcher: `messagesE2EClient.refuse(chatID, err)` answers every `send*` to that chat with the error before the fake records anything. AGENTS.md describes it.

**7. [Commits] a `wip` commit and split test commit**
- The session was paused mid-task 2, so the hub's production code landed as `wip(21-06)` (fabf813), which the changelog lists under "Other", not "Features". The plan's planned closing `feat(22-06)` commit would have carried no production code, so the closing commit is `test(22-06)` with the tests and the task's AGENTS.md text together (the AGENTS.md text follows the change as closely as the pause allows). The user-visible Details button itself is announced by task 1's `feat` commit.

**8. [Check] lint base**
- golangci-lint was run with `--new-from-rev=origin/main` (the caller's instruction after the merge) rather than the handoff's `9940d97`.

### Expected-set edits of earlier tests (widened, no other change)

- `TestSettingsMessagesOpens`: the hub's widths `[1 2]` to `[1 1 2]` and Back's row index 1 to 2 (fabf813).
- `settingsMsgScreens`: the hub's widths `[1 2]` to `[1 1 2]`, the "all custom" state with both options on; `TestSettingsMessagesCallbackBytes` gained `ho` at 41 (fabf813).

No `TestLegacy*Requests` golden and no `TestSettingsDeletionPhase16ScreensUnchanged` digest was touched.

## Not verified

- The whole `fuku/modules` package was not run (targeted runs only); `make test`, `make test-shard-check` and `make lint` itself were not run (the CI-equivalent golangci-lint runs above were).
- No PostgreSQL run: nothing of this plan rests on a constraint.
- No live Telegram check: that a URL button can be added by `editMessageReplyMarkup` to a message the bot posted, and how `/start pmcopy_...` behaves for someone who never started the bot, are covered by fakes only.

## Notes for plan 22-07 (the log line)

- Hook the `#PM_COPY` log line beside the `pmCopyAddDetails` call at the end of `runEventPM` (same "not delivered" condition, a shed or capped copy never reaches it), gated by `o.Options.PMLog`.
- The log option is one more row of `settingsMsgPMOptions` (`t` = `l`, `Value` reads `Options.PMLog`) with a new `applyEventPMLog` beside `applyEventPMDetails` (`eventMsgWrites.PMLog` = `eventmsg.SetPMLog`, `eventMsgOptionsReadBack` with what `"log"`); the renderer and `ho` need no change.
- The hub then becomes `[1 1 1 2]`: widen `TestSettingsMessagesOpens` (Back moves to row 3), `settingsMsgScreens`, `TestSettingsMessagesPMOptions`' two `settingsMsgWantWidths` calls, and AGENTS.md's two hub row mentions. `settingsMsgDetailsToggle` finds the Details toggle by `t=d`, so it keeps working with a second toggle.
- Add the log line to `settings_msg_hint_pm_options` and `settings_msg_hint_pm` in all 8 languages, with the limits rows in `fuku/i18n/settings_locale_test.go`.
- `messagesE2EClient.refuse` is available to drive a blocked copy through the real dispatcher for the log line's drift test.

## Known Stubs

None.

## Threat Flags

None: no surface beyond the plan's threat model. T-22-22 (the user-ID check, mutation proof), T-22-23 (128-bit tokens, one neutral reply, "forged"), T-22-24 (strict parse, read by token only), T-22-25 (rows kept, "custom group message keeps its rows") and T-22-26 (`settingsRightChangeInfo` on `ho`, `TestSettingsAuthorityMessagesPMOptions`, mutation proof) are mitigated and tested.

## Self-Check: PASSED

- Files: `fuku/modules/event_message_pm_details.go`, `fuku/modules/event_message_pm_details_test.go` and `fuku/modules/settings_messages_pm_options_test.go` exist.
- Commits: 4cf6afb, fabf813 and 14a74b0 are ancestors of HEAD.
