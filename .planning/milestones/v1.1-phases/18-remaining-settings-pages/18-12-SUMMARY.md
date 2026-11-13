---
phase: 18-remaining-settings-pages
plan: 12
subsystem: settings-menu
tags: [telegram, locks, settings-page, pager, right-line, main-grid]
status: complete

requires:
  - phase: 18-remaining-settings-pages
    provides: "plan 18-11 (applyLock, settingsLockRows, settingsLockPageSize), plan 18-09 (settingsToggleGrid, the table-driven toggle page pattern), plan 18-06 (settingsToggleSet), plan 18-02 (the Locks strings), and the Phase 16 right line (settingsDelReadRight)"
provides:
  - "The Locks page (page k, Order 44, icon 🔐) in settings_locks.go: 22 tg-only controls generated from settingsLockRows (Validate 0 or 1, settingsRightAdminFloodBot), each writing exactly its lock through applyLock via settingsToggleSet and redrawing the page the lock is on; the kg pager action (exactly v, one or two digits, clamped to 1..2, settingsRightAdmin); settingsLocksRead (locks.GetChatLocksCachedContext, error returned, no row created); Main's {n} locked line; the pure renderSettingsLocks"
  - "The right line on every draw: one live getChatMember of the bot through settingsDelReadRight (the Deletion page's machinery, no new pacer, key or drain), made after the press is answered and before the edit; none for a forged or alert-refused press"
  - "The final Main grid s|l / c|f / n|k / w|g / r|m / d|t / u|o / y, Access and the navigation row (10 rows for the owner), through the Main-shape constants of settings_main_test.go"
  - "A failed write shows the command's own locks_lock_failed or locks_unlock_failed naming the lock by its localized name, escaped"
  - "Tests: TestSettingsLocksOpens, TestSettingsParityLock, TestSettingsLocksPager, TestSettingsLocksRightLine, TestSettingsLocksForged, TestSettingsLocksFailedWrite, TestSettingsDriftLocks, TestSettingsAuthorityParityLocks, TestSettingsLocksReadsCreateNothing, TestSettingsLocksCallbackBytes, TestSettingsLocksPageFitsEveryLocale, TestSettingsLocksCost"
affects: [18-13, 18-15]

plan_head_before: 0ef26d77bc33b4000d2a10a82b7cb9d3c8e69188
plan_head_after: a4f04f97b8047cca90e41348d0a01ae43eddcd02

actuals:
  tokens: 16000   # chars/4 over the added non-planning lines of 0ef26d7..a4f04f9 (64219 chars), tests and AGENTS.md included
  tasks: 3
  commits: 4   # MEASURED: git rev-list --count 0ef26d7..a4f04f9 (before the SUMMARY and state commits)

key-files:
  created:
    - fuku/modules/settings_locks_screens_test.go
  modified:
    - fuku/modules/settings_locks.go
    - fuku/modules/settings_locks_test.go
    - fuku/modules/settings_main_test.go
    - AGENTS.md
    - README.md
    - docs/src/content/docs/api-reference/callbacks.md
    - docs/src/content/docs/api-reference/permissions.md
    - .planning/phases/18-remaining-settings-pages/18-SPIKE.md

key-decisions:
  - "The earlier executor's uncommitted draft of settings_locks.go, settings_locks_test.go and settings_main_test.go was verified line by line against the plan and the UI-SPEC and kept as written: it compiled, vetted and passed its two tests unchanged. Only the tests of tasks 2 and 3 and the docs were added on top."
  - "The draft's pager and right line (the plan's task 2) sit in the task 1 commit, because the draft's files could not be split by path; task 2's commit carries their tests and the AGENTS.md completion."
  - "A press refused with a NOTICE (the bot not an administrator by the cached list) redraws through the framework's common open path, so that draw states the right too; only a forged press (step 1) or an alert refusal (a plain member, the owner gate) makes no read of the bot. The plan's 'none for a press refused by the right' is read as the alert refusals."

patterns-established:
  - "A test that counts the right line's one read warms the chat's admin cache while the bot reads as an administrator (settingsLocksWarmBotRight) and only then scripts the bot's record: a cold admin cache loads through a getChatMember of the bot of its own, which belongs to the pipeline's cached bot-administrator predicate and not to the page."

requirements-completed: []

