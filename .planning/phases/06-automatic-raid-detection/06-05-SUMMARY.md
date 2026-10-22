---
phase: 06-automatic-raid-detection
plan: 05
subsystem: lockdown
tags: [raid-detection, settings, gotgbot, gorm-upsert, cache-invalidation, anonymous-admin, i18n]

requires:
  - phase: 06-automatic-raid-detection
    provides: raiddetection.GetSettings and Defaults, ChatRaidDetection, raidSettingsLookup, the join surge and flood triggers that read the settings
provides:
  - /autolockdown, registered through WrapCommand and RegisterAnonymousAdminHandler, showing both triggers and changing either one
  - autolockdownApply, the whole command body, reusable by the /autoantiraid alias of plan 06-10
  - raiddetection.SetJoinEnabled, SetJoinThreshold, SetFloodEnabled, SetFloodThreshold and ErrThresholdOutOfRange (column-targeted upserts with DeleteCache)
  - raid_detection keys excluded from the in-process cache layer (skipLocal)
  - eleven lockdown_auto_* locale keys in all seven locale files
affects: [06-06, 06-10, 06-11, 06-13, phase-07-ai-trigger]

actuals:
  tokens: 16000
  tasks: 2
  commits: 3

plan_head_before: 946dd04c83f44b06bebf81d7ecbd97ebe074111a
plan_head_after: 254b884a14b0f36c33c319d483d480c97e7247d1

commits: 3

tech-stack:
  added: []
  patterns:
    - "One INSERT ... ON CONFLICT DO UPDATE per column set, with the other columns carried as Go defaults on first insert"
    - "One command, two authorities: the check picks view or change by whether arguments follow, both through the live requireLockdownAuthority"
    - "A package-variable seam (autolockdownWrite) so a test can make the save fail"

key-files:
  created:
    - alita/modules/autolockdown.go
    - alita/modules/autolockdown_test.go
    - alita/db/raiddetection/repository_test.go
    - alita/db/raiddetection/testmain_test.go
  modified:
    - alita/modules/lockdown.go
    - alita/db/raiddetection/repository.go
    - alita/db/cache/local.go
    - alita/db/cache/local_test.go
    - AGENTS.md
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml

key-decisions:
  - "Authority is chosen in requireAutoLockdownAuthority by whether arguments follow, using requireLockdownAuthority(false) to view and requireLockdownAuthority(true) to change, so anonymous admins, the sender_chat refusal and the live-lookup failure path are inherited, not copied"
  - "A number sets the threshold and turns the trigger on; on and off keep the stored threshold (D-21); a number too long for 64 bits is a range error, a decimal or a plus sign is a usage error"
  - "The repository also refuses an out-of-range threshold (ErrThresholdOutOfRange) and the command maps it to the range reply, so the 3..500 rule holds even if a caller skips the parser"
  - "The cache key literal \"raid_detection\" is written at the DeleteCache call, next to the GetSettings constant, so the write and read keys stay greppable together"

patterns-established:
  - "Settings commands call autolockdownApply(b, ctx, args); an alias only prepends words"

requirements-completed: [RAID-06]

duration: 18min
completed: 2026-10-18
status: complete
---

# Phase 6 Plan 05: Admin control of the automatic triggers Summary

**`/autolockdown` shows the join surge and flood triggers to any live admin and lets the owner or an administrator who can restrict members switch either off or set its threshold (3 to 500), with one column-targeted upsert per trigger, a cache key kept out of the in-process layer, and the next burst following the new value on any replica.**

## Performance

- **Duration:** about 18 min
- **Started:** 2026-10-18T12:33Z (ledger written 12:39Z)
- **Completed:** 2026-10-18T12:51Z
- **Tasks:** 2
- **Files modified:** 16 (4 created)

## Accomplishments

