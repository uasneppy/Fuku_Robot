# Architecture Research

**Domain:** Telegram group-management bot (Go, gotgbot v2, GORM/PostgreSQL, Redis). Subsequent milestone on the existing Alita Robot fork: Staff Group fan-out moderation, raid lockdown, Turnstile Mini App captcha, inline settings menu.
**Researched:** 2026-10-04
**Confidence:** HIGH for how the new code attaches to the existing codebase (read from source). MEDIUM for Telegram and Cloudflare platform behaviour (web sources, primary docs were egress-blocked). LOW for two flagged items (approved-user permission exceptions, image classification). Both need a short spike. See "Open Questions / Spikes".

Every codebase claim below was checked against the source in `/home/user/Fuku_Robot` on 2026-10-04. File references are given so the roadmap and plan phases can verify them quickly.

---

## Standard Architecture

### System Overview (what is added, and where it plugs in)

New components are marked `[NEW]`. Handler group numbers follow `AGENTS.md`.

```
                         Telegram (updates in)                       Public internet (Mini App)
                                  |                                            |
                +-----------------+------------------+                 +-------+----------------+
                |   gotgbot dispatcher (groups run in numeric order)  |  net/http mux (existing |
                +-----------------+------------------+                 |  server, same port)    |
                                  |                                    |  /health /metrics      |
 -10 captcha sweeper              |                                    |  /webhook              |
  -6 fed-ban                      |                                    |  [NEW] /captcha/app    |
  -5 [CHANGED] lockdown guard  <--+-- replaces antiraid enforcement    |  [NEW] /captcha/verify |
  -3 [NEW] staff-link ownership watcher (chat_member)                  +-------+----------------+
  -2 admin-cache refresh                                                       |
  -1 users tracker  (feeds @username resolution)                               |
   0 commands. [NEW] staff command interceptor registers FIRST                 |
   2 [NEW] raid flood detector                                                 |
   3 aispam  (+ [NEW] verdict-burst hook)                                      |
   4 antiflood (per-user, in-process, unchanged)                               |
 5/6 locks, 7 blacklists, 8 reports, 9 filters, 10 pins, 11 log-channel        |
                                                                               |
 +--------------------- modules layer (alita/modules) -----------------------+ |
 | [NEW] staff.go           designate / link / unlink / /staff panel         | |
 | [NEW] staff_actions.go   gated command interceptor -> fan-out op          | |
 | [NEW] staff_ownership.go chat_member watcher + periodic sweeper           | |
 | [NEW] lockdown.go        state machine, alerts, callbacks (ns "lockdown") | |
 | [NEW] lockdown_detect.go join-rate / flood-rate / ai-burst / ai-assist    | |
 | [CHANGED] captcha.go     "turnstile" mode + shared completion function  <-+-+
 | [NEW] settings_menu.go   page registry + callbacks (ns "cfg")             | |
 +--------------+---------------------------------+--------------------------+ |
                |                                 |                            |
   +------------v-----------+       +-------------v-------------+   +----------v-----------+
   | [NEW] utils/fanout     |       | [CHANGED] utils/actionlog |   | [NEW] utils/tgwebapp |
   | bounded, rate-limited  |       | + markup variant + Staff  |   | initData validator   |
   | per-chat executor      |       +---------------------------+   | [NEW] utils/turnstile|
   +------------+-----------+                                       | siteverify client    |
                |                                                   +----------------------+
   +------------v-------------------------------------------------------------------------+
   | [NEW] utils/chat_status: fresh (uncached) admin snapshot for a chat ID               |
   +------------+-------------------------------------------------------------------------+
                |
   +------------v-------------------------------------------------------------------------+
   | Repositories (alita/db/<domain>) + cache.GetFromCacheOrLoad / DeleteCache            |
   | [NEW] db/staff   [NEW] db/lockdown   [CHANGED] db/captcha (mode enum)                |
   +------------+---------------------------------------+---------------------------------+
                |                                       |
        PostgreSQL (migrations/*.sql)           Redis: cache (alita:cache:*) and operational
        [NEW] staff_groups, staff_group_links,  keys outside the cache prefix
        chat_lockdowns, lockdown_joiners,       [NEW] alita:raid:* counters (ephemeral only)
        lockdown_settings; captcha_mode enum
```

### Component Responsibilities

| Component | Responsibility | Why it lives there |
|-----------|----------------|--------------------|
| `db/staff` repository | Staff Group designation and group-to-staff links. Authoritative store for "who is linked to what, and whose ownership backs it". | One repo per domain, reads via `GetFromCacheOrLoad`, writes `DeleteCache` (AGENTS Data). |
| `utils/chat_status` fresh admin snapshot | One `getChatAdministrators` call returns owner, issuer rights and target-is-admin for a chat ID, uncached. | Existing predicates (`CanUserRestrict` etc.) read the admin cache and read the sender from the *current* update's context, which is the Staff Group. Both are wrong for per-group authorization. See Pattern 2. |
| `utils/fanout` | Run one operation per chat with a worker pool, a global rate limit, 429 `retry_after` handling, and a per-chat `Result`. Knows nothing about staff or Telegram semantics. | Reusable by Staff actions and by the lockdown "ban N joiners" button. Pure utils package, so no import cycle with `modules`. |
| `modules/staff*.go` | Command surface, authorization, target resolution, the per-group op closure, summary rendering, ownership watcher. | Handlers live in `modules`, business rules call repos and utils. |
| `modules/lockdown*.go` | Lockdown state machine, join and join-request enforcement, detectors, alert routing, `lockdown|` callbacks. | Replaces the *behaviour* of `antiraid.go`. Reuses its sliding-window Lua script. |
| `db/lockdown` | Persistent lockdown state (never Redis-only), permission snapshot, kicked-joiner list, per-chat detector settings. | "Never lifts on its own" and "Ban N recent joiners" hours later must survive a Redis flush or restart. |
| `utils/tgwebapp` | Pure functions: validate Mini App `initData` (HMAC, `auth_date` window), parse `user` and `start_param`. | Pure and table-testable. No HTTP, no DB. |
| `utils/turnstile` | `Verify(ctx, token, remoteIP, idemKey) (Result, error)` against a configurable base URL. | The base URL is injectable, so tests use `httptest` instead of mocks (AGENTS Testing). |
| `httpserver` (existing) | Gains an exported `Handle(pattern, http.Handler)` and a small hardened-route helper (body limit, security headers). Holds no captcha logic. | `httpserver` imports `db`, `config`, `cache`. It must not import `modules`. `main.go` is the composition root and wires `modules` handlers into it. |
| `modules/captcha.go` (changed) | Third mode `turnstile`. Success branch extracted into `completeCaptchaSuccess` so the callback path and the HTTP path share one implementation. | The completion path is already atomic (`CompleteCaptchaAttemptAtomic`). Do not fork it. |
| `modules/settings_menu.go` | Page registry, one callback namespace, permission re-check on every press, edit-in-place rendering. | Mirrors the existing registries (`RegisterDeepLinkHandler`, `RegisterAnonymousAdminHandler`, `SetModuleHelp`). |

