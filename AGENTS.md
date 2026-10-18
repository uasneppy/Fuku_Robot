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
- Shutdown runs LIFO within 60 s. Register new drains *after* DB-close so they run before it. Each handler gets 10 s
  (`defaultHandlerTimeout`) unless registered with `RegisterHandlerWithTimeout`; a drain whose own wait is longer is
  registered with that wait plus `shutdownDrainGrace` (1 s), so its own timer ends it and the DB close never runs under
  it. The 60 s budget still bounds everything and exits with code 1 wherever it runs out, so deploy manifests must allow
  more than 60 s between SIGTERM and SIGKILL (`stop_grace_period: 75s` in `docker-compose.yml`).
- `StopStaffActions` is one of those drains, registered through `RegisterStaffActionsDrain` (`staffActionStopWait` plus
  grace). In webhook mode the drains before it (HTTP stop 10 s, AI spam 10 s, lockdown 6 s) still leave its 31 s inside
  the 60 s; in polling mode the updater's extra 10 s can let the global budget cut it short. It cancels running staff fan-outs, marks every unfinished group
  "interrupted by restart", delivers the final summary and returns within 30 s. A delivery still waiting out a 429
  when the 30 s are up is cut off with the process. A hard crash can still leave ⏳ lines. A run cancelled by the
  shutdown skips its remaining log posts, so a group applied just before it may have no post. Undo runs join the
  same drain: their final record write (`FinalizeUndo`, or `ReleaseUndo` when no group reached its write, both through
  `db.DB`, never the cancelled run context) and summary delivery happen inside the same 30 s as a staff action's, and an
  unfinished group is recorded `fail_interrupted`. An undo Confirm joins `staffActionRunsWG` (`joinStaffRuns`) before it
  claims, so `StopStaffActions` waits for a Confirm that has already claimed, and its run gives the claim back when the
  shutdown cut every group off before its write. Once `StopStaffActions` has cancelled the run context an undo Confirm
  claims nothing and aborts its card with the restart text (`staff_undo_abort_restarting`).
- `StopLockdownWorker` is another drain registered after DB-close, through `RegisterLockdownWorkerDrain`
  (`lockdownWorkerStopWait` plus grace). It cancels the lockdown worker and waits at most
  5 s (`lockdownWorkerStopWait`), because every lockdown row resumes from the database: a cycle cut off at shutdown
  loses nothing, and a restart or a second replica picks the rows up again. A claim (an acting or unbanning joiner row
  with `claimed_at`) older than 2 minutes (`lockdownStaleClaim`) belongs to a worker that stopped: the first cycle of any
  replica releases it through `lockdown.ReleaseStaleClaims`, before any other step and without counting an attempt.
  Acting goes back to pending in an active lockdown, to banned (a ban) or cancelled (a join request) in a lifting one,
  and unbanning goes back to banned, so a restart resumes every ban, decline and unban and the live check at the lift
  keeps a repeated unban safe.
- Deploy manifests set `AUTO_MIGRATE=true`; the code default is `false`. Never call `gorm.AutoMigrate` in production code.

## Handlers

- Group numbers are execution order: `-10` captcha sweeper · `-7` lockdown join guard (it also counts the join surge
  before it reads the lockdown) · `-6` fed-ban · `-4` raid flood counter (counts, never ends an update; there is no
  antiraid handler any more) · `-3` staff watchers (chat-migration re-key; later ownership and bot health) · `-2` admin-cache ·
  `-1` users tracker · `0` commands/help/greetings · `3` aispam · `4` antiflood · `5`/`6` locks · `7` blacklists ·
  `8` reports+reactions · `9` filters · `10` pins · `11` log-channel.
- gotgbot stops a group at the first matching handler. Watchers return `ext.ContinueGroups`; commands return
  `ext.EndGroups`. A group-0 watcher returning `nil` silently kills every later group-0 handler.
- Modules register in `init()` via `RegisterLegacyModule(name, priority, load)`. Use a unique name; duplicates are
  dropped silently. Priority sets load order and group-0 precedence; `LoadHelp` loads last.
- New commands use `helpers.WrapCommand(dispatcher, CommandDescriptor{...}, handler)`. Set `Disableable: true` for any
  command chats may disable. Do not add replies for failed checks; the pipeline sends them.
