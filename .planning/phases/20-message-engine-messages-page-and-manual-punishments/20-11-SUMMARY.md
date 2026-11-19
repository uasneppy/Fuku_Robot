---
phase: 20-message-engine-messages-page-and-manual-punishments
plan: 11
subsystem: event messages / Messages page / end to end and phase gates
status: complete
tags: [event-messages, settings, messages-page, e2e, MSG-01, MSG-02, MSG-03, MSG-04, MSG-05, MSG-06, MSG-07, MSG-08, MSG-09, MSG-11, EVT-01]
requires: [20-01, 20-02, 20-03, 20-04, 20-05, 20-06, 20-07, 20-08, 20-09, 20-10]
provides:
  - TestMessagesEndToEnd (12 subtests) and its toolkit messagesE2E with the messagesE2EClient wrapper
  - the Phase 20 AGENTS.md pass (Testing, Permissions, the Messages page bullet)
affects: [21]
tech-stack:
  added: []
  patterns:
    - "an end-to-end test drives the page only through the buttons the bot drew (settingsMsgButtonWith on the current keyboard) and the deep link, so a renamed code or a moved button fails it"
    - "a thin Bot API wrapper (messagesE2EClient) answers what the shared fake cannot (a member's getChat as a private chat, a media send as a message of its own chat) and passes every request on, so the fake still records it"
    - "a reply that must equal a golden recorded in another chat is compared byte for byte after putting in only the member ID and the command's message ID"
key-files:
  created:
    - fuku/modules/messages_e2e_test.go
  modified:
    - AGENTS.md
decisions:
  - "No production change: every subtest passed against the code of plans 20-01 to 20-10 on its first run, and five mutations of production code (refusal fallback off, Silent's cache drop off, the deletion schedule off, the anonymous mark off, Silent's right lowered to the admin check) each failed the subtests that guard them."
  - "The refused and remove subtests use the real locale files so the default reply is compared with goldenModerationBanReason byte for byte (member ID and command message ID put in); the other ten use the marker locale, so no subtest pays the locale load it does not need."
  - "The plan's behavior list had ten subtests; staff group and no redis were added because the executor brief asked for the Staff Group and Redis-down cases end to end."
  - "With cache.DisableRedisForTest the event message read still goes through the gocache store (a memory store in tests), so the no-redis subtest proves the send path and the Silent write work without the operational Redis client, and its Silent step depends on the write's cache drop (a mutation without it fails)."
metrics:
  duration: ~75 min
  completed: 2026-11-19
actuals:
  tokens: 8400
  tasks: 2
  commits: 2
plan_head_before: bc7456e1f6521b9d316eb6225cd87a128c1226b1
plan_head_after: 9581c803611b927e9bae57b600b1cb7a1327bd40
---

# Phase 20 Plan 11: Custom event messages end to end Summary

The phase's five success criteria are now observed through the running bot: the owner sets a ban message's text, photo and
button from the private Messages page with real presses and typed private messages, an administrator's `/ban` replying to a
member bans first and then posts that message with every placeholder filled and escaped, the Unban row first and the message
queued on the Bot messages timer; Full preview reaches the owner alone; a timed mute fills its duration and end in the group's
zone; a refused custom message is replaced by the golden default after the ban; Silent posts nothing and the silent variants stay
silent; Remove brings back the golden reply; an anonymous admin reads as "an admin"; every change needs `can_change_info`, judged
on every press and typed message; a group menu shows only the note; a Staff Group has no page; the send path works without
Redis; and a Silent press on one replica is seen at the next command on another. No subtest found a defect, so no production
code changed.

## Tasks

| Task | Name | Commit |
|------|------|--------|
| 1 | TestMessagesEndToEnd and the messagesE2E toolkit (AGENTS.md Testing bullet in the same commit) | 10b49aa |
| 2 | Phase gates and the final AGENTS.md pass (hub rows, preview rights in Permissions) | 9581c80 |

## Requirements

