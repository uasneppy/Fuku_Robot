---
phase: 20-message-engine-messages-page-and-manual-punishments
plan: 08
subsystem: settings menu / Messages page / message editor
status: complete
tags: [settings, messages-page, event-messages, message-editor, typed-reply-door, MSG-06, MSG-08, MSG-01]
requires: [20-03, 20-06, 20-07]
provides:
  - Set (hs), Remove (hr) and Cancel (hc) on every event screen, from settingsEditResolvedActions
  - applyEventText, applyEventMedia, applyEventButtons and their eventMsgWrites seams
  - the event definitions' Remove and Write hooks and the family's own text part
  - settings_msg_wait_text_manual and settings_msg_hint_manual in 8 languages
affects: [20-09, 20-10, 20-11]
tech-stack:
  added: []
  patterns:
    - "a hooked editor definition gets its Set, Remove and typed reply with no new door: the page registers the editor's own actions and supplies Remove and Write hooks over its apply functions"
    - "one read-back helper (eventMsgApplyReadBack) shared by every event message apply function"
key-files:
  created:
    - fuku/modules/settings_messages_edit_test.go
  modified:
    - fuku/modules/event_message_apply.go
    - fuku/modules/settings_messages.go
    - fuku/modules/settings_messages_test.go
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
  - "settingsMsgEditCodes (the five codes and settingsRightChangeInfo) is the one source of the event definitions' codes and of the page actions; settingsMsgEditDef copies it, so the two cannot drift."
  - "settingsMsgEditActions takes only hs, hr and hc from settingsEditResolvedActions; hv and hf stay unregistered until plan 20-09, and the event screen draws no preview row."
  - "renderSettingsMsgEvent now takes the definition, so the part lines and part rows come from def.Parts (the package-level settingsMsgParts list is gone)."
  - "The body adds the family hint after the Silent line with a blank line before it and another before the Silent hint, matching the editor's paragraph spacing."
  - "applyEventSilent now goes through the shared eventMsgApplyReadBack; its log lines read exactly as before."
metrics:
  duration: ~35 min
  completed: 2026-11-19
actuals:
  tokens: 20500
  tasks: 3
  commits: 3
plan_head_before: 3b6819d70696531823fa69a40b10a28187b14a27
plan_head_after: 804ee583ab642b354ba04e13165a2cd7811169a4
---

# Phase 20 Plan 08: The editor on event screens Summary

Every event screen of the private Messages page now has the v1.1 editor's part rows: Set writes the owner's one wait
for that event and part (page `h` + code + `g`, e.g. `hbng`) and turns the message into the part's prompt, Cancel ends
it, Remove clears the part at once, and the owner's next private message is judged by the editor's own check and
stored through `applyEventText`, `applyEventMedia` or `applyEventButtons`, which the send path reads at the very next
command. Every change needs `can_change_info`, judged live on every press and typed message.

## Tasks

| Task | Name | Commit |
|------|------|--------|
| 1 | Set, Cancel and Remove on every event screen through the parts' apply functions; two strings in 8 languages; AGENTS.md | 4d78d5a |
| 2 | TestSettingsMessagesTyped: the typed text, media and buttons and the editor's refusals | 481f407 |
| 3 | TestSettingsAuthorityMessagesEdit and TestSettingsDriftMessagesEdit | 804ee58 |