- Anonymous admins bypass `WrapCommand`. Admin commands that must work for them also need
  `RegisterAnonymousAdminHandler` + `anonPipelineHandler`. After the proof tap, `verifyAnonymousAdmin` swaps the callback
  update for a message update carrying the cached command and points `EffectiveChat` at that command's chat, so the
  re-entered command, the `RequiredChecks` that `anonPipelineHandler` re-runs, and `PermissionResponder` see the
  command's chat as if the admin had sent it, with the tapper as the sender. `verifyAnonymousAdmin` refuses the tap as
  an invalid request, deleting and running nothing, unless the callback's `c`, the proof button's chat and the cached
  command's chat are the same chat.
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
- The `lockdown|v1` namespace is the lockdown alert's buttons (`lockdownAlertCallback`, registered with
  `callbackquery.Prefix("lockdown|")` and `SetAllowChannel(true)`: gotgbot drops a callback from a channel message
  otherwise, and the buttons on the log channel copy would be dead). Field `r` is the alert copy row's ID: the handler
  loads that row and its lockdown fresh and takes the locked chat from the lockdown row, never from the chat the button
  sits in or from the callback data. Every press is checked in this order, each step refusing before the next runs: a
  Telegram service identity (`staffServiceUserIDs`) is answered "post as yourself" (`lockdown_alert_post_as_yourself`)
  before any read; `r` must parse (`lockdownParseCopyID`, 1 up to the largest 63-bit value) and name an existing copy,
  else "no longer valid"; the copy row is honoured only on the chat it was posted in (`dest_chat_id` equals the pressed
  message's chat), else "no longer valid" with no lockdown read and no presser lookup; the lockdown must exist; a
  lockdown that is not active makes the press stale (below); only then does the action run. Every path answers the
  callback query exactly once.
- Actions: Lift is `a=lk&r=..` (first row of the alert), Ban recent joiners is `a=bn&r=..` (second row; the label
  carries N, or a generic label when N is 0 or unreadable) and Revoke link is `a=rk&r=..` (third row). Each ask posts a
  confirm card that replies to the pressed message, with Confirm `a=lc`, `a=bc` or `a=rc` (`&r=..&u=<asker>&e=<expiry>`)
  and the shared Cancel `a=x&r=..&u=<asker>`. The cards are stateless: the asker `u` and the expiry minute `e` (unix
  minutes, five minutes ahead, `lockdownCardLifetime`) travel in the data, so they work without Redis, and only the
  asker may confirm or cancel (another user is answered "not yours" and nothing is edited). `lockdownParseAsker` and
  `lockdownParseCardFields` accept only positive 63-bit integers for `u` and `e`: a Confirm with a bad field is answered
  "no longer valid" before any presser lookup, and Cancel needs only a valid `u`. Expiry is lazy, with no timer and no
  Redis key: a Confirm whose `e` is before the current unix minute edits the card to `lockdown_alert_card_expired` and
  lifts, bans or revokes nothing. Cards are edited to a final text and never deleted.
- Authority is a live `getChatMember` of the locked chat, at the ask and again at Confirm; never the admin cache, never
  membership of the Staff Group and never the chat the button sits in, and a failed lookup refuses
  (`lockdown_alert_check_failed`). Lift and Ban need the creator or an administrator with `can_restrict_members`
  (`lockdownButtonAuthority`): an administrator without that right is answered `lockdown_alert_no_restrict`, anyone
  else `lockdown_alert_not_admin`. Revoke needs the creator or an administrator with `can_invite_users`
  (`lockdownInviteAuthority`); the restrict right is not enough.
- A press on a button of a lockdown that is not active (lifting or lifted) is judged before any action
  (`lockdownStalePress`): it answers "Already lifted by X at T", changes nothing, posts no card, and closes that copy at
  once (`lockdownCloseSlotNow`, or edits a confirm card to the same text). The copy row names its own lockdown, so the
  handler never looks at the chat's current lockdown and an old alert's button cannot reach a newer lockdown of the
  same group.
- Revoke link is shown only while the copy's stored link is one the bot can revoke (`lockdownLinkRevocable`: a link in
  full, so one the bot made) and no revoke was recorded. The link revoked is always the one stored on the pressed copy,
  never anything from the callback data. Confirm calls `revokeChatInviteLink` through `lockdownPaced` first and records
  the revoke (`lockdown.RecordLinkRevoked`) only after Telegram confirmed it; a refused revoke edits the card with a
  plain reason (`lockdownRevokeRefusalKey`: busy, no rights, link not found, or a generic line; only descriptions that
  name the cause are mapped, so a bare 403 or "chat not found" gets the generic line; Telegram's own text goes to the log
  only) and records nothing, except that a refusal met after another admin's revoke was recorded (the lockdown is read
  again) shows `lockdown_revoke_already` instead.

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
  registered with `RegisterAnonymousAdminHandler` for that re-entry. Their own `requireLockdownGroup` and
  `lockdownRefuse` read `c.Chat` and reply through `c.Msg`; they predate the re-entry fix described under Handlers and
  stay.
- `/autolockdown` (`autolockdownApply`) shows or changes the automatic lockdown triggers. Its authority is the same live
  `getChatMember` check, chosen by whether arguments follow (`requireAutoLockdownAuthority`): with none, any live creator
  or administrator may view; with any, only the creator or an administrator with `can_restrict_members` may change, and
  a refusal, a bad argument, a basic group or a failed save writes nothing. The grammar is exactly `joins|flood` then
  `on`, `off` or a whole number 3..500 (a number sets the threshold and turns the trigger on; `on` and `off` keep it).
  Anonymous admins always get the proof first and re-enter through `RegisterAnonymousAdminHandler`; the cached command's
  arguments then pick view or change for the tapper. The command is not `Disableable`. `/autoantiraid` is its alias for
  the join threshold (see the retired raid mode under Subsystem traps) with exactly the same checks and anonymous-admin
  re-entry, so neither is ever decided by the cached admin list.

## Data

- Repositories live in `alita/db/<domain>/`. Read via `cache.GetFromCacheOrLoad` with `cache.CacheKey` keys.
  **Every write must `cache.DeleteCache` the keys it affects.**
- `GetFromCacheOrLoad` has an in-process layer (`CACHE_LOCAL_TTL`, default 10 s) in front of Redis. `DeleteCache`
  evicts it on this replica only; list freshness-critical keys in `skipLocal` (`alita/db/cache/local.go`). Tests that
  write rows directly must call `cache.ResetLocalForTest()` or `DeleteCache` before reading through the cache.
- Two packages are named `cache`; the loader and generation guards are in `alita/db/cache`, not `alita/utils/cache`.
- Operational Redis keys (`alita:anonAdmin:*`, `alita:staff:*`, `alita:lockdown:*`, `alita:raid:*`) sit outside the
  `alita:cache:` prefix; `CLEAR_CACHE_ON_STARTUP` does not clear them. `alita:antiraid:*` is retired: nothing reads or
  writes it, and `sweepRetiredAntiRaidKeys` removes what the old raid mode left, once per process at startup.
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
- `alita:lockdown:recent:<chat>` is the best-effort 10-minute join tracker (`lockdown_recent.go`): a sorted set scored by
  join time in unix milliseconds, written by both group `-7` join handlers (`lockdownOnJoinMember`,
  `lockdownOnJoinMessage`) for every supergroup join before they read the lockdown, whether or not one is active. Join
  requests and the bot's own join are not tracked. A member is compact JSON (`i` id, `n` name, `u` username, `b` bot,
  `l` invite link; the chat_member path carries the link, the service message does not), so the two deliveries of one
  join are merged by user ID at read time (`readLockdownRecentJoins`, behind the `lockdownRecentLookup` seam: earliest
  time wins, a non-empty link, name or username fills in). One pipelined round trip under 250 ms
  (`lockdownRecentTimeout`) adds, trims to 10 minutes, keeps the newest 500 (`lockdownRecentCap`) and sets a 700 s TTL.
  It is empty without Redis and is never a ban decision. The joins of the 10 minutes before the lock are saved with the
  lockdown's alert (`chat_lockdown_burst`); a flood lockdown also saves its flooders there (`alita:raid:{<chat>}:flooders`
  below). The raid join counter does not use it.
