---
phase: "16"
slug: "scheduled-deletion-of-service-and-bot-messages"
status: all_fixed
findings_in_scope: 4
fixed: 4
accepted: 2
skipped: 0
source: 16-SECURITY.md (New Threats Found by the Audit: SEC-01 to SEC-05)
red_commits: "f768995, 681061f"
green_commits: "6dae7ef, b45ac0a, 62965b3, 4f689bf, 491bbf2, 85b3b99"
created: "2026-11-12"
---

# Phase 16 - Security hardening pass (SEC-01, SEC-02 parts 1 and 2, SEC-04, SEC-05)

Four findings of the audit in `16-SECURITY.md` were fixed in the orchestrator's order. SEC-01, SEC-02 and SEC-05 went test first (a
RED `test(16-sec)` commit that failed for the stated reason, then GREEN `fix(16-sec)` commits); SEC-04 is docs only. SEC-02 part 3 (a
cap on the shared pacer's waits, AR-1) and SEC-03 (`render.yaml`, AR-2) were not touched, by decision. No `go.mod`, `go.sum`,
migration, handler group, drain, shutdown timing or secret changed; one Redis key was added (below). Nothing was pushed, no history
was rewritten, no requirement was ticked or unticked, `phase.complete` was not run and `16-SECURITY.md` was not edited.

## Commits

| Finding | RED (test) | GREEN (fix) |
|---------|------------|-------------|
| SEC-01 | f768995 | 6dae7ef (+ 85b3b99, a no-behaviour refactor for gocyclo) |
| SEC-02 part 1 (inline latch decision) | 681061f | 4f689bf |
| SEC-02 part 2 (supergroup flag trust) | 681061f | 62965b3 |
| SEC-05 | 681061f | b45ac0a |
| SEC-04 | | 491bbf2 (docs) |

RED 681061f is one commit for the three findings that share `deletion_sec_test.go`; each has its own GREEN commit. No production code
was touched by either RED commit, so neither failure is a compile error. The new Redis key is named in the tests as a literal
(`fuku:delete:chk:<chat>`), which is its documented contract.

What each RED run printed on the then-current code:

- SEC-01: `queue rows = map[5001:{... bot:punish_auto ... 24 h ...} 12002:{... bot:punish_auto ... DeleteAt 05:53 ...}], want exactly
  one (message 5001)` (a valid button on another message queued that message) and `queue row = kind bot:punish_auto delete_at
  05:53:08, want ... 2026-11-13 05:23:08` (a hold with no stored notice ID still rescheduled the pressed message). The positive case
  (the real notice is rescheduled) already passed.
- SEC-02 part 1: `latch = "", want "service"` and `live reads of the bot = 0, want exactly 1 for four refusals`. The controls (a bot
  kind, no Redis, a Redis error) already passed.
- SEC-02 part 2: `latch = "", want "service"` for a supergroup ID and for the first supergroup ID (-1000000000000); the basic-group
  and positive ID cases already passed.
- SEC-05: `deleteMessage calls = 1, want none` (delay 0) and `Staff Group reads = 1, want none` (both delays) for chat IDs 0, 4242424
  and 1234567890123.

## SEC-01 (low): "I fixed it" rescheduled whatever message was pressed

**What changed.** `memberReqFixedRun` (`fuku/modules/memberreq_fixed.go`) passes `memberReqFixedEdit` a new `reschedule` flag, true only
when `memberReqIsOwnNotice(taken, pressedID)` holds: the taken hold's `notice_msg_id` is positive and equals the pressed message's ID.
`memberReqFixedEdit` calls `scheduleBotMessageFromNow` only when the flag is set and the edit succeeded. When the pressed message is
another message of the chat, or the hold never stored its notice ID, nothing is scheduled and no row is written; the unmute, the answer,
the hold take and the edit are exactly as before, and the Phase 13 rule that the notice ID is never compared for the unmute is
untouched (it is compared for the reschedule alone). The real notice's 24 hour row stays.

**Files.** `fuku/modules/memberreq_fixed.go`, `AGENTS.md` (the "Scheduling the bot's own messages" bullet and step (10) of the
`mreq|v1` bullet).

**Tests** (`TestMemberReqFixedReschedulesOnlyTheNotice`, `memberreq_notice_schedule_test.go`): a valid button pressed on another message
of the same chat unmutes once, answers once, edits the pressed message once, does not edit the real notice and leaves exactly the real
notice's untouched 24 hour row (no row for the pressed message); the press on the real notice reschedules it from the tap; a hold whose
`notice_msg_id` was cleared edits the notice and leaves its 24 hour row. `TestMemberReqFixedRescheduleMovesUp` and the other fixed
families passed unchanged.

## SEC-02 (low): one misconfigured group could use up the fleet-wide deletion budget

**Part 1: the inline latch decision.** After an inline `service:*` delete is refused with a message-level answer
(`deletionVerdictRefused`), `deletionInlineSettle` now calls `deletionInlineDecideLatch` (`deletion_schedule.go`). It does nothing for a
bot kind. Otherwise it takes the guard `fuku:delete:chk:<chat>` with `SET NX EX 600` (`deletionTakeCheck`; no Redis, a Redis error and
a lost `SET NX` all mean no check) and then runs the worker's own `deletionDecideServiceLatch`, so only the classifier's live `cannot`
answer writes the `service` latch and drops the chat's pending service rows. It runs inside the inline delete's goroutine (which
already starts with `defer error_handling.RecoverFromPanic` and is counted in `deletionInline`) on a context derived from
`deletionContext()` with the inline timeout, so the worker's stop cancels it. The guard is taken whatever the check then answers
(can, cannot or unknown), so one chat costs at most one live read per 10 minutes across replicas. The key sits in the `fuku:delete:*`
family, outside the `fuku:cache:` prefix; `AGENTS.md` documents it in the Data section, in the `fuku:delete:norights` bullet (a refusal
no longer "never" leads to a latch: it leads to the live check) and in the "Scheduled deletion core" bullet.

**Part 2: the worker trusts the administrator flag in supergroups.** `deletionDecideServiceLatch` (`deletion_right.go`) now passes
`deletionTrustAdminFlag(chatID)` to the reader: true for `chatID <= -1000000000000` (Bot API supergroup and channel IDs), false for a
basic group's ID (-1 to -999999999999) and for any other ID. An administrator bot without `can_delete_messages` therefore classifies as
`cannot` in a supergroup and the latch is set; basic groups keep the old distrust. The settings page's own read is unchanged (it trusts
the flag by the chat type it holds). `deletionClassifyBotRight`'s signature and table are unchanged. `AGENTS.md`'s worker bullet no
longer says the worker never trusts the flag; it states the ID-range rule and its residual cost for basic groups.

