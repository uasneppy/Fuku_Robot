---
phase: 14-profile-photo-and-channel-rules
plan: 08
subsystem: member-requirements
tags: [notice, i-fixed-it, lookup-rules, approvals, photo-rule, channel-rule]

requires:
  - phase: 14-profile-photo-and-channel-rules
    provides: plans 14-03 to 14-07 (the lookup layer with Fresh and Classify, the seven-row rule table, the executor and watcher lookup paths, the reworded locale keys and the new keys mreq_remedy_photo and mreq_fixed_still_broken_many)
provides:
  - the photo remedy tail of the warn, kick and mute notices (memberReqNoticeHasRemedy, inside memberReqNoticeText)
  - memberReqStillBrokenText (list answer for one to four rules, count answer from five)
  - step 4b of "I fixed it" (approvals through the cached memberReqApprovedLookup seam, after the live marker check)
  - memberReqFixedJudge, step 5 with fresh lookups under memberReqFixedLookupBudget (5 s), broken first, unknown changes nothing
  - named proofs for each branch, including Phase 13's not-ours table replayed with the lookup rules on
affects: [14-09, 14-10, 14-11, 14-12]

plan_head_before: 110a62077b291ba426c590690932236f76fbd343
plan_head_after: 8457b56fc591158fa2c425d2e1a38cbe5377a9f8
actuals:
  tokens: 28000
  tasks: 3
  commits: 6

tech-stack:
  added: []
  patterns:
    - "A press judges in a fixed order of cheapness and authority: presser, hold, live marker, approvals, settings, fresh lookups, permissions; each step refuses before the next, and no lookup is made for a mute the requirements did not place"
    - "A definite broken rule is the answer even when another lookup was unanswerable; an unanswered lookup alone changes nothing"

key-files:
  created:
    - fuku/modules/memberreq_notice_lookup_test.go
    - fuku/modules/memberreq_fixed_lookup_test.go
  modified:
    - fuku/modules/memberreq_notice.go
    - fuku/modules/memberreq_fixed.go
    - fuku/modules/memberreq_screens_test.go
    - AGENTS.md

key-decisions:
  - "Step 5 asks the lookups even when a name rule is already broken, so the still-broken answer names every broken rule at once instead of making the member fix one rule per tap"
  - "The judgement of step 5 is its own function (memberReqFixedJudge) returning the refusal or nil, which keeps memberReqFixedRun under the cyclomatic limit and lets the 4b branch skip it whole"
  - "TestMemberReqFixedLookups carries the unanswerable, broken-first, slow and real-locale cases as subtests, so the plan's exact-name verify command runs all of them"

requirements-completed: [MREQ-11]

