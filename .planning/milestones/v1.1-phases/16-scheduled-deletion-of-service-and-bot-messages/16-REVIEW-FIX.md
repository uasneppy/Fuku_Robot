---
phase: 16-scheduled-deletion-of-service-and-bot-messages
review: 16-REVIEW.md
status: all_fixed
findings_in_scope: 7
fixed: 7
skipped: 1
fixed_at: 2026-11-12
---

# Phase 16: Review Fix Report

Fix pass for WR-01, WR-02, WR-03, IN-01, IN-02, IN-04 and IN-05 of `16-REVIEW.md`, run sequentially on branch
`claude/ecstatic-goldberg-qsss05`. Every behaviour finding (WR-01, WR-02, WR-03, IN-01, IN-05) has a RED `test(16-fix)` commit and a
GREEN `fix(16-fix)` commit; IN-04 is a refactor and IN-02 is documentation only. Deferred by the orchestrator (code untouched): IN-03.
No requirement was ticked or unticked, `phase.complete` was not run, no planning file other than this report was edited, `go.mod` and
`go.sum` are unchanged since 8c0c827, nothing was pushed.

## Commits

| Finding | Preparation | RED (test) | GREEN (fix) |
|---------|-------------|------------|-------------|
| WR-01 | 490a450 (inline timeout made a variable) | 18e6895 | 95badfc |
| WR-02 | | a33e6c7 | 9b93c18 |
| WR-03 | | b798c81 | a129472 (+ test adaptations 3555ee5, 24c766c) |
| IN-01 | | 210dd86 | 08d309a |
| IN-05 | | 940e2b5 | f985759 |
| IN-04 | | | b925737 |
| IN-02 | | | d4fa04c |

Each RED commit failed its new assertions for the stated reason on the then-current code, never on a compile error:

- WR-01: `queue rows = [], want one due now` (a 30 s flood-control block, a 150 ms inline deadline: no delete request and no row).
- WR-02: repository `a counted release set delete_at to <old time>, want <retry time>` and `claim one second before the retry time = 2
  rows`; worker `delete_at after a counted failure = ..., want at least now + 1m0s`.
- WR-03: `queue of chat ... = map[70001:pending 70002:pending 70003:acting 70004:pending], want map[70003:acting 70004:pending]` in all
  seven positive cases (a service kind, two bot kinds, the prefix case, Keep from joined and from left, `/cleanservice off`); the negative
  cases already held and stayed green.
- IN-01: `queue rows ... Kind:bot:punish_manual ..., want none` for `/ban -1001234567890` and `/unban -1001234567890`.
- IN-05: `private chat: Schedule = (true, <nil>), want (false, invalid scheduled deletion entry)`, plus one Staff Group read and one row for
  the private chat.

Two RED commits carry a mechanical production edit so the tests compile: 490a450 (`deletionInlineTimeout` const to var, a seam, no
behaviour change) and a33e6c7 (`Release` takes the retry time, the worker passes `deletionNow() + deletionRetryBackoff`, but the statement
does not use it yet). No test is skipped, `t.Skip` and `t.Parallel` are not used.

## WR-01: a flood-control wait over 15 s lost inline "delete now" messages

**What changed.** `deletionInlineSettle` now asks `deletionInlineShed(ctx, err)` for a retry-free answer: true for `ratelimit.ErrRateLimited`
(as before) and also when the inline context's own deadline ran out (`errors.Is(ctx.Err(), context.DeadlineExceeded)`) while the deletion
root is still live (`deletionContext().Err() == nil`). Both queue the message as a row due now plus a wake. A cancelled root (shutdown) is
`context.Canceled`, so it still writes nothing from a goroutine whose database may be closing, and `TestDeletionInlineCapFallsBackToQueue`
(its cancel half) is unchanged and passes. `deletionInlineTimeout` is a variable now (the one seam; tests swap it with `mreqSwap`).

**Files.** `fuku/modules/deletion_schedule.go`, `AGENTS.md` ("Scheduled deletion core").

**Test.** `TestDeletionInlineDeadlineFallsBackToQueue` (`deletion_schedule_test.go`): a shared block of 30 s (under `MaxWait`, so the pacer
sleeps through it), inline timeout 150 ms; no `deleteMessage` request, exactly one row for the message due now.

## WR-02: counted retries had no backoff

