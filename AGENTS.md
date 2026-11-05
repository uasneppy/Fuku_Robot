# AGENTS.md

Fuku Robot: Telegram group-management bot. Go module `github.com/uasneppy/fuku_robot`; PostgreSQL (GORM) + Redis.
Update this file in the same commit as any change that invalidates a rule below.

## Commands

```bash
make test                 # the only valid full test run (tags, race, coverage, skip gate; 15 min per package, fuku/modules runs 7-11 min)
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

- Group numbers are execution order: `-10` captcha sweeper · `-7` lockdown join guard (it also counts the join surge
  before it reads the lockdown) · `-6` fed-ban · `-4` raid flood counter (counts everything but slash-prefixed texts, never ends an update; there is no
  antiraid handler any more) · `-3` staff watchers (chat-migration re-key; later ownership and bot health) · `-2` admin-cache ·
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
  interceptors; it matches through `handlers.NewCommand`'s own `CheckUpdate`), takes it first, replies and writes nothing,
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
- The `settings|v1` namespace is the settings menu (`settings_menu.go`, `settings_pages.go`). Fields: `a` action, `c` the
  group's chat ID (a menu opened in private), `u` the opener's user ID (a menu opened in the group), `p` page (`s` staff,
  `l` lockdown, `c` captcha, `f` antiflood, `w` welcome, `g` goodbye, `r` rules, `a` access), `t` control, `v` the absolute target value; `e` (confirm expiry), `r`
  (the lockdown row ID of a Lift card), `l` (the link row ID of an Unlink card) and `s` (Staff Group row ID) arrive with
  the confirm cards and the Link pick. Core action codes: `oh` open here (the choice
  message becomes the Main page), `op` open page `p` (also Refresh and a stepper's value button), `bk` Back to Main, `cl`
  Close, `tg` toggle, `st` set, `no` cancel a card back to page `p`; a page declares its own codes through
  `registerSettingsPage`, which panics at init on a duplicate page key or action code. Exactly one of `c` and `u` is
  present: `c` is accepted only on a message in a private chat and `u` only on one in a non-private chat, and each action
  takes an allow-listed field set (an unknown key is refused), all checked before any I/O. A page action may also declare `Validate`, a strict check of its own fields that runs in that same
  step 1, after the allow-list and before any I/O, and whose false answer is `common_callback_invalid_request`; a nil
  `Validate` keeps the fields allowed and not required. A `tg` or `st` press must also name a control its page registers
  (`settingsControlFieldsValid`), and a control may declare `Validate` of `v`; both run in that same step 1, before any
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
  A new page is one `registerSettingsPage` call from its own file's `init` plus its strings, and never edits
  `settings_pages.go`. A private press makes two Telegram calls, `getChatMember` of the presser in `c` and `getChat` of
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
  with Back and Close. Main lists the three pages after Antiflood (`s|l / c|f / w|g / r`, then Access for the owner) with
  `common_status_on` or `common_status_off` for Welcome and Goodbye and `settings_state_set` or `settings_state_not_set`
  for Rules.
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
  kill switch for the whole raid AI path: `raidAITextConfigured` and `raidAIImageConfigured` both require it, the
  watcher enqueues nothing, and a job already queued is dropped by `raidAIProcessJob` / `raidAIProcessImage` without
  a `getFile` or any provider call. The `/lockdown` help says what the AI trigger sends to
  TypeSafe and to Anthropic Claude (`lockdown_help_msg`, all 8 locales, D-15), and its length is held by
  `TestLockdownHelpFitsOneMessage`.
  Anonymous admins always get the proof first and re-enter through `RegisterAnonymousAdminHandler`; the cached command's
  arguments then pick view or change for the tapper. The command is not `Disableable`. `/autoantiraid` is its alias for
  the join threshold (see the retired raid mode under Subsystem traps) with exactly the same checks and anonymous-admin
  re-entry, so neither is ever decided by the cached admin list.

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
  `settings_access_owner_only`; only the Access page's control and action declare it.
  `settingsRightChangeInfo` needs the creator or an administrator with `can_change_info` from the same member record (no
  extra call) and refuses with the alert `settings_no_change_info` (the editor's Delete last, as `/cleanwelcome` and
  `/cleangoodbye` refuse through `CanUserChangeInfo`). `settingsRightRefusalFor` is the one pure judgement of a right's
  member-record part, shared by a button press and a typed reply, so the two can never judge a right differently.

## Data

- Repositories live in `fuku/db/<domain>/`. Read via `cache.GetFromCacheOrLoad` with `cache.CacheKey` keys.
  **Every write must `cache.DeleteCache` the keys it affects.**
- `GetFromCacheOrLoad` has an in-process layer (`CACHE_LOCAL_TTL`, default 10 s) in front of Redis. `DeleteCache`
  evicts it on this replica only; list freshness-critical keys in `skipLocal` (`fuku/db/cache/local.go`). Tests that
  write rows directly must call `cache.ResetLocalForTest()` or `DeleteCache` before reading through the cache.
- Two packages are named `cache`; the loader and generation guards are in `fuku/db/cache`, not `fuku/utils/cache`.
- Operational Redis keys (`fuku:anonAdmin:*`, `fuku:staff:*`, `fuku:lockdown:*`, `fuku:raid:*`) sit outside the
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
- `chat_requirements` (migration 20261106120000) is the member requirements setting, one row per chat: five `SMALLINT`
  action columns (`last_name_action`, `username_action`, `russian_action`, `arabic_action`, `chinese_action`, 0 off, 1 warn,
  2 kick, 3 ban, 4 mute, each under its own named CHECK `chk_chat_requirements_<rule>` for 0..4) and the `check_on_join`
  and `delete_messages` switches. A chat without a row means every rule off and both switches off. It is read through
  `cache.GetFromCacheOrLoad` under `cache.CacheKey("member_requirements", chat)` (`memberreq.GetSettings`; `GetSettingsFresh`
  is the same query without the cache) and creates no row. `memberreq.SetRuleAction`, `SetCheckOnJoin` and
  `SetDeleteMessages` are three column-targeted `INSERT ... ON CONFLICT (chat_id) DO UPDATE` of their own column plus
  `updated_at`, so a Go 0 or `false` is written and two admins changing two rules never clobber each other, each followed by
  `cache.DeleteCache` of that key; a value outside 0..4 or an unknown rule is refused before any write. The model has no
  `default:` tag on its booleans. It is never part of backup, export, import or reset.
- `chat_requirement_holds` (same migration) is one member's requirement episode in one chat, unique on `(chat_id, user_id)`
  (`uk_chat_requirement_holds_chat_user`): both the latch that makes a violation punished once across replicas and the
  record of a mute the bot placed. `memberreq.ClaimHold` is one `INSERT ... ON CONFLICT DO NOTHING` and only the caller whose
  insert landed (`RowsAffected == 1`) acts; a pending hold (not yet confirmed punished) older than `memberreq.StaleClaim`
  (2 minutes) is taken over by `ReclaimStaleHold`, one conditional update with one winner. The other operations (mark
  punished, notice ID, welcome held, backoff, revert to pending, take by ID with `DELETE ... RETURNING`, restore, the clean
  predicate, clear all, take before a join) are fresh database statements too. The hold is read fresh, never cached (so no
  `DeleteCache` applies), never touches Redis and works without it, and is never part of backup, export, import or reset.
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
  call, so a member is claimed exactly where they are really welcomed. A captcha-failed joiner, a joiner the lockdown
  guard records for a ban, a join while the Welcome switch is off, any join while Greet once is off and every preview
  write no row, because none of them reaches the claim (`TestGreetOnceWelcomeOffNoClaim`, `TestGreetOnceOffNoRecord`,
  `TestGreetOnceCaptchaPathAndFailure`, `TestGreetOnceLockdownBanNoRecord`). `TestGreetOnceConcurrentOneWelcome` proves
  one welcome for 16 concurrent joins of one member. Accepted limits: a basic group's migration to a supergroup changes
  the chat ID, so its members are welcomed once more; members who join while the Welcome switch is off are not recorded
  and are welcomed once when it is back on; bots that join are claimed like any member; rows are kept for good. A 429
  counts as a definite refusal (Telegram answered without processing the request), so a member whose first welcome hit a
  flood limit is welcomed on their next join.
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
  names, chat or user data. A verdict of at least `raidAIImageBar` (0.85, a code constant) counts as one spam item for
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
  command, which any cached admin could use. There is no join handler at group `-5` and no expiry poller: nobody is
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
  them. A Staff Group's own page (the read-only panel inside a Staff Group) may show its linked groups to the people of
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
- SQL log lines carry no parameters: the GORM logger in `fuku/db/conn.go` sets `ParameterizedQueries: true`, so a failed or
  slow (over 1 s) statement is logged with its placeholders and never the welcome, goodbye or rules text, button links or
  other values it wrote. Do not add a logger that prints bound values.
- A file the bot downloads itself never puts its URL in an error or a log: the Telegram file URL holds the bot token and
  `net/http` errors embed it. Rebuild a transport error from `url.Error.Err`, report a bad status as the code alone,
  follow no redirect (`CheckRedirect` returning `http.ErrUseLastResponse`), and never wrap or `%v` a request or a file
  (`raidAIDownload` is the pattern).
- `fuku/utils/claude` imports the standard library only, so `scripts/claude-spike` can use it without the pre-main
  database connection of `fuku/config` and `fuku/db`. The Anthropic key travels only in the `x-api-key` header, never
  in a URL, and no error that package returns contains it. No sampling parameter (`temperature`, `top_p`, `top_k`) is
  ever sent, because the default model rejects them with a 400. `claude.Options` and `GetModel` exist for the spike tool
  only; its zero value is the production request, and the bot never sets it.
- A missing i18n key returns `""` with an error callers ignore; a typo'd key in Go ships an empty message. Copy key
  names from `locales/en.yml`. `locales/config.yml` is the help alt-name pseudo-locale, not a language.
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
- Defaults that surprise: `REDIS_DB=1`; `ENABLE_AISPAM=true` (the global AI kill switch, text and images; inert without `TYPESAFE_API_KEY`); `ANTHROPIC_API_KEY` empty
  means no raid image check and no image leaves the server, and `ANTHROPIC_MODEL` falls back to `claude.DefaultModel`
  (with a warning) when empty or not a valid model name; because the bot reads the standard `ANTHROPIC_API_KEY` name, a
  key exported for another tool on the same host turns the image checks on; perf monitoring and
  background stats are on only when `DEBUG=false`; `HTTP_PORT` falls back to `PORT` then 8080. Integer env parsing is
  lenient, so a typo silently becomes the default.

## Testing

- Always pass `-tags testtools`; without it, test files and the helpers in production packages that carry that build tag drop out silently.
- Postgres tests need both `DATABASE_URL` and `FUKU_TEST_DATABASE=true`; otherwise they run on SQLite.
- Use real fixtures: `internal/testdb.Run` (SQLite), miniredis, hand-written `gotgbot.BotClient` fakes. No mock libraries.
- No test calls a real AI provider (TypeSafe or Anthropic): the Claude client, the raid AI image path and the
  `scripts/claude-spike` tool are tested against `httptest` servers or a fake transport, with no `t.Skip`, so they run
  in `make test`. The live Claude spike (`go run ./scripts/claude-spike`, needs `ANTHROPIC_API_KEY`) is a manual owner
  step, never part of CI. It refuses an `-endpoint` that is not https (plain http is accepted only for `localhost`,
  `127.0.0.1` and `::1`), because the key goes there in a header, and it exits 1 when every request failed.
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
  The Phase 13 guarantees are `TestChatRequirementsSettings` (`./fuku/db/memberreq`: the five action CHECKs refuse 5 and -1
  with SQLSTATE 23514, column-targeted upserts of two rules at once never clobber each other, a Go 0 and `false` are
  written), `TestRequirementHoldClaimConcurrent` (of 16 concurrent claims of one `(chat_id, user_id)` exactly one wins, and of
  16 concurrent reclaims of one stale pending hold exactly one wins) and `TestRequirementHolds` (the take, restore, clean and
  clear semantics); all three also run on SQLite in `make test`, and `fuku/db/memberreq/testmain_test.go` migrates both models.
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

## Commits

- Use Conventional Commits. The changelog drops `docs:`/`test:`/`chore:`/`ci:`/`deps:`, so user-visible changes need
  `feat:` or `fix:`.
