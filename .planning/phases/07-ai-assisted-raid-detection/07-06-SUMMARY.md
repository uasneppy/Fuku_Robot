---
phase: 07-ai-assisted-raid-detection
plan: 06
subsystem: raid-detection
tags: [go, redis, lua, aispam, typesafe, gotgbot, worker-pool]

requires:
  - phase: 07-ai-assisted-raid-detection
    provides: "raidAIRecordVerdict, aispamIsDelete, the AI verdict counter and newAIBurstEnv (plan 07-01)"
  - phase: 07-ai-assisted-raid-detection
    provides: "config.AppConfig.GeminiAPIKey (plan 07-03) and the per-group ai_enabled / ai_threshold settings (plan 07-04)"
provides:
  - "raidAIWatcher at handler group 2: a new member's link, forward or repeated text is checked once by TypeSafe off the update path"
  - "raid AI pool: 128-job queue, 4 recovered workers, shed when full, raidAIJobs tracker, raidAIProcess seam"
  - "raidAIProcessText: raid-only text check that reports a spam verdict to the burst counter and never acts"
  - "aispamWillJudge: one TypeSafe call per message whichever path judges it"
  - "raidAIConfigured / raidAITextConfigured / raidAIImageConfigured and GetRaidAIStats"
  - "Redis key alita:raid:{<chat>}:ar:<user> (hashes of a new member's recent texts, never raw text)"
affects: [07-07, 07-08, 07-10, 07-14]

actuals:
  tokens: 12000
  tasks: 2
  commits: 4
plan_head_before: ef1d2c405073f439509ce3f04d870a8a45dc2b37
plan_head_after: b5a09ae55084e33bcff8686f0a7dcc03077143fa

tech-stack:
  added: []
  patterns:
    - "Candidate watcher at its own handler group that always continues the update and hands work to a bounded pool"
    - "One Lua script answers 'is this member new' and 'is this a repeat' and records only a hash, only for new members"
    - "Raid-only AI jobs share the /aispam breaker but ignore its tripped return, so no notice is ever posted from the raid path"

key-files:
  created:
    - alita/modules/raid_ai.go
    - alita/modules/raid_ai_text_test.go
    - alita/modules/raid_ai_breaker_test.go
  modified:
    - alita/modules/lockdown.go
    - alita/modules/raid_ai_burst_test.go
    - alita/modules/test_harness_test.go
    - alita/modules/staff_action_fake_test.go
    - alita/modules/lockdown_alert_test.go
    - AGENTS.md

key-decisions:
  - "The watcher is registered in LoadLockdown beside the flood watchers at group 2 and returns ext.ContinueGroups on every path; the work lives in raidAIWatchText so the watcher has no return nil"
  - "The candidate script records a hash only for a member who joined within 24 h, so an established member leaves no key; a text under 10 runes has no hash and is never a repeat"
  - "aispamWillJudge is checked right before the candidate script, so a text /aispam will judge costs one settings read and no Redis write on the raid path"
  - "The raid text job builds aispamTask.state(nil, 1): no sender history is read or written (it stores raw text), and the breaker's tripped return is ignored"
  - "The fakes' read helpers (callsFor, sentTo, sentChats) wait for raidAIJobs before raidPosts, because a raid AI worker can start the lockdown whose posts are waited for next"
  - "enableAI was split: serveTypeSafe configures the TypeSafe fake without turning /aispam on for any chat, which newRaidAIEnv needs"

patterns-established:
  - "newRaidAIEnv: flood env plus a TypeSafe fake, /aispam off, settleAI registered as cleanup"

requirements-completed: [RAID-04, RAID-05, RAID-03]

