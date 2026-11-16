---
phase: 261113-tjj-staff-group-forward-source-blocklist-blo
plan: 01
subsystem: staff
tags: [telegram, staff-group, forward-blocklist, settings-menu, gorm, postgres, redis, i18n]

requires:
  - phase: staff-group
    provides: Staff Groups, links, recheckLink, chat_status.CheckOwner, the staff pacer and the staff commands
  - phase: settings-menu
    provides: the list engine (Blacklist, Filters, Notes), the typed-reply door, PrivateOnly pages and the settings rights
provides:
  - staff_forward_blocks and staff_forward_block_settings (migration 20261113130000), models and the cached repository with a skipLocal key
  - applyForwardBlockAdd, applyForwardBlockRemove, applyForwardBlockRemoveByID, applyForwardBlockAction and the shared result texts
  - the commands /addfwdblock, /rmfwdblock, /fwdblocklist and /fwdblockaction (writes are the Staff Group creator's)
  - the group -9 watcher that deletes a forward from a listed source in every linked group and applies the list's one action once per member per 60 s
  - the private-only Blocked sources settings page (page v) and the tenth settings right settingsRightStaffOwner
affects: [staff, settings-menu, handler-groups, locales, docs]

plan_head_before: e713ed5b54ff1a2f317794f08c8b01a882c9fb0f
plan_head_after: b9070520711b96db1c1688807126bd5a2f52c9aa

actuals:
  tokens: 86000
  tasks: 3
  commits: 5

tech-stack:
  added: []
  patterns:
    - "a list-engine table with an additive Hidden hook that also guards every action of the table"
    - "one apply function per write shared by the commands and the settings page, with the Staff Group creator as the only writer"
    - "a watcher that never ends an update, with a live recheckLink authority step memoised on success only and an in-process punish latch"

key-files:
  created:
    - migrations/20261113130000_add_staff_forward_blocks.sql
    - fuku/db/models/staff_forward_block.go
    - fuku/db/staff/forwardblock.go
    - fuku/modules/staff_forwardblock.go
    - fuku/modules/staff_forwardblock_watch.go
    - fuku/modules/forwardblock_notice.go
    - fuku/modules/settings_fwdblock.go
  modified:
    - fuku/db/staff/repository.go
    - fuku/db/staff/rekey.go
    - fuku/db/cache/local.go
    - fuku/modules/staff.go
    - fuku/modules/settings_list.go
    - fuku/modules/settings_list_input.go
    - fuku/modules/settings_menu.go
    - Makefile
    - AGENTS.md
    - docs/src/content/docs/architecture/handler-groups.md
    - docs/src/content/docs/commands/staff/index.md
    - locales/en.yml, es.yml, fr.yml, hi.yml, id.yml, pt.yml, ru.yml, uk.yml

key-decisions:
  - "Writes (add, remove, set action) are the Staff Group's live creator only, by command and by page alike; other live admins of the Staff Group can only view (plan-level deviation from 'creator or administrator')."
  - "A tenth settings right, settingsRightStaffOwner, carries the creator rule for the page, because settingsRightOwner's alert text is about the Access switch."
  - "Enforcement is guarded by a live recheckLink on a fresh link before any delete or punishment, memoised 60 s on success only, so a stale link never enforces."
  - "The watcher is a new group -9, never ends an update, and acts only in the group where the forward was posted (no cross-group fan-out, so no pacer or partial-failure report)."
  - "The list's one action reuses the blacklist vocabulary (none means delete only, warn, mute, kick, ban); blacklistWatcher is not touched."

patterns-established:
  - "settingsListDef.Hidden hides a list page for a chat and refuses every action code of the table there"
  - "an @username is resolved by one live getChat through a seam and stored as its numeric ID, never re-resolved"

requirements-completed: [QUICK-261113-tjj]

duration: not measured (three sequential executor runs)
completed: 2026-11-13
status: complete
---

# Quick task 261113-tjj: Staff Group forward source blocklist Summary

**The Staff Group's creator lists channels or groups by numeric ID or @username, from three commands or a private-only Blocked sources page, and the bot deletes a forward from any of them in every linked group and applies the list's one action (delete only, warn, mute, kick or ban) to the member who forwarded it, once per album.**

## Performance

- **Duration:** not measured (three sequential executor runs, one per task)
- **Completed:** 2026-11-13
- **Tasks:** 3 of 3
- **Files modified:** 50 files, 5,497 insertions and 48 deletions across the five code commits (nothing under `.planning/`)

## Accomplishments

- Storage and commands (Task 1): two tables with inline named constraints (UNIQUE `(staff_chat_id, source_chat_id)`, CHECK source negative, CHECK action in five values, soft cap 200 inside the insert transaction), a cached `staff.GetForwardBlockRules` under a `skipLocal` key dropped on every write, `RekeyChat` re-keying both tables, `DeleteStaffGroupWithLinks` deleting them, apply functions with a scripted live resolver (point A), and `/addfwdblock`, `/rmfwdblock`, `/fwdblocklist`, `/fwdblockaction` through `helpers.WrapCommand`.
- Enforcement (Task 2): the group -9 watcher, with a cheap-first pipeline (forward filter, chat type, pure source, cached link gate, cached rules, sender shape, `memberDeletionExempt`, cached bot rights, live `recheckLink` memo), the 60 s punish latch, five hourly failure gates and the `bot:punish_auto` notices. Every unknown (list, link, owner check, approvals, admin status) acts on nothing.
- Settings page (Task 3): page `v` on the list engine, private-only and hidden outside a Staff Group, with typed Add, a two-tap delete, paging and a list-wide action screen drawn even for an empty list, all through the same apply functions; the tenth right, the list engine's `Hidden` hook (which also guards every action of the table) and the `(v,s)` part.
- Locale keys in all 8 locale files for every task, `staff_help_msg` extended, AGENTS.md and `handler-groups.md` updated in the same commits.

## Task Commits

1. **Task 1: storage, apply functions and the four Staff Group commands** - `9ef2cfc` (feat)
2. **Task 2: the group -9 watcher, exemptions, live authority and punishments** - `ba2f490` (feat)
3. **Task 3: the private-only Blocked sources page, the Staff Group creator right** - `b907052` (feat)

Two small fixes found by the CI stand-ins between the tasks:

- `95445fd` fix(test): remove the unused `fwdLastText` helper (staticcheck `unused`; Task 1's test file)
- `17e5ce0` fix(staff): mark the group-list placeholder constant as not a credential (gosec G101; Task 2)

Five commits in all (`git rev-list --count e713ed5..HEAD`). The docs commit of this summary and the notes files is left to the orchestrator.

## Files Created/Modified

See the frontmatter for the key files. Tests: `fuku/db/staff/forwardblock_test.go`, the migration test `fuku/db/migrations/staff_forward_block_migration_test.go`, `fuku/modules/staff_forwardblock_{apply,command,watch,env}_test.go`, `settings_fwdblock_test.go` and `settings_fwdblock_screens_test.go`, plus expected-set edits to existing tests (below).

## Decisions Made

The plan's Resolved design points A to H stand. Beyond them: the watcher's notice lives in `forwardblock_notice.go` (the never-delete guard scans `staff*.go`), the punish latch is claimed only after the delete step and only when a punishment is due, and the page's action screen is drawn for an empty list because the action is list-wide.

## Deviations from Plan

### Plan-level (the orchestrator relayed these)

1. Writes are the Staff Group creator's only, not "creator or administrator" (point B): the list changes what the bot does in every linked group, and the creator is the one person the code proves is the creator of every linked group.
2. A new tenth settings right `settingsRightStaffOwner` carries that rule for the page, with the command's own alert `staff_fwdblock_owner_only`.
3. The Staff Group's live creator is trusted as the editor on the strength of the link invariant, and a live `recheckLink` guards the enforcement.

### Task 1 (9ef2cfc)

- `fuku/db/migrations/blacklist_action_check_test.go` (not in the plan's file list): `TestBlacklistActionMigrationStatements` failed whenever a migration newer than the blacklist one existed, so the mandated file name 20261113130000 tripped it; its "nothing sorts after it" loop became "sorts after the previous newest migration", statement assertions untouched.
- The write commands also run `RequireUserAdmin` before `requireStaffGroupCreator`, so a plain member is refused cheaply with the standard text; writes stay creator-only.
- `/rmfwdblock` takes the source chat ID the list shows (the page's delete uses the row ID through `applyForwardBlockRemoveByID`).
- Repository functions are context-first with `db.DB.WithContext`; models use `int64` IDs.
- The `staff_help_msg` section is four command lines plus one limit sentence (about 480 characters in English), because the docs generator renders one bullet per line; `TestStaffPanelRenderFitsInEveryLocale` passes.

### Task 2 (ba2f490 and 17e5ce0)

- `forwardBlockNotice` is in `forwardblock_notice.go`, not the watcher file, because `TestMemberDeletionExcludedMessages` fails any `staff*.go` file naming `scheduleBotMessage`; the guard was not loosened.
- The punish latch is claimed after the delete step only for an action that needs a punishment while the cached restrict right holds; a full latch table refuses the punishment, a full memo table asks again.
- The notice splices the `blacklists_bl_watcher_*` templates with `fmt.Sprintf` like the blacklist watcher (the Hindi template takes its arguments in the reverse order, an existing locale quirk, not edited).
- Test check helpers take the subtest's own `t`.
- No existing test was edited or re-recorded.

### Task 3 (b907052)

- `settingsListGuardHidden` (not in the plan): page actions do not consult `Hidden`, so the list engine wraps every action of a table that has the hook; the forged-press cases fail without it (checked by mutation).
- More existing tests needed expected-set edits than the three the plan named, all of one kind (a hidden page is not in a non-Staff-Group chat's Main): `TestSettingsMainGrid`, `TestSettingsNavigation`, `TestSettingsTracer`, `TestSettingsPhase18Codes`, `TestSettingsPhase18InStaffGroup`, `TestSettingsMainPhase19`, `TestSettingsPhase19Codes`, `TestSettingsRightBotHalves`, `TestSettingsRightRefusalForParity`; no assertion was loosened and no golden touched.
- No `docs/` file changed in Task 3 (`make generate-docs` regenerates nothing for it).
- `settings_blacklist.go` is not touched (its pre-existing G115 `int64(row.ID)` would be flagged for the PR); the page has its own 12-line choice-row builder and its models use `int64` IDs.
- `TestSettingsStaffPrivacy` is not edited (it renders no Staff Group's own menu for list pages); a sentinel privacy check in the group-menu subtest and the e2e staff group subtest hold the same fact.
- Entry numbering on the page follows the engine's text sort while `/fwdblocklist` orders by row ID.

**Impact on plan:** every deviation was needed for correctness or to keep a CI guard honest; no scope creep.

## Verification

All with the CI environment (`cienv.sh`, `FUKU_TEST_DATABASE` unset: SQLite and miniredis) unless noted.

- Task 1: `gofmt` on changed files clean, `CGO_ENABLED=0 go build ./...`, `go vet -tags testtools` on the touched packages, `go test -tags testtools -race ./fuku/db/staff ./fuku/db/cache ./fuku/db/migrations` ok, `-run '^(TestApplyForwardBlock|TestForwardBlock|TestStaff|TestLoadStaff|TestModulesRecordTheirCommands|TestNoRawNewCommand)' ./fuku/modules` ok (151 s), `./fuku/i18n ./scripts/generate_docs/...` ok, `make check-translations`, `make check-docs`; on a local PostgreSQL 16 (`FUKU_TEST_DATABASE=true`, migration chain through `scripts/migrate_psql.sh`) `./fuku/db/staff ./fuku/db/migrations ./fuku/db/cache` ok, including `TestStaffForwardBlockConstraints` and `TestStaffForwardBlockMigrationDefaults` (really run, not skipped).
- Task 2: the plan's run-list (`TestForwardBlock|TestApplyForwardBlock|TestStaff|TestLoadStaff|TestMemberDeletion|TestMemberEdited|TestMemberSelfDestruct|TestBlacklistWatcher|TestSettingsPhase18InStaffGroup|TestModulesRecordTheirCommands|TestNoRawNewCommand|TestCommandRepliesStayInsideTheHandler`) ok, 307 top-level tests in 205 s; 19 new top-level tests ok in 8 s and bare; wider handler-group families `TestMemberReq*` (150 tests) and the member deletion, blacklist, antiflood, lock, raid, lockdown, command deletion and dispatcher families (193 tests) PASS; a mutation check (removing `memberDeletionExempt`) made the Spares and UnknownNeverActs tests fail.
- Task 3: the whole `TestSettings` and `TestLoadSettings` families in the race-built `fuku/modules` binary in three processes (`^TestSettings[A-L]` 317 names, `^TestSettings[M-R]` 77, `^(TestSettings[S-Z]|TestLoadSettings)` 26, together the 420 names `go test -list` gives for `./fuku/modules` and `./fuku/i18n`), all exit 0 after the final edits; `./fuku/i18n` settings tests, `^(TestForwardBlock|TestApplyForwardBlock)`, `./scripts/generate_docs/...` ok; the new tests bare (`env -i`) ok; two mutation checks (the page's right, the `Hidden` guard) made the authority and opens tests fail. `make check-translations`, `make generate-docs`, `make check-docs` clean.
- Lint stand-in (`lint-local-fixed.py e713ed5`): no dupl, gocyclo or godox findings. gosec 2.29: the 18 pre-existing findings, none in a file the tasks added or changed. `make lint` could not run (golangci-lint is built with Go 1.25 here); `make test` and `make test-postgres-integrity` are left to CI.

## Accepted limits (stated, not hidden)

- An album is one punishment per member per 60 s through an in-process latch, so two replicas can each punish once; the authority memo is per replica too.
- The 200-source cap is a soft cap read inside the insert transaction, so two concurrent adds can pass by one.
- A stored source title can go stale after a rename.
- A blocked forward the blacklist watcher also matches can be acted on by both, because the new watcher never ends an update.
- A channel identity (`sender_chat`) and an anonymous admin are exempt.
- A forward from a person is a user or hidden-user origin and can never match a group ID; only channels and groups whose anonymous admin posted are sources, which AGENTS.md and the staff help say.
- Source entry is one source per command or typed message.
- Page entry numbering follows the engine's text sort, `/fwdblocklist` the row ID order.

## Issues Encountered

- `TestBlacklistActionMigrationStatements` tripped on the new migration's file name (Task 1, fixed as above).
- The never-delete guard scans `staff*.go` for `scheduleBotMessage` (Task 2, the notice moved to its own file).
- CI stand-ins caught an unused test helper and a G101 placeholder constant (the two fix commits).
- A one-process run of the wide Task 2 families hit the 30 minute background limit with no output; re-run split with a 90 minute test timeout.
- Task 3's first full settings run found four expected-set failures, fixed and re-run clean.

## Threat Flags

None beyond the plan's threat model (T-tjj-01 to T-tjj-10 are each mitigated and tested as listed in the plan); the page adds no network endpoint.

## Self-Check: PASSED

- Commits `9ef2cfc`, `ba2f490`, `b907052`, `95445fd` and `17e5ce0` are ancestors of HEAD (`git rev-list --count e713ed5..HEAD` is 5); nothing under `.planning/` is in any of them; nothing was pushed.
- The created files listed above exist in the tree (`settings_fwdblock.go`, `staff_forwardblock.go`, `staff_forwardblock_watch.go`, `forwardblock_notice.go`, `fuku/db/staff/forwardblock.go`, the migration).