- `alita:raid:{<chat>}:joined` is the join counter of automatic raid detection (`raid_counters.go`): a sorted set whose
  member is the user ID and whose score is that account's latest join in unix milliseconds, so the `chat_member` update
  and the service message of one join are one member and a re-join refreshes the account. One Lua script
  (`raidJoinScript`, the chat ID is a hash tag) adds the joins, trims what is older than 24 h, keeps the newest 5000
  (`raidJoinedCap`), sets a 25 h TTL and returns how many different accounts joined in the last 60 s. It is best effort
  and Redis only: without Redis automatic detection is inert on that replica (one info log per process) and `/lockdown`
  still works. `alita:raid:{<chat>}:fire` is a `SET NX EX 60` latch that only sheds load while a trigger starts a
  lockdown; exactly one lockdown is still `uk_chat_lockdowns_active` plus `lockdown.Start`, and one alert still
  `lockdown.BeginAlert`, however many triggers and replicas fire at once. After the latch `raidFire` reads the chat's
  active lockdown fresh and stays silent, with no Telegram call, while one exists, and for 10 minutes (`raidLiftQuiet`)
  after `lockdown.RecentlyLiftedFresh` finds a lift of the chat started (`lift_started_at` in PostgreSQL, so a Redis
  flush does not end it, and real time); an unreadable answer shortens the latch to 10 s. The latch then lasts 60 s
  after a started, already-active or unknown-lock outcome, 10 s (`raidRetryLatch`) after a failed bot-rights check,
  unreadable permissions or failed state write, and 10 minutes (`raidCantLockLatch`) when the group is not a
  supergroup, the bot cannot restrict members or Telegram refused the lock. Do not reuse
  `alita:lockdown:recent:<chat>` for counting: its JSON members count one join twice.
