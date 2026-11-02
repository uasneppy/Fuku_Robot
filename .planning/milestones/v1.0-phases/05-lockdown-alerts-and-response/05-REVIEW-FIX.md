---
phase: 05-lockdown-alerts-and-response
fixed_at: 2026-10-18T00:00:00Z
review_path: .planning/phases/05-lockdown-alerts-and-response/05-REVIEW.md
iteration: 1
findings_in_scope: 12
fixed: 12
skipped: 0
status: all_fixed
---

# Phase 5: Code Review Fix Report

**Scope:** all 12 findings (3 warnings and 9 info).
**Result:** all 12 fixed, each in its own commit. Two pairs of closely related findings share a commit (IN-02 with IN-07, IN-06 with IN-09).

## Fixed Issues

### WR-01: Ban run result lost when the first delivery fails
**Commit:** 66b588c

A finished ban run's result is now delivered by its own worker step, which resumes from the database after a failure or restart:
- Migration `20261011160000_add_lockdown_ban_run_report.sql` adds the columns that track delivery.
- `lockdownReportBanRuns` claims one run at a time, so only one replica delivers it.
- The card edit and the `#LOCKDOWN_BAN` post are separate steps, each recorded once, so a delivered step is never sent again.
- A rate limit or shutdown gives the claim back without using an attempt; a step is given up after 3 failed attempts.
- A claim abandoned by a stopped worker is released by `lockdownReleaseStale`.

Covered by regression tests.

### WR-02: Permanent ban possible after a lift started during the paced wait
**Commit:** d2a4c7c

`lockdownResponseBanOne` now reads the lockdown again inside the paced unit, right before `banChatMember`. If the lockdown is no longer active, it puts the joiner row back, returns the account to the queue without using an attempt, and sends no ban. Covered by a regression test.

### WR-03: PostgreSQL integrity list missing the new conditional-update guarantees
**Commit:** 8d49660

These tests were added to the `-run` list of `test-postgres-integrity`:
- `TestRecordLinkRevoked`
- `TestReminderSlots`
- `TestUnqueueBanRun`
- `TestAlertSync`
- `TestAlertSyncKeepsAMarkSetDuringTheEdit`
- `TestAlertClose`
- `TestBanRunReport`

The AGENTS.md Testing bullet was updated to match. These run on PostgreSQL in CI only; there is no PostgreSQL here.

### IN-01: Staff copy posted without re-checking the link
**Commit:** fe835d6

Before a staff copy is sent, the group's link is read fresh:
- No longer linked, or linked to another Staff Group: the copy is failed with `no longer linked` and nothing is sent.
- The link cannot be read: the copy stays pending without using an attempt.

Covered by a regression test.

### IN-02: Misleading refusal after losing a revoke race
**Commit:** 9d522be

When Telegram refuses a revoke, the lockdown is read again. If another admin's revoke was already recorded, the admin sees `lockdown_revoke_already`. Covered by a regression test.

### IN-03: Lift confirm text says everyone removed is unbanned
**Commit:** 0f9d828

`lockdown_alert_lift_ask` now says, in all 7 locales, that accounts banned for good stay banned.

### IN-04: Failed refreshes use up a copy's close attempts
**Commit:** 220dac7

When `ClaimAlertClose` claims a copy still marked `edit_pending`, it resets `attempts` to zero and clears the mark. Covered by a regression test.

### IN-05: Dead `SetBanRunCard`
**Commit:** 3edfed0

Removed, along with its test.

### IN-06: Staff help line without a list marker
**Commit:** 123f760

The help text is fixed in all 7 locales and the docs were regenerated.

### IN-07: Broad revoke refusal mapping
**Commit:** 9d522be

Only Telegram descriptions that name the cause are mapped to a reason. A bare 403 or "chat not found" now gets the generic line.

### IN-08: Copies posted after a revoke lack the revoked footer
**Commit:** 048c0d9

Every copy posted after a revoke, alert or reminder, is now sent with the footers. Its stored `body` stays without them.

### IN-09: One revoke per lockdown hides Revoke for a later link
**Commit:** 123f760

The design is kept: a lockdown still allows one revoke. The lockdown help text in all 7 locales now says so, and tells admins to revoke any later link in Telegram's group settings.

## Skipped Issues

None.

## Verification

- `CGO_ENABLED=0 go build ./...` passes.
- `make check-translations` passes.
- `make check-docs` shows no drift.
- The full `make test` is run by the orchestrator after this report.
