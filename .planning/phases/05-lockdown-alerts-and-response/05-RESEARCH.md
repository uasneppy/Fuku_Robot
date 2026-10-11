# Phase 5: Lockdown Alerts and Response - Research

**Researched:** 2026-10-11
**Domain:** Go Telegram bot (gotgbot v2, GORM/PostgreSQL, Redis): alert posting, inline-button response flows, DB-driven worker extensions, on top of the Phase 4 lockdown
**Confidence:** MEDIUM-HIGH (codebase facts HIGH; Telegram behaviour that could not be fetched from core.telegram.org is cited from the gotgbot doc comments, which mirror the Bot API, or tagged `[ASSUMED]`)

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**The alert itself**
- **D-01:** The alert goes to the linked Staff Group (if any) and to the group's log channel. Nothing extra is posted in the locked group, because the `/lockdown` reply is already there. A group with neither destination gets no alert.
- **D-02:** The burst of a manual `/lockdown` has two parts:
  - every joiner the lockdown recorded (`chat_lockdown_joiners`);
  - anyone who joined in the 10 minutes before the lockdown, read from antiraid's Redis join list (`alita:antiraid:joins:<chat>`). This part is best effort and is empty without Redis.

  Phase 6's automatic triggers will define their own burst.
- **D-03:** The alert lists up to 10 accounts, each with name, @username and ID, followed by "and N more". Bots are marked.
- **D-04:** The alert stays as posted. Fresh counts come in the reminders. At the lift, every copy of the alert and of the reminders is edited to "Lifted by X at T" and loses its buttons.

**The "Lift lockdown" button**
- **D-05:** Only a live creator, or an administrator with `can_restrict_members`, of the locked group may press it. This is the same check as `/unlockdown` (`requireLockdownAuthority`): a live `getChatMember`, never the admin cache.
  - Being in the Staff Group grants nothing by itself.
  - A press from an anonymous admin or a channel identity is refused with "post as yourself".
- **D-06:** Tapping shows a confirm card ("Lift lockdown in {group}? Confirm / Cancel"). Only the person who tapped can confirm.
- **D-07:** If the lockdown is already lifted or lifting, the press is answered "Already lifted by X at T", nothing changes, and that alert is edited to its lifted state.
- **D-08:** The lift runs exactly the `/unlockdown` path:
  - It goes through `lockdownBeginLift`, which the Phase 4 rule says is the only entry to `BeginLift`.
  - It restores the snapshot first, then runs the paced unbans and posts one tally.
  - The tally is still posted in the locked group. The alert copies are edited to show "Lifted by X".
  - The recorded lifter is the presser.

**"Ban N recent joiners"**
- **D-09:** The targets are the whole D-02 burst list, minus:
  - live creators and administrators of the group;
  - approved users;
  - the bot itself.

  Bots in the list are banned.
- **D-10:** The ban is permanent.
  - For a lockdown joiner, the row is marked as deliberately banned, so the lift keeps the ban (Phase 4 D-05).
  - Calls go through the lockdown pacer and use the same "never shorten or lift a deliberate ban" check as the lift: the live status decides each call, and a joiner already kicked by someone else's ban is left alone.
- **D-11:** Tapping shows a confirm card ("Ban N accounts in {group}?"). Only the presser can confirm.
  - The result reports banned, skipped (with reasons) and failed.
  - The group's log channel gets one `#LOCKDOWN_BAN` post.
  - These bans are not part of the staff audit record and cannot be undone.
- **D-12:** The button stays until the lift. A later press bans only accounts that no earlier press handled, and N counts only those. Only one ban run per lockdown can go at a time.

**Invite link and reminders**
- **D-13:** The alert shows the invite link most of the burst used, with how many joined through it, plus "+N other links" when there are others. It is shown only when Telegram reported a link (the joiner rows' `invite_link`).
- **D-14:** "Revoke link" may be pressed only by a live creator, or an administrator with `can_invite_users`, of that group.
  - Tapping shows a confirm card, then calls `revokeChatInviteLink`.
  - The alert then says "Link revoked by X".
  - If Telegram refuses, the answer says why.
- **D-15:** A reminder is posted every 60 minutes while the group stays locked.
  - The database-driven worker sends it, with a conditional update so only one replica posts each reminder and a restart loses nothing.
  - Each reminder is a new message in the same destinations as the alert, and the previous reminder loses its buttons.
  - Reminders stop at the lift.
- **D-16:** A reminder says "Still locked since T", the reason, the number of joiners removed so far and the number still pending. It carries the same three buttons and no sample.

### Claude's Discretion
- The table and column layout for alert messages and reminder bookkeeping, following the append-only migration rules. The existing `chat_lockdowns.trigger_kind` and the joiner rows' `invite_link`, `first_name`, `username` and `is_bot` are already there for this.
- The callback namespace and the token scheme, within the 64-byte `callbackcodec` cap. Callback data carries only IDs or tokens.
- The exact wording of the alert, reminder, confirm and result texts in all 7 locales.

### Deferred Ideas (OUT OF SCOPE)
- A persistent PostgreSQL log of recent joins in every group, so the pre-lockdown part of the burst also works without Redis. It was considered for D-02 and declined for now, because it costs one write per join in every group. Phase 6 may revisit it with its join-burst detector.
- Undo for "Ban N recent joiners" through the staff audit. Declined for v1 (D-11).
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| LOCK-11 | Each lockdown posts one alert to the Staff Group (if linked) and the log channel with trigger, counts, a joiner sample, "Lift lockdown" and "Ban N recent joiners" | Exactly-once gate `BeginAlert` (conditional update + slot inserts in one transaction), worker-posted slots, stored message copies (Patterns 1, 2). Finding F1 changes how the "sample" is sourced. |
| LOCK-12 | An admin of the locked group can press "Ban N recent joiners"; admins and approved users excluded; result reported | Stateless confirm card, burst table, per-account claims, pure decision table `decideLockdownResponseBan`, DB-driven ban run (Patterns 4, 5) |
| LOCK-13 | The alert shows the invite link the burst used; "Revoke link" usable only by admins of that group | Top-link query on joiner rows plus tracker snapshot; revoke only works for bot-created links (Finding F2); live `can_invite_users` check (Pattern 6) |
| LOCK-14 | While locked, the alert is re-posted periodically until lifted | Reminder slots keyed `(lockdown, kind, seq, dest)` in the existing worker cycle (Pattern 3) |
| LOCK-06 (finish) | Lift from the alert's button, wherever the alert is | Extract the `unlockdown` body into one shared lift function; button and command both call it (Pattern 7) |
| LOCK-01 (finish) | Removed joiners "listed on the alert" | Sample reads joiner rows plus the pre-lockdown snapshot (Pattern 2) |
</phase_requirements>

## Summary

Phase 5 adds three things on top of Phase 4's durable lockdown: (1) an exactly-once alert posted to the Staff Group and the log channel by the existing DB-driven lockdown worker, with every posted copy remembered so it can be closed at the lift; (2) three response buttons (Lift, Ban N recent joiners, Revoke link), each behind a stateless confirm card and a fresh live authority check against the locked group, never the chat the button sits in; (3) hourly reminders that use the same slot mechanism. Everything stays PostgreSQL-first so it works without Redis, matching the Phase 4 rule that a lockdown never depends on Redis.

Research found four problems with the discussed design that need an owner decision before planning is final (details in Open Questions): **F1** antiraid's Redis join list holds 60 seconds, not 10 minutes, and is not populated at all unless the group set an auto-threshold (default 0), so D-02's pre-lockdown half is empty in practice and a manual lockdown's alert, posted at t=0, would show an empty sample. **F2** `revokeChatInviteLink` works only on links the bot itself created, and Telegram shows the bot other admins' links truncated with `...`, so "Revoke link" can only ever work for a small subset of links. **F3** the Phase 3 staff run engine (`startStaffRun`) is tied to linked groups, the Redis card hash and the staff pacer and cannot run a per-user ban list, so STATE.md's "reuse it" advice does not apply. **F4** the staff confirm card needs Redis (`errStaffCardNoRedis`), which would make the Lift button dead without Redis; a stateless confirm (asker's user ID inside the callback data) avoids that.

**Primary recommendation:** Add one append-only migration with three tables (`chat_lockdown_alerts`, `chat_lockdown_burst`, `chat_lockdown_ban_runs`) and a few columns on `chat_lockdowns`; gate the alert with a single conditional update inside a transaction that also inserts the destination slots; let `runLockdownCycle` post, remind, close and ban through `lockdownPaced`; build one `lockdown|v1` callback namespace with stateless Confirm/Cancel buttons carrying the asker's ID; and extract `unlockdown`'s body into one shared lift function so the button and the command cannot diverge.