- `alita:raid:{<chat>}:fi`, `:fn` and `:fa` are the message flood counter of automatic raid detection
  (`raid_flood.go`): three sorted sets scored in unix ms, the window's item keys (`m:<message id>`, or `g:<media group
  id>` so an album is one item), the item keys sent by new members and those members' user IDs. One Lua script
  (`raidFloodScript`) adds the item, trims all three to 30 s, caps the first two at 2000 (`raidFloodCap`), sets
  `PEXPIRE` 2 minutes on each and returns their sizes; a member is new when `:joined` scores them within 24 h
  (`ZSCORE`). It is Redis only, like the join counter.
- `alita:raid:{<chat>}:flooders` is the flood alert's snapshot (`raid_flood.go`, D-17). When a flood trigger fires,
  `raidFire` calls `raidSnapshotFlooders` after the latch, the active check and the quiet period and before
  `startLockdownRow`: one Lua script (`raidFloodersScript`) replaces the key with the newest accounts of `:fa` (at most
  `raidFloodersMax` = 500) whose `:joined` score is within 24 h, scored by that join time, with a 15-minute TTL
  (`raidFloodersTTL`). A failed snapshot is logged at debug and the lockdown still starts. `lockdownBurstSnapshot` reads
  it for a lockdown whose `trigger_kind` is `flood` only (`raidFloodersLookup`, a test seam; no Redis or no snapshot adds
  nothing) and `lockdownMergeFlooders` merges it with the 10-minute tracker's joins by user ID (the tracker's name,
  username, link and bot flag win; the earliest time wins), names the rest from the users table with one
  `user.FindUsersByIDs` query (a failed read leaves them unnamed but still saved) and skips the bot. They are saved as
  `pre` burst rows by `lockdown.BeginAlert`'s winner, so a flooder who joined hours before the lock is on the alert's
  sample and in "Ban N recent joiners" (through the unchanged Phase 5 ban run and its exclusions), and a Redis flush
  after the alert changes nothing. A flood alert's counts line is `lockdown_alert_counts_flood`
  (`lockdownRenderSample` takes the trigger kind); the join-surge and manual alerts keep `lockdown_alert_counts`.
- Automatic lockdowns are announced in `raid_trigger.go`, every post through `lockdownPaced` under `raidPostTimeout`
  (30 s), and a failed post is only logged: it never changes the lockdown or its absence. The call of `raidFire` that
  got `lockdownStartStarted` posts one public notice in the locked group (`raidPostNotice`, the group's language: the
  trigger and its numbers, `lockdown_note_approved_muted`, and the no-delete and no-invite notes `/lockdown` adds),
  and no other outcome does. When the bot cannot lock a group (`lockdownStartNotSupergroup`,
  `lockdownStartBotCannotRestrict` or a definitively refused lock, `lockdownStartLockFailed`) nothing is started and
  `raidWarnCannotLock` warns the group's Staff Group (`staff.GetLinkOfGroupFresh`, the Staff Group's language) and its
  log channel (`actionlog.Destination` under the admin category, the group's language, starting with the usual header),
  each only when it exists. One `SET NX EX 86400` on `alita:raid:{<chat>}:warn` gates both copies, so a group is
  warned at most once a day across replicas, and a Redis error or a lost `SetNX` sends nothing. The other outcomes
  (bot rights or permissions unreadable, a failed state write, a lock whose answer was lost) only log. The group title
  is escaped and spliced in after translation; the trigger sentence (`lockdownTriggerSentence`) holds only numbers.
- `chat_raid_detection` (migration 20261018130000) holds one chat's automatic raid detection settings: the join surge
  and flood switches and thresholds (`CHECK` 3..500, defaults 15 and 30). A missing row means the defaults, both triggers
  on (`raiddetection.Defaults`); the Go model carries no `default:` tag on its booleans, because GORM's Create would
  turn a Go `false` back into the SQL default. It is read through `cache.GetFromCacheOrLoad` under
  `cache.CacheKey("raid_detection", chat)` (`raiddetection.GetSettings`), every write must `cache.DeleteCache` that key,
  and it is never part of backup/export/import/reset. The migration carries every non-zero
  `antiraid_settings.auto_antiraid_threshold` over as the join threshold, clamped by `LEAST(GREATEST(x, 3), 500)`: a
  value below 3 becomes 3 (D-18) and the old `/autoantiraid` values above 500 become 500 (D-22), so the copy satisfies
  the `CHECK` and cannot abort startup. The key `raid_detection:*` is listed in `skipLocal` (D-12), so another replica
  never serves it from the in-process layer after a change. Writes are four column-targeted upserts, one per trigger
  and kind (`SetJoinEnabled`, `SetJoinThreshold`, `SetFloodEnabled`, `SetFloodThreshold`; a threshold also turns its
  trigger on), each `INSERT ... ON CONFLICT (chat_id) DO UPDATE` of only its own columns plus `updated_at` followed by
  `cache.DeleteCache`, so two admins changing the two triggers never clobber each other, a Go `false` is always written,
  and a threshold outside 3..500 returns `ErrThresholdOutOfRange` before any write. The developer `/stats` counts the
  rows through `raiddetection.LoadRaidDetectionStats` (chats with a row, and the chats with each trigger off).
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
  confirmed the lock, and a lock Telegram refused with a 4xx answer (`lockdownDefinitiveRefusal`) deletes its unconfirmed
  row. Any other failure of the lock call (a timeout, a dropped connection, an unreadable answer, a 5xx) may have taken
  effect, so `/lockdown` keeps the row, which holds the only copy of the permissions to restore, and answers
  `lockdown_lock_unknown`; the worker settles that row from the live permissions like any half-started lockdown, and
  `/unlockdown` can restore from it at once. A lift restores the snapshot first and
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
  `BeginLift` is reached only through `lockdownBeginLift` in `liftLockdownRow`, the one lift path shared by `/unlockdown`
  and the alert's Lift Confirm (restore the stored permissions, then one conditional record, then `FinishLift`). The
  button path also posts the public `lockdown_lifted` notice in the locked group, through `lockdownPaced`. The
  `chat_lockdowns` columns `trigger_count`, `trigger_window` (seconds) and `trigger_percent` (migration 20261018120000,
  with `idx_chat_lockdowns_chat_lift` on `(chat_id, lift_started_at)`) record the numbers behind an automatic trigger:
  `trigger_kind` is `manual`, `join_surge` or `flood`, and all three numbers are 0 for a manual lockdown. The numbers are
  columns and never part of `reason`, so each destination renders them in its own language
  (`lockdownAlertTriggerLine`).
- `chat_lockdown_alerts` and the `chat_lockdowns` columns `chat_title` and `alert_started_at` (migration
  20261011120000) are the lockdown alerts. One row per posted copy, an alert or a reminder (`kind`), unique on
  `(lockdown_id, kind, seq, dest)`; `message_id` and `body` are the exact text posted, so later edits start from it,
  and `link` is the invite link the copy shows, which its Revoke button revokes. The claim (`claimed_at`, `attempts`,
  `detail`), `buttons_live`, `edit_pending` and `closed_at` columns carry the delivery, refresh and close state
  described under Subsystem traps. The rows are read only through fresh queries, never cached (so no `DeleteCache`
  applies), and never part of backup/export/import/reset. `chat_title` is stored at `lockdown.Start` (empty before this
  migration, shown as the chat ID).
- `chat_lockdown_burst` (migration 20261011130000) holds one row per account of a lockdown's burst, unique on
  `(lockdown_id, user_id)`. It is read fresh, never cached (so no `DeleteCache` applies), and never part of
  backup/export/import/reset. The tracker's pre-lockdown snapshot (source `pre`, the joins in
  `(locked_at - lockdownRecentWindow, locked_at]` read through `lockdownRecentLookup`) is saved there by
  `lockdown.BeginAlert`, in the transaction that decides the alert (`AlertPlan.Burst`, `ON CONFLICT DO NOTHING`, only
  for the winner), so the alert, the Ban button and the reminders never need Redis afterwards. A failed tracker read or
  no Redis saves nothing and the alert still posts (before count 0). The alert lists up to `lockdownAlertSampleMax` (10)
  accounts through `lockdown.BurstSampleFresh`: the pre rows by join time, then the lockdown's joiner rows whose user
  has no pre row (any state), with the counts behind them; names, usernames and IDs are escaped and spliced in after the
  translator. The response columns (`state`, `reason`, `detail`, `run_id`, `attempts`, `claimed_at`) are moved by the
  ban run, not by the alert.
- `chat_lockdown_ban_runs` (migration 20261011140000) is one row per "Ban N recent joiners" run. One running run per
  lockdown is the `uk_chat_lockdown_ban_runs_running` partial unique index plus `lockdown.StartBanRun`'s
  `ON CONFLICT DO NOTHING` (a refused insert writes nothing). It is read fresh, never cached (so no `DeleteCache`
  applies), and never part of backup/export/import/reset. A run queues only unhandled burst rows (state `''`) that are
  not approved users or the bot (the caller passes them as `exclude`), copies the lockdown's joiner rows into the burst
  first, and marks the unhandled rows of joiners recorded exempt skipped `let_in`; rows an earlier run handled are never
  touched. A run with nothing to queue is done at once, `FinishBanRun` finishes a run exactly once (done only while no
  row of the run is queued or acting), and `ReleaseStaleBurstClaims` returns a dead worker's acting rows to the queue.
  The columns `report_card_done_at`, `report_log_done_at`, `reported_at`, `report_claimed_at` and `report_attempts`
  (migration 20261011160000) carry the delivery of a finished run's result; a run with nothing to queue is stored
  already reported, and runs that finished before the migration were marked reported by it.
- `chat_lockdowns.link_revoked_at`, `link_revoked_by`, `link_revoked_by_name` and `link_revoked_link` (migration
  20261011150000) record the one revoke of the raid's invite link a lockdown can have. `lockdown.RecordLinkRevoked` is a
  conditional update (`link_revoked_at IS NULL`), so one confirm wins, and in the same transaction it marks every posted,
  open copy with live buttons `edit_pending`. The columns are read fresh, never cached, and never part of
  backup/export/import/reset.
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
- Antiflood counters are in-process (per replica). The raid detection counters (`alita:raid:*`) are Redis-only: without
  Redis the automatic triggers do nothing, and `/lockdown` still works.
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
- The `/staff` panel marks a linked group whose lockdown is confirmed with one `staff_panel_row_lockdown` line (the
  confirmation time in UTC, `staffLinkRow.LockedSince`), also when the bot has left that group. `buildStaffPanel` loads
  every locked group with one `lockdown.ListActiveByChatsFresh` call after `buildStaffPanelRows`, so `renderStaffPanel`
  stays pure; an unconfirmed lockdown, a lift in progress and a lifted one show nothing, a failed query shows the panel
  unmarked, and the row never carries the reason or who started it (they stay in the group's own `/lockdownstatus`).
- The staff sweeper (`StartStaffSweeper`/`StopStaffSweeper`) rechecks every Staff Group link hourly (first run 1-5 min
  after start) behind `SETNX alita:staff:sweep:lock`; without Redis it runs unguarded because every staff write is
  conditional. Staff tables are never part of backup/export/import/reset.
- The lockdown join guard (handler group `-7`, `lockdownOnJoinMember`) reads the lockdown fresh from PostgreSQL, records
  the joiner row first and only then ends the update with `ext.EndGroups`, so no welcome, captcha challenge or captcha
  attempt follows. It acts only on a confirmed lockdown (`locked_at` set); an unconfirmed row is not a lockdown yet and
  every join path lets the joiner through. Its write to the `alita:lockdown:recent:<chat>` tracker comes first, is best
  effort and its result is ignored. It never gates a ban on Redis, never uses the cached admin predicates (a
  performer is judged by a live `getChatMember`, and `decideLockdownJoin` bans everything but a user a live creator or
  administrator, or an anonymous admin, added), and never makes a Telegram write: the worker (`StartLockdownWorker`, a
  DB-driven loop on every replica that claims rows with conditional updates) makes every write and bans with
  `until_date` = the row's `ban_until`, 330 days after the join, the marker that tells the lockdown's own ban from a
  deliberate one. Before every ban `lockdownBanOne` reads the joiner's live `getChatMember` in the same paced unit: a
  joiner already kicked on another end date (an admin's or a staff `/ban` placed after the guard recorded them) is
  moved to `kept` with no ban call, because `banChatMember` on a kicked user would replace that ban's end date with the
  marker and the lift would then remove it; one kicked on the row's own `ban_until` is the lockdown's ban whose answer
  was lost and is recorded `banned`; an unreadable status is a failed call and never a ban. A failed record write lets the joiner in, still muted by the locked default permissions, because an
  unrecorded ban would never be lifted.
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
- `startLockdownRow` (`lockdown_start.go`) is the one start path, shared by `/lockdown` and every automatic trigger, so a
  trigger's lockdown is exactly a `/lockdown`: the live bot-rights check, the raw permissions snapshot, the insert behind
  `uk_chat_lockdowns_active`, the lock call, the confirmation and the worker wake. Of two simultaneous callers for one
  chat exactly one gets `lockdownStartStarted` and the other `lockdownStartAlreadyActive` with the running row. It judges
  no person's authority (the caller has) and `/lockdown` maps each outcome to the reply it always sent.
- The join surge trigger is counted by both group `-7` join guards, `lockdownOnJoinMember` through
  `raidCountMemberJoin` and `lockdownOnJoinMessage` (the `new_chat_members` service message) through
  `raidCountMessageJoins`, right after the lockdown join tracker write and before the lockdown is read, so the join
  that tips the threshold is handled by the lockdown it starts. The service message's accounts go to Redis in one
  script call, and the account is the set member, so the two deliveries of one join count once. Both counters and both
  guard handlers build their `lockdownJoinInput` with `lockdownMemberJoinInput` and `lockdownMessageJoinInputs`, so
  the performer rule cannot drift. It is best effort and Redis only, and a count never bans,
  restricts or deletes anything: when `raidJoinCounts` holds (not the bot, not a request, not an approved user, not
  added by the anonymous admin or by a live creator or administrator) and the chat's different accounts joined within
  60 s reach its threshold, `raidFire` starts the lockdown through `startLockdownRow` exactly as `/lockdown` does, with
  `trigger_kind` `join_surge`, the bot as starter, an empty `reason` and the numbers in the trigger columns. The
  trigger runs synchronously in the handler (no goroutine, no drain); a crash half way leaves an unconfirmed row the
  worker settles. The counter shares its one live `getChatMember` of the performer with the guard (it returns the
  status it read and whether it asked), so a join or a join message still makes at most one performer lookup. Settings come from
  `raidSettingsLookup`, a test seam; an unreadable read counts nothing.
- The flood trigger is counted by the group `-4` watcher (`raidFloodWatcher`, registered in `LoadLockdown`), which returns
  `ext.ContinueGroups` on every path, so aispam, antiflood, locks and blacklists still see every message, and deletes
  nothing: the lockdown's muting stops the flood. What counts (`raidFloodItemKey`) is any user content in a supergroup:
  text that is not a command (a `bot_command` entity at offset 0, or a text starting with `/` and a non-space
  character, which gotgbot's command handler also runs), photo, GIF (animation, checked before the
  document Telegram also sets), sticker, video, video note, voice, audio, document, poll, contact, location, venue, dice
  and story, forwards and replies included; an album counts once. Commands, service messages, edited messages, channel
  posts and bot senders never count (gotgbot drops the last three before the watcher). Who never counts: a sender with
  `sender_chat` set (an anonymous administrator, a channel, a linked channel's automatic forward: gotgbot's `IsBot` is
  false for all of them), a Telegram service ID (`staffServiceUserIDs`), the bot, an approved user
  (`IsApprovedForUpdate`) and an administrator by the cached list (`antifloodModule.userIsFloodExempt`; its bounded live
  fallback assumes an administrator when saturated, which only lowers the counts). Checks run cheapest first (chat type,
  sender, item, Redis, settings, approvals, admin list, then one script call under 250 ms; an error is no information).
  The rule is `raidFloodCounts`: at least the chat's threshold (default 30) of items in the fixed 30 s window, at least
  60% of them from members who joined in the last 24 h, from at least 3 such accounts; the windows, the 60% and the
  3-account floor are code constants. It then calls `raidFire` with `trigger_kind` `flood`, the item count, window 30
  and the percent. Without Redis it is inert.
- An automatic lockdown (`lockdownStartedAutomatically`: `trigger_kind` `join_surge` or `flood`) is described as such to
  admins. `/lockdownstatus` opens with `lockdown_status_active_auto` and the trigger sentence
  (`lockdownTriggerSentence`, from the row's trigger columns, never from `reason`) instead of "started by" the bot, and
  `/lockdown` during one answers `lockdown_already_active_auto` plus the same sentence and starts nothing
  (`lockdownAlreadyActiveText`); every other kind, a manual lockdown included, reads as before.
- A lockdown never touches another group: every lockdown query is keyed by `chat_id` or `lockdown_id`, and the worker
  calls Telegram with the row's own chat, so two locked groups lift in either order without a call or post in the other.
- The old raid mode is retired (`antiraid.go`, LOCK-15). `/antiraid`, `/raidtime` and `/raidactiontime` are one pointer
  command (`antiraidRetiredDesc`, `Disableable`, no checks) that replies `lockdown_antiraid_retired` and changes
  nothing; a press on an old `antiraid|` button is answered once with the same text and changes nothing. `/autoantiraid`
  (`autoantiraidDesc`) is kept as an alias of the join-surge threshold: with words it runs `autolockdownApply` with
  `joins` in front, alone it shows the triggers, and it has exactly `/autolockdown`'s checks
  (`requireAutoLockdownAuthority`, a live `getChatMember`) and anonymous-admin re-entry, so it is stricter than the old
  command, which any cached admin could use. There is no join handler at group `-5` and no expiry poller: nobody is
  temp-banned by the raid mode, whatever an old `alita:antiraid:*` key says. The keys are removed by
  `sweepRetiredAntiRaidKeys` (`SCAN` with `COUNT 100` and `UNLINK`, started once per process from `LoadAntiRaid` in a
  recover-protected goroutine, bounded to 30 s, a no-op without Redis). During a rolling deploy an old replica can still
  temp-ban until it stops and can leave new keys, which the next start's sweep clears. The `antiraid_settings` table and
  `alita/db/antiraid` stay (migrations are append-only, and the legacy backup module reads them); no command reads its
  timing columns any more.
- The lockdown lift runs in the same worker after `/unlockdown` restored the permissions. It unbans (`only_if_banned=true`)
  only a joiner whose live `getChatMember` shows kicked with that row's `ban_until` (`isLockdownBan`, 2 s tolerance); anyone
  else, a deliberate `/ban` or `/tban` included, is kept and never touched (a staff `/ban` or `/tban` over a lockdown's
  ban replaces it with its own end date, so it falls here too; see the StaffActions bullet). Joiners still pending when
  the lift starts are
  cancelled and never banned. Rows are handled in the order they were recorded, and the tally is posted once, by the
  replica whose `FinishLift` conditional update won; a lockdown with nothing to report posts none.
- A lockdown has no end date but its bans do: each joiner's ban lasts `lockdownBanSpan` (330 days) from the join, so a
  lockdown that outlasts that stops banning the raiders it removed (Telegram bans of 30 s to 366 days are the only
  finite ones, and a longer one would be permanent and look like a deliberate ban). This is an accepted limit; the
  worker does not re-ban, and the help text says so. At the lift a joiner who is `left` with a `ban_until` in the past
  (`lockdownBanExpired`) is moved to `kept` with detail `models.JoinerDetailBanExpired` (the joiner state check has no
  state for it, and migrations are append-only), and the tally reports those as `lockdown_lift_tally_expired`, apart
  from the `kept` ones someone unbanned early or banned on purpose; it never says they were left on purpose.
- The alert of a lockdown is decided once, by the conditional `lockdown.BeginAlert` (`alert_started_at IS NULL`, in a
  transaction that also creates the copy rows and saves the burst snapshot), and only the lockdown worker runs it
  (`lockdownPlanAlerts`, for every confirmed active lockdown without `alert_started_at`), so `/lockdown`, the settle
  step and a restart share one path (`/lockdown` only wakes the worker). A group with no destination still gets
  stamped, so a Staff Group linked or a log channel set later never receives an old alert. The alert goes to the linked
  Staff Group (always) and to the group's log channel only when its `admin` category is on (`lockdownAlertDestinations`,
  through `actionlog.Destination`, which also gives the usual group header the log copy starts with and stores in its
  `body`); the log copy is in the locked group's language. The locked group itself never gets an alert. The log
  destination is checked again just before a log copy is sent: a channel that no longer takes the `admin` category gets
  nothing, and the copy is ended as failed after three tries. A staff copy is checked the same way against a fresh
  `staff.GetLinkOfGroupFresh`: when the group is no longer linked, or linked to another Staff Group than the copy's
  destination, the copy is ended as failed at once with the detail `no longer linked` and nothing is sent; an unreadable
  link leaves the copy pending without an attempt. Copies are claimed with a conditional update and posted
  through `lockdownPaced` (`lockdownPostAlerts`); only a pending copy of an active lockdown is listed, so a lifted
  lockdown's copy is never sent. The alert shows the trigger, the counts, a sample of up to 10 accounts and the invite
  link (see `chat_lockdown_burst`), and every name, username and link is escaped and spliced in after the translator.
- The alert copies resume like joiner rows. A copy claimed (`posting`) or close-claimed (`posted` with `claimed_at`) for
  over 2 minutes (`lockdownStaleClaim`) is released by the first cycle of any replica, in `lockdownReleaseStale` through
  `lockdown.ReleaseStaleAlertClaims`, without counting an attempt, so a crash between the send and the record can post a
  second copy (accepted over a lost one). A send is tried three times (`lockdownMaxAttempts`, then `FailAlertSlot` with
  Telegram's reason), a rate limit or a shutdown costs no attempt, each copy has its own claim and attempts so one
  failing destination never holds the other back, and a lockdown confirmed by the settle step is alerted in the same
  cycle (the plan step runs after `lockdownSettleUnconfirmed`).
- Alert copies marked `edit_pending` are refreshed by `lockdownSyncAlertCopies` (a step of `runLockdownCycle`, before the
  close step): `ListAlertSlotsToSyncFresh` lists posted, open, unclaimed copies of active lockdowns, `ClaimAlertSync`
  claims one with the close claim's `claimed_at`, and the copy is edited to its stored `body` plus the footers
  (`lockdownAlertFooters`, today the "Link revoked by X" line) with the keyboard `lockdownAlertKeyboard` gives it now (no
  Revoke button after a revoke, Lift and Ban stay), bounded by `lockdownAlertBatch` per cycle. A rate limit or a shutdown
  gives the claim back free, a gone message closes the copy, and the third failed edit gives the refresh up
  (`RecordAlertSynced`) so a copy never loops. The close step appends the same footers before the lifted footer. A copy posted after a revoke (alert or reminder,
  `lockdownPostAlertSlot`) is sent with the same footers at once, while its stored `body` stays without them.
  `RecordAlertSynced(id, buttonsLive)` clears `edit_pending` only while `buttons_live` still has the value the edit was
  made with, and the sync step reads the copy again after claiming it, so a mark set while an edit ran (a reminder
  taking the previous one's buttons off) survives and the next cycle edits the copy again.
- Reminders: every `lockdownReminderEvery` (1 hour, counted from `alert_started_at`, never from the lock) the worker
  (`lockdownPlanReminders`, right before the poster in `runLockdownCycle`) plans one reminder copy per alert destination:
  kind `reminder`, `seq = floor((now - alert_started_at) / lockdownReminderEvery)`, inserted by
  `lockdown.PlanReminderSlots` with `ON CONFLICT DO NOTHING` on the slot key, so exactly one row exists per reminder and
  destination whichever replica plans it. Only the current seq is ever planned (no backlog after downtime, and an
  alert older than this feature does not fire a burst), and a pending reminder behind the current seq is marked failed
  with detail `superseded` instead of posted. The destinations are the alert's destination kinds
  (`lockdown.AlertDestKinds`) resolved again by `lockdownAlertDestinations`, so a Staff Group unlinked since, a kind the
  alert never went to and a log channel whose `admin` category is now off get none. Reminders use the alert's claims,
  retries and stale-claim release, so the duplicate-over-lost rule above holds: a reminder claimed in `posting` for over
  2 minutes is released and its send repeated, which can post it twice after a crash, unless a newer reminder is due by
  then, in which case it is superseded and lost on purpose. Posting a reminder calls `lockdown.SupersedeReminders`: the
  older reminders of that destination get `buttons_live` false and `edit_pending` true and lose their buttons through the
  sync step; the original alert copy keeps its buttons until the lift. Planning stops with the lockdown being active
  (`PlanReminderSlots` checks it in its transaction, the poster lists only active lockdowns), and the lift's close step
  edits every alert and reminder copy to the lifted footer. A reminder shows the reason (for an automatic lockdown the
  trigger sentence from `lockdownTriggerSentence` in its place, since it has no reason), the joiners removed so far
  (`lockdownRemovedSoFar`, response bans included), the ones still pending and the alert's link line, with no sample;
  its Ban label is counted when the reminder is posted.
- At the lift, whether it came from `/unlockdown` or from the alert's button, the worker edits every posted alert copy to its
  stored `body` plus "Lifted by X at T" (`lockdown_alert_lifted_footer`, the lifter and `lift_started_at` read from the
  lockdown row) with no buttons (`lockdownCloseAlerts`, run after `lockdownProcessLifts` in every cycle, so a restart and
  a second replica finish it). A copy is claimed through `lockdown.ClaimAlertClose` (it reuses `claimed_at`, which a
  posted copy has no other use for; `attempts` is one counter for post, refresh and close, so a claim of a copy still
  marked `edit_pending` resets it to zero and clears the mark in the same update, and the close is then read again, so
  refreshes that failed before the lift cannot use up the close's attempts), listed only while its lockdown is not active, newest first and at most
  `lockdownAlertBatch` per cycle, and recorded with `closed_at` by `RecordAlertClosed`, so it is edited exactly once. A
  gone ("message to edit not found", "message can't be edited", `MESSAGE_ID_INVALID`) or unmodified copy counts as closed;
  a rate limit or shutdown gives the claim back without an attempt and ends the step for that cycle; any other failure
  costs an attempt and the third closes the copy with Telegram's reason.
- "Ban N recent joiners" decides each account of the burst with `decideLockdownResponseBan` from its live status and
  bans with empty options (no end date, no `revoke_messages`) through `lockdownPaced`, in `lockdownResponseBanOne` only.
  A live creator or administrator, an approved user, the bot itself and a joiner recorded exempt (let in by an admin,
  D-22: skipped `let_in` from a fresh read, with no Telegram call, also when the row turned exempt after the run was
  queued) are never banned, and a ban someone else placed is never shortened (a kicked account is banned again only when
  it is under the lockdown's own ban, which the new ban upgrades). The joiner row is claimed to acting before the
  call and moved to `kept` with detail `response_ban` after it, so the worker's marker ban and the lift's unban never
  touch it; on a skip, a failure or a rate limit the joiner row goes back to the state and detail it came from, and a
  row held by the worker or the lift (acting, unbanning) puts the account back in the queue without an attempt. The
  lift never unbans a response ban and its tally counts it among the ones left alone on purpose, while
  `/lockdownstatus` still counts it as removed (`lockdownRemovedSoFar`: banned plus kept with detail `response_ban`).
  Kept rows are not reclaimed on a re-join (`lockdownReclaimStates`), so someone an admin unbans by hand during the
  lockdown and who joins again is not banned again and gets no new joiner row, still muted by the locked defaults; this
  is an accepted limit (research Pitfall 3).
- The "Ban N recent joiners" button is wired end to end. Approved users are read fresh from the database through
  `approvals.ListApprovedIDsFresh` (the `lockdownApprovedLookup` seam), never the cached `IsUserApproved`, which turns a
  failed read into "not approved": when the read fails the Ask answers `lockdown_ban_check_failed` and posts no card, a
  Confirm starts no run, and a running run waits (the next pass reads again), so nobody is banned without the list. N
  and the run's queue leave out approved users, the bot and, through `CountBanCandidatesFresh`, joiners an admin let in.
  Confirm edits the card to "Banning N…" first and then calls `lockdown.StartBanRun` with the card's chat and message
  stored on the run in the same insert, so the worker's result edit can never be overwritten by the running text. The
  lockdown worker executes the run (`lockdownProcessBanRuns`, after `lockdownBanPending` in every cycle, in batches of
  `lockdownWorkerBatch`) through `lockdownResponseBanOne`, and the result of a finished run is delivered by
  `lockdownReportBanRuns`, a step of `runLockdownCycle` right after it, which resumes from the database like the alert
  close step: a run (done or stopped) with `reported_at` NULL is claimed (`ClaimBanRunReport`, `report_claimed_at`), so
  one replica delivers it; the card edit (text from `lockdownBanResultText`) and the one `#LOCKDOWN_BAN` post are
  separate steps, each recorded once settled (`RecordBanRunReportStep`), so a step that was delivered is never repeated
  while the other is retried and the post is not sent twice once recorded. A rate limit or a shutdown gives the claim
  back without an attempt and ends the step for the cycle, any other failure costs an attempt on `report_attempts`
  (back to zero when a step settles) and the third gives that step up with a warning, and `lockdownReleaseStale` returns
  a claim older than `lockdownStaleClaim` through `ReleaseStaleBanRunReportClaims`, so a crash between the send and the
  record can repeat a step (duplicate accepted over a lost one). The button stays
  until the lift: a later press starts a new run that handles only accounts no earlier press handled (a burst row in a
  terminal state is never queued again; approved users are never queued and stay unhandled), and its N counts only those.
  A restart resumes a run on any replica (`ReleaseStaleBurstClaims` returns an abandoned claim to the queue). A run of a
  lockdown that is no longer active is stopped, not left: the lockdown is read again before every account, so a lift
  that starts in the middle of a batch ends the run at the next account (and `lockdownResponseBanOne` reads it once more
  inside the paced unit, right before `banChatMember`, because the wait for a slot can take a minute: a lockdown no
  longer active then restores the joiner row, queues the account again without an attempt and sends no ban), `UnqueueBanRun` gives the queued accounts back
  (unhandled, no run; an acting row is left to its call in flight), the run moves to `stopped` through the same
  conditional finish and its card ends with `lockdown_ban_stopped`. The report counts banned (with the declined join
  requests among them on their own line), skipped with one line per reason (admin, approved, already banned, let in by an
  admin) and failed, naming up to 10 failed accounts with their ID and Telegram's stored reason and counting the rest.
  The `#LOCKDOWN_BAN` post goes to the locked group's log channel under the admin category through `actionlog.Destination`
  and the lockdown pacer, names the admin who pressed Ban, is not localized in its hashtag, and is sent only for a run
  that handled at least one account; a failed post is retried as above and nothing is posted when the category is off.
  `lockdownReleaseStale` also returns stale burst claims to the queue. These bans write nothing to the staff audit and
  cannot be undone. The card and button say only that the accounts are banned for good and cannot rejoin, which is
  exactly what the call does (no `revoke_messages` is sent, so nothing is said about their messages).
