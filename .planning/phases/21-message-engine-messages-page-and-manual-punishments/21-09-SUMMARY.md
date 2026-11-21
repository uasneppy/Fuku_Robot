---
phase: 20-message-engine-messages-page-and-manual-punishments
plan: 09
subsystem: settings menu / Messages page / message editor previews
status: complete
tags: [settings, messages-page, event-messages, message-editor, preview, MSG-06]
requires: [21-03, 21-04, 21-05, 21-08]
provides:
  - Preview (hv) and Full preview (hf) on every event screen, from the page's one settingsEditResolvedActions map
  - settingsMsgPreview (the event definitions' Preview hook) and settingsMsgPreviewReply
  - settingsMsgEvent.Default (the send site's default builder) and .Timed
  - settings_msg_preview_header and settings_msg_sample_reason in 8 languages
affects: [21-10, 21-11]
tech-stack:
  added: []
  patterns:
    - "a preview of an event message reuses the send path's composer (composeEventMessage) with a reply built from the event's own default builder, so the preview cannot drift from what members see"
    - "the preview's reply carries no options, so the bot's live action row (Unban, Unmute) is never composed into a private chat"
key-files:
  created:
    - fuku/modules/settings_messages_preview_test.go
  modified:
    - fuku/modules/settings_messages.go
    - fuku/modules/settings_messages_test.go
    - fuku/modules/settings_messages_edit_test.go
    - fuku/modules/settings_messages_silent_test.go
    - fuku/i18n/settings_locale_test.go
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml
    - locales/uk.yml
    - AGENTS.md
decisions:
  - "An event's text Preview is offered only while a text is stored (settingsEditPreviewable is unchanged: only Welcome and Goodbye treat the default text as previewable); the default text is seen through Full preview, which is always drawn."
  - "Full preview takes the group's language from lang.GetGroupLanguage (the Rules preview's read); the header line stays in the presser's language (settingsEditPreviewSend's p.tr)."
  - "The sample duration is the constant `2 hours`, the untranslated form a timed reply shows (parseTemporaryDuration's timeStr), not a translated phrase."
  - "Preview text is renderEventText with no buttons (so a {rules} button is not shown there, as the Welcome text preview drops it); Full preview carries the stored and {rules} buttons."
  - "event_message.go is not edited: the preview calls composeEventMessage, renderEventText and eventPlaceholderValues as they are."
metrics:
  duration: ~35 min
  completed: 2026-11-19
actuals:
  tokens: 26000
  tasks: 2
  commits: 2
plan_head_before: 15b96c16b0acf86a4ae6512abfb7b7346b521fcb
plan_head_after: 92a14f718324cb4432da82b0b3f4c3e61f25aba0
---

# Phase 21 Plan 09: Event message previews Summary

Every event screen of the private Messages page now has the editor's preview rows. Full preview sends the presser, in
their own private chat only, the message the event would post in the group: built by `composeEventMessage`, the send
path's own composer, from the event's own default builder (`banDefaultReply`, `tbanDefaultReply`, ...) and the stored
parts, filled with the presser as the member and the admin, a sample reason and, for `/tban` and `/tmute`, `2 hours`
and an end two hours ahead, in the group's language. Preview text, media and buttons show one part each. A preview needs
only the admin check, ignores Silent, carries no live Unban or Unmute button, writes nothing and schedules nothing.

## Tasks

| Task | Name | Commit |
|------|------|--------|
| 1 | Preview and Full preview of every event through the send path's composer; two strings in 8 languages; the event screen's row expectations; AGENTS.md | 1a9d827 |
| 1 (tests) | TestSettingsMessagesPreview | 92a14f7 |
| 2 | AGENTS.md (landed in the Task 1 commit, with the change it describes) | 1a9d827 |

## What plans 21-10 and 21-11 use

| Kind | Name |
|------|------|
| Table | `settingsMsgEvent.Default eventDefaultReply` (ban `banDefaultReply`, tban `tbanDefaultReply`, kick `kickDefaultReply`, mute `muteDefaultReply`, tmute `tmuteDefaultReply`, unban `unbanDefaultReply`, unmute `unmuteDefaultReply`) and `.Timed` (tban, tmute) |
| Preview | `settingsMsgPreview(ev)` (the Preview hook), `settingsMsgPreviewReply(p, groupTr, ev)` (the reply with no options), `settingsMsgSampleDuration` (`2 hours`), `settingsMsgSampleSpan` (2 h); every definition's `PreviewHeaderKey` is `settings_msg_preview_header` |
| Actions | `settingsMsgEditActions()` now returns `hs hr hc hv hf`; `hv` fields exactly `t` and `k`, `hf` exactly `k`, both `settingsRightAdmin`; 42 and 38 bytes at a 14-character chat ID |
| Screen | `renderSettingsMsgEvent` draws `settingsEditPreviewRows` after `settingsEditPartRows`: rows `[1 1 1 1 1 2]` with nothing stored, `[2 1 1 2 1 2]` with a text, `[2 2 2 2 2 1 2]` with every part set (seven rows, the worst case) |
| Strings | `settings_msg_preview_header` ({130, 182}), `settings_msg_sample_reason` ({30, 42}); 531 `settings_` lines per locale file |
| Test helpers (`settings_messages_preview_test.go`) | `settingsMsgPreviewSeen(t, env, user, code, part)` (answer, alert and every send to the presser since the press), `settingsMsgPreviewContent` (requires the header plus one content message and returns its method and decoded params), `settingsMsgSentKeyboard(t, params)` (the inline keyboard of decoded params) |
| Widened helper | `settingsMsgWantSilentScreen` now asserts three part rows, one or two preview rows ending with Full preview, the toggle (second to last row) and Back and Close |

### Notes for plan 21-10 (safeguards)

- The event screen is now at most seven rows (`[2 2 2 2 2 1 2]`); a length and row test over every event screen in 8
  languages should use the full state (text, media, buttons, Silent on) and a 64-rune title.
- The preview header (`settings_msg_preview_header`, 137 code points in Spanish) and the sample reason are in the limits
  table; a privacy sweep (`TestSettingsStaffPrivacy`) of the previews could press `hv`/`hf` in a private menu of an
  administrator who is not the creator and assert nothing names the Staff Group (the preview names only the presser and
  the group's stored content).
- `TestSettingsGateAllDoors` was not extended with a preview door; the previews run the same press pipeline (owner gate at
  step 4b), so a column there would only repeat the press door.
- A forged `hv`/`hf` (group menu `u`, unknown `k`, unknown `t`, an extra field) is refused in step 1 by
  `settingsEditResolvedActions`' Validate; no test of this plan presses a forged preview, so 21-10's forged-press sweep
  should add `hv` and `hf` beside `hs hr hc hq`.
- A `{rules}` token in a stored event text makes Full preview and Preview text read the group's rules through
  `FormattingReplacer`, which creates an empty rules row for a group without one (the send path's own side effect); the
  tests of this plan use no `{rules}`.

## Verification

- `go test -tags testtools -race -count=1 -run '^(TestSettingsMessagesPreview)$' -v ./fuku/modules`: PASS, 8 subtests, no SKIP.
- `go test -tags testtools -race -count=1 -run '^(TestSettingsMessagesPreview|TestSettingsMessagesSet|TestSettingsMessagesRemove|TestSettingsMessagesOpens|TestSettingsMessagesSilent)$' -v ./fuku/modules`: all PASS.
- `go test -tags testtools -race -count=1 -timeout 30m -run '^(TestSettingsMessages.*|TestSettingsAuthorityMessages.*|TestSettingsDriftMessages.*|TestSettingsPhase1[89]Codes|TestSettingsGateAllDoors|TestSettingsStaffPrivacy|TestSettingsPageActionValidate|TestSettingsControlActionAllowList|TestSettingsEditResolvedDefs|TestSettingsEditReservedButtons|TestSettingsListForged|TestSettingsMainGrid|TestEventMessage.*)$' ./fuku/modules`: ok (76 s).
- `go test -tags testtools -race -count=1 -timeout 30m -run '^(TestSettingsEdit.*Preview.*|TestSettingsEditFullPreview.*)$' -v ./fuku/modules`: 9 tests PASS (the Welcome, Goodbye and Rules previews are unchanged).
- `go test -tags testtools -race -count=1 -run '^(TestSettingsLocaleKeys|TestLocaleStyle|TestLocalePlaceholdersMatchOtherLanguages|TestSettingsLabelLengths|TestSettingsReusedKeyPlaceholders|TestSettingsEditLocaleTokens)$' ./fuku/i18n`: ok; `go test -tags testtools -race -count=1 -run 'Settings|Locale' ./fuku/i18n`: ok.
- `make check-translations`: all present. `make check-docs`: no drift.
- `go vet -tags testtools ./fuku/modules ./fuku/i18n` clean; `CGO_ENABLED=0 go build ./...` ok; `gofmt -l` clean on every file this plan touched.
- Mutation check: with `settingsMsgPreviewReply` passing the default's options (so the bot's row is composed), three
  assertions of `TestSettingsMessagesPreview` fail (the ban's Unban row, the mute's Unmute row before the Rules button,
  the Spanish default's Unban row). Reverted.
- Acceptance greps: `func settingsMsgPreview(` 1, `composeEventMessage(` 1 and `settingsEditResolvedActions(` 1 in
  `settings_messages.go`; 531 `settings_` lines in every locale file; `event_message.go` has no diff and no `(21-09)`
  commit; `composeEventMessage` 2 and `settings_msg_sample_reason` 1 in AGENTS.md.

## Not verified

- `make lint` was not run (the installed golangci-lint cannot load the Go 1.26 module), as instructed; `unused` and
  `ineffassign` were checked by hand (every new production identifier has a production caller; no assignment in the new
  test is overwritten unread).
- The whole `fuku/modules` package was not run, as instructed.
- No PostgreSQL run: nothing here rests on a constraint.
- The real Telegram rendering of a preview (a photo with a caption and a URL button in a private chat) is not checked live.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Expectations of earlier tests] The event screen's rows**
- `TestSettingsMessagesOpens` (`[2 1 1 1 2]`, Back at row 4), `TestSettingsMessagesSet` and `TestSettingsMessagesRemove`
  (`[1 1 1 1 2]`, `[2 2 2 1 2]`, `[1 2 2 1 2]`) and `settingsMsgWantSilentScreen` (five rows, toggle at row 3) expected the
  screen without preview rows; they now expect `[2 1 1 2 1 2]` (Back at row 5), `[1 1 1 1 1 2]`, `[2 2 2 2 2 1 2]`,
  `[1 2 2 2 1 1 2]`, and the helper asserts the preview rows between the part rows and the toggle, as plan 21-08's notes said.
- **Commit:** 1a9d827

**2. Commit split**
- The plan put the `feat(21-09): preview an event message before it is posted` subject on Task 2 (AGENTS.md). AGENTS.md
  lands in the feature commit (the rule that it changes with the code it describes), together with the strings and the
  row expectations that the feature changes; the new test file is its own `test` commit.

**3. Two environments in the test**
- The plan's behavior said one environment. The default-text check needs the real locale (the marker locale drops the
  `%s` verbs of the default builders, so their output would carry `%!(EXTRA ...)`); that subtest builds its own
  real-locale settings environment with the presser in English and the group in Spanish, and everything else uses the
  marker environment.

## Known Stubs

None.

## Threat Flags

None: T-21-28 (no bot row in a private preview), T-21-29 (the target is always `query.From.Id`, the test asserts no send
to the group) and T-21-30 (no row, queue or wait change, asserted) are mitigated as planned.

## Self-Check: PASSED

- FOUND: fuku/modules/settings_messages_preview_test.go, fuku/modules/settings_messages.go
- FOUND commits: 1a9d827, 92a14f7, both ancestors of HEAD
