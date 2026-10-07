# AGENTS.md

Alita Robot: Telegram group-management bot. Go module `github.com/divkix/Alita_Robot`; PostgreSQL (GORM) + Redis.
Update this file in the same commit as any change that invalidates a rule below.

## Commands

```bash
make test                 # the only valid full test run (tags, race, coverage, skip gate)
make lint                 # golangci-lint v2
make check-translations   # missing locale keys
make check-docs           # generated-docs drift; fix with `make generate-docs`
make bump-version TAG=vX.Y.Z   # the only way to change version strings
go test -tags testtools -race -count=1 -run '^TestName$' ./alita/db/warns   # one test
CGO_ENABLED=0 go build ./...   # compile check; `make build` needs goreleaser v2 + Docker
```

## Startup and shutdown

- `alita/config` `init()` → `alita/db` `init()` connects to Postgres **before `main()`** and runs migrations when
  `AUTO_MIGRATE=true`. Never add DB access to an `init()` that can run before `alita/db`'s.
- `--version`, `--health`, and `*.test` binaries (unless `ALITA_TEST_DATABASE=true`) skip DB init. Guard new pre-`main`
  side effects with `isCliModeActive`.
- Shutdown runs LIFO within 60 s. Register new drains *after* DB-close so they run before it.
- `StopStaffActions` is one of those drains: it cancels running staff fan-outs, marks every unfinished group
  "interrupted by restart", delivers the final summary and returns within 30 s. A delivery still waiting out a 429
  when the 30 s are up is cut off with the process. A hard crash can still leave ⏳ lines. A run cancelled by the
  shutdown skips its remaining log posts, so a group applied just before it may have no post. Undo runs join the
  same drain: their final record write (`FinalizeUndo`, or `ReleaseUndo` when no group reached its write, both through
  `db.DB`, never the cancelled run context) and summary delivery happen inside the same 30 s as a staff action's, and an
  unfinished group is recorded `fail_interrupted`. An undo Confirm joins `staffActionRunsWG` (`joinStaffRuns`) before it
  claims, so `StopStaffActions` waits for a Confirm that has already claimed, and its run gives the claim back when the
  shutdown cut every group off before its write. Once `StopStaffActions` has cancelled the run context an undo Confirm
  claims nothing and aborts its card with the restart text (`staff_undo_abort_restarting`).
- `StopLockdownWorker` is another drain registered after DB-close. It cancels the lockdown worker and waits at most
  5 s (`lockdownWorkerStopWait`), because every lockdown row resumes from the database: a cycle cut off at shutdown
  loses nothing, and a restart or a second replica picks the rows up again. A claim (an acting or unbanning joiner row
  with `claimed_at`) older than 2 minutes (`lockdownStaleClaim`) belongs to a worker that stopped: the first cycle of any
  replica releases it through `lockdown.ReleaseStaleClaims`, before any other step and without counting an attempt.
  Acting goes back to pending in an active lockdown, to banned (a ban) or cancelled (a join request) in a lifting one,
  and unbanning goes back to banned, so a restart resumes every ban, decline and unban and the live check at the lift
  keeps a repeated unban safe.
- Deploy manifests set `AUTO_MIGRATE=true`; the code default is `false`. Never call `gorm.AutoMigrate` in production code.

## Handlers

- Group numbers are execution order: `-10` captcha sweeper · `-7` lockdown join guard · `-6` fed-ban · `-5` antiraid ·
  `-3` staff watchers (chat-migration re-key; later ownership and bot health) · `-2` admin-cache ·
  `-1` users tracker · `0` commands/help/greetings · `3` aispam · `4` antiflood · `5`/`6` locks · `7` blacklists ·
  `8` reports+reactions · `9` filters · `10` pins · `11` log-channel.
- gotgbot stops a group at the first matching handler. Watchers return `ext.ContinueGroups`; commands return
  `ext.EndGroups`. A group-0 watcher returning `nil` silently kills every later group-0 handler.
