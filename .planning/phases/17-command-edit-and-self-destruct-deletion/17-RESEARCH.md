# Phase 17: Command, Edit and Self-Destruct Deletion - Research

**Researched:** 2026-11-12
**Domain:** Go Telegram bot (gotgbot v2.0.0-rc.36, GORM on PostgreSQL / SQLite tests, Redis): deleting members' messages (commands after the answer, late edits, self-destruct) on the Phase 16 queue, worker, pacer and Deletion page
**Confidence:** HIGH for every code fact (each discrete value below was read this session and is quoted with file and line), MEDIUM for Telegram behaviour that the pinned library docs do not state (the Bot API site was not reachable from this session, so those items are listed as live checks)

Provenance tags: `[VERIFIED: file:line]` = file opened this session, value quoted verbatim; `[VERIFIED: run]` = I ran it here (a throwaway PostgreSQL 16 on a private socket, started and stopped by me; nothing of the repository touched); `[CITED: gotgbot doc comment]` = text of the pinned module `github.com/PaulSonOfLars/gotgbot/v2@v2.0.0-rc.36.0.20260919140833-240296efadb4`, which mirrors the Bot API text; `[ASSUMED]` = reasoning or training knowledge, listed in the Assumptions Log.

<user_constraints>
## User Constraints (from CONTEXT.md)

The decisions D-01 to D-20 below are the owner's provisional defaults (CONTEXT "Provisional status": none confirmed, owner confirmations A17-1 to A17-8). They are copied verbatim by script from `17-CONTEXT.md`. Where the code disagrees with one, this file says so as a numbered correction C-nn with a recommended amendment and does not deviate silently.

### Locked Decisions

### Delete commands (area 1, DEL-03)
- **D-01:** A command is a message in a group or supergroup whose first entity is a `bot_command` at offset 0 and whose command name is one this bot registers, written bare (`/ban`) or addressed to this bot (`/ban@thisbot`). A command addressed to another bot (`/ban@otherbot`) and a `/word` this bot has no handler for are never deleted, because in a group with several bots a bare `/word` may belong to another bot and deleting it breaks that bot's reply. The sender may be anyone who is not a Telegram service identity: members, approved users and admins alike (closed owner decision). A command refused by a check (not an admin, command disabled, wrong chat type, bot not admin) or failed with an error is deleted too, because it was addressed to this bot and answered.
- **D-02:** The delete happens after the answer, never before: every handler that replies to the command runs first, and the command is deleted once all groups of the update have run. The mechanism is the researcher's and planner's to settle (research flag, decision 5), against these requirements: it covers commands handled by `helpers.WrapCommand`, the raw `NewCommand` interceptors (the StaffActions table and `settingsCancelPassOn`) and the anonymous-admin re-entry, it is one place and not one hook per command, and the recommended shape is a thin wrapper of the dispatcher's `Processor` (today `tracing.TracingProcessor{}` in `main.go`) that inspects the finished update and calls the deletion core. An anonymous admin's command is deleted only after the proof tap re-entered and ran it, from the re-entered update, never at the first message (the proof prompt replies to that message) and never when the tap never comes (the message then stays, which is accepted). A Staff Group is never touched (D-17).
- **D-03:** The command row is a switch: `member:commands` with a stored delay of 0 means On, an absent row means Off, and the delete goes through `deletionAct` with delay 0 (the inline path with the pacer, the queue fallback on a long wait and the latches of Phase 16), so a busy group's commands never park dispatcher goroutines. Replies the bot sent to a command are not touched (DEL2-01), and a bot reply that targets a deleted command still sends because the reply parameters allow sending without the reply target where they do not already (planner checks every `WrapCommand` reply path, Pitfall 13).

### Delete edited messages (area 2, DEL-04)
- **D-04:** An edit is any `edited_message` update of a group or supergroup message whose sender is a real user. There is no stored copy of the original text, so an edit that changes nothing (a link preview filling in, a caption touch) is indistinguishable from an author's edit and counts the same: a stated limit, bounded by the grace period (a preview arrives within seconds), and the live check "Edited-message noise" from the roadmap stays an owner step (deferred, like the other live checks). The rule judges `edit_date - date`: an edit made within the grace period of the message being sent is kept (usually a typo fix), a later one deletes the message at once through `deletionAct` with delay 0 (the edit time is already past the grace).
- **D-05:** The grace is the stored delay of `member:edited` in seconds, with presets 30 s, 1 min, 5 min, 15 min and 1 h and nothing else; switching the rule on from Off stores 60 s (the 1-minute default of the roadmap). An absent row means Off. A message older than 47 hours since its own date is skipped with no call (Telegram refuses it), and no notice is posted anywhere.
- **D-06:** Exempt: creators and administrators by the cached admin list (an unknown list bias to keep, as the member requirements watcher does), approved users, an anonymous admin or a channel (`sender_chat` set), bots, `via_bot` messages (an inline bot's edit is reported as the user's), automatic forwards, a Staff Group chat, and a member with a pending captcha (their messages are held for replay at group -10). The handler is a small raw handler for edited updates (`handlers.Handler` whose `CheckUpdate` is `ctx.EditedMessage != nil`), never `NewMessage(...).SetAllowEdited(true)`, which would also match every new message. It returns `ext.ContinueGroups` on every path with its result preset so a recovered panic cannot return `nil`; its handler group is the planner's under the AGENTS.md rules (group 1 holds the settings door, which only matches new private messages, so they never meet). This is the first handler that consumes edits: antiflood, locks, blacklists and aispam still do not see edits (an existing gap, out of scope).

