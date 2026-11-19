---
phase: 20-message-engine-messages-page-and-manual-punishments
plan: 07
subsystem: settings menu / Messages page
status: complete
tags: [settings, messages-page, event-messages, silent, MSG-07, MSG-08]
requires: [20-04, 20-06]
provides:
  - the Silent switch on every event screen (page action hq, fields k and v)
  - applyEventSilent, eventMsgWrites, eventMsgApplyResult in fuku/modules/event_message_apply.go
  - settingsMsgStateFromMessage (the editor state of one stored event message)
  - 3 settings_ locale keys in 8 languages
affects: [20-08, 20-09, 20-10, 20-11]
tech-stack:
  added: []
  patterns:
    - "a switch with no command: one apply function with a write seam, a drift test and an authority test in place of a parity test"
    - "the absolute target in the toggle's v, so a double tap or a stale screen writes the same value"
key-files:
  created:
    - fuku/modules/event_message_apply.go
    - fuku/modules/settings_messages_silent_test.go
  modified:
    - fuku/modules/settings_messages.go
    - fuku/modules/settings_messages_test.go
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
  - "applyEventSilent reuses greetings.go's generic applyReadBack (write, then a fresh read on context.WithoutCancel under greetingApplyTimeout), so the outcome type is greetingApplyOutcome as the plan asked and the failure logging matches the greeting apply functions."
  - "The read-back answers a nil Row both when the read failed and when the event has no row after the write (it cannot happen after a successful upsert, but a nil Row only makes the caller reload)."
  - "settingsMsgStateFromMessage was factored out of settingsMsgLoad so the press redraws from the read-back row with exactly the state the Load hook builds."
  - "The Permissions bullet of AGENTS.md was updated in the feat commit (the commit that adds the right's new use), not in the test commit as the plan's task split put it, so the rule and the change land together."