- `/autolockdown` with no words replies with the title, the joins line and the flood line (on or off with the threshold) and the hint, to any live creator or administrator. `/autolockdown joins|flood N|on|off` changes one trigger and replies "Saved" followed by the fresh status.
- A change needs the creator or an administrator with `can_restrict_members`, judged by a live `getChatMember` (never the admin cache); a member, a failed lookup, a private chat, a message sent as another chat and a tapper without the right are all refused with nothing saved. An anonymous admin gets the proof first, then the tapper is checked live and the cached command's arguments pick view or change.
- Four repository writes, each a single `INSERT ... ON CONFLICT (chat_id) DO UPDATE` of only its own columns plus `updated_at`, followed by `cache.DeleteCache`. A Go `false` is always written, a first write inserts the other trigger's defaults, and a threshold outside 3..500 returns `ErrThresholdOutOfRange` before any write.
- `raid_detection:` is in `skipLocal`; `TestSkipLocalMatchesFreshnessCriticalKeys` pins it (and that `raid_detectionx` is still cached locally).
- End to end: `/autolockdown joins 20` then 19 joins do not lock and the 20th does (trigger count 20, after `ResetLocalForTest`); `joins off` keeps 25 joins from locking and `joins on` keeps the threshold 15 so the next join locks; `flood off` stops a 30-item flood from recent joiners while a join surge of 15 still locks.
- A basic group gets the existing `lockdown_basic_group` text, for viewing and for changing, and saves nothing. A failing save replies `lockdown_auto_save_failed`, never "Saved", and the stored value stays.
- AGENTS.md carries the `/autolockdown` authority rule (Permissions) and the settings write and `skipLocal` rule (Data, the `chat_raid_detection` bullet).

## Task Commits

1. **Task 1: an admin changes a trigger and the next burst follows it, on any replica**
   - `7e7ad5c` (test) TestAutoLockdown, TestRaidDetectionSettings, the extended skipLocal test, with compile scaffolding in the repository
   - `a242911` (feat) skipLocal entry, the four upserts, autolockdown.go, registration, 77 locale lines, AGENTS.md
2. **Task 2: authority, grammar, range and basic groups pinned**
   - `254b884` (test) TestAutoLockdownAuthority, TestAutoLockdownArgs, TestAutoLockdownBadInput, TestAutoLockdownBasicGroup, TestAutoLockdownSaveFails (no implementation commit was needed, see below)

**Plan metadata:** the docs commit adding this file.

## TDD Gate Compliance

- Task 1 RED (`7e7ad5c`): needed compile scaffolding, which the plan allows. The repository tests name `ErrThresholdOutOfRange` and the four setters, so the commit carries them as stubs that write nothing. Each target test then failed on its own assertion: `TestAutoLockdown` ("the bot sent nothing to the lockdown chat": the command did not exist), `TestRaidDetectionSettings` (for example "no row after SetJoinEnabled(false)", "join threshold after the write = 15, want 20") and `TestSkipLocalMatchesFreshnessCriticalKeys` ("raid_detection key not excluded from the local layer"). No test failed on a compile, fixture or unrelated error. `autolockdown.go` was left out of the RED commit because Task 1's module test uses none of its symbols. GREEN (`a242911`) made all three pass.
- Task 2: all cases passed when first run, because Task 1's `requireAutoLockdownAuthority`, `parseAutoLockdownArgs` and `autolockdownApply` already implemented them. This is an unexpected GREEN on the test commit, investigated rather than ignored. As the plan allows, the test commit stands alone, and each guard was shown able to fail with a temporary mutation, reverted afterwards (`git diff` of `autolockdown.go` is clean against `a242911`):
  - replacing the change check with the view check failed `an admin without the restrict right may view but not change` and `an anonymous admin who taps without the restrict right saves nothing` (the row was written);
  - replacing the supergroup check failed both `TestAutoLockdownBasicGroup` subtests;
  - loosening the parser's minimum to 1 failed `TestAutoLockdownArgs/below the range`. `TestAutoLockdownBadInput/below the range` still passed under that mutation, because the repository refuses the value and the command maps it to the same range reply: two independent guards.
- There is therefore no `fix(06-05)` commit.

## Deviations from Plan