coverage:
  - id: D1
    description: "the warn, kick and mute notices end with mreq_remedy_photo after one space while the photo rule is among the broken rules; the ban and the three warn-limit notices never carry it, and none carries it when the photo rule is not broken; the mute notice keeps its one button and the name is spliced after the tail"
    requirement: "MREQ-11"
    verification:
      - kind: unit
        ref: "TestMemberReqNoticePhotoRemedy"
        status: pass
  - id: D2
    description: "one to four broken rules answer mreq_fixed_still_broken naming exactly them in rule order, five or more answer mreq_fixed_still_broken_many with the count; with the four longest phrases of each language the list answer is at most 160 code points in English and 200 elsewhere, the count answer at most 160 in all 8 languages"
    requirement: "MREQ-11"
    verification:
      - kind: unit
        ref: "TestMemberReqStillBrokenText, TestMemberReqScreensFitEveryLocale"
        status: pass
  - id: D3
    description: "a member muted for no photo who added one is freed by a fresh lookup that ignores a stale cached positive and refreshes it; one whose photo is still missing is told so and the stale positive is deleted"
    requirement: "MREQ-11"
    verification:
      - kind: unit
        ref: "TestMemberReqFixedLookups"
        status: pass
  - id: D4
    description: "an approved member holding a name, photo, channel or join mute is freed with no settings read and no lookup, after the live marker check; an approvals error changes nothing; a member who is not approved goes on to settings then lookups; another member and an admin are refused before the approvals read"
    requirement: "MREQ-10 (contribution only)"
    verification:
      - kind: unit
        ref: "TestMemberReqFixedApproved"
        status: pass
  - id: D5
    description: "a definite broken rule beats an unanswerable lookup; with nothing broken every unanswerable photo and channel shape (plain error, 400, 403, 429 past the retry, 500, null body, shed, timeout, shutdown root, unrecognised status, bot standing unreadable) answers mreq_fixed_check_failed with no restrict, the hold untouched and the press inside 10 s"
    requirement: "MREQ-10 (contribution only)"
    verification:
      - kind: unit
        ref: "TestMemberReqFixedLookups"
        status: pass
  - id: D6
    description: "a channel lookup that cannot be answered frees the member when the bot is definitely no channel administrator (member, left, kicked, restricted, 400, 403) or the group has no linked channel, and answers mreq_fixed_check_failed for a 429, a 502, a transport error, a timeout, a shed or an administrator bot; the bot-status request goes through the lookup pacer and takes no slot of the notice pacer; the enforcement job with the same lookup punishes nobody"
    requirement: "MREQ-12 (contribution only)"
    verification:
      - kind: unit
        ref: "TestMemberReqChannelInert"
        status: pass
  - id: D7
    description: "with the photo and channel rules on, every mute that is not the requirements' (an admin's, /tmute, staff, captcha, antiflood, warn-limit, markers off by 3 s, an unmuted member, a member who left) is answered mreq_fixed_not_ours twice with no restrict, no edit, no approvals read, no settings read and no lookup request"
    requirement: "MREQ-11"
    verification:
      - kind: unit
        ref: "TestMemberReqFixedNeverLiftsWithLookupRules, TestMemberReqFixedNeverLifts"
        status: pass
  - id: D8
    description: "every notice (64-rune name, all seven phrases, the remedy tail) is at most 520 visible code points in all 8 languages and no notice or answer names the chat, the Staff Group, the admin who set a rule or the linked channel"
    requirement: "MREQ-11"
    verification:
      - kind: unit
        ref: "TestMemberReqScreensFitEveryLocale"
        status: pass

duration: 95min
completed: 2026-11-08
status: complete
---

# Phase 14 Plan 08: The photo remedy and the fresh-lookup "I fixed it" Summary

**A member muted for no visible profile photo is now told the real remedy in the notice, can add a photo (or subscribe) and tap "I fixed it" to be freed after fresh live lookups that cannot be fooled by a cached positive, an approved member is freed at the tap, a member still missing something is told exactly what, and nothing frees or keeps a member on an unanswered lookup.**

## Accomplishments

- `fuku/modules/memberreq_notice.go`: `memberReqNoticeHasRemedy` (the plain warn, kick and mute templates only, and only when the table row with code `ph` is in the broken mask) and the tail in `memberReqNoticeText`, appended after translation and before the name is spliced; `memberReqStillBrokenText` and `memberReqStillBrokenListMax` (4): one to four rules use the list key, five or more the count key.
- `fuku/modules/memberreq_fixed.go`: `memberReqFixedLookupBudget` (5 s, a variable), step 4b (`memberReqApprovedLookup` after the live marker check: error is `mreq_fixed_check_failed`, approved skips the judgement) and `memberReqFixedJudge` (step 5: settings read, name rules on the live record, `memberReqLookupBroken` with `memberReqLookupMode{Fresh: true, Classify: true}` under the budget, broken first, unknown alone is `mreq_fixed_check_failed`). Everything after step 5 is unchanged.
- Tests: `memberreq_notice_lookup_test.go` (`TestMemberReqNoticePhotoRemedy`, `TestMemberReqStillBrokenText`), `memberreq_fixed_lookup_test.go` (`TestMemberReqFixedLookups` with its subtests, `TestMemberReqFixedApproved`, `TestMemberReqChannelInert`, `TestMemberReqFixedNeverLiftsWithLookupRules`) and `memberreq_screens_test.go` extended (the count-aware answers, the four longest phrases, the remedy tail per action, no linked channel in any text).
- AGENTS.md: the `mreq|v1` bullet's order gains 4b and the new step 5 (fresh lookups, budget, broken first, the channel inert classification); the Permissions "I fixed it" bullet says an approved member is freed at the tap and the remedy "ask an admin to approve you" is true for a mute; the notice sentence records the remedy tail and the 520 cap.

