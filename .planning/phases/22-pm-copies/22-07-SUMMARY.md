---
phase: 22-pm-copies
plan: 07
subsystem: event-messages-pm-copies
status: complete
tags: [pm-copies, log-channel, settings-menu]
requires: [22-06]
provides:
  - pmCopyPostLog(ctx, o, outcome), the paced #PM_COPY post in the group's log channel under the admin category
  - pmCopyLogText(tr, o, outcome) (pure, the line without the header), eventPMEventName(tr, event), pmCopyLogTag
  - eventPMOffer.TargetName and eventPMOffer.Tr (the group's translator, set only while the log option is on)
  - applyEventPMLog, eventMsgWrites.PMLog
  - the hub's Log row of settingsMsgPMOptions (t = l) and the hint settings_msg_hint_pm_log; hub rows [1 1 1 2]
  - strings eventmsg_pm_log, eventmsg_pm_why_blocked, eventmsg_pm_why_failed, settings_msg_line_pm_log, settings_btn_msg_pm_log, settings_msg_hint_pm_log
  - tests TestEventPMLogLine; the Log rows of TestSettingsMessagesPMOptions, TestSettingsDriftMessagesPMOptions, TestSettingsAuthorityMessagesPMOptions
  - toolkit eventPMLogLink, eventPMLogOn, eventPMLogPosts, eventPMLogWant; settingsMsgOptionCases, settingsMsgOptionLine, settingsMsgOptionToggle, settingsMsgOptionEffect
affects: [22-08, 22-09]
tech-stack:
  added: []
  patterns:
    - "a best-effort admin log post from a background job: the handler hands the job a translator, the job's only database read is the log destination"
    - "the hub's option tests run every case for every option row from one table"
key-files:
  created:
    - fuku/modules/event_message_pm_log.go
    - fuku/modules/event_message_pm_log_test.go
  modified:
    - fuku/modules/event_message_pm.go
    - fuku/modules/event_message.go
    - fuku/modules/event_message_apply.go
    - fuku/modules/settings_messages.go
    - fuku/modules/settings_messages_pm_options_test.go
    - fuku/modules/settings_messages_test.go
    - fuku/modules/settings_messages_screens_test.go
    - fuku/i18n/settings_locale_test.go
    - fuku/i18n/eventmsg_locale_test.go
    - locales/en.yml, es.yml, fr.yml, hi.yml, id.yml, pt.yml, ru.yml, uk.yml
    - AGENTS.md
decisions:
  - "The job gets the group's translator on the offer (eventPMOffer.Tr, set by offerReplyPM only while the log option is on), so the log destination stays the job's only database read; a nil translator posts nothing."
  - "The event's name in the sentence is quoted in every language (\"…\" / «…»), because the page's name keys are capitalised nouns (Ban, Бан)."
  - "The log option's hint is its own key (settings_msg_hint_pm_log, as the plan names it) on the line after the Details hint; settings_msg_hint_pm (the private copy screen) now says the Details button and the log line are both set on the hub. settings_msg_hint_pm_options keeps describing the Details button only."
  - "With no name known, the mention's text is the member's ID, so the link is never empty."
  - "AGENTS.md text went into the commit of the change it describes (task 1's PM copies bullet and Testing, task 2's Messages page, sizes, rows and Permissions), so there is no separate task 3 commit."
metrics:
  duration: ~45 min
  completed: 2026-11-21
actuals:
  tokens: 11856
  tasks: 3
  commits: 2
plan_head_before: fe69bda909f6edeaee90fba0a4910ada934d24b1
plan_head_after: 6caa0ca316b166b4197794d2aff71907fc631b10
---

# Phase 22 Plan 07: The log line Summary

With a group's log option on, a private copy that could not reach the punished person (blocked, refused, busy or unknown) now
leaves one line in the group's log channel under its admin category: the usual header, `#PM_COPY` (untranslated) and one sentence
in the group's language naming the member by mention and ID, the event's name from the Messages page and why (blocked, or Telegram
did not deliver it). It goes through the private copies' own pacer, is best effort and never touches the punishment, the group
reply or the copy. Nothing is posted for a delivered, shed or capped copy, a group with no log channel or with the admin category
off. The owner switches it from a second toggle in the Messages hub's private copies block (`ho`, `t=l`, `can_change_info`).

## What later plans use

| Kind | Name |
|------|------|
| Post | `pmCopyPostLog(ctx, o, outcome)` at the end of `runEventPM`, after the Details step, when `o.Options.PMLog`; `pmCopyLogTag` = `#PM_COPY` |
| Text | `pmCopyLogText(tr, o, outcome)` (pure, no header), `eventPMEventName(tr, event)` (the page's name key, the event key escaped for an unlisted event) |
| Offer | `eventPMOffer.TargetName` (always), `eventPMOffer.Tr` (only while the log option is on) |
| Option apply | `applyEventPMLog(ctx, chatID, on, by)`, `eventMsgWrites.PMLog` |
| Hub | the Log row of `settingsMsgPMOptions` (`l`, `settings_msg_line_pm_log`, `settings_btn_msg_pm_log`), `settings_msg_hint_pm_log` after `settings_msg_hint_pm_options`; rows `[1 1 1 2]` |
| Toolkit | `eventPMLogLink`, `eventPMLogOn`, `eventPMLogPosts` (channel 0 means any chat), `eventPMLogWant` (`event_message_pm_log_test.go`); `settingsMsgOptionCases`, `settingsMsgOptionLine`, `settingsMsgOptionToggle`, `settingsMsgOptionEffect` (`settings_messages_pm_options_test.go`, replacing 22-06's `settingsMsgDetailsLine` and `settingsMsgDetailsToggle`) |

## Tasks

| Task | Name | Commit |
|------|------|--------|
| 1 | the log line: `event_message_pm_log.go`, the offer fields, the hook in `runEventPM`, three strings in 8 languages with limits rows, `TestEventPMLogLine`, AGENTS.md PM copies bullet and Testing | f72b23c |
| 2 | the hub's log toggle: `applyEventPMLog`, the Log row and its hint, three strings in 8 languages and the `settings_msg_hint_pm` update, the three option tests as tables over both rows, the widened hub expectations, AGENTS.md Messages page, sizes, rows and Permissions | 6caa0ca |
| 3 | AGENTS.md | folded into f72b23c and 6caa0ca |

## Verification

- `go test -tags testtools -race -count=1 -run '^(TestEventPMLogLine|TestEventPMDetails|TestEventPMFailures|TestEventPMLogsCarryIDsOnly|TestPMCopyCallers|TestCommandRepliesStayInsideTheHandler|TestLegacyModerationRequests)$' -v ./fuku/modules`: ok (11 s), every test PASS, no SKIP.
- `go test -tags testtools -race -count=1 -run '^(TestSettingsMessagesPMOptions|TestSettingsDriftMessagesPMOptions|TestSettingsAuthorityMessagesPMOptions)$' -v ./fuku/modules`: ok, every subtest of both rows PASS.
- `go test -tags testtools -race -count=1 -run '^(TestSettingsMessages.*|TestSettings(Authority|Drift)Messages.*|TestMessagesEndToEnd|TestEventPM.*|TestPMCopy.*|TestEventMessage.*|TestLegacyModerationRequests|TestSettingsStaffPrivacy|TestCommandRepliesStayInsideTheHandler|TestSettingsPhase1[89]Codes|TestSettingsControlActionAllowList|TestSettingsMain.*|TestSettingsGateAllDoors)$' ./fuku/modules`: ok (140 s).
- `go test -tags testtools -race -count=1 -run '^(TestSettingsLocaleKeys|TestSettingsLabelLengths|TestSettingsEditLocaleTokens|TestLocaleStyle|TestEventMsgLocaleKeys|TestLocalePlaceholdersMatchOtherLanguages|TestSettings.*)$' ./fuku/i18n`: ok.
- `make check-translations`: all present; `make check-docs`: no drift.
- `CGO_ENABLED=0 go build ./...` ok; `gofmt -l` empty on every touched Go file.
- golangci-lint: `run --new-from-rev=origin/main ./...` 0 issues; with `--build-tags=notesttools` 0 issues.
- Acceptance: `func pmCopyPostLog(` once in `event_message_pm_log.go`, `pmCopyPostLog(` once in `event_message_pm.go`, `#PM_COPY` in `event_message_pm_log.go`; every locale file has the three `eventmsg_pm_log`/`eventmsg_pm_why_` keys; `func applyEventPMLog(` once; `settings_msg_line_pm_log` once in `settings_messages.go`; `#PM_COPY` twice and `pmCopyPostLog` once in AGENTS.md; `git diff --exit-code 9940d97 -- go.mod go.sum fuku/utils/actionlog/actionlog.go` exits 0, and neither plan commit touches `main.go` or `moderation_legacy_request_test.go`.
- Mutation proofs: with the destination check (`if !ok`) disabled in `pmCopyPostLog`, `TestEventPMLogLine/admin_category_off` and `/no_log_channel` failed (once the "nothing" cases count posts to any chat); reverted. With the `Options.PMLog` gate in `runEventPM` replaced by `true`, the tests still passed, because the translator is only on the offer while the option is on and `pmCopyPostLog` posts nothing without it: the gate is held twice and a test cannot tell the two apart.

## Deviations from Plan

### Auto-fixed Issues

None: no bug was found in existing code.

### Other deviations

**1. [Design] the translator travels on the offer**
- The plan's `pmCopyPostLog(ctx, o, outcome)` needs the group's language; the job has no translator and the plan keeps the destination as its only database read, so `offerReplyPM` puts `c.Tr` on the offer (`eventPMOffer.Tr`) while the log option is on. A capped test-only offer carries one explicitly.

**2. [Commits] no separate task 3 commit**
- The caller's rule puts AGENTS.md text in the commit of the change it describes, so task 1's commit carries the PM copies bullet and the Testing entry, task 2's the Messages page, sizes, rows and Permissions text. Task 1 has the plan's planned subject (`feat(22-07): log a private copy that could not be delivered`); task 2 is a second `feat`.

**3. [Strings] the hints**
- The caller's note said to add the log line to `settings_msg_hint_pm_options` and `settings_msg_hint_pm`; the plan named a new key `settings_msg_hint_pm_log`. Both are honoured where they do not conflict: the new key holds the log line's hint (on the line after the Details hint), and `settings_msg_hint_pm` now says the Details button and the log line are set on the hub's first screen. `settings_msg_hint_pm_options` is unchanged.

**4. [Test] the "nothing" cases look at every chat**
- `eventPMLogPosts(t, s, 0)` counts `#PM_COPY` posts to any chat, so a post sent to the wrong destination cannot pass as "nothing posted" (found by the mutation above).

**5. [Test] helpers renamed**
- 22-06's `settingsMsgDetailsLine` and `settingsMsgDetailsToggle` became `settingsMsgOptionLine` and `settingsMsgOptionToggle` over `settingsMsgOptionCases`; AGENTS.md's toolkit list follows.

### Expected-set edits of earlier tests (widened, no other change)

- `TestSettingsMessagesOpens`: the hub's widths `[1 1 2]` to `[1 1 1 2]` and Back's row index 2 to 3.
- `settingsMsgScreens`: the hub's widths `[1 1 2]` to `[1 1 1 2]` (the "all custom" state already had both options on).
- `TestEventMsgLocaleKeys`: the three new keys join the exception list of keys that need not name `{group}` (the log channel's header names the group).

No `TestLegacy*Requests` golden and no `TestSettingsDeletionPhase16ScreensUnchanged` digest was touched; `TestLegacyModerationRequests` passes unedited.

## Not verified

- The whole `fuku/modules` package was not run (targeted runs only); `make test`, `make test-shard-check` and `make lint` itself were not run (the CI-equivalent golangci-lint runs above were).
- No PostgreSQL run: nothing of this plan rests on a constraint.
- No live Telegram check of the log post's look in a real channel.
- `TestEventPMLogsCarryIDsOnly` was not extended to the log post's failure line (one Warn with the chat ID and the code); `TestEventPMLogLine` covers the posted line's content only.

## Notes for plan 22-08 (safeguards)

- Extend `TestEventPMLogsCarryIDsOnly` with a failing log post (an error on `sendMessage` to the log channel): the Warn line `the #PM_COPY log line of chat <id> could not be posted (code N)` must carry no name, text or token.
- Sweep `pmCopyLogText` in 8 languages: render it with a 64-rune name full of markup and every event, check the length, that it names no Staff Group, and that the member's name appears only inside the mention (the T-22-29 sweep the plan assigns to 22-08).
- `TestSettingsStaffPrivacy` renders the hub through `settingsMsgScreens`, so the Log row and hint are already in its 8-language run; the log post itself is not a settings screen.
- The log post uses `actionlog.Destination` (a cached `logchannels.Get`), so a pacer-block test can block `fuku:pmcopy:pace:block` and expect the log line to wait or fail with the copy, never to delay the ban.
- `eventPMOffer.Tr` is nil unless the log option is on; a test that offers a copy straight to the engine with `Options.PMLog` must set `Tr` (and `TargetName`) itself, as the `capped` subtest does.

## Known Stubs

None.

## Threat Flags

None: no surface beyond the plan's threat model. T-22-27 (`TestEventPMLogLine` "no content"), T-22-28 (shed and capped post nothing, "capped"; posts on the private copies' pacer), T-22-29 (the line is built from the event's chat and the offer only; the 8-language sweep is 22-08's) and T-22-30 (the Log rows of `TestSettingsAuthorityMessagesPMOptions`) are mitigated.

## Self-Check: PASSED

- Files: `fuku/modules/event_message_pm_log.go` and `fuku/modules/event_message_pm_log_test.go` exist.
- Commits: f72b23c and 6caa0ca are ancestors of HEAD.