### Self-destruct (area 3, DEL-05)
- **D-07:** Storage is PostgreSQL rows in the existing `scheduled_deletions` queue (kind `member:selfdestruct`), not a Redis sorted set (research decision 6, settled): a Redis flush or eviction would silently drop every pending deletion, while the table survives a restart, runs once across replicas and already has the worker, the claim, the stale release and the 47-hour drop. The accepted failure modes are database write load in a busy group and a message that stays when its row was shed (D-09); neither ever deletes something that should stay.
- **D-08:** The write path never stalls the dispatcher. The recommended shape is a bounded in-process batcher (a channel flushed every few hundred milliseconds or at 100 rows into one `ON CONFLICT` insert that keeps the earliest time, every flush under `defer error_handling.RecoverFromPanic`), joined into `StopDeletionWorker`'s existing 3 s wait so it adds no drain and no allowance; a message that cannot be queued because the channel is full or the flush failed is simply kept (shed with a counter and one rate-limited log line, fail toward keeping). The planner and researcher may replace the batcher with a plain synchronous insert only if they show the dispatcher cost is bounded by one round trip and the busy-group test (below) passes.
- **D-09:** `delete_at` is the message's own send time plus the stored delay (D-08 of Phase 16). Presets: 1 min, 5 min, 10 min, 1 h, 6 h and 24 h, the cap of Phase 16 (`deletion.MaxDelaySecs`) and its 47-hour age drop unchanged. A per-chat ceiling on queued self-destruct rows (20,000, a package variable only so a test can shrink it) refuses new rows beyond it, so a flood cannot grow the table without bound; the ceiling is checked cheaply (the planner chooses between a bounded count at flush and an approximate counter) and a refused message is kept. A delay change on the page changes only messages sent afterwards; rows already queued keep their time, as everywhere in Phase 16.
- **D-10:** What counts: every message of a real user in a group or supergroup (text, media, stickers, polls, commands, forwards, replies) while the rule is on. Exempt, never queued: creators and administrators by the cached list, approved users, `sender_chat` senders (anonymous admins, channels, linked-channel automatic forwards), bots, service messages (the service rules of Phase 16 own those), any Staff Group chat, a member with a pending captcha (their held messages), and a message the bot itself sent (its own messages are the bot-message kinds' business). The watcher runs before group 0 like the other watchers so the commands and everything else are seen, never ends an update (`ext.ContinueGroups` on every path, named result preset), is registered through `handlers.NewMessage` with no `SetAllow*`, and takes a handler group number the planner chooses under the AGENTS.md rule (never a second message handler in group -2, and group 12 stays last).
- **D-11:** Pinned messages stay. When a `pinned_message` service message names a message, the queue row of that message and kind is deleted at once (a conditional delete by chat and message ID), so a message pinned between its send and its deletion time survives. Unpinning does not queue anything again (accepted: the message stays until an admin removes it). The currently pinned message of a chat at the moment the rule is switched on is not found by a scan, because the rule only queues messages sent after it is on.
- **D-12:** Switching self-destruct Off drops the chat's pending `member:selfdestruct` rows in the same write (the existing `dropPendingKindsTx` exact-kind drop), so nothing already queued is deleted after the owner turned it off; rows an `acting` worker already claimed are left to it. Turning it On or changing the delay drops nothing.
- **D-13:** Throughput: the existing worker (1 s tick and wake, 200 rows a cycle, cycles back to back until nothing settles, `deleteMessages` of up to 100 IDs per call, the deletion pacer with its own keys, per-ID fallback) is the only deleter, so a busy group deletes in batches of 100 and never races the bot's replies for a pacer budget (the deletion pacer is separate from every other). The planner proves it with a busy-group test over the real worker and pacer: a few thousand queued rows are deleted inside the cycle budget with no more calls than rows/100 plus the fallbacks, the bot's reply latency in the same run is unchanged, and a 429 block delays only deletions.

### Page and safeguards (area 4)
- **D-14:** The Deletion page gains a third section, Member messages (section code `m`, the hub screen lists Service messages, Bot messages and Member messages), with three rows: Commands (`mc`), Edited messages (`me`) and Self-destruct (`ms`). They are registered from the deletion page's own tables (the section and row tables of `deletion_kinds.go`), so `settings_deletion.go` gains rows and `settings_pages.go` is not edited again. The planner checks the new callback codes against every registered code. The three kinds are added to `deletion.ValidKind` (`member:commands`, `member:edited`, `member:selfdestruct`) with a per-kind delay rule enforced in Go: commands 0 only, edited 30 to 3600 in the presets, self-destruct 60 to 86400 in the presets.
- **D-15:** Row screens use the existing row-screen shape (`a=dr&t=<row>`, controls `st` only with the absolute value `v`): Commands offers Off and On; Edited messages offers Off and the five grace presets (the hint says "an edit made within this time of sending is kept"); Self-destruct offers Off and the six presets. Every choice writes at once except turning Self-destruct on (from Off, or from one delay to another): that draws the shared two-tap card (`settingsCardRows`, `e` the expiry minute, Yes is the page action with the stored-row check of the card helper) saying that every message members send from now on is deleted after the chosen time (messages already sent are not touched) and that admins, approved users and pinned messages stay. A press of the stored value writes nothing. The card is the only safeguard the page adds, because the rule can be switched off with one tap and Off drops what is queued (D-12).
- **D-16:** Rights are Phase 16's: opening the page and its screens needs `settingsRightAdmin`, every choice and the card's Yes need `settingsRightChangeInfo` judged on the press's fresh member record, there is no command for these rules (so each carries a drift test and an authority test in place of a parity test), the page is hidden and refused in a Staff Group (`Hidden`, D-18 of Phase 16), and every screen of the member section carries the same live status line about the bot's right to delete (DEL-08: `deletionReadBotRight`, nothing when it can, the cannot line, the unknown mark). No screen names the Staff Group. The hub's state counts every rule that is not Off, the three new ones included.
- **D-17:** DEL-07 stays enforced in three layers: the new watchers and the command and edit hooks refuse a Staff Group chat through `deletionAct` (which already refuses a non-negative chat ID and a Staff Group, failing closed on a read error), the queue writer refuses it in `deletion.Schedule`, and the page is not drawn in a Staff Group. A test lists the new kinds and asserts a Staff Group chat gets no row and no delete from any of the three rules, and that a lockdown alert copy, a captcha challenge, a staff card and a log post are never queued by self-destruct (they are bot messages, and D-10 excludes the bot's own messages).
- **D-18:** Strings: every new string exists in all 8 locales with a row in the limits table of `fuku/i18n/settings_locale_test.go` and labels held to the 20-code-point cell and 64-byte callback limits; the section name, three row names, descriptions, hints (including "a command is deleted after the bot answered it", "an edit within the grace is kept", "admins, approved users and pinned messages stay"), the card text and the preset labels reuse Phase 16's delay words where they exist. `TestSettingsStaffPrivacy` renders the new screens in all 8 languages.
- **D-19:** No new shutdown drain and no new Redis key family is expected: the batcher (D-08) is joined into `StopDeletionWorker`'s 3 s wait, the latches, guards and pacer of Phase 16 are reused, and the shutdown arithmetic of AGENTS.md (68 s allowances against the 60 s budget, `stop_grace_period: 75s`) is restated unchanged. If research finds one is needed, `AGENTS.md` restates the arithmetic in the same commit. A migration is expected to be unnecessary (`kind` is validated in Go); the planner adds one only for an index the ceiling count needs, with `scripts/migrate_psql.sh` and `runner.go` agreeing.
- **D-20:** The documentation, the README feature list, `AGENTS.md` (the handler groups, the Deletion page bullet, the Subsystem traps for the three rules and their stated limits) and the generated docs are updated in the same commits as the code, and every deferred live check goes in a `17-SPIKE.md`: edited-message noise, a bare `/word` in a multi-bot group, the anonymous-admin command deletion, pinned messages seen by the watcher, `deleteMessages` at busy-group volume.

### Claude's Discretion

- The mechanism of D-02 (processor wrapper or per-entry hooks), the batcher's exact flush timings and channel size (D-08), the handler group numbers (D-06, D-10), the ceiling's counting method (D-09), the section and row labels, the preset button layout, locale key names, and the structure of the plans (the tracer slice first: a command deleted after its answer through the real dispatcher, then edits, then self-destruct and the batcher, then the page rows).

### Deferred Ideas (OUT OF SCOPE)

- DEL2-01 (deleting the bot's replies to members' commands) and DEL2-02 (global silence and mass deletion).
- A custom delay number, a typed grace, and self-destruct per message kind (only media, only links).
- Comparing an edit with the original text so that a no-op edit is kept (needs a stored copy of every message).
- Deleting the messages a pin left behind when it is unpinned, and a scan of existing messages when a rule is switched on.
- `/settings` help text naming the Deletion page (A16-A11, still open).
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| DEL-03 | With "Delete commands" on, every member's command in the group (admins' included) is deleted after the bot has answered it | S-01 (Processor wrapper, the registered-name predicate, the anonymous-admin post-state), C-01, C-02, C-03, C-04 |
| DEL-04 | With "Delete edited messages" on, a member's message edited after a grace period (1 minute by default, adjustable) is deleted; admins and approved users exempt | S-02 (how edits arrive, the exemption order, the grace rule), C-08, C-09 |
| DEL-05 | With "Self-destruct" on, every member message is deleted N minutes after it was sent; admins, approved users and pinned messages exempt | S-03 (write path, batcher, ceiling, group, pin drop, shutdown), S-06 (throughput), C-05, C-06, C-07, C-13, C-14 |
| DEL-07 (bind) | Deletion never touches Staff Group items, lockdown alerts, captcha challenges, log posts | S-07 (three layers, exact checks and tests) |
| DEL-08 (bind) | Every deletion page says so when the bot cannot delete | S-04 (member screens reuse the Phase 16 line), C-04 (the latch must cover member kinds) |
| PLAT-05 / PLAT-06 (bind) | 18 new strings in 8 locales; background work shares budgets, recovers, drains within the shutdown budget | S-04, S-03 (shutdown arithmetic unchanged: 68 s allowances against the 60 s budget) |
</phase_requirements>

## Project Constraints (from CLAUDE.md / AGENTS.md)

- Stay on Go 1.26.0, gotgbot v2, GORM, Redis; no new dependency; no mock library; tests use real fixtures (`internal/testdb.Run`, miniredis, hand-written `gotgbot.BotClient` fakes), always `-tags testtools`, `make test` is the only full run, no `t.Skip` to dodge a case. [VERIFIED: AGENTS.md "Testing", "Commands"]
- Migrations are append-only and timestamped; none is expected here (S-05). Every write `cache.DeleteCache`s the keys it affects; a key read on every message belongs in `skipLocal` (`delete_rules:*` already is). [VERIFIED: AGENTS.md "Data", "Migrations"]
- Group numbers are execution order; watchers return `ext.ContinueGroups` with a named result preset (a recovered panic must not return `nil`); never add a second message handler in group `-2`; group `12` stays last; value receivers on `moduleStruct`; new commands use `helpers.WrapCommand`. [VERIFIED: AGENTS.md "Handlers"]
- Every fire-and-forget goroutine starts with `defer error_handling.RecoverFromPanic(...)`; shutdown drains are registered after DB-close and restated in AGENTS.md in the same commit; all strings in all 8 locales; callback data through `callbackcodec` (64 bytes); no log line carries message text, claim tokens or message IDs on the deletion paths. [VERIFIED: AGENTS.md "Go rules", "Callbacks", "Subsystem traps"]
- The per-group live admin check and the owner gate are never skipped by a settings door; `AGENTS.md` is updated in the same commit as any rule it invalidates. [VERIFIED: AGENTS.md "Permissions", header]

## Summary

Phase 17 needs **no new package, no new dependency, no migration and no new Redis key family**. Everything rides the Phase 16 core: `deletionAct` for the two immediate rules (commands, edits), the `scheduled_deletions` queue and the worker for self-destruct, the deletion pacer, the `delete_rules` cache key (already in `skipLocal`) and the Deletion page tables. The three rules are Go table rows plus one new watcher module, one `ext.Processor` wrapper, one raw edit handler and one bounded batcher. PostgreSQL was exercised this session (see S-03): a multi-row `INSERT ... ON CONFLICT ... DO UPDATE SET delete_at = LEAST(...)` keeps the earliest time, **the same key twice in one statement fails** (`ON CONFLICT DO UPDATE command cannot affect row a second time`), and an exact per-chat `COUNT` for the ceiling costs **2.7 ms at 20,000 rows inside a 520,000-row table** (it uses `uk_scheduled_deletions_message`), so the ceiling needs neither a counter nor an index.

The decisions hold with **fourteen corrections** (C-01 to C-14, section "Corrections"). The ones that change the design: (1) "a command this bot registers" has **no existing registry** (134 raw `handlers.NewCommand` sites and 61 `WrapCommand` sites; the disable list covers only disableable ones), so a one-line recording shim and a guard test are needed, and the docs generator's regex must follow it (C-01); (2) the Processor wrapper must read the **post-run state** of the `ext.Context`: the anonymous-admin tap replaces `ctx.Update`, the cached message's `From` is the Group Anonymous Bot (a Telegram service ID), and the tracing processor's context is cancelled when it returns (C-02); (3) the delete-right latch of Phase 16 is **service-only**, so a group where the bot cannot delete members' messages would cost one failed paced call per command, edit and row forever (C-04); (4) D-12's "Off cancels what is queued" has a **race window** (batcher buffer, another replica, an `acting` batch) that a flush-time and a worker-time rule check close (C-06); (5) D-13's "no more calls than rows/100" is false across many chats: the worker chunks **per chat per cycle** and claims 200 rows, so its drain rate is about `2000 / N` rows per second for N active chats (C-14).

**Primary recommendation:** build the tracer first (the registry shim plus the Processor wrapper, one real `WrapCommand` command deleted after its real answer through the real dispatcher, with `tracing.TracingProcessor` as the inner processor), then edits, then the self-destruct write path (batcher + `ScheduleBatch`) and watcher, then the page rows, then locales, then the closing sweeps. Take handler group **-8** for the new watchers (verified free), keep the Processor wrapper outside `TracingProcessor` and give its post-step its own fresh context, route every deletion through `deletionAct` (commands, edits) or the batcher (self-destruct), and generalise the "needs the delete right" latch to the member kinds.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Command deletion after the answer | API / Backend (Processor wrapper in `fuku/modules`, wired in `main.go`) | Database (rules read), Telegram (inline delete) | Only the dispatcher knows every group of the update has run; a wrapper is the one place covering raw and wrapped commands |
| Registered command names | API / Backend (`fuku/utils/helpers` registry filled at registration) | Docs generator (regex follows the shim) | The names must come from the registrations themselves, not a second list |
| Edited-message deletion | API / Backend (raw handler, group -8) | Database (rules, approvals), cache (admin list) | Update-driven; the grace is arithmetic on `edit_date - date` |
| Self-destruct queueing | API / Backend (watcher + bounded batcher) | Database (`scheduled_deletions` bulk upsert) | The dispatcher must never wait on a write; the table is the durable store (D-07) |
| Self-destruct execution | API / Backend (the Phase 16 worker, unchanged except the rule screen) | Redis (pacer keys, latch) | One deleter, fleet-wide pacing, batches of 100 |
| Rules storage and validation | Database (`chat_delete_rules`) + Go (`fuku/db/deletion` per-kind rules) | Cache (`delete_rules:<chat>`, `skipLocal`) | `kind` is validated in Go by design; the CHECK already bounds 0..86400 |
| Deletion page rows and card | API / Backend (settings framework tables) | Telegram client (inline keyboard) | Rows are table entries; `settings_pages.go` is not edited |
| Exclusions (DEL-07) | API / Backend (`deletionAct`, `deletion.Schedule`, the batcher, the page's `Hidden`) | Database (fresh Staff Group read) | Three layers, as Phase 16 |

## Standard Stack

### Core (all existing; no new dependency)

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/PaulSonOfLars/gotgbot/v2` | v2.0.0-rc.36 (`...20260919140833-240296efadb4`) | `ext.Processor`, `ext.Handler`, `Message.PinnedMessage`, `DeleteMessages` | The bot's client; the Processor seam and the handler interface are exported [VERIFIED: ext/processor.go, ext/handler.go:5-12] |
| `gorm.io/gorm` + postgres/sqlite drivers | v1.31.2 / v1.6.3 / v1.6.0 | one multi-row `Create` with `clause.OnConflict` | the queue repository's existing pattern (`queue.go:110-115`) |
| `github.com/redis/go-redis/v9`, `eko/gocache` | v9.22.0, v4.4.0 | latch, pacer keys, the cached rules read | existing |
| `fuku/utils/ratelimit` (`NewTelegramPacer`) | in repo | the deletion pacer, unchanged | `deletionPacer` [VERIFIED: deletion_schedule.go:34-40] |

**Installation:** none. **Version verification:** nothing is added; `go list -m -f '{{.Dir}}' github.com/PaulSonOfLars/gotgbot/v2` gave the directory read for this research.

## Package Legitimacy Audit

No external package is added. Packages removed (SLOP): none. Packages flagged (SUS): none. The planner needs no `checkpoint:human-verify` install task.

## Corrections to CONTEXT.md and the UI-SPEC (the planner must carry these)

| # | Where | Problem (evidence) | Recommended amendment |
|---|-------|--------------------|-----------------------|
| C-01 | D-01 "a command name this bot registers", D-02 "one place" | There is **no registry of registered command names**. 134 raw `handlers.NewCommand(` sites and 61 `helpers.WrapCommand(` sites exist (`grep` this session); `helpers.AddCmdToDisableable` records only the disableable ones (`decorators.go:21-25`); some names come from tables (`handlers.NewCommand(spec.Name, staffActionEntry(spec))`, `staff_action.go:84`). The wording "first entity is a `bot_command` at offset 0" is also narrower than the matcher: gotgbot's `Command.checkMessage` accepts text with **no entities** that starts with `/`, matches `Caption` through `GetText()`, is case-insensitive and compares `@name` to the bot's username [VERIFIED: ext/handlers/command.go:95-130]. The docs generator **regex-scans** `handlers.NewCommand("name", handler)` (`scripts/generate_docs/parsers.go:152`), so rewriting the call sites silently drops commands from the generated docs. | Add `helpers.NewBotCommand(name, resp) handlers.Command` that records the lower-cased name in a mutex-guarded set and returns `handlers.NewCommand`; `helpers.register()` uses it; replace the 134 sites mechanically; a `go/parser` guard test fails on any `handlers.NewCommand(` outside the shim; update `parsers.go:152` and `main.go:308` (a label only) of the generator to match the shim name in the same commit (`make check-docs` is the gate). Reword D-01: "a message gotgbot's command matcher would hand to a registered handler" (pure function `botCommandName(b, msg)` mirroring `checkMessage`, with a differential test against `handlers.NewCommand(name).CheckUpdate` over a table of texts) |
| C-02 | D-02 "inspects the finished update", "from the re-entered update" | Three traps in the post-run state. (a) `verifyAnonymousAdmin` **replaces** `ctx.Update`: `msg.SenderChat = nil` / `ctx.Update = &gotgbot.Update{UpdateId: updateID, Message: msg}` / `ctx.EffectiveSender = &gotgbot.Sender{User: &fromCopy, ChatId: chatId}` [VERIFIED: bot_updates.go:231,232,238]; `ctx.Message` stays nil, so the wrapper must read `ctx.Update.Message`. (b) The cached message's `From` is the Group Anonymous Bot, and `staffServiceUserIDs = map[int64]bool{777000: true, 1087968824: true, 136817688: true}` [VERIFIED: staff_action_decide.go:513] holds 1087968824: a "not a service identity" test on `msg.From` would skip every re-entered command; the sender must be read from `ctx.EffectiveSender`. (c) `TracingProcessor` creates `baseCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)` and `defer cancel()` [VERIFIED: tracing/processor.go:62-63] in polling mode, so `tracing.UpdateContext(ctx)` is **cancelled** once the processor returned; webhook mode passes a live context (`server.go:370`), so the bug would show in polling only. | The wrapper is `commandDeletionProcessor{Inner ext.Processor}` placed **outside** `tracing.TracingProcessor{}` (`main.go:431`); its post-step uses `context.WithTimeout(context.Background(), deletionRecordTimeout)`, never `tracing.UpdateContext`; it reads `ctx.Update.Message`, `ctx.EffectiveSender` and `ctx.EffectiveChat` after `Inner` returned, skips a bot sender (`ctx.EffectiveSender.IsBot()`, `sender.go:137-139`), a sender with `Chat != nil` or `msg.SenderChat != nil` (the first anonymous update), a non-group chat and a panic path (no `defer`: an unrecovered handler panic skips the delete). Tests: with `BaseProcessor` **and** with `TracingProcessor` as `Inner`, polling-style context |
| C-03 | D-03 "a bot reply that targets a deleted command still sends because the reply parameters allow sending without the reply target ... planner checks every WrapCommand reply path"; Pitfall 13 "set `AllowSendingWithoutReply` where the target may be deleted" | The premise is wrong in the useful direction: handlers reply **synchronously inside the update**, and the wrapper deletes **after** `Inner.ProcessUpdate` returned, so every reply is already sent. gotgbot's `replyTo` does not set `AllowSendingWithoutReply` [VERIFIED: custom_helpers.go:51-62], and 53 `ReplyParameters` sites exist, so a blanket edit is both unnecessary and risky. The goroutines in `fuku/modules` (list of `go func` files, this session) are tickers, aispam, antiflood actions, the purge notices (which delete bot messages), raid posts (replies to joins) and Staff Group runs (excluded); none replies to a member's command after the handler returned. The one late reply to a command is the anonymous-admin proof flow, handled by C-02. No callback reads `ReplyToMessage` of a card (`grep` this session: no match with `query`/`qmsg`/`callback`). | Drop the "planner checks every reply path / set AllowSendingWithoutReply" task. Replace by (1) the ordering test "reply request precedes the delete request in the fake's call log" for a wrapped command, a raw command and the anonymous re-entry, and (2) a **guard test** that any new `go func` in a command handler that replies must not target the command (reviewed in the closing plan) |
| C-04 | D-01 "refused or failed commands too", D-16 (DEL-08), D-17 | The negative latch and the live right check are **service-only**: `case deletionScopeService: return deletion.IsServiceKind(kind)` [VERIFIED: deletion_schedule.go:248-249], `if !deletion.IsServiceKind(a.Kind) { return }` [VERIFIED: deletion_schedule.go:451], and in the worker `if r.decided || !deletion.IsServiceKind(row.Kind) {` [VERIFIED: deletion_worker.go:423] and `deletion.IsServiceKind(other.Kind)` [VERIFIED: deletion_worker.go:432]. A bot that is a plain member can delete its own messages but not members' (the Phase 16 docs quote), so in such a group every command, edit and queued row would cost one paced failing call with no latch, forever. | Add `deletion.NeedsDeleteRight(kind)` (service and member kinds, never bot kinds) and use it at the four sites; the latch's `service` scope keeps its stored value but its meaning becomes "cannot delete other people's messages". Tests: a group where the bot is a plain member makes one live check, then no calls, for each of the three rules |
| C-05 | D-14 "a per-kind delay rule enforced in Go: commands 0 only, edited 30 to 3600, self-destruct 60 to 86400" | Two different domains share one `delay` word. `deletion.SetRule` and `Schedule` both call `validateDelay` (0..86400) [VERIFIED: rules.go:72, queue.go:82]. The **rule** delay of `member:edited` is the grace (30..3600), but a **queue entry** of that kind is the immediate-delete fallback (`deletionAct` delay 0 becomes a row due now when the inline cap or the pacer refuses, `deletion_schedule.go:365-371`), so its entry delay is 0. Putting the rule table into `Schedule` would refuse the fallback row. | Two functions in `fuku/db/deletion`: `ValidateRuleDelay(kind, secs)` (called by `SetRule` and `applyDeleteRule`: bot and service kinds keep 0..86400 as today, `member:commands` 0, `member:edited` in {30,60,300,900,3600}, `member:selfdestruct` in {60,300,600,3600,21600,86400}, **by value, not range**, so a database edit cannot create an unlisted grace) and `ValidateEntryDelay(kind, secs)` (called by `Schedule`/`ScheduleBatch`: `member:commands` and `member:edited` entries 0 only, `member:selfdestruct` the same six values, others 0..86400). Both return `ErrDelayOutOfRange` before any write |
| C-06 | D-12 "Switching Self-destruct Off drops the chat's pending rows ... so nothing already queued is deleted after the owner turned it off" | `RemoveRule` drops **pending** rows in the same transaction [VERIFIED: rules.go:111-115, `dropPendingKindsTx` 94-97] but three paths can still produce or hold a row after the commit: a row sitting in the batcher's buffer, a watcher on another replica that read the rule just before the Off, and an `acting` batch the worker already claimed (the UI hint says "still waiting"). With a batcher flushing every few hundred milliseconds the first two windows are 100 times wider than Phase 16's synchronous write. | Defence in depth, both cheap: (a) the **flush** reads `deletionRulesLookup` once per chat per flush and drops that chat's `member:selfdestruct` entries when the rule is absent; (b) the **worker** screens claimed `member:selfdestruct` rows with one cached rules read per chat per cycle and drops them (completes without a call) when the rule is absent, before the first call of the chunk. Reword D-12: "nothing is deleted after the owner turned it off except a delete call already in flight". The row's own delay is never rewritten |
| C-07 | D-08 "the batcher ... one `ON CONFLICT` insert", D-09 | `deletion.Schedule` makes a fresh Staff Group read plus one insert per message (`queue.go:91`, `110`), and `deletionAct` adds another Staff Group read and a Redis latch read before it (`deletion_schedule.go:360-362`): about **two synchronous database reads and one write per message on the dispatcher goroutine** if the watcher called `deletionAct`. One statement with the same key twice **fails on PostgreSQL** [VERIFIED: run]. | The self-destruct watcher never calls `deletionAct`/`deletionSchedule`; it offers an entry to the batcher (non-blocking). The flush: dedupe by `(chat, message)` in Go (earliest `delete_at` wins), one fresh Staff Group read per distinct chat (a failed read drops that chat's entries: fail closed, message kept), the rule check of C-06, the exact ceiling count per chat (below), then one `deletion.ScheduleBatch` per at most 100 entries (`clause.OnConflict` with `leastExpr`). The ceiling is an exact `SELECT count(*) FROM (SELECT 1 FROM scheduled_deletions WHERE chat_id = ? AND kind = ? LIMIT <ceiling>) s`, no counter and no index (S-03) |
| C-08 | D-10, D-06 "a member with a pending captcha" | For **new messages** the exemption is structural: group `-10` stores the held message, deletes it and `return ext.EndGroups` [VERIFIED: captcha.go:2230,2232], and `ext.EndGroups` stops every later group, so no watcher after `-10` ever sees a pending member's message. For **edits** it is not: `handlers.NewMessage(nil, ...)` at `-10` does not match `EditedMessage` (`ext/handlers/message.go:61-78`). The Processor wrapper does see the update of a held command (the delete is then a wasted call answered "message to delete not found", classified refused and dropped quietly). | New messages: no per-message captcha lookup in the self-destruct watcher (a test asserts a pending member's message leaves no row). Edits: an explicit pending check (the first half of `aispamCarvedOut`: `captcha.HasPendingCaptchaAttemptsContext` then `captcha.GetCaptchaAttempt`). Accept the one wasted call per command of a held member |
| C-09 | D-06 "creators and administrators by the cached admin list (an unknown list bias to keep)" and D-10 | `cachedAdminStatus` alone returns `known=false` for a cold cache [VERIFIED: antiflood.go:107-111] and `IsUserAdminForUpdate` may make an unbounded live Telegram call (`aispamCarvedOut`, aispam.go:239-249). `antifloodModule.userIsFloodExempt` is cached list first, then a **bounded live fallback that assumes an administrator when saturated** (AGENTS.md, flood counter bullet), which is exactly the safe direction for deleting. | Both new paths use one helper over `userIsFloodExempt` for the admin exemption (unknown or saturated means exempt, the message is kept). Approvals through `chat_status.IsApprovedForUpdate(b, ctx, chatID, userID)` (memoised per update, `update_memo.go:23-27`) |
| C-10 | D-06, D-10 "takes a handler group number the planner chooses" | Groups in use (verified by `grep` of every `AddHandlerToGroup` and group constant): `-10, -7, -6, -5, -4, -3, -2, -1`, `0` to `12`. **`-9` and `-8` are free.** gotgbot moves to the next handler of the same group only on `ext.ContinueGroups` (`ext/dispatcher.go` `iterateOverHandlerGroups`: `continue` on `ContinueGroups`, `break` on anything else), so two non-ending handlers may share a group. | One module `MemberDeletion` registers both new handlers at **group -8** (`memberDeleteGroup = -8`): the self-destruct `NewMessage` watcher and the raw edit handler (their `CheckUpdate` are disjoint: new message vs `EditedMessage`). It runs after the captcha hold (-10) and before the fed-ban (-6), the raid counters (-4) and every command (0). The pin row drop lives in the same watcher (S-03), **not** in group -5 |
| C-11 | D-05 "switching the rule on from Off stores 60 s" and UI-SPEC A17-A12 | The page offers presets only, so there is no "switch on without a value" caller; a stored implicit 60 would be dead code. | Remove the sentence from D-05; the 1-minute default is a preset the owner taps. Keep a constant only if a test wants it |
| C-12 | UI-SPEC M31 (row fields) and "tests that change" | Implementable against the real code: `deletionRow` has `Code, Kind, NameKey, DescKey, HintKey, Presets, Match` [VERIFIED: deletion_kinds.go:16-33]; `settingsDelSection.ChoiceRows` is section-level [VERIFIED: settings_deletion.go:58-60]; `settingsDelOptions` labels presets with `settingsDelDelayLabel` (0 reads `settings_del_state_now`), `renderSettingsDelRow` hard-codes `settings_del_hint_time`, `settingsDelStateOf(tr, noneKey, delay, on)` hard-codes `settings_del_state_now` and `settings_del_state_after`. The collisions check passes: no `mc`, `me`, `ms`, `dy` constant exists (`grep -nE '= "(mc\|me\|ms\|dy\|m)"'` finds only page keys `m`). Pinned tests that change **legitimately**: `TestDeletionKindsNeverStaffLockdownCaptchaLog` (`len(registry) != 17` and "want 17", deletion_kinds_test.go:341-343) becomes 20 and the loop keeps rejecting kinds naming staff, lockdown, captcha, log or raid (`member:selfdestruct` contains none); `TestDeletionKindRegistryMatchesRepository` and `TestDeletionKindKeysExist` gain the member table and the new fields; `TestSettingsDeletionStates`/`settingsDelWantState` gain the words struct (Commands `🗑 On`, Edited `⏳ Grace {delay}`); `TestSettingsDeletionNoCard` keeps passing for the hub, lists and row screens (none carries `e`), and gains the card screen with `dy`, `no`, `e` allowed on that screen only; the hub row-budget test's production shape becomes `{2,1,2}`. None is re-recorded to make a change pass | Implement M31 as written with `deletionMemberRows` in `deletion_kinds.go`; keep the Phase 16 render output byte-identical for rows that set none of the new fields (a golden test over the 17 existing rows before the change) |
| C-13 | D-09 ceiling 20,000 and the hint "In a very busy group some messages may be kept." | The ceiling supports a sustained rate of at most `20000 / delay` messages per second per chat: 1 min 333/s, 5 min 66/s, 10 min 33/s, 1 h 5.5/s, 6 h 0.9/s, **24 h 0.23/s (one message every 4.3 s, 833 an hour)**. A group that posts 30 messages a minute with the 24 h preset starts shedding after 11 hours and then keeps every new message permanently; "in a very busy group" understates that. | Keep 20,000 (a larger ceiling multiplies a table that is never pruned: about 250 bytes a row with its index, 5 MB a chat at the cap) but (a) state the table above in AGENTS.md and in owner confirmation A17-6, (b) log the shed as one rate-limited Warn per chat per hour with the chat ID only, and (c) let the hint stand. If the owner wants long delays on lively groups the ceiling becomes a table-driven number per preset (a Go table, no migration) |
| C-14 | D-13 "no more calls than rows/100 plus the fallbacks" and the worker's batch | The worker claims `deletionBatch = 200` rows ordered by `delete_at` across **all chats** and makes **one paced call per chunk of at most 100 IDs per chat per cycle** [VERIFIED: deletion_worker.go:25,27 `deletionBatch = 200`, `deletionChunk = 100`; 245-269 `run`, 319-321 chunk loop]. With the pacer at 100 ms (10 calls a second fleet-wide, `deletion_schedule.go:37`) the calls of a cycle are the number of distinct chats in the claim, so the drain rate is about **`2000 / N` rows per second** for N chats whose rows are due together (20 chats: 100/s, 50 chats: 40/s, 100 chats: 20/s), and the insert rate `N * r` must stay under it. D-13's bound holds per chat, not in total. | Amend D-13's assertion to "calls per cycle <= the sum over chats of ceil(rows of that chat / 100) plus the fallbacks". Raise `deletionBatch` from 200 to **1000** (the claim query is 0.1 ms with 520,000 pending rows [VERIFIED: run, `idx_scheduled_deletions_due`], a 1000-row cycle is at most 10 chunk calls and stays far under `deletionCycleGuard` 90 s and the 2 minute stale claim), which lifts the many-chat drain to about `10000 / N` rows per second. The busy-group test (S-06) asserts it |

(Not wrong, noted for the closing plan: the UI-SPEC's "30 s" grace is inside the hub's shared time hint "The longest is 24 hours", which the hub keeps by contract; the owner's native-speaker read list stands.)

## Settled Findings (what the planner builds on)

### S-01. Command deletion after the answer (question 1; DEL-03; C-01 to C-04)

**The seam.** `Dispatcher.ProcessUpdate` runs `err = d.Processor.ProcessUpdate(d, b, ctx)` [VERIFIED: gotgbot ext/dispatcher.go:274], and the base processor is `return d.iterateOverHandlerGroups(b, ctx)` [VERIFIED: ext/processor.go:19-21], which walks **every group in order** and returns when a handler returns `ext.EndGroups` or the groups run out. Both transports call `ProcessUpdate`: polling goes `Dispatcher.Start` then `processRawUpdate` then `d.ProcessUpdate(b, &upd, nil)` [VERIFIED: ext/dispatcher.go `Start`, `processRawUpdate`], webhook goes `s.dispatcher.ProcessUpdate(s.bot, &update.update, map[string]any{tracing.ContextDataKey: ctx})` [VERIFIED: httpserver/server.go:370]. So a wrapper's code **after** `Inner.ProcessUpdate` runs after every handler group of that update, in both modes, on the update's own goroutine (it holds one `MaxRoutines` slot meanwhile, so the post-step must not block). `main.go:431` builds `Processor:   tracing.TracingProcessor{},` [VERIFIED]; `TracingProcessor` embeds the concrete `ext.BaseProcessor`, so it cannot take an inner processor: the wrapper goes **around** it.

**Wrapper vs per-entry hooks.**

| | Processor wrapper (recommended) | Per-entry hook in `WrapCommand` |
|---|---|---|
| Coverage | every handler: 134 raw `NewCommand` sites + 61 wrapped | the 61 wrapped only; the raw majority (approvals, rules, filters, notes, logchannels, backup, staff interceptors, `/cancel` pass-on) is missed |
| Anonymous admin | sees the swapped `ctx.Update` after the tap (C-02) | needs a second hook in `verifyAnonymousAdmin`/`anonPipelineHandler` |
| Drift | one registry (C-01) + one guard test | every new raw command must remember a hook |
| Failure modes | cancelled tracing context (C-02), wasted call for a held or fed-banned command, a panic skips the delete (safe), post-step runs outside the tracing span | covers fewer commands, silently |

**The predicate** (all pure, cheapest first; `b` is the bot): (1) `ctx.Update != nil` and `ctx.Update.Message != nil` (new messages only, read **after** `Inner`); (2) `msg.Chat.Type` is `group` or `supergroup`; (3) the text or caption starts with `/` (cheap string test, no allocation); (4) `name, ok := botCommandName(b, msg)` mirroring `Command.checkMessage` [VERIFIED: ext/handlers/command.go:95-130]: `RichMessage` branch, `msg.GetText()` non-empty, `ents := msg.GetEntities()`, reject when `len(ents) != 0 && ents[0].Offset == 0 && ents[0].Type != "bot_command"`, first field up to whitespace, lower-cased, `@x` must equal `strings.ToLower(b.User.Username)`, trigger `/`; (5) `helpers.IsBotCommand(name)` from the recording shim; (6) the sender from `ctx.EffectiveSender` (never `msg.From`): `User != nil`, `Chat == nil`, not `IsBot()`, `msg.SenderChat == nil`, not in `staffServiceUserIDs`, not `b.Id`; (7) the rule: one `deletionRulesLookup(ctx, chat)` (cached, `skipLocal`): `member:commands` present; (8) `deletionAct(b, deletionAction{ChatID: msg.Chat.Id, MessageID: msg.MessageId, Kind: deletion.KindMemberCommands, SentAt: deletionSentAt(msg.Date), Delay: 0})` which refuses a non-negative chat, a latched chat and a Staff Group, runs the delete on a counted recovered goroutine under the pacer and falls back to a due-now row when the inline cap (64) or a long pacer wait refuses [VERIFIED: deletion_schedule.go:355-372, 380-400, 468-473]. The wrapper reads Redis and the database only for a registered command in a group, so non-command traffic pays two string tests.

**Anonymous admin, step by step** [VERIFIED: bot_updates.go:134-250, chat_status.go:88-98]. (1) The admin's command arrives with `sender_chat` set; the handler calls `PromptAnonAdminProof`, which caches the message under `fuku:anonAdmin:<chat>:<msg>` for `anonChatMapExpiration = 20 * time.Second` and replies to it with the proof button; the wrapper skips this update (`SenderChat != nil`). (2) The tap is a **callback update**: `verifyAnonymousAdmin` checks `IsUserAdmin(b, chatId, query.From.Id)`, reads the cache, refuses when the callback's chat, the button's chat and the cached message's chat differ, deletes the proof message (`qmsg.Delete`), clears `msg.SenderChat`, **replaces** `ctx.Update` with a message update carrying the cached command, sets `ctx.EffectiveChat` to the command's chat and `ctx.EffectiveSender` to the tapper, and calls `HandleAnonymousAdmin` for the command name. (3) After `Inner` returns, the wrapper sees `ctx.Update.Message` = the cached command, a real-user `EffectiveSender`, and deletes it: only after the re-entered command ran, from the re-entered update, never at the first message, and never when the tap does not come (the message stays; the cache entry expires after 20 s). The replay risk is bounded: the first tap deletes the button before running, the cache entry is not deleted by the tap, and a second racing tap would run the command twice (pre-existing) and delete an already deleted message (refused quietly). `verifyAnonymousAdmin` returns `ext.EndGroups` even when `HandleAnonymousAdmin` found no handler, so the wrapper would delete a command nothing answered; that only affects names with no anonymous handler that an anonymous admin sent through the proof flow, and the accepted outcome is a deleted unanswered command.

**Reply paths.** Every reply path of a registered command runs inside its handler (synchronous), or in the anonymous flow (above). The goroutines found in `fuku/modules` are `admin_index_sweeper`, `aispam` (async AI check), `antiflood` (3), `antiraid` (key sweep), `backup` (cleanup ticker), `captcha`, the deletion core, `lockdown_worker`, `memberreq_watch`, `purges` (4: bulk delete workers and 3 s notice deletes), `raid_*`, `staff_action_run` (Staff Group), `staff_sweeper`, `users` [VERIFIED: `grep -c "go func"` this session]. None replies to the command after the handler returned. The pacer waits (`lockdownPaced`, `memberReqPaced`) run **inside** the handler, so the wrapper waits for them. Result: no `AllowSendingWithoutReply` edit is needed (C-03). Residual race, an owner decision (A17-1): a **bare** `/help`, `/start` or `/rules` that another bot in the group also handles may be deleted before that bot replies to it; `reply_parameters` without `allow_sending_without_reply` is documented as sending only when the target exists [CITED: gotgbot gen_types.go:10661 "Pass True if the message should be sent even if the specified message to be replied to is not found"], the exact failure is a live check (S-09, `17-SPIKE.md` check 2).

**Edge cases the tests must pin.** Edited commands are never handled (`Command.AllowEdited` false) and never matched by the wrapper (`ctx.Update.Message` only). `/cancel` in a group: `settingsCancelPassOn` returns `nil` and the pipeline's `/cancel` check replies nothing (AGENTS.md), so the wrapper deletes a command nobody answered, which D-01 already accepts ("refused by a check"). `/purge` and the silent variants delete the command themselves; the wrapper's second delete is refused quietly. A private chat is never touched (chat type).

### S-02. Edited messages (question 2; DEL-04; C-08, C-09, C-10, C-11)

**Arrival.** Both transports pass `config.AppConfig.AllowedUpdates` [VERIFIED: main.go:347, httpserver/server.go:232], which holds `"edited_message"` [VERIFIED: fuku/config/config.go:479-482]. `ext.NewContext` maps `update.EditedMessage` to `ctx.EffectiveMessage`, `EffectiveChat` and `EffectiveUser` [VERIFIED: ext/context.go:55-58], `ctx.Message` stays nil. No handler consumes edits today: `grep -rn "SetAllowEdited\|EditedMessage\|EditDate" fuku main.go` finds nothing in non-test code (this session), `handlers.NewCommand` defaults `AllowEdited: false`, and `handlers.NewMessage` only matches `ctx.EditedMessage` after `SetAllowEdited` (`ext/handlers/message.go` `CheckUpdate`). Types: `Message.Date int64`, `Message.EditDate int64` ("Optional. Date the message was last edited in Unix time", `gen_types.go:8000-8001`), `ViaBot *User`, `SenderChat *Chat`, `IsAutomaticForward bool` [VERIFIED: gen_types.go:7955-8001]. The library states the noise itself: `EditedMessage` is "New version of a message that is known to the bot and was edited. This update may at times be triggered by changes to message fields that are either unavailable or not actively used by your bot." [CITED: gotgbot gen_types.go:15430]. Whether a link preview or a caption touch yields an update, and with what `edit_date`, stays the owner's live check (S-09, check 1).

**Handler.** A small type implementing `ext.Handler` [VERIFIED: ext/handler.go:5-12]: `CheckUpdate` is `ctx.EditedMessage != nil`; `Name()` unique (`member_edited`); registered with `AddHandlerToGroup(h, memberDeleteGroup)` where `memberDeleteGroup = -8` (C-10); `HandleUpdate` returns through a named result preset to `ext.ContinueGroups` with `defer error_handling.RecoverFromPanic`. Never `NewMessage(...).SetAllowEdited(true)` (it would also match every new message).

**Exemption order, cheapest first** (no I/O until step 5):
1. `msg.Chat.Type` is `group`/`supergroup`.
2. Sender shape: `ctx.EffectiveSender` real user (`User != nil`, `Chat == nil`, not `IsBot()`), `msg.SenderChat == nil`, `!msg.IsAutomaticForward`, `msg.ViaBot == nil` (an inline bot's edit is reported as the user's, Pitfall 15), not in `staffServiceUserIDs`, not `b.Id`. `raidFloodSenderCounts` already covers everything except `ViaBot` and `IsBot` [VERIFIED: raid_flood.go:199-213]; reuse it and add the two tests.
3. Time, pure: `msg.EditDate > 0 && msg.Date > 0 && msg.EditDate > msg.Date`; age `deletionNow().Sub(time.Unix(msg.Date,0)) <= deletionDropAge` (47 h, the existing constant, `deletion_worker.go:44`); an edit is **kept** when `msg.EditDate - msg.Date <= grace` and deleted when it is greater. A zero or missing `EditDate` keeps the message (fail toward keeping).
4. The rule: `deletionRulesLookup` (cached) holds `member:edited`; its stored delay is the grace, and a stored value outside the five presets is treated as absent (C-05: never act on an unlisted grace).
5. Approvals: `chat_status.IsApprovedForUpdate` (memoised). 6. Admin: the C-09 helper. 7. Pending captcha: explicit (C-08).
8. `deletionAct(b, deletionAction{ChatID, MessageID, Kind: deletion.KindMemberEdited, SentAt: deletionSentAt(msg.Date), Delay: 0})`.

The check runs once per update, so a message edited twice is judged by its **last** `edit_date` each time: a first edit inside the grace is kept, a later one outside it deletes. No stored copy of the original is needed, which is D-04's stated limit.

### S-03. Self-destruct write path, group, pin drop and shutdown (question 3; DEL-05; C-05, C-06, C-07, C-13)

**Watcher** (`memberDeleteWatcher`, group -8, `handlers.NewMessage(message.All, ...)`, no `SetAllow*`, so no edits, channel posts or bot senders; named result preset to `ext.ContinueGroups`; `defer RecoverFromPanic`). Order, cheapest first: (1) `msg.PinnedMessage != nil`: run the pin drop (below) and return (a pin service message is not content); (2) chat type group/supergroup; (3) sender shape through `raidFloodSenderCounts` (so no `sender_chat`, automatic forward, service ID, the bot; a `via_bot` message **is** queued, D-10 exempts it only for edits); (4) `memberContentMessage(msg)`: not a service message, defined as the negation of `deletionServiceKindOf` plus the creation and migrate fields (`group_chat_created`, `supergroup_chat_created`, `migrate_*`), so `TestServiceKindsCoverMessageFields` keeps the list honest when gotgbot adds a field; (5) the rule through `deletionRulesLookup` (one Redis `GET`, the key is in `skipLocal`; if the busy-group test shows the read dominates, a 2 s in-process memo is safe because C-06(b) screens at delete time); (6) approvals `IsApprovedForUpdate`; (7) admin (C-09); (8) offer `deletion.Entry{ChatID, MessageID, Kind: KindMemberSelfDestruct, SentAt: deletionSentAt(msg.Date), DelaySecs: delay}` to the batcher. A pending-captcha member never arrives (C-08).

**Batcher** (`member_selfdestruct_batcher.go`): a buffered channel (suggest 4096 entries, a package variable for tests) fed by a non-blocking `select` (full channel: shed with an atomic counter and one rate-limited Warn per chat per hour, chat ID only; the message is kept); one goroutine started by `StartDeletionWorker` (so `main.go` is not touched) with `defer error_handling.RecoverFromPanic`, flushing at 100 entries or every 300 ms; each flush on `context.WithTimeout(context.WithoutCancel(...), deletionRecordTimeout)`: dedupe, per chat one fresh Staff Group read (`deletionStaffLookup`, an error drops that chat's entries), the rule check (C-06a), the exact ceiling count (`SELECT count(*) FROM (SELECT 1 FROM scheduled_deletions WHERE chat_id = ? AND kind = ? LIMIT 20000) s`, entries beyond `ceiling - count` are shed), then `deletion.ScheduleBatch(ctx, entries)`. `ScheduleBatch` validates every entry like `Schedule` (kind, `ValidateEntryDelay`, negative chat, positive message, sent time), builds the rows, and runs **one** `db.DB.Clauses(clause.OnConflict{Columns: chat_id, message_id; DoUpdates: delete_at = leastExpr(db.DB)}).Create(&rows)` per at most 100 rows (GORM issues a single multi-row INSERT for a slice; the repository test counts statements). Verified on PostgreSQL 16 [VERIFIED: run]:

```
INSERT ... VALUES (-100,1,...,now()+60s),(-100,2,...,now()+60s) ON CONFLICT (chat_id,message_id)
  DO UPDATE SET delete_at = LEAST(scheduled_deletions.delete_at, EXCLUDED.delete_at);   -- ok
INSERT ... VALUES (-100,3,...,now()+60s),(-100,3,...,now()+30s) ON CONFLICT ...;       -- ERROR:  ON CONFLICT DO UPDATE command cannot affect row a second time
a later batch (-100,1, +600 s)                                                           -- delete_at stays +60 s (earliest kept)
EXPLAIN (ANALYZE) SELECT count(*) ... WHERE chat_id=-100 AND kind='member:selfdestruct'  -- 20,002 rows among 520,002: Bitmap Index Scan on uk_scheduled_deletions_message, Execution Time: 2.692 ms
claim: ... WHERE state='pending' AND delete_at <= now() ORDER BY delete_at, id LIMIT 200 -- Index Scan using idx_scheduled_deletions_due, Execution Time: 0.111 ms
```

SQLite (the repo's tests) accepts the same statement with `MIN(scheduled_deletions.delete_at, excluded.delete_at)` (the existing `leastExpr`); parameters per row are about 10, so 100 rows is 1000 variables, which is under SQLite's default limit of 32766 for the bundled driver `[ASSUMED]`: a test inserts a full 100-row batch to prove it, and a smaller batch is the fallback.

**Exemption of pinned messages (D-11).** `Message.PinnedMessage` is a `MaybeInaccessibleMessage` interface [VERIFIED: gen_types.go:8097 `PinnedMessage MaybeInaccessibleMessage`; gen_types.go:7657 `GetMessageId() int64` on the interface; `InaccessibleMessage` implements it, gen_types.go:3430]. In the watcher's step (1): `deletion.DropPendingMessage(ctx, msg.Chat.Id, msg.PinnedMessage.GetMessageId(), KindMemberSelfDestruct)`, one `DELETE ... WHERE chat_id = ? AND message_id = ? AND kind = ? AND state = 'pending'` (it uses the unique index), chat taken from the **service message**, never from the nested object. Accepted windows, stated in AGENTS.md: a pin that lands while the message is still in the batcher buffer (about 300 ms; a bounded tombstone set of 256 entries for 2 minutes checked at flush closes it if the planner wants), and a message the worker already claimed. Unpin queues nothing. The service watcher at group -5 is not touched (it stays the only handler there).

**Shutdown arithmetic (D-19), unchanged.** `StopDeletionWorker` today: `cancelDeletionContext()` then waits on the worker's `done` for `deletionWorkerStopWait = 3 * time.Second` [VERIFIED: deletion_worker.go:83-102, 45]. The batcher joins the same wait: stop sets `draining` (further offers are shed), signals the batcher goroutine to do **one final flush with its own 2 s bound on `context.WithoutCancel`**, and `StopDeletionWorker` waits for both `done` channels under **one** 3 s timer, so the function still returns within 3 s and the AGENTS.md figures (68 s of allowances in webhook mode against the 60 s budget, `stop_grace_period: 75s`) are restated unchanged. If the database is slow the flush may be cut off: the entries are lost, which keeps their messages (the safe direction). The worker's own cancel already releases claimed rows free.

**Phase 16's 24 h cap and 47 h drop need no change.** A 24 h row has `delete_at = date + 86400`; the worker drops rows with `sent_at` older than `deletionDropAge` (47 h) and a restart gap of up to 23 h still leaves the message deletable under Telegram's 48 h. The edit skip reuses the same constant.

### S-04. The page (question 4; D-14 to D-18, UI-SPEC M29 to M33; C-12)

Implementable as written; what must change, from the real files:
- `deletion_kinds.go`: add `deletionMemberRows` (Code `mc`, `me`, `ms`; kinds from the new constants) and the optional `deletionRow` fields (`ChoiceRows []int`, `LabelKeys map[int]string`, `ZeroKey`, `AfterKey`, `TimeKey string`, `NoTime bool`, `ConfirmKey string`). Presets: `mc` `[0]`, `me` `[30, 60, 300, 900, 3600]`, `ms` `[60, 300, 600, 3600, 21600, 86400]`.
- `settings_deletion.go`: a third `settingsDelSection` (`Code: "m"`, `Icon: "💬"`, `NoneKey: "settings_state_off_plain"`, `Rows: deletionMemberRows`); `init()` already registers one control per row of every section with `settingsDelPresetValidator(row.Presets)` (a `v` of `n` or the canonical digits of a preset, so `mc` accepts `n` and `0` only) [VERIFIED: settings_deletion.go:90-99, 120-128]; add the page action `dy` next to `dl` and `dr` with `Fields: []string{"t", "v", "e"}` and `Right: settingsRightChangeInfo`, in the order the UI-SPEC fixes (Staff gate, expiry, stored check, write, redraw); `settingsDelStateOf` takes a words struct (icon, zero key, after key, none key) so the list line, the cell, the Currently line and the `✅` mark stay one function; `settingsDelOptions` labels through `LabelKeys`; `renderSettingsDelRow` asks for `row.ChoiceRows`, `TimeKey`, `NoTime`; the card is drawn by the control's `Set` for a row with a `ConfirmKey` through `settingsDelPrepare` and `settingsCardRows(k, map[string]string{"a": settingsActDelYes, "t": row.Code, "v": value}, settingsPageDeletion)`, whose `No` is the core `no` action with `p=d` (the hub) [VERIFIED: settings_card.go:63-81].
- Counts: `grep -cE '^settings_' locales/en.yml` is **312** today; none of the 18 new keys exists, `settings_del_intro` exists and is the one changed text [VERIFIED: `grep -c` per key this session]. `settingsLocaleLimits` gains 18 rows (one per key, it must cover exactly the `settings_` keys, 330 after), `TestSettingsLabelLengths` gains the new names and labels, `TestDeletionKindKeysExist` is extended to the member table **and** the new fields (`make check-translations` cannot see a key read through a table field).
- The page's `Hidden`/`MainState` already count `settingsDelAllRows()`, so the Main line and the hub count include the three new rows with no edit [VERIFIED: settings_deletion.go:152-159, 225-239].
- Callback bytes: UI-SPEC measured 57 bytes for the card's Yes at worst case; the planner's worst-case test (`TestSettingsDeletionCallbackBytes`, `settings_deletion_screens_test.go:132`) is extended with `dy`.

### S-05. Rules, validation and migration (question 5; C-05)

`validateDelay` is the only delay check today (`delay < 0 || delay > MaxDelaySecs`, `kinds.go`), used by `SetRule` (rules.go:72) and `Schedule` (queue.go:82), and `ValidKind` is a linear scan of `kinds` (`kinds.go`). Plan: three constants (`KindMemberCommands = "member:commands"`, `KindMemberEdited = "member:edited"`, `KindMemberSelfDestruct = "member:selfdestruct"`), appended to `kinds`, `IsMemberKind`, `NeedsDeleteRight` (C-04), the two delay functions of C-05 (the rule table by **value**), `ScheduleBatch`, `CountPending(ctx, chat, kind, limit)` and `DropPendingMessage`. `RemoveRule` already drops pending rows of the exact kind (D-12) and `applyDeleteRule` already refuses unknown kinds and reads the stored state through the cached loader before writing [VERIFIED: deletion_apply.go:107-135].

**No migration.** `chat_delete_rules.kind` and `scheduled_deletions.kind` are `VARCHAR(40)`; the longest new kind is `member:selfdestruct` (19 characters). The only database check on delays is `CHECK (delay_secs BETWEEN 0 AND 86400)` [VERIFIED: migrations/20261110120000_add_scheduled_deletion.sql]; every per-kind range sits inside it. `kind` has no CHECK by design (AGENTS.md "Data"). No index is needed: the exact ceiling count uses `uk_scheduled_deletions_message` (2.7 ms at the cap, S-03), the claim uses `idx_scheduled_deletions_due`, the pin drop and the per-chat drop use the unique key's `chat_id` prefix. `scripts/migrate_psql.sh` and `runner.go` stay untouched.

### S-06. Throughput and starvation (question 6; Pitfall 16; C-14)

From the code: the worker loop runs cycles back to back until one settles nothing, then waits for the 1 s tick or a wake [VERIFIED: deletion_worker.go:107-120]; a cycle is `ReleaseStale` (1 statement), `ClaimDue` (1 UPDATE + 1 SELECT, `LIMIT` = `deletionBatch`), then per chat in ascending chat order the latch reads (one Redis `GET` per kind), one Staff Group read, one `deleteMessages` call per chunk of 100 under the pacer, and one `Complete` per chunk [VERIFIED: deletion_worker.go:211-324]. The pacer is 100 ms and 3 retries, with a shared 429 block (`deletion_schedule.go:34-40`); every call of the worker, the inline deletes of commands and edits, and the page's right check share it.

| Quantity | Value | Basis |
|----------|-------|-------|
| Fleet-wide ceiling | 10 calls/s, so at most 1000 message IDs/s | pacer interval 100 ms x 100 IDs per call [VERIFIED: code] |
| One chat, one cycle | 200 rows = 2 calls | `deletionBatch` 200, `deletionChunk` 100 |
| N chats due together | about `2000 / N` rows/s (200 rows, N calls, 10 calls/s) | claim is global by `delete_at`; chunks are per chat |
| After C-14 (`deletionBatch` 1000) | about `10000 / N` rows/s, capped at 1000/s | same arithmetic |
| Inline command/edit deletes | up to 64 in flight, then a due-now row (`deletionInlineMax`) | deletion_schedule.go:104 |
| Insert side (watcher + batcher) | 0 database calls on the dispatcher; one flush per 300 ms per process | S-03 |

**Worst case, named:** many moderately busy self-destruct groups at once. With 200 rows per claim, 50 groups at 1 message a second each insert 50 rows a second and the worker drains about 40, so the backlog grows by about 10 rows a second and every kind (a due welcome, a due service notice) waits behind the overdue rows of older `delete_at`. Raising the claim to 1000 gives about 200 rows a second for 50 groups. A single chat is never the problem (1000/s). A 429 block (the shared `fuku:delete:pace:block`) delays only deletions and releases the cycle's rows free (`deletionVerdictRetryFree` sets `cut`), so the backlog is bounded by the per-chat ceiling (20,000 rows) times the number of chats. `ClaimDue` orders by `delete_at, id`, so due self-destruct rows can delay due rows of other kinds by (overdue backlog) / (drain rate); a kind-priority claim is rejected because `ORDER BY (kind = ...)` defeats `idx_scheduled_deletions_due`, and the planner measures instead.

**Busy-group test (`TestSelfDestructBusyGroup`, real worker, real pacer, real Redis in miniredis, hand-written fake client):** (a) one chat, 3000 messages through the real dispatcher with self-destruct on and a short delay: all offered rows are written (`shed == 0`), the dispatcher goroutines never wait (`dispatcher.CurrentUsage() == 0` after the burst and every update returned before the first flush), the fake's `deleteMessages` calls are at most `ceil(3000/100)` plus fallbacks, the real 100 ms pacer interval is used so the elapsed time is at least `(calls - 1) * 100 ms` and below the cycle budget, and no `deleteMessage` per-ID call occurs; (b) 40 chats x 25 rows due together: calls at most `ceil(rows per chat/100)` summed per cycle (40 per cycle) and the drain finishes within `ceil(1000/deletionBatch)` cycles; (c) in the same run a `WrapCommand` command in an unrelated chat is answered: its reply request is sent while the worker is draining, and the reply path touches no `fuku:delete:pace:*` key; (d) the fake answers 429 `retry_after` to `deleteMessages`: replies keep flowing, only deletions wait, rows stay pending and nothing is dropped; (e) the ceiling shrunk to 50 (package variable): entries beyond it are shed, nothing beyond the ceiling is written, and the kept messages are not deleted. Assertions are on call logs and row states, not wall-clock sleeps, except the one pacer-interval lower bound.

### S-07. Exclusions (question 7; DEL-07, Pitfall 15)

Three layers, as Phase 16, each with its test. (1) **Call sites are opt-in**: the wrapper, the edit handler and the batcher are the only new producers; none is in staff, lockdown, captcha or log code; the bot's own messages never arrive as updates and `raidFloodSenderCounts` rejects `b.Id` anyway; lockdown alerts, captcha challenges, staff cards and log posts are **bot messages**, so no member rule can name them (the member kinds only ever carry a member's `message_id` taken from the update). (2) **Writers refuse a Staff Group**: `deletionAct` (`latched || deletionIsStaffChat(a.ChatID)`, `deletion_schedule.go:362`), `deletion.Schedule` (`queue.go:91`), the batcher's per-chat Staff read, and the worker's per-chat Staff read (`deletion_worker.go:307-318`). (3) **The page is not drawn in a Staff Group** (`Hidden`, `settingsDelStaffGate` first in `dl`, `dr`, every control and `dy`). Tests: `TestMemberDeletionStaffGroupNeverTouched` (each of the three rules in a Staff Group chat, a seeded rule row, a failing Staff read: no row, no delete request), `TestMemberDeletionExcludedMessages` (a Staff Group card/summary, a lockdown alert copy, a captcha challenge and a log post sent through their real senders with all three rules on: no row, no delete, also after a worker cycle), the extended `TestDeletionKindsNeverStaffLockdownCaptchaLog`, and `TestMemberKindsNeedDeleteRight` (C-04).

### S-08. Test and CI strategy (question 8)

- New SQL that needs the PostgreSQL integrity list (`Makefile` `test-postgres-integrity`, `./fuku/db/deletion` is already in the package list): `TestScheduleBatchKeepsEarliest` (the multi-row upsert and the later-batch earliest, also concurrent callers), `TestScheduleBatchDuplicateKey` (dedupe in Go; the raw duplicate statement is the PostgreSQL error above), `TestCountPending` (the bounded count), `TestDropPendingMessage` (exact kind, state `pending`, other chats and `acting` rows untouched). All four also run on SQLite in `make test`. No migration test is added (no migration). Add the four names to the `-run` list in the same commit.
- Harness: no new model, so no `AutoMigrate` list change (`ScheduledDeletion` and `DeleteRule` are already in `fuku/modules/test_harness_test.go`, `fuku/db/deletion/testmain_test.go`). The dispatcher helpers of the deletion tests build `ext.NewDispatcher(&ext.DispatcherOpts{MaxRoutines: -1})` (BaseProcessor) [VERIFIED: deletion_env_test.go:90]; add `Processor: newCommandDeletionProcessor(ext.BaseProcessor{})` to `newDeletionEnv` and a second constructor with `tracing.TracingProcessor{}` for the polling-context test (C-02). Toolkit to extend: `deletionE2E` (`newDeletionE2E`, `deliver`, `command`, `deleted`, `wantRow`, `replicaBot`), `newDeletionEnv`, `deletionShortestRules`, the fixed `deletionNow`, `withFastDeletionPacer`, `deletionCleanup`, `staffCleanup`, the settings toolkit (`newSettingsEnv`, `press`, `settingsDelEnv`). A real-pacer variant `withRealDeletionPacer` is new (the busy-group test).
- Hygiene (Phase 16 rules): fresh IDs from `uniqueLinkOwnerID()` and `uniqueModuleChatID()`; `t.Cleanup` deletes the `users`, rules and queue rows and the `delete_rules` cache keys; the batcher is restored with a `restoreMemberDeleteBatcher` helper (a test that calls `StopDeletionWorker` must, like `restoreRaidAIPool`); `t.Cleanup(deletionInline.wait)` is registered last; no `t.Skip`, no `t.Parallel` with the clock swap; assertions on observable behaviour (a delete request sent, a row persisted, a key dropped), never on source substrings.
- The registry guard (C-01) is a `go/parser` test over `fuku/modules/*.go`; it is a source scan only for **where** `handlers.NewCommand` may appear, never an assertion on a literal.

### S-09. Live checks for `17-SPIKE.md` (owner steps, deferred under the standing choice)

1. **Edited-message noise**: how often a link preview filling in, a caption touch, a reaction or a poll vote arrives as `edited_message`, and with what `edit_date` (roadmap flag). The code keeps on a missing or equal `edit_date`.
2. **Bare `/word` in a multi-bot group**: with a second bot that answers the same bare command, is its reply lost when the command is deleted first, and is the `message to be replied not found` text what the Bot API answers (the failure is only documented by the `allow_sending_without_reply` comment).
3. **Anonymous-admin command deletion**: the proof tap, the command deleted after the re-entered command ran, the message left when no tap comes.
4. **Pinned messages seen by the watcher**: a `pinned_message` service message names the message (`message_id` of the nested object) for a recent and for an old message, and the queued row is dropped.
5. **`deleteMessages` at busy-group volume**: 100 IDs per call with some already deleted, the rate Telegram tolerates before a 429 on a group, the 429 `retry_after` value.
6. A claimed batch at the moment of Off (the `acting` window), phone-width cells in hi, ru and uk for the new names, the native-speaker read of the 18 keys, and a second replica over one database (the batcher on both).

## Architecture Patterns

### System architecture diagram

```
Telegram update (polling Dispatcher.Start / webhook processWebhookUpdate)
        |
        v
Dispatcher.ProcessUpdate -> commandDeletionProcessor.ProcessUpdate        [new, outside TracingProcessor]
        |   before := ctx.Update
        |-> tracing.TracingProcessor -> iterate groups:
        |      -10 captcha hold (ends the update for a pending member's message)
        |      -8  MemberDeletion:  new message -> pin drop | exemptions | rule | batcher.Offer (non-blocking)
        |                           edited msg  -> exemptions | grace | deletionAct(delay 0)
        |      -7 .. -1 guards, counters, users
        |       0  commands (WrapCommand / raw NewCommand / StaffActions / /cancel)   -> replies sent here
        |       1..12 settings door, aispam, antiflood, locks, blacklists, filters, ...
        v
   post-step (fresh ctx, never tracing.UpdateContext):
        msg := ctx.Update.Message   (swapped by verifyAnonymousAdmin after a tap)
        group chat? registered name (helpers registry)? real-user EffectiveSender? rule member:commands?
        -> deletionAct(delay 0): negative chat, latch, Staff Group refusal, inline delete (pacer) or a due-now row

batcher goroutine (started by StartDeletionWorker, joined in StopDeletionWorker's 3 s wait)
   channel(4096) -> every 300 ms / 100 entries: dedupe -> per chat: Staff read, rule check, exact ceiling count
   -> deletion.ScheduleBatch (one multi-row upsert, earliest delete_at) -> wake worker only if due now

deletion worker (unchanged, + rule screen for member:selfdestruct, batch 1000)
   claim -> per chat: latch (NeedsDeleteRight), Staff read, rule screen -> deleteMessages(<=100) under the pacer
```

### Recommended project structure (additions only)

```
fuku/db/deletion/kinds.go        # + 3 kinds, IsMemberKind, NeedsDeleteRight, ValidateRuleDelay, ValidateEntryDelay
fuku/db/deletion/queue.go        # + ScheduleBatch, CountPending, DropPendingMessage
fuku/utils/helpers/command_registry.go   # NewBotCommand shim, IsBotCommand
fuku/modules/command_deletion.go         # processor wrapper, botCommandName, afterUpdate
fuku/modules/member_deletion.go          # module MemberDeletion, group -8, edit handler, watcher, exemptions
fuku/modules/member_deletion_batcher.go  # bounded batcher, flush, shed counters, stop
fuku/modules/deletion_kinds.go           # + deletionMemberRows and the optional row fields
fuku/modules/settings_deletion.go        # + section m, dy, words struct
```

### Pattern: Processor wrapper (skeleton; every value is quoted in the findings above)

```go
// Source: gotgbot ext/processor.go (interface), fuku/utils/tracing/processor.go, fuku/modules/bot_updates.go:231-238
type commandDeletionProcessor struct{ Inner ext.Processor }

func (p commandDeletionProcessor) ProcessUpdate(d *ext.Dispatcher, b *gotgbot.Bot, ctx *ext.Context) error {
	err := p.Inner.ProcessUpdate(d, b, ctx) // no defer: an unrecovered handler panic skips the delete
	if ctx != nil && ctx.Update != nil {
		commandDeletionAfter(b, ctx)        // own recover, own fresh context, never tracing.UpdateContext
	}
	return err
}
```

### Anti-patterns to avoid

- **Calling `deletionAct`/`deletion.Schedule` per message from the self-destruct watcher** (two reads and a write per message on the dispatcher goroutine, C-07).
- **Reading `msg.From` or `ctx.Message` in the wrapper** (C-02) or **`tracing.UpdateContext(ctx)` after the tracing processor returned**.
- **A second command list** (C-01), or a regex source scan as the runtime registry (it misses table-driven names).
- **Adding the watcher at group -5 or -2** (AGENTS.md), or letting any new handler return `nil`.
- **Putting the per-kind rule table into `Schedule`** (C-05).
- **Re-recording a Phase 16 test to make a change pass**; the legitimate updates are listed in C-12.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Deciding whether the bot handles a command | a hand-written command list or a regex over the source | the recording shim (C-01) + a differential test against gotgbot's own `Command.CheckUpdate` | the matcher has caption, entity-less, case and `@bot` rules |
| Admin exemption | a new admin cache | `cachedAdminStatus` + `userIsFloodExempt` (bounded fallback assuming admin) | already race-safe and safe-direction |
| Approvals exemption | a DB read per message | `chat_status.IsApprovedForUpdate` (memoised) | |
| Sender shape | new filters | `raidFloodSenderCounts` + `ViaBot`/`IsBot` | one definition of "a real user" |
| Pacing and 429 handling | a limiter in the batcher | `deletionPaced` for deletes; the batcher makes database writes only | one fleet-wide budget |
| Queue writes | per-message inserts | `ScheduleBatch` with `leastExpr` | earliest-wins semantics already verified |
| Confirm card | a new card | `settingsCardRows`, `settingsCardExpired` | Phase 13 to 16 precedent |
| Keyboard layout and state words | per-row branches | the optional row fields (M31) | no identity branches |

## Common Pitfalls

1. **The registry misses table-driven or future commands.** *Avoid:* the shim is the only way to construct a command handler; the guard test fails the build otherwise; the docs generator regex follows it.
2. **Anonymous-admin commands skipped or deleted at the wrong time.** *Avoid:* C-02 (`ctx.Update.Message`, `ctx.EffectiveSender`); test the three states (before the tap, after the tap, no tap).
3. **A cancelled tracing context in polling.** *Avoid:* a fresh context; the polling-style test with `TracingProcessor` inner.
4. **A latch that never covers member kinds** (C-04): a rightless group costs a failed call per command forever.
5. **Duplicate key inside one batch fails the whole statement on PostgreSQL** and SQLite hides it. *Avoid:* dedupe in Go; a PostgreSQL-integrity test.
6. **Off races the batcher** (C-06): flush-time and worker-time rule screens.
7. **The ceiling shed is silent**: a rate-limited Warn per chat per hour (chat ID only) and an AGENTS.md table (C-13).
8. **Many-chat drain collapses** (C-14): batch 1000, assertion per chat.
9. **A pinned message queued in the same 300 ms as its pin**: documented window or a tombstone.
10. **Edit noise** (D-04): any later edit counts; the spike measures it; `EditDate` zero keeps.
11. **Docs drift**: `make generate-docs` and `make check-docs` after the shim, AGENTS.md updated in the same commits (Handlers: group -8; Callbacks: `dy`, section `m`; Data: no new key, the member kinds; Subsystem traps: the three rules, the limits of C-13 and S-03; Testing: the new families).

## Code Examples

### Bulk upsert (statement shape verified on PostgreSQL 16; GORM spelling follows `queue.go:110-115`)

```go
// Source: fuku/db/deletion/queue.go (Schedule), verified statement in S-03
err := db.DB.WithContext(ctx).Clauses(clause.OnConflict{
	Columns:   []clause.Column{{Name: "chat_id"}, {Name: "message_id"}},
	DoUpdates: clause.Set{{Column: clause.Column{Name: "delete_at"}, Value: leastExpr(db.DB)}},
}).Create(&rows).Error // rows deduplicated by (chat_id, message_id) first: PostgreSQL error 21000 otherwise
```

### Exact bounded ceiling count

```go
// Source: verified plan, S-03 (Bitmap Index Scan on uk_scheduled_deletions_message, 2.7 ms at 20,002 rows)
var n int64
err := db.DB.WithContext(ctx).Raw(
	`SELECT count(*) FROM (SELECT 1 FROM scheduled_deletions WHERE chat_id = ? AND kind = ? LIMIT ?) s`,
	chatID, kind, ceiling).Scan(&n).Error
```

## State of the Art

| Old approach | Current approach | Impact |
|--------------|------------------|--------|
| One synchronous queue write per bot message (Phase 16) | bounded batcher + multi-row upsert for member messages | the dispatcher never waits on a write at 50+ messages a second |
| `latch scope service` meant service messages | means "cannot delete other people's messages" | covers members' commands, edits and rows |
| Handlers do all deletion work | a Processor wrapper owns "after the answer" | one seam for raw, wrapped and anonymous commands |

No deprecated library is involved.

## Runtime State Inventory

Not a rename or migration phase; omitted. (Existing state touched without change: `chat_delete_rules` rows gain three kinds; `scheduled_deletions` rows gain `member:selfdestruct` (and due-now `member:commands`/`member:edited` fallbacks); both outside backup, export, import and reset by `TestDeletionTablesStayOutOfBackup`.)

## Proposed plan decomposition (small plans, tracer first)

| Plan | Wave | Content (about 3 tasks) | Depends on | Owns (files) |
|------|------|-------------------------|-----------|--------------|
| 17-01 kinds and data | 1 | (1) three kinds, `IsMemberKind`, `NeedsDeleteRight`, `ValidateRuleDelay`/`ValidateEntryDelay` wired into `SetRule`, `Schedule`, `applyDeleteRule`; (2) `ScheduleBatch`, `CountPending`, `DropPendingMessage` + tests; (3) Makefile integrity list, kind-registry test updates (17 to 20) | none | `fuku/db/deletion/*`, `fuku/modules/deletion_apply.go` (validate call), `Makefile`, `deletion_kinds_test.go` |
| 17-02 command registry | 1 | (1) `helpers.NewBotCommand` + `IsBotCommand`; (2) mechanical rewrite of the 134 sites + `register()`; (3) guard `go/parser` test, docs generator regex, `make generate-docs`/`check-docs` | none | ~25 files in `fuku/modules`, `fuku/utils/helpers`, `scripts/generate_docs/parsers.go`, generated docs |
| 17-03 tracer | 2 | (1) `commandDeletionProcessor`, `botCommandName` + differential test, wiring in `main.go`; (2) rule seeding, the real dispatcher test: a `WrapCommand` command and a raw command answered then deleted, reply-before-delete ordering, with `BaseProcessor` and `TracingProcessor` inners; (3) `deletionE2E` additions | 17-01, 17-02 | `command_deletion.go`, `main.go`, `deletion_env_test.go` |
| 17-04 command edge cases and latch | 3 | (1) anonymous-admin three states, bot/`sender_chat`/service senders, Staff Group, held and fed-banned wasted call; (2) `NeedsDeleteRight` at the four sites + the rightless-group test; (3) DEL-07 test for commands | 17-03 | `deletion_schedule.go`, `deletion_worker.go`, tests |
| 17-05 edits | 3 | (1) module `MemberDeletion`, group -8, raw edit handler, grace arithmetic; (2) exemption order and helpers (admin, approvals, pending captcha, `ViaBot`); (3) tests incl. edit at 20 s kept / 90 s deleted, noise cases, Staff Group | 17-01 | `member_deletion.go` (edit half), tests |
| 17-06 self-destruct write path | 4 | (1) batcher (channel, flush, shed counters, ceiling, rule re-check); (2) joined into `StartDeletionWorker`/`StopDeletionWorker`, shutdown test; (3) worker rule screen + `deletionBatch` 1000 | 17-01 | `member_deletion_batcher.go`, `deletion_worker.go` |
| 17-07 self-destruct watcher | 5 | (1) watcher, exemptions, pin drop; (2) pending-captcha, admin, approved, via-bot, album tests; (3) `TestSelfDestructBusyGroup` | 17-05, 17-06 | `member_deletion.go` (watcher half), tests |
| 17-08 page | 4 (parallel with 17-06/07) | (1) `deletionMemberRows`, row fields, section `m`; (2) words struct, `dy`, the card; (3) page tests (`TestSettingsDeletionMember*`, drift, authority, bytes, fit, privacy) | 17-01 | `deletion_kinds.go`, `settings_deletion.go`, settings tests |
| 17-09 locales | 6 | the 18 keys + changed intro in 8 files, `settingsLocaleLimits`, label-length groups, `TestDeletionKindKeysExist`, native-read notes | 17-08 | `locales/*.yml`, `fuku/i18n/*_test.go` |
| 17-10 closing | 7 | (1) `TestMemberDeletionEndToEnd` through the real dispatcher with the page; (2) AGENTS.md, README, docs, `17-SPIKE.md`; (3) the phase gate (`make test`, lint, translations, docs, PostgreSQL integrity) | all | `AGENTS.md`, README, docs, spike file |

`use_worktrees` is false, so `locales/*.yml`, `AGENTS.md`, `main.go` and `Makefile` are single-owner per wave (`main.go`: 17-03 only; `Makefile`: 17-01 only; `AGENTS.md`: 17-10 only, with short staged notes in the earlier plans' summaries).

## Validation Architecture

### Test framework

| Property | Value |
|----------|-------|
| Framework | Go `testing`, `-tags testtools -race`, testify assertions only, `internal/testdb.Run` (SQLite), miniredis, hand-written `gotgbot.BotClient` fakes |
| Config file | none (Makefile targets) |
| Quick run command | `go test -tags testtools -race -count=1 -run '^(TestCommandDeletion\|TestBotCommand\|TestMemberEdited\|TestMemberSelfDestruct\|TestSelfDestructBusyGroup\|TestMemberDeletion\|TestSettingsDeletionMember\|TestMemberKinds)' ./fuku/modules ./fuku/db/deletion` |
| Full suite command | `make test` (the only valid full run), plus `make test-postgres-integrity`, `make lint`, `make check-translations`, `make check-docs` |

### Phase requirements to test map

| Req | Behaviour | Test type | Automated command | File exists? |
|-----|-----------|-----------|-------------------|--------------|
| DEL-03 | a registered command (wrapped and raw) is answered, then deleted; reply request before delete request | integration (real dispatcher, fake client) | `go test -tags testtools -race -count=1 -run '^TestCommandDeletion' ./fuku/modules` | Wave 0 (17-03) |
| DEL-03 | `@otherbot`, unregistered `/word`, edited command, bot sender, `sender_chat`, service ID, private chat: no delete | integration | `-run '^TestCommandDeletionKeeps'` | Wave 0 |
| DEL-03 | anonymous admin: nothing at the first update, delete after the tap ran, none without a tap | integration | `-run '^TestCommandDeletionAnonymousAdmin'` | Wave 0 |
| DEL-03 | with `TracingProcessor` inner (polling context) the delete still happens | integration | `-run '^TestCommandDeletionPollingContext'` | Wave 0 |
| DEL-03 | `botCommandName` equals gotgbot's matcher over a table; every `NewCommand` goes through the shim | unit | `-run '^(TestBotCommandNameMatchesGotgbot\|TestNoRawNewCommand)'` | Wave 0 |
| DEL-04 | edit at 20 s kept, at 90 s with a 60 s grace deleted; admin, approved, via_bot, `sender_chat`, forward, held captcha, bot, older than 47 h, `EditDate` 0 kept | integration | `-run '^TestMemberEdited'` | Wave 0 (17-05) |
| DEL-05 | queued at `date + delay`, admin/approved/pinned/held/service/bot messages never queued; Off drops waiting rows; flush and worker rule screens; ceiling shed | integration + repository | `-run '^TestMemberSelfDestruct'` ; `go test -tags testtools -race -count=1 ./fuku/db/deletion` | Wave 0 (17-06, 17-07) |
| DEL-05 | busy group keeps pace; replies unaffected; 429 delays only deletions | integration | `-run '^TestSelfDestructBusyGroup'` | Wave 0 |
| DEL-05 | batcher final flush inside `StopDeletionWorker`'s 3 s; shed after stop | integration | `-run '^TestShutdownEndsMemberDeleteBatcher'` | Wave 0 |
| DEL-07 | Staff Group, alert copy, captcha challenge, staff card, log post never queued or deleted | integration | `-run '^(TestMemberDeletionStaffGroupNeverTouched\|TestMemberDeletionExcludedMessages)'` | Wave 0 |
| DEL-08 | rightless group: one live check then silence for each rule | integration | `-run '^TestMemberKindsNeedDeleteRight'` | Wave 0 |
| PAGE (D-14 to D-18) | the C17-1 to C17-16 checks of the UI-SPEC | integration | `go test -tags testtools -race -count=1 -run '^(TestSettings\|TestLoadSettings)' ./fuku/modules ./fuku/i18n` | Wave 0 (17-08, 17-09) |
| SQL on PostgreSQL | batch upsert earliest, duplicate dedupe, bounded count, pin drop | integrity | `make test-postgres-integrity` (four names added) | Wave 0 |

### Sampling rate

- **Per task commit:** the quick run command for the files touched.
- **Per wave merge:** `go test -tags testtools -race -count=1 ./fuku/db/deletion ./fuku/utils/helpers` and the deletion and settings families of `./fuku/modules`.
- **Phase gate:** `make test`, `make test-postgres-integrity`, `make lint`, `make check-translations`, `make check-docs` green before `/gsd-verify-work`.

### Wave 0 gaps

- [ ] `fuku/utils/helpers/command_registry_test.go` and `fuku/modules/command_registry_guard_test.go` (shim, differential test, guard) - 17-02/17-03.
- [ ] `fuku/modules/command_deletion_test.go`, `member_edited_test.go`, `member_selfdestruct_test.go`, `member_selfdestruct_busy_test.go` - 17-03 to 17-07.
- [ ] `fuku/db/deletion/queue_batch_test.go` - 17-01.
- [ ] `withRealDeletionPacer`, `restoreMemberDeleteBatcher`, a `Processor` option in `newDeletionEnv`, and the `TracingProcessor` variant of the harness.
- Framework install: none.

## Security Domain

`security_enforcement` is not disabled in `.planning/config.json` (the key is absent), so this section applies.

### Applicable ASVS categories

| ASVS category | Applies | Standard control |
|---------------|---------|------------------|
| V2 Authentication | no (Telegram authenticates senders) | - |
| V3 Session Management | no (anonymous-admin proof reuses its 20 s cache entry) | - |
| V4 Access Control | yes | the press pipeline (live `getChatMember`, owner gate), `settingsRightChangeInfo` on every choice and on `dy`, `settingsDelStaffGate` first, the admin/approved exemptions fail toward keeping |
| V5 Input Validation | yes | `callbackcodec` strict decode, `settingsDelPresetValidator`, `settingsCardExpired`, `ValidateRuleDelay`/`ValidateEntryDelay` before any write, `msg.Chat.Id` from the update only |
| V7 Error Handling and Logging | yes | no message text, ID, claim token or name in any log line; chat IDs and counters only |
| V8 Data Protection | yes | no message text or user ID is stored (`scheduled_deletions` has none), rows never in backup |
| V11 Business Logic | yes | the 20,000 ceiling, the bounded batcher, the 47 h skip, the pacer, the latch |

### Known threat patterns (STRIDE)

| Pattern | STRIDE | Mitigation |
|---------|--------|------------|
| Spoofed sender: `sender_chat`, `via_bot`, forward, automatic forward, Group Anonymous Bot ID | Spoofing | sender read from `ctx.EffectiveSender`; `raidFloodSenderCounts`; `ViaBot`; service IDs refused (C-02, S-02) |
| Forged callback on `dy`, `dl`, `dr`, `st` (extra keys, `t=mc` with `dy`, `v=n`, `e` junk, `c` and `u` together) | Tampering | step-1 validation before any I/O, `ConfirmKey` check, canonical digits, expiry; the card Yes writes the absolute value it named only after the right is judged again |
| Message-ID or chat-ID confusion (pin object's chat, anonymous cache chat, callback `c`) | Tampering | chat from the update's own message; the pin drop uses the service message's chat; `verifyAnonymousAdmin` already refuses mismatched chats |
| Deleting another bot's command (`/x@otherbot`, unregistered `/word`) | Tampering | `botCommandName` + registry; bare-name race is accepted and surfaced (A17-1, spike check 2) |
| Deleting admin content through a stale admin cache | Tampering / Elevation | `userIsFloodExempt` bounded live fallback assuming admin; `adminCacheAutoUpdate` invalidates on promotion; limit stated: exemptions are judged at send time, rows hold no user ID, so a later promotion or approval does not rescue a queued message |
| Queue growth (a flood fills the table) | Denial of service | bounded channel, per-chat ceiling 20,000 (exact count), global shed counter, inline cap 64, pacer; shed always keeps the message |
| Deletion storm starving replies or other kinds | Denial of service | own pacer keys, the batcher touches the database only, `deletionBatch` 1000, rule screens drop late rows |
| Staff Group or alert/captcha/log item deleted | Information disclosure / Tampering | three layers, S-07 tests |
| Replay of the anonymous-admin proof | Repudiation / Tampering | button deleted at the first tap, 20 s cache; a second run deletes an already deleted message (quiet) |
| A group where the bot cannot delete hammered with failing calls | Denial of service | latch generalised (C-04), one live check per chat per 10 minutes |

## Assumptions Log

| # | Claim | Section | Risk if wrong |
|---|-------|---------|---------------|
| A1 | SQLite's bundled driver accepts a 100-row, about 1000-variable multi-row INSERT (default limit 32766) | S-03 | a smaller batch size (50) is the fallback; a test inserts a full batch |
| A2 | Sending a reply to a deleted message fails with a "message to be replied not found" style 400 unless `allow_sending_without_reply` is set | S-01 | only affects the bare-command race with another bot; spike check 2 measures it |
| A3 | A cycle of 200 to 1000 rows takes 0.25 to 1 s on a healthy link (timing of the drain model) | S-06 | the formulas count calls, not seconds; the busy-group test asserts counts |
| A4 | Link-preview and caption touches arrive as `edited_message` sometimes, with an `edit_date` | S-02 | the rule's stated limit already covers it (D-04); the spike measures |
| A5 | GORM's `Create` of a slice with `OnConflict` is one INSERT statement | S-03 | the repository test counts statements; `CreateInBatches` is the fallback |
| A6 | A 2 s in-process memo of the rule would be safe because of the worker-time screen | S-03 | optional; only if the busy-group test shows the Redis read matters |

## Open Questions

1. **Bare commands that another bot also handles (A17-1).**
   - Known: the matcher cannot tell which bot a bare `/help` was meant for; deleting after our answer may beat a slow second bot's reply.
   - Unclear: how often owners run such bots together with this one, and the exact Bot API answer.
   - Recommendation: ship D-01 as decided, state the limit in AGENTS.md, run spike check 2, and only then consider a short grace (a queue delay for member commands) as a follow-up.
2. **Ceiling versus long delays (A17-6).** C-13's table: the 24 h preset on a group above 0.23 messages a second sheds. Recommendation: keep 20,000, publish the table, offer a per-preset ceiling table later.
3. **`deletionBatch` 1000 (C-14).** Recommendation: take it in 17-06 with the multi-chat assertion; revisit only if the live `deleteMessages` check shows a lower safe rate.
4. **Exemptions judged at send time only.** A member approved or promoted after sending still loses queued messages. Recommendation: accept and document; a user column on the queue would need a migration and is not worth it.
5. **Edited messages with `edit_date` equal to `date`.** Treated as within the grace (kept). Recommendation: keep.

## Environment Availability

| Dependency | Required by | Available | Version | Fallback |
|------------|-------------|-----------|---------|----------|
| Go toolchain | build and tests | yes | go1.26.0 linux/amd64 | - |
| PostgreSQL 16 (local, for the research probe and `make test-postgres-integrity`) | integrity tests | yes (binaries under `/usr/lib/postgresql/16/bin`, started on a private socket for this research and stopped) | 16 | CI runs it; the SQLite suite covers the logic |
| Docker | `make build` only | yes | 29.9.2 | not needed for tests |
| core.telegram.org (Bot API pages) | live-behaviour citations | **no** (the host did not resolve from this session; it has to be allowed in the session's network settings to be fetched) | - | gotgbot doc comments cited instead; live checks in S-09 |

**Missing dependencies with no fallback:** none. **With fallback:** the Bot API site (above).

## Sources

### Primary (HIGH confidence)
- Repository files read this session: `fuku/utils/tracing/processor.go`, `main.go` (431), `fuku/utils/httpserver/server.go` (231-240, 363-372), `fuku/config/config.go` (479-482), `fuku/utils/helpers/command_pipeline.go`, `decorators.go`, `fuku/modules/{bot_updates.go (120-290), anonymous_admin_router.go, captcha.go (2141-2233), deletion_schedule.go, deletion_worker.go, deletion_service.go, deletion_kinds.go, deletion_apply.go, settings_deletion.go, settings_card.go, raid_flood.go, antiflood.go, aispam.go, registry.go}`, `fuku/db/deletion/{kinds.go, queue.go, rules.go}`, `migrations/20261110120000_add_scheduled_deletion.sql`, `Makefile`, `scripts/generate_docs/{main.go, parsers.go}`, the Phase 16 and 17 planning files, `AGENTS.md`.
- gotgbot v2.0.0-rc.36.0.20260919140833-240296efadb4: `ext/dispatcher.go`, `ext/processor.go`, `ext/handler.go`, `ext/context.go`, `ext/handlers/command.go`, `ext/handlers/message.go`, `sender.go`, `custom_helpers.go`, `gen_types.go`, `gen_methods.go` (doc comments).
- A throwaway PostgreSQL 16: the migration applied, multi-row upsert, duplicate-key error, `EXPLAIN (ANALYZE)` of the count and the claim query (server stopped afterwards).

### Secondary / Tertiary
- Bot API behaviour beyond the gotgbot doc comments (edited-message noise, reply to a deleted message, `deleteMessages` rates) is `[ASSUMED]` and listed as live checks.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH, nothing new.
- Architecture: HIGH, each seam read in the library and the repository.
- Pitfalls: HIGH for code-derived ones (C-01 to C-14), MEDIUM for Telegram-side behaviour.

**Research date:** 2026-11-12
**Valid until:** 2026-12-12 (stable; revisit if gotgbot is upgraded, because `TestServiceKindsCoverMessageFields` and the pinned `Processor`/`Command` behaviour are version-bound)
