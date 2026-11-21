---
phase: 20-message-engine-messages-page-and-manual-punishments
plan: 06
subsystem: settings menu / Messages page
status: complete
tags: [settings, messages-page, private-only, event-messages, MSG-05, MSG-08]
requires: [21-02, 21-03, 21-05]
provides:
  - the Messages page (key h, Order 74, icon 📨) registered from fuku/modules/settings_messages.go
  - settingsPage.PrivateNoteKey and renderSettingsPrivateOnlyNoteWith
  - one hooked message editor definition per event (hbng, htbg, hkkg, hmug, htmg, hubg, humg)
  - 23 settings_ locale keys in 8 languages
affects: [21-07, 21-08, 21-09, 21-10, 21-11]
tech-stack:
  added: []
  patterns:
    - "table-driven families and events (the Deletion page's section and row style)"
    - "a private-only page names its own group note through PrivateNoteKey"
key-files:
  created:
    - fuku/modules/settings_messages.go
    - fuku/modules/settings_messages_env_test.go
    - fuku/modules/settings_messages_test.go
  modified:
    - fuku/modules/settings_pages.go
    - fuku/modules/settings_menu.go
    - fuku/modules/settings_edit.go
    - fuku/modules/settings_main_test.go
    - fuku/modules/settings_phase18_test.go
    - fuku/modules/settings_phase19_test.go
    - fuku/modules/settings_fwdblock_test.go
    - fuku/i18n/settings_locale_test.go
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml
    - locales/uk.yml
    - docs/src/content/docs/commands/settings/index.md
    - AGENTS.md
decisions:
  - "openPage keeps calling renderSettingsPrivateOnlyNote for a page without PrivateNoteKey and calls renderSettingsPrivateOnlyNoteWith only for a page that names one, so both functions keep a production caller for the unused linter."
  - "The event state is Silent before Custom: a Silent row with a stored text reads Silent, because the bot then posts nothing."
  - "The event screen's part lines come from the page's own parts list (settingsMsgParts), so renderSettingsMsgEvent keeps the plan's signature (no def argument); plan 21-08 replaces the text part in that one list and in the definitions."
  - "TestSettingsFwdBlockMainShape's row count is now settingsMainOwnerRows - 1 (see Deviations): the Staff Group's Main shapes and row counts are unchanged."
metrics:
  duration: ~45 min
  completed: 2026-11-19
actuals:
  tokens: 29000
  tasks: 3
  commits: 3
plan_head_before: 7a18e0c
plan_head_after: 368bdae
---

# Phase 21 Plan 06: The private Messages page Summary

The group's private `/settings` menu has a Messages page (📨, page `h`, right after Reactions) that lists
the Admin actions family with each of the seven manual events' state (Default, Custom or Silent), opens
each event's screen with the state of its text, media and buttons, shows only a note in a group menu and
does not exist in a Staff Group; every event is a hooked message editor definition ready for plans 21-07
to 21-09.

## Tasks

| Task | Name | Commit |
|------|------|--------|
| 1 | The page's 23 strings in 8 languages, the help text, the limits rows | 751e1b1 |
| 2 | Register the Messages page with its families, events, screens, Main state and group note | 1246bce |
| 3 | The registry tests follow the new Main; AGENTS.md describes the page and the editor hooks | 368bdae |

## What plans 21-07 to 21-11 use