- `resolveUnmutePermissions` returns `(permissions, error)` and is the one choke point of every unmute. During an
  active lockdown it returns the lockdown's stored pre-lockdown permissions, because the group's live defaults are then
  the locked set and copying them into a user's own restriction would leave them muted after the lift (D-23). Every unmute
  path goes through it: `/unmute`, the unrestrict button's Unmute, a captcha pass and staff `/unmute` (and so the undo of a
  staff mute). It reads the lockdown fresh through the `lockdownSnapshotLookup` seam, only for a chat with a non-zero ID,
  and only an active row counts. None of them may restrict or announce an unmute when it returns an error: `/unmute` and
  the button report the error, a captcha pass schedules its retry, and staff `/unmute` fails that group. The `/unmute`
  reply adds `mutes_unmute_lockdown_note` during a lockdown.
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
  Its one exception to "a ban never shortens a ban" is a lockdown's own ban: for a staff ban (`/ban`, `/tban`) on a
  kicked target with an end date, `runStaffActionInGroup` asks `staffLockdownBanLookup` (`lockdown.HasJoinerBanFresh`
  on the live `until_date`) and sets `staffTargetState.LockdownBan`, and `decideStaffBan` then sends the ban even when
  it ends sooner, so the staff ban gets its own end date and the lockdown's lift keeps it (D-05). A failed lookup
  fails that group as an internal error with no write. Undo decisions live in `decideStaffUndo` alone, beside `decideStaffAction`, and `executeStaffUndoCall` switches only
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
- The harness `AutoMigrate` lists (`alita/modules/test_harness_test.go`, `alita/db/staff/testmain_test.go`,
  `alita/db/testmain_test.go`) include the staff audit models, and `staffCleanup` deletes audit rows. The lockdown
  models are in `alita/modules/test_harness_test.go`, `alita/db/testmain_test.go` and
  `alita/db/lockdown/testmain_test.go` (not in the staff package's list), and `lockdownCleanup` deletes lockdown rows.
  The Phase 5 models (`ChatLockdownAlert`, `ChatLockdownBurst`, `ChatLockdownBanRun`) are in
  `alita/modules/test_harness_test.go` and `alita/db/lockdown/testmain_test.go` only; the `alita/db` package's own
  tests never touch them. The Phase 6 model `ChatRaidDetection` is in `alita/modules/test_harness_test.go`,
  `alita/db/raiddetection/testmain_test.go` and `alita/db/devs/testmain_test.go` (the developer `/stats` reads it), and
  `raidCleanup` in the modules tests deletes its rows and their cached copies.
- `make test-postgres-integrity` is the PostgreSQL run in CI. Its `-run` list holds the guarantees SQLite cannot
  prove: `TestStartLockdownOneActivePerChat`, `TestBeginAlert` (the alert is decided exactly once) and
  `TestBanRunStart` (one running ban run per lockdown, the partial unique index), plus the Phase 5 conditional updates:
  `TestRecordLinkRevoked` (one revoke wins), `TestReminderSlots` (`PlanReminderSlots`, one reminder row per
  destination through the slot unique index and `ON CONFLICT DO NOTHING`), `TestBanRunStart` and `TestUnqueueBanRun`
  (`FinishBanRun`'s `NOT EXISTS` guard and its single finish), `TestAlertSync` and
  `TestAlertSyncKeepsAMarkSetDuringTheEdit` (`RecordAlertSynced`'s parameterised `CASE`), `TestAlertClose`
  (`ClaimAlertClose`'s `CASE` over the old row) and `TestBanRunReport` (one report claim wins, steps settled
  separately). The Phase 6 guarantees are `TestRaidDetectionSettings` (`./alita/db/raiddetection`: the `CHECK` 3..500
  and the column-targeted upserts that never clobber each other) and `TestRaidDetectionCarryOver`
  (`./alita/db/migrations`: the clamped carry-over of `antiraid_settings.auto_antiraid_threshold`, which executes
  migration 20261018130000 against the schema the migration-chain step already built, so it must run after that step).
  The pattern is anchored with `$`, so list the real top-level test function names and add the test's package to the
  target. Add a test there when a new guarantee rests on a partial unique index, a `CHECK` or a conditional update.
- In CI, keep the migration-chain step before `make test`; its `schema_migrations` rows back the checksum test.

## Commits

- Use Conventional Commits. The changelog drops `docs:`/`test:`/`chore:`/`ci:`/`deps:`, so user-visible changes need
  `feat:` or `fix:`.
