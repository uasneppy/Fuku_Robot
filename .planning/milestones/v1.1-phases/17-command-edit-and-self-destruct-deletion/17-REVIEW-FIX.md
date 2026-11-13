---
phase: 17-command-edit-and-self-destruct-deletion
review: 17-REVIEW.md
security: 17-SECURITY.md
status: all_fixed
findings_in_scope: 9
fixed: 9
skipped: 0
fixed_at: 2026-11-12
---

# Phase 17: Review Fix Report

Fix pass for WR-01, WR-02, WR-03, IN-01, IN-02, IN-03 of `17-REVIEW.md` and SEC-17-01 to SEC-17-05 of `17-SECURITY.md`
(SEC-17-01 is WR-01's log half and SEC-17-02 is WR-02, so the nine items are six review findings plus SEC-17-03, SEC-17-04
and SEC-17-05), run sequentially on branch `claude/ecstatic-goldberg-qsss05`. Nothing was pushed, no history was rewritten,
`go.mod` and `go.sum` are unchanged since 8c0c827, no requirement was ticked or unticked, `phase.complete` was not run, no
migration, Redis key family or shutdown drain was added. The root-pin guard exited 0 before the first edit and before every
commit.

Every behaviour fix is one `fix(17)` commit that carries its test (the fix needs new seams, so a separate red commit could
not compile). Each test was proved red by switching the fix off in the working tree (the production lines temporarily
short-circuited, then restored), never by a compile error; the failure text is quoted below.

## Commits

| Item | Commit | Subject |
|------|--------|---------|
| WR-01 + SEC-17-01 | b53cd8b | fix(17): warm the admin list once and throttle member deletion failure logs |
| WR-01 follow-up | 1ceae78 | fix(17): gate member deletion failure lines per kind and keep the cold-cache e2e cases cold |
| IN-01 | b8864e0 | fix(17): throttle the worker's unreadable-rules line to once per chat per hour |
| WR-02 + SEC-17-02 | ccd711d | fix(17): shed a latched chat's self-destruct entries in the batcher |
| WR-03 | c8c7022 | fix(17): keep a message pinned in the batcher buffer out of the queue and retry the pin drop |
| IN-03 | afcba8c | fix(17): skip the pin drop in a chat with no self-destruct rule |
| IN-02 | fc42977 | fix(17): give the batcher's write its own bounded context |
| SEC-17-03 | 7bdffab | docs(17): scope the member rule exemptions to Edited messages and Self-destruct |
| SEC-17-05 | 62ad877 | docs(17): say a claimed and screened batch can finish after Self-destruct is turned off |
| SEC-17-04 | 4f297a9 | docs(17): record the rolling-deploy caveat of the member deletion kinds |

## Fixed Issues

### WR-01 and SEC-17-01: cold admin cache cost a live read per message and never warmed; failure lines logged per message

**Files:** `fuku/modules/member_deletion.go`, `fuku/modules/member_deletion_batcher.go` (the gate type only), `AGENTS.md`.
**Commits:** b53cd8b, 1ceae78.
**Applied fix.** `memberDeletionAdminStatus` now, after the cached list says unknown, takes the same semaphore and 3 s timeout and
warms the list once through `cache.LoadAdminCache` (`memberDeletionAdminWarm`, wrapped in its own singleflight keyed by chat that
re-reads the cache first, so concurrent messages and a flight that starts right after another share one `getChatAdministrators`),
re-reads `cachedAdminStatus`, and only when the cache still cannot say makes the per-user `getChatMember`. The warm is skipped while
the cache holds any entry (a negative or empty one is stored for its own TTL; asking again would repeat the bot's own
`getChatMember` per message) and for 30 s after a load that stored nothing (`memberDeletionWarmBackoff`, bounded at 10,000 chats), so
a Telegram incident costs one load per chat per window. The load itself cannot be cancelled, so it runs on a goroutine that recovers
from panics and the caller waits only until its own timeout; a slow answer still warms the cache for the next message. Unknown still
keeps the message (D-33). The four failure lines (saturated, admin status unknown, approvals unknown, captcha unknown for chat and
for user) go through `memberDeletionWarnf` and an hourly per-chat gate, the batcher's `memberDeleteWarnGate` type (given a window and
a label field, zero values unchanged), IDs and the error class only.
**Deviation found by the whole-package run (the lesson of this phase).** The first commit used one gate per chat, so the first
failure line hid the others and `TestMemberDeletionLogsCarryIDsOnly` failed (it drives each failing seam and wants its line);
`TestMemberDeletionEndToEnd`'s "cold admin cache whose live read answers 429" case also failed, because a list that can now be loaded
answers the check. Follow-up 1ceae78 gives each line kind its own gate (`memberDeletionWarnGates`) and makes that e2e case, and the
sweep's `cold()` helper, keep the cache cold through the `memberDeletionAdminWarm` seam (a no-op: the list cannot be loaded and the
live read fails, so unknown keeps the message). `coldAdminCache()` of the member deletion toolkit does the same; the new
`warmableAdminCache()` leaves the loader live. No assertion was weakened: the cases still prove "unknown never deletes", now with the
list that cannot be loaded as the stated precondition.
**Tests.** `TestMemberDeletionAdminWarm` (`member_deletion_admin_warm_test.go`): five messages behind a cold cache make one
`getChatAdministrators` and no live member read, a later administrator is known from the warmed list; six concurrent checks share one
load; a list that cannot be loaded falls back to the live read, is not asked again inside the backoff, is asked again after it, and a
failing live read is unknown; a saturated semaphore keeps five messages and logs one line, another after the window; five failing
approvals reads log one line that carries no message text.
**Red:** `getChatAdministrators calls = 0, want 1`; `list loads = 0, want 1`; `saturated lines = 5 for five messages, want 1`; `lines
for five failing reads = 5, want 1`.