| Kind | Name |
|------|------|
| Page | `settingsPageMessages` = `h`, `Order` 74, icon 📨, `TitleKey` `settings_page_msgs`, `PrivateOnly`, `PrivateNoteKey` `settings_msg_group_note`, `Hidden` `settingsMsgHidden` |
| Actions registered | `settingsActMsgFamily` `hl` (field `t`, `settingsMsgFamilyFields`, `settingsMsgRunFamily`), `settingsActMsgEvent` `he` (field `k`, `settingsMsgEventFields`, `settingsMsgRunEvent` = `settingsEditRedraw(p, def, "")`); both `settingsRightAdmin` |
| Editor codes (in every definition, not yet page actions) | `settingsActMsgSet` `hs`, `settingsActMsgRemove` `hr`, `settingsActMsgCancel` `hc`, `settingsActMsgPreview` `hv`, `settingsActMsgFull` `hf`; `hq` (Silent) is free for 21-07 |
| Fields | `settingsMsgEventField` = `k`, `settingsMsgDestGroupCode` = `g` |
| Table | `settingsMsgFamily{Code, Icon, NameKey, IntroKey, Events}`, `settingsMsgEvent{Key, Code, Icon, NameKey, DescKey, ReservedButtons}`, `settingsMsgFamilies` (variable; one family `m` 🔨 `settings_msg_fam_manual`), `settingsMsgManualEvents` (ban `bn` 🚫 reserved 1, tban `tb` ⏳, kick `kk` 👢, mute `mu` 🔇 reserved 1, tmute `tm` 🔕, unban `ub` 🔓, unmute `um` 🔊), `settingsMsgParts` (the stand-in text part `settingsEditTextPart`, then media and buttons) |
| Lookups | `settingsMsgFamilyByCode(code) (fam, ok)` (families with events only), `settingsMsgEventByCode(code) (fam, ev, ok)`, `settingsMsgWaitPage(ev)` (`h`+code+`g`), `settingsMsgEditDef(fam, ev) settingsEditPageDef`, `settingsMsgDefByFields(fields) (def, ok)` (false without `c`) |
| State | `settingsMsgState` (`settingsMsgDefault`, `settingsMsgCustom`, `settingsMsgSilent`), `settingsMsgStateOf(msgs, ev)`, `settingsMsgStateWord(tr, state)`, `settingsMsgCountChanged(msgs)` |
| Reads | `var settingsMsgRead = eventmsg.GetFresh` (the seam tests swap), `var settingsMsgStaffLookup = staff.GetStaffGroupFresh` (Hidden's own seam), `settingsMsgReadNow(chatID)` (with `settingsReadTimeout`), `settingsMsgLoad(ev)` (Load hook), `settingsMsgMainState`, `settingsMsgOpen` |
| Renderers | `renderSettingsMsgHub(v, msgs, notice)`, `renderSettingsMsgFamily(v, fam, msgs, notice)`, `renderSettingsMsgEvent(v, fam, ev, st, opts)` (the Draw hook through `settingsMsgDraw(fam, ev)`), `settingsMsgEventLines` |
| Framework | `settingsPage.PrivateNoteKey`, `renderSettingsPrivateOnlyNoteWith(v, titleKey, noteKey, botUsername)` |
| Test toolkit | `newSettingsMsgEnv(t)` (marker locale, bot username `FukuTestBot`), `(*settingsMsgEnv).addChat()`, `settingsMsgCleanup(t, chatIDs...)`, `settingsMsgSeed(t, chatID, event, text, silent)`, `settingsMsgRows(t, chatID)`, `settingsMsgPrivateMenu(e, chat, user)` with `.press(kv...)` and `.pressButton(button)`, `settingsMsgOpenPrivate(e, user)`; in the test file `settingsMsgButtonWith`, `settingsMsgWantWidths`, `settingsMsgWantStates` |

Event screen keyboard today is Back (`a=hl&t=m`) and Close only; the body is the icon and bold name, the
description, a blank line, the three part lines and the no-caption note. Plan 21-07 adds the Silent line and
toggle, 21-08 the part rows and swaps `settingsEditTextPart` for the family's own text part (in
`settingsMsgParts` and therefore in every definition), 21-09 the previews.

## Verification

- Task 1: `go test -tags testtools -race -count=1 -run '^(TestSettingsLocaleKeys|TestSettingsLabelLengths|TestSettingsReusedKeyPlaceholders|TestSettingsHelpFitsOneMessage|TestLocaleStyle|TestLocalePlaceholdersMatchOtherLanguages)$' ./fuku/i18n` ok; `make check-translations` all present; `make generate-docs` then `make check-docs` no drift. Each locale file has 524 `settings_` lines; `reactions, messages` appears once in the generated settings docs page.
- Task 2: `go test -tags testtools -race -count=1 -run '^(TestSettingsMessagesOpens|TestSettingsMessagesMainState|TestSettingsMessagesGroupMenu|TestSettingsMessagesStaffGroup|TestSettingsPrivateOnlyPage|TestSettingsEditResolvedDefs)$' -v ./fuku/modules`: all PASS, no SKIP.
- Task 3: `go test -tags testtools -race -count=1 -run '^(TestSettingsMain|TestSettingsPhase18|TestSettingsPhase19Codes|TestSettingsMainPhase19|TestSettingsDeletionOpensFromMain|TestSettingsDeletionRowBudget|TestSettingsEditRegistry|TestSettingsMreqOpensFromMain|TestSettingsFwdBlockMainShape|TestSettingsBlacklistOpens|TestSettingsMessages)' ./fuku/modules`: ok.
- Broad: `go test -tags testtools -race -count=1 -timeout 50m -run '^(TestSettings|TestLoadSettings)' ./fuku/modules` (967 s) failed only `TestSettingsDeletionHidden` (two subtests: one Staff Group read more per Main draw, deviation 3); after the fix `TestSettingsDeletionHidden`, `TestSettingsMessages.*`, `TestSettingsFwdBlockMainShape` and `TestSettingsPhase18InStaffGroup` pass. No SKIP in the run.
- `CGO_ENABLED=0 go build ./...` ok; `go vet -tags testtools ./fuku/modules ./fuku/i18n` clean; `gofmt -l` clean on every file this plan touched. `make lint` was not run (the installed golangci-lint cannot load the Go 1.26 module), as instructed.
- Acceptance greps: `registerSettingsPage(` 1, `registerSettingsEditDef(settingsMsgEditDef(` 1, `eventmsg.GetFresh` 1, `eventmsg.Get(` 0, `PrivateNoteKey` 2 in settings_pages.go and 3 in settings_menu.go, each test function once; `x|h / m|d / t|u / z|o / y` 2 and `settingsMainOwnerRows    = 13` 1 in settings_main_test.go; AGENTS.md `PrivateNoteKey` 2, `registerSettingsEditDef` 2, the page 5.

