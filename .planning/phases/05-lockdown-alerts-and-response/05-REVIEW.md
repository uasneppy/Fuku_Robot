---
phase: 05-lockdown-alerts-and-response
reviewed: 2026-10-11T00:00:00Z
depth: standard
files_reviewed: 49
files_reviewed_list:
  - AGENTS.md
  - Makefile
  - alita/db/approvals/repository.go
  - alita/db/approvals/repository_test.go
  - alita/db/lockdown/alerts.go
  - alita/db/lockdown/alerts_test.go
  - alita/db/lockdown/burst.go
  - alita/db/lockdown/burst_test.go
  - alita/db/lockdown/joiners.go
  - alita/db/lockdown/links.go
  - alita/db/lockdown/links_test.go
  - alita/db/lockdown/testmain_test.go
  - alita/db/models/lockdown.go
  - alita/modules/greetings_command_test.go
  - alita/modules/lockdown.go
  - alita/modules/lockdown_alert.go
  - alita/modules/lockdown_alert_auth_test.go
  - alita/modules/lockdown_alert_callbacks.go
  - alita/modules/lockdown_alert_close_test.go
  - alita/modules/lockdown_alert_delivery_test.go
  - alita/modules/lockdown_alert_lift_test.go
  - alita/modules/lockdown_alert_link_test.go
  - alita/modules/lockdown_alert_sample_test.go
  - alita/modules/lockdown_alert_test.go
  - alita/modules/lockdown_ban_run_test.go
  - alita/modules/lockdown_ban_test.go
  - alita/modules/lockdown_fake_test.go
  - alita/modules/lockdown_guard.go
  - alita/modules/lockdown_recent.go
  - alita/modules/lockdown_recent_test.go
  - alita/modules/lockdown_reminder_test.go
  - alita/modules/lockdown_response.go
  - alita/modules/lockdown_response_test.go
  - alita/modules/lockdown_revoke_test.go
  - alita/modules/lockdown_status.go
  - alita/modules/lockdown_worker.go
  - alita/modules/test_harness_test.go
  - docs/src/content/docs/commands/lockdown/index.md
  - docs/src/content/docs/commands/staff/index.md
  - locales/en.yml
  - locales/es.yml
  - locales/fr.yml
  - locales/hi.yml
  - locales/id.yml
  - locales/pt.yml
  - locales/ru.yml
  - main_test.go
  - migrations/20261011120000_add_lockdown_alerts.sql
  - migrations/20261011130000_add_lockdown_burst.sql
  - migrations/20261011140000_add_lockdown_ban_runs.sql
  - migrations/20261011150000_add_lockdown_link_revoke.sql
findings:
  critical: 0
  warning: 3
  info: 9
  total: 12
status: issues_found
---

# Phase 5: Code Review Report

**Reviewed:** 2026-10-11
**Depth:** standard
**Files Reviewed:** 49
**Status:** issues_found

## Summary

The review covered the Phase 5 lockdown alerts, reminders, and the Ban recent joiners, Revoke link and Lift buttons.

Files read:
- In full: `alita/modules/lockdown_alert.go`, `alita/modules/lockdown_alert_callbacks.go`, `alita/modules/lockdown_response.go`, `alita/db/lockdown/alerts.go`, `alita/db/lockdown/burst.go`, `alita/modules/lockdown_recent.go` and `alita/db/lockdown/links.go`.
- Diffs only: `lockdown.go`, `lockdown_guard.go`, `lockdown_worker.go`, `lockdown_status.go`, `joiners.go`, `approvals/repository.go`, the models, the migrations and the locale additions.

What holds up:
- **Authority:** a live `getChatMember` of the locked chat, taken from the lockdown row, runs at both Ask and Confirm, and a failed lookup refuses.
- **Exactly-once claims:** `BeginAlert`, `ClaimAlertSlot`, `RecordLinkRevoked`, `FinishBanRun` and `StartBanRun` use the right guards.
- **Bans:** another person's ban is never touched.
- **HTML escaping:** user-supplied names and links are escaped and spliced in after the translator.
- **Callback size:** the new callback data stays under 64 bytes.
- **Migrations:** append-only, one transaction each.
- **i18n:** all 98 `lockdown_*` keys are in all 7 locales with matching placeholders.

Nothing found is a correctness or security blocker. The three WARNINGs are:
- a result report that is delivered at most once (WR-01);
- a rule stated in AGENTS.md but not enforced in the Ban run (WR-02);
- missing PostgreSQL coverage for new conditional-update guarantees (WR-03).

## Warnings

### WR-01: Ban run result (card edit and #LOCKDOWN_BAN post) is lost for good when the first delivery fails

**File:** `alita/modules/lockdown_response.go:320-327, 473-505`

**Issue:** `lockdownProcessBanRun` calls `FinishBanRun`, and only the caller whose conditional update won calls `lockdownBanRunResult`. That function makes one paced edit of the card and one paced log post. It has no retry and no stored "result delivered" flag. Once `FinishBanRun` moves the run out of `running`, nothing ever looks at that run again.

The edit and the post can fail in three ways:
- `lockdownPaced` refuses with `ErrRateLimited`. This is likely right after a run has used the shared pacer for up to 50 bans plus lookups.
- `StopLockdownWorker` cancels the context.
- The winning replica dies.

