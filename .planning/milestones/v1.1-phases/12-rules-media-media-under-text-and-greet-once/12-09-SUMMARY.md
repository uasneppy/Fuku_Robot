---
phase: 12-rules-media-media-under-text-and-greet-once
plan: 09
subsystem: greetings
tags: [greet-once, welcome, claim, exactly-once, apply-function]
requires:
  - phase: 12-rules-media-media-under-text-and-greet-once
    provides: 12-03 ClaimGreeting, ReleaseGreeting and SetWelcomeOnce, 12-04 media.ErrNoPermission, 12-08 SendWelcomeMessage through media.SendGreetingMessage
provides:
  - greetingWrites.WelcomeOnce seam field and applyWelcomeOnce in fuku/modules/greetings.go
  - greetOnceStore (Claim and Release test seam), claimWelcome, giveBackWelcome and welcomeDefinitelyNotSent
  - the greet-once claim and give-back inside SendWelcomeMessage
affects: [12-10, 12-14, 12-16]

actuals:
  tokens: 11500
  tasks: 3
  commits: 4

commits: 4
plan_head_before: 395b9dbbf09847d374ce97bf834cf906941ec3a9
plan_head_after: f18b57ce2eba27b9499847fdc255005ae4dd1f58

tech-stack:
  added: []
  patterns:
    - "Claim before the side effect, give back only on a proven non-delivery, keep on an unknown outcome"
    - "A red commit carries a compile-only stub so the target test fails on its assertion"

key-files:
  created:
    - fuku/modules/greet_once_test.go
  modified:
    - fuku/modules/greetings.go
    - AGENTS.md

key-decisions:
  - "A 429 counts as a definite refusal (planner assumption A4, kept as the plan flagged it): Telegram answered without processing the request, so the claim is given back"
  - "The claim and give-back live in two small helpers (claimWelcome, giveBackWelcome) so SendWelcomeMessage stays under the gocyclo limit; the claim database error returns send=false (fail closed)"
  - "ReleaseGreeting takes no context and runs on db.DB, so a cancelled update cannot stop the give-back; a release error is logged at Warn with chat ID only"

patterns-established:
  - "greetOnceStore is the one seam over the greeted_members claim, restored by t.Cleanup in a test"

requirements-completed: []

coverage:
  - id: D1
    description: "With Greet once on a member who joins, leaves and joins again is welcomed once (one greeted_members row); another member is welcomed; with the switch off the table is not read and the member is welcomed again"
    requirement: "EDIT-09"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestGreetOnceFirstJoinOnly$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D2
    description: "Only a real welcome claims: Welcome off, Greet once off, a captcha challenge, a captcha failure and a lockdown-banned joiner write no row; a captcha pass welcomes and records once; no Redis changes nothing"
    requirement: "EDIT-09"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestGreetOnceWelcomeOffNoClaim|TestGreetOnceOffNoRecord|TestGreetOnceCaptchaPathAndFailure|TestGreetOnceLockdownBanNoRecord|TestGreetOnceWithoutRedis)$' -v ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D3
    description: "16 concurrent SendWelcomeMessage calls for one member send exactly one welcome and leave one row (SQLite here; PostgreSQL run is CI, via 12-16)"
    requirement: "EDIT-09"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestGreetOnceConcurrentOneWelcome$' ./fuku/modules"
        status: pass
      - kind: unit
        ref: "TestGreetedMemberClaimConcurrent on PostgreSQL (CI integrity run once 12-16 adds it)"
        status: not-run
    human_judgment: false
  - id: D4
    description: "A definite refusal (400, 403, 429, a restricted chat) gives the claim back and the next join is welcomed; an unknown outcome (502, transport error, deadline) keeps it and the next join sends nothing; a claim error sends nothing"
    requirement: "EDIT-09"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestGreetOnceDefiniteRefusalReleases|TestGreetOnceUnknownOutcomeKeeps|TestGreetOnceClaimErrorFailsClosed)$' -v ./fuku/modules"
        status: pass
    human_judgment: false

