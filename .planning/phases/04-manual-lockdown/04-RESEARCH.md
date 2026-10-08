# Phase 4: Manual Lockdown - Research

**Researched:** 2026-10-06
**Domain:** Go Telegram bot (gotgbot v2 rc.36), Bot API chat permissions / bans / join handling, PostgreSQL state machine, multi-replica pacing
**Confidence:** MEDIUM-HIGH (all in-repo facts and gotgbot shapes read from source this session; a handful of Telegram server behaviours are `[ASSUMED]` and listed for UAT / fixture confirmation)

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**Approved users (LOCK-03)**
- **D-01:** **Approved users are muted like everyone else** (Telegram can't exempt them, D-02). Only admins can talk during a lockdown. The lock notice (D-15) says that approved users are muted too. Telegram enforces it on its own, so it keeps working when the bot is slow, rate-limited or down.
- **D-02:** **Spike 1 result: Telegram can't exempt one user from a locked default.** The owner ran the check on 2026-10-06 in a supergroup they own: with "Send messages" turned off for all members, the "Send messages" toggle for a user added under Exceptions is greyed out. D-01 applies. Locking makes no per-user calls, and no approved-user exception is planned.
- **D-03:** **Spike 2 is designed away, not gated.** The join guard handles all three ways a join can arrive: the `chat_member` update, the `new_chat_members` service message, and `chat_join_request`. A join delivered twice is acted on once. The ban is never gated on a Redis claim (Pitfall 10: `claimRecentJoinProcessing` fails closed). An automated test proves the guard stops greetings and captcha for a banned joiner. Live join delivery and EndGroups behaviour are UAT items.

**Removing joiners (LOCK-01)**
- **D-04:** **A joiner is banned until the lift, not kicked.** Every non-exempt joiner (D-07, D-08) is banned on joining and added to the lockdown's joiner list, so they can't rejoin while it lasts and each raider costs one call. The lift unbans them, paced, and reports any it couldn't unban. — **Reversibility:** costly — Phase 5's alert sample and "Ban N recent joiners" are built on the recorded joiner list and on the lift's unban step.
- **D-05:** **The lift never lifts a deliberate ban.** It unbans only joiners whose ban is still the lockdown's own. A joiner banned on purpose during the lockdown stays banned, whether by `/ban`, a staff `/ban`, or Phase 5's "Ban N recent joiners". This is the same never-lift-a-ban rule as `decideStaffAction` and undo.
- **D-06:** **Join requests that arrive during a lockdown are declined at once**, so the person can request again after the lift. Auto-approve (`greetings.pendingJoins`) never admits anyone while a group is locked.
- **D-07:** **Only a user added directly by a live admin of the group gets in.** The join's performer must be a creator or administrator at that moment, which includes an admin approving a pending request by hand. Everyone else is banned until the lift: people joining through any invite link (an admin's own link too), approved users who rejoin, and returning members.
- **D-08:** **Bots are banned until the lift whoever adds them**, an admin included. An admin can add the bot again after the lift.
- **D-09:** **Joiners still on their captcha when the lockdown starts keep their attempt.** If they pass, they stay muted by the lockdown until the lift (through D-23). If they fail or time out, the group's captcha action applies as usual.
- **D-10:** **A banned joiner gets no welcome, no captcha and no goodbye.** Their join service message is deleted when the bot has the delete right. If it doesn't, the message stays (D-20).

**Lock & lift commands (LOCK-06, LOCK-09)**
- **D-11:** **`/lockdown` and `/unlockdown` need the group's owner, or an administrator with `can_restrict_members`, checked live with `getChatMember`.** The admin cache is never used. Everyone else is refused through the command pipeline's standard reply.
- **D-12:** **Anonymous admins go through the existing "prove you're admin" button** (`RegisterAnonymousAdminHandler` + `anonPipelineHandler`). Whoever taps it is checked live for D-11's right, runs the command, and is named as the one who locked or lifted. The group's AnonAdmin mode never skips this live check.
- **D-13:** **Both commands act at once, with no Confirm tap.** `/lockdown [reason]` takes an optional reason.
- **D-14:** **`/lockdown` during an active lockdown starts nothing.** It replies with the existing one: since when, who started it, and the reason (ROADMAP criterion 3).
- **D-15:** **The group sees a public notice on lock and on lift.**
  - Lock: the group is in lockdown, only admins can talk, and new members are removed until it lifts. It also says who locked it, gives the reason if there is one, and says that approved users are muted too (D-01).
  - Lift: "Lockdown lifted by Name", how many joiners were unbanned, and any that couldn't be.
- **D-16:** **`/lockdownstatus` is a new read-only command for any admin of the group.** It shows whether the group is locked, since when, who locked it, the reason, how many joiners have been removed so far, and the D-19 warning when permissions were changed by hand. It never changes anything. The name can't be confused with `/lock`, `/locks` or `/locktypes`.
- **D-17:** **`/staff` marks a locked group with one marker on its row**, like "🔒 in lockdown since 5 Oct 12:04". The reason and who locked it are not shown there. They stay in `/lockdownstatus`, and Phase 5's alert brings them to the Staff Group.

**Hand edits & restore (LOCK-05, LOCK-07, LOCK-08)**
- **D-18:** **The lift always restores the exact pre-lockdown permissions.** If the live permissions are no longer the ones the lockdown set, the lift reply says a manual change was replaced. There's no prompt or choice.
- **D-19:** **The bot never fights a manual unlock.** If an admin reopens the group by hand in Telegram, the lockdown stays active until `/unlockdown`, and joiners keep being banned. The bot doesn't re-lock. `/lockdownstatus` and the lift reply point out the manual change.
- **D-20:** **`/lockdown` is refused unless the bot can lock.** The bot must be an administrator with `can_restrict_members`, and the group's permissions must be readable and settable. On refusal the reason is given and nothing is recorded. A missing "Delete messages" right doesn't block the lockdown: join messages just stay, and the reply says so.
- **D-21:** **A failed restore leaves the group locked.** If restoring the permissions fails (a Telegram error, or the bot lost its rights), the lockdown stays active and nobody is unbanned. The admin is told why and to fix it and run `/unlockdown` again. The bot never reports "lifted" until Telegram confirms the restore. Joiner unbans start only after that confirmation, and any that fail are listed.
- **D-22:** **The bot being removed from a locked group doesn't end the lockdown.** It stays recorded as active, because it never lifts on its own. `/staff` shows the group as locked and the bot as missing. Re-adding the bot, then `/unlockdown`, ends it cleanly. (Claude's default, accepted by the owner.)
- **D-23:** **Anyone unmuted during a lockdown gets the pre-lockdown permissions, not the locked set (LOCK-08).** This covers `/unmute`, a captcha pass, staff `/unmute` and the unban restore. `resolveUnmutePermissions` is the single choke point for all of them, and it must return the active lockdown's stored snapshot instead of the live (locked) defaults. Otherwise those users stay muted after the lift (Pitfall 7b).

### Claude's Discretion
- **Schema.** The lockdown row and the joiner list. The pre-lockdown permissions are stored exactly as Telegram returned them, with every field round-tripped (Pitfall 8), taken from a fresh `getChat` before locking and never re-read later. The locked set the bot applied is stored too, for D-18's and D-19's "changed by hand" check. There is one active lockdown per group, enforced by a conditional insert or a partial unique index (groundwork for LOCK-10), and a trigger-kind field so Phase 6 can add automatic triggers. Migration rules in AGENTS.md apply.
- **Recognising "the lockdown's own ban" (D-05).** For example, every ban path marks the joiner's row, or the lockdown bans with a distinctive end date that the lift compares against the live status. Any approach works as long as no deliberate ban is lifted and no lockdown ban is left in place.
- **Never ban a joiner without recording them first.** An unrecorded ban would never be unbanned. If the record write fails, skip the ban: the joiner is still muted by the locked defaults. No cap on the joiner list may leave anyone banned forever.
- **A restart in the middle of a lift** must not leave joiners banned forever. Remaining unbans resume, or are retried, after a restart. Background work joins the shutdown drain (AGENTS.md: drains are registered after DB-close).
- **Pacing.** Joiner bans, lift unbans and declines respect Telegram rate limits across replicas, through a fleet-wide budget like `staffPaced`, not a per-replica limiter. A manual lockdown still works with Redis down (the state is in PostgreSQL). Only pacing degrades.
- **Handler group for the join guard.** It must run before greetings, captcha and auto-approve (group 0), and must not share `-5` with antiraid's `onJoin` (gotgbot stops a group at the first matching handler). Update AGENTS.md's group list in the same commit. Until Phase 6, `/antiraid` keeps working as it does now, but while a lockdown is active each joiner is handled once, by the lockdown.
- **Cross-path dedupe of joins (D-03)**, and suppressing goodbye messages when the bot itself did the ban.
- **How lockdown state is read** on each join: fresh, or through `GetFromCacheOrLoad` with the key added to `skipLocal`, so that every replica enforces a new lockdown at once.
- **Basic groups.** Whether `/lockdown` works there or asks the admin to upgrade first. Restricting and unmuting individual users needs a supergroup.
- **`/lockdown` inside a Staff Group** is allowed whenever the D-11 and D-20 checks pass.
- **Wording and formats.** The exact text in all 7 locale files, the reason length cap and HTML escaping (about 300 characters, as Phase 3 suggested for log-post reasons), the time format, help text, and docs (`make generate-docs`). Also what `/unmute` replies during a lockdown, for example "they can talk once the lockdown lifts" under D-01.

### Deferred Ideas (OUT OF SCOPE)
None came up. The discussion stayed within the phase. Notes for later phases:
- Phase 5: "Ban N recent joiners" must mark those joiners so the lift keeps them banned (D-05). The alert's sample reads the D-04 joiner list.
- Phase 6: LOCK-10 builds on the one-active-lockdown rule (D-14), and LOCK-15 retires `/antiraid`, which coexists with lockdown until then.

