---
phase: 261113-tjj-staff-group-forward-source-blocklist-blo
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - migrations/20261113130000_add_staff_forward_blocks.sql
  - fuku/db/models/staff_forward_block.go
  - fuku/db/staff/forwardblock.go
  - fuku/db/staff/forwardblock_test.go
  - fuku/db/staff/repository.go
  - fuku/db/staff/repository_test.go
  - fuku/db/staff/rekey.go
  - fuku/db/staff/rekey_test.go
  - fuku/db/staff/testmain_test.go
  - fuku/db/cache/local.go
  - fuku/db/cache/local_test.go
  - fuku/db/migrations/staff_forward_block_migration_test.go
  - Makefile
  - fuku/modules/staff.go
  - fuku/modules/staff_forwardblock.go
  - fuku/modules/staff_forwardblock_watch.go
  - fuku/modules/staff_forwardblock_apply_test.go
  - fuku/modules/staff_forwardblock_command_test.go
  - fuku/modules/staff_forwardblock_watch_test.go
  - fuku/modules/staff_forwardblock_env_test.go
  - fuku/modules/staff_helpers_test.go
  - fuku/modules/test_harness_test.go
  - fuku/modules/settings_fwdblock.go
  - fuku/modules/settings_fwdblock_test.go
  - fuku/modules/settings_fwdblock_screens_test.go
  - fuku/modules/settings_list.go
  - fuku/modules/settings_list_input.go
  - fuku/modules/settings_menu.go
  - fuku/modules/settings_phase19_test.go
  - fuku/modules/settings_list_framework_test.go
  - fuku/modules/settings_edit_hooks_test.go
  - fuku/modules/settings_list_locale_test.go
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
  - docs/src/content/docs/architecture/handler-groups.md
  - docs/src/content/docs/commands/staff/index.md
autonomous: true
requirements: [QUICK-261113-tjj]

estimate:
  tokens: 150000
  raw_tokens: 150000
  tasks: 3
  confidence: low

must_haves:
  truths:
    - "A message that a member forwards from a listed source (a channel, or a group whose anonymous admin posted it) into a group linked to the Staff Group is deleted, and the member gets the Staff Group's one action (delete only, warn, mute, kick or ban). The same forward in an unlinked group, in the Staff Group itself, from an unlisted source, or from a user or hidden user is left alone."
    - "Creators and administrators, approved users, anonymous admins, every sender_chat sender, automatic forwards, bots, Telegram service accounts and the bot itself are never deleted or punished, and any state the watcher cannot read (the list, the link, the owner check, approvals, admin status) never deletes or punishes."
    - "Only the Staff Group's live creator can add a source, remove one or set the action, by command and by the settings page alike (the creator is, by the link invariant, the creator of every linked group). Other live admins of the Staff Group can only view. A refusal writes nothing."
    - "An @username source is resolved by a live getChat and stored as its numeric chat ID; an unresolvable name or a target that is not a channel or group is refused; a negative numeric ID is stored as typed, with no title when Telegram cannot resolve it."
    - "The page and the commands call the same apply functions, so a source added by one is the row the other lists, and every write drops cache key staff_forward_block:<staff chat> (listed in skipLocal) on every replica."
    - "A Staff Group chat migration moves the list and its action with the Staff Group; unsetting a Staff Group deletes them; an unlinked group stops being enforced at the next forward."
    - "A bot without the delete or restrict right in a linked group logs one line per kind per hour per chat and is never retried; an album of blocked forwards deletes every item and punishes the member once."
  artifacts:
    - path: "migrations/20261113130000_add_staff_forward_blocks.sql"
      provides: "staff_forward_blocks and staff_forward_block_settings with inline named constraints"
    - path: "fuku/db/staff/forwardblock.go"
      provides: "cached rules read, fresh reads, add, remove, set action, key invalidation"
    - path: "fuku/modules/staff_forwardblock.go"
      provides: "apply functions, live resolver, shared result texts, the four commands"
    - path: "fuku/modules/staff_forwardblock_watch.go"
      provides: "the group -9 watcher with exemptions, authority memo, punish latch and failure gates"
    - path: "fuku/modules/settings_fwdblock.go"
      provides: "the private-only Blocked sources page (list engine table, typed add, action screen)"
  key_links:
    - from: "fuku/modules/staff_forwardblock_watch.go"
      to: "fuku/db/staff/forwardblock.go"
      via: "staff.GetLinkOfGroup gate, then staff.GetForwardBlockRules under a skipLocal key"
    - from: "fuku/modules/staff_forwardblock_watch.go"
      to: "fuku/modules/staff_recheck.go"
      via: "recheckLink on a fresh link before any delete or punishment (memoised 60 s on success only)"
    - from: "fuku/db/staff/rekey.go"
      to: "staff_forward_blocks"
      via: "RekeyChat re-keys both tables and DeleteStaffGroupWithLinks deletes them in the same transaction"
    - from: "fuku/modules/settings_fwdblock.go"
      to: "fuku/modules/staff_forwardblock.go"
      via: "the page's Add, delete and action writes call applyForwardBlockAdd, applyForwardBlockRemoveByID and applyForwardBlockAction"
---

<objective>
Add a Staff Group forward-source blocklist: the Staff Group's creator lists channels or groups by numeric ID or @username, and the bot deletes a message forwarded from any of them in every group linked to that Staff Group and applies the list's one action to the member who forwarded it (D-01, D-02, D-03).

Purpose: the owner's staff protect every linked community from one place without per-group setup, and the bot still never lets anyone change moderation in a group where they are not the group's admin.
Output: migration and repository, apply functions and four commands, the group -9 watcher, a private-only settings page, locale keys in all 8 files, AGENTS.md and generated docs.

Decomposition note: tracer-first is satisfied by three vertical tasks instead of a throwaway slice, because the owner capped the quick task at three tasks and suggested this split. Each task ends with something an operator can do (manage by command, enforcement, manage by page) and leaves the tree green and committed. The enforcement path is proven end to end by the first test of task 2 through the real dispatcher.

Owner decisions, locked: D-01 scope is the Staff Group (one list for all linked groups, no per-group list). D-02 one action for the whole list from the blacklist vocabulary (none meaning delete only, warn, mute, kick, ban), no per-source action, no timed durations. D-03 a private-only list page on the list engine plus matching commands through helpers.WrapCommand, sharing one apply function per write. D-04 tracked as a quick task. No deferred ideas were given.

Plan-level deviations the orchestrator must relay (full reasoning in Resolved design points): (1) writes are the Staff Group creator only, not "creator or administrator" (point B); (2) a new tenth settings right is appended for it, because settingsRightOwner's alert text is about the Access switch; (3) the Staff Group's live creator is trusted as the editor on the strength of the link invariant, and a live recheckLink guards the enforcement.
</objective>

<execution_context>
@/home/user/Fuku_Robot/.claude/gsd-core/workflows/execute-plan.md
@/home/user/Fuku_Robot/.claude/gsd-core/templates/summary.md
</execution_context>

<context>
@/home/user/Fuku_Robot/.planning/STATE.md
@/home/user/Fuku_Robot/AGENTS.md
@/home/user/Fuku_Robot/.claude/CLAUDE.md

## Live observations at planning time (the only scope authority)