coverage:
  - id: D1
    description: "A text from a member who joined the supergroup in the last 24 h that carries a link, is a forward, or repeats one of their last texts is checked once by TypeSafe, and a spam verdict counts toward the AI burst"
    requirement: RAID-04
    verification:
      - kind: integration
        ref: "alita/modules/raid_ai_text_test.go#TestRaidAICandidateText"
        status: pass
    human_judgment: false
  - id: D2
    description: "Established members, plain texts, short repeats, admins, approved users, pending captcha, anonymous and channel senders, automatic forwards, service accounts, the bot, basic groups, a chat with the AI trigger off and captions are never sent to any AI"
    requirement: RAID-04
    verification:
      - kind: integration
        ref: "alita/modules/raid_ai_text_test.go#TestRaidAICandidateText"
        status: pass
    human_judgment: false
  - id: D3
    description: "A message gets one TypeSafe call whichever path judges it: /aispam on reuses its own delete verdict, /aispam off lets the raid path check and delete nothing, and a keep verdict counts on neither"
    requirement: RAID-04
    verification:
      - kind: integration
        ref: "alita/modules/raid_ai_burst_test.go#TestRaidAISpamSharesVerdict"
        status: pass
    human_judgment: false
  - id: D4
    description: "The watcher never waits on AI and never acts: a hung provider returns the update within 500 ms, and no delete, send, ban or restrict call is made"
    requirement: RAID-05
    verification:
      - kind: integration
        ref: "alita/modules/raid_ai_text_test.go#TestRaidAICandidateText"
        status: pass
    human_judgment: false
  - id: D5
    description: "Raid-only failures pause the chat through the shared breaker without posting the /aispam notice, another chat is still checked, and checks resume after the cooldown"
    requirement: RAID-05
    verification:
      - kind: integration
        ref: "alita/modules/raid_ai_breaker_test.go#TestRaidAIBreaker"
        status: pass
    human_judgment: false
  - id: D6
    description: "Without Redis nothing is checked or counted, the no-Redis notice is logged at most once and /lockdown still works"
    requirement: RAID-03
    verification:
      - kind: integration
        ref: "alita/modules/raid_ai_text_test.go#TestRaidAINoRedis"
        status: pass
    human_judgment: false
  - id: D7
    description: "No raw message text is stored: the repeat list holds only 16-character hex hashes and no alita:aispam: key is written"
    requirement: RAID-04
    verification:
      - kind: integration
        ref: "alita/modules/raid_ai_text_test.go#TestRaidAICandidateText"
        status: pass
    human_judgment: false
  - id: D8
    description: "With a real TypeSafe key, a fresh account's link spam in a group that never enabled /aispam is judged and a burst of it locks the group"
    requirement: RAID-04
    verification: []
    human_judgment: true
    rationale: "Needs a live TypeSafe key and a real supergroup; the fake provider cannot show the real model's verdicts"

duration: 35min
completed: 2026-10-29
status: complete
---

# Phase 07 Plan 06: Raid AI text candidates Summary

**A group-2 watcher sends only a new member's link, forward or repeated text to a bounded raid AI pool where one TypeSafe check per text reports a spam verdict to the AI burst counter, never waiting on AI and never acting, and reusing /aispam's verdict wherever /aispam judges.**

## Performance

- **Duration:** about 35 min
- **Tasks:** 2 of 2
- **Files:** 3 created, 6 modified

## Accomplishments

- `alita/modules/raid_ai.go` holds the whole text path. `raidAIWatcher` (group 2, registered in `LoadLockdown`) applies the candidate rules cheapest first: supergroup, `raidFloodSenderCounts`, a non-empty `Text` (captions are left to the image path), `raidAITextConfigured`, `raidRedis`, the chat's `AIEnabled`, `aispamWillJudge`, the candidate script and rule, then `aispamCarvedOut` (pending captcha, cached admin, approved), then the enqueue. It returns `ext.ContinueGroups` on every path and has no `return nil`.
- `raidAICandidateScript` (KEYS: the `:joined` set and `alita:raid:{<chat>}:ar:<user>`) returns `{new, repeated}` and records an 8-byte SHA-256 hex of the normalised text only for a member who joined within 24 h; the list keeps 5 hashes for 1 h, a text under 10 runes has no hash. The rule is new AND (link domains OR forward OR repeated).
- The raid AI pool has a 128-job queue and 4 workers started once, each wrapped in `RecoverFromPanic` (worker and job), a `raidPostTracker` in-flight counter taken before `raidAIDraining` is read, `raidAIEnqueue` shedding a job when the queue is full, and `raidAIProcess` as the test seam. `GetRaidAIStats` reports checked, spam, kept, shed and failed.
- `raidAIProcessText` refuses a paused chat (counted shed, no request), builds `aispamTask.state(nil, 1)` so no sender history is read or written, records failures on `/aispam`'s breaker while ignoring its tripped return (no notice from the raid path), resets it on success, decides spam with `aispamIsDelete` and reports through `raidAIRecordVerdict`. It logs only chat, user and message IDs.
- `aispamWillJudge` (TypeSafe configured, `aispam.IsAISpamEnabled`, breaker not paused) makes the raid path stand aside wherever `/aispam` will judge the text, so a message makes at most one TypeSafe call and `/aispam`'s own delete verdict feeds the counter.
- AGENTS.md was updated in the same commits: the group list gains group 2, a Data bullet for the `ar` key, and three Subsystem-trap bullets (candidate rules, the shared-verdict rule with the silent breaker, the pool).
- Test helpers: `newRaidAIEnv`, `sendText`, `watchDirect`, `wantNoActions`, `seedJoinedIn`; `enableAI` was split so `serveTypeSafe` configures the fake without enabling `/aispam`; `settleAI` waits for `raidAIJobs` first; `callsFor`, `sentTo` and `sentChats` also wait for `raidAIJobs`.