---

## Recommended Project Structure

```
migrations/
  2026100500xxxx_add_staff_groups.sql          # staff_groups, staff_group_links
  2026100600xxxx_add_lockdown.sql              # chat_lockdowns, lockdown_joiners, lockdown_settings
  2026100700xxxx_captcha_mode_turnstile.sql    # DROP/ADD chk_captcha_mode (name verified in 20260412000000_*)
alita/
  config/                                      # + TurnstileSiteKey, TurnstileSecretKey, PublicBaseURL, MiniAppShortName
  db/
    models/staff.go, lockdown.go               # TableName() must match migration names exactly
    staff/repository.go (+ _test, testmain_test) # add models to the testmain AutoMigrate list
    lockdown/repository.go (+ tests)
    cache/local.go                             # add staff_* prefixes to skipLocal (security-relevant freshness)
  modules/
    staff.go  staff_actions.go  staff_ownership.go
    lockdown.go  lockdown_detect.go  lockdown_classify.go
    settings_menu.go  settings_pages_*.go
    captcha_turnstile.go                       # HTTP handler constructor + button builder
  utils/
    fanout/            chat_status/fresh.go    tgwebapp/   turnstile/
    httpserver/webapp/index.html, app.js       # go:embed, no JS build toolchain
    actionlog/         # extend, do not fork
```

### Structure Rationale

- **Fresh-admin helper in `chat_status`, executor in `utils/fanout`:** keeps the "permission predicates return bools and never reply" rule (AGENTS Permissions). The executor never replies. The module renders the summary.
- **`db/lockdown` separate from `db/antiraid`:** `antiraid_settings` has the wrong shape (ban duration, expiry). A new table avoids an `ALTER` on a live table and leaves the old commands working during migration.
- **Static page via `go:embed`, plain JS:** the repo has no Node toolchain and a Docker/goreleaser build. A single HTML file and about 60 lines of JS keeps `CGO_ENABLED=0 go build ./...` the only build step.
- **Mini App handler built in `modules`, mounted in `main.go`:** avoids an `httpserver` to `modules` import cycle (see Component Responsibilities).

---

## Architectural Patterns

### Pattern 1: Gated command interceptor (Staff commands shadow per-group commands)

**What:** In the Staff Group, `/ban`, `/tban`, `/mute`, `/tmute`, `/kick`, `/unban`, `/unmute` must mean "act on all linked groups". The same names are already registered at group 0 by Bans (priority 70) and Mutes (80). gotgbot runs only the first matching handler in a group, and a handler returning `ContinueGroups` jumps to the *next group*, not the next handler in the same group. So a staff handler that "declines" by returning `ContinueGroups` would skip the real `/ban` in every non-staff chat.

**Solution:** The decision must be made in `CheckUpdate`, not in the handler. Register the staff handlers at group 0 *before* Bans/Mutes load (module priority below 70, for example 65) with a gate predicate. For a non-staff chat the gate returns false, so dispatch moves on to the real handler.

- Verified in the vendored source: `handlers.Command` is a plain struct exposing `CheckUpdate`/`HandleUpdate`/`Name`. A tiny wrapper that embeds it and ANDs a predicate into `CheckUpdate` is enough.
- To honour the AGENTS rule "new commands use `WrapCommand`", add an optional `Gate func(*ext.Context) bool` to `helpers.CommandDescriptor`. Zero value means today's behaviour, and `register()` wraps `handlers.NewCommand` when `Gate != nil`. This is additive and keeps `RecoverFromPanic`, `BuildCommandContext` and `RequiredChecks`.

**When to use:** any feature that overloads an existing command name based on chat identity.
**Trade-offs:** one extra cached lookup per staff-named command, in every chat (cheap: only evaluated after the command-name match, so the cost is zero for other messages). The priority number becomes load-bearing; add a comment and a registry test asserting that Staff's priority is below Bans and Mutes.

```go
// helpers/command_pipeline.go (additive)
type CommandDescriptor struct {
    Name, Aliases, Group, RequiredChecks, Disableable // existing
    Gate func(*ext.Context) bool // optional; nil = always eligible
}

type gatedCommand struct {
    handlers.Command
    gate func(*ext.Context) bool
}
func (g gatedCommand) CheckUpdate(b *gotgbot.Bot, ctx *ext.Context) bool {
    return g.Command.CheckUpdate(b, ctx) && g.gate(ctx) // command match first = cheap reject
}
```

Staff gate: `staff.IsStaffGroup(ctx.EffectiveChat.Id)` via `GetFromCacheOrLoad`. Add `RequireStaffMember()` and `RejectAnonymousSender()` `CheckFunc`s. Anonymous admins bypass `WrapCommand`; in the Staff Group they cannot be authorized per group anyway, so do **not** register anonymous handlers for staff actions. Reply once: "post as yourself" (PROJECT.md Context).

### Pattern 2: Per-group authorization from one fresh admin snapshot

**What:** The core safety rule is "the bot never lets anyone act in a group where they aren't an admin". Do the check per target group from live data, not from the admin cache:

```
op(chatID):
  admins := chat_status.FreshAdmins(bot, chatID)      // one uncached getChatAdministrators
  link still valid?  admins.CreatorID == link.OwnerUserID   // else unlink + Skipped(link_broken)
  issuer ok?         creator OR (admin AND can_restrict_members)  // else Skipped(not_admin / missing_right)
  target protected?  target in admins                          // else Skipped(target_admin)
  act:               ban / unban / kick / restrict / unrestrict  // Telegram enforces the bot's own rights
  map errors:        bot lacks rights -> Failed("bot lacks rights"); not a member -> Skipped
```