**What changed.** `deletion.Release(ctx, token, ids, countAttempt, maxAttempts, retryAt)` takes a Go-computed retry time. A counted release
adds an attempt and sets `delete_at` to `retryAt` (UTC, whole seconds) in the same update, so SQLite and PostgreSQL run one statement; an
uncounted release (a 429, a shutdown) leaves `delete_at` alone and ignores `retryAt`; a counted release with a zero time is
`ErrInvalidRetryAt`. The worker passes `deletionNow().Add(deletionRetryBackoff)` with `deletionRetryBackoff` = 60 s, so the three counted
tries are at least a minute apart and a few seconds of Telegram trouble no longer drops rows. The 47 hour drop still bounds a row's life.

**Files.** `fuku/db/deletion/queue.go`, `fuku/modules/deletion_worker.go`, `AGENTS.md` (the `scheduled_deletions` Data bullet, the worker
bullet; "three counted tries" and "2 minute stale release" are unchanged and still true, the shutdown arithmetic is untouched).

**Tests.** Repository (`fuku/db/deletion/queue_test.go`, SQLite): a counted release moves `delete_at` to the retry time (a claim one second
before finds nothing, a claim at it finds both rows), an uncounted release leaves `delete_at` exactly as it was, a counted release with no
retry time is `ErrInvalidRetryAt`. Worker: `TestDeletionWorkerClassifiesErrors` (below) and `TestDeletionWorkerRateLimitedFree` now also
asserts a free release did not move `delete_at` past now.

**Deviation (edited existing tests).** `TestDeletionWorkerClassifiesErrors` encoded three back-to-back cycles as intended behaviour. Its
"retry" case now asserts the failed row's `delete_at` is at least now plus the backoff, that the next cycle makes no Telegram request and
leaves the row at one attempt, and then advances the test clock (a new helper `deletionTestAdvance`, through the `deletionNow` seam) past
the backoff for the second and third tries, which end in the drop. The repository test "the third counted release drops the row" claims at
`now + 2 min` instead of `now`, for the same reason. Nothing else of either test changed.

**PostgreSQL.** The changed statement sets `delete_at` from a bound Go time exactly as `claimed_at` already is, and `attempts + 1` is
unchanged; I see nothing PostgreSQL could treat differently, but I did not run it there (not in the integrity list, see NEW-04).

## WR-03: queued rows survived switching a rule off

**What changed.** A private repository helper `dropPendingKindsTx(tx, chat, kinds...)` deletes the chat's `pending` rows whose kind is
exactly one of the listed kinds (`kind IN ?`, no prefix `LIKE`; `DropChatPending` and its prefix are untouched and still used by the worker's
gone and service-latch paths). It is called inside the transaction that already holds the rule change:

- `RemoveRule` is now one transaction: delete the rule row, drop that exact kind's pending rows, then `cache.DeleteCache` as before.
- `ApplyJoinLeftRule` Keep drops the pending `service:new_chat_members` and `service:left_chat_member` rows with the two rule rows and the
  flag write; Now and After N drop nothing.
- `ApplyCleanServiceFlag(off)` drops the same two kinds in its transaction; `on` drops nothing.

`acting` rows (already claimed by a worker) are never touched, nor are other kinds, other chats, or a delay change (`SetRule`). Every
existing `cache.DeleteCache` is kept. The apply functions needed no code change: `applyDeleteRule` and `applyCleanServiceRule` report
`deletionApplied` only when the repository call (rule plus drop) returned nil, and a failure stays `deletionSaveFailed` with the error; the
hook-failed transaction case proves the rows and the rule stay.

**Files.** `fuku/db/deletion/rules.go`, `AGENTS.md` (the `chat_delete_rules` Data bullet).

**Tests** (`fuku/modules/deletion_apply_drop_test.go`, real queue through the apply functions): a service kind, a bot kind and a bot kind
beside a service kind (pending rows dropped, an acting row, another kind and another chat kept); a prefix case (`bot:punish_auto` removal
keeps `bot:punish_manual`); delay changes (1200, 0, 30) drop nothing; a failed `Remove` reports a save failure carrying the error and drops
nothing; Keep from joined and from left drops both kinds' pending rows only; Now and After drop nothing; a failure inside the transaction
(the `SetApplyHookForTest` seam) keeps the flag, the rule row and the rows; `/cleanservice on` keeps the queue and `off` drops the two kinds.

