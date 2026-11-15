---
phase: 14-profile-photo-and-channel-rules
plan: 04
subsystem: member-requirements
tags: [lookup, channel, subscription, classifier, redis, member-requirements]

requires:
  - phase: 14-profile-photo-and-channel-rules
    provides: plan 14-03 (memberReqVerdict, lookup pacer, flight, cache helpers, the fake)
provides:
  - memberReqLinkedChannel, the linked channel read only from getChat(group).LinkedChatId (cache fuku:memberreq:lc:<chat>, 10 minutes)
  - memberReqSubscribed, memberReqChannelRule and the subscription cache fuku:memberreq:sub:<channel>:<user> (30 minutes, pass only)
  - memberReqBotStatus, memberReqClassifyBot and memberReqBotChannelStatus with the positive-only cache fuku:memberreq:ba:<channel> (2 minutes)
  - the D-22 gate (memberReqGateBreach) and memberReqLookupMode.Classify
  - memberReqFlightValue, the generic flight under memberReqFlight
affects: [14-05, 14-06, 14-07, 14-08, 14-09, 14-10]

plan_head_before: fb3cef6af33322ce960712780abbe830c705c75a
plan_head_after: 9be886c3b42826b323cb2a1269008e258c395a81
actuals:
  tokens: 21000
  tasks: 3
  commits: 4

tech-stack:
  added: []
  patterns:
    - "A not-subscribed answer is only a breach candidate: it counts as a breach only after the bot is established as an administrator of the channel (D-22)"
    - "The bot's own getChatMember error is classified by Telegram code alone (cache.IsBotGoneError), never by description"

key-files:
  created: []
  modified:
    - fuku/modules/memberreq_lookup.go
    - fuku/modules/memberreq_lookup_fake_test.go
    - fuku/modules/memberreq_lookup_test.go
    - AGENTS.md

key-decisions:
  - "memberReqFlight is now a thin wrapper over a generic memberReqFlightValue[T] (value plus ok), so the linked channel (a struct) and the bot status (an enum) use the same panic-safe flight as the verdicts; flight keys of the four families differ by prefix"
  - "A positive linked_chat_id reads as none and is not cached, while an absent one (0) is none and is cached as 0: the plan's 'never written on a non-negative ID' contradicts its own 'none as 0' test, so the cache rule is: negative and zero are cached, positive is neither"
  - "memberReqChannelRule applies the Fresh stale-pass delete after the D-22 gate, on a definite breach only, so an inert answer from a non-administrator bot never drops a positive"
  - "Classify is read only after an unknown member verdict; a pass or a gated candidate never makes the extra bot-status call"

requirements-completed: []

coverage:
  - id: D1
    description: "the linked channel comes only from getChat(group).LinkedChatId through the lookup pacer and a cache, never from callback data or the update; an error, null or positive answer is not known or none and is never wrongly cached (T-14-14)"
    requirement: "MREQ-12 (contribution only)"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestMemberReqLinkedChannel$' ./fuku/modules"
        status: pass
  - id: D2
    description: "a subscribed answer passes and is cached; every error, timeout, shed, null body and unrecognised status is unknown, never a breach and never cached (T-14-16)"
    requirement: "MREQ-10 (contribution only)"
    verification:
      - kind: unit
        ref: "TestMemberReqChannelRule"
        status: pass
  - id: D3
    description: "left, kicked and restricted-not-member are a breach only after the bot is an administrator of the channel; a left answer for every member never punishes anybody without one (T-14-13, D-22)"
    requirement: "MREQ-12 (contribution only)"
    verification:
      - kind: unit
        ref: "TestMemberReqChannelBreachNeedsBotAdmin"
        status: pass
  - id: D4
    description: "the classifier maps the bot's own answer by status and code alone: 400/403 inert, 429/shed/5xx/transport/timeout/null/unrecognised unknown, a 500 whose text says chat not found unknown (T-14-15)"
    requirement: "MREQ-10 (contribution only)"
    verification:
      - kind: unit
        ref: "TestMemberReqBotStatusClassifier"
        status: pass
  - id: D5
    description: "the three channel flights make one call for 100 callers; the caches have the stated keys, values and TTLs and hold only positives; mreqLookupCleanup removes every key"
    requirement: "PLAT-06 (contribution only)"
    verification:
      - kind: unit
        ref: "TestMemberReqLookupSingleflight (-race -count=20), TestMemberReqLookupCacheKeys"
        status: pass

duration: 60min
completed: 2026-11-08
status: complete
---

# Phase 14 Plan 04: The channel lookups and the administrator gate Summary

**A linked-channel, subscription and bot-status lookup layer in which a "not subscribed" answer is only a breach candidate and counts as a breach only after the bot is shown to administer the channel, with a code-only classifier of the bot's own answer.**

## Accomplishments

