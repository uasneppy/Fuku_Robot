---
phase: 21-pm-copies
plan: 05
subsystem: settings-messages-page
status: complete
tags: [pm-copies, settings-menu, message-editor, previews]
requires: [22-04]
provides:
  - settingsMsgPMPreview(ev), the private copy definitions' Preview hook; PreviewHeaderKey settings_msg_preview_header_pm
  - settingsMsgGroupTranslator(p) (shared by the group and the private copy previews)
  - renderSettingsMsgPM draws settingsEditPreviewRows; worst [2 2 2 2 2 1 2], empty [1 1 1 1 1 2]
  - settingsMsgPMOn(msgs, ev), settingsMsgPMSuffix (" · 📩"), the new settingsMsgCountChanged
  - settings_msg_preview_header_pm in 8 languages with its limits row
  - TestSettingsMessagesPMPreview, TestSettingsMessagesPMStates
  - toolkit: settingsMsgSeedPM, settingsMsgSendCount, settingsMsgPMPreviewSeen, settingsMsgPMPreviewContent
affects: [22-06, 22-07, 22-08, 22-09]
tech-stack:
  added: []
  patterns:
    - "a preview composes with the send path's own composer, so it cannot drift from what the person receives"
    - "a state mark drawn code-side as an icon, so it needs no string"
key-files:
  created:
    - fuku/modules/settings_messages_pm_preview_test.go
  modified:
    - fuku/modules/settings_messages.go
    - fuku/modules/settings_messages_pm_test.go
    - fuku/modules/settings_messages_screens_test.go
    - fuku/i18n/settings_locale_test.go
    - locales/en.yml, es.yml, fr.yml, hi.yml, id.yml, pt.yml, ru.yml, uk.yml
    - AGENTS.md
decisions:
  - "Preview text composes with composeEventPM too (a row holding the stored text only) and sends the composed text with no keyboard, so its filling is the send path's; Full preview passes the screen's media and buttons as well."
  - "The group's translator is read by settingsMsgGroupTranslator, now shared by Phase 21's settingsMsgPreview and the new hook (same read, same timeout), so the two previews cannot read the language differently."
  - "settingsMsgStateFor reads PMOn through settingsMsgPMOn, so the event screen's line, the hub's mark and Main's count use one predicate."
  - "AGENTS.md lands with each production change (the user's rule): the previews in task 1's commit, the mark and the count in task 2's."
  - "Task 1's commit is feat(22-05) like task 2's: the plan names only task 2's subject, and task 1 adds a user-visible feature."
metrics:
  duration: ~40 min
  completed: 2026-11-19
actuals:
  tokens: 13027
  tasks: 2
  commits: 2
plan_head_before: 563abd5300f01ed5233f67d4e0dae4675cf64962
plan_head_after: 262d905d532b1f3d132490feb05bfc1a30b9a289
---

# Phase 22 Plan 05: Private copy previews and the hub mark Summary

The owner now previews an event's private copy from its screen: Preview of each set part and Full preview, sent only to the presser's
own private chat under the header `settings_msg_preview_header_pm`, built by `composeEventPM` (the composer the send path uses) with the
presser as the member and the admin, the sample reason and, for a timed event, the sample time, in the group's language. With no stored text
Full preview shows the default private text. A preview ignores the switch, writes nothing, schedules nothing and carries no Unban or Unmute
row. On the hub and the family list an event's line ends with ` · 📩` while its private copy is on, and Main counts an event once when its
group message is not Default or its private copy is on.

## What later plans use