## Expected-set edits (Task 3)

- `settings_main_test.go`: `settingsMainShapeGroup` and `settingsMainShapePrivate` now `s|l / c|f / n|k / b|i / w|g / r|e / x|h / m|d / t|u / z|o / y / a / close` (private: `… / a / groups|close`); `settingsMainOwnerRows` 13; `settingsMainTitleKeys` gains `settings_page_msgs` after `settings_page_reactions` (comment now twenty-two pages); `settingsMainWorstState` gains `settings_state_msg_n` with n 99.
- `settings_phase18_test.go` and `settings_phase19_test.go`: the registered page-key lists gain `h` after `x`; `TestSettingsPhase18InStaffGroup` expects `len(settingsMainTitleKeys)-1` state lines (every page but Deletion and Messages, plus Blocked sources).
- `settings_fwdblock_test.go`: see Deviations.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Plan conflict] `TestSettingsFwdBlockMainShape` read the ordinary Main's row constant**
- **Found during:** Task 3
- **Issue:** the plan sets `settingsMainOwnerRows` to 13 and also requires `settings_fwdblock_test.go` to stay byte-identical, but that test checks the Staff Group's Main rows against `settingsMainOwnerRows` (and `-1`). The Staff Group's Main is unchanged at 12 rows for the owner and 11 for an administrator because the Messages page is Hidden there, so the two requirements cannot both hold.
- **Fix:** the test's const block gains `staffOwnerRows = settingsMainOwnerRows - 1`, used for its four row checks, and its comment says why. Its four shape strings and the row counts it asserts (12 and 11) are unchanged, which is the criterion's intent ("the Staff Group's Main is unchanged"); `git diff --exit-code c491b48 -- fuku/modules/settings_fwdblock_test.go` therefore no longer exits 0.
- **Files modified:** fuku/modules/settings_fwdblock_test.go
- **Commit:** 368bdae

**2. [Rule 2 - Lint] `renderSettingsPrivateOnlyNote` keeps a production caller**
- **Issue:** routing every private-only page through the new `renderSettingsPrivateOnlyNoteWith` would leave the old function with test callers only, which CI's `unused` linter reports.
- **Fix:** `openPage` calls the old function when `PrivateNoteKey` is empty and the new one otherwise; output is identical for the list pages.
- **Commit:** 1246bce

**3. [Rule 1 - Bug] The page's Hidden doubled the Deletion page's counted Staff Group reads**
- **Found during:** the broad settings run after Task 3
- **Issue:** the plan's assumption had `settingsMsgHidden` read through `deletionStaffLookup`, the Deletion page's seam; `TestSettingsDeletionHidden` counts that seam's calls per Main draw (1 in a Staff Group, 2 elsewhere) and saw one more.
- **Fix:** the page reads through its own seam `settingsMsgStaffLookup` (= `staff.GetStaffGroupFresh`), same semantics (a definite row hides, an unreadable one shows); the test is unedited.
- **Files modified:** fuku/modules/settings_messages.go, AGENTS.md
- **Commit:** 368bdae

### Other notes

- AGENTS.md's `settings|v1` page list was also missing `x` reactions, `z` timezone and `v` blocked sources; they were added together with `h`.

## nolint:unused

- Removed: `registerSettingsEditDef`'s `//nolint:unused // first production caller arrives in plan 21-06` (its caller is this plan's `init`).
- `settingsEditState.Silent` and `settingsEditEvent` now have production users (`settingsMsgLoad` and `settingsMsgEditDef`).
- None added. Every new production function, type, constant and field has a production caller in this plan, except the five editor code constants, which are read by `settingsMsgEditDef` (so used) but registered as page actions only from plan 21-08.

## Known Stubs

- The event definitions carry the Welcome text part (`settingsEditTextPart`) as a stand-in whose wait text is never shown: no Set action exists until plan 21-08, which replaces it with the family's own text part (planned, flagged in the plan's assumptions).

## Threat Flags

None: T-21-17 to T-21-20 are mitigated as planned (PrivateOnly note with no read, `Validate` requiring `c` and a known code before any I/O, the press pipeline's live check and owner gate, Hidden in a Staff Group).

## Self-Check: PASSED

- FOUND: fuku/modules/settings_messages.go, settings_messages_env_test.go, settings_messages_test.go
- FOUND commits: 751e1b1, 1246bce, 368bdae (all ancestors of HEAD)