- Branch is claude/ecstatic-goldberg-qsss05 with a clean tree. Newest migration is migrations/20261113120000_drop_legacy_blacklist_action_check.sql, so the new one is 20261113130000.
- gofmt -l fuku scripts already lists 16 unformatted files at HEAD (none of them is modified by this plan), so formatting is gated on the changed files only. make check-translations and make check-docs both pass at HEAD. golangci-lint is on PATH but built with Go 1.25 and cannot run here.
- staff.GetLinkOfGroup and staff.GetStaffGroup are cached gates whose keys (staff_link_of:, staff_group:) are already in skipLocal, so they are never stale across replicas; GetLinkOfGroupFresh and GetStaffGroupFresh are the authority reads. Links exist only while one person is, live, the creator of both the linked group and the Staff Group (recheckLink, D-12 of the staff code), and /setstaff requires the bot to be an administrator.
- All handler groups 0..12 and -10, -8..-1 are taken; -9 is the only free group below 12. Group 12 must stay last.
- gotgbot gives a forward's source as msg.ForwardOrigin.MergeMessageOrigin(): Chat is set for origin type channel, SenderChat for origin type chat (a message an anonymous admin of a group posted), SenderUser for user, and only SenderUserName for hidden_user. A plain member's message forwarded from a group therefore carries a user or hidden_user origin and can never match a group ID. Channels are the real use. This is stated in AGENTS.md and the docs.
- Reusable, so not rebuilt: raidFloodSenderCounts (sender shape), memberDeletionExempt (tri-state approvals plus admin status, unknown is exempt, warms the admin list once per chat), recheckLink and newStaffOwnerPass, parseStaffChatIDArg and staffUsernameRe, isTelegramClientError, memberDeleteWarnGate (hourly per-chat log gate), scheduleBotMessage with deletion.KindBotAuto, warnsModule.warnThisUser with warnSourceAuto, kickMember, MutedPermissions, blacklistActionValues, settingsBlacklistActionWord, the locale keys blacklists_bl_watcher_muted_user, _banned_user, _kicked_user ("Muted %s due to %s"), settings_bl_action_line, settings_bl_action_intro, settings_btn_bl_action and the settings_opt_* words.
- blacklistWatcher's punishment switch is inline and is not edited: it has no characterization golden for its requests, so extracting it would risk its behaviour for no gain. The new watcher composes the same primitives and reuses the vocabulary, texts and scheduling; a test pins that the two action vocabularies are equal. No existing command is extracted, so no new TestLegacy golden is needed and none is re-recorded.

## Resolved design points (each is testable, each is cited by the tasks)