Not in this phase (CONTEXT domain section): alerts to the Staff Group and log channel, the alert's "Lift lockdown" button, joiner sample, "Ban N recent joiners", "Revoke link", reminders (Phase 5, LOCK-11..14); automatic triggers, LOCK-10, retiring `/antiraid` (Phase 6); locking other linked groups.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| LOCK-01 | Joiners removed at once, can rejoin after lift, recorded for the alert | Join guard at group -7 over three paths; ban-with-marker + `chat_lockdown_joiners` rows; lift unbans (Patterns 2, 3, 5) |
| LOCK-02 | Everyone except admins muted | `setChatPermissions` with an all-false JSON set and `use_independent_chat_permissions=true`; admins are unaffected by default permissions (Pattern 1) |
| LOCK-03 | Approved users talk, or owner-chosen fallback | Spike resolved (D-01/D-02): no per-user calls, notice says approved users are muted too |
| LOCK-04 | Affects only its own group | Rows keyed by `chat_id`; no fan-out; guard reads by chat; test with two chats |
| LOCK-05 | Never lifts on its own; survives restart and Redis flush | State only in PostgreSQL, fresh reads (no cache); resumable DB-driven worker; no TTL anywhere (Patterns 2, 6) |
| LOCK-06 | Only a group admin lifts (`/unlockdown`) | Live `getChatMember` authority check inside pipeline checks, anonymous-admin proof flow (Pattern 4) |
| LOCK-07 | Exact restore of prior permissions | Raw `getChat.permissions` JSON stored before the lock call and replayed verbatim (Pattern 1) |
| LOCK-08 | Unmute during lockdown must not leave user muted after lift | `resolveUnmutePermissions` returns the active lockdown's snapshot (Pattern 7) |
| LOCK-09 | `/lockdown` and status (active, since when, why) | `/lockdown`, `/lockdownstatus`, `/staff` row marker (Patterns 4, 8) |
</phase_requirements>

## Summary

Phase 4 adds one new module (`Lockdown`, priority 238) built from five parts: a PostgreSQL state machine (`chat_lockdowns`, `chat_lockdown_joiners`), raw-JSON permission snapshot/lock/restore helpers, a join guard registered at **handler group -7** for the three join paths, a DB-driven background worker that performs the paced bans, lift unbans and restart recovery, and the three commands. No new dependency is needed: everything uses gotgbot rc.36, GORM, `ratelimit.NewTelegramPacer`, and patterns already present in the Phase 1-3 staff code. All lockdown reads are **fresh DB queries, never cached** (the staff audit-record precedent), which removes the stale-replica window and the `skipLocal` / `DeleteCache` obligations entirely.

The three hard problems, and the recommended answer to each:
1. **Lossless restore.** Read `getChat` through `bot.RequestWithContext` and keep `permissions` as raw JSON; replay it as a `json.RawMessage` through `setChatPermissions` with `use_independent_chat_permissions=true`. The typed `gotgbot.ChatPermissions` struct is lossy (bool fields are `omitempty`; three fields are `*bool` with "defaults to" semantics).
2. **"The lockdown's own ban" (D-05).** Ban joiners with a **distinctive finite `until_date`** stored on the joiner row. At lift, a live `getChatMember` must show `kicked` with exactly that `until_date`, otherwise the ban was replaced (deliberately) and is left alone. One existing code path defeats this and needs a small fix: `decideStaffBan` returns `skip_already_banned` for a timed staff ban that ends before the lockdown ban, so a short staff `/tban` on a lockdown joiner would be lost at lift. Add a `LockdownBan` flag to `staffTargetState` (Pitfall 4).
3. **Never starving the dispatcher in a raid.** A paced ban inside a handler blocks one of up to 200 dispatcher goroutines for up to 60 s. The guard therefore only **records** the joiner (state `pending`) and returns; the worker claims pending rows with conditional updates and bans them under a fleet-wide pacer. This also gives restart safety for free.

**Primary recommendation:** Build the DB state machine and raw-permission helpers first (testable against the existing `staffActionFake` extended with `setChatPermissions`), then the guard + worker, then commands, then the `resolveUnmutePermissions` signature change (it gains an `error` return and four call-site edits), then panel/locales/docs/AGENTS.md.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Lock/unlock default permissions | Telegram (enforcement) | Bot handler (issues `setChatPermissions`) | Telegram enforces the muted default on its own even if the bot is down (D-01) |
| Lockdown state of record | PostgreSQL | none (no cache, no Redis) | "Never lifts on its own; survives Redis flush" (LOCK-05) |
| Per-join decision (ban or exempt) | Dispatcher handler (group -7) | PostgreSQL (row claim), Telegram (live performer check) | Must run before groups -5 and 0, and read fresh state on every replica |
| Paced ban / unban / decline execution | Background worker (per replica) | Redis pacer keys (fleet budget) | Keeps handler goroutines short; pacing degrades to per-replica without Redis |
| Authority for `/lockdown`, `/unlockdown`, performer exemption | Telegram live `getChatMember` | none | Admin cache and `tgAdminList` are explicitly not authority (D-11) |
| Recognising "own ban" at lift | Telegram live `getChatMember.until_date` | joiner row `ban_until` | Works for every ban path without touching them |
| Unmute permission resolution | Bot (`resolveUnmutePermissions`) | PostgreSQL (snapshot) | Single choke point for four callers (D-23) |
| `/staff` panel marker | Dispatcher (pure render) | PostgreSQL (one batch query per panel build) | Panel render stays pure; data is loaded beside the existing row build |

## Standard Stack

### Core
No new libraries. Everything below is already in `go.mod` (CLAUDE.md "Technology Stack", read this session).

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/PaulSonOfLars/gotgbot/v2` | v2.0.0-rc.36.0.20260919140833-240296efadb4 (module cache dir name) | Bot API client, dispatcher, `RequestWithContext` for raw JSON calls | Already the stack; raw `Request` is the only lossless permission path |
| `gorm.io/gorm` + postgres / sqlite drivers | v1.31.2 / v1.6.3 / v1.6.0 | Models, conditional writes, partial unique index via tags | Precedent: `channels.go` partial unique index tag; staff conditional `Updates` |
| `alita/utils/ratelimit` `TelegramPacer` | in-repo | Fleet-wide pacing (Redis slot + shared 429 block, local fallback) | `NewTelegramPacer` is already generic: distinct `NextKey`/`BlockKey` give an independent budget |
| `alita/utils/chat_status` (`FetchBotMember`, `CheckOwner`) | in-repo | Live bot rights / live creator checks | Tri-state results; fail closed on Unknown |
| `github.com/alicebob/miniredis/v2` | v2.39.0 | Tests: Redis pacer / flush | Existing test fixture |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `golang.org/x/sync/errgroup` | v0.23.0 | Bounded concurrency | Only if the worker uses a bounded fan-out; a fixed worker count with a loop is simpler |
| `encoding/json` (stdlib) | Go 1.26.0 | Raw permission JSON, canonical compare | Snapshot, restore, hand-edit detection |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Raw JSON snapshot | typed `gotgbot.ChatPermissions` | Rejected: lossy (`omitempty` bools, `*bool` "defaults to" fields) - Pitfall 8 |
| Distinctive `until_date` marker | Marking every ban path's rows | Rejected: ban/fban/captcha/antiflood/warn/blacklist paths are too many; Telegram overwrites the status so the live `until_date` is the one source that is always right |
| DB-queue worker for bans | Inline paced ban in the handler | Rejected: a 60 s pacer wait inside a handler pins dispatcher goroutines (max 200) during a raid |
| Fresh DB read in the guard | `GetFromCacheOrLoad` + `skipLocal` | Fresh is simpler and has no stale-negative race across replicas (generation guards are per-process); cost is one indexed read per join update |

**Installation:** none. `go.mod` is unchanged.

## Package Legitimacy Audit

No external packages are installed or added by this phase, so the legitimacy gate (`package-legitimacy check`) has nothing to evaluate.

| Package | Registry | Age | Downloads | Source Repo | Verdict | Disposition |
|---------|----------|-----|-----------|-------------|---------|-------------|
| (none) | - | - | - | - | - | No new dependency |

**Packages removed due to [SLOP] verdict:** none
**Packages flagged as suspicious [SUS]:** none

## Architecture Patterns

### System Architecture Diagram

```
 Telegram updates (polling or webhook), dispatcher up to DISPATCHER_MAX_ROUTINES in parallel
   |
   |  chat_member (join)        message.new_chat_members        chat_join_request
   |        \                           |                              /
   v         v                          v                             v
 +-------------------- group -7  LOCKDOWN JOIN GUARD (new) ---------------------+
 | 1. fresh read: active lockdown for chat?  (no -> ContinueGroups, zero cost)   |
 |    (locked_at set?)  DB error -> log, fail open (ContinueGroups)              |
 | 2. skip the bot itself                                                        |
 | 3. join_request  -> enqueue decline (D-06) -> EndGroups                       |
 | 4. decide(path, performer, member): exempt only if performer != member,       |
 |      performer != bot, member not a bot, performer LIVE creator/administrator |
 | 5. exempt -> ContinueGroups (greetings, captcha, antiraid behave as today)    |
 | 6. else ClaimJoin: INSERT joiner row state=pending (record BEFORE ban)        |
 |      insert failed -> log, ContinueGroups (muted by locked default)           |
 |      row exists + recent  -> duplicate delivery -> EndGroups, no second ban   |
 | 7. wake worker (non-blocking), delete join service msg if possible -> EndGroups|
 +--------------------------------------------------------------------------------+
   | (EndGroups ends ALL groups: antiraid -5, fed -6, greetings 0, captcha never see the joiner)
   v
 groups -6 fed, -5 antiraid, -3 watchers, -2/-1, 0 greetings/commands ...

 +-------------- Lockdown worker (per replica goroutine, DB-driven) --------------+
 | tick 2 s + wake channel:                                                       |
 |  a. unconfirmed lock rows (locked_at IS NULL, older than 30 s): re-apply lock  |
 |  b. claim pending joiner rows (conditional UPDATE -> banning)                  |
 |       -> staffPaced-style pacer: banChatMember(until=ban_until) -> banned      |
 |       -> after ban: re-read lockdown; no longer active -> unban own ban at once|
 |  c. lifting lockdowns: claim banned rows -> getChatMember -> own ban? unban    |
 |       else mark kept; final row -> CAS lifted -> post tally exactly once       |
 |  d. stale 'banning'/'unbanning' claims (>2 min) are re-claimable (crash safe)  |
 +--------------------------------------------------------------------------------+

 Commands (WrapCommand, group 0):
   /lockdown [reason]  -> checks -> getChat(raw) -> INSERT active row (snapshot) -> setChatPermissions(locked)
                          -> CAS locked_at -> public notice
   /unlockdown         -> checks -> restore snapshot -> CAS active->lifting -> notice -> worker unbans
   /lockdownstatus     -> read-only
 PostgreSQL: chat_lockdowns (one active per chat, partial unique index) + chat_lockdown_joiners
 Redis (optional): alita:lockdown:pace:next / :block only