- Modules register in `init()` via `RegisterLegacyModule(name, priority, load)`. Use a unique name; duplicates are
  dropped silently. Priority sets load order and group-0 precedence; `LoadHelp` loads last.
- New commands use `helpers.WrapCommand(dispatcher, CommandDescriptor{...}, handler)`. Set `Disableable: true` for any
  command chats may disable. Do not add replies for failed checks; the pipeline sends them.
- Anonymous admins bypass `WrapCommand`. Admin commands that must work for them also need
  `RegisterAnonymousAdminHandler` + `anonPipelineHandler`.
- The `StaffActions` module (priority 65) registers raw `handlers.NewCommand` interceptors at group 0 ahead of Bans (70)
  and Mutes (80), looping over a table so the docs generator skips them. Outside a Staff Group they return
  `ext.ContinueGroups` with no reply, write or Telegram call. They are a documented exception to the `WrapCommand`
  rule, because `BuildCommandContext` replies to sender-less updates, and they have no anonymous-admin re-entry.
- Handler methods use value receivers on `moduleStruct`.

## Callbacks

- Encode/decode only through `alita/utils/callbackcodec` (`<ns>|v1|<url-encoded>`, 64-byte cap). Never `strings.Split`.
- `encodeCallbackData` returns `""` on overflow, which ships a dead button. Put user text in Redis behind a short token.
- The staff namespace's history list is `a=rc&o=<offset>` (the `/staff` panel's Recent actions, and its Prev and Next).
  Paging uses an offset cursor, not a page number, so a page shrunk to the 3800-unit cap hides no entry: Next starts
  right after the last line shown. Callback data carries only numbers or tokens, never names or reasons, and the
  handler refuses an offset outside 0..`staffHistoryMaxOffset` as expired.