| Kind | Name |
|------|------|
| Preview | `settingsMsgPMPreview(ev)`: `""` and `x` compose with `composeEventPM(p.b, p.group, groupTr, spec, settingsMsgPreviewReply(p, groupTr, ev), row, eventActor{User: &presser})`, `m` and `b` are `settingsEditPreviewMediaAlone` and `settingsEditPreviewButtonsAlone`; `errSettingsMsgNoPMSpec` for an event without a spec (unreachable: `settingsMsgDefByFields` refuses it in step 1) |
| Translator | `settingsMsgGroupTranslator(p)` (the group's language by chat ID, under `settingsReadTimeout`) |
| Screen | `renderSettingsMsgPM` with `settingsEditPreviewRows` after the part rows |
| States | `settingsMsgPMOn(msgs, ev)`, `settingsMsgPMSuffix`, `settingsMsgCountChanged` (group not Default or copy on, once per event) |
| Toolkit | `settingsMsgSeedPM(t, chatID, event, text, full)`, `settingsMsgSendCount(env)`, `settingsMsgPMPreviewSeen`, `settingsMsgPMPreviewContent` (`settings_messages_pm_preview_test.go`) |

## Tasks

| Task | Name | Commit |
|------|------|--------|
| 1 | the Preview hook through `composeEventPM`, the preview rows, the header string in 8 languages with its limits row, `TestSettingsMessagesPMPreview`, the widened row and byte expectations and AGENTS.md | 20d8647 |
| 2 | `settingsMsgPMOn`, the line mark, the new Main count, `TestSettingsMessagesPMStates` and AGENTS.md | 262d905 |

## Verification

- Task 1 gate `go test -tags testtools -race -count=1 -run '^(TestSettingsMessagesPMPreview|TestSettingsMessagesPreview|TestSettingsMessagesPMScreen|TestSettingsMessagesCallbackBytes|TestSettingsMessagesPageFitsEveryLocale)$' ./fuku/modules`: ok (the first run failed only on the test's own expectation of the group title: the press's group is the fake's `getChat` answer, "Test Chat", which the test now uses).
- `go test -tags testtools -race -count=1 -run '^(TestSettings.*|TestLocaleStyle|TestLocalePlaceholdersMatchOtherLanguages|TestEventMsgLocaleKeys)$' ./fuku/i18n`: ok.
- Task 2 gate `go test ... -run '^(TestSettingsMessagesPMStates|TestSettingsMessagesMainState|TestSettingsMessagesOpens|TestSettingsMainPhase19|TestSettingsMainGrid)$' ./fuku/modules`: ok.
- Final run `go test -tags testtools -race -count=1 -run '^(TestSettingsMessages.*|TestSettings(Authority|Drift)Messages.*|TestMessagesEndToEnd|TestEventPM.*|TestPMCopy.*|TestSettingsPhase1[89]Codes|TestSettingsStaffPrivacy|TestSettingsMain.*)$' ./fuku/modules`: ok (86 s), no FAIL or SKIP.
- `make check-translations`: all present; `make check-docs`: no drift.
- golangci-lint v2.13.1: `run --new-from-rev=9940d97 ./...` 0 issues; with `--build-tags=notesttools` 0 issues.
- `CGO_ENABLED=0 go build ./...` ok; `gofmt -l` empty on every Go file changed since 9940d97.
- Acceptance: `func settingsMsgPMPreview(` once and `composeEventPM(` once in `settings_messages.go`; `settings_msg_preview_header_pm` once in each of the 8 locale files; `func settingsMsgPMOn(` once; `settingsMsgPMPreview` 4 times in AGENTS.md; `git diff --exit-code 9940d97 -- go.mod go.sum main.go` exits 0; no 22-05 commit touches `event_message.go`, `event_message_pm.go` or `settings_edit_preview.go`.
- Mutation proof (task 1, T-22-20): with the hook composing through `composeEventMessage` and the reply's `Opts` set to the group default's options (the Unban row), `TestSettingsMessagesPMPreview` failed twice: "full preview of a custom copy" (the Unban callback row before the Rules button) and "full preview of the default" (the Spanish group default and an Unban row in place of the default private text and no keyboard). Reverted from a copy before any commit; the test passed again.
- Mutation proof (task 2): with the count's `|| settingsMsgPMOn(msgs, ev)` removed, `TestSettingsMessagesPMStates` failed ("Main's Messages line ... want @@settings-state-msg-n@@ 1"); reverted.

## Deviations from Plan

### Auto-fixed Issues

None: no bug was found in existing code.

### Other deviations

**1. [Commits] AGENTS.md in both commits, both feat**
- The user's rule (AGENTS.md in the same commit as the change it describes) puts the previews' AGENTS.md text in task 1's commit and the mark and count in task 2's. Task 2 keeps the plan's subject `feat(22-05): preview a private copy and show it on the Messages hub`; task 1 is `feat(22-05): preview a private copy through the send path's composer`.

**2. [Refactor] `settingsMsgGroupTranslator` and `settingsMsgStateFor`**
- The group-language read of Phase 21's `settingsMsgPreview` moved into `settingsMsgGroupTranslator`, shared with the new hook (behaviour unchanged, `TestSettingsMessagesPreview` passes unedited). `settingsMsgStateFor` reads `PMOn` through `settingsMsgPMOn` (same predicate as before: a spec and `Resolve(..., DestPM).Silent` false).

**3. [Test] the rights subtest's 403 is on the content**
- As the plan asks, the 403 is scripted on `sendPhoto` (the content), so the header is sent and the photo refused: the alert is `settings_edit_preview_need_start` after two recorded sends (Phase 21's preview test scripts the header instead).

### Expected-set edits of earlier tests (widened, no other change)

- `TestSettingsMessagesPMScreen` (22-04): the empty private copy screen `[1 1 1 1 2]` to `[1 1 1 1 1 2]`, the switch's row index 3 to 4 (message "fourth" to "fifth"), Back's row index 4 to 5.
- `settingsMsgScreens` (`settings_messages_screens_test.go`): private copy screens `[1 1 1 1 2]` to `[1 1 1 1 1 2]` and `[2 2 2 1 2]` to `[2 2 2 2 2 1 2]`; `TestSettingsMessagesCallbackBytes`' table `hv` 42 to 46 and `hf` 38 to 42 (the private copy's previews carry `d=p`); the `settingsMsgFullState` comment's "five rows" to "seven rows".
- `TestSettingsMessagesMainState` needed no edit (it stores no private copy, so its counts did not move).

No `TestLegacy*Requests` golden and no `TestSettingsDeletionPhase16ScreensUnchanged` digest was touched.

## Not verified

- The whole `fuku/modules` package was not run (targeted runs only, as instructed); `make test`, `make test-shard-check` and `make lint` itself were not run (the CI-equivalent golangci-lint runs above were).
- No PostgreSQL run: nothing of this plan rests on a constraint.
- The real locales' rendering of the preview header and the hub mark is covered by the 8-language length test over pure renders and the one real-locale subtest (Spanish group, English presser); no live Telegram check.

## Notes for plan 22-06 (Details button and deep link)

- The Details record must hold the composed copy exactly as the job tried to send it (`eventPMOffer.Message`, or the default after a refused custom copy); the preview's composition is not reusable there, because a preview fills the presser, not the punished member.
- The Details option toggle `ho` (`t=d`) joins the hub; once it and the Log toggle exist, the hub rows become `[1 1 1 2]` and `settingsMsgScreens`' hub widths `[1 2]` and `TestSettingsMessagesOpens`' hub assertions (`settingsMsgWantWidths(t, "the hub", keyboard, 1, 2)` and Back at row 1) need the same widening; the ` · 📩` mark on event lines stays per event.
- `settings_msg_hint_pm` is the place to add "The Details button and the log line are set on the Messages page" once 22-06 and 22-07 add the hub toggles (22-04's note still stands).
- The deep link payload `pmcopy_<22 chars>` is parsed strictly like `noteIDDeepLinkHandler`; the neutral reply `eventmsg_pm_details_none` goes in `eventMsgLocaleLimits` (`fuku/i18n/eventmsg_locale_test.go`), not the settings table.
- `TestPMCopyCallers` lists `offerEventPM` callers; nothing in this plan calls it (the preview sends directly to the presser).

## Known Stubs

None.

## Threat Flags

None: no surface beyond the plan's threat model. T-22-19 (`TestSettingsMessagesPMPreview` counts every send to any chat and requires all of them to be the presser's, and none to the group), T-22-20 (the stored button alone, no callback button; the mutation proof) and T-22-21 (the pm row's `UpdatedAt` and text unchanged, the row count, the deletion queue and the wait checked after every preview) are mitigated and tested.

## Self-Check: PASSED

- Files: `fuku/modules/settings_messages_pm_preview_test.go` exists; the modified files carry their edits.
- Commits: 20d8647 and 262d905 are ancestors of HEAD.