**Tests** (`deletion_sec_test.go`): `TestDeletionInlineRefusalDecidesLatch`: a plain-member bot is latched after one refused inline pin
with exactly one live read and later pins cost no call and no read; an unknown answer (500) latches nothing and four refusals make one
read, the guard has a TTL within (0, 10 min], and after `FastForward(11m)` the next refusal reads again; a bot kind (`deletionAct` with
`bot:welcome`, delay 0) refused makes no read, no latch and takes no guard; with no Redis and with a failing Redis client no read is
made and the one refused call is still made. `TestDeletionWorkerTrustsAdminFlagInSupergroups` (worker, administrator without the
right, three refused service rows): a supergroup ID, -1000000000000 -> latched `service`, one live read, no rows left; -999999999999,
-4242424 and 4242424 -> no latch, one read, no rows left. Each test uses a fresh miniredis, fresh IDs from `uniqueModuleChatID()` and
`deletionCleanup`; the clock moves only through `deletionNow` and miniredis `FastForward`.

**Existing test edited (one subtest, named).** `TestDeletionServiceLatchNeedsLiveCheck`, subtest "an administrator without the right is
not latched, the flag is not trusted" (`deletion_right_test.go`), encoded the old worker behaviour for a supergroup ID (the test chat
IDs are all at or below -1000000000000). It is now "an administrator without the right is latched in a supergroup, the flag is trusted":
latch `service`, one live read, one single call (the other service rows are dropped with no call), no rows left. The basic-group half
of the old assertion moved to the new test above. Edited in the GREEN commit 62965b3. No other test needed a change:
`TestDeletionBotRightClassifier` and `TestDeletionReadBotRight` pass unchanged because the classifier is unchanged.

**Part 3 (accepted, AR-1).** The shared pacer and the cap pacers' waits were not changed.

## SEC-04 (info): handler-groups page said `message.All`

`docs/src/content/docs/architecture/handler-groups.md`, group `-5` row: the filter column now reads "`serviceDeleteFilter` (a deletable
service message type) with `SetAllowBot`". The page is not frozen and the generator does not own the line, so it was edited by hand
(`make check-docs`: no drift before and after). Everything else on the page is as before. Commit 491bbf2.

## SEC-05 (info): the inline branch of `deletionAct` had no ID check

`deletionAct` returns first for `a.ChatID >= 0`, before the latch read, the Staff Group read, the inline delete and the queue write, so
both branches refuse a zero or positive chat ID (symmetric with `deletion.Schedule`, IN-05). `AGENTS.md`'s "Scheduled deletion core"
bullet says so. Test `TestDeletionActRefusesNonNegativeChat`: chat IDs 0, 4242424 and 1234567890123, delay 0 (inline) and 600 (queue):
no `deleteMessage` request, no row, no Staff Group read. Commit b45ac0a.

## New concerns (for the orchestrator to disposition)

- **HN-01 (low, recorded, not acted on): the inline guard is taken even when the live check is unknown.** A 429, a 5xx, a timeout or a
  shed pacer at the one live read leaves no latch and the next try waits the 10 minutes, so a transient fault on that read costs one
  window of refused calls (the pre-pass cost, bounded by the pacer and the 64 inline goroutines). Giving the guard back on unknown
  would reopen an unbounded loop during a Telegram outage, which the brief's "fail quiet" forbids, so I kept it and documented it in
  `AGENTS.md`. Release on unknown with a short retry is a possible refinement.
