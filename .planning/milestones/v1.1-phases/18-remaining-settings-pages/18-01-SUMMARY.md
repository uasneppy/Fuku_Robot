---
phase: 18-remaining-settings-pages
plan: 01
subsystem: settings-menu
tags: [telegram, settings-menu, warns, apply-function, characterization-golden, i18n]

requires:
  - phase: 17-command-edit-and-self-destruct-deletion
    provides: the settings page framework (registerSettingsPage, settingsStepperRow, settingsRightAdminFloodBot), the Main status block and the registry tests this plan adapts
provides:
  - applyWarnLimit and applyWarnMode in warns.go (typed warnSettingsResult, write seam warnSettingsWrites, accepted words warnSettingsModes) used by /setwarnlimit and /setwarnmode
  - warns.LoadWarnSettingsContext, the error-returning non-creating reader on the commands' warn_settings key
  - the Warnings page (page n, Order 42, icon 🚩) with the limit stepper behind settingsRightAdminFloodBot
  - four settings_ keys in 8 locales (334 settings_ keys)
  - TestLegacyWarnSettingsRequests, the golden of both warn commands, recorded before the extraction
  - the shared Main-shape constants (settingsMainShapeGroup, settingsMainShapePrivate, settingsMainOwnerRows)
affects: [18-02, 18-03, 18-04, 18-05, 18-06, 18-07, 18-08, 18-09, 18-10, 18-11, 18-12, 18-13, 18-14, 18-15]

plan_head_before: 758c0c6fe11eae71ad70ff9ff56907001a2de9c5
plan_head_after: ef11c15da884e07e25511f6a79648d8bb897a6b0

actuals:
  tokens: 15900   # chars/4 over the added lines of 758c0c6..ef11c15 (63439 chars), tests, locales and docs included
  tasks: 3
  commits: 4      # MEASURED: git rev-list --count 758c0c6..HEAD (before this SUMMARY commit)

tech-stack:
  added: []
  patterns:
    - "golden-first extraction: the characterization test of a command is committed alone before the production change"
    - "reader that returns the default for a missing row without caching it (the command's loader creates the row on a miss)"
    - "registry tests read one set of Main-shape constants, so a page-adding plan edits only the constants"

key-files:
  created:
    - fuku/modules/warns_legacy_request_test.go
    - fuku/modules/warns_settings_apply_test.go
    - fuku/db/warns/load_settings_test.go
    - fuku/modules/settings_warns.go
    - fuku/modules/settings_warns_test.go
  modified:
    - fuku/modules/warns.go
    - fuku/db/warns/repository.go
    - fuku/modules/settings_main_test.go
    - fuku/modules/settings_mreq_test.go
    - fuku/modules/settings_deletion_test.go
    - fuku/modules/settings_deletion_screens_test.go
    - fuku/modules/settings_edit_test.go
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

key-decisions:
  - "The reader returns a miss as the defaults (limit 3, mode mute) through a loader error, so GetFromCacheOrLoad caches nothing for a chat with no row and the command loader's next read still creates the row as before."
  - "The page's body is the limit line and the hint only; the action line, the action choice row and the inert line are plan 18-03's (their keys arrive with 18-02)."
  - "Deviation note 1 applied: the Warnings limit control is st only (UI-SPEC), and the Phase 13 allow-list subtest skips only a control that declares Actions and whose Validate accepts v=1."

patterns-established:
  - "settingsWarnsRead seam (a variable) so a test can make the page's read fail"
  - "page-prefixed test helpers in settings_warns_test.go (settingsWarnsCleanup, settingsWarnsRow, settingsWarnsSeed, settingsWarnsPress, settingsWarnsCommand, settingsWarnsCached)"

requirements-completed: []

