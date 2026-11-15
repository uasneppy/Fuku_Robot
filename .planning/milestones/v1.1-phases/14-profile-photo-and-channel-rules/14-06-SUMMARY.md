---
phase: 14-profile-photo-and-channel-rules
plan: 06
subsystem: member-requirements
tags: [executor, lookup-rules, join-entry, shutdown, photo-rule, channel-rule]

requires:
  - phase: 14-profile-photo-and-channel-rules
    provides: plans 14-03 to 14-05 (memberReqPhotoRule, memberReqChannelRule, the lookup pacer and flights, the seven-row rule table, memberReqLookupBroken)
provides:
  - step 6b of the executor in memberReqLiveViolation: the lookup mask OR-ed into the name mask under memberReqLookupJobBudget (20 s) or memberReqLookupJoinBudget (4 s)
  - memberReqLookupBroken's production caller (memberReqLiveViolation), so the photo and channel rules punish
  - arrival hooks on the lookup fake (setOnPhoto, setOnChannel) and a mreqChannel that no longer wipes the group's stored settings at registration
  - named proofs of punishing, sparing, ordering, unanswerable shapes, name-breach independence, CR-01, cap and pause, one punishment across replicas, the join budget and the shutdown path
affects: [14-07, 14-08, 14-09, 14-10]

plan_head_before: 47dda27c4a3ff4b65bf33be5ad02811dffb5d93e
plan_head_after: b7e85d0804575ca3891f6c8293a0538eab3dc69a
actuals:
  tokens: 36000
  tasks: 3
  commits: 4

tech-stack:
  added: []
  patterns:
    - "A lookup verdict is an explicit breach or nothing: the executor folds only breach bits, so every unanswerable shape is 'not broken' and the claim, which follows every lookup, never happens"
    - "The lookup budget bounds only the caller's wait; the shared flight runs on the shutdown root with its own timeout, so one caller's short budget never cancels a lookup others share"

key-files:
  created:
    - fuku/modules/memberreq_lookup_enforce_test.go
  modified:
    - fuku/modules/memberreq_enforce.go
    - fuku/modules/memberreq_lookup_fake_test.go
    - fuku/modules/shutdown_drains_test.go
    - AGENTS.md

key-decisions:
  - "An unknown lookup never blocks an established name breach (research S-02): a member whose names break a rule is punished for the names when the photo or channel lookup is unanswerable, the hold's mask and the notice name only the established rules (a partial episode)"
  - "The lookup phase runs after the live getChatMember and the settings re-read and the names, and before the mute-over-restriction check, the cap and the claim; a paused chat, a held member and an exempt member never reach it"
  - "mreqChannel registers the group only for an after-test Redis cleanup, because a before-test cleanup of the group deleted the requirement settings and greetings the test had stored a moment earlier"

requirements-completed: []

