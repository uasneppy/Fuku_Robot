---
phase: 18-remaining-settings-pages
plan: 13
subsystem: settings-menu
tags: [telegram, settings-page, main-grid, privacy, cross-page-tests]
status: complete

requires:
  - phase: 18-remaining-settings-pages
    provides: "plans 18-03, 18-06, 18-07, 18-09, 18-10 and 18-12 (the six pages Warnings n, Switches t, Language u, Log channel o, Anonymous admins y and Locks k with their renderers, readers, apply functions and per-page proofs) and the final Main-shape constants"
provides:
  - "The 15-cell Main proved: the hand-built fifteen-cell subtest of TestSettingsMainGrid, TestSettingsMainPhase18 (sixteen state lines in order with the defaults, ten owner rows and nine admin rows, the unknown mark for each failing reader with every other line unchanged, no Telegram call of its own, no row written, the earlier ten pages' cells and data unchanged) and settingsMainWorstState extended to the longest Phase 18 state of each language"
  - "settingsPrivacyPhase18Run and the Phase 18 screens of TestSettingsStaffPrivacy: 1112 screens in 8 languages and the six pages driven end to end in a group menu and in the private menu of an administrator who is not the creator, searched for the Staff Group, a log channel's title and ID and a member"
  - "settings_phase18_test.go: TestSettingsPhase18InStaffGroup, TestSettingsPhase18Codes, TestSettingsPhase18ReadsCreateNothing, TestSettingsPhase18Cost and the helpers settingsPhase18Tables / RowCounts / RowCount / Calls"
  - "AGENTS.md: the final Main and why it is over the content-page row budget, and the Phase 18 test families"
affects: [18-14, 18-15]

plan_head_before: bc424c4
plan_head_after: 90e61ea84f86b404e0d39e7211fa243a32a387f2

actuals:
  tokens: 12700   # chars/4 over the added non-planning lines of bc424c4..90e61ea (50902 chars), tests and AGENTS.md included
  tasks: 3
  commits: 3   # MEASURED: git rev-list --count bc424c4..90e61ea (before the SUMMARY and state commits)

key-files:
  created:
    - fuku/modules/settings_phase18_test.go
  modified:
    - fuku/modules/settings_main_test.go
    - fuku/modules/settings_staff_privacy_test.go
    - AGENTS.md

key-decisions:
  - "No production code changed: every proof held at HEAD, so there is no fix(18-13) commit. Each new test was checked able to fail (a mutation of settingsAnonRead that creates the admin row made TestSettingsPhase18ReadsCreateNothing fail, then was restored)."
  - "The Switches line on a chat with no rows reads {n} on with n = 1, not Off as the plan text says: a chat with no report row reads Reports on by the model default, AI off and Auto-approve off. TestSettingsMainPhase18 pins the real contract (settings_state_sw_on with 1), which also is what TestSettingsSwitchesOpens already pins."
  - "The 'nine settings' tables' the ReadsCreateNothing sweep checks are warn_settings, report_chat_settings, ai_spam_settings, greetings, chats, admin, log_channels, locks and rules (Main also reads the Rules state); the language is stored in chats."
  - "TestSettingsPhase18ReadsCreateNothing compares the Main-and-six-pages chat with a second fresh chat that only had the Phase 17 Deletion page opened the same way, so what the menu's own pipeline writes is the baseline; both are empty."

patterns-established:
  - "A sweep over six pages renders each screen once per language and view with the worst case of each kind: two notices (none and the save-failed one) and the longest lock failure notice stand for the rest, because a notice only adds its own length."
  - "A cost test counts requests by method since a mark (settingsPhase18CallsSince) and compares the whole map, so an extra call of any method fails it, not only getChatMember."

requirements-completed: []

coverage:
  - id: D1
    description: "Main shows the fifteen grid cells in the agreed order (10 rows for the owner, 9 for an administrator), one state line per page from fresh reads with the defaults, the unknown mark on a failing page only, no Telegram call of its own and no row written, within 3800 UTF-16 units in 8 languages"
    requirement: "PAGE-01 to PAGE-06, PLAT-05 (contribution only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^(TestSettingsMainGrid|TestSettingsMainPhase18|TestSettingsMainPageFitsEveryLocale)$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D2
    description: "No Phase 18 screen names the Staff Group, a log channel's title or ID or a member, in 8 languages, a group menu and the private menu of an administrator who is not the creator; the six pages work inside a Staff Group"
    requirement: "PAGE-01 to PAGE-06, PLAT-05 (contribution only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^(TestSettingsStaffPrivacy|TestSettingsPhase18InStaffGroup)$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D3
    description: "Across the six pages the page keys, the action codes (kg included), the control keys and every allow-list are the contract's, opening Main and every page creates no row in the nine tables, and each open costs what the contract says (also with Redis down)"
    requirement: "PAGE-10 (contribution only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^TestSettingsPhase18(Codes|ReadsCreateNothing|Cost)$' ./fuku/modules"
        status: pass
    human_judgment: false
