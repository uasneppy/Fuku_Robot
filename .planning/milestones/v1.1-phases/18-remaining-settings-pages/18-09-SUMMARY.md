---
phase: 18-remaining-settings-pages
plan: 09
subsystem: settings-menu
tags: [telegram, settings-menu, log-channel, toggle-grid, audit-post, privacy]
status: complete

requires:
  - phase: 18-remaining-settings-pages
    provides: "plan 18-08 (applyLogCategory and logchannels.LoadContext) and plan 18-06 (settingsToggleSet and the Switches page pattern)"
provides:
  - "The Log channel page (page o, Order 88, icon 🧾) in settings_logchannel.go: the linked state and six category toggles from one table, each tg only with settingsRightAdmin, writing exactly one category through applyLogCategory and then posting the same #SETTINGS audit entry /log and /nolog post"
  - "settingsToggleGrid and settingsToggleCell in settings_toggle.go: the two-column toggle grid (cells 'icon name', two to a row, odd last cell alone, icons passed in) the Locks page of 18-12 reuses"
  - "The not-linked behaviour: six Off lines, cells drawn, a press refused with logs_not_set and nothing written or posted; the privacy rule: the renderer receives only the linked flag and the six booleans, never the row"
  - "Eleven tests in two files: Opens, ParityLogCategory, NotLinked, Privacy, Forged, FailedSave, Drift, AuthorityParity, ReadsCreateNothing, CallbackBytes, PageFitsEveryLocale"
affects: [18-12, 18-13, 18-15]

plan_head_before: 9123947d97be04adc7a515e74ff436ba1c08fbfb
plan_head_after: be57ed559fc67050523d75af8399321e43ea1d8b

actuals:
  tokens: 15000   # chars/4 over the added non-planning lines of 9123947..be57ed5 (60108 chars), tests and docs included
  tasks: 3
  commits: 4   # MEASURED: git rev-list --count 9123947..HEAD (before the SUMMARY and state commits)

key-files:
  created:
    - fuku/modules/settings_logchannel.go
    - fuku/modules/settings_logchannel_test.go
    - fuku/modules/settings_logchannel_screens_test.go
  modified:
    - fuku/modules/settings_toggle.go
    - fuku/modules/settings_main_test.go
    - AGENTS.md
    - README.md
    - docs/src/content/docs/api-reference/callbacks.md
    - docs/src/content/docs/api-reference/permissions.md
    - .planning/phases/18-remaining-settings-pages/18-SPIKE.md

key-decisions:
  - "The audit post follows the redraw: the control's Set answers and edits the page itself (p.answer then p.edit) and returns nil, then calls actionlog.Settings, so a send never keeps the menu waiting. The commands inline the same call and there is no shared poster to reuse, so the page makes the one call with the apply function's key plus the repository name."
  - "The reader seam settingsLogRead is logchannels.LoadContext itself (as settingsLangRead is the language reader); settingsLogStateOf reduces the row to settingsLogState at the seam, so the renderer and everything drawn can never see log_channel_id."
  - "No new locale keys: all eight settings_log_* keys, settings_state_linked, settings_state_not_linked and settings_page_log were added by plan 18-02 with their limits rows and label-length checks, so TestSettingsLabelLengths and the limits table already guard this page's names."

patterns-established:
  - "A page whose change posts an audit line redraws first and posts after, by answering and editing inside Set and returning nil."
  - "A privacy-sensitive page passes its renderer a reduced state type, never the stored row."

requirements-completed: []

coverage:
  - id: D1
    description: "A press of a category cell writes exactly that category as /log or /nolog write it and posts the same #SETTINGS audit post; turning the settings category off posts nothing"
    requirement: "PAGE-04, PAGE-10 (contribution only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^(TestSettingsLogChannelOpens|TestSettingsParityLogCategory|TestSettingsDriftLogChannel)$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D2
    description: "With no channel linked the six lines read Off, the cells are drawn and a press is refused with logs_not_set, writing and posting nothing; a failed save redraws the stored state and posts nothing"
    requirement: "PAGE-04 (contribution only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^(TestSettingsLogChannelNotLinked|TestSettingsLogChannelFailedSave|TestSettingsLogChannelForged)$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D3
    description: "No edit, button, callback data, notice, alert or message of the page in a group menu or a private menu of a non-creator administrator contains the log channel's title or ID"
    requirement: "PAGE-04 (contribution only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^TestSettingsLogChannelPrivacy$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D4
    description: "The page is refused what /log refuses (member, owner gate carve-out), reads create nothing, buttons fit 64 bytes (45 private, 41 group) and the page fits 3800 UTF-16 units in 8 languages"
    requirement: "PAGE-04, PAGE-10 (contribution only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^(TestSettingsAuthorityParityLogChannel|TestSettingsLogChannelReadsCreateNothing|TestSettingsLogChannelCallbackBytes|TestSettingsLogChannelPageFitsEveryLocale)$' ./fuku/modules"
        status: pass
    human_judgment: false
