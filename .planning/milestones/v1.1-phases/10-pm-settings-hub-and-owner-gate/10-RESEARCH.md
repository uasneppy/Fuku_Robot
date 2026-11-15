# Phase 10: PM Settings Hub and Owner Gate - Research

**Researched:** 2026-11-04
**Domain:** Telegram bot (Go 1.26, gotgbot v2 rc.36, GORM, Redis): a private group list, an owner gate on every settings door, a hub feed (admin roster) with a startup backfill and hourly sweep
**Confidence:** HIGH for everything grounded in the repo code; MEDIUM for three Telegram behaviours that could not be probed here (see Assumptions Log A1-A3)

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Each row of the list is one button showing the group's title only, like the owner's GroupHelp screenshot. No role marker and no attention marker. The title is escaped and spliced in after translation. A pick opens that group's main menu through the existing live-checked pipeline.
- **D-02:** A group where the bot is not an admin is hidden. The list holds only groups the bot has verified, because the bot learns a group's owner and admins from its admin list, which it loads only where it is an admin. The empty-list hint tells owners to make the bot an admin and run `/admincache`.
- **D-03:** `/settings` in the private chat always shows the list, even when the user has exactly one manageable group.
- **D-04:** The list is alphabetical by group title, eight groups per page, paged.
- **D-05:** Right after deploy the list is filled by a one-time startup backfill: the bot loads the admin list of every group it is in where it is an admin, paced to respect Telegram's limits and shared across replicas. An hourly sweep then rechecks, modelled on the Staff Group sweeper (`StartStaffSweeper`: first run shortly after start, a Redis `SETNX` lock across replicas, no lock needed to stay correct without Redis), so ownership changes Telegram never announced are caught without anyone acting.
- **D-06:** The record is also refreshed whenever the bot loads a group's admin list, on a promotion or demotion update, and cleared when the bot leaves the group (from MENU-10). It is only a hint: every pick is checked live, and a stale row is refused at the pick, never trusted.
- **D-07:** The repair path for a missing group is `/admincache` (or `/settings` in that group) and nothing else. There is no `/reload` alias and no Refresh button. The empty or incomplete list says so.
- **D-08:** The switch lives on its own Access page, opened from an "Access" entry on the main menu. That entry and page are shown only to the live owner; an admin who is allowed in never sees them.
- **D-09:** Turning the switch ON asks for a second tap first (the shared two-tap card of `settings_card.go`: stateless, asker and expiry minute in the data, five minutes, Yes re-checks the owner live). Turning it OFF is a single tap, because it only narrows access.
- **D-10:** When the owner turns the switch OFF, an admin who already has a menu open is refused on their next press with a short alert, and the menu is left as it is. No extra machinery: every press already re-checks live, and the gate is part of that check.
- **D-11:** Nothing announces a flip anywhere (no group notice, no log-channel post). The Access page shows the current state.
- **D-12:** A change applies on every replica at the next press (roadmap criterion 3). How the stored value is read to guarantee that is phase research's call (see Open items).
- **D-13:** An admin who is not the owner and types `/settings` in a group while the switch is off gets a plain public refusal: only the group's owner can open settings. The reply does not mention the switch or the Access page, and never names the Staff Group.
- **D-14:** When a user's private list is empty (including an admin whose groups are all gated), it shows one neutral text for everyone: no groups to show; only group owners can change settings unless an owner lets admins in; if a group you own is missing, make sure the bot is an admin there and send `/admincache` in it. The text never differs by what the bot knows about the person, so it reveals which groups exist to no one.
- **D-15:** A group with no live owner (the creator's account is gone) lets its live-checked administrators in, as in v1.0, so the group can still be configured. Every press is still a live `getChatMember` check.
- **D-16:** The owner gate sits on top of the live admin check and never replaces it. Every door re-checks the person with `getChatMember` against the group it affects, never the admin cache or Redis state (Phase 9 D-04, roadmap constraint).
- **D-17:** An owner posting anonymously is identified by the proof tap, and the tapper's live status decides (existing `PromptAnonAdminProof` and `RegisterAnonymousAdminHandler` re-entry).
- **D-18:** A private menu speaks the opener's language, a group menu the group's language (Phase 9 D-10). Nothing shows the Staff Group's title or ID to anyone but the live creator of the group being configured (Phase 9 D-21, D-32).
- **D-19:** One setting, one code path: the Access page's write goes through one apply function that holds validation, the refusals, the write and its `cache.DeleteCache`, with the parity, drift and authority tests Phase 9 set up. Because the menu is now intentionally stricter than the commands (owner-only), the authority-parity tests get a documented carve-out; the commands are not tightened.
- **D-20:** The main menu is a two-column grid with Close, in the opener's language, and every page has Back (roadmap criterion 4). Every new message and button label exists in all 8 locales (en, es, fr, hi, id, pt, ru, uk), and `AGENTS.md` is updated to say 8 (PLAT-05). *Clarification recorded 2026-11-04 (orchestrator, not a new owner choice):* "every page" means every page of a group's menu; the private group list is the top level of the hub and has Close only; in a private menu the main page gains a Groups button that returns to the list.

### Claude's Discretion
- Exact button labels, emoji, page-control layout, and where the Access entry sits in the two-column grid (settled in `10-UI-SPEC.md`). Whether Main in a private menu gets a "Groups" button back to the list.
- Names of the new table(s), columns, repository functions and the callback namespace of the list pages (research decisions 9, 10 and the hub half of 16 in `research/SUMMARY.md`). The extraction of the shared authorization step out of `settingsPress.verify` (research proposes `settingsAuthorize`) and the in-group check (`requireSettingsAccess`).
- The pacing, lock and first-run timing of the startup backfill and the hourly sweep, and their shutdown drain.
- The exact words of the deep-link refusal for a gated admin and of the in-menu refusal alerts, as long as no refusal names the Staff Group or reveals whether the bot is in a given chat to someone who is not an admin of it.

### Deferred Ideas (OUT OF SCOPE)
- Role and attention markers on list rows (owned versus allowed-admin, bot not admin, lockdown active).
- Showing a group where the bot is not an admin as "unavailable" instead of hiding it.
- Opening the menu directly when the user has exactly one group.
- A `/reload` alias of `/admincache` and a Refresh button on the PM list.
- Ordering the list by last opened or newest group.
- Announcing a switch flip (a public group notice, or a log-channel post; settings changes in the log channel are MENU2-01).
- Closing an open admin menu when the switch is turned off, instead of refusing in place.
- A tailored empty-list text for admins whose groups are gated.
- Treating only admins with every admin right as the stand-in for a missing owner.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| MENU-09 | `/settings` in the bot's private chat lists the groups the user may manage (paged); picking one opens that group's main menu | R-01 roster table, R-03 `shub` namespace, R-06 list query, `pk` action in the existing pipeline (R-07) |
| MENU-10 | List built from the bot's own record, refreshed on admin-list load / promotion / demotion, cleared when the bot leaves; every pick live-checked; empty list points at `/admincache` | R-04 observer hook in `utils/cache`, R-05 sweeper + backfill, `adminCacheAutoUpdate` already fires on promotion/demotion/bot removal |
| MENU-11 | Owner-only by default on all four doors and every button press | R-07 one gate function, `requireSettingsAccess`, deep link and `verify` step 4b |
| MENU-12 | Owner switch "Admins can change settings"; only the live owner sees and presses it | R-02 `chat_settings_access`, Access page (`OwnerOnly`), `settingsRightOwner`, two-tap card |
| MENU-13 | With the switch off a regular admin does not see the group in the list; in-group `/settings` and the deep link refuse with a short reply; in-group commands unchanged | list query (R-06), `settings_refuse_owner_only` / `settings_start_owner_only`, no change to any command |
| MENU-14 | A change applies on every replica at once; anonymous owner identified by the proof tap | R-02 fresh (uncached) read + two-replica test; `requireSettingsAccess` re-run by `anonPipelineHandler` after the swap |
| MENU-15 | Main shows every page in a two-column grid with Close, in the opener's language; every page has Back | `renderSettingsMain` rewrite (grid, status block, Groups button) |
| PLAT-05 | Every new message and button label in all 8 locales; `AGENTS.md` says 8 | R-10: code and tests already enumerate 8; the work is 19 keys x 8 files, the limits table, the guard test and the docs |
</phase_requirements>

## Summary

The phase is a bounded extension of the Phase 9 menu: one extraction (`settingsGate`, the step 4b decision, shared by five doors), two small tables, one feed hook, one sweeper, one new callback namespace for the list, a Main page rewrite, and 19 locale keys. No new Go dependency is needed. The riskiest parts are not the code size but (a) the test harness, where the default `settingsEnv` fixtures would route every existing admin press into the new gate, (b) the owner-state read, whose "no live owner" signal for a deleted-account owner cannot be verified from this environment, and (c) the many call sites that must change together (deep link, `verify`, `settingsDesc`, Main renderer, locale tables, docs).

Every open item in CONTEXT.md is settled below with a concrete recommendation grounded in the code: the roster table is `chat_admin_index` (one row per (chat, human admin), fed by an observer hook that `utils/cache` calls from `loadAdminCacheFromTelegram`, so `utils/cache` never imports the database layer); the owner switch is `chat_settings_access`, read **fresh** (uncached), which makes "applies on every replica at the next press" true by construction and needs no `skipLocal` entry; the list uses a separate `shub|v1` namespace while the pick stays `a=pk` inside `settings|v1`; the owner-state read reuses the proven `getChatAdministrators` scan (the repo already has `chat_status.CheckOwner`), with a 3 s timeout, singleflight and a 30 s positive-only memo, plus a "Deleted Account" creator heuristic that D-15 needs to ever fire.

**Primary recommendation:** Build it in this order: migration + models + repositories + harness lists (Wave 0), the gate and its seams with the classification helper, the Access page, the `settingsRightOwner` right and the Main grid, the hub list and `pk`, the feed hook and sweeper, then locales, docs and `AGENTS.md`. Seed `admins_allowed = true` for the default test environment so the ~100 existing settings tests keep passing unchanged, and write the gate tests on top of explicit "switch off" fixtures.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Private group list (read) | API / Backend (module handler) | Database | Built from the bot's roster rows for `query.From.Id`; no Telegram call, so the DB owns the data |
| Roster feed (write) | API / Backend (observer + sweeper) | Database | Telegram admin lists are read in the backend; rows are a hint, never authority |
| Owner gate decision | API / Backend (`settingsGate`) | Telegram (live `getChatMember` / `getChatAdministrators`) | Authority is always live Telegram state; the DB only holds the owner's switch |
| Owner switch storage | Database (fresh read) | Cache (not used) | Authority input; AGENTS keeps authority reads off the cache layer |
| Main grid / Access / list rendering | API / Backend (pure renderers) | none | Telegram messages + inline keyboards only |
| Callback routing (`settings|`, `shub|`) | API / Backend (dispatcher prefix handlers) | none | Prefix registration in `LoadSettings` |
| Backfill + hourly sweep | API / Backend (background goroutine, Redis `SETNX` lock) | Database | Same shape as `StartStaffSweeper` |

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| gotgbot v2 | `v2.0.0-rc.36.0.20260919140833-240296efadb4` (pinned in go.mod) | Bot API client; `TelegramError.Code`, `GetChatAdministratorsWithContext`, `GetChatMemberWithContext` | Already the stack; no bump needed [VERIFIED: go list -m, module source request.go:61-72] |
| gorm.io/gorm + drivers | v1.31.2 (postgres 1.6.3, sqlite 1.6.0) | Two new tables; column-targeted upserts | Existing repository pattern (`fuku/db/raiddetection`) |
| golang.org/x/sync/singleflight | v0.23.0 (already a direct dependency) | Per-chat dedup of the owner-state read | Already used in `fuku/utils/cache/adminCache.go:12,17` |
| go-redis v9 | v9.22.0 | Sweeper `SETNX` lock | Same as `staffSweepLockKey` |
| testify NOT used; stdlib `testing` + miniredis | miniredis v2.39.0 | Tests | AGENTS: real fixtures, no mock libraries |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `fuku/utils/callbackcodec` | in repo | `shub|v1|...` and `settings|v1|...` encoding | Every button; never `strings.Split` |
| `fuku/utils/error_handling` | in repo | `RecoverFromPanic` in the sweeper goroutine | Every fire-and-forget goroutine |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Fresh DB read of the switch | `cache.GetFromCacheOrLoad` + `skipLocal` entry | Adds a cache key, an invalidation obligation and a Redis dependency to an authority input; saves one primary-key read per gated admin press. Rejected (see R-02) |
| Observer hook in `utils/cache` | Calling the DB repository from `utils/cache` | Import cycle risk: `utils/cache` imports only `config`, `constants`, `metrics` (verified with `go list -deps`); `fuku/db` imports `utils/cache`. Rejected |
| New `my_chat_member` watcher at group -3 | Reuse `adminCacheAutoUpdate` | The existing handler already matches every bot admin transition (old or new status administrator/creator); a new group would also need an AGENTS group entry. Reuse |

**Installation:** none. No new package is installed.

**Version verification:** `go list -m github.com/PaulSonOfLars/gotgbot/v2` resolves to the pinned rc.36 pseudo-version above; `CGO_ENABLED=0 go build ./...` succeeds in this environment (run this session).

## Package Legitimacy Audit

No external package is added by this phase. `golang.org/x/sync` (singleflight) and everything else used is already in `go.mod` and already imported by production code. Audit table intentionally empty.

| Package | Registry | Age | Downloads | Source Repo | Verdict | Disposition |
|---------|----------|-----|-----------|-------------|---------|-------------|
| (none) | - | - | - | - | - | - |

**Packages removed due to [SLOP] verdict:** none
**Packages flagged as suspicious [SUS]:** none

## Settled Decisions (the CONTEXT open items)

### R-01. Roster table behind the list (decision 9): `chat_admin_index`

Columns: `id BIGSERIAL PK`, `chat_id BIGINT NOT NULL`, `user_id BIGINT NOT NULL`, `role VARCHAR(16) NOT NULL` with `CHECK (role IN ('creator','administrator'))`, `created_at`, `updated_at`; `UNIQUE (chat_id, user_id)`; `CREATE INDEX idx_chat_admin_index_user_id ON chat_admin_index(user_id)` (the list query is `WHERE user_id = ?`). No foreign key on `chat_id` (the staff, federation, lockdown and raid precedent). Model `ChatAdminIndex` in `fuku/db/models/`, `TableName()` = `chat_admin_index`, repository package `fuku/db/adminindex`.

- **No title column.** The list label comes from `chats.chat_name` through a `LEFT JOIN`-free second query (`SELECT chat_id, chat_name FROM chats WHERE chat_id IN ?`), falling back to the chat ID when empty (UI-SPEC: "An empty title shows the chat ID"). Do not filter by `chats.is_inactive`: the activity monitor sets it for quiet groups [VERIFIED: fuku/utils/monitoring/activity_monitor.go:116-118 `Where("is_inactive = ? AND last_activity < ?", false, inactiveThreshold).Update("is_inactive", true)`], which would hide a quiet group the owner still manages.
- **Sort and page in Go, not SQL.** UI-SPEC fixes "alphabetical by lower-cased title (code point order, not locale collation), ties by chat ID ascending". PostgreSQL's default collation is locale aware and SQLite's `lower()` is ASCII-only, so the repository returns all rows of one user (a person manages tens of groups) and the module sorts with `strings.ToLower` + byte comparison and slices 8 per page.
- **Rows are humans only.** The feeder drops bots (`user.IsBot`); `LoadAdminCache` calls `getChatAdministrators` with `ReturnBots: true` [VERIFIED: fuku/utils/cache/adminCache.go:113-117], so the observer's list contains bots.
- **Who is listed (the repository's decision, UI-SPEC "The list door").** Three small portable queries, no JOIN: (1) the user's rows; (2) for the chats where the user is `administrator`, the `chat_settings_access` rows and the set of chats that have a `creator` row; (3) titles. Include a creator row always; include an administrator row when `admins_allowed` is true OR the chat has no creator row. This keeps D-15 for the list. The deleted-owner case is handled by the same helper as the gate (R-04, A1): the feeder does not write a creator row for a creator that `settingsCreatorGone` recognises.
- **Replace semantics (concurrency-safe on PostgreSQL).** `ReplaceChat(chatID, rows)` = one transaction: `INSERT ... ON CONFLICT (chat_id, user_id) DO UPDATE SET role, updated_at` for the new roster, then `DELETE WHERE chat_id = ? AND user_id NOT IN (new ids)` (all rows when the roster is empty). A naive delete-all-then-insert fails with a unique violation when two replicas replace one chat at once under read committed. This is a guarantee SQLite cannot prove, so it belongs in `make test-postgres-integrity` (see Validation Architecture). Also `ClearChat(chatID)` (a plain delete).
- **Do not seed from `chat_users`.** The table is gone [VERIFIED: migrations/20250814100001_drop_unused_chat_users_table.sql], and `chats.users` (jsonb of user IDs) only records people who spoke, with no role. Seeding would list groups the user merely talks in (PITFALLS 8) and cannot prove ownership where the bot is not an admin. Answer to the open item: **no**.
- **Never part of backup/export/import/reset.** Backup modules are an explicit name list [VERIFIED: fuku/db/staff/repository_test.go:964-975 `TestStaffTablesStayOutOfBackup` checks `backup.IsValidModule` and `AllExportableModules`]; add the twin test for the two new tables. Avoid a table name that starts with `admin` (an `admin` backup module exists for `AdminSettings`).

### R-02. Owner switch (decision 10): `chat_settings_access`, read fresh

Columns: `id BIGSERIAL PK`, `chat_id BIGINT NOT NULL` (`CONSTRAINT uk_chat_settings_access_chat UNIQUE (chat_id)`), `admins_allowed BOOLEAN NOT NULL DEFAULT FALSE`, `updated_by BIGINT NOT NULL DEFAULT 0`, `created_at`, `updated_at`. A missing row means owner only. Package `fuku/db/menuaccess`: `AdminsAllowedFresh(ctx, chatID) (bool, error)` (`gorm.ErrRecordNotFound` -> `false, nil`; any other error is returned wrapped so the gate refuses, R6) and `SetAdminsAllowed(ctx, chatID, allowed bool, by int64) error` (one `INSERT ... ON CONFLICT (chat_id) DO UPDATE` of `admins_allowed, updated_by, updated_at`, the `fuku/db/raiddetection` `upsertSettings` shape). The model's boolean carries **no** `default:` tag (GORM's Create would turn a Go `false` back into the SQL default) [VERIFIED: fuku/db/models/raid_detection.go comment on the booleans].

**Read policy: fresh, never cached.** Reasons grounded in code: AGENTS already keeps authority reads fresh ("Staff links: authority reads use uncached `staff.*Fresh`"; lockdown tables "read only through fresh queries, never cached"); the cost is one primary-key read and only for an administrator who is not the creator; there is no cache key, so there is no `skipLocal` list to maintain and no `DeleteCache` to forget. `skipLocal` works by key prefix [VERIFIED: fuku/db/cache/local.go:55-71, quoted: `return strings.HasPrefix(key, captchaPendingPrefix) || strings.HasPrefix(key, staffGroupPrefix) || strings.HasPrefix(key, staffLinkOfPrefix) || strings.HasPrefix(key, raidDetectionPrefix)`] and would only give freshness while Redis and `DeleteCache` both succeed. Note for the planner: D-19's wording mentions `cache.DeleteCache`; with an uncached table the apply function has nothing to invalidate, which satisfies the rule's intent (AGENTS "Data": fresh-read tables, "so no `DeleteCache` applies, and a cache added later needs `skipLocal` and a `DeleteCache` on every write"). Record that sentence in AGENTS.

**Proof of "next press on every replica"**: a two-replica test (`settingsEnv.otherReplica()`, modelled on `staffActionEnv.otherReplica` [VERIFIED: fuku/modules/staff_action_lifecycle_test.go:90-115]: a second bot fake + a second dispatcher sharing the DB and miniredis). Sequence: replica A owner turns ON (Yes) -> replica B admin press passes; A owner turns OFF -> B admin's very next press answers `settings_gate_owner_only`. The in-process owner-state memo cannot hide a flip to ON (the switch is read before the memo is consulted, step 4b.2 before 4b.3) and can only keep an admin out after a flip to OFF, which is the safe direction.

### R-03. Callback namespaces (decision 16, hub half)

Use the UI-SPEC recommendation: list pages and Close in a separate namespace `shub` (`shub|v1|a=lp&v=<page>` and `shub|v1|a=cx`); the **pick stays `a=pk` inside `settings|v1`** because it must run the whole press pipeline. Evidence:
- `settingsCheckFields` enforces exactly one of `c` and `u` before any I/O [VERIFIED: fuku/modules/settings_menu.go:194-205]; the list buttons carry neither, so putting them in `settings|v1` would force a special case into that invariant.
- Registration is by prefix: `dispatcher.AddHandler(handlers.NewCallback(callbackquery.Prefix("settings|"), settingsModule.settingsCallback))` [VERIFIED: fuku/modules/settings.go:173]. `shub|v1|...` does not start with `settings|`, so add a second `callbackquery.Prefix("shub|")` handler in `LoadSettings`; no handler group is added (AGENTS "The Settings module adds no handler group" stays true).
- `callbackcodec.Encode` caps at 64 bytes and returns an error on overflow [VERIFIED: fuku/utils/callbackcodec/callbackcodec.go:15-41, `MaxCallbackDataLen = 64`]; `url.Values.Encode()` sorts keys, so `a=pk&c=-1001234567890` is exactly the UI-SPEC byte count (33 for the pick, 45 for the largest Access button).
- The hub handler must: decode `shub`; require `query.Message` non-nil and a private chat; refuse a Telegram service identity (`staffServiceUserIDs`, `lockdown_alert_post_as_yourself`); build the list from `query.From.Id` only; answer exactly once; edit in place. It can reuse `settingsPress.answer/edit/closeMenu` by constructing a `settingsPress{b, ctx, query, tr}` with no group. Add `pk` to `settingsCoreActions` so a page can never register it.

### R-04. The owner-state read (UI-SPEC step 4b.3): confirmed implementable

- **Mechanism:** `b.GetChatAdministratorsWithContext(ctx, chatID, nil)` [VERIFIED: gotgbot gen_methods.go:2858]. The repo already has the creator scan in `chat_status.CheckOwner` (8 s `liveCheckTimeout`, returns `OwnerMismatch, 0, nil` when the list has no creator) [VERIFIED: fuku/utils/chat_status/owner.go:103-122]. The gate needs more (the presser's own entry must be present; a 3 s bound), so write a small dedicated `settingsOwnerState(ctx, b, chatID, presserID)` rather than bending `CheckOwner`; keep it behind a package var seam (`settingsOwnerLookup`).
- **Outcomes** (binding per UI-SPEC): `ownerExists` (a creator entry, not the presser, not "gone") -> gated; `noOwner` (list usable, no creator entry or only a gone creator) -> in; anything else (error, timeout, empty list, list without the presser as administrator, creator entry equal to the presser) -> `settings_check_failed`.
- **singleflight + memo:** `singleflight.Group` keyed by `strconv.FormatInt(chatID, 10)` (same package already used for `adminCacheGroup`). Positive memo: `map[int64]time.Time` under a mutex, `settingsOwnerMemoTTL = 30 * time.Second`, with a clock var for tests; never store "no owner", errors or timeouts. `settingsOwnerLookupTimeout = 3 * time.Second` as a var so a test can shorten it.
- **Bot not a member / large groups:** the gate only runs after step 4 (the live `getChatMember` of the presser) succeeded, so a bot that cannot see the chat already failed closed there. Telegram returns the administrators of a group in one reply; the list is small. `ReturnBots` stays false (the presser is human) [VERIFIED: gen_methods.go:2840-2843 "By default, bots other than the current bot are omitted"].
- **Deleted-account owner (flag, A1):** Telegram support threads report a "Deleted Account" owner can persist for a long time without a transfer [CITED: tginfo.me/how-to-get-ownership-back-en via web search; not an API document]. If `getChatAdministrators` then still returns a creator entry (most likely, [ASSUMED]), "no creator entry" never happens and D-15 would be dead code while every non-owner admin of such a group is locked out. Recommended: one helper `settingsCreatorGone(u gotgbot.User) bool` = `u.FirstName == "Deleted Account" && u.LastName == "" && u.Username == ""`, used by the owner-state read (creator gone -> `noOwner`), the roster feeder (do not store such a creator row) and nothing else. A real owner who renamed themselves "Deleted Account" is unaffected (a creator is always in at 4b.1); the only effect is that admins of that group are let in, i.e. the owner weakens only their own group. Ship it behind a human-verify checkpoint (owner confirms on a real deleted-owner group, or accepts the fail-closed limit).
- **Alternatives to an extra call that I checked and rejected:** the admin cache / roster can be stale in the unsafe direction ("no creator" when one exists would let an admin in), `getChat` (ChatFullInfo) carries no owner, and `getChatMember` of the presser says nothing about whether a creator exists. The live `getChatAdministrators` is the only fresh source, and the UI-SPEC already confines it to the rare path.

### R-05. Feeding the roster

1. **Observer hook in `fuku/utils/cache`** (so it never imports the DB layer): a package-level setter `SetAdminListObserver(func(AdminListEvent))` stored in an `atomic.Pointer`, with `AdminListEvent{ChatID int64, Kind (Loaded | BotNotAdmin | BotGone), Admins []gotgbot.MergedChatMember}`. Call it from **inside** `loadAdminCacheFromTelegram` (it runs under the per-chat `singleflight`), not from `LoadAdminCache`:
   - after `storeResult(...)` of a non-empty list: `Loaded`, but only when `generation == adminCacheGeneration.Load()` (otherwise the value predates an invalidation, exactly the guard the cache store already applies [VERIFIED: adminCache.go:56,62]);
   - in the `botStatus != administrator/creator` branch: `BotNotAdmin` (clears the chat's rows, which is D-02);
   - in the bot-lookup error branch: no event, except that the module-side `LoadSettings` observer is given `err` too if you want a definitive clear for `chat_status.IsBotMissingError` (the cache package cannot import `chat_status`); simplest is to pass `BotErr error` in the event and let the module decide.
   - Never on the three-retry failure path (`return AdminCache{}`): an unreadable list must leave rows alone.
   The observer wrapper in `fuku/modules` runs the DB write synchronously with a 3 s context and `recover`; a failed write is logged and never changes the Telegram result. Synchronous (not a goroutine) keeps per-chat ordering under the existing singleflight and needs no new shutdown drain. Install it from `LoadSettings` (a pointer set, not DB access, so the AGENTS pre-main rule is respected); add the two models to the harness `AutoMigrate` lists so tests that run `LoadAdminCache` after `LoadSettings` find the table.
2. **Promotion, demotion, bot removal need no new handler.** `adminCacheAutoUpdate` is registered for both `my_chat_member` (group -2) and `chat_member` (group 0) with the filter `chat_status.ExtractAdminUpdateStatusChange`, which is true when the old or new status is administrator or creator [VERIFIED: fuku/modules/bot_updates.go:256-276 and fuku/utils/chat_status/chat_status.go:601-611]; the handler does `cache.InvalidateAdminCache(chat.Id); cache.LoadAdminCache(b, chat.Id)` [VERIFIED: bot_updates.go:93-105]. So a promotion, a demotion, the bot being added as admin, demoted, or removed while admin all reload the list and hit the observer. The one gap is "bot removed": the reload's bot lookup errors, so add to `adminCacheAutoUpdate` (it has `ctx.MyChatMember`): when the new bot status is `left` or `kicked`, call `adminindex.ClearChat(chat.Id)` before the reload. `AllowedUpdates` contains both updates [VERIFIED: fuku/config/config.go:477-492: `"my_chat_member", "chat_member", "chat_join_request",`], passed in polling (main.go:317) and webhook (httpserver/server.go:232); Telegram only delivers `chat_member` while the bot is an administrator [CITED: gotgbot gen_types.go:15468 "The bot must be an administrator in the chat and must explicitly specify "chat_member" in the list of allowed_updates"].
3. **`/admincache` refreshes on its main path, not on every path.** `adminCache` calls `cache.LoadAdminCache(b, chat.Id)` (a fresh Telegram load, not a cache read) after `RequireBotAdmin` [VERIFIED: fuku/modules/admin.go:~677-687]; with a non-admin bot it replies `chat_status_bot_not_admin` and loads nothing, so a stale row of a demoted bot is cleared only by the demotion update or the sweep.
4. **`/settings` in a group is the second repair path (D-07)** and today does not refresh anything: after the choice message is sent, call a lean `refreshChatAdminIndex(ctx, b, chatID)` (bot lookup, `getChatAdministrators`, `ReplaceChat`) synchronously and ignore its error. Two extra Telegram calls on a rare command.

### R-06. Startup backfill and hourly sweep (`admin_index_sweeper.go`)

Model on `staff_sweeper.go`: `StartAdminIndexSweeper(b)` / `StopAdminIndexSweeper()`, `sync.Mutex` + `context.CancelFunc` + `sync.WaitGroup`, goroutine starts with `defer error_handling.RecoverFromPanic(...)`, each cycle with its own recover, package vars for interval, pace and first delay so tests shorten them [VERIFIED: fuku/modules/staff_sweeper.go:27-33,68-77,86-149].
- **Backfill == first cycle.** One code path: the first cycle runs after a jittered 30 to 90 s (shorter than the staff sweeper's 1 to 5 min [VERIFIED: staff_sweeper.go:71-73] because an empty list after a deploy is user-visible), then every hour. The first cycle on an empty table is the backfill; later cycles are the sweep.
- **Lock:** `SETNX fuku:settings:sweep:lock` with a per-process UUID owner and TTL = interval - 1 min; no Redis or a Redis error runs unguarded (every write is idempotent), as `acquireStaffSweepLock` does [VERIFIED: staff_sweeper.go:19-21,161-177]. Add one improvement: when a cycle is cut off by cancel, compare-and-delete the lock with the owner token so the next replica's first cycle is not skipped for an hour after a crash-restart loop. New operational key family `fuku:settings:*` (outside `fuku:cache:`).
- **Candidates:** `SELECT chat_id FROM chats WHERE chat_id < 0` (every group the bot has seen, inactive ones included; a left group costs one failed call and a row delete). Per chat: bot lookup (ctx-aware `GetChatMemberWithContext(sweepCtx, chatID, b.Id)` with a 5 s timeout rather than `chat_status.FetchBotMember`, which uses `context.Background()` and an 8 s bound [VERIFIED: owner.go:73-94], so cancel is immediate); if the bot is not administrator/creator or definitively missing (4xx) -> `ClearChat`; if administrator -> `getChatAdministrators` -> `ReplaceChat`.
- **Unreadable list (the open item):** leave rows exactly as they are, log, continue ("never delete on an error", the staff sweep's P4 rule [VERIFIED: staff_sweeper.go:226-230]). Only definitive facts change rows. A 429: wait `min(retry_after, 30 s)` context-aware and retry that chat once; a second 429 ends the cycle.
- **Pace:** 250 ms before each Telegram call (as `staffSweepPace`), i.e. 2 calls per group = 2 groups/s; 1000 groups = about 8 min.
- **Shutdown:** a plain `shutdownManager.RegisterHandler` (10 s default) right after `StopStaffSweeper` in `main.go:204-208`, so LIFO stops it before the DB closes; all its Telegram calls take the sweep context so cancel returns at once. Start it in `postInit` next to `modules.StartStaffSweeper(b)` (main.go:483). No custom wait/grace drain is needed (`RegisterStaffActionsDrain` style is for work that must finish; this work is idempotent and resumable).

### R-07. The gate and the doors (what changes in code)

One function, shared by all five doors: `settingsGate(b, chatID, presser gotgbot.User, member gotgbot.MergedChatMember) settingsGateVerdict` with verdicts `in`, `ownerOnly`, `undecided`; precondition: the caller already proved `member.Status` is creator or administrator. Order: creator -> in; read `menuaccess.AdminsAllowedFresh` (error -> undecided); true -> in; else owner-state read (R-04). Test seams: `settingsAccessLookup` and `settingsOwnerLookup` package vars (same style as `lockdownCurrentLookup`).

| Door | Change |
|------|--------|
| In-group `/settings` | `settingsDesc.RequiredChecks` becomes `{requireSettingsAccess()}`. It returns true for a private chat (the list path needs no group), and in a group runs the live authority check, then `settingsGate`, replying `lockdownRefuse(c, "settings_refuse_owner_only")` on `ownerOnly` and `lockdownRefuse(c, "settings_check_failed")` on `undecided`. **Needs a small extraction:** `requireLockdownAuthority` discards the live member record [VERIFIED: lockdown.go:156-176], and the gate needs creator-vs-admin. Extract `lockdownAuthorityMember(c, needRestrict) (gotgbot.MergedChatMember, bool)` and make `requireLockdownAuthority` a wrapper (no behaviour change; its tests stay). `lockdownRefuse` replies to the command message in plain text [VERIFIED: lockdown.go:73-88], which is the public refusal D-13 wants. |
| Anonymous admin re-entry | `RegisterAnonymousAdminHandler("settings", anonPipelineHandler(settingsDesc, ...))` already re-runs `desc.RequiredChecks` after the update swap [VERIFIED: fuku/modules/moderation.go:349-360], so the gate inside `requireSettingsAccess` runs against the tapper (D-17) with no extra code. The proof still comes first because `lockdownAuthorityMember` keeps the `PromptAnonAdminProof` branch. |
| Deep link | `settingsStartDeepLink` (settings.go:102-164): after the strict parse, `lockdownLiveMember`; classify with the shared helper (R-08); non-admin -> `settings_start_not_admin`; **then the gate** (`ownerOnly` -> `settings_start_owner_only`, `undecided` -> `settings_check_failed`); then one `getChat` with the same classification (4xx or non-group -> `settings_start_not_admin`; others -> `settings_check_failed`). Set `Owner: member.Status == creator` on the view it builds. This replaces the three-phrase `IsBotMissingError` mapping and the `settings_cannot_open` answers at this door. |
| Button press | In `settingsPress.verify`, insert step 4b between the member check (settings_menu.go:315-318) and the private `getChat` (324-343): `switch settingsGate(...)`, `ownerOnly` -> `refuse("settings_gate_owner_only")`, `undecided` -> `refuse("settings_check_failed")`. The existing private `getChat` therefore moves **after** the gate, as in the deep link order. |
| Pick (`pk`) | New core action `settingsActPick = "pk"` in `settingsCheckFields` (exactly `a` and `c`, owner must be `c`) and `dispatch` (`p.edit(settingsMainScreen(p, ""))`). Inside `verify`, for `p.action == settingsActPick` only: non-admin status and any client error (R-08) at step 4 -> `settings_pick_unavailable` (not `settings_not_admin`, not `cannotOpen`); a client error or non-group at `getChat` -> `settings_pick_unavailable`; other failures -> `settings_check_failed`. Other private actions keep the Phase 9 mapping (see Pitfall 7). |
| Owner right | New `settingsRightOwner` in `settingsRightRefusal`: `p.member.Status == creator` else alert `settings_access_owner_only`; no extra call. Add a page field `OwnerOnly bool` to `settingsPage`; `openPage` refuses an `OwnerOnly` page for a non-creator with the same alert (this covers a forged `op`, `bk`-redraw and `no`-cancel naming page `a`). `settingsMainEntries` takes `viewerIsOwner bool` and skips OwnerOnly pages (no `MainState` read for them) when false. `settingsView` gains `Owner bool` (live creator, private **and** group) while the existing `Creator` stays "private menu of the creator" for the Staff privacy rule (D-32). |

### R-08. Client-error classification (one helper for the pick and the deep link)

```go
// Source: gotgbot request.go:61-72 (TelegramError.Code), fuku/modules/lockdown.go:397-402 (the 4xx test).
// isTelegramClientError reports a 4xx answer from Telegram other than 429.
func isTelegramClientError(err error) bool {
	var tgErr *gotgbot.TelegramError
	return errors.As(err, &tgErr) && tgErr.Code >= 400 && tgErr.Code < 500 && tgErr.Code != 429
}
```
[VERIFIED: `TelegramError` has `Code int` "The error code returned by telegram" at request.go:66-67, set from `r.ErrorCode` at request.go:163-169.] Do **not** reuse `lockdownDefinitiveRefusal`: it counts 429 as a refusal. `chat_status.IsBotMissingError` matches three lowercase description fragments only [VERIFIED: owner.go:43 `[]string{"bot was kicked", "bot is not a member", "chat not found"}`], which is exactly why the UI-SPEC rejects it for these two doors. `errors.As` sees through wrapping; a `context.DeadlineExceeded` or transport error is not a `TelegramError`, so it is correctly "not a client error".

### R-09. Test-harness impact (the biggest planning risk)

`newSettingsEnvWith` makes `env.admin` a live **administrator** (lockdown env: `setMember(... Administrator, CanRestrictMembers)`) and also `fake.setCreator(env.chat.Id, env.admin.Id)` so `getChatAdministrators` lists the same user as creator (this inconsistency is what the staff link tests rely on through `CheckOwner`) [VERIFIED: settings_env_test.go:55, staff_helpers_test.go:66-70,112-120]. After this phase every existing press by `env.admin` would hit the gate with the switch off and (a) read the admin list, find itself as the creator -> "unusable" -> `settings_check_failed`. Hundreds of assertions would fail for the wrong reason.

Recommendation (keeps ~all Phase 9 tests untouched): in `newSettingsEnvWith` and `settingsEnv.addChat()`, **seed `chat_settings_access.admins_allowed = true`** for the environment's chats through `menuaccess.SetAdminsAllowed`, with cleanup in `settingsCleanup`. The authority-parity tests then run exactly as today (they exercise `env.admin` as an allowed admin), and the new gate tests start from explicit fixtures: `env.owner` (a user whose live `getChatMember` is `creator`), `env.setAdminsAllowed(chat, bool)`, and `env.otherReplica()`. Required harness additions (Wave 0): (1) `staffActionFake`/`staffBotClient.adminList` must also list administrator records, because today it lists only the creator and the bot [VERIFIED: staff_helpers_test.go:112-120] and the owner-state rule requires the presser's own entry; (2) the two models in `fuku/modules/test_harness_test.go` (list ends at line ~376), `fuku/db/adminindex/testmain_test.go`, `fuku/db/menuaccess/testmain_test.go` (`testdb.Run(m, &models.X{})`). Tests that must be rewritten (not just re-run): the Main-button-state assertions (`settings_antiflood_test.go:91-110`, `settings_lockdown_test.go:306-317`, `settings_staff_test.go:171,310,351`, `settings_staff_privacy_test.go:102-106`), `TestSettingsLabelLengths` (fuku/i18n, settings_locale_test.go:255-308), the `/settings` in private expectation (`settings_test.go:510`, `chat_status_group_only_error`), the deep-link table rows that expect `settings_cannot_open` (`settings_private_test.go:189,203`), and the private navigation tests (Main nav is now `[Groups | Close]`).

### R-10. Eight locales: what already says 8 and what must change

`make check-translations` is green this session (all 8 files, 1060 literal keys), and a flattened key comparison shows `uk.yml` has **all 1418 keys** of `en.yml` (and 83 `settings_` keys, equal to en); its smaller line count (2,437 vs 3,095) is formatting, not missing keys [VERIFIED: ran both this session]. So there is no uk backfill to do. `check-translations` only sees literal `GetString("...")` arguments [VERIFIED: scripts/check_translations/main.go:130-150], so keys read through `settingsText(tr, "key")` are invisible to it; `TestSettingsLocaleKeys` is the real guard.

Code and tests already enumerate 8: `cfg.ValidLangCodes = []string{"uk", "en", "es", "fr", "hi", "id", "pt", "ru"}` [VERIFIED: fuku/config/config.go:496], `var staffLocales = []string{"uk", "en", "es", "fr", "hi", "id", "pt", "ru"}` [VERIFIED: fuku/i18n/staff_locale_test.go:18], and eight-element lists in `settings_env_test.go:73` and six other modules tests. The docs generator reads only `locales/en.yml` and `config.yml` [VERIFIED: scripts/generate_docs/parsers.go:47,114]. What says "7" and must become 8 (PLAT-05): `AGENTS.md` lines 301, 675, 1213, 1235, 1293, 1336; `README.md:880` (tree lists seven files) and `:924`; `.planning/PROJECT.md:131`; `.planning/codebase/ARCHITECTURE.md:94,135,253` and `STRUCTURE.md:235,303,329-330` (regenerate `.claude/CLAUDE.md` after, it is generated from them: it carries "Locale keys go in all 7 locale files", "Loads 7 locales" and "7 languages (en, es, fr, hi, id, pt, ru)"); comments in 6 Go test files ("all 7 locales/languages"). Add one guard test in `fuku/i18n` asserting `staffLocales` equals the set of `locales/*.yml` minus `config.yml` and has 8 entries, so the count can never drift again silently.

New keys: the 19 in UI-SPEC "New locale keys" plus a rewritten `settings_help_msg` in all 8 files. In `fuku/i18n/settings_locale_test.go`: add 19 rows to `settingsLocaleLimits`, add the three alert keys (`settings_gate_owner_only`, `settings_access_owner_only`, `settings_pick_unavailable`) to `settingsAlertKeys`, change the help row from `{900, 1260}` to `{1100, 1540}` and the same constants inside `TestSettingsHelpFitsOneMessage` (lines ~384-386), and replace `TestSettingsLabelLengths` with the grid-label (<= 20), toggle (<= 40), `settings_btn_groups` (<= 18) checks. `settings_list_hint` carries `<code>` (allowed by the tag check); `settings_main_state_line` has `{icon}`, `{page}`, `{state}` (placeholders are compared against en by `staffPlaceholders`).

### R-11. Migration

Latest file today is `migrations/20261101120000_default_language_ukrainian.sql` [VERIFIED: `ls migrations | sort | tail -3`]. Add one file `migrations/20261104120000_add_settings_hub_tables.sql` (re-check the max at execution time) holding both `CREATE TABLE IF NOT EXISTS` statements, the unique constraints and the CHECK **inline in the CREATE TABLE** (the raid-detection migration does this [VERIFIED: migrations/20261018130000_add_chat_raid_detection.sql]) and `CREATE INDEX IF NOT EXISTS`. Inline constraints matter: both appliers swallow `duplicate_object` on a bare `ALTER TABLE ... ADD CONSTRAINT` (AGENTS Data bullet on migration 20261030120000). No `BEGIN/COMMIT`, no `CONCURRENTLY`. The runner globs `*.sql` and sorts [VERIFIED: runner.go getMigrationFiles] and `scripts/migrate_psql.sh` prepares the same directory, so a plain additive file needs **no change to either applier**; the "change both together" rule only applies if runner behaviour is edited. A database-free statements test (like `TestCaptchaModeMigrationStatements`) and a PostgreSQL test are described in Validation Architecture.

### R-12. PostgreSQL integrity list

Yes, add to `make test-postgres-integrity` (Makefile line 41 `-run` list and its package list): `TestAdminIndexReplaceConcurrent` (`./fuku/db/adminindex`: two concurrent `ReplaceChat` of one chat never fail and leave exactly the last roster; the `CHECK` on `role` and the unique key refuse bad rows) and `TestMenuAccessUpsert` (`./fuku/db/menuaccess`: two upserts, a Go `false` is written, `updated_by` follows). The pattern is anchored with `$`, so list the exact top-level names and add the two packages.

## Architecture Patterns

### System Architecture Diagram

```
 Telegram updates                                     Telegram Bot API (live)
   |  my_chat_member / chat_member  (bot admin)            ^   ^   ^
   v                                                       |   |   |
 adminCacheAutoUpdate (bot_updates.go) --Invalidate+Load--> loadAdminCacheFromTelegram
   |  bot left/kicked -> ClearChat                         |   (cache.AdminListObserver)
   |                                                       v
   |                                       observer (modules) --> adminindex.ReplaceChat / ClearChat
 admin_index_sweeper (first run 30-90 s, hourly, SETNX lock) -------^        |
 /settings in group --> refreshChatAdminIndex ------------------------^   chat_admin_index (hint)
                                                                              |
 PM  /settings  --> requireSettingsAccess (private: pass) --> settings() --> hub list (From.Id) --reads--+
 shub|v1 a=lp / a=cx -----------------------------------------> hub handler (private only, rebuilt from From.Id)
 settings|v1 a=pk  --> verify: 1 decode, 2 service id, 4 live getChatMember(presser)
                                |                 \--client error / non-admin --> settings_pick_unavailable (pk only)
                                +--> 4b settingsGate --> creator: in
                                |        admin: menuaccess.AdminsAllowedFresh --on--> in
                                |                       |off --> settingsOwnerState (singleflight, 3 s, 30 s positive memo)
                                |                                 owner exists -> ownerOnly | none -> in | unusable -> check_failed
                                +--> getChat (private) --> step 5 control right (settingsRightOwner for Access) --> apply, answer once, edit
 In-group /settings, anonymous re-entry, /start settings_<id> --> same settingsGate --> refusals per door
```

### Recommended Project Structure
```
migrations/20261104120000_add_settings_hub_tables.sql
fuku/db/models/chat_admin_index.go, chat_settings_access.go
fuku/db/adminindex/        # ReplaceChat, ClearChat, ListForUserFresh, ListChatIDsFresh; testmain_test.go
fuku/db/menuaccess/        # AdminsAllowedFresh, SetAdminsAllowed; testmain_test.go
fuku/utils/cache/adminCache.go   # + observer type, setter, three call sites
fuku/modules/settings_gate.go     # settingsGate, owner-state read, isTelegramClientError, settingsCreatorGone
fuku/modules/settings_hub.go      # list render, shub handler, pick helpers
fuku/modules/settings_access.go   # Access page, applyAdminsAllowed, ON card (registerSettingsPage key "a")
fuku/modules/admin_index_sweeper.go, admin_index_feed.go (observer wrapper, refreshChatAdminIndex)
fuku/modules/settings_pages.go    # OwnerOnly field, grid + status-block renderer, entries signature
fuku/modules/settings_menu.go     # step 4b, pk, settingsRightOwner, openPage OwnerOnly check
fuku/modules/settings.go          # settingsDesc checks, private list entry, deep link
fuku/modules/lockdown.go          # extract lockdownAuthorityMember
main.go                           # StartAdminIndexSweeper + stop handler
```

### Pattern 1: Access apply function (one setting, one code path)
**What:** `applyAdminsAllowed(ctx, member status, chatID, allowed, byUserID) accessApplyResult{Outcome, Allowed, Err}` holds validation (absolute value only), the refusal (non-creator -> not applied, defence in depth behind `settingsRightOwner`), the write and the returned stored value; the toggle's `v=0` write, the `ay` Yes and any future command all call it; the redraw uses the returned value, not a re-read (UI-SPEC).
**Example:**
```go
// Source: shape of fuku/db/raiddetection/repository.go upsertSettings (VERIFIED, quoted above)
func SetAdminsAllowed(ctx context.Context, chatID int64, allowed bool, by int64) error {
	row := &models.ChatSettingsAccess{ChatID: chatID, AdminsAllowed: allowed, UpdatedBy: by}
	err := db.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "chat_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"admins_allowed", "updated_by", "updated_at"}),
	}).Create(row).Error
	return fukuerrors.Wrapf(err, "write menu access of chat %d", chatID) // returns nil for nil err
}
```
The toggle control's `Set` must treat `tg` and `st` the same: `v=="1"` draws the ON card and writes nothing (also when sent as `st`), `v=="0"` writes OFF, anything else is an invalid request.

### Pattern 2: Main page grid and status block
`renderSettingsMain` keeps its signature `(v settingsView, entries []settingsMainEntry, notice string)`. Entries are drawn as `{icon} {name}` buttons, two per row left to right in `Order`, an odd last page alone; the Access entry (`OwnerOnly`, only when `v.Owner`) is always its own full-width row after the grid; the status block is `settings_main_state_line` per entry (`❔` when `Known` is false) after one blank line below the hint; nav row for a private Main is `[settings_btn_groups | settings_btn_close]` with the Groups button encoded in the `shub` namespace (it cannot use `settingsKeyboard.button`, which forces the settings namespace and the view's owner field), a group Main keeps `[Close]` alone. `settingsKeyboard.navRow(main bool)` already knows `k.view.Private`.

### Anti-Patterns to Avoid
- **Reading the switch or the roster through the cache for the decision.** Fresh only (R-02).
- **Mapping Telegram errors by wording at the pick or the deep link.** By class (R-08).
- **Letting the Access page open from `op`/`no`/`bk` for a non-creator.** `openPage` enforces `OwnerOnly`.
- **Deleting roster rows on any error.** Only definitive facts change rows (R-06).
- **Putting a group title, user name or the Staff Group in callback data or any new key.** Callback data carries numbers and codes only.
- **`strings.Split` on callback data; a second write path for the switch.**

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Callback encode/decode | string concatenation | `encodeCallbackData` / `decodeCallbackData` (`callbackcodec`) | 64-byte cap, `""` on overflow, tested worst case |
| Two-tap confirm | a new card/Redis state | `settingsCardRows`, `settingsCardExpired` (`settings_card.go`) | stateless, lazy expiry, already tested |
| Per-chat dedup of the owner read | a hand-written map of in-flight calls | `golang.org/x/sync/singleflight` | already used in `adminCache.go` |
| Hourly sweeper lifecycle | a new goroutine skeleton | copy `staff_sweeper.go` structure | idempotent start/stop, panic recovery, cycle hook for tests |
| Pacing of Telegram calls | a new limiter | the sweeper's `staffSweepPaceFunc` pattern (sleep, ctx-aware) | the lock already limits the fleet to one runner |
| Anonymous admin proof | any new anonymous handling | `PromptAnonAdminProof` + `RegisterAnonymousAdminHandler` re-entry | D-17; the gate rides the re-run `RequiredChecks` |
| Live member lookup | a new `getChatMember` wrapper | `lockdownLiveMember` | already used by verify, deep link and the command check |
| Group title escaping | manual `html.EscapeString` | `staffDisplayTitle` (text) and a plain 32-rune cut for buttons | one place decides the 64-rune cap and escaping |

**Key insight:** every piece this phase needs already exists once in the repo; the risk is in wiring them in the right order, not in inventing mechanisms.

## Common Pitfalls

### Pitfall 1: The default settings test environment trips the new gate
**What goes wrong:** every existing admin press fails with `settings_check_failed`/`settings_gate_owner_only`.
**Why:** `env.admin` is an administrator by `getChatMember` and also the creator by `getChatAdministrators` (R-09).
**How to avoid:** seed `admins_allowed = true` in the default env; build gate tests from explicit owner/admin/switch fixtures; extend the fake's admin list.
**Warning signs:** a wall of failures in `TestSettings*` after the first `verify` edit.

### Pitfall 2: Deleted-account owner makes D-15 dead code
**What goes wrong:** the owner-state read always finds a creator entry, so non-owner admins of a group whose owner deleted their account are locked out forever.
**How to avoid:** `settingsCreatorGone` heuristic (R-04), human-verify checkpoint, test both list shapes.

### Pitfall 3: Forged access to the Access page by an allowed admin
**What goes wrong:** the page is only hidden, not protected.
**How to avoid:** `OwnerOnly` check in `openPage`, `settingsRightOwner` on the control and the `ay` action, defence in the apply function; tests press `op&p=a`, `no&p=a`, `tg&p=a&t=s&v=0`, `st&...`, `ay&e=...` as an allowed admin.

### Pitfall 4: ON written by the toggle press
**How to avoid:** `Set` for `v=1` draws the card only, via both `tg` and `st`; only `ay` writes ON; `ay` re-runs the full pipeline and `settingsCardExpired`.

### Pitfall 5: Observer fires on stale data or on failure
**What goes wrong:** an invalidation landed mid-load, or all three retries failed, and the roster is overwritten with old/empty data.
**How to avoid:** call it only when `generation == adminCacheGeneration.Load()` and only on a successful list read; `BotNotAdmin` only from the authoritative negative branch.

### Pitfall 6: Replace-by-delete-then-insert races on PostgreSQL
**How to avoid:** upsert + `NOT IN` prune in one transaction (R-01); a PostgreSQL integrity test.

### Pitfall 7: Phase 9 phrase mapping remains for non-pick private presses
**What goes wrong (observation, not a bug):** a private `op`/`tg` press with a forged `c` still answers `cannotOpen` for a bot-missing error and `settings_not_admin` for a non-admin, which differs by whether the bot is in the chat (R3). The UI-SPEC deliberately limits the class rule to `pk` and the deep link, because a legitimately stale open menu for a vanished group is better closed than answered "unavailable". A real client cannot send arbitrary callback data (only a custom MTProto client can). Keep the UI-SPEC scope; note it in AGENTS and in the security review.

### Pitfall 8: Sorting/paging by SQL collation
**How to avoid:** sort in Go by lowered title bytes, ties by chat ID; test with mixed case, Cyrillic and equal titles.

### Pitfall 9: `Creator` vs `Owner` view flags confused
`settingsView.Creator` gates the Staff Group title (private, D-32); the new `Owner` gates Access (group and private). Using one for both leaks the Staff title into group menus or hides Access in group menus. Set `Owner` in the deep link view too (it builds the view by hand).

### Pitfall 10: Docs drift
`docs/src/content/docs/commands/settings/index.md` and `api-reference/commands.md` are generated from `settings_help_msg` (run `make generate-docs`, then `make check-docs`); `docs/src/content/docs/api-reference/permissions.md` is `MANUALLY MAINTAINED` and its "Settings menu" section (around line 419-430) must be edited by hand to describe the owner gate.

## Code Examples

### Gate skeleton
```go
// Source: UI-SPEC step 4b + fuku/modules/lockdown.go lockdownLiveMember (VERIFIED)
type settingsGateVerdict int

const (
	settingsGateIn settingsGateVerdict = iota
	settingsGateOwnerOnly
	settingsGateUndecided
)

// settingsGate decides step 4b for a presser already proven creator or administrator.
func settingsGate(b *gotgbot.Bot, chatID int64, presser gotgbot.User, member gotgbot.MergedChatMember) settingsGateVerdict {
	if member.Status == gotgbot.ChatMemberStatusCreator {
		return settingsGateIn
	}
	if member.Status != gotgbot.ChatMemberStatusAdministrator {
		return settingsGateUndecided // unknown status fails closed (R6)
	}
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()
	allowed, err := settingsAccessLookup(ctx, chatID)
	if err != nil {
		return settingsGateUndecided
	}
	if allowed {
		return settingsGateIn
	}
	switch settingsOwnerLookup(b, chatID, presser.Id) {
	case ownerExists:
		return settingsGateOwnerOnly
	case ownerNone:
		return settingsGateIn
	}
	return settingsGateUndecided
}
```

### Observer in the cache package
```go
// Source: shape of fuku/utils/cache/adminCache.go loadAdminCacheFromTelegram (VERIFIED)
type AdminListKind int

const (
	AdminListLoaded AdminListKind = iota
	AdminListBotNotAdmin
)

type AdminListEvent struct {
	ChatID int64
	Kind   AdminListKind
	Admins []gotgbot.MergedChatMember // humans and bots; the observer filters
}

var adminListObserver atomic.Pointer[func(AdminListEvent)]

func SetAdminListObserver(fn func(AdminListEvent)) { adminListObserver.Store(&fn) }

func notifyAdminList(ev AdminListEvent) {
	fn := adminListObserver.Load()
	if fn == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("[AdminCache] observer panic for chat %d: %v", ev.ChatID, r)
		}
	}()
	(*fn)(ev)
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `/settings` refuses private chats (`requireLockdownGroup`) | private chat lists manageable groups; in-group keeps the live check plus the gate | this phase | `settingsDesc.RequiredChecks` changes |
| Main page: one button per page with state inside the label | grid of `icon name` buttons + status block in the body | this phase | old sizing test replaced |
| Deep link maps three error phrases | class rule (4xx except 429) | this phase | `settings_cannot_open` no longer used at that door |
| Menu authority: live admin only | live admin + owner gate | this phase | parity tests need a documented carve-out |

**Deprecated/outdated:** `TestSettingsLabelLengths` main-label ceiling (34/44) is obsolete once state leaves the button.

## Runtime State Inventory

Not a rename/refactor phase. New persistent state only: two new tables (no data migration, rows created lazily and by the backfill), one new Redis key family `fuku:settings:sweep:lock` (outside the cache prefix, survives `CLEAR_CACHE_ON_STARTUP`), and no change to any existing stored string or secret. Nothing stored is keyed to a renamed identifier.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | For a group whose owner deleted their account, `getChatAdministrators` returns a creator entry whose user has `first_name` "Deleted Account", no last name and no username (rather than no creator entry) | R-04, Pitfall 2 | If the entry is absent, the heuristic is redundant but harmless; if the first name differs (localised or another word), D-15 stays dead for that case and admins of such a group are locked out (fail closed) |
| A2 | A bot that is a plain member can call `getChatAdministrators` on a supergroup | R-04 | The gate only runs after step 4 succeeded; if it errors, admins get `settings_check_failed` (fail closed) in groups where the bot is not an admin and the switch is off |
| A3 | A supergroup has a small number of administrators so one reply carries them all | R-04 | A truncated list would look "usable but missing the presser" and refuse with `settings_check_failed` (fail closed) |
| A4 | `getChatMember` of a user who is the creator but anonymous still reports status `creator` | R-07 | The anonymous owner would be gated like an admin; the proof tap still lets the owner in only if the live status is creator |
| A5 | First backfill run 30 to 90 s after start is acceptable (staff sweeper uses 1 to 5 min) | R-06 | Slightly more load right after start; tunable var |

**Needs user confirmation before it becomes locked:** A1 (it implements D-15 for the only realistic "no live owner" case).

## Open Questions (RESOLVED)

1. **How does Telegram's Bot API represent a deleted-account owner in `getChatAdministrators`?**
   - What we know: Telegram forum threads say a "Deleted Account" owner can persist; the Bot API docs available through gotgbot say nothing.
   - What's unclear: the exact `first_name` and whether the entry is present.
   - Recommendation: ship the heuristic behind `settingsCreatorGone` with a human-verify task (owner checks against a real group, or accepts fail-closed), test both shapes.
   - **RESOLVED:** answered by assumption A1 with a fail-closed gate. `settingsCreatorGone` (first name "Deleted Account", no last name, no username) lives in the owner gate, plan 10-05, which tests both shapes (a gone creator lets live admins in, D-15; any other unusable answer refuses with `settings_check_failed`); the roster feeder, plan 10-07, does not store such a creator. Per the orchestrator there is no blocking human-verify task: the owner's live confirmation is a non-blocking check in `10-SPIKE.md`, written by plan 10-10. If the real entry differs, admins of such a group are refused, never wrongly let in.
2. **Should a gone creator's group appear in an admin's private list?** With the feeder skipping gone-creator rows it appears (hint only); the pick still goes through the gate. Recommendation: yes, consistent with D-15.
   - **RESOLVED:** yes. The feeder (plan 10-07) skips the gone creator's row, so `ListForUserFresh`'s "no creator row" rule (plan 10-01) lists the group for its recorded admins; the list (plan 10-09) shows it, and the pick still runs the live check and the gate (plans 10-05, 10-09), which let the admin in only because no live owner exists.
3. **Pace for very large installs.** 2 calls per group at 4 calls/s per replica. Recommendation: keep; add a per-cycle cap only if the owner reports slow cycles.
   - **RESOLVED:** keep the fixed pace. The sweep (plan 10-08) paces 250 ms before each Telegram call (2 calls per group, about 8 minutes for 1000 groups), one runner per cycle through the `fuku:settings:sweep:lock` SETNX lock, with the 429 backoff; no per-cycle cap is added unless the owner reports slow cycles.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go | build, tests | yes | 1.26.0 | none needed |
| gcc / CGO | `go-sqlite3` in tests | yes | present | none needed |
| golangci-lint | `make lint` | yes | /usr/local/bin | none needed |
| Node 22 | captcha page script test | yes | v22.23.4 | not touched by this phase |
| psql / pg_isready | `make test-postgres-integrity` | binaries yes, **server not running** (`pg_isready`: no response) | - | PostgreSQL tests run in CI; locally start a server or rely on SQLite for everything else |
| redis-cli / Redis server | live Redis | binary yes, **server not running** | - | miniredis in tests (the repo standard) |
| core.telegram.org | Bot API docs | **blocked by the session network policy** | - | gotgbot generated doc comments (used here); A1-A3 stay unverified |

**Missing dependencies with no fallback:** none for planning.
**Missing dependencies with fallback:** PostgreSQL server (CI runs it); Telegram docs (host must be allowed in the session network settings to verify A1-A3).

## Validation Architecture

Nyquist validation is enabled (key absent in `.planning/config.json` treated as enabled).

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go `testing`, build tag `testtools`, miniredis, hand-written `gotgbot.BotClient` fakes (no mock libraries) |
| Config file | none; `Makefile` target `test` |
| Quick run command | `go test -tags testtools -race -count=1 -run '^(TestSettings|TestLoadSettings|TestAdminIndex|TestMenuAccess)' ./fuku/modules ./fuku/i18n ./fuku/db/adminindex ./fuku/db/menuaccess ./fuku/utils/cache` |
| Full suite command | `make test` (never run a bare `go test ./...`) |

### Phase Requirements to Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| MENU-09 | private `/settings` sends the list (one group still a list; 9 groups: page 1 has 8 + Next, page 2 has 1 + Prev; empty list text; sort by lower-cased title then chat ID; label cut at 32 runes + ellipsis; pick edits the list into that group's Main with `[Groups | Close]`) | integration | `go test -tags testtools -race -count=1 -run '^(TestSettingsHubList|TestSettingsHubPaging|TestSettingsHubEmpty|TestSettingsHubPick)$' ./fuku/modules` | ❌ Wave 0 `settings_hub_test.go` |
| MENU-09 | `shub` callbacks: private only, rebuilt from `From.Id`, service identity refused, bad `v` invalid, Close deletes, one answer | integration | `... -run '^TestSettingsHubCallbacks$' ./fuku/modules` | ❌ Wave 0 |
| MENU-09 | worst-case callback bytes (45 max, `Omitted` 0) for list, Main with Access, Access, ON card | unit | `... -run '^(TestSettingsHubCallbackBytes|TestSettingsAccessCallbackBytes|TestSettingsCallbackBytes)$' ./fuku/modules` | ❌ / existing `TestSettingsCallbackBytes` extended |
| MENU-10 | observer writes the roster on `LoadAdminCache`; humans only; promotion/demotion update reloads; bot demoted -> rows cleared; bot left/kicked -> cleared; failed load / stale generation leaves rows; `/admincache` refreshes; group `/settings` refreshes | integration | `... -run '^(TestAdminIndexFeed|TestAdminIndexBotRemoval|TestAdminIndexKeepsRowsOnFailure|TestAdminIndexAdmincache|TestAdminIndexSettingsCommandRefresh)$' ./fuku/modules` | ❌ Wave 0 |
| MENU-10 | repository: `ReplaceChat` upsert+prune, `ClearChat`, `ListForUser` rules (creator always; admin when switch on or no creator), no `chats.is_inactive` filter | unit (SQLite) | `go test -tags testtools -race -count=1 ./fuku/db/adminindex` | ❌ Wave 0 |
| MENU-10 | sweeper: lock one runner, skips when held, runs without Redis, unreadable list never deletes, bot-not-admin clears, 429 backs off, stops on cancel, start/stop idempotent, cycle panic survives | integration | `... -run '^TestAdminIndexSweep' ./fuku/modules` | ❌ Wave 0 (mirror `staff_sweeper_test.go`) |
| MENU-10 | PostgreSQL guarantees (concurrent replace, CHECK, unique) | PG integrity | `make test-postgres-integrity` (after adding `TestAdminIndexReplaceConcurrent`, `TestMenuAccessUpsert` and their packages) | ❌ Wave 0 |
| MENU-11 | gate table: creator in with no `getChatAdministrators` call; admin + switch on in with no call; admin + off + other creator -> owner-only; admin + off + no creator (and gone creator) -> in; error/timeout/empty list/list without presser/creator==presser/unreadable switch -> `settings_check_failed`; one call for simultaneous presses; positive memo <= 30 s; "no owner"/error never memoised | integration | `... -run '^(TestSettingsGate|TestSettingsOwnerState|TestSettingsOwnerStateSingleflight|TestSettingsOwnerStateMemo)$' ./fuku/modules` | ❌ Wave 0 `settings_gate_test.go` |
| MENU-11 | every door gives the same decision for the same member record and owner state (group command, anonymous re-entry, deep link, press, pick) | integration | `... -run '^TestSettingsGateAllDoors$' ./fuku/modules` | ❌ Wave 0 |
| MENU-11 | pick rules (R3): non-member, bot-missing, gone chat, non-group, unrecognised 4xx wording all give `settings_pick_unavailable`; transport/timeout/5xx/429 give `settings_check_failed`; gated admin `settings_gate_owner_only`; list not edited; one answer | integration | `... -run '^TestSettingsPickSameAnswer$' ./fuku/modules` | ❌ Wave 0 |
| MENU-11 | deep link classification (`settings_start_not_admin` for non-admin/any 4xx/non-group; `settings_check_failed` for 429/transport/5xx; `settings_start_owner_only`; invalid payload unchanged) | integration | `... -run '^TestSettingsStartDeepLink' ./fuku/modules` | existing, rewrite rows |
| MENU-12 | Access page visible/pressable by live creator only (Main button + status line + page); forged `op/no/tg/st/ay` by an allowed admin -> `settings_access_owner_only`, nothing written; OFF one tap; ON two taps (toggle never writes ON, also via `st`); Yes expired -> `settings_card_expired`; Yes re-checks creator; write failure notice keeps old state; read failure -> cannot-open edit | integration | `... -run '^TestSettingsAccess' ./fuku/modules` | ❌ Wave 0 `settings_access_test.go` |
| MENU-12 | drift + owner-authority tests (no command exists, so no parity test) | integration | `... -run '^(TestSettingsDriftAccess|TestSettingsOwnerAuthorityAccess)$' ./fuku/modules` | ❌ Wave 0 |
| MENU-13 | list hides a group from an admin while the switch is off (and shows it when on / no owner); public `settings_refuse_owner_only` reply names no switch/Staff Group; in-group commands (`/captchamode`, `/setflood`...) still work for the same admin; empty list identical for everyone | integration | `... -run '^(TestSettingsListHidesGated|TestSettingsInGroupRefusal|TestSettingsCommandsUngated|TestSettingsEmptyListSameForAll)$' ./fuku/modules` | ❌ Wave 0 |
| MENU-13 | authority-parity carve-out: with the switch off the menu refuses what the command allows; for every control "menu allows" implies "command allows" | integration | `... -run '^TestSettingsAuthorityParity' ./fuku/modules` | existing + new carve-out test |
| MENU-14 | two replicas: flip on A, next press on B sees it (both directions); owner-state memo cannot hide a flip to ON | integration | `... -run '^TestSettingsAccessAcrossReplicas$' ./fuku/modules` | ❌ Wave 0 (`settingsEnv.otherReplica`) |
| MENU-14 | anonymous owner via proof tap passes, anonymous admin refused, with the gate after the swap | integration | `... -run '^(TestSettingsAnonymousAdmin|TestSettingsAnonymousOwnerGate)$' ./fuku/modules` | existing + new |
| MENU-15 | Main grid: two columns, odd page alone, Access always own last row for the live owner only, status block lines and `❔`, private nav `[Groups | Close]`, group nav `[Close]`, every sub-page has Back | integration/pure | `... -run '^(TestSettingsMainGrid|TestSettingsNavigation)$' ./fuku/modules` | ❌ / existing rewritten |
| MENU-15 | privacy: list, Main, Access screens never name the Staff Group in any of 8 languages (group menu, non-creator private menu) | integration | `... -run '^TestSettingsStaffPrivacy$' ./fuku/modules` | existing, extended |
| MENU-15 / PLAT-05 | page-fits-every-locale (list page, empty list, Access, ON card, Main with Access) <= 3800 UTF-16 units in 8 languages; grid label <= 20, toggle <= 40, `settings_btn_groups` <= 18; list label <= 33 runes | unit | `... -run '^(TestSettingsHubPageFitsEveryLocale|TestSettingsAccessPageFitsEveryLocale|TestSettingsLabelLengths)$' ./fuku/modules ./fuku/i18n` | ❌ / rewritten |
| PLAT-05 | 19 keys in 8 locales (present, non-empty, ceilings, placeholders, tags), alerts <= 200, help message fits one message, `staffLocales` equals `locales/*.yml` minus `config.yml` | unit | `go test -tags testtools -race -count=1 -run '^(TestSettingsLocaleKeys|TestSettingsHelpFitsOneMessage|TestLocalesAreTheEight)$' ./fuku/i18n` | existing + one new |
| PLAT-05 | docs say 8 | shell | `! grep -nE '\b7 (locale|language)|all 7 files|seven locale' AGENTS.md README.md .planning/PROJECT.md .claude/CLAUDE.md` | n/a |
| tables stay out of backup | `TestAccessTablesStayOutOfBackup` (twin of `TestStaffTablesStayOutOfBackup`) | unit | `go test -tags testtools -race -count=1 -run '^TestAccessTablesStayOutOfBackup$' ./fuku/db/adminindex` | ❌ Wave 0 |
| migration | statements are idempotent, inline constraints, no `BEGIN` | unit | `go test -tags testtools -race -count=1 -run '^TestSettingsHubMigrationStatements$' ./fuku/db/migrations` | ❌ Wave 0 |

### Sampling Rate
- **Per task commit:** the quick command above (settings + hub + gate), under 30 s for a focused `-run`.
- **Per wave merge:** `go test -tags testtools -race -count=1 ./fuku/modules ./fuku/i18n ./fuku/db/... ./fuku/utils/cache` plus `make lint`.
- **Phase gate:** `make test`, `make lint`, `make check-translations`, `make generate-docs` then `make check-docs`, `node --test scripts/captcha-page/*.test.mjs` untouched but cheap, and `make test-postgres-integrity` in CI (needs a PostgreSQL server).

### Wave 0 Gaps
- [ ] Migration `20261104120000_add_settings_hub_tables.sql`, models, `fuku/db/adminindex`, `fuku/db/menuaccess` (+ their `testmain_test.go` with `testdb.Run`)
- [ ] Add both models to `fuku/modules/test_harness_test.go` `AutoMigrate` (list ends near line 376)
- [ ] Harness: seed `admins_allowed = true` in `newSettingsEnvWith` / `addChat` (+ cleanup), `env.owner`, `env.setAdminsAllowed`, `env.otherReplica()`, admin entries in the fake's `getChatAdministrators`
- [ ] `settings_hub_test.go`, `settings_gate_test.go`, `settings_access_test.go`, `admin_index_feed_test.go`, `admin_index_sweeper_test.go`
- [ ] `fuku/i18n`: 19 rows + 3 alert keys + help ceiling in `settings_locale_test.go`; new locale-set guard test
- [ ] Makefile `test-postgres-integrity`: add the two test names and two packages

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no (Telegram identifies the user) | n/a |
| V3 Session Management | no (stateless menu) | n/a |
| V4 Access Control | **yes** | live `getChatMember` on every door, owner gate on top (D-16), `OwnerOnly` page + `settingsRightOwner`, fail closed on any unknown (R6, R7) |
| V5 Input Validation | **yes** | `settingsCheckFields` allow-lists, `parseStaffChatIDArg`, `settingsParseNumber` (page <= 3 digits), `settingsCardNumber` for `e` |
| V6 Cryptography | no | no new secret, token or crypto |
| V7 Error handling / logging | yes | Telegram wording never reaches the user; titles not logged; refusals are class-based |
| V8 Data protection | yes | roster holds IDs and roles only; no message text; tables out of backup |

### Known Threat Patterns for this stack

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Forged `c` in a pick/press to learn whether the bot is in a chat | Information disclosure | one neutral alert for every non-admin outcome and every 4xx (`settings_pick_unavailable`, `settings_start_not_admin`), by class not wording (R-08) |
| Allowed admin forges an Access control press | Elevation of privilege | `OwnerOnly` check in `openPage`, `settingsRightOwner`, apply function refuses non-creator |
| Admin keeps using an open menu after the owner turns the switch off | Elevation of privilege | step 4b on every press, fresh switch read |
| Stale switch served by another replica | Tampering | uncached read (R-02), two-replica test |
| Roster row of a demoted admin lists a group title | Information disclosure | rows are a hint; pick refuses; titles shown only to users who were recorded admins; stale window <= 1 h (accepted, documented) |
| Owner-state read used as a load amplifier | Denial of service | gate runs only for gated admins, singleflight, 3 s timeout, positive 30 s memo, nothing else memoised |
| Replay of a list button by another user | Spoofing | list rebuilt from `query.From.Id`, never from callback data; private-only |
| ON card replayed after expiry or after ownership change | Elevation of privilege | lazy 5-minute expiry, Yes re-runs the full pipeline and the creator check |
| SQL injection | Tampering | GORM parameterised queries only |
| Overflowing callback data ships a dead button | Denial of service (UX) | `encodeCallbackData` returns `""`, `Omitted` counted, worst-case tests |

## Sources

### Primary (HIGH confidence)
- Repository files read this session: `fuku/modules/settings.go`, `settings_menu.go`, `settings_pages.go`, `settings_card.go`, `settings_captcha.go`, `settings_staff.go`, `lockdown.go`, `moderation.go`, `bot_updates.go`, `staff_watchers.go`, `staff_sweeper.go`, `admin.go`; `fuku/utils/cache/adminCache.go`, `cache.go`; `fuku/utils/chat_status/owner.go`, `chat_status.go`; `fuku/db/cache/local.go`; `fuku/db/raiddetection/repository.go`; `fuku/db/models/*`; `fuku/utils/callbackcodec/callbackcodec.go`; `fuku/config/config.go`; `main.go`; `Makefile`; `migrations/`; `scripts/check_translations/main.go`; test files `settings_env_test.go`, `staff_helpers_test.go`, `staff_action_fake_test.go`, `lockdown_fake_test.go`, `staff_action_lifecycle_test.go`, `staff_sweeper_test.go`, `settings_locale_test.go`.
- gotgbot module source (`request.go`, `gen_methods.go`, `gen_types.go`) at the pinned version.
- `go build`, `make check-translations`, key-parity comparison of the 8 locale files, `go list -deps ./fuku/utils/cache`, all run this session.

### Secondary (MEDIUM confidence)
- Web search on deleted-account group owners (tginfo.me and bugs.telegram.org threads): community behaviour, not API documentation.

### Tertiary (LOW confidence)
- Bot API representation of a deleted creator and `getChatAdministrators` visibility for a non-admin bot: [ASSUMED], A1-A3.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH, no new packages, everything verified in `go.mod` and source
- Architecture: HIGH for hooks, doors, namespace, tables (read from code); MEDIUM for the deleted-owner signal
- Pitfalls: HIGH, most derived from reading the harness and pipeline

**Research date:** 2026-11-04
**Valid until:** 2026-12-04 (the code moves with every v1.1 phase; re-check the migration max, `settingsCoreActions` and the Main renderer before planning if another phase merges first)
