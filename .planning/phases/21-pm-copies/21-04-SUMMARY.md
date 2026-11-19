---
phase: 21-pm-copies
plan: 04
subsystem: settings-messages-page
status: complete
tags: [pm-copies, settings-menu, message-editor, callback-data, rights]
requires: [21-02, 21-03]
provides:
  - settingsMsgDestField (d), settingsMsgDestPMCode (p), settingsActMsgPM (hp), settingsMsgPMIcon
  - settingsMsgPMWaitPage(ev) (h<code>p), settingsMsgPMEditDef(fam, ev), settingsMsgPMDefOf(code)
  - settingsMsgDefByFields resolving k and d; settingsMsgPMFields; settingsMsgRunPM; settingsMsgRedrawApplied
  - settingsMsgStateFor(msgs, ev, dest); settingsMsgLoad/Remove/Write(ev, dest); settingsMsgPMDraw; renderSettingsMsgPM
  - settingsMsgPartLines, settingsMsgSwitchRow (shared by the event screen and the private copy screen)
  - settingsEditState.PMOn
  - applyEventPM; applyEventText/Media/Buttons and eventMsgApplyReadBack with a destination; eventMsgApplyResult.Messages
  - six settings_ strings (settings_msg_pm_title, _pm_intro, _line_pm, _hint_pm, settings_btn_msg_pm, _pm_open) in 8 languages
  - TestSettingsMessagesPMScreen, TestSettingsMessagesPMSwitch, TestSettingsMessagesPMTyped, TestSettingsDriftMessagesPM, TestSettingsAuthorityMessagesPM
  - toolkit: settingsMsgStoredPM, settingsMsgOpenPM, settingsMsgSetPMWait, settingsMsgPMLine, settingsMsgSendPathPM, settingsMsgPMSwitch, withoutEventPMSpec, settingsMsgPMOffState
affects: [21-05, 21-06, 21-07, 21-08, 21-09]
tech-stack:
  added: []
  patterns:
    - "one set of editor action codes serving two destinations, resolved by a callback field that is absent for the old buttons"
    - "register a definition unconditionally at init and gate it at press time, so no init order across files is relied on"
    - "redraw from the whole fresh read the apply function kept, so a screen that shows two destinations keeps both after a write"
key-files:
  created:
    - fuku/modules/settings_messages_pm_test.go
  modified:
    - fuku/modules/settings_messages.go
    - fuku/modules/event_message_apply.go
    - fuku/modules/settings_edit.go
    - fuku/modules/settings_messages_env_test.go
    - fuku/modules/settings_messages_test.go
    - fuku/modules/settings_messages_edit_test.go
    - fuku/modules/settings_messages_silent_test.go
    - fuku/modules/settings_messages_screens_test.go
    - fuku/i18n/settings_locale_test.go
    - locales/en.yml, es.yml, fr.yml, hi.yml, id.yml, pt.yml, ru.yml, uk.yml
    - AGENTS.md
decisions:
  - "The private copy definitions are registered for every event, not only for events with a spec: eventPMSpecs is filled by event_message_pm.go's init and init order across files is not relied on (registerSettingsEditDef's own rule). The spec is required at press and draw time by settingsMsgPMDefOf, settingsMsgDefByFields and renderSettingsMsgEvent."
  - "settings_msg_hint_pm leaves out the plan's last sentence (the Details button and the log line are set on the Messages page): those options arrive in plans 21-06 and 21-07, so the sentence would be false until then."
  - "AGENTS.md is updated in the feat commit with the production change (the user's rule), so task 3's commit is test(21-04) with the test families; the plan put the AGENTS.md edit and the feat subject in task 3."
  - "settingsMsgPMDefOf returns the definition and the event only (no family): no caller needs the family, and unparam would flag an unused result."
  - "applyEventSilent keeps the group destination in a local variable so event_message_apply.go names eventmsg.DestGroup once, as the acceptance grep asks."
  - "The screens test also renders every private copy screen, prompt and refusal (in settingsMsgScreens), so the existing 8-language length test, the callback bytes test and TestSettingsStaffPrivacy cover the new screens."
metrics:
  duration: ~70 min
  completed: 2026-11-19
actuals:
  tokens: 34271
  tasks: 3
  commits: 3
