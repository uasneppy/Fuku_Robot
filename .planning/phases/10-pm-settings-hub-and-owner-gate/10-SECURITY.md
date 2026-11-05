---
phase: "10"
slug: "pm-settings-hub-and-owner-gate"
status: verified
# threats_open = count of OPEN threats at or above workflow.security_block_on severity (the blocking gate)
threats_open: 0
asvs_level: 1
register_authored_at_plan_time: true
created: "2026-11-04"
---

# Phase 10 — Security

> Per-phase security contract: threat register, accepted risks, and audit trail.
> Audited by `gsd-security-auditor` against the code at `b05388c`; every mitigation was read in the code and the
> security-relevant tests were run (all green, no skips). Evidence below is `file:line` in `fuku/modules/` unless a
> path is given. Short names: `gate` = `settings_gate.go`, `menu` = `settings_menu.go`, `set` = `settings.go`,
> `acc` = `settings_access.go`, `hub` = `settings_hub.go`, `pages` = `settings_pages.go`,
> `feed` = `admin_index_feed.go`, `sweep` = `admin_index_sweeper.go`, `acache` = `fuku/utils/cache/adminCache.go`.

---

## Trust Boundaries

| # | Boundary | Description | Data Crossing |
|---|----------|-------------|---------------|
| 1 | Telegram user → bot | `/settings`, `/start settings_<id>`, `settings\|v1` and `shub\|v1` callback data, all attacker-controlled | chat IDs, user IDs, action codes |
| 2 | Bot → Telegram Bot API | live `getChatMember`, `getChatAdministrators`, `getChat`; can fail, time out or be slow | membership and role records |
| 3 | Replica → PostgreSQL | owner switch (`chat_settings_access`) and admin roster (`chat_admin_index`) | switch value, admin user IDs |
| 4 | Replica ↔ replica | shared PostgreSQL + Redis sweep lock; per-replica owner-state memo | switch value, lock key |
| 5 | Admin cache → database (observer hook) | one hook in `fuku/utils/cache` writes the roster | admin lists |
| 6 | Group menu → group members | menu text is readable by every member of the group | page state words, never the Staff Group |
| 7 | Roster rows → private list | the list is a hint only; every pick is re-checked live | group titles |
| 8 | Backup / import → new tables | the two tables are in no backup module | — |
| 9 | Locale files → rendered messages | placeholders, tags, the set of 8 locales | translated strings |
| 10 | SQLite tests → PostgreSQL production | two PostgreSQL-only guarantees are in `make test-postgres-integrity` (CI) | — |
| 11 | Process shutdown → DB close | `StopAdminIndexSweeper` runs before the DB close (LIFO) | — |
| 12 | Third-party dependencies | `go.mod` / `go.sum` unchanged since baseline `6ef6ad9` | — |

---

## Threat Register

`T-10-SC` (package installs, high, mitigate) is registered in every plan (10-01 to 10-11); it is closed in all 11
because `go.mod` and `go.sum` are unchanged since `6ef6ad9`. All other IDs appear in exactly one plan.