---

# Phase 18 Plan 09: the Log channel page Summary

An administrator can now turn each of the six log categories on or off from a Log channel page exactly as `/log` and `/nolog` do, with the same `#SETTINGS` audit line, and the page says Linked or Not linked without ever naming the channel.

## What changed

- **Task 1, tracer (commit d9d1023).** `settings_logchannel.go`: `settingsLogCategories` (one table of the control code `s a u m r o`, the repository name and the `settings_log_cat_*` key, in the order of `logchannels.AllCategories`), the seam `settingsLogRead` (`logchannels.LoadContext`), `settingsLogState` (linked flag and six booleans, nothing else), `settingsLogStateOf`, `settingsLogMainState` (Linked or Not linked, the unknown mark on a failed read), `settingsLogOpen`, the pure `renderSettingsLogChannel` and `settingsLogControl`. `init` registers the page once with a control per table row (`settingsRightAdmin`, `tg` only, `settingsEditSwitchValue`). A press goes through `settingsToggleSet` and `applyLogCategory`; after a stored change `Set` answers and edits the page itself and then calls `actionlog.Settings(bot, group, presser, key+" "+name)`, returning nil so the dispatcher edits nothing twice. `settings_toggle.go`: `settingsToggleGrid` with `settingsToggleCell`. `settings_main_test.go`: the Main-shape constants gain `o` and nine rows. AGENTS.md: the callbacks page list, the Main order sentence and a new Log channel page bullet.
- **Task 2 (commits c8540cc, e3389f6).** `TestSettingsLogChannelNotLinked` (six presses and the opposite value refused with the notice, a channel unlinked after the draw), `TestSettingsLogChannelPrivacy` (a group menu and a private menu of an administrator who is not the creator, open, each cell, a failed save and a press after an unlink, searching every edit, answer and message except the posts to the channel itself for the channel's title and ID, plus a subtest that the exclusion is meaningful), `TestSettingsLogChannelForged` (6 shapes on each of the 6 categories, including the repository name as a control, plus an unknown control) and `TestSettingsLogChannelFailedSave`. Docs: AGENTS.md completed, README, `callbacks.md`, `permissions.md` and three new `18-SPIKE.md` checks (11 to 13, with the accepted T-18-32 note).
- **Task 3 (commit be57ed5).** `TestSettingsDriftLogChannel`, `TestSettingsAuthorityParityLogChannel`, `TestSettingsLogChannelReadsCreateNothing`, `TestSettingsLogChannelCallbackBytes` and `TestSettingsLogChannelPageFitsEveryLocale` (the last two over all 65 states in `settings_logchannel_screens_test.go`).

## Verification (real results)

- Task 1 verify, plus the neighbouring Main-shape tests (`TestSettingsLogChannelOpens`, `TestSettingsParityLogCategory`, `TestLegacyLogCategoryRequests`, `TestSettingsMainGrid`, `TestSettingsDeletionOpensFromMain`, `TestSettingsDeletionRowBudget`, `TestSettingsEditRegistry`, `TestSettingsMreqOpensFromMain`, `TestSettingsSwitchesOpens`): all `--- PASS`. Mutation check: replacing the `actionlog.Settings` call with a no-op made `TestSettingsParityLogCategory` fail on the audit post count; restored.
- Tasks 2 and 3 verify blocks: all new tests `--- PASS`, `make check-docs` "No drift detected".
- Focused run in the CI environment (`cienv.sh`: `BOT_TOKEN`, `OWNER_ID`, `REDIS_ADDRESS`, `DATABASE_URL`, `DEBUG=false`) of the eleven new tests plus `^(TestLegacy|TestSettingsMain|TestSettingsStaffPrivacy)` on `./fuku/modules`: `ok`, 62 s; the same set run bare (those variables unset): `ok`, 63 s. `./fuku/i18n` `-run '^(TestSettingsLabelLengths|TestSettings)'`: `ok`; whole `./fuku/i18n` (`-v`, log `i18n-18-09.log` in the scratchpad): `ok ... 23.743s`, 0 FAIL or SKIP lines.
- Whole `fuku/modules` package, CI environment, `go test -tags testtools -race -count=1 -timeout 30m -v ./fuku/modules` (background, log `/tmp/claude-0/-home-user-Fuku-Robot/4ce545b5-6832-57a3-a863-43c972932c7c/scratchpad/modules-18-09.log`): `ok github.com/uasneppy/fuku_robot/fuku/modules 1655.771s`, `exit 0`, `grep -cE -- '--- (FAIL|SKIP)|^FAIL'` = 0, 1725 top-level `--- PASS` lines (309 of them `TestSettings*`), and all eleven new tests `--- PASS` in it.
- `CGO_ENABLED=0 go build ./...` ok; `go vet -tags testtools ./fuku/modules ./fuku/i18n` clean; `gofmt -l` on the five Go files prints nothing; `make check-translations` all present; `make check-docs` no drift; `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827` exit 0 ("no dupl, gocyclo or godox findings"), read directly (an earlier run found an unused test method, removed); `git diff --exit-code 8c0c827 -- go.mod go.sum` exit 0; `git diff --exit-code 29aca33 -- fuku/modules/settings_pages.go migrations fuku/db/models main.go` exit 0; no `t.Skip` in the new tests; no `reflect.Ptr` or `ioutil.`.
- Acceptance greps: each required test function prints `1`; `registerSettingsPage` once (comments excluded); `func settingsToggleGrid` once; `applyLogCategory(`/`actionlog.Settings(` twice; `logs_current|LogChannelID|log_channel_id` zero outside comments; `/nolog` appears in `permissions.md`.
- Not run: `make lint` (installed golangci-lint is built with Go 1.25, a documented sandbox fact), gosec, `make test`.

## Deviations from Plan

### Auto-fixed Issues

None needing a production fix. Notes:

1. **[Scope] `TestSettingsStaffPrivacy` not edited.** The task prompt asked to add the page to it "as the plan says", but the plan does not list that file and assigns the multi-language privacy sweep to 18-13 (T-18-29). No earlier Phase 18 plan added its page there either. The page's own privacy proof is `TestSettingsLogChannelPrivacy`, which runs in both menu kinds; it reads no Staff Group row at all. The sweep stays for 18-13.
2. **[Plan text] No locale keys added.** The brief's locale-file rule does not apply: plan 18-02 already put every key this page uses (`settings_page_log`, `settings_state_linked`, `settings_state_not_linked`, `settings_log_status`, `settings_log_hint`, six `settings_log_cat_*`) in all eight files with their `settingsLocaleLimits` rows and the `TestSettingsLabelLengths` groups, so no limits row or label-length group was added.
3. **[Test design] Parity runs the command and the page on the same chat.** The plan suggested two chats; the chats' titles differ in the fixtures, which would change the audit post's header. One chat, put back with the repository between the command and the press, makes the posted requests directly comparable (text and parse mode), and the row and the cached row are compared too. A double tap posts once per tap, as the plan assumed.
4. **[Fixture] The privacy test's channel has a fixed ID and title** (`-1002468013579`, "ZZ Log Channel Marker", the constants Phase 14 uses for its channel) so the digits are searchable; every row it writes is deleted in `t.Cleanup` before and after.
5. **Earlier-plan tests edited:** only `settings_main_test.go`'s three Main-shape constants, as the plan says; no golden was re-recorded.

**Total deviations:** 0 auto-fixed, 5 notes. **Impact:** none on the contract.

## Known Stubs

None. Every control writes through the repository and the page reads the real table.

## Threat Flags

None. The page adds no endpoint, no auth path and no schema; its one outward effect, the `#SETTINGS` post, is the existing one the commands make.

## Requirements

`requirements-completed: []`. PAGE-04 and PAGE-10 stay Pending; only plan 18-15 completes them.

## Self-Check: PASSED

- Created files exist on disk (`settings_logchannel.go`, `settings_logchannel_test.go`, `settings_logchannel_screens_test.go`) and the commits are ancestors of HEAD: d9d1023, c8540cc, e3389f6, be57ed5.
- Every task's acceptance criteria were re-run and print the required values (see Verification).