coverage:
  - id: D1
    description: "with the photo rule on, a member with no photo is warned, kicked, banned or muted once with one hold (Rules = bit 32) and one notice naming the photo phrase; a member with a photo is spared and the pass is cached, so a second enforcement makes no request"
    requirement: "MREQ-11 (contribution only)"
    verification:
      - kind: unit
        ref: "TestMemberReqLookupPunishes, TestMemberReqLookupSpares"
        status: pass
  - id: D2
    description: "with the channel rule on and the bot administering the linked channel, a member who left is punished by each of Warn, Kick, Ban and Mute (hold Rules = bit 64); a subscribed member, a bot that does not administer the channel and a group with no linked channel punish nobody"
    requirement: "MREQ-12 (contribution only)"
    verification:
      - kind: unit
        ref: "TestMemberReqLookupPunishes/channel_rule"
        status: pass
  - id: D3
    description: "28 unanswerable shapes (photo: error, 400, 403, 429 past the retry, 500, null body, pacer shed, timeout, root cancelled; channel: getChat and member lookups with the same failures, unrecognised and empty status, the bot's own standing unreadable, shed, root cancelled) punish nobody and leave no hold, notice, warn or write, through the job and through the join entry, which welcomes the joiner"
    requirement: "MREQ-10 (contribution only)"
    verification:
      - kind: unit
        ref: "TestMemberReqLookupUnanswerable"
        status: pass
  - id: D4
    description: "an unanswerable lookup never blocks an established name breach; with both a name and a photo breach the hold carries both bits, the strictest action decides and the notice lists both phrases in rule order"
    requirement: "MREQ-10 (contribution only)"
    verification:
      - kind: unit
        ref: "TestMemberReqLookupDoesNotBlockNameBreach"
        status: pass
  - id: D5
    description: "the lookups keep their place: photo before channel, a rule that is Off asks nothing, an approved member, a pending captcha, a live administrator, the bot and a service ID make no lookup, a held member and a paused chat make none, and at the photo request no hold exists and the cap counter has not counted the member"
    requirement: "MREQ-10 (contribution only)"
    verification:
      - kind: unit
        ref: "TestMemberReqLookupOrder, TestMemberReqLookupCapAndPause"
        status: pass
  - id: D6
    description: "a Mute for the photo rule never replaces a restriction someone else placed (None, no hold, no notice, no restrictChatMember, one Info line with IDs only); Warn, Kick and Ban for it still act on a restricted member"
    requirement: "MREQ-11 (contribution only)"
    verification:
      - kind: unit
        ref: "TestMemberReqLookupNeverReplacesRestriction"
        status: pass
  - id: D7
    description: "the join entry reaches the lookups through the same executor: a photo-less joiner is muted with welcome_held, a joiner with a photo or a subscription is welcomed, a Warn welcomes after its notice, Kick, Ban and Mute handle an unsubscribed joiner, and a hung lookup is cut at the 4 s join budget (300 ms in the test), welcomes the joiner, writes no hold, and the first message is still checked"
    requirement: "MREQ-10, MREQ-11, MREQ-12 (contributions only)"
    verification:
      - kind: unit
        ref: "TestMemberReqJoinLookups"
        status: pass
  - id: D8
    description: "one punishment across three processes: nine concurrent enforcements of one photo-less member (Mute and Kick) give one punishment request, one hold and one notice"
    requirement: "MREQ-11 (contribution only)"
    verification:
      - kind: unit
        ref: "TestMemberReqLookupOnePunishmentAcrossReplicas"
        status: pass
  - id: D9
    description: "a job inside a hung photo lookup is ended by StopMemberReqJobs inside its existing wait, leaves no hold, notice, warn or write, a later job starts nothing, and the flight goroutine of the cancelled lookup has ended; no drain and no pool added"
    requirement: "PLAT-06 (contribution only)"
    verification:
      - kind: unit
        ref: "TestShutdownEndsMemberReqLookups"
        status: pass

duration: 95min
completed: 2026-11-08
status: complete
---

# Phase 14 Plan 06: The lookup phase of the executor and the join entry Summary

**The photo and channel rules now punish: memberReqLiveViolation folds memberReqLookupBroken's explicit breaches into the name mask after the live check and before the cap and the claim, so a member with no photo or no subscription is warned, removed or muted once with one notice, and every unanswerable lookup, through the job and the join entry, punishes nobody and leaves nothing behind.**

## Accomplishments

- `fuku/modules/memberreq_enforce.go`: two variables, `memberReqLookupJobBudget` (20 s) and `memberReqLookupJoinBudget` (4 s), chosen by `memberReqLookupBudget(c.Source)`; in `memberReqLiveViolation`, after the names, `memberReqLookupBroken` runs under a context of that budget and its mask is OR-ed into `broken` (the unknown flag is deliberately unused: unknown means "not broken"). The executor's numbered order comment names the step and says the claim follows every lookup. `memberReqEnforce` and `memberReqJoinThenWelcome` needed no other edit.
- `fuku/modules/memberreq_lookup_enforce_test.go` (new) and `memberreq_lookup_fake_test.go`: the ten tests the plan names (the channel subtests live inside `TestMemberReqLookupPunishes`), a probe that logs the order of lookup requests and, at each photo request, whether a hold exists and what the cap counter says, and arrival hooks (`setOnPhoto`, `setOnChannel`) on the wrapper fake.
- `fuku/modules/shutdown_drains_test.go`: `TestShutdownEndsMemberReqLookups`.
- AGENTS.md: the executor bullet (the lookup step, budgets, partial episode, place in the order), the join bullet (same executor, 4 s budget, welcomed and caught at the first message), the shutdown bullet (a job in a lookup ends at once, flight ends with it, 65 s worst case unchanged) and a pointer from the rule-table bullet to the one production caller.