**Why not reuse `chat_status.CanUserRestrict` and friends:** they go through `hasUserPermission`, which (a) calls `checkAnonAdmin` using the *current update's* sender, i.e. the Staff Group message, and (b) reads `cache.GetAdminCacheUser`. A cached view lets a just-demoted admin keep acting for up to the cache TTL. The new helper is a separate function with an explicit `chatID`, not a variant of the predicates.

**Why not pre-check the bot's rights:** `getChatAdministrators` excludes bots, so the bot's own rights need a second call. Attempt the action and map Telegram's error to `Failed(bot lacks rights)`. This is one read plus one write per group instead of three calls, and the spec already lists "bot lacks rights" as a *failed* outcome.

**Trade-offs:** about 2 API calls per group per action (plus an optional log-channel send). At 30 groups that is about 60 to 90 calls, which is a few seconds under the rate limiter below.

### Pattern 3: Bounded fan-out executor with partial-success results

**What:** `utils/fanout.Run(ctx, targets, op, opts) []Result`.

- Worker pool of about 5, plus a shared token bucket (add `golang.org/x/time/rate`, not currently in `go.mod`; or a 30-line pacer) at about 20 req/s, safely under the Bot API's ~30/s broadcast ceiling (MEDIUM, widely documented, primary doc not fetchable).
- On `*gotgbot.TelegramError` with `Code == 429`, read `ResponseParams.RetryAfter` (verified present in `gen_types.go`), sleep, retry that chat up to N times, then report `Failed(rate limited)`. Never drop silently.
- Result is `{ChatID, Title, Outcome (Done|Skipped|Failed), ReasonKey (i18n), Err}`. Reason keys are i18n keys, so the renderer localizes. Never embed English in results.
- Every goroutine starts with `defer error_handling.RecoverFromPanic(...)` (AGENTS Go rules). The command handler waits for the pool, so no shutdown WaitGroup is needed unless the work is made detached; if it is, join a drain registered *after* DB-close (AGENTS Startup: LIFO).

**Reply flow:** send one "Applying to N groups..." message immediately, run the fan-out, then **edit once** with the final summary. Do not edit per group (edit rate limits and noise). Cap the summary at about 3500 characters: list non-Done lines first, then collapse the Done groups into a count.

**Contrast with `applyActiveFban` (`federations.go:756`):** it is `go`-detached, ignores errors (`log.Debugf`), has no concurrency limit and no result. It is a model for *shape* only, as PROJECT.md says. Do not copy it.

### Pattern 4: Persisted state machine with atomic transitions (lockdown)

**What:** Postgres is the source of truth; Redis holds only ephemeral counters.

```
              trigger (rule | AI-assisted | manual)
 Idle ────────────────────────────────────────────▶ Active(applied=false)
                                                        │ snapshot taken BEFORE insert,
                                                        │ stored in the row
                                                        ▼
                                                  Active(applied=true)  ◀── join: kick + record
                                                        │
              admin presses Lift / /unlockdown          │
 Idle ◀──────────────────────────────────────── Lifting ┘  (row stays Active until restore succeeds)
```

- `chat_lockdowns(id, chat_id, state, trigger, triggered_by, snapshot_perms (text/JSON), applied, started_at, lifted_at, lifted_by)`.
- **One active lockdown per chat is enforced by a partial unique index** `UNIQUE (chat_id) WHERE state='active'` (supported by both PostgreSQL and SQLite). Triggering is `INSERT ... ON CONFLICT DO NOTHING`; zero rows affected means somebody else already started it, so do nothing. This makes triggers idempotent across replicas and across a join-surge plus flood firing in the same second.
- **Take the snapshot (`getChat` to `Permissions`) before the INSERT and write it in the same row.** Never re-read permissions after applying, or the locked state gets stored as the "original" (see Anti-Pattern 4).
- Lift is `UPDATE ... SET state='lifted' WHERE id=? AND state='active'` **after** `setChatPermissions(snapshot)` succeeds. Double-presses and two-alert races resolve on rows-affected. A failed restore leaves the row active and tells the admin to retry.
- **Startup recovery** for rows with `applied=false`: re-apply. This mirrors the existing `runOrphanedCaptchaRecovery` (`captcha.go:244`). Register it in `postInit`, not in an `init()` (AGENTS Startup).

### Pattern 5: Namespaced callbacks that carry an ID, never user text

**What:** All new buttons go through `callbackcodec` (via the package-local `encodeCallbackData`/`decodeCallbackData`). Payloads reference a **DB row ID**, not chat IDs or text:

| Button | Data (`ns|v1|payload`) | Bytes |
|--------|------------------------|-------|
| Lift lockdown | `lockdown|v1|a=lift&l=123` | about 25 |
| Ban N recent joiners | `lockdown|v1|a=banj&l=123` | about 26 |
| Staff panel actions | `staff|v1|a=unlink&g=-1001234567890` | about 36 |
| Settings menu | `cfg|v1|p=cap&a=t&v=1` (chat comes from the message or the PM connection) | about 22 |

- The handler loads the lockdown row, takes `chat_id` **from the row**, and authorizes `query.From.Id` against that chat. This is what makes "only an admin of the locked group can lift it, wherever they press the button" correct: the alert copies live in the Staff Group and a log *channel*, where `ctx.EffectiveChat` is the wrong chat.
- **Namespace collision trap (verified):** existing registrations use `callbackquery.Prefix("antiraid")`, `"report"`, `"backup"`, `"about"`, `"restrict"`, `"formatting"`, `"helpq"` and so on, with no trailing `|`. A new namespace that *starts with* one of those (`antiraid_lockdown`, `report_x`) is swallowed by the older handler, which returns `ContinueGroups`. That skips the rest of group 0, so the new handler never runs. Use `lockdown`, `staff`, `cfg`, register each with a trailing `|` (as `unbanall|` and the federations namespace do), and add a registry test that no registered prefix is a prefix of another.
- A dead button is silent: `encodeCallbackData` returns `""` on overflow (AGENTS Callbacks). Add a unit test that encodes the longest realistic payload for each namespace.

### Pattern 6: Settings-page registry (menu is a router, not a rewrite)

**What:**

```go
type MenuPage struct {
    ID       string                         // short code used in cfg|p=
    Order    int
    Perm     PagePerm                       // admin | admin+right | owner
    Render   func(m *MenuCtx) (text string, kb [][]gotgbot.InlineKeyboardButton)
    Handle   func(m *MenuCtx, action, value string) error // mutate via existing repo setters, then re-render
}
func RegisterMenuPage(p MenuPage) // called from each feature module's init(), like RegisterDeepLinkHandler
```

