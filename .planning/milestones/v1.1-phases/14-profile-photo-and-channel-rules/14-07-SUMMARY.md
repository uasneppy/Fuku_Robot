---
phase: 14-profile-photo-and-channel-rules
plan: 07
subsystem: member-requirements
tags: [watcher, lookup-rules, held-release, runner, photo-rule, channel-rule]

requires:
  - phase: 14-profile-photo-and-channel-rules
    provides: plans 14-03 to 14-06 (the lookup layer and its cache keys, the rule table, the lookup phase of the executor)
provides:
  - the lookup branch of the group -2 watcher: memberReqLookupCached (one pipelined Redis read of the pass key, the photo key, the linked-channel key and, only when a channel is linked, the subscription key), memberReqWatchClean, memberReqStartJob
  - the D-25 skip (memberReqMissExempt): no job on a lookup miss for a cached administrator, an approved member or a failed approvals read
  - memberReqHeldReleased (memberreq_held.go): the held release (D-21) called before the Phase 13 held path
  - named proofs of the watcher's lookup path, one lookup for 100 messages, the pass key never standing in for a lookup, Redis down, ContinueGroups on every path and the held release
affects: [14-08, 14-09, 14-10, 14-11, 14-12]

plan_head_before: 051f6c5f95da0991642ce656b9d26a3436572a04
plan_head_after: 260c09a448ade4999e0780ff03b6b69ab3210f2f
actuals:
  tokens: 18300
  tasks: 3
  commits: 6

tech-stack:
  added: []
  patterns:
    - "The watcher decides 'clean' only from cached positives and otherwise hands the member to the one runner; it never asks Telegram, never classifies and never writes a verdict"
    - "A job counter for tests without production code: the runner refused (memberReqMaxInFlight 0), one 'was shed' line per start attempt"

key-files:
  created:
    - fuku/modules/memberreq_held.go
    - fuku/modules/memberreq_held_test.go
    - fuku/modules/memberreq_watch_lookup_test.go
  modified:
    - fuku/modules/memberreq_watch.go
    - fuku/modules/memberreq_enforce.go
    - fuku/modules/memberreq_lookup.go
    - AGENTS.md

key-decisions:
  - "A hold with name bits only is released by the same call when the names are clean now (plan text kept such a hold). Without it a name hold of a member who fixed their name could never end in a group with a lookup rule on: the watcher no longer reaches the clean predicate for a member whose lookups are not cached, and a held member's job never asks Telegram, so no positive is ever cached"
  - "The photo and subscription flights read their cache once more inside the flight, so 100 messages of one member make exactly one request even when a caller's cache miss lands just before the previous flight's positive"
  - "The watcher treats lc reading 0 as satisfied (no linked channel, nothing to verify) and a missing, positive or unparsable lc as not satisfied"

requirements-completed: []