- Undo is `a=ya&r=<record id>` (the button on a finished summary, any live Staff Group member) and `a=yc` / `a=yn&t=<token>`
  (the undo card's Confirm and Cancel, Confirm only by the member who pressed Undo). The staff action Confirm (`xc`)
  refuses an undo card and the undo Confirm refuses an action card, so a replica on older code never runs an undo card.
  A press from a service identity (`staffServiceUserIDs`) is refused with "post as yourself" at Ask and Confirm before
  any lookup, and another member's tap on an undo card gets the presser-only alert. On a record whose undo was claimed,
  Ask and Confirm say whether that undo is running, was interrupted, undid something or changed nothing
  (`staffUndoStateOf`, through `staffUndoClaimedText`), and post nothing else.
- The history detail view is `a=dt&r=<record id>&o=<offset>` (`r` is the `staff_actions` row ID, `o` the list offset
  Back returns to). The record is loaded fresh and refused unless its `staff_chat_id` is the pressed message's chat, so a
  forged or replayed button naming another Staff Group's record shows nothing. A zero, negative, non-numeric or
  over-63-bit ID is answered as expired before any read. The view reads only the audit record and the current link list
  and makes no Telegram call to linked groups; an unfinished record's stale pending groups are shown as interrupted
  without writing anything. While `staffActionUndoable` holds it also shows the same `a=ya&r=<record id>` Undo button as
  the summary, above Back; that press goes through `staffUndoAsk` like any other, so every undo check runs again and the
  button grants nothing by itself. Once an undo is claimed the button is gone and the view shows who undid it and each
  group's undo result. The Recent actions line and the detail view show the undo's state from the stored undo outcomes
  (`staffUndoStateOf`, with the list's count from `TallyActionGroups`' `UndoDone`), never from the claim alone: undo
  running, undo interrupted (unfinished with a heartbeat older than `staffTargetLockTTL`; its pending groups are shown
  as interrupted without writing anything), undone (at least one group undone) or undo changed nothing. The Undo button
  shows only while `staffActionUndoable` holds.

## Permissions

- `alita/utils/chat_status` predicates return bools and never reply; use `PermissionResponder` to message.
  `helpers.CheckFunc` replies and is valid only inside `WrapCommand`.
- `IsUserAdmin` returns false for channel and non-positive IDs. Never pass a chat ID as a user ID.
- `*ForUpdate` predicates are memoised per update. Watchers only, never after a state change in the same update.
- `/lockdown` and `/unlockdown` authority is `requireLockdownAuthority`, a live `getChatMember` of the sender (creator,
  or administrator with `can_restrict_members`), never the admin cache, the Telegram service IDs or a chat's AnonAdmin
  setting; a failed lookup refuses. Lockdowns need a supergroup, and `/lockdown` is refused, with nothing recorded,
  unless the bot is an administrator with `can_restrict_members` and the group's permissions are readable.
  `/lockdownstatus` uses the same live check without needing `can_restrict_members` (any creator or administrator).
  An anonymous admin always gets `chat_status.PromptAnonAdminProof`, whatever the chat's AnonAdmin setting
  (`checkAnonAdmin`'s shortcut is not used); after the proof the tapper is the sender, is checked live like anyone
  else, and is the one recorded as having locked or lifted. `lockdown`, `unlockdown` and `lockdownstatus` are
  registered with `RegisterAnonymousAdminHandler` for that re-entry. After the proof the update no longer carries the
  callback query, so `chat_status.extractChatFromContext` (behind `helpers.RequireGroup` and `PermissionResponder`)
  finds no chat and fails silently: the lockdown commands use their own `requireLockdownGroup` and `lockdownRefuse`,
  which read `c.Chat` and reply through `c.Msg`. Keep every check and refusal of an anonymous-capable command off
  `ctx.Update`.

## Data

- Repositories live in `alita/db/<domain>/`. Read via `cache.GetFromCacheOrLoad` with `cache.CacheKey` keys.
  **Every write must `cache.DeleteCache` the keys it affects.**
- `GetFromCacheOrLoad` has an in-process layer (`CACHE_LOCAL_TTL`, default 10 s) in front of Redis. `DeleteCache`
  evicts it on this replica only; list freshness-critical keys in `skipLocal` (`alita/db/cache/local.go`). Tests that
  write rows directly must call `cache.ResetLocalForTest()` or `DeleteCache` before reading through the cache.
- Two packages are named `cache`; the loader and generation guards are in `alita/db/cache`, not `alita/utils/cache`.
- Operational Redis keys (`alita:antiraid:*`, `alita:anonAdmin:*`, `alita:staff:*`, `alita:lockdown:*`) sit outside the
  `alita:cache:` prefix; `CLEAR_CACHE_ON_STARTUP` does not clear them.
- `alita:staff:act:<token>` is the staff action card hash. It lives 5 minutes plus 1 minute grace while pending and
  1 hour once terminal, and moves state only through the Lua compare-and-set in `staff_action_card.go`.
  It expires through a timer on the creating replica plus a lazy check on any tap, and Confirm aborts when the
  signature of the sorted linked group IDs (`links_sig`) changed since the card was shown. The same hash also holds
  undo confirm cards (`action=undo`, `undo_of`, `undo_kind`) whose issuer is the presser of Undo. One undo per action
  is the conditional update of `staff_actions.undo_started_at` (`ClaimUndo`), not the card, which expires. The claim is
  given back by `ReleaseUndo` (one conditional update scoped to that claim's `undo_by` and `undo_started_at`, which also
  clears the groups' undo columns) when no group of the run reached its Telegram write (`staffGroupResult.Reached`, set
  once `executeStaffUndoCall` returned, whatever it returned, and for every group `sweepPending` swept after a panic,
  which may have reached its write before it was lost), and kept once any group did.
- `alita:staff:lock:target:<id>` is the per-target fan-out lock: `SET NX` with the card token as value and a 30-minute
  TTL, taken at Confirm, renewed every 10 minutes (`staffTargetLockRenewEvery`) by the run's coordinator through a
  compare-and-set on that token while the fan-out runs (a vanished key is re-taken, another card's lock is never
  touched), and released by compare-and-delete with that token when the run ends or the Confirm aborts. An undo's
  Confirm takes the same lock, so an undo and a staff action on one person never run at once; the loser is answered
  "target busy" and its card stays pending.
- `alita:staff:pace:next` and `alita:staff:pace:block` are the fleet-wide Telegram pacing for staff fan-outs: a slot
  reservation plus a shared `retry_after` block. The block keeps Telegram's full `retry_after`; a call whose slot is
  more than `MaxWait` (60 s) away fails at once as rate limited, with no Telegram request and without taking the slot.
  Every staff fan-out Telegram call goes through `staffPaced`, and per-replica limiters must not be used for shared
  budgets.
- `alita:lockdown:pace:next` and `alita:lockdown:pace:block` are the fleet-wide Telegram pacing of lockdown bans,
  unbans, declines and deletes (`lockdownPaced`, `lockdownPacer`): the same slot-plus-block design as the staff pacer,
  with its own keys so a raid and a staff fan-out each keep their own budget. A slot more than `MaxWait` away fails at
  once as rate limited and the joiner row goes back without counting an attempt.
- `staff_actions` and `staff_action_groups` are the staff audit record (migration 20261005120000). They are read only
  through fresh queries, never cached (so no `DeleteCache` applies to them), and never part of
  backup/export/import/reset. A record is created at Confirm and a failed create aborts the card; each group's prior
  state is written before its Telegram write and a failed write fails that group closed. Record writes never use the
  run's context, which a shutdown cancels first.
- `chat_lockdowns` and `chat_lockdown_joiners` (migration 20261006120000) are lockdown state. They are read only through
  fresh queries, never cached (so no `DeleteCache` applies, and a cache added later needs `skipLocal` and a
  `DeleteCache` on every write), and never part of backup/export/import/reset. One active lockdown per chat is the
  `uk_chat_lockdowns_active` partial unique index plus `lockdown.Start`'s `ON CONFLICT DO NOTHING`. `pre_permissions` is
  the raw `permissions` JSON of the `getChat` answer read before the lock call, replayed verbatim with
  `use_independent_chat_permissions`, never re-read and never passed through the typed `gotgbot.ChatPermissions` (its
  `omitempty` bools and `*bool` defaults lose a right that was explicitly off). `locked_at` stays NULL until Telegram
  confirmed the lock, and a lock Telegram refused deletes its unconfirmed row. A lift restores the snapshot first and
  only then records it through the `BeginLift` conditional update, so one lifter wins when two admins lift together; a
  failed restore keeps the lockdown active and unbans nobody, a failed record write does the same, and nothing ever ends
  a lockdown except a lift that Telegram confirmed. `manual_change` is set when the live permissions differ from
  `locked_permissions` at the lift (compared by granted rights, not by bytes, and not at all before the lock was
  confirmed); it never blocks the lift. An unconfirmed row (`locked_at` NULL) not changed for over a minute
  (`lockdownUnconfirmedGrace`) is a `/lockdown` that stopped half way: the worker's cycle (`lockdownSettleUnconfirmed`,
  and `/lockdown` itself when it meets such a row, through `settleUnconfirmedLockdown`) reads the live permissions and
  confirms the row when they equal the locked set, or deletes it otherwise, and never announces either; a failed read
  touches the row (`TouchLockdown`) and retries after another grace period. A younger unconfirmed row is left alone,
  because its own `/lockdown` may still be locking. A lockdown works without Redis: its state is only in PostgreSQL,
  the guard never needs Redis, and the pacer falls back to one replica's interval. Nothing but a confirmed
  `/unlockdown` ever ends a lockdown: no TTL, timer, Redis expiry, restart, worker cycle or expired ban lifts one, and
  `BeginLift` is reached only through `lockdownBeginLift` in `unlockdown`.
- `UpdateRecord` skips zero values. Use `UpdateRecordWithZeroValues` to write `false`/`0`/`""`. Both return
  `gorm.ErrRecordNotFound` when no row matched.
- Check `TableName()` before raw SQL: `ConnectionSettings→connection` (per user), `ConnectionChatSettings→
  connection_settings` (per chat), `AdminSettings→admin`, `DisableSettings→disable`.

## Migrations

- `migrations/*.sql` is the schema source of truth. Never edit an applied file; the SHA-256 checksum aborts startup.
- Name new files with a timestamp greater than every existing one; order is plain filename sort.
- Each file runs in one transaction: no top-level `BEGIN`/`COMMIT`/`ROLLBACK`, no `CREATE INDEX CONCURRENTLY`.
- Change `alita/db/migrations/runner.go` and `scripts/migrate_psql.sh` together; both appliers must agree.

## Adding a module

1. Add a migration in `migrations/` (rules above).
2. Add the model in `alita/db/models/` with `TableName()` returning the migration's table name.
3. Add the repository in `alita/db/<domain>/`: reads through `cache.GetFromCacheOrLoad`, `cache.DeleteCache` on writes.
4. Add the model to the `AutoMigrate` list in the affected `testmain_test.go` or `internal/testdb.Run` call. Do the same
   whenever you change an existing model.
5. Add `alita/modules/<name>.go` with `LoadXxx`, registered in `init()` via `RegisterLegacyModule`.
6. Register commands with `helpers.WrapCommand`; add `RegisterAnonymousAdminHandler` for admin commands.
7. Add locale keys to all 7 files in `locales/`.
8. Run `make generate-docs`, `make check-translations`, `make test`, `make lint`.

## Subsystem traps

- Approved users skip antiflood, locks, blacklists, and captcha.
- Antiflood counters are in-process (per replica). Antiraid is Redis-only and does nothing without Redis.
- Fed-ban lookups cache per `(fed, user)` with a negative sentinel; every ban/unban write must invalidate it.
- A join arrives as both `ChatMemberUpdated` and a service message; dedupe through `claimRecentJoinProcessing`
  (the lockdown guard, which runs first, dedupes by its joiner row instead and never gates a ban on Redis).
- Entity offsets are UTF-16: slice with `extractEntityText`, and match against both `Entities` and `CaptionEntities`.
- Captcha allows one attempt per `(user, chat)`; group `-10` stores the pending user's messages for replay.
- `alita:cache:captcha_pending:<chat>` is a per-chat pending flag; any new code that inserts into `captcha_attempts`
  must `DeleteCache` it after commit.
- Staff Group chat migration is re-keyed by `staff.RekeyChat` from both migrate service messages and the 400
  `migrate_to_chat_id` parameter; other per-chat settings are not migrated. It also re-keys `staff_actions.staff_chat_id`,
  never summary_chat_id or `staff_action_groups.group_chat_id`, and `summary_msg_id` is only valid together with
  `summary_chat_id`.
- Staff links: authority reads use uncached `staff.*Fresh` plus live `chat_status.CheckOwner`; only `OwnerMismatch`
  removes a link (errors are unknown); each automatic removal or health change is one conditional statement, and only
  the caller with RowsAffected == 1 posts the Staff Group notice.
- The staff sweeper (`StartStaffSweeper`/`StopStaffSweeper`) rechecks every Staff Group link hourly (first run 1-5 min
  after start) behind `SETNX alita:staff:sweep:lock`; without Redis it runs unguarded because every staff write is
  conditional. Staff tables are never part of backup/export/import/reset.
- The lockdown join guard (handler group `-7`, `lockdownOnJoinMember`) reads the lockdown fresh from PostgreSQL, records
  the joiner row first and only then ends the update with `ext.EndGroups`, so no welcome, captcha challenge or captcha
  attempt follows. It never gates a ban on Redis, never uses the cached admin predicates (a performer is judged by a live
  `getChatMember`, and `decideLockdownJoin` bans everything but a user a live creator or administrator added), and never
  makes a Telegram write: the worker (`StartLockdownWorker`, a DB-driven loop on every replica that claims rows with
  conditional updates) makes every write and bans with `until_date` = the row's `ban_until`, 330 days after the join,
  the marker that tells the lockdown's own ban from a deliberate one. A failed record write lets the joiner in, still
  muted by the locked default permissions, because an unrecorded ban would never be lifted.
- The lockdown guard handles the other join paths in the same group `-7`: the `new_chat_members` service message
  (`lockdownOnJoinMessage`, registered with `SetAllowBot` because a bot or an anonymous admin can add users; a
  service message's performer is its sender, or the anonymous admin when `sender_chat` is the group or `from` is the
  Group Anonymous Bot) and the kicked update (`lockdownOnKicked`).
  Deduplication is the `(lockdown_id, user_id)` row alone and never Redis (it does not use `claimRecentJoinProcessing`):
  the first path to insert decides, a second delivery within 20 s (`lockdownJoinDedupeWindow`) of the row's last change
  is the same join, and an older finished row (`lockdownReclaimStates`) is a re-join, reclaimed by one conditional
  update with a new `ban_until`. Only a user added by a live creator or administrator, or by an anonymous admin, is
  exempt, and bots never are (D-07, D-08); a failed performer lookup bans. The chat_member path may turn a still
  pending row exempt, because that update shows an admin added or approved the user; once the worker claimed the row
  the ban stands until the lift (D-24). The service message's `NewChatMembers` is filtered in place to the users let in
  and the bot itself, so greetings welcome only them, and the update ends when nobody is left. The join message ID is
  stored only when every user in it was banned, and the worker deletes it after the ban (best effort, no delete right
  just leaves it). A kicked update whose `until_date` matches a joiner row's `ban_until` (2 s) ends handling, so no
  goodbye is sent and captcha rows are left alone for the lockdown's own ban; any other ban still gets the goodbye.
- Join requests during a lockdown (`lockdownOnJoinRequest`, group `-7`, `chat_join_request`) are recorded as a joiner row
  with `join_path` request and declined by the worker (`lockdownDeclineOne`, paced, three attempts; a request Telegram
  reports gone counts as declined), never by the handler. The handler always ends the update, so `pendingJoins` (group 0)
  never auto-approves and posts no approve card, and the request path fails closed: a failed lockdown read or row write
  leaves the request pending instead of handing it on (the join paths fail open, because a joiner is still muted).
  A new request from someone with a finished row is reclaimed at once, with no dedupe window, because Telegram delivers
  each request once; requests still pending at the lift are cancelled. The guard reads the lockdown through
  `lockdownActiveLookup`, a test seam. The greetings Accept button (`joinRequestHandler`, `a=accept`) answers an alert
  (`greetings_join_request_lockdown`) and approves nothing while a lockdown is active or cannot be read; Decline and Ban
  still work.
- A lockdown never touches another group: every lockdown query is keyed by `chat_id` or `lockdown_id`, and the worker
  calls Telegram with the row's own chat, so two locked groups lift in either order without a call or post in the other.
  Antiraid's join handler (`onJoin`, group `-5`) returns at once while a lockdown is active in the chat (until Phase 6
  retires it), so it neither temp-bans a user an admin added nor counts joins toward its auto trigger, and each joiner is
  handled once, by the lockdown; a lockdown it cannot read does not stop it.
