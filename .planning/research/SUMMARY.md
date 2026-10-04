# Project Research Summary

**Project:** Fuku Robot (Alita fork) — Telegram group-management bot milestone  
**Domain:** Go/gotgbot v2 Telegram bot; cross-group moderation, raid protection, captcha, settings  
**Researched:** 2026-10-04  
**Overall Confidence:** MEDIUM-HIGH (codebase HIGH, platforms MEDIUM due to egress blocks)

---

## Executive Summary

This milestone adds four interconnected features to an existing Telegram group-management bot: Staff Group fan-out moderation, raid lockdown with multiple triggers, Cloudflare Turnstile Mini App captcha, and an inline settings menu. The architecture is well-founded on existing patterns (captcha, federation, antiraid) and introduces no new external dependencies beyond two small Go modules (`golang.org/x/time/rate` for rate limiting and `anthropic-sdk-go` for image classification). The core risk is **authorization and state management complexity**: five distinct subsystems (Staff links, lockdowns, captchas, callbacks, settings) must coordinate around live Telegram admin snapshots and durable Postgres state while avoiding cache staleness, TOCTOU races, and permission leaks. The recommended approach isolates authorization into a fresh per-call helper, fixes an existing bug in permission restore (`resolveUnmutePermissions`), uses atomic state transitions with partial-unique indexes, and runs all new code through comprehensive table-driven tests before attempting any realistic scenarios.

Execution order is strict: **Phase 1 (Staff Group) builds reusable infrastructure** (paced fan-out executor, fresh admin snapshot, gated command dispatch, audit table) that Phases 2–4 depend on. Phase 2 (Raid lockdown) adds durable state and triggers. Phase 3 (Turnstile) integrates the Mini App and HTTP verification. Phase 4 (Settings menu) is a thin registry layer that reuses Phase 1–3 repos. This order avoids rebuilding and reduces the cost of fixing authorization bugs early.

---

## Key Findings

### Recommended Stack

**Core technologies (new):**
- `golang.org/x/time/rate` v0.16.0: token-bucket rate limiter for Staff Group fan-out (~20 req/s, shared with other hot paths), replacing manual pacing
- `github.com/anthropics/anthropic-sdk-go` v1.78.0: Claude Haiku 4.5 for image classification in the raid pipeline (vision-capable, $1/$5/MTok, structured outputs, retries); text remains on TypeSafe Jev (existing)
- Cloudflare Turnstile `siteverify` API (no SDK needed; stdlib `net/http`, ~60 lines in the style of `aispam_jev.go`)
- Telegram Mini App `initData` validation (stdlib `crypto/hmac`, hand-written, ~30 lines) and direct-link launch via `?startapp=` nonce
- Go `embed` + `html/template` + existing HTTP server for hosting the captcha page (no frontend toolchain)

**Existing modules reused:**
- `redis/go-redis/v9` v9.22.0 (nonce store with `GETDEL` for single-use, lockdown counters, alert tokens)
- `google/uuid` v1.6.0 (nonces and idempotency keys)
- `alita/utils/callbackcodec` (all buttons, 64-byte cap)
- `alita/utils/logredact` (register `TURNSTILE_SECRET_KEY` and `ANTHROPIC_API_KEY`)

**Confidence:** STACK overall is MEDIUM-HIGH. Library versions verified via `proxy.golang.org`. Telegram and Cloudflare behavior claims come from web search (egress blocks prevented primary doc access); each phase must re-confirm items marked "verify in phase".

### Expected Features

**Launch with (v1):**
1. **Staff Group** (Phase 1): owner-of-both link/unlink, live admin-right checks per group, `/ban /mute /kick /unban /unmute` (timed) fan-out, per-group result summary (done/skipped/failed), 429-safe pacing, audit table, `/staff` panel, chat migration re-keying
2. **Raid Lockdown** (Phase 2): durable DB state, kick joiners, mute non-admins/approved, manual + auto triggers (join surge, flood, AI burst), alert to Staff Group + log channel, Lift and "Ban N joiners" buttons, permission snapshot/restore
3. **Turnstile captcha** (Phase 3): new `webapp` mode, server-side `initData` + `siteverify` validation, nonce-bound to user+chat, math/text modes kept as fallback
4. **Settings menu** (Phase 4): `/settings` with live admin re-check per press, pages for Staff, Lockdown, Captcha, Antiflood

**Dependencies:** Staff actions enable the audit table and `/staff` panel (Phase 1 foundation). Lockdown requires durable state + chat-level counters + join log. Turnstile requires the permission restore fix from Phase 2. Settings menu wraps existing repos; no new logic.

### Architecture Approach

The system adds 7 major components across 2 new database tables (staff + lockdown), 2 new handler groups (-3 for ownership watcher, 2 for flood detector), and 1 HTTP verification path. **Pattern highlights:**

1. **Gated command dispatch (Pattern 1):** Staff commands `/ban`, `/mute`, etc. reuse the names of per-group commands via a CheckUpdate gate that decides if the Staff Group context applies. Priority ordering enforces correct shadowing.