## Task Commits

1. `2d9fd60` test(14-06): a member with no photo is punished and one with a photo is spared (RED: 5 of 6 subtests failed against the HEAD executor)
2. `60e3858` feat(14-06): enforce the photo rule through the lookup phase of the executor (GREEN of Task 1; the tracer slice is verified end to end: with the photo rule set, a member with no visible photo is punished through the executor)
3. `6dff238` test(14-06): every unanswerable lookup punishes nobody and the lookups keep their place (Task 2; see Deviations 1)
4. `b7e85d0` feat(14-06): the join entry and shutdown reach the lookups through the one executor (Task 3 tests and AGENTS.md; see Deviations 2)

## Which subtests were red

Task 1: `TestMemberReqLookupPunishes` (all four photo subtests) and the cache-key subtest of `TestMemberReqLookupSpares` failed on the commit before the executor change; the "spared with a photo under every punishment" subtests passed against the old executor, as expected (an executor that ignores the rule spares everyone). Tasks 2 and 3: no subtest was red against the Task 1 code, because step 6b is one fold that already satisfied them; their first red runs were test-setup faults (below). To prove the matrix can fail, I temporarily made an unknown verdict set the photo bit: `TestMemberReqLookupUnanswerable` failed on every photo shape for the job and the join entry (mutation reverted, `git diff` empty).

## Verification (real results)

