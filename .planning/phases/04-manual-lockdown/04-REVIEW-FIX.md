---
phase: 04-manual-lockdown
fixed_at: 2026-10-07T04:25:00Z
review_path: .planning/phases/04-manual-lockdown/04-REVIEW.md
iteration: 1
findings_in_scope: 3
fixed: 3
skipped: 0
status: all_fixed
---

# Phase 04: Code Review Fix Report

**Fixed at:** 2026-10-07T04:25:00Z
**Source review:** .planning/phases/04-manual-lockdown/04-REVIEW.md
**Iteration:** 1

**Summary:**
- Findings in scope: 3
- Fixed: 3
- Skipped: 0

Scope was Critical and Warning only (CR-01, WR-01, WR-02). IN-01 to IN-04 were not attempted. Work was done directly on branch `claude/cool-hamilton-hvs9nl`, with no worktree (as instructed). Each fix is one commit with its regression test.

## Fixed Issues

### CR-01: The worker's ban overwrites a deliberate ban, and the lift then removes it (violates D-05)

**Files modified:** `alita/modules/lockdown_worker.go`, `alita/modules/lockdown_ban_live_test.go`, `AGENTS.md`
**Commit:** 005c802
**Status:** fixed: requires human verification (logic change)
**Applied fix:** `lockdownBanOne` now reads the joiner's live `getChatMember` in the same paced unit as the ban.
- Kicked on a date other than the row's `ban_until`: the row moves `acting -> kept` with no `banChatMember` call, so the lift never touches it.
- Kicked on the row's own `ban_until`: that is the lockdown's ban whose answer was lost, so it is recorded `banned` and no second ban is sent.
- Status unreadable: treated as a failed call (attempt counted, row back to pending). It never turns into a ban. This differs from the review's snippet, which banned anyway when the lookup failed, because that would re-open the hole.
- AGENTS.md updated (join guard / worker bullet).

Regression tests (`lockdown_ban_live_test.go`):
- A permanent ban and a `/tban`-style ban placed between `RecordJoin` and the worker cycle: no ban call, row `kept`, no unban at `/unlockdown`, the member is still kicked on its original date, and a second ordinary raider is still unbanned.
- Own ban already in place: recorded `banned` and unbanned at the lift.
- Unreadable status: no ban call, one attempt counted, banned on the next cycle.

Verified to fail on the pre-fix worker (all four subtests) and pass after.

Residual: a ban placed in the instant between the live look-up and the `banChatMember` call cannot be excluded through the Bot API (no conditional ban). The window is two back-to-back calls instead of the whole queue wait.

### WR-01: An ambiguous lock failure deletes the only copy of the pre-lockdown permissions and says "nothing changed"

**Files modified:** `alita/modules/lockdown.go`, `alita/modules/lockdown_fake_test.go`, `alita/modules/lockdown_lock_unknown_test.go`, `locales/{en,es,fr,hi,id,pt,ru}.yml`, `AGENTS.md`
**Commit:** 8749ab9
**Status:** fixed: requires human verification (logic change)
**Applied fix:** a new `lockdownDefinitiveRefusal` treats only a `*gotgbot.TelegramError` with a 4xx code as a refusal. For that, the unconfirmed row is deleted and `lockdown_lock_failed` is shown, as before. For anything else (timeout, dropped connection, unreadable answer, 5xx) the row is kept unconfirmed and `/lockdown` answers with the new `lockdown_lock_unknown` key, added to all 7 locales with `{detail}`. The worker's existing `settleUnconfirmedLockdown` then confirms or drops the row from the live permissions, and `/unlockdown` can restore from the kept snapshot at once. A 5xx is treated as ambiguous rather than refused (a deliberate narrowing of the review's "any `*gotgbot.TelegramError`"), because Telegram does not promise a 5xx means "not applied".

Regression tests (`lockdown_lock_unknown_test.go`; the fake gained `setFailAfterSetPermissions`, which applies the permissions and then fails):
- Lock applied then timed out: row kept unconfirmed with its snapshot, `lockdown_lock_unknown` shown, `/unlockdown` restores the exact pre-lockdown permissions.
- Same, then the worker settles after the grace period: row confirmed.
- 502 before the lock applied: row kept, then dropped by the worker, permissions unchanged.