- `fuku/modules/memberreq_lookup.go`:
  - `memberReqLinkedChannel(ctx, b, chatID, mode) (int64, bool)`: cache `fuku:memberreq:lc:<chat>` (decimal channel ID or `0`, 10 minutes) unless Fresh, else flight `l:<chat>` around the paced `getChat(group)`; an error, shed, timeout or null answer is not known and writes nothing, a positive `linked_chat_id` reads as none and is not cached.
  - `memberReqSubscribed(member)`: creator, administrator, member and restricted with `is_member` pass; restricted without `is_member`, left and kicked return `memberReqBreach` as a candidate only; anything else (the empty status of a null or unrecognised answer included) is unknown.
  - `memberReqBotStatus` (zero value unknown, then admin, not-admin), `memberReqClassifyBot(member, err)` (by status, and by `cache.IsBotGoneError` for an error: any 400 or 403 is not-admin, everything else unknown, a shed checked first) and `memberReqBotChannelStatus` (cache `ba` unless Fresh, flight `b:<channel>`, `ba` written only for an administrator, a stale `ba` dropped on a Fresh not-administrator).
  - `memberReqChannelRule(ctx, b, chatID, userID, mode)`: link (unknown or inert), `sub` cache, flight `s:<channel>:<user>` around the paced member lookup, then `memberReqGateBreach` for a candidate (admin: breach, not-admin: inert, unknown: unknown); with `Classify` an unknown member verdict becomes inert only for a definite not-administrator bot. The member lookup uses `memberReqLookupPaced` and its error is never inspected.
  - `memberReqFlightValue[T]`: the generic flight (panic recovery inside, flight context from `memberReqContext()`, caller selects on its own deadline); `memberReqFlight` is now a wrapper. `memberReqCacheSetValue` and `memberReqCacheGet` join the cache helpers.
- Tests and fake: the wrapper fake answers `getChat` with a configured `linked_chat_id` (or null), and `getChatMember` per (chat, user) including the bot's own status, with scripted errors and holds keyed per call; `mreqChannel(t, state, group, botStatus)` links a fresh channel and registers its ID (and the group's) for cleanup.
- AGENTS.md: Data bullets for `lc`, `sub` and `ba`; the flights bullet lists the four families; the Go rules `DoChan` bullet names `memberReqFlightValue`; one Subsystem traps bullet, "Member requirements, the lookups", with the verdict type, both rules, the D-22 gate, the classifier, `Classify` and the per-message cost (D-24).

## Task Commits

1. `74a30f0` test(14-04): the channel lookup resolves the link and judges a member by an administrator bot (RED: `go vet` failed on `undefined: memberReqLinkedChannel`)
2. `6eef191` feat(14-04): judge a channel subscription only as an administrator of the channel (GREEN of Task 1, and all the production code of the plan)
3. `6a3755e` feat(14-04): a channel the bot cannot read is inert, never a breach (tests of Task 2, see deviations)
4. `9be886c` docs(14-04): describe the channel lookups, their caches and the administrator gate (the Task 3 tests and AGENTS.md)

## Verification (real results)