- Task 1 verify (`TestMemberReqLookupPunishes|TestMemberReqLookupSpares`): ok. Task 2 verify (the seven named tests): ok. Task 3 verify, first command (`TestMemberReqJoinLookups|TestMemberReqLookupOnePunishmentAcrossReplicas|TestShutdownEndsMemberReqLookups`): ok.
- Task 3 verify, second command (the 19 named Phase 13 executor, join, cap, held-path and shutdown tests): `ok 23.644s`.
- Task 3 verify, flake run `-count=20` of `TestMemberReqLookupOnePunishmentAcrossReplicas|TestMemberReqJoinLookups`: `ok 82.431s` (20 of 20).
- Family run `-run '^(TestMemberReqLookup)' ./fuku/modules` (includes plan 14-03 and 14-04's lookup tests, so the `mreqChannel` change is covered): ok.
- Phase 13 regression `-run '^(TestMemberReq|TestSettingsMreq|TestSettingsDriftMreq|TestSettingsAuthorityMreq|TestSettingsControlActionAllowList|TestSettingsStaffPrivacy|TestShutdown|TestApplyWarn|TestApplyMemberReqAction)' ./fuku/modules` (SQLite): `ok 192.500s`.
- `CGO_ENABLED=0 go build ./...` ok; `go vet -tags testtools ./fuku/modules` clean; `gofmt -l` on the four Go files I touched: nothing (the plain `gofmt -l fuku/modules/` lists eight other files that were already unformatted before this plan: `blacklists_command_test.go`, `callback_codec.go`, `deeplink_router.go`, `error_logging.go`, `formatting.go`, `help.go`, `overwrite.go`; not mine, left alone).
- Lint stand-in `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827`, exit code read directly: 0 ("no dupl, gocyclo or godox findings on lines added since 8c0c827").
- `git diff --exit-code 8c0c827 -- go.mod go.sum`: clean.
- Acceptance greps: `memberReqLookupBroken(` in `memberreq_enforce.go` 1, `memberReqLookupJoinBudget` there at least 1, `func TestMemberReqLookupPunishes` 1, `func TestMemberReqLookup` in the new test file 8 (at least 7), `staffFake429` in it at least 1, `func TestShutdownEndsMemberReqLookups` 1, `memberReqLookupJobBudget` in AGENTS.md 1.
- Not run: `make lint` and gosec (cannot run in this sandbox), `make test` (plan 14-12's gate), the settings family (this plan changed no settings code; the Phase 13 regression above includes the `TestSettingsMreq*`, drift, authority and privacy tests), PostgreSQL (nothing here rests on it; the harness does not run on PostgreSQL here).

## Deviations from Plan

**1. [Test-setup bug, Rule 1] `mreqChannel` wiped the group's stored settings.** Plan 14-03/14-04's helper registered the group through `mreqLookupCleanup(t, nil, []int64{channel, group})`, and `memberReqCleanup` runs its cleanup once immediately ("before the test"), which deleted the requirement settings and greetings the test had stored for its group a moment earlier. My first Task 2 run showed it as channel rules that never punished and joiners that were never welcomed. Fixed in the helper (`memberreq_lookup_fake_test.go`): the channel keeps the immediate and after-test cleanup, the group gets only an after-test `memberReqDeleteRedisKeys` (the group's `lc:` key). Plan 14-04's own tests still pass (`TestMemberReqLookup` family).

**2. [Commit type] Task 2 is one `test(14-06)` commit and Task 3's code change is the tests plus AGENTS.md.** The plan allowed `feat(...)` titles for the tasks, but no source file changed in Tasks 2 and 3 (step 6b was complete after Task 1), so the changelog-visible `feat:` commit is Task 1's. Task 2's commit message says "31-shape matrix"; the matrix has 28 shapes (9 photo, 19 channel), each run for the job and for the join entry (56 subtests); the message cannot be amended.

**3. [Scope, test detail] The first message after a cut-off join lookup is the executor run directly.** The watcher starts no job for a member whose names are clean until plan 14-07, so `TestMemberReqJoinLookups` checks "the first message is still checked" with `mreqEnforce` on a fresh case dated now (the plan's assumption names this); 14-07 owns the dispatcher end-to-end test.

**4. [Test detail] `TestMemberReqLookupCapAndPause` uses the production cap (10), 12 members and Kick**, so it needs no override and proves the cap counter at the photo request of member i is i (the cap is counted after the lookups). The replica test uses nine callers, below the cap of 10, so the cap never muddies the one-punishment count.

## Known Stubs

None.

## Threat Flags

None. No new endpoint, auth path, schema or goroutine pool; the lookups were already added in plans 14-03 and 14-04 and this plan only decides when the executor asks them (T-14-21 to T-14-25 are covered by `TestMemberReqLookupUnanswerable`, `TestShutdownEndsMemberReqLookups`, `TestMemberReqLookupOnePunishmentAcrossReplicas`, `TestMemberReqLookupNeverReplacesRestriction` and `TestMemberReqJoinLookups`; T-14-SC by the clean go.mod/go.sum diff).

## Known limits and new concerns

- The lookup budget bounds the caller's wait only. A hung lookup keeps its shared flight alive until `memberReqLookupFlightTimeout` (15 s) or the shutdown root cancel, so a raid of joiners with a hung Telegram can hold one flight goroutine per distinct member for up to 15 s each (each is a goroutine, not a pool; the lookup pacer sheds the calls beyond its 5 s `MaxWait`, so the number of real Telegram requests stays bounded). Not changed here; it is the 14-03 design (research A8).
- The partial episode is by design (decision S-02): a member punished for a name rule while the photo lookup was shed carries a hold without the photo bit, and the photo rule is judged for them again only after the hold clears (plan 14-07's held release owns that for holds that carry lookup bits).
- Between this plan and plan 14-07 the watcher does not hand a clean-named member to a job, so the photo and channel rules punish at join (when Check on join is on) and through any message job a name violator already starts, but not yet at a clean-named member's first message. No release happens between plans.
- Eight files of `fuku/modules` are not gofmt-clean at HEAD (see Verification); out of scope, untouched.

## Self-Check: PASSED

The created and modified files exist and the four commits (`2d9fd60`, `60e3858`, `6dff238`, `b7e85d0`) are ancestors of HEAD; `git rev-list --count 47dda27..HEAD` printed 4 before this summary.