```

### Recommended Project Structure
```
migrations/20261006120000_add_chat_lockdowns.sql        # tables + partial unique index (timestamp > 20261005120000)
alita/db/models/lockdown.go                             # ChatLockdown, LockdownJoiner (+ TableName())
alita/db/lockdown/repository.go                         # Start, GetActiveFresh, ListActiveByChatsFresh, ConfirmLocked, BeginLift, joiner state moves
alita/db/lockdown/testmain_test.go                      # testdb.Run(m, &models.ChatLockdown{}, &models.LockdownJoiner{})
alita/modules/lockdown.go                               # LoadLockdown, commands, descriptors, anon registration, init() priority 238
alita/modules/lockdown_perms.go                         # raw getChat/setChatPermissions helpers, canonical compare, locked-set constant
alita/modules/lockdown_guard.go                         # decideLockdownJoin (pure) + three handlers + goodbye suppression
alita/modules/lockdown_worker.go                        # Start/StopLockdownWorker, pacer var, ban / unban / decline / recovery
alita/modules/chat_permissions.go                       # resolveUnmutePermissions now lockdown-aware, returns (perms, error)
alita/modules/staff_panel.go                            # staffLinkRow.LockedSince + marker line
alita/modules/staff_action_decide.go / staff_action_run.go  # LockdownBan fix for D-05
alita/utils/chat_status/                                 # exported anonymous-admin proof prompt
locales/*.yml (7) + locales/config.yml (alt_names Lockdown: [lockdown, unlockdown, lockdownstatus])
AGENTS.md                                               # group -7, Redis keys, fresh-read rule, resolveUnmutePermissions note
```

### Pattern 1: Raw-JSON permission snapshot, lock and restore
**What:** Never touch `gotgbot.ChatPermissions` for the group default. `getChat` is called through `bot.RequestWithContext`; only the `permissions` member is kept, as raw bytes.
**Why:** `ChatPermissions` fields are plain `bool` with `omitempty` and three `*bool` fields documented "If omitted, defaults to the value of can_send_messages / can_pin_messages" `[VERIFIED: gotgbot gen_types.go:2719-2752]`:
```go
CanSendMessages bool `json:"can_send_messages,omitempty"`
...
CanReactToMessages *bool `json:"can_react_to_messages,omitempty"`
CanEditTag *bool `json:"can_edit_tag,omitempty"`
...
CanManageTopics *bool `json:"can_manage_topics,omitempty"`
```
`SetChatPermissionsOpts` carries the flag `[VERIFIED: gen_methods.go:7181-7187]`: `UseIndependentChatPermissions bool` and `SetChatPermissionsWithContext` sends `v["permissions"] = permissions` as a typed struct. The raw call path accepts any `map[string]any`; a `json.RawMessage` value falls to the `default:` branch of `getFieldContents` and is `json.Marshal`ed verbatim `[VERIFIED: gotgbot request.go:268-325]`.
**Example:**
```go
// Source: gotgbot request.go getFieldContents default branch; gen_methods.go SetChatPermissionsWithContext shape.
raw, err := b.RequestWithContext(ctx, "getChat", map[string]any{"chat_id": chatID}, nil)
var env struct {
	Type        string          `json:"type"`
	Permissions json.RawMessage `json:"permissions"`
}
_ = json.Unmarshal(raw, &env) // refuse if env.Type != "supergroup" or len(env.Permissions) == 0 (D-20)

// lock: a constant with every key explicit false, so no omitempty or "defaults to" surprise
const lockdownLockedPermissions = `{"can_send_messages":false,"can_send_audios":false,"can_send_documents":false,
"can_send_photos":false,"can_send_videos":false,"can_send_video_notes":false,"can_send_voice_notes":false,
"can_send_polls":false,"can_send_other_messages":false,"can_add_web_page_previews":false,
"can_react_to_messages":false,"can_edit_tag":false,"can_change_info":false,"can_invite_users":false,
"can_pin_messages":false,"can_manage_topics":false}`
_, err = b.RequestWithContext(ctx, "setChatPermissions", map[string]any{
	"chat_id": chatID, "permissions": json.RawMessage(lockdownLockedPermissions),
	"use_independent_chat_permissions": true}, nil)
// restore: identical call with json.RawMessage(row.PrePermissions)
```
**Order of operations (Anti-Pattern 4 in ARCHITECTURE.md):** read `getChat` -> INSERT the row with `pre_permissions` -> call the lock -> `ConfirmLocked`. Never re-read permissions after locking.
**Hand-edit detection (D-18/D-19):** compare live vs the locked set through a canonical form: decode both into `map[string]bool`, treat a missing key as false, compare. Both sides come from the same server serialization, so no assumption about omitted-false keys is needed for this compare. Do not store a read-back; store the constant that was sent as `locked_permissions`.
**Post-restore check:** after `setChatPermissions(snapshot)` succeeds, `true` is Telegram's confirmation (D-21). An optional read-back compare should only add a warning line to the lift reply, never fail the lift (a normalisation difference would make a lift impossible).
**Per-user restrict caveat:** the four `resolveUnmutePermissions` callers call `RestrictChatMember(..., nil)` without `UseIndependentChatPermissions`. For a typical snapshot (consistent implied permissions) that equals the live behaviour today, so leave it; note the residual risk for a group that had independent permissions configured.

### Pattern 2: One active lockdown per chat (conditional insert)
Model (additive columns are cheap now, expensive after Phase 5):
```go
type ChatLockdown struct {
	ID                uint       `gorm:"primaryKey;autoIncrement"`
	ChatID            int64      `gorm:"column:chat_id;not null;uniqueIndex:uk_chat_lockdowns_active,where:state = 'active'"`
	State             string     `gorm:"column:state;size:8;not null;default:'active';check:chk_chat_lockdown_state,state IN ('active','lifting','lifted')"`
	TriggerKind       string     `gorm:"column:trigger_kind;size:16;not null;default:'manual'"` // Phase 6 adds values; no CHECK on purpose
	Reason            string     `gorm:"column:reason;not null;default:''"`                     // capped to 300 runes before insert
	StartedBy         int64      `gorm:"column:started_by;not null"`
	StartedByName     string     `gorm:"column:started_by_name;not null;default:''"`
	PrePermissions    string     `gorm:"column:pre_permissions;not null"`                       // raw getChat.permissions JSON
	LockedPermissions string     `gorm:"column:locked_permissions;not null;default:''"`         // the JSON sent
	LockedAt          *time.Time `gorm:"column:locked_at"`                                      // NULL until Telegram confirmed the lock
	LiftedBy, LiftedByName, LiftStartedAt, LiftedAt, ManualChange (bool), CreatedAt, UpdatedAt ...
}
```
- The tag form is proven in this repo: `uniqueIndex:idx_channels_username,expression:LOWER(username),where:username <> ''` `[VERIFIED: alita/db/models/channels.go:10]`. The same partial unique index must be written by hand in the migration (`CREATE UNIQUE INDEX ... ON chat_lockdowns (chat_id) WHERE state = 'active'`), as `20260730010000_enforce_channel_username_ownership.sql` does for channels.
- Start: `db.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)`; `RowsAffected == 1` means this caller started it; `0` means someone else did, so read the existing row fresh and reply with it (D-14). `ON CONFLICT DO NOTHING` without a target is PostgreSQL and SQLite upsert syntax `[ASSUMED]` (see A9); a Wave 0 test on SQLite and a PostgreSQL test in the `test-postgres-integrity` list close it.
- Timestamps: truncate to microseconds before storing (`time.Now().UTC().Truncate(time.Microsecond)`), as `ClaimUndo` does `[VERIFIED: alita/db/staff/actions.go:280-285]`.
- Updates through `Updates(map[string]any{...})`, never `UpdateRecord` (it skips zero values, AGENTS.md "Data").
- No FK on chat IDs (staff precedent). The joiner table keeps a SQL-only FK to `chat_lockdowns(id) ON DELETE CASCADE`; the Go models declare no relation.
- A chat can have one `active` row and any number of `lifting`/`lifted` rows, so a new lockdown may start while the old lift is still unbanning. The `until_date` marker keeps the two apart (see Pitfall 2).

Joiner table: `lockdown_id`, `user_id` (unique together), `first_name` and `username` (capped, for Phase 5's sample), `state` (`pending|banning|banned|ban_failed|unbanning|unbanned|kept|unban_failed`), `ban_until` (BIGINT unix seconds, the marker), `join_msg_id` (service message to delete, 0 if none), `attempts`, `claimed_at`, `detail`, `created_at`, `updated_at`. Add `idx (lockdown_id, state)`. Do not add a cap.

### Pattern 3: Join guard at group -7, three paths, record-then-act
- **Group number:** `-7`. Existing negative groups, read this session: `-10` captcha sweeper (`captcha.go:2067`), `-6` `fedHandlerGroup` (`federations.go:35`), `-5` antiraid (`antiraid.go:43`), `-3` staff watchers (`staff_watchers.go:24`), `-2` admin-cache update and `-1` bot-joined / users tracker (`bot_updates.go`, `users.go:57`). `-7` is free and orders the guard before fed-ban and antiraid, so a banned joiner is handled once by the lockdown. Update AGENTS.md's group list in the same commit.
- **Dispatcher semantics `[VERIFIED: gotgbot ext/dispatcher.go:270-320]`:** `ContinueGroups` tries the next handler of the same group, then later groups; `EndGroups` "Stop all group handling"; any other return (incl. nil) leaves the current group and moves on. The guard returns `EndGroups` only for a joiner it banned / declined / recognised as a duplicate, otherwise `ContinueGroups`.
- **Handlers registered in group -7:**
  1. `handlers.NewChatMember(joinFilter, onJoinChatMember)` using `chat_status.ExtractJoinLeftStatusChange` (`!wasMember && isMember`), performer = `ctx.ChatMember.From`, member = `NewChatMember.MergeChatMember().User`, `ViaJoinRequest` available.
  2. `handlers.NewMessage(func(m) bool { return m.NewChatMembers != nil }, onJoinMessage).SetAllowBot(true)` (the staff watchers use `SetAllowBot(true)` for service messages `[VERIFIED: staff_watchers.go:171-190]`), performer = `msg.From`; an anonymous admin has `msg.SenderChat.Id == chat.Id` `[ASSUMED]`.
  3. `handlers.NewChatJoinRequest(chatjoinrequest.All, onJoinRequest)`: decline and `EndGroups` while locked. Declining needs the bot's `can_invite_users` `[VERIFIED: gotgbot DeclineChatJoinRequest doc, gen_methods.go:1057-1059]`, so add it to the D-20 reply as a non-blocking note like the delete right.
  4. `handlers.NewChatMember(kickedFilter, onKickedChatMember)`: when the update moves a member to `kicked` and a joiner row of the active lockdown exists with a matching `until_date`, return `EndGroups` so `greetings.leftMember` sends no goodbye and does not touch captcha rows (D-10). `leftMember` is the group-0 handler for `wasMember && !isMember` `[VERIFIED: greetings.go:1114-1122, 685-740]`. A bot-API ban shows `From == bot`, so `From` alone is not distinctive; match the row.
- **Pure decision function** `decideLockdownJoin(in) verdict` with table tests, mirroring `decideStaffAction`: inputs `path`, `performerID`, `memberID`, `memberIsBot`, `botID`, `performerLiveAdmin` (tri-state), `viaJoinRequest`. Exempt only when `performerID != memberID && performerID != botID && !memberIsBot && performer is a live creator/administrator`. Everything else bans (D-07, D-08). A performer lookup error is not exempt (fail closed to the ban).
- **Live performer check:** `getChatMember(chat, performer)` and read `Status` (reuse `fetchLiveMember`, `staff_action_run.go:572`, which goes through the staff pacer, or a lockdown-paced variant). **Never** use `chat_status.IsUserAdmin`: it consults the admin cache and returns true for `tgAdminList` IDs `[VERIFIED: chat_status.go:182 `if slices.Contains(tgAdminList, userId) {` followed by `return true`]`.
- **Dedupe (D-03):** the first path to INSERT `(lockdown_id, user_id)` wins. A second delivery finds the row: if `updated_at` is within a short window (20 s) it is a duplicate, return `EndGroups` with no second ban; if older, the user rejoined after an admin unban, so reclaim the row with one conditional update (`WHERE state IN ('banned','unbanned','kept','ban_failed') AND updated_at < ?`) and set it `pending` again. No Redis claim anywhere (Pitfall 10).
- **Mixed service message:** a message whose `NewChatMembers` holds an exempt human and a banned bot must keep the greeting for the human only. Filter the banned users out of `ctx.EffectiveMessage.NewChatMembers` in place before returning `ContinueGroups` (the message pointer is shared by later handlers), and unit-test it.
- **Fail-open rules:** a DB error reading the active row or writing the joiner row logs and returns `ContinueGroups` (the joiner is still muted by the locked default). Never ban without a stored row (D-04, Claude's discretion rule).
- **Service-message delete (D-10):** store `join_msg_id` on the row and let the worker delete it through the pacer after the ban, best effort and ignoring "expected" Telegram errors; this keeps the handler free of paced calls.

### Pattern 4: Commands with live authority and the anonymous-admin proof
- Descriptors (not `Disableable`, so a chat cannot disable its raid tool):
```go
lockdownDesc  = helpers.CommandDescriptor{Name: "lockdown",       RequiredChecks: []helpers.CheckFunc{helpers.RequireGroup(), requireLockdownAuthority()}}
unlockDesc    = helpers.CommandDescriptor{Name: "unlockdown",     RequiredChecks: []helpers.CheckFunc{helpers.RequireGroup(), requireLockdownAuthority()}}
statusDesc    = helpers.CommandDescriptor{Name: "lockdownstatus", RequiredChecks: []helpers.CheckFunc{helpers.RequireGroup(), helpers.RequireUserAdmin()}}
```
  registered with `helpers.WrapCommand(dispatcher, desc, pipelineHandler(handler))` and, for the two mutating commands, `RegisterAnonymousAdminHandler("lockdown", anonPipelineHandler(lockdownDesc, m.lockdown))` (pattern at `bans.go:1131`). `anonPipelineHandler` re-runs `desc.RequiredChecks` after the proof `[VERIFIED: moderation.go:349-359]`, which is exactly where the live check belongs.
- `requireLockdownAuthority()` is a new `helpers.CheckFunc` (it replies through `PermissionResponder`, valid only inside the pipeline). Steps: if `ctx.EffectiveSender.IsAnonymousAdmin()` then prompt the proof button **regardless of AnonAdmin mode** and return false; reject other anonymous senders; then live `getChatMember(chat, c.User.Id)`; `staffIssuerSkipReason(member) == ""` (creator, or administrator with `CanRestrictMembers`, `staff_action_run.go:596-607`) passes, anything else is refused with an existing key (`chat_status_restrict_cmd_error` or `chat_status_user_admin_cmd_error`); a lookup error is a refusal ("could not check", fail closed).
- **Why a new exported helper is needed:** the proof prompt lives in unexported `chat_status.checkAnonAdmin`, which short-circuits to "admin" when `admin.GetAdminSettings(chat.Id).AnonAdmin` is on `[VERIFIED: chat_status.go:63-79]`; `sendAnonAdminKeyboard` and `setAnonAdminCache` are unexported too. Add one exported function in `chat_status` (e.g. `PromptAnonAdminProof(b, chat, msg)`) that does `setAnonAdminCache` + `sendAnonAdminKeyboard` unconditionally. After the proof, `verifyAnonymousAdmin` restores `ctx.EffectiveUser`/`EffectiveSender` to the tapper `[VERIFIED: bot_updates.go:187-196]`, so use `chat_status.GetEffectiveUser(ctx)` (the tapper), never `msg.From`, for "who locked" and for the live check. The proof handler itself only checks the cached admin list `[VERIFIED: bot_updates.go:149]`, which is why the live check inside the registered handler's checks is not optional.
- `/lockdown [reason]`: reason = `strings.TrimSpace(strings.Join(ctx.Args()[1:], " "))`, cap at 300 runes (cut by runes), store raw, HTML-escape at render. Steps: fresh active row exists -> reply with it (D-14, no Telegram call). Else `chat.Type == "supergroup"` (basic group refused with an upgrade hint, see A6). Live `chat_status.FetchBotMember`: `Found` + administrator + `CanRestrictMembers`, `Missing`/`Unknown` -> refusal text. Raw `getChat`; no `permissions` -> refuse. Notes (non-blocking): `!CanDeleteMessages`, `!CanInviteUsers`. Then Start (INSERT), lock call, `ConfirmLocked`. A failed lock call deletes the unconfirmed row (`DELETE ... WHERE id=? AND locked_at IS NULL`; the guard ignores unconfirmed rows so no joiner row can exist) and reports the Telegram error ("nothing is recorded"). Announce only after Telegram confirmed (Pitfall 11).
- `/unlockdown`: authority, fresh active row (none -> "no active lockdown"), fresh raw `getChat` and canonical compare to `locked_permissions` -> `manualChange` (a failed read here does not block the lift), restore call (failure -> keep active, tell the admin why, nobody unbanned, D-21), then `BeginLift`: `UPDATE chat_lockdowns SET state='lifting', lifted_by=?, lift_started_at=? WHERE id=? AND state='active'`; `RowsAffected == 1` is the one lifter, `0` is "already lifted". A DB error after a confirmed restore says "permissions restored but the record could not be updated, run /unlockdown again" (the restore is idempotent). Then reply, wake the worker.
- `/lockdownstatus`: read-only. Not locked, locked (since, by, reason, removed count, optional manual-change warning, unconfirmed-lock warning), or lifting (unbanned so far). Never writes.
- `/lockdown` in a Staff Group is allowed once D-11 and D-20 pass; the staff interceptors only table `/ban /mute /kick /unban /unmute` and variants `[VERIFIED: AGENTS.md StaffActions paragraph]`, so there is no conflict.
- User-controlled text (reason, names): splice after translation through a token, as `staffGroupsToken` does, because "the translator runs a printf-style pass over the interpolated text" `[VERIFIED: alita/modules/staff.go comment above staffGroupsToken]`.

### Pattern 5: Own-ban marker (D-05) and the lift
- `lockdownBanUntil = now + 330*24h` as unix seconds, stored on the joiner row before the call. Bot API: a ban shorter than 30 s or longer than 366 days is permanent, so the value must sit strictly inside that window `[VERIFIED: gotgbot BanChatMemberOpts doc, gen_methods.go:419-422]`; `ChatMemberBanned.UntilDate` is reported by `getChatMember` `[VERIFIED: gen_types.go:2362-2367]`.
- Lift per row (worker): claim `banned` row -> paced `getChatMember` -> `isLockdownBan(member, row.BanUntil)` (pure func: `Status == kicked && UntilDate == row.BanUntil`) -> yes: paced `unbanChatMember(only_if_banned=true)` -> `unbanned`; no (left, restricted, member, or kicked with another `until_date`) -> `kept` (never touched); error -> `unban_failed` with a detail, listed in the final tally.
- Why not "every ban path marks the row": `/ban` calls `BanMember(c.Bot, t.userID, nil)` (permanent, `until_date` 0) for any target, kicked ones included, because `banTargetValidation` passes `checkInChat=false` `[VERIFIED: bans.go:67-70, 205, 300]`, so it replaces the marker with no extra code. Fed-ban, captcha "ban", `/tban`, `/dban` also overwrite the status.
- A ban that expires after 330 days simply makes the user a non-member; the guard bans them again if they rejoin while the lockdown still runs.
- Tolerance: compare exactly; if the UAT fixture shows Telegram does not echo the value verbatim (A2), switch to a +/-2 s tolerance in the same pure function.

### Pattern 6: Worker, pacing, restart safety
- **Pacer:** a new package var `lockdownPacer = ratelimit.NewTelegramPacer(ratelimit.TelegramPacerOptions{NextKey: "alita:lockdown:pace:next", BlockKey: "alita:lockdown:pace:block", Interval: 100*time.Millisecond, MaxRetries: 3, MaxWait: 60*time.Second})`, a variable so tests install a fast one (same shape as `staffActionPacer`, `staff_action_run.go:172-178`). A separate budget is deliberate: staff runs are untouched, and two 10/s pacers stay well under Telegram's global bulk limit `[ASSUMED]`. **Do not generalise `staffPaced`**: it is a one-line wrapper over `TelegramPacer.Do`, and the pacer type is already generic. A call whose slot is more than `MaxWait` away fails at once with `ratelimit.ErrRateLimited` and takes no slot `[VERIFIED: telegram_pacer.go:132-139, 164-186]`; the worker returns that row to `pending` with `attempts+1` and retries on a later tick. Without Redis the pacer falls back to a per-replica interval, so only pacing degrades (the state is in PostgreSQL).
- **Claims:** per-row conditional updates (`WHERE id=? AND (state='pending' OR (state='banning' AND claimed_at < now-2min))`), exactly-once under several replicas and crash-resumable, no `SKIP LOCKED` (not available on SQLite).
- **Restart / lift resume:** the loop selects work from the DB each tick, so a restart changes nothing: `pending`, stale `banning`, `banned` rows of `lifting` lockdowns, stale `unbanning`. It also re-applies the lock for rows with `locked_at IS NULL` older than 30 s (idempotent `setChatPermissions`, then `ConfirmLocked`), matching ARCHITECTURE.md's "startup recovery for applied=false".
- **Ban/lift race:** after a successful ban the worker re-reads the lockdown row; if it is no longer `active`, it unbans that user immediately (the ban is its own, `until_date` known). The lift's final-row transition `lifting -> lifted` happens only when no row is `pending|banning|banned|unbanning`. A 30 s grace after `lift_started_at` before declaring "done" covers a ban call that was in flight at the CAS.
- **Exactly-once tally:** the replica whose conditional `UPDATE ... SET state='lifted', lifted_at=? WHERE id=? AND state='lifting' AND NOT EXISTS (unfinished joiner rows)` returns `RowsAffected == 1` posts the final line (counts, any unban failures by name and ID) in the group, in the chat language via `staffChatTranslator(chatID)` `[VERIFIED: staff_notify.go:24-26]`. If there were no joiners the `/unlockdown` reply is the only message.
- **Lifecycle:** `StartLockdownWorker(b)` called from `postInit` (beside `modules.StartStaffSweeper(b)`, `main.go:433`), `StopLockdownWorker()` registered **after** the DB-close handler (`main.go:160-163`, DB close at line 162) so LIFO runs it before the DB closes, like the sweeper registration `[VERIFIED: main.go:208-214]`. The goroutine starts with `defer error_handling.RecoverFromPanic(...)` and cancels quickly on stop: the shutdown manager gives each handler 10 s `[VERIFIED: graceful.go:83 `context.WithTimeout(ctx, 10*time.Second)`]`, and because all work is resumable the stop can cancel and wait about 5 s, unlike staff's 30 s drain (the open STATE blocker).
- **Wake signal:** a buffered channel of 1 written non-blockingly by the guard; other replicas pick work up at the next tick.

### Pattern 7: `resolveUnmutePermissions` becomes lockdown-aware (D-23)
Current function `[VERIFIED: alita/modules/chat_permissions.go:47-53]`:
```go
func resolveUnmutePermissions(chatInfo *gotgbot.ChatFullInfo) gotgbot.ChatPermissions {
	if chatInfo != nil && chatInfo.Permissions != nil {
		return *chatInfo.Permissions
	}
	return defaultUnmutePermissions()
}
```
It has exactly four callers `[VERIFIED: grep + Read]`: `mute.go:228` (`/unmute`), `bans.go:817` (unrestrict callback `unmute`), `captcha.go:1396` (`unmuteCaptchaUser`), `staff_action_run.go:667` (staff unmute). Each already holds a `*gotgbot.ChatFullInfo` from a live `getChat`, so `chatInfo.Id` is the lockdown key (`ChatFullInfo.Id int64`, `gen_types.go:1725`).
- New signature `resolveUnmutePermissions(chatInfo) (gotgbot.ChatPermissions, error)`: when `chatInfo != nil && chatInfo.Id != 0`, read the active lockdown fresh; if found, `json.Unmarshal` its `PrePermissions` into `gotgbot.ChatPermissions` and return it; a DB error is returned (callers already propagate errors: `mute.go` execute returns it, `bans.go` returns it, `captcha` `unmuteCaptchaUser` returns it, staff's `paced` closure returns it), so a failed lookup never announces an unmute that would later leave the user muted (Pitfall 11).
- Keep the `Id != 0` guard: `TestResolveUnmutePermissions` builds `ChatFullInfo{Permissions: ...}` with `Id` 0 under `t.Parallel()` `[VERIFIED: chat_permissions_test.go:65-100]`; those cases must not hit the database. Add lockdown cases next to them.
- Effect: during the lockdown `/unmute` writes a per-user restriction with the snapshot; the user stays muted because Telegram combines the locked default with the per-user set (A7), and talks as soon as the default is restored. Without this the per-user record would hold the locked set and survive the lift (Pitfall 7b).
- `/unmute` reply (discretion): when a lockdown is active append the new key (e.g. `mutes_unmute_lockdown_note`: "They can talk once the lockdown lifts.") so the reply does not claim they can talk now.

### Pattern 8: `/staff` panel marker (D-17)
`staffLinkRow` is built in `buildStaffPanelRows` and rendered by pure `renderStaffRow` `[VERIFIED: staff_panel.go:154-172, 404-444]`. Add `LockedSince time.Time` (zero = not locked). In `buildStaffPanel` (which already loads links fresh, `staff_panel.go:455`) call one `lockdown.ListActiveByChatsFresh(groupIDs)` and set the field on the rows after `buildStaffPanelRows`; this keeps the panel build to one extra query and `renderStaffPanel` pure. In `renderStaffRow` add a line `staff_panel_row_lockdown` ("🔒 in lockdown since {time}") after the status line, time formatted `"2 Jan 15:04"` UTC as the history lines do `[VERIFIED: staff_history.go:136]`. Show it for confirmed rows (`locked_at` set). The bot-missing status already comes from the health fields, so D-22 needs no extra code. Show no reason or issuer there. Re-check `panel_render` tests for the page length cap (3800 UTF-16 units): the extra line per row is covered by `staffPanelTextFor`'s truncation loop.

### Anti-Patterns to Avoid
- **A paced Telegram call inside a join handler:** pins dispatcher goroutines (max 200) for up to `MaxWait`; use the worker.
- **`chat_status.IsUserAdmin` for the performer or the commander:** cache + `tgAdminList` shortcut.
- **Caching lockdown reads:** negative sentinels written by a loader on replica B can outlive replica A's `DeleteCache` (generation guards are per process); fresh reads sidestep it.
- **Gating the ban on `claimRecentJoinProcessing`:** it fails closed on any Redis error (`greetings.go:84-101`).
- **Returning `nil` from the guard:** in group -7 that silently ends the group and is harmless, but every non-handled path must be `ContinueGroups` by repo convention (AGENTS.md Handlers).
- **Re-reading permissions after locking** to "remember" them.
- **Callback namespace starting with `antiraid`:** Phase 4 has no buttons, so register no callback; Phase 5 will need `lockdown|`.
- **`gorm.AutoMigrate` outside tests; editing an applied migration.**

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Fleet-wide Telegram pacing + 429 handling | A new limiter | `ratelimit.NewTelegramPacer` with `alita:lockdown:pace:*` keys | Redis slot reservation, shared `retry_after` block, local fallback, tested |
| Creator/admin-with-right test | New status switch | `staffIssuerSkipReason(member) == ""` over a live `getChatMember` | Same rule as staff actions (creator, or administrator + `CanRestrictMembers`) |
| Live bot rights probe | Custom `getChatMember(bot)` | `chat_status.FetchBotMember` (tri-state Found/Missing/Unknown) | Fail-closed contract exists |
| Telegram error text in replies | Own escaper | `telegramErrorDetail(err)` (HTML-escaped, 120 runes) | `staff_action_run.go:769` |
| Chat language outside a command | Reading ctx | `staffChatTranslator(chatID)` | Works from a worker |
| One-active-row guarantee | Mutex / Redis lock | Partial unique index + `ON CONFLICT DO NOTHING` + `RowsAffected` | Cross-replica, survives restart |
| Exactly-once notices | Locks | Conditional `UPDATE` with `RowsAffected == 1` | Phase 1 and 3 convention |
| Anonymous-admin proof flow | New button/callback | Existing `anon_admin` callback + `RegisterAnonymousAdminHandler` | Already routed; only an exported prompt helper is missing |
| Panic safety for goroutines | Raw `recover` | `defer error_handling.RecoverFromPanic(func, module)` | AGENTS.md Go rules |
| Permission JSON round trip | `gotgbot.ChatPermissions` | Raw `json.RawMessage` | Lossy otherwise |

**Key insight:** every hard part of this phase already has a proven analogue in the staff code (live authority, decision table, conditional-update state machine, resumable runs, pacer); the risk is in the places where lockdown rows interact with older code (`decideStaffBan`, `resolveUnmutePermissions`, `greetings.leftMember`, the group-0 join handlers), not in new infrastructure.

## Common Pitfalls

### Pitfall 1: Permission snapshot taken after, or through, a lossy type
**What goes wrong:** the lift restores a locked or altered set (reactions flipped, independent flags lost).
**Why:** typed struct `omitempty`/pointer defaults; re-reading after the lock; a second lockdown snapshotting the locked state.
**How to avoid:** Pattern 1 and 2 (raw JSON, read-before-insert, partial unique index so a second trigger never snapshots).
**Warning signs:** a `prior_permissions` column typed as booleans; `setChatPermissions` called with a `gotgbot.ChatPermissions` value.

### Pitfall 2: Lift unbans a deliberate ban, or a newer lockdown's ban
**What goes wrong:** D-05 violated.
**Why:** unbanning every recorded joiner; a lifting old lockdown and a new active one both touching one user.
**How to avoid:** unban only after `getChatMember` shows `kicked` with `until_date == row.ban_until`. Two lockdowns' markers differ (different start seconds); if a start collides to the same second for one user, the later row's `ban_until` can be made unique by adding `lockdown_id % 100` seconds (cheap insurance).
**Warning signs:** a lift code path calling `unbanChatMember` without a prior `getChatMember`.

### Pitfall 3: `ext.EndGroups` side effects of the guard
**What goes wrong:** EndGroups ends all groups for that update, so the users tracker (-1), logging (11) and the captcha sweeper never see the join, and a mixed service message loses the human's greeting.
**How to avoid:** return EndGroups only for handled-and-banned joiners, filter `NewChatMembers` for mixed messages, ContinueGroups for exempt joiners and for any fail-open path. UAT confirms live behaviour (D-03).

### Pitfall 4: Staff `/ban` on a lockdown joiner is skipped as "already banned"
**What goes wrong:** `decideStaffBan` on a `kicked` target re-issues the ban only when the new end outlasts the current one `[VERIFIED: staff_action_decide.go:204-212, staffEndsLater at 178-188]`. A permanent staff ban (`newUntil == 0`) does outlast the lockdown ban (current `until` non-zero), but a timed staff ban shorter than 330 days returns `skip_already_banned` with no write, so the lockdown's marker stays and the lift unbans the person the staff meant to ban.
**How to avoid:** add `LockdownBan bool` to `staffTargetState`, set it in `runStaffActionInGroup` after `fetchLiveMember` (compare the live `until_date` with the group's active joiner row), and in `decideStaffBan`'s `Kicked` case return the ban call (reason `staffReasonBanned`) when `st.LockdownBan`. Check first whether Phase 2 tests build `staffTargetState` with unkeyed literals (STATE notes unkeyed `staffVerdict` literals); append the field last, keep literals compiling, add a decision-table test row.
**Staff undo:** undo of a staff ban on such a target restores the prior ban with the recorded `until_date` only if it still has more than 120 s left (`staffUndoMinRemaining`), which re-creates exactly the lockdown marker, so it is correct as is.

### Pitfall 5: Guard handlers blocking or double-acting
**What goes wrong:** raid of hundreds of joins stalls updates for all groups; both delivery paths ban twice; the ban is skipped when Redis errors.
**How to avoid:** Pattern 3 (record-and-return, row-based dedupe, no Redis claim) and Pattern 6 (worker).

### Pitfall 6: The anonymous-admin path skips the live check
**What goes wrong:** AnonAdmin mode returns "admin" for an anonymous sender, so `/lockdown` would run with identity 1087968824.
**How to avoid:** Pattern 4: detect `IsAnonymousAdmin()` in the new check and force the proof prompt; after the proof use the tapper's identity and the live check.

### Pitfall 7: Group-0 join handlers that still run for a banned joiner
**What goes wrong:** `greetings.pendingJoins` auto-approves or posts an approve card; `newMember` welcomes; `cleanService` greets.
**How to avoid:** the guard's group -7 runs first and returns EndGroups for banned joiners and declined requests (verified ordering: group numbers are ascending, `iterateOverHandlerGroups`). A join request that arrives before the lock is confirmed is not seen; an already-posted approve card for a pre-lockdown request can still be accepted by the **bot** (button `join_request|a=accept`), and that approval arrives as a join with `From == bot`, which the guard bans. Recommended: while a lockdown is active, make the accept button answer an alert ("lockdown active, approve in Telegram") instead of approving. See Open Question 2.

### Pitfall 8: Lock announced before it took effect, or lift announced before the restore
**How to avoid:** every reply after the awaited Telegram call returned success (D-20, D-21, Pitfall 11); `/unlockdown` posts "lifted" only after `setChatPermissions(snapshot)` returned true and the CAS succeeded.

### Pitfall 9: Basic groups
`restrictChatMember` is documented "in a supergroup" `[VERIFIED: gotgbot RestrictChatMember doc, gen_methods.go:4643]`, and a ban in a basic group does not prevent returning `[VERIFIED: BanChatMember doc, gen_methods.go:430: "In the case of supergroups and channels, the user will not be able to return..."]`. So the "banned until the lift" promise (D-04) cannot hold there. Refuse `/lockdown` in a basic group with an upgrade hint (A6). Because supergroups never change ID again, no `RekeyChat` work is needed.

### Pitfall 10: Unbounded joiner list or per-update DB cost
One indexed point query per join update (`WHERE chat_id=? AND state='active'`) is enough; index `(chat_id)` partial exists already. Do not scan joiner rows in the guard except for the dedupe row lookup `(lockdown_id, user_id)`.

### Pitfall 11: Stale unconfirmed lock rows
A crash between the INSERT and the lock call leaves `locked_at IS NULL`. The guard ignores such rows (so nothing is banned for a lock that never took effect), the worker re-applies the lock after 30 s, `/lockdown` on an unconfirmed row retries the lock synchronously, and `/unlockdown` restores the (idempotent) snapshot. `resolveUnmutePermissions` still uses the row's snapshot, which equals the live permissions in that state.

## Code Examples

### Own-ban recognition (pure, table-tested)
```go
// Source: gotgbot gen_types.go ChatMemberBanned.UntilDate; staff_action_decide.go staffTargetStateFrom for MergedChatMember.
func isLockdownBan(m gotgbot.MergedChatMember, banUntil int64) bool {
	return m.Status == gotgbot.ChatMemberStatusKicked && banUntil != 0 && m.UntilDate == banUntil
}
```

### Canonical permission compare (hand-edit detection)
```go
func canonicalPermissions(raw string) (map[string]bool, error) {
	var generic map[string]any
	if err := json.Unmarshal([]byte(raw), &generic); err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(generic))
	for k, v := range generic {
		if b, ok := v.(bool); ok && b {
			out[k] = true // false and missing are the same
		}
	}
	return out, nil
}
// manualChange := !maps.Equal(canonicalPermissions(live), canonicalPermissions(row.LockedPermissions))
```

### Conditional lift claim
```go
// Source pattern: alita/db/staff/actions.go ClaimUndo (RowsAffected == 1 means this caller won).
res := db.DB.Model(&models.ChatLockdown{}).
	Where("id = ? AND state = ?", id, "active").
	Updates(map[string]any{"state": "lifting", "lifted_by": by, "lifted_by_name": name,
		"lift_started_at": now, "manual_change": manual, "updated_at": now})
won := res.Error == nil && res.RowsAffected == 1
```

### Guard decision table (shape)
```go
// pure, no I/O; the live performer lookup result is passed in
func decideLockdownJoin(in lockdownJoinInput) lockdownJoinVerdict // ban | exempt
// exempt iff in.PerformerID != in.MemberID && in.PerformerID != in.BotID &&
//            !in.MemberIsBot && in.PerformerStatus is creator|administrator
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Redis-only antiraid state with expiry (`alita:antiraid:state:*`) | PostgreSQL state machine, never expires | This phase | Survives Redis flush; `/antiraid` keeps working until Phase 6 |
| Temp-ban (`RaidActionTime`) for raiders | Ban with a marker `until_date`, unban at lift | This phase | No auto-rejoin during the lockdown; lift is selective |
| Typed `ChatPermissions` for chat defaults | Raw JSON + `use_independent_chat_permissions=true` | This phase | Lossless restore |

**Deprecated/outdated:** none in scope; `/antiraid` is retired only in Phase 6 (LOCK-15).

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | `getChat.permissions` always contains every permission key explicitly (false included) | Pattern 1 | If Telegram omitted false-valued keys, a snapshot with `can_react_to_messages=false` and `can_send_messages=true` replays as "omitted -> defaults to can_send_messages", flipping reactions on. Mitigation: record one real `getChat` response as a test fixture and check; the hand-edit compare is unaffected (same-source compare) |
| A2 | `getChatMember` reports `until_date` exactly as sent to `banChatMember` | Pattern 5 | Lift would classify every lockdown ban as "not ours" (nobody unbanned). Mitigation: UAT/fixture check; widen to +/-2 s in `isLockdownBan` |
| A3 | A chat_member update for an admin-approved join request carries the approving admin in `from`; the matching service message carries the joiner as `From` | Pattern 3, Open Q1 | The D-07 "admin approves by hand" exemption would not work reliably; self-join service message looks identical to an invite-link join |
| A4 | A service message for a member added by an anonymous admin has `SenderChat.Id == chat.Id` | Pattern 3 | An admin adding someone while anonymous would be banned |
| A5 | Chat_member updates are delivered when the bot is admin (config lists them) and the order versus the service message is unspecified | Pattern 3 | Handled by row-based dedupe either way; only the A3 exemption depends on order |
| A6 | Basic groups cannot honour "banned until lift" and per-user restrict; refusing is the right behaviour | Pitfall 9 | Owner may prefer "works in basic groups with removal only"; low impact (no staff setup in basic groups either) |
| A7 | Effective member permissions are the default restrictions combined with the per-user set, so a per-user "can send" cannot exceed a locked default (spike D-02 observed the greyed toggle) | Pattern 7 | If Telegram let a per-user grant exceed the default, `/unmute` during lockdown would unmute at once and the lock notice text "approved users muted" would be inaccurate for explicit unmutes only |
| A8 | Telegram's global bulk limit is about 30 messages/s; two 10/s pacers stay below it | Pattern 6 | Higher 429 rate in a simultaneous raid + staff fan-out; pacer handles it with shared blocks |
| A9 | `INSERT ... ON CONFLICT DO NOTHING` without a conflict target covers a partial unique index on PostgreSQL and on SQLite | Pattern 2 | Start could raise a unique-violation error instead of `RowsAffected == 0`; mitigation: Wave 0 repository tests on SQLite plus a PostgreSQL test in the `test-postgres-integrity` list, and treat a unique-violation error as "already active" |
| A10 | Telegram Bot API docs could not be fetched (core.telegram.org is blocked by the egress proxy); Bot API wording was taken from the gotgbot generated doc comments in the module cache | Sources | Low: gotgbot generates them from the official docs, but a newer Bot API revision could differ |
| A11 | Declining a join request does not prevent the user from requesting again, so a raid can loop requests | Open Q6 | Extra decline calls; bounded by pacing |

## Open Questions

> **Owner answers (2026-10-06):** Q1 accepted as recommended (04-CONTEXT D-24); Q2 accepted as recommended (04-CONTEXT D-25). Q3-Q7 are left to the planner with the recommendations below.

1. **D-07 "admin approves a pending request by hand" cannot be made race-free.**
   - Known: the `chat_member` update has `via_join_request` and a performer; the service message has neither (A3).
   - Unclear: which path arrives first; a self-join service message that wins the DB claim bans an admin-approved user.
   - Recommendation: accept it, implement `decideLockdownJoin` so the chat_member path decides with full information and the service-message self-join path bans only if no row exists, and put an admin-approval case in UAT. If the owner dislikes the residual race, option B is to drop the approval clause (approved requests are banned like any invite-link joiner). Flag to the owner at plan review.
2. **Bot-performed approvals (the greetings "accept" button, auto-approve) look like `From == bot` and are banned.**
   - Recommendation: during an active lockdown make `joinRequestHandler`'s accept answer an alert and approve nothing; auto-approve is already never reached because the guard declines new requests (D-06). One fresh read in that handler.
3. **Phase 2 literal sites for `staffTargetState`** (Pitfall 4). Check before adding a field; if any are unkeyed, convert them in the same task.
4. **Fixtures for A1 and A2.** Record one real `getChat` permissions JSON and one real banned `getChatMember` from the owner's test supergroup during UAT and commit them as test fixtures.
5. **`ban_until` span.** 330 days is a default; owner may prefer a shorter span (rejoin allowed sooner after expiry, the guard re-bans while locked).
6. **Join-request loops.** Declined users can request again immediately (A11). Pacing bounds the cost; no per-user suppression is planned. Revisit in Phase 6 with detection.
7. **Where Phase 5's alert will read the sample.** Joiner rows carry `first_name`/`username`/`created_at`; confirm no extra column is wanted before the migration is merged (additive columns after merge need a new migration).

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go + cgo (gcc) | build, `-race` tests with go-sqlite3 | yes | go1.26.0, gcc 13.3.0, `CGO_ENABLED=1` | none needed |
| golangci-lint | `make lint` | yes | 2.5.0 | none needed |
| PostgreSQL server | PG-only migration/index test | binaries at `/usr/lib/postgresql/16`, server not running (`pg_isready`: no response) | 16 | SQLite tests; start a local PG 16 for the one PG test |
| psql | migration inspection | yes | 16.14 | none needed |
| Redis server / redis-cli | pacer tests (real) | `redis-server` present, nothing listening | 7.0.15 cli | miniredis (existing fixture) |
| Docker | `make build` only | yes | 29.6.2 | not needed for tests |
| core.telegram.org | Bot API doc lookup | no (egress blocked) | - | gotgbot generated doc comments |

**Missing dependencies with no fallback:** none.
**Missing dependencies with fallback:** running PostgreSQL (use SQLite for everything except the one PG test) and a live Redis (miniredis).
Note: the first `-race` test build in this sandbox took about 2.5 minutes cold; warm runs of a single test finish in about a second.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go `testing` + testify (assertions only), real fixtures: `internal/testdb.Run` (SQLite), miniredis, hand-written `gotgbot.BotClient` fakes (`staffActionFake`) |
| Config file | none; build tag `testtools` mandatory; `CGO_ENABLED=1` |
| Quick run command | `go test -tags testtools -race -count=1 -run 'Lockdown' ./alita/db/lockdown ./alita/modules` |
| Full suite command | `make test` |

### Phase Requirements -> Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| LOCK-01 | chat_member join banned + recorded, EndGroups, no welcome/captcha | integration (real dispatcher + fake) | `go test -tags testtools -race -count=1 -run '^TestLockdownGuardBansChatMemberJoin$' ./alita/modules` | no, Wave 0 |
| LOCK-01 | service-message join, join delivered twice acts once | integration | `... -run '^TestLockdownGuardDedupesTwoPaths$' ./alita/modules` | no, Wave 0 |
| LOCK-01 | join request declined, never auto-approved | integration | `... -run '^TestLockdownGuardDeclinesJoinRequest$' ./alita/modules` | no, Wave 0 |
| LOCK-01 | exempt admin-added human continues; bot always banned; self-join banned | unit (decision table) | `... -run '^TestDecideLockdownJoin$' ./alita/modules` | no, Wave 0 |
| LOCK-01 | no goodbye for lockdown-banned joiner | integration | `... -run '^TestLockdownSuppressesGoodbye$' ./alita/modules` | no, Wave 0 |
| LOCK-01 | rejoin possible after lift | integration | `... -run '^TestLockdownLiftUnbansJoiners$' ./alita/modules` | no, Wave 0 |
| LOCK-02 | lock call sends all-false set with independent flag; no per-user restrict | integration | `... -run '^TestLockdownLockSendsLockedSet$' ./alita/modules` | no, Wave 0 |
| LOCK-03 | notice states approved users are muted; no approved-user calls | integration | `... -run '^TestLockdownNoticeAndNoPerUserCalls$' ./alita/modules` | no, Wave 0 |
| LOCK-04 | other chat unaffected (guard + permissions) | integration | `... -run '^TestLockdownAffectsOnlyItsChat$' ./alita/modules` | no, Wave 0 |
| LOCK-05 | enforcement with Redis flushed / absent; state in DB only | integration (miniredis flush) | `... -run '^TestLockdownSurvivesRedisFlush$' ./alita/modules` | no, Wave 0 |
| LOCK-05 | restart mid-lift: worker resumes remaining unbans | integration | `... -run '^TestLockdownLiftResumesAfterRestart$' ./alita/modules` | no, Wave 0 |
| LOCK-05 | one active lockdown per chat (SQLite) | repository | `go test -tags testtools -race -count=1 -run '^TestStartLockdownOneActivePerChat$' ./alita/db/lockdown` | no, Wave 0 |
| LOCK-05 | same on PostgreSQL | repository (PG) | add to `test-postgres-integrity` `-run` list in `Makefile` | no, Wave 0 |
| LOCK-06 | non-admin / admin without restrict refused; cache not used | integration | `... -run '^TestLockdownCommandsRefuseNonAuthority$' ./alita/modules` | no, Wave 0 |
| LOCK-06 | anonymous admin gets the proof button even in AnonAdmin mode; tapper named | integration | `... -run '^TestLockdownAnonymousAdminProof$' ./alita/modules` | no, Wave 0 |
| LOCK-06 | double `/unlockdown` lifts once | integration (concurrent) | `... -run '^TestUnlockdownLiftsOnce$' ./alita/modules` | no, Wave 0 |
| LOCK-07 | exact restore of snapshot including pointer fields | integration | `... -run '^TestLockdownLiftRestoresExactSnapshot$' ./alita/modules` | no, Wave 0 |
| LOCK-07 | hand edit replaced and reported; restore failure leaves locked, nobody unbanned | integration | `... -run '^TestLockdownLift(ManualChange\|RestoreFailure)$' ./alita/modules` | no, Wave 0 |
| LOCK-07 | deliberate ban survives lift (replaced `until_date`, left, member) | unit + integration | `... -run '^Test(IsLockdownBan\|LockdownLiftKeepsDeliberateBan)$' ./alita/modules` | no, Wave 0 |
| LOCK-08 | unmute during lockdown uses snapshot for all four callers | integration | `... -run '^TestUnmuteDuringLockdownUsesSnapshot$' ./alita/modules` | no, Wave 0 (extend `chat_permissions_test.go` too) |
| LOCK-09 | `/lockdown` starts, second `/lockdown` replies with existing, `/lockdownstatus` content | integration | `... -run '^TestLockdownCommand(Starts\|ReportsExisting)$\|^TestLockdownStatusCommand$' ./alita/modules` | no, Wave 0 |
| LOCK-09 | refusals: bot lacks rights / permissions unreadable / basic group, nothing recorded | integration | `... -run '^TestLockdownRefusals$' ./alita/modules` | no, Wave 0 |
| SETUP-08 (deferred part) | `/staff` row shows the marker; bot-missing locked group shown | unit (pure render) | `... -run '^TestRenderStaffRowLockdownMarker$' ./alita/modules` | no, Wave 0 |
| D-05 / Pitfall 4 | staff timed `/ban` on a lockdown joiner re-issues the ban | unit (decision table) | `... -run '^TestDecideStaffBanLockdownJoiner$' ./alita/modules` | no, Wave 0 |
| platform | migration chain, checksums | existing | `go test -tags testtools -race -count=1 -run '^TestRepositoryMigrationChain$' ./alita/db/migrations` | yes |
| platform | all 7 locales carry new keys | script | `make check-translations` | yes |
| platform | docs drift | script | `make check-docs` (after `make generate-docs`) | yes |

### Sampling Rate
- **Per task commit:** the quick run command above (use `-run` with the touched test names for the 1-2 s warm loop).
- **Per wave merge:** `go test -tags testtools -race -count=1 ./alita/db/... ./alita/modules/...` and `make check-translations`.
- **Phase gate:** `make test`, `make lint`, `make check-docs` green, plus the PostgreSQL test run locally, before `/gsd-verify-work`.

### Wave 0 Gaps
- [ ] `alita/db/lockdown/testmain_test.go` and repository tests (SQLite harness via `testdb.Run`, tables `&models.ChatLockdown{}`, `&models.LockdownJoiner{}`).
- [ ] Add both models to the `AutoMigrate` list in `alita/modules/test_harness_test.go` and `alita/db/testmain_test.go` (staff models sit at `alita/db/testmain_test.go:79-80`).
- [ ] Extend `staffActionFake` (or add a sibling `lockdownFake` embedding it): `setChatPermissions` (store raw permissions, honour `use_independent_chat_permissions`), `getChat` returning the stored raw permissions JSON (today it marshals `chatPerms` typed values, `staff_action_fake_test.go:383-395`), `declineChatJoinRequest`, `deleteMessage`, `getChatMember` for a kicked member echoing `until_date`, scripted errors per method (already supported by `scripted`).
- [ ] Test helper `lockdownEnv` (dispatcher with Lockdown + Greetings + Captcha + AntiRaid + Staff loaded, miniredis, installed fast `lockdownPacer`, deterministic clock hooks for the worker tick).
- [ ] Lockdown cases in `chat_permissions_test.go` (the existing parallel pure cases stay on `Id == 0`).
- [ ] PostgreSQL index test added to the Makefile `test-postgres-integrity` `-run` list.
- [ ] Locale fixtures: 7 locale files + `config.yml` `alt_names`.

## Security Domain

`security_enforcement` is enabled in `.planning/config.json` (ASVS level 1, block on high).

### Applicable ASVS Categories
| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | Telegram identity; no credentials added |
| V3 Session Management | no | Callback/anon-proof state is existing (`alita:anonAdmin:*`, 5-minute cache) |
| V4 Access Control | yes | Live `getChatMember` per command and per performer; never admin cache, never `tgAdminList`; anonymous admins prove identity; lockdown and unlock are per-chat, keyed by `chat_id` taken from the update's chat |
| V5 Input Validation | yes | Reason capped (300 runes) and HTML-escaped; names spliced after translation; no callback data in this phase |
| V6 Cryptography | no | none |
| V7 Error Handling and Logging | yes | No secrets added; log only IDs; errors wrapped, no DB error discarded on state-changing paths |
| V11 Business Logic | yes | Conditional-update state transitions (exactly once), fail-closed on unknown authority, fail-open only for the guard's own read failures (documented) |

### Known Threat Patterns for this stack
| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Non-admin or stale admin lifts a lockdown | Elevation of Privilege | Live creator / administrator-with-`can_restrict_members` check inside the pipeline checks, re-run by `anonPipelineHandler` |
| AnonAdmin mode or `tgAdminList` IDs (1087968824, 777000) accepted as an admin | Spoofing | New check detects `IsAnonymousAdmin()` and forces the proof; performer check uses live status, not `IsUserAdmin` |
| Reason or display names injecting HTML / printf verbs | Tampering | `html.EscapeString` at render, token splice after translation, rune cap |
| Raid exhausting the dispatcher or the Bot API | Denial of Service | Record-and-return guard, DB-driven bounded worker, fleet pacer with `MaxWait`, rows retried not dropped |
| Replay or duplicate delivery causing double ban/unban | Tampering | Row-based dedupe, conditional claims, `only_if_banned=true`, own-ban marker |
| Lift reverting a deliberate ban or another lockdown's ban | Tampering | `until_date` marker check against live status |
| Cross-chat effect | Elevation of Privilege | No fan-out; all queries scoped by `chat_id`; test with two chats |
| Lost permissions after crash between snapshot and lock | Repudiation / Integrity | Snapshot persisted before the lock call; unconfirmed-row recovery |

## Project Constraints (from CLAUDE.md / AGENTS.md)

- Stack stays Go 1.26.0, gotgbot v2, GORM on PostgreSQL (SQLite in tests), Redis; no new dependencies.
- Migrations append-only, timestamp greater than every existing one (last is `20261005120000`), one transaction per file, no top-level BEGIN/COMMIT, no `CREATE INDEX CONCURRENTLY`; change `runner.go` and `scripts/migrate_psql.sh` together only if the runner semantics change (a new SQL file does not).
- Every write that affects a cached key must `cache.DeleteCache` it: this phase caches nothing (fresh reads), so none applies; state this in AGENTS.md so nobody adds a cache later without `skipLocal`.
- Commands through `helpers.WrapCommand`; anonymous-admin commands also `RegisterAnonymousAdminHandler` + `anonPipelineHandler`; do not add replies for failed checks (the pipeline sends them).
- Handler group numbers are execution order: add `-7` guard to AGENTS.md; watchers return `ext.ContinueGroups`; commands `ext.EndGroups`; value receivers on `moduleStruct`.
- Callbacks only through `callbackcodec`; none needed here; Phase 5 uses `lockdown|` (never a prefix that starts with `antiraid`).
- Locale keys in all 7 locale files (`en, es, fr, hi, id, pt, ru`); `locales/config.yml` is the alt-name pseudo-locale (add `Lockdown`); copy key names from `locales/en.yml`; a typo'd key ships an empty message; run `make check-translations`.
- Every fire-and-forget goroutine starts with `defer error_handling.RecoverFromPanic(...)`; drains registered after the DB-close handler.
- Never discard a DB error on a state-changing path; use `UpdateRecordWithZeroValues` or `Updates(map)` when writing false/0/"".
- Register new secrets with `logredact.RegisterSecret` (none are added here).
- Tests: `-tags testtools`, real fixtures, no mock libraries, assert observable behaviour (reply sent, row persisted, gate enforced), `make test` is the only valid full run.
- Conventional Commits; user-visible changes use `feat:`/`fix:`.
- Per-group authority is always the live `getChatMember`; the per-group admin check is never skipped (PROJECT security constraint).
- Work through a GSD workflow before edits (GSD enforcement in CLAUDE.md); this research does not commit.

## Sources

### Primary (HIGH confidence)
- gotgbot v2 module source, `v2@v2.0.0-rc.36.0.20260919140833-240296efadb4`: `gen_types.go` (ChatPermissions 2719-2752, ChatMemberUpdated 2644-2661, ChatMemberBanned 2362-2367, ChatMemberRestricted 2545+, ChatFullInfo 1723-1725, ChatJoinRequest 1987-2002), `gen_methods.go` (SetChatPermissions 7181-7220, BanChatMemberOpts 419-422, UnbanChatMember 8302-8304, RestrictChatMember 4641-4643, DeclineChatJoinRequest 1057-1059, GetChat 2822-2838), `request.go` (getFieldContents 268-325), `ext/dispatcher.go` (iterateOverHandlerGroups 270-320), `ext/handlers/message.go` (AllowBot).
- Repository files read this session: `alita/modules/chat_permissions.go`, `antiraid.go`, `greetings.go`, `membership.go`, `mute.go`, `bans.go`, `captcha.go`, `bot_updates.go`, `moderation.go`, `staff_action_run.go`, `staff_action_decide.go`, `staff_panel.go`, `staff_watchers.go`, `staff_sweeper.go`, `anonymous_admin_router.go`, `users.go`, `federations.go`; `alita/utils/ratelimit/telegram_pacer.go`; `alita/utils/chat_status/{chat_status,access,owner}.go`; `alita/utils/helpers/command_pipeline.go`; `alita/db/cache/{loader,local}.go`; `alita/db/staff/{repository,actions}.go`; `alita/db/models/{staff,staff_action,channels}.go`; `main.go`; `alita/utils/shutdown/graceful.go`; `migrations/20261005120000_add_staff_actions.sql`, `20260730010000_enforce_channel_username_ownership.sql`; `.planning/research/{ARCHITECTURE,PITFALLS}.md` sections named in CONTEXT; `.planning/{REQUIREMENTS,STATE,config}`, `AGENTS.md`.
- Observed in this session: `go test -tags testtools -race -count=1 -run '^TestResolveUnmutePermissions' ./alita/modules` passed (cold build 2m26s).

### Secondary (MEDIUM confidence)
- WebSearch result pages (aiogram, python-telegram-bot docs) restating `via_join_request`; they do not state who `from` is on an approval.

### Tertiary (LOW confidence, see Assumptions Log)
- Telegram server behaviours A1-A8, A11 (no live Telegram access here; core.telegram.org blocked).

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - no new dependencies; every reused component read in source.
- Architecture: MEDIUM-HIGH - patterns mirror proven staff code; the worker/queue split and the `LockdownBan` staff tweak are new design, reasoned from the verified dispatcher and pacer semantics.
- Pitfalls: MEDIUM - interaction pitfalls verified in code; Telegram echo/omission behaviours need the two fixtures (Open Question 4).

**Research date:** 2026-10-06
**Valid until:** 2026-11-05 (gotgbot is pinned; revisit if the Bot API or gotgbot is bumped)