Before the plan, at the coordinator's request, a separate lint fix for plan 20-07: 3b6819d
`test(20-07): drop an unused assignment in the Silent switch test` (ineffassign at
`settings_messages_silent_test.go:100`, the second press's text is now `_`).

## What plans 20-09 to 20-11 use

| Kind | Name |
|------|------|
| Apply | `applyEventText(ctx, chatID, event, text, by)`, `applyEventMedia(ctx, chatID, event, fileID, mediaType, by)`, `applyEventButtons(ctx, chatID, event, buttons []db.Button, by)`, each an `eventMsgApplyResult`; `eventMsgApplyReadBack(ctx, chatID, event, what, writeErr)` (the shared fresh read-back) |
| Seams | `eventMsgWrites.Text`, `.Media`, `.Buttons` (defaults `eventmsg.SetText`, `SetMedia`, `SetButtons`, `eventmsg.DestGroup`) beside `.Silent` |
| Table | `settingsMsgFamily.WaitTextKey` and `.HintKey` (manual: `settings_msg_wait_text_manual`, `settings_msg_hint_manual`) |
| Parts | `settingsMsgTextPart(fam)` (editor text part with the family's prompt and `settings_edit_removed_text_default`), `settingsMsgPartsOf(fam)` (`x m b`) |
| Codes | `settingsMsgEditCodes` (the five codes and `EditRight` `settingsRightChangeInfo`), copied by `settingsMsgEditDef` |
| Actions | `settingsMsgEditActions()`: `hs`, `hr`, `hc` from `settingsEditResolvedActions(settingsMsgEditCodes, []string{"k"}, settingsMsgDefByFields)`; fields exactly `t` and `k` plus the owner field `c`; Set and Remove `settingsRightChangeInfo`, Cancel `settingsRightAdmin` |
| Hooks | `settingsMsgRemove(ev)`, `settingsMsgWrite(ev)`, `settingsMsgApplied(res)` (outcome and editor state, nil to reload) |
| Screen | `renderSettingsMsgEvent(v, fam, ev, def, st, opts)`: part lines from `def.Parts`; body ends with the Silent line, a blank line, `fam.HintKey`, a blank line, `settings_msg_hint_silent`; keyboard `settingsEditPartRows`, the Silent toggle, Back and Close: rows `[1|2 1|2 1|2 1 2]` |
| Strings | `settings_msg_wait_text_manual` (limits {600, 840}; literal `<code>{…}</code>` tokens and `{max}` in `TestSettingsEditLocaleTokens`, `{id}` allowed in `TestSettingsReusedKeyPlaceholders`), `settings_msg_hint_manual` ({180, 252}); 529 `settings_` lines per locale file |
| Test helpers (`settings_messages_edit_test.go`) | `settingsMsgStored(t, chatID, event)` (the row from the database), `settingsMsgSeedFull` (text, photo, one button), `settingsMsgPartButton(t, keyboard, action, part)`, `settingsMsgSendPathText(t, chatID, event)`, `settingsMsgSetWait(t, env, user, code, part)`, `settingsMsgLastSent(t, env, user)` |
| Widened helper | `settingsMsgWantSilentScreen` now asserts rows `[1|2 1|2 1|2 1 2]`: three part rows whose first button is `hs`, then the toggle (row 3), then Back and Close. Plan 20-09's preview rows will need it widened again |

### Notes for plan 20-09 (previews)

- Register `hv` and `hf` by adding `settingsActMsgPreview` and `settingsActMsgFull` to the code list in
  `settingsMsgEditActions` (`settingsEditResolvedActions` already builds them with `settingsRightAdmin` and the field `k`;
  `hf` has fields exactly `k`).
- Add a `Preview` hook and a `PreviewHeaderKey` to `settingsMsgEditDef`'s hooks; without a `Preview` hook every preview is
  `settings_edit_preview_failed` with nothing sent.
- `renderSettingsMsgEvent` draws no preview rows yet: call `settingsEditPreviewRows(k, def, st)` after
  `settingsEditPartRows`. `settingsEditPreviewable` treats only Welcome and Goodbye as having a previewable default text
  (`def.isGreeting()`), so an event's text Preview is offered only while a text is stored; the plan decides whether the
  default text should be previewable too.
- `settingsMsgWantSilentScreen` and the width assertions in `settings_messages_test.go` and `settings_messages_edit_test.go`
  (`[1 1 1 1 2]`, `[2 1 1 1 2]`, `[2 2 2 1 2]`, `[1 2 2 1 2]`) change when the preview rows arrive.

## Verification

- `go test -tags testtools -race -count=1 -run '^(TestSettingsMessagesSet|TestSettingsMessagesRemove|TestSettingsMessagesTyped|TestSettingsAuthorityMessagesEdit|TestSettingsDriftMessagesEdit)$' -v ./fuku/modules`: all PASS, 9 `TestSettingsMessagesTyped/` subtests, 5 authority subtests, no SKIP.
- `go test -tags testtools -race -count=1 -run '^(TestSettingsMessages.*|TestSettingsAuthorityMessagesEdit|TestSettingsDriftMessagesEdit|TestSettingsGateAllDoors)$' ./fuku/modules`: ok.
- `go test -tags testtools -race -count=1 -timeout 30m -run '^(TestSettingsMessages.*|TestSettingsAuthorityMessages.*|TestSettingsDriftMessages.*|TestSettingsPhase1[89]Codes|TestSettingsGateAllDoors|TestSettingsStaffPrivacy|TestSettingsPageActionValidate|TestSettingsDeletionMemberCodes.*|TestSettingsControlActionAllowList|TestSettingsMainGrid|TestSettingsEditResolvedDefs|TestSettingsEditReservedButtons|TestSettingsListForged)$' ./fuku/modules`: ok (66 s).
- `go test -tags testtools -race -count=1 -timeout 40m -run '^(TestSettingsEdit|TestSettingsInput|TestSettingsParityGreeting|TestSettingsAuthorityParityEdit|TestSettingsRules)' ./fuku/modules` (the whole editor, door and Rules families, started on the final production code): ok (266 s), no FAIL, no SKIP.
- `go test -tags testtools -race -count=1 -run '^(TestSettingsLocaleKeys|TestSettingsEditLocaleTokens|TestLocaleStyle|TestLocalePlaceholdersMatchOtherLanguages|TestSettingsReusedKeyPlaceholders|TestSettingsLabelLengths)$' ./fuku/i18n`: ok.
- `make check-translations`: all present. `make check-docs`: no drift.
- `go vet -tags testtools ./fuku/modules` clean; `CGO_ENABLED=0 go build ./...` ok; `gofmt -l` clean on every file this plan touched.
- Mutation checks: with the edit codes' right set to `settingsRightAdmin`, two `TestSettingsAuthorityMessagesEdit` subtests fail; with the ban's and mute's `ReservedButtons` set to 0, the 100-button refusal fails. Both reverted.
- Acceptance greps: 3 apply functions in `event_message_apply.go`; `settingsEditResolvedActions(` once in `settings_messages.go`; 529 `settings_` lines in every locale file; each test function once; `applyEventText` and `hbng` in AGENTS.md; `settings_input_door.go` and `settings_gate_test.go` not edited.

## Not verified

- `make lint` was not run (the installed golangci-lint cannot load the Go 1.26 module), as instructed; `unused` and
  `ineffassign` were checked by hand on the new code and tests (every new production identifier has a production caller;
  no assignment in the new tests is overwritten unread).
- The whole `fuku/modules` package was not run, as instructed.
- No PostgreSQL run: nothing here rests on a constraint (the writes are plan 20-02's upserts, already in the integrity list).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Plan conflict] The prompt's limit row**
- **Found during:** Task 1
- **Issue:** the plan's English text for `settings_msg_wait_text_manual` is 586 characters, over the plan's own ceiling of 560.
- **Fix:** the limits row is {600, 840}; the English text is the plan's, word for word. The longest translation (French) is 664.
- **Commit:** 4d78d5a

**2. [Rule 3 - Blocking] `{id}` in a second settings_ prompt**
- **Issue:** `TestSettingsReusedKeyPlaceholders` allows the literal `{id}` only in `settings_edit_wait_text`; the new prompt lists it too.
- **Fix:** the exception covers `settings_msg_wait_text_manual` as well (both are copyable code, never filled in).
- **Commit:** 4d78d5a

**3. [Rule 1 - Expectations of earlier tests] The event screen's rows**
- `TestSettingsMessagesOpens` expected the ban screen's rows `[1 2]` and `settingsMsgWantSilentScreen` asserted `[1 2]` with
  the toggle first; with the part rows they are `[2 1 1 1 2]` (a stored text) and three part rows, the toggle, Back and
  Close. Both expectations follow, as plan 20-07 said they would.
- **Commit:** 4d78d5a

**4. Tests use the marker locale, not real locales**
- The plan's behavior said `TestSettingsMessagesTyped` would use real locales for the refusal texts. Every refusal the test
  checks is a locale key the door sends through `settingsText`, so the marker locale of `newSettingsMsgEnv` asserts the exact
  key; no refusal checked is a finished command text. The test is lighter and consistent with the editor's door tests.

**5. Commit split and attribution**
- The feature, its strings, AGENTS.md and the Set and Remove tests are one `feat` commit; the typed-reply tests and the
  authority and drift tests are two `test` commits (the plan put the `feat(20-08): set an event message's text, media and
  buttons in private` subject on Task 3; it is on Task 1, where the feature lands, and AGENTS.md lands with the rule it
  changes). The commit trailers follow the session's attribution settings.

## Known Stubs

None. (The stand-in Welcome text part of plan 20-06 is replaced.)

## Threat Flags

None: T-20-24 to T-20-27 are mitigated as planned (the door's `authorize(def.EditRight)` on every typed message, the
editor's `content.ValidButtonURL`, the keyboard limits with the reserved row, logs with IDs, event and part only).

## Self-Check: PASSED

- FOUND: fuku/modules/settings_messages_edit_test.go, fuku/modules/event_message_apply.go
- FOUND commits: 4d78d5a, 481f407, 804ee58 (and the 20-07 lint fix 3b6819d), all ancestors of HEAD