If the edit fails, the card stays on "Banning N…" forever. If the post fails, the `#LOCKDOWN_BAN` post never goes out. Every other lockdown delivery resumes from the database; only this one does not.

**Fix:** Make delivery resumable. Add a `reported_at` column (new migration) on `chat_lockdown_ban_runs`. Add a worker step that claims finished runs with `reported_at IS NULL`, following the alert close step's pattern: claim, give the claim back on a rate limit, and stop after 3 attempts.

### WR-02: Ban run does not enforce "no permanent ban after a lift started" across the paced wait

**File:** `alita/modules/lockdown_response.go:298-316, 183-200`

**Issue:** The lockdown is read again before each account. After that read, `lockdownPaced` can wait up to 60 s for a slot before it calls `getChatMember` and `banChatMember`. If a lift is recorded during that wait, the account still gets its permanent ban, against D-12 and AGENTS.md. The wait is longest when the pacer is busiest, which is during a raid.

**Fix:** Read the lockdown again inside the paced unit, right before the ban call. If it is no longer active, restore the joiner row, put the account back in the queue and stop.

### WR-03: New PostgreSQL-only guarantees are not in the `test-postgres-integrity` run list

**File:** `Makefile:39-43`, `alita/db/lockdown/alerts.go:322, 493, 402`, `alita/db/lockdown/burst.go:263`

**Issue:** These guarantees run only on SQLite in CI:
- `RecordLinkRevoked`: one revoke wins.
- `PlanReminderSlots`: one reminder row per destination, through the slot unique index plus `ON CONFLICT DO NOTHING`.
- `FinishBanRun`: the `NOT EXISTS` guard.
- `RecordAlertSynced`: a parameterised `CASE` whose bound-parameter typing is specific to PostgreSQL.

**Fix:** Add their tests to the `-run` alternation of `test-postgres-integrity`.

## Info

### IN-01: A staff alert copy is posted to the destination chosen at plan time without re-checking the link

**File:** `alita/modules/lockdown_alert.go:562-620`

**Issue:** The log copy is checked again just before sending; the staff copy is not. If the group was unlinked while the copy was pending, the alert's details land in a former Staff Group.

**Fix:** Before sending a staff copy, check `staff.GetLinkOfGroupFresh(ld.ChatID)` and require `link.StaffChatID == slot.DestChatID`. Otherwise end the copy as failed with the detail "no longer linked".

### IN-02: A Revoke Confirm that loses the race can show a misleading refusal

**File:** `alita/modules/lockdown_alert_callbacks.go:656-668`

**Issue:** When two admins confirm Revoke together, Telegram can refuse the second call. That admin is then told the link "no longer works or was not made by me" instead of "already revoked by X".

**Fix:** In the refusal branch, read the lockdown again. When `LinkRevokedAt` is set, show `lockdownRevokeAlreadyText`.

### IN-03: The Lift confirm text says all removed people are unbanned

**File:** `locales/*.yml` (`lockdown_alert_lift_ask`)

**Issue:** Accounts banned for good by "Ban recent joiners" stay banned at the lift, but the Lift confirm text says everyone removed is unbanned.

**Fix:** Correct the text in all 7 locales.

### IN-04: One `attempts` counter serves post, refresh and close of an alert copy

**File:** `alita/db/lockdown/alerts.go:402-437`, `alita/modules/lockdown_alert.go:825, 949`

**Issue:** Failed refreshes use up the attempts that the close step later needs. A copy can then be closed as failed after a single close attempt and keep its live buttons.

**Fix:** Reset `attempts` when the close claim is made, or give each step its own counter.

### IN-05: `SetBanRunCard` is dead production code

**File:** `alita/db/lockdown/burst.go:247-257`

**Issue:** Only a test calls it.

**Fix:** Remove it and its test.

### IN-06: Staff help line added without a list marker

**File:** `docs/src/content/docs/commands/staff/index.md:18` (generated from `locales/en.yml`)

**Issue:** The new line has no list marker, so it renders as part of the paragraph above it.

**Fix:** Make it a bullet or its own paragraph in the source help text, then run `make generate-docs`.

### IN-07: Revoke refusal mapping is broad

**File:** `alita/modules/lockdown_alert_callbacks.go:507-529`

**Issue:** Two refusals are mapped too broadly:
- Any "not found" text, including "chat not found", is shown as "this link no longer works".
- Every 403 is shown as "I need the right to invite users", even when the bot was removed from the chat.

**Fix:** Match the specific descriptions and let other errors fall through to the generic line.

### IN-08: Alert copies posted after a revoke, and the Ban label, can be stale

**File:** `alita/modules/lockdown_alert.go:621-636`, `alita/db/lockdown/alerts.go:342-348`

**Issue:** Two things can go stale, though pressing a button is safe in both cases:
- An initial copy still pending when the revoke is recorded is posted without the "link revoked" footer.
- The Ban button's count is fixed when the copy is posted.

**Fix:** Add the footers to every kind of copy at post time.

### IN-09: One revoke per lockdown hides Revoke for a later raid link

**File:** `alita/modules/lockdown_alert.go:114`, `alita/db/lockdown/alerts.go:322`

**Issue:** This follows the documented decision of one revoke per lockdown. If the raid moves to another link the bot made, staff must revoke that link by hand.

**Fix:** State the limit in the help text, or record revokes per link.

---

_Reviewed: 2026-10-11_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