None - plan executed exactly as written. The only choices beyond its wording were an extra repository guard mapped to the range reply (the plan's repository already returns `ErrThresholdOutOfRange`), the parser treating an over-long digit string as a range error, and the sender-as-another-channel, private-chat and view-by-anonymous-admin cases added to the authority test.

## Issues Encountered

- `make lint` cannot run here (golangci-lint is built with an older Go than the module needs). `gofmt -l` on the touched files and `go vet -tags testtools ./alita/modules ./alita/db/raiddetection ./alita/db/cache` stand in and are clean. `alita/modules/greetings_command_test.go` is not gofmt-clean on the base commit and was left alone.
- No PostgreSQL server exists in this sandbox and none was started. The upsert's `ON CONFLICT (chat_id)` was exercised on SQLite through the model's unique index; the PostgreSQL proof of `TestRaidDetectionSettings` is plan 06-13 (the integrity list).
- Some long shell commands were refused by the worktree guard and re-run as separate, plain commands or through the edit tools; nothing was lost.
- The full `make test` was not run (the orchestrator owns it).

## Verification Run

| Check | Result |
|-------|--------|
| `go test -tags testtools -race -run '^TestAutoLockdownAuthority$\|^TestAutoLockdownArgs$\|^TestAutoLockdownBasicGroup$\|^TestAutoLockdownBadInput$\|^TestAutoLockdownSaveFails$\|^TestAutoLockdown$' ./alita/modules` | pass (all subtests) |
| `go test -tags testtools -race -run 'Lockdown\|Unlockdown\|Raid\|Anon' ./alita/modules` | pass (121 s) |
| `go test -tags testtools -race ./alita/db/raiddetection ./alita/db/cache` | pass |
| `go test -tags testtools -run 'TestLockdownLocaleKeys\|TestLockdownStringsHaveBalancedTags\|TestLockdownRepliesCarryNoTelegramDetail' ./alita/i18n` | pass |
| `make check-translations`, `make check-docs` | clean (no drift) |
| `CGO_ENABLED=0 go build ./...`, `go vet`, `gofmt -l` | clean |
| `git diff --exit-code go.mod go.sum` | clean |
| Task 1 and Task 2 acceptance criteria (`raidDetectionPrefix` twice, literal `DeleteCache(cache.CacheKey("raid_detection"`, `RegisterAnonymousAdminHandler("autolockdown"`, `lockdown_auto_saved` once per locale, AGENTS.md mentions, `requireAutoLockdownAuthority` and `parseAutoLockdownArgs` defined) | all met |

## Known Stubs

None. The RED-commit stubs were replaced in `a242911`.

## Threat Flags

None beyond the plan's register. T-06-15: every change goes through `requireLockdownAuthority(true)`, live and uncached, and the tests cover a stale admin cache, a failed lookup and an anonymous tapper without the right. T-06-16: the parser and the repository both enforce 3..500 before any write. T-06-17: the key is in `skipLocal`, every write deletes it, and `TestAutoLockdown` reads after `ResetLocalForTest`. T-06-18: one column-targeted upsert per trigger (`TestRaidDetectionSettings`). T-06-SC: no installs, `go.mod` and `go.sum` unchanged.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Plan 06-10 can register `/autoantiraid` as `autolockdownApply(b, ctx, append([]string{"joins"}, args...))`.
- Plan 06-13 should add `TestRaidDetectionSettings` to the PostgreSQL integrity list.
- `docs/` is unchanged: `make check-docs` reports no drift for the new command, so a documentation page for `/autolockdown` belongs to the later documentation plan of the phase.

---
*Phase: 06-automatic-raid-detection*
*Completed: 2026-10-18*

## Self-Check: PASSED

- Created files exist: `alita/modules/autolockdown.go`, `alita/modules/autolockdown_test.go`, `alita/db/raiddetection/repository_test.go`, `alita/db/raiddetection/testmain_test.go`.
- Commits `7e7ad5c`, `a242911` and `254b884` are in `git log`; `git rev-list --count 946dd04..HEAD` was 3 before this file.
- `STATE.md` and `ROADMAP.md` are untouched.