- Each press **re-authorizes** `query.From` against the target chat (callbacks can be pressed by anyone who can see the message; `antiraid.callbackHandler` already does this).
- Pages call the existing repository setters (`captcha.SetCaptchaEnabled`, `antiraid`/`lockdown` settings setters, etc.). Those already `DeleteCache`; the menu adds no new write paths.
- **Constraint on phases 1 to 3:** keep business logic in repositories and put status rendering in pure functions `(chat, user, tr) -> (text, keyboard)`. The menu then only wires them in. `/staff` is built that way in phase 1, so folding it into a "Staff Group" page is a registration, not a rewrite.
- `/settings` goes through `WrapCommand` with `RequireGroup`, `RequireUserAdmin`, `Disableable: true`, and an `anonPipelineHandler` registration. The PM variant resolves the chat via the existing connection (`IsUserConnected`) instead of encoding chat IDs.

---

## Data Flow

### Flow A: Staff Group moderation command

```
Staff member: "/ban @someone spamming" in Staff Group S
  -1  users tracker records the sender and username (feeds lookups)
   0  gated staff handler (priority 65) CheckUpdate: command matches AND IsStaffGroup(S) [cached]
      WrapCommand -> RequiredChecks: RejectAnonymousSender, RequireStaffMember
      resolve target: extraction.ExtractUserAndText -> GetUserId(@name) -> users table
                      (-> channels -> getChat fallback), or a numeric ID. Reply-to also works for free.
      refuse: target == issuer / bot / a Staff Group admin
      links := staff.ListLinksFresh(S)             // DB read, deliberately NOT cached (authority)
      reply "Applying to N groups..."
      fanout.Run(links, op)                        // Pattern 3, op per Pattern 2
          op -> FreshAdmins(G) -> checks -> BanChatMember(G, target, until?) 
             -> on Done: actionlog.Admin(bot, G, issuer, "STAFF_BAN via <S>", target, reason)
      edit reply with per-group summary (+ reason)
  return ext.EndGroups
```

- Timed variants: parse the duration **once** in the staff handler (`extraction.TemporaryUntilDate` / the time parser), pass `untilDate` into every op. Do not call `extraction.ExtractTime` per group; it replies into the current chat.
- `kick` is ban-then-unban per group; reuse `kickMember` semantics (`UnbanChatMember(OnlyIfBanned:false)`).
- `ban` on a non-member is still performed (pre-emptive ban across all groups, which matches "act on a bad actor everywhere"). `mute`/`kick` on a non-member become `Skipped(not a member)`.

### Flow B: Link, unlink, and auto-unlink

```
/setstaff  (in S, owner only: helpers.RequireUserOwner + anon handler, owners are often anonymous)
   -> staff_groups upsert(chat_id=S, owner_user_id=U); chat must be group/supergroup, not channel
/linkstaff (in G, issued by U)
   -> RequireUserOwner in G; find staff groups where owner_user_id=U; verify U is creator of S now
   -> transaction: reject if G is itself a staff group or already linked (unique index on group_chat_id)
   -> insert staff_group_links(group_chat_id=G, staff_chat_id=S, owner_user_id=U)
   -> DeleteCache(staff_link_of:G, staff_links:S, staff_group:S) after commit
Ownership change detection (three layers, because chat_member updates can be missed):
   (1) group -3 watcher: chat_member where old OR new status == "creator" in a staff/linked chat
       -> re-verify via getChatMember -> unlink + notify S
   (2) fan-out time: Pattern 2 compares the live creator with link.owner_user_id for every group
   (3) hourly sweeper goroutine (recover + lifecycle context + stop handler): getChatMember for each link
```

The ownership watcher must **not** be group 0: `bot_updates.go` registers a group-0 `NewChatMember(ExtractAdminUpdateStatusChange, adminCacheAutoUpdate)` that matches creator-to-admin transitions and returns `ContinueGroups`, which would shadow any later group-0 `chat_member` handler. Use group `-3` (free) and always return `ContinueGroups`.

### Flow C: Raid detection to lockdown to lift

```
JOIN SIGNALS (service message AND ChatMemberUpdated both arrive; neither is guaranteed)
  group -5 lockdown guard handles both signals
     ZADD alita:raid:joins:<chat>  (member = userID, score = ts)   // member = user => both signals dedupe
     ZADD alita:raid:newmembers:<chat> (userID, joinTs)             // for "new member" weighting
     if lockdown active: kick joiner (+ record in lockdown_joiners) and return EndGroups
                         (so greetings/captcha at group 0 never welcome or challenge a kicked user)
     else: join-rate rule (reuse trackJoinScript semantics) -> Trigger?
  chat_join_request: decline while active (greetings.pendingJoins would otherwise approve/queue)

MESSAGE SIGNALS
  group 2 raid flood detector (after users tracker, before aispam/antiflood so raw volume is counted)
     skip admins/approved (IsUserAdminForUpdate / IsApprovedForUpdate, memoised)
     Lua: record msg, return {msgs, distinctSenders, newMemberShare, mediaShare} over window T
     rule: msgs >= M AND distinct >= K AND newMemberShare >= S  -> Trigger
           between soft and hard thresholds                      -> Borderline

AI SIGNALS (assist only)
  aispam worker: after a "delete" verdict -> raid.ObserveSpamVerdict(chat, user, isNew)
        burst rule: >= V deletions from >= K distinct users inside T -> Trigger
  Borderline -> rate-limited async classify of a sample (own small queue, per-chat cooldown,
        honours aispamBreakerPaused). Result may ESCALATE Borderline -> Trigger. It never
        de-escalates a rule Trigger and never fires alone.

TRIGGER
  lockdown.Engage(chat, trigger):
     getChat -> snapshot; INSERT chat_lockdowns ON CONFLICT DO NOTHING (loser stops)
     setChatPermissions(all false)  (needs can_restrict_members; failure => applied=false, alert says so)
     approved-user exceptions (see Spike 1)
     alert -> [Staff Group of this chat via staff_group_links] + [chat's log channel] + fallback: the chat itself
              buttons: Lift lockdown | Ban N recent joiners     (ns "lockdown|", ID-only payload)
     store alert (chat_id, message_id) pairs so Lift can edit every copy

LIFT
  press -> decode -> load row -> IsUserAdmin(query.From, row.ChatID) [fresh]
        -> setChatPermissions(snapshot) -> UPDATE state='lifted' WHERE state='active'
        -> edit all alert copies ("Lifted by X"), answer callback
```