plan_head_before: e3eedb8b74b0ab981468e63e8f2a0583986913c1
plan_head_after: 8bb4ccd264e5ea5b56be61eb6c8c0e91ecb1eb26
---

# Phase 21 Plan 04: Private copy on the Messages page Summary

From the bot's private settings menu the owner now opens each event's private copy, sets its text, one media item and URL buttons by
typing in private (the editor and the typed-reply door unchanged, the wait page `h<code>p`), and switches it on or off with `hp`,
which writes the pm row's Silent through `applyEventPM`. The event screen shows the private copy's state and opens its screen through
the new field `d=p`; no Phase 20 button changed its bytes, every change needs `can_change_info` judged live, and what the page writes
is what `/ban` sends in private at the next command.

## What later plans use

| Kind | Name |
|------|------|
| Constants | `settingsMsgDestField` = `d`, `settingsMsgDestPMCode` = `p`, `settingsActMsgPM` = `hp`, `settingsMsgPMIcon` = 📩 |
| Definitions | `settingsMsgPMWaitPage(ev)`, `settingsMsgPMEditDef(fam, ev)` (no `Preview` hook yet: plan 21-05 adds it and `PreviewHeaderKey`), `settingsMsgPMDefOf(code) (def, ev, ok)` (false without a spec) |
| Resolution | `settingsMsgDefByFields(fields)`: `c` required, no `d` the group definition, `d=p` the private copy (spec required), any other `d` false; `he` and the five editor actions allow `k` and `d` |
| State | `settingsMsgStateFor(msgs, ev, dest)` (`Resolve`, plus `PMOn` on the group state for an event with a spec); `settingsEditState.PMOn` |
| Screens | `renderSettingsMsgEvent` (private copy line after the Silent line, the open row after the Silent toggle, both only with a spec); `renderSettingsMsgPM(v, fam, ev, def, st, opts)` (no preview rows yet); `settingsMsgPartLines`, `settingsMsgSwitchRow` |
| Writes | `applyEventPM(ctx, chatID, event, on, by)`; `applyEventText/Media/Buttons(ctx, chatID, event, dest, ...)`; `eventMsgApplyReadBack(ctx, chatID, event, dest, what, writeErr)`; `eventMsgApplyResult.Messages`; `settingsMsgRedrawApplied` (shared by `hq` and `hp`) |
| Toolkit | `settingsMsgStoredPM`, `settingsMsgOpenPM(t, m, code)` (env file); `settingsMsgSetPMWait`, `settingsMsgPMLine(on)`, `settingsMsgSendPathPM`, `settingsMsgPMSwitch`, `withoutEventPMSpec` (pm test file); `settingsMsgPMOffState` (screens file) |

## Tasks

| Task | Name | Commit |
|------|------|--------|
| 1 | the private copy's strings in 8 languages, the two updated intros, their limits rows, label groups and token checks | dbb0f10 |
| 2 | the definitions, the screen, the switch `hp`, the event screen's line and row, the apply functions by destination, `TestSettingsMessagesPMScreen`, `TestSettingsMessagesPMSwitch`, the expected-set edits and AGENTS.md | 662c9fe |
| 3 | `TestSettingsMessagesPMTyped`, `TestSettingsDriftMessagesPM`, `TestSettingsAuthorityMessagesPM` and the AGENTS.md test families | 8bb4ccd |

## Verification