**Deviation (existing tests edited, three).** WR-03 changes what three Phase 16 tests encoded: `TestBotWelcomeScheduled` and
`TestBotGoodbyeScheduled` removed the kind's rule and expected the already queued rows to stay (3555ee5, now expect none), and
`TestMemberReqFixedRescheduleMovesUp`'s "the kind turned off after the notice" case expected the mute notice's 24 hour row to survive
`RemoveRule` (24c766c: the row goes with the rule, the case asserts one row before, none after, and none after the tap). I found these by
running the Phase 16 families only after the GREEN commit, so a129472 itself leaves those three tests red; the two follow-up `test(16-fix)`
commits turn them green. History was not rewritten.

## IN-01: the `/ban` and `/unban` "reply to a message" hint was scheduled

**What changed.** New `moderationCtx.replyHint` sends the same `Msg.Reply` request as `replySuccess` and never schedules. The two hint
branches (`bans_anonymous_ban_reply_only`, `bans_anonymous_unban_reply_only`) use it through a small `logReplyError` that logs and returns
the error exactly as the reply funcs did; the success texts keep `replySuccess`.

**Files.** `fuku/modules/bans.go`, `fuku/modules/moderation.go`, `AGENTS.md` (manual-kind bullet).

**Test.** `TestBotManualRepliesNotCovered` has two new cases (`/ban -1001234567890`, `/unban -1001234567890`, one send each, no row).

## IN-05: `deletion.Schedule` accepts a private chat ID

**What changed.** `Schedule` refuses `ChatID >= 0` with `ErrInvalidEntry` before any read or write (it replaces the `== 0` check).

**Files.** `fuku/db/deletion/queue.go`, `AGENTS.md` (the `scheduled_deletions` Data bullet).

**Test.** `TestScheduleRules` "refused before any write" has a private-chat case, asserts the Staff Group lookup seam was never called, and
that no row exists for that ID. The existing `TestSchedule*` cases already used negative IDs, and no `fuku/modules` test needed a change.

## IN-04: test-only helpers in production code

`deletionRowOf`, `deletionRowOfKind` and `deletionFindRow` had only same-package test callers and moved to
`fuku/modules/deletion_kinds_helpers_test.go`. `deletion.Kinds` and `deletion.IsBotKind` are used by tests of two packages, so they moved to
the `testtools`-tagged `fuku/db/deletion/hook_testtools.go`, together with the `botPrefix` constant only `IsBotKind` used (the production
`kinds` slice stays). A grep before the move found no production caller of any of the five, and AGENTS.md never named them.

## IN-02: `/cleanservice` does nothing inside a Staff Group

One sentence added to the `ServiceDeletion` bullet of AGENTS.md (Handlers): join and leave cleaning does nothing inside a Staff Group
because `deletionAct` refuses every deletion there (D-18, DEL-07), a change from before Phase 16. No code change.

## Deferred (not fixed, by decision)

- **IN-03** (a read-back failure between the claim `UPDATE` and the read in `ClaimDue` strands the claimed rows until the 2 minute stale
  release): accepted. Bounded by `deletionStaleClaim`, already documented ("claimed, until the 2 minute stale release"), and not reproduced
  because it needs a failure between two statements. The orchestrator writes the disposition.

## Verification

All commands run from `/home/user/Fuku_Robot` on the final tree (HEAD 24c766c); real outputs.

- `CGO_ENABLED=0 go build ./...`: exit 0. `go vet -tags testtools ./fuku/modules ./fuku/db/...`: exit 0.
- `git diff --exit-code 8c0c827 -- go.mod go.sum`: exit 0.
- `gofmt -l` over every `.go` file changed since 8c0c827: only `main.go`, which is also unformatted at 8c0c827 and not touched.
- Skip and legacy-name grep over the added Go lines since the review commit (`t.Skip`, `t.Parallel`, `reflect.Ptr`, `ioutil.`, `//nolint`): no match.
- Lint stand-in `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827`, exit read directly, run twice (after the
  last code commit and after the last test commit): exit 0, "no dupl, gocyclo or godox findings on lines added since 8c0c827".
- `make check-docs`: "No drift detected". `make check-translations`: "All translations are present!".
- The whole settings family, in the background after the last production commit:
  `go test -tags testtools -race -count=1 -timeout 30m -run '^(TestSettings|TestLoadSettings)' ./fuku/modules ./fuku/i18n`:
  `ok fuku/modules 474.607s`, `ok fuku/i18n 7.140s`, exit 0 (output in the scratchpad, `settings-family.txt`).