### Flow D: Turnstile Mini App captcha

```
Join (group 0 greetings -> ProcessSingleJoin -> SendCaptcha)         [existing path]
  SendCaptcha, mode == "turnstile":
     CreateCaptchaAttemptPreMessageIfEnabled(user, chat, answer=<128-bit random nonce>, timeout)  // reuse; answer column holds the nonce
     RestrictMember(MutedPermissions)                                   // existing
     send message with ONE url button:  https://t.me/<bot>/<MiniAppShortName>?startapp=t_<attemptID>_<nonce>
     scheduleCaptchaTimeout(...)                                        // existing: kick on expiry, can rejoin

User taps button (opens Mini App page from the group via Direct Link)
  GET  /captcha/app   -> embedded HTML (sitekey templated), strict CSP, loads telegram-web-app.js + Turnstile
  JS   Turnstile solved -> POST /captcha/verify {init_data: WebApp.initData, token}

POST /captcha/verify  (modules.NewTurnstileHandler(bot) mounted by main.go)
  1 body limit (16 KB), per-IP and per-attempt rate limit, JSON only
  2 tgwebapp.Validate(initData, botToken, maxAge)       // HMAC "WebAppData"; reject stale auth_date
  3 attempt := GetCaptchaAttemptByID(id from start_param)
        require attempt.UserID == initData.user.id      // the binding: Telegram-signed identity == pending user
        require constant-time attempt.Answer == nonce   // the binding: this exact attempt
        require not expired, mode == turnstile
  4 turnstile.Verify(token, remoteIP, idemKey)          // once per token; success, hostname == PublicBaseURL host,
                                                        // action == "tg_captcha"; token life 300 s, single use
  5 completeCaptchaSuccess(...)                         // CompleteCaptchaAttemptAtomic -> restore perms ->
                                                        // stored-message summary -> welcome (shared with callback path)
  6 respond JSON {ok:true}; page calls WebApp.close()
```

Notes:
- `InlineKeyboardButton.web_app` works only in **private** chats, so it cannot be used for the group captcha message. A URL button with a Direct Link Mini App is the group-compatible launch method, and `initData` then carries `start_param` plus the signed `user` (MEDIUM; see Spike 3). It needs a Mini App created in BotFather (`/newapp`) whose short name goes into config.
- The HTTP handler has no `ext.Context`. `handleCaptchaTimeout` and `membershipDepsForJoin` already synthesize `ext.Context{EffectiveChat: ...}` for the same reason, so there is precedent. The extracted completion function takes explicit `(bot, chatID, userID, lang)` rather than a context.

### Flow E: Settings menu

```
/settings (group, admin) or PM (connected)  ->  root page lists pages by Order
button  cfg|v1|p=<page>&a=<open|t|set|inc|dec>&v=<val>
   -> decode (callbackcodec) -> resolve chat (message chat, or connection) -> re-authorize query.From
   -> page.Handle (existing repo setter, which DeleteCaches) -> page.Render -> msg.EditText
```

---

## Handler Group Plan (additions to the AGENTS.md table)

| Group | Existing | Change |
|-------|----------|--------|
| `-5` | antiraid | Lockdown guard (join, join-request) takes over; antiraid's ban/expiry enforcement is retired in the same phase so two handlers never share `-5` |
| `-3` | free | **Staff-link ownership watcher** (`chat_member`), always `ContinueGroups` |
| `0` | commands/help/greetings | Staff interceptor registers first via module priority 65 (< Bans 70), gated. `/lockdown`, `/unlockdown`, `/staff`, `/setstaff`, `/linkstaff`, `/unlinkstaff`, `/settings`, `cfg|`, `lockdown|`, `staff|` callbacks |
| `2` | free | **Raid flood detector** (after users tracker `-1`, before aispam `3` and antiflood `4`), always `ContinueGroups` |
| `3` | aispam | + one hook call after a delete verdict |

Update `AGENTS.md` in the same commit that allocates `-3` and `2`, and add `alita:raid:*` to its list of operational Redis keys that sit outside `alita:cache:`.

---

## Data Model (migrations; names must match `TableName()`)

Use timestamps greater than `20261001120000` (the current latest). One transaction per file. No `CREATE INDEX CONCURRENTLY`. Update both `alita/db/migrations/runner.go` and `scripts/migrate_psql.sh` only if the applier changes, which it should not. Add every new model to the relevant `testmain_test.go` AutoMigrate list.

| Table | Key columns | Constraints / indexes | Cache keys to `DeleteCache` on write |
|-------|-------------|-----------------------|--------------------------------------|
| `staff_groups` | `chat_id`, `owner_user_id`, timestamps | `UNIQUE(chat_id)` | `staff_group:<chat>` |
| `staff_group_links` | `group_chat_id`, `staff_chat_id`, `owner_user_id`, `created_at` | `UNIQUE(group_chat_id)` (one Staff Group per group, enforced by the DB), `INDEX(staff_chat_id)`, `CHECK(group_chat_id <> staff_chat_id)` | `staff_link_of:<group>`, `staff_links:<staff>` |
| `chat_lockdowns` | `chat_id`, `state`, `trigger`, `triggered_by`, `snapshot_perms`, `applied`, `started_at`, `lifted_at`, `lifted_by`, `alerts` (JSON text of message refs) | partial `UNIQUE(chat_id) WHERE state='active'`, `CHECK(state IN ('active','lifted'))` | `lockdown_active:<chat>` |
| `lockdown_joiners` | `lockdown_id`, `user_id`, `first_name`, `joined_at`, `banned_at NULL` | `UNIQUE(lockdown_id,user_id)`; cap rows per lockdown in code (about 500) | none (read uncached) |
| `lockdown_settings` | `chat_id`, `enabled`, join threshold and window, flood M/K/S/T, `ai_assist`, `ai_burst` | `UNIQUE(chat_id)`; defaults **off** (a false-positive lockdown is disruptive) | `lockdown_settings:<chat>` |
| `captcha_settings` | extend `chk_captcha_mode` to include `'turnstile'` | `DROP CONSTRAINT IF EXISTS` then `ADD` (same name as in `20260412000000_*`); update the model's `check:` tag so the SQLite test schema agrees | existing captcha-settings key |

