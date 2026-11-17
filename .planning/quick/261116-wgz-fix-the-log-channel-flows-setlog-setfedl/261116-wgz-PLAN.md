---
phase: 261116-wgz-fix-the-log-channel-flows-setlog-setfedl
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - fuku/modules/logchannels.go
  - fuku/modules/logchannels_flow_test.go
  - fuku/modules/logchannels_categories_test.go
  - fuku/modules/logchannels_legacy_request_test.go
  - fuku/modules/logchannels_apply_test.go
  - fuku/modules/federations.go
  - fuku/modules/federations_setfedlog_flow_test.go
  - fuku/modules/federations_command_test.go
  - fuku/modules/settings_logchannel.go
  - fuku/modules/settings_logchannel_test.go
  - fuku/modules/settings_logchannel_screens_test.go
  - fuku/modules/settings_phase18_test.go
  - fuku/db/logchannels/repository.go
  - fuku/db/logchannels/repository_test.go
  - fuku/db/backup/logchannels_old_export_test.go
  - fuku/i18n/settings_locale_test.go
  - locales/en.yml
  - locales/es.yml
  - locales/fr.yml
  - locales/hi.yml
  - locales/id.yml
  - locales/pt.yml
  - locales/ru.yml
  - locales/uk.yml
  - README.md
  - README.uk.md
  - AGENTS.md
  - docs/src/content/docs/commands/federations/index.md
  - docs/src/content/docs/commands/logchannels/index.md
autonomous: true
requirements: [QUICK-261116-wgz]

estimate:                   # no calibration run was available while planning: factor 1, confidence low
  tokens: 150000
  raw_tokens: 150000
  tasks: 3
  confidence: low

must_haves:
  truths:
    - "A channel post `/setlog` (no `from`) is accepted by the bot and stores a one-hour marker; the same post forwarded by a live administrator of a group, to that group, binds the channel as the group's log channel and replies logs_set_ok, in one update, through the real dispatcher. A member's forward, a forward with no marker and a forward of another message bind nothing, and a member's forward does not consume the marker. A plain `/setlog` typed in a group still answers logs_need_channel."
    - "A channel post `/setfedlog <fed id>` stores a one-hour marker whose value is the fed ID and binds nothing. The federation OWNER forwarding that post to the bot's private chat or to a group sets the federation's log chat to the CHANNEL (never to the chat the forward was sent in). A non-owner's forward binds nothing and leaves the marker; a forward with no live marker binds nothing and says so; a marker is single use. Typed `/setfedlog` in a group keeps setting that group."
    - "The two log categories nothing ever posts to are gone from everything a user can see or set: /log and /nolog treat them as unknown names, the Log channel settings page has four cells (rows {2,2,2}), logs_categories and logchannels_help_msg list four names, the two page-name locale keys are removed. Their database columns stay, and an old backup export that carries them still imports."
    - "No change touches a per-group admin check: /setlog still needs a live admin of the group before the marker is consumed, /setfedlog needs the live fed owner before the marker is consumed."
  artifacts:
    - path: "fuku/modules/logchannels.go"
      provides: "channel-post /setlog, one shared bind function for the command and the group-11 watcher, shared marker helpers (store, peek, consume) and channelForwardOrigin"
    - path: "fuku/modules/federations.go"
      provides: "channel-post /setfedlog marker step and the owner's forward step"
    - path: "fuku/modules/logchannels_flow_test.go"
      provides: "TestSetLogFlowThroughDispatcher (channel post then forward, memory and miniredis stores)"
    - path: "fuku/modules/federations_setfedlog_flow_test.go"
      provides: "TestSetFedLogFlowThroughDispatcher and TestSetFedLogLocaleKeys"
    - path: "fuku/modules/logchannels_categories_test.go"
      provides: "TestLogCategoriesAcceptOnlyLiveNames and TestLogCategoryLocaleTexts"
    - path: "fuku/db/backup/logchannels_old_export_test.go"
      provides: "guard that an export carrying all six flags still imports"
  key_links:
    - from: "fuku/modules/logchannels.go (setLog, group branch)"
      to: "fuku/modules/logchannels.go (bindSetLogForward)"
      via: "the group-0 command binds a forwarded channel post itself; the group-11 watcher calls the same function for forwards that carry no command text"
    - from: "fuku/modules/federations.go (setFedLog)"
      to: "fuku/modules/logchannels.go (channelForwardOrigin, storePendingMarker, peekPendingMarker, consumePendingMarker)"
      via: "same one-hour marker store, keyed fuku:setfedlog:<channel>:<message>, outside the fuku:cache: prefix"
    - from: "fuku/modules/settings_logchannel.go (settingsLogCategories)"
      to: "fuku/db/logchannels/repository.go (AllCategories)"
      via: "the page table lists exactly the accepted names, in AllCategories order"
---

