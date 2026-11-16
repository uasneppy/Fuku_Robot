# 261113-tjj Task 1 notes (uncommitted progress note)

Commit: 9ef2cfc `feat(staff): forward source blocklist storage and Staff Group commands` on claude/ecstatic-goldberg-qsss05. Not pushed. Nothing under .planning/ is in it.

## Built

- Migration `migrations/20261113130000_add_staff_forward_blocks.sql` (two CREATE TABLE IF NOT EXISTS, inline named constraints). Verified through `scripts/migrate_psql.sh migrate` on a local PostgreSQL 16 (I started cluster 16/main and redis-server in the sandbox; both are still running).
- Models `fuku/db/models/staff_forward_block.go`: `StaffForwardBlock`, `StaffForwardBlockSettings`.
- Repository `fuku/db/staff/forwardblock.go`; RekeyChat re-keys both tables (`rekeyForwardBlocksTx`), DeleteStaffGroupWithLinks deletes them (`deleteForwardBlocksTx`), both drop the key.
- `fuku/db/cache/local.go`: `staff_forward_block:` prefix in skipLocal (+ test rows).
- Modules `fuku/modules/staff_forwardblock.go`: checks, seams, apply functions, texts, four commands, registered in `LoadStaff`.
- 17 `staff_fwdblock_*` locale keys x 8 files; `staff_help_msg` gains a "Forward blocklist" section in all 8; `make generate-docs` regenerated docs/.../commands/staff/index.md.
- AGENTS.md: Data bullet, RekeyChat bullet, Staff links bullet (unset), Subsystem traps commands bullet, PostgreSQL integrity list, harness AutoMigrate sentence.
- Makefile: `TestStaffForwardBlockConstraints|TestStaffForwardBlockMigrationDefaults` added to test-postgres-integrity `-run`.

## Names and signatures Task 2 and Task 3 will call

Repository (`fuku/db/staff`, all context-first):
- `type ForwardBlockRules struct{ Action string; Sources []int64 }`, method `IsBlocked(sourceChatID int64) bool`
- `GetForwardBlockRules(ctx, staffChatID) (ForwardBlockRules, error)` (cached, skipLocal key `cache.CacheKey("staff_forward_block", id)`), `GetForwardBlockRulesFresh`
- `ListForwardBlocksFresh(ctx, staffChatID) ([]models.StaffForwardBlock, error)` (ordered by id), `GetForwardBlockFresh(ctx, staffChatID, id) (*models.StaffForwardBlock, error)` (nil for missing or foreign), `CountForwardBlocksFresh(ctx, staffChatID) (int64, error)`
- `AddForwardBlock(ctx, staffChatID, sourceChatID int64, title string, userID int64) (added bool, err error)`, `RemoveForwardBlockByID(ctx, staffChatID, id) (*models.StaffForwardBlock, error)`, `RemoveForwardBlockBySource(ctx, staffChatID, sourceChatID) (bool, error)`, `SetForwardBlockAction(ctx, staffChatID, userID int64, action string) error`
- Consts/vars: `ForwardBlockMax` (200), `ForwardBlockActionNone`, `ForwardBlockActions`, `ErrForwardBlockFull`, `ErrForwardBlockAction`, existing `ErrStaffGroupMissing`.

Modules (`fuku/modules/staff_forwardblock.go`):
- `applyForwardBlockAdd(ctx, b *gotgbot.Bot, staffChatID, userID int64, rawRef string) forwardBlockAddResult` (`Outcome forwardBlockAddOutcome` with zero value `forwardBlockAdded`; others `forwardBlockAlready`, `forwardBlockInvalidRef`, `forwardBlockNotFound`, `forwardBlockNotSource`, `forwardBlockCheckFailed`, `forwardBlockFull`, `forwardBlockNoStaffGroup`, `forwardBlockSaveFailed`; fields `SourceChatID`, `Title`)
- `applyForwardBlockRemove(ctx, staffChatID, sourceChatID) forwardBlockRemoveResult` and `applyForwardBlockRemoveByID(ctx, staffChatID, id) forwardBlockRemoveResult` (`Outcome` zero value `forwardBlockRemoved`, then `forwardBlockGone`, `forwardBlockRemoveFailed`; fields `SourceChatID`, `Title`)
- `applyForwardBlockAction(ctx, staffChatID, userID int64, action string) forwardBlockActionResult` (`Outcome` zero value `forwardBlockActionApplied`, `forwardBlockActionUnknown`, `forwardBlockActionNoStaffGroup`, `forwardBlockActionSaveFailed`; field `Action`)
- Texts: `forwardBlockAddText(tr, result)`, `forwardBlockRemoveText(tr, result)`, `forwardBlockActionText(tr, result)`, `forwardBlockEntryText(sourceChatID, title)` (plain text: ID then title; splice with `settingsListEntryText(tr, key, "source", text)`), `forwardBlockListText`.
- Seams: `forwardBlockWrites` (`forwardBlockWriteSeams{Add, RemoveByID, RemoveBySource, SetAction}`), `forwardBlockResolve func(ctx, b, chat any) (forwardBlockChat, error)`, `parseForwardBlockRef`. Constants `forwardBlockListMax` (40), `forwardBlockTitleMaxRunes` (64).
- Checks: `requireStaffGroup()`, `requireStaffGroupCreator()` (helpers.CheckFunc), `forwardBlockChecks(creator bool)`.
- The page's "one-source-at-a-time" refusal and Add prompt keys are Task 3's. Reused locale keys: `staff_unset_not_staff`, `staff_check_failed`, `common_settings_save_failed`, `settings_check_failed`, `settings_opt_*`, `settings_bl_opt_none`.
- Locale keys added: staff_fwdblock_owner_only, _usage_add, _usage_rm, _added, _already, _not_found, _not_source, _full ({max}), _check_failed, _removed, _not_listed, _list_header ({action}), _list_empty, _list_more ({n}), _action_current ({action}), _action_changed ({action}), _action_usage. `staff_fwdblock_reason` is Task 2's.