coverage:
  - id: D1
    description: "a member with clean names and no photo, or no subscription to the linked channel, sends a message through the real dispatcher and is muted once (one hold with the photo or channel bit, one notice); a second message adds no punishment"
    requirement: "MREQ-11, MREQ-12 (contributions only)"
    verification:
      - kind: unit
        ref: "TestMemberReqWatcherLookupRules"
        status: pass
  - id: D2
    description: "a member with the photo positive cached and the pass stored, a group whose lc reads 0, and a member whose subscription is cached start no job, make no request and write nothing; a satisfied member without a pass runs the Phase 13 clean predicate and stores the pass"
    requirement: "MREQ-10 (contribution only)"
    verification:
      - kind: unit
        ref: "TestMemberReqWatcherLookupRules"
        status: pass
  - id: D3
    description: "a cached administrator, an approved member and a failed approvals read start no job on a miss (D-25), counted by the runner's own shed line; a member the admin cache does not know starts exactly one"
    requirement: "PLAT-06 (contribution only)"
    verification:
      - kind: unit
        ref: "TestMemberReqWatcherLookupRules"
        status: pass
  - id: D4
    description: "Redis down hands a job per message, a lookup that cannot be answered punishes nobody, and a full runner sheds with one ID-only line"
    requirement: "MREQ-10, PLAT-06 (contributions only)"
    verification:
      - kind: unit
        ref: "TestMemberReqWatcherLookupRules"
        status: pass
  - id: D5
    description: "100 messages of one member while the fake holds the first photo request make exactly one getUserProfilePhotos request, no hold, no notice, the photo key exists, shed lines carry IDs only and a later message is clean from the cache"
    requirement: "PLAT-06 (contribution only)"
    verification:
      - kind: unit
        ref: "TestMemberReqWatcherLookupSingleflight"
        status: pass
  - id: D6
    description: "a pass hit with no lookup positive still hands a job; an unknown or inert lookup leaves no verdict under any key naming the member and the next message asks again; a lookup positive or pass of one user, channel or chat is never read for another"
    requirement: "MREQ-10 (contribution only)"
    verification:
      - kind: unit
        ref: "TestMemberReqPassCacheWithLookups"
        status: pass
  - id: D7
    description: "the watcher returns ext.ContinueGroups and the update reaches groups -1 and 3 for a job handed off, a shed job, a clean member from the cache, an exempt member, a Redis failure and a recovered panic"
    requirement: "PLAT-06 (contribution only)"
    verification:
      - kind: unit
        ref: "TestMemberReqWatcherLookupContinuesGroups"
        status: pass
  - id: D8
    description: "a held member whose hold carries lookup bits and whose names are clean is released when the photo now exists (no delete, no punishment, no restrict), and kept, with the message deleted, when it is still missing, unknown or the names break; the lookup is made again at the next message and a missing photo is never cached"
    requirement: "MREQ-10 (contribution only)"
    verification:
      - kind: unit
        ref: "TestMemberReqHeldRelease, TestMemberReqLookupHeldPath"
        status: pass
  - id: D9
    description: "channel-only holds: released when the member is subscribed and when the channel is inert (no linked channel, the bot not an administrator, a 403 for both lookups), kept when the member left a channel the bot administers, on a 500, on both lookups failing and on a timeout"
    requirement: "MREQ-12 (contribution only)"
    verification:
      - kind: unit
        ref: "TestMemberReqHeldReleaseChannel"
        status: pass
  - id: D10
    description: "no branch of the release sends a restrict, ban or unban; a punished mute hold is released only past the 2 s margin, a hold created at the message's time is kept, a join case is never released"
    requirement: "MREQ-10 (contribution only)"
    verification:
      - kind: unit
        ref: "TestMemberReqHeldRelease"
        status: pass

duration: 130min
completed: 2026-11-08
status: complete
---

# Phase 14 Plan 07: The watcher's lookup path and the held release Summary

**The group -2 watcher now checks every message of a clean-named member against the photo and channel rules from cached positives alone (one pipelined Redis round, no Telegram call) and hands a miss to one runner job, skipping known administrators and approved members, and the job releases a held member who fixed the photo, the subscription or the name, without ever punishing or lifting a mute.**

## Accomplishments

- `fuku/modules/memberreq_watch.go`: `memberReqLookupRulesOn`, `memberReqLookupCached` (pass key, photo key and lc in one pipeline, the subscription key in a second read only when lc holds a channel ID; every command read on its own so a miss and an error are told apart), `memberReqCachedChannel`, `memberReqStartJob` (the one place a case is built for the runner, shared by the violator and lookup paths), `memberReqWatchClean` and `memberReqMissExempt`. A group with name rules only keeps its single `GET` exactly as before. The watcher still returns the preset `ext.ContinueGroups`, writes nothing about a lookup and makes no Telegram call.
- `fuku/modules/memberreq_held.go` (new): `memberReqHeldReleased` and `memberReqHeldLookupsClear`. The executor's held branch calls it before `memberReqDeleteHeld`; a released hold returns an empty result (no Held outcome, no delete).
- `fuku/modules/memberreq_lookup.go`: the photo and subscription flights read the cache once more inside the flight (see Deviations 2).
- Tests: `memberreq_watch_lookup_test.go` (the four watcher tests) and `memberreq_held_test.go` (`TestMemberReqLookupHeldPath`, `TestMemberReqHeldRelease`, `TestMemberReqHeldReleaseChannel`, 8 channel shapes).
- AGENTS.md: the watcher bullet (the lookup path, D-25, no verdicts), the pass-key Data bullet (never stands in for a lookup), the runner bullet (a miss is a job like a violator, Redis down costs a job per message by design, the unreadable-channel cost), the executor bullet's held sentence and a new held-release bullet (including D-24's accepted cost). The Testing sentence is left to plan 14-11.