coverage:
  - id: D1
    description: "A lock is turned on or off from the page exactly as /lock and /unlock turn it: same rows, same locks_map state, same /locks listing, the page the lock is on redrawn, a double tap writing the same row, the bot not an administrator refused with the command's notice"
    requirement: "PAGE-06, PAGE-10 (contribution only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^(TestSettingsLocksOpens|TestSettingsParityLock)$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D2
    description: "The page turns with kg (Next only on page 1, Prev only on page 2, v clamped), every draw states the bot's delete right once from exactly one live read made after the press is answered, forged presses are refused in step 1 with no read, and a failed write redraws the stored state naming the lock"
    requirement: "PAGE-06 (contribution only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^(TestSettingsLocksPager|TestSettingsLocksRightLine|TestSettingsLocksForged|TestSettingsLocksFailedWrite)$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D3
    description: "The page draws what the lock watchers read and refuses what /lock refuses (admin, plain member, bot not admin, owner gate carve-out), creates no row on a read, and keeps its bytes (46/42 toggle, 37/33 pager), its row budget, its length in 8 languages and its call cost"
    requirement: "PAGE-06, PAGE-10 (contribution only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^(TestSettingsDriftLocks|TestSettingsAuthorityParityLocks|TestSettingsLocksReadsCreateNothing|TestSettingsLocksCallbackBytes|TestSettingsLocksPageFitsEveryLocale|TestSettingsLocksCost)$' ./fuku/modules"
        status: pass
    human_judgment: false
---

# Phase 18 Plan 12: the Locks page Summary

**All 22 lock types are turned on or off from a paged Locks page through `applyLock`, exactly as `/lock` and `/unlock` do; every draw says once whether the bot can delete; Main shows the final 15-cell grid.**

## What changed