- `go test -tags testtools -race -count=1 -run '^(TestSettingsLocaleKeys|TestSettingsLabelLengths|TestSettingsEditLocaleTokens|TestSettingsReusedKeyPlaceholders|TestLocaleStyle|TestLocalePlaceholdersMatchOtherLanguages|TestSettings.*)$' ./fuku/i18n`: ok (task 1 gate; the first run failed on the two updated intros' old ceilings, raised to `{285, 336}` and `{230, 270}`, the longest new translations).
- `make check-translations`: all present; `make check-docs`: no drift.
- Task 2 gate `go test ... -run '^(TestSettingsMessagesPMScreen|TestSettingsMessagesPMSwitch|TestSettingsMessages.*|TestSettingsDriftMessages.*|TestSettingsAuthorityMessages.*|TestSettingsEditResolvedDefs|TestSettingsEditReservedButtons|TestSettingsPhase1[89]Codes|TestSettingsControlActionAllowList|TestSettingsMainGrid)$' ./fuku/modules`: ok.
- Task 3 gate `go test ... -run '^(TestSettingsMessagesPMTyped|TestSettingsDriftMessagesPM|TestSettingsAuthorityMessagesPM)$' ./fuku/modules`: ok.
- Final run `go test -tags testtools -race -count=1 -run '^(TestSettingsMessages.*|TestSettings(Authority|Drift)Messages.*|TestMessagesEndToEnd|TestEventPM.*|TestPMCopy.*|TestSettingsPhase1[89]Codes|TestSettingsGateAllDoors|TestSettingsStaffPrivacy|TestSettingsEditResolvedDefs|TestSettingsEditReservedButtons|TestSettingsControlActionAllowList|TestSettingsMainGrid|TestCommandRepliesStayInsideTheHandler|TestLegacyModerationRequests)$' ./fuku/modules`: ok (102 s), no FAIL or SKIP; the i18n settings tests ok (13 s).
- golangci-lint v2.13.1: `run --new-from-rev=9940d97 ./...` 0 issues; with `--build-tags=notesttools` 0 issues.
- `CGO_ENABLED=0 go build ./...` ok; `gofmt -l` empty on every Go file changed since 9940d97.
- Acceptance: six new keys in each of the 8 locale files; `settings_msg_pm_title` in the limits table; `func renderSettingsMsgPM(`, `func applyEventPM(`, `registerSettingsEditDef(settingsMsgPMEditDef(` once each; `eventmsg.DestGroup` once in `event_message_apply.go`; `git diff --exit-code 9940d97 -- settings_input_door.go settings_edit_actions.go settings_gate_test.go settings_pages.go` exits 0; `event_message.go`, `moderation.go`, `bans.go` and `mute.go` unchanged since 21-03; the five PM tests once each in `settings_messages_pm_test.go`; `applyEventPM` and `hbnp` in AGENTS.md.
- Mutation proof: with `hp`'s right set to `settingsRightAdmin`, `TestSettingsAuthorityMessagesPM` failed ("an administrator without can_change_info": an empty answer, one edit and a pm row written); reverted with `git checkout -- fuku/modules/settings_messages.go` before any commit, then the test passed again.

## Deviations from Plan

### Auto-fixed Issues

None: no bug was found in existing code.

### Other deviations

**1. [Robustness] PM definitions registered for every event, gated at press time**
- **Issue:** the plan registers them "for every event with a spec (`eventPMSpecFor`)" in `settings_messages.go`'s `init`, but the spec table is filled by `event_message_pm.go`'s `init`; that only works through Go's lexical file order, which `registerSettingsEditDef`'s comment says is not relied on.
- **Fix:** every event registers its private copy definition; `settingsMsgPMDefOf` (used by `settingsMsgDefByFields`, `settingsMsgPMFields`, `settingsMsgRunPM`) and the event screen's renderer require the spec when they run. `TestSettingsMessagesPMScreen` "unknown d" removes the unban spec (`withoutEventPMSpec`) and proves `he&d=p`, `hs&d=p` and `hp` are invalid requests before any lookup and the unban screen draws neither the line nor the row.

**2. [Text] The private copy hint drops its last sentence**
- "The Details button and the log line are set on the Messages page" would be false until plans 21-06 and 21-07 add those toggles. The hint ends after "I never send it for raid bans or Staff Group actions." in all 8 languages.

**3. [Commits] AGENTS.md lands with the production change**
- The user's rule (AGENTS.md in the same commit as the change it describes) moves the Messages page, `settings|v1`, Permissions and Hooked definitions edits into the task-2 commit, whose subject is the plan's `feat(21-04): set a private copy from the Messages page`; task 3 is `test(21-04)` with the test families.

**4. [Test] The over-4096 refusal key**
- A raw text of 4097 code points is refused by the content extraction with `content_text_too_long` (the door's first check), not `settings_edit_refuse_too_long_filled`; `TestSettingsMessagesPMTyped` asserts the key the door sends.

**5. [Toolkit] `settingsMsgOpenPM(t, m, code)` takes the menu, not `(e, menu, code)`**
- The menu already carries the environment.

### Expected-set edits of Phase 20 tests (widened, no other change)

- `TestSettingsMessagesOpens`: the ban screen's widths `[2 1 1 2 1 2]` to `[2 1 1 2 1 1 2]`, Back's row index 5 to 6.
- `TestSettingsMessagesSet` and `TestSettingsMessagesRemove`: four width lists gain the private copy row (a `1` before the final `2`).
- `settingsMsgWantSilentScreen` (used by `TestSettingsMessagesSilent`, `TestSettingsDriftMessagesSilent`, `TestSettingsMessagesSet`, `TestSettingsAuthorityMessagesEdit`): 7 to 8 rows, the toggle at `last-2`, the private copy row at `last-1`.
- `settingsMsgScreens`: event screens `[1 1 1 1 1 1 2]` and `[2 2 2 2 2 1 1 2]`, and the private copy screens (`[1 1 1 1 2]`, `[2 2 2 1 2]`), prompts and refusals added; `settingsMsgScreenFields` gains `d`; `TestSettingsMessagesCallbackBytes`' table: `he` 38 to 42, `hs hr hc` 42 to 46 (with `d=p`), `hp` 42 new.
- `fuku/i18n/settings_locale_test.go`: `settings_msg_intro` `{240, 336}` to `{285, 336}`, `settings_msg_fam_manual_intro` `{160, 224}` to `{230, 270}`.

No `TestLegacy*Requests` golden and no `TestSettingsDeletionPhase16ScreensUnchanged` digest was touched.

## Not verified

- The whole `fuku/modules` package was not run (targeted runs only, as instructed); `make test-shard-check` and `make test` were not run.
- `make lint` itself was not run; the CI-equivalent `golangci-lint run --new-from-rev=9940d97`, with and without `notesttools`, was.
- No PostgreSQL run: nothing of this plan rests on a constraint (the pm row's default Silent is Phase 21 plan 21-01's, held by `TestEventMessagesUpsert`).
- The real locales' rendering of the new screens is covered only by `TestSettingsMessagesPageFitsEveryLocale` over pure renders; no dispatcher test runs the private copy screen with real locales.

## Notes for plan 21-05 (previews and hub mark)

- Add the `Preview` hook and `PreviewHeaderKey` (`settings_msg_preview_header_pm`) to `settingsMsgPMEditDef`; it must compose through `composeEventPM` with the presser as member and admin and send only to `query.From.Id`, never `offerEventPM` (`TestPMCopyCallers`). Today a forged `hv`/`hf` with `d=p` reaches the editor's `settingsEditPreviewSend`, which returns `errSettingsEditNoPreview` before any send because the hook is nil.
- `renderSettingsMsgPM` draws no preview rows: add `settingsEditPreviewRows(k, def, st)` after `settingsEditPartRows`. Note `settingsEditPreviewable` treats an event's text as previewable only while set, so with nothing stored only Full preview is drawn: the screen becomes `[1 1 1 1 1 2]` empty and `[2 2 2 2 2 1 2]` full. Update `settingsMsgScreens` (the two `pm ...` widths) and `TestSettingsMessagesPMScreen`'s `[1 1 1 1 2]` and its row indexes (switch at row 3, Back at row 4), and `TestSettingsMessagesCallbackBytes` (`hv` with `d=p` is 46, `hf` with `d=p` 42).
- The hub and family list still use `settingsMsgStateOf` (group only); the ` · 📩` suffix and Main's count need `msgs.Resolve(ev.Key, eventmsg.DestPM).Silent` (the same read `settingsMsgStateFor` makes for `PMOn`).
- `settings_msg_hint_pm` is the place to add "The Details button and the log line are set on the Messages page" once 21-06 and 21-07 add the hub toggles.

## Known Stubs

None.

## Threat Flags

None: no surface beyond the plan's threat model. T-21-15 (`TestSettingsAuthorityMessagesPM`, the mutation proof), T-21-16 (`TestSettingsMessagesPMScreen` "unknown d", including an event without a spec), T-21-17 (`TestSettingsMessagesPMTyped`: the `javascript:` link, the over-limit text and the slash text keep the wait and store nothing) and T-21-18 (`TestSettingsAuthorityMessagesPM` "a group menu's presses are refused in step 1") are mitigated and tested.

## Self-Check: PASSED

- Files: `fuku/modules/settings_messages_pm_test.go` exists; the modified files carry their edits.
- Commits: dbb0f10, 662c9fe and 8bb4ccd are ancestors of HEAD.