## Task Commits

1. `08b2ff7` test(14-07): the watcher checks the lookup rules at a member's message (RED)
2. `b2adb2d` feat(14-07): the watcher hands a lookup miss to the enforcement runner (GREEN of the tracer)
3. `ad7325f` test(14-07): exempt members, one lookup for 100 messages, the pass key and Redis down
4. `ef1935f` feat(14-07): exempt members start no lookup job on a miss
5. `1e221b4` test(14-07): a held member who fixed the photo is released and one who did not is kept (RED)
6. `260c09a` feat(14-07): release a held member who fixed the photo or the subscription

## Which subtests were red

- Task 1 (`08b2ff7`): the muted-once subtest and the channel-job subtest of `TestMemberReqWatcherLookupRules` failed (no job was started for a clean-named member); the satisfied-from-cache subtests passed already, as expected.
- Task 2 (`ad7325f`): the three D-25 subtests (cached administrator, approved member, failed approvals read) failed on the job-start counter (3, 3 and 1 starts). The approved and failed-approvals shapes could not show red through requests alone, because the job's own exemption makes no request for them, so the tests count start attempts through the runner (`memberReqMaxInFlight` 0, one "was shed" line per attempt). `TestMemberReqWatcherLookupSingleflight`, `TestMemberReqPassCacheWithLookups` and `TestMemberReqWatcherLookupContinuesGroups` passed against the Task 1 code; the singleflight test then failed once with 2 requests after the D-25 change (Deviations 2).
- Task 3 (`1e221b4`, with the enforce call and `memberreq_held.go` removed from the tree for the run): 10 of the subtests failed (every "released" shape and the "asked again" shape); the "kept" shapes and `TestMemberReqLookupHeldPath` passed against the old held path, as expected.

## Verification (real results)