Test toolkit (fuku/modules, test files):
- `fwdTestStaffGroup(t)` (miniredis + a Staff Group row + staffCleanup), `fwdResolver{answers, errs, calls}.install(t)` (swaps `forwardBlockResolve`), `fwdRowCount(t, staffChat)`, `fwdWantReply(t, env, chat, from, text, markerKey)`, `fwdLastText`.
- Command tests use `newStaffActionEnv(t, 0)` (owner = live creator of `env.staffChat`; `env.fake.listAdministrator(chat, userID)` adds a non-creator admin; `env.fake.script("getChatAdministrators", chat, staffFake429(1))` fails the next live owner check, warm the admin cache first).
- `staffCleanup` now deletes both tables' rows and the `staff_forward_block` cache keys of its chats. Both models are in `fuku/modules/test_harness_test.go` and `fuku/db/staff/testmain_test.go` AutoMigrate lists.

## Commands run (results)

- `gofmt -l` on changed .go files: empty. `CGO_ENABLED=0 go build ./...`: ok. `go vet -tags testtools ./fuku/db/staff ./fuku/db/models ./fuku/db/cache ./fuku/db/migrations ./fuku/modules`: ok.
- CI env (cienv.sh, SQLite since FUKU_TEST_DATABASE unset): `go test -tags testtools -race -count=1 ./fuku/db/staff ./fuku/db/cache ./fuku/db/migrations`: ok. `-run '^(TestApplyForwardBlock|TestForwardBlock|TestStaff|TestLoadStaff|TestModulesRecordTheirCommands|TestNoRawNewCommand)' ./fuku/modules`: ok (151 s). `./fuku/i18n ./scripts/generate_docs/...`: ok.
- Bare (no env): `-run '^(TestApplyForwardBlock|TestForwardBlock|TestParseForwardBlockRef)' ./fuku/modules`: ok.
- PostgreSQL (local cluster, migration chain applied by scripts/migrate_psql.sh, `FUKU_TEST_DATABASE=true`): `./fuku/db/staff ./fuku/db/migrations ./fuku/db/cache`: ok, including `TestStaffForwardBlockConstraints` and `TestStaffForwardBlockMigrationDefaults` (the latter really ran, not skipped; on SQLite it skips by design).
- `make check-translations`: all present. `make generate-docs` then `make check-docs`: no drift.
- `make lint` not run (golangci-lint is Go 1.25 here). New functions are small (largest switch about 10).

## Deviations from the plan

1. `fuku/db/migrations/blacklist_action_check_test.go` (not in the plan's file list): `TestBlacklistActionMigrationStatements` failed any time a migration newer than the blacklist one existed ("no migration sorts after it"). The plan's mandated filename 20261113130000 trips it. Smallest fix: replace that "nothing sorts after" loop with "sorts after the previous newest migration (scheduledDeletionMigration)", the same pattern the other migration tests use. The statement-content assertions are untouched.
2. Command checks include `RequireUserAdmin` on writes too (before `requireStaffGroupCreator`), so a plain member is refused cheaply with the standard admin-only text and costs no live owner lookup. Writes remain creator-only.
3. `rmfwdblock` takes the source chat ID the list shows (the apply function is `applyForwardBlockRemove` by source); the page's delete (Task 3) uses `applyForwardBlockRemoveByID`.
4. The repository functions are context-first with `db.DB.WithContext`; models use `int64` IDs (not uint like StaffGroup) to match `RETURNING` scanning and the row-ID callback rules.
5. staff help section is four short command lines plus one limit sentence (about 480 characters in English), because the docs generator renders one bullet per `×` line. `TestStaffPanelRenderFitsInEveryLocale` passes (it accepts the hint fallback when a locale's help does not fit).

## Notes for the next executor

- PostgreSQL 16 and redis-server were started by hand in this sandbox (pg_ctlcluster 16 main start; redis-server --daemonize yes; DB `fuku_test` migrated). They may be gone in a new session.
- `staff_forwardblock.go` is 542 lines; nothing in it needs Task 2 edits except the watcher file being new.