Rules that come straight from AGENTS and bite here:
- **Authority reads are uncached.** The executor and the lockdown callbacks read links and lockdown rows straight from the DB. The cached copies serve only the cheap gate (`IsStaffGroup`) and UI. Also add `staff_`/`lockdown_active` prefixes to `skipLocal` (`alita/db/cache/local.go`) so a second replica cannot serve a stale link for `CACHE_LOCAL_TTL`.
- Writes that set booleans to `false` (`enabled`, `ai_assist`) must use `UpdateRecordWithZeroValues`; both update helpers return `gorm.ErrRecordNotFound` on no match.
- Never discard a DB error on these state-changing paths (Engage, Link, Unlink, Lift).

---

## Integration Seams With Existing Subsystems

| Existing | Relationship | Concrete seam / risk |
|----------|--------------|----------------------|
| **antiraid** (`-5`, Redis-only, `antiraid.go`) | Superseded in behaviour. Keep `/antiraid`, `/autoantiraid` as aliases (`on` = manual lockdown, `off` = lift, `autoantiraid N` = join threshold). Reuse `trackJoinScript` (sliding ZSET). Delete: ban action, expiry poller, `StopAntiRaidExpiryPoller` shutdown handler, `raidtime`/`raidactiontime`. | Its `onJoin` only sees the **service-message** signal. Join service messages can be absent in large groups, so the guard must also take `ChatMemberUpdated`. ZSET member in the old script is `user:ts:nano` (unique), which would double-count if both signals feed it. Use `member = userID`. |
| **antiflood** (`4`, in-process per replica) | Unchanged. It is per-user. The raid flood detector is cross-user and Redis-backed because it must be cluster-wide. | Do not extend antiflood's `syncHelperMap`. Different keying and different scope. |
| **aispam** (`3`, queue of 256, 4 workers, breaker) | Producer of one signal (post-delete hook) and host of the assist classifier. | Reuse `aispamBreakerPaused`. Give the raid classifier its own queue and per-chat cooldown so a raid cannot starve per-message checks. The Jev state struct is **text-only** (`aispamState`), so "classify borderline images" has no existing path (Spike 4). |
| **captcha** (`-10` sweeper, group 0 challenge) | Lockdown kicks joiners at `-5` *before* group 0, so no captcha starts for them. Users already pending keep their attempt. | **Trap:** `unmuteCaptchaUser` (and `/unmute`, `/unrestrict`) compute permissions from `resolveUnmutePermissions(getChat)`. During a lockdown `getChat().Permissions` is all-false, so a captcha pass or timeout would write the *locked* defaults as that user's own restriction and they stay muted after the lift. Make `resolveUnmutePermissions` (single choke point, `chat_permissions.go`) return the active lockdown's snapshot when one exists. |
| **greetings** (group 0) | `leftMember` posts goodbye messages and `cleanService` deletes join messages. | Lockdown kicks fire `chat_member` left updates, so a raid produces N goodbye messages. Skip goodbye when the update's actor (`ChatMember.From.Id`) is the bot, or when a short Redis flag `alita:raid:kicked:<chat>:<user>` is set. Do **not** call `claimRecentJoinProcessing` from the lockdown guard. Its dedupe key is shared with greetings and claiming it first would suppress welcome/captcha for legitimate joiners. |
| **approvals** | Exempt from lockdown mute and from the flood detector. | Chat-level mute does not exempt them automatically. See Spike 1. |
| **log channels** (`actionlog`) | Staff actions use `actionlog.Admin` per affected group (category `admin`, per-category toggles honoured). Lockdown alerts need buttons. | `actionlog.Log` takes no `ReplyMarkup`. Add `LogWithMarkup` (returns the sent message ref) and use it for alerts. The helper needs a `*gotgbot.Chat` with `Title` for its header, so source titles from the `chats` table. |
| **federations** | Untouched. `/fban` keeps its own path and `enforceFedBan` stays at `-6`. | Staff fan-out does not write fed bans and does not read them. |
| **anonymous-admin router** | Needed for owner-only and admin-only commands (`/setstaff`, `/linkstaff`, `/lockdown`, `/settings`). Not wanted for staff moderation. | The owner is frequently anonymous. Register `RegisterAnonymousAdminHandler` + `anonPipelineHandler` for those, none for staff actions. |
| **users tracker** (`-1`) | Source of truth for `@username` to ID. | A recycled username can map to the wrong user, and the action hits every group. Echo the resolved ID and name in the summary so a mistake is visible and one `/unban` away. A confirm step is a product decision (Open Questions). |
| **HTTP server** | Routes added on the same mux and port. | `ReadTimeout`/`WriteTimeout` are 10 s (fine). `/metrics` and pprof stay behind their existing auth. The new routes are public, so give them their own body limit and rate limiting. In polling mode `httpServer.Start()` runs *before* `postInit` (`main.go:261` vs `285`), so mount the routes before `Start()` in both modes. |

---

## Anti-Patterns

### Anti-Pattern 1: Declining from a group-0 handler with `ContinueGroups`

**What people do:** register `/ban` for staff and `return ext.ContinueGroups` when the chat is not a Staff Group.
**Why it's wrong:** it skips the real `/ban` and every later group-0 handler for that update. This is the failure mode AGENTS warns about.
**Do this instead:** Pattern 1. Decide in `CheckUpdate` so non-staff chats fall through to the next handler untouched.

### Anti-Pattern 2: Reusing `chat_status.CanUser*` for cross-chat authorization

**What people do:** call `CanUserRestrict(b, ctx, chat, issuer)` once per linked group.
**Why it's wrong:** it consults the admin cache (stale after demotion) and `checkAnonAdmin` looks at the current update's sender. The safeguard the whole feature rests on would be cache-dependent.
**Do this instead:** Pattern 2, an explicit uncached `FreshAdmins(chatID)`.

### Anti-Pattern 3: Redis-only lockdown state

**What people do:** copy `antiraid`'s `raidState` into Redis with a TTL.
**Why it's wrong:** "a lockdown never lifts on its own" is violated by a Redis flush, `CLEAR_CACHE_ON_STARTUP` mistakes, or an eviction. The `antiraid` precedent also silently does nothing without Redis.
**Do this instead:** Postgres state (Pattern 4). Redis for counters only. Manual `/lockdown` works with no Redis. Auto-triggers log a warning and are disabled without it.

### Anti-Pattern 4: Snapshotting permissions after applying the lock

**What people do:** `setChatPermissions(locked)` then read `getChat()` "to remember what to restore".
**Why it's wrong:** it stores the locked state as the original. The group stays muted after lift.
**Do this instead:** read before the INSERT, persist in the lockdown row, and only ever restore from that row.