| Threat ID | Plan | Category | Component | Severity | Disposition | Mitigation (evidence) | Status |
|-----------|------|----------|-----------|----------|-------------|-----------------------|--------|
| T-10-01 | 10-01 | Tampering | `adminindex.ReplaceChat` under concurrent replicas | medium | mitigate | per-chat `pg_advisory_xact_lock` first on PostgreSQL, upsert then `NOT IN` prune in one transaction (`fuku/db/adminindex/repository.go:90-111`); `TestAdminIndexReplaceConcurrent` in the Makefile integrity list | closed |
| T-10-02 | 10-01 | Info disclosure | backup export of the new tables | medium | mitigate | no reference in `fuku/db/backup/`; `TestAccessTablesStayOutOfBackup` | closed |
| T-10-03 | 10-01 | Elevation | `AdminsAllowed` default tag | high | mitigate | no `default:` tag (`fuku/db/models/chat_settings_access.go:16`); column-targeted upsert (`fuku/db/menuaccess/repository.go:45-58`); `TestMenuAccessUpsert` | closed |
| T-10-04 | 10-01 | Elevation | `AdminsAllowedFresh` read failure | high | mitigate | missing row is `(false, nil)`, any other error returned, gate refuses (`menuaccess/repository.go:28-41`, `gate:100-104`); `TestMenuAccessReadError` | closed |
| T-10-05 | 10-01 | DoS | migration at startup | medium | mitigate | `IF NOT EXISTS`, inline constraints, no `BEGIN`/`CONCURRENTLY` (`migrations/20261104120000_add_settings_hub_tables.sql`); `TestSettingsHubMigrationStatements` | closed |
| T-10-06 | 10-02 | Info disclosure | new `settings_` strings | medium | mitigate | placeholders only `{icon}`, `{page}`, `{state}`; present in 8 locales; `settings_locale_test.go` guards | closed |
| T-10-07 | 10-02 | Info disclosure | empty-list text | medium | mitigate | one key, one code path (`hub:138-141`); `TestSettingsEmptyListSameForAll` | closed |
| T-10-08 | 10-02 | Tampering | HTML in locale values | low | mitigate | tag allow-list, balanced tags, tag-free alert keys (`settings_locale_test.go`) | closed |
| T-10-35 | 10-03 | Tampering | locale-set drift | low | mitigate | `TestLocalesAreTheEight` (`fuku/i18n/locale_set_test.go`) | closed |
| T-10-36 | 10-04 | Elevation | harness seed `admins_allowed=true` | medium | mitigate | seed only in `_test.go` files; pinned by `TestSettingsEnvSeedsAdminAccess` | closed |
| T-10-37 | 10-04 | Tampering | PostgreSQL-only guarantees | medium | mitigate | `Makefile:41,44`; CI runs `make test-postgres-integrity` (`ci.yml:335`) | closed |
| T-10-09 | 10-05 | Elevation | in-group `/settings` and press | high | mitigate | only callers of `settingsGate`: `gate:279`, `set:185`, `menu:360`; live `getChatMember` record; `TestSettingsInGroupRefusal`, `TestSettingsGatePress`, `TestSettingsGateAllDoors` | closed |
| T-10-10 | 10-05 | Elevation | owner-state read | high | mitigate | every unknown state refuses: error, timeout, late answer, empty list, presser not listed, unreadable switch (`gate:100-231`); zero value is `Undecided`; see AR-01 | closed |
| T-10-12 | 10-05 | Spoofing | anonymous admin taken for the owner | high | mitigate | anonymous proof always first (`lockdown.go:157-162`), `anonPipelineHandler` re-runs `RequiredChecks` incl. the gate; `TestSettingsAnonymousOwnerGate` | closed |
| T-10-13 | 10-05 | DoS | `getChatAdministrators` per gated press | medium | mitigate | reached only for an admin with the switch off; singleflight, 3 s bound, 30 s positive memo (`gate:69-165`) | closed |
| T-10-14 | 10-05 | Elevation | stale owner memo | medium | mitigate | switch read before memo; only "owner exists" is memoised, which can only keep an admin out | closed |
| T-10-15 | 10-05 | Tampering | gate leaking onto setting commands | medium | mitigate | `settingsGate` called only from `gate`, `set`, `menu`; `TestSettingsCommandsUngated` | closed |
| T-10-16 | 10-06 | Elevation | Access page controls | high | mitigate | three layers: `OwnerOnly` page + `openPage` guard, `settingsRightOwner`, `applyAdminsAllowed` re-checks the creator (`acc:37-77,159-162`, `menu:452-455`); `TestSettingsAccessForgedPresses` | closed |
| T-10-17 | 10-06 | Tampering | ON by one tap or a replayed card | medium | mitigate | `v=1` draws the confirm card only; Yes checks expiry and re-runs the pipeline (`acc:169-198`); `TestSettingsAccessCard` | closed |
| T-10-18 | 10-06 | Tampering | stale switch on another replica | high | mitigate | fresh uncached PostgreSQL read through one seam for page and gate; `TestSettingsAccessAcrossReplicas` | closed |
| T-10-19 | 10-06 | Info disclosure | Access text in a group menu | low | accept | wording is generic; page shown only to the live creator | accepted (AR-04) |
| T-10-20 | 10-07 | Tampering | observer on stale or failed loads | medium | mitigate | `Loaded` only for a non-empty list with a matching generation; `BotGone` only for 400/403 (`acache:83-89,155-169,250-252`); `TestAdminListObserver` | closed |
| T-10-21 | 10-07 | DoS | observer panic or slow write | medium | mitigate | recover (`acache:61-72`, `feed:36`), 3 s bound, failure only logged | closed |
| T-10-22 | 10-07 | Info disclosure | roster shows a title to a just-demoted admin | low | accept | roster is a hint; every pick is checked live | accepted (AR-04) |
| T-10-23 | 10-08 | DoS | Telegram rate limits during a sweep | medium | mitigate | SETNX lock, 250 ms pace, 5 s per call, 429 backoff capped at 30 s (`sweep:34-38,177,213-303`) | closed |
| T-10-24 | 10-08 | DoS | shutdown overrun | medium | mitigate | calls derive from the sweep context; lock release bounded; `main.go:161-164,216-220` order; `TestAdminIndexSweepStops` | closed |
| T-10-25 | 10-08 | Tampering | rows deleted on a transient error | low | mitigate | rows change only on definitive facts (`feed:77-112`); WR-03 fix `0be0f65` narrowed "gone" to 400/403; `TestAdminIndexSweepKeepsRowsOnError`, `TestAdminIndexBotLookupRefused` | closed |
| T-10-38 | 10-08 | DoS | group `/settings` repair refresh | low | mitigate | runs after `RequiredChecks`, at most 2 calls, 10 s bound, error only logged (`set:32,122-136`) | closed |
| T-10-26 | 10-09 | Info disclosure | forged pick `c` | medium | mitigate | one neutral answer by Telegram error class at each step (`menu:332-395`, `gate:256-262`); `TestSettingsPickSameAnswer`; residual in AR-02 | closed |
| T-10-27 | 10-09 | Spoofing | replayed or forwarded list button | medium | mitigate | list rebuilt from `query.From.Id`, private only, service identities refused (`hub:219-239`) | closed |
| T-10-28 | 10-09 | Info disclosure | empty list | medium | mitigate | one text for everyone (`hub:138-141`) | closed |
| T-10-29 | 10-09 | Elevation | pick bypassing the gate | high | mitigate | pick is a core action through the full pipeline: live member, gate, then `getChat` (`menu:276-401`); `TestSettingsGateAllDoors` (pick door) | closed |
| T-10-30 | 10-09 | Tampering | `shub` callback in a group chat | low | mitigate | refused as an invalid request (`hub:219-221`) | closed |
| T-10-31 | 10-09 | DoS | overflowing callback data ships a dead button | low | mitigate | overflow counted and logged; `TestSettingsHubCallbackBytes` | closed |
| T-10-32 | 10-10 | Info disclosure | Main status block in a group menu | high | mitigate | every `MainState` returns fixed words or numbers; `settingsStaffViewFor` copies the Staff Group only for the creator's private menu; `TestSettingsStaffPrivacy` (8 languages) | closed |
| T-10-33 | 10-10 | Elevation | Access row shown to admins | medium | mitigate | `OwnerOnly` entries skipped without calling `MainState` unless the viewer is the live creator (`pages:456-462`); `TestSettingsAccessHiddenFromAdmins` | closed |
| T-10-34 | 10-10 | DoS | Main buttons dropped by overflow | low | mitigate | overflow counted (`pages:223-242`); `TestSettingsCallbackBytes` | closed |
| T-10-11 | 10-11 | Info disclosure | deep link answers | medium | mitigate | 4xx or non-admin gives `settings_start_not_admin`; 429/5xx/transport give `settings_check_failed` (`set:171-206`); `TestSettingsStartDeepLinkSameAnswer`; residual in AR-02 | closed |
| T-10-39 | 10-11 | Elevation | deep link door | high | mitigate | strict parse, live `getChatMember`, `settingsGate` on that record, only then `getChat` (`set:165-195`) | closed |
| T-10-40 | 10-11 | Elevation | drift between the doors | medium | mitigate | `TestSettingsGateAllDoors`: five fixtures across five doors incl. the pick | closed |
| T-10-SC (x11) | 10-01 … 10-11 | Tampering | package installs | high | mitigate | `go.mod` and `go.sum` unchanged since `6ef6ad9` | closed |