- The lockdown lift runs in the same worker after `/unlockdown` restored the permissions. It unbans (`only_if_banned=true`)
  only a joiner whose live `getChatMember` shows kicked with that row's `ban_until` (`isLockdownBan`, 2 s tolerance); anyone
  else, a deliberate `/ban` or `/tban` included, is kept and never touched. Joiners still pending when the lift starts are
  cancelled and never banned. Rows are handled in the order they were recorded, and the tally is posted once, by the
  replica whose `FinishLift` conditional update won; a lockdown with nothing to report posts none.
- StaffActions (staff `/ban` across linked groups): per-group authority is only the live `getChatMember(group, issuer)`
  answer, creator or administrator with `can_restrict_members`, never the cached admin predicates. The link owner is
  rechecked through `recheckLink`. No write ever targets the Staff Group and nothing is posted into a linked group's own
  chat. Each group where an action was applied gets one post in its log channel under the admin category
  (`staff_log.go`), sent through `staffPaced` after the group's result is set, naming the issuer and target and saying
  "via Staff Group" but never the Staff Group's title or ID. A failed post never changes that result. Undos are
  logged the same way, as `#STAFF_UNDO` through the shared `sendStaffLogPost`: one post per group where the undo
  succeeded, sent after that group's undo result is stored, naming the presser (not the original issuer), the target
  and what was undone, never the Staff Group; skipped and failed groups get no post.