### Anti-Pattern 5: Trusting `initData` without a freshness check, or the Turnstile token without the hostname/action

**What people do:** verify the HMAC only, and accept any `success:true`.
**Why it's wrong:** a captured `initData` verifies forever; a token minted on another site that shares the sitekey could be replayed.
**Do this instead:** reject old `auth_date`, bind `user.id` to the attempt's user and the nonce to the attempt, check `hostname` and `action` in the siteverify response, call siteverify exactly once per token.

### Anti-Pattern 6: Putting the Turnstile logic in `httpserver`

**What people do:** import `modules` or `db/captcha` from `httpserver` to finish the verification.
**Why it's wrong:** import cycle risk and a second copy of the completion path.
**Do this instead:** `httpserver` exposes `Handle`; `modules` builds the handler; `main.go` mounts it.

### Anti-Pattern 7: Menu callbacks that carry chat IDs plus user text

**What people do:** `cfg|...&c=<chat>&t=<free text>` for setters that take values.
**Why it's wrong:** 64-byte cap, a dead button on overflow, and anyone can read and replay the payload.
**Do this instead:** enumerated actions and small values in the payload. Free-text settings use a Redis token behind a short key (AGENTS Callbacks) and a "reply to this message" prompt.

---

## Scalability Considerations

The project is single-owner and small by design. The relevant "scale" axes are linked groups and raid size, not user count.

| Concern | 5 linked groups | 50 linked groups | 500 linked groups (not a goal) |
|---------|-----------------|------------------|--------------------------------|
| Staff action latency | under 1 s | about 5 to 8 s at 20 req/s with 2 calls per group | minutes; would need background mode and progress edits |
| Summary message size | trivial | collapse Done groups into a count | paginate or send as a document |
| Raid with 500 joins/min | counters are O(1) Redis ops | same | kicking is bound by the rate limiter. Batch the kicks through `fanout`, and expect the kick queue, not the detector, to lag |
| Lockdown alert fan-out | 2 to 3 sends | same | same |
| Lockdown approved-user exceptions | one `restrictChatMember` per approved member | same | the cost grows with the approved list (Spike 1) |

**First bottleneck:** Bot API request rate during a large raid (kick plus delete plus alert). The shared token bucket in `fanout` should also front the lockdown kicker, so both features draw from one budget.
**Second bottleneck:** the approved-user exception loop on lockdown start, if a chat has hundreds of approved users.

---

## Suggested Build Order (dependencies drive phases)

```
Phase 1  Staff Group ────────────────────────────┐
  1a schema + models + db/staff + cache/skipLocal │  (nothing else depends on 1a except 2's alert routing)
  1b chat_status.FreshAdmins, utils/fanout, actionlog.Staff, CommandDescriptor.Gate
  1c /setstaff /linkstaff /unlinkstaff + ownership checks + group -3 watcher + sweeper
  1d gated interceptor, target resolution, op closure, summary, log-channel posts
  1e /staff panel as a pure render function (+ staff| callbacks)
                                                  │
Phase 2  Raid protection  (needs: 1a for alert routing, 1b fanout for kicks, actionlog markup)
  2-spike  approved-user exceptions; chat_member join delivery; EndGroups at -5
  2a lockdown schema + repo + Engage/Lift + resolveUnmutePermissions seam + manual /lockdown /unlockdown
  2b join guard (-5): kick, record joiners, decline join requests, goodbye suppression, alerts + callbacks
  2c detectors: join-rate (reuse Lua), flood (group 2), ai-burst hook
  2d AI-assist classifier (text first), then retire antiraid enforcement and re-point its commands
                                                  │
Phase 3  Turnstile captcha  (independent of 1 and 2 except the 2a unmute seam)
  3-spike  Direct Link launch from a group, Turnstile inside Telegram WebViews (Android/iOS/Desktop)
  3a config + RegisterSecret + migration (mode enum)
  3b utils/tgwebapp + utils/turnstile (pure, fully table/httptest tested)
  3c extract completeCaptchaSuccess; turnstile mode in SendCaptcha; button
  3d HTTP routes + embedded page + hardening; mount in main.go
                                                  │
Phase 4  Settings menu  (needs the pages to exist: captcha, lockdown settings, antiflood, staff panel)
  4a page registry + /settings + callbacks   4b pages migrate/register   4c Staff page = panel render
```

**Ordering rationale:**
- Phase 1 first (owner priority, and it is the only phase that creates reusable infrastructure: `fanout`, fresh admin snapshot, gated commands, link table).
- Phase 2 depends on Phase 1 only for alert routing (`staff_group_links`), so a Phase 2 started without it would still work by falling back to log channel and the group itself. The ordering keeps the owner's order and avoids stubs.
- Phase 3 touches `captcha.go` heavily. Doing it after Phase 2 means the `resolveUnmutePermissions` lockdown seam already exists, so the new completion path cannot reintroduce the muted-after-lift bug.
- Phase 4 last, but **the constraint flows backwards**: phases 1 to 3 must keep logic in repos and renderers so the menu is a thin layer.
- Cross-cutting each phase: 7-locale keys, `make generate-docs`, `make check-translations`, `make test`, `make lint`, and an `AGENTS.md` update when a group number, Redis key family, or rule changes.

**Research flags for the roadmap:**
- Phase 2: needs a short spike before planning (Spikes 1, 2, 4).
- Phase 3: needs a short spike (Spikes 3, 5).
- Phase 1 and Phase 4: standard patterns on top of existing code. No external research needed.

### Testing shape (per AGENTS: real fixtures, no mocking libraries)
- Fan-out and staff actions: hand-written `gotgbot.BotClient` fake that records calls and can return a scripted `TelegramError` (including 429 with `retry_after`), SQLite via `internal/testdb.Run`. Assert observable results: summary text, rows, cache invalidation, who was *not* banned.
- Lockdown: miniredis for counters; assert the partial-unique-index race (two concurrent `Engage`), restore-failure leaves `active`, and snapshot never overwritten.
- Mini App: `httptest.Server` for both the bot's route and a fake Turnstile endpoint (injectable base URL), `initData` built in the test with the real HMAC recipe, replay/stale/wrong-user/wrong-nonce cases.
- Callback codec: encode the max-length payload per namespace; registry test for prefix collisions.

---

## Integration Points

### External Services