<objective>
Fix the three defects of batch Q1 of the 2026-11-16 audit, items 1 to 3 of section Q1 in .planning/quick/AUDIT-261116-FINDINGS.md, and nothing else: `/setlog` cannot link a log channel (item 1), `/setfedlog <fed id>` in a channel can never run (item 2), and two log categories that nothing posts to are still offered to users (item 3). The owner's decisions in that file are locked: fix `/setfedlog` with the same two-step proof as `/setlog` (do not drop it), and remove the two dead categories from what users see and set without a migration.

Purpose: a group's admins can finally link a log channel by the documented steps, a federation owner can set a channel as the federation log, and the Log channel page and `/log` stop offering switches that do nothing.
Output: handler fixes with dispatcher-level tests, locale strings in all 8 files, README, generated docs and AGENTS.md updated in the same commits.

Decomposition note: tracer-first is met by Task 1, which proves the whole channel post to forward to bound path through the real dispatcher, the marker store and the database for `/setlog` and leaves the shared marker helpers Task 2 reuses. Task 2 expands the same proof to the federation log. Task 3 removes the dead categories. Each task ends green and is committed on its own (one commit per defect, tests and fix together, the failing run recorded first).
</objective>

<execution_context>
@.claude/gsd-core/workflows/execute-plan.md
@.claude/gsd-core/templates/summary.md
</execution_context>

<context>
@.planning/STATE.md
@.planning/quick/AUDIT-261116-FINDINGS.md
@AGENTS.md
@.claude/CLAUDE.md

## Verified against the current tree (commit 98497d3), line numbers re-located by symbol

Item 1, confirmed, all three defects exist in fuku/modules/logchannels.go:
- 1a. `LoadLogChannels` registers `helpers.NewBotCommand("setlog", ...)` with no `SetAllowChannel(true)`; gotgbot's `Command.CheckUpdate` only looks at `ctx.ChannelPost` when `AllowChannel` is set, so the channel post is dropped (grep count of SetAllowChannel in the file is 0 at HEAD).
- 1b. `setLog` calls `requireUser` first; for a channel post `ctx.EffectiveSender.User` is nil (no `from`), so it replies common_cannot_identify_user and ends.
- 1c. A forwarded channel post keeps its text and its bot_command entity at offset 0, so in the group the group-0 `setlog` command matches the forward first; the handler replies logs_need_channel and returns ext.EndGroups, and `captureSetLogForward` (group 11) is never reached. The existing tests call `setLog` and `captureSetLogForward` directly with a user and so hid all three.
- The marker keys are fine: `cache.GetMarshal().Set` and the raw `GetDel` address the same Redis key `fuku:setlog:<channel>:<message>`. The new test runs on the memory store and on miniredis to prove it.

Item 2, confirmed, in fuku/modules/federations.go:
- Registration of `setfedlog` (in `LoadFederations`) has no `SetAllowChannel`; `setFedLog` calls `requireUser` first (nil for a channel post) and its channel branch compares `fed.OwnerID` with `from.Id`, which needs a user.
- Extra finding that fixes the design: a forwarded channel post carries the text `/setfedlog <fed id>`, so the command also matches the forward. Today a forward to a group would fall into the group branch and set THE GROUP as the log, and a forward to the private chat is refused with feds_group_only (`RequirePrivate` is true for a private chat). The handler must therefore recognise a forward whose origin is a channel before the group branch, and bind the channel the post came from.
- `federations_help_msg` lists `/setfedlog /unsetfedlog` under "Owner (in private)", but the handler refuses a private chat. `/unsetfedlog` has no chat restriction and works in private. Locations of the help text: en.yml and uk.yml are one quoted line, es, fr, hi, id, pt and ru are `|` blocks (English text in those six today).

Item 3, confirmed: nothing calls `actionlog.Log` with either category. The wrappers in fuku/utils/actionlog/actionlog.go use the settings, admin, user and reports categories, and the five `actionlog.Destination` callers (lockdown alert, lockdown response, raid trigger, staff log) all use admin. The only other references are the Log channel page table in fuku/modules/settings_logchannel.go, the category table in fuku/db/logchannels/repository.go and tests. Backup import is already tolerant: `importLogChannels` decodes the whole `models.LogChannel` row (its JSON keys and columns stay), it never reads `AllCategories`, so no import change is needed; a guard test is added.

Baselines at HEAD for the gates below: dead-category identifiers and locale keys in fuku and locales: 28 matches; SetAllowChannel in federations.go and logchannels.go: 0 each; the page-controls literal, the four-row width literal, the "65 states" text and the "452" key count in AGENTS.md: 1, 2, 1 and 1 matches; the combined `/setfedlog /unsetfedlog` help line: 1 in each of the 8 locale files; "automated" in README.md and README.uk.md: 2 each.

## Out of scope, seen while planning (relay to the owner, no change planned)
Typed `/setfedlog` in a group (the group branch of `setFedLog`) checks only that the sender owns a federation, not that the sender is an administrator of that group, so a federation owner who is a plain member can point the federation log at that group. It is not in section Q1; it is the kind of per-group authority gap AGENTS.md forbids, so it deserves its own item.