- Task 1 verify (`TestMemberReqWatcherLookupRules`): ok. Task 2 verify (the four watcher tests): ok. Task 3 verify (`TestMemberReqLookupHeldPath|TestMemberReqHeldRelease`, plus my `TestMemberReqHeldReleaseChannel`): ok.
- Task 3 second command (the 12 named Phase 13 watcher, held-path, delete, cap and shutdown tests): `ok 15.066s`.
- Flake loop, final tree: `-count=20` of `TestMemberReqWatcherLookupSingleflight|TestMemberReqWatcherLookupRules|TestMemberReqWatcherLookupContinuesGroups|TestMemberReqPassCacheWithLookups|TestMemberReqLookupHeldPath|TestMemberReqHeldRelease|TestMemberReqHeldReleaseChannel`: `ok 244.420s` (20 of 20). Earlier loop on the Task 2 tree (`Singleflight|Rules`): `ok 92.622s`.
- Family run `-run '^(TestMemberReq|TestShutdown|TestRequirement|TestChatRequirements)' ./fuku/modules ./fuku/db/memberreq` (SQLite): `ok 170.428s` and `ok 1.141s` (includes every earlier lookup test, so the flight change is covered). The lookup family on its own after the flight change: `ok 79.815s`.
- `CGO_ENABLED=0 go build ./...` ok; `go vet -tags testtools ./fuku/modules` clean; `gofmt -l` on the files I touched: nothing (the plain `gofmt -l fuku/modules/` still lists the seven files that were already unformatted before this plan).
- Lint stand-in `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827`, exit code read directly: 0 ("no dupl, gocyclo or godox findings on lines added since 8c0c827").
- `git diff --exit-code 8c0c827 -- go.mod go.sum`: clean.
- Acceptance greps: `memberReqPhotoKey(` and `memberReqLinkedKey(` in `memberreq_watch.go` at least 1 each; `cachedAdminStatus(` in it 2; `func TestMemberReq` in `memberreq_watch_lookup_test.go` 4; `func memberReqHeldReleased(` 1 and `ClearHoldOnClean(` 1 in `memberreq_held.go`; `memberReqHeldReleased(` in `memberreq_enforce.go` 1 (a doc-comment mention aside); `held release` in AGENTS.md 3.
- Not run: `make lint` and gosec (cannot run here), `make test` (plan 14-12's gate), the settings family (no settings code changed), PostgreSQL (nothing here rests on it; the harness does not run on PostgreSQL here).

## Deviations from Plan

**1. [Rule 1 - bug, scope] A hold with name bits only is released when the names are clean now.** The plan listed "a hold that has only name bits" among the holds the release keeps. That deadlocks a name hold in any group that also has a lookup rule on: the watcher reaches the Phase 13 clean predicate only for a member whose lookups are cached positive, a held member's job never asks Telegram, so no positive is cached, and the hold of a member who fixed their name survives for ever (their messages deleted when Delete messages is on, a job per message). `memberReqHeldReleased` therefore does not require a lookup bit: a name-only hold with clean names is released through `ClearHoldOnClean` with zero lookups, which is exactly what the watcher's clean path did in Phase 13. A hold whose names still break stays held with no lookup, as the plan's `TestMemberReqLookupHeldPath` says (its case uses broken names); `TestMemberReqHeldRelease` has a subtest for the released name hold. A member released this way who lacks the photo is a new episode at their next message, as for any rule broken after a fix. AGENTS.md says so.

**2. [Rule 1 - bug] Two flights read the cache once more inside the flight (`memberreq_lookup.go`, outside the plan's file list).** `TestMemberReqWatcherLookupSingleflight` failed once with 2 `getUserProfilePhotos` requests: a job that read the cache a moment before the first flight wrote its positive, then found no flight running and started a second one. The photo flight and the subscription flight now check the cache first inside the flight function, which makes "100 messages, one lookup" deterministic (20 of 20 after the change). The linked-channel and bot-status flights are not changed (group-level, not per member).

**3. [Test detail] `mreqSeedHold` was already a helper of plan 13's "I fixed it" tests;** mine is `mreqSeedHeld` in `memberreq_held_test.go`.

**4. [Test detail] No `getChat` assertion in the "lc cached as 0" subtest.** Another handler of the settings environment already makes one `getChat` of the group per message at HEAD, so the count could not prove the watcher's silence; the subtest asserts the job-start counter, zero requests about the member and no hold instead.

**5. [Coverage] The held channel table has no "shed" shape.** Timeout and the 500 shapes are there; a shed lookup is the same `unknown` and is covered across the job and join paths by plan 14-06's `TestMemberReqLookupUnanswerable`.

## Known Stubs

None.

## Threat Flags

None. No new endpoint, auth path, schema, Redis key family or goroutine pool: the watcher reads three existing key families, the release deletes a hold row through the existing predicate, and T-14-26 to T-14-29 are covered by `TestMemberReqWatcherLookupRules`, `TestMemberReqWatcherLookupContinuesGroups`, `TestMemberReqWatcherLookupSingleflight`, `TestMemberReqHeldRelease` (no restrict, ban or unban in any branch) and `TestMemberReqPassCacheWithLookups`; T-14-SC by the clean go.mod/go.sum diff.

## Known limits and new concerns

- **A group linked to a channel the bot cannot read costs a job per message for every non-exempt member.** An inert or unknown channel verdict is never cached (R2, D-24), so the watcher sees the subscription key missing at every message and starts a job, which makes a live `getChatMember` of the group, the paced subscription lookup and, on a not-subscribed answer, the bot-status lookup. It is bounded by the 32-job runner, the lookup pacer and shedding and punishes nobody, and is now written in AGENTS.md, but it is a real steady cost for a misconfigured group; a short negative-inert cache with its own key would remove it and is a design decision for the owner (it would break "nothing but a pass is cached").
- A held member still without the photo or the subscription costs one paced lookup per message (D-24, accepted and documented).
- `TestMemberReqWatcherLookupContinuesGroups` logs the recovered panic's stack at error level in its output; that is the injected panic, not a fault.
- Seven files of `fuku/modules` are not gofmt-clean at HEAD; out of scope, untouched.

## Self-Check: PASSED

The created and modified files exist, the six commits (`08b2ff7`, `b2adb2d`, `ad7325f`, `ef1935f`, `1e221b4`, `260c09a`) are ancestors of HEAD, and `git rev-list --count 051f6c5..HEAD` printed 6 before this summary.