| Service | Integration Pattern | Notes |
|---------|---------------------|-------|
| Telegram Bot API | Existing `*gotgbot.Bot`. New calls: `GetChatAdministrators`, `SetChatPermissions`, `RestrictChatMember` (with `UseIndependentChatPermissions`), `DeclineChatJoinRequest`, `DeleteMessages`. | All exist in the vendored gotgbot rc.36 (verified). 429 surfaces as `*TelegramError{Code:429, ResponseParams.RetryAfter}`. |
| Cloudflare Turnstile | `POST https://challenges.cloudflare.com/turnstile/v0/siteverify` with `secret`, `response`, optional `remoteip`, `idempotency_key`. | Token valid 300 s, single use (`timeout-or-duplicate` on replay), validate `hostname` and `action`. MEDIUM. Secret registered with `logredact.RegisterSecret` (>= 6 chars) in `config.go`'s existing call. |
| Telegram Mini Apps | `initData` HMAC: `secret = HMAC_SHA256(key="WebAppData", msg=bot_token)`, `hash = HMAC_SHA256(secret, data_check_string)`, data-check-string = all fields except `hash`/`signature`, sorted, `k=v` joined by `\n`. | MEDIUM (secondary sources agree). Keep the Ed25519 `signature` path out of scope: the bot holds its own token. |
| TypeSafe (Jev) | Existing client (`aispam_jev.go`). | Text-only today. |

### Internal Boundaries

| Boundary | Communication | Notes |
|----------|---------------|-------|
| `modules/staff` to `utils/fanout` | Direct call with an op closure | The closure owns Telegram semantics; the executor owns concurrency and rate. |
| `modules/lockdown` to `modules/aispam` | Direct function call (same package) | One-way: aispam notifies, lockdown never calls back into the worker pool except via the classify queue. |
| `modules/lockdown` to `modules/captcha` | `resolveUnmutePermissions` reads the active snapshot | Single choke point; keep it the only coupling. |
| `modules/captcha_turnstile` to `httpserver` | `main.go` calls `httpServer.Handle("/captcha/", modules.NewTurnstileHandler(bot))` | `httpserver` stays free of module imports. |
| All feature modules to `settings_menu` | `RegisterMenuPage` in `init()` | Same registration style as deep links and anon-admin handlers. |
| Shutdown | New background goroutines (ownership sweeper, lockdown recovery, classify worker) get a lifecycle context and a stop function registered **after** the DB-close handler so they drain before it (LIFO, 60 s). | Follow `StartCaptchaLifecycle`/`StopCaptchaLifecycle`. Start them from `postInit`, never `init()`. |

---

## Open Questions / Spikes (do before planning the phase that depends on them)

1. **Per-user exceptions over restrictive default permissions (Phase 2). LOW.** The approved-user exemption assumes that `restrictChatMember(user, snapshotPerms, UseIndependentChatPermissions=true)` lets that user send while the group default is all-false. Telegram's client "exceptions" feature suggests yes, but the Bot API docs that were reachable did not confirm it. Test once in a throwaway supergroup. If it fails, the fallback is: approved users are muted during lockdown (admins are never affected), documented in the alert text.
2. **Join delivery and `EndGroups` at `-5` (Phase 2). MEDIUM.** Confirm in a real group that (a) `chat_member` updates arrive for joins when the bot is admin, (b) a join service message is absent in some configurations, and (c) returning `EndGroups` from the `-5` guard really stops greetings at group 0 for the same update, as the AGENTS description implies.
3. **Direct Link Mini App from a group (Phase 3). MEDIUM.** URL button to `t.me/<bot>/<short>?startapp=...` inside a group, `initData.user` present and signed, and a non-target user tapping it gets rejected by the `user.id` binding. Also confirm `start_param` length and charset limits for the `t_<id>_<nonce>` format (keep the nonce about 22 base64url chars).
4. **Image classification (Phase 2d). LOW.** The existing Jev integration is text-only. Either defer image assist, or add a second classifier behind the same `Classifier` interface. Rules must remain sufficient on their own (they are, by design).
5. **Turnstile in Telegram WebViews (Phase 3). MEDIUM.** Widget rendering and token callback on Android, iOS and Telegram Desktop; confirm hostname reported by siteverify equals the Mini App host and that CSP `frame-src https://challenges.cloudflare.com` is sufficient.
6. **Product decisions to confirm with the owner:** confirm-before-fanout for `@username` targets (stale/recycled username risk); whether Staff Group members are protected from being targeted; whether the Staff Group itself should also receive the ban when the target is a member; lockdown auto-trigger defaults (recommended: off until configured).

---

## Sources

- Repository source, read 2026-10-04 (HIGH): `AGENTS.md`; `alita/utils/helpers/command_pipeline.go`; `alita/modules/{antiraid,captcha,greetings,membership,bot_updates,federations,moderation,bans,aispam,antiflood,users,logchannels,callback_codec,chat_permissions,registry,anonymous_admin_router}.go`; `alita/utils/{chat_status,callbackcodec,actionlog,extraction,httpserver}/`; `alita/db/{captcha,federations,logchannels,cache,antiraid,models}`; `alita/config/config.go`; `main.go`; `migrations/`.
- gotgbot v2 rc.36 vendored source in the module cache (HIGH): `ext/handlers/command.go` (Command struct and `CheckUpdate`), `request.go` (`TelegramError`), `gen_types.go` (`ResponseParameters.RetryAfter`, `WebAppInfo`), `gen_methods.go` (`SetChatPermissions`, `RestrictChatMember` options).
- Telegram Mini Apps initData validation and Direct Link behaviour (MEDIUM, secondary): https://docs.telegram-mini-apps.com/platform/init-data , https://docs.telegram-mini-apps.com/packages/tma-js-init-data-node/validating , https://core.telegram.org/bots/webapps (search snippet only; fetch blocked)
- `web_app` inline button private-chat restriction (MEDIUM): https://docs.python-telegram-bot.org/en/stable/telegram.inlinekeyboardbutton.html
- Cloudflare Turnstile server-side validation (MEDIUM, secondary): https://developers.cloudflare.com/turnstile/get-started/server-side-validation/ (search snippet; direct fetch blocked), https://flaviocopes.com/courses/cloudflare/verify-context-and-prevent-replay/
- `use_independent_chat_permissions` semantics (MEDIUM): https://docs.aiogram.dev/en/dev-3.x/api/methods/restrict_chat_member.html

---
*Architecture research for: Fuku Robot (Alita fork) Staff Group, raid lockdown, Turnstile captcha, settings menu*
*Researched: 2026-10-04*
