---
phase: 10-pm-settings-hub-and-owner-gate
plan: 07
subsystem: permissions
tags: [admin-cache, observer, roster, chat-admin-index, telegram]
requires:
  - phase: 10-pm-settings-hub-and-owner-gate
    provides: adminindex.ReplaceChat, ClearChat and ListChatFresh (10-01), the settings test fixtures newOwner, listAdmin, setDeletedCreator (10-04), settingsCreatorGone and isTelegramClientError (10-05)
provides:
  - cache.SetAdminListObserver, AdminListEvent and the three AdminListKind facts (loaded, bot not admin, bot gone)
  - The AdminIndex module (priority 31) that installs the observer and keeps chat_admin_index current from every admin list load
  - adminCacheAutoUpdate clearing a group's rows when the bot is taken to left or kicked
  - refreshChatAdminIndex, the lean one-group refresh the sweep and the group /settings repair path (plan 10-08) reuse
affects: [10-08, 10-09]

status: complete
actuals:
  tokens: 10100
  tasks: 2
  commits: 2

commits: 2
plan_head_before: f544715479d7e69d8f7a404e00ad706b8ba9d27e
plan_head_after: b65fc6b22e35934cce2822579700e319162ed80f

tech-stack:
  added: []
  patterns:
    - "An observer hook in a leaf package (atomic.Pointer, recover around the call) lets the cache package report facts without importing the database layer"
    - "Definitive facts only: a roster changes on a fresh complete list, a non-admin bot or a 4xx bot lookup, never on an error, timeout, 429 or empty list"

key-files:
  created:
    - fuku/modules/admin_index_feed.go
    - fuku/modules/admin_index_feed_test.go
    - fuku/utils/cache/admin_list_observer_test.go
  modified:
    - fuku/utils/cache/adminCache.go
    - fuku/modules/bot_updates.go

key-decisions:
  - "The observer runs synchronously under the chat's admin-cache singleflight with a 3 s write bound (adminIndexWriteTimeout), so one chat's writes stay in order and no goroutine or shutdown drain is added"
  - "Each of the three events is guarded by the load's generation at the call site, so a load overtaken by an invalidation reports nothing"
  - "adminIndexUnchanged is the zero value of the refresh outcome, so an uninitialised outcome never claims a change"
  - "A refused bot lookup in refreshChatAdminIndex clears the rows and still returns the Telegram error, so a caller can log the reason"

requirements-completed: []

coverage:
  - id: D1
    description: "Every successful admin-list load stores the group's creator and human administrators; the bot is never stored and a deleted-account creator is not stored"
    requirement: MENU-10
    verification:
      - kind: unit
        ref: "fuku/modules/admin_index_feed_test.go#TestAdminIndexFeed"
        status: pass
      - kind: unit
        ref: "fuku/modules/admin_index_feed_test.go#TestAdminIndexEntries"
        status: pass
    human_judgment: false
  - id: D2
    description: "The cache package reports exactly one event per definitive fact and none for a server error, 429, transport error, empty list, exhausted retries or a load overtaken by an invalidation; a panicking observer does not change the result"
    requirement: MENU-10
    verification:
      - kind: unit
        ref: "fuku/utils/cache/admin_list_observer_test.go#TestAdminListObserver"
        status: pass
    human_judgment: false
  - id: D3
    description: "/admincache by an administrator refreshes the rows, a failing roster write is only logged, and with a non-admin bot nothing is loaded and the rows are left"
    requirement: MENU-10
    verification:
      - kind: unit
        ref: "fuku/modules/admin_index_feed_test.go#TestAdminIndexAdmincache"
        status: pass
    human_judgment: false
  - id: D4
    description: "A failed or ambiguous Telegram answer never deletes rows"
    requirement: MENU-10
    verification:
      - kind: unit
        ref: "fuku/modules/admin_index_feed_test.go#TestAdminIndexKeepsRowsOnFailure"
        status: pass
    human_judgment: false
  - id: D5
    description: "The bot leaving, being kicked or being demoted clears the group's rows and no other group's; a promotion adds a row and a demotion removes it"
    requirement: MENU-10
    verification:
      - kind: unit
        ref: "fuku/modules/admin_index_feed_test.go#TestAdminIndexBotRemoval"
        status: pass
      - kind: unit
        ref: "fuku/modules/admin_index_feed_test.go#TestAdminIndexPromotion"
        status: pass
    human_judgment: false
  - id: D6
    description: "refreshChatAdminIndex replaces, clears or leaves rows only on definitive facts, with at most two Telegram calls, and keeps a 429's retry_after"
    requirement: MENU-10
    verification:
      - kind: unit
        ref: "fuku/modules/admin_index_feed_test.go#TestAdminIndexRefresh"
        status: pass
    human_judgment: false
  - id: D7
    description: "The private list that reads these rows (and tells the user to run /admincache or /settings for a missing group) is not built yet; MENU-10 is delivered only with plans 10-08 and 10-09"
    requirement: MENU-10
    verification: []
    human_judgment: true
    rationale: "This plan delivers the feed. The list, the sweep and the repair path that complete MENU-10 belong to later plans."
---

# Phase 10 Plan 07: Admin roster feed Summary

**An observer hook in the admin cache loader, installed by a new AdminIndex module, keeps `chat_admin_index` current from every admin list the bot loads, and `adminCacheAutoUpdate` clears it when the bot leaves; `refreshChatAdminIndex` is the one-group refresh for the sweep.**

## Accomplishments