- **HN-02 (low, residual of SEC-02 part 2): a basic group whose administrator bot lacks the delete right is still never latched.** The
  flag stays distrusted there (the worker has no chat type), so each refused service delete still costs one paced call; the inline
  decision's live read answers `can` once per 10 minutes and changes nothing. The amplifier is smaller than for a supergroup only
  because basic groups are small and the bot leaves non-supergroups on join (`botJoinedGroup`); I did not verify that this covers every
  group that exists today.
- **HN-03 (info): rolling deploy.** A replica on the previous code has neither the guard nor the supergroup trust, so during a deploy
  it behaves as before. Same class as the other rolling-deploy caveats; no action.
- **HN-04 (info): an existing test name overstates.** `TestDeletionLatch`'s subtest "the inline path latches a gone chat and never
  latches a refused message" still passes (the bot there can delete, so the live check answers `can` and nothing is latched) but a
  refused inline pin now makes one live read. The name was left alone to avoid churn.

## Verification (real results)

All from `/home/user/Fuku_Robot` on the final code (HEAD `85b3b99`; this report is the only later commit).

- `CGO_ENABLED=0 go build ./...`: exit 0. `go vet -tags testtools ./fuku/modules ./fuku/db/...`: exit 0.
- `git diff --exit-code 8c0c827 -- go.mod go.sum`: exit 0.
- `gofmt -l` over every `.go` file changed since the audit base `6e02f16`: no output.
- Added-lines grep (`t.Skip`, `t.Parallel`, `reflect.Ptr`, `ioutil.`, `//nolint`) over `git diff 512b46f.. -U0 -- '*.go'`: no match.
- Lint stand-in `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827`: the first run exited 1 with
  `gocyclo: 20 modules memberReqFixedRun` (my SEC-01 condition pushed it over 19); fixed by the extraction in 85b3b99; the rerun,
  exit read directly (`echo lint-exit=$?` right after the command, output to a file): **exit 0**, "no dupl, gocyclo or godox findings on
  lines added since 8c0c827".
- `make check-docs`: "No drift detected" (run after the docs edit and again at the end). `make check-translations`: "All translations
  are present!". No locale key was added or changed.
- The whole settings family, run in the background after the last behaviour commit (4f689bf, before the docs and refactor commits that
  followed): `go test -tags testtools -race -count=1 -timeout 30m -run '^(TestSettings|TestLoadSettings)' ./fuku/modules ./fuku/i18n`:
  `ok fuku/modules 485.509s`, `ok fuku/i18n 6.929s`. The only production change after it is the no-behaviour extraction 85b3b99 and a
  docs line; the fixed and channel-inert families were re-run after it (below).
- The Phase 16 families through CI's gate, on HEAD `85b3b99`: `go test -tags testtools -json -race -count=1 -timeout 30m -run
  '^(TestDeletion|TestServiceWatcher|TestCleanService|TestApply|TestBot|TestScheduleBot|TestShutdownEndsDeletionWorker|TestSettingsDeletion|TestWarnThisUserSource|TestMemberReqNoticeScheduled|TestMemberReqMuteNoticeAtCap|TestMemberReqFixed|TestMemberReqChannelInert|TestMemberReqNotice|TestMemberReqHeldRelease|TestGreet|TestLegacyGreetings|TestRulesLegacy|TestWelcome|TestMemberJoin|TestLeftMember)'
  ./fuku/modules ./fuku/i18n` (exit 0) plus `go test -tags testtools -json -race -count=1 ./fuku/db/deletion ./fuku/db/cache
  ./fuku/db/greetings ./fuku/db/backup ./fuku/db/migrations` (exit 0), the two streams concatenated and fed to
  `go run ./scripts/check_test_results`: **exit 0**; 251 top-level tests passed, 0 failed, 18 skipped (the PostgreSQL-only migration
  tests, which the gate allows). `TestMemberReqFixed` here includes `TestMemberReqFixedRescheduleMovesUp` and the new
  `TestMemberReqFixedReschedulesOnlyTheNotice`; `TestMemberReqNotice` and `TestMemberReqHeldRelease` were run as the brief asked.
- Repeat run: the four new top-level tests (`TestDeletionInlineRefusalDecidesLatch`, `TestDeletionWorkerTrustsAdminFlagInSupergroups`,
  `TestDeletionActRefusesNonNegativeChat`, `TestMemberReqFixedReschedulesOnlyTheNotice`) with `-race -count=5`: ok (31.9 s).

## Not run

`make test` and `make lint` (cannot run here, per the brief), the unfiltered `fuku/modules` suite, gosec, any PostgreSQL proof (no
migration, schema or SQL changed; the new key is Redis only), and `make test-postgres-integrity` (nothing in this pass rests on a
partial index, CHECK or conditional update). Telegram's behaviour for SEC-01 (whether it checks callback data against a message's
keyboard) remains unverified live, as in the audit; the fix is correct under either answer.