A. @username resolution. The ref is parsed strictly first (parseStaffChatIDArg for a negative numeric ID, staffUsernameRe for @name; anything else is refused with no Telegram call). An @username is resolved by one live getChat through the seam forwardBlockResolve (a raw getChat request, because gotgbot's GetChat takes only an int64), bounded by 8 s. A resolved chat must be of type channel, supergroup or group with a negative ID; a private type or a positive ID is refused as not a source. A Telegram 4xx other than 429 for a name is "not found" (refused); a 429, 5xx, transport error or timeout is "check failed" (refused, nothing stored). The stored value is always the numeric chat ID, never re-resolved. A negative numeric ID is stored as typed: a resolvable one gets its title and type-checked, a definite 4xx (the bot cannot see a private channel) is stored unverified with an empty title, and an unknown error refuses. The title is stored (cut to 64 code points) because the list is shown only to the Staff Group's own admins (private-only page, view-only commands in the Staff Group); no notice, log line or group-menu screen ever names a source or the Staff Group.

B. Who may edit. Writes (add, remove, set action) need the Staff Group's live creator: commands use chat_status.CheckOwner on the Staff Group (live getChatAdministrators, never the admin cache; unknown refuses with staff_check_failed); the page uses a new right judged on the press's own live member record. Reading (the list command, the action status, the page's view and pager) needs only a creator or administrator of the Staff Group (RequireUserAdmin for the commands; the page's existing live admin check plus the owner gate). Why not "creator or administrator": the list changes what the bot does to members in every linked group, an administrator of the Staff Group is not known to be an administrator of each linked group, and the core rule forbids anyone acting in a group where they are not an admin. The creator is the one person the code proves is the creator of every linked group, because a link is deleted when its maker stops being the live creator of either side. A per-group check of the editor in every linked group would need a fan-out with pacing and partial-failure reporting and would still go stale after the edit. The recommendation to check recheckLink is kept where it matters: enforcement. The enforcement is the bot acting in the group where the forward happened under its own rights, and before any write the watcher reads the link fresh and runs recheckLink (memoised for 60 s on success only), so a link whose maker no longer owns both sides never enforces (T-tjj-02). Relaxing B later is one right swap, which is why it is a plan-level deviation and not buried.

C. Enforcement scope. The watcher acts only in the group where the forward was posted, because the list is read through that group's link. No cross-group fan-out, so no staff pacer, no partial-failure report and no Telegram fan-out cost.

D. Exemptions. Decided and tested: the sender shape through raidFloodSenderCounts (no sender_chat, so anonymous admins, channel identities and the group itself are out; no automatic forward; no Telegram service account; not the bot), plus msg.IsAutomaticForward and the bot flag; then memberDeletionExempt (approved, creator or administrator, and either lookup being unreadable all exempt; an unknown status never punishes). Staff Group chat itself: it has no link, so the link gate returns nil; a test proves a blocked forward there is untouched. A sender_chat channel identity posting a blocked forward is left alone, a stated limit.

E. Placement. A new group -9, a message handler registered from LoadStaff with the cheap filter msg.ForwardOrigin != nil, the only handler of the group. The result is a named value preset to ext.ContinueGroups with defer error_handling.RecoverFromPanic, so nothing ends an update (later groups still see the message: the raid flood counter and users tracker need it) and no panic can return nil. Order, cheapest first: (1) filter, a nil check; (2) chat type group or supergroup; (3) forwardBlockSourceOf, pure, a negative ID of type channel, supergroup or group from the origin (user and hidden_user stop here); (4) staff.GetLinkOfGroup, the first I/O, one Redis GET because the key is skipLocal, nil for an unlinked group; (5) staff.GetForwardBlockRules for the link's Staff Group, one cached read, source not in the list stops; (6) pure sender shape; (7) memberDeletionExempt; (8) bot rights from the cached predicates (IsBotAdmin, CanBotDelete, CanBotRestrict); (9) the live authority step; (10) delete, then punish once per member per 60 s. Cost for a group not linked to a Staff Group: a non-forward costs one nil check and no I/O, a forward from a user or hidden user costs a pure switch, a forward from a channel or group costs one Redis GET and returns. A linked group adds one more cached read and only a listed source reaches steps 6 to 10. The watcher starts no goroutine, so there is no shutdown drain and no new Redis key.

F. Rights and failure. If the bot is not an administrator the watcher does nothing and logs one line (kind cannot-act) per chat per hour through its own memberDeleteWarnGate array; with the delete right missing it still punishes, with the restrict right missing it still deletes, each logging its own gated line; a refused delete or restrict call is logged through its gate by chat and user ID and error class only. Nothing is retried, and the cached bot-right predicates stop calls to a chat the bot cannot act in, so there is no loop and no new latch. The Staff Group sees the same fact in its /staff panel through the link's health column, which already reports a bot that cannot restrict. A recheckLink that is unknown (Telegram did not answer) acts on nothing and logs through its gate.

G. Cache. Key cache.CacheKey("staff_forward_block", staffChatID) holding one struct with the action and the source IDs, read through cache.GetFromCacheOrLoad, prefix added to skipLocal in fuku/db/cache/local.go (and its test), because the watcher reads it on every listed forward and a removed source must stop acting on every replica at the next message. Every write (add, remove by ID, remove by source, set action) drops the key after commit. RekeyChat re-keys both tables in its transaction and drops the old and the new key; DeleteStaffGroupWithLinks deletes both tables' rows in its transaction and drops the key; unlinking a group needs nothing because the link gate stops enforcement. The tables are never part of backup, export, import or reset.

H. Documentation. AGENTS.md is edited in the same commit as each change that invalidates a rule: task 1 (Data, RekeyChat and unset, PostgreSQL integrity list), task 2 (the group list gains -9, the watcher bullet, Subsystem traps), task 3 (rights count, part table, Callbacks page bullet, Testing families). docs/src/content/docs/architecture/handler-groups.md gets its -9 row by hand in task 2. make generate-docs runs in tasks 1 and 3 so docs/.../commands/staff/index.md follows staff_help_msg.

## Accepted limits (stated, not hidden)

An album is one punishment per member per 60 s through an in-process latch, so two replicas can each punish once. The list cap of 200 is a soft cap read inside the insert transaction, so two concurrent adds can pass by one. A stored title can go stale after a rename. A blocked forward that the blacklist watcher also matches can be acted on by both, because this watcher never ends an update. A channel identity (sender_chat) and an anonymous admin are exempt. Source entry is one source per command or typed message.
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Storage, apply functions and the four Staff Group commands</name>
  <files>migrations/20261113130000_add_staff_forward_blocks.sql, fuku/db/models/staff_forward_block.go, fuku/db/staff/forwardblock.go, fuku/db/staff/forwardblock_test.go, fuku/db/staff/repository.go, fuku/db/staff/repository_test.go, fuku/db/staff/rekey.go, fuku/db/staff/rekey_test.go, fuku/db/staff/testmain_test.go, fuku/db/cache/local.go, fuku/db/cache/local_test.go, fuku/db/migrations/staff_forward_block_migration_test.go, Makefile, fuku/modules/staff.go, fuku/modules/staff_forwardblock.go, fuku/modules/staff_forwardblock_apply_test.go, fuku/modules/staff_forwardblock_command_test.go, fuku/modules/staff_helpers_test.go, fuku/modules/test_harness_test.go, locales/en.yml, locales/es.yml, locales/fr.yml, locales/hi.yml, locales/id.yml, locales/pt.yml, locales/ru.yml, locales/uk.yml, AGENTS.md, docs/src/content/docs/commands/staff/index.md</files>
  <read_first>
    AGENTS.md (Data, Migrations, Adding a module, Go rules, Testing, Subsystem traps: Staff links and Staff Group chat migration bullets)
    migrations/20261110120000_add_scheduled_deletion.sql
    migrations/20261106120000_add_member_requirements.sql
    fuku/db/staff/repository.go
    fuku/db/staff/rekey.go
    fuku/db/memberreq/settings.go
    fuku/db/filters/repository.go (RemoveFilterByID)
    fuku/db/models/staff.go
    fuku/db/migrations/scheduled_deletion_migration_test.go
    fuku/modules/staff.go
    fuku/modules/staff_link.go (parseStaffChatIDArg)
    fuku/modules/staff_action_parse.go (staffUsernameRe)
    fuku/modules/blacklists.go (blacklistActionValues and the write-seam pattern)
    fuku/modules/staff_helpers_test.go
    fuku/modules/settings_list.go (settingsListEntryText)
    fuku/db/cache/local.go and local_test.go
  </read_first>
  <behavior>
    - Repository: add stores one row per (staff chat, source) and a second add of the same source reports not added; the 201st source is refused with ErrForwardBlockFull; a Staff Group that does not exist is refused with ErrStaffGroupMissing; remove by ID deletes only a row of that staff chat and a foreign or unknown ID removes nothing; the action defaults to none, accepts exactly none, warn, mute, kick, ban, and anything else is refused before any write.
    - Cache: GetForwardBlockRules returns the action and the sources, and after every write (add, remove, set action, RekeyChat, DeleteStaffGroupWithLinks) the next read sees the new state; the key prefix is in skipLocal.
    - Chat migration moves sources and the action to the new ID and leaves another Staff Group's list alone; unsetting a Staff Group deletes its sources and its action row in the same transaction.
    - Constraints: a duplicate (staff chat, source), a non-negative source ID and an action outside the five values are refused by the database itself.
    - Add: @name resolves live to the numeric ID and is stored with its title; a name that is not found, a resolved private chat, a name that is unresolvable because Telegram failed (429, 5xx, timeout) and a malformed ref each store nothing and answer their own result; a negative numeric ID that Telegram answers with a 400 is stored with an empty title; a numeric ID that resolves to a non-chat or whose lookup failed unknown stores nothing; a repeat is already listed; a full list is refused.
    - Commands: in the Staff Group the creator can add, remove, list and set the action; another live administrator can only list and read the action, and a refused add, remove or action stores nothing and sends no Telegram write; an anonymous sender, a chat that is not a Staff Group, a basic non-group and an unknown owner check are each refused with their own existing text; the action word must be one of blacklistActionValues.
    - Vocabulary: staff.ForwardBlockActions equals blacklistActionValues as a set.
  </behavior>
  <action>
Implement D-01, D-02 and D-03 (storage, shared apply functions and the commands). Write the tests first, then the code, and keep every new function under cyclomatic complexity 15 (split steps into small functions) so the CI gocyclo gate at 20 cannot trip; add no TODO or FIXME comment (godox).

1. Migration migrations/20261113130000_add_staff_forward_blocks.sql. A header comment in the style of the member requirements migration, then exactly two CREATE TABLE IF NOT EXISTS statements with constraints inline, no top-level ADD CONSTRAINT, no BEGIN or COMMIT, no CONCURRENTLY, no NOT VALID, no foreign key (RekeyChat updates staff_chat_id and rows are deleted with the Staff Group in the same transaction). staff_forward_blocks has id BIGSERIAL primary key, staff_chat_id and source_chat_id BIGINT NOT NULL, source_title TEXT NOT NULL DEFAULT empty, created_by BIGINT NOT NULL DEFAULT 0, created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(), CONSTRAINT uk_staff_forward_blocks_source UNIQUE (staff_chat_id, source_chat_id) and CONSTRAINT chk_staff_forward_blocks_source CHECK (source_chat_id < 0). staff_forward_block_settings has staff_chat_id BIGINT PRIMARY KEY, action VARCHAR(8) NOT NULL DEFAULT 'none', updated_by BIGINT NOT NULL DEFAULT 0, updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW() and CONSTRAINT chk_staff_forward_block_action CHECK (action IN ('none','warn','mute','kick','ban')). The runner.go and migrate_psql.sh need no change.

2. Models in fuku/db/models/staff_forward_block.go: StaffForwardBlock and StaffForwardBlockSettings with TableName returning the two table names, gorm tags that produce the same UNIQUE and CHECK constraints on SQLite (named as in the migration, in the style of StaffGroupLink's check tags), the settings primary key staff_chat_id with autoIncrement false, and no default: tag on any field a Go zero value must be able to write.

3. Repository fuku/db/staff/forwardblock.go (same package, so RekeyChat and DeleteStaffGroupWithLinks can reach it without an import cycle). Export ForwardBlockMax (200), ForwardBlockActions (none, warn, mute, kick, ban), ErrForwardBlockFull, ErrForwardBlockAction and the cached struct ForwardBlockRules (Action string and Sources []int64, with an IsBlocked(sourceChatID) method). Functions, every one context-first, error-returning, wrapping with fukuerrors and never discarding a database error: GetForwardBlockRules (cache.GetFromCacheOrLoad under cache.CacheKey("staff_forward_block", staffChatID) and cache.DefaultCacheTTL, no row means action none and no sources, a failed load is returned and never replaced by defaults) and GetForwardBlockRulesFresh; ListForwardBlocksFresh (ordered by id), GetForwardBlockFresh (by id and staff chat, nil for a missing or foreign row), CountForwardBlocksFresh; AddForwardBlock (one transaction: the staff_groups row must exist else ErrStaffGroupMissing, the count must be below ForwardBlockMax else ErrForwardBlockFull, then INSERT with ON CONFLICT DO NOTHING on the unique key, reporting added=false on a repeat); RemoveForwardBlockByID (one conditional DELETE by id and staff chat with the RETURNING clause, as RemoveFilterByID does, returning the removed row) and RemoveForwardBlockBySource; SetForwardBlockAction (validates against ForwardBlockActions before any write, requires the Staff Group row as above, one column-targeted INSERT ... ON CONFLICT (staff_chat_id) DO UPDATE of action, updated_by and updated_at so a value is always written). Every write drops the key after commit through one helper invalidateForwardBlockKeys(ids...).

4. fuku/db/staff/rekey.go: add both tables to the transaction's update list with the staff_chat_id column, and after the commit drop the forward block keys of the old and the new ID (also when only these tables changed). fuku/db/staff/repository.go: inside DeleteStaffGroupWithLinks's transaction, after the links are deleted and only when the Staff Group row was claimed, delete both tables' rows for that chat, and drop the key after the commit. Update the doc comments of both functions.

5. fuku/db/cache/local.go: add staffForwardBlockPrefix = CacheKey("staff_forward_block") + ":" next to the other prefixes, add it to skipLocal with a comment (the watcher reads it on every listed forward, a removed source must stop acting on every replica at the next message), and extend the prefix tables of local_test.go (the matching cases and the near-miss staff_forward_blockx).

6. Test wiring: add both models to the AutoMigrate list of fuku/db/staff/testmain_test.go and fuku/modules/test_harness_test.go (the only packages whose tests reach RekeyChat or DeleteStaffGroupWithLinks); extend staffCleanup in fuku/modules/staff_helpers_test.go to delete both tables' rows of its chats and the forward block cache keys; extend TestStaffTablesStayOutOfBackup in fuku/db/staff/repository_test.go with the names staff_forward_blocks, staff_forward_block_settings and forward_blocks.

7. Tests in the staff package (fuku/db/staff/forwardblock_test.go plus additions to rekey_test.go): TestStaffForwardBlockRepository (add, repeat, cap, missing Staff Group, remove by ID with a foreign chat's ID, remove by source, action default and every accepted and refused value, fresh reads), TestStaffForwardBlockCache (every write drops the key; a read after each write sees the new state; with the local layer on, the key is never served stale), TestStaffForwardBlockRekeyAndUnset (re-key moves both tables and both keys, another Staff Group untouched, unset deletes both tables, a second unset deletes nothing), TestStaffForwardBlockConstraints (UNIQUE, CHECK source negative, CHECK action, refused by the database with the named constraint where the dialect reports it). Add fuku/db/migrations/staff_forward_block_migration_test.go in the style of scheduled_deletion_migration_test.go: TestStaffForwardBlockMigrationStatements (database-free: two statements, the three named constraints inline, no transaction control, CONCURRENTLY, NOT VALID or top-level ADD CONSTRAINT, and a file name that sorts after 20261113120000) and TestStaffForwardBlockMigrationDefaults (PostgreSQL only: the statements run twice without error, a row with only the required columns reads the defaults, the constraints exist in pg_constraint and refuse a duplicate, a non-negative source and an action outside the five). Add TestStaffForwardBlockConstraints and TestStaffForwardBlockMigrationDefaults to the -run list of test-postgres-integrity in the Makefile (./fuku/db/staff and ./fuku/db/migrations are already in its package list).

8. Apply layer and commands in fuku/modules/staff_forwardblock.go. Constants and seams first. A write-seam struct forwardBlockWrites (Add, RemoveByID, RemoveBySource, SetAction, defaulting to the repository functions) in the style of blacklistWrites, and the variable forwardBlockResolve, a raw getChat through b.RequestWithContext with chat_id as the int64 for a numeric ref and the @name string for a name, decoding id, type and title, bounded by forwardBlockResolveTimeout (8 s). parseForwardBlockRef is pure: a negative numeric ID through parseStaffChatIDArg, an @name through staffUsernameRe, nothing else. Apply functions, each returning a typed result with a zero-value success outcome, judging no person's rights (that is the door's job), and logging the chat ID and the error only: applyForwardBlockAdd(ctx, b, staffChatID, userID, ref) implements point A exactly (outcomes added, already listed, invalid ref, not found, not a source, check failed, list full, no such Staff Group, save failed; a title is cut to 64 code points); applyForwardBlockRemove(ctx, staffChatID, sourceChatID) and applyForwardBlockRemoveByID(ctx, staffChatID, id), each distinguishing removed, gone (a missing, foreign or already removed row answer alike) and save failed; applyForwardBlockAction(ctx, staffChatID, userID, action) which checks the action against blacklistActionValues (the reused vocabulary) before any write and maps the repository's refusals. A pure function forwardBlockEntryText(sourceChatID, title) builds the text the page and the commands show (the ID, then the title collapsed to one line and escaped by settingsListLabel when spliced), and a pure forwardBlockAddText(tr, result) and its siblings map each outcome to the shared locale key so the commands and the page word every outcome identically; entries are spliced into the translated text through settingsListEntryText, never through the translator.

9. Commands, all through helpers.WrapCommand with descriptors named addFwdBlockDesc, rmFwdBlockDesc, fwdBlockListDesc and fwdBlockActionDesc (command names addfwdblock, rmfwdblock, fwdblocklist, fwdblockaction), handlers as value-receiver methods on moduleStruct registered from LoadStaff in fuku/modules/staff.go with staffModule.method so the docs generator lists them under Staff; none is Disableable. Every descriptor's checks are helpers.RejectAnonymousSender, helpers.RequireGroup and a new check requireStaffGroup that reads staff.GetStaffGroupFresh and replies staff_unset_not_staff for a chat that is not one (an error replies staff_check_failed). Writes (the add and remove commands, and fwdblockaction with an argument) additionally call requireStaffGroupCreator: chat_status.CheckOwner on the Staff Group with the sender, unknown replies staff_check_failed, a mismatch replies staff_fwdblock_owner_only, nothing is written on either (point B). Reads (fwdblocklist, fwdblockaction with no argument) use helpers.RequireUserAdmin. addfwdblock and rmfwdblock take exactly one argument (rmfwdblock takes the numeric ID the list shows), zero or more replies the usage key. fwdblocklist shows at most 40 sources ordered by ID with the ID in code and the escaped title, then a more-line with the remaining count, and the current action word through settingsBlacklistActionWord. fwdblockaction with an argument lower-cases it and answers the changed text or the usage text. All replies go through replyStaff.

10. Locale keys, added to all 8 locale files in this task and nothing else: staff_fwdblock_owner_only, staff_fwdblock_usage_add, staff_fwdblock_usage_rm, staff_fwdblock_added, staff_fwdblock_already, staff_fwdblock_not_found, staff_fwdblock_not_source, staff_fwdblock_full (takes {max}), staff_fwdblock_check_failed, staff_fwdblock_removed, staff_fwdblock_not_listed, staff_fwdblock_list_header, staff_fwdblock_list_empty, staff_fwdblock_list_more, staff_fwdblock_action_current, staff_fwdblock_action_changed, staff_fwdblock_action_usage. Entry-bearing keys use a named param ({source}) that the code splices after translation. The owner-only text is also the page's alert later, so keep it under 180 characters and plain. Extend staff_help_msg in all 8 files with one short section (about 350 characters in English, shorter in the longer languages so the /staff panel keeps fitting its help) naming the four commands, that only the Staff Group creator changes the list, that it applies in every linked group, and the group-origin limit in one sentence. Reuse staff_unset_not_staff, staff_check_failed, common_settings_save_failed and the settings_opt_* words; do not add a second copy of any of them.

11. Tests in fuku/modules: TestApplyForwardBlockAdd (a table through the forwardBlockResolve seam with a scripted fake, covering every outcome of point A and that nothing is stored on every refusal), TestApplyForwardBlockRemove and TestApplyForwardBlockAction (including a failing write seam, and that a refused action writes nothing), TestForwardBlockVocabulary (staff.ForwardBlockActions equals blacklistActionValues as a set), TestForwardBlockCommands (the real dispatcher with LoadStaff and the staff bot fake from staff_helpers_test.go: the creator adds by ID and by @name, lists, removes, sets and reads the action; a live administrator who is not the creator can list but every write is refused with staff_fwdblock_owner_only and the row count and cache are unchanged; an anonymous sender, a plain group that is not a Staff Group, and an unknown CheckOwner each get their own answer and write nothing; the Staff Group's cache key is dropped by each write), and a registration test that the four commands are recorded by helpers.IsBotCommand. Use fresh chat IDs from uniqueModuleChatID, clean up with staffCleanup, never call t.Skip, and keep the cross product small because fuku/modules is near its CI time limit.

12. AGENTS.md, same commit. Data: a bullet for staff_forward_blocks and staff_forward_block_settings (columns, constraints, the key and its skipLocal listing, the soft cap, never part of backup, import, export or reset); amend the Staff Group chat migration bullet (RekeyChat also re-keys these two tables and drops their keys) and the Staff links and staff tables bullet where it says what unset deletes; add the two new test names to the make test-postgres-integrity description. Subsystem traps: one bullet for the commands (authority is the live creator for writes, any admin for reads, the apply functions, the resolver seam and point A). Then run make generate-docs and keep the regenerated docs/src/content/docs/commands/staff/index.md.

Using the creator-only write per point B (research suggested creator or administrator; the core rule and the link invariant decided it), and the reused blacklist vocabulary per D-02.
  </action>
  <verify>
    <automated>changed=$(git status --porcelain) && test -z "$(printf '%s\n' "$changed" | awk '{print $2}' | grep '\.go$' | xargs -r gofmt -l)" && CGO_ENABLED=0 go build ./... && CGO_ENABLED=1 go vet -tags testtools ./fuku/db/staff ./fuku/db/models ./fuku/db/cache ./fuku/db/migrations ./fuku/modules && CGO_ENABLED=1 go test -tags testtools -race -count=1 ./fuku/db/staff ./fuku/db/cache ./fuku/db/migrations && CGO_ENABLED=1 go test -tags testtools -race -count=1 -run '^(TestApplyForwardBlock|TestForwardBlock|TestStaff|TestLoadStaff|TestModulesRecordTheirCommands|TestNoRawNewCommand)' ./fuku/modules && make check-translations && make check-docs</automated>
  </verify>
  <done>An operator who is the live creator of a Staff Group can add (by ID or @username), list, remove and set the action of the forward blocklist by command; any other admin can only read; the data survives a chat migration and is deleted with the Staff Group; the cache key is skipLocal and dropped on every write; the migration, the model and the repository agree on UNIQUE and CHECK; locale files, staff help and generated docs are in sync; the whole staff test family still passes; AGENTS.md describes the data, the commands and the new RekeyChat behaviour; committed alone as feat(staff) on the current branch, never pushed.</done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: The group -9 watcher, exemptions, live authority and punishments</name>
  <files>fuku/modules/staff_forwardblock_watch.go, fuku/modules/staff.go, fuku/modules/staff_forwardblock_watch_test.go, fuku/modules/staff_forwardblock_env_test.go, locales/en.yml, locales/es.yml, locales/fr.yml, locales/hi.yml, locales/id.yml, locales/pt.yml, locales/ru.yml, locales/uk.yml, AGENTS.md, docs/src/content/docs/architecture/handler-groups.md</files>
  <read_first>
    AGENTS.md (Handlers: group numbers, the -8 MemberDeletion and -2 bullets; Permissions; Subsystem traps: the staff links, member deletion, AI never decides alone bullets)
    fuku/modules/blacklists.go (blacklistWatcher, the punishment primitives and notices)
    fuku/modules/member_deletion.go (memberDeletionExempt, memberDeletionWarnf, the named-result handler)
    fuku/modules/member_deletion_batcher.go (memberDeleteWarnGate)
    fuku/modules/raid_flood.go (raidFloodSenderCounts)
    fuku/modules/staff_recheck.go (recheckLink)
    fuku/modules/staff_watchers.go
    fuku/modules/staff.go (LoadStaff)
    fuku/modules/staff_helpers_test.go and staff_action_fake_test.go (the bot fakes)
    fuku/modules/member_deletion_env_test.go (environment style)
    fuku/modules/warns.go (warnThisUser, warnSource)
    docs/src/content/docs/architecture/handler-groups.md
  </read_first>
  <behavior>
    - The tracer, through the real dispatcher: with a Staff Group, a link, a listed channel and action none, a member's message forwarded from that channel into the linked group is deleted and nothing else is sent.
    - Each action: warn issues one automatic warn, mute restricts with MutedPermissions and posts the notice, kick removes the member and posts the notice, ban bans with no end date and posts the notice, and each notice is scheduled as bot:punish_auto; the notice and the warn reason say the member forwarded from a blocked source and name neither the source nor the Staff Group.
    - An album of three forwarded items deletes all three and punishes the member once inside the 60 s window; a later forward after the window punishes again.
    - Spared with no write and no Telegram call beyond reads: not in the list, an unlinked group, the Staff Group chat itself, a non-forward, a forward whose origin is a user or hidden user, a creator or administrator, an approved user, an anonymous admin, any sender_chat sender, an automatic forward, a bot, a Telegram service account, the bot itself.
    - Unknown never acts: an unreadable list, an approvals error, an unknown admin status, a failed fresh link read, a link whose fresh Staff Group differs from the cached one, a recheckLink answering unknown, removed or gone each delete and punish nothing.
    - Authority memo: after one successful recheck a second listed forward within 60 s makes no owner check; after the memo expires or the link row's ID changes it asks again; a failed or unknown recheck is never remembered.
    - Failures: a bot that is not an administrator does nothing and logs one gated line; a missing delete right still punishes, a missing restrict right still deletes; a refused delete or punishment is logged once per hour per chat and nothing retries.
    - The handler returns ext.ContinueGroups on every path including a recovered panic, so a sibling handler of a later group still runs.
    - Cost: a non-forward and a forward from a user or hidden user make no cache read and no Telegram call; a channel forward in an unlinked group makes exactly one link read.
  </behavior>
  <action>
Implement D-01 (the list read through the group's link, acting only in the group where the forward was posted) and D-02 (one action for the whole list, from the blacklist vocabulary), points C to F.

1. Write fuku/modules/staff_forwardblock_watch.go with these parts. Constants: staffForwardBlockGroup (-9), forwardBlockPunishWindow (60 s), forwardBlockAuthMemoTTL (60 s), forwardBlockStateMax (10000). forwardBlockFilter is the handler filter and is exactly the check that the message has a forward origin. forwardBlockSourceOf(msg) is pure: it merges the origin and returns the chat ID of Chat (type channel) or, when that is nil, of SenderChat (type chat), only when its type is channel, supergroup or group and its ID is negative; user and hidden_user origins return false.

2. The watcher is a value-receiver method on staffModule with a named result preset to ext.ContinueGroups and defer error_handling.RecoverFromPanic as its first statement, returning that result on every path (a recovered panic must not return nil, which would leave the group and skip later handlers). Its body is a short pipeline of small functions in the exact order of point E: chat type, forwardBlockSourceOf, the cached link gate staff.GetLinkOfGroup, staff.GetForwardBlockRules for link.StaffChatID (an error logs through a gate and returns, a source not in the list returns), the pure sender shape (raidFloodSenderCounts, plus msg.IsAutomaticForward and msg.SenderChat), memberDeletionExempt, the bot rights from the cached predicates chat_status.IsBotAdmin, CanBotDelete and CanBotRestrict, the live authority step, then the actions. Reuse memberDeletionExempt as it is: its failure lines carry the MemberDeletion label, which is accepted. Do not call IsUserAdminForUpdate, IsApprovedForUpdate or chat_status.IsUserAdmin, which read a failed lookup as negative.

3. Live authority (forwardBlockAuthorized): read staff.GetLinkOfGroupFresh for the chat and require a non-nil link whose StaffChatID equals the cached link's, then consult the memo keyed by chat and holding the link ID, the Staff Group ID and an expiry from deletionNow() plus forwardBlockAuthMemoTTL; on a miss run recheckLink with newStaffOwnerPass() and proceed only on staffRecheckOK, remembering only that result. staffRecheckRemoved, staffRecheckGone and staffRecheckUnknown act on nothing (unknown logs through its gate). The memo is an in-process mutex-guarded map bounded at forwardBlockStateMax: a full map forgets expired entries and, when none can be forgotten, records nothing (as memberDeletionWarmMark does).

4. Punishing. The delete always happens first through helpers.DeleteMessageWithErrorHandling when the bot can delete; a punishment follows only when the in-process latch grants this (chat, member) pair for forwardBlockPunishWindow (an atomic check-and-set under a mutex, same bounding rule as the memo, clock from deletionNow()), so an album is deleted item by item and punished once. The action is the rules' action. none does nothing more. warn calls warnsModule.warnThisUser with warnSourceAuto and the translated reason and ignores the ext.EndGroups and ext.ContinueGroups sentinels exactly as blacklistWatcher does. mute calls RestrictChatMember with MutedPermissions and no end date, kick calls kickMember, ban calls BanChatMember with no end date. After a mute, kick or ban the notice is sent with the existing templates blacklists_bl_watcher_muted_user, _kicked_user and _banned_user spliced with formatting.MentionHtml and the translated reason, in the group's language (ctxTr), and scheduled through scheduleBotMessage(sent, deletion.KindBotAuto). The reason is one new key, staff_fwdblock_reason, added to all 8 locale files in this task (its text is a short noun phrase that fits "Muted X due to ...", names no source and no Staff Group, and is not a printf template). A skipped punishment (the latch refused, the restrict right missing) is not an error.

5. Failure gates (point F): a package array of memberDeleteWarnGate values forwardBlockWarnGates, one per kind (cannot act, list unreadable, authority unknown, delete failed, punishment failed), each with its own label, logged through a small forwardBlockWarnf that takes the kind and chat ID; every line carries chat and user IDs and the Telegram error code or class only, never a title, text or message ID. The watcher starts no goroutine and writes nothing to Redis.

6. Register the handler in LoadStaff (fuku/modules/staff.go) with dispatcher.AddHandlerToGroup(handlers.NewMessage(forwardBlockFilter, staffModule.forwardBlockWatcher), staffForwardBlockGroup) and no setter, so edits, channel posts and bot senders never reach it. Group -9 holds no other handler.

7. Tests (write them first). Build the toolkit in staff_forwardblock_env_test.go on the pieces the existing tests use: a real dispatcher with LoadStaff, miniredis, a Staff Group, a link, the staff bot fake extended with scripted answers and request recording for deleteMessage, restrictChatMember, banChatMember, unbanChatMember, sendMessage, getChatMember and getChatAdministrators, and seams swapped through mreqSwap-style helpers restored in t.Cleanup; use fresh IDs from uniqueModuleChatID and uniqueLinkOwnerID and clean up the forward block rows, keys, warns and latch state. TestForwardBlockWatcherDeletes is the first test and is the end-to-end tracer. Then TestForwardBlockWatcherPunishments (all four actions plus the album latch and the window), TestForwardBlockWatcherSpares (one case table for every spared case of the behaviour list, each asserting no delete, no punishment and no write), TestForwardBlockWatcherUnknownNeverActs, TestForwardBlockWatcherAuthority (owner changed removes the link and acts on nothing, the memo and its expiry and link-ID change, unknown is not remembered), TestForwardBlockWatcherBotRights, TestForwardBlockWatcherNeverEndsUpdate (every path returns ext.ContinueGroups, a panic injected through a seam still does, and a later-group sibling handler runs), TestForwardBlockWatcherCost (counting the cache and Telegram reads for the three cheap cases), TestForwardBlockSourceOf (a table over origin types), and TestForwardBlockWatcherGroup (the handler is registered at -9 and the edit of a forwarded message never reaches it). Keep cases few and light: fuku/modules ran 19 to 30 minutes on CI runners, so use one environment per test and the worst cases, not cross products.

8. AGENTS.md, same commit. Handlers: add -9 to the group list (the forward blocklist watcher, the only handler of its group, never ends an update) and a bullet describing the watcher (the filter, the order of point E, the reused exemption, the authority step and memo, the latch, the notices, the group-origin limit, the sender_chat and anonymous admin exemption, that it never ends an update so the blacklist watcher can also act). Subsystem traps: one bullet for the failure gates and the accepted limits (per replica latch and memo, double action with the blacklist, albums). Add the new test family names to the Testing section. docs/src/content/docs/architecture/handler-groups.md: add the -9 row in the table's style and one bullet in its notes list. Then run make generate-docs and keep whatever it rewrites under docs/src/content/docs (the generator also scans message watchers); make check-docs must pass.

Using group -9 per point E (the only free group below 12), the safe-failure exemption per D rule "unknown never punishes", and the reused blacklist notices and vocabulary per D-02.
  </action>
  <verify>
    <automated>changed=$(git status --porcelain) && test -z "$(printf '%s\n' "$changed" | awk '{print $2}' | grep '\.go$' | xargs -r gofmt -l)" && CGO_ENABLED=0 go build ./... && CGO_ENABLED=1 go vet -tags testtools ./fuku/modules && CGO_ENABLED=1 go test -tags testtools -race -count=1 -run '^(TestForwardBlock|TestApplyForwardBlock|TestStaff|TestLoadStaff|TestMemberDeletion|TestMemberEdited|TestMemberSelfDestruct|TestBlacklistWatcher|TestSettingsPhase18InStaffGroup|TestModulesRecordTheirCommands|TestNoRawNewCommand|TestCommandRepliesStayInsideTheHandler)' ./fuku/modules && make check-translations && make check-docs</automated>
  </verify>
  <done>A message forwarded from a listed source into a linked group is deleted and the forwarder gets the list's action once per album, through the real dispatcher; every exemption and every unknown case acts on nothing; the live authority step and its memo are tested; failures log through hourly gates and never retry; the handler never ends an update and runs at group -9 as its only handler; AGENTS.md and handler-groups.md describe it; every earlier staff, member deletion and blacklist test still passes; committed alone as feat(staff) on the current branch, never pushed.</done>
</task>

<task type="auto" tdd="true">
  <name>Task 3: The private-only Blocked sources page, the Staff Group creator right, docs</name>
  <files>fuku/modules/settings_fwdblock.go, fuku/modules/settings_fwdblock_test.go, fuku/modules/settings_fwdblock_screens_test.go, fuku/modules/settings_list.go, fuku/modules/settings_list_input.go, fuku/modules/settings_menu.go, fuku/modules/settings_phase19_test.go, fuku/modules/settings_list_framework_test.go, fuku/modules/settings_edit_hooks_test.go, fuku/modules/settings_list_locale_test.go, fuku/i18n/settings_locale_test.go, locales/en.yml, locales/es.yml, locales/fr.yml, locales/hi.yml, locales/id.yml, locales/pt.yml, locales/ru.yml, locales/uk.yml, AGENTS.md, docs/src/content/docs/commands/staff/index.md</files>
  <read_first>
    AGENTS.md (Callbacks: settings|v1, the list engine and Blacklist page bullets, PrivateOnly; Permissions: the settings rights bullets; Testing: the settings and Phase 19 families)
    fuku/modules/settings_list.go and settings_list_input.go
    fuku/modules/settings_blacklist.go (the model of this page)
    fuku/modules/settings_filters.go
    fuku/modules/settings_menu.go (settingsRight, settingsRightRefusalFor, settingsRightBotFirst and settingsRightBotAfter)
    fuku/modules/settings_pages.go (settingsPage, Hidden, settingsMainEntries)
    fuku/modules/settings_input_door.go (authorize, refuse, keep, end)
    fuku/modules/settings_list_env_test.go (the list toolkit) and settings_blacklist_test.go
    fuku/modules/settings_phase19_test.go, settings_list_framework_test.go, settings_edit_hooks_test.go
    fuku/i18n/settings_locale_test.go
    fuku/modules/staff_forwardblock.go (the apply functions and shared texts from task 1)
  </read_first>
  <behavior>
    - The page exists only for a Staff Group: Main leaves it out and openPage refuses a forged press in every other group, and in the Staff Group's own group menu it shows the private-only note.
    - In the Staff Group's private menu the page lists the sources eight a page (ID and escaped title), pages, shows the action line, and an empty list shows the intro and the empty line plus the Add and Action buttons.
    - The live creator of the Staff Group can Add (one source per typed message through applyForwardBlockAdd, with every refusal worded by the shared texts and the wait kept), delete one behind the two-tap card (applyForwardBlockRemoveByID, a foreign or gone row answers already removed) and set the action (applyForwardBlockAction) from the Action screen; every one of those presses by a live administrator who is not the creator is refused with the staff_fwdblock_owner_only alert and writes nothing, while viewing and paging work for them.
    - The page and the commands leave the same row and cache state for the same input, and the page draws exactly what the commands and the watcher read.
    - Every button is at most 64 bytes at the worst case, every screen fits 3800 UTF-16 units in all 8 languages, and opening Main or the page on a Staff Group with no rows creates no row.
  </behavior>
  <action>
Implement D-03 (a private-only list page on the list engine, sharing one apply function per write with the commands) and point B (writes are the Staff Group creator's).

1. Framework edits, small and additive, each with its tests. (a) settings_menu.go: append a tenth right settingsRightStaffOwner after settingsRightRestrictBotRestrict so the zero value stays settingsRightAdmin; settingsRightRefusalFor answers "" for a member whose status is creator and otherwise the alert key staff_fwdblock_owner_only (alert true); it has no bot half, so settingsRightBotFirst and settingsRightBotAfter are unchanged. Update the doc comments. (b) settings_list.go: add an optional Hidden func(*gotgbot.Chat) bool to settingsListDef and pass it to the settingsPage that settingsListPage builds. (c) settings_list_input.go: add the part (page v, code s, step 1, WordKey settings_entries_part_source) to the static settingsListParts table. Update the three existing tests that pin these sets as expected-set edits only, never loosening an assertion: settings_list_framework_test.go (the rights table becomes ten and gets the new right with no bot half), settings_edit_hooks_test.go (the member-part verdict table gets creator allowed, administrator and plain member refused with the new key), settings_phase19_test.go (the registered page keys and the part table gain the new page and part, and the list action codes gain the seven new ones). Run the whole TestSettings family and fix only expectations that legitimately change, for example a Staff Group's Main shape and counts of registered pages.

2. Page file fuku/modules/settings_fwdblock.go, registered from its own init through registerSettingsList (settings_pages.go is not edited). Page key v, Order 11, icon and title key settings_page_fwdblock (the cell, icon and name, must fit 20 code points in every language), Hidden true unless the cached gate staff.GetStaffGroup(group.Id) answers a Staff Group (an error or a missing row hides the page; the apply functions and the repository's in-transaction Staff Group check are what stop a write after an unset). Action codes: pager vg, delete ask vx, delete Yes vy, Add va, Cancel vc, action screen ve and action set vs. CountMax is staff.ForwardBlockMax so Main reads n/200. Right is settingsRightStaffOwner for the delete ask and Yes, Add and the action set; viewing, paging and Cancel keep settingsRightAdmin through the engine. The reads are variables so a test can count or fail them: settingsFwdBlockRead, settingsFwdBlockEntry and settingsFwdBlockCount, built with the engine's generic adapters over staff.ListForwardBlocksFresh, GetForwardBlockFresh and CountForwardBlocksFresh, never the cached rules; the Read also reads the list's action once (staff.GetForwardBlockRulesFresh) and stamps it on every entry's Action so the engine's BodyExtra can draw the action line with the reused settingsBlacklistActionLine (all entries share one action). An entry's text is forwardBlockEntryText. Typed is a one-step handler like settingsBlacklistTyped: the text must be exactly one token (otherwise refuse with a one-source-at-a-time key), claim the wait, call applyForwardBlockAdd with the reply's group as the Staff Group and the sender as the user, and on any outcome other than added release the claim and refuse with the shared text (a check-failed outcome keeps the wait with settings_check_failed, as the door does); on added finish the wait, log the chat and user IDs only, send the saved list at the page of the new entry with a notice and close the prompt. The Action screen is a small renderer in the page file: the reused settings_bl_action_intro and settings_bl_action_line, one hint key of its own, the two choice rows [Warn | Mute | Kick] and [Ban | Delete only] with the stored action marked through settingsSelectedPrefix, and Back to page 1 and Close; each choice presses vs with v one of blacklistActionValues (validated in step 1) and calls applyForwardBlockAction; unlike the blacklist screen it is also drawn for an empty list, because the action is a list-wide setting. Reuse settingsBlacklistActionWord and settings_btn_bl_action for the button; do not copy the blacklist's choice-row builder logic beyond the few lines that bind the new action code.

3. Locale keys, all 8 files: settings_page_fwdblock, settings_fwd_intro, settings_fwd_count (takes {n} and {max}), settings_fwd_empty, settings_fwd_delete_ask ({source}), settings_fwd_removed ({source}), settings_btn_fwd_add, settings_fwd_wait, settings_fwd_wait_rules, settings_fwd_action_hint, settings_fwd_saved ({source}), settings_fwd_refuse_one, settings_entries_part_source. Each gets a row in settingsLocaleLimits (fuku/i18n/settings_locale_test.go) and the page title in the label-length groups (cell at most 20 code points with the icon); the page name, delete ask and removed notice are read through table fields that make check-translations cannot see, so list their keys in settingsListExtraKeys (fuku/modules/settings_list_locale_test.go), the mechanism the Filters and Notes pages use; staff_fwdblock_owner_only joins the alert-length list. The intro says what the page does in one or two sentences (deleted in every linked group, then the action, admins and approved members exempt) and names no Staff Group title.

4. Tests (write them first), in settings_fwdblock_test.go and settings_fwdblock_screens_test.go on the list toolkit (settings_list_env_test.go) with the Staff Group fixture of task 1: TestSettingsFwdBlockOpens (Main and the page in the Staff Group's private menu; Hidden and the forged-press refusal in a non-staff group and in a Staff Group whose cached gate says no; the private-only note in the Staff Group's own group menu), TestSettingsFwdBlockAdd (the typed add through the door: success, one-source rule, every refusal outcome with the wait kept, check failed keeps the wait, a Redis-less Add is the alert), TestSettingsFwdBlockDelete (the card binds the row, expiry, foreign and gone rows, a demotion between the taps refuses), TestSettingsFwdBlockAction (every value, the stale and forged values refused in step 1, an empty list), TestSettingsAuthorityParityFwdBlock (creator writes by page and command alike; a live administrator who is not the creator is refused by both with nothing written, viewing and paging work; an administrator the owner gate refuses is refused by the menu, the framework's documented carve-out), TestSettingsParityFwdBlock (page and command leave the same row and cache state, through the shared apply functions), TestSettingsDriftFwdBlock (the page draws what the commands stored and the watcher acts on), TestSettingsListFwdBlockCallbackBytes (render a long list with 17-digit row IDs and a 14-character chat ID: every button at most 64 bytes and Omitted zero), TestSettingsListFwdBlockPageFitsEveryLocale (3800 UTF-16 units and 20-code-point cells in 8 languages with a 64-rune title and the longest refusals) and TestSettingsFwdBlockReadsCreateNothing (opening Main and the page on a Staff Group with no rows creates no row in either table and fills no cache key). Add the page to TestSettingsStaffPrivacy's Staff Group screens only where that test already renders a Staff Group's own menu. Keep the locale sweep to the worst few cases.

5. AGENTS.md, same commit. Update every rule this task invalidates: the nine rights become ten (the Permissions rights bullets and the sentence that counts them), the static part table gains (v,s) (Callbacks and the Phase 19 Testing bullet that says five), the Main grid notes (the page is hidden outside a Staff Group, so the 18-cell grid and its constants are unchanged, and a Staff Group's Main gains one cell). Add: a Callbacks bullet for the Blocked sources page (key v, Order 11, the seven codes, Hidden, the right, the typed add, the action screen drawn for an empty list, the reused words) in the style of the Blacklist page bullet; a Permissions bullet for settingsRightStaffOwner and for the creator-only rule with its reasoning (point B) and the view-only administrators; a Testing bullet naming the new families. Then run make generate-docs and keep the regenerated docs, and run the whole settings family.

Using the new creator right per point B, the list engine's Hidden hook (an additive field), and the shared apply functions of task 1 so no second write path exists (D-03).
  </action>
  <verify>
    <automated>changed=$(git status --porcelain) && test -z "$(printf '%s\n' "$changed" | awk '{print $2}' | grep '\.go$' | xargs -r gofmt -l)" && CGO_ENABLED=0 go build ./... && CGO_ENABLED=1 go vet -tags testtools ./fuku/modules ./fuku/i18n && CGO_ENABLED=1 go test -tags testtools -race -count=1 -run '^(TestSettings|TestLoadSettings|TestForwardBlock|TestApplyForwardBlock)' ./fuku/modules ./fuku/i18n && CGO_ENABLED=1 go test -tags testtools -race -count=1 ./scripts/generate_docs/... && make check-translations && make generate-docs && make check-docs</automated>
  </verify>
  <done>The Staff Group's creator manages the blocklist from a private-only Blocked sources page that shows only for a Staff Group, with Add, a two-tap delete, a list-wide action screen and paging; a live administrator who is not the creator can view and is refused every write with the same alert as the command; the page and commands share the apply functions; the tenth right, the Hidden hook and the new part are tested and every existing settings test still passes with only expected-set edits; the locale files, limits tables, generated docs and AGENTS.md are in sync; committed alone as feat(settings) on the current branch, never pushed.</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| Telegram update to watcher | A member chooses what to forward; forward_origin is set by Telegram, but the sender, text and album shape are attacker-controlled |
| Staff Group editor to linked groups | One list changes what the bot does in every linked group, groups the editor may not administer |
| Operator input to repository | An @username or ID typed in the Staff Group or in the private page |
| Cache and replicas | The list and the link are read per message on several replicas |
| Callback data to page | Callback data is not authenticated by Telegram |

## STRIDE Threat Register

Threat IDs are unique within this quick task (no other plan of the phase exists). T-tjj-SC is reserved and kept.

| Threat ID | Category | Component | Severity | Disposition | Mitigation Plan |
|-----------|----------|-----------|----------|-------------|-----------------|
| T-tjj-01 | Elevation of privilege | list writes (commands and page) | critical | mitigate | Writes need the Staff Group's live creator: chat_status.CheckOwner for the commands, settingsRightStaffOwner on the press's own fresh member record for the page and the typed reply; other admins view only. The creator is the creator of every linked group because a link is removed when its maker stops being the live creator of either side. Tests: TestForwardBlockCommands, TestSettingsAuthorityParityFwdBlock write nothing for a non-creator administrator. |
| T-tjj-02 | Elevation of privilege | enforcement through a stale link | high | mitigate | Before any delete or punishment the watcher reads the link fresh, requires the same Staff Group as the cached gate, and runs recheckLink (live owner check of both sides); removed, gone or unknown acts on nothing; success is memoised 60 s only. Tests: TestForwardBlockWatcherAuthority. |
| T-tjj-03 | Tampering | wrongful deletion or punishment of exempt or unknowable senders | high | mitigate | memberDeletionExempt (approved, creator or administrator, and unknown all exempt), sender shape (sender_chat, automatic forward, service accounts, bots, the bot), unreadable list or link never acts. Tests: TestForwardBlockWatcherSpares, TestForwardBlockWatcherUnknownNeverActs. |
| T-tjj-04 | Denial of service | album or forward flood causing API calls, warns and owner checks | medium | mitigate | In-process punish latch per (chat, member) for 60 s, authority memo, cached bot-right predicates, hourly per-chat log gates, bounded maps (10000), no goroutine. Tests: TestForwardBlockWatcherPunishments (album), TestForwardBlockWatcherAuthority (memo), TestForwardBlockWatcherBotRights. |
| T-tjj-05 | Information disclosure | notices, logs and menus naming a source or the Staff Group | medium | mitigate | The reason and notices name neither; logs carry chat and user IDs and error class only; the page is private-only and the commands answer only inside the Staff Group; a group menu shows only the framework note. Tests: TestSettingsFwdBlockOpens, the Staff Group privacy screens, a log-hook assertion in TestForwardBlockWatcherBotRights. |
| T-tjj-06 | Spoofing | an @username that resolves to something else | medium | mitigate | Live getChat, type must be channel, supergroup or group with a negative ID, the numeric ID is stored and never re-resolved, only the creator can enter it. Tests: TestApplyForwardBlockAdd. |
| T-tjj-07 | Tampering | stale cache after a write on another replica, a chat migration or an unset | medium | mitigate | skipLocal prefix, DeleteCache on every write, RekeyChat re-keys both tables and DeleteStaffGroupWithLinks deletes them in its transaction. Tests: TestStaffForwardBlockCache, TestStaffForwardBlockRekeyAndUnset. |
| T-tjj-08 | Repudiation | who changed the list | low | mitigate | created_by and updated_by columns plus an Info log line with chat and user IDs on each write; no audit post is sent (accepted, the list is staff policy visible to the Staff Group's admins). |
| T-tjj-09 | Denial of service | unbounded list growth | low | mitigate | Soft cap of 200 sources enforced inside the insert transaction; the page shows n/200. |
| T-tjj-10 | Tampering | forged page callbacks | medium | mitigate | Strict step 1 field validation with the owner field c required, row IDs scoped by (id, staff chat) and read fresh, the delete card binds the row, every press re-judges the right on a fresh member record. Tests: the page's forged-press cases and TestSettingsListForged coverage for the new codes. |
| T-tjj-SC | Tampering | package installs | high | accept | No new dependency is added: no npm, pip, cargo or Go module change, so no package legitimacy gate applies. |
</threat_model>

<verification>
Run after each task, from the checkout root (the full list is in each task's automated command):
1. Format: gofmt on the changed Go files only (16 unrelated files are already unformatted at HEAD).
2. CGO_ENABLED=0 go build ./...
3. go vet -tags testtools on the changed packages.
4. Focused tests with CGO_ENABLED=1 go test -tags testtools -race -count=1 and the regexes in the task commands; task 3 also runs the whole TestSettings family.
5. make check-translations and make check-docs (make generate-docs first in task 3).
6. Lint fallback: golangci-lint is built with Go 1.25 and cannot run here, so rely on gofmt and go vet, keep every new function small (cyclomatic complexity under 15, the CI gate is 20), avoid repeated blocks over 100 tokens (dupl), and add no TODO or FIXME comment (godox); CI runs make lint.
7. make test (the only valid full run, 40 minutes) and make test-postgres-integrity are left to CI; the new PostgreSQL tests are TestStaffForwardBlockConstraints and TestStaffForwardBlockMigrationDefaults.
8. Before finishing: git log shows three commits on claude/ecstatic-goldberg-qsss05, nothing pushed, no new branch, no .planning files in the code commits.
</verification>

<success_criteria>
- The Staff Group creator can manage the list by command and by the private-only page; other admins can only view.
- A listed source's forwards are deleted in linked groups and the forwarder gets the one chosen action, once per album, with no action on any exempt or unknowable case.
- Chat migration moves the list, unset deletes it, every write drops the skipLocal key.
- Locale keys exist in all 8 files, check-translations and check-docs pass, AGENTS.md and handler-groups.md describe the feature, and the three commits are atomic and unpushed.
</success_criteria>

<output>
Create `.planning/quick/261113-tjj-staff-group-forward-source-blocklist-blo/261113-tjj-SUMMARY.md` when done
</output>