metrics:
  duration: ~35 min (this executor; the killed executor's earlier time is not counted)
  completed: 2026-11-19
actuals:
  tokens: 12000
  tasks: 2
  commits: 2
plan_head_before: 5ef4b33
plan_head_after: 9d88bfa
---

# Phase 20 Plan 07: The Silent switch Summary

Every event screen of the private Messages page has a Silent line, its hint and a full-width toggle
(`hq`) that needs `can_change_info`; the press writes only through `applyEventSilent` and the repository's
`eventmsg.SetSilent`, which drops `event_messages:<chat>` (in `skipLocal`), so the next command of that event
on every replica posts nothing in the group.

## Tasks

| Task | Name | Commit |
|------|------|--------|
| 1 | The Silent switch on every event screen, written through applyEventSilent; 3 strings in 8 languages; AGENTS.md | b539e9c |
| 2 | The drift and authority tests of the switch; AGENTS.md names them | 9d88bfa |

The first executor of this plan was killed by a container restart with Task 1's production code, strings and
limits rows uncommitted in the tree. This executor audited that work against the plan, kept it (see below),
wrote the tests, fixed the one existing test the new row broke and committed both tasks.

## What plans 20-08 to 20-11 use

| Kind | Name |
|------|------|
| Action | `settingsActMsgSilent` = `hq`: fields exactly `k` and `v`, `Right` `settingsRightChangeInfo`, `Validate` `settingsMsgSilentFields` (= `settingsMsgEventFields` (the private owner field `c` and a known event) and `settingsEditSwitchValue(v)`), `Run` `settingsMsgRunSilent` |
| Apply | `applyEventSilent(ctx, chatID int64, event string, on bool, by int64) eventMsgApplyResult` (`event_message_apply.go`): writes through `eventMsgWrites.Silent` (default `eventmsg.SetSilent`, `eventmsg.DestGroup`), reads back with `eventmsg.GetFresh` |
| Seam | `var eventMsgWrites = struct{ Silent func(ctx, chatID, event, dest string, silent bool, by int64) error }` — plan 20-08 adds its `Text`, `Media` and `Buttons` writers to the same struct |
| Result | `eventMsgApplyResult{Outcome greetingApplyOutcome; Row *eventmsg.Message; Err error}`: `greetingSaveFailed` with the error and a nil `Row` for a failed write; `greetingApplied` with a nil `Row` when the read-back failed (the caller reloads) |
| State | `settingsMsgStateFromMessage(msg eventmsg.Message) settingsEditState` (the zero Message reads as the default text, no media, no buttons, Silent off); `settingsMsgLoad` now calls it |
| Screen | `renderSettingsMsgEvent`: body ends with `settings_msg_line_silent` (`settingsStateWord`), a blank line and `settings_msg_hint_silent`; keyboard is the toggle row (`settings_btn_msg_silent` with `settingsStateIcon`, data `a=hq&c=<chat>&k=<code>&v=<opposite of stored>`, 42 bytes at a 14-character chat ID), then Back (`a=hl&t=<family>`) and Close: rows `[1 2]`. Plan 20-08's part rows and 20-09's previews go above the Silent row or between it and Back as their UI-SPEC rows say |
| Strings | `settings_msg_line_silent` (`Silent: {state}`, limits {20, 28}), `settings_btn_msg_silent` (`{state} Silent`, {20, 26}, rendered with each icon at most 40 in `TestSettingsLabelLengths`'s new `iconToggles` group), `settings_msg_hint_silent` ({90, 126}); both `{state}` keys are in `TestSettingsEditLocaleTokens`. Each locale file now has 527 `settings_` lines |
| Test helpers | in `settings_messages_silent_test.go`: `settingsMsgSilentStored(t, chatID, event) (silent, by, ok)` (straight from the database), `settingsMsgSilentToggle(t, keyboard)`, `settingsMsgWantSilentScreen(t, text, keyboard, code, on)` (asserts rows `[1 2]`; plan 20-08 must widen it when the part rows arrive), `settingsMsgSendPathSilent(t, chatID, event)` (the send path's cached `eventMessagesLookup`) |

## The inherited work: kept or changed

- `fuku/modules/event_message_apply.go` (untracked): kept as written. It matches the plan's assumptions (one write
  path, no rights judged, `eventmsg.SetSilent` with `DestGroup`, fresh read-back, `greetingSaveFailed` / `greetingApplied`
  with a nil `Row`), and every identifier has a production caller.
- `fuku/modules/settings_messages.go`: kept. The action registration, `settingsMsgSilentFields`, `settingsMsgRunSilent`
  (redraw from the read-back through `settingsEditDraw`, reload through `settingsEditRedraw` when the row is missing,
  save-failed notice on a failed write) and the Silent block of `renderSettingsMsgEvent` are what the plan describes.
- The 8 locale files and `fuku/i18n/settings_locale_test.go` (limits rows, the `iconToggles` group, the two token rows):
  kept; the i18n tests and `make check-translations` pass on them.
- Added by this executor: `settings_messages_silent_test.go` (all three tests), the `TestSettingsMessagesOpens`
  expectation update, and both AGENTS.md edits.

## Verification

- `go test -tags testtools -race -count=1 -run '^(TestSettingsMessagesSilent|TestSettingsMessages.*)$' -v ./fuku/modules`: all PASS (Silent with its two subtests, Opens, MainState, GroupMenu, StaffGroup), no SKIP.
- `go test -tags testtools -race -count=1 -run '^(TestSettingsDriftMessagesSilent|TestSettingsAuthorityMessagesSilent|TestSettingsMessagesSilent)$' -v ./fuku/modules`: all PASS, six authority subtests, no SKIP.
- Mutation check: with the action's right changed to `settingsRightAdmin`, `TestSettingsAuthorityMessagesSilent` fails; reverted.
- `go test -tags testtools -race -count=1 -run '^(TestSettingsMessages.*|TestSettingsDriftMessagesSilent|TestSettingsAuthorityMessagesSilent|TestSettingsPhase18Codes|TestSettingsPhase19Codes|TestSettingsGateAllDoors|TestSettingsStaffPrivacy|TestSettingsControlActionAllowList|TestSettingsMainGrid|TestSettingsEdit.*Resolved.*|TestSettingsEditDefs.*)$' ./fuku/modules`: ok (60 s).
- `go test -tags testtools -race -count=1 -run '^(TestSettingsLocaleKeys|TestSettingsLabelLengths|TestSettingsEditLocaleTokens|TestLocaleStyle|TestLocalePlaceholdersMatchOtherLanguages|TestSettings.*)$' ./fuku/i18n`: ok.
- `make check-translations`: all present. `make check-docs`: no drift (no generated doc changed).
- `go vet -tags testtools ./fuku/modules ./fuku/i18n` clean; `CGO_ENABLED=0 go build ./...` ok; `gofmt -l` clean on every file this plan touched (it lists 16 files this plan did not touch, untouched here).
- Acceptance greps: `func applyEventSilent(` 1 in the apply file, `applyEventSilent(` 1 in `settings_messages.go`, `eventmsg.SetSilent` 1 in the apply file, 527 `settings_` lines in every locale file, each test function once, `applyEventSilent` in AGENTS.md.

## Not verified

- `make lint` was not run (the installed golangci-lint cannot load the Go 1.26 module), as instructed; `unused` was
  checked by hand: every new production identifier has a production caller.
- The whole `fuku/modules` package was not run (over 15 minutes), as instructed; only the targeted patterns above.
- `TestSettingsGateAllDoors` was run but gains no `hq` column: the plan did not ask for one, and the authority test covers
  the owner gate's refusal and the live creator for this action.
- No PostgreSQL run: nothing of this plan rests on a constraint (the write is the 20-02 upsert already in the integrity list).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug in an existing expectation] `TestSettingsMessagesOpens` expected the event screen's old keyboard**
- **Found during:** Task 1
- **Issue:** the 20-06 test asserted the ban screen's rows as `[2]` with Back at `keyboard[0][0]`; the Silent row the plan adds makes them `[1 2]`.
- **Fix:** the expectation is now `[1 2]` with Back at `keyboard[1][0]` (and its comment names the toggle). The file is not in the plan's `files_modified`.
- **Files modified:** fuku/modules/settings_messages_test.go
- **Commit:** b539e9c

**2. AGENTS.md Permissions edit moved to Task 1**
- The plan put the `settingsRightChangeInfo` sentence in Task 2; it landed in the feat commit, with the change it describes (AGENTS.md's own rule). Task 2's commit only adds the drift and authority tests to the Messages bullet.

**3. Commit subject of Task 2**
- The plan's action text gives `feat(20-07): silence an event's group message from its event screen` for the closing commit; that subject was used for Task 1 (the feature), and Task 2, which adds tests and their AGENTS.md mention only, is `test(20-07): ...`.

## Self-Check: PASSED

- FOUND: fuku/modules/event_message_apply.go, fuku/modules/settings_messages_silent_test.go
- FOUND: b539e9c, 9d88bfa (ancestors of HEAD)
