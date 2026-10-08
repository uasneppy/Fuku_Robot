# Walking Skeleton — Fuku Robot

**Phase:** 1
**Generated:** 2026-10-04

This is a brownfield project (a fork of Alita Robot). The skeleton does not re-scaffold anything. It proves that one new feature module fits end-to-end into the existing bot: the dispatcher, the migration runner, the repository and cache layer, Telegram replies, the 7 locale files, the docs generator, and the Docker deploy with `AUTO_MIGRATE=true`. Plan `01-01` builds it.

## Capability Proven End-to-End

A group's owner sends `/setstaff` in their supergroup. The bot checks live with Telegram that the sender is the group's creator and stores the Staff Group in PostgreSQL. `/staff` in that group then replies with the Staff Group help text and the group's chat ID. This runs on the existing Docker deployment, where `AUTO_MIGRATE=true` applies the new migration at startup.

## Architectural Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Framework | Existing gotgbot v2 dispatcher. New module `alita/modules/staff.go` is registered with `RegisterLegacyModule("Staff", 236, LoadStaff)`. Commands use `helpers.WrapCommand` with `xxxDesc` descriptors defined in `staff.go`. | AGENTS.md rules. The docs generator only finds commands registered as `helpers.WrapCommand(dispatcher, xDesc, staffModule.handler)`. |
| Event watchers | A separate non-help module `StaffWatchers` (`alita/modules/staff_watchers.go`, priority 237) runs at handler group `-3`. Watchers return `ext.ContinueGroups` and use `SetAllowBot(true)`. | Group `-3` runs before `-1`'s `botJoinedGroup`, which returns `EndGroups`. In gotgbot, `ContinueGroups` means "next handler in the same group" (RESEARCH C1). A separate module lets the watcher slices ship without editing `staff.go`. |
| Data layer | Two tables, `staff_groups` and `staff_group_links`, created by the append-only migration `migrations/20261004120000_add_staff_groups_and_links.sql`. GORM models are in `alita/db/models/staff.go`. The repository is `alita/db/staff/`. `UNIQUE(chat_id)`, `UNIQUE(group_chat_id)`, `CHECK(group_chat_id <> staff_chat_id)` and a health enum CHECK are enforced in the DB. The cross-table D-10 trigger is a separate migration, gated by a decision in plan `01-04`. | D-09, D-10, D-12. The DB arbitrates races. No FK on chat IDs (the `federation_chats` precedent), so SQLite tests behave the same as PostgreSQL. |
| Cache | Cached gate reads only: `staff_group:<chat>` and `staff_link_of:<group>`, with a zero-value sentinel for "none". Both prefixes are in `skipLocal`. Authority reads are `...Fresh` and uncached. Every write calls `cache.DeleteCache` after commit, for both old and new IDs on a re-key. | AGENTS.md Data rules. Several replicas are the deployment target, so a replica must not serve a stale link for `CACHE_LOCAL_TTL`. |
| Authority ("auth") | `chat_status.CheckOwner(b, chatID, userID)` calls `getChatAdministrators` live, with no cache. It returns three states: match, mismatch, or unknown. Only a mismatch may remove anything, and an unknown fails closed for actions. Anonymous and service senders are refused with "post as yourself". | The core value. Telegram identity is the only identity, so there are no app sessions or passwords. The cached admin list (30 min) is never an authority. |
| Exactly-once side effects | Each removal or health change is one conditional statement (`DELETE ... WHERE id AND owner_user_id`, `UPDATE ... WHERE health <> ?`). Only the caller with `RowsAffected == 1` posts the Staff Group notice. | Works the same with or without Redis, and across replicas. |
| UI | Telegram command replies plus inline buttons, using the `staff|v1|...` callback namespace through `callbackcodec` (64-byte cap). The panel is built by a pure `renderStaffPanel` in `alita/modules/staff_panel.go`. | The Phase 9 settings menu can reuse the pure renderer. |
| i18n | Every new string goes in all 7 locales (`en es fr hi id pt ru`), using only literal `tr.GetString("staff_...")` calls. `alita/i18n/staff_locale_test.go` checks key parity, non-empty values and placeholder sets. | PLAT-03. `make check-translations` only sees literal `GetString` keys (Pitfall 11). |
| Background work | An hourly sweeper (`alita/modules/staff_sweeper.go`) holds a Redis `SetNX` lock `alita:staff:sweep:lock`. Its lifecycle context and WaitGroup are started from `postInit` and stopped by a shutdown handler. | D-15.2 and PLAT-01. One replica sweeps at a time, and the work is idempotent if Redis is missing. |
| Deployment target | The existing Docker image and `docker-compose.yml` (`AUTO_MIGRATE: "true"`). For a local run with Postgres and Redis available: `go run main.go`. | No deployment changes. New migrations apply on the next deploy. |
| Directory layout | `alita/modules/staff*.go` (commands, panel, link flows, watchers, recheck, sweeper), `alita/db/staff/` (repository, rekey), `alita/db/models/staff.go`, `alita/utils/chat_status/owner.go`, `locales/*.yml`. | Follows the repo's domain-package pattern. |
| Test harness | Real SQLite through `internal/testdb.Run` and the modules `TestMain`, miniredis, and the hand-written `staffBotClient` (`alita/modules/staff_helpers_test.go`) with per-chat creators, bot roles, members and scripted `TelegramError`s. The marker locale `withStaffLocale(t)` is built from `locales/en.yml`. Every test file carries `//go:build testtools`. | AGENTS.md: real fixtures, no mock libraries. In the sandbox, run `go test -tags testtools -race -count=1 ...`, because `make test` and `make lint` fail here for toolchain reasons. |

