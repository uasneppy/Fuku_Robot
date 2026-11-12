# AGENTS.md

Fuku Robot: Telegram group-management bot. Go module `github.com/uasneppy/fuku_robot`; PostgreSQL (GORM) + Redis.
Update this file in the same commit as any change that invalidates a rule below.

## Commands

```bash
make test                 # the only valid full test run (tags, race, coverage, skip gate; 30 min per package, 45 min for the CI job; fuku/modules runs 9-15 min depending on the machine, a CI runner can be 1.7x slower than another)
make lint                 # golangci-lint v2
make check-translations   # missing locale keys
make check-docs           # generated-docs drift; fix with `make generate-docs`
make bump-version TAG=vX.Y.Z   # the only way to change version strings
go test -tags testtools -race -count=1 -run '^TestName$' ./fuku/db/warns   # one test
CGO_ENABLED=0 go build ./...   # compile check; `make build` needs goreleaser v2 + Docker
```

## Startup and shutdown

- `fuku/config` `init()` → `fuku/db` `init()` connects to Postgres **before `main()`** and runs migrations when
  `AUTO_MIGRATE=true`. Never add DB access to an `init()` that can run before `fuku/db`'s.
- `--version`, `--health`, and `*.test` binaries (unless `FUKU_TEST_DATABASE=true`) skip DB init. Guard new pre-`main`
  side effects with `isCliModeActive`.
- Shutdown runs LIFO within 60 s. Register new drains *after* DB-close so they run before it. Each handler gets 10 s
  (`defaultHandlerTimeout`) unless registered with `RegisterHandlerWithTimeout`; a drain whose own wait is longer is
  registered with that wait plus `shutdownDrainGrace` (1 s), so its own timer ends it and the DB close never runs under
  it. The 60 s budget still bounds everything and exits with code 1 wherever it runs out, so deploy manifests must allow
  more than 60 s between SIGTERM and SIGKILL (`stop_grace_period: 75s` in `docker-compose.yml`).
- `StopStaffActions` is one of those drains, registered through `RegisterStaffActionsDrain` (`staffActionStopWait` plus
  grace). In webhook mode the drains before it (HTTP stop 10 s, AI spam 10 s, raid AI 2 s, raid posts 3 s, lockdown 6 s)
  leave its 31 s inside the 60 s only while they are not all at their limit: when every one of them runs to its limit the
  staff drain can be cut up to 2 s short (idle drains return at once, so this is the worst case, not the usual one); in polling mode the updater's extra 10 s can let the global budget cut it short. It cancels running staff fan-outs, marks every unfinished group
  "interrupted by restart", delivers the final summary and returns within 30 s. A delivery still waiting out a 429
  when the 30 s are up is cut off with the process. A hard crash can still leave ⏳ lines. A run cancelled by the
  shutdown skips its remaining log posts, so a group applied just before it may have no post. Undo runs join the
  same drain: their final record write (`FinalizeUndo`, or `ReleaseUndo` when no group reached its write, both through
  `db.DB`, never the cancelled run context) and summary delivery happen inside the same 30 s as a staff action's, and an
  unfinished group is recorded `fail_interrupted`. An undo Confirm joins `staffActionRunsWG` (`joinStaffRuns`) before it
  claims, so `StopStaffActions` waits for a Confirm that has already claimed, and its run gives the claim back when the
  shutdown cut every group off before its write. Once `StopStaffActions` has cancelled the run context an undo Confirm
  claims nothing and aborts its card with the restart text (`staff_undo_abort_restarting`).
- `StopMemberReqJobs` is the drain of the member requirement enforcement jobs, registered after DB-close through
  `RegisterMemberReqDrain` in `main.go` right before `RegisterStaffActionsDrain`. Its wait is `memberReqStopWait` (2 s)
  plus `shutdownDrainGrace` (1 s), 3 s in all. It sets `memberReqDraining` (a late message starts no job), cancels the
  jobs' root context first (`memberReqContext`: every paced call of a job derives from it) and waits at most 2 s for
  `memberReqJobs`, logging one warning when the wait runs out. Each job runs under `memberReqJobTimeout` (90 s), which
  ends 30 s before `memberreq.StaleClaim` (2 minutes) lets a reclaim start, so two jobs never act on one hold. LIFO
  runs it after the staff drain and before the admin index sweeper stop, the staff sweeper stop, the captcha lifecycle
  stop, the tracer shutdown, the monitoring stops, the users async writes drain and the database close. In webhook mode
  the worst case of the drains up to and including it is 65 s (HTTP stop 10, AI spam 10, raid AI 2, raid posts 3,
  lockdown 6, staff 31, member requirements 3; `StopDeletionWorker`, registered right before it, runs after it in LIFO
  order and adds 3 s, see below) against the 60 s budget, so when every earlier drain runs to its limit
  the staff drain is cut 2 s short and the member requirements drain and every handler after it in LIFO order, the
  database close included, are cut, and the process exits with code 1 inside `stop_grace_period: 75s`, before SIGKILL; in
  polling mode the updater's extra 10 s moves the cut earlier. This worst case costs no correctness, because every hold
  resumes from the database and a job cut off leaves a pending hold that a reclaim retries after `memberreq.StaleClaim`
  (the member is punished once and their notice comes late at worst). The pause latch of the per-chat punishment cap
  (`fuku:memberreq:{<chat>}:paused`) is a Redis key with its own expiry, so a shutdown cut short neither ends a pause nor
  starts one. The lookups of
  the photo and channel rules derive from `memberReqContext()`, so `StopMemberReqJobs` ends them too (a job inside a lookup returns at once when the root is cancelled, leaving no hold, notice
  or write, because the claim follows every lookup, and the flight goroutine of the cancelled lookup ends with it), they read and write only
  Redis and Telegram and never PostgreSQL, and they added no drain and no second: the 65 s worst case is unchanged
  (`TestShutdownEndsMemberReqLookups`). The provider call of the AI spam-name rule (`memberReqAIRule`, Phase 15) derives
  from `memberReqContext()` too, inside its flight, so `StopMemberReqJobs` ends it (a job cut off leaves no hold, notice or
  cached verdict); it reads and writes only Redis and Anthropic, never PostgreSQL or Telegram, and no drain or second was
  added: the 65 s worst case is unchanged. `TestShutdownEndsMemberReqAI` proves it: a job inside a hung provider call is
  ended by the drain's own cancel inside `memberReqStopWait`, leaves no hold, notice, write, warn row or cached verdict, is
  not counted as a provider failure by the breaker, releases its gate slot, and its flight goroutine ends (a fresh flight of
  the same `n:` key is not joined), and a later message starts no job.
- `StopLockdownWorker` is another drain registered after DB-close, through `RegisterLockdownWorkerDrain`
  (`lockdownWorkerStopWait` plus grace). It cancels the lockdown worker and waits at most
  5 s (`lockdownWorkerStopWait`), because every lockdown row resumes from the database: a cycle cut off at shutdown
  loses nothing, and a restart or a second replica picks the rows up again. A claim (an acting or unbanning joiner row
  with `claimed_at`) older than 2 minutes (`lockdownStaleClaim`) belongs to a worker that stopped: the first cycle of any
  replica releases it through `lockdown.ReleaseStaleClaims`, before any other step and without counting an attempt.
  Acting goes back to pending in an active lockdown, to banned (a ban) or cancelled (a join request) in a lifting one,
  and unbanning goes back to banned, so a restart resumes every ban, decline and unban and the live check at the lift
  keeps a repeated unban safe.
- `StopRaidPosts` is a drain registered after DB-close too, through `RegisterRaidPostsDrain` (`raidPostStopWait`, 2 s,
  plus grace). It waits for the public notice and the cannot-lock warning of an automatic lockdown that are in flight
  (`raidPosts`, a counter and not a `sync.WaitGroup`, because the drain gives up after its wait while a new post can still
  start). A post is best effort: the lockdown it announces is already in place and its alert is driven by the database,
  so a post still running when the 2 s are up is cut off with the process and only that notice is lost.
- `StopRaidAI` is a drain registered after DB-close as well, through `RegisterRaidAIDrain` (`raidAIStopWait`, 1 s, plus
  grace), in `main.go` right after `RegisterRaidPostsDrain`. Shutdown is LIFO, so it runs after `DrainAISpamChecks`
  (whose verdicts can start lockdowns) and before `StopRaidPosts` (a start can post a notice) and the database close. It
  sets `raidAIDraining`, cancels the raid AI pool's context first (`raidAIContext`: every provider call of a raid-only
  check derives from it, so all of them abort at once, and a check cut off is no verdict, never a breaker failure), then
  waits at most 1 s for `raidAIJobs` and `raidAIFires` to go idle, logging one warning when the wait runs out. Queued
  jobs are dropped as no verdict. A lockdown start still running after the wait is cut off with the process and leaves at
  worst an unconfirmed row that the lockdown worker settles.
- `StopAdminIndexSweeper` is a plain shutdown handler (`RegisterHandler`, the 10 s default, no drain registration)
  that `main.go` registers right after the staff sweeper's stop, so LIFO runs it after `StopStaffActions` (it never takes
  from that drain's 31 s) and before the database close. It cancels the loop and waits for it. It returns at once because
  every Telegram call of a sweep derives from the sweep context (`adminIndexPacedClient` paces and bounds each request
  from it), and the one roster write in flight is bounded by that same context. A cycle cut off by the cancel releases
  its lock by compare-and-delete on its own owner token (`releaseAdminIndexSweepLock`), so a restarted replica's first
  cycle is not skipped for an hour; a finished cycle, and one ended by a second 429, keeps the lock until its TTL.
- `StopDeletionWorker` is a plain shutdown handler (`RegisterHandler`, the 10 s default, no drain registration and no
  `RegisterHandlerWithTimeout`) that `main.go` registers right after the admin index sweeper's stop and before
  `RegisterMemberReqDrain`, with `StartDeletionWorker` in `postInit` right after `StartLockdownWorker`. LIFO runs it after the
  staff actions drain and the member requirements drain and before the admin index sweeper stop and the database close. It
  cancels the deletion root context (`cancelDeletionContext`: the loop, every Telegram call of a cycle and every inline
  delete derive from it) and waits at most `deletionWorkerStopWait` (3 s, an internal wait under the 10 s default), logging
  one warning when the wait runs out. It adds 3 s of allowance: in webhook mode the worst case of the drains up to and
  including it is 68 s (HTTP stop 10, AI spam 10, raid AI 2, raid posts 3, lockdown 6, staff actions 31, member requirements
  3, deletion worker 3) against the enforced 60 s budget, so when every earlier drain runs to its limit the staff drain is
  cut short and the deletion stop and every handler after it in LIFO order, the database close included, are cut, and the
  process exits with code 1 inside `stop_grace_period: 75s`, before SIGKILL; in polling mode the updater's extra 10 s moves
  the cut earlier. This costs no correctness: every row resumes from the database, and a cycle cut off leaves its rows
  pending (or claimed, until the 2 minute stale release), because the unfinished rows are released free on a context
  detached from the cancelled run (`TestShutdownEndsDeletionWorker`). It also stops the member self-destruct batcher
  (`memberDeleteStop`, Phase 17): it sets the draining flag (later offers are shed), asks the batcher for one final
  flush bounded to 2 s (`memberDeleteFinalFlushWait`) on a context detached from the cancelled root, and waits for the worker
  and the batcher under that one 3 s timer, so the stop adds no new drain, no handler registration and no allowance: the 68 s
  figure and `stop_grace_period: 75s` stand unchanged. A flush cut off loses only entries that were never queued, whose
  messages stay (`TestShutdownEndsMemberDeletionBatcher`).
- Deploy manifests set `AUTO_MIGRATE=true`; the code default is `false`. Never call `gorm.AutoMigrate` in production code.
- The Turnstile web captcha is configured in `main` (`registerCaptchaWeb`) when `config.AppConfig.CaptchaWebConfigured()`
  holds: all four of `CAPTCHA_WEB_BASE_URL`, `TELEGRAM_MINI_APP_SHORT_NAME`, `TURNSTILE_SITE_KEY` and
  `TURNSTILE_SECRET_KEY` (some but not all stops startup, and the secret is registered with `logredact`). `main` builds
  the page's string tables from `i18n.StringTable(captchaweb.PageLanguages, captchaweb.PageKeys)` (a missing or empty
  key is fatal), installs the runtime with `modules.ConfigureCaptchaWeb` (`StartNotReady` true, `TrustedProxies` from
  `CAPTCHA_WEB_TRUSTED_PROXIES`, a bad entry stops startup) and mounts `modules.CaptchaWebRoutes()` (`/captcha`,
  `/captcha/verify`, `/captcha/health`) through `Server.Handle` before `Start`. The patterns carry no method on purpose: a
  Go 1.22 method pattern makes the mux answer a wrong method itself with a bare text 405 that has none of the headers
  below, so each handler checks its own method (`Handler.Verify`: POST; `Handler.Health` and the page: GET and HEAD) and
  answers 405 with `Allow`, `Cache-Control: no-store` and `X-Content-Type-Options: nosniff` (a JSON body on verify and
  health). `/captcha/` is a 404. A test drives all of it through a real `http.ServeMux` and `httptest.Server`. With the
  settings unset none of the three routes exists. Without Redis the runtime is not installed (the challenges live in
  Redis), `main` logs one warning, no route is mounted and Turnstile groups get the math captcha. The verify route
  answers 503 retry, and the health route 503, until `modules.MarkCaptchaWebReady` at the very end of `postInit`
  (polling mode starts the HTTP server before the modules load, D-30). No new shutdown drain is added: the HTTP server's
  own `Stop` runs first and a web pass's finish joins `StopCaptchaLifecycle`. The captcha routes share the port with
  `/metrics`, `/db_metrics`, `/health`, pprof and the webhook, so `modules.CaptchaWebStartupWarnings` makes `main` warn
  when `METRICS_AUTH_TOKEN` or `CAPTCHA_WEB_TRUSTED_PROXIES` is empty, and the owner's public proxy must forward only
  `/captcha` and `/captcha/*` (D-31; the bot enforces no path allow-list).

## Handlers

- Group numbers are execution order: `-10` captcha sweeper · `-8` member deletion (two handlers: the edit handler and the self-destruct watcher, a `handlers.NewMessage(message.All, ...)` with no setter; neither ends an update, see the `MemberDeletion` bullet) · `-7` lockdown join guard (it also counts the join surge
  before it reads the lockdown) · `-6` fed-ban · `-5` service message deletion watcher (the only handler of the group, never ends an update, `SetAllowBot`; a deletable service type is deleted now or queued by the group's stored rule) · `-4` raid flood counter (counts everything but slash-prefixed texts, never ends an update; there is no
  antiraid handler any more) · `-3` staff watchers (chat-migration re-key; later ownership and bot health) · `-2` admin-cache (`my_chat_member` only) and the member requirements watcher (supergroup messages only, the only message handler of the group, `ext.ContinueGroups` on every path; never add another message handler to group -2) ·
  `-1` users tracker · `0` commands/help/greetings · `1` settings typed-reply door (private messages only;
  `ext.ContinueGroups` for anyone with no wait and no marker, consumes and ends the update otherwise) · `2` raid AI candidate watcher (hands a new member's suspicious
  item to the raid AI pool, never ends an update) · `3` aispam · `4` antiflood · `5`/`6` locks · `7` blacklists ·
  `8` reports+reactions · `9` filters · `10` pins · `11` log-channel · `12` late raid flood counter (slash-prefixed texts only; keep it
  after every other group).
- gotgbot stops a group at the first matching handler. Watchers return `ext.ContinueGroups`; commands return
  `ext.EndGroups`. A group-0 watcher returning `nil` silently kills every later group-0 handler.
- Modules register in `init()` via `RegisterLegacyModule(name, priority, load)`. Use a unique name; duplicates are
  dropped silently. Priority sets load order and group-0 precedence; `LoadHelp` loads last.
- New commands use `helpers.WrapCommand(dispatcher, CommandDescriptor{...}, handler)`. Set `Disableable: true` for any
  command chats may disable. Do not add replies for failed checks; the pipeline sends them. Every command handler,
  wrapped or raw, is built by `helpers.NewBotCommand` (`fuku/utils/helpers/command_registry.go`), which records the
  lower-cased name and returns the library's own command handler unchanged (so routing is the library's). Command
  deletion reads the registry through `helpers.IsBotCommand`, so only a command this bot registers is ever deleted.
  `WrapCommand`'s `register` and `MultiCommand` go through it, and `TestNoRawNewCommand` (a `go/parser` walk of `fuku/`
  and `main.go`, tests excluded) fails on any other construction of the library's command handler. Never call
  `handlers.NewCommand` or build a `handlers.Command` literal outside that file.
- Anonymous admins bypass `WrapCommand`. Admin commands that must work for them also need
  `RegisterAnonymousAdminHandler` + `anonPipelineHandler`. After the proof tap, `verifyAnonymousAdmin` swaps the callback
  update for a message update carrying the cached command and points `EffectiveChat` at that command's chat, so the
  re-entered command, the `RequiredChecks` that `anonPipelineHandler` re-runs, and `PermissionResponder` see the
  command's chat as if the admin had sent it, with the tapper as the sender. `verifyAnonymousAdmin` refuses the tap as
  an invalid request, deleting and running nothing, unless the callback's `c`, the proof button's chat and the cached
  command's chat are the same chat.
- The `StaffActions` module (priority 65) registers raw interceptors, built through `helpers.NewBotCommand`, at group 0 ahead of Bans (70)
  and Mutes (80), looping over a table so the docs generator skips them. Outside a Staff Group they return
  `ext.ContinueGroups` with no reply, write or Telegram call. They are a documented exception to the `WrapCommand`
  rule, because `BuildCommandContext` replies to sender-less updates, and they have no anonymous-admin re-entry.
- Handler methods use value receivers on `moduleStruct`.
- The `ServiceDeletion` module (priority 242, `deletion_service.go`) registers the service message deletion watcher
  at group `-5` (`serviceDeleteGroup`) through `handlers.NewMessage` with `SetAllowBot(true)`, the only handler of that
  group, so never add a second message handler there that ends an update. The watcher's result is a named value preset to
  `ext.ContinueGroups` (a recovered panic cannot return `nil`) and it returns it on every path. Its order, cheapest first:
  a message in a group or supergroup, `deletionServiceKindOf` (one of the deletable service types), for joined and left only
  a bot sender (`ctx.EffectiveSender.IsBot()`: `cleanService` accepts no bot, so a join or leave a bot caused is the
  watcher's and every other sender's is `cleanService`'s, never both; an anonymous admin has `sender_chat` set and is not a
  bot, a message with no sender is handled by neither path), then `deletionRuleFor` (the cached rules read; joined and left
  also the shared flag through `deletionFlagLookup`; an error deletes nothing) and `deletionAct`. `cleanService` (group 0)
  keeps join processing and deletes only after it, with the same effective rule of D-02 (flag off is Keep even with a stale
  row, flag on with no row is delete now, flag on with a row is a queue row at the message date plus the delay), never
  returns a delete error and always ends with `ext.EndGroups`. Join and leave cleaning (`/cleanservice`) does nothing inside a
  Staff Group, because `deletionAct` refuses every deletion there (D-18, DEL-07): a Staff Group with the flag on keeps its join
  and leave service messages, which is a change from before Phase 16.
- Command deletion (`command_deletion.go`, Phase 17, DEL-03): `modules.CommandDeletionProcessor` wraps
  `tracing.TracingProcessor` in `newConfiguredDispatcher` (`Processor: modules.CommandDeletionProcessor{Inner:
  tracing.TracingProcessor{}}`). After every handler group of an update has run it deletes a member's command (an admin's
  included) when the chat stores `member:commands` as 0; a stored value other than 0 is Off. A command is a message
  gotgbot's matcher hands to a name `helpers.IsBotCommand` knows: `botCommandName` mirrors the library's `checkMessage`
  (bare or `@thisbot`, case-insensitive, caption and entity-less forms included; `@otherbot`, an unregistered word, an
  edit, a private chat, a bot, a `sender_chat`, a Telegram service account and the bot itself never), and
  `TestBotCommandNameMatchesGotgbot` proves it against the library's own `Command.CheckUpdate`. The wrapper reads
  `ctx.Update.Message` and `ctx.EffectiveSender` after `Inner` returned (an anonymous-admin tap replaces `ctx.Update`),
  uses a fresh context and never `tracing.UpdateContext` (the tracing processor cancels its own context when it returns
  in polling mode), deletes through `deletionAct` with delay 0 (so the delete-right latch, the Staff Group refusal, the
  deletion pacer and the due-now fallback apply) and has no defer around `Inner`: a handler panic that reaches the
  dispatcher skips the delete. No reply path is edited (handlers reply inside the update, the delete follows it) and it
  adds no handler group; it is the one documented place that acts after the dispatcher's groups. Its limits (plan 17-04,
  `TestCommandDeletionAnonymousAdmin`, `TestCommandDeletionSenders`, `TestCommandDeletionRefusedAndFailed`,
  `TestCommandDeletionStaffGroupNeverTouched`): an anonymous admin's command is deleted only from the re-entered update
  after the proof tap, in the order proof message deleted, the command's own effect and reply, the command message
  deleted, and it stays when nobody taps; one that needs no proof stays because its sender is the group, and in a group
  whose AnonAdmin setting is on an anonymous admin's command runs at its first update with `sender_chat` set, so the wrapper
  skips it and that command message stays: the stated limit of D-34 and owner confirmation A17-2, deferred because the
  wrapper cannot tell a fully handled anonymous command from a proof prompt. A command refused by a check (not an admin,
  disabled, wrong chat type, bot not an administrator) or failed with an error is deleted after its answer, `/cancel` in
  the group too. A held member's command (deleted at group -10) or a fed-banned user's, and a command a handler already
  deleted (`/del`), cost one refused delete call that is dropped quietly with no row and no message ID in the log. A Staff
  Group is never touched, and a failing Staff Group read deletes nothing (`deletionAct`). The bare-name race with another
  bot that handles the same word is accepted and is a live check. `TestCommandRepliesStayInsideTheHandler` guards that no
  goroutine in `fuku/modules` replies, so no handler can reply to a command after the wrapper deleted it.
- The `MemberDeletion` module (priority 243, `member_deletion.go`, Phase 17, DEL-04) hosts the member message rules that need
  a handler of their own, at handler group `-8` (`memberDeleteGroup`, free before: the groups in use were -10, -7 to -1 and 0
  to 12; it runs after the captcha hold and before the lockdown join guard). It holds two handlers: the edit handler,
  `memberEditedHandler`, a raw `ext.Handler` whose `CheckUpdate` is `ctx.EditedMessage != nil` and nothing else (never a
  `handlers.NewMessage` with edits allowed, so a new message never reaches it), registered by `LoadMemberDeletion` with
  `AddHandlerToGroup`, and the self-destruct watcher (below), in the same group, which is safe because neither handler ever
  ends an update: gotgbot moves to the next handler of a group only on `ext.ContinueGroups`, and the edit handler's result is a
  named value preset to it with `defer error_handling.RecoverFromPanic`, so a recovered panic cannot return `nil` (which would
  leave the group and skip the sibling) and no path returns `ext.EndGroups`. It is the first handler of the bot that consumes
  edits: antiflood, locks, blacklists and aispam still see new messages only. The order is cheapest first and does no I/O before
  the rule read: the chat is a group or supergroup; the sender's shape through `raidFloodSenderCounts` (no `sender_chat`,
  automatic forward, service ID or the bot itself), not a bot (`ctx.EffectiveSender.IsBot()`) and no `via_bot`; the pure
  `memberEditDue`; the stored `member:edited` grace through `deletionRulesLookup` (only one of the five presets counts, any
  other stored value, a hand-written 45 included, is Off, and an unreadable rule deletes nothing); the shared exemption
  `memberDeletionExempt`; the pending captcha; then `deletionAct` with kind `member:edited`, delay 0 and `SentAt` the message's
  own date (so the delete-right latch, the Staff Group refusal that fails closed, the deletion pacer and the due-now fallback
  apply as for every kind). Unknown never deletes (D-33): the exemptions are tri-state and a lookup that fails keeps the
  message. Approvals go through `memberDeletionApprovedLookup` (default `approvals.IsUserApprovedContext`; approved or an
  error is exempt, never the cached predicate that turns a failed load into "not approved"), administrators through
  `memberDeletionAdminStatus` (the cached admin list when it knows, otherwise one live `getChatMember` through the seam
  `memberDeletionMemberLookup`, bounded by `memberDeletionAdminTimeout` (3 s) and by the semaphore `memberDeletionAdminSlots`
  (8, a full one answers unknown at once and never waits); creator and administrator are admins, member, restricted, left and
  kicked are not, and an error, a 429, a 5xx, a timeout, an empty or unrecognised answer or a full semaphore is unknown, which
  is exempt), and the pending captcha explicitly for edits (`memberEditPendingCaptcha` through the member requirements' two
  captcha seams: an attempt or any read error is pending). It never uses `antifloodModule.userIsFloodExempt`,
  `chat_status.IsUserAdmin` or `IsApprovedForUpdate` as its answer, because each reads a failed lookup as "not an admin" or
  "not approved". The self-destruct watcher reuses `memberDeletionExempt` unchanged. The handler logs by chat and user
  ID and the error class only, never a name, text or message ID. `TestMemberEditedDeletesLateEdit`, `TestMemberEditedGrace`,
  `TestMemberDeletionExemptTriState`, `TestMemberEditedKeeps`, `TestMemberEditedNeverEndsUpdate`,
  `TestMemberEditedStaffGroupNeverTouched` and `TestMemberEditedNeedsDeleteRight` hold it.
  The self-destruct watcher, `memberSelfDestructWatch` (Phase 17, DEL-05; a value-receiver method of `moduleStruct`),
  is registered by the same `LoadMemberDeletion` with `handlers.NewMessage(message.All, ...)` at `memberDeleteGroup` and
  no setter, so it sees new messages of real users only (no edits, channel posts or bot senders), and it arrives together
  with every exemption, the pin drop and the `/pin` hook, never before. Its result is a named value preset to
  `ext.ContinueGroups` with `defer error_handling.RecoverFromPanic`, and it never ends an update. It only offers to the
  batcher (`memberDeleteOffer`, see the batcher bullet): it never calls `deletionAct` or `deletionSchedule`, never writes
  to the database and never calls Telegram itself, and group `-5` and group `-2` gain no handler. Its order is cheapest
  first: a chat that is a group or supergroup; a `pinned_message` service message first of all, which drops the pinned
  message's pending self-destruct row (`memberDeletePinned`, the chat taken from the service message, never the nested
  object) and returns; the sender's shape through `raidFloodSenderCounts` (a `via_bot` message counts here, unlike for edits)
  and not a bot; `memberContentMessage` (false for every message `deletionServiceKindOf` recognises and for the creation and
  migration fields); the stored `member:selfdestruct` delay through `deletionRulesLookup` (only one of the six presets
  counts, any other stored value is Off, an unreadable rule queues nothing); `memberDeletionExempt` (D-33: approved, an
  approvals read that failed, an administrator and an unknown admin status are all exempt, so a read that fails keeps the
  message); then the offer of an entry with `SentAt` the message's own date and the stored delay. There is no captcha lookup
  in it, because group `-10` ends a pending member's update before this group runs (while the captcha is enabled).
- The `MemberRequirements` module (priority 241, `memberreq.go`, `memberreq_watch.go`) registers one message watcher at
  group `-2` (`memberReqWatchGroup`), through `handlers.NewMessage` with a supergroup-only filter and no `SetAllow*`
  (new messages only: no edits, channel posts or bot senders), the only message handler of that group. The member
  requirements watcher returns `ext.ContinueGroups` on every path, its result is a named value preset to it so a
  recovered panic cannot return `nil` (which would leave group 0 and kill every later group-0 handler), and it makes no
  Telegram call and no database write for a clean member beyond the pass-guarded clean predicate. Its order is cheapest
  first: the switch `memberReqEnforcementLive`, the item (`raidFloodItemKey`) and the sender's shape
  (`raidFloodSenderCounts`: no `sender_chat`, automatic forward, service account or the bot), the cached settings
  (`memberReqSettingsLookup`; an error or no rule on ends it, so an unreadable setting punishes nobody), the pure name
  rules, then for a clean member the pass value and the clean predicate and for a violator the cached admin list
  (`cachedAdminStatus`: a known admin ends it; an unknown cache goes to the job, whose live `getChatMember` decides).
  For a member with clean names in a group with a lookup rule on (the photo or the channel rule, action not Off; the AI
  rule's part is described after this one) the
  watcher reads the cached lookup verdicts first (`memberReqLookupCached`, Redis only and still no Telegram call): one
  pipelined round of single-key `GET`s of the pass key, `fuku:memberreq:photo:<user>` (photo rule on) and
  `fuku:memberreq:lc:<chat>` (channel rule on), then `fuku:memberreq:sub:<channel>:<user>` only when `lc` holds a channel
  ID (`lc` reading `0`, a group with no linked channel, is nothing to verify), and `fuku:memberreq:cna:<channel>` after it
  only when the subscription key is missing (the channel-level not-administrator fact below: the rule is inert while it is
  set, so the channel part counts as satisfied). The member is clean only when every lookup
  rule that is on has its cached positive (or the channel is inert), and then the Phase 13 clean path runs with the pass answer already read; the
  pass key is read in that same round but is never consulted without the lookup positives and never stands in for one.
  A miss, a Redis error, no Redis and an `lc` it cannot read are all "not satisfied", and then the watcher starts no job
  for a member the cached admin list knows as an administrator, for an approved member or when the approvals read fails
  (`memberReqMissExempt`, D-25: an exempt member is exempt inside the job and so never gets a positive cached, and would
  cost a job per message for ever), and otherwise hands exactly one job to the runner (below). The watcher writes no
  verdict of any kind (it never caches anything about a member, and the lookups cache only a pass, never an inert or
  unknown verdict; the one other thing they cache is the channel-level `cna` fact below, a fact about the channel and the
  bot and never about a member) and never classifies; a group with name rules only keeps its single `GET` exactly as before.
  While the AI spam-name rule is live (`memberReqAILive`: its action is not Off and the process is configured; an
  unconfigured stored value counts as Off there, or every message of every member would start a job for ever) the same
  round also reads `fuku:memberreq:nv:<verdict key hash>`, `fuku:memberreq:nm:<user>` and `fuku:memberreq:nu:<chat>:<user>`
  (`memberReqAIWatchQueue`: the canonical names and the verdict key come from pure Go, with `config.AppConfig.AnthropicModel`),
  and the AI part is satisfied when the verdict parses and is below the bar, or when the marker is absent and the baseline
  equals the current names hash (`memberReqAIWatchSatisfied`, pure: a miss, an unparsable value, NaN, an infinity and any
  Redis error are not satisfied); names that are not judgeable are satisfied with no key read, and everything else is one job
  after `memberReqMissExempt`. `memberReqLookupCached` takes the names for this and its complexity is held at 18 or less by
  keeping the AI part in those two small functions. The watcher writes no verdict, baseline or marker and calls nobody.
  While the AI breaker is paused for the chat (`memberReqAIPaused`: in memory and pure, the provider-wide pause or the group's
  own) or the chat is latched (`memberReqAILatched`: the refusal latch below, in memory and pure) the watcher consults them
  (Phase 15 review WR-04 and security SEC-02, `memberReqAIOnlyPaused`): when the AI part is the ONLY reason the member
  is not satisfied (one more Redis-only read, `memberReqLookupCachedWith` with the AI rule left out, only while paused or
  latched) it starts no job and runs no clean path and returns `ext.ContinueGroups` having done nothing, so a marker holder
  costs no job and no paced live `getChatMember` per message during a pause or a latch, and the marker stays, so the first
  message after it is judged. It must not treat the AI part as "satisfied": that would take the clean path
  (`ClearHoldOnClean`) and delete the hold of a member held for an AI breach. Any other open reason (a name rule, the photo or
  channel rule) still hands a job as before (`TestMemberReqWatcherAIPaused`, `TestMemberReqWatcherAILatch`). While paused or
  latched a member whose string has a cached breach is not acted on either (the job that would punish from the cache is not
  started), and is judged by the first message after the pause or latch: the safe direction, as for the pause. The
  baseline rule is the same in both halves (SEC-01): a member whose baseline equals their names and who has no marker is
  satisfied in the watcher and passes in the job even when another account's judgement of the same string has since cached a
  breach, because the baseline is the bot's record that these names were accepted for this member (it errs toward not
  punishing, and members already in the group are checked only after a name change, as the page says); a joiner, a marker
  holder, a renamer, the held release and "I fixed it" still read the cached breach.
  The enforcement switch `memberReqEnforcementLive` is an `atomic.Bool` read only through `memberReqLive()`. It is true by
  default since the "I fixed it" handler exists (the `mreq|v1` bullet under Callbacks): `memberreq.go`'s `init()` stores
  true, and that store is the only production write of it (`LoadMemberRequirements` does not write it, because a test loads
  the module after turning the switch off and a store at load would turn it back on under that test). So the watcher and
  the join entry run in every group with a rule on, and the captcha guard reads a hold on every captcha unmute, a
  hold-read error failing closed. It stays the one switch that turns enforcement, the join entry and the guard off
  everywhere. It is atomic because goroutines a test does not own read it (the dispatcher, and the captcha lifecycle
  goroutines that reach the join entry can outlive the test that started them) and `make test` runs with `-race`; tests
  write it only through `withMemberReqLive`, which restores the previous value (now true) in `t.Cleanup` and is never
  combined with `t.Parallel`. `TestMemberReqEnforcementLive` asserts the default and the registration together.
- The web captcha adds no handler group: its routes are HTTP routes on the bot's server, not dispatcher handlers.
- The Settings module (priority 239, `settings.go`) adds one handler group, `1`, for the typed-reply door below; its other entry points are `/settings`, the callbacks and the deep link. `/settings` goes through `helpers.WrapCommand`
  with `settingsDesc.RequiredChecks` of `requireSettingsAccess()` alone (`settings_gate.go`: a private chat passes at
  once, because the private list needs no group; in a group a live creator or administrator from
  `lockdownAuthorityMember`, the body of `requireLockdownAuthority(false)`, never the admin cache, then the owner gate
  below). In the bot's private chat `/settings` sends the group list (`settingsPrivateList`) as a new message in the
  user's language, built only from the sender's own ID with a database read (`settingsHubLookup`, default
  `adminindex.ListForUserFresh`) and no Telegram call; a failed read is one plain `settings_check_failed` reply. An administrator the gate refuses gets the plain public reply `settings_refuse_owner_only` to the command with
  no keyboard, and an undecided gate `settings_check_failed` (D-13); a plain member still gets the lockdown check's
  `chat_status_user_admin_cmd_error` and never an owner-only text. The command is not `Disableable`, and it re-enters for
  anonymous admins through `RegisterAnonymousAdminHandler("settings", ...)`: the tapper becomes the opener, is checked
  live and gated like anyone else (`anonPipelineHandler` re-runs `RequiredChecks`, so an anonymous owner gets the choice
  message and an anonymous gated administrator `settings_refuse_owner_only`). The `settings|` callback is registered with `callbackquery.Prefix("settings|")` and no
  `SetAllowChannel`, because a menu is never a channel message.
  `LoadSettings` ends by calling `LoadSettingsInput` (`settings_input.go`), which registers `/cancel` through
  `helpers.WrapCommand` (its one check, `settingsInputPrivateOnly`, passes only in the bot's private chat and never
  replies; it is not `Disableable`) and the typed-reply door (below). A `/cancel` sent anywhere but the bot's private
  chat is not the bot's command and is never ended: `settingsCancelPassOn`, a raw handler registered at group 0 just
  before the pipeline's `/cancel` (a second documented exception to the `WrapCommand` rule, beside the StaffActions
  interceptors; it matches through the `CheckUpdate` of the command handler `helpers.NewBotCommand` builds), takes it first, replies and writes nothing,
  makes no Telegram call and returns `nil`. In this gotgbot `nil` leaves the group and moves on to group 1 and later, while
  `ext.ContinueGroups` would only try the next handler of group 0, which is the pipeline's `/cancel` that ends the update
  (so a member's `/cancel <spam link>` would skip aispam, antiflood, locks, blacklists, filters, the log channel and the
  late raid flood counter, which now see it like any other text, and group 12 counts it as an unknown slash word). `/cancel` deletes the sender's own wait and
  marker (the keys are built from the sender's ID alone) and answers `settings_edit_cancelled`, or
  `settings_edit_cancel_nothing` when there was none, when a write holds the wait, and without Redis.
  The typed-reply door (`settings_input_door.go`, `settingsModule.settingsInput`, registered by `LoadSettingsInput` with
  `handlers.NewMessage(settingsInputIsPrivate, ...)` at group `settingsInputGroup` = 1, after the group-0 commands and
  before every group that could end a private update) reads the owner's next private message after Set. A candidate is a
  new private message whose sender is the chat itself (not the bot, not a Telegram service identity), with no
  `rich_message` and no command (`settingsInputLooksLikeCommand`: a `bot_command` entity at offset 0 or a slash followed
  by a non-space, in the text or the caption); anything else, and a sender with neither a wait nor a marker, returns
  `ext.ContinueGroups` with no reply, no write and no Telegram call, so ordinary private chat is untouched. The order is
  fixed, each step refusing before the next: `settingsInputRead` (an error or no Redis is not the door's); no wait but a
  marker is one plain `settings_edit_wait_ended_note` naming the part (the marker taken with `GETDEL`, so two deliveries
  answer once; nothing is saved); with a live wait, a message whose `media_group_id` is 1 to 20 digits passes the album gate
  (`fuku:settings:inputalbum:<user>:<media_group_id>`, `SET NX EX 30`, first wins) before the live checks and the claim, so an
  album is checked and saved once, the items that lost the gate get no reply and no Telegram call, a refused album gets one
  refusal and a late one the one wait-ended reply, and a Redis error ends the update with no write; the live `getChatMember` of the sender, the owner gate (`settingsGate`), the right of
  the part (`settingsRightRefusalFor`, and `chat_status.IsBotAdmin` on Rules), a live `getChat` of the group, the
  stored state, `settingsEditCheck`, `settingsInputClaim`, the part's apply function, `settingsInputFinish`. A lost
  authority ends the wait and answers one plain reply (a non-admin status or a Telegram 4xx other than 429 at
  `getChatMember` or `getChat`, and a chat that is not a group or supergroup: `settings_edit_refuse_access_lost`; the gate
  refusing an administrator: `settings_gate_owner_only`; a lost right: `settings_no_change_info` or
  `chat_status_bot_not_admin`; every reason except `settings_edit_refuse_access_lost`, whose text says it, is followed by
  a blank line and `settings_edit_wait_stopped`, so the person knows the next message is not read). The Rules bot check
  proceeds only when `chat_status.IsBotAdmin` is true, as the command does; because that predicate returns a bare bool,
  a false is classified by one live `getChatMember` of the bot (`botAdmin`): a definite non-administrator status or a
  Telegram 4xx other than 429 ends the wait with `chat_status_bot_not_admin`, while a 429, a 5xx, a transport error, a
  timeout, or a live answer that says the bot is an administrator keeps it with `settings_check_failed`. An answer that cannot be established (a 429, a 5xx, a transport error, a timeout,
  an undecided gate, an unreadable page) answers `settings_check_failed` and keeps the wait. A message the check refuses
  is a reply with the reason, a blank line, `settings_edit_still_waiting` and one Cancel button, and the wait is neither
  claimed nor extended (its message ID is noted on the wait by `settingsInputNoteRefusal`, so that Cancel button ends this
  wait); a write that fails after the claim releases the claim and answers the same way with
  `common_settings_save_failed`. A save sends the page as a new message in the private chat (with
  `settings_edit_saved_text`, `_media` or `_buttons`, plus `settings_edit_caption_ignored` when a caption was not used and
  `settings_edit_album_cut` when the saved message came from an album)
  and edits the prompt to `settings_edit_prompt_closed`, best effort. No log line of the door carries the message text,
  a caption, a button name, a link or a file ID. The media part stores what `/setwelcome` or `/setgoodbye` by reply stores (a GIF saved from Phase 12 on as `db.ANIMATION`, one saved earlier as a document, the caption not used, the text
  and buttons kept) and the buttons part what they store for the same lines, replacing the set, but the editor refuses a link
  that is not http or https instead of dropping it (EDIT-05). On the Rules page the three parts go through `settingsInputReply.applyRules`
  (`applyRulesText`, `applyRulesMedia`, `applyRulesButtons`), so Rules media and buttons store what the Welcome's store for the same
  message, with the same saved page, album gate and caption note. The typed reply is the sixth door of `TestSettingsGateAllDoors`.
  After a group `/settings` passes its checks and the choice message is sent, `settings()` refreshes that group's roster
  record through `refreshChatAdminIndex` (at most two Telegram calls, a 10 s context, the error only logged), the
  second repair path after `/admincache` (D-07, MENU-10). It runs after the reply, so a slow or failed refresh never
  delays or changes what the sender sees, and a gated or refused sender, who never reaches the handler, never triggers it.
- The settings menu's private entry is `RegisterDeepLinkHandler("settings_", settingsStartDeepLink)`, the Open in private
  button's `/start settings_<chat id>`; `help.go`'s `start` is not edited and the router picks the longest prefix. The
  payload is parsed strictly (`parseStaffChatIDArg`: an optional minus, digits, at most 20 characters, a negative
  result; anything else is `settings_start_invalid` with no Telegram call) and never authorizes anything. The order is
  fixed and each step refuses before the next runs: the strict parse; a live `getChatMember` of the user in that group,
  which must show the creator or an administrator (a Telegram 4xx other than 429, and any non-admin status, answer
  `settings_start_not_admin`; a 429, a 5xx, a transport error or a timeout answers `settings_check_failed`); then the
  owner gate, `settingsGate` on the member record that check returned (`settings_start_owner_only` for an administrator
  it refuses, `settings_check_failed` when it cannot decide, so a gated administrator costs no `getChat`); then one
  `getChat` of the group (a 4xx other than 429, or a chat that is not a group or supergroup, answers
  `settings_start_not_admin`; anything else that fails `settings_check_failed`). Errors are classified by
  `isTelegramClientError`, never by wording, and the deep link no longer answers `settings_cannot_open`: a plain member,
  a chat the bot is not in, a gone chat, a channel and any unrecognised 4xx wording get the same words, because the
  payload is attacker-controlled text and a different answer would tell anyone whether the bot is in chat X
  (`TestSettingsStartDeepLinkSameAnswer`). Every refusal is a plain reply with no keyboard. The Main page is sent to the
  private chat in the user's language, its buttons carrying `c` (the group's chat ID) and no opener field. A `/start`
  with extra words (`settings_ -100`) has an unexpected argument count, so `start` ignores it and the handler never runs.

## Callbacks

- Encode/decode only through `fuku/utils/callbackcodec` (`<ns>|v1|<url-encoded>`, 64-byte cap). Never `strings.Split`.
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
- The `mreq|v1` namespace is the "I fixed it" button of a requirement mute notice (`memberReqFixedCallback`, `memberreq_fixed.go`,
  registered by `LoadMemberRequirements` with `callbackquery.Prefix("mreq|")` at group 0 and no channel permission, because a
  notice is never a channel message). The fields are exactly `a=f`, `h` (the hold row ID) and `u` (the held member), parsed
  strictly before any I/O by `memberReqParseFixed` (positive whole numbers of at most 19 digits, nothing else, the message
  present and in a group or supergroup; anything else is `common_callback_invalid_request`). The data is at most 52 bytes
  (33 typically) and carries no name, rule or text. Every press runs one fixed order under `memberReqFixedTimeout` (10 s), each step
  refusing before the next: (1) the strict decode; (2) the presser must be `u`, before any read, else `mreq_fixed_not_yours`
  (an admin is refused too and lifts a mute with `/unmute`); (3) the hold read fresh through `memberreq.GetHoldByIDFresh` and
  matched by its ID, the chat and the member alone (the stored notice message ID is never compared: the message edited is
  the one the button sits on, so a tap before the ID was stored, or after storing it failed, still works), else
  `mreq_fixed_stale`; (4) one live `getChatMember` through `memberReqPaced` that must show the requirement marker
  (`isMemberReqMute` against the hold's `mute_until`: an error is `mreq_fixed_check_failed`, anything else, including an
  admin's, staff, captcha, antiflood or warn-limit mute and a member no longer restricted, is `mreq_fixed_not_ours` with no
  write, the same on every repeat); (4b) the approvals, read through the cached `memberReqApprovedLookup` seam (the tri-state
  `approvals.IsUserApprovedContext`, never a fresh database read) only after the live marker check, so only a mute the
  requirements placed can be freed this way: an error is `mreq_fixed_check_failed` with nothing changed, and an approved
  member is judged clean for every rule, the name rules included, so the order continues at the permissions step (6) with no
  settings read and no lookup; (5) every enabled rule judged again against the current settings (`memberReqFixedJudge`,
  `memberReqSettingsLookup`; a rule now Off is not checked, an unreadable setting is `mreq_fixed_check_failed`): the name
  rules (`memberReqBroken`) on the live record, and the photo and channel rules with fresh lookups
  (`memberReqLookupBroken` with `memberReqLookupMode{Fresh: true, Classify: true, Names: &liveNames}`) that bypass both
  positive caches (a stale positive is deleted on a definite breach and a positive refreshes the cache) through the lookup
  pacer under `memberReqFixedLookupBudget` (5 s) of the press's 10 s, so at least 5 s remain for the unmute. The AI rule
  (Phase 15, D-23) is judged with the live record's names and ignores `Fresh`: it answers from the verdict cache, so the press
  is deterministic for unchanged names (the cached verdict, a breach or a pass, with no request, however often the button is
  tapped) and makes one provider call only for names with no verdict (a rename), through the breaker, the gate and the
  member's per-account budget inside that 5 s; a pass writes the baseline and deletes the marker exactly as a job does, a
  breach sets the marker, and an unknown (an error, a 429, a timeout, a shed, a paused check, any budget spent, a refusal, no
  tool call, a value outside 0 to 1) is `mreq_fixed_check_failed` with nothing changed; an AI that is not configured is inert,
  not broken and not unknown. The AI row never bypasses the cache for a press: a bypass would turn the button into a re-roll of
  a model whose sampling cannot be fixed, and one lucky pass would overwrite a cached breach for every group for 24 hours. A rule
  still broken is the definite answer and beats an unknown lookup: one to four broken rules are `mreq_fixed_still_broken` naming
  exactly them, and five or more (up to eight) are `mreq_fixed_still_broken_many` with the count of broken rules whose
  punishment is not Off (`memberReqStillBrokenText`); with nothing broken, an unknown lookup is `mreq_fixed_check_failed` and changes nothing,
  like an unreadable setting. A channel lookup for the member that cannot be answered is classified by one paced
  `getChatMember(channel, bot)`: a definite non-administrator status or a 400 or 403 makes the channel rule inert (not broken,
  no answer of its own), and a 429, a shed, a 5xx, a transport error, a timeout or an administrator bot stays unknown;
  (6) and (7) `getChat` and `resolveUnmutePermissionsWithLockdown` through `memberReqUnmutePermissions`,
  both before the hold is touched, any error being `mreq_fixed_check_failed` with nothing changed; (7b) Phase 14 review
  WR-04: when the fresh lookups of step 5 went beyond the cache (a flight was started or joined, `memberReqFixedJudge`'s second
  result), the live record is read once more through `memberReqLiveMember` right before the take and must still show the marker
  (`isMemberReqMute` against the hold's `mute_until`: an admin's `/mute` that landed during the lookups replaced it and is
  answered `mreq_fixed_not_ours`, never lifted; a read that fails is `mreq_fixed_check_failed`), with nothing changed either way
  and no extra request when step 5 made none (an approved member, a name-only group, all-cached lookups). The press's worst
  case is the hold read, the live record, the lookups (capped at 5 s), the permissions, this read, the lift and the edit, inside
  its 10 s; (8) the conditional delete
  `memberreq.TakeHoldByID`, on a context that survives the press's deadline, so of two taps on two replicas one wins and the
  other gets `mreq_fixed_stale`; (9) `memberReqLiftMute` with exactly those permissions, never `MutedPermissions`, and when it
  fails `memberreq.RestoreHold` puts the row back with the same ID, notice ID and every other column and the answer is
  `mreq_fixed_check_failed`, so the same button works on the next tap; (10) the toast `mreq_fixed_done` and the notice edited
  to `mreq_notice_fixed` with no keyboard, or, when the permissions came from an active lockdown, the alert
  `mreq_fixed_done_lockdown` and `mreq_notice_fixed_lockdown`; the edited message is rescheduled for deletion only when it is
  the hold's own stored notice (the notice ID is compared for that alone, never for the unmute: SEC-01). Only a hold with `welcome_held` (a join mute) then sends the held
  welcome, once, through `SendWelcomeMessage` after the query is answered, so Greet once claims the member only then; a
  message hold sends none. The query is answered exactly once on every path, in the group's language (`ctxTr` of the group
  callback): `memberReqFixedCallback` recovers from a panic and defers a single answer, as `settingsCallback` does. Every
  Telegram call goes through `memberReqPaced`, the edit of the notice is logged by its error code only and never changes the
  outcome, and a log line carries chat, user and hold IDs only.
- The `settings|v1` namespace is the settings menu (`settings_menu.go`, `settings_pages.go`). Fields: `a` action, `c` the
  group's chat ID (a menu opened in private), `u` the opener's user ID (a menu opened in the group), `p` page (`s` staff,
  `l` lockdown, `c` captcha, `f` antiflood, `w` welcome, `g` goodbye, `r` rules, `m` member requirements, `d` deletion, `a` access), `t` control, `v` the absolute target value; `e` (confirm expiry), `r`
  (the lockdown row ID of a Lift card), `l` (the link row ID of an Unlink card) and `s` (Staff Group row ID) arrive with
  the confirm cards and the Link pick. Core action codes: `oh` open here (the choice
  message becomes the Main page), `op` open page `p` (also Refresh and a stepper's value button), `bk` Back to Main, `cl`
  Close, `tg` toggle, `st` set, `no` cancel a card back to page `p`; a page declares its own codes through
  `registerSettingsPage`, which panics at init on a duplicate page key or action code. Exactly one of `c` and `u` is
  present: `c` is accepted only on a message in a private chat and `u` only on one in a non-private chat, and each action
  takes an allow-listed field set (an unknown key is refused), all checked before any I/O. A page action may also declare `Validate`, a strict check of its own fields that runs in that same
  step 1, after the allow-list and before any I/O, and whose false answer is `common_callback_invalid_request`; a nil
  `Validate` keeps the fields allowed and not required. A `tg` or `st` press must also name a control its page registers
  (`settingsControlFieldsValid`), and a control may declare `Validate` of `v` and `Actions`, the core codes it accepts
  (`tg` and `st`; a press with any other code is refused in that same step 1, and nil keeps both); all run before any
  I/O. Steppers and toggles send the
  absolute target value (`v`), never a delta, so a double tap is harmless. Every press runs one pipeline, each step
  refusing before the next (`settingsPress.verify`): (1) decode and structure, refused with the alert
  `common_callback_invalid_request` (a missing message is `common_callback_message_unavailable`); (2) a Telegram service
  identity (`staffServiceUserIDs`) gets `lockdown_alert_post_as_yourself` before any lookup; (3) in a group menu
  `query.From.Id` must equal `u`, else `settings_not_your_menu`, with no lookup; (4) one live `getChatMember` of the presser
  in the group being configured (the message's chat for a group menu, `c` for a private one, after one `getChat` of `c`),
  creator or administrator, else `settings_not_admin`, and a failed lookup `settings_check_failed`; then (4b) the owner gate
  `settingsGate(b, groupID, query.From, member)` on that same record, before the private `getChat`: an administrator
  the gate refuses is answered the alert `settings_gate_owner_only` and an undecided gate `settings_check_failed`, each
  editing and writing nothing (D-10: a menu opened earlier is gated again on every press, Close included, so an
  administrator the owner has since shut out loses it at the next press, and deletes the stale message with Telegram's
  own delete); then (5) the right of the control. The callback is answered exactly once, only through `settingsPress.answer` (which calls
  `answerStaffCallback`; no settings file calls `query.Answer`; `settingsCallback` defers an empty answer after its
  recover, so a page that panics is recovered and still answers once), and the menu message is edited in place; Telegram's
  "message is not modified" is success, and a failed or rate-limited edit is logged once, never retried and never undoes
  a write. Close deletes the message and, when the delete fails, edits it to `settings_closed` with no keyboard. A page
  that cannot read its data is the cannot-open edit (`settings_cannot_open`, no keyboard, with the alert
  `settings_cannot_open_alert`). Every button's data is at most 64 bytes for a 14-character chat ID and a 10-digit user
  ID (a Lift or Unlink Yes also carries a row ID, which fits up to 17 digits); an encode that overflows is counted in `settingsScreen.Omitted` and logged, and a worst-case test fails on any.
  Registering a new page is one `registerSettingsPage` call from its own file's `init` plus its strings, and never edits
  `settings_pages.go` (the `Actions` field of `settingsControl` was added there once, in Phase 13). A private press makes two Telegram calls, `getChatMember` of the presser in `c` and `getChat` of
  `c`; an error of either that `chat_status.IsBotMissingError` recognises ("bot was kicked", "bot is not a member",
  "chat not found") is the cannot-open edit with its alert (`settingsPress.cannotOpen`, D-11), and every other error is
  the `settings_check_failed` alert with no edit. A `c` whose `getChat` answer is not a `group` or `supergroup` (callback data
  is not authenticated, so it can name a channel or a private chat the presser administers) is the same cannot-open edit
  with its alert and no write, as the deep link refuses it. A group menu's pipeline is unchanged.
- The Main page (`renderSettingsMain` in `settings_pages.go`, MENU-15) is a two-column grid with a status block. Each
  registered page is an "icon name" button (`a=op`, `p=<key>`, never a state in the label), two per row filled left to
  right in page order (`Order`, then `Key`), and an odd last one stays alone on a full-width row. An `OwnerOnly` page
  (`settingsMainEntry.OwnerOnly`, copied from the page) is never paired into the grid: for the live owner it gets its own
  full-width row after the last grid row, so the owner's grid and an administrator's differ by that row only, and an
  administrator's Main has neither the row nor the line. Each page's state is one `settings_main_state_line` (icon, page
  name and the page's `MainState`, or the unknown mark `settingsUnknownState` when it could not be read) in a status
  block of the body, in the grid's order with the owner-only line last, after the hint (and the owner line in a group
  menu) and one blank line; with no visible page there is no status block and no blank line, only the hint and the
  navigation row. The states are spliced as the pages return them and never name the Staff Group. The navigation row is
  `settingsKeyboard.navRow`: a private Main ends with Groups (`settingsHubGroupsButton`, the `shub|v1` press `a=lp&v=1`,
  at most 16 bytes, which `settingsHubCallback` answers by redrawing the list at page 1 in the same message; the page the
  user came from is not remembered) and Close, a group Main with Close alone, and every other page, Access included, with
  Back and Close. A page added by a later phase joins the grid by its `Order` with no edit to the renderer.
- The private group list and the pick (`settings_hub.go`, MENU-09, MENU-13). The list is a message in the bot's private
  chat: the header with the Groups page name, `settings_list_intro`, `settings_list_hint` (it names `/admincache` and
  `/settings` in the group, the two repair paths), a `Page N/M` line only when there is more than one page, one
  full-width button per group, a paging row and Close. Zero groups is the one neutral text `settings_list_empty` plus the
  hint with Close only, byte-identical for a stranger, a gated administrator and an owner with no recorded group; one
  group is still a one-button list (D-03). Rows are the plain title (a button label is plain text that Telegram never
  parses, so it is not HTML-escaped; line breaks and runs of spaces collapse to one space, trimmed, cut at 32 code
  points with an ellipsis appended, the chat ID when the title is empty), sorted by the lower-cased collapsed title in
  code point order, ties by chat ID ascending, 8 a page (`settingsHubSort`, `settingsHubLabel`); a user with more than
  999 pages (7,992 groups) sees the last page as the limit. A gated administrator's group is not listed
  (`adminindex.ListForUserFresh`: a creator row, or an administrator row with the switch on or no creator row at all). The
  message text names no group. The list's own buttons are the `shub|v1` namespace (registered with
  `callbackquery.Prefix("shub|")` and no `SetAllowChannel`, no handler group): `lp` with exactly `a` and `v` (a whole
  number of 1 to 3 digits; 0 and anything else is `common_callback_invalid_request`, a number past the last page shows the
  last page) and `cx` with exactly `a` (Close, deleting the message or editing it to `settings_closed` when the delete
  fails). `settingsHubCallback` accepts a press only on a message in a private chat, refuses a Telegram service identity
  (`staffServiceUserIDs`) with `lockdown_alert_post_as_yourself` before any read, rebuilds the list from `query.From.Id`
  and never from the callback data, so a replayed or forwarded list button shows the presser their own groups, and
  answers the query exactly once (a failed read is the alert `settings_check_failed` with no edit). The roster is a hint,
  never authority. Every button's data is at most 64 bytes (a pick for `-1001234567890` is 33, a paging button 18, Close
  12) and carries no title, name or Staff Group.
- The pick is the core action `pk` of `settings|v1` (`settingsActPick`, exactly `a` and `c`, private only, no page may
  register it). It runs the whole press pipeline, so the live `getChatMember`, the owner gate (step 4b) and the one
  `getChat` all run before anything is shown, and on success edits the list message in place into that group's Main page
  (the list message is never deleted and never turned into the cannot-open edit). Because `c` is attacker-controlled,
  the pick overrides two mappings of the other actions (`settingsPress.verify`): every non-admin status and every
  Telegram client error (`isTelegramClientError`, any 4xx other than 429, by class and never by wording) at the member
  lookup or at `getChat`, and a chat that is not a group or supergroup, answer the alert `settings_pick_unavailable`
  with the list left exactly as drawn, nothing written and one answer, whether or not the bot is in that chat
  (`TestSettingsPickSameAnswer`); a 429, a 5xx, a transport error or a timeout answers `settings_check_failed`; a proven
  administrator the gate refuses gets `settings_gate_owner_only`, and an undecided gate `settings_check_failed`. The pick
  is the fifth door of `TestSettingsGateAllDoors`. Every other private action keeps the step-4 mapping above, so a forged
  `c` on `op` or `tg` can still meet the cannot-open edit (known note: the class rule is scoped to the pick and the deep
  link, research Pitfall 7).
- The Staff Group page (`settings_staff.go`, page `s`) declares four actions: `lk` Link (with an optional `s`), `ua` Unlink
  ask, `uy` Unlink Yes (with `e` and `l`) and `pg` (with `v`, the page of the read-only panel, parsed by `settingsParseNumber`
  and clamped; a value that is not a number is an invalid request). `s` is the staff_groups row ID, never a chat ID:
  a pick lists the presser's own Staff Groups again (`staff.ListStaffGroupsByOwner`), takes the row whose ID equals `s`
  (never a position, so a list that was reordered since it was drawn cannot link another group), and only then makes
  `resolveNamedStaffGroup`'s live creator check. An `s` that is not a positive number of at most 10 digits is an invalid
  request; one that names no row of the presser is refused with `staff_link_refuse_not_staff_owner` and links nothing.
  Link without `s` makes `/linkstaff`'s own decision over the same live list (`decideOwnStaffGroup`): none is
  `settings_staff_no_group`, one match with a group that could not be verified is `staff_check_failed` (never a one-button
  picker), one clean match links at once, and several make the picker, which lists at most 8 and adds
  `settings_staff_pick_more` (how many are missing and that `/linkstaff <id>` links any of them) when there are more.
  The Unlink card is the shared two-tap card of `settings_card.go`: `settingsCardRows` adds `[Yes | No]` and the
  navigation row, Yes carries `e` (unix minutes, `settingsCardLifetime` 5 minutes ahead, a variable only for tests), expiry
  is lazy (`settingsCardExpired`: a Yes whose minute is past redraws the page with `settings_card_expired` and runs
  nothing, an `e` that is not digits only, positive and 63-bit is an invalid request), there is no timer and no Redis
  key, and Yes goes through the whole press pipeline and then the action's own live checks (`runUnlinkGroup` checks the
  creator of both groups again). A card binds the object it was drawn for (D-33): Yes carries `l`, the `staff_group_links`
  row ID, parsed by `settingsCardObjectID` (a positive 63-bit number of at most 19 digits, else an invalid request
  answered before any lookup, like `e`), and after the live authority checks the link current for the group is read and
  a link under another ID (the card's was unlinked and the group linked again, perhaps to another Staff Group) is
  answered `staff_unlink_not_linked` with nothing removed. The Lockdown page's Lift card uses the same helper.
- The Lockdown page (`settings_lockdown.go`, page `l`) declares two more actions besides its trigger controls: `ls` Lift
  ask and `ly` Lift Yes (with `e` and `r`, the `chat_lockdowns` row ID the card was drawn for, parsed like `l`). The Lift row is the first content row, drawn only while the chat's lockdown is
  active (`models.LockdownStateActive`, a confirmed one or not); a lockdown being lifted, or none, shows no Lift. Both
  actions declare `settingsRightRestrict`, so each press judges a live `getChatMember` record at the ask and again at
  Yes (a demotion in between is refused with the alert `settings_no_restrict` and edits nothing). The ask reads the
  lockdown fresh (`lockdownCurrentLookup`): none, one being lifted or an unreadable state redraws the page with the
  command's own text (`lockdown_not_active`, `lockdown_lift_in_progress`, `lockdown_state_failed`) and draws no card;
  an active one draws the card `lockdown_alert_lift_ask` (`renderSettingsLockdownLiftCard`, the stored group title
  escaped and spliced in after translation). Yes goes through `settingsCardExpired` (an expired minute redraws with
  `settings_card_expired` and lifts nothing), answers the press empty first, then lifts only through
  `applyLockdownLiftOf` (`applyLockdownLift` for one confirmed row: it reads the chat's current lockdown once, and a row
  under another ID than `r` is `liftApplyLost`, `lockdown_already_lifted`, with nothing lifted, so a stale Yes can never
  lift a newer lockdown); the menu never calls `setChatPermissions`, `BeginLift` or `FinishLift` itself. Every outcome redraws the page with the command's words as a notice
  (`lockdownLiftApplyText`), and a recorded lift redraws it before anything is announced (the announcement waits for a
  pacer slot, up to a minute, and the card with its live Yes must not stay on screen meanwhile). A lift from a private
  menu is then announced through `lockdownAnnounceLift`; a lift from a menu opened in the group only wakes the worker,
  because the redrawn menu already says it in that same chat and a second public message would say it twice. The page
  never starts a lockdown.
- The Access page (`settings_access.go`, page `a`, `OwnerOnly`) is the owner switch "Admins can change settings". It
  declares control `s` and the page action `ay` (Yes, with `e`), both with `settingsRightOwner`. An `OwnerOnly` page is
  drawn and opened only for the live creator: `settingsMainEntries` skips it without calling its `MainState` unless the
  viewer is the owner, and `openPage` refuses it for anyone else with the alert `settings_access_owner_only`, editing
  nothing, so a forged `op`, `no` or Refresh naming page `a` is refused too. `settingsView.Owner` (the press's own live
  member record is the creator, in a group menu and in a private one, and in the deep link) is distinct from `Creator`
  (a private menu of the creator, which gates only the Staff Group title, D-32). The toggle with `v=1` (`tg` and `st`
  alike) draws the shared two-tap card (`settings_access_confirm`) and writes nothing; Yes runs `settingsCardExpired`
  (an expired minute redraws the page with `settings_card_expired`, a bad `e` is an invalid request) and writes ON
  after the whole press pipeline, so a Yes by someone who is no longer the live creator is refused; `v=0` writes OFF at
  once, and any other `v` is an invalid request. Both writes go through `applyAdminsAllowed`, which refuses a status
  other than creator (`accessRefusedNotOwner`, behind the right), writes through the `settingsAccessWrite` seam
  (`menuaccess.SetAdminsAllowed`) and returns the value to draw: the requested one after a write, the unchanged one
  after a failure, which redraws the page with `common_settings_save_failed`. The page and the gate read the switch
  through the same fresh `settingsAccessLookup` seam, there is no cache key to invalidate, an unreadable switch is the
  cannot-open edit on the page and the unknown mark on Main, and a flip is announced nowhere (no group, Staff Group or
  log channel message, D-11).
- The Member requirements page (`settings_mreq.go`, page `m`, `Order` 80, icon 🛂, `settings_page_mreq`, between Rules and
  Access) registers from its own file through `registerSettingsPage`, so `settings_pages.go` is not edited. It reads
  through `memberReqSettingsLookup` (`memberreq.GetSettings`, the cached `member_requirements:<chat>` key, the one read
  that the page, Main and the message watcher share; the key is in `skipLocal`).
  Main's state is Off when every rule is Off (whatever the two switches say), `{n} on` for the n rules that are not Off
  (n up to 8), and the unknown mark when the read failed; a chat with no row draws every rule Off and both switches Off, and a read
  creates nothing. The body is the intro, the all-off line while every rule is Off, one line per rule of `memberReqRules`
  in table order (icon from `memberReqActionIcon` plus the option word), the Check on join and Delete messages lines, the
  cannot-work block (below) and the six hints (`settings_mreq_hint_lookup` follows the blank-name hint); a failed read is
  the cannot-open edit. The keyboard is the rule grid, the switch row and the navigation
  row, six rows in a private and in a group menu (the rule grid's four rows of widths 2, 2, 2, 2, the switch row and the
  navigation row, `TestSettingsMreqRowBudget`) and no Open in private row. The rule grid
  is one cell per rule of `memberReqRules` in rule order, eight cells (`ln un ru ar zh ph ch ai`), two per row (four full
  rows, no odd last cell), each the rule's
  punishment icon (`memberReqActionIcon`) and short name (the `CellKey` of the table); a cell presses `a=mo&t=<code>`
  (`ln un ru ar zh ph ch ai`; the new codes are two letters like the old ones, so no callback grows: settings buttons stay at
  most 53 bytes), the page action `mo` (exactly `t`, a rule code, right `settingsRightAdmin`), which opens the rule
  screen (`renderSettingsMreqRuleScreen`, pure): the rule's name, what breaks it, the rule's own hint right after it when its
  table row has a `HintKey` (the blank last name hint on the last-name rule, `settings_mreq_hint_photo` on the photo rule,
  which recommends Warn or Mute and hides no punishment; no renderer branches on a rule's identity for it), the status slot
  (below), `Punishment: icon word`, the two hints and the choice rows `[Off | Warn]` and
  `[Kick | Ban | Mute]` with `✅ ` on the stored value, then Back to the page (`a=op&p=m`, the one Back that does not
  open Main) and Close. The rule controls `ln un ru ar zh ph ch ai` accept `st` only (`Actions: []string{"st"}`, so a `tg` on one
  is an invalid request in step 1) with `v` exactly one digit 0..4 and right `settingsRightRestrict`. Off and Warn are
  written at once through `applyMemberReqAction(ctx, chatID, rule, action)` (in `memberreq_settings.go`: the rule and the
  value 0..4 are checked before any write, then the `memberReqWrites.Action` column-targeted write, a Go 0 written, the
  cache key dropped by the repository, the row read back); Kick, Ban and Mute write nothing and draw the shared card
  (`renderSettingsMreqCard`, `settingsCardRows`: the question with the rule's name, a line saying the rule acts on every
  member at their next message, members already in the group included, `[Yes | No]`, then Back to Main and Close; a rule
  with `ConfirmKeys` in its table row, the AI rule, says its own scope instead, below). Yes is
  the page action `my` (`settingsActMreqYes`, `a=my&t&v&e`: exactly `t`, `v` 2..4 and `e`, right `settingsRightRestrict`, so a Yes runs the whole press
  pipeline again and a presser demoted since the first tap is refused with `settings_no_restrict`; a Yes can never carry
  Off or Warn), an expired Yes writes nothing, and the four exits are asymmetric on purpose: Yes and an expired Yes redraw
  the rule screen (the expired one with `settings_card_expired`), No redraws the page, Back opens Main. A press of the
  value already stored writes nothing and draws no card, and "already stored" is judged by a fresh database read
  (`settingsMreqFresh`, `memberreq.GetSettingsFresh`), never the cache, because another replica's in-process layer can be
  `CACHE_LOCAL_TTL` behind (a failed fresh read redraws the rule screen with `common_settings_save_failed`). The
  switches are controls `j` Check on
  join and `d` Delete messages: `tg` or `st`, `v` exactly 0 or 1 (`settingsEditSwitchValue` as the control's `Validate`,
  so anything else is an invalid request before any lookup), right `settingsRightRestrict`, written through
  `applyMemberReqJoin` and `applyMemberReqDelete` in `memberreq_settings.go` (beside the requirements code, because there
  is no command: column-targeted write through the `memberReqWrites` seam, a Go `false` written, the cache key dropped by
  the repository, the row read back with `memberreq.GetSettingsFresh`). A failed write redraws the stored state with
  `common_settings_save_failed`. Drift and authority tests stand in for a parity test (`TestSettingsDriftMreq`,
  `TestSettingsAuthorityMreq`). No screen of the page names the Staff Group, a member or a count of held members
  (`TestSettingsStaffPrivacy` renders it in 8 languages).
  The status block (Phase 14, MREQ-10, D-15, D-08): the page says in words when a rule cannot work here, and never draws an
  error. The page body's cannot-work block is a list of lines between the switch lines and the hints, omitted together
  with its blank line when it has no line (`settingsMreqHereLines`, so a later rule appends its line with no renderer
  change): the basic-group line (`settings_mreq_here_basic`, `🚫 `, one line, never one per rule) while at least one rule is
  not Off and the group is a basic group, else in a supergroup, while the channel rule is not Off, the no-linked-channel
  line (`🚫 `), the bot-not-an-administrator line (`🚫 `) or the could-not-check line (the page's unknown mark), each with
  `{rule}` the translated channel rule name; the icon is a code-side prefix, never inside a template, and no line names the
  linked channel's title or ID, the Staff Group or a member. Each rule screen has a status slot, one line between its
  description (and hint) and the blank before `Punishment:`: the basic-group line on every rule, the channel rule's line on
  its own screen (always, whatever the stored value), nothing for the other rules and nothing for a linked channel whose bot is
  an administrator. While the channel rule cannot be linked (a basic group, or no linked channel found) its screen is
  `[Off]` alone, two rows, the choose and actions hints omitted, and a stored non-Off value stays visible and inert
  (`settingsMreqOffOnly`); every other state keeps the three-row shape (the could-not-check state keeps both choice rows).
  The AI line (Phase 15, MREQ-10, D-14, UI-SPEC M15 to M17): `settingsMreqStatus.AINotConfigured` is the process fact that
  the AI name check cannot run (`memberReqAIConfigured` false: `ENABLE_AISPAM` off or no Anthropic key), read through the
  table field `memberReqRule.Configured` from `config.AppConfig` alone, so it makes no Telegram call, no Redis command and
  no network call in any group type, and `settingsMreqReadStatus` sets it at every return (a wrapper over
  `settingsMreqReadChannelStatus`, so the basic-group return and the page-scope return carry it too); its zero value claims
  nothing. A rule's table row also carries `UnconfiguredKey` (the line's key, taking `{rule}`) and `ConfirmKeys`, so no
  renderer or gate tests the AI rule's identity (`settingsMreqRuleUnconfigured(rule, status)` is true when the row has a
  `Configured` test and the status says it fails; a later rule with a process-level precondition adds its own field and one
  line there). The line is the code-side `🚫 ` plus `settings_mreq_here_ai` with the translated rule name, never an error
  text, a key, a variable, a model or a provider. `settingsMreqHereLines` builds the group-level lines first (the
  basic-group line, or in a supergroup the channel rule's line) and then appends the line of each rule that cannot work
  while its action is not Off, so the page body can hold two lines; the basic-group line is still one line. The AI rule's
  screen has one status slot with exactly one line: while the AI is not configured the AI line, whatever the stored value
  is, and it wins over the basic-group line (it explains the Off-only form); every other rule's screen keeps the basic-group
  line. While the AI is not configured the AI screen is the Off-only form in every group type (`[Off]` alone, two rows,
  the choose and actions hints omitted, a stored non-Off value visible and inert, the rule's own hint
  `settings_mreq_hint_ai` still shown: its Warn or Mute advice is intentionally not actionable there until the AI is
  configured); configured, it keeps the three-row shape. A Redis outage, a paused breaker and a failing provider are never
  a line (D-04, M22). The AI card says its own consequence (`settings_mreq_confirm_ai_kick`, `_ban`, `_mute`, from
  `ConfirmKeys`): the generic sentence "members already in the group included" would be false, because the AI rule judges an
  established member only after a name change; it also says a new member is checked "when they join or at their first
  message" (security SEC-04: with Check on join off a new member is judged at their first message), in all 8 locales.
  `TestSettingsMreqAIStatus` proves the six combinations of the contract's
  composition table in both menus, and `TestSettingsMreqOpenCost` that the AI screen costs what a name rule's screen costs.
  The one reader is `settingsMreqReadStatus(p, row, scope, fresh)`, and every draw of the page and of a rule screen goes
  through `settingsMreqDrawPage` or `settingsMreqDrawRule`: the database is read first (a failed read is the cannot-open
  edit), the press is answered empty, then the status is read, then the edit is made, so a redraw after a switch, a
  choice, a card Yes or an expired Yes keeps its line and the callback is answered exactly once on every path. The read
  makes no call for Main, for every rule screen but the channel's (they need only the basic-group flag, read from
  `p.group.Type`, which both menus already hold), for a page whose channel rule is Off and for a basic group; otherwise it
  makes at most two calls under one `settingsMreqHereTimeout` (3 s, a variable) through the lookup pacer and never the
  Phase 13 pacer: `memberReqLinkedChannel` (a cached link costs none) and, for a linked channel only,
  `memberReqBotChannelStatus` (a cached administrator costs none). The page and a redraw read through the caches, so an
  owner who has just linked a channel can see "no linked channel" on the page for up to 10 minutes; opening the channel
  screen (`mo`) reads fresh and refreshes `lc` and `ba`. The status is judged by the one classifier of the lookup layer: a
  definite non-administrator status of the bot, or a 400 or 403, is "not an administrator", and everything else (a shed
  pacer, a timeout, a 429, a 5xx, an unreadable or null answer) is unknown and drawn as the unknown mark, never as an error
  text, a refusal or the cannot-open edit. Known and accepted: the private press's own `getChat` already carries the linked
  chat but is not reused (one code path for both menus), so a private draw makes one more `getChat` than a group draw.
  `TestSettingsMreqChannelStatus`, `TestSettingsMreqChannelStatusScreen`, `TestSettingsMreqBasicGroup`,
  `TestSettingsMreqOpenCost` and `TestSettingsMreqRedrawKeepsStatus` hold it.
  The live link check of a punishing channel press (Phase 14, MREQ-12, D-05, D-06, D-08, `settingsMreqChannelGate`): the
  screen offers a punishment only to a group that can be linked, and this check enforces it against a stale screen, a
  stale card and a forged button. It applies to the channel rule alone and to a value other than Off (an `st` with `v` 1 to
  4 and the card's Yes `my`), and runs after every other step, in this order: the whole press pipeline (strict decode,
  service identity, opener, live `getChatMember`, owner gate, right), for a Yes the expiry check (an expired Yes redraws
  the rule screen with `settings_card_expired` and makes no link check), for `st` the fresh same-value database read (a
  press of the stored value writes nothing, draws no card and makes no link check), then one fresh `getChat` of the group
  through the lookup pacer under `settingsMreqHereTimeout` (3 s), which refreshes `fuku:memberreq:lc:<chat>`. It runs
  before any answer, because its alert is the press's one answer. A basic group (read from `p.group.Type`, no lookup) or a
  `getChat` that carries no linked channel answers the alert `settings_mreq_here_nolink` (the status line's own key, with
  `{rule}`, no icon), writes nothing, draws no card (a Yes replaces its card) and redraws the channel screen as the
  Off-only form, so the stale screen is repaired. A link that cannot be read (an error, a timeout, a shed pacer, a null
  answer) answers `mreq_fixed_check_failed`, writes nothing and edits nothing (fail closed, UI-SPEC P14-A4). A linked
  channel proceeds exactly as for the other rules: Warn writes, Kick, Ban and Mute draw the card, a Yes writes. The bot not
  being an administrator of the linked channel never refuses a press (the punishment can be set and the check fails open),
  Off (`v=0`) never makes the check so an administrator can always clear a stored value, and the link is only ever the
  live `getChat` answer, never callback data or the update. The photo rule and the five name rules have no link check.
  The press gate of the AI rule (Phase 15, MREQ-10, D-14, `settingsMreqAIGate`): the Off-only form offers Off alone, so a
  punishing press reaches the bot only from a stale screen, a stale card or a forged button, and it is refused by a pure
  check on a rule whose table row has a `Configured` test (the AI rule only) and an action other than Off (`st` with `v` 1 to
  4 and the card's Yes). It is called where `settingsMreqChannelGate` is, after the whole press pipeline, for a Yes the
  expiry check (an expired Yes redraws with `settings_card_expired` and makes no check) and for `st` the fresh same-value
  read (a press of the stored value makes no check); `Configured()` is read live, so a press has no status. Not configured
  answers the alert `settings_mreq_here_ai` (the status line's own key with `{rule}`, no icon) as the press's one answer,
  writes nothing (no repository call, no cache write), draws no card (a Yes replaces its card) and redraws the rule screen as
  the Off-only form from the stored row (`settingsMreqRedrawRefused`, shared with the channel gate; no notice line, the
  redrawn 🚫 line already says it). There is no unknown branch (the fact is process-level and always decided), no Redis
  read, no group-type read and no network call, and Off (`v=0`) never makes the check, so an administrator can always clear a
  stored value. A screen drawn Off-only still takes its single `[Off]` once the AI is configured and the next draw shows three
  rows. `TestSettingsMreqAIGate` holds it, with the stale-screen cases in both directions.
  `TestSettingsMreqChannelRefusal` and `TestSettingsMreqForgedFields` hold it, and the table-driven page `m` tests
  (`TestSettingsMreqCard`, `TestSettingsAuthorityMreq`, `TestSettingsDriftMreq`, `TestSettingsControlActionAllowList`,
  `TestSettingsMreqRuleScreen`) cover the channel rule's punishing values through a linked-channel fixture
  (`settingsMreqLinkChannel`).
- The Deletion page (`settings_deletion.go`, page `d`, `Order` 85, icon 🧹, `settings_page_del`, between Member requirements and
  Access, Phase 16) registers from its own file. Its one framework hook is the optional `settingsPage.Hidden func(group
  *gotgbot.Chat) bool` (D-28, the only edit to `settings_pages.go` and its matching guard in `openPage`): `settingsMainEntries`
  skips a hidden page without calling its `MainState`, and `openPage` refuses it with `common_callback_invalid_request`, so
  the page is not drawn or opened in a Staff Group (D-18). `Hidden` is a fresh Staff Group read (`deletionStaffLookup`,
  true only for a definite row; an error hides nothing), `MainState` makes the same read and answers the unknown mark on an
  error, and `Open` is the cannot-open edit for an unreadable row. Three levels, all built from two tables (the sections
  `settingsDelSections`, `s` Service messages, `b` Bot messages and, since Phase 17, `m` Member messages, icon 💬, and the
  row tables of `deletion_kinds.go`; nothing branches on a row's code or kind): the hub (`a=op&p=d`, Back is Main), a section's list (`a=dl&t=<section>`, Back is
  `a=op&p=d`) and a row's screen (`a=dr&t=<row>`, Back is `a=dl&t=<its section>`). The row codes are `sj sl sp sh sn sv sf
  sb sa sk sg so` (service), `bw bg br ba bm` (bot) and `mc me ms` (member: Commands, Edited messages, Self-destruct).
  `dl` and `dr` take exactly `t` (a section or a row code, checked in
  step 1) and `settingsRightAdmin`; the controls are one per row code, `st` only (a `tg` naming one is refused in step 1),
  `p=d`, with `v` exactly `n` (Keep on a service row, Off on a bot row) or the canonical decimal seconds of one of that
  row's own presets (service 0, 10, 60, 600, 3600, 21600, 86400; bot 60, 600, 3600, 21600, 86400; Commands 0, Edited
  messages 30, 60, 300, 900, 3600, Self-destruct 60, 300, 600, 3600, 21600, 86400: no sign, space, non-ASCII digit or
  leading zero), right `settingsRightChangeInfo`, judged on the press's fresh member record. Every choice of a Phase 16
  row, Commands and Edited messages writes at once, with no card and no `e` field (M28, `settingsActCancel` is never
  produced by them; the one card is Self-destruct's, below): a press of the stored state writes nothing, a failed write redraws the stored state with `common_settings_save_failed`. A row writes through
  `applyDeleteRule`; joined and left write the shared flag through `applyCleanServiceRule` (Keep, Delete now, After N), as
  `/cleanservice` does, and are drawn by their effective state. One function (`settingsDelStateOf`, fed one
  `settingsDelWords` struct: the none, zero and after keys of the row) maps a stored delay to
  the icon (➖ none, 🗑 delete now, ⏳ a delay), the word and the marked choice; a stored 0 on a bot row reads Delete now and
  marks nothing, a delay that is no preset reads `{n} s` and marks nothing. Every deletion screen (hub, both lists, every
  row) carries one status line about the bot's right to delete, from one live `getChatMember` of the bot per draw through
  `deletionReadBotRight` and the deletion pacer under `settingsDelHereTimeout` (3 s), made after the press is answered
  empty and before the edit (so a redraw after a write reads once too): nothing when the bot can delete (an administrator's
  flag is trusted outside basic groups only), `🚫 settings_del_no_right` for a definite cannot, `❔
  settings_del_right_unknown` for anything that could not be established, never an error text, a refusal or the
  cannot-open edit, and no button is ever disabled (a rule can be set with the line showing). Main makes no Telegram call.
  `dl`, `dr` and every control run `settingsDelStaffGate` first, after the press pipeline and before any read or write
  (a Staff Group answers the generic invalid-request alert and writes nothing, an unreadable row is the cannot-open edit).
  The Member messages section (`m`, Phase 17, 17-09) is table entries only (`settings_pages.go`, `settings_menu.go` and
  `settings_card.go` are not edited). Its hub cell is the odd last one alone on its row, so the hub is three state lines and
  rows of widths `[2 1 2]` (the grid, then Back and Close, which opens Main), and its list (`a=dl&t=m`, no list hint) is
  `[2 1 2]` too. The member rows set optional presentation fields on `deletionRow` (M31), empty for every Phase 16 row, which
  renders byte-identically (`TestSettingsDeletionPhase16ScreensUnchanged`, digests recorded before the renderer changed and
  never re-recorded): `ChoiceRows` (the row's own choice layout over the none choice and the presets: Commands `{2}` over
  `[Off, On]`, Edited messages `{3,3}`, Self-destruct `{1,3,3}`), `LabelKeys` (the 0 of Commands is labelled On), `ZeroKey`
  (a stored 0 reads `🗑 On` on Commands, else Delete now), `AfterKey` (Edited messages reads `⏳ Grace {delay}`, else After
  {delay}), `TimeKey` and `NoTime` (the time line of a row screen: Edited messages has its own, Commands none because it has
  no delay, Self-destruct the shared one) and `ConfirmKey` (a row that draws a card). The delay labels gain 30 s, 5 min and
  15 min. Nothing in the renderer, the validators or the gates names `mc`, `me` or `ms`; they read the fields. The one card
  is the Self-destruct card, drawn by the control's `Set` only for a row with a `ConfirmKey` and a delay (never Off): a delay
  equal to the stored one writes nothing and draws no card, any other writes nothing and draws the shared card
  (`settingsCardRows`: `[Yes | No]`, then Back to Main and Close; `renderSettingsDelCard`, with the right line like every
  deletion screen). Its Yes is the page action `dy` (`settingsActDelYes`, fields exactly `t` a row with a `ConfirmKey`, `v`
  one of its presets and never `n`, and `e` digits-only positive; right `settingsRightChangeInfo`, so the whole press pipeline
  judges the presser again on a fresh record and a demoted one is refused with `settings_no_change_info`, nothing written).
  `settingsDelRunYes` runs the Staff Group gate, then `settingsCardExpired` (an expired Yes redraws the row with
  `settings_card_expired` and writes nothing), then the write through `applyDeleteRule` (a Yes of the stored value writes
  nothing) and the redraw from the read-back. No is `a=no&p=d` (the hub). Self-destruct Off is one tap with no card: it removes
  the row and `deletion.RemoveRule` drops the chat's waiting `member:selfdestruct` rows in the same transaction. The largest
  button of the page is 57 bytes (the card's Yes, 53 in a group menu), every choice at most 50 (46 in a group).
  `TestSettingsDeletion*`, `TestSettingsAuthorityDeletion`, `TestSettingsDriftDeletion` and `TestSettingsParityCleanService`
  hold it (`settings_deletion_test.go`, `settings_deletion_right_test.go`, `settings_deletion_screens_test.go`,
  `settings_deletion_golden_test.go`, `settings_deletion_member_test.go`, toolkit `settings_deletion_env_test.go`).
- The message editor's pages (`settings_edit.go`, pages `w` Welcome, `g` Goodbye and `r` Rules, orders 50, 60 and 70, each
  registered from that file's `init` through `registerSettingsPage` so `settings_pages.go` is never edited) show which
  parts of a message are set, one line per part from one parts table (`settingsEditPageDef.Parts`): Welcome and Goodbye
  have parts `x` text, `m` media and `b` buttons, and Rules has the same three (`x m b`; its parts table is
  `settingsEditRulesTextPart`, `settingsEditMediaPart`, `settingsEditButtonsPart`), so a later part is a table row and not new renderer
  code. A line is a state word and never the stored content: text is Default or Set with its visible length
  (`formatting.VisibleLength`), media Not set or its kind word (a GIF saved from Phase 12 on is stored as `db.ANIMATION` (9) and reads GIF; a GIF saved earlier stays a document and reads File or GIF),
  buttons Not set or Set with the total; the stored text, a button name and a link are never echoed, and a sticker or a
  video note adds `settings_edit_note_no_caption`. `renderSettingsEditor` is pure and built in row blocks (part rows,
  switches, Open in private, navigation), so a later plan adds a block without touching the others. The switches are a
  two-column grid built from the page's switch table (`settingsEditSwitch`: control, label, body line, hint, right,
  value and apply), filled left to right with an odd last cell alone on its row (`settingsEditSwitchGrid`). Each button
  is the state icon alone and the name (`settings_btn_edit_switch_*`, `settingsStateIcon`: a cell must fit 20 code
  points in every language), and each state word is a body line, one per drawn switch between the part lines and the
  hints (`settings_edit_line_*`, `settingsStateWord`); a drawn switch's own hint paragraph follows the page hint, in grid
  order. Welcome has controls `e` the on/off switch (`settingsRightAdmin`, as `/welcome on|off`), `d` Delete last
  (`settingsRightChangeInfo`, as `/cleanwelcome` and `/cleangoodbye`) and `o` Greet once (`settingsRightAdmin`,
  `applyWelcomeOnce`, hint `settings_edit_hint_once`) and `u` Media under text between them (below). Goodbye has `e`, `d`
  and `u`, Rules `u` alone (below), and Greet once is never offered on Goodbye or Rules. Greet
  once has no command, so
  it carries a drift test and an authority test instead of a parity test (`TestSettingsDriftOnce`,
  `TestSettingsAuthorityOnce`); `/resetwelcome`, `/resetgoodbye` and the Remove buttons leave it as it is, and the page
  never reads `greeted_members`. Known Delete-last race (plan 12-15 not shipped): two welcomes or goodbyes close together on
  two replicas can leave one old message undeleted (research Pitfall 22). Media under text (`u`, `settings_btn_edit_switch_below`, line `settings_edit_line_below`)
  needs the page's `EditRight` (`settingsRightChangeInfo`, UI-SPEC assumption A2), calls `applyWelcomeMediaBelow` or
  `applyGoodbyeMediaBelow`, and on Rules it needs `settingsRightAdminBotAdmin` (the page's `EditRight`, so the bot must be an
  administrator, notice `chat_status_bot_not_admin`) and calls `applyRulesMediaBelow` (`rules.SetRulesMediaBelow`); it is drawn, with its body line, only while `settingsEditMediaBelowEligible` holds: media is
  stored and `media.CaptionAboveEligible` (the sender's own predicate: a photo, a video or an animation) says its kind can
  carry the caption above it, so a document (a GIF saved before Phase 12), audio, voice, sticker, video note or no media
  closes the grid up. A press of the switch loads the page fresh first (`settingsEditStaleSwitch`, a cell with a `Drawn`
  function): when the cell is no longer drawn it writes nothing and redraws with `settings_edit_below_unavailable` (UI-SPEC
  assumption A5). A stored on is kept when the media changes to an ineligible kind or is removed, is neither drawn nor sent
  then, and comes back with an eligible one; Remove and the resets leave it (assumption A3). A press makes no length check,
  because the switch changes no length. Each press sends the absolute target `v` ("0" or "1", anything else is an
  invalid request: `settingsEditSwitchValue`), calls `applyWelcomeEnabled`, `applyGoodbyeEnabled`, `applyCleanWelcome`,
  `applyCleanGoodbye`, `applyWelcomeOnce` or `applyRulesMediaBelow`, and redraws from the row the function read back, with no notice; a failed
  write redraws the freshly loaded page with `common_settings_save_failed` and the old state. A menu opened in the group has an Open in private URL row (the
  existing `/start settings_<chat id>` deep link, left out and logged when the bot has no username) above the navigation
  row, a private menu has none; a private page has at most 8 rows and a group-menu page at most 9, and every page ends
  with Back and Close. Main lists the pages after Antiflood (`s|l / c|f / w|g / r|m / d`, then Access for the owner, so seven rows in a private and in a group menu; `m` is the Member requirements page and `d` the Deletion page below) with
  `common_status_on` or `common_status_off` for Welcome and Goodbye and `settings_state_set` or `settings_state_not_set`
  for Rules. The Deletion line reads `Off` or `{n} on`, and `n` counts the three member rules too (at most 20 rules).
  Each part has its row right after the body (`settingsEditPartRows`): `[Set]` while the part is not set and
  `[Set | Remove]` while it is (a Welcome or Goodbye text counts as set only while it differs from the default), in the
  order of the state lines, and a group menu draws them too. The actions are Welcome `ws` `wr` `wc` (Set, Remove,
  Cancel), Goodbye `gs` `gr` `gc` and Rules `rs` `rr` `rc`, each with the one field `t` (a part code of that page),
  checked by `Validate` (`settingsEditValidatePart`) before any I/O, so a missing, unknown or extra field is an invalid
  request with no `getChatMember`. Set and Remove need the right of the matching commands (`EditRight`): Welcome and
  Goodbye `settingsRightChangeInfo`, Rules `settingsRightAdminBotAdmin`; Cancel needs only `settingsRightAdmin`. Set
  writes the wait (`settingsInputSet`, the press's own group, page, part, message ID and a fresh nonce, ten minutes) only
  from a private menu and edits the message into the waiting prompt (`renderSettingsEditPrompt`: the part's prompt body,
  the limit after "Up to" from `settingsEditPromptMax`: 1024 while the group has stored media (Rules too), 3800 on Rules without media, 4096 otherwise, the
  end time in UTC, one Cancel button and no Back or Close); a wait it replaced for another group, page or part adds
  `settings_edit_replaced_note`, the same part only refreshes the ten minutes. With no Redis, or when the write fails, Set
  answers the alert `settings_edit_unavailable` and edits nothing, while Remove and the switches still work. In a menu
  opened in the group, Set, Remove and Cancel write nothing and await nothing: the page is redrawn with the notice
  `settings_edit_group_note` and its Open in private row, so no group menu ever creates a wait (D-01). Cancel
  (`settingsInputCancel`) compares group, page and part with the wait, and the ID of the message the button was pressed on
  with the wait's prompt (field `m`, no new callback field: the script gets `query.Message`'s own ID) or with a refusal
  reply noted on the wait, before deleting anything, so the Cancel of an older prompt of the same group, page and part
  (two private menus open) never ends the newer wait: a deleted wait edits the
  message into the page with the cancelled notice, a prompt whose wait was replaced, used or is being saved answers
  `settings_edit_prompt_stale` and leaves the live wait alone, one after the ten minutes answers
  `settings_edit_wait_expired`, and each of those edits the message into the page. A Cancel that meets a Redis error other
  than "no Redis" (a timeout, a dropped connection: the script may never have run, so the wait can still be live) answers
  the alert `settings_edit_unavailable` and edits nothing, so the prompt and its Cancel button stay; with no Redis no wait
  can exist and the page is drawn. Remove is one tap with no card, through
  the commands' apply functions (`applyGreetingRemoveText`, `applyGreetingRemoveMedia`, `applyGreetingRemoveButtons`,
  `applyRulesText("")`, `applyRulesMedia`, `applyRulesButtons`), so the three Remove presses of a Welcome or Goodbye page
  together store what `/resetwelcome` or `/resetgoodbye` store. On Rules, Remove clears just its part (text through
  `applyRulesText` with an empty text, media through `applyRulesMedia` with no file and type 0, buttons through
  `applyRulesButtons` with none) and `/clearrules` clears all three through `applyRulesClear` (`rules.ClearRules`), so the
  three Remove presses store exactly what `/clearrules` stores and none of them touches Media under text, private rules or
  the rules button (assumption A3). The Rules page and its Main line read the whole row through
  `rules.LoadRulesSettingsContext` (the `settingsEditRulesRead` seam, which creates nothing), and Main reads Set when the text,
  the media or the buttons exist (`RulesSettings.HasContent`, assumption A1). A Remove redraws from the read-back with a notice that says
  what was removed (the long text cannot be undone in the menu), a stale Remove of an empty part redraws with
  `settings_edit_nothing_to_remove` and writes nothing, and a failed write redraws the freshly loaded page with
  `common_settings_save_failed`.

## Permissions

- `fuku/utils/chat_status` predicates return bools and never reply; use `PermissionResponder` to message.
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
  a refusal, a bad argument, a basic group or a failed save writes nothing. The grammar is exactly `joins|flood|ai` then
  `on`, `off` or a whole number 3..500 (a number sets the threshold and turns the trigger on; `on` and `off` keep it).
  `ai` is checked and saved like the other two (`raiddetection.SetAIThreshold` / `SetAIEnabled`), so it has no
  refusal text of its own. The status lists the AI line after the flood line and adds
  `lockdown_auto_ai_unavailable` ("AI checks are not set up") when `raidAIConfigured()` is false, that is when
  neither the text checks (`ENABLE_AISPAM` with `TYPESAFE_API_KEY`) nor the image checks (`ENABLE_AISPAM` with
  `ANTHROPIC_API_KEY`) are available; a server with one of the two shows no notice. `ENABLE_AISPAM=false` is the global AI
  kill switch for the whole raid AI path (it also turns the AI spam-name rule of the member requirements off: the rule is
  then inert): `raidAITextConfigured` and `raidAIImageConfigured` both require it, the
  watcher enqueues nothing, and a job already queued is dropped by `raidAIProcessJob` / `raidAIProcessImage` without
  a `getFile` or any provider call. The `/lockdown` help says what the AI trigger sends to
  TypeSafe and to Anthropic Claude (`lockdown_help_msg`, all 8 locales, D-15), and its length is held by
  `TestLockdownHelpFitsOneMessage`.
  Anonymous admins always get the proof first and re-enter through `RegisterAnonymousAdminHandler`; the cached command's
  arguments then pick view or change for the tapper. The command is not `Disableable`. `/autoantiraid` is its alias for
  the join threshold (see the retired raid mode under Subsystem traps) with exactly the same checks and anonymous-admin
  re-entry, so neither is ever decided by the cached admin list.

- "I fixed it" (`mreq|v1`, `memberReqFixedCallback`) is for the held member only, with no admin override: the presser must be
  the member `u` of the button, an admin who presses it is refused, and admins lift a mute with `/unmute`. It lifts only a
  mute the requirements placed, proved by the live marker (`isMemberReqMute`), never an admin's, a staff, the captcha's,
  antiflood's or a warn-limit mute (MREQ-07), and only with permissions from `resolveUnmutePermissionsWithLockdown`. An
  approved member is freed at the tap (step 4b above) once the marker proves the mute is the requirements', so the remedy
  "ask an admin to approve you" is true for a mute: an admin approves the member and the member taps "I fixed it" (an
  approval does not lift the mute by itself). A member held for a spam-like name (the AI rule) gets out by changing their name
  (the button judges the new string, one provider call at most) or by an admin's approval (freed at the tap by step 4b); an
  admin who thinks the verdict is wrong approves the member, because the button for unchanged names reads the same cached
  verdict every time (D-23).
- `/captchamode turnstile` keeps `captchaAdminGate` (a per-group admin check, first and unchanged) and then refuses,
  writing nothing, in this order (`captchaWebModeRefusal`): no web captcha runtime (the four settings are not all set,
  or the bot has no username, `captcha_web_refuse_not_configured`), no Redis `PING` within 250 ms (a fresh probe, not the
  5 s memo of `captchaWebAvailable`, `captcha_web_refuse_no_redis`), or the bot not the creator or an administrator with
  `can_restrict_members` by a live `chat_status.FetchBotMember` (never the admin cache; an unknown answer refuses,
  `captcha_web_refuse_no_rights`). Math and text are unchanged and need none of these checks. The refusal texts name no
  setting, variable, secret or URL.
- The settings owner gate (`settings_gate.go`): `settingsGate` is the one decision every settings door calls after its own
  live `getChatMember` of the person, today in-group `/settings` with its anonymous re-entry (`requireSettingsAccess`),
  the `/start settings_<chat id>` deep link (`settingsStartDeepLink`, on the member record of its own check and before its
  `getChat`), every button press (step 4b), the pick of the private list (the same step 4b) and the typed reply of the
  message editor (`settingsInput`, on the member record of its own live check); the other doors call it as
  they arrive, and `TestSettingsGateAllDoors`
  proves that every door decides the same over the four worlds (owner with the switch off, switch on, no owner, owner
  state unknown), each later door adding a column to it. The order is fixed: a live creator is in with no
  read; any status other than administrator is undecided; for an administrator the switch is read fresh
  (`menuaccess.AdminsAllowedFresh`, a missing row is off, an error is undecided) and true is in, never consulting the
  memo below, so a flip to on is seen at the very next press; with the switch off the owner state decides. The owner-state
  read (`settingsOwnerState`) is one `getChatAdministrators` call bounded to 3 s (`settingsOwnerLookupTimeout`), shared per
  chat by a singleflight, and an answer that arrives after the deadline is discarded. It is evaluated per presser: a
  creator entry that is not a deleted account means an owner exists and keeps the administrator out; no creator entry, or
  only deleted accounts, means no owner and lets the administrator in (D-15); an error, a timeout, an empty list, a list
  without the presser's own administrator entry and a creator entry that is the presser are unknown and refuse. Only
  "an owner exists" is remembered, per chat on this replica, for less than 30 s (`settingsOwnerMemoTTL`; a memo exactly that
  old is expired), because it can only keep an administrator out; nothing else is memoised. `settingsCreatorGone`
  (planner assumption A1) is `isDeletedAccount` of the first name plus no last name and no username, the one test for a
  gone creator. Accepted limitation: it decides from the display name alone, because Telegram gives no second signal,
  so a creator with a username or a last name is always a live owner, but a live owner whose first name is exactly
  "Deleted Account" with neither looks gone, and with the switch off every administrator is then let in. The failure
  direction is that administrators are allowed in, never that the owner is locked out (a live creator is in before the
  owner state is read), and the owner fixes it by adding a username or a last name. The owner's live check A1 in
  `.planning/phases/10-pm-settings-hub-and-owner-gate/10-SPIKE.md` confirms what the API returns for a deleted owner. Unknown always
  refuses with `settings_check_failed`, never with the owner-only text, and no refusal names the Staff Group, a group
  title, a chat ID or the switch. `isTelegramClientError` (a Telegram 4xx other than 429, also wrapped) is the one
  client-error classifier of the settings doors. The in-group setting commands (`/captchamode`, `/setflood`,
  `/autolockdown` and the rest) are not gated and keep their own checks (MENU-13); the authority-parity tests carry the
  documented carve-out (`TestSettingsAuthorityParityOwnerGate`): for a non-owner administrator with the switch off the
  menu is stricter than the commands, and where the menu allows a control the command allows it too.
- Each settings menu control declares one `settingsRight`, and the menu refuses what the matching command refuses.
  `settingsRightAdmin` needs only the live admin check every press already makes (`/captchamode`, `/captchatime`,
  `/captchaaction`, the editor's Welcome and Goodbye switches and Greet once). `settingsRightAdminBotAdmin` also needs `chat_status.IsBotAdmin` and refuses with the notice
  `chat_status_bot_not_admin` (`/captcha`). `settingsRightAdminFloodBot` also needs `chat_status.IsUserAdmin(b, chat, b.Id)`
  and refuses with the notice `connections_is_user_connected_bot_not_admin` (`/setflood`, `/setfloodmode`, `/delflood`).
  `settingsRightRestrict` needs the creator or an administrator with `can_restrict_members` from the same member record
  (`staffIssuerSkipReason`) and refuses with the alert `settings_no_restrict` (lockdown lift and triggers). The user check
  is live, so it is stricter than the commands' cached one; the bot check uses the commands' own predicate, never a live
  `FetchBotMember`. A notice refusal answers empty and redraws the page with the warning line; an alert refusal edits
  nothing. A control the presser lacks the right for is still drawn and its press refused.
  `settingsRightOwner` needs the creator from the press's own member record (no extra call) and refuses with the alert
  `settings_access_owner_only`; only the Access page's control and action declare it. On the Member requirements page
  (`m`) opening the page and a rule screen needs `settingsRightAdmin`, and every punishment choice (any `v`, so the card is
  not even drawn without the right), the card's Yes and both switches need `settingsRightRestrict`; this holds for the photo
  (`ph`), channel (`ch`) and AI (`ai`) rules too, the rights are judged on a fresh member record on every press, and the
  channel rule's live link check and the AI rule's configuration check (`settingsMreqAIGate`) come after the right, so a
  presser demoted before a Yes is refused with `settings_no_restrict`, no link is looked up and the configuration check
  never runs.
  `settingsRightChangeInfo` needs the creator or an administrator with `can_change_info` from the same member record (no
  extra call) and refuses with the alert `settings_no_change_info` (the editor's Delete last, as `/cleanwelcome` and
  `/cleangoodbye` refuse through `CanUserChangeInfo`); it also gates every choice of the Deletion page (`/cleanservice` asks for it too). `settingsRightRefusalFor` is the one pure judgement of a right's
  member-record part, shared by a button press and a typed reply, so the two can never judge a right differently.

## Data

- Repositories live in `fuku/db/<domain>/`. Read via `cache.GetFromCacheOrLoad` with `cache.CacheKey` keys.
  **Every write must `cache.DeleteCache` the keys it affects.**
- `GetFromCacheOrLoad` has an in-process layer (`CACHE_LOCAL_TTL`, default 10 s) in front of Redis. `DeleteCache`
  evicts it on this replica only; list freshness-critical keys in `skipLocal` (`fuku/db/cache/local.go`). Tests that
  write rows directly must call `cache.ResetLocalForTest()` or `DeleteCache` before reading through the cache.
- Two packages are named `cache`; the loader and generation guards are in `fuku/db/cache`, not `fuku/utils/cache`.
- Operational Redis keys (`fuku:anonAdmin:*`, `fuku:staff:*`, `fuku:lockdown:*`, `fuku:raid:*`, `fuku:memberreq:*`) sit outside the
  `fuku:cache:` prefix; `CLEAR_CACHE_ON_STARTUP` does not clear them. `fuku:antiraid:*` is retired: nothing reads or
  writes it, and `sweepRetiredAntiRaidKeys` removes what the old raid mode left, once per process at startup.
- `fuku:settings:sweep:lock` (new family `fuku:settings:*`, also outside the `fuku:cache:` prefix, so
  `CLEAR_CACHE_ON_STARTUP` keeps it) is the admin index sweep's per-cycle lock (`admin_index_sweeper.go`): `SET NX` with a
  per-process owner (`adminIndexSweepOwner`, a UUID) and a TTL of the interval minus one minute, so one replica runs each
  hourly cycle. Without Redis, or on a Redis error, a cycle runs unguarded and stays correct, because every roster write is
  a replace of the whole roster or a clear. Only a cycle cut off by a cancel gives it back, by compare-and-delete.
- The message editor's wait for a typed reply (`settings_input.go`, all outside the `fuku:cache:` prefix in the
  `fuku:settings:*` family, so `CLEAR_CACHE_ON_STARTUP` keeps them, and holding IDs and codes only, never message text or
  a file ID). `fuku:settings:input:<user>` is one hash per user with `c` (group chat ID), `p` (page key), `t` (part), `m`
  (prompt message ID), `n` (16 hex nonce from `crypto/rand`), `x` (expiry, unix ms), `r<message id>` for each refusal reply that carries a Cancel button (at most 40 fields in
  all, no text) and, while a claim is held, `k`; its own
  10-minute `PEXPIRE` is the wait's expiry, with no timer, no process map and no shutdown drain.
  `fuku:settings:inputdone:<user>` is the marker `c|p|t`, 15 minutes, written by every Set; every deliberate end (finish,
  the Cancel button, `/cancel`, an ended access) deletes both keys in one script, so a marker with no wait means only
  "the wait expired". `fuku:settings:inputalbum:<user>:<media_group_id>` is the album memory, `SET NX EX 30`, for a media
  group ID of 1 to 20 digits (anything else is not an album and writes nothing). Atomicity is seven Lua scripts in
  `settings_input.go`: Set (writes both keys and returns the replaced wait's fields; it deletes the hash first, so the
  refusal fields below never survive a new Set), note-refusal (adds the field `r<message id>` for a refusal reply, only for the
  holder of the nonce and at most 40 fields in all), claim (only the holder of the
  current nonce, once), finish and release (only the claim holder; finish and release run on `context.WithoutCancel` with
  their own bound, so a cancelled update never strands a claim; release removes only `k`, so the wait keeps its remaining
  TTL), and the Cancel button's and `/cancel`'s scripts (a held claim answers busy and deletes nothing; the button also
  compares `c`, `p`, `t` and the pressed message's ID with `m` or a noted refusal `r<id>`, so a stale prompt never ends a
  newer wait, not even one for the same group, page and part). Reads are one pipeline of `HGETALL` and `GET`; a hash
  that does not parse is no wait. Without Redis every store function returns `errSettingsInputUnavailable`, so Set
  answers `settings_edit_unavailable` and nothing else needs Redis.
- `fuku:staff:act:<token>` is the staff action card hash. It lives 5 minutes plus 1 minute grace while pending and
  1 hour once terminal, and moves state only through the Lua compare-and-set in `staff_action_card.go`.
  It expires through a timer on the creating replica plus a lazy check on any tap, and Confirm aborts when the
  signature of the sorted linked group IDs (`links_sig`) changed since the card was shown. The same hash also holds
  undo confirm cards (`action=undo`, `undo_of`, `undo_kind`) whose issuer is the presser of Undo. One undo per action
  is the conditional update of `staff_actions.undo_started_at` (`ClaimUndo`), not the card, which expires. The claim is
  given back by `ReleaseUndo` (one conditional update scoped to that claim's `undo_by` and `undo_started_at`, which also
  clears the groups' undo columns) when no group of the run reached its Telegram write (`staffGroupResult.Reached`, set
  once `executeStaffUndoCall` returned, whatever it returned, and for every group `sweepPending` swept after a panic,
  which may have reached its write before it was lost), and kept once any group did.
- `fuku:staff:lock:target:<id>` is the per-target fan-out lock: `SET NX` with the card token as value and a 30-minute
  TTL, taken at Confirm, renewed every 10 minutes (`staffTargetLockRenewEvery`) by the run's coordinator through a
  compare-and-set on that token while the fan-out runs (a vanished key is re-taken, another card's lock is never
  touched), and released by compare-and-delete with that token when the run ends or the Confirm aborts. An undo's
  Confirm takes the same lock, so an undo and a staff action on one person never run at once; the loser is answered
  "target busy" and its card stays pending.
- `fuku:staff:pace:next` and `fuku:staff:pace:block` are the fleet-wide Telegram pacing for staff fan-outs: a slot
  reservation plus a shared `retry_after` block. The block keeps Telegram's full `retry_after`; a call whose slot is
  more than `MaxWait` (60 s) away fails at once as rate limited, with no Telegram request and without taking the slot.
  Every staff fan-out Telegram call goes through `staffPaced`, and per-replica limiters must not be used for shared
  budgets.
- `fuku:lockdown:recent:<chat>` is the best-effort 10-minute join tracker (`lockdown_recent.go`): a sorted set scored by
  join time in unix milliseconds, written by both group `-7` join handlers (`lockdownOnJoinMember`,
  `lockdownOnJoinMessage`) for every supergroup join before they read the lockdown, whether or not one is active. Join
  requests and the bot's own join are not tracked. A member is compact JSON (`i` id, `n` name, `u` username, `b` bot,
  `l` invite link; the chat_member path carries the link, the service message does not), so the two deliveries of one
  join are merged by user ID at read time (`readLockdownRecentJoins`, behind the `lockdownRecentLookup` seam: earliest
  time wins, a non-empty link, name or username fills in). One pipelined round trip under 250 ms
  (`lockdownRecentTimeout`) adds, trims to 10 minutes, keeps the newest 500 (`lockdownRecentCap`) and sets a 700 s TTL.
  It is empty without Redis and is never a ban decision. The joins of the 10 minutes before the lock are saved with the
  lockdown's alert (`chat_lockdown_burst`); a flood lockdown also saves its flooders there (`fuku:raid:{<chat>}:flooders`
  below). The raid join counter does not use it.
- `fuku:raid:{<chat>}:joined` is the join counter of automatic raid detection (`raid_counters.go`): a sorted set whose
  member is the user ID and whose score is that account's latest join in unix milliseconds, so the `chat_member` update
  and the service message of one join are one member and a re-join refreshes the account. One Lua script
  (`raidJoinScript`, the chat ID is a hash tag) adds the joins, trims what is older than 24 h, keeps the newest 5000
  (`raidJoinedCap`), sets a 25 h TTL and returns how many different accounts joined in the last 60 s. It is best effort
  and Redis only: without Redis automatic detection is inert on that replica (one info log per process) and `/lockdown`
  still works. `fuku:raid:{<chat>}:fire` is a `SET NX EX 60` latch that only sheds load while a trigger starts a
  lockdown; exactly one lockdown is still `uk_chat_lockdowns_active` plus `lockdown.Start`, and one alert still
  `lockdown.BeginAlert`, however many triggers and replicas fire at once. After the latch `raidFire` reads the chat's
  active lockdown fresh and stays silent, with no Telegram call, while one exists, and for 10 minutes (`raidLiftQuiet`)
  after `lockdown.RecentlyLiftedFresh` finds a lift of the chat started (`lift_started_at` in PostgreSQL, so a Redis
  flush does not end it, and real time); an unreadable answer shortens the latch to 10 s. The latch then lasts 60 s
  after a started, already-active or unknown-lock outcome, 10 s (`raidRetryLatch`) after a failed bot-rights check,
  unreadable permissions or failed state write, and 10 minutes (`raidCantLockLatch`) when the group is not a
  supergroup, the bot cannot restrict members or Telegram refused the lock. Do not reuse
  `fuku:lockdown:recent:<chat>` for counting: its JSON members count one join twice.
- `fuku:raid:{<chat>}:fi`, `:fn` and `:fa` are the message flood counter of automatic raid detection
  (`raid_flood.go`): three sorted sets scored in unix ms, the window's item keys (`m:<message id>`, or `g:<media group
  id>` so an album is one item), the item keys sent by new members and those members' user IDs. One Lua script
  (`raidFloodScript`) adds the item, trims all three to 30 s, caps the first two at 2000 (`raidFloodCap`), sets
  `PEXPIRE` 2 minutes on each and returns their sizes; a member is new when `:joined` scores them within 24 h
  (`ZSCORE`). It is Redis only, like the join counter.
- `fuku:raid:{<chat>}:av` and `:aa` are the AI spam verdict counter of automatic raid detection (`raid_ai_count.go`):
  two sorted sets scored in unix ms, the verdict items (`m:<message id>`, or `g:<media group id>` so an album is one
  item) and the accounts behind them (user ID, scored by that account's latest newly counted verdict). One Lua script
  (`raidAIVerdictScript`) adds the item with `ZADD NX`, so a second report of one message or album is a no-op, and
  writes the account only when its item was newly added, so a duplicate can never refresh an account whose items aged
  out. It trims both sets to the fixed 120 s window (`raidAIWindow`), caps each at 1000 (`raidAICap`, a variable only
  for tests, never set below 500 except to prove the cap), sets `PEXPIRE` 5 minutes and returns the two sizes. It is
  Redis only, like the other raid counters: without Redis the trigger is inert on that replica. Message text is never
  stored.
- `fuku:raid:{<chat>}:ar:<user>` is the repeat test of the raid AI candidate watcher (`raid_ai.go`): a list of hashes of
  a new member's recent texts, each the first 8 bytes of the SHA-256 of the normalised text (lower-cased, whitespace
  collapsed) as 16 hex characters, only for a text of at least 10 runes (`raidAIRepeatMinRunes`), newest 5
  (`raidAIRepeatHistory`), 1 h TTL (`raidAIRepeatTTL`). One Lua script (`raidAICandidateScript`) answers whether the
  sender is new (`:joined` scores them within 24 h) and whether the hash is one of their last few, and records the
  hash for a new member only, so an established member leaves no key. Raw message text is never stored, here or by
  the raid path anywhere else. It is Redis only, like the other raid counters.
- The image checks' cost guards (`raid_ai_image.go`, D-20) keep five Redis key families, all outside the cache prefix.
  `fuku:raid:{<chat>}:ag:<media_group_id>` is the album gate, `SET NX EX 120` (`raidAIAlbumGateTTL`): the first photo of
  an album is judged and the others are dropped, except that a first photo refused for a reason of that moment before any
  provider call (a paused breaker, a budget refusal, a Redis error, the fire latch) gives the gate back with `DEL`
  (`raidAIAlbumRelease`), so the next photo of the album can still be judged; a photo that reached the provider keeps it.
  `fuku:raid:{<chat>}:ab:<yyyymmddhh>` and `fuku:raid:aibudget:<yyyymmddhh>`
  are the hourly budgets, 150 provider calls per group (`raidAIChatHourlyBudget`) and 500 in all
  (`raidAIGlobalHourlyBudget`), counted by `INCR` plus a 2 h expiry (`raidAIBudgetTTL`) in one transaction, the UTC hour
  from `raidNow`. In front of both sits the per-account budget `fuku:raid:{<chat>}:abu:<user>:<yyyymmddhh>`, 20 calls
  per account per group per hour (`raidAIUserHourlyBudget`), counted the same way and reserved first: an account that
  is over it is `Capped` and spends neither shared unit, so one fresh account posting distinct photos can neither empty
  its group's 150 nor the fleet's 500 (WR-01). A cached verdict is answered before any budget and spends none. `fuku:raid:aiv:<file_unique_id>` is the verdict cache: the probability as a decimal string, `SET EX
  3600` (`raidAIVerdictCacheTTL`), written only after a verdict (never for a failure or a block). The budget is reserved
  before the download, so a failed call still spends its unit; a group over its own budget never spends the fleet's. The
  two budgets are package variables only so a test can make them small. A Redis error on any of these keys is no verdict:
  the image is not sent, never an unbounded call. The all-groups key has no `{chat}` hash tag, so it is only ever used in
  single-key commands, never in a multi-key script.
- `fuku:raid:{<chat>}:flooders` is the flood alert's snapshot (`raid_flood.go`, D-17). When a flood trigger fires,
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
- `fuku:raid:{<chat>}:aispammers` is the AI burst alert's snapshot (`raid_ai_count.go`), the twin of `:flooders`. When an
  AI trigger fires, `raidFire` calls `raidSnapshotAISpammers` at the same point (after the latch, the active check and
  the quiet period, before `startLockdownRow`): one Lua script (`raidAISpammersScript`) replaces the key with the newest
  accounts of `:aa` (at most `raidFloodersMax` = 500) whose `:joined` score is within 24 h, scored by that join time,
  with a 15-minute TTL (`raidFloodersTTL`). The AI burst snapshot lists recent joiners only (WR-04): an established member
  whose `/aispam` delete verdict counted (D-19) still counts toward the threshold and the 3-account floor, but is never
  saved, so the alert's sample and "Ban N recent joiners" can never offer one. A failed snapshot is logged at debug and the lockdown still starts. `lockdownBurstSnapshot` reads
  it for a lockdown whose `trigger_kind` is `ai_burst` only (`raidAISpammersLookup`, a test seam; no Redis or no snapshot
  adds nothing) and `lockdownMergeAISpammers` merges it with the 10-minute tracker's joins through
  `lockdownMergeSnapshot`, the one merge that `lockdownMergeFlooders` shares (by user ID, the tracker's name, username,
  link and bot flag win, the earliest time wins, the rest are named with one `user.FindUsersByIDs` query, the bot is
  skipped). They are saved as `pre` burst rows by `lockdown.BeginAlert`'s winner, so the alert's sample and "Ban N recent
  joiners" include them with the unchanged Phase 5 exclusions. An AI alert's counts line is `lockdown_alert_counts_ai`,
  which does not say the listed accounts joined.
- Automatic lockdowns are announced in `raid_trigger.go`, every post through `lockdownPaced` under `raidPostTimeout`
  (30 s per send), and a failed post is only logged: it never changes the lockdown or its absence. The lock itself is
  synchronous, but the notice and the cannot-lock warning are sent from a goroutine (`raidPostAsync`: counted in
  `raidPosts`, starting with `defer error_handling.RecoverFromPanic`), so the join guard records the joiner who tipped the
  threshold and the message pipeline reaches aispam, antiflood and the rest without waiting on a pacer slot. They are
  best effort and drained at shutdown by `StopRaidPosts` (below, under shutdown). Tests wait for them with
  `raidPosts.wait()`; the fake's read helpers (`sentTo`, `sentChats`, `callsFor`) do it themselves. The call of `raidFire` that
  got `lockdownStartStarted` posts one public notice in the locked group (`raidPostNotice`, the group's language: the
  trigger and its numbers, `lockdown_note_approved_muted`, and the no-delete and no-invite notes `/lockdown` adds),
  and no other outcome does. When the bot cannot lock a group (`lockdownStartNotSupergroup`,
  `lockdownStartBotCannotRestrict` or a definitively refused lock, `lockdownStartLockFailed`) nothing is started and
  `raidWarnCannotLock` warns the group's Staff Group (`staff.GetLinkOfGroupFresh`, the Staff Group's language) and its
  log channel (`actionlog.Destination` under the admin category, the group's language, starting with the usual header),
  each only when it exists. The destinations are resolved before the gate is touched: with neither, nothing is
  attempted and the gate stays free, so a Staff Group linked later is still warned. One `SET NX EX 86400` on
  `fuku:raid:{<chat>}:warn` then gates both copies, so a group is warned at most once a day across replicas, and a
  Redis error or a lost `SetNX` sends nothing. The gate records a warning that reached somebody: when no copy was
  delivered it is given back with `DEL` (`raidReleaseWarnGate`, best effort), and when at least one was delivered it
  stays. The other outcomes
  (bot rights or permissions unreadable, a failed state write, a lock whose answer was lost) only log. The group title
  is escaped and spliced in after translation; the trigger sentence (`lockdownTriggerSentence`) holds only numbers.
- `chat_raid_detection` (migration 20261018130000) holds one chat's automatic raid detection settings: the join surge
  and flood switches and thresholds (`CHECK` 3..500, defaults 15 and 30). Migration 20261029120000 adds the AI burst
  trigger's `ai_enabled` (default true) and `ai_threshold` (default 5, `CHECK` 3..500 as `chk_chat_raid_detection_ai`);
  existing rows take those defaults. A missing row means the defaults, all three triggers
  on (`raiddetection.Defaults`, with `DefaultAIThreshold`); the Go model carries no `default:` tag on its booleans, because GORM's Create would
  turn a Go `false` back into the SQL default. It is read through `cache.GetFromCacheOrLoad` under
  `cache.CacheKey("raid_detection", chat)` (`raiddetection.GetSettings`), every write must `cache.DeleteCache` that key,
  and it is never part of backup/export/import/reset. The migration carries every non-zero
  `antiraid_settings.auto_antiraid_threshold` over as the join threshold, clamped by `LEAST(GREATEST(x, 3), 500)`: a
  value below 3 becomes 3 (D-18) and the old `/autoantiraid` values above 500 become 500 (D-22), so the copy satisfies
  the `CHECK` and cannot abort startup. The key `raid_detection:*` is listed in `skipLocal` (D-12), so another replica
  never serves it from the in-process layer after a change. A value the previous build cached has no AI fields and decodes
  with `ai_threshold` 0 (stored rows are never below 3), so `GetSettings` reads it as the AI defaults (on, at 5), never as
  the trigger off. Rolling-deploy caveat (accepted, like the others here): the settings cache is shared in Redis, so an
  old-build replica that reloads a chat's row after an admin ran `/autolockdown ai off` caches an entry without the AI
  fields, and the new replicas read it as AI on until that entry expires (`cache.DefaultCacheTTL`, 30 minutes) or the
  next write; a single-replica deploy is unaffected, and `/autolockdown ai off` run again after the deploy settles it.
  Writes are six column-targeted upserts, one per trigger
  and kind (`SetJoinEnabled`, `SetJoinThreshold`, `SetFloodEnabled`, `SetFloodThreshold`, `SetAIEnabled`,
  `SetAIThreshold`; a threshold also turns its
  trigger on), each `INSERT ... ON CONFLICT (chat_id) DO UPDATE` of only its own columns plus `updated_at` followed by
  `cache.DeleteCache`, so two admins changing the two triggers never clobber each other, a Go `false` is always written,
  and a threshold outside 3..500 returns `ErrThresholdOutOfRange` before any write. The developer `/stats` counts the
  rows through `raiddetection.LoadRaidDetectionStats` (chats with a row, and the chats with each trigger off: the
  "Join surge off", "Flood off" and "AI spam off" lines; `AI spam off` is the chats with `ai_enabled` false). `getStats`
  then appends `raidAIStatsSection`, a heading that says "this replica, since its last restart" and two lines of
  in-memory numbers only (no text, file ID, chat, user or key): the raid AI pool's counters (`RaidAIStats`: checked,
  spam, kept, shed, failed, blocked, capped, cached) and `/aispam`'s (checked, deleted, kept, shed, failed). They belong
  to the process that answers, start at zero on every restart and are not fleet totals.
- `fuku:lockdown:pace:next` and `fuku:lockdown:pace:block` are the fleet-wide Telegram pacing of lockdown bans,
  unbans, declines and deletes (`lockdownPaced`, `lockdownPacer`): the same slot-plus-block design as the staff pacer,
  with its own keys so a raid and a staff fan-out each keep their own budget. A slot more than `MaxWait` away fails at
  once as rate limited and the joiner row goes back without counting an attempt.
- `fuku:memberreq:pace:next` and `fuku:memberreq:pace:block` are the fleet-wide Telegram pacing of the member
  requirements executor (`memberReqPaced`, `memberReqPacer`: 100 ms, 3 retries, 60 s): the staff pacer's slot-plus-block
  design with its own keys, so screening, a staff fan-out and a lockdown each keep their own budget. Every Telegram call
  of `memberReqEnforce` (the live `getChatMember`, the punishment, the notice, the delete) goes through it, each bounded by 10 s,
  except the limit action the warn module takes when a warn reaches the limit (`applyWarn`'s kick or ban): that one call is
  neither paced here nor bounded by the job context, and is bounded instead by the HTTP client's 30 s timeout and the
  per-chat cap.
- `fuku:memberreq:{<chat>}:cap:<unix minute>` and `fuku:memberreq:{<chat>}:paused` are the member requirements' per-chat
  brake (`memberreq_limits.go`, the chat ID a hash tag, outside the cache prefix so `CLEAR_CACHE_ON_STARTUP` keeps them).
  The counter is a fixed 60 s window (`raidNow().Unix()/60`, so a test can stop the clock) of punishment attempts,
  `INCR` plus `EXPIRE 120` in one pipeline under 250 ms, counted before the claim (`memberReqCapAllow`, between the live
  lookup and `memberreq.ClaimHold`), so an attempt that is refused, lost or never confirmed still counts and a bot that keeps
  being refused reaches the pause. More than `memberReqCapPerMinute` (10) refuses the punishment and sets the latch with
  `SET NX EX` for `memberReqPause` (10 minutes); only the caller whose `SET NX` succeeded posts the one `mreq_notice_paused`
  (`{n}` the cap, `{minutes}` the pause) in the group's language through `memberReqPostNotice`, best effort, so one notice
  is posted however many replicas tip the cap at once. While the latch stands (`memberReqPaused`, read after the held path
  and before the live lookup) no new punishment, hold or notice starts and the live `getChatMember` is not made; the held
  members' own messages and the deletes go on (below). The brake never drops a notice already due, because the cap is
  counted before the claim and never after the punishment. A Redis error or no Redis allows the punishment and pauses
  nothing (accepted, like the raid counters): nothing the requirements need for correctness lives only in Redis. Both
  numbers are package variables only so a test can shrink them.
- `fuku:captcha:ch:<token>` is the Turnstile captcha's challenge handle (`fuku/utils/captchaweb`, `challenge.go`): a hash
  of decimal strings `u` (user ID), `a` (attempt ID), `c` (chat ID) and `x` (the attempt's expiry in unix ms), living the
  attempt's time left plus 5 minutes (`ChallengeGrace`). The token is 16 random bytes in base64url (22 characters) and is
  public, because it is the `startapp` value of the group's URL button: the barrier is the signed `initData` `user.id`,
  never the token's secrecy. `SendCaptcha` writes it for a Turnstile attempt (`Store.Issue`, one round trip under 250 ms)
  and the verify handler consumes it with a user-compared Lua script (`Store.ConsumeIfUser`): an unknown or used key
  answers nothing, another member's verify changes nothing (`not_for_you`), and only the target's uses it up. The script
  claims the key (a `k` field holding a random nonce) and does not delete it: the reply is read and parsed first, then a
  second script deletes the key only while it still carries that nonce, and any error after the script ran (a reply lost
  because the context ended or the connection dropped, a record that does not parse) releases the claim with the same
  nonce, so an error from `ConsumeIfUser` always means nothing was used up and the member can tap again. A claimed key is
  used up for every other caller, which keeps 50 parallel verifies to one; `Rearm` treats a key that still carries a
  claim (a delete that failed) as absent. The handler runs the claim on `context.WithoutCancel` of the request with a
  750 ms deadline, so a client that disconnects never abandons it, and the release and the delete run on a context of
  their own. If Redis is unreachable for the release too, the claim stays until the challenge expires and the member
  times out as for any other Redis fault (accepted, WR-03). It is Redis
  only and sits outside the `fuku:cache:` prefix beside the existing `fuku:captcha:refresh:cooldown:*`, so
  `CLEAR_CACHE_ON_STARTUP` keeps it; a Redis flush loses the open challenges and those members time out as usual. A retry
  puts the same key back with `Store.Rearm` (one Lua script, bounded by `RedisTimeout`): the time it had left when it was
  consumed minus the time since, written only when the key is absent and time remains, so a re-arm never stretches a
  challenge past its attempt and never overwrites another one.
- `fuku:captcha:rl:ip:<ip>`, `fuku:captcha:rl:u:<user>` and `fuku:captcha:rl:t:<token>:<user>` are the abuse windows of
  the Turnstile verify route (`captchaweb/limiter.go`, `Limiter.Allow`): fixed windows of 60 s counted by one Lua script
  (`INCR`, `PEXPIRE` on the first increment, a key that lost its expiry gets one again), with budgets of 60 per client
  address (an IPv6 client counts by its /64), 20 per verified user and 8 per token and verified user
  (`verifyIPLimit`, `verifyUserLimit`, `verifyTokenUserLimit`). They are Redis only and outside the cache prefix, so
  every replica shares them and `CLEAR_CACHE_ON_STARTUP` keeps them; there is no in-process map. A Redis error on any
  window refuses with 503 retry and never lets the request through; a spent budget answers 429 retry with `Retry-After`
  in whole seconds. The user and token windows are keyed by the verified `initData` user, never by the token alone, so
  another member who hammers a target's public token spends only their own budgets and the target still passes (D-26).
  The address window is counted before the body is read, the other two right after `initData` is verified, all before the
  in-flight slot is taken.
- `fuku:captcha:streak` is the Turnstile key mismatch breaker (`captchaweb/breaker.go`, `Store.RecordFailedSolve` and
  `Store.ClearFailedSolves`, WR-04): one fleet-wide counter of failed solves (`OutcomeFailed`, any reason) since the last
  confirmed pass, incremented by one Lua script that also renews a 10 minute expiry (`StreakTTL`), so a streak that goes
  on stays tripped and a quiet one ends. Cloudflare answers `invalid-input-response` for a token minted by a widget that
  is not the configured secret's (a site key and a secret copied from two widgets, or Cloudflare's dummy test keys), and
  D-28 reads that as the member's fault, so without a guard a bad key pair kicks every joiner. The first
  `StreakLimit` (5) failed solves in a row are enforced; from the sixth on, `suspectKeyMismatch` holds the failed verdict
  back as a `retry` (the member stays muted, the attempt and the challenge stay intact, no marker is written, nobody is
  kicked or banned) and the attempt's own timeout still applies the group's `/captchaaction` to anyone who really does
  not solve. The hold is logged at Error as `[CaptchaWeb] suspected key mismatch ...` by reason name only, at most once a
  minute per replica, and counted as an error. Every verdict that confirms a pass runs `ClearFailedSolves` (`DEL`). It is
  Redis only and outside the cache prefix; a Redis error fails open (the failure is enforced as D-28 says, nobody is
  blocked, a pass still goes through). A single bad token is a streak of one and never trips it.
- `fuku:captcha:done:<token>` is the Turnstile verify route's decided-answer marker (`challenge.go`, `Store.MarkDone` and
  `Store.Done`): a string `<user>:<result>[:<action>]` (result `passed`, `failed`, `expired` or `invalid`, the neutral answer of
  a failure action that could not be applied; action `kick`, `ban` or `mute` and empty for passed and invalid), 10 minutes (`DoneTTL`), written with `SET NX` after every decided answer and before the
  response is written, so a response lost on the way can be asked for again. When the challenge key is gone, a verify from
  the same verified user with a marker gets the same answer with no backend call and no Cloudflare request, another
  verified user gets `not_for_you`, and no marker means `invalid`. A retry answer writes none (its token is re-armed
  instead), and a marker write error is logged by outcome name and never changes the answer. Parallel verifies of one
  token act once because only the Lua consume of `fuku:captcha:ch:<token>` succeeds for one of them. It is Redis only and
  outside the cache prefix.
- The developer `/stats` also gets the web captcha's counters: `getStats` appends `captchaWebStatsSection` after
  `raidAIStatsSection` (issued, passed, failed, expired, invalid, retry, limited, errors). They are in-memory numbers of the
  replica that answers (`captchaweb.StatsSnapshot`), start at zero on every restart and are never fleet totals, and the
  section holds no token, initData, ID or Cloudflare text. Each event is counted once: `writeAnswer` counts a decided
  answer by its result (`invalid` and `not_for_you` as invalid; an `expired` answer is not counted, because the expiry is
  counted where the attempt was claimed), `writeLimited` counts a 429 or an in-flight 503 as limited and not as retry, a
  remembered answer replayed from `fuku:captcha:done:<token>` counts nothing, `captchaweb.CountIssued` runs in
  `SendCaptcha` once a Turnstile challenge message was sent and its ID stored, and `captchaweb.CountExpired` runs in
  `expireCaptchaAttempt` when `handleCaptchaTimeout` claimed an attempt whose answer is reserved (the timer, the cleanup
  tick and a late check all go through it). Errors are configuration faults (`Verdict.ConfigError`), backend and Redis
  errors and a failed unmute; a Cloudflare 5xx or a network fault is a retry only.
- `captcha_settings.captcha_mode` is `math`, `text` or `turnstile` under `chk_captcha_mode` (migration 20261030120000).
  That migration drops `chk_captcha_mode` and the legacy inline column check of 20250807103246 (PostgreSQL named it
  `captcha_settings_captcha_mode_check`, and no later migration removed it, so every database the chain built had two
  checks) before re-adding `chk_captcha_mode` `NOT VALID`, because both appliers swallow `duplicate_object` on an
  `ADD CONSTRAINT`: an add without the drops would change nothing and say nothing. `captcha.SetCaptchaMode` accepts
  the three values and invalidates `captcha_settings` after the write. A `turnstile` stored on a bot whose web captcha
  is unavailable (or imported from another bot) is handled at join by the math fallback, never by refusing the row.
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
  (`applyLockdownLift`), the alert's Lift Confirm and the settings menu's Lift Yes (`applyLockdownLift`) (restore the
  stored permissions, then one conditional record, then `FinishLift`). The alert's Lift and the settings menu's Lift
  from a private menu also post the public `lockdown_lifted` notice in the locked group, through `lockdownAnnounceLift`
  (`lockdownPaced`), and wake the worker, so a lift by button is never silent, also from a private menu; a Lift from a
  menu opened in the group only wakes the worker, its redrawn menu being the notice in that same chat. The
  `chat_lockdowns` columns `trigger_count`, `trigger_window` (seconds) and `trigger_percent` (migration 20261018120000,
  with `idx_chat_lockdowns_chat_lift` on `(chat_id, lift_started_at)`) record the numbers behind an automatic trigger:
  `trigger_kind` is `manual`, `join_surge`, `flood` or `ai_burst`, and all three numbers are 0 for a manual lockdown
  (for `ai_burst`, `trigger_count` is the spam verdict items, `trigger_window` 120 and `trigger_percent` 0; the 8
  characters fit the 16 of the column, which has no `CHECK`, so the kind needed no migration). The numbers are
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
- `chat_settings_access` (migration 20261104120000) is the owner switch "Admins can change settings", one row per chat; a
  chat without a row means owner only. It is read only through `menuaccess.AdminsAllowedFresh`, a fresh query that is never
  cached (so no `DeleteCache` applies, and a cache added later needs `skipLocal` and a `DeleteCache` on every write), and an
  unreadable switch is an error the caller refuses on, never a guessed value. `menuaccess.SetAdminsAllowed` is one
  column-targeted `INSERT ... ON CONFLICT (chat_id) DO UPDATE` of the switch, `updated_by` and `updated_at`, so a Go `false`
  is written (the model has no `default:` tag on the boolean). It is never part of backup, export, import or reset, so
  restoring a backup can never widen who may change a group's settings.
- `chat_admin_index` (same migration) is the bot's record of each group's creator and human administrators: one row per chat
  and user, `role` under the `chk_chat_admin_index_role` CHECK (`creator` or `administrator`), unique on `(chat_id, user_id)`
  and an index on `user_id`. It is a hint for the private settings list and never authority: every pick from the list is
  checked against a live `getChatMember`. `adminindex.ReplaceChat` takes a per-chat `pg_advisory_xact_lock` as the first
  statement of its transaction on PostgreSQL, then upserts every entry and prunes every other user of the chat, never delete
  then insert (two replaces of one chat would race into a unique violation, and two replaces with disjoint rosters would
  commit the union); `ClearChat` goes through the same locked transaction as an empty roster. `ListForUserFresh` lists a
  chat when the user has a creator row, and also when the user has an administrator row and either the chat's switch is on
  or the chat has no creator row at all; it does not filter on `chats.is_inactive`, because the activity monitor marks quiet
  groups inactive. It is read fresh, never cached, and never part of backup, export, import or reset.
- `greeted_members` (migration 20261105120000) is the Greet once record: one row per `(chat_id, user_id)` the bot
  welcomed while the welcome's `welcome_once` switch was on, with a primary key on both columns, `created_at` and no
  foreign key (the claim must work for a chat the `chats` table does not know yet). It is read fresh through the
  database, never cached, so no `DeleteCache` applies. It is never part of backup, export, import or reset
  (`TestGreetedMembersStayOutOfBackup`) and rows are kept for good. `models.GreetedMember` maps it. The claim rule:
  `SendWelcomeMessage` claims the member with `greetings.ClaimGreeting` (one `INSERT ... ON CONFLICT DO NOTHING`) after
  the Welcome-on check and before the message is built and sent, while `welcome_once` is on; a lost claim sends nothing
  and runs no clean-old step, a claim database error sends nothing (fail closed), and so does a failed greetings read
  (`SendWelcomeMessage` reads through `greetings.LoadGreetingSettings`, which returns the read error, and sends and claims
  nothing on it, `TestGreetOnceReadErrorFailsClosed`; `GetGreetingSettings` and every other caller keep the fallback
  defaults on a read error), a definite refusal
  (`welcomeDefinitelyNotSent`: `media.ErrNoPermission`, a Telegram 4xx other than 429 by code, or a 429) gives the claim
  back with `greetings.ReleaseGreeting`, and an unknown outcome (a transport error, a timeout, a 5xx, an unreadable
  answer) keeps it, because a double welcome is worse than a missing one; a panic between the claim and the send
  (`SendWelcomeMessage`'s own recover, `TestGreetOncePanicBeforeSendReleasesClaim`) gives the claim back too, since nothing
  was sent, while a panic after the send was attempted keeps it (`TestGreetOncePanicAfterSendKeepsClaim`). The claim never uses Redis or
  `claimRecentJoinProcessing`, so it holds across replicas and with no Redis. The switch is written by
  `applyWelcomeOnce` (`greetingWrites.WelcomeOnce`), the one write path of the Welcome page's Greet once switch (control
  `o`, no command).
- `chat_requirements` (migration 20261106120000) is the member requirements setting, one row per chat: eight `SMALLINT`
  action columns (`last_name_action`, `username_action`, `russian_action`, `arabic_action`, `chinese_action`, `photo_action`,
  `channel_action`, `ai_action`, 0 off, 1 warn, 2 kick, 3 ban, 4 mute, each under its own named CHECK
  `chk_chat_requirements_<rule>` for 0..4, the last three `chk_chat_requirements_photo`, `chk_chat_requirements_channel` and
  `chk_chat_requirements_ai`) and the `check_on_join`
  and `delete_messages` switches. Migration 20261108120000 added `photo_action` and `channel_action` (the profile photo rule
  and the channel subscription rule, rule IDs `photo` and `channel` in `memberreq.Rules` after the five name rules) with two
  `ALTER TABLE ... ADD COLUMN IF NOT EXISTS` statements, each `NOT NULL DEFAULT 0` with its CHECK inline (no top-level
  `ADD CONSTRAINT`, so both appliers run it unchanged), so a row written before it, and a cached value the previous build wrote
  (msgpack decodes the two missing fields as 0), reads both rules off. Migration 20261109120000 added `ai_action` (the AI spam-name rule, rule ID `ai`,
  `memberreq.RuleAI`, appended to `memberreq.Rules` after `channel`) with one `ADD COLUMN IF NOT EXISTS` statement, `NOT NULL
  DEFAULT 0` with its CHECK inline, so a row written before it, and a cached value the previous build wrote, reads the rule
  off. A chat without a row means every rule off and both
  switches off. It is read through
  `cache.GetFromCacheOrLoad` under `cache.CacheKey("member_requirements", chat)` (`memberreq.GetSettings`; `GetSettingsFresh`
  is the same query without the cache) and creates no row. `memberreq.SetRuleAction`, `SetCheckOnJoin` and
  `SetDeleteMessages` are three column-targeted `INSERT ... ON CONFLICT (chat_id) DO UPDATE` of their own column plus
  `updated_at`, so a Go 0 or `false` is written and two admins changing two rules never clobber each other, each followed by
  `cache.DeleteCache` of that key; a value outside 0..4 or an unknown rule is refused before any write. The model has no
  `default:` tag on its booleans. The key `member_requirements:*` is listed in `skipLocal`, because the watcher reads it on
  every supergroup message and a rule switched off must stop punishing on every replica at the next message, not after
  `CACHE_LOCAL_TTL` (D-20). It is never part of backup, export, import or reset.
- `fuku:memberreq:pass:<chat>:<user>` is the member requirements watcher's clean verdict (`memberreq_watch.go`): positive
  only, a 16 hex fingerprint (`memberReqPassValue`, a SHA-256 of the rule actions (every rule of `memberreq.Rules`, eight since migration 20261109120000, which changed every member's fingerprint once more: one more harmless pass-cache miss) and the three names, so no name is in
  the key or the value and a rename or any settings change is a miss), `SET EX` 30 minutes (`memberReqPassTTL`), written only
  after a clean verdict and a successful clean predicate (`ClearHoldOnClean`) for a message not older than 2 minutes
  (`memberReqPassMaxAge`, so a backlog message cannot hide a later hold from the predicate). It saves the clean `DELETE`, not
  the rules, and a violation is never cached. A Redis error or no Redis is a miss and the predicate runs on every message
  (correct, slower). In a group with a lookup rule on the key is read in the same pipelined round as the lookup positives
  but is never consulted without them and never stands in for a lookup verdict: a pass hit with no photo positive still
  hands the member to a job, so a pass can never hide a missing photo or subscription (R2); the same holds for the AI
  rule: the pass key never stands in for the AI positive (a verdict below the bar, or a baseline equal to the names with no
  marker). It sits in the
  `fuku:memberreq:*` family, so `CLEAR_CACHE_ON_STARTUP` keeps it.
- `fuku:memberreq:photo:<user>` is the positive-only profile photo verdict of the lookup layer (`memberreq_lookup.go`,
  `memberReqPhotoRule`): the value is `1`, `SET EX` 30 minutes (`memberReqPositiveTTL`), written inside the flight only when
  `getUserProfilePhotos` answered with a total above zero or a non-empty list. A missing photo and every unknown answer (an
  error, a 400, 403 or 429, a timeout, a shed, a cancelled context, a null body) are never cached. The key holds no chat,
  because a photo is the user's, and no name or file ID. `CLEAR_CACHE_ON_STARTUP` keeps it and no Redis is a miss. The
  cost is one paced lookup per message of a member who stays without a photo (D-24). A `Fresh` lookup ("I fixed it", the
  page) bypasses it and drops a stale one on a definite breach.
- `fuku:memberreq:lc:<chat>` is the group's linked channel (`memberReqLinkedChannel`, `memberReqLinkedKey`): the decimal
  channel ID (a negative number) or `0` for a group with none. A positive channel is `SET EX` 60 seconds
  (`memberReqLinkedChannelTTL`) and a none 10 minutes (`memberReqLinkedTTL`): a stale none only makes the rule inert, but a
  stale channel ID is the dangerous direction (Phase 14 review WR-05: an owner who unlinks or relinks the group would
  otherwise see members punished for a channel the group is no longer linked to for up to ten minutes), so it is short and
  the breach path also reads the link fresh (below). The channel comes only from `getChat(group).LinkedChatId`, asked through the lookup pacer in the
  flight `l:<chat>`, and never from callback data or an update, so nobody can point the rule at a channel of their own. An
  error, a shed, a timeout and a null answer are not known and write nothing, a `linked_chat_id` that is positive (no
  channel has one) reads as none and is not cached, and a cached value that is not zero or negative is a miss. A `Fresh`
  read bypasses it and refreshes it on a definite answer. It sits outside the `fuku:cache:` prefix.
- `fuku:memberreq:sub:<channel>:<user>` is the positive-only subscription verdict (`memberReqChannelRule`,
  `memberReqSubKey`): the value is `1`, `SET EX` 30 minutes (`memberReqPositiveTTL`), written only when
  `getChatMember(channel, user)` answered creator, administrator, member, or restricted with `is_member`. A not-subscribed
  answer, an inert verdict and every unknown one are never cached, so each message of a member who stays unsubscribed costs
  one paced lookup (D-24). A `Fresh` lookup bypasses it and drops it on a definite breach.
- `fuku:memberreq:ba:<channel>` is the bot's definite administrator status in a channel (`memberReqBotChannelStatus`,
  `memberReqBotAdminKey`): the value is `1`, `SET EX` 2 minutes (`memberReqBotAdminTTL`), written only when the bot's own
  `getChatMember(channel, bot)` answered administrator or creator. The settings page's status line and an unanswerable
  member lookup read it (the breach gate below does not, see SEC-03); a not-administrator is cached only as the channel-level `cna` fact below and an unknown status is never
  cached, so `ba` bounds how long a demoted bot is still read as an administrator by the settings page's status line and
  by an unanswerable member lookup, but never by the breach gate: that reads the standing fresh (SEC-03, below). A `Fresh`
  read bypasses it and drops a stale one on a definite not-administrator.
- `fuku:memberreq:cna:<channel>` is the channel-level "the bot is not an administrator of this channel" fact
  (`memberReqChanNotAdminKey`, WR-01): the value is the fixed marker `1`, `SET EX` 60 seconds (`memberReqChanNotAdminTTL`),
  keyed by the channel ID alone, never by a user, and never a verdict about a member or a breach. The bot-status flight
  (`memberReqBotChannelStatus`) writes it whenever the bot's own `getChatMember(channel, bot)` establishes a definite
  not-administrator (a member, restricted, left or kicked status, or a Telegram 400 or 403 by `cache.IsBotGoneError`, the
  classifier of the `ba` key), whether the question came from a breach candidate (the D-22 gate), from an unanswerable member
  lookup (the punishing path asks the bot's standing too, once per channel per flight, and stays unknown for that one call;
  only a `Classify` caller reads it as inert) or from the settings page's fresh read. A 429, a shed, a 5xx, a transport
  error, a timeout and a null or unrecognised answer are still unknown and write nothing. It is read in two places: at the top
  of `memberReqChannelRule`, once the link is known, where a set key makes the rule inert at once with no Telegram call (never
  a breach, never an unknown), and in the watcher's cached read (`memberReqLookupCached`), where it makes the channel part
  satisfied so no job starts. A definite administrator answer deletes it, and a `Fresh` lookup ("I fixed it", the page)
  bypasses it. This amends D-24 for this one channel-level fact only: D-24's rule that no verdict about a user is ever
  cached (a missing photo, a missing subscription, an unknown answer) stands. It bounds the cost of a group linked to a
  channel the bot cannot read (below): after the first message the group costs one `getChatMember` of the bot per channel
  per minute, not a runner job and paced calls per message. It sits in the `fuku:memberreq:*` family, outside the cache
  prefix.
- `fuku:memberreq:nv:<verdict key hash>` is the AI spam-name rule's verdict cache (`memberreq_ai.go`, `memberReqAIVerdictKey`):
  the value is the spam probability as a decimal string (`memberReqAIParseProbability` is the one place it is parsed: a
  finite number from 0 to 1, else a miss, `math.IsNaN` and `math.IsInf` before the range test), `SET EX` 24 hours
  (`memberReqAIVerdictTTL`), written by the flight only after a definite answer, a pass and a breach alike and never an
  unknown, so a retuned bar applies to cached verdicts at once. The key is the first 16 bytes of the SHA-256 of the prompt
  version tag `mrv1`, the model and the names hash (`namecanon.VerdictKey`), so a model change or a changed instruction never
  serves an old verdict; it holds a hash and a probability and never a name, and it sits outside the `fuku:cache:` prefix, so
  `CLEAR_CACHE_ON_STARTUP` keeps it. It amends Phase 14's "positive only" rule for this one string-level fact: a verdict about
  a member is still never cached, only one about a string of names.
- `fuku:memberreq:nm:<user>` is the AI spam-name rule's baseline (`memberreq_ai.go`, `memberReqAIBaselineKey`): the names hash
  (32 hex characters, never a name) of the names the bot last accepted for this account, `SET EX` 180 days
  (`memberReqAIBaselineTTL`; the marker below keeps `memberReqAIStateTTL`, 30 days). The TTL is absolute and nothing
  refreshes it (the watcher is read-only and an equal baseline is a no-write pass), so a member dormant for over 180 days
  is first-sighted again and baselined with no call, whatever names they carry then (accepted, the same D-02 limit as an
  established member who was in the group when the rule was switched on; Phase 15 review WR-02 raised it from 30 days,
  which reopened that hole for every member every month). A name belongs to the account, so the key has no chat. Only the rule writes it, never the watcher:
  `SET NX` for an established member's first sighting (which makes no call), and overwritten by every definite pass together
  with the deletion of the marker below, in one transaction. A baseline that differs from the current names hash is a rename
  and is judged. It sits outside the `fuku:cache:` prefix, so `CLEAR_CACHE_ON_STARTUP` keeps it; Redis only.
- `fuku:memberreq:nu:<chat>:<user>` is the AI spam-name rule's new-member marker (`memberReqAIMarkerKey`): the value is the
  digit `1`, `SET EX` 30 days. A membership belongs to a group, so the key has the chat. The join entry sets it before its
  Check-on-join branch while the rule is live (`memberReqAIMarkJoin`, whatever the raid settings and Check on join say), and
  the rule sets it on every breach, also one read from the verdict cache; every definite pass deletes it together with
  `nm = h`, and an unknown writes nothing. It exists because "no baseline" is two different members: an established one the
  rule has never seen, and a new joiner whose join call was unanswered. Without the marker that joiner would be baselined at
  their first message and never judged, and a baseline that expired during a long hold would launder a member whose names were
  judged spam (a breach keeps the marker, so the member is judged again instead). Like `nm` it is Redis only, outside the
  cache prefix and kept by `CLEAR_CACHE_ON_STARTUP`. The pass key never stands in for the AI positive.
- `fuku:memberreq:aib:u:<user>:<yyyymmddhh>`, `fuku:memberreq:aib:c:<chat>:<yyyymmddhh>` and `fuku:memberreq:aib:g:<yyyymmddhh>`
  are the AI spam-name check's hourly budgets (`memberreq_ai_budget.go`): 5 provider calls per account (fleet-wide: a name
  belongs to the account, not to a group, and renaming is the attack), 100 per group and 400 in all
  (`memberReqAIUserHourlyBudget`, `memberReqAIChatHourlyBudget`, `memberReqAIGlobalHourlyBudget`, package variables only so a
  test can shrink them). Each unit is one `INCR` plus a 2 hour expiry in one transaction through the stateless
  `raidAIBudgetTake` (the seam `memberReqAITake`) on the UTC hour of `raidNow` (`raidAIHourStamp`). `memberReqAIReserve` takes
  the account's unit first, then the group's, then the fleet's, and a refusal or an error stops the chain, so an account over
  its own budget spends neither shared unit and a group over its own never spends the fleet's. The unit is reserved before the
  call, so a failed call still spends it. A verdict served from the cache is answered before any budget and spends none. A
  used-up allowance and a Redis error on a counter (and no Redis client) are both sticky unknowns (WR-03: states of the
  system, not faults of one answer): either way no call is made, never an unbounded one. The family is the name check's own, separate from the raid image budgets (`fuku:raid:*`): a name burst never
  spends the image allowance and the reverse, and only the stateless helpers are shared. Every key is used in single-key
  commands, so there is no hash tag; they sit in the `fuku:memberreq:*` family, outside the cache prefix, so
  `CLEAR_CACHE_ON_STARTUP` keeps them.
- `fuku:memberreq:lkp:pace:next` and `fuku:memberreq:lkp:pace:block` are the fleet-wide pacing of photo and channel
  lookups (`memberReqLookupPaced`, `memberReqLookupPacer`, built from `memberReqLookupPacerOptions`): the staff pacer's
  slot-plus-block design (`ratelimit.NewTelegramPacer`) with Interval 100 ms, MaxRetries 1 and MaxWait 5 s. A slot, or a
  shared block after a 429, more than 5 s away is shed as unknown with no Telegram request and no slot taken. The keys are
  separate from `fuku:memberreq:pace:*`, so a burst of lookups never delays a mute notice, and only lookups use them: the
  settings page and "I fixed it" go through the same lookup layer, while the live `getChatMember`, a punishment, a notice
  and a delete stay on the requirement pacer.
- The lookup flights (`memberReqFlight` over the generic `memberReqFlightValue`, `memberReqLookupFlight`, a
  `singleflight.Group`) make every concurrent lookup of one key one Telegram call: the key is `p:<user>` for the photo,
  `l:<chat>` for a group's linked channel, `s:<channel>:<user>` for a subscription and `b:<channel>` for the bot's status in
  a channel (the families differ, because their values have different types). A Fresh lookup (`mode.Fresh`: "I fixed it",
  the channel screen's reads) uses the same key behind an `f:` prefix (`memberReqFlightKey`), so a Fresh caller never joins
  a running non-Fresh flight, which can answer from a stale positive cache read, and a non-Fresh caller never joins a Fresh
  one, while a hundred callers of one mode still make one request (`TestMemberReqLookupFlightsKeepModesApart`). Every
  non-Fresh flight (the photo, the link, the subscription and the bot's status) reads its cache once more inside the
  flight, so a caller that missed the cache just before the previous flight wrote it makes no second request
  (`TestMemberReqFlightsReadCacheAgain`). The flight
  function runs the paced call and the cache write on a context derived from `memberReqContext()` and bounded by
  `memberReqLookupFlightTimeout` (15 s), never from a caller, so `StopMemberReqJobs` ends every flight and one caller's
  short deadline never cancels a lookup that others share; every caller waits with `select` on its own deadline and gets
  unknown (no value) when it runs out. A lookup returns a `memberReqVerdict` whose zero value is `memberReqUnknown`, so a
  missing verdict, a recovered panic and a shed all do nothing; only `memberReqBreach` can lead to a punishment. The AI row
  adds the family `n:<verdict key hash>`, so every concurrent judgement of one string of names is one provider call, and,
  unlike the lookups above, it ignores `mode.Fresh` and `mode.Classify` (the AI row answers from the cache whatever the mode).
- `fuku:memberreq:warned:<chat>:<user>` is the recorded-warn note of the member requirements executor
  (`memberreq_warned.go`, WR-01): the value is the decimal ID of the hold whose warn was just stored, `SET EX` for
  `memberreq.StaleClaim` plus 10 minutes (`memberReqWarnedGrace`), written inside the warn lock before the hold is marked
  punished and deleted once the mark landed, so it survives only when the mark failed three times (or the process stopped
  between the two). `memberReqAdoptRecordedWarn` reads it when a stale pending Warn hold is reclaimed for another Warn and
  compares the value with the hold's ID, so a note an earlier episode left never matches a later hold. Redis only and best
  effort (no Redis or a Redis error is "no note", so the reclaim warns again, accepted); it sits in the `fuku:memberreq:*`
  family, so `CLEAR_CACHE_ON_STARTUP` keeps it.
- `chat_requirement_holds` (same migration) is one member's requirement episode in one chat, unique on `(chat_id, user_id)`
  (`uk_chat_requirement_holds_chat_user`): both the latch that makes a violation punished once across replicas and the
  record of a mute the bot placed. `memberreq.ClaimHold` is one `INSERT ... ON CONFLICT DO NOTHING` and only the caller whose
  insert landed (`RowsAffected == 1`) acts; a pending hold (not yet confirmed punished) older than `memberreq.StaleClaim`
  (2 minutes) is taken over by `ReclaimStaleHold`, one conditional update with one winner. The other operations (mark
  punished, notice ID, welcome held, backoff, revert to pending, take by ID with `DELETE ... RETURNING`, restore, the clean
  predicate, clear all, take before a join) are fresh database statements too. The hold is read fresh, never cached (so no
  `DeleteCache` applies), never touches Redis and works without it, and is never part of backup, export, import or reset.
  A detected leave (`leftMember`) deletes every hold of the member in the chat (`ClearHolds`); a rejoin deletes only the
  holds created before the join's own date (`TakeHoldsBefore`) and lifts a live marker mute of a mute hold first, punished or
  still pending (WR-03: the restrict may have taken effect before the hold was marked; restoring the hold on failure), and a leave that Telegram does not report (restricted to restricted) is settled the same
  way at the rejoin, never by deleting a mute hold at the leave.
- `chat_delete_rules` (migration 20261110120000, with `scheduled_deletions` below) is one chat's delay for one kind of
  message, one row per `(chat_id, kind)`: `kind` is `service:<type>`, `bot:<kind>` or one of the three `member:*` kinds of
  Phase 17 (`fuku/db/deletion/kinds.go`, 20 kinds, validated in Go by `deletion.ValidKind` and never by a CHECK, so a later
  type needs no migration) and `delay_secs` is 0
  for "delete now" and 1..86400 for "delete after N seconds" under the named CHECK `chk_chat_delete_rules_delay` (0..86400).
  The delay a rule may store is checked by value per kind by `deletion.ValidateRuleDelay`, called by `SetRule` and by the
  menu's `applyDeleteRule` before any write (a refusal is `ErrDelayOutOfRange`): `member:commands` accepts 0 only (a stored
  0 means On), `member:edited` 30, 60, 300, 900 or 3600 (the grace in seconds) and `member:selfdestruct` 60, 300, 600, 3600,
  21600 or 86400, while a service or bot kind keeps 0..86400, so a database edit cannot create a grace no page offers. A
  stored value a rule does not accept is treated as Off by that rule's reader (the readers of the command, edit and
  self-destruct rules rely on this; `deletion.NeedsDeleteRight` is true for a service or member kind and false for a bot
  kind, which needs no delete right).
  A chat with no row for a kind means Keep (Off for a bot kind), and a read creates nothing. It is read through
  `cache.GetFromCacheOrLoad` under `cache.CacheKey("delete_rules", chat)` (`deletion.GetRules`, one `Rules` struct whose
  `Delays` map holds only the kinds that have a row: a stored 0 is present, an absent kind has no rule; `GetRulesFresh` is
  the same query without the cache), the key `delete_rules:*` is listed in `skipLocal` (the watcher and every send site
  read it, so a rule switched off must stop acting on every replica at the next message), and every write drops it
  (`SetRule` is a column-targeted `INSERT ... ON CONFLICT (chat_id, kind) DO UPDATE` of `delay_secs` and `updated_at`, a Go 0
  written, the kind and the range checked before any write; `RemoveRule` deletes the row and, in the same transaction, the
  chat's PENDING `scheduled_deletions` rows of exactly that kind, WR-03). Switching a rule off stops what was already queued
  too: `dropPendingKindsTx` matches the kind exactly (never a prefix), leaves `acting` rows to the worker that claimed them
  and never touches another chat or another kind, and a change of delay or a Now or After choice drops nothing. The model
  carries no `default:` tag. The `service:new_chat_members` and `service:left_chat_member` rows only hold the delay: whether those two kinds act at
  all is the shared `/cleanservice` flag (`greetings.clean_service_settings`), written together with the rows in ONE
  transaction by `deletion.ApplyJoinLeftRule` (Keep: flag false, both rows removed and the pending queue rows of both kinds dropped; Now: flag true and this kind's
  row removed; After N: flag true and this kind's row upserted) and, for the `/cleanservice` command, by
  `deletion.ApplyCleanServiceFlag` (the flag on or off and BOTH rows removed, so a stale delay never comes back; off also
  drops the pending queue rows of both kinds in the same transaction, on drops none); both
  drop the `greetings` and `delete_rules` keys after the commit, and `greetings.SetCleanServiceTx` guarantees the chat and
  greetings rows and writes the flag with a column-targeted update (the column's `default:false` tag would swallow a Go
  false on a plain Create). The table is never part of backup, export, import or reset
  (`TestDeletionTablesStayOutOfBackup`).
- `scheduled_deletions` (same migration) is the durable queue of messages waiting to be deleted: `id`, `chat_id`,
  `message_id`, `kind`, `sent_at` (the message's own date, which the worker uses to drop a row Telegram can no longer
  delete) and `delete_at` (`sent_at` plus the delay), `state` (`pending` or `acting` under `chk_scheduled_deletions_state`),
  `claim_token`, `claimed_at`, `attempts` (`chk_scheduled_deletions_attempts`) and `created_at`, unique on `(chat_id,
  message_id)` (`uk_scheduled_deletions_message`), with the partial indexes `idx_scheduled_deletions_due` (`delete_at` where
  pending) and `idx_scheduled_deletions_acting` (`claimed_at` where acting). Times are stored as UTC at second precision
  (SQLite compares them as text). `deletion.Schedule` is the one writer: it validates the kind, the delay and the IDs (a chat ID
  that is not negative is `ErrInvalidEntry` before any read, so a private chat is never queued, whatever a caller checked),
  refuses a Staff Group chat after a fresh `staff.GetStaffGroupFresh` read (a read error writes nothing) and upserts so that
  scheduling one message twice keeps the earlier `delete_at` (`LEAST(scheduled_deletions.delete_at, EXCLUDED.delete_at)` on
  PostgreSQL, `MIN(scheduled_deletions.delete_at, excluded.delete_at)` on SQLite, the table-qualified column being
  required because an unqualified one is ambiguous on PostgreSQL) and never rewrites the kind or `sent_at`. `ClaimDue`
  claims due pending rows with one conditional `UPDATE` (state acting, a `crypto/rand` token, `claimed_at`, re-checking
  `state = 'pending'`) and reads them back by token, so among concurrent callers each row has one winner; `Complete` and
  `Release` touch only rows that still carry the caller's token (a late completion of a stale claim removes nothing, and an
  empty token is refused), a counted `Release` adds an attempt and moves `delete_at` to the retry time the caller passes (UTC,
  whole seconds, a Go-computed time so SQLite and PostgreSQL run one statement; a counted release with no retry time is
  `ErrInvalidRetryAt`), the third counted `Release` deletes the row instead, an uncounted `Release` (a 429, a shutdown) leaves
  `delete_at` alone, and `ReleaseStale` returns old acting rows to pending with no attempt. A queue entry's delay is checked
  by `deletion.ValidateEntryDelay` in `Schedule` and `ScheduleBatch` (a `member:commands` or `member:edited` entry takes 0
  only, being the due-now fallback of an inline delete; `member:selfdestruct` the six presets; other kinds 0..86400).
  `deletion.ScheduleBatch` writes self-destruct entries in multi-row statements of at most `ScheduleBatchMax` (100) rows
  after merging entries of one `(chat, message)` to the earliest time in Go, because one PostgreSQL statement cannot touch a
  key twice (error 21000), sorted by `(chat, message)` so concurrent batches lock rows in one order, with the same
  table-qualified earliest-wins update as `Schedule`; it validates every entry before any write and refuses a Staff Group
  with one fresh read per distinct chat (a Staff Group chat's entries are dropped, a failed read drops that chat's entries
  and is returned while other chats are written). `deletion.CountPending` is the exact count of a chat's queued rows of one
  kind bounded by a limit (`SELECT count(*) FROM (SELECT 1 ... LIMIT n)`, served by `uk_scheduled_deletions_message`) and
  `deletion.DropPendingMessage` deletes the one pending row of a `(chat, message, kind)`; no migration was added for them.
  The queue is read and written only through fresh queries, so no `DeleteCache` applies, and it is
  never part of backup, export, import or reset. The worker (`deletion_worker.go`) claims at most 1000 due rows per cycle
  (`deletionBatch`) and screens the claimed member:selfdestruct rows against the chat's current rule, releases claims older than 2 minutes (`deletionStaleClaim`) without counting an attempt, ends a cycle
  90 s after its claim (`deletionCycleGuard`, under the stale release, so a live cycle's rows are never handed to a second
  replica: what it has not finished is released free before it returns), drops a row whose `sent_at` is more than 47 hours
  old (`deletionDropAge`) with no call, and gives a row three counted tries (`deletionMaxAttempts`, a 5xx, a transport error or a per-call timeout; a 429
  and a shutdown cost none) spaced by `deletionRetryBackoff` (60 s, WR-02): a counted failure moves the row's `delete_at` to
  the cycle clock (`deletionNow()`) plus the backoff, so the tries are not re-claimed on consecutive 1 s ticks and a few
  seconds of Telegram trouble no longer drops a row (the 47 hour drop still bounds a row's life).
- `fuku:delete:pace:next` and `fuku:delete:pace:block` are the fleet-wide Telegram pacing of the deletion code
  (`deletionPaced`, `deletionPacer`: 100 ms, 3 retries, 60 s): the staff pacer's slot-plus-block design with its own keys, so
  deletions, member requirements, lockdown and staff fan-outs each keep their own budget (`TestDeletionPacerIsIndependent`).
- `fuku:delete:norights:<chat>` is the 10 minute negative latch of a chat the bot cannot act in (`deletionLatchKey`,
  `deletionLatched`, `deletionSetLatch`, `deletionLatchTTL`), outside the `fuku:cache:` prefix so `CLEAR_CACHE_ON_STARTUP`
  keeps it. Its value is the scope: `gone` (the bot is not in the chat, was kicked, or the chat is gone or was upgraded: stops
  every kind, set from the classifier, overwriting) or `service` (stops every kind that needs the right to delete other people's messages, the `service:*` and the `member:*`
  kinds, `deletion.NeedsDeleteRight`, and never a `bot:*` kind; Phase 17 widened its meaning from service kinds only, the stored
  value and the key are unchanged; set only after a live right check, `SET NX`, so it never downgrades `gone`). A
  message-level answer (`message to delete not found`, `message can't be deleted`) never sets a latch by itself, because a
  plain-member bot can still delete its own messages; for a `service:*` or `member:*` kind it only leads to the live right
  check (below), and only that check's `cannot` answer sets the `service` scope. No Redis and
  a Redis error read as "not latched": the latch only saves calls.
- `fuku:delete:chk:<chat>` is the guard of the inline latch decision (`deletionCheckKey`, `deletionTakeCheck`,
  `deletionCheckTTL`, SEC-02): `SET NX EX 600` with the value `1`, taken after an inline `service:*` or `member:*` delete was refused with a
  message-level answer, so the live right check (`deletionDecideServiceLatch`) of a chat runs at most once per chat per 10
  minutes across replicas. It sits in the `fuku:delete:*` family, outside the `fuku:cache:` prefix, so `CLEAR_CACHE_ON_STARTUP`
  keeps it. No Redis, a Redis error and a lost `SET NX` all mean no guard and no check (the latch only saves calls, so this
  fails quiet and never loops); the key is taken whether the check then says can, cannot or unknown, so an unknown answer
  (a 429, a 5xx, a timeout) also waits out the 10 minutes before the next try. A bot kind never takes it.
- The same migration adds the Phase 12 message columns: `greetings.welcome_media_below`, `goodbye_media_below` and
  `welcome_once` (the Media under text switches and Greet once), and `rules.file_id` (empty for no media), `media_type`
  (0 means no media), `btns` (the rules message's URL buttons, jsonb, `'[]'`) and `media_below`. Every column is
  `NOT NULL DEFAULT` its zero value, so a row saved before it reads exactly as it did, and the Go models
  (`WelcomeSettings.MediaBelow` and `Once`, `GoodbyeSettings.MediaBelow`, `RulesSettings.FileID`, `MediaType`, `Buttons`
  and `MediaBelow`) carry no `default:` tag on any boolean. They travel in backup JSON through their model tags
  (`media_below`, `once`, `file_id`, `media_type`, `btns`); an export made before the migration has none of those keys and
  imports as zeros, the old behaviour (`TestOldGreetingsExportDecodesToZeros`). `RulesSettings.HasContent` (a non-blank
  text, a file ID or at least one button) is the one test of "Rules is set".
- `UpdateRecord` skips zero values. Use `UpdateRecordWithZeroValues` to write `false`/`0`/`""`. Both return
  `gorm.ErrRecordNotFound` when no row matched.
- Check `TableName()` before raw SQL: `ConnectionSettings→connection` (per user), `ConnectionChatSettings→
  connection_settings` (per chat), `AdminSettings→admin`, `DisableSettings→disable`.

## Migrations

- `migrations/*.sql` is the schema source of truth. Never edit an applied file; the SHA-256 checksum aborts startup.
- Name new files with a timestamp greater than every existing one; order is plain filename sort.
- Each file runs in one transaction: no top-level `BEGIN`/`COMMIT`/`ROLLBACK`, no `CREATE INDEX CONCURRENTLY`.
- Change `fuku/db/migrations/runner.go` and `scripts/migrate_psql.sh` together; both appliers must agree.

## Adding a module

1. Add a migration in `migrations/` (rules above).
2. Add the model in `fuku/db/models/` with `TableName()` returning the migration's table name.
3. Add the repository in `fuku/db/<domain>/`: reads through `cache.GetFromCacheOrLoad`, `cache.DeleteCache` on writes.
4. Add the model to the `AutoMigrate` list in the affected `testmain_test.go` or `internal/testdb.Run` call. Do the same
   whenever you change an existing model.
5. Add `fuku/modules/<name>.go` with `LoadXxx`, registered in `init()` via `RegisterLegacyModule`.
6. Register commands with `helpers.WrapCommand`; add `RegisterAnonymousAdminHandler` for admin commands.
7. Add locale keys to all 8 files in `locales/` (en, es, fr, hi, id, pt, ru, uk).
8. Run `make generate-docs`, `make check-translations`, `make test`, `make lint`.

## Subsystem traps

- Approved users skip antiflood, locks, blacklists, and captcha.
- Antiflood first write (D-27): on a chat with no `antiflood_settings` row, the first `/setfloodmode` (pinned for ban) or
  `/delflood` stores `flood_limit` 5, the model's `default:5`, because the repository's `FirstOrCreate` insert leaves
  the limit at its Go zero value; an admin who only picks an action or turns message deleting on therefore also turns
  antiflood on at 5 messages. `TestSetFloodModeOnFreshChatStoredLimit` and `TestSetFloodMsgDelOnFreshChatStoredLimit`
  (`fuku/db/antiflood`) pin it, and a Mute press on an unconfigured chat writes no row at all (the cached default is
  already mute). The menu's action and delete presses call the same `applyFlood*` functions, so it keeps this on
  purpose and draws the stored row from their database read-back, never a guess. Because a tap in a menu is easier to
  hit by accident than a typed command, the menu's action and delete presses also show the notice
  `settings_antiflood_now_on` (menu only, with the stored limit) when the limit read before the write was 0 and the
  read-back is not (`settingsFloodTurnedOn`, D-34); a limit press never shows it, and the commands' replies are unchanged. A repository fix (insert with limit 0
  on the first write) would change `/setfloodmode` and `/delflood` too, so it is a separate follow-up and must update
  those two tests and this bullet.
- Antiflood counters are in-process (per replica). The raid detection counters (`fuku:raid:*`) are Redis-only: without
  Redis the automatic triggers do nothing, and `/lockdown` still works.
- Fed-ban lookups cache per `(fed, user)` with a negative sentinel; every ban/unban write must invalidate it.
- A join arrives as both `ChatMemberUpdated` and a service message; dedupe through `claimRecentJoinProcessing`
  (the lockdown guard, which runs first, dedupes by its joiner row instead and never gates a ban on Redis).
- Greet once (`welcome_once`, `greeted_members`): the only place that claims is `SendWelcomeMessage`, which the
  `chat_member` join, the join service message and the captcha pass (button and web, through `finishCaptchaPass`) all
  reach through `memberReqJoinThenWelcome` (the member requirements' join entry, below), so a member is claimed exactly
  where they are really welcomed: a member the requirements kick, ban or mute on join is never claimed, because the entry
  sends them no welcome. A captcha-failed joiner, a joiner the lockdown
  guard records for a ban, a join while the Welcome switch is off, any join while Greet once is off and every preview
  write no row, because none of them reaches the claim (`TestGreetOnceWelcomeOffNoClaim`, `TestGreetOnceOffNoRecord`,
  `TestGreetOnceCaptchaPathAndFailure`, `TestGreetOnceLockdownBanNoRecord`). `TestGreetOnceConcurrentOneWelcome` proves
  one welcome for 16 concurrent joins of one member. Accepted limits: a basic group's migration to a supergroup changes
  the chat ID, so its members are welcomed once more; members who join while the Welcome switch is off are not recorded
  and are welcomed once when it is back on; bots that join are claimed like any member; rows are kept for good. A 429
  counts as a definite refusal (Telegram answered without processing the request), so a member whose first welcome hit a
  flood limit is welcomed on their next join.
- Member requirements, the lookups (`memberreq_lookup.go`): the photo rule and the channel rule answer one
  `memberReqVerdict` whose zero value is unknown (the AI spam-name rule answers the same type and is described in its own
  bullet below). Unknown and inert both mean "not broken": a lookup that failed, was shed,
  timed out, was cancelled, came back null or with an unrecognised status is unknown, and a rule that cannot be judged here
  (a group with no linked channel, or a channel the bot does not administer) is inert; only `memberReqBreach` can lead to
  a punishment. The photo rule is a total of zero from `getUserProfilePhotos` (limit 1), a pass is a total above zero or a
  non-empty list. The channel rule (`memberReqChannelRule`) runs in a fixed order: the linked channel (unreadable is
  unknown, none is inert with no member request), the `cna` fact (inert at once), the subscription cache, then one
  `getChatMember(channel, user)` in the
  flight `s:<channel>:<user>` judged by `memberReqSubscribed` (creator, administrator, member and restricted with
  `is_member` pass; left, kicked and restricted without `is_member` are only a breach candidate; anything else is unknown).
  A not-subscribed candidate first re-reads the link fresh (Phase 14 review WR-05, `memberReqLinkUnchanged`: `getChat` through
  the lookup pacer in the flight `f:l:<chat>`, bounded, and not for a `Fresh` caller, which just read it fresh): a link that
  changed, was removed or cannot be read makes the verdict unknown, never a breach, and the fresh read refreshes `lc`, so a
  punishment never rests on a link older than one request and the next message judges the channel the group is linked to
  now. A subscribed member (a pass) costs no extra call. The D-22 gate (`memberReqGateBreach`): a candidate is a breach only
  while `memberReqBotChannelStatus` shows the bot as an administrator of the channel, inert when the bot is definitely not
  one, and unknown when that cannot be read, so a bot that cannot see a channel's members and is told "left" about everyone
  punishes nobody. SEC-03: the gate always reads the bot's standing fresh, never from the `ba` positive (a bot demoted
  within the last 2 minutes would still be believed an administrator, and a non-administrator bot may be told "left"
  about everyone): one coalesced `Fresh` bot-status flight (`f:b:<channel>`) through the lookup pacer, bounded, so only a
  breach candidate pays one extra request (a pass and a `Fresh` caller, which already reads fresh, pay none). A fresh
  answer that is not a definite administrator makes the candidate inert and writes the `cna` fact and drops the stale `ba`;
  an answer that cannot be established (a 429, a shed, a 5xx, a timeout, a null or unrecognised answer) is unknown, never a
  breach; a definite administrator caches `ba` again and the breach stands. The classifier
  `memberReqClassifyBot` reads the bot's own answer by status and by Telegram error code only: administrator and creator
  are an administrator; member, restricted, left, kicked and any Telegram 400 or 403 (`cache.IsBotGoneError`, also
  wrapped) are not; a 429, a shed, a 5xx, a transport error, a timeout, a null answer and an unrecognised status are
  unknown. It never reads a description (a 500 whose text says "chat not found" is unknown) and the channel path never
  calls `chat_status.IsBotMissingError`, whose markers read "chat not found" as the bot being missing; it never produces a
  breach. `memberReqLookupMode.Classify` is set only by the held release and "I fixed it": a member lookup that could not
  be answered is then inert when the bot is definitely not a channel administrator and stays unknown otherwise; the
  punishing path and the watcher never set it, and the punishing path's unknown stays unknown for that call, though it asks
  the bot's standing too and leaves the `cna` fact behind (WR-01). Positives are cached (the keys above), plus the
  channel-level `cna` fact, and no
  lookup has a new drain: the flights end with `memberReqContext()`. The cost to know: a verdict about a member that is
  inert or unknown is never cached, so a group whose linked channel the bot cannot read (the bot is no administrator there,
  or Telegram will not answer for its members) costs one runner job and the lookups of one message, then, once the bot's
  standing is known not to be an administrator, one `getChatMember` of the bot per channel per minute (the `cna` key, 60 s):
  while it stands the watcher starts no job and the executor makes no channel call. The expensive window is therefore the
  first message after the key expires (and a standing that cannot be established, a 429 or 5xx, writes nothing and costs the
  per-message job as before until it can) until the owner fixes the bot's standing or turns the channel rule Off (the
  watcher and the runner bullets under Subsystem traps give the cap and the shed). Logs of every lookup path carry chat, channel and user IDs and a Telegram error by its code only
  (`memberReqLookupLogFailure`, `memberReqErrText`); a transport error text is gotgbot's own, whose URL is already scrubbed of
  the bot credential, and `TestMemberReqLookupLogsCarryIDsOnly` holds it (a log of a non-gotgbot client's transport errors would
  need its own scrubbing: none exists, and the test relies on gotgbot's). A member can be stuck behind "I fixed it" only while a
  channel lookup for them stays unanswerable AND the bot is neither a definite non-administrator nor bot-gone (the classifier
  then keeps the rule unknown): an administrator's `/unmute` is the fallback, and the 330 day marker mute is the worst case.
  Two facts rest on live Telegram behaviour that no fake proves (a member who hides their photo reads as a total of zero,
  what a bot that is not a channel's administrator is answered, and whether `getChat` of a group reaches a plain-member bot
  with `linked_chat_id`); `.planning/phases/14-profile-photo-and-channel-rules/14-SPIKE.md` lists the owner's checks, and the
  code is safe under every outcome (an unknown never punishes, D-22 makes a status answer to a non-administrator bot inert).
- Member requirements, the rule table (`memberreq_rules.go`): `memberReqRules` has eight rows in the order of
  `memberreq.Rules` (`ln un ru ar zh ph ch ai`, bits 1 to 128; the next free bit is 256). The five name rows have a pure `Broken`
  test and no `Lookup`; the photo, channel and AI rows have a `Lookup` function (`memberReqPhotoRule`, `memberReqChannelRule`,
  `memberReqAIRule`) and no `Broken`, so the lookup rules are three and `memberReqLookupMode` gains `Names` (the live record's
  names, read only by the AI row), and exactly one of the two is set on every row (`TestMemberReqRuleTable`). `memberReqBroken` judges the name
  rules only: it skips a row with no `Broken`, so it never reports a lookup bit and a nil function is never called.
  `memberReqLookupBroken` folds the lookup verdicts into the same mask in table order (photo, channel, then the AI rule): it skips a row
  with no `Lookup` and a row whose action is Off, a `memberReqBreach` sets the row's bit, a `memberReqUnknown` sets only the
  returned unknown flag, and `memberReqPass` and `memberReqInert` add nothing, so a lookup row can add a bit only through an
  explicit breach; its one production caller is `memberReqFixedJudge` ("I fixed it", with `Fresh` and `Classify`), and the executor's `memberReqLiveViolation` (the executor bullet below) calls the same fold through `memberReqLookupMasks`, which also returns the unknown mask. `memberReqPassValue` fingerprints the eight rule actions. A row's `HintKey` is the hint its settings screen
  shows after the description.
- Member requirements, the AI spam-name rule (`memberreq_ai.go`): the eighth rule (`ai`, bit 128). It is judged only when
  `memberReqAIConfigured()` holds: `ENABLE_AISPAM` on and an Anthropic key set, never Redis and never the raid `ai_enabled`
  switch (they are other facts). `memberReqAIRule` runs in this order: no names in the mode is unknown, an AI that is not
  configured is inert, names that are not judgeable (`namecanon.Of`: all empty, or a lone "Deleted Account") are inert, no Redis
  or a Redis read error is unknown (the provider is not asked while the cache cannot be read), a cached probability is the
  answer, and otherwise one `claude.Client.ClassifyNames` call of the three canonical strings inside one flight per verdict
  key, on a context derived from `memberReqContext()` and bounded by `memberReqLookupFlightTimeout`. A probability at or above
  `memberReqAIBar` (0.9) is a breach and a lower one a pass, and the probability is cached; unknown and inert both mean "not
  broken", and every answer the process cannot turn into a probability (an HTTP error, a refusal stop, a missing tool call, a
  value outside 0 to 1, an unreadable body, a transport failure, a cancelled context) is unknown, leaves no hold, notice or
  cached verdict and punishes nobody. The names come from the executor's live `getChatMember` record (`memberReqLiveViolation`
  passes `Names`), exactly as the name rules judge them, so the rules never disagree, and the row is asked after the photo and
  channel rows, only after the exemptions, the hold read, the pause and the live eligibility check. The provider key travels in
  the `x-api-key` header only and no name, hash, probability, instruction or key is logged: a failure is one Warn line with the
  chat and user IDs, the HTTP status and the error type (`claude.HTTPError`), a refusal one Info line. The notice's remedy, the
  button and the page status of the rule are in the executor bullet, the `mreq|v1` bullet and the page `m` bullet.
  The cost guards (plan 15-06; D-10, D-12, D-22, D-24, D-26, D-29). Inside the flight, after the cache is read once more, the
  order is: the breaker, the in-flight gate, the three hourly budgets, the call, the breaker's record, the cache write. The gate
  (`memberReqAIGate`, `semaphore.NewWeighted(4)`) allows four provider calls at once per replica and is waited for at most
  `memberReqAIGateWait` (3 s) under the flight's own context; the slot is released by `defer` on every path, a recovered panic
  and a cancel included, and it is taken before the budget, so a flight that cannot get a slot spends no unit. The flight's
  value is `memberReqAIAnswer{Verdict, Sticky}`. A paused breaker, a used-up budget and a gate that could not be had while the
  flight's context is live are a sticky unknown (a state of the system), and so are no Redis client, a failed state read
  (`memberReqAIPrepare`, noted by `memberReqAIRule` through the plan's `sticky` field, because the prepare step runs before the
  flight), a failed cache re-read inside the flight and a budget counter that failed (Phase 15 review WR-03: a Redis outage
  would otherwise defer every weaker name punishment in every group whose AI action is stricter); a cancelled context is a
  plain unknown (shutdown is not a state), and so is every provider answer that is not a probability (a 5xx, a timeout, a 400). The name check has its own breaker
  (`memberReqAIBreaker`, `memberreq_ai_breaker.go`, state of its own, never shared with `raid_ai_breaker.go`): five provider
  failures in one group pause that group's checks for 5 minutes (`memberReqAICooldown`), a rejected key or account (401, 402,
  403, or an authentication, billing or permission error type) pauses every group for 5 minutes (`memberReqAIKeyPause`), and a
  429 whose retry delay is over 2 s pauses every group for that delay, at most 5 minutes, while a shorter one only counts
  toward its group. A refusal stop is an answer (it clears the count), a cancelled context is neither a failure nor a success,
  a pause that is running is not extended or logged again by checks still in flight, the map is bounded at 10,000 groups (an
  idle breaker is pruned to make room, and when every tracked group is failing or paused a new one stays untracked, with one
  warning per process), and each pause is one warning naming the status and the error type only. An image failure never
  pauses name checks and the reverse (`TestMemberReqAIBreaker`). The refusal latch (security SEC-02, `memberreq_ai_latch.go`,
  `memberReqAILatches`, state of its own, in memory and per replica) covers the sticky states the watcher cannot see, so
  that a marker holder's every message is not a runner job plus a paced live `getChatMember` that ends in nothing (an
  attacker can cause that: 20 accounts spend one group's 100 units): the AI row latches the chat when the budget reserve is
  refused for the GROUP's or the FLEET's allowance (`memberReqAICappedGroup`, `memberReqAICappedFleet`) until the end of the
  current UTC hour (`memberReqAILatchHour`, from `raidNow`), and for 60 s (`memberReqAILatchCooldown`,
  `memberReqAILatchBriefly`) when the gate stayed full while the flight's context was live or the budget counter failed. An
  ACCOUNT's own allowance (`memberReqAICapped`) latches nothing: it is that member's limit and the watcher cannot key on it
  (accepted, AR-18, one job per message of that member until their hour ends, at most their own messages). A fleet refusal
  latches only the chat whose call met it, so each other chat pays one job when it meets the refusal itself. The map is
  bounded at 10,000 chats like the breaker's (an ended latch is forgotten on read and pruned to make room, and when every
  tracked chat is still latched a new one stays untracked, with one warning per process), a latch never shortens a longer one,
  and a restart or a second replica starts with none. It changes nothing of what the executor, the join entry, the held
  release or "I fixed it" decide (they ignore it). The sticky bit reaches the executor without the executor
  naming the rule: `memberReqLookupMasks` stamps each row's bit into the context of its `Lookup`, `memberReqNoteSticky` ORs it
  into the run's `memberReqLookupTouch.sticky`, and the held release and "I fixed it" (no touch) still see an unknown. The
  stale-names guard is the table field `memberReqRule.NamesBound` (true on the AI row only, `memberReqNamesBoundMask`): after
  the post-lookup re-read the executor compares the canonical names hash (`memberReqAINamesHash`: the empty string when not
  judgeable, so an invisible character is no change and a change from judgeable to not is one) of the record the lookups judged
  with the hash of the re-read record, and drops the names-bound bit when they differ. The counters (`memberReqAIStatsSnapshot`:
  checked, passed, breached, cached, capped, shed, failed, blocked) are in memory, belong to the process that answers and start at
  zero on every restart; there is no `/stats` section for them.
  The full order of `memberReqAIRule` (plan 15-05; D-02, D-21): after the checks above one pipelined round reads the verdict,
  the baseline `nm` and the marker `nu` (`memberReqAIReadState`; any error other than a miss is unknown), then
  `memberReqAIDecide` (security SEC-01 moved the baseline step first): (1) not joining (and not `IgnoreBaseline`), no marker and
  a baseline equal to the current names hash is a pass with no write, answered before the verdict cache is looked at, so
  another account's cached breach of an identical canonical string (the cache is global to the fleet) can never reach a
  member who never renamed, in the job exactly as in the watcher (`TestMemberReqAIEstablishedMemberIgnoresCachedBreach`);
  (2) a readable verdict decides next, below the bar records a pass and passes, at or above it sets the marker and breaches,
  answered before any provider call, and it applies to a joiner, a marker holder, a renamer, a member with no baseline and
  every `IgnoreBaseline` caller; (3) not joining, no marker and no baseline records `nm` with `SET NX` and passes with no
  call; (4) otherwise (a joiner, a marker, a baseline that differs) the flight judges and the caller records the answer (a
  pass writes `nm = h` and deletes `nu`, a breach sets `nu`, an unknown writes nothing). The flight is shared by callers with
  different user IDs and the same string, so it does only the call and the verdict write. `memberReqLookupMode.Joining` is
  set only for the join entry's executor run (`Source == memberReqFromJoin`): a joiner is judged and the baseline is ignored,
  because a rejoiner is a new membership. `memberReqLookupMode.IgnoreBaseline` (Phase 15 review WR-01) skips the baseline
  step the same way without saying the member is joining: the held release (`memberReqHeldLookupsClear`) and "I fixed it"
  (`memberReqFixedJudge`) set it, so a hold that carries bit 128, the durable record that the names were judged spam, is
  never released and a mute never freed by a baseline-only pass when Redis has lost `nv`, `nm` and `nu` (a flush, an
  eviction); only the verdict cache or one real call may pass such a member (cost: one call per held member after a Redis
  loss, bounded by the budgets). The baseline-only pass stays for the watcher's and the job's first sight of an established
  member who has no hold. "New" means "recorded as joining while the rule is live", written by the join entry
  for every joiner it reaches and by nothing else. The accepted limit: with no Redis, a failed marker write or a join the
  entry never saw, a real new member is baselined at their first message and unjudged until they rename (the safe
  direction); a joiner who never reaches the entry (a captcha failure, a lockdown ban) gets no marker. The baseline lives
  180 days and is never refreshed, so a member dormant for over 180 days is first-sighted again and baselined with no call
  (accepted, the same D-02 limit). A member who renames
  and never writes costs nothing: no update arrives. D-03's "a held member's message makes no lookup" is amended for holds
  that carry bit 128: the held release asks the rule with the sender's current names (`memberReqHeldLookupsClear` passes
  `Names`), bounded by the same verdict cache, breaker, gate and budgets, and only for a member the exemptions (approved,
  pending captcha) and the watcher's known-administrator skip did not stop; every other held member's message still makes
  no lookup (a hold with a name bit or the photo bit makes no AI request). An administrator whose list entry is unknown can
  cost one call and a released hold, never a punishment (the executor's live eligibility check comes after the hold read,
  exactly as for the photo and channel rules).
- Member requirements, the executor (`memberreq_enforce.go`, `memberreq_notice.go`, `memberreq_unmute.go`): `memberReqEnforce` is the one
  place that punishes a member for a broken rule; its production callers are the message watcher's job and the join entry
  `memberReqJoinThenWelcome` (below), and only while the enforcement switch is on (it is, by default, since the `mreq|` handler exists). Its order is fixed and every step that cannot be answered returns `memberReqNone` with no hold: the bot
  itself, bots and `staffServiceUserIDs`; approvals through `approvals.IsUserApprovedContext` (yes, no or unknown: a
  failed load is an error, never "not approved") and the pending captcha through the two context reads (same three
  states); the hold read fresh (one that is not a stale pending one is first given the held release, below, and otherwise
  is `memberReqHeld` and adds no warn, notice or punishment, and with Delete messages on its message is deleted, below); a paused chat (the Data bullet of the cap) ends
  there with nothing punished or posted; one live
  `getChatMember` through `memberReqPaced`, which is the admin check (creator, administrator, left, kicked, a restricted
  non-member and bots are never punished) and the source of the names the rules judge; then the group's settings are
  read again (`memberReqSettingsLookup`, the cached key that is in `skipLocal`) and the broken rules, the strictest of them
  whose punishment is not Off and the Delete messages switch all come from that fresh row, not from the snapshot the
  watcher or the join entry handed in (WR-02, D-20: a rule switched Off, changed or a Delete messages turned off while the
  job waited behind the pacer is honoured, and an unreadable setting punishes nobody; the snapshot is only the cheap
  pre-filter, and the held and paused paths before the live lookup still use it for their deletes); then the lookups (`memberReqLookupBroken`,
  photo, then channel, then the AI rule, each only while its action is not Off in that fresh row, under `memberReqLookupJobBudget` (20 s) for a job or
  `memberReqLookupJoinBudget` (4 s) for the join entry, both variables), whose mask is OR-ed into the name mask before the strictest
  punishment is chosen: a `memberReqBreach` adds its bit and an unknown or inert verdict adds none, so an unanswerable lookup (an
  error, a 429 past the retry, a timeout, a pacer shed, the root cancelled by `StopMemberReqJobs`, a null body, an unrecognised
  status) punishes nobody, leaves no hold, notice or write, and never blocks a name breach already established when the
  rule it could not answer is equal or weaker: the member is punished for the names, and the hold and the notice name only the
  rules that were established (a partial episode: the hold's mask lacks the unknown rule, which is judged again once the hold
  clears; research S-02, decided here so that Phase 13's name rules never depend on the lookup pacer's health). Phase 14
  review WR-02 closes the one case where that partial episode did harm: when a rule that could not be answered
  (`memberReqLookupMasks`' unknown mask) is switched on with a punishment stricter by `memberReqSeverity` than the strictest
  punishment of the rules found broken (`memberReqUnknownDefers`), the executor makes no claim and returns `memberReqNone` with
  one Info line (chat and user IDs only): a Warn hold for a blank last name would otherwise stand while a Ban channel rule
  stayed unjudged, because a held member's messages make no lookup. The member is judged again at their next message, so a
  stricter rule is never silently dropped behind a weaker punishment and its hold, and an unknown with no established breach
  is simply not broken as before. An unknown that is equal or weaker than the punishment chosen defers nothing, and so does an
  unknown that is a sticky state of the AI rule (a paused breaker, a used-up budget, a full gate, D-22, or a Redis outage or
  counter fault, WR-03): `memberReqLiveViolation`
  removes the run's `touch.sticky` bits from the unknown mask before `memberReqUnknownDefers`, so an outage, an exhausted hour or
  an attacker spending the fleet's budget never switches off the weaker name punishments of every group, while every other
  unknown (a timeout, a 5xx, a 429, a refusal, an unreadable answer) still defers as before. The cost is
  that a stricter lookup rule that stays unanswerable for a member (a persistent 5xx or timeout on the channel; a bot that is
  no administrator is inert through the `cna` fact and defers nothing after the first message) holds the weaker name
  punishment back for as long as it stays unanswerable, which is the safe direction: nobody is punished on a guess. The lookups sit after the exemptions, the hold read, the pause and the live
  `getChatMember` (a held, exempt or paused member makes none) and before the mute-over-restriction check, the cap and the claim,
  so a lookup cut off by a crash or a shutdown leaves no hold and no cap count. Phase 14 review WR-03: the live record was
  read before a lookup phase that can take seconds under a loaded pacer (up to 20 s for a job, 4 s for the join entry), so
  when something is broken and the phase went beyond the cache (some flight was started or joined, carried through
  `memberReqLookupTouch` in the lookup context) or took longer than `memberReqLookupStaleAfter` (1 s), the record is read once
  more through `memberReqLiveMember` and the eligibility check (a member promoted to administrator or gone meanwhile is never
  punished), the names, the mute-over-restriction check and the marker adoption all
  rest on that record; a re-read that fails punishes nobody. D-29: the AI rule's bit is dropped in that branch when the
  re-read record's canonical names hash differs from the one the lookups judged (`NamesBound`,
  `memberReqNamesBoundMask`), so a member who renamed during the provider call is never punished for the old string and is judged
  again at their next message (the old string's verdict stays cached). SEC-02: in that same branch (`memberReqRereadAfterLookups`) the
  exemptions are judged again first, through `memberReqExempt`: a member an admin approved, or whose captcha attempt started,
  during the lookup phase, and an approvals or captcha read that fails then (an unknown is exempt) are punished by nobody.
  A phase of cache hits that was fast makes no extra request and no extra approvals read.
  Then, when that strictest punishment is a Mute and the live record is `restricted` with no
  marker of this episode (the stale hold's marker that `memberReqLiveMarker` adopts is the one exception), the one
  paced `getChat` of `memberReqUnmutePermissions` reads the group's defaults and `memberReqRestrictedBelow` asks whether
  the member lacks a right the group grants: if so the restriction is someone else's (an admin's `/mute`, a partial
  media restriction, a staff mute) and `memberReqMuteOverRestriction` refuses the mute (CR-01), because
  `restrictChatMember` replaces the whole restriction and the later lift ("I fixed it", the rejoin reset, the undelivered
  notice lift) would free the member of it. The refusal returns `memberReqNone` with no hold, no notice, no delete and no
  further Telegram call and logs one Info line with the chat and user IDs; a `getChat` that fails refuses too (nothing is
  replaced on a guess). A member restricted only to the group's own defaults (what the captcha, `/unmute` and the
  requirement lifts leave in a group whose defaults are restrictive) lacks nothing and is muted as before. Only Mute is
  refused: Warn leaves the restriction intact, Kick and Ban remove the member, so all three still act on a restricted
  member, and a partially restricted member who breaks a Mute rule is not muted (the owner uses Warn, Kick or Ban for
  such groups); the chat's per-minute cap, counted before the claim; the claim of the hold before any punishment (`memberreq.ClaimHold`, or
  `ReclaimStaleHold` for a pending hold older than two minutes); then the punishment and one notice. Never use
  `IsUserAdmin`, `IsUserApproved` or `aispamCarvedOut` there. A warn goes through `applyWarn` with the hold marked
  punished in its `afterRecord` hook, inside the warn lock right after the warn is stored, so a recorded warn and a
  punished hold are not separated in the normal case, and the warn module's own reply and Remove warn button are never sent.
  Marking a hold punished (`memberReqMarkPunished`) is tried 3 times (`memberReqMarkAttempts`, `memberReqMarkRetryWait`
  apart, one detached 5 s record context, the write behind the `memberReqMarkHold` seam). A warn whose mark still fails
  must never be punished twice (WR-01): `afterRecord` first writes `fuku:memberreq:warned:<chat>:<user>` (value the hold's ID,
  `StaleClaim` plus 10 minutes, best effort and Redis only: no Redis or a Redis error writes and finds nothing and the
  reclaim then warns again, accepted) and drops it once the mark landed; the reclaim of a stale pending hold that was a Warn
  with a Warn action now (`memberReqAdoptRecordedWarn`, after the claim was won) finds the note naming that hold, marks the
  hold punished (a failure leaves it pending and the next reclaim adopts it again), warns and announces nothing and returns
  `memberReqHeld`. Kick and ban recover through the leave update and a mute through its live marker, so only the warn
  needs the note. An `applyWarn` error with `Recorded` true (the limit action ran and the reset failed) is not a failed warn:
  the hold is marked, `LimitApplied` is set from `LimitReached` and no `LimitErr`, the notice is posted and the outcome is
  `memberReqWarned` or `memberReqRemoved`; an error before the warn was stored is the backoff below.
  Kick is `kickMemberContext`, ban a `banChatMember` with no end date, mute a `restrictChatMember` with
  `MutedPermissions` and the marker `until_date` (now plus 330 days, `lockdownBanSpan`) stored on the hold before the
  call. The requirement mute is owned by its marker alone (`isMemberReqMute`: restricted, still a member, `until_date`
  within 2 s of the hold's `mute_until`); no other mute path is changed. A punishment Telegram definitely refuses (a 4xx
  other than 429 by code, or the pacer's `ratelimit.ErrRateLimited`: `memberReqRefused`) posts nothing and backs the
  pending hold off (`memberreq.BackoffHold` refreshes `created_at`), so the member's next messages inside
  `memberreq.StaleClaim` are the held path with no Telegram call, and after it one winner reclaims the hold and retries;
  a warn that fails is the same backoff. An unknown answer (a 5xx, a timeout, a dropped connection) of a mute is checked
  with one live `getChatMember` against the stored marker (`memberReqSendFailed`): the marker present counts the mute as
  applied, absent backs off, an unreadable record leaves the hold pending, and a kick or ban with an unknown answer
  leaves it pending with no notice. A reclaimed pending mute hold whose old marker is still live on the member adopts it
  (`memberReqLiveMarker`) instead of muting twice. One notice per episode is a new message in the group's language,
  sent after the punishment took effect, with the member's mention (`memberReqMention`, the one helper of the notices and
  of the "I fixed it" edit, which strips the bidirectional controls U+061C, U+200E, U+200F, U+202A to U+202E and U+2066 to
  U+2069 from the name before it is cut to 64 runes and wrapped in its own isolate, so a name cannot close the isolate and
  reorder the line; the rules still judge the original name) spliced in after translation and
  every broken rule whose punishment is not Off in rule order (the three lookup phrases after the five name phrases); the plain
  warn, kick and mute notices (`memberReqNoticeRemedies`, never the warn-limit notices, the ban notice or an answer) end
  with one trailing sentence per remedy-bearing broken rule, each after one space, in rule order, appended after translation
  and before the name is spliced: the `RemedyKey` of the rule's table row (`mreq_remedy_photo` for the photo rule, then
  `mreq_remedy_ai` for the AI rule), so a rule with a remedy is one table field and no code names a rule's code or bit for it;
  a notice, the remedies included, is at most 520 code points of visible text in every language
  (`TestMemberReqScreensFitEveryLocale`, `TestMemberReqNoticeAIRemedy`). The cap is proved by that rendered check over the real
  translations and not by the key ceilings (at every ceiling the sum is 636; the worst real case is French mute at 506), and
  `mreq_warn_reason` with all eight phrases is at most 300; a translation that breaks the cap is shortened, never the cap
  raised; only a
  mute carries the `mreq|v1` "I fixed it" button (`a=f`, `h`, `u`). A notice is queued for deletion by `memberReqPostNotice`
  (as `bot:punish_auto`, a mute notice at the 24 hour cap) and the "I fixed it" edit reschedules it from the tap (see
  "Scheduling the bot's own messages" under Subsystem traps). A mute notice is never dropped by pacing: `memberReqPostNotice` waits
  `memberReqNoticeRateLimitWait` and sends again whenever the pacer gives up on it, until its context or
  `memberReqMuteNoticeBudget` ends, and every notice whose send failed otherwise is retried once (a notice without a
  button is not retried after `ErrRateLimited`). A mute whose notice still cannot be delivered is never left in place
  without its button: `memberReqUndeliveredMute` first makes one paced live `getChatMember` (`memberReqLiveMember`) and
  lifts only while `isMemberReqMute` still holds for the hold's marker, because a notice that waits out a rate limit can
  take up to 5 minutes and an admin may have restricted, unmuted or banned the member, or the member left, meanwhile:
  when the live record does not carry the marker nothing is lifted and no `restrictChatMember` is sent, the hold still
  goes back to pending and one Warn (IDs only) says so, and when the read itself fails the lift goes ahead anyway (a
  failed read on top of a failed notice is a double fault, and a member left under a 330 day mute with no button is the
  worse outcome, so this fails toward lifting). The lift goes through `memberReqUnmutePermissions` (one paced `getChat`, then
  `resolveUnmutePermissionsWithLockdown`, so an active lockdown's stored permissions apply) and `memberReqLiftMute` (one
  paced `restrictChatMember`, no end date), on a context that survives the run's cancellation, and turns the hold back
  into a pending backoff row with `memberreq.RevertHoldToPending` (`punished_at` NULL, `created_at` now), so the
  member's first message after `StaleClaim` starts the episode again with a fresh notice; it logs one Warn with the chat
  and user IDs only, and a failed revert is logged at Error. Accepted limit: when Telegram also refuses the lift, the
  mute stays and the hold stays punished with one Warn saying the mute could not be lifted; it takes two refusals in a
  row, and the rejoin reset or an admin's `/unmute` followed by a clean message still ends it. Every requirement unmute
  takes its permissions only from `resolveUnmutePermissionsWithLockdown`. Delete messages (`memberReqDeleteMessage`, one
  paced `deleteMessage`, only for a message case with a message ID, never for a join or a clean member) deletes the
  violating message after the punishment whatever became of it (it took effect, was refused or unknown, or another
  replica won the claim), every further message of a held member, and during a pause the message of a violator the
  cached admin list knows as a non-admin. It is not a punishment and posts no notice; a failed delete is only logged
  (chat, user and message IDs and the error code, never the content) and never changes a punishment, hold or notice. A
  message is deleted only when its sender is established as a non-admin (MREQ-09), by `memberReqSenderNonAdmin`: on the
  punishing path the job's own live `getChatMember`; on the held and paused paths the cached admin list read at the
  moment of the decision (`cachedAdminStatus`: known, and the sender not in it). When the list cannot say, the held
  path outside a pause makes one live `getChatMember` (a creator, an administrator, someone gone and a failed lookup mean
  no delete), and during a pause nothing is deleted and no Telegram call is made, so a pause never spends a call to decide
  a delete. A held member promoted to administrator is never deleted: the promotion's `chat_member` update reaches
  `adminCacheAutoUpdate` (group 0), which invalidates and reloads the admin list, so the member reads as an administrator
  or as unknown, never as a known non-admin (a message handled at the very moment of the promotion, before that update's
  invalidation, is the one window the watcher's own known-admin skip has too). Accepted limit: an unknown-status
  violator's message is not deleted during a pause (the cached list cannot say and a pause forbids the live lookup). Logs
  carry chat and user IDs and outcomes only, never a name, a rule text or a notice text.
- Member requirements, the join (`memberreq_join.go`): `memberReqJoinThenWelcome` is the one entry of both welcome paths. The
  Welcome dependency of `membershipDepsForJoin` (the `chat_member` join and the join service message, after
  `claimRecentJoinProcessing`) and the last step of `finishCaptchaPass` (the button pass and the web pass, so the captcha
  always runs first and the paths cannot drift) both call it, and `captcha.go` no longer calls `SendWelcomeMessage`
  itself. It reads the switch through `memberReqLive()` first (never the variable: it runs on captcha lifecycle goroutines)
  and, while it is false, sends exactly the plain welcome of before, as it does for a chat that is not a supergroup, a bot,
  the bot itself and a Telegram service account (`memberReqNeverChecked`), a settings read that failed and a group with
  every rule off. With a rule on it reads the member's hold fresh through `memberReqHoldRead` (the one hold read of the
  entry and the guard, a test seam): a punished requirement mute, which is the D-08 race of a member who posted before
  their captcha attempt existed, records `welcome_held` through `memberreq.SetHoldWelcomeHeld` and sends no welcome,
  whatever Check on join says. Next, while the AI rule is live (`memberReqAILive`), the entry sets the member's new-member
  marker `fuku:memberreq:nu:<chat>:<user>` (`memberReqAIMarkJoin`: one `SET` under `raidRedisTimeout`, no-op without Redis, an
  error only logged at debug by IDs) before its Check-on-join branch, so the marker is the join record whether or not Check
  on join is on: a joiner whose call was unanswered keeps it and is judged at their first message, and a member who joined
  with Check on join off is judged at that first message with no call at join (a member whose hold read failed returned to
  the plain welcome before it and gets no marker, the safe direction). With Check on join off the member is welcomed with no
  further read and no `getChatMember`. Otherwise `memberReqEnforce` runs synchronously (`memberReqJoinTimeout`, 15 s, `memberReqFromJoin`) with
  its one live `getChatMember` and, for a rule of the photo, channel or AI kind, the same lookups through the same executor under the 4 s
  `memberReqLookupJoinBudget` (a joiner whose lookup does not finish in 4 s is welcomed, leaves no hold, and is caught at their first
  message, so a raid of joins never parks a dispatcher goroutine for the entry's 15 s; `memberReqJoinThenWelcome` itself is unchanged,
  because `memberreq.AnyRuleOn` reads the eight-rule table, so a group with only the photo rule on is screened at join): a kick or ban sends no welcome, a mute sends none and has stored `welcome_held` on its hold
  at the claim, a warn posts its notice and the usual welcome follows (a warn whose limit action kicked or banned is a
  removal and gets none), and a clean member, an exempt one (approved, a live administrator, a bot), a check that could
  not be answered, and a joiner whose Mute is refused because an administrator's restriction is on them (CR-01: a rejoiner
  restricted for good, say; `memberReqNone`, no hold) are welcomed. The settings the entry read are only the pre-filter:
  `memberReqEnforce` reads them again before its claim (WR-02), so a rule that went Off in between punishes nobody. A captcha-failed joiner and a joiner the lockdown guard records for a ban never reach the
  entry. The captcha guard (`memberReqCaptchaGuard`) is the first statement of `unmuteCaptchaUser`, the one choke point of
  every captcha unmute (the pass, `restoreCaptchaPermissions`, restart recovery and the `captcha_muted_users` retry
  worker). While `memberReqLive()` is false it reads nothing and changes no captcha unmute: no hold read, no Telegram call,
  no error (`TestMemberReqCaptchaGuardOffUntilLive`, with a counting always-failing `memberReqHoldRead`). The switch is
  true in production, so the guard reads a hold on every captcha unmute. Once the switch is on it reads the member's hold, and a punished requirement mute makes it re-apply `MutedPermissions` with `until_date`
  the hold's own marker through `memberReqPaced` instead of unmuting, so a captcha pass never frees a held member and the
  marker stays valid for "I fixed it" (D-08). A hold-read error fails closed: the guard returns an error wrapped with the
  chat and user IDs only (`errMemberReqHoldUnreadable`, none of the database's text), `isPermanentUnmuteError` answers
  false for it, and the member stays muted while the caller's own retry applies (a `captcha_muted_users` row that the
  5-minute worker tries again, a kept row, or the logged error of restart recovery). A permanent answer would have made
  the worker delete its row and leave the member muted with no retry. Accepted cost: a passing database fault delays a
  captcha unmute until the next retry that can read, rather than freeing a member the requirements muted. No other mute
  path is edited. `MembershipDeps.Reset` (nil-safe, so every `MembershipDeps` literal without it keeps working) runs in
  `ProcessSingleJoin` after the delivery that won the dedupe claim and before the join is decided, so the join service
  message and the `chat_member` update of one join reset once. `membershipDepsForJoin` and `processSingleNewMember` carry
  the join's own date (`newMember`: `ctx.ChatMember.Date`; `cleanService`: `msg.Date`; `memberReqJoinDate` reads a missing
  date as now) and fill `Reset` with `memberReqJoinReset`. A requirement-muted member's leave and rejoin are invisible to
  the `chat_member` handlers (`ExtractJoinLeftStatusChange` reads restricted to restricted as neither a leave nor a join),
  so the rejoin's service message is what ends that episode: the reset deletes the member's holds created before the join
  date (`memberreq.TakeHoldsBefore`, so a hold the watcher created for this very membership survives) and, for a mute hold
  (punished or still pending, because a restrict whose mark failed or never ran leaves a pending hold with a live marker),
  reads the live record and lifts the mute only while it still carries the hold's marker (`isMemberReqMute`; an
  admin's, staff's or captcha's mute is never lifted), only with permissions from
  `resolveUnmutePermissionsWithLockdown` (`memberReqUnmutePermissions`, `memberReqLiftMute`), and puts the hold back with its
  ID and notice ID (`memberreq.RestoreHold`) when any step of the lift fails, so the next join retries; the member is then
  checked again on join. A crash between the delete and the lift can strand a mute with no hold (accepted: an admin's
  `/unmute` ends it). `leftMember` calls `memberreq.ClearHolds` right after `clearRecentJoinProcessing`, so a kicked member
  who rejoins is checked again; a failure is logged by IDs and never stops the goodbye. Neither the reset nor the leave
  hook is behind `memberReqLive()`: they only delete holds or lift a requirement mute, and while the switch is off no hold
  exists; each costs one statement per join or leave. Nothing of the requirements ever unbans, so a banned member stays
  banned. Known limit: the goodbye after a requirement kick or ban is kept (parity with `/kick`, the captcha kick and the
  warn limit), so with goodbye on every such removal posts one.
- Member requirements, the held release (`memberreq_held.go`, D-21): `memberReqHeldReleased` is the one re-check a held
  member's message gets before the Phase 13 held path (`memberReqDeleteHeld`). A hold ends by a leave, a rejoin, the clean
  predicate and "I fixed it", and the clean predicate runs only in the watcher's clean path, which a group with a lookup
  rule on reaches only for a member whose lookups are cached positive; a held member's messages never ask Telegram, so a
  Warn hold with a photo or channel bit would otherwise never end after the member fixed it, and the watcher would hand
  the runner a job for every message of theirs. The release runs in the job, after the exemption, the hold read and
  before the pause check, and returns true (the executor then returns an empty result: no Held outcome, no delete) only
  when all of these hold: the case is a message with a message ID and a date (a join is never released); the settings read
  again through `memberReqSettingsLookup` without an error (an unreadable one keeps the hold); the sender's names break no
  rule that is on (a member who still breaks a name rule stays held and no lookup is spent on them); every lookup rule the
  hold carries a bit of and that is still on answers pass or inert when run with `memberReqLookupMode{Classify: true, Names: ...}`
  (the sender's current names, which only the AI rule reads, with `IgnoreBaseline` set so no baseline-only pass can release a
  hold carrying bit 128; a hold carrying bit 128 is released after the member renames to
  an ordinary name: the rule judges the new names, a pass writes `nm = h` and deletes the marker, and the predicate then
  deletes the hold) under `memberReqLookupJobBudget` (an unknown, a breach, a cut-off lookup keep the hold; a rule now Off counts as
  released, and only the hold's own lookup bits are asked); and `memberreq.ClearHoldOnClean` with its two second margin
  deleted the row. A hold with name bits only needs no lookup, so a member who fixed their name is released by the same
  call, which keeps the Phase 13 behaviour (a clean message ends a warn hold) true in a group that also has a lookup rule
  on, where the watcher's clean path is no longer reached by a member whose lookups are not cached. The release only
  deletes the hold row: it never punishes, claims, warns, posts or restricts, and it never lifts a mute (for a punished mute
  the predicate deletes the row only when the mute was confirmed more than two seconds before the message, and a member
  the bot muted cannot send, so that message shows an administrator lifted it; otherwise a mute is freed only by "I fixed
  it" or an administrator). A passing lookup is cached by the lookup itself and a missing photo or subscription never is, so
  a held member who still lacks the photo or the subscription costs one paced lookup per message, bounded by the lookup
  pacer, the flights and the runner: accepted (D-24).
- The member requirements runner (`memberReqRunAsync`, `memberreq_watch.go`): a violating message is handed to a job on its
  own goroutine, counted in `memberReqJobs` (a `raidPostTracker`) before `memberReqDraining` is read, capped at
  `memberReqMaxInFlight` (32) by `startBelow` (check and increment in one step), recovering from panics. A full runner sheds
  the job with one Debug-level line (not Warn: a busy group can shed many a second) carrying the chat and user IDs only, and the member's next message is checked again: an
  accepted, logged limit, so under overload a violator's message can stay undeleted until their next message. A lookup
  miss (a member with clean names and no cached lookup positive, after the D-25 skip) is handed to the runner exactly like
  a violator, so it is capped at the same 32, counted in `memberReqJobs`, refused while `memberReqDraining` is set and shed
  with the same one line when the runner is full; 100 messages of one member make one lookup request, because the jobs
  share one flight per member, the flight reads the cache once more before it asks Telegram and writes the positive before
  it returns. With Redis down every message of a member in a group with a lookup rule on is a job (nothing is cached and
  the watcher cannot say the member is clean), by design (D-12): it costs at most 32 jobs, the lookup pacer's one-replica
  fallback interval and shedding, and never a punishment, because a lookup that cannot be answered is never a breach.
  The same cost applies, for the same reason, to a group whose linked channel the bot cannot read only until the bot's
  standing is established as not an administrator: the channel-level `cna` fact (60 s) then makes the channel rule inert for
  the watcher and the executor with no job and no Telegram call, so the cost is bounded by that cache after the first message.
  A standing that cannot be established (a 429, a 5xx, a timeout, a shed) writes nothing and the per-message job stays until
  it can, bounded by the 32 jobs and the lookup pacer.
  Switching the AI spam-name rule on in a large group costs one baseline-only job per distinct poster once and zero provider
  calls: the first message of an established member is a job that records `nm` with `SET NX` and passes with no Claude call
  (`TestMemberReqWatcherAIBurst`), and their next message is satisfied in the watcher and costs no job.
  A basic group is never screened (mute and kick are supergroup methods): the watcher's filter is supergroups only, and the
  page says so with its basic-group line (`settings_mreq_here_basic`, one line while any rule is not Off), so it is a stated
  limit and no longer a silent one.
- Member deletion, edited messages (`member_deletion.go`, Phase 17, DEL-04): with `member:edited` stored as a grace of 30, 60,
  300, 900 or 3600 seconds, a member's message whose `edit_date` is more than the grace after its own `date` is deleted at once
  (the rule judges `edit_date - date`, so a gap equal to the grace is kept, a missing or non-later `edit_date` keeps the
  message, and a message edited twice is judged by its last `edit_date` each time). A message older than 47 hours by its own
  date is skipped with no call and no notice (`deletionDropAge`, Telegram can no longer delete it). The exemptions are judged at
  the edit, in the order of the `MemberDeletion` bullet under Handlers: a creator or administrator, an approved user, a
  `sender_chat` sender, a bot, a `via_bot` message, an automatic forward, a Telegram service ID, the bot itself, a pending
  captcha, a private chat and a Staff Group are kept, and so is every message whose exemption could not be read. Stated limits:
  no copy of the original message is stored, so any edit after the grace counts, a link preview or a caption touch included
  (D-04, a deferred idea; a live check is in `17-SPIKE.md`); a cold admin cache costs one bounded live `getChatMember` per
  edit until it warms (a burst beyond the semaphore keeps the messages); a group where the bot cannot delete other people's
  messages costs one refused call and one live check, then the latch of the deletion core takes over (D-24).
- Scheduled deletion core (`deletion_schedule.go`): `deletionAct` is the one place that decides what happens to a message with
  a delay: a chat ID that is zero or positive (not a group's) returns first, before any read, write or Telegram call, on the
  inline branch as well as the queue branch (SEC-05, symmetric with `deletion.Schedule`'s own refusal); a latched chat returns, a chat that is a Staff Group or cannot be told apart from one (`deletionStaffLookup`, the
  fresh read with `staff.GetStaffGroupFresh` as its default, an error counts as a Staff Group) returns, delay 0 is an inline
  delete and any other delay a queue row at `deletionSentAt(msg.Date)` plus the delay (the message's own date, or now when
  the date is zero, negative or in the future). The Staff Group is refused in three places: the watcher and `cleanService`
  through `deletionAct`, and `deletion.Schedule` itself. `deletionSchedule` is the only caller of the queue writer (through
  the `deletionQueueWrite` seam): it refuses a delay outside 0..86400, writes nothing for a latched chat and wakes the
  worker for a row due now. The inline delete (`deletionDeleteNow`) runs on a goroutine that starts with
  `defer error_handling.RecoverFromPanic`, derives its context from `deletionContext()` (cancelled by the worker's stop) and
  is counted in `deletionInline` with a cap of `deletionInlineMax` (64): a call beyond the cap, a delete the pacer shed
  (`ratelimit.ErrRateLimited`) and a delete whose own `deletionInlineTimeout` (15 s, a variable only for tests) ran out inside the
  pacer's wait while the deletion root is still live (a flood-control block longer than that, under the pacer's `MaxWait`;
  `deletionInlineShed`) become a queue row due now plus a wake instead of parking goroutines or losing the message; a delete
  cut off by a cancelled root (shutdown) writes nothing and is lost; a `gone` answer sets the
  `gone` latch, a refused one is dropped with no log line (a refused `service:*` or `member:*` one first decides the service latch through
  the live right check of `deletionDecideServiceLatch`, in the same goroutine and on a context derived from the deletion root,
  at most once per chat per 10 minutes behind the `fuku:delete:chk:<chat>` guard (SEC-02); a bot kind never asks, and only a
  live `cannot` answer writes the latch) and any other failure loses that one message. `deletionClassify` is a
  table of predicates (retryFree, gone, refused, else retry) that matches on lower-cased fragments and codes, never one string.
  Logs carry the chat ID only, never a message ID, text or member.
- Deletion worker (`deletion_worker.go`, `deletion_right.go`): `StartDeletionWorker` runs a pass at start, then one per
  `deletionWorkerTick` (1 s) or wake, a pass being cycles back to back until one settles nothing; `runDeletionCycle` starts with
  `defer error_handling.RecoverFromPanic`. The cycle order is fixed (research S-05): release stale claims, claim up to 1000
  rows with one conditional update read by token (`deletionBatch`; about 0.1 ms with 520,000 pending rows, far under the 90 s cycle guard and the 2 minute stale claim), drop the rows whose message is over 47 hours old, then per chat in
  ascending chat ID order: the latch (a `gone` value drops every kind of the chat, a `service` value drops its `service:*` and
  `member:*` rows (`deletion.NeedsDeleteRight`) and lets `bot:*` through), a fresh Staff Group read through `deletionStaffLookup` (a row drops the chat's rows with no
  call, an error releases them free), the self-destruct rule screen, then one paced `deleteMessages` call per chunk of at most 100 IDs. A success
  completes the chunk. A failure is read by `deletionClassify`: retry-free (a 429 past the pacer's retries, a shed pacer, a
  cancelled context) releases every unsettled row free and ends the cycle, so the next cycle's first call fails at once
  under the shared block with no request; gone sets the `gone` latch and drops the chat's claimed and pending rows;
  anything else falls back to one `deleteMessage` per ID, each classified alone: done and refused (not found, can't be
  deleted, any other 4xx) drop the row quietly, a 5xx or timeout releases it counted (an attempt and a 60 s backoff on `delete_at`). Every row write (`Complete`, `Release`,
  `DropChatPending`, the latch) runs on `context.WithoutCancel` with its own 5 s bound through `deletionCycle.writeContext`,
  never the run context a shutdown cancels. The cycle logs one debug line per chat batch with counts and the chat ID, and
  never a message ID, a claim token or text (`TestDeletionWorkerLogsNoMessageIDs`); a dropped row has no line.
  Throughput (D-32): a chat costs one paced call per chunk of 100 per cycle, so a cycle makes at most the sum over its chats of
  the rows of the chat divided by 100, rounded up, calls (40 chats of 25 due rows are one cycle of 40 calls,
  `TestDeletionWorkerManyChats`); with N chats due together that drains about 10,000 divided by N rows a second per chat,
  capped at 1,000 a second by the pacer's 100 ms interval. The self-destruct rule screen (`screenSelfDestruct`, D-26b) runs after
  the Staff Group read and before the first chunk: one cached `deletionRulesLookup` per chat per cycle on the cycle context
  judges the chat's claimed `member:selfdestruct` rows, which are completed with no call when the rule is absent or holds a
  value `deletion.ValidateRuleDelay` refuses (nothing is deleted after the owner switched it off except a call already in
  flight) and put back free, with no attempt counted, when the rules cannot be read (unknown never deletes); the rows of every
  other kind pass untouched (`TestDeletionWorkerScreensSelfDestructOff`).
  The service latch is set only after a live check: when a `service:*` or `member:*` message is refused with a message-level answer, once
  per chat per cycle `deletionDecideServiceLatch` reads the bot's own member record through the pacer
  (`deletionReadBotRight`, classified by `deletionClassifyBotRight` from status and error code only, never a description:
  creator can; an administrator can when it has the delete right or the flag is not trusted; member, restricted, left, kicked,
  a 400 and a 403 cannot; a 429, a 5xx, a transport error, a timeout, a shed pacer and a null or unrecognised answer are
  unknown). Only `cannot` latches the chat for the kinds that need the right (`service:*` and `member:*`) and drops its pending
  rows of both prefixes (`deletionServicePrefix`, `deletionMemberPrefix`, one `DropChatPending` each); `can` and unknown only
  drop the one refused row. The worker's claimed-row drop after a `cannot` covers the same kinds (`maybeLatchService`). The worker has no chat type, so it decides whether to trust the administrator flag by the chat ID
  alone (`deletionTrustAdminFlag`, SEC-02): for a supergroup or channel ID (at or below -1000000000000, the Bot API ranges)
  the flag is reliable, so an administrator without `can_delete_messages` reads as cannot and the chat is latched for service
  kinds; for a basic group's ID (-1 to -999999999999) and for any other ID the flag is not trusted and an administrator is
  never latched, which avoids a false latch in a basic group whose administrator reads `can_delete_messages` false, at the
  cost of one refused call per notice there (bounded by the pacer and by the inline decision's once per 10 minutes). A missing
  right never stops `bot:*` rows, because a plain-member bot can delete its own messages. The settings page reads the right
  through the same `deletionReadBotRight`, trusting the flag outside basic groups. After the stop cancels the root, a
  delete-now starts no goroutine and takes the durable path (a row due now), so it resumes after a restart.
- Member deletion, the self-destruct batcher (`member_deletion_batcher.go`, Phase 17): the self-destruct watcher only offers
  entries (`memberDeleteOffer`: one non-blocking send into a buffered channel of 4096, `memberDeleteQueueSize`; a full channel, a
  draining process and an entry of another kind are refused, and a refusal keeps the message) and never touches the database,
  `deletionAct` or `deletionSchedule`. The batcher goroutine, started and stopped with the worker (`memberDeleteStart`
  in `StartDeletionWorker`, `defer error_handling.RecoverFromPanic`, each flush under its own recovery), flushes every 300 ms
  (`memberDeleteFlushEvery`) or at once when 100 entries are buffered (`memberDeleteFlushAt`, a non-blocking kick). The flush
  order is fixed: take what is buffered (at most one buffer's worth), merge entries of one `(chat, message)` to the earliest
  time, then per chat in ascending chat ID one `deletionRulesLookup` (an unreadable rule sheds the chat's entries; a
  `member:selfdestruct` rule that is absent or not accepted by `deletion.ValidateRuleDelay` drops them as correct, not a shed,
  D-26a), one exact `deletion.CountPending` bounded by the ceiling (`memberSelfDestructCeiling`, 20,000, through the
  `memberDeleteCountPending` seam: a count that fails sheds the chat's entries, because unknown never deletes, D-33), the entries
  past the ceiling shed latest first, and then one `memberDeleteBatchWrite` (`deletion.ScheduleBatch`: statements of at most 100
  rows, the Staff Group refused inside it, so the flush makes no second read) over every kept entry of every chat. A write that
  returns an error sheds what it did not write and is logged once an hour by chat count. Each flush runs on
  `context.WithTimeout(context.WithoutCancel(deletionContext()), deletionRecordTimeout)`, never the cancelled run context, and
  wakes the worker only when a written entry is already due. Shed reasons are counted separately in atomic counters (full,
  stopping, ceiling, rules unreadable, count unreadable, write failed) and a shed logs at most one Warn per chat per hour with the
  chat ID and the reason name only, never a message ID (`memberDeleteShedWarn`, a map of at most 10,000 chats that forgets
  entries older than an hour and, when none can be forgotten, logs nothing more for a new chat after one process-wide line).
  The ceiling bounds the table: 20,000 queued rows support at most 20000 divided by the delay messages a second per chat (1 min
  333/s, 5 min 66/s, 10 min 33/s, 1 h 5.5/s, 6 h 0.9/s, 24 h 0.23/s), past which new messages are kept, not deleted. The
  batcher makes database writes only and never a Telegram call; it adds no Redis key. `TestMemberSelfDestructBatcherQueues`,
  `TestMemberSelfDestructBatcherSheds`, `TestMemberSelfDestructBatcherOffRace` and `TestShutdownEndsMemberDeletionBatcher` hold it.
- Member deletion, self-destruct (`member_deletion.go`, Phase 17, DEL-05): with `member:selfdestruct` stored as 60 s, 5 min, 10 min,
  1 h, 6 h or 24 h (`deletion.ValidateRuleDelay`; any other stored value is Off), every message of a real member in a group or
  supergroup (text, media, stickers, polls, commands, forwards, replies, each item of an album, `via_bot` messages) is queued by
  the batcher at its own date plus the delay and deleted by the worker (D-09), under the deletion pacer's own budget. What
  never queues, each unknown included (D-33): a creator or administrator and anyone whose admin status could not be read, an
  approved user and anyone whose approvals read failed, `sender_chat` senders (anonymous admins, channels, a linked channel's
  automatic forward), bots and the bot's own messages, Telegram service IDs, every service message (the service rules own
  them), a private chat, a Staff Group (`deletion.ScheduleBatch` refuses it and fails closed on a read error, DEL-07) and a
  member whose captcha is pending (group `-10` holds and ends that update). Pinned messages stay (D-11): a `pinned_message`
  service message drops the pending row of the message it names (exactly its chat, message, kind and the pending state: an
  acting row is left to the worker), and the bot's own `/pin` does the same through the hook in `pins.go` after its successful
  `pinChatMessage`, because the pin service message the bot causes has a bot sender that a watcher without `SetAllowBot` never
  sees; unpinning queues nothing, and `/permapin` pins a message the bot sent, which is never queued. Switching Self-destruct
  Off drops the chat's pending self-destruct rows in the same write (`RemoveRule`), and turning it on or changing the delay
  drops nothing: rows queued under the old delay keep their time (D-12). Accepted limits, stated and not tested as wanted
  behaviour: a pin that lands while the message is still in the batcher's buffer (about 300 ms) or after the worker claimed its
  row still deletes it; a pin by another bot is never seen; exemptions are judged at send time, so a member approved or
  promoted later still loses messages already queued; a command message deleted by command deletion leaves a self-destruct row
  whose later delete is refused quietly. A busy group keeps pace: the burst is offered with no write, the worker claims
  `deletionBatch` (1000) rows a cycle and makes at most one `deleteMessages` call per 100 rows of a chat, so 3000 messages are 30
  paced calls on the `fuku:delete:pace:*` keys that no reply ever touches (`TestSelfDestructBusyGroup`: the burst, a command
  answered under a blocked pacer, a 429 that leaves rows pending with no attempt counted, and the ceiling).
  `TestMemberSelfDestructQueuesMessages`, `TestMemberSelfDestructKeeps`, `TestMemberSelfDestructPinnedStays`,
  `TestMemberSelfDestructOffCancels`, `TestMemberSelfDestructNeverEndsUpdate`, `TestMemberContentMessageExcludesServiceFields`
  and `TestMemberDeletionStaffGroupNeverTouched` (all three member rules, a Staff Group and a failing Staff Group read) hold it.
- Scheduling the bot's own messages (`deletion_bot.go`): `scheduleBotMessage(sent, kind)` is opt-in at named send sites only
  (D-12): the welcome in `SendWelcomeMessage` (`bot:welcome`), the goodbye in `leftMember` (`bot:goodbye`), the group `/rules`
  reply and the click-to-read message of a group with private rules (`bot:rules`), the success replies of the manual
  punishment commands through `moderationCtx.replySuccess` (`bot:punish_manual`: `/ban`, `/dban`, `/tban`, `/kick`,
  `/dkick`, `/unban`, `/mute`, `/dmute`, `/tmute`, `/unmute`, five sites in `bans.go` and three in `mute.go`), and the one
  warn send of `warnThisUser`, whose final `source` argument (`warnSourceManual` for `/warn`, `/dwarn` and `/swarn`,
  `warnSourceAuto` for the blacklist's warn action) picks `bot:punish_manual` or `bot:punish_auto`. Nothing hooks
  `helpers.SendMessageWithErrorHandling`, `media.Send`, `media.SendGreeting` or the bot client, so a captcha challenge, a
  staff, lockdown, log or raid message, a preview and the private rules message are never reached (D-18, DEL-07; a
  grep over `fuku/utils` for the helper finds nothing). The helper returns at once for a nil message and for a chat that is
  not a `group` or `supergroup` (the type comes from the sent message Telegram answered, so a `/rules` from a private chat
  connected to a group, and the deep link's private message, are refused), reads the chat's cached rules through
  `deletionRulesLookup` (an unreadable read, a nil answer and a kind with no row schedule nothing), computes `sent_at` from
  the message's own date (`deletionSentAt`: zero, negative or future means now, a past date is never stretched) and writes
  through `deletionSchedule`, so the latch and the Staff Group refusal apply. It has no result: every failure is one Warn
  with the chat ID and kind (never a message ID, text or member), it runs a 3 s synchronous write on its own context and
  starts no goroutine. A stored delay of 0 for a bot kind (the page offers none, but the repository allows it) is a row due
  now. The body is `scheduleBotRow`, which always reads the rules first (a kind with no row is Off and writes nothing, so
  an override never creates a row) and takes an optional delay override that replaces a stored delay of any value; its two
  variants are `scheduleBotMessageCapped` (a notice with a button the member needs, queued at the 24 hour cap whatever the
  stored delay says) and `scheduleBotMessageFromNow` (the stored delay counted from this moment, for a message the bot has
  just edited). `sendRulesMessage` returns the sent message (its requests are unchanged, the Phase 12 goldens pass unedited);
  only `/rules` schedules it, the deep link and the settings preview discard it. The manual kind covers success replies
  only: validation and error replies (the "reply to a message" hint of `/ban` and `/unban` for a channel target goes through
  `moderationCtx.replyHint`, which never schedules), warn lists, `/resetwarns`, `/rmwarn`, the `/restrict` and `/unrestrict`
  question cards, `/kickme`, `/banme` and the silent variants (which send no reply) are never scheduled
  (`TestBotManualRepliesNotCovered`). Anonymous-admin re-entry runs the same command, so the hooks need no special case.
  The automatic kind (`bot:punish_auto`, plan 16-06) has these named sites and no other: every member requirement notice
  (the warn, kick, ban and mute notices of Phases 13 to 15 and the per-chat cap's pause notice) through
  `memberReqPostNotice`, the one site both callers (`memberReqAnnounce`, the cap) reach, right after a successful send and
  from the sent message's own date, so a retried or rate-limit-waited notice counts from the message that went out
  (`memberReqScheduleNotice`; a notice without a button follows the timer directly, a failed send writes nothing); the
  mute notice, which carries the "I fixed it" button, is queued at the 24 hour cap (86400 s) whatever the timer says, so a
  short timer never removes the member's only self-service way out, and with the kind Off no row is written (D-14, D-25);
  after the member taps the button, `memberReqFixedEdit` (which takes the chat type and a `reschedule` flag, SEC-01: true
  only when the hold's stored `notice_msg_id` is positive and equals the pressed message's ID, so the same button attached
  to any other message of the chat is edited as before but queues nothing for it, and a hold whose notice ID was never
  stored reschedules nothing, the 24 hour row staying) reschedules the edited
  confirmation with `scheduleBotMessageFromNow`, whose base is the moment of the tap and never `sent_at` (that plus the
  delay can already be in the past and the confirmation would vanish at once), and the queue keeps the earlier time, so a
  shorter timer moves the 24 hour row up and a longer one changes nothing; a failed edit schedules nothing and the 24
  hour row stays; the antiflood action notice in `checkFlood`; the blacklist's mute, ban and kick notices (its warn action
  goes through `warnThisUser` with `warnSourceAuto`); and the AI spam breaker's pause notice in `aispamNotifyBreaker`,
  which D-27 counts as an automatic punishment message because it is aispam's only group message (the owner can narrow it
  when confirming the phase). A delete verdict of aispam is silent in the group and the owner-log dump post of
  `aispamMirrorDeletion` is never a kind. The antiflood and aispam sends go through
  `helpers.SendMessageWithErrorHandling`, which answers no message for a chat the bot cannot post in: the hook keeps the
  returned message and `scheduleBotMessage` ignores nil. Every hook is additive around the existing send (no request, error
  path or return changes) and runs only after a send succeeded (`TestBotAutoPunishmentNotices`, `TestMemberReqNoticeScheduled`,
  `TestMemberReqMuteNoticeAtCap`, `TestMemberReqFixedRescheduleMovesUp`).
- Joined and left share one flag with `/cleanservice` (the Deletion page and the command, D-02, D-24, D-26). Whether either
  kind acts at all is `greetings.clean_service_settings`, and the two `chat_delete_rules` rows only hold a delay. Keep on
  the page and `/cleanservice off` clear the flag and remove BOTH rows; `/cleanservice on` removes both rows too, so a stale
  delay never comes back; Delete now or After N on either row turns the flag on, so the OTHER row then reads its own stored
  delay or, with none, Delete now. That coupling is documented and accepted: the owner who sets Member joined to After 10 min
  sees Member left read Delete now at once. With the flag off a stored row is ignored (it reads Keep and writes no queue
  row). A join or leave a bot caused is covered too (the watcher's, D-24; `cleanService` has never accepted a bot sender),
  and `TestDeletionEndToEnd` drives the page, the command, the human and the bot-caused paths over the one flag.
- A supergroup migration re-keys neither `chat_delete_rules` nor `scheduled_deletions` (like every per-chat setting but the
  Staff Group's own link rows): the new supergroup starts at Keep and Off, and the pending rows of the old chat name a chat
  Telegram answers "group chat was upgraded" (or the `migrate_to_chat_id` parameter), which `deletionClassify` reads as
  `gone`: the worker latches the old chat, drops its rows and deletes nothing. The owner sets the rules again on the new group.
- Entity offsets are UTF-16: slice with `extractEntityText`, and match against both `Entities` and `CaptionEntities`.
- Captcha allows one attempt per `(user, chat)`; group `-10` stores the pending user's messages for replay.
- `fuku:cache:captcha_pending:<chat>` is a per-chat pending flag; any new code that inserts into `captcha_attempts`
  must `DeleteCache` it after commit.
- Turnstile captcha: a Turnstile attempt is an ordinary `captcha_attempts` row whose answer is the reserved `ts1:` plus
  32 hex characters (`captchaweb.NewReservedAnswer`); there is no second table or kind column, so the one attempt per
  `(user, chat)`, the timer, the cleanup tick and restart recovery cover it unchanged. The pass path is
  `claimCaptchaPass` (held messages read, `CompleteCaptchaAttemptAtomic`, unmute) then `finishCaptchaPass` (summary,
  challenge deleted, success note, greeting), shared by the `captcha_verify` callback and the web verify so the two
  cannot drift. The web finish runs through `startCaptchaLifecycleTask` (drained by `StopCaptchaLifecycle`, so there is
  no new shutdown drain) or inline when the lifecycle is not running, and the work after the claim runs on
  `context.WithoutCancel` with its own 20 s deadline, so a client that disconnects never abandons an unmute. The page
  decides nothing: a pass needs signed `initData` whose `user.id` is the challenge's user and a `siteverify` success
  that echoes the expected hostname, `captcha_join` and the `cdata` (`hex(sha256("cd1:" + token))`). The package
  `fuku/utils/captchaweb` imports the standard library, go-redis and logrus only; its `Backend` implementation is
  `fuku/modules/captcha_web.go` (`captchaWebBackend`), and `ConfigureCaptchaWeb` installs the runtime. Callback data is
  not authenticated by Telegram, so both `captcha_verify` and `captcha_refresh` refuse an attempt whose answer starts with
  `ts1:` with `captcha_attempt_not_valid` and change nothing (no count, no image, no row change): a hand-built callback
  can never pass or convert a Turnstile attempt. Rolling-deploy caveat (accepted, like the others here): a replica on
  older code has no such guard until it is replaced. A Turnstile group whose web captcha is unavailable (no runtime
  installed, no bot username, or no Redis `PING` answered within 250 ms in the last 5 s, memoised in
  `captchaWebProbe`, one `atomic.Pointer` to the probe's runtime, time and answer, replaced as a unit, so a raid of joins
  costs one probe per 5 s) gives the joining member the
  math captcha with no extra message (D-09, `captchaWebAvailable`); `cache.IsRedisAvailable` is not used for this,
  because it only says a client object exists. An issue or send failure rolls the attempt back and unmutes (the existing
  `rollbackCaptchaAttempt`), with no fallback to math from inside a failed join. A re-join replaces the attempt, so the
  old token answers `expired` and never touches the new attempt: the backend matches the challenge record's attempt ID,
  never the member's current attempt.
  Outcomes (D-28): only `invalid-input-response` and `timeout-or-duplicate` as the only error codes, and a confirmed token
  whose `cdata` belongs to another challenge, are a failed solve (`captchaweb.OutcomeFailed`); it runs
  `settleCaptchaAttempt` once through `captchaWebBackend.Fail`, the decision the timer, the cleanup tick and the late check
  take as well: an approved member, or a group that turned the captcha off, is released and unmuted (no removal is
  reported, the answer is the neutral `invalid`), and everyone else gets the group's failure action through
  `handleCaptchaTimeout` (kick is one member-removing `unbanChatMember`, ban a `banChatMember`, mute the 24 h mute), the
  challenge message is deleted, the failure notice is posted and the answer is `failed` with the action. Every other
  answer is a retry and kicks nobody: a transport error, timeout, redirect (never followed, whatever transport the client has), 5xx, a body that does not parse or is over
  64 KiB, `internal-error`, `bad-request`, `missing-input-response`, an unknown or empty code list, a secret error
  (`missing-input-secret`, `invalid-input-secret`) and a hostname or action that does not match. The secret errors and the
  hostname and action mismatches are the owner's configuration faults (`Verdict.ConfigError`), logged once at Error as
  `[CaptchaWeb] siteverify configuration fault: <reason>` by reason name only. A retry gives the token back through
  `Store.Rearm` with its remaining lifetime; a wrong user, an unknown token, a failed solve, an expiry and a pass never
  re-arm. The bot's own Restrict right is checked live (`BotCanRestrict`, 3 s, never the admin cache) before any
  Cloudflare call, and a bot without it, or an unknown answer, is a retry. A `Backend` method returns an error only when
  it changed nothing, which is what makes the re-arm after one safe. `Fail` and `Expire` return a
  `captchaweb.ActionResult`: `ActionLost` (someone else decided), `ActionApplied` or `ActionNotApplied` (this call won the
  claim but the action did not take effect: Telegram refused the kick or ban, so the member was unmuted and is still in
  the group). A not-applied outcome is answered with the neutral `invalid` and no action, never `failed` or `expired`
  with a removal text, the replay marker stores the same `invalid`, and the handler logs `outcome=<result> applied=false`
  at Warn and counts an error (WR-07).
  Exactly once on any replica: a pass, a failed solve and a timeout are decided only by the conditional delete of the
  attempt row (`CompleteCaptchaAttemptAtomic` or `ReleaseCaptchaAttemptAtomic`), never by a Redis flag, so a challenge
  issued on one replica passes on another and a timer, a cleanup tick and a check on three replicas cannot all act. A
  check that arrives after `expires_at` runs `expireCaptchaAttempt` at once through `captchaWebBackend.Expire`
  (idempotent through its own claim; a live row is left to the timer) and answers `expired` with the action; it never
  unmutes. A check that lost the claim (a pass or a failed solve whose delete matched nothing) does not assume the row is
  gone, because the pass claim also matches the message ID and refresh count read a moment before and `SendCaptcha`
  stores the message ID after its send: `challengeRun.lostClaim` reads the attempt again, and a still live attempt gives
  the challenge back and answers `retry`, an unreadable one is a `retry` too, and only a row confirmed gone or expired
  answers `expired`. For the same reason `captchaWebBackend.Fail` reads the row again after an unclaimed
  `settleCaptchaAttempt` (the timeout path reports a database error as "not claimed") and returns an error when the
  attempt is still pending, so a blip re-arms the challenge instead of burning it (WR-01). A claim whose unmute failed answers `retry`,
  not `passed` (the member is not unmuted yet): the claim already wrote the `captcha_muted_users` row due now, the
  5-minute worker retries it, the challenge message is deleted and the token is not re-armed, but the pass is written
  to the done marker as `passed`, so a second tap (the page's Try again) replays `passed` with no side effect instead of
  ending on a dead `invalid` screen (WR-02). A replica that crashes between the consume and a re-arm
  loses that member's challenge until the group's time limit ends the attempt (accepted).
  The verify route (`Handler.Verify`, `POST /captcha/verify`) refuses cheapest first: the handler's own write deadline is
  extended to 20 s (the HTTP server's `WriteTimeout` is 10 s, shorter than a siteverify call plus a pass), then a method
  other than POST gets 405 with `Allow: POST` (the handler checks it itself, see the mounting above; it is not counted in
  the stats), then a handler that is not ready answers 503 retry, then the
  address window, then the body: `Content-Type` must be `application/json` (415), at most 16 KiB (413), no unknown field
  and no second JSON value (400), then the signed `initData` and the two per-user windows, and only then the in-flight
  slot: more than 64 requests holding a slot on the replica (`verifyMaxInFlight`) get 503 retry before the challenge is
  claimed or the backend is called. The slot is taken that late on purpose (WR-06): a client that sends headers and drips
  a body, or posts unsigned data, holds no slot, and it still spends its address window, which is counted whether or not
  a slot is free. Every answer, the 405 and 429 ones included, carries `Cache-Control: no-store`,
  `X-Content-Type-Options: nosniff` and a JSON body, and the route sets no CORS header and answers no preflight, so another
  site can neither read it nor post a form to it. Work after the challenge is claimed never uses the request context (see
  above). A handler built with `Options.StartNotReady` answers 503 retry, reading nothing and touching neither Redis nor
  the backend, until `SetReady(true)` (polling mode starts the HTTP server before the modules load, D-30).
  `GET /captcha/health` (`Handler.Health`, any other method than GET and HEAD gets 405) answers `200 {"status":"ok"}` only when the handler is ready and Redis answers a
  `PING` within 250 ms, else `503 {"status":"unavailable"}`, and never names an ID, a setting or a secret. The patterns are
  `RoutePage`, `RouteVerify` and `RouteHealth`, returned by `Handler.Routes`.
- The Turnstile verify route's client address is `captchaweb.ClientIP`: `RemoteAddr`, unless `RemoteAddr` is inside
  `CAPTCHA_WEB_TRUSTED_PROXIES` (`captchaweb.ParseTrustedProxies`, comma-separated IPs and CIDRs; empty entries such as a trailing comma are skipped
  by it and by the startup check `validCaptchaWebProxies` alike, and both tests hold one table of inputs, because
  `captchaweb` is a leaf and cannot share the code), in which case
  `X-Forwarded-For` (all header lines as one list) is walked from the right to the first address outside the list. `X-Real-IP`
  and `CF-Connecting-IP` are never read, because anyone can send them. A chain that is empty, all trusted or unparsable
  falls back to `RemoteAddr`. With a loopback, private or link-local peer and no trusted list, a trusted proxy's own
  address, or such an address taken from `X-Forwarded-For` (an inner proxy that is not in the list), every request would
  share one address, so the per-address window is skipped (`bucket` false) and the per-user
  windows still apply.
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
  after start) behind `SETNX fuku:staff:sweep:lock`; without Redis it runs unguarded because every staff write is
  conditional. Staff tables are never part of backup/export/import/reset.
- The settings roster (`chat_admin_index`, keep it current without anyone acting). The feed: `cache.SetAdminListObserver`
  is a hook in the admin cache loader (`fuku/utils/cache` imports nothing from `fuku/db`), installed by the `AdminIndex`
  module (priority 31, `LoadAdminIndex`, no handler and no database access at load). It reports one event per definitive
  fact, each guarded by the load's generation, so a load overtaken by an invalidation reports nothing: a successful
  non-empty list replaces the chat's rows with its creator and human administrators (a bot is never stored, and a creator
  that `settingsCreatorGone` recognises is not stored, so that group reads as having no live owner), and a bot that is not
  an administrator (`BotNotAdmin`) or whose own lookup was refused with Bot API error code 400 or 403 (`BotGone`,
  `cache.IsBotGoneError`: "chat not found", "bot was kicked", "bot is not a member", "bot was blocked") clears them; a
  401 (a revoked or rotated token), 404, 409, 429, 5xx, a transport error and a timeout leave them, so one fleet-wide
  fault cannot empty the table. This is a narrower rule than `isTelegramClientError`, which the settings doors keep
  for choosing an answer and which never changes a row. The
  retry-failure and the empty-list paths report nothing. The write runs synchronously under the chat's admin-cache
  singleflight with a 3 s bound (`adminIndexWriteTimeout`) and a failed write is only logged. `adminCacheAutoUpdate`
  clears a group's rows when a `my_chat_member` update takes the bot to left or kicked. `/admincache` refreshes the rows on
  its main path, and with a non-admin bot it loads nothing and leaves them. `refreshChatAdminIndex` is the one-group
  refresh (at most two Telegram calls, never sleeps, never touches the admin cache) shared by the sweep and the group
  `/settings` repair path: it changes rows only on definitive facts, and returns the Telegram error whatever the outcome.
  The sweep (`StartAdminIndexSweeper`/`StopAdminIndexSweeper`, `admin_index_sweeper.go`, started in `postInit` right after
  `StartStaffSweeper`): the first cycle 30 to 90 s after start is the backfill, so the private list works right after a
  deploy, then one cycle every hour, one code path for both. The candidates are every negative chat ID in `chats`
  (`adminindex.ListCandidateChatIDsFresh`), inactive chats included, because the activity monitor marks quiet groups
  inactive. Every Telegram request waits a 250 ms pace and runs under 5 s (4 calls a second per runner, about 8 minutes for
  1000 groups). Definitive facts only: an error, a timeout, a 429 or an empty list leaves a chat's rows as they were, and
  only a bot that is not an administrator, or whose own lookup answered 400 or 403 (`cache.IsBotGoneError`), clears them;
  a 401 or any other error code leaves them. A 429 waits `min(retry_after, 30 s)` and
  retries that chat once; a second 429 ends the cycle with the remaining chats untouched. A panic in a cycle is recovered
  and the next cycle runs, Start twice runs one loop and Stop is idempotent. The roster is a hint for the private list and
  never authority.
- The lockdown join guard (handler group `-7`, `lockdownOnJoinMember`) reads the lockdown fresh from PostgreSQL, records
  the joiner row first and only then ends the update with `ext.EndGroups`, so no welcome, captcha challenge or captcha
  attempt follows. It acts only on a confirmed lockdown (`locked_at` set); an unconfirmed row is not a lockdown yet and
  every join path lets the joiner through. Its write to the `fuku:lockdown:recent:<chat>` tracker comes first, is best
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
  start runs synchronously in the handler, and only the posts that follow it run on goroutines (see the announcement
  bullet above); a crash half way leaves an unconfirmed row the worker settles. The counter shares its one live `getChatMember` of the performer with the guard (it returns the
  status it read and whether it asked), so a join or a join message still makes at most one performer lookup. Settings come from
  `raidSettingsLookup`, a test seam; an unreadable read counts nothing.
- The flood trigger is counted by two watchers that share one function, `raidFloodCountUpdate`: the group `-4` watcher
  (`raidFloodWatcher`, registered in `LoadLockdown`) and the group `12` watcher (`raidFloodLateWatcher`,
  `raidFloodLateGroup`). Both return `ext.ContinueGroups` on every path, so aispam, antiflood, locks and blacklists still
  see every message, and delete nothing: the lockdown's muting stops the flood. What counts (`raidFloodItemKey`) is any
  user content in a supergroup: text, photo, GIF (animation, checked before the document Telegram also sets), sticker,
  video, video note, voice, audio, document, poll, contact, location, venue, dice and story, forwards and replies
  included; an album counts once. A command the bot handles does not count (D-07), and the dispatcher is what says
  which: a handled command returns `ext.EndGroups`, which ends every later group. So the `-4` watcher counts every item
  except a slash-prefixed text (`raidFloodIsSlashText`: a `bot_command` entity at offset 0, or a text starting with `/`
  and a non-space character, which gotgbot's command handler also runs), and the group `12` watcher counts only the
  slash-prefixed texts that reach it, the ones no command took: `/word` with no handler, `/////`, and a command addressed
  to another bot (`/start@otherbot`). Never keep a second list of command names for this. Group `12` must stay after every
  other group, so every new handler group needs a number below it. A slash text that a handler ends is not counted, and
  neither is one that an earlier handler ends (an aispam, antiflood, lock or blacklist deletion at groups `3` to `7`); a
  command that returns `nil` or an error instead of `ext.EndGroups` is counted late, which only raises the counts, so
  new commands should still end with `ext.EndGroups`. Service messages, edited messages, channel posts and bot senders
  never count (gotgbot drops the last three before the watcher). Who never counts: a sender with
  `sender_chat` set (an anonymous administrator, a channel, a linked channel's automatic forward: gotgbot's `IsBot` is
  false for all of them), a Telegram service ID (`staffServiceUserIDs`), the bot, an approved user
  (`IsApprovedForUpdate`) and an administrator by the cached list (`antifloodModule.userIsFloodExempt`; its bounded live
  fallback assumes an administrator when saturated, which only lowers the counts). Checks run cheapest first (chat type,
  sender, item, Redis, settings, approvals, admin list, then one script call under 250 ms; an error is no information).
  The rule is `raidFloodCounts`: at least the chat's threshold (default 30) of items in the fixed 30 s window, at least
  60% of them from members who joined in the last 24 h, from at least 3 such accounts; the windows, the 60% and the
  3-account floor are code constants. It then calls `raidFire` with `trigger_kind` `flood`, the item count, window 30
  and the percent. Without Redis it is inert.
- AI never decides alone (RAID-05). A `/aispam` delete verdict only adds one item to the AI burst counter, through
  `raidAIRecordVerdict` at the end of `processAISpam`'s delete branch, which first reads the chat's own settings
  (`raidSettingsLookup` under `raidSettingsTimeout`): an off `ai_enabled` or an unreadable setting counts nothing, so
  the Redis sets are not written either. Only `raidAICounts` (at least the chat's `ai_threshold`, default 5
  (`raiddetection.DefaultAIThreshold`), of verdict items from at least 3 accounts in the fixed 120 s window, and a
  threshold below `raiddetection.MinThreshold` never counts) calls `raidFire` with `trigger_kind` `ai_burst`, exactly
  as `/lockdown` and the other triggers start a lockdown. Every delete verdict from any account feeds it (D-19), an
  established member's included, and a keep verdict never does; a basic group, an anonymous or non-positive sender and
  the bot itself never count. Nothing on the counting path deletes, restricts or bans: the `/aispam` deletion itself is
  unchanged, and the lockdown's muting is what stops the raid. The start runs on its own goroutine
  (`raidAIFireAsync`, counted in `raidAIFires`, a `raidPostTracker`, and starting with
  `defer error_handling.RecoverFromPanic`), so no `/aispam` or raid AI worker waits on it; `raidFire` keeps its own
  `raidFireTimeout` and its 60 s latch, which bounds how many starts exist per chat. During a rolling
  deploy an old replica that wins `BeginAlert` for an `ai_burst` row renders the manual wording and no spammer sample;
  that is accepted, like the other rolling-deploy caveats here.
- The raid AI candidate watcher (`raidAIWatcher`, group `2`, registered in `LoadLockdown`) picks the texts the raid path
  asks TypeSafe about, even in a group that never enabled `/aispam` (D-01, D-02). It returns `ext.ContinueGroups` on
  every path, never waits on AI, and deletes, restricts, bans and posts nothing. It is supergroups only, and a text
  candidate is a message with `Text` set: a caption belongs to the image path. Checks run cheapest first: the chat
  type, `raidFloodSenderCounts` (so no `sender_chat`, automatic forward, Telegram service ID or the bot), the item, the
  provider (`raidAITextConfigured`: `ENABLE_AISPAM` and `TYPESAFE_API_KEY`), Redis, the chat's settings (`ai_enabled`),
  the candidate script, the rule, then `aispamCarvedOut` (pending captcha, cached admin, approved) because it costs
  more, then the enqueue. The rule is: the sender is a new member (joined within 24 h) and the text carries a link
  (`aispamLinkDomains`), is a forward, or repeats one of their last 5 texts of at least 10 runes. Established members'
  messages, plain texts, admins, approved users, members with a pending captcha and basic groups are never sent to any AI.
- A raid-only text check sends TypeSafe less than `/aispam` does (`raidAIState`, built by `raidAIStateOf`, WR-03), because the
  chat never switched `/aispam` on: the chat's title and language, the sender's first-seen days and a
  `messages_last_hour` of 1 (the raid path keeps no sender history), and the message's text, `has_link`, link domains
  and `is_forward`. It never carries the chat description (no live `getChat`), the chat rules, the sender's warn
  count, the reply flag or `recent_from_sender`; `raidAIState` has no field for them, and `/aispam`'s own `aispamState`
  request is unchanged (`aispamJevDecide` takes either). `TestRaidAITextRequestIsMinimal` reads the request body the
  TypeSafe server receives. The `/lockdown` help never claimed more than "suspicious text ... is checked by TypeSafe",
  so it is unchanged.
- The raid path's text checks never double the `/aispam` call: `aispamWillJudge` (the TypeSafe configuration,
  `aispam.IsAISpamEnabled` and a breaker that is not paused, the gates at the top of `checkAISpam`) hands a text to
  `/aispam` whenever `/aispam` will judge it, and the watcher then sends nothing, so a message makes at most one
  TypeSafe call; `/aispam`'s own delete verdict is what feeds the burst counter there (D-03, D-19). Where `/aispam` is
  off, the raid-only check reports a verdict and never deletes. Raid-only checks never record the `/aispam` sender
  history (it stores raw text), never post the `/aispam` breaker notice (the tripped return of
  `aispamBreakerRecordFailure` is ignored, because the chat may never have enabled `/aispam`) and share its per-chat
  breaker silently: a paused chat is not asked and the check counts as shed. Without Redis the watcher does nothing.
- The raid AI pool (`raid_ai.go`, D-18) is separate from the `/aispam` queue and shared by raid-only text and image
  checks: 128 queued jobs (`raidAIQueueCapacity`), 4 workers (`raidAIWorkerCount`) started once, each recovering from
  panics, per job too. A full queue or a draining process sheds the job (`raidAIShed`); the in-flight count is a
  `raidPostTracker` (`raidAIJobs`), taken before `raidAIDraining` is read, never a `sync.WaitGroup`. A raid-only text job
  reports a spam verdict (`aispamIsDelete`) to `raidAIRecordVerdict` and nothing else; a slow, failing or paused
  provider is no verdict. The worker that reports the tipping verdict hands the lockdown start to `raidAIFireAsync` and
  moves on (above). Every provider call derives from `raidAIContext()`, which `StopRaidAI` cancels at shutdown (see
  Startup and shutdown). Tests that call `StopRaidAI` must restore the pool afterwards (`restoreRaidAIPool`), because a
  drain sets the draining flag and cancels the context for good.
- The raid AI image checks (`raid_ai_media.go`, `raid_ai_image.go`) are the Claude half of RAID-04. The same group `2`
  watcher (`raidAIWatch`) sends a message with no `Text` and a media frame (`raidAIMediaRef`) down the image path: only a
  new member's media is a candidate (the candidate script asked with an empty hash, so nothing is recorded), after
  `raidAIImageConfigured` (`ENABLE_AISPAM` and `ANTHROPIC_API_KEY`), Redis, the chat's `ai_enabled` and `aispamCarvedOut`, and the job joins the
  shared pool. Without the key no `getFile` is made and no image leaves the server. The frame is the photo size whose
  larger side is nearest 1024 px, a regular static sticker's WebP, or Telegram's own thumbnail for a GIF, video, animated
  or video sticker; a missing thumbnail, a custom emoji or mask sticker, a video note, a document, a voice note, a
  declared size over `claude.MaxImageBytes` (5 MiB), a body over it, or bytes that `http.DetectContentType` does not call
  JPEG, PNG or WebP give nothing. Only the image bytes and the code-owned instruction go to Anthropic, never the caption,
  names, chat or user data (the separate AI spam-name rule sends the three names and nothing else). A verdict of at least `raidAIImageBar` (0.85, a code constant) counts as one spam item for
  the AI burst counter, anything below it is kept; a refusal stop counts in `RaidAIStats.Blocked` and every other
  failure (`getFile`, download, provider) in `Failed`, and both are no verdict. A retired or unknown model answers 404
  `not_found_error`, a failure logged at Warn with its status and error type (there is no model-stage warning: a Messages
  answer carries no lifecycle stage). The request carries the image block, one fixed text block, the code-owned system
  instruction and one forced strict tool (`claude.ToolName`, a single `spam_probability` number); thinking is disabled
  and no `temperature`, `top_p` or `top_k` is sent, because the default model answers 400 to them.
  Nothing on this path deletes, restricts, bans or posts, and every call derives from `raidAIContext()`.
- Before any provider call an image job passes, cheapest and most decisive first (`raidAIProcessImage`): the album gate, the
  fire-latch skip (`EXISTS fuku:raid:{<chat>}:fire`: a lockdown was just attempted, so more checks are wasted), the verdict
  cache by `file_unique_id`, then the hourly budgets, then the download and the call. Each refusal is no verdict and is
  counted: `RaidAIStats.Capped` for a budget refusal, `RaidAIStats.Cached` for a verdict served from the cache, a photo
  dropped by the album gate or a check skipped for the latch (a Redis error counts as `Shed`). A cached verdict costs no
  budget and no provider call; it is judged against the same bar and counted as spam or kept like a fresh one, but it is
  not `Checked`, which counts only provider answers that carried a verdict. Jobs that carry one image at the same moment
  share one verdict step (`raidAIImageFlight`, a singleflight keyed by chat ID and `file_unique_id`), so overlapping
  posts of one sticker in one chat make one call, the followers count as cached and they share its failure too. Another
  chat never joins that flight, so it cannot inherit a refusal that belongs to the leader's chat; the verdict cache still
  serves it afterwards. The join and flood triggers are
  not touched by any of this.
- The image checks have their own provider breakers (`raid_ai_breaker.go`), checked after the verdict cache and before the
  budget, so a paused check spends no unit. Five provider failures in one chat pause that chat's image checks for 5 minutes
  (`raidAIImageFailures`, `raidAIImageCooldown`); a success clears the count. A provider-wide pause stops every chat's
  image checks: a rejected key or account (`claude.HTTPError.KeyRejected()`: 401, 402 or 403) pauses all chats for 5
  minutes (`raidAIImageKeyPause`), and a 429 whose `retry-after` (`RetryDelay`) is over 2 s pauses all chats for that
  delay, at most 5 minutes (the Claude client itself retries a shorter wait once and drops a longer one; a 5xx, 529,
  network error or unusable answer counts toward the chat's five). Each pause logs one warning naming its reason (status
  and error type only) and its end, and none posts anything: a paused check is `Shed`. Only provider answers count; a
  `getFile` or download failure never does, and a refusal stop counts as an answer, not a failure. The breaker clock is
  `raidNow`. The breaker map holds at most 10000 chats (`raidAIImageBreakerMaxChats`); when it is full the idle breakers (no failures,
  not paused) are evicted, and if every tracked chat is failing or paused a new chat stays untracked, so it cannot be
  paused, with one warning per process. The
  raid-only TypeSafe text checks keep sharing `/aispam`'s per-chat breaker silently and never touch these.
- An automatic lockdown (`lockdownStartedAutomatically`: `trigger_kind` `join_surge`, `flood` or `ai_burst`) is
  described as such to admins. Every site that switches on the kind knows all three: `lockdownTriggerSentence`,
  `raidPostNotice` (an unknown kind posts no public notice), `lockdownAlertTriggerLine` and `lockdownRenderSample`
  (counts line `lockdown_alert_counts_ai` for an AI alert, which does not say the listed accounts joined; the burst holds
  recent joiners only, see `:aispammers` above). `TestLockdownTriggerKindsAllSurfaces` renders every kind in
  `models.LockdownTriggerKinds()` on every one of them, so a new kind added there without the same case everywhere
  fails it. `/lockdownstatus` opens with `lockdown_status_active_auto` and the trigger sentence
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
  command, which any cached admin could use. There is no join handler at group `-5` (the group now hosts the service message deletion watcher, which is not a join handler) and no expiry poller: nobody is
  temp-banned by the raid mode, whatever an old `fuku:antiraid:*` key says. The keys are removed by
  `sweepRetiredAntiRaidKeys` (`SCAN` with `COUNT 100` and `UNLINK`, started once per process from `LoadAntiRaid` in a
  recover-protected goroutine, bounded to 30 s, a no-op without Redis). During a rolling deploy an old replica can still
  temp-ban until it stops and can leave new keys, which the next start's sweep clears. The `antiraid_settings` table and
  its model stay (migrations are append-only, and the legacy backup module reads and resets the table directly); the
  `fuku/db/antiraid` repository is gone, since nothing but tests called it, and no command reads the timing columns
  any more.
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
- `resolveUnmutePermissions` returns `(permissions, error)` and is the one choke point of every unmute; it is a wrapper of
  `resolveUnmutePermissionsWithLockdown`, which returns the same permissions and error plus whether a lockdown was
  active, so a caller can choose its words without a second lookup (the member requirements' lift of a mute whose notice
  could not be delivered, their rejoin reset and "I fixed it" use the sibling through `memberReqUnmutePermissions` and
  `memberReqLiftMute`).
  During an active lockdown it returns the lockdown's stored pre-lockdown permissions, because the group's live defaults are then
  the locked set and copying them into a user's own restriction would leave them muted after the lift (D-23). Every unmute
  path goes through it: `/unmute`, the unrestrict button's Unmute, a captcha pass, staff `/unmute` (and so the undo of a
  staff mute) and the requirements' lifts (the undeliverable notice, the rejoin reset and "I fixed it"). It reads the lockdown fresh through the `lockdownSnapshotLookup` seam, only for a chat with a non-zero ID,
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
- Greeting media types (Phase 12): `db.ANIMATION` (9) is a GIF saved by `/setwelcome`, `/setgoodbye` or the message
  editor's media part, stored with the animation's file ID and sent with `sendAnimation`. Only the greeting extraction
  returns it: `extractGreetingMedia` (animation first, then `extractMediaFromReply`) sits behind `ExtractWelcome` and
  `ExtractMedia`, while notes, filters and pins keep `extractMediaFromReply` and store a GIF as a document
  (`TestExtractNoteAndFilterGifStaysDocument`, `pins.go` untouched). A GIF stored before this phase stays a document, is
  not converted, sends `sendDocument` as before and reads File or GIF on the page. `media.Content.CaptionAbove` becomes
  `show_caption_above_media` only in `sendPhoto`, `sendVideo` and `sendAnimation` (now one `sendCaptioned`; no other
  sender reads it) and a false flag adds no parameter, so every request of a message saved before is unchanged.
  `media.CaptionAboveEligible` (photo, video, animation) is the one predicate of "Media under text works for this
  media". `media.SendGreetingMessage` ANDs the flag with it, and `media.SendGreeting` is its flag-off wrapper. Rolling
  deploy caveat (accepted, like the others here): a replica on the previous code sends a type 9 welcome as its text
  only (`Unknown message type 9, falling back to text`) until it is replaced.
- One setting, one code path (the settings menu): every setting the menu changes has one apply function beside its
  command (`applyCaptchaTimeout` in `captcha.go` first), which holds the validation, the refusals, the repository write
  and its `cache.DeleteCache`, and returns a typed result (`captchaApplyResult`: outcome, the locale key the command
  replies with, the stored row read back, the repository error). The command maps the result to its unchanged reply and
  the menu to a notice or a redraw, so a new setting adds one apply function and never a second write path. Pages read
  through the getter their command reads, and a read error is the cannot-open edit, so a getter that hides read errors is
  never used by a page (`settingsCaptchaRead` is the seam). The menu keeps no state: no Redis key, no migration, no model,
  no shutdown drain, so a press works on any replica with Redis down. Accepted skew: the `captcha_settings` and
  `antiflood` keys are not in `skipLocal`, so another replica can show an old value for up to `CACHE_LOCAL_TTL`, exactly
  as the commands can; the replica that took the press is consistent. A group menu's text is readable by every member of
  the group, and a menu or choice message stays until used or closed (no auto-close timer). The "Only {name} can use this
  menu." line carries an escaped name, not a mention, so it pings nobody. The message editor follows the same rule: the
  switches, Remove and every typed part call the greetings and rules apply functions that sit beside the commands, so
  the menu, the typed reply and `/setwelcome`, `/resetwelcome`, `/setrules` and the rest write one way. The join, the
  leave, `/welcome`, `/rules` and the previews all build their message through `renderGreeting` and `buildRulesMessage`
  (which builds its text with `buildRulesText`), never a second builder, and a preview never writes and never sends to
  the group.
- Rules message (`rules.go`): `/rules` (in the group and in a private chat connected to it), the private rules message the
  `/start rules_<chat id>` deep link opens and the Rules page's Full preview are built by
  `buildRulesMessage` and sent by `sendRulesMessage`. The builder has two forms, the group form (`rulesFormGroup`: heading
  `rules_for_chat_header`, no-rules line `rules_no_rules_set`) and the private form (`rulesFormPrivate`: `rules_for_chat`,
  `rules_not_set`); a row with media or buttons but no text gets the heading alone as its text or caption. Rules are set
  when `RulesSettings.HasContent` holds (a text, a media or a button), which is also what gates a welcome's `{rules}`
  button, so a media-only rules row keeps that button; only a row with none of the three answers the no-rules line.
  `sendRulesMessage`'s text-only branch is each caller's old `SendMessage` call (the options are passed in, so an old row's
  request is byte-identical: previews on and the empty reply markup for `/rules`, `formatting.Shtml()` for the deep link;
  the `rules_legacy_request_test.go` goldens prove it and are never re-recorded) with the rules keyboard added only when
  the rules have buttons, and it is never routed through `media.SendGreeting`, which turns link previews off and drops the
  reply target. Its media branch is one `media.Send`: the media with the text as its caption and the buttons under it,
  and `show_caption_above_media` only when Media under text is on and `CaptionAboveEligible` accepts the kind (a photo, a
  video or an animation; a stored on with any other kind is ignored). With private rules on, the click-to-read message in
  the group is unchanged and carries no media and no buttons; they are in the private message. When Telegram definitely
  refuses the media send (a 4xx other than 429, by `isTelegramClientError`: a caption over 1024 that `/setrules` or a
  backup import let in, another bot's file ID), `/rules` and the private rules message retry once as the text and the
  buttons alone. This retry is a deliberate exception to the one-message rule (Phase 12 D-03): it drops the owner's media
  and sends the text and the buttons alone, so a rules media can disappear from `/rules` and the private rules message.
  Every such retry
  is logged at Warn with the chat ID only (`[Rules] the rules media of chat <id> was refused`; the chat is the one the
  message was sent to, so the member's private chat for the deep link), never the text, a caption, a button, a link or
  a file ID. A 429, a 5xx and a transport error are not retried. Full preview never falls back (it passes `degrade`
  false), so the owner sees the refusal as `settings_edit_preview_failed` and can fix the stored message.
- A menu opened in the group never shows the Staff Group's title or chat ID (D-21): not in any text, button label, notice,
  alert or callback data, in any of the 8 languages, on any page, picker, card or refusal. Nor does a private menu, except
  to the live creator of the group being configured (D-32): a private menu is shown to every live admin of the group, who
  need not be staff, so only the person who can link the group (the press's own live `getChatMember` record has status
  creator, `settingsView.Creator`, no extra Telegram call) sees `settings_staff_linked_to`; any other admin gets the same
  masked line (`settings_staff_linked_hidden`) as a group menu, and the page does not even read the Staff Group row for
  them. The Deletion page is not drawn in a Staff Group at all (`Hidden`, D-18) and its screens never name one. A Staff Group's own page (the read-only panel inside a Staff Group) may show its linked groups to the people of
  that Staff Group. `settingsStaffViewFor` is the one place that decides: it copies the title and chat ID into
  `settingsStaffView` only for the creator's private menu or a Staff Group's own page and clears the row's link either
  way, so every other renderer never receives them; the picker's labels in a group menu are positions
  (`settings_btn_staff_pick_n`) and its buttons carry the row ID (a private picker lists only the presser's own Staff
  Groups, by title). Filtering text after the fact is never the fix.
  `staff_link_done` and `staff_notice_unlinked_by` go to the Staff Group only, as the commands post them, and nothing about
  a link is posted into the linked group. Link and Unlink run `listVerifiedOwnStaffGroups`, `linkToStaffGroup`,
  `announceStaffLink`, `checkUnlinkAuthority` and `runUnlinkGroup`, so the menu and `/linkstaff` and `/unlinkstaff` cannot
  diverge, and a refusal is the command's own text through `staffLinkRefusalText` or `staffUnlinkRefusalText` as a notice, except that
  a presser who owns no Staff Group gets `settings_staff_no_group` (`settingsStaffLinkRefusalText`), because the command's
  text ends "send this command again", which is wrong in a button flow (D-34).
  The page reads the database first (a failed read is the cannot-open alert and edit), then answers the press empty, then
  makes its live health checks (`buildStaffPanelRows`, at most 2N+1 Telegram calls, `staffPanelBuildTimeout`), and a link
  the owner check removed draws as "Not linked" with no notice. Inside a Staff Group the page is the `/staff` panel
  read-only (`renderSettingsStaffPanel`, 8 groups a page, inside 3800 UTF-16 units with the settings header and the
  `settings_staff_readonly` line) with no Link, Unlink, Add group or Recent actions. `TestSettingsStaffPrivacy` renders
  every reachable screen in all 8 languages (the private list empty and populated, the Main status block, the Access page
  and its card included), runs a group menu end to end and runs a private menu of an admin who is not the creator end to
  end.
- The editor pages and their Main lines read through `greetings.LoadSettingsContext` and `rules.LoadRulesSettingsContext`
  (the `settingsEditGreetingsRead` and `settingsEditRulesRead` seams), which create nothing and return their errors, and
  never through `GetGreetingSettings` or `GetChatRulesInfo`, which create rows and hide a failed read: opening Main or a
  page leaves no greetings, rules or chats row behind, and a failed read is the cannot-open edit on the page and the
  unknown mark on Main, never a default. A chat with no greetings row reads as Welcome On, Goodbye On (the database
  default of `goodbye_enabled`, which the first write stores for a chat the bot knows) and Delete last Off, which is what
  the bot then does, and the pages show exactly that (`TestSettingsEditDefaultsMatchRuntime`).
- Message editor (Phase 11), the pieces in one place; each has its own bullet: the pages, Set, Cancel and Remove are
  under Callbacks (`settings|v1`, codes `w g r`, parts `x m b`, controls `e d o u`: `o` Greet once on Welcome, `u` Media under text on all three pages), the wait and its keys under Data
  (`fuku:settings:input:*`), the typed-reply door at handler group 1 and `/cancel` under Handlers, the check
  (`settingsEditCheck`, pure; the text and the buttons part hold one keyboard limit between them, at most 100 buttons and 8
  in a row, with the `{rules}` button that a stored text adds counted against the stored buttons whichever part is being
  set) and the right of each part under Permissions. A part is written only by the apply function
  of its command, whether the owner pressed a button or typed the message, and a typed reply is judged like a press:
  live `getChatMember`, owner gate, right, live `getChat`, then the check, the claim and the write. Previews
  (`settings_edit_preview.go`) are the actions `wv` `wf` (Welcome), `gv` `gf` (Goodbye) and `rv` `rf` (Rules): `v` previews
  the part named by `t` (only a part that is set, plus a Welcome or Goodbye text, which always has the default), `f` is the
  whole message with no `t`, and both need only the admin check every press makes. A preview is the header line
  (`settings_edit_preview_header`, or `_rules`) and then the content. Full preview of a Welcome or Goodbye is built by
  `renderGreeting` (the join's own builder, filled in with the presser's name and the group's title) and sent by
  `media.SendGreetingMessage` with `CaptionAbove` from the stored Media under text switch (the join's own sender, so a
  photo, video or GIF arrives with its caption above it exactly as the join or the leave sends it); Preview of one part is
  sent by `media.SendGreeting` (the media alone has no caption, so it never carries the placement; the Rules `m` and `b`
  parts use the same two helpers, `settingsEditPreviewMediaAlone` and `settingsEditPreviewButtonsAlone`). Every preview goes
  to the presser's own private chat (`query.From.Id`), never to the group and never through the senders that keep the
  clean-old bookkeeping, so it writes no `last_msg_id`, deletes no message and writes no greeting or rules text, and it
  never calls `SendWelcomeMessage` or `ClaimGreeting`: a preview ignores Greet once (A9), claims no member and leaves no
  `greeted_members` row, so the member's real join is still welcomed (`TestSettingsEditPreviewNeverClaims`). One side effect
  is the join path's own: a Full preview, or a text preview, of a Welcome or Goodbye whose text contains `{rules}` reads the
  rules through the join path's reader (`renderGreeting`, then `rules.GetChatRulesInfo`), which creates an empty rules row
  (and a chats row) for a group that has none, exactly as a real join does; nothing else is written. The Rules Full preview is built
  by `buildRulesMessage` (group form) in the group's language, read by `lang.GetGroupLanguage` with the group's chat ID (the
  cached read `/rules` makes, uk on a miss, no row created), and sent by `sendRulesMessage` without the member-path
  degrade, so it is the message `/rules` sends in the group with private rules off, media, caption placement and
  buttons included; the header line before it stays in the presser's language. With private rules on it still shows the
  group form (members then read the private message in their own language, which no single preview can show, A12). A menu opened in the
  private chat answers the press empty; one opened in the group answers the toast `settings_edit_preview_sent`; a Telegram
  403 (classified by code, `settingsEditIsForbidden`) is the alert `settings_edit_preview_need_start` and any other
  failure `settings_edit_preview_failed`, with Telegram's wording in the log only; a stale Preview of an empty part is
  `settings_edit_nothing_to_preview` and sends nothing. The page reads are uncached and have no side effect (see the bullet
  above), so a page and a typed save never create a row or fill a cache entry the commands would not; a preview does so
  only through the `{rules}` read described above, which is the join path's. No
  editor screen names the Staff Group (D-21, D-32) and every prompt, refusal, saved page and wait-ended reply fits 3800
  UTF-16 units and 64-byte buttons in all 8 languages (`TestSettingsStaffPrivacy`, `TestSettingsEditScreensFitEveryLocale`;
  the door's refusal and the saved notice come from the pure `renderSettingsInputRefusal` and
  `settingsInputSavedNotice`, so a test renders exactly what is sent). Known and accepted: the `/rules` text is escaped
  twice for a rules text with `'`, `&` or `<` and no formatting (a pre-existing behaviour the editor keeps for parity), and
  a first Set on a group with no greetings row also turns the goodbye on (the database default; the page shows it).
  Also known and accepted, like the other rolling-deploy caveats here: during a rolling deploy a typed reply that lands on
  a replica still on the previous code (no group-1 door and no `/cancel`) passes through with no answer, and the wait stays
  until it expires. EDIT-07 limit (D-07, D-08): a GIF stored before Phase 12 is still a document, shows exactly as before and gets no Media under text switch until the owner sets that GIF again, which stores it as an animation and makes the switch appear. A text over the limit that the Telegram client splits into several consecutive messages is handled
  message by message, so whichever part claims the wait first is stored and the other parts are dropped without a reply;
  the owner sees the saved page with its length, and there is no ordering signal for plain texts to do better. A claim
  that is never finished or released (a crash, or a Redis timeout on finish) stays until the wait's 10 minutes end,
  during which the sender's non-command private messages get no reply, the prompt's Cancel answers that it is no longer
  waiting and `/cancel` says there is nothing to cancel; pressing Set again replaces the wait and its claim. Rules shares the Welcome's media and buttons predicates (no list of kinds of its own), holds heading plus text to 1024 UTF-16 units
  while any media is stored (`settings_edit_refuse_text_rules_long` on the text part, `settings_edit_refuse_media_rules_long` on the media part,
  both measured by `settingsEditRulesLongest` over both heading forms and the 8 languages with the group's real title), keeps the old 4096 limit
  and its `settings_edit_refuse_too_long_filled` wording without media (so removing the media keeps a long text up to 3800), and counts no `{rules}`
  token of its stored text as a button, because `/rules` adds none (`TestSettingsRulesCaptionParity` holds the Rules and Welcome boundaries
  together for every media kind).

## Go rules

- Never discard a DB error on a state-changing path.
- Every fire-and-forget goroutine starts with `defer error_handling.RecoverFromPanic(...)`. Async user/chat writers
  join the WaitGroup drained by `DrainUsersAsyncWrites`.
- Register every new secret with `logredact.RegisterSecret` (≥6 chars), or it is logged verbatim.
- A `singleflight` `DoChan` leader that panics crashes the process (the library re-raises the panic in a goroutine nothing can
  recover), so the function handed to `DoChan` recovers internally (`memberReqFlightValue` starts with
  `defer error_handling.RecoverFromPanic`) and returns no value, which `memberReqFlight` reads as the zero verdict.
- SQL log lines carry no parameters: the GORM logger in `fuku/db/conn.go` sets `ParameterizedQueries: true`, so a failed or
  slow (over 1 s) statement is logged with its placeholders and never the welcome, goodbye or rules text, button links or
  other values it wrote. Do not add a logger that prints bound values.
- A file the bot downloads itself never puts its URL in an error or a log: the Telegram file URL holds the bot token and
  `net/http` errors embed it. Rebuild a transport error from `url.Error.Err`, report a bad status as the code alone,
  follow no redirect (`CheckRedirect` returning `http.ErrUseLastResponse`), and never wrap or `%v` a request or a file
  (`raidAIDownload` is the pattern).
- `fuku/utils/claude` imports the standard library only, so `scripts/claude-spike` can use it (and the leaf
  `fuku/utils/namecanon`, see below) without the pre-main database connection of `fuku/config` and `fuku/db`. The Anthropic key travels only in the `x-api-key` header, never
  in a URL, and no error that package returns contains it. No sampling parameter (`temperature`, `top_p`, `top_k`) is
  ever sent, because the default model rejects them with a 400. `claude.Options` and `GetModel` exist for the spike tool
  only; its zero value is the production request, and the bot never sets it. The package also holds the names request
  (`Client.ClassifyNames`, Phase 15): one strict forced tool call (`ToolName`, `thinking` disabled, `max_tokens` 128),
  the code-owned `NamesInstruction` as the system text, the three canonical strings as one JSON object in one user block
  (`first_name`, `last_name`, `username`, a defensive rune cut of 64, 64 and 32) and nothing else about the account or
  the group, and the same strict `parseVerdict` as the image path (a refusal is `ErrBlocked`, any other unreadable answer
  wraps `ErrNoVerdict`, only a finite 0..1 number from a tool call that stopped with `tool_use` is a verdict). Both
  requests are assembled by one `newRequest`, which alone applies `Options`. The model must accept a forced tool call,
  so a Haiku-class model: Opus 5.5, Sonnet 5.5 and the Fable 5.1 family answer 400 to it, which the breaker reads as a
  provider failure. The package never imports the canonical form (`fuku/utils/namecanon`): `ClassifyNames` takes names
  that are already canonical and logs nothing.
- `fuku/utils/namecanon` (standard library plus `golang.org/x/text/unicode/norm`, no `fuku/config` or `fuku/db` import,
  asserted by `TestCanonPackageIsLeaf`) is the one definition of a member's canonical names (`Of`: per field it drops
  control, format and default-ignorable runes and U+2800, applies NFKC, collapses whitespace, cuts by runes to 64, 64
  and 32, strips one leading `@` from the lower-cased username, and keeps the case of the first and last name in the
  text that is sent), of the names hash (tag `mrn1`, length-prefixed, first and last lower-cased inside the hash only)
  and of the verdict key (`VerdictKey`, tag `mrv1`, includes the model). The bot and `scripts/claude-spike` share it.
  A change to the canonical form bumps `mrn1` and `mrv1`, and a change to `claude.NamesInstruction` bumps `mrv1`, in the
  same commit, so no old baseline or cached verdict is read under a new meaning; `TestCanonHash` and
  `TestCanonVerdictKey` pin one vector each for that reason.
- A missing i18n key returns `""` with an error callers ignore; a typo'd key in Go ships an empty message. Copy key
  names from `locales/en.yml`. `locales/config.yml` is the help alt-name pseudo-locale, not a language.
- The docs generator finds raw commands in the module source by the shim's call, `helpers.NewBotCommand("name", ...)`
  (`scripts/generate_docs/parsers.go`); a command registered another way is not listed, and the guard test rejects it.
- The docs generator reads only `locales/en.yml`. Pages marked `<!-- MANUALLY MAINTAINED: do not regenerate -->` in
  their first 512 bytes are frozen; `api-reference/lock-types.md` ignores the marker.
- Imports: stdlib, third-party, internal, separated by blank lines. Use `helpers.Ptr[T]` for option pointers.

## Toolchain

- Production builds use `CGO_ENABLED=0`; tests need `CGO_ENABLED=1` (go-sqlite3). Do not unify them.
- Go 1.26.0 with no `toolchain` directive. Keep dependencies on the latest upstream versions that pass required
  checks. Any intentional pin must document its compatibility reason and recheck trigger.
- Version strings are shape-locked (`BotVersion:` with two spaces in `fuku/config/config.go`, `version =` in
  `main.go`). Run `make bump-version` on a PR branch, merge, then tag. Tags match `^v\d+\.\d+\.\d+(-[0-9A-Za-z.]+)?$`.
- `.env` in cwd auto-loads without overriding real env. Run `--version` smoke checks from `/tmp`.
- Defaults that surprise: `REDIS_DB=1`; `ENABLE_AISPAM=true` (the global AI kill switch, text, images and the AI spam-name rule; the text checks are inert without `TYPESAFE_API_KEY`); `ANTHROPIC_API_KEY` empty
  means no raid image check, no AI name check and no image or name leaves the server, and `ANTHROPIC_MODEL` must be a model that
  accepts a forced tool call (a Haiku-class model: the names request and the image request both force the call) and falls back to `claude.DefaultModel`
  (with a warning) when empty or not a valid model name; because the bot reads the standard `ANTHROPIC_API_KEY` name, a
  key exported for another tool on the same host turns the image checks on, and the AI name check too, wherever the AI rule is
  already set to a punishment (the page then also stops saying it is not configured and offers every choice); perf monitoring and
  background stats are on only when `DEBUG=false`; `HTTP_PORT` falls back to `PORT` then 8080. Integer env parsing is
  lenient, so a typo silently becomes the default.

## Testing

- Always pass `-tags testtools`; without it, test files and the helpers in production packages that carry that build tag drop out silently.
- Postgres tests need both `DATABASE_URL` and `FUKU_TEST_DATABASE=true`; otherwise they run on SQLite.
- Use real fixtures: `internal/testdb.Run` (SQLite), miniredis, hand-written `gotgbot.BotClient` fakes. No mock libraries.
- A test never calls `t.Skip` to dodge a case: restructure so the case is not registered (a field on the case table and a
  `continue` in the loop). CI's skip gate (`scripts/check_test_results`) fails any unexpected skip; the only allowed ones are the
  PostgreSQL-only migration tests when no database is configured.
- No test calls a real AI provider (TypeSafe or Anthropic): the Claude client, the raid AI image path and the
  `scripts/claude-spike` tool are tested against `httptest` servers or a fake transport, with no `t.Skip`, so they run
  in `make test`. The live Claude spike (`go run ./scripts/claude-spike`, needs `ANTHROPIC_API_KEY`) is a manual owner
  step, never part of CI. It refuses an `-endpoint` that is not https (plain http is accepted only for `localhost`,
  `127.0.0.1` and `::1`), because the key goes there in a header, and it exits 1 when every request failed.
  The tool has a names mode for the AI spam-name rule: `-names <csv>` (header `label,first_name,last_name,username`, an
  optional `note`, `label` spam or ham, at most 5000 rows; exclusive with `-dir`, `-probe` and `-models-get`) and `-repeat N`
  (1 to 10, only with `-names`; judges every name N times and reports the spread of the probability and how many names
  straddle the bar). The bar is 0.9 in that mode unless `-bar` is given. Each row goes through `namecanon.Of` and
  `claude.Client.ClassifyNames`, the two calls the bot makes, so the spike measures what the bot sends; the summary, sweep,
  latency and cost are the image mode's functions. Names appear only in the owner's CSV (stdout or `-csv`), never on
  stderr, and the key never appears anywhere. A name cell that starts with `=`, `+`, `-`, `@`, a tab or a carriage return is
  written with a single quote in front (`csvSafe`, `TestNamesCSVNeutralisesFormulas`, review IN-03), because the names are
  attacker-controlled and would run as a formula when the file is opened in a spreadsheet; the input file's parsing is
  unchanged. It is tested against `httptest` with no real provider
  (`TestRunNamesMode`, `TestNamesFileParse`, `TestNamesSummaryAtBar`, `TestNamesRepeatStability`,
  `TestNamesModeKeyNeverPrinted`, `TestNamesModeExclusiveFlags`). The tool imports only the `claude` and `namecanon`
  packages of the repository. `.planning/phases/15-ai-spam-name-check/15-SPIKE.md` is the owner's non-blocking checklist
  for the live Haiku spike and the test-group walk-through.
- Assert observable behavior (reply sent, row persisted, cache invalidated, gate enforced). Never assert literals,
  source substrings, or test-double internals.
- The Turnstile page's script test is `node --test scripts/captcha-page/*.test.mjs` (the directory form does not work on
  Node 22). CI runs it in the `test` job (`actions/setup-node` with Node 22, before `make test`); it is not part of
  `make test`, so run it yourself when the page changes. `TestPageLanguagesMatchScript` compares the page's `LANGS` array
  with `captchaweb.PageLanguages`, so the two language lists cannot drift. The page
  (`fuku/utils/captchaweb/page/captcha.html`) is served with a per-response nonce CSP (`captchaweb.CSPFor`) and must
  keep no inline `style` attribute and no inline event handler, because the CSP would block them. `make
  check-translations` cannot see the `captcha_web_` keys the page reads from `captchaweb.PageKeys`;
  `TestCaptchaWebLocaleKeys` (`fuku/i18n`) guards them in all 8 locales (present, not empty, length and no markup), and
  `TestStringTable` guards the table `main` builds from them.
- The settings hub tables (`ChatSettingsAccess`, `ChatAdminIndex`) are in the AutoMigrate list of
  `fuku/modules/test_harness_test.go` and in the testmains of `fuku/db/menuaccess` and `fuku/db/adminindex`, and
  `settingsCleanup` deletes their rows of its chats. The default settings environment (`newSettingsEnvWith` and
  `settingsEnv.addChat`) seeds `admins_allowed` true for its chats through the real `menuaccess` repository, because it
  is an administrator the owner let in; the seed keeps every Phase 9 press test valid once the owner gate exists
  (`TestSettingsEnvSeedsAdminAccess` pins it). A gate test never relies on it: it sets the switch it depends on with
  `settingsEnv.setAdminsAllowed`. The other fixtures are `newOwner` (a live creator who is also the fake's creator entry,
  with the default admin listed), `listAdmin` and the fake's `listAdministrator` (an administrator appears in
  `getChatAdministrators` only when a test opts in), `setDeletedCreator` (the creator entry reads "Deleted Account" with no
  last name and no username), `clearCreator` (no creator entry and no live creator) and `otherReplica` (a second dispatcher
  and bot fake over the same database and miniredis, with update IDs from 50000 so they never meet the base environment's).
- The harness `AutoMigrate` lists (`fuku/modules/test_harness_test.go`, `fuku/db/staff/testmain_test.go`,
  `fuku/db/testmain_test.go`) include the staff audit models, and `staffCleanup` deletes audit rows. The lockdown
  models are in `fuku/modules/test_harness_test.go`, `fuku/db/testmain_test.go` and
  `fuku/db/lockdown/testmain_test.go` (not in the staff package's list), and `lockdownCleanup` deletes lockdown rows.
  The Phase 5 models (`ChatLockdownAlert`, `ChatLockdownBurst`, `ChatLockdownBanRun`) are in
  `fuku/modules/test_harness_test.go` and `fuku/db/lockdown/testmain_test.go` only; the `fuku/db` package's own
  tests never touch them. The Phase 6 model `ChatRaidDetection` is in `fuku/modules/test_harness_test.go`,
  `fuku/db/raiddetection/testmain_test.go` and `fuku/db/devs/testmain_test.go` (the developer `/stats` reads it), and
  `raidCleanup` in the modules tests deletes its rows and their cached copies. A fixture that creates
  `ChatRaidDetection` rows directly must set `AIThreshold` (3..500): the `CHECK` applies on SQLite too, because GORM
  builds it from the model tag (`seedRaidSettings` fills the AI defaults when it is left at 0). The Phase 12 model
  `GreetedMember` is in the AutoMigrate lists of `fuku/modules/test_harness_test.go`, `fuku/db/greetings/testmain_test.go`
  and `fuku/db/backup/testmain_test.go`. The Phase 12 characterization tests record what the code did before the phase and
  must never be re-recorded to make a change pass: `TestRulesLegacyRowSendsTodaysRequest` (a Rules row with no media sends
  today's request) and `TestLegacyGreetingsRequestsUnchanged` (an old greetings row sends today's request), both in
  `rules_legacy_request_test.go`. The other Phase 12 families are `TestGreetOnce*` (`greet_once_test.go`),
  `TestGreetingsMediaBelow*` (`greetings_media_below_test.go`), `TestRules*` in `rules_media_send_test.go`,
  `TestSettingsEditMediaBelow*`, `TestSettings*MediaBelow`, `TestSettings*Once`, `TestSettingsEditSwitchGrid` and
  `TestSettingsEditRules*` (`settings_edit_switch_test.go`, `settings_edit_rules_test.go`, `settings_input_rules_test.go`),
  `TestSettingsRulesCaptionParity` and `TestSettingsControlValidate` (`settings_control_validate_test.go`). A test that
  drives a press, a door or a join with a user ID must delete its `users` rows and cache entries, and its greetings, rules
  and `greeted_members` rows, in `t.Cleanup` and use a fresh chat ID, because CI runs all of `fuku/modules` in one
  PostgreSQL database in source order.
- `make test-postgres-integrity` is the PostgreSQL run in CI. Its `-run` list holds the guarantees SQLite cannot
  prove: `TestStartLockdownOneActivePerChat`, `TestBeginAlert` (the alert is decided exactly once) and
  `TestBanRunStart` (one running ban run per lockdown, the partial unique index), plus the Phase 5 conditional updates:
  `TestRecordLinkRevoked` (one revoke wins), `TestReminderSlots` (`PlanReminderSlots`, one reminder row per
  destination through the slot unique index and `ON CONFLICT DO NOTHING`), `TestBanRunStart` and `TestUnqueueBanRun`
  (`FinishBanRun`'s `NOT EXISTS` guard and its single finish), `TestAlertSync` and
  `TestAlertSyncKeepsAMarkSetDuringTheEdit` (`RecordAlertSynced`'s parameterised `CASE`), `TestAlertClose`
  (`ClaimAlertClose`'s `CASE` over the old row) and `TestBanRunReport` (one report claim wins, steps settled
  separately). The Phase 6 guarantees are `TestRaidDetectionSettings` (`./fuku/db/raiddetection`: the `CHECK` 3..500, the AI
  column's `chk_chat_raid_detection_ai` included, and the column-targeted upserts that never clobber each other) and `TestRaidDetectionCarryOver`
  (`./fuku/db/migrations`: the clamped carry-over of `antiraid_settings.auto_antiraid_threshold`, which executes
  migration 20261018130000 against the schema the migration-chain step already built, so it must run after that step).
  The Phase 8 guarantee is `TestCaptchaModeTurnstileConstraint` (`./fuku/db/migrations`: the widened check accepts
  `turnstile`, `math` and `text` and refuses the rest with `check_violation`, exactly one `captcha_mode` check is left
  on `captcha_settings`, and the migration's statements run twice without error; `TestCaptchaModeMigrationStatements`
  is its database-free half and runs in `make test`).
  The Phase 10 guarantees are `TestAdminIndexReplaceConcurrent` (`./fuku/db/adminindex`: concurrent replaces of one chat
  leave one roster, with the role CHECK and the unique key intact) and `TestMenuAccessUpsert` (`./fuku/db/menuaccess`: a
  Go `false` is written, one row per chat).
  The Phase 12 guarantees are `TestGreetedMemberClaimConcurrent` (`./fuku/db/greetings`: of many concurrent claims of one
  `(chat_id, user_id)` exactly one wins, the primary key; it also runs on SQLite in `make test`) and
  `TestRulesMediaMigrationDefaults` (`./fuku/db/migrations`: the Phase 12 columns take their zero defaults, the migration's
  statements run twice without error and `greeted_members` holds one row per member; PostgreSQL only, it skips locally) and
  `TestImportGreetingsOmittedBlocks` (`./fuku/db/backup`: a greetings import whose payload omits `welcome_settings` or
  `goodbye_settings` imports and writes the three NOT NULL switch columns as false, because `importGreetings` fills a missing
  block with an empty one; without it the nil embedded block inserts NULL and PostgreSQL answers SQLSTATE 23502).
  The Phase 13 guarantees are `TestChatRequirementsSettings` (`./fuku/db/memberreq`: the action CHECKs refuse 5 and -1
  with SQLSTATE 23514, column-targeted upserts of two rules at once never clobber each other, a Go 0 and `false` are
  written), `TestRequirementHoldClaimConcurrent` (of 16 concurrent claims of one `(chat_id, user_id)` exactly one wins, and of
  16 concurrent reclaims of one stale pending hold exactly one wins) and `TestRequirementHolds` (the take, restore, clean and
  clear semantics); all three also run on SQLite in `make test`, and `fuku/db/memberreq/testmain_test.go` migrates both models.
  `TestChatRequirementsSettings` covers every rule column (the photo and channel since Phase 14, the AI rule since Phase 15:
  eight in all, CHECKs refusing 5 and -1 on every rule's column, writes of different rules never clobbering each other, an
  old row or previous-build cached value reading the new rules off). The Phase 14 guarantee is `TestMemberRequirementLookupMigrationDefaults`
  (`./fuku/db/migrations`: the migration's statements run twice without error, a row with only `chat_id` reads both new
  columns 0, both named constraints exist in `pg_constraint`, a direct update to 5 fails with SQLSTATE 23514 naming the
  constraint; PostgreSQL only, it skips locally; `TestMemberRequirementLookupMigrationStatements` is its database-free half
  and runs in `make test`).
  The Phase 15 guarantee is `TestMemberRequirementAIMigrationDefaults` (`./fuku/db/migrations`: the statement runs twice
  without error, a row with only `chat_id` reads the new column 0, the named constraint `chk_chat_requirements_ai` exists, a
  direct update to 5 fails with SQLSTATE 23514 naming it; PostgreSQL only, it skips locally;
  `TestMemberRequirementAIMigrationStatements` is its database-free half and runs in `make test`), and
  `TestChatRequirementsSettings` now covers eight columns (the AI rule included).
  The Phase 16 guarantees are `TestScheduleKeepsEarliest` (`./fuku/db/deletion`: scheduling one `(chat, message)` again keeps
  the earliest `delete_at` with the table-qualified `LEAST`, also under concurrent callers) and `TestClaimBatchOneWinner`
  (`./fuku/db/deletion`: eight concurrent `ClaimDue` callers each win a disjoint set of rows and drain the queue exactly
  once), both of which also run on SQLite in `make test`, and `TestScheduledDeletionMigrationDefaults`
  (`./fuku/db/migrations`: the statements run twice without error, a row with only its required columns reads pending, an
  empty token, 0 attempts and no claim time, the named constraints refuse a delay of 86401, a duplicate `(chat, message)`, a
  state of `x` and negative attempts, and the due index keeps its pending predicate; PostgreSQL only, it skips locally;
  `TestScheduledDeletionMigrationStatements` is its database-free half and runs in `make test`).
  The review fixes added three more names to the list, all in `./fuku/db/deletion` and all also run on SQLite in `make test`:
  `TestReleaseStaleClaims` (a counted release sets `delete_at` to the caller's retry time and drops the row at its third
  counted failure, an uncounted one leaves `delete_at` alone), `TestDeleteRules` (`RemoveRule` drops that exact kind's pending
  rows in the same transaction) and `TestApplyJoinLeftRule` (Keep and `/cleanservice off` drop the pending joined and left rows,
  by exact kind and never an acting row, another kind or another chat).
  Phase 17 adds four names, all in `./fuku/db/deletion` and all also run on SQLite in `make test`:
  `TestScheduleBatchKeepsEarliest` (a batch keeps the earliest time under concurrent callers, and never rewrites kind or
  sent time), `TestScheduleBatchDuplicateKey` (one call naming one key several times writes one row, the Go dedupe PostgreSQL
  needs), `TestCountPending` (the bounded exact count) and `TestDropPendingMessage` (one pending row by chat, message and kind).
  The pattern is anchored with `$`, so list the real top-level test function names and add the test's package to the
  target. Add a test there when a new guarantee rests on a partial unique index, a `CHECK` or a conditional update.
- In CI, keep the migration-chain step before `make test`; its `schema_migrations` rows back the checksum test.
- The settings menu tests share one toolkit, `fuku/modules/settings_env_test.go` (`newSettingsEnv`, `newSettingsEnvWith`
  with `RealLocale`, `openMenu`, `press`, `pressRaw`, `pressPrivate`, `pressNoMessage`, `sendIn`, `sendPrivate`, `edits`,
  `lastEdit`, `answerCount`, `settingsButtonByMarker`, `settingsButtonFields`): page tests call it by name and do not edit
  it. The message editor adds `fuku/modules/settings_edit_env_test.go` on top of it (`newSettingsEditEnv`, `editCleanup`,
  `settingsEditAdminJSON`, `listEditAdmin`, `settingsEditStored`, `settingsEditRulesStored`, `openEditPage`,
  `openEditPagePrivate`) and its families `TestSettingsEdit*`, `TestSettingsParityGreeting*`, `TestSettingsDriftGreetings`
  and `TestSettingsAuthorityParityGreetings` (`settings_edit_test.go`), plus `TestSettingsEditSet*`,
  `TestSettingsEditCancel*`, `TestSettingsEditRemove*` and `TestSettingsAuthorityParityEdit` (`settings_edit_actions_test.go`:
  the wait, Remove, their parity with the commands and the rights). The typed-reply door adds `TestSettingsEditTyped*`,
  `TestSettingsEditDoor*` and `TestSettingsEditAlbum*` (`settings_input_door_test.go`, `settings_input_media_test.go`:
  the door's order, the album gate, the media and buttons parts and `TestSettingsEditWholeMessageParity`), the previews
  `TestSettingsEditPreview*` and `TestSettingsEditFullPreview` (`settings_edit_preview_test.go`), the pure check
  `TestSettingsEditCheck*` (`settings_edit_check_test.go`), the store `settings_input_test.go` and the switch grid with
  Greet once (`settings_edit_switch_test.go`: `TestSettingsEditOnceSwitch`, `TestSettingsEditSwitchGrid`,
  `TestSettingsDriftOnce` and `TestSettingsAuthorityOnce`, which decides over the four owner worlds exactly as the press
  door of `TestSettingsGateAllDoors`; `editCleanup` also deletes the chats' `greeted_members` rows), and Media under text
  on Welcome and Goodbye in the same file (`TestSettingsEditMediaBelowSwitch`, `TestSettingsEditMediaBelowEligibility`,
  `TestSettingsEditMediaBelowStalePress`, `TestSettingsEditMediaBelowStoredOnIgnored`, `TestSettingsParityMediaBelow` (the
  press and `applyWelcomeMediaBelow` leave the same row and cache state, there being no command to compare with),
  `TestSettingsDriftMediaBelow` and `TestSettingsAuthorityMediaBelow`). The test files are
  `settings_edit_*_test.go` and `settings_input*_test.go`; the typed reply is the sixth door of `TestSettingsGateAllDoors`.
  `TestSettingsEditScreensFitEveryLocale` (`settings_edit_screens_test.go`, whose `settingsEditScreens` builds every
  non-page editor screen and is also what `TestSettingsStaffPrivacy` renders) holds them to 3800 UTF-16 units and 64-byte
  buttons with a 64-rune title. `RealLocale` installs the real locale files through `i18n.OverrideManagerLanguagesForTest`, because the test binary
  has no locale manager of its own. `fuku/i18n/settings_locale_test.go` keeps a limits table that must cover exactly the
  `settings_` keys of the locale files, so a plan that adds a key adds its row.
- Run the settings tests with `go test -tags testtools -race -count=1 -run '^(TestSettings|TestLoadSettings)' ./fuku/modules ./fuku/i18n`.
  Page tests add page-prefixed helpers in their own files (`settings_<page>_test.go`). Every setting the menu changes
  has a parity test (`TestSettingsParity*`: the menu press and the command leave the same stored row and the same
  cache), a drift test (`TestSettingsDrift*`: the page reads what the command reads) and an authority-parity test
  (`TestSettingsAuthorityParity*`: the menu refuses what the command refuses, with the right the control declares),
  and every page has a worst-case callback test (`TestSettings*CallbackBytes`, 64 bytes) and a length test in all 8
  languages (`TestSettings*PageFitsEveryLocale`); a new setting in the menu adds all of them. `TestSettingsStaffPrivacy`
  guards D-21 and D-32 (a group menu, and a private menu of an admin who is not the creator, never name the Staff Group), `TestSettingsHelpFitsOneMessage` holds `settings_help_msg`
  to one Telegram message in every language, and `TestLoadSettingsRegistersHelp` guards the `/help` entry. No settings
  guarantee rests on PostgreSQL (no migration, no partial index, no `CHECK`), so nothing of the menu joins
  `make test-postgres-integrity`. The hub, the owner gate and the Main grid have their own families:
  `TestSettingsHub*` (list, paging, pick, Groups), `TestSettingsAccess*`, `TestSettingsGate*`, `TestSettingsOwnerState*`,
  `TestSettingsMain*` (`TestSettingsMainGrid`, `TestSettingsMainPageFitsEveryLocale`: 3800 UTF-16 units and the button
  limits with a 64-rune title in every language) and `TestAdminIndex*`; `TestSettingsLabelLengths`
  (`fuku/i18n/settings_locale_test.go`) holds the two-column limits (a grid button at most 20 code points, the Groups
  button 18) in code points. The Access controls have no command, so they carry `TestSettingsDriftAccess` and
  `TestSettingsOwnerAuthorityAccess` instead of a parity test, and `TestSettingsAccessAcrossReplicas` proves the fresh
  read across two replicas (a flip on one is seen by the very next press on the other, a memoised owner never hides a
  flip to ON, D-12).
  The Member requirements page has `TestSettingsMreq*` (`TestSettingsMreqOpensFromMain`, `TestSettingsMreqMainState`,
  `TestSettingsMreqSwitches`, `TestSettingsMreqRuleScreen`, `TestSettingsControlActionAllowList`,
  `TestSettingsMreqRowBudget`, `TestSettingsMreqPageFitsEveryLocale`, `TestSettingsMreqCallbackBytes`),
  `TestApplyMemberReqAction`, `TestSettingsDriftMreq` and `TestSettingsAuthorityMreq` (`settings_mreq_test.go`) and
  `TestSettingsMreqCard` (`settings_mreq_card_test.go`: the card's four exits, the demotion between the taps, the stale
  cache and the forged `my` fields; the AI rule's card expects its own consequence keys). Phase 15's page families are
  `TestSettingsMreqAIStatus` and `TestSettingsMreqAIGate` (`settings_mreq_ai_test.go`), with the helpers
  `withAINotConfigured(t, how)` (no key, or the switch off) and `withAIConfigured(t)`, which put `config.AppConfig` back in
  `t.Cleanup`: the Phase 13 and 14 page tests that open the AI screen and expect its usual shape (`TestSettingsMreqRuleScreen`,
  `TestSettingsMreqCard`, the basic-group screens) say `withAIConfigured` because the test process has no key. The module harness carries both Phase
  13 tables (`ChatRequirements`, `ChatRequirementHold` in the `AutoMigrate` list of `test_harness_test.go`), and
  `settingsCleanup` deletes their rows of its chats and the `member_requirements` cache key.
  The watcher families are `TestMemberReqWatcher*`, `TestMemberReqPassCache`, `TestMemberReqCleanEndsWarnHold`,
  `TestMemberReqOnePunishmentAcrossReplicas` (a mute at once across three processes; the warn variant judges its first
  message alone, because on the SQLite of this suite a concurrent writer makes the warn module's read-then-write transaction
  fail with "database is locked" where PostgreSQL waits), `TestMemberReqRunnerSheds` and `TestShutdownWaitsForMemberReqJobs`
  (`memberreq_watch_test.go`, `shutdown_drains_test.go`). `newMemberReqEnv` turns the enforcement switch on through
  `withMemberReqLive`, which restores the previous value in `t.Cleanup`; a test that uses it never calls `t.Parallel`. A test
  that calls `StopMemberReqJobs` restores the runner with `restoreMemberReqRunner` (the drain sets the draining flag and
  cancels the root context for good), and `memberReqCleanup` deletes the `fuku:memberreq:*` keys of its chats, the pass keys
  included. `mreqReplica` builds a second bot process over the same database and Redis, `mreqSend` and `mreqSendAt` run a
  message through the dispatcher (the settings environment's `send` stamps 1970), and the test environment waits for
  `memberReqJobs` before its cleanups run. The delete and cap families are `TestMemberReqDeleteMessages`,
  `TestMemberReqDeleteDuringPause`, `TestMemberReqCapPausesPunishments` and `TestMemberReqCapWithoutRedis`
  (`memberreq_delete_test.go`, `memberreq_limits_test.go`): they prime the admin cache with `mreqAdminList` or empty it with
  `mreqNoAdminList`, simulate a promotion with `mreqPromote` (the fake's administrator list plus
  `cache.InvalidateAdminCache`, as the promotion's own update does), drop the chat's admin cache entry in `t.Cleanup`, stop
  the minute clock with `mreqFixedClock` (a swap of `raidNow`) so a window never straddles a minute, and restore
  `memberReqCapPerMinute` and `memberReqRedisClient` with `mreqSwap`.
  The review fix pass added `TestMemberReqMuteNeverReplacesRestriction` and `TestMemberReqJoinNeverReplacesRestriction`
  (CR-01, `memberreq_enforce_test.go`, `memberreq_join_test.go`; `mreqRestrictedBySomeoneElse` sets a restricted member
  record), `TestMemberReqEnforceReadsSettingsAfterLookup` and `TestMemberReqJoinReadsSettingsAgain` (WR-02: a case built
  from an old snapshot, then the stored row changed, or `memberReqSettingsLookup` swapped), `TestMemberReqWarnRecovery`
  (WR-01: `memberReqMarkHold` and `memberReqMarkRetryWait` swapped, the Redis note, `memberReqApplyWarn` wrapped to return
  the reset's error) and `TestMemberReqRejoinResetsPendingMuteHold` (WR-03, `mreqPendingMuteHold`).
  The Phase 13 families in one place. Every `TestMemberReq*` test of `fuku/modules` lives in a `memberreq_*_test.go` file
  (rules and corpus, the executor, the watcher, the join and the captcha guard, the rejoin reset, the notices, the limits,
  the deletes, the failures and the "I fixed it" button: `TestMemberReqFixedUnmutes`, `TestMemberReqFixedRefusals`,
  `TestMemberReqFixedNeverLifts`, `TestMemberReqFixedDuringLockdown`, `TestMemberReqFixedHeldWelcome`,
  `TestMemberReqEnforcementLive`); `TestMemberReqLocaleKeys` is in `fuku/i18n`; `TestApplyWarn` (`warns_apply_test.go`) is the
  warn extraction's own test; `TestChatRequirementsSettings`, `TestRequirementHoldClaimConcurrent` and `TestRequirementHolds`
  are in `fuku/db/memberreq`. The toolkit is `fuku/modules/memberreq_env_test.go`, the `memberReqEnv` type on top of the settings environment:
  `newMemberReqEnv` (and `newMemberReqEnvWith`), `withMemberReqLive`, `mreqMember` (a member with the names a test asks for),
  `mreqSend`, `mreqReplica`, `mreqPressFixed` and `memberReqCleanup`. Its hygiene rule, which CI needs because all of
  `fuku/modules` runs in one PostgreSQL database in source order: every member comes from `uniqueLinkOwnerID()` and every
  chat from `uniqueModuleChatID()`, and `t.Cleanup` deletes the members' `users` rows and `user` cache keys, the rows of
  both Phase 13 tables, their warns, the `member_requirements` settings key, the approvals keys and the `fuku:memberreq:*`
  keys, then calls `ResetLocalForTest`. The size and privacy sweep is `TestMemberReqScreensFitEveryLocale` (every notice,
  every "I fixed it" answer at 200 code points and the two notices that button edits in, in all 8 languages, none naming the
  chat, the Staff Group or the admin who set the rule) and the Phase 13 part of `TestSettingsStaffPrivacy` (page `m`, eight rule
  screens and twenty-four cards in every language, and a private run by an administrator who is not the creator through the page,
  a rule screen, a Mute card and its Yes and both switches). `TestMemberRequirementsStayOutOfBackup` (`fuku/db/backup`,
  SQLite, so not in the PostgreSQL list) proves that neither `chat_requirements` nor `chat_requirement_holds` is exported,
  imported or reset, and since Phase 14 that the export file names no requirement column or table and a reset leaves the
  photo and channel columns as they were.
- Phase 14 (the photo and channel rules) test families, all in `fuku/modules` unless a package is named, each in a file of its
  own prefix: `TestMemberReqPhotoRule`, `TestMemberReqLinkedChannel`, `TestMemberReqChannelRule`,
  `TestMemberReqChannelBreachNeedsBotAdmin`, `TestMemberReqBotStatusClassifier` and the `TestMemberReqLookup*` family
  (`memberreq_lookup_test.go`, `memberreq_lookup_enforce_test.go`: the flights, the pacer, the cache keys, punishments,
  unanswerable shapes, order, the cap and one punishment across replicas); the watcher's `TestMemberReqWatcherLookup*` and
  `TestMemberReqPassCacheWithLookups` and the held path's `TestMemberReqLookupHeldPath` and `TestMemberReqHeldRelease*`;
  the notices' `TestMemberReqNoticePhotoRemedy` and `TestMemberReqStillBrokenText`; "I fixed it" with lookups
  `TestMemberReqFixedApproved`, `TestMemberReqFixedLookups` and `TestMemberReqChannelInert`; the page's
  `TestSettingsMreqChannelStatus`, `TestSettingsMreqOpenCost`, `TestSettingsMreqBasicGroup`,
  `TestSettingsMreqChannelRefusal` and `TestSettingsMreqForgedFields`; the shutdown's `TestShutdownEndsMemberReqLookups`; the
  key hygiene `TestMemberReqNoProfileInReworkedKeys`; and the cross-cutting three of plan 14-11
  (`memberreq_lookup_budget_test.go`): `TestMemberReqPacersAreIndependent` (a 30 s block on one pacer's keys sheds the other
  entries' lookups or leaves them alone at all four entries: the job, the join entry, "I fixed it" and the page, and a lookup
  block delays no notice or removal), `TestMemberReqLookupLogsCarryIDsOnly` (a logrus hook at debug level over every failure
  shape of the photo, channel, bot status, held release, status reader and gate paths, plus a real `gotgbot.BaseBotClient`
  over a failing transport and over a server that refuses with a description: no file ID, name, title, description, notice
  text or bot credential in any line, and a recovered flight panic is one line naming no ID) and
  `TestMemberReqLookupAlertsFitEveryLocale` (the refused-press alert, built by `settingsMreqNoLinkAlert` as the gate sends it,
  and `mreq_fixed_check_failed`, at most 200 code points in 8 languages, naming no ID, Staff Group or member). The closing
  plan 14-12 adds `TestMemberReqLookupEndToEnd` (`memberreq_lookup_e2e_test.go`): the owner's page press and card Yes, the real
  dispatcher, the executor and the "I fixed it" button for both rules, including the cases that must punish nobody (the bot not
  an administrator, the group unlinked); the group's `getChat` answered by the lookup fake carries no `permissions`, so a channel
  unmute is asserted against `defaultUnmutePermissions()` and a photo one, whose group `getChat` the base fake answers, against
  `lockdownSnapshotPermissions`.
  `TestSettingsStaffPrivacy` also renders the page and every rule screen under every channel status (unread, ok, notadmin,
  nolink, unknown) and in a basic group, Main with every rule on and both alerts, and runs a private and a group-menu Phase 14
  run of an administrator who is not the creator over a linked channel whose title and ID the fake knows and a held member
  whose name it knows; none may name the Staff Group, the channel, the member or a count. The lookup toolkit is
  `fuku/modules/memberreq_lookup_fake_test.go`: `mreqLookupFake` wraps the shared lockdown fake and answers
  `getUserProfilePhotos`, the group's `getChat` (its linked channel) and `getChatMember` of a channel from an
  `mreqLookupState` (scripted errors, holds, null bodies, a photo file ID, a channel title), `mreqInstallLookupFake` installs
  it, `mreqChannel` links a fresh channel and registers its ID (`settingsMreqLinkChannel` does it for a plain settings
  environment), `withFastLookupPacer` swaps in a 1 ms lookup pacer and `mreqLookupCleanup` deletes what a lookup
  test wrote. Its hygiene: fresh IDs from `uniqueLinkOwnerID()` and `uniqueModuleChatID()`; a channel is registered as a chat
  ID with `memberReqCleanup` (`mreqChannel` does it) so its `sub:`, `lc:` and `ba:` keys go, and `fuku:memberreq:photo:<user>`
  is deleted by `mreqLookupCleanup`; the singleflight group and the lookup pacer are package globals, so a test that swaps
  one restores it in `t.Cleanup` (`withFastLookupPacer`, `mreqSwap`); a flight a test holds open with `hold` or `holdKey` is
  released in `t.Cleanup`; a test that calls `StopMemberReqJobs` also calls `restoreMemberReqRunner`; a test that runs many
  goroutines uses Mute or Kick and never Warn (the warn module's SQLite transaction); a test that joins, rejoins or sends with
  a date takes the date before the message and at least 2 s apart; and a test never `t.Skip`s (CI's
  `scripts/check_test_results` fails on an unexpected skip, so a case that cannot run in a menu kind is not registered
  there). The locale files now hold 330 `settings_` keys and 31 `mreq_` keys, each `settings_` key one row of the limits table
  of `fuku/i18n/settings_locale_test.go` and each `mreq_` key one row of `memberReqLocaleLimits`. Phase 16 added 57
  `settings_` keys (rows 1 to 57 of the Deletion page: `settings_page_del`, `settings_state_del_on` and 55 `settings_del_`
  keys), each one row of `settingsLocaleLimits`, with label-length groups in `TestSettingsLabelLengths` for the page name,
  the two section names, the twelve service and five bot names (rendered with each of the three state icons, at most 20)
  and the choice labels Keep, Delete now and the six delays, and `settings_del_hint_punish_auto` in `memberReqQuotingKeys`
  (it quotes the words of `mreq_btn_fixed`). The names, covers and hints of the rows of `deletionServiceRows` and
  `deletionBotRows` are read through the table fields `NameKey`, `DescKey` and `HintKey`, which `make check-translations`
  cannot see: `TestDeletionKindKeysExist` (`fuku/modules`, `deletion_kinds_locale_test.go`) proves every one of them, the two
  section names and the delay words translate in all 8 languages. Phase 17 added 18 `settings_` keys (the Member messages
  section: its name and list intro, three row names, three descriptions, four hints, two state words, three delay labels
  and the card `settings_del_confirm_self`) and changed the text of `settings_del_intro`, each new key with a row of
  `settingsLocaleLimits`, the section and row names (rendered with each state icon and the section's 💬, at most 20) and
  the choice labels On, 30 s, 5 min and 15 min in `TestSettingsLabelLengths`; the member rows' keys are read through table
  fields that `make check-translations` cannot see, so `TestDeletionKindKeysExist` (extended in plan 17-09) and the limits
  table are their guard. Phase 15's ten keys
  (eight `settings_`, two `mreq_`) were added before the table row that reads them: `mreq_rule_ai` is read
  through the table field `PhraseKey` and `mreq_remedy_ai` by the notice builder through a key held in a variable, which
  `make check-translations` cannot see, so the two limits tables and `TestMemberReqLocaleKeys` are their only guard. The 520 code point cap of the notices is a rendered
  check over the real translations (eight phrases, 66 code point name, the photo tail, the AI tail and both), not a sum
  of the ceilings, which at 636 no longer proves it. The two PostgreSQL facts of Phase 14 are `TestMemberRequirementLookupMigrationDefaults`
  (in the list above) and that the `fuku/modules` harness does not run on PostgreSQL (its AutoMigrate fails on
  `uni_users_user_id` against the migration-built schema), so Phase 14's module tests run on SQLite.
- Phase 15 (the AI spam-name rule) test families, all in `fuku/modules` unless a package is named. The toolkit is
  `memberreq_ai_fake_test.go` on top of the Phase 13 and 14 ones: `mreqInstallAIFake` starts the Anthropic stand-in (an
  `httptest` server driven by an `mreqAIState`: scripted steps `mreqAIProbability`, `mreqAIStatus`, `mreqAIHang`,
  `mreqAIGated`, `mreqAIDelayed`, `mreqAIDrop`, the recorded request bodies and the count) and points `memberReqAIEndpoint`
  and `memberReqAIHTTP` at it; `withAIConfigured` and `withAINotConfigured` set `config.AppConfig` and install a fresh
  breaker; `withAIGate` and `withAIBudgets` resize the gate and the three hourly budgets; `mreqAIMarkNew` records a member as
  joining the way the join entry does; `mreqAICleanup` deletes the verdict, baseline, marker and budget keys and installs an empty refusal latch (`memberReqAILatches`, restored by `mreqSwap`), so a test that latches a chat never leaves it latched for another; `mreqAIUnique`
  is a counter that names a member without the member's ID digits (the sweeps give a member `mreqAINameSentinel` plus the
  counter, so an ID and a name can each be looked for in a body or a log line), and `mreqAIStaticForbidden` with
  `mreqAIForbiddenOf` is the list of what no log line of an AI path may hold. Hygiene: fresh IDs from `uniqueLinkOwnerID()`
  and `uniqueModuleChatID()`; names that carry the member's ID (`mreqAIMember`) or a counter, so no two tests share a verdict
  key; `mreqAICleanup` in every test that judges; every swapped variable (the gate, the budgets, the endpoint, the HTTP
  client, `memberReqAICall`, the breaker, the latch, the join and flight budgets) restored in `t.Cleanup` through `mreqSwap`; a test that
  calls `StopMemberReqJobs` calls `restoreMemberReqRunner`; no real provider, no key, no `t.Skip` and no `t.Parallel`, and a
  test that runs many goroutines uses Mute or Kick and never Warn. The families by name: `TestMemberReqAIRule`,
  `TestMemberReqAIEnforcePunishes`, `TestMemberReqAINotConfigured`, `TestMemberReqAISparesExempt`,
  `TestMemberReqAILookupOrder`, `TestMemberReqAIUnanswerable`, `TestMemberReqAIBaseline`, `TestMemberReqAINewMember`,
  `TestMemberReqAIRename`, `TestMemberReqAICheckOnJoinOff`, `TestMemberReqAIMarkerSurvivesLongHold`,
  `TestMemberReqAIBudget`, `TestMemberReqAIGate`, `TestMemberReqAIFlight`, `TestMemberReqAIBreaker`, `TestMemberReqAILatch`,
  `TestMemberReqWatcherAILatch`, `TestMemberReqAIEstablishedMemberIgnoresCachedBreach`,
  `TestMemberReqAIUnknownDefers`, `TestMemberReqAINamesChangedDuringLookup`, `TestMemberReqAIVerdictCache`,
  `TestMemberReqWatcherAI`, `TestMemberReqWatcherAIBurst`, `TestMemberReqPassCacheWithAI`, `TestMemberReqHeldReleaseAI`,
  `TestMemberReqNoticeAIRemedy`, `TestMemberReqFixedAI`, `TestSettingsMreqAIStatus` and `TestSettingsMreqAIGate`; the
  cross-cutting sweeps of plan 15-10 are `TestShutdownEndsMemberReqAI` (the drain, above), `TestMemberReqAIJoinBudget` (the
  join returns at `memberReqLookupJoinBudget` while the flight finishes and caches, the first message then judges from the
  cache, a photo lookup that spends the budget leaves the AI row unasked, a hung job never delays the update),
  `TestMemberReqAIRequestIsMinimal` (the body is exactly the fixed shape, the three canonical names and
  `claude.NamesInstruction`: no title, message, chat or user ID, no field beyond the shape, the key in its header alone),
  `TestMemberReqAIPacersAreIndependent` (a block on the lookup pacer's keys leaves the AI call alone, a burst of name checks
  writes no key of `fuku:memberreq:pace:*` or `fuku:memberreq:lkp:pace:*`, a saturated gate delays no name-rule punishment
  or notice), and the extended `TestMemberReqLookupLogsCarryIDsOnly` (every provider and guard failure shape on the job,
  join, held release and button paths, a transport error and a recovered flight panic: no name, names hash, verdict key,
  probability, instruction fragment or key in any line, and each expected status or error type must appear) and
  `TestMemberReqLookupAlertsFitEveryLocale` (the refused punishing AI press built with `settingsMreqUnconfiguredText`, at
  most 200 code points in 8 languages). The closing plan 15-11 adds `TestMemberReqAIEndToEnd` (`memberreq_ai_e2e_test.go`), the
  vertical slice through the real dispatcher with only Telegram and the provider faked: the owner sets the AI rule to Mute
  from the page (the AI cell, Mute, the card's Yes), a joiner rated 0.97 is muted once with one notice that ends with the AI
  remedy and carries the button (a second delivery of the join punishes nobody again), the member renames and taps the
  button and is freed by one provider call for the new string, an established member is recorded with no call and judged
  when they rename (a Warn with the remedy, released by a rename back), approved members and administrators are never
  judged, the AI not configured (no key, or `ENABLE_AISPAM` off) shows the page's AI line and the Off-only screen, refuses
  every punishing press and a forged Yes and welcomes joiners with no request, and a rejected key punishes nobody, pauses
  every group's checks until the clock passes the pause and a 500 at join leaves the marker so the first message is judged.
  The global log redactor (`logredact`,
  installed by `fuku/config`'s `init`, so before a test hook) already replaces an Anthropic key in a line before the sweep
  sees it, which means the sweep cannot catch a key leak on its own (checked by mutation): the key is held by that
  redactor and `memberReqAILogFailure`'s own scrub, while the names and hashes have no such net and are held by the
  sweep alone.
- Phase 16 plan 09 (the closing plan) adds `fuku/modules/deletion_e2e_test.go`: `TestDeletionEndToEnd` (the vertical slice
  through the real dispatcher with only Telegram faked: the owner's page presses, a title notice deleted now, a pin notice
  queued at its date plus N and deleted by `runDeletionCycle` only when `deletionNow` reaches it, the welcome, a manual `/ban`
  reply, a requirement Mute notice at the 24 hour cap moved to the tap plus the timer by the button, and a Warn notice, the
  joined and left coupling over `/cleanservice`, a bot that cannot delete, a Staff Group, a restart, two replicas and a 48 hour
  old row), `TestDeletionExcludedMessagesNeverScheduled` (a Staff Group card and summary, a lockdown alert copy, a captcha
  challenge and a log post, each through its real sender with every rule at its shortest delay: no row and no delete request,
  also after a worker cycle a day and a half later) and `TestDeletionLogsCarryIDsOnly` (a logrus hook over every failing seam
  and worker failure shape: no text, claim token or message ID). Its toolkit is `deletionE2E` (`newDeletionE2E`: the settings
  Deletion environment plus the member requirements toolkit plus a settable `deletionNow`; `deliver`, `command`, `pass`,
  `deleted`, `wantRow`, `replicaBot`) and `deletionShortestRules`. `TestSettingsStaffPrivacy` also renders the Deletion hub, both
  lists and all 17 row screens under every right line, with and without rules and with a failed write in 8 languages, and runs
  a private and a group menu of an administrator who is not the creator through the page (`settingsPrivacyPhase16Run`).
- Phase 16 plan 05 (scheduling the bot's own messages) test families: `TestScheduleBotMessage` and
  `TestScheduleBotMessageClampsDate` (the helper's table: nil, private, channel, no rule, latch, Staff Group, failed read,
  failed write, the date clamp), `TestBotWelcomeScheduled` and `TestBotGoodbyeScheduled` (`deletion_bot_test.go`, through the
  real dispatcher of `newDeletionEnv`), and `TestBotRulesScheduled`, `TestBotRulesPrivateNeverScheduled`,
  `TestBotManualPunishmentReplies`, `TestBotManualRepliesNotCovered` and `TestWarnThisUserSource`
  (`deletion_bot_sites_test.go`, on the `botSite` helper: a scripted `moduleBotClient` whose send answers carry the chat's
  own ID and type and the fixed clock's date, because the helper reads the chat from the answer). The shared staff fake gains
  `staffActionFake.sendDate` (nil means 1, so every earlier test is unchanged); the deletion environment sets it from
  `deletionNow()`, so no queued row lands in 1970.

## Commits

- Use Conventional Commits. The changelog drops `docs:`/`test:`/`chore:`/`ci:`/`deps:`, so user-visible changes need
  `feat:` or `fix:`.