coverage:
  - id: D1
    description: "/setwarnlimit and /setwarnmode write through applyWarnLimit and applyWarnMode with byte-identical requests and stored rows"
    requirement: "PAGE-10 (contribution only)"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestLegacyWarnSettingsRequests|TestApplyWarnSettings|TestWarnSettingsCommandsUpdateAndDisplay|TestApplyWarn)$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D2
    description: "The warn settings read back with their error through a reader that creates no row and caches no default"
    requirement: "PAGE-10 (contribution only)"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestLoadWarnSettingsContext$' ./fuku/db/warns"
        status: pass
    human_judgment: false
  - id: D3
    description: "An administrator raises the warn limit from the Warnings page and the stored row and cache equal what /setwarnlimit leaves"
    requirement: "PAGE-01 (contribution only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^(TestSettingsWarnsOpens|TestSettingsParityWarnLimit)$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D4
    description: "Four Warnings keys in all 8 locales within their ceilings"
    requirement: "PLAT-05 (contribution only)"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestSettingsLocaleKeys|TestSettingsLabelLengths|TestSettingsReusedKeyPlaceholders)$' ./fuku/i18n && make check-translations"
        status: pass
    human_judgment: true
    rationale: "the translations are machine-written under the ceilings; the UI-SPEC asks for a native read before the owner relies on them"

duration: 65min
completed: 2026-11-12
status: complete
---

# Phase 18 Plan 01: the Warnings page tracer Summary

An administrator can now raise or lower the warn limit from a Warnings page (🚩, between Antiflood and Welcome) and it is stored exactly as `/setwarnlimit` stores it, because both go through the new `applyWarnLimit`; `/setwarnmode` was extracted the same way into `applyWarnMode`, behind a golden of both commands recorded before the change.

## What was built