### IN-01: unreadable rules made the worker log a Warn per chat per second

**Files:** `fuku/modules/deletion_worker.go`, `AGENTS.md`. **Commit:** b8864e0.
**Applied fix.** The `screenSelfDestruct` line goes through an hourly per-chat gate (`deletionScreenWarn`); the free release, the
rows put back and the attempt count are unchanged.
**Test.** `TestDeletionWorkerScreenLogThrottled`: six cycles with the rules unreadable log one line and leave the rows pending with
no attempt; one more line after the window. **Red:** `lines for six cycles with the rules unreadable = 6, want 1`.

### WR-02 and SEC-17-02: the batcher ignored the delete-right latch

**Files:** `fuku/modules/member_deletion_batcher.go`, `AGENTS.md`. **Commit:** ccd711d.
**Applied fix.** `memberDeleteScreenChat` reads `deletionLatched(ctx, chat, KindMemberSelfDestruct)` once per chat per flush, after
the rule check and before the count, and sheds the chat's entries under a new counted reason `no delete right` (the messages stay,
one Warn per chat per hour like every shed). `deletionLatched` already treats no Redis and a Redis error as "not latched", so a
failing latch read writes as before. A chat with the rule off is dropped as off before the latch is read.
**Test.** `TestMemberSelfDestructBatcherLatch`: a service-scope latch leaves no row and an unlatched chat in the same flush writes
its two; the gone scope stops the chat; a broken Redis client writes as before; an off chat is dropped as off, not as latched.
**Red:** `memberDeleteFlush() = 5, want 2 (only the unlatched chat's entries)`; `memberDeleteFlush() = 2, want 0`.

### WR-03: a pin could lose the race with the batcher buffer, and a failed drop was only logged

**Files:** `fuku/modules/member_deletion.go`, `fuku/modules/member_deletion_batcher.go`, `AGENTS.md`. **Commit:** c8c7022.
**Applied fix.** `memberDeletePinned` first notes the pin in a bounded in-process set (`memberPinnedNotes`: 10 s TTL, 4096 keys via
the `memberPinnedMax` variable, expired notes pruned on every write, the oldest evicted when full so the newest pin is always kept,
clock `deletionNow`). The flush consults it right before its write (`memberDeleteDropPinnedEntries`), not only when it takes the
buffer: that is a superset, because it also covers a pin that lands during the screening of up to 5 s. Dropped entries are counted
(`memberDeletePinnedDropped`) as correct, not a shed. The database drop then runs through the `memberDeleteDropPinned` seam and is
retried once on a fresh context. This closes the buffer window on the replica that handles the pin only; a pin handled by another
replica stays the accepted limit AR-5, and AGENTS.md says so.
**Tests.** `TestMemberSelfDestructPinRace`: offer, pin through the real dispatcher, flush leaves no row for the pinned message and
writes the other; the bot's own pin reaches the buffer and the note expires after the TTL; the set stays bounded and prunes on
write; a failed drop is attempted exactly twice. **Red:** `memberDeleteFlush() = 2, want 1`; `want 0 for a message pinned a moment
ago`; `drop attempts = 1, want 2`.

### IN-03: every pin service message cost a synchronous database write in group -8

**Files:** `fuku/modules/member_deletion.go`, `AGENTS.md`. **Commit:** afcba8c.
**Applied fix.** `memberDeletePinned` reads the cached rules first (`memberSelfDestructRuleStored`, `deletionRulesLookup`) and
returns, with no note and no database call, when the chat holds no `member:selfdestruct` entry. An unreadable rule counts as stored,
so the old behaviour (attempt the drop) is kept.
**Test.** `TestMemberDeletePinnedNeedsRule`: no rule means no drop call and no note (also through the dispatcher with a service
message); a rule on means one call; an unreadable rule means one call. **Red:** `drop calls for a chat without the rule = 2, want 0`.

### IN-02: one 5 s context covered screening and write