*Status: open · closed · accepted*
*Severity: critical > high > medium > low — only open threats at or above workflow.security_block_on (high) count toward threats_open*

**Tally:** 51 register rows (40 numbered IDs plus T-10-SC in 11 plans): 49 closed, 2 accepted, 0 open.

---

## Accepted Risks Log

| Risk ID | Threat Ref | Rationale | Accepted By | Date |
|---------|------------|-----------|-------------|------|
| AR-01 | WR-01 (review), T-10-10 | An owner whose first name is exactly "Deleted Account" (case-insensitive) with no last name and no username looks like a deleted owner, so with the switch off administrators are let in. Telegram gives no second signal; only the owner's own display name causes it; the live creator is gated in before this heuristic runs, so the owner is never locked out; administrators can already run every setting command (MENU-13), and creator-only actions (Link, Unlink, Access) keep their own live creator check. The comment and AGENTS.md state the limit (commit `0bdbfac`). Residual: low. Recheck: owner live check A1 in `10-SPIKE.md`. | orchestrator (autonomous run), owner to review | 2026-11-04 |
| AR-02 | WR-02 (review), T-10-11, T-10-26 | A hand-built `settings\|v1` callback with a forged `c` on a non-pick private action (`op`, `tg`, `st`, `bk`, page actions) shows the "cannot open" alert when the bot is not in that chat and "not admin" when it is, so it tells the sender whether the bot is in chat X. The pick and the deep link are neutral. Accepted because it needs a hand-built callback and a guessed chat ID, leaks only bot membership, exposes no authority (every press still needs creator or admin plus the gate), and Phase 9 decision D-11 (the self-closing menu a real owner sees after the bot left) is locked. Residual: low to medium. Suggested later hardening: apply the pick's class rule to every private `c` action once the presser is proven an administrator. | orchestrator (autonomous run), owner to review | 2026-11-04 |
| AR-03 | IN-01, IN-02, IN-04 (review) | IN-01: the owner switch resets to owner-only when a basic group migrates to a supergroup (not re-keyed); fails in the safe direction. IN-02: the roster hint can show a title to a demoted admin until the next refresh (same as T-10-22). IN-04: one locked DB transaction per admin-cache load in a group where the bot is not an admin (performance only, research R-05.1). | orchestrator (autonomous run) | 2026-11-04 |
| AR-04 | T-10-19, T-10-22 | Register `accept` rows, low severity: Access wording is generic and shown to the live owner only (if the owner opens the menu in the group, members can read "Admins can change settings: On/Off"); the roster is a hint re-checked on every pick. | planner (plan time) | 2026-11-04 |

*Accepted risks do not resurface in future audit runs.*

---

## Security Audit Trail

| Audit Date | Threats Total | Closed | Open | Run By |
|------------|---------------|--------|------|--------|
| 2026-11-04 | 51 | 49 (+2 accepted) | 0 | gsd-security-auditor (verdict), orchestrator (file) |

Tests run for the audit (all passing, no skips): the gate, owner-state, access, hub, deep-link, all-doors, privacy,
main-grid, callback-byte, harness-seed and admin-index sets in `fuku/modules`; `fuku/db/adminindex`,
`fuku/db/menuaccess`, `fuku/db/migrations`, `fuku/utils/cache`, `fuku/i18n`. The PostgreSQL-only run
(`make test-postgres-integrity`) is executed by CI (`.github/workflows/ci.yml:335`), not here.

---

## Sign-Off

- [x] All threats have a disposition (mitigate / accept / transfer)
- [x] Accepted risks documented in Accepted Risks Log
- [x] `threats_open: 0` confirmed

**Approval:** verified 2026-11-04