---

# Phase 18 Plan 13: Main with fifteen pages and the cross-page proofs Summary

**Main is proved with all six new pages (fifteen cells, sixteen state lines, ten owner rows), every Phase 18 screen is proved private in 8 languages and both menu kinds, the pages work in a Staff Group, and the codes, allow-lists, no-row reads and call costs are proved across the pages. Test-only plus AGENTS.md; no production file changed.**

## What changed

- **Task 1, tracer (commit fd7e0b8).** `settings_main_test.go`: the hand-built fifteen-cell subtest of `TestSettingsMainGrid` (the real page keys in the contract's order: widths `{2,2,2,2,2,2,2,1}`, Access, navigation, 10 owner rows, 9 admin rows, 16 state lines, at most 8 buttons to a row); `TestSettingsMainPhase18` (the real dispatcher on the marker locale: the final grid and sixteen lines in order for the owner in a group and in private, each new page's default line, the administrator's nine rows and no Access line, the ten earlier pages' cells and callback data equal to their pre-Phase-18 values, one subtest per failing reader (warns, locks, switches, language, log channel, anonymous admins) that shows the unknown mark on that line only and leaves the other fifteen unchanged, and a Back press that costs the presser's `getChatMember`, one edit and one answer and writes no row); `settingsMainWorstState` extended with the Switches count 3, the Locks count 22, the Warnings line at limit 100 for every stored action and the eight language names with their flags, and the fit test now requires 16 registered pages. AGENTS.md: the `renderSettingsMain` bullet states the fifteen cells, 10 and 9 rows, that Main is the one screen over the content-page row budget and why, and that the second hub of the UI-SPEC is not built.
- **Task 2 (commit a82f948).** `settings_staff_privacy_test.go`: `settingsPrivacyPhase18Screens` (Warnings with a stored, an inert and an empty action at limit 100; Switches in six combinations; Language with each of the 8 stored codes and an unknown one; the Log channel page mixed, all on and not linked; Anonymous admins on and off; both Locks pages with each right line and the longest failure notice; and the alerts the pages answer with), added to the per-language screen map of `TestSettingsStaffPrivacy` for a group menu and a private administrator's menu, each searched for the Staff Group, the log channel and the member. `settingsPrivacyPhase18Run` (called for both menu kinds) makes permitted presses on each page (warn limit and action, Reports, language, two log categories, two locks across the pager) and refused ones (a warn action the bot does not act on, the AI filter that cannot run, Auto-approve without `can_change_info`, the anonymous-admins toggle for a non-creator, an unsupported language), asserts the stored effects, that the audit posts did reach the log channel (so the search is meaningful), and that nothing the bot sent to the group or the private chat names the Staff Group, the channel's title and ID or the member. `TestSettingsPhase18InStaffGroup` (new `settings_phase18_test.go`): in a real Staff Group (Deletion is absent, proving it) Main lists the six pages with a state, each opens, and a permitted press on each stores through its apply function.
- **Task 3 (commit 90e61ea).** `settings_phase18_test.go`: `TestSettingsPhase18Codes` (the sixteen registered keys and no more, every action code owned by one page and never a core code, `kg` the Locks pager, exact control sets, 22 distinct lock codes, each allow-list through `settingsControlFieldsValid` including the bad values, and the pager's validator), `TestSettingsPhase18ReadsCreateNothing` (Main and the six pages, Locks page 2 included, on a fresh chat in a group and a private menu: no row in the nine tables, compared with the Phase 17 Deletion page opened the same way), `TestSettingsPhase18Cost` (Main's states make no call; opening each page costs `getChatMember` + edit + answer in a group menu and adds `getChat` in a private one, Locks one more `getChatMember` of the bot, the same with Redis down; seven kinds of press refused in step 1 cost only their answer and leave no row). AGENTS.md: the Phase 18 test families bullet.

## Verification (real results)

- Task verify blocks: `^(TestSettingsMainGrid|TestSettingsMainPhase18|TestSettingsMainPageFitsEveryLocale|TestSettingsEditScreensFitEveryLocale)$`, `^(TestSettingsStaffPrivacy|TestSettingsPhase18InStaffGroup)$` and `^TestSettingsPhase18(Codes|ReadsCreateNothing|Cost)$` pass in the CI environment (`cienv.sh`) and bare.
- Focused run `^(TestLegacy|TestSettingsMain|TestSettingsLabelLengths|TestSettingsStaffPrivacy|TestSettingsControlActionAllowList|TestSettingsPhase18)` on `./fuku/modules ./fuku/i18n`: `ok` in the CI environment (modules 41.7 s) and bare (modules 41.5 s).
- Whole `fuku/modules`, CI environment, `go test -tags testtools -race -count=1 -timeout 40m -v ./fuku/modules` (log `/tmp/claude-0/-home-user-Fuku-Robot/4ce545b5-6832-57a3-a863-43c972932c7c/scratchpad/modules-18-13.log`): `ok  github.com/uasneppy/fuku_robot/fuku/modules  1662.055s`, last line `exit 0`, 0 `--- FAIL`, 0 `--- SKIP`, 0 `^FAIL`, 7795 `--- PASS` (7763 at 18-12). `./fuku/i18n`: `ok` (23.7 s).
- `CGO_ENABLED=0 go build ./...` ok; `go vet -tags testtools ./fuku/modules ./fuku/i18n` clean; `gofmt -l` on the three Go files prints nothing; `make check-translations` all present; `make check-docs` no drift; `git diff --exit-code 8c0c827 -- go.mod go.sum` and `git diff --exit-code 29aca33 -- fuku/modules/settings_pages.go` exit 0; `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827` exit 0 (read directly): no dupl, gocyclo or godox findings. No `t.Skip`, `reflect.Ptr` or `ioutil` in the new code.
- Not run: `make lint` (the installed golangci-lint is built with Go 1.25, a documented sandbox fact), gosec, `make test`.

## Time added to the whole-package run

About 5.5 s, measured from the whole-package log and focused runs: `TestSettingsMainPhase18` 1.9 s, the fifteen-cell subtest and the fit test's extension about 0.1 s, `TestSettingsStaffPrivacy` +1 s of pure renders (1112 screens render in 0.38 s, the rest is the leak search) and the two Phase 18 runs 0.45 s and 0.44 s, `TestSettingsPhase18InStaffGroup` 0.41 s, `TestSettingsPhase18ReadsCreateNothing` 0.84 s, `TestSettingsPhase18Cost` 0.39 s, `TestSettingsPhase18Codes` under 0.01 s. No new test is over 2 s. `TestSettingsStaffPrivacy` as a whole is 22.8 s in the whole run (about 23 s focused; it was about 20 s before). The whole package took 1662 s here, inside the 2400 s limit.

## Deviations from Plan

None needing a production fix. Notes:

1. **[Plan text] The Switches default.** The plan says each new page reads its default on a chat with no rows "Off" for Switches; the real state is `{n} on` with n = 1 (Reports reads on with no row, by the model default, as `TestSettingsSwitchesOpens` already pins). The test asserts the real contract.
2. **[Plan text] "The nine settings' tables".** The sweep covers nine tables: the eight the six readers touch plus `rules`, which Main's Rules line reads (`greetings` and `chats` included; the language lives in `chats`).
3. **[Scope] The Phase 18 privacy screens use two notices** (none, and the save-failed one) and the single longest lock failure notice rather than every notice the pages can show: a notice only adds its own length and changes no button, and the per-page fit tests already hold each page's longest notices.
4. **[Pre-existing] `gofmt -l fuku/modules/*_test.go` lists `blacklists_command_test.go`**, a file this plan did not touch; the three files this plan touched are clean.

**Total deviations:** 0 auto-fixed, 4 notes. **Impact:** none on the contract.

## Known Stubs

None. The helpers `settingsPhase18Tables`, `settingsPhase18RowCounts`, `settingsPhase18RowCount`, `settingsPhase18Calls`, `settingsPhase18CallsSince`, `settingsPhase18Open`, `settingsPhase18OpenAll` and `settingsPhase18Target` are all used by the tests of this plan.

## Threat Flags

None. No endpoint, auth path or schema is added. T-18-44 (a Phase 18 screen naming the Staff Group, a log channel, a member or a count) is mitigated by the privacy screens and `settingsPrivacyPhase18Run` in 8 languages and both menus; T-18-45 (two pages or actions sharing a code) by `registerSettingsPage`'s init panics and `TestSettingsPhase18Codes`; T-18-46 (opening Main or a page creating rows or spending calls) by `TestSettingsPhase18ReadsCreateNothing`, `TestSettingsPhase18Cost` and `TestSettingsMainPhase18`; T-18-SC by an unchanged go.mod and go.sum.

## Requirements

`requirements-completed: []`. PAGE-01 to PAGE-06, PAGE-10 and PLAT-05 stay Pending; only plan 18-15 completes them after 18-14's end-to-end test.

## Self-Check: PASSED

- `fuku/modules/settings_phase18_test.go` exists; `grep -c 'func TestSettingsPhase18InStaffGroup(' ` prints 1, and `TestSettingsPhase18Codes`, `TestSettingsPhase18ReadsCreateNothing` and `TestSettingsPhase18Cost` each appear once; `grep -c 'func TestSettingsMainPhase18' fuku/modules/settings_main_test.go` prints 1 and `settings_state_locks_on` appears in it; `settingsPrivacyPhase18Run` appears three times in `settings_staff_privacy_test.go`; `grep -c 'settingsPrivacyPhase18Run\|TestSettingsPhase18' AGENTS.md` is at least 1.
- Commits fd7e0b8, a82f948 and 90e61ea are ancestors of HEAD.