**Files:** `fuku/modules/member_deletion_batcher.go`, `AGENTS.md`. **Commit:** fc42977.
**Applied fix.** The write runs on `memberDeleteWriteContext` (`context.WithTimeout(context.WithoutCancel(flush ctx),
memberDeleteWriteTimeout)`, 5 s). `memberDeleteFlush` keeps its signature (every existing test and caller unchanged) and delegates to
`memberDeleteFlushWith`; the final flush of a stop passes its own one-context writer, so the stop still never waits longer than its
2 s and the 3 s shutdown arithmetic is untouched. An empty kept set makes no write context.
**Test.** `TestMemberDeleteFlushWriteOwnContext`: the count seam returns only when the flush deadline ran out, then the write seam
must see a live context with its own deadline and the real `ScheduleBatch` must write both rows. **Red:** `memberDeleteFlush() = 0,
want 2 written although screening used the whole budget`.

### SEC-17-03: README overstated the exemptions

**Files:** `README.md`. **Commit:** 7bdffab. The sentence now says Edited messages and Self-destruct leave admins' and approved
members' messages alone, Self-destruct also keeps pinned messages, a message whose sender cannot be checked is kept, and Commands
has no exemption (an admin's command is deleted too). `docs/` holds no copy: its handler-group row describes the two handlers that do
exempt, and the other `docs/` hits are antiflood, locks and approvals pages of other features. Docs only, no test.

### SEC-17-05: "nothing is deleted after Off except a call already in flight" was too strong

**Files:** `AGENTS.md` (the worker bullet), `fuku/modules/deletion_worker.go` (the `screenSelfDestruct` comment),
`.planning/phases/17-command-edit-and-self-destruct-deletion/17-SPIKE.md` (D-26), `README.md` (the Off sentence, in 7bdffab).
**Commit:** 62ad877. They now say a row the cycle has not yet claimed and screened is never deleted after Off, and a batch the worker
had already claimed and screened can finish within its cycle (up to the 90 s guard), like Phase 16's accepted NEW-01. The planning
history files (17-CONTEXT D-12 and D-26, 17-RESEARCH C-06, the plans) keep their original wording on purpose: they record the
decision as made. Docs and one code comment, no test.

### SEC-17-04: no rolling-deploy caveat for the member kinds

**Files:** `AGENTS.md`. **Commit:** 4f297a9. One sentence in the self-destruct Subsystem traps bullet: until every replica runs the new
code, an old-code worker claims `member:*` rows without the self-destruct Off screen and with a service-only latch; transient. Docs
only, no test.

## Skipped Issues

None. No finding turned out to be wrong and no fix broke a decision D-01 to D-34 (D-33 "unknown never deletes" is the invariant every
change was tested against).

## Verification

Run on the final tree (HEAD after 4f297a9), on SQLite for `fuku/modules` as always, with `-tags testtools -race -count=1`:

- Focused families: `go test -tags testtools -race -count=1 -timeout 30m -run '^(TestMember|TestSelfDestruct|TestDeletion|TestCommand|TestShutdown|TestSettingsDeletion|TestSettingsStaffPrivacy|TestSettingsGateAllDoors)' ./fuku/modules`
  result `ok  github.com/uasneppy/fuku_robot/fuku/modules  611.931s`, exit 0.
- `go test -tags testtools -race -count=1 ./fuku/db/deletion` result `ok  github.com/uasneppy/fuku_robot/fuku/db/deletion  1.239s`.
- Whole package, once, in the background: `go test -tags testtools -race -count=1 -timeout 30m ./fuku/modules` result `ok
  github.com/uasneppy/fuku_robot/fuku/modules  1585.637s`, exit 0.
- Lint stand-in `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827`: exit 0 (read directly), "no dupl,
  gocyclo or godox findings on lines added since 8c0c827"; `go vet -tags testtools ./fuku/modules` clean; `gofmt -l` clean on every
  file touched (the other files `gofmt -l` lists were already unformatted and are not mine); `make check-docs`: no drift;
  `make check-translations`: all present; `git diff --exit-code 8c0c827 -- go.mod go.sum`: no change.
- Not run: `make lint` (golangci-lint cannot run on Go 1.26.0 here), `make test` as a whole, `make test-postgres-integrity` (nothing
  of this pass touches SQL, a migration, an index or a CHECK), live Telegram checks (`17-SPIKE.md`).

The first whole-run of the `TestMember|TestSelfDestruct` family, started after the WR-01 commit, failed two tests that no focused run
of WR-01 had touched (see WR-01, deviation); both are fixed by 1ceae78 and pass in the focused and whole runs above.

## Notes for the owner

- Test toolkit edits, named: `coldAdminCache()` (`member_deletion_env_test.go`) now also makes the shared loader's warm a no-op;
  `warmableAdminCache()` and `resetMemberDeletionFailureState()` are new; `resetMemberDeleteState()` also clears the pinned notes and
  the pinned-drop counter; the e2e sweep's `cold()` helper and its 429 case (`member_deletion_e2e_test.go`) keep the cache cold.
- Behaviour change worth knowing: in a group where the bot is an administrator, the first non-exempt message after the admin cache
  expires now costs one `getChatAdministrators` (plus the bot's own `getChatMember`) instead of one `getChatMember` per message.
- The per-kind failure gates and the warm backoff are in memory and per replica, like the shed log gate.

---

_Fixed: 2026-11-12_
_Fixer: Claude (gsd-code-fixer)_
_Iteration: 1_