- **Golden first** (`warns_legacy_request_test.go`, commit 871d39c, alone, before any production edit): `TestLegacyWarnSettingsRequests`, 23 cases on fixed chat IDs `-1009918001101` to `-1009918001123`. For `/setwarnlimit`: no argument, `abc`, 0, 101, 1, 100, 5 on a new chat (stores mode mute) and on a seeded one (keeps mode ban), a range refusal that keeps the row, a write failure, a non-admin sender and a bot that is not an administrator. For `/setwarnmode`: no argument, `BAN`, `kick`, `mute`, a seeded limit kept, `tban`, `xyz`, `a&b<c` (recorded unescaped, the golden shows `&` and `<` only because JSON encodes them), a write failure (a real GORM callback on the create chain, matching table `warns_settings` and the case's chat ID) and the two refusals. Each case asserts the request JSON (the chat ID replaced by `<chat>`) and the stored row read straight from the table. The test passes unchanged after the extraction.
- **Extraction** (`warns.go`, commit 07ea430): `warnSettingsModes` (ban, kick, mute), `warnSettingsOutcome` (`warnSettingsApplied` zero value, `OutOfRange`, `UnknownMode`, `SaveFailed`), `warnSettingsResult` (outcome, the command's reply key, the row read back, the error), the write seam `warnSettingsWrites` (defaults `warns.SetWarnLimit` and `warns.SetWarnMode`), `applyWarnLimit` (1..100) and `applyWarnMode` (exactly ban, kick or mute as spelled). `setWarnLimit` and `setWarnMode` keep their parsing, gates, `Smarkdown`/`Shtml` replies and the unescaped `fmt.Sprintf(temp, args[0])`, and map the result to the unchanged reply.
- **Reader** (`fuku/db/warns/repository.go`, additions only): `LoadWarnSettingsContext` goes through `cache.GetFromCacheOrLoad` under the key and TTL of `GetWarnSettingContext`, returns the read error, inserts nothing and, for a chat with no row, returns limit 3 and mode mute without caching them. `TestLoadWarnSettingsContext` (miniredis, four subtests) proves no row and no cache key after a read of a missing row, the command's next read still creating its row, a stored row cached under the command's key, a write dropping the key, and a read error returned with no cache entry.
- **Page** (`settings_warns.go`, commit d342f24): page `n`, Order 42, icon 🚩, no `Hidden`; Main state `settings_state_warns` (`Limit {n}, {action}` through `settingsFloodActionWord`, `settings_state_not_set` for an empty mode, the unknown mark on a failed read); body limit line plus hint; one control `l` (`st` only, `Validate` `settingsParseNumber(v, 3)`, right `settingsRightAdminFloodBot`) with the stepper -10, -1, +1, +10 clamped to 1..100 sent as absolute targets; a step at 1 or 100 answers `settings_min_value` or `settings_max_value` and writes nothing; every other press calls `applyWarnLimit` and redraws from its read-back (a refusal or failed save redraws the stored state under a warn notice with the command's own key).
- **Locales**: `settings_page_warns`, `settings_state_warns`, `settings_warns_limit`, `settings_warns_hint` in all 8 files (334 `settings_` keys each), one limits row each, and `{"🚩", "settings_page_warns"}` in `TestSettingsLabelLengths`.
- **Registry tests adapted** (legitimate consequences of the contract, named below).
- **AGENTS.md**: the "One setting, one code path" bullet (the apply functions, the reader, the `warn_settings` skew, the mode words), the `settings|v1` page list (`n`), a Warnings page bullet after the Deletion one, the Main sentence (order and row count), the toolkit bullet (the Main-shape constants and the narrowed subtest) and the Testing bullet (the golden is never re-recorded, commit ef11c15).

## Verification results (real)

- `go test -tags testtools -race -count=1 -run '^(TestLegacyWarnSettingsRequests|TestApplyWarnSettings|TestWarnSettingsCommandsUpdateAndDisplay|TestApplyWarn)$' -v ./fuku/modules`: all four PASS, no FAIL or SKIP. `go test -tags testtools -race -count=1 ./fuku/db/warns`: `ok` (whole package, includes `TestLoadWarnSettingsContext` and `TestWarnUserCreatesMissingParentRows`).
- `-run '^(TestSettingsWarnsOpens|TestSettingsParityWarnLimit|TestLegacyWarnSettingsRequests|TestSettingsControlActionAllowList|TestSettingsMainGrid|TestSettingsDeletionOpensFromMain|TestSettingsDeletionRowBudget|TestSettingsEditRegistry|TestSettingsMreqOpensFromMain)$' -v ./fuku/modules`: all 9 PASS.
- `go test -tags testtools -race -count=1 -run '^(TestSettingsLocaleKeys|TestSettingsLabelLengths|TestSettingsReusedKeyPlaceholders)$' ./fuku/i18n`: ok; `-run '^(TestSettings|TestLoadSettings)' ./fuku/i18n`: ok; `make check-translations`: "All translations are present!".
- **Whole `fuku/modules` package** (background, `-race -count=1 -timeout 30m -v`, log `/tmp/claude-0/-home-user-Fuku-Robot/4ce545b5-6832-57a3-a863-43c972932c7c/scratchpad/modules-18-01.log`): `ok github.com/uasneppy/fuku_robot/fuku/modules 1541.226s`, last line `exit 0`, `grep -cE -- '--- (FAIL|SKIP)|^FAIL'` prints 0, 1669 top-level `--- PASS` lines. It includes the settings family (`TestSettings*`, `TestLoadSettings*`). `grep -c -- '^--- PASS: TestLegacyWarnSettingsRequests'` prints 1 and `'^--- PASS: TestSettingsParityWarnLimit'` prints 1 (the plan's unanchored grep prints 24 and 7 because the subtests match too).
- Acceptance greps: first commit touching `warns_legacy_request_test.go` or `warns.go` after `29aca33` is `test(18-01): pin the warn settings commands' requests before the extraction`; `func TestLegacyWarnSettingsRequests(` 1, `func TestApplyWarnSettings(` 1, `func TestLoadWarnSettingsContext(` 1; `func applyWarnLimit(` 1, `func applyWarnMode(` 1, `applyWarnMode(` 2; `func LoadWarnSettingsContext(` 1 and removed lines in `repository.go` since `29aca33` 0; `TestSettingsWarnsOpens` and `TestSettingsParityWarnLimit` each defined once; `registerSettingsPage` outside comments in `settings_warns.go` 1; `settings_` keys per locale file 334 in all 8; `git diff --exit-code 29aca33 -- fuku/modules/settings_pages.go migrations fuku/db/models main.go` exit 0; `TestLegacyWarnSettingsRequests` in AGENTS.md 2.
- `go vet -tags testtools ./fuku/modules ./fuku/db/warns ./fuku/i18n` clean; `CGO_ENABLED=0 go build ./...` ok; `gofmt -l` on the 13 files this plan created or edited prints nothing; `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827` exit 0 (read directly, run before the code commit and again at the end): "no dupl, gocyclo or godox findings"; `git diff --exit-code 8c0c827 -- go.mod go.sum` exit 0.
- Not run: `make lint` (cannot run here, the installed golangci-lint is built with Go 1.25), `make test` (not this plan's task), the PostgreSQL proofs (the plan asks for none: no migration, no partial index, no CHECK was added), the separate `-run '^(TestSettings|TestLoadSettings)' ./fuku/modules ./fuku/i18n` command as one invocation (its two halves ran: the modules half inside the whole-package run, the i18n half on its own).

## Deviations from Plan

### Legitimate Phase 9 to 17 test changes (named as the plan requires; none re-recorded, no assertion weakened)

1. `TestSettingsControlActionAllowList` (`settings_mreq_test.go`): the "every control of the other pages still accepts tg and st alike" subtest now skips a control that declares an `Actions` list AND whose `Validate` accepts `v=1` (deviation note 1: the Warnings limit, and later the Phase 18 toggles). Every control it checked before is still checked (it still requires at least 8; the Captcha and Access half is unchanged). A new subtest, "the Warnings limit control accepts st only", asserts `tg` is refused in step 1 before any `getChatMember` (the `refused` closure) for v 1, 5, 50 and 100, that `st` is accepted for those values and refused for `""`, `x`, `-1`, `1000` and `05x`.
2. The five Main-shape assertions now read three constants added to `settings_main_test.go` (`settingsMainShapeGroup = "s|l / c|f / n|w / g|r / m|d / a / close"`, `settingsMainShapePrivate` (ending `groups|close`) and `settingsMainOwnerRows = 7`): `TestSettingsDeletionOpensFromMain` (shape and row count), `TestSettingsDeletionRowBudget` (row count), `TestSettingsEditRegistry` (group shape), `TestSettingsMreqOpensFromMain` (group and private shapes). The hand-built subtests of `TestSettingsMainGrid` keep their own literals.
3. `fuku/i18n/settings_locale_test.go`: four limits rows and the 🚩 entry in the page list of `TestSettingsLabelLengths` (the table must cover exactly the `settings_` keys).

### Auto-fixed

None.

### Process notes

- Planned acceptance line "`grep -c -- '--- PASS: TestLegacyWarnSettingsRequests'` prints 1" is only true anchored (`^--- PASS`), because the subtests print the same prefix; recorded above with both counts.
- `TestApplyWarnSettings` checks that a write drops `warn_settings:<chat>` by reading through the command's cached getter after the write (the read-back immediately re-caches the row, so absence of the key cannot be observed); a failed write leaves the cached value unchanged.
- A stray file `/warn-got.txt` (a temporary capture from a mistyped redirect with an unset variable, outside the repository) exists in the filesystem root; the sandbox refused to let me delete it. It holds only captured test output and belongs to no repository path.
- The golden's `warns_mode_unknown` replies are recorded in uk (the test language default) with the real locale files.

## Requirements

`requirements-completed: []`. PAGE-01, PAGE-10 and PLAT-05 are contributed to (the warn limit from the menu, the first two apply functions, the reader and the first parity test, four keys in 8 locales) and stay Pending; only plan 18-15 completes PAGE-01 to PAGE-06 and PAGE-10 and re-confirms PLAT-05. `requirements.mark-complete` was not called.

## Known Stubs

None. The page deliberately shows only the limit line and the hint; the action line, the action choice row and the inert line are plan 18-03's (their keys arrive with 18-02) and Main's state line already shows the stored action.

## Threat Flags

None. T-18-01 (forged limit press) is covered by the step-1 `Validate` and the `st`-only allow-list plus `applyWarnLimit`'s refusal of 0 and over 100; T-18-02 (an admin the command refuses) by `settingsRightAdminFloodBot` and the bot-not-admin parity row; T-18-03 (the extraction changing the commands) by the golden recorded first; T-18-04 (the reader creating a row or caching a default) by `TestLoadWarnSettingsContext` and `TestSettingsWarnsOpens`. No new endpoint, auth path, migration, model, Redis key family, handler group or shutdown drain.

## Self-Check: PASSED

- Created files `fuku/modules/warns_legacy_request_test.go`, `fuku/modules/warns_settings_apply_test.go`, `fuku/db/warns/load_settings_test.go`, `fuku/modules/settings_warns.go`, `fuku/modules/settings_warns_test.go`: FOUND.
- Commits 871d39c, 07ea430, d342f24, ef11c15: all reachable from HEAD.