## Rules for every task (from the audit file and AGENTS.md)
- Failing test first through the REAL dispatcher (`ext.NewDispatcher(&ext.DispatcherOpts{MaxRoutines: -1})` plus the module's `Load` function and `dispatcher.ProcessUpdate`, as aispam_test.go does), fresh IDs from uniqueModuleChatID and uniqueLinkOwnerID/uniquePositiveUserID, cleanup in t.Cleanup, no mock libraries, no t.Skip (a case that cannot apply is not registered), keep tests light. New test files carry `//go:build testtools`.
- Targeted runs only: `go test -tags testtools -race -count=1 -run '^...$' ./pkg`. Never `make test`. Finish with `CGO_ENABLED=0 go build ./...`, `go vet -tags testtools ./...`, `make check-translations`, `make check-docs`. A first `-race` compile of fuku/modules takes about three minutes.
- Never re-record or weaken a TestLegacy*Requests golden; where a fix changes a pinned byte on purpose, change only that expectation and say so in the commit message.
- Locale keys go in all 8 files (en, es, fr, hi, id, pt, ru, uk) with a real translation in the language of each file, in the voice of the neighbouring feds_ and logs_ strings; the bot's name stays Xenobot-72. Text sent with replyHTML is HTML: write angle brackets as `&lt;` and `&gt;`.
- Locale, AGENTS.md and README edits are located by key name or heading, never by line number: other quick tasks of the same audit may land first.
- Conventional Commits, `fix:`, one commit per defect. The commit body ends with the attribution lines required for this session. Do not push.
</context>

<tasks>

<task type="tracer" tdd="true">
  <name>Task 1: /setlog works end to end through the dispatcher (Q1 item 1: 1a, 1b, 1c)</name>
  <files>fuku/modules/logchannels.go, fuku/modules/logchannels_flow_test.go, AGENTS.md</files>
  <read_first>
    fuku/modules/logchannels.go
    fuku/modules/logchannels_command_test.go
    fuku/modules/aispam_test.go (aispamProcessUpdate, aispamMessageUpdate for the ProcessUpdate pattern)
    fuku/modules/testredis_test.go (withMiniredisServer)
    fuku/modules/settings_env_test.go (withRealLocales)
    fuku/utils/helpers/command_registry.go
    .planning/quick/AUDIT-261116-FINDINGS.md (section Q1, item 1)
  </read_first>
  <behavior>
    Through one dispatcher with LoadLogChannels, run every case on the in-memory store and again on miniredis (`withMiniredisServer`, the production GETDEL path). Each case uses a fresh group, a fresh channel, `chats.EnsureChatInDb` for both and a cleanup that unsets the log channel row and drops the log_channel cache key. The admin is user 777000 (the fake answers creator), the member is user 42 (the fake answers member). The channel post is an update with `ChannelPost` only: chat type channel, `SenderChat` the channel, no `From`, text `/setlog` with a bot_command entity at offset 0 length 7. The forward is a group `Message` from the sender with the same text and entity and `ForwardOrigin` a gotgbot.MessageOriginChannel naming the channel and the post's message ID.
    - Channel post then admin forward in the group: after the post alone the group has no log channel and the bot sent logs_forward_to_group to the channel; after the forward `logchannels.Get(group)` names the channel and the group got logs_set_ok (assert the real uk text through `i18n.MustNewTranslator("uk")` after `withRealLocales`).
    - A member's forward binds nothing, and the admin's forward afterwards still binds (the member did not consume the marker).
    - A forward whose origin message ID was never posted, and a forward after the bind (single use, after unsetting the row) bind nothing.
    - A plain `/setlog` typed by an admin in the group answers logs_need_channel and binds nothing.
    - With a nil cache marshaler (`withNilCacheMarshal`) the channel post answers common_settings_save_failed and the forward binds nothing.
  </behavior>
  <action>
    RED first. Write fuku/modules/logchannels_flow_test.go as described in the behavior block, run it and record that it fails at HEAD (the channel post is dropped, so the forward is answered with logs_need_channel and no row is written). Only then change production code.

    Fix in fuku/modules/logchannels.go, all three defects together:
    1. 1a: register `setlog` as `helpers.NewBotCommand("setlog", logChannelsModule.setLog).SetAllowChannel(true)`. It must stay built through helpers.NewBotCommand (TestNoRawNewCommand). The other five commands are unchanged.
    2. 1b: in `setLog`, handle the channel branch before any user lookup. The channel branch needs no user: the post proves a channel admin wrote it, and the authority check is step 2 (the forwarder must be an administrator of the group). Keep the nil guards for chat and message first. The non-channel branch keeps today's `requireUser` call so its behaviour for a sender-less update is unchanged.
    3. 1c: in the non-channel branch of `setLog`, before replying logs_need_channel, call the shared bind function; when it bound, return ext.EndGroups with no second reply; when it did not, reply logs_need_channel and end exactly as today (so a plain `/setlog`, a member's forward and a forward with no marker are answered as they are now).
    4. Extract `bindSetLogForward(b, ctx) bool` from `captureSetLogForward`: everything from the channel-origin check to the logs_set_ok reply and the `actionlog.Settings` audit post, returning true only when the log channel was stored; it replies nothing and returns false on every other path. Keep the order: chat type (group or supergroup only), channel origin with a non-zero message ID, the sender (silent `chat_status.RequireUser`), `chat_status.RequireUserAdmin` for that group, and only then the marker consume, then `logchannels.Set`, so a non-admin forward never consumes the marker. `captureSetLogForward` becomes a thin watcher that calls it and returns ext.ContinueGroups on every path (kept for forwards that carry no command text).
    5. Add the small shared helpers Task 2 reuses, in the same file: `channelForwardOrigin(msg) (channelID, messageID int64, ok bool)` (a forward whose merged origin chat is of type channel; false for a nil message, a nil origin, or a non-channel origin), `storePendingMarker(key string, value []byte) error` (the marshaler `Set` with a one-hour expiry; an error when the marshaler is nil), and `consumePendingMarker(key string) bool` (the atomic consume that `captureSetLogForward` has today: raw GETDEL when a Redis client exists, otherwise marshaler get then delete; code moved unchanged). `setLog`'s channel branch stores its marker through `storePendingMarker` with the value `1`, keeping the existing key helper `setlogPendingKey`.

    Do not touch logchannels_command_test.go: its direct-call tests must keep passing. After the fix, run the new test and the existing log channel tests, run `gofmt -l` on the changed Go files (fix only your own files), then add one bullet to AGENTS.md as the last bullet of the `## Subsystem traps` section, directly before the `## Go rules` heading: the log channel link is a two-step proof (a channel post `/setlog` stores a one-hour marker keyed by channel and message ID outside the cache prefix; the forward by a live administrator of the group binds, authority checked before the marker is consumed); `setlog` is a documented raw `helpers.NewBotCommand` with `SetAllowChannel(true)` because a channel post has no sender and WrapCommand would reply to it; the group-0 command binds a forwarded post itself because the forward still carries the command text, and the group-11 watcher stays for forwards without command text; the group-11 watcher and the command share `bindSetLogForward`; a forward of a public channel's marked post by an administrator of another group within the hour binds that channel to that group, a stated limit of the owner's design. Commit as `fix(logchannels): make /setlog work end to end`.
  </action>
  <verify>
    <automated>test "$(grep -c 'NewBotCommand("setlog".*SetAllowChannel(true)' fuku/modules/logchannels.go)" -eq 1 && go test -tags testtools -race -count=1 -run '^(TestSetLogFlowThroughDispatcher|TestLogChannelSetAndUnset|TestLogChannelUnknownCategory|TestLogChannelCommandReportsUnset|TestLogChannelCommandsAndForwardBind|TestSetLogForwardRequiresExactMessageAndFailsClosedWithoutCache)$' ./fuku/modules</automated>
  </verify>
  <done>The new dispatcher test passes on both stores and failed before the change; a channel post `/setlog` reaches the handler without a user and stores the marker; the group-0 command and the group-11 watcher bind through one function; member forwards, unmarked forwards and replays bind nothing; the existing direct-call tests still pass; AGENTS.md has the new bullet; one commit.</done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: /setfedlog channel flow with the same two-step proof (Q1 item 2)</name>
  <files>fuku/modules/federations.go, fuku/modules/federations_setfedlog_flow_test.go, fuku/modules/federations_command_test.go, locales/en.yml, locales/es.yml, locales/fr.yml, locales/hi.yml, locales/id.yml, locales/pt.yml, locales/ru.yml, locales/uk.yml, README.md, README.uk.md, docs/src/content/docs/commands/federations/index.md, AGENTS.md</files>
  <read_first>
    fuku/modules/federations.go (setFedLog, unsetFedLog, ownedFedOrReply, parseFedID, LoadFederations)
    fuku/modules/logchannels.go (the helpers Task 1 added)
    fuku/modules/federations_command_test.go (TestImportFBansNeedsFileAndSetFedLogInChannel, TestFedSubscribeLogExportAndDelete)
    fuku/modules/logchannels_flow_test.go (the dispatcher helpers from Task 1)
    locales/en.yml and locales/es.yml (feds_setfedlog_* keys and federations_help_msg)
    .planning/quick/AUDIT-261116-FINDINGS.md (section Q1, item 2)
  </read_first>
  <behavior>
    Through one dispatcher with LoadFederations, on the memory store and on miniredis. Every case makes its own owner with `uniquePositiveUserID`, a federation with `federations.CreateFederation` (one per owner, deleted in t.Cleanup) and fresh channel and group IDs. The channel post is an update with `ChannelPost` only (no `From`), text `/setfedlog <fed id>` with a bot_command entity. The forward is a Message from the forwarder with the same text and `ForwardOrigin` a gotgbot.MessageOriginChannel naming the channel and the post's ID, sent in the owner's private chat or in a supergroup.
    - Channel post alone: the federation's LogChatID stays 0 and the bot replied to the channel (feds_setfedlog_forward_to_owner).
    - Owner forwards in private: LogChatID equals the CHANNEL's ID and the owner got feds_setfedlog_ok. Owner forwards in a supergroup: LogChatID equals the channel's ID, not the group's.
    - A stranger's forward binds nothing, answers feds_not_owner, and the marker survives: the owner's forward afterwards binds.
    - A forward with no marker, and a second forward after a successful bind (reset the log with `federations.SetFedLogChat(fedID, 0)` first), bind nothing and answer feds_setfedlog_no_pending.
    - A marker made for another owner's federation, forwarded by this owner, binds nothing and answers feds_not_owner.
    - A channel post with a missing or malformed fed ID answers feds_setfedlog_need_id and stores no marker.
    - Typed `/setfedlog` (no forward origin) in a supergroup by the owner still sets that group, and a forward whose origin is a user does not take the channel path.
    Locale test: after `withRealLocales`, every language of the eight has both new keys non-empty, and federations_help_msg in every language names `/setfedlog` and `/unsetfedlog` but no longer joins them in the combined `/setfedlog /unsetfedlog` form.
  </behavior>
  <action>
    RED first. Write fuku/modules/federations_setfedlog_flow_test.go (TestSetFedLogFlowThroughDispatcher, TestSetFedLogLocaleKeys), run it and record that it fails at HEAD (the channel post is dropped and the locale keys are missing). Then edit fuku/modules/federations_command_test.go: in TestImportFBansNeedsFileAndSetFedLogInChannel the direct channel call with a user used to expect an immediate bind; that expectation is replaced on purpose by the new two-step contract, so the call now expects LogChatID to stay 0 (the post only records a marker). Say so in the commit message. Leave every other federation test untouched; the typed group `/setfedlog` test must still pass.

    Fix in fuku/modules/federations.go:
    1. Register `setfedlog` as `helpers.NewBotCommand("setfedlog", federationsModule.setFedLog).SetAllowChannel(true)`; `unsetfedlog` is unchanged.
    2. Add `setfedlogPendingKey(channelID, messageID int64)` returning `fuku:setfedlog:<channel>:<message>` (outside the `fuku:cache:` prefix) and use Task 1's `storePendingMarker` with the fed ID as the value (one hour).
    3. `setFedLog` channel branch, before any user lookup and with no user required: parse the fed ID with `parseFedID` (a bad or missing one answers feds_setfedlog_need_id and stores nothing), store the marker (a nil marshaler or a store error answers common_settings_save_failed), reply feds_setfedlog_forward_to_owner. The post must not check or reveal whether the federation exists or who owns it, because the poster is not identified; the owner's forward is the proof.
    4. `setFedLog` non-channel branch: keep `requireUser`. Then, before the private refusal and before the group branch, when `channelForwardOrigin(msg)` reports a channel origin, run the forward step and return ext.EndGroups on every path; a forward from a channel must never fall into the group branch and never set the current chat. The forward step runs in this order, each step refusing before the next: (a) read the marker without consuming it (add `peekPendingMarker(key string) ([]byte, bool)` next to the other helpers in logchannels.go: marshaler get of the stored bytes; fail closed when the marshaler is nil) and read the fed ID from it through `parseFedID`; none or expired answers feds_setfedlog_no_pending; (b) load the federation with `federations.GetFedContext` (tracing.UpdateContext of the update) and require `OwnerID` equal to the forwarder's user ID, else answer the existing feds_not_owner and leave the marker; (c) `consumePendingMarker(key)`, and a lost race answers feds_setfedlog_no_pending; (d) `federations.SetFedLogChat(fed.FedID, channelID)` with the channel taken from the forward's origin, never from text; a failure answers the existing feds_create_failed (it is the key this handler already uses for that failure); (e) reply feds_setfedlog_ok. Log lines carry the channel ID and the error only. A forward whose origin is a user, a hidden user or a chat is not a channel forward and keeps today's behaviour. There is no group-11 watcher for this flow: every post that can create a marker is itself a command message and a forward keeps its text and entities, so the command handler sees every legitimate forward.
    5. Locales, all 8 files, keys placed next to feds_setfedlog_ok, each translated into that file's language in the voice of the neighbouring strings: feds_setfedlog_forward_to_owner (the request is recorded; now forward this post, from the federation owner's account, to me in a private chat or in a group I am in, within 1 hour) and feds_setfedlog_no_pending (that forward matches no pending /setfedlog post or it expired; post `/setfedlog` followed by the fed ID placeholder in the channel again and forward the new post within 1 hour; write the placeholder's angle brackets as the HTML entities for less-than and greater-than, because the reply is sent as HTML). Correct federations_help_msg in all 8 files: `/unsetfedlog` stays in the owner block (it works in private) with its own line; the combined line is removed; a new block headed as the federation log chat (owner) says that `/setfedlog` typed in a group makes that group the federation log, and that for a channel the owner posts `/setfedlog <fed_id>` in the channel (the bot must be an admin there) and forwards that post to the bot in private or to a group from the owner account. In en.yml and uk.yml the text is one quoted line with `\n` escapes; in es, fr, hi, id, pt and ru it is a `|` block whose body is English today: edit those six in English and translate nothing else in them. Keep the existing bullet style and the `*bold:*` headings.
    6. Then run `make generate-docs` and keep only the diff in docs/src/content/docs/commands/federations/index.md. In README.md and README.uk.md the federation owner row lists `/setfedlog` under the private chat: move `/setfedlog` out of that row into its own short row or note that says it is used in a group, or as a channel post plus a forward.
    7. AGENTS.md: add a bullet directly after the Task 1 bullet in `## Subsystem traps`: the federation log chat (`fuku:setfedlog:<channel>:<message>`, value the fed ID, one hour, single use; the owner check runs before the marker is consumed; the channel branch needs no user; the forward step is part of the `setfedlog` command and binds the CHANNEL the post came from; typed `/setfedlog` in a group is unchanged).

    Run the new tests and the federation tests, `gofmt -l` on the changed Go files, `make check-translations` and `make check-docs`, then commit as `fix(federations): make the /setfedlog channel flow reachable`.
  </action>
  <verify>
    <automated>test "$(grep -c 'NewBotCommand("setfedlog".*SetAllowChannel(true)' fuku/modules/federations.go)" -eq 1 && go test -tags testtools -race -count=1 -run '^(TestSetFedLogFlowThroughDispatcher|TestSetFedLogLocaleKeys|TestImportFBansNeedsFileAndSetFedLogInChannel|TestFedSubscribeLogExportAndDelete|TestFedCommandErrorPaths|TestFedLifecycleCommands|TestSetLogFlowThroughDispatcher)$' ./fuku/modules && make check-translations && make check-docs</automated>
  </verify>
  <done>A channel post `/setfedlog <fed id>` stores a marker and binds nothing; only the live fed owner's forward, in private or in a group, sets the federation log to the channel; a non-owner, a missing marker and a replay bind nothing; typed group `/setfedlog` is unchanged; the two new keys exist in all 8 locales and the help text no longer lists `/setfedlog` under private use; README, generated docs and AGENTS.md match; one commit.</done>
</task>

<task type="auto" tdd="true">
  <name>Task 3: remove the two dead log categories from what users see and set (Q1 item 3)</name>
  <files>fuku/db/logchannels/repository.go, fuku/db/logchannels/repository_test.go, fuku/db/backup/logchannels_old_export_test.go, fuku/modules/settings_logchannel.go, fuku/modules/settings_logchannel_test.go, fuku/modules/settings_logchannel_screens_test.go, fuku/modules/settings_phase18_test.go, fuku/modules/logchannels_legacy_request_test.go, fuku/modules/logchannels_apply_test.go, fuku/modules/logchannels_categories_test.go, fuku/i18n/settings_locale_test.go, locales/en.yml, locales/es.yml, locales/fr.yml, locales/hi.yml, locales/id.yml, locales/pt.yml, locales/ru.yml, locales/uk.yml, README.md, README.uk.md, docs/src/content/docs/commands/logchannels/index.md, AGENTS.md</files>
  <read_first>
    fuku/db/logchannels/repository.go
    fuku/modules/settings_logchannel.go
    fuku/modules/logchannels_legacy_request_test.go
    fuku/modules/logchannels_apply_test.go
    fuku/modules/settings_logchannel_test.go
    fuku/modules/settings_logchannel_screens_test.go
    fuku/modules/settings_phase18_test.go (settingsPhase18Controls)
    fuku/i18n/settings_locale_test.go (the limits table and the log category names list)
    fuku/db/backup/backup_test.go (TestImportChatData_EarlierBotNameExport pattern)
    .planning/quick/AUDIT-261116-FINDINGS.md (section Q1, item 3)
  </read_first>
  <behavior>
    - Through the real dispatcher with LoadLogChannels, a chat with a linked channel and all flags on: an admin's `/nolog` naming the two dead categories, alone or next to a live one, treats them as unknown names (logs_unknown_category when nothing live is named, only the live ones listed otherwise) and their stored flags stay on; `/log` and `/nolog` of a live category are unchanged.
    - The Log channel page draws four cells in rows {2,2,2} (two rows of two cells, then Back and Close), with control codes s, a, u and r unchanged, the four lines of the body and the 17 states (16 flag combinations with a channel, plus the unlinked page) of the worst-case tests.
    - logchannels.AllCategories is exactly settings, admin, user, reports; IsValidCategory and the column lookup refuse the two dead names; SetCategory refuses them with an error and writes nothing; Set still writes the full row.
    - An old backup export whose log channel settings carry all six flags (the two dead ones included) still imports and keeps the stored values of those two columns.
    - In all 8 languages logs_categories and the Categories line of logchannels_help_msg name the four live categories and neither dead one.
  </behavior>
  <action>
    RED first. Write fuku/modules/logchannels_categories_test.go with TestLogCategoriesAcceptOnlyLiveNames (dispatcher, behavior block above) and TestLogCategoryLocaleTexts (`withRealLocales`, eight translators, every live name present, the word automated absent, and no ", other" tail in logs_categories). Write fuku/db/backup/logchannels_old_export_test.go (TestImportLogChannelsOldExportKeepsAllSixFlags, built like TestImportChatData_EarlierBotNameExport from a raw JSON export with a logchannels module whose settings carry all six `cat_` flags with the two dead ones false); it passes before and after the change and exists to prove the import stays tolerant. Run the first two and record that the dispatcher test fails at HEAD, then change production code.

    Production changes, by concept (do not search for the old identifiers by hand; the grep gate below finds every leftover):
    1. fuku/db/logchannels/repository.go: remove the two constants for the automated and other categories, their entries in `AllCategories` (leaving settings, admin, user, reports in that order) and their entries in the category map behind `IsValidCategory`, `categoryColumn` and `CategoryEnabled`. Leave `Set` writing the full row and leave the model, its columns and its JSON tags alone: no migration, and the old export keeps importing. Update the package comment so it says the other two columns are kept for old rows and exports.
    2. fuku/modules/settings_logchannel.go: set the category count constant to 4 and the table to the four live rows with their control codes s, a, u and r; update the comments that say six.
    3. Update the expectations that the change moves on purpose, nothing else: repository_test.go (the helper test now expects the two dead names invalid and their columns empty, AllCategories of length four); logchannels_apply_test.go (the baseline that turned a dead category off uses the reports category instead); settings_logchannel_test.go and settings_logchannel_screens_test.go (rows `[2 2 2]` and `{2,2,2}`, button counts of four cells plus Back and Close, the message texts that say six or three rows); settings_phase18_test.go (the Log channel page's control list becomes the four codes); fuku/i18n/settings_locale_test.go (remove the two limits-table rows and the two names from the log category names list); any further test that fails to compile or fails after the change, found by running the targeted lists below. In logchannels_legacy_request_test.go keep the six-character flag strings of every case byte-identical by making `logGoldenFlags` read the six stored columns straight from the row (the two dead columns are still in the table), and change only the case that names the dead word, `several_off`: `/nolog admin user` plus the dead category now answers and audits only `admin, user` (the dead name is unknown while two live ones apply), and its stored flags keep that column on. Say in the commit message that this pinned expectation changed on purpose; no other golden changes.
    4. Locales, all 8 files: remove the page-name key of each dead category (the two `settings_log_cat_` keys for them); change logs_categories and the Categories line of logchannels_help_msg to the four live names (English text in es, fr, hi, id, pt and ru as today; the Ukrainian line stays Ukrainian). In es, fr, hi, id, pt and ru the help message is a `|` block, in en and uk one quoted line.
    5. README.md (two places: the feature line that says six categories, and the `/logcategories` row) and README.uk.md (the same two places, the Ukrainian feature line says six): four categories. Run `make generate-docs` and keep only the diff in docs/src/content/docs/commands/logchannels/index.md.
    6. AGENTS.md, in the Log channel page bullet and its neighbours: six categories become four everywhere in that bullet (the category table and its control code list, the sentence about the other five categories being untouched, the privacy sentence about the flags, the no-channel sentence about the lines reading Off, the row widths in both places, and the state count of the worst-case test, sixty-five today, becoming seventeen), the "six log category names" wording in the two testing bullets becomes four, the count of `settings_` keys in the locale files drops by two, and the note on TestLegacyLogCategoryRequests records that only the case naming the dead word changed on purpose. Add one sentence where the log categories are described: the two columns and the two JSON keys stay for old rows and exports and nothing writes or reads them.

    Run the targeted lists below, `gofmt -l` on the changed Go files, `make check-translations` and `make check-docs`, then commit as `fix(logchannels): drop the two log categories nothing posts to`.
  </action>
  <verify>
    <automated>test "$(grep -rn 'CategoryAutomated\|CategoryOther\|settings_log_cat_automated\|settings_log_cat_other' fuku locales --include=*.go --include=*.yml | wc -l)" -eq 0 && ! grep -Fq 's a u m r o' AGENTS.md && ! grep -Fq '65 states' AGENTS.md && ! grep -Fq '{2,2,2,2}' AGENTS.md && go test -tags testtools -race -count=1 ./fuku/db/logchannels && go test -tags testtools -race -count=1 -run '^(TestImportLogChannelsOldExportKeepsAllSixFlags|TestExportChatData_Full)$' ./fuku/db/backup && go test -tags testtools -race -count=1 -run '^(TestSettingsLocaleKeys|TestSettingsLabelLengths|TestSettingsReusedKeyPlaceholders)$' ./fuku/i18n && go test -tags testtools -race -count=1 -run '^(TestLogCategoriesAcceptOnlyLiveNames|TestLogCategoryLocaleTexts|TestLegacyLogCategoryRequests|TestApplyLogCategory|TestNologTogglesCategory|TestSettingsLogChannel.*|TestSettingsParityLogCategory|TestSettingsDriftLogChannel|TestSettingsAuthorityParityLogChannel|TestSettingsPhase18Codes|TestSettingsLocksKeysExist|TestSettingsMainPhase18|TestSettingsPhase18EndToEnd|TestSettingsStaffPrivacy)$' ./fuku/modules</automated>
  </verify>
  <done>/log and /nolog accept only settings, admin, user and reports; the Log channel page has four cells in rows {2,2,2}; the two locale keys, the two limits rows and every text that listed six categories are gone or corrected in all 8 languages, README and generated docs; the legacy golden changed only in the case that named a dead category; an old export with six flags still imports; AGENTS.md matches; the grep gate over fuku and locales finds no leftover; one commit.</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| channel post to bot | a post proves only that someone who can post in the channel wrote it; it carries no user |
| forward to bot (private or group) | the forwarder is a user the bot must authorise itself before acting |
| callback and command text | attacker-controlled: fed IDs, channel IDs in text are never trusted for the target |
| Redis marker store | one-hour markers keyed by channel and message ID, outside the cache prefix |

## STRIDE Threat Register

Threat IDs are unique within this plan; `T-wgz-SC` is the reserved supply-chain row.

| Threat ID | Category | Component | Severity | Disposition | Mitigation Plan |
|-----------|----------|-----------|----------|-------------|-----------------|
| T-wgz-01 | Spoofing | /setfedlog forward step | high | mitigate | the forwarder must be the live owner of the federation named by the marker (GetFedContext, OwnerID equals the forwarder) before the marker is consumed; a stranger binds nothing and does not burn the marker; the target is the forward origin's channel ID, never text |
| T-wgz-02 | Elevation of privilege | /setlog forward step | high | mitigate | `chat_status.RequireUserAdmin` for that group runs before the marker is consumed and before `logchannels.Set`; the per-group admin check is not skipped on the command path that now binds |
| T-wgz-03 | Tampering | marker forgery | medium | mitigate | a marker exists only when the bot received a channel post (only channel admins post); keys carry channel and message ID; one hour expiry; consumed once (GETDEL, or get then delete without Redis); a nil marshaler fails closed |
| T-wgz-04 | Information disclosure | replies and logs | low | mitigate | the channel post reply does not reveal whether a federation exists or who owns it; no reply names the channel or the federation; log lines carry chat or channel IDs and the error only |
| T-wgz-05 | Tampering | forward of a public channel's marked /setlog post by an admin of another group within the hour | medium | accept | owner's design for step 2 (the authority is the forwarder's admin status in the group); marker is single use and expires in one hour; /setfedlog is not exposed to this because its marker names the federation and the owner is checked; stated in AGENTS.md |
| T-wgz-06 | Denial of service | marker spam by channel admins | low | accept | markers expire in one hour and hold a few bytes; only channel admins can create them |
| T-wgz-07 | Repudiation | audit of a bound log channel | low | mitigate | the existing `#SETTINGS` audit post of a /setlog bind is kept in the shared bind function |
| T-wgz-SC | Tampering | npm/pip/cargo installs | high | accept | no dependency is added or changed by this plan; no install task exists |
</threat_model>

<verification>
Run from the repository root after the three commits (each task already ran its own targeted tests):
- `CGO_ENABLED=0 go build ./...`
- `go vet -tags testtools ./...`
- `go test -tags testtools -race -count=1 -run '^(TestSetLogFlowThroughDispatcher|TestSetFedLogFlowThroughDispatcher|TestSetFedLogLocaleKeys|TestLogCategoriesAcceptOnlyLiveNames|TestLogCategoryLocaleTexts|TestNoRawNewCommand|TestLegacyLogCategoryRequests)$' ./fuku/modules`
- `make check-translations` and `make check-docs`
- `git log --oneline -3` shows three `fix(...)` commits, none pushed.
Never run `make test`.
</verification>

<success_criteria>
- `/setlog` as a channel post, then forwarded by a live administrator of a group, links that channel as the group's log channel in one dispatcher run; member forwards, unmarked forwards and replays bind nothing.
- `/setfedlog <fed id>` as a channel post, then forwarded by the federation owner, sets the federation log chat to the channel; every other forwarder binds nothing and leaves the marker alone.
- `/log`, `/nolog`, `/logcategories`, the help text and the Log channel page offer exactly four categories; the old export with six flags still imports.
- The new strings exist in all 8 locale files; `make check-translations` and `make check-docs` pass; AGENTS.md, README and generated docs describe the new behaviour.
- No existing golden was re-recorded except the one expectation named in Task 3, and the commit says so.
</success_criteria>

<output>
Create `.planning/quick/261116-wgz-fix-the-log-channel-flows-setlog-setfedl/261116-wgz-SUMMARY.md` when done
</output>