- Every staff run goes through `startStaffRun` with a `staffRunSpec`. Its coordinator goroutine is the only writer of
  the run's message, and the per-group hooks (record result, log post) run in the worker after `progress.set`.
- Staff `/ban`, `/mute`, `/kick`, `/unban` and `/unmute` never lift a ban by accident. `restrictChatMember` and
  `unbanChatMember` replace the target's status server-side, so per group the target's live status decides the call:
  restrict goes only to a member or a restricted target (on a kicked target it would replace the ban), the
  member-removing unban (`only_if_banned=false`) is only `/kick` on a current member, and every `/unban` sends
  `only_if_banned=true`. A kicked target skips mute, kick and unmute as "not in group". The decision lives in
  `decideStaffAction` alone; `executeStaffCall` switches only on its verdict. Keep new actions inside that table.
  Undo decisions live in `decideStaffUndo` alone, beside `decideStaffAction`, and `executeStaffUndoCall` switches only
  on its verdict. Its one exception to the never-lift rule, a restrict or ban sent to a kicked or left target, is
  allowed only when the live state equals what the staff action left behind. An undo touches only groups where the
  action was applied, rechecks each one live for the presser (creator, or administrator with `can_restrict_members`,
  through `staffGroupPrechecks` with the presser as actor), and its result is a new summary replying to the original.