duration: 40min
completed: 2026-11-05
status: complete
---

# Phase 12 Plan 09: Greet once runtime Summary

**A group with Greet once on welcomes each member only the first time they ever join, on any replica and with no Redis: `SendWelcomeMessage` claims the member in `greeted_members` after the Welcome-on check and before the send, gives the claim back only when Telegram definitely refused the welcome, and keeps it on an unknown outcome.**

## What changed

- `greetings.go`: seam field `greetingWrites.WelcomeOnce` (= `greetings.SetWelcomeOnce`); `applyWelcomeOnce` returns `finishGreetingApply(...)`, the one write path of the Welcome page's switch; `greetOnceStore` (Claim and Release, a test seam over `greetings.ClaimGreeting` and `ReleaseGreeting`); `welcomeDefinitelyNotSent` (`media.ErrNoPermission`, `isTelegramClientError`, or a `*gotgbot.TelegramError` with code 429, by code and never by wording); `claimWelcome` and `giveBackWelcome`. In `SendWelcomeMessage` the claim sits inside the `ShouldWelcome` branch before `renderGreeting`; a lost claim or a claim error returns nil with no send and no clean-old step; a failed send of a claimed welcome calls `giveBackWelcome`.
- `greet_once_test.go`: ten behaviour tests, one helper set (`greetOnceSetup`, `greetOnceJoin`, `greetOnceJoinErr`, `greetOnceLeave`, `greetedRows`, `greetOnceWelcomes`, `greetOnceCleanup`, `greetOnceSwitchOn`). No `t.Skip`.
- `AGENTS.md`: the `greeted_members` Data bullet now states the claim rule in place of "arrives with plan 12-09"; a new Subsystem traps bullet "Greet once" lists where the claim is made, who is never recorded and the accepted limits (chat migration, a welcome switched off records nobody, bots claimed like members, rows kept for good, 429 as a definite refusal).

## TDD record

- **Task 1 (tracer) RED** `3b39ec1`: `TestGreetOnceFirstJoinOnly` failed on `greeted rows after A's first join = 0, want 1`; `applyWelcomeOnce` was a compile-only stub returning the zero result, so the failure is the planned assertion. **GREEN** `03ee9e4`: real `applyWelcomeOnce`, the seam field, `greetOnceStore`, the classifier and the claim. (The seam field was added in the GREEN commit; the RED stub never referenced it.) The first GREEN run failed once on my own test counting the goodbye a leave sends as a welcome; the helper now counts only texts containing "Welcome".
- **Task 2** `a5734fd`: the five tests passed at once on Task 1's code, as the plan expects (every welcome goes through `SendWelcomeMessage`); they are characterisation and regression guards, not RED tests.
- **Task 3** `f18b57c`: the four tests also passed at once. To show they are not vacuous I ran two temporary mutations (not committed): making `giveBackWelcome` a no-op failed all four `TestGreetOnceDefiniteRefusalReleases` cases; making `claimWelcome` always return send=true failed `TestGreetOnceFirstJoinOnly` and `TestGreetOnceConcurrentOneWelcome`. The file was restored from a backup and the tree diff showed only the test file.
- `gsd_run check tdd-red-evidence` was not run (plan is `type: execute`, evidence is a console report; semantic check done by reading the `--- FAIL` line).

## Commits

- 3b39ec1 test(12-09): add failing test for greeting a member only on their first join
- 03ee9e4 feat(12-09): greet a member only the first time they ever join
- a5734fd test(12-09): the greet-once claim follows only a real welcome
- f18b57c test(12-09): one welcome across replicas, given back only on a definite refusal

## Verification (real results)

