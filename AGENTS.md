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
  shutdown cut every group off before its write.
- Deploy manifests set `AUTO_MIGRATE=true`; the code default is `false`. Never call `gorm.AutoMigrate` in production code.

## Handlers

- Group numbers are execution order: `-10` captcha sweeper · `-6` fed-ban · `-5` antiraid ·
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
  any lookup, and another member's tap on an undo card gets the presser-only alert.
- The history detail view is `a=dt&r=<record id>&o=<offset>` (`r` is the `staff_actions` row ID, `o` the list offset
  Back returns to). The record is loaded fresh and refused unless its `staff_chat_id` is the pressed message's chat, so a
  forged or replayed button naming another Staff Group's record shows nothing. A zero, negative, non-numeric or
  over-63-bit ID is answered as expired before any read. The view reads only the audit record and the current link list
  and makes no Telegram call to linked groups; an unfinished record's stale pending groups are shown as interrupted
  without writing anything. While `staffActionUndoable` holds it also shows the same `a=ya&r=<record id>` Undo button as
  the summary, above Back; that press goes through `staffUndoAsk` like any other, so every undo check runs again and the
  button grants nothing by itself. Once an undo is claimed the button is gone and the view shows who undid it and each
  group's undo result.

## Permissions

- `alita/utils/chat_status` predicates return bools and never reply; use `PermissionResponder` to message.
  `helpers.CheckFunc` replies and is valid only inside `WrapCommand`.
- `IsUserAdmin` returns false for channel and non-positive IDs. Never pass a chat ID as a user ID.
- `*ForUpdate` predicates are memoised per update. Watchers only, never after a state change in the same update.

## Data

- Repositories live in `alita/db/<domain>/`. Read via `cache.GetFromCacheOrLoad` with `cache.CacheKey` keys.
  **Every write must `cache.DeleteCache` the keys it affects.**
- `GetFromCacheOrLoad` has an in-process layer (`CACHE_LOCAL_TTL`, default 10 s) in front of Redis. `DeleteCache`
  evicts it on this replica only; list freshness-critical keys in `skipLocal` (`alita/db/cache/local.go`). Tests that
  write rows directly must call `cache.ResetLocalForTest()` or `DeleteCache` before reading through the cache.
- Two packages are named `cache`; the loader and generation guards are in `alita/db/cache`, not `alita/utils/cache`.
- Operational Redis keys (`alita:antiraid:*`, `alita:anonAdmin:*`, `alita:staff:*`) sit outside the `alita:cache:` prefix;
  `CLEAR_CACHE_ON_STARTUP` does not clear them.
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
- `staff_actions` and `staff_action_groups` are the staff audit record (migration 20261005120000). They are read only
  through fresh queries, never cached (so no `DeleteCache` applies to them), and never part of
  backup/export/import/reset. A record is created at Confirm and a failed create aborts the card; each group's prior
  state is written before its Telegram write and a failed write fails that group closed. Record writes never use the
  run's context, which a shutdown cancels first.
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
- A join arrives as both `ChatMemberUpdated` and a service message; dedupe through `claimRecentJoinProcessing`.
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
  `alita/db/testmain_test.go`) include the staff audit models, and `staffCleanup` deletes audit rows.
- In CI, keep the migration-chain step before `make test`; its `schema_migrations` rows back the checksum test.

## Commits

- Use Conventional Commits. The changelog drops `docs:`/`test:`/`chore:`/`ci:`/`deps:`, so user-visible changes need
  `feat:` or `fix:`.