- The Undo button exists only on the final summary of a finished, finalized record, for ban, mute, unban and unmute
  with at least one applied group (`/tban` and `/tmute` are recorded as ban and mute). When the final edit falls back to
  a new message the button goes on it and `summary_msg_id` follows. The original summary is edited ("Undone by") and
  replied to only when the record's `summary_chat_id` is the chat the press came from: message IDs belong to their
  chat, so after a Staff Group migration the card is posted without a reply and nothing old is edited. That edit is
  made at the end of the undo run, after the undo's own summary is delivered and on its own budget, not at Confirm.
  The original reads "Undone by" only when at least one group was undone and "changed nothing" when writes were tried
  and none succeeded, and loses its Undo button in both cases; it is left alone with its Undo button when the claim
  was given back, and the undo's own summary then says no group was changed and the action can still be undone.
- Staff targets are the first argument and never guessed: a numeric ID, a `text_mention` entity starting exactly at that
  UTF-16 offset, or an `@username` (4-32 of `A-Za-z0-9_`). A username resolves through `user.FindUsersByUsername` (users
  table only, case-insensitive, up to 10 rows, newest activity first) behind the `staffUserLookup` seam: no row is
  "never seen", more than one is refused with each name, ID and last-seen date, and a database error is "could not
  check". Never call `extraction.GetUserId` or the extract-user helpers for staff targets: they are case-sensitive and
  fall back to the channels table and live `getChat`. A reply is never a target (a bare reply gets a hint; a reply plus
  an explicit target uses the explicit one). `/sban`, `/dban`, `/skick`, `/dkick`, `/smute` and `/dmute` are
  intercepted in a Staff Group only to be refused with a hint (`staffCommandSpec.Refused`), after the anonymous check.