- **Task 1, tracer (commit 595f462).** `settings_locks.go`: the page registration (page `k`, `Order` 44, icon 🔐, no `Hidden`), 22 `tg`-only controls built from `settingsLockRows` (`settingsRightAdminFloodBot`, `settingsEditSwitchValue`), `settingsLocksRead`, `settingsLocksMainState` (`{n} locked`, the unknown mark on a failed read, no Telegram call), the pure `renderSettingsLocks` (intro, one state line per lock of the page in words, the `staff_panel_page` line, the right-line slot, the toggle grid with 🔒 and 🔓, the pager row, Back and Close), `settingsLocksBuild` (read, answer the press empty, one right read, draw) and a `Set` per row through `settingsToggleSet` and `applyLock`. The Main-shape constants are the final grid and 10 owner rows. AGENTS.md: page `k` in the callbacks list, the final-grid sentence and a Locks page bullet.
- **Task 2 (commits 6c5505a, f597762).** `TestSettingsLocksPager`, `TestSettingsLocksRightLine`, `TestSettingsLocksForged`, `TestSettingsLocksFailedWrite` and the AGENTS.md bullet's completion (pager, failure notice, cost, tests); then the docs: README, `callbacks.md` (page `k`, its 22 codes, `kg`, the largest buttons), `permissions.md` (`Locks: each lock on or off | Admin, and the bot an admin | /lock, /unlock`) and three new owner checks in `18-SPIKE.md` (17 the broad locks in a real group, 18 the 22 cells on a narrow phone and the right line, 19 whether `all` and `messages` should ask for a card).
- **Task 3 (commit a4f04f9).** `TestSettingsDriftLocks` (a `/lock url` or a page write is what the real `permHandler` deletes by, across the cache), `TestSettingsAuthorityParityLocks` (admin, plain member, bot not admin with the pager needing only the admin check, the owner-gate carve-out), `TestSettingsLocksReadsCreateNothing` (no lock or chats row; a failing reader is the cannot-open edit on the page, the pager and a toggle, and the unknown mark on Main) and, in the new `settings_locks_screens_test.go`, `TestSettingsLocksCallbackBytes` (46/42 and 37/33), `TestSettingsLocksPageFitsEveryLocale` (8 languages, three 64-rune titles, three right-line states, every lock's failure notice, widths `{2,2,2,2,2,2,1,2}` and `{2,2,2,2,2,1,2}`, the row budget 8 private and 9 group, cells at most 20 code points) and `TestSettingsLocksCost`.

## The draft kept and what changed from it

Kept without a code change: the whole of `settings_locks.go` (it was complete, not cut off), `newSettingsLocksEnv` and its helpers, `TestSettingsLocksOpens`, `TestSettingsParityLock`, the Main-shape constants. Added on top: everything under task 2 and 3 above, the docs and the AGENTS.md bullets. Nothing from the draft was discarded.

## Verification (real results)

- Task 1 verify (the plan's list with `TestSettingsMainGrid`, `TestSettingsDeletionOpensFromMain`, `TestSettingsDeletionRowBudget`, `TestSettingsEditRegistry`, `TestSettingsMreqOpensFromMain`, `TestSettingsControlActionAllowList`, `TestLegacyLockRequests`, `TestSettingsLocksTableMatchesCommand`): all pass, CI environment.
- Task 2 and 3 verify blocks pass; `make check-docs`: no drift.
- Focused run `^(TestSettingsLocks|TestSettingsParityLock|TestSettingsDriftLocks|TestSettingsAuthorityParityLocks|TestLegacy|TestSettingsMain|TestSettingsLabelLengths|TestSettingsStaffPrivacy|TestSettingsControlActionAllowList)` on `./fuku/modules ./fuku/i18n`: `ok` in the CI environment (modules 93.8 s) and bare (97.5 s).
- Whole `fuku/modules`, CI environment, `go test -tags testtools -race -count=1 -timeout 30m -v ./fuku/modules` (log `/tmp/claude-0/-home-user-Fuku-Robot/4ce545b5-6832-57a3-a863-43c972932c7c/scratchpad/modules-18-12.log`): `ok  github.com/uasneppy/fuku_robot/fuku/modules  1721.524s`, last line `exit 0`, 0 `--- FAIL`, 0 `--- SKIP`, 7763 `--- PASS`. Note: 1721 s is close to the 30 minute timeout of that command on this machine (CI allows 30 minutes per package).
- `./fuku/i18n`: `ok` (23.8 s) in the CI environment.
- `CGO_ENABLED=0 go build ./...` ok; `go vet -tags testtools ./fuku/modules ./fuku/i18n` clean; `gofmt -l` on the four Go files prints nothing; `make check-translations` all present; `make check-docs` no drift; `git diff --exit-code 8c0c827 -- go.mod go.sum` and `git diff --exit-code 29aca33 -- fuku/modules/settings_pages.go fuku/modules/deletion_right.go migrations fuku/db/models main.go` both exit 0; `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827` exit 0 (read directly): no dupl, gocyclo or godox findings.
- Not run: `make lint` (the installed golangci-lint is built with Go 1.25, a documented sandbox fact), gosec, `make test`.

## Deviations from Plan

None needing a production fix. Notes:

1. **[Commit split] The pager and the right line are in the task 1 commit**, not the task 2 commit, because the draft's files carried them already and a split by path was impossible. Task 2's commit carries their tests.
2. **[Plan text] "None for a press refused by the right."** A press refused with a notice (the bot not an administrator by the cached list) redraws through the framework's common open path, which states the right too (so the redraw is one more read); it is the same for every page that redraws on a notice. A forged press and an alert refusal (a plain member, the owner gate) make no read; `TestSettingsLocksRightLine` and `TestSettingsLocksCost` hold that, and the AGENTS.md bullet says it.
3. **[Test fact] The pipeline's bot check loads the admin cache with a `getChatMember` of the bot while the cache is cold** (`IsUserAdmin(bot)` of `settingsRightAdminFloodBot`). The page is not the cause; the cost tests warm the cache first so they count the right line's one read, and AGENTS.md states the fact.
4. **No locale key added**; the Locks strings exist from plan 18-02 and `staff_panel_*` are reused.

**Total deviations:** 0 auto-fixed, 4 notes. **Impact:** none on the contract.

## Known Stubs

None. `settingsLockRows`, `settingsLockPageSize` and the row fields now have a production reader (the Locks page).

## Threat Flags

None. No endpoint, auth path or schema is added. T-18-40 (a presser `/lock` refuses) is mitigated by `settingsRightAdminFloodBot` and `TestSettingsAuthorityParityLocks`; T-18-41 (forged code, kind, value or page) by the `Actions`, `Validate` and `kg` field checks and `TestSettingsLocksForged`; T-18-43 (the right line's read per draw) by one call per draw through the shared deletion pacer and `TestSettingsLocksCost`; T-18-42 (a stray tap on `all` or `messages`) is accepted, with the owner's decision in `18-SPIKE.md` check 19.

## Requirements

`requirements-completed: []`. PAGE-06 and PAGE-10 stay Pending; only plan 18-15 completes them.

## Self-Check: PASSED

- `fuku/modules/settings_locks.go`, `settings_locks_test.go`, `settings_locks_screens_test.go` exist; commits 595f462, 6c5505a, f597762, a4f04f9 are ancestors of HEAD.
- Acceptance greps: `grep -c 'func TestSettingsLocksOpens(\|func TestSettingsParityLock(' settings_locks_test.go` finds both; `registerSettingsPage` once in non-comment lines of `settings_locks.go`; `applyLock(` and `"kg"` present (`settingsActLocksPage = "kg"`); `grep -c 'n|k / w|g / r|m / d|t / u|o / y' settings_main_test.go` prints 2; `grep -c '/unlock\b' permissions.md` prints 1; the three screens tests are in `settings_locks_screens_test.go` and the three drift, authority and reads tests in `settings_locks_test.go`.