2. **Fresh admin snapshot per-group (Pattern 2):** A dedicated authorizer reads live `getChatAdministrators` per target group, never the cached admin list. This is the core safety invariant: no TOCTOU window between the check and the action.

3. **Bounded fan-out executor (Pattern 3):** Worker pool with a shared token bucket, 429 retry with backoff, typed result per group. Reused by staff actions and "Ban N joiners" button.

4. **Persisted state machine (Pattern 4):** Lockdown row in Postgres with an atomic compare-and-set on `state='active'`, snapshot of permissions stored in the row (never re-read after lock), optional per-user approved-user exceptions. Postgres is source of truth; Redis holds only counters.

5. **Namespaced callbacks (Pattern 5):** All buttons use `callbackcodec` with a DB row ID in the payload. The handler loads the row, takes chat/user from the row (not the message), and re-authorizes.

6. **Settings page registry (Pattern 6):** Each feature module registers a menu page. Pages call existing repo setters (which already `DeleteCache`), so the menu adds no new write paths.

### Critical Pitfalls (Top 5)

1. **Pitfall 1: Using cached admin list for cross-group checks.** The existing `IsUserAdmin` returns true for bot IDs, caches for 30 min, and never checks `can_restrict_members`. Staff actions must call a dedicated fresh authorizer that verifies creator status in **both** Staff Group and target group before every action. (Phase 1; blocks everything else)

2. **Pitfall 3: TOCTOU on ownership and chat migration.** A link between groups must be re-verified at every fan-out. Chat ID changes on group→supergroup upgrade; existing code does not handle `migrate_to_chat_id`. A single per-group live `getChatMember` check during fan-out is the safeguard; also add a periodic sweep. (Phase 1)

3. **Pitfall 7b: Permission restore bug freezes lockdown.** `/unmute`, captcha pass, and unban all call `resolveUnmutePermissions(getChat)` to copy current defaults into a per-user restriction. During lockdown the defaults are all-false; a user unmuted during lockdown gets a locked restriction and stays muted after lift. **Must be fixed before Phase 2:** make `resolveUnmutePermissions` return the active lockdown's pre-lock snapshot, not the current defaults. (Phase 2 blocker)

4. **Pitfall 9: Lockdown state in Redis only, and module name collision.** Legacy antiraid is Redis-only; a Redis flush loses lockdown (violates "never lifts"). The new lockdown must use Postgres (source of truth) with Redis accelerators. Also, reusing the `antiraid` callback namespace or command names risks old buttons firing new behavior. Use new namespace + new command names. (Phase 2)

5. **Pitfall 10: Three join paths, double-counting, and kick loops.** `chat_member` updates, service messages, and `chat_join_request` all signal a join. Service messages can be absent in large groups. Use `(chat, user)` as the ZSET member (idempotent, dedupes) but never gate the kick on dedupe. Lockdown guard must run before group 0 so captcha/greetings never run for a kicked user. (Phase 2)

---

## Implications for Roadmap

### Suggested Phase Structure

**Phase 1: Staff Group — Build Authorization Foundation**

**Rationale:** Owner's priority (PROJECT.md). Establishes reusable infrastructure that all later phases depend on: fresh admin authorizer, bounded fan-out executor, command-gating pattern, audit table, link invalidation contract.

**Delivers:**
- `chat_status.FreshAdmins(chatID) -> Decision` authorizer (uncached, live `getChatAdministrators`, owner/right checks, rejects bot IDs and anonymous senders)
- `utils/fanout.Run(ctx, targets, op) -> []Result` executor (worker pool ~4-5, shared token bucket, 429 retry with backoff, typed results)
- `helpers.CommandDescriptor.Gate` optional predicate (shadows existing commands in Staff Group only)
- `db/staff` repo: `staff_groups`, `staff_group_links` with `owner_user_id` and re-verification contract
- Group `-3` ownership watcher (`chat_member` events, event-driven + periodic sweep)
- Staff commands: `/setstaff`, `/linkstaff`, `/unlinkstaff`, `/ban /mute /kick /unban /unmute` (timed) with live per-group checks
- Audit table: `staff_actions(issuer, target, action, reason, per_group_outcomes)`
- `/staff` panel (pure render function, status per link: linked/admin ok/right ok/owner matches)
- Chat migration re-keying: `migrate_to_chat_id`/`migrate_from_chat_id` handlers re-key link rows and staff group designation

**Avoids Pitfalls:** 1 (authorization), 3 (TOCTOU), 6 (command collision, via gating)

**Research flags:** None; follows established patterns

---

**Phase 2: Raid Lockdown — Durable State & Detection**

**Rationale:** Depends on Phase 1 (alert routing, fan-out executor). Introduces durable state and detection rules.

**Pre-planning spikes required:**
- Spike 1: Test approved-user per-user permission override on throwaway group
- Spike 2: Confirm join delivery, `EndGroups` behavior
- Spike 4: Evaluate image classification

**Delivers:**
- `db/lockdown` repo with durable state, partial-unique index for idempotent triggering
- Kick-on-join guard with deduping but always-kick behavior
- Join-rate, flood, and AI-burst triggers
- Lockdown alert with Lift and "Ban N joiners" buttons
- Manual `/lockdown`, `/unlockdown`, status commands
- **Critical fix:** `resolveUnmutePermissions` now returns pre-lock snapshot, not current defaults
- Permission snapshot in raw JSON with `use_independent_chat_permissions=true`

