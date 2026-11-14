---
phase: 18-remaining-settings-pages
plan: 11
subsystem: settings-menu
tags: [telegram, locks, apply-function, golden, lock-table]
status: complete

requires:
  - phase: 18-remaining-settings-pages
    provides: "plan 18-02 (the 22 settings_lock_* name keys, the Locks words, the page titles and the log category names with their limits rows) and plan 18-09 (settingsLogCategories)"
provides:
  - "TestLegacyLockRequests: /lock, /unlock, /locks and /locktypes pinned before the extraction (19 cases, chats from -1009918001801): the bot not an administrator, a non-admin, no argument, an unknown name and an upper-case name refusing the whole call before any write, one lock, several, a restriction with a lock, one of several failing to save, unlock never locked, plus the /locks text that follows each write, /locktypes and an empty /locks"
  - "applyLock(ctx, chatID, kind, on) in locks.go with the lockWrites seam and the typed result (lockApplied, lockUnknownType, lockSaveFailed; the map read back through locks.GetChatLocksCachedContext), and applyLockList, the shared loop /lock and /unlock write their validated names through"
  - "settingsLockRows and settingsLockPageSize in settings_locks.go: the 22 locks with two-letter codes, name keys and pages (12 and 10), held equal to /locktypes by tests"
affects: [18-12, 18-13, 18-15]

plan_head_before: d5ec0b280ae1d497412f799d518d002c06e5766b
plan_head_after: b5d302f

actuals:
  tokens: 9200   # chars/4 over the added non-planning lines of d5ec0b2..b5d302f (36922 chars), tests and AGENTS.md included
  tasks: 2
  commits: 3   # MEASURED: git rev-list --count d5ec0b2..HEAD (before the SUMMARY and state commits)

key-files:
  created:
    - fuku/modules/locks_legacy_request_test.go
    - fuku/modules/locks_apply_test.go
    - fuku/modules/settings_locks.go
    - fuku/modules/settings_locks_test.go
  modified:
    - fuku/modules/locks.go
    - AGENTS.md

key-decisions:
  - "The lock write loops of /lock and /unlock became one helper, applyLockList(chatID, kinds, on), instead of two inline loops calling applyLock: the loops were symmetric and the helper keeps the same failure list, the same Warn line per failed save (verb lock or unlock derived from on) and avoids a new dupl finding, with the existing //nolint:dupl on the two commands untouched."
  - "Control codes are the UI-SPEC's two-letter codes, never the lock's name (D-09 and the UI-SPEC against D-18's allowance); the kind lives only in the table."
  - "A failed read-back is still lockApplied with a nil map (the write landed), so a caller must not treat a nil map as a failure."

patterns-established:
  - "A table-field guard test for a page that no make target can check: the row table is held equal to the command's own source of truth in both directions, and every key it reads is translated in all 8 locales."

requirements-completed: []

coverage:
  - id: D1
    description: "/lock and /unlock write each lock through applyLock with every request and stored row unchanged, the all-or-nothing validation and the failure list included"
    requirement: "PAGE-06, PAGE-10 (contribution only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^(TestLegacyLockRequests|TestApplyLock|TestLockAndUnlockCommandsPersistSettings)$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D2
    description: "The 22 locks are named in one table held equal to /locktypes, with unique two-letter codes that avoid the pager and the core actions, and every name, Locks word, page title and log category name translated in 8 locales"
    requirement: "PAGE-06, PLAT-05 (contribution only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^(TestSettingsLocksTableMatchesCommand|TestSettingsLocksKeysExist|TestDeletionKindKeysExist)$' ./fuku/modules"
        status: pass
    human_judgment: false
---

# Phase 18 Plan 11: the lock write and the lock table Summary

**Every lock is written by one function, `applyLock`, behind a golden recorded first, and one table names the 22 locks the Locks page will show, held equal to the commands and fully translated.**

## What changed