## Project Constraints (from CLAUDE.md and AGENTS.md)

The planner must verify every plan against these. CLAUDE.md imports AGENTS.md (`@../AGENTS.md`) and adds the stack and security constraints.

- **Stack:** Go 1.26.0, gotgbot v2, GORM on PostgreSQL (SQLite in tests), Redis. No new dependencies.
- **Migrations:** append-only, timestamp greater than every existing file (latest is `20261006120000_add_chat_lockdowns.sql`), one transaction per file, no top-level `BEGIN`/`COMMIT`/`ROLLBACK`, no `CREATE INDEX CONCURRENTLY`. Never `gorm.AutoMigrate` in production code. If `alita/db/migrations/runner.go` changes, `scripts/migrate_psql.sh` changes with it.
- **Data:** every write `cache.DeleteCache`s the keys it affects; the lockdown tables are read fresh and never cached, so none applies, and any cache added later needs `skipLocal` plus `DeleteCache` on every write. Lockdown tables are never part of backup/export/import/reset. Models define `TableName()`.
- **Commands and callbacks:** commands go through `helpers.WrapCommand` (Phase 5 adds no command); callbacks only through `alita/utils/callbackcodec` (`<ns>|v1|<url-encoded>`, 64-byte cap); `encodeCallbackData` returns `""` on overflow and ships a dead button, so user text goes behind a token or stays out. The new namespace must not start with `antiraid`.
- **Permissions:** `chat_status` predicates return bools and never reply; lockdown authority is a live `getChatMember`, never the admin cache or the service IDs; a failed lookup refuses.
- **Handlers:** group numbers are execution order; watchers return `ext.ContinueGroups`; handler methods use value receivers on `moduleStruct`; fire-and-forget goroutines start with `defer error_handling.RecoverFromPanic(...)`.
- **Shutdown:** the lockdown worker drain is registered after DB-close with `lockdownWorkerStopWait` (5 s) plus grace. New worker steps must be bounded per cycle and stop on `ctx.Err()`.
- **Locales:** every new key in all 7 files (`en, es, fr, hi, id, pt, ru`); copy key names from `locales/en.yml`; a typo'd key ships an empty message; `make check-translations`. The docs generator reads only `locales/en.yml`; changing `lockdown_help_msg` needs `make generate-docs`.
- **Tests:** always `-tags testtools`; real fixtures (SQLite via `internal/testdb`, miniredis, hand-written `gotgbot.BotClient` fakes), no mock libraries; assert observable behaviour, never literals or test-double internals; `make test` is the only valid full run; keep the migration-chain CI step before `make test`.
- **Never discard a DB error on a state-changing path.** `UpdateRecord` skips zero values (use `UpdateRecordWithZeroValues` or explicit `Updates(map...)` for `false`/`0`/`""`).
- **Secrets:** register any new secret with `logredact.RegisterSecret` (Phase 5 adds none).
- **Telegram limits:** fan-out respects Bot API limits and reports partial failures, never drops them; pacing is fleet-wide (`lockdownPaced`), not per replica.
- **Commits:** Conventional Commits; user-visible changes need `feat:` or `fix:`. Update `AGENTS.md` in the same commit as any change that invalidates a rule (this phase changes the "`BeginLift` only through `lockdownBeginLift` in `unlockdown`" sentence, adds the `lockdown|` callback namespace and the `alita:lockdown:recent:*` key family, and documents the new tables).
- **Workflow:** edits start from a GSD command (`/gsd-execute-phase` for planned work).

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Decide "one alert per lockdown" | PostgreSQL (conditional update in a transaction) | Worker | Must hold across replicas and restarts; Redis is not allowed to gate it |
| Post alert / reminders / closing edits | Lockdown worker (every replica) | Telegram Bot API | Retry and resume from rows; never inline in the command |
| Remember posted message IDs | PostgreSQL (`chat_lockdown_alerts`) | — | Needed at the lift, possibly days later, after restarts |
| Pre-lockdown recent joins | Redis (best-effort zset) | Handlers at group -7 feed it | Declined PG log (Deferred); empty without Redis by design |
| Button authority | Callback handler → live `getChatMember` of the locked group | PostgreSQL (lockdown row gives the chat) | Pitfall 12: never trust the pressed message's chat or the payload |
| Confirm cards | Callback data (stateless, asker ID inside) | — | Works with Redis down; Telegram client cannot forge callback data |
| Lift | Shared lift function (restore, `lockdownBeginLift`, `FinishLift`) | Worker (unbans, tally) | Same code path for command and button |
| "Ban N" execution | DB-driven ban run in the worker | Lockdown pacer (fleet-wide) | Resumable, one run per lockdown, joins the existing 5 s stop wait |
| Invite link revoke | Callback handler | Telegram `revokeChatInviteLink` | One-shot, conditional record of who revoked |

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/PaulSonOfLars/gotgbot/v2` | v2.0.0-rc.36.0.20260919140833-240296efadb4 `[VERIFIED: /root/go/pkg/mod/github.com/!paul!son!of!lars/gotgbot/v2@v2.0.0-rc.36.0.20260919140833-240296efadb4]` | `SendMessage`, `EditMessageText`, `EditMessageReplyMarkup`, `RevokeChatInviteLink`, `BanChatMember`, `AnswerCallbackQuery` | Already the bot client; all methods used below exist (`gen_methods.go:2294`, `2361`, `4689`) |
| `gorm.io/gorm` + PostgreSQL / SQLite (tests) | existing | New tables, conditional updates | Repo standard; `RowsAffected == 1` is the exactly-once idiom |
| `github.com/redis/go-redis/v9` | existing | Recent-joins zset (best effort), pacer keys | Already used by antiraid and the pacers |
| `alita/utils/callbackcodec` | in repo | `lockdown|v1|...`, 64-byte cap | AGENTS.md: only way to encode callbacks |
| `alita/utils/ratelimit` `TelegramPacer` via `lockdownPaced` | in repo | Fleet-wide pacing of every worker Telegram call | D-10; slot more than 60 s away fails at once as rate limited |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `github.com/alicebob/miniredis/v2` | v2.39.0 (CLAUDE.md) | Tracker and pacer tests | Tests only; real fixtures, no mock libraries |
| `alita/utils/actionlog` `Destination` | in repo | Resolves log channel and header, honours the category switch | Reuse for the log-channel destination (`actionlog.Destination(chat, logchannels.CategoryAdmin)`) |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Stateless confirm (asker ID in callback data) | Redis card hash like `staff_action_card.go` | Redis card gives expiry timers and terminal states but is dead when Redis is down; lockdown is explicitly Redis-independent |
| DB-driven ban run in the worker | Reuse `startStaffRun` | Engine is bound to `[]models.StaffGroupLink`, `staffActionCard`, the staff pacer and `staffActionRunsWG`; not reusable for per-user lists (F3) |
| Dedicated 10-minute recent-joins tracker | Read antiraid's zset as D-02 says | Antiraid's list is 60 s and off by default (F1) |

**Installation:** none. No new external packages.

**Version verification:** gotgbot version read from the module cache directory name and `go version` reports `go1.26.0 linux/amd64`. `[VERIFIED: shell]`

## Package Legitimacy Audit

| Package | Registry | Age | Downloads | Source Repo | Verdict | Disposition |
|---------|----------|-----|-----------|-------------|---------|-------------|
| (none added) | — | — | — | — | — | Phase uses only the standard library and dependencies already in `go.mod` |

**Packages removed due to [SLOP] verdict:** none
**Packages flagged as suspicious [SUS]:** none. No `gsd_run query package-legitimacy check` run is needed because nothing is installed.

## Architecture Patterns

### System Architecture Diagram

```
/lockdown command ──ConfirmLocked ok──▶ lockdownBeginAlert(row)         worker settle path
(lockdown.go:387)                        │  (tx: conditional UPDATE alert_started_at,   (confirmed in
                                         │   INSERT slots staff/log, INSERT pre-burst)   settleUnconfirmed)
group -7 join handlers ──ZADD──▶ Redis   │                                               │
alita:lockdown:recent:<chat> ◀───read────┘ (snapshot before the tx, best effort)         │
                                         ▼                                               ▼
                         chat_lockdown_alerts (slot rows: pending)  ◀── lockdownPlanAlerts (cycle step, finds
                                         │                              confirmed active rows with no alert yet)
                      runLockdownCycle (every replica, 2 s tick / wake)
   ┌──────────────┬────────────────────┬──────────────────────┬──────────────────────┐
   ▼              ▼                    ▼                      ▼                      ▼
 post slots   hourly reminder      close copies            ban runs              (existing: settle,
 (claim→send  slots (INSERT slot   (state != active:       (claim run, claim      ban joiners,
 →record id)  seq n, winner posts, edit "Lifted by X",     account, live check,   delete join msgs,
              old reminder loses   drop buttons,           ban permanent,         lift tally)
              buttons)             record closed)          mark kept/response_ban)
   │              │                    │                      │
   └──────────────┴──── lockdownPaced (alita:lockdown:pace:*) ┴──▶ Telegram
                                                                         ▲