## Task Commits

1. `4707f8f` test(14-08): the photo remedy in the notice and a fresh photo check at the tap (tracer RED: the package's tests did not compile, `memberReqStillBrokenText` did not exist)
2. `aa6382e` feat(14-08): tell the member the photo remedy and re-check the photo at the tap (tracer GREEN)
3. `fc870e4` test(14-08): approvals at the tap, broken first and the inert channel (RED for the approved subtests)
4. `d4ecd95` feat(14-08): free an approved member at the tap and never guess on an unknown
5. `1c7bfaf` test(14-08): the button never lifts another mute with the lookup rules on
6. `8457b56` docs(14-08): describe the approvals step, the fresh lookups and the photo remedy

## Which subtests were red

- Task 1 (`4707f8f`): the whole package failed to compile (`undefined: memberReqStillBrokenText`), so every test was red by build error; after `aa6382e` the three Task 1 tests passed at once.
- Task 2 (`fc870e4`): six `TestMemberReqFixedApproved` subtests failed (the three approved-member frees, the join welcome, the approvals error, and "not approved goes on to step 5" whose order needs the approvals read first). The broken-first, unanswerable (all 28 shapes), slow, real-locale and `TestMemberReqChannelInert` subtests, and the rest of `TestMemberReqFixedApproved`, already passed against the Task 1 code, because the lookup layer already classifies inert and unknown and step 5 already answers broken first.
- Task 3: `TestMemberReqFixedNeverLiftsWithLookupRules` and the screens extension passed on first run (the guarantee they prove comes from the order established in Task 2); they are regression locks.

## Verification (real results)

- Task 1 verify (`TestMemberReqNoticePhotoRemedy|TestMemberReqStillBrokenText|TestMemberReqFixedLookups`): ok. Task 2 verify (`TestMemberReqFixedApproved|TestMemberReqFixedLookups|TestMemberReqChannelInert`): ok. Task 3 verify, first command (`TestMemberReqFixedNeverLiftsWithLookupRules|TestMemberReqScreensFitEveryLocale|TestMemberReqNoticePhotoRemedy|TestMemberReqStillBrokenText`): ok, and the plan's combined verification run of all seven new and extended tests plus `TestMemberReqFixedApproved|Lookups|ChannelInert`: `ok 34.459s`.
- Task 3 second command (the 14 named Phase 13 button and notice tests, unchanged files for `TestMemberReqFixedUnmutes` and `TestMemberReqFixedRefusals`): `ok 20.138s`.
- `go test -tags testtools -race -count=1 ./fuku/i18n`: `ok 23.672s`.
- Family run `-run '^(TestMemberReq|TestApplyWarn|TestRaid|TestSettingsMreq|TestSettingsStaffPrivacy)' ./fuku/modules` (SQLite): `ok 320.678s` (all earlier plans' lookup, watcher, held, join and executor tests included).
- `CGO_ENABLED=0 go build ./...` ok; `go vet -tags testtools ./fuku/modules` clean; `gofmt -l` on the six files I touched: nothing (the plain `gofmt -l fuku/modules/` still lists the seven files that were already unformatted before this plan).
- Lint stand-in `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827`, exit code read directly: 0 ("no dupl, gocyclo or godox findings on lines added since 8c0c827").
- `git diff --exit-code 8c0c827 -- go.mod go.sum`: clean.
- Acceptance greps: `func memberReqStillBrokenText(` 1 and `mreq_remedy_photo` 2 in `memberreq_notice.go`; `memberReqFixedLookupBudget` 4 in `memberreq_fixed.go`; `memberReqApprovedLookup(` 1 and `memberReqLookupMode{Fresh: true, Classify: true}` 1 in `memberreq_fixed.go`; `memberReqApprovedLookup` 1 and `mreq_fixed_still_broken_many` 1 in AGENTS.md; `memberReqStillBrokenText` in `memberreq_screens_test.go` at least 1.
- Not run: `make lint` and gosec (cannot run here), `make test` (plan 14-12's gate), the settings family (no settings code changed), PostgreSQL (nothing here rests on it; the harness does not run on PostgreSQL here).

## Deviations from Plan

**1. [Test layout] The unanswerable, broken-first, slow and real-locale cases are subtests of `TestMemberReqFixedLookups`** (called as `mreqFixedLookups*` helpers), not separate top-level tests, because the plan's verify command matches the exact name `TestMemberReqFixedLookups` and a sibling would not have run under it.

**2. [Scope, none needed] `TestMemberReqFixedUnmutes` and `TestMemberReqFixedRefusals` needed no edit.** Research Conflict C7 said they might; they approve nobody and passed unchanged, so no Phase 13 test was edited.

**3. [Test detail] The channel mute tests expect `defaultUnmutePermissions()`, not the group's snapshot.** A group linked to a channel makes the lookup fake answer the group's own `getChat` with no `permissions`, so `resolveUnmutePermissionsWithLockdown` returns the code's defaults; the photo and name-rule tests keep `lockdownSnapshotPermissions`. The trace helper ignores the group's own `getChat` (the unmute's permissions read) and the approved test asserts exactly one such call, so a linked-channel lookup would still be caught.

**4. [Plan wording] The plan names step 5's ordering "names, then lookups" but the code asks the lookups even when a name rule is already broken** (key decision above), so one answer lists everything that is still broken. This costs up to four paced calls for a member who is surely not freed; the 5 s budget bounds it.

## Known Stubs

None.

## Threat Flags

None. No new endpoint, auth path, schema, Redis key family or goroutine: the press reads the existing approvals and lookup layers, and T-14-30 to T-14-34 are covered by `TestMemberReqFixedNeverLiftsWithLookupRules` and `TestMemberReqFixedNeverLifts` (T-14-30), `TestMemberReqFixedLookups` (T-14-31, T-14-32), `TestMemberReqChannelInert` (T-14-32), `TestMemberReqScreensFitEveryLocale` (T-14-33, T-14-34); T-14-SC by the clean go.mod/go.sum diff.

## Known limits and new concerns

- **A fresh lookup of the press can join a non-fresh flight.** The photo, subscription, linked-channel and bot-status flights are keyed by user, member and channel only (`p:<user>`, `s:<channel>:<user>`, `l:<chat>`, `b:<channel>`), not by `Fresh`. A press that arrives while a watcher or held-release flight of the same key is running with `Fresh` false would share that flight's result, which may have been served from a stale positive read inside the flight. The window is the length of one flight (milliseconds) and only a stale positive (30 minutes, or 2 minutes for the bot status) can be served, but it is a gap in "bypasses both caches". Not fixed: it is in the lookup layer of plan 14-03 (adding the mode to the flight key is a one-line change, owner decision for 14-09 or 14-12).
- A member told "still broken" because of the cached approvals list on another replica can wait up to `CACHE_LOCAL_TTL` (10 s) after an admin approves them, as the plan's planner assumption states (the cached seam is used on purpose).
- A requirement mute placed while the bot could read the channel, with every later lookup unknown and no admin acting, keeps the 330-day marker mute until `/unmute` or a rejoin (UI-SPEC P14-A9, accepted).
- The "RED" commit of Task 1 leaves the package's tests uncompilable until `aa6382e`; it is the first of two consecutive commits.

## Self-Check: PASSED

The created and modified files exist, the six commits (`4707f8f`, `aa6382e`, `fc870e4`, `d4ecd95`, `1c7bfaf`, `8457b56`) are ancestors of HEAD, and `git rev-list --count 110a620..HEAD` printed 6 before this summary.