**Avoids Pitfalls:** 3 (TOCTOU), 7 (permission restore), 8 (snapshot), 9 (Redis-only), 10 (join paths), 11 (false positives)

**Research flags:** Phase 2 needs pre-planning spikes on approved-user permissions, join delivery, and AI classification

---

**Phase 3: Turnstile Mini App Captcha — Integration & Hardening**

**Rationale:** Depends on Phase 2 (permission fix). Independent HTTP integration with high security surface.

**Pre-planning spikes required:**
- Spike 3: Test direct-link Mini App launch from group on real clients
- Spike 5: Test Turnstile widget rendering across clients

**Delivers:**
- `utils/tgwebapp` and `utils/turnstile` (pure, fully table-tested)
- New `turnstile` captcha mode alongside math/text
- GET `/captcha/app` (embedded HTML + JS, no build toolchain)
- POST `/captcha/verify` with `initData`, nonce, and siteverify validation
- Shared completion path with existing captcha
- Rate limiting, body limits, fail-closed semantics
- Deploy prerequisite docs (BotFather `/newapp`, public HTTPS, Turnstile allowlist)

**Avoids Pitfalls:** 7 (permission fix done), 13–18 (Mini App specifics)

**Research flags:** Phase 3 needs pre-planning spikes on direct-link launch and widget rendering

---

**Phase 4: Settings Menu — Page Registry & Re-check**

**Rationale:** Last; wraps repos from Phases 1–3. No new business logic.

**Delivers:**
- `settings_menu.MenuPage` registry with permission levels
- Pages for Staff, Lockdown, Captcha, Antiflood
- Live admin re-check on every button press
- Localization for 7 locales
- Pagination for long lists

**Avoids Pitfalls:** 19–21 (callback size, permission re-check, locale)

**Research flags:** None; builds on Phase 1–3 patterns

---

### Phase Ordering Rationale

- **Phase 1 first (owner priority + foundation).** Authorization bug here breaks everything; fix early.
- **Phase 2 depends on Phase 1 for alert routing and fan-out.** Spike decisions required before planning.
- **Phase 3 independent in code, but depends on Phase 2's permission fix.** Avoids reintroducing the muted-after-lift bug.
- **Phase 4 last.** Wraps existing repos; no new logic.
- **Chat migration re-keying (within Phase 1).** Dependency for both links and lockdowns.

---

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| **Stack** | MEDIUM-HIGH | Library versions verified via `proxy.golang.org`. Telegram Mini App and Cloudflare behavior from web search (primary docs egress-blocked); must verify in phase. |
| **Features** | MEDIUM-HIGH | Derived from existing bot behavior and PROJECT.md decisions. Dependencies well-understood. |
| **Architecture** | HIGH | All patterns verified in source code. Component boundaries clear. Existing subsystems thoroughly analyzed. |
| **Pitfalls** | MEDIUM-HIGH | 21 critical pitfalls documented with prevention strategies. Codebase traps verified in source. Telegram API behavior marked "verify in phase". |

**Overall: MEDIUM-HIGH.** Roadmap can proceed with high confidence in Phase order and architecture. Implementation confidence depends on spike results.

---

## Gaps to Address

1. **Approved-user permission exemption (Phase 2 spike).** Whether per-user restrict can override locked defaults is unconfirmed. Fallback: approved users muted during lockdown.

2. **Join delivery reliability (Phase 2 spike).** Confirm `chat_member` updates, service message absence, and `EndGroups` behavior in realistic groups.

3. **Mini App launch mechanics (Phase 3 spike).** Direct-link button behavior, `initData` presence, and Turnstile rendering on real clients.

4. **Legacy `/antiraid` migration.** Define treatment of old command, `raidtime`/`raidactiontime` settings. Confirm with owner in Phase 2 planning.

5. **Multi-replica token bucket.** Single-replica uses in-process bucket; multi-replica requires Redis. Decision on deployment goal needed.

---

## Sources

**Primary (HIGH):**
- Repository source (2026-10-04): AGENTS.md, `alita/modules/`, `alita/db/`, architecture verified in code.
- gotgbot v2 rc.36 vendored source: Command dispatch, TelegramError, permissions.

**Secondary (MEDIUM):**
- Claude API skill (2026-09-25): Haiku 4.5 capabilities, pricing, image support.
- Web search summaries: Turnstile, Mini App `initData`, Bot API 10.2.

**Tertiary (MEDIUM, verify in phase):**
- Telegram Bot API behavior: Admin-call limits, permission semantics, chat-member delivery.
- Cloudflare Turnstile: Token validity, error codes, test keys.
- Mini App platform: Direct-link behavior, `start_param` limits.

---

*Research synthesized: 2026-10-04*  
*Confidence: MEDIUM-HIGH for phase order; MEDIUM for platform behaviors (validate in spikes)*  
*Ready for roadmap creation and requirements definition*