- **`fuku/utils/cache/adminCache.go`**: `AdminListKind` (`AdminListLoaded`, `AdminListBotNotAdmin`, `AdminListBotGone`), `AdminListEvent`, `SetAdminListObserver` (nil clears), `notifyAdminList` (recovers a panicking observer and logs it) and `isBotLookupClientError`. Three call sites in `loadAdminCacheFromTelegram`, each guarded by the load's generation: a refused bot lookup (4xx except 429), a bot that is not an administrator, and the stored non-empty list. The retry-failure and empty-list returns report nothing. The package still imports nothing from `fuku/db`.
- **`fuku/modules/admin_index_feed.go`**: `LoadAdminIndex` (installs the observer, no handler, no database access), `adminIndexObserve` (recover, 3 s context, `ReplaceChat` or `ClearChat`, a failed write is only logged), `adminIndexEntries` (creator and human administrators; bots and a creator `settingsCreatorGone` recognises are dropped) and `refreshChatAdminIndex` with `adminIndexRefreshOutcome`.
- **`fuku/modules/bot_updates.go`**: `adminCacheAutoUpdate` clears the group's rows when a my_chat_member update takes the bot to left or kicked, before the unchanged invalidate and reload.
- **Tests**: `TestAdminListObserver` (nine cases in the cache package), and in `fuku/modules` `TestAdminIndexFeed`, `TestAdminIndexEntries`, `TestAdminIndexAdmincache`, `TestAdminIndexKeepsRowsOnFailure`, `TestAdminIndexBotRemoval`, `TestAdminIndexPromotion`, `TestAdminIndexRefresh`.
- **No AGENTS.md edit**, as the plan states: no handler group, namespace, key family, table or drain was added and no existing rule changed. The feed's rules are written with the sweep in plan 10-08.

## Task Commits

1. Task 1 (tracer): `/admincache` and every admin list load write the roster - `09df474`
2. Task 2 (TDD): bot leaving clears the rows, one-group refresh - `b65fc6b`

Tracer gate: the tracer `<verify>` is automated only (`end-of-phase` mode), so it was re-run end to end before Task 2 and passed.

TDD note: the RED state of Task 2 was observed (the refresh tests could not compile without `refreshChatAdminIndex`, and with that function added `TestAdminIndexBotRemoval` left and kicked failed on the old `adminCacheAutoUpdate`), then the handler clear turned them green. The tests and implementation went into one commit rather than a separate `test:` commit, because a commit whose test package does not compile would break every later bisect step.

## Verification Results

- `go test -tags testtools -race -count=1 -run '^TestAdminListObserver$' ./fuku/utils/cache` - PASS (4.0 s)
- `go test -tags testtools -race -count=1 -run '^(TestAdminIndexFeed|TestAdminIndexAdmincache)$' ./fuku/modules` - PASS
- `go test -tags testtools -race -count=1 -run '^(TestAdminIndexKeepsRowsOnFailure|TestAdminIndexBotRemoval|TestAdminIndexPromotion|TestAdminIndexRefresh)$' ./fuku/modules` - PASS
- `go test -tags testtools -race -count=1 -run '^(TestAdminCache|TestBotJoined|TestAdminIndex)' ./fuku/modules` - `ok 10.7 s`
- `go test -tags testtools -race -count=1 ./fuku/utils/cache` - `ok 4.1 s` (the existing admin cache tests are unchanged)
- `go test -tags testtools -race -count=1 -run '^(TestAdmin|TestAdminIndex|TestSettings|TestBotJoined|TestLoadSettings)' ./fuku/modules` - `ok 94.5 s`
- Registry tests (`Registry|LegacyModule|TestModules`) - PASS; `make check-docs` - no drift.
- Acceptance greps: `go list -deps ./fuku/utils/cache | grep -c 'fuku_robot/fuku/db'` 0; `notifyAdminList(` in adminCache.go 4; `RegisterLegacyModule("AdminIndex", 31, LoadAdminIndex)` 1; `adminindex.ClearChat` in bot_updates.go 1; `func refreshChatAdminIndex(` 1; `time.Sleep` in admin_index_feed.go 0.
- `CGO_ENABLED=0 go build ./...` passes; `go vet -tags testtools ./fuku/modules ./fuku/utils/cache` is clean; `git diff --exit-code 6ef6ad9 -- go.mod go.sum` is clean.
- `make lint` and the full `make test` were not run (the installed golangci-lint refuses the repository's Go 1.26.0 target, as in earlier plans of this phase).

## Deviations from Plan

None - plan executed exactly as written.

Small choices inside the plan's freedom: `isBotLookupClientError` is a local copy of the classifier in the cache package (it cannot import `fuku/modules`); the failing-write test drops the `chat_admin_index` table and recreates it in `t.Cleanup` instead of adding a test seam to the repository; `TestAdminIndexAdmincache` also covers the plan's "non-admin bot loads nothing" must-have.

## Requirements

`MENU-10` is **left Pending** in `.planning/REQUIREMENTS.md` (checkbox and traceability row): this plan delivers the feed that builds the bot's record, but the private list that reads it and the "run `/admincache` or `/settings`" hint for an empty list land with plans 10-08 and 10-09. `requirements.mark-complete` was not run.

## Known Stubs

None.

## Threat Flags

None. The surface (a roster write from the admin list load path) is the one the plan's T-10-20 and T-10-21 cover; T-10-22 stays accepted, and the roster is never used to authorize anything.

## Self-Check: PASSED

- Created files present: `fuku/modules/admin_index_feed.go`, `fuku/modules/admin_index_feed_test.go`, `fuku/utils/cache/admin_list_observer_test.go`.
- Commits `09df474` and `b65fc6b` are ancestors of HEAD; the measured count from `f544715` is 2.
- Task acceptance criteria and the plan-level verification commands re-run and passing (except the lint and full `make test` runs noted above).