- Task 1: RED confirmed by `go vet -tags testtools ./fuku/modules` (`undefined: memberReqLinkedChannel`) before the test commit; GREEN `go test ... -run '^(TestMemberReqLinkedChannel|TestMemberReqChannelRule)$'` passes, and the Phase 14-03 photo and singleflight tests still pass.
- Task 2: `go test -tags testtools -race -count=1 -run '^(TestMemberReqBotStatusClassifier|TestMemberReqChannelRule|TestMemberReqChannelBreachNeedsBotAdmin|TestMemberReqLinkedChannel)$' ./fuku/modules`: ok. Subtests that were red: none (see deviation 1). Mutation checks on the production file (restored from a copy afterwards, `git status` showed only test files modified): gate returning a breach for a not-administrator bot, 14 failing tests; reading "chat not found" in the classifier, 4 failing; `Classify` ignored, 20 failing; restricted-without-`is_member` passing, 2 failing.
- Task 3: `go test -tags testtools -race -count=1 -run '^(TestMemberReqLookupSingleflight|TestMemberReqLookupCacheKeys|...)$'` ok; `-race -count=20 -run '^TestMemberReqLookupSingleflight$'`: `ok ... 77.791s`, no failure in any run.
- Plan verification: `go test -tags testtools -race -count=1 -run '^(TestMemberReqVerdictZeroValue|TestMemberReqPhotoRule|TestMemberReqLookup|TestMemberReqLinkedChannel|TestMemberReqChannelRule|TestMemberReqChannelBreachNeedsBotAdmin|TestMemberReqBotStatusClassifier)' ./fuku/modules`: ok. `go test -tags testtools -race -count=1 ./fuku/utils/cache`: ok. Phase 13 regression (`TestMemberReqRunnerSheds`, `TestShutdownWaitsForMemberReqJobs`, `TestMemberReqWatcherContinuesGroups`, `TestMemberReqEnforcementLive`): ok (SQLite).
- `CGO_ENABLED=0 go build ./...` ok; `gofmt -l` on the three Go files of the plan: nothing; `go vet -tags testtools ./fuku/modules`: clean; `git diff --exit-code 8c0c827 -- go.mod go.sum`: clean.
- Lint stand-in `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827`: first run exit 1 on one staticcheck finding (ST1008, the fake's `beginChannel` returned an error in the middle of its result list), fixed by returning a `mreqChannelStep` struct; second and third runs exit 0 read directly ("no dupl, gocyclo or godox findings on lines added since 8c0c827"). `make lint` and gosec cannot run in this sandbox.
- Acceptance greps: `func memberReqChannelRule(` 1, `func memberReqLinkedChannel(` 1, `fuku:memberreq:ba:` in the Go file 2, `fuku:memberreq:lc:` 2, `IsBotGoneError` 2, `Classify` 7, `func TestMemberReqChannelBreachNeedsBotAdmin` 1, in AGENTS.md `fuku:memberreq:ba:` 1, `fuku:memberreq:sub:` 1, `Member requirements, the lookups` 1.
- Not run: `make test` (plan 14-12's gate), the full settings family (this plan changes no settings code), PostgreSQL (the layer reads and writes Redis only), the live Telegram spikes S2 and S3 (deferred; the gate makes every outcome of S2 safe by construction).

## Deviations from Plan

**1. [Process] Task 2 had no RED commit.** The code of Task 1 already honoured every Task 2 behaviour, as the plan allowed, so the Task 2 tests passed on first run and one commit holds them (`6a3755e`, subject as the plan names it though it adds only tests). The four mutation checks above stand in for the red state and show the tests detect a missing gate, a wording-based classifier, an ignored `Classify` and a mis-judged restricted member.

**2. [Judgment] The `lc` cache rule.** The plan says both "never written on ... a non-negative ID" and "'none' as 0 ... cached" and tests "no linked_chat_id answers (0, known) and caches '0'". Implemented: zero and negative are cached, a positive `linked_chat_id` (no channel has one) reads as none and is not cached; a cached value that is positive or unparsable is a miss. Recorded in the AGENTS.md bullet.

**3. [Rule 1 - lint] ST1008 in the test fake**, fixed as described under Verification.

**4. [Scope, tests] Extra tests beyond the plan's list:** `TestMemberReqLinkedChannel` also covers a null answer, a positive ID, 403, 429, timeout, shed, cancelled context, Fresh and a junk cached value; `TestMemberReqChannelRule` also covers an unreadable link and "the bot's status is read only for a candidate"; `TestMemberReqChannelBreachNeedsBotAdmin` also covers a Fresh drop of a stale subscription pass.

## Known Stubs

None as placeholder data. **Staged symbols with no production caller yet** (plan 14-05 puts the channel rule in the table, plan 14-06 folds it into the executor, 14-07 and 14-08 use `Classify` and `Fresh`): `memberReqLinkedChannel`, `memberReqSubscribed`, `memberReqBotStatus` and its three values, `memberReqClassifyBot`, `memberReqBotChannelStatus`, `memberReqChannelRule`, `memberReqGateBreach`, `memberReqLookupMode.Classify`, `memberReqSubKey`, `memberReqLinkedKey`, `memberReqBotAdminKey`, `memberReqLinkedTTL`, `memberReqBotAdminTTL`, `memberReqCacheSetValue` and `memberReqCacheGet` (the last two are reached by the linked-channel lookup). Each is called by a test of this plan (`TestMemberReqLinkedChannel`, `TestMemberReqChannelRule`, `TestMemberReqChannelBreachNeedsBotAdmin`, `TestMemberReqBotStatusClassifier`, `TestMemberReqLookupCacheKeys`), and the lint stand-in's unused check passes. `memberReqFlightValue` is used in production by the photo path through `memberReqFlight`.

## Threat Flags

None. No new endpoint, auth path or schema; the lookups only call `getChat` and `getChatMember` through the existing lookup pacer.

## Known limits and new concerns

- `memberReqClassifyBot` treats any Telegram 400 or 403 of the bot's own lookup as "not an administrator" (R3). This errs only toward inert; a 400 whose cause is something else (a malformed request) would silence the channel rule for that channel until the next definite answer. Spike S2 may narrow it.
- The `ba` positive lives 2 minutes, so a demoted bot can still be read as an administrator for up to that long on the non-Fresh path; the gate then still needs the member answer to say left or kicked.
- A caller that misses a cache and enters `DoChan` just after another flight finished can make a second request (the 14-03 known limit); the 100-caller tests hold the call open and do not depend on it.

## Self-Check: PASSED

Files modified exist and the four commits (`74a30f0`, `6eef191`, `6a3755e`, `9be886c`) are ancestors of HEAD; `git rev-list --count fb3cef6..HEAD` printed 4 before this summary.