Staff Group / log channel button press ─▶ lockdown|v1 callback handler ──┘
   a=lk (Lift ask) / a=bn (Ban ask) / a=rk (Revoke ask)   → live getChatMember of the LOCKED group
   a=lc / a=bc / a=rc (Confirm, carries u=<asker>)         → refuse or post confirm card (reply to alert)
   a=x (Cancel)                                            → Confirm: re-check live, then act:
        Lift   → shared lift fn (restore → lockdownBeginLift → FinishLift) → worker closes copies
        Ban    → INSERT run (partial unique: one running per lockdown) → worker executes
        Revoke → revokeChatInviteLink → conditional record → edit copies
```

### Recommended Project Structure
```
migrations/20261011120000_add_lockdown_alerts.sql   # name must sort after 20261006120000
alita/db/models/lockdown_alert.go                   # ChatLockdownAlert, ChatLockdownBurst, ChatLockdownBanRun (+ TableName)
alita/db/lockdown/alerts.go                         # BeginAlert, slot claims, reminders, close claims, link revoke record
alita/db/lockdown/burst.go                          # burst rows, candidate query, run claims
alita/modules/lockdown_alert.go                     # render alert/reminder/closed text, post/close/remind worker steps
alita/modules/lockdown_alert_callbacks.go           # lockdown| namespace: ask/confirm/cancel for lift, ban, revoke
alita/modules/lockdown_response.go                  # decideLockdownResponseBan + ban run executor
alita/modules/lockdown_recent.go                    # Redis recent-joins tracker (write at group -7, snapshot read)
alita/modules/lockdown.go                           # extract shared lift function; call lockdownBeginAlert after ConfirmLocked
alita/modules/lockdown_worker.go                    # add steps to runLockdownCycle; settle path calls BeginAlert
```

### Pattern 1: Exactly-once alert gate (hook points)
**What:** The alert is "decided" once per lockdown by one conditional update that, in the same transaction, inserts the destination slots and the pre-lockdown burst snapshot. Posting is separate and retryable.
**Where it hooks:** the one place a lockdown becomes confirmed in each path.
- `/lockdown`: `lockdown.ConfirmLocked(row.ID)` at `alita/modules/lockdown.go:387-391` `[VERIFIED: lockdown.go:387]`:
  ```
  	if _, err := lockdown.ConfirmLocked(row.ID); err != nil {
  		if _, retryErr := lockdown.ConfirmLocked(row.ID); retryErr != nil {
  ```
  Call the gate right after (success or not): a confirm that failed twice is settled later by the worker.
- Worker settle path: `settleUnconfirmedLockdown` (`lockdown_worker.go:100`) calls `lockdown.ConfirmLocked(ld.ID)` at line 124; its doc says "Settling never posts a message" (`lockdown_worker.go:132`). Phase 5 deliberately changes that for the alert only. Do not call the gate from inside settle; a cycle step `lockdownPlanAlerts` ("every active row with `locked_at IS NOT NULL AND alert_started_at IS NULL`") covers both paths and crashes between `ConfirmLocked` and the gate.
- Gate SQL: `UPDATE chat_lockdowns SET alert_started_at=?, chat_title=? WHERE id=? AND state='active' AND locked_at IS NOT NULL AND alert_started_at IS NULL`; `RowsAffected == 1` ⇒ this caller inserts slots in the same transaction. Same idiom as `ConfirmLocked`/`BeginLift` `[VERIFIED: alita/db/lockdown/repository.go:193-203, 255-272]`.
- Destinations are decided inside the winner: Staff Group via `staff.GetLinkOfGroupFresh(chatID)` (uncached authority read `[VERIFIED: alita/db/staff/repository.go:325]`) → `link.StaffChatID`; log channel via `actionlog.Destination(chat, logchannels.CategoryAdmin)` (returns `ok=false` for a chat with no log channel or the category off `[VERIFIED: alita/utils/actionlog/actionlog.go:21-33, Destination]`). No destination ⇒ gate still stamps `alert_started_at` and inserts no slots (so a Staff Group linked three hours later never receives a stale alert).
- Reuse note: `sendStaffLogPost` (`staff_log.go:87`) cannot post the alert: it takes a `StaffGroupLink`, uses `staffPaced` (the staff budget, not `lockdownPaced`), discards the sent message (`_, err := b.SendMessageWithContext(...)`, line 100) and sends no keyboard `[VERIFIED: staff_log.go:87-109]`. Write a new `lockdownSendAlert` that returns the message ID, uses `lockdownPaced`, and sends `ReplyMarkup`.
- Phase 6 forward compatibility: make the gate take a `lockdownAlertInfo{TriggerKind string; Counts ...}` argument; LOCK-10's "single alert" is then automatic because `uk_chat_lockdowns_active` already allows one active row per chat `[VERIFIED: migration 20261006120000:53]`.

### Pattern 2: Slot rows, remembered message copies, stored body
**What:** One row in `chat_lockdown_alerts` per message copy: `(lockdown_id, kind alert|reminder, seq, dest staff|log, dest_chat_id, state, message_id, body, link, buttons_live, closed_at, claimed_at, attempts, detail)` with `UNIQUE (lockdown_id, kind, seq, dest)`.
- Posting: `pending → posting` (claim with `claimed_at`) → send → `posted` + `message_id`. A send failure goes back to `pending` with an attempt counted; the third is `failed` (mirror `lockdownMaxAttempts = 3`, `lockdown_worker.go:62`). A claim older than `lockdownStaleClaim` (`2 * time.Minute`, `lockdown_worker.go:69`) is released to `pending`; a crash between send and record can therefore post a duplicate. Accept and document: a duplicate alert is harmless, a lost alert is not.
- Store the rendered HTML `body` in the row. Closing, "Link revoked by X" and "Lifted by X" edits compose `body + footer` instead of re-rendering, so D-04 ("stays as posted") holds and the Redis snapshot is not needed again. Keep it under 4096 characters: sample capped at 10 lines (D-03), names capped at `lockdownNameMaxRunes = 64` `[VERIFIED: lockdown.go:38]`, reason at 300 `[VERIFIED: lockdown.go:36]`.
- Render in the destination's language: Staff Group copy with `staffChatTranslator(staffChatID)`, log channel copy with `staffChatTranslator(lockedChatID)` (same as staff log posts, `staff_log.go:96`). User text (names, reason, link) goes in through the existing token-splice helpers (`lockdownSplice`, `lockdownMention`, `lockdown.go:205,225`) so it never passes the translator's printf pass.
- Callback buttons carry the slot row ID (`r=`), known before the send because the row is inserted first. The handler loads the row, then the lockdown, then takes the chat from the lockdown row.

### Pattern 3: Hourly reminders
**What:** In `runLockdownCycle` (`lockdown_worker.go:262-272`) add steps. For each active confirmed lockdown with `alert_started_at` set, compute `seq = floor((now - alert_started_at) / lockdownReminderEvery)` (use `alert_started_at`, not `locked_at`, so lockdowns that predate the migration do not fire a burst of catch-up reminders). If `seq >= 1`, `INSERT` one slot per destination with `ON CONFLICT DO NOTHING`; the caller with `RowsAffected == 1` owns that reminder and posts it.
- Missed periods (bot down for hours) are not back-filled: only the current `seq` is attempted.
- After a reminder is `posted`, remove the buttons of the previous *reminder* in that destination (`editMessageReplyMarkup` with no markup). The original alert keeps its buttons until the lift (interpretation of D-15; note it in the plan).
- Reminder text (D-16): "Still locked since T", reason, removed so far, still pending. Removed = `tally[banned]` plus kept rows with detail `response_ban` (otherwise permanent bans placed by "Ban N" would make the number go down); pending = `tally[pending] + tally[acting]` `[VERIFIED: lockdown_status.go:75-76 uses tally[models.JoinerStateBanned] only]`. Update `lockdownStatus` the same way.
- Interval is a package variable `lockdownReminderEvery = time.Hour` so tests can shorten it (same style as `lockdownStaleClaim`).
- Stop at the lift: only `state = 'active'` rows are scanned.

### Pattern 4: Stateless confirm cards and the `lockdown|v1` namespace
**What:** Ask button → handler checks authority and posts a new message (reply to the alert) with Confirm/Cancel. Confirm data embeds the asker: `lockdown|v1|a=lc&r=<slot id>&u=<asker id>&e=<unix minutes>`. Confirm refuses unless `query.From.Id == u`.
- Namespace: register `callbackquery.Prefix("lockdown|")` next to the others. Existing prefixes are `backup, change_language, restrict, unrestrict, unbanall|, rmAllApprovals, rmAllFilters, filters_overwrite, deleteMsg, reactions_help, staff|, rmAllNotes, notes.overwrite, formatting, anon_admin, join_request, report, connbtns, captcha_verify, captcha_refresh, rmAllChatWarns, rmWarn, fed namespace, rmAllBlacklist, unpinallbtn, helpq, configuration, about, antiraid` `[VERIFIED: grep of callbackquery.Prefix in alita/modules]`; none is a prefix of `lockdown|`, and `lockdown|` does not begin with `antiraid` (the Phase 4 trap).
- Byte budget: `callbackcodec.MaxCallbackDataLen = 64` `[VERIFIED: alita/utils/callbackcodec/callbackcodec.go: MaxCallbackDataLen = 64]`. Worst case `lockdown|v1|a=lc&e=99999999&r=999999999&u=9999999999999999` is 58 bytes; keep user IDs as the only unbounded field and add a `TestLockdownCallbackDataFitsLimit` with 16-digit IDs. `encodeCallbackData` returns `""` on overflow (a dead button) `[VERIFIED: alita/modules/callback_codec.go:15-25]`, so every keyboard builder must treat `""` as "do not send".
- Service identities: refuse `staffServiceUserIDs[query.From.Id]` before any lookup `[VERIFIED: staff_action_decide.go:513 `var staffServiceUserIDs = map[int64]bool{777000: true, 1087968824: true, 136817688: true}`; used at staff_undo.go:213]`. A callback's `from` is the real pressing user (`CallbackQuery.From` is documented "Sender" `[CITED: gotgbot gen_types.go:1226]`), so this is defence in depth and the "anonymous admin" refusal of D-05 never triggers in practice.
- Expiry `e` is lazy: on a late Confirm edit the card to "expired". No Redis timer is needed. Every authority fact is re-checked live at Confirm, so a stale card is harmless.
- Answer every press exactly once (`answerStaffCallback` pattern, `staff.go` `answerStaffCallback`), including denials.

### Pattern 5: "Ban N recent joiners" as a DB-driven run
**What:** Confirm inserts a `chat_lockdown_ban_runs` row (`UNIQUE (lockdown_id) WHERE state='running'`, so a second concurrent run is impossible; same partial-index idiom as `uk_chat_lockdowns_active`) with the card chat/message. The worker executes it; whichever replica wins the final conditional update (`running → done`) edits the card with banned/skipped/failed and posts one `#LOCKDOWN_BAN` log message (through `actionlog.Destination`, admin category).
- Targets: burst rows `chat_lockdown_burst (lockdown_id, user_id UNIQUE, first_name, username, is_bot, invite_link, source pre|joiner, state '' |acting|banned|skipped|failed, reason, detail, run_id, claimed_at, attempts)`. Pre-lockdown snapshot rows are inserted by the alert gate; joiner-sourced rows are inserted lazily at Confirm with `INSERT ... SELECT FROM chat_lockdown_joiners ... ON CONFLICT DO NOTHING` for states `pending, acting, banned, ban_failed` (and, see Open Question Q4, `declined`), never `exempt` (an admin let them in), `cancelled`, `unbanned`.
- Per account, in this order, all paced through `lockdownPaced`:
  1. `approvals.IsUserApproved(chatID, userID)` `[VERIFIED: alita/db/approvals/repository.go:31]` → skip `approved` (no Telegram call).
  2. user is `b.Id` → skip `bot_self`.
  3. live `getChatMember` → `decideLockdownResponseBan(member, row)` (pure function, like `decideStaffAction`): `creator|administrator` → skip `admin`; `kicked` with `isLockdownBan(member, row.BanUntil)` → ban permanent (upgrade the lockdown's own ban); `kicked` with `UntilDate == 0` or any other end date → skip `already_banned` (D-10: left alone); `left|member|restricted` → ban permanent.
  4. permanent ban = `BanChatMember` with no `UntilDate`. Doc: "If user is banned for more than 366 days or less than 30 seconds from the current time they are considered to be banned forever" `[CITED: gotgbot gen_methods.go:420]`; omitting it is forever. It also always deletes the user's messages in supergroups (`RevokeMessages` "Always True for supergroups and channels" `[CITED: gen_methods.go:422]`), so the confirm text must say so.
- Interlock with the Phase 4 worker and the lift (this is the part most likely to be gotten wrong). For a joiner-sourced account claim its joiner row first with `lockdown.ClaimJoiner(id, from, JoinerStateActing)` where `from` is its current state (`pending` or `banned`); a lost claim means the worker or the lift owns it, so skip with `busy`. After a successful permanent ban move the row `acting → kept` with `detail = "response_ban"` (use `lockdown.MoveJoiner`; `kept` is final and not in `unfinishedJoinerStates` `[VERIFIED: repository.go:27-32]`). Why this order is safe:
  - `lockdownBanOne` reads the live status and then bans in separate calls (`lockdown_worker.go:379-390`). Without the claim, a permanent ban placed between its read and its write would be overwritten by the 330-day marker ban and then lifted. The claim excludes it.
  - The lift claims `banned → unbanning` (`lockdown_worker.go:495`). Without the claim, the lift's live read could precede the permanent ban and its `UnbanChatMember(OnlyIfBanned)` would undo it. Both claims are conditional on the same row state, so exactly one wins.
  - A crash mid-run: `ReleaseStaleClaims` returns an `acting` row of an active lockdown to `pending` and of a lifting one to `banned` `[VERIFIED: alita/db/lockdown/joiners.go:289-296 doc comment]`; in both cases the existing live check (`isLockdownBan`, `lockdownBanOne`'s deliberate check) keeps a permanent ban. Burst rows need an equivalent stale release.
- Even without the `kept` mark the lift is safe: a permanent ban reports `until_date` 0, and `isLockdownBan` returns false when `m.UntilDate == 0` `[VERIFIED: lockdown_marker.go:21 `if m.Status != gotgbot.ChatMemberStatusKicked || banUntil == 0 || m.UntilDate == 0 {`]`. The `kept`/`response_ban` mark makes it explicit, saves the lift a `getChatMember`, and lets the lift tally say "banned on purpose" (text `lockdown_lift_tally_kept` already reads "...or banned them on purpose" `[VERIFIED: locales/en.yml:2881]`).
- No pre-existing state needs altering: the joiner `state` CHECK `('pending', 'acting', 'banned', 'ban_failed', 'declined', 'decline_failed', 'exempt', 'cancelled', 'unbanning', 'unbanned', 'kept', 'unban_failed')` `[VERIFIED: migrations/20261006120000_add_chat_lockdowns.sql:81]` already contains everything needed, so no CHECK rewrite is required.
- Stop conditions: before each account re-read the lockdown fresh; if no longer `active`, stop and report "stopped: lockdown lifted" in the result. Bound the run by the pacer (not a hard cap), but cap the per-press list shown on the card.
- `N` on the Ask card counts: unhandled burst rows + joiner rows in the target states with no burst row, minus approved users (a single `GetApprovedUsers` read). It is an upper bound (live admins are only known at run time); say "up to N" or report the real numbers in the result. When `N = 0` at post time render the generic label "Ban recent joiners" rather than "Ban 0 recent joiners".

### Pattern 6: Revoke link
**What:** The alert's displayed link comes from joiner rows (`SELECT invite_link, COUNT(*) ... WHERE lockdown_id = ? AND invite_link <> '' GROUP BY invite_link ORDER BY 2 DESC LIMIT 2`) plus the pre-lockdown snapshot rows. D-13: top link with its count and "+N other links".
- Facts: `ChatMemberUpdated.InviteLink` is "Chat invite link, which was used by the user to join the chat; for joining by invite link events only" `[CITED: gotgbot gen_types.go:2655]`. `ChatInviteLink.InviteLink`: "If the link was created by another chat administrator, then the second part of the link will be replaced with \"...\"" `[CITED: gotgbot gen_types.go:1960]`. `RevokeChatInviteLink`: "Use this method to revoke an invite link created by the bot. If the primary link is revoked, a new link is automatically generated. The bot must be an administrator in the chat for this to work and must have the appropriate administrator rights." `[CITED: gotgbot gen_methods.go:4685]`.
- Consequence (F2): a link the bot sees as `https://t.me/+abc...` was made by another admin and cannot be revoked by the bot. Only a full link (no `...`) is revocable, which in this codebase means a link the bot exported itself (e.g. the `/invitelink` command uses `GetChat(...).InviteLink` `[VERIFIED: alita/modules/admin.go:404 area]`). Show the "Revoke link" button only when the displayed link has no `...`; otherwise show the link plus a hint line "created by another admin; revoke it in Telegram's invite link list" (new locale key).
- The handler reads the link from the slot row (`link` column) at press time, never from callback data (64-byte cap and trust). Authority: live `getChatMember(lockedChat, presser)`: creator, or administrator with `CanInviteUsers` `[VERIFIED: MergedChatMember.CanInviteUsers exists, gen_types.go:2067]`. Check the bot's own `can_invite_users` first via `chat_status.FetchBotMember` and refuse with a friendly line if missing.
- Record once: `UPDATE chat_lockdowns SET link_revoked_at=?, link_revoked_by=?, link_revoked_by_name=?, link_revoked_link=? WHERE id=? AND link_revoked_at IS NULL`. Call Telegram first, record only on success; on Telegram refusal map the error class to a friendly line (Phase 4 quick task 261008-j3d removed raw Telegram text from replies); log the raw error at warn. Then edit every live copy of the alert to `body + "Link revoked by X"` and drop the Revoke button.

### Pattern 7: One shared lift function
**What:** Move the body of `unlockdown` (`lockdown.go:447-515`: `GetCurrentFresh`, manual-change compare, `setLockdownPermissions(row.PrePermissions)`, `lockdownBeginLift`, `FinishLift`) into `liftLockdownRow(ctx, b, row, actorID, actorName)` returning an outcome enum (`done`, `restoreFailed`, `recordFailed`, `lostRace`) plus `manualChange` and `finished`. The command replies from the outcome; the button Confirm calls the same function. The AGENTS.md sentence "`BeginLift` is reached only through `lockdownBeginLift` in `unlockdown`" must change to name the shared function.
- Order is the safety property and must not change: restore first, then `lockdownBeginLift(row.ID, actor.Id, name, manualChange)` `[VERIFIED: lockdown.go:481,487]`. `TestLockdownLiftManualChange` already proves "restore comes before the record" and must pass unchanged on the command path; add the same assertion for the button path.
- The lifter recorded is the presser (D-08): `staffFullName(&query.From)` capped with `lockdownCapRunes(..., lockdownNameMaxRunes)`.
- From a callback there is no `c.Msg` in the locked group: the public "Lockdown lifted by X" notice (Phase 4 D-15) must be sent with `SendMessage` to `row.ChatID` through `lockdownPaced`, not a reply. Open Question Q3.
- Closing copies is the worker's job (Pattern 8) so it also covers `/unlockdown` and survives restarts.

### Pattern 8: Closing copies at the lift
**What:** A cycle step lists slot rows with `state='posted' AND closed_at IS NULL` whose lockdown is no longer `active` (state `lifting` or `lifted`), claims each (conditional `close_claimed_at`), edits to `body` with a "Lifted by X at T" footer (`lifted_by_name`, `lift_started_at` are on the row `[VERIFIED: models/lockdown.go:81-84]`) and no keyboard, then sets `closed_at`. Treat "message is not modified" (`isMessageNotModified`, `staff_panel.go:506`) as success and "message to edit not found" as closed. Do not wait for `FinishLift`: close as soon as the state leaves `active`.
- D-07 (press on a lifted alert): the handler answers "Already lifted by X at T" with the same text and edits the pressed message immediately; the worker later finds it already closed (idempotent).
- Bound each cycle (e.g. 20 edits) so `StopLockdownWorker`'s 5 s wait (`lockdownWorkerStopWait = 5 * time.Second` `[VERIFIED: lockdown_worker.go:54]`) is not blown by a lockdown that accumulated many reminders; the pacer already honours 429 `retry_after`.

### Pattern 9: Recent-joins tracker (Finding F1)
See F1 below. Group -7 handlers (`lockdownOnJoinMember`, `lockdownOnJoinMessage`) run for every supergroup join before anything else and return `ext.ContinueGroups` when there is no lockdown (`lockdown_guard.go:128-130, 196-198`). Add one best-effort Redis write there.
- Key `alita:lockdown:recent:<chat>` (inside the existing operational family `alita:lockdown:*`, outside `alita:cache:` so it survives `CLEAR_CACHE_ON_STARTUP` `[CITED: AGENTS.md Data section]`). Sorted set, score = join unix time, member = compact JSON of `{id, name (≤64 runes), username, bot, link}` so the same user dedupes across the `chat_member` and service-message paths and the sample needs no extra lookup.
- Trim by time (`ZREMRANGEBYSCORE ... -inf now-600`), cap by rank (`ZREMRANGEBYRANK 0 -501`), `EXPIRE 700`. One pipelined round trip with a short timeout (`cache.ContextWithTimeout()` is 5 s, too long on a hot path `[VERIFIED: alita/utils/cache/cache.go:32-34]`; use ~250 ms). Skip silently without Redis.
- Snapshot read happens in the alert gate before the transaction: members with score ≤ `locked_at`.

### Anti-Patterns to Avoid
- **Authorizing against the pressed message's chat** (Staff Group / log channel). The locked chat is `lockdown.GetFresh(row.LockdownID).ChatID`.
- **Putting the link, names or reasons in callback data.** IDs only.
- **Redis-backed Confirm tokens.** Dead Lift button when Redis is down.
- **Calling `BeginLift` from the button handler.** Go through the shared lift function.
- **Posting the alert inline from `/lockdown`.** A slow or failing channel would slow the command; use the slot rows so a failed send retries.
- **Editing alerts on every join.** D-04: alerts stay as posted. Research notes ~1 edit/second/chat limits on alert edits (`.planning/research/PITFALLS.md:586`).

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Exactly-once posting across replicas | A Redis lock or an in-memory flag | Conditional `UPDATE`/`INSERT ... ON CONFLICT DO NOTHING` and `RowsAffected == 1` | Survives restarts and Redis loss; the repo idiom |
| Telegram pacing / 429 handling | A new rate limiter | `lockdownPaced` (`lockdownPacer`, keys `alita:lockdown:pace:*`) | Fleet-wide; a slot over 60 s away fails at once |
| Callback encoding | `strings.Split` on `|` | `encodeCallbackData` / `decodeCallbackData` | AGENTS.md; handles URL-encoding and the 64-byte cap |
| Live authority | Admin cache, `IsUserAdmin` | `lockdownLiveMember` + a shared rights helper split out of `requireLockdownAuthority` | Phase 4 D-11; a failed lookup refuses |
| Ban/unban status decisions | Ad-hoc `if`s in the executor | A pure `decideLockdownResponseBan` table, like `decideStaffAction` / `decideLockdownJoin` | Keeps "never shorten or lift a ban" in one testable place |
| HTML with user text | `fmt.Sprintf` into translated strings | `lockdownSplice` tokens + `html.EscapeString` / `formatting.MentionHtml` | Translator printf pass corrupts `%`; injection |
| Fan-out engine | Fork `startStaffRun` | The lockdown worker with claim rows | Engine is staff-link and card bound (F3) |
| Epoch maths for ban end | Raw ints | Omit `until_date` for permanent | `BanChatMember` doc: <30 s or >366 d is forever |

**Key insight:** every hard part of this phase (once-only, resumable, rate-limited, no-Redis) already has a Phase 4 mechanism; the work is wiring new rows into it, not inventing new machinery.

## Common Pitfalls

### F1: The pre-lockdown burst source does not exist as described
**What goes wrong:** D-02 says "anyone who joined in the 10 minutes before the lockdown, read from antiraid's Redis join list". The list holds 60 seconds, and only when the group has an auto-threshold.
**Evidence:** `antiraidJoinWindowSeconds = 60` (`antiraid.go:35`); the script does `redis.call('EXPIRE', KEYS[1], ARGV[3])` and `redis.call('ZREMRANGEBYSCORE', KEYS[1], 0, ARGV[4])` with `window := antiraidJoinWindowSeconds` and `cutoff := now - int64(antiraidJoinWindowSeconds)` (`antiraid.go:73-74,140-141`) `[VERIFIED]`. Joins are tracked only after `if settings.AutoAntiRaidThreshold <= 0 {` → `continue` (`antiraid.go:495-497`), and the column default is `auto_antiraid_threshold INT NOT NULL DEFAULT 0,` (`migrations/20260518113000_add_antiraid_settings.sql:7`) `[VERIFIED]`. `onJoin` also returns at once while a lockdown is confirmed (`antiraid.go:462-463`) and skips approved users and admins. The member format is `%d:%d:%d` = `userID:unix:nanos` (`antiraid.go:139`), no name.
**Consequence:** by default the pre-lockdown part is empty, and since the alert is posted at lock time and "stays as posted" (D-04), a manual lockdown's alert would show an empty sample and "Ban 0 recent joiners".
**How to avoid:** Pattern 9 (dedicated 10-minute tracker, Redis-only, best effort, empty without Redis as the owner already accepted). It is also the join history Phase 6's detector needs. Needs an owner confirmation (Q1) because it replaces a locked decision's mechanism, not its intent.
**Warning signs:** an alert test that passes only because the test seeds antiraid's key by hand.

### F2: Revoke only reaches bot-created links
See Pattern 6. A test double that lets `revokeChatInviteLink` succeed for any string hides this. UAT must try both a bot-created link and another admin's link and record Telegram's exact answer (`[ASSUMED]` error wording, A4).

### F3: The staff run engine cannot run a per-user list
`staffRunSpec` has `Links []models.StaffGroupLink`, `Card *staffActionCard`, hooks that take a group index, and the coordinator releases a per-target Redis lock and marks the Redis card done (`staff_action_run.go:203`, coordinator in `startJoinedStaffRun`) `[VERIFIED]`. STATE.md's note that Phase 5 "should reuse it" does not survive contact; reuse its ideas (single writer of the card, throttled edits, final-edit fallback) only if the card progress edits are wanted. Recommendation: no live progress edits; the card is edited once at the end (and the Confirm edit says "Banning..."), because D-04 style churn buys little.

### F4: Confirm cards that need Redis
The staff card helpers return `errStaffCardNoRedis` without a Redis client (`staff_action_card.go:152,163,182`) and both staff Ask paths answer `staff_act_redis_unavailable` (`staff_action.go:162`, `staff_undo.go:299`) `[VERIFIED]`. A lockdown button that dies with Redis contradicts Phase 4's "a lockdown works without Redis". Use stateless confirm (Pattern 4).

### Pitfall 1: Stale alert button hits the next lockdown
**What goes wrong:** An old alert's Lift pressed during a new lockdown of the same chat.
**How to avoid:** Buttons carry the slot row (and so the lockdown ID). The handler loads that lockdown; if it is not `active` answer "Already lifted by X at T" and never look up the chat's current lockdown. Test: lock, lift, lock again, press the first alert's Lift; the second lockdown stays active.

### Pitfall 2: A permanent ban racing the marker ban or the lift
See Pattern 5. Claim the joiner row first. Test with the fake's `onBan` hook.

### Pitfall 3: `kept` rows are not reclaimable on rejoin
`lockdownReclaimStates` is `banned, ban_failed, exempt, declined, decline_failed, cancelled` (`lockdown_guard.go:375-382`) `[VERIFIED]`. A row that is `kept` makes `lockdownExistingJoin` return `(false, true)` (handled, no ban) for a user an admin unbanned who rejoins mid-lockdown. This pre-dates Phase 5 (a `/ban`-then-unban case) but Phase 5 creates many `kept` rows. A response-banned user is permanently banned and cannot rejoin unless an admin unbans them first, so treat it as a known low-risk limit; add a characterization test and a one-line note rather than widening Phase 5.

### Pitfall 4: Counts that drop after "Ban N"
`lockdownStatus` shows `tally[banned]` as "Joiners removed so far" (`lockdown_status.go:75-76`). Moving response-banned rows to `kept` would make it fall. Count `banned + kept/response_ban` in both status and reminders, and add a test.

### Pitfall 5: Editing a lot of old reminders at the lift
A multi-day lockdown has up to 24 reminders per day per destination. D-04 says edit every copy; do it, but in capped batches from the worker, newest first.

### Pitfall 6: Language and chat title in the log channel
The log channel's callback context has no group language; `ctxTr(ctx)` would give the channel's default. Use the slot's destination to choose the translator (Pattern 2). The locked group's title is needed for headers and reminders: store `chat_title` at the gate (from `ctx.EffectiveChat.Title` in `/lockdown`; for the settle path extend `lockdownChat` with `Title` and read it from the `getChat` the settle already makes — the fake already returns `"title":"Test Chat"` `[VERIFIED: lockdown_fake_test.go:181-182]`).

### Pitfall 7: Confirm cards cluttering the log channel
Each Ask posts a message into a channel others read. Edit the card to its terminal text (Cancelled, Expired, result) as the staff cards do (their D-08: never deleted), do not delete it.

### Pitfall 8: Answering the callback late
Answer the callback (toast) before the long work, then edit the card with the outcome (staff pattern `staff_action_card.go` Confirm). The lift does a `getChat`, a `setChatPermissions` and a DB write.

## Code Examples

### Callback data (verified API)
```go
// Source: alita/modules/callback_codec.go:15 encodeCallbackData; alita/utils/callbackcodec
confirm := encodeCallbackData(lockdownCallbackNamespace, map[string]string{
	"a": lockdownActLiftConfirm, // "lc"
	"r": strconv.FormatUint(uint64(slot.ID), 10),
	"u": strconv.FormatInt(query.From.Id, 10),
	"e": strconv.FormatInt(time.Now().Add(lockdownCardLifetime).Unix()/60, 10),
})
if confirm == "" { /* dead button: do not send the card */ }
```

### Decision table skeleton
```go
// decideLockdownResponseBan is the whole policy for one account of "Ban N recent joiners".
// Pure; never lifts or shortens a ban.
func decideLockdownResponseBan(m gotgbot.MergedChatMember, banUntil int64) (ban bool, skip string) {
	switch m.Status {
	case gotgbot.ChatMemberStatusCreator, gotgbot.ChatMemberStatusAdministrator:
		return false, "admin"
	case gotgbot.ChatMemberStatusKicked:
		if isLockdownBan(m, banUntil) { // lockdown's own 330-day marker: make it permanent
			return true, ""
		}
		return false, "already_banned" // permanent, or someone else's end date (D-10)
	}
	return true, "" // member, restricted, left
}
```

### Exactly-once reminder slot
```sql
-- Source pattern: alita/db/lockdown/joiners.go RecordJoin (ON CONFLICT DO NOTHING, RowsAffected == 1)
INSERT INTO chat_lockdown_alerts (lockdown_id, kind, seq, dest, dest_chat_id, state, claimed_at, ...)
VALUES (?, 'reminder', ?, ?, ?, 'posting', now(), ...)
ON CONFLICT (lockdown_id, kind, seq, dest) DO NOTHING;   -- winner posts; loser does nothing
```

### Schema sketch (planner finalises; migration file name must sort after `20261006120000_add_chat_lockdowns.sql`)
```sql
ALTER TABLE chat_lockdowns
  ADD COLUMN IF NOT EXISTS alert_started_at TIMESTAMP WITH TIME ZONE,
  ADD COLUMN IF NOT EXISTS chat_title TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS link_revoked_at TIMESTAMP WITH TIME ZONE,
  ADD COLUMN IF NOT EXISTS link_revoked_by BIGINT,
  ADD COLUMN IF NOT EXISTS link_revoked_by_name TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS link_revoked_link TEXT NOT NULL DEFAULT '';
CREATE TABLE IF NOT EXISTS chat_lockdown_alerts (...);   -- UNIQUE (lockdown_id, kind, seq, dest), CHECKs on kind/dest/state
CREATE TABLE IF NOT EXISTS chat_lockdown_burst (...);    -- UNIQUE (lockdown_id, user_id)
CREATE TABLE IF NOT EXISTS chat_lockdown_ban_runs (...); -- partial UNIQUE (lockdown_id) WHERE state = 'running'
```
Rules that apply: no top-level `BEGIN`/`COMMIT`, no `CREATE INDEX CONCURRENTLY`, one transaction per file, idempotent statements (`IF NOT EXISTS`), SQL-only foreign key to `chat_lockdowns(id)` with `ON DELETE CASCADE` and no Go relation (as in Phase 4), `TableName()` on every model, and the new models in all three harness `AutoMigrate` lists (Wave 0). `[CITED: AGENTS.md Migrations / Adding a module]`

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `antiraid.callbackHandler` authorizes against the pressed message's chat (`antiraid.go:814-816` per PITFALLS) | Authority from the lockdown row's chat, live `getChatMember` | Phase 5 | Required, the alert sits in other chats |
| Redis-hash confirm cards (staff) | Stateless confirm for lockdown buttons | Phase 5 | Works without Redis |

**Deprecated/outdated:** nothing is retired here; `/antiraid` retirement is Phase 6.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Inline buttons on a bot post in a channel deliver `callback_query` to the bot, with `from` the real pressing user and `message` the channel post | Pattern 4 | If channel presses are not delivered, the log-channel copy has dead buttons; then the log channel gets text only and the Staff Group is the control surface. UAT item. |
| A2 | The bot can `editMessageText` its own channel posts when it has the channel post right (no extra edit right, no 48 h limit for its own messages; the 48 h note applies to business messages not sent by the bot, `gen_methods.go:2359`) | Pattern 8 | Closing edits in the log channel fail; mark the row closed and log. |
| A3 | `revokeChatInviteLink` needs the bot to hold `can_invite_users` (the doc says only "appropriate administrator rights") | Pattern 6 | Wrong refusal wording; the bot-rights pre-check would block a call that would work. Use the pre-check only as a hint and let Telegram decide. |
| A4 | Telegram's error text when revoking a link the bot did not create (wording and 400 vs 403) | Pattern 6 | Friendly mapping falls into the generic branch; no correctness risk. UAT records the real answer. |
| A5 | Bot API per-chat limits (about 20 messages per minute to a group, 1 per second per chat) | Pattern 3, 8 | Closing edits could hit 429s; handled by the pacer's retry_after, cost is latency only. |
| A6 | Real `getChat` answers include `title` | Pitfall 6 | Settle-path alert header falls back to the stored title or the chat ID. |
| A7 | A bot-created invite link is returned in full (no `...`) in `chat_member` updates | Pattern 6 | If Telegram truncates it too, the button never shows; no safety impact. Verify in UAT. |

## Open Questions

1. **Q1 (needs owner decision): D-02's pre-lockdown source is not 10 minutes and is empty by default (F1).**
   - What we know: antiraid's list is 60 s, only filled when `auto_antiraid_threshold > 0` (default 0), and skipped while a lockdown is active.
   - What's unclear: whether the owner wants (a) a dedicated 10-minute Redis tracker (recommended, about one pipelined ZADD per join, no PostgreSQL write, stays within the Deferred decision), (b) antiraid's list as is (alert sample will be empty for a manual lockdown), or (c) the declined PostgreSQL join log.
   - Recommendation: (a). It keeps D-02's intent, is best effort and empty without Redis, and Phase 6's join-burst detector can read the same key.
2. **Q2 (owner awareness): "Revoke link" only works for links the bot created (F2).** Recommendation: show the button only for a full link, else show the link and a hint to revoke it in Telegram; UAT with both kinds.
3. **Q3: Public "Lockdown lifted by X" notice in the locked group when the lift came from a button.** Phase 4 D-15 posts one on lift; D-01/D-08 mention only the tally. Recommendation: post the same notice (via `SendMessage`, not a reply), so the group learns why permissions came back.
4. **Q4: Are declined join requests part of "Ban N recent joiners"?** D-02 says "every joiner the lockdown recorded", which includes `declined` request rows, but those people never became members. Recommendation: include them (the ban stops a re-request after the lift) and report them in the result as "declined requests".
5. **Q5: Respect the log channel's category switch for the alert?** `actionlog.Destination` skips a channel whose `admin` category is off. Recommendation: respect it, consistent with staff log posts; an owner who wants alerts regardless needs a different rule.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go | build, tests | ✓ | go1.26.0 linux/amd64 | — |
| gcc / CGO | `go-sqlite3` tests | ✓ | CGO_ENABLED=1 | — |
| make | `make test`, `make check-translations`, `make generate-docs` | ✓ | /usr/bin/make | — |
| golangci-lint | `make lint` | ✓ but too old | 2.5.0 built with go1.25.1 | Use `go vet` and `gofmt` only (known tooling gap, STATE.md) |
| Redis | tests use miniredis (in-process) | ✓ | redis-server 7.0.15 present, not needed | miniredis |
| PostgreSQL | migration-chain test (`ALITA_TEST_MIGRATION_CHAIN=true`), `make test-postgres-integrity` | ✗ server not running (`pg_isready`: no response); psql client and `/usr/lib/postgresql/16` present | 16 | Start a local cluster or Docker (docker client present, daemon not verified); otherwise rely on SQLite tests and review the SQL by hand |
| core.telegram.org | Bot API doc lookups | ✗ blocked by the egress policy (CONNECT 403) | — | gotgbot doc comments (mirror the Bot API) |

**Missing dependencies with no fallback:** none blocking.
**Missing dependencies with fallback:** PostgreSQL for the migration chain (SQLite covers behaviour; the new SQL must be reviewed against the chain test before merge), Bot API site (use gotgbot comments and UAT).

Baseline run in this session: `go test -tags testtools -race -count=1 -run '^TestUnlockdownLiftsOnce$|^TestLockdownCommandStarts$' ./alita/modules` → `ok ... 1.646s` (first build 74 s). `[VERIFIED: shell]`

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go `testing`, build tag `testtools`, SQLite via `internal/testdb`, miniredis, hand-written `gotgbot.BotClient` fakes (`lockdownFake` embeds `staffActionFake`) |
| Config file | none; `-tags testtools` is mandatory (AGENTS.md) |
| Quick run command | `go test -tags testtools -race -count=1 -run '^TestLockdown' ./alita/modules` |
| Repository tests | `go test -tags testtools -race -count=1 ./alita/db/lockdown` |
| Full suite command | `make test` |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| LOCK-11 | Alert gate runs once; two cycles/replicas post one alert per destination | integration | `go test -tags testtools -race -count=1 -run '^TestLockdownAlertPostsOnce$' ./alita/modules` | ❌ Wave 0 |
| LOCK-11 | Alert to Staff Group and log channel; none when neither exists; log category off | integration | `... -run '^TestLockdownAlertDestinations$'` | ❌ Wave 0 |
| LOCK-11 | Settle path (unconfirmed row confirmed by worker) also alerts | integration | `... -run '^TestLockdownAlertAfterSettle$'` | ❌ Wave 0 |
| LOCK-11 | Sample: 10 max, "and N more", bots marked, names HTML-escaped, pre-lockdown members from tracker | unit/integration | `... -run '^TestLockdownAlertSample$'` | ❌ Wave 0 |
| LOCK-11 | Send failure retries, stale claim reposts, third failure is final and does not block others | integration | `... -run '^TestLockdownAlertDelivery$'` | ❌ Wave 0 |
| LOCK-11 | Recent-joins tracker: dedup across join paths, 10 min window, cap, no Redis is silent | unit | `... -run '^TestLockdownRecentJoins$'` | ❌ Wave 0 |
| LOCK-06 | Lift from Staff Group and from log channel works for a locked-group admin; presser recorded as lifter; restore precedes record | integration | `... -run '^TestLockdownAlertLift$'` | ❌ Wave 0 |
| LOCK-06 | Staff Group admin who is not an admin of the locked group is refused, nothing changes; non-restrict admin refused; failed lookup refuses; service ID refused | integration | `... -run '^TestLockdownAlertLiftAuthority$'` | ❌ Wave 0 |
| LOCK-06 | Only the asker can Confirm; Cancel edits the card; late Confirm is expired | integration | `... -run '^TestLockdownAlertConfirmCard$'` | ❌ Wave 0 |
| LOCK-06 | Already lifted/lifting answers "Already lifted by X at T", edits that alert, changes nothing; old alert never lifts a newer lockdown | integration | `... -run '^TestLockdownAlertStaleButtons$'` | ❌ Wave 0 |
| LOCK-06 | Lift via command or button closes every alert and reminder copy once, buttons removed, resumes after restart; command path regression | integration | `... -run '^TestLockdownAlertClosedAtLift$'` and existing `^TestLockdownLiftManualChange$` | ❌ Wave 0 / ✅ existing |
| LOCK-12 | Targets exclude admins, approved, bot, exempt rows; bots banned; permanent ban (no `until_date`) | integration | `... -run '^TestLockdownBanRecentJoiners$'` | ❌ Wave 0 |
| LOCK-12 | Decision table: kicked with marker upgraded, kicked with other end date or permanent left alone, left/member/restricted banned | unit | `... -run '^TestDecideLockdownResponseBan$'` | ❌ Wave 0 |
| LOCK-12 | Lift keeps a response-banned joiner banned (no unban call), tally says "on purpose" | integration | `... -run '^TestLockdownBanSurvivesLift$'` | ❌ Wave 0 |
| LOCK-12 | Race: response ban vs worker ban vs lift never overwrites a permanent ban | integration | `... -run '^TestLockdownBanRaces$'` | ❌ Wave 0 |
| LOCK-12 | Second press bans only unhandled accounts; N counts only those; only one run at a time; run resumes after restart; stops when lifted | integration | `... -run '^TestLockdownBanRun$'` | ❌ Wave 0 |
| LOCK-12 | Result reports banned/skipped (reasons)/failed; one `#LOCKDOWN_BAN` log post | integration | `... -run '^TestLockdownBanResult$'` | ❌ Wave 0 |
| LOCK-13 | Alert shows top link, count, "+N other links"; none when Telegram reported none | integration | `... -run '^TestLockdownAlertInviteLink$'` | ❌ Wave 0 |
| LOCK-13 | Revoke: only live creator/admin with can_invite_users; truncated (`...`) link has no button; success edits "Link revoked by X"; Telegram refusal answered with a mapped line; once only | integration | `... -run '^TestLockdownRevokeLink$'` | ❌ Wave 0 |
| LOCK-14 | One reminder per hour per destination, one replica posts, previous reminder loses buttons, none after lift, no catch-up after downtime | integration | `... -run '^TestLockdownReminders$'` | ❌ Wave 0 |
| all | Callback data fits 64 bytes with 16-digit IDs; unknown action answered once | unit | `... -run '^TestLockdownCallbackData$'` | ❌ Wave 0 |
| repo | `BeginAlert` once; slot uniqueness; claim/release; close claim; link revoke once; burst candidate query; run uniqueness | repository | `go test -tags testtools -race -count=1 -run '^(TestBeginAlert|TestAlertSlots|TestBurst|TestBanRun)' ./alita/db/lockdown` | ❌ Wave 0 |
| platform | Migration applies on PostgreSQL | chain | `ALITA_TEST_MIGRATION_CHAIN=true DATABASE_URL=... go test -tags testtools -run '^TestRepositoryMigrationChain$' ./alita/db/migrations` | ✅ existing (needs PG) |
| PLAT-03 | New keys in all 7 locale files | script | `make check-translations` | ✅ existing |
| docs | Help text change reflected in generated docs | script | `make check-docs` (fix: `make generate-docs`) | ✅ existing |

Tests assert observable behaviour (message sent, row persisted, call made or not made), through `withStaffLocale` markers (`staffMarker("key")`), never literals. No mock libraries.

### Sampling Rate
- **Per task commit:** `go test -tags testtools -race -count=1 -run '^TestLockdown' ./alita/modules ./alita/db/lockdown` plus `go vet ./...` and `gofmt -l alita` (golangci-lint here is built with Go 1.25).
- **Per wave merge:** `go test -tags testtools -race -count=1 ./alita/...`
- **Phase gate:** `make test`, `make check-translations`, `make check-docs` green before `/gsd-verify-work`; migration-chain test on PostgreSQL if a server can be started.

### Wave 0 Gaps
- [ ] Add `ChatLockdownAlert`, `ChatLockdownBurst`, `ChatLockdownBanRun` to the three `AutoMigrate` lists named in AGENTS.md: `alita/modules/test_harness_test.go`, `alita/db/testmain_test.go`, `alita/db/lockdown/testmain_test.go`; extend `lockdownCleanup` (`lockdown_fake_test.go`) to delete the new rows (children first).
- [ ] `lockdownFake`: handle `editMessageReplyMarkup`, `revokeChatInviteLink` (scriptable via `script(...)`, records `invite_link`), per-chat message IDs for a channel, a `banChatMember` hook (`onBan`) to interleave the race tests, and `getChatMember` for many burst users (`setMember`). The base `staffActionFake` already answers `sendMessage` (returns a message ID), `editMessageText`, `banChatMember`, `unbanChatMember`, `getChat` `[VERIFIED: staff_action_fake_test.go:296-418]`.
- [ ] `lockdownEnv` helpers: link the env chat to a Staff Group and set a log channel (`logchannels.Set`), `press(from, chat, msgID, data)` modelled on `pressJoinRequest` (`lockdown_fake_test.go:462`), `slots()` / `alertText()` readers, and an `ageAlert(...)` helper like `ageJoiner` (`lockdown_fake_test.go:496`) to make reminders due without sleeping (or a shortened `lockdownReminderEvery`).
- [ ] Test seams as package variables (Phase 4 style): `lockdownReminderEvery`, `lockdownRecentLookup`, `lockdownAlertDestinations`.

## Security Domain

`security_enforcement` is enabled (absent in config means enabled; `.planning/config.json` has `"security_enforcement": true`, `"security_asvs_level": 1`). `[VERIFIED: .planning/config.json]`

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | The bot authenticates nobody itself; identity is Telegram's `from` |
| V3 Session Management | no | Stateless callbacks; no sessions |
| V4 Access Control | yes | Live `getChatMember` of the locked group per press (creator / admin with `can_restrict_members`; `can_invite_users` for revoke); authority never from message chat, payload or admin cache |
| V5 Input Validation | yes | Strict parse of callback fields (positive, 63-bit, bounded `e`); fresh row loads; reject mismatched `u`; HTML-escape every user string |
| V6 Cryptography | no | No new secrets or crypto (no `logredact.RegisterSecret` needed) |
| V7 Error handling / logging | yes | Never log invite links or callback tokens above debug; raw Telegram errors stay in logs, users see mapped lines |
| V11 Business logic | yes | Conditional updates for every once-only transition; stale-button and double-confirm tests |

### Known Threat Patterns for this stack

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Staff Group member (not an admin of the locked group) presses Lift/Ban/Revoke | Elevation of privilege | Live check against the lockdown row's chat; test "admin in Staff Group, not in locked group → refused, nothing changes" |
| Channel subscriber presses a log-channel button | Elevation of privilege | Same live check; denial answered; no state change |
| Replayed or stale callback (old alert vs new lockdown; second Confirm) | Tampering | Row-bound IDs, state checks, conditional claims, asker ID in Confirm data |
| Forged callback data | Tampering | Treated as untrusted request; chat and link come from DB rows, never from data (Bot API: the originating message "can contain no callback buttons with this data" `[CITED: gotgbot gen_types.go:1234]`) |
| Another user confirms someone else's card | Spoofing | `query.From.Id == u` check; service IDs refused |
| HTML/markup injection through names, usernames, reason, invite link | Tampering | `html.EscapeString`, token splice after translation, link in `<code>` |
| Invite link disclosure to Staff Group members who are not admins of the locked group | Information disclosure | Links are posted only when the bot sees the full link (its own); truncated links are inherently safe; document that staff see a live link |
| Irreversible message deletion by `banChatMember` | Tampering | Confirm card states it; banned/skipped/failed result; `#LOCKDOWN_BAN` log with who |
| Ban run hammering Telegram | Denial of service | `lockdownPaced`, one run per lockdown (partial unique index), 60 s MaxWait fail-fast |
| Lost audit of who acted | Repudiation | `lifted_by`, `link_revoked_by`, ban run `started_by` stored; `#LOCKDOWN_BAN` post names the presser |

## Sources

### Primary (HIGH confidence)
- Repository files read this session (cited inline with paths and lines): `alita/modules/lockdown.go`, `lockdown_worker.go`, `lockdown_guard.go`, `lockdown_marker.go`, `lockdown_status.go`, `lockdown_perms.go`, `staff_log.go`, `staff_action_card.go`, `staff_action_run.go`, `staff_undo.go`, `staff.go`, `antiraid.go`; `alita/db/lockdown/repository.go`, `joiners.go`; `alita/db/models/lockdown.go`; `migrations/20261006120000_add_chat_lockdowns.sql`, `20260518113000_add_antiraid_settings.sql`; test fakes `lockdown_fake_test.go`, `staff_action_fake_test.go`; `AGENTS.md`; `.planning/phases/04-manual-lockdown/04-CONTEXT.md`, `04-RESEARCH.md`; `.planning/research/PITFALLS.md` (Pitfall 12).
- gotgbot doc comments in `/root/go/pkg/mod/github.com/!paul!son!of!lars/gotgbot/v2@v2.0.0-rc.36.0.20260919140833-240296efadb4/gen_methods.go` and `gen_types.go` (mirror of the Bot API text): revoke, ban, edit, `ChatInviteLink`, `ChatMemberUpdated`, `CallbackQuery`.

### Secondary (MEDIUM confidence)
- Web search result summaries (aiogram docs for `revokeChatInviteLink`) agreeing that the method revokes links created by the bot.

### Tertiary (LOW confidence)
- Channel callback delivery, channel edit rights, Bot API rate limits, Telegram's revoke error wording (A1-A5): training knowledge, to be confirmed in UAT. `core.telegram.org` was unreachable (egress policy 403), so the official page could not be fetched this session.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH, no new dependencies, all APIs found in the vendored gotgbot source.
- Architecture: HIGH for the worker/slot/claim design (direct extension of Phase 4 patterns); MEDIUM for the exact schema, left to the planner.
- Pitfalls: HIGH for F1, F3, F4 and the race analysis (read from code); MEDIUM for F2 consequences (doc-cited, behaviour untested live).

**Research date:** 2026-10-11
**Valid until:** 2026-11-10 (stable codebase; re-check Bot API wording if gotgbot is bumped)