- The Phase 16 families through CI's gate. `go test -tags testtools -json -race -count=1 -timeout 30m -run
  '^(TestDeletion|TestServiceWatcher|TestCleanService|TestApply|TestBot|TestScheduleBot|TestShutdownEndsDeletionWorker|TestSettingsDeletion|TestWarnThisUserSource|TestMemberReqNoticeScheduled|TestMemberReqMuteNoticeAtCap|TestMemberReqFixedReschedule|TestGreet|TestLegacyGreetings|TestRulesLegacy|TestWelcome|TestMemberJoin|TestLeftMember)'
  ./fuku/modules ./fuku/i18n` exited 0 with 128 top-level tests passed, 0 failed, 0 skipped; `go test -tags testtools -json -race
  -count=1 ./fuku/db/deletion ./fuku/db/cache ./fuku/db/greetings ./fuku/db/backup ./fuku/db/migrations` exited 0 (96 top-level tests
  passed, the skips are the PostgreSQL-only migration tests); the two streams concatenated and fed to `go run ./scripts/check_test_results`:
  exit 0. An earlier run of the same command, before the 24c766c test edit, had `TestMemberReqFixedRescheduleMovesUp` red (the WR-03
  fallout above); it is green in the final run.
- Repeat runs: the new and edited tests (`TestDeletionInlineDeadlineFallsBackToQueue`, `TestDeletionWorkerClassifiesErrors`,
  `TestApplyDeleteRuleRemoveDropsPendingRows`, `TestApplyCleanServiceRuleKeepDropsPendingRows`,
  `TestApplyCleanServiceFlagOffDropsPendingRows`, `TestBotManualRepliesNotCovered`) with `-race -count=5`: ok; `./fuku/db/deletion` with
  `-race -count=5`: ok.
- Not run: `make test` and `make lint` (cannot run here, see the brief; the whole `fuku/modules` package was not run, by instruction),
  gosec, and any PostgreSQL proof (no migration or schema changed, the touched SQL is a bound-time `UPDATE`, an `IN` delete and an
  unchanged `attempts + 1`; the `fuku/modules` harness runs on SQLite). Not re-run: tests outside the families above that call
  `RemoveRule`, `ApplyCleanServiceFlag` or `ApplyJoinLeftRule` are all inside them (`deletion_e2e_test.go`, `settings_deletion_test.go`,
  `greetings_command_test.go`, `memberreq_notice_schedule_test.go`, `deletion_bot_test.go`, the `fuku/db/deletion` package).

## Concerns noted, not fixed

- **NEW-01.** WR-03 has a narrow race: a send site that read the rule just before the owner switched it off can write its queue row after
  the drop committed, and that one row is then deleted at its time although the rule is Off. The window is the gap between the rules read
  and the row write (milliseconds), a row is one message, and it is the same class as the existing cache skew; closing it would need the
  worker to re-read the rule per `(chat, kind)` before each call (the reviewer's alternative fix), which also costs a read per chat per cycle.
  Rows queued before this fix deploys, for a rule that is already Off, are likewise left to run (nothing drops them retroactively).
- **NEW-02.** WR-03 changes the owner-visible meaning of the mute notice's 24 hour row: switching `bot:punish_auto` Off now drops it, so
  the notice and its "I fixed it" button stay in the group for ever instead of until the 24 hour cap. That is the safe direction for the
  member (the button stays) and is what Off means, but it is a behaviour change worth knowing; the old test had asserted the opposite and
  was edited (24c766c).
- **NEW-03.** WR-01 closes the loss for the inline deadline only. A single hung Telegram request that runs into the 10 s call timeout
  inside the inline delete is still the plain "any other failure loses that one message" of D-08, and a delete cut off by shutdown is still
  lost by design. A deadline hit after the request left may queue a row for a message Telegram already deleted; the worker drops that row
  quietly on "message to delete not found".
- **NEW-04.** WR-02's `delete_at` update and WR-03's `kind IN ?` delete are exercised on SQLite only (as are the rest of the queue tests
  that are not on the integrity list). I judged neither PostgreSQL-sensitive, so I added nothing to `make test-postgres-integrity`; if
  the owner wants proof, `TestReleaseStaleClaims` and `TestScheduleRules` of `./fuku/db/deletion` are the two to add to that list.
- **NEW-05.** a129472 (the WR-03 GREEN commit) leaves three tests red until the two `test(16-fix)` commits that follow it, 3555ee5
  (`TestBotWelcomeScheduled`, `TestBotGoodbyeScheduled`) and 24c766c (`TestMemberReqFixedRescheduleMovesUp`). Anyone bisecting between
  a129472 and 24c766c can hit them. I did not rewrite history to fold them in.