- **Task 1, tracer (commits 1593804, 92216f0).** `TestLegacyLockRequests` (`locks_legacy_request_test.go`, chats from `-1009918001801`, a real GORM create callback that fails one lock type of one chat) was committed alone and run green on the unchanged code in the CI environment and bare. The expected requests were recorded from the unchanged code with a temporary file hook that is not in the commit. Then `locks.go` gained `lockWrites`, `lockApplyResult`, `errLockUnknownType`, `applyLock` and `applyLockList`; `lockPerm` and `unlockPerm` keep their gates, pre-validation, replies and `//nolint:dupl`, and call `applyLockList` where they called `locks.UpdateLock` in a loop. `TestApplyLock` (every kind both ways, the cache drop, a repeated call, six unknown shapes with no write, a failing write, a failed read-back that is still an applied write, the commands going through the function with only check and reply Telegram calls) is new. AGENTS.md: the "One setting, one code path" bullet gained the Locks paragraph.
- **Task 2 (commit b5d302f).** `settings_locks.go`: `settingsLockPageSize` (12), `settingsLockRow{Code, Kind, NameKey, Page}` and `settingsLockRows` (22 rows in the UI-SPEC order), registering no page. `TestSettingsLocksTableMatchesCommand` (set equality with `getLockMapAsArray` in both directions, 22 rows, two lower-case letters, unique, not `kg`, not a core action code, not the lock's own name, name key `settings_lock_<kind>`, pages 12 and 10 in order, `all` last) and `TestSettingsLocksKeysExist` (the 22 name keys, the three Locks words, the six Phase 18 page titles and the six log category names, 8 languages). AGENTS.md: the table sentence in the same paragraph.

## Verification (real results)

- Task 1 verify: `TestLegacyLockRequests` (19 subtests), `TestApplyLock`, `TestLockAndUnlockCommandsPersistSettings` pass, CI environment and bare. The golden passed unchanged before and after the extraction.
- Task 2 verify: `TestSettingsLocksTableMatchesCommand`, `TestSettingsLocksKeysExist`, `TestDeletionKindKeysExist` pass in both environments.
- Focused run `^(TestLegacy|TestSettingsMain|TestSettingsLabelLengths|TestApplyLock|TestSettingsLocks)` on `./fuku/modules ./fuku/i18n`: `ok` in the CI environment (modules 16.1 s) and bare (16.3 s).
- Whole `fuku/modules` package, CI environment (`cienv.sh`), `go test -tags testtools -race -count=1 -timeout 30m -v ./fuku/modules` (log `/tmp/claude-0/-home-user-Fuku-Robot/4ce545b5-6832-57a3-a863-43c972932c7c/scratchpad/modules-18-11.log`): `ok  github.com/uasneppy/fuku_robot/fuku/modules 1594.689s`, last line `exit 0`, `grep -cE -- '--- (FAIL|SKIP)|^FAIL'` = 0, 1740 top-level `--- PASS` lines, with `TestLegacyLockRequests`, `TestApplyLock`, `TestSettingsLocksTableMatchesCommand` and `TestSettingsLocksKeysExist` among them. `./fuku/db/locks` and `./fuku/i18n`: `ok`.
- `CGO_ENABLED=0 go build ./...` ok; `go vet -tags testtools ./fuku/modules ./fuku/db/locks ./fuku/i18n` clean; `gofmt -l` on the five Go files prints nothing; `make check-translations` all present; `make check-docs` no drift; `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827` exit 0 ("no dupl, gocyclo or godox findings"), read directly; `git diff --exit-code 8c0c827 -- go.mod go.sum` exit 0; `git diff --exit-code 29aca33 -- fuku/db/locks fuku/db/models migrations` exit 0; no `t.Skip`, `reflect.Ptr` or `ioutil.` in the new files; no `//nolint` added (the count equals 29aca33's).
- Not run: `make lint` (the installed golangci-lint is built with Go 1.25, a documented sandbox fact), gosec, `make test`.

## Deviations from Plan

None needing a production fix. Notes:

1. **[Design] `applyLockList` instead of two inline loops.** The plan says the write loops call `applyLock`; a shared helper does exactly that and keeps the failure list, the Warn line and the replies byte for byte, while avoiding duplicate loop bodies.
2. **[Plan text] No locale key added.** The 22 lock names, the Locks words, the six page titles and the category names all exist from plan 18-02 with their limits rows, so this plan adds none.
3. **[Plan text] The golden's `/lock` bot-not-admin case** pins the first refusal the fake reaches (`chat_status_bot_not_admin`, the bot's member status faked as a plain member), as the plan allows.
4. **No test of "no Telegram request" on `applyLock` alone**, because the function takes no bot; the claim is proved at the command level (only `sendMessage` and the check calls, `TestApplyLock`'s last subtest).

**Total deviations:** 0 auto-fixed, 4 notes. **Impact:** none on the contract.

## Known Stubs

`settingsLockRows`, `settingsLockPageSize` and the `settingsLockRow` fields have no production reader yet: plan 18-12 registers the Locks page that reads them (the brief's rule for a table a later plan of this phase is the only production user of; plan 18-11's own tests use all of it). `applyLock`'s production callers today are `/lock` and `/unlock` through `applyLockList`.

## Threat Flags

None. No endpoint, auth path or schema is added. T-18-37 (the extraction changing which locks a multi-name call writes) is mitigated by the golden recorded first; T-18-38 (a forged kind from a page) by `applyLock`'s refusal, tested for six shapes; T-18-39 (table drift) by `TestSettingsLocksTableMatchesCommand`; T-18-SC by an unchanged go.mod and go.sum.

## Requirements

`requirements-completed: []`. PAGE-06, PAGE-10 and PLAT-05 stay Pending; only plan 18-15 completes them.

## Self-Check: PASSED

- Created files exist on disk (`locks_legacy_request_test.go`, `locks_apply_test.go`, `settings_locks.go`, `settings_locks_test.go`) and the commits are ancestors of HEAD: 1593804, 92216f0, b5d302f.
- Acceptance greps re-run: the first commit touching `locks_legacy_request_test.go` or `locks.go` starts with `test(18-11):`; `func TestLegacyLockRequests(`, `func TestApplyLock(`, `func applyLock(`, `TestSettingsLocksTableMatchesCommand` and `TestSettingsLocksKeysExist` print 1 each; `settingsLockRows` appears in `settings_locks.go`; `registerSettingsPage` appears 0 times outside comments there.