- A staff action card has one writer once its run starts: the run's coordinator goroutine, which edits at most every
  2.5 s (`staffActionEditEvery`), only when a result changed, and never before a progress edit's 429 `retry_after` has
  passed. The summary never drops a group: over 3800 UTF-16 units (counted on the escaped HTML) only done lines
  collapse into a count, skipped and failed lines are always listed, and overflow goes to continuation messages in the
  Staff Group. The final edit waits out that hold and is retried after `retry_after`, and a card that cannot be edited
  gets the summary as a new message. The final edit, the fallback and each continuation message each get their own
  budget (`staffActionDeliverPartTimeout`, 3 x (60 s cap + 10 s edit timeout)), so one long wait never starves the
  next message.
- Staff durations: `/ban` and `/mute` take an optional `<digits><m|h|d|w>` token right after the target, `/tban` and
  `/tmute` require it, and the grammar is `extraction.ParseDurationToken`, shared with the per-group commands. Longer
  than 366 days means permanent on the card and in the call, never a clamped value. The card stores the duration, not
  an end date: `until_date` is computed once at Confirm and that one value goes to every group.

## Go rules

- Never discard a DB error on a state-changing path.
- Every fire-and-forget goroutine starts with `defer error_handling.RecoverFromPanic(...)`. Async user/chat writers
  join the WaitGroup drained by `DrainUsersAsyncWrites`.