## Stack Touched in Phase 1

- [x] Project scaffold: existing; reused, not re-created (dispatcher, migration runner, test harness, docs generator)
- [ ] Routing: `/setstaff` and `/staff` registered through `helpers.WrapCommand` in `LoadStaff` (plan 01-01)
- [ ] Database: one real write (`CreateStaffGroup`) and one real read (`GetStaffGroup` cached gate plus `GetStaffGroupFresh`) against the new migration (plan 01-01)
- [ ] UI: `/staff` replies with the panel text, and Telegram's command reply is the interaction (plan 01-01). Inline buttons arrive in plans 01-02, 01-05, 01-06 and 01-09.
- [ ] Deployment: the migration applies through `AUTO_MIGRATE=true` in `docker-compose.yml`, with no manifest change. The local full-stack run is `go run main.go` with `DATABASE_URL`, `REDIS_ADDRESS` and `BOT_TOKEN` set.

## Out of Scope (Deferred to Later Slices)

- Staff moderation actions across groups (`/ban`, `/mute`, `/kick`, `/unban`, `/unmute` in the Staff Group): Phase 2
- The audit record, recent actions in `/staff`, and "Undo everywhere": Phase 3
- The "in lockdown" status in `/staff`: Phase 4
- `/staff` in private chats or in linked groups, and private messages to the link maker (CONTEXT Deferred Ideas)
- Nested Staff Groups (rejected by D-10)
- Migrating other per-chat settings (warns, filters and so on) on a basic-group to supergroup upgrade (an existing gap)

## Subsequent Slice Plan

Each later phase adds one vertical slice on top of this skeleton without changing its architectural decisions:

- Phase 2: staff act on a bad actor in every linked group at once (fan-out, Confirm tap, live per-group admin checks, Redis-paced)
- Phase 3: staff actions are logged, listed in `/staff` and reversible
- Phase 4: manual `/lockdown` and `/unlockdown`; `/staff` gains the in-lockdown status
- Phase 5: lockdown alerts with one-tap responses in the Staff Group
- Phase 6: automatic raid detection replaces `/antiraid`
- Phase 7: AI-assisted raid triggers (TypeSafe and Gemini)
- Phase 8: Turnstile web captcha as a Telegram Mini App
- Phase 9: `/settings` inline menu, which reuses `renderStaffPanel` for its Staff Group page