Verified to fail on the pre-fix `lockdown.go` and pass after. The existing 400 "lock call refused" refusal test still passes.

### WR-02: Joiner bans expire after 330 days, so a lockdown can outlive its own enforcement

**Files modified:** `alita/modules/lockdown_worker.go`, `alita/modules/lockdown_marker.go`, `alita/db/models/lockdown.go`, `alita/db/lockdown/joiners.go`, `alita/db/lockdown/joiners_lift_test.go`, `alita/modules/lockdown_expired_test.go`, `locales/{en,es,fr,hi,id,pt,ru}.yml`, `docs/src/content/docs/commands/lockdown/index.md` (generated), `AGENTS.md`
**Commit:** d14718f
**Status:** fixed: requires human verification (logic change)
**Option taken:** (a) plus (c) of the review: document the 330-day ceiling as an accepted limit, and count a lapsed ban as "expired" in the tally. Not (b), the re-ban machine. Re-banning would add a new write path, a new claim/expiry state and more Telegram traffic on a group nobody is looking at, for a lockdown the owner decided never ends on its own but which in practice is lifted long before 330 days. The smaller fix removes the untrue statement and tells the admins about the limit.
**Applied fix:**
- At the lift, a joiner who is `left` with a `ban_until` in the past (`lockdownBanExpired`) is moved to `kept` with detail `ban_expired` (`models.JoinerDetailBanExpired`). The `chk_chat_lockdown_joiner_state` CHECK has no state for it and migrations are append-only, so a new state would need a migration; the detail marker avoids that.
- `lockdown.CountJoinersWithDetail` counts them, and the tally posts them under the new `lockdown_lift_tally_expired` text (all 7 locales), apart from the kept rows someone unbanned early or banned on purpose. It no longer says they were left on purpose.
- The `lockdown_help_msg` text (all 7 locales, and the generated docs page) now states that joiner bans last 330 days and that a longer lockdown lets them lapse. AGENTS.md documents the limit and the marker.

Regression tests: `TestLockdownLiftReportsExpiredBans` (expired ban reported as expired, early unban still reported as kept, live ban still unbanned, no unban call for the lapsed one; fails on the pre-fix worker) and `TestCountJoinersWithDetail` (DB layer, scoped to one lockdown, state and detail).

Known gaps, left as is: a person who was unbanned by hand before day 330 and is checked after day 330 is reported as expired, which is still true of the ban's end date. A lockdown lifted when only kept rows exist finishes inside `/unlockdown` and posts no tally (pre-existing behaviour, not part of the finding).

## Skipped Issues

None. IN-01 to IN-04 are out of scope for `fix_scope: critical_warning`.

## Verification

Ran in the main checkout on branch `claude/cool-hamilton-hvs9nl`, not a worktree.

- After each fix: `go test -tags testtools -race -count=1 -run 'Lockdown|Joiner|StaffAction' ./alita/modules ./alita/db/lockdown` passed; `CGO_ENABLED=0 go build ./...` and `go vet -tags testtools ./alita/modules ./alita/db/lockdown` clean; `gofmt -l` clean on every touched file.
- `make check-translations`, `make generate-docs` and `make check-docs` pass (no drift) after the two locale-changing fixes.
- Each new test was shown to fail with the fix reverted (`git stash` of the production file only) and pass with it.
- `make test` once at the end: exit 0, no `FAIL` lines, coverage 70.1%. The known flaky `TestStaffActionNonStaffUnchanged/tmute_as_a_reply` did not fail.
- `make lint` was not run: golangci-lint is built with go1.25 and the module is go1.26.0. `gofmt -l alita` reports only `alita/modules/greetings_command_test.go`, which this phase did not touch and is not part of these fixes.

---

_Fixed: 2026-10-07T04:25:00Z_
_Fixer: Claude (gsd-code-fixer)_
_Iteration: 1_