- Register every new secret with `logredact.RegisterSecret` (≥6 chars), or it is logged verbatim.
- A missing i18n key returns `""` with an error callers ignore; a typo'd key in Go ships an empty message. Copy key
  names from `locales/en.yml`. `locales/config.yml` is the help alt-name pseudo-locale, not a language.
- The docs generator reads only `locales/en.yml`. Pages marked `<!-- MANUALLY MAINTAINED: do not regenerate -->` in
  their first 512 bytes are frozen; `api-reference/lock-types.md` ignores the marker.
- Imports: stdlib, third-party, internal, separated by blank lines. Use `helpers.Ptr[T]` for option pointers.

## Toolchain

- Production builds use `CGO_ENABLED=0`; tests need `CGO_ENABLED=1` (go-sqlite3). Do not unify them.
- Go 1.26.0 with no `toolchain` directive. Keep dependencies on the latest upstream versions that pass required
  checks. Any intentional pin must document its compatibility reason and recheck trigger.
- Version strings are shape-locked (`BotVersion:` with two spaces in `alita/config/config.go`, `version =` in
  `main.go`). Run `make bump-version` on a PR branch, merge, then tag. Tags match `^v\d+\.\d+\.\d+(-[0-9A-Za-z.]+)?$`.
- `.env` in cwd auto-loads without overriding real env. Run `--version` smoke checks from `/tmp`.
- Defaults that surprise: `REDIS_DB=1`; `ENABLE_AISPAM=true` (inert without `TYPESAFE_API_KEY`); perf monitoring and
  background stats are on only when `DEBUG=false`; `HTTP_PORT` falls back to `PORT` then 8080. Integer env parsing is
  lenient, so a typo silently becomes the default.

## Testing

- Always pass `-tags testtools`; without it, test files and the helpers in production packages that carry that build tag drop out silently.
- Postgres tests need both `DATABASE_URL` and `ALITA_TEST_DATABASE=true`; otherwise they run on SQLite.
- Use real fixtures: `internal/testdb.Run` (SQLite), miniredis, hand-written `gotgbot.BotClient` fakes. No mock libraries.
- Assert observable behavior (reply sent, row persisted, cache invalidated, gate enforced). Never assert literals,
  source substrings, or test-double internals.
- The three harness `AutoMigrate` lists (`alita/modules/test_harness_test.go`, `alita/db/staff/testmain_test.go`,
  `alita/db/testmain_test.go`) include the staff audit models, and `staffCleanup` deletes audit rows. The same three
  lists plus `alita/db/lockdown/testmain_test.go` include the lockdown models, and `lockdownCleanup` deletes lockdown rows.
- In CI, keep the migration-chain step before `make test`; its `schema_migrations` rows back the checksum test.

## Commits

- Use Conventional Commits. The changelog drops `docs:`/`test:`/`chore:`/`ci:`/`deps:`, so user-visible changes need
  `feat:` or `fix:`.