| Requirement | Proved by |
|-------------|-----------|
| MSG-01 (one custom message per event and destination; nothing stored sends today's reply) | `TestMessagesEndToEnd/owner_sets_a_ban_message` (text, one photo, a URL button stored from the page and posted), `.../remove` (back to the golden reply), `TestLegacyModerationRequests`, `TestEventMessagesUpsert`. Scope: the `group` destination; the PM destination is Phase 21 |
| MSG-02 (Welcome and event placeholders, empty when no value, user values escaped) | `owner_sets_a_ban_message` (`{first}`, `{reason}`, `{admin}` escaped, `{until}` empty for a permanent ban), `timed` (`{duration}` and `{until}` in the group's zone), `TestRenderEventText` |
| MSG-03 (`{admin}` is "an admin" for an anonymous admin; never the Staff Group) | `anonymous` (proved `/ban`: `eventmsg_admin_anon`, the tapper never named), `TestEventMessageAnonymousAdmin`, `TestEventMessageAfterAnonymousProof`, `settingsPrivacyPhase20Screens`. Automatic actions are Phases 22 to 24 |
| MSG-04 (a refused message falls back to the default; the punishment is never delayed or skipped) | `refused` (method order ban, refused photo, default; the default equals `goldenModerationBanReason` and is queued), `TestEventMessageFallback` |
| MSG-05 (private-only Messages page by family with states; group menu shows the note) | `owner_sets_a_ban_message` (hub reads Custom for ban, Default for the rest), `group menu` (Main reads In private, the cell draws the note alone), `staff group`, `TestSettingsMessagesOpens`, `TestSettingsMessagesGroupMenu` |
| MSG-06 (the editor: Set and Remove by typed messages, limits, Preview and Full preview) | `owner_sets_a_ban_message` (three typed parts), `remove`, `previews` (Full preview through `composeEventMessage`, to the owner only), `TestSettingsMessagesTyped`, `TestSettingsMessagesPreview` |
| MSG-07 (Silent: the bot acts and posts nothing) | `silent` (`/kick` removes and posts nothing), `no redis`, `two replicas`, `TestSettingsMessagesSilent` |
| MSG-08 (viewing: live admin check and owner gate; every change: `can_change_info` live on every press and typed message) | `rights` (screen and Full preview work, Set, Remove and Silent are the `settings_no_change_info` alert with no edit, a demoted sender's typed message stores nothing), `TestSettingsAuthorityMessagesEdit`, `TestSettingsAuthorityMessagesSilent` |
| MSG-09 (Bot messages timers apply to custom messages) | `owner_sets_a_ban_message` (the custom photo queued `bot:punish_manual` at its date + 600 s), `timed`, `refused`, `remove`, `TestBotManualPunishmentReplies` |
| MSG-11 (every existing reply byte-identical with nothing stored) | `TestLegacyModerationRequests` (one commit, 31e7e18, unchanged since), `remove` and `refused` (the default sent through the dispatcher equals the golden) |
| EVT-01 (seven manual events; silent and delete-first variants unchanged) | `owner_sets_a_ban_message`, `timed`, `silent` (`/sban`, `/skick` silent), `TestEventMessageSites`, `TestEventMessageSilentVariants` |

## Phase gates

| Gate | Result | Duration |
|------|--------|----------|
| `go test -tags testtools -race -count=1 -run '^(TestMessagesEndToEnd\|TestLegacyModerationRequests)$' -v ./fuku/modules` | PASS, 12 + 17 subtests, no SKIP (TestMessagesEndToEnd 7.0 s) | 9.5 s package |
| golangci-lint v2.13.1 (`go install` with go1.26.11, `CGO_ENABLED=0`), `run --new-from-rev=origin/main ./...` with the repo config (testtools tag) | 0 issues | 71 s |
| the same with `--build-tags=notesttools` (production files alone, `unused`) | 0 issues | 32 s |
| the same with `--default=none --enable dupl` (CI's duplicate step) | 0 issues | 6 s |
| `make check-translations` | all present | 1 s |
| `make check-docs` | no drift | 1 s |
| `make test-shard-check` | lists equal | 27 s |
| `CGO_ENABLED=0 go build ./...` | ok | 2 s |
| `go test -tags testtools -race -count=1 ./fuku/db/... ./fuku/i18n/... ./fuku/utils/...` | ok | 78 s |
| `go test ./fuku/db/eventmsg ./fuku/db/cache`; `TestChatEventMessagesMigrationStatements`, `TestEventMessagesStayOutOfBackup`; `-run '^TestSettings' ./fuku/i18n` | ok | 15 s |
| `go test -tags testtools -race -count=1 -timeout 45m ./fuku/modules` (the whole package, once) | ok, exit 0 | 2098 s (35 min; it ran beside the lint and the other gates, so the machine was shared) |
| `git diff --exit-code c491b48 -- go.mod go.sum main.go` | no change | - |
| `ls migrations \| grep -c '^2026111912'` | 1 | - |
| `//nolint:unused // first production caller arrives in plan 20-NN` comments | none left in the tree | - |

## Mutation checks (all reverted)

| Mutation | Failed subtests |
|----------|-----------------|
| `replyEvent` never treats a send error as a refusal | `refused` |
| `eventmsg.SetSilent` without its `DeleteCache` | `no redis`, `two replicas` |
| `replyEvent` without `scheduleBotMessage` | `owner sets a ban message`, `timed` |
| `markAnonymousActor` sets no mark | `anonymous` |
| the Silent action's right lowered to `settingsRightAdmin` | `rights` |

## Deviations from Plan

### Auto-fixed Issues

None in production code.

**1. Two subtests added**
- `staff group` and `no redis` beside the ten subtests of the plan, as the executor brief asked for those cases end to end.

**2. Commit split**
- The plan named one commit with the subject `test(20-11): prove custom event messages end to end`; that subject carries the test and its
  AGENTS.md Testing sentence (same-commit rule), and the AGENTS.md corrections of the final pass are their own `docs(20-11)` commit.

**3. Lint ran with a fetched golangci-lint**
- `make lint` cannot run with the installed binary; golangci-lint v2.13.1 (CI's version) was installed with `go install` under Go 1.26.11
  into the scratchpad and run three ways (above). It ran with `CGO_ENABLED=0`, which CI does not set.

## Known Stubs

None.

## Threat Flags

None: T-20-34 (`rights`), T-20-35 (`refused`, method order) and T-20-36 (`two replicas`) are mitigated as planned.

## Not verified

- No PostgreSQL run (`make test-postgres-integrity`): nothing of this plan rests on a constraint, and `TestEventMessagesUpsert` and
  `TestChatEventMessagesMigrationDefaults` are already in its list.
- The full `make test` with coverage and its CI shards is left to the phase verifier; the plain full `fuku/modules` run is above.
- `skipLocal` for `event_messages:` cannot be proved by the two-replica subtest (both replicas share one process and its local layer);
  the drop of the Redis value by every write is what it proves.

## Self-Check: PASSED

- FOUND: fuku/modules/messages_e2e_test.go
- FOUND commits: 10b49aa, 9581c80, both ancestors of HEAD