## Task Commits

1. RED tests for the text candidates: `13a9424` (test)
2. Task 1 GREEN, watcher, pool, stats, group-2 registration, AGENTS.md: `ade1f8b` (feat)
3. RED tests for the shared verdict, no-Redis and breaker: `b5a0e20` (test)
4. Task 2 GREEN, `aispamWillJudge` and the AGENTS.md rule: `b5a09ae` (feat)

## Verification

- `go test -tags testtools -race -count=1 -run '^(TestRaidAISpamSharesVerdict|TestRaidAINoRedis|TestRaidAIBreaker|TestRaidAICandidateText)$' -v ./alita/modules`: all four PASS (RED run before `aispamWillJudge` failed exactly the two `/aispam on` cases with 2 requests instead of 1).
- `go test -tags testtools -race -count=1 -run 'AISpam|Raid|Lockdown|Unlockdown' ./alita/modules`: ok (about 135 s), after Task 1 and after Task 2.
- `CGO_ENABLED=0 go build ./...`: ok. `go vet -tags testtools ./alita/modules`: clean. `gofmt -l alita`: only the pre-existing `greetings_command_test.go`. `git diff --exit-code go.mod go.sum`: unchanged.
- Acceptance criteria re-run for both tasks: `raidAIGroup = 2` x1, queue 128 x1, workers 4 x1, `RecoverFromPanic` x3, registration x1, `return ext.ContinueGroups` x1, `return nil` in the watcher x0, `alita:raid:{<chat>}:ar:<user>` and `raid AI candidate watcher` in AGENTS.md, `func aispamWillJudge(` x1, both new test functions x1, `aispamWillJudge` in AGENTS.md.
- `make lint` could not run (golangci-lint is built with Go 1.25, the module is Go 1.26); `go vet -tags testtools` and `gofmt -l` stood in. The orchestrator runs the full `make test`.

## Deviations from Plan

None - plan executed exactly as written. Notes: the Task 1 RED commit does not compile (the test references `raidAIWatcher` and the pool), which is what a RED commit with a missing symbol is; the plan-commit ledger and spawn sentinel under the shared git dir were written explicitly to `.git/worktrees/<name>/` because the compound shell form was refused by the sandbox.

## Known Stubs

None. `raidAIProcessJob` only dispatches to the text job today; plan 07-08 adds the image kind.

## Threat Flags

None. T-07-20 (only 16-hex hashes of new members' texts, 5 each, 1 h, no `/aispam` history, tested), T-07-21 (one settings read and one script call on the update path, pool of 128 and 4, hung-provider test), T-07-22 (every D-02 case sends nothing, tested) and T-07-23 (no `sendMessage` from the raid breaker, tested) are mitigated as planned.

## Pending Human Verification

- Live check with a real `TYPESAFE_API_KEY` and a supergroup that never enabled `/aispam`: a fresh account posting link spam is judged (one TypeSafe request per text), a burst of five from three accounts locks the group, and an established member's link is never sent.
- Observe in production logs that raid-only failures during a TypeSafe outage pause the chat and post no breaker notice in groups that never enabled `/aispam`.
- Until plan 07-14 moves lockdown starts off the workers, a worker reporting the tipping verdict runs `raidFire` itself (bounded by `raidFireTimeout` and the 60 s latch); worth watching shed counts during a real raid.

## Self-Check: PASSED

- Created files present: alita/modules/raid_ai.go, alita/modules/raid_ai_text_test.go, alita/modules/raid_ai_breaker_test.go.
- Commits 13a9424, ade1f8b, b5a0e20, b5a09ae present in `git log`.