- Task 1: `-run '^(TestGreetOnceFirstJoinOnly|TestLegacyGreetingsRequestsUnchanged|TestGreetingsMediaBelowEverywhere)$' ./fuku/modules`: ok.
- Task 2: `-run '^(TestGreetOnceWelcomeOffNoClaim|TestGreetOnceOffNoRecord|TestGreetOnceCaptchaPathAndFailure|TestGreetOnceLockdownBanNoRecord|TestGreetOnceWithoutRedis)$' -v`: five `--- PASS`, no SKIP.
- Task 3: `-run '^(TestGreetOnceConcurrentOneWelcome|TestGreetOnceDefiniteRefusalReleases|TestGreetOnceUnknownOutcomeKeeps|TestGreetOnceClaimErrorFailsClosed)$' -v`: all PASS with their subtests; `grep -c welcomeDefinitelyNotSent AGENTS.md` prints 1; `grep -q 'Greet once' AGENTS.md` exits 0.
- Plan family `-run '^(TestGreetOnce|TestGreeting|TestMemberJoin|TestLeftMember|TestCaptcha|TestLockdown|TestLegacyGreetings|TestSetWelcome|TestProcessSingleNewMember|TestCleanService)' ./fuku/modules`: ok (84.5 s). It includes `TestLegacyGreetingsRequestsUnchanged`, `TestGreetingsMediaBelow*`, `TestMemberJoin*`, `TestSetWelcome*`, the captcha tests and the lockdown join tests.
- `go test -tags testtools -race -count=1 ./fuku/db/greetings`: ok. `CGO_ENABLED=0 go build ./...`: ok. `go vet -tags testtools ./fuku/modules`: clean. `gofmt -l` on `greetings.go` and `greet_once_test.go`: empty. `git diff --exit-code 72a692b -- go.mod go.sum` and against `bd2f794`: clean.
- `python3 .../lint-local.py 72a692b`: exit 0 (no dupl, gocyclo or godox findings) before the last code commit (this plan's last code change is the test file) and again before this SUMMARY.

## Deviations from Plan

None - plan executed as written. The plan's lint command names `.planning/phases/11-message-editor/handoff/lint-local.py bd2f794`; the executor brief's stand-in with base `72a692b` was used instead, as instructed.

## Known Stubs

None. `applyWelcomeOnce` has no production caller until plan 12-10 (the Welcome page's Greet once switch); `TestGreetOnceFirstJoinOnly` and every other test of this plan call it, so the unused check stays quiet.

## Not run / notes

- `TestGreetedMemberClaimConcurrent` and the PostgreSQL run of the claim: not run here (no PostgreSQL); `TestGreetOnceConcurrentOneWelcome` ran on SQLite. The PostgreSQL run happens in CI once 12-16 adds it to `make test-postgres-integrity`.
- `make test`, `make lint` and the settings test family were not run (sandbox limits; the plan does not ask for them).
- A Release error is only logged (Warn, chat ID only); it is not unit tested, because the seam's Release is exercised only through the refusal cases where it succeeds.
- Requirements: this plan completes none (`requirements-completed: []`). EDIT-09 stays Pending in REQUIREMENTS.md until 12-10 adds the Welcome page switch.

## Threat Flags

None beyond the plan's register. Mitigations: T-12-25 (primary-key claim before the send, `TestGreetOnceConcurrentOneWelcome`), T-12-26 (`TestGreetOnceDefiniteRefusalReleases`), T-12-28 (`TestGreetOnceWelcomeOffNoClaim`, `TestGreetOnceCaptchaPathAndFailure`, `TestGreetOnceLockdownBanNoRecord`), T-12-29 (claim and release logs carry chat ID and error only), T-12-SC (go.mod and go.sum unchanged).

## Self-Check: PASSED

`fuku/modules/greet_once_test.go` exists and `greetings.go` and `AGENTS.md` are changed; commits 3b39ec1, 03ee9e4, a5734fd and f18b57c are ancestors of HEAD.
